#!/usr/bin/env python3
"""Validate and score the Task Lens v0.1 known-development rerun.

This evaluator is deliberately standalone.  It reads sealed artifacts only,
never imports the v0 holdout harness, and never invokes a command in the target
repository.  The six former holdout episodes are scored as known development;
the configuration episode is a cheap-exit calibration case only.
"""

from __future__ import annotations

import argparse
import hashlib
import html
import json
import os
import re
import stat
import sys
import tempfile
from dataclasses import dataclass, field
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Iterable, Mapping, Sequence

sys.dont_write_bytecode = True


class EvaluationError(RuntimeError):
    """Raised for an invalid evaluation input or artifact set."""


@dataclass(frozen=True)
class EpisodeSpec:
    episode_id: str
    base_revision: str
    evaluation_scope: str = "primary_regression"
    cheap_exit_target: bool = False


PRIMARY_EPISODES = (
    EpisodeSpec("openapi_example_tag_parsing", "84fadc7a86fa097f240d5dafda3d86f7a784b3ec"),
    EpisodeSpec("openapi_required_nullable_semantics", "c8a6e9dcd907314a613b2f8ff325ba3f01372774"),
    EpisodeSpec(
        "accept_header_wrong_status",
        "4bc28418acc1e8523276d0da2d6581fd98393106",
        cheap_exit_target=True,
    ),
    EpisodeSpec(
        "nil_body_validation_panic",
        "29023c565e324759e5a50e90583f4afdcdca11e4",
        cheap_exit_target=True,
    ),
    EpisodeSpec("httperror_pointer_value", "fbf3dcfcaa69bdee8a17858a8f82656a4a8485d0"),
    EpisodeSpec("multi_module_release_script", "62113a7cff6210c0db16ed51e003d26043398cf2"),
)

CALIBRATION_EPISODES = (
    EpisodeSpec(
        "openapi_disable_messages_config",
        "5f914dc7beb960bef84dede11960744a0a96a3c1",
        evaluation_scope="cheap_exit_only",
        cheap_exit_target=True,
    ),
)

EPISODES = PRIMARY_EPISODES + CALIBRATION_EPISODES
EPISODE_BY_ID = {episode.episode_id: episode for episode in EPISODES}
PRIMARY_IDS = tuple(episode.episode_id for episode in PRIMARY_EPISODES)
CHEAP_EXIT_IDS = tuple(episode.episode_id for episode in EPISODES if episode.cheap_exit_target)

V0_FREEZE_MANIFEST_SHA256 = "afaf100ee7bcc0f21df8a2eb160406f70c94fd8e18fccfc1a1e24ef233f4da29"
V0_BINARY_SHA256 = "05ed471bf4ee77d8320e2de116a627cf76b7360a173c406d4dbe71598232ca01"
V0_PROVIDER_MODEL = "deepseek-v4-flash"
V0_PROVIDER_ENDPOINT = "https://api.deepseek.com/chat/completions"

QUARTET = (
    "task_investigation_bundle.json",
    "task_investigation_attempt.json",
    "task_investigation.json",
    "task_investigation_status.json",
)

SCORE_DIMENSIONS = (
    "subsystem_localization",
    "decisive_anchor_recall",
    "key_role_coverage",
    "must_read_file_recall",
    "causal_decisive_relation",
    "verification_usefulness",
    "sufficient_flag_correctness",
    "source_scope_completeness",
    "irrelevant_anchor_control",
    "task_interpretation",
    "uncertainty_calibration",
    "cost_appropriateness",
)

REPORT_COMPARISON_DIMENSIONS = (
    ("subsystem", "subsystem_localization", "subsystem_localization"),
    ("files", "must_read_file_recall", "must_read_file_recall"),
    ("symbols/key roles", "key_symbol_recall", "decisive_anchor_recall"),
    ("decisive relation", "causal_evidence_join_quality", "causal_decisive_relation"),
    ("verification", "verification_usefulness", "verification_usefulness"),
    ("uncertainty", "uncertainty_calibration", "uncertainty_calibration"),
    ("cost", "cost_appropriateness", "cost_appropriateness"),
)

REQUIRED_RENDERED_REPORTS = {
    "BASELINE_COMPARISON.md",
    "CHEAP_EXIT_EVALUATION.md",
    "DEV_REGRESSION.md",
    "PRODUCT_FINDINGS.md",
    "RETRIEVAL_FAILURES.md",
    "SOURCE_SCOPE_AUDIT.md",
    "SUPERVISOR_REPORT.md",
    "WALKTHROUGH.md",
    "review/index.html",
}

SUPERVISOR_OPENING_LABELS = (
    "outcome:",
    "technical result:",
    "product-development result:",
    "investment result:",
    "episodes evaluated:",
    "decisive-anchor target result:",
    "verification target result:",
    "zero-call target result:",
    "single main blocker:",
    "exactly one recommended next step:",
)

SCOPE_KINDS = {
    "complete_enclosing_symbol",
    "complete_document_section",
    "complete_file",
    "matched_fragments",
    "partial_window",
}

RELATION_KINDS = {
    "direct_call",
    "field_copy",
    "field_read",
    "field_write",
    "error_created",
    "error_mapped",
    "error_exposed",
    "value_transformed",
    "type_name_generated",
    "config_applied",
    "script_invokes",
    "test_exercises",
    "fixture_records",
    "documented_uses",
    "shared_state_alias",
    "scope_unknown",
}

SUPPORT_CLASSES = {
    "locally_observed",
    "document_supported",
    "model_hypothesis",
    "unresolved",
}

VERIFICATION_AUTHORITIES = {
    "exact_existing_test",
    "exact_generated_fixture",
    "exact_example",
    "documented_command",
    "proposed_test_location",
    "missing_evidence",
}

GROUNDED_AUTHORITIES = {
    "exact_existing_test",
    "exact_generated_fixture",
    "exact_example",
    "documented_command",
}

GOLD_LOSS_STAGES = {
    "present_before_ranking",
    "dropped_during_ranking",
    "never_generated",
    "clipped_during_source_retention",
}

CHEAP_EXIT_GATES = {
    "unambiguous_area",
    "all_key_roles",
    "decisive_locally_observed_relation",
    "exact_verification_anchor_or_effect",
    "no_unresolved_competing_hypothesis",
}

NEGATIVE_EVIDENCE_BASES = {
    "none",
    "complete_scope",
    "exhaustive_bounded_exact_search",
    "deterministic_manifest",
}


@dataclass
class Audit:
    errors: list[str] = field(default_factory=list)
    warnings: list[str] = field(default_factory=list)

    def error(self, message: str) -> None:
        self.errors.append(message)

    def warn(self, message: str) -> None:
        self.warnings.append(message)


def _reject_duplicate_keys(pairs: Sequence[tuple[str, Any]]) -> dict[str, Any]:
    result: dict[str, Any] = {}
    for key, value in pairs:
        if key in result:
            raise EvaluationError(f"duplicate JSON key: {key}")
        result[key] = value
    return result


def read_json(path: Path) -> dict[str, Any]:
    try:
        value = json.loads(path.read_text(encoding="utf-8"), object_pairs_hook=_reject_duplicate_keys)
    except (OSError, UnicodeError, json.JSONDecodeError) as exc:
        raise EvaluationError(f"cannot read JSON {path}: {exc}") from exc
    if not isinstance(value, dict):
        raise EvaluationError(f"expected JSON object in {path}")
    return value


def sha256_bytes(raw: bytes) -> str:
    return hashlib.sha256(raw).hexdigest()


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    try:
        with path.open("rb") as source:
            for block in iter(lambda: source.read(1024 * 1024), b""):
                digest.update(block)
    except OSError as exc:
        raise EvaluationError(f"cannot hash {path}: {exc}") from exc
    return digest.hexdigest()


def canonical_json_sha256(value: Any) -> str:
    raw = json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=False).encode("utf-8")
    return sha256_bytes(raw)


def go_compact_json_sha256(value: Any) -> str:
    """Match encoding/json Marshal for JSON objects decoded in field order."""
    raw = json.dumps(value, ensure_ascii=False, separators=(",", ":"))
    raw = raw.replace("&", "\\u0026").replace("<", "\\u003c").replace(">", "\\u003e")
    raw = raw.replace("\u2028", "\\u2028").replace("\u2029", "\\u2029")
    return sha256_bytes(raw.encode("utf-8"))


def utc_now() -> str:
    return datetime.now(timezone.utc).replace(microsecond=0).isoformat()


def _json_bytes(value: Any) -> bytes:
    return (json.dumps(value, indent=2, sort_keys=True, ensure_ascii=False) + "\n").encode("utf-8")


def atomic_write(path: Path, raw: bytes) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    descriptor, temporary = tempfile.mkstemp(prefix=f".{path.name}.", dir=path.parent)
    try:
        with os.fdopen(descriptor, "wb") as output:
            output.write(raw)
            output.flush()
            os.fsync(output.fileno())
        os.replace(temporary, path)
    except BaseException:
        try:
            os.unlink(temporary)
        except FileNotFoundError:
            pass
        raise


def write_json(path: Path, value: Any) -> None:
    atomic_write(path, _json_bytes(value))


def write_text(path: Path, value: str) -> None:
    if not value.endswith("\n"):
        value += "\n"
    atomic_write(path, value.encode("utf-8"))


def _safe_relative_path(raw: str, source: Path) -> Path:
    path = Path(raw)
    if path.is_absolute() or not raw or any(part in {"", ".", ".."} for part in path.parts):
        raise EvaluationError(f"unsafe inventory path {raw!r} in {source}")
    return path


def verify_baseline(root: Path) -> dict[str, Any]:
    baseline = root / "baseline"
    inventory_path = root / "BASELINE_INVENTORY.sha256"
    if not baseline.is_dir():
        raise EvaluationError(f"missing baseline directory: {baseline}")
    if not inventory_path.is_file():
        raise EvaluationError(f"missing baseline inventory: {inventory_path}")

    checked = 0
    seen: set[str] = set()
    for line_number, raw_line in enumerate(inventory_path.read_text(encoding="utf-8").splitlines(), 1):
        if not raw_line.strip():
            continue
        match = re.fullmatch(r"([0-9a-f]{64})  (.+)", raw_line)
        if match is None:
            raise EvaluationError(f"invalid inventory line {line_number} in {inventory_path}")
        expected, raw_relative = match.groups()
        relative = _safe_relative_path(raw_relative, inventory_path)
        try:
            baseline_index = relative.parts.index("baseline")
        except ValueError as exc:
            raise EvaluationError(f"inventory entry has no baseline component: {raw_relative}") from exc
        root_relative = Path(*relative.parts[baseline_index:])
        normalized = root_relative.as_posix()
        if normalized in seen:
            raise EvaluationError(f"duplicate inventory path: {normalized}")
        seen.add(normalized)
        target = root / root_relative
        try:
            target.relative_to(baseline)
        except ValueError as exc:
            raise EvaluationError(f"inventory entry is outside baseline: {normalized}") from exc
        if not target.is_file() or target.is_symlink():
            raise EvaluationError(f"inventory entry is not a regular file: {target}")
        actual = sha256_file(target)
        if actual != expected:
            raise EvaluationError(f"baseline hash mismatch for {normalized}: {actual} != {expected}")
        checked += 1

    actual_regular = {
        path.relative_to(root).as_posix()
        for path in baseline.rglob("*")
        if path.is_file() and not path.is_symlink()
    }
    missing_from_inventory = sorted(actual_regular - seen)
    if missing_from_inventory:
        raise EvaluationError(
            "baseline inventory omits regular files: " + ", ".join(missing_from_inventory[:10])
        )

    writable: list[str] = []
    for path in (baseline, *baseline.rglob("*")):
        if path.is_symlink():
            continue
        try:
            mode = stat.S_IMODE(path.stat().st_mode)
        except OSError as exc:
            raise EvaluationError(f"cannot stat baseline path {path}: {exc}") from exc
        if mode & 0o222:
            writable.append(path.relative_to(root).as_posix())
    if writable:
        raise EvaluationError("baseline is not read-only: " + ", ".join(writable[:10]))

    freeze_manifest = baseline / "FREEZE_MANIFEST.json"
    frozen_binary = baseline / "freeze" / "repomap"
    if sha256_file(freeze_manifest) != V0_FREEZE_MANIFEST_SHA256:
        raise EvaluationError("v0 FREEZE_MANIFEST.json is not the preserved frozen manifest")
    if sha256_file(frozen_binary) != V0_BINARY_SHA256:
        raise EvaluationError("v0 frozen binary is not byte-identical to the preserved binary")

    for episode in PRIMARY_EPISODES:
        attempt_dir = baseline / "holdout" / episode.episode_id / "attempt"
        _verify_baseline_episode_seal(attempt_dir, episode)

    return {
        "state": "verified",
        "inventory_path": inventory_path.relative_to(root).as_posix(),
        "inventory_sha256": sha256_file(inventory_path),
        "regular_files_verified": checked,
        "freeze_manifest_sha256": V0_FREEZE_MANIFEST_SHA256,
        "frozen_binary_sha256": V0_BINARY_SHA256,
        "read_only": True,
    }


def _verify_baseline_episode_seal(attempt_dir: Path, episode: EpisodeSpec) -> None:
    seal_path = attempt_dir / "SEALED.json"
    seal = read_json(seal_path)
    if seal.get("episode_id") != episode.episode_id:
        raise EvaluationError(f"baseline seal episode mismatch: {seal_path}")
    if seal.get("base_revision") != episode.base_revision:
        raise EvaluationError(f"baseline seal revision mismatch: {seal_path}")
    artifact_hashes = seal.get("artifact_sha256")
    if not isinstance(artifact_hashes, dict) or set(artifact_hashes) != set(QUARTET):
        raise EvaluationError(f"baseline seal quartet mismatch: {seal_path}")
    run_dir = _discover_run_dir(attempt_dir)
    for name in QUARTET:
        target = run_dir / name
        if sha256_file(target) != artifact_hashes[name]:
            raise EvaluationError(f"baseline sealed artifact hash mismatch: {target}")
    audit = Audit()
    _verify_seal_inventory(
        attempt_dir,
        seal,
        [run_dir / name for name in QUARTET],
        f"baseline {episode.episode_id}",
        audit,
    )
    if audit.errors:
        raise EvaluationError(audit.errors[0])


def development_manifest() -> dict[str, Any]:
    return {
        "version": 1,
        "kind": "task_lens_v01_known_development",
        "fresh_generalization": False,
        "primary_episode_count": len(PRIMARY_EPISODES),
        "cheap_exit_calibration_count": len(CALIBRATION_EPISODES),
        "episodes": [
            {
                "episode_id": episode.episode_id,
                "base_revision": episode.base_revision,
                "evaluation_scope": episode.evaluation_scope,
                "cheap_exit_target": episode.cheap_exit_target,
                "final_attempt": f"episodes/{episode.episode_id}/final/attempt",
                "baseline_attempt": (
                    f"baseline/holdout/{episode.episode_id}/attempt"
                    if episode.evaluation_scope == "primary_regression"
                    else None
                ),
            }
            for episode in EPISODES
        ],
        "constraints": {
            "semantic_retries": 0,
            "maximum_model_calls_per_episode": 1,
            "maximum_total_fresh_model_calls": 8,
            "provider_model": V0_PROVIDER_MODEL,
            "provider_endpoint": V0_PROVIDER_ENDPOINT,
            "target_repository_commands": 0,
        },
    }


def scores_template() -> dict[str, Any]:
    return {
        "version": 1,
        "kind": "task_lens_v01_supervisor_scores",
        "known_development_only": True,
        "opaque_average_computed": False,
        "recommended_next_experiment": None,
        "episodes": [
            {
                "id": episode.episode_id,
                "evaluation_scope": episode.evaluation_scope,
                "scores": (
                    {dimension: None for dimension in SCORE_DIMENSIONS}
                    if episode.evaluation_scope == "primary_regression"
                    else {}
                ),
                "decisive_key_roles_present": None,
                "major_unsupported_claims": [],
                "absence_claims_from_incomplete_scope": [],
                "clipped_before_known_task_match_without_partial_window": [],
                "gold_loss_stage": None,
                "gold_candidate_id": None,
                "gold_anchor_id": None,
                "gold_loss_detail": None,
                "useful": None,
                "notes": [],
            }
            for episode in EPISODES
        ],
    }


def prepare(root: Path) -> dict[str, Any]:
    baseline = verify_baseline(root)
    evaluation_dir = root / "evaluation-v01"
    manifest_path = evaluation_dir / "DEVELOPMENT_SET.json"
    scores_path = evaluation_dir / "SCORES.template.json"
    _write_idempotent_json(manifest_path, development_manifest())
    _write_idempotent_json(scores_path, scores_template())
    return {
        "baseline": baseline,
        "development_set": str(manifest_path),
        "scores_template": str(scores_path),
        "canonical_final_attempt": "episodes/<episode-id>/final/attempt",
    }


