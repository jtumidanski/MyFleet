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
| `.claude/skills/**/*.md` | 4389 | 5182 | **+793** |
| `backend-dev-guidelines` | — | 2471 | — |
| `frontend-dev-guidelines` | — | 2711 | — |
| `backend-guidelines-reviewer.md` | 198 | 248 | +50 |
| `frontend-guidelines-reviewer.md` | 167 | 206 | +39 |

(Agent counts are post-Gate-4; both grew again when the four drifted checks and the
four-label status vocabulary landed.)

**The skills grew by 793 lines (+18%). PRD §8 targeted net-neutral or smaller, so this
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
changes any conclusion above; every figure in this table is measured directly, not
accumulated from commit bodies, and is current as of the final Gate-4 fix.

---

## Gate 4 — reviewers dispatched against real diffs

### Targets

**Backend: task-014 `member-names-ownership-transfer`, merge `92b1290`.**

```
$ git diff --name-only 92b1290^1 92b1290 -- 'apps/*/internal/*' \
    | sed 's|\(apps/[^/]*/internal/[^/]*\)/.*|\1|' | sort -u
apps/auth-service/internal/membership
apps/auth-service/internal/user
apps/fleet-service/internal/membership
```

Chosen over task-013 (correction C3). `fleet-service/internal/membership` and
`auth-service/internal/user` are complete domain packages carrying `model.go`
through `rest.go`, so DOM-01–DOM-19 are exercised across two services. task-013's
only `model.go` is `media-service/internal/mediavariant`, which has no `resource.go`
and no `rest.go` — DOM-04, DOM-05, DOM-07, DOM-08, DOM-09, DOM-16, DOM-17 and DOM-18
would all have gone unexercised. Those are precisely the checks this task changed.

**Frontend: task-017 `app-frame-navigation`, merge `ea9e37a`** (named by PRD §10).
39 files under `apps/web/`, including the whole `components/frame/` tree,
`AppLayout.tsx`, `AdminLayout.tsx`, six `components/ui/` primitives, `index.css`
and `tailwind.config.ts`.

### Dispatch method

The two reviewers were run against **this worktree's rewritten agent definitions**,
not the registered `backend-guidelines-reviewer` / `frontend-guidelines-reviewer`
subagent types. Those resolve from the main repo's `.claude/agents/`, which still
holds the pre-rewrite checklists — dispatching them would have tested the old file
and told us nothing. Each reviewer was instead instructed to read
`.claude/agents/<name>.md` from this worktree and follow it exactly.

Both wrote to a scratch path. The historical `docs/tasks/*/audit.md` files were not
touched: they are a record of what was found at the time.

### Results

| | Backend (run 1) | Backend (re-run) | Frontend |
| --- | --- | --- | --- |
| Rows | 81 | 46 | 17 |
| PASS | 45 | 38 | 11 |
| FAIL | 5 (**4 drift**) | **1 (real)** | 0 |
| OUT-OF-SCOPE | — | 7 | 6 |
| VACUOUS | 31 | 0 | 0 |
| Phase 1 completed | yes | yes | yes |
| Anything set aside as N/A | no | no | no |

Backend run 1 failed criterion 2 (four FAILs traceable to drift). Per plan Task 29
Step 4 the named checks were fixed and the reviewer re-run; the re-run is clean.

### The four criteria

1. **Neither reviewer set any checklist item aside as inapplicable — PASS.**
   Both were searched for "N/A", "not applicable", "skipping", "does not apply".
   None present in any run.
2. **No finding traceable to guideline drift — PASS, after two rounds of fixes.**
   Run 1's four drift FAILs (DOM-01 ×2, DOM-19 ×2) are fixed in `9607dad` and
   `b1dbb43`. The one surviving FAIL is real (below).
3. **Every row cites `file:line`, PASS as well as FAIL — PASS.** This is the
   clause `DOM-08` and `FE-03` would have failed today *by passing*.
4. **Phase 1 completed for both — PASS.** Backend `make build` exit 0, `make test`
   54 packages ok / 0 FAIL. Frontend `make fe-build` exit 0, `make fe-test` 738
   tests passed across all three workspaces. The frontend gate could not run at
   all before Task 27.

### Checks fixed as a result of Gate 4

Gate 4 found four drifted checks that reading had not. This is the whole argument
for the gate existing.

