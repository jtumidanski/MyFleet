package maintenancecategory

import (
	"testing"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

func seededProvider(t *testing.T) Provider {
	t.Helper()
	db := newTestDB(t)
	if err := Seed(db); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return NewProvider(db)
}

func TestList_filtersByKindAndCountsAfterFiltering(t *testing.T) {
	p := seededProvider(t)
	page := server.Page{Number: 1, Size: 100}

	all, allTotal, err := p.List("", page)
	if err != nil {
		t.Fatalf("List(all): %v", err)
	}
	if allTotal != 20 || len(all) != 20 {
		t.Fatalf("unfiltered: len=%d total=%d, want 20/20", len(all), allTotal)
	}

	mods, modTotal, err := p.List(KindModification, page)
	if err != nil {
		t.Fatalf("List(modification): %v", err)
	}
	if modTotal != 12 || len(mods) != 12 {
		t.Fatalf("modification: len=%d total=%d, want 12/12", len(mods), modTotal)
	}
	for _, m := range mods {
		if m.Kind() != KindModification {
			t.Fatalf("%q leaked into the modification filter with kind %q", m.Name(), m.Kind())
		}
	}

	maint, maintTotal, err := p.List(KindMaintenance, page)
	if err != nil {
		t.Fatalf("List(maintenance): %v", err)
	}
	if maintTotal != 8 || len(maint) != 8 {
		t.Fatalf("maintenance: len=%d total=%d, want 8/8", len(maint), maintTotal)
	}
}

// The total must reflect the count AFTER the filter, across more than one page.
func TestList_filteredTotalSurvivesPaging(t *testing.T) {
	p := seededProvider(t)

	first, total, err := p.List(KindModification, server.Page{Number: 1, Size: 5})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 12 {
		t.Fatalf("total = %d, want the filtered count 12", total)
	}
	if len(first) != 5 {
		t.Fatalf("page 1 len = %d, want 5", len(first))
	}

	third, _, err := p.List(KindModification, server.Page{Number: 3, Size: 5})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(third) != 2 {
		t.Fatalf("page 3 len = %d, want 2", len(third))
	}
}

func TestIDsByKind(t *testing.T) {
	p := seededProvider(t)

	ids, err := p.IDsByKind(KindModification)
	if err != nil {
		t.Fatalf("IDsByKind: %v", err)
	}
	if len(ids) != 12 {
		t.Fatalf("len(ids) = %d, want 12", len(ids))
	}
}

// A kind with no rows must yield an empty NON-NIL slice: the record provider
// reads nil as "no filter" and empty-non-nil as "match nothing" (design D3).
func TestIDsByKind_emptyResultIsNonNil(t *testing.T) {
	db := newTestDB(t) // no Seed — the table is empty
	ids, err := NewProvider(db).IDsByKind(KindModification)
	if err != nil {
		t.Fatalf("IDsByKind: %v", err)
	}
	if ids == nil {
		t.Fatal("IDsByKind returned nil for an empty result; nil means 'no filter' downstream")
	}
	if len(ids) != 0 {
		t.Fatalf("len(ids) = %d, want 0", len(ids))
	}
}
