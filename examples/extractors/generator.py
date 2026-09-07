"""Example: describe a company's generator from its own JSON configuration."""
import json
import pathlib
import sys

request = json.load(sys.stdin)
config_path = request["options"].get("config", "codegen.json")
config = json.loads((pathlib.Path(request["root"]) / config_path).read_text())

nodes = [{"id": "generator", "name": "Company generator", "path": config_path}]
links = []
for role in ("input", "output"):
    nodes.append({"id": role, "path": config[role]})
    links.append({"from": "generator", "to": role, "label": "configured " + role})

json.dump({"version": 1, "nodes": nodes, "links": links}, sys.stdout)
