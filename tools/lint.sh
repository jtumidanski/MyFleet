#!/usr/bin/env bash
# tools/lint.sh — shared lint & format guard.
#
# One entry point for both local use (fix mode) and CI (--check mode), so the
# two can never disagree. golangci-lint v2 is the single authority for Go
# formatting (gofumpt + goimports via .golangci.yml `formatters`) and linting
# (`standard` group). The web workspaces use Prettier + ESLint via npm scripts.
#
# Both formatting and linter findings are enforced TREE-WIDE — this repo
# adopted the guard with a zero-finding baseline, so there is no burn-down and
# nothing is rev-gated. Keep it that way: fix findings, do not gate them.
#
# golangci-lint runs per-module in WORKSPACE MODE (root go.work active):
# service go.mod files are not standalone-consistent, so GOWORK=off would fail
# type-loading. The guard never requires `go work sync`.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=lint.versions
source "$ROOT/tools/lint.versions"

usage() {
    cat <<'EOF'
Usage: tools/lint.sh [--check] [--fmt] [--go|--web] [path ...]

  (no flags)    fix mode: rewrite files in place (formatters + lint --fix)
  --check       check mode: mutate nothing; non-zero exit on any violation
  --fmt         formatter layer only (produces the baseline reformat)
  --go / --web  restrict to one ecosystem (default: both)
  path ...      restrict Go module discovery to modules under these paths
                (no paths = whole tree)

Versions are pinned in tools/lint.versions. Exit: 0 clean, 1 violations, 2 usage.
EOF
}

CHECK=0
FMT_ONLY=0
DO_GO=1
DO_WEB=1
PATHS=()

while [ $# -gt 0 ]; do
    case "$1" in
        --check) CHECK=1 ;;
        --fmt)   FMT_ONLY=1 ;;
        --go)    DO_WEB=0 ;;
        --web)   DO_GO=0 ;;
        -h|--help) usage; exit 0 ;;
        -*) echo "lint.sh: unknown flag: $1" >&2; usage >&2; exit 2 ;;
        *) PATHS+=("$1") ;;
    esac
    shift
done

TOOLS_BIN="$ROOT/.cache/tools/bin"
GOLANGCI="$TOOLS_BIN/golangci-lint-$GOLANGCI_LINT_VERSION"

GO_RC=0
WEB_RC=0
FAILED=()

ensure_golangci() {
    [ -x "$GOLANGCI" ] && return 0
    mkdir -p "$TOOLS_BIN"

    # Fast path: download the pinned prebuilt release binary and verify it
    # against the release's published SHA256 checksums. This is ~10s vs the
    # multi-minute `go install` source build — it dominates cold-cache CI time.
    # Falls back to `go install` when the download path is unavailable (no
    # curl/sha256sum, unknown platform, or offline).
    local ver="${GOLANGCI_LINT_VERSION#v}" os="" arch="" asset url tmp
    case "$(uname -s)" in
        Linux) os=linux ;;
        Darwin) os=darwin ;;
    esac
    case "$(uname -m)" in
        x86_64 | amd64) arch=amd64 ;;
        arm64 | aarch64) arch=arm64 ;;
    esac

    if [ -n "$os" ] && [ -n "$arch" ] \
        && command -v curl >/dev/null 2>&1 && command -v sha256sum >/dev/null 2>&1; then
        asset="golangci-lint-${ver}-${os}-${arch}.tar.gz"
        url="https://github.com/golangci/golangci-lint/releases/download/${GOLANGCI_LINT_VERSION}"
        echo "lint.sh: downloading golangci-lint $GOLANGCI_LINT_VERSION prebuilt ($os-$arch) into $TOOLS_BIN ..."
        tmp="$(mktemp -d)"
        if curl -sSfL "$url/$asset" -o "$tmp/$asset" \
            && curl -sSfL "$url/golangci-lint-${ver}-checksums.txt" -o "$tmp/checksums.txt" \
            && (cd "$tmp" && grep " ${asset}\$" checksums.txt | sha256sum -c - >/dev/null 2>&1) \
            && tar -xzf "$tmp/$asset" -C "$tmp" \
            && mv "$tmp/golangci-lint-${ver}-${os}-${arch}/golangci-lint" "$GOLANGCI"; then
            chmod +x "$GOLANGCI"
            rm -rf "$tmp"
            return 0
        fi
        echo "lint.sh: WARNING — prebuilt download/verify failed; falling back to 'go install' (slower)." >&2
        rm -rf "$tmp"
    fi

    # Fallback: build from source (requires the Go toolchain).
    if ! command -v go >/dev/null 2>&1; then
        echo "lint.sh: ERROR — cannot fetch prebuilt golangci-lint and no go toolchain for the source fallback" >&2
        exit 1
    fi
    echo "lint.sh: installing golangci-lint $GOLANGCI_LINT_VERSION from source into $TOOLS_BIN ..."
    tmp="$(mktemp -d)"
    GOBIN="$tmp" go install "github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$GOLANGCI_LINT_VERSION"
    mv "$tmp/golangci-lint" "$GOLANGCI"
    rm -rf "$tmp"
}

