// Package events holds the shared `data` payload shapes for each event type
// (design §7). Consumers import these instead of a producer's internal/ package.
package events

type VehicleCreatedData struct {
	VehicleID string `json:"vehicle_id"`
	FleetID   string `json:"fleet_id"`
}

type MaintenanceCompletedData struct {
	ScheduleID        string `json:"schedule_id"`
	VehicleID         string `json:"vehicle_id"`
	MaintenanceRecord string `json:"maintenance_record_id"`
	CategoryID        string `json:"category_id"`
}

type FuelLoggedData struct {
	FuelLogID string  `json:"fuel_log_id"`
	VehicleID string  `json:"vehicle_id"`
	Mileage   int     `json:"mileage"`
	TotalCost float64 `json:"total_cost"`
}

type ScheduleOverdueData struct {
	ScheduleID string `json:"schedule_id"`
	VehicleID  string `json:"vehicle_id"`
	Severity   string `json:"severity"`
	// DueCycle identifies the due window the schedule entered (next-due date +
	// mileage). It is the dedupe token: the notification consumer and the daily
	// reminder safety-net build the SAME per-user dedupe_key from it, so the two
	// paths cannot double-fire for one overdue cycle (design A6).
	DueCycle string `json:"due_cycle"`
}

type MemberInvitedData struct {
	InviteID string `json:"invite_id"`
	Email    string `json:"email"`
	Role     string `json:"role"`
}

// InviteCreatedData is the payload of invite.created, emitted when an invite row
// is created or its token is rotated by a resend. Deliberately NOT an alias of
// MemberInvitedData: the two events mean opposite things (created vs accepted),
// and aliasing would let a field added for one change the other's wire format.
//
// It carries NO token. The token is a bearer credential; the email consumer
// fetches it over internal HTTP instead (design §2).
type InviteCreatedData struct {
	InviteID string `json:"invite_id"`
	Email    string `json:"email"`
	Role     string `json:"role"`
}

type MediaUploadedData struct {
	MediaID     string `json:"media_id"`
	ContentType string `json:"content_type"`
}
