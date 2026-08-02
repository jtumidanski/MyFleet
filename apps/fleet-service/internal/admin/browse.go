package admin

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/adminclient"
	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// DeletedFilter is the tri-state ?deleted= parameter.
//
// It replaces the PRD's ?include_deleted= boolean because the PRD contradicted
// itself: FR-ADMIN-FLEET-2 wanted soft-deleted fleets hidden by default,
// FR-ADMIN-UI-7 wanted them struck through with a countdown rather than
// vanishing. The second is right — a console whose recovery window is invisible
// by default hides the thing it exists to let you undo (design OQ-4).
type DeletedFilter string

const (
	// DeletedInclude is the default: live fleets plus those pending purge.
	DeletedInclude DeletedFilter = "include"
	// DeletedExclude is the "show me the live platform" view.
	DeletedExclude DeletedFilter = "exclude"
	// DeletedOnly is the "what is pending" view.
	DeletedOnly DeletedFilter = "only"
)

// ParseDeletedFilter reads the query parameter, defaulting to include.
func ParseDeletedFilter(raw string) (DeletedFilter, error) {
	switch DeletedFilter(raw) {
	case "":
		return DeletedInclude, nil
	case DeletedInclude, DeletedExclude, DeletedOnly:
		return DeletedFilter(raw), nil
	}
	return "", server.Detailed(server.ErrValidation, "deleted must be include, exclude or only")
}

// VehicleStatusDeriver derives one vehicle's read-only status.
//
// Declared as a port rather than importing the vehicle domain, so this package
// never calls another domain's internals directly; main.go injects an adapter
// over vehicle.StatusDeps, exactly as the vehicle resource already receives one.
// A nil deriver simply omits status, which is also what the underlying
// machinery does on a gather error.
type VehicleStatusDeriver interface {
	DeriveStatusByID(vehicleID string, now time.Time) string
}

// FleetRow is one row of the admin fleet list. Counts come from aggregate
// queries — the list NEVER loads any fleet's vehicles (PRD §8 Performance).
type FleetRow struct {
	ID               string
	Name             string
	CreatedAt        time.Time
	OwnerUserID      string
	OwnerEmail       string
	OwnerDisplayName string
	MemberCount      int
	VehicleCount     int
	PendingPurge     bool
	PurgeAfter       *time.Time
}

// FleetPage is a page of the fleet list plus any degradation warnings.
type FleetPage struct {
	Rows     []FleetRow
	Total    int
	Warnings []string
}

// MemberRow is one membership in the fleet inspector.
type MemberRow struct {
	UserID      string    `json:"user_id"`
	Email       string    `json:"email"`
	DisplayName string    `json:"display_name"`
	Role        string    `json:"role"`
	Status      string    `json:"status"`
	JoinedAt    time.Time `json:"joined_at"`
}

// VehicleRow is one vehicle in the fleet inspector.
type VehicleRow struct {
	ID           string `json:"id"`
	Nickname     string `json:"nickname"`
	Make         string `json:"make"`
	Model        string `json:"model"`
	Year         int    `json:"year"`
	Mileage      int    `json:"mileage"`
	Status       string `json:"status"`
	PendingPurge bool   `json:"pending_purge"`
}

// InviteRow is one outstanding invite in the fleet inspector.
type InviteRow struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	ExpiresAt time.Time `json:"expires_at"`
}

// FleetDetail is everything the fleet inspector's right pane needs.
type FleetDetail struct {
	FleetRow
	Members  []MemberRow
	Vehicles []VehicleRow
	Invites  []InviteRow
	Counts   map[string]int
	Warnings []string
}

// UserPage is one page of the cross-fleet user directory (FR-ADMIN-FLEET-6).
type UserPage struct {
	Rows     []UserRow
	Total    int
	Warnings []string
}

// UserRow is one user plus the fleets they belong to. The identity half comes
// over HTTP from auth-service; the membership half is this service's own data.
type UserRow struct {
	ID          string
	Email       string
	DisplayName string
	CreatedAt   time.Time
	LastLoginAt *time.Time
	Fleets      []UserFleetRow
}

// UserFleetRow is one of a user's fleet memberships.
type UserFleetRow struct {
	FleetID string `json:"fleet_id"`
	Name    string `json:"name"`
	Role    string `json:"role"`
}

