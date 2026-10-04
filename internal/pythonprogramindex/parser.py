import ast
import base64
import builtins
import hashlib
import io
import json
import sys
import tokenize


# The names the running interpreter's builtins module defines: what a bare
# name no scope of its module binds falls back to (RelationVisitor.builtin_name).
BUILTIN_NAMES = frozenset(vars(builtins))


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


def statement_position(node):
    """Where a statement starts, for source order within one module."""
    return (getattr(node, "lineno", 0), getattr(node, "col_offset", 0))


LOOPS = (ast.For, ast.AsyncFor, ast.While)
# A list, set or dict comprehension runs where it stands, its body repeating
# like a loop's; a generator expression runs when it is consumed.
EAGER_COMPREHENSIONS = (ast.ListComp, ast.SetComp, ast.DictComp)


# A body that runs when it is called, consumed or awaited, not where it is
# written (stored_callees' closures).
CLOSURES = (ast.FunctionDef, ast.AsyncFunctionDef, ast.Lambda, ast.GeneratorExp)
TRIES = (ast.Try,) + ((ast.TryStar,) if hasattr(ast, "TryStar") else ())
MODULE_END = (10 ** 9, 0)


def store_span(node):
    """Where a store's statement starts and ends, for stored_callees."""
    start = statement_position(node)
    return {"start": start, "end": (getattr(node, "end_lineno", start[0]), getattr(node, "end_col_offset", start[1]))}


def repeating_spans(node):
    """The spans of a loop or an eager comprehension that run again on each
    pass: a for's target and body, a while's test and body, a
    comprehension's all but its first iterable; never an else."""
    if isinstance(node, (ast.For, ast.AsyncFor)):
        parts = [node.target] + node.body
    elif isinstance(node, ast.While):
        parts = [node.test] + node.body
    else:
        parts = [node.key, node.value] if isinstance(node, ast.DictComp) else [node.elt]
        for index, generator in enumerate(node.generators):
            parts += [generator.target] + list(generator.ifs) + ([generator.iter] if index else [])
    return [tuple(store_span(part).values()) for part in parts]


def location_key(location):
    """A source location's order across modules: path, line, column."""
    location = location or {}
    return (location.get("path", ""), location.get("line", 0), location.get("column", 0))


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


def origins_invalidated(binding):
    # Whether a name's source-ordered binding no longer establishes the
    # class of its value: a store other than None preceded it. A binding
    # that records nothing about None stores falls back to its value rule.
    return binding.get("origin_invalidated", binding.get("value_invalidated", False))


BLOCKS = (ast.If, ast.For, ast.AsyncFor, ast.While, ast.Try, ast.With, ast.AsyncWith, ast.Match) + \
    ((ast.TryStar,) if hasattr(ast, "TryStar") else ())
DEFINITIONS = (ast.FunctionDef, ast.AsyncFunctionDef, ast.ClassDef, ast.Lambda)
COMPREHENSIONS = (ast.ListComp, ast.SetComp, ast.DictComp, ast.GeneratorExp)


def bound_names(node):
    # The names a statement or expression binds where it stands: an
    # assignment's, a loop's, a with item's or a deletion's targets.
    if isinstance(node, (ast.Assign, ast.Delete)):
        targets = node.targets
    elif isinstance(node, (ast.AnnAssign, ast.AugAssign, ast.NamedExpr, ast.For, ast.AsyncFor, ast.comprehension)):
        targets = [node.target]
    elif isinstance(node, (ast.With, ast.AsyncWith)):
        targets = [item.optional_vars for item in node.items if item.optional_vars is not None]
    elif isinstance(node, ast.ExceptHandler):
        return {node.name} if node.name else set()
    else:
        return set()
    return {part.id for target in targets for part in ast.walk(target) if isinstance(part, ast.Name)}


def pass_jumps(node):
    # The statements in node that end a loop's pass early: return and raise
    # anywhere, break and continue outside an inner loop's body. A nested
    # definition runs apart.
    found = set()
    pending = [(node, False)]
    while pending:
        value, inner = pending.pop()
        if isinstance(value, DEFINITIONS):
            continue
        if isinstance(value, (ast.Return, ast.Raise)) or \
                isinstance(value, (ast.Break, ast.Continue)) and not inner:
            found.add(type(value))
        loop = isinstance(value, (ast.For, ast.AsyncFor, ast.While))
        for child in ast.iter_child_nodes(value):
            pending.append((child, inner or (loop and any(child is item for item in value.body))))
    return found


def unconditional_subscripts(node):
    # The subscripts an expression or a plain statement evaluates whenever
    # it runs: not in a conditional expression's branches, after the first
    # operand of and/or, in a lambda or in a comprehension.
    found = set()
    pending = [node]
    while pending:
        value = pending.pop()
        if isinstance(value, ast.Subscript):
            found.add(id(value))
        if isinstance(value, ast.IfExp):
            pending.append(value.test)
        elif isinstance(value, ast.BoolOp):
            pending.append(value.values[0])
        elif not isinstance(value, DEFINITIONS + COMPREHENSIONS):
            pending.extend(ast.iter_child_nodes(value))
    return found


def pass_subscripts(body):
    # The subscripts a for statement's body evaluates on every pass, for a
    # subscript keyed by its elements (record_table_key): none when the body
    # can end the loop early (break, return, raise); else those in its plain
    # statements before one that can skip the rest of a pass (continue),
    # never in a block's own statements.
    jumps = set()
    for statement in body:
        jumps |= pass_jumps(statement)
    if jumps & {ast.Break, ast.Return, ast.Raise}:
        return set()
    reached = set()
    for statement in body:
        if ast.Continue in pass_jumps(statement):
            break
        if not isinstance(statement, BLOCKS + DEFINITIONS):
            reached |= unconditional_subscripts(statement)
    return reached


def compared_operand(test):
    """The expression one comparison with words compares, or None."""
    if len(test.ops) != 1:
        return None
    left, right, operator = test.left, test.comparators[0], test.ops[0]
    def text(value):
        return isinstance(value, ast.Constant) and isinstance(value.value, str)
    if isinstance(operator, ast.Eq):
        if text(right) and not isinstance(left, ast.Constant):
            return left
        if text(left) and not isinstance(right, ast.Constant):
            return right
    if isinstance(operator, ast.In) and isinstance(right, (ast.Tuple, ast.List, ast.Set)) and right.elts and \
            all(text(value) for value in right.elts) and not isinstance(left, ast.Constant):
        return left
    return None


def only_when_compared(test, key):
    """A condition that holds only when the expression dumped as key equals
    one of the words it is compared with: such comparisons joined by or, or
    an and one of whose operands is one. `len(v) != 2 or v == "nu"` may
    hold for other values."""
    if isinstance(test, ast.BoolOp):
        results = [only_when_compared(value, key) for value in test.values]
        return all(results) if isinstance(test.op, ast.Or) else any(results)
    if isinstance(test, ast.Compare):
        compared = compared_operand(test)
        return compared is not None and ast.dump(compared) == key
    return False


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
        # Where a module writes each export binding, and where its star
        # imports are, as (line, column) statement positions. A star may bind
        # any name where it runs; a binding written after the last one is
        # still the module's own.
        self.export_positions = {}
        self.star_imports = []
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
        # The AST node the scope is (the module's tree, a def, a class or a
        # lambda), for stored_callees' walk of its body.
        self.node = None
        # The scope's parameters, and the names it binds without a recorded
        # store (a for target, an augmented assignment): a value no store
        # names may be in them (stored_callees).
        self.parameters = set()
        self.unstored_names = set()

    def binding(self, name):
        owner = self.owner(name)
        return owner.bindings[name] if owner is not None else None

    def owner(self, name):
        # A class body's names are visible in that body only: Python looks a
        # name a method or a lambda uses up in it, the functions around it
        # and the module, never in a class body around it (a method calling
        # range() calls the builtin, not the class's own range method).
        current = self
        while current is not None:
            if name in current.bindings and (current is self or current.kind != "type"):
                return current
            current = current.parent
        return None


