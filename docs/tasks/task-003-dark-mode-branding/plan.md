# Dark Mode & Application Branding — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn the dormant `.dark` palette into a real, per-user, server-persisted theme system, convert every hardcoded palette colour to semantic tokens, and give the app a generated brand mark used as favicon, installed-app icon, and in-app header mark.

**Architecture:** `auth-service` gains a `theme_preference` column, a `PATCH /auth/me` route, and the value on `GET /auth/me`. The SPA keeps client theme state in a network-unaware `ThemeContext`, bridges the server value in through a purpose-built `ThemeSync` component, and paints the correct theme before first frame via a synchronous inline script in `index.html` backed by a `localStorage` cache. Icons are emitted by one Python generator whose geometry table — not the SVG — is the single source; every output is committed.

**Tech Stack:** Go 1.x + chi + GORM (auth-service); React 18 + TypeScript + Vite + Tailwind + TanStack Query + sonner + lucide-react (apps/web); Python 3 + Pillow 12.2 (icon generation, dev-time only).

## Global Constraints

- **No new runtime dependencies.** `lucide-react`, `sonner`, and the existing context patterns cover everything (FR-PERF-3). No new Go modules, environment variables, config, Kubernetes manifests, or gateway routes.
- **Storage key is exactly `myfleet.theme`.** It appears in three places — `src/lib/theme.ts`, the inline script in `index.html`, and tests — and must match verbatim.
- **The three preference values are exactly `light`, `dark`, `system`.** Any other value is invalid, on both sides of the wire.
- **Validation failures on `PATCH /auth/me` return 422, not the PRD's 400** (design §3.1). `shared-go` has no 400 sentinel, and `RegisterInputHandler` already emits 422 for a malformed body; shipping 400 for a bad enum next to 422 for bad JSON would be incoherent. `shared-go` is not modified. Wherever the PRD says 400, read 422.
- **CSS token format is `H S% L%` with no `hsl()` wrapper**, matching `apps/web/src/index.css`.
- **`--destructive` is never touched** (FR-TOKEN-4). `danger` is status indication; `destructive` styles destructive controls. They coexist.
- **`themePreference` never enters the JWT** (FR-DATA-3).
- **Error responses never carry database internals.** `server.WriteError` copies `err.Error()` into the response title; the persistence-failure branch renders the package-level `errInternal` sentinel only (FR-SEC-3).
- **Icon assets stay under 100 KB in aggregate** (FR-PERF-4). The generator prints the total; current output is 36.4 KiB.
- **Committed icon outputs are never hand-edited.** `tools/generate-icons.py` regenerates all of them.
- **`make ci` is the gate:** `lint-check`, `vet`, `test`, `build`, `fe-test`, `fe-build`, `manifests`. No manifest change is expected, but `manifests` runs regardless.
- **Node is not always on `PATH`.** If `npm` is missing: `export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22`.
- **Prettier covers** `apps/web/src/**/*.{ts,tsx,css}` and `apps/web/*.{ts,js}`. It does **not** cover `index.html`, `public/site.webmanifest`, or `tools/*.py`. Generated `.ts` under `src/` **is** covered — run `npm run format` after regenerating.
- All work happens in `/home/tumidanski/source/MyFleet/.worktrees/task-003-dark-mode-branding` on branch `task-003-dark-mode-branding`.

---

## File Structure

**`apps/auth-service/internal/user/`**

| File | Responsibility |
|---|---|
| `theme.go` *(new)* | The three constants and `IsValidTheme`. Single source of the allow-list. |
| `model.go` | `themePreference` field, accessor, `WithThemePreference`, `ErrInvalidTheme`. |
| `entity.go` | Column, `Make` normalisation, `ToEntity` mapping. |
| `builder.go` | `NewBuilder()` seeds `ThemeSystem`; `SetThemePreference`. |
| `processor.go` | `UpdateTheme(userID, pref)` — validate → load → mutate → persist. |
| `rest.go` | `themePreference` in `Attributes` and `Transform`. |
| `resource.go` | `PATCH /auth/me` + `errThemeValidation` sentinel. |
| `administrator.go` | **Unchanged** (design §3.2) — `Update` already does a full `db.Save`. |

**`apps/web/src/`**

| File | Responsibility |
|---|---|
| `lib/theme.ts` *(new)* | Pure helpers + storage key. No React, no network; one `classList` call. |
| `context/ThemeContext.tsx` *(new)* | Client theme state, `matchMedia` subscription, the `dark` class. No network, no auth. |
| `lib/hooks/api/auth.ts` | `useUpdateTheme()` — the PATCH and its cache write, nothing else. |
| `components/ThemeToggle.tsx` *(new)* | Header cycle button; owns the failure toast. |
| `components/ThemeSync.tsx` *(new)* | One-way bridge, server preference → theme state. Renders `null`. |
| `components/BrandMark.tsx` *(new)* | Inline SVG mark via `currentColor`. |
| `components/brandMarkPath.ts` *(new, generated)* | The `d` string. Do not hand-edit. |
| `components/providers/AppProviders.tsx` | Provider tree + `ThemedToaster`. |
| `test/setup.ts` | Adds the `matchMedia` stub beside the existing `MemoryStorage` polyfill. |
| `test/conventions.test.ts` *(new)* | Three guard tests: pre-paint script, no hardcoded palette classes, path/SVG agreement. |
| `types/models/user.ts` | `ThemePreference` type + `themePreference` attribute. |
| `index.css`, `../tailwind.config.ts`, `../index.html` | Tokens, Tailwind registration, `<head>` wiring. |

**`apps/web/public/`** *(new)* — `favicon.svg`, `favicon.ico`, `apple-touch-icon.png`, `icon-192.png`, `icon-512.png`, `icon-512-maskable.png`, `site.webmanifest`.

**`tools/`** — `generate-icons.py` *(new)*, `generate-icons.sh` *(new)*. Not in the build graph.

**`docs/tasks/task-003-dark-mode-branding/`** — `contrast.md` *(new)*, the FR-A11Y-1 record.

---

## Task 1: Theme allow-list

**Files:**
- Create: `apps/auth-service/internal/user/theme.go`
- Test: `apps/auth-service/internal/user/theme_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `user.ThemeLight`, `user.ThemeDark`, `user.ThemeSystem` (untyped string constants); `user.IsValidTheme(string) bool`.

Plain `string` constants, not a named type: a named type would have to thread through `Attributes`, `Entity`, and every fixture for no benefit the allow-list does not already give, and no other enum in this codebase is a named type (design §7.1).

- [ ] **Step 1: Write the failing test**

`apps/auth-service/internal/user/theme_test.go`:

```go
package user

import "testing"

