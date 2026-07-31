// Package mediaclient is the internal HTTP client fleet-service uses to prove
// that a maintenance record's documentMediaIds belong to the caller's active
// fleet, via media-service's network-restricted (no-JWT) internal endpoint.
//
// Cross-service data is fetched over the API, never via a cross-service DB read
// (design D6). Modelled directly on
// apps/notification-service/internal/fleetclient.
package mediaclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// Media is one active media object returned by GET /internal/media.
type Media struct {
	ID          string `json:"id"`
	Status      string `json:"status"`
	ContentType string `json:"content_type"`
}

type response struct {
	Media []Media `json:"media"`
}

// Client calls media-service's internal endpoints. base is the service base URL
// (e.g. http://media-service:8080), from MEDIA_INTERNAL_URL.
type Client struct {
	base string
	hc   *http.Client
}

// NewClient returns a Client targeting the given media-service base URL.
func NewClient(base string) *Client { return &Client{base: base, hc: http.DefaultClient} }

// ValidateOwnership returns nil when every requested ID came back from
// media-service, which means every one is active AND in fleetID.
//
// A short set is server.ErrValidation (422). Whether an ID does not exist, was
// deleted, or belongs to another fleet is deliberately indistinguishable: a 403
// would confirm to the caller that the ID exists somewhere else.
//
// A transport failure or a non-200 propagates unchanged, so StatusFor maps it
// to 500 and no record is written. Failing closed is correct here even though
// it couples record creation to media-service availability, because the only
// requests affected are those that carry attachments (design D7).
//
// fleetID is a filter media-service applies, not an assertion it trusts.
func (c *Client) ValidateOwnership(ctx context.Context, fleetID string, mediaIDs []string) error {
	if len(mediaIDs) == 0 {
		return nil
	}

	q := url.Values{}
	q.Set("fleet_id", fleetID)
	q.Set("ids", strings.Join(mediaIDs, ","))
	endpoint := c.base + "/internal/media?" + q.Encode()

	var out response
	if err := c.getJSON(ctx, endpoint, &out); err != nil {
		return err
	}

	found := make(map[string]struct{}, len(out.Media))
	for _, m := range out.Media {
		found[m.ID] = struct{}{}
	}
	for _, id := range mediaIDs {
		if _, ok := found[id]; !ok {
			return fmt.Errorf("%w: attachment %s is not available to this fleet", server.ErrValidation, id)
		}
	}
	return nil
}

func (c *Client) getJSON(ctx context.Context, endpoint string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	res, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("media internal %s: status %d", endpoint, res.StatusCode)
	}
	return json.NewDecoder(res.Body).Decode(dst)
}
