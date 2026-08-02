# Task 010 — Implementation Context

Companion to [`plan.md`](./plan.md). Everything here was verified against source
in this worktree (branched from `main` @ `e9a76e6`), not recalled.

---

## 1. Key files, as they stand today

### Backend — `apps/auth-service`

| File | What matters |
| --- | --- |
| `internal/oidc/resource.go` | 216 lines. `Dependencies` at `:29-43`; `callbackHandler` at `:65-147`; the nine `http.Error` exits at `:70, 75, 84, 90, 97, 104, 111, 123, 129`; signed-state-cookie helpers (`setStateCookie`, `verifyStateCookie`, `clearStateCookie`, `sign`) at `:152-212`. `stateCookieName = "oidc_state"`, `stateTTL = 10 * time.Minute`. |
| `internal/oidc/processor.go` | `*Processor` methods the new `Authenticator` interface mirrors: `AuthCodeURL(state, nonce string) string` (`:42`), `Exchange(ctx, code) (string, error)` (`:47`), `Verify(ctx, rawIDToken) (user.GoogleProfile, string, error)` (`:63`). |
| `internal/oidc/processor_test.go` | The only existing test in the package. Package `oidc` (internal), plain `testing`, no fixtures — the new `resource_test.go` joins the same package so the cookie helpers are reachable. |
| `internal/user/processor.go:33` | `ProvisionFromGoogle(gp GoogleProfile) (Model, error)` — returns a **value**, not a pointer. |
| `internal/user/builder.go` | `NewBuilder().SetGoogleSub(_).SetEmail(_).Build() Model` — how the test fixture makes a `user.Model` without a database. |
| `internal/user/model.go:26-31, :48-53` | `Model.ID()`, `.Email()`; `GoogleProfile{Sub, Email, Name, Avatar}`. |
| `internal/session/processor.go:59, :77` | `MintAccess(pr Principal) (string, error)`, `IssueRefresh(userID string) (string, error)`. |
| `internal/session/resource.go:18` | `type MembershipResolver func(ctx context.Context, userID string) (fleetID, role string, err error)` — already an injected func type, so `Resolve` is fakeable without change. |
| `cmd/main.go:72-82` | The `oidc.Dependencies` composite literal. Assigns `oidcProc`, `users`, `sess` **by field name**, so it compiles unchanged once those fields become interfaces. `LoginPath` is added here. |

### Frontend — `apps/web`

| File | What matters |
| --- | --- |
| `src/pages/LoginPage.tsx` | 36 lines. Card + `bg-muted` + `variant="outline"` button wired straight to `onClick={login}`. The `useEffect` auth bounce at `:17-19` is preserved verbatim. |
| `src/context/AuthContext.tsx:21, :47-50` | `login: () => void` takes **no argument on `main` today**; it sets `window.location.href = '/api/auth/login/google'` — a full navigation, which is why the redirecting state is one-way. |
| `src/lib/api/token.ts:26-37` | `captureTokenFromHash()` — returns early unless the hash contains `access_token=`, then strips via `history.replaceState(null, '', pathname + search)`. The new `loginError.ts` mirrors this shape exactly. |
| `src/context/ThemeContext.tsx` | `useTheme()` → `{ preference, resolvedTheme, setPreference, adoptServerPreference, clearLocalOverride }`. Issues no network request and knows nothing about auth — the boundary FR-PRETOGGLE-5 protects. |
| `src/components/ThemeToggle.tsx` | 61 lines. `NEXT` and `META` maps, the aria contract, and `<Button variant="ghost" size="icon">` all move to `ThemeToggleButton` unchanged. |
| `src/components/ThemeToggle.test.tsx` | **Not edited.** Asserts `getByRole('button')`, `aria-label`, `title`, and the `lucide-monitor` class — none of which move, so it passes against the reduced wrapper. |
| `src/components/BrandMark.tsx` | The precedent the Google mark deliberately departs from: `fill="currentColor"`, `aria-hidden`, sizing from `className`. |
| `src/components/ui/button.tsx` | `variant` default is `bg-primary text-primary-foreground`; base class already carries `gap-2`, `disabled:pointer-events-none disabled:opacity-50`, and the `focus-visible:ring-ring` focus ring. So the solid button and the disabled state need no extra classes. |
| `src/components/ui/card.tsx:7` | `Card` renders `rounded-lg border bg-card text-card-foreground shadow-sm` — `.bg-card` is the marker the "no card" assertion queries for. |
| `src/index.css:41-44, :86-89` | `--danger-subtle`, `--danger-subtle-foreground`, `--danger-border` defined for both themes; `tailwind.config` maps them to `bg-danger-subtle` / `text-danger-subtle-foreground` / `border-danger-border` at `:63-68`. |
| `src/components/features/vehicles/maintenance/MaintenanceQueueView.tsx:47-60` | The established callout treatment the login page reuses. |
| `src/test/conventions.test.ts:113-154` | The palette-class scanner. Walks every `.tsx` under `apps/web/src` and `packages/ui-components/src`. |
| `src/test/setup.ts` | Provides an in-memory `localStorage`, a driveable `matchMedia`, and exports `setPrefersDark` / `resetMatchMedia`. |
| `src/components/AppLayout.test.tsx:9-33` | The `vi.mock('../context/AuthContext')` + `baseAuth(overrides)` pattern `LoginPage.test.tsx` copies. |

