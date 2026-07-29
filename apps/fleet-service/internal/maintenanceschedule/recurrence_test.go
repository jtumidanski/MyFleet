package maintenanceschedule

import (
	"testing"
	"time"
)

var base = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func TestNextDue(t *testing.T) {
	cases := []struct {
		name          string
		recur         string
		months, miles int
		lastDate      time.Time
		lastMiles     int
		wantDate      time.Time
		wantMiles     int
	}{
		{"time", "time", 12, 0, base, 0, base.AddDate(0, 12, 0), 0},
		{"mileage", "mileage", 0, 5000, base, 30000, time.Time{}, 35000},
		{"hybrid", "hybrid", 12, 5000, base, 30000, base.AddDate(0, 12, 0), 35000},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := Schedule{RecurrenceType: c.recur, IntervalMonths: c.months, IntervalMiles: c.miles, LastCompletedDate: c.lastDate, LastCompletedMileage: c.lastMiles}
			nd, nm := NextDue(s)
			if c.recur != "mileage" && !nd.Equal(c.wantDate) {
				t.Fatalf("next_due_date = %v want %v", nd, c.wantDate)
			}
			if c.recur != "time" && nm != c.wantMiles {
				t.Fatalf("next_due_mileage = %d want %d", nm, c.wantMiles)
			}
		})
	}
}

func TestDueState(t *testing.T) {
	s := Schedule{RecurrenceType: "hybrid", IntervalMonths: 12, IntervalMiles: 5000, LastCompletedDate: base, LastCompletedMileage: 30000}
	// today far before due, mileage well under → ok
	if got := DueState(s, base.AddDate(0, 1, 0), 31000, DefaultThresholds); got != "ok" {
		t.Fatalf("want ok, got %s", got)
	}
	// mileage within 500 of 35000 → upcoming
	if got := DueState(s, base.AddDate(0, 1, 0), 34600, DefaultThresholds); got != "upcoming" {
		t.Fatalf("want upcoming (within 500mi), got %s", got)
	}
	// past 35000 → overdue
	if got := DueState(s, base.AddDate(0, 1, 0), 35001, DefaultThresholds); got != "overdue" {
		t.Fatalf("want overdue (mileage exceeded), got %s", got)
	}
	// time past next_due_date → overdue
	if got := DueState(s, base.AddDate(0, 13, 0), 31000, DefaultThresholds); got != "overdue" {
		t.Fatalf("want overdue (time exceeded), got %s", got)
	}
}

func TestSeverity(t *testing.T) {
	if Severity("ok") != "informational" || Severity("upcoming") != "recommended" || Severity("overdue") != "urgent" {
		t.Fatal("severity bands per design §10.1")
	}
}
