# Vehicle Card Photo & Quick Actions — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give each vehicle card on `/vehicles` a primary-photo thumbnail plus explicit "open details" and "Carfax report" icon buttons, and teach media-service to serve the already-generated `thumbnail`/`display` image variants so the list costs kilobytes per card instead of megabytes.

**Architecture:** Three separable slices in dependency order. (1) media-service gains an optional `variant` query parameter on the existing `GET /media/{id}/content` route; `mediaobject` reaches `mediavariant` through a narrow port declared in `mediaobject` and adapted in `cmd/main.go`, so no arrow is drawn between the two sibling domain packages. (2) The SPA gains its first runtime-configuration mechanism — a `config.json` fetched before the React tree mounts, ConfigMap-backed in Kubernetes — because the Carfax URL template must change without rebuilding the image. (3) `VehicleCard` is rebuilt around a new `VehiclePhotoThumbnail` component and two `Button asChild` anchors, with `variant` plumbed through the media service client, React Query key, and content hook.

**Tech Stack:** Go 1.x (chi, GORM, logrus, SQLite for tests), React 18 + TypeScript (strict), Vite, TanStack React Query v5, Tailwind + shadcn/ui primitives, Radix `Slot`, `lucide-react`, zod, Vitest + React Testing Library, kustomize.

## Global Constraints

- **Worktree:** all work happens in `/home/tumidanski/source/MyFleet/.worktrees/task-005-vehicle-card-photo-actions` on branch `task-005-vehicle-card-photo-actions`. Never edit the main checkout.
- **Cross-fleet reads return `404`, not `403`** (design D1). `AuthorizeAccess` maps a fleet mismatch to `server.ErrNotFound` so cross-fleet existence is never leaked. The PRD's `403` in §5.1 and its acceptance criteria are wrong; the design corrects them.
- **No `import.meta.env` / `VITE_*` variable is introduced anywhere** (design D3). The Carfax template is runtime config.
- **Accepted `variant` values are exactly `original`, `thumbnail`, `display`**, lowercase, exact match. Absent or empty → `original`. Anything else → `400`. Never a silent fallback to the original.
- **`Cache-Control: private, max-age=300`** stays on every success response of `GET /media/{id}/content`, variants included.
- **Carfax URL template default (byte-for-byte, used in three places — the TS constant, `apps/web/public/config/config.json`, and the k8s ConfigMap):**
  `https://www.carfax.com/VehicleHistory/p/Report.cfx?vin={vin}`
- **No `any` in TypeScript.** Strict mode is on and `npm run -w apps/web lint` runs with `--max-warnings 0`.
- **Loading states use `Skeleton`, never spinners.**
- **Thumbnail box class, used identically in all four states:** `h-20 w-20 shrink-0 rounded-md`.
- **Node is not always on `PATH`.** Before any `npm`/`make fe-*` command:
  `export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22`
- **Verification per branch:** `make ci` (which includes `make manifests`), plus both `kubectl apply --dry-run=server` runs from CLAUDE.md.

---

## File Structure

**Backend — `packages/shared-go`**

| File | Responsibility |
| --- | --- |
| `server/errors.go` (modify) | Add the `ErrBadRequest` (400) sentinel and its `StatusFor` case. |
| `server/server.go` (modify) | Add `codeFor` entries for 400 and 413. |
| `server/errors_test.go` (modify) | Cover the two new mappings. |

**Backend — `apps/media-service`**

| File | Responsibility |
| --- | --- |
| `internal/mediavariant/provider.go` (modify) | Add `GetByMediaObjectAndVariant` — one row by (media object, variant kind). |
| `internal/mediavariant/provider_test.go` (create) | SQLite-backed test for the new read. |
| `internal/mediaobject/contentvariant.go` (create) | `ContentVariant` type, its three constants, and the pure `ParseContentVariant`. |
| `internal/mediaobject/contentvariant_test.go` (create) | Table test for the parser. |
| `internal/mediaobject/processor.go` (modify) | Declare the `VariantRef` value type and `VariantLookup` port; add `ContentInfo`; make `Content` variant-aware; `NewProcessor` takes the port. |
| `internal/mediaobject/processor_test.go` (modify) | Add `fakeVariants`; extend `fakeStore` for per-key bodies and call recording; update the 8 `NewProcessor` call sites; cover the four `Content` paths. |
| `internal/mediaobject/resource.go` (modify) | Parse `variant` on `GET /media/{id}/content`; set headers from `ContentInfo`; `InitializeRoutes` takes the port. |
| `internal/mediaobject/resource_test.go` (modify) | `testRouterWithVariants` helper; the six HTTP-level variant cases. |
| `cmd/main.go` (modify) | The composition root: the ten-line `mediavariant.Provider` → `mediaobject.VariantLookup` adapter, wired into `InitializeRoutes`. |

**Frontend — `apps/web`**

| File | Responsibility |
| --- | --- |
| `src/types/models/media.ts` (modify) | `MediaVariant` union type. |
| `src/services/api/MediaService.ts` (modify) | `getContentBlob(id, variant?)` appends `?variant=`. |
| `src/lib/hooks/api/media.ts` (modify) | Variant-aware `mediaKeys.content` and `useMediaContentUrl`. |
| `src/lib/hooks/api/media.test.ts` (modify) | Update the key-shape assertion; import the lifted object-URL stub. |
| `src/lib/config/runtimeConfig.ts` (create) | Fetch/parse/latch the runtime config. Never throws. |
| `src/lib/config/runtimeConfig.test.ts` (create) | Pure-parse cases plus mocked-`fetch` failure cases. |
| `src/lib/carfax.ts` (create) | Pure `buildCarfaxUrl(template, vin) → string | null`. |
| `src/lib/carfax.test.ts` (create) | Encoding, trimming, missing placeholder, non-`https:`. |
| `src/main.tsx` (modify) | Load the config before mounting. |
| `public/config/config.json` (create) | The baked-in default document. |
| `nginx.conf` (modify) | `Cache-Control: no-cache` on `/config/config.json`. |
| `src/test/renderWithProviders.tsx` (create) | Render helper: `QueryClientProvider` + `MemoryRouter`. |
| `src/test/objectUrl.ts` (create) | `URL.createObjectURL`/`revokeObjectURL` stub, lifted from `media.test.ts`. |
| `src/components/features/vehicles/VehiclePhotoThumbnail.tsx` (create) | Four-state 80×80 photo box for the card. |
| `src/components/features/vehicles/VehiclePhotoThumbnail.test.tsx` (create) | Photo / no id / error / loading. |
| `src/components/features/vehicles/VehicleCard.tsx` (modify) | New layout, thumbnail, two icon-button anchors; wrapping `<Link>` deleted. |
| `src/components/features/vehicles/VehicleCard.test.tsx` (create) | Photo & VIN permutations, labels, `target`/`rel`, non-clickable body. |
| `src/components/features/vehicles/VehicleList.tsx` (modify) | Skeleton height `h-28` → `h-44`. |

**Deployment — `deploy/k8s`**

| File | Responsibility |
| --- | --- |
| `base/web/configmap.yaml` (create) | ConfigMap `web-config`, key `config.json`, real default value. |
| `base/web/deployment.yaml` (modify) | Read-only ConfigMap volume at `/usr/share/nginx/html/config`. |
| `base/kustomization.yaml` (modify) | Register `web/configmap.yaml`. |

---

## Task 1: `server.ErrBadRequest` and the `codeFor` gaps

`packages/shared-go/server/errors.go` has no 400 sentinel — the closest is `ErrValidation` (422). FR-7.3 requires a real 400 for an unrecognised `variant`, and adding the sentinel keeps the route handler a one-liner (`server.WriteError(w, err)`).

`codeFor` also has no `413` case, so a 413 body today carries `"code": "internal_error"` — while the frontend's own client-side size rejection already uses the code `payload_too_large` (`apps/web/src/lib/hooks/api/media.ts:42`). Since the switch is being edited anyway, close that gap too. This is deliberate and in scope; it is not scope creep.

**Files:**
- Modify: `packages/shared-go/server/errors.go`
- Modify: `packages/shared-go/server/server.go:12-29`
- Test: `packages/shared-go/server/errors_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `server.ErrBadRequest` (an `error` mapping to HTTP 400 via `server.StatusFor`, rendered by `server.WriteError` with `"code": "bad_request"`). Task 3 returns it; Task 5 writes it.

- [ ] **Step 1: Write the failing tests**

Replace the whole body of `packages/shared-go/server/errors_test.go` with:

```go
package server

import "testing"

func TestStatusFor_mapsDomainErrors(t *testing.T) {
	cases := map[error]int{
		ErrBadRequest:            400,
		ErrUnauthorized:          401,
		ErrForbidden:             403,
		ErrNotFound:              404,
		ErrConflict:              409,
		ErrGone:                  410,
		ErrRequestEntityTooLarge: 413,
		ErrValidation:            422,
	}
	for err, want := range cases {
		if got := StatusFor(err); got != want {
			t.Fatalf("StatusFor(%v)=%d want %d", err, got, want)
		}
	}
}

