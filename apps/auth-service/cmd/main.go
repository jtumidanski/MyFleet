package main

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"strconv"

	"github.com/go-chi/chi/v5"

	authmw "github.com/jtumidanski/myfleet/packages/shared-go/auth"
	"github.com/jtumidanski/myfleet/packages/shared-go/config"
	"github.com/jtumidanski/myfleet/packages/shared-go/database"
	"github.com/jtumidanski/myfleet/packages/shared-go/health"
	"github.com/jtumidanski/myfleet/packages/shared-go/server"
	"github.com/jtumidanski/myfleet/packages/shared-go/telemetry"

	"github.com/jtumidanski/myfleet/apps/auth-service/internal/jwks"
	"github.com/jtumidanski/myfleet/apps/auth-service/internal/membership"
	"github.com/jtumidanski/myfleet/apps/auth-service/internal/oidc"
	"github.com/jtumidanski/myfleet/apps/auth-service/internal/session"
	"github.com/jtumidanski/myfleet/apps/auth-service/internal/user"
)

func main() {
	log := telemetry.NewLogger()
	telemetry.InitTracer("auth-service")

	db, err := database.Connect(log, database.SetMigrations(user.Migration, session.Migration))
	if err != nil {
		log.WithError(err).Fatal("db connect")
	}

	// In-memory signing key. auth-service validates its OWN tokens against this
	// key directly (Decision 2) rather than HTTP-fetching its own JWKS, which
	// would deadlock at startup. Other services use NewJWKSKeyfunc against the
	// JWKS_URL this service publishes.
	ks := loadKeySet()

	// session processor wired with its read/write store for refresh rotation.
	sess := session.
		NewProcessor(log, ks, config.Get("JWT_ISSUER", "myfleet-auth"), config.Get("JWT_AUDIENCE", "myfleet")).
		WithStore(session.NewProvider(db), session.NewAdministrator(db))

	// Decision 1: build the concrete membership client and adapt it to the
	// MembershipResolver function value injected into session/oidc, so neither
	// of those packages imports the concrete client (no import cycle).
	fleetClient := membership.NewClient(config.Get("FLEET_SERVICE_URL", "http://fleet-service:8080"))
	resolve := func(ctx context.Context, userID string) (string, string, error) {
		m, err := fleetClient.Active(ctx, userID)
		if err != nil {
			return "", "", err
		}
		return m.FleetID, m.Role, nil
	}

	// COOKIE_SECURE controls the Secure flag on every cookie this service sets.
	// Local dev runs over plaintext HTTP (Traefik :80) where Secure cookies are
	// dropped by browsers, so it defaults true but is set false in dev .env.
	cookieSecure, err := strconv.ParseBool(config.Get("COOKIE_SECURE", "true"))
	if err != nil {
		cookieSecure = true
	}

	users := user.NewProcessor(log, user.NewProvider(db), user.NewAdministrator(db))

	oidcProc := oidc.NewProcessor(
		config.MustGet("GOOGLE_CLIENT_ID"),
		config.MustGet("GOOGLE_CLIENT_SECRET"),
		config.MustGet("GOOGLE_REDIRECT_URL"),
	)
	oidcDeps := oidc.Dependencies{
		OIDC:           oidcProc,
		Users:          users,
		Sessions:       sess,
		Resolve:        resolve,
		StateSecret:    []byte(config.MustGet("OIDC_STATE_SECRET")),
		AppBaseURL:     config.Get("APP_BASE_URL", "http://localhost"),
		HomePath:       config.Get("APP_HOME_PATH", "/"),
		OnboardingPath: config.Get("APP_ONBOARDING_PATH", "/onboarding"),
		CookieSecure:   cookieSecure,
	}

	if err := server.New(log).
		Use(telemetry.CorrelationID).
		// public routes (no JWT): jwks, oidc login/callback, refresh/logout
		AddRouteInitializer(jwks.InitializeRoutes(ks)).
		AddRouteInitializer(oidc.InitializeRoutes(log, oidcDeps)).
		AddRouteInitializer(session.InitializePublicRoutes(log, sess, resolve, cookieSecure)).
		// protected routes (JWT validated against the in-memory key): /auth/me
		AddRouteInitializer(func(r chi.Router) {
			r.Group(func(pr chi.Router) {
				pr.Use(authmw.JWT(ks.Keyfunc()))
				user.InitializeRoutes(log, db)(pr)
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

func loadKeySet() *jwks.KeySet {
	block, _ := pem.Decode([]byte(config.MustGet("JWT_PRIVATE_KEY_PEM")))
	if block == nil {
		panic("JWT_PRIVATE_KEY_PEM is not a valid PEM block")
	}
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		panic(err)
	}
	return jwks.NewKeySet(key, config.Get("JWT_KID", "kid-1"))
}