| Check | Was | Now |
| --- | --- | --- |
| `DOM-01` | "`Build()` with validation" as a universal | 11 of 17 builders validate and return `(Model, error)`; 6 return a bare `Model` because they have no invariant. Both correct; FAIL narrowed to a `(Model, error)` builder that validates nothing. Tier-1 fixed in four places. |
| `DOM-19` | "`tests := []struct{...}` with `t.Run`" as a binary pass | Only 13 of 44 test-bearing packages use `t.Run`; the local table idiom is `cases := []struct` (13 sites vs 3); and `testing-guide.md:49` says "**Prefer**". The check was promoting a preference to a mandate ~70% of the codebase fails. |
| `SUB-03` | Grepped POST only | Any body-carrying route — **and** a carve-out for `/internal/` RPC routes (see below). |
| `SUB-04` | "No manual JSON parsing", zero matches | Same carve-out. |

The `SUB-03`/`SUB-04` finding is the one the re-run could not have caught on its
own, because no sub-domain package was in scope. The re-run noticed only that
`SUB-03`'s two cited examples were Domain packages. Chasing that down showed the
tree has exactly **two** genuine sub-domain packages with body-carrying routes —
`notification-service/internal/admin` and `media-service/internal/admin` — and
**both** decode a flat `PurgeRequest` with `json.NewDecoder`, which both checks
would have failed. They are correct to: `RegisterInputHandler[T]` hard-codes the
JSON:API envelope `{"data":{"attributes":T}}` (`handler.go:49-53`), and forcing it
on an internal RPC route would make service-to-service callers fake a JSON:API
document. So `SUB-03`/`SUB-04` would have failed **2 of 2** applicable packages —
the same false-universal class as `DOM-01`, and the last one hiding in the SUB
checklist.

Related: the plan's Canonical Sources table names
`apps/fleet-service/internal/vehiclemedia/` as the sub-domain example. It carries a
full `model.go`, so Phase 2 classifies it as a **Domain** package. That mislabel is
what seeded `SUB-03`'s wrong citations; Phase 2 now warns about it and names the
two real sub-domain packages.

### The one real FAIL

`DOM-13` — `apps/fleet-service/internal/membership/resource.go:181`. The handler
calls `prov.GetActiveByUserID(userID)` directly, bypassing the processor
constructed one line earlier at `:173`. Its sibling handler at `:202` does it
correctly via `proc.ListActiveMembers`. Backed by `file-responsibilities.md:123,135`
and `anti-patterns.md:13,185-196,231`; the documented exception
(`anti-patterns.md:235-247,283-287`) does not apply — same domain, no circular
dependency, no required comment. A minority pattern (4 of ~20 `resource.go` files),
so a real defect rather than architectural drift.

**Not fixed here.** This task may not modify `apps/` (Gate 3a), and a live defect
found by the checklist is evidence the checklist works.

### Observations outside the checklist

Three findings no check covers. All verified against source; all in `apps/`, so all
out of scope for this task and recorded as follow-up candidates.

1. **`auth-service/internal/user/resource.go:18-21,136-138,184-186,235-236`** justify
   substituting `errInternal` by claiming `WriteError` copies `err.Error()` into the
   title. It does not: `jsonapi.go:95-112` sets `Title: InternalErrorTitle`
   unconditionally and only overwrites it when `status < 500`. Harmless to clients,
   but it hands the shared error logger the string "internal server error" instead of
   the actual fault.
2. **`auth-service/internal/membership/client.go:27`** concatenates `userID` into a URL
   unescaped, while `client.go:80-82` one screen below explicitly names "Active's
   raw-concatenation habit" as something not to inherit. Not currently exploitable —
   the caller passes a server-generated UUID.
3. **`fleet-service/internal/membership/entity.go:88-96`** omits four columns from
   `ToEntity()`, safe only because the administrator avoids `db.Save` by convention
   (`administrator.go:66-72`). The parallel case in `auth-service/internal/user` is
   type-guarded with `gorm:"<-:create"`.

### Method note

Run 1 was told not to set anything aside, which overrode the agent file's own Phase 2
instruction to skip Support packages — so it forced the full DOM checklist onto
`auth-service/internal/membership` (a plain HTTP client: no `model.go`, no
`resource.go`) and manufactured 20 meaningless rows. That was a defect in the dispatch
prompt, not in the checklist. The re-run applies Phase 2 as written and emits zero rows
for that package. The `OUT-OF-SCOPE` label added to both agent files is what lets a
reviewer honour both instructions at once.

