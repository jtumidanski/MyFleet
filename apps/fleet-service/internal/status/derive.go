// Package status derives a vehicle's status on read (design §10.2); never stored.
package status

import "time"

type Input struct {
	ScheduleStates []string // each "ok"|"upcoming"|"overdue"
	LastActivityAt time.Time
	Now            time.Time
	InactivityDays int // default 365
}

func Derive(in Input) string {
	for _, s := range in.ScheduleStates {
		if s == "overdue" {
			return "Overdue"
		}
	}
	for _, s := range in.ScheduleStates {
		if s == "upcoming" {
			return "Upcoming Maintenance"
		}
	}
	if in.LastActivityAt.Before(in.Now.AddDate(0, 0, -in.InactivityDays)) {
		return "Inactive"
	}
	return "Healthy"
}
