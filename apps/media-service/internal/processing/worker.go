// Package processing implements the in-process Kafka worker pool that turns an
// uploaded original image into derived variants (thumbnail, display) and marks
// the media object ready (design A7 / §7 / §8.3). The pool is self-contained
// within media-service: it uses a real Kafka consumer group and does not depend
// on any other service's outbox.
package processing

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"

	"github.com/sirupsen/logrus"
	xdraw "golang.org/x/image/draw"

	"github.com/jtumidanski/myfleet/packages/shared-go/events"

	"github.com/jtumidanski/myfleet/apps/media-service/internal/mediaobject"
	"github.com/jtumidanski/myfleet/apps/media-service/internal/mediavariant"
	"github.com/jtumidanski/myfleet/apps/media-service/internal/processedevents"
	"github.com/jtumidanski/myfleet/apps/media-service/internal/storage"
)

const (
	thumbnailMaxEdge = 320
	displayMaxEdge   = 1280
)

// ResizeDims computes target dimensions so the longest edge equals maxEdge while
// preserving aspect ratio. It never upscales: if both dimensions are already
// <= maxEdge the original dimensions are returned unchanged.
func ResizeDims(w, h, maxEdge int) (int, int) {
	if w <= 0 || h <= 0 {
		return w, h
	}
	longest := w
	if h > longest {
		longest = h
	}
	if longest <= maxEdge {
		return w, h // never upscale
	}
	if w >= h {
		nh := h * maxEdge / w
		if nh < 1 {
			nh = 1
		}
		return maxEdge, nh
	}
	nw := w * maxEdge / h
	if nw < 1 {
		nw = 1
	}
	return nw, maxEdge
}

// ObjectStore is the subset of storage.Client the worker needs.
type ObjectStore interface {
	GetObject(ctx context.Context, key string) (io.ReadCloser, error)
	PutObject(ctx context.Context, key string, r io.Reader, size int64, contentType string) error
}

// Worker generates image variants in response to media.uploaded events.
type Worker struct {
	log         logrus.FieldLogger
	store       ObjectStore
	objects     mediaobject.Provider
	objectAdmin mediaobject.Administrator
	variants    mediavariant.Administrator
	dedupe      *processedevents.Store
}

// NewWorker constructs a Worker with all of its collaborators injected.
func NewWorker(
	log logrus.FieldLogger,
	store ObjectStore,
	objects mediaobject.Provider,
	objectAdmin mediaobject.Administrator,
	variants mediavariant.Administrator,
	dedupe *processedevents.Store,
) *Worker {
	return &Worker{
		log:         log,
		store:       store,
		objects:     objects,
		objectAdmin: objectAdmin,
		variants:    variants,
		dedupe:      dedupe,
	}
}

// Run blocks consuming media.uploaded from the given brokers/group until ctx is
// cancelled. Each worker goroutine shares the same consumer group so partitions
// are balanced across the pool.
func (w *Worker) Run(ctx context.Context, brokers []string, group string) {
	events.Consume(ctx, w.log, brokers, group, mediaobject.EventTypeMediaUploaded, w.handle)
}

