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
	"github.com/jtumidanski/myfleet/packages/shared-go/jobs"
	"github.com/jtumidanski/myfleet/packages/shared-go/server"
	"github.com/jtumidanski/myfleet/packages/shared-go/telemetry"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/fleet"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/fuel"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/invite"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/maintenancecategory"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/maintenancerecord"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/maintenanceschedule"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/membership"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/mileage"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/vehicle"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/vehiclemedia"
)

func main() {
	log := telemetry.NewLogger()
	telemetry.InitTracer("fleet-service")

	db, err := database.Connect(log, database.SetMigrations(
		fleet.Migration,
		membership.Migration,
		invite.Migration,
		vehicle.Migration,
		vehiclemedia.Migration,
		mileage.Migration,
		maintenancecategory.Migration,
		maintenancerecord.Migration,
		maintenanceschedule.Migration,
		fuel.Migration,
		events.MigrateOutbox,
	))
	if err != nil {
		log.WithError(err).Fatal("db connect")
	}

	// Seed system-defined maintenance categories (idempotent; FR-MAINT-1).
	if err := maintenancecategory.Seed(db); err != nil {
		log.WithError(err).Fatal("seed maintenance categories")
	}

	// Fleet-service validates auth-service JWT tokens via JWKS.
	// Retry up to 10 times (3 s between attempts) so fleet-service waits for
	// auth-service to be ready instead of fataling immediately.
	jwksURL := config.MustGet("JWKS_URL")
	keyfn := mustJWKSKeyfunc(log, jwksURL, 10, 3*time.Second)

	membershipAdmin := membership.NewAdministrator(db)
	membershipProc := membership.NewProcessor(log, membership.NewProvider(db))

	// Build shared processors for cross-domain injection.
	vehicleAdmin := vehicle.NewAdministrator(db)
	vehicleProc := vehicle.NewProcessor(log, vehicle.NewProvider(db), vehicleAdmin)
	vehiclemediaProc := vehiclemedia.NewProcessor(log, vehiclemedia.NewProvider(db), vehiclemedia.NewAdministrator(db))

	// Maintenance: schedule processor (for the recompute job) + completion deps
	// (record insert + mileage append + schedule advance, run in one tx — §10.3).
	scheduleProc := maintenanceschedule.NewProcessor(log, maintenanceschedule.NewProvider(db), maintenanceschedule.NewAdministrator(db))
	completionDeps := maintenanceschedule.NewCompletionDeps(db, maintenancerecord.NewAdministrator(db), maintenanceschedule.NewAdministrator(db))

	// Read-only accessors for deriving vehicle status on read (design §10.2).
	// Schedule states come from the schedule processor (live DueState); last
	// activity comes from the activity domain (falls back to vehicle created_at).
	vehicleStatusDeps := vehicle.StatusDeps{
		Schedules: scheduleProc,
		Activity:  zeroActivity{},
	}

	// Background sweep: hard-delete soft-deleted vehicles past their purge window.
	// Runs under advisory lock so only one replica executes per tick (design A9).
	ctx := context.Background()
	go jobs.Every(ctx, 24*time.Hour, func(ctx context.Context) error {
		_, err := database.WithLeaderLock(db, "vehicle-purge", func() error {
			return vehicle.PurgeExpired(db)
		})
		if err != nil {
			log.WithError(err).Warn("vehicle purge sweep failed")
		}
		return err
	})

	// Background recompute: re-derive status/severity/next_due_* for active
	// maintenance schedules hourly (FR-MAINT-6). Runs under advisory lock so
	// only one replica executes per tick (design A9).
	go jobs.Every(ctx, 1*time.Hour, func(ctx context.Context) error {
		_, err := database.WithLeaderLock(db, "maintenance-recompute", func() error {
			return scheduleProc.RecomputeAll(time.Now().UTC())
		})
		if err != nil {
			log.WithError(err).Warn("maintenance recompute sweep failed")
		}
		return err
	})

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
				vehicle.InitializeRoutes(log, db, membershipProc, vehiclemediaProc, vehicleStatusDeps)(pr)
				vehiclemedia.InitializeRoutes(log, db, vehicleProc)(pr)
				mileage.InitializeRoutes(log, db, vehicleProc, vehicleAdmin)(pr)
				fuel.InitializeRoutes(log, db, vehicleProc)(pr)
				maintenancecategory.InitializeRoutes(log, db)(pr)
				maintenancerecord.InitializeRoutes(log, db, vehicleProc)(pr)
				maintenanceschedule.InitializeRoutes(log, db, vehicleProc, completionDeps)(pr)
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

// zeroActivity is a placeholder LastActivityGatherer used until the activity
// domain is wired (Phase 11.2). Returning the zero time makes DeriveStatus fall
// back to the vehicle's created_at, so a fresh vehicle reads as "Healthy".
type zeroActivity struct{}

func (zeroActivity) LastActivityByVehicle(string) (time.Time, error) { return time.Time{}, nil }

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