func TestIsValidTheme(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"light", ThemeLight, true},
		{"dark", ThemeDark, true},
		{"system", ThemeSystem, true},
		{"empty is not a preference", "", false},
		{"unknown value", "purple", false},
		{"case sensitive", "Dark", false},
		{"whitespace is not trimmed for us", " dark", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsValidTheme(tt.input); got != tt.want {
				t.Fatalf("IsValidTheme(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./apps/auth-service/internal/user/ -run TestIsValidTheme -v`
Expected: FAIL — `undefined: ThemeLight`, `undefined: IsValidTheme`.

- [ ] **Step 3: Write minimal implementation**

`apps/auth-service/internal/user/theme.go`:

```go
package user

// The complete set of theme preferences. Plain string constants rather than a
// named type: a named type would have to thread through Attributes, Entity and
// every test fixture for no benefit the allow-list below does not already
// provide, and no other enum in this codebase is modelled as one.
const (
	ThemeLight  = "light"
	ThemeDark   = "dark"
	ThemeSystem = "system"
)

// IsValidTheme is the single source of the allow-list. Everything that accepts
// a preference — the processor, the read-side normalisation in Make — goes
// through here, so adding a fourth theme is a one-line change.
func IsValidTheme(s string) bool {
	return s == ThemeLight || s == ThemeDark || s == ThemeSystem
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./apps/auth-service/internal/user/ -run TestIsValidTheme -v`
Expected: PASS (7 subtests).

- [ ] **Step 5: Commit**

```bash
git add apps/auth-service/internal/user/theme.go apps/auth-service/internal/user/theme_test.go
git commit -m "feat(auth-service): add theme preference allow-list"
```

---

## Task 2: Persist the preference on the user record

**Files:**
- Modify: `apps/auth-service/internal/user/model.go`
- Modify: `apps/auth-service/internal/user/entity.go`
- Modify: `apps/auth-service/internal/user/builder.go`
- Modify: `apps/auth-service/internal/user/provider_test.go` (test DDL — see below)
- Test: `apps/auth-service/internal/user/entity_test.go` (new)

**Interfaces:**
- Consumes: `ThemeSystem`, `IsValidTheme` (Task 1).
- Produces: `Model.ThemePreference() string`; `Model.WithThemePreference(string) Model`; `user.ErrInvalidTheme`; `Entity.ThemePreference string`; `(*Builder).SetThemePreference(string) *Builder`.

**Critical:** `provider_test.go`'s `newTestDB` carries explicit DDL marked *"KEEP IN SYNC WITH entity.go"* (it cannot call `AutoMigrate` because the `uniqueIndex` tags emit unqualified `CREATE UNIQUE INDEX` that can't resolve against the attached `auth` schema). Adding a column to `Entity` without adding it there makes every `resource_test.go` and `provider_test.go` test fail with *"no such column: theme_preference"*.

`WithThemePreference` does **not** validate — validation is the processor's job, so invalid input produces a typed domain error rather than a silently ignored mutation (PRD §6.2).

- [ ] **Step 1: Write the failing test**

`apps/auth-service/internal/user/entity_test.go`:

```go
package user

import "testing"

// FR-DATA-4: a row written before the column existed, or edited out of band,
// must not surface an out-of-range value to clients. Normalising on read means
// GET /auth/me can promise the value is always one of the three.
func TestMake_normalisesUnknownStoredThemes(t *testing.T) {
	tests := []struct {
		name   string
		stored string
		want   string
	}{
		{"empty backfills to system", "", ThemeSystem},
		{"unknown value falls back to system", "purple", ThemeSystem},
		{"light survives", ThemeLight, ThemeLight},
		{"dark survives", ThemeDark, ThemeDark},
		{"system survives", ThemeSystem, ThemeSystem},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := Make(Entity{ID: "u1", ThemePreference: tt.stored})
			if got := m.ThemePreference(); got != tt.want {
				t.Fatalf("Make(Entity{ThemePreference: %q}).ThemePreference() = %q, want %q",
					tt.stored, got, tt.want)
			}
		})
	}
}

// The round trip must not drop the column, or Administrator.Update would write
// an empty string back over a good value on every login.
func TestToEntity_carriesThemePreference(t *testing.T) {
	m := Make(Entity{ID: "u1", ThemePreference: ThemeDark})
	if got := m.ToEntity().ThemePreference; got != ThemeDark {
		t.Fatalf("ToEntity().ThemePreference = %q, want %q", got, ThemeDark)
	}
}

func TestWithThemePreference_returnsACopy(t *testing.T) {
	original := Make(Entity{ID: "u1", ThemePreference: ThemeLight})
	updated := original.WithThemePreference(ThemeDark)

	if original.ThemePreference() != ThemeLight {
		t.Fatalf("WithThemePreference mutated the receiver: %q", original.ThemePreference())
	}
	if updated.ThemePreference() != ThemeDark {
		t.Fatalf("WithThemePreference returned %q, want %q", updated.ThemePreference(), ThemeDark)
	}
}

// Validation belongs to the processor (PRD §6.2), so the domain setter must
// accept whatever it is given. A setter that silently dropped bad input would
// make an invalid PATCH look like a success.
func TestWithThemePreference_doesNotValidate(t *testing.T) {
	if got := Make(Entity{ID: "u1"}).WithThemePreference("purple").ThemePreference(); got != "purple" {
		t.Fatalf("WithThemePreference(%q) = %q; the setter must not validate", "purple", got)
	}
}

// Design §3.4: ProvisionFromGoogle inserts via the builder, so the builder —
// not the Postgres column default — is what puts a real value on a new row.
// Whether GORM omits a zero-valued column carrying a `default:` tag and reads
// it back via RETURNING is version- and dialect-dependent; do not rely on it.
func TestNewBuilder_seedsSystem(t *testing.T) {
	if got := NewBuilder().Build().ThemePreference(); got != ThemeSystem {
		t.Fatalf("NewBuilder().Build().ThemePreference() = %q, want %q", got, ThemeSystem)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./apps/auth-service/internal/user/ -run 'TestMake_normalises|TestToEntity_carries|TestWithThemePreference|TestNewBuilder_seeds' -v`
Expected: FAIL — `unknown field ThemePreference in struct literal`, `m.ThemePreference undefined`.

- [ ] **Step 3: Add the model field, accessor, setter and sentinel**

In `apps/auth-service/internal/user/model.go`, add the sentinel next to `ErrNotFound`, the field to `Model`, and the accessor + setter:

```go
var ErrNotFound = errors.New("user not found")

// ErrInvalidTheme is returned by Processor.UpdateTheme for a value outside the
// allow-list. It is deliberately distinguishable from ErrNotFound so the
// transport layer can render 422 and 404 apart.
var ErrInvalidTheme = errors.New("invalid theme preference")

// Model is immutable; state changes return new instances (design §6).
type Model struct {
	id              string
	googleSub       string
	email           string
	displayName     string
	avatarURL       string
	themePreference string
	lastLoginAt     time.Time
}
```

and below the existing accessors:

```go
func (m Model) ThemePreference() string { return m.themePreference }

// WithThemePreference returns a copy carrying the new preference. It does NOT
// validate: validation is the processor's job (PRD §6.2), so an out-of-range
// value produces a typed domain error at the call site rather than a mutation
// that silently does nothing.
func (m Model) WithThemePreference(pref string) Model {
	m.themePreference = pref
	return m
}
```

- [ ] **Step 4: Add the column, normalisation and mapping**

Replace `apps/auth-service/internal/user/entity.go` in full:

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
	// AutoMigrate issues ADD COLUMN theme_preference ... NOT NULL DEFAULT
	// 'system', which Postgres backfills across existing rows. The default is
	// what migrates old rows; new rows get their value from NewBuilder
	// (design §3.4), so no insert path depends on it.
	ThemePreference string `gorm:"not null;default:'system'"`
	LastLoginAt     time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (Entity) TableName() string { return "auth.users" }

func Migration(db *gorm.DB) error { return db.AutoMigrate(&Entity{}) }

func Make(e Entity) Model {
	// FR-DATA-4: normalise on read. A row written before the column existed, or
	// edited out of band, must never reach a client as an out-of-range value —
	// GET /auth/me promises one of exactly three.
	theme := e.ThemePreference
	if !IsValidTheme(theme) {
		theme = ThemeSystem
	}
	return Model{id: e.ID, googleSub: e.GoogleSub, email: e.Email, displayName: e.DisplayName, avatarURL: e.AvatarURL, themePreference: theme, lastLoginAt: e.LastLoginAt}
}

func (m Model) ToEntity() Entity {
	return Entity{ID: m.id, GoogleSub: m.googleSub, Email: m.email, DisplayName: m.displayName, AvatarURL: m.avatarURL, ThemePreference: m.themePreference, LastLoginAt: m.lastLoginAt}
}
```

- [ ] **Step 5: Seed the builder**

Replace `apps/auth-service/internal/user/builder.go` in full:

```go
package user

import "github.com/google/uuid"

type Builder struct{ m Model }

// NewBuilder seeds themePreference explicitly rather than leaning on the
// Postgres column default. ProvisionFromGoogle inserts through this builder,
// and whether GORM omits a zero-valued column carrying a `default:` tag —
// then reads the value back via RETURNING — is version- and dialect-dependent
// (design §3.4).
func NewBuilder() *Builder {
	return &Builder{m: Model{id: uuid.NewString(), themePreference: ThemeSystem}}
}

func (b *Builder) SetGoogleSub(s string) *Builder        { b.m.googleSub = s; return b }
func (b *Builder) SetEmail(e string) *Builder            { b.m.email = e; return b }
func (b *Builder) SetDisplayName(n string) *Builder      { b.m.displayName = n; return b }
func (b *Builder) SetAvatarURL(a string) *Builder        { b.m.avatarURL = a; return b }
func (b *Builder) SetThemePreference(p string) *Builder  { b.m.themePreference = p; return b }
func (b *Builder) Build() Model                          { return b.m }
```

- [ ] **Step 6: Add the column to the test DDL**

In `apps/auth-service/internal/user/provider_test.go`, inside `newTestDB`'s `CREATE TABLE auth.users` statement, add the column after `avatar_url`:

```go
	if err := db.Exec(`CREATE TABLE auth.users (
		id               text primary key,
		google_sub       text not null unique,
		email            text not null unique,
		display_name     text,
		avatar_url       text,
		theme_preference text not null default 'system',
		last_login_at    datetime,
		created_at       datetime,
		updated_at       datetime
	)`).Error; err != nil {
		t.Fatalf("create table: %v", err)
	}
```

- [ ] **Step 7: Run the package tests**

Run: `go test ./apps/auth-service/internal/user/ -v`
Expected: PASS — the five new tests plus every pre-existing provider/processor/resource test still green.

- [ ] **Step 8: Format and commit**

```bash
./tools/lint.sh --go apps/auth-service
git add apps/auth-service/internal/user/
git commit -m "feat(auth-service): persist theme preference on the user record"
```

---

## Task 3: `Processor.UpdateTheme`

**Files:**
- Modify: `apps/auth-service/internal/user/processor.go`
- Test: `apps/auth-service/internal/user/processor_test.go`

**Interfaces:**
- Consumes: `IsValidTheme`, `ErrInvalidTheme`, `WithThemePreference`, `Provider.GetByID`, `Administrator.Update`.
- Produces: `(*Processor).UpdateTheme(userID, pref string) (Model, error)`.

Validation runs **before** the read (design §7.2): an invalid value then costs no database round trip and cannot leave a partially-applied state. The three outcomes are distinguishable at the call site — `ErrInvalidTheme`, `ErrNotFound`, anything else.

- [ ] **Step 1: Write the failing test**

Append to `apps/auth-service/internal/user/processor_test.go`:

```go
func TestUpdateTheme_persistsEachValidValue(t *testing.T) {
	for _, pref := range []string{ThemeLight, ThemeDark, ThemeSystem} {
		t.Run(pref, func(t *testing.T) {
			existing := Make(Entity{ID: "u1", Email: "a@b.com", ThemePreference: ThemeSystem})
			w := &fakeAdmin{}
			p := NewProcessor(logrus.New(), &fakeProvider{byID: map[string]Model{"u1": existing}}, w)

			got, err := p.UpdateTheme("u1", pref)
			if err != nil {
				t.Fatalf("UpdateTheme(%q) returned %v", pref, err)
			}
			if got.ThemePreference() != pref {
				t.Fatalf("UpdateTheme(%q) returned preference %q", pref, got.ThemePreference())
			}
			if w.updated != 1 {
				t.Fatalf("expected exactly one persist call, got %d", w.updated)
			}
		})
	}
}

// An invalid value must be rejected BEFORE the read, so it can never leave a
// partially-applied state and costs no database round trip (design §7.2).
func TestUpdateTheme_rejectsInvalidWithoutTouchingStorage(t *testing.T) {
	for _, pref := range []string{"", "purple", "Dark"} {
		t.Run(pref, func(t *testing.T) {
			existing := Make(Entity{ID: "u1", ThemePreference: ThemeLight})
			w := &fakeAdmin{}
			p := NewProcessor(logrus.New(), &fakeProvider{byID: map[string]Model{"u1": existing}}, w)

			if _, err := p.UpdateTheme("u1", pref); !errors.Is(err, ErrInvalidTheme) {
				t.Fatalf("UpdateTheme(%q) = %v, want ErrInvalidTheme", pref, err)
			}
			if w.updated != 0 {
				t.Fatalf("UpdateTheme(%q) persisted %d time(s); an invalid value must not reach storage", pref, w.updated)
			}
		})
	}
}

func TestUpdateTheme_unknownUserIsNotFound(t *testing.T) {
	w := &fakeAdmin{}
	p := NewProcessor(logrus.New(), &fakeProvider{byID: map[string]Model{}}, w)

	if _, err := p.UpdateTheme("nobody", ThemeDark); !errors.Is(err, ErrNotFound) {
		t.Fatalf("UpdateTheme for an unknown user = %v, want ErrNotFound", err)
	}
}
```

Add `"errors"` to the import block of `processor_test.go`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./apps/auth-service/internal/user/ -run TestUpdateTheme -v`
Expected: FAIL — `p.UpdateTheme undefined`.

- [ ] **Step 3: Write the implementation**

Append to `apps/auth-service/internal/user/processor.go`:

```go
// UpdateTheme validates, loads, mutates and persists the caller's theme
// preference (PRD §5.2).
//
// Validation runs before the read on purpose: an out-of-range value then costs
// no database round trip and cannot leave a partially-applied state. The three
// error outcomes — ErrInvalidTheme, ErrNotFound, and anything else — are
// distinguishable at the call site, which is what lets the handler render 422,
// 404 and a bare 500 apart.
func (pr *Processor) UpdateTheme(userID string, pref string) (Model, error) {
	if !IsValidTheme(pref) {
		return Model{}, ErrInvalidTheme
	}
	m, err := pr.p.GetByID(userID)
	if err != nil {
		return Model{}, err
	}
	return pr.a.Update(m.WithThemePreference(pref))
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./apps/auth-service/internal/user/ -v`
Expected: PASS — all `TestUpdateTheme*` subtests plus the pre-existing suite.

- [ ] **Step 5: Commit**

```bash
./tools/lint.sh --go apps/auth-service
git add apps/auth-service/internal/user/processor.go apps/auth-service/internal/user/processor_test.go
git commit -m "feat(auth-service): add Processor.UpdateTheme"
```

---

## Task 4: Expose `themePreference` on the user resource

**Files:**
- Modify: `apps/auth-service/internal/user/rest.go`
- Modify: `apps/auth-service/internal/user/resource_test.go` (generalise the harness, add the FR-TEST-3 regression)

**Interfaces:**
- Consumes: `Model.ThemePreference()`.
- Produces: `Attributes.ThemePreference` with JSON tag `themePreference`; test helpers `newAuthRouter(t) (chi.Router, *gorm.DB)` and `serve(r chi.Router, method, path, body, userID string) *httptest.ResponseRecorder`, both consumed by Task 5.

The harness is generalised now rather than duplicated later (design §9): Task 5 needs to drive a PATCH and then a GET **against the same database** to prove the write landed.

- [ ] **Step 1: Write the failing test**

Replace the top of `apps/auth-service/internal/user/resource_test.go` — everything from the `serveMe` doc comment through the closing brace of `serveMe` — with:

```go
// The regression guard for the login loop, at the layer where the bug actually
// lived. provider_test.go pins what each lookup means, but it cannot catch this
// handler calling the wrong one — reintroducing `proc.GetBySub(id.UserID)` here
// leaves every provider test green. This test drives the real chi route against
// a real database, so the wiring itself is covered.
//
// Symptom when this fails: GET /auth/me 404s for a perfectly valid token, the
// SPA reads that as logged-out, and the user is bounced back to the login page
// forever — having just completed a successful Google round-trip.

// newAuthRouter builds the real router over a seeded database and returns both,
// so a test can drive several requests against ONE dataset — PATCH then GET, to
// prove the write actually landed rather than merely echoing its own input.
func newAuthRouter(t *testing.T) (chi.Router, *gorm.DB) {
	t.Helper()
	db := newTestDB(t)
	seedUser(t, db)

	log := logrus.New()
	log.SetOutput(io.Discard)

	r := chi.NewRouter()
	r.Group(InitializeRoutes(log, db))
	return r, db
}

// serve drives one request with a validated Identity in context, standing in
// for the JWT middleware the real router mounts upstream.
func serve(r chi.Router, method, path, body, userID string) *httptest.ResponseRecorder {
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	req = req.WithContext(auth.WithIdentity(req.Context(), auth.Identity{
		UserID:        userID,
		Email:         "a@b.com",
		ActiveFleetID: "fleet-1",
		Role:          "owner",
	}))

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func serveMe(t *testing.T, userID string) *httptest.ResponseRecorder {
	t.Helper()
	r, _ := newAuthRouter(t)
	return serve(r, http.MethodGet, "/auth/me", "", userID)
}
```

Add `"gorm.io/gorm"` to the import block.

Then append the FR-TEST-3 regression:

```go
// FR-TEST-3 / PRD §5.1: the attribute is always present, so a client never has
// to guess a default.
func TestAuthMe_includesThemePreference(t *testing.T) {
	rec := serveMe(t, testUserID)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /auth/me = %d, want 200. Body: %s", rec.Code, strings.TrimSpace(rec.Body.String()))
	}
	if !strings.Contains(rec.Body.String(), `"themePreference":"system"`) {
		t.Fatalf("GET /auth/me omitted themePreference. Body: %s", rec.Body.String())
	}
}

// FR-DATA-4 end to end: a row holding a value the allow-list does not know —
// written before the column existed, or edited out of band — surfaces as
// `system` rather than leaking an out-of-range value to the SPA.
func TestAuthMe_normalisesAnEmptyStoredTheme(t *testing.T) {
	r, db := newAuthRouter(t)
	if err := db.Exec("UPDATE auth.users SET theme_preference = '' WHERE id = ?", testUserID).Error; err != nil {
		t.Fatalf("blank the stored theme: %v", err)
	}

	rec := serve(r, http.MethodGet, "/auth/me", "", testUserID)
	if !strings.Contains(rec.Body.String(), `"themePreference":"system"`) {
		t.Fatalf("a blank stored theme must read back as system. Body: %s", rec.Body.String())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./apps/auth-service/internal/user/ -run TestAuthMe -v`
Expected: FAIL — `TestAuthMe_includesThemePreference` reports the body has no `themePreference`.

- [ ] **Step 3: Add the attribute**

Replace `apps/auth-service/internal/user/rest.go` in full:

```go
package user

import "github.com/jtumidanski/myfleet/packages/shared-go/server"

// Mirrored by apps/web/src/types/models/user.ts — keep the JSON tags and that
// file's UserAttributes in step.
type Attributes struct {
	Email           string `json:"email"`
	DisplayName     string `json:"displayName"`
	AvatarURL       string `json:"avatarUrl"`
	ThemePreference string `json:"themePreference"`
}

func Transform(m Model) server.Resource {
	return server.Resource{Type: "users", ID: m.ID(), Attributes: Attributes{Email: m.Email(), DisplayName: m.DisplayName(), AvatarURL: m.AvatarURL(), ThemePreference: m.ThemePreference()}}
}

func TransformSlice(ms []Model) []server.Resource {
	out := make([]server.Resource, 0, len(ms))
	for _, m := range ms {
		out = append(out, Transform(m))
	}
	return out
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./apps/auth-service/... -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
./tools/lint.sh --go apps/auth-service
git add apps/auth-service/internal/user/rest.go apps/auth-service/internal/user/resource_test.go
git commit -m "feat(auth-service): return themePreference from GET /auth/me"
```

---

## Task 5: `PATCH /auth/me`

**Files:**
- Modify: `apps/auth-service/internal/user/resource.go`
- Test: `apps/auth-service/internal/user/resource_test.go`

**Interfaces:**
- Consumes: `newAuthRouter`, `serve` (Task 4); `UpdateTheme`, `ErrInvalidTheme`, `ErrNotFound`.
- Produces: `PATCH /auth/me` on the existing JWT-protected group.

The target user is `id.UserID` — the validated JWT `sub`. No path parameter, no body field, no query parameter carries a user identifier, so horizontal privilege escalation is not a check that could be forgotten but a shape that cannot express the attack (FR-SEC-1).

Status mapping: 401 from the middleware (not the handler); 422 for a malformed body (from `RegisterInputHandler`) and for a value outside the allow-list; 404 for an unknown user; bare 500 for a persistence failure.

- [ ] **Step 1: Write the failing test**

Append to `apps/auth-service/internal/user/resource_test.go`:

```go
const patchBody = `{"data":{"type":"users","attributes":{"themePreference":%q}}}`

// The value must survive a round trip through storage, not merely be echoed
// back out of the request that set it.
func TestPatchMe_persistsAndEchoesTheNewPreference(t *testing.T) {
	r, _ := newAuthRouter(t)

	rec := serve(r, http.MethodPatch, "/auth/me", fmt.Sprintf(patchBody, ThemeDark), testUserID)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH /auth/me = %d, want 200. Body: %s", rec.Code, strings.TrimSpace(rec.Body.String()))
	}
	if !strings.Contains(rec.Body.String(), `"themePreference":"dark"`) {
		t.Fatalf("PATCH /auth/me did not echo the new value. Body: %s", rec.Body.String())
	}

	got := serve(r, http.MethodGet, "/auth/me", "", testUserID)
	if !strings.Contains(got.Body.String(), `"themePreference":"dark"`) {
		t.Fatalf("the PATCH did not reach storage — a later GET returned: %s", got.Body.String())
	}
}

// PRD §5.2: the field is required. PATCH-as-partial-update is not supported for
// a single-field resource, so an absent or empty value is a client error rather
// than a no-op that silently reports success.
func TestPatchMe_rejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"empty string", fmt.Sprintf(patchBody, "")},
		{"unknown value", fmt.Sprintf(patchBody, "purple")},
		{"absent attribute", `{"data":{"type":"users","attributes":{}}}`},
		{"malformed json", `{"data":`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, _ := newAuthRouter(t)

			// 422, not the PRD's 400: shared-go has no 400 sentinel, and
			// RegisterInputHandler already renders a malformed body as 422
			// (design §3.1).
			rec := serve(r, http.MethodPatch, "/auth/me", tt.body, testUserID)
			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("PATCH %s = %d, want 422. Body: %s", tt.name, rec.Code, strings.TrimSpace(rec.Body.String()))
			}

			// A rejected write must leave the stored value alone.
			got := serve(r, http.MethodGet, "/auth/me", "", testUserID)
			if !strings.Contains(got.Body.String(), `"themePreference":"system"`) {
				t.Fatalf("a rejected PATCH modified the stored value: %s", got.Body.String())
			}
		})
	}
}

// The title names the field and the accepted values without echoing the
// caller's raw input back (PRD §5.2, FR-SEC-2).
func TestPatchMe_validationTitleNamesTheFieldNotTheInput(t *testing.T) {
	r, _ := newAuthRouter(t)

	rec := serve(r, http.MethodPatch, "/auth/me", fmt.Sprintf(patchBody, "<script>purple"), testUserID)
	body := rec.Body.String()
	if !strings.Contains(body, "themePreference") || !strings.Contains(body, "light, dark, system") {
		t.Fatalf("the validation error must name the field and the allow-list: %s", body)
	}
	if strings.Contains(body, "purple") || strings.Contains(body, "<script>") {
		t.Fatalf("the validation error echoed the caller's raw input back: %s", body)
	}
}

func TestPatchMe_unknownUserIsNotFound(t *testing.T) {
	r, _ := newAuthRouter(t)

	rec := serve(r, http.MethodPatch, "/auth/me", fmt.Sprintf(patchBody, ThemeDark), testGoogleSub)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("PATCH /auth/me for an unknown user = %d, want 404. Body: %s",
			rec.Code, strings.TrimSpace(rec.Body.String()))
	}
}
```

Add `"fmt"` to the import block.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./apps/auth-service/internal/user/ -run TestPatchMe -v`
Expected: FAIL — every case returns 405 (chi has no PATCH route registered).

- [ ] **Step 3: Write the implementation**

In `apps/auth-service/internal/user/resource.go`, add the validation sentinel below `errInternal`:

```go
// errThemeValidation names the offending field and the accepted values without
// echoing the caller's input (PRD §5.2, FR-SEC-2). It wraps server.ErrValidation
// so StatusFor still yields 422, and the message is a compile-time constant so
// no attacker-supplied string can reach the response title.
//
// Note the pair: user.ErrInvalidTheme is the DOMAIN error the processor
// returns; errThemeValidation is the TRANSPORT envelope rendered for it.
var errThemeValidation = fmt.Errorf("%w: themePreference must be one of light, dark, system",
	server.ErrValidation)
```

Add `"fmt"` to the import block.

Then register the route inside the `InitializeRoutes` closure, immediately after the existing `r.Get("/auth/me", …)` block, so both share the one `Processor`:

```go
		// PATCH /auth/me — updates the caller's own preferences (PRD §5.2).
		//
		// The target user is id.UserID, the validated JWT `sub`. There is no
		// path parameter, no body field and no query parameter carrying a user
		// identifier, so horizontal privilege escalation is not a check that
		// could be forgotten but a shape that cannot express the attack
		// (FR-SEC-1).
		r.Patch("/auth/me", server.RegisterInputHandler(func(w http.ResponseWriter, req *http.Request, attrs struct {
			ThemePreference string `json:"themePreference"`
		},
		) {
			id := auth.IdentityFromContext(req.Context())
			m, err := proc.UpdateTheme(id.UserID, attrs.ThemePreference)
			if err != nil {
				// Client errors are not incidents — do not log them (FR-OBS-1).
				if errors.Is(err, ErrInvalidTheme) {
					server.WriteError(w, errThemeValidation)
					return
				}
				if errors.Is(err, ErrNotFound) {
					server.WriteError(w, server.ErrNotFound)
					return
				}
				log.WithError(err).WithField("user_id", id.UserID).Error("auth/me theme update failed")
				// Deliberately not WriteError(w, err): the envelope puts
				// err.Error() in the title, which would leak database internals
				// (FR-SEC-3).
				server.WriteError(w, errInternal)
				return
			}
			// No meta block: active fleet and role are token-derived and
			// unaffected by this call (PRD §5.2).
			server.WriteJSON(w, http.StatusOK, server.Document{Data: Transform(m)})
		}))
```

Update the `InitializeRoutes` doc comment to mention both routes:

```go
// InitializeRoutes wires GET /auth/me (design §8.1, FR-AUTH-3) and
// PATCH /auth/me (PRD §5.2). Active fleet/role are read from the validated
// token's Identity; profile from the DB.
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./apps/auth-service/... -v`
Expected: PASS — all `TestPatchMe*` cases plus the whole pre-existing suite.

- [ ] **Step 5: Commit**

```bash
./tools/lint.sh --go apps/auth-service
git add apps/auth-service/internal/user/resource.go apps/auth-service/internal/user/resource_test.go
git commit -m "feat(auth-service): add PATCH /auth/me for theme preference"
```

---

## Task 6: Semantic status tokens

**Files:**
- Modify: `apps/web/src/index.css`
- Modify: `apps/web/tailwind.config.ts`
- Modify: `apps/web/src/types/models/user.ts`
- Create: `docs/tasks/task-003-dark-mode-branding/contrast.md`

**Interfaces:**
- Consumes: nothing.
- Produces: `ThemePreference` type (`'light' | 'dark' | 'system'`) and `UserAttributes.themePreference`; Tailwind classes `text-{success,warning,danger,info}`, `bg-*-subtle`, `text-*-subtle-foreground`, `border-*-border`.

`ThemePreference` lives in `types/models/user.ts` because it is part of the API contract and that file already declares itself a mirror of `rest.go`. `ResolvedTheme` will live in `lib/theme.ts` (Task 7) because it never crosses the wire. Keeping them in separate files is the cheapest defence against the collapse PRD §6.3 warns about — `system` is a valid preference but never a valid resolved value.

**The ratios below are computed, not estimated.** All sixteen required text pairings clear 4.5:1; the tightest is light `--warning` on white at 5.01:1. Do not adjust these values.

- [ ] **Step 1: Add the tokens to `index.css`**

In `apps/web/src/index.css`, inside the `:root` block, after `--ring: 222.2 84% 4.9%;` and before `--radius`:

```css
    /* Semantic status families (FR-TOKEN-1). The bare token is for text and
       numerals on --background/--card; the -subtle / -subtle-foreground /
       -border trio is for chips and callout blocks. Distinct from
       --destructive, which styles destructive CONTROLS and is untouched
       (FR-TOKEN-4). Measured contrast ratios: docs/tasks/
       task-003-dark-mode-branding/contrast.md */
    --success: 142.4 71.8% 29.2%;
    --success-subtle: 140.6 84.2% 92.5%;
    --success-subtle-foreground: 142.8 64.2% 24.1%;
    --success-border: 141 78.9% 85.1%;
    --warning: 26 90.5% 37.1%;
    --warning-subtle: 48 96.5% 88.8%;
    --warning-subtle-foreground: 22.7 82.5% 31.4%;
    --warning-border: 48 96.6% 76.7%;
    --danger: 0 72.2% 41.1%;
    --danger-subtle: 0 93.3% 94.1%;
    --danger-subtle-foreground: 0 70% 35.3%;
    --danger-border: 0 96.3% 89.4%;
    --info: 224.3 76.3% 48%;
    --info-subtle: 214.3 94.6% 92.7%;
    --info-subtle-foreground: 226 70.7% 40.2%;
    --info-border: 213.3 96.9% 87.3%;
```

Inside the `.dark` block, after `--ring: 212.7 26.8% 83.9%;`:

```css
    /* Not inversions (FR-TOKEN-5). The fills are low-lightness/moderate
       saturation and the foregrounds high-lightness, mirroring the light
       -100/-800 relationship rather than negating it. The bare tokens move to
       the 400 band because a 700-band colour on a 222.2 84% 4.9% background
       fails badly. */
    --success: 141.7 76.6% 73.1%;
    --success-subtle: 142 40% 14%;
    --success-subtle-foreground: 142 70% 78%;
    --success-border: 142 35% 24%;
    --warning: 43.3 96.4% 56.3%;
    --warning-subtle: 30 45% 14%;
    --warning-subtle-foreground: 43 90% 76%;
    --warning-border: 32 40% 25%;
    --danger: 0 90.6% 70.8%;
    --danger-subtle: 0 45% 15%;
    --danger-subtle-foreground: 0 90% 80%;
    --danger-border: 0 40% 26%;
    --info: 213.1 93.9% 67.8%;
    --info-subtle: 217 45% 17%;
    --info-subtle-foreground: 213 92% 80%;
    --info-border: 217 40% 28%;
```

- [ ] **Step 2: Register them with Tailwind**

In `apps/web/tailwind.config.ts`, inside `theme.extend.colors`, after the `card` entry:

```ts
        success: {
          DEFAULT: 'hsl(var(--success))',
          subtle: 'hsl(var(--success-subtle))',
          'subtle-foreground': 'hsl(var(--success-subtle-foreground))',
          border: 'hsl(var(--success-border))',
        },
        warning: {
          DEFAULT: 'hsl(var(--warning))',
          subtle: 'hsl(var(--warning-subtle))',
          'subtle-foreground': 'hsl(var(--warning-subtle-foreground))',
          border: 'hsl(var(--warning-border))',
        },
        danger: {
          DEFAULT: 'hsl(var(--danger))',
          subtle: 'hsl(var(--danger-subtle))',
          'subtle-foreground': 'hsl(var(--danger-subtle-foreground))',
          border: 'hsl(var(--danger-border))',
        },
        info: {
          DEFAULT: 'hsl(var(--info))',
          subtle: 'hsl(var(--info-subtle))',
          'subtle-foreground': 'hsl(var(--info-subtle-foreground))',
          border: 'hsl(var(--info-border))',
        },
```

`packages/ui-components/src` is already in the `content` globs (`tailwind.config.ts:9`), so `StatusBadge` picks these up with no further config change.

- [ ] **Step 3: Add the frontend contract type**

Replace `apps/web/src/types/models/user.ts` in full:

```ts
import type { JsonApiResource } from '@myfleet/shared-ts';

// The user's theme choice, as stored server-side. `system` is a valid
// PREFERENCE but never a valid RESOLVED theme — see ResolvedTheme in
// src/lib/theme.ts. Collapsing the two is the most likely source of bugs here,
// which is why they live in separate files.
export type ThemePreference = 'light' | 'dark' | 'system';

// Mirrors auth-service user resource (apps/auth-service/internal/user/rest.go).
export interface UserAttributes {
  email: string;
  displayName: string;
  avatarUrl: string;
  themePreference: ThemePreference;
}

export type User = JsonApiResource<UserAttributes>;

// Roles within the active fleet (apps/fleet-service/internal/authz/scope.go).
export type FleetRole = 'owner' | 'member' | 'viewer';

// `GET /api/auth/me` meta block.
export interface AuthMeta {
  activeFleetId: string | null;
  role: FleetRole | null;
}
```

- [ ] **Step 4: Record the contrast ratios (FR-A11Y-1)**

Create `docs/tasks/task-003-dark-mode-branding/contrast.md`:

````markdown
# Status token contrast record

FR-A11Y-1 requires a recorded ratio for every new token pairing in both themes.
These are **computed**, not estimated — reproduce them with the script at the
bottom of this file.

`--background` and `--card` hold identical values in both themes (`0 0% 100%`
light, `222.2 84% 4.9%` dark), so "bare on background" and "bare on card"
collapse into one measurement per family per theme. That gives the sixteen
pairings the design calls for: 4 families × 2 measurements × 2 themes.

## Light theme

| Family | bare on `--background` / `--card` | `-subtle-foreground` on `-subtle` |
|---|---|---|
| success | 5.02:1 | 6.50:1 |
| warning | 5.01:1 | 6.36:1 |
| danger | 6.67:1 | 6.80:1 |
| info | 6.71:1 | 7.15:1 |

## Dark theme

| Family | bare on `--background` / `--card` | `-subtle-foreground` on `-subtle` |
|---|---|---|
| success | 14.23:1 | 10.25:1 |
| warning | 11.99:1 | 10.97:1 |
| danger | 7.23:1 | 8.14:1 |
| info | 7.85:1 | 8.60:1 |

**All sixteen clear the 4.5:1 body-text threshold.** The tightest is light
`--warning` on white at 5.01:1.

## On the `-border` tokens

The `-border` values are deliberately low-contrast against the page background
(1.2–2.3:1) and are **not** part of the FR-A11Y-1 requirement. They are
decorative separators around chips and callouts whose meaning is carried by a
text label in every case (FR-A11Y-2) — they are not the sole means of
identifying a UI component or its state, which is what WCAG 1.4.11 governs. A
chip border pulled to 3:1 would read as a heavy outline and fight the subtle
fill it encloses.

## Reproducing

```python
import colorsys

def rgb(h, s, l):
    return colorsys.hls_to_rgb(h / 360.0, l / 100.0, s / 100.0)

def luminance(c):
    f = lambda v: v / 12.92 if v <= 0.03928 else ((v + 0.055) / 1.055) ** 2.4
    r, g, b = (f(v) for v in c)
    return 0.2126 * r + 0.7152 * g + 0.0722 * b

def ratio(a, b):
    la, lb = luminance(rgb(*a)), luminance(rgb(*b))
    hi, lo = max(la, lb), min(la, lb)
    return (hi + 0.05) / (lo + 0.05)

LIGHT = {
    'success': {'bare': (142.4, 71.8, 29.2), 'subtle': (140.6, 84.2, 92.5), 'sf': (142.8, 64.2, 24.1)},
    'warning': {'bare': (26, 90.5, 37.1), 'subtle': (48, 96.5, 88.8), 'sf': (22.7, 82.5, 31.4)},
    'danger':  {'bare': (0, 72.2, 41.1), 'subtle': (0, 93.3, 94.1), 'sf': (0, 70, 35.3)},
    'info':    {'bare': (224.3, 76.3, 48), 'subtle': (214.3, 94.6, 92.7), 'sf': (226, 70.7, 40.2)},
}
DARK = {
    'success': {'bare': (141.7, 76.6, 73.1), 'subtle': (142, 40, 14), 'sf': (142, 70, 78)},
    'warning': {'bare': (43.3, 96.4, 56.3), 'subtle': (30, 45, 14), 'sf': (43, 90, 76)},
    'danger':  {'bare': (0, 90.6, 70.8), 'subtle': (0, 45, 15), 'sf': (0, 90, 80)},
    'info':    {'bare': (213.1, 93.9, 67.8), 'subtle': (217, 45, 17), 'sf': (213, 92, 80)},
}

for theme, table, bg in (('light', LIGHT, (0, 0, 100)), ('dark', DARK, (222.2, 84, 4.9))):
    for family, t in table.items():
        print(f"{theme:6} {family:8} bare-on-bg {ratio(t['bare'], bg):5.2f}:1"
              f"   sf-on-subtle {ratio(t['sf'], t['subtle']):5.2f}:1")
```
````

- [ ] **Step 5: Verify the build and formatting**

```bash
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
npm run format
npm run -w apps/web build
```

Expected: `tsc -b` fails is acceptable **only** if the failure is `themePreference` missing from a fixture — there are none in the tree, so it should build clean. If `tsc` reports errors in unrelated files, stop and investigate.

- [ ] **Step 6: Commit**

```bash
git add apps/web/src/index.css apps/web/tailwind.config.ts apps/web/src/types/models/user.ts docs/tasks/task-003-dark-mode-branding/contrast.md
git commit -m "feat(web): add semantic status tokens and the ThemePreference type"
```

---

## Task 7: Pure theme helpers

**Files:**
- Create: `apps/web/src/lib/theme.ts`
- Test: `apps/web/src/lib/theme.test.ts`

**Interfaces:**
- Consumes: `ThemePreference` (Task 6).
- Produces: `THEME_STORAGE_KEY`; `ResolvedTheme`; `isThemePreference(v: unknown): v is ThemePreference`; `readCachedTheme(): ThemePreference | null`; `writeCachedTheme(p: ThemePreference): void`; `resolveTheme(p: ThemePreference, prefersDark: boolean): ResolvedTheme`; `applyThemeClass(r: ResolvedTheme): void`.

`resolveTheme` takes `prefersDark` as a **parameter** rather than reading `matchMedia` itself. That is what makes FR-TEST-4's "system resolution in both media states" a two-line test instead of a jsdom mocking exercise, and it is the reason this module needs no test setup at all.

- [ ] **Step 1: Write the failing test**

`apps/web/src/lib/theme.test.ts`:

```ts
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import {
  THEME_STORAGE_KEY,
  applyThemeClass,
  isThemePreference,
  readCachedTheme,
  resolveTheme,
  writeCachedTheme,
} from './theme';

describe('isThemePreference', () => {
  it('accepts exactly the three valid values', () => {
    expect(isThemePreference('light')).toBe(true);
    expect(isThemePreference('dark')).toBe(true);
    expect(isThemePreference('system')).toBe(true);
  });

  it('rejects anything else', () => {
    expect(isThemePreference('purple')).toBe(false);
    expect(isThemePreference('')).toBe(false);
    expect(isThemePreference('Dark')).toBe(false);
    expect(isThemePreference(null)).toBe(false);
    expect(isThemePreference(undefined)).toBe(false);
    expect(isThemePreference(1)).toBe(false);
  });
});

describe('resolveTheme', () => {
  // FR-THEME-3. The media state is a parameter, so both branches are testable
  // without touching matchMedia.
  it('resolves system from the media state', () => {
    expect(resolveTheme('system', true)).toBe('dark');
    expect(resolveTheme('system', false)).toBe('light');
  });

  // FR-THEME-6: an explicit choice ignores the OS.
  it('ignores the media state for an explicit preference', () => {
    expect(resolveTheme('light', true)).toBe('light');
    expect(resolveTheme('dark', false)).toBe('dark');
  });
});

describe('readCachedTheme', () => {
  beforeEach(() => localStorage.clear());

  it('returns null when the key is absent', () => {
    expect(readCachedTheme()).toBeNull();
  });

  it('returns a cached valid value', () => {
    localStorage.setItem(THEME_STORAGE_KEY, 'dark');
    expect(readCachedTheme()).toBe('dark');
  });

  // FR-SEC-4: the stored value is attacker-controllable — any script or
  // extension on the origin can write it — so it is validated before use.
  it('returns null for a corrupted value', () => {
    localStorage.setItem(THEME_STORAGE_KEY, 'purple');
    expect(readCachedTheme()).toBeNull();
  });

  // FR-FLASH-3: localStorage blocked by privacy settings must not break boot.
  it('returns null when localStorage throws', () => {
    const spy = vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('blocked');
    });
    expect(readCachedTheme()).toBeNull();
    spy.mockRestore();
  });
});

describe('writeCachedTheme', () => {
  beforeEach(() => localStorage.clear());

  it('writes under the shared key', () => {
    writeCachedTheme('light');
    expect(localStorage.getItem(THEME_STORAGE_KEY)).toBe('light');
  });

  it('swallows a throwing localStorage', () => {
    const spy = vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('blocked');
    });
    expect(() => writeCachedTheme('dark')).not.toThrow();
    spy.mockRestore();
  });
});