// handle processes one media.uploaded event idempotently. The dedupe gate is
// checked first: a re-delivered event whose ID is already recorded is skipped.
func (w *Worker) handle(ctx context.Context, e events.Envelope) error {
	already, err := w.dedupe.MarkProcessed(e.EventID)
	if err != nil {
		return fmt.Errorf("dedupe check: %w", err)
	}
	if already {
		w.log.WithField("event_id", e.EventID).Debug("media.uploaded already processed; skipping")
		return nil
	}

	mediaID, _ := e.Data["media_id"].(string)
	if mediaID == "" {
		w.log.WithField("event_id", e.EventID).Error("media.uploaded missing media_id; skipping")
		return nil // bad payload — committing avoids a poison-pill retry loop
	}

	obj, err := w.objects.GetByID(mediaID)
	if err != nil {
		return fmt.Errorf("load media object %s: %w", mediaID, err)
	}

	src, err := w.decodeOriginal(ctx, obj.ObjectKey())
	if err != nil {
		return fmt.Errorf("decode original %s: %w", obj.ObjectKey(), err)
	}

	built := make([]mediavariant.Model, 0, 2)
	for _, spec := range []struct {
		kind    mediavariant.Variant
		maxEdge int
	}{
		{mediavariant.VariantThumbnail, thumbnailMaxEdge},
		{mediavariant.VariantDisplay, displayMaxEdge},
	} {
		v, err := w.generateVariant(ctx, obj, src, spec.kind, spec.maxEdge)
		if err != nil {
			return fmt.Errorf("generate %s variant: %w", spec.kind, err)
		}
		built = append(built, v)
	}

	// Idempotent: replace any prior variants for this media object.
	if err := w.variants.ReplaceForMediaObject(mediaID, built); err != nil {
		return fmt.Errorf("persist variants: %w", err)
	}

	// processing → ready (transition guard lives in mediaobject.MarkReady).
	ready, err := mediaobject.MarkReady(obj)
	if err != nil {
		// Already ready (or unexpected state) — the variants are persisted, so
		// treat as success to avoid an endless redelivery loop.
		w.log.WithField("media_id", mediaID).WithError(err).Warn("media object not in processing state; leaving status unchanged")
		return nil
	}
	if _, err := w.objectAdmin.Update(ready); err != nil {
		return fmt.Errorf("mark media object ready: %w", err)
	}
	w.log.WithField("media_id", mediaID).Info("media variants generated; object ready")
	return nil
}

// decodeOriginal downloads and decodes the original image (jpeg or png).
func (w *Worker) decodeOriginal(ctx context.Context, key string) (image.Image, error) {
	rc, err := w.store.GetObject(ctx, key)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	img, _, err := image.Decode(rc)
	if err != nil {
		return nil, err
	}
	return img, nil
}

// generateVariant scales src to the variant's max edge (never upscaling),
// encodes it, uploads it to MinIO under a variant-suffixed key, and returns the
// variant model.
func (w *Worker) generateVariant(ctx context.Context, obj mediaobject.Model, src image.Image, kind mediavariant.Variant, maxEdge int) (mediavariant.Model, error) {
	b := src.Bounds()
	tw, th := ResizeDims(b.Dx(), b.Dy(), maxEdge)

	dst := image.NewRGBA(image.Rect(0, 0, tw, th))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, b, xdraw.Over, nil)

	contentType, ext := variantEncoding(obj.ContentType())
	var buf bytes.Buffer
	if err := encode(&buf, dst, contentType); err != nil {
		return mediavariant.Model{}, err
	}

	key := storage.ObjectKey(obj.FleetID(), obj.ID(), string(kind)+ext)
	if err := w.store.PutObject(ctx, key, bytes.NewReader(buf.Bytes()), int64(buf.Len()), contentType); err != nil {
		return mediavariant.Model{}, err
	}

	return mediavariant.NewBuilder().
		SetMediaObjectID(obj.ID()).
		SetVariant(kind).
		SetObjectKey(key).
		SetWidth(tw).
		SetHeight(th).
		SetContentType(contentType).
		Build()
}

// variantEncoding picks the output content type and extension for a variant
// based on the original content type. PNG originals stay PNG; everything else
// (incl. JPEG) is encoded as JPEG.
func variantEncoding(originalContentType string) (contentType, ext string) {
	if originalContentType == "image/png" {
		return "image/png", ".png"
	}
	return "image/jpeg", ".jpg"
}

func encode(w io.Writer, img image.Image, contentType string) error {
	if contentType == "image/png" {
		return png.Encode(w, img)
	}
	return jpeg.Encode(w, img, &jpeg.Options{Quality: 85})
}
