package user

import (
	"errors"
	"testing"

	"github.com/sirupsen/logrus"
)

// Note: a fake keyed by an opaque map cannot express which column the real
// query filters on, which is why the /auth/me login-loop bug was invisible
// here. The column-level guarantees live in provider_test.go, against a real
// database.
type fakeProvider struct {
	byID  map[string]Model
	bySub map[string]Model
}

func (f *fakeProvider) GetByID(id string) (Model, error) {
	if m, ok := f.byID[id]; ok {
		return m, nil
	}
	return Model{}, ErrNotFound
}

func (f *fakeProvider) GetBySub(sub string) (Model, error) {
	if m, ok := f.bySub[sub]; ok {
		return m, nil
	}
	return Model{}, ErrNotFound
}

type fakeAdmin struct{ created, updated int }

func (f *fakeAdmin) Insert(m Model) (Model, error) { f.created++; return m, nil }
func (f *fakeAdmin) Update(m Model) (Model, error) { f.updated++; return m, nil }

func TestProvisionFromGoogle_insertsWhenNew(t *testing.T) {
	w := &fakeAdmin{}
	p := NewProcessor(logrus.New(), &fakeProvider{bySub: map[string]Model{}}, w)
	got, err := p.ProvisionFromGoogle(GoogleProfile{Sub: "g1", Email: "a@b.com", Name: "A"})
	if err != nil {
		t.Fatal(err)
	}
	if w.created != 1 || got.GoogleSub() != "g1" {
		t.Fatalf("expected new user inserted; created=%d", w.created)
	}
}

func TestProvisionFromGoogle_updatesLoginWhenExisting(t *testing.T) {
	existing := NewBuilder().SetGoogleSub("g1").SetEmail("a@b.com").Build()
	w := &fakeAdmin{}
	p := NewProcessor(logrus.New(), &fakeProvider{bySub: map[string]Model{"g1": existing}}, w)
	if _, err := p.ProvisionFromGoogle(GoogleProfile{Sub: "g1", Email: "a@b.com"}); err != nil {
		t.Fatal(err)
	}
	if w.updated != 1 || w.created != 0 {
		t.Fatalf("expected update only; created=%d updated=%d", w.created, w.updated)
	}
}

func TestUpdateTheme_persistsEachValidValue(t *testing.T) {
	for _, pref := range []string{ThemeLight, ThemeDark, ThemeSystem} {
		t.Run(pref, func(t *testing.T) {
			existing := Make(Entity{ID: "u1", Email: "a@b.com", ThemePreference: ThemeSystem})
			w := &fakeAdmin{}
			p := NewProcessor(logrus.New(), &fakeProvider{byID: map[string]Model{"u1": existing}}, w)

			got, err := p.UpdateTheme("u1", pref)
			if err != nil {
				t.Fatalf("UpdateTheme(%q) returned %v", pref, err)
			}
			if got.ThemePreference() != pref {
				t.Fatalf("UpdateTheme(%q) returned preference %q", pref, got.ThemePreference())
			}
			if w.updated != 1 {
				t.Fatalf("expected exactly one persist call, got %d", w.updated)
			}
		})
	}
}

// An invalid value must be rejected BEFORE the read, so it can never leave a
// partially-applied state and costs no database round trip (design §7.2).
func TestUpdateTheme_rejectsInvalidWithoutTouchingStorage(t *testing.T) {
	for _, pref := range []string{"", "purple", "Dark"} {
		t.Run(pref, func(t *testing.T) {
			existing := Make(Entity{ID: "u1", ThemePreference: ThemeLight})
			w := &fakeAdmin{}
			p := NewProcessor(logrus.New(), &fakeProvider{byID: map[string]Model{"u1": existing}}, w)

			if _, err := p.UpdateTheme("u1", pref); !errors.Is(err, ErrInvalidTheme) {
				t.Fatalf("UpdateTheme(%q) = %v, want ErrInvalidTheme", pref, err)
			}
			if w.updated != 0 {
				t.Fatalf("UpdateTheme(%q) persisted %d time(s); an invalid value must not reach storage", pref, w.updated)
			}
		})
	}
}

func TestUpdateTheme_unknownUserIsNotFound(t *testing.T) {
	w := &fakeAdmin{}
	p := NewProcessor(logrus.New(), &fakeProvider{byID: map[string]Model{}}, w)

	if _, err := p.UpdateTheme("nobody", ThemeDark); !errors.Is(err, ErrNotFound) {
		t.Fatalf("UpdateTheme for an unknown user = %v, want ErrNotFound", err)
	}
}

// FR-ADMIN-AUTH-2: a bootstrap admin who does not exist at first migration gets
// the grant when they first sign in.
func TestProvisionFromGoogle_grantsBootstrapAdmin(t *testing.T) {
	var granted []string
	proc := NewProcessor(logrus.New(), &fakeProvider{}, &fakeAdmin{}).
		WithBootstrapAdmins(
			map[string]bool{"jtumidanski@gmail.com": true},
			func(userID string) error { granted = append(granted, userID); return nil },
		)

	if _, err := proc.ProvisionFromGoogle(GoogleProfile{
		Sub: "sub-1", Email: "JTumidanski@Gmail.com", Name: "J",
	}); err != nil {
		t.Fatalf("provision: %v", err)
	}
	if len(granted) != 1 {
		t.Fatalf("want one grant, got %v", granted)
	}
}

func TestProvisionFromGoogle_doesNotGrantOtherUsers(t *testing.T) {
	var granted []string
	proc := NewProcessor(logrus.New(), &fakeProvider{}, &fakeAdmin{}).
		WithBootstrapAdmins(
			map[string]bool{"jtumidanski@gmail.com": true},
			func(userID string) error { granted = append(granted, userID); return nil },
		)
	if _, err := proc.ProvisionFromGoogle(GoogleProfile{
		Sub: "sub-2", Email: "someone@example.com", Name: "Someone",
	}); err != nil {
		t.Fatalf("provision: %v", err)
	}
	if len(granted) != 0 {
		t.Errorf("a non-bootstrap email must not be granted admin: %v", granted)
	}
}

// A failing grant must not fail the login. The startup seed re-runs on every
// boot and will catch it; refusing to log the user in would be a worse outcome
// than a delayed grant.
func TestProvisionFromGoogle_survivesAFailingGrant(t *testing.T) {
	proc := NewProcessor(logrus.New(), &fakeProvider{}, &fakeAdmin{}).
		WithBootstrapAdmins(
			map[string]bool{"jtumidanski@gmail.com": true},
			func(string) error { return errors.New("database down") },
		)
	if _, err := proc.ProvisionFromGoogle(GoogleProfile{
		Sub: "sub-1", Email: "jtumidanski@gmail.com", Name: "J",
	}); err != nil {
		t.Fatalf("a failing admin grant must not fail login, got %v", err)
	}
}