// TestCodeFor_namesEveryMappedStatus pins the JSON:API `code` string for every
// status StatusFor can produce. 400 and 413 previously fell through to
// "internal_error", which tells a client nothing about what it did wrong.
func TestCodeFor_namesEveryMappedStatus(t *testing.T) {
	cases := map[int]string{
		400: "bad_request",
		401: "unauthorized",
		403: "forbidden",
		404: "not_found",
		409: "conflict",
		410: "gone",
		413: "payload_too_large",
		422: "validation_error",
		500: "internal_error",
	}
	for status, want := range cases {
		if got := codeFor(status); got != want {
			t.Fatalf("codeFor(%d)=%q want %q", status, got, want)
		}
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test github.com/jtumidanski/myfleet/packages/shared-go/server/...`
Expected: FAIL — `undefined: ErrBadRequest` (compile error).

- [ ] **Step 3: Add the sentinel and its status mapping**

In `packages/shared-go/server/errors.go`, the `var` block becomes:

```go
var (
	ErrBadRequest            = errors.New("bad request")              // 400
	ErrUnauthorized          = errors.New("unauthorized")             // 401
	ErrForbidden             = errors.New("forbidden")                // 403
	ErrNotFound              = errors.New("not found")                // 404
	ErrConflict              = errors.New("conflict")                 // 409
	ErrGone                  = errors.New("gone")                     // 410
	ErrRequestEntityTooLarge = errors.New("request entity too large") // 413
	ErrValidation            = errors.New("validation")               // 422
)
```

and the first case of `StatusFor` becomes:

```go
	switch {
	case errors.Is(err, ErrBadRequest):
		return 400
	case errors.Is(err, ErrUnauthorized):
		return 401
```

(leave the remaining cases untouched).

- [ ] **Step 4: Add the `codeFor` entries**

In `packages/shared-go/server/server.go`, `codeFor` becomes:

```go
func codeFor(status int) string {
	switch status {
	case 400:
		return "bad_request"
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
	case 413:
		return "payload_too_large"
	case 422:
		return "validation_error"
	default:
		return "internal_error"
	}
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test github.com/jtumidanski/myfleet/packages/shared-go/server/...`
Expected: PASS.

- [ ] **Step 6: Confirm nothing else regressed**

Run: `go build github.com/jtumidanski/myfleet/... && go test github.com/jtumidanski/myfleet/...`
Expected: PASS. `ErrBadRequest` is additive; the only behavioural change to existing responses is the `code` string on 413 bodies, which no Go test asserts on.

- [ ] **Step 7: Commit**

```bash
git add packages/shared-go/server/errors.go packages/shared-go/server/server.go packages/shared-go/server/errors_test.go
git commit -m "feat(shared-go): add ErrBadRequest sentinel and name the 400/413 error codes"
```

---

## Task 2: `mediavariant.GetByMediaObjectAndVariant`

`Provider` today offers only `ListByMediaObject`, which loads every variant row to use one. The content route needs exactly one row, so add a narrower read backed by a `WHERE media_object_id = ? AND variant = ?` query.

A miss is a **normal** outcome, not an error: variants do not exist until the processing worker has run, and never exist for non-image media. So the signature returns `(Model, bool, error)` rather than a sentinel error.

`ListByMediaObject` stays. (Note: it currently has no production caller — the worker's idempotent replace path uses `Administrator.ReplaceForMediaObject`, not this method. Removing it would leave `Provider` empty, so leave it alone; this is not part of the task.)

**Files:**
- Modify: `apps/media-service/internal/mediavariant/provider.go`
- Test: `apps/media-service/internal/mediavariant/provider_test.go` (create)

**Interfaces:**
- Consumes: nothing (the existing `Model`, `Variant`, `Entity`, `Make` are already in the package).
- Produces: `mediavariant.Provider.GetByMediaObjectAndVariant(mediaObjectID string, v Variant) (Model, bool, error)`. Task 5's adapter in `cmd/main.go` is the only caller.

- [ ] **Step 1: Write the failing test**

Create `apps/media-service/internal/mediavariant/provider_test.go`:

```go
package mediavariant

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newVariantTestDB opens an in-memory SQLite database with a "media" schema
// attached and creates media_variants with raw SQL.
//
// GORM AutoMigrate mishandles schema-qualified table names (media.media_variants)
// on SQLite when the entity carries index tags, so the table is created directly
// — the same approach mediaobject's tests take.
func newVariantTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec("ATTACH DATABASE ':memory:' AS media").Error; err != nil {
		t.Fatalf("attach media schema: %v", err)
	}
	if err := db.Exec(`CREATE TABLE media.media_variants (
		id              TEXT PRIMARY KEY,
		media_object_id TEXT NOT NULL,
		variant         TEXT NOT NULL,
		object_key      TEXT NOT NULL,
		width           INTEGER,
		height          INTEGER,
		content_type    TEXT,
		created_at      DATETIME
	)`).Error; err != nil {
		t.Fatalf("create media_variants: %v", err)
	}
	return db
}

func seedVariant(t *testing.T, db *gorm.DB, mediaObjectID string, v Variant, key, contentType string) {
	t.Helper()
	m, err := NewBuilder().
		SetMediaObjectID(mediaObjectID).
		SetVariant(v).
		SetObjectKey(key).
		SetWidth(320).
		SetHeight(240).
		SetContentType(contentType).
		Build()
	if err != nil {
		t.Fatalf("build variant: %v", err)
	}
	if err := NewAdministrator(db).ReplaceForMediaObject(mediaObjectID, []Model{m}); err != nil {
		t.Fatalf("seed variant: %v", err)
	}
}

// TestGetByMediaObjectAndVariant_returnsTheNamedRow is the read the content
// route depends on: one row, selected by both media object AND variant kind.
func TestGetByMediaObjectAndVariant_returnsTheNamedRow(t *testing.T) {
	db := newVariantTestDB(t)
	seedVariant(t, db, "m1", VariantThumbnail, "fleet-a/m1/thumbnail.jpg", "image/jpeg")

	got, found, err := NewProvider(db).GetByMediaObjectAndVariant("m1", VariantThumbnail)
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if !found {
		t.Fatal("found = false, want true for a seeded variant")
	}
	if got.ObjectKey() != "fleet-a/m1/thumbnail.jpg" {
		t.Fatalf("ObjectKey = %q, want fleet-a/m1/thumbnail.jpg", got.ObjectKey())
	}
	if got.ContentType() != "image/jpeg" {
		t.Fatalf("ContentType = %q, want image/jpeg", got.ContentType())
	}
}

// TestGetByMediaObjectAndVariant_missIsNotAnError pins the contract the content
// route leans on: a media object whose processing has not run (or which is not a
// processable image) reports found=false with a nil error, so the caller can
// fall back to the original rather than failing the request.
func TestGetByMediaObjectAndVariant_missIsNotAnError(t *testing.T) {
	db := newVariantTestDB(t)
	seedVariant(t, db, "m1", VariantThumbnail, "fleet-a/m1/thumbnail.jpg", "image/jpeg")

	// Right media object, variant kind that was never generated.
	if _, found, err := NewProvider(db).GetByMediaObjectAndVariant("m1", VariantDisplay); err != nil || found {
		t.Fatalf("GetByMediaObjectAndVariant(m1, display) = (_, %v, %v), want (_, false, nil)", found, err)
	}
	// Right variant kind, different media object — must not leak another row.
	if _, found, err := NewProvider(db).GetByMediaObjectAndVariant("m2", VariantThumbnail); err != nil || found {
		t.Fatalf("GetByMediaObjectAndVariant(m2, thumbnail) = (_, %v, %v), want (_, false, nil)", found, err)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test github.com/jtumidanski/myfleet/apps/media-service/internal/mediavariant/...`
Expected: FAIL — `db.GetByMediaObjectAndVariant undefined` (compile error).

- [ ] **Step 3: Add the method**

Replace `apps/media-service/internal/mediavariant/provider.go` with:

```go
package mediavariant

import (
	"errors"

	"gorm.io/gorm"
)

// Provider is the read-only interface for media-variant data access.
type Provider interface {
	ListByMediaObject(mediaObjectID string) ([]Model, error)
	// GetByMediaObjectAndVariant returns the named variant, or found=false when
	// the worker has not produced it (or never will, for non-image media). A
	// miss is a normal outcome, not an error.
	GetByMediaObjectAndVariant(mediaObjectID string, v Variant) (Model, bool, error)
}

type dbProvider struct{ db *gorm.DB }

// NewProvider returns a read-only Provider backed by the given database.
func NewProvider(db *gorm.DB) Provider { return &dbProvider{db: db} }

func (p *dbProvider) ListByMediaObject(mediaObjectID string) ([]Model, error) {
	var es []Entity
	if err := p.db.Where("media_object_id = ?", mediaObjectID).Find(&es).Error; err != nil {
		return nil, err
	}
	out := make([]Model, 0, len(es))
	for _, e := range es {
		out = append(out, Make(e))
	}
	return out, nil
}

func (p *dbProvider) GetByMediaObjectAndVariant(mediaObjectID string, v Variant) (Model, bool, error) {
	var e Entity
	err := p.db.Where("media_object_id = ? AND variant = ?", mediaObjectID, string(v)).First(&e).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Model{}, false, nil
	}
	if err != nil {
		return Model{}, false, err
	}
	return Make(e), true, nil
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test github.com/jtumidanski/myfleet/apps/media-service/internal/mediavariant/...`
Expected: PASS (both tests).

- [ ] **Step 5: Commit**

```bash
git add apps/media-service/internal/mediavariant/provider.go apps/media-service/internal/mediavariant/provider_test.go
git commit -m "feat(media-service): add GetByMediaObjectAndVariant read to mediavariant"
```

---

## Task 3: `ParseContentVariant`

The `variant` query parameter is parsed by a pure, dependency-free function so it is unit-testable without an HTTP round trip — the same pattern as `classifyUploadError` (`resource.go:32`).

Matching is **exact and lowercase**. `?variant=Thumbnail` is a 400: case-insensitive matching would be a courtesy that makes the accepted set fuzzy, and strictness costs a caller one character while keeping the contract exact. An absent or empty value means the original, which is what preserves the pre-existing contract byte-for-byte.

**Files:**
- Create: `apps/media-service/internal/mediaobject/contentvariant.go`
- Test: `apps/media-service/internal/mediaobject/contentvariant_test.go`

**Interfaces:**
- Consumes: `server.ErrBadRequest` (Task 1).
- Produces:
  - `mediaobject.ContentVariant` (a `string`-backed type) with constants `ContentOriginal`, `ContentThumbnail`, `ContentDisplay`.
  - `mediaobject.ParseContentVariant(raw string) (ContentVariant, error)`.
  Tasks 4 and 5 both use these.

- [ ] **Step 1: Write the failing test**

Create `apps/media-service/internal/mediaobject/contentvariant_test.go`:

```go
package mediaobject

import (
	"errors"
	"testing"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// TestParseContentVariant covers the whole accepted set plus the two shapes that
// must NOT be accepted. A silent fallback to the original on an unknown value
// would ship multi-megabyte responses for a typo, which is the reason this
// returns an error instead.
func TestParseContentVariant(t *testing.T) {
	valid := map[string]ContentVariant{
		"":          ContentOriginal, // parameter absent, or ?variant= with no value
		"original":  ContentOriginal,
		"thumbnail": ContentThumbnail,
		"display":   ContentDisplay,
	}
	for raw, want := range valid {
		got, err := ParseContentVariant(raw)
		if err != nil {
			t.Fatalf("ParseContentVariant(%q) returned error %v, want %q", raw, err, want)
		}
		if got != want {
			t.Fatalf("ParseContentVariant(%q) = %q, want %q", raw, got, want)
		}
	}

	// Exact lowercase matching only: a wrong-case value and an unknown value are
	// both 400s, never a silent fallback.
	for _, raw := range []string{"Thumbnail", "THUMBNAIL", "bogus", "small", " thumbnail"} {
		got, err := ParseContentVariant(raw)
		if !errors.Is(err, server.ErrBadRequest) {
			t.Fatalf("ParseContentVariant(%q) = (%q, %v), want ErrBadRequest", raw, got, err)
		}
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test github.com/jtumidanski/myfleet/apps/media-service/internal/mediaobject/... -run TestParseContentVariant -v`
Expected: FAIL — `undefined: ContentVariant` / `undefined: ParseContentVariant` (compile error).

- [ ] **Step 3: Write the implementation**

Create `apps/media-service/internal/mediaobject/contentvariant.go`:

```go
package mediaobject

import "github.com/jtumidanski/myfleet/packages/shared-go/server"

// ContentVariant names which rendition of a media object's bytes to serve.
// The values match mediavariant.Variant's for the derived kinds; "original"
// has no variant row because it is the uploaded bytes themselves.
type ContentVariant string

const (
	ContentOriginal  ContentVariant = "original"
	ContentThumbnail ContentVariant = "thumbnail"
	ContentDisplay   ContentVariant = "display"
)

// ParseContentVariant maps the raw ?variant= query value to a ContentVariant.
//
// An absent or empty parameter means the original, which preserves the
// pre-existing contract of GET /media/{id}/content exactly. Anything else that
// is not an exact lowercase match is server.ErrBadRequest (400) — never a
// silent fallback to the original, which would ship multi-megabyte responses
// for a typo.
func ParseContentVariant(raw string) (ContentVariant, error) {
	switch raw {
	case "":
		return ContentOriginal, nil
	case string(ContentOriginal):
		return ContentOriginal, nil
	case string(ContentThumbnail):
		return ContentThumbnail, nil
	case string(ContentDisplay):
		return ContentDisplay, nil
	default:
		return "", server.ErrBadRequest
	}
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test github.com/jtumidanski/myfleet/apps/media-service/internal/mediaobject/... -run TestParseContentVariant -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/media-service/internal/mediaobject/contentvariant.go apps/media-service/internal/mediaobject/contentvariant_test.go
git commit -m "feat(media-service): add ParseContentVariant for the ?variant= query parameter"
```

---

## Task 4: The `VariantLookup` port and a variant-aware `Processor.Content`

`mediaobject` must resolve a variant's `object_key`, which lives in `mediavariant`. Rather than importing that sibling domain package, `mediaobject` declares a **narrow port it owns** and the composition root supplies the adapter (Task 5). This mirrors a precedent sitting in the very file being edited: `ObjectStore` (`processor.go:36`) is declared in `mediaobject`, implemented by `storage.Client`, and `storage` does not import `mediaobject`.

`Content` gains a parameter rather than growing a sibling method. Two content methods would mean two places that must each remember to call `AuthorizeAccess` first, and a future edit to one could silently diverge. Authorization is the one thing here that must not have a second implementation.

**Order of operations (FR-7.5): resolve and fleet-scope the media object BEFORE any variant lookup or object-store read.** A cross-fleet caller exits at step 1 with `404` and never reaches the store.

**Decisions the design left implicit, resolved here:**
- A **`Lookup` error is returned as-is** (→ 500), not swallowed. `GetByID` already read `media_objects` from the same database immediately before, so a lookup error implies the DB is genuinely broken; masking it would hide a real fault. The port's contract makes a normal miss `found=false`, so errors mean errors.
- A **variant row whose object is missing from the store** falls back to the original and logs at **warn** — unlike the step-3 miss, that is DB/store drift and someone should see it. A `404` would be a worse answer: the resource plainly exists and we are holding readable bytes for it. Cost is one extra store round trip on a path that should never be taken.

**Files:**
- Modify: `apps/media-service/internal/mediaobject/processor.go:70-83` (struct + constructor), `:209-231` (`Content`)
- Test: `apps/media-service/internal/mediaobject/processor_test.go`

**Interfaces:**
- Consumes: `ContentVariant`, `ContentOriginal`, `ContentThumbnail`, `ContentDisplay` (Task 3).
- Produces:
  - `mediaobject.VariantRef{ObjectKey, ContentType string}`
  - `mediaobject.VariantLookup interface { Lookup(mediaObjectID, variant string) (VariantRef, bool, error) }`
  - `mediaobject.ContentInfo{ContentType string; Size int64; Served ContentVariant}`
  - `NewProcessor(log logrus.FieldLogger, p Provider, a Administrator, st ObjectStore, variants VariantLookup) *Processor`
  - `(*Processor).Content(ctx context.Context, id, identityFleetID string, want ContentVariant) (ContentInfo, io.ReadCloser, error)`
  Task 5 consumes all of these.

- [ ] **Step 1: Extend the test fakes**

In `apps/media-service/internal/mediaobject/processor_test.go`, replace the `fakeStore` type and its `GetObject` method (`:309-321` and `:338-344`) with the versions below, and add `fakeVariants` beneath them. `getKey` and `getBody` keep their existing meaning so the six current tests are untouched; the new fields let a test serve different bytes per key and assert which keys were fetched.

```go
type fakeStore struct {
	bucket   string
	putCalls int
	putKey   string
	putBody  []byte
	putSize  int64
	putCT    string
	putErr   error

	getKey  string
	getBody []byte
	getErr  error

	// getBodies serves per-key bytes, so a test can hold a variant and its
	// original at once; keys absent from the map fall back to getBody.
	getBodies map[string][]byte
	// missing marks keys that answer storage.ErrObjectNotFound, which is how
	// DB/store drift (a variant row whose object is gone) is simulated.
	missing map[string]bool
	// getCalls records every key requested, in order, so a test can assert that
	// a cross-fleet read touched storage zero times.
	getCalls []string
}
```

```go
func (f *fakeStore) GetObject(ctx context.Context, key string) (io.ReadCloser, error) {
	f.getCalls = append(f.getCalls, key)
	if f.missing[key] {
		return nil, storage.ErrObjectNotFound
	}
	if f.getErr != nil {
		return nil, f.getErr
	}
	f.getKey = key
	if b, ok := f.getBodies[key]; ok {
		return io.NopCloser(bytes.NewReader(b)), nil
	}
	return io.NopCloser(bytes.NewReader(f.getBody)), nil
}

// fakeVariants stands in for the mediavariant-backed adapter. refs is keyed by
// variant name ("thumbnail", "display"); an absent key is the normal miss.
type fakeVariants struct {
	refs  map[string]VariantRef
	err   error
	calls []string
}

func (f *fakeVariants) Lookup(mediaObjectID, variant string) (VariantRef, bool, error) {
	f.calls = append(f.calls, variant)
	if f.err != nil {
		return VariantRef{}, false, f.err
	}
	ref, ok := f.refs[variant]
	return ref, ok, nil
}
```

- [ ] **Step 2: Update every existing `NewProcessor` call site**

`NewProcessor` gains a fifth parameter. Update the eight call sites in `processor_test.go` (lines 349, 383, 407, 437, 470, 501, 524, 550) to pass an empty fake — those tests never request a variant, and a real fake rather than `nil` means a future test that does request one gets a miss instead of a panic:

```bash
sed -i 's/NewProcessor(logrus.New(), NewProvider(db), NewAdministrator(db), store)/NewProcessor(logrus.New(), NewProvider(db), NewAdministrator(db), store, \&fakeVariants{})/g' \
  apps/media-service/internal/mediaobject/processor_test.go
```

Verify: `grep -c 'NewProcessor(logrus.New()' apps/media-service/internal/mediaobject/processor_test.go` → 8, and none of them still end in `store)`.

- [ ] **Step 3: Write the failing tests for `Content`**

Append to `apps/media-service/internal/mediaobject/processor_test.go`:

```go
// seedReadyObject creates a media object and records its byte count, returning
// the model — the state a completed upload leaves behind.
func seedReadyObject(t *testing.T, pr *Processor, fleetID string, payload []byte) Model {
	t.Helper()
	created, err := pr.InitUpload(fleetID, "u1", "image/png", "photo.png")
	if err != nil {
		t.Fatalf("init upload: %v", err)
	}
	stored, err := pr.StoreContent(context.Background(), created.ID(), fleetID, bytes.NewReader(payload), int64(len(payload)))
	if err != nil {
		t.Fatalf("store content: %v", err)
	}
	return stored
}

func readAllAndClose(t *testing.T, rc io.ReadCloser) string {
	t.Helper()
	defer func() { _ = rc.Close() }()
	b, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read content: %v", err)
	}
	return string(b)
}

// TestContent_originalIsUnchanged pins the pre-existing contract: asking for the
// original serves the media object's own key, content type, and recorded size.
func TestContent_originalIsUnchanged(t *testing.T) {
	db := newConfirmTestDB(t)
	store := &fakeStore{bucket: "myfleet-media", getBody: []byte("original-bytes")}
	variants := &fakeVariants{}
	pr := NewProcessor(logrus.New(), NewProvider(db), NewAdministrator(db), store, variants)

	obj := seedReadyObject(t, pr, "fleet-a", []byte("original-bytes"))

	info, rc, err := pr.Content(context.Background(), obj.ID(), "fleet-a", ContentOriginal)
	if err != nil {
		t.Fatalf("Content: %v", err)
	}
	if got := readAllAndClose(t, rc); got != "original-bytes" {
		t.Fatalf("body = %q, want original-bytes", got)
	}
	if info.ContentType != "image/png" {
		t.Fatalf("ContentType = %q, want image/png", info.ContentType)
	}
	if info.Size != int64(len("original-bytes")) {
		t.Fatalf("Size = %d, want %d", info.Size, len("original-bytes"))
	}
	if info.Served != ContentOriginal {
		t.Fatalf("Served = %q, want original", info.Served)
	}
	if len(variants.calls) != 0 {
		t.Fatalf("variant lookup ran %v times for an original request; it must not", variants.calls)
	}
}

// TestContent_variantFoundServesVariantBytes is the whole point of the feature:
// the variant's own key and content type, and NO size — media_variants records
// width/height/content_type but no byte count, so Content-Length must be omitted.
func TestContent_variantFoundServesVariantBytes(t *testing.T) {
	db := newConfirmTestDB(t)
	store := &fakeStore{
		bucket:    "myfleet-media",
		getBody:   []byte("original-bytes"),
		getBodies: map[string][]byte{"fleet-a/thumb.jpg": []byte("thumb-bytes")},
	}
	variants := &fakeVariants{refs: map[string]VariantRef{
		"thumbnail": {ObjectKey: "fleet-a/thumb.jpg", ContentType: "image/jpeg"},
	}}
	pr := NewProcessor(logrus.New(), NewProvider(db), NewAdministrator(db), store, variants)

	obj := seedReadyObject(t, pr, "fleet-a", []byte("original-bytes"))

	info, rc, err := pr.Content(context.Background(), obj.ID(), "fleet-a", ContentThumbnail)
	if err != nil {
		t.Fatalf("Content: %v", err)
	}
	if got := readAllAndClose(t, rc); got != "thumb-bytes" {
		t.Fatalf("body = %q, want thumb-bytes", got)
	}
	if info.ContentType != "image/jpeg" {
		t.Fatalf("ContentType = %q, want the variant's own image/jpeg", info.ContentType)
	}
	if info.Size != 0 {
		t.Fatalf("Size = %d, want 0 — the original's size must never describe a variant", info.Size)
	}
	if info.Served != ContentThumbnail {
		t.Fatalf("Served = %q, want thumbnail", info.Served)
	}
}

// TestContent_variantMissingFallsBackToOriginal is the normal state for media
// whose processing has not finished, and for anything that is not a processable
// image. Clients must not have to special-case it.
func TestContent_variantMissingFallsBackToOriginal(t *testing.T) {
	db := newConfirmTestDB(t)
	store := &fakeStore{bucket: "myfleet-media", getBody: []byte("original-bytes")}
	variants := &fakeVariants{} // no rows at all
	pr := NewProcessor(logrus.New(), NewProvider(db), NewAdministrator(db), store, variants)

	obj := seedReadyObject(t, pr, "fleet-a", []byte("original-bytes"))

	info, rc, err := pr.Content(context.Background(), obj.ID(), "fleet-a", ContentThumbnail)
	if err != nil {
		t.Fatalf("Content: %v", err)
	}
	if got := readAllAndClose(t, rc); got != "original-bytes" {
		t.Fatalf("body = %q, want the original bytes as a fallback", got)
	}
	if info.Served != ContentOriginal {
		t.Fatalf("Served = %q, want original after a fallback", info.Served)
	}
	if info.ContentType != "image/png" {
		t.Fatalf("ContentType = %q, want the media object's own image/png", info.ContentType)
	}
	if info.Size == 0 {
		t.Fatal("Size = 0; a fallback serves the ORIGINAL, whose size is known")
	}
}

// TestContent_variantObjectMissingFallsBackToOriginal covers DB/store drift: the
// variant row exists but its object is gone from MinIO. 404 would be a worse
// answer than the original — the resource plainly exists and we hold readable
// bytes for it.
func TestContent_variantObjectMissingFallsBackToOriginal(t *testing.T) {
	db := newConfirmTestDB(t)
	store := &fakeStore{
		bucket:  "myfleet-media",
		getBody: []byte("original-bytes"),
		missing: map[string]bool{"fleet-a/gone.jpg": true},
	}
	variants := &fakeVariants{refs: map[string]VariantRef{
		"thumbnail": {ObjectKey: "fleet-a/gone.jpg", ContentType: "image/jpeg"},
	}}
	pr := NewProcessor(logrus.New(), NewProvider(db), NewAdministrator(db), store, variants)

	obj := seedReadyObject(t, pr, "fleet-a", []byte("original-bytes"))

	info, rc, err := pr.Content(context.Background(), obj.ID(), "fleet-a", ContentThumbnail)
	if err != nil {
		t.Fatalf("Content: %v", err)
	}
	if got := readAllAndClose(t, rc); got != "original-bytes" {
		t.Fatalf("body = %q, want the original bytes", got)
	}
	if info.Served != ContentOriginal {
		t.Fatalf("Served = %q, want original", info.Served)
	}
	if len(store.getCalls) != 2 {
		t.Fatalf("store was called %v; want the missing variant key then the original", store.getCalls)
	}
}

// TestContent_crossFleetNeverTouchesLookupOrStore is FR-7.5: the media object is
// resolved and fleet-scoped BEFORE any variant lookup or object-store read, so a
// variant can never be reachable by a caller who could not read the original.
func TestContent_crossFleetNeverTouchesLookupOrStore(t *testing.T) {
	db := newConfirmTestDB(t)
	store := &fakeStore{bucket: "myfleet-media", getBody: []byte("original-bytes")}
	variants := &fakeVariants{refs: map[string]VariantRef{
		"thumbnail": {ObjectKey: "fleet-b/thumb.jpg", ContentType: "image/jpeg"},
	}}
	pr := NewProcessor(logrus.New(), NewProvider(db), NewAdministrator(db), store, variants)

	obj := seedReadyObject(t, pr, "fleet-b", []byte("original-bytes"))

	_, _, err := pr.Content(context.Background(), obj.ID(), "fleet-a", ContentThumbnail)
	if !errors.Is(err, server.ErrNotFound) {
		t.Fatalf("Content across fleets = %v, want ErrNotFound (404, never 403 — 403 would leak existence)", err)
	}
	if len(variants.calls) != 0 {
		t.Fatalf("variant lookup ran %v for a cross-fleet read", variants.calls)
	}
	if len(store.getCalls) != 0 {
		t.Fatalf("cross-fleet read touched storage: %v", store.getCalls)
	}
}

// TestContent_lookupErrorIsReturned: a miss is found=false, so an actual error
// means the database is broken. Masking it behind the original would hide a real
// fault.
func TestContent_lookupErrorIsReturned(t *testing.T) {
	db := newConfirmTestDB(t)
	store := &fakeStore{bucket: "myfleet-media", getBody: []byte("original-bytes")}
	boom := errors.New("simulated variant query failure")
	variants := &fakeVariants{err: boom}
	pr := NewProcessor(logrus.New(), NewProvider(db), NewAdministrator(db), store, variants)

	obj := seedReadyObject(t, pr, "fleet-a", []byte("original-bytes"))

	if _, _, err := pr.Content(context.Background(), obj.ID(), "fleet-a", ContentThumbnail); !errors.Is(err, boom) {
		t.Fatalf("Content = %v, want the lookup error propagated", err)
	}
}
```

- [ ] **Step 4: Run the tests to verify they fail**

Run: `go test github.com/jtumidanski/myfleet/apps/media-service/internal/mediaobject/... -run TestContent -v`
Expected: FAIL — `undefined: VariantRef`, `too many arguments in call to NewProcessor` (compile errors).

- [ ] **Step 5: Add the port, `ContentInfo`, and the processor field**

In `apps/media-service/internal/mediaobject/processor.go`, insert immediately after the `ObjectStore` interface (after line 40):

```go
// VariantRef is what the processor needs in order to stream a derived image:
// where the bytes live and what they are. Nothing else about a variant is
// relevant here.
type VariantRef struct {
	ObjectKey   string
	ContentType string
}

// VariantLookup resolves a derived image for a media object. It is declared
// here, in the package that consumes it, and implemented in the composition
// root — the same shape as ObjectStore above — so mediaobject never imports the
// sibling mediavariant package and the dependency graph stays a tree.
//
// variant crosses the port as a plain string so the implementer does not need
// mediaobject's types either.
//
// A miss is a normal outcome (found=false), not an error: variants do not exist
// until the processing worker has run, and never exist for non-image media.
type VariantLookup interface {
	Lookup(mediaObjectID, variant string) (VariantRef, bool, error)
}

// ContentInfo describes the bytes actually being served, which are not always
// the media object's own metadata: a variant is re-encoded and carries its own
// content type, and its length is not recorded anywhere. Returning this instead
// of the Model is what lets the handler set headers from the bytes it is about
// to write.
type ContentInfo struct {
	ContentType string
	// Size is 0 when unknown; the handler then omits Content-Length. The
	// original's size must never be sent for a variant response.
	Size int64
	// Served is what was actually served, which may be Original after a
	// fallback even though a derived variant was requested.
	Served ContentVariant
}
```

Then change the `Processor` struct and constructor (currently `:74-83`) to:

```go
type Processor struct {
	log      logrus.FieldLogger
	p        Provider
	a        Administrator
	storage  ObjectStore
	variants VariantLookup
}

func NewProcessor(log logrus.FieldLogger, p Provider, a Administrator, st ObjectStore, variants VariantLookup) *Processor {
	return &Processor{log: log, p: p, a: a, storage: st, variants: variants}
}
```

- [ ] **Step 6: Rewrite `Content`**

Replace the whole `Content` method (`:209-231`) with:

```go
// Content authorizes by fleet and opens the requested rendition's bytes for
// streaming to the client. The caller owns closing the returned ReadCloser.
// Bytes are proxied rather than presigned so MinIO stays unreachable from the
// browser.
//
// The media object is resolved and fleet-scoped FIRST, before any variant
// lookup or object-store read, so a variant is never reachable by a caller who
// could not read the original (FR-7.5). A cross-fleet caller exits here with
// 404 — never 403, which would restore the existence oracle AuthorizeAccess
// exists to prevent.
//
// A derived variant that cannot be served falls back to the original rather
// than failing: that is the normal state for media whose processing has not
// finished and for anything that is not a processable image, and a client must
// not have to special-case it.
func (pr *Processor) Content(ctx context.Context, id, identityFleetID string, want ContentVariant) (ContentInfo, io.ReadCloser, error) {
	m, err := pr.GetByID(id, identityFleetID)
	if err != nil {
		return ContentInfo{}, nil, err
	}

	if want != ContentOriginal {
		ref, found, err := pr.variants.Lookup(id, string(want))
		if err != nil {
			// A miss is found=false, so an error here means the database is
			// broken — and GetByID just read the same database successfully.
			// Serving the original instead would hide a real fault.
			return ContentInfo{}, nil, err
		}
		if !found {
			// Expected whenever processing has not run yet; debug, not warn.
			pr.log.WithField("media_id", id).WithField("variant", string(want)).
				Debug("no stored variant; serving the original")
		} else {
			rc, err := pr.storage.GetObject(ctx, ref.ObjectKey)
			switch {
			case err == nil:
				ct := ref.ContentType
				if ct == "" {
					// Should never happen — variants are re-encoded and always
					// record a type — but an empty header is worse than a
					// slightly wrong one.
					ct = m.ContentType()
				}
				// Size stays 0: media_variants records width/height/content_type
				// but no byte count, so Content-Length is omitted (FR-7.8).
				return ContentInfo{ContentType: ct, Served: want}, rc, nil
			case errors.Is(err, storage.ErrObjectNotFound):
				// DB/store drift, unlike the miss above — someone should see it.
				pr.log.WithField("media_id", id).WithField("variant", string(want)).
					WithField("object_key", ref.ObjectKey).
					Warn("variant row has no object in storage; serving the original")
			default:
				return ContentInfo{}, nil, err
			}
		}
	}

	return pr.openOriginal(ctx, m)
}

// openOriginal streams the uploaded bytes. Kept separate so the not-found
// mapping has exactly one implementation, whether it is reached directly or via
// a variant fallback.
func (pr *Processor) openOriginal(ctx context.Context, m Model) (ContentInfo, io.ReadCloser, error) {
	rc, err := pr.storage.GetObject(ctx, m.ObjectKey())
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotFound) {
			// The row exists but its bytes do not: InitUpload creates the row
			// before any content is PUT, and a PUT that fails leaves exactly
			// that state. 404 rather than 500 because nothing is broken
			// server-side — this sub-resource simply does not exist yet — and
			// it matches what the client used to see when it followed a
			// presigned URL straight to MinIO.
			return ContentInfo{}, nil, server.ErrNotFound
		}
		return ContentInfo{}, nil, err
	}
	return ContentInfo{
		ContentType: m.ContentType(),
		Size:        m.Size(),
		Served:      ContentOriginal,
	}, rc, nil
}
```

- [ ] **Step 7: Run the package tests**

Run: `go test github.com/jtumidanski/myfleet/apps/media-service/internal/mediaobject/... -v`
Expected: the six new `TestContent_*` PASS. `resource.go` and `resource_test.go` still call the four-argument `NewProcessor` / three-argument `Content`, so the package **will not compile yet** — that is Task 5. If you want a green gate before moving on, do Steps 1-2 of Task 5 first; otherwise proceed directly.

- [ ] **Step 8: Commit**

```bash
git add apps/media-service/internal/mediaobject/processor.go apps/media-service/internal/mediaobject/processor_test.go
git commit -m "feat(media-service): make Processor.Content variant-aware behind a VariantLookup port"
```

---

## Task 5: The route, the composition-root adapter, and HTTP-level tests

The handler gains four lines at the top and a header block driven by `ContentInfo` rather than the `Model`. `InitializeRoutes` gains the port as a parameter — it cannot construct the adapter itself from its `db` argument without importing `mediavariant`, which is the thing the port exists to avoid.

`Cache-Control: private, max-age=300` is set unconditionally, so it applies to variants too (FR-7.7, NFR-5). No `Vary` header is added: the variant is in the query string, so distinct variants are already distinct cache keys.

**Files:**
- Modify: `apps/media-service/internal/mediaobject/resource.go:44-45` and `:148-178`
- Modify: `apps/media-service/cmd/main.go`
- Test: `apps/media-service/internal/mediaobject/resource_test.go`

**Interfaces:**
- Consumes: `ParseContentVariant` (Task 3); `VariantLookup`, `VariantRef`, `ContentInfo`, the new `NewProcessor` and `Content` signatures (Task 4); `mediavariant.Provider.GetByMediaObjectAndVariant` (Task 2).
- Produces: `mediaobject.InitializeRoutes(log logrus.FieldLogger, db *gorm.DB, st ObjectStore, variants VariantLookup, maxUploadBytes int64) func(chi.Router)` — a signature change with one production call site (`cmd/main.go:124`). Frontend Task 6 depends on the HTTP contract this establishes.

- [ ] **Step 1: Update `InitializeRoutes` and the content handler**

In `apps/media-service/internal/mediaobject/resource.go`, change the signature and processor construction (`:44-45`) to:

```go
func InitializeRoutes(log logrus.FieldLogger, db *gorm.DB, st ObjectStore, variants VariantLookup, maxUploadBytes int64) func(chi.Router) {
	proc := NewProcessor(log, NewProvider(db), NewAdministrator(db), st, variants)
```

Then replace the whole `GET /media/{id}/content` handler (`:145-178`) with:

```go
		// GET /media/{id}/content — stream the bytes after authz. Proxied, not
		// presigned: MinIO is a shared cluster service and is never exposed
		// outside the cluster.
		//
		// The optional ?variant= parameter selects a stored rendition
		// (thumbnail/display); omitting it serves the original, byte-for-byte
		// as before. An unrecognised value is a 400, never a silent fallback —
		// a typo would otherwise ship multi-megabyte responses undetected.
		r.Get("/media/{id}/content", func(w http.ResponseWriter, req *http.Request) {
			identity := auth.IdentityFromContext(req.Context())
			id := chi.URLParam(req, "id")
			v, err := ParseContentVariant(req.URL.Query().Get("variant"))
			if err != nil {
				server.WriteError(w, err)
				return
			}
			info, rc, err := proc.Content(req.Context(), id, identity.ActiveFleetID, v)
			if err != nil {
				server.WriteError(w, err)
				return
			}
			defer func() { _ = rc.Close() }()

			// proc.Content has already proven the object is readable (the
			// store issues its GET before returning), so committing 200 here
			// no longer risks answering an empty body for bytes that were
			// never stored. A copy that fails *after* this point is a genuine
			// mid-stream failure; the status is on the wire and cannot be
			// changed, so it is logged and the client sees a short read.
			//
			// Headers come from ContentInfo, i.e. from the bytes about to be
			// written — a variant is re-encoded and has its own content type,
			// and its length is unknown, so Content-Length is omitted for it
			// rather than describing the original.
			if info.ContentType != "" {
				w.Header().Set("Content-Type", info.ContentType)
			}
			if info.Size > 0 {
				w.Header().Set("Content-Length", strconv.FormatInt(info.Size, 10))
			}
			// Per-fleet authorized bytes — never store in a shared cache.
			w.Header().Set("Cache-Control", "private, max-age=300")
			w.WriteHeader(http.StatusOK)
			if _, err := io.Copy(w, rc); err != nil {
				// Headers are already written, so the status cannot be changed;
				// log and let the client see a truncated body.
				log.WithError(err).Warn("media content stream interrupted")
			}
		})
```

- [ ] **Step 2: Update the test router helper**

In `apps/media-service/internal/mediaobject/resource_test.go`, replace `testRouter` (`:56-67`) with:

```go
// testRouter mounts the real routes over an in-memory DB and the supplied
// store, with no stored variants — the shape every pre-variant test needs.
func testRouter(t *testing.T, store ObjectStore, maxUploadBytes int64) (http.Handler, *Processor) {
	t.Helper()
	return testRouterWithVariants(t, store, &fakeVariants{}, maxUploadBytes)
}

// testRouterWithVariants mounts the real routes with a variant lookup under the
// test's control, and injects the identity the JWT middleware would normally put
// on the context so the handlers can be exercised end to end.
func testRouterWithVariants(t *testing.T, store ObjectStore, variants VariantLookup, maxUploadBytes int64) (http.Handler, *Processor) {
	t.Helper()
	db := newConfirmTestDB(t)
	log := logrus.New()
	log.SetOutput(io.Discard)
	r := chi.NewRouter()
	r.Group(InitializeRoutes(log, db, store, variants, maxUploadBytes))
	return r, NewProcessor(log, NewProvider(db), NewAdministrator(db), store, variants)
}
```

- [ ] **Step 3: Write the failing HTTP tests**

Append to `apps/media-service/internal/mediaobject/resource_test.go`:

```go
// seedStoredObject creates a media object and records its bytes, returning the
// media id — the state a completed upload leaves behind.
func seedStoredObject(t *testing.T, pr *Processor, fleetID string, payload []byte) string {
	t.Helper()
	created, err := pr.InitUpload(fleetID, "u1", "image/png", "photo.png")
	if err != nil {
		t.Fatalf("init upload: %v", err)
	}
	if _, err := pr.StoreContent(context.Background(), created.ID(), fleetID, bytes.NewReader(payload), int64(len(payload))); err != nil {
		t.Fatalf("store content: %v", err)
	}
	return created.ID()
}

func thumbnailRouter(t *testing.T) (http.Handler, *Processor, *fakeStore) {
	t.Helper()
	store := &fakeStore{
		bucket:  "myfleet-media",
		getBody: []byte("original-bytes"),
		getBodies: map[string][]byte{
			"fleet-a/thumb.jpg":   []byte("thumb-bytes"),
			"fleet-a/display.jpg": []byte("display-bytes"),
		},
	}
	variants := &fakeVariants{refs: map[string]VariantRef{
		"thumbnail": {ObjectKey: "fleet-a/thumb.jpg", ContentType: "image/jpeg"},
		"display":   {ObjectKey: "fleet-a/display.jpg", ContentType: "image/jpeg"},
	}}
	router, proc := testRouterWithVariants(t, store, variants, 1024)
	return router, proc, store
}

// TestGetContent_noVariantParamIsUnchanged is the backwards-compatibility gate:
// a request with no ?variant= must behave exactly as it did before this feature,
// Content-Length included. Every existing caller depends on it.
func TestGetContent_noVariantParamIsUnchanged(t *testing.T) {
	router, proc, _ := thumbnailRouter(t)
	id := seedStoredObject(t, proc, "fleet-a", []byte("original-bytes"))

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, memberRequest(http.MethodGet, "/media/"+id+"/content", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "original-bytes" {
		t.Fatalf("body = %q, want original-bytes", rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/png" {
		t.Fatalf("Content-Type = %q, want image/png", ct)
	}
	if cl := rec.Header().Get("Content-Length"); cl != "14" {
		t.Fatalf("Content-Length = %q, want 14 (len(\"original-bytes\"))", cl)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "private, max-age=300" {
		t.Fatalf("Cache-Control = %q, want private, max-age=300", cc)
	}
}

// TestGetContent_thumbnailServesVariantWithoutContentLength is the request the
// vehicles list makes. media_variants records no byte count, so sending the
// ORIGINAL's Content-Length here would truncate or hang the response.
func TestGetContent_thumbnailServesVariantWithoutContentLength(t *testing.T) {
	router, proc, _ := thumbnailRouter(t)
	id := seedStoredObject(t, proc, "fleet-a", []byte("original-bytes"))

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, memberRequest(http.MethodGet, "/media/"+id+"/content?variant=thumbnail", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "thumb-bytes" {
		t.Fatalf("body = %q, want thumb-bytes", rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/jpeg" {
		t.Fatalf("Content-Type = %q, want the variant's own image/jpeg", ct)
	}
	if cl := rec.Header().Get("Content-Length"); cl != "" {
		t.Fatalf("Content-Length = %q, want it omitted — that value describes the original, not the variant", cl)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "private, max-age=300" {
		t.Fatalf("Cache-Control = %q, want private, max-age=300 on a variant response too", cc)
	}
}

func TestGetContent_displayAndOriginalVariantsAreAccepted(t *testing.T) {
	router, proc, _ := thumbnailRouter(t)
	id := seedStoredObject(t, proc, "fleet-a", []byte("original-bytes"))

	for _, tc := range []struct{ variant, want string }{
		{"display", "display-bytes"},
		{"original", "original-bytes"},
	} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, memberRequest(http.MethodGet, "/media/"+id+"/content?variant="+tc.variant, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("?variant=%s status = %d, want 200; body: %s", tc.variant, rec.Code, rec.Body.String())
		}
		if rec.Body.String() != tc.want {
			t.Fatalf("?variant=%s body = %q, want %q", tc.variant, rec.Body.String(), tc.want)
		}
	}
}

// TestGetContent_variantWithNoRowFallsBackToOriginal: the normal state for a
// media object whose processing has not completed yet.
func TestGetContent_variantWithNoRowFallsBackToOriginal(t *testing.T) {
	store := &fakeStore{bucket: "myfleet-media", getBody: []byte("original-bytes")}
	router, proc := testRouterWithVariants(t, store, &fakeVariants{}, 1024)
	id := seedStoredObject(t, proc, "fleet-a", []byte("original-bytes"))

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, memberRequest(http.MethodGet, "/media/"+id+"/content?variant=thumbnail", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "original-bytes" {
		t.Fatalf("body = %q, want the original as a fallback", rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/png" {
		t.Fatalf("Content-Type = %q, want the media object's own image/png on a fallback", ct)
	}
}

// TestGetContent_bogusVariantIs400WithNoBytes: a typo must be loud, not a
// silent multi-megabyte download.
func TestGetContent_bogusVariantIs400WithNoBytes(t *testing.T) {
	router, proc, store := thumbnailRouter(t)
	id := seedStoredObject(t, proc, "fleet-a", []byte("original-bytes"))
	callsBefore := len(store.getCalls)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, memberRequest(http.MethodGet, "/media/"+id+"/content?variant=bogus", nil))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"errors"`) || !strings.Contains(rec.Body.String(), `"bad_request"`) {
		t.Fatalf("body = %s, want a JSON:API error envelope with code bad_request", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "original-bytes") || strings.Contains(rec.Body.String(), "thumb-bytes") {
		t.Fatalf("a rejected variant returned image bytes: %s", rec.Body.String())
	}
	if len(store.getCalls) != callsBefore {
		t.Fatalf("a rejected variant touched storage: %v", store.getCalls[callsBefore:])
	}
}

// TestGetContent_variantCrossFleetIs404WithNoStoreRead: a variant must never be
// reachable by a caller who could not read the original, and the 404 (not 403)
// keeps cross-fleet existence unleakable.
func TestGetContent_variantCrossFleetIs404WithNoStoreRead(t *testing.T) {
	store := &fakeStore{bucket: "myfleet-media", getBody: []byte("original-bytes")}
	variants := &fakeVariants{refs: map[string]VariantRef{
		"thumbnail": {ObjectKey: "fleet-b/thumb.jpg", ContentType: "image/jpeg"},
	}}
	router, proc := testRouterWithVariants(t, store, variants, 1024)
	id := seedStoredObject(t, proc, "fleet-b", []byte("original-bytes")) // other fleet
	callsBefore := len(store.getCalls)

	rec := httptest.NewRecorder()
	// memberRequest carries ActiveFleetID "fleet-a".
	router.ServeHTTP(rec, memberRequest(http.MethodGet, "/media/"+id+"/content?variant=thumbnail", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for a cross-fleet variant read", rec.Code)
	}
	if len(store.getCalls) != callsBefore {
		t.Fatalf("cross-fleet variant read touched storage: %v", store.getCalls[callsBefore:])
	}
	if len(variants.calls) != 0 {
		t.Fatalf("cross-fleet variant read ran the variant lookup: %v", variants.calls)
	}
}
```

- [ ] **Step 4: Run the tests to verify they fail**

Run: `go test github.com/jtumidanski/myfleet/apps/media-service/internal/mediaobject/... -run TestGetContent -v`
Expected: FAIL — the package does not compile until Steps 1-2 are in place; once they are, the new tests fail on assertions if the handler is wrong. If Steps 1-2 are already applied, run this to see them pass in Step 5 instead.

- [ ] **Step 5: Run the whole media-service package suite**

Run: `go test github.com/jtumidanski/myfleet/apps/media-service/...`
Expected: PASS. `cmd/main.go` still calls the old `InitializeRoutes` signature, so `go build` will fail until Step 6.

- [ ] **Step 6: Wire the adapter in the composition root**

In `apps/media-service/cmd/main.go`, add the adapter type at the bottom of the file (after `purgeExpired`):

```go
// variantLookup adapts mediavariant.Provider to mediaobject.VariantLookup.
//
// It lives here, in the composition root — the one place that already imports
// both packages — so that mediaobject never imports mediavariant and the two
// sibling domain packages stay independent (design §3.1).
type variantLookup struct{ p mediavariant.Provider }

func (v variantLookup) Lookup(mediaObjectID, variant string) (mediaobject.VariantRef, bool, error) {
	m, found, err := v.p.GetByMediaObjectAndVariant(mediaObjectID, mediavariant.Variant(variant))
	if err != nil || !found {
		return mediaobject.VariantRef{}, false, err
	}
	return mediaobject.VariantRef{ObjectKey: m.ObjectKey(), ContentType: m.ContentType()}, true, nil
}
```

and change the route registration (`:124`) to:

```go
					mediaobject.InitializeRoutes(log, db, store, variantLookup{p: mediavariant.NewProvider(db)}, maxUploadBytes)(pr)
```

- [ ] **Step 7: Verify the whole Go workspace builds and passes**

Run: `make vet && make test && make build`
Expected: all PASS.

- [ ] **Step 8: Commit**

```bash
git add apps/media-service/internal/mediaobject/resource.go apps/media-service/internal/mediaobject/resource_test.go apps/media-service/cmd/main.go
git commit -m "feat(media-service): serve thumbnail/display variants from GET /media/{id}/content"
```

---

## Task 6: Frontend media-variant plumbing

Four small, mechanical changes so the card can ask for `thumbnail` while every existing caller keeps requesting originals.

The parameter is **omitted** rather than sent as `?variant=original`, so every existing request stays byte-identical on the wire.

The query key becomes variant-aware because a thumbnail and an original for the same media id hold different bytes; without the variant in the key one would be served in place of the other. The `mediaKeys.contents()` prefix is unchanged, so any prefix-based invalidation still matches, and the cache is in-memory only so the key-shape change needs no migration.

`MediaThumbnail` is deliberately **not** changed: the detail gallery keeps requesting originals (design §10), which is now a one-line fix whenever that page is next touched.

**Files:**
- Modify: `apps/web/src/types/models/media.ts`
- Modify: `apps/web/src/services/api/MediaService.ts:54-57`
- Modify: `apps/web/src/lib/hooks/api/media.ts:13-21` and `:120-152`
- Test: `apps/web/src/lib/hooks/api/media.test.ts:23`

**Interfaces:**
- Consumes: the HTTP contract from Task 5.
- Produces:
  - `MediaVariant = 'original' | 'thumbnail' | 'display'` from `types/models/media`
  - `mediaService.getContentBlob(id: string, variant?: MediaVariant): Promise<Blob>`
  - `mediaKeys.content(id: string, variant?: MediaVariant)` → `['media','content',id,variant]`
  - `useMediaContentUrl(id: string | null | undefined, variant?: MediaVariant)` → `{ url: string | null; isLoading: boolean; isError: boolean; error: unknown }`
  Task 10's `VehiclePhotoThumbnail` calls `useMediaContentUrl(mediaId, 'thumbnail')`.

- [ ] **Step 1: Add the type**

In `apps/web/src/types/models/media.ts`, insert above `MediaObjectAttributes`:

```ts
/**
 * Renditions served by GET /api/media/{id}/content?variant=…
 * (apps/media-service/internal/mediaobject/contentvariant.go). Omitting the
 * parameter means 'original'; the list view asks for 'thumbnail' so a card
 * costs kilobytes rather than the full-size upload.
 */
export type MediaVariant = 'original' | 'thumbnail' | 'display';
```

- [ ] **Step 2: Update the failing key assertion**

In `apps/web/src/lib/hooks/api/media.test.ts`, replace line 23 with:

```ts
    expect(mediaKeys.content('m1')).toEqual(['media', 'content', 'm1', 'original']);
    expect(mediaKeys.content('m1', 'thumbnail')).toEqual(['media', 'content', 'm1', 'thumbnail']);
```

- [ ] **Step 3: Run the test to verify it fails**

```sh
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
npm run -w apps/web test -- src/lib/hooks/api/media.test.ts
```
Expected: FAIL — `expected ['media','content','m1'] to deeply equal ['media','content','m1','original']`.

- [ ] **Step 4: Thread the variant through the service**

In `apps/web/src/services/api/MediaService.ts`, add `MediaVariant` to the type import on line 3:

```ts
import type {
  MediaObjectAttributes,
  InitMediaUploadAttributes,
  MediaVariant,
} from '../../types/models/media';
```

and replace `getContentBlob` (`:54-57`) with:

```ts
  /**
   * GET /api/media/{id}/content — the raw bytes, authenticated.
   *
   * `original` sends no query parameter at all, so every pre-existing caller's
   * request stays byte-identical on the wire.
   */
  async getContentBlob(id: string, variant: MediaVariant = 'original'): Promise<Blob> {
    const suffix = variant === 'original' ? '' : `?variant=${variant}`;
    return apiClient.requestBlob(`${this.basePath}/${id}/content${suffix}`);
  }
```

Also update the route comment block at the top of the class (line 12) to:

```ts
 *   GET    /api/media/{id}/content  — stream the bytes (proxied from MinIO);
 *                                     optional ?variant=thumbnail|display
```

- [ ] **Step 5: Make the key and hook variant-aware**

In `apps/web/src/lib/hooks/api/media.ts`, add the type import to line 6:

```ts
import type {
  MediaObjectAttributes,
  InitMediaUploadAttributes,
  MediaVariant,
} from '../../../types/models/media';
```

Replace the key-factory comment and `content` entry (`:8-21`) with:

```ts
// Hierarchical query-key factory.
// all                       -> ['media']
// detail('m1')              -> ['media', 'detail', 'm1']
// content('m1')             -> ['media', 'content', 'm1', 'original']
// content('m1','thumbnail') -> ['media', 'content', 'm1', 'thumbnail']
// vehicleMedia(vehicleId)   -> ['media', 'vehicle', vehicleId]
export const mediaKeys = {
  all: ['media'] as const,
  details: () => [...mediaKeys.all, 'detail'] as const,
  detail: (id: string) => [...mediaKeys.details(), id] as const,
  contents: () => [...mediaKeys.all, 'content'] as const,
  // The variant is part of the key because a thumbnail and an original for the
  // same media id hold different bytes; without it one would be served in place
  // of the other. The `contents()` prefix is unchanged, so prefix-based
  // invalidation still matches every variant of an id.
  content: (id: string, variant: MediaVariant = 'original') =>
    [...mediaKeys.contents(), id, variant] as const,
  vehicleMediaAll: () => [...mediaKeys.all, 'vehicle'] as const,
  vehicleMedia: (vehicleId: string) => [...mediaKeys.vehicleMediaAll(), vehicleId] as const,
};
```

Then change the `useMediaContentUrl` signature and query (`:120-127`) to:

```ts
export function useMediaContentUrl(
  id: string | null | undefined,
  variant: MediaVariant = 'original',
) {
  const { data, isLoading, isError, error } = useQuery({
    queryKey: mediaKeys.content(id ?? '', variant),
    queryFn: () => mediaService.getContentBlob(id as string, variant),
    enabled: !!id,
    staleTime: 5 * 60 * 1000,
    gcTime: 6 * 60 * 1000,
  });
```

Leave the rest of the hook body — the object-URL effect and the one-frame `isLoading` hold — untouched. That behaviour is exactly what keeps `VehiclePhotoThumbnail` from flashing a placeholder, and the 5-minute `staleTime` is what satisfies FR-2.5 and NFR-2.

- [ ] **Step 6: Run the tests to verify they pass**

```sh
npm run -w apps/web test -- src/lib/hooks/api/media.test.ts
```
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add apps/web/src/types/models/media.ts apps/web/src/services/api/MediaService.ts apps/web/src/lib/hooks/api/media.ts apps/web/src/lib/hooks/api/media.test.ts
git commit -m "feat(web): plumb a media variant through the service, query key, and content hook"
```

---

## Task 7: Runtime configuration (`config.json`)

Vite inlines `import.meta.env` at build time, so a build-time template would mean rebuilding and republishing the `web` image and re-pinning the overlay digest — a full release cycle to edit one URL. Instead the SPA fetches a small JSON document before mounting.

This is the first runtime-config mechanism in the frontend, so it is built as a **general facility with exactly one key today**, not as a Carfax special case.

**Nothing about a config failure may prevent the app from rendering.** Every failure path resolves to `DEFAULT_RUNTIME_CONFIG`, which is itself a correct Carfax URL, so the feature keeps working regardless. The 2s abort timeout matters more than it looks: without it a wedged nginx worker turns a missing 60-byte file into a permanent white screen.

A module-level latched value is used rather than React context, because the Carfax URL builder must stay a pure function (NFR-15); threading context into it would make it a hook and its tests a render harness.

**Files:**
- Create: `apps/web/src/lib/config/runtimeConfig.ts`
- Create: `apps/web/src/lib/config/runtimeConfig.test.ts`
- Create: `apps/web/public/config/config.json`
- Modify: `apps/web/src/main.tsx`
- Modify: `apps/web/nginx.conf`

**Interfaces:**
- Consumes: `zod` (already a dependency).
- Produces:
  - `RuntimeConfig { carfaxUrlTemplate: string }`
  - `DEFAULT_RUNTIME_CONFIG: RuntimeConfig`
  - `parseRuntimeConfig(raw: unknown): RuntimeConfig` — pure, never throws
  - `loadRuntimeConfig(): Promise<RuntimeConfig>` — never throws, never rejects
  - `getRuntimeConfig(): RuntimeConfig` — the latched value; defaults until `loadRuntimeConfig` resolves
  Task 11's `VehicleCard` calls `getRuntimeConfig().carfaxUrlTemplate`.

- [ ] **Step 1: Write the failing tests**

Create `apps/web/src/lib/config/runtimeConfig.test.ts`:

```ts
import { describe, it, expect, vi, afterEach } from 'vitest';
import {
  DEFAULT_RUNTIME_CONFIG,
  getRuntimeConfig,
  loadRuntimeConfig,
  parseRuntimeConfig,
} from './runtimeConfig';

const CUSTOM = 'https://example.test/report?vin={vin}';

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

// parseRuntimeConfig is pure, so it is tested without touching fetch or the
// module's latched state.
describe('parseRuntimeConfig', () => {
  it('accepts a valid document', () => {
    expect(parseRuntimeConfig({ carfaxUrlTemplate: CUSTOM })).toEqual({
      carfaxUrlTemplate: CUSTOM,
    });
  });

  it('falls back per field rather than discarding the document', () => {
    expect(parseRuntimeConfig({ carfaxUrlTemplate: 42 })).toEqual(DEFAULT_RUNTIME_CONFIG);
    expect(parseRuntimeConfig({ carfaxUrlTemplate: '' })).toEqual(DEFAULT_RUNTIME_CONFIG);
    expect(parseRuntimeConfig({})).toEqual(DEFAULT_RUNTIME_CONFIG);
  });

  it('ignores unknown keys', () => {
    expect(parseRuntimeConfig({ carfaxUrlTemplate: CUSTOM, somethingElse: true })).toEqual({
      carfaxUrlTemplate: CUSTOM,
    });
  });

  it('never throws on a non-object', () => {
    expect(parseRuntimeConfig(null)).toEqual(DEFAULT_RUNTIME_CONFIG);
    expect(parseRuntimeConfig('nope')).toEqual(DEFAULT_RUNTIME_CONFIG);
    expect(parseRuntimeConfig(undefined)).toEqual(DEFAULT_RUNTIME_CONFIG);
  });
});

describe('loadRuntimeConfig', () => {
  it('latches a served document so getRuntimeConfig returns it', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => new Response(JSON.stringify({ carfaxUrlTemplate: CUSTOM }), { status: 200 })),
    );

    await expect(loadRuntimeConfig()).resolves.toEqual({ carfaxUrlTemplate: CUSTOM });
    expect(getRuntimeConfig()).toEqual({ carfaxUrlTemplate: CUSTOM });
  });

  // Each of these must resolve to the defaults, never reject: a config failure
  // must not be able to stop the app from rendering.
  it('falls back to defaults on a 404 (no ConfigMap, older image)', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response('not found', { status: 404 })));
    vi.spyOn(console, 'warn').mockImplementation(() => {});

    await expect(loadRuntimeConfig()).resolves.toEqual(DEFAULT_RUNTIME_CONFIG);
    expect(getRuntimeConfig()).toEqual(DEFAULT_RUNTIME_CONFIG);
  });

  it('falls back to defaults on a network error', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => {
        throw new TypeError('Failed to fetch');
      }),
    );
    vi.spyOn(console, 'warn').mockImplementation(() => {});

    await expect(loadRuntimeConfig()).resolves.toEqual(DEFAULT_RUNTIME_CONFIG);
  });

  it('falls back to defaults on malformed JSON', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response('{ not json', { status: 200 })));
    vi.spyOn(console, 'warn').mockImplementation(() => {});

    await expect(loadRuntimeConfig()).resolves.toEqual(DEFAULT_RUNTIME_CONFIG);
  });

  it('gives up rather than hanging when the request never settles', async () => {
    // A wedged server would otherwise leave the app unmounted forever. The
    // abort signal is what breaks the deadlock.
    vi.stubGlobal(
      'fetch',
      vi.fn(
        (_url: string, init?: RequestInit) =>
          new Promise<Response>((_resolve, reject) => {
            init?.signal?.addEventListener('abort', () => reject(new Error('aborted')));
          }),
      ),
    );
    vi.spyOn(console, 'warn').mockImplementation(() => {});
    vi.useFakeTimers();

    const pending = loadRuntimeConfig();
    await vi.advanceTimersByTimeAsync(2000);
    await expect(pending).resolves.toEqual(DEFAULT_RUNTIME_CONFIG);

    vi.useRealTimers();
  });
});
```

- [ ] **Step 2: Run the tests to verify they fail**

```sh
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
npm run -w apps/web test -- src/lib/config/runtimeConfig.test.ts
```
Expected: FAIL — cannot resolve `./runtimeConfig`.

- [ ] **Step 3: Write the module**

Create `apps/web/src/lib/config/runtimeConfig.ts`:

```ts
import { z } from 'zod';

/**
 * Configuration the SPA reads at runtime rather than at build time.
 *
 * Vite inlines `import.meta.env` into the bundle, so anything configured that
 * way can only change by rebuilding and republishing the web image. This module
 * fetches a small JSON document instead, served by the SPA's own nginx and
 * backed by a ConfigMap in Kubernetes, so a value can be edited with
 * `kubectl apply` alone.
 *
 * It is deliberately built as a general facility with one key today rather than
 * as a Carfax special case, so the second key does not require redesigning it.
 */
export interface RuntimeConfig {
  carfaxUrlTemplate: string;
}

/**
 * Compiled into the bundle, so the app works with no ConfigMap at all: local
 * `vite dev`, a bare `docker run` of the image, and any overlay that has not
 * adopted the ConfigMap.
 */
export const DEFAULT_RUNTIME_CONFIG: RuntimeConfig = {
  carfaxUrlTemplate: 'https://www.carfax.com/VehicleHistory/p/Report.cfx?vin={vin}',
};

const RUNTIME_CONFIG_URL = '/config/config.json';

/**
 * Without a bound, a wedged nginx worker turns a missing 60-byte file into a
 * permanent white screen, because nothing renders until this settles.
 */
const FETCH_TIMEOUT_MS = 2000;

// `.catch()` per field, then once more on the object, is what makes a partially
// broken document degrade rather than being discarded: one malformed key falls
// back on its own while the rest of the document is honoured. Unknown keys are
// stripped by zod's default object behaviour.
const runtimeConfigSchema = z
  .object({
    carfaxUrlTemplate: z.string().min(1).catch(DEFAULT_RUNTIME_CONFIG.carfaxUrlTemplate),
  })
  .catch(DEFAULT_RUNTIME_CONFIG);

/** Validates one parsed document, falling back per field. Never throws. */
export function parseRuntimeConfig(raw: unknown): RuntimeConfig {
  return runtimeConfigSchema.parse(raw);
}

let latched: RuntimeConfig = DEFAULT_RUNTIME_CONFIG;

/**
 * The latched config. Returns the compiled-in defaults until loadRuntimeConfig
 * has resolved, which is why the defaults must be a usable value rather than
 * placeholders.
 */
export function getRuntimeConfig(): RuntimeConfig {
  return latched;
}

/**
 * Fetches, parses, and latches the config. Never throws and never rejects —
 * every failure (404, network error, malformed body, hung request) resolves to
 * DEFAULT_RUNTIME_CONFIG and logs a single warning. Nothing about a config
 * failure may prevent the app from rendering.
 */
export async function loadRuntimeConfig(): Promise<RuntimeConfig> {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), FETCH_TIMEOUT_MS);
  try {
    const res = await fetch(RUNTIME_CONFIG_URL, {
      signal: controller.signal,
      cache: 'no-store',
    });
    if (!res.ok) {
      throw new Error(`config request returned ${res.status}`);
    }
    latched = parseRuntimeConfig(await res.json());
  } catch (err) {
    console.warn('[runtime-config] using built-in defaults:', err);
    latched = DEFAULT_RUNTIME_CONFIG;
  } finally {
    clearTimeout(timer);
  }
  return latched;
}
```

- [ ] **Step 4: Run the tests to verify they pass**

```sh
npm run -w apps/web test -- src/lib/config/runtimeConfig.test.ts
```
Expected: PASS (all 9 cases).

- [ ] **Step 5: Add the baked-in default document**

Create `apps/web/public/config/config.json`:

```json
{
  "carfaxUrlTemplate": "https://www.carfax.com/VehicleHistory/p/Report.cfx?vin={vin}"
}
```

Vite serves `public/` at the site root in dev and copies it into `dist/` on build, and the Dockerfile copies `dist` to `/usr/share/nginx/html` — so this lands at `/config/config.json` in every environment. The Kubernetes ConfigMap (Task 8) mounts over the directory and replaces it.

- [ ] **Step 6: Stop browsers caching the config**

In `apps/web/nginx.conf`, add inside the `server` block, **above** the `location /` block:

```nginx
    # Runtime configuration, replaced in Kubernetes by the `web-config`
    # ConfigMap mounted over this directory. It must not be cached: a browser
    # serving a stale template long after the ConfigMap changed would quietly
    # defeat the entire point of choosing runtime configuration over a
    # build-time value.
    location = /config/config.json {
        add_header Cache-Control "no-cache";
    }
