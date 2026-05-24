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
}

type MemberInvitedData struct {
	InviteID string `json:"invite_id"`
	Email    string `json:"email"`
	Role     string `json:"role"`
}

type MediaUploadedData struct {
	MediaID     string `json:"media_id"`
	ContentType string `json:"content_type"`
}
