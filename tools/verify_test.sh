#!/usr/bin/env bash
# verify_test.sh — tests for tools/verify.sh's contract.
#
# What is under test is the CONTRACT, not the gates: which exit codes the flags
# produce, and that a flagged run says in words that it does not authorize
# calling the branch done. The gates themselves take many minutes and are
# short-circuited here by VERIFY_DRY_RUN=1, which records each selected gate as
# passed without executing it.
#
# Also under test, structurally: gate outcomes are recorded in exactly one place
# each. If a future edit appends to PASSED/FAILED outside step(), the summary
# starts answering from a second source of truth and these assertions fail.

set -uo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
VERIFY="$HERE/verify.sh"
[ -x "$VERIFY" ] || { echo "FATAL: $VERIFY not executable" >&2; exit 2; }

fails=0
ok()  { echo "ok   - $1"; }
bad() { echo "FAIL - $1"; fails=$((fails + 1)); }

assert_exit() {
    local desc="$1" want="$2"; shift 2
    "$@" >/dev/null 2>&1
    local got=$?
    [ "$got" = "$want" ] && ok "$desc" || bad "$desc (want exit $want, got $got)"
}
assert_contains() {
    local desc="$1" needle="$2" hay="$3"
    case "$hay" in *"$needle"*) ok "$desc" ;; *) bad "$desc (missing: $needle)" ;; esac
}
assert_eq() {
    local desc="$1" want="$2" got="$3"
    [ "$want" = "$got" ] && ok "$desc" || bad "$desc (want '$want', got '$got')"
}

NONAUTH='does NOT authorize calling the branch done'
AUTH='All gates passed — the branch may be called done.'

# --- argument parsing ------------------------------------------------------

assert_exit "--help exits 0"        0 "$VERIFY" --help
assert_exit "-h exits 0"            0 "$VERIFY" -h
assert_exit "unknown flag exits 2"  2 "$VERIFY" --bogus
assert_exit "unknown flag exits 2 (bare word)" 2 "$VERIFY" nonsense

# --- help text names both overlays and all five images ---------------------

help_out="$("$VERIFY" --help 2>&1)"
assert_contains "help names the main overlay"  'overlays/main'  "$help_out"
assert_contains "help names the local overlay" 'overlays/local' "$help_out"
assert_contains "help names --quick"           '--quick'        "$help_out"
assert_contains "help names --no-docker"       '--no-docker'    "$help_out"

# --- non-authorization semantics -------------------------------------------

quick_out="$(VERIFY_DRY_RUN=1 "$VERIFY" --quick 2>&1)"; quick_rc=$?
assert_eq       "--quick exits 0 on success" 0 "$quick_rc"
assert_contains "--quick says it does not authorize done" "$NONAUTH" "$quick_out"

nodocker_out="$(VERIFY_DRY_RUN=1 "$VERIFY" --no-docker 2>&1)"; nodocker_rc=$?
assert_eq       "--no-docker exits 0 on success" 0 "$nodocker_rc"
assert_contains "--no-docker says it does not authorize done" "$NONAUTH" "$nodocker_out"

flagless_out="$(VERIFY_DRY_RUN=1 "$VERIFY" 2>&1)"; flagless_rc=$?
assert_eq       "flagless exits 0 on success" 0 "$flagless_rc"
assert_contains "flagless authorizes done"    "$AUTH"   "$flagless_out"
case "$flagless_out" in
    *"$NONAUTH"*) bad "flagless must NOT print the non-authorization sentence" ;;
    *)            ok  "flagless does not print the non-authorization sentence" ;;
esac