```

- [ ] **Step 7: Load the config before mounting**

Replace `apps/web/src/main.tsx` with:

```tsx
import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { AppProviders } from './components/providers/AppProviders';
import { App } from './App';
import { loadRuntimeConfig } from './lib/config/runtimeConfig';
import './index.css';

const rootElement = document.getElementById('root');
if (!rootElement) throw new Error('Root element #root not found');

// Runtime config is latched before the tree mounts so synchronous readers
// (getRuntimeConfig) never observe a half-initialised value.
//
// `.then` rather than `.catch` is the mechanism on purpose: loadRuntimeConfig
// catches everything internally and always resolves, so this callback always
// runs and the app always renders — with the compiled-in defaults if the fetch
// failed. It costs one same-origin request ahead of first paint, which is not a
// new class of delay: the app already gates its first meaningful render on the
// auth bootstrap.
void loadRuntimeConfig().then(() => {
  createRoot(rootElement).render(
    <StrictMode>
      <AppProviders>
        <App />
      </AppProviders>
    </StrictMode>,
  );
});
```

- [ ] **Step 8: Verify lint, tests, and the build**

```sh
npm run -w apps/web lint && npm run -w apps/web test && npm run -w apps/web build
```
Expected: all PASS. Confirm the default document was emitted:
`ls apps/web/dist/config/config.json`

- [ ] **Step 9: Commit**

```bash
git add apps/web/src/lib/config/runtimeConfig.ts apps/web/src/lib/config/runtimeConfig.test.ts apps/web/public/config/config.json apps/web/src/main.tsx apps/web/nginx.conf
git commit -m "feat(web): add a runtime config document fetched before the app mounts"
```

---

## Task 8: Ship the config as a ConfigMap

The ConfigMap is mounted as a **directory**, replacing the baked-in one, rather than with `subPath`. `subPath` mounts do not receive ConfigMap updates without a pod restart, and the manifests use plain `configmap.yaml` files rather than a hash-suffixed `configMapGenerator` — so a `kubectl apply` of an edited ConfigMap would otherwise leave every pod serving the old value with no signal that anything had happened.

`readOnlyRootFilesystem: true` is unaffected: ConfigMap volumes are read-only mounts.

The stored value is the **real default**, not a placeholder, so the `main` overlay stays placeholder-free (`tools/check-manifests.sh` greps for `REPLACE_ME`). No Secret, no PVC, no ClusterRole — the `main` overlay's constraints are untouched.

**Files:**
- Create: `deploy/k8s/base/web/configmap.yaml`
- Modify: `deploy/k8s/base/web/deployment.yaml`
- Modify: `deploy/k8s/base/kustomization.yaml`

**Interfaces:**
- Consumes: the `/config/config.json` URL and document shape from Task 7.
- Produces: ConfigMap `web-config` with key `config.json`, mounted at `/usr/share/nginx/html/config`.

- [ ] **Step 1: Create the ConfigMap**

Create `deploy/k8s/base/web/configmap.yaml`:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: web-config
  labels:
    app: web
data:
  # Served by the SPA's own nginx at /config/config.json and read once, before
  # the React tree mounts (apps/web/src/lib/config/runtimeConfig.ts). Editing
  # this value takes effect on a `kubectl apply` — no image rebuild — which is
  # the whole reason it is not an inlined Vite env var.
  #
  # The volume is mounted as a directory rather than with subPath so kubelet
  # propagates updates to running pods.
  config.json: |
    {
      "carfaxUrlTemplate": "https://www.carfax.com/VehicleHistory/p/Report.cfx?vin={vin}"
    }
```

