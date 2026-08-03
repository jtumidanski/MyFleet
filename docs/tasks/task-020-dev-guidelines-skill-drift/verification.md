# task-020 — Verification Record

Evidence for Gates 1–4. Run on branch `task-020-dev-guidelines-skill-drift`.

---

## Gate 1 — mechanical greps

```
$ bash docs/tasks/task-020-dev-guidelines-skill-drift/drift-gate.sh; echo "exit=$?"
PASS  G-01     api2go
PASS  G-02     ServerInformation
PASS  G-03     MarshalResponse
PASS  G-04     model.* composition lib
PASS  G-05     EntityProvider
PASS  G-06     RouteInitializer
PASS  G-07     RegisterHandler(
PASS  G-08     services/<svc>-service
PASS  G-09     Jest (not jest-dom)
PASS  G-10     prior-project vocabulary
PASS  G-11     RestModel / GetName()
PASS  G-12     @/ path alias
PASS  G-13     __mocks__/watchAll/mux
PASS  G-14     uuid.UUID entity ids
PASS  G-15     uint32 entity ids
PASS  G-16     gorilla mux idiom
PASS  G-17     fake handler deps
PASS  G-18     testify in Go examples
PASS  G-19     mock-struct convention
PASS  G-20     frontend/ root path
PASS  G-21     dead FE paths
PASS  G-22     unset tsconfig flags
PASS  G-23     nginx.conf/bruno

drift-gate: ALL CHECKS PASS
exit=0
```

**23/23 PASS.** At baseline (`drift-baseline.txt`) all 23 failed.

### Gate amendments made during execution

Four commits amended the gate itself. Each is a scoping fix, not a weakening:

| Commit | Change | Reason |
| --- | --- | --- |
| `36c9baa` | Anchor G-06 | It flagged the real `AddRouteInitializer`, which is not drift |
| `e3102ad`, `212edae` | `ALLOW-VOCAB` opt-out on G-14/G-15, made per-check | A backend guideline legitimately names `uuid.UUID`/`uint32` when saying *not* to use them |
| `bec653f` | Narrow G-23 | Deliberately given no escape hatch — see the comment in the script |
| `d3e42b3` | `ALLOW-VOCAB:G-21` opt-out | The rewritten docs deny dead paths by name (“there is no `types/api/`”), which is the opposite of drift. The gate comment restricts the marker to denials only. |

Three lines carry a marker today: two in `patterns-provider.md` (G-14, G-15), one in
`patterns-react-query.md` (G-10, the English word “policy”), and four G-21 denials
across `SKILL.md` and `ai-guidance.md`.

---

## Gate 2 — per-identifier existence

### Backend

