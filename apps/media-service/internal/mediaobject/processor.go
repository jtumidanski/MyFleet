package mediaobject

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
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
//
// GetObject must have determined that the object is actually readable before
// it returns — callers commit an HTTP status line on the strength of its nil
// error — and must report a missing key as storage.ErrObjectNotFound.
type ObjectStore interface {
	PutObject(ctx context.Context, key string, r io.Reader, size int64, contentType string) error
	GetObject(ctx context.Context, key string) (io.ReadCloser, error)
	Bucket() string
}

// VariantRef is what the processor needs in order to stream a derived image:
// where the bytes live and what they are. Nothing else about a variant is
// relevant here.
type VariantRef struct {
	ObjectKey   string
	ContentType string
}

// VariantLookup resolves a derived image for a media object. It is declared
// here, in the package that consumes it, and implemented in the composition
// root — the same shape as ObjectStore above — so mediaobject never imports the
// sibling mediavariant package and the dependency graph stays a tree.
//
// variant crosses the port as a plain string so the implementer does not need
// mediaobject's types either. ctx is the request context, threaded so the
// implementation can cancel its query when the client disconnects — the same
// shape as ObjectStore above, whose every method takes one.
//
// A miss is a normal outcome (found=false), not an error: variants do not exist
// until the processing worker has run, and never exist for non-image media.
// Content still refuses to serve anything for such a request (see Content), but
// it must be able to tell "not generated" apart from "the database is broken".
type VariantLookup interface {
	Lookup(ctx context.Context, mediaObjectID, variant string) (VariantRef, bool, error)
}

// CardSource is what the generator needs in order to derive a card variant. It
// crosses the port as plain data so the implementer needs none of mediaobject's
// types — the same shape as VariantRef above.
//
// A named struct rather than four positional strings: four same-typed arguments
// in a row is a swap waiting to happen, and a transposed fleetID/objectKey would
// write a variant under the wrong key.
type CardSource struct {
	MediaObjectID string
	FleetID       string
	ObjectKey     string
	ContentType   string
}

// CardGenerator schedules background generation of a missing card variant.
//
// Generate MUST return without blocking: it is called while serving an HTTP
// response, and the whole point of the lazy path is that the request does not
// wait for a decode. It has no error return because the caller has nothing to do
// with one — the response has already been decided.
//
// Declared here and implemented in the composition root, the same shape as
// VariantLookup, so mediaobject never imports the processing package.
type CardGenerator interface {
	Generate(src CardSource)
}

// nopCardGenerator is the default when no generator is wired
// (MEDIA_LAZY_VARIANT_CONCURRENCY=0). Expressing "lazy generation is off" as a
// no-op implementation rather than a nil check is what keeps Content free of a
// branch that exists only for configuration.
type nopCardGenerator struct{}

func (nopCardGenerator) Generate(CardSource) {}

