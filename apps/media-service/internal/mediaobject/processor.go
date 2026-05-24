package mediaobject

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

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
// Administrator, a presigner (MinIO), and an events.Producer (real Kafka per
// design A7).
type Processor struct {
	log      logrus.FieldLogger
	p        Provider
	a        Administrator
	storage  Presigner
	producer events.Producer
}

func NewProcessor(log logrus.FieldLogger, p Provider, a Administrator, st Presigner, producer events.Producer) *Processor {
	return &Processor{log: log, p: p, a: a, storage: st, producer: producer}
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

// Confirm transitions the object uploaded → processing and publishes a
// media.uploaded event so the worker pool generates variants. Fleet-scoped.
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
	updated, err := pr.a.Update(processing)
	if err != nil {
		return Model{}, err
	}
	env := events.Envelope{
		EventID:     uuid.NewString(),
		Type:        EventTypeMediaUploaded,
		Version:     1,
		OccurredAt:  time.Now().UTC(),
		FleetID:     updated.FleetID(),
		ActorUserID: updated.UploadedByUserID(),
		TraceID:     telemetry.CorrelationIDFromContext(ctx),
		Data: mustData(dtoevents.MediaUploadedData{
			MediaID:     updated.ID(),
			ContentType: updated.ContentType(),
		}),
	}
	if err := pr.producer.Publish(ctx, env); err != nil {
		pr.log.WithError(err).WithField("media_id", updated.ID()).Error("publish media.uploaded failed")
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
