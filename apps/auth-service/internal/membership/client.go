// Package membership calls fleet-service's internal endpoint to resolve a user's
// active fleet/role for token minting (design §8.1; allowed under D2 — API, not join).
package membership

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

type Membership struct {
	FleetID string `json:"fleet_id"`
	Role    string `json:"role"`
}

type Client struct {
	base string
	hc   *http.Client
}

func NewClient(base string) *Client { return &Client{base: base, hc: http.DefaultClient} }

// fleetLookupTimeout bounds one auth→fleet hop, for BOTH calls in this file.
// The Client shares http.DefaultClient, which has NO timeout, so without this a
// wedged fleet-service pins an auth-service handler open indefinitely — and a
// pinned refresh handler is exactly the outage shape that used to end as a
// logout.
//
// A var rather than a const so the timeout test can drive the deadline in
// milliseconds instead of parking the suite for five seconds. Nothing in
// production writes it.
var fleetLookupTimeout = 5 * time.Second

func (c *Client) Active(ctx context.Context, userID string) (Membership, error) {
	ctx, cancel := context.WithTimeout(ctx, fleetLookupTimeout)
	defer cancel()

	// QueryEscape even though userID is an internal UUID off a validated token:
	// it costs nothing and stops this file contradicting FleetMemberIDs's own
	// comment, which names Active's raw concatenation as the habit not to
	// inherit.
	endpoint := c.base + "/internal/memberships/active?user_id=" + url.QueryEscape(userID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		// Deliberately NOT transient: a base URL that will not parse is our own
		// misconfiguration, and no amount of retrying fixes it.
		return Membership{}, err
	}
	res, err := c.hc.Do(req)
	if err != nil {
		// Connection refused, DNS failure, TLS error — and the deadline above,
		// which surfaces here as a *url.Error wrapping context.DeadlineExceeded.
		// All of them mean the answer is UNKNOWN, which is not the same as "this
		// user has no fleet" and must not end the session.
		//
		// url.Error's own message embeds the request URL, and this URL carries
		// the user id as a query parameter, so unwrap to the transport error
		// underneath: "connection refused", "no such host", "context deadline
		// exceeded" — the diagnostic value without the address.
		detail := err
		var urlErr *url.Error
		if errors.As(err, &urlErr) && urlErr.Err != nil {
			detail = urlErr.Err
		}
		return Membership{}, fmt.Errorf("%w: active membership lookup transport failure: %v",
			server.ErrServiceUnavailable, detail)
	}
	defer func() { _ = res.Body.Close() }()
	// 404 is the one status that is not a failure: a user with no fleet resolves
	// to a zero Membership, and the OIDC callback keys its onboarding redirect
	// off the resulting empty ActiveFleetID. Do not turn this into an error —
	// it would break a brand-new user's first login.
	if res.StatusCode == http.StatusNotFound {
		return Membership{}, nil
	}
	// 5xx and 429 are the upstream saying "not now", not "no". Classifying them
	// transient is what lets the caller answer 503 and keep the session, instead
	// of reading someone else's outage as a dead credential.
	if res.StatusCode >= 500 || res.StatusCode == http.StatusTooManyRequests {
		return Membership{}, fmt.Errorf("%w: active membership lookup failed with status %d",
			server.ErrServiceUnavailable, res.StatusCode)
	}
	// Every OTHER non-2xx must be an error, and a PERMANENT one. fleet-service's
	// error envelope is JSON, so without this check a non-2xx decodes cleanly
	// into a zero Membership with err == nil — indistinguishable from the 404
	// above. The caller then mints a valid token claiming the user has no fleet
	// and the SPA offers to create a duplicate one. A 400 or 403 here is a
	// contract or authorization fault between the two services that retrying
	// will not fix, so it stays off the transient path.
	//
	// Status code and a fixed description only: the body is upstream-controlled
	// and could carry anything, and the user id must not ride along in a
	// message that ends up in a log as an address.
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return Membership{}, fmt.Errorf("active membership lookup failed with status %d", res.StatusCode)
	}
	var m Membership
	if err := json.NewDecoder(res.Body).Decode(&m); err != nil {
		// Fixed text, and the decoder's error is dropped: its message quotes
		// bytes from an upstream-controlled body.
		return Membership{}, fmt.Errorf("%w: active membership lookup returned an unparseable body",
			server.ErrServiceUnavailable)
	}
	return m, nil
}

// FleetMemberIDs returns the user ids of a fleet's active members via
// fleet-service's EXISTING internal endpoint (GET /internal/fleets/{id}/members).
// No new fleet-service route is introduced, and the call direction is auth→fleet,
// the direction that already exists — so no import or dependency cycle.
//
// Unlike Active, 404 is an ERROR here, not a sentinel. Active maps 404 to a zero
// value because "this user has no fleet" is a real state; the fleet id passed
// here comes from a validated token, so its absence is a fault. Letting it mean
// "no members" would silently blank every name in the member list.
func (c *Client) FleetMemberIDs(ctx context.Context, fleetID string) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, fleetLookupTimeout)
	defer cancel()

	// PathEscape even though fleetID is a validated JWT claim and so is not
	// attacker-shaped: it costs nothing and stops the next caller inheriting
	// Active's raw-concatenation habit.
	endpoint := c.base + "/internal/fleets/" + url.PathEscape(fleetID) + "/members"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	res, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()

	// Every non-2xx is an error. fleet-service's error envelope is JSON, so
	// without this a 500 decodes cleanly into an empty slice with err == nil —
	// indistinguishable from a fleet that really has no members.
	//
	// Status code and a fixed description only: the body is upstream-controlled
	// and the fleet id must not ride along into a log line.
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return nil, fmt.Errorf("fleet member lookup failed with status %d", res.StatusCode)
	}

	var rows []struct {
		UserID string `json:"user_id"`
	}
	if err := json.NewDecoder(res.Body).Decode(&rows); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.UserID)
	}
	return out, nil
}
