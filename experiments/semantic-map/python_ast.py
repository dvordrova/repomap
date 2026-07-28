#!/usr/bin/env python3
"""Extract bounded, syntax-only facts from Python source slices."""

from __future__ import annotations

import argparse
import ast
import json
import os
from pathlib import Path
import posixpath
import tempfile
import textwrap
from typing import Any, Iterable


MAX_FACTS = 48
MAX_PACKET_BYTES = 32 << 10
MAX_SOURCE_BYTES = 24 << 10
MAX_SOURCE_SLICES = 12
MAX_SLICE_LINES = 201
MAX_PATH_BYTES = 240
MAX_SYNTAX_BYTES = 240
WRAPPER_NAME = "__repomap_source_slice__"

FactKey = tuple[str, int, int, str, str, str, int, int]


def _output_key(fact: FactKey) -> tuple[str, int, int, str, str, str]:
    return fact[:6]


def _syntax(node: ast.AST | None) -> str:
    if node is None:
        return "()"
    return ast.unparse(node)


def _call_arguments(node: ast.Call) -> str:
    parts = [_syntax(argument) for argument in node.args]
    for keyword in node.keywords:
        if keyword.arg is None:
            parts.append(f"**{_syntax(keyword.value)}")
        else:
            parts.append(f"{keyword.arg}={_syntax(keyword.value)}")
    return ", ".join(parts) if parts else "()"


def _syntax_scalar(value: str) -> bool:
    return (
        bool(value)
        and len(value.encode("utf-8")) <= MAX_SYNTAX_BYTES
        and "\x00" not in value
        and "\r" not in value
        and "\n" not in value
        and value.strip() == value
    )


def _parse_source(text: str) -> tuple[ast.Module, int, int] | None:
    """Return a recovered tree, trimmed leading lines, and wrapper line count."""
    lines = text.splitlines()
    if not lines:
        return None

    windows = [(0, len(lines))]
    windows.extend((0, end) for end in range(len(lines) - 1, 0, -1))
    windows.extend((start, len(lines)) for start in range(1, len(lines)))

    for leading, end in windows:
        source = "\n".join(lines[leading:end])
        dedented = textwrap.dedent(source)
        candidates = [(source, 0)]
        if dedented != source:
            candidates.append((dedented, 0))
        candidates.append(
            (
                f"def {WRAPPER_NAME}():\n{textwrap.indent(dedented, '    ')}\n",
                1,
            )
        )

        for candidate, wrapper_lines in candidates:
            try:
                return ast.parse(candidate), leading, wrapper_lines
            except SyntaxError:
                continue

    return None


