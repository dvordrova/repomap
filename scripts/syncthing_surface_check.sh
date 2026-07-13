#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

SYNCTHING_REPO="${1:-../syncthing}"
if [ ! -d "$SYNCTHING_REPO/.git" ]; then
    echo "Skipping syncthing_surface_check: $SYNCTHING_REPO is not a git repo"
    echo "  Usage: $0 [path-to-syncthing]"
    exit 0
fi

EXPECTED_SYNCTHING_COMMIT="d4cffd848eb13d65f3caca5ef6da9a3fd25a2d6a"
ACTUAL_SYNCTHING_COMMIT="$(git -C "$SYNCTHING_REPO" rev-parse HEAD)"
if [ "$ACTUAL_SYNCTHING_COMMIT" != "$EXPECTED_SYNCTHING_COMMIT" ]; then
    echo "FAIL: Syncthing baseline is $ACTUAL_SYNCTHING_COMMIT; expected $EXPECTED_SYNCTHING_COMMIT" >&2
    exit 1
fi

OUTPUT_DIR="$(mktemp -d)"
trap 'rm -rf "$OUTPUT_DIR"' EXIT

go run ./cmd/repomap "$SYNCTHING_REPO" \
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
    raise SystemExit("offline Syncthing product run did not emit one report.json/report.html pair")

report = json.loads(reports[0].read_text())
surfaces = report.get("discovered_surfaces") or {}
triggers = surfaces.get("triggers") or []
if len(triggers) != 36 or len({trigger.get("id") for trigger in triggers}) != 36:
    raise SystemExit(f"Syncthing surface multiplicity mismatch: {len(triggers)} records")

kind_counts = {
    kind: sum(trigger.get("kind") == kind for trigger in triggers)
    for kind in {"process_entry", "http_route", "http_server"}
}
if kind_counts != {"process_entry": 18, "http_route": 12, "http_server": 6}:
    raise SystemExit(f"Syncthing surface kinds changed: {kind_counts}")

role_counts = {
    role: sum(trigger.get("executable_role") == role for trigger in triggers)
    for role in {"primary_application", "secondary_service", "tooling", "test_or_helper"}
}
if role_counts != {
    "primary_application": 1,
    "secondary_service": 24,
    "tooling": 10,
    "test_or_helper": 1,
}:
    raise SystemExit(f"Syncthing executable roles changed: {role_counts}")

readiness_counts = {
    value: sum(trigger.get("trace_readiness") == value for trigger in triggers)
    for value in {"trace_ready", "partial_trace_ready", "unsupported", "rejected"}
}
if readiness_counts != {
    "trace_ready": 8,
    "partial_trace_ready": 25,
    "unsupported": 1,
    "rejected": 2,
}:
    raise SystemExit(f"Syncthing trace-readiness changed: {readiness_counts}")

surface_role_counts = {
    value: sum(trigger.get("surface_role") == value for trigger in triggers)
    for value in {"entry_surface", "dynamic_frontier", "runtime_activity", "rejected", "noisy"}
}
if surface_role_counts != {
    "entry_surface": 33,
    "dynamic_frontier": 1,
    "runtime_activity": 0,
    "rejected": 2,
    "noisy": 0,
}:
    raise SystemExit(f"Syncthing surface roles changed: {surface_role_counts}")

process_entries = [trigger for trigger in triggers if trigger.get("kind") == "process_entry"]
primary = [trigger for trigger in process_entries if trigger.get("executable_role") == "primary_application"]
if len(primary) != 1 or primary[0].get("process_entrypoint", {}).get("location", {}).get("path") != "cmd/syncthing/main.go":
    raise SystemExit(f"exact primary process entry missing: {primary}")
if sum(trigger.get("executable_role") == "tooling" for trigger in process_entries) != 10:
    raise SystemExit("developer-tool process entries were not kept separate")
if sum(trigger.get("executable_role") == "secondary_service" for trigger in process_entries) != 6:
    raise SystemExit("secondary-service process entries were not retained")

diagnostics = surfaces.get("package_diagnostics") or []
relay_pool = [
    item for item in diagnostics
    if item.get("location", {}).get("path") == "cmd/infra/strelaypoolsrv/main.go"
    and item.get("location", {}).get("line") == 289
    and "undefined: auto.Assets" in item.get("message", "")
]
if len(diagnostics) != 3 or len(relay_pool) != 1:
    raise SystemExit(f"scoped generated-asset diagnostics changed: {diagnostics}")
if surfaces.get("unavailable_package_count") != 5:
    raise SystemExit(f"unavailable package count changed: {surfaces.get('unavailable_package_count')!r}")

html = html_reports[0].read_text()
for text in [
    "All surfaces",
    "Process entries",
    "Secondary services",
    "Package diagnostics",
    "Trace readiness reason",
    "trace-ready",
    "partial-trace candidates",
    "rejected/noisy",
]:
    if text not in html:
        raise SystemExit(f"rendered Syncthing report is missing {text!r}")

print(
    "OK: Syncthing retained 36 surfaces across 18 exact process entries while "
    "scoping three generated-asset diagnostics to five unavailable typed packages"
)
PY

go test ./internal/orient \
    -run TestReplaySavedSyncthingOrientationSeedsPartialTracesWithoutProvider \
    -count=1
