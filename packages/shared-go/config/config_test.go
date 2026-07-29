package config

import "testing"

func TestGet_returnsDefaultWhenUnset(t *testing.T) {
	if got := Get("MYFLEET_MISSING_KEY", "fallback"); got != "fallback" {
		t.Fatalf("want fallback, got %q", got)
	}
}

func TestMustGet_panicsWhenUnset(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic for missing required key")
		}
	}()
	MustGet("MYFLEET_REQUIRED_MISSING")
}

func TestGetInt_parsesOrDefaults(t *testing.T) {
	t.Setenv("MYFLEET_PORT", "9090")
	if got := GetInt("MYFLEET_PORT", 8080); got != 9090 {
		t.Fatalf("want 9090, got %d", got)
	}
	if got := GetInt("MYFLEET_UNSET_INT", 8080); got != 8080 {
		t.Fatalf("want default 8080, got %d", got)
	}
}
