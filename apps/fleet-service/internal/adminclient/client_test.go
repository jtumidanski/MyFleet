package adminclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAuthClient_UsersChunksLargeIDSets(t *testing.T) {
	var batches [][]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ids := strings.Split(r.URL.Query().Get("ids"), ",")
		batches = append(batches, ids)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"users":[]}`))
	}))
	defer srv.Close()

	ids := make([]string, MaxLookupIDs+5)
	for i := range ids {
		ids[i] = "u" + string(rune('a'+i%26))
	}
	if _, err := NewAuthClient(srv.URL).Users(context.Background(), ids); err != nil {
		t.Fatalf("users: %v", err)
	}
	if len(batches) != 2 {
		t.Fatalf("want 2 chunked requests, got %d", len(batches))
	}
	if len(batches[0]) != MaxLookupIDs {
		t.Errorf("first chunk = %d ids, want %d", len(batches[0]), MaxLookupIDs)
	}
}

// FR-ADMIN-AUTH-7 depends on this distinction: 404 means "not an admin", any
// other failure means "we could not tell" and must NOT read as false.
func TestAuthClient_IsPlatformAdmin(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		want    bool
		wantErr bool
	}{
		{"granted", http.StatusOK, true, false},
		{"revoked", http.StatusNotFound, false, false},
		{"service error", http.StatusInternalServerError, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
			}))
			defer srv.Close()
			got, err := NewAuthClient(srv.URL).IsPlatformAdmin(context.Background(), "u1")
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("IsPlatformAdmin = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestMediaClient_PurgeReturnsAffectedCounts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/internal/admin/purge" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"affected":{"media_objects":12,"media_variants":24}}`))
	}))
	defer srv.Close()

	got, err := NewMediaClient(srv.URL).Purge(context.Background(), PurgeRequest{
		OperationID: "op-1", Scope: "fleet", FleetIDs: []string{"f1"},
	})
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if got["media_objects"] != 12 || got["media_variants"] != 24 {
		t.Errorf("affected = %v", got)
	}
}

func TestMediaClient_PurgePropagatesANon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	if _, err := NewMediaClient(srv.URL).Purge(context.Background(), PurgeRequest{
		OperationID: "op-1", Scope: "system",
	}); err == nil {
		t.Error("a 503 must surface as an error so the operation is marked partial")
	}
}