- [ ] **Step 2: Mount it**

In `deploy/k8s/base/web/deployment.yaml`, extend `volumeMounts` and `volumes`:

```yaml
          volumeMounts:
            # nginx writes its pid and temp bodies at runtime; the root
            # filesystem is read-only, so give it tmpfs.
            - name: nginx-tmp
              mountPath: /tmp
            # Runtime config. Mounted over the directory the image bakes in
            # (not with subPath) so a ConfigMap edit reaches running pods.
            - name: web-config
              mountPath: /usr/share/nginx/html/config
              readOnly: true
      volumes:
        - name: nginx-tmp
          emptyDir: {}
        - name: web-config
          configMap:
            name: web-config
```

- [ ] **Step 3: Register it with kustomize**

In `deploy/k8s/base/kustomization.yaml`, change the `web` block to:

```yaml
  - web/configmap.yaml
  - web/deployment.yaml
  - web/service.yaml
```

- [ ] **Step 4: Render and check the invariants**

```sh
make manifests
```
Expected: PASS — both overlays render, `main` still has zero PVCs/Secrets/ClusterRoles/ClusterRoleBindings and no `REPLACE_ME`, and the IngressRoute route sets stay identical.

Then confirm the ConfigMap actually lands in both overlays:

```sh
kustomize build deploy/k8s/overlays/main  | grep -A 6 'name: web-config'
kustomize build deploy/k8s/overlays/local | grep -A 6 'name: web-config'
```
Expected: the ConfigMap and the deployment's volume reference appear in both.