---

## 2. Decisions inherited from design.md

| Decision | Where | Consequence for the implementer |
| --- | --- | --- |
| **Decision B** — narrow three `Dependencies` fields to consumer-declared interfaces | §3.2 | The only way all nine exits become testable. `make build` is the proof the concrete types still satisfy them. |
| One `failLogin` helper, typed code constants | §3.1 | The closed set is enforced by the compiler, not by review. |
| `clearStateCookie` before `http.Redirect` | §3.1 | `http.Redirect` calls `WriteHeader`; headers set afterwards are dropped silently. |
| No `url.QueryEscape` on the redirect target | §3.1 | Escaping would turn `#` into `%23` and put the code in the path. Safe because the code is a constant (FR-ERR-9). |
| Double clear on late exits is fine | §3.1 | Two identical `MaxAge:-1` `Set-Cookie` headers are harmless; the test looks for *any* deleting cookie rather than counting. |
| `access_denied` → `cancelled`, every other `?error=` → `auth_failed` | §3.3, §9 | **Deviates from FR-ERR-6**, deliberately. Telling a user "cancelled" when the OAuth client is misconfigured hides a real fault. |
| Provider-error logged at **Info**, not Error | §3.3 | A cancel must not inflate the error rate, but a spike in `access_denied` is a UX signal worth keeping. |
| `consumeLoginError` memoises at **module scope** | §4.1 | Not in `useState`, not in `sessionStorage`. StrictMode's mount→unmount→remount would otherwise swallow the notice in dev and not in prod. Drives the `vi.resetModules()` test pattern in both frontend test files. |
| Raw fragment value discarded at the parser | §4.1 | Nothing downstream can render attacker-supplied text (FR-STATE-6). |
| One shared failure sentence for three codes | §4.2 | Resolves open questions 1 and 3. The lookup table is still keyed on all four, so diverging one later is one line. |
| Keep the footer | §4.5 | Resolves open question 2. It is the only sentence on the page that says what MyFleet does. |
| Theme control identical to the header one | §4.3 | Resolves open question 4. Differentiation is placement alone (absolute, top-right). |
| No `LoginNotice` component | §4.5, §6 | One call site, no state of its own — indirection without isolation. |

---

## 3. Dependency order

```
Task 1 (interfaces + test harness)
  └─ Task 2 (LoginPath, failLogin, nine exits, main.go wiring)
       └─ Task 3 (provider-error pre-check)

Task 4 (loginError.ts)  ─┐
Task 5 (ThemeToggleButton) ─┴─ Task 6 (GoogleMark + LoginPage)

Tasks 1-6 ─── Task 7 (make ci, overlays, manual, review)
```