discover_modules() {
    if [ "${#PATHS[@]}" -eq 0 ]; then
        find "$ROOT/apps" "$ROOT/packages" -name go.mod -not -path '*/node_modules/*' -print0 \
            | xargs -0 -n1 dirname | sort -u
    else
        local p target
        for p in "${PATHS[@]}"; do
            case "$p" in
                /*) target="$p" ;;
                *)  target="$ROOT/${p#./}" ;;
            esac
            find "$target" -name go.mod -not -path '*/node_modules/*' -print0 2>/dev/null \
                | xargs -0 -r -n1 dirname
        done | sort -u
    fi
}

run_go() {
    ensure_golangci

    local moddir rel fmt_out
    while IFS= read -r moddir; do
        rel="${moddir#"$ROOT"/}"

        # ---- formatter layer ----------------------------------------------
        if [ "$CHECK" -eq 1 ]; then
            if fmt_out="$(cd "$moddir" && "$GOLANGCI" fmt --diff -c "$ROOT/.golangci.yml" ./... 2>&1)" \
                    && [ -z "$fmt_out" ]; then
                : # clean
            else
                echo "lint.sh: FMT FAIL — $rel"
                printf '%s\n' "$fmt_out" | head -40 || true
                GO_RC=1
                FAILED+=("fmt:$rel")
            fi
        else
            if ! (cd "$moddir" && "$GOLANGCI" fmt -c "$ROOT/.golangci.yml" ./...); then
                echo "lint.sh: FMT ERROR — $rel"
                GO_RC=1
                FAILED+=("fmt:$rel")
            fi
        fi

        # ---- linter layer --------------------------------------------------
        if [ "$FMT_ONLY" -eq 0 ]; then
            local -a lintargs=(run -c "$ROOT/.golangci.yml")
            if [ "$CHECK" -eq 0 ]; then
                lintargs+=(--fix)
            fi
            if ! (cd "$moddir" && "$GOLANGCI" "${lintargs[@]}" ./...); then
                echo "lint.sh: LINT FAIL — $rel"
                GO_RC=1
                FAILED+=("lint:$rel")
            fi
        fi
    done < <(discover_modules)
}

run_web() {
    if ! command -v node >/dev/null 2>&1; then
        echo "lint.sh: ERROR — node not found; the web checks need Node" >&2
        WEB_RC=1
        FAILED+=("web:node-missing")
        return
    fi
    if [ ! -d "$ROOT/node_modules" ]; then
        echo "lint.sh: bootstrapping web dev tooling (npm ci) ..."
        (cd "$ROOT" && npm ci)
    fi

    if [ "$CHECK" -eq 1 ]; then
        if ! (cd "$ROOT" && npm run format:check); then
            echo "lint.sh: WEB FMT FAIL"
            WEB_RC=1
            FAILED+=("web:prettier")
        fi
        if [ "$FMT_ONLY" -eq 0 ]; then
            if ! (cd "$ROOT" && npm run lint); then
                echo "lint.sh: WEB LINT FAIL"
                WEB_RC=1
                FAILED+=("web:eslint")
            fi
        fi
    else
        if ! (cd "$ROOT" && npm run format); then
            WEB_RC=1
            FAILED+=("web:prettier")
        fi
        if [ "$FMT_ONLY" -eq 0 ]; then
            if ! (cd "$ROOT" && npm run lint -- --fix); then
                echo "lint.sh: WEB LINT FAIL — unfixable findings remain"
                WEB_RC=1
                FAILED+=("web:eslint")
            fi
        fi
    fi
}

if [ "$DO_GO" -eq 1 ]; then
    run_go
fi
if [ "$DO_WEB" -eq 1 ]; then
    run_web
fi

if [ "$GO_RC" -ne 0 ] || [ "$WEB_RC" -ne 0 ]; then
    echo ""
    echo "lint.sh: FAIL — ${#FAILED[@]} failing target(s):"
    printf 'lint.sh:   %s\n' "${FAILED[@]}"
    if [ "$CHECK" -eq 1 ]; then
        echo "lint.sh: run 'tools/lint.sh' (fix mode) locally, then commit the result."
    fi
    exit 1
fi
echo "lint.sh: OK"
