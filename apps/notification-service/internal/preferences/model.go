package preferences

// Model is the immutable per-user/per-type notification preference (PRD §6).
// A missing row is treated as "enabled" by the processor, so users receive
// notifications before they customize anything.
type Model struct {
	id           string
	userID       string
	typ          string
	inAppEnabled bool
}

func (m Model) ID() string         { return m.id }
func (m Model) UserID() string     { return m.userID }
func (m Model) Type() string       { return m.typ }
func (m Model) InAppEnabled() bool { return m.inAppEnabled }

// WithInAppEnabled returns a copy with the in-app toggle changed.
func (m Model) WithInAppEnabled(enabled bool) Model {
	m.inAppEnabled = enabled
	return m
}
