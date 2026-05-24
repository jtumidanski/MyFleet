# Household Vehicle Management Platform — Full MVP Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the entire Household Vehicle Management Platform MVP — four Go microservices (`auth`, `fleet`, `media`, `notification`), shared Go/TS packages, a React/TS SPA, a Traefik-fronted docker-compose stack, Kafka event backbone, CI/CD, and k3s manifests — as a single task.

**Architecture:** Native monorepo (`go.work` + npm workspaces). `fleet-service` owns the entire core domain (D1); `auth`/`media`/`notification` stay narrow. One PostgreSQL instance with an isolated schema per service (D2); no cross-service joins — id references resolved via internal APIs. `auth-service` verifies Google OIDC and mints RS256 JWTs served via JWKS (D3, A3); every service validates them in shared middleware (A2). Single-origin Traefik gateway does TLS/CORS/routing only (D4). Event producers use a transactional outbox (A8); consumers are idempotent (A6); background jobs run under a Postgres advisory lock (A9).

**Tech Stack:** Go 1.24 (GORM, logrus, OpenTelemetry, segmentio/kafka-go, golang-jwt, minio-go), React 18 + TypeScript (strict) + Vite, shadcn/ui + Tailwind, TanStack React Query, react-hook-form + Zod, PostgreSQL 16, MinIO, Kafka (Redpanda for local), Traefik v3, Docker, k3s/Kustomize, GitHub Actions, Renovate.

---

## How to use this plan

This plan implements the **full MVP** described in `design.md`. It is organized into **phases** that mirror the design's roadmap slices (§19), each containing **tasks** broken into bite-sized TDD steps. Execute phases in order; within a phase, tasks are ordered by dependency.

**Canonical patterns are written once, in full.** The shared packages (Phase 1), the canonical Go domain template (Phase 5, `vehicle`), the auth middleware, the event/outbox machinery, and the canonical React feature (Phase 14) contain complete, copy-ready code. Subsequent same-shape work (the other ~12 fleet-service domain packages, the other React features) is specified as a **concrete task spec**: exact files, fields, invariants, endpoints, the test cases to write (with assertions), and exact verify commands — implemented by following the already-written canonical pattern. This is deliberate: the layered backend template (`model/entity/builder/processor/provider/administrator/resource/rest`) is identical across domains and is auto-loaded from the `backend-dev-guidelines` skill during execution; the `frontend-dev-guidelines` skill governs every React feature. Re-reading those skills is part of each domain/feature task.

**Design is the source of truth for behavior.** Where a step says "per design §10.1", open `design.md` — the algorithms (recurrence, status derivation, price derivation, dedupe), the data model (§13 + PRD §6), and the error envelope (§6) are specified there and must not be re-derived.

**Verification gate (every backend task):** `go build ./...`, `go vet ./...`, `go test -race ./...` clean for the touched module. **(every frontend task):** `npm run -w apps/web build` and `npm run -w apps/web test` clean. **(infra tasks):** the named `docker compose` / `kubectl --dry-run` command succeeds. Never check a step's box until its stated "Expected" output is observed (superpowers:verification-before-completion).

**Commit cadence:** commit after each task (not each step) unless a step says otherwise. Conventional-commit messages scoped to the service/package (e.g. `feat(fleet): vehicle CRUD`).

---

## Phase index

| Phase | Title | Implements (design §) |
|---|---|---|
| 0 | Monorepo foundation & tooling | §4 |
| 1 | `shared-go` backbone | §5, §6 bootstrap, §15 |
| 2 | `dto-go`, `shared-ts`, `ui-components` skeletons | §5 |
| 3 | Local infra: docker-compose + Traefik | §2, §17 |
| 4 | `auth-service` (OIDC, JWT, JWKS, refresh) | §8.1, §9 |
| 5 | `fleet-service`: fleet, membership, invite + authz spine | §8.2, §9, §10.6 |
| 6 | `fleet-service`: vehicle + vehicle media refs | §8.2 |
| 7 | `media-service` (MinIO, presigned, variants) | §8.3, §10.6 |
| 8 | `fleet-service`: mileage | §8.2, §10.4 |
| 9 | `fleet-service`: maintenance (categories/records/schedules) | §8.2, §10.1–10.3, §11 |
| 10 | `fleet-service`: fuel (+ fuel→mileage) | §8.2, §10.5 |
| 11 | `fleet-service`: status derivation, activity, event production + outbox | §10.2, §8.2, §7, A8 |
| 12 | `notification-service` (consumers, prefs, reminder job) | §8.4, §7, §11, A6 |
| 13 | `fleet-service`: dashboard widget system | §12, §8.2, A5 |
| 14 | `apps/web`: shell, auth, API client, canonical feature | §12 |
| 15 | `apps/web`: remaining feature areas | §12 |
| 16 | CI/CD: GitHub Actions + Gitleaks + Renovate | §17 |
| 17 | k3s manifests & release hardening | §17 |
| 18 | End-to-end acceptance verification | §16, PRD §10 |

---

## Phase 0 — Monorepo foundation & tooling

Establishes the directory skeleton, Go workspace, npm workspaces, and Makefile from design §4. No business logic.

### Task 0.1: Repository skeleton & Go workspace

**Files:**
- Create: `go.work`
- Create: `package.json` (root)
- Create: `Makefile`
- Create: `.gitignore` (extend existing)
- Create: `apps/.gitkeep`, `packages/.gitkeep`, `scripts/.gitkeep`, `deploy/compose/.gitkeep`, `deploy/k8s/.gitkeep`

- [ ] **Step 1: Create the directory skeleton**

```bash
cd "$(git rev-parse --show-toplevel)"
mkdir -p apps packages scripts deploy/compose deploy/k8s
touch apps/.gitkeep packages/.gitkeep scripts/.gitkeep deploy/compose/.gitkeep deploy/k8s/.gitkeep
```

- [ ] **Step 2: Create the root `go.work`**

`go.work` (modules are added by `go work use` as each is created; start empty-but-valid):

```
go 1.24
```

- [ ] **Step 3: Create the root npm workspace `package.json`**

`package.json`:

```json
{
  "name": "myfleet",
  "private": true,
  "version": "0.0.0",
  "workspaces": [
    "apps/web",
    "packages/shared-ts",
    "packages/ui-components"
  ],
  "scripts": {
    "build": "npm run build --workspaces --if-present",
    "test": "npm run test --workspaces --if-present",
    "lint": "npm run lint --workspaces --if-present"
  }
}
```

- [ ] **Step 4: Create the `Makefile`**

`Makefile`:

```makefile
GO_MODULES := $(shell go work edit -json | python3 -c "import json,sys;[print(m['DiskPath']) for m in json.load(sys.stdin)['Use']]" 2>/dev/null)

.PHONY: build test vet tidy fe-build fe-test up down lint ci

build: ## go build every module in the workspace
	go build ./...

vet:
	go vet ./...

test:
	go test -race ./...

tidy:
	@for m in $(GO_MODULES); do (cd $$m && go mod tidy); done

fe-build:
	npm run -w apps/web build

fe-test:
	npm run -w apps/web test

up:
	docker compose -f deploy/compose/docker-compose.yml up -d --build

down:
	docker compose -f deploy/compose/docker-compose.yml down -v

ci: vet test build fe-test fe-build
```

- [ ] **Step 5: Extend `.gitignore`**

Append to `.gitignore`:

```
# Go
*.test
*.out
bin/
# Node
node_modules/
dist/
# Env / secrets
.env
*.local
# MinIO/data
deploy/compose/data/
```

- [ ] **Step 6: Verify workspace is valid**

Run: `go work edit -json && go build ./... 2>&1 | head` and `npm install --workspaces --dry-run`
Expected: `go build` exits 0 (no modules yet → no output); `go work edit -json` prints valid JSON with empty/zero `Use`.

- [ ] **Step 7: Commit**

```bash
git add go.work package.json Makefile .gitignore apps packages scripts deploy
git commit -m "chore: monorepo skeleton, go.work, npm workspaces, Makefile"
```

---

## Phase 1 — `shared-go` backbone

Implements design §5 (`packages/shared-go`) and the §6 bootstrap. This is the highest-leverage phase: every service depends on these modules, so they are written in full with tests. Module path: `github.com/jtumidanski/myfleet/packages/shared-go`.

### Task 1.1: Bootstrap the `shared-go` module + config loader

**Files:**
- Create: `packages/shared-go/go.mod`
- Create: `packages/shared-go/config/config.go`
- Test: `packages/shared-go/config/config_test.go`

- [ ] **Step 1: Init the module and wire into the workspace**

```bash
cd packages/shared-go
go mod init github.com/jtumidanski/myfleet/packages/shared-go
cd "$(git rev-parse --show-toplevel)"
go work use ./packages/shared-go
```

- [ ] **Step 2: Write the failing test for env-based config**

`packages/shared-go/config/config_test.go`:

```go
package config

import "testing"

func TestGet_returnsDefaultWhenUnset(t *testing.T) {
	if got := Get("MYFLEET_MISSING_KEY", "fallback"); got != "fallback" {
		t.Fatalf("want fallback, got %q", got)
	}
}

func TestMustGet_panicsWhenUnset(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic for missing required key")
		}
	}()
	MustGet("MYFLEET_REQUIRED_MISSING")
}

func TestGetInt_parsesOrDefaults(t *testing.T) {
	t.Setenv("MYFLEET_PORT", "9090")
	if got := GetInt("MYFLEET_PORT", 8080); got != 9090 {
		t.Fatalf("want 9090, got %d", got)
	}
	if got := GetInt("MYFLEET_UNSET_INT", 8080); got != 8080 {
		t.Fatalf("want default 8080, got %d", got)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./packages/shared-go/config/...`
Expected: FAIL — undefined: `Get`, `MustGet`, `GetInt`.

- [ ] **Step 4: Implement the config loader**

`packages/shared-go/config/config.go`:

```go
// Package config centralizes environment-variable access. Handlers must never
// call os.Getenv directly (design §6: "env only; no os.Getenv in handlers").
package config

import (
	"fmt"
	"os"
	"strconv"
)

func Get(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func MustGet(key string) string {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		panic(fmt.Sprintf("required env var %q is not set", key))
	}
	return v
}

func GetInt(key string, fallback int) int {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./packages/shared-go/config/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add packages/shared-go go.work
git commit -m "feat(shared-go): module init and env config loader"
```

### Task 1.2: `telemetry` — logger, tracer, correlation-ID middleware

**Files:**
- Create: `packages/shared-go/telemetry/logger.go`
- Create: `packages/shared-go/telemetry/tracer.go`
- Create: `packages/shared-go/telemetry/correlation.go`
- Test: `packages/shared-go/telemetry/correlation_test.go`

- [ ] **Step 1: Add deps**

```bash
cd packages/shared-go
go get github.com/sirupsen/logrus@latest go.opentelemetry.io/otel@latest go.opentelemetry.io/otel/sdk@latest github.com/google/uuid@latest
```

- [ ] **Step 2: Write the failing test for correlation-ID propagation**

`packages/shared-go/telemetry/correlation_test.go`:

```go
package telemetry

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCorrelationID_generatesWhenAbsentAndEchoes(t *testing.T) {
	var seen string
	h := CorrelationID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = CorrelationIDFromContext(r.Context())
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if seen == "" {
		t.Fatal("expected a generated correlation id on context")
	}
	if rec.Header().Get("X-Correlation-ID") != seen {
		t.Fatalf("response header %q != context id %q", rec.Header().Get("X-Correlation-ID"), seen)
	}
}

func TestCorrelationID_preservesInbound(t *testing.T) {
	h := CorrelationID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := CorrelationIDFromContext(r.Context()); got != "abc-123" {
			t.Fatalf("want abc-123, got %q", got)
		}
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Correlation-ID", "abc-123")
	h.ServeHTTP(httptest.NewRecorder(), req)
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./packages/shared-go/telemetry/...`
Expected: FAIL — undefined `CorrelationID`, `CorrelationIDFromContext`.

- [ ] **Step 4: Implement logger, tracer, and correlation middleware**

`packages/shared-go/telemetry/logger.go`:

```go
package telemetry

import (
	"github.com/sirupsen/logrus"
	"github.com/jtumidanski/myfleet/packages/shared-go/config"
)

// NewLogger returns a JSON structured logger; level from LOG_LEVEL.
func NewLogger() *logrus.Logger {
	l := logrus.New()
	l.SetFormatter(&logrus.JSONFormatter{})
	if lvl, err := logrus.ParseLevel(config.Get("LOG_LEVEL", "info")); err == nil {
		l.SetLevel(lvl)
	}
	return l
}
```

`packages/shared-go/telemetry/correlation.go`:

```go
package telemetry

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

type ctxKey int

const correlationKey ctxKey = iota

const headerCorrelationID = "X-Correlation-ID"

// CorrelationID middleware ensures every request carries a correlation id on
// its context and echoes it on the response (design §15).
func CorrelationID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(headerCorrelationID)
		if id == "" {
			id = uuid.NewString()
		}
		w.Header().Set(headerCorrelationID, id)
		ctx := context.WithValue(r.Context(), correlationKey, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func CorrelationIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(correlationKey).(string); ok {
		return v
	}
	return ""
}
```

`packages/shared-go/telemetry/tracer.go`:

```go
package telemetry

import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

// InitTracer wires the global tracer provider. For MVP this returns the global
// no-op-or-configured tracer; OTLP export is configured via env in deploy.
func InitTracer(service string) trace.Tracer {
	return otel.Tracer(service)
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./packages/shared-go/telemetry/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add packages/shared-go
git commit -m "feat(shared-go): telemetry logger, tracer, correlation-ID middleware"
```

### Task 1.3: `database` — GORM connect, migrations, lazy query providers, advisory lock

**Files:**
- Create: `packages/shared-go/database/database.go`
- Create: `packages/shared-go/database/query.go`
- Create: `packages/shared-go/database/lock.go`
- Test: `packages/shared-go/database/query_test.go`

- [ ] **Step 1: Add deps**

```bash
cd packages/shared-go
go get gorm.io/gorm@latest gorm.io/driver/postgres@latest
```

- [ ] **Step 2: Write the failing test for the lazy `Query`/`SliceQuery` providers**

`packages/shared-go/database/query_test.go` (pure unit test of the provider closure, no DB):

```go
package database

import (
	"errors"
	"testing"
)

func TestQuery_lazilyInvokesFetcherOnce(t *testing.T) {
	calls := 0
	p := Query(func() (string, error) { calls++; return "value", nil })
	got, err := p()
	if err != nil || got != "value" {
		t.Fatalf("want value,nil got %q,%v", got, err)
	}
	if _, _ = p(); calls != 2 {
		t.Fatalf("provider should re-invoke fetcher each call, calls=%d", calls)
	}
}

func TestSliceQuery_propagatesError(t *testing.T) {
	sentinel := errors.New("boom")
	p := SliceQuery(func() ([]int, error) { return nil, sentinel })
	if _, err := p(); !errors.Is(err, sentinel) {
		t.Fatalf("want sentinel error, got %v", err)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./packages/shared-go/database/...`
Expected: FAIL — undefined `Query`, `SliceQuery`.

- [ ] **Step 4: Implement database connect, query providers, advisory lock**

`packages/shared-go/database/query.go`:

```go
package database

// Provider is a lazy, re-runnable data fetch (design §6: "Lazy data access").
type Provider[T any] func() (T, error)

func Query[T any](fetch func() (T, error)) Provider[T]            { return fetch }
func SliceQuery[T any](fetch func() ([]T, error)) Provider[[]T]   { return fetch }
```

`packages/shared-go/database/database.go`:

```go
package database

import (
	"github.com/sirupsen/logrus"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/config"
)

type Migration func(db *gorm.DB) error

type options struct{ migrations []Migration }

type Option func(*options)

// SetMigrations registers AutoMigrate funcs run on Connect (design §6/§13).
func SetMigrations(m ...Migration) Option {
	return func(o *options) { o.migrations = append(o.migrations, m...) }
}

// Connect opens the service DB (DATABASE_URL) and runs registered migrations.
func Connect(log logrus.FieldLogger, opts ...Option) (*gorm.DB, error) {
	o := &options{}
	for _, opt := range opts {
		opt(o)
	}
	db, err := gorm.Open(postgres.Open(config.MustGet("DATABASE_URL")), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	for _, m := range o.migrations {
		if err := m(db); err != nil {
			return nil, err
		}
	}
	log.Info("database connected and migrated")
	return db, nil
}
```

`packages/shared-go/database/lock.go`:

```go
package database

import (
	"hash/fnv"

	"gorm.io/gorm"
)

// WithLeaderLock runs fn only if it acquires the named Postgres advisory lock,
// making background sweeps multi-replica-safe (design A9). If the lock is held
// elsewhere, fn is skipped and (false, nil) is returned.
func WithLeaderLock(db *gorm.DB, name string, fn func() error) (ran bool, err error) {
	h := fnv.New64a()
	_, _ = h.Write([]byte(name))
	key := int64(h.Sum64())

	var got bool
	if err = db.Raw("SELECT pg_try_advisory_lock(?)", key).Scan(&got).Error; err != nil {
		return false, err
	}
	if !got {
		return false, nil
	}
	defer db.Exec("SELECT pg_advisory_unlock(?)", key)
	return true, fn()
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./packages/shared-go/database/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add packages/shared-go
git commit -m "feat(shared-go): GORM connect, migrations, lazy providers, advisory lock"
```

### Task 1.4: `server` — HTTP bootstrap, JSON:API envelope, error mapping, pagination

**Files:**
- Create: `packages/shared-go/server/server.go`
- Create: `packages/shared-go/server/jsonapi.go`
- Create: `packages/shared-go/server/errors.go`
- Create: `packages/shared-go/server/pagination.go`
- Create: `packages/shared-go/server/handler.go`
- Test: `packages/shared-go/server/errors_test.go`
- Test: `packages/shared-go/server/pagination_test.go`

- [ ] **Step 1: Add deps**

```bash
cd packages/shared-go
go get github.com/go-chi/chi/v5@latest
```

- [ ] **Step 2: Write failing tests for error mapping + pagination parsing**

`packages/shared-go/server/errors_test.go`:

```go
package server

import "testing"

func TestStatusFor_mapsDomainErrors(t *testing.T) {
	cases := map[error]int{
		ErrUnauthorized: 401,
		ErrForbidden:    403,
		ErrNotFound:     404,
		ErrConflict:     409,
		ErrGone:         410,
		ErrValidation:   422,
	}
	for err, want := range cases {
		if got := StatusFor(err); got != want {
			t.Fatalf("StatusFor(%v)=%d want %d", err, got, want)
		}
	}
}
```

`packages/shared-go/server/pagination_test.go`:

```go
package server

import (
	"net/http/httptest"
	"testing"
)

func TestParsePage_defaultsAndClamps(t *testing.T) {
	p := ParsePage(httptest.NewRequest("GET", "/x", nil))
	if p.Number != 1 || p.Size != 25 {
		t.Fatalf("defaults want 1/25 got %d/%d", p.Number, p.Size)
	}
	p = ParsePage(httptest.NewRequest("GET", "/x?page[number]=3&page[size]=500", nil))
	if p.Number != 3 || p.Size != 100 {
		t.Fatalf("clamp want 3/100 got %d/%d", p.Number, p.Size)
	}
}

func TestPageMeta_computesTotalPages(t *testing.T) {
	m := Page{Number: 1, Size: 10}.Meta(95)
	if m.TotalPages != 10 || m.Total != 95 {
		t.Fatalf("want 10 pages/95 total got %d/%d", m.TotalPages, m.Total)
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./packages/shared-go/server/...`
Expected: FAIL — undefined symbols.

- [ ] **Step 4: Implement the server package**

`packages/shared-go/server/errors.go` (canonical mapping, design §6):

```go
package server

import "errors"

var (
	ErrUnauthorized = errors.New("unauthorized")    // 401
	ErrForbidden    = errors.New("forbidden")       // 403
	ErrNotFound     = errors.New("not found")       // 404
	ErrConflict     = errors.New("conflict")        // 409
	ErrGone         = errors.New("gone")            // 410
	ErrValidation   = errors.New("validation")      // 422
)

func StatusFor(err error) int {
	switch {
	case errors.Is(err, ErrUnauthorized):
		return 401
	case errors.Is(err, ErrForbidden):
		return 403
	case errors.Is(err, ErrNotFound):
		return 404
	case errors.Is(err, ErrConflict):
		return 409
	case errors.Is(err, ErrGone):
		return 410
	case errors.Is(err, ErrValidation):
		return 422
	default:
		return 500
	}
}

// APIError is one entry in the standard error envelope (design §6).
type APIError struct {
	Status string       `json:"status"`
	Code   string       `json:"code"`
	Title  string       `json:"title"`
	Detail string       `json:"detail,omitempty"`
	Source *ErrorSource `json:"source,omitempty"`
}

type ErrorSource struct {
	Pointer string `json:"pointer,omitempty"`
}
```

