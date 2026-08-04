#!/usr/bin/env python3
"""Evaluate and report the bounded Task Lens v0.2 verification redesign.

The evaluator intentionally reuses only the v0.1 artifact-schema validators.
Its experiment set, score kinds, baseline closure, gates, evaluation seals, and
report seals are v0.2-specific.  It never invokes a target-repository command.
"""

from __future__ import annotations

import argparse
import hashlib
import html
import json
import os
import shutil
import stat
import sys
import tempfile
from collections import Counter
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Iterable, Mapping, Sequence

sys.dont_write_bytecode = True

import task_lens_v01_eval as v01


EvaluationError = v01.EvaluationError
EpisodeSpec = v01.EpisodeSpec
Audit = v01.Audit
SCORE_DIMENSIONS = v01.SCORE_DIMENSIONS
QUARTET = v01.QUARTET
GOLD_LOSS_STAGES = v01.GOLD_LOSS_STAGES

PRIMARY_EPISODES = tuple(v01.PRIMARY_EPISODES)
EPISODES = PRIMARY_EPISODES
PRIMARY_IDS = tuple(episode.episode_id for episode in PRIMARY_EPISODES)
EPISODE_BY_ID = {episode.episode_id: episode for episode in PRIMARY_EPISODES}

# Only the one useful zero-call episode in the same six-primary v0.1 set is a
# no-regression target.  The former seventh configuration calibration is not a
# v0.2 evaluation episode.
ZERO_CALL_BASELINE_IDS = ("nil_body_validation_panic",)
IMPROVEMENT_TARGET_IDS = (
    "openapi_required_nullable_semantics",
    "accept_header_wrong_status",
    "multi_module_release_script",
)

V01_PINNED_IDENTITIES = {
    "final_run_config_sha256": "f5c0439e82c1f921c84d065e21c634e1fd4eb9d7b0b5350ef845a77521c5b4ee",
    "development_manifest_sha256": "a760994c6da2258edc360ca976b994824a5c391ac00c62b0348b7369e39bb207",
    "binary_sha256": "30780fde698e48822d2d07653e43036610aee5283ccb3c57537c7d2792a0050f",
    "route_plan_sha256": "7504f07cda716a862eb3276df3d9ddf980087ae400c071f19f4bfa78d0e5f652",
    "evaluation_seal_sha256": "d9f8c0d10d3b750da03b9157f04784f96eed32be446505e0a2c993f718ac2fd4",
    "scorecard_sha256": "b4cbab57f27da1d00bc2b1fce3a4d4788f8a8e126848b9922502e049245bed87",
    "supervisor_report_sha256": "4365e5ea41cdcda3cc6ff21bcd7ce6c9f9e75efcb56b0be1e4caac435cd079d7",
}

BASELINE_DIR_NAME = "baseline-v01"
BASELINE_MANIFEST_NAME = "BASELINE_V01_MANIFEST.json"
BASELINE_MANIFEST_SIDECAR = "BASELINE_V01_MANIFEST.sha256"
EVALUATION_DIR_NAME = "evaluation-v02"
REPORT_SEAL_NAME = "REPORT_SEAL.json"
REPORT_SEAL_SIDECAR = "REPORT_SEAL.sha256"

BASELINE_TOP_LEVEL_FILES = {
    "FINAL_RUN_CONFIG.json",
    "FINAL_RUN_CONFIG.sha256",
    "SUPERVISOR_REPORT.md",
    "inputs/DEVELOPMENT_SET.json",
    "inputs/repomap",
    "evaluation-v01/ARTIFACT_VALIDATION.json",
    "evaluation-v01/DEVELOPMENT_SET.json",
    "evaluation-v01/EVALUATION_RESULT.json",
    "evaluation-v01/EVALUATION_SEAL.json",
    "evaluation-v01/EVALUATION_SEAL.sha256",
    "evaluation-v01/GOLD_LOSS_LEDGER.json",
    "evaluation-v01/SCORECARD.json",
    "evaluation-v01/SCORES.json",
}

BASELINE_SOURCE_PATHS = (
    "FINAL_RUN_CONFIG.json",
    "FINAL_RUN_CONFIG.sha256",
    "SUPERVISOR_REPORT.md",
    "inputs/DEVELOPMENT_SET.json",
    "inputs/repomap",
    "evaluation-v01/ARTIFACT_VALIDATION.json",
    "evaluation-v01/DEVELOPMENT_SET.json",
    "evaluation-v01/EVALUATION_RESULT.json",
    "evaluation-v01/EVALUATION_SEAL.json",
    "evaluation-v01/EVALUATION_SEAL.sha256",
    "evaluation-v01/GOLD_LOSS_LEDGER.json",
    "evaluation-v01/SCORECARD.json",
    "evaluation-v01/SCORES.json",
    *(
        relative
        for episode_id in PRIMARY_IDS
        for relative in (
            f"episodes/{episode_id}/task.md",
            f"episodes/{episode_id}/final/attempt",
        )
    ),
)