def _write_idempotent_json(path: Path, value: Any) -> None:
    if path.exists():
        existing = read_json(path)
        if existing != value:
            raise EvaluationError(f"refusing to replace non-matching prepared input: {path}")
        return
    write_json(path, value)


def _discover_final_attempt(episode_dir: Path) -> Path:
    explicit_final_harnesses = sorted((episode_dir / "final").rglob("HARNESS_ATTEMPT.json"))
    if len(explicit_final_harnesses) > 1:
        raise EvaluationError(
            f"expected one immutable final attempt under {episode_dir / 'final'}; "
            f"found {len(explicit_final_harnesses)}"
        )
    preferred = (episode_dir / "final" / "attempt", episode_dir / "final", episode_dir / "attempt")
    for candidate in preferred:
        if candidate.is_dir() and (
            (candidate / "HARNESS_ATTEMPT.json").is_file()
            or (candidate / "SEALED.json").is_file()
            or _contains_quartet(candidate)
        ):
            return candidate

    candidates: list[Path] = []
    for harness_path in episode_dir.rglob("HARNESS_ATTEMPT.json"):
        attempt_dir = harness_path.parent
        try:
            harness = read_json(harness_path)
        except EvaluationError:
            continue
        phase = str(harness.get("phase", ""))
        if harness.get("final") is True or phase in {"final", "development_final", "dev_final"}:
            candidates.append(attempt_dir)
    unique = sorted(set(candidates))
    if len(unique) != 1:
        raise EvaluationError(
            f"expected exactly one final attempt under {episode_dir}; found {len(unique)}. "
            "Use episodes/<id>/final/attempt."
        )
    return unique[0]


def _contains_quartet(directory: Path) -> bool:
    return all((directory / name).is_file() for name in QUARTET)


def _discover_run_dir(attempt_dir: Path) -> Path:
    latest = attempt_dir / "run" / "latest"
    if latest.exists():
        try:
            resolved = latest.resolve(strict=True)
        except OSError as exc:
            raise EvaluationError(f"invalid run/latest under {attempt_dir}: {exc}") from exc
        if _contains_quartet(resolved):
            return resolved
    if _contains_quartet(attempt_dir):
        return attempt_dir
    candidates = sorted(
        directory
        for directory in {path.parent for name in QUARTET for path in attempt_dir.rglob(name)}
        if _contains_quartet(directory)
    )
    if len(candidates) != 1:
        raise EvaluationError(
            f"expected exactly one canonical quartet below {attempt_dir}; found {len(candidates)}"
        )
    return candidates[0]


def _optional_json(paths: Iterable[Path]) -> tuple[dict[str, Any] | None, Path | None]:
    for path in paths:
        if path.is_file():
            return read_json(path), path
    return None, None


def _optional_text(paths: Iterable[Path]) -> Path | None:
    for path in paths:
        if path.is_file():
            return path
    return None


def _support_paths(attempt_dir: Path, run_dir: Path, episode_dir: Path, names: Sequence[str]) -> list[Path]:
    result: list[Path] = []
    for base in (run_dir, attempt_dir, attempt_dir.parent, episode_dir):
        for name in names:
            candidate = base / name
            if candidate not in result:
                result.append(candidate)
    return result


def _get_object(container: Mapping[str, Any], names: Sequence[str]) -> dict[str, Any] | None:
    for name in names:
        value = container.get(name)
        if isinstance(value, dict):
            return value
    return None


def _get_list(container: Mapping[str, Any], names: Sequence[str]) -> list[Any] | None:
    for name in names:
        value = container.get(name)
        if isinstance(value, list):
            return value
    return None


def _provider_metrics(*documents: Mapping[str, Any]) -> dict[str, int]:
    found: list[dict[str, int]] = []
    fields = (
        "calls",
        "transport_attempts",
        "request_bytes",
        "response_bytes",
        "input_tokens",
        "output_tokens",
        "prompt_cache_hit_tokens",
        "prompt_cache_miss_tokens",
        "latency_millis",
    )
    for document in documents:
        provider = document.get("provider")
        if not isinstance(provider, dict):
            continue
        normalized: dict[str, int] = {}
        for name in fields:
            value = provider.get(name, 0)
            if isinstance(value, bool) or not isinstance(value, int) or value < 0:
                raise EvaluationError(f"provider.{name} must be a non-negative integer")
            normalized[name] = value
        found.append(normalized)
    if not found:
        raise EvaluationError("provider accounting is missing")
    first = found[0]
    for other in found[1:]:
        if other != first:
            raise EvaluationError("provider accounting differs across attempt/status/metrics")
    return first


def _validate_source_scope(scope: Mapping[str, Any], label: str, audit: Audit) -> None:
    required = (
        "scope_kind",
        "scope_start",
        "scope_end",
        "source_total_lines",
        "truncated",
        "truncation_reason",
        "task_matches_outside_window",
        "negative_claims_allowed",
        "negative_evidence_basis",
    )
    missing = [name for name in required if name not in scope]
    if missing:
        audit.error(f"{label}: source scope missing fields: {', '.join(missing)}")
        return
    kind = scope.get("scope_kind")
    if kind not in SCOPE_KINDS:
        audit.error(f"{label}: invalid scope_kind {kind!r}")
    start = scope.get("scope_start")
    end = scope.get("scope_end")
    total = scope.get("source_total_lines")
    if any(isinstance(value, bool) or not isinstance(value, int) for value in (start, end, total)):
        audit.error(f"{label}: scope line bounds must be integers")
    elif (
        start < 1
        or end < start
        or total < 0
        or (total == 0 and scope.get("truncated") is False)
        or (total > 0 and total < end)
    ):
        audit.error(f"{label}: invalid scope bounds {start}-{end} of {total}")
    for field_name in ("truncated", "task_matches_outside_window", "negative_claims_allowed"):
        if not isinstance(scope.get(field_name), bool):
            audit.error(f"{label}: {field_name} must be boolean")
    truncated = scope.get("truncated")
    negative_allowed = scope.get("negative_claims_allowed")
    reason = scope.get("truncation_reason")
    if truncated is True and (not isinstance(reason, str) or not reason.strip()):
        audit.error(f"{label}: truncated scope requires truncation_reason")
    if truncated is False and isinstance(reason, str) and reason.strip():
        audit.error(f"{label}: complete scope must not carry truncation_reason")
    is_complete = kind in {
        "complete_enclosing_symbol",
        "complete_document_section",
        "complete_file",
    }
    if is_complete and truncated is not False:
        audit.error(f"{label}: complete scope cannot be marked truncated")
    if kind in {"partial_window", "matched_fragments"} and truncated is not True:
        audit.error(f"{label}: {kind} must be marked truncated")
    basis = scope.get("negative_evidence_basis")
    if basis not in NEGATIVE_EVIDENCE_BASES:
        audit.error(f"{label}: invalid negative_evidence_basis {basis!r}")
    if kind == "partial_window" and negative_allowed is not False:
        audit.error(f"{label}: partial window must prohibit negative claims")
    if negative_allowed is True and not (
        is_complete or basis in {"exhaustive_bounded_exact_search", "deterministic_manifest"}
    ):
        audit.error(f"{label}: allowed negative claims lack complete or exhaustive authority")
    if basis == "complete_scope" and not is_complete:
        audit.error(f"{label}: incomplete scope cannot claim complete-scope authority")
    if kind == "complete_file" and (start != 1 or end != total):
        audit.error(f"{label}: complete_file must span the whole source")
    if scope.get("task_matches_outside_window") is True and kind not in {"partial_window", "matched_fragments"}:
        audit.error(f"{label}: task matches outside a supposedly complete scope")


def _normalize_selected_ids(value: Any) -> list[str]:
    if not isinstance(value, list):
        return []
    result: list[str] = []
    for item in value:
        if isinstance(item, str):
            result.append(item)
        elif isinstance(item, dict):
            identifier = item.get("anchor_id", item.get("id"))
            if isinstance(identifier, str) and identifier:
                result.append(identifier)
    return result


def _validate_trace(trace: Mapping[str, Any], label: str, audit: Audit) -> dict[str, Any]:
    required = (
        "task_kind",
        "task_profile",
        "task_terms",
        "candidates_before_ranking",
        "relationships",
        "selected_anchors",
        "dropped_anchors",
        "source_scopes",
        "role_coverage",
        "verification_frontier",
        "budgets",
        "limits",
    )
    for name in required:
        if name not in trace:
            audit.error(f"{label}: retrieval trace missing {name}")
    if trace.get("gold_assessment") is not None:
        audit.error(f"{label}: production retrieval trace was mutated with development gold")

    terms = trace.get("task_terms")
    if not isinstance(terms, list):
        audit.error(f"{label}: task_terms must be a list")
    else:
        normalized_terms: set[str] = set()
        for index, term in enumerate(terms):
            if not isinstance(term, dict):
                audit.error(f"{label}: task term {index} is not an object")
                continue
            normalized = term.get("normalized")
            if (
                not isinstance(term.get("text"), str)
                or not isinstance(normalized, str)
                or not normalized
                or not isinstance(term.get("found"), bool)
                or isinstance(term.get("weight"), bool)
                or not isinstance(term.get("weight"), int)
                or term.get("weight", -1) < 0
            ):
                audit.error(f"{label}: task term {index} is invalid")
                continue
            if normalized in normalized_terms:
                audit.error(f"{label}: duplicate normalized task term {normalized!r}")
            normalized_terms.add(normalized)

    candidates = trace.get("candidates_before_ranking")
    if not isinstance(candidates, list):
        audit.error(f"{label}: candidates_before_ranking must be a list")
        candidates = []
    candidate_ids: set[str] = set()
    discovery_orders: set[int] = set()
    for index, candidate in enumerate(candidates):
        if not isinstance(candidate, dict):
            audit.error(f"{label}: candidate {index} is not an object")
            continue
        if not candidate.get("path"):
            audit.error(f"{label}: candidate {index} has no path")
        candidate_id = candidate.get("id")
        if not isinstance(candidate_id, str) or not candidate_id or candidate_id in candidate_ids:
            audit.error(f"{label}: candidate {index} has an invalid or duplicate id")
        else:
            candidate_ids.add(candidate_id)
        if candidate.get("stage") not in {
            "initial",
            "completion_expansion_1",
            "completion_expansion_2",
            "verification_probe",
        }:
            audit.error(f"{label}: candidate {index} has an invalid retrieval stage")
        discovery_order = candidate.get("discovery_order")
        if (
            isinstance(discovery_order, bool)
            or not isinstance(discovery_order, int)
            or discovery_order <= 0
            or discovery_order in discovery_orders
        ):
            audit.error(f"{label}: candidate {index} has an invalid discovery_order")
        else:
            discovery_orders.add(discovery_order)
        roles = candidate.get("roles", candidate.get("candidate_roles"))
        if not isinstance(roles, list):
            audit.error(f"{label}: candidate {index} has no role list")
        if isinstance(candidate.get("score"), bool) or not isinstance(candidate.get("score"), (int, float)):
            audit.error(f"{label}: candidate {index} has no numeric score")
        components = candidate.get("score_components")
        if not isinstance(components, (dict, list)):
            audit.error(f"{label}: candidate {index} has no score_components")
        elif isinstance(components, list):
            component_total = 0
            for component_index, component in enumerate(components):
                if (
                    not isinstance(component, dict)
                    or not isinstance(component.get("kind"), str)
                    or isinstance(component.get("value"), bool)
                    or not isinstance(component.get("value"), int)
                ):
                    audit.error(
                        f"{label}: candidate {index} score component {component_index} is invalid"
                    )
                else:
                    component_total += component["value"]
            if candidate.get("score") != component_total:
                audit.error(f"{label}: candidate {index} score differs from its components")

    dropped = trace.get("dropped_anchors")
    if not isinstance(dropped, list):
        audit.error(f"{label}: dropped_anchors must be a list")
    else:
        for index, item in enumerate(dropped):
            if (
                not isinstance(item, dict)
                or item.get("candidate_id") not in candidate_ids
                or not str(item.get("reason", "")).strip()
            ):
                audit.error(f"{label}: dropped anchor {index} lacks an exact reason")

    selected_ids = _normalize_selected_ids(trace.get("selected_anchors"))
    if not selected_ids:
        audit.error(f"{label}: selected_anchors is empty")
    if len(selected_ids) != len(set(selected_ids)):
        audit.error(f"{label}: selected_anchors contains duplicates")
    selected_candidates: set[str] = set()
    selected_raw = trace.get("selected_anchors")
    if isinstance(selected_raw, list):
        ranks: list[int] = []
        for index, item in enumerate(selected_raw):
            if not isinstance(item, dict):
                audit.error(f"{label}: selected anchor {index} is not a selection object")
                continue
            candidate_id = item.get("candidate_id")
            rank = item.get("rank")
            if candidate_id not in candidate_ids or candidate_id in selected_candidates:
                audit.error(f"{label}: selected anchor {index} has an invalid candidate_id")
            else:
                selected_candidates.add(candidate_id)
            if isinstance(rank, bool) or not isinstance(rank, int) or rank <= 0:
                audit.error(f"{label}: selected anchor {index} has an invalid rank")
            else:
                ranks.append(rank)
            if not isinstance(item.get("reason"), str) or not item.get("reason"):
                audit.error(f"{label}: selected anchor {index} has no reason")
        if sorted(ranks) != list(range(1, len(ranks) + 1)):
            audit.error(f"{label}: selected anchor ranks are not contiguous")
    dropped_candidates = {
        item.get("candidate_id") for item in dropped if isinstance(item, dict)
    } if isinstance(dropped, list) else set()
    if selected_candidates & dropped_candidates or selected_candidates | dropped_candidates != candidate_ids:
        audit.error(f"{label}: every candidate must have exactly one selected/dropped outcome")

    scopes = trace.get("source_scopes")
    scope_by_id: dict[str, Mapping[str, Any]] = {}
    if isinstance(scopes, dict):
        scopes = [
            {"anchor_id": anchor_id, **scope}
            for anchor_id, scope in scopes.items()
            if isinstance(anchor_id, str) and isinstance(scope, dict)
        ]
    if not isinstance(scopes, list):
        audit.error(f"{label}: source_scopes must be a list")
        scopes = []
    for index, raw_scope in enumerate(scopes):
        if not isinstance(raw_scope, dict):
            audit.error(f"{label}: source scope {index} is not an object")
            continue
        anchor_id = raw_scope.get("anchor_id", raw_scope.get("id"))
        nested_scope = raw_scope.get("scope")
        scope_value = nested_scope if isinstance(nested_scope, dict) else raw_scope
        if not isinstance(anchor_id, str) or not anchor_id:
            audit.error(f"{label}: source scope {index} has no anchor_id")
        else:
            scope_by_id[anchor_id] = scope_value
        _validate_source_scope(scope_value, f"{label} scope {index}", audit)
    missing_scopes = sorted(set(selected_ids) - set(scope_by_id))
    if missing_scopes:
        audit.error(f"{label}: selected anchors lack source scopes: {', '.join(missing_scopes)}")

    relationships = trace.get("relationships")
    if not isinstance(relationships, list):
        audit.error(f"{label}: relationships must be a list")
        relationships = []
    decisive_relations = 0
    for index, relation in enumerate(relationships):
        if not isinstance(relation, dict):
            audit.error(f"{label}: relationship {index} is not an object")
            continue
        kind = relation.get("kind", relation.get("relation_kind"))
        support = relation.get("support_class", relation.get("support_type"))
        if kind not in RELATION_KINDS:
            audit.error(f"{label}: relationship {index} has invalid kind {kind!r}")
        if support not in SUPPORT_CLASSES:
            audit.error(f"{label}: relationship {index} has invalid support class {support!r}")
        left_id = relation.get("left_candidate_id", relation.get("left_anchor_id"))
        right_id = relation.get("right_candidate_id", relation.get("right_anchor_id"))
        if left_id not in candidate_ids or right_id not in candidate_ids or left_id == right_id:
            audit.error(f"{label}: relationship {index} has invalid candidate endpoints")
        evidence = relation.get("evidence_ids", relation.get("support_ids"))
        if not isinstance(evidence, list):
            audit.error(f"{label}: relationship {index} has no evidence_ids")
        non_guarantees = relation.get("non_guarantees", relation.get("scope_non_guarantees"))
        if not isinstance(non_guarantees, (str, list)) or not non_guarantees:
            audit.error(f"{label}: relationship {index} has no explicit non-guarantees")
        if support in {"locally_observed", "document_supported"} and kind != "scope_unknown":
            decisive_relations += 1

    role_coverage = trace.get("role_coverage")
    role_result = _validate_role_coverage(role_coverage, f"{label} role coverage", audit)
    if isinstance(role_coverage, dict) and role_coverage.get("profile") != trace.get("task_profile"):
        audit.error(f"{label}: role coverage profile disagrees with task profile")
    frontier = trace.get("verification_frontier")
    frontier_result = _validate_verification_frontier(frontier, f"{label} verification frontier", audit)
    if not set(role_result["anchor_ids"]).issubset(selected_ids):
        audit.error(f"{label}: role coverage references an unselected anchor")
    if not set(frontier_result["anchor_ids"]).issubset(selected_ids):
        audit.error(f"{label}: verification frontier references an unselected anchor")
    _validate_limits(trace.get("limits"), f"{label} limits", audit)
    return {
        "candidate_ids": sorted(candidate_ids),
        "selected_anchor_count": len(selected_ids),
        "selected_anchor_ids": selected_ids,
        "partial_scope_count": sum(bool(scope.get("truncated")) for scope in scope_by_id.values()),
        "missing_key_roles": role_result["missing_key_roles"],
        "grounded_verification": frontier_result["grounded"],
        "decisive_local_relation_count": decisive_relations,
    }


