import ast
import base64
import hashlib
import json
import sys


PYTHON_PUBLIC_SYMBOL_DOMAIN = "python_public_symbol_v1"


def stable_ref(domain, *parts):
    wire = json.dumps([domain, *parts], ensure_ascii=False, separators=(",", ":")).encode("utf-8")
    return "python-source-" + hashlib.sha256(wire).hexdigest()[:32]


def source_location(path, node):
    if getattr(node, "no_location", False):
        return None
    line = getattr(node, "lineno", 0)
    column = getattr(node, "col_offset", -1) + 1
    if line < 1 or column < 1:
        return None
    return {"path": path, "line": line, "column": column}


def callee_location(path, node):
    if isinstance(node, ast.Attribute):
        line = getattr(node, "end_lineno", 0)
        end_column = getattr(node, "end_col_offset", -1)
        column = end_column - len(node.attr.encode("utf-8")) + 1
        if line >= 1 and column >= 1:
            return {"path": path, "line": line, "column": column}
    return source_location(path, node)


def visibility(name, forced_internal=False):
    if forced_internal or name.startswith("_"):
        return "internal"
    return "public"


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
    return ""


def safe_arguments(arguments):
    # Parameter identifiers and Python's positional/keyword markers describe
    # the callable shape without copying annotations or default expressions.
    # Defaults and annotations can contain credentials or other source
    # literals and therefore never enter a persistent ProgramIndex.
    values = []
    positional_only = [value.arg for value in arguments.posonlyargs]
    positional = [value.arg for value in arguments.args]
    values.extend(positional_only)
    if positional_only:
        values.append("/")
    values.extend(positional)
    if arguments.vararg is not None:
        values.append("*" + arguments.vararg.arg)
    elif arguments.kwonlyargs:
        values.append("*")
    values.extend(value.arg for value in arguments.kwonlyargs)
    if arguments.kwarg is not None:
        values.append("**" + arguments.kwarg.arg)
    return ", ".join(values)


def function_signature(node):
    prefix = "async " if isinstance(node, ast.AsyncFunctionDef) else ""
    return bounded_text(prefix + node.name + "(" + safe_arguments(node.args) + ")")


def class_signature(node):
    bases = [safe_expression_name(value) for value in node.bases]
    bases = [value for value in bases if value]
    if not bases:
        return "class " + node.name
    return bounded_text("class " + node.name + "(" + ", ".join(bases) + ")")


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

    def binding(self, name):
        current = self
        while current is not None:
            if name in current.bindings:
                return current.bindings[name]
            current = current.parent
        return None


