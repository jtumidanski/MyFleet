# task-029 — Implementation Context

Companion to [plan.md](plan.md). Everything an implementer needs that is not a
numbered step: where things live, what was already decided and why, and what
will bite you.

## Workspace

- **Worktree:** `/home/tumidanski/source/MyFleet/.worktrees/task-029-go-127-migration`
- **Branch:** `task-029-go-127-migration`, off `main` @ `285e175`
- **Local Go:** `go1.27.0 linux/amd64` (linuxbrew, `/home/linuxbrew/.linuxbrew/Cellar/go/1.27.0`) — already the target version, which is *why* the pinned linter's incompatibility was discovered locally before CI ever saw it.
- **Worktree starts clean.** The *main* checkout shows a modified `go.work.sum`; that is pre-existing noise outside this worktree, not this task's to clean up.
- **Node:** not always on `PATH`. `export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22` before any `make ci` / `make fe-*`.

## Key files

| File | Role in this task |
|---|---|
| `go.work:1` | Workspace `go` directive — `1.25.0` → `1.27.0` |
| `apps/{auth,fleet,media,notification}-service/go.mod:3` | Module directives, same change |
| `packages/{dto-go,shared-go}/go.mod:3` | Module directives, same change |
| `apps/{auth,fleet,media,notification}-service/Dockerfile:1` | `golang:1.26-alpine` → `golang:1.27-alpine` (line 31's `alpine:3.24` runtime stays) |
| `.github/workflows/pr.yml:15,50` | `setup-go` inputs, `'1.26'` → `'1.27'` |
| `.github/workflows/main.yml:44` | Third `setup-go` input, same change |
| `tools/lint.versions:5` | `GOLANGCI_LINT_VERSION` — `v2.12.2` → `v2.13.1`. Shell-sourced key=value, read by both `tools/lint.sh` and CI. Single source of truth for lint tooling. |
| `tools/lint.sh` | `--check` = CI's check-only mode; bare `--go` = fix mode. States the "fix, don't suppress, don't rev-gate" policy this task must honour. |
| `.golangci.yml` | `default: standard` + gofumpt/goimports formatters. **Not modified** — the `standard` group's membership did not change between v2.12.2 and v2.13.1. |
| `README.md:87` | `Go 1.25+` → `Go 1.27+` |
| `.claude/skills/backend-dev-guidelines/resources/architecture-overview.md:18-19` | Claims Go 1.25 and cites CI as `'1.25'` — both wrong, both fixed |
| `.claude/skills/backend-dev-guidelines/resources/scaffolding-checklist.md:70,73` | `golang:1.25-alpine` and `alpine:3.20` — both stale, both fixed |
| `renovate.json` | `enabledManagers` + new `customManagers` + one appended `packageRules` entry |

The three source files touched by the newer gofumpt:

- `apps/fleet-service/internal/fuel/builder.go` (finding at `:19`; the run of single-line setters at lines 19–25)
- `apps/fleet-service/internal/maintenancerecord/builder.go` (finding at `:26`; setters at lines 19–27)
- `apps/media-service/internal/processing/worker_test.go` (finding at `:39`; the two `fakeProvider` single-line methods at 39–40)

## Decisions already made — do not relitigate

All five of the PRD's open questions were resolved empirically during design
([design.md](design.md) §2), by running things rather than reasoning about them.
Summarised so nobody re-opens them mid-implementation:

1. **golangci-lint v2.13.1 is clean across all six modules** after three
   whitespace fixes. Zero analyzer findings — the newer vendored
   `honnef.co/go/tools v0.8.0` surfaced nothing. FR-12's "fix, don't suppress"
   policy holds without a carve-out.
2. **No `.go-version` / `mise` / `asdf` file.** Adding one would create a *fifth*
   place a Go version is written, authoritative only for people already running
   a version manager. Its own task, if ever.
3. **`1.27-alpine`, not `1.27.0-alpine`.** The floating minor tag picks up Go
   patch releases on the next rebuild instead of routing every security rollup
   through a 7-day-aged Renovate PR.
4. **No CI cache key change.** No key embeds the Go version, and Go's build
   cache is content-addressed including the toolchain build ID, so stale 1.26
   entries are inert misses rather than wrongly-reused artifacts. The
   `go.work.sum` churn and the `tools/lint.versions` edit bust what needs
   busting, for free.
5. **Renovate needs a `customManagers` regex manager, not just a
   `packageRules` entry** — and `custom.regex` must be added to the
   `enabledManagers` allowlist or the whole thing is a silent no-op.

## Dependencies and ordering

The only hard constraint in the whole task:

> **The golangci-lint pin bump must precede the CI `go-version` bump.**

`v2.12.2` fails all six modules under `go1.27.0` — `packages/dto-go` reports
`internal/poll/splice_linux.go:237:21: unknown field rfd in struct literal of
type splicePipe (typecheck)`; the larger modules panic inside
`goanalysis/runner_loadingpackage.go:74`. That is v2.12.2's vendored `go/types`
choking on 1.27 stdlib source. It is true *today* and invisible only because CI
still pins `1.26`. Land the CI bump first and you have a commit that is red by
construction.

Within the sweep, the internal order is forced by tooling rather than taste:
directives → `make tidy` (needs the new directives to produce the right sum
churn) → Dockerfiles → CI → docs → Renovate. `make tidy` cannot move earlier and
cannot be skipped.

Task 1 also captures the pre-change kustomize renders that Task 9 diffs against
— if you skip that step, Task 9 has no baseline.

## Traps

- **The Renovate `enabledManagers` omission.** The single most likely way to
  implement FR-18 and have it quietly not work: the config validates, Renovate
  runs, nothing is grouped, and nobody notices for months. Plan Task 6 Step 1
  exists solely to make this a checked item.
- **`fileMatch` vs `managerFilePatterns`.** `fileMatch` is the deprecated
  spelling. Use `managerFilePatterns`.
- **The prose `go-version` at `.github/workflows/main.yml:46`.** The string
  `go-version` appears inside a comment explaining setup-go's cache-key
  derivation. It has no `:`-then-quote after it, so the regex manager skips it —
  but a lazier regex would break here, and a careless `sed` would rewrite the
  comment. The comments around all three `go-version` lines must stay
  byte-identical (FR-9); `main.yml` cites a specific observed CI run ID that
  documents a real cache race.
- **The builder-file alignment churn is bait.** Seven lines of whitespace in
  `fuel/builder.go` will tempt "while we're here, let's restructure these
  builders." Do not. `git diff -w` on the three files must be empty.
- **Feature adoption creep.** The compiler side is already de-risked — the tree
  tested green on `go1.27.0` before any change. So the directive bump changes
  only *what the language permits*, which is exactly the surface someone starts
  exercising. Outside the three gofumpt files, `git diff main -- '*.go'` must be
  empty.
- **`docs/tasks/task-0NN-*/` keeps its stale `1.25`/`1.26` strings.** Ten files.
  They are historical records; rewriting them falsifies the archive. FR-17's
  acceptance grep is written to expect exactly these hits, which turns the
  exclusion into an assertion rather than an oversight.
- **The local kustomize overlay is not exempt from the server dry-run.**
  CLAUDE.md records that running only the `main` dry-run hid a real failure
  (`ClusterRoleBinding "myfleet-traefik" is invalid: subjects[0].namespace:
  Required value`) through ten reviews.

## Expected churn, measured not guessed

`make tidy` under the 1.27 directive produced, at probe time: **+12 lines in
`go.work.sum`, all additions, all `/go.mod` hash lines** for modules already in
the graph (`github.com/alecthomas/units`, `github.com/creack/pty`,
`github.com/google/gofuzz`, `github.com/modern-go/concurrent`, and similar). No
`h1:` module-content hash added or removed. No dependency version changed. Any
removed line, or any added `h1:` hash for a module/version pair not previously
present, means something other than bookkeeping happened.

## Verification commands

```sh
tools/lint.sh --check --go          # lint layer, isolates gofumpt from toolchain fallout
tools/lint.sh --go                  # fix mode — let it write the whitespace, don't hand-edit
make vet && make build && make test # compile + `go test -race` across the workspace
make ci                             # full gate (needs nvm)
docker build -f apps/<svc>/Dockerfile .          # context is the repo root, all four services
kustomize build deploy/k8s/overlays/{local,main} # diff against Task 1's baselines
kustomize build deploy/k8s/overlays/main  | kubectl apply --dry-run=server -f -
kustomize build deploy/k8s/overlays/local | kubectl apply --dry-run=server -f -
npx --yes --package renovate renovate-config-validator --strict
```

## What "done" does not include

`renovate-config-validator` proves *syntax*, not behaviour — it even reports
"Validating as global config", a looser check than repo-config validation. The
regex manager's real proof is the next Renovate run's Dependency Dashboard
showing a `go` dependency at `1.27`. That lands **post-merge** and must not
block the PR; flag it in the PR description as a follow-up to eyeball.