def _validate_role_contract(value: Any, label: str, audit: Audit) -> None:
    if not isinstance(value, dict):
        audit.error(f"{label}: task-role contract is missing")
        return
    if not isinstance(value.get("profile"), str) or not value.get("profile"):
        audit.error(f"{label}: role contract profile is missing")
    for importance in ("key", "supporting", "optional"):
        roles = value.get(importance)
        if not isinstance(roles, list):
            audit.error(f"{label}: role contract {importance} must be a list")
            continue
        for index, role in enumerate(roles):
            if not isinstance(role, dict) or not isinstance(role.get("role"), str):
                audit.error(f"{label}: invalid {importance} role {index}")
                continue
            minimum = role.get("minimum_anchors")
            if isinstance(minimum, bool) or not isinstance(minimum, int) or minimum <= 0:
                audit.error(f"{label}: invalid minimum_anchors for {importance} role {index}")


def _validate_role_coverage(value: Any, label: str, audit: Audit) -> dict[str, Any]:
    missing_key_roles: list[str] = []
    referenced_anchor_ids: set[str] = set()
    if not isinstance(value, dict):
        audit.error(f"{label}: role coverage is missing")
        return {"missing_key_roles": ["<coverage-missing>"], "anchor_ids": []}
    if not isinstance(value.get("profile"), str) or not value.get("profile"):
        audit.error(f"{label}: role coverage profile is missing")
    for importance in ("key", "supporting", "optional"):
        roles = value.get(importance)
        if not isinstance(roles, list):
            audit.error(f"{label}: {importance} must be a list")
            continue
        for index, role in enumerate(roles):
            if not isinstance(role, dict):
                audit.error(f"{label}: invalid {importance} role {index}")
                continue
            name = role.get("role")
            anchors = role.get("anchor_ids")
            minimum = role.get("minimum_anchors")
            represented = role.get("represented")
            if not isinstance(name, str) or not name:
                audit.error(f"{label}: {importance} role {index} has no name")
                continue
            if not isinstance(anchors, list) or not all(isinstance(item, str) for item in anchors):
                audit.error(f"{label}: role {name} anchor_ids must be strings")
                anchors = []
            referenced_anchor_ids.update(anchors)
            if isinstance(minimum, bool) or not isinstance(minimum, int) or minimum <= 0:
                audit.error(f"{label}: role {name} minimum_anchors is invalid")
                minimum = 0
            calculated = len(anchors) >= minimum
            if not isinstance(represented, bool) or represented != calculated:
                audit.error(f"{label}: role {name} represented flag disagrees with anchor_ids")
            if importance == "key" and not calculated:
                missing_key_roles.append(name)
    return {"missing_key_roles": missing_key_roles, "anchor_ids": sorted(referenced_anchor_ids)}


def _frontier_items(value: Any) -> list[Mapping[str, Any]]:
    if value is None:
        return []
    if isinstance(value, dict):
        return [value]
    if isinstance(value, list):
        return [item for item in value if isinstance(item, dict)]
    return []


def _validate_verification_frontier(value: Any, label: str, audit: Audit) -> dict[str, Any]:
    if not isinstance(value, dict):
        audit.error(f"{label}: verification frontier is missing")
        return {"grounded": False, "authorities": [], "anchor_ids": []}
    decisive_anchor_id = value.get("decisive_anchor_id", "")
    if not isinstance(decisive_anchor_id, str):
        audit.error(f"{label}: decisive_anchor_id must be a string")
    anchors = _frontier_items(value.get("anchors"))
    fixtures = _frontier_items(value.get("fixture"))
    commands = _frontier_items(value.get("command_or_effect"))
    if len(anchors) > 2:
        audit.error(f"{label}: more than two verification anchors")
    if len(fixtures) > 1:
        audit.error(f"{label}: more than one fixture")
    if len(commands) > 1:
        audit.error(f"{label}: more than one command/effect")
    authorities: list[str] = []
    referenced_anchor_ids: set[str] = set()
    for index, item in enumerate((*anchors, *fixtures, *commands)):
        authority = item.get("authority")
        anchor_id = item.get("anchor_id")
        if isinstance(anchor_id, str) and anchor_id:
            referenced_anchor_ids.add(anchor_id)
        if authority not in VERIFICATION_AUTHORITIES:
            audit.error(f"{label}: item {index} has invalid authority {authority!r}")
        else:
            authorities.append(authority)
        evidence = item.get("evidence_ids")
        if authority in GROUNDED_AUTHORITIES and (not isinstance(evidence, list) or not evidence):
            audit.error(f"{label}: grounded item {index} has no evidence_ids")
    for index, item in enumerate(anchors):
        if item.get("authority") in {"exact_generated_fixture", "documented_command"}:
            audit.error(f"{label}: anchor item {index} is in the wrong frontier slot")
    if fixtures and fixtures[0].get("authority") != "exact_generated_fixture":
        audit.error(f"{label}: fixture has invalid authority")
    if commands and commands[0].get("authority") != "documented_command":
        audit.error(f"{label}: command/effect has invalid authority")
    grounded = bool(decisive_anchor_id) and any(item in GROUNDED_AUTHORITIES for item in authorities)
    if decisive_anchor_id:
        referenced_anchor_ids.add(decisive_anchor_id)
    return {
        "grounded": grounded,
        "authorities": authorities,
        "anchor_ids": sorted(referenced_anchor_ids),
    }


def _validate_limits(value: Any, label: str, audit: Audit) -> None:
    if not isinstance(value, (list, dict)):
        audit.error(f"{label}: limit causation record is missing")
        return
    records = list(value.values()) if isinstance(value, dict) else value
    seen_names: set[str] = set()
    for index, record in enumerate(records):
        if isinstance(record, bool):
            continue
        if not isinstance(record, dict):
            audit.error(f"{label}: invalid limit record {index}")
            continue
        name = record.get("name")
        limit = record.get("limit")
        observed = record.get("observed")
        applied = record.get("applied")
        loss_reason = record.get("loss_reason", "")
        if not isinstance(name, str) or not name or name in seen_names:
            audit.error(f"{label}: limit record {index} has an invalid or duplicate name")
        else:
            seen_names.add(name)
        if (
            isinstance(limit, bool)
            or not isinstance(limit, int)
            or limit < 0
            or isinstance(observed, bool)
            or not isinstance(observed, int)
            or observed < 0
        ):
            audit.error(f"{label}: limit record {index} has invalid numeric accounting")
        if not isinstance(applied, bool):
            audit.error(f"{label}: limit record {index} applied must be boolean")
        if not isinstance(loss_reason, str):
            audit.error(f"{label}: limit record {index} loss_reason must be a string")
        caused = record.get("caused_loss", record.get("limit_caused_loss"))
        if not isinstance(caused, bool):
            audit.error(f"{label}: limit record {index} must state whether it caused loss")
        elif caused and (applied is not True or not loss_reason):
            audit.error(f"{label}: loss-causing limit {name!r} lacks an exact reason")
        elif not caused and loss_reason:
            audit.error(f"{label}: non-loss limit {name!r} carries a loss reason")


def _validate_cheap_exit(
    value: Any,
    calls: int,
    label: str,
    audit: Audit,
    *,
    allow_deferred_single_call: bool = False,
) -> dict[str, Any]:
    if not isinstance(value, dict):
        audit.error(f"{label}: CheapExitDecision is missing")
        return {"eligible": False, "route": "unknown", "reasons": []}
    eligible = value.get("eligible")
    route = value.get("route")
    gates = value.get("gates")
    reasons = value.get("reasons")
    if not isinstance(eligible, bool):
        audit.error(f"{label}: cheap-exit eligible must be boolean")
        eligible = False
    if route not in {"zero_call", "single_synthesis_call"}:
        audit.error(f"{label}: invalid cheap-exit route {route!r}")
    if (
        not isinstance(reasons, list)
        or not all(isinstance(reason, str) and reason for reason in reasons)
    ):
        audit.error(f"{label}: cheap-exit reasons must be non-empty strings")
        reasons = []
    seen_gates: set[str] = set()
    if not isinstance(gates, list):
        audit.error(f"{label}: cheap-exit gates must be a list")
        gates = []
    for index, gate in enumerate(gates):
        if not isinstance(gate, dict):
            audit.error(f"{label}: invalid cheap-exit gate {index}")
            continue
        name = gate.get("gate")
        passed = gate.get("passed")
        reason = gate.get("reason")
        if not isinstance(name, str) or not name:
            audit.error(f"{label}: cheap-exit gate {index} has no name")
            continue
        seen_gates.add(name)
        if not isinstance(passed, bool):
            audit.error(f"{label}: cheap-exit gate {name} passed must be boolean")
        if not isinstance(reason, str) or not reason:
            audit.error(f"{label}: cheap-exit gate {name} has no reason")
        if eligible and passed is not True:
            audit.error(f"{label}: eligible cheap exit has a failing gate: {name}")
    missing = sorted(CHEAP_EXIT_GATES - seen_gates)
    if missing:
        audit.error(f"{label}: cheap-exit decision lacks gates: {', '.join(missing)}")
    if eligible != (route == "zero_call"):
        audit.error(f"{label}: eligible and route are inconsistent")
    if route == "zero_call" and calls != 0:
        audit.error(f"{label}: zero_call route recorded {calls} provider calls")
    if (
        route == "single_synthesis_call"
        and calls != 1
        and not (allow_deferred_single_call and calls == 0)
    ):
        audit.error(f"{label}: single_synthesis_call route recorded {calls} provider calls")
    return {"eligible": eligible, "route": route, "reasons": reasons}


def _is_offline_partial_context(
    harness: Mapping[str, Any],
    attempt: Mapping[str, Any],
    status: Mapping[str, Any],
    provider: Mapping[str, int],
) -> bool:
    return (
        harness.get("offline") is True
        and attempt.get("state") == "skipped_offline"
        and status.get("state") == "partial_local"
        and status.get("sufficient") is False
        and all(value == 0 for value in provider.values())
    )


def _validate_execution_protocol(
    harness: Mapping[str, Any],
    attempt: Mapping[str, Any],
    status: Mapping[str, Any],
    provider: Mapping[str, int],
    cheap_exit: Mapping[str, Any],
    label: str,
    audit: Audit,
) -> None:
    offline = harness.get("offline")
    if not isinstance(offline, bool):
        audit.error(f"{label}: harness offline must be boolean")
        return

    calls = provider["calls"]
    route = cheap_exit.get("route")
    eligible = cheap_exit.get("eligible")
    if offline:
        if attempt.get("state") != "skipped_offline":
            audit.error(f"{label}: offline attempt state must be skipped_offline")
        if status.get("state") != "partial_local":
            audit.error(f"{label}: offline status state must be partial_local")
        if status.get("sufficient") is not False:
            audit.error(f"{label}: offline partial status must have sufficient=false")
        if any(value != 0 for value in provider.values()):
            audit.error(f"{label}: offline partial attempt must have all provider counters at zero")
        if route != "single_synthesis_call" or eligible is not False:
            audit.error(
                f"{label}: offline partial attempt must retain an ineligible "
                "single_synthesis_call decision"
            )
        return

    if attempt.get("state") == "skipped_offline":
        audit.error(f"{label}: skipped_offline attempt requires harness offline=true")
    if route == "zero_call":
        if attempt.get("state") != "skipped_local_complete":
            audit.error(f"{label}: zero_call attempt state must be skipped_local_complete")
        if status.get("state") != "accepted_local_complete":
            audit.error(f"{label}: zero_call status state must be accepted_local_complete")
        if status.get("sufficient") is not True:
            audit.error(f"{label}: zero_call status must have sufficient=true")


def _find_metadata(run_dir: Path) -> tuple[dict[str, Any] | None, Path | None]:
    return _optional_json((run_dir / "metadata.json",))


def _validate_provider_identity(
    metadata: Mapping[str, Any] | None,
    provider: Mapping[str, int],
    label: str,
    audit: Audit,
) -> None:
    if metadata is None:
        audit.error(f"{label}: run metadata is missing")
        return
    calls = provider["calls"]
    if metadata.get("model") != V0_PROVIDER_MODEL:
        audit.error(f"{label}: provider model differs from v0: {metadata.get('model')!r}")
    if metadata.get("endpoint") != V0_PROVIDER_ENDPOINT:
        audit.error(f"{label}: provider endpoint differs from v0: {metadata.get('endpoint')!r}")
    if "provider_request_count" not in metadata and all(value == 0 for value in provider.values()):
        return
    count = metadata.get("provider_request_count")
    if isinstance(count, bool) or not isinstance(count, int) or count < 0:
        audit.error(f"{label}: metadata provider_request_count is invalid: {count!r}")
    elif count != calls:
        audit.error(f"{label}: metadata provider_request_count {count!r} != {calls}")


def _verify_seal_inventory(
    attempt_dir: Path,
    seal: Mapping[str, Any],
    artifacts: Sequence[Path],
    label: str,
    audit: Audit,
) -> None:
    files = seal.get("files")
    if not isinstance(files, list):
        audit.error(f"{label}: SEALED.json has no file inventory")
        return
    digest_inventory: set[tuple[str, str]] = set()
    seen_paths: set[str] = set()
    for index, entry in enumerate(files):
        if not isinstance(entry, dict):
            audit.error(f"{label}: invalid sealed inventory entry {index}")
            continue
        raw_path = entry.get("path")
        digest = entry.get("sha256")
        kind = entry.get("kind")
        if not isinstance(raw_path, str) or not isinstance(digest, str):
            audit.error(f"{label}: sealed inventory entry {index} lacks path/hash")
            continue
        try:
            relative_path = _safe_relative_path(raw_path, attempt_dir / "SEALED.json")
        except EvaluationError as exc:
            audit.error(f"{label}: {exc}")
            continue
        relative = relative_path.as_posix()
        if relative in seen_paths:
            audit.error(f"{label}: duplicate sealed inventory path {relative}")
            continue
        seen_paths.add(relative)
        target = attempt_dir / relative_path
        if kind == "file":
            if not target.is_file() or target.is_symlink():
                audit.error(f"{label}: sealed file is missing or not regular: {relative}")
                continue
            actual = sha256_file(target)
            if actual != digest:
                audit.error(f"{label}: sealed file hash mismatch: {relative}")
                continue
            expected_bytes = entry.get("bytes")
            if expected_bytes != target.stat().st_size:
                audit.error(f"{label}: sealed file byte count mismatch: {relative}")
                continue
            digest_inventory.add((relative, digest))
        elif kind == "symlink":
            if not target.is_symlink():
                audit.error(f"{label}: sealed symlink is missing: {relative}")
                continue
            link_target = os.readlink(target)
            if entry.get("target") != link_target:
                audit.error(f"{label}: sealed symlink target mismatch: {relative}")
            if digest != sha256_bytes(link_target.encode("utf-8")):
                audit.error(f"{label}: sealed symlink hash mismatch: {relative}")
        else:
            audit.error(f"{label}: unsupported sealed inventory kind {kind!r}")
    for artifact in artifacts:
        try:
            relative = artifact.resolve(strict=True).relative_to(attempt_dir.resolve(strict=True)).as_posix()
        except (OSError, ValueError) as exc:
            audit.error(f"{label}: sealed artifact is outside the final attempt: {artifact}")
            continue
        digest = sha256_file(artifact)
        if (relative, digest) not in digest_inventory:
            audit.error(f"{label}: sealed inventory does not bind {relative} ({digest})")


def _negative_claims(pack: Mapping[str, Any]) -> list[str]:
    fragments: list[str] = []
    for item in pack.get("working_hypothesis", []):
        if isinstance(item, dict) and isinstance(item.get("text"), str):
            fragments.append(item["text"])
    for item in pack.get("likely_areas", []):
        if isinstance(item, dict) and isinstance(item.get("why"), str):
            fragments.append(item["why"])
    for item in pack.get("evidence_joins", []):
        if isinstance(item, dict) and isinstance(item.get("explanation"), str):
            fragments.append(item["explanation"])
    pattern = re.compile(r"\b(absent|lacks?|no\s+(?:test|fixture|check|support)|does\s+not\s+(?:contain|include)|never)\b", re.I)
    return [fragment for fragment in fragments if pattern.search(fragment)]