26 distinct `pkg.Symbol` identifiers extracted from every ```go block in the backend skill.
Six did not resolve to a first-party declaration:

| Identifier | Disposition |
| --- | --- |
| `chi.URLParam` | Third-party; 67 real uses under `apps/` |
| `gorm.ErrRecordNotFound` | Third-party; 31 real uses |
| `gorm.Open` | Third-party; 34 real uses |
| `logrus.FieldLogger` | Third-party; 104 real uses |
| `logrus.StandardLogger` | Third-party; 2 real uses — and cited in the guideline as the thing *not* to do |
| `server.ErrPreconditionFailed` | **Correctly absent.** It appears only in `ai-guidance.md:39-47` as the worked example of assuming a sentinel exists without checking, whose documented answer is “it doesn't exist in this tree.” The gate flagging it confirms the example is honest. |

**Zero misses among first-party `server.*`, `database.*`, `entityguard.*`, `auth.*`, `telemetry.*`.**

### Frontend

Every symbol named in the frontend skill resolves in `apps/web/src`, `packages/shared-ts/src`
or `packages/ui-components/src`: `cn`, `apiClient`, `ApiClient`, `createErrorFromUnknown`,
`ApiError`, `BaseService`, `ListResult`, `JsonApiResource`, `JsonApiDocument`, `PageMeta`,
`renderWithProviders`, `createTestQueryClient`, `vehicleKeys`, `useVehicles`, `useVehicle`,
`useCreateVehicle`, `vehicleService`, `PageHeader`, `AppRoutes`, `RequireAuth`,
`RequirePlatformAdmin`, `AppLayout`, `AdminLayout`, `FLEETLESS_ROUTES`, `TRAILS`,
`isThemePreference`, `useTheme`, `ThemeSync`, `MileageForm`, `LogMileageDialog`,
`StatusBadge`, `VehicleList`, `zodResolver`. **Zero unverified.**

---

## Gate 3a — no source was touched

```
$ git diff --name-only main...HEAD | grep -E '^(apps|packages|deploy|\.github)/'
scope clean
$ git diff --name-only main...HEAD | grep -E '^\.claude/settings'
settings untouched
```

33 files changed, all under `.claude/skills/**`, `.claude/agents/{backend,frontend}-guidelines-reviewer.md`
and `docs/tasks/task-020-dev-guidelines-skill-drift/**`.

---

## Gate 3b — `make ci`

```
$ make ci
…
54 Go packages ok
✓ built in 4.03s
manifest checks passed
carfax template check passed
exit=0
```

Expected, since no source changed. Note: `npm ci` had to be run once in this worktree —
`node_modules` is not shared between worktrees, and `make fe-build` fails with three
`TS2307: Cannot find module '@radix-ui/…'` errors until it is. Worth knowing before
concluding a fresh worktree has a real build break.

---

## Gate 3c — every documented path resolves

Two survivors, both legitimate:

| Path | Reason |
| --- | --- |
| `docs/audits/` | An **output** path the reviewers write to when invoked standalone, not an input that must pre-exist |
| `docs/audits/frontend/audit.md` | Same |

---

## Gate 3d — skill activation

**Name / directory / key all agree:**

```
name: backend-dev-guidelines      (.claude/skills/backend-dev-guidelines/SKILL.md)
name: frontend-dev-guidelines     (.claude/skills/frontend-dev-guidelines/SKILL.md)
skill-rules keys: ['backend-dev-guidelines', 'frontend-dev-guidelines']
directories:      backend-dev-guidelines, frontend-dev-guidelines
```

**The skill loads (PRD FR-5.2).** `Skill("backend-dev-guidelines")` was invoked and
returned content — the rename did not break activation. Task 11 does not need reverting.

**Two limitations this gate cannot clear from inside the worktree, recorded rather than
papered over:**

1. **The skill resolved against the main repo, not this worktree.** The loaded content was
   the pre-rewrite version. Main is still at `name: golang-microservice` inside a directory
   called `backend-dev-guidelines/` — and the call *still* succeeded, which is the direct
   explanation for how the mismatch survived unnoticed: the harness addresses a skill by its
   **directory name**, so a wrong `name:` frontmatter is invisible at the call site. The
   rewritten content only takes effect once this branch merges.
2. **The file-trigger hook could not be exercised here** for the same reason: the
   `skill-activation-prompt` hook reads main's `skill-rules.json`, where
   `backend-dev-guidelines.fileTriggers.pathPatterns` is still `[]`. The trigger data
   committed in Task 25 was validated structurally instead — valid JSON, three backend
   patterns matching real files (`apps/fleet-service/internal/vehicle/*.go`,
   `packages/shared-go/server/*.go`, `packages/dto-go/`), five frontend patterns with no
   dead `frontend/**` entries. **Confirm the hook fires on a Go file change after merge.**

---

## Token budget (PRD §8)

| | Baseline | Now | Delta |
| --- | --- | --- | --- |
| `.claude/skills/**/*.md` | 4389 | 5170 | **+781** |
| `backend-dev-guidelines` | — | 2459 | — |
| `frontend-dev-guidelines` | — | 2711 | — |
| `backend-guidelines-reviewer.md` | 198 | 230 | +32 |
| `frontend-guidelines-reviewer.md` | 167 | 191 | +24 |

**The skills grew by 781 lines (+18%). PRD §8 targeted net-neutral or smaller, so this
missed its target.** Stated plainly rather than trimmed, because the content that grew is
the content Gate 2 depends on:

- The largest deletions did happen — `patterns-rest-jsonapi.md` 444→222, backend
  `ai-guidance.md` 277→214, backend `testing-guide.md` 308→214 — removing api2go,
  `MarshalResponse`, testify and the mock-directory convention.
- They were outweighed by the files that were near-empty stubs describing a different
  codebase: `patterns-provider.md` 39→166, backend `architecture-overview.md` 57→113,
  `patterns-functional.md` 59→111, frontend `architecture-overview.md` 104→272,
  `patterns-routing.md` 121→213.
- A rule stated without its `file:line` and its worked example is exactly the drift this
  task removed. Trimming back to the baseline line count would reintroduce it.

The running total recorded in commit bodies has two bookkeeping errors, corrected here
against actual file sizes: the frontend `SKILL.md` commit recorded 152 lines (actual 148,
delta +7 not +11), and the backend reviewer commit recorded 231 (actual 230). Neither
changes any conclusion above; the table is measured, not accumulated.

---

## Gate 4 — reviewers dispatched against real diffs

See below (appended by Task 29).