- [ ] **Step 5: Server dry-run against the cluster**

```sh
kustomize build deploy/k8s/overlays/main  | kubectl apply --dry-run=server -f -
kustomize build deploy/k8s/overlays/local | kubectl apply --dry-run=server -f -
```
Expected: every object reports `(server dry run)` with no error. `--dry-run=server` validates against the API server without persisting anything, so it is safe against the shared `bee` context. **Rendering alone does not catch namespace or cross-resource-reference errors — do not skip the local overlay.** If no cluster is reachable, say so explicitly in the task report rather than marking this step done.

- [ ] **Step 6: Commit**

```bash
git add deploy/k8s/base/web/configmap.yaml deploy/k8s/base/web/deployment.yaml deploy/k8s/base/kustomization.yaml
git commit -m "feat(deploy): serve the web runtime config from a ConfigMap volume"
```

---

## Task 9: The Carfax URL builder

A pure function with no React and no config import — the template arrives as an argument, which is what keeps it directly unit-testable (NFR-15) and keeps its tests away from the latched singleton.

It returns `null` to mean **render no button**, for four reasons: no VIN, a template that ignores the VIN, a URL that does not parse, and a scheme that is not `https:`.

The scheme check exists because moving the template out of the bundle means whoever can edit the ConfigMap chooses the URL an anchor's `href` points at; a `javascript:` template would otherwise be stored XSS. `https:`-only is deliberately strict — permitting `http:` would let a config change silently downgrade a link that carries a VIN. Relaxing it later is a one-line change.