def validate_episode(root: Path, episode: EpisodeSpec) -> dict[str, Any]:
    root = root.resolve()
    label = episode.episode_id
    audit = Audit()
    episode_dir = root / "episodes" / label
    if not episode_dir.is_dir():
        raise EvaluationError(f"{label}: missing episode directory {episode_dir}")
    attempt_dir = _discover_final_attempt(episode_dir)
    run_dir = _discover_run_dir(attempt_dir)

    documents = {name: read_json(run_dir / name) for name in QUARTET}
    bundle = documents[QUARTET[0]]
    attempt = documents[QUARTET[1]]
    pack = documents[QUARTET[2]]
    status = documents[QUARTET[3]]

    bundle_hash = go_compact_json_sha256(bundle)
    for document_name, document in (("attempt", attempt), ("pack", pack), ("status", status)):
        if document.get("bundle_sha256") != bundle_hash:
            audit.error(f"{label}: {document_name} bundle_sha256 does not bind canonical bundle")
    if status.get("attempt_sha256") != sha256_file(run_dir / QUARTET[1]):
        audit.error(f"{label}: status attempt_sha256 mismatch")
    if status.get("pack_sha256") != sha256_file(run_dir / QUARTET[2]):
        audit.error(f"{label}: status pack_sha256 mismatch")

    repository = bundle.get("repository")
    if not isinstance(repository, dict):
        audit.error(f"{label}: bundle repository is missing")
        repository = {}
    revision = repository.get("revision")
    if revision != episode.base_revision:
        audit.error(f"{label}: bundle revision {revision!r} != {episode.base_revision}")
    pack_repository = pack.get("repository")
    if not isinstance(pack_repository, dict) or pack_repository.get("revision") != episode.base_revision:
        audit.error(f"{label}: pack revision mismatch")
    if status.get("captured_revision") != episode.base_revision:
        audit.error(f"{label}: status revision mismatch")
    task_id = bundle.get("id")
    if pack.get("id") != task_id or status.get("task_id") != task_id:
        audit.error(f"{label}: quartet task IDs disagree")

    task_path = episode_dir / "task.md"
    if task_path.is_file():
        task = bundle.get("task")
        task_text = task.get("text") if isinstance(task, dict) else None
        if not isinstance(task_text, str) or task_text.strip() != task_path.read_text(encoding="utf-8").strip():
            audit.error(f"{label}: task.md does not match bundled task text")
    else:
        audit.error(f"{label}: task.md is missing")

    harness, harness_path = _optional_json((attempt_dir / "HARNESS_ATTEMPT.json",))
    if harness is None:
        audit.error(f"{label}: HARNESS_ATTEMPT.json is missing")
        harness = {}
    else:
        if harness.get("episode_id") != label:
            audit.error(f"{label}: harness episode ID mismatch")
        if harness.get("base_revision") != episode.base_revision:
            audit.error(f"{label}: harness base revision mismatch")
        if harness.get("semantic_retry") is not False:
            audit.error(f"{label}: semantic_retry must be false")
        if harness.get("one_process_invocation") is not True:
            audit.error(f"{label}: one_process_invocation must be true")
        if harness.get("return_code") != 0:
            audit.error(f"{label}: final process did not return zero")
    wall_millis = harness.get("wall_millis", 0)
    if isinstance(wall_millis, bool) or not isinstance(wall_millis, int) or wall_millis < 0:
        audit.error(f"{label}: harness wall_millis must be a non-negative integer")
        wall_millis = 0

    metrics, metrics_path = _optional_json((attempt_dir / "METRICS.json", run_dir / "METRICS.json"))
    if metrics is None:
        audit.error(f"{label}: METRICS.json is missing")
        metrics = {}
    try:
        provider = _provider_metrics(attempt, status, metrics)
    except EvaluationError as exc:
        audit.error(f"{label}: {exc}")
        provider = {name: 0 for name in (
            "calls", "transport_attempts", "request_bytes", "response_bytes", "input_tokens",
            "output_tokens", "prompt_cache_hit_tokens", "prompt_cache_miss_tokens", "latency_millis",
        )}
    calls = provider["calls"]
    if calls not in {0, 1}:
        audit.error(f"{label}: provider calls must be zero or one, got {calls}")
    if calls == 0 and any(
        provider[name] != 0
        for name in (
            "transport_attempts",
            "request_bytes",
            "response_bytes",
            "input_tokens",
            "output_tokens",
            "prompt_cache_hit_tokens",
            "prompt_cache_miss_tokens",
            "latency_millis",
        )
    ):
        audit.error(f"{label}: zero-call route has non-zero provider accounting")
    if calls == 1 and (provider["transport_attempts"] < 1 or provider["request_bytes"] < 1):
        audit.error(f"{label}: one-call route has incomplete provider accounting")
    metadata, metadata_path = _find_metadata(run_dir)
    _validate_provider_identity(metadata, provider, label, audit)

    trace, trace_path = _optional_json(
        _support_paths(attempt_dir, run_dir, episode_dir, ("retrieval_trace.json",))
    )
    trace_md_path = _optional_text(
        _support_paths(attempt_dir, run_dir, episode_dir, ("retrieval_trace.md",))
    )
    if trace is None or trace_path is None:
        audit.error(f"{label}: retrieval_trace.json is missing")
        trace = {}
        trace_result = {
            "candidate_ids": [],
            "selected_anchor_count": 0,
            "selected_anchor_ids": [],
            "partial_scope_count": 0,
            "missing_key_roles": ["<trace-missing>"],
            "grounded_verification": False,
            "decisive_local_relation_count": 0,
        }
    else:
        trace_result = _validate_trace(trace, label, audit)
    if trace_md_path is None:
        audit.error(f"{label}: retrieval_trace.md is missing")
    if trace_path is not None and status.get("retrieval_trace_sha256") != sha256_file(trace_path):
        audit.error(f"{label}: status retrieval_trace_sha256 mismatch")
    if (
        trace_md_path is not None
        and status.get("retrieval_trace_markdown_sha256") != sha256_file(trace_md_path)
    ):
        audit.error(f"{label}: status retrieval_trace_markdown_sha256 mismatch")

    bundle_anchors = bundle.get("anchors")
    bundle_anchor_ids = {
        item.get("id")
        for item in bundle_anchors
        if isinstance(item, dict) and isinstance(item.get("id"), str)
    } if isinstance(bundle_anchors, list) else set()
    if set(trace_result["selected_anchor_ids"]) != bundle_anchor_ids:
        audit.error(f"{label}: trace selected anchors do not exactly match the bundle")
    pack_anchors = pack.get("investigation_anchors")
    pack_anchor_ids = {
        item.get("id")
        for item in pack_anchors
        if isinstance(item, dict) and isinstance(item.get("id"), str)
    } if isinstance(pack_anchors, list) else set()
    if not pack_anchor_ids.issubset(bundle_anchor_ids):
        audit.error(f"{label}: pack references anchors outside the sealed bundle")

    role_contract, role_contract_path = _optional_json(
        _support_paths(
            attempt_dir,
            run_dir,
            episode_dir,
            ("task_role_contract.json", "role_contract.json"),
        )
    )
    if role_contract is None:
        role_contract = _get_object(bundle, ("task_role_contract", "role_contract"))
    if role_contract is None:
        role_contract = _get_object(trace, ("task_role_contract", "role_contract"))
    _validate_role_contract(role_contract, label, audit)
    if isinstance(role_contract, dict) and role_contract.get("profile") != trace.get("task_profile"):
        audit.error(f"{label}: role contract profile disagrees with retrieval trace")

    frontier, frontier_path = _optional_json(
        _support_paths(attempt_dir, run_dir, episode_dir, ("verification_frontier.json",))
    )
    if frontier is None:
        frontier = _get_object(bundle, ("verification_frontier",))
    if frontier is None:
        frontier = _get_object(trace, ("verification_frontier",))
    frontier_result = _validate_verification_frontier(frontier, f"{label} projection", audit)

    status_sufficient = status.get("sufficient")
    if not isinstance(status_sufficient, bool):
        audit.error(f"{label}: status sufficient must be boolean")
        status_sufficient = False

    cheap_exit, cheap_exit_path = _optional_json(
        _support_paths(attempt_dir, run_dir, episode_dir, ("cheap_exit_decision.json",))
    )
    if cheap_exit is None:
        cheap_exit = _get_object(attempt, ("cheap_exit", "cheap_exit_decision", "synthesis_decision"))
    if cheap_exit is None:
        cheap_exit = _get_object(status, ("cheap_exit", "cheap_exit_decision", "synthesis_decision"))
    if cheap_exit is None:
        cheap_exit = _get_object(bundle, ("cheap_exit", "cheap_exit_decision"))
    cheap_result = _validate_cheap_exit(
        cheap_exit,
        calls,
        label,
        audit,
        allow_deferred_single_call=_is_offline_partial_context(
            harness,
            attempt,
            status,
            provider,
        ),
    )
    _validate_execution_protocol(
        harness,
        attempt,
        status,
        provider,
        cheap_result,
        label,
        audit,
    )
    if isinstance(cheap_exit, dict):
        for projection_name, projection in (
            ("bundle", _get_object(bundle, ("cheap_exit",))),
            ("pack", _get_object(pack, ("cheap_exit",))),
            ("status", _get_object(status, ("cheap_exit",))),
        ):
            if projection != cheap_exit:
                audit.error(f"{label}: {projection_name} cheap-exit projection disagrees")
    bundle_coverage = _get_object(bundle, ("role_coverage",))
    if _get_object(bundle, ("role_contract",)) != role_contract:
        audit.error(f"{label}: bundle role contract disagrees with sealed projection")
    if bundle_coverage != trace.get("role_coverage"):
        audit.error(f"{label}: bundle role coverage disagrees with trace")
    if _get_object(pack, ("role_coverage",)) != bundle_coverage:
        audit.error(f"{label}: pack role coverage disagrees with bundle")
    if _get_object(pack, ("role_contract",)) != role_contract:
        audit.error(f"{label}: pack role contract disagrees with bundle")
    if _get_object(bundle, ("verification_frontier",)) != frontier:
        audit.error(f"{label}: bundle verification frontier disagrees with trace")
    if _get_object(trace, ("verification_frontier",)) != frontier:
        audit.error(f"{label}: retrieval trace verification frontier disagrees with bundle")
    if _get_object(pack, ("verification_frontier",)) != frontier:
        audit.error(f"{label}: pack verification frontier disagrees with trace")

    if status_sufficient:
        if trace_result["missing_key_roles"]:
            audit.error(f"{label}: sufficient=true with missing key roles")
        if trace_result["decisive_local_relation_count"] < 1:
            audit.error(f"{label}: sufficient=true without a decisive supported relation")
        if not trace_result["grounded_verification"] or not frontier_result["grounded"]:
            audit.error(f"{label}: sufficient=true without grounded verification")

    negative_claims = _negative_claims(pack)
    if negative_claims and trace_result["partial_scope_count"]:
        audit.warn(
            f"{label}: absence-style prose and partial scopes coexist; the scored claim audit must "
            "confirm that every claim is bound only to negative-authorized anchors"
        )

    seal, seal_path = _optional_json((attempt_dir / "SEALED.json",))
    if seal is None or seal_path is None:
        audit.error(f"{label}: SEALED.json is missing")
    else:
        if seal.get("episode_id") != label:
            audit.error(f"{label}: seal episode ID mismatch")
        if seal.get("base_revision") != episode.base_revision:
            audit.error(f"{label}: seal revision mismatch")
        artifact_hashes = seal.get("artifact_sha256")
        if not isinstance(artifact_hashes, dict) or not set(QUARTET).issubset(artifact_hashes):
            audit.error(f"{label}: seal does not bind the canonical quartet")
        else:
            for name in QUARTET:
                if artifact_hashes.get(name) != sha256_file(run_dir / name):
                    audit.error(f"{label}: seal quartet hash mismatch for {name}")
        if seal.get("state") != status.get("state") or seal.get("sufficient") != status_sufficient:
            audit.error(f"{label}: seal state/sufficiency differs from status")
        sealed_artifacts = [run_dir / name for name in QUARTET]
        for optional_path in (
            trace_path,
            trace_md_path,
            role_contract_path,
            frontier_path,
            cheap_exit_path,
            metrics_path,
            harness_path,
            metadata_path,
        ):
            if optional_path is not None:
                sealed_artifacts.append(optional_path)
        _verify_seal_inventory(attempt_dir, seal, sealed_artifacts, label, audit)

    return {
        "id": label,
        "evaluation_scope": episode.evaluation_scope,
        "attempt_dir": attempt_dir.relative_to(root).as_posix(),
        "run_dir": run_dir.relative_to(root).as_posix(),
        "artifact_valid": not audit.errors,
        "errors": audit.errors,
        "warnings": audit.warnings,
        "provider": provider,
        "wall_millis": wall_millis,
        "offline": harness.get("offline") is True,
        "status": {"state": status.get("state"), "sufficient": status_sufficient},
        "cheap_exit": cheap_result,
        "trace": {
            **trace_result,
            "gold_loss_stage": None,
            "sha256": sha256_file(trace_path) if trace_path else None,
            "json_path": trace_path.relative_to(root).as_posix() if trace_path else None,
            "markdown_path": trace_md_path.relative_to(root).as_posix() if trace_md_path else None,
        },
        "negative_claims_detected": negative_claims,
    }


def validate_artifacts(root: Path) -> dict[str, Any]:
    baseline = verify_baseline(root)
    episodes: list[dict[str, Any]] = []
    top_errors: list[str] = []
    for episode in EPISODES:
        try:
            episodes.append(validate_episode(root, episode))
        except EvaluationError as exc:
            episodes.append(
                {
                    "id": episode.episode_id,
                    "evaluation_scope": episode.evaluation_scope,
                    "artifact_valid": False,
                    "errors": [str(exc)],
                    "warnings": [],
                    "provider": {"calls": 0},
                    "wall_millis": 0,
                    "offline": False,
                    "status": {"state": "missing", "sufficient": False},
                    "cheap_exit": {"eligible": False, "route": "unknown", "reasons": []},
                    "trace": {},
                    "negative_claims_detected": [],
                }
            )
    total_calls = sum(int(episode["provider"].get("calls", 0)) for episode in episodes)
    if total_calls > 8:
        top_errors.append(f"final run used {total_calls} model calls; maximum is 8")
    for episode in episodes:
        if int(episode["provider"].get("calls", 0)) not in {0, 1}:
            top_errors.append(f"{episode['id']} exceeded one model call")
    all_errors = [*top_errors, *(error for episode in episodes for error in episode["errors"])]
    return {
        "version": 1,
        "kind": "task_lens_v01_artifact_validation",
        "validated_at": utc_now(),
        "known_development_only": True,
        "target_repository_commands_executed": 0,
        "baseline": baseline,
        "episodes": episodes,
        "resource_totals": {
            "model_calls": total_calls,
            "input_tokens": sum(int(item["provider"].get("input_tokens", 0)) for item in episodes),
            "output_tokens": sum(int(item["provider"].get("output_tokens", 0)) for item in episodes),
            "provider_latency_millis": sum(
                int(item["provider"].get("latency_millis", 0)) for item in episodes
            ),
            "wall_millis": sum(int(item.get("wall_millis", 0)) for item in episodes),
        },
        "errors": all_errors,
        "passed": not all_errors,
    }


