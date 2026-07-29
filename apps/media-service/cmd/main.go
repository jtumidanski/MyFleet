package main

import (
	"context"
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
)

func main() {
	log := telemetry.NewLogger()
	telemetry.InitTracer("media-service")

	ctx := context.Background()

	db, err := database.Connect(log, database.SetMigrations(
		mediaobject.Migration,
		mediavariant.Migration,
		processedevents.Migration,
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
		AddRouteInitializer(func(r chi.Router) {
			r.Group(func(pr chi.Router) {
				pr.Use(authmw.JWT(keyfn))
				mediaobject.InitializeRoutes(log, db, store)(pr)
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
