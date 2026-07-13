#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

CADDY_REPO="${1:-../caddy}"
if [ ! -d "$CADDY_REPO/.git" ]; then
    echo "Skipping caddy_surface_check: $CADDY_REPO is not a git repo"
    echo "  Usage: $0 [path-to-caddy]"
    exit 0
fi

EXPECTED_CADDY_COMMIT="873fac5fc094fe538d0c477509127bb321d51a32"
ACTUAL_CADDY_COMMIT="$(git -C "$CADDY_REPO" rev-parse HEAD)"
if [ "$ACTUAL_CADDY_COMMIT" != "$EXPECTED_CADDY_COMMIT" ]; then
    echo "FAIL: Caddy baseline is $ACTUAL_CADDY_COMMIT; expected $EXPECTED_CADDY_COMMIT" >&2
    exit 1
fi

OUTPUT_DIR="$(mktemp -d)"
trap 'rm -rf "$OUTPUT_DIR"' EXIT

go run ./cmd/surface-discovery-playground --repo "$CADDY_REPO" --out "$OUTPUT_DIR"

python3 - "$OUTPUT_DIR/trigger_catalog.json" "$OUTPUT_DIR/surface_coverage.json" "$OUTPUT_DIR/architecture_grounding.json" <<'PY'
import json
import pathlib
import sys

catalog = json.loads(pathlib.Path(sys.argv[1]).read_text())
coverage = json.loads(pathlib.Path(sys.argv[2]).read_text())
grounding = json.loads(pathlib.Path(sys.argv[3]).read_text())
triggers = catalog["triggers"]

expected_registration_routes = {
    "/config/",
    "/id/",
    "/stop",
    "/debug/pprof/",
    "/debug/pprof/cmdline",
    "/debug/pprof/profile",
    "/debug/pprof/symbol",
    "/debug/pprof/trace",
    "/debug/vars",
}
expected_descriptor_routes = {
    "/load",
    "/adapt",
    "/metrics",
    "/pki/",
    "/reverse_proxy/upstreams",
}
exact_registration_routes = {
    trigger["identity"]["path"].get("text", "")
    for trigger in triggers
    if trigger["kind"] == "http_route"
    and trigger["identity"]["path"].get("known")
}
exact_descriptor_routes = {
    trigger["identity"]["path"].get("text", "")
    for trigger in triggers
    if trigger["kind"] == "http_route_descriptor"
    and trigger["identity"]["path"].get("known")
}
if exact_registration_routes != expected_registration_routes:
    raise SystemExit(f"Caddy registration mismatch: got={sorted(exact_registration_routes)}")
if exact_descriptor_routes != expected_descriptor_routes:
    raise SystemExit(f"Caddy descriptor mismatch: got={sorted(exact_descriptor_routes)}")

server_sites = {
    (trigger["server_start_site"]["path"], trigger["server_start_site"]["line"])
    for trigger in triggers
    if trigger["kind"] == "http_server"
}
expected_server_sites = {
    ("admin.go", 442),
    ("admin.go", 601),
    ("modules/caddyhttp/app.go", 619),
    ("modules/caddyhttp/server.go", 838),
}
if server_sites != expected_server_sites:
    raise SystemExit(f"Caddy server-root mismatch: got={sorted(server_sites)}")

dynamic_routes = [
    trigger
    for trigger in triggers
    if trigger["kind"] == "http_route" and not trigger["identity"]["path"].get("known")
]
if len(dynamic_routes) != 1 or dynamic_routes[0]["registration_site"] != {
    "path": "admin.go",
    "line": 240,
    "column": 21,
}:
    raise SystemExit(f"Caddy dynamic route frontier mismatch: {dynamic_routes}")

provider_routes = [trigger for trigger in triggers if trigger["kind"] == "http_route_descriptor"]
if len(provider_routes) != 5 or not all(
    trigger["status"] == "confirmed_route_descriptor"
    and trigger["resolution"] == "exact"
    and not trigger["provisional_id"]
    and trigger.get("descriptor_site")
    and trigger["framework"] == "caddy-admin"
    and
    any(frontier["kind"] == "route_provider_dispatch_candidate" for frontier in trigger["dynamic_frontier"])
    for trigger in provider_routes
):
    raise SystemExit("Caddy admin provider evidence is incomplete")

route_frontiers = [trigger for trigger in triggers if trigger["kind"] == "http_route_frontier"]
if len(route_frontiers) != 2 or not all(any(
    frontier["kind"] == "configuration_assembled_route_inventory"
    for frontier in trigger["dynamic_frontier"]
) for trigger in route_frontiers):
    raise SystemExit(f"configured-site route frontier mismatch: {route_frontiers}")
