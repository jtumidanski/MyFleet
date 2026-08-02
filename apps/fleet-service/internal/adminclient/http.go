// Package adminclient holds fleet-service's HTTP clients for the other three
// services' internal admin routes.
//
// Modelled on internal/mediaclient: explicit timeout, never http.DefaultClient,
// context-aware, non-200 becomes an error. Cross-service data is fetched over
// the API, never via a cross-service DB read (design D6).
package adminclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// clientTimeout bounds every call. Two of these run on a user-facing request
// path (the purge create and the stats fan-out), so they cannot inherit
// http.DefaultClient's no-timeout behaviour: a stalled connection would hang the
// handler goroutine indefinitely, because the request context only cancels if
// the browser disconnects.
const clientTimeout = 5 * time.Second

// MaxLookupIDs bounds a single id-list query parameter, matching the ceiling
// auth-service and media-service enforce. Callers chunk larger sets.
const MaxLookupIDs = 50

// PurgeRequest is the one body shape both downstream purge endpoints accept.
// MediaIDs is media-service only.
type PurgeRequest struct {
	OperationID string   `json:"operation_id"`
	Scope       string   `json:"scope"`
	FleetIDs    []string `json:"fleet_ids,omitempty"`
	MediaIDs    []string `json:"media_ids,omitempty"`
}

// affectedResponse is the shape both services return from purge/restore/reap.
type affectedResponse struct {
	Affected map[string]int `json:"affected"`
}

type transport struct {
	base string
	hc   *http.Client
}

func newTransport(base string) transport {
	return transport{base: base, hc: &http.Client{Timeout: clientTimeout}}
}

func (t transport) do(ctx context.Context, method, path string, body, dst any) (int, error) {
	var rdr *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return 0, err
		}
		rdr = bytes.NewReader(raw)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req, err := http.NewRequestWithContext(ctx, method, t.base+path, rdr)
	if err != nil {
		return 0, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := t.hc.Do(req)
	if err != nil {
		return 0, err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode == http.StatusOK && dst != nil {
		if derr := json.NewDecoder(res.Body).Decode(dst); derr != nil {
			return res.StatusCode, fmt.Errorf("%s %s: decode: %w", method, path, derr)
		}
	}
	return res.StatusCode, nil
}

// expectOK turns any non-200 into an error, so a caller cannot mistake a 503 for
// an empty result.
func (t transport) expectOK(ctx context.Context, method, path string, body, dst any) error {
	status, err := t.do(ctx, method, path, body, dst)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("%s %s%s: status %d", method, t.base, path, status)
	}
	return nil
}

// chunk splits ids into batches of at most MaxLookupIDs.
func chunk(ids []string, size int) [][]string {
	var out [][]string
	for len(ids) > size {
		out = append(out, ids[:size])
		ids = ids[size:]
	}
	if len(ids) > 0 {
		out = append(out, ids)
	}
	return out
}
