import ast
import base64
import hashlib
import io
import json
import sys
import tokenize


def stable_ref(domain, *parts):
    wire = json.dumps([domain, *parts], ensure_ascii=False, separators=(",", ":")).encode("utf-8")
    return "python-source-" + hashlib.sha256(wire).hexdigest()[:32]


def code_line_set(content, tree):
    """The lines of a module holding code: a token that is not a comment,
    outside every docstring (the first statement string of a module, class or
    function). Only line spans are read, never the text."""
    documented = set()
    for node in ast.walk(tree):
        if isinstance(node, (ast.Module, ast.FunctionDef, ast.AsyncFunctionDef, ast.ClassDef)) and node.body:
            first = node.body[0]
            if isinstance(first, ast.Expr) and isinstance(first.value, ast.Constant) and isinstance(first.value.value, str):
                documented.update(range(first.lineno, first.end_lineno + 1))
    skipped = {tokenize.COMMENT, tokenize.NL, tokenize.NEWLINE, tokenize.INDENT, tokenize.DEDENT,
               tokenize.ENCODING, tokenize.ENDMARKER}
    lines = set()
    try:
        for token in tokenize.generate_tokens(io.StringIO(content).readline):
            if token.type in skipped:
                continue
            for row in range(token.start[0], token.end[0] + 1):
                if row not in documented:
                    lines.add(row)
    except (tokenize.TokenError, SyntaxError):
        return frozenset()
    return frozenset(lines)


def code_lines_between(tree, first, last):
    lines = getattr(tree, "repomap_code_lines", frozenset())
    return sum(1 for row in lines if first <= row <= last)


def source_location(path, node):
    if getattr(node, "no_location", False):
        return None
    line = getattr(node, "lineno", 0)
    column = getattr(node, "col_offset", -1) + 1
    if line < 1 or column < 1:
        return None
    return {"path": path, "line": line, "column": column}


def source_identity(path, node):
    location = source_location(path, node)
    if location is None:
        return ""
    key = "%s:%d:%d" % (location["path"], location["line"], location["column"])
    if isinstance(node, ast.Call):
        # Nested fluent calls share their starting position. Their complete
        # AST spans distinguish the calls and each call's original arguments.
        key += ":%d:%d" % (node.end_lineno, node.end_col_offset)
    return key


def callee_location(path, node):
    if isinstance(node, ast.Attribute):
        line = getattr(node, "end_lineno", 0)
        end_column = getattr(node, "end_col_offset", -1)
        column = end_column - len(node.attr.encode("utf-8")) + 1
        if line >= 1 and column >= 1:
            return {"path": path, "line": line, "column": column}
    return source_location(path, node)


def visibility(name, forced_internal=False, scope=None):
    if forced_internal or name.startswith("_"):
        return "internal"
    # A module that declares __all__ exports exactly the names it lists.
    if scope is not None and scope.kind == "module" and scope.declared_all is not None and name not in scope.declared_all:
        return "internal"
    return "public"


def declared_all(tree):
    """The names a module's top-level __all__ lists, or None when it declares
    none or not as a literal list or tuple of strings."""
    names = None
    for statement in getattr(tree, "body", []):
        if isinstance(statement, ast.Assign):
            targets, value = statement.targets, statement.value
        elif isinstance(statement, ast.AnnAssign) and statement.value is not None:
            targets, value = [statement.target], statement.value
        else:
            continue
        if not any(isinstance(target, ast.Name) and target.id == "__all__" for target in targets):
            continue
        if not isinstance(value, (ast.List, ast.Tuple)) or not all(
                isinstance(item, ast.Constant) and isinstance(item.value, str) for item in value.elts):
            return None
        names = {item.value for item in value.elts}
    return names


def bounded_text(value):
    # The shared ProgramIndex aggregate/envelope bounds own rejection. Local
    # clipping here used to preserve a plausible but incomplete fact.
    return " ".join(str(value).split())


def safe_expression_name(node):
    if isinstance(node, ast.Name):
        return node.id
    if isinstance(node, ast.Attribute):
        base = safe_expression_name(node.value)
        return base + "." + node.attr if base else node.attr
    if isinstance(node, ast.Subscript):
        # Generic parameters and subscription keys can contain arbitrary
        # literals. The structural callee/base name is sufficient here.
        return safe_expression_name(node.value)
    if isinstance(node, ast.Call):
        # Preserve the shape of fluent registrations without copying argument
        # contents: scheduler.every().day.at().do is not merely "do".
        base = safe_expression_name(node.func)
        return base + "()" if base else ""
    return ""


def declaration_arguments(arguments):
    # Preserve declaration syntax without evaluating annotations or defaults.
    # ast.unparse escapes multiline strings and preserves spaces inside literals.
    return ast.unparse(arguments)


def type_parameters(node):
    # PEP 695 type parameters are part of the declaration, as in
    # class Box[T]: and def first[T](items: list[T]) -> T. Interpreters
    # before Python 3.12 have no such field.
    parameters = getattr(node, "type_params", None)
    if not parameters:
        return ""
    return "[" + ", ".join(ast.unparse(value) for value in parameters) + "]"


def function_signature(node):
    prefix = "async " if isinstance(node, ast.AsyncFunctionDef) else ""
    signature = prefix + node.name + type_parameters(node) + "(" + declaration_arguments(node.args) + ")"
    if node.returns is not None:
        signature += " -> " + ast.unparse(node.returns)
    return signature


def class_signature(node):
    bases = [ast.unparse(value) for value in node.bases]
    bases.extend(ast.unparse(keyword) for keyword in node.keywords)
    header = "class " + node.name + type_parameters(node)
    if not bases:
        return header
    return header + "(" + ", ".join(bases) + ")"


def relative_module(current, is_package, level, module):
    if level == 0:
        return module or ""
    parts = current.split(".") if is_package else current.split(".")[:-1]
    remove = level - 1
    if remove > len(parts):
        return ""
    parts = parts[:len(parts) - remove]
    if module:
        parts.extend(module.split("."))
    return ".".join(parts)


class Scope:
    def __init__(self, ref, qname, kind, parent=None, class_ref="", class_qname=""):
        self.ref = ref
        self.qname = qname
        self.kind = kind
        self.parent = parent
        self.class_ref = class_ref or (parent.class_ref if parent else "")
        self.class_qname = class_qname or (parent.class_qname if parent else "")
        self.bindings = {}
        self.declared_all = None
        self.export_bindings = {}
        self.export_star_import = False
        self.global_names = set()
        self.nonlocal_names = set()
        self.opaque_names = set()
        # Every assignment, def, class and import binding a name in this
        # scope, in source order. The collector's branch depth when the scope
        # began separates a store under a branch of this scope from one that
        # merely sits in a function declared under a branch.
        self.stores = {}
        self.branched = {}
        self.conditional_base = 0

    def binding(self, name):
        owner = self.owner(name)
        return owner.bindings[name] if owner is not None else None

    def owner(self, name):
        current = self
        while current is not None:
            if name in current.bindings:
                return current
            current = current.parent
        return None


