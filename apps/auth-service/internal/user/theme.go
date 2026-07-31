package user

// The complete set of theme preferences. Plain string constants rather than a
// named type: a named type would have to thread through Attributes, Entity and
// every test fixture for no benefit the allow-list below does not already
// provide, and no other enum in this codebase is modelled as one.
const (
	ThemeLight  = "light"
	ThemeDark   = "dark"
	ThemeSystem = "system"
)

// IsValidTheme is the single source of the allow-list. Everything that accepts
// a preference — the processor, the read-side normalisation in Make — goes
// through here, so adding a fourth theme is a one-line change.
func IsValidTheme(s string) bool {
	return s == ThemeLight || s == ThemeDark || s == ThemeSystem
}
