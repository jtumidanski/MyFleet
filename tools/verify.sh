#!/usr/bin/env bash
# tools/verify.sh — the pre-PR verification gate.
#
# One entry point for everything that must be clean before a branch is called
# "done". Gate 1 mirrors `make ci`, which is what CI runs; gate 2 mirrors the
# image builds in .github/workflows/pr.yml, which `make ci` does not do; gates 3
# and 4 have no CI analogue because CI has no cluster. When this script and CI
# disagree, CI is the authority and this script is the bug.
#
# Rationale, per-gate detail and what to do when a gate fails live in
# docs/verification.md. This script is the executable form of that document —
# do not restate its contents here or in CLAUDE.md.
#
# Every gate runs even after an earlier one fails, so one pass gives the
# complete picture. Exit status is non-zero if any gate failed.

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

NO_DOCKER=0
QUICK=0

usage() {
    cat <<'EOF'
usage: tools/verify.sh [options]

  --no-docker   skip the container-build gate. Faster inner loop; the run then
                does NOT authorize calling the branch done.
  --quick       skip the container builds AND both cluster dry-runs. Fastest
                inner loop; the run then does NOT authorize calling the branch
                done.
  -h, --help    this message

Gates, in order:

  1. repo gate            make ci
                          (lint-check vet test build fe-test fe-build
                           manifests carfax-template)
  2. container builds     docker build -f apps/<svc>/Dockerfile .
                          for auth-service, fleet-service, media-service,
                          notification-service, web — context is the repo root
  3. cluster dry-run      kustomize build deploy/k8s/overlays/main \
                            | kubectl apply --dry-run=server -f -
  4. cluster dry-run      kustomize build deploy/k8s/overlays/local \
                            | kubectl apply --dry-run=server -f -

Gates 3 and 4 are separate on purpose: a missing namespace: in
deploy/k8s/infra-local/kustomization.yaml once broke the local overlay while
main stayed green. When no cluster is reachable, both are recorded SKIPPED with
the attempted context named, and a flagless run still exits 0.

Exit: 0 every gate that ran passed, 1 a gate failed, 2 usage error.
Only a FLAGLESS exit 0 authorizes calling the branch done.
EOF
}

while [ $# -gt 0 ]; do
    case "$1" in
        --no-docker) NO_DOCKER=1; shift ;;
        --quick)     QUICK=1; NO_DOCKER=1; shift ;;
        -h|--help)   usage; exit 0 ;;
        *) echo "verify.sh: unknown option $1" >&2; usage >&2; exit 2 ;;
    esac
done

# VERIFY_DRY_RUN=1 records each selected gate as passed without executing it.
# It exists for tools/verify_test.sh, which tests the CONTRACT (exit codes,
# gate selection, summary wording) rather than the gates. The selection logic
# below is the real logic — the dry run IS the real run with the work removed.
DRY="${VERIFY_DRY_RUN:-0}"

PASSED=()
FAILED=()
SKIPPED=()
LOUD_SKIPPED=()

step() {
    local label="$1"; shift
    local rc=0
    if [ "$DRY" != "1" ]; then
        printf '\n\033[1m── %s\033[0m\n' "$label"
        "$@" || rc=1
    fi
    if [ "$rc" -eq 0 ]; then
        PASSED+=("$label")
    else
        FAILED+=("$label")
        printf '\033[31m✗ %s FAILED\033[0m\n' "$label"
    fi
}

# skip: an intentional skip the caller asked for with a flag.
skip() { SKIPPED+=("$1"); }

# loud_skip: a skip the ENVIRONMENT forced. A skip that is invisible is the
# exact failure the cluster gates exist to prevent, so these get a ⚠ line of
# their own above the pass list and a closing warning.
loud_skip() { LOUD_SKIPPED+=("$1"); }

have() {
    [ "$DRY" = "1" ] && return 0
    command -v "$1" >/dev/null 2>&1
}

# ------------------------------------------------------------------ node env
#
# Bootstrap ONLY when npm is absent; when it is present, touch nothing. When it
# is present at the wrong major, warn without changing anything — a wrong-major
# Node makes an fe-test/fe-build failure look like a code defect.

node_env() {
    [ "$DRY" = "1" ] && return 0
    if ! command -v npm >/dev/null 2>&1; then
        export NVM_DIR="$HOME/.nvm"
        # shellcheck disable=SC1091
        if [ -s "$NVM_DIR/nvm.sh" ]; then
            . "$NVM_DIR/nvm.sh" && nvm use 22 >/dev/null 2>&1 || true
        fi
        command -v npm >/dev/null 2>&1 \
            || printf '\033[33mverify.sh: npm still not on PATH after nvm bootstrap; the frontend gates will fail\033[0m\n' >&2
        return 0
    fi
    local major
    major="$(node --version 2>/dev/null | sed -n 's/^v\([0-9][0-9]*\).*/\1/p')"
    if [ -n "$major" ] && [ "$major" != "22" ]; then
        printf '\033[33mverify.sh: node v%s detected; this repo targets 22 (nvm use 22)\033[0m\n' "$major" >&2
    fi
}

