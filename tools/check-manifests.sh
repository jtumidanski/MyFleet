#!/usr/bin/env bash
# Renders the Kubernetes overlays and asserts the invariants that CLAUDE.md and
# docs/runbooks/k3s-deployment.md state in prose but nothing previously enforced.
#
# The one that matters most is route-set parity. :80 and :443 are served by two
# IngressRoute objects, because a Traefik IngressRoute carrying a `tls` section
# is TLS-only. The route set includes the priority-200 internal-deny rule that
# keeps fleet-service's unauthenticated /internal/* surface off the public
# internet. kustomize `replacements` copies the routes from myfleet-routes into
# myfleet-routes-tls so the two cannot drift — this check proves the replacement
# actually fired. If it silently stops working, :443 renders with zero routes
# and the deny rule vanishes from the public entrypoint.
set -euo pipefail

cd "$(dirname "$0")/.."

command -v kustomize >/dev/null || { echo "FAIL: kustomize not on PATH"; exit 1; }

fail=0
note() { echo "  $1"; }
bad() { echo "  FAIL: $1"; fail=1; }

echo "==> rendering overlays"
for overlay in main local; do
  if ! kustomize build "deploy/k8s/overlays/$overlay" > "/tmp/myfleet-$overlay.yaml" 2>/tmp/myfleet-$overlay.err; then
    bad "deploy/k8s/overlays/$overlay does not render"
    sed 's/^/    /' "/tmp/myfleet-$overlay.err"
  else
    note "$overlay renders"
  fi
done

echo "==> main overlay must ship no cluster-scoped or stateful resources"
# The main overlay targets a SHARED cluster. Postgres/Kafka/MinIO are
# pre-existing there, Secrets are applied out-of-band so Argo CD's prune cannot
# remove them, and MyFleet has no business holding a ClusterRole.
for kind in PersistentVolumeClaim Secret ClusterRole ClusterRoleBinding; do
  n=$(grep -c "^kind: $kind$" /tmp/myfleet-main.yaml || true)
  if [ "$n" -ne 0 ]; then bad "main renders $n $kind (expected 0)"; else note "no $kind"; fi
done

if grep -q 'REPLACE_ME' /tmp/myfleet-main.yaml; then
  bad "main renders a REPLACE_ME placeholder"
else
  note "no placeholders"
fi

echo "==> IngressRoute route-set parity (:80 vs :443)"
python3 - <<'PY' || fail=1
import json, sys
try:
    import yaml
except ImportError:
    # Deliberately fatal rather than a skip. This is the check that guards the
    # internal-deny rule on :443; silently passing when it cannot run is the
    # worst outcome. Install with: pip install pyyaml
    print("  FAIL: PyYAML not installed, cannot verify route-set parity")
    sys.exit(1)

docs = [d for d in yaml.safe_load_all(open("/tmp/myfleet-main.yaml")) if d]
ir = {d["metadata"]["name"]: d for d in docs if d.get("kind") == "IngressRoute"}

ok = True
for name in ("myfleet-routes", "myfleet-routes-tls"):
    if name not in ir:
        print(f"  FAIL: {name} not rendered"); ok = False
if not ok:
    sys.exit(1)

web, tls = ir["myfleet-routes"], ir["myfleet-routes-tls"]
wr, tr = web["spec"].get("routes", []), tls["spec"].get("routes", [])

if not tr:
    print("  FAIL: myfleet-routes-tls rendered with NO routes — the kustomize "
          "replacement did not fire, so :443 serves nothing and the "
          "internal-deny rule is absent from the public entrypoint")
    ok = False
elif json.dumps(wr, sort_keys=True) != json.dumps(tr, sort_keys=True):
    print(f"  FAIL: route sets differ ({len(wr)} on :80, {len(tr)} on :443)")
    ok = False
else:
    print(f"  route sets identical ({len(wr)} routes on both entrypoints)")

# The deny rule is the reason parity matters; assert it explicitly rather than
# trusting that "identical" means "present".
for label, routes in (("web", wr), ("tls", tr)):
    deny = [r for r in routes if "internal" in r.get("match", "")]
    if len(deny) != 1:
        print(f"  FAIL: {label} has {len(deny)} internal-deny routes (expected 1)"); ok = False
        continue
    d = deny[0]
    if d.get("priority") != 200:
        print(f"  FAIL: {label} internal-deny priority is {d.get('priority')}, expected 200"); ok = False
    if "internal-deny" not in [m.get("name") for m in d.get("middlewares", [])]:
        print(f"  FAIL: {label} internal-deny route is missing the internal-deny middleware"); ok = False

if ok:
    print("  internal-deny present at priority 200 on both entrypoints")

# Every host must appear in every rule. A host present in the catch-all but
# missing from the deny rule would expose /internal/* on that hostname.
hosts = ("myfleet.tumidanski.com", "myfleet.tumidanski.me", "myfleet.home")
for label, routes in (("web", wr), ("tls", tr)):
    for r in routes:
        missing = [h for h in hosts if f"`{h}`" not in r.get("match", "")]
        if missing:
            print(f"  FAIL: {label} route '{r.get('match','')[:48]}…' omits {missing}")
            ok = False
if ok:
    print("  all three hosts present in every rule")

sys.exit(0 if ok else 1)
PY

echo
if [ "$fail" -ne 0 ]; then echo "manifest checks FAILED"; exit 1; fi
echo "manifest checks passed"
