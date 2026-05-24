package mediaobject

import "time"

// Status is the media object lifecycle state. Transitions are guarded in the
// processor: uploaded → processing → ready (design §8.3).
type Status string

const (
	StatusUploaded   Status = "uploaded"
	StatusProcessing Status = "processing"
	StatusReady      Status = "ready"
)

// Model is immutable; state changes return new instances.
type Model struct {
	id               string
	fleetID          string
	uploadedByUserID string
	bucket           string
	objectKey        string
	contentType      string
	size             int64
	originalFilename string
	status           Status
	createdAt        time.Time
	deletedAt        *time.Time
	purgeAfter       *time.Time
}

func (m Model) ID() string               { return m.id }
func (m Model) FleetID() string          { return m.fleetID }
func (m Model) UploadedByUserID() string { return m.uploadedByUserID }
func (m Model) Bucket() string           { return m.bucket }
func (m Model) ObjectKey() string        { return m.objectKey }
func (m Model) ContentType() string      { return m.contentType }
func (m Model) Size() int64              { return m.size }
func (m Model) OriginalFilename() string { return m.originalFilename }
func (m Model) Status() Status           { return m.status }
func (m Model) CreatedAt() time.Time     { return m.createdAt }
func (m Model) DeletedAt() *time.Time    { return m.deletedAt }
func (m Model) PurgeAfter() *time.Time   { return m.purgeAfter }

// WithStatus returns a copy with the status changed.
func (m Model) WithStatus(s Status) Model {
	m.status = s
	return m
}

// WithSize returns a copy with the byte size changed (set on confirm).
func (m Model) WithSize(size int64) Model {
	m.size = size
	return m
}

// WithSoftDelete returns a copy stamped with deletedAt and purgeAfter.
func (m Model) WithSoftDelete(deletedAt time.Time) Model {
	purge := ComputePurgeAfter(deletedAt)
	m.deletedAt = &deletedAt
	m.purgeAfter = &purge
	return m
}
