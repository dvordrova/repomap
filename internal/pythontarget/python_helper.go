package pythontarget

// pythonParserHelper uses only Python's standard library and never imports or
// executes repository code. Repository bytes arrive over stdin; the isolated
// interpreter has no reason to put the repository on sys.path.
const pythonParserHelper = `
import ast
import base64
import configparser
import json
import re
import sys

try:
    import tomllib
except ImportError:
    print(json.dumps({"fatal": "python 3.11 or newer is required for TOML parsing"}))
    raise SystemExit(0)

def decode_file(item):
    try:
        return base64.b64decode(item["content"], validate=True).decode("utf-8")
    except Exception:
        return None

def toml_line(text, section, key):
    active = False
    for number, raw in enumerate(text.splitlines(), 1):
        stripped = raw.strip()
        if stripped.startswith("[") and stripped.endswith("]"):
            active = stripped == "[" + section + "]"
            continue
        if not active or "=" not in stripped:
            continue
        left = stripped.split("=", 1)[0].strip().strip("'\"")
        if left == key:
            return number
    return 0

def cfg_line(text, section, key):
    active = False
    for number, raw in enumerate(text.splitlines(), 1):
        stripped = raw.strip()
        if stripped.startswith("[") and stripped.endswith("]"):
            active = stripped[1:-1].strip().lower() == section.lower()
            continue
        if active and "=" in stripped and stripped.split("=", 1)[0].strip() == key:
            return number
    return 0

def script_rows(mapping, kind, path, text, section, errors):
    rows = []
    if mapping is None:
        return rows
    if not isinstance(mapping, dict):
        errors.append({"path": path, "reason": "scripts table is not a mapping"})
        return rows
    for name, value in mapping.items():
        if isinstance(value, dict):
            value = value.get("reference")
        if not isinstance(name, str) or not isinstance(value, str):
            errors.append({"path": path, "reason": "script name and value must be strings"})
            continue
        rows.append({
            "name": name.strip(), "value": value.strip(), "kind": kind,
            "path": path, "line": toml_line(text, section, name),
        })
    return rows

def pyproject(item, text):
    path = item["path"]
    try:
        data = tomllib.loads(text)
    except Exception:
        return {"path": path, "errors": [{"path": path, "reason": "malformed TOML"}]}
    errors = []
    scripts = []
    project = data.get("project", {})
    if project is not None and not isinstance(project, dict):
        errors.append({"path": path, "reason": "project table is not a mapping"})
        project = {}
    scripts.extend(script_rows(project.get("scripts"), "pep621", path, text, "project.scripts", errors))
    scripts.extend(script_rows(project.get("gui-scripts"), "pep621_gui", path, text, "project.gui-scripts", errors))
    tool = data.get("tool", {})
    if not isinstance(tool, dict):
        tool = {}
    poetry = tool.get("poetry", {})
    if isinstance(poetry, dict):
        scripts.extend(script_rows(poetry.get("scripts"), "poetry", path, text, "tool.poetry.scripts", errors))
    roots = []
    packages_out = []
    setuptools = tool.get("setuptools", {})
    if isinstance(setuptools, dict):
        package_dir = setuptools.get("package-dir", {})
        if isinstance(package_dir, dict):
            root = package_dir.get("")
            if isinstance(root, str):
                roots.append(root.strip())
        packages = setuptools.get("packages", {})
        if isinstance(packages, dict):
            find = packages.get("find", {})
            if isinstance(find, dict):
                where = find.get("where")
                if isinstance(where, str):
                    roots.append(where.strip())
                elif isinstance(where, list):
                    roots.extend(value.strip() for value in where if isinstance(value, str))
    hatch = tool.get("hatch", {})
    if isinstance(hatch, dict):
        build = hatch.get("build", {})
        targets = build.get("targets", {}) if isinstance(build, dict) else {}
        wheel = targets.get("wheel", {}) if isinstance(targets, dict) else {}
        hatch_packages = wheel.get("packages", []) if isinstance(wheel, dict) else []
        if isinstance(hatch_packages, list):
            packages_out.extend(value.strip() for value in hatch_packages if isinstance(value, str))
            for value in hatch_packages:
                if isinstance(value, str) and "/" in value.strip("/"):
                    roots.append(value.strip("/").rsplit("/", 1)[0])
    distribution = bool(project) or bool(poetry) or bool(packages_out)
    return {"path": path, "scripts": scripts, "source_roots": roots, "packages": packages_out, "distribution": distribution, "errors": errors}

def setup_cfg(item, text):
    path = item["path"]
    parser = configparser.ConfigParser(interpolation=None, strict=True)
    try:
        parser.read_string(text)
    except Exception:
        return {"path": path, "errors": [{"path": path, "reason": "malformed setup.cfg"}]}
    scripts = []
    errors = []
    section = "options.entry_points"
    for group, kind in (("console_scripts", "setup_cfg"), ("gui_scripts", "setup_cfg_gui")):
        if parser.has_option(section, group):
            value = parser.get(section, group)
            start_line = cfg_line(text, section, group)
            for offset, row in enumerate(value.splitlines()):
                row = row.strip().rstrip(",")
                if not row or row.startswith("#"):
                    continue
                scripts.append({"name_value": row, "kind": kind, "path": path, "line": start_line + offset})
    roots = []
    if parser.has_option("options.packages.find", "where"):
        roots.extend(value.strip() for value in parser.get("options.packages.find", "where").splitlines() if value.strip())
    if parser.has_option("options", "package_dir"):
        for row in parser.get("options", "package_dir").splitlines():
            row = row.strip()
            if row.startswith("="):
                roots.append(row[1:].strip())
    distribution = parser.has_section("metadata") or parser.has_section("options") or parser.has_section("options.entry_points")
    return {"path": path, "scripts": scripts, "source_roots": roots, "packages": [], "distribution": distribution, "errors": errors}

class UnsafeLiteral(Exception):
    pass

def safe_literal(node, env):
    if isinstance(node, ast.Constant) and isinstance(node.value, (str, int, float, bool, type(None))):
        return node.value
    if isinstance(node, (ast.List, ast.Tuple, ast.Set)):
        return [safe_literal(value, env) for value in node.elts]
    if isinstance(node, ast.Dict):
        return {
            safe_literal(key, env): safe_literal(value, env)
            for key, value in zip(node.keys, node.values) if key is not None
        }
    if isinstance(node, ast.Name) and node.id in env:
        return env[node.id]
    raise UnsafeLiteral()

def setup_py(item, text, tree):
    path = item["path"]
    env = {}
    scripts = []
    roots = []
    dynamic = False
    packages_out = []
    distribution = False
    setup_names = set()
    setup_modules = set()
    def target_names(target):
        if isinstance(target, ast.Name):
            return [target.id]
        if isinstance(target, (ast.Tuple, ast.List)):
            result = []
            for value in target.elts:
                result.extend(target_names(value))
            return result
        return []
    def invalidate(names):
        for name in names:
            env.pop(name, None)
            setup_names.discard(name)
            setup_modules.discard(name)
    for statement in tree.body:
        if isinstance(statement, ast.ImportFrom) and statement.module in ("setuptools", "distutils.core"):
            for imported in statement.names:
                invalidate([imported.asname or imported.name])
                if imported.name == "setup":
                    setup_names.add(imported.asname or imported.name)
            continue
        if isinstance(statement, ast.ImportFrom):
            invalidate([imported.asname or imported.name for imported in statement.names if imported.name != "*"])
            continue
        if isinstance(statement, ast.Import):
            for imported in statement.names:
                invalidate([imported.asname or imported.name.split(".")[0]])
                if imported.name == "setuptools":
                    setup_modules.add(imported.asname or "setuptools")
            continue
        if isinstance(statement, ast.Assign):
            names = []
            for target in statement.targets:
                names.extend(target_names(target))
            value_ok = False
            value = None
            if len(statement.targets) == 1 and isinstance(statement.targets[0], ast.Name):
                try:
                    value = safe_literal(statement.value, env)
                    value_ok = True
                except (UnsafeLiteral, TypeError):
                    pass
            invalidate(names)
            if value_ok:
                env[statement.targets[0].id] = value
            continue
        if isinstance(statement, ast.AnnAssign):
            names = target_names(statement.target)
            value_ok = False
            value = None
            if isinstance(statement.target, ast.Name) and statement.value is not None:
                try:
                    value = safe_literal(statement.value, env)
                    value_ok = True
                except (UnsafeLiteral, TypeError):
                    pass
            invalidate(names)
            if value_ok:
                env[statement.target.id] = value
            continue
        if isinstance(statement, ast.AugAssign):
            invalidate(target_names(statement.target))
            continue
        if isinstance(statement, ast.Delete):
            names = []
            for target in statement.targets:
                names.extend(target_names(target))
            invalidate(names)
            continue
        if isinstance(statement, (ast.FunctionDef, ast.AsyncFunctionDef, ast.ClassDef)):
            invalidate([statement.name])
            continue
        if not isinstance(statement, ast.Expr) or not isinstance(statement.value, ast.Call):
            continue
        node = statement.value
        is_setup = (isinstance(node.func, ast.Name) and node.func.id in setup_names) or (
            isinstance(node.func, ast.Attribute) and node.func.attr == "setup" and
            isinstance(node.func.value, ast.Name) and node.func.value.id in setup_modules)
        if not is_setup:
            if (isinstance(node.func, ast.Name) and node.func.id == "setup") or (
                isinstance(node.func, ast.Attribute) and node.func.attr == "setup"):
                dynamic = True
            continue
        distribution = True
        keywords = {keyword.arg: keyword.value for keyword in node.keywords if keyword.arg}
        if "package_dir" in keywords:
            try:
                package_dir = safe_literal(keywords["package_dir"], env)
                if isinstance(package_dir, dict) and isinstance(package_dir.get(""), str):
                    roots.append(package_dir[""].strip())
            except (UnsafeLiteral, TypeError):
                pass
        if "packages" in keywords:
            try:
                package_values = safe_literal(keywords["packages"], env)
                if isinstance(package_values, list):
                    packages_out.extend(value.strip() for value in package_values if isinstance(value, str))
            except (UnsafeLiteral, TypeError):
                pass
        if "py_modules" in keywords:
            try:
                module_values = safe_literal(keywords["py_modules"], env)
                if isinstance(module_values, list):
                    packages_out.extend(value.strip() for value in module_values if isinstance(value, str))
            except (UnsafeLiteral, TypeError):
                pass
        if "entry_points" not in keywords:
            continue
        try:
            entry_points = safe_literal(keywords["entry_points"], env)
        except (UnsafeLiteral, TypeError):
            dynamic = True
            continue
        if not isinstance(entry_points, dict):
            dynamic = True
            continue
        for group, kind in (("console_scripts", "setup_py"), ("gui_scripts", "setup_py_gui")):
            values = entry_points.get(group, [])
            if isinstance(values, str):
                values = [values]
            if not isinstance(values, list) or not all(isinstance(value, str) for value in values):
                dynamic = True
                continue
            for value in values:
                scripts.append({"name_value": value.strip(), "kind": kind, "path": path, "line": node.lineno})
    return {"path": path, "scripts": scripts, "source_roots": roots, "packages": packages_out, "distribution": distribution, "dynamic": dynamic, "errors": []}

def exact_main_guard(test):
    if not isinstance(test, ast.Compare) or len(test.ops) != 1 or not isinstance(test.ops[0], ast.Eq) or len(test.comparators) != 1:
        return False
    left, right = test.left, test.comparators[0]
    def is_name(node):
        return isinstance(node, ast.Name) and node.id == "__name__"
    def is_main(node):
        return isinstance(node, ast.Constant) and node.value == "__main__"
    return (is_name(left) and is_main(right)) or (is_main(left) and is_name(right))

def source_file(item, text):
    path = item["path"]
    try:
        tree = ast.parse(text, filename=path)
    except (SyntaxError, ValueError):
        return {"path": path, "syntax_error": True, "bindings": [], "guards": []}, None
    bindings = []
    state = {}
    def bound_names(target):
        if isinstance(target, ast.Name):
            return [target.id]
        if isinstance(target, (ast.Tuple, ast.List)):
            result = []
            for value in target.elts:
                result.extend(bound_names(value))
            return result
        return []
    def bind(name, kind, node, module="", target="", level=0):
        bindings.append({
            "name": name, "kind": kind, "line": node.lineno,
            "module": module, "target": target, "level": level,
        })
        if kind == "delete":
            state.pop(name, None)
        else:
            state[name] = {"kind": kind}
    for node in tree.body:
        if isinstance(node, ast.FunctionDef):
            bind(node.name, "function", node)
        elif isinstance(node, ast.AsyncFunctionDef):
            bind(node.name, "async_function", node)
        elif isinstance(node, ast.ClassDef):
            bind(node.name, "class", node)
        elif isinstance(node, ast.Assign):
            for assignment_target in node.targets:
                for name in bound_names(assignment_target):
                    bind(name, "object", node)
        elif isinstance(node, ast.AnnAssign):
            for name in bound_names(node.target):
                bind(name, "object", node)
        elif isinstance(node, ast.AugAssign):
            for name in bound_names(node.target):
                bind(name, "object", node)
        elif isinstance(node, ast.Delete):
            for deleted_target in node.targets:
                for name in bound_names(deleted_target):
                    bind(name, "delete", node)
        elif isinstance(node, ast.ImportFrom):
            for name in node.names:
                if name.name == "*":
                    continue
                bind(name.asname or name.name, "alias", node, node.module or "", name.name, node.level)
        elif isinstance(node, ast.Import):
            for name in node.names:
                local = name.asname or name.name.split(".")[0]
                bind(local, "alias_module", node, name.name, "", 0)
    guards = [node.lineno for node in tree.body if isinstance(node, ast.If) and exact_main_guard(node.test)]
    return {"path": path, "syntax_error": False, "bindings": bindings, "guards": guards}, tree

def main():
    try:
        request = json.load(sys.stdin)
    except Exception:
        print(json.dumps({"fatal": "invalid helper request"}))
        return
    configs = []
    sources = []
    setup_trees = {}
    decoded = {}
    for item in request.get("files", []):
        text = decode_file(item)
        if text is None:
            configs.append({"path": item.get("path", ""), "errors": [{"path": item.get("path", ""), "reason": "file is not UTF-8"}]})
            continue
        decoded[item["path"]] = text
        if item["kind"] in ("python", "python_script", "setup_py"):
            source, tree = source_file(item, text)
            sources.append(source)
            if item["kind"] == "setup_py" and tree is not None:
                setup_trees[item["path"]] = tree
    for item in request.get("files", []):
        text = decoded.get(item.get("path"))
        if text is None:
            continue
        if item["kind"] == "pyproject":
            configs.append(pyproject(item, text))
        elif item["kind"] == "setup_cfg":
            configs.append(setup_cfg(item, text))
        elif item["kind"] == "setup_py" and item["path"] in setup_trees:
            configs.append(setup_py(item, text, setup_trees[item["path"]]))
    print(json.dumps({"configs": configs, "sources": sources}, sort_keys=True, separators=(",", ":")))

main()
`