# --- loud-skip semantics (a flagless run must never over-claim) ------------
#
# have()/cluster_reachable() are gate-SELECTION logic, not gate BODY work, so
# VERIFY_DRY_RUN=1 does not shortcut them (see their comments in verify.sh) —
# only step()'s execution of the gate body is faked. That means the real PATH
# still drives selection under a dry run, so hiding docker/kubectl/kustomize
# from a restricted PATH forces the loud-skip branches for real, without any
# extra failure-injection hook or second gate-selection path in verify.sh.
loudskip_bin="$(mktemp -d)"
trap 'rm -rf "$loudskip_bin"' EXIT
ln -s "$(command -v bash)" "$loudskip_bin/bash"
ln -s "$(command -v dirname)" "$loudskip_bin/dirname"

loudskip_out="$(VERIFY_DRY_RUN=1 PATH="$loudskip_bin" "$VERIFY" 2>&1)"; loudskip_rc=$?
assert_eq "loud-skip flagless run still exits 0" 0 "$loudskip_rc"
assert_contains "loud-skip flagless run states it does not authorize done" \
    "$NONAUTH" "$loudskip_out"
assert_contains "loud-skip flagless run names both skipped gates" \
    '2 gate(s) were skipped for lack of an environment' "$loudskip_out"
case "$loudskip_out" in
    *"$AUTH"*) bad "loud-skip flagless run must NOT print the authorization sentence" ;;
    *)         ok  "loud-skip flagless run does not print the authorization sentence" ;;
esac

rm -rf "$loudskip_bin"
trap - EXIT

# --- gate selection ----------------------------------------------------
#
# Each assertion pins the PASSED-line or SKIPPED-line rendering the summary
# actually emits (see verify.sh's step()/skip() label text and its summary
# printf formats), not just the gate name, so an assertion here fails if the
# gate lands in the wrong bucket — e.g. if --quick stopped skipping the
# container builds and ran them instead, the PASSED-line text below would
# appear in $quick_out and the SKIPPED-line assertion would fail.

assert_contains "flagless runs the container-build gate (PASSED)" \
    'container builds (5 images, context = repo root) PASSED' "$flagless_out"
assert_contains "flagless runs the main dry-run (PASSED)" \
    'cluster dry-run, main overlay PASSED' "$flagless_out"
assert_contains "flagless runs the local dry-run (PASSED)" \
    'cluster dry-run, local overlay PASSED' "$flagless_out"

assert_contains "--quick skips the container builds (SKIPPED)" \
    'container builds (--quick) SKIPPED' "$quick_out"
assert_contains "--quick skips the main dry-run (SKIPPED)" \
    'cluster dry-run, main overlay (--quick) SKIPPED' "$quick_out"
assert_contains "--quick skips the local dry-run (SKIPPED)" \
    'cluster dry-run, local overlay (--quick) SKIPPED' "$quick_out"

assert_contains "--no-docker skips the container builds (SKIPPED)" \
    'container builds (--no-docker) SKIPPED' "$nodocker_out"
assert_contains "--no-docker still runs the main dry-run (PASSED)" \
    'cluster dry-run, main overlay PASSED' "$nodocker_out"
assert_contains "--no-docker still runs the local dry-run (PASSED)" \
    'cluster dry-run, local overlay PASSED' "$nodocker_out"

# --- structural: outcomes are recorded in exactly one place each ------------

assert_eq "PASSED is appended in exactly one place" "1" "$(grep -c 'PASSED+=' "$VERIFY")"
assert_eq "FAILED is appended in exactly one place" "1" "$(grep -c 'FAILED+=' "$VERIFY")"
assert_eq "that place is inside step()" "2" \
    "$(awk '/^step\(\) \{/,/^\}/' "$VERIFY" | grep -c -e 'PASSED+=' -e 'FAILED+=')"

# The two dry-runs must be two separate step() calls, never one loop — the
# local-overlay incident is exactly the case where main passed and local did not.
assert_eq "two separate dry-run step() calls" "2" "$(grep -c 'step "cluster dry-run' "$VERIFY")"

echo
if [ "$fails" -eq 0 ]; then
    echo "verify_test.sh: all assertions passed"
    exit 0
fi
echo "verify_test.sh: $fails failure(s)" >&2
exit 1