describe('applyThemeClass', () => {
  afterEach(() => document.documentElement.classList.remove('dark'));

  // FR-THEME-4: the class on <html>, nothing else.
  it('adds and removes the dark class on the document element', () => {
    applyThemeClass('dark');
    expect(document.documentElement.classList.contains('dark')).toBe(true);
    applyThemeClass('light');
    expect(document.documentElement.classList.contains('dark')).toBe(false);
  });
});
```

Note on the two "throws" cases: `src/test/setup.ts` replaces `localStorage` with a plain `MemoryStorage` class instance, **not** a real `Storage`, so `Storage.prototype` may not be on its prototype chain. If either spy fails to intercept once Step 3 lands, swap it for `vi.spyOn(Object.getPrototypeOf(localStorage) as Storage, 'getItem')` (and `'setItem'`), which targets the polyfill's own prototype.

- [ ] **Step 2: Run test to verify it fails**

```bash
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
npm run -w apps/web test -- src/lib/theme.test.ts
```

Expected: FAIL — cannot resolve `./theme`.

- [ ] **Step 3: Write the implementation**

`apps/web/src/lib/theme.ts`:

```ts
import type { ThemePreference } from '../types/models/user';

/**
 * Pure theme helpers. No React, no network; the only DOM this module touches is
 * the single classList call in applyThemeClass. Keeping the logic here rather
 * than inside the provider is what makes it testable without a DOM-heavy
 * harness.
 */