def load_scores(path: Path) -> dict[str, Any]:
    scores = read_json(path)
    if scores.get("known_development_only") is not True:
        raise EvaluationError("scores must state known_development_only=true")
    if scores.get("opaque_average_computed") is not False:
        raise EvaluationError("opaque averages are prohibited")
    recommendation = scores.get("recommended_next_experiment")
    if (
        not isinstance(recommendation, str)
        or not recommendation.strip()
        or "\n" in recommendation.strip()
    ):
        raise EvaluationError("scores require exactly one single-line recommended_next_experiment")
    raw_episodes = scores.get("episodes")
    if not isinstance(raw_episodes, list):
        raise EvaluationError("scores.episodes must be a list")
    by_id: dict[str, dict[str, Any]] = {}
    for raw in raw_episodes:
        if not isinstance(raw, dict) or not isinstance(raw.get("id"), str):
            raise EvaluationError("every score entry requires an id")
        episode_id = raw["id"]
        if episode_id in by_id:
            raise EvaluationError(f"duplicate score entry: {episode_id}")
        by_id[episode_id] = raw
    if set(by_id) != set(EPISODE_BY_ID):
        missing = sorted(set(EPISODE_BY_ID) - set(by_id))
        extra = sorted(set(by_id) - set(EPISODE_BY_ID))
        raise EvaluationError(f"score episode set mismatch; missing={missing}, extra={extra}")

    for episode in EPISODES:
        raw = by_id[episode.episode_id]
        if raw.get("evaluation_scope") != episode.evaluation_scope:
            raise EvaluationError(f"{episode.episode_id}: evaluation_scope mismatch")
        if not isinstance(raw.get("decisive_key_roles_present"), bool):
            raise EvaluationError(f"{episode.episode_id}: decisive_key_roles_present must be boolean")
        if not isinstance(raw.get("useful"), bool):
            raise EvaluationError(f"{episode.episode_id}: useful must be boolean")
        gold_stage = raw.get("gold_loss_stage")
        if gold_stage not in GOLD_LOSS_STAGES:
            raise EvaluationError(f"{episode.episode_id}: invalid gold_loss_stage {gold_stage!r}")
        for identity_name in ("gold_candidate_id", "gold_anchor_id"):
            identity = raw.get(identity_name)
            if identity is not None and (not isinstance(identity, str) or not identity):
                raise EvaluationError(f"{episode.episode_id}: {identity_name} must be null or non-empty")
        detail = raw.get("gold_loss_detail")
        if not isinstance(detail, str) or not detail.strip():
            raise EvaluationError(f"{episode.episode_id}: gold_loss_detail must be non-empty")
        for list_name in (
            "major_unsupported_claims",
            "absence_claims_from_incomplete_scope",
            "clipped_before_known_task_match_without_partial_window",
            "notes",
        ):
            values = raw.get(list_name)
            if not isinstance(values, list) or not all(isinstance(item, str) for item in values):
                raise EvaluationError(f"{episode.episode_id}: {list_name} must be a string list")
        episode_scores = raw.get("scores")
        if not isinstance(episode_scores, dict):
            raise EvaluationError(f"{episode.episode_id}: scores must be an object")
        if episode.evaluation_scope == "primary_regression":
            if set(episode_scores) != set(SCORE_DIMENSIONS):
                raise EvaluationError(
                    f"{episode.episode_id}: scores must contain exactly {', '.join(SCORE_DIMENSIONS)}"
                )
            for dimension, value in episode_scores.items():
                if isinstance(value, bool) or not isinstance(value, int) or not 0 <= value <= 4:
                    raise EvaluationError(f"{episode.episode_id}: {dimension} must be an integer 0-4")
        elif episode_scores:
            raise EvaluationError(f"{episode.episode_id}: cheap-exit-only scores must stay empty")
    return scores


def _is_real_cheap_exit(entry: Mapping[str, Any]) -> bool:
    provider = entry.get("provider")
    cheap_exit = entry.get("cheap_exit")
    status = entry.get("status")
    return (
        entry.get("useful") is True
        and isinstance(provider, Mapping)
        and provider.get("calls") == 0
        and isinstance(cheap_exit, Mapping)
        and cheap_exit.get("eligible") is True
        and cheap_exit.get("route") == "zero_call"
        and entry.get("offline") is False
        and isinstance(status, Mapping)
        and status.get("sufficient") is True
    )


def compute_thresholds(
    score_entries: Mapping[str, Mapping[str, Any]],
    validation_entries: Mapping[str, Mapping[str, Any]],
) -> dict[str, Any]:
    primary = [score_entries[episode_id] for episode_id in PRIMARY_IDS]
    all_entries = [score_entries[episode.episode_id] for episode in EPISODES]
    major_claims = sum(len(entry["major_unsupported_claims"]) for entry in all_entries)
    absence_violations = sum(
        len(entry["absence_claims_from_incomplete_scope"]) for entry in all_entries
    )
    clipping_violations = sum(
        len(entry["clipped_before_known_task_match_without_partial_window"]) for entry in all_entries
    )
    decisive_count = sum(bool(entry["decisive_key_roles_present"]) for entry in primary)
    must_read_count = sum(entry["scores"]["must_read_file_recall"] >= 3 for entry in primary)
    relation_count = sum(entry["scores"]["causal_decisive_relation"] >= 3 for entry in primary)
    verification_count = sum(entry["scores"]["verification_usefulness"] >= 3 for entry in primary)
    useful_zero_calls = [
        episode_id
        for episode_id in CHEAP_EXIT_IDS
        if _is_real_cheap_exit(
            {**score_entries[episode_id], **validation_entries[episode_id]}
        )
    ]
    artifact_failures = [
        episode_id for episode_id, entry in validation_entries.items() if not entry.get("artifact_valid")
    ]
    result = {
        "artifact_integrity": {
            "passed": not artifact_failures,
            "failed_episodes": artifact_failures,
        },
        "decisive_key_roles_present": {
            "count": decisive_count,
            "target": 5,
            "denominator": 6,
            "passed": decisive_count >= 5,
        },
        "must_read_file_recall_at_least_3": {
            "count": must_read_count,
            "target": 5,
            "denominator": 6,
            "passed": must_read_count >= 5,
        },
        "causal_decisive_relation_at_least_3": {
            "count": relation_count,
            "target": 4,
            "denominator": 6,
            "passed": relation_count >= 4,
        },
        "verification_usefulness_at_least_3": {
            "count": verification_count,
            "target": 4,
            "denominator": 6,
            "passed": verification_count >= 4,
        },
        "major_unsupported_claims": {
            "count": major_claims,
            "target": 0,
            "passed": major_claims == 0,
        },
        "useful_zero_call_packs": {
            "count": len(useful_zero_calls),
            "target": 2,
            "denominator": 3,
            "episodes": useful_zero_calls,
            "passed": len(useful_zero_calls) >= 2,
        },
        "clipped_before_known_match_without_partial_window": {
            "count": clipping_violations,
            "target": 0,
            "passed": clipping_violations == 0,
        },
        "absence_claim_from_incomplete_scope": {
            "count": absence_violations,
            "target": 0,
            "passed": absence_violations == 0,
        },
    }
    return result


def _baseline_scores(root: Path) -> dict[str, dict[str, Any]]:
    scorecard = read_json(root / "baseline" / "evaluation" / "SCORECARD.json")
    raw_episodes = scorecard.get("episodes")
    if not isinstance(raw_episodes, list):
        raise EvaluationError("baseline SCORECARD.json has no episodes")
    result: dict[str, dict[str, Any]] = {}
    for raw in raw_episodes:
        if isinstance(raw, dict) and raw.get("id") in PRIMARY_IDS:
            result[raw["id"]] = raw
    if set(result) != set(PRIMARY_IDS):
        raise EvaluationError("baseline scorecard does not contain all six primary episodes")
    return result


def _baseline_metrics(root: Path) -> dict[str, dict[str, Any]]:
    result: dict[str, dict[str, Any]] = {}
    for episode in PRIMARY_EPISODES:
        metrics = read_json(
            root / "baseline" / "holdout" / episode.episode_id / "attempt" / "METRICS.json"
        )
        provider = metrics.get("provider")
        if not isinstance(provider, dict):
            raise EvaluationError(f"baseline metrics missing provider for {episode.episode_id}")
        result[episode.episode_id] = {
            "provider": provider,
            "wall_millis": metrics.get("wall_millis", 0),
        }
    return result


def build_gold_loss_ledger(
    score_entries: Mapping[str, Mapping[str, Any]],
    validation_entries: Mapping[str, Mapping[str, Any]],
) -> dict[str, Any]:
    entries: list[dict[str, Any]] = []
    for spec in EPISODES:
        episode_id = spec.episode_id
        score = score_entries[episode_id]
        artifact = validation_entries[episode_id]
        trace = artifact.get("trace")
        if not isinstance(trace, dict):
            raise EvaluationError(f"{episode_id}: validated raw trace metadata is missing")
        candidate_ids = set(trace.get("candidate_ids", []))
        selected_anchor_ids = set(trace.get("selected_anchor_ids", []))
        candidate_id = score.get("gold_candidate_id")
        anchor_id = score.get("gold_anchor_id")
        disposition = score["gold_loss_stage"]
        if candidate_id is not None and candidate_id not in candidate_ids:
            raise EvaluationError(f"{episode_id}: gold_candidate_id is absent from the raw trace")
        if anchor_id is not None and anchor_id not in selected_anchor_ids:
            raise EvaluationError(f"{episode_id}: gold_anchor_id is absent from selected raw anchors")
        if disposition in {"present_before_ranking", "dropped_during_ranking"} and candidate_id is None:
            raise EvaluationError(f"{episode_id}: {disposition} requires gold_candidate_id")
        if disposition == "dropped_during_ranking" and anchor_id is not None:
            raise EvaluationError(f"{episode_id}: dropped gold cannot name a selected gold_anchor_id")
        if disposition == "clipped_during_source_retention" and (
            candidate_id is None or anchor_id is None
        ):
            raise EvaluationError(
                f"{episode_id}: clipped source retention requires candidate and selected anchor IDs"
            )
        if disposition == "never_generated" and (candidate_id is not None or anchor_id is not None):
            raise EvaluationError(f"{episode_id}: never-generated gold cannot claim a raw trace identity")
        raw_sha = trace.get("sha256")
        raw_path = trace.get("json_path")
        if not isinstance(raw_sha, str) or not isinstance(raw_path, str):
            raise EvaluationError(f"{episode_id}: raw trace path/hash is missing")
        entries.append(
            {
                "episode_id": episode_id,
                "evaluation_scope": spec.evaluation_scope,
                "classification_source": "development_gold",
                "disposition": disposition,
                "candidate_id": candidate_id,
                "anchor_id": anchor_id,
                "detail": score["gold_loss_detail"].strip(),
                "raw_retrieval_trace_path": raw_path,
                "raw_retrieval_trace_sha256": raw_sha,
                "production_trace_mutated": False,
            }
        )
    return {
        "version": 1,
        "kind": "task_lens_v01_evaluation_gold_loss_ledger",
        "created_at": utc_now(),
        "classification_source": "development_gold",
        "production_trace_mutated": False,
        "entries": entries,
    }


def _evaluation_seal(evaluation_dir: Path, names: Sequence[str]) -> dict[str, Any]:
    artifacts = []
    for name in names:
        path = evaluation_dir / name
        artifacts.append(
            {
                "path": name,
                "bytes": path.stat().st_size,
                "sha256": sha256_file(path),
            }
        )
    return {
        "version": 1,
        "kind": "task_lens_v01_evaluation_seal",
        "sealed_at": utc_now(),
        "state": "sealed_known_development_evaluation",
        "artifacts": artifacts,
    }


def evaluate(root: Path, scores_path: Path) -> dict[str, Any]:
    validation = validate_artifacts(root)
    if not validation["passed"]:
        first = validation["errors"][0] if validation["errors"] else "unknown validation failure"
        raise EvaluationError(f"artifact validation failed ({len(validation['errors'])} errors): {first}")
    scores = load_scores(scores_path)
    score_by_id = {entry["id"]: entry for entry in scores["episodes"]}
    validation_by_id = {entry["id"]: entry for entry in validation["episodes"]}
    gold_loss_ledger = build_gold_loss_ledger(score_by_id, validation_by_id)
    evaluation_dir = root / "evaluation-v01"
    validation_path = evaluation_dir / "ARTIFACT_VALIDATION.json"
    ledger_path = evaluation_dir / "GOLD_LOSS_LEDGER.json"
    write_json(validation_path, validation)
    write_json(ledger_path, gold_loss_ledger)
    ledger_sha256 = sha256_file(ledger_path)

    thresholds = compute_thresholds(score_by_id, validation_by_id)
    passed = all(value["passed"] for value in thresholds.values())
    baseline_scores = _baseline_scores(root)
    baseline_metrics = _baseline_metrics(root)
    episodes: list[dict[str, Any]] = []
    for spec in EPISODES:
        score = score_by_id[spec.episode_id]
        artifact = validation_by_id[spec.episode_id]
        baseline = baseline_scores.get(spec.episode_id)
        episodes.append(
            {
                **score,
                "provider": artifact["provider"],
                "wall_millis": artifact["wall_millis"],
                "offline": artifact["offline"],
                "status": artifact["status"],
                "cheap_exit": artifact["cheap_exit"],
                "trace": artifact["trace"],
                "attempt_dir": artifact["attempt_dir"],
                "run_dir": artifact["run_dir"],
                "artifact_warnings": artifact["warnings"],
                "negative_claims_detected": artifact["negative_claims_detected"],
                "baseline_scores": baseline.get("scores") if baseline else None,
                "baseline_major_unsupported_claims": baseline.get("major_unsupported_claims") if baseline else None,
                "baseline_gold_comparison": baseline.get("gold_comparison") if baseline else None,
                "baseline_missing_anchors": baseline.get("important_missing_anchors") if baseline else None,
                "baseline_irrelevant_anchors": baseline.get("irrelevant_anchors") if baseline else None,
                "baseline_failure_notes": baseline.get("failure_notes") if baseline else None,
                "baseline_metrics": baseline_metrics.get(spec.episode_id),
            }
        )

    scorecard = {
        "version": 1,
        "kind": "task_lens_v01_development_scorecard",
        "rendered_at": utc_now(),
        "known_development_only": True,
        "fresh_generalization": False,
        "opaque_average_computed": False,
        "primary_episode_ids": list(PRIMARY_IDS),
        "cheap_exit_only_episode_ids": [episode.episode_id for episode in CALIBRATION_EPISODES],
        "dimensions": list(SCORE_DIMENSIONS),
        "episodes": episodes,
        "thresholds": thresholds,
        "gold_loss_ledger_sha256": ledger_sha256,
        "development_target_passed": passed,
        "authorization": (
            "Passing authorizes one new untouched holdout only; it does not authorize product integration."
            if passed
            else "The known-development target did not pass; product integration and a new holdout are not authorized."
        ),
        "recommended_next_experiment": scores["recommended_next_experiment"].strip(),
    }
    scorecard_path = evaluation_dir / "SCORECARD.json"
    write_json(scorecard_path, scorecard)
    scorecard_digest = sha256_file(scorecard_path)
    result = {
        "version": 1,
        "kind": "task_lens_v01_development_evaluation",
        "evaluated_at": utc_now(),
        "outcome": "passed" if passed else "partial",
        "known_development_only": True,
        "fresh_generalization": False,
        "product_integration_authorized": False,
        "new_untouched_holdout_authorized": passed,
        "opaque_average_computed": False,
        "episodes_evaluated": len(PRIMARY_EPISODES),
        "cheap_exit_calibrations": len(CALIBRATION_EPISODES),
        "resource_totals": validation["resource_totals"],
        "thresholds": thresholds,
        "scorecard_sha256": scorecard_digest,
        "artifact_validation_sha256": sha256_file(validation_path),
        "gold_loss_ledger_sha256": ledger_sha256,
        "production_trace_mutated_with_gold": False,
        "recommended_next_experiment": scores["recommended_next_experiment"].strip(),
    }
    result_path = evaluation_dir / "EVALUATION_RESULT.json"
    write_json(result_path, result)
    sealed_names = (
        "ARTIFACT_VALIDATION.json",
        "GOLD_LOSS_LEDGER.json",
        "SCORECARD.json",
        "EVALUATION_RESULT.json",
    )
    evaluation_seal = _evaluation_seal(evaluation_dir, sealed_names)
    evaluation_seal_path = evaluation_dir / "EVALUATION_SEAL.json"
    write_json(evaluation_seal_path, evaluation_seal)
    write_text(
        evaluation_dir / "EVALUATION_SEAL.sha256",
        f"{sha256_file(evaluation_seal_path)}  EVALUATION_SEAL.json",
    )
    return {
        "validation": validation,
        "gold_loss_ledger": gold_loss_ledger,
        "scorecard": scorecard,
        "result": result,
        "evaluation_seal": evaluation_seal,
    }


def _markdown_table(headers: Sequence[str], rows: Sequence[Sequence[Any]]) -> str:
    def clean(value: Any) -> str:
        return str(value).replace("|", "\\|").replace("\n", " ")

    lines = ["| " + " | ".join(map(clean, headers)) + " |"]
    lines.append("| " + " | ".join("---" for _ in headers) + " |")
    lines.extend("| " + " | ".join(clean(value) for value in row) + " |" for row in rows)
    return "\n".join(lines)


def _pass(value: bool) -> str:
    return "PASS" if value else "FAIL"