class Analyzer:
    def __init__(self, view, parsed_sources):
        if not hasattr(sys, "stdlib_module_names"):
            raise ValueError("Python runtime does not expose exact stdlib module authority")
        self.stdlib_modules = frozenset(sys.stdlib_module_names)
        self.files = sorted(view.get("files", []), key=lambda value: value.get("path", ""))
        # Configured test sources: what their code stores is not the
        # program's (a test replacing a class attribute with a mock).
        self.test_paths = {value.get("path", "") for value in self.files if value.get("test")}
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
        # The defs whose decorators resolve to typing.overload, every other
        # one outside the repository: signatures of the implementation that
        # follows, folded into it (fold_overloads).
        self.overload_stubs = set()
        self.constructor_fields = {}
        self.field_write_counts = {}
        self.field_type_origins = {}
        self.field_type_writes = set()
        # Every store of a class's field or class attribute, by (class
        # qname, name): the stored value when it is the one value of a plain
        # assignment, else None. field_call_origins caches what a field
        # stored once from an outside call carries (field_call_origin).
        self.field_stores = {}
        self.field_call_origins = {}
        # Each store written through a name that is not self or cls
        # (`Trade.session = scoped_session(...)`, `setattr(Trade, ...)`), as
        # (the name, the attribute or "*" for any, the stored value of a plain
        # one-target assignment or None, the storing scope), outside test
        # sources; resolved to its class once, by (class ref, attribute), in
        # class_name_stores, and what each class attribute read through its
        # class holds, in class_attribute_values (class_attribute_value).
        self.class_name_store_rows = []
        self.class_name_stores = None
        self.class_attribute_values = {}
        # The statements of each module by the statement list they stand in
        # (statement_block), every attribute written in the program by its
        # name (attribute_uses), and the classes each list field of a class
        # is closed over (registered_classes).
        self.statement_blocks = {}
        self.attribute_index = None
        self.registered_elements = {}
        # Each callable's return statements, as (returned expression or
        # None, the callable's scope), and the parameters of every def.
        self.return_nodes = {}
        self.parameter_refs = set()
        # The names some scope of each module declares global or nonlocal:
        # that scope's stores of them are not their owner's (stored_callees).
        self.shared_names = {}
        # Each module's nodes' parents and each scope's names read, for
        # stored_callees.
        self.parents = {}
        self.body_indexes = {}
        # The keyword-only ones, which no positional argument fills; each
        # call a def makes of its own parameter, and each call into a
        # repository callable with what it hands (hand_parameter_calls).
        self.keyword_only_parameters = set()
        self.parameter_calls = []
        self.calls_into = []
        # Each field an __init__ stores from its own parameter, by the stored
        # name's node, and each call on such a field's method whose class no
        # store names (type_field_parameters).
        self.stored_parameters = {}
        self.field_parameter_calls = []
        # Each class's written bases with the scope and module that define
        # it, and the repository classes they resolve to there (class_bases);
        # the classes naming each class as a base, and every class deriving
        # from each (derived_classes).
        self.class_definitions = {}
        self.class_base_refs = {}
        self.subclasses = {}
        self.derived_by_class = {}
        # The repository functions whose returned call is being resolved.
        self.producing = set()
        self.module_scopes = {}
        self.relations = []
        self.relations_by_key = {}
        # The words each callable or module body compares a value with, by
        # the scope's object ref (compared_word), and the rows of each
        # module-level table, by its variable's ref, None once a second
        # module-level assignment writes the name.
        self.compared_words = {}
        self.table_rows = {}
        # Keys a module-level variable is read with (attach_table_keys):
        # each read of a variable whose elements a subscript uses as keys,
        # with the subscripted variable and the subscript, the parameters
        # of a callable it uses so, by callable, and each read of a
        # variable handed to a repository callable as an argument.
        self.key_reads = []
        self.parameter_keys = {}
        self.handed_reads = []
        # Each read testing a value's membership in a variable, with its
        # scope, the value and its case (attach_memberships).
        self.membership_reads = []

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
                     patterns_observed=0, source_argument=None, extra_witnesses=(), dispatch=""):
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
        if dispatch:
            relation["dispatch"] = dispatch
        if location is not None:
            relation["location"] = location
        if source_argument is not None:
            relation["source_argument"] = source_argument
        guard = getattr(node, "repomap_guard", None)
        if guard is not None and kind in ("calls", "invokes_external", "passes_callback"):
            relation["guard"] = dict(guard)
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
            scope.node = module["tree"]
            scope.declared_all = declared_all(module["tree"])
            self.module_scopes[module["name"]] = scope
            collector = Collector(self, module, scope)
            collector.visit(module["tree"])

        # Every class's bases, resolved where the class is defined, before
        # any call is read: a read of self.<field> in a base class sees the
        # stores its subclasses write.
        for class_ref in sorted(self.class_definitions):
            _, _, module = self.class_definitions[class_ref]
            self.current_path = module["path"]
            resolver = RelationVisitor(self, module, self.module_scopes[module["name"]])
            for authority, base_ref in resolver.class_bases(class_ref):
                base = self.objects_by_ref.get(base_ref) if base_ref else None
                if authority == "local" and base is not None and base["kind"] == "type":
                    self.subclasses.setdefault(base_ref, []).append(class_ref)

        for module in decoded:
            self.current_path = module["path"]
            visitor = RelationVisitor(self, module, self.module_scopes[module["name"]])
            visitor.visit(module["tree"])
        self.attach_comparisons()
        self.attach_memberships()
        self.attach_table_keys()
        self.hand_parameter_calls()
        self.type_field_parameters()
        self.fold_overloads(decoded)
        for ref, rows in self.table_rows.items():
            value = self.objects_by_ref.get(ref)
            if rows and value is not None and value["kind"] == "variable":
                value["rows"] = rows

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
                name = value.get("text")
                # The field's __init__ store in its class, or in the base
                # the class inherits it from, and the store of each class
                # deriving from it: self may be an instance of any of them,
                # so each store is one alternative, none of them chosen.
                classes = self.field_store_classes(
                    class_ref, name,
                    lambda member: self.field_write_counts.get((member, name), 0) > 0 or name in self.constructor_fields.get(member, {}),
                ) if class_ref else []
                stores = []
                for member in classes:
                    field = self.constructor_fields.get(member, {}).get(name)
                    if field is None and len(classes) > 1:
                        # This class stores the field only outside __init__:
                        # an alternative whose value is not followed.
                        stores.append((member, {"kind": "unknown", "text": "field stored outside __init__"}))
                        continue
                    if not field or id(field) in active:
                        continue
                    if self.field_write_counts.get((member, name), 0) > 1:
                        field = {**field, "parts": [{"kind": "unknown", "text": "reassigned field", "anchor": field["anchor"]}]}
                    stores.append((member, field_initializers(field, active)))
                if len(stores) == 1:
                    member, result["initializer"] = stores[0]
                    init_ref = self.objects_by_qname.get(self.object_qname(member) + ".__init__", "")
                    result["owner"] = self.objects_by_ref.get(init_ref, {}).get("location")
                elif stores:
                    stores.sort(key=lambda store: location_key(store[1].get("anchor")))
                    result["initializer"] = {"kind": "alternatives", "parts": [field for _, field in stores]}
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

    def fold_overloads(self, modules):
        # A `@typing.overload` stub is a signature of the implementation of
        # its name that follows it, the def that runs and that a call
        # reaches, never a declaration of its own (PYTHON, ProgramIndex
        # Overload): the first later def of its qualified name, no stub,
        # written in the stub's own statement list or one enclosing it (a
        # stub under `if TYPE_CHECKING:` folds into the def after the if; a
        # stub in an if branch never into the else's). It keeps its
        # signature, place, lines and typed values on the implementation;
        # its parameters go with it, and what pointed at it points at the
        # implementation. A stub with no implementation after it (a .pyi
        # file, a Protocol) stays a declaration.
        if not self.overload_stubs:
            return
        chains = {}

        def walk(statements, chain):
            for position, statement in enumerate(statements):
                here = chain + ((id(statements), position),)
                if isinstance(statement, (ast.FunctionDef, ast.AsyncFunctionDef)):
                    ref = self.node_refs.get(id(statement))
                    if ref:
                        chains[ref] = here
                for field in ("body", "orelse", "finalbody"):
                    inner = getattr(statement, field, None)
                    if isinstance(inner, list) and inner and isinstance(inner[0], ast.stmt):
                        walk(inner, here)
                for handler in getattr(statement, "handlers", None) or []:
                    walk(handler.body, here)
                for case in getattr(statement, "cases", None) or []:
                    walk(case.body, here)

        for module in modules:
            walk(module["tree"].body, ())

        def follows(stub, chain):
            for depth in range(len(stub)):
                if len(chain) == depth + 1 and chain[:depth] == stub[:depth] and \
                        chain[depth][0] == stub[depth][0] and chain[depth][1] > stub[depth][1]:
                    return True
            return False

        by_qname = {}
        for ref in chains:
            qname = self.qnames_by_ref.get(ref)
            if qname:
                by_qname.setdefault(qname, []).append(ref)
        folded = {}
        for stub in sorted(self.overload_stubs):
            qname = self.qnames_by_ref.get(stub)
            if stub not in chains or not qname:
                continue
            later = [
                ref for ref in by_qname.get(qname, [])
                if ref not in self.overload_stubs and follows(chains[stub], chains[ref])
            ]
            if later:
                folded[stub] = min(later, key=lambda ref: location_key(self.objects_by_ref[ref].get("location")))
        if not folded:
            return
        removed = set(folded)
        changed = True
        while changed:
            changed = False
            for value in self.objects:
                ref = value["source_ref"]
                if ref not in removed and (value.get("container_ref") in removed or value.get("owner_ref") in removed):
                    removed.add(ref)
                    changed = True
        for stub, implementation in sorted(folded.items(), key=lambda item: location_key(self.objects_by_ref[item[0]].get("location"))):
            value = self.objects_by_ref[stub]
            overload = {"location": value["location"]}
            for key in ("signature", "end_line", "code_lines", "parameters", "results"):
                if value.get(key):
                    overload[key] = value[key]
            self.objects_by_ref[implementation].setdefault("overloads", []).append(overload)
        for value in self.objects_by_ref.values():
            if value.get("overloads"):
                value["overloads"].sort(key=lambda overload: location_key(overload.get("location")))
        self.objects = [value for value in self.objects if value["source_ref"] not in removed]
        for ref in removed:
            self.objects_by_ref.pop(ref, None)
        for qname, ref in list(self.objects_by_qname.items()):
            if ref in folded:
                self.objects_by_qname[qname] = folded[ref]
            elif ref in removed:
                del self.objects_by_qname[qname]

        # Relations are remapped in place: later passes reach them through
        # relations_by_key too.
        def remap(value):
            if isinstance(value, list):
                for position, item in enumerate(value):
                    if isinstance(item, str):
                        value[position] = folded.get(item, item)
                    else:
                        remap(item)
            elif isinstance(value, dict):
                for key, item in value.items():
                    if isinstance(item, str):
                        value[key] = folded.get(item, item)
                    else:
                        remap(item)

        kept = []
        for relation in self.relations:
            source = relation.get("from_ref")
            # A stub's own decorators (its overload, a staticmethod) are the
            # implementation's to state; its parameters go with it.
            if source in removed and (relation.get("kind") == "decorates" or source not in folded):
                continue
            targets = relation.get("to_refs", [])
            if any(target in removed and target not in folded for target in targets):
                relation["to_refs"] = [target for target in targets if target not in removed or target in folded]
                if targets and not relation["to_refs"]:
                    continue
            remap(relation)
            if "to_refs" in relation:
                relation["to_refs"] = sorted(set(relation["to_refs"]))
            kept.append(relation)
        self.relations = kept

    def attach_comparisons(self):
        # A value one scope compares with two or more different non-empty
        # words in two or more cases is one comparison (PROGRAM_INDEX): the
        # branches of an if/elif chain and the cases of a match on the same
        # expression. A lone comparison, or one condition naming several
        # words for one branch, is none.
        for ref, words in self.compared_words.items():
            value = self.objects_by_ref.get(ref)
            if value is None or value["kind"] not in ("function", "method", "lambda", "module"):
                continue
            groups = {}
            for word in words:
                groups.setdefault(word["key"], []).append(word)
            comparisons = []
            for group in groups.values():
                group.sort(key=lambda word: location_key(word["location"]))
                distinct = {word["word"] for word in group if word["word"]}
                branches = {word["case"] for word in group if word["word"]}
                if len(distinct) < 2 or len(branches) < 2:
                    continue
                cases, by_case = [], {}
                for word in group:
                    if word["case"] in by_case:
                        by_case[word["case"]]["words"].append(word["word"])
                        continue
                    item = {"form": word["form"], "words": [word["word"]], "location": word["location"]}
                    if word["branch"]:
                        item["branch"] = {"line": word["branch"][0], "end_line": word["branch"][1]}
                    if word.get("exclusive"):
                        item["exclusive"] = True
                    by_case[word["case"]] = item
                    cases.append(item)
                first = group[0]
                comparison = {"value": first["text"], "location": first["location"], "cases": cases}
                if first["origin"]:
                    comparison["origin"] = first["origin"]
                comparisons.append(comparison)
            if comparisons:
                comparisons.sort(key=lambda comparison: location_key(comparison["location"]))
                value["comparisons"] = comparisons

    def attach_memberships(self):
        # A membership test stays one only when its scope compares the same
        # value in no other case: `if command in READ_ONLY: ... elif command
        # in WRITES:` compares command case by case, as an if/elif chain of
        # words does, so neither read is a membership test (PROGRAM_INDEX).
        if not self.membership_reads:
            return
        cases = {}
        for ref, words in self.compared_words.items():
            for word in words:
                cases.setdefault((ref, word["key"]), set()).add(word["case"])
        for _, scope, key, case in self.membership_reads:
            cases.setdefault((scope, key), set()).add(case)
        relations = {relation["source_ref"]: relation for relation in self.relations}
        for read, scope, key, _ in self.membership_reads:
            if len(cases[(scope, key)]) > 1:
                for witness in relations[read]["witnesses"]:
                    if witness["kind"] == "membership":
                        witness["kind"] = "variable_read"

    def attach_table_keys(self):
        # A read of a module-level variable whose elements a subscript of
        # another module-level variable uses as keys gains a `keys` witness
        # naming the subscripted variable at the subscript (PROGRAM_INDEX):
        # the variable iterated where it is read, or handed as an argument
        # to a repository callable that iterates that parameter. A keyword
        # argument meets its parameter by name, a positional one by its
        # position as a call site counts (the receiver excluded).
        keyed = list(self.key_reads)
        for callee, position, keyword, read in self.handed_reads:
            for parameter, name, table, location in self.parameter_keys.get(callee, []):
                if (keyword and keyword == name) or (not keyword and position == parameter):
                    keyed.append((read, table, location))
        if not keyed:
            return
        relations = {relation["source_ref"]: relation for relation in self.relations}
        for read, table, location in keyed:
            relation = relations[read]
            # A table looked up with its own rows (`HELP[name] for name in
            # HELP`) holds no keys of another.
            if table in relation["to_refs"]:
                continue
            witness = {"kind": "keys", "object_ref": table, "detail": self.objects_by_ref[table]["name"], "location": location}
            if witness not in relation["witnesses"]:
                relation["witnesses"].append(witness)
                relation["witnesses_observed"] += 1

    def hand_parameter_calls(self):
        # A def calling its own parameter calls what the program's calls
        # into the def hand that parameter, as the C adapter joins what a
        # function's callers pass (PYTHON): one callable is the call's exact
        # target, several its alternatives, each named by a
        # function_value_store witness at the argument handing it. A call
        # handing any other value, or leaving the parameter to its default
        # or to a spread, and a def the program reaches otherwise than by a
        # call of it (handed over as a callable, a decorator, one of a
        # call's alternatives) leave the call unresolved, the callables
        # handed still its witnesses. The call runs a function value either
        # way. A test's code hands the program's own defs nothing: its
        # values run only under the test runner (test_paths).
        if not self.parameter_calls:
            return
        callees = {call[1] for call in self.parameter_calls}

        def tested(ref):
            location = (self.objects_by_ref.get(ref) or {}).get("location") or {}
            return location.get("path", "") in self.test_paths

        relations = {relation["source_ref"]: relation for relation in self.relations}
        entering, direct = {}, set()
        for call in self.calls_into:
            if call["callee"] in callees:
                entering.setdefault(call["callee"], []).append(call)
                direct.add(call["relation"])
        reached_otherwise = set()
        for relation in self.relations:
            if relation["kind"] == "imports" or relation["source_ref"] in direct:
                continue
            for callee in relation.get("to_refs", ()):
                if callee in callees and not (tested(relation["from_ref"]) and not tested(callee)):
                    reached_otherwise.add(callee)
        for relation_ref, callee, position, name, positional in self.parameter_calls:
            relation = relations[relation_ref]
            relation["dispatch"] = "function_value"
            targets, witnesses = [], []
            known = callee not in reached_otherwise
            for call in entering.get(callee, ()):
                if tested(relations[call["relation"]]["from_ref"]) and not tested(callee):
                    continue
                hand = call["hands"].get(("keyword", name))
                if hand is None and positional and position < call["positional"]:
                    hand = call["hands"].get(("position", position))
                if hand is None or not hand[0]:
                    known = False
                    continue
                handed, location = hand
                if handed not in targets:
                    targets.append(handed)
                witness = {
                    "kind": "function_value_store", "object_ref": handed,
                    "detail": bounded_text(self.objects_by_ref[handed]["name"] + " passed to " + self.objects_by_ref[callee]["name"]),
                }
                if location is not None:
                    witness["location"] = location
                if witness not in witnesses and witness not in relation["witnesses"]:
                    witnesses.append(witness)
            relation["witnesses"].extend(witnesses)
            relation["witnesses_observed"] += len(witnesses)
            if known and targets:
                relation["to_refs"] = sorted(targets)
                relation["resolution"] = "exact" if len(targets) == 1 else "alternatives"
                relation["targets_observed"] = len(targets)

    def type_field_parameters(self):
        # A field an __init__ stores once from its own parameter, never
        # rebound, holds the instance every static construction hands that
        # parameter (freqtrade's RPC.__init__(self, freqtrade) storing
        # self._freqtrade, built only by RPCManager(self) in FreqtradeBot, as
        # RPC(freqtrade) with RPCManager's own parameter): a call on the
        # field's method is that class's method, its own or inherited, exact
        # for one class and alternatives for several, each named by an
        # interface_field_assignment witness at the argument handing it, as
        # Go's constructor-injected interface fields are (GO). A parameter of
        # the calling def, never rebound, hands what that def's callers hand
        # it. A call handing any other value, or leaving the parameter to its
        # default or a spread, a construction the program never makes and a
        # test's construction of the program's class leave the call
        # unresolved.
        if not self.field_parameter_calls:
            return

        def tested(ref):
            location = (self.objects_by_ref.get(ref) or {}).get("location") or {}
            return location.get("path", "") in self.test_paths

        relations = {relation["source_ref"]: relation for relation in self.relations}
        entering = {}
        for call in self.calls_into:
            entering.setdefault(call["callee"], []).append(call)

        def handed(callee, position, name, positional, seen):
            # The classes every call into callee hands its parameter, each
            # with the argument handing it; None when any hands another value.
            if callee in seen:
                return None
            seen = seen | {callee}
            calls = [call for call in entering.get(callee, ())
                     if not (tested(relations[call["relation"]]["from_ref"]) and not tested(callee))]
            if not calls:
                return None
            found = []
            for call in calls:
                hand = call["instances"].get(("keyword", name))
                if hand is None and positional and position < call["positional"]:
                    hand = call["instances"].get(("position", position))
                if hand is None or hand[0] is None:
                    return None
                instance, location = hand
                if instance[0] == "class":
                    found.append((instance[1], location, callee))
                    continue
                nested = handed(*instance[1:], seen)
                if nested is None:
                    return None
                found.extend(nested)
            return found

        for relation_ref, stored, method in self.field_parameter_calls:
            parameter = self.stored_parameters.get(stored)
            relation = relations.get(relation_ref)
            if parameter is None or relation is None or relation["resolution"] != "unresolved":
                continue
            found = handed(*parameter, frozenset())
            if not found:
                continue
            targets, witnesses = [], []
            for class_ref, location, callee in found:
                target = ""
                for member in self.base_chain(class_ref):
                    target = self.objects_by_qname.get(self.object_qname(member) + "." + method, "")
                    if target:
                        break
                if not target or self.objects_by_ref[target]["kind"] not in ("function", "method"):
                    targets = []
                    break
                if target not in targets:
                    targets.append(target)
                witness = {
                    "kind": "interface_field_assignment", "object_ref": class_ref,
                    "detail": bounded_text(self.objects_by_ref[class_ref]["name"] + " handed to " + self.object_qname(callee)),
                }
                if location is not None:
                    witness["location"] = location
                if witness not in witnesses:
                    witnesses.append(witness)
            if not targets:
                continue
            relation["to_refs"] = sorted(targets)
            relation["resolution"] = "exact" if len(targets) == 1 else "alternatives"
            relation["targets_observed"] = len(targets)
            if len(targets) > 1:
                relation["dispatch"] = "interface"
            relation["witnesses"].extend(witnesses)
            relation["witnesses_observed"] += len(witnesses)

    def object_qname(self, ref):
        return self.qnames_by_ref.get(ref, "")

    def base_chain(self, class_ref):
        # The class, then its chain of single repository bases, as a member
        # lookup walks it: several bases, or a base outside the repository
        # or unknown, ends the chain.
        chain, current = [], class_ref
        while current and current not in chain:
            chain.append(current)
            bases = self.class_base_refs.get(current) or []
            if len(bases) != 1:
                break
            authority, base_ref = bases[0]
            base = self.objects_by_ref.get(base_ref) if base_ref else None
            if authority != "local" or base is None or base["kind"] != "type":
                break
            current = base_ref
        return chain

    def derived_classes(self, class_ref):
        # Every repository class that names this class as a base, directly
        # or through another, in source order of their definitions.
        cached = self.derived_by_class.get(class_ref)
        if cached is not None:
            return cached
        derived, pending = [], [class_ref]
        while pending:
            for subclass in self.subclasses.get(pending.pop(), ()):
                if subclass != class_ref and subclass not in derived:
                    derived.append(subclass)
                    pending.append(subclass)
        derived.sort(key=lambda ref: location_key(self.objects_by_ref.get(ref, {}).get("location")))
        self.derived_by_class[class_ref] = derived
        return derived

    def field_store_classes(self, class_ref, name, stores_field):
        # The classes whose stores of a field a read of self.<name> in this
        # class's code may see: the first class of its base chain that
        # stores it (the class itself first), and every class deriving from
        # it that stores it too, since self may be one of theirs.
        classes = []
        for member in self.base_chain(class_ref):
            if stores_field(member):
                classes.append(member)
                break
        classes.extend(member for member in self.derived_classes(class_ref) if stores_field(member))
        return classes


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
            self.scope.export_positions[name] = statement_position(self.statement)

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

    def record_store(self, target, binding=None, value=None, span=None):
        # One assignment of a name, with the callable its value names. Under a
        # branch, the name afterwards holds whatever the branch left there.
        # The store takes effect where its statement (an assignment
        # expression's own span) ends: a call in its value runs before it.
        if isinstance(target, ast.Name):
            self.scope.stores.setdefault(target.id, []).append({
                "binding": dict(binding) if binding is not None else None,
                "conditional": self.conditional_depth > self.scope.conditional_base,
                "node": value if binding is not None and value is not None else target,
                **store_span(span or self.statement or target),
            })
        elif isinstance(target, (ast.Tuple, ast.List)):
            for element in target.elts:
                self.record_store(element, span=span)

    def record_declaration(self, name, binding, node):
        # A def, class or import binds the name too. It is one more value the
        # name may hold, never what makes an assigned name conditional.
        self.scope.stores.setdefault(name, []).append({
            "binding": dict(binding),
            "conditional": self.conditional_depth > self.scope.conditional_base,
            "node": node, "declaration": True,
            **store_span(self.statement or node),
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
        # setattr(Trade, "session", ...) stores a class attribute through its
        # class's name as an assignment does, of no known value; a name not
        # written as a literal may be any attribute.
        if isinstance(node.func, ast.Name) and node.func.id == "setattr" and node.args and isinstance(node.args[0], ast.Name) and \
                self.module["path"] not in self.analyzer.test_paths:
            name = node.args[1].value if len(node.args) > 1 and isinstance(node.args[1], ast.Constant) and isinstance(node.args[1].value, str) else "*"
            self.analyzer.class_name_store_rows.append((node.args[0], name, None, self.scope))
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
        arguments = list(node.args.posonlyargs) + list(node.args.args) + list(node.args.kwonlyargs)
        if node.args.vararg is not None:
            arguments.append(node.args.vararg)
        if node.args.kwarg is not None:
            arguments.append(node.args.kwarg)
        # Decorators, defaults, annotations and type parameters are expressions
        # of the defining scope; the relation pass reads each of them there.
        header = list(node.decorator_list) + list(node.args.defaults) + list(node.args.kw_defaults)
        header += [argument.annotation for argument in arguments] + [node.returns]
        for value in header + list(getattr(node, "type_params", [])):
            if value is not None:
                self.visit(value)
        child = Scope(
            ref, qname, kind, parent,
            class_ref=parent.ref if kind == "method" else parent.class_ref,
            class_qname=parent.qname if kind == "method" else parent.class_qname,
        )
        child.conditional_base = self.conditional_depth
        self.analyzer.node_scopes[id(node)] = child
        child.node = node
        previous, self.scope = self.scope, child
        for argument in arguments:
            self.add_variable(argument.arg, argument, True)
            child.parameters.add(argument.arg)
            self.analyzer.parameter_refs.add(self.analyzer.node_refs[id(argument)])
            if any(argument is value for value in node.args.kwonlyargs):
                self.analyzer.keyword_only_parameters.add(self.analyzer.node_refs[id(argument)])
            if argument.annotation is not None:
                self.analyzer.variable_annotations[self.analyzer.node_refs[id(argument)]] = (argument.annotation, parent)
        for statement in node.body:
            self.visit(statement)
        self.scope = previous

    def visit_Return(self, node):
        # What a callable returns, for a field stored from its call; a bare
        # return is one more value, None.
        self.analyzer.return_nodes.setdefault(self.scope.ref, []).append((node.value, self.scope))
        self.generic_visit(node)

    def visit_Yield(self, node):
        # Calling a generator returns the generator, not what it returns.
        self.analyzer.suspended_callables.add(self.scope.ref)
        self.generic_visit(node)

    visit_YieldFrom = visit_Yield

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
        self.analyzer.class_definitions[ref] = (list(node.bases), parent, self.module)
        for value in list(node.decorator_list) + list(node.bases) + [keyword.value for keyword in node.keywords]:
            self.visit(value)
        for parameter in getattr(node, "type_params", []):
            self.visit(parameter)
        child = Scope(ref, qname, "type", parent, class_ref=ref, class_qname=qname)
        child.conditional_base = self.conditional_depth
        self.analyzer.node_scopes[id(node)] = child
        child.node = node
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
        for value in list(node.args.defaults) + list(node.args.kw_defaults):
            if value is not None:
                self.visit(value)
        child = Scope(ref, qname, "lambda", parent, class_ref=parent.class_ref, class_qname=parent.class_qname)
        child.conditional_base = self.conditional_depth
        self.analyzer.node_scopes[id(node)] = child
        child.node = node
        previous, self.scope = self.scope, child
        for argument in list(node.args.posonlyargs) + list(node.args.args) + list(node.args.kwonlyargs):
            self.add_variable(argument.arg, argument, True)
        child.parameters.update(argument.arg for argument in
                                list(node.args.posonlyargs) + list(node.args.args) + list(node.args.kwonlyargs) +
                                [value for value in (node.args.vararg, node.args.kwarg) if value is not None])
        self.visit(node.body)
        self.scope = previous

    def _target_reads(self, target):
        # A store target's receiver and index are expressions of this scope,
        # so a lambda or call there is declared like any other; the relation
        # pass reads the same parts.
        if isinstance(target, ast.Attribute):
            self.visit(target.value)
        elif isinstance(target, ast.Subscript):
            self.visit(target.value)
            self.visit(target.slice)
        elif isinstance(target, (ast.Tuple, ast.List)):
            for value in target.elts:
                self._target_reads(value)

    def record_field_store(self, target, value=None):
        # A store of an instance field (self.name in a method) or of a class
        # attribute in the class body. value is the assigned expression of a
        # plain one-target instance assignment, None for any other store
        # (augmented, deleted, unpacked, a loop or with target, and every
        # class attribute: a dataclass's field(...), a model's Column(...) or
        # a default is not what an instance holds).
        if isinstance(target, ast.Attribute) and isinstance(target.value, ast.Name) and target.value.id == "self" and self.scope.class_ref:
            key = (self.scope.class_qname, target.attr)
        elif isinstance(target, ast.Attribute) and self.scope.class_ref and (
                isinstance(target.value, ast.Name) and target.value.id == "cls" or
                isinstance(target.value, ast.Call) and isinstance(target.value.func, ast.Name) and target.value.func.id == "type"):
            # cls.name or type(self).name stores the attribute of whichever
            # class the method runs for: one more store, of no known value.
            key, value = (self.scope.class_qname, target.attr), None
        elif isinstance(target, ast.Attribute) and isinstance(target.value, ast.Name):
            if self.module["path"] not in self.analyzer.test_paths:
                self.analyzer.class_name_store_rows.append((target.value, target.attr, value, self.scope))
            return
        elif isinstance(target, ast.Name) and self.scope.kind == "type":
            key, value = (self.scope.qname, target.id), None
        elif isinstance(target, (ast.Tuple, ast.List)):
            for element in target.elts:
                self.record_field_store(element)
            return
        elif isinstance(target, ast.Starred):
            self.record_field_store(target.value)
            return
        else:
            return
        self.analyzer.field_stores.setdefault(key, []).append((value, self.scope))

    def visit_Assign(self, node):
        alias_binding = self.callable_alias_binding(node.value)
        self.visit(node.value)
        for target in node.targets:
            self.record_field_store(target, node.value if len(node.targets) == 1 else None)
        for target in node.targets:
            self._target_reads(target)
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
            self.record_field_store(node.target, node.value)
        self._target_reads(node.target)
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
        self.record_store(node.target, alias_binding, node.value, node)

    def visit_For(self, node):
        self.visit(node.iter)
        self.conditional_depth += 1
        self._target_reads(node.target)
        self.record_field_store(node.target)
        self.bind_targets(node.target, True)
        self.scope.unstored_names.update(part.id for part in ast.walk(node.target) if isinstance(part, ast.Name))
        for statement in node.body + node.orelse:
            self.visit(statement)
        self.conditional_depth -= 1

    visit_AsyncFor = visit_For

    def visit_AugAssign(self, node):
        if isinstance(node.target, ast.Name):
            self.record_export(node.target.id)
            self.scope.unstored_names.add(node.target.id)
        self.record_field_store(node.target)
        self.generic_visit(node)

    def visit_Delete(self, node):
        for target in node.targets:
            self.record_field_store(target)
            for child in ast.walk(target):
                if isinstance(child, ast.Name):
                    self.record_export(child.id)
        self.generic_visit(node)

    def visit_With(self, node):
        for item in node.items:
            if item.optional_vars is not None:
                self.record_field_store(item.optional_vars)
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
        self.analyzer.shared_names.setdefault(self.module["name"], set()).update(node.names)

    def visit_Nonlocal(self, node):
        self.scope.nonlocal_names.update(node.names)
        self.analyzer.shared_names.setdefault(self.module["name"], set()).update(node.names)

    def visit_Import(self, node):
        for alias in node.names:
            bound = alias.asname or alias.name.split(".")[0]
            module_name = alias.name if alias.asname else alias.name.split(".")[0]
            # An import an earlier module already made has its outside
            # symbol by now; that keeps it outside, not local.
            known = self.analyzer.objects_by_qname.get(module_name, "")
            external = module_name not in self.analyzer.modules and (
                not known or self.analyzer.objects_by_ref[known]["kind"] == "external_symbol")
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
                    self.scope.star_imports.append(statement_position(node))
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
        # The loops and comprehensions around the node being visited, each
        # with the scope it runs in (stored_callees).
        self.enclosing = []
        # Pattern receiver provenance is deliberately source ordered. The
        # declaration collector has already created stable variable objects,
        # but it must not let a later assignment explain an earlier call.
        self.pattern_bindings = {id(scope): {}}
        self.read_shadows = set()
        # A comprehension target's element (iterated), while it shadows,
        # and, by scope, how many try statements with handlers the visit is
        # in the body of (visit_Try).
        self.comprehension_elements = {}
        self.handled = {}
        # The statement whose plain assignment or with item binds names now
        # (bind_pattern_name): a name it rebinds in the statement list of its
        # previous binding keeps its value (statement_block).
        self.binding_statement = None

    def object(self, ref):
        return self.analyzer.objects_by_ref.get(ref)

    def import_target(self, module_name, imported_name="", allow_external=True, seen=None):
        qname = module_name + (("." + imported_name) if imported_name else "")
        canonical_module = self.analyzer.canonical_qname(module_name)
        scope = self.analyzer.module_scopes.get(canonical_module)
        if imported_name and scope is not None:
            key = (canonical_module, imported_name)
            seen = set() if seen is None else seen
            if key in seen:
                return "unknown", ""
            binding = scope.export_bindings.get(imported_name)
            if imported_name in scope.export_bindings and binding is None:
                return "unknown", ""
            # A star import may bind any name, under a branch too. Only one
            # unconditional binding the module writes after its last star
            # is still its own; a name a star could bind stays unknown.
            if scope.star_imports and (binding is None or scope.export_positions[imported_name] < max(scope.star_imports)):
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

    def visit(self, node):
        if not isinstance(node, LOOPS + COMPREHENSIONS):
            return super().visit(node)
        self.enclosing.append((node, self.scope))
        try:
            return super().visit(node)
        finally:
            self.enclosing.pop()

    def stored_callees(self, call):
        """The callables a call through a name assigned under a branch
        calls, each (origin, ref), with the witnesses of the stores that put
        them there: the stores that may reach the call (owner, 2026-09-30:
        several known targets are alternatives; Go's phi of a function value
        a branch chooses). [] leaves the call open: a store of anything but a
        function, a class or an outside symbol, or a name some value no store
        names may be in (a parameter, a for target, an augmented or opaque
        name, a name declared global or nonlocal, a module with star imports,
        a module or class name no store before the call always binds, which
        falls back to a builtin or a global)."""
        func = call.func
        if not isinstance(func, ast.Name) or func.id in self.read_shadows:
            return [], []
        name = func.id
        stores = self.branch_stores(name)
        owner = self.scope.owner(name)
        if not stores or owner is None or not self.settled_name(owner, name) or \
                (owner.kind == "type" and owner is not self.scope):
            return [], []
        if owner.node is None or self.body_index(owner)["evaluates"]:
            return [], []
        reaching = self.possible_stores(call, stores, owner)
        if not reaching or (owner.kind in ("module", "type") and all(store["conditional"] for store in reaching)):
            return [], []
        chosen = []
        for store in reaching:
            if store["binding"] is None or not self.settled_alias(owner, store):
                return [], []
            origin, ref = self.binding_target(store["binding"])
            value = self.object(ref) if ref else None
            if origin not in ("local", "external") or value is None or \
                    value["kind"] not in ("function", "method", "lambda", "type", "external_symbol"):
                return [], []
            chosen.append((origin, ref))
        return chosen, self.stored_function_witnesses(func, reaching)

    def possible_stores(self, call, stores, owner):
        """The stores that may be in the name when the call runs; [] when the
        call is not established (owner rule: an edge never claims a target no
        execution order would call, nor denies one some order calls).
        - A call in the name's own scope: the stores that reach it
          (ordered_stores), a list, set or dict comprehension running where
          it stands.
        - A call in a closure (a nested def, a lambda, a generator
          expression): those of each point its body may run at
          (closure_releases), what reaches a point the code there calls it
          at, or, from a point on at a time the index does not know, what
          reaches that point and every later store. No store reaching a
          point leaves the call open: there the name is still unbound, so a
          body run at once raises and one run later finds a later store."""
        closure = self.outermost_closure(call, owner.node)
        if closure is None:
            return self.ordered_stores(stores, statement_position(call), call, owner.node)
        # A body nested in another closure, an async def's, a generator's or
        # a generator expression's runs after the point that calls it.
        deferred = self.innermost_closure(call, owner.node) is not closure or isinstance(closure, (ast.AsyncFunctionDef, ast.GeneratorExp)) or \
            any(isinstance(node, (ast.Yield, ast.YieldFrom)) and self.innermost_closure(node, owner.node) is closure for node in ast.walk(closure))
        releases = self.closure_releases(closure, owner, frozenset())
        if not releases:
            return []
        possible = []
        for start, end, node, known in releases:
            reaching = self.ordered_stores(stores, start, node, owner.node)
            if not reaching:
                return []
            later = [] if known and not deferred else [store for store in stores if store["start"] >= end]
            possible += [store for store in reaching + later if not any(store is seen for seen in possible)]
        return possible

    def ordered_stores(self, stores, position, node, owner_node):
        # A store whose statement ends before the point may reach it, from
        # the last one no branch skips on; one after it only from the part of
        # the outermost loop or eager comprehension around the point that
        # runs again (repeating_spans).
        within = lambda spans, start, end: any(first <= start and end <= last for first, last in spans)
        before = sorted((store for store in stores if store["end"] <= position), key=lambda store: store["end"])
        last = max((index for index, store in enumerate(before) if not store["conditional"]), default=0)
        reaching = before[last:]
        loop = None
        current = self.parent_of(node) if node is not None else None
        while current is not None and current is not owner_node:
            if isinstance(current, LOOPS + EAGER_COMPREHENSIONS) and within(repeating_spans(current), position, position):
                loop = repeating_spans(current)
            current = self.parent_of(current)
        if loop:
            reaching += [store for store in stores if store["end"] > position and within(loop, store["start"], store["end"])]
        return reaching

    def parent_of(self, node):
        parents = self.analyzer.parents.get(self.module["name"])
        if parents is None:
            parents = {}
            for parent in ast.walk(self.module["tree"]):
                for child in ast.iter_child_nodes(parent):
                    parents[id(child)] = parent
            self.analyzer.parents[self.module["name"]] = parents
        return parents.get(id(node))

    def outermost_closure(self, node, owner_node):
        outer, current = None, self.parent_of(node)
        while current is not None and current is not owner_node:
            if isinstance(current, CLOSURES):
                outer = current
            current = self.parent_of(current)
        return outer

    def innermost_closure(self, node, owner_node):
        current = self.parent_of(node)
        while current is not None and current is not owner_node:
            if isinstance(current, CLOSURES):
                return current
            current = self.parent_of(current)
        return None

    def finally_after(self, statement, owner_node):
        # Whether a `finally` of the scope runs after the return.
        current = statement
        while current is not None and current is not owner_node:
            parent = self.parent_of(current)
            if isinstance(parent, TRIES) and parent.finalbody and not any(current is part for part in parent.finalbody):
                return True
            current = parent
        return False

    def closure_releases(self, closure, owner, seen):
        """The points of the name's own scope a closure's body may run at,
        each (start, end, node, known): known when the code there calls it
        (a lambda called where it is written, a call of the name holding it,
        a return handing it back), else from that point on at a time the
        index does not know (handed to a call, decorated, stored, read by
        another closure, a class's method, the module's end for what an
        importer may call)."""
        parent = self.parent_of(closure)
        point = lambda node, known: (statement_position(node), store_span(node)["end"], node, known)
        if isinstance(parent, ast.Call) and parent.func is closure:
            return [point(parent, True)]
        holder = self.parent_of(parent) if isinstance(parent, (ast.keyword, ast.Starred)) else parent
        if isinstance(holder, ast.Call) and holder.func is not closure and holder.func is not parent:
            return [point(holder, False)]
        if isinstance(parent, ast.Return):
            return [point(parent, not self.finally_after(parent, owner.node))]
        if isinstance(closure, (ast.FunctionDef, ast.AsyncFunctionDef)):
            if isinstance(parent, ast.ClassDef):
                return [(start, end, node, False) for start, end, node, _ in self.name_releases(parent.name, owner, seen)]
            releases = self.name_releases(closure.name, owner, seen)
            return releases + ([point(closure, False)] if closure.decorator_list else [])
        if isinstance(parent, ast.Assign) and len(parent.targets) == 1 and isinstance(parent.targets[0], ast.Name) and \
                len(owner.stores.get(parent.targets[0].id, [])) == 1:
            return self.name_releases(parent.targets[0].id, owner, seen)
        return [point(closure, False)]

    def name_releases(self, name, owner, seen):
        # The points a def, class or lambda named once may run at: each call
        # of the name in its scope, each return of it, each other read from
        # there on, the points that run a closure reading it, and, for a
        # module's, its end, since an importer may call it.
        if (id(owner), name) in seen:
            return []
        seen = seen | {(id(owner), name)}
        point = lambda node, known: (statement_position(node), store_span(node)["end"], node, known)
        releases = [(MODULE_END, MODULE_END, None, False)] if owner.kind == "module" else []
        for node in self.body_index(owner)["names"].get(name, []):
            scope_node = self.parent_of(node)
            while scope_node is not None and not isinstance(scope_node, DEFINITIONS + (ast.Module,)):
                scope_node = self.parent_of(scope_node)
            scope = owner if scope_node is owner.node else self.analyzer.node_scopes.get(id(scope_node))
            if scope is None or scope.owner(name) is not owner:
                continue
            inner = self.outermost_closure(node, owner.node)
            if inner is not None:
                releases += [(start, end, at, False) for start, end, at, _ in self.closure_releases(inner, owner, seen)]
                continue
            parent = self.parent_of(node)
            if isinstance(parent, ast.Call) and parent.func is node:
                releases.append(point(parent, True))
            elif isinstance(parent, ast.Return):
                releases.append(point(parent, not self.finally_after(parent, owner.node)))
            else:
                releases.append(point(node, False))
        return releases

    def body_index(self, owner):
        # Each name the scope's body reads (nested scopes included), and
        # whether it calls eval, exec, locals, globals or vars, which may
        # write any name.
        index = self.analyzer.body_indexes.get(id(owner))
        if index is None:
            index = {"names": {}, "evaluates": False}
            for node in ast.walk(owner.node):
                if isinstance(node, ast.Name) and isinstance(node.ctx, ast.Load):
                    index["names"].setdefault(node.id, []).append(node)
                    if node.id in ("eval", "exec", "locals", "globals", "vars") and isinstance(self.parent_of(node), ast.Call):
                        index["evaluates"] = True
            self.analyzer.body_indexes[id(owner)] = index
        return index

    def settled_name(self, owner, name):
        # Every value of the name is one of its owner's stores.
        return name not in owner.parameters and name not in owner.opaque_names and \
            name not in owner.unstored_names and name not in self.analyzer.shared_names.get(self.module["name"], ()) and \
            not (owner.kind == "module" and owner.star_imports)

    def settled_alias(self, owner, store):
        # `handler = accept` stores the value accept holds where the store
        # runs: accept's one store when it has only one, else any of them.
        value = store["node"]
        if store.get("declaration") or not isinstance(value, ast.Name):
            return True
        source = owner.owner(value.id)
        return source is not None and self.settled_name(source, value.id) and len(source.stores.get(value.id, [])) == 1

    def stored_function_witnesses(self, node, stores=None):
        # A call through a name assigned under a branch names each function
        # those assignments store, as the C adapter names a pointer's stores.
        # A call of an attribute of that name names each module or class
        # stored in it. A resolved call names the stores that reach it.
        root = node
        while isinstance(root, ast.Attribute):
            root = root.value
        if not isinstance(root, ast.Name):
            return []
        kinds = ("function", "method", "lambda", "type", "external_symbol")
        if root is not node:
            kinds += ("module", "package")
        witnesses = []
        for store in self.branch_stores(root.id) if stores is None else stores:
            if store["binding"] is None:
                continue
            authority, ref = self.binding_target(store["binding"])
            candidate = self.object(ref) if ref else None
            if authority == "unknown" or candidate is None or candidate["kind"] not in kinds:
                continue
            detail = candidate["name"] + " stored in " + root.id
            if store["conditional"]:
                detail += " under a condition"
            # The witness names what the store put there, the call's target
            # only when stored_callees resolves it.
            witness = {"kind": "function_value_store", "detail": bounded_text(detail), "object_ref": ref}
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
                if not ref and len(parts) == 1:
                    # self.helper() is the method the class declares or
                    # inherits along its chain of single repository bases.
                    ref = self.class_member(self.scope.class_ref, parts[0])
                if not ref and len(parts) > 1:
                    # The field the class stores, or inherits from a base.
                    field_ref = self.class_member(self.scope.class_ref, parts[0], ("variable",))
                    owner_ref = self.analyzer.field_type_origins.get(field_ref, "")
                    if owner_ref:
                        owner_name = self.analyzer.object_qname(owner_ref)
                        ref = self.analyzer.objects_by_qname.get(owner_name + "." + ".".join(parts[1:]), "")
                    else:
                        # self.parser.add_subparsers is the outside call's
                        # own member when the field holds one call's result.
                        origin = self.field_call_origin(self.scope.class_ref, parts[0])
                        if origin:
                            return "external", self.analyzer.ensure_external(self.object(origin)["name"] + "." + ".".join(parts[1:]))
                return ("local", ref) if ref else ("unknown", "")
            if isinstance(current, ast.Call):
                return self.call_result_member(current, parts)
            if isinstance(current, ast.Name):
                binding = self.scope.binding(current.id)
                if binding and binding["kind"] == "module":
                    if self.branch_stores(current.id):
                        return "unknown", ""
                    return self.imported_attribute(binding["module"], parts, binding.get("external", False))
                value_binding = self.pattern_binding(current.id)
                typed_parameter = value_binding and value_binding.get("annotation_origin") and len(parts) == 1
                origins = value_binding.get("origin_refs", []) if value_binding and (not origins_invalidated(value_binding) or typed_parameter) else []
                if len(origins) == 1:
                    owner_ref = self.produced_class(origins[0])
                    if owner_ref:
                        qname = self.analyzer.object_qname(owner_ref)
                        # A method the class inherits is its member too.
                        if len(parts) == 1:
                            target = self.class_member(owner_ref, parts[0])
                        else:
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

    def call_result_member(self, call, parts):
        # A member of what a call returns: Worker(name).run() is the
        # repository class's method, Path(p).open() the outside symbol's
        # (pathlib.Path.open), and a member of a member of that result, or
        # a call on a call's result, continues the same outside symbol
        # (schedule.Scheduler.every.day.at.do).
        authority, ref = self.resolved_call_target(call.func)
        if authority == "local":
            owner_ref = self.produced_class(ref)
            if owner_ref:
                target = self.class_member(owner_ref, parts[0]) if len(parts) == 1 else ""
                return ("local", target) if target else ("unknown", "")
        origin = self.produced_external(authority, ref)
        if origin:
            return "external", self.analyzer.ensure_external(self.object(origin)["name"] + "." + ".".join(parts))
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

    def conditional_callees(self, node):
        """The callables a conditional expression's branches name, nested
        conditions included, each (origin, ref): Lua 5.1.5's C f_parser and
        `(seconds if flag else millis)(ms)` alike. A constant condition takes
        the branch it selects. Any branch that names no callable the index
        knows returns [], and the call stays open."""
        if not isinstance(node, ast.IfExp):
            return []
        branches = [node.body, node.orelse]
        if isinstance(node.test, ast.Constant) and isinstance(node.test.value, (bool, int)):
            branches = [node.body] if node.test.value else [node.orelse]
        chosen = []
        for branch in branches:
            if isinstance(branch, ast.IfExp):
                nested = self.conditional_callees(branch)
                if not nested:
                    return []
                chosen.extend(nested)
                continue
            if not isinstance(branch, (ast.Name, ast.Attribute)):
                return []
            origin, ref = self.resolved_call_target(branch)
            if origin not in ("local", "external") or not ref:
                return []
            if origin == "local" and (self.object(ref) or {}).get("kind") not in ("function", "method", "lambda", "type"):
                return []
            chosen.append((origin, ref))
        return chosen

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
            # A name bound once to a repository function's call holds the
            # outside value that function produces (produced_external).
            factories = (binding or {}).get("origin_refs", [])
            if binding and len(factories) == 1 and not origins_invalidated(binding) \
                    and (binding.get("origin_resolution") or "exact") == "exact":
                origin = self.produced_external("local", factories[0])
                if origin:
                    return "external", self.analyzer.ensure_external(self.object(origin)["name"] + "." + node.attr)
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
        # A plain assignment or with item rebinding a name in the statement
        # list of its previous binding, whose value is known, runs after it
        # on every path through that list: reads that follow see this value
        # (`stmt = update(...); execute(stmt); stmt = update(...);
        # execute(stmt)`, two `with engine.begin() as connection` blocks).
        # A rebinding in another list, of a global or nonlocal name, or
        # after an unknown value leaves it unknown, and a nested callable
        # reading a rebound name never knows it (source_value).
        block = self.statement_block(self.binding_statement) if self.binding_statement is not None else None
        rebinds_in_place = (reassigned and block is not None and previous.get("block") is not None
                            and self.dominated_by(self.binding_statement, previous["block"])
                            and previous.get("source_origin") is not None
                            and node.id not in self.scope.global_names and node.id not in self.scope.nonlocal_names)
        # The class a call on the name reaches ignores the name's stores of
        # None: None has no member to call, so a call on the name is made on
        # its other value (`worker = None` before `worker = Worker(args)`).
        # A second store of anything else leaves the class unknown.
        none = origin.get("none", False)
        if none:
            none_only = previous is None or previous.get("none_only", False)
            origin_invalidated = previous is not None and origins_invalidated(previous)
        else:
            none_only = False
            origin_invalidated = previous is not None and (not previous.get("none_only", False) or origins_invalidated(previous))
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
            "origin_invalidated": origin_invalidated,
            "none_only": none_only,
            "value_candidate": value_candidate,
            "source_origin": source_origin if not invalidated or rebinds_in_place else None,
            "block": block,
            "rebound": reassigned,
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
        if isinstance(value, ast.Constant) and value.value is None:
            return {"observed": 0, "none": True}
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
        if owner and owner.get("name") == "__init__" and isinstance(value, ast.Name):
            # A field stored from the __init__'s own parameter holds what
            # the class's constructions hand it (type_field_parameters).
            parameter = self.called_parameter(value)
            if parameter is not None:
                self.analyzer.stored_parameters[id(value)] = (self.scope.ref,) + parameter
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

    def class_bases(self, class_ref):
        # What a class's written bases resolve to where the class is
        # defined, one (authority, ref) per base; a subscripted base
        # (Box[V]) is its class. Cached; a cycle through bases gives none.
        cache = self.analyzer.class_base_refs
        if class_ref in cache:
            return cache[class_ref] or []
        definition = self.analyzer.class_definitions.get(class_ref)
        if definition is None:
            return []
        cache[class_ref] = None
        bases, scope, module = definition
        previous = self.scope, self.module
        self.scope, self.module = scope, module
        try:
            resolved = [self.resolve(base.value if isinstance(base, ast.Subscript) else base) for base in bases]
        finally:
            self.scope, self.module = previous
        cache[class_ref] = resolved
        return resolved

    def class_member(self, class_ref, name, kinds=None):
        # The member of this name a class declares, or inherits along a
        # chain of single repository bases: the first class of the chain
        # that declares it. A class with several bases (whose order the
        # runtime decides), or a base outside the repository or unknown,
        # ends the chain with no member.
        seen = set()
        while class_ref and class_ref not in seen:
            seen.add(class_ref)
            ref = self.analyzer.objects_by_qname.get(self.analyzer.object_qname(class_ref) + "." + name, "")
            if ref:
                member = self.object(ref)
                return ref if member and (kinds is None or member["kind"] in kinds) else ""
            bases = self.class_bases(class_ref)
            if len(bases) != 1:
                return ""
            authority, base_ref = bases[0]
            base = self.object(base_ref) if base_ref else None
            if authority != "local" or base is None or base["kind"] != "type":
                return ""
            class_ref = base_ref
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

    def field_stores_seen(self, class_ref, name):
        # The stores of a field a read of self.<name> in this class may see:
        # those of the first class of its base chain that stores it, and
        # those of every class deriving from it (Analyzer.field_store_classes).
        stores = []
        for member in self.analyzer.field_store_classes(
                class_ref, name,
                lambda member: (self.analyzer.object_qname(member), name) in self.analyzer.field_stores):
            stores.extend(self.analyzer.field_stores[(self.analyzer.object_qname(member), name)])
        return stores

    def field_call_origin(self, class_ref, name):
        # A class's field stored exactly once, by a plain assignment of what
        # an outside call produces (self.parser = argparse.ArgumentParser(...)
        # in __init__ or any method, or a repository factory declared to
        # return an outside type), or of a parameter annotated with an
        # outside type, holds that value wherever the class reads it: its
        # origin is the outside symbol. The one store may be the class's
        # own or its base's (a subclass sees its base's field); a store in a
        # class deriving from it is a second store. A second store of any
        # kind (a class attribute, an augmented or deleted field, another
        # assignment) or a value nothing outside produced leaves it unknown.
        # The value is resolved in the scope that stores it, whichever
        # method this visitor is in.
        # A call on a local name resolves through that name's source-ordered
        # binding, so only a found origin or a disqualifying store is kept;
        # a store still being resolved is none (self.a = self.a.copy()).
        key = (class_ref, name)
        cache = self.analyzer.field_call_origins
        if cache.get(key) is not None:
            return cache[key]
        stores = self.field_stores_seen(class_ref, name)
        if len(stores) != 1 or not isinstance(stores[0][0], (ast.Call, ast.Name)):
            cache[key] = ""
            return ""
        if key in cache:
            return ""
        cache[key] = None
        value, scope = stores[0]
        previous = self.scope, self.module
        self.scope, self.module = scope, self.scope_module(scope)
        try:
            if isinstance(value, ast.Call):
                origin = self.produced_external(*self.resolved_call_target(value.func))
            else:
                origin = self.parameter_type_origin(value)
        finally:
            self.scope, self.module = previous
            del cache[key]
        if origin:
            cache[key] = origin
        return origin

    def scope_module(self, scope):
        # The module whose code a scope is.
        while scope.parent is not None:
            scope = scope.parent
        return self.analyzer.modules.get(scope.qname, self.module)

    def outside_type(self, annotation):
        # The outside symbol an annotation written as a plain name or
        # attribute (ccxt.Exchange) names where it is written; "" for a
        # repository class, a container, a union, a string or anything else.
        expression, declared_scope = annotation
        if not isinstance(expression, (ast.Name, ast.Attribute)):
            return ""
        previous = self.scope, self.module
        self.scope, self.module = declared_scope, self.scope_module(declared_scope)
        try:
            authority, ref = self.resolve(expression)
        finally:
            self.scope, self.module = previous
        candidate = self.object(ref) if ref else None
        if authority != "external" or not candidate or candidate["kind"] != "external_symbol":
            return ""
        # typing.Any declares no type and typing.Self the repository class
        # itself: neither is an outside class.
        if candidate["external"]["package_path"] in ("typing", "typing_extensions"):
            return ""
        return ref

    def parameter_type_origin(self, name):
        # A parameter of the storing def annotated with an outside type
        # (ccxt_object: ccxt.Exchange) and never reassigned there.
        binding = self.scope.bindings.get(name.id)
        if binding is None or binding["kind"] != "object" or binding["ref"] not in self.analyzer.parameter_refs:
            return ""
        if self.scope.stores.get(name.id) or name.id in self.scope.opaque_names:
            return ""
        annotation = self.analyzer.variable_annotations.get(binding["ref"])
        return self.outside_type(annotation) if annotation is not None else ""

    def produced_external(self, authority, ref):
        # The outside symbol whose value a call produces: the outside symbol
        # it calls, the outside type a repository function declares it
        # returns (_init_ccxt(...) -> ccxt.Exchange), or, for a repository
        # function with no declared return type, the outside call its one
        # return statement returns (return Application.builder()...build()).
        # Calling a coroutine function returns a coroutine and calling a
        # generator function a generator, neither what they return; any
        # other case is "".
        candidate = self.object(ref) if ref else None
        if candidate is None:
            return ""
        if authority == "external":
            return ref if candidate["kind"] == "external_symbol" else ""
        if authority != "local" or candidate["kind"] not in ("function", "method") \
                or candidate.get("signature", "").startswith("async "):
            return ""
        annotation = self.analyzer.return_annotations.get(ref)
        if annotation is not None:
            return self.outside_type(annotation)
        returns = self.analyzer.return_nodes.get(ref, [])
        if ref in self.analyzer.suspended_callables or len(returns) != 1 or not isinstance(returns[0][0], ast.Call):
            return ""
        value, scope = returns[0]
        active = self.analyzer.producing
        if ref in active:
            return ""
        active.add(ref)
        previous = self.scope, self.module
        self.scope, self.module = scope, self.scope_module(scope)
        try:
            authority, returned = self.resolved_call_target(value.func)
        finally:
            self.scope, self.module = previous
            active.discard(ref)
        target = self.object(returned) if returned else None
        return returned if authority == "external" and target and target["kind"] == "external_symbol" else ""

    def self_field_call(self, node):
        # The call a class's field (self.name) is stored from, when it is
        # the field's one store it may see and a plain assignment.
        if not (isinstance(node, ast.Attribute) and isinstance(node.value, ast.Name) and node.value.id == "self" and self.scope.class_qname):
            return None
        stores = self.field_stores_seen(self.scope.class_ref, node.attr)
        if len(stores) == 1 and isinstance(stores[0][0], ast.Call):
            return stores[0]
        return None

    def class_name_store_index(self):
        # Every store through a class's name (Collector.record_field_store,
        # setattr), by (the class's ref, the attribute or "*"), each name
        # resolved where it is written; a name no repository class answers
        # stores no class attribute.
        index = self.analyzer.class_name_stores
        if index is not None:
            return index
        index = {}
        for name, attribute, value, scope in self.analyzer.class_name_store_rows:
            previous = self.scope, self.module
            self.scope, self.module = scope, self.scope_module(scope)
            try:
                authority, ref = self.resolve(name)
            finally:
                self.scope, self.module = previous
            candidate = self.object(ref) if ref else None
            if authority == "local" and candidate is not None and candidate["kind"] == "type":
                index.setdefault((ref, attribute), []).append((value, scope))
        self.analyzer.class_name_stores = index
        return index

    def class_attribute_stores(self, class_ref, name):
        # The stores a read of Class.<name> may see: through the name of
        # the first class of its chain of single repository bases storing it,
        # and of every class deriving from it, with every store of the name
        # in those classes' bodies or methods (self.name, cls.name), which
        # hold no value this read is given.
        index = self.class_name_store_index()

        def stores(member):
            found = list(index.get((member, name), [])) + list(index.get((member, "*"), []))
            found += [(None, scope) for _, scope in self.analyzer.field_stores.get((self.analyzer.object_qname(member), name), [])]
            return found

        result = []
        for member in self.analyzer.field_store_classes(class_ref, name, lambda member: bool(stores(member))):
            result.extend(stores(member))
        return result

    def class_attribute_value(self, node):
        # A class attribute the program stores once, by a plain assignment
        # through its class's name anywhere (`Trade.session =
        # scoped_session(...)` in init_db), holds that store's value wherever
        # the class is read so: the call's result, or what the class
        # attribute it is given holds (`Order.session = Trade.session`). A
        # second store of any kind, in the class, a base it inherits the
        # attribute from or a class deriving from it (a class body value,
        # self.name, cls.name, setattr), leaves it unknown; a test's store is
        # not the program's. Call resolution is not changed.
        if not (isinstance(node, ast.Attribute) and isinstance(node.value, ast.Name)) or node.value.id in ("self", "cls"):
            return None
        authority, class_ref = self.resolve(node.value)
        candidate = self.object(class_ref) if class_ref else None
        if authority != "local" or candidate is None or candidate["kind"] != "type":
            return None
        key = (class_ref, node.attr)
        cache = self.analyzer.class_attribute_values
        if key in cache:
            return cache[key]
        cache[key] = None
        stores = self.class_attribute_stores(class_ref, node.attr)
        result = None
        if len(stores) == 1 and stores[0][0] is not None:
            value, scope = stores[0]
            previous = self.scope, self.module
            self.scope, self.module = scope, self.scope_module(scope)
            try:
                if isinstance(value, ast.Call):
                    result = {"kind": "call_result", "text": safe_expression_name(value.func),
                              "anchor": callee_location(self.module["path"], value.func)}
                elif isinstance(value, ast.Attribute):
                    result = self.class_attribute_value(value)
            finally:
                self.scope, self.module = previous
        cache[key] = result
        return result

    def self_field_origin(self, node):
        if isinstance(node, ast.Attribute) and isinstance(node.value, ast.Name) and node.value.id == "self" and self.scope.class_qname:
            return self.field_call_origin(self.scope.class_ref, node.attr)
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
            result = {"receiver_ref": ref} if authority == "local" and ref else {}
            origin = self.self_field_origin(callee.value)
            if origin:
                result.update({"receiver_origin_refs": [origin], "receiver_origin_resolution": "exact", "receiver_origins_observed": 1})
            return result
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

    def partial_callable(self, node):
        # functools.partial(f, ...) given as an argument hands f over, as a
        # bare f would: the repository callable its first argument names,
        # through a partial of a partial too.
        if not isinstance(node, ast.Call) or not node.args or isinstance(node.args[0], ast.Starred):
            return None
        authority, ref = self.resolve(node.func)
        partial = self.object(ref) if ref else None
        if authority != "external" or partial is None or partial["name"] != "functools.partial":
            return None
        handed = self.partial_callable(node.args[0])
        if handed is not None:
            return handed
        authority, ref = self.resolve(node.args[0])
        value = self.object(ref) if ref else None
        if authority in ("local", "literal") and value and value["kind"] in ("function", "method", "lambda"):
            return authority, ref
        return None

    def pattern_argument_authority(self, node):
        handed = self.partial_callable(node)
        if handed is not None:
            resolution, refs = self.pattern_resolution(handed)
            return {"object_refs": refs, "resolution": resolution, "objects_observed": 1}
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
                    # A nested callable runs later: an enclosing name bound
                    # more than once may hold any of its values then.
                    if scope is not self.scope and binding.get("rebound"):
                        return {**unknown, "text": node.id}
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
            # A class attribute stored once through its class's name holds
            # that store's value (class_attribute_value).
            stored = self.class_attribute_value(node)
            if stored is not None:
                return stored
            # A field its class stores once, from a call, holds that call's
            # result, as a local name bound to a call does.
            stored = self.self_field_call(node)
            if stored is not None:
                call, scope = stored
                return {"kind": "call_result", "text": safe_expression_name(call.func),
                        "anchor": callee_location(self.scope_module(scope)["path"], call.func)}
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
        overload, outside = False, True
        for decorator in node.decorator_list:
            target = decorator.func if isinstance(decorator, ast.Call) else decorator
            detail = self.expression_name(target)
            pattern, patterns_observed = (None, 0)
            if isinstance(decorator, ast.Call):
                pattern, patterns_observed = self.relation_pattern(
                    decorator, "decorator_call", defined_ref,
                )
            resolved = self.resolve(target)
            # A stub is known by what its decorator resolves to, never by
            # the decorator's spelling (`ov`, `t.overload`).
            decorator_object = self.analyzer.objects_by_ref.get(resolved[1]) if resolved[1] else None
            if resolved[0] == "external" and decorator_object is not None and decorator_object.get("name") in ("typing.overload", "typing_extensions.overload"):
                overload = True
            elif resolved[0] != "external":
                outside = False
            self.emit_decorator(
                resolved, defined_ref, decorator,
                "decorator", detail, exact_authorities=("literal",),
                pattern=pattern, patterns_observed=patterns_observed,
            )
            if isinstance(decorator, ast.Call):
                for argument in list(decorator.args) + [value.value for value in decorator.keywords]:
                    self.visit(argument)
        # A repository decorator on a stub runs at import and may keep it:
        # such a stub stays a declaration.
        if overload and outside and isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef)):
            self.analyzer.overload_stubs.add(defined_ref)
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
        self.rebind_definition(node.name)

    def rebind_definition(self, name):
        # A def or class rebinds its name where it is written: a value the
        # name held before is no longer what it reads.
        if name in self.current_pattern_bindings():
            self.bind_nonvalue_name(name)

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
        self.rebind_definition(node.name)

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
            # The right side of `in`/`not in` is read as a set a value is
            # tested against (visit_Compare): a `membership` witness.
            membership = getattr(node, "repomap_membership", None)
            read = self.emit_resolved("reads", self.scope.ref, resolved, node,
                                      "membership" if membership else "variable_read", safe_expression_name(node))
            if membership:
                self.analyzer.membership_reads.append((read, self.scope.ref) + membership)
            return read
        return ""

    def module_variable(self, ref):
        # A variable a module body declares.
        value = self.object(ref) if ref else None
        return bool(value) and value["kind"] == "variable" and \
            (self.object(value.get("container_ref", "")) or {}).get("kind") == "module"

    def iterated(self, node):
        # Visits what a loop iterates and says what its elements are, for a
        # subscript keyed by them (record_table_key): a parameter of this
        # callable, by position and name, while the name is still bound to
        # it, or a module-level variable, by the relation reading it here.
        if not isinstance(node, ast.Name) or not isinstance(node.ctx, ast.Load):
            self.visit(node)
            return None
        resolved = self.read_name(node.id)
        read = self.record_read(node, resolved)
        origin = (self.current_pattern_bindings().get(node.id) or {}).get("source_origin") or {}
        if origin.get("kind") == "parameter" and node.id not in self.read_shadows:
            return {"parameter": origin["position"], "name": origin["text"]}
        if read and self.module_variable(resolved[1]):
            return {"read": read}
        return None

    def element_of(self, name):
        # What a name holds as a loop's element here, if it still does: a
        # comprehension's target, or a for statement's until it is bound
        # again (the binding is replaced).
        if name in self.read_shadows:
            return self.comprehension_elements.get(name)
        return (self.current_pattern_bindings().get(name) or {}).get("element_of")

    def visit_Subscript(self, node):
        if isinstance(node.ctx, ast.Load) and isinstance(node.slice, ast.Name):
            self.record_table_key(node)
        self.generic_visit(node)

    def record_table_key(self, node):
        # `OPTIONS[val]` with val an element of a loop over a parameter or a
        # module-level variable, OPTIONS a module-level variable: those
        # elements are keys OPTIONS is read with (attach_table_keys).
        element = self.element_of(node.slice.id)
        if element is None or id(node) not in element["reached"]:
            return
        subscripted = node.value
        resolved = ("unknown", "")
        if isinstance(subscripted, ast.Name):
            resolved = self.read_name(subscripted.id)
        elif isinstance(subscripted, ast.Attribute) and isinstance(subscripted.value, ast.Name):
            # `options.OPTIONS[val]`: a module's variable, as visit_Attribute
            # resolves one.
            base = self.read_name(subscripted.value.id)
            if (self.object(base[1]) or {}).get("kind") in ("module", "package") and self.resolve(subscripted.value) == base:
                resolved = self.resolve(subscripted)
        if not self.module_variable(resolved[1]):
            return
        location = source_location(self.module["path"], node)
        if "read" in element:
            self.analyzer.key_reads.append((element["read"], resolved[1], location))
        else:
            self.analyzer.parameter_keys.setdefault(self.scope.ref, []).append(
                (element["parameter"], element["name"], resolved[1], location))

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
        previous, previous_elements = self.read_shadows, self.comprehension_elements
        self.read_shadows, self.comprehension_elements = set(previous), dict(previous_elements)
        # One generator with no condition evaluates its result for every
        # element: the subscripts that result always evaluates are keyed by
        # the element (record_table_key).
        results = [node.key, node.value] if isinstance(node, ast.DictComp) else [node.elt]
        single = len(node.generators) == 1 and not node.generators[0].ifs and not self.handled.get(id(self.scope))
        for generator in node.generators:
            element = self.iterated(generator.iter)
            names = [part.id for part in ast.walk(generator.target) if isinstance(part, ast.Name)]
            self.read_shadows.update(names)
            for name in names:
                self.comprehension_elements[name] = None
            if element is not None and single and isinstance(generator.target, ast.Name):
                reached = set()
                for result in results:
                    reached |= unconditional_subscripts(result)
                self.comprehension_elements[generator.target.id] = {**element, "reached": reached}
            for condition in generator.ifs:
                self.visit(condition)
        if isinstance(node, ast.DictComp):
            self.visit(node.key)
            self.visit(node.value)
        else:
            self.visit(node.elt)
        self.read_shadows, self.comprehension_elements = previous, previous_elements

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

    def builtin_witness(self, func):
        """A bare call of a name no scope of its module binds calls Python's
        builtin of that name, which Python looks up last (PYTHON
        "Builtins"). The call stays unresolved: every builtin is no outside
        symbol of the index. Its witness names the builtin."""
        name = self.builtin_name(func)
        if not name:
            return ()
        witness = {"kind": "builtin", "detail": name}
        location = callee_location(self.module["path"], func)
        if location is not None:
            witness["location"] = location
        return (witness,)

    def builtin_name(self, func):
        """The builtin a bare name falls back to, or "" when the name may be
        something else: any binding of it in its scope or a function or
        module around it (a def, a class, an import, an assignment, a
        parameter, a for, with, except or match target, a comprehension's
        target), a global or nonlocal declaration of it anywhere in the
        module, or a star import that may bind any name leaves it unknown, as
        a name the builtins module does not define does. A class body around
        the call is no scope it is looked up in. The builtins are the running
        interpreter's (BUILTIN_NAMES): a later Python's new builtin is a
        module global to an earlier one."""
        if not isinstance(func, ast.Name) or func.id not in BUILTIN_NAMES or func.id in self.read_shadows:
            return ""
        name = func.id
        if name in self.analyzer.shared_names.get(self.module["name"], ()) or \
                self.module["name"] + "." + name in self.analyzer.objects_by_qname:
            return ""
        scope = self.scope
        while scope is not None:
            # A class body around the call binds nothing it sees (Scope.owner).
            visible = scope is self.scope or scope.kind != "type"
            if visible and (name in scope.bindings or name in scope.stores or name in scope.parameters or
                            name in scope.unstored_names or name in scope.opaque_names or
                            name in scope.global_names or name in scope.nonlocal_names) or scope.star_imports:
                return ""
            scope = scope.parent
        return name

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
        # A callee chosen by a condition whose every branch names a callable
        # calls one of them, the condition deciding which (C.md's conditional
        # callee). One callable, a constant condition or both branches the
        # same, is the ordinary call; several are alternatives below.
        chosen = self.conditional_callees(node.func) if not resolved[1] else []
        # A call through a name assigned under a branch calls what the
        # stores reaching it put there, alike (stored_callees).
        stored = []
        if not resolved[1] and not chosen:
            chosen, stored = self.stored_callees(node)
        if len({ref for _, ref in chosen}) == 1:
            resolved, chosen = chosen[0], []
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
            # Calling a repository class constructs an instance: the call
            # stays the class's, with its arguments and result, and it also
            # runs the class's __init__, its own or the one it inherits.
            constructed = resolved[0] == "local" and called is not None and called["kind"] == "type"
            if constructed:
                invocation = "construct"
            # A call on an element of a list closed over its classes
            # (registered_classes) is each class's method of that name: its
            # implementations, several of them alternatives.
            members = self.registered_members(node.func) if not resolved[1] else []
            if chosen:
                # Several callables a condition or a branch's stores choose
                # between: alternatives through a function value, the owner's
                # rule for several known targets. Classes are constructed.
                refs = sorted({ref for _, ref in chosen})
                names = [ref_name for _, ref_name in sorted((ref, self.object(ref)["name"]) for ref in refs)]
                witnesses = stored
                if not stored:
                    witness = {"kind": "python_conditional_callee",
                               "detail": bounded_text(ast.unparse(node.func) + " calls " + " or ".join(names) + ", as its condition decides")}
                    location = source_location(self.module["path"], node.func)
                    if location is not None:
                        witness["location"] = location
                    witnesses = [witness]
                if all(self.object(ref)["kind"] == "type" for ref in refs):
                    invocation = "construct"
                call_relation_ref = self.analyzer.add_relation(
                    "invokes_external" if all(origin == "external" for origin, _ in chosen) else "calls",
                    self.scope.ref, refs, "alternatives", node, "callsite",
                    name, invocation=invocation, targets_observed=len(refs), source_expression=source_expression,
                    witness_callee=node.func, patterns=[pattern] if pattern else [], patterns_observed=patterns_observed,
                    extra_witnesses=tuple(witnesses), dispatch="function_value",
                )
            elif members:
                call_relation_ref = self.analyzer.add_relation(
                    "calls", self.scope.ref, members, "alternatives" if len(members) > 1 else "exact", node, "callsite",
                    name, invocation=invocation, targets_observed=len(members), source_expression=source_expression,
                    witness_callee=node.func, patterns=[pattern] if pattern else [], patterns_observed=patterns_observed,
                    dispatch="interface" if len(members) > 1 else "",
                )
            else:
                call_relation_ref = self.emit_resolved(
                    kind, self.scope.ref, resolved, node, "callsite", name, invocation,
                    exact_authorities=("literal",), source_expression=source_expression,
                    witness_callee=node.func, pattern=pattern, patterns_observed=patterns_observed,
                    extra_witnesses=tuple(self.stored_function_witnesses(node.func)) + self.builtin_witness(node.func) if not resolved[1] else (),
                )
                parameter = self.called_parameter(node.func) if not resolved[1] else None
                if parameter is not None and call_relation_ref:
                    self.analyzer.parameter_calls.append((call_relation_ref, self.scope.ref) + parameter)
                stored = self.field_parameter_store(node.func) if not resolved[1] else None
                if stored is not None and call_relation_ref:
                    self.analyzer.field_parameter_calls.append((call_relation_ref,) + stored)
            initializer = self.class_member(resolved[1], "__init__", ("method",)) if constructed else ""
            if initializer:
                initializer_ref = self.analyzer.add_relation(
                    "calls", self.scope.ref, [initializer], "exact", node, "constructor",
                    name, invocation="construct", targets_observed=1,
                    source_expression=source_expression, witness_callee=node.func,
                )

        arguments = [(argument, position, "") for position, argument in enumerate(node.args, 1)]
        arguments.extend((value.value, 0, value.arg or "") for value in node.keywords)
        # A module-level variable handed to a repository callable: its read
        # meets the callable's parameter in attach_table_keys, by keyword or
        # by position before any starred argument. A method called through
        # its class is handed its receiver as its first positional argument,
        # which a parameter's position does not count: only its keyword
        # arguments meet parameters.
        callee = self.object(resolved[1]) if resolved[0] == "local" and resolved[1] else None
        positional = next((position for position, argument in enumerate(node.args, 1) if isinstance(argument, ast.Starred)), len(node.args) + 1)
        if callee is not None and callee["kind"] == "method" and isinstance(node.func, ast.Attribute) and \
                (self.object(self.resolve(node.func.value)[1]) or {}).get("kind") == "type":
            positional = 1
        # What the call hands each parameter it meets: the callable an
        # argument names, or "" for any other value (hand_parameter_calls).
        # A class's call hands its __init__ the same arguments.
        entered = ""
        if callee is not None and callee["kind"] in ("function", "method") and call_relation_ref:
            entered = (resolved[1], call_relation_ref)
        elif not dynamic_only and initializer:
            entered = (initializer, initializer_ref)
        hands, instances = {}, {}
        for argument, position, keyword in arguments:
            authority, ref = self.partial_callable(argument) or self.resolve(argument)
            value = self.object(ref) if ref else None
            handed = authority in ("local", "literal") and value and value["kind"] in ("function", "method", "lambda")
            if entered and (keyword or 0 < position < positional):
                key = ("keyword", keyword) if keyword else ("position", position)
                hands[key] = (ref if handed else "", source_location(self.module["path"], argument))
                instances[key] = (self.instance_handed(argument), source_location(self.module["path"], argument))
            if handed:
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
            if callee is not None and callee["kind"] in ("function", "method") and \
                    isinstance(argument, ast.Name) and isinstance(argument.ctx, ast.Load) and \
                    (keyword or position < positional):
                argument_resolved = self.read_name(argument.id)
                read = self.record_read(argument, argument_resolved)
                if read and self.module_variable(argument_resolved[1]):
                    self.analyzer.handed_reads.append((resolved[1], position, keyword, read))
            else:
                self.visit(argument)
        if entered:
            self.analyzer.calls_into.append({
                "callee": entered[0], "relation": entered[1], "path": self.module["path"],
                "positional": positional, "spread": any(value.arg is None for value in node.keywords),
                "hands": hands, "instances": instances,
            })
        self.visit(node.func)

    def instance_handed(self, argument):
        # The instance an argument hands a parameter (type_field_parameters):
        # self in a method is its class's; a name bound once to a class's
        # construction, or a parameter annotated with a repository class, is
        # that class's; a class call, or a factory declared to return a
        # repository class, makes one. A parameter of the calling def, never
        # rebound, hands what that def's callers hand it. None for anything
        # else.
        if isinstance(argument, ast.Name):
            if argument.id == "self" and self.scope.class_ref:
                return ("class", self.scope.class_ref)
            binding = self.pattern_binding(argument.id)
            typed = binding and binding.get("annotation_origin")
            origins = binding.get("origin_refs", []) if binding and (not origins_invalidated(binding) or typed) else []
            if len(origins) == 1:
                owner = self.produced_class(origins[0])
                if owner:
                    return ("class", owner)
            parameter = self.called_parameter(argument)
            if parameter is not None:
                return ("parameter", self.scope.ref) + parameter
            return None
        if isinstance(argument, ast.Call):
            authority, ref = self.resolved_call_target(argument.func)
            owner = self.produced_class(ref) if authority == "local" else ""
            return ("class", owner) if owner else None
        return None

    def field_parameter_store(self, func):
        # A call of a method on self.<field> whose one store it may see is a
        # plain name (the __init__'s parameter, type_field_parameters): the
        # store's node and the method's name.
        if not (isinstance(func, ast.Attribute) and isinstance(func.value, ast.Attribute) and
                isinstance(func.value.value, ast.Name) and func.value.value.id == "self" and self.scope.class_ref):
            return None
        stores = self.field_stores_seen(self.scope.class_ref, func.value.attr)
        if len(stores) != 1 or not isinstance(stores[0][0], ast.Name):
            return None
        return id(stores[0][0]), func.attr

    def called_parameter(self, func):
        # A call of the def's own parameter, which the def never rebinds:
        # the parameter's position (the receiver not counted), its name and
        # whether a positional argument can fill it (hand_parameter_calls).
        if not isinstance(func, ast.Name) or func.id in self.read_shadows:
            return None
        binding = self.scope.bindings.get(func.id)
        if binding is None or binding["kind"] != "object" or binding["ref"] not in self.analyzer.parameter_refs:
            return None
        if self.scope.stores.get(func.id) or func.id in self.scope.opaque_names:
            return None
        origin = (self.current_pattern_bindings().get(func.id) or {}).get("source_origin") or {}
        if origin.get("kind") != "parameter":
            return None
        return origin["position"], origin["text"], binding["ref"] not in self.analyzer.keyword_only_parameters

    def visit_Assign(self, node):
        for target in node.targets:
            self._attribute_write(target)
            self._target_reads(target)
        self.visit(node.value)
        if len(node.targets) == 1:
            self.record_table(node.targets[0], node.value)
        origin = self.assignment_origin(node.value)
        initializer = self.initializer_value_candidate(node.value)
        source_origin = self.source_value(node.value)
        previous, self.binding_statement = self.binding_statement, (node if len(node.targets) == 1 else None)
        try:
            for target in node.targets:
                if self.bind_parallel(target, node.value):
                    continue
                self.bind_pattern_target(target, origin, initializer, source_origin)
                self.bind_field_type(target, node.value)
        finally:
            self.binding_statement = previous

    def bind_parallel(self, target, value):
        # `cmd, args = args[0], args[1:]` binds each name to its own value,
        # every value read before any name is bound.
        if not isinstance(target, (ast.Tuple, ast.List)) or not isinstance(value, (ast.Tuple, ast.List)) or \
                len(target.elts) != len(value.elts) or \
                any(isinstance(item, ast.Starred) for item in list(target.elts) + list(value.elts)):
            return False
        bound = [(item, self.assignment_origin(written), self.initializer_value_candidate(written), self.source_value(written))
                 for item, written in zip(target.elts, value.elts)]
        for item, origin, initializer, source_origin in bound:
            self.bind_pattern_target(item, origin, initializer, source_origin)
        return True

    def record_table(self, target, value):
        # A module-level variable written once whose value is a list, tuple,
        # set or dict of one shape: every element a string, a call to one
        # callee, or a tuple of constants of one length, and a dict's keys
        # strings (its values of one such shape, or constants). Each element
        # is a row of the string literals it writes in order: a dict's key,
        # then a call's positional and keyword words, as written. A nested
        # dict or a mixed collection is no table. build.go keeps the rows
        # only of a table a function outside tests reads.
        if self.scope.kind != "module" or not isinstance(target, ast.Name):
            return
        binding = self.scope.bindings.get(target.id)
        if not binding or binding.get("kind") != "object":
            return
        ref = binding["ref"]
        if ref in self.analyzer.table_rows:
            self.analyzer.table_rows[ref] = None
            return
        self.analyzer.table_rows[ref] = self.written_rows(value)

    def written_rows(self, value):
        path = self.module["path"]

        def text(node):
            return isinstance(node, ast.Constant) and isinstance(node.value, str)

        def literal(node, field=""):
            location = source_location(path, node)
            return {"value": node.value, "location": location, **({"field": field} if field else {})} if location else None

        def shape(node):
            if text(node):
                return "string"
            if isinstance(node, ast.Call):
                callee = safe_expression_name(node.func)
                return "call " + callee if callee else None
            if isinstance(node, (ast.Tuple, ast.List)) and node.elts and all(isinstance(item, ast.Constant) for item in node.elts):
                return "tuple %d" % len(node.elts)
            if isinstance(node, ast.Constant):
                return "constant"
            return None

        def words(node):
            if text(node):
                return [literal(node)]
            if isinstance(node, ast.Call):
                return [literal(item) for item in node.args if text(item)] + \
                    [literal(item.value, item.arg) for item in node.keywords if item.arg and text(item.value)]
            if isinstance(node, (ast.Tuple, ast.List)):
                return [literal(item) for item in node.elts if text(item)]
            return []

        if isinstance(value, (ast.List, ast.Tuple, ast.Set)):
            shapes = {shape(item) for item in value.elts}
            if len(shapes) != 1 or None in shapes or "constant" in shapes:
                return None
            rows = [words(item) for item in value.elts]
        elif isinstance(value, ast.Dict):
            if any(key is None or not text(key) for key in value.keys):
                return None
            shapes = {shape(item) for item in value.values}
            if len(shapes) != 1 or None in shapes:
                return None
            rows = [[literal(key)] + words(item) for key, item in zip(value.keys, value.values)]
        else:
            return None
        if len(rows) < 2 or any(not row or None in row for row in rows):
            return None
        return [{"literals": row} for row in rows]

    def compared_word(self, compared, literals, form, node, case=None, branch=None, exclusive=None):
        # One word or several a scope compares a value with, in one case:
        # the if statement whose condition holds the comparison, or a match
        # case (attach_comparisons).
        if self.scope.kind not in ("function", "method", "lambda", "module"):
            return
        if case is None:
            case, branch = getattr(node, "repomap_case", (id(node), None))
        if exclusive is None:
            exclusive = getattr(node, "repomap_exclusive", False)
        origin = self.source_value(compared)
        for item in literals:
            location = source_location(self.module["path"], item)
            if location is None:
                continue
            self.analyzer.compared_words.setdefault(self.scope.ref, []).append({
                "key": ast.dump(compared), "text": bounded_text(ast.unparse(compared)), "origin": origin,
                "word": item.value, "location": location, "form": form, "case": case, "branch": branch,
                "exclusive": exclusive,
            })

    def visit_If(self, node):
        # A comparison in an if statement's condition, alone or joined by
        # and/or, selects the statement's block.
        branch = (node.body[0].lineno, node.body[-1].end_lineno) if node.body else None
        pending = [node.test]
        while pending:
            test = pending.pop()
            if isinstance(test, ast.BoolOp):
                pending.extend(test.values)
            elif isinstance(test, ast.Compare):
                test.repomap_case = (id(node), branch)
                compared = compared_operand(test)
                # The block runs only for the compared words when the whole
                # condition says so (only_when_compared).
                test.repomap_exclusive = compared is not None and only_when_compared(node.test, ast.dump(compared))
        # Each arm starts from the bindings before the statement, and a name
        # an arm binds is joined after it (join_branches).
        self.visit(node.test)
        before = dict(self.current_pattern_bindings())
        for statement in node.body:
            self.visit(statement)
        after_body = dict(self.current_pattern_bindings())
        self.pattern_bindings[id(self.scope)] = dict(before)
        for statement in node.orelse:
            self.visit(statement)
        self.join_branches(node, before, [after_body, dict(self.current_pattern_bindings())])

    def visit_Compare(self, node):
        if len(node.ops) == 1:
            left, right, operator = node.left, node.comparators[0], node.ops[0]
            def text(value):
                return isinstance(value, ast.Constant) and isinstance(value.value, str)
            if isinstance(operator, ast.Eq):
                if text(right) and not isinstance(left, ast.Constant):
                    self.compared_word(left, [right], "equals", node)
                elif text(left) and not isinstance(right, ast.Constant):
                    self.compared_word(right, [left], "equals", node)
            elif isinstance(operator, ast.In) and isinstance(right, (ast.Tuple, ast.List, ast.Set)) and right.elts and \
                    all(text(value) for value in right.elts) and not isinstance(left, ast.Constant):
                self.compared_word(left, right.elts, "equals", node)
            # `command in NO_CONFIG`: the variable is read as the words a
            # value is tested against (record_read).
            if isinstance(operator, (ast.In, ast.NotIn)) and isinstance(right, (ast.Name, ast.Attribute)):
                right.repomap_membership = (ast.dump(left), getattr(node, "repomap_case", (id(node), None))[0])
        self.generic_visit(node)

    def visit_Match(self, node):
        # Each case matching the subject with string values: the words of
        # one case (a | b), and its body as the lines it selects.
        for case in node.cases:
            values = []
            pending = [case.pattern]
            while pending:
                pattern = pending.pop(0)
                if isinstance(pattern, ast.MatchOr):
                    pending[0:0] = pattern.patterns
                elif isinstance(pattern, ast.MatchValue) and isinstance(pattern.value, ast.Constant) and isinstance(pattern.value.value, str):
                    values.append(pattern.value)
                else:
                    values = []
                    break
            if values:
                branch = (case.body[0].lineno, case.body[-1].end_lineno) if case.body else None
                # A match case runs only for its values; there is no
                # falling through.
                self.compared_word(node.subject, values, "case", case, id(case), branch, True)
        self.generic_visit(node)

    def visit_AnnAssign(self, node):
        self._attribute_write(node.target)
        self._target_reads(node.target)
        if node.value is not None:
            self.visit(node.value)
            origin, initializer, source_origin = self.assignment_origin(node.value), self.initializer_value_candidate(node.value), self.source_value(node.value)
            previous, self.binding_statement = self.binding_statement, node
            try:
                self.bind_pattern_target(node.target, origin, initializer, source_origin)
            finally:
                self.binding_statement = previous
            self.bind_field_type(node.target, node.value)
        else:
            self.bind_pattern_target(node.target, {"observed": 0})
        if isinstance(node.target, ast.Name):
            self.current_pattern_bindings()[node.target.id]["iterable_annotation"] = (node.annotation, self.scope)

    def visit_With(self, node):
        # `with engine.begin() as connection` binds the name to what
        # entering the context manager gives, which need not be the manager
        # itself: an "entered" value of the manager's value. A tuple or
        # attribute target is no such name, and each name in it is unknown.
        for item in node.items:
            self.visit(item.context_expr)
            if item.optional_vars is None:
                continue
            self.visit(item.optional_vars)
            if isinstance(item.optional_vars, ast.Name):
                entered = {"kind": "entered", "text": item.optional_vars.id,
                           "anchor": source_location(self.module["path"], item.optional_vars),
                           "parts": [self.source_value(item.context_expr)]}
                previous, self.binding_statement = self.binding_statement, node
                try:
                    self.bind_pattern_target(item.optional_vars, {"observed": 0}, None, entered)
                finally:
                    self.binding_statement = previous
            else:
                for target in ast.walk(item.optional_vars):
                    if isinstance(target, ast.Name):
                        self.bind_nonvalue_name(target.id)
        for statement in node.body:
            self.visit(statement)

    visit_AsyncWith = visit_With

    def visit_ExceptHandler(self, node):
        # `except E as name` rebinds the name to the exception caught.
        if node.type is not None:
            self.visit(node.type)
        if node.name:
            self.bind_nonvalue_name(node.name)
        for statement in node.body:
            self.visit(statement)

    def visit_MatchAs(self, node):
        if node.pattern is not None:
            self.visit(node.pattern)
        if node.name:
            self.bind_nonvalue_name(node.name)

    def visit_MatchStar(self, node):
        if node.name:
            self.bind_nonvalue_name(node.name)

    def visit_MatchMapping(self, node):
        for key in node.keys:
            self.visit(key)
        for pattern in node.patterns:
            self.visit(pattern)
        if node.rest:
            self.bind_nonvalue_name(node.rest)

    def attribute_uses(self):
        # Every attribute the program's code outside tests writes, by its
        # name: the attribute node, its parent and grandparent, the class
        # definition it is written in and the def it runs in, each module
        # walked once.
        index = self.analyzer.attribute_index
        if index is not None:
            return index
        index = {}
        for name in sorted(self.analyzer.modules):
            module = self.analyzer.modules[name]
            if module["path"] in self.analyzer.test_paths:
                continue
            stack = [(module["tree"], None, None, None, None)]
            while stack:
                node, parent, grandparent, klass, function = stack.pop()
                if isinstance(node, ast.Attribute):
                    index.setdefault(node.attr, []).append((node, parent, grandparent, klass, function, module))
                inner_class = node if isinstance(node, ast.ClassDef) else klass
                inner_function = node if isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef, ast.Lambda)) else function
                for child in ast.iter_child_nodes(node):
                    stack.append((child, node, parent, inner_class, inner_function))
        self.analyzer.attribute_index = index
        return index

    def registered_classes(self, expression):
        # `for mod in self.registered_modules` (freqtrade's RPCManager): a
        # list field a class stores once as an empty list and only ever
        # appends constructions of repository classes to holds objects of
        # those classes, a closed set. Every use of the field in the
        # program must be the class's own `self.<field>`: the one store, an
        # append of `C(...)` or of a local bound once to one, pop, remove,
        # clear, iteration, a truth test, len and an element read. Any other
        # use (another receiver's `.<field>`, an alias, extend, insert, +=,
        # a slice or element store, handing or returning the list, a class
        # deriving from it using it) may put anything in it: none.
        if not (isinstance(expression, ast.Attribute) and isinstance(expression.value, ast.Name)
                and expression.value.id == "self" and self.scope.class_ref):
            return []
        class_ref, name = self.scope.class_ref, expression.attr
        key = (class_ref, name)
        cache = self.analyzer.registered_elements
        if key in cache:
            return cache[key]
        cache[key] = []
        stores = self.field_stores_seen(class_ref, name)
        if len(stores) != 1 or not isinstance(stores[0][0], ast.List) or stores[0][0].elts or \
                self.class_attribute_stores(class_ref, name) != [(None, stores[0][1])]:
            return []
        # self in a class unrelated to this one is that class's object; in
        # a base or a class deriving from it, it may be this class's.
        related = set(self.analyzer.base_chain(class_ref)) | set(self.analyzer.derived_classes(class_ref))
        classes = []
        for node, parent, grandparent, klass, function, module in self.attribute_uses().get(name, []):
            owner = self.analyzer.node_refs.get(id(klass)) if klass is not None else None
            if isinstance(node.value, ast.Name) and node.value.id == "self" and owner is not None and owner not in related:
                continue
            if owner != class_ref or not isinstance(node.value, ast.Name) or node.value.id != "self" or function is None:
                return []
            if isinstance(parent, (ast.Assign, ast.AnnAssign)) and node in (parent.targets if isinstance(parent, ast.Assign) else [parent.target]):
                continue
            if isinstance(parent, (ast.For, ast.comprehension)) and parent.iter is node or \
                    isinstance(parent, (ast.While, ast.If, ast.IfExp, ast.Assert)) and parent.test is node or \
                    isinstance(parent, (ast.UnaryOp, ast.BoolOp)) or \
                    isinstance(parent, ast.Subscript) and parent.value is node and isinstance(parent.ctx, ast.Load) or \
                    isinstance(parent, ast.Call) and isinstance(parent.func, ast.Name) and parent.func.id == "len" and parent.args == [node]:
                continue
            if isinstance(parent, ast.Attribute) and parent.value is node and isinstance(grandparent, ast.Call) and grandparent.func is parent:
                if parent.attr in ("pop", "remove", "clear"):
                    continue
                if parent.attr == "append" and len(grandparent.args) == 1 and not grandparent.keywords:
                    constructed = self.constructed_class(grandparent.args[0], function, module)
                    if constructed:
                        if constructed not in classes:
                            classes.append(constructed)
                        continue
            return []
        classes.sort(key=lambda ref: location_key(self.object(ref).get("location")))
        cache[key] = classes
        return classes

    def constructed_class(self, value, function, module):
        # The repository class a value constructs where it is written: a
        # call of the class, or a local name bound once, in the def it is
        # read in, to such a call. "" otherwise.
        if isinstance(value, ast.Name):
            bound = [node for node in ast.walk(function) if value.id in bound_names(node)]
            if len(bound) != 1 or not isinstance(bound[0], ast.Assign) or len(bound[0].targets) != 1:
                return ""
            value = bound[0].value
        if not isinstance(value, ast.Call):
            return ""
        scope = self.analyzer.node_scopes.get(id(function))
        if scope is None:
            return ""
        previous = self.scope, self.module
        self.scope, self.module = scope, module
        try:
            authority, ref = self.resolve(value.func)
        finally:
            self.scope, self.module = previous
        candidate = self.object(ref) if ref else None
        return ref if authority == "local" and candidate is not None and candidate["kind"] == "type" else ""

    def registered_members(self, callee):
        # The methods a call on a loop's element over a registered list
        # reaches: each class's member of that name, all of them repository
        # methods; none when one class has no such method.
        if not (isinstance(callee, ast.Attribute) and isinstance(callee.value, ast.Name)):
            return []
        binding = self.pattern_binding(callee.value.id)
        if not binding or not binding.get("registered"):
            return []
        members = []
        for class_ref in binding["registered"]:
            member = self.class_member(class_ref, callee.attr, ("method", "function"))
            if not member:
                return []
            if member not in members:
                members.append(member)
        return members

    def statement_block(self, statement):
        # The statement list a statement stands in, as (the node owning it,
        # the field), gathered once per module.
        path = self.module["path"]
        blocks = self.analyzer.statement_blocks.get(path)
        if blocks is None:
            blocks = {}
            for owner in ast.walk(self.module["tree"]):
                for field, value in ast.iter_fields(owner):
                    if isinstance(value, list):
                        for child in value:
                            if isinstance(child, ast.stmt):
                                blocks[id(child)] = (owner, field)
            self.analyzer.statement_blocks[path] = blocks
        return blocks.get(id(statement))

    def dominated_by(self, statement, block):
        # Whether a statement runs only after the statement list block has
        # reached it: it stands in that list, or in an arm of an if
        # statement standing there (an arm's value is joined after the if,
        # join_branches), however deep. A loop, try, with or match between
        # them runs its body again, not at all or only in part: no.
        current = statement
        while True:
            standing = self.statement_block(current)
            if standing is None:
                return False
            if standing[0] is block[0] and standing[1] == block[1]:
                return True
            owner, field = standing
            if not isinstance(owner, ast.If) or field not in ("body", "orelse"):
                return False
            current = owner

    def join_branches(self, statement, before, arms):
        # After an if statement a name holds what one of its paths left in
        # it: each arm's last value, and the value before the statement for
        # a path that did not bind it (no else, an arm leaving it alone). One
        # value is that value; several are their alternatives, none chosen,
        # as Go's SSA joins them; an unknown one leaves the name unknown. A
        # path on which the name is unbound cannot read it. Which class a
        # call on it reaches stays unknown whenever two bindings meet.
        current = self.current_pattern_bindings()
        block = self.statement_block(statement)
        names = set()
        for arm in arms:
            for name, binding in arm.items():
                if before.get(name) is not binding:
                    names.add(name)
        for name in sorted(names):
            bindings = [arm.get(name) for arm in arms]
            present = [binding for binding in bindings if binding is not None]
            distinct = []
            for binding in present:
                if not any(binding is known for known in distinct):
                    distinct.append(binding)
            values, known = [], True
            for binding in distinct:
                origin = binding.get("source_origin")
                if origin is None:
                    known = False
                    break
                if origin not in values:
                    values.append(origin)
            last = next(binding for binding in reversed(bindings) if binding is not None and binding is not before.get(name))
            joined = dict(last)
            if not known:
                joined["source_origin"] = None
            elif len(values) == 1:
                joined["source_origin"] = values[0]
            else:
                joined["source_origin"] = {"kind": "alternatives", "parts": values}
            if len(distinct) > 1:
                joined["value_invalidated"] = True
                joined["origin_invalidated"] = True
                joined["rebound"] = True
            joined["block"] = block
            current[name] = joined

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
        element = self.iterated(node.iter)
        self._target_reads(node.target)
        # The classes a list field is closed over win over its annotation:
        # they are the objects the list can hold (registered_classes).
        registered = self.registered_classes(node.iter) if isinstance(node, ast.For) else []
        type_ref = self.iterated_class(node.iter) if isinstance(node, ast.For) and not registered else ""
        self.bind_pattern_target(node.target, {"observed": 0})
        if type_ref and isinstance(node.target, ast.Name):
            self.current_pattern_bindings()[node.target.id].update({
                "origin_refs": [type_ref], "origin_resolution": "alternatives", "origins_observed": 1,
                "annotation_origin": True, "iteration_origin": True,
            })
        if registered and isinstance(node.target, ast.Name):
            self.current_pattern_bindings()[node.target.id].update({"registered": list(registered)})
        if element is not None and isinstance(node.target, ast.Name) and not self.handled.get(id(self.scope)):
            self.current_pattern_bindings()[node.target.id]["element_of"] = {**element, "reached": pass_subscripts(node.body)}
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

    def visit_Try(self, node):
        # A loop in the body of a try statement with handlers need not look
        # every element up: a handler may catch the failed lookup.
        key = id(self.scope)
        if node.handlers:
            self.handled[key] = self.handled.get(key, 0) + 1
        for statement in node.body:
            self.visit(statement)
        if node.handlers:
            self.handled[key] -= 1
        for part in list(node.handlers) + list(node.orelse) + list(node.finalbody):
            self.visit(part)

    visit_TryStar = visit_Try

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
            if binding and (binding.get("annotation_origin") or not origins_invalidated(binding)):
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