// Shared with the pre-paint script in apps/web/index.html. If this changes,
// that script changes too — src/test/conventions.test.ts pins the script's
// presence but cannot check the key for you.
export const THEME_STORAGE_KEY = 'myfleet.theme';

// The COMPUTED outcome, distinct from the user's ThemePreference: `system` is a
// valid preference but never a valid resolved value. This type stays out of
// types/models/user.ts because it never crosses the wire.
export type ResolvedTheme = 'light' | 'dark';

const VALID: readonly string[] = ['light', 'dark', 'system'];

export function isThemePreference(value: unknown): value is ThemePreference {
  return typeof value === 'string' && VALID.includes(value);
}

/**
 * The cached preference, or null if absent, corrupted, or unreadable.
 *
 * The stored value is attacker-controllable — any script or extension on the
 * origin can write it — so it is validated against the allow-list before use
 * (FR-SEC-4), and a blocked localStorage yields null rather than throwing
 * (FR-FLASH-3).
 */
export function readCachedTheme(): ThemePreference | null {
  try {
    const raw = localStorage.getItem(THEME_STORAGE_KEY);
    return isThemePreference(raw) ? raw : null;
  } catch {
    return null;
  }
}

/** Best-effort cache write. This is a cache, not a source of truth (FR-PERSIST-2). */
export function writeCachedTheme(preference: ThemePreference): void {
  try {
    localStorage.setItem(THEME_STORAGE_KEY, preference);
  } catch {
    // Storage blocked by privacy settings. The preference still applies for
    // this session; only the pre-paint hint is lost, so the next load flashes
    // once rather than failing to boot.
  }
}

/**
 * FR-THEME-3. `prefersDark` is a parameter rather than a matchMedia read so
 * both branches are testable without mocking jsdom.
 */
export function resolveTheme(preference: ThemePreference, prefersDark: boolean): ResolvedTheme {
  if (preference === 'system') return prefersDark ? 'dark' : 'light';
  return preference;
}

/**
 * FR-THEME-4: the `dark` class on <html>, and nothing else — tailwind.config.ts
 * is already set to `darkMode: ['class']`. The class name is a literal; the
 * stored preference is never concatenated into it (FR-SEC-4).
 */
