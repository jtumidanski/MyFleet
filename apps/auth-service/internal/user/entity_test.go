package user

import "testing"

// FR-DATA-4: a row written before the column existed, or edited out of band,
// must not surface an out-of-range value to clients. Normalising on read means
// GET /auth/me can promise the value is always one of the three.
func TestMake_normalisesUnknownStoredThemes(t *testing.T) {
	tests := []struct {
		name   string
		stored string
		want   string
	}{
		{"empty backfills to system", "", ThemeSystem},
		{"unknown value falls back to system", "purple", ThemeSystem},
		{"light survives", ThemeLight, ThemeLight},
		{"dark survives", ThemeDark, ThemeDark},
		{"system survives", ThemeSystem, ThemeSystem},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := Make(Entity{ID: "u1", ThemePreference: tt.stored})
			if got := m.ThemePreference(); got != tt.want {
				t.Fatalf("Make(Entity{ThemePreference: %q}).ThemePreference() = %q, want %q",
					tt.stored, got, tt.want)
			}
		})
	}
}

// The round trip must not drop the column, or Administrator.Update would write
// an empty string back over a good value on every login.
func TestToEntity_carriesThemePreference(t *testing.T) {
	m := Make(Entity{ID: "u1", ThemePreference: ThemeDark})
	if got := m.ToEntity().ThemePreference; got != ThemeDark {
		t.Fatalf("ToEntity().ThemePreference = %q, want %q", got, ThemeDark)
	}
}

func TestWithThemePreference_returnsACopy(t *testing.T) {
	original := Make(Entity{ID: "u1", ThemePreference: ThemeLight})
	updated := original.WithThemePreference(ThemeDark)

	if original.ThemePreference() != ThemeLight {
		t.Fatalf("WithThemePreference mutated the receiver: %q", original.ThemePreference())
	}
	if updated.ThemePreference() != ThemeDark {
		t.Fatalf("WithThemePreference returned %q, want %q", updated.ThemePreference(), ThemeDark)
	}
}

// Validation belongs to the processor (PRD §6.2), so the domain setter must
// accept whatever it is given. A setter that silently dropped bad input would
// make an invalid PATCH look like a success.
func TestWithThemePreference_doesNotValidate(t *testing.T) {
	if got := Make(Entity{ID: "u1"}).WithThemePreference("purple").ThemePreference(); got != "purple" {
		t.Fatalf("WithThemePreference(%q) = %q; the setter must not validate", "purple", got)
	}
}

// Design §3.4: ProvisionFromGoogle inserts via the builder, so the builder —
// not the Postgres column default — is what puts a real value on a new row.
// Whether GORM omits a zero-valued column carrying a `default:` tag and reads
// it back via RETURNING is version- and dialect-dependent; do not rely on it.
func TestNewBuilder_seedsSystem(t *testing.T) {
	if got := NewBuilder().Build().ThemePreference(); got != ThemeSystem {
		t.Fatalf("NewBuilder().Build().ThemePreference() = %q, want %q", got, ThemeSystem)
	}
}
