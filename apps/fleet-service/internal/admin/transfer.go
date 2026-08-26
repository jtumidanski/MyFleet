package admin

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// TransferSpec is the resolved, validated input to a vehicle transfer — the
// analogue of Root for the purge path.
//
// Now is a parameter rather than SQL now() because the entire test harness is
// SQLite, which has no now(). The production call site passes time.Now().UTC().
type TransferSpec struct {
	VehicleID     string
	SourceFleetID string
	DestFleetID   string
	// Label is the confirmation phrase (FR-XFER-CONF-2): the vehicle's
	// nickname, or "{year} {make} {model}" when it has none.
	Label       string
	ActorUserID string
	Now         time.Time
}

// TransferStep is one table the transfer counts and, sometimes, rewrites.
//
// Where and Set are used by BOTH CountTransfer and ApplyTransfer, which is what
// makes the blast-radius preview and the rows actually touched provably equal
// rather than equal by discipline — the same property admin.Count/Stamp have.
type TransferStep struct {
	// Key is the name used in affected_counts JSON and in the console's
	// blast-radius panel. It is API surface: renaming one is breaking.
	Key   string
	Table string
	// Where selects this table's rows for the moved vehicle. It binds exactly
	// one argument: the vehicle id.
	Where string
	// Set is the assignment ApplyTransfer writes, binding exactly one
	// argument: the destination fleet id.
	//
	// EMPTY MEANS COUNT-ONLY. Those tables key on vehicle_id alone, so they
	// follow the car for free and the transfer must not rewrite them
	// (FR-XFER-MOVE-2). The absence is the enforcement, and
	// TestTransferPlan_countOnlyStepsHaveNoSetClause pins it.
	Set string
}

// TransferPlan is the hand-enumerated list of tables a vehicle transfer
// touches, mirroring Manifest's role for a purge.
//
// The full fleet-service table inventory, with the transfer question answered
// for each (design §2.1):
//
//	fleet.vehicles                       rewritten explicitly, not a step (see ApplyTransfer)
//	fleet.activity_events                rewritten where vehicle_id matches
//	fleet.maintenance_categories         find-or-create in the destination (ResolveCategories)
//	fleet.dashboard_widgets              source-fleet rows pinned to the vehicle are DELETED (PruneWidgets)
//	fleet.maintenance_records            category_id remapped only
//	fleet.maintenance_schedules          category_id remapped only
//	fleet.maintenance_record_documents   untouched; source of receipt media ids
//	fleet.fuel_logs                      untouched
//	fleet.mileage_records                untouched
//	fleet.vehicle_media                  untouched; source of photo media ids
//	fleet.dashboards                     untouched (fleet-scoped, not vehicle-scoped)
//	fleet.fleets, fleet_memberships, fleet_invites  untouched (PRD non-goals)
//	fleet.purge_operations               untouched
//	fleet.admin_audit_events             one row appended
//	media.media_objects                  delegated to media-service
//	notification.notifications           delegated to notification-service
//
// If a future table gains a fleet_id or a vehicle reference, answer the
// transfer question here at the same time arch_test.go forces you to answer the
// purge question in Manifest.
var TransferPlan = []TransferStep{
	{Key: "maintenance_records", Table: "fleet.maintenance_records", Where: "vehicle_id = ?"},
	{Key: "maintenance_schedules", Table: "fleet.maintenance_schedules", Where: "vehicle_id = ?"},
	{Key: "fuel_logs", Table: "fleet.fuel_logs", Where: "vehicle_id = ?"},
	{Key: "mileage_records", Table: "fleet.mileage_records", Where: "vehicle_id = ?"},
	{Key: "vehicle_media", Table: "fleet.vehicle_media", Where: "vehicle_id = ?"},
	{
		// The car's timeline follows the car (FR-XFER-MOVE-3). Rows with a NULL
		// vehicle_id describe the FLEET and are never matched by this
		// predicate, so FR-XFER-MOVE-4 holds by construction.
		Key: "activity_events", Table: "fleet.activity_events",
		Where: "vehicle_id = ?", Set: "fleet_id = ?",
	},
}

// CountTransfer returns, per plan key, how many LIVE rows the transfer covers.
// It is the preview's only source for these keys and uses COUNT aggregates —
// no counted row is ever loaded (PRD §8 Performance).
func CountTransfer(db *gorm.DB, vehicleID string) (map[string]int, error) {
	out := make(map[string]int, len(TransferPlan))
	for _, s := range TransferPlan {
		var n int64
		q := "SELECT count(*) FROM " + s.Table + " WHERE (" + s.Where + ") AND deleted_at IS NULL"
		if err := db.Raw(q, vehicleID).Scan(&n).Error; err != nil {
			return nil, fmt.Errorf("count transfer %s: %w", s.Table, err)
		}
		out[s.Key] = int(n)
	}
	return out, nil
}

