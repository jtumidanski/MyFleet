package user

import "testing"

func TestIsValidTheme(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"light", ThemeLight, true},
		{"dark", ThemeDark, true},
		{"system", ThemeSystem, true},
		{"empty is not a preference", "", false},
		{"unknown value", "purple", false},
		{"case sensitive", "Dark", false},
		{"whitespace is not trimmed for us", " dark", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsValidTheme(tt.input); got != tt.want {
				t.Fatalf("IsValidTheme(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
