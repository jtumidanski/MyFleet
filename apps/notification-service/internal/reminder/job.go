// Package reminder implements the daily maintenance-reminder safety-net (design
// §11, A6). Once a day it re-derives the currently upcoming/overdue schedules
// from fleet-service's internal feed and generates per-user overdue
// notifications using the EXACT same dedupe_key scheme as the event-path
// consumer, so the safety-net cannot double-fire against the event path. Runs
// under a Postgres advisory lock so only one replica fires per tick (design A9).
package reminder

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/database"
	"github.com/jtumidanski/myfleet/packages/shared-go/jobs"

	"github.com/jtumidanski/myfleet/apps/notification-service/internal/consumer"
	"github.com/jtumidanski/myfleet/apps/notification-service/internal/fleetclient"
	"github.com/jtumidanski/myfleet/apps/notification-service/internal/notification"
)

// Fleet is the fleet-service surface the reminder needs (satisfied by
// *fleetclient.Client).
type Fleet interface {
	DueSchedules(ctx context.Context) ([]fleetclient.DueSchedule, error)
	ActiveMembers(ctx context.Context, fleetID string) ([]fleetclient.Member, error)
}

// Generator generates one notification (satisfied by *notification.Processor).
type Generator interface {
	Generate(in notification.GenerateInput) error
}

// Job is the daily reminder safety-net.
type Job struct {
	log   logrus.FieldLogger
	db    *gorm.DB
	fleet Fleet
	gen   Generator
}

// NewJob constructs a reminder Job with its collaborators injected.
func NewJob(log logrus.FieldLogger, db *gorm.DB, fleet Fleet, gen Generator) *Job {
	return &Job{log: log, db: db, fleet: fleet, gen: gen}
}

// Start runs the safety-net every 24h under an advisory lock until ctx is
// cancelled.
func (j *Job) Start(ctx context.Context) {
	jobs.Every(ctx, 24*time.Hour, func(ctx context.Context) error {
		_, err := database.WithLeaderLock(j.db, "notification-reminder", func() error {
			return j.RunOnce(ctx)
		})
		if err != nil {
			j.log.WithError(err).Warn("notification reminder sweep failed")
		}
		return err
	})
}

// RunOnce performs one safety-net pass: fetch the due feed, resolve each fleet's
// members, and Generate per recipient with the same per-user dedupe_key the
// consumer uses for schedule.overdue. Generate is idempotent, so any schedule
// already notified via the event path is a no-op here.
func (j *Job) RunOnce(ctx context.Context) error {
	due, err := j.fleet.DueSchedules(ctx)
	if err != nil {
		return fmt.Errorf("fetch due schedules: %w", err)
	}

	// Cache members per fleet to avoid re-resolving for multiple schedules in the
	// same fleet during one pass.
	membersByFleet := map[string][]fleetclient.Member{}

	for _, d := range due {
		recipients, ok := membersByFleet[d.FleetID]
		if !ok {
			recipients, err = j.fleet.ActiveMembers(ctx, d.FleetID)
			if err != nil {
				return fmt.Errorf("resolve recipients for fleet %s: %w", d.FleetID, err)
			}
			membersByFleet[d.FleetID] = recipients
		}

		cycle := DueCycleToken(d)
		for _, r := range recipients {
			key := consumer.OverdueDedupeKey(r.UserID, d.ScheduleID, cycle)
			if err := j.gen.Generate(notification.GenerateInput{
				UserID:    r.UserID,
				Type:      "overdue",
				DedupeKey: key,
				Title:     "Maintenance overdue",
				Body:      "A maintenance schedule is overdue.",
				VehicleID: d.VehicleID,
				FleetID:   d.FleetID,
			}); err != nil {
				return fmt.Errorf("generate reminder for user %s: %w", r.UserID, err)
			}
		}
	}
	return nil
}

// DueCycleToken builds the due-window token from the internal feed's next-due
// fields, byte-identical to fleet-service's maintenanceschedule.DueCycleToken
// ("<next_due_date>|<next_due_mileage>"). The feed already formats next_due_date
// as RFC3339 (empty for pure-mileage schedules), so it is used verbatim.
func DueCycleToken(d fleetclient.DueSchedule) string {
	return fmt.Sprintf("%s|%d", d.NextDueDate, d.NextDueMileage)
}