REQUIRED_RENDERED_REPORTS = {
    "BASELINE_COMPARISON.md",
    "CHEAP_EXIT_EVALUATION.md",
    "DEV_REGRESSION.md",
    "PRODUCT_FINDINGS.md",
    "RETRIEVAL_FAILURES.md",
    "SOURCE_SCOPE_AUDIT.md",
    "SUPERVISOR_REPORT.md",
    "VERIFICATION_BINDING.md",
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

FROZEN_BUDGETS = {
    "initial_candidates": 40,
    "retained_anchors": 16,
    "evidence_files_considered": 12,
    "read_files": 12,
    "read_bytes": 128 * 1024,
    "source_scan_bytes": 4 * 1024 * 1024,
    "retained_source_bytes": 128 * 1024,
    "frontier_expansions": 2,
    "local_wall_millis": 10_000,
}


def utc_now() -> str:
    return datetime.now(timezone.utc).replace(microsecond=0).isoformat()


def sha256_bytes(raw: bytes) -> str:
    return hashlib.sha256(raw).hexdigest()


def sha256_file(path: Path) -> str:
    return v01.sha256_file(path)


def canonical_json_sha256(value: Any) -> str:
    return v01.canonical_json_sha256(value)


def harness_json_sha256(value: Any) -> str:
    raw = (json.dumps(value, indent=2, sort_keys=True, ensure_ascii=False) + "\n").encode(
        "utf-8"
    )
    return sha256_bytes(raw)


def read_json(path: Path) -> dict[str, Any]:
    return v01.read_json(path)


def write_json(path: Path, value: Any) -> None:
    v01.write_json(path, value)


def write_text(path: Path, value: str) -> None:
    v01.write_text(path, value)


def _safe_relative_path(raw: str, source: Path) -> Path:
    return v01._safe_relative_path(raw, source)


def _baseline_path_allowed(relative: str) -> bool:
    """Return whether a file/symlink belongs to the minimal v0.1 closure."""
    if relative in BASELINE_TOP_LEVEL_FILES:
        return True
    parts = Path(relative).parts
    forbidden = {"baseline", "baseline-v01", "holdout", "screenshots", "worktree"}
    if any(part.lower() in forbidden for part in parts):
        return False
    if len(parts) == 3:
        return parts[0] == "episodes" and parts[1] in PRIMARY_IDS and parts[2] == "task.md"
    return (
        len(parts) >= 5
        and parts[0] == "episodes"
        and parts[1] in PRIMARY_IDS
        and parts[2:4] == ("final", "attempt")
    )


def _inventory_entry(path: Path, baseline: Path) -> dict[str, Any]:
    relative = path.relative_to(baseline).as_posix()
    if path.is_symlink():
        target = os.readlink(path)
        return {
            "path": relative,
            "kind": "symlink",
            "target": target,
            "bytes": len(target.encode("utf-8")),
            "sha256": sha256_bytes(target.encode("utf-8")),
        }
    return {
        "path": relative,
        "kind": "file",
        "bytes": path.stat().st_size,
        "sha256": sha256_file(path),
    }


def _scan_baseline(baseline: Path) -> list[dict[str, Any]]:
    entries: list[dict[str, Any]] = []
    for path in sorted(baseline.rglob("*")):
        if path.is_symlink() or path.is_file():
            entries.append(_inventory_entry(path, baseline))
    return entries


def _verify_pinned_v01_identities(baseline: Path) -> None:
    checks = {
        "final_run_config_sha256": baseline / "FINAL_RUN_CONFIG.json",
        "development_manifest_sha256": baseline / "inputs" / "DEVELOPMENT_SET.json",
        "binary_sha256": baseline / "inputs" / "repomap",
        "evaluation_seal_sha256": baseline / "evaluation-v01" / "EVALUATION_SEAL.json",
        "scorecard_sha256": baseline / "evaluation-v01" / "SCORECARD.json",
        "supervisor_report_sha256": baseline / "SUPERVISOR_REPORT.md",
    }
    for identity, path in checks.items():
        actual = sha256_file(path)
        expected = V01_PINNED_IDENTITIES[identity]
        if actual != expected:
            raise EvaluationError(f"baseline v0.1 {identity} mismatch: {actual} != {expected}")
    config = read_json(baseline / "FINAL_RUN_CONFIG.json")
    if config.get("kind") != "task_lens_v01_final_run_config":
        raise EvaluationError("baseline FINAL_RUN_CONFIG.json is not the sealed v0.1 config")
    if config.get("route_plan_sha256") != V01_PINNED_IDENTITIES["route_plan_sha256"]:
        raise EvaluationError("baseline v0.1 route plan identity mismatch")
    if harness_json_sha256(config.get("route_plan")) != V01_PINNED_IDENTITIES["route_plan_sha256"]:
        raise EvaluationError("baseline v0.1 route plan content does not match its pinned hash")
    scorecard = read_json(baseline / "evaluation-v01" / "SCORECARD.json")
    if scorecard.get("kind") != "task_lens_v01_development_scorecard":
        raise EvaluationError("baseline scorecard is not the pinned v0.1 scorecard")
    evaluation = read_json(baseline / "evaluation-v01" / "EVALUATION_RESULT.json")
    if (
        evaluation.get("kind") != "task_lens_v01_development_evaluation"
        or evaluation.get("episodes_evaluated") != 6
    ):
        raise EvaluationError("baseline v0.1 evaluation identity/count mismatch")


def _verify_manifest_sidecar(root: Path, name: str, sidecar_name: str) -> None:
    path = root / name
    sidecar = root / sidecar_name
    try:
        value = sidecar.read_text(encoding="utf-8").strip()
    except OSError as exc:
        raise EvaluationError(f"cannot read {sidecar}: {exc}") from exc
    expected = f"{sha256_file(path)}  {name}"
    if value != expected:
        raise EvaluationError(f"{sidecar_name} does not bind {name}")


def _make_read_only(baseline: Path) -> None:
    for path in sorted(baseline.rglob("*"), reverse=True):
        if path.is_symlink():
            continue
        mode = stat.S_IMODE(path.stat().st_mode)
        if path.is_dir():
            path.chmod((mode & 0o555) or 0o555)
        else:
            path.chmod(0o555 if mode & 0o111 else 0o444)
    baseline.chmod(0o555)


def freeze_baseline(root: Path, source: Path) -> dict[str, Any]:
    """Copy the allowlisted v0.1 closure and freeze it read-only."""
    baseline = root / BASELINE_DIR_NAME
    manifest_path = root / BASELINE_MANIFEST_NAME
    sidecar_path = root / BASELINE_MANIFEST_SIDECAR
    if baseline.exists() or manifest_path.exists() or sidecar_path.exists():
        raise EvaluationError("refusing to replace an existing v0.1 baseline or manifest")
    if not source.is_dir():
        raise EvaluationError(f"v0.1 source root is missing: {source}")
    root.mkdir(parents=True, exist_ok=True)
    temporary = Path(tempfile.mkdtemp(prefix=".baseline-v01.", dir=root))
    try:
        for raw_relative in BASELINE_SOURCE_PATHS:
            relative = Path(raw_relative)
            source_path = source / relative
            target = temporary / relative
            if source_path.is_dir():
                target.parent.mkdir(parents=True, exist_ok=True)
                shutil.copytree(source_path, target, symlinks=True)
            elif source_path.is_file():
                target.parent.mkdir(parents=True, exist_ok=True)
                shutil.copy2(source_path, target)
            else:
                raise EvaluationError(f"baseline source closure is incomplete: {source_path}")
        for entry in _scan_baseline(temporary):
            if not _baseline_path_allowed(entry["path"]):
                raise EvaluationError(f"baseline source closure contains a forbidden path: {entry['path']}")
        _verify_pinned_v01_identities(temporary)
        for episode in PRIMARY_EPISODES:
            validated = v01.validate_episode(temporary, episode)
            if not validated["artifact_valid"]:
                raise EvaluationError(
                    f"baseline v0.1 episode {episode.episode_id} is invalid: "
                    f"{validated['errors'][0]}"
                )
        entries = _scan_baseline(temporary)
        os.replace(temporary, baseline)
        _make_read_only(baseline)
    except BaseException:
        if temporary.exists():
            shutil.rmtree(temporary, ignore_errors=True)
        raise
    manifest = {
        "version": 1,
        "kind": "task_lens_v02_baseline_v01_manifest",
        "created_at": utc_now(),
        "baseline_dir": BASELINE_DIR_NAME,
        "source_result": "Task Lens v0.1 known-development partial",
        "primary_episode_ids": list(PRIMARY_IDS),
        "excluded": [
            "v0 holdout and freeze",
            "nested baseline",
            "worktrees",
            "screenshots",
            "cheap-exit-only seventh calibration episode",
        ],
        "pinned_identities": dict(V01_PINNED_IDENTITIES),
        "entries": entries,
    }
    write_json(manifest_path, manifest)
    write_text(
        sidecar_path,
        f"{sha256_file(manifest_path)}  {BASELINE_MANIFEST_NAME}",
    )
    return verify_baseline(root)


def verify_baseline(root: Path) -> dict[str, Any]:
    baseline = root / BASELINE_DIR_NAME
    manifest_path = root / BASELINE_MANIFEST_NAME
    if not baseline.is_dir():
        raise EvaluationError(f"missing baseline directory: {baseline}")
    _verify_manifest_sidecar(root, BASELINE_MANIFEST_NAME, BASELINE_MANIFEST_SIDECAR)
    manifest = read_json(manifest_path)
    if manifest.get("kind") != "task_lens_v02_baseline_v01_manifest":
        raise EvaluationError("baseline manifest kind is not task_lens_v02")
    if manifest.get("baseline_dir") != BASELINE_DIR_NAME:
        raise EvaluationError("baseline manifest directory mismatch")
    if manifest.get("primary_episode_ids") != list(PRIMARY_IDS):
        raise EvaluationError("baseline manifest does not contain exactly the six primary episodes")
    if manifest.get("pinned_identities") != V01_PINNED_IDENTITIES:
        raise EvaluationError("baseline manifest pinned identities differ from the v0.1 freeze")
    raw_entries = manifest.get("entries")
    if not isinstance(raw_entries, list):
        raise EvaluationError("baseline manifest entries must be a list")
    by_path: dict[str, Mapping[str, Any]] = {}
    for raw in raw_entries:
        if not isinstance(raw, dict) or not isinstance(raw.get("path"), str):
            raise EvaluationError("baseline manifest contains an invalid entry")
        relative = _safe_relative_path(raw["path"], manifest_path).as_posix()
        if relative in by_path:
            raise EvaluationError(f"duplicate baseline manifest path: {relative}")
        if not _baseline_path_allowed(relative):
            raise EvaluationError(f"baseline manifest contains forbidden path: {relative}")
        by_path[relative] = raw
    actual = {entry["path"]: entry for entry in _scan_baseline(baseline)}
    if set(actual) != set(by_path):
        omitted = sorted(set(actual) - set(by_path))
        missing = sorted(set(by_path) - set(actual))
        raise EvaluationError(f"baseline inventory mismatch; omitted={omitted[:5]}, missing={missing[:5]}")
    for relative, expected in by_path.items():
        current = actual[relative]
        for field_name in ("kind", "bytes", "sha256"):
            if current.get(field_name) != expected.get(field_name):
                raise EvaluationError(f"baseline inventory mismatch for {relative}: {field_name}")
        if current["kind"] == "symlink":
            if current.get("target") != expected.get("target"):
                raise EvaluationError(f"baseline symlink target mismatch: {relative}")
            target_path = baseline / relative
            try:
                resolved = target_path.resolve(strict=True)
                resolved.relative_to(baseline.resolve())
            except (OSError, ValueError) as exc:
                raise EvaluationError(f"baseline symlink escapes closure: {relative}") from exc
    writable: list[str] = []
    for path in (baseline, *baseline.rglob("*")):
        if path.is_symlink():
            continue
        if stat.S_IMODE(path.stat().st_mode) & 0o222:
            writable.append(path.relative_to(root).as_posix())
    if writable:
        raise EvaluationError("baseline is not read-only: " + ", ".join(writable[:10]))
    _verify_pinned_v01_identities(baseline)
    for episode in PRIMARY_EPISODES:
        validated = v01.validate_episode(baseline, episode)
        if not validated["artifact_valid"]:
            raise EvaluationError(
                f"baseline v0.1 episode {episode.episode_id} is invalid: "
                f"{validated['errors'][0]}"
            )
    return {
        "state": "verified",
        "manifest_path": BASELINE_MANIFEST_NAME,
        "manifest_sha256": sha256_file(manifest_path),
        "entries_verified": len(actual),
        "read_only": True,
        "pinned_identities": dict(V01_PINNED_IDENTITIES),
    }


def development_manifest(root: Path | None = None) -> dict[str, Any]:
    baseline_sha = None
    if root is not None and (root / BASELINE_MANIFEST_NAME).is_file():
        baseline_sha = sha256_file(root / BASELINE_MANIFEST_NAME)
    return {
        "version": 2,
        "kind": "task_lens_v02_known_development",
        "known_development_only": True,
        "fresh_generalization": False,
        "primary_episode_count": 6,
        "cheap_exit_calibration_count": 0,
        "primary_episode_ids": list(PRIMARY_IDS),
        "baseline_v01_manifest_sha256": baseline_sha,
        "episodes": [
            {
                "episode_id": episode.episode_id,
                "base_revision": episode.base_revision,
                "evaluation_scope": "primary_regression",
                "cheap_exit_target": episode.cheap_exit_target,
                "final_attempt": f"episodes/{episode.episode_id}/final/attempt",
                "baseline_attempt": (
                    f"{BASELINE_DIR_NAME}/episodes/{episode.episode_id}/final/attempt"
                ),
            }
            for episode in PRIMARY_EPISODES
        ],
        "constraints": {
            "semantic_retries": 0,
            "maximum_model_calls_per_episode": 1,
            "maximum_total_fresh_model_calls": 6,
            "provider_model": v01.V0_PROVIDER_MODEL,
            "provider_endpoint": v01.V0_PROVIDER_ENDPOINT,
            "target_repository_commands": 0,
        },
    }


def scores_template() -> dict[str, Any]:
    return {
        "version": 1,
        "kind": "task_lens_v02_supervisor_scores",
        "known_development_only": True,
        "opaque_average_computed": False,
        "recommended_next_experiment": None,
        "episodes": [
            {
                "id": episode.episode_id,
                "evaluation_scope": "primary_regression",
                "scores": {dimension: None for dimension in SCORE_DIMENSIONS},
                "decisive_key_roles_present": None,
                "major_unsupported_claims": [],
                "absence_claims_from_incomplete_scope": [],
                "clipped_before_known_task_match_without_partial_window": [],
                "false_exact_or_unrelated_verification_anchors": [],
                "unsupported_verification_effects_or_invented_commands": [],
                "gold_loss_stage": None,
                "gold_candidate_id": None,
                "gold_anchor_id": None,
                "gold_loss_detail": None,
                "useful": None,
                "notes": [],
            }
            for episode in PRIMARY_EPISODES
        ],
    }


def _write_idempotent_json(path: Path, value: Any) -> None:
    if path.exists():
        if read_json(path) != value:
            raise EvaluationError(f"refusing to replace non-matching prepared input: {path}")
        return
    write_json(path, value)


def prepare(root: Path) -> dict[str, Any]:
    baseline = verify_baseline(root)
    evaluation_dir = root / EVALUATION_DIR_NAME
    manifest_path = evaluation_dir / "DEVELOPMENT_SET.json"
    scores_path = evaluation_dir / "SCORES.template.json"
    _write_idempotent_json(manifest_path, development_manifest(root))
    _write_idempotent_json(scores_path, scores_template())
    return {
        "baseline": baseline,
        "development_set": str(manifest_path),
        "scores_template": str(scores_path),
        "canonical_final_attempt": "episodes/<episode-id>/final/attempt",
    }


def _validate_final_run_config(root: Path) -> dict[str, Any]:
    path = root / "FINAL_RUN_CONFIG.json"
    sidecar = root / "FINAL_RUN_CONFIG.sha256"
    config = read_json(path)
    if config.get("kind") != "task_lens_v02_final_run_config":
        raise EvaluationError("FINAL_RUN_CONFIG.json kind must be task_lens_v02_final_run_config")
    if config.get("known_development_only") is not True:
        raise EvaluationError("FINAL_RUN_CONFIG.json must state known_development_only=true")
    if config.get("semantic_retries") != 0:
        raise EvaluationError("FINAL_RUN_CONFIG.json semantic_retries must be zero")
    if config.get("provider_environment_disabled") is not True:
        raise EvaluationError("FINAL_RUN_CONFIG.json must disable the provider environment")
    if config.get("target_repository_commands_executed") != 0:
        raise EvaluationError("FINAL_RUN_CONFIG.json recorded target-repository execution")
    if config.get("maximum_process_invocations_per_episode") != 1:
        raise EvaluationError("FINAL_RUN_CONFIG.json must allow one process invocation per episode")
    raw_episodes = config.get("episodes")
    if not isinstance(raw_episodes, list) or len(raw_episodes) != 6:
        raise EvaluationError("FINAL_RUN_CONFIG.json must contain exactly six episodes")
    configured = {
        entry.get("episode_id"): entry
        for entry in raw_episodes
        if isinstance(entry, dict) and isinstance(entry.get("episode_id"), str)
    }
    if set(configured) != set(PRIMARY_IDS):
        raise EvaluationError("FINAL_RUN_CONFIG.json episode set is not the six primary episodes")
    for spec in PRIMARY_EPISODES:
        if configured[spec.episode_id].get("base_revision") != spec.base_revision:
            raise EvaluationError(
                f"FINAL_RUN_CONFIG.json revision mismatch for {spec.episode_id}"
            )
    for path_field, hash_field in (
        ("binary_path", "binary_sha256"),
        ("manifest_path", "manifest_sha256"),
    ):
        raw_relative = config.get(path_field)
        if not isinstance(raw_relative, str):
            raise EvaluationError(f"FINAL_RUN_CONFIG.json {path_field} is missing")
        target = root / _safe_relative_path(raw_relative, path)
        if sha256_file(target) != config.get(hash_field):
            raise EvaluationError(
                f"FINAL_RUN_CONFIG.json {hash_field} does not bind {path_field}"
            )
    route = config.get("route_plan")
    if not isinstance(route, dict) or route.get("kind") != "task_lens_v02_frozen_route_plan":
        raise EvaluationError("FINAL_RUN_CONFIG.json route plan kind must be task_lens_v02")
    if config.get("route_plan_sha256") != harness_json_sha256(route):
        raise EvaluationError("FINAL_RUN_CONFIG.json route plan hash mismatch")
    zero_ids = route.get("zero_call_episode_ids")
    offline_ids = route.get("offline_partial_episode_ids")
    if (
        not isinstance(zero_ids, list)
        or not isinstance(offline_ids, list)
        or set(zero_ids) | set(offline_ids) != set(PRIMARY_IDS)
        or set(zero_ids) & set(offline_ids)
    ):
        raise EvaluationError("v0.2 route plan must partition exactly the six primary episodes")
    expected_sidecar = f"{sha256_file(path)}  FINAL_RUN_CONFIG.json"
    try:
        actual_sidecar = sidecar.read_text(encoding="utf-8").strip()
    except OSError as exc:
        raise EvaluationError(f"cannot read FINAL_RUN_CONFIG.sha256: {exc}") from exc
    if actual_sidecar != expected_sidecar:
        raise EvaluationError("FINAL_RUN_CONFIG.sha256 does not bind FINAL_RUN_CONFIG.json")
    return {
        "path": path.relative_to(root).as_posix(),
        "sha256": sha256_file(path),
        "route_plan_sha256": config["route_plan_sha256"],
        "zero_call_episode_ids": zero_ids,
    }


def _validate_frozen_budgets(root: Path, episode: Mapping[str, Any]) -> list[str]:
    errors: list[str] = []
    run_dir = root / str(episode["run_dir"])
    trace = read_json(run_dir / "retrieval_trace.json")
    budgets = trace.get("budgets")
    if not isinstance(budgets, dict):
        return [f"{episode['id']}: retrieval budgets are missing"]
    capped_names = (
        "initial_candidates",
        "retained_anchors",
        "evidence_files_considered",
        "read_files",
        "read_bytes",
        "source_scan_bytes",
        "retained_source_bytes",
        "frontier_expansions",
        "local_wall_millis",
    )
    for name in capped_names:
        # source_scan_bytes is omitted by the Go artifact when its measured
        # value is zero. Every frozen number is a ceiling, not a requirement
        # to consume the full budget on smaller candidate sets.
        value = budgets.get(name, 0)
        if isinstance(value, bool) or not isinstance(value, int) or value > FROZEN_BUDGETS[name]:
            errors.append(
                f"{episode['id']}: {name} exceeds frozen cap {FROZEN_BUDGETS[name]}"
            )
    selected = trace.get("selected_anchors")
    per_file: Counter[str] = Counter()
    if isinstance(selected, list):
        candidates = {
            candidate.get("id"): candidate
            for candidate in trace.get("candidates_before_ranking", [])
            if isinstance(candidate, dict)
        }
        for item in selected:
            if not isinstance(item, dict):
                continue
            candidate = candidates.get(item.get("candidate_id"))
            if isinstance(candidate, dict) and isinstance(candidate.get("path"), str):
                per_file[candidate["path"]] += 1
    if per_file and max(per_file.values()) > 8:
        errors.append(f"{episode['id']}: retained more than eight anchors from one file")
    frontier = trace.get("verification_frontier")
    if isinstance(frontier, dict):
        anchors = frontier.get("anchors")
        if isinstance(anchors, list) and len(anchors) > 2:
            errors.append(f"{episode['id']}: retained more than two verification anchors")
    return errors


def validate_artifacts(root: Path) -> dict[str, Any]:
    baseline = verify_baseline(root)
    final_config = _validate_final_run_config(root)
    manifest_path = root / EVALUATION_DIR_NAME / "DEVELOPMENT_SET.json"
    if read_json(manifest_path) != development_manifest(root):
        raise EvaluationError("evaluation-v02 development manifest is stale or not v0.2")
    episodes: list[dict[str, Any]] = []
    top_errors: list[str] = []
    for spec in PRIMARY_EPISODES:
        try:
            entry = v01.validate_episode(root, spec)
        except EvaluationError as exc:
            entry = {
                "id": spec.episode_id,
                "evaluation_scope": "primary_regression",
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
        if entry.get("provider", {}).get("calls") != 0:
            entry["errors"].append(f"{spec.episode_id}: scored v0.2 run must use zero provider calls")
            entry["artifact_valid"] = False
        if entry.get("artifact_valid"):
            budget_errors = _validate_frozen_budgets(root, entry)
            entry["errors"].extend(budget_errors)
            if budget_errors:
                entry["artifact_valid"] = False
        episodes.append(entry)
    if len(episodes) != 6 or {entry["id"] for entry in episodes} != set(PRIMARY_IDS):
        top_errors.append("artifact validation did not evaluate exactly six primary episodes")
    all_errors = [*top_errors, *(error for entry in episodes for error in entry["errors"])]
    totals = {
        "model_calls": sum(int(entry["provider"].get("calls", 0)) for entry in episodes),
        "input_tokens": sum(int(entry["provider"].get("input_tokens", 0)) for entry in episodes),
        "output_tokens": sum(int(entry["provider"].get("output_tokens", 0)) for entry in episodes),
        "provider_latency_millis": sum(
            int(entry["provider"].get("latency_millis", 0)) for entry in episodes
        ),
        "wall_millis": sum(int(entry.get("wall_millis", 0)) for entry in episodes),
    }
    if totals["model_calls"] != 0:
        all_errors.append("v0.2 scored run used provider/model calls")
    return {
        "version": 1,
        "kind": "task_lens_v02_artifact_validation",
        "validated_at": utc_now(),
        "known_development_only": True,
        "target_repository_commands_executed": 0,
        "baseline": baseline,
        "final_run_config": final_config,
        "episodes": episodes,
        "resource_totals": totals,
        "errors": all_errors,
        "passed": not all_errors,
    }


def load_scores(path: Path) -> dict[str, Any]:
    scores = read_json(path)
    kind = scores.get("kind")
    if kind != "task_lens_v02_supervisor_scores":
        if isinstance(kind, str) and ("v01" in kind or "v0.1" in kind):
            raise EvaluationError("v0.1 score kinds are forbidden in evaluation-v02")
        raise EvaluationError("scores kind must be task_lens_v02_supervisor_scores")
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
        if raw["id"] in by_id:
            raise EvaluationError(f"duplicate score entry: {raw['id']}")
        by_id[raw["id"]] = raw
    if set(by_id) != set(PRIMARY_IDS) or len(raw_episodes) != 6:
        missing = sorted(set(PRIMARY_IDS) - set(by_id))
        extra = sorted(set(by_id) - set(PRIMARY_IDS))
        raise EvaluationError(
            f"scores must contain exactly six primary episodes; missing={missing}, extra={extra}"
        )
    list_fields = (
        "major_unsupported_claims",
        "absence_claims_from_incomplete_scope",
        "clipped_before_known_task_match_without_partial_window",
        "false_exact_or_unrelated_verification_anchors",
        "unsupported_verification_effects_or_invented_commands",
        "notes",
    )
    for spec in PRIMARY_EPISODES:
        raw = by_id[spec.episode_id]
        if raw.get("evaluation_scope") != "primary_regression":
            raise EvaluationError(f"{spec.episode_id}: evaluation_scope mismatch")
        if not isinstance(raw.get("decisive_key_roles_present"), bool):
            raise EvaluationError(f"{spec.episode_id}: decisive_key_roles_present must be boolean")
        if not isinstance(raw.get("useful"), bool):
            raise EvaluationError(f"{spec.episode_id}: useful must be boolean")
        episode_scores = raw.get("scores")
        if not isinstance(episode_scores, dict) or set(episode_scores) != set(SCORE_DIMENSIONS):
            raise EvaluationError(f"{spec.episode_id}: score dimension set mismatch")
        for dimension, value in episode_scores.items():
            if isinstance(value, bool) or not isinstance(value, int) or not 0 <= value <= 4:
                raise EvaluationError(f"{spec.episode_id}: {dimension} must be an integer 0-4")
        for field_name in list_fields:
            value = raw.get(field_name)
            if not isinstance(value, list) or not all(isinstance(item, str) for item in value):
                raise EvaluationError(f"{spec.episode_id}: {field_name} must be a string list")
        stage = raw.get("gold_loss_stage")
        if stage not in GOLD_LOSS_STAGES:
            raise EvaluationError(f"{spec.episode_id}: invalid gold_loss_stage {stage!r}")
        for identity_name in ("gold_candidate_id", "gold_anchor_id"):
            identity = raw.get(identity_name)
            if identity is not None and (not isinstance(identity, str) or not identity):
                raise EvaluationError(f"{spec.episode_id}: {identity_name} must be null or non-empty")
        if not isinstance(raw.get("gold_loss_detail"), str) or not raw["gold_loss_detail"].strip():
            raise EvaluationError(f"{spec.episode_id}: gold_loss_detail must be non-empty")
    return scores


def _baseline_scorecard(root: Path) -> tuple[dict[str, Any], dict[str, dict[str, Any]]]:
    path = root / BASELINE_DIR_NAME / "evaluation-v01" / "SCORECARD.json"
    scorecard = read_json(path)
    if (
        scorecard.get("kind") != "task_lens_v01_development_scorecard"
        or sha256_file(path) != V01_PINNED_IDENTITIES["scorecard_sha256"]
    ):
        raise EvaluationError("baseline scorecard is not the pinned v0.1 scorecard")
    raw = scorecard.get("episodes")
    if not isinstance(raw, list):
        raise EvaluationError("baseline v0.1 scorecard has no episodes")
    by_id = {
        entry["id"]: entry
        for entry in raw
        if isinstance(entry, dict) and entry.get("id") in PRIMARY_IDS
    }
    if set(by_id) != set(PRIMARY_IDS):
        raise EvaluationError("baseline v0.1 scorecard lacks one of the six primary episodes")
    return scorecard, by_id


def _is_real_zero_call(
    score: Mapping[str, Any],
    artifact: Mapping[str, Any],
) -> bool:
    merged = {**score, **artifact}
    return v01._is_real_cheap_exit(merged)


def compute_thresholds(
    score_entries: Mapping[str, Mapping[str, Any]],
    validation_entries: Mapping[str, Mapping[str, Any]],
    baseline_entries: Mapping[str, Mapping[str, Any]],
) -> dict[str, Any]:
    primary = [score_entries[episode_id] for episode_id in PRIMARY_IDS]
    decisive_count = sum(bool(entry["decisive_key_roles_present"]) for entry in primary)
    must_read_count = sum(entry["scores"]["must_read_file_recall"] >= 3 for entry in primary)
    relation_count = sum(entry["scores"]["causal_decisive_relation"] >= 3 for entry in primary)
    verification_count = sum(entry["scores"]["verification_usefulness"] >= 3 for entry in primary)
    targeted_improvements = [
        episode_id
        for episode_id in IMPROVEMENT_TARGET_IDS
        if score_entries[episode_id]["scores"]["verification_usefulness"] >= 3
        and baseline_entries[episode_id]["scores"]["verification_usefulness"] < 3
    ]
    regressions: list[dict[str, Any]] = []
    for episode_id in PRIMARY_IDS:
        before_scores = baseline_entries[episode_id]["scores"]
        after_scores = score_entries[episode_id]["scores"]
        for dimension in SCORE_DIMENSIONS:
            before = before_scores[dimension]
            after = after_scores[dimension]
            if before >= 3 and after < before:
                regressions.append(
                    {
                        "episode_id": episode_id,
                        "dimension": dimension,
                        "v01": before,
                        "v02": after,
                    }
                )
    zero_calls = [
        episode_id
        for episode_id in ZERO_CALL_BASELINE_IDS
        if _is_real_zero_call(score_entries[episode_id], validation_entries[episode_id])
    ]
    major_claims = sum(len(entry["major_unsupported_claims"]) for entry in primary)
    absence = sum(len(entry["absence_claims_from_incomplete_scope"]) for entry in primary)
    clipping = sum(
        len(entry["clipped_before_known_task_match_without_partial_window"])
        for entry in primary
    )
    false_exact = sum(
        len(entry["false_exact_or_unrelated_verification_anchors"]) for entry in primary
    )
    unsupported_verify = sum(
        len(entry["unsupported_verification_effects_or_invented_commands"])
        for entry in primary
    )
    artifact_failures = [
        episode_id
        for episode_id, entry in validation_entries.items()
        if not entry.get("artifact_valid")
    ]
    model_calls = sum(
        int(entry.get("provider", {}).get("calls", 0))
        for entry in validation_entries.values()
    )
    return {
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
        "targeted_verification_improvement": {
            "count": len(targeted_improvements),
            "target": 1,
            "denominator": 3,
            "episodes": targeted_improvements,
            "passed": bool(targeted_improvements),
        },
        "no_v01_score_regression_at_or_above_3": {
            "count": len(regressions),
            "target": 0,
            "regressions": regressions,
            "passed": not regressions,
        },
        "useful_zero_call_same_six": {
            "count": len(zero_calls),
            "target": 1,
            "denominator": 1,
            "episodes": zero_calls,
            "passed": len(zero_calls) >= 1,
        },
        "major_unsupported_claims": {
            "count": major_claims,
            "target": 0,
            "passed": major_claims == 0,
        },
        "false_exact_or_unrelated_verification_anchors": {
            "count": false_exact,
            "target": 0,
            "passed": false_exact == 0,
        },
        "unsupported_verification_effects_or_invented_commands": {
            "count": unsupported_verify,
            "target": 0,
            "passed": unsupported_verify == 0,
        },
        "clipped_before_known_match_without_partial_window": {
            "count": clipping,
            "target": 0,
            "passed": clipping == 0,
        },
        "absence_claim_from_incomplete_scope": {
            "count": absence,
            "target": 0,
            "passed": absence == 0,
        },
        "zero_provider_calls": {
            "count": model_calls,
            "target": 0,
            "passed": model_calls == 0,
        },
    }


def build_gold_loss_ledger(
    score_entries: Mapping[str, Mapping[str, Any]],
    validation_entries: Mapping[str, Mapping[str, Any]],
) -> dict[str, Any]:
    entries: list[dict[str, Any]] = []
    for spec in PRIMARY_EPISODES:
        episode_id = spec.episode_id
        score = score_entries[episode_id]
        artifact = validation_entries[episode_id]
        trace = artifact.get("trace")
        if not isinstance(trace, dict):
            raise EvaluationError(f"{episode_id}: validated raw trace metadata is missing")
        candidate_ids = set(trace.get("candidate_ids", []))
        selected_ids = set(trace.get("selected_anchor_ids", []))
        candidate_id = score.get("gold_candidate_id")
        anchor_id = score.get("gold_anchor_id")
        disposition = score["gold_loss_stage"]
        if candidate_id is not None and candidate_id not in candidate_ids:
            raise EvaluationError(f"{episode_id}: gold_candidate_id is absent from raw trace")
        if anchor_id is not None and anchor_id not in selected_ids:
            raise EvaluationError(f"{episode_id}: gold_anchor_id is absent from selected anchors")
        if disposition in {"present_before_ranking", "dropped_during_ranking"} and candidate_id is None:
            raise EvaluationError(f"{episode_id}: {disposition} requires gold_candidate_id")
        if disposition == "dropped_during_ranking" and anchor_id is not None:
            raise EvaluationError(f"{episode_id}: dropped gold cannot name a selected anchor")
        if disposition == "clipped_during_source_retention" and (
            candidate_id is None or anchor_id is None
        ):
            raise EvaluationError(
                f"{episode_id}: clipped retention requires candidate and selected anchor IDs"
            )
        if disposition == "never_generated" and (candidate_id is not None or anchor_id is not None):
            raise EvaluationError(f"{episode_id}: never-generated gold cannot name raw identities")
        raw_sha = trace.get("sha256")
        raw_path = trace.get("json_path")
        if not isinstance(raw_sha, str) or not isinstance(raw_path, str):
            raise EvaluationError(f"{episode_id}: raw trace path/hash is missing")
        entries.append(
            {
                "episode_id": episode_id,
                "evaluation_scope": "primary_regression",
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
        "kind": "task_lens_v02_evaluation_gold_loss_ledger",
        "created_at": utc_now(),
        "classification_source": "development_gold",
        "production_trace_mutated": False,
        "entries": entries,
    }


def _evaluation_seal(evaluation_dir: Path, names: Sequence[str]) -> dict[str, Any]:
    return {
        "version": 1,
        "kind": "task_lens_v02_evaluation_seal",
        "sealed_at": utc_now(),
        "state": "sealed_known_development_evaluation",
        "artifacts": [
            {
                "path": name,
                "bytes": (evaluation_dir / name).stat().st_size,
                "sha256": sha256_file(evaluation_dir / name),
            }
            for name in names
        ],
    }


def evaluate(root: Path, scores_path: Path) -> dict[str, Any]:
    validation = validate_artifacts(root)
    if not validation["passed"]:
        first = validation["errors"][0] if validation["errors"] else "unknown validation failure"
        raise EvaluationError(f"artifact validation failed: {first}")
    scores = load_scores(scores_path)
    score_by_id = {entry["id"]: entry for entry in scores["episodes"]}
    validation_by_id = {entry["id"]: entry for entry in validation["episodes"]}
    _, baseline_by_id = _baseline_scorecard(root)
    thresholds = compute_thresholds(score_by_id, validation_by_id, baseline_by_id)
    passed = all(value["passed"] for value in thresholds.values())
    evaluation_dir = root / EVALUATION_DIR_NAME
    validation_path = evaluation_dir / "ARTIFACT_VALIDATION.json"
    ledger_path = evaluation_dir / "GOLD_LOSS_LEDGER.json"
    write_json(validation_path, validation)
    ledger = build_gold_loss_ledger(score_by_id, validation_by_id)
    write_json(ledger_path, ledger)
    episodes: list[dict[str, Any]] = []
    for spec in PRIMARY_EPISODES:
        score = score_by_id[spec.episode_id]
        artifact = validation_by_id[spec.episode_id]
        baseline = baseline_by_id[spec.episode_id]
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
                "baseline_v01_scores": baseline["scores"],
                "baseline_v01_provider": baseline.get("provider", {}),
                "baseline_v01_wall_millis": baseline.get("wall_millis", 0),
                "baseline_v01_cheap_exit": baseline.get("cheap_exit", {}),
                "baseline_v01_offline": baseline.get("offline", False),
            }
        )
    scorecard = {
        "version": 1,
        "kind": "task_lens_v02_development_scorecard",
        "rendered_at": utc_now(),
        "known_development_only": True,
        "fresh_generalization": False,
        "opaque_average_computed": False,
        "primary_episode_ids": list(PRIMARY_IDS),
        "dimensions": list(SCORE_DIMENSIONS),
        "baseline_v01_scorecard_sha256": V01_PINNED_IDENTITIES["scorecard_sha256"],
        "episodes": episodes,
        "thresholds": thresholds,
        "gold_loss_ledger_sha256": sha256_file(ledger_path),
        "development_target_passed": passed,
        "authorization": (
            "Passing authorizes preparation of one untouched holdout only; it does not start it or authorize product integration."
            if passed
            else "The bounded verification redesign did not pass; stop tuning on the known set."
        ),
        "recommended_next_experiment": scores["recommended_next_experiment"].strip(),
    }
    scorecard_path = evaluation_dir / "SCORECARD.json"
    write_json(scorecard_path, scorecard)
    result = {
        "version": 1,
        "kind": "task_lens_v02_development_evaluation",
        "evaluated_at": utc_now(),
        "outcome": "passed" if passed else "partial",
        "known_development_only": True,
        "fresh_generalization": False,
        "product_integration_authorized": False,
        "new_untouched_holdout_preparation_authorized": passed,
        "new_untouched_holdout_started": False,
        "opaque_average_computed": False,
        "episodes_evaluated": 6,
        "resource_totals": validation["resource_totals"],
        "thresholds": thresholds,
        "scorecard_sha256": sha256_file(scorecard_path),
        "artifact_validation_sha256": sha256_file(validation_path),
        "gold_loss_ledger_sha256": sha256_file(ledger_path),
        "baseline_v01_manifest_sha256": sha256_file(root / BASELINE_MANIFEST_NAME),
        "baseline_v01_evaluation_seal_sha256": V01_PINNED_IDENTITIES[
            "evaluation_seal_sha256"
        ],
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
    seal = _evaluation_seal(evaluation_dir, sealed_names)
    seal_path = evaluation_dir / "EVALUATION_SEAL.json"
    write_json(seal_path, seal)
    write_text(
        evaluation_dir / "EVALUATION_SEAL.sha256",
        f"{sha256_file(seal_path)}  EVALUATION_SEAL.json",
    )
    return {
        "validation": validation,
        "gold_loss_ledger": ledger,
        "scorecard": scorecard,
        "result": result,
        "evaluation_seal": seal,
    }


def verify_evaluation_seal(root: Path) -> dict[str, Any]:
    evaluation_dir = root / EVALUATION_DIR_NAME
    seal_path = evaluation_dir / "EVALUATION_SEAL.json"
    _verify_manifest_sidecar(
        evaluation_dir,
        "EVALUATION_SEAL.json",
        "EVALUATION_SEAL.sha256",
    )
    seal = read_json(seal_path)
    if seal.get("kind") != "task_lens_v02_evaluation_seal":
        raise EvaluationError("evaluation seal kind is stale or not v0.2")
    artifacts = seal.get("artifacts")
    if not isinstance(artifacts, list):
        raise EvaluationError("evaluation seal artifact list is missing")
    expected_names = {
        "ARTIFACT_VALIDATION.json",
        "GOLD_LOSS_LEDGER.json",
        "SCORECARD.json",
        "EVALUATION_RESULT.json",
    }
    by_name = {
        entry.get("path"): entry
        for entry in artifacts
        if isinstance(entry, dict) and isinstance(entry.get("path"), str)
    }
    if set(by_name) != expected_names or len(artifacts) != len(expected_names):
        raise EvaluationError("evaluation-v02 seal artifact set is incomplete")
    for name, entry in by_name.items():
        path = evaluation_dir / name
        if sha256_file(path) != entry.get("sha256") or path.stat().st_size != entry.get("bytes"):
            raise EvaluationError(f"evaluation-v02 seal mismatch for {name}")
    result = read_json(evaluation_dir / "EVALUATION_RESULT.json")
    scorecard = read_json(evaluation_dir / "SCORECARD.json")
    validation = read_json(evaluation_dir / "ARTIFACT_VALIDATION.json")
    ledger = read_json(evaluation_dir / "GOLD_LOSS_LEDGER.json")
    kind_checks = (
        (result, "task_lens_v02_development_evaluation"),
        (scorecard, "task_lens_v02_development_scorecard"),
        (validation, "task_lens_v02_artifact_validation"),
        (ledger, "task_lens_v02_evaluation_gold_loss_ledger"),
    )
    if any(document.get("kind") != expected for document, expected in kind_checks):
        raise EvaluationError("evaluation-v02 contains a stale v0.1 or invalid artifact kind")
    if result.get("episodes_evaluated") != 6:
        raise EvaluationError("evaluation-v02 did not score exactly six episodes")
    if scorecard.get("primary_episode_ids") != list(PRIMARY_IDS):
        raise EvaluationError("evaluation-v02 scorecard episode set/order mismatch")
    bindings = (
        ("scorecard_sha256", "SCORECARD.json"),
        ("artifact_validation_sha256", "ARTIFACT_VALIDATION.json"),
        ("gold_loss_ledger_sha256", "GOLD_LOSS_LEDGER.json"),
    )
    for field_name, name in bindings:
        if result.get(field_name) != sha256_file(evaluation_dir / name):
            raise EvaluationError(f"EVALUATION_RESULT.json does not bind {name}")
    if scorecard.get("gold_loss_ledger_sha256") != sha256_file(
        evaluation_dir / "GOLD_LOSS_LEDGER.json"
    ):
        raise EvaluationError("SCORECARD.json does not bind GOLD_LOSS_LEDGER.json")
    entries = ledger.get("entries")
    if not isinstance(entries, list) or len(entries) != 6:
        raise EvaluationError("v0.2 gold-loss ledger must contain exactly six episodes")
    for entry in entries:
        if not isinstance(entry, dict) or entry.get("production_trace_mutated") is not False:
            raise EvaluationError("v0.2 gold-loss ledger contains an invalid entry")
        raw_path = entry.get("raw_retrieval_trace_path")
        raw_sha = entry.get("raw_retrieval_trace_sha256")
        if not isinstance(raw_path, str) or not isinstance(raw_sha, str):
            raise EvaluationError("v0.2 gold-loss ledger lacks a raw trace binding")
        target = root / _safe_relative_path(raw_path, evaluation_dir / "GOLD_LOSS_LEDGER.json")
        if sha256_file(target) != raw_sha:
            raise EvaluationError(f"v0.2 gold-loss raw trace mismatch: {raw_path}")
        if read_json(target).get("gold_assessment") is not None:
            raise EvaluationError(f"production trace contains development gold: {raw_path}")
    baseline = verify_baseline(root)
    if result.get("baseline_v01_manifest_sha256") != baseline["manifest_sha256"]:
        raise EvaluationError("evaluation-v02 does not bind the verified v0.1 baseline manifest")
    if (
        result.get("baseline_v01_evaluation_seal_sha256")
        != V01_PINNED_IDENTITIES["evaluation_seal_sha256"]
    ):
        raise EvaluationError("evaluation-v02 does not bind the pinned v0.1 evaluation")
    if (
        scorecard.get("baseline_v01_scorecard_sha256")
        != V01_PINNED_IDENTITIES["scorecard_sha256"]
    ):
        raise EvaluationError("evaluation-v02 scorecard does not bind the pinned v0.1 scorecard")
    return {
        "seal": seal,
        "result": result,
        "scorecard": scorecard,
        "validation": validation,
        "ledger": ledger,
    }


def _markdown_table(headers: Sequence[str], rows: Iterable[Sequence[Any]]) -> str:
    def clean(value: Any) -> str:
        return str(value).replace("|", "\\|").replace("\n", " ")

    lines = ["| " + " | ".join(clean(item) for item in headers) + " |"]
    lines.append("| " + " | ".join("---" for _ in headers) + " |")
    lines.extend("| " + " | ".join(clean(item) for item in row) + " |" for row in rows)
    return "\n".join(lines)


def _pass(value: bool) -> str:
    return "PASS" if value else "FAIL"


def _main_blocker(result: Mapping[str, Any]) -> str:
    labels = (
        ("artifact_integrity", "sealed artifact integrity failed"),
        ("verification_usefulness_at_least_3", "verification usefulness remains below 4/6"),
        ("targeted_verification_improvement", "none of the three declared weak verification episodes reached 3"),
        ("no_v01_score_regression_at_or_above_3", "a v0.1 score at or above 3 regressed"),
        ("false_exact_or_unrelated_verification_anchors", "a false-exact or unrelated verification anchor was scored"),
        ("unsupported_verification_effects_or_invented_commands", "verification overclaimed an effect or command"),
        ("useful_zero_call_same_six", "the same-six zero-call result regressed"),
        ("decisive_key_roles_present", "decisive key-role coverage regressed"),
        ("must_read_file_recall_at_least_3", "must-read file recall regressed"),
        ("causal_decisive_relation_at_least_3", "causal decisive relation coverage regressed"),
        ("major_unsupported_claims", "major unsupported claims remain"),
        ("clipped_before_known_match_without_partial_window", "unmarked source clipping remains"),
        ("absence_claim_from_incomplete_scope", "an incomplete-scope absence claim escaped"),
        ("zero_provider_calls", "the scored run used provider calls"),
    )
    for name, label in labels:
        if not result["thresholds"][name]["passed"]:
            return label
    return "fresh generalization remains unmeasured"


def _validate_supervisor_text(text: str, recommendation: str) -> None:
    lines = text.splitlines()
    opening = lines[: len(SUPERVISOR_OPENING_LABELS)]
    if len(opening) != len(SUPERVISOR_OPENING_LABELS):
        raise EvaluationError("v0.2 supervisor opening is incomplete")
    for index, (line, label) in enumerate(zip(opening, SUPERVISOR_OPENING_LABELS), 1):
        if not line.startswith(label + " "):
            raise EvaluationError(f"v0.2 supervisor opening line {index} must start with {label!r}")
    if len(lines) <= 10 or lines[10] != "":
        raise EvaluationError("v0.2 supervisor opening must be ten lines followed by a blank")
    expected = f"{SUPERVISOR_OPENING_LABELS[-1]} {recommendation}"
    matches = [line for line in lines if line.startswith(SUPERVISOR_OPENING_LABELS[-1] + " ")]
    if matches != [expected]:
        raise EvaluationError("v0.2 supervisor must contain exactly one next action")
    if "# Task Lens v0.2 supervisor report" not in text:
        raise EvaluationError("stale supervisor: v0.2 title is missing")
    if "# Task Lens v0.1 supervisor report" in text:
        raise EvaluationError("stale v0.1 supervisor report content is forbidden")
    for episode_id in PRIMARY_IDS:
        if f"### {episode_id}\n" not in text:
            raise EvaluationError(f"v0.2 supervisor omits {episode_id}")


def _validate_rendered_reports(
    root: Path,
    files: Mapping[Path, str],
    recommendation: str,
) -> None:
    names = {path.relative_to(root).as_posix() for path in files}
    if names != REQUIRED_RENDERED_REPORTS:
        raise EvaluationError(
            "rendered v0.2 report set mismatch; "
            f"missing={sorted(REQUIRED_RENDERED_REPORTS - names)}, "
            f"extra={sorted(names - REQUIRED_RENDERED_REPORTS)}"
        )
    _validate_supervisor_text(files[root / "SUPERVISOR_REPORT.md"], recommendation)
    review = files[root / "review" / "index.html"]
    lowered = review.lower()
    if "<script" in lowered or "javascript:" in lowered:
        raise EvaluationError("static v0.2 review contains executable script content")
    for episode_id in PRIMARY_IDS:
        if f'id="{episode_id}"' not in review:
            raise EvaluationError(f"static v0.2 review omits {episode_id}")
    for name in REQUIRED_RENDERED_REPORTS - {"review/index.html"}:
        if name not in review:
            raise EvaluationError(f"static v0.2 review has no link for {name}")


def render_reports(root: Path) -> dict[str, str]:
    sealed = verify_evaluation_seal(root)
    result = sealed["result"]
    scorecard = sealed["scorecard"]
    episodes = {entry["id"]: entry for entry in scorecard["episodes"]}
    recommendation = result["recommended_next_experiment"]
    thresholds = result["thresholds"]

    regression_rows = [
        (
            episode_id,
            episodes[episode_id]["scores"]["decisive_anchor_recall"],
            episodes[episode_id]["scores"]["must_read_file_recall"],
            episodes[episode_id]["scores"]["causal_decisive_relation"],
            episodes[episode_id]["scores"]["verification_usefulness"],
            episodes[episode_id]["provider"].get("calls", 0),
        )
        for episode_id in PRIMARY_IDS
    ]
    regression = """# Task Lens v0.2 development regression

This scores exactly six known development episodes. It is not a fresh holdout and no opaque average is computed.

""" + _markdown_table(
        ("episode", "decisive", "must-read", "relation", "verification", "calls"),
        regression_rows,
    ) + "\n\n## Fixed gates\n\n" + _markdown_table(
        ("gate", "observed", "target", "result"),
        (
            (name, gate.get("count", "all"), gate.get("target", "pass"), _pass(gate["passed"]))
            for name, gate in thresholds.items()
        ),
    ) + "\n"

    comparison_rows: list[tuple[Any, ...]] = []
    for episode_id in PRIMARY_IDS:
        entry = episodes[episode_id]
        for dimension in SCORE_DIMENSIONS:
            before = entry["baseline_v01_scores"][dimension]
            after = entry["scores"][dimension]
            comparison_rows.append((episode_id, dimension, before, after, after - before))
    comparison = """# Task Lens v0.1 to v0.2 comparison

The v0.1 baseline is a pinned read-only allowlisted closure. Scores remain separate; no aggregate is calculated.

""" + _markdown_table(
        ("episode", "dimension", "v0.1", "v0.2", "delta"),
        comparison_rows,
    ) + "\n"

    verification_rows = [
        (
            episode_id,
            episodes[episode_id]["baseline_v01_scores"]["verification_usefulness"],
            episodes[episode_id]["scores"]["verification_usefulness"],
            "; ".join(
                episodes[episode_id]["false_exact_or_unrelated_verification_anchors"]
            )
            or "none",
            "; ".join(
                episodes[episode_id]["unsupported_verification_effects_or_invented_commands"]
            )
            or "none",
        )
        for episode_id in PRIMARY_IDS
    ]
    verification = """# Task Lens v0.2 verification binding

Exact authority is scored separately from lexical proximity. Proposed locations and missing evidence never count as historical verification.

""" + _markdown_table(
        ("episode", "v0.1", "v0.2", "false exact/unrelated", "unsupported effect/command"),
        verification_rows,
    ) + "\n"

    zero_rows = [
        (
            episode_id,
            episode_id in ZERO_CALL_BASELINE_IDS,
            entry["cheap_exit"].get("eligible"),
            entry["cheap_exit"].get("route"),
            entry["provider"].get("calls", 0),
            entry["status"].get("sufficient"),
            entry["useful"],
        )
        for episode_id, entry in episodes.items()
    ]
    cheap = """# Task Lens v0.2 same-six zero-call evaluation

The seventh v0.1 configuration calibration is deliberately excluded. The no-regression target is the one useful zero-call result among the same six primary episodes.

""" + _markdown_table(
        ("episode", "v0.1 target", "eligible", "route", "calls", "sufficient", "useful"),
        zero_rows,
    ) + "\n"

    scope_rows = [
        (
            episode_id,
            episodes[episode_id]["scores"]["source_scope_completeness"],
            len(
                episodes[episode_id][
                    "clipped_before_known_task_match_without_partial_window"
                ]
            ),
            len(episodes[episode_id]["absence_claims_from_incomplete_scope"]),
        )
        for episode_id in PRIMARY_IDS
    ]
    scope = "# Task Lens v0.2 source-scope audit\n\n" + _markdown_table(
        ("episode", "score", "unmarked clipping", "invalid absence claims"),
        scope_rows,
    ) + "\n"

    failure_sections = []
    for episode_id in PRIMARY_IDS:
        entry = episodes[episode_id]
        failure_sections.append(
            f"""## {episode_id}

- Gold classification: `{entry['gold_loss_stage']}` — {entry['gold_loss_detail']}
- Verification score: {entry['baseline_v01_scores']['verification_usefulness']} → {entry['scores']['verification_usefulness']}
- Notes: {'; '.join(entry['notes']) or 'none'}
"""
        )
    failures = """# Task Lens v0.2 retrieval and verification failures

Development gold is post-run only and is bound to immutable raw retrieval-trace hashes.

""" + "\n".join(failure_sections)

    decisive = thresholds["decisive_key_roles_present"]
    verification_gate = thresholds["verification_usefulness_at_least_3"]
    zero_gate = thresholds["useful_zero_call_same_six"]
    technical = (
        "passed: baseline, six final attempts, evaluation-v02, and seals validated"
        if thresholds["artifact_integrity"]["passed"]
        else "failed: baseline or final artifacts did not validate"
    )
    product = (
        "passed for known development only; fresh generalization remains unmeasured"
        if result["outcome"] == "passed"
        else "partial: at least one frozen known-development gate failed"
    )
    investment = (
        "authorize preparation of one untouched holdout only; do not start it or integrate"
        if result["new_untouched_holdout_preparation_authorized"]
        else "stop repeated tuning on the known set; do not start a holdout or integrate"
    )
    audit_sections = "\n".join(
        f"""### {episode_id}

- v0.1 → v0.2 verification: {episodes[episode_id]['baseline_v01_scores']['verification_usefulness']} → {episodes[episode_id]['scores']['verification_usefulness']}
- Decisive relation: {episodes[episode_id]['scores']['causal_decisive_relation']}
- False-exact/unrelated anchors: {len(episodes[episode_id]['false_exact_or_unrelated_verification_anchors'])}
- Unsupported effects/commands: {len(episodes[episode_id]['unsupported_verification_effects_or_invented_commands'])}
"""
        for episode_id in PRIMARY_IDS
    )
    supervisor = f"""outcome: {result['outcome']}
technical result: {technical}
product-development result: {product}
investment result: {investment}
episodes evaluated: 6 known development regressions; no calibration episode and no holdout
decisive-anchor target result: {_pass(decisive['passed']).lower()} ({decisive['count']}/6; target 5)
verification target result: {_pass(verification_gate['passed']).lower()} ({verification_gate['count']}/6; target 4)
zero-call target result: {_pass(zero_gate['passed']).lower()} ({zero_gate['count']}/1 same-six result; target 1)
single main blocker: {_main_blocker(result)}
exactly one recommended next step: {recommendation}

# Task Lens v0.2 supervisor report

This is a known-development verification-binding result. It does not measure fresh generalization, does not start a holdout, and never authorizes product integration.

## Root-cause audit

{audit_sections}

## Verification decision

Verification usefulness reached {verification_gate['count']}/6; the targeted-improvement gate is {_pass(thresholds['targeted_verification_improvement']['passed'])}; false-exact count is {thresholds['false_exact_or_unrelated_verification_anchors']['count']}.

## v0.1 non-regression

Scores previously at or above 3 produced {thresholds['no_v01_score_regression_at_or_above_3']['count']} regressions. Decisive, must-read, causal relation, safety, cost, and same-six zero-call gates remain separate.

## Model/resource accounting

The scored run used {result['resource_totals']['model_calls']} provider calls, {result['resource_totals']['input_tokens']} input tokens, {result['resource_totals']['output_tokens']} output tokens, and no target-repository commands or tests.

## Product decision

Holdout preparation authorized: **{'yes' if result['new_untouched_holdout_preparation_authorized'] else 'no'}**. Holdout started: **no**. Product integration authorized: **no**.
"""

    product_findings = f"""# Task Lens v0.2 product findings

Outcome: **{result['outcome']}** on six known development episodes.

- Verification gate: {verification_gate['count']}/6 at score 3 or higher.
- Targeted weak-episode improvement: {thresholds['targeted_verification_improvement']['count']}/3.
- False-exact/unrelated verification anchors: {thresholds['false_exact_or_unrelated_verification_anchors']['count']}.
- Existing v0.1 scores at or above 3 that regressed: {thresholds['no_v01_score_regression_at_or_above_3']['count']}.
- Same-six useful zero-call preservation: {zero_gate['count']}/1.

Fresh generalization and Project Input Contract work are outside this iteration.
"""

    walkthrough = """# Task Lens v0.2 review walkthrough

1. Read `SUPERVISOR_REPORT.md` for the ten-line decision and one next action.
2. Read `VERIFICATION_BINDING.md` for exact-authority and overclaim checks.
3. Compare every separate v0.1/v0.2 score in `BASELINE_COMPARISON.md`.
4. Follow the sealed evaluation and episode links from `review/index.html`.
"""

    report_links = "".join(
        f'<a href="../{html.escape(name)}">{html.escape(name)}</a>'
        for name in sorted(REQUIRED_RENDERED_REPORTS - {"review/index.html"})
    )
    cards = "".join(
        (
            f'<article id="{html.escape(episode_id)}"><h2>{html.escape(episode_id)}</h2>'
            f"<p>Verification {entry['baseline_v01_scores']['verification_usefulness']} "
            f"→ {entry['scores']['verification_usefulness']}; "
            f"calls {entry['provider'].get('calls', 0)}.</p>"
            f'<a href="../{html.escape(entry["attempt_dir"])}/SEALED.json">SEALED.json</a>'
            f'<a href="../{html.escape(entry["run_dir"])}/retrieval_trace.json">retrieval_trace.json</a>'
            "</article>"
        )
        for episode_id, entry in episodes.items()
    )
    review = f"""<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Task Lens v0.2 review</title>
<style>body{{font:16px/1.5 system-ui;max-width:1100px;margin:auto;padding:24px}}nav,article{{display:grid;gap:8px;margin:16px 0;padding:16px;border:1px solid #bbb;border-radius:10px}}a{{overflow-wrap:anywhere}}</style></head>
<body><h1>Task Lens v0.2 known-development review</h1><p>Outcome: <strong>{html.escape(result['outcome'])}</strong>. No fresh holdout or integration claim.</p><nav>{report_links}</nav>{cards}</body></html>
"""
    files = {
        root / "DEV_REGRESSION.md": regression,
        root / "BASELINE_COMPARISON.md": comparison,
        root / "VERIFICATION_BINDING.md": verification,
        root / "CHEAP_EXIT_EVALUATION.md": cheap,
        root / "SOURCE_SCOPE_AUDIT.md": scope,
        root / "RETRIEVAL_FAILURES.md": failures,
        root / "SUPERVISOR_REPORT.md": supervisor,
        root / "PRODUCT_FINDINGS.md": product_findings,
        root / "WALKTHROUGH.md": walkthrough,
        root / "review" / "index.html": review,
    }
    _validate_rendered_reports(root, files, recommendation)
    for path, content in files.items():
        write_text(path, content)
    seal_path = root / EVALUATION_DIR_NAME / "EVALUATION_SEAL.json"
    baseline_manifest = root / BASELINE_MANIFEST_NAME
    report_entries = [
        {
            "path": path.relative_to(root).as_posix(),
            "bytes": path.stat().st_size,
            "sha256": sha256_file(path),
        }
        for path in sorted(files)
    ]
    report_seal = {
        "version": 1,
        "kind": "task_lens_v02_report_seal",
        "sealed_at": utc_now(),
        "evaluation_seal": {
            "path": f"{EVALUATION_DIR_NAME}/EVALUATION_SEAL.json",
            "sha256": sha256_file(seal_path),
        },
        "baseline_manifest": {
            "path": BASELINE_MANIFEST_NAME,
            "sha256": sha256_file(baseline_manifest),
        },
        "reports": report_entries,
    }
    write_json(root / REPORT_SEAL_NAME, report_seal)
    write_text(
        root / REPORT_SEAL_SIDECAR,
        f"{sha256_file(root / REPORT_SEAL_NAME)}  {REPORT_SEAL_NAME}",
    )
    verify_report_seal(root)
    return {
        path.relative_to(root).as_posix(): str(path)
        for path in (*files, root / REPORT_SEAL_NAME, root / REPORT_SEAL_SIDECAR)
    }


def verify_report_seal(root: Path) -> dict[str, Any]:
    sealed = verify_evaluation_seal(root)
    baseline = verify_baseline(root)
    _verify_manifest_sidecar(root, REPORT_SEAL_NAME, REPORT_SEAL_SIDECAR)
    seal = read_json(root / REPORT_SEAL_NAME)
    if seal.get("kind") != "task_lens_v02_report_seal":
        raise EvaluationError("report seal kind is stale or not v0.2")
    evaluation_binding = seal.get("evaluation_seal")
    baseline_binding = seal.get("baseline_manifest")
    if not isinstance(evaluation_binding, dict) or not isinstance(baseline_binding, dict):
        raise EvaluationError("report seal lacks evaluation/baseline bindings")
    if evaluation_binding != {
        "path": f"{EVALUATION_DIR_NAME}/EVALUATION_SEAL.json",
        "sha256": sha256_file(root / EVALUATION_DIR_NAME / "EVALUATION_SEAL.json"),
    }:
        raise EvaluationError("REPORT_SEAL.json does not bind evaluation-v02")
    if baseline_binding != {
        "path": BASELINE_MANIFEST_NAME,
        "sha256": sha256_file(root / BASELINE_MANIFEST_NAME),
    }:
        raise EvaluationError("REPORT_SEAL.json does not bind the v0.1 baseline manifest")
    reports = seal.get("reports")
    if not isinstance(reports, list):
        raise EvaluationError("REPORT_SEAL.json has no report inventory")
    by_path = {
        entry.get("path"): entry
        for entry in reports
        if isinstance(entry, dict) and isinstance(entry.get("path"), str)
    }
    if set(by_path) != REQUIRED_RENDERED_REPORTS or len(reports) != len(REQUIRED_RENDERED_REPORTS):
        raise EvaluationError("REPORT_SEAL.json report set mismatch")
    for relative, entry in by_path.items():
        path = root / _safe_relative_path(relative, root / REPORT_SEAL_NAME)
        if sha256_file(path) != entry.get("sha256") or path.stat().st_size != entry.get("bytes"):
            raise EvaluationError(f"REPORT_SEAL.json mismatch for {relative}")
    supervisor_path = root / "SUPERVISOR_REPORT.md"
    if sha256_file(supervisor_path) == V01_PINNED_IDENTITIES["supervisor_report_sha256"]:
        raise EvaluationError("stale v0.1 supervisor report was returned as v0.2")
    _validate_supervisor_text(
        supervisor_path.read_text(encoding="utf-8"),
        sealed["result"]["recommended_next_experiment"],
    )
    return {
        "state": "verified",
        "kind": seal["kind"],
        "report_seal_sha256": sha256_file(root / REPORT_SEAL_NAME),
        "evaluation_seal_sha256": evaluation_binding["sha256"],
        "baseline_manifest_sha256": baseline["manifest_sha256"],
        "reports_verified": len(by_path),
    }


def command_freeze_baseline(args: argparse.Namespace) -> dict[str, Any]:
    return freeze_baseline(Path(args.root).resolve(), Path(args.source).resolve())


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


def command_verify_report(args: argparse.Namespace) -> dict[str, Any]:
    return verify_report_seal(Path(args.root).resolve())


def command_all(args: argparse.Namespace) -> dict[str, Any]:
    root = Path(args.root).resolve()
    evaluated = evaluate(root, Path(args.scores).resolve())
    reports = render_reports(root)
    verified = verify_report_seal(root)
    return {"result": evaluated["result"], "reports": reports, "report_verification": verified}


def parser() -> argparse.ArgumentParser:
    result = argparse.ArgumentParser(description=__doc__)
    commands = result.add_subparsers(dest="command", required=True)

    freeze = commands.add_parser(
        "freeze-baseline",
        help="copy and freeze the allowlisted v0.1 closure",
    )
    freeze.add_argument("--root", default="tmp/task-lens-v02")
    freeze.add_argument("--source", default="tmp/task-lens-v01")
    freeze.set_defaults(func=command_freeze_baseline)

    prepare_parser = commands.add_parser(
        "prepare",
        help="verify baseline-v01 and write evaluation-v02 inputs",
    )
    prepare_parser.add_argument("--root", default="tmp/task-lens-v02")
    prepare_parser.set_defaults(func=command_prepare)

    validate_parser = commands.add_parser(
        "validate",
        help="validate baseline, six final attempts, frozen budgets, and seals",
    )
    validate_parser.add_argument("--root", default="tmp/task-lens-v02")
    validate_parser.add_argument("--output")
    validate_parser.set_defaults(func=command_validate)

    evaluate_parser = commands.add_parser(
        "evaluate",
        help="validate artifacts and compute the fixed v0.2 gates",
    )
    evaluate_parser.add_argument("--root", default="tmp/task-lens-v02")
    evaluate_parser.add_argument("--scores", required=True)
    evaluate_parser.set_defaults(func=command_evaluate)

    report_parser = commands.add_parser(
        "report",
        help="render and seal the v0.2 report bundle",
    )
    report_parser.add_argument("--root", default="tmp/task-lens-v02")
    report_parser.set_defaults(func=command_report)

    verify_parser = commands.add_parser(
        "verify-report",
        help="replay the evaluation, baseline, and report seals",
    )
    verify_parser.add_argument("--root", default="tmp/task-lens-v02")
    verify_parser.set_defaults(func=command_verify_report)

    all_parser = commands.add_parser(
        "all",
        help="validate, evaluate, render, seal, and verify reports",
    )
    all_parser.add_argument("--root", default="tmp/task-lens-v02")
    all_parser.add_argument("--scores", required=True)
    all_parser.set_defaults(func=command_all)
    return result


def main(argv: Sequence[str] | None = None) -> int:
    try:
        args = parser().parse_args(argv)
        value = args.func(args)
        print(json.dumps(value, indent=2, sort_keys=True, ensure_ascii=False))
        return 0
    except EvaluationError as exc:
        print(f"task-lens-v02-eval: error: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
