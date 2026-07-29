package mediaobject

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	dtoevents "github.com/jtumidanski/myfleet/packages/dto-go/events"
	"github.com/jtumidanski/myfleet/packages/shared-go/events"
	"github.com/jtumidanski/myfleet/packages/shared-go/server"
	"github.com/jtumidanski/myfleet/packages/shared-go/telemetry"

	"github.com/jtumidanski/myfleet/apps/media-service/internal/storage"
)

// EventTypeMediaUploaded is the topic/type published when an upload is
// confirmed; the variant worker pool consumes it (design §7/§8.3).
const EventTypeMediaUploaded = "media.uploaded"

// presignTTL is the lifetime of presigned upload/download URLs (design §8.3).
const presignTTL = 15 * time.Minute

// Presigner is the subset of storage.Client the processor needs. Implemented by
// *storage.Client; kept as an interface so the processor is unit-testable.
type Presigner interface {
	PresignPut(ctx context.Context, key string, ttl time.Duration) (string, error)
	PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error)
	Bucket() string
}

// AuthorizeAccess enforces fleet scoping. media-service trusts the token's
// active_fleet_id claim (design §9): if the object belongs to a different fleet
// we return 404 so cross-fleet existence is never leaked.
func AuthorizeAccess(m Model, identityFleetID string) error {
	if m.FleetID() != identityFleetID {
		return server.ErrNotFound
	}
	return nil
}

// MarkProcessing transitions uploaded → processing. Any other source state is a
// conflict (409).
func MarkProcessing(m Model) (Model, error) {
	if m.Status() != StatusUploaded {
		return Model{}, server.ErrConflict
	}
	return m.WithStatus(StatusProcessing), nil
}

// MarkReady transitions processing → ready. Any other source state is a
// conflict (409) — ready is only valid from processing.
func MarkReady(m Model) (Model, error) {
	if m.Status() != StatusProcessing {
		return Model{}, server.ErrConflict
	}
	return m.WithStatus(StatusReady), nil
}

// Processor contains media-object business logic, injected with Provider,
// Administrator, and a presigner (MinIO). Event publication is handled by the
// transactional-outbox relay (design A8); the processor never calls Publish
// directly.
type Processor struct {
	log     logrus.FieldLogger
	p       Provider
	a       Administrator
	storage Presigner
}

func NewProcessor(log logrus.FieldLogger, p Provider, a Administrator, st Presigner) *Processor {
	return &Processor{log: log, p: p, a: a, storage: st}
}

// InitUpload creates a media-object row in the uploaded state and returns it
// alongside a short-lived presigned PUT URL the client uses to upload the bytes
// directly to MinIO (design §8.3).
func (pr *Processor) InitUpload(fleetID, userID, contentType, filename string) (Model, string, error) {
	id := uuid.NewString()
	key := storage.ObjectKey(fleetID, id, filename)
	m, err := NewBuilder().
		SetID(id).
		SetFleetID(fleetID).
		SetUploadedByUserID(userID).
		SetBucket(pr.storage.Bucket()).
		SetObjectKey(key).
		SetContentType(contentType).
		SetOriginalFilename(filename).
		SetStatus(StatusUploaded).
		Build()
	if err != nil {
		return Model{}, "", err
	}
	created, err := pr.a.Insert(m)
	if err != nil {
		return Model{}, "", err
	}
	url, err := pr.storage.PresignPut(context.Background(), created.ObjectKey(), presignTTL)
	if err != nil {
		return Model{}, "", err
	}
	return created, url, nil
}

// Confirm transitions the object uploaded → processing and enqueues a
// media.uploaded event in the outbox atomically (design A8). The outbox relay
// delivers it to Kafka asynchronously; the variant worker pool consumes it.
// Fleet-scoped.
func (pr *Processor) Confirm(ctx context.Context, id, identityFleetID string) (Model, error) {
	m, err := pr.getActive(id)
	if err != nil {
		return Model{}, err
	}
	if err := AuthorizeAccess(m, identityFleetID); err != nil {
		return Model{}, err
	}
	processing, err := MarkProcessing(m)
	if err != nil {
		return Model{}, err
	}
	// Build the envelope before opening the transaction so any marshalling
	// errors are caught outside the tx (no rollback needed).
	env := events.Envelope{
		EventID:    uuid.NewString(),
		Type:       EventTypeMediaUploaded,
		Version:    1,
		OccurredAt: time.Now().UTC(),
		FleetID:    processing.FleetID(),
		// ActorUserID is the uploader; TraceID propagates the HTTP correlation.
		ActorUserID: processing.UploadedByUserID(),
		TraceID:     telemetry.CorrelationIDFromContext(ctx),
		Data: mustData(dtoevents.MediaUploadedData{
			MediaID:     processing.ID(),
			ContentType: processing.ContentType(),
		}),
	}
	// Status update + outbox enqueue in one atomic transaction (design A8).
	// If either write fails the whole tx rolls back; the object stays in
	// uploaded and Confirm can be retried.
	updated, err := pr.a.UpdateInTx(processing, func(tx *gorm.DB) error {
		return events.Enqueue(tx, env)
	})
	if err != nil {
		return Model{}, err
	}
	return updated, nil
}

// GetByID returns the object metadata, fleet-scoped (cross-fleet → 404).
func (pr *Processor) GetByID(id, identityFleetID string) (Model, error) {
	m, err := pr.getActive(id)
	if err != nil {
		return Model{}, err
	}
	if err := AuthorizeAccess(m, identityFleetID); err != nil {
		return Model{}, err
	}
	return m, nil
}

// DownloadURL authorizes by fleet and returns a short-lived presigned GET URL.
func (pr *Processor) DownloadURL(id, identityFleetID string) (string, error) {
	m, err := pr.GetByID(id, identityFleetID)
	if err != nil {
		return "", err
	}
	return pr.storage.PresignGet(context.Background(), m.ObjectKey(), presignTTL)
}

// SoftDelete marks an object deleted (sets deleted_at + purge_after), scoped to
// the caller's fleet.
func (pr *Processor) SoftDelete(id, identityFleetID string) error {
	m, err := pr.getActive(id)
	if err != nil {
		return err
	}
	if err := AuthorizeAccess(m, identityFleetID); err != nil {
		return err
	}
	if _, err := pr.a.SoftDelete(id); err != nil {
		return err
	}
	return nil
}

// mustData marshals a typed event payload into the Envelope's map[string]any
// Data field via a JSON round-trip. The input is a fixed struct, so marshaling
// cannot fail in practice; on the impossible error path it returns nil.
func mustData(v any) map[string]any {
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil
	}
	return m
}

// getActive fetches a non-deleted object, mapping not-found/gone to HTTP errors.
func (pr *Processor) getActive(id string) (Model, error) {
	m, err := pr.p.GetByIDIncludingDeleted(id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Model{}, server.ErrNotFound
		}
		return Model{}, err
	}
	if m.DeletedAt() != nil && IsPurgeable(m.PurgeAfter()) {
		return Model{}, server.ErrGone
	}
	if m.DeletedAt() != nil {
		return Model{}, server.ErrNotFound
	}
	return m, nil
}