**Files:**
- Create: `apps/web/src/lib/carfax.ts`
- Create: `apps/web/src/lib/carfax.test.ts`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `VIN_PLACEHOLDER = '{vin}'`
  - `buildCarfaxUrl(template: string, vin: string | null | undefined): string | null`
  Task 11's `VehicleCard` calls it with `getRuntimeConfig().carfaxUrlTemplate`.

- [ ] **Step 1: Write the failing tests**

Create `apps/web/src/lib/carfax.test.ts`:

```ts
import { describe, it, expect } from 'vitest';
import { buildCarfaxUrl, VIN_PLACEHOLDER } from './carfax';

const TEMPLATE = 'https://www.carfax.com/VehicleHistory/p/Report.cfx?vin={vin}';

describe('buildCarfaxUrl', () => {
  it('substitutes the VIN', () => {
    expect(buildCarfaxUrl(TEMPLATE, '1HGCM82633A004352')).toBe(
      'https://www.carfax.com/VehicleHistory/p/Report.cfx?vin=1HGCM82633A004352',
    );
  });

  it('URL-encodes the VIN', () => {
    // A VIN is interpolated into a URL and is never used to build markup, so
    // encoding is the whole defence against a value with reserved characters.
    expect(buildCarfaxUrl(TEMPLATE, 'A B&C=D')).toBe(
      'https://www.carfax.com/VehicleHistory/p/Report.cfx?vin=A%20B%26C%3DD',
    );
  });

  it('trims surrounding whitespace before substituting', () => {
    expect(buildCarfaxUrl(TEMPLATE, '  1HGCM82633A004352  ')).toBe(
      'https://www.carfax.com/VehicleHistory/p/Report.cfx?vin=1HGCM82633A004352',
    );
  });

  it('replaces every occurrence of the placeholder', () => {
    expect(buildCarfaxUrl('https://x.test/{vin}/report?vin={vin}', 'ABC')).toBe(
      'https://x.test/ABC/report?vin=ABC',
    );
  });

  // null means "render no button" — each of these must produce it.
  it('returns null without a usable VIN', () => {
    expect(buildCarfaxUrl(TEMPLATE, undefined)).toBeNull();
    expect(buildCarfaxUrl(TEMPLATE, null)).toBeNull();
    expect(buildCarfaxUrl(TEMPLATE, '')).toBeNull();
    expect(buildCarfaxUrl(TEMPLATE, '   ')).toBeNull();
  });

  it('returns null when the template ignores the VIN', () => {
    // A template without the placeholder would send every user to the same
    // generic page; failing closed is the correct reading of "the button opens
    // THIS vehicle's report".
    expect(buildCarfaxUrl('https://www.carfax.com/', 'ABC')).toBeNull();
  });

  it('returns null for any scheme other than https', () => {
    // The template comes from a runtime ConfigMap, so whoever can edit it
    // chooses what an anchor's href points at. A javascript: template would
    // otherwise be stored XSS.
    expect(buildCarfaxUrl('javascript:alert("{vin}")', 'ABC')).toBeNull();
    expect(buildCarfaxUrl('http://www.carfax.com/?vin={vin}', 'ABC')).toBeNull();
    expect(buildCarfaxUrl('data:text/html,{vin}', 'ABC')).toBeNull();
  });

  it('returns null when the result is not a parseable URL', () => {
    expect(buildCarfaxUrl('not a url {vin}', 'ABC')).toBeNull();
  });

  it('exports the placeholder it recognises', () => {
    expect(VIN_PLACEHOLDER).toBe('{vin}');
  });
});
```

- [ ] **Step 2: Run the tests to verify they fail**

```sh
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
npm run -w apps/web test -- src/lib/carfax.test.ts
```
Expected: FAIL — cannot resolve `./carfax`.

- [ ] **Step 3: Write the implementation**

Create `apps/web/src/lib/carfax.ts`:

```ts
/** The only placeholder a Carfax URL template may use. */
export const VIN_PLACEHOLDER = '{vin}';

/**
 * Builds a Carfax report URL from a template, or returns null when no button
 * should be rendered at all.
 *
 * null is returned when:
 *  - the VIN is missing or empty after trimming — there is nothing to look up;
 *  - the template does not contain {vin} — it would send every user to the same
 *    generic page, and failing closed is the correct reading of "the button
 *    opens THIS vehicle's report";
 *  - the constructed URL does not parse, or its scheme is not https:. The
 *    template is runtime configuration, so whoever can edit the ConfigMap
 *    chooses what an anchor's href points at; a javascript: template would
 *    otherwise be stored XSS, and permitting http: would let a config change
 *    silently downgrade a link that carries a VIN.
 *
 * Pure: no React, no config import. The template arrives as an argument so this
 * stays directly unit-testable.
 */
export function buildCarfaxUrl(template: string, vin: string | null | undefined): string | null {
  const trimmed = vin?.trim() ?? '';
  if (!trimmed) return null;
  if (!template.includes(VIN_PLACEHOLDER)) return null;

  // split/join replaces every occurrence without needing a regex, so a template
  // that uses the placeholder twice (path and query) works.
  const url = template.split(VIN_PLACEHOLDER).join(encodeURIComponent(trimmed));

  try {
    if (new URL(url).protocol !== 'https:') return null;
  } catch {
    return null;
  }
  return url;
}
```

- [ ] **Step 4: Run the tests to verify they pass**

```sh
npm run -w apps/web test -- src/lib/carfax.test.ts
```
Expected: PASS (9 cases).

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/lib/carfax.ts apps/web/src/lib/carfax.test.ts
git commit -m "feat(web): add a pure Carfax URL builder that fails closed"
```

---

## Task 10: Test helpers and `VehiclePhotoThumbnail`

`MediaThumbnail` was considered for reuse and rejected on four counts, all of which matter on a list page: it calls `useMediaObject(mediaId)` for its `alt` text (one extra `GET /media/{id}` per card — twelve avoidable requests on a twelve-vehicle list, against NFR-2 and NFR-3, when the card already knows the vehicle's name); its failure state is a red `border-destructive` tile reading "Load failed", where FR-2.4 requires the neutral placeholder because a wall of red tiles is not something a user can act on; its empty state is the text "No image" where FR-3.1 requires the `Car` icon; and every dimension is hardcoded `h-24 w-24`. What survives is the *hook* — the two components share `useMediaContentUrl` and nothing else, which is the correct seam.

All four states occupy an identical `h-20 w-20 shrink-0 rounded-md` box, so cards align across a grid row whatever each one resolves to. The two placeholder labels differ so the failure stays distinguishable to assistive technology even though it is visually identical — the visual sameness is the point of FR-2.4, and the label is what keeps that from erasing information.

**No toast on error, by construction:** the component has no toast call, so N broken thumbnails produce N placeholders and zero notifications.

This is the first component test under `components/`, so the two helpers it needs are created here.

**Files:**
- Create: `apps/web/src/test/renderWithProviders.tsx`
- Create: `apps/web/src/test/objectUrl.ts`
- Modify: `apps/web/src/lib/hooks/api/media.test.ts:126-133` (use the lifted stub)
- Create: `apps/web/src/components/features/vehicles/VehiclePhotoThumbnail.tsx`
- Create: `apps/web/src/components/features/vehicles/VehiclePhotoThumbnail.test.tsx`

**Interfaces:**
- Consumes: `useMediaContentUrl(id, 'thumbnail')` (Task 6).
- Produces:
  - `renderWithProviders(ui, options?) => RenderResult & { queryClient: QueryClient }` from `src/test/renderWithProviders`
  - `stubObjectUrl() => { createObjectURL, revokeObjectURL }` from `src/test/objectUrl`
  - `<VehiclePhotoThumbnail mediaId?: string; vehicleLabel: string; className?: string />`
  Task 11's `VehicleCard` renders it and its test uses both helpers.

- [ ] **Step 1: Create the render helper**

Create `apps/web/src/test/renderWithProviders.tsx`:

```tsx
import type { ReactElement, ReactNode } from 'react';
import { render, type RenderOptions, type RenderResult } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';

/**
 * A QueryClient with retries off, so a test asserting an error state reaches it
 * on the first rejection instead of waiting out the app's retry policy.
 */
export function createTestQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: 0 },
      mutations: { retry: false },
    },
  });
}

interface RenderWithProvidersOptions extends Omit<RenderOptions, 'wrapper'> {
  queryClient?: QueryClient;
  /** Initial history entry — components rendering <Link> need a router. */
  route?: string;
}

/**
 * Renders a component inside the providers every feature component assumes:
 * React Query and a router. Returns the QueryClient so a test can seed or
 * inspect the cache.
 */