// deletedPredicate is the WHERE fragment for the tri-state filter, aliased to
// the fleets table.
//
// "deleted" means ADMIN-STAMPED (purge_operation_id IS NOT NULL). A fleet
// removed through an ordinary product flow is not recoverable through this
// console, and showing it would imply otherwise.
func deletedPredicate(alias string, f DeletedFilter) string {
	switch f {
	case DeletedExclude:
		return alias + ".deleted_at IS NULL"
	case DeletedOnly:
		return alias + ".deleted_at IS NOT NULL AND " + alias + ".purge_operation_id IS NOT NULL"
	default: // DeletedInclude
		return "(" + alias + ".deleted_at IS NULL OR " + alias + ".purge_operation_id IS NOT NULL)"
	}
}

// ListFleets returns every fleet in the system, regardless of the caller's
// membership (FR-ADMIN-FLEET-1).
//
// fleet.fleets is the one table on gorm.DeletedAt, so this query is raw SQL and
// filters deleted_at by hand — otherwise GORM would silently hide exactly the
// rows the ?deleted= filter exists to show.
func (p *Processor) ListFleets(ctx context.Context, q string, deleted DeletedFilter, page server.Page) (FleetPage, error) {
	where := []string{deletedPredicate("f", deleted)}
	args := []any{}
	if s := strings.TrimSpace(q); s != "" {
		// Name search only at the SQL layer; owner-email search is applied after
		// the auth-service lookup below, because emails do not live in this
		// database and a cross-service join is forbidden (design D2).
		where = append(where, "lower(f.name) LIKE ?")
		args = append(args, "%"+strings.ToLower(s)+"%")
	}
	pred := strings.Join(where, " AND ")

	var total int64
	if err := p.d.DB.Raw("SELECT count(*) FROM fleet.fleets f WHERE "+pred, args...).
		Scan(&total).Error; err != nil {
		return FleetPage{}, fmt.Errorf("count fleets: %w", err)
	}

	// The counts are correlated sub-queries, not a join with GROUP BY: the list
	// must never load any fleet's vehicles (PRD §8 Performance), and two
	// count(*) over indexed fleet_id columns is what "never" looks like.
	//
	// purge_after comes from the operation row so the console can render the
	// countdown chip without a second round trip (FR-ADMIN-UI-7).
	const listSQL = `
		SELECT f.id, f.name, f.created_at, f.deleted_at, f.purge_operation_id,
		       f.created_by_user_id AS owner_user_id,
		       (SELECT count(*) FROM fleet.fleet_memberships m
		          WHERE m.fleet_id = f.id AND m.deleted_at IS NULL) AS member_count,
		       (SELECT count(*) FROM fleet.vehicles v
		          WHERE v.fleet_id = f.id AND v.deleted_at IS NULL) AS vehicle_count,
		       (SELECT o.purge_after FROM fleet.purge_operations o
		          WHERE o.id = f.purge_operation_id) AS purge_after
		FROM fleet.fleets f
		WHERE %s
		ORDER BY f.created_at DESC
		LIMIT ? OFFSET ?`

	type row struct {
		ID               string
		Name             string
		CreatedAt        time.Time
		DeletedAt        *time.Time
		PurgeOperationID *string
		OwnerUserID      string
		MemberCount      int
		VehicleCount     int
		PurgeAfter       *time.Time
	}
	var rows []row
	listArgs := append(append([]any{}, args...), page.Size, page.Offset())
	if err := p.d.DB.Raw(fmt.Sprintf(listSQL, pred), listArgs...).Scan(&rows).Error; err != nil {
		return FleetPage{}, fmt.Errorf("list fleets: %w", err)
	}

	out := FleetPage{Rows: make([]FleetRow, 0, len(rows)), Total: int(total), Warnings: []string{}}
	ownerIDs := make([]string, 0, len(rows))
	for _, r := range rows {
		ownerIDs = append(ownerIDs, r.OwnerUserID)
	}

	// Owner identity comes over HTTP, never from a cross-service join
	// (design D2). A failure degrades the row rather than the request: ids stay,
	// names are empty, and the caller is told (FR-ADMIN-FLEET-5).
	owners, warn := p.resolveUsers(ctx, ownerIDs, "owner names")
	if warn != "" {
		out.Warnings = append(out.Warnings, warn)
	}

	for _, r := range rows {
		o := owners[r.OwnerUserID]
		out.Rows = append(out.Rows, FleetRow{
			ID:               r.ID,
			Name:             r.Name,
			CreatedAt:        r.CreatedAt,
			OwnerUserID:      r.OwnerUserID,
			OwnerEmail:       o.Email,
			OwnerDisplayName: o.DisplayName,
			MemberCount:      r.MemberCount,
			VehicleCount:     r.VehicleCount,
			PendingPurge:     r.DeletedAt != nil && r.PurgeOperationID != nil,
			PurgeAfter:       r.PurgeAfter,
		})
	}
	return out, nil
}