// VehicleMediaIDs is the set of media objects that must be re-homed with the
// vehicle (FR-XFER-MEDIA-2). Media objects carry a fleet_id, but "the media
// belonging to this vehicle" is a fact only fleet-service holds.
//
// Three sources, unioned:
//
//  1. the vehicle's photos (fleet.vehicle_media),
//  2. its primary image — a plain NOT NULL string column where "none" is the
//     EMPTY STRING, so it is filtered with a not-equal-to-empty test rather
//     than with IS NOT NULL,
//  3. receipts and attachments on its maintenance records
//     (fleet.maintenance_record_documents, the only attachment table).
//
// UNION, not UNION ALL: the primary image is usually also a vehicle_media row,
// and sending media-service the same id twice would double-count the read-back.
func VehicleMediaIDs(db *gorm.DB, vehicleID string) ([]string, error) {
	var ids []string
	q := `
		SELECT media_id FROM fleet.vehicle_media
		 WHERE vehicle_id = ? AND deleted_at IS NULL
		   AND media_id IS NOT NULL AND media_id <> ''
		UNION
		SELECT primary_image_media_id FROM fleet.vehicles
		 WHERE id = ? AND primary_image_media_id IS NOT NULL
		   AND primary_image_media_id <> ''
		UNION
		SELECT d.media_id FROM fleet.maintenance_record_documents d
		  JOIN fleet.maintenance_records m ON m.id = d.maintenance_record_id
		 WHERE m.vehicle_id = ? AND d.deleted_at IS NULL AND m.deleted_at IS NULL
		   AND d.media_id IS NOT NULL AND d.media_id <> ''`
	if err := db.Raw(q, vehicleID, vehicleID, vehicleID).Scan(&ids).Error; err != nil {
		return nil, fmt.Errorf("resolve transfer media ids: %w", err)
	}
	return ids, nil
}

// CategoryToCreate names a category the transfer would add to the destination
// fleet. The console lists these under the blast radius (FR-XFER-UI-4).
type CategoryToCreate struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
}

// sourceCategory is one fleet-scoped category the moved vehicle references.
type sourceCategory struct {
	ID          string
	Name        string
	Description string
	Kind        string
}

// candidateCategories returns the DISTINCT source-fleet categories the moved
// vehicle's live records and schedules point at.
//
// The `c.fleet_id = ?` predicate is what makes FR-XFER-CAT-2 hold by
// construction rather than by a check: a system category has a NULL fleet_id
// and can never satisfy it, so it is never a candidate and never remapped.
func candidateCategories(db *gorm.DB, spec TransferSpec) ([]sourceCategory, error) {
	var out []sourceCategory
	q := `
		SELECT DISTINCT c.id, c.name, c.description, c.kind
		  FROM fleet.maintenance_categories c
		 WHERE c.fleet_id = ?
		   AND (c.id IN (SELECT category_id FROM fleet.maintenance_records
		                  WHERE vehicle_id = ? AND deleted_at IS NULL)
		     OR c.id IN (SELECT category_id FROM fleet.maintenance_schedules
		                  WHERE vehicle_id = ? AND deleted_at IS NULL))`
	if err := db.Raw(q, spec.SourceFleetID, spec.VehicleID, spec.VehicleID).Scan(&out).Error; err != nil {
		return nil, fmt.Errorf("resolve source categories: %w", err)
	}
	return out, nil
}

// findDestinationCategory returns the id of the destination-fleet category
// matching name (case-INSENSITIVELY) and kind (EXACTLY), or "" if there is none.
//
// The lookup and the backing unique index deliberately disagree:
// idx_maintenance_categories_scope is case-SENSITIVE on (fleet_id, name, kind)
// and is a backstop against a double-submit, while this LOWER() comparison is
// the real user-facing match, consistent with the domain's own FindByName.
func findDestinationCategory(db *gorm.DB, destFleetID, name, kind string) (string, error) {
	var ids []string
	q := `SELECT id FROM fleet.maintenance_categories
	       WHERE fleet_id = ? AND LOWER(name) = LOWER(?) AND kind = ?
	       LIMIT 1`
	if err := db.Raw(q, destFleetID, name, kind).Scan(&ids).Error; err != nil {
		return "", fmt.Errorf("find destination category: %w", err)
	}
	if len(ids) == 0 {
		return "", nil
	}
	return ids[0], nil
}

