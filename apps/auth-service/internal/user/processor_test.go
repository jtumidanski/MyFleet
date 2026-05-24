package user

import (
	"testing"

	"github.com/sirupsen/logrus"
)

type fakeProvider struct{ byID map[string]Model; bySub map[string]Model }

func (f *fakeProvider) GetBySub(sub string) (Model, error) {
	if m, ok := f.bySub[sub]; ok {
		return m, nil
	}
	return Model{}, ErrNotFound
}

type fakeWriter struct{ created, updated int }

func (f *fakeWriter) Insert(m Model) (Model, error) { f.created++; return m, nil }
func (f *fakeWriter) Update(m Model) (Model, error) { f.updated++; return m, nil }

func TestProvisionFromGoogle_insertsWhenNew(t *testing.T) {
	p := NewProcessor(logrus.New(), &fakeProvider{bySub: map[string]Model{}})
	w := &fakeWriter{}
	got, err := p.ProvisionFromGoogle(w, GoogleProfile{Sub: "g1", Email: "a@b.com", Name: "A"})
	if err != nil {
		t.Fatal(err)
	}
	if w.created != 1 || got.GoogleSub() != "g1" {
		t.Fatalf("expected new user inserted; created=%d", w.created)
	}
}

func TestProvisionFromGoogle_updatesLoginWhenExisting(t *testing.T) {
	existing := NewBuilder().SetGoogleSub("g1").SetEmail("a@b.com").Build()
	p := NewProcessor(logrus.New(), &fakeProvider{bySub: map[string]Model{"g1": existing}})
	w := &fakeWriter{}
	if _, err := p.ProvisionFromGoogle(w, GoogleProfile{Sub: "g1", Email: "a@b.com"}); err != nil {
		t.Fatal(err)
	}
	if w.updated != 1 || w.created != 0 {
		t.Fatalf("expected update only; created=%d updated=%d", w.created, w.updated)
	}
}