node_env

# ------------------------------------------------------------------ gate 1
#
# `make ci` already runs tools/check-manifests.sh via its `manifests` target,
# which renders BOTH overlays and asserts the main-overlay invariants (no PVCs,
# no Secrets, no ClusterRole, no placeholders). Do not re-implement those here.

step "make ci (lint-check vet test build fe-test fe-build manifests carfax-template)" \
    make ci

# ------------------------------------------------------------------ gate 2

SERVICES=(auth-service fleet-service media-service notification-service web)

container_builds() {
    local rc=0 svc
    for svc in "${SERVICES[@]}"; do
        printf '\n  → apps/%s\n' "$svc"
        docker build -f "apps/$svc/Dockerfile" . || rc=1
    done
    return "$rc"
}

if [ "$QUICK" -eq 1 ]; then
    skip "container builds (--quick)"
elif [ "$NO_DOCKER" -eq 1 ]; then
    skip "container builds (--no-docker)"
elif ! have docker; then
    loud_skip "container builds (docker not on PATH)"
else
    step "container builds (${#SERVICES[@]} images, context = repo root)" container_builds
fi

# ------------------------------------------------------------ gates 3 and 4

dry_run() { # dry_run <overlay>
    kustomize build "deploy/k8s/overlays/$1" | kubectl apply --dry-run=server -f -
}

cluster_reachable() {
    [ "$DRY" = "1" ] && return 0
    kubectl cluster-info --request-timeout=5s >/dev/null 2>&1
}

# The CONTEXT NAME only. Never print kubeconfig contents or credentials.
kube_context() {
    kubectl config current-context 2>/dev/null || echo "none"
}

if [ "$QUICK" -eq 1 ]; then
    skip "cluster dry-run, main overlay (--quick)"
    skip "cluster dry-run, local overlay (--quick)"
elif ! have kubectl || ! have kustomize; then
    loud_skip "cluster dry-runs, BOTH overlays (kubectl or kustomize not on PATH)"
elif ! cluster_reachable; then
    loud_skip "cluster dry-runs, BOTH overlays (no reachable cluster; context: $(kube_context))"
else
    step "cluster dry-run, main overlay"  dry_run main
    step "cluster dry-run, local overlay" dry_run local
fi

# ----------------------------------------------------------------- summary

printf '\n\033[1m════ verify.sh summary ════\033[0m\n'
for s in ${SKIPPED[@]+"${SKIPPED[@]}"};      do printf '  \033[2m− %s SKIPPED\033[0m\n' "$s"; done
for s in ${LOUD_SKIPPED[@]+"${LOUD_SKIPPED[@]}"}; do printf '  \033[33m⚠ %s SKIPPED\033[0m\n' "$s"; done
for s in ${PASSED[@]+"${PASSED[@]}"};        do printf '  \033[32m✓\033[0m %s PASSED\n' "$s"; done
for s in ${FAILED[@]+"${FAILED[@]}"};        do printf '  \033[31m✗ %s FAILED\033[0m\n' "$s"; done

if [ "${#FAILED[@]}" -gt 0 ]; then
    printf '\n\033[31m%d gate(s) FAILED — the branch is not ready.\033[0m\n' "${#FAILED[@]}"
    exit 1
fi

if [ "$QUICK" -eq 1 ] || [ "$NO_DOCKER" -eq 1 ]; then
    printf '\n\033[33mAll gates that ran passed, but slow gates were skipped by a flag:\n'
    printf 'this run does NOT authorize calling the branch done. Re-run flagless.\033[0m\n'
fi

# This block must run on EVERY exit path that has loud skips, flagged or not —
# a --no-docker/--quick run that also hit an environment-forced skip (e.g. no
# reachable cluster) must not lose this warning to an early exit above.
if [ "${#LOUD_SKIPPED[@]}" -gt 0 ]; then
    printf '\n\033[33m⚠ %d gate(s) were skipped for lack of an environment — see the ⚠ lines above.\n' \
        "${#LOUD_SKIPPED[@]}"
    printf '  Everything that could run passed; the skipped gates were never evaluated.\033[0m\n'
fi

if [ "$QUICK" -eq 1 ] || [ "$NO_DOCKER" -eq 1 ]; then
    exit 0
fi

printf '\n\033[32mAll gates passed — the branch may be called done.\033[0m\n'
