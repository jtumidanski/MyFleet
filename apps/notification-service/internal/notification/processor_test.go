package notification

import (
	"testing"

	"github.com/sirupsen/logrus"
)

type fakeStore struct {
	existing map[string]bool // dedupe_key set
	inserts  int
}

func (f *fakeStore) ExistsByDedupeKey(k string) (bool, error) { return f.existing[k], nil }
func (f *fakeStore) Insert(n Model) error                     { f.inserts++; f.existing[n.DedupeKey()] = true; return nil }

type fakePrefs struct{ enabled map[string]bool }

func (f fakePrefs) Enabled(userID, typ string) (bool, error) { return f.enabled[typ], nil }

func TestGenerate_dedupesByKey(t *testing.T) {
	st := &fakeStore{existing: map[string]bool{}}
	p := NewProcessor(logrus.New(), st, fakePrefs{enabled: map[string]bool{"overdue": true}})
	in := GenerateInput{UserID: "u1", Type: "overdue", DedupeKey: "overdue:sched1:cycle3", Title: "Overdue"}
	_ = p.Generate(in)
	_ = p.Generate(in) // redelivery
	if st.inserts != 1 {
		t.Fatalf("dedupe_key must insert once, got %d", st.inserts)
	}
}

func TestGenerate_skipsWhenPreferenceDisabled(t *testing.T) {
	st := &fakeStore{existing: map[string]bool{}}
	p := NewProcessor(logrus.New(), st, fakePrefs{enabled: map[string]bool{"overdue": false}})
	_ = p.Generate(GenerateInput{UserID: "u1", Type: "overdue", DedupeKey: "k", Title: "x"})
	if st.inserts != 0 {
		t.Fatal("disabled preference must suppress generation")
	}
}
