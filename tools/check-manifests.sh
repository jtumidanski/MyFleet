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

# The deny rules are the reason parity matters; assert them explicitly rather
# than trusting that "identical" means "present". Every service with an
# unauthenticated /internal/* surface MUST have exactly one priority-200
# internal-deny rule on EACH entrypoint. This is checked by service name, not
# just by aggregate count: an aggregate-count-and-symmetry check would still
# pass if, say, the media-service rule were deleted from both entrypoints at
# once (a realistic failure mode, since kustomize `replacements` keeps :80
# and :443 in lockstep) — it would just see "1 deny rule, present on both
# sides" and call that fine. Naming the required set closes that hole.
#
# auth-service and notification-service joined the set with the platform admin
# console. notification-service's entry is the load-bearing one: its
# stripprefix removes the FULL /api/notifications prefix, so without the rule a
# public request to /api/notifications/internal/admin/purge arrives at the
# service as /internal/admin/purge — an unauthenticated "delete everything for
# this fleet" endpoint. auth-service is safe by accident today (every public
# route lives under /auth/…), and is listed anyway because "safe by accident" is
# one prefix change away from "not safe".
REQUIRED_DENY_SERVICES = {"fleet-service", "media-service", "auth-service", "notification-service"}

for label, routes in (("web", wr), ("tls", tr)):
    deny = [r for r in routes if "internal" in r.get("match", "")]
    by_service = {}
    for d in deny:
        svc_names = [s.get("name") for s in d.get("services", [])]
        key = svc_names[0] if len(svc_names) == 1 else tuple(svc_names)
        by_service.setdefault(key, []).append(d)

    for svc in REQUIRED_DENY_SERVICES:
        matches = by_service.get(svc, [])
        if len(matches) != 1:
            print(f"  FAIL: {label} has {len(matches)} internal-deny route(s) for service {svc!r} (expected 1)"); ok = False
            continue
        d = matches[0]
        if d.get("priority") != 200:
            print(f"  FAIL: {label} internal-deny route for {svc!r} priority is {d.get('priority')}, expected 200"); ok = False
        if "internal-deny" not in [m.get("name") for m in d.get("middlewares", [])]:
            print(f"  FAIL: {label} internal-deny route for {svc!r} is missing the internal-deny middleware"); ok = False

    unexpected = set(by_service) - REQUIRED_DENY_SERVICES
    if unexpected:
        print(f"  FAIL: {label} has internal-deny route(s) for unexpected service(s) {unexpected} — add them to REQUIRED_DENY_SERVICES if intentional"); ok = False

# Secondary diagnostic only: the route-set equality check above (wr == tr) is
# what actually guarantees :80/:443 parity for every route, deny or not. This
# just restates that fact in deny-rule terms for a human skimming output.
if ok:
    print(f"  internal-deny present at priority 200 on both entrypoints for {sorted(REQUIRED_DENY_SERVICES)}")

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
