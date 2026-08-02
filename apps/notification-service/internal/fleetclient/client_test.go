package fleetclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestInvite_decodesTheInternalResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/invites/inv-1" {
			t.Errorf("path=%q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"invite_id":"inv-1","fleet_id":"f1","fleet_name":"The Smiths",
			"email":"a@b.com","role":"member","token":"tok-1",
			"expires_at":"2026-08-09T12:00:00Z","accepted_at":null,
			"invited_by_user_id":"u1"}`))
	}))
	defer srv.Close()

	got, err := NewClient(srv.URL).Invite(context.Background(), "inv-1")
	if err != nil {
		t.Fatalf("Invite: %v", err)
	}
	if got.InviteID != "inv-1" || got.FleetID != "f1" || got.FleetName != "The Smiths" {
		t.Fatalf("identity fields wrong: %+v", got)
	}
	if got.Token != "tok-1" || got.Email != "a@b.com" || got.Role != "member" {
		t.Fatalf("payload fields wrong: %+v", got)
	}
	if got.ExpiresAt != "2026-08-09T12:00:00Z" || got.AcceptedAt != nil {
		t.Fatalf("time fields wrong: %+v", got)
	}
}

// A 404 must be distinguishable from a blip, or the consumer retries four times
// against a row that will never exist.
func TestInvite_404IsASentinel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	if _, err := NewClient(srv.URL).Invite(context.Background(), "gone"); !errors.Is(err, ErrInviteNotFound) {
		t.Fatalf("err=%v want ErrInviteNotFound", err)
	}
}

// Any other non-200 stays a plain error, i.e. transient, and is retried.
func TestInvite_500IsNotTheSentinel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := NewClient(srv.URL).Invite(context.Background(), "inv-1")
	if err == nil || errors.Is(err, ErrInviteNotFound) {
		t.Fatalf("err=%v want a non-sentinel error", err)
	}
}

// The existing callers must keep behaving identically: *statusError still
// satisfies error and formats the same way.
func TestActiveMembers_nonOKStillErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	if _, err := NewClient(srv.URL).ActiveMembers(context.Background(), "f1"); err == nil {
		t.Fatal("want an error for a 502")
	}
}
