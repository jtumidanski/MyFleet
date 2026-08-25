# Go 1.27 Toolchain Migration — Design

Version: v1
Status: Approved for planning
Created: 2026-08-25
PRD: [prd.md](prd.md)
Probe baseline: [probe-results.md](probe-results.md)
---

## 1. What this design is actually about

There is no interesting code architecture in this task. Nothing about the
domain model, the transport layer, or the service boundaries changes. What
needs designing is the **ordering and atomicity** of a change that touches four
independent version-declaring surfaces which currently disagree with each other,
plus the **mechanism** that stops them disagreeing again.

Three questions carry all the design weight:

1. **Ordering.** The pinned linter cannot type-check the Go 1.27 standard
   library. That makes the lint pin a *prerequisite* of the toolchain bump, not
   a companion to it. Get the order wrong and the branch has a commit where
   `make ci` is red by construction.
2. **Blast radius of the newer linter.** A newer golangci-lint vendors a newer
   staticcheck and a newer gofumpt. The PRD's FR-12 forbids suppressing new
   findings, so the size of the fix list determines whether this task is a
   half-hour change or needs a scope negotiation.
3. **The anti-drift mechanism.** FR-18 asks Renovate to group the Go toolchain
   surfaces. Renovate does not see two of the three surfaces by default. The
   design has to specify the mechanism, not gesture at it.

Everything else — seven `go` directives, four `FROM` lines, three `go-version`
inputs, three documentation lines — is mechanical find-and-replace whose only
risk is being incomplete. Section 7 pins that down with an exhaustive inventory
so "incomplete" is checkable rather than hoped for.

## 2. Open questions from the PRD — all five resolved empirically

The PRD deferred five questions to this phase. All five were answered by running
the thing rather than reasoning about it. The probes ran in this worktree
against a real `go1.27.0` toolchain and were reverted before this document was
written (`git status` clean).

### OQ-1 — Does golangci-lint v2.13.1 stay clean across all six modules?

**Yes, after three whitespace fixes.** This was the question that could have
blown up the task, and it did not.

Probe: bumped all seven `go` directives to `1.27.0` and
`GOLANGCI_LINT_VERSION` to `v2.13.1`, then ran `tools/lint.sh --check --go`
tree-wide under `go1.27.0`.

Result — 3 findings across 6 modules, all `gofumpt`, zero from any analyzer:

| Module | Findings |
|---|---|
| `apps/auth-service` | 0 |
| `apps/fleet-service` | 2 (`internal/fuel/builder.go:19`, `internal/maintenancerecord/builder.go:26`) |
| `apps/media-service` | 1 (`internal/processing/worker_test.go:39`) |
| `apps/notification-service` | 0 |
| `packages/dto-go` | 0 |
| `packages/shared-go` | 0 |

The finding is a single newer-gofumpt rule: a run of single-line function
bodies whose alignment column differs from the declaration that follows must be
separated by a blank line. The fix is a blank line plus the re-alignment that
follows from it. Illustrative diff (`apps/fleet-service/internal/fuel/builder.go`):

```diff
-func (b *Builder) SetPricePerGallon(price float64) *Builder  { b.m.pricePerGallon = price; return b }
+func (b *Builder) SetPricePerGallon(price float64) *Builder { b.m.pricePerGallon = price; return b }
+
 func (b *Builder) SetCreatedByUserID(userID string) *Builder { b.m.createdByUserID = userID; return b }
```

`tools/lint.sh --go` (fix mode) resolves all three automatically; the
subsequent `--check` run returns `lint.sh: OK`, `0 issues.` per module. Verified.

**Consequence for the design.** FR-12's "fix, don't suppress" policy holds
without strain and needs no carve-out. The three edits are pure whitespace —
no identifier, expression, or statement changes — which keeps the PRD's
scope-discipline criterion ("no `.go` file modified except where required")
trivially auditable: `git diff -w` on those three files must be empty.

**Anti-goal recorded here so the plan does not drift into it:** the alignment
churn in `fuel/builder.go` will tempt a reviewer toward "while we're here, let's
restructure these builders." Do not. Seven lines of whitespace, nothing else.

### OQ-2 — Introduce `.go-version` / `mise` / `asdf`?

**No. Confirmed out of scope**, and the design actively prefers keeping it out.

