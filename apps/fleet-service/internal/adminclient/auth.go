package adminclient

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// User is one resolved user from auth-service's internal lookup.
type User struct {
	ID          string     `json:"id"`
	Email       string     `json:"email"`
	DisplayName string     `json:"display_name"`
	CreatedAt   time.Time  `json:"created_at"`
	LastLoginAt *time.Time `json:"last_login_at"`
}

// AuthClient calls auth-service's internal admin routes. base comes from
// AUTH_INTERNAL_URL.
type AuthClient struct{ t transport }

// NewAuthClient returns a client targeting the given auth-service base URL.
func NewAuthClient(base string) *AuthClient { return &AuthClient{t: newTransport(base)} }

// Stats returns the total user count.
func (c *AuthClient) Stats(ctx context.Context) (int, error) {
	var body struct {
		Users int `json:"users"`
	}
	if err := c.t.expectOK(ctx, http.MethodGet, "/internal/admin/stats", nil, &body); err != nil {
		return 0, err
	}
	return body.Users, nil
}

// Users resolves ids to email and display name, chunking the request so no
// single call exceeds the endpoint's bound. Ids that do not resolve are simply
// absent from the map; the caller decides whether that is a warning.
func (c *AuthClient) Users(ctx context.Context, ids []string) (map[string]User, error) {
	out := make(map[string]User, len(ids))
	for _, batch := range chunk(ids, MaxLookupIDs) {
		q := url.Values{}
		q.Set("ids", strings.Join(batch, ","))
		var body struct {
			Users []User `json:"users"`
			Total int    `json:"total"`
		}
		if err := c.t.expectOK(ctx, http.MethodGet, "/internal/admin/users?"+q.Encode(), nil, &body); err != nil {
			return nil, err
		}
		for _, u := range body.Users {
			out[u.ID] = u
		}
	}
	return out, nil
}

// ListUsers returns one page of the user directory plus the total
// (FR-ADMIN-FLEET-6).
//
// It is a distinct method from Users rather than an overload: the two differ in
// failure semantics. An unresolved id in Users is normal and produces a warning;
// a failure here means the directory page cannot be rendered at all.
func (c *AuthClient) ListUsers(ctx context.Context, page server.Page) ([]User, int, error) {
	q := url.Values{}
	q.Set("page[number]", strconv.Itoa(page.Number))
	q.Set("page[size]", strconv.Itoa(page.Size))
	var body struct {
		Users []User `json:"users"`
		Total int    `json:"total"`
	}
	if err := c.t.expectOK(ctx, http.MethodGet, "/internal/admin/users?"+q.Encode(), nil, &body); err != nil {
		return nil, 0, err
	}
	return body.Users, body.Total, nil
}

// IsPlatformAdmin re-verifies the privilege against auth.platform_admins
// (FR-ADMIN-AUTH-7).
//
// 404 means "not an admin" and is NOT an error. Anything else is: the caller
// must be able to tell "revoked" from "we could not reach auth-service", because
// the first is a 403 and the second is a 500 that stamps nothing.
func (c *AuthClient) IsPlatformAdmin(ctx context.Context, userID string) (bool, error) {
	status, err := c.t.do(ctx, http.MethodGet, "/internal/admin/platform-admins/"+url.PathEscape(userID), nil, nil)
	if err != nil {
		return false, err
	}
	switch status {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		return false, fmt.Errorf("GET platform-admins/%s: status %d", userID, status)
	}
}
