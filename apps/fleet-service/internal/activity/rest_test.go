package activity

import (
	"testing"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/systemactor"
)

// The stored actor is a uuid (the column is `uuid NOT NULL`); the feed shows a
// word. Transform is the seam between the two, and the frontend's
// activityEventMeta.SYSTEM_ACTOR check is what depends on it: without the
// substitution the feed would try to resolve a row of zeroes to a display name
// against auth-service, and show the raw sentinel when that failed.
func TestTransform_rendersTheSystemSentinelAsAWord(t *testing.T) {
	m := NewBuilder().
		SetFleetID("fleet-1").
		SetActorUserID(systemactor.ID).
		SetType("schedule.overdue").
		Build()

	r, err := Transform(m)
	if err != nil {
		t.Fatalf("transform: %v", err)
	}
	attrs, ok := r.Attributes.(Attributes)
	if !ok {
		t.Fatalf("attributes = %T, want Attributes", r.Attributes)
	}
	if attrs.ActorUserID != systemactor.Label {
		t.Errorf("actorUserId = %q, want %q", attrs.ActorUserID, systemactor.Label)
	}
}

func TestTransform_leavesAHumanActorAlone(t *testing.T) {
	const human = "7a186017-d27e-4d65-90e3-6b240bf9880a"
	m := NewBuilder().
		SetFleetID("fleet-1").
		SetActorUserID(human).
		SetType("vehicle.created").
		Build()

	r, err := Transform(m)
	if err != nil {
		t.Fatalf("transform: %v", err)
	}
	attrs, _ := r.Attributes.(Attributes)
	if attrs.ActorUserID != human {
		t.Errorf("actorUserId = %q, want the id untouched", attrs.ActorUserID)
	}
}
