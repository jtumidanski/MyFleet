package main

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	authmw "github.com/jtumidanski/myfleet/packages/shared-go/auth"
	"github.com/jtumidanski/myfleet/packages/shared-go/config"
	"github.com/jtumidanski/myfleet/packages/shared-go/database"
	"github.com/jtumidanski/myfleet/packages/shared-go/events"
	"github.com/jtumidanski/myfleet/packages/shared-go/health"
	"github.com/jtumidanski/myfleet/packages/shared-go/jobs"
	"github.com/jtumidanski/myfleet/packages/shared-go/server"
	"github.com/jtumidanski/myfleet/packages/shared-go/telemetry"

	"github.com/jtumidanski/myfleet/apps/media-service/internal/mediaobject"
	"github.com/jtumidanski/myfleet/apps/media-service/internal/mediavariant"
	"github.com/jtumidanski/myfleet/apps/media-service/internal/processedevents"
	"github.com/jtumidanski/myfleet/apps/media-service/internal/processing"
	"github.com/jtumidanski/myfleet/apps/media-service/internal/storage"
	"github.com/jtumidanski/myfleet/apps/media-service/internal/variantfailures"
)

func main() {
	log := telemetry.NewLogger()
	telemetry.InitTracer("media-service")

	ctx := context.Background()

	db, err := database.Connect(log, database.SetMigrations(
		mediaobject.Migration,
		mediavariant.Migration,
		processedevents.Migration,
		variantfailures.Migration,
		events.MigrateOutbox,
	))
	if err != nil {
		log.WithError(err).Fatal("db connect")
	}

	// MinIO client (private bucket, auto-created on startup).
	store, err := storage.New(ctx, storage.Config{
		Endpoint:  config.MustGet("MINIO_ENDPOINT"),
		AccessKey: config.MustGet("MINIO_ACCESS_KEY"),
		SecretKey: config.MustGet("MINIO_SECRET_KEY"),
		UseSSL:    config.Get("MINIO_USE_SSL", "false") == "true",
		Bucket:    config.MustGet("MEDIA_BUCKET"),
	})
	if err != nil {
		log.WithError(err).Fatal("minio connect")
	}

	// Kafka producer for the outbox relay (design A8). The relay loop reads
	// unsent outbox rows and publishes them; Confirm no longer calls Publish
	// directly so the status update and the outbox row are always atomic.
	brokers := strings.Split(config.MustGet("KAFKA_BROKERS"), ",")
	producer := events.NewKafkaProducer(brokers)
	defer func() {
		// Close flushes buffered writes; unflushed rows stay unsent in the
		// outbox and the relay retries them on the next boot, so log and move on.
		if err := producer.Close(); err != nil {
			log.WithError(err).Warn("closing kafka producer")
		}
	}()

	// media-service validates auth-service JWTs via JWKS. Bounded retry so it
	// waits for auth-service readiness (same approach as fleet-service).
	keyfn := mustJWKSKeyfunc(log, config.MustGet("JWKS_URL"), 10, 3*time.Second)

	// Variant worker pool: each goroutine joins the same consumer group so
	// partitions of media.uploaded are balanced across workers (design A7).
	dedupe := processedevents.New(log, db)
	worker := processing.NewWorker(
		log,
		store,
		mediaobject.NewProvider(db),
		mediaobject.NewAdministrator(db),
		mediavariant.NewAdministrator(db),
		dedupe,
	)
	workerCount := config.GetInt("MEDIA_WORKERS", 2)
	if workerCount < 1 {
		workerCount = 1
	}
	// Cap proxied uploads. 25 MiB sits under Cloudflare's free-plan request-body
	// ceiling, so the edge is not the first thing a user discovers.
	maxUploadBytes := int64(config.GetInt("MEDIA_MAX_UPLOAD_BYTES", 26214400))

	// Lazy card-variant generation (task-013). 0 disables it entirely and
	// negatives clamp to 0 inside NewCardGenerator — this feature schedules work
	// in response to request traffic, so an operator who sees it misbehave needs
	// an off switch that is not a rollback. With it off, a missing card is still
	// served as a thumbnail; that is simply the pre-task behaviour.
	cardGen := processing.NewCardGenerator(
		ctx,
		log,
		store,
		mediavariant.NewAdministrator(db),
		variantfailures.New(log, db),
		config.GetInt("MEDIA_LAZY_VARIANT_CONCURRENCY", 4),
	)

	// The allowlist is a security control (PRD FR-MEDIA-1). Fatal on a parse
	// error: a malformed list must not boot into "allow nothing" or "allow
	// everything".
	allow, err := mediaobject.ParseAllowlist(
		config.Get("MEDIA_ALLOWED_CONTENT_TYPES", mediaobject.DefaultAllowedContentTypes))
	if err != nil {
		log.WithError(err).Fatal("parse MEDIA_ALLOWED_CONTENT_TYPES")
	}
	for i := 0; i < workerCount; i++ {
		go worker.Run(ctx, brokers, "media-variant-workers")
	}

	// Daily media purge: hard-delete soft-deleted objects past purge_after,
	// removing both the rows and the MinIO objects. Under advisory lock so only
	// one replica runs per tick (design §10.6 / A9).
	go jobs.Every(ctx, 24*time.Hour, func(ctx context.Context) error {
		_, err := database.WithLeaderLock(db, "media-purge", func() error {
			return purgeExpired(ctx, log, db, store)
		})
		if err != nil {
			log.WithError(err).Warn("media purge sweep failed")
		}
		return err
	})

	// Transactional-outbox relay (design A8): every 2s, publish unsent outbox
	// rows to Kafka and mark them sent. Runs under an advisory lock so only one
	// replica relays per tick (design A9), preventing duplicate publishes.
	go jobs.Every(ctx, 2*time.Second, func(ctx context.Context) error {
		_, err := database.WithLeaderLock(db, "media-outbox", func() error {
			return events.RelayOnce(ctx, log, db, producer)
		})
		return err
	})

	if err := server.New(log).
		Use(telemetry.CorrelationID).
		// Internal route: no JWT, network-restricted (consumed by fleet-service
		// to validate documentMediaIds). Kept off the public internet by the
		// priority-200 internal-deny rule in the main overlay's ingressroute.
		AddRouteInitializer(mediaobject.InitializeInternalRoutes(log, db)).
		AddRouteInitializer(func(r chi.Router) {
			r.Group(func(pr chi.Router) {
				pr.Use(authmw.JWT(keyfn))
				mediaobject.InitializeRoutes(log, db, store,
					variantLookup{p: mediavariant.NewProvider(db)},
					maxUploadBytes, allow,
					mediaobject.WithCardGenerator(cardGenerator{g: cardGen}),
				)(pr)
			})
		}).
		AddRouteInitializer(func(r chi.Router) {
			r.Get("/healthz", health.Liveness())
			r.Get("/readyz", health.Readiness(func() error { d, _ := db.DB(); return d.Ping() }))
			r.Handle("/metrics", health.Metrics())
		}).
		Run(); err != nil {
		log.WithError(err).Fatal("server stopped")
	}
}