class _FactVisitor(ast.NodeVisitor):
    def __init__(
        self,
        path: str,
        slice_start: int,
        slice_end: int,
        leading_lines: int,
        wrapper_lines: int,
    ) -> None:
        self.path = path
        self.slice_start = slice_start
        self.slice_end = slice_end
        self.leading_lines = leading_lines
        self.wrapper_lines = wrapper_lines
        self.depth = 0
        self.region = 0
        self.next_region = 0
        self.facts: set[FactKey] = set()

    def _record(
        self, node: ast.AST, kind: str, subject: str, object_: str
    ) -> None:
        if not _syntax_scalar(subject) or not _syntax_scalar(object_):
            return
        line = getattr(node, "lineno", None)
        if line is None:
            return
        end_line = getattr(node, "end_lineno", None) or line
        local_start = line - self.wrapper_lines
        local_end = end_line - self.wrapper_lines
        if local_start < 1:
            return

        start = self.slice_start + self.leading_lines + local_start - 1
        end = self.slice_start + self.leading_lines + local_end - 1
        if start > self.slice_end or end < self.slice_start:
            return
        end = min(end, self.slice_end)
        self.facts.add(
            (
                self.path,
                start,
                end,
                kind,
                subject,
                object_,
                self.depth,
                self.region,
            )
        )

    def _visit_nested(self, statements: Iterable[ast.AST]) -> None:
        previous = self.depth
        self.depth += 1
        for statement in statements:
            self.visit(statement)
        self.depth = previous

    def visit_ClassDef(self, node: ast.ClassDef) -> None:
        for decorator in node.decorator_list:
            self.visit(decorator)
        for base in node.bases:
            self.visit(base)
        for keyword in node.keywords:
            self.visit(keyword)
        previous = self.depth
        self.depth = 0
        for statement in node.body:
            self.visit(statement)
        self.depth = previous

    def visit_FunctionDef(self, node: ast.FunctionDef) -> None:
        for decorator in node.decorator_list:
            self.visit(decorator)
        for default in [*node.args.defaults, *node.args.kw_defaults]:
            if default is not None:
                self.visit(default)
        previous_depth = self.depth
        previous_region = self.region
        self.next_region += 1
        self.region = self.next_region
        self.depth = 0
        for statement in node.body:
            self.visit(statement)
        self.depth = previous_depth
        self.region = previous_region

    def visit_AsyncFunctionDef(self, node: ast.AsyncFunctionDef) -> None:
        self.visit_FunctionDef(node)

    def visit_Lambda(self, node: ast.Lambda) -> None:
        previous_depth = self.depth
        previous_region = self.region
        self.next_region += 1
        self.region = self.next_region
        self.depth = 0
        self.visit(node.body)
        self.depth = previous_depth
        self.region = previous_region

    def visit_Expr(self, node: ast.Expr) -> None:
        if isinstance(node.value, ast.Call):
            self._record(
                node.value,
                "call",
                _syntax(node.value.func),
                _call_arguments(node.value),
            )
        self.generic_visit(node)

    def visit_Assign(self, node: ast.Assign) -> None:
        targets = ", ".join(_syntax(target) for target in node.targets)
        self._record(node, "assign", targets, _syntax(node.value))
        self.generic_visit(node)

    def visit_AnnAssign(self, node: ast.AnnAssign) -> None:
        if node.value is not None:
            self._record(node, "assign", _syntax(node.target), _syntax(node.value))
        self.generic_visit(node)

    def visit_AugAssign(self, node: ast.AugAssign) -> None:
        self._record(node, "assign", _syntax(node.target), _syntax(node))
        self.generic_visit(node)

    def visit_NamedExpr(self, node: ast.NamedExpr) -> None:
        self._record(node, "assign", _syntax(node.target), _syntax(node.value))
        self.generic_visit(node)

    def visit_If(self, node: ast.If) -> None:
        self._record(node, "branch", "if", _syntax(node.test))
        self.visit(node.test)
        self._visit_nested([*node.body, *node.orelse])

    def visit_IfExp(self, node: ast.IfExp) -> None:
        self._record(node, "branch", "if", _syntax(node.test))
        self.visit(node.test)
        self._visit_nested([node.body, node.orelse])

    def visit_Match(self, node: ast.Match) -> None:
        self._record(node, "branch", "match", _syntax(node.subject))
        self.visit(node.subject)
        nested = []
        for case in node.cases:
            if case.guard is not None:
                nested.append(case.guard)
            nested.extend(case.body)
        self._visit_nested(nested)

    def visit_For(self, node: ast.For) -> None:
        self._record(node, "loop", _syntax(node.target), _syntax(node.iter))
        self.visit(node.target)
        self.visit(node.iter)
        self._visit_nested([*node.body, *node.orelse])

    def visit_AsyncFor(self, node: ast.AsyncFor) -> None:
        self.visit_For(node)

    def visit_While(self, node: ast.While) -> None:
        self._record(node, "loop", "while", _syntax(node.test))
        self.visit(node.test)
        self._visit_nested([*node.body, *node.orelse])

    def visit_Try(self, node: ast.Try) -> None:
        nested: list[ast.AST] = [*node.body, *node.orelse, *node.finalbody]
        nested.extend(node.handlers)
        self._visit_nested(nested)

    def visit_TryStar(self, node: ast.TryStar) -> None:
        self.visit_Try(node)

    def visit_Return(self, node: ast.Return) -> None:
        self._record(node, "return", "return", _syntax(node.value))
        self.generic_visit(node)


