package adminclient

import (
	"context"
	"net/http"
	"net/url"
)

// MediaClient calls media-service's internal admin routes. base comes from
// MEDIA_INTERNAL_URL.
type MediaClient struct{ t transport }

// NewMediaClient returns a client targeting the given media-service base URL.
func NewMediaClient(base string) *MediaClient { return &MediaClient{t: newTransport(base)} }

// Stats returns the live media-object count.
func (c *MediaClient) Stats(ctx context.Context) (int, error) {
	var body struct {
		MediaObjects int `json:"media_objects"`
	}
	if err := c.t.expectOK(ctx, http.MethodGet, "/internal/admin/stats", nil, &body); err != nil {
		return 0, err
	}
	return body.MediaObjects, nil
}

// Purge stamps media-service's rows for the operation. Idempotent: a replay
// returns the same counts (FR-ADMIN-PURGE-10), which is what makes the retry
// endpoint safe to press repeatedly.
func (c *MediaClient) Purge(ctx context.Context, req PurgeRequest) (map[string]int, error) {
	var body affectedResponse
	if err := c.t.expectOK(ctx, http.MethodPost, "/internal/admin/purge", req, &body); err != nil {
		return nil, err
	}
	return body.Affected, nil
}

// Restore clears media-service's stamp for the operation.
func (c *MediaClient) Restore(ctx context.Context, opID string) (map[string]int, error) {
	var body affectedResponse
	if err := c.t.expectOK(ctx, http.MethodDelete,
		"/internal/admin/purge/"+url.PathEscape(opID), nil, &body); err != nil {
		return nil, err
	}
	return body.Affected, nil
}

// Reap hard-deletes media-service's rows for the operation and removes the
// backing MinIO objects.
func (c *MediaClient) Reap(ctx context.Context, opID string) (map[string]int, error) {
	var body affectedResponse
	if err := c.t.expectOK(ctx, http.MethodPost,
		"/internal/admin/reap/"+url.PathEscape(opID), nil, &body); err != nil {
		return nil, err
	}
	return body.Affected, nil
}

// Reassign re-homes the named media objects to another fleet, which is what
// keeps a transferred vehicle's photos and receipts readable by the destination
// fleet's members — media-service gates access on fleet_id equality and
// otherwise answers 404.
//
// Idempotent (FR-XFER-MEDIA-4): media-service reads the count back rather than
// taking RowsAffected, so a replay changes nothing and reports the same number.
// That is what makes the compensating reverse call safe to attempt.
//
// The ids are sent unchunked, like Purge's. MaxLookupIDs bounds QUERY-PARAMETER
// lookups; this is a POST body. Chunking would reintroduce partial application
// across chunks, which is the one outcome this operation must not have.
func (c *MediaClient) Reassign(ctx context.Context, mediaIDs []string, destFleetID string) (map[string]int, error) {
	var body affectedResponse
	req := ReassignRequest{MediaIDs: mediaIDs, DestinationFleetID: destFleetID}
	if err := c.t.expectOK(ctx, http.MethodPost, "/internal/admin/reassign-fleet", req, &body); err != nil {
		return nil, err
	}
	return body.Affected, nil
}