// purgeExpired removes soft-deleted media objects past their purge window: it
// deletes the MinIO object then the DB row for each.
func purgeExpired(ctx context.Context, log logrus.FieldLogger, db *gorm.DB, store *storage.Client) error {
	objs, err := mediaobject.ListPurgeable(db)
	if err != nil {
		return err
	}
	for _, o := range objs {
		if err := store.RemoveObject(ctx, o.ObjectKey()); err != nil {
			log.WithError(err).WithField("media_id", o.ID()).Warn("remove minio object during purge failed")
			continue // leave the row so a later sweep retries
		}
		if err := mediaobject.DeleteRow(db, o.ID()); err != nil {
			log.WithError(err).WithField("media_id", o.ID()).Warn("delete media row during purge failed")
		}
	}
	return nil
}

// variantLookup adapts mediavariant.Provider to mediaobject.VariantLookup.
//
// It lives here, in the composition root — the one place that already imports
// both packages — so that mediaobject never imports mediavariant and the two
// sibling domain packages stay independent (design §3.1).
// It is also where mediavariant.ErrNotFound is translated into the port's
// found=false, so mediaobject never has to import the sibling package's
// sentinel to tell "not generated yet" from "the database is broken".
type variantLookup struct{ p mediavariant.Provider }

func (v variantLookup) Lookup(ctx context.Context, mediaObjectID, variant string) (mediaobject.VariantRef, bool, error) {
	m, err := v.p.GetByMediaObjectAndVariant(ctx, mediaObjectID, mediavariant.Variant(variant))
	if errors.Is(err, mediavariant.ErrNotFound) {
		return mediaobject.VariantRef{}, false, nil
	}
	if err != nil {
		return mediaobject.VariantRef{}, false, err
	}
	return mediaobject.VariantRef{ObjectKey: m.ObjectKey(), ContentType: m.ContentType()}, true, nil
}

// cardGenerator adapts processing.CardGenerator to mediaobject.CardGenerator.
//
// It lives here, in the composition root — the one place that already imports
// both packages — so that mediaobject never imports processing and the two
// sibling packages stay independent, exactly as variantLookup does above.
type cardGenerator struct{ g *processing.CardGenerator }

func (c cardGenerator) Generate(src mediaobject.CardSource) {
	c.g.Generate(processing.Source{
		MediaObjectID: src.MediaObjectID,
		FleetID:       src.FleetID,
		ObjectKey:     src.ObjectKey,
		ContentType:   src.ContentType,
	})
}

// mustJWKSKeyfunc builds the JWKS keyfunc, retrying up to maxAttempts times with
// the given delay between attempts. Fatals if all attempts fail.
func mustJWKSKeyfunc(log *logrus.Logger, jwksURL string, maxAttempts int, delay time.Duration) jwt.Keyfunc {
	ctx := context.Background()
	var (
		keyfn jwt.Keyfunc
		err   error
	)
	for i := 1; i <= maxAttempts; i++ {
		keyfn, err = authmw.NewJWKSKeyfunc(ctx, jwksURL)
		if err == nil {
			return keyfn
		}
		log.WithError(err).Warnf("JWKS keyfunc attempt %d/%d failed; retrying in %s", i, maxAttempts, delay)
		if i < maxAttempts {
			time.Sleep(delay)
		}
	}
	log.WithError(err).Fatal("JWKS keyfunc init failed after all attempts")
	return nil // unreachable
}