The PRD assumed this and the assumption survives scrutiny. Adding
`.go-version` would introduce a *fifth* place a Go version is written, and its
value would be authoritative only for contributors who happen to run a version
manager that reads it. It solves the local-vs-CI divergence that motivated this
task only for people who already have the tooling. Meanwhile it adds a surface
Renovate must also be taught about (see OQ-5) for a benefit this task does not
need. The right time to introduce it is if and when the repo adopts a version
manager as a documented prerequisite — its own task, with its own README change.

### OQ-3 — `1.27-alpine` or `1.27.0-alpine`?

**`1.27-alpine`** — keep the existing floating-minor convention.

The tradeoff the PRD flagged is real: the floating tag means Renovate sees only
minor bumps, not patch ones. But that is the *desired* behaviour here. Go patch
releases are security and bugfix rollups that the repo wants picked up on the
next image rebuild without a PR; a pinned `1.27.0-alpine` would convert every Go
patch release into a Renovate PR gated behind `minimumReleaseAge: 7 days`,
delaying security patches to get a changelog entry nobody reads. The floating
tag also keeps the diff to one character per Dockerfile.

Registry facts confirmed at probe time: `golang:1.27-alpine` exists and resolves
to the `alpine3.24` variant, matching the runtime stage exactly.

### OQ-4 — Does the CI Go cache key need a manual bust?

**No.** The keys do not embed the Go version, and they do not need to.

Actual keys, read from source:

- `.github/workflows/pr.yml:26` and `.github/workflows/main.yml:65` —
  `go-build-${{ runner.os }}-${{ hashFiles('**/go.sum', 'go.work.sum') }}`
- `.github/workflows/pr.yml:63` —
  `go-lint-${{ runner.os }}-${{ hashFiles('**/go.sum', 'go.work.sum') }}`
- `.github/workflows/pr.yml:69` —
  `lint-tools-${{ runner.os }}-${{ hashFiles('tools/lint.versions') }}`

The PRD's stated worry — "stale 1.26 build objects could be restored into a 1.27
job" — is not a correctness hazard. Go's build cache is content-addressed on a
key that includes the toolchain's build ID, so 1.26 entries restored into a 1.27
job are inert misses, never wrongly-reused artifacts. `GOMODCACHE` is
toolchain-independent entirely. The only cost is that one restored archive
carries dead weight until Go's own cache trimming ages it out.

Two things happen for free as a consequence of the change itself:

- `go.work.sum` churn (see §4) changes `hashFiles(...)`, so `go-build-*` and
  `go-lint-*` miss once and fall back through `restore-keys` to a warm-ish
  prefix match. This is exactly the one-run slowdown FR-10 anticipates.
- `lint-tools-*` keys on `hashFiles('tools/lint.versions')`, which this change
  edits. The lint tooling cache therefore self-busts, which is precisely what
  is wanted — no stale `golangci-lint-v2.12.2` binary survives the pin bump.

**Rejected alternative:** adding `${{ env.GO_VERSION }}` to the two build-cache
keys. It would be marginally more correct in the "no dead weight" sense, but it
requires touching three keys across two files, and the two `go-build-*` keys
share a prefix *deliberately* — `main.yml:60-63` documents that a PR run should
restore what main last built. Introducing a version segment risks desynchronising
that intent for no correctness gain. FR-10 already says no key change; this
design agrees, now with a reason rather than an assumption.

### OQ-5 — What mechanism does Renovate actually need for FR-18?

This is the one place the PRD's phrasing ("gains a package rule") is
insufficient, and the design departs from it. **`packageRules` alone cannot do
this.** Three things are true:

1. Renovate's `github-actions` manager parses `uses:` action references and
   `container:`/`services:` images. It does **not** parse arbitrary action
   inputs, so `go-version: '1.27'` in `setup-go` is invisible to it. A
   `packageRules` entry cannot group a dependency Renovate never extracted.
2. Making it visible requires a `customManagers` regex manager.
3. This repo sets an explicit `enabledManagers` allowlist
   (`renovate.json:10-15`). Custom managers are **disabled** unless
   `"custom.regex"` is added to that list. Omitting it is a silent no-op — the
   config validates, Renovate runs, and nothing is ever grouped. This is the
   single most likely way to implement FR-18 and have it quietly not work.