// ContentInfo describes the bytes actually being served, which are not always
// the media object's own metadata: a variant is re-encoded and carries its own
// content type, and its length is not recorded anywhere. Returning this instead
// of the Model is what lets the handler set headers from the bytes it is about
// to write.
type ContentInfo struct {
	// ContentType is always re-resolved through the allowlist, never the raw
	// stored value, so an unrecognised type degrades to octet-stream rather
	// than being echoed back to the browser (design D15, PRD FR-DL-4).
	ContentType string
	// Size is 0 when unknown; the handler then omits Content-Length. The
	// original's size must never be sent for a variant response.
	Size int64
	// Disposition is the complete Content-Disposition header value. It is
	// built here rather than in the handler because it needs the Model — the
	// original filename and the ID fallback — which the handler no longer
	// sees. Building it alongside ContentType is also what stops the two
	// drifting: the class that picks inline-vs-attachment is the same class
	// the type was resolved to.
	Disposition string
	// Downgraded is true when the bytes being served are a smaller rendition
	// than the caller asked for: today, a thumbnail standing in for a card
	// that has not been generated yet. It is a statement of fact about the
	// bytes, deliberately carrying no HTTP vocabulary — the handler decides
	// what the fact means on the wire. The false zero value is correct for
	// every path that serves what was asked for, which is why no existing
	// construction site needs to change.
	Downgraded bool
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

// MarkReadyDirect transitions uploaded → ready for objects that need no
// processing (documents). Any other source state is a conflict (409). MarkReady
// is deliberately left untouched so the worker's behaviour and tests are
// unchanged (design D12).
func MarkReadyDirect(m Model) (Model, error) {
	if m.Status() != StatusUploaded {
		return Model{}, server.ErrConflict
	}
	return m.WithStatus(StatusReady), nil
}

// MarkFailed is the terminal failure transition. It accepts uploaded or
// processing; anything else is a conflict. It is what guarantees no object
// stays in processing forever and no Kafka partition is blocked by one bad
// file (design D13, PRD FR-MEDIA-5).
func MarkFailed(m Model) (Model, error) {
	if m.Status() != StatusUploaded && m.Status() != StatusProcessing {
		return Model{}, server.ErrConflict
	}
	return m.WithStatus(StatusFailed), nil
}

// Processor contains media-object business logic, injected with Provider,
// Administrator, and an ObjectStore (MinIO). Event publication is handled by the
// transactional-outbox relay (design A8); the processor never calls Publish
// directly.
type Processor struct {
	log      logrus.FieldLogger
	p        Provider
	a        Administrator
	storage  ObjectStore
	variants VariantLookup
	allow    Allowlist
	cards    CardGenerator
}

// ProcessorOption configures an optional Processor dependency.
type ProcessorOption func(*Processor)

// WithCardGenerator enables lazy generation of missing card variants. It is an
// option rather than a parameter because the dependency is genuinely optional —
// MEDIA_LAZY_VARIANT_CONCURRENCY=0 wires no generator, and that is a supported
// deployment, not a degraded one.
func WithCardGenerator(g CardGenerator) ProcessorOption {
	return func(pr *Processor) { pr.cards = g }
}

func NewProcessor(log logrus.FieldLogger, p Provider, a Administrator, st ObjectStore, variants VariantLookup, allow Allowlist, opts ...ProcessorOption) *Processor {
	pr := &Processor{
		log: log, p: p, a: a, storage: st, variants: variants, allow: allow,
		cards: nopCardGenerator{},
	}
	for _, opt := range opts {
		opt(pr)
	}
	return pr
}

// InitUpload creates a media-object row in the uploaded state. The client then
// PUTs the bytes to /media/{id}/content; this service proxies them to object
// storage so MinIO is never reachable from the browser.
//
// The client-supplied content type is validated against the server-side
// allowlist and stored NORMALISED (parameters discarded, lowercased), so no
// arbitrary client string is ever persisted and GET /media/{id}/content cannot
// echo one back (PRD FR-MEDIA-1, design D10).
func (pr *Processor) InitUpload(fleetID, userID, contentType, filename string) (Model, error) {
	normalized, ok := pr.allow.Normalize(contentType)
	if !ok {
		// Log the offending type, never the bytes and never the filename.
		pr.log.WithFields(logrus.Fields{
			"content_type": contentType,
			"fleet_id":     fleetID,
			"user_id":      userID,
		}).Warn("upload rejected: content type is not on the allowlist")
		return Model{}, fmt.Errorf("%w: accepted types are %s",
			server.ErrUnsupportedMediaType, strings.Join(pr.allow.Accepted(), ", "))
	}

	id := uuid.NewString()
	key := storage.ObjectKey(fleetID, id, filename)
	m, err := NewBuilder().
		SetID(id).
		SetFleetID(fleetID).
		SetUploadedByUserID(userID).
		SetBucket(pr.storage.Bucket()).
		SetObjectKey(key).
		SetContentType(normalized).
		SetOriginalFilename(filename).
		SetStatus(StatusUploaded).
		Build()
	if err != nil {
		return Model{}, err
	}
	return pr.a.Insert(m)
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
	// Classification decides everything downstream (design §2). ClassUnknown is
	// deliberately folded in with documents: a pre-allowlist row whose content
	// type nobody recognises must never be handed to image.Decode. Legacy
	// JPEG/PNG rows still classify as ClassImage — their stored type is on the
	// allowlist — so nothing regresses.
	if pr.allow.Classify(m.ContentType()) != ClassImage {
		ready, err := MarkReadyDirect(m)
		if err != nil {
			return Model{}, err
		}
		return pr.a.Update(ready)
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

// Content authorizes by fleet and opens the requested rendition's bytes for
// streaming to the client. The caller owns closing the returned ReadCloser.
// Bytes are proxied rather than presigned so MinIO stays unreachable from the
// browser.
//
// The media object is resolved and fleet-scoped FIRST, before any variant
// lookup, object-store read, or scheduling decision, so a variant is never
// reachable by a caller who could not read the original (FR-7.5) and a caller
// who cannot read a media object can cause no work to be scheduled for it. A
// cross-fleet caller exits here with 404 — never 403, which would restore the
// existence oracle AuthorizeAccess exists to prevent.
//
// A derived variant that cannot be served is a 404, with exactly one exception:
// a missing card is served as a thumbnail. It does NOT fall back to the
// original. Falling back to the original looks harmless per request and is
// ruinous per page: a twelve-card grid asking for thumbnails would quietly pull
// twelve full-size originals, up to 25 MiB each, which is precisely the cost the
// derived variants exist to avoid. The card exception carries none of that: a
// 320px thumbnail standing in for a 768px card is SMALLER than what was asked
// for, never larger, and it exists because card variants are filled in lazily
// for media uploaded before the variant existed. It is scoped to
// card → thumbnail and generalises no further — a missing display still 404s,
// so a detail view can never silently receive a smaller rendition than it asked
// for.
//
// ?variant=original and a request with no parameter are untouched by any of
// this: they serve the original with its Content-Length exactly as they always
// have. That is the backwards-compatibility contract every pre-existing caller
// depends on.
func (pr *Processor) Content(ctx context.Context, id, identityFleetID string, want ContentVariant) (ContentInfo, io.ReadCloser, error) {
	m, err := pr.GetByID(id, identityFleetID)
	if err != nil {
		return ContentInfo{}, nil, err
	}
	if want == ContentOriginal {
		return pr.openOriginal(ctx, m)
	}

	info, rc, err := pr.openVariant(ctx, m, want)
	if err == nil {
		return info, rc, nil
	}
	// Everything that is not a missing card leaves here: display and thumbnail
	// 404s, and every 500. Only server.ErrNotFound downgrades, so a database or
	// store fault still surfaces as a fault rather than being hidden behind a
	// thumbnail.
	if want != ContentCard || !errors.Is(err, server.ErrNotFound) {
		return ContentInfo{}, nil, err
	}

	pr.scheduleCard(m)
	// Expected and common during the lazy-fill period, so Debug: logging it
	// loudly would be noise.
	pr.log.WithField("media_id", m.ID()).
		Debug("serving the thumbnail in place of an unavailable card variant")
	// 200 with the thumbnail's own bytes, type and disposition — or its own 404
	// if there is no thumbnail either. No third attempt.
	info, rc, err = pr.openVariant(ctx, m, ContentThumbnail)
	if err != nil {
		// No thumbnail either: a 404, and no response to mark. Setting the
		// flag before this check would stamp it on a zero struct being
		// discarded — harmless today, and a trap the first time someone
		// inspects info on an error path.
		return ContentInfo{}, nil, err
	}
	// Only this call site knows a substitution happened. openVariant opened
	// exactly the rendition it was asked for, so from where it stands nothing
	// was substituted; pushing the flag down there would make an explicit
	// ?variant=thumbnail request and a downgraded one indistinguishable again,
	// which is the bug this change exists to fix.
	info.Downgraded = true
	return info, rc, nil
}

// openVariant opens one stored rendition. It returns server.ErrNotFound for both
// ways a variant can be unservable — no row at all, and a row whose object is
// missing from the store — because the caller's response is the same in both
// cases; only the log level differs, since the second is drift someone should
// see.
func (pr *Processor) openVariant(ctx context.Context, m Model, want ContentVariant) (ContentInfo, io.ReadCloser, error) {
	ref, found, err := pr.variants.Lookup(ctx, m.ID(), string(want))
	if err != nil {
		// A miss is found=false, so an error here means the database is broken
		// — and GetByID just read the same database successfully. A 500 is the
		// honest answer; a 404 would hide a real fault.
		return ContentInfo{}, nil, err
	}
	if !found {
		// Expected whenever processing has not run yet, or the media is not a
		// processable image; debug, not warn.
		pr.log.WithField("media_id", m.ID()).WithField("variant", string(want)).
			Debug("no stored variant for the requested rendition")
		return ContentInfo{}, nil, server.ErrNotFound
	}
	rc, err := pr.storage.GetObject(ctx, ref.ObjectKey)
	switch {
	case err == nil:
		ct := ref.ContentType
		if ct == "" {
			// Should never happen — variants are re-encoded and always record a
			// type — but an empty header is worse than a slightly wrong one.
			ct = m.ContentType()
		}
		// A variant is re-encoded by the worker rather than supplied by a
		// client, but it is still resolved through the allowlist so every
		// response — original or variant — is described by the same trusted
		// vocabulary, and a type nobody recognises degrades to octet-stream +
		// attachment instead of being served as-is.
		resolved, class := pr.allow.Resolve(ct)
		// Size stays 0: media_variants records width/height/content_type but no
		// byte count, so Content-Length is omitted (FR-7.8).
		return ContentInfo{
			ContentType: resolved,
			Disposition: ContentDisposition(class, m.OriginalFilename(), m.ID()),
		}, rc, nil
	case errors.Is(err, storage.ErrObjectNotFound):
		// DB/store drift, unlike the miss above — someone should see it, so this
		// stays a Warn even though the response is the same 404.
		pr.log.WithField("media_id", m.ID()).WithField("variant", string(want)).
			WithField("object_key", ref.ObjectKey).
			Warn("variant row has no object in storage")
		return ContentInfo{}, nil, server.ErrNotFound
	default:
		return ContentInfo{}, nil, err
	}
}

// scheduleCard asks the generator to fill in a missing card variant, if this
// object is one a card can be derived from.
//
// Eligibility lives here rather than in the generator because the processor
// holds the Model and the Allowlist and would otherwise be handing both across
// the port. The generator owns single-flight, the concurrency cap, the failure
// ledger and the work itself, because those are all its own state. Neither needs
// to know the other's rules.
//
// It is called only after GetByID has authorized the caller, and only for
// ContentCard — a missing thumbnail or display schedules nothing.
func (pr *Processor) scheduleCard(m Model) {
	if m.Status() != StatusReady {
		pr.log.WithField("media_id", m.ID()).WithField("status", string(m.Status())).
			Debug("card generation not scheduled: media object is not ready")
		return
	}
	// ClassUnknown fails this check too, so a pre-allowlist row with an
	// unrecognised type is never handed to image.Decode — the same guard Confirm
	// applies before publishing media.uploaded.
	if pr.allow.Classify(m.ContentType()) != ClassImage {
		pr.log.WithField("media_id", m.ID()).
			Debug("card generation not scheduled: media object is not a renderable image")
		return
	}
	pr.cards.Generate(CardSource{
		MediaObjectID: m.ID(),
		FleetID:       m.FleetID(),
		ObjectKey:     m.ObjectKey(),
		ContentType:   m.ContentType(),
	})
}

// openOriginal streams the uploaded bytes. Kept separate from Content so the
// not-found mapping for the original has exactly one implementation.
func (pr *Processor) openOriginal(ctx context.Context, m Model) (ContentInfo, io.ReadCloser, error) {
	rc, err := pr.storage.GetObject(ctx, m.ObjectKey())
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotFound) {
			// The row exists but its bytes do not: InitUpload creates the row
			// before any content is PUT, and a PUT that fails leaves exactly
			// that state. 404 rather than 500 because nothing is broken
			// server-side — this sub-resource simply does not exist yet — and
			// it matches what the client used to see when it followed a
			// presigned URL straight to MinIO.
			return ContentInfo{}, nil, server.ErrNotFound
		}
		return ContentInfo{}, nil, err
	}
	// The Content-Type is re-resolved through the allowlist on every read
	// rather than trusting the stored value, so shrinking the allowlist
	// retroactively downgrades already-stored objects and rows created before
	// the allowlist existed are covered too (design D15, PRD FR-DL-4).
	ct, class := pr.allow.Resolve(m.ContentType())
	return ContentInfo{
		ContentType: ct,
		Size:        m.Size(),
		Disposition: ContentDisposition(class, m.OriginalFilename(), m.ID()),
	}, rc, nil
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