def verify_evaluation_seal(root: Path) -> dict[str, Any]:
    evaluation_dir = root / "evaluation-v01"
    seal_path = evaluation_dir / "EVALUATION_SEAL.json"
    sidecar_path = evaluation_dir / "EVALUATION_SEAL.sha256"
    try:
        sidecar = sidecar_path.read_text(encoding="utf-8").strip()
    except OSError as exc:
        raise EvaluationError(f"cannot read evaluation seal sidecar: {exc}") from exc
    expected_sidecar = f"{sha256_file(seal_path)}  EVALUATION_SEAL.json"
    if sidecar != expected_sidecar:
        raise EvaluationError("EVALUATION_SEAL.sha256 does not bind EVALUATION_SEAL.json")
    seal = read_json(seal_path)
    artifacts = seal.get("artifacts")
    if not isinstance(artifacts, list):
        raise EvaluationError("EVALUATION_SEAL.json has no artifact list")
    expected_names = {
        "ARTIFACT_VALIDATION.json",
        "GOLD_LOSS_LEDGER.json",
        "SCORECARD.json",
        "EVALUATION_RESULT.json",
    }
    by_name: dict[str, Mapping[str, Any]] = {}
    for entry in artifacts:
        if not isinstance(entry, dict) or not isinstance(entry.get("path"), str):
            raise EvaluationError("EVALUATION_SEAL.json has an invalid entry")
        name = entry["path"]
        if name in by_name:
            raise EvaluationError(f"duplicate evaluation seal entry: {name}")
        by_name[name] = entry
    if set(by_name) != expected_names:
        raise EvaluationError("evaluation seal artifact set is incomplete")
    for name, entry in by_name.items():
        path = evaluation_dir / name
        if sha256_file(path) != entry.get("sha256") or path.stat().st_size != entry.get("bytes"):
            raise EvaluationError(f"evaluation seal mismatch for {name}")

    result = read_json(evaluation_dir / "EVALUATION_RESULT.json")
    scorecard_path = evaluation_dir / "SCORECARD.json"
    validation_path = evaluation_dir / "ARTIFACT_VALIDATION.json"
    ledger_path = evaluation_dir / "GOLD_LOSS_LEDGER.json"
    if result.get("scorecard_sha256") != sha256_file(scorecard_path):
        raise EvaluationError("EVALUATION_RESULT.json does not bind SCORECARD.json")
    if result.get("artifact_validation_sha256") != sha256_file(validation_path):
        raise EvaluationError("EVALUATION_RESULT.json does not bind ARTIFACT_VALIDATION.json")
    if result.get("gold_loss_ledger_sha256") != sha256_file(ledger_path):
        raise EvaluationError("EVALUATION_RESULT.json does not bind GOLD_LOSS_LEDGER.json")
    scorecard = read_json(scorecard_path)
    if scorecard.get("gold_loss_ledger_sha256") != sha256_file(ledger_path):
        raise EvaluationError("SCORECARD.json does not bind GOLD_LOSS_LEDGER.json")
    ledger = read_json(ledger_path)
    if ledger.get("production_trace_mutated") is not False:
        raise EvaluationError("gold-loss ledger does not preserve immutable production traces")
    entries = ledger.get("entries")
    if not isinstance(entries, list) or len(entries) != len(EPISODES):
        raise EvaluationError("gold-loss ledger episode count mismatch")
    for entry in entries:
        if not isinstance(entry, dict) or entry.get("production_trace_mutated") is not False:
            raise EvaluationError("invalid gold-loss ledger entry")
        raw_path = entry.get("raw_retrieval_trace_path")
        raw_sha = entry.get("raw_retrieval_trace_sha256")
        if not isinstance(raw_path, str) or not isinstance(raw_sha, str):
            raise EvaluationError("gold-loss ledger entry lacks raw trace binding")
        target = root / _safe_relative_path(raw_path, ledger_path)
        if sha256_file(target) != raw_sha:
            raise EvaluationError(f"gold-loss ledger raw trace mismatch: {raw_path}")
        if read_json(target).get("gold_assessment") is not None:
            raise EvaluationError(f"production trace contains evaluation gold: {raw_path}")
    return {"seal": seal, "result": result, "scorecard": scorecard, "ledger": ledger}


def _episode_report_artifacts(root: Path, episode: Mapping[str, Any]) -> dict[str, Any]:
    run_dir = root / str(episode["run_dir"])
    attempt_dir = root / str(episode["attempt_dir"])
    bundle = read_json(run_dir / QUARTET[0])
    pack = read_json(run_dir / QUARTET[2])
    trace = read_json(run_dir / "retrieval_trace.json")
    metrics = read_json(attempt_dir / "METRICS.json")
    anchors = bundle.get("anchors") if isinstance(bundle.get("anchors"), list) else []
    labels = {}
    for anchor in anchors:
        if not isinstance(anchor, dict) or not isinstance(anchor.get("id"), str):
            continue
        location = str(anchor.get("path", "<unknown>"))
        symbol = anchor.get("symbol")
        if isinstance(symbol, str) and symbol:
            location += ":" + symbol
        labels[anchor["id"]] = location
    relation_id = pack.get("decisive_relation_id")
    relationships = trace.get("relationships") if isinstance(trace.get("relationships"), list) else []
    decisive = next(
        (
            item
            for item in relationships
            if isinstance(item, dict) and item.get("id") == relation_id
        ),
        {},
    )
    left_id = decisive.get("left_candidate_id")
    right_id = decisive.get("right_candidate_id")
    if decisive:
        relation = (
            f"{decisive.get('kind', 'unknown')}: "
            f"{labels.get(left_id, left_id)} -> {labels.get(right_id, right_id)} "
            f"({decisive.get('support_type', 'unknown')})"
        )
    else:
        relation = "missing decisive relation"
    coverage = trace.get("role_coverage") if isinstance(trace.get("role_coverage"), dict) else {}
    key_coverage = coverage.get("key") if isinstance(coverage.get("key"), list) else []
    key_roles = [str(item.get("role")) for item in key_coverage if isinstance(item, dict)]
    missing_roles = [
        str(item.get("role"))
        for item in key_coverage
        if isinstance(item, dict) and item.get("represented") is not True
    ]
    contract = bundle.get("role_contract") if isinstance(bundle.get("role_contract"), dict) else {}
    contract_tiers = []
    for tier in ("key", "supporting", "optional"):
        tier_items = contract.get(tier) if isinstance(contract.get(tier), list) else []
        roles = [
            str(item.get("role"))
            for item in tier_items
            if isinstance(item, dict) and item.get("role")
        ]
        contract_tiers.append(f"{tier}={','.join(roles) or 'none'}")
    frontier = pack.get("verification_frontier")
    verification: list[str] = []
    if isinstance(frontier, dict):
        for item in frontier.get("anchors", []):
            if not isinstance(item, dict):
                continue
            location = str(item.get("path", "<unknown>"))
            if item.get("symbol"):
                location += ":" + str(item["symbol"])
            verification.append(f"{item.get('authority', 'unknown')} {location}")
        for name in ("fixture", "command_or_effect"):
            item = frontier.get(name)
            if isinstance(item, dict):
                verification.append(
                    f"{item.get('authority', name)} {item.get('path') or item.get('text') or name}"
                )
    source_scopes = trace.get("source_scopes") if isinstance(trace.get("source_scopes"), list) else []
    partial_scopes = 0
    for item in source_scopes:
        scope = item.get("scope") if isinstance(item, dict) else None
        if isinstance(scope, dict) and scope.get("scope_kind") in {"matched_fragments", "partial_window"}:
            partial_scopes += 1
    limits = trace.get("limits") if isinstance(trace.get("limits"), list) else []
    bound_limits = [
        str(item.get("name"))
        for item in limits
        if isinstance(item, dict) and item.get("applied") is True
    ]
    loss_limits = [
        str(item.get("name"))
        for item in limits
        if isinstance(item, dict) and item.get("caused_loss") is True
    ]
    budgets = trace.get("budgets") if isinstance(trace.get("budgets"), dict) else {}
    verify = pack.get("verify") if isinstance(pack.get("verify"), dict) else {}
    hypotheses = pack.get("working_hypothesis") if isinstance(pack.get("working_hypothesis"), list) else []
    task = bundle.get("task") if isinstance(bundle.get("task"), dict) else {}
    return {
        "bundle": bundle,
        "pack": pack,
        "trace": trace,
        "metrics": metrics,
        "labels": labels,
        "relation": relation,
        "relation_non_guarantees": str(decisive.get("non_guarantees", "")),
        "key_roles": key_roles,
        "missing_roles": missing_roles,
        "role_contract": "; ".join(contract_tiers),
        "verification": verification,
        "partial_scopes": partial_scopes,
        "scope_count": len(source_scopes),
        "bound_limits": bound_limits,
        "loss_limits": loss_limits,
        "completion_expansions": int(budgets.get("frontier_expansions", 0)),
        "effect": str(verify.get("effect_to_observe", "missing bounded effect")),
        "hypotheses": hypotheses,
        "task_text": str(task.get("text", "task text unavailable")),
        "task_profile": str(bundle.get("task_profile", "unknown")),
        "pack_bytes": (run_dir / QUARTET[2]).stat().st_size,
    }


def _main_blocker(result: Mapping[str, Any]) -> str:
    labels = (
        ("artifact_integrity", "sealed artifact integrity failed"),
        ("decisive_key_roles_present", "decisive key-role coverage remains below target"),
        ("must_read_file_recall_at_least_3", "must-read file recall remains below target"),
        ("causal_decisive_relation_at_least_3", "typed decisive relations remain below target"),
        ("verification_usefulness_at_least_3", "grounded verification usefulness remains below target"),
        ("major_unsupported_claims", "major unsupported claims remain"),
        ("useful_zero_call_packs", "too few useful zero-call packs qualified"),
        ("clipped_before_known_match_without_partial_window", "source clipping is not marked safely"),
        ("absence_claim_from_incomplete_scope", "absence claims escaped incomplete-source safeguards"),
    )
    thresholds = result["thresholds"]
    for key, text_value in labels:
        if not thresholds[key]["passed"]:
            return text_value
    return "fresh generalization remains unmeasured, so development success cannot justify integration"


def _validate_rendered_reports(
    root: Path,
    files: Mapping[Path, str],
    recommendation: str,
) -> None:
    rendered_names = {
        path.relative_to(root).as_posix()
        for path in files
    }
    missing = sorted(REQUIRED_RENDERED_REPORTS - rendered_names)
    if missing:
        raise EvaluationError("rendered report bundle is incomplete: " + ", ".join(missing))

    supervisor = files[root / "SUPERVISOR_REPORT.md"]
    supervisor_lines = supervisor.splitlines()
    opening = supervisor_lines[: len(SUPERVISOR_OPENING_LABELS)]
    if len(opening) != len(SUPERVISOR_OPENING_LABELS):
        raise EvaluationError("supervisor report has an incomplete opening decision block")
    for index, (line, label) in enumerate(zip(opening, SUPERVISOR_OPENING_LABELS), 1):
        if not line.startswith(label + " "):
            raise EvaluationError(
                f"supervisor report opening line {index} must start with {label!r}"
            )
    if (
        len(supervisor_lines) <= len(SUPERVISOR_OPENING_LABELS)
        or supervisor_lines[len(SUPERVISOR_OPENING_LABELS)] != ""
    ):
        raise EvaluationError("supervisor opening must be exactly ten lines followed by a blank line")
    recommendation_lines = [
        line
        for line in supervisor_lines
        if line.startswith(SUPERVISOR_OPENING_LABELS[-1] + " ")
    ]
    expected_recommendation = f"{SUPERVISOR_OPENING_LABELS[-1]} {recommendation}"
    if recommendation_lines != [expected_recommendation]:
        raise EvaluationError("supervisor report must contain exactly one recommended next step")
    for section in (
        "## What v0 got right",
        "## Root-cause audit",
        "## Source-scope contract",
        "## Completion and verification",
        "## Cheap exit",
        "## Before and after",
        "## Model/resource accounting",
        "## Product decision",
    ):
        if section not in supervisor:
            raise EvaluationError(f"supervisor report is missing section {section!r}")
    for episode in EPISODES:
        if f"### {episode.episode_id}\n" not in supervisor:
            raise EvaluationError(
                f"supervisor root-cause audit omits {episode.episode_id}"
            )
    baseline_trace_note = (
        "The preserved v0 attempts predate `retrieval_trace.json` and "
        "`retrieval_trace.md`."
    )
    if baseline_trace_note not in supervisor:
        raise EvaluationError("supervisor report omits the v0 trace provenance limit")
    if baseline_trace_note not in files[root / "RETRIEVAL_FAILURES.md"]:
        raise EvaluationError("retrieval failure report omits the v0 trace provenance limit")

    review = files[root / "review" / "index.html"]
    lowered = review.lower()
    if re.search(r"<script\b", lowered) or re.search(
        r"href\s*=\s*['\"]\s*javascript:", lowered
    ):
        raise EvaluationError("static review unexpectedly contains executable script content")
    for episode in EPISODES:
        if f'id="{episode.episode_id}"' not in review:
            raise EvaluationError(f"static review omits {episode.episode_id}")
        attempt_prefix = f"../episodes/{episode.episode_id}/final/attempt/"
        for projection in (
            "role_contract.json",
            "role_coverage.json",
            "verification_frontier.json",
            "cheap_exit_decision.json",
            "source_scopes.json",
        ):
            expected_href = f'href="{attempt_prefix}{projection}"'
            if expected_href not in review:
                raise EvaluationError(
                    f"static review omits sealed {projection} link for {episode.episode_id}"
                )
    for artifact in (
        "PLAN.md",
        "RUN_LOG.md",
        "EXPERIMENTS.jsonl",
        "SUPERVISOR_REPORT.md",
        "WALKTHROUGH.md",
        "EVALUATION_RESULT.json",
        "task.md",
        "task_investigation_bundle.json",
        "task_investigation_attempt.json",
        "task_investigation.json",
        "task_investigation_status.json",
        "retrieval_trace.json",
        "role_contract.json",
        "role_coverage.json",
        "verification_frontier.json",
        "cheap_exit_decision.json",
        "source_scopes.json",
        "METRICS.json",
    ):
        if artifact not in review:
            raise EvaluationError(f"static review has no link for {artifact}")