class Analyzer:
    def __init__(self, view, parsed_sources):
        if not hasattr(sys, "stdlib_module_names"):
            raise ValueError("Python runtime does not expose exact stdlib module authority")
        self.stdlib_modules = frozenset(sys.stdlib_module_names)
        self.files = sorted(view.get("files", []), key=lambda value: value.get("path", ""))
        self.package_rows = sorted(view.get("packages", []), key=lambda value: value.get("name", ""))
        self.parsed_sources = parsed_sources
        file_paths = [value.get("path", "") for value in self.files]
        if len(file_paths) != len(set(file_paths)) or set(file_paths) != set(parsed_sources):
            raise ValueError("semantic view source inventory does not match parsed sources")
        self.modules = {}
        self.module_aliases = {}
        self.objects = []
        self.objects_by_ref = {}
        self.objects_by_qname = {}
        self.node_refs = {}
        self.node_scopes = {}
        self.call_result_refs = {}
        self.module_scopes = {}
        self.relations = []
        self.relations_by_key = {}

    def symbol_link_identity(self, value, qname):
        if not qname:
            return None
        if value.get("kind") == "external_symbol":
            pass
        elif value.get("kind") not in ("function", "method", "type") or \
                value.get("visibility") != "public":
            return None
        if value.get("kind") != "external_symbol" and \
                any(part.startswith("_") for part in qname.split(".")):
            # A public-looking member of a private module/type is not a public
            # import binding for another target.
            return None
        return {
            "domain": PYTHON_PUBLIC_SYMBOL_DOMAIN,
            # Python's stable cross-shard fact is the public binding's fully
            # qualified name. Its runtime value can still be rebound, so call
            # relations remain alternatives rather than borrowing authority
            # from this identity.
            "parts": ["binding", bounded_text(qname)],
            "display": bounded_text(qname),
        }

    def add_symbol_link_identity(self, value, qname):
        identity = self.symbol_link_identity(value, qname)
        if identity is None:
            return
        identities = value.setdefault("symbol_link_identities", [])
        if identity not in identities:
            identities.append(identity)
            identities.sort(key=lambda item: (item["domain"], item["parts"], item["display"]))

    def add_object(self, value, qname=""):
        ref = value["source_ref"]
        existing = self.objects_by_ref.get(ref)
        if existing is not None:
            if existing.get("location") is None and value.get("location") is not None:
                existing["location"] = value["location"]
            if qname:
                self.add_symbol_link_identity(existing, qname)
                self.objects_by_qname[qname] = ref
            return ref
        if qname:
            self.add_symbol_link_identity(value, qname)
        self.objects.append(value)
        self.objects_by_ref[ref] = value
        if qname:
            self.objects_by_qname[qname] = ref
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
                     detail="", invocation="", targets_observed=None, witnesses_observed=1,
                     source_expression="", witness_callee=None, patterns=None,
                     patterns_observed=0, source_argument=None):
        path = self.current_path
        location = source_location(path, node)
        witness_location = callee_location(path, witness_callee) \
            if witness_callee is not None else location
        to_refs = sorted(set(to_refs))
        if targets_observed is None:
            targets_observed = len(to_refs) if to_refs else 1
        location_key = ""
        if location is not None:
            location_key = "%s:%d:%d" % (location["path"], location["line"], location["column"])
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
            existing["witnesses_observed"] += witnesses_observed
            existing["patterns_observed"] += patterns_observed
            candidate = json.dumps(witness, sort_keys=True, separators=(",", ":"))
            known = {
                json.dumps(value, sort_keys=True, separators=(",", ":"))
                for value in existing["witnesses"]
            }
            if candidate not in known:
                existing["witnesses"].append(witness)
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
            "witnesses": [witness],
            "witnesses_observed": witnesses_observed,
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
            self.add_object({
                "source_ref": module["source_ref"],
                "kind": kind,
                "name": module["name"],
                "visibility": visibility(module["name"].split(".")[-1]),
                "location": {"path": path, "line": 1, "column": 1},
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
            self.module_scopes[module["name"]] = scope
            collector = Collector(self, module, scope)
            collector.visit(module["tree"])

        for module in decoded:
            self.current_path = module["path"]
            visitor = RelationVisitor(self, module, self.module_scopes[module["name"]])
            visitor.visit(module["tree"])

        for value in list(self.objects):
            container = value.get("container_ref", "")
            if not container:
                continue
            location = value.get("location")
            node = SyntheticNode(location)
            self.current_path = location["path"] if location is not None else decoded[0]["path"]
            self.add_relation(
                "contains", container, [value["source_ref"]], "exact", node,
                "declaration" if location is not None else "declared_scope",
                value["name"], targets_observed=1,
            )

        self.objects.sort(key=lambda value: value["source_ref"])
        self.relations.sort(key=lambda value: value["source_ref"])
        return {
            "objects": self.objects,
            "relations": self.relations,
        }


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

    def object_ref(self, kind, qname, node):
        return stable_ref(
            "object", self.module["source_ref"], kind, qname,
            str(getattr(node, "lineno", 0)), str(getattr(node, "col_offset", -1) + 1),
        )

    def add_variable(self, name, node, forced_internal=False):
        if not name or name == "_":
            return ""
        qname = self.scope.qname + "." + name
        ref = self.object_ref("variable", qname, node)
        self.analyzer.add_object({
            "source_ref": ref,
            "kind": "variable",
            "name": name,
            "visibility": visibility(name, forced_internal or self.scope.kind in ("function", "method", "lambda")),
            "container_ref": self.scope.ref,
            "location": source_location(self.module["path"], node),
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

    def ensure_call_result(self, node):
        existing = self.analyzer.call_result_refs.get(id(node), "")
        if existing:
            return existing
        location = source_location(self.module["path"], node)
        ref = stable_ref(
            "call-result", self.module["source_ref"],
            str(getattr(node, "lineno", 0)), str(getattr(node, "col_offset", -1) + 1),
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
        self.generic_visit(node)

    def visit_FunctionDef(self, node):
        self._visit_function(node)

    def visit_AsyncFunctionDef(self, node):
        self._visit_function(node)

    def _visit_function(self, node):
        parent = self.scope
        kind = "method" if parent.kind == "type" else "function"
        qname = parent.qname + "." + node.name
        ref = self.object_ref(kind, qname, node)
        owner_ref = parent.ref if kind == "method" else ""
        self.analyzer.add_object({
            "source_ref": ref,
            "kind": kind,
            "name": node.name,
            "visibility": visibility(node.name),
            "signature": function_signature(node),
            **({"owner_ref": owner_ref} if owner_ref else {}),
            "container_ref": parent.ref,
            "location": source_location(self.module["path"], node),
        }, qname)
        parent.bindings[node.name] = {"kind": "object", "ref": ref}
        self.analyzer.node_refs[id(node)] = ref
        for value in list(node.decorator_list) + list(node.args.defaults) + list(node.args.kw_defaults):
            if value is not None:
                self.visit(value)
        child = Scope(
            ref, qname, kind, parent,
            class_ref=parent.ref if kind == "method" else parent.class_ref,
            class_qname=parent.qname if kind == "method" else parent.class_qname,
        )
        self.analyzer.node_scopes[id(node)] = child
        previous, self.scope = self.scope, child
        arguments = list(node.args.posonlyargs) + list(node.args.args) + list(node.args.kwonlyargs)
        if node.args.vararg is not None:
            arguments.append(node.args.vararg)
        if node.args.kwarg is not None:
            arguments.append(node.args.kwarg)
        for argument in arguments:
            self.add_variable(argument.arg, argument, True)
        for statement in node.body:
            self.visit(statement)
        self.scope = previous

    def visit_ClassDef(self, node):
        parent = self.scope
        qname = parent.qname + "." + node.name
        ref = self.object_ref("type", qname, node)
        self.analyzer.add_object({
            "source_ref": ref,
            "kind": "type",
            "name": node.name,
            "visibility": visibility(node.name),
            "signature": class_signature(node),
            "container_ref": parent.ref,
            "location": source_location(self.module["path"], node),
        }, qname)
        parent.bindings[node.name] = {"kind": "object", "ref": ref}
        self.analyzer.node_refs[id(node)] = ref
        for value in list(node.decorator_list) + list(node.bases) + [keyword.value for keyword in node.keywords]:
            self.visit(value)
        child = Scope(ref, qname, "type", parent, class_ref=ref, class_qname=qname)
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
            "signature": bounded_text("lambda " + safe_arguments(node.args)),
            "container_ref": parent.ref,
            "location": source_location(self.module["path"], node),
        }, qname)
        self.analyzer.node_refs[id(node)] = ref
        child = Scope(ref, qname, "lambda", parent, class_ref=parent.class_ref, class_qname=parent.class_qname)
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
            self.bind_targets(target)
            self.bind_callable_alias(target, alias_binding)
        if isinstance(node.value, ast.Lambda):
            lambda_ref = self.analyzer.node_refs.get(id(node.value), "")
            if lambda_ref:
                for target in node.targets:
                    if isinstance(target, ast.Name):
                        self.scope.bindings[target.id] = {"kind": "object", "ref": lambda_ref}

    def visit_AnnAssign(self, node):
        alias_binding = self.callable_alias_binding(node.value) if node.value is not None else None
        if node.value is not None:
            self.visit(node.value)
        self.bind_targets(node.target)
        self.bind_callable_alias(node.target, alias_binding)

    def visit_NamedExpr(self, node):
        alias_binding = self.callable_alias_binding(node.value)
        self.visit(node.value)
        self.bind_targets(node.target, True)
        self.bind_callable_alias(node.target, alias_binding)

    def visit_For(self, node):
        self.visit(node.iter)
        self.bind_targets(node.target, True)
        for statement in node.body + node.orelse:
            self.visit(statement)

    visit_AsyncFor = visit_For

    def visit_Import(self, node):
        for alias in node.names:
            bound = alias.asname or alias.name.split(".")[0]
            module_name = alias.name if alias.asname else alias.name.split(".")[0]
            external = module_name not in self.analyzer.modules and module_name not in self.analyzer.objects_by_qname
            if external:
                self.analyzer.ensure_external(module_name)
            self.scope.bindings[bound] = {
                "kind": "module", "module": module_name, "external": external,
            }

    def visit_ImportFrom(self, node):
        base = relative_module(self.module["name"], self.module["package"], node.level, node.module)
        for alias in node.names:
            if alias.name == "*":
                continue
            self.scope.bindings[alias.asname or alias.name] = {
                "kind": "from", "module": base, "name": alias.name,
                "relative": node.level > 0,
            }


class RelationVisitor(ast.NodeVisitor):
    def __init__(self, analyzer, module, scope):
        self.analyzer = analyzer
        self.module = module
        self.scope = scope
        self.invocation = "direct"
        # Pattern receiver provenance is deliberately source ordered. The
        # declaration collector has already created stable variable objects,
        # but it must not let a later assignment explain an earlier call.
        self.pattern_bindings = {id(scope): {}}

    def object(self, ref):
        return self.analyzer.objects_by_ref.get(ref)

    def import_target(self, module_name, imported_name="", allow_external=True):
        qname = module_name + (("." + imported_name) if imported_name else "")
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
        if not allow_external:
            return "unknown", ""
        return "external", self.analyzer.ensure_external(qname)

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
                return ("local", ref) if ref else ("unknown", "")
            if isinstance(current, ast.Name):
                binding = self.scope.binding(current.id)
                if binding and binding["kind"] == "module":
                    qname = binding["module"] + "." + ".".join(parts)
                    if qname in self.analyzer.objects_by_qname:
                        ref = self.analyzer.objects_by_qname[qname]
                        value = self.object(ref)
                        return ("external" if value and value["kind"] == "external_symbol" else "local"), ref
                    canonical = self.analyzer.canonical_qname(qname)
                    if canonical in self.analyzer.objects_by_qname:
                        ref = self.analyzer.objects_by_qname[canonical]
                        value = self.object(ref)
                        return ("external" if value and value["kind"] == "external_symbol" else "local"), ref
                    if binding.get("external"):
                        return "external", self.analyzer.ensure_external(qname)
                    return "unknown", ""
                base_kind, base_ref = self.resolve(current)
                base = self.object(base_ref) if base_ref else None
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
            return "alternatives", [ref]
        return "unresolved", []

    def resolved_call_target(self, node):
        resolved = self.resolve(node)
        if resolved[0] == "local" and resolved[1]:
            value = self.object(resolved[1])
            if value is None or value["kind"] not in ("function", "method", "lambda", "type"):
                return "unknown", ""
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

    def bind_pattern_name(self, node, origin, initializer=None):
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
        }

    def bind_pattern_target(self, target, origin, initializer=None):
        if isinstance(target, ast.Name):
            self.bind_pattern_name(target, origin, initializer)
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

    def pattern_receiver(self, callee):
        if not isinstance(callee, ast.Attribute):
            return {}
        if isinstance(callee.value, ast.Call):
            ref = self.analyzer.call_result_refs.get(id(callee.value), "")
            return {"receiver_ref": ref} if ref else {}
        if not isinstance(callee.value, ast.Name):
            return {}
        binding = self.pattern_binding(callee.value.id)
        if binding is None:
            return {}
        result = {
            "receiver_origins_observed": binding.get("origins_observed", 0),
        }
        if binding.get("ref"):
            result["receiver_ref"] = binding["ref"]
        if binding.get("origin_refs"):
            result["receiver_origin_refs"] = list(binding["origin_refs"])
        if binding.get("origin_resolution"):
            result["receiver_origin_resolution"] = binding["origin_resolution"]
        return result

    def pattern_argument_authority(self, node):
        if isinstance(node, ast.Name):
            binding = self.pattern_binding(node.id)
            if binding is not None and binding.get("ref"):
                return {
                    "object_refs": [binding["ref"]],
                    "resolution": "alternatives",
                    "objects_observed": 1,
                }
        resolution, refs = self.pattern_resolution(self.resolve(node))
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
        result.update(self.pattern_argument_authority(node))
        if result["kind"] == "dynamic" and isinstance(node, ast.Name):
            binding = self.pattern_binding(node.id)
            candidate = binding.get("value_candidate") if binding is not None else None
            if candidate is not None and not binding.get("value_invalidated", False):
                result["value_candidates"] = [dict(candidate)]
                result["value_candidates_observed"] = 1
        return result

    def relation_pattern(self, call, form, from_ref):
        selector = self.pattern_selector(call.func)
        if not selector:
            # The call is observed but cannot be represented as a selector
            # candidate. Relation.PatternsObserved exposes this one omission.
            return None, 1
        location = source_location(self.module["path"], call)
        location_key = ""
        if location is not None:
            location_key = "%s:%d:%d" % (
                location["path"], location["line"], location["column"],
            )
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
        if location is not None:
            pattern["location"] = location
        result_ref = self.analyzer.call_result_refs.get(id(call), "")
        if result_ref:
            pattern["result_ref"] = result_ref
        pattern.update(self.pattern_receiver(call.func))
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
                      pattern=None, patterns_observed=0, source_argument=None):
        authority, ref = resolved
        if authority in exact_authorities and ref:
            return self.analyzer.add_relation(
                kind, from_ref, [ref], "exact", node, witness_kind, detail,
                invocation=invocation, targets_observed=1, source_expression=source_expression,
                witness_callee=witness_callee, patterns=[pattern] if pattern else [],
                patterns_observed=patterns_observed, source_argument=source_argument,
            )
        if authority in ("local", "external", "literal") and ref:
            # Python names, attributes, descriptors and class members are
            # mutable runtime joints. Retain the locally observed candidate as
            # an alternative edge, but do not promote even a single candidate
            # to an exact runtime target.
            candidate = self.object(ref)
            candidate_detail = self.candidate_detail(detail, candidate)
            return self.analyzer.add_relation(
                kind, from_ref, [ref], "alternatives", node, witness_kind + "_candidate",
                candidate_detail, invocation=invocation, targets_observed=1,
                source_expression=source_expression, witness_callee=witness_callee,
                patterns=[pattern] if pattern else [], patterns_observed=patterns_observed,
                source_argument=source_argument,
            )
        return self.analyzer.add_relation(
            kind, from_ref, [], "unresolved", node, witness_kind, detail,
            invocation=invocation, targets_observed=1, source_expression=source_expression,
            witness_callee=witness_callee, patterns=[pattern] if pattern else [],
            patterns_observed=patterns_observed, source_argument=source_argument,
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
        witness = witness_kind + "_candidate" if candidate else witness_kind
        # Direction is always decorated declaration -> decorator candidate.
        # Python's mutable binding keeps a known candidate alternative rather
        # than changing relation orientation or claiming exact dispatch.
        self.analyzer.add_relation(
            "decorates", decorated_ref, [decorator_ref] if candidate else [],
            "alternatives" if candidate else "unresolved", node, witness,
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
            if resolved[0] == "unknown":
                boundary_ref = self.local_import_target(base)
                boundary = self.object(boundary_ref) if boundary_ref else None
                if boundary is not None and boundary["kind"] in ("module", "package"):
                    # A package facade may expose a mutable or re-exported
                    # member whose declaration identity is not locally exact.
                    # The named import still establishes its local module
                    # boundary, just as a wildcard import does. Retain only
                    # that boundary and do not invent the imported member.
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
        for argument in arguments:
            if argument.annotation is not None:
                self.visit(argument.annotation)
        if node.returns is not None:
            self.visit(node.returns)
        for parameter in getattr(node, "type_params", []):
            self.visit(parameter)
        previous = self.scope
        self.scope = self.analyzer.node_scopes[id(node)]
        self.pattern_bindings[id(self.scope)] = {}
        for argument in arguments:
            ref = self.analyzer.node_refs.get(id(argument), "")
            self.current_pattern_bindings()[argument.arg] = {
                "ref": ref, "origin_refs": [], "origin_resolution": "",
                "origins_observed": 0,
                "binding_observed": True, "value_invalidated": True,
                "value_candidate": None,
            }
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
        for argument in arguments:
            ref = self.analyzer.node_refs.get(id(argument), "")
            self.current_pattern_bindings()[argument.arg] = {
                "ref": ref, "origin_refs": [], "origin_resolution": "",
                "origins_observed": 0,
                "binding_observed": True, "value_invalidated": True,
                "value_candidate": None,
            }
        self.visit(node.body)
        self.scope = previous

    def visit_Await(self, node):
        previous, self.invocation = self.invocation, "awaited"
        self.visit(node.value)
        self.invocation = previous

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
            call_relation_ref = self.emit_resolved(
                kind, self.scope.ref, resolved, node, "callsite", name, self.invocation,
                exact_authorities=("literal",), source_expression=source_expression,
                witness_callee=node.func, pattern=pattern, patterns_observed=patterns_observed,
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
                    "callback_argument", safe_expression_name(argument),
                    exact_authorities=("literal",), source_argument=source_argument,
                )
            # Every child expression is visited exactly once. Calling
            # generic_visit after this loop would recursively double nested
            # call traversal and inflate witness accounting.
            self.visit(argument)
        self.visit(node.func)

    def visit_Assign(self, node):
        for target in node.targets:
            self._attribute_write(target)
        self.visit(node.value)
        origin = self.assignment_origin(node.value)
        initializer = self.initializer_value_candidate(node.value)
        for target in node.targets:
            self.bind_pattern_target(target, origin, initializer)

    def visit_AnnAssign(self, node):
        self._attribute_write(node.target)
        if node.value is not None:
            self.visit(node.value)
            self.bind_pattern_target(
                node.target, self.assignment_origin(node.value),
                self.initializer_value_candidate(node.value),
            )
        else:
            self.bind_pattern_target(node.target, {"observed": 0})

    def visit_AugAssign(self, node):
        self._attribute_write(node.target)
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
        )

    def visit_For(self, node):
        self.visit(node.iter)
        self.bind_pattern_target(node.target, {"observed": 0})
        for statement in node.body + node.orelse:
            self.visit(statement)

    visit_AsyncFor = visit_For

    def visit_Delete(self, node):
        for target in node.targets:
            self._attribute_write(target)
            if isinstance(target, ast.Name):
                self.bind_nonvalue_name(target.id)

    def _attribute_write(self, target):
        if isinstance(target, ast.Attribute):
            self.analyzer.add_relation(
                "writes", self.scope.ref, [], "unresolved", target,
                "dynamic_attribute_write", self.expression_name(target), targets_observed=1,
            )
        elif isinstance(target, (ast.Tuple, ast.List)):
            for value in target.elts:
                self._attribute_write(value)


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
        except (SyntaxError, ValueError):
            raise ValueError("module %s has invalid Python syntax" % path)
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
