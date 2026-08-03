#!/usr/bin/env bash
# Drift gate for task-020. Every check must report 0.
# Scope: the two dev-guidelines skills plus the two reviewer agents that read them.
set -uo pipefail
cd "$(git rev-parse --show-toplevel)" || exit 2

SCOPE=(
  .claude/skills/backend-dev-guidelines
  .claude/skills/frontend-dev-guidelines
  .claude/skills/skill-rules.json
  .claude/agents/backend-guidelines-reviewer.md
  .claude/agents/frontend-guidelines-reviewer.md
)
FE=.claude/skills/frontend-dev-guidelines
BE=.claude/skills/backend-dev-guidelines
FEAGENT=.claude/agents/frontend-guidelines-reviewer.md
BEAGENT=.claude/agents/backend-guidelines-reviewer.md

fail=0
check() { # check <id> <description> <grep-output>
  local id=$1 desc=$2 out=$3 n
  n=$(printf '%s' "$out" | grep -c .)
  if [ "$n" -eq 0 ]; then
    printf 'PASS  %-8s %s\n' "$id" "$desc"
  else
    fail=1
    printf 'FAIL  %-8s %s  (%d)\n' "$id" "$desc" "$n"
    printf '%s\n' "$out" | sed 's/^/        /'
  fi
}

# --- PRD §10 mechanical checks ---
check G-01 "api2go"                  "$(grep -rin 'api2go' "${SCOPE[@]}")"
check G-02 "ServerInformation"       "$(grep -rn 'ServerInformation' "${SCOPE[@]}")"
check G-03 "MarshalResponse"         "$(grep -rn 'MarshalResponse' "${SCOPE[@]}")"
check G-04 "model.* composition lib" "$(grep -rnE 'model\.(Provider|Map|SliceMap|ParallelMap|ErrorProvider|FixedProvider)' "${SCOPE[@]}")"
check G-05 "EntityProvider"          "$(grep -rn 'EntityProvider' "${SCOPE[@]}")"
check G-06 "RouteInitializer"        "$(grep -rnE '\bRouteInitializer' "${SCOPE[@]}")"
check G-07 "RegisterHandler("        "$(grep -rnE 'RegisterHandler\(' "${SCOPE[@]}")"
check G-08 "services/<svc>-service"  "$(grep -rnE '\bservices/[a-z-]+-service' "${SCOPE[@]}")"
check G-09 "Jest (not jest-dom)"     "$(grep -rinE '\bjest\b' "$FE" "$FEAGENT" | grep -v 'jest-dom')"
# Substring, not \b-anchored: the drift also lives inside identifiers
# (bucketKeys, bansService, CreateBucketDialog, BanType). English words that
# merely contain "ban" are filtered; a legitimate prose use of "policy" can be
# kept by putting ALLOW-VOCAB:G-10 on the line. Markers are per-check
# (ALLOW-VOCAB:G-10, ALLOW-VOCAB:G-14, ALLOW-VOCAB:G-15, ...) so exempting a
# line from one check never silently exempts it from another.
check G-10 "prior-project vocabulary" "$(grep -rinE 'bucket|replication|\bban|policy|policies' "${SCOPE[@]}" | grep -viE 'banner|bandwidth|abandon|\bband\b|ALLOW-VOCAB:G-10')"

# --- design §9 Gate 1 additions ---
check G-11 "RestModel / GetName()"   "$(grep -rnE 'RestModel|GetName\(\)' "${SCOPE[@]}")"
check G-12 "@/ path alias"           "$(grep -rnE "from ['\"]@/" "$FE" "$FEAGENT")"
check G-13 "__mocks__/watchAll/mux"  "$(grep -rnE '__mocks__|watchAll|Methods\(http\.Method' "${SCOPE[@]}")"

# --- additions from Phase-3 verification ---
check G-14 "uuid.UUID entity ids"    "$(grep -rn 'uuid\.UUID' "${SCOPE[@]}" | grep -v 'ALLOW-VOCAB:G-14')"
check G-15 "uint32 entity ids"       "$(grep -rn 'uint32' "${SCOPE[@]}" | grep -v 'ALLOW-VOCAB:G-15')"
check G-16 "gorilla mux idiom"       "$(grep -rnE 'mux\.Router|router\.HandleFunc' "${SCOPE[@]}")"
check G-17 "fake handler deps"       "$(grep -rnE 'HandlerDependency|HandlerContext|server\.GetHandler|d\.Logger\(\)|ParseId\(' "${SCOPE[@]}")"
check G-18 "testify in Go examples"  "$(grep -rnE 'require\.(NoError|Error|Equal)|assert\.' "$BE" "$BEAGENT")"
check G-19 "mock-struct convention"  "$(grep -rnE 'Mock struct|\{package\}/mock/|mock/processor\.go|mock/provider\.go' "${SCOPE[@]}")"
check G-20 "frontend/ root path"     "$(grep -rnE '(^|[^a-z/])frontend/' "$FE" "$FEAGENT")"
# A guideline that says "there is no types/api/" is the opposite of drift: it
# stops the next reader re-deriving a path the old docs taught. But the check
# cannot tell that sentence from "put your types in types/api/", so those lines
# opt out with ALLOW-VOCAB:G-21, per-check like the other markers. Use it ONLY
# on a line that denies the path exists -- never to keep an instruction.
check G-21 "dead FE paths"           "$(grep -rnE 'components/common/|types/api/|lib/breadcrumbs/|lib/query-client|React Table|DataTable|data-table|\.service\.ts|services/api/index\.ts' "$FE" "$FEAGENT" | grep -v 'ALLOW-VOCAB:G-21')"
check G-22 "unset tsconfig flags"    "$(grep -rnE 'exactOptionalPropertyTypes|noImplicitOverride' "$FE")"
# Deliberately no ALLOW-VOCAB:G-23 escape, unlike G-10/G-14/G-15. Those
# checks exempt lines because their terms have legitimate uses in a backend
# guideline (prose mentioning "policy", a real uuid.UUID/uint32 type name).
# nginx.conf and bruno have no such legitimate use here: the only nginx.conf
# in the tree serves the SPA (apps/web), not backend API routing, and no
# bruno collection exists anywhere. A future task should not "fix" this
# asymmetry with G-10/G-14/G-15 by adding a marker -- that would let a real
# nginx/bruno regression back in unflagged.
check G-23 "nginx.conf/bruno"        "$(grep -rniE 'nginx\.conf|bruno' "$BE")"

echo
if [ "$fail" -eq 0 ]; then echo "drift-gate: ALL CHECKS PASS"; else echo "drift-gate: FAILURES PRESENT"; fi
exit "$fail"
