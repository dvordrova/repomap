#!/usr/bin/env bash
set -euo pipefail

config_dir="${XDG_CONFIG_HOME:-$HOME/.config}/opencode"
json_file="$config_dir/opencode.json"
jsonc_file="$config_dir/opencode.jsonc"

mkdir -p "$config_dir"

if [[ -f "$jsonc_file" ]]; then
  cat >&2 <<EOF
Found $jsonc_file.

This helper will not rewrite JSONC because doing so could remove comments.
Add the following manually under the top-level "permission" object:

"external_directory": {
  "~/Library/Caches/repomap/**": "allow",
  "~/git/**": "allow"
}

The workflow agents already contain these permissions, so /go will work without this
global change.
EOF
  exit 2
fi

python3 - "$json_file" <<'PY'
import json
import sys
from pathlib import Path

path = Path(sys.argv[1])
if path.exists():
    try:
        data = json.loads(path.read_text(encoding="utf-8"))
    except Exception as exc:
        raise SystemExit(
            f"Cannot parse {path} as strict JSON: {exc}\n"
            "Back it up or edit the external_directory permission manually."
        )
else:
    data = {}

if not isinstance(data, dict):
    raise SystemExit(f"{path} must contain a top-level JSON object")

data.setdefault("$schema", "https://opencode.ai/config.json")
permission = data.setdefault("permission", {})
if not isinstance(permission, dict):
    raise SystemExit(
        'Existing top-level "permission" is not an object. Convert it to object syntax '
        "before merging granular external_directory rules."
    )

external = permission.setdefault("external_directory", {})
if not isinstance(external, dict):
    raise SystemExit(
        'Existing "permission.external_directory" is not an object. Convert it to '
        "object syntax before merging path rules."
    )

external["~/Library/Caches/repomap/**"] = "allow"
external["~/git/**"] = "allow"

backup = path.with_suffix(path.suffix + ".bak")
if path.exists():
    backup.write_text(path.read_text(encoding="utf-8"), encoding="utf-8")

path.write_text(json.dumps(data, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
print(f"Updated {path}")
if backup.exists():
    print(f"Backup:  {backup}")
PY

echo "Restart OpenCode for the global permission change to take effect."