class Analyzer:
    def __init__(self, view, parsed_sources):
        if not hasattr(sys, "stdlib_module_names"):
            raise ValueError("Python runtime does not expose exact stdlib module authority")
        self.stdlib_modules = frozenset(sys.stdlib_module_names)
        self.files = sorted(view.get("files", []), key=lambda value: value.get("path", ""))
        self.package_rows = sorted(view.get("packages", []), key=lambda value: value.get("name", ""))
        self.namespace_packages = {row["name"] for row in self.package_rows if row.get("namespace", False)}
        self.parsed_sources = parsed_sources
        file_paths = [value.get("path", "") for value in self.files]
        if len(file_paths) != len(set(file_paths)) or set(file_paths) != set(parsed_sources):
            raise ValueError("semantic view source inventory does not match parsed sources")
        self.modules = {}
        self.module_aliases = {}
        self.objects = []
        self.objects_by_ref = {}
        self.objects_by_qname = {}
        self.qnames_by_ref = {}
        self.node_refs = {}
        self.node_scopes = {}
        self.call_result_refs = {}
        self.return_annotations = {}
        self.variable_annotations = {}
        self.return_values = {}
        self.suspended_callables = set()
        self.constructor_fields = {}
        self.field_write_counts = {}
        self.field_type_origins = {}
        self.field_type_writes = set()
        self.module_scopes = {}
        self.relations = []
        self.relations_by_key = {}

    def add_object(self, value, qname=""):
        ref = value["source_ref"]
        existing = self.objects_by_ref.get(ref)
        if existing is not None:
            if existing.get("location") is None and value.get("location") is not None:
                existing["location"] = value["location"]
            if qname:
                self.objects_by_qname[qname] = ref
                self.qnames_by_ref.setdefault(ref, qname)
            return ref
        self.objects.append(value)
        self.objects_by_ref[ref] = value
        if qname:
            self.objects_by_qname[qname] = ref
            self.qnames_by_ref.setdefault(ref, qname)
        return ref

    def ensure_external(self, name):
        name = bounded_text(name)
        # Keep one external object per public Python binding. Different import
        # spellings may expose the same binding, and splitting object identity
        # by the spelling's package prefix would create contradictory copies.
        # The top-level import name is the only package authority Python gives
        # us without importing and executing the package.
        package_path = bounded_text(name.split(".", 1)[0])
        suffix = name[len(package_path):].lstrip(".")
        parts = suffix.split(".") if suffix else [name.rsplit(".", 1)[-1]]
        symbol_name = parts[-1]
        receiver = ".".join(parts[:-1])
        ref = stable_ref("external", name)
        self.add_object({
            "source_ref": ref,
            "kind": "external_symbol",
            "name": name,
            "visibility": "unknown",
            "external": {
                "authority_kind": "platform" if package_path in self.stdlib_modules else "package",
                "package_path": package_path,
                **({"receiver": receiver} if receiver else {}),
                "name": symbol_name,
            },
        }, name)
        return ref

    def canonical_qname(self, value):
        matches = []
        for alias, canonical in self.module_aliases.items():
            if value == alias or value.startswith(alias + "."):
                matches.append((len(alias), alias, canonical))
        if not matches:
            return value
        _, alias, canonical = max(matches)
        return canonical + value[len(alias):]

    def add_relation(self, kind, from_ref, to_refs, resolution, node, witness_kind,
                     detail="", invocation="", targets_observed=None,
                     source_expression="", witness_callee=None, patterns=None,
                     patterns_observed=0, source_argument=None, extra_witnesses=()):
        path = self.current_path
        location = source_location(path, node)
        witness_location = callee_location(path, witness_callee) \
            if witness_callee is not None else location
        to_refs = sorted(set(to_refs))
        if targets_observed is None:
            targets_observed = len(to_refs) if to_refs else 1
        location_key = source_identity(path, node)
        source_argument_key = ""
        if source_argument is not None:
            source_argument_key = json.dumps(source_argument, sort_keys=True, separators=(",", ":"))
        key = (kind, from_ref, tuple(to_refs), resolution, invocation, location_key, source_argument_key)
        witness = {"kind": witness_kind}
        if detail:
            witness["detail"] = bounded_text(detail)
        if source_expression:
            witness["source_expression"] = bounded_text(source_expression)
        if witness_location is not None:
            witness["location"] = witness_location
        existing = self.relations_by_key.get(key)
        if existing is not None:
            existing["patterns_observed"] += patterns_observed
            known = {
                json.dumps(value, sort_keys=True, separators=(",", ":"))
                for value in existing["witnesses"]
            }
            for value in [witness] + list(extra_witnesses):
                candidate = json.dumps(value, sort_keys=True, separators=(",", ":"))
                if candidate not in known:
                    known.add(candidate)
                    existing["witnesses"].append(value)
                    existing["witnesses_observed"] += 1
            known_patterns = {
                value.get("source_ref", "") for value in existing.get("patterns", [])
            }
            for pattern in patterns or []:
                if pattern.get("source_ref", "") not in known_patterns:
                    existing.setdefault("patterns", []).append(pattern)
                    known_patterns.add(pattern.get("source_ref", ""))
            if source_argument is not None:
                known_source = existing.get("source_argument")
                if known_source is not None and known_source != source_argument:
                    raise ValueError("conflicting relation source argument")
                existing["source_argument"] = source_argument
            return existing["source_ref"]
        ref = stable_ref(
            "relation", kind, from_ref, "\0".join(to_refs), resolution,
            invocation, location_key, source_argument_key,
        )
        relation = {
            "source_ref": ref,
            "kind": kind,
            "from_ref": from_ref,
            "to_refs": to_refs,
            "resolution": resolution,
            "targets_observed": targets_observed,
            "witnesses": [witness] + list(extra_witnesses),
            "witnesses_observed": 1 + len(extra_witnesses),
            "patterns": list(patterns or []),
            "patterns_observed": patterns_observed,
        }
        if invocation:
            relation["invocation"] = invocation
        if location is not None:
            relation["location"] = location
        if source_argument is not None:
            relation["source_argument"] = source_argument
        self.relations.append(relation)
        self.relations_by_key[key] = relation
        return ref

    def prepare(self):
        decoded = []
        file_by_path = {value.get("path", ""): value for value in self.files}
        for row in self.package_rows:
            name = row.get("name", "")
            ref = row.get("source_ref", "")
            location = None
            if row.get("path") in file_by_path:
                location = {"path": row["path"], "line": 1, "column": 1}
            self.add_object({
                "source_ref": ref,
                "kind": "package",
                "name": name,
                "visibility": visibility(name.split(".")[-1]),
                "directory": row["directory"],
                **({"location": location} if location is not None else {}),
            }, name)

        for item in self.files:
            path = item.get("path", "")
            tree = self.parsed_sources.get(path)
            if tree is None:
                raise ValueError("module %s has no parsed source" % path)
            module = {
                "path": path,
                "name": item.get("name", ""),
                "package": bool(item.get("package")),
                "source_ref": item.get("source_ref", ""),
                "tree": tree,
            }
            names = [module["name"]] + sorted(set(item.get("aliases", [])))
            for name in names:
                previous = self.modules.get(name)
                if previous is not None and previous["source_ref"] != module["source_ref"]:
                    raise ValueError("module alias %s is ambiguous" % name)
                self.modules[name] = module
                self.module_aliases[name] = module["name"]
            decoded.append(module)
            kind = "package" if module["package"] else "module"
            module_lines = len(getattr(tree, "repomap_code_lines", ()))
            self.add_object({
                "source_ref": module["source_ref"],
                "kind": kind,
                "name": module["name"],
                "visibility": visibility(module["name"].split(".")[-1]),
                "location": {"path": path, "line": 1, "column": 1},
                **({"code_lines": module_lines} if module_lines else {}),
            }, module["name"])
            for alias in names:
                self.objects_by_qname[alias] = module["source_ref"]

        package_names = {
            name for name, ref in self.objects_by_qname.items()
            if self.objects_by_ref[ref]["kind"] == "package"
        }
        for module in decoded:
            parts = module["name"].split(".")[:-1]
            parent_ref = ""
            while parts:
                candidate = ".".join(parts)
                if candidate in package_names:
                    parent_ref = self.objects_by_qname[candidate]
                    break
                parts.pop()
            value = self.objects_by_ref[module["source_ref"]]
            if parent_ref and parent_ref != module["source_ref"]:
                value["container_ref"] = parent_ref

        for module in decoded:
            scope = Scope(module["source_ref"], module["name"], "module")
            scope.declared_all = declared_all(module["tree"])
            self.module_scopes[module["name"]] = scope
            collector = Collector(self, module, scope)
            collector.visit(module["tree"])

        for module in decoded:
            self.current_path = module["path"]
            visitor = RelationVisitor(self, module, self.module_scopes[module["name"]])
            visitor.visit(module["tree"])

        for relation in self.relations:
            values = []
            for target in relation.get("to_refs", []):
                if target not in self.suspended_callables:
                    values.extend(self.return_values.get(target, []))
                fields = self.constructor_fields.get(target)
                if fields:
                    parts = []
                    for name, field in sorted(fields.items()):
                        if self.field_write_counts.get((target, name), 0) > 1:
                            field = {**field, "parts": [{"kind": "unknown", "text": "reassigned field", "anchor": field["anchor"]}]}
                        parts.append(field)
                    owner_ref = self.objects_by_qname.get(self.object_qname(target) + ".__init__", "")
                    owner = self.objects_by_ref.get(owner_ref, {}).get("location")
                    values.append({"kind": "record", "owner": owner, "parts": parts})
            if not values:
                continue
            result = values[0] if len(values) == 1 else {"kind": "alternatives", "parts": values}
            for pattern in relation.get("patterns", []):
                pattern["result_value"] = result

        owners = {}
        for value in self.objects:
            if value.get("location") and value.get("owner_ref"):
                loc = value["location"]
                owners[(loc["path"], loc["line"], loc.get("column", 0))] = value["owner_ref"]

        def field_initializers(value, active):
            if not isinstance(value, dict) or id(value) in active:
                return value
            active = active | {id(value)}
            result = dict(value)
            if "parts" in result:
                result["parts"] = [field_initializers(part, active) for part in result["parts"]]
            if value.get("kind") == "field" and value.get("parts"):
                base = value["parts"][0]
                owner = base.get("owner", {}) if base.get("kind") == "receiver" else {}
                class_ref = owners.get((owner.get("path"), owner.get("line"), owner.get("column", 0)), "")
                field = self.constructor_fields.get(class_ref, {}).get(value.get("text"))
                if field and id(field) not in active:
                    if self.field_write_counts.get((class_ref, value["text"]), 0) > 1:
                        field = {**field, "parts": [{"kind": "unknown", "text": "reassigned field", "anchor": field["anchor"]}]}
                    result["initializer"] = field_initializers(field, active)
                    init_ref = self.objects_by_qname.get(self.object_qname(class_ref) + ".__init__", "")
                    result["owner"] = self.objects_by_ref.get(init_ref, {}).get("location")
            return result

        for relation in self.relations:
            for pattern in relation.get("patterns", []):
                for key in ("receiver_value", "result_value"):
                    if key in pattern:
                        pattern[key] = field_initializers(pattern[key], set())
                for argument in pattern.get("arguments", []):
                    if "origin" in argument:
                        argument["origin"] = field_initializers(argument["origin"], set())

        self.objects.sort(key=lambda value: value["source_ref"])
        self.relations.sort(key=lambda value: value["source_ref"])
        return {
            "objects": self.objects,
            "relations": self.relations,
        }

    def object_qname(self, ref):
        return self.qnames_by_ref.get(ref, "")


class SyntheticNode:
    def __init__(self, location):
        self.no_location = location is None
        if location is None:
            self.lineno = 1
            self.col_offset = 0
        else:
            self.lineno = location["line"]
            self.col_offset = location["column"] - 1


