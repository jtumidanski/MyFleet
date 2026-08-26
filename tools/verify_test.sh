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
AUTH='the branch may be called done'

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

# --- gate selection --------------------------------------------------------

assert_contains "flagless selects the container-build gate" 'container builds' "$flagless_out"
assert_contains "flagless selects the main dry-run"  'main overlay'  "$flagless_out"
assert_contains "flagless selects the local dry-run" 'local overlay' "$flagless_out"
assert_contains "--quick skips the container builds" 'container builds' "$quick_out"
assert_contains "--no-docker still selects the main dry-run"  'main overlay'  "$nodocker_out"
assert_contains "--no-docker still selects the local dry-run" 'local overlay' "$nodocker_out"

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
