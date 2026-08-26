package adminclient_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/adminclient"
	"github.com/jtumidanski/myfleet/packages/shared-go/telemetry"
)

func TestMediaClient_Reassign_postsIDsAndParsesAffected(t *testing.T) {
	var gotPath string
	var gotBody adminclient.ReassignRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"affected":{"media_objects":9}}`))
	}))
	defer srv.Close()

	got, err := adminclient.NewMediaClient(srv.URL).
		Reassign(context.Background(), []string{"m1", "m2"}, "fleet-a", "fleet-b")
	if err != nil {
		t.Fatalf("reassign: %v", err)
	}
	if gotPath != "/internal/admin/reassign-fleet" {
		t.Errorf("path = %q", gotPath)
	}
	if len(gotBody.MediaIDs) != 2 || gotBody.DestinationFleetID != "fleet-b" {
		t.Errorf("body = %+v", gotBody)
	}
	// The source fleet is media-service's ownership predicate. Omitting it
	// would let the endpoint move an object belonging to any fleet at all.
	if gotBody.SourceFleetID != "fleet-a" {
		t.Errorf("source_fleet_id = %q, want fleet-a", gotBody.SourceFleetID)
	}
	if got["media_objects"] != 9 {
		t.Errorf("affected = %v, want media_objects 9", got)
	}
}

func TestNotificationClient_Reassign_sendsVehicleIDs(t *testing.T) {
	var gotBody adminclient.ReassignRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = w.Write([]byte(`{"affected":{"notifications":4}}`))
	}))
	defer srv.Close()

	got, err := adminclient.NewNotificationClient(srv.URL).
		Reassign(context.Background(), []string{"v1"}, "fleet-b")
	if err != nil {
		t.Fatalf("reassign: %v", err)
	}
	if len(gotBody.VehicleIDs) != 1 || gotBody.VehicleIDs[0] != "v1" {
		t.Errorf("vehicle_ids = %v", gotBody.VehicleIDs)
	}
	if len(gotBody.MediaIDs) != 0 {
		t.Errorf("media_ids should be omitted for notification-service, got %v", gotBody.MediaIDs)
	}
	// source_fleet_id is media-service's field. notification-service neither
	// sends nor reads it, and omitempty must keep it off this request entirely.
	if gotBody.SourceFleetID != "" {
		t.Errorf("source_fleet_id should be omitted for notification-service, got %q", gotBody.SourceFleetID)
	}
	if got["notifications"] != 4 {
		t.Errorf("affected = %v", got)
	}
}

// expectOK must not let a non-200 read as an empty result: a stranded transfer
// is the outcome that has to be impossible.
func TestMediaClient_Reassign_nonOKIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
	}))
	defer srv.Close()

	if _, err := adminclient.NewMediaClient(srv.URL).
		Reassign(context.Background(), []string{"m1"}, "fleet-a", "fleet-b"); err == nil {
		t.Fatal("expected an error for a 422 response")
	}
}

// NFR Observability: the correlation id must reach the downstream service. This
// also retroactively covers Purge/Restore/Reap/Stats, which share the transport.
func TestTransport_propagatesCorrelationID(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get(telemetry.HeaderCorrelationID)
		_, _ = w.Write([]byte(`{"affected":{}}`))
	}))
	defer srv.Close()

	ctx := telemetry.ContextWithCorrelationID(context.Background(), "corr-42")
	if _, err := adminclient.NewMediaClient(srv.URL).
		Reassign(ctx, []string{"m1"}, "fleet-a", "fleet-b"); err != nil {
		t.Fatalf("reassign: %v", err)
	}
	if got != "corr-42" {
		t.Errorf("X-Correlation-ID = %q, want corr-42", got)
	}
}

func TestTransport_omitsCorrelationIDWhenAbsent(t *testing.T) {
	var present bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, present = r.Header[http.CanonicalHeaderKey(telemetry.HeaderCorrelationID)]
		_, _ = w.Write([]byte(`{"affected":{}}`))
	}))
	defer srv.Close()

	if _, err := adminclient.NewMediaClient(srv.URL).
		Reassign(context.Background(), []string{"m1"}, "fleet-a", "fleet-b"); err != nil {
		t.Fatalf("reassign: %v", err)
	}
	if present {
		t.Error("X-Correlation-ID was sent for a context that carries none")
	}
}