Proposed shape (validated — see below):

```jsonc
"enabledManagers": ["gomod", "npm", "dockerfile", "github-actions", "custom.regex"],

"customManagers": [
  {
    "customType": "regex",
    "description": "actions/setup-go's go-version input is a plain YAML value the github-actions manager does not extract.",
    "managerFilePatterns": ["/^\\.github/workflows/[^/]+\\.ya?ml$/"],
    "matchStrings": ["go-version:\\s*['\"](?<currentValue>[0-9.]+)['\"]"],
    "datasourceTemplate": "golang-version",
    "depNameTemplate": "go",
    "versioningTemplate": "docker"
  }
]
```

and, appended **after** the four existing grouping rules so it wins the
last-rule-wins merge:

```jsonc
{
  "description": "Go toolchain surfaces — builder image, CI input, and the go.mod directive — move as one PR so they cannot drift apart the way 1.25/1.26 did.",
  "matchDatasources": ["golang-version", "docker"],
  "matchPackageNames": ["go", "golang"],
  "groupName": "Go toolchain"
}
```

Design notes on this shape:

- **`managerFilePatterns`, not `fileMatch`.** `fileMatch` is the deprecated
  spelling; current Renovate wants `managerFilePatterns`.
- **The regex is anchored on `go-version:` followed by a quoted value.** The
  literal string `go-version` also appears in a prose comment at
  `.github/workflows/main.yml:46` ("its key from OS/arch/go-version/file-hash
  only"). That occurrence has no `:`-then-quote following it, so it does not
  match. This was checked, not assumed — but it is exactly the kind of thing
  that a lazier regex would break on, so the plan must assert it.
- **The rule catches a third surface for free.** Renovate's `gomod` manager
  *does* extract the `go` directive as a `golang-version` dependency named `go`.
  So the same grouping rule sweeps the `go.mod`/`go.work` directive into the
  same PR as the builder image and the CI input. That is a bonus, and it is why
  `matchPackageNames` includes `go` as well as `golang`.
- **No `automerge`, no `minimumReleaseAge` override.** Both inherit from the
  top-level config, satisfying FR-18's constraint. The existing "Separate major
  upgrades" rule sets only `automerge`/`labels` and no `groupName`, so it
  composes with this rule rather than fighting it.

**Validation performed:** the proposed config passes
`renovate-config-validator --strict`. Caveat carried into the plan: the
validator reports "Validating as global config", which is a looser schema check
than repo-config validation and proves *syntax*, not *behaviour*. The plan must
treat validation as necessary-not-sufficient and note that the regex manager's
real proof is the next Renovate run's Dependency Dashboard showing a `go`
dependency at `1.27`. That confirmation lands post-merge and must not block the
PR.

## 3. Ordering: the one constraint that actually matters

The probe established that under `go1.27.0`, the *pinned* linter fails all six
modules — `packages/dto-go` with `internal/poll/splice_linux.go:237:21: unknown
field rfd in struct literal of type splicePipe (typecheck)`, the larger modules
by panicking inside `goanalysis/runner_loadingpackage.go`. That is v2.12.2's
vendored `go/types` choking on 1.27 stdlib source. It is true *today*, and is
invisible only because CI is still on 1.26.

This yields a hard rule: **the lint pin bump must precede or accompany the CI
`go-version` bump, never follow it.** Three orderings were considered.

**A. Lint pin first, as its own commit, then everything else.** — *Recommended.*
The first commit raises `GOLANGCI_LINT_VERSION` to `v2.13.1` and lands the three
gofumpt fixes. It is independently green on the *current* toolchain: v2.13.1
type-checks 1.26 stdlib fine, and the whitespace fixes are what v2.13.1 wants
regardless of Go version. The second commit does the version sweep. Every commit
on the branch is independently green, which means `git bisect` stays meaningful
and a revert of the toolchain sweep does not drag the linter back to a version
that cannot handle the toolchain a developer has installed locally.

**B. One atomic commit.** Simpler to describe and satisfies the PRD's "same
change" requirement literally. Rejected as the primary because it forfeits the
bisect property for no gain — the PRD's requirement is that the pin bump land in
the same *pull request*, which A also satisfies.

**C. Toolchain sweep first, lint pin second.** Rejected outright. It creates a
commit where CI is red by construction. Named here only so the plan explicitly
forbids it.

Within option A's second commit, the internal order is forced by tooling, not
preference: directives → `make tidy` (needs the new directives to produce the
right sum churn) → Dockerfiles → CI → docs → Renovate. `make tidy` cannot be
moved earlier and cannot be skipped.

## 4. `go.work.sum` churn — expected, and expected to be additive-only

Running `make tidy` under the 1.27 directive produces `go.work.sum` churn. The
probe measured it: **+12 lines, all additions, all `/go.mod` hash lines** for
modules already present in the graph — `github.com/alecthomas/units`,
`github.com/creack/pty`, `github.com/google/gofuzz`,
`github.com/modern-go/concurrent`, and similar. No `h1:` module-content hash
was added or removed, and no dependency version changed.

This matters because FR-4 and the acceptance criteria draw a bright line: only
checksum bookkeeping may change, never dependency versions. The probe confirms
the change lands on the right side of that line. The plan should make the check
mechanical rather than eyeball-based — a diff of `go.work.sum` that contains any
removed line, or any added line bearing an `h1:` hash for a module/version pair
not previously present, means something other than bookkeeping happened and the
change needs root-causing.

Note also that the *main* repo checkout currently shows `go.work.sum` as
modified on disk. That is pre-existing noise outside this worktree and is not
this task's to clean up; the plan should verify the task worktree starts clean
(it does) and not be surprised by the main checkout's state.

## 5. Scope boundary: version bump, not feature adoption

The PRD is emphatic that no Go 1.27 language or stdlib feature is adopted here,
and the design reinforces it with a concrete test rather than an exhortation.

The reason this boundary is worth defending: the workspace tests already pass
under `go1.27.0` with the tree unchanged at `go 1.25.0` (probe-results.md §
"Baseline"). The compiler side of this migration is already de-risked. The
*only* thing the directive bump changes is what the language permits — which is
exactly the surface a well-meaning implementer would start exercising. Every
such edit would be a behaviour change smuggled into a version-bump PR, invisible
to a reviewer scanning for `1.25`→`1.27`.

Mechanical boundary for the plan and for review: outside the three gofumpt
files, `git diff -- '*.go'` must be empty. Inside them, `git diff -w` must be
empty. Two commands, no judgement calls.

## 6. Alpine runtime base (FR-6)

Resolves to a verification with no edit. All four services already run
`FROM alpine:3.24` (`apps/*/Dockerfile:31`), 3.24 is the current series, and the
floating `3.24` tag already tracks the latest patch (3.24.1 at probe time). The
`golang:1.27-alpine` builder resolves to the `alpine3.24` variant, so builder
and runtime stay on the same Alpine series — which is what keeps the statically
linked binary's few dynamic assumptions (CA bundle, `/etc/passwd` layout) valid
across stages.

Only `scaffolding-checklist.md:73` is wrong, claiming `alpine:3.20`. That is a
documentation fix (FR-16), not a Dockerfile change.

## 7. Complete change inventory

Verified exhaustive by tree-wide sweep across `*.md`, `*.yml`, `*.yaml`,
`*.json`, `Dockerfile*`, `Makefile`, and `*.sh`, excluding `node_modules` and
`docs/tasks/`.

**Version declarations (7 files, 1 line each):** `go.work:1`,
`apps/{auth,fleet,media,notification}-service/go.mod:3`,
`packages/{dto-go,shared-go}/go.mod:3` — `go 1.25.0` → `go 1.27.0`.

**Builder images (4 files, line 1):** `apps/{auth,fleet,media,notification}-service/Dockerfile`
— `golang:1.26-alpine` → `golang:1.27-alpine`.

**CI (3 lines, 2 files):** `.github/workflows/pr.yml:15`, `pr.yml:50`,
`.github/workflows/main.yml:44` — `'1.26'` → `'1.27'`. Adjacent comments byte-identical.

**Lint pin (1 line):** `tools/lint.versions:5` — `v2.12.2` → `v2.13.1`.

**Lint fallout (3 files, whitespace only):**
`apps/fleet-service/internal/fuel/builder.go`,
`apps/fleet-service/internal/maintenancerecord/builder.go`,
`apps/media-service/internal/processing/worker_test.go`.

**Checksums:** `go.work.sum` (+12 additive lines, measured). Per-module `go.sum`
files as `make tidy` dictates.

**Documentation (3 files):** `README.md:87` (`Go 1.25+` → `Go 1.27+`);
`architecture-overview.md:18-19` (currently claims Go 1.25 *and* cites CI as
`'1.25'`, which was wrong even before this task — CI was 1.26 — so both the
version and the citation need correcting); `scaffolding-checklist.md:70,73`
(`golang:1.25-alpine` → `golang:1.27-alpine`; `alpine:3.20` → `alpine:3.24`).

**Renovate:** `renovate.json` — `enabledManagers` gains `custom.regex`, a new
`customManagers` array, one appended `packageRules` entry.

**Explicitly unmodified:** `apps/web/Dockerfile`, everything under `deploy/k8s`
(no manifest references a Go version — confirmed by sweep), `.golangci.yml` (the
`standard` group's membership did not change between v2.12.2 and v2.13.1; the
zero new analyzer findings prove it), and everything under `docs/tasks/` (§8).

## 8. Historical archives stay stale, deliberately

The sweep found `1.25`/`1.26` in ten files under `docs/tasks/task-0NN-*/`
(plans, contexts, audits from tasks 001–020). These are records of what was true
when that work happened. Rewriting them would falsify the record and make future
archaeology — "why did task-013 rely on multi-`%w` wrapping?" — misleading.
They stay. FR-17's acceptance grep is written to expect exactly these hits and
nothing else, which turns the exclusion into an assertion rather than an
oversight.

## 9. Verification strategy

Layered, because the layers fail differently and a single `make ci` at the end
would not tell you which one broke.

1. **Lint layer, before the sweep.** `tools/lint.sh --check --go` under the new
   pin and the *old* directives. Proves the pin bump is independently green and
   isolates gofumpt fallout from toolchain fallout.
2. **Compile and test layer.** `make vet`, `make build`, `make test`. The
   `-race` suite is the meaningful one; the probe captured it green on 1.27
   pre-change, so any post-change failure is attributable to the bump and gets
   root-caused rather than retried.
3. **Full gate.** `make ci`.
4. **Containers.** `docker build -f apps/<service>/Dockerfile .` for all four,
   from the repo root. This is the only check that exercises `golang:1.27-alpine`
   as an actual image rather than a string — the `go.mod` directive means a
   builder image older than 1.27 now fails loudly, so a typo in the tag surfaces
   here and nowhere else.
5. **Manifests.** Both `kustomize build` renders diffed against pre-change
   output (expected: byte-identical, since no manifest carries a Go version),
   plus both `kubectl apply --dry-run=server` invocations per CLAUDE.md. The
   local overlay is not exempt — CLAUDE.md records that skipping it hid a real
   failure through ten reviews.
6. **Renovate.** `renovate-config-validator --strict`, understood as a syntax
   check only (§2, OQ-5). Behavioural confirmation is post-merge.

## 10. Risks

**A newer golangci-lint finds something on CI that it did not find locally.**
Low — the probe ran the same script CI runs, tree-wide, on the same pinned
binary. Residual risk is platform-specific analysis (CI is linux/amd64, as is
the probe machine), so this is close to eliminated.

**A contributor on Go < 1.27 is hard-blocked.** Intended, per the PRD's
non-goals: no `toolchain` directive is added, so they get an explicit
"requires go >= 1.27.0" rather than a silent multi-hundred-megabyte auto-download.
The README prerequisite change is what makes this discoverable, which is why
FR-14 is not cosmetic.

**The Renovate regex manager silently does nothing.** Moderate likelihood, low
impact — this is the `custom.regex`-missing-from-`enabledManagers` trap in
OQ-5. Impact is limited to "drift protection does not activate", which is the
status quo. The plan should carry the `enabledManagers` edit as its own checked
item rather than folding it into "update renovate.json", precisely because it is
the easy thing to forget.

**Image size or startup regression.** Low. Go minor bumps move binary size by
low single-digit percentages. Worth a glance at the built image sizes during
step 4 of §9; anything beyond a few percent gets investigated before merge.

**Rollback.** Trivial and total. Revert the PR. No migration, no data change, no
manifest change, no persisted state — the only artifacts are container images,
and `deploy/k8s` pins by SHA tag, so a revert plus a rebuild fully restores the
prior state.
