package user

import (
	"testing"

	"github.com/sirupsen/logrus"
)

type fakeProvider struct {
	bySub map[string]Model
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