// PreviewCategories names the categories a transfer would create in the
// destination fleet. It writes nothing — it runs the same candidate query and
// the same destination lookup ResolveCategories uses, and simply stops there.
func PreviewCategories(db *gorm.DB, spec TransferSpec) ([]CategoryToCreate, error) {
	cands, err := candidateCategories(db, spec)
	if err != nil {
		return nil, err
	}
	out := make([]CategoryToCreate, 0, len(cands))
	for _, c := range cands {
		existing, ferr := findDestinationCategory(db, spec.DestFleetID, c.Name, c.Kind)
		if ferr != nil {
			return nil, ferr
		}
		if existing == "" {
			out = append(out, CategoryToCreate{Name: c.Name, Kind: c.Kind})
		}
	}
	return out, nil
}

// ResolveCategories find-or-creates a destination-fleet equivalent for every
// source-fleet category the moved vehicle references, then rewrites
// category_id on the vehicle's records and schedules to point at it
// (FR-XFER-CAT-3/4/5). It returns how many categories it CREATED.
//
// Source categories are only ever read. They are never deleted, renamed or
// re-scoped even when the moved vehicle was their only consumer, because other
// vehicles and future records in the source fleet may still use them
// (FR-XFER-CAT-6).
func ResolveCategories(tx *gorm.DB, spec TransferSpec) (int, error) {
	cands, err := candidateCategories(tx, spec)
	if err != nil {
		return 0, err
	}
	created := 0
	for _, c := range cands {
		destID, rerr := resolveOneCategory(tx, spec.DestFleetID, c, &created)
		if rerr != nil {
			return 0, rerr
		}
		if err := remapCategory(tx, spec.VehicleID, c.ID, destID); err != nil {
			return 0, err
		}
	}
	return created, nil
}

// resolveOneCategory returns the destination id for one source category,
// inserting it if absent.
//
// A unique violation on the insert means a concurrent transfer created the same
// category between our lookup and our write. That is "someone else created it,
// re-read it", never a 500 — so the lookup runs once more and the winner is
// used. The re-read is bounded to one attempt, and it is a recovery, not a
// blanket catch: when it finds no winner the insert failed for some other
// reason, and that ORIGINAL insert error is returned rather than swallowed.
func resolveOneCategory(tx *gorm.DB, destFleetID string, c sourceCategory, created *int) (string, error) {
	existing, err := findDestinationCategory(tx, destFleetID, c.Name, c.Kind)
	if err != nil {
		return "", err
	}
	if existing != "" {
		return existing, nil
	}
	newID := uuid.NewString()
	ins := `INSERT INTO fleet.maintenance_categories
	          (id, name, description, system_defined, kind, fleet_id)
	        VALUES (?, ?, ?, ?, ?, ?)`
	if ierr := tx.Exec(ins, newID, c.Name, c.Description, false, c.Kind, destFleetID).Error; ierr != nil {
		winner, ferr := findDestinationCategory(tx, destFleetID, c.Name, c.Kind)
		if ferr != nil {
			return "", ferr
		}
		if winner == "" {
			return "", fmt.Errorf("create destination category %q: %w", c.Name, ierr)
		}
		return winner, nil
	}
	*created++
	return newID, nil
}

// remapCategory repoints the moved vehicle's rows from one source category to
// its destination equivalent. Two set-based statements; never a row-by-row loop.
func remapCategory(tx *gorm.DB, vehicleID, fromID, toID string) error {
	for _, table := range []string{"fleet.maintenance_records", "fleet.maintenance_schedules"} {
		q := "UPDATE " + table + " SET category_id = ?" +
			" WHERE vehicle_id = ? AND category_id = ? AND deleted_at IS NULL"
		if err := tx.Exec(q, toID, vehicleID, fromID).Error; err != nil {
			return fmt.Errorf("remap %s: %w", table, err)
		}
	}
	return nil
}

// widgetConfig is the only part of a dashboard widget's jsonb config this
// package reads. A widget that pins no vehicle unmarshals to the empty string
// and is skipped.
type widgetConfig struct {
	VehicleID string `json:"vehicleId"`
}

