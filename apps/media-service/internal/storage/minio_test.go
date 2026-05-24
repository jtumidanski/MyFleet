package storage

import (
	"strings"
	"testing"
)

func TestObjectKey_namespacedByFleet(t *testing.T) {
	k := ObjectKey("f1", "id-1", "My Receipt.pdf")
	if !strings.HasPrefix(k, "f1/id-1/") || strings.Contains(k, " ") {
		t.Fatalf("unexpected key %q", k)
	}
}

func TestObjectKey_slugifiesUnsafeChars(t *testing.T) {
	k := ObjectKey("fleet-a", "obj-2", "Some Weird/Name*?.JPG")
	if !strings.HasPrefix(k, "fleet-a/obj-2/") {
		t.Fatalf("expected fleet/obj prefix, got %q", k)
	}
	slug := strings.TrimPrefix(k, "fleet-a/obj-2/")
	for _, c := range []string{" ", "/", "*", "?"} {
		if strings.Contains(slug, c) {
			t.Fatalf("slug %q still contains unsafe char %q", slug, c)
		}
	}
	if slug != strings.ToLower(slug) {
		t.Fatalf("slug should be lowercase, got %q", slug)
	}
}

func TestObjectKey_emptyFilenameFallsBack(t *testing.T) {
	k := ObjectKey("f1", "id-1", "")
	if !strings.HasPrefix(k, "f1/id-1/") || strings.HasSuffix(k, "/") {
		t.Fatalf("empty filename must fall back to a non-empty slug, got %q", k)
	}
}
