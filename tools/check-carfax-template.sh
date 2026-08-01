#!/usr/bin/env bash
# Asserts the Carfax URL template is byte-identical in all three places it
# lives. Nothing else enforces this, and the failure mode is silent: the button
# is only rendered when the template contains {vin} and uses https:
# (apps/web/src/lib/carfax.ts), so a ConfigMap edit that drops either one
# removes the Carfax button fleet-wide without a single error anywhere.
#
# The three homes and why each exists:
#   1. apps/web/src/lib/schemas/runtimeConfig.ts — DEFAULT_CARFAX_URL_TEMPLATE,
#      compiled into the bundle so the app works with no ConfigMap at all
#      (vite dev, a bare `docker run`, an overlay that has not adopted it).
#   2. apps/web/public/config/config.json — served by the dev server and baked
#      into the image, so the fetch succeeds before Kubernetes is involved.
#   3. deploy/k8s/base/web/configmap.yaml — what actually reaches a cluster.
#
# They cannot be collapsed into one file: (1) is TypeScript the bundler inlines,
# (2) is a static asset fetched at runtime, (3) is a Kubernetes object. So they
# are kept in step by this check instead.
set -euo pipefail

cd "$(dirname "$0")/.."

TS_FILE="apps/web/src/lib/schemas/runtimeConfig.ts"
JSON_FILE="apps/web/public/config/config.json"
CM_FILE="deploy/k8s/base/web/configmap.yaml"

fail=0
note() { echo "  $1"; }
bad() { echo "  FAIL: $1"; fail=1; }

echo "==> carfaxUrlTemplate must be identical in all three homes"

for f in "$TS_FILE" "$JSON_FILE" "$CM_FILE"; do
  [ -f "$f" ] || { bad "$f is missing"; }
done
[ "$fail" -eq 0 ] || { echo; echo "carfax template check FAILED"; exit 1; }

python3 - "$TS_FILE" "$JSON_FILE" "$CM_FILE" <<'PY' || fail=1
import json, re, sys

ts_file, json_file, cm_file = sys.argv[1:4]
ok = True


def bad(msg):
    global ok
    print(f"  FAIL: {msg}")
    ok = False


# 1. The compiled-in default. Matched on the exported constant specifically, so
#    an unrelated URL elsewhere in the module cannot satisfy the check.
ts = open(ts_file).read()
m = re.search(
    r"export const DEFAULT_CARFAX_URL_TEMPLATE\s*(?::\s*string\s*)?=\s*['\"]([^'\"]+)['\"]",
    ts,
)
ts_value = m.group(1) if m else None
if ts_value is None:
    bad(f"{ts_file} has no DEFAULT_CARFAX_URL_TEMPLATE string literal to compare")

# 2. The static asset the SPA fetches.
try:
    json_value = json.load(open(json_file)).get("carfaxUrlTemplate")
except (OSError, ValueError) as e:
    json_value = None
    bad(f"{json_file} is not valid JSON: {e}")
if json_value is None and ok:
    bad(f"{json_file} has no carfaxUrlTemplate key")

# 3. The ConfigMap. Its config.json is an embedded block scalar, so parse the
#    YAML and then the embedded document — a substring grep would pass on a
#    ConfigMap whose JSON is malformed and therefore unusable at runtime.
cm_value = None
try:
    import yaml
except ImportError:
    # Deliberately fatal rather than a skip: a check that silently passes when
    # it cannot run is worse than no check. Install with: pip install pyyaml
    bad("PyYAML not installed, cannot read the ConfigMap")
else:
    try:
        docs = [d for d in yaml.safe_load_all(open(cm_file)) if d]
    except (OSError, yaml.YAMLError) as e:
        docs = []
        bad(f"{cm_file} is not valid YAML: {e}")
    cms = [d for d in docs if d.get("kind") == "ConfigMap" and "config.json" in d.get("data", {})]
    if not cms:
        bad(f"{cm_file} has no ConfigMap carrying a config.json key")
    else:
        raw = cms[0]["data"]["config.json"]
        try:
            cm_value = json.loads(raw).get("carfaxUrlTemplate")
        except ValueError as e:
            bad(f"{cm_file}: data['config.json'] is not valid JSON ({e}) — "
                "the SPA would fall back to its built-in default at runtime")
        if cm_value is None and ok:
            bad(f"{cm_file}: data['config.json'] has no carfaxUrlTemplate key")

if ok:
    values = {ts_file: ts_value, json_file: json_value, cm_file: cm_value}
    if len(set(values.values())) != 1:
        bad("carfaxUrlTemplate has drifted:")
        for path, value in values.items():
            print(f"    {path}: {value!r}")
        print("    A ConfigMap that loses '{vin}' or the https: scheme silently "
              "removes the Carfax button for every vehicle.")
    else:
        print(f"  all three agree: {ts_value!r}")

sys.exit(0 if ok else 1)
PY

echo
if [ "$fail" -ne 0 ]; then echo "carfax template check FAILED"; exit 1; fi
echo "carfax template check passed"
