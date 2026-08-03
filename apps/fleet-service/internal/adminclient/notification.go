package adminclient

import (
	"context"
	"net/http"
	"net/url"
)

// NotificationClient calls notification-service's internal admin routes. base
// comes from NOTIFICATION_INTERNAL_URL.
type NotificationClient struct{ t transport }

// NewNotificationClient returns a client targeting the given
// notification-service base URL.
func NewNotificationClient(base string) *NotificationClient {
	return &NotificationClient{t: newTransport(base)}
}

// Stats returns the live notification count.
func (c *NotificationClient) Stats(ctx context.Context) (int, error) {
	var body struct {
		Notifications int `json:"notifications"`
	}
	if err := c.t.expectOK(ctx, http.MethodGet, "/internal/admin/stats", nil, &body); err != nil {
		return 0, err
	}
	return body.Notifications, nil
}

// Purge stamps notification-service's rows for the operation. It takes the same
// PurgeRequest as MediaClient; MediaIDs is simply never populated for it, since
// notifications are reachable from a fleet id alone.
func (c *NotificationClient) Purge(ctx context.Context, req PurgeRequest) (map[string]int, error) {
	var body affectedResponse
	if err := c.t.expectOK(ctx, http.MethodPost, "/internal/admin/purge", req, &body); err != nil {
		return nil, err
	}
	return body.Affected, nil
}

// Restore clears notification-service's stamp for the operation.
func (c *NotificationClient) Restore(ctx context.Context, opID string) (map[string]int, error) {
	var body affectedResponse
	if err := c.t.expectOK(ctx, http.MethodDelete,
		"/internal/admin/purge/"+url.PathEscape(opID), nil, &body); err != nil {
		return nil, err
	}
	return body.Affected, nil
}

// Reap hard-deletes notification-service's rows for the operation.
func (c *NotificationClient) Reap(ctx context.Context, opID string) (map[string]int, error) {
	var body affectedResponse
	if err := c.t.expectOK(ctx, http.MethodPost,
		"/internal/admin/reap/"+url.PathEscape(opID), nil, &body); err != nil {
		return nil, err
	}
	return body.Affected, nil
}