def attach_guards(tree, path):
    # A call's guard (ProgramIndex Guard): the strongest construct of its
    # function it runs under, at that construct. "branch": an if or else
    # arm, a conditional expression's arm, a match case, the operands of
    # and/or after the first, a try's else. "error": what a raise raises or
    # chains, an except body, an assert's message, an arm ending in a raise.
    # A test, a subject or a try body is in no arm; a function, lambda or
    # class body starts afresh.
    def strength(guard):
        return 0 if guard is None else (1 if guard["kind"] == "branch" else 2)

    def stronger(guard, kind, node):
        inner = {"kind": kind, "location": source_location(path, node)}
        return guard if strength(guard) > strength(inner) else inner

    def returns(nodes):
        # A return before the raise is a way out that does not fail.
        for node in nodes:
            if isinstance(node, ast.Return):
                return True
            if isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef, ast.Lambda, ast.ClassDef)):
                continue
            if returns(list(ast.iter_child_nodes(node))):
                return True
        return False

    def ends_in_raise(body):
        return bool(body) and isinstance(body[-1], ast.Raise) and not returns(body)

    def arm(guard, body, node):
        return stronger(guard, "error" if ends_in_raise(body) else "branch", node)

    def walk(node, guard):
        if guard is not None:
            node.repomap_guard = guard
        if isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef, ast.Lambda, ast.ClassDef)):
            body = node.body if isinstance(node.body, list) else [node.body]
            for child in ast.iter_child_nodes(node):
                walk(child, None if child in body else guard)
            return
        if isinstance(node, ast.If):
            walk(node.test, guard)
            inner = arm(guard, node.body, node)
            for child in node.body:
                walk(child, inner)
            if node.orelse:
                inner = arm(guard, node.orelse, node.orelse[0])
                for child in node.orelse:
                    walk(child, inner)
            return
        if isinstance(node, ast.IfExp):
            walk(node.test, guard)
            walk(node.body, stronger(guard, "branch", node))
            walk(node.orelse, stronger(guard, "branch", node))
            return
        if isinstance(node, ast.BoolOp):
            walk(node.values[0], guard)
            for value in node.values[1:]:
                walk(value, stronger(guard, "branch", node))
            return
        if hasattr(ast, "Match") and isinstance(node, ast.Match):
            walk(node.subject, guard)
            for case in node.cases:
                inner = arm(guard, case.body, case.pattern)
                if case.guard is not None:
                    walk(case.guard, guard)
                for child in case.body:
                    walk(child, inner)
            return
        if isinstance(node, (ast.Try, getattr(ast, "TryStar", ast.Try))):
            for child in node.body + node.finalbody:
                walk(child, guard)
            for handler in node.handlers:
                inner = stronger(guard, "error", handler)
                if handler.type is not None:
                    walk(handler.type, guard)
                for child in handler.body:
                    walk(child, inner)
            if node.orelse:
                inner = stronger(guard, "branch", node.orelse[0])
                for child in node.orelse:
                    walk(child, inner)
            return
        if isinstance(node, ast.Raise):
            inner = stronger(guard, "error", node)
            for child in ast.iter_child_nodes(node):
                walk(child, inner)
            return
        if isinstance(node, ast.Assert):
            walk(node.test, guard)
            if node.msg is not None:
                walk(node.msg, stronger(guard, "error", node))
            return
        for child in ast.iter_child_nodes(node):
            walk(child, guard)
    walk(tree, None)


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
            attach_guards(parsed[path], path)
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