class Collector(ast.NodeVisitor):
    def __init__(self, analyzer, module, scope):
        self.analyzer = analyzer
        self.module = module
        self.scope = scope
        self.conditional_depth = 0
        self.statement = None

    def visit(self, node):
        if not isinstance(node, ast.stmt):
            return super().visit(node)
        previous, self.statement = self.statement, node
        try:
            return super().visit(node)
        finally:
            self.statement = previous

    def code_lines(self, first, last):
        count = code_lines_between(self.module["tree"], first, last)
        return {"code_lines": count} if count else {}

    def variable_code_lines(self):
        # A module or class variable counts the lines of the assignment that
        # binds it; a local variable follows its callable.
        statement = self.statement
        if self.scope.kind not in ("module", "type") or not isinstance(statement, (ast.Assign, ast.AnnAssign, ast.AugAssign)):
            return {}
        return self.code_lines(statement.lineno, getattr(statement, "end_lineno", statement.lineno))

    def record_export(self, name, binding=None):
        if self.scope.kind != "module":
            return
        # Re-exports follow one unconditional written binding. A second write
        # or a conditional import does not establish a module member's value.
        if name in self.scope.export_bindings or self.conditional_depth:
            self.scope.export_bindings[name] = None
        else:
            self.scope.export_bindings[name] = binding or {"kind": "declaration"}

    def generic_visit(self, node):
        # A comprehension decides whether an assignment expression inside it
        # runs; the other branches below keep their condition unconditional.
        conditional = isinstance(node, (ast.ListComp, ast.SetComp, ast.DictComp, ast.GeneratorExp))
        self.conditional_depth += int(conditional)
        super().generic_visit(node)
        self.conditional_depth -= int(conditional)

    def visit_branches(self, first, branches):
        # The condition, the subject or the first operand always runs, as
        # the C adapter walks an if's condition and the left of && and ||;
        # what follows runs only when it decides so.
        for value in first:
            self.visit(value)
        self.conditional_depth += 1
        for value in branches:
            self.visit(value)
        self.conditional_depth -= 1

    def visit_If(self, node):
        self.visit_branches([node.test], node.body + node.orelse)

    visit_While = visit_If

    def visit_IfExp(self, node):
        self.visit_branches([node.test], [node.body, node.orelse])

    def visit_BoolOp(self, node):
        self.visit_branches(node.values[:1], node.values[1:])

    def visit_Match(self, node):
        self.visit_branches([node.subject], node.cases)

    def visit_Try(self, node):
        # A finally body runs whenever the statement after the try does.
        self.visit_branches([], node.body + node.handlers + node.orelse)
        for statement in node.finalbody:
            self.visit(statement)

    visit_TryStar = visit_Try

    def object_ref(self, kind, qname, node):
        return stable_ref(
            "object", self.module["source_ref"], kind, qname,
            str(getattr(node, "lineno", 0)), str(getattr(node, "col_offset", -1) + 1),
        )

    def add_variable(self, name, node, forced_internal=False, signature=""):
        if not name:
            return ""
        self.record_export(name)
        qname = self.scope.qname + "." + name
        ref = self.object_ref("variable", qname, node)
        self.analyzer.add_object({
            "source_ref": ref,
            "kind": "variable",
            "name": name,
            "visibility": visibility(name, forced_internal or self.scope.kind in ("function", "method", "lambda"), self.scope),
            **({"owner_ref": self.scope.ref} if self.scope.kind == "type" else {}),
            **({"signature": signature} if signature else {}),
            "container_ref": self.scope.ref,
            "location": source_location(self.module["path"], node),
            **self.variable_code_lines(),
        }, qname)
        self.scope.bindings[name] = {"kind": "object", "ref": ref}
        self.analyzer.node_refs[id(node)] = ref
        return ref

    def bind_targets(self, target, forced_internal=False):
        if isinstance(target, ast.Name):
            self.add_variable(target.id, target, forced_internal)
        elif isinstance(target, (ast.Tuple, ast.List)):
            for value in target.elts:
                self.bind_targets(value, forced_internal)
        elif isinstance(target, ast.Attribute) and isinstance(target.value, ast.Name) and target.value.id == "self" and self.scope.class_ref:
            # This is an observed instance-field assignment, not an inferred
            # runtime receiver type. Retain the field slot so constructor
            # results and later calls on that exact field can stay connected.
            qname = self.scope.class_qname + "." + target.attr
            ref = self.analyzer.objects_by_qname.get(qname, "")
            if not ref:
                ref = self.object_ref("variable", qname, target)
                self.analyzer.add_object({
                    "source_ref": ref, "kind": "variable", "name": target.attr,
                    "visibility": visibility(target.attr), "owner_ref": self.scope.class_ref,
                    "container_ref": self.scope.class_ref, "location": source_location(self.module["path"], target),
                }, qname)
            self.analyzer.node_refs[id(target)] = ref

    def callable_alias_binding(self, value):
        if not isinstance(value, ast.Name):
            return None
        binding = self.scope.binding(value.id)
        if binding is None:
            return None
        if binding["kind"] in ("module", "from"):
            # Imported names are request-local structural candidates. Keeping
            # the binding through a plain assignment lets the relation pass
            # retain that candidate without claiming immutable Python runtime
            # identity.
            return dict(binding)
        if binding["kind"] != "object":
            return None
        candidate = self.analyzer.objects_by_ref.get(binding.get("ref", ""))
        if candidate is None or candidate["kind"] not in ("function", "method", "lambda", "type"):
            return None
        return dict(binding)

    def bind_callable_alias(self, target, binding):
        if binding is not None and isinstance(target, ast.Name):
            self.scope.bindings[target.id] = dict(binding)

    def record_store(self, target, binding=None, value=None):
        # One assignment of a name, with the callable its value names. Under a
        # branch, the name afterwards holds whatever the branch left there.
        if isinstance(target, ast.Name):
            self.scope.stores.setdefault(target.id, []).append({
                "binding": dict(binding) if binding is not None else None,
                "conditional": self.conditional_depth > self.scope.conditional_base,
                "node": value if binding is not None and value is not None else target,
            })
        elif isinstance(target, (ast.Tuple, ast.List)):
            for element in target.elts:
                self.record_store(element)

    def record_declaration(self, name, binding, node):
        # A def, class or import binds the name too. It is one more value the
        # name may hold, never what makes an assigned name conditional.
        self.scope.stores.setdefault(name, []).append({
            "binding": dict(binding),
            "conditional": self.conditional_depth > self.scope.conditional_base,
            "node": node, "declaration": True,
        })

    def ensure_call_result(self, node):
        existing = self.analyzer.call_result_refs.get(id(node), "")
        if existing:
            return existing
        location = source_location(self.module["path"], node)
        ref = stable_ref(
            "call-result", self.module["source_ref"],
            source_identity(self.module["path"], node),
        )
        self.analyzer.add_object({
            "source_ref": ref,
            "kind": "variable",
            "name": "call result",
            "visibility": "internal",
            "container_ref": self.scope.ref,
            **({"location": location} if location is not None else {}),
        })
        self.analyzer.call_result_refs[id(node)] = ref
        return ref

    def visit_Call(self, node):
        # A chained call consumes the exact syntactic value produced by its
        # receiver call. Retain that value without assigning any framework or
        # runtime meaning to either selector.
        if isinstance(node.func, ast.Attribute) and isinstance(node.func.value, ast.Call):
            self.ensure_call_result(node.func.value)
        for argument in list(node.args) + [value.value for value in node.keywords]:
            if isinstance(argument, ast.Call):
                self.ensure_call_result(argument)
        self.generic_visit(node)

    def visit_FunctionDef(self, node):
        self._visit_function(node)

    def visit_AsyncFunctionDef(self, node):
        self._visit_function(node)

    def _visit_function(self, node):
        parent = self.scope
        self.record_export(node.name)
        kind = "method" if parent.kind == "type" else "function"
        qname = parent.qname + "." + node.name
        ref = self.object_ref(kind, qname, node)
        owner_ref = parent.ref if kind == "method" else ""
        self.analyzer.add_object({
            "source_ref": ref,
            "kind": kind,
            "name": node.name,
            "visibility": visibility(node.name, scope=parent),
            "signature": function_signature(node),
            **({"owner_ref": owner_ref} if owner_ref else {}),
            "container_ref": parent.ref,
            "location": source_location(self.module["path"], node),
            "end_line": getattr(node, "end_lineno", 0),
            **self.code_lines(node.lineno, getattr(node, "end_lineno", node.lineno)),
        }, qname)
        parent.bindings[node.name] = {"kind": "object", "ref": ref}
        self.record_declaration(node.name, parent.bindings[node.name], node)
        self.analyzer.node_refs[id(node)] = ref
        if isinstance(node, ast.AsyncFunctionDef):
            self.analyzer.suspended_callables.add(ref)
        if node.returns is not None:
            self.analyzer.return_annotations[ref] = (node.returns, parent)
        for value in list(node.decorator_list) + list(node.args.defaults) + list(node.args.kw_defaults):
            if value is not None:
                self.visit(value)
        child = Scope(
            ref, qname, kind, parent,
            class_ref=parent.ref if kind == "method" else parent.class_ref,
            class_qname=parent.qname if kind == "method" else parent.class_qname,
        )
        child.conditional_base = self.conditional_depth
        self.analyzer.node_scopes[id(node)] = child
        previous, self.scope = self.scope, child
        arguments = list(node.args.posonlyargs) + list(node.args.args) + list(node.args.kwonlyargs)
        if node.args.vararg is not None:
            arguments.append(node.args.vararg)
        if node.args.kwarg is not None:
            arguments.append(node.args.kwarg)
        for argument in arguments:
            self.add_variable(argument.arg, argument, True)
            if argument.annotation is not None:
                self.analyzer.variable_annotations[self.analyzer.node_refs[id(argument)]] = (argument.annotation, parent)
        for statement in node.body:
            self.visit(statement)
        self.scope = previous

    def visit_ClassDef(self, node):
        parent = self.scope
        self.record_export(node.name)
        qname = parent.qname + "." + node.name
        ref = self.object_ref("type", qname, node)
        self.analyzer.add_object({
            "source_ref": ref,
            "kind": "type",
            "name": node.name,
            "visibility": visibility(node.name, scope=parent),
            "signature": class_signature(node),
            **({"owner_ref": parent.ref} if parent.kind == "type" else {}),
            "container_ref": parent.ref,
            "location": source_location(self.module["path"], node),
            "end_line": getattr(node, "end_lineno", 0),
            **self.code_lines(node.lineno, getattr(node, "end_lineno", node.lineno)),
        }, qname)
        parent.bindings[node.name] = {"kind": "object", "ref": ref}
        self.record_declaration(node.name, parent.bindings[node.name], node)
        self.analyzer.node_refs[id(node)] = ref
        for value in list(node.decorator_list) + list(node.bases) + [keyword.value for keyword in node.keywords]:
            self.visit(value)
        child = Scope(ref, qname, "type", parent, class_ref=ref, class_qname=qname)
        child.conditional_base = self.conditional_depth
        self.analyzer.node_scopes[id(node)] = child
        previous, self.scope = self.scope, child
        for statement in node.body:
            self.visit(statement)
        self.scope = previous

    def visit_Lambda(self, node):
        parent = self.scope
        name = "lambda@%d:%d" % (node.lineno, node.col_offset + 1)
        qname = parent.qname + "." + name
        ref = self.object_ref("lambda", qname, node)
        self.analyzer.add_object({
            "source_ref": ref,
            "kind": "lambda",
            "name": name,
            "visibility": "internal",
            "signature": ("lambda " + declaration_arguments(node.args)).rstrip(),
            "container_ref": parent.ref,
            "location": source_location(self.module["path"], node),
        }, qname)
        self.analyzer.node_refs[id(node)] = ref
        child = Scope(ref, qname, "lambda", parent, class_ref=parent.class_ref, class_qname=parent.class_qname)
        child.conditional_base = self.conditional_depth
        self.analyzer.node_scopes[id(node)] = child
        previous, self.scope = self.scope, child
        for argument in list(node.args.posonlyargs) + list(node.args.args) + list(node.args.kwonlyargs):
            self.add_variable(argument.arg, argument, True)
        self.visit(node.body)
        self.scope = previous

    def visit_Assign(self, node):
        alias_binding = self.callable_alias_binding(node.value)
        self.visit(node.value)
        for target in node.targets:
            if self.scope.kind == "type" and isinstance(target, ast.Name):
                self.add_variable(target.id, target, signature=target.id + " = " + ast.unparse(node.value))
            else:
                self.bind_targets(target)
            self.bind_callable_alias(target, alias_binding)
        if isinstance(node.value, ast.Call) and len(node.targets) == 1:
            ref = self.analyzer.node_refs.get(id(node.targets[0]), "")
            if ref:
                self.analyzer.call_result_refs[id(node.value)] = ref
        stored = alias_binding
        if isinstance(node.value, ast.Lambda):
            lambda_ref = self.analyzer.node_refs.get(id(node.value), "")
            if lambda_ref:
                stored = {"kind": "object", "ref": lambda_ref}
                for target in node.targets:
                    if isinstance(target, ast.Name):
                        self.scope.bindings[target.id] = {"kind": "object", "ref": lambda_ref}
        for target in node.targets:
            self.record_store(target, stored, node.value)

    def visit_AnnAssign(self, node):
        alias_binding = self.callable_alias_binding(node.value) if node.value is not None else None
        if node.value is not None:
            self.visit(node.value)
        if isinstance(node.target, ast.Name):
            signature = node.target.id + ": " + ast.unparse(node.annotation)
            if node.value is not None and self.scope.kind in ("module", "type"):
                signature += " = " + ast.unparse(node.value)
            self.add_variable(node.target.id, node.target, signature=signature)
            if isinstance(node.value, ast.Call):
                self.analyzer.call_result_refs[id(node.value)] = self.analyzer.node_refs[id(node.target)]
        else:
            self.bind_targets(node.target)
            if isinstance(node.value, ast.Call):
                ref = self.analyzer.node_refs.get(id(node.target), "")
                if ref:
                    self.analyzer.call_result_refs[id(node.value)] = ref
        self.bind_callable_alias(node.target, alias_binding)
        if node.value is not None:
            self.record_store(node.target, alias_binding, node.value)
        ref = self.analyzer.node_refs.get(id(node.target))
        if ref:
            self.analyzer.variable_annotations[ref] = (node.annotation, self.scope)

    def visit_NamedExpr(self, node):
        alias_binding = self.callable_alias_binding(node.value)
        self.visit(node.value)
        self.bind_targets(node.target, True)
        self.bind_callable_alias(node.target, alias_binding)
        self.record_store(node.target, alias_binding, node.value)

    def visit_For(self, node):
        self.visit(node.iter)
        self.conditional_depth += 1
        self.bind_targets(node.target, True)
        for statement in node.body + node.orelse:
            self.visit(statement)
        self.conditional_depth -= 1

    visit_AsyncFor = visit_For

    def visit_AugAssign(self, node):
        if isinstance(node.target, ast.Name):
            self.record_export(node.target.id)
        self.generic_visit(node)

    def visit_Delete(self, node):
        for target in node.targets:
            for child in ast.walk(target):
                if isinstance(child, ast.Name):
                    self.record_export(child.id)
        self.generic_visit(node)

    def visit_With(self, node):
        for item in node.items:
            if item.optional_vars is not None:
                for target in ast.walk(item.optional_vars):
                    if isinstance(target, ast.Name):
                        self.record_export(target.id)
                        self.scope.opaque_names.add(target.id)
        # A context manager may suppress an exception raised before a store
        # in its body.
        self.visit_branches(node.items, node.body)

    visit_AsyncWith = visit_With

    def visit_ExceptHandler(self, node):
        if node.name:
            self.record_export(node.name)
            self.scope.opaque_names.add(node.name)
        self.generic_visit(node)

    def visit_MatchAs(self, node):
        if node.name:
            self.record_export(node.name)
            self.scope.opaque_names.add(node.name)
        self.generic_visit(node)

    visit_MatchStar = visit_MatchAs

    def visit_MatchMapping(self, node):
        if node.rest:
            self.record_export(node.rest)
            self.scope.opaque_names.add(node.rest)
        self.generic_visit(node)

    def visit_Global(self, node):
        self.scope.global_names.update(node.names)

    def visit_Nonlocal(self, node):
        self.scope.nonlocal_names.update(node.names)

    def visit_Import(self, node):
        for alias in node.names:
            bound = alias.asname or alias.name.split(".")[0]
            module_name = alias.name if alias.asname else alias.name.split(".")[0]
            external = module_name not in self.analyzer.modules and module_name not in self.analyzer.objects_by_qname
            if external:
                self.analyzer.ensure_external(module_name)
            binding = {
                "kind": "module", "module": module_name, "external": external,
            }
            self.record_export(bound, binding)
            self.scope.bindings[bound] = binding
            self.record_declaration(bound, binding, alias if getattr(alias, "lineno", 0) else node)

    def visit_ImportFrom(self, node):
        base = relative_module(self.module["name"], self.module["package"], node.level, node.module)
        for alias in node.names:
            if alias.name == "*":
                if self.scope.kind == "module":
                    self.scope.export_star_import = True
                continue
            name = alias.asname or alias.name
            binding = {
                "kind": "from", "module": base, "name": alias.name,
                "relative": node.level > 0,
            }
            self.record_export(name, binding)
            self.scope.bindings[name] = binding
            self.record_declaration(name, binding, alias if getattr(alias, "lineno", 0) else node)


