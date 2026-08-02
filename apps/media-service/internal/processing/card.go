package processing

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/jtumidanski/myfleet/apps/media-service/internal/mediavariant"
	"github.com/jtumidanski/myfleet/apps/media-service/internal/storage"
	"github.com/jtumidanski/myfleet/apps/media-service/internal/variantfailures"
)

// generateTimeout bounds one lazy generation. It is ample for decoding, scaling
// and uploading an original at the MEDIA_MAX_UPLOAD_BYTES ceiling, and short
// enough that a wedged object-store call cannot hold a concurrency slot
// indefinitely.
//
// media-service has no graceful-shutdown path — server.Run is
// http.ListenAndServe and SIGTERM kills the process — so in-flight generations
// die with the pod. That is harmless: the Upsert is the only write and it is the
// last step, so nothing is ever half-written, and the next request reschedules.
const generateTimeout = 60 * time.Second

// CardGenerator produces the card variant for media objects that predate it, on
// demand and in the background.
//
// Card only, by construction: a missing thumbnail or display still 404s and
// schedules nothing. The bytes it produces are identical to the upload worker's
// because it calls the same decodeOriginal and buildVariant.
type CardGenerator struct {
	log      logrus.FieldLogger
	store    ObjectStore
	variants mediavariant.Administrator
	failures *variantfailures.Store

	// base is the process lifetime, captured at construction — never a request
	// context. A client that disconnects mid-download must not cancel the work
	// its request triggered (FR-4.4).
	base context.Context
	// sem is the global concurrency cap: a cold twelve-card grid must not spawn
	// twelve simultaneous full-size image decodes. It is acquired with a
	// non-blocking send, so a saturated cap DROPS the request rather than
	// queueing it — including when capacity is 0, where an unbuffered channel
	// with no receiver rejects every send and lazy generation is off entirely.
	sem chan struct{}

	mu sync.Mutex
	// inFlight is keyed by media object ID with no variant component, because
	// card is the only variant that ever enters this path.
	inFlight map[string]struct{}
}

// NewCardGenerator builds a generator bounded by concurrency simultaneous
// generations. base must outlive any request — pass the process context.
//
// concurrency 0 disables lazy generation entirely, and negatives clamp to 0.
// This deviates from the MEDIA_WORKERS precedent, which clamps up to 1, and it
// is deliberate: this feature schedules work in response to request traffic, so
// an operator who sees it misbehave needs an off switch that is not a rollback.
// Disabled is a coherent state — the content endpoint keeps serving the
// thumbnail downgrade, which is exactly the pre-task behaviour.
func NewCardGenerator(
	base context.Context,
	log logrus.FieldLogger,
	store ObjectStore,
	variants mediavariant.Administrator,
	failures *variantfailures.Store,
	concurrency int,
) *CardGenerator {
	if concurrency < 0 {
		concurrency = 0
	}
	return &CardGenerator{
		log:      log,
		store:    store,
		variants: variants,
		failures: failures,
		base:     base,
		sem:      make(chan struct{}, concurrency),
		inFlight: make(map[string]struct{}),
	}
}

// Generate schedules card generation for src and returns immediately, before any
// I/O. It is called while serving an HTTP response, so it must never block: the
// whole point of the lazy path is that the request does not wait for a decode.
//
// Both admission checks are taken synchronously here and released by paired
// defers in the one goroutine that owns them, so no slot can leak. Taking the
// semaphore BEFORE spawning is what makes the cap an actual bound on live
// goroutines rather than something they merely converge on.
func (g *CardGenerator) Generate(src Source) {
	l := g.log.WithField("media_id", src.MediaObjectID)

	if !g.reserve(src.MediaObjectID) {
		l.Debug("card generation already in flight for this media object; skipping")
		return
	}
	select {
	case g.sem <- struct{}{}:
	default:
		g.release(src.MediaObjectID)
		l.Debug("lazy variant concurrency cap saturated; dropping card generation")
		return
	}

	go func() {
		// Registered first so it runs LAST (defers are LIFO): by the time a
		// recovered panic is swallowed, the semaphore token and the in-flight
		// key have already been released by the defers below. image.Decode runs
		// on arbitrary stored bytes, and on the upload path a decoder panic
		// crashes the pod once; on this lazy path the trigger is read traffic —
		// every render of a grid containing the bad object re-invokes Generate —
		// so an unrecovered panic here would turn a one-off decoder bug into a
		// crash loop instead. A panic is not a classified permanent failure, so
		// it is logged and dropped, never recorded in the failure ledger.
		defer func() {
			if r := recover(); r != nil {
				l.WithField("panic", r).Warn("card generation panicked; recovered without recording a failure")
			}
		}()
		defer func() { <-g.sem }()
		defer g.release(src.MediaObjectID)

		ctx, cancel := context.WithTimeout(g.base, generateTimeout)
		defer cancel()

		// The ledger check sits here rather than in Generate deliberately.
		// Doing it synchronously would put a database round trip on the COMMON
		// path — every downgraded response for the whole lazy-fill period — to
		// guard a case that should never occur. The observable contract holds
		// exactly: a second request performs no decode. What it costs is one
		// goroutine and one indexed read on an almost-always-empty table.
		recorded, err := g.failures.Recorded(src.MediaObjectID, string(mediavariant.VariantCard))
		if err != nil {
			l.WithError(err).Warn("reading the variant-failure ledger failed; skipping card generation")
			return
		}
		if recorded {
			l.Debug("card generation recorded as permanently impossible; skipping")
			return
		}

		img, err := decodeOriginal(ctx, g.store, src.ObjectKey)
		if err != nil {
			if errors.Is(err, ErrPermanent) {
				reason := variantfailures.ReasonUndecodable
				if errors.Is(err, storage.ErrObjectNotFound) {
					reason = variantfailures.ReasonOriginalMissing
				}
				if rerr := g.failures.Record(src.MediaObjectID, string(mediavariant.VariantCard), reason); rerr != nil {
					l.WithError(rerr).Warn("recording a permanent card-generation failure failed")
				}
				l.WithError(err).WithField("reason", reason).
					Warn("card generation failed permanently; it will not be retried")
				return
			}
			// Transient — the store was briefly unreachable, say. Recording it
			// would permanently condemn a perfectly good image.
			l.WithError(err).Warn("card generation failed transiently; a later request may retry")
			return
		}

		v, err := buildVariant(ctx, g.store, src, img, mediavariant.VariantCard, cardMaxEdge)
		if err != nil {
			l.WithError(err).Warn("building the card variant failed; a later request may retry")
			return
		}
		// Upsert, never ReplaceForMediaObject: the latter deletes every row for
		// the media object first and would destroy its thumbnail and display.
		if err := g.variants.Upsert(v); err != nil {
			l.WithError(err).Warn("persisting the card variant failed; a later request may retry")
			return
		}
		l.WithField("object_key", v.ObjectKey()).Info("card variant generated")
	}()
}

// reserve takes the single-flight slot for a media object, reporting false when
// another generation for the same object already holds it (FR-4.1).
func (g *CardGenerator) reserve(mediaObjectID string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, taken := g.inFlight[mediaObjectID]; taken {
		return false
	}
	g.inFlight[mediaObjectID] = struct{}{}
	return true
}

func (g *CardGenerator) release(mediaObjectID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.inFlight, mediaObjectID)
}

// inFlightFor reports whether a generation for this media object is running. It
// exists so tests can wait for an asynchronous generation to finish instead of
// sleeping a guessed interval.
func (g *CardGenerator) inFlightFor(mediaObjectID string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	_, taken := g.inFlight[mediaObjectID]
	return taken
}