export function renderWithProviders(
  ui: ReactElement,
  options: RenderWithProvidersOptions = {},
): RenderResult & { queryClient: QueryClient } {
  const { queryClient = createTestQueryClient(), route = '/', ...renderOptions } = options;

  function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>
        <MemoryRouter initialEntries={[route]}>{children}</MemoryRouter>
      </QueryClientProvider>
    );
  }

  return { ...render(ui, { wrapper: Wrapper, ...renderOptions }), queryClient };
}
```

- [ ] **Step 2: Lift the object-URL stub**

Create `apps/web/src/test/objectUrl.ts`:

```ts
import { vi } from 'vitest';

/**
 * jsdom implements neither createObjectURL nor revokeObjectURL, so anything
 * going through useMediaContentUrl needs them stubbed. Returns the mocks so a
 * test can assert on call counts and arguments (the hook's create/revoke pairing
 * is what keeps it from leaking allocations under StrictMode).
 */
export function stubObjectUrl(): {
  createObjectURL: ReturnType<typeof vi.fn>;
  revokeObjectURL: ReturnType<typeof vi.fn>;
} {
  let counter = 0;
  const createObjectURL = vi.fn(() => `blob:mock-${counter++}`);
  const revokeObjectURL = vi.fn();
  vi.stubGlobal('URL', Object.assign(URL, { createObjectURL, revokeObjectURL }));
  return { createObjectURL, revokeObjectURL };
}
```

Then in `apps/web/src/lib/hooks/api/media.test.ts`, delete the local `stubObjectUrl` function (`:126-133`, including its two-line comment) and import the shared one instead — add to the imports at the top:

```ts
import { stubObjectUrl } from '../../../test/objectUrl';
```

- [ ] **Step 3: Verify the lift did not break the existing hook tests**

```sh
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
npm run -w apps/web test -- src/lib/hooks/api/media.test.ts
```
Expected: PASS, unchanged.

- [ ] **Step 4: Write the failing component tests**

Create `apps/web/src/components/features/vehicles/VehiclePhotoThumbnail.test.tsx`:

```tsx
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { screen, waitFor } from '@testing-library/react';
import { ApiError } from '@myfleet/shared-ts';
import { renderWithProviders } from '../../../test/renderWithProviders';
import { stubObjectUrl } from '../../../test/objectUrl';
import { mediaService } from '../../../services/api/MediaService';
import { VehiclePhotoThumbnail } from './VehiclePhotoThumbnail';

vi.mock('../../../services/api/MediaService', () => ({
  mediaService: { getContentBlob: vi.fn() },
}));

beforeEach(() => {
  stubObjectUrl();
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.clearAllMocks();
});

describe('VehiclePhotoThumbnail', () => {
  it('renders the photo once the bytes arrive, labelled with the vehicle', async () => {
    vi.mocked(mediaService.getContentBlob).mockResolvedValue(new Blob(['x']));

    renderWithProviders(<VehiclePhotoThumbnail mediaId="m1" vehicleLabel="2019 Honda Civic" />);

    const img = await screen.findByAltText('Photo of 2019 Honda Civic');
    expect(img).toHaveAttribute('src', expect.stringContaining('blob:'));
  });

  it('requests the thumbnail variant, not the original', async () => {
    // The whole point of the backend half of this task: a card must cost
    // kilobytes, not the full-size upload.
    vi.mocked(mediaService.getContentBlob).mockResolvedValue(new Blob(['x']));

    renderWithProviders(<VehiclePhotoThumbnail mediaId="m1" vehicleLabel="2019 Honda Civic" />);

    await waitFor(() => {
      expect(mediaService.getContentBlob).toHaveBeenCalledWith('m1', 'thumbnail');
    });
  });

  it('shows the "No photo" placeholder when there is no media id, and fetches nothing', () => {
    renderWithProviders(<VehiclePhotoThumbnail vehicleLabel="2019 Honda Civic" />);

    expect(screen.getByRole('img', { name: 'No photo' })).toBeInTheDocument();
    expect(mediaService.getContentBlob).not.toHaveBeenCalled();
  });

  it('falls back to a placeholder on a failed load, with a distinct label', async () => {
    // Visually identical to the no-photo case on purpose — a broken thumbnail
    // on a list card is not actionable — but the label keeps that from erasing
    // the information for assistive technology.
    vi.mocked(mediaService.getContentBlob).mockRejectedValue(
      new ApiError(404, 'not_found', 'gone'),
    );

    renderWithProviders(<VehiclePhotoThumbnail mediaId="m1" vehicleLabel="2019 Honda Civic" />);

    expect(await screen.findByRole('img', { name: 'Photo unavailable' })).toBeInTheDocument();
    expect(screen.queryByAltText('Photo of 2019 Honda Civic')).not.toBeInTheDocument();
  });

  it('holds a skeleton while loading rather than flashing the placeholder', () => {
    vi.mocked(mediaService.getContentBlob).mockReturnValue(new Promise(() => {}));

    const { container } = renderWithProviders(
      <VehiclePhotoThumbnail mediaId="m1" vehicleLabel="2019 Honda Civic" />,
    );

    expect(screen.queryByRole('img')).not.toBeInTheDocument();
    // The skeleton occupies exactly the thumbnail's box so the card does not
    // reflow when the image arrives.
    expect(container.querySelector('.h-20.w-20')).toBeInTheDocument();
  });
});
```

- [ ] **Step 5: Run the tests to verify they fail**

```sh
npm run -w apps/web test -- src/components/features/vehicles/VehiclePhotoThumbnail.test.tsx
```
Expected: FAIL — cannot resolve `./VehiclePhotoThumbnail`.

- [ ] **Step 6: Write the component**

Create `apps/web/src/components/features/vehicles/VehiclePhotoThumbnail.tsx`:

```tsx
import { Car } from 'lucide-react';
import { Skeleton } from '../../ui/skeleton';
import { cn } from '../../../lib/utils';
import { useMediaContentUrl } from '../../../lib/hooks/api/media';

/**
 * Every state occupies exactly this box, so cards with and without photos align
 * within a grid row and the card does not reflow when an image arrives.
 * `shrink-0` is what stops a long title from squeezing it.
 */
const BOX = 'h-20 w-20 shrink-0 rounded-md';

interface VehiclePhotoThumbnailProps {
  mediaId?: string;
  /** Used for the image's alt text — the card already knows it, so no metadata request is needed. */
  vehicleLabel: string;
  className?: string;
}

/**
 * The neutral fallback for both "no photo uploaded" and "the photo failed to
 * load". The two cases look identical on purpose — a wall of red error tiles on
 * a list page is not something a user can act on — so the accessible label is
 * what keeps them distinguishable.
 */
function PhotoPlaceholder({ label, className }: { label: string; className?: string }) {
  return (
    <div
      role="img"
      aria-label={label}
      className={cn(
        BOX,
        'flex items-center justify-center bg-muted text-muted-foreground',
        className,
      )}
    >
      <Car className="h-8 w-8" aria-hidden="true" />
    </div>
  );
}

/**
 * A vehicle's primary photo, sized for a list card.
 *
 * Bytes come through the authenticated API as an object URL — a bare <img src>
 * cannot be used because the API requires an Authorization header that the
 * browser will not send for image subresource requests. The thumbnail variant
 * is requested, so a card costs tens of kilobytes rather than the full-size
 * upload.
 *
 * Deliberately not MediaThumbnail: that component issues a second metadata
 * request per tile for its alt text (N avoidable requests on a list of N
 * vehicles), renders a red "Load failed" tile on error, and hardcodes a
 * different size. The shared piece is the hook, which is the correct seam.
 *
 * No toast on failure, by construction: N broken thumbnails produce N
 * placeholders and zero notifications.
 */
export function VehiclePhotoThumbnail({
  mediaId,
  vehicleLabel,
  className,
}: VehiclePhotoThumbnailProps) {
  const { url, isLoading, isError } = useMediaContentUrl(mediaId, 'thumbnail');

  if (!mediaId) {
    return <PhotoPlaceholder label="No photo" className={className} />;
  }
  if (isLoading) {
    return <Skeleton className={cn(BOX, className)} />;
  }
  if (isError || !url) {
    return (
      <PhotoPlaceholder label={isError ? 'Photo unavailable' : 'No photo'} className={className} />
    );
  }
  return (
    <img
      src={url}
      alt={`Photo of ${vehicleLabel}`}
      className={cn(BOX, 'object-cover', className)}
    />
  );
}
```

- [ ] **Step 7: Run the tests to verify they pass**

```sh
npm run -w apps/web test -- src/components/features/vehicles/VehiclePhotoThumbnail.test.tsx
```
Expected: PASS (5 cases).

- [ ] **Step 8: Commit**

```bash
git add apps/web/src/test/renderWithProviders.tsx apps/web/src/test/objectUrl.ts apps/web/src/lib/hooks/api/media.test.ts apps/web/src/components/features/vehicles/VehiclePhotoThumbnail.tsx apps/web/src/components/features/vehicles/VehiclePhotoThumbnail.test.tsx
git commit -m "feat(web): add VehiclePhotoThumbnail and the shared component-test helpers"
```

---

## Task 11: Rebuild `VehicleCard`

The wrapping `<Link>` is deleted. The card body, the thumbnail, and the text stop being clickable; navigation moves to an explicit icon button.

`min-w-0` on **both** flex children is what makes `truncate` work at all inside a flex row — without it the text sets a minimum content width and the card overflows horizontally at the single-column breakpoint. This is the one layout detail most likely to be dropped, and it is exactly what FR-1.6 tests.

The action row sits at the bottom rather than beside the status badge: it keeps actions visually separate from the badge, gives both buttons a stable position regardless of whether mileage renders, and avoids crowding three elements into the top-right corner.

Both buttons are real anchors via `Button asChild`, which renders through a Radix `Slot` — so the element *is* the `<a>`, and middle-click, cmd/ctrl-click, and the link context menu all work. Both are shown for every role including `viewer`: nothing here is gated on write permission and `requireWrite` is not consulted. The Carfax link is a plain `<a>`, not a react-router `<Link>`, because it leaves the SPA; `rel="noopener noreferrer"` blocks `window.opener` access and suppresses the referrer. There is no `onMouseEnter` handler, no prefetch, and no `<link rel="prefetch">` — nothing contacts Carfax until a click, so no VIN leaves the browser before then.

Both `aria-label`s name the vehicle. A grid of buttons all labelled "Open details" is the regression FR-4.4 and NFR-8 exist to prevent.

**Accepted regression (NFR-10):** the whole-card click target is gone. Two 40×40 targets replace one card-sized one. The trade is eliminating a link nested inside a link, which made the Carfax button's click behaviour ambiguous and is invalid HTML besides.

**Files:**
- Modify: `apps/web/src/components/features/vehicles/VehicleCard.tsx` (full rewrite)
- Create: `apps/web/src/components/features/vehicles/VehicleCard.test.tsx`
- Modify: `apps/web/src/components/features/vehicles/VehicleList.tsx:15`

**Interfaces:**
- Consumes: `VehiclePhotoThumbnail` (Task 10), `buildCarfaxUrl` (Task 9), `getRuntimeConfig` (Task 7), `renderWithProviders` / `stubObjectUrl` (Task 10).
- Produces: nothing downstream.

- [ ] **Step 1: Write the failing tests**

Create `apps/web/src/components/features/vehicles/VehicleCard.test.tsx`:

```tsx
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { screen } from '@testing-library/react';
import { renderWithProviders } from '../../../test/renderWithProviders';
import { stubObjectUrl } from '../../../test/objectUrl';
import { mediaService } from '../../../services/api/MediaService';
import { getRuntimeConfig } from '../../../lib/config/runtimeConfig';
import { VehicleCard } from './VehicleCard';
import type { Vehicle } from '../../../types/models/vehicle';

vi.mock('../../../services/api/MediaService', () => ({
  mediaService: { getContentBlob: vi.fn() },
}));

vi.mock('../../../lib/config/runtimeConfig', () => ({
  getRuntimeConfig: vi.fn(),
}));

const TEMPLATE = 'https://www.carfax.com/VehicleHistory/p/Report.cfx?vin={vin}';

function makeVehicle(attrs: Partial<Vehicle['attributes']> = {}): Vehicle {
  return {
    type: 'vehicles',
    id: 'v1',
    attributes: {
      fleetId: 'f1',
      make: 'Honda',
      model: 'Civic',
      year: 2019,
      ...attrs,
    },
  };
}

beforeEach(() => {
  stubObjectUrl();
  vi.mocked(getRuntimeConfig).mockReturnValue({ carfaxUrlTemplate: TEMPLATE });
  vi.mocked(mediaService.getContentBlob).mockResolvedValue(new Blob(['x']));
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.clearAllMocks();
});

