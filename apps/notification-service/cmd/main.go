package main

import (
	"context"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/sirupsen/logrus"

	authmw "github.com/jtumidanski/myfleet/packages/shared-go/auth"
	"github.com/jtumidanski/myfleet/packages/shared-go/config"
	"github.com/jtumidanski/myfleet/packages/shared-go/database"
	"github.com/jtumidanski/myfleet/packages/shared-go/health"
	"github.com/jtumidanski/myfleet/packages/shared-go/server"
	"github.com/jtumidanski/myfleet/packages/shared-go/telemetry"

	"github.com/jtumidanski/myfleet/apps/notification-service/internal/admin"
	"github.com/jtumidanski/myfleet/apps/notification-service/internal/consumer"
	"github.com/jtumidanski/myfleet/apps/notification-service/internal/fleetclient"
	"github.com/jtumidanski/myfleet/apps/notification-service/internal/inbox"
	"github.com/jtumidanski/myfleet/apps/notification-service/internal/mailconsumer"
	"github.com/jtumidanski/myfleet/apps/notification-service/internal/mailer"
	"github.com/jtumidanski/myfleet/apps/notification-service/internal/notification"
	"github.com/jtumidanski/myfleet/apps/notification-service/internal/preferences"
	"github.com/jtumidanski/myfleet/apps/notification-service/internal/reminder"
)

func main() {
	log := telemetry.NewLogger()
	telemetry.InitTracer("notification-service")

	ctx := context.Background()

	db, err := database.Connect(log, database.SetMigrations(
		notification.Migration,
		preferences.Migration,
		inbox.Migration,
	))
	if err != nil {
		log.WithError(err).Fatal("db connect")
	}

	// Internal client for recipient resolution + the reminder due-feed (design D2):
	// cross-service data comes from fleet-service's network-restricted internal
	// endpoints, never from a cross-service DB join.
	fleetClient := fleetclient.NewClient(config.MustGet("FLEET_INTERNAL_URL"))

	// Shared notification generator (dedupe + per-user preference checks).
	prefsProc := preferences.NewProcessor(log, preferences.NewProvider(db), preferences.NewAdministrator(db))
	notifProc := notification.NewProcessor(log, notification.NewAdministrator(db), prefsProc)

	// Idempotent event consumers: one goroutine per subscribed topic, all in the
	// "notification" consumer group so partitions balance across replicas
	// (design §7). Each handler is mark-after-success → at-least-once safe.
	brokers := strings.Split(config.MustGet("KAFKA_BROKERS"), ",")
	inboxStore := inbox.New(log, db)
	cons := consumer.NewConsumer(log, inboxStore, fleetClient, notifProc)
	for _, topic := range consumer.Topics {
		go cons.Run(ctx, brokers, topic)
	}

	// Invite email delivery (design §3). A SEPARATE consumer group from the
	// in-app "notification" group, so a stalled relay cannot hold back in-app
	// notification offsets and vice versa.
	//
	// ConfigFromEnv fails at startup on a misconfiguration (FR-CFG-5); when
	// SMTP_ENABLED is false it reads nothing else and the consumer short-circuits
	// before any network call, so a cluster with no relay credentials is a
	// documented no-op rather than a crash loop (FR-CFG-4).
	mailCfg := mailer.ConfigFromEnv()
	var mailSender mailer.Sender
	if mailCfg.Enabled {
		mailSender = mailer.NewSMTPSender(mailCfg)
	}
	go mailconsumer.NewConsumer(log, inboxStore, fleetClient, mailSender, mailCfg).Run(ctx, brokers)
	if !mailCfg.Enabled {
		log.Warn("SMTP is disabled; invite emails will be recorded and skipped")
	}

	// Daily reminder safety-net: re-derive overdue schedules and generate the
	// same per-user notifications, deduped against the event path (design §11, A6).
	go reminder.NewJob(log, db, fleetClient, notifProc).Start(ctx)

	// notification-service validates auth-service JWTs via JWKS. Bounded retry so
	// it waits for auth-service readiness (same approach as the other services).
	keyfn := mustJWKSKeyfunc(log, config.MustGet("JWKS_URL"), 10, 3*time.Second)

	if err := server.New(log).
		Use(telemetry.CorrelationID).
		// Internal routes: no JWT, network-restricted (consumed by
		// fleet-service's admin console).
		//
		// SECURITY: notifications-stripprefix strips the FULL /api/notifications
		// prefix, so a public request to /api/notifications/internal/admin/purge
		// arrives here as /internal/admin/purge. These are the FIRST internal
		// routes this service has ever had and they DELETE DATA. The
		// priority-200 internal-deny rule in the main overlay's ingressroute is
		// what keeps them off the public internet; the two ship together and
		// never separately (design F2).
		AddRouteInitializer(admin.InitializeInternalRoutes(log, db)).
		AddRouteInitializer(func(r chi.Router) {
			r.Group(func(pr chi.Router) {
				pr.Use(authmw.JWT(keyfn))
				// notification-service trusts the token's user id; every endpoint is
				// scoped to the caller's own notifications (no fleet recheck).
				notification.InitializeRoutes(log, db, prefsProc)(pr)
				preferences.InitializeRoutes(log, db)(pr)
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