frontier_sites = {
    (trigger["registration_site"]["path"], trigger["registration_site"]["line"])
    for trigger in route_frontiers
}
if frontier_sites != {
    ("modules/caddyhttp/app.go", 369),
    ("modules/caddyhttp/app.go", 379),
}:
    raise SystemExit(f"configured-site frontier sites mismatch: {sorted(frontier_sites)}")

exact_routes = [
    trigger for trigger in triggers
    if trigger["kind"] == "http_route" and trigger["identity"]["path"].get("known")
]
if len(exact_routes) != 9 or any(trigger["resolution"] != "exact" for trigger in exact_routes):
    raise SystemExit("exact Caddy registrations were degraded by auxiliary reachability frontiers")
server_roots = [trigger for trigger in triggers if trigger["kind"] == "http_server"]
if any(trigger["resolution"] == "dynamic" or trigger["provisional_id"] for trigger in server_roots):
    raise SystemExit("exact Caddy server start sites were presented as dynamically identified")

if any(pathlib.PurePath(trigger["registration_site"]["path"]).is_absolute() for trigger in triggers):
    raise SystemExit("Caddy catalog retained a dependency-owned absolute registration site")

required_seeds = {
    "net-http-servemux-handle",
    "net-http-server-serve",
    "quic-go-http3-server-serve-listener",
    "caddy-admin-load-routes",
    "caddy-admin-metrics-routes",
    "caddy-admin-pki-routes",
    "caddy-admin-reverse-proxy-routes",
    "caddy-http-route-list-compile",
}
matched_seeds = set(coverage["configured_seeds_matched"])
if not required_seeds <= matched_seeds:
    raise SystemExit(f"Caddy seeds missing: {sorted(required_seeds - matched_seeds)}")

if len(triggers) != 21 or len({trigger["id"] for trigger in triggers}) != 21:
    raise SystemExit(f"Caddy surface multiplicity mismatch: {len(triggers)} records")

anchors = grounding.get("behavior_anchors") or []
anchor_counts = {
    kind: sum(anchor.get("kind") == kind for anchor in anchors)
    for kind in {anchor.get("kind") for anchor in anchors}
}
if len(anchors) != 13 or any(count > 16 for count in anchor_counts.values()):
    raise SystemExit(
        "detached surface recovery polluted bounded Caddy architecture anchors: "
        f"total={len(anchors)} by_kind={anchor_counts}"
    )

print(
    "OK: Caddy matched 14 exact admin route facts, 2 configured-site route frontiers, "
    "and 4 static HTTP server start sites without inventing configured site routes"
)
PY

go run ./cmd/repomap "$CADDY_REPO" \
    --offline \
    --no-open \
    --no-serve \
    --debug-dir "$OUTPUT_DIR/runs" \
    >/dev/null

python3 - "$OUTPUT_DIR/runs" <<'PY'
import json
import pathlib
import sys

reports = sorted(path for path in pathlib.Path(sys.argv[1]).glob("*/report.json") if path.parent.name != "latest")
html_reports = sorted(path for path in pathlib.Path(sys.argv[1]).glob("*/report.html") if path.parent.name != "latest")
if len(reports) != 1 or len(html_reports) != 1:
    raise SystemExit("offline Caddy product run did not emit one report.json/report.html pair")
report = json.loads(reports[0].read_text())
surfaces = report.get("discovered_surfaces") or {}
triggers = surfaces.get("triggers") or []
expected_counts = {
    "http_route": 10,
    "http_route_descriptor": 5,
    "http_route_frontier": 2,
    "http_server": 4,
}
actual_counts = {
    kind: sum(trigger.get("kind") == kind for trigger in triggers)
    for kind in expected_counts
}
if actual_counts != expected_counts:
    raise SystemExit(f"projected Caddy surface mismatch: {actual_counts}")
expected_projected_fields = {
    "http_route_count": 10,
    "http_route_descriptor_count": 5,
    "http_route_frontier_count": 2,
    "http_server_count": 4,
}
for field, expected in expected_projected_fields.items():
    if surfaces.get(field) != expected:
        raise SystemExit(f"projected Caddy count {field}={surfaces.get(field)!r}, expected {expected}")
if surfaces.get("application_count") != 21 or surfaces.get("unassigned_count") != 0:
    raise SystemExit(
        "Caddy surfaces were not attributed to the exact repository-named main executable: "
        f"application={surfaces.get('application_count')!r} unassigned={surfaces.get('unassigned_count')!r}"
    )
html = html_reports[0].read_text()
for text in [
    "All surfaces",
    "complete route inventory",
    "Server start sites",
    "Route descriptors",
    "HTTP registrations",
    "hasWrapperEvidence",
    "hasDynamicEvidence",
]:
    if text not in html:
        raise SystemExit(f"rendered Caddy report is missing {text!r}")
print("OK: offline repomap product report preserves the Caddy surface distinctions")
PY
