# Verification

This is the owner document for what "done" means on a MyFleet branch. The
executable form of this document is `tools/verify.sh`; usage details (flags,
exit codes) live in `tools/verify.sh --help`, not here — this document does
not restate that text, only the rationale behind it.

`tools/verify.sh` runs four gates, in order, every one of them on every run
regardless of whether an earlier gate failed, so a single pass gives the
complete picture:

1. **repo gate** — `make ci`, which is exactly:

   ```
   lint-check vet test build fe-test fe-build manifests carfax-template
   ```

2. **container builds** — for each of the five service images:

   ```sh
   docker build -f apps/<svc>/Dockerfile .
   ```

   for `auth-service`, `fleet-service`, `media-service`,
   `notification-service`, and `web`. The build **context is the repo root**
   for all five, `apps/web` included — none of them build from their own
   subdirectory.

3. **cluster dry-run, `main` overlay**:

   ```sh
   kustomize build deploy/k8s/overlays/main | kubectl apply --dry-run=server -f -
   ```

4. **cluster dry-run, `local` overlay**:

   ```sh
   kustomize build deploy/k8s/overlays/local | kubectl apply --dry-run=server -f -
   ```

## Only a flagless run authorizes "done"

`tools/verify.sh` takes exactly three flags: `--quick`, `--no-docker`, and
`-h`/`--help`. An unrecognized option prints usage to stderr and exits 2.

`--no-docker` skips gate 2. `--quick` skips gates 2, 3, and 4. Both exit 0 on
success, and both print the script's own sentence when they do:

> this run does NOT authorize calling the branch done

Only a **flagless** run that exits 0 authorizes calling the branch done. Use
`--quick`/`--no-docker` for a fast inner loop while iterating, then run
`tools/verify.sh` flagless before opening or merging the PR. A successful
flagless run ends with:

> All gates passed — the branch may be called done.

## The unreachable-cluster skip

Gates 3 and 4 need a reachable Kubernetes context. If `kubectl` or
`kustomize` is missing, or no cluster answers `kubectl cluster-info`, the
script records both dry-run gates as `SKIPPED` with the attempted context
named, and — this is the sharp edge — a **flagless** run still exits 0 in
that state.

A flagless-0 exit in that state has **not** verified the manifests. Before
the PR merges, re-run `tools/verify.sh` from an environment with a reachable
cluster context (the shared `bee` context is fine for this — see below) so
gates 3 and 4 actually execute.

## Why gates 3 and 4 are two gates, not one

A missing `namespace:` in `deploy/k8s/infra-local/kustomization.yaml` made
`kubectl apply -k deploy/k8s/overlays/local` fail outright (`ClusterRoleBinding
"myfleet-traefik" is invalid: subjects[0].namespace: Required value`) and
slipped through ten reviews because only the `main` dry-run was ever run.
Rendering the `local` overlay cleanly does not imply applying it cleanly, and
a green `main` dry-run says nothing about `local`. The two overlays diverge
enough (bundled infra + dev Traefik for `local`; shared infra, no PVCs, no
Secrets for `main`) that collapsing them into one gate would have hidden
exactly this failure again.

`--dry-run=server` is safe to point at the shared `bee` context for both
gates: it validates against the API server without persisting anything, and
it needs the `traefik.io` CRDs present, which `bee` has.

## The `main`-overlay invariants live in one place

The `main` overlay must render with **no PersistentVolumeClaims, no Secrets,
no ClusterRole, and no placeholder values**. Those invariants are asserted in
exactly one place: `tools/check-manifests.sh`, which is run by `make ci`'s
`manifests` target (gate 1). `tools/verify.sh` never re-implements them —
gate 3 is a `kubectl` dry-run only, not a re-check of these rules. If the
invariants need to change, change `tools/check-manifests.sh`; nothing else in
this repo should grow a parallel copy of them.

## Node bootstrap

`node_env()` inside `tools/verify.sh` runs before gate 1:

- If `npm` is **absent** from `PATH`, it bootstraps:

  ```sh
  export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
  ```

- If `npm` is **present**, it changes nothing, regardless of version.
- If `npm` is present at a **non-22 major**, it prints a warning and takes no
  action.

The wrong-major case is a warning rather than an automatic fix on purpose: a
wrong-major Node makes an `fe-test`/`fe-build` failure look like a code
defect (a real regression), when the actual cause is the environment. Silently
switching versions out from under a developer would hide that distinction as
surely as ignoring it would; the warning keeps the two failure modes
distinguishable without touching a version the developer may have chosen
deliberately.

## When the script and CI disagree, CI is the authority

If `tools/verify.sh` ever passes on something CI fails, or vice versa, **CI is
the authority and the script is the bug** — fix `tools/verify.sh`, not the
expectation. MyFleet's CI is `.github/workflows/pr.yml`. Its `containers` job
builds only four images — `auth-service`, `fleet-service`, `media-service`,
`notification-service` — and excludes `apps/web`; `web`'s container image is
first built only after merge, by `.github/workflows/main.yml`. `make ci` does
not build any container images at all, so `tools/verify.sh` gate 2 is the
only pre-merge check that the `apps/web` image builds — without it, a
Dockerfile-breaking change to `apps/web` would pass every local `make`
target and every PR check, and only fail once it reached `main`.

## Individual `make` targets, for narrowing a failure

Running the full `make ci` (or `tools/verify.sh`) is the right call before a
PR, but while iterating on a single failure it is faster to run just the
target that's failing:

- `make vet`
- `make test`
- `make build`
- `make fe-test`
- `make fe-build`
- `make lint-check` — check-only, mutates nothing; this is what CI runs
- `make lint` — fixes what it can, then leaves the rest for you to fix by hand

## Module-local gates

For iterating inside a single module without paying for a whole-repo `make
ci` pass:

```sh
cd apps/<svc> && go build ./... && go vet ./... && go test -race ./...
tools/lint.sh --check --go apps/<svc>
npm run -w apps/web test
```

`golangci-lint` runs in workspace mode with the root `go.work` active, so a
module-local `tools/lint.sh --check --go apps/<svc>` needs no `go work sync`
first.

## What to do when a gate fails

**Gate 1 (`make ci`) fails** — narrow it to the specific sub-target that
failed (`make vet`, `make test`, `make build`, `make fe-test`, `make
fe-build`, `make lint-check`, or run `tools/check-manifests.sh` /
`carfax-template`'s check directly) rather than re-running the whole thing.
Module-local gates (above) narrow further inside one Go module or the web
app.

**Gate 2 (container builds) fails** — reproduce with the single failing
service's `docker build -f apps/<svc>/Dockerfile .` from the repo root
(matching CI's context exactly), not from inside `apps/<svc>`; a context
mismatch is a common false failure here.

**Gate 3 (`main` overlay dry-run) fails** — re-render locally with `kustomize
build deploy/k8s/overlays/main` to separate a kustomize-level error from an
API-server rejection, and check whether `tools/check-manifests.sh` (gate 1)
already caught an invariant violation — that is the more specific error.

**Gate 4 (`local` overlay dry-run) fails** — same approach as gate 3, against
`deploy/k8s/overlays/local`. Because this overlay bundles infra manifests
(see the incident above), check `deploy/k8s/infra-local/kustomization.yaml`
first for a missing `namespace:` or similar cross-resource reference before
assuming the failure is in an application manifest.

If gates 3 or 4 report `SKIPPED` rather than pass/fail, that is the
unreachable-cluster case above, not a failure — re-run from an environment
with a reachable context before treating the branch as verified.