class RelationVisitor(ast.NodeVisitor):
    def __init__(self, analyzer, module, scope):
        self.analyzer = analyzer
        self.module = module
        self.scope = scope
        self.invocation = ""
        # Pattern receiver provenance is deliberately source ordered. The
        # declaration collector has already created stable variable objects,
        # but it must not let a later assignment explain an earlier call.
        self.pattern_bindings = {id(scope): {}}
        self.read_shadows = set()

    def object(self, ref):
        return self.analyzer.objects_by_ref.get(ref)

    def import_target(self, module_name, imported_name="", allow_external=True, seen=None):
        qname = module_name + (("." + imported_name) if imported_name else "")
        canonical_module = self.analyzer.canonical_qname(module_name)
        scope = self.analyzer.module_scopes.get(canonical_module)
        if imported_name and scope is not None:
            key = (canonical_module, imported_name)
            seen = set() if seen is None else seen
            if key in seen or scope.export_star_import:
                return "unknown", ""
            binding = scope.export_bindings.get(imported_name)
            if imported_name in scope.export_bindings and binding is None:
                return "unknown", ""
            if binding and binding["kind"] in ("from", "module"):
                # `from . import child` names an indexed child module, not a
                # cycle through the package's own member binding.
                if binding["kind"] == "from" and binding["module"] == canonical_module and binding["name"] == imported_name and qname in self.analyzer.modules:
                    return "local", self.analyzer.modules[qname]["source_ref"]
                return self.import_target(
                    binding["module"], binding.get("name", ""),
                    allow_external=not binding.get("relative", False), seen=seen | {key},
                )
        if qname in self.analyzer.objects_by_qname:
            ref = self.analyzer.objects_by_qname[qname]
            value = self.object(ref)
            return ("external" if value and value["kind"] == "external_symbol" else "local"), ref
        if qname in self.analyzer.modules:
            return "local", self.analyzer.modules[qname]["source_ref"]
        canonical = self.analyzer.canonical_qname(qname)
        if canonical in self.analyzer.objects_by_qname:
            ref = self.analyzer.objects_by_qname[canonical]
            value = self.object(ref)
            return ("external" if value and value["kind"] == "external_symbol" else "local"), ref
        if canonical in self.analyzer.modules:
            return "local", self.analyzer.modules[canonical]["source_ref"]
        base_ref = self.analyzer.objects_by_qname.get(module_name, "")
        base_object = self.object(base_ref) if base_ref else None
        local_base = module_name in self.analyzer.modules or \
            (base_object is not None and base_object["kind"] != "external_symbol")
        if local_base:
            return "unknown", ""
        if not allow_external and not self.namespace_external_import(module_name):
            return "unknown", ""
        return "external", self.analyzer.ensure_external(qname)

    def imported_attribute(self, module_name, parts, allow_external=True):
        authority, ref = self.import_target(module_name, parts[0], allow_external)
        for part in parts[1:]:
            value = self.object(ref) if ref else None
            if not value:
                return "unknown", ""
            if value["kind"] in ("module", "package", "external_symbol"):
                authority, ref = self.import_target(self.analyzer.object_qname(ref), part, authority == "external")
            elif value["kind"] == "type":
                ref = self.analyzer.objects_by_qname.get(self.analyzer.object_qname(ref) + "." + part, "")
                authority = "local" if ref else "unknown"
            else:
                return "unknown", ""
        return authority, ref

    def namespace_external_import(self, module_name):
        """A declared namespace may have portions outside this parser view.

        A nearer ordinary module/package still owns its own missing children;
        reaching a namespace above it does not supply their import authority.
        """
        name = self.analyzer.canonical_qname(module_name)
        while name:
            ref = self.analyzer.objects_by_qname.get(name, "")
            value = self.object(ref) if ref else None
            if value is not None and value["kind"] in ("module", "package"):
                return name in self.analyzer.namespace_packages
            name = name.rpartition(".")[0]
        return False

    def local_import_target(self, module_name):
        """Resolve only an already catalogued local module, without mutation."""
        if module_name in self.analyzer.objects_by_qname:
            return self.analyzer.objects_by_qname[module_name]
        if module_name in self.analyzer.modules:
            return self.analyzer.modules[module_name]["source_ref"]
        canonical = self.analyzer.canonical_qname(module_name)
        if canonical in self.analyzer.objects_by_qname:
            return self.analyzer.objects_by_qname[canonical]
        if canonical in self.analyzer.modules:
            return self.analyzer.modules[canonical]["source_ref"]
        return ""

    # A signature's values with the repository class each carries: the
    # annotation as written, and the class it names once containers such as
    # list[X] and Optional[X] are opened.
    def typed_parameters(self, node, kind):
        arguments = node.args
        values = list(arguments.posonlyargs) + list(arguments.args)
        if arguments.vararg is not None:
            values.append(arguments.vararg)
        values.extend(arguments.kwonlyargs)
        if arguments.kwarg is not None:
            values.append(arguments.kwarg)
        result = []
        for position, argument in enumerate(values):
            if position == 0 and kind == "method" and argument.arg in ("self", "cls"):
                continue
            result.append(self.typed_value(argument.arg, argument.annotation))
        return result

    def typed_results(self, node):
        if node.returns is None:
            return []
        return [self.typed_value("", node.returns)]

    def typed_value(self, name, annotation):
        value = {"name": name}
        if annotation is None:
            return value
        value["type"] = ast.unparse(annotation)
        carried = annotation
        while isinstance(carried, ast.Subscript):
            carried = carried.slice
            if isinstance(carried, ast.Tuple) and carried.elts:
                carried = carried.elts[0]
        if isinstance(carried, (ast.Name, ast.Attribute)):
            origin, ref = self.resolve(carried)
            target = self.object(ref) if ref else None
            if origin == "local" and target and target["kind"] == "type":
                value["type_ref"] = ref
        return value

    def binding_target(self, binding):
        if binding["kind"] == "object":
            ref = binding["ref"]
            value = self.object(ref)
            return ("external" if value and value["kind"] == "external_symbol" else "local", ref)
        if binding["kind"] == "module":
            return self.import_target(binding["module"])
        if binding["kind"] == "from":
            return self.import_target(
                binding["module"], binding["name"],
                allow_external=not binding.get("relative", False),
            )
        return "unknown", ""

    def branch_stores(self, name):
        """The stores of an assigned name in the scope that owns it, when one
        of them is under a branch; otherwise none. A def, class or import
        under a branch counts once the name is also assigned there."""
        owner = self.scope.owner(name)
        if owner is None:
            return []
        if name not in owner.branched:
            stores = owner.stores.get(name, [])
            assigned = any(not store.get("declaration") for store in stores)
            owner.branched[name] = stores if assigned and any(store["conditional"] for store in stores) else []
        return owner.branched[name]

    def stored_function_witnesses(self, node):
        # A call through a name assigned under a branch names each function
        # those assignments store, as the C adapter names a pointer's stores.
        # A call of an attribute of that name names each module or class
        # stored in it.
        root = node
        while isinstance(root, ast.Attribute):
            root = root.value
        if not isinstance(root, ast.Name):
            return []
        kinds = ("function", "method", "lambda", "type", "external_symbol")
        if root is not node:
            kinds += ("module", "package")
        witnesses = []
        for store in self.branch_stores(root.id):
            if store["binding"] is None:
                continue
            authority, ref = self.binding_target(store["binding"])
            candidate = self.object(ref) if ref else None
            if authority == "unknown" or candidate is None or candidate["kind"] not in kinds:
                continue
            detail = candidate["name"] + " stored in " + root.id
            if store["conditional"]:
                detail += " under a condition"
            witness = {"kind": "function_value_store", "detail": bounded_text(detail)}
            location = source_location(self.module["path"], store["node"])
            if location is not None:
                witness["location"] = location
            witnesses.append(witness)
        return witnesses

    def resolve(self, node):
        if isinstance(node, ast.Lambda):
            ref = self.analyzer.node_refs.get(id(node), "")
            # A literal lambda expression is the one callable whose exact
            # declaration object is established by this callsite itself.
            return ("literal", ref) if ref else ("unknown", "")
        if isinstance(node, ast.Name):
            binding = self.scope.binding(node.id)
            if binding is None:
                qname = self.module["name"] + "." + node.id
                ref = self.analyzer.objects_by_qname.get(qname, "")
                return ("local", ref) if ref else ("unknown", "")
            resolved = self.binding_target(binding)
            value = self.object(resolved[1]) if resolved[1] else None
            if value and value["kind"] != "variable" and self.branch_stores(node.id):
                # The last assignment is only one of the values a branch
                # leaves in the name; its declared slot stays a variable.
                return "unknown", ""
            return resolved
        if isinstance(node, ast.Attribute):
            parts = []
            current = node
            while isinstance(current, ast.Attribute):
                parts.append(current.attr)
                current = current.value
            parts.reverse()
            if isinstance(current, ast.Name) and current.id == "self" and self.scope.class_qname:
                qname = self.scope.class_qname + "." + ".".join(parts)
                ref = self.analyzer.objects_by_qname.get(qname, "")
                if not ref and len(parts) > 1:
                    field_ref = self.analyzer.objects_by_qname.get(self.scope.class_qname + "." + parts[0], "")
                    owner_ref = self.analyzer.field_type_origins.get(field_ref, "")
                    if owner_ref:
                        owner_name = self.analyzer.object_qname(owner_ref)
                        ref = self.analyzer.objects_by_qname.get(owner_name + "." + ".".join(parts[1:]), "")
                return ("local", ref) if ref else ("unknown", "")
            if isinstance(current, ast.Name):
                binding = self.scope.binding(current.id)
                if binding and binding["kind"] == "module":
                    if self.branch_stores(current.id):
                        return "unknown", ""
                    return self.imported_attribute(binding["module"], parts, binding.get("external", False))
                value_binding = self.pattern_binding(current.id)
                typed_parameter = value_binding and value_binding.get("annotation_origin") and len(parts) == 1
                origins = value_binding.get("origin_refs", []) if value_binding and (not value_binding.get("value_invalidated") or typed_parameter) else []
                if len(origins) == 1:
                    owner_ref = self.produced_class(origins[0])
                    if owner_ref:
                        qname = self.analyzer.object_qname(owner_ref)
                        target = self.analyzer.objects_by_qname.get(qname + "." + ".".join(parts), "")
                        if target:
                            return "local", target
                base_kind, base_ref = self.resolve(current)
                base = self.object(base_ref) if base_ref else None
                if base_kind == "local" and base and base["kind"] in ("module", "package"):
                    return self.imported_attribute(self.analyzer.object_qname(base_ref), parts, False)
                if base_kind == "local" and base and base["kind"] in ("module", "package", "type"):
                    for qname, ref in self.analyzer.objects_by_qname.items():
                        if ref == base_ref:
                            target = self.analyzer.objects_by_qname.get(qname + "." + ".".join(parts), "")
                            return ("local", target) if target else ("unknown", "")
                if base_kind == "external" and base:
                    return "external", self.analyzer.ensure_external(base["name"] + "." + ".".join(parts))
            return "unknown", ""
        return "unknown", ""

    def expression_name(self, node):
        if isinstance(node, ast.Name):
            binding = self.scope.binding(node.id)
            if self.branch_stores(node.id):
                return node.id
            if binding and binding["kind"] == "module":
                return binding["module"]
            if binding and binding["kind"] == "from":
                return binding["module"] + "." + binding["name"]
            return node.id
        return safe_expression_name(node)

    def pattern_selector(self, node):
        if isinstance(node, ast.Name):
            value = node.id
        elif isinstance(node, ast.Attribute):
            value = node.attr
        else:
            return ""
        return value

    def pattern_resolution(self, resolved):
        authority, ref = resolved
        if not ref:
            return "unresolved", []
        if authority == "literal":
            return "exact", [ref]
        if authority in ("local", "external"):
            return "exact", [ref]
        return "unresolved", []

    def resolved_call_target(self, node):
        resolved = self.resolve(node)
        if resolved[0] == "local" and resolved[1]:
            value = self.object(resolved[1])
            if value is None or value["kind"] not in ("function", "method", "lambda", "type"):
                return "unknown", ""
        if resolved[0] == "unknown" and isinstance(node, ast.Attribute) and isinstance(node.value, ast.Name):
            # `session.exec(...)` on a parameter annotated with an outside
            # class is that class's method: the annotation names the origin.
            binding = self.pattern_binding(node.value.id)
            origins = [ref for ref in (binding or {}).get("origin_refs", []) if (self.object(ref) or {}).get("kind") == "external_symbol"]
            if binding and len(origins) == 1 and (binding.get("origin_resolution") or "exact") == "exact":
                origin = self.object(origins[0])
                return "external", self.analyzer.ensure_external(origin["name"] + "." + node.attr)
        return resolved

    def current_pattern_bindings(self):
        return self.pattern_bindings.setdefault(id(self.scope), {})

    def pattern_binding(self, name):
        current = self.scope
        while current is not None:
            value = self.pattern_bindings.get(id(current), {}).get(name)
            if value is not None:
                return value
            current = current.parent
        return None

    def bind_pattern_name(self, node, origin, initializer=None, source_origin=None):
        if not isinstance(node, ast.Name):
            return
        ref = self.analyzer.node_refs.get(id(node), "")
        current = self.current_pattern_bindings()
        previous = current.get(node.id)
        reassigned = previous is not None and previous.get("binding_observed", False)
        invalidated = reassigned or bool(previous and previous.get("value_invalidated", False))
        value_candidate = None
        if not invalidated and initializer is not None and ref:
            value_candidate = dict(initializer)
            value_candidate["source_object_refs"] = [ref]
            value_candidate["source_objects_observed"] = 1
        # Reassigning a name permanently clears initializer-to-use value
        # authority in this lexical scope. Python names are mutable and a
        # later simple literal assignment must not erase the earlier write.
        self.current_pattern_bindings()[node.id] = {
            "ref": ref,
            "origin_refs": list(origin.get("refs", [])),
            "origin_resolution": origin.get("resolution", ""),
            "origins_observed": origin.get("observed", 0),
            "binding_observed": True,
            "value_invalidated": invalidated,
            "value_candidate": value_candidate,
            "source_origin": source_origin if not invalidated else None,
        }

    def bind_pattern_target(self, target, origin, initializer=None, source_origin=None):
        if isinstance(target, ast.Name):
            self.bind_pattern_name(target, origin, initializer, source_origin)
        elif isinstance(target, (ast.Tuple, ast.List)):
            for value in target.elts:
                self.bind_pattern_target(value, {"observed": 0})

    def bind_nonvalue_name(self, name, ref=""):
        if not name:
            return
        previous = self.current_pattern_bindings().get(name)
        self.current_pattern_bindings()[name] = {
            "ref": ref,
            "origin_refs": [],
            "origin_resolution": "",
            "origins_observed": 0,
            "binding_observed": True,
            "value_invalidated": True,
            "value_candidate": None,
            **({"previous_binding": True} if previous is not None else {}),
        }

    def assignment_origin(self, value):
        if not isinstance(value, ast.Call):
            return {"observed": 0}
        resolution, refs = self.pattern_resolution(self.resolved_call_target(value.func))
        return {"observed": 1, "resolution": resolution, "refs": refs}

    def bind_field_type(self, target, value):
        if not isinstance(target, ast.Attribute) or not isinstance(target.value, ast.Name) or target.value.id != "self":
            return
        ref = self.analyzer.node_refs.get(id(target), "")
        if not ref:
            return
        owner = self.object(self.scope.ref)
        if owner and owner.get("name") == "__init__":
            self.analyzer.constructor_fields.setdefault(self.scope.class_ref, {})[target.attr] = {
                "kind": "field_value", "text": target.attr,
                "anchor": source_location(self.module["path"], target),
                "parts": [self.source_value(value)],
            }
        if ref in self.analyzer.field_type_writes:
            self.analyzer.field_type_origins.pop(ref, None)
            return
        self.analyzer.field_type_writes.add(ref)
        if not isinstance(value, ast.Call):
            return
        authority, called_ref = self.resolved_call_target(value.func)
        type_ref = self.produced_class(called_ref) if authority == "local" else ""
        if type_ref:
            self.analyzer.field_type_origins[ref] = type_ref

    def produced_class(self, called_ref):
        called = self.object(called_ref) if called_ref else None
        if called and called["kind"] == "type":
            return called_ref
        annotation = self.analyzer.return_annotations.get(called_ref)
        if annotation is None:
            return ""
        expression, declared_scope = annotation
        previous, self.scope = self.scope, declared_scope
        try:
            authority, type_ref = self.resolve(expression)
        finally:
            self.scope = previous
        candidate = self.object(type_ref) if type_ref else None
        if authority == "local" and candidate and candidate["kind"] == "type":
            # An explicit return annotation offers a possible receiver class;
            # it never proves exact runtime dispatch or a final field value.
            return type_ref
        return ""

    def iterable_element_type(self, annotation):
        if annotation is None:
            return ""
        expression, declared_scope = annotation
        if not isinstance(expression, ast.Subscript):
            return ""
        previous, self.scope = self.scope, declared_scope
        try:
            container = expression.value
            builtin = (isinstance(container, ast.Name) and self.scope.binding(container.id) is None
                       and container.id in ("list", "set", "frozenset", "tuple"))
            authority, ref = self.resolve(container)
            external = self.object(ref) if authority == "external" else None
            generic = external and external["name"] in (
                "typing.List", "typing.Set", "typing.FrozenSet", "typing.Tuple",
                "typing.Sequence", "typing.Iterable", "typing.Iterator",
                "collections.abc.Sequence", "collections.abc.Iterable", "collections.abc.Iterator",
            )
            if not builtin and not generic:
                return ""
            element = expression.slice
            if isinstance(element, ast.Tuple):
                # Only homogeneous tuple[T, ...]; a heterogeneous tuple or
                # a mapping must not borrow its first member as every item.
                is_tuple = (builtin and container.id == "tuple") or (external and external["name"] == "typing.Tuple")
                if not is_tuple or len(element.elts) != 2 or not isinstance(element.elts[1], ast.Constant) or element.elts[1].value is not Ellipsis:
                    return ""
                element = element.elts[0]
            if not isinstance(element, (ast.Name, ast.Attribute)):
                return ""
            authority, ref = self.resolve(element)
            candidate = self.object(ref) if authority == "local" else None
            return ref if candidate and candidate["kind"] == "type" else ""
        finally:
            self.scope = previous

    def iterated_class(self, expression):
        if isinstance(expression, ast.Call):
            # sorted preserves element types. A local parameter/import named
            # sorted, an arbitrary wrapper or star expansion proves nothing.
            if (isinstance(expression.func, ast.Name) and expression.func.id == "sorted"
                    and self.scope.binding("sorted") is None and "sorted" not in self.read_shadows
                    and len(expression.args) == 1 and not isinstance(expression.args[0], ast.Starred)
                    and all(k.arg in ("key", "reverse") for k in expression.keywords)):
                return self.iterated_class(expression.args[0])
            return ""
        if isinstance(expression, ast.Name):
            binding = self.pattern_binding(expression.id)
            if binding and self.read_name(expression.id)[1] == binding.get("ref"):
                return self.iterable_element_type(binding.get("iterable_annotation"))
        if isinstance(expression, ast.Attribute):
            _, ref = self.written_field(expression)
            field = self.object(ref)
            if field and not self.analyzer.field_write_counts.get((field.get("owner_ref"), field["name"])):
                return self.iterable_element_type(self.analyzer.variable_annotations.get(ref))
        return ""

    def pattern_receiver(self, callee):
        if not isinstance(callee, ast.Attribute):
            return {}
        if isinstance(callee.value, ast.Call):
            ref = self.analyzer.call_result_refs.get(id(callee.value), "")
            return {"receiver_ref": ref} if ref else {}
        binding = self.pattern_binding(callee.value.id) if isinstance(callee.value, ast.Name) else None
        if binding is None:
            authority, ref = self.resolve(callee.value)
            return {"receiver_ref": ref} if authority == "local" and ref else {}
        result = {
            "receiver_origins_observed": binding.get("origins_observed", 0),
        }
        if binding.get("ref"):
            result["receiver_ref"] = binding["ref"]
        elif isinstance(callee.value, ast.Name):
            # `from app.api import bp` binds the name here; the value is the
            # module-level declaration it was imported from.
            authority, ref = self.resolve(callee.value)
            if authority == "local" and ref:
                result["receiver_ref"] = ref
        if binding.get("origin_refs"):
            result["receiver_origin_refs"] = list(binding["origin_refs"])
        if binding.get("origin_resolution"):
            result["receiver_origin_resolution"] = binding["origin_resolution"]
        return result

    def pattern_argument_authority(self, node):
        if isinstance(node, ast.Call):
            ref = self.analyzer.call_result_refs.get(id(node), "")
            return {"object_refs": [ref], "resolution": "exact", "objects_observed": 1} if ref else {"objects_observed": 0}
        resolved = self.resolve(node)
        candidate = self.object(resolved[1]) if resolved[1] else None
        # A lexical callable alias is the same possible declaration used by
        # passes_callback. The assignment variable remains its own object, but
        # must not replace that candidate in the argument the transfer cites.
        if resolved[0] in ("local", "literal") and candidate and candidate["kind"] in ("function", "method", "lambda"):
            resolution, refs = self.pattern_resolution(resolved)
            return {"object_refs": refs, "resolution": resolution, "objects_observed": 1}
        if isinstance(node, ast.Name):
            binding = self.pattern_binding(node.id)
            if binding is not None and binding.get("ref"):
                return {
                    "object_refs": [binding["ref"]],
                    "resolution": "exact",
                    "objects_observed": 1,
                }
        resolution, refs = self.pattern_resolution(resolved)
        if refs:
            return {
                "object_refs": refs,
                "resolution": resolution,
                "objects_observed": 1,
            }
        if isinstance(node, (ast.Name, ast.Attribute, ast.Lambda)):
            return {"resolution": "unresolved", "objects_observed": 1}
        return {"objects_observed": 0}

    def static_pattern_value(self, node):
        if isinstance(node, ast.Constant) and isinstance(node.value, str):
            return {"kind": "literal_string", "value": node.value}
        if isinstance(node, ast.JoinedStr):
            parts = []
            valid = True
            has_hole = False
            for value in node.values:
                if isinstance(value, ast.Constant) and isinstance(value.value, str):
                    raw = value.value
                    if raw:
                        parts.append({"kind": "literal", "text": raw})
                elif isinstance(value, ast.FormattedValue):
                    has_hole = True
                    parts.append({"kind": "hole"})
                else:
                    valid = False
                    break
            if valid and has_hole:
                return {"kind": "string_template", "parts": parts}
            if valid:
                literal = "".join(value.get("text", "") for value in parts)
                return {"kind": "literal_string", "value": literal}
        return None

    def initializer_value_candidate(self, node):
        value = self.static_pattern_value(node)
        if value is None:
            return None
        result = dict(value)
        result.update({
            "resolution": "possible",
            "source_kind": "initializer",
        })
        return result

    def pattern_argument_value(self, node):
        result = self.static_pattern_value(node) or {"kind": "dynamic"}
        result["origin"] = self.source_value(node)
        result.update(self.pattern_argument_authority(node))
        if result["kind"] == "dynamic" and isinstance(node, ast.Name):
            binding = self.pattern_binding(node.id)
            candidate = binding.get("value_candidate") if binding is not None else None
            if candidate is not None and not binding.get("value_invalidated", False):
                result["value_candidates"] = [dict(candidate)]
                result["value_candidates_observed"] = 1
        return result

    def source_value(self, node):
        """Keep syntax provenance; never evaluate a repository expression."""
        anchor = source_location(self.module["path"], node)
        unknown = {"kind": "unknown", **({"anchor": anchor} if anchor else {})}
        if isinstance(node, ast.Constant):
            if isinstance(node.value, (str, int, float, bool)) or node.value is None:
                text = node.value if isinstance(node.value, str) else ast.unparse(node)
                return {"kind": "literal", "text": text, "anchor": anchor}
            return unknown
        if isinstance(node, ast.Name):
            scope = self.scope
            while scope is not None:
                binding = self.pattern_bindings.get(id(scope), {}).get(node.id)
                if binding is not None:
                    return binding.get("source_origin") or {**unknown, "text": node.id}
                # A lexically local name cannot read a same-spelled outer
                # parameter before its own assignment has been visited.
                if node.id in scope.bindings:
                    break
                scope = scope.parent
            return {**unknown, "text": node.id}
        if isinstance(node, ast.Call):
            return {"kind": "call_result", "text": safe_expression_name(node.func),
                    "anchor": callee_location(self.module["path"], node.func)}
        if isinstance(node, ast.Attribute):
            return {"kind": "field", "text": node.attr, "anchor": anchor,
                    "parts": [self.source_value(node.value)]}
        if isinstance(node, ast.Subscript):
            return {"kind": "index", "anchor": anchor,
                    "parts": [self.source_value(node.value), self.source_value(node.slice)]}
        if isinstance(node, ast.BinOp) and isinstance(node.op, ast.Add):
            return {"kind": "concat", "anchor": anchor,
                    "parts": [self.source_value(node.left), self.source_value(node.right)]}
        if isinstance(node, ast.JoinedStr):
            parts = []
            for value in node.values:
                if isinstance(value, ast.FormattedValue):
                    # Formatting can transform values. Preserve a formatted
                    # hole as unknown when conversion/specification is used.
                    parts.append(self.source_value(value.value) if value.conversion == -1 and value.format_spec is None
                                 else {"kind": "unknown", "anchor": source_location(self.module["path"], value)})
                else:
                    parts.append(self.source_value(value))
            if len(parts) == 1:
                return parts[0]
            if parts:
                return {"kind": "concat", "anchor": anchor, "parts": parts}
            return {"kind": "literal", "text": "", "anchor": anchor}
        if isinstance(node, ast.IfExp):
            return {"kind": "alternatives", "anchor": anchor,
                    "parts": [self.source_value(node.body), self.source_value(node.orelse)]}
        return unknown

    def bind_source_parameters(self, node, arguments, annotation_origins):
        owner = self.object(self.scope.ref).get("location")
        positional = list(node.args.posonlyargs) + list(node.args.args)
        decorators = [safe_expression_name(value) for value in getattr(node, "decorator_list", [])]
        bound_receiver = self.scope.kind == "method" and "staticmethod" not in decorators
        receiver = positional[0] if bound_receiver and positional else None
        position = 0
        for argument in arguments:
            ref = self.analyzer.node_refs.get(id(argument), "")
            origin = None
            if argument is receiver:
                origin = {"kind": "receiver", "text": argument.arg, "owner": owner,
                          "anchor": source_location(self.module["path"], argument)}
            if argument is not receiver and argument not in (node.args.vararg, node.args.kwarg):
                position += 1
                origin = {"kind": "parameter", "text": argument.arg,
                          "position": position, "owner": owner,
                          "anchor": source_location(self.module["path"], argument)}
            resolution, origins = annotation_origins.get(argument.arg, ("", []))
            self.current_pattern_bindings()[argument.arg] = {
                "ref": ref, "origin_refs": origins, "origin_resolution": resolution,
                "origins_observed": len(origins),
                "binding_observed": True, "value_invalidated": True,
                # Type evidence is independent of literal-value authority.
                # Any later assignment replaces this source-ordered binding.
                "annotation_origin": bool(origins),
                "value_candidate": None, "source_origin": origin,
                "iterable_annotation": self.analyzer.variable_annotations.get(ref),
            }

    def relation_pattern(self, call, form, from_ref):
        selector = self.pattern_selector(call.func)
        if not selector:
            # The call is observed but cannot be represented as a selector
            # candidate. Relation.PatternsObserved exposes this one omission.
            return None, 1
        location = callee_location(self.module["path"], call.func)
        location_key = source_identity(self.module["path"], call)
        arguments = []
        for position, argument in enumerate(call.args, 1):
            if isinstance(argument, ast.Starred):
                continue
            value = self.pattern_argument_value(argument)
            value["position"] = position
            arguments.append(value)
        for keyword in call.keywords:
            if keyword.arg is None:
                continue
            value = self.pattern_argument_value(keyword.value)
            value["keyword"] = keyword.arg
            arguments.append(value)
        pattern = {
            "source_ref": stable_ref("pattern", form, from_ref, selector, location_key),
            "form": form,
            "selector": selector,
            "arguments": arguments,
            "arguments_observed": len(call.args) + len(call.keywords),
        }
        if getattr(call, "repomap_control_context", None):
            pattern["context"] = call.repomap_control_context
        if location is not None:
            pattern["location"] = location
        result_ref = self.analyzer.call_result_refs.get(id(call), "")
        if result_ref:
            pattern["result_ref"] = result_ref
        pattern.update(self.pattern_receiver(call.func))
        if isinstance(call.func, ast.Attribute):
            pattern["receiver_value"] = self.source_value(call.func.value)
        return pattern, 1

    def candidate_detail(self, detail, candidate):
        candidate_name = candidate.get("name", "") if candidate else ""
        if detail and candidate_name and detail != candidate_name:
            return bounded_text(detail + " -> " + candidate_name)
        return detail or candidate_name

    def import_witness(self, authority, module_name, from_import=False):
        if authority != "external":
            return "from_import" if from_import else "import"
        root = module_name.split(".", 1)[0]
        prefix = "python_stdlib" if root in self.analyzer.stdlib_modules else "python_external"
        return prefix + ("_from_import" if from_import else "_import")

    def local_literal_import(self, node):
        if not node.args:
            return "", ""
        argument = node.args[0]
        if not isinstance(argument, ast.Constant) or not isinstance(argument.value, str):
            return "", ""
        # This lookup must be side-effect free. An arbitrary source literal is
        # frontier evidence, not authority for inventing an external object.
        ref = self.local_import_target(argument.value)
        candidate = self.object(ref) if ref else None
        if candidate is None or candidate["kind"] not in ("module", "package"):
            return "", ""
        # Persist the canonical catalog name, never the source literal. The
        # exact fact is the requested local module dependency; the callable
        # dispatch itself remains a separate possible or unresolved relation.
        return ref, candidate["name"]

    def is_stdlib_importlib_import_module(self, node):
        """Prove that a call target came from the stdlib importlib module."""
        if "importlib" not in self.analyzer.stdlib_modules:
            return False
        if isinstance(node, ast.Name):
            binding = self.scope.binding(node.id)
            imported = binding is not None and binding.get("kind") == "from" and \
                binding.get("module") == "importlib" and \
                binding.get("name") == "import_module" and \
                not binding.get("relative", False)
        elif isinstance(node, ast.Attribute) and node.attr == "import_module" and \
                isinstance(node.value, ast.Name):
            binding = self.scope.binding(node.value.id)
            imported = binding is not None and binding.get("kind") == "module" and \
                binding.get("module") == "importlib" and binding.get("external", False)
        else:
            imported = False
        if not imported:
            return False
        authority, ref = self.resolve(node)
        candidate = self.object(ref) if ref else None
        return authority == "external" and candidate is not None and \
            candidate.get("name") == "importlib.import_module"

    def emit_resolved(self, kind, from_ref, resolved, node, witness_kind, detail="", invocation="",
                      exact_authorities=(), source_expression="", witness_callee=None,
                      pattern=None, patterns_observed=0, source_argument=None, extra_witnesses=()):
        authority, ref = resolved
        if authority in exact_authorities and ref:
            return self.analyzer.add_relation(
                kind, from_ref, [ref], "exact", node, witness_kind, detail,
                invocation=invocation, targets_observed=1, source_expression=source_expression,
                witness_callee=witness_callee, patterns=[pattern] if pattern else [],
                patterns_observed=patterns_observed, source_argument=source_argument,
                extra_witnesses=extra_witnesses,
            )
        if authority in ("local", "external", "literal") and ref:
            # The one binding this name resolves to is the target. Rebinding the
            # name elsewhere makes its resolution unknown instead.
            candidate = self.object(ref)
            candidate_detail = self.candidate_detail(detail, candidate)
            return self.analyzer.add_relation(
                kind, from_ref, [ref], "exact", node, witness_kind,
                candidate_detail, invocation=invocation, targets_observed=1,
                source_expression=source_expression, witness_callee=witness_callee,
                patterns=[pattern] if pattern else [], patterns_observed=patterns_observed,
                source_argument=source_argument, extra_witnesses=extra_witnesses,
            )
        return self.analyzer.add_relation(
            kind, from_ref, [], "unresolved", node, witness_kind, detail,
            invocation=invocation, targets_observed=1, source_expression=source_expression,
            witness_callee=witness_callee, patterns=[pattern] if pattern else [],
            patterns_observed=patterns_observed, source_argument=source_argument,
            extra_witnesses=extra_witnesses,
        )

    def emit_decorator(self, resolved, decorated_ref, node, witness_kind, detail="",
                       exact_authorities=(), pattern=None, patterns_observed=0):
        authority, decorator_ref = resolved
        if authority in exact_authorities and decorator_ref:
            self.analyzer.add_relation(
                "decorates", decorated_ref, [decorator_ref], "exact", node, witness_kind, detail,
                targets_observed=1, patterns=[pattern] if pattern else [],
                patterns_observed=patterns_observed,
            )
            return True
        candidate = self.object(decorator_ref) if decorator_ref else None
        witness = witness_kind
        # Direction is always decorated declaration -> decorator.
        self.analyzer.add_relation(
            "decorates", decorated_ref, [decorator_ref] if candidate else [],
            "exact" if candidate else "unresolved", node, witness,
            self.candidate_detail(detail, candidate),
            targets_observed=1, patterns=[pattern] if pattern else [],
            patterns_observed=patterns_observed,
        )
        return candidate is not None

    def visit_Import(self, node):
        for alias in node.names:
            authority, ref = self.import_target(alias.name)
            # The import statement itself establishes this named structural
            # dependency even though later attribute access remains dynamic.
            self.emit_resolved(
                "imports", self.scope.ref, (authority, ref), node,
                self.import_witness(authority, alias.name), alias.name,
                exact_authorities=("local", "external"),
            )
            self.bind_nonvalue_name(alias.asname or alias.name.split(".")[0])

    def visit_ImportFrom(self, node):
        base = relative_module(self.module["name"], self.module["package"], node.level, node.module)
        for alias in node.names:
            if alias.name == "*":
                authority, ref = self.import_target(base, allow_external=node.level == 0)
                witness = self.import_witness(authority, base) if authority == "external" else "wildcard_import"
                # Star expansion leaves the imported member set dynamic, but
                # the import statement still names one exact module boundary.
                # Retain that module as dependency authority without
                # inventing any declaration imported from it.
                self.emit_resolved(
                    "imports", self.scope.ref, (authority, ref), node, witness, base,
                    exact_authorities=("local", "external"),
                )
                continue
            resolved = self.import_target(base, alias.name, allow_external=node.level == 0)
            witness = self.import_witness(resolved[0], base, from_import=True)
            scope = self.analyzer.module_scopes.get(self.analyzer.canonical_qname(base))
            exported = scope.export_bindings.get(alias.name) if scope else None
            reexport = exported is not None and exported["kind"] in ("from", "module")
            if resolved[0] == "unknown" or reexport:
                boundary_ref = self.local_import_target(base)
                boundary = self.object(boundary_ref) if boundary_ref else None
                if boundary is not None and boundary["kind"] in ("module", "package"):
                    # A package facade may expose a mutable or re-exported
                    # member. Preserve the written module boundary, including
                    # when later calls can follow that explicit re-export to a
                    # possible declaration; the import did not skip the facade.
                    resolved = "local", boundary_ref
                    witness = "from_import_module_boundary"
            self.emit_resolved(
                "imports", self.scope.ref, resolved, node,
                witness, base + "." + alias.name,
                exact_authorities=("local", "external"),
            )
            self.bind_nonvalue_name(alias.asname or alias.name)

    def visit_FunctionDef(self, node):
        self._visit_definition(node)

    def visit_AsyncFunctionDef(self, node):
        self._visit_definition(node)

    def _visit_definition(self, node):
        defined_ref = self.analyzer.node_refs[id(node)]
        if isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef)):
            # Classes are declared by now, so annotations resolve to them.
            defined = self.object(defined_ref)
            defined["parameters"] = self.typed_parameters(node, defined["kind"])
            defined["results"] = self.typed_results(node)
        for decorator in node.decorator_list:
            target = decorator.func if isinstance(decorator, ast.Call) else decorator
            detail = self.expression_name(target)
            pattern, patterns_observed = (None, 0)
            if isinstance(decorator, ast.Call):
                pattern, patterns_observed = self.relation_pattern(
                    decorator, "decorator_call", defined_ref,
                )
            self.emit_decorator(
                self.resolve(target), defined_ref, decorator,
                "decorator", detail, exact_authorities=("literal",),
                pattern=pattern, patterns_observed=patterns_observed,
            )
            if isinstance(decorator, ast.Call):
                for argument in list(decorator.args) + [value.value for value in decorator.keywords]:
                    self.visit(argument)
        # Defaults and annotations execute in the defining scope, not in the
        # function body. They may contain calls, callbacks, and dynamic imports
        # and therefore must enter the same exact/possible/unresolved ledger.
        for value in list(node.args.defaults) + list(node.args.kw_defaults):
            if value is not None:
                self.visit(value)
        arguments = list(node.args.posonlyargs) + list(node.args.args) + list(node.args.kwonlyargs)
        if node.args.vararg is not None:
            arguments.append(node.args.vararg)
        if node.args.kwarg is not None:
            arguments.append(node.args.kwarg)
        annotation_origins = {}
        for argument in arguments:
            if argument.annotation is not None:
                self.visit(argument.annotation)
                # A written parameter type supplies a possible receiver origin,
                # not a runtime value or call edge. Resolve it in the defining
                # scope before parameter names can shadow the annotation.
                if (argument not in (node.args.vararg, node.args.kwarg)
                        and isinstance(argument.annotation, (ast.Name, ast.Attribute))):
                    resolved = self.resolve(argument.annotation)
                    candidate = self.object(resolved[1]) if resolved[1] else None
                    if candidate and candidate["kind"] in ("type", "external_symbol"):
                        annotation_origins[argument.arg] = self.pattern_resolution(resolved)
        if node.returns is not None:
            self.visit(node.returns)
        for parameter in getattr(node, "type_params", []):
            self.visit(parameter)
        previous = self.scope
        self.scope = self.analyzer.node_scopes[id(node)]
        self.pattern_bindings[id(self.scope)] = {}
        self.bind_source_parameters(node, arguments, annotation_origins)
        for statement in node.body:
            self.visit(statement)
        self.scope = previous

    def visit_ClassDef(self, node):
        defined_ref = self.analyzer.node_refs[id(node)]
        for base in node.bases:
            self.emit_resolved(
                "implements", defined_ref, self.resolve(base), base,
                "base_class", self.expression_name(base),
            )
            self.visit(base)
        for keyword in node.keywords:
            self.visit(keyword.value)
        for decorator in node.decorator_list:
            target = decorator.func if isinstance(decorator, ast.Call) else decorator
            detail = self.expression_name(target)
            pattern, patterns_observed = (None, 0)
            if isinstance(decorator, ast.Call):
                pattern, patterns_observed = self.relation_pattern(
                    decorator, "decorator_call", defined_ref,
                )
            self.emit_decorator(
                self.resolve(target), defined_ref, decorator,
                "decorator", detail, exact_authorities=("literal",),
                pattern=pattern, patterns_observed=patterns_observed,
            )
            if isinstance(decorator, ast.Call):
                for argument in list(decorator.args) + [value.value for value in decorator.keywords]:
                    self.visit(argument)
        for parameter in getattr(node, "type_params", []):
            self.visit(parameter)
        previous = self.scope
        self.scope = self.analyzer.node_scopes[id(node)]
        self.pattern_bindings[id(self.scope)] = {}
        for statement in node.body:
            self.visit(statement)
        self.scope = previous

    def visit_Lambda(self, node):
        for value in list(node.args.defaults) + list(node.args.kw_defaults):
            if value is not None:
                self.visit(value)
        arguments = list(node.args.posonlyargs) + list(node.args.args) + list(node.args.kwonlyargs)
        if node.args.vararg is not None:
            arguments.append(node.args.vararg)
        if node.args.kwarg is not None:
            arguments.append(node.args.kwarg)
        for argument in arguments:
            if argument.annotation is not None:
                self.visit(argument.annotation)
        previous = self.scope
        self.scope = self.analyzer.node_scopes[id(node)]
        self.pattern_bindings[id(self.scope)] = {}
        self.bind_source_parameters(node, arguments, {})
        self.visit(node.body)
        self.scope = previous

    def read_name(self, name):
        # Reading a declared slot does not prove its runtime value. Keep the
        # lexical declaration (including imported originals), not a guessed
        # value or a method call on it. Class bodies are not method closures.
        if name in self.read_shadows:
            return "unknown", ""
        scope = self.scope
        if name in scope.global_names:
            while scope.parent is not None:
                scope = scope.parent
        elif name in scope.nonlocal_names:
            scope = scope.parent
            while scope is not None and scope.kind == "type":
                scope = scope.parent
        while scope is not None:
            if name in scope.opaque_names:
                return "unknown", ""
            binding = scope.bindings.get(name)
            if binding:
                if binding["kind"] == "object":
                    return "local", binding["ref"]
                if binding["kind"] == "from":
                    return self.import_target(binding["module"], binding["name"], not binding.get("relative", False))
                if binding["kind"] == "module":
                    return self.import_target(binding["module"])
                return "unknown", ""
            scope = scope.parent
            while scope is not None and scope.kind == "type":
                scope = scope.parent
        return "unknown", ""

    def record_read(self, node, resolved):
        candidate = self.object(resolved[1]) if resolved[1] else None
        # A callable reading its own parameter or local is not a program
        # relation; the value's origin stays on the patterns that use it.
        if candidate and self.scope.kind != "module" and self.scope.kind != "type" and \
                candidate.get("container_ref") == self.scope.ref:
            return
        if candidate and candidate["kind"] == "variable":
            self.emit_resolved("reads", self.scope.ref, resolved, node,
                               "variable_read", safe_expression_name(node))

    def visit_Name(self, node):
        if isinstance(node.ctx, ast.Load):
            self.record_read(node, self.read_name(node.id))

    def visit_Attribute(self, node):
        if isinstance(node.ctx, ast.Load):
            root = node
            while isinstance(root, ast.Attribute):
                root = root.value
            resolved = ("unknown", "")
            if isinstance(root, ast.Name):
                base = self.read_name(root.id)
                value = self.object(base[1]) if base[1] else None
                if value and value["kind"] in ("module", "package", "type"):
                    # resolve's declaration binding must agree with lexical
                    # lookup before following module/class attributes.
                    if self.resolve(root) == base:
                        resolved = self.resolve(node)
                elif value and root.id not in self.read_shadows:
                    resolved = self.written_field(node)
            self.record_read(node, resolved)
        self.visit(node.value)

    def visit_ListComp(self, node):
        previous = self.read_shadows
        self.read_shadows = set(previous)
        for generator in node.generators:
            self.visit(generator.iter)
            self.read_shadows.update(part.id for part in ast.walk(generator.target) if isinstance(part, ast.Name))
            for condition in generator.ifs:
                self.visit(condition)
        if isinstance(node, ast.DictComp):
            self.visit(node.key)
            self.visit(node.value)
        else:
            self.visit(node.elt)
        self.read_shadows = previous

    visit_SetComp = visit_ListComp
    visit_DictComp = visit_ListComp
    visit_GeneratorExp = visit_ListComp

    def visit_Await(self, node):
        previous, self.invocation = self.invocation, ""
        self.visit(node.value)
        self.invocation = previous

    def visit_Return(self, node):
        if node.value is not None:
            self.analyzer.return_values.setdefault(self.scope.ref, []).append(self.source_value(node.value))
            self.visit(node.value)

    def visit_Yield(self, node):
        self.analyzer.suspended_callables.add(self.scope.ref)
        if node.value is not None:
            self.visit(node.value)

    visit_YieldFrom = visit_Yield

    def visit_Call(self, node):
        name = self.expression_name(node.func)
        source_expression = safe_expression_name(node.func)
        dynamic_only = False
        if name in ("getattr", "builtins.getattr") or name.endswith(".getattr"):
            self.analyzer.add_relation(
                "reads", self.scope.ref, [], "unresolved", node, "dynamic_getattr",
                name, targets_observed=1,
            )
            dynamic_only = True
        if name in ("setattr", "delattr", "builtins.setattr", "builtins.delattr") or \
                name.endswith(".setattr") or name.endswith(".delattr"):
            self.analyzer.add_relation(
                "writes", self.scope.ref, [], "unresolved", node, "dynamic_setattr",
                name, targets_observed=1,
            )
            dynamic_only = True
        if self.is_stdlib_importlib_import_module(node.func):
            imported_ref, imported_name = self.local_literal_import(node)
            if imported_ref:
                self.analyzer.add_relation(
                    "imports", self.scope.ref, [imported_ref], "exact", node,
                    "dynamic_import_literal", imported_name, targets_observed=1,
                )
            else:
                self.analyzer.add_relation(
                    "imports", self.scope.ref, [], "unresolved", node, "dynamic_import",
                    name, targets_observed=1,
                )

        resolved = self.resolved_call_target(node.func)
        # Filled only for the ordinary call relation retained below. Dynamic-only
        # builtins have no nested pattern argument to cite.
        pattern = None
        call_relation_ref = ""
        if not dynamic_only:
            kind = "invokes_external" if resolved[0] == "external" else "calls"
            pattern, patterns_observed = self.relation_pattern(node, "call", self.scope.ref)
            invocation = self.invocation
            consumer = getattr(node, "repomap_result_consumer", "")
            called = self.object(resolved[1]) if resolved[1] else None
            if consumer and called and called.get("signature", "").startswith("async "):
                invocation = "async_task"
            call_relation_ref = self.emit_resolved(
                kind, self.scope.ref, resolved, node, "callsite", name, invocation,
                exact_authorities=("literal",), source_expression=source_expression,
                witness_callee=node.func, pattern=pattern, patterns_observed=patterns_observed,
                extra_witnesses=self.stored_function_witnesses(node.func) if not resolved[1] else (),
            )

        arguments = [(argument, position, "") for position, argument in enumerate(node.args, 1)]
        arguments.extend((value.value, 0, value.arg or "") for value in node.keywords)
        for argument, position, keyword in arguments:
            authority, ref = self.resolve(argument)
            value = self.object(ref) if ref else None
            if authority in ("local", "literal") and value and value["kind"] in ("function", "method", "lambda"):
                source_argument = None
                if pattern is not None and call_relation_ref and (position > 0 or keyword):
                    source_argument = {
                        "relation_source_ref": call_relation_ref,
                        "pattern_source_ref": pattern["source_ref"],
                        **({"position": position} if position > 0 else {"keyword": keyword}),
                    }
                self.emit_resolved(
                    "passes_callback", self.scope.ref, (authority, ref), argument,
                    "callback_argument", "argument " + (keyword or str(position)) + " of " + name,
                    exact_authorities=("literal",), source_argument=source_argument,
                )
            # Every child expression is visited exactly once. Calling
            # generic_visit after this loop would recursively double nested
            # call traversal and inflate witness accounting.
            if isinstance(argument, ast.Call):
                argument.repomap_result_consumer = name
            self.visit(argument)
        self.visit(node.func)

    def visit_Assign(self, node):
        for target in node.targets:
            self._attribute_write(target)
            self._target_reads(target)
        self.visit(node.value)
        origin = self.assignment_origin(node.value)
        initializer = self.initializer_value_candidate(node.value)
        source_origin = self.source_value(node.value)
        for target in node.targets:
            self.bind_pattern_target(target, origin, initializer, source_origin)
            self.bind_field_type(target, node.value)

    def visit_AnnAssign(self, node):
        self._attribute_write(node.target)
        self._target_reads(node.target)
        if node.value is not None:
            self.visit(node.value)
            self.bind_pattern_target(
                node.target, self.assignment_origin(node.value),
                self.initializer_value_candidate(node.value),
                self.source_value(node.value),
            )
            self.bind_field_type(node.target, node.value)
        else:
            self.bind_pattern_target(node.target, {"observed": 0})
        if isinstance(node.target, ast.Name):
            self.current_pattern_bindings()[node.target.id]["iterable_annotation"] = (node.annotation, self.scope)

    def visit_AugAssign(self, node):
        self._attribute_write(node.target)
        self._target_reads(node.target)
        if isinstance(node.target, ast.Name):
            self.record_read(node.target, self.read_name(node.target.id))
        elif isinstance(node.target, ast.Attribute):
            self.record_read(node.target, self.written_field(node.target))
        self.visit(node.value)
        if isinstance(node.target, ast.Name):
            previous = self.pattern_binding(node.target.id) or {}
            self.current_pattern_bindings()[node.target.id] = {
                "ref": previous.get("ref", ""), "origin_refs": [],
                "origin_resolution": "", "origins_observed": 0,
                "binding_observed": True, "value_invalidated": True,
                "value_candidate": None,
            }

    def visit_NamedExpr(self, node):
        self.visit(node.value)
        self.bind_pattern_target(
            node.target, self.assignment_origin(node.value),
            self.initializer_value_candidate(node.value),
            self.source_value(node.value),
        )

    def visit_For(self, node):
        self.visit(node.iter)
        self._target_reads(node.target)
        type_ref = self.iterated_class(node.iter) if isinstance(node, ast.For) else ""
        self.bind_pattern_target(node.target, {"observed": 0})
        if type_ref and isinstance(node.target, ast.Name):
            self.current_pattern_bindings()[node.target.id].update({
                "origin_refs": [type_ref], "origin_resolution": "alternatives", "origins_observed": 1,
                "annotation_origin": True, "iteration_origin": True,
            })
        for statement in node.body:
            self.visit(statement)
        # A loop can execute zero times; its item type is not established for
        # a later use. Assignments inside the body already clear it in order.
        for target in ast.walk(node.target):
            if isinstance(target, ast.Name):
                self.bind_nonvalue_name(target.id)
        for statement in node.orelse:
            self.visit(statement)

    visit_AsyncFor = visit_For

    def visit_Delete(self, node):
        for target in node.targets:
            self._attribute_write(target)
            self._target_reads(target)
            if isinstance(target, ast.Name):
                self.bind_nonvalue_name(target.id)

    def _target_reads(self, target):
        # A store reads the receiver/index, not the slot it overwrites.
        if isinstance(target, ast.Attribute):
            self.visit(target.value)
        elif isinstance(target, ast.Subscript):
            self.visit(target.value)
            self.visit(target.slice)
        elif isinstance(target, (ast.Tuple, ast.List)):
            for value in target.elts:
                self._target_reads(value)

    def _attribute_write(self, target):
        if isinstance(target, ast.Attribute):
            if isinstance(target.value, ast.Name) and target.value.id == "self" and self.scope.class_ref:
                key = (self.scope.class_ref, target.attr)
                self.analyzer.field_write_counts[key] = self.analyzer.field_write_counts.get(key, 0) + 1
            self.emit_resolved(
                "writes", self.scope.ref, self.written_field(target), target,
                "dynamic_attribute_write", self.expression_name(target),
            )
        elif isinstance(target, (ast.Tuple, ast.List)):
            for value in target.elts:
                self._attribute_write(value)

    def written_field(self, target):
        # A written attribute is a possible field of an observed receiver,
        # not an exact runtime slot (descriptors and __setattr__ still apply).
        # Do not use the lexical spelling `self`: it may be rebound, static,
        # or captured from a different enclosing class.
        receiver = target.value
        if not isinstance(receiver, ast.Name):
            return "unknown", ""
        origin = self.source_value(receiver)
        owner_ref = ""
        if origin.get("kind") == "receiver":
            scope = self.scope
            while scope is not None:
                declaration = self.object(scope.ref)
                if declaration and declaration.get("location") == origin.get("owner"):
                    owner_ref = declaration.get("owner_ref", "")
                    break
                scope = scope.parent
        elif origin.get("kind") in ("parameter", "call_result") or (self.pattern_binding(receiver.id) or {}).get("iteration_origin"):
            binding = self.pattern_binding(receiver.id)
            if binding and (binding.get("annotation_origin") or not binding.get("value_invalidated")):
                refs = binding.get("origin_refs", [])
                if len(refs) == 1:
                    owner_ref = self.produced_class(refs[0])
        owner = self.object(owner_ref)
        if not owner or owner["kind"] != "type":
            return "unknown", ""
        ref = self.analyzer.objects_by_qname.get(self.analyzer.object_qname(owner_ref) + "." + target.attr, "")
        field = self.object(ref)
        if field and field["kind"] == "variable" and field.get("owner_ref") == owner_ref:
            return "local", ref
        return "unknown", ""


