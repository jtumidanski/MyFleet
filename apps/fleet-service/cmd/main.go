package main

import (
	"context"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/sirupsen/logrus"

	authmw "github.com/jtumidanski/myfleet/packages/shared-go/auth"
	"github.com/jtumidanski/myfleet/packages/shared-go/config"
	"github.com/jtumidanski/myfleet/packages/shared-go/database"
	"github.com/jtumidanski/myfleet/packages/shared-go/events"
	"github.com/jtumidanski/myfleet/packages/shared-go/health"
	"github.com/jtumidanski/myfleet/packages/shared-go/server"
	"github.com/jtumidanski/myfleet/packages/shared-go/telemetry"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/fleet"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/invite"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/membership"
)

func main() {
	log := telemetry.NewLogger()
	telemetry.InitTracer("fleet-service")

	db, err := database.Connect(log, database.SetMigrations(
		fleet.Migration,
		membership.Migration,
		invite.Migration,
		events.MigrateOutbox,
	))
	if err != nil {
		log.WithError(err).Fatal("db connect")
	}

	// Fleet-service validates auth-service JWT tokens via JWKS.
	// Retry up to 10 times (3 s between attempts) so fleet-service waits for
	// auth-service to be ready instead of fataling immediately.
	jwksURL := config.MustGet("JWKS_URL")
	keyfn := mustJWKSKeyfunc(log, jwksURL, 10, 3*time.Second)

	membershipAdmin := membership.NewAdministrator(db)
	membershipProc := membership.NewProcessor(log, membership.NewProvider(db))

	if err := server.New(log).
		Use(telemetry.CorrelationID).
		// Internal route: no JWT, network-restricted.
		AddRouteInitializer(membership.InitializeInternalRoutes(log, db)).
		// Protected routes: JWT required.
		AddRouteInitializer(func(r chi.Router) {
			r.Group(func(pr chi.Router) {
				pr.Use(authmw.JWT(keyfn))
				fleet.InitializeRoutes(log, db, membershipAdmin, membershipProc)(pr)
				membership.InitializeRoutes(log, db)(pr)
				invite.InitializeRoutes(log, db, membershipProc)(pr)
			})
		}).
		AddRouteInitializer(func(r chi.Router) {
			r.Get("/healthz", health.Liveness())
			r.Get("/readyz", health.Readiness(func() error { d, _ := db.DB(); return d.Ping() }))
		}).
		Run(); err != nil {
		log.WithError(err).Fatal("server stopped")
	}
}

// mustJWKSKeyfunc builds the JWKS keyfunc, retrying up to maxAttempts times
// with the given delay between attempts. Fatals if all attempts fail.
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
