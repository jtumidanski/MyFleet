package status

import (
	"testing"
	"time"
)

func TestDerive(t *testing.T) {
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name           string
		scheduleStates []string
		lastActivity   time.Time
		want           string
	}{
		{"overdue wins", []string{"overdue", "upcoming"}, now, "Overdue"},
		{"upcoming next", []string{"ok", "upcoming"}, now, "Upcoming Maintenance"},
		{"inactive when stale", []string{"ok"}, now.AddDate(-1, 0, -1), "Inactive"},
		{"healthy otherwise", []string{"ok"}, now, "Healthy"},
		{"no schedules + recent → healthy", nil, now, "Healthy"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Derive(Input{ScheduleStates: c.scheduleStates, LastActivityAt: c.lastActivity, Now: now, InactivityDays: 365})
			if got != c.want {
				t.Fatalf("Derive=%s want %s", got, c.want)
			}
		})
	}
}