def attach_control_context(tree, path):
    # Reuse the parsed tree across target views. Bodies of newly declared
    # callables do not inherit the loop in which the callable was created.
    def walk(node, context):
        if isinstance(node, ast.Call) and context:
            node.repomap_control_context = context
        if isinstance(node, (ast.For, ast.AsyncFor, ast.While)):
            if isinstance(node, ast.While):
                walk(node.test, context)
                kind = "while body with constant true condition" if isinstance(node.test, ast.Constant) and node.test.value is True else "while body"
            else:
                walk(node.iter, context)
                kind = "async for body" if isinstance(node, ast.AsyncFor) else "for body"
            nested = context + [{"kind": "control_context", "detail": kind, "location": source_location(path, node)}]
            for child in node.body:
                walk(child, nested)
            for child in node.orelse:
                walk(child, context)
            return
        if isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef, ast.Lambda, ast.ClassDef)):
            body = node.body if isinstance(node.body, list) else [node.body]
            for child in ast.iter_child_nodes(node):
                walk(child, [] if child in body else context)
            return
        for child in ast.iter_child_nodes(node):
            walk(child, context)
    walk(tree, [])


def parse_sources(rows):
    parsed = {}
    for item in sorted(rows, key=lambda value: value.get("path", "")):
        path = item.get("path", "")
        if not path or path in parsed:
            raise ValueError("source paths are empty or duplicated")
        try:
            content = base64.b64decode(item.get("content", ""), validate=True).decode("utf-8")
        except Exception:
            raise ValueError("module %s is not valid base64 UTF-8" % path)
        try:
            parsed[path] = ast.parse(content, filename=path, type_comments=True)
            attach_control_context(parsed[path], path)
        except (SyntaxError, ValueError):
            raise ValueError("module %s has invalid Python syntax" % path)
        parsed[path].repomap_code_lines = code_line_set(content, parsed[path])
    if not parsed:
        raise ValueError("source inventory is empty")
    return parsed


def main():
    try:
        request = json.load(sys.stdin)
        parsed_sources = parse_sources(request.get("sources", []))
        views = request.get("views", [])
        if not isinstance(views, list) or not views:
            raise ValueError("semantic view inventory is empty")
        results = [Analyzer(view, parsed_sources).prepare() for view in views]
        result = {
            "python_version": sys.implementation.name + "-" + ".".join(
                str(value) for value in sys.version_info[:3]
            ),
            "views": [
                {"objects": value["objects"], "relations": value["relations"]}
                for value in results
            ],
        }
    except Exception as error:
        result = {"fatal": bounded_text(str(error)), "python_version": "", "views": []}
    print(json.dumps(result, ensure_ascii=False, sort_keys=True, separators=(",", ":")))


main()
