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

	"github.com/jtumidanski/myfleet/packages/shared-go/telemetry"
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

// ReassignRequest is the body both reassign-fleet endpoints accept. Each
// service reads only its own id list — media-service MediaIDs, notification
// -service VehicleIDs — and omitempty keeps the other out of the wire entirely
// rather than sending a null the receiver has to ignore.
type ReassignRequest struct {
	MediaIDs           []string `json:"media_ids,omitempty"`
	VehicleIDs         []string `json:"vehicle_ids,omitempty"`
	DestinationFleetID string   `json:"destination_fleet_id"`
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
	// One user action, one id, across every service it touches. Without this
	// the receiving service's CorrelationID middleware mints a FRESH id and the
	// two halves of a transfer — or of a purge fan-out — cannot be joined in the
	// logs. Set only when the context actually carries one, so an absent id
	// stays absent rather than becoming the empty string.
	if cid := telemetry.CorrelationIDFromContext(ctx); cid != "" {
		req.Header.Set(telemetry.HeaderCorrelationID, cid)
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
