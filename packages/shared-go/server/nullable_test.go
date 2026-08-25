package server

import (
	"encoding/json"
	"testing"
)

func TestNullableUnmarshal(t *testing.T) {
	type payload struct {
		Due Nullable[string] `json:"due"`
	}

	cases := []struct {
		name        string
		body        string
		wantPresent bool
		wantValid   bool
		wantValue   string
	}{
		{"absent", `{}`, false, false, ""},
		{"explicit null", `{"due":null}`, true, false, ""},
		{"value", `{"due":"2026-11-30T00:00:00Z"}`, true, true, "2026-11-30T00:00:00Z"},
		{"empty string is a value", `{"due":""}`, true, true, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var p payload
			if err := json.Unmarshal([]byte(c.body), &p); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if p.Due.Present != c.wantPresent {
				t.Errorf("Present = %v want %v", p.Due.Present, c.wantPresent)
			}
			if p.Due.Valid != c.wantValid {
				t.Errorf("Valid = %v want %v", p.Due.Valid, c.wantValid)
			}
			if p.Due.Value != c.wantValue {
				t.Errorf("Value = %q want %q", p.Due.Value, c.wantValue)
			}
		})
	}
}

// A malformed value must surface as an error rather than silently decoding to
// the zero value: RegisterInputHandler turns a decode error into a 400.
func TestNullableUnmarshal_malformed(t *testing.T) {
	type payload struct {
		Due Nullable[string] `json:"due"`
	}
	var p payload
	if err := json.Unmarshal([]byte(`{"due":42}`), &p); err == nil {
		t.Fatal("want an error decoding a number into Nullable[string]")
	}
}

// Nullable[int] proves the type parameter is not string-specific.
func TestNullableUnmarshal_int(t *testing.T) {
	type payload struct {
		N Nullable[int] `json:"n"`
	}
	var p payload
	if err := json.Unmarshal([]byte(`{"n":7}`), &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !p.N.Present || !p.N.Valid || p.N.Value != 7 {
		t.Fatalf("got %+v, want present/valid/7", p.N)
	}
}