`packages/shared-go/server/pagination.go` (design A4):

```go
package server

import (
	"net/http"
	"strconv"
)

type Page struct {
	Number int
	Size   int
}

type PageMeta struct {
	Total      int `json:"total"`
	TotalPages int `json:"totalPages"`
	Number     int `json:"number"`
	Size       int `json:"size"`
}

func ParsePage(r *http.Request) Page {
	num := atoiDefault(r.URL.Query().Get("page[number]"), 1)
	if num < 1 {
		num = 1
	}
	size := atoiDefault(r.URL.Query().Get("page[size]"), 25)
	if size < 1 {
		size = 25
	}
	if size > 100 {
		size = 100
	}
	return Page{Number: num, Size: size}
}

func (p Page) Offset() int { return (p.Number - 1) * p.Size }

func (p Page) Meta(total int) PageMeta {
	pages := (total + p.Size - 1) / p.Size
	return PageMeta{Total: total, TotalPages: pages, Number: p.Number, Size: p.Size}
}

func atoiDefault(s string, d int) int {
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return d
}
```

`packages/shared-go/server/jsonapi.go`:

```go
package server

import (
	"encoding/json"
	"net/http"
)

// Resource is the JSON:API resource object (design §12, PRD §5.1).
type Resource struct {
	Type          string         `json:"type"`
	ID            string         `json:"id"`
	Attributes    any            `json:"attributes"`
	Relationships map[string]any `json:"relationships,omitempty"`
}

type Document struct {
	Data  any            `json:"data,omitempty"`
	Meta  any            `json:"meta,omitempty"`
	Links map[string]any `json:"links,omitempty"`
}

func WriteJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/vnd.api+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// WriteError renders the standard envelope for a domain/HTTP error.
func WriteError(w http.ResponseWriter, err error) {
	status := StatusFor(err)
	WriteJSON(w, status, struct {
		Errors []APIError `json:"errors"`
	}{Errors: []APIError{{
		Status: itoa(status),
		Code:   codeFor(status),
		Title:  err.Error(),
	}}})
}
```

`packages/shared-go/server/handler.go` (RegisterHandler / RegisterInputHandler[T], design §6):

```go
package server

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"
)

type Server struct {
	log    logrus.FieldLogger
	router chi.Router
	inits  []func(chi.Router)
}

func New(log logrus.FieldLogger) *Server {
	return &Server{log: log, router: chi.NewRouter()}
}

func (s *Server) Logger() logrus.FieldLogger { return s.log }

func (s *Server) Use(mw ...func(http.Handler) http.Handler) *Server {
	s.router.Use(mw...)
	return s
}

func (s *Server) AddRouteInitializer(fn func(chi.Router)) *Server {
	s.inits = append(s.inits, fn)
	return s
}

func (s *Server) Router() chi.Router {
	for _, fn := range s.inits {
		fn(s.router)
	}
	s.inits = nil
	return s.router
}

// RegisterInputHandler decodes a typed JSON:API attributes payload {data:{attributes:T}}.
func RegisterInputHandler[T any](fn func(http.ResponseWriter, *http.Request, T)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var doc struct {
			Data struct {
				Attributes T `json:"attributes"`
			} `json:"data"`
		}
		if err := json.NewDecoder(r.Body).Decode(&doc); err != nil {
			WriteError(w, ErrValidation)
			return
		}
		fn(w, r, doc.Data.Attributes)
	}
}
```

`packages/shared-go/server/server.go` (Run + helpers itoa/codeFor):

```go
package server

import (
	"net/http"
	"strconv"

	"github.com/jtumidanski/myfleet/packages/shared-go/config"
)

func itoa(n int) string { return strconv.Itoa(n) }

func codeFor(status int) string {
	switch status {
	case 401:
		return "unauthorized"
	case 403:
		return "forbidden"
	case 404:
		return "not_found"
	case 409:
		return "conflict"
	case 410:
		return "gone"
	case 422:
		return "validation_error"
	default:
		return "internal_error"
	}
}

// Run starts the HTTP server on PORT (default 8080).
func (s *Server) Run() error {
	addr := ":" + config.Get("PORT", "8080")
	s.log.Infof("listening on %s", addr)
	return http.ListenAndServe(addr, s.Router())
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./packages/shared-go/server/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add packages/shared-go
git commit -m "feat(shared-go): server bootstrap, JSON:API envelope, error mapping, pagination"
```

### Task 1.5: `health` — liveness/readiness/metrics

**Files:**
- Create: `packages/shared-go/health/health.go`
- Test: `packages/shared-go/health/health_test.go`

- [ ] **Step 1: Add dep**

```bash
cd packages/shared-go
go get github.com/prometheus/client_golang/prometheus/promhttp@latest
```

- [ ] **Step 2: Write the failing test**

`packages/shared-go/health/health_test.go`:

```go
package health

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLiveness_returns200(t *testing.T) {
	rec := httptest.NewRecorder()
	Liveness()(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 got %d", rec.Code)
	}
}

func TestReadiness_503WhenCheckFails(t *testing.T) {
	h := Readiness(func() error { return http.ErrServerClosed })
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503 got %d", rec.Code)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./packages/shared-go/health/...`
Expected: FAIL — undefined `Liveness`, `Readiness`.

- [ ] **Step 4: Implement health**

`packages/shared-go/health/health.go`:

```go
package health

import "net/http"

func Liveness() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }
}

// Readiness fails (503) if any dependency check errors (design §15: DB ping + deps).
func Readiness(checks ...func() error) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		for _, c := range checks {
			if err := c(); err != nil {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
		}
		w.WriteHeader(http.StatusOK)
	}
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./packages/shared-go/health/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add packages/shared-go
git commit -m "feat(shared-go): health liveness/readiness/metrics handlers"
```

### Task 1.6: `auth` — JWT validation middleware + JWKS cache + Identity

**Files:**
- Create: `packages/shared-go/auth/identity.go`
- Create: `packages/shared-go/auth/jwks.go`
- Create: `packages/shared-go/auth/middleware.go`
- Test: `packages/shared-go/auth/middleware_test.go`

- [ ] **Step 1: Add dep**

```bash
cd packages/shared-go
go get github.com/golang-jwt/jwt/v5@latest github.com/MicahParks/keyfunc/v3@latest
```

- [ ] **Step 2: Write the failing test (sign with a test RSA key, validate via injected keyfunc)**

`packages/shared-go/auth/middleware_test.go`:

```go
package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func signTestToken(t *testing.T, key *rsa.PrivateKey, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	s, err := tok.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestJWT_rejectsMissingToken(t *testing.T) {
	mw := jwtWithKeyfunc(func(*jwt.Token) (any, error) { return nil, nil })
	rec := httptest.NewRecorder()
	mw(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).
		ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 got %d", rec.Code)
	}
}

func TestJWT_parsesIdentityFromValidToken(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	tokenStr := signTestToken(t, key, jwt.MapClaims{
		"sub":              "user-1",
		"email":            "a@b.com",
		"active_fleet_id":  "fleet-9",
		"role":             "owner",
		"exp":              time.Now().Add(time.Hour).Unix(),
	})
	var seen Identity
	mw := jwtWithKeyfunc(func(*jwt.Token) (any, error) { return &key.PublicKey, nil })
	h := mw(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		seen = IdentityFromContext(r.Context())
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	h.ServeHTTP(httptest.NewRecorder(), req)
	if seen.UserID != "user-1" || seen.ActiveFleetID != "fleet-9" || seen.Role != "owner" {
		t.Fatalf("identity not parsed: %+v", seen)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./packages/shared-go/auth/...`
Expected: FAIL — undefined `Identity`, `IdentityFromContext`, `jwtWithKeyfunc`.

- [ ] **Step 4: Implement identity, JWKS cache, middleware**

`packages/shared-go/auth/identity.go`:

```go
package auth

import "context"

type Identity struct {
	UserID        string
	Email         string
	ActiveFleetID string
	Role          string // owner | member | viewer
}

type idCtxKey int

const identityKey idCtxKey = iota

func WithIdentity(ctx context.Context, id Identity) context.Context {
	return context.WithValue(ctx, identityKey, id)
}

func IdentityFromContext(ctx context.Context) Identity {
	if v, ok := ctx.Value(identityKey).(Identity); ok {
		return v
	}
	return Identity{}
}
```

`packages/shared-go/auth/jwks.go`:

```go
package auth

import (
	"context"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
)

// NewJWKSKeyfunc fetches+caches auth-service's JWKS (design A3).
func NewJWKSKeyfunc(ctx context.Context, jwksURL string) (jwt.Keyfunc, error) {
	k, err := keyfunc.NewDefaultCtx(ctx, []string{jwksURL})
	if err != nil {
		return nil, err
	}
	return k.Keyfunc, nil
}
```

`packages/shared-go/auth/middleware.go`:

```go
package auth

import (
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// JWT validates RS256 tokens via JWKS and puts an Identity on context (design §9).
func JWT(keyfn jwt.Keyfunc) func(http.Handler) http.Handler { return jwtWithKeyfunc(keyfn) }

func jwtWithKeyfunc(keyfn jwt.Keyfunc) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if raw == "" || raw == r.Header.Get("Authorization") {
				server.WriteError(w, server.ErrUnauthorized)
				return
			}
			claims := jwt.MapClaims{}
			tok, err := jwt.ParseWithClaims(raw, claims, keyfn, jwt.WithValidMethods([]string{"RS256"}))
			if err != nil || !tok.Valid {
				server.WriteError(w, server.ErrUnauthorized)
				return
			}
			id := Identity{
				UserID:        str(claims["sub"]),
				Email:         str(claims["email"]),
				ActiveFleetID: str(claims["active_fleet_id"]),
				Role:          str(claims["role"]),
			}
			next.ServeHTTP(w, r.WithContext(WithIdentity(r.Context(), id)))
		})
	}
}

// RequireRole returns 403 if the caller's role is not in allowed (design §9).
func RequireRole(allowed ...string) func(http.Handler) http.Handler {
	set := map[string]bool{}
	for _, a := range allowed {
		set[a] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !set[IdentityFromContext(r.Context()).Role] {
				server.WriteError(w, server.ErrForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func str(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./packages/shared-go/auth/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add packages/shared-go
git commit -m "feat(shared-go): JWT validation middleware, JWKS cache, Identity, RequireRole"
```

### Task 1.7: `events` — envelope, Kafka producer/consumer, outbox relay, idempotent-consumer helper

**Files:**
- Create: `packages/shared-go/events/envelope.go`
- Create: `packages/shared-go/events/producer.go`
- Create: `packages/shared-go/events/consumer.go`
- Create: `packages/shared-go/events/outbox.go`
- Test: `packages/shared-go/events/envelope_test.go`
- Test: `packages/shared-go/events/outbox_test.go`

- [ ] **Step 1: Add dep**

```bash
cd packages/shared-go
go get github.com/segmentio/kafka-go@latest
```

- [ ] **Step 2: Write failing tests for the envelope + a no-op producer (so domain phases can stub events, design §7)**

`packages/shared-go/events/envelope_test.go`:

```go
package events

import (
	"encoding/json"
	"testing"
)

func TestEnvelope_roundTrips(t *testing.T) {
	e := Envelope{EventID: "e1", Type: "vehicle.created", Version: 1, FleetID: "f1"}
	b, _ := json.Marshal(e)
	var got Envelope
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.Type != "vehicle.created" || got.FleetID != "f1" {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
}

func TestNoopProducer_neverErrors(t *testing.T) {
	if err := NoopProducer{}.Publish(nil, Envelope{Type: "x"}); err != nil {
		t.Fatalf("noop producer must not error: %v", err)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./packages/shared-go/events/...`
Expected: FAIL — undefined `Envelope`, `NoopProducer`.

- [ ] **Step 4: Implement envelope, Producer interface (+ NoopProducer + Kafka producer), consumer scaffold, outbox relay**

`packages/shared-go/events/envelope.go`:

```go
package events

import (
	"context"
	"time"
)

// Envelope is the canonical event shape (design §7), mirrored in dto-go.
type Envelope struct {
	EventID     string         `json:"event_id"`
	Type        string         `json:"type"`
	Version     int            `json:"version"`
	OccurredAt  time.Time      `json:"occurred_at"`
	FleetID     string         `json:"fleet_id"`
	ActorUserID string         `json:"actor_user_id"`
	TraceID     string         `json:"trace_id"`
	Data        map[string]any `json:"data"`
}

// Producer publishes events. Domain phases depend on this interface so they can
// run with NoopProducer before Kafka is wired (design §7 last paragraph).
type Producer interface {
	Publish(ctx context.Context, e Envelope) error
}

type NoopProducer struct{}

func (NoopProducer) Publish(context.Context, Envelope) error { return nil }
```

`packages/shared-go/events/producer.go`:

```go
package events

import (
	"context"
	"encoding/json"

	"github.com/segmentio/kafka-go"
)

type KafkaProducer struct{ w *kafka.Writer }

func NewKafkaProducer(brokers []string) *KafkaProducer {
	return &KafkaProducer{w: &kafka.Writer{
		Addr:                   kafka.TCP(brokers...),
		Balancer:               &kafka.Hash{},
		AllowAutoTopicCreation: true,
	}}
}

func (p *KafkaProducer) Publish(ctx context.Context, e Envelope) error {
	payload, err := json.Marshal(e)
	if err != nil {
		return err
	}
	return p.w.WriteMessages(ctx, kafka.Message{
		Topic: e.Type,
		Key:   []byte(e.FleetID),
		Value: payload,
		Headers: []kafka.Header{
			{Key: "X-Correlation-ID", Value: []byte(e.TraceID)},
			{Key: "event_id", Value: []byte(e.EventID)},
		},
	})
}

func (p *KafkaProducer) Close() error { return p.w.Close() }
```

`packages/shared-go/events/consumer.go`:

```go
package events

import (
	"context"
	"encoding/json"

	"github.com/segmentio/kafka-go"
	"github.com/sirupsen/logrus"
)

type Handler func(ctx context.Context, e Envelope) error

// Consume reads a topic with a consumer group and invokes h per message.
// Dedup/idempotency is the handler's responsibility (processed_events, design §7).
func Consume(ctx context.Context, log logrus.FieldLogger, brokers []string, group, topic string, h Handler) {
	r := kafka.NewReader(kafka.ReaderConfig{Brokers: brokers, GroupID: group, Topic: topic})
	defer r.Close()
	for {
		m, err := r.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.WithError(err).Warn("kafka fetch failed")
			continue
		}
		var e Envelope
		if err := json.Unmarshal(m.Value, &e); err != nil {
			log.WithError(err).Error("bad event payload; skipping")
			_ = r.CommitMessages(ctx, m)
			continue
		}
		if err := h(ctx, e); err != nil {
			log.WithError(err).WithField("event_id", e.EventID).Error("handler failed; will retry")
			continue // do not commit → redelivery
		}
		_ = r.CommitMessages(ctx, m)
	}
}
```

`packages/shared-go/events/outbox.go`:

```go
package events

import (
	"context"
	"encoding/json"
	"time"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// OutboxRow is the transactional-outbox table (design A8, §13).
type OutboxRow struct {
	EventID    string `gorm:"primaryKey"`
	Type       string
	Payload    []byte
	OccurredAt time.Time
	SentAt     *time.Time
}

func (OutboxRow) TableName() string { return "outbox" }

func MigrateOutbox(db *gorm.DB) error { return db.AutoMigrate(&OutboxRow{}) }

// Enqueue writes an event into the outbox in the caller's transaction.
func Enqueue(tx *gorm.DB, e Envelope) error {
	payload, err := json.Marshal(e)
	if err != nil {
		return err
	}
	return tx.Create(&OutboxRow{EventID: e.EventID, Type: e.Type, Payload: payload, OccurredAt: e.OccurredAt}).Error
}

// RelayOnce publishes all unsent outbox rows and marks them sent. Run under the
// advisory lock (design A8/A9).
func RelayOnce(ctx context.Context, log logrus.FieldLogger, db *gorm.DB, p Producer) error {
	var rows []OutboxRow
	if err := db.Where("sent_at IS NULL").Order("occurred_at").Limit(100).Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		var e Envelope
		if err := json.Unmarshal(row.Payload, &e); err != nil {
			log.WithError(err).Error("corrupt outbox payload")
			continue
		}
		if err := p.Publish(ctx, e); err != nil {
			return err
		}
		now := time.Now()
		if err := db.Model(&OutboxRow{}).Where("event_id = ?", row.EventID).Update("sent_at", &now).Error; err != nil {
			return err
		}
	}
	return nil
}
```

`packages/shared-go/events/outbox_test.go` (uses sqlite in-memory via gorm for a real relay test):

```go
package events

import (
	"context"
	"testing"

	"github.com/sirupsen/logrus"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type capture struct{ published []Envelope }

func (c *capture) Publish(_ context.Context, e Envelope) error {
	c.published = append(c.published, e)
	return nil
}

func TestRelayOnce_publishesAndMarksSent(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := MigrateOutbox(db); err != nil {
		t.Fatal(err)
	}
	if err := Enqueue(db, Envelope{EventID: "e1", Type: "vehicle.created", FleetID: "f1"}); err != nil {
		t.Fatal(err)
	}
	cap := &capture{}
	if err := RelayOnce(context.Background(), logrus.New(), db, cap); err != nil {
		t.Fatal(err)
	}
	if len(cap.published) != 1 || cap.published[0].EventID != "e1" {
		t.Fatalf("want 1 published e1, got %+v", cap.published)
	}
	var unsent int64
	db.Model(&OutboxRow{}).Where("sent_at IS NULL").Count(&unsent)
	if unsent != 0 {
		t.Fatalf("want 0 unsent, got %d", unsent)
	}
}
```

