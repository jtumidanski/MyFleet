package mediaobject

import (
	"context"
	"encoding/json"
	"errors"
	"io"
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

// ObjectStore is the subset of storage.Client the processor needs. Implemented
// by *storage.Client; kept as an interface so the processor is unit-testable.
//
// Bytes are proxied through this service rather than handed to the browser as
// presigned URLs: MinIO is a shared cluster service that also holds other
// applications' buckets, so it is never exposed outside the cluster.
type ObjectStore interface {
	PutObject(ctx context.Context, key string, r io.Reader, size int64, contentType string) error
	GetObject(ctx context.Context, key string) (io.ReadCloser, error)
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
// Administrator, and an ObjectStore (MinIO). Event publication is handled by the
// transactional-outbox relay (design A8); the processor never calls Publish
// directly.
type Processor struct {
	log     logrus.FieldLogger
	p       Provider
	a       Administrator
	storage ObjectStore
}

func NewProcessor(log logrus.FieldLogger, p Provider, a Administrator, st ObjectStore) *Processor {
	return &Processor{log: log, p: p, a: a, storage: st}
}

// InitUpload creates a media-object row in the uploaded state. The client then
// PUTs the bytes to /media/{id}/content; this service proxies them to object
// storage so MinIO is never reachable from the browser.
func (pr *Processor) InitUpload(fleetID, userID, contentType, filename string) (Model, error) {
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
		return Model{}, err
	}
	created, err := pr.a.Insert(m)
	if err != nil {
		return Model{}, err
	}
	return created, nil
}

// countingReader wraps an io.Reader and tallies the bytes actually read, so
// the caller can learn the true length of a stream whose advertised size is
// unknown (or untrustworthy) up front.
type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

// StoreContent streams the request body into object storage for an object still
// in the uploaded state and records the byte count. Fleet-scoped; the content
// type comes from the row created at init, never from the request, so a client
// cannot relabel someone else's bytes. The status transition and the
// media.uploaded event stay in Confirm.
//
// size is passed through to PutObject exactly as given — including -1 for a
// body of unknown/untrusted length, which lets the SDK stream it — but the
// value persisted via WithSize is always the number of bytes this method
// actually read, counted while streaming, never the caller-supplied size.
func (pr *Processor) StoreContent(ctx context.Context, id, identityFleetID string, r io.Reader, size int64) (Model, error) {
	m, err := pr.getActive(id)
	if err != nil {
		return Model{}, err
	}
	if err := AuthorizeAccess(m, identityFleetID); err != nil {
		return Model{}, err
	}
	if m.Status() != StatusUploaded {
		return Model{}, server.ErrConflict
	}
	counted := &countingReader{r: r}
	if err := pr.storage.PutObject(ctx, m.ObjectKey(), counted, size, m.ContentType()); err != nil {
		return Model{}, err
	}
	return pr.a.Update(m.WithSize(counted.n))
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

// Content authorizes by fleet and opens the object's bytes for streaming to the
// client. The caller owns closing the returned ReadCloser. Bytes are proxied
// rather than presigned so MinIO stays unreachable from the browser.
func (pr *Processor) Content(ctx context.Context, id, identityFleetID string) (Model, io.ReadCloser, error) {
	m, err := pr.GetByID(id, identityFleetID)
	if err != nil {
		return Model{}, nil, err
	}
	rc, err := pr.storage.GetObject(ctx, m.ObjectKey())
	if err != nil {
		return Model{}, nil, err
	}
	return m, rc, nil
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