// resolveUsers looks identities up over HTTP, turning a failure into a warning
// string rather than an error. Every caller degrades the same way: ids survive,
// names are empty, and nobody invents a display name (FR-ADMIN-FLEET-5).
func (p *Processor) resolveUsers(ctx context.Context, ids []string, what string) (map[string]adminclient.User, string) {
	if len(ids) == 0 || p.d.AuthUsers == nil {
		return map[string]adminclient.User{}, ""
	}
	resolved, err := p.d.AuthUsers.Users(ctx, ids)
	if err != nil {
		p.log.WithError(err).Warnf("user lookup failed; %s omitted", what)
		return map[string]adminclient.User{}, "auth-service unreachable; " + what + " omitted"
	}
	return resolved, ""
}

// GetFleet returns the fleet inspector's full right pane (FR-ADMIN-FLEET-4).
//
// Counts come from BlastRadius, so the detail figures and the blast-radius panel
// are the same numbers by construction rather than by two queries that agree
// today (FR-ADMIN-UI-9).
func (p *Processor) GetFleet(ctx context.Context, id string) (FleetDetail, error) {
	type row struct {
		ID               string
		Name             string
		CreatedAt        time.Time
		DeletedAt        *time.Time
		PurgeOperationID *string
		OwnerUserID      string
		PurgeAfter       *time.Time
	}
	var head []row
	if err := p.d.DB.Raw(`
		SELECT f.id, f.name, f.created_at, f.deleted_at, f.purge_operation_id,
		       f.created_by_user_id AS owner_user_id,
		       (SELECT o.purge_after FROM fleet.purge_operations o
		          WHERE o.id = f.purge_operation_id) AS purge_after
		FROM fleet.fleets f WHERE f.id = ?`, id).Scan(&head).Error; err != nil {
		return FleetDetail{}, fmt.Errorf("get fleet: %w", err)
	}
	if len(head) == 0 {
		return FleetDetail{}, server.ErrNotFound
	}
	h := head[0]

	out := FleetDetail{Warnings: []string{}}
	out.FleetRow = FleetRow{
		ID:           h.ID,
		Name:         h.Name,
		CreatedAt:    h.CreatedAt,
		OwnerUserID:  h.OwnerUserID,
		PendingPurge: h.DeletedAt != nil && h.PurgeOperationID != nil,
		PurgeAfter:   h.PurgeAfter,
	}

	type memberRow struct {
		UserID    string
		Role      string
		Status    string
		CreatedAt time.Time
	}
	var members []memberRow
	if err := p.d.DB.Raw(`SELECT user_id, role, status, created_at
	                      FROM fleet.fleet_memberships
	                      WHERE fleet_id = ? AND deleted_at IS NULL
	                      ORDER BY created_at ASC`, id).Scan(&members).Error; err != nil {
		return FleetDetail{}, fmt.Errorf("list members: %w", err)
	}

	ids := make([]string, 0, len(members)+1)
	ids = append(ids, h.OwnerUserID)
	for _, m := range members {
		ids = append(ids, m.UserID)
	}
	resolved, warn := p.resolveUsers(ctx, ids, "member names")
	if warn != "" {
		out.Warnings = append(out.Warnings, warn)
	}
	owner := resolved[h.OwnerUserID]
	out.OwnerEmail, out.OwnerDisplayName = owner.Email, owner.DisplayName

	out.Members = make([]MemberRow, 0, len(members))
	for _, m := range members {
		u := resolved[m.UserID]
		out.Members = append(out.Members, MemberRow{
			UserID:      m.UserID,
			Email:       u.Email,
			DisplayName: u.DisplayName,
			Role:        m.Role,
			Status:      m.Status,
			JoinedAt:    m.CreatedAt,
		})
	}
	out.MemberCount = len(out.Members)

	type vehicleRow struct {
		ID               string
		Nickname         string
		Make             string
		Model            string
		Year             int
		CurrentMileage   int
		DeletedAt        *time.Time
		PurgeOperationID *string
	}
	var vehicles []vehicleRow
	// Both live and pending-purge vehicles: the inspector is where an operator
	// checks what a purge took, so hiding the stamped rows would hide the answer.
	if err := p.d.DB.Raw(`SELECT id, nickname, make, model, year, current_mileage,
	                             deleted_at, purge_operation_id
	                      FROM fleet.vehicles
	                      WHERE fleet_id = ? AND (deleted_at IS NULL OR purge_operation_id IS NOT NULL)
	                      ORDER BY created_at ASC`, id).Scan(&vehicles).Error; err != nil {
		return FleetDetail{}, fmt.Errorf("list vehicles: %w", err)
	}
	now := p.d.Now()
	out.Vehicles = make([]VehicleRow, 0, len(vehicles))
	for _, v := range vehicles {
		vr := VehicleRow{
			ID:           v.ID,
			Nickname:     v.Nickname,
			Make:         v.Make,
			Model:        v.Model,
			Year:         v.Year,
			Mileage:      v.CurrentMileage,
			PendingPurge: v.DeletedAt != nil && v.PurgeOperationID != nil,
		}
		// Derived status is affordable for ONE fleet, which is exactly why the
		// list view carries counts only (PRD §8 Performance).
		if p.d.VehicleStatus != nil && !vr.PendingPurge {
			vr.Status = p.d.VehicleStatus.DeriveStatusByID(v.ID, now)
		}
		out.Vehicles = append(out.Vehicles, vr)
		if !vr.PendingPurge {
			out.VehicleCount++
		}
	}

	var invites []InviteRow
	if err := p.d.DB.Raw(`SELECT id, email, role, expires_at
	                      FROM fleet.fleet_invites
	                      WHERE fleet_id = ? AND deleted_at IS NULL AND accepted_at IS NULL
	                      ORDER BY created_at ASC`, id).Scan(&invites).Error; err != nil {
		return FleetDetail{}, fmt.Errorf("list invites: %w", err)
	}
	if invites == nil {
		invites = []InviteRow{}
	}
	out.Invites = invites

	counts, err := p.BlastRadius(Root{Scope: ScopeFleet, TargetID: id})
	if err != nil {
		return FleetDetail{}, fmt.Errorf("blast radius: %w", err)
	}
	out.Counts = counts

	return out, nil
}