// WidgetsPinnedToVehicle returns the ids of live SOURCE-fleet dashboard widgets
// whose config pins the moved vehicle (FR-XFER-SRC-1/2/3).
//
// fleet.dashboard_widgets has no fleet_id of its own; it joins to
// fleet.dashboards, which does. That join is what scopes the candidate set to
// the source fleet, so destination-fleet and third-fleet widgets are never even
// considered.
//
// The vehicle match is made in Go, on the PARSED config, rather than in SQL.
// Postgres would express it as config->>'vehicleId' = ?, but every DB-backed
// test in this package runs on SQLite, which has no ->> operator. A dialect
// branch on a PREDICATE would mean the tested path and the production path are
// different SQL — exactly the class of bug that hid a broken local overlay for
// ten reviews. A one-off DDL branch is a different thing; this is not that.
//
// The candidate set is bounded by (members × widgets per dashboard) — one live
// dashboard per (fleet, user) is enforced by a partial unique index — so this is
// tens of rows, not thousands. The NFR's "never a row-by-row loop" is about the
// WRITE, and PruneWidgets is a single set-based DELETE.
//
// A malformed config is skipped rather than fatal, matching how MakeAudit
// tolerates malformed affected_counts.
func WidgetsPinnedToVehicle(db *gorm.DB, sourceFleetID, vehicleID string) ([]string, error) {
	var rows []struct {
		ID     string
		Config []byte
	}
	q := `SELECT w.id, w.config
	        FROM fleet.dashboard_widgets w
	        JOIN fleet.dashboards d ON d.id = w.dashboard_id
	       WHERE d.fleet_id = ? AND w.deleted_at IS NULL AND d.deleted_at IS NULL`
	if err := db.Raw(q, sourceFleetID).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("list source dashboard widgets: %w", err)
	}
	var ids []string
	for _, r := range rows {
		var cfg widgetConfig
		if err := json.Unmarshal(r.Config, &cfg); err != nil {
			continue
		}
		if cfg.VehicleID == vehicleID {
			ids = append(ids, r.ID)
		}
	}
	return ids, nil
}

// PruneWidgets hard-deletes the named widgets and returns how many rows went.
//
// A HARD delete, deliberately: FR-XFER-SRC-1 says "deleted", these rows carry
// no history, and a soft-deleted widget would still occupy its slot in the
// layout's position grid. This is the one place a transfer destroys data. It is
// bounded, it is reported as widgets_removed, and the operator sees the number
// in the blast-radius panel before they confirm.
func PruneWidgets(tx *gorm.DB, ids []string) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	res := tx.Exec(`DELETE FROM fleet.dashboard_widgets WHERE id IN ?`, ids)
	if res.Error != nil {
		return 0, fmt.Errorf("prune dashboard widgets: %w", res.Error)
	}
	return int(res.RowsAffected), nil
}

// Activity event types the transfer emits (FR-XFER-SRC-4).
//
// Declared here, local to internal/admin, rather than in a shared constants
// block: the eight existing event types are inline literals in the six domains
// that emit them, a shared block would have to live in internal/activity and be
// imported by all six, and doing that as a side effect of a transfer feature is
// unrelated refactoring. These two are emitted here, so they live here.
const (
	EventVehicleTransferredOut = "vehicle.transferred_out"
	EventVehicleTransferredIn  = "vehicle.transferred_in"
)

// transferPayload is the activity event body for both halves of a transfer.
type transferPayload struct {
	CounterpartFleetID string `json:"counterpart_fleet_id"`
	VehicleLabel       string `json:"vehicle_label"`
}

// validate rejects a spec whose ids are empty, or whose source and destination
// are the same fleet.
//
// This is not defensive boilerplate. An empty vehicle id is a WILDCARD, not a
// no-op: WidgetsPinnedToVehicle compares the PARSED config's vehicleId against
// it, so every widget whose config pins no vehicle would match and PruneWidgets
// would HARD DELETE widgets that have nothing to do with this transfer. An
// empty destination fleet id would likewise strand the vehicle in no fleet at
// all. Checked before the first statement runs, so a bad spec writes nothing.
//
// The same-fleet check is defense in depth, not the primary control: the API
// contract already specifies a 422 for a same-fleet destination, and Task 9
// rejects it earlier in the request path. It is repeated here because this is
// the destructive call site — a same-fleet spec would resolve every category
// to itself, insert two nonsense activity events, and hard-delete the fleet's
// own widgets pinned to the vehicle via PruneWidgets — and this guard should
// never be the one a user actually sees fire.
func (s TransferSpec) validate() error {
	switch {
	case s.VehicleID == "":
		return errors.New("transfer spec: vehicle id is required")
	case s.SourceFleetID == "":
		return errors.New("transfer spec: source fleet id is required")
	case s.DestFleetID == "":
		return errors.New("transfer spec: destination fleet id is required")
	case s.SourceFleetID == s.DestFleetID:
		return errors.New("transfer spec: source and destination fleet must differ")
	}
	return nil
}