describe('VehicleCard', () => {
  it('renders the vehicle photo when one is set', async () => {
    renderWithProviders(<VehicleCard vehicle={makeVehicle({ primaryImageMediaId: 'm1' })} />);
    expect(await screen.findByAltText('Photo of 2019 Honda Civic')).toBeInTheDocument();
  });

  it('renders the placeholder when no photo is set', () => {
    renderWithProviders(<VehicleCard vehicle={makeVehicle()} />);
    expect(screen.getByRole('img', { name: 'No photo' })).toBeInTheDocument();
    expect(mediaService.getContentBlob).not.toHaveBeenCalled();
  });

  it('navigates to the detail page through a real anchor named for the vehicle', () => {
    // A real <a href> is what preserves middle-click and cmd/ctrl-click; an
    // onClick handler on a <button> would not. And the label must name the
    // vehicle — a grid of "Open details" buttons is unusable with a screen
    // reader.
    renderWithProviders(<VehicleCard vehicle={makeVehicle()} />);

    const link = screen.getByRole('link', { name: 'Open details for 2019 Honda Civic' });
    expect(link).toHaveAttribute('href', '/vehicles/v1');
  });

  it('uses the nickname in the labels when the vehicle has one', () => {
    renderWithProviders(<VehicleCard vehicle={makeVehicle({ nickname: 'Daily Driver' })} />);
    expect(screen.getByRole('link', { name: 'Open details for Daily Driver' })).toBeInTheDocument();
  });

  it('does not make the card body or the thumbnail clickable', () => {
    renderWithProviders(<VehicleCard vehicle={makeVehicle()} />);

    // Exactly one link: the detail button. No wrapping card link, and no VIN so
    // no Carfax link either.
    expect(screen.getAllByRole('link')).toHaveLength(1);
    expect(screen.getByRole('img', { name: 'No photo' }).closest('a')).toBeNull();
  });

  it('renders a Carfax link with the VIN substituted when a VIN is present', () => {
    renderWithProviders(<VehicleCard vehicle={makeVehicle({ vin: '1HGCM82633A004352' })} />);

    const carfax = screen.getByRole('link', {
      name: 'View Carfax report for 2019 Honda Civic (opens in a new tab)',
    });
    expect(carfax).toHaveAttribute(
      'href',
      'https://www.carfax.com/VehicleHistory/p/Report.cfx?vin=1HGCM82633A004352',
    );
    expect(carfax).toHaveAttribute('target', '_blank');
    // noopener blocks window.opener access from the opened page; noreferrer
    // stops MyFleet being sent as the referrer alongside the VIN.
    expect(carfax).toHaveAttribute('rel', 'noopener noreferrer');
  });

  it('omits the Carfax button entirely when there is no VIN', () => {
    // Omitted, not disabled: a disabled button would occupy space and attract
    // focus for no reason.
    renderWithProviders(<VehicleCard vehicle={makeVehicle({ vin: '   ' })} />);
    expect(screen.queryByRole('link', { name: /Carfax/ })).not.toBeInTheDocument();
    expect(screen.getAllByRole('link')).toHaveLength(1);
  });

  it('omits the Carfax button when the configured template ignores the VIN', () => {
    vi.mocked(getRuntimeConfig).mockReturnValue({ carfaxUrlTemplate: 'https://www.carfax.com/' });

    renderWithProviders(<VehicleCard vehicle={makeVehicle({ vin: '1HGCM82633A004352' })} />);
    expect(screen.queryByRole('link', { name: /Carfax/ })).not.toBeInTheDocument();
  });

  it('keeps the detail button first and in the same place with or without a VIN', () => {
    const { unmount } = renderWithProviders(<VehicleCard vehicle={makeVehicle()} />);
    expect(screen.getAllByRole('link')[0]).toHaveAttribute('href', '/vehicles/v1');
    unmount();

    renderWithProviders(<VehicleCard vehicle={makeVehicle({ vin: 'ABC' })} />);
    const links = screen.getAllByRole('link');
    expect(links).toHaveLength(2);
    expect(links[0]).toHaveAttribute('href', '/vehicles/v1');
    expect(links[1]).toHaveAttribute('href', expect.stringContaining('carfax.com'));
  });

  it('still shows the title, subtitle, status, and mileage', () => {
    renderWithProviders(
      <VehicleCard
        vehicle={makeVehicle({ trim: 'Sport', currentMileage: 42000, status: 'Healthy' })}
      />,
    );

    expect(screen.getByText('2019 Honda Civic Sport')).toBeInTheDocument();
    expect(screen.getByText('Healthy')).toBeInTheDocument();
    expect(screen.getByText(/42,000/)).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run the tests to verify they fail**

```sh
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
npm run -w apps/web test -- src/components/features/vehicles/VehicleCard.test.tsx
```
Expected: FAIL — the current card wraps everything in a `<Link>`, so there is no "Open details for …" link and `getAllByRole('link')` finds the card wrapper.

- [ ] **Step 3: Rewrite the card**

Replace `apps/web/src/components/features/vehicles/VehicleCard.tsx` with:

```tsx
import { Link } from 'react-router-dom';
import { ChevronRight, History } from 'lucide-react';
import { StatusBadge, formatMileage, type VehicleStatus } from '@myfleet/ui-components';
import { Button } from '../../ui/button';
import { Card } from '../../ui/card';
import { VehiclePhotoThumbnail } from './VehiclePhotoThumbnail';
import { buildCarfaxUrl } from '../../../lib/carfax';
import { getRuntimeConfig } from '../../../lib/config/runtimeConfig';
import type { Vehicle } from '../../../types/models/vehicle';

const KNOWN_STATUSES: readonly VehicleStatus[] = [
  'Healthy',
  'Upcoming Maintenance',
  'Overdue',
  'Inactive',
];

function asVehicleStatus(value: string | undefined): VehicleStatus | null {
  return value && (KNOWN_STATUSES as readonly string[]).includes(value)
    ? (value as VehicleStatus)
    : null;
}

export function VehicleCard({ vehicle }: { vehicle: Vehicle }) {
  const { attributes } = vehicle;
  const title =
    attributes.nickname?.trim() ||
    `${attributes.year} ${attributes.make} ${attributes.model}`.trim();
  const status = asVehicleStatus(attributes.status);
  // null means "render no button": no VIN, a template that ignores {vin}, or a
  // template whose scheme is not https:. Nothing contacts Carfax until a click.
  const carfaxUrl = buildCarfaxUrl(getRuntimeConfig().carfaxUrlTemplate, attributes.vin);

  return (
    <Card className="p-4">
      <div className="flex items-start gap-4">
        <VehiclePhotoThumbnail mediaId={attributes.primaryImageMediaId} vehicleLabel={title} />
        {/* min-w-0 on BOTH flex children is what lets `truncate` work inside a
            flex row; without it the text sets a minimum content width and the
            card overflows horizontally at the single-column breakpoint. */}
        <div className="min-w-0 flex-1">
          <div className="flex items-start justify-between gap-2">
            <div className="min-w-0">
              <div className="truncate font-medium text-foreground">{title}</div>
              <div className="truncate text-sm text-muted-foreground">
                {attributes.year} {attributes.make} {attributes.model}
                {attributes.trim ? ` ${attributes.trim}` : ''}
              </div>
            </div>
            {status && <StatusBadge status={status} />}
          </div>
          {typeof attributes.currentMileage === 'number' && (
            <div className="mt-2 text-sm text-muted-foreground">
              {formatMileage(attributes.currentMileage)}
            </div>
          )}
        </div>
      </div>

      {/* Actions live in their own row so they stay clear of the status badge
          and hold the same position whether or not mileage renders. Detail
          first, Carfax second — always. */}
      <div className="mt-3 flex items-center justify-end gap-1">
        {/* asChild renders through a Radix Slot, so the element IS the router's
            <a>: middle-click, cmd/ctrl-click, and the link context menu all
            work, which an onClick handler would not preserve. Shown for every
            role including viewer — nothing here is gated on write permission. */}
        <Button asChild variant="ghost" size="icon">
          <Link to={`/vehicles/${vehicle.id}`} aria-label={`Open details for ${title}`}>
            <ChevronRight className="h-4 w-4" aria-hidden="true" />
          </Link>
        </Button>
        {carfaxUrl && (
          // A plain <a>, not a react-router <Link> — this leaves the SPA.
          // rel="noopener noreferrer" stops the opened page reaching back
          // through window.opener and suppresses the referrer, which matters
          // because the URL carries the VIN.
          <Button asChild variant="ghost" size="icon">
            <a
              href={carfaxUrl}
              target="_blank"
              rel="noopener noreferrer"
              aria-label={`View Carfax report for ${title} (opens in a new tab)`}
            >
              <History className="h-4 w-4" aria-hidden="true" />
            </a>
          </Button>
        )}
      </div>
    </Card>
  );
}
```

- [ ] **Step 4: Run the tests to verify they pass**

```sh
npm run -w apps/web test -- src/components/features/vehicles/VehicleCard.test.tsx
```
Expected: PASS (10 cases).

- [ ] **Step 5: Match the loading skeleton to the taller card**

The card is now roughly `p-4` (32) + 80 thumbnail + `mt-3` (12) + 40 action row ≈ 164px. In `apps/web/src/components/features/vehicles/VehicleList.tsx`, change line 15 to:

```tsx
          <Skeleton key={i} className="h-44 w-full" />
```

`h-44` is 176px. Its only job is to stop the skeleton-to-content transition from jumping, so eyeball it against the rendered card in Step 6 and nudge it if the jump is visible.

- [ ] **Step 6: Look at the running UI**

```sh
npm run -w apps/web dev
```

Check, at a browser width narrow enough to force `grid-cols-1` and again at `lg` (three columns):
- no horizontal overflow at the single-column breakpoint (FR-1.6);
- cards with and without photos align within a row;
- no visible jump between skeleton and content — nudge the `h-44` if there is;
- the card body and thumbnail do nothing when clicked;
- both buttons are reachable by Tab and show the focus ring;
- whether three columns still read well at this card height (PRD §9.4 flagged this as a visual judgement — the grid class is a one-token change if it does not).

Report what you saw. If the column count needs changing, say so rather than changing it silently.

- [ ] **Step 7: Lint, test, and build the whole web app**

```sh
npm run -w apps/web lint && npm run -w apps/web test && npm run -w apps/web build
```
Expected: all PASS with zero warnings.

- [ ] **Step 8: Commit**

```bash
git add apps/web/src/components/features/vehicles/VehicleCard.tsx apps/web/src/components/features/vehicles/VehicleCard.test.tsx apps/web/src/components/features/vehicles/VehicleList.tsx
git commit -m "feat(web): rebuild the vehicle card around a photo thumbnail and explicit actions"
```

---

## Task 12: Whole-branch verification

Everything above ran its own gate; this proves the branch as a unit and produces the evidence the code-review step expects.

**Files:** none modified (unless a failure surfaces, in which case fix it in the file that owns it and note it in the report).

**Interfaces:** none.

- [ ] **Step 1: Run the full CI gate**

```sh
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
make ci
```
Expected: PASS — `lint-check`, `vet`, `test`, `build`, `fe-test`, `fe-build`, `manifests` all green. Paste the actual tail of the output into the task report; do not assert success from memory.

- [ ] **Step 2: Both server dry-runs**

```sh
kustomize build deploy/k8s/overlays/main  | kubectl apply --dry-run=server -f -
kustomize build deploy/k8s/overlays/local | kubectl apply --dry-run=server -f -
```
Expected: every object reports `(server dry run)` with no error.

**The local overlay is not exempt.** A missing `namespace:` in `deploy/k8s/infra-local/kustomization.yaml` once made `kubectl apply -k deploy/k8s/overlays/local` fail outright and slipped through ten reviews because only the `main` dry-run was ever run. If no cluster is reachable, report that explicitly rather than marking the step done.

- [ ] **Step 3: Walk the acceptance criteria**

Open `docs/tasks/task-005-vehicle-card-photo-actions/prd.md` §10 and check each box, **with one correction carried from design D1**: the cross-fleet criterion reads `403` in the PRD and is wrong — the implemented and correct behaviour is `404`, so that cross-fleet existence is never leaked. Note the correction in the report rather than silently ticking the box.

For the two criteria that need a running system rather than a test — `?variant=thumbnail` visible in the network panel, and no request to Carfax before a click — verify them in the browser during Step 4.

- [ ] **Step 4: End-to-end check against the running stack**

```sh
make up
npm run -w apps/web dev
```

With a vehicle that has a primary photo and a VIN, confirm in the browser's network panel:
- the card's image request is `GET /api/media/{id}/content?variant=thumbnail` and returns `200` with `Cache-Control: private, max-age=300` and no `Content-Length`;
- a request with no `variant` parameter still returns the original with its `Content-Length`;
- `GET /api/media/{id}/content?variant=bogus` returns `400` with a JSON:API body carrying `"code": "bad_request"`;
- `GET /config/config.json` returns `200` with `Cache-Control: no-cache`;
- **no** request to any `carfax.com` host is made until the Carfax button is clicked.

Then `make down`.

- [ ] **Step 5: Run the code review**

Per CLAUDE.md, the review step is mandatory before a PR and must not be skipped even when the plan looks complete. Both a Go service and frontend files changed, so all three reviewers apply:

Invoke `superpowers:requesting-code-review`, which dispatches `plan-adherence-reviewer`, `backend-guidelines-reviewer`, and `frontend-guidelines-reviewer` in parallel. Findings land in `docs/tasks/task-005-vehicle-card-photo-actions/audit.md`.

- [ ] **Step 6: Commit any review fixes and the audit**

```bash
git add -A
git commit -m "chore(task-005): address code-review findings"
```

(Skip the commit if the review produced no changes.)

---

## Self-Review

**Spec coverage** — every design section maps to a task:

| Design section | Task |
| --- | --- |
| D1 (cross-fleet 404) | Global Constraints; Tasks 4 (`TestContent_crossFleetNeverTouchesLookupOrStore`), 5, 12 Step 3 |
| D2 (`ErrBadRequest`, `codeFor` 400/413) | Task 1 |
| D3 (runtime config, no `VITE_*`) | Tasks 7, 8 |
| D4 (all three variants accepted) | Tasks 3, 5 |
| D5 (no VIN validation) | Task 9 — the builder requires only a non-empty trimmed VIN |
| §3.1 (`VariantLookup` port, adapter in the composition root) | Tasks 4, 5 |
| §3.2 (`GetByMediaObjectAndVariant`) | Task 2 |
| §3.3 (`ParseContentVariant`) | Task 3 |
| §3.4 (`Content` takes the variant; `ContentInfo`; store-drift fallback) | Task 4 |
| §3.5 (handler, headers, no `Vary`) | Task 5 |
| §3.6 (backend tests, NFR-13) | Tasks 2, 3, 4, 5 |
| §4 (type, service, key, hook; `MediaThumbnail` untouched) | Task 6 |
| §5.1-5.4 (delivery, shape, module, bootstrap & failures) | Tasks 7, 8 |
| §5.5 / §6.1 (injection surface, `https:`-only builder) | Task 9 |
| §6.2 (`VehiclePhotoThumbnail`, four states) | Task 10 |
| §6.3 (card layout, both buttons) | Task 11 |
| §6.4 (skeleton height) | Task 11 Step 5 |
| §7 (deployment) | Task 8 |
| §8 (frontend tests + the two test helpers) | Tasks 7, 9, 10, 11 |
| §9 (risks) | Task 7 (timeout/defaults), Task 9 (scheme), Task 8 (directory mount), Task 7 Step 6 (`no-cache`), Task 6 (key collision), Task 11 Step 6 (grid density), Task 5 Step 7 (caller sweep) |

**Corrections carried into this plan.** Design §3.2 says `ListByMediaObject` is kept because "the worker's replace path uses it"; the worker actually uses `Administrator.ReplaceForMediaObject`, and `ListByMediaObject` has no production caller. The conclusion (keep it) is unchanged — removing it would leave `Provider` empty — but Task 2 states the real reason rather than repeating the wrong one.

**Gaps the design left open, decided in Task 4** and flagged there in prose: a `Lookup` **error** propagates (→ 500) rather than falling back, because `GetByID` just read the same database successfully, so an error means the database is genuinely broken and masking it would hide a real fault. A missing variant **object** still falls back, per design §3.4.

**Type consistency** — checked across tasks: `ContentVariant`/`ContentOriginal`/`ContentThumbnail`/`ContentDisplay` (Task 3) are used unchanged in Tasks 4 and 5. `VariantRef{ObjectKey, ContentType}` and `VariantLookup.Lookup(mediaObjectID, variant string) (VariantRef, bool, error)` (Task 4) are implemented verbatim by `variantLookup` in Task 5. `ContentInfo{ContentType, Size, Served}` (Task 4) is read field-for-field by the handler in Task 5. `GetByMediaObjectAndVariant(mediaObjectID string, v Variant) (Model, bool, error)` (Task 2) is called with `mediavariant.Variant(variant)` in Task 5. `MediaVariant` (Task 6) is the parameter type of `getContentBlob`, `mediaKeys.content`, and `useMediaContentUrl`, and `'thumbnail'` is the literal passed in Task 10. `buildCarfaxUrl(template, vin)` (Task 9) is called with `getRuntimeConfig().carfaxUrlTemplate` (Task 7) in Task 11. `renderWithProviders` and `stubObjectUrl` (Task 10) are imported at the same paths in Task 11.
