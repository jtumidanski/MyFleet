package admin

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// NewProcessorClockForTest returns a copy of the processor with a fixed clock.
// Tests use it to wind past the recovery window without sleeping; production
// code uses the injected Deps.Now.
func NewProcessorClockForTest(p *Processor, now time.Time) *Processor {
	cp := *p
	cp.d.Now = func() time.Time { return now }
	return &cp
}

// ReapDue hard-deletes every operation whose recovery window has elapsed.
//
// Run hourly under database.WithLeaderLock(db, "admin-purge-reap", …). Hourly,
// not daily: jobs.Every's FIRST tick is at T+interval, so a 24-hour job in a
// service that redeploys more often than daily never runs at all — and the
// console shows a countdown to permanence, which a daily cadence would make
// wrong by up to 24 hours (design OQ-5). The tick is cheap: an indexed
// status/purge_after scan that is a no-op on almost every run.
//
// One operation failing never aborts the run: the others are independent, and a
// failure simply leaves that operation pending for the next tick.
func (p *Processor) ReapDue(ctx context.Context) error {
	now := p.d.Now()
	due, err := p.d.Provider.ListDue(now)
	if err != nil {
		return err
	}
	if len(due) == 0 {
		return nil
	}

	var reaped int
	totals := map[string]int{}
	for _, op := range due {
		deleted, rerr := p.reapOne(ctx, op, now)
		if rerr != nil {
			p.log.WithError(rerr).WithField("operation_id", op.ID()).
				Warn("reap failed; the operation stays due and the next tick retries it")
			continue
		}
		reaped++
		for k, v := range deleted {
			totals[k] += v
		}
	}

	// One summary line per run (PRD §8 Observability).
	p.log.WithFields(logrus.Fields{
		"operations_due":    len(due),
		"operations_reaped": reaped,
		"rows_deleted":      totals,
	}).Info("admin purge reaper run complete")
	return nil
}

// reapOne runs the destructive sequence for a single operation.
//
// Order matters exactly once, here: downstream BEFORE local. The local Reap
// destroys the purge_operation_id values, which are the only handle the
// downstream calls have; a crash between the two must leave enough state to
// retry. Every step keys on that id and is idempotent, so a crash anywhere
// leaves the operation pending and the next tick re-runs it
// (FR-ADMIN-RESTORE-6).
func (p *Processor) reapOne(ctx context.Context, op Operation, now time.Time) (map[string]int, error) {
	for _, d := range p.d.Downstream {
		if _, err := d.Reap(ctx, op.ID()); err != nil {
			// Abort THIS operation. Marking it reaped now would strip the ids
			// the next attempt needs and strand the downstream rows forever.
			return nil, err
		}
	}

	var deleted map[string]int
	if err := p.d.DB.Transaction(func(tx *gorm.DB) error {
		var rerr error
		deleted, rerr = Reap(tx, op.ID())
		if rerr != nil {
			return rerr
		}
		if serr := p.d.Administrator.SetStatus(tx, op.ID(), StatusReaped, nil, now); serr != nil {
			return serr
		}
		// Actor is the system: attributing a scheduled deletion to the person
		// who requested it days earlier would misread the trail
		// (FR-ADMIN-UI-13).
		return p.d.Administrator.InsertAudit(tx, AuditEvent{
			ID:               uuid.NewString(),
			ActorUserID:      ActorSystem,
			ActorEmail:       ActorSystem,
			Action:           ActionPurgeReaped,
			Scope:            string(op.Scope()),
			TargetType:       op.TargetType(),
			TargetID:         op.TargetID(),
			TargetLabel:      op.TargetLabel(),
			PurgeOperationID: op.ID(),
			AffectedCounts:   deleted,
			CreatedAt:        now,
		})
	}); err != nil {
		return nil, err
	}
	return deleted, nil
}