// ApplyTransfer performs every LOCAL write a vehicle transfer needs and returns
// the affected counts.
//
// It must be called inside the caller's transaction — every statement below
// joins it, so any failure, including the downstream calls the processor makes
// afterwards, unwinds all of this (design D4).
//
// media_objects and notifications are NOT set here: those counts come from the
// downstream services' read-backs, and the processor merges them in.
func ApplyTransfer(tx *gorm.DB, spec TransferSpec) (map[string]int, error) {
	if err := spec.validate(); err != nil {
		return nil, err
	}

	// Counted FIRST, before any write. The two transfer events inserted at the
	// end match the activity_events predicate, and counting after them would
	// report two more rows than the operator was shown in the preview.
	counts, err := CountTransfer(tx, spec.VehicleID)
	if err != nil {
		return nil, err
	}

	// The single UPDATE the whole operation exists to perform. It names
	// fleet_id and updated_at and nothing else — in particular it never
	// mentions created_at, which is a stronger guarantee than GORM's
	// `<-:create` tag, since that only protects db.Save (FR-XFER-MOVE-1).
	if err := tx.Exec(`UPDATE fleet.vehicles SET fleet_id = ?, updated_at = ? WHERE id = ?`,
		spec.DestFleetID, spec.Now, spec.VehicleID).Error; err != nil {
		return nil, fmt.Errorf("move vehicle: %w", err)
	}

	// Every plan step that carries a Set clause. The count-only steps are
	// skipped by the same emptiness test that documents them.
	for _, s := range TransferPlan {
		if s.Set == "" {
			continue
		}
		q := "UPDATE " + s.Table + " SET " + s.Set +
			" WHERE (" + s.Where + ") AND deleted_at IS NULL"
		if err := tx.Exec(q, spec.DestFleetID, spec.VehicleID).Error; err != nil {
			return nil, fmt.Errorf("apply transfer %s: %w", s.Table, err)
		}
	}

	created, err := ResolveCategories(tx, spec)
	if err != nil {
		return nil, err
	}
	counts["categories_created"] = created

	widgetIDs, err := WidgetsPinnedToVehicle(tx, spec.SourceFleetID, spec.VehicleID)
	if err != nil {
		return nil, err
	}
	removed, err := PruneWidgets(tx, widgetIDs)
	if err != nil {
		return nil, err
	}
	counts["widgets_removed"] = removed

	// AFTER the bulk fleet_id rewrite, deliberately. The OUT event carries the
	// SOURCE fleet id and its vehicle_id matches the rewrite predicate, so
	// inserting it earlier would sweep it into the destination fleet and the
	// source fleet would have no record that the car left.
	if err := recordTransferEvent(tx, spec, EventVehicleTransferredOut,
		spec.SourceFleetID, spec.DestFleetID); err != nil {
		return nil, err
	}
	if err := recordTransferEvent(tx, spec, EventVehicleTransferredIn,
		spec.DestFleetID, spec.SourceFleetID); err != nil {
		return nil, err
	}

	return counts, nil
}

// recordTransferEvent appends one activity row. It writes raw SQL rather than
// calling internal/activity, because internal/admin never touches another
// domain's internals — and because internal/activity deliberately exposes no
// way to write a row with an arbitrary fleet id.
func recordTransferEvent(tx *gorm.DB, spec TransferSpec, eventType, fleetID, counterpartID string) error {
	payload, err := json.Marshal(transferPayload{
		CounterpartFleetID: counterpartID,
		VehicleLabel:       spec.Label,
	})
	if err != nil {
		return fmt.Errorf("marshal %s payload: %w", eventType, err)
	}
	q := `INSERT INTO fleet.activity_events
	        (id, fleet_id, vehicle_id, actor_user_id, type, payload, created_at)
	      VALUES (?, ?, ?, ?, ?, ?, ?)`
	if err := tx.Exec(q, uuid.NewString(), fleetID, spec.VehicleID,
		spec.ActorUserID, eventType, payload, spec.Now).Error; err != nil {
		return fmt.Errorf("record %s: %w", eventType, err)
	}
	return nil
}