def _slice_facts(source_slice: Any) -> list[FactKey]:
    path, start_line, end_line, text = _validate_source_slice(source_slice)
    if posixpath.splitext(path)[1] != ".py":
        return []

    parsed = _parse_source(text)
    if parsed is None:
        return []
    tree, leading_lines, wrapper_lines = parsed
    visitor = _FactVisitor(
        path, start_line, end_line, leading_lines, wrapper_lines
    )
    visitor.visit(tree)
    shallowest: dict[
        tuple[str, int, int, str, str, str], FactKey
    ] = {}
    for fact in visitor.facts:
        key = _output_key(fact)
        previous = shallowest.get(key)
        if previous is None or (fact[6], fact[7]) < (
            previous[6],
            previous[7],
        ):
            shallowest[key] = fact
    return sorted(shallowest.values(), key=_output_key)


def _validate_source_slice(
    source_slice: Any,
) -> tuple[str, int, int, str]:
    if not isinstance(source_slice, dict):
        raise ValueError("source_slices entries must be objects")

    path = source_slice.get("path")
    start_line = source_slice.get("start_line")
    end_line = source_slice.get("end_line")
    text = source_slice.get("text")
    if (
        not isinstance(path, str)
        or not path
        or "\x00" in path
        or "\\" in path
        or ":" in path
        or path.startswith("/")
        or posixpath.normpath(path) != path
        or path == "."
        or path == ".."
        or path.startswith("../")
    ):
        raise ValueError(
            "source slice path must be bounded canonical repository-relative syntax"
        )
    try:
        path_bytes = len(path.encode("utf-8"))
    except UnicodeEncodeError as error:
        raise ValueError("source slice path must be valid UTF-8") from error
    if path_bytes > MAX_PATH_BYTES:
        raise ValueError("source slice path exceeds 240 bytes")
    if type(start_line) is not int or start_line < 1:
        raise ValueError("source slice start_line must be a positive integer")
    if (
        type(end_line) is not int
        or end_line < start_line
        or end_line - start_line + 1 > MAX_SLICE_LINES
    ):
        raise ValueError("source slice line range is invalid or too large")
    if not isinstance(text, str) or not text or "\x00" in text:
        raise ValueError("source slice text must be non-empty and bounded")
    try:
        text_bytes = len(text.encode("utf-8"))
    except UnicodeEncodeError as error:
        raise ValueError("source slice text must be valid UTF-8") from error
    if text_bytes > MAX_SOURCE_BYTES:
        raise ValueError("source slice text exceeds 24576 bytes")
    comparable_text = text[:-1] if text.endswith("\n") else text
    if len(comparable_text.split("\n")) != end_line - start_line + 1:
        raise ValueError("source slice text line count does not match its range")
    return path, start_line, end_line, text


def extract_facts(source_slices: Iterable[Any]) -> list[dict[str, Any]]:
    groups = []
    for source_slice in source_slices:
        path, start_line, _, _ = _validate_source_slice(source_slice)
        by_region: dict[int, list[FactKey]] = {}
        for fact in _slice_facts(source_slice):
            by_region.setdefault(fact[7], []).append(fact)
        for region, facts in by_region.items():
            groups.append((path, start_line, region, facts))
    groups.sort(key=lambda group: (group[0], group[1], group[2]))
    per_slice = [group[3] for group in groups]
    quotas = _coverage_quotas([len(facts) for facts in per_slice], MAX_FACTS)
    selected = {
        _output_key(fact): fact
        for facts, quota in zip(per_slice, quotas)
        for fact in _select_group(facts, quota)
    }

    output = []
    for position, fact in enumerate(
        sorted(selected.values(), key=_output_key), start=1
    ):
        path, start_line, end_line, kind, subject, object_, _, _ = fact
        output.append(
            {
                "id": f"r{position}",
                "path": path,
                "start_line": start_line,
                "end_line": end_line,
                "kind": kind,
                "subject": subject,
                "object": object_,
            }
        )
    return output