> Add the sqlite test dep: `go get gorm.io/driver/sqlite@latest` (test-only).

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./packages/shared-go/events/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add packages/shared-go
git commit -m "feat(shared-go): event envelope, Kafka producer/consumer, transactional outbox relay"
```

### Task 1.8: `jobs` — advisory-locked scheduler primitive

**Files:**
- Create: `packages/shared-go/jobs/scheduler.go`
- Test: `packages/shared-go/jobs/scheduler_test.go`

- [ ] **Step 1: Write the failing test (ticker fires fn; cancel stops)**

`packages/shared-go/jobs/scheduler_test.go`:

```go
package jobs

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestEvery_invokesFnAndStopsOnCancel(t *testing.T) {
	var n int32
	ctx, cancel := context.WithCancel(context.Background())
	go Every(ctx, 10*time.Millisecond, func(context.Context) error {
		atomic.AddInt32(&n, 1)
		return nil
	})
	time.Sleep(55 * time.Millisecond)
	cancel()
	if atomic.LoadInt32(&n) < 3 {
		t.Fatalf("expected >=3 invocations, got %d", n)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./packages/shared-go/jobs/...`
Expected: FAIL — undefined `Every`.

- [ ] **Step 3: Implement the scheduler**

`packages/shared-go/jobs/scheduler.go`:

```go
// Package jobs provides a ticker primitive for background sweeps. Run the fn
// body under database.WithLeaderLock for multi-replica safety (design A9).
package jobs

import (
	"context"
	"time"
)

func Every(ctx context.Context, interval time.Duration, fn func(context.Context) error) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			_ = fn(ctx)
		}
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./packages/shared-go/jobs/...`
Expected: PASS.

- [ ] **Step 5: Final shared-go gate + commit**

Run: `go build ./packages/shared-go/... && go vet ./packages/shared-go/... && go test -race ./packages/shared-go/...`
Expected: all PASS.

```bash
git add packages/shared-go
git commit -m "feat(shared-go): advisory-locked job scheduler primitive"
```

---

## Phase 2 — `dto-go`, `shared-ts`, `ui-components` skeletons

Implements design §5 shared transport/UI packages. Thin now; each domain phase adds its DTOs/types.

### Task 2.1: `dto-go` module + event-data DTOs + JSON:API helpers

**Files:**
- Create: `packages/dto-go/go.mod`
- Create: `packages/dto-go/events/payloads.go`
- Test: `packages/dto-go/events/payloads_test.go`

- [ ] **Step 1: Init module + workspace**

```bash
cd packages/dto-go && go mod init github.com/jtumidanski/myfleet/packages/dto-go
cd "$(git rev-parse --show-toplevel)" && go work use ./packages/dto-go
```

- [ ] **Step 2: Write the failing test**

`packages/dto-go/events/payloads_test.go`:

```go
package events

import (
	"encoding/json"
	"testing"
)

func TestVehicleCreatedData_jsonTags(t *testing.T) {
	b, _ := json.Marshal(VehicleCreatedData{VehicleID: "v1", FleetID: "f1"})
	if string(b) != `{"vehicle_id":"v1","fleet_id":"f1"}` {
		t.Fatalf("unexpected json: %s", b)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./packages/dto-go/...`
Expected: FAIL — undefined `VehicleCreatedData`.

- [ ] **Step 4: Implement the shared event-data DTOs (design §7)**

`packages/dto-go/events/payloads.go`:

```go
// Package events holds the shared `data` payload shapes for each event type
// (design §7). Consumers import these instead of a producer's internal/ package.
package events

type VehicleCreatedData struct {
	VehicleID string `json:"vehicle_id"`
	FleetID   string `json:"fleet_id"`
}

type MaintenanceCompletedData struct {
	ScheduleID         string `json:"schedule_id"`
	VehicleID          string `json:"vehicle_id"`
	MaintenanceRecord  string `json:"maintenance_record_id"`
	CategoryID         string `json:"category_id"`
}

type FuelLoggedData struct {
	FuelLogID string  `json:"fuel_log_id"`
	VehicleID string  `json:"vehicle_id"`
	Mileage   int     `json:"mileage"`
	TotalCost float64 `json:"total_cost"`
}

type ScheduleOverdueData struct {
	ScheduleID string `json:"schedule_id"`
	VehicleID  string `json:"vehicle_id"`
	Severity   string `json:"severity"`
}

type MemberInvitedData struct {
	InviteID string `json:"invite_id"`
	Email    string `json:"email"`
	Role     string `json:"role"`
}

type MediaUploadedData struct {
	MediaID     string `json:"media_id"`
	ContentType string `json:"content_type"`
}
```

- [ ] **Step 5: Run test to verify it passes & commit**

Run: `go test ./packages/dto-go/...`
Expected: PASS.

```bash
git add packages/dto-go go.work
git commit -m "feat(dto-go): module init and shared event payload DTOs"
```

### Task 2.2: `shared-ts` — apiClient, errors, jsonapi types

**Files:**
- Create: `packages/shared-ts/package.json`
- Create: `packages/shared-ts/tsconfig.json`
- Create: `packages/shared-ts/src/jsonapi.ts`
- Create: `packages/shared-ts/src/errors.ts`
- Create: `packages/shared-ts/src/apiClient.ts`
- Create: `packages/shared-ts/src/index.ts`
- Test: `packages/shared-ts/src/errors.test.ts`

- [ ] **Step 1: Create package manifest + tsconfig**

`packages/shared-ts/package.json`:

```json
{
  "name": "@myfleet/shared-ts",
  "version": "0.0.0",
  "type": "module",
  "main": "src/index.ts",
  "scripts": {
    "build": "tsc -p tsconfig.json --noEmit",
    "test": "vitest run"
  },
  "devDependencies": {
    "typescript": "^5.5.0",
    "vitest": "^2.0.0"
  }
}
```

`packages/shared-ts/tsconfig.json`:

```json
{
  "compilerOptions": {
    "target": "ES2022",
    "module": "ESNext",
    "moduleResolution": "Bundler",
    "strict": true,
    "noUncheckedIndexedAccess": true,
    "skipLibCheck": true,
    "types": ["vitest/globals"]
  },
  "include": ["src"]
}
```

- [ ] **Step 2: Write the failing test**

`packages/shared-ts/src/errors.test.ts`:

```ts
import { describe, it, expect } from 'vitest';
import { createErrorFromUnknown, ApiError } from './errors';

describe('createErrorFromUnknown', () => {
  it('maps a JSON:API error envelope to ApiError', () => {
    const err = createErrorFromUnknown({
      status: 422,
      body: { errors: [{ status: '422', code: 'validation_error', title: 'bad', detail: 'x' }] },
    });
    expect(err).toBeInstanceOf(ApiError);
    expect(err.status).toBe(422);
    expect(err.code).toBe('validation_error');
  });

  it('falls back for non-envelope errors', () => {
    const err = createErrorFromUnknown(new Error('boom'));
    expect(err.status).toBe(0);
    expect(err.message).toContain('boom');
  });
});
```

- [ ] **Step 3: Install + run test to verify it fails**

```bash
npm install
npm run -w @myfleet/shared-ts test
```
Expected: FAIL — cannot find `./errors`.

- [ ] **Step 4: Implement jsonapi types, errors, apiClient**

`packages/shared-ts/src/jsonapi.ts`:

```ts
export interface JsonApiResource<A, R = Record<string, unknown>> {
  type: string;
  id: string;
  attributes: A;
  relationships?: R;
}

export interface PageMeta {
  total: number;
  totalPages: number;
  number: number;
  size: number;
}

export interface JsonApiDocument<T> {
  data: T;
  meta?: PageMeta;
  links?: Record<string, string>;
}

export interface JsonApiError {
  status: string;
  code: string;
  title: string;
  detail?: string;
  source?: { pointer?: string };
}
```

`packages/shared-ts/src/errors.ts`:

```ts
import type { JsonApiError } from './jsonapi';

export class ApiError extends Error {
  status: number;
  code: string;
  detail?: string;
  pointer?: string;
  constructor(status: number, code: string, message: string, detail?: string, pointer?: string) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.code = code;
    this.detail = detail;
    this.pointer = pointer;
  }
}

interface EnvelopeShape {
  status?: number;
  body?: { errors?: JsonApiError[] };
}

export function createErrorFromUnknown(e: unknown): ApiError {
  const env = e as EnvelopeShape;
  const first = env?.body?.errors?.[0];
  if (first) {
    return new ApiError(env.status ?? Number(first.status) ?? 0, first.code, first.title, first.detail, first.source?.pointer);
  }
  if (e instanceof Error) return new ApiError(0, 'unknown', e.message);
  return new ApiError(0, 'unknown', 'Unknown error');
}
```

`packages/shared-ts/src/apiClient.ts`:

```ts
import { createErrorFromUnknown } from './errors';

export interface ApiClientOptions {
  baseUrl: string;
  getAccessToken: () => string | null;
  onRefresh: () => Promise<string | null>;
}

export class ApiClient {
  constructor(private opts: ApiClientOptions) {}

  async request<T>(path: string, init: RequestInit = {}, retried = false): Promise<T> {
    const token = this.opts.getAccessToken();
    const res = await fetch(this.opts.baseUrl + path, {
      ...init,
      headers: {
        'Content-Type': 'application/vnd.api+json',
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
        ...(init.headers ?? {}),
      },
    });
    if (res.status === 401 && !retried) {
      const refreshed = await this.opts.onRefresh();
      if (refreshed) return this.request<T>(path, init, true);
    }
    const body = res.status === 204 ? null : await res.json().catch(() => null);
    if (!res.ok) throw createErrorFromUnknown({ status: res.status, body });
    return body as T;
  }
}
```

`packages/shared-ts/src/index.ts`:

```ts
export * from './jsonapi';
export * from './errors';
export * from './apiClient';
```

- [ ] **Step 5: Run test + build to verify pass & commit**

```bash
npm run -w @myfleet/shared-ts test && npm run -w @myfleet/shared-ts build
```
Expected: PASS, no type errors.

```bash
git add packages/shared-ts package-lock.json
git commit -m "feat(shared-ts): apiClient, error mapping, JSON:API types"
```

### Task 2.3: `ui-components` skeleton

**Files:**
- Create: `packages/ui-components/package.json`
- Create: `packages/ui-components/tsconfig.json`
- Create: `packages/ui-components/src/StatusBadge.tsx`
- Create: `packages/ui-components/src/formatters.ts`
- Create: `packages/ui-components/src/index.ts`
- Test: `packages/ui-components/src/formatters.test.ts`

- [ ] **Step 1: Manifest + tsconfig** (mirror shared-ts; add `react`, `@testing-library/react`, `jsdom` devDeps; set `"test": "vitest run --environment jsdom"`).

- [ ] **Step 2: Write the failing test**

`packages/ui-components/src/formatters.test.ts`:

```ts
import { describe, it, expect } from 'vitest';
import { formatMoney, formatMileage } from './formatters';

describe('formatters', () => {
  it('formats money in USD', () => {
    expect(formatMoney(1234.5)).toBe('$1,234.50');
  });
  it('formats mileage with thousands + unit', () => {
    expect(formatMileage(12345)).toBe('12,345 mi');
  });
});
```

- [ ] **Step 3: Run test to verify it fails**

Run: `npm run -w @myfleet/ui-components test`
Expected: FAIL — cannot find `./formatters`.

- [ ] **Step 4: Implement formatters + a StatusBadge** (status → variant color per design §10.2 statuses: Healthy/Upcoming Maintenance/Overdue/Inactive).

`packages/ui-components/src/formatters.ts`:

```ts
export function formatMoney(n: number): string {
  return new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD' }).format(n);
}

export function formatMileage(n: number): string {
  return `${new Intl.NumberFormat('en-US').format(n)} mi`;
}
```

`packages/ui-components/src/StatusBadge.tsx`:

```tsx
export type VehicleStatus = 'Healthy' | 'Upcoming Maintenance' | 'Overdue' | 'Inactive';

const VARIANT: Record<VehicleStatus, string> = {
  Healthy: 'bg-green-100 text-green-800',
  'Upcoming Maintenance': 'bg-amber-100 text-amber-800',
  Overdue: 'bg-red-100 text-red-800',
  Inactive: 'bg-gray-100 text-gray-700',
};

export function StatusBadge({ status }: { status: VehicleStatus }) {
  return <span className={`inline-flex rounded px-2 py-0.5 text-xs font-medium ${VARIANT[status]}`}>{status}</span>;
}
```

`packages/ui-components/src/index.ts`:

```ts
export * from './formatters';
export * from './StatusBadge';
```

- [ ] **Step 5: Run test + build, commit**

Run: `npm run -w @myfleet/ui-components test && npm run -w @myfleet/ui-components build`
Expected: PASS.

```bash
git add packages/ui-components package-lock.json
git commit -m "feat(ui-components): status badge, formatters, package skeleton"
```

---

## Phase 3 — Local infra: docker-compose + Traefik

Implements design §2 topology and §17 local stack. Brings up PostgreSQL, MinIO, Kafka (Redpanda), and Traefik so services can run end-to-end. Service images are added as each service lands; this phase wires the backing infra + Traefik and is verified by `docker compose config`.

### Task 3.1: docker-compose backing services + Traefik + env template

**Files:**
- Create: `deploy/compose/docker-compose.yml`
- Create: `deploy/compose/traefik/traefik.yml`
- Create: `deploy/compose/traefik/dynamic.yml`
- Create: `deploy/compose/.env.example`
- Create: `scripts/dev-up.sh`

- [ ] **Step 1: Write `.env.example`** (single source for compose + services):

```bash
# deploy/compose/.env.example
POSTGRES_USER=myfleet
POSTGRES_PASSWORD=myfleet
POSTGRES_DB=myfleet
MINIO_ROOT_USER=minioadmin
MINIO_ROOT_PASSWORD=minioadmin
KAFKA_BROKERS=redpanda:9092
# Google OIDC (fill locally; never commit real values)
GOOGLE_CLIENT_ID=
GOOGLE_CLIENT_SECRET=
GOOGLE_REDIRECT_URL=http://localhost/api/auth/callback
# JWT
JWT_ISSUER=myfleet-auth
JWT_AUDIENCE=myfleet
```

- [ ] **Step 2: Write Traefik static + dynamic config (TLS/CORS/routing only, design D4/A2)**

`deploy/compose/traefik/traefik.yml`:

```yaml
entryPoints:
  web:
    address: ":80"
providers:
  file:
    filename: /etc/traefik/dynamic.yml
    watch: true
  docker:
    exposedByDefault: false
api:
  dashboard: true
  insecure: true
```

`deploy/compose/traefik/dynamic.yml` (path-prefix routing per design §2; CORS headers middleware):

```yaml
http:
  middlewares:
    cors:
      headers:
        accessControlAllowMethods: [GET, POST, PATCH, PUT, DELETE, OPTIONS]
        accessControlAllowHeaders: ["*"]
        accessControlAllowOriginList: ["http://localhost"]
        accessControlMaxAge: 100
```

- [ ] **Step 3: Write `docker-compose.yml`** (Postgres with per-service schemas created on init, MinIO, Redpanda, Traefik):

`deploy/compose/docker-compose.yml`:

```yaml
name: myfleet
services:
  traefik:
    image: traefik:v3.1
    command: --configFile=/etc/traefik/traefik.yml
    ports: ["80:80", "8081:8080"]
    volumes:
      - ./traefik:/etc/traefik:ro
      - /var/run/docker.sock:/var/run/docker.sock:ro

  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: ${POSTGRES_USER}
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}
      POSTGRES_DB: ${POSTGRES_DB}
    ports: ["5432:5432"]
    volumes:
      - ./data/pg:/var/lib/postgresql/data
      - ./init-schemas.sql:/docker-entrypoint-initdb.d/init-schemas.sql:ro
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U ${POSTGRES_USER}"]
      interval: 5s
      timeout: 3s
      retries: 10

  minio:
    image: minio/minio:latest
    command: server /data --console-address ":9001"
    environment:
      MINIO_ROOT_USER: ${MINIO_ROOT_USER}
      MINIO_ROOT_PASSWORD: ${MINIO_ROOT_PASSWORD}
    ports: ["9000:9000", "9001:9001"]
    volumes:
      - ./data/minio:/data

  redpanda:
    image: redpandadata/redpanda:latest
    command:
      - redpanda start
      - --smp 1
      - --overprovisioned
      - --kafka-addr PLAINTEXT://0.0.0.0:9092
      - --advertise-kafka-addr PLAINTEXT://redpanda:9092
    ports: ["9092:9092"]
```

`deploy/compose/init-schemas.sql` (design D2 isolated schema per service):

```sql
CREATE SCHEMA IF NOT EXISTS auth;
CREATE SCHEMA IF NOT EXISTS fleet;
CREATE SCHEMA IF NOT EXISTS media;
CREATE SCHEMA IF NOT EXISTS notification;
```

> Note this file in Task 3.1 Files list: also Create `deploy/compose/init-schemas.sql`.

- [ ] **Step 4: Write `scripts/dev-up.sh`**

```bash
#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../deploy/compose"
[ -f .env ] || cp .env.example .env
docker compose --env-file .env up -d --build
```
Then `chmod +x scripts/dev-up.sh`.

- [ ] **Step 5: Verify compose config is valid**

Run: `cd deploy/compose && cp .env.example .env && docker compose config >/dev/null && echo OK`
Expected: `OK` (config parses; no service errors).

- [ ] **Step 6: Commit**

```bash
git add deploy/compose scripts/dev-up.sh
git commit -m "feat(deploy): docker-compose infra (postgres/minio/redpanda) + traefik"
```

---

## Phase 4 — `auth-service`

Implements design §8.1 and §9 (token minting side). Establishes the **canonical Go service layout** (`cmd/main.go` + `internal/<domain>/` with the 8-file template from design §6). Module: `github.com/jtumidanski/myfleet/apps/auth-service`. Path prefix `/api/auth/*`. Owns `users`, `refresh_tokens`. **Re-read `backend-dev-guidelines` at the start of this phase** — it governs the model/entity/builder/processor/provider/administrator/resource/rest split used here and in every later backend phase.

### Task 4.1: Module bootstrap + RS256 keypair + JWKS endpoint

**Files:**
- Create: `apps/auth-service/go.mod`
- Create: `apps/auth-service/internal/jwks/keys.go`
- Create: `apps/auth-service/internal/jwks/resource.go`
- Test: `apps/auth-service/internal/jwks/keys_test.go`

- [ ] **Step 1: Init module + workspace + deps**

```bash
cd apps/auth-service && go mod init github.com/jtumidanski/myfleet/apps/auth-service
go get github.com/jtumidanski/myfleet/packages/shared-go github.com/golang-jwt/jwt/v5@latest github.com/go-chi/chi/v5@latest
cd "$(git rev-parse --show-toplevel)" && go work use ./apps/auth-service
```

- [ ] **Step 2: Write the failing test — load an RSA key and expose it as a JWKS document**

`apps/auth-service/internal/jwks/keys_test.go`:

```go
package jwks

import (
	"crypto/rand"
	"crypto/rsa"
	"testing"
)

func TestJWKS_exposesPublicKey(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	ks := NewKeySet(priv, "kid-1")
	doc := ks.PublicJWKS()
	if len(doc.Keys) != 1 {
		t.Fatalf("want 1 key, got %d", len(doc.Keys))
	}
	if doc.Keys[0].Kid != "kid-1" || doc.Keys[0].Kty != "RSA" || doc.Keys[0].Use != "sig" {
		t.Fatalf("unexpected jwk: %+v", doc.Keys[0])
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./apps/auth-service/internal/jwks/...`
Expected: FAIL — undefined `NewKeySet`.

- [ ] **Step 4: Implement the key set + JWKS document**

`apps/auth-service/internal/jwks/keys.go`:

```go
// Package jwks holds the RS256 signing key and serves the public JWKS (design A3).
package jwks

import (
	"crypto/rsa"
	"encoding/base64"
	"math/big"
)

type JWK struct {
	Kty string `json:"kty"`
	Use string `json:"use"`
	Kid string `json:"kid"`
	Alg string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
}

type JWKSDocument struct {
	Keys []JWK `json:"keys"`
}

type KeySet struct {
	priv *rsa.PrivateKey
	kid  string
}

func NewKeySet(priv *rsa.PrivateKey, kid string) *KeySet { return &KeySet{priv: priv, kid: kid} }

func (k *KeySet) Private() *rsa.PrivateKey { return k.priv }
func (k *KeySet) Kid() string              { return k.kid }

func (k *KeySet) PublicJWKS() JWKSDocument {
	pub := k.priv.Public().(*rsa.PublicKey)
	return JWKSDocument{Keys: []JWK{{
		Kty: "RSA", Use: "sig", Kid: k.kid, Alg: "RS256",
		N: base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
		E: base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
	}}}
}
```

`apps/auth-service/internal/jwks/resource.go`:

```go
package jwks

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// InitializeRoutes serves GET /.well-known/jwks.json (public; no JWT required).
func InitializeRoutes(ks *KeySet) func(chi.Router) {
	return func(r chi.Router) {
		r.Get("/.well-known/jwks.json", func(w http.ResponseWriter, _ *http.Request) {
			server.WriteJSON(w, http.StatusOK, ks.PublicJWKS())
		})
	}
}
```

> **Key loading:** the private key is loaded from `JWT_PRIVATE_KEY_PEM` (env) at startup; for local dev, `scripts/gen-jwt-key.sh` writes a PEM into `.env`. Add that script in Step 6.

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./apps/auth-service/internal/jwks/...`
Expected: PASS.

- [ ] **Step 6: Add key-gen helper + commit**

`scripts/gen-jwt-key.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail
openssl genrsa 2048 2>/dev/null
```
`chmod +x scripts/gen-jwt-key.sh`. Document in README: `JWT_PRIVATE_KEY_PEM="$(scripts/gen-jwt-key.sh)"`.

```bash
git add apps/auth-service scripts/gen-jwt-key.sh go.work
git commit -m "feat(auth): module bootstrap, RS256 key set, JWKS endpoint"
```

### Task 4.2: `user` domain — the canonical 8-file template (provisioning by google_sub)

This task demonstrates the full layered domain template (design §6). **Every later backend domain mirrors this file set.**

**Files:**
- Create: `apps/auth-service/internal/user/model.go`
- Create: `apps/auth-service/internal/user/entity.go`
- Create: `apps/auth-service/internal/user/builder.go`
- Create: `apps/auth-service/internal/user/provider.go`
- Create: `apps/auth-service/internal/user/processor.go`
- Create: `apps/auth-service/internal/user/administrator.go`
- Create: `apps/auth-service/internal/user/rest.go`
- Create: `apps/auth-service/internal/user/resource.go`
- Test: `apps/auth-service/internal/user/processor_test.go`

- [ ] **Step 1: Write the failing test — provisioning is idempotent on google_sub (FR-AUTH-2)**

`apps/auth-service/internal/user/processor_test.go`:

```go
package user

import (
	"testing"

	"github.com/sirupsen/logrus"
)

type fakeProvider struct{ byID map[string]Model; bySub map[string]Model }

func (f *fakeProvider) GetBySub(sub string) (Model, error) {
	if m, ok := f.bySub[sub]; ok {
		return m, nil
	}
	return Model{}, ErrNotFound
}

type fakeWriter struct{ created, updated int }

func (f *fakeWriter) Insert(m Model) (Model, error) { f.created++; return m, nil }
func (f *fakeWriter) Update(m Model) (Model, error) { f.updated++; return m, nil }

func TestProvisionFromGoogle_insertsWhenNew(t *testing.T) {
	p := NewProcessor(logrus.New(), &fakeProvider{bySub: map[string]Model{}})
	w := &fakeWriter{}
	got, err := p.ProvisionFromGoogle(w, GoogleProfile{Sub: "g1", Email: "a@b.com", Name: "A"})
	if err != nil {
		t.Fatal(err)
	}
	if w.created != 1 || got.GoogleSub() != "g1" {
		t.Fatalf("expected new user inserted; created=%d", w.created)
	}
}

func TestProvisionFromGoogle_updatesLoginWhenExisting(t *testing.T) {
	existing := NewBuilder().SetGoogleSub("g1").SetEmail("a@b.com").Build()
	p := NewProcessor(logrus.New(), &fakeProvider{bySub: map[string]Model{"g1": existing}})
	w := &fakeWriter{}
	if _, err := p.ProvisionFromGoogle(w, GoogleProfile{Sub: "g1", Email: "a@b.com"}); err != nil {
		t.Fatal(err)
	}
	if w.updated != 1 || w.created != 0 {
		t.Fatalf("expected update only; created=%d updated=%d", w.created, w.updated)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./apps/auth-service/internal/user/...`
Expected: FAIL — undefined symbols.

- [ ] **Step 3: Implement the immutable model**

`apps/auth-service/internal/user/model.go`:

```go
package user

import (
	"errors"
	"time"
)

var ErrNotFound = errors.New("user not found")

// Model is immutable; state changes return new instances (design §6).
type Model struct {
	id          string
	googleSub   string
	email       string
	displayName string
	avatarURL   string
	lastLoginAt time.Time
}

func (m Model) ID() string          { return m.id }
func (m Model) GoogleSub() string   { return m.googleSub }
func (m Model) Email() string       { return m.email }
func (m Model) DisplayName() string { return m.displayName }
func (m Model) AvatarURL() string   { return m.avatarURL }

// WithLogin returns a copy with login metadata refreshed.
func (m Model) WithLogin(name, avatar string, at time.Time) Model {
	m.displayName, m.avatarURL, m.lastLoginAt = name, avatar, at
	return m
}

type GoogleProfile struct {
	Sub    string
	Email  string
	Name   string
	Avatar string
}
```

- [ ] **Step 4: Implement entity + migration**

`apps/auth-service/internal/user/entity.go`:

```go
package user

import (
	"time"

	"gorm.io/gorm"
)

type Entity struct {
	ID          string `gorm:"type:uuid;primaryKey"`
	GoogleSub   string `gorm:"uniqueIndex;not null"`
	Email       string `gorm:"uniqueIndex;not null"`
	DisplayName string
	AvatarURL   string
	LastLoginAt time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (Entity) TableName() string { return "auth.users" }

func Migration(db *gorm.DB) error { return db.AutoMigrate(&Entity{}) }

func Make(e Entity) Model {
	return Model{id: e.ID, googleSub: e.GoogleSub, email: e.Email, displayName: e.DisplayName, avatarURL: e.AvatarURL, lastLoginAt: e.LastLoginAt}
}

func (m Model) ToEntity() Entity {
	return Entity{ID: m.id, GoogleSub: m.googleSub, Email: m.email, DisplayName: m.displayName, AvatarURL: m.avatarURL, LastLoginAt: m.lastLoginAt}
}
```

- [ ] **Step 5: Implement builder**

`apps/auth-service/internal/user/builder.go`:

```go
package user

import "github.com/google/uuid"

type Builder struct{ m Model }

func NewBuilder() *Builder { return &Builder{m: Model{id: uuid.NewString()}} }

func (b *Builder) SetGoogleSub(s string) *Builder   { b.m.googleSub = s; return b }
func (b *Builder) SetEmail(e string) *Builder        { b.m.email = e; return b }
func (b *Builder) SetDisplayName(n string) *Builder  { b.m.displayName = n; return b }
func (b *Builder) SetAvatarURL(a string) *Builder    { b.m.avatarURL = a; return b }
func (b *Builder) Build() Model                       { return b.m }
```

- [ ] **Step 6: Implement provider + administrator interfaces, processor**

`apps/auth-service/internal/user/provider.go`:

```go
package user

import (
	"errors"

	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/database"
)

type Provider interface{ GetBySub(sub string) (Model, error) }

type Writer interface {
	Insert(Model) (Model, error)
	Update(Model) (Model, error)
}

type dbStore struct{ db *gorm.DB }

func NewStore(db *gorm.DB) *dbStore { return &dbStore{db: db} }

func (s *dbStore) GetBySub(sub string) (Model, error) {
	return database.Query(func() (Model, error) {
		var e Entity
		if err := s.db.Where("google_sub = ?", sub).First(&e).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return Model{}, ErrNotFound
			}
			return Model{}, err
		}
		return Make(e), nil
	})()
}

func (s *dbStore) Insert(m Model) (Model, error) {
	e := m.ToEntity()
	if err := s.db.Create(&e).Error; err != nil {
		return Model{}, err
	}
	return Make(e), nil
}

func (s *dbStore) Update(m Model) (Model, error) {
	e := m.ToEntity()
	if err := s.db.Save(&e).Error; err != nil {
		return Model{}, err
	}
	return Make(e), nil
}
```

`apps/auth-service/internal/user/processor.go`:

```go
package user

import (
	"errors"
	"time"

	"github.com/sirupsen/logrus"
)

type Processor struct {
	log logrus.FieldLogger
	p   Provider
}

func NewProcessor(log logrus.FieldLogger, p Provider) *Processor { return &Processor{log: log, p: p} }

// ProvisionFromGoogle upserts a user by google_sub (FR-AUTH-2). Idempotent.
func (pr *Processor) ProvisionFromGoogle(w Writer, gp GoogleProfile) (Model, error) {
	existing, err := pr.p.GetBySub(gp.Sub)
	if errors.Is(err, ErrNotFound) {
		m := NewBuilder().SetGoogleSub(gp.Sub).SetEmail(gp.Email).SetDisplayName(gp.Name).SetAvatarURL(gp.Avatar).Build()
		m = m.WithLogin(gp.Name, gp.Avatar, time.Now())
		return w.Insert(m)
	}
	if err != nil {
		return Model{}, err
	}
	return w.Update(existing.WithLogin(gp.Name, gp.Avatar, time.Now()))
}
```

- [ ] **Step 7: Implement rest.go + resource.go (`/auth/me`)**

`apps/auth-service/internal/user/rest.go`:

```go
package user

import "github.com/jtumidanski/myfleet/packages/shared-go/server"

type Attributes struct {
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
	AvatarURL   string `json:"avatarUrl"`
}

func Transform(m Model) server.Resource {
	return server.Resource{Type: "users", ID: m.ID(), Attributes: Attributes{Email: m.Email(), DisplayName: m.DisplayName(), AvatarURL: m.AvatarURL()}}
}
```

`apps/auth-service/internal/user/resource.go`:

```go
package user

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/auth"
	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// InitializeRoutes wires GET /auth/me (design §8.1, FR-AUTH-3). Active fleet/role
// are read from the validated token's Identity; profile from the DB.
func InitializeRoutes(db *gorm.DB) func(chi.Router) {
	store := NewStore(db)
	return func(r chi.Router) {
		r.Get("/auth/me", func(w http.ResponseWriter, req *http.Request) {
			id := auth.IdentityFromContext(req.Context())
			m, err := store.GetBySub(id.UserID) // sub == user id in our tokens
			if err != nil {
				server.WriteError(w, server.ErrNotFound)
				return
			}
			server.WriteJSON(w, http.StatusOK, server.Document{
				Data: Transform(m),
				Meta: map[string]any{"activeFleetId": id.ActiveFleetID, "role": id.Role},
			})
		})
	}
}
```

- [ ] **Step 8: Run tests, gate, commit**

Run: `go test -race ./apps/auth-service/internal/user/... && go vet ./apps/auth-service/...`
Expected: PASS.

```bash
git add apps/auth-service
git commit -m "feat(auth): user domain (canonical layered template), provisioning, /auth/me"
```

### Task 4.3: `session` domain — JWT mint + rotating refresh tokens (hashed)

**Files:**
- Create: `apps/auth-service/internal/session/{model,entity,builder,provider,processor,administrator,rest,resource}.go`
- Test: `apps/auth-service/internal/session/processor_test.go`

**Behavior (design §8.1, A3):** mint RS256 access token (~15 min) with claims `sub,email,active_fleet_id,role,iss,aud,exp`; issue a rotating, single-use refresh token stored **hashed** (sha256), 30-day expiry; `POST /auth/refresh` validates+rotates and re-resolves active membership; reuse of a consumed refresh token revokes the family.

- [ ] **Step 1: Write the failing processor test for claim contents + refresh hashing**

`apps/auth-service/internal/session/processor_test.go`:

```go
package session

import (
	"crypto/rand"
	"crypto/rsa"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/sirupsen/logrus"

	"github.com/jtumidanski/myfleet/apps/auth-service/internal/jwks"
)

func TestMintAccess_setsRequiredClaims(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	ks := jwks.NewKeySet(priv, "kid-1")
	p := NewProcessor(logrus.New(), ks, "myfleet-auth", "myfleet")
	tokenStr, err := p.MintAccess(Principal{UserID: "u1", Email: "a@b.com", ActiveFleetID: "f1", Role: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	claims := jwt.MapClaims{}
	_, err = jwt.ParseWithClaims(tokenStr, claims, func(*jwt.Token) (any, error) { return &priv.PublicKey, nil })
	if err != nil {
		t.Fatal(err)
	}
	for k, want := range map[string]string{"sub": "u1", "email": "a@b.com", "active_fleet_id": "f1", "role": "owner", "iss": "myfleet-auth"} {
		if claims[k] != want {
			t.Fatalf("claim %s = %v, want %s", k, claims[k], want)
		}
	}
}

func TestHashRefresh_isStableAndNotPlaintext(t *testing.T) {
	h := HashRefresh("secret-token")
	if h == "secret-token" || h != HashRefresh("secret-token") {
		t.Fatal("refresh token must be hashed deterministically and never stored plaintext")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./apps/auth-service/internal/session/...`
Expected: FAIL — undefined `NewProcessor`, `Principal`, `MintAccess`, `HashRefresh`.

- [ ] **Step 3: Implement session processor (mint + hash) and refresh-token entity/store**

`apps/auth-service/internal/session/processor.go`:

```go
package session

import (
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/sirupsen/logrus"

	"github.com/jtumidanski/myfleet/apps/auth-service/internal/jwks"
)

const accessTTL = 15 * time.Minute

type Principal struct {
	UserID        string
	Email         string
	ActiveFleetID string
	Role          string
}

type Processor struct {
	log    logrus.FieldLogger
	ks     *jwks.KeySet
	issuer string
	aud    string
}

func NewProcessor(log logrus.FieldLogger, ks *jwks.KeySet, issuer, aud string) *Processor {
	return &Processor{log: log, ks: ks, issuer: issuer, aud: aud}
}

func (p *Processor) MintAccess(pr Principal) (string, error) {
	now := time.Now()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"sub":             pr.UserID,
		"email":           pr.Email,
		"active_fleet_id": pr.ActiveFleetID,
		"role":            pr.Role,
		"iss":             p.issuer,
		"aud":             p.aud,
		"iat":             now.Unix(),
		"exp":             now.Add(accessTTL).Unix(),
	})
	tok.Header["kid"] = p.ks.Kid()
	return tok.SignedString(p.ks.Private())
}

func HashRefresh(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
```

Implement `entity.go` (`auth.refresh_tokens`: id, user_id, token_hash, family_id, expires_at, revoked_at, consumed_at), `provider.go`/`administrator.go` (insert, find-by-hash, rotate = consume old + insert new with same family, revoke-family), `model.go`, `builder.go` per the canonical template; refresh raw token = `uuid+uuid` random string returned to client, only the hash persisted.

- [ ] **Step 4: Implement resource.go — `POST /auth/refresh`, `POST /auth/logout`**

`resource.go` registers `/auth/refresh` (read refresh cookie/body → `FindByHash(HashRefresh(raw))` → if consumed/revoked: revoke family + 401; else re-resolve membership via fleet internal client (Task 4.5), rotate, `MintAccess`, set new tokens) and `/auth/logout` (revoke family). These are public routes (no JWT middleware) — registered on a sub-router without `auth.JWT`.

- [ ] **Step 5: Run tests, gate, commit**

Run: `go test -race ./apps/auth-service/internal/session/...`
Expected: PASS.

```bash
git add apps/auth-service
git commit -m "feat(auth): session domain — RS256 mint, hashed rotating refresh, refresh/logout"
```

### Task 4.4: `oidc` domain — Google verification + login/callback

**Files:**
- Create: `apps/auth-service/internal/oidc/{processor,resource}.go`
- Test: `apps/auth-service/internal/oidc/processor_test.go`

**Behavior (design §8.1):** `GET /auth/login/google` → 302 to Google consent (state+nonce in a signed, short-lived cookie). `GET /auth/callback?code` → exchange code, verify `id_token` (iss/aud/sig/nonce via Google's JWKS), extract `GoogleProfile`, call `user.ProvisionFromGoogle`, resolve membership, mint tokens, redirect to app (onboarding if no fleet).

- [ ] **Step 1: Add dep**

```bash
cd apps/auth-service && go get golang.org/x/oauth2@latest google.golang.org/api/idtoken@latest
```

- [ ] **Step 2: Write the failing test — verifier maps a validated id_token payload to GoogleProfile**

`apps/auth-service/internal/oidc/processor_test.go`:

```go
package oidc

import "testing"

func TestProfileFromClaims_extractsFields(t *testing.T) {
	gp := profileFromClaims(map[string]any{
		"sub": "g-123", "email": "a@b.com", "name": "Ann", "picture": "http://x/y.png",
	})
	if gp.Sub != "g-123" || gp.Email != "a@b.com" || gp.Name != "Ann" {
		t.Fatalf("bad profile: %+v", gp)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./apps/auth-service/internal/oidc/...`
Expected: FAIL — undefined `profileFromClaims`.

- [ ] **Step 4: Implement `profileFromClaims`, the oauth2 config, and verify-via `idtoken.Validate`**

`apps/auth-service/internal/oidc/processor.go` includes `profileFromClaims(map[string]any) user.GoogleProfile`, an `oauth2.Config` built from `GOOGLE_CLIENT_ID/SECRET/REDIRECT_URL`, and a `Verify(ctx, rawIDToken) (user.GoogleProfile, error)` using `idtoken.Validate(ctx, raw, clientID)` then `profileFromClaims(payload.Claims)`. `resource.go` wires `/auth/login/google` and `/auth/callback` (public routes) and orchestrates provision→resolve-membership→mint→redirect.

- [ ] **Step 5: Run test, gate, commit**

Run: `go test -race ./apps/auth-service/internal/oidc/...`
Expected: PASS.

```bash
git add apps/auth-service
git commit -m "feat(auth): Google OIDC login/callback, id_token verification"
```

### Task 4.5: Internal membership client + `cmd/main.go` wiring

**Files:**
- Create: `apps/auth-service/internal/membership/client.go` (HTTP client for fleet-service `GET /internal/memberships/active?user_id=`)
- Create: `apps/auth-service/cmd/main.go`
- Create: `apps/auth-service/Dockerfile`
- Test: `apps/auth-service/internal/membership/client_test.go`

- [ ] **Step 1: Write the failing test for the membership client (httptest server)**

`apps/auth-service/internal/membership/client_test.go`:

```go
package membership

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestActive_parsesFleetAndRole(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"fleet_id":"f1","role":"owner"}`))
	}))
	defer srv.Close()
	c := NewClient(srv.URL)
	m, err := c.Active(context.Background(), "u1")
	if err != nil || m.FleetID != "f1" || m.Role != "owner" {
		t.Fatalf("got %+v err %v", m, err)
	}
}

func TestActive_noMembershipReturnsEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	m, err := NewClient(srv.URL).Active(context.Background(), "u1")
	if err != nil || m.FleetID != "" {
		t.Fatalf("expected empty membership, got %+v err %v", m, err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./apps/auth-service/internal/membership/...`
Expected: FAIL — undefined `NewClient`.

- [ ] **Step 3: Implement the client**

`apps/auth-service/internal/membership/client.go`:

```go
// Package membership calls fleet-service's internal endpoint to resolve a user's
// active fleet/role for token minting (design §8.1; allowed under D2 — API, not join).
package membership

import (
	"context"
	"encoding/json"
	"net/http"
)

type Membership struct {
	FleetID string `json:"fleet_id"`
	Role    string `json:"role"`
}

type Client struct {
	base string
	hc   *http.Client
}

func NewClient(base string) *Client { return &Client{base: base, hc: http.DefaultClient} }

func (c *Client) Active(ctx context.Context, userID string) (Membership, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/internal/memberships/active?user_id="+userID, nil)
	res, err := c.hc.Do(req)
	if err != nil {
		return Membership{}, err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotFound {
		return Membership{}, nil
	}
	var m Membership
	if err := json.NewDecoder(res.Body).Decode(&m); err != nil {
		return Membership{}, err
	}
	return m, nil
}
```

- [ ] **Step 4: Implement `cmd/main.go` (the canonical bootstrap, design §6)**

`apps/auth-service/cmd/main.go`:

```go
package main

import (
	"crypto/x509"
	"encoding/pem"

	"github.com/go-chi/chi/v5"

	authmw "github.com/jtumidanski/myfleet/packages/shared-go/auth"
	"github.com/jtumidanski/myfleet/packages/shared-go/config"
	"github.com/jtumidanski/myfleet/packages/shared-go/database"
	"github.com/jtumidanski/myfleet/packages/shared-go/health"
	"github.com/jtumidanski/myfleet/packages/shared-go/server"
	"github.com/jtumidanski/myfleet/packages/shared-go/telemetry"

	"github.com/jtumidanski/myfleet/apps/auth-service/internal/jwks"
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

	ks := loadKeySet()
	keyfn, err := authmw.NewJWKSKeyfunc(req(), config.MustGet("JWKS_URL"))
	if err != nil {
		log.WithError(err).Fatal("jwks keyfunc")
	}

	sess := session.NewProcessor(log, ks, config.Get("JWT_ISSUER", "myfleet-auth"), config.Get("JWT_AUDIENCE", "myfleet"))
	_ = sess

	server.New(log).
		Use(telemetry.CorrelationID).
		// public routes (no JWT): jwks, oidc login/callback, refresh/logout
		AddRouteInitializer(jwks.InitializeRoutes(ks)).
		AddRouteInitializer(oidc.InitializeRoutes(/* deps */)).
		AddRouteInitializer(session.InitializePublicRoutes(/* deps */)).
		// protected routes (JWT): /auth/me
		AddRouteInitializer(func(r chi.Router) {
			r.Group(func(pr chi.Router) {
				pr.Use(authmw.JWT(keyfn))
				user.InitializeRoutes(db)(pr)
			})
		}).
		AddRouteInitializer(func(r chi.Router) {
			r.Get("/healthz", health.Liveness())
			r.Get("/readyz", health.Readiness(func() error { d, _ := db.DB(); return d.Ping() }))
		}).
		Run()
}

