package systemactor_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/systemactor"
)

// The one property the whole package exists to hold. Every actor_user_id column
// in this service is `uuid NOT NULL`; a sentinel that is not a uuid is rejected
// by Postgres and takes its enclosing transaction down with it.
//
// No SQLite test can assert this — SQLite has no uuid type — so it is asserted
// on the constant directly.
func TestID_isAValidUUID(t *testing.T) {
	if _, err := uuid.Parse(systemactor.ID); err != nil {
		t.Fatalf("systemactor.ID (%q) is not a uuid: %v — Postgres will reject "+
			"every audit and activity row a background job writes", systemactor.ID, err)
	}
}

// The sentinel must never be mistakeable for a real account.
func TestID_isTheNilUUID(t *testing.T) {
	if uuid.MustParse(systemactor.ID) != uuid.Nil {
		t.Errorf("systemactor.ID = %q, want the nil uuid: a generated id could "+
			"collide with a real user and would not be stable across databases",
			systemactor.ID)
	}
}

func TestDisplay(t *testing.T) {
	if got := systemactor.Display(systemactor.ID); got != systemactor.Label {
		t.Errorf("Display(sentinel) = %q, want %q", got, systemactor.Label)
	}
	const human = "7a186017-d27e-4d65-90e3-6b240bf9880a"
	if got := systemactor.Display(human); got != human {
		t.Errorf("Display(%q) = %q, want it untouched", human, got)
	}
}

func TestResolve_isDisplaysInverse(t *testing.T) {
	if got := systemactor.Resolve(systemactor.Label); got != systemactor.ID {
		t.Errorf("Resolve(%q) = %q, want the sentinel", systemactor.Label, got)
	}
	const human = "7a186017-d27e-4d65-90e3-6b240bf9880a"
	if got := systemactor.Resolve(human); got != human {
		t.Errorf("Resolve(%q) = %q, want it untouched", human, got)
	}
	// Round-tripping what the console showed must find what the database holds.
	if got := systemactor.Resolve(systemactor.Display(systemactor.ID)); got != systemactor.ID {
		t.Errorf("Resolve(Display(sentinel)) = %q, want the sentinel back", got)
	}
}
