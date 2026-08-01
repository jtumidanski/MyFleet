package mediaobject

import (
	"errors"
	"testing"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// TestParseContentVariant covers the whole accepted set plus the two shapes that
// must NOT be accepted. A silent fallback to the original on an unknown value
// would ship multi-megabyte responses for a typo, which is the reason this
// returns an error instead.
func TestParseContentVariant(t *testing.T) {
	valid := map[string]ContentVariant{
		"":          ContentOriginal, // parameter absent, or ?variant= with no value
		"original":  ContentOriginal,
		"thumbnail": ContentThumbnail,
		"display":   ContentDisplay,
	}
	for raw, want := range valid {
		got, err := ParseContentVariant(raw)
		if err != nil {
			t.Fatalf("ParseContentVariant(%q) returned error %v, want %q", raw, err, want)
		}
		if got != want {
			t.Fatalf("ParseContentVariant(%q) = %q, want %q", raw, got, want)
		}
	}

	// Exact lowercase matching only: a wrong-case value and an unknown value are
	// both 400s, never a silent fallback.
	for _, raw := range []string{"Thumbnail", "THUMBNAIL", "bogus", "small", " thumbnail"} {
		got, err := ParseContentVariant(raw)
		if !errors.Is(err, server.ErrBadRequest) {
			t.Fatalf("ParseContentVariant(%q) = (%q, %v), want ErrBadRequest", raw, got, err)
		}
	}
}
