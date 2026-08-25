# task-029 — Pre-spec probe results

Empirical findings gathered on 2026-08-25 in the task worktree, before any file
was modified. These are measurements, not assumptions; the design phase should
build on them rather than re-derive them.

## Environment

- Local toolchain: `go version go1.27.0 linux/amd64` (linuxbrew,
  `/home/linuxbrew/.linuxbrew/Cellar/go/1.27.0`)
- Branch: `task-029-go-127-migration` off `main` @ `285e175`

## Baseline: workspace tests are green on Go 1.27 as-is

`go test github.com/jtumidanski/myfleet/...` passes with the tree unchanged
(`go 1.25.0` directive) under the 1.27 toolchain. All packages `ok`; no
failures, no build errors. This means the *compiler* side of the migration is
already de-risked — the language directive bump is the formality, not the
hazard.

## Blocker: golangci-lint v2.12.2 cannot type-check the Go 1.27 stdlib

`tools/lint.sh --check --go` under go1.27.0 fails **all six modules**:

```
lint.sh: FAIL — 6 failing target(s):
lint.sh:   lint:apps/auth-service
lint.sh:   lint:apps/fleet-service
lint.sh:   lint:apps/media-service
lint.sh:   lint:apps/notification-service
lint.sh:   lint:packages/dto-go
lint.sh:   lint:packages/shared-go
```

The smallest module gives the clean diagnosis:

```
$ tools/lint.sh --check --go packages/dto-go
.../go/1.27.0/libexec/src/internal/poll/splice_linux.go:237:21:
  unknown field rfd in struct literal of type splicePipe (typecheck)
	return &splicePipe{rfd: fds[0], wfd: fds[1]}
```

The larger modules do not report a diagnostic at all — they panic inside
`goanalysis/runner_loadingpackage.go:74` while loading packages.

This is v2.12.2's vendored `go/types` failing on 1.27 standard-library source.
It is **already true today** and is invisible to CI only because CI still pins
`go-version: '1.26'`. The instant FR-9 lands, lint breaks. The pin bump is
therefore a prerequisite of the toolchain bump, not a follow-up to it.

## Fix confirmed: v2.13.1 is clean

Latest release at probe time is `v2.13.1` (GitHub releases API; the pinned
`v2.12.2` is three releases behind). Run against `packages/dto-go` with the
repo's own config:

```
$ go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.1 \
    run -c ../../.golangci.yml ./...
0 issues.
```

**Caveat carried into Open Question 1:** only `packages/dto-go` was probed — the
smallest module, and the one least likely to surface new findings. v2.13.1
vendors `honnef.co/go/tools v0.8.0`, a newer staticcheck than v2.12.2 shipped,
so `packages/shared-go` and the four services must be probed before planning.

## Registry facts

- `golang:1.27-alpine` exists and resolves to the `alpine3.24` variant, matching
  the services' `alpine:3.24` runtime stage. Also available: `1.27.0-alpine`,
  `1.27-alpine3.23`.
- Alpine's newest series is `3.24` (latest patch `3.24.1`). The floating
  `alpine:3.24` tag already tracks it, so the runtime base needs no edit.

## Version drift inventory (pre-change)

| Location | Declares |
|---|---|
| `go.work:1` + all six `go.mod` | `go 1.25.0` |
| `apps/{auth,fleet,media,notification}-service/Dockerfile:1` | `golang:1.26-alpine` |
| `.github/workflows/pr.yml:15,50` | `go-version: '1.26'` |
| `.github/workflows/main.yml:44` | `go-version: '1.26'` |
| `README.md:87` | `Go 1.25+` |
| `architecture-overview.md:18` | Go 1.25, cites CI as `'1.25'` (wrong — CI is 1.26) |
| `scaffolding-checklist.md:70,73` | `golang:1.25-alpine`, `alpine:3.20` (both wrong) |
| `tools/lint.versions` | `GOLANGCI_LINT_VERSION=v2.12.2` |

Four distinct claimed versions across eight locations. No `.go-version`,
`.tool-versions`, or `mise.toml` exists to arbitrate.
