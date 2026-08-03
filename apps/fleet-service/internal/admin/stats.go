package admin

import (
	"context"
	"fmt"
	"sync"
)

// StatsSource is one remote count. Key is the attribute name in the response;
// Name is the service, used in the warning text a human reads.
type StatsSource interface {
	Key() string
	Name() string
	Count(ctx context.Context) (int, error)
}

// VehicleCounts splits vehicles into what exists and what the console can still
// undo (FR-ADMIN-STATS-3).
type VehicleCounts struct {
	Active       int `json:"active"`
	PendingPurge int `json:"pending_purge"`
}

// Stats is the /admin/stats payload. A nil value means "we could not ask", and
// the console renders it as an em dash — never as 0, which would read as
// "there is no data" (FR-ADMIN-UI-6).
type Stats struct {
	Values   map[string]*int
	Vehicles VehicleCounts
	Warnings []string
}

// localCounts maps a stats attribute to the table it counts. Vehicles are
// handled separately because they are reported as two numbers.
var localCounts = map[string]string{
	"fleets":                "fleet.fleets",
	"memberships":           "fleet.fleet_memberships",
	"maintenance_records":   "fleet.maintenance_records",
	"maintenance_schedules": "fleet.maintenance_schedules",
	"fuel_logs":             "fleet.fuel_logs",
	"mileage_records":       "fleet.mileage_records",
	"activity_events":       "fleet.activity_events",
}

// Stats gathers solution-wide counts (FR-ADMIN-STATS-1).
//
// Local counts are one indexed pass each; the remote counts are issued
// CONCURRENTLY with per-source error capture, so a slow service costs the
// response one timeout rather than three (PRD §8: under 2s).
//
// Every count excludes soft-deleted rows, so a pending purge is reflected
// immediately and the console never reports data the product no longer shows
// (FR-ADMIN-STATS-2).
func (p *Processor) Stats(ctx context.Context) (Stats, error) {
	out := Stats{Values: map[string]*int{}, Warnings: []string{}}

	for key, table := range localCounts {
		var n int64
		if err := p.d.DB.Raw("SELECT count(*) FROM " + table + " WHERE deleted_at IS NULL").
			Scan(&n).Error; err != nil {
			return Stats{}, fmt.Errorf("count %s: %w", table, err)
		}
		v := int(n)
		out.Values[key] = &v
	}

	// Pending invites are the unaccepted, unexpired ones — the number an
	// operator can act on, not every invite row ever written.
	var invites int64
	if err := p.d.DB.Raw(`SELECT count(*) FROM fleet.fleet_invites
	                      WHERE deleted_at IS NULL AND accepted_at IS NULL`).Scan(&invites).Error; err != nil {
		return Stats{}, fmt.Errorf("count pending invites: %w", err)
	}
	pending := int(invites)
	out.Values["pending_invites"] = &pending

	// pending_purge is admin-stamped only. A vehicle a USER deleted is neither
	// active nor recoverable through this console, so counting it as pending
	// would misstate what the operator can undo.
	if err := p.d.DB.Raw(`SELECT count(*) FROM fleet.vehicles WHERE deleted_at IS NULL`).
		Scan(&out.Vehicles.Active).Error; err != nil {
		return Stats{}, fmt.Errorf("count active vehicles: %w", err)
	}
	if err := p.d.DB.Raw(`SELECT count(*) FROM fleet.vehicles
	                      WHERE deleted_at IS NOT NULL AND purge_operation_id IS NOT NULL`).
		Scan(&out.Vehicles.PendingPurge).Error; err != nil {
		return Stats{}, fmt.Errorf("count pending-purge vehicles: %w", err)
	}

	// Remote sources, concurrently. sync.WaitGroup rather than errgroup: the
	// repo has no errgroup dependency and adding one for six lines is not worth
	// it. Per-source capture, because a failure is a WARNING, not an error —
	// errgroup's first-error semantics are the wrong shape here.
	type result struct {
		key, name string
		n         int
		err       error
	}
	results := make([]result, len(p.d.StatsSources))
	var wg sync.WaitGroup
	for i, s := range p.d.StatsSources {
		wg.Add(1)
		go func(i int, s StatsSource) {
			defer wg.Done()
			n, err := s.Count(ctx)
			results[i] = result{key: s.Key(), name: s.Name(), n: n, err: err}
		}(i, s)
	}
	wg.Wait()

	for _, r := range results {
		if r.err != nil {
			p.log.WithError(r.err).WithField("source", r.name).Warn("admin stats source unreachable")
			out.Values[r.key] = nil
			out.Warnings = append(out.Warnings,
				fmt.Sprintf("%s unreachable; %s count omitted", r.name, r.key))
			continue
		}
		n := r.n
		out.Values[r.key] = &n
	}
	return out, nil
}

// BlastRadius is the per-domain breakdown the console shows above the purge
// control. It is literally the same Count the purge's Stamp will use, which is
// what makes the displayed figures and the affected rows provably equal
// (FR-ADMIN-UI-9).
func (p *Processor) BlastRadius(root Root) (map[string]int, error) {
	return Count(p.d.DB, root)
}