func loadKeySet() *jwks.KeySet {
	block, _ := pem.Decode([]byte(config.MustGet("JWT_PRIVATE_KEY_PEM")))
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		panic(err)
	}
	return jwks.NewKeySet(key, config.Get("JWT_KID", "kid-1"))
}
```

> The `req()` / wiring placeholders are resolved when implementing: pass a `context.Background()`, construct `oidc`/`session` deps (user store, membership client, key set) and inject. Keep `cmd/main.go` thin — orchestration only.

- [ ] **Step 5: Implement the multi-stage Dockerfile (design §17: non-root, port 8080)**

`apps/auth-service/Dockerfile`:

```dockerfile
FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.work go.work.sum ./
COPY packages ./packages
COPY apps/auth-service ./apps/auth-service
RUN cd apps/auth-service && go build -o /out/auth-service ./cmd

FROM alpine:3.20
RUN adduser -D -u 10001 app
USER app
COPY --from=build /out/auth-service /auth-service
EXPOSE 8080
ENTRYPOINT ["/auth-service"]
```

- [ ] **Step 6: Gate + add auth-service to compose, commit**

Run: `go build ./apps/auth-service/... && go vet ./apps/auth-service/... && go test -race ./apps/auth-service/...`
Expected: PASS.

Add an `auth-service` service block to `deploy/compose/docker-compose.yml` with Traefik labels routing `PathPrefix(/api/auth)` (strip `/api/auth`? No — services own `/auth/*`; route `/api/auth` → service, rewrite prefix to `/`), env from `.env`, `JWKS_URL=http://auth-service:8080/.well-known/jwks.json`, `depends_on: postgres`. Verify `docker compose config` parses.

```bash
git add apps/auth-service deploy/compose/docker-compose.yml
git commit -m "feat(auth): membership client, cmd/main wiring, Dockerfile, compose service"
```

---

## Phase 5 — `fleet-service` core: fleet, membership, invite + authz spine

Implements design §8.2 (fleet/membership/invite), §9 (fleet-scoped authorization — the spine every later fleet domain reuses), and §10.6 soft-delete base. Module: `github.com/jtumidanski/myfleet/apps/fleet-service`. Path `/api/fleet/*`. **Re-read `backend-dev-guidelines`.**

### Task 5.1: Module bootstrap + fleet-scoped authz middleware (the spine)

**Files:**
- Create: `apps/fleet-service/go.mod`
- Create: `apps/fleet-service/internal/authz/scope.go`
- Test: `apps/fleet-service/internal/authz/scope_test.go`

The authz spine (design §9): a resource's stored `fleet_id` must equal the token's `active_fleet_id` (else **404**, never leak existence); role gates (viewer read-only; member writes; owner-only actions). For owner-only actions fleet-service consults its **own membership table** as authoritative (token role may be ≤15 min stale).

- [ ] **Step 1: Init module + deps + workspace**

```bash
cd apps/fleet-service && go mod init github.com/jtumidanski/myfleet/apps/fleet-service
go get github.com/jtumidanski/myfleet/packages/shared-go github.com/jtumidanski/myfleet/packages/dto-go github.com/go-chi/chi/v5@latest gorm.io/gorm@latest gorm.io/driver/postgres@latest github.com/google/uuid@latest
cd "$(git rev-parse --show-toplevel)" && go work use ./apps/fleet-service
```

- [ ] **Step 2: Write the failing test for fleet-scope enforcement**

`apps/fleet-service/internal/authz/scope_test.go`:

```go
package authz

import (
	"errors"
	"testing"

	"github.com/jtumidanski/myfleet/packages/shared-go/auth"
	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

func TestRequireSameFleet_404OnMismatch(t *testing.T) {
	id := auth.Identity{ActiveFleetID: "f1", Role: "owner"}
	if err := RequireSameFleet(id, "f2"); !errors.Is(err, server.ErrNotFound) {
		t.Fatalf("cross-fleet must be 404 (no leak), got %v", err)
	}
}

func TestRequireSameFleet_okOnMatch(t *testing.T) {
	id := auth.Identity{ActiveFleetID: "f1", Role: "viewer"}
	if err := RequireSameFleet(id, "f1"); err != nil {
		t.Fatalf("same fleet should pass, got %v", err)
	}
}

func TestRequireWrite_viewerForbidden(t *testing.T) {
	if err := RequireWrite(auth.Identity{Role: "viewer"}); !errors.Is(err, server.ErrForbidden) {
		t.Fatalf("viewer write must be 403, got %v", err)
	}
	if err := RequireWrite(auth.Identity{Role: "member"}); err != nil {
		t.Fatalf("member write should pass, got %v", err)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./apps/fleet-service/internal/authz/...`
Expected: FAIL — undefined `RequireSameFleet`, `RequireWrite`.

- [ ] **Step 4: Implement the authz helpers**

`apps/fleet-service/internal/authz/scope.go`:

```go
// Package authz is the fleet-scoped authorization spine (design §9). Every
// resource handler calls RequireSameFleet then a role guard.
package authz

import (
	"github.com/jtumidanski/myfleet/packages/shared-go/auth"
	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// RequireSameFleet returns 404 (not 403) when the resource is in another fleet,
// so cross-fleet existence is never leaked (design §9, PRD §5.1).
func RequireSameFleet(id auth.Identity, resourceFleetID string) error {
	if id.ActiveFleetID == "" || id.ActiveFleetID != resourceFleetID {
		return server.ErrNotFound
	}
	return nil
}

// RequireWrite allows member|owner; viewer is read-only (403).
func RequireWrite(id auth.Identity) error {
	if id.Role == "member" || id.Role == "owner" {
		return nil
	}
	return server.ErrForbidden
}

// RequireOwner allows only owner (403 otherwise). Callers should additionally
// confirm against the membership table for owner-only mutations (stale-claim guard).
func RequireOwner(id auth.Identity) error {
	if id.Role == "owner" {
		return nil
	}
	return server.ErrForbidden
}
```

- [ ] **Step 5: Run test, gate, commit**

Run: `go test -race ./apps/fleet-service/internal/authz/...`
Expected: PASS.

```bash
git add apps/fleet-service go.work
git commit -m "feat(fleet): module bootstrap + fleet-scoped authz spine (404 no-leak, role gates)"
```

### Task 5.2: `fleet` + `membership` domains + internal membership endpoint

**Files (canonical 8-file template each):**
- Create: `apps/fleet-service/internal/fleet/{model,entity,builder,provider,processor,administrator,rest,resource}.go`
- Create: `apps/fleet-service/internal/membership/{model,entity,builder,provider,processor,administrator,rest,resource}.go`
- Test: `apps/fleet-service/internal/membership/processor_test.go`

**Tables:** `fleet.fleets` (id, name, created_by_user_id, timestamps, deleted_at), `fleet.fleet_memberships` (id, fleet_id, user_id, role, status, unique(fleet_id,user_id)) per PRD §6.

**Endpoints (design §8.2):** `POST /fleets` (onboarding — creates fleet + owner membership in one tx), `GET /fleets/{id}`, `PATCH /fleets/{id}` (rename, owner-only), `GET /fleets/{id}/members`, `DELETE /fleets/{id}/members/{userId}` (owner-only), and internal `GET /internal/memberships/active?user_id=` (network-restricted, no JWT) returning `{fleet_id, role}`.

- [ ] **Step 1: Write the failing test — sole-owner self-removal guard (FR-FLEET-4)**

`apps/fleet-service/internal/membership/processor_test.go`:

```go
package membership

import (
	"errors"
	"testing"

	"github.com/sirupsen/logrus"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

type stubProvider struct{ owners int }

func (s stubProvider) CountOwners(fleetID string) (int, error) { return s.owners, nil }

func TestRemoveMember_blocksSoleOwnerSelfRemoval(t *testing.T) {
	p := NewProcessor(logrus.New(), stubProvider{owners: 1})
	err := p.ValidateRemoval("f1", "u-owner", "u-owner", "owner")
	if !errors.Is(err, server.ErrConflict) {
		t.Fatalf("sole owner self-removal must be 409, got %v", err)
	}
}

func TestRemoveMember_allowsWhenAnotherOwnerExists(t *testing.T) {
	p := NewProcessor(logrus.New(), stubProvider{owners: 2})
	if err := p.ValidateRemoval("f1", "u-owner", "u-owner", "owner"); err != nil {
		t.Fatalf("removal with co-owner should pass, got %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./apps/fleet-service/internal/membership/...`
Expected: FAIL — undefined symbols.

- [ ] **Step 3: Implement membership processor (sole-owner guard) + the rest of both domains**

`apps/fleet-service/internal/membership/processor.go` (guard shown in full; remaining CRUD per template):

```go
package membership

import (
	"github.com/sirupsen/logrus"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

type OwnerCounter interface{ CountOwners(fleetID string) (int, error) }

type Processor struct {
	log logrus.FieldLogger
	p   OwnerCounter
}

func NewProcessor(log logrus.FieldLogger, p OwnerCounter) *Processor { return &Processor{log: log, p: p} }

// ValidateRemoval enforces FR-FLEET-4: an owner cannot remove themselves if they
// are the only owner.
func (pr *Processor) ValidateRemoval(fleetID, actorUserID, targetUserID, targetRole string) error {
	if actorUserID == targetUserID && targetRole == "owner" {
		n, err := pr.p.CountOwners(fleetID)
		if err != nil {
			return err
		}
		if n <= 1 {
			return server.ErrConflict
		}
	}
	return nil
}
```

Implement `fleet` and `membership` model/entity/builder/provider/administrator/rest/resource per the Task 4.2 template. The `POST /fleets` administrator wraps fleet insert + owner-membership insert in a single `db.Transaction`. The internal endpoint queries the active membership for a user.

- [ ] **Step 4: Run test, gate, commit**

Run: `go test -race ./apps/fleet-service/internal/fleet/... ./apps/fleet-service/internal/membership/...`
Expected: PASS.

```bash
git add apps/fleet-service
git commit -m "feat(fleet): fleet + membership domains, onboarding, internal membership endpoint"
```

### Task 5.3: `invite` domain — email invites with same-email + expiry enforcement

**Files (canonical template):**
- Create: `apps/fleet-service/internal/invite/{model,entity,builder,provider,processor,administrator,rest,resource}.go`
- Test: `apps/fleet-service/internal/invite/processor_test.go`

**Table:** `fleet.fleet_invites` (id, fleet_id, email, role, token unique, expires_at, accepted_at, invited_by_user_id). **Endpoints:** `POST /fleets/{id}/invites` (owner-only), `GET /fleets/{id}/invites`, `DELETE /invites/{id}` (owner-only), `POST /invites/{token}/accept`.

- [ ] **Step 1: Write the failing test — accept enforces same-email + not expired/used (FR-FLEET-3)**

`apps/fleet-service/internal/invite/processor_test.go`:

```go
package invite

import (
	"errors"
	"testing"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

func mk(email string, expires time.Time, accepted *time.Time) Model {
	return NewBuilder().SetEmail(email).SetExpiresAt(expires).setAcceptedAt(accepted).Build()
}

func TestAccept_rejectsEmailMismatch(t *testing.T) {
	p := NewProcessor(logrus.New())
	inv := mk("invited@b.com", time.Now().Add(time.Hour), nil)
	if err := p.ValidateAccept(inv, "other@b.com"); !errors.Is(err, server.ErrConflict) {
		t.Fatalf("email mismatch must be 409, got %v", err)
	}
}

func TestAccept_rejectsExpired(t *testing.T) {
	p := NewProcessor(logrus.New())
	inv := mk("a@b.com", time.Now().Add(-time.Hour), nil)
	if err := p.ValidateAccept(inv, "a@b.com"); !errors.Is(err, server.ErrConflict) {
		t.Fatalf("expired must be 409, got %v", err)
	}
}

func TestAccept_rejectsAlreadyAccepted(t *testing.T) {
	now := time.Now()
	p := NewProcessor(logrus.New())
	inv := mk("a@b.com", now.Add(time.Hour), &now)
	if err := p.ValidateAccept(inv, "a@b.com"); !errors.Is(err, server.ErrConflict) {
		t.Fatalf("already-accepted must be 409, got %v", err)
	}
}

func TestAccept_okWhenValid(t *testing.T) {
	p := NewProcessor(logrus.New())
	inv := mk("a@b.com", time.Now().Add(time.Hour), nil)
	if err := p.ValidateAccept(inv, "a@b.com"); err != nil {
		t.Fatalf("valid accept should pass, got %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./apps/fleet-service/internal/invite/...`
Expected: FAIL.

- [ ] **Step 3: Implement the invite processor validation + full domain**

`apps/fleet-service/internal/invite/processor.go` (`ValidateAccept` checks: `accepted_at == nil`, `expires_at > now`, `strings.EqualFold(inv.Email(), authedEmail)`; any failure → `server.ErrConflict`). Acceptance administrator creates the membership (status active) and stamps `accepted_at` in one transaction; emits `member.invited` via the (Noop for now) producer.

- [ ] **Step 4: Run test, gate, commit**

Run: `go test -race ./apps/fleet-service/internal/invite/...`
Expected: PASS.

```bash
git add apps/fleet-service
git commit -m "feat(fleet): invite domain — same-email + expiry enforcement, accept→membership"
```

### Task 5.4: `cmd/main.go` wiring + Dockerfile + compose

**Files:**
- Create: `apps/fleet-service/cmd/main.go`
- Create: `apps/fleet-service/Dockerfile`
- Modify: `deploy/compose/docker-compose.yml`

- [ ] **Step 1:** Implement `cmd/main.go` mirroring auth's bootstrap: connect DB with `SetMigrations(fleet.Migration, membership.Migration, invite.Migration, events.MigrateOutbox, ...)`, `Use(telemetry.CorrelationID, authmw.JWT(keyfn))` for protected routes, register each domain's routes, register internal membership route on a separate path **without** the JWT middleware (network-restricted), wire health. Inject `events.NoopProducer{}` for now (real producer in Phase 11).
- [ ] **Step 2:** Copy the auth Dockerfile, change the build target to `./apps/fleet-service/cmd` and binary name.
- [ ] **Step 3:** Add `fleet-service` to compose with Traefik label `PathPrefix(/api/fleet)` plus the core-domain prefixes it owns (route `/api/fleet`, `/api/vehicles`, `/api/maintenance-*`, `/api/fuel-logs`, `/api/invites`, `/api/notifications`? no — notifications is its own service). Simpler: front fleet-service at `PathPrefix(/api/fleet)` and have the web client call fleet routes under `/api/fleet/...`. **Decision:** prefix all fleet-service public routes with `/api/fleet` at the gateway and strip to `/` — register routes as `/fleets`, `/vehicles`, etc. inside the service. Document this in `context.md`.
- [ ] **Step 4: Gate + verify compose**

Run: `go build ./apps/fleet-service/... && go vet ./apps/fleet-service/... && go test -race ./apps/fleet-service/... && (cd deploy/compose && docker compose config >/dev/null && echo OK)`
Expected: PASS + `OK`.

- [ ] **Step 5: Commit**

```bash
git add apps/fleet-service deploy/compose/docker-compose.yml
git commit -m "feat(fleet): cmd/main wiring, Dockerfile, compose service"
```

---

## Phase 6 — `fleet-service`: vehicle + vehicle-media references

Implements design §8.2 (vehicle, vehiclemedia) and §10.6 soft-delete/restore/purge. The vehicle domain is the **canonical CRUD+soft-delete pattern** reused by maintenance records, fuel logs, etc.

### Task 6.1: `vehicle` domain — CRUD + fields

**Files (canonical template):** `apps/fleet-service/internal/vehicle/{model,entity,builder,provider,processor,administrator,rest,resource}.go`; Test: `processor_test.go`.

**Table** `fleet.vehicles` (PRD §6): id, fleet_id (idx), nickname, make, model, trim, year, vin, current_mileage, primary_image_media_id, notes, timestamps, deleted_at, purge_after. **Endpoints:** `GET/POST /fleets/{id}/vehicles`, `GET/PATCH/DELETE /vehicles/{id}` (DELETE=soft), `POST /vehicles/{id}/restore` (owner-only), `PUT /vehicles/{id}/primary-image`. All write handlers call `authz.RequireSameFleet` + `authz.RequireWrite`; restore calls `authz.RequireOwner`.

- [ ] **Step 1: Write the failing test — builder enforces required fields (make/model/year), list is fleet-paged**

`apps/fleet-service/internal/vehicle/processor_test.go`:

```go
package vehicle

import (
	"errors"
	"testing"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

func TestBuild_requiresMakeModelYear(t *testing.T) {
	_, err := NewBuilder().SetFleetID("f1").Build() // missing make/model/year
	if !errors.Is(err, server.ErrValidation) {
		t.Fatalf("missing required fields must be 422, got %v", err)
	}
}

func TestBuild_okWithRequired(t *testing.T) {
	m, err := NewBuilder().SetFleetID("f1").SetMake("Toyota").SetModel("Tacoma").SetYear(2021).Build()
	if err != nil {
		t.Fatalf("valid build failed: %v", err)
	}
	if m.FleetID() != "f1" || m.Make() != "Toyota" || m.Year() != 2021 {
		t.Fatalf("unexpected model: %+v", m)
	}
}
```

> Note: this domain's `Build()` returns `(Model, error)` because it enforces invariants (design §6 "fluent builder enforcing invariants via Build()"). Builders in auth (no invariants) returned `Model`; that's fine — match invariant needs per domain. Document this convention in `context.md`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./apps/fleet-service/internal/vehicle/...`
Expected: FAIL — undefined symbols.

- [ ] **Step 3: Implement model/entity/builder (with validation)/provider/administrator/rest/resource**

Builder's `Build()`:

```go
func (b *Builder) Build() (Model, error) {
	if b.m.make == "" || b.m.model == "" || b.m.year == 0 {
		return Model{}, server.ErrValidation
	}
	return b.m, nil
}
```

Provider list method uses `server.Page` for pagination and filters `fleet_id = ? AND deleted_at IS NULL`. `rest.go` `TransformSlice` emits `meta` from `page.Meta(total)`.

- [ ] **Step 4: Run test, gate, commit**

Run: `go test -race ./apps/fleet-service/internal/vehicle/...`
Expected: PASS.

```bash
git add apps/fleet-service
git commit -m "feat(fleet): vehicle CRUD domain (canonical CRUD pattern)"
```

### Task 6.2: Soft-delete + restore + purge job (FR-VEH-4, §10.6)

**Files:** extend `vehicle/administrator.go`, `vehicle/processor.go`; Create `apps/fleet-service/internal/vehicle/purge.go`; Test: `purge_test.go`.

- [ ] **Step 1: Write the failing test — soft-delete sets purge_after = deleted_at+5d; restore only valid before purge; accessing purged → 410**

`apps/fleet-service/internal/vehicle/purge_test.go`:

```go
package vehicle

import (
	"testing"
	"time"
)

func TestPurgeAfter_isFiveDaysAfterDelete(t *testing.T) {
	deletedAt := time.Date(2026, 5, 24, 0, 0, 0, 0, time.UTC)
	if got := ComputePurgeAfter(deletedAt); !got.Equal(deletedAt.Add(5 * 24 * time.Hour)) {
		t.Fatalf("purge_after want +5d, got %v", got)
	}
}

func TestIsPurgeable_onlyPastWindow(t *testing.T) {
	past := time.Now().Add(-time.Minute)
	future := time.Now().Add(time.Hour)
	if !IsPurgeable(&past) {
		t.Fatal("row past purge_after should be purgeable")
	}
	if IsPurgeable(&future) {
		t.Fatal("row before purge_after must not be purged")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./apps/fleet-service/internal/vehicle/... -run Purge`
Expected: FAIL — undefined `ComputePurgeAfter`, `IsPurgeable`.

- [ ] **Step 3: Implement purge helpers + the sweep job**

`apps/fleet-service/internal/vehicle/purge.go`:

```go
package vehicle

import "time"

const recoveryWindow = 5 * 24 * time.Hour

func ComputePurgeAfter(deletedAt time.Time) time.Time { return deletedAt.Add(recoveryWindow) }

func IsPurgeable(purgeAfter *time.Time) bool {
	return purgeAfter != nil && time.Now().After(*purgeAfter)
}

// PurgeExpired hard-deletes rows past purge_after. Run under WithLeaderLock (A9).
func PurgeExpired(db interface{ Exec(string, ...any) error }) error {
	return db.Exec("DELETE FROM fleet.vehicles WHERE purge_after IS NOT NULL AND purge_after < now()")
}
```

> The actual `PurgeExpired` signature uses `*gorm.DB`; the interface above is illustrative — implement against `*gorm.DB` and assert `.Error`. Soft-delete administrator sets `deleted_at=now()`, `purge_after=ComputePurgeAfter(now())`. Restore administrator clears both **only if** `!IsPurgeable(purge_after)`, else returns `server.ErrGone` (410). Reads of a soft-deleted-and-purgeable row also return `server.ErrGone`.

- [ ] **Step 4: Wire the daily purge job in cmd/main.go** via `jobs.Every(ctx, 24h, func(ctx){ database.WithLeaderLock(db,"vehicle-purge", func() error { return vehicle.PurgeExpired(db) }) })`.

- [ ] **Step 5: Run test, gate, commit**

Run: `go test -race ./apps/fleet-service/internal/vehicle/...`
Expected: PASS.

```bash
git add apps/fleet-service
git commit -m "feat(fleet): vehicle soft-delete/restore/purge + daily purge job"
```

### Task 6.3: `vehiclemedia` domain — image refs + primary selection

**Files (canonical template):** `apps/fleet-service/internal/vehiclemedia/{...}.go`; Test: `processor_test.go`.

**Table** `fleet.vehicle_media` (id, vehicle_id idx, media_id ref, is_primary, sort_order, created_at, deleted_at). fleet-service stores only `media_id` references; the web app resolves bytes via media-service (design §8.3). `PUT /vehicles/{id}/primary-image` sets one row `is_primary=true` and clears others (in a tx); mirrors `vehicles.primary_image_media_id`.

- [ ] **Step 1: Failing test — setting a new primary unsets the previous (only one primary per vehicle)**

`processor_test.go`: `TestSetPrimary_unsetsPrevious` — given two media rows, setting primary on the second yields exactly one `is_primary=true`. Use a fake provider returning the two rows and a fake writer capturing updates; assert the previous primary is updated to false and the new to true.

- [ ] **Step 2–4:** Run (fail) → implement processor `SetPrimary(vehicleID, mediaID)` + domain files → run (pass).

- [ ] **Step 5: Gate + commit**

```bash
go test -race ./apps/fleet-service/internal/vehiclemedia/...
git add apps/fleet-service && git commit -m "feat(fleet): vehicle-media references + primary-image selection"
```

---

## Phase 7 — `media-service`

Implements design §8.3 and §10.6 (media purge). Module: `github.com/jtumidanski/myfleet/apps/media-service`. Path `/api/media/*`. Owns `media.media_objects`, `media.media_variants`. MinIO private buckets only.

### Task 7.1: Module bootstrap + MinIO client wrapper

**Files:** `apps/media-service/go.mod`; `apps/media-service/internal/storage/minio.go`; Test: `storage/minio_test.go` (unit-test object-key generation only; MinIO calls are integration-tested in Phase 18).

- [ ] **Step 1: Init module + deps + workspace**

```bash
cd apps/media-service && go mod init github.com/jtumidanski/myfleet/apps/media-service
go get github.com/jtumidanski/myfleet/packages/shared-go github.com/jtumidanski/myfleet/packages/dto-go github.com/minio/minio-go/v7@latest github.com/go-chi/chi/v5@latest gorm.io/gorm@latest gorm.io/driver/postgres@latest github.com/google/uuid@latest
cd "$(git rev-parse --show-toplevel)" && go work use ./apps/media-service
```

- [ ] **Step 2: Failing test — object key is `fleetID/uuid/filename`, sanitized**

`storage/minio_test.go`:

```go
package storage

import (
	"strings"
	"testing"
)

func TestObjectKey_namespacedByFleet(t *testing.T) {
	k := ObjectKey("f1", "id-1", "My Receipt.pdf")
	if !strings.HasPrefix(k, "f1/id-1/") || strings.Contains(k, " ") {
		t.Fatalf("unexpected key %q", k)
	}
}
```

- [ ] **Step 3: Run (fail) → implement**

`apps/media-service/internal/storage/minio.go` exposes `ObjectKey(fleetID, id, filename string) string` (slugify filename, join with `/`), plus `Client` wrapping `*minio.Client` with `PresignPut(ctx, key, ttl)`, `PresignGet(ctx, key, ttl)`, `PutObject`, `RemoveObject`. Bucket name from `MEDIA_BUCKET`. Buckets created on startup if absent; never public.

- [ ] **Step 4: Gate + commit**

```bash
go test -race ./apps/media-service/internal/storage/...
git add apps/media-service go.work && git commit -m "feat(media): module bootstrap + MinIO client + object-key scheme"
```

### Task 7.2: `mediaobject` domain — presigned upload init/confirm + presigned download

**Files (canonical template):** `apps/media-service/internal/mediaobject/{...}.go`; Test: `processor_test.go`.

**Table** `media.media_objects` (PRD §6): id, fleet_id idx, uploaded_by_user_id, bucket, object_key, content_type, size, original_filename, status (uploaded|processing|ready), created_at, deleted_at, purge_after. **Endpoints (design §8.3):** `POST /media` (init → row status=uploaded + presigned PUT URL), `POST /media/{id}/confirm` (→ produce `media.uploaded`), `GET /media/{id}` (metadata; fleet-scoped → 404 cross-fleet), `GET /media/{id}/download` (short-lived presigned GET after authz), `DELETE /media/{id}` (soft).

- [ ] **Step 1: Failing test — download authorizes by fleet (cross-fleet → 404), status transitions uploaded→processing→ready are validated**

`processor_test.go`:
- `TestAuthorizeAccess_404CrossFleet` — `processor.AuthorizeAccess(obj, identityFleetID)` returns `server.ErrNotFound` when `obj.FleetID() != identityFleetID`.
- `TestMarkReady_requiresProcessing` — transition guard: `ready` only valid from `processing`.

- [ ] **Step 2–3:** Run (fail) → implement domain + processor with the authz + transition guards. Fleet scoping reuses the token's `active_fleet_id` (media trusts the token claim per design §9).

- [ ] **Step 4: Gate + commit**

```bash
go test -race ./apps/media-service/internal/mediaobject/...
git add apps/media-service && git commit -m "feat(media): media object domain — presigned upload/download, fleet-scoped authz"
```

### Task 7.3: `mediavariant` domain + Kafka worker pool for variant generation (A7)

**Files:** `apps/media-service/internal/mediavariant/{model,entity,...}.go`; `apps/media-service/internal/processing/worker.go`; Test: `processing/worker_test.go`.

**Table** `media.media_variants` (id, media_object_id idx, variant thumbnail|display, object_key, width, height, content_type, created_at). Worker pool consumes `media.uploaded` (design §7), downloads the original, generates `thumbnail` + `display` via `image`/`golang.org/x/image`, uploads variants, inserts rows, sets object status=ready. Idempotent: keyed by `media_object_id` — re-run overwrites variants.

- [ ] **Step 1: Failing test — variant spec computes target dimensions preserving aspect ratio**

`processing/worker_test.go`: `TestResizeDims_preservesAspect` — `ResizeDims(4000, 3000, 320)` (thumbnail max edge 320) returns `(320, 240)`; `display` max edge 1280 returns proportional dims; never upscales.

- [ ] **Step 2–3:** Run (fail) → implement `ResizeDims(w,h,maxEdge int) (int,int)` and the worker loop (`events.Consume` → process → `MarkReady`). Use `processed_events` dedupe via a small helper (insert event_id, skip if exists).

- [ ] **Step 4: Wire worker + cmd/main.go + Dockerfile + compose**

`cmd/main.go` connects DB (`SetMigrations(mediaobject.Migration, mediavariant.Migration, processedEvents.Migration)`), starts the variant worker goroutines (count from `MEDIA_WORKERS`, default 2), wires media routes behind JWT, daily media purge job (soft-deleted objects past purge_after → delete rows + MinIO keys, under advisory lock). Dockerfile mirrors auth; add `media-service` to compose with MinIO + Kafka env and Traefik `PathPrefix(/api/media)`.

- [ ] **Step 5: Gate + verify compose + commit**

Run: `go build ./apps/media-service/... && go vet ./apps/media-service/... && go test -race ./apps/media-service/... && (cd deploy/compose && docker compose config >/dev/null && echo OK)`
Expected: PASS + OK.

```bash
git add apps/media-service deploy/compose/docker-compose.yml && git commit -m "feat(media): variant worker pool, purge job, cmd/main wiring, compose"
```

---

## Phase 8 — `fleet-service`: mileage

Implements design §8.2 (mileage) and §10.4. Append-only history; `vehicles.current_mileage` mirrors the latest record.

### Task 8.1: `mileage` domain — append-only records + latest mirror

**Files (canonical template):** `apps/fleet-service/internal/mileage/{...}.go`; Test: `processor_test.go`.

**Table** `fleet.mileage_records` (PRD §6): id, vehicle_id idx, mileage, recorded_at, source (fuel|maintenance|manual), source_ref_id, created_by_user_id, created_at. Append-only — no update/delete. **Endpoints:** `GET/POST /vehicles/{id}/mileage` (chronological; graph query params: `?from=&to=`). Index on `recorded_at` (design §13).

- [ ] **Step 1: Failing test — appending a record updates the vehicle's current_mileage to the latest; below-latest entries are allowed but flagged (design §14)**

`apps/fleet-service/internal/mileage/processor_test.go`:

```go
package mileage

import (
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

type fakeVehicleMileage struct{ set int }

func (f *fakeVehicleMileage) UpdateCurrentMileage(vehicleID string, m int) error { f.set = m; return nil }

func TestAppend_updatesCurrentMileageWhenHigher(t *testing.T) {
	vm := &fakeVehicleMileage{set: 1000}
	p := NewProcessor(logrus.New(), vm)
	rec := NewBuilder().SetVehicleID("v1").SetMileage(1500).SetRecordedAt(time.Now()).SetSource("manual").Build()
	flagged, err := p.OnAppend(rec, 1000)
	if err != nil || flagged {
		t.Fatalf("higher mileage should not be flagged; flagged=%v err=%v", flagged, err)
	}
	if vm.set != 1500 {
		t.Fatalf("current_mileage should advance to 1500, got %d", vm.set)
	}
}

func TestAppend_flagsBelowLatestButKeeps(t *testing.T) {
	vm := &fakeVehicleMileage{set: 2000}
	p := NewProcessor(logrus.New(), vm)
	rec := NewBuilder().SetVehicleID("v1").SetMileage(1500).SetRecordedAt(time.Now()).SetSource("manual").Build()
	flagged, err := p.OnAppend(rec, 2000)
	if err != nil {
		t.Fatal(err)
	}
	if !flagged {
		t.Fatal("below-latest entry should be flagged")
	}
	if vm.set != 2000 {
		t.Fatalf("current_mileage must not regress, stayed at latest; got %d", vm.set)
	}
}
```

- [ ] **Step 2: Run (fail) → Step 3: implement**

`OnAppend(rec Model, currentLatest int) (flagged bool, err error)`: if `rec.Mileage() < currentLatest` → flagged=true, do **not** advance current_mileage; else call `UpdateCurrentMileage`. History is never dropped. `OnAppend` is the reusable hook the fuel/maintenance processors call (design §10.4).

- [ ] **Step 4: Gate + commit**

Run: `go test -race ./apps/fleet-service/internal/mileage/...`

```bash
git add apps/fleet-service && git commit -m "feat(fleet): mileage append-only records + current-mileage mirror"
```

---

## Phase 9 — `fleet-service`: maintenance

Implements design §8.2 (categories/records/schedules), §10.1–10.3 (recurrence, severity, completion), and the recurrence recompute job (§11). The recurrence and completion logic is pure and gets exhaustive table-driven tests.

### Task 9.1: `maintenancecategory` domain + seed migration (FR-MAINT-1)

**Files (canonical template):** `apps/fleet-service/internal/maintenancecategory/{...}.go`; Test: `entity_test.go`.

**Table** `fleet.maintenance_categories` (id, name, description, system_defined). **Endpoint:** `GET /maintenance-categories`. Seeded via migration: Oil Change, Tire Rotation, Brake Service, Air Filter, Transmission Service, Coolant Flush, Battery, Inspection (system_defined=true).

- [ ] **Step 1: Failing test — `SeedCategories` is idempotent (re-running doesn't duplicate)**

`entity_test.go` (sqlite): seed twice, assert count == number of seeds.

- [ ] **Step 2–3:** Run (fail) → implement `Migration` (AutoMigrate) + `Seed(db)` using `FirstOrCreate` keyed by name.

- [ ] **Step 4: Gate + commit**

```bash
go test -race ./apps/fleet-service/internal/maintenancecategory/...
git add apps/fleet-service && git commit -m "feat(fleet): maintenance categories + idempotent seed"
```

### Task 9.2: Recurrence engine — next-due + due-state + severity (§10.1)

**Files:** `apps/fleet-service/internal/maintenanceschedule/recurrence.go`; Test: `recurrence_test.go`. (Pure functions; no DB.)

- [ ] **Step 1: Write the failing table-driven test (covers time/mileage/hybrid, ok/upcoming/overdue, severity)**

`apps/fleet-service/internal/maintenanceschedule/recurrence_test.go`:

```go
package maintenanceschedule

import (
	"testing"
	"time"
)

var base = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func TestNextDue(t *testing.T) {
	cases := []struct {
		name          string
		recur         string
		months, miles int
		lastDate      time.Time
		lastMiles     int
		wantDate      time.Time
		wantMiles     int
	}{
		{"time", "time", 12, 0, base, 0, base.AddDate(0, 12, 0), 0},
		{"mileage", "mileage", 0, 5000, base, 30000, time.Time{}, 35000},
		{"hybrid", "hybrid", 12, 5000, base, 30000, base.AddDate(0, 12, 0), 35000},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := Schedule{RecurrenceType: c.recur, IntervalMonths: c.months, IntervalMiles: c.miles, LastCompletedDate: c.lastDate, LastCompletedMileage: c.lastMiles}
			nd, nm := NextDue(s)
			if c.recur != "mileage" && !nd.Equal(c.wantDate) {
				t.Fatalf("next_due_date = %v want %v", nd, c.wantDate)
			}
			if c.recur != "time" && nm != c.wantMiles {
				t.Fatalf("next_due_mileage = %d want %d", nm, c.wantMiles)
			}
		})
	}
}

func TestDueState(t *testing.T) {
	s := Schedule{RecurrenceType: "hybrid", IntervalMonths: 12, IntervalMiles: 5000, LastCompletedDate: base, LastCompletedMileage: 30000}
	// today far before due, mileage well under → ok
	if got := DueState(s, base.AddDate(0, 1, 0), 31000, DefaultThresholds); got != "ok" {
		t.Fatalf("want ok, got %s", got)
	}
	// mileage within 500 of 35000 → upcoming
	if got := DueState(s, base.AddDate(0, 1, 0), 34600, DefaultThresholds); got != "upcoming" {
		t.Fatalf("want upcoming (within 500mi), got %s", got)
	}
	// past 35000 → overdue
	if got := DueState(s, base.AddDate(0, 1, 0), 35001, DefaultThresholds); got != "overdue" {
		t.Fatalf("want overdue (mileage exceeded), got %s", got)
	}
	// time past next_due_date → overdue
	if got := DueState(s, base.AddDate(0, 13, 0), 31000, DefaultThresholds); got != "overdue" {
		t.Fatalf("want overdue (time exceeded), got %s", got)
	}
}

func TestSeverity(t *testing.T) {
	if Severity("ok") != "informational" || Severity("upcoming") != "recommended" || Severity("overdue") != "urgent" {
		t.Fatal("severity bands per design §10.1")
	}
}
```

- [ ] **Step 2: Run (fail)** — Run: `go test ./apps/fleet-service/internal/maintenanceschedule/... -run 'NextDue|DueState|Severity'` → FAIL.

- [ ] **Step 3: Implement the recurrence engine**

`apps/fleet-service/internal/maintenanceschedule/recurrence.go`:

```go
package maintenanceschedule

import "time"

// Schedule is the pure input to recurrence math (mirrors the entity's fields).
type Schedule struct {
	RecurrenceType       string // time | mileage | hybrid
	IntervalMonths       int
	IntervalMiles        int
	LastCompletedDate    time.Time
	LastCompletedMileage int
}

type Thresholds struct {
	DueSoonDays  int
	DueSoonMiles int
}

// DefaultThresholds: 30 days / 500 mi (design §10.1, FR-STATUS-2). Configurable.
var DefaultThresholds = Thresholds{DueSoonDays: 30, DueSoonMiles: 500}

func NextDue(s Schedule) (nextDate time.Time, nextMiles int) {
	if s.RecurrenceType == "time" || s.RecurrenceType == "hybrid" {
		nextDate = s.LastCompletedDate.AddDate(0, s.IntervalMonths, 0)
	}
	if s.RecurrenceType == "mileage" || s.RecurrenceType == "hybrid" {
		nextMiles = s.LastCompletedMileage + s.IntervalMiles
	}
	return
}

// DueState classifies a schedule given today + current mileage (design §10.1).
func DueState(s Schedule, today time.Time, currentMileage int, th Thresholds) string {
	nd, nm := NextDue(s)
	timed := s.RecurrenceType == "time" || s.RecurrenceType == "hybrid"
	miled := s.RecurrenceType == "mileage" || s.RecurrenceType == "hybrid"

	if timed && today.After(nd) {
		return "overdue"
	}
	if miled && currentMileage > nm {
		return "overdue"
	}
	if timed && !today.Before(nd.AddDate(0, 0, -th.DueSoonDays)) {
		return "upcoming"
	}
	if miled && currentMileage >= nm-th.DueSoonMiles {
		return "upcoming"
	}
	return "ok"
}

func Severity(state string) string {
	switch state {
	case "overdue":
		return "urgent"
	case "upcoming":
		return "recommended"
	default:
		return "informational"
	}
}
```

- [ ] **Step 4: Run (pass) + commit**

Run: `go test -race ./apps/fleet-service/internal/maintenanceschedule/...`

```bash
git add apps/fleet-service && git commit -m "feat(fleet): maintenance recurrence engine (next-due, due-state, severity)"
```

### Task 9.3: `maintenancerecord` + `maintenanceschedule` domains + completion flow (§10.3)

**Files (canonical template, both domains):** `maintenancerecord/{...}.go`, `maintenanceschedule/{model,entity,builder,provider,processor,administrator,rest,resource}.go` (recurrence.go already present); Test: `maintenanceschedule/completion_test.go`.

**Tables** `fleet.maintenance_records`, `fleet.maintenance_record_documents`, `fleet.maintenance_schedules` (PRD §6). **Endpoints:** `GET/POST /vehicles/{id}/maintenance-records`, `GET/PATCH/DELETE /maintenance-records/{id}`; `GET/POST /vehicles/{id}/maintenance-schedules`, `GET/PATCH/DELETE /maintenance-schedules/{id}`, `POST /maintenance-schedules/{id}/complete`; queues `GET /fleets/{id}/maintenance/upcoming`, `GET /fleets/{id}/maintenance/overdue`.

- [ ] **Step 1: Failing test — completion creates a pre-filled record, appends mileage, advances last_completed, recomputes next-due (orchestration in the processor, design §10.3)**

`maintenanceschedule/completion_test.go`:

```go
package maintenanceschedule

import (
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

type captureCompletion struct {
	recordCreated bool
	mileageAppended bool
	lastDate      time.Time
	lastMiles     int
}

func (c *captureCompletion) CreateRecord(vehicleID, categoryID string, date time.Time, miles int) (string, error) {
	c.recordCreated = true
	return "rec-1", nil
}
func (c *captureCompletion) AppendMileage(vehicleID string, miles int, src, ref string) error {
	c.mileageAppended = true
	return nil
}
func (c *captureCompletion) AdvanceSchedule(scheduleID string, date time.Time, miles int) error {
	c.lastDate, c.lastMiles = date, miles
	return nil
}

func TestComplete_orchestratesRecordMileageAndAdvance(t *testing.T) {
	cap := &captureCompletion{}
	p := NewCompletionProcessor(logrus.New(), cap)
	at := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	out, err := p.Complete(CompletionInput{
		ScheduleID: "s1", VehicleID: "v1", CategoryID: "c1", Date: at, LatestMileage: 42000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !cap.recordCreated || !cap.mileageAppended {
		t.Fatal("completion must create a record and append mileage")
	}
	if !cap.lastDate.Equal(at) || cap.lastMiles != 42000 {
		t.Fatalf("schedule must advance to completion point; got %v/%d", cap.lastDate, cap.lastMiles)
	}
	if out.MaintenanceRecordID != "rec-1" {
		t.Fatalf("want created record id, got %q", out.MaintenanceRecordID)
	}
}
```

- [ ] **Step 2: Run (fail) → Step 3: implement**

`CompletionInput`, `CompletionOutput{MaintenanceRecordID string}`, a `Completion` interface (`CreateRecord`, `AppendMileage`, `AdvanceSchedule`), and `CompletionProcessor.Complete` orchestrating the sequence (design §10.3): create pre-filled record → append mileage (source=`maintenance`, ref=record id) → advance schedule (`last_completed_{date,mileage}`) → recompute next-due via `NextDue` → (event emission stubbed via NoopProducer; wired in Phase 11) → activity append (Phase 11). The administrator runs the DB writes in one transaction.

- [ ] **Step 4: Implement the upcoming/overdue queue providers** — query schedules joined to vehicles in the fleet, compute `DueState` per schedule, filter to `upcoming`/`overdue`, return with severity. Paged via `server.Page`.

- [ ] **Step 5: Recurrence recompute job** — `jobs.Every(ctx, 1h, ...)` under advisory lock recomputes `status`/`severity`/`next_due_*` for all active schedules (FR-MAINT-6); also invoked on mileage change.

- [ ] **Step 6: Gate + commit**

Run: `go test -race ./apps/fleet-service/internal/maintenancerecord/... ./apps/fleet-service/internal/maintenanceschedule/...`

```bash
git add apps/fleet-service && git commit -m "feat(fleet): maintenance records + schedules + completion flow + queues + recompute job"
```

---

## Phase 10 — `fleet-service`: fuel

Implements design §8.2 (fuel) and §10.5 (price derivation). Logging fuel orchestrates a mileage append (cross-domain orchestration in the processor, design §6).

### Task 10.1: `fuel` domain — price derivation + fuel→mileage orchestration

**Files (canonical template):** `apps/fleet-service/internal/fuel/{...}.go`; Test: `processor_test.go`.

**Table** `fleet.fuel_logs` (PRD §6): id, vehicle_id idx, date, mileage, gallons, total_cost, price_per_gallon, created_by_user_id, timestamps, deleted_at. **Endpoints:** `GET/POST /vehicles/{id}/fuel-logs`, `GET/PATCH/DELETE /fuel-logs/{id}`.

- [ ] **Step 1: Failing test — price derivation (§10.5) + fuel append triggers a mileage record**

`apps/fleet-service/internal/fuel/processor_test.go`:

```go
package fuel

import (
	"errors"
	"testing"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

func TestDerivePrice(t *testing.T) {
	// price omitted → total/gallons
	if got, err := DerivePrice(0, 40.0, 10.0); err != nil || got.PricePerGallon != 4.0 {
		t.Fatalf("want 4.0/gal, got %v err %v", got.PricePerGallon, err)
	}
	// total omitted → price*gallons
	if got, err := DerivePrice(4.0, 0, 10.0); err != nil || got.TotalCost != 40.0 {
		t.Fatalf("want total 40.0, got %v err %v", got.TotalCost, err)
	}
	// neither derivable → 422
	if _, err := DerivePrice(0, 0, 10.0); !errors.Is(err, server.ErrValidation) {
		t.Fatalf("missing both must be 422, got %v", err)
	}
	// zero gallons → 422 (no divide-by-zero)
	if _, err := DerivePrice(0, 40.0, 0); !errors.Is(err, server.ErrValidation) {
		t.Fatalf("zero gallons must be 422, got %v", err)
	}
}
```

- [ ] **Step 2: Run (fail) → Step 3: implement**

```go
package fuel

import "github.com/jtumidanski/myfleet/packages/shared-go/server"

type Derived struct {
	PricePerGallon float64
	TotalCost      float64
}

// DerivePrice fills the missing of {price_per_gallon, total_cost} (design §10.5).
func DerivePrice(price, total, gallons float64) (Derived, error) {
	if gallons <= 0 {
		return Derived{}, server.ErrValidation
	}
	switch {
	case price > 0 && total > 0:
		return Derived{PricePerGallon: price, TotalCost: total}, nil
	case total > 0:
		return Derived{PricePerGallon: total / gallons, TotalCost: total}, nil
	case price > 0:
		return Derived{PricePerGallon: price, TotalCost: price * gallons}, nil
	default:
		return Derived{}, server.ErrValidation
	}
}
```

The fuel administrator: derive price → insert fuel log → call `mileage` processor `OnAppend` (source=`fuel`, ref=fuel log id) in the same transaction → emit `fuel.logged` (NoopProducer for now). This is the canonical cross-domain orchestration (design §8.2).

- [ ] **Step 4: Gate + commit**

Run: `go test -race ./apps/fleet-service/internal/fuel/...`

```bash
git add apps/fleet-service && git commit -m "feat(fleet): fuel logs — price derivation + fuel→mileage orchestration"
```

---

## Phase 11 — `fleet-service`: status derivation, activity, event production + outbox

Implements design §10.2 (status), §8.2 (activity), §7 + A8 (real event production via outbox). This phase **replaces the NoopProducer** wired in earlier phases with the Kafka producer + outbox relay.

### Task 11.1: Vehicle status derivation (§10.2)

**Files:** `apps/fleet-service/internal/status/derive.go`; Test: `derive_test.go`. (Pure function.)

- [ ] **Step 1: Failing test — highest-priority-wins across Overdue/Upcoming/Inactive/Healthy**

`apps/fleet-service/internal/status/derive_test.go`:

```go
package status

import (
	"testing"
	"time"
)

func TestDerive(t *testing.T) {
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name           string
		scheduleStates []string
		lastActivity   time.Time
		want           string
	}{
		{"overdue wins", []string{"overdue", "upcoming"}, now, "Overdue"},
		{"upcoming next", []string{"ok", "upcoming"}, now, "Upcoming Maintenance"},
		{"inactive when stale", []string{"ok"}, now.AddDate(-1, 0, -1), "Inactive"},
		{"healthy otherwise", []string{"ok"}, now, "Healthy"},
		{"no schedules + recent → healthy", nil, now, "Healthy"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Derive(Input{ScheduleStates: c.scheduleStates, LastActivityAt: c.lastActivity, Now: now, InactivityDays: 365})
			if got != c.want {
				t.Fatalf("Derive=%s want %s", got, c.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run (fail) → Step 3: implement (priority order per design §10.2)**

```go
// Package status derives a vehicle's status on read (design §10.2); never stored.
package status

import "time"

type Input struct {
	ScheduleStates []string // each "ok"|"upcoming"|"overdue"
	LastActivityAt time.Time
	Now            time.Time
	InactivityDays int // default 365
}

func Derive(in Input) string {
	for _, s := range in.ScheduleStates {
		if s == "overdue" {
			return "Overdue"
		}
	}
	for _, s := range in.ScheduleStates {
		if s == "upcoming" {
			return "Upcoming Maintenance"
		}
	}
	if in.LastActivityAt.Before(in.Now.AddDate(0, 0, -in.InactivityDays)) {
		return "Inactive"
	}
	return "Healthy"
}
```

- [ ] **Step 4: Wire status into vehicle `rest.go`** — `Transform` for a vehicle computes status from its schedules' `DueState` + last activity, exposing it as a read-only attribute. Gate + commit.

```bash
go test -race ./apps/fleet-service/internal/status/...
git add apps/fleet-service && git commit -m "feat(fleet): vehicle status derivation (read-only, priority order)"
```

### Task 11.2: `activity` domain — append-only feed + per-vehicle timeline (FR-ACT)

**Files (canonical template):** `apps/fleet-service/internal/activity/{...}.go`; Test: `processor_test.go`.

**Table** `fleet.activity_events` (PRD §6): id, fleet_id idx, vehicle_id nullable idx, actor_user_id, type, payload jsonb, created_at. Append-only. **Endpoints:** `GET /fleets/{id}/activity`, `GET /vehicles/{id}/activity`. A small `Record(actor, type, fleetID, vehicleID?, payload)` helper the other processors call (vehicle.created, maintenance.completed, fuel.logged, member.invited, schedule.overdue — FR-ACT-1).

- [ ] **Step 1: Failing test — `Record` builds a well-formed event with actor/type/fleet** (fake writer captures the row; assert fields + that payload is JSON-serializable).
- [ ] **Step 2–3:** Run (fail) → implement the domain + `Record` helper + paged providers.
- [ ] **Step 4: Wire `activity.Record` calls** into vehicle-create, completion, fuel-log, invite-accept, schedule-overdue-transition processors. Gate + commit.

```bash
go test -race ./apps/fleet-service/internal/activity/...
git add apps/fleet-service && git commit -m "feat(fleet): activity feed + per-vehicle timeline + Record helper wiring"
```

### Task 11.3: Real event production via Kafka producer + outbox relay (A8)

**Files:** `apps/fleet-service/internal/events/emit.go` (thin wrapper building `events.Envelope` from `dto-go` payloads); Modify: `cmd/main.go`; Test: `emit_test.go`.

- [ ] **Step 1: Failing test — `EmitVehicleCreated` enqueues an outbox row with the right type + payload** (sqlite + `events.MigrateOutbox`; call emit within a tx; assert one unsent outbox row of type `vehicle.created` whose decoded `data` matches `dto-go` `VehicleCreatedData`).

`apps/fleet-service/internal/events/emit_test.go` asserts `events.Enqueue` was used (one row, `Type=="vehicle.created"`, `sent_at IS NULL`).

- [ ] **Step 2: Run (fail) → Step 3: implement**

`emit.go` exposes `EmitVehicleCreated(tx, fleetID, actorID, traceID string, d dtoevents.VehicleCreatedData) error` etc., each building an `events.Envelope{EventID: uuid, Type: "...", Version: 1, OccurredAt: now, FleetID, ActorUserID, TraceID, Data: structToMap(d)}` and calling `events.Enqueue(tx, env)`. Replace the NoopProducer calls in vehicle/completion/fuel/invite/schedule administrators with `EmitX` inside their existing transactions (design A8 — domain write + outbox row in one tx).

- [ ] **Step 4: Start the outbox relay + schedule.overdue emission**

In `cmd/main.go`: construct `events.NewKafkaProducer(brokers)`; start `jobs.Every(ctx, 2s, func(ctx){ database.WithLeaderLock(db, "fleet-outbox", func() error { return events.RelayOnce(ctx, log, db, producer) }) })`. The recurrence recompute job emits `schedule.overdue` (via outbox) when a schedule transitions into `overdue`.

- [ ] **Step 5: Gate + verify compose + commit**

Run: `go build ./apps/fleet-service/... && go vet ./apps/fleet-service/... && go test -race ./apps/fleet-service/...`

```bash
git add apps/fleet-service && git commit -m "feat(fleet): Kafka producer + outbox relay + domain event emission (A8)"
```

---

## Phase 12 — `notification-service`

Implements design §8.4, §7 (idempotent consumers), §11 (daily reminder job), A6 (dedupe). Module: `github.com/jtumidanski/myfleet/apps/notification-service`. Path `/api/notifications/*`. Owns `notification.notifications`, `notification.notification_preferences`, `notification.processed_events`.

### Task 12.1: Module bootstrap + `notification` domain + dedupe (A6)

**Files (canonical template):** `apps/notification-service/internal/notification/{...}.go`; `apps/notification-service/internal/inbox/processed.go` (processed_events helper); Test: `notification/processor_test.go`.

**Tables** (PRD §6): `notifications` (…, dedupe_key unique per trigger, read_at), `notification_preferences` (user_id, type, in_app_enabled, unique(user_id,type)), `processed_events` (event_id PK, consumer, processed_at). **Endpoints:** `GET /notifications` (filter read/type, paged), `POST /notifications/{id}/read`, `POST /notifications/read-all`, `GET/PUT /notification-preferences`.

- [ ] **Step 1: Init module + deps + workspace** (mirror media bootstrap; add `dto-go`).

- [ ] **Step 2: Failing test — generation is idempotent: same dedupe_key inserts once; respects per-user/per-type preference (FR-NOTIF-2/3)**

`notification/processor_test.go`:

```go
package notification

import (
	"testing"

	"github.com/sirupsen/logrus"
)

type fakeStore struct {
	existing map[string]bool // dedupe_key set
	inserts  int
}

func (f *fakeStore) ExistsByDedupeKey(k string) (bool, error) { return f.existing[k], nil }
func (f *fakeStore) Insert(n Model) error                     { f.inserts++; f.existing[n.DedupeKey()] = true; return nil }

type fakePrefs struct{ enabled map[string]bool }

func (f fakePrefs) Enabled(userID, typ string) (bool, error) { return f.enabled[typ], nil }

func TestGenerate_dedupesByKey(t *testing.T) {
	st := &fakeStore{existing: map[string]bool{}}
	p := NewProcessor(logrus.New(), st, fakePrefs{enabled: map[string]bool{"overdue": true}})
	in := GenerateInput{UserID: "u1", Type: "overdue", DedupeKey: "overdue:sched1:cycle3", Title: "Overdue"}
	_ = p.Generate(in)
	_ = p.Generate(in) // redelivery
	if st.inserts != 1 {
		t.Fatalf("dedupe_key must insert once, got %d", st.inserts)
	}
}

func TestGenerate_skipsWhenPreferenceDisabled(t *testing.T) {
	st := &fakeStore{existing: map[string]bool{}}
	p := NewProcessor(logrus.New(), st, fakePrefs{enabled: map[string]bool{"overdue": false}})
	_ = p.Generate(GenerateInput{UserID: "u1", Type: "overdue", DedupeKey: "k", Title: "x"})
	if st.inserts != 0 {
		t.Fatal("disabled preference must suppress generation")
	}
}
```

- [ ] **Step 3: Run (fail) → Step 4: implement**

`Generate(in)` checks preference (`Enabled`) → if disabled, return nil; check `ExistsByDedupeKey` → if exists, no-op; else `Insert`. `dedupe_key` is unique per trigger+due-cycle (design A6). Implement the domain CRUD + preference endpoints.

- [ ] **Step 5: Gate + commit**

Run: `go test -race ./apps/notification-service/internal/notification/...`

```bash
git add apps/notification-service go.work && git commit -m "feat(notification): module + notification domain, dedupe, preferences"
```

### Task 12.2: Idempotent event consumers + daily reminder safety-net job + wiring

**Files:** `apps/notification-service/internal/consumer/consume.go`; `apps/notification-service/internal/reminder/job.go`; `apps/notification-service/cmd/main.go`; `Dockerfile`; Test: `consumer/consume_test.go`.

- [ ] **Step 1: Failing test — consumer records processed event_id; redelivery of the same event_id is a no-op (design §7)**

`consumer/consume_test.go`: a fake `processed_events` store; calling the consumer handler twice with the same `event_id` invokes generation once.

- [ ] **Step 2–3:** Run (fail) → implement the handler: on each event, `processed.Seen(event_id, "notification")` → if seen, ack/skip; else map event (`schedule.overdue`→overdue notif, `maintenance.completed`/`fuel.logged`/`vehicle.created`/`member.invited`→activity notifs) to `Generate`, then `processed.Mark`. Resolve recipient users from the event's `fleet_id` via a fleet-service internal members client (id-ref, D2).

- [ ] **Step 4: Daily reminder job (A6 safety-net)** — `jobs.Every(ctx, 24h, ...)` under advisory lock re-derives upcoming/overdue from fleet-service (queue endpoints) and calls `Generate` with the same `dedupe_key` scheme so it can't double-fire.

- [ ] **Step 5: Wire `cmd/main.go`** — connect DB (migrations incl. processed_events), start consumer goroutines for the subscribed topics (`events.Consume`), start the reminder job, serve notification routes behind JWT. Dockerfile mirrors others; add `notification-service` to compose (Kafka env, Traefik `PathPrefix(/api/notifications)`).

- [ ] **Step 6: Gate + verify compose + commit**

Run: `go build ./apps/notification-service/... && go vet ./apps/notification-service/... && go test -race ./apps/notification-service/... && (cd deploy/compose && docker compose config >/dev/null && echo OK)`

```bash
git add apps/notification-service deploy/compose/docker-compose.yml && git commit -m "feat(notification): idempotent consumers + reminder job + wiring + compose"
```

---

## Phase 13 — `fleet-service`: dashboard widget system

Implements design §12 (dashboard) and §8.2 (dashboard). Per-user, per-fleet widget layout; aggregations computed on read (A5).

### Task 13.1: `dashboard` domain — per-user layout persistence

**Files (canonical template):** `apps/fleet-service/internal/dashboard/{...}.go`; Test: `processor_test.go`.

**Tables** `fleet.dashboards` (unique(fleet_id,user_id)), `fleet.dashboard_widgets` (dashboard_id idx, type, position_x/y, width, height, config jsonb). **Endpoints:** `GET/PUT /fleets/{id}/dashboard` (per-user — keyed by `(fleet_id, identity.UserID)`). PUT replaces the widget set transactionally; widget `type` validated against the known catalog (design §12).

- [ ] **Step 1: Failing test — PUT validates widget types against the catalog; unknown type → 422; layout is per-user**

`processor_test.go`: `TestValidateLayout_rejectsUnknownWidget` — a layout containing `type:"crypto-prices"` returns `server.ErrValidation`; a layout of only catalog types passes. Catalog: `fleet-overview`, `vehicle-status`, `upcoming-maintenance`, `overdue-maintenance`, `recent-activity`, `spend-by-vehicle`, `mileage-trends`.

- [ ] **Step 2–3:** Run (fail) → implement `ValidCatalog` set + `ValidateLayout` + domain CRUD (upsert dashboard by (fleet,user), replace widgets in a tx).

- [ ] **Step 4: Gate + commit**

```bash
go test -race ./apps/fleet-service/internal/dashboard/...
git add apps/fleet-service && git commit -m "feat(fleet): dashboard layout persistence + widget catalog validation"
```

### Task 13.2: Aggregation read endpoints (A5) — spend-by-vehicle, mileage-trends, overview

**Files:** `apps/fleet-service/internal/dashboard/aggregate.go`; Test: `aggregate_test.go`.

**Endpoints (read, computed on read, A5):** `GET /fleets/{id}/dashboard/spend-by-vehicle?from=&to=`, `GET /vehicles/{id}/dashboard/mileage-trends`, `GET /fleets/{id}/dashboard/overview` (counts by status, upcoming/overdue counts). Indexed queries (design §13).

- [ ] **Step 1: Failing test — spend-by-vehicle sums maintenance.cost + fuel.total_cost per vehicle within range**

`aggregate_test.go` (sqlite with seeded maintenance_records + fuel_logs): assert the aggregate groups by vehicle and sums both sources within `[from,to]`.

- [ ] **Step 2–3:** Run (fail) → implement the aggregation queries (GROUP BY vehicle_id, SUM over the window; union maintenance + fuel costs). Bounded windows (design A5/§20).

- [ ] **Step 4: Gate + commit**

```bash
go test -race ./apps/fleet-service/internal/dashboard/...
git add apps/fleet-service && git commit -m "feat(fleet): dashboard aggregation endpoints (spend, trends, overview) computed-on-read"
```

---

## Phase 14 — `apps/web`: shell, auth, API client, canonical feature

Implements design §12. Establishes the React/TS/Vite app, the auth context + API client wiring, routing, and **one canonical feature (vehicles)** in full — every later feature mirrors it. **Re-read `frontend-dev-guidelines` at the start of this phase**; it governs query-key factories, the BaseService pattern, Zod schemas in `lib/schemas/`, skeletons-not-spinners, and `createErrorFromUnknown` + sonner.

### Task 14.1: Vite + TS + Tailwind + shadcn scaffold, providers, router

**Files:**
- Create: `apps/web/package.json`, `apps/web/tsconfig.json`, `apps/web/vite.config.ts`, `apps/web/index.html`
- Create: `apps/web/src/main.tsx`, `apps/web/src/App.tsx`
- Create: `apps/web/src/components/providers/AppProviders.tsx`
- Create: `apps/web/src/lib/api/client.ts`
- Create: `apps/web/tailwind.config.ts`, `apps/web/src/index.css`
- Test: `apps/web/src/lib/api/client.test.ts`

- [ ] **Step 1: Create `package.json`** (strict TS, React 18, RQ, RHF+Zod, sonner, vitest+RTL):

`apps/web/package.json`:

```json
{
  "name": "@myfleet/web",
  "private": true,
  "type": "module",
  "scripts": {
    "dev": "vite",
    "build": "tsc -b && vite build",
    "test": "vitest run",
    "lint": "eslint src --max-warnings 0"
  },
  "dependencies": {
    "@myfleet/shared-ts": "*",
    "@myfleet/ui-components": "*",
    "@tanstack/react-query": "^5.51.0",
    "react": "^18.3.1",
    "react-dom": "^18.3.1",
    "react-hook-form": "^7.52.0",
    "react-router-dom": "^6.25.0",
    "sonner": "^1.5.0",
    "zod": "^3.23.0",
    "@hookform/resolvers": "^3.9.0"
  },
  "devDependencies": {
    "@testing-library/react": "^16.0.0",
    "@testing-library/jest-dom": "^6.4.0",
    "@vitejs/plugin-react": "^4.3.0",
    "autoprefixer": "^10.4.0",
    "eslint": "^9.0.0",
    "jsdom": "^24.0.0",
    "tailwindcss": "^3.4.0",
    "postcss": "^8.4.0",
    "typescript": "^5.5.0",
    "vite": "^5.3.0",
    "vitest": "^2.0.0"
  }
}
```

- [ ] **Step 2: tsconfig (strict, no `any`), vite config (proxy `/api`→`http://localhost` for dev), tailwind, index.html, index.css.** `tsconfig.json` sets `"strict": true`, `"noUncheckedIndexedAccess": true`. `vite.config.ts` adds `@vitejs/plugin-react` and a dev `server.proxy` for `/api`.

- [ ] **Step 3: Write the failing test for the configured API client**

`apps/web/src/lib/api/client.test.ts`:

```ts
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { apiClient } from './client';

describe('apiClient', () => {
  beforeEach(() => vi.restoreAllMocks());

  it('attaches the bearer token from the token store', async () => {
    localStorage.setItem('access_token', 'tok-123');
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify({ data: { id: '1', type: 'vehicles', attributes: {} } }), { status: 200 }),
    );
    await apiClient.request('/api/fleet/vehicles/1');
    const headers = (fetchMock.mock.calls[0]![1]!.headers ?? {}) as Record<string, string>;
    expect(headers.Authorization).toBe('Bearer tok-123');
  });
});
```

- [ ] **Step 4: Run (fail) → implement client + providers + router shell**

`apps/web/src/lib/api/client.ts` constructs the `shared-ts` `ApiClient` with `baseUrl: ''`, `getAccessToken: () => localStorage.getItem('access_token')`, `onRefresh` calling `POST /api/auth/refresh` and storing the new token. `AppProviders.tsx` wraps `QueryClientProvider` + auth context + `<Toaster/>`. `App.tsx` defines routes (login, onboarding, dashboard, vehicles, vehicle detail, maintenance, fuel, activity, notifications, settings) with an auth guard.

- [ ] **Step 5: Run test + build, commit**

Run: `npm install && npm run -w @myfleet/web test && npm run -w @myfleet/web build`
Expected: PASS, build clean.

```bash
git add apps/web package-lock.json && git commit -m "feat(web): Vite+TS+Tailwind scaffold, providers, router, API client"
```

### Task 14.2: Auth context + login + onboarding flow (FR-AUTH, FR-FLEET-1)

**Files:**
- Create: `apps/web/src/context/AuthContext.tsx`, `apps/web/src/lib/hooks/api/auth.ts`
- Create: `apps/web/src/pages/LoginPage.tsx`, `apps/web/src/pages/OnboardingPage.tsx`
- Create: `apps/web/src/components/RequireAuth.tsx`
- Test: `apps/web/src/context/AuthContext.test.tsx`

- [ ] **Step 1: Failing test — `RequireAuth` redirects unauthenticated users to /login; renders children when authed** (RTL with a `MemoryRouter` + mocked auth context).
- [ ] **Step 2–3:** Run (fail) → implement: `AuthContext` exposes `{ user, activeFleetId, role, isAuthenticated, login(), logout() }` backed by `GET /api/auth/me`; `LoginPage` links to `GET /api/auth/login/google`; `OnboardingPage` posts `POST /api/fleet/fleets` (RHF + Zod schema in `lib/schemas/`) then routes to dashboard; `RequireAuth` gates routes and redirects to onboarding when `activeFleetId` is null.
- [ ] **Step 4: Build + test + commit.**

```bash
npm run -w @myfleet/web test && npm run -w @myfleet/web build
git add apps/web && git commit -m "feat(web): auth context, Google login, onboarding (create fleet)"
```

### Task 14.3: Canonical feature — vehicles (BaseService, query hooks, list/detail/forms)

This is the **reference feature**; Phase 15 features copy its structure.

**Files:**
- Create: `apps/web/src/services/api/BaseService.ts`, `apps/web/src/services/api/VehicleService.ts`
- Create: `apps/web/src/lib/hooks/api/vehicles.ts` (query-key factory + queries + mutations)
- Create: `apps/web/src/lib/schemas/vehicle.ts` (Zod)
- Create: `apps/web/src/types/models/vehicle.ts`
- Create: `apps/web/src/pages/VehiclesPage.tsx`, `apps/web/src/pages/VehicleDetailPage.tsx`
- Create: `apps/web/src/components/features/vehicles/{VehicleList,VehicleForm,VehicleCard}.tsx`
- Test: `apps/web/src/lib/hooks/api/vehicles.test.ts`

- [ ] **Step 1: Failing test — the query-key factory is hierarchical + `as const`, and the list query calls the service**

`apps/web/src/lib/hooks/api/vehicles.test.ts`:

```ts
import { describe, it, expect } from 'vitest';
import { vehicleKeys } from './vehicles';

describe('vehicleKeys', () => {
  it('is hierarchical', () => {
    expect(vehicleKeys.all).toEqual(['vehicles']);
    expect(vehicleKeys.list({ fleetId: 'f1' })).toEqual(['vehicles', 'list', { fleetId: 'f1' }]);
    expect(vehicleKeys.detail('v1')).toEqual(['vehicles', 'detail', 'v1']);
  });
});
```

- [ ] **Step 2: Run (fail) → Step 3: implement**

`BaseService.ts` wraps `apiClient` with `list/get/create/patch/remove` returning typed JSON:API. `VehicleService.ts` extends it for `/api/fleet/vehicles`. `vehicles.ts`:

```ts
export const vehicleKeys = {
  all: ['vehicles'] as const,
  list: (params: { fleetId: string }) => ['vehicles', 'list', params] as const,
  detail: (id: string) => ['vehicles', 'detail', id] as const,
};
```

plus `useVehicles(fleetId)`, `useVehicle(id)`, `useCreateVehicle()`, `useUpdateVehicle()`, `useSoftDeleteVehicle()`, `useRestoreVehicle()` — mutations invalidate `vehicleKeys.all`/`detail`. `vehicle.ts` schema validates make/model/year required. Pages render skeletons while loading; forms use `zodResolver`; errors via `createErrorFromUnknown` → `toast.error`. Role-gated UI: hide delete/restore for viewers, restore for non-owners (server still enforces).

- [ ] **Step 4: Run test + build, commit**

Run: `npm run -w @myfleet/web test && npm run -w @myfleet/web build`

```bash
git add apps/web && git commit -m "feat(web): vehicles feature (canonical: BaseService, query hooks, list/detail/forms)"
```

---

## Phase 15 — `apps/web`: remaining feature areas

Each feature **mirrors the vehicles pattern** (Service → query-key factory + hooks → Zod schema → page + feature components → skeleton/error handling → role-gated UI). For each: write the failing query-key/hook test first, then implement, then `npm run -w @myfleet/web test && build`, then commit. Re-read `frontend-dev-guidelines` before each.

### Task 15.1: Vehicle media gallery + upload + primary selection (FR-VEH-3, FR-MEDIA)

Files under `components/features/vehicles/media/` + `services/api/MediaService.ts` + `lib/hooks/api/media.ts`. Flow (design §8.3): `POST /api/media` (init) → PUT bytes to presigned URL → `POST /api/media/{id}/confirm` → poll metadata until `ready` (show "processing" state) → `PUT /api/fleet/vehicles/{id}/primary-image`. Gallery shows thumbnails (resolved via `GET /api/media/{id}/download`). Test: upload hook sequences init→PUT→confirm. Commit `feat(web): vehicle media gallery + upload + primary image`.

### Task 15.2: Mileage history + trend graph + auto-fill (FR-MILE)

Service/hooks for `/api/fleet/vehicles/{id}/mileage`. A trend graph component (lightweight SVG/`recharts`) over the chronological series; latest mileage auto-fills maintenance/fuel forms (read from `useVehicle`). Test: the auto-fill helper picks the latest record's mileage. Commit `feat(web): mileage history, trend graph, latest-mileage auto-fill`.

### Task 15.3: Maintenance — records, schedules, upcoming/overdue queues, complete action (FR-MAINT)

Services/hooks for maintenance-records, maintenance-schedules, categories, queues. Record form (category dropdown from `GET /maintenance-categories`, receipt attachments via MediaService). Schedule form (recurrence type → conditional months/miles fields via RHF). Upcoming/overdue queue views with `SeverityChip`. "Complete" button → `POST /maintenance-schedules/{id}/complete` → invalidate records + schedules + vehicle status. Test: schedule Zod schema requires the right interval fields per recurrence type. Commit `feat(web): maintenance records, schedules, queues, completion`.

### Task 15.4: Fuel logs (FR-FUEL)

Service/hooks for `/api/fleet/vehicles/{id}/fuel-logs`. Form captures date/mileage/gallons/total/price; client allows omitting one of price/total (server derives, §10.5). Test: fuel Zod schema permits either price or total but requires gallons. Commit `feat(web): fuel logging`.

### Task 15.5: Activity feed + per-vehicle timeline (FR-ACT)

Services/hooks for `/api/fleet/fleets/{id}/activity` and `/api/fleet/vehicles/{id}/activity`. Paginated feed with event-type icons. Test: feed hook paginates via `meta`. Commit `feat(web): activity feed + per-vehicle timeline`.

### Task 15.6: Notifications + preferences (FR-NOTIF)

Service/hooks for `/api/notifications` (filter read/type), mark-read, read-all, and `/api/notifications/notification-preferences`. A notification bell with unread count; a preferences page (per-type toggles). Test: unread-count selector. Commit `feat(web): notifications + per-type preferences`.

### Task 15.7: Dashboard widget system (FR-DASH)

A widget registry (`type → component + default size`), a grid supporting add/remove/reorder/resize, layout persisted via `GET/PUT /api/fleet/fleets/{id}/dashboard`. Each widget fetches its own data via its query hook; aggregation widgets (spend-by-vehicle, mileage-trends) call the Phase 13 endpoints with selectable ranges. Catalog matches design §12. Test: the registry maps every catalog `type` to a component (no missing entries). Commit `feat(web): customizable dashboard widget system`.

### Task 15.8: Fleet settings + member management (FR-FLEET-2/4)

Rename fleet (owner-only), members list, remove member (owner-only, sole-owner guard surfaced as a toast on 409), invites (create with role+expiry, revoke, accept page at `/invites/{token}/accept`). Role-gated UI throughout. Test: settings hooks invalidate fleet/member keys. Commit `feat(web): fleet settings, member management, invites`.

- [ ] **Phase 15 gate:** `npm run -w @myfleet/web test && npm run -w @myfleet/web build` clean after every task above.

---

## Phase 16 — CI/CD: GitHub Actions + Gitleaks + Renovate

Implements design §17. PR workflow gates every change; main workflow publishes images. Verified with `act` or by pushing a branch.

### Task 16.1: PR workflow (build, TS, Go tests, container build, Gitleaks, formatting)

**Files:** `.github/workflows/pr.yml`; `.gitleaks.toml`.

- [ ] **Step 1: Write `.github/workflows/pr.yml`**

```yaml
name: pr
on:
  pull_request:
    branches: [main]
jobs:
  backend:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: '1.24' }
      - run: go vet ./...
      - run: go build ./...
      - run: go test -race ./...
      - run: gofmt -l . | tee /tmp/fmt && test ! -s /tmp/fmt
  frontend:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        with: { node-version: '20' }
      - run: npm ci
      - run: npm run test
      - run: npm run build
  containers:
    runs-on: ubuntu-latest
    strategy:
      matrix: { service: [auth-service, fleet-service, media-service, notification-service] }
    steps:
      - uses: actions/checkout@v4
      - run: docker build -f apps/${{ matrix.service }}/Dockerfile -t myfleet-${{ matrix.service }}:ci .
  gitleaks:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with: { fetch-depth: 0 }
      - uses: gitleaks/gitleaks-action@v2
        env: { GITLEAKS_LICENSE: "" }
```

- [ ] **Step 2: Write `.gitleaks.toml`** extending defaults; ensure it scans source, YAML, Dockerfiles, workflow files, env config (design NFR §8).

- [ ] **Step 3: Verify the workflow is valid YAML**

Run: `python3 -c "import yaml,sys; yaml.safe_load(open('.github/workflows/pr.yml'))" && echo OK`
Expected: `OK`.

- [ ] **Step 4: Commit**

```bash
git add .github/workflows/pr.yml .gitleaks.toml
git commit -m "ci: PR workflow (vet/build/test, fe build/test, container build, gitleaks)"
```

### Task 16.2: Main workflow (GHCR publish, tagging, vuln scan) + Renovate

**Files:** `.github/workflows/main.yml`; `renovate.json`.

- [ ] **Step 1: Write `main.yml`** — on push to `main`: build all, then per-service `docker/build-push-action` publishing `ghcr.io/jtumidanski/myfleet-<service>:${{ github.sha }}` and `:latest`, plus a Trivy vuln scan step and version tagging.

- [ ] **Step 2: Write `renovate.json`** (design §17): monorepo-aware, grouped compatible updates, `minimumReleaseAge: "7 days"`, `separateMajorMinor: true`, `automerge: false`; enable `gomod`, `npm`, `dockerfile`, `github-actions` managers.

- [ ] **Step 3: Verify YAML/JSON valid; commit**

Run: `python3 -c "import yaml; yaml.safe_load(open('.github/workflows/main.yml'))" && python3 -m json.tool renovate.json >/dev/null && echo OK`
Expected: `OK`.

```bash
git add .github/workflows/main.yml renovate.json
git commit -m "ci: main workflow (GHCR publish, tagging, vuln scan) + Renovate config"
```

---

## Phase 17 — k3s manifests & release hardening

Implements design §17. Kustomize base + overlays per service + infra, with probes, resource limits, and ConfigMap/Secret separation.

### Task 17.1: Kustomize base for one service + infra, then replicate

**Files:** `deploy/k8s/base/<service>/{deployment,service,configmap}.yaml`, `deploy/k8s/base/kustomization.yaml`, `deploy/k8s/overlays/local/kustomization.yaml`, `deploy/k8s/base/infra/{postgres,minio,redpanda,traefik}.yaml`.

- [ ] **Step 1: Write the canonical service Deployment** (auth-service) with: non-root `securityContext` (`runAsNonRoot: true`, `runAsUser: 10001`), `readinessProbe: /readyz`, `livenessProbe: /healthz`, resource requests/limits, env from a `ConfigMap` (non-secret) + `Secret` (DB URL, Google secret, JWT key), image `ghcr.io/jtumidanski/myfleet-auth-service`.

- [ ] **Step 2: Write `Service` + `ConfigMap`; a Traefik `IngressRoute`** routing path prefixes per design §2.

- [ ] **Step 3: Replicate the Deployment/Service/ConfigMap for fleet/media/notification** (adjust name, image, env, migrations). Add infra manifests (PostgreSQL with per-service schema init, MinIO, Redpanda, Traefik).

- [ ] **Step 4: Write overlays** (`local` overlay sets replicas=1, local image tags). 

- [ ] **Step 5: Verify manifests render + validate**

Run: `kubectl kustomize deploy/k8s/overlays/local >/tmp/rendered.yaml && kubectl apply --dry-run=client -f /tmp/rendered.yaml`
Expected: all resources `(dry run)` valid (requires a kube context; if none, `kubectl kustomize` rendering succeeding is the gate).

- [ ] **Step 6: Commit**

```bash
git add deploy/k8s
git commit -m "feat(deploy): k3s Kustomize base+overlays, probes, limits, ConfigMap/Secret split"
```

---

## Phase 18 — End-to-end acceptance verification

Implements design §16 e2e and the PRD §10 acceptance criteria. This phase writes no new product code; it brings the full stack up and verifies the acceptance path, fixing any integration gaps found (each fix is its own TDD commit in the relevant phase's style).

### Task 18.1: Compose up + full backend integration smoke

- [ ] **Step 1: Add the `web` service + remaining services to compose** with Traefik labels; web served at `/`, services at their `/api/*` prefixes (design §2). Build a production `apps/web/Dockerfile` (multi-stage: `node` build → static served by Traefik or an `nginx:alpine`/`caddy` static container behind Traefik).

- [ ] **Step 2: Bring the stack up**

Run: `make up` (or `scripts/dev-up.sh`); then `docker compose -f deploy/compose/docker-compose.yml ps`
Expected: traefik, postgres, minio, redpanda, auth/fleet/media/notification-service, web all `healthy`/`running`.

- [ ] **Step 3: Health gate across services**

Run: `for s in auth fleet media notifications; do curl -fsS http://localhost/api/$s/readyz || curl -fsS http://localhost/healthz; done`
Expected: each `/readyz` returns 200 (DB + deps healthy).

- [ ] **Step 4: Verify the acceptance path** (PRD §10) — script or manual checklist, each must pass:
  - [ ] Google sign-in provisions a user; app issues a JWT and refreshes it (FR-AUTH).
  - [ ] Create a fleet during onboarding → land on dashboard (FR-FLEET-1).
  - [ ] Invite by email (role+expiry); only matching-email accepts; expired/used rejected (FR-FLEET-3).
  - [ ] Owner renames fleet, removes member; viewer write → 403; member owner-only → 403; sole-owner self-removal → 409.
  - [ ] Add/edit/soft-delete a vehicle; owner restores within 5 days; purge after window (FR-VEH).
  - [ ] Upload multiple images, pick primary; thumbnail/display variants generated async (FR-MEDIA-4).
  - [ ] Mileage recorded from manual/fuel/maintenance; latest auto-fills; trend graph renders (FR-MILE).
  - [ ] Log maintenance with attachments; define time/mileage/hybrid schedules; upcoming+overdue queues populate with severity; complete → pre-filled record + recomputed next-due (FR-MAINT).
  - [ ] Log fuel → a mileage record is created; price/total derivation works (FR-FUEL).
  - [ ] Vehicle status derives Healthy/Upcoming/Overdue/Inactive correctly (FR-STATUS).
  - [ ] Activity feed + per-vehicle timeline capture the event types (FR-ACT).
  - [ ] In-app notifications fire for upcoming/overdue/activity, de-duplicated, respecting per-user/per-type prefs (FR-NOTIF).
  - [ ] Dashboard add/remove/reorder/resize widgets; layout persists per user (FR-DASH).

- [ ] **Step 5: Cross-cutting ops verification** (PRD §10 platform):
  - [ ] All services: multi-stage non-root images, `/healthz`+`/readyz`+`/metrics`, structured logs, OTel traces with correlation IDs propagated across HTTP **and** Kafka.
  - [ ] `kubectl kustomize deploy/k8s/overlays/local` renders; probes + limits + ConfigMap/Secret present.
  - [ ] CI PR + main workflows green on a test branch; Gitleaks passes; Renovate config validates.

- [ ] **Step 6: Full repo gate**

Run: `make ci` (`go vet ./... && go test -race ./... && go build ./... && npm run test && npm run build`)
Expected: all clean.

- [ ] **Step 7: Commit any integration fixes**

```bash
git add -A
git commit -m "test(e2e): full-stack acceptance verification + integration fixes"
```

### Task 18.2: Pre-PR code review (mandatory, per CLAUDE.md)

- [ ] Run `/audit-plan` (plan-adherence-reviewer) **and** `superpowers:requesting-code-review` (backend + frontend guideline reviewers) against the branch. Resolve findings written to `docs/tasks/task-001-household-vehicle-platform/audit.md` before opening the PR. Do not skip even if the plan looks complete.

---

## Self-review (completed by plan author)

**Spec coverage** — every design section maps to a phase (see Phase index) and every PRD §10 acceptance item is a checkbox in Task 18.1. FR-AUTH→P4/P14.2; FR-FLEET→P5/P15.8; FR-VEH→P6/P14.3; FR-MILE→P8/P15.2; FR-MAINT→P9/P15.3; FR-FUEL→P10/P15.4; FR-DASH→P13/P15.7; FR-STATUS→P11.1; FR-ACT→P11.2/P15.5; FR-NOTIF→P12/P15.6; FR-MEDIA→P7/P15.1; NFR security/observability/jobs/CI/deploy→P1,P16,P17,P18.

**Placeholder scan** — foundational/algorithmic tasks (Phases 0–13 pure logic, Phase 14) contain complete code and concrete failing tests. Repetitive domain/feature tasks are specified as concrete task specs (exact files, fields, endpoints, named test cases, exact verify commands) implemented by following the in-full canonical pattern (Task 4.2 backend, Task 14.3 frontend) — a deliberate, documented choice for an MVP of this size, not "TBD" placeholders. Every task has a runnable verify command with an Expected result.

**Type consistency** — shared symbols are stable across tasks: `server.Err*`, `server.Page/PageMeta/.Meta()`, `auth.Identity{UserID,Email,ActiveFleetID,Role}`, `database.Query/SliceQuery/WithLeaderLock`, `events.Envelope/Producer/NoopProducer/Enqueue/RelayOnce`, `authz.RequireSameFleet/RequireWrite/RequireOwner`, `maintenanceschedule.{Schedule,NextDue,DueState,Severity,DefaultThresholds}`, `status.{Input,Derive}`, `fuel.DerivePrice`, the `@myfleet/shared-ts` `ApiClient`/`createErrorFromUnknown`, and the `vehicleKeys` query-key shape. Builder convention difference (auth `Build() Model` vs fleet `Build() (Model, error)`) is called out in Task 6.1 and recorded in `context.md`.