Tasks 1→2→3 are strictly sequential (each builds on the previous task's test
harness). Tasks 4 and 5 are independent of each other and of the backend chain;
Task 6 needs both. Task 7 needs everything.

---

## 4. Verification commands

```sh
# Node is not always on PATH
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22

# Go — one package, then everything
go test github.com/jtumidanski/myfleet/apps/auth-service/internal/oidc -v
make vet && make build && make test

# Frontend — one file, then everything
npm run -w apps/web test -- src/lib/auth/loginError.test.ts
npm run -w apps/web test

# The whole gate
make ci        # lint-check vet test build fe-test fe-build manifests carfax-template

# PRD acceptance criterion
grep -c "http.Error" apps/auth-service/internal/oidc/resource.go   # expect 0

# Manifests (unchanged by this task, re-verified per CLAUDE.md)
kustomize build deploy/k8s/overlays/main  | kubectl apply --dry-run=server -f -
kustomize build deploy/k8s/overlays/local | kubectl apply --dry-run=server -f -
```

`make lint` fixes what `make lint-check` reports.

---

## 5. Risks and traps

| Trap | Why it bites | Guard |
| --- | --- | --- |
| **`vi.resetModules()` after the dynamic import** | The memoised `captured` survives, and every `#error=` case after the first sees `null`. Symptom: `loginError.test.ts` passes but `LoginPage.test.tsx`'s notice assertions fail. | Reset *before* `await import(...)` in both test helpers. Called out inline at Task 6 Step 5. |
| **Headers after `http.Redirect`** | `WriteHeader` has already fired; a `Set-Cookie` added afterwards vanishes with no error. | `failLogin` clears the cookie on its first line. |
| **Query-escaping the redirect target** | `#` becomes `%23`; the code lands in the path and the SPA never sees a fragment. | Documented in `failLogin`'s doc comment. |
| **`bg-white` for the Google mark's backing** | `conventions.test.ts` fails the build on any hardcoded palette class in a `.tsx`. | In-SVG `<circle fill="#FFFFFF">`, with the rationale in `GoogleMark.tsx`'s doc comment so review does not read it as a violation. |
| **Editing `ThemeToggle.test.tsx`** | It is an explicit PRD acceptance criterion that it passes unmodified — it is the proof the extraction changed no behaviour. | The extraction keeps the rendered DOM byte-identical. |
| **Recolouring the Google mark to `currentColor`** | Matches every other icon in the app, and violates Google's branding terms. | Fixed hex fills, comment explaining why. |
| **`Dependencies` collides with the in-flight `return_to` branch** | Both branches edit the struct and all four of `LoginPage.tsx`, `LoginPage.test.tsx`, `resource.go`, `resource_test.go`. | PRD §7.4 / design §7: rebase before the PR, take both field sets, merge the test case lists. The `onClick` wrapper in `LoginPage.tsx` is the designated seam for `login(from)`. |
| **`logrus/hooks/test` looks like a new dependency** | It is not — it ships inside `github.com/sirupsen/logrus v1.9.4`, already in `apps/auth-service/go.mod:10`. | No `go get`, no `go.mod` change. If `go.mod` changes, something is wrong. |
| **Only dry-running the `main` overlay** | A missing `namespace:` in the local overlay once slipped past ten reviews for exactly this reason (CLAUDE.md). | Task 7 Step 3 runs both. |

---

## 6. Out of scope — do not build

- The `return_to` / post-login return-path work (PRD non-goal, §7.4).
- Any visual change to `/onboarding` or `/invites/:token/accept`.
- Any change to the `--destructive` token or the semantic status palette.
- Any change to token minting, refresh rotation, cookie handling, or the HMAC state scheme.
- A non-Google identity provider or a "sign in with email" affordance.
- `deploy/` manifests — `APP_LOGIN_PATH` follows `APP_HOME_PATH` and `APP_ONBOARDING_PATH`, which rely on their Go defaults and appear in no manifest.
- Error details, correlation ids, or any fifth code on the wire.
- Retry/backoff or automatic re-attempt. The button is the retry.