def render_reports(root: Path) -> dict[str, str]:
    evaluation_dir = root / "evaluation-v01"
    sealed = verify_evaluation_seal(root)
    result = sealed["result"]
    scorecard = sealed["scorecard"]
    validation = read_json(evaluation_dir / "ARTIFACT_VALIDATION.json")
    episodes = {entry["id"]: entry for entry in scorecard["episodes"]}
    details = {
        episode_id: _episode_report_artifacts(root, entry)
        for episode_id, entry in episodes.items()
    }

    regression_rows: list[list[Any]] = []
    for episode_id in PRIMARY_IDS:
        entry = episodes[episode_id]
        regression_rows.append(
            [
                episode_id,
                entry["scores"]["subsystem_localization"],
                entry["scores"]["decisive_anchor_recall"],
                entry["scores"]["key_role_coverage"],
                entry["scores"]["must_read_file_recall"],
                entry["scores"]["causal_decisive_relation"],
                entry["scores"]["verification_usefulness"],
                entry["provider"]["calls"],
                len(entry["major_unsupported_claims"]),
            ]
        )
    regression = """# Task Lens v0.1 development regression

This is a known-development regression over the six former holdout episodes. It is not a fresh-generalization result, and no opaque average is computed.

""" + _markdown_table(
        ("episode", "subsystem", "decisive", "roles", "must-read", "relation", "verify", "calls", "major claims"),
        regression_rows,
    ) + "\n\n## Thresholds\n\n" + _markdown_table(
        ("gate", "observed", "target", "result"),
        [
            (name, value.get("count", "all"), value.get("target", "pass"), _pass(value["passed"]))
            for name, value in result["thresholds"].items()
        ],
    ) + "\n"

    comparison_rows: list[list[Any]] = []
    resource_rows: list[list[Any]] = []
    for episode_id in PRIMARY_IDS:
        entry = episodes[episode_id]
        baseline = entry["baseline_scores"]
        for label, baseline_dimension, current_dimension in REPORT_COMPARISON_DIMENSIONS:
            before = baseline[baseline_dimension]
            after = entry["scores"][current_dimension]
            comparison_rows.append((episode_id, label, before, after, after - before))
        comparison_rows.append(
            (
                episode_id,
                "irrelevant anchors",
                f"not separately scored; {len(entry.get('baseline_irrelevant_anchors') or [])} named",
                entry["scores"]["irrelevant_anchor_control"],
                "n/a",
            )
        )
        baseline_resource = entry["baseline_metrics"]
        resource_rows.append(
            (
                episode_id,
                baseline_resource["provider"].get("calls", 0),
                entry["provider"].get("calls", 0),
                baseline_resource["provider"].get("input_tokens", 0),
                entry["provider"].get("input_tokens", 0),
                baseline_resource["provider"].get("output_tokens", 0),
                entry["provider"].get("output_tokens", 0),
                baseline_resource.get("wall_millis", 0),
                entry.get("wall_millis", 0),
            )
        )
    comparison = """# Task Lens v0 to v0.1 baseline comparison

The v0 artifacts are immutable and hash-verified. The comparison is artifact-level on the same known Fuego histories; it is not evidence of unseen-task or cross-repository generalization. Scores stay separate; no aggregate is calculated.

""" + _markdown_table(("episode", "dimension", "v0", "v0.1", "delta"), comparison_rows) + (
        "\n\n## Resource comparison\n\n"
        + _markdown_table(
            ("episode", "v0 calls", "v0.1 calls", "v0 input", "v0.1 input", "v0 output", "v0.1 output", "v0 wall ms", "v0.1 wall ms"),
            resource_rows,
        )
        + "\n"
    )

    cheap_rows = []
    for entry in scorecard["episodes"]:
        episode_id = entry["id"]
        detail = details[episode_id]
        baseline_metrics = entry.get("baseline_metrics") or {}
        baseline_provider = baseline_metrics.get("provider") or {}
        baseline_wall = baseline_metrics.get("wall_millis")
        baseline_input = baseline_provider.get("input_tokens")
        baseline_output = baseline_provider.get("output_tokens")
        current_input = entry["provider"].get("input_tokens", 0)
        current_output = entry["provider"].get("output_tokens", 0)
        wall_saving = (
            f"{baseline_wall} -> {entry['wall_millis']} "
            f"(saved {baseline_wall - entry['wall_millis']})"
            if isinstance(baseline_wall, int)
            else f"n/a -> {entry['wall_millis']}"
        )
        input_saving = (
            f"{baseline_input} -> {current_input} (saved {baseline_input - current_input})"
            if isinstance(baseline_input, int)
            else f"n/a -> {current_input}"
        )
        output_saving = (
            f"{baseline_output} -> {current_output} (saved {baseline_output - current_output})"
            if isinstance(baseline_output, int)
            else f"n/a -> {current_output}"
        )
        reasons = "; ".join(entry["cheap_exit"].get("reasons", []))
        if _is_real_cheap_exit(entry):
            synthesis_reason = "synthesis not needed: all deterministic local gates passed"
        elif entry["provider"]["calls"] == 1:
            synthesis_reason = "synthesis used: " + (reasons or "local evidence was incomplete")
        elif entry["offline"]:
            synthesis_reason = "synthesis needed but unavailable offline: " + (
                reasons or "local evidence was incomplete"
            )
        else:
            synthesis_reason = "synthesis not completed: " + (
                reasons or "local evidence was incomplete"
            )
        cheap_rows.append(
            (
                episode_id,
                "yes" if episode_id in CHEAP_EXIT_IDS else "no",
                entry["cheap_exit"].get("eligible", False),
                detail["relation"],
                entry["provider"]["calls"],
                entry["cheap_exit"]["route"],
                entry["status"]["sufficient"],
                entry["useful"],
                "PASS" if _is_real_cheap_exit(entry) else "MISS",
                wall_saving,
                input_saving,
                output_saving,
                synthesis_reason,
            )
        )
    cheap = """# Task Lens v0.1 cheap-exit evaluation

A real cheap exit is a useful final pack with `provider.calls=0`, a sealed `zero_call` decision, all local gates satisfied, and no forced offline skip. Eligibility follows final local evidence, not the development-target label.

""" + _markdown_table(
        (
            "episode",
            "declared target",
            "eligible",
            "exact local relation",
            "calls",
            "route",
            "sufficient",
            "useful",
            "result",
            "wall ms v0 -> v0.1",
            "input tokens v0 -> v0.1",
            "output tokens v0 -> v0.1",
            "why synthesis was or was not needed",
        ),
        cheap_rows,
    ) + "\n"

    scope_rows = []
    unmarked_clipping_count = 0
    invalid_absence_count = 0
    for entry in scorecard["episodes"]:
        detail = details[entry["id"]]
        unmarked_clipping_count += len(
            entry["clipped_before_known_task_match_without_partial_window"]
        )
        invalid_absence_count += len(entry["absence_claims_from_incomplete_scope"])
        scope_rows.append(
            (
                entry["id"],
                detail["scope_count"],
                detail["partial_scopes"],
                len(entry["clipped_before_known_task_match_without_partial_window"]),
                len(entry["absence_claims_from_incomplete_scope"]),
                entry["scores"].get("source_scope_completeness", "calibration"),
            )
        )
    release_scope_detail = details["multi_module_release_script"]
    release_scope_result = (
        "all selected release scopes are complete"
        if release_scope_detail["partial_scopes"] == 0
        else (
            f"{release_scope_detail['partial_scopes']} selected release scope(s) are explicitly "
            "partial and cannot support absence claims"
        )
    )
    scope = """# Task Lens v0.1 source-scope audit

Every selected anchor carries a SourceScope record. Complete Go symbols are retained when bounded; operational, documentation, and configuration files at or below 64 KiB are retained as complete files. Fragment and partial-window scopes prohibit absence claims.

""" + _markdown_table(
        ("episode", "scopes", "partial", "unmarked clipping", "invalid absence claims", "score"),
        scope_rows,
    ) + """

## Contract result

- Complete versus partial behavior is explicit in every sealed trace.
- Multi-module release result: %s.
- Known-match clipping without an explicit partial window: %d.
- Absence claims attributed to incomplete scope: %d.
- Exact bounded searches and deterministic manifests remain valid negative-evidence authorities.
""" % (release_scope_result, unmarked_clipping_count, invalid_absence_count)

    failure_sections: list[str] = []
    for episode_id, entry in episodes.items():
        detail = details[episode_id]
        baseline_loss = entry.get("baseline_gold_comparison") or "No v0 baseline exists for this calibration episode."
        failure_sections.append(
            f"""## {episode_id}

- v0 loss: {baseline_loss}
- v0.1 gold classification: `{entry['gold_loss_stage']}` — {entry['gold_loss_detail']}
- Missing key role after local completion: {', '.join(detail['missing_roles']) or 'none'}
- Completion expansions: {detail['completion_expansions']}/2; bound limits: {', '.join(detail['bound_limits']) or 'none'}; limits that caused loss: {', '.join(detail['loss_limits']) or 'none'}
- Source-scope failure: {'partial scopes remain, but are explicitly marked' if detail['partial_scopes'] else 'none; all selected scopes are complete'}
- Verification failure/gap: {'; '.join(detail['verification']) or 'missing evidence'}; effect: {detail['effect']}
- Model versus local failure: route `{entry['cheap_exit']['route']}`, calls {entry['provider']['calls']}, offline {str(entry['offline']).lower()}; {('; '.join(entry['notes'])) or 'no additional scored failure note'}
"""
        )
    failures = """# Task Lens v0.1 retrieval failures

The preserved v0 attempts predate `retrieval_trace.json` and `retrieval_trace.md`. Their `report.json` and `run_manifest.json` are the available v0 provenance; the v0 loss labels below are post-run development-gold classifications, not reconstructed production traces.

Gold is used only in this post-run ledger to classify where the known decisive anchor was generated, ranked, dropped, never generated, or clipped. Gold is not a production ranking input, and the raw trace hashes are sealed before classification.

""" + "\n".join(failure_sections)

    outcome = result["outcome"]
    decisive_gate = result["thresholds"]["decisive_key_roles_present"]
    verification_gate = result["thresholds"]["verification_usefulness_at_least_3"]
    zero_gate = result["thresholds"]["useful_zero_call_packs"]
    technical_result = (
        "passed: the frozen binary, seven one-shot attempts, artifact quartet, traces, projections, and SHA-256 seals validated"
        if result["thresholds"]["artifact_integrity"]["passed"]
        else "failed: at least one frozen attempt or sealed artifact did not validate"
    )
    product_result = (
        "passed for known development only; fresh generalization and product integration remain unmeasured"
        if outcome == "passed"
        else "partial: one or more known-development targets remain below the declared threshold"
    )
    investment_result = (
        "authorize one untouched cross-repository holdout only; do not integrate the product surface"
        if result["new_untouched_holdout_authorized"]
        else "do not integrate or start a new holdout; keep work bounded to the failed development gate"
    )
    main_blocker = _main_blocker(result)

    completion_rows = []
    accounting_rows = []
    for episode_id, entry in episodes.items():
        detail = details[episode_id]
        completion_rows.append(
            (
                episode_id,
                detail["task_profile"],
                detail["role_contract"],
                ", ".join(detail["missing_roles"]) or "none",
                detail["completion_expansions"],
                detail["relation"],
                ("; ".join(detail["verification"]) or "missing evidence")
                + f"; effect: {detail['effect']}",
                entry["status"]["sufficient"],
                len(entry["major_unsupported_claims"]),
            )
        )
        provider = entry["provider"]
        local = detail["bundle"].get("metrics", {})
        accounting_rows.append(
            (
                episode_id,
                provider.get("calls", 0),
                provider.get("request_bytes", 0),
                provider.get("response_bytes", 0),
                provider.get("input_tokens", 0),
                provider.get("output_tokens", 0),
                f"{provider.get('prompt_cache_hit_tokens', 0)}/{provider.get('prompt_cache_miss_tokens', 0)}",
                provider.get("latency_millis", 0),
                entry["wall_millis"],
                f"grep={local.get('git_grep_queries', 0)}, AST={local.get('ast_parses', 0)}, gopls={detail['bundle'].get('budgets', {}).get('gopls_queries', 0)}",
                ", ".join(detail["bound_limits"]) or "none",
            )
        )

    failure_cause_rows = []
    failed_classes: list[str] = []
    for episode_id in PRIMARY_IDS:
        entry = episodes[episode_id]
        scores = entry["scores"]
        causes = []
        if (
            not entry["decisive_key_roles_present"]
            or scores["decisive_anchor_recall"] < 3
            or scores["key_role_coverage"] < 3
            or scores["must_read_file_recall"] < 3
        ):
            causes.append("retrieval")
        if scores["causal_decisive_relation"] < 3:
            causes.append("relation semantics")
        if scores["verification_usefulness"] < 3:
            causes.append("missing verification/runtime evidence")
        if causes:
            task_profile = details[episode_id]["task_profile"]
            failed_classes.append(task_profile)
            failure_cause_rows.append((task_profile, episode_id, ", ".join(causes)))
    failed_classes = list(dict.fromkeys(failed_classes))
    failure_cause_summary = (
        ", ".join(failed_classes)
        if failed_classes
        else "None in this known set; untouched generalization remains unknown."
    )
    root_cause_audit = "\n".join(
        section.replace("## ", "### ", 1)
        for section in failure_sections
    )
    supervisor = f"""outcome: {outcome}
technical result: {technical_result}
product-development result: {product_result}
investment result: {investment_result}
episodes evaluated: 6 known development regressions plus 1 cheap-exit calibration
decisive-anchor target result: {_pass(decisive_gate['passed']).lower()} ({decisive_gate['count']}/6; target 5)
verification target result: {_pass(verification_gate['passed']).lower()} ({verification_gate['count']}/6; target 4)
zero-call target result: {_pass(zero_gate['passed']).lower()} ({zero_gate['count']}/3 useful target packs; target 2)
single main blocker: {main_blocker}
exactly one recommended next step: {result['recommended_next_experiment']}

# Task Lens v0.1 supervisor report

This is a known-development regression. It does not measure fresh generalization and never authorizes default-product integration. Production traces were not mutated with gold; the post-run gold ledger is bound by `{result['gold_loss_ledger_sha256']}`.

The preserved v0 attempts predate `retrieval_trace.json` and `retrieval_trace.md`. Their `report.json` and `run_manifest.json` are the available v0 provenance; every v0 loss label in this report is a post-run development-gold classification, not a reconstructed production trace.

## What v0 got right

- Harness integrity: exact detached revisions, clean trees, frozen binary identity, artifact quartets, replay, and seals were already strong.
- Cost reduction: v0 reduced the six task-conditioned runs from 102 generic model calls to 6.
- Localization: five of six v0 episodes reached the right subsystem even when the decisive cross-file relation was incomplete.
- Epistemic separation: local observations, model hypotheses, missing evidence, and non-guarantees stayed distinct.
- Optional report projection: Task Lens remained opt-in and did not alter the default `./repomap` product.

## Root-cause audit

The same immutable post-run classifications are also available in `RETRIEVAL_FAILURES.md`.

{root_cause_audit}

## Source-scope contract

{_markdown_table(('episode', 'scopes', 'partial', 'unmarked clipping', 'invalid absence claims', 'score'), scope_rows)}

The contract retains complete Go functions and bounded operational/document/configuration files when they fit. Oversized selections become explicit partial scopes, retain task-matching neighborhoods, and cannot authorize absence claims. Observed unmarked clipping: {unmarked_clipping_count}; observed incomplete-scope absence claims: {invalid_absence_count}; multi-module release: {release_scope_result}.

## Completion and verification

{_markdown_table(('episode', 'task profile', 'role contract', 'missing key roles', 'expansions', 'decisive relation', 'verification anchor/effect', 'sufficient', 'major unsupported'), completion_rows)}

Every completion pass is capped at two actual expansions; the separate verification probe returns no more than two exact anchors. Relation support remains syntactic/documentary and preserves its non-guarantees.

## Cheap exit

{_markdown_table(('episode', 'declared target', 'eligible', 'exact local relation', 'calls', 'route', 'sufficient', 'useful', 'result', 'wall ms v0 -> v0.1', 'input tokens v0 -> v0.1', 'output tokens v0 -> v0.1', 'why synthesis was or was not needed'), cheap_rows)}

Zero-call packs are fixed presentations of exact local facts. Offline partials, where present, remain explicitly incomplete and are not relabeled as successful cheap exits. The table includes every episode so both actual eligibility and declared-target accounting are visible.

## Before and after

{_markdown_table(('episode', 'dimension', 'v0', 'v0.1', 'delta'), comparison_rows)}

Scores are separate 0–4 judgments. No opaque average is computed; irrelevant-anchor control is reported separately because v0 did not score that dimension directly.

## Model/resource accounting

{_markdown_table(('episode', 'calls', 'request B', 'response B', 'input', 'output', 'cache hit/miss', 'provider ms', 'wall ms', 'local actions', 'bound/truncation events'), accounting_rows)}

The final run executed no target-repository commands or tests. It used the v0 provider/model identity and a maximum of one process and one allowed synthesis call per episode; actual calls are shown above.

## Product decision

- Is the primitive good enough for a new untouched holdout? **{'Yes, for one holdout only.' if result['new_untouched_holdout_authorized'] else 'No.'}**
- Which task classes still fail? {failure_cause_summary}
- Are failures caused by retrieval, relation semantics, or missing runtime evidence?

{_markdown_table(('task class', 'episode', 'observed failure category'), failure_cause_rows) if failure_cause_rows else 'No known-development class fell below the retrieval, relation, or verification thresholds; missing runtime evidence may still appear on an untouched holdout.'}

Static syntax is never upgraded to runtime causality.

- Is zero-call local investigation viable? **{'Yes for the declared bounded classes.' if zero_gate['passed'] else 'Promising but below the declared two-of-three target.'}**
- Should the next experiment be a holdout, another redesign, or stop? The single selected action is recorded in the opening decision block.

Passing authorizes only one new untouched holdout. It does not authorize product integration.
"""

    product = f"""# Task Lens v0.1 product findings

This is a known-development result with outcome **{outcome}**. Fresh task and cross-repository generalization were not measured, and integration into the default product is not authorized.

## Demonstrated value

- The retriever now makes complete-source authority, task-role coverage, decisive typed relations, and verification authority inspectable before synthesis.
- Deterministic zero-call investigation is {'viable on at least two declared targets' if zero_gate['passed'] else 'not yet reliable enough on the declared targets'}.
- The final accounting is sealed at {result['resource_totals']['model_calls']} model calls, {result['resource_totals']['input_tokens']} input tokens, {result['resource_totals']['output_tokens']} output tokens, and {result['resource_totals']['wall_millis']} ms summed wall time.

## Remaining limits

- Classes below relation or verification score 3: {', '.join(failed_classes) if failed_classes else 'none in the known development set'}.
- Static local relations prove retained syntax or documentation only, not runtime reachability or ordering.
- Offline partials expose missing synthesis honestly; they are not evidence that a one-call narrative would have passed.
- The experiment provides no fresh-generalization evidence.

## Decision

New untouched holdout authorized: **{'yes' if result['new_untouched_holdout_authorized'] else 'no'}**. Default product integration authorized: **no**. The opening supervisor decision contains the single next action.
"""

    walkthrough = f"""# Task Lens v0.1 review walkthrough

1. Start at `review/index.html` and read the development-only notice and decision gates.
2. Open an episode card. It answers where v0 lost the decisive evidence, what v0.1 recovered, whether the relation is locally supported, what verification anchor exists, and whether a model call remained necessary.
3. Follow the sealed `retrieval_trace.json` and final pack links for raw evidence; use the baseline attempt link for the immutable v0 comparison.
4. Read `SUPERVISOR_REPORT.md` for the investment boundary and `BASELINE_COMPARISON.md` for separate scores and resources.

Serve locally with:

```sh
python3 -m http.server 8768 --bind 127.0.0.1 --directory {root}
```

Then open `http://127.0.0.1:8768/review/`.

Outcome: **{outcome}**. Known development only; no product integration is authorized.
"""

    files = {
        root / "DEV_REGRESSION.md": regression,
        root / "BASELINE_COMPARISON.md": comparison,
        root / "CHEAP_EXIT_EVALUATION.md": cheap,
        root / "SOURCE_SCOPE_AUDIT.md": scope,
        root / "RETRIEVAL_FAILURES.md": failures,
        root / "SUPERVISOR_REPORT.md": supervisor,
        root / "PRODUCT_FINDINGS.md": product,
        root / "WALKTHROUGH.md": walkthrough,
    }
    review_path = root / "review" / "index.html"
    review_html = _render_review_html(root, result, scorecard, validation, details)
    files[review_path] = review_html
    _validate_rendered_reports(root, files, result["recommended_next_experiment"])
    for path, content in files.items():
        write_text(path, content)
    return {path.name if path.parent == root else path.relative_to(root).as_posix(): str(path) for path in files}


