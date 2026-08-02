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

	dtoevents "github.com/jtumidanski/myfleet/packages/dto-go/events"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/activity"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/dashboard"
	fleetevents "github.com/jtumidanski/myfleet/apps/fleet-service/internal/events"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/fleet"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/fuel"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/invite"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/maintenancecategory"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/maintenancerecord"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/maintenanceschedule"
	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/mediaclient"
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
		activity.Migration,
		events.MigrateOutbox,
		dashboard.Migration,
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

	// Activity feed (append-only). Record is called from other domains' txns.
	activityProc := activity.NewProcessor(log, activity.NewProvider(db))

	// Outbox emit adapters (design A8). Each builds the canonical Envelope from a
	// dto-go payload and Enqueues it on the caller's tx; failures roll the tx back.
	emitVehicleCreated := func(tx *gorm.DB, fleetID, actorID, traceID, vehicleID string) error {
		return fleetevents.EmitVehicleCreated(tx, fleetID, actorID, traceID,
			dtoevents.VehicleCreatedData{VehicleID: vehicleID, FleetID: fleetID})
	}
	emitFuelLogged := func(tx *gorm.DB, fleetID, actorID, traceID, fuelLogID, vehicleID string, mileage int, totalCost float64) error {
		return fleetevents.EmitFuelLogged(tx, fleetID, actorID, traceID,
			dtoevents.FuelLoggedData{FuelLogID: fuelLogID, VehicleID: vehicleID, Mileage: mileage, TotalCost: totalCost})
	}
	emitMemberInvited := func(tx *gorm.DB, fleetID, actorID, traceID, inviteID, email, role string) error {
		return fleetevents.EmitMemberInvited(tx, fleetID, actorID, traceID,
			dtoevents.MemberInvitedData{InviteID: inviteID, Email: email, Role: role})
	}
	emitMaintenanceCompleted := func(tx *gorm.DB, fleetID, actorID, traceID, scheduleID, vehicleID, recordID, categoryID string) error {
		return fleetevents.EmitMaintenanceCompleted(tx, fleetID, actorID, traceID,
			dtoevents.MaintenanceCompletedData{ScheduleID: scheduleID, VehicleID: vehicleID, MaintenanceRecord: recordID, CategoryID: categoryID})
	}
	emitScheduleOverdue := func(tx *gorm.DB, fleetID, scheduleID, vehicleID, severity, dueCycle string) error {
		// System-generated transition: no human actor / correlation id.
		return fleetevents.EmitScheduleOverdue(tx, fleetID, "system", "",
			dtoevents.ScheduleOverdueData{ScheduleID: scheduleID, VehicleID: vehicleID, Severity: severity, DueCycle: dueCycle})
	}

	// Maintenance: schedule processor (for the recompute job) + completion deps
	// (record insert + mileage append + schedule advance, run in one tx — §10.3).
	// The recompute job appends a schedule.overdue activity event + outbox event
	// on the transition edge (A8).
	scheduleProc := maintenanceschedule.NewProcessor(log, maintenanceschedule.NewProvider(db), maintenanceschedule.NewAdministrator(db)).
		WithOverdueHooks(db, activity.Record, emitScheduleOverdue)
	completionDeps := maintenanceschedule.NewCompletionDeps(db, maintenancerecord.NewAdministrator(db), maintenanceschedule.NewAdministrator(db)).
		WithActivityRecorder(activity.Record).
		WithEmitter(emitMaintenanceCompleted)

	// Read-only accessors for deriving a vehicle's status, last activity, and
	// governing due detail on read (design §10.2). Schedule detail comes from the
	// schedule processor through an adapter; last activity comes from the
	// activity domain (falls back to vehicle created_at).
	vehicleStatusDeps := vehicle.StatusDeps{
		Schedules: scheduleDueAdapter{p: scheduleProc},
		Activity:  activityProc,
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

	// Transactional-outbox relay (design A8): every 2s, publish unsent outbox
	// rows to Kafka and mark them sent. Runs under an advisory lock so only one
	// replica relays per tick (design A9), preventing duplicate publishes.
	brokers := strings.Split(config.MustGet("KAFKA_BROKERS"), ",")
	producer := events.NewKafkaProducer(brokers)
	go jobs.Every(ctx, 2*time.Second, func(ctx context.Context) error {
		_, err := database.WithLeaderLock(db, "fleet-outbox", func() error {
			return events.RelayOnce(ctx, log, db, producer)
		})
		return err
	})

	// Category accessor for the record list's ?kind= filter. The processor is
	// stateless, so constructing a second one here rather than reshaping
	// maintenancecategory.InitializeRoutes costs nothing.
	categoryProc := maintenancecategory.NewProcessor(log, maintenancecategory.NewProvider(db))

	// Attachment ownership validation (PRD FR-DOC-6). Cluster-internal; the
	// endpoint it calls is kept off the public internet by the priority-200
	// internal-deny rule in the main overlay's ingressroute.
	mediaClient := mediaclient.NewClient(config.Get("MEDIA_INTERNAL_URL", "http://media-service:8080"))

	if err := server.New(log).
		Use(telemetry.CorrelationID).
		// Internal routes: no JWT, network-restricted (consumed by other services).
		AddRouteInitializer(membership.InitializeInternalRoutes(log, db)).
		AddRouteInitializer(maintenanceschedule.InitializeInternalRoutes(log, db)).
		// Protected routes: JWT required.
		AddRouteInitializer(func(r chi.Router) {
			r.Group(func(pr chi.Router) {
				pr.Use(authmw.JWT(keyfn, authmw.WithLogger(log)))
				fleet.InitializeRoutes(log, db, membershipAdmin, membershipProc)(pr)
				membership.InitializeRoutes(log, db)(pr)
				invite.InitializeRoutes(log, db, membershipProc, activity.Record, emitMemberInvited)(pr)
				vehicle.InitializeRoutes(log, db, membershipProc, vehiclemediaProc, vehicleStatusDeps, activity.Record, emitVehicleCreated)(pr)
				vehiclemedia.InitializeRoutes(log, db, vehicleProc)(pr)
				mileage.InitializeRoutes(log, db, vehicleProc, vehicleAdmin)(pr)
				fuel.InitializeRoutes(log, db, vehicleProc, activity.Record, emitFuelLogged)(pr)
				maintenancecategory.InitializeRoutes(log, db)(pr)
				maintenancerecord.InitializeRoutes(log, db, vehicleProc, categoryProc, mediaClient)(pr)
				maintenanceschedule.InitializeRoutes(log, db, vehicleProc, completionDeps)(pr)
				activity.InitializeRoutes(log, db, vehicleProc)(pr)
				dashboard.InitializeRoutes(log, db, scheduleProc, activityProc, vehicleProc)(pr)
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

// scheduleDueAdapter maps maintenanceschedule's due detail onto the vehicle
// domain's port type. The mapping lives here, in the composition root, so
// neither domain imports the other; a field added on one side becomes a compile
// error here rather than a silently dropped value.
//
// The previous binding worked by structural typing because the gatherer returned
// a []string. With named struct types on both sides it cannot, which is the
// point: the boundary is now explicit.
type scheduleDueAdapter struct {
	p *maintenanceschedule.Processor
}

func (a scheduleDueAdapter) ScheduleDueByVehicle(vehicleID string) ([]vehicle.ScheduleDue, error) {
	dues, err := a.p.ScheduleDueByVehicle(vehicleID)
	if err != nil {
		return nil, err
	}
	out := make([]vehicle.ScheduleDue, 0, len(dues))
	for _, d := range dues {
		breaches := make([]vehicle.Breach, 0, len(d.Breaches))
		for _, b := range d.Breaches {
			breaches = append(breaches, vehicle.Breach{
				Axis:    b.Axis,
				Days:    b.Days,
				Miles:   b.Miles,
				Urgency: b.Urgency,
			})
		}
		out = append(out, vehicle.ScheduleDue{
			ScheduleID: d.ScheduleID,
			State:      d.State,
			Breaches:   breaches,
		})
	}
	return out, nil
}