export function applyThemeClass(resolved: ResolvedTheme): void {
  document.documentElement.classList.toggle('dark', resolved === 'dark');
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `npm run -w apps/web test -- src/lib/theme.test.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
npm run format
git add apps/web/src/lib/theme.ts apps/web/src/lib/theme.test.ts
git commit -m "feat(web): add pure theme helpers"
```

---

## Task 8: `ThemeContext`

**Files:**
- Create: `apps/web/src/context/ThemeContext.tsx`
- Modify: `apps/web/src/test/setup.ts` (add the `matchMedia` stub)
- Test: `apps/web/src/context/ThemeContext.test.tsx`

**Interfaces:**
- Consumes: `lib/theme` (Task 7), `ThemePreference` (Task 6).
- Produces: `ThemeProvider`; `useTheme(): ThemeContextValue` where `ThemeContextValue = { preference: ThemePreference; resolvedTheme: ResolvedTheme; setPreference(p): void; adoptServerPreference(p): void; clearLocalOverride(): void }`. Test helpers `setPrefersDark(value: boolean): void` and `resetMatchMedia(): void` exported from `src/test/setup.ts`.

The provider makes **no network call and knows nothing about auth**. Two write paths, and the distinction is load-bearing:

- `setPreference` — **user intent**. Sets state, writes the cache, and marks the session as locally overridden.
- `adoptServerPreference` — **server truth**. Same effects, but a no-op once the user has chosen this session.

The `hasLocalOverride` ref is what stops a background `me` refetch carrying the pre-change server value from flipping the theme back under the user's cursor (FR-PERSIST-5). It starts `false`, so the first `me` result of a session always wins — which is exactly FR-PERSIST-3, and what makes the preference land on a newly signed-in device.

The media listener is registered unconditionally; `resolveTheme` simply ignores `systemPrefersDark` unless the preference is `system`. That satisfies FR-THEME-6 without conditional subscription logic.

- [ ] **Step 1: Add the `matchMedia` stub**

Append to `apps/web/src/test/setup.ts`:

```ts
// jsdom's matchMedia is a stub that never fires `change`, so theme code
// subscribing to (prefers-color-scheme: dark) cannot be exercised against it.
// This replacement is driven from tests via setPrefersDark, which is what makes
// FR-TEST-5's live-update case testable at all.
const DARK_QUERY = '(prefers-color-scheme: dark)';

type ChangeListener = (event: MediaQueryListEvent) => void;

const mediaListeners = new Set<ChangeListener>();
let mediaPrefersDark = false;

/** Flip the simulated OS appearance and fire `change` at every listener. */
export function setPrefersDark(value: boolean): void {
  if (mediaPrefersDark === value) return;
  mediaPrefersDark = value;
  const event = { matches: value, media: DARK_QUERY } as MediaQueryListEvent;
  mediaListeners.forEach((listener) => listener(event));
}

/** Restore the default (light) and drop any listeners a test left behind. */
export function resetMatchMedia(): void {
  mediaListeners.clear();
  mediaPrefersDark = false;
}

Object.defineProperty(window, 'matchMedia', {
  writable: true,
  configurable: true,
  value: (query: string): MediaQueryList =>
    ({
      get matches() {
        return query === DARK_QUERY ? mediaPrefersDark : false;
      },
      media: query,
      onchange: null,
      addEventListener: (_type: 'change', listener: ChangeListener) => {
        mediaListeners.add(listener);
      },
      removeEventListener: (_type: 'change', listener: ChangeListener) => {
        mediaListeners.delete(listener);
      },
      addListener: () => {},
      removeListener: () => {},
      dispatchEvent: () => false,
    }) as unknown as MediaQueryList,
});

/** Listener count, so a test can assert the provider unsubscribes on unmount. */
export function mediaListenerCount(): number {
  return mediaListeners.size;
}
```

- [ ] **Step 2: Write the failing test**

`apps/web/src/context/ThemeContext.test.tsx`:

```tsx
import { describe, it, expect, beforeEach } from 'vitest';
import { render, screen, act } from '@testing-library/react';
import { ThemeProvider, useTheme } from './ThemeContext';
import { THEME_STORAGE_KEY } from '../lib/theme';
import { setPrefersDark, resetMatchMedia, mediaListenerCount } from '../test/setup';

function Probe() {
  const { preference, resolvedTheme, setPreference, adoptServerPreference } = useTheme();
  return (
    <div>
      <span data-testid="preference">{preference}</span>
      <span data-testid="resolved">{resolvedTheme}</span>
      <button type="button" onClick={() => setPreference('dark')}>
        choose dark
      </button>
      <button type="button" onClick={() => adoptServerPreference('light')}>
        adopt light
      </button>
    </div>
  );
}

function renderProvider() {
  return render(
    <ThemeProvider>
      <Probe />
    </ThemeProvider>,
  );
}

describe('ThemeProvider', () => {
  beforeEach(() => {
    localStorage.clear();
    resetMatchMedia();
    document.documentElement.classList.remove('dark');
  });

  it('defaults to system when nothing is cached', () => {
    renderProvider();
    expect(screen.getByTestId('preference')).toHaveTextContent('system');
    expect(screen.getByTestId('resolved')).toHaveTextContent('light');
  });

  it('resolves the initial theme from the cache', () => {
    localStorage.setItem(THEME_STORAGE_KEY, 'dark');
    renderProvider();
    expect(screen.getByTestId('preference')).toHaveTextContent('dark');
    expect(screen.getByTestId('resolved')).toHaveTextContent('dark');
    expect(document.documentElement.classList.contains('dark')).toBe(true);
  });

  it('falls back to system for a corrupted cached value', () => {
    localStorage.setItem(THEME_STORAGE_KEY, 'purple');
    renderProvider();
    expect(screen.getByTestId('preference')).toHaveTextContent('system');
  });

  // FR-THEME-5: the OS flipping at sunset updates the app with no reload.
  it('follows live media changes while on system', () => {
    renderProvider();
    expect(screen.getByTestId('resolved')).toHaveTextContent('light');

    act(() => setPrefersDark(true));
    expect(screen.getByTestId('resolved')).toHaveTextContent('dark');
    expect(document.documentElement.classList.contains('dark')).toBe(true);
  });

  // FR-THEME-6: an explicit choice pins the theme against the OS.
  it('ignores media changes while on an explicit preference', () => {
    localStorage.setItem(THEME_STORAGE_KEY, 'light');
    renderProvider();

    act(() => setPrefersDark(true));
    expect(screen.getByTestId('resolved')).toHaveTextContent('light');
    expect(document.documentElement.classList.contains('dark')).toBe(false);
  });

  it('removes the media listener on unmount', () => {
    const { unmount } = renderProvider();
    expect(mediaListenerCount()).toBeGreaterThan(0);
    unmount();
    expect(mediaListenerCount()).toBe(0);
  });

  it('setPreference applies and caches the choice', () => {
    renderProvider();
    act(() => screen.getByText('choose dark').click());

    expect(screen.getByTestId('resolved')).toHaveTextContent('dark');
    expect(localStorage.getItem(THEME_STORAGE_KEY)).toBe('dark');
  });

  // FR-PERSIST-3: the first server value of a session wins over the cache,
  // which is what makes the preference land on a newly signed-in device.
  it('adoptServerPreference overrides the cache before the user chooses', () => {
    localStorage.setItem(THEME_STORAGE_KEY, 'dark');
    renderProvider();
    act(() => screen.getByText('adopt light').click());

    expect(screen.getByTestId('preference')).toHaveTextContent('light');
    expect(localStorage.getItem(THEME_STORAGE_KEY)).toBe('light');
  });

  // FR-PERSIST-5: once the user has chosen, a background refetch carrying the
  // pre-change server value must not flip the theme out from under them.
  it('adoptServerPreference is a no-op after the user has chosen', () => {
    renderProvider();
    act(() => screen.getByText('choose dark').click());
    act(() => screen.getByText('adopt light').click());

    expect(screen.getByTestId('preference')).toHaveTextContent('dark');
    expect(localStorage.getItem(THEME_STORAGE_KEY)).toBe('dark');
  });
});
```

- [ ] **Step 3: Run test to verify it fails**

Run: `npm run -w apps/web test -- src/context/ThemeContext.test.tsx`
Expected: FAIL — cannot resolve `./ThemeContext`.

- [ ] **Step 4: Write the implementation**

`apps/web/src/context/ThemeContext.tsx`:

```tsx
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from 'react';
import type { ThemePreference } from '../types/models/user';
import {
  applyThemeClass,
  readCachedTheme,
  resolveTheme,
  writeCachedTheme,
  type ResolvedTheme,
} from '../lib/theme';

const MEDIA_QUERY = '(prefers-color-scheme: dark)';

export interface ThemeContextValue {
  preference: ThemePreference;
  resolvedTheme: ResolvedTheme;
  /** User intent: applies, caches, and pins the choice for the session. */
  setPreference: (preference: ThemePreference) => void;
  /** Server truth echoed back; ignored once the user has chosen this session. */
  adoptServerPreference: (preference: ThemePreference) => void;
  /** Called on sign-out so the next sign-in adopts the server value afresh. */
  clearLocalOverride: () => void;
}

const ThemeContext = createContext<ThemeContextValue | undefined>(undefined);

/**
 * Client theme state. Deliberately knows nothing about auth and issues no
 * network request — the server value arrives through ThemeSync, which is what
 * lets this provider be unit-tested with a bare render() rather than a
 * QueryClientProvider plus a token fixture (design §3.3).
 */
export function ThemeProvider({ children }: { children: ReactNode }) {
  const [preference, setPreferenceState] = useState<ThemePreference>(
    () => readCachedTheme() ?? 'system',
  );
  const [systemPrefersDark, setSystemPrefersDark] = useState<boolean>(
    () => window.matchMedia(MEDIA_QUERY).matches,
  );

  // Once the user has chosen this session, a background `me` refetch carrying
  // the pre-change server value must not flip the theme back under their cursor
  // (FR-PERSIST-5). Starts false, so the FIRST server value of a session always
  // wins — that is FR-PERSIST-3, and what lands the preference on a new device.
  const hasLocalOverride = useRef(false);

  const resolvedTheme = resolveTheme(preference, systemPrefersDark);

  useEffect(() => {
    applyThemeClass(resolvedTheme);
  }, [resolvedTheme]);

  // Subscribed unconditionally: resolveTheme ignores systemPrefersDark unless
  // the preference is `system`, so FR-THEME-6 falls out of the resolution rule
  // rather than needing conditional subscribe/unsubscribe logic.
  useEffect(() => {
    const query = window.matchMedia(MEDIA_QUERY);
    const onChange = (event: MediaQueryListEvent) => setSystemPrefersDark(event.matches);
    query.addEventListener('change', onChange);
    return () => query.removeEventListener('change', onChange);
  }, []);

  const setPreference = useCallback((next: ThemePreference) => {
    hasLocalOverride.current = true;
    setPreferenceState(next);
    writeCachedTheme(next);
  }, []);

  const adoptServerPreference = useCallback((next: ThemePreference) => {
    if (hasLocalOverride.current) return;
    setPreferenceState(next);
    writeCachedTheme(next);
  }, []);

  const clearLocalOverride = useCallback(() => {
    hasLocalOverride.current = false;
  }, []);

  const value = useMemo<ThemeContextValue>(
    () => ({
      preference,
      resolvedTheme,
      setPreference,
      adoptServerPreference,
      clearLocalOverride,
    }),
    [preference, resolvedTheme, setPreference, adoptServerPreference, clearLocalOverride],
  );

  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>;
}

export function useTheme(): ThemeContextValue {
  const ctx = useContext(ThemeContext);
  if (!ctx) throw new Error('useTheme must be used within a ThemeProvider');
  return ctx;
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `npm run -w apps/web test`
Expected: PASS — all ten `ThemeProvider` cases plus the existing suite.

- [ ] **Step 6: Commit**

```bash
npm run format
git add apps/web/src/context/ThemeContext.tsx apps/web/src/context/ThemeContext.test.tsx apps/web/src/test/setup.ts
git commit -m "feat(web): add ThemeContext with system-preference tracking"
```

---

## Task 9: `useUpdateTheme`

**Files:**
- Modify: `apps/web/src/lib/hooks/api/auth.ts`
- Test: `apps/web/src/lib/hooks/api/auth.test.ts` (new)

**Interfaces:**
- Consumes: `apiClient`, `getAccessToken`, `authKeys`, `MeResult`, `ThemePreference`, `User`.
- Produces: `updateThemePreference(p: ThemePreference): Promise<User | null>`; `useUpdateTheme()` returning a TanStack mutation whose `mutate` takes a `ThemePreference`.

**The textbook optimistic pattern would violate FR-PERSIST-5.** `onMutate` writes the new value into `authKeys.me()`, and there is deliberately **no `onError` rollback**: rolling back would make the next `ThemeSync` pass re-adopt the old value and flip the theme out from under the user. The cache is knowingly optimistic-but-wrong until a genuine refetch; the toast (Task 10) tells the user exactly that.

`updateThemePreference` **resolves** without a request when there is no token (FR-PERSIST-8) — it must not reject, or a signed-out toggle would raise the failure toast.

- [ ] **Step 1: Write the failing test**

`apps/web/src/lib/hooks/api/auth.test.ts`:

```ts
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { updateThemePreference } from './auth';
import { setAccessToken, clearAccessToken } from '../../api/token';

describe('updateThemePreference', () => {
  beforeEach(() => {
    localStorage.clear();
    clearAccessToken();
  });
  afterEach(() => vi.unstubAllGlobals());

  // FR-PERSIST-8: no token, no request. It RESOLVES rather than rejecting —
  // rejecting would raise the save-failure toast for a user who never had a
  // session to save into.
  it('makes no request and resolves when there is no token', async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);

    await expect(updateThemePreference('dark')).resolves.toBeNull();
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('PATCHes the JSON:API envelope to /api/auth/me', async () => {
    setAccessToken('token-123');
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({
        data: {
          id: 'u1',
          type: 'users',
          attributes: {
            email: 'a@b.com',
            displayName: 'A',
            avatarUrl: '',
            themePreference: 'dark',
          },
        },
      }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const user = await updateThemePreference('dark');

    expect(user?.attributes.themePreference).toBe('dark');
    const [path, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(path).toBe('/api/auth/me');
    expect(init.method).toBe('PATCH');
    expect(JSON.parse(init.body as string)).toEqual({
      data: { type: 'users', attributes: { themePreference: 'dark' } },
    });
  });

  // FR-SEC-1: the target user is the token's `sub`. Nothing in the request
  // names a user, so there is no identifier for a caller to tamper with.
  it('sends no user identifier in the path or body', async () => {
    setAccessToken('token-123');
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({ data: { id: 'u1', type: 'users', attributes: {} } }),
    });
    vi.stubGlobal('fetch', fetchMock);

    await updateThemePreference('light');

    const [path, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(path).not.toMatch(/\/users\//);
    expect(init.body as string).not.toContain('"id"');
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npm run -w apps/web test -- src/lib/hooks/api/auth.test.ts`
Expected: FAIL — `updateThemePreference` is not exported.

- [ ] **Step 3: Write the implementation**

In `apps/web/src/lib/hooks/api/auth.ts`, change the imports:

```ts
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import type { JsonApiDocument } from '@myfleet/shared-ts';
import { apiClient } from '../../api/client';
import { getAccessToken } from '../../api/token';
import type { AuthMeta, ThemePreference, User } from '../../../types/models/user';
```

and append:

```ts
/**
 * `PATCH /api/auth/me` — updates the caller's own theme preference.
 *
 * Resolves with `null` and issues no request when there is no access token
 * (FR-PERSIST-8). It resolves rather than rejecting on purpose: a rejection
 * would raise the save-failure toast at a user who never had a session to save
 * into. Since FR-TOGGLE-7 confines the toggle to the authenticated shell, this
 * is defence-in-depth rather than a live path.
 *
 * No user identifier appears in the path or the body — the server derives the
 * target from the validated JWT `sub` (FR-SEC-1).
 */
export async function updateThemePreference(
  themePreference: ThemePreference,
): Promise<User | null> {
  if (!getAccessToken()) return null;
  const doc = await apiClient.request<JsonApiDocument<User>>('/api/auth/me', {
    method: 'PATCH',
    body: JSON.stringify({ data: { type: 'users', attributes: { themePreference } } }),
  });
  return doc.data;
}

/**
 * Optimistic theme mutation.
 *
 * `onMutate` writes the new value into the identity cache so a later refetch
 * cannot momentarily revert the theme (FR-PERSIST-6).
 *
 * There is deliberately NO onError rollback. The textbook optimistic pattern
 * restores the snapshot on failure, but here that would make the next ThemeSync
 * pass re-adopt the old value and flip the theme out from under the user —
 * exactly what FR-PERSIST-5 forbids. The cache stays knowingly
 * optimistic-but-wrong until a genuine refetch, and ThemeToggle's toast tells
 * the user the preference will not survive the session.
 */
export function useUpdateTheme() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: updateThemePreference,
    onMutate: (themePreference: ThemePreference) => {
      queryClient.setQueryData<MeResult>(authKeys.me(), (previous) =>
        previous
          ? {
              ...previous,
              user: {
                ...previous.user,
                attributes: { ...previous.user.attributes, themePreference },
              },
            }
          : previous,
      );
    },
  });
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `npm run -w apps/web test -- src/lib/hooks/api/auth.test.ts`
Expected: PASS (3 cases).

- [ ] **Step 5: Commit**

```bash
npm run format
git add apps/web/src/lib/hooks/api/auth.ts apps/web/src/lib/hooks/api/auth.test.ts
git commit -m "feat(web): add the optimistic theme-preference mutation"
```

---

## Task 10: `ThemeToggle`

**Files:**
- Create: `apps/web/src/components/ThemeToggle.tsx`
- Test: `apps/web/src/components/ThemeToggle.test.tsx`

**Interfaces:**
- Consumes: `useTheme` (Task 8), `useUpdateTheme` (Task 9), `Button`, `sonner`, `lucide-react`.
- Produces: `<ThemeToggle />`, consumed by `AppLayout` (Task 15).

This is the **only** place where a state change and a network mutation fire together — one file to read to understand when the network is touched.

Cycle: `light` → `dark` → `system` → `light` (FR-TOGGLE-2). The icon reflects the **current preference**, not the resolved theme (FR-TOGGLE-3).

- [ ] **Step 1: Write the failing test**

`apps/web/src/components/ThemeToggle.test.tsx`:

```tsx
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import { ThemeToggle } from './ThemeToggle';
import { ThemeProvider } from '../context/ThemeContext';
import { THEME_STORAGE_KEY } from '../lib/theme';
import { resetMatchMedia } from '../test/setup';

const mutate = vi.fn();
vi.mock('../lib/hooks/api/auth', () => ({
  useUpdateTheme: () => ({ mutate }),
}));

const toastError = vi.fn();
vi.mock('sonner', () => ({
  toast: { error: (...args: unknown[]) => toastError(...args) },
}));

function renderToggle() {
  return render(
    <ThemeProvider>
      <ThemeToggle />
    </ThemeProvider>,
  );
}

describe('ThemeToggle', () => {
  beforeEach(() => {
    localStorage.clear();
    resetMatchMedia();
    document.documentElement.classList.remove('dark');
    mutate.mockReset();
    toastError.mockReset();
  });

  // FR-TOGGLE-2 / FR-TOGGLE-4: the label names the current state AND the next
  // action, so a screen-reader user can operate the cycle without sighted
  // feedback.
  it('cycles light -> dark -> system -> light with matching labels', () => {
    localStorage.setItem(THEME_STORAGE_KEY, 'light');
    renderToggle();

    const button = () => screen.getByRole('button');
    expect(button()).toHaveAttribute('aria-label', 'Theme: light. Switch to dark.');
    expect(button()).toHaveAttribute('title', 'Theme: light');

    button().click();
    expect(button()).toHaveAttribute('aria-label', 'Theme: dark. Switch to system.');

    button().click();
    expect(button()).toHaveAttribute('aria-label', 'Theme: system. Switch to light.');

    button().click();
    expect(button()).toHaveAttribute('aria-label', 'Theme: light. Switch to dark.');
  });

  it('applies the choice to the document immediately', () => {
    localStorage.setItem(THEME_STORAGE_KEY, 'light');
    renderToggle();

    screen.getByRole('button').click();
    expect(document.documentElement.classList.contains('dark')).toBe(true);
  });

  it('issues the mutation with the newly chosen preference', () => {
    localStorage.setItem(THEME_STORAGE_KEY, 'light');
    renderToggle();

    screen.getByRole('button').click();
    expect(mutate).toHaveBeenCalledTimes(1);
    expect(mutate.mock.calls[0][0]).toBe('dark');
  });

  // FR-PERSIST-5 / FR-TEST-7: the user's intent stays applied on failure. The
  // theme is NOT rolled back — reverting under the cursor is more jarring than
  // a preference that fails to stick — and a non-blocking toast explains it.
  it('keeps the theme applied and toasts when the save fails', async () => {
    localStorage.setItem(THEME_STORAGE_KEY, 'light');
    mutate.mockImplementation((_preference: string, options: { onError: () => void }) => {
      options.onError();
    });
    renderToggle();

    screen.getByRole('button').click();

    expect(document.documentElement.classList.contains('dark')).toBe(true);
    expect(localStorage.getItem(THEME_STORAGE_KEY)).toBe('dark');
    await waitFor(() =>
      expect(toastError).toHaveBeenCalledWith(
        "Couldn't save your theme preference. It'll reset next time you sign in.",
      ),
    );
  });

  // FR-TOGGLE-3: the icon tracks the PREFERENCE, not the resolved theme, or
  // `system` would be indistinguishable from whichever theme it resolved to.
  it('shows an icon per preference', () => {
    localStorage.setItem(THEME_STORAGE_KEY, 'system');
    renderToggle();

    expect(screen.getByRole('button').querySelector('svg')).toBeTruthy();
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npm run -w apps/web test -- src/components/ThemeToggle.test.tsx`
Expected: FAIL — cannot resolve `./ThemeToggle`.

- [ ] **Step 3: Write the implementation**

`apps/web/src/components/ThemeToggle.tsx`:

```tsx
import { Monitor, Moon, Sun, type LucideIcon } from 'lucide-react';
import { toast } from 'sonner';
import { useTheme } from '../context/ThemeContext';
import { useUpdateTheme } from '../lib/hooks/api/auth';
import type { ThemePreference } from '../types/models/user';
import { Button } from './ui/button';

// FR-TOGGLE-2: a fixed cycle, so the control is predictable without a menu.
const NEXT: Record<ThemePreference, ThemePreference> = {
  light: 'dark',
  dark: 'system',
  system: 'light',
};

// FR-TOGGLE-3: keyed on the PREFERENCE, not the resolved theme — otherwise
// `system` would be indistinguishable from whichever theme it resolved to.
const META: Record<ThemePreference, { Icon: LucideIcon; label: string }> = {
  light: { Icon: Sun, label: 'light' },
  dark: { Icon: Moon, label: 'dark' },
  system: { Icon: Monitor, label: 'system' },
};

/**
 * The header theme control — the only place in the app where a theme change and
 * a network mutation fire together, so there is exactly one file to read to
 * understand when theming touches the network.
 *
 * The visual change is applied optimistically and is never rolled back on a
 * failed save (FR-PERSIST-4, FR-PERSIST-5): reverting the theme under the
 * user's cursor is more jarring than a preference that fails to stick, so the
 * toast explains the outcome instead.
 */
export function ThemeToggle() {
  const { preference, setPreference } = useTheme();
  const updateTheme = useUpdateTheme();

  const next = NEXT[preference];
  const { Icon, label } = META[preference];

  const onClick = () => {
    setPreference(next);
    updateTheme.mutate(next, {
      onError: () => {
        toast.error("Couldn't save your theme preference. It'll reset next time you sign in.");
      },
    });
  };

  return (
    <Button
      type="button"
      variant="ghost"
      size="icon"
      onClick={onClick}
      title={`Theme: ${label}`}
      aria-label={`Theme: ${label}. Switch to ${META[next].label}.`}
    >
      <Icon className="h-4 w-4" aria-hidden="true" />
    </Button>
  );
}
```

`variant="ghost"` already carries `focus-visible:ring-2 focus-visible:ring-ring` from `buttonVariants`, satisfying FR-TOGGLE-6 with no new CSS.

- [ ] **Step 4: Run tests to verify they pass**

Run: `npm run -w apps/web test -- src/components/ThemeToggle.test.tsx`
Expected: PASS (5 cases).

- [ ] **Step 5: Commit**

```bash
npm run format
git add apps/web/src/components/ThemeToggle.tsx apps/web/src/components/ThemeToggle.test.tsx
git commit -m "feat(web): add the header theme toggle"
```

---

## Task 11: `ThemeSync` and the provider tree

**Files:**
- Create: `apps/web/src/components/ThemeSync.tsx`
- Modify: `apps/web/src/components/providers/AppProviders.tsx`
- Test: `apps/web/src/components/ThemeSync.test.tsx`

**Interfaces:**
- Consumes: `useAuth`, `useTheme`, `isThemePreference`.
- Produces: `<ThemeSync />` (renders `null`); the wired provider tree.

The bridge is what keeps `AuthContext` unaware that theming exists and `ThemeContext` unaware that auth exists (design §3.3).

Required tree — `ThemeProvider` must sit **above** `<Toaster />` (FR-3P-2) and above `AppLayout`, while the authoritative preference arrives from `useMe()`, which lives *below* it:

```
QueryClientProvider
└── ThemeProvider
    ├── AuthProvider
    │   ├── ThemeSync
    │   └── children
    └── ThemedToaster
```

`ThemedToaster` is a local component because `<Toaster>` cannot read the theme context from where `AppProviders` itself renders — that call site is outside `ThemeProvider`'s subtree.

- [ ] **Step 1: Write the failing test**

`apps/web/src/components/ThemeSync.test.tsx`:

```tsx
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import { ThemeSync } from './ThemeSync';
import { ThemeProvider, useTheme } from '../context/ThemeContext';
import { THEME_STORAGE_KEY } from '../lib/theme';
import { resetMatchMedia } from '../test/setup';
import type { AuthContextValue } from '../context/AuthContext';
import type { User } from '../types/models/user';

const mockAuth = vi.fn<() => AuthContextValue>();
vi.mock('../context/AuthContext', () => ({
  useAuth: () => mockAuth(),
}));

function userWithTheme(themePreference: string): User {
  return {
    id: 'u1',
    type: 'users',
    attributes: {
      email: 'a@b.com',
      displayName: 'A',
      avatarUrl: '',
      themePreference,
    },
  } as User;
}

function baseAuth(overrides: Partial<AuthContextValue>): AuthContextValue {
  return {
    user: null,
    activeFleetId: null,
    role: null,
    isAuthenticated: false,
    isLoading: false,
    login: vi.fn(),
    logout: vi.fn(),
    ...overrides,
  };
}

function Probe() {
  const { preference } = useTheme();
  return <span data-testid="preference">{preference}</span>;
}

function renderSync() {
  return render(
    <ThemeProvider>
      <ThemeSync />
      <Probe />
    </ThemeProvider>,
  );
}

describe('ThemeSync', () => {
  beforeEach(() => {
    localStorage.clear();
    resetMatchMedia();
    document.documentElement.classList.remove('dark');
    mockAuth.mockReset();
  });

  // FR-PERSIST-3: the server is authoritative, which is what makes the
  // preference propagate to a new device on first sign-in.
  it('adopts the server preference over the cached value', () => {
    localStorage.setItem(THEME_STORAGE_KEY, 'light');
    mockAuth.mockReturnValue(baseAuth({ user: userWithTheme('dark') }));

    renderSync();

    expect(screen.getByTestId('preference')).toHaveTextContent('dark');
    expect(document.documentElement.classList.contains('dark')).toBe(true);
  });

  it('leaves the cached preference alone when signed out', () => {
    localStorage.setItem(THEME_STORAGE_KEY, 'dark');
    mockAuth.mockReturnValue(baseAuth({ user: null }));

    renderSync();

    expect(screen.getByTestId('preference')).toHaveTextContent('dark');
  });

  // FR-SEC-4 applies to the wire too: a value outside the allow-list — from an
  // older service, or a tampered response — must not reach theme state.
  it('ignores an out-of-range server value', () => {
    localStorage.setItem(THEME_STORAGE_KEY, 'light');
    mockAuth.mockReturnValue(baseAuth({ user: userWithTheme('purple') }));

    renderSync();

    expect(screen.getByTestId('preference')).toHaveTextContent('light');
  });

  it('renders nothing', () => {
    mockAuth.mockReturnValue(baseAuth({ user: null }));
    const { container } = render(
      <ThemeProvider>
        <ThemeSync />
      </ThemeProvider>,
    );
    expect(container.textContent).toBe('');
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npm run -w apps/web test -- src/components/ThemeSync.test.tsx`
Expected: FAIL — cannot resolve `./ThemeSync`.

- [ ] **Step 3: Write `ThemeSync`**

`apps/web/src/components/ThemeSync.tsx`:

```tsx
import { useEffect, useRef } from 'react';
import { useAuth } from '../context/AuthContext';
import { useTheme } from '../context/ThemeContext';
import { isThemePreference } from '../lib/theme';

/**
 * One-way bridge: server preference -> theme state. Renders nothing.
 *
 * Housed here rather than inside AuthContext so AuthContext stays unaware that
 * theming exists and ThemeContext stays unaware that auth exists (design §3.3).
 * That separation is the difference between a theme provider testable with a
 * bare render() and one needing a QueryClientProvider plus a token fixture.
 */
export function ThemeSync() {
  const { user } = useAuth();
  const { adoptServerPreference, clearLocalOverride } = useTheme();

  // Tracks whether a session has been observed, so sign-out is distinguishable
  // from "never signed in" — the latter must not clear an override the user set
  // on a pre-auth page.
  const wasSignedIn = useRef(false);

  const serverPreference = user?.attributes.themePreference;

  useEffect(() => {
    // Validated even though it comes from our own service: an older service, or
    // a tampered response, must not put an out-of-range value into theme state.
    if (!isThemePreference(serverPreference)) return;
    wasSignedIn.current = true;
    adoptServerPreference(serverPreference);
  }, [serverPreference, adoptServerPreference]);

  useEffect(() => {
    // AuthContext.logout already clears authKeys.all from the query cache; the
    // session-local override has to go too, or the next sign-in on this device
    // would ignore the server value. Per FR-PERSIST-7 the localStorage cache is
    // NOT cleared, so the login page keeps the theme the device was last using.
    if (user || !wasSignedIn.current) return;
    wasSignedIn.current = false;
    clearLocalOverride();
  }, [user, clearLocalOverride]);

  return null;
}
```

- [ ] **Step 4: Wire the provider tree**

Replace `apps/web/src/components/providers/AppProviders.tsx` in full:

```tsx
import { useState, type ReactNode } from 'react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { Toaster } from 'sonner';
import { AuthProvider } from '../../context/AuthContext';
import { ThemeProvider, useTheme } from '../../context/ThemeContext';
import { ThemeSync } from '../ThemeSync';

/**
 * sonner renders into its own portal AND computes its own colours from a
 * `theme` prop, so unlike the Radix portals it cannot inherit the token cascade
 * and needs the resolved theme passed explicitly (FR-3P-1).
 *
 * This is a separate component because <Toaster> cannot read the theme context
 * from where AppProviders itself renders it — that call site is outside
 * ThemeProvider's subtree.
 */
function ThemedToaster() {
  const { resolvedTheme } = useTheme();
  return <Toaster richColors position="top-right" theme={resolvedTheme} />;
}

/**
 * Root provider stack: React Query client, theme, auth context, and the toast
 * portal. The QueryClient is created once per app instance via useState
 * initializer.
 *
 * ThemeProvider sits ABOVE the toaster (FR-3P-2) and above the app shell, but
 * the authoritative preference arrives from useMe(), which lives BELOW it —
 * ThemeSync bridges the two without either context importing the other.
 */
export function AppProviders({ children }: { children: ReactNode }) {
  const [queryClient] = useState(
    () =>
      new QueryClient({
        defaultOptions: {
          queries: {
            retry: 1,
            refetchOnWindowFocus: false,
          },
        },
      }),
  );

  return (
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <AuthProvider>
          <ThemeSync />
          {children}
        </AuthProvider>
        <ThemedToaster />
      </ThemeProvider>
    </QueryClientProvider>
  );
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `npm run -w apps/web test`
Expected: PASS — the four `ThemeSync` cases plus the whole suite.

- [ ] **Step 6: Commit**

```bash
npm run format
git add apps/web/src/components/ThemeSync.tsx apps/web/src/components/ThemeSync.test.tsx apps/web/src/components/providers/AppProviders.tsx
git commit -m "feat(web): bridge the server theme preference into theme state"
```

---

## Task 12: Pre-paint script and the first guard test

**Files:**
- Modify: `apps/web/index.html`
- Create: `apps/web/src/test/conventions.test.ts`

**Interfaces:**
- Consumes: `THEME_STORAGE_KEY`'s literal value (`myfleet.theme`).
- Produces: `conventions.test.ts`, extended by Tasks 14 and 16.

The script duplicates `resolveTheme`'s logic and **cannot** share it — a module import in `<head>` is asynchronous by definition and would reintroduce the flash. FR-TEST-8's guard test is the mitigation: it asserts the script is still present, so a future cleanup cannot silently delete it.

Three properties matter:
- The allow-list is applied by **narrowing to a known-good default**, not by rejecting bad input — a corrupted `"purple"` and an absent key take the identical `system` path (FR-FLASH-2, FR-SEC-4).
- The class name is a literal. The stored value is never concatenated into a class, URL, or markup (FR-SEC-4).
- The whole body is in `try/catch`, so a blocked `localStorage` leaves the document in the light theme rather than failing to boot (FR-FLASH-3).

`index.html` is **not** in Prettier's globs, so its formatting is not enforced.

- [ ] **Step 1: Write the failing test**

`apps/web/src/test/conventions.test.ts`:

```ts
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, resolve } from 'node:path';
import { describe, it, expect } from 'vitest';

// src/test -> apps/web
const WEB_ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '../..');

// FR-TEST-8. The pre-paint script cannot be shared with src/lib/theme.ts — a
// module import in <head> is asynchronous by definition and would reintroduce
// exactly the flash it exists to prevent. This test is the mitigation for that
// duplication: a future cleanup that "removes the redundant inline script"
// fails here instead of silently regressing every page load.
describe('index.html pre-paint theme script', () => {
  const html = readFileSync(resolve(WEB_ROOT, 'index.html'), 'utf8');

  it('is present and reads the shared storage key', () => {
    expect(html).toContain("localStorage.getItem('myfleet.theme')");
  });

  it('applies the dark class before the module bundle loads', () => {
    expect(html).toContain("document.documentElement.classList.add('dark')");
    const scriptIndex = html.indexOf("localStorage.getItem('myfleet.theme')");
    const moduleIndex = html.indexOf('type="module"');
    expect(scriptIndex).toBeGreaterThan(-1);
    expect(moduleIndex).toBeGreaterThan(-1);
    expect(scriptIndex).toBeLessThan(moduleIndex);
  });

  // FR-FLASH-1: neither defer nor async, or it stops being pre-paint.
  it('is synchronous', () => {
    const openTag = html.slice(html.lastIndexOf('<script', html.indexOf('myfleet.theme')));
    const firstClose = openTag.slice(0, openTag.indexOf('>'));
    expect(firstClose).not.toContain('defer');
    expect(firstClose).not.toContain('async');
    expect(firstClose).not.toContain('type="module"');
  });

  // FR-FLASH-3: localStorage blocked by privacy settings must not stop the app
  // booting.
  it('is wrapped in try/catch', () => {
    const scriptStart = html.lastIndexOf('<script', html.indexOf('myfleet.theme'));
    const scriptEnd = html.indexOf('</script>', scriptStart);
    const body = html.slice(scriptStart, scriptEnd);
    expect(body).toContain('try {');
    expect(body).toContain('catch');
  });

  // FR-PERF-1: under 500 bytes.
  it('stays small', () => {
    const scriptStart = html.lastIndexOf('<script', html.indexOf('myfleet.theme'));
    const scriptEnd = html.indexOf('</script>', scriptStart);
    const body = html.slice(scriptStart, scriptEnd);
    expect(body.length).toBeLessThan(500);
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npm run -w apps/web test -- src/test/conventions.test.ts`
Expected: FAIL — `index.html` contains no `myfleet.theme`.

- [ ] **Step 3: Add the script to `index.html`**

Replace `apps/web/index.html` in full (icon/manifest links land in Task 14):

```html
<!doctype html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>MyFleet</title>
    <!--
      Pre-paint theme (FR-FLASH-1..3). Synchronous and inline on purpose: a
      module import in <head> is asynchronous by definition and would
      reintroduce the flash this exists to prevent, so it duplicates
      resolveTheme's four lines from src/lib/theme.ts. The duplication is
      guarded by src/test/conventions.test.ts.

      Note the shape: it narrows to a known-good default rather than rejecting
      bad input, so a corrupted "purple" and an absent key take the identical
      `system` path (FR-FLASH-2, FR-SEC-4). The class name is a literal — the
      stored value, which any script or extension on the origin can write, is
      never concatenated into a class, URL, or markup.
    -->
    <script>
      try {
        var p = localStorage.getItem('myfleet.theme');
        if (p !== 'light' && p !== 'dark') p = 'system';
        var d =
          p === 'dark' ||
          (p === 'system' && window.matchMedia('(prefers-color-scheme: dark)').matches);
        if (d) document.documentElement.classList.add('dark');
      } catch (e) {}
    </script>
  </head>
  <body>
    <div id="root"></div>
    <script type="module" src="/src/main.tsx"></script>
  </body>
</html>
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `npm run -w apps/web test -- src/test/conventions.test.ts`
Expected: PASS (5 cases).

- [ ] **Step 5: Commit**

```bash
git add apps/web/index.html apps/web/src/test/conventions.test.ts
git commit -m "feat(web): apply the theme before first paint"
```

---

## Task 13: Icon generator

**Files:**
- Create: `tools/generate-icons.py`
- Create: `tools/generate-icons.sh`
- Create (generated): `apps/web/public/favicon.svg`, `favicon.ico`, `apple-touch-icon.png`, `icon-192.png`, `icon-512.png`, `icon-512-maskable.png`
- Create (generated): `apps/web/src/components/brandMarkPath.ts`

**Interfaces:**
- Consumes: nothing.
- Produces: `BRAND_MARK_PATH` (exported string constant), consumed by `BrandMark` (Task 14); the committed asset set.

**Environment fact, verified:** this environment has Python 3 with Pillow 12.2 and **none** of `rsvg-convert`, `magick`, `convert`, or `inkscape`. The generator computes the chevron geometry once and emits every artefact from it, so the geometry table — not the SVG — is the single source. The SVG is itself an output.

**Deviation from design §8.2, deliberate:** the design has `generate-icons.sh` prefer a true SVG rasteriser and fall back to Python. That order is inverted here, because only the Python path can regenerate `favicon.svg` and `brandMarkPath.ts`. Preferring a rasteriser would mean that on a machine with ImageMagick installed, a geometry change would silently re-raster a *stale* SVG while leaving the SVG and the path constant stale too. Python is preferred; a rasteriser is a degraded fallback that warns loudly about what it cannot produce.

The exact values below are verified: total output 36.4 KiB (budget 100 KB), the ICO carries both 16×16 and 32×32 entries, and the emitted `d` string appears verbatim in `favicon.svg`.

- [ ] **Step 1: Write the generator**

`tools/generate-icons.py`:

```python
#!/usr/bin/env python3
"""Generate every MyFleet icon artefact from one geometry table (FR-ICON-12).

Neither CI nor apps/web/Dockerfile runs this; every output is committed. Invoke
it through tools/generate-icons.sh when the mark changes.

The geometry table below — not favicon.svg — is the single source. The SVG is
itself an output, so hand-editing it will be overwritten (and is caught by
apps/web/src/test/conventions.test.ts, which pins favicon.svg against the
generated brandMarkPath.ts constant).
"""

from __future__ import annotations

import math
import pathlib

from PIL import Image, ImageDraw

# --- geometry ---------------------------------------------------------------
# Three right-pointing filled chevrons on a shared centreline, tapering in
# scale: a fleet in motion (design §8.1).
#
# Filled polygons rather than strokes. A fill downscales predictably, whereas a
# stroke's effective weight depends on how the rasteriser resolves stroke-width
# against a scaled viewBox — which is exactly the kind of drift that makes a
# 16px favicon turn to mush.
VIEWBOX = 24.0
CENTRE_Y = 12.0
HALF_HEIGHT = 6.5  # vertical half-extent of the largest chevron
ARM_RUN = 3.0      # horizontal run of each arm (steeper than 45 degrees, which
                   # buys the horizontal room for three chevrons plus real gaps)
THICKNESS = 2.8    # uniform horizontal thickness
SCALES = (1.0, 0.82, 0.64)
GAP = 2.46         # horizontal gap between successive chevrons
LEFT = 2.4         # cluster starts at the inner 80% of the canvas

MARK_LIGHT = '#020817'  # --foreground, against light browser chrome
MARK_DARK = '#f8fafc'   # near .dark's --foreground, against dark chrome
RASTER_BG = '#ffffff'   # opaque: iOS ignores transparency and composites onto
                        # black, which would erase a dark mark (FR-ICON-6)
SUPERSAMPLE = 4
MASKABLE_SAFE_RADIUS = 0.4 * VIEWBOX  # Android crops to an arbitrary shape

ROOT = pathlib.Path(__file__).resolve().parent.parent
PUBLIC = ROOT / 'apps' / 'web' / 'public'
BRAND_TS = ROOT / 'apps' / 'web' / 'src' / 'components' / 'brandMarkPath.ts'


def chevrons() -> list[list[tuple[float, float]]]:
    """Six vertices per chevron: the right-facing V, then its inner offset."""
    polys: list[list[tuple[float, float]]] = []
    x = LEFT
    for scale in SCALES:
        h, r, t = HALF_HEIGHT * scale, ARM_RUN * scale, THICKNESS * scale
        apex = x + r + t
        polys.append([
            (apex - r, CENTRE_Y - h),
            (apex, CENTRE_Y),
            (apex - r, CENTRE_Y + h),
            (apex - r - t, CENTRE_Y + h),
            (apex - t, CENTRE_Y),
            (apex - r - t, CENTRE_Y - h),
        ])
        x = apex + GAP
    return polys


POLYGONS = chevrons()


def path_data() -> str:
    return ' '.join(
        'M' + ' L'.join(f'{x:.2f} {y:.2f}' for x, y in poly) + ' Z'
        for poly in POLYGONS
    )


def maskable_shrink() -> float:
    """Scale that pulls every vertex inside the maskable safe circle.

    Computed rather than hardcoded so a geometry change cannot silently push the
    mark outside the safe zone and get it cropped on Android.
    """
    furthest = max(
        math.hypot(x - VIEWBOX / 2, y - VIEWBOX / 2)
        for poly in POLYGONS
        for x, y in poly
    )
    return min(1.0, MASKABLE_SAFE_RADIUS / furthest)


def raster(px: int, shrink: float = 1.0) -> Image.Image:
    img = Image.new('RGB', (px * SUPERSAMPLE, px * SUPERSAMPLE), RASTER_BG)
    draw = ImageDraw.Draw(img)
    k = px * SUPERSAMPLE / VIEWBOX
    c = VIEWBOX / 2
    for poly in POLYGONS:
        draw.polygon(
            [(((x - c) * shrink + c) * k, ((y - c) * shrink + c) * k) for x, y in poly],
            fill=MARK_LIGHT,
        )
    return img.resize((px, px), Image.LANCZOS)


SVG = """<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
  <style>
    path {{ fill: {light}; }}
    @media (prefers-color-scheme: dark) {{ path {{ fill: {dark}; }} }}
  </style>
  <path d="{d}" />
</svg>
"""

TS = """// GENERATED by tools/generate-icons.py — DO NOT EDIT.
// Regenerate with: tools/generate-icons.sh
//
// The same `d` string is embedded in apps/web/public/favicon.svg;
// src/test/conventions.test.ts asserts the two have not drifted apart.
export const BRAND_MARK_PATH =
  '{d}';
"""


def main() -> None:
    PUBLIC.mkdir(parents=True, exist_ok=True)
    d = path_data()

    (PUBLIC / 'favicon.svg').write_text(SVG.format(light=MARK_LIGHT, dark=MARK_DARK, d=d))
    BRAND_TS.write_text(TS.format(d=d))

    raster(180).save(PUBLIC / 'apple-touch-icon.png')
    raster(192).save(PUBLIC / 'icon-192.png')
    raster(512).save(PUBLIC / 'icon-512.png')
    raster(512, maskable_shrink()).save(PUBLIC / 'icon-512-maskable.png')
    # Rendered at 64 so Pillow downsamples cleanly into both ICO entries.
    raster(64).save(PUBLIC / 'favicon.ico', sizes=[(16, 16), (32, 32)])

    total = sum(p.stat().st_size for p in PUBLIC.iterdir() if p.is_file())
    print(f'generate-icons: wrote {PUBLIC} ({total / 1024:.1f} KiB total)')
    if total > 100 * 1024:
        raise SystemExit('generate-icons: FAIL — assets exceed the 100 KB budget (FR-PERF-4)')


if __name__ == '__main__':
    main()
```

- [ ] **Step 2: Write the entry-point script**

`tools/generate-icons.sh`:

```sh
#!/usr/bin/env bash
# tools/generate-icons.sh — regenerate the MyFleet icon set (FR-ICON-12).
#
# Outputs are COMMITTED. Neither CI nor apps/web/Dockerfile invokes this script
# or depends on any image-processing binary being installed; it exists so the
# assets can be reproduced when the mark changes.
#
# Backend preference — python3 + Pillow FIRST, deliberately:
#
#   Only the Python generator can emit favicon.svg and brandMarkPath.ts, because
#   the chevron geometry (not the SVG) is the single source. Preferring an SVG
#   rasteriser would mean that on a machine with ImageMagick installed, a
#   geometry change silently re-rasters a STALE favicon.svg while leaving the
#   SVG and the path constant stale too. A rasteriser is therefore only a
#   degraded fallback, and it says so.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PUBLIC="$ROOT/apps/web/public"

if command -v python3 >/dev/null 2>&1 && python3 -c 'import PIL' >/dev/null 2>&1; then
    echo "generate-icons: backend = python3 + Pillow (full regeneration)"
    exec python3 "$ROOT/tools/generate-icons.py"
fi

for bin in rsvg-convert magick convert; do
    command -v "$bin" >/dev/null 2>&1 || continue

    echo "generate-icons: backend = $bin (DEGRADED — Pillow not found)" >&2
    echo "generate-icons: WARNING — favicon.svg, favicon.ico, icon-512-maskable.png and" >&2
    echo "generate-icons:           apps/web/src/components/brandMarkPath.ts are NOT" >&2
    echo "generate-icons:           regenerated on this path. If you changed the mark" >&2
    echo "generate-icons:           geometry, install Pillow and re-run." >&2

    if [ ! -f "$PUBLIC/favicon.svg" ]; then
        echo "generate-icons: ERROR — $PUBLIC/favicon.svg is missing and only Pillow can create it." >&2
        exit 1
    fi

    for spec in 180:apple-touch-icon 192:icon-192 512:icon-512; do
        size="${spec%%:*}"
        name="${spec##*:}"
        case "$bin" in
            rsvg-convert)
                rsvg-convert -w "$size" -h "$size" -b '#ffffff' \
                    "$PUBLIC/favicon.svg" -o "$PUBLIC/$name.png"
                ;;
            *)
                "$bin" -background '#ffffff' -flatten \
                    -resize "${size}x${size}" "$PUBLIC/favicon.svg" "$PUBLIC/$name.png"
                ;;
        esac
    done
    echo "generate-icons: done (partial)"
    exit 0
done

cat >&2 <<'EOF'
generate-icons: ERROR — no usable backend found.

Install ONE of the following and re-run:
  * python3 with Pillow   (preferred — regenerates every artefact)
        pip install Pillow
  * rsvg-convert          (librsvg; partial regeneration only)
  * ImageMagick           (`magick` or `convert`; partial regeneration only)
EOF
exit 1
```

- [ ] **Step 3: Make it executable and run it**

```bash
chmod +x tools/generate-icons.sh
./tools/generate-icons.sh
```

Expected output: `generate-icons: backend = python3 + Pillow (full regeneration)` followed by `generate-icons: wrote .../apps/web/public (36.4 KiB total)`.

- [ ] **Step 4: Verify the outputs**

```bash
ls -l apps/web/public/
python3 -c "from PIL import Image; print(sorted(Image.open('apps/web/public/favicon.ico').info['sizes']))"
```

Expected: six files; `[(16, 16), (32, 32)]`.

Then confirm the path constant and the SVG agree:

```bash
python3 - <<'PY'
import re
ts = open('apps/web/src/components/brandMarkPath.ts').read()
svg = open('apps/web/public/favicon.svg').read()
d = re.search(r"'([^']+)'", ts).group(1)
print('MATCH' if d in svg else 'MISMATCH')
PY
```

Expected: `MATCH`.

- [ ] **Step 5: Confirm Prettier accepts the generated TypeScript**

```bash
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
npm run format:check
```

Expected: pass. If Prettier reports `brandMarkPath.ts`, run `npm run format` and commit the reformatted file — but then re-run Step 4's MATCH check, since reformatting must not alter the string itself.

- [ ] **Step 6: Commit**

```bash
git add tools/generate-icons.py tools/generate-icons.sh apps/web/public/ apps/web/src/components/brandMarkPath.ts
git commit -m "feat(web): generate the MyFleet brand mark and icon set"
```

---

## Task 14: `BrandMark`, manifest, and `<head>` wiring

**Files:**
- Create: `apps/web/src/components/BrandMark.tsx`
- Create: `apps/web/public/site.webmanifest`
- Modify: `apps/web/index.html`
- Modify: `apps/web/src/test/conventions.test.ts`

**Interfaces:**
- Consumes: `BRAND_MARK_PATH` (Task 13).
- Produces: `<BrandMark className?: string />`, consumed by `AppLayout` (Task 15).

- [ ] **Step 1: Extend the guard test**

Append to `apps/web/src/test/conventions.test.ts`:

```ts
// The generator emits the path into BOTH brandMarkPath.ts and favicon.svg
// (design §8.2). Cheap insurance against someone hand-editing one and not the
// other, which would silently give the tab a different mark from the sidebar.
describe('brand mark', () => {
  it('is identical in brandMarkPath.ts and favicon.svg', () => {
    const ts = readFileSync(resolve(WEB_ROOT, 'src/components/brandMarkPath.ts'), 'utf8');
    const svg = readFileSync(resolve(WEB_ROOT, 'public/favicon.svg'), 'utf8');

    const match = /'([^']+)'/.exec(ts);
    if (!match) throw new Error('brandMarkPath.ts should export a single-quoted path string');
    expect(svg).toContain(match[1]);
  });
});

describe('index.html icon wiring', () => {
  const html = readFileSync(resolve(WEB_ROOT, 'index.html'), 'utf8');

  it('declares the SVG favicon with the ICO as an explicit alternate', () => {
    // rel="alternate icon" so SVG-capable browsers prefer the vector
    // (FR-ICON-5).
    expect(html).toContain('<link rel="icon" href="/favicon.svg" type="image/svg+xml" />');
    expect(html).toContain('rel="alternate icon"');
  });

  it('declares the apple-touch icon and the manifest', () => {
    expect(html).toContain('rel="apple-touch-icon"');
    expect(html).toContain('rel="manifest"');
  });

  // FR-ICON-8: the manifest format has no media-query support, so per-theme
  // browser chrome comes from these two metas instead. The values are the
  // rendered equivalents of the --background tokens; nothing enforces that
  // coupling, so it is recorded in the design's deployment notes.
  it('declares both theme-color metas', () => {
    expect(html).toContain('media="(prefers-color-scheme: light)" content="#ffffff"');
    expect(html).toContain('media="(prefers-color-scheme: dark)" content="#020817"');
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npm run -w apps/web test -- src/test/conventions.test.ts`
Expected: FAIL — the `index.html icon wiring` cases fail (the brand-mark case should already pass from Task 13).

- [ ] **Step 3: Add the `<head>` wiring**

In `apps/web/index.html`, insert immediately after the `<title>MyFleet</title>` line and before the pre-paint script comment:

```html
    <link rel="icon" href="/favicon.svg" type="image/svg+xml" />
    <link rel="alternate icon" href="/favicon.ico" sizes="16x16 32x32" />
    <link rel="apple-touch-icon" href="/apple-touch-icon.png" />
    <link rel="manifest" href="/site.webmanifest" />
    <!--
      Per-theme browser chrome. The manifest's theme_color is static — the
      format has no media-query support — so these two carry the light/dark
      split (FR-ICON-8). The values are the rendered equivalents of the
      --background tokens in src/index.css (0 0% 100% and 222.2 84% 4.9%). If
      those tokens change, these must change with them; nothing enforces it.
    -->
    <meta name="theme-color" media="(prefers-color-scheme: light)" content="#ffffff" />
    <meta name="theme-color" media="(prefers-color-scheme: dark)" content="#020817" />
```

- [ ] **Step 4: Add the manifest**

`apps/web/public/site.webmanifest`:

```json
{
  "name": "MyFleet",
  "short_name": "MyFleet",
  "start_url": "/",
  "display": "standalone",
  "background_color": "#ffffff",
  "theme_color": "#ffffff",
  "icons": [
    { "src": "/icon-192.png", "sizes": "192x192", "type": "image/png", "purpose": "any" },
    { "src": "/icon-512.png", "sizes": "512x512", "type": "image/png", "purpose": "any" },
    {
      "src": "/icon-512-maskable.png",
      "sizes": "512x512",
      "type": "image/png",
      "purpose": "maskable"
    }
  ]
}
```

- [ ] **Step 5: Write `BrandMark`**

`apps/web/src/components/BrandMark.tsx`:

```tsx
import { BRAND_MARK_PATH } from './brandMarkPath';

/**
 * The MyFleet mark, inline.
 *
 * `fill="currentColor"` means it inherits the surrounding text colour and needs
 * no dark variant (FR-ICON-9). Sizing comes from `className`; there are no
 * hardcoded pixel dimensions.
 *
 * aria-hidden because its only call site sits beside the visible "MyFleet"
 * wordmark, which already supplies the accessible name — a duplicate would make
 * screen readers announce the brand twice (FR-ICON-10). A future non-decorative
 * placement needs its own labelled wrapper.
 */
export function BrandMark({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 24 24" fill="currentColor" className={className} aria-hidden="true">
      <path d={BRAND_MARK_PATH} />
    </svg>
  );
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `npm run -w apps/web test -- src/test/conventions.test.ts`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
npm run format
git add apps/web/src/components/BrandMark.tsx apps/web/public/site.webmanifest apps/web/index.html apps/web/src/test/conventions.test.ts
git commit -m "feat(web): wire the brand mark, manifest and theme-color metas"
```

---

## Task 15: `AppLayout` — tokens, brand mark, toggle

**Files:**
- Modify: `apps/web/src/components/AppLayout.tsx`

**Interfaces:**
- Consumes: `<BrandMark />` (Task 14), `<ThemeToggle />` (Task 10), `Button`.
- Produces: nothing downstream.

Satisfies FR-CONVERT-1, FR-ICON-10, and FR-TOGGLE-1 in one file.

**Read this before choosing token classes.** FR-CONVERT-1 offers `bg-muted` *or* `bg-card` for the sidebar and `bg-accent` for the active nav link. `--muted` and `--accent` hold **identical values** in both themes (`210 40% 96.1%` light, `217.2 32.6% 17.5%` dark). A `bg-muted` sidebar with `bg-accent` active links and `hover:bg-accent` hovers would render the active item, the hover state, and the sidebar as one flat colour — the nav would look broken. The sidebar therefore uses `bg-card`. Do not "simplify" it back to `bg-muted`.

Active and hover sharing `bg-accent` is fine and is standard shadcn sidebar behaviour: only one item can be hovered, and active is persistent.

FR-TOGGLE-1 places the control "to the left of the existing display-name / Sign-out cluster". The header is `justify-between` with the display name at the far left, so the toggle goes immediately left of the Sign-out button inside a new right-hand group.

- [ ] **Step 1: Rewrite the component**

Replace `apps/web/src/components/AppLayout.tsx` in full:

```tsx
import { NavLink, Outlet } from 'react-router-dom';
import { useAuth } from '../context/AuthContext';
import { cn } from '../lib/utils';
import { BrandMark } from './BrandMark';
import { ThemeToggle } from './ThemeToggle';
import { Button } from './ui/button';

const NAV = [
  { to: '/', label: 'Dashboard', end: true },
  { to: '/vehicles', label: 'Vehicles' },
  { to: '/maintenance', label: 'Maintenance' },
  { to: '/fuel', label: 'Fuel' },
  { to: '/activity', label: 'Activity' },
  { to: '/notifications', label: 'Notifications' },
  { to: '/settings', label: 'Settings' },
];

export function AppLayout() {
  const { user, logout } = useAuth();

  return (
    <div className="flex min-h-screen">
      {/*
        bg-card, not bg-muted: --muted and --accent are the SAME value in both
        themes, so a muted sidebar would swallow the bg-accent active state and
        the hover state, flattening the nav into one colour.
      */}
      <aside className="w-56 shrink-0 border-r border-border bg-card p-4">
        <div className="mb-6 flex items-center gap-2 text-lg font-semibold">
          <BrandMark className="h-5 w-5" />
          MyFleet
        </div>
        <nav className="flex flex-col gap-1">
          {NAV.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.end}
              className={({ isActive }) =>
                cn(
                  'rounded px-3 py-2 text-sm font-medium',
                  isActive
                    ? 'bg-accent text-accent-foreground'
                    : 'text-muted-foreground hover:bg-accent hover:text-accent-foreground',
                )
              }
            >
              {item.label}
            </NavLink>
          ))}
        </nav>
      </aside>
      <div className="flex flex-1 flex-col">
        <header className="flex items-center justify-between border-b border-border px-6 py-3">
          <span className="text-sm text-muted-foreground">{user?.attributes.displayName ?? ''}</span>
          <div className="flex items-center gap-2">
            <ThemeToggle />
            <Button type="button" variant="outline" size="sm" onClick={() => void logout()}>
              Sign out
            </Button>
          </div>
        </header>
        <main className="flex-1 p-6">
          <Outlet />
        </main>
      </div>
    </div>
  );
}
```

The `outline` variant is already `border border-input bg-background hover:bg-accent hover:text-accent-foreground` with `focus-visible:ring-ring`, so the hand-rolled button classes are replaced rather than translated.

- [ ] **Step 2: Verify the layout builds and the suite is green**

```bash
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
npm run -w apps/web build
npm run -w apps/web test
```

Expected: both pass.

- [ ] **Step 3: Confirm no hardcoded palette classes remain in this file**

```bash
grep -nE '(bg|text|border|ring|divide)-(gray|slate|zinc|neutral|white|black|red|green|blue|amber|yellow|emerald|orange)' apps/web/src/components/AppLayout.tsx
```

Expected: no output.

- [ ] **Step 4: Commit**

```bash
npm run format
git add apps/web/src/components/AppLayout.tsx
git commit -m "feat(web): tokenise AppLayout and add the brand mark and theme toggle"
```

---

## Task 16: Convert the remaining hardcoded colours

**Files:**
- Modify: `apps/web/src/pages/PlaceholderPage.tsx`
- Modify: `apps/web/src/components/features/activity/ActivityEventIcon.tsx`
- Modify: `apps/web/src/components/features/vehicles/maintenance/SeverityChip.tsx`
- Modify: `apps/web/src/components/features/vehicles/maintenance/MaintenanceQueueView.tsx`
- Modify: `apps/web/src/components/features/dashboard/widgets/FleetOverviewWidget.tsx`
- Modify: `apps/web/src/components/features/dashboard/widgets/OverdueMaintenanceWidget.tsx`
- Modify: `apps/web/src/components/features/dashboard/widgets/UpcomingMaintenanceWidget.tsx`
- Modify: `packages/ui-components/src/StatusBadge.tsx`
- Modify: `apps/web/src/test/conventions.test.ts`

**Interfaces:**
- Consumes: the token classes from Task 6.
- Produces: nothing downstream.

Covers FR-CONVERT-2 … FR-CONVERT-10. The FR-CONVERT-10 grep was re-verified against this working tree: **21 matches across 9 files**, exactly the PRD's list, nothing else. Task 15 cleared six of them; the remaining fifteen are below.

Two conversions change appearance slightly and are **not** regressions:

- `FleetOverviewWidget`'s counters move from the `-600` band to `--success`/`--warning`/`--danger`, which sit at the `-700` band — slightly darker in light mode. The reason is that `text-amber-600` on white is about 3.1:1 and fails FR-A11Y-1 for body-weight text.
- `MaintenanceQueueView`'s callout moves from `bg-red-50` to `bg-danger-subtle` (the `-100` band) — a marginally stronger fill in light mode, and the only value that gives the dark theme somewhere sane to land instead of a near-white callout.

- [ ] **Step 1: Add the guard test**

Append to `apps/web/src/test/conventions.test.ts`. First widen the two existing imports at the top of that file:

```ts
import { readFileSync, readdirSync, statSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
```

`fileURLToPath` is already imported from `node:url` by Task 12. Then append:

```ts
// FR-CONVERT-10 / FR-TEST-9. Hardcoded palette classes are how dark mode rots:
// each one renders light-on-light or as an unreadable smear once the background
// goes dark, and nothing in the type system stops a new one appearing.
describe('no hardcoded palette classes', () => {
  const PALETTE =
    /(bg|text|border|ring|divide)-(gray|slate|zinc|neutral|white|black|red|green|blue|amber|yellow|emerald|orange)/;

  function tsxFiles(dir: string): string[] {
    return readdirSync(dir).flatMap((entry) => {
      const full = join(dir, entry);
      if (statSync(full).isDirectory()) return tsxFiles(full);
      return full.endsWith('.tsx') ? [full] : [];
    });
  }

  it('are absent from apps/web/src and packages/ui-components/src', () => {
    const roots = [
      resolve(WEB_ROOT, 'src'),
      resolve(WEB_ROOT, '../../packages/ui-components/src'),
    ];
    // This file necessarily contains the pattern. It is a .ts, and the scan is
    // .tsx-only, so it is out of scope by construction — the explicit skip is
    // belt-and-braces against a future rename.
    const self = fileURLToPath(import.meta.url);

    const offenders = roots
      .flatMap(tsxFiles)
      .filter((file) => file !== self)
      .flatMap((file) =>
        readFileSync(file, 'utf8')
          .split('\n')
          .map((line, index) => ({ file, line: index + 1, text: line }))
          .filter((entry) => PALETTE.test(entry.text)),
      )
      .map((entry) => `${entry.file}:${entry.line}  ${entry.text.trim()}`);

    expect(offenders, `use the semantic tokens in src/index.css instead`).toEqual([]);
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npm run -w apps/web test -- src/test/conventions.test.ts`
Expected: FAIL — the offenders array lists the fifteen remaining sites.

- [ ] **Step 3: Convert the two trivial sites**

`apps/web/src/pages/PlaceholderPage.tsx:7` — `text-gray-500` → `text-muted-foreground`:

```tsx
      <p className="mt-2 text-sm text-muted-foreground">Coming soon.</p>
```

`apps/web/src/components/features/activity/ActivityEventIcon.tsx:15` — `bg-gray-100` → `bg-muted`:

```tsx
      className="inline-flex h-8 w-8 items-center justify-center rounded-full bg-muted text-base"
```

- [ ] **Step 4: Convert `SeverityChip`**

In `apps/web/src/components/features/vehicles/maintenance/SeverityChip.tsx`, replace the comment and the `severityConfig` map (lines 10–25). The existing comment asserting these colours "have no shadcn semantic equivalent" is now false and must go (FR-CONVERT-3):

```tsx
// Status colours come from the semantic families in apps/web/src/index.css:
// urgent -> danger, recommended -> warning, informational -> info. Each chip
// also carries a text label, so colour is never the only signal (FR-A11Y-2).
const severityConfig: Record<Severity, { label: string; className: string }> = {
  urgent: {
    label: 'Urgent',
    className: 'bg-danger-subtle text-danger-subtle-foreground border-danger-border',
  },
  recommended: {
    label: 'Recommended',
    className: 'bg-warning-subtle text-warning-subtle-foreground border-warning-border',
  },
  informational: {
    label: 'Info',
    className: 'bg-info-subtle text-info-subtle-foreground border-info-border',
  },
};
```

- [ ] **Step 5: Convert `MaintenanceQueueView`**

Three sites in `apps/web/src/components/features/vehicles/maintenance/MaintenanceQueueView.tsx`:

Line 37 comment and line 38 heading:

```tsx
      {/* Overdue — the danger family; the "Overdue Maintenance" heading carries
          the meaning, colour only reinforces it (FR-A11Y-2). */}
      <Card>
        <CardContent className="pt-6">
          <h2 className="mb-4 text-base font-semibold text-danger">Overdue Maintenance</h2>
```

Line 46 — the callout block. `bg-danger-subtle` is the `-100` band rather than `bg-red-50`'s `-50`: a marginally stronger fill in light mode, and the only value that gives the dark theme somewhere sane to land instead of a near-white callout:

```tsx
                  className="flex items-center justify-between rounded-md border border-danger-border bg-danger-subtle p-3"
```

Line 74:

```tsx
          <h2 className="mb-4 text-base font-semibold text-warning">Upcoming Maintenance</h2>
```

- [ ] **Step 6: Convert the three dashboard widgets**

`FleetOverviewWidget.tsx` — replace the comment on line 27 and the three counter colours. The bare tokens sit at the `-700` band rather than `-600`, which is slightly darker in light mode and deliberate: `text-amber-600` on white is about 3.1:1 and fails FR-A11Y-1 for body-weight text.

```tsx
        {/* Status colours from the semantic families; each counter is labelled
            beneath, so colour is never the only signal (FR-A11Y-2). */}
```

```tsx
              <div className="text-2xl font-bold text-success">{data.healthy}</div>
```
```tsx
              <div className="text-2xl font-bold text-warning">{data.upcomingMaintenance}</div>
```
```tsx
              <div className="text-2xl font-bold text-danger">{data.overdue}</div>
```

`OverdueMaintenanceWidget.tsx:28`:

```tsx
        <h3 className="text-sm font-semibold mb-3 text-danger">Overdue Maintenance</h3>
```

`UpcomingMaintenanceWidget.tsx:28`:

```tsx
        <h3 className="text-sm font-semibold mb-3 text-warning">Upcoming Maintenance</h3>
```

- [ ] **Step 7: Convert `StatusBadge`**

Replace `packages/ui-components/src/StatusBadge.tsx` in full:

```tsx
export type VehicleStatus = 'Healthy' | 'Upcoming Maintenance' | 'Overdue' | 'Inactive';

// Semantic status families from apps/web/src/index.css. This package is already
// in the web app's Tailwind content globs (apps/web/tailwind.config.ts:9), so
// the classes are picked up with no config change. Each badge shows its status
// as text, so colour is never the only signal (FR-A11Y-2).
const VARIANT: Record<VehicleStatus, string> = {
  Healthy: 'bg-success-subtle text-success-subtle-foreground',
  'Upcoming Maintenance': 'bg-warning-subtle text-warning-subtle-foreground',
  Overdue: 'bg-danger-subtle text-danger-subtle-foreground',
  Inactive: 'bg-muted text-muted-foreground',
};

export function StatusBadge({ status }: { status: VehicleStatus }) {
  return (
    <span className={`inline-flex rounded px-2 py-0.5 text-xs font-medium ${VARIANT[status]}`}>
      {status}
    </span>
  );
}
```

- [ ] **Step 8: Run the guard test and the suite**

```bash
npm run -w apps/web test
grep -rE '(bg|text|border|ring|divide)-(gray|slate|zinc|neutral|white|black|red|green|blue|amber|yellow|emerald|orange)' apps/web/src packages/ui-components/src --include='*.tsx'
```

Expected: tests PASS; the grep prints nothing.

- [ ] **Step 9: Commit**

```bash
npm run format
git add apps/web/src packages/ui-components/src
git commit -m "feat(web): convert hardcoded palette colours to semantic tokens"
```

---

## Task 17: Full verification

**Files:**
- Modify: `CLAUDE.md` (only if a build command changed — it has not; skip unless something surprises you)

**Interfaces:**
- Consumes: everything.
- Produces: a branch ready for `/audit-plan`.

- [ ] **Step 1: Run the full gate**

```bash
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
make ci
```

Expected: `lint-check`, `vet`, `test`, `build`, `fe-test`, `fe-build`, `manifests` all pass. No manifest change is expected in this task, but `manifests` runs regardless.

If `lint-check` fails on formatting, run `make lint` and re-run `make ci`.

- [ ] **Step 2: Verify the container serves the assets (FR-ICON-2)**

```bash
docker build -f apps/web/Dockerfile -t myfleet-web-task003 .
docker run --rm -d -p 8099:80 --name myfleet-web-task003 myfleet-web-task003
for path in /favicon.svg /favicon.ico /apple-touch-icon.png /site.webmanifest /icon-192.png /icon-512.png /icon-512-maskable.png; do
    printf '%s -> ' "$path"
    curl -s -o /dev/null -w '%{http_code} %{content_type}\n' "http://localhost:8099$path"
done
docker stop myfleet-web-task003
```

Expected: every path returns `200` with a non-HTML content type. A `text/html` response means the request fell through to the SPA `index.html` fallback and the asset is not actually in the image — investigate before proceeding.

- [ ] **Step 3: Confirm the asset budget (FR-PERF-4)**

```bash
du -cb apps/web/public/* | tail -1
```

Expected: under `102400` bytes. Current value is roughly 37 KB.

- [ ] **Step 4: Manual visual pass**

Start the app (`npm run -w apps/web dev`, with the backend up via `make up`) and walk the FR-3P-3 and PRD §10 "Visual completeness" list in **both** themes:

- Every route: Dashboard, Vehicles, Vehicle Detail, Maintenance, Fuel, Activity, Notifications, Settings, Login, Onboarding, Invite Accept.
- Sidebar, nav links **including the active state**, header, Sign-out button.
- Severity chips (Urgent / Recommended / Info) and status badges (Healthy / Upcoming Maintenance / Overdue / Inactive) — readable *and* distinguishable from each other.
- The `MaintenanceQueueView` overdue callout renders a dark fill, not a near-white one.
- Toasts, both success and error.
- **Radix `select` dropdown content in dark mode.** Structurally it cannot misbehave — Radix portals mount into `document.body`, a descendant of the `<html>` element carrying the `dark` class, so the token cascade reaches them by construction. FR-3P-3 requires visual confirmation anyway, and `select` is the one Radix surface rendering a floating panel detached from its trigger.
- Form inputs, focus rings, skeletons, card borders.
- Toggle behaviour: the three-step cycle; OS flip while on System updates live; OS flip while on Light does nothing.
- Hard refresh on Dark shows no flash.

Record anything that looks wrong; do not fix it silently in this task.

- [ ] **Step 5: Cross-device persistence check (the point of the whole feature)**

Set Dark, sign out, then sign in from a **different browser profile**. The app must open in Dark — that proves server-side persistence rather than `localStorage`.

Then, in a fresh profile with devtools open, set `localStorage['myfleet.theme'] = 'purple'` and reload: the app must load normally in system theme. Block `localStorage` entirely (site settings → cookies/site data) and reload: the app must still boot and the toggle must still work for the session.

- [ ] **Step 6: Commit anything outstanding**

```bash
git status
```

Expected: clean. If `make lint` reformatted anything, commit it:

```bash
git add -A
git commit -m "chore(task-003): formatting after full verification"
```

---

## Coverage against the spec

| Requirement group | Tasks |
|---|---|
| FR-THEME-1…6 | 7, 8 |
| FR-FLASH-1…4 | 7, 12 |
| FR-PERSIST-1…8 | 5, 8, 9, 10, 11 |
| FR-TOGGLE-1…7 | 10, 15 |
| FR-TOKEN-1…5 | 6 |
| FR-CONVERT-1…10 | 15, 16 |
| FR-3P-1…3 | 11, 17 |
| FR-ICON-1…12 | 13, 14, 15, 17 |
| FR-DATA-1…4 | 2, 4 |
| FR-A11Y-1…4 | 6, 10, 16, 17 |
| FR-PERF-1…4 | 9, 12, 13, 17 |
| FR-SEC-1…5 | 5, 7, 9, 12; CSP recorded in design §10 |
| FR-OBS-1…2 | 5 |
| FR-TEST-1…10 | 1, 3, 5 (Go); 7, 8, 10, 11 (FE); 12, 14, 16 (guards); 17 (`make ci`) |
| API §5.1, §5.2 | 4, 5 |
| Data model §6.1–6.3 | 2, 6 |
| Branding, icon-assets.md | 13, 14, 15, 17 |

**Not implemented, and why:** `administrator.go` is untouched — `Update` already does a full `db.Save(&e)` and `ToEntity` carries the new column, so the PRD §7 line about "persistence for the updated preference" needs no code (design §3.2). The PRD's `400` status codes ship as `422` (design §3.1, restated under Global Constraints). No CSP work: none exists today, and design §10 records the note FR-SEC-5 asks for.