def _render_review_html(
    root: Path,
    result: Mapping[str, Any],
    scorecard: Mapping[str, Any],
    validation: Mapping[str, Any],
    details: Mapping[str, Mapping[str, Any]],
) -> str:
    artifact_links = "".join(
        f'<a href="../{html.escape(path)}">{html.escape(label)}</a>'
        for path, label in (
            ("PLAN.md", "plan"),
            ("RUN_LOG.md", "run log"),
            ("EXPERIMENTS.jsonl", "experiments"),
            ("SUPERVISOR_REPORT.md", "supervisor report"),
            ("DEV_REGRESSION.md", "development scores"),
            ("BASELINE_COMPARISON.md", "baseline comparison"),
            ("RETRIEVAL_FAILURES.md", "retrieval failures"),
            ("SOURCE_SCOPE_AUDIT.md", "source-scope audit"),
            ("CHEAP_EXIT_EVALUATION.md", "cheap-exit evaluation"),
            ("PRODUCT_FINDINGS.md", "product findings"),
            ("WALKTHROUGH.md", "walkthrough"),
            ("evaluation-v01/EVALUATION_RESULT.json", "evaluation result"),
            ("evaluation-v01/SCORECARD.json", "scorecard"),
            ("baseline/", "immutable baseline"),
            ("episodes/", "episode artifacts"),
            ("screenshots/", "screenshots"),
        )
    )
    gate_rows = "".join(
        "<tr>"
        f"<td>{html.escape(name.replace('_', ' '))}</td>"
        f"<td>{html.escape(str(value.get('count', 'all')))}</td>"
        f"<td>{html.escape(str(value.get('target', 'pass')))}</td>"
        f"<td><span class=\"badge {'pass' if value['passed'] else 'fail'}\">{_pass(value['passed'])}</span></td>"
        "</tr>"
        for name, value in result["thresholds"].items()
    )
    cards: list[str] = []
    for episode in scorecard["episodes"]:
        episode_id = episode["id"]
        detail = details[episode_id]
        scores = episode["scores"]
        if not scores:
            score_cells = '<span class="muted">calibration only; no v0 score comparison</span>'
        else:
            baseline_scores = episode["baseline_scores"]
            score_cells = "".join(
                f"<div><b>{html.escape(label)}</b>"
                f"<span>{baseline_scores[baseline_dimension]}/4 → "
                f"{scores[current_dimension]}/4</span></div>"
                for label, baseline_dimension, current_dimension in REPORT_COMPARISON_DIMENSIONS
            )
            score_cells += (
                "<div><b>irrelevant anchors</b>"
                f"<span>n/a → {scores['irrelevant_anchor_control']}/4</span></div>"
            )
        baseline_bytes: int | None = None
        baseline_links = '<span class="muted">no v0 attempt</span>'
        if episode_id in PRIMARY_IDS:
            baseline_attempt = root / "baseline" / "holdout" / episode_id / "attempt"
            baseline_run = _discover_run_dir(baseline_attempt)
            baseline_bytes = (baseline_run / QUARTET[2]).stat().st_size
            baseline_prefix = "../" + baseline_run.relative_to(root).as_posix() + "/"
            baseline_links = "".join(
                (
                    f'<a href="{html.escape(baseline_prefix + filename)}">'
                    f"{html.escape(label)}</a>"
                )
                for filename, label in (
                    ("task_investigation_bundle.json", "v0 bundle"),
                    ("task_investigation_attempt.json", "v0 attempt"),
                    ("task_investigation.json", "v0 pack"),
                    ("task_investigation_status.json", "v0 status"),
                    ("run_manifest.json", "v0 run manifest"),
                    ("report.json", "v0 report data"),
                )
            )
        size_text = (
            f"v0 {baseline_bytes:,} B -> v0.1 {detail['pack_bytes']:,} B "
            f"({'shorter' if detail['pack_bytes'] < baseline_bytes else 'not shorter'})"
            if baseline_bytes is not None
            else f"v0.1 {detail['pack_bytes']:,} B; no v0 baseline"
        )
        verification = "; ".join(detail["verification"]) or "missing evidence"
        recovered = "; ".join(episode.get("notes") or []) or episode["gold_loss_detail"]
        run_prefix = "../" + str(episode["run_dir"]) + "/"
        attempt_prefix = "../" + str(episode["attempt_dir"]) + "/"
        task_href = "../episodes/" + episode_id + "/task.md"
        final_links = "".join(
            (
                f'<a href="{html.escape(run_prefix + filename)}">'
                f"{html.escape(label)}</a>"
            )
            for filename, label in (
                ("task_investigation_bundle.json", "v0.1 bundle"),
                ("task_investigation_attempt.json", "v0.1 attempt"),
                ("task_investigation.json", "v0.1 pack"),
                ("task_investigation_status.json", "v0.1 status"),
                ("retrieval_trace.json", "retrieval trace"),
                ("retrieval_trace.md", "trace projection"),
            )
        )
        attempt_links = "".join(
            (
                f'<a href="{html.escape(attempt_prefix + filename)}">'
                f"{html.escape(label)}</a>"
            )
            for filename, label in (
                ("role_contract.json", "role contract"),
                ("role_coverage.json", "role coverage"),
                ("verification_frontier.json", "verification frontier"),
                ("cheap_exit_decision.json", "cheap-exit decision"),
                ("source_scopes.json", "source scopes"),
                ("METRICS.json", "metrics"),
                ("HARNESS_ATTEMPT.json", "harness attempt"),
                ("SEALED.json", "attempt seal"),
            )
        )
        route_class = "pass" if _is_real_cheap_exit(episode) else ("warn" if episode["offline"] else "neutral")
        cards.append(
            f"""<article class="episode" id="{html.escape(episode_id)}">
<header><div><p class="eyebrow">{html.escape(episode['evaluation_scope'].replace('_', ' '))}</p><h3>{html.escape(episode_id)}</h3></div><div class="badges"><span class="badge {route_class}">{html.escape(episode['cheap_exit']['route'])}</span><span class="badge neutral">{episode['provider']['calls']} call(s)</span></div></header>
<div class="answers">
<section><h4>Task</h4><p>{html.escape(detail['task_text'])}</p></section>
<section><h4>Where v0 stopped</h4><p>{html.escape(episode.get('baseline_gold_comparison') or 'Calibration episode without a v0 baseline.')}</p></section>
<section><h4>What v0.1 recovered</h4><p><code>{html.escape(episode['gold_loss_stage'])}</code> — {html.escape(recovered)}</p></section>
<section><h4>Local relation</h4><p>{html.escape(detail['relation'])}</p><p class="muted">{html.escape(detail['relation_non_guarantees'])}</p></section>
<section><h4>Verification</h4><p>{html.escape(verification)}</p><p class="muted">Effect: {html.escape(detail['effect'])}</p></section>
<section><h4>Scope and roles</h4><p>{detail['scope_count']} selected scope(s), {detail['partial_scopes']} partial; missing key roles: {html.escape(', '.join(detail['missing_roles']) or 'none')}.</p><p class="muted">Limits causing loss: {html.escape(', '.join(detail['loss_limits']) or 'none')}.</p></section>
<section><h4>Synthesis decision</h4><p>Calls {episode['provider']['calls']}; offline {str(episode['offline']).lower()}; sufficient {str(episode['status']['sufficient']).lower()}; useful {str(episode['useful']).lower()}.</p></section>
<section><h4>Length and utility</h4><p>{html.escape(size_text)}. Completion expansions {detail['completion_expansions']}/2.</p></section>
</div>
<div class="scoregrid">{score_cells}</div>
<footer><a href="{html.escape(task_href)}">task.md</a>{baseline_links}{final_links}{attempt_links}</footer>
</article>"""
        )
    totals = result["resource_totals"]
    return f"""<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Task Lens v0.1 development review</title>
<style>
:root{{--ink:#17202a;--muted:#667085;--line:#d9e0e8;--paper:#fff;--wash:#f3f6f8;--accent:#234f83;--good:#176b45;--bad:#a33a32;--warn:#8a5b10}}
*{{box-sizing:border-box}} body{{margin:0;background:var(--wash);color:var(--ink);font:15px/1.5 ui-sans-serif,system-ui,-apple-system,sans-serif}}
main{{max-width:1240px;min-width:0;margin:auto;padding:42px 24px 80px}} h1{{font:700 clamp(2.3rem,5vw,4.7rem)/.98 ui-serif,Georgia,serif;letter-spacing:-.045em;margin:.2rem 0 1rem;max-width:900px}} h2{{font:700 1.65rem/1.2 ui-serif,Georgia,serif;margin:3rem 0 1rem}} h3{{font:700 1.35rem/1.2 ui-serif,Georgia,serif;margin:.15rem 0}} h4{{font-size:.75rem;text-transform:uppercase;letter-spacing:.09em;margin:0 0 .45rem;color:var(--accent)}} p{{margin:.35rem 0}} code{{font-size:.9em;overflow-wrap:anywhere}} a{{color:var(--accent);font-weight:650;text-underline-offset:3px}}
.hero{{background:var(--paper);border:1px solid var(--line);border-radius:24px;padding:clamp(24px,5vw,56px);box-shadow:0 18px 60px rgba(27,47,68,.08)}} .kicker,.eyebrow{{color:var(--accent);font-weight:800;text-transform:uppercase;letter-spacing:.12em;font-size:.72rem}} .notice{{max-width:850px;border-left:4px solid var(--warn);padding:.8rem 1rem;background:#fff8ea;margin:1.5rem 0}} .metrics{{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:12px;margin-top:1.8rem}} .metric{{background:var(--wash);padding:14px;border-radius:12px}} .metric b{{display:block;font-size:1.35rem}} .artifact-nav{{display:flex;gap:12px;flex-wrap:wrap;background:var(--paper);border:1px solid var(--line);border-radius:14px;padding:16px;margin:20px 0}}
table{{border-collapse:collapse;width:100%;background:var(--paper);border-radius:14px;overflow:hidden;box-shadow:0 1px 0 var(--line)}} th,td{{border-bottom:1px solid var(--line);padding:.72rem;text-align:left;vertical-align:top}} th{{background:#eaf0f5;font-size:.74rem;text-transform:uppercase;letter-spacing:.06em}} .badge{{display:inline-flex;border-radius:999px;padding:.22rem .62rem;font-size:.73rem;font-weight:800;background:#edf1f5}} .badge.pass{{background:#daf3e6;color:var(--good)}} .badge.fail{{background:#fde1df;color:var(--bad)}} .badge.warn{{background:#fff0cc;color:var(--warn)}} .badge.neutral{{background:#e7edf3;color:#40546a}}
.episodes{{display:grid;gap:20px;min-width:0}} .episode{{min-width:0;overflow-wrap:anywhere;background:var(--paper);border:1px solid var(--line);border-radius:18px;padding:24px;box-shadow:0 10px 30px rgba(27,47,68,.05)}} .episode header{{display:flex;min-width:0;justify-content:space-between;gap:18px;align-items:flex-start;border-bottom:1px solid var(--line);padding-bottom:16px}} .badges{{display:flex;min-width:0;gap:7px;flex-wrap:wrap;justify-content:flex-end}} .answers{{display:grid;min-width:0;grid-template-columns:repeat(3,minmax(0,1fr));gap:18px;padding:20px 0}} .answers section{{min-width:0;border-left:2px solid #dfe7ee;padding-left:12px}} .muted{{color:var(--muted)}} .scoregrid{{display:grid;min-width:0;grid-template-columns:repeat(4,minmax(0,1fr));gap:7px;background:var(--wash);padding:12px;border-radius:12px}} .scoregrid div{{display:flex;min-width:0;overflow-wrap:anywhere;justify-content:space-between;gap:8px;background:white;border:1px solid var(--line);padding:7px 9px;border-radius:8px;font-size:.78rem}} .episode footer{{display:flex;min-width:0;gap:16px;flex-wrap:wrap;margin-top:18px}}
.decision{{background:#172f49;color:#f6fbff;border-radius:18px;padding:24px;margin-top:28px}} .decision h2{{margin-top:0;color:white}} .decision p{{font-size:1.08rem}} @media(max-width:850px){{.metrics,.answers,.scoregrid{{grid-template-columns:1fr 1fr}}}} @media(max-width:560px){{main{{padding:20px 12px 50px}}.metrics,.answers,.scoregrid{{grid-template-columns:1fr}}.episode header{{display:block}}.badges{{justify-content:flex-start;margin-top:12px}}}}
</style></head><body><main>
<section class="hero"><p class="kicker">Repomap · frozen development regression</p><h1>Task Lens v0.1 evidence review</h1><p class="notice"><strong>Known development only.</strong> Fresh generalization was not measured, and product integration is not authorized.</p><div class="metrics"><div class="metric"><span>Outcome</span><b>{html.escape(result['outcome'])}</b></div><div class="metric"><span>Model calls</span><b>{totals['model_calls']}</b></div><div class="metric"><span>Input / output tokens</span><b>{totals['input_tokens']} / {totals['output_tokens']}</b></div><div class="metric"><span>Baseline files verified</span><b>{validation['baseline']['regular_files_verified']}</b></div></div></section>
<nav class="artifact-nav" aria-label="Review artifacts">{artifact_links}</nav>
<h2>Decision gates</h2><table><thead><tr><th>Gate</th><th>Observed</th><th>Target</th><th>Result</th></tr></thead><tbody>{gate_rows}</tbody></table>
<h2>Episode evidence</h2><div class="episodes">{''.join(cards)}</div>
<section class="decision"><h2>One next step</h2><p>{html.escape(result['recommended_next_experiment'])}</p><p>Passing this page's gates authorizes at most one untouched holdout, never product integration.</p></section>
</main></body></html>"""


def command_prepare(args: argparse.Namespace) -> dict[str, Any]:
    return prepare(Path(args.root).resolve())


def command_validate(args: argparse.Namespace) -> dict[str, Any]:
    result = validate_artifacts(Path(args.root).resolve())
    if args.output:
        write_json(Path(args.output).resolve(), result)
    if not result["passed"]:
        raise EvaluationError(f"artifact validation failed with {len(result['errors'])} errors")
    return result


def command_evaluate(args: argparse.Namespace) -> dict[str, Any]:
    return evaluate(Path(args.root).resolve(), Path(args.scores).resolve())["result"]


def command_report(args: argparse.Namespace) -> dict[str, Any]:
    return render_reports(Path(args.root).resolve())


def command_all(args: argparse.Namespace) -> dict[str, Any]:
    root = Path(args.root).resolve()
    evaluated = evaluate(root, Path(args.scores).resolve())
    reports = render_reports(root)
    return {"result": evaluated["result"], "reports": reports}


def parser() -> argparse.ArgumentParser:
    result = argparse.ArgumentParser(description=__doc__)
    commands = result.add_subparsers(dest="command", required=True)

    prepare_parser = commands.add_parser("prepare", help="verify v0 and write the v0.1 manifest/score template")
    prepare_parser.add_argument("--root", default="tmp/task-lens-v01")
    prepare_parser.set_defaults(func=command_prepare)

    validate_parser = commands.add_parser("validate", help="validate baseline, final attempts, traces, and seals")
    validate_parser.add_argument("--root", default="tmp/task-lens-v01")
    validate_parser.add_argument("--output", help="optional JSON validation output path")
    validate_parser.set_defaults(func=command_validate)

    evaluate_parser = commands.add_parser("evaluate", help="validate artifacts and compute the fixed development gates")
    evaluate_parser.add_argument("--root", default="tmp/task-lens-v01")
    evaluate_parser.add_argument("--scores", required=True, help="completed copy of SCORES.template.json")
    evaluate_parser.set_defaults(func=command_evaluate)

    report_parser = commands.add_parser("report", help="render the Markdown and static HTML review from sealed evaluation JSON")
    report_parser.add_argument("--root", default="tmp/task-lens-v01")
    report_parser.set_defaults(func=command_report)

    all_parser = commands.add_parser("all", help="validate, evaluate, and render reports")
    all_parser.add_argument("--root", default="tmp/task-lens-v01")
    all_parser.add_argument("--scores", required=True, help="completed copy of SCORES.template.json")
    all_parser.set_defaults(func=command_all)
    return result


def main(argv: Sequence[str] | None = None) -> int:
    try:
        args = parser().parse_args(argv)
        value = args.func(args)
        print(json.dumps(value, indent=2, sort_keys=True, ensure_ascii=False))
        return 0
    except EvaluationError as exc:
        print(f"task-lens-v01-eval: error: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