// ListUsers returns one page of the cross-fleet user directory
// (FR-ADMIN-FLEET-6).
//
// The identity half comes from auth-service over HTTP; the membership half is
// joined LOCALLY, because a cross-service database join is forbidden (design D2)
// and memberships are this service's own data.
func (p *Processor) ListUsers(ctx context.Context, page server.Page) (UserPage, error) {
	out := UserPage{Rows: []UserRow{}, Warnings: []string{}}
	if p.d.AuthUsers == nil {
		return out, nil
	}
	users, total, err := p.d.AuthUsers.ListUsers(ctx, page)
	if err != nil {
		// Unlike a member-name lookup, this one IS the page: there is nothing
		// left to render, so the failure surfaces rather than degrading.
		return UserPage{}, err
	}
	out.Total = total

	ids := make([]string, 0, len(users))
	for _, u := range users {
		ids = append(ids, u.ID)
	}

	type membershipRow struct {
		UserID  string
		FleetID string
		Name    string
		Role    string
	}
	byUser := map[string][]UserFleetRow{}
	if len(ids) > 0 {
		var rows []membershipRow
		if err := p.d.DB.Raw(`SELECT m.user_id, m.fleet_id, f.name, m.role
		                      FROM fleet.fleet_memberships m
		                      JOIN fleet.fleets f ON f.id = m.fleet_id
		                      WHERE m.user_id IN ? AND m.deleted_at IS NULL AND f.deleted_at IS NULL`,
			ids).Scan(&rows).Error; err != nil {
			return UserPage{}, fmt.Errorf("list memberships: %w", err)
		}
		for _, r := range rows {
			byUser[r.UserID] = append(byUser[r.UserID],
				UserFleetRow{FleetID: r.FleetID, Name: r.Name, Role: r.Role})
		}
	}

	for _, u := range users {
		fleets := byUser[u.ID]
		if fleets == nil {
			fleets = []UserFleetRow{}
		}
		out.Rows = append(out.Rows, UserRow{
			ID:          u.ID,
			Email:       u.Email,
			DisplayName: u.DisplayName,
			CreatedAt:   u.CreatedAt,
			LastLoginAt: u.LastLoginAt,
			Fleets:      fleets,
		})
	}
	return out, nil
}