def _coverage_quotas(lengths: list[int], limit: int) -> list[int]:
    quotas = [0] * len(lengths)
    selected = 0
    while selected < limit:
        best: int | None = None
        for index, length in enumerate(lengths):
            if quotas[index] >= length:
                continue
            if (
                best is None
                or quotas[index] * lengths[best] < quotas[best] * length
            ):
                best = index
        if best is None:
            break
        quotas[best] += 1
        selected += 1
    return quotas


def _select_group(facts: list[FactKey], count: int) -> list[FactKey]:
    if count >= len(facts):
        return list(facts)
    selected: set[int] = set()

    if (
        count >= 2
        and facts[-1][3] == "return"
        and facts[-1][5] not in {"None", "()"}
    ):
        selected.add(len(facts) - 1)

    controls = []
    actions = []
    returns = []
    for index, fact in enumerate(facts):
        kind = fact[3]
        if kind in {"assign", "call", "defer"}:
            actions.append(index)
        elif kind in {"branch", "loop"}:
            controls.append(index)
        elif kind == "return":
            returns.append(index)
    if count >= 3 and controls:
        selected.add(controls[len(controls) // 2])
    if count >= 4 and len(actions) <= 2 and len(returns) > 1:
        selected.add(returns[0])

    for index in _order_actions(facts, actions):
        if index in selected:
            continue
        selected.add(index)
        if len(selected) == count:
            break

    for index in _spread_order(len(facts)):
        if len(selected) >= count:
            break
        selected.add(index)
    return [facts[index] for index in sorted(selected)]


def _order_actions(facts: list[FactKey], indexes: list[int]) -> list[int]:
    if len(indexes) == 2:
        left = facts[indexes[0]]
        right = facts[indexes[1]]
        if left[3] == "assign" and right[3] != "assign":
            return indexes
        if right[3] == "assign" and left[3] != "assign":
            return [indexes[1], indexes[0]]
        return [indexes[1], indexes[0]]
    return [indexes[position] for position in _spread_order(len(indexes))]


def _spread_order(length: int) -> list[int]:
    if length == 0:
        return []
    order = []
    seen = set()
    for index in [
        length // 2,
        0,
        length - 1,
        length // 4,
        3 * (length - 1) // 4,
        length // 3,
        2 * (length - 1) // 3,
        *range(length),
    ]:
        if index in seen:
            continue
        seen.add(index)
        order.append(index)
    return order


def _read_source_slices(path: Path) -> list[Any]:
    packet_bytes = path.read_bytes()
    if not packet_bytes or len(packet_bytes) > MAX_PACKET_BYTES:
        raise ValueError("input packet is empty or exceeds 32768 bytes")
    packet = json.loads(packet_bytes)
    if not isinstance(packet, dict):
        raise ValueError("input packet must be an object")
    source_slices = packet.get("source_slices")
    if (
        not isinstance(source_slices, list)
        or not source_slices
        or len(source_slices) > MAX_SOURCE_SLICES
    ):
        raise ValueError("input packet source_slices count must be 1..12")

    total_text_bytes = 0
    for source_slice in source_slices:
        _, _, _, text = _validate_source_slice(source_slice)
        total_text_bytes += len(text.encode("utf-8"))
        if total_text_bytes > MAX_SOURCE_BYTES:
            raise ValueError("source slice text exceeds 24576 bytes")
    return source_slices


def _atomic_write(path: Path, facts: list[dict[str, Any]]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    descriptor, temporary_name = tempfile.mkstemp(
        dir=path.parent, prefix=f".{path.name}.", suffix=".tmp"
    )
    try:
        with os.fdopen(descriptor, "w", encoding="utf-8") as output:
            json.dump(facts, output, ensure_ascii=False, indent=2)
            output.write("\n")
            output.flush()
            os.fsync(output.fileno())
        os.replace(temporary_name, path)
    except BaseException:
        try:
            os.unlink(temporary_name)
        except FileNotFoundError:
            pass
        raise


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("input_packet", type=Path)
    parser.add_argument("output_facts", type=Path)
    args = parser.parse_args()

    facts = extract_facts(_read_source_slices(args.input_packet))
    _atomic_write(args.output_facts, facts)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
