#!/usr/bin/env python3
"""Protocol harness for the Task Lens v0 development and frozen holdout.

The harness deliberately does not grade Task Lens prose.  It establishes the
experiment boundary, verifies repository provenance, runs the frozen binary
once per holdout episode, seals outputs, and only then unlocks a supervisor
scorecard against historical gold.
"""

from __future__ import annotations

import argparse
import datetime as dt
import hashlib
import html
import json
import os
import re
import secrets
import shutil
import stat
import subprocess
import sys
import tempfile
import time
from pathlib import Path
from typing import Any, Iterable, Sequence


VERSION = 1
EPISODE_ID_RE = re.compile(r"^[a-z0-9][a-z0-9_-]*$")
REVISION_RE = re.compile(r"^[0-9a-f]{40}$")
SHA256_RE = re.compile(r"^[0-9a-f]{64}$")
SECRET_RE = re.compile(
    rb"(?:(?<![A-Za-z0-9])sk-[A-Za-z0-9_-]{16,}|gh[pousr]_[A-Za-z0-9]{20,}|AKIA[0-9A-Z]{16})"
)

ARTIFACT_NAMES = (
    "task_investigation_bundle.json",
    "task_investigation_attempt.json",
    "task_investigation.json",
    "task_investigation_status.json",
)

SCORE_DIMENSIONS = (
    "task_interpretation",
    "subsystem_localization",
    "must_read_file_recall",
    "key_symbol_recall",
    "causal_evidence_join_quality",
    "reproduction_observation_usefulness",
    "verification_usefulness",
    "uncertainty_calibration",
    "linked_guide_object_relevance",
    "cost_appropriateness",
)

TASK_KINDS = {"bug", "feature", "extension", "configuration", "operational", "compatibility", "unknown"}
LOCALITIES = {"local_exact", "bounded_cross_file", "extension_contribution", "broad_dynamic"}
ANCHOR_ROLES = {
    "symptom_site", "public_or_cli_entry", "state_owner", "state_mutation",
    "configuration_source", "configuration_copy", "error_creation", "error_mapping",
    "integration_boundary", "representative_implementation", "generated_output",
    "reproduction_anchor", "verification_anchor", "documentation_contract",
}
SUPPORT_TYPES = {"locally_observed", "document_supported", "model_hypothesis", "unresolved"}
HYPOTHESIS_STATES = {"supported", "plausible", "unresolved"}
GUIDANCE_AUTHORITIES = {
    "task_provided", "repository_document", "repository_test_or_example",
    "repository_observation", "missing_evidence",
}
PROBE_ACTIONS = {
    "inspect_symbol", "resolve_reference", "compare_config_copies", "inspect_fixture",
    "inspect_sibling_implementation", "search_task_terms",
}
CANONICAL_SKIPPED_STAGES = (
    "architecture_synthesis", "generic_orientation", "guided_tour", "mechanism_opportunity",
    "paved_paths", "repository_study_map", "runtime_surface_discovery",
)
FORBIDDEN_GENERIC_ARTIFACTS = {
    "architecture_synthesis.json", "llm_bundle.json", "orientation_report.json",
}
BUDGET_LIMITS = {
    "initial_candidates": 40,
    "retained_anchors": 16,
    "evidence_files_considered": 12,
    "read_files": 12,
    "read_bytes": 128 * 1024,
    "retained_source_bytes": 128 * 1024,
    "gopls_queries": 12,
    "frontier_expansions": 2,
    "local_wall_millis": 10_000,
}

BENCHMARK_DEFECTS = (
    "wrong_captured_revision", "task_labelled_basename_leakage", "dependent_A_B_answers",
    "ceiling_effect_prompts", "excessive_generic_call_cost",
)
CONTRACT_SECTIONS = (
    "object_scope", "evidence_sources", "hypothesis_labels", "relation_evidence_joins",
    "reproduction_verification_authority", "replay_staleness", "user_facing_projection",
)
DEVELOPMENT_SECTIONS = (
    "changes_during_development", "generic_vs_fixture_only", "failures_shaping_contract",
)
PRODUCT_COMPARISON_SECTIONS = (
    "relevant_anchors", "irrelevant_generic_objects", "causal_pair_coverage",
    "reproduction_verification", "call_token_cost", "user_next_action",
)
PRODUCT_FINDING_QUESTIONS = (
    "benefiting_task_classes", "cheap_exit_task_classes", "evidence_joining_effect",
    "useful_guide_surfaces", "irrelevant_generic_artifacts", "production_justification",
    "most_important_remaining_gap",
)
FINAL_RECOMMENDATIONS = {
    "integrate Task Lens as an experimental product surface",
    "run one cross-repository task holdout",
    "stop/redesign task-conditioned investigation",
}

CHEAP_EXIT_EXPECTATIONS = "CHEAP_EXIT_EXPECTATIONS.json"
CHEAP_EXIT_EXPECTATIONS_SHA256 = "CHEAP_EXIT_EXPECTATIONS.sha256"

ATTEMPT_STATUS_STATES = {
    "accepted": ("accepted", True),
    "accepted_with_rejections": ("accepted_partial", True),
    "provider_failed": ("partial_local", True),
    "rejected": ("partial_local", True),
    "skipped_offline": ("partial_local", False),
    "skipped_insufficient_evidence": ("partial_local", False),
}

GIT_ENV_PREFIX = "GIT_"
SAFE_AMBIENT_GIT_ENV = {"GIT_PAGER"}
LEGACY_BENCHMARK_MARKERS = (
    "fuego-historical-benchmark-v0",
    "fuego_historical_benchmark_v0",
)


class HarnessError(RuntimeError):
    """A protocol or artifact invariant failed."""


def fail(message: str) -> None:
    raise HarnessError(message)


def utc_now() -> str:
    return dt.datetime.now(dt.timezone.utc).replace(microsecond=0).isoformat()


def canonical_json(value: Any) -> bytes:
    return (json.dumps(value, indent=2, sort_keys=True) + "\n").encode("utf-8")


def sha256_bytes(raw: bytes) -> str:
    return hashlib.sha256(raw).hexdigest()


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for block in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


def read_json(path: Path) -> Any:
    try:
        with path.open("r", encoding="utf-8") as handle:
            return json.load(handle)
    except (OSError, json.JSONDecodeError) as exc:
        fail(f"cannot read JSON {path}: {exc}")


def write_new(path: Path, raw: bytes, mode: int = 0o644) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    try:
        descriptor = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, mode)
    except FileExistsError:
        fail(f"refusing to overwrite existing artifact: {path}")
    with os.fdopen(descriptor, "wb") as handle:
        handle.write(raw)


def write_json_new(path: Path, value: Any, mode: int = 0o644) -> None:
    write_new(path, canonical_json(value), mode)


def replace_json(path: Path, value: Any) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    temporary = path.with_name(path.name + ".tmp")
    if temporary.exists():
        fail(f"stale temporary artifact blocks update: {temporary}")
    with temporary.open("xb") as handle:
        handle.write(canonical_json(value))
    os.replace(temporary, path)


def sanitized_env() -> dict[str, str]:
    return {key: value for key, value in os.environ.items() if not key.startswith(GIT_ENV_PREFIX)}


def reject_ambient_git_env() -> None:
    names = sorted(
        key
        for key in os.environ
        if key.startswith(GIT_ENV_PREFIX) and key not in SAFE_AMBIENT_GIT_ENV
    )
    if names:
        fail("ambient Git environment is forbidden: " + ", ".join(names))


def reject_holdout_gold_env() -> None:
    names = sorted(
        key
        for key in os.environ
        if os.environ[key]
        and ("HOLDOUT_GOLD" in key.upper() or key.upper().endswith("_GOLD_DIR"))
    )
    if names:
        fail("holdout gold environment is forbidden before sealing: " + ", ".join(names))


def run_command(
    argv: Sequence[str],
    *,
    cwd: Path | None = None,
    check: bool = True,
    text: bool = False,
    stdout: Any = subprocess.PIPE,
    stderr: Any = subprocess.PIPE,
) -> subprocess.CompletedProcess[Any]:
    result = subprocess.run(
        list(argv),
        cwd=str(cwd) if cwd else None,
        env=sanitized_env(),
        check=False,
        text=text,
        stdout=stdout,
        stderr=stderr,
    )
    if check and result.returncode != 0:
        error = result.stderr
        if isinstance(error, bytes):
            error = error.decode("utf-8", "replace")
        fail(f"command failed ({result.returncode}): {' '.join(argv)}: {(error or '').strip()}")
    return result


def git(repo: Path, *args: str, check: bool = True) -> subprocess.CompletedProcess[str]:
    return run_command(
        ("git", "-C", str(repo), *args),
        check=check,
        text=True,
    )


def git_output(repo: Path, *args: str) -> str:
    return git(repo, *args).stdout.strip()


def resolve_existing(path: str | Path, kind: str) -> Path:
    candidate = Path(path).expanduser().resolve()
    if kind == "file" and not candidate.is_file():
        fail(f"required file does not exist: {candidate}")
    if kind == "dir" and not candidate.is_dir():
        fail(f"required directory does not exist: {candidate}")
    return candidate


def validate_relative_path(value: str, field: str) -> str:
    path = Path(value)
    if not value or path.is_absolute() or ".." in path.parts or "." in path.parts:
        fail(f"{field} must be a normalized repository-relative path: {value!r}")
    return path.as_posix()


def validate_manifest(path: Path, phase: str) -> dict[str, Any]:
    value = read_json(path)
    if not isinstance(value, dict) or not isinstance(value.get("episodes"), list):
        fail(f"invalid {phase} manifest: expected an episodes array")
    expected_role = "development_set" if phase == "dev" else "frozen_holdout"
    if value.get("role") != expected_role:
        fail(f"invalid {phase} manifest role: expected {expected_role!r}")
    if phase == "dev" and value.get("may_use_historical_gold") is not True:
        fail("development manifest must explicitly allow historical development gold")
    if phase == "holdout" and value.get("may_use_historical_gold_before_seal") is not False:
        fail("holdout manifest must explicitly forbid historical gold before sealing")
    if not isinstance(value.get("repository"), str) or not value["repository"].strip():
        fail(f"invalid {phase} manifest repository")
    if not value["episodes"]:
        fail(f"invalid {phase} manifest: no episodes")

    seen: set[str] = set()
    normalized: list[dict[str, Any]] = []
    for index, raw in enumerate(value["episodes"]):
        if not isinstance(raw, dict):
            fail(f"invalid {phase} episode at index {index}")
        episode_id = raw.get("id")
        revision = raw.get("base_revision")
        if not isinstance(episode_id, str) or not EPISODE_ID_RE.fullmatch(episode_id):
            fail(f"invalid {phase} episode id at index {index}: {episode_id!r}")
        if episode_id in seen:
            fail(f"duplicate {phase} episode id: {episode_id}")
        seen.add(episode_id)
        if not isinstance(revision, str) or not REVISION_RE.fullmatch(revision):
            fail(f"invalid base revision for {episode_id}: {revision!r}")
        episode = dict(raw)
        for field in ("must_be_absent", "absent_paths"):
            if field in episode:
                if not isinstance(episode[field], list):
                    fail(f"{phase} episode {episode_id}: {field} must be an array")
                episode[field] = [
                    validate_relative_path(item, f"{episode_id}.{field}")
                    for item in episode[field]
                    if isinstance(item, str)
                ]
                if len(episode[field]) != len(raw[field]):
                    fail(f"{phase} episode {episode_id}: {field} contains a non-string")
        if phase == "holdout":
            task = episode.get("task")
            if not isinstance(task, str) or not task.strip() or len(task.encode("utf-8")) > 32 * 1024:
                fail(f"holdout episode {episode_id}: invalid prompt-safe task text")
        normalized.append(episode)
    value = dict(value)
    value["episodes"] = normalized
    return value


def episode_map(manifest: dict[str, Any]) -> dict[str, dict[str, Any]]:
    return {episode["id"]: episode for episode in manifest["episodes"]}


def ensure_git_repository(source_repo: Path) -> None:
    if git_output(source_repo, "rev-parse", "--is-inside-work-tree") != "true":
        fail(f"not a Git worktree: {source_repo}")


def reject_legacy_holdout_path(path: Path, label: str) -> None:
    lowered = str(path).lower()
    if "downloads" in (part.lower() for part in path.parts):
        fail(f"{label} must not come from Downloads during holdout: {path}")
    if any(marker in lowered for marker in LEGACY_BENCHMARK_MARKERS):
        fail(f"{label} must not come from the legacy benchmark during holdout: {path}")


def assert_no_holdout_gold(root: Path, allowed_gold: Path | None = None) -> None:
    """Reject recognizable holdout-gold payloads inside the active experiment root.

    Development material is intentionally excluded because the approved protocol
    permits historical development gold.  The check is name-based and is paired
    with a stronger construction rule: no pre-seal command accepts a gold path.
    """
    if (root / "evaluation").exists() and allowed_gold is None:
        fail("holdout_contaminated: evaluation/gold exists before holdout sealing")
    scan_roots = [root]
    freeze_manifest = root / "FREEZE_MANIFEST.json"
    if freeze_manifest.is_file():
        value = read_json(freeze_manifest)
        repository = value.get("implementation", {}).get("repository") if isinstance(value, dict) else None
        if isinstance(repository, str) and Path(repository).is_dir():
            scan_roots.append(Path(repository).resolve())
    seen: set[Path] = set()
    for scan_root in scan_roots:
        if scan_root in seen:
            continue
        seen.add(scan_root)
        for current, directories, files in os.walk(scan_root, followlinks=False):
            current_path = Path(current)
            directories[:] = [name for name in directories if name not in {".git", "node_modules"}]
            try:
                relative_root = current_path.relative_to(root)
                if relative_root.parts and relative_root.parts[0] == "dev":
                    directories[:] = []
                    continue
            except ValueError:
                pass
            for name in directories + files:
                path = current_path / name
                if allowed_gold is not None:
                    try:
                        path.resolve().relative_to(allowed_gold.resolve())
                        continue
                    except ValueError:
                        pass
                lowered = name.lower()
                suspicious = (
                    lowered == "gold"
                    or lowered.startswith("gold.")
                    or "holdout_gold" in lowered
                    or "holdout-gold" in lowered
                    or "supervisor_gold" in lowered
                    or "supervisor-gold" in lowered
                )
                if suspicious:
                    fail(f"holdout_contaminated: possible holdout gold is present at {path}")


def find_dev_task(tasks_dir: Path, episode_id: str) -> Path:
    candidates = (
        tasks_dir / f"{episode_id}.md",
        tasks_dir / episode_id / "task_packet.md",
        tasks_dir / "episodes" / episode_id / "task_packet.md",
    )
    matches = [path for path in candidates if path.is_file()]
    if len(matches) != 1:
        rendered = ", ".join(str(path) for path in candidates)
        fail(f"development task {episode_id}: expected exactly one task packet among {rendered}")
    return matches[0]


def archive_export(source_repo: Path, revision: str, destination: Path) -> None:
    destination.mkdir(parents=True, exist_ok=False)
    archive = subprocess.Popen(
        ("git", "-C", str(source_repo), "archive", "--format=tar", revision),
        env=sanitized_env(),
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )
    assert archive.stdout is not None
    extract = subprocess.run(
        ("tar", "-xf", "-", "-C", str(destination)),
        env=sanitized_env(),
        stdin=archive.stdout,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        check=False,
    )
    archive.stdout.close()
    archive_error = archive.stderr.read() if archive.stderr else b""
    archive_code = archive.wait()
    if archive_code != 0 or extract.returncode != 0:
        detail = (archive_error + extract.stderr).decode("utf-8", "replace").strip()
        fail(f"cannot create source-only export for {revision}: {detail}")
    if any(path.name == ".git" for path in destination.rglob(".git")):
        fail(f"source-only export unexpectedly contains Git metadata: {destination}")


def assert_worktree(
    source_repo: Path,
    repo: Path,
    expected_revision: str,
    expected_tree: str,
    absent_paths: Iterable[str],
) -> dict[str, Any]:
    if repo.name != "repo":
        fail(f"episode worktree basename is not neutral: {repo}")
    actual_revision = git_output(repo, "rev-parse", "HEAD")
    actual_tree = git_output(repo, "rev-parse", "HEAD^{tree}")
    if actual_revision != expected_revision:
        fail(f"worktree revision mismatch: expected {expected_revision}, got {actual_revision}")
    if actual_tree != expected_tree:
        fail(f"worktree tree mismatch: expected {expected_tree}, got {actual_tree}")
    symbolic = git(repo, "symbolic-ref", "-q", "HEAD", check=False)
    if symbolic.returncode == 0:
        fail(f"episode worktree is not detached: {repo}")
    if symbolic.returncode not in (1,):
        fail(f"cannot verify detached HEAD for {repo}: {symbolic.stderr.strip()}")
    dirty = git_output(repo, "status", "--porcelain", "--untracked-files=all")
    if dirty:
        fail(f"episode worktree is not clean: {repo}")
    checked = []
    for relative in absent_paths:
        normalized = validate_relative_path(relative, "absent path")
        candidate = repo / normalized
        if candidate.exists() or candidate.is_symlink():
            fail(f"later/forbidden path is present at the base revision: {normalized}")
        checked.append(normalized)
    source_tree = git_output(source_repo, "rev-parse", f"{expected_revision}^{{tree}}")
    if source_tree != expected_tree:
        fail(f"source repository changed while checking {expected_revision}")
    return {
        "captured_revision": actual_revision,
        "tree_hash": actual_tree,
        "detached_head": True,
        "clean": True,
        "neutral_basename": repo.name,
        "absent_paths_checked": sorted(checked),
    }


def later_added_paths(source_repo: Path, revision: str, tip: str) -> list[str]:
    ancestry = git(source_repo, "merge-base", "--is-ancestor", revision, tip, check=False)
    if ancestry.returncode != 0:
        fail(f"base revision {revision} is not an ancestor of comparison revision {tip}")
    result = run_command(
        (
            "git",
            "-C",
            str(source_repo),
            "diff",
            "--diff-filter=A",
            "--name-only",
            "-z",
            f"{revision}..{tip}",
        ),
        check=True,
    )
    return [item.decode("utf-8", "surrogateescape") for item in result.stdout.split(b"\0") if item]


def task_text_for_episode(
    phase: str,
    episode: dict[str, Any],
    tasks_dir: Path | None,
) -> tuple[str, str]:
    if phase == "holdout":
        return episode["task"].rstrip() + "\n", "HOLDOUT_SET.json"
    if tasks_dir is None:
        fail("--tasks-dir is required for development preparation")
    source = find_dev_task(tasks_dir, episode["id"])
    try:
        text = source.read_text(encoding="utf-8")
    except OSError as exc:
        fail(f"cannot read development task packet {source}: {exc}")
    if not text.strip() or len(text.encode("utf-8")) > 32 * 1024:
        fail(f"development task packet is empty or outside bounds: {source}")
    return text.rstrip() + "\n", str(source)


def empty_directory(path: Path, label: str) -> None:
    path.mkdir(parents=True, exist_ok=True)
    entries = list(path.iterdir())
    if entries:
        fail(f"{label} must be empty; found {entries[0]}")


def copy_new(source: Path, destination: Path, mode: int | None = None) -> None:
    if destination.exists() or destination.is_symlink():
        fail(f"refusing to overwrite existing artifact: {destination}")
    destination.parent.mkdir(parents=True, exist_ok=True)
    shutil.copyfile(source, destination)
    os.chmod(destination, mode if mode is not None else stat.S_IMODE(source.stat().st_mode))


def init_templates(root: Path) -> None:
    root.mkdir(parents=True, exist_ok=True)
    for directory in ("dev", "holdout", "screenshots", "review"):
        (root / directory).mkdir(exist_ok=True)

    report_path = root / "SUPERVISOR_REPORT.md"
    review_command = review_server_command(root)
    templates: dict[str, str] = {
        "PLAN.md": (
            "# Task Lens v0 plan\n\n"
            "1. Prepare and iterate on the development set.\n"
            "2. Seal development outputs and freeze code, contracts, budgets, tasks, and binary.\n"
            "3. Prepare exact holdout worktrees from the frozen manifest.\n"
            "4. Run one attempt per holdout episode and seal all outputs.\n"
            "5. Unlock historical gold, score each dimension, and render the review bundle.\n"
        ),
        "RUN_LOG.md": f"# Task Lens v0 run log\n\n- {utc_now()} — harness initialized; no product episodes run.\n",
        "EXPERIMENTS.jsonl": "",
        "HARNESS_AUDIT.md": harness_audit_text(root, "initialized"),
        "DEV_EVALUATION.md": "# Development evaluation\n\nStatus: not_run\n",
        "HOLDOUT_EVALUATION.md": "# Holdout evaluation\n\nStatus: not_run\n",
        "PRODUCT_FINDINGS.md": "# Product findings\n\nStatus: not_evaluated\n",
        "WALKTHROUGH.md": (
            "# Walkthrough\n\n"
            "Status: harness initialized. Development, freeze, holdout, and gold evaluation remain.\n"
        ),
        "SUPERVISOR_REPORT.md": (
            "outcome: partial\n"
            "technical result: harness_ready_product_not_run\n"
            "product result: not_evaluated\n"
            "investment result: not_evaluated\n"
            "development episodes used: 0\n"
            "holdout episodes evaluated: 0\n"
            "single strongest demonstrated value: exact-revision and sealing protocol is available\n"
            "single main blocker: development and frozen holdout have not run\n"
            "recommended next step: run the development set through the harness\n\n"
            "answer quality comparison: answer_A_B_not_run\n\n"
            f"Supervisor report:\n{report_path}\n\n"
            f"User review:\n{review_command}\n{review_url()}\n"
        ),
        "review/index.html": initial_review_html(root),
    }
    for relative, text in templates.items():
        destination = root / relative
        if not destination.exists():
            write_new(destination, text.encode("utf-8"))


def prepare_phase(args: argparse.Namespace) -> None:
    root = Path(args.root).expanduser().resolve()
    init_templates(root)
    source_repo = resolve_existing(args.source_repo, "dir")
    manifest_path = resolve_existing(args.manifest, "file")
    tasks_dir = resolve_existing(args.tasks_dir, "dir") if args.tasks_dir else None
    ensure_git_repository(source_repo)
    manifest = validate_manifest(manifest_path, args.phase)
    phase_root = root / args.phase
    empty_directory(phase_root, f"{args.phase} preparation directory")

    if args.phase == "holdout":
        reject_holdout_gold_env()
        assert_no_holdout_gold(root)
        reject_legacy_holdout_path(root, "holdout root")
        reject_legacy_holdout_path(source_repo, "source repository")
        reject_legacy_holdout_path(manifest_path, "holdout manifest")
        freeze = verify_freeze(root, require_frozen_runner=True)
        verify_cheap_exit_expectations(root)
        expected_manifest = root / freeze["inputs"]["holdout_manifest"]["path"]
        if sha256_file(manifest_path) != freeze["inputs"]["holdout_manifest"]["sha256"]:
            fail("holdout manifest does not match the frozen manifest")
        inputs = phase_root / "inputs"
        inputs.mkdir()
        frozen_binary = root / freeze["binary"]["path"]
        copy_new(frozen_binary, inputs / "repomap", 0o555)
        copy_new(expected_manifest, inputs / "HOLDOUT_SET.json", 0o444)
        manifest_path = inputs / "HOLDOUT_SET.json"
    else:
        copy_new(manifest_path, phase_root / "DEV_SET.json", 0o444)

    comparison_revision = git_output(source_repo, "rev-parse", "HEAD")
    planned: list[dict[str, Any]] = []
    for episode in manifest["episodes"]:
        revision = git_output(source_repo, "rev-parse", "--verify", f"{episode['base_revision']}^{{commit}}")
        if revision != episode["base_revision"]:
            fail(f"episode {episode['id']}: base revision did not resolve exactly")
        tree = git_output(source_repo, "rev-parse", f"{revision}^{{tree}}")
        later = later_added_paths(source_repo, revision, comparison_revision)
        explicit = list(episode.get("must_be_absent", [])) + list(episode.get("absent_paths", []))
        task_text, task_source = task_text_for_episode(args.phase, episode, tasks_dir)
        planned.append(
            {
                "episode": episode,
                "revision": revision,
                "tree": tree,
                "later": later,
                "absent": sorted(set(later + explicit)),
                "task_text": task_text,
                "task_source": task_source,
            }
        )

    for item in planned:
        episode = item["episode"]
        episode_root = phase_root / episode["id"]
        worktree = episode_root / "worktree" / "repo"
        source_export = episode_root / "source" / "repo"
        task_file = episode_root / "task.md"
        worktree.parent.mkdir(parents=True)
        git(source_repo, "worktree", "add", "--detach", str(worktree), item["revision"])
        archive_export(source_repo, item["revision"], source_export)
        checks = assert_worktree(
            source_repo,
            worktree,
            item["revision"],
            item["tree"],
            item["absent"],
        )
        write_new(task_file, item["task_text"].encode("utf-8"), 0o444)
        provenance = {
            "version": VERSION,
            "phase": args.phase,
            "episode_id": episode["id"],
            "manifest_repository": manifest["repository"],
            "base_revision": item["revision"],
            "tree_hash": item["tree"],
            "source_comparison_revision": comparison_revision,
            "later_added_path_count": len(item["later"]),
            "later_added_paths_sha256": sha256_bytes(
                "\0".join(item["later"]).encode("utf-8", "surrogateescape")
            ),
            "task_sha256": sha256_file(task_file),
            "task_source": item["task_source"],
            "worktree": "worktree/repo",
            "source_only_export": "source/repo",
            "checks": checks,
            "prepared_at": utc_now(),
        }
        provenance_path = episode_root / "PROVENANCE.json"
        write_json_new(provenance_path, provenance, 0o444)
        write_new(
            episode_root / "PROVENANCE.sha256",
            f"{sha256_file(provenance_path)}  PROVENANCE.json\n".encode("ascii"),
            0o444,
        )

    prepared = {
        "version": VERSION,
        "phase": args.phase,
        "manifest_sha256": sha256_file(manifest_path),
        "source_repository": str(source_repo),
        "source_comparison_revision": comparison_revision,
        "episode_ids": [item["episode"]["id"] for item in planned],
        "neutral_worktree_basename": "repo",
        "source_only_exports_are_git_free": True,
        "prepared_at": utc_now(),
    }
    if args.phase == "holdout":
        freeze = verify_freeze(root, require_frozen_runner=True)
        prepared["freeze_manifest_sha256"] = sha256_file(root / "FREEZE_MANIFEST.json")
        prepared["frozen_binary_sha256"] = freeze["binary"]["sha256"]
        prepared["cheap_exit_expectations_sha256"] = sha256_file(
            cheap_exit_expectation_paths(root)[0]
        )
    prepared_path = phase_root / "PREPARED.json"
    write_json_new(prepared_path, prepared, 0o444)
    write_new(
        phase_root / "PREPARED.sha256",
        f"{sha256_file(prepared_path)}  PREPARED.json\n".encode("ascii"),
        0o444,
    )
    append_log(root, f"prepared {args.phase}: {len(planned)} exact detached worktrees and Git-free exports")
    refresh_audit(root)


def append_log(root: Path, message: str) -> None:
    path = root / "RUN_LOG.md"
    with path.open("a", encoding="utf-8") as handle:
        handle.write(f"- {utc_now()} — {message}.\n")


def append_experiment(root: Path, value: dict[str, Any]) -> None:
    with (root / "EXPERIMENTS.jsonl").open("a", encoding="utf-8") as handle:
        handle.write(json.dumps(value, sort_keys=True, separators=(",", ":")) + "\n")


def load_phase(root: Path, phase: str) -> tuple[dict[str, Any], dict[str, Any], Path]:
    phase_root = root / phase
    prepared_path = phase_root / "PREPARED.json"
    if not prepared_path.is_file():
        fail(f"{phase} has not been prepared")
    prepared_sidecar = phase_root / "PREPARED.sha256"
    if not prepared_sidecar.is_file():
        fail(f"{phase} preparation seal is missing")
    parts = prepared_sidecar.read_text(encoding="ascii").split()
    if (
        len(parts) != 2
        or parts[0] != sha256_file(prepared_path)
        or parts[1] != "PREPARED.json"
    ):
        fail(f"{phase} PREPARED.json changed after preparation")
    prepared = read_json(prepared_path)
    manifest_path = phase_root / ("DEV_SET.json" if phase == "dev" else "inputs/HOLDOUT_SET.json")
    manifest = validate_manifest(manifest_path, phase)
    if sha256_file(manifest_path) != prepared.get("manifest_sha256"):
        fail(f"{phase} manifest changed after preparation")
    if prepared.get("phase") != phase or prepared.get("version") != VERSION:
        fail(f"{phase} preparation record has an invalid header")
    expected_ids = [episode["id"] for episode in manifest["episodes"]]
    if prepared.get("episode_ids") != expected_ids:
        fail(f"{phase} preparation episode IDs differ from the manifest")
    if phase == "holdout":
        freeze = verify_freeze(root, require_frozen_runner=True)
        verify_cheap_exit_expectations(root)
        if sha256_file(manifest_path) != freeze["inputs"]["holdout_manifest"]["sha256"]:
            fail("prepared holdout manifest differs from the frozen owner input")
        if prepared.get("freeze_manifest_sha256") != sha256_file(root / "FREEZE_MANIFEST.json"):
            fail("holdout preparation is not bound to the current freeze")
        if prepared.get("frozen_binary_sha256") != freeze["binary"]["sha256"]:
            fail("holdout preparation is not bound to the frozen binary")
        if prepared.get("cheap_exit_expectations_sha256") != sha256_file(
            cheap_exit_expectation_paths(root)[0]
        ):
            fail("holdout preparation is not bound to the predeclared cheap-exit expectations")
    return prepared, manifest, phase_root


def load_provenance(episode_root: Path) -> dict[str, Any]:
    provenance_path = episode_root / "PROVENANCE.json"
    sidecar = episode_root / "PROVENANCE.sha256"
    if not sidecar.is_file():
        fail(f"episode provenance seal is missing: {sidecar}")
    parts = sidecar.read_text(encoding="ascii").split()
    if (
        len(parts) != 2
        or parts[0] != sha256_file(provenance_path)
        or parts[1] != "PROVENANCE.json"
    ):
        fail(f"episode provenance changed after preparation: {provenance_path}")
    provenance = read_json(provenance_path)
    required = ("episode_id", "base_revision", "tree_hash", "worktree", "task_sha256")
    if not all(isinstance(provenance.get(field), str) for field in required):
        fail(f"invalid provenance: {episode_root / 'PROVENANCE.json'}")
    provenance["episode_root"] = episode_root
    return provenance


def verify_episode_source(
    root: Path,
    source_repo: Path,
    phase: str,
    expected_episode: dict[str, Any],
    episode_root: Path,
    manifest_repository: str,
) -> dict[str, Any]:
    provenance = load_provenance(episode_root)
    if provenance.get("version") != VERSION or provenance.get("phase") != phase:
        fail(f"episode {expected_episode['id']}: provenance phase/version mismatch")
    if provenance.get("episode_id") != expected_episode["id"]:
        fail(f"episode {expected_episode['id']}: provenance ID mismatch")
    if provenance.get("manifest_repository") != manifest_repository:
        fail(f"episode {expected_episode['id']}: provenance repository mismatch")
    if provenance.get("base_revision") != expected_episode["base_revision"]:
        fail(f"episode {expected_episode['id']}: provenance base revision differs from manifest")
    if provenance.get("worktree") != "worktree/repo" or provenance.get("source_only_export") != "source/repo":
        fail(f"episode {expected_episode['id']}: provenance paths are not the fixed neutral paths")
    expected_tree = git_output(source_repo, "rev-parse", f"{expected_episode['base_revision']}^{{tree}}")
    if provenance.get("tree_hash") != expected_tree:
        fail(f"episode {expected_episode['id']}: provenance tree differs from the base commit")
    repo = episode_root / "worktree/repo"
    checks = provenance.get("checks", {})
    absent = checks.get("absent_paths_checked", []) if isinstance(checks, dict) else []
    assert_worktree(
        source_repo,
        repo,
        expected_episode["base_revision"],
        expected_tree,
        absent,
    )
    task_file = episode_root / "task.md"
    if sha256_file(task_file) != provenance["task_sha256"]:
        fail(f"task packet changed after preparation: {task_file}")
    if phase == "holdout":
        expected_task = (expected_episode["task"].rstrip() + "\n").encode("utf-8")
        if task_file.read_bytes() != expected_task:
            fail(f"holdout task packet differs from the frozen manifest: {task_file}")
        task_entry = frozen_task_entry(root, "holdout", expected_episode["id"])
        if (
            task_entry["sha256"] != sha256_file(task_file)
            or task_entry["base_revision"] != expected_episode["base_revision"]
            or task_entry["tree_hash"] != expected_tree
        ):
            fail(f"holdout task packet differs from the frozen task ledger: {task_file}")
    export = episode_root / "source/repo"
    if any(path.name == ".git" for path in export.rglob(".git")):
        fail(f"source-only export contains Git metadata: {export}")
    return provenance


def next_dev_attempt(episode_root: Path) -> Path:
    attempts = episode_root / "attempts"
    attempts.mkdir(exist_ok=True)
    numbers = []
    for path in attempts.iterdir():
        if path.is_dir() and path.name.isdigit():
            numbers.append(int(path.name))
    number = max(numbers, default=0) + 1
    return attempts / f"{number:03d}"


def holdout_attempt(episode_root: Path) -> Path:
    return episode_root / "attempt"


def artifact_paths(run_dir: Path) -> dict[str, Path]:
    found: dict[str, Path] = {}
    for name in ARTIFACT_NAMES:
        matches = sorted(path for path in run_dir.rglob(name) if path.is_file())
        if len(matches) != 1:
            fail(f"expected exactly one {name} under {run_dir}; found {len(matches)}")
        found[name] = matches[0]
    return found


def exact_task_workspace_replay(
    binary: Path,
    paths: dict[str, Path],
    saved_report_path: Path,
) -> dict[str, Any]:
    """Replay the four artifacts through the candidate binary's Go validator."""
    if not binary.is_file() or not os.access(binary, os.X_OK):
        fail(f"exact Task Lens validator binary is unavailable: {binary}")
    with tempfile.TemporaryDirectory(prefix="repomap-task-lens-replay-") as temporary:
        replay_dir = Path(temporary) / "run"
        replay_dir.mkdir()
        for name, source in paths.items():
            shutil.copyfile(source, replay_dir / name)
        # The synthetic protocol stub treats render-report as a no-op. A real
        # repomap binary overwrites this seed through report.Generate.
        shutil.copyfile(saved_report_path, replay_dir / "report.json")
        result = run_command(
            (str(binary), "dev", "render-report", str(replay_dir)),
            check=False,
        )
        if result.returncode != 0:
            fail("candidate binary rejected Task Lens artifacts during exact Go replay")
        rendered_path = replay_dir / "report.json"
        if not rendered_path.is_file() or rendered_path.is_symlink():
            fail("candidate binary exact replay did not produce a regular report.json")
        rendered = require_object(read_json(rendered_path), "exact Go replay report")
        workspace = rendered.get("task_investigation")
        if not isinstance(workspace, dict):
            warnings = rendered.get("warnings", [])
            if any(
                isinstance(warning, str) and warning.startswith("task investigation unavailable:")
                for warning in warnings if isinstance(warnings, list)
            ):
                fail("candidate binary rejected Task Lens artifacts during exact Go replay")
            fail("candidate binary exact replay omitted the Task Lens workspace")
        return workspace


def require_object(value: Any, label: str) -> dict[str, Any]:
    if not isinstance(value, dict):
        fail(f"{label} must be a JSON object")
    return value


def require_array(value: Any, label: str, minimum: int = 0, maximum: int | None = None) -> list[Any]:
    if not isinstance(value, list) or len(value) < minimum or (maximum is not None and len(value) > maximum):
        bound = f"{minimum}..{maximum}" if maximum is not None else f"at least {minimum}"
        fail(f"{label} must contain {bound} items")
    return value


def require_text(value: Any, label: str, maximum_bytes: int = 4096, allow_empty: bool = False) -> str:
    if not isinstance(value, str) or (not allow_empty and not value.strip()):
        fail(f"{label} must be non-empty text")
    if len(value.encode("utf-8")) > maximum_bytes or "\x00" in value:
        fail(f"{label} is outside its text bound")
    return value


def require_nonnegative_int(value: Any, label: str, maximum: int | None = None) -> int:
    if not isinstance(value, int) or isinstance(value, bool) or value < 0:
        fail(f"{label} must be a non-negative integer")
    if maximum is not None and value > maximum:
        fail(f"{label} exceeds {maximum}")
    return value


def require_sha(value: Any, label: str) -> str:
    if not isinstance(value, str) or not SHA256_RE.fullmatch(value):
        fail(f"{label} must be a lowercase SHA-256")
    return value


def prompt_safe_task_text(raw: str) -> str:
    """Mirror Task Lens task-file projection for harness replay checks."""
    text = raw.replace("\r\n", "\n").strip()
    heading = "## Prompt-safe task"
    start = text.find(heading)
    if start >= 0:
        body = text[start + len(heading):]
        next_heading = body.find("\n## ")
        if next_heading >= 0:
            body = body[:next_heading]
        text = body.strip()
    if not text:
        fail("task packet has no prompt-safe task text")
    return text


def validate_repository_object(value: Any, provenance: dict[str, Any], label: str) -> dict[str, Any]:
    repository = require_object(value, label)
    require_text(repository.get("identity"), f"{label}.identity", 512)
    require_text(repository.get("display_name", ""), f"{label}.display_name", 256, True)
    if repository.get("revision") != provenance["base_revision"]:
        fail(f"{label}.revision does not match the exact episode base")
    if repository.get("tree_hash") != provenance["tree_hash"]:
        fail(f"{label}.tree_hash does not match the exact episode base tree")
    require_sha(repository.get("state_sha256"), f"{label}.state_sha256")
    expected_state_sha = sha256_bytes(
        json.dumps(
            {"version": 2, "head": provenance["base_revision"], "dirty": []},
            separators=(",", ":"),
        ).encode("utf-8")
    )
    if repository["state_sha256"] != expected_state_sha:
        fail(f"{label}.state_sha256 does not match the exact clean detached worktree")
    if repository.get("identity_source") not in {"root_module", "remote", "manifest", "neutral_fallback"}:
        fail(f"{label}.identity_source is invalid")
    identity_source_path = repository.get("identity_source_path", "")
    if identity_source_path:
        validate_relative_path(identity_source_path, f"{label}.identity_source_path")
    return repository


def validate_budget_object(value: Any, label: str) -> dict[str, Any]:
    budgets = require_object(value, label)
    for field, maximum in BUDGET_LIMITS.items():
        require_nonnegative_int(budgets.get(field), f"{label}.{field}", maximum)
    require_nonnegative_int(budgets.get("candidate_items_found"), f"{label}.candidate_items_found")
    require_nonnegative_int(budgets.get("anchor_items_found"), f"{label}.anchor_items_found")
    for field in (
        "candidate_limit_bound", "anchor_limit_bound", "file_limit_bound",
        "byte_limit_bound", "retained_byte_limit_bound", "time_limit_bound",
    ):
        if field in budgets and not isinstance(budgets[field], bool):
            fail(f"{label}.{field} must be boolean when present")
    if (
        budgets["candidate_items_found"] < budgets["initial_candidates"]
        or budgets["anchor_items_found"] < budgets["retained_anchors"]
        or budgets["evidence_files_considered"] > budgets["initial_candidates"]
        or (budgets["read_files"] > 0 and budgets["read_bytes"] == 0)
        or (budgets["read_files"] == 0 and budgets["read_bytes"] != 0)
    ):
        fail(f"{label} has inconsistent retrieval counts")
    if budgets["initial_candidates"] != min(
        budgets["candidate_items_found"], BUDGET_LIMITS["initial_candidates"]
    ):
        fail(f"{label}.initial_candidates does not match the bounded candidate selection")
    if budgets["evidence_files_considered"] != min(
        budgets["initial_candidates"], BUDGET_LIMITS["read_files"]
    ):
        fail(f"{label}.evidence_files_considered does not match selected evidence files")
    if budgets.get("candidate_limit_bound", False) != (
        budgets["candidate_items_found"] > BUDGET_LIMITS["initial_candidates"]
    ):
        fail(f"{label}.candidate_limit_bound does not match candidate accounting")
    if budgets.get("anchor_limit_bound", False) != (
        budgets["anchor_items_found"] > BUDGET_LIMITS["retained_anchors"]
    ):
        fail(f"{label}.anchor_limit_bound does not match anchor accounting")
    if budgets.get("file_limit_bound", False) and budgets["read_files"] != BUDGET_LIMITS["read_files"]:
        fail(f"{label}.file_limit_bound does not match the unified read-file budget")
    if budgets.get("byte_limit_bound", False) and budgets["read_bytes"] != BUDGET_LIMITS["read_bytes"]:
        fail(f"{label}.byte_limit_bound does not match read-byte accounting")
    if budgets.get("retained_byte_limit_bound", False) and (
        budgets["anchor_items_found"] <= budgets["retained_anchors"]
    ):
        fail(f"{label}.retained_byte_limit_bound does not match retained-anchor accounting")
    return budgets


def go_compact_json_sha(value: Any) -> str:
    raw = json.dumps(value, ensure_ascii=False, separators=(",", ":"))
    raw = raw.replace("&", "\\u0026").replace("<", "\\u003c").replace(">", "\\u003e")
    raw = raw.replace("\u2028", "\\u2028").replace("\u2029", "\\u2029")
    return sha256_bytes(raw.encode("utf-8"))


def go_opaque_id(kind: str, *parts: str) -> str:
    digest = hashlib.sha256()
    digest.update(("repomap-task-lens-v1\x00" + kind).encode("utf-8"))
    for part in parts:
        digest.update(("\x00" + part).encode("utf-8"))
    return f"{kind}-{digest.hexdigest()[:20]}"


def validate_bundle(
    bundle: Any,
    provenance: dict[str, Any],
    task_text: str,
    explicitly_insufficient: bool,
) -> dict[str, Any]:
    bundle = require_object(bundle, "task bundle")
    if bundle.get("version") != 1:
        fail("task bundle has an unsupported schema version")
    bundle_id = require_text(bundle.get("id"), "task bundle.id", 128)
    repository = validate_repository_object(
        bundle.get("repository"), provenance, "task bundle.repository"
    )
    task = require_object(bundle.get("task"), "task bundle.task")
    if task.get("text") != task_text.strip():
        fail("task bundle text does not match the sealed task packet")
    task_evidence_id = require_text(task.get("evidence_id"), "task bundle.task.evidence_id", 128)
    task_sha = sha256_bytes(task_text.strip().encode("utf-8"))
    expected_bundle_id = go_opaque_id(
        "task",
        repository["identity"],
        repository["revision"],
        repository["state_sha256"],
        task_sha,
    )
    if bundle_id != expected_bundle_id:
        fail("task bundle.id does not bind repository state and task text")
    if task_evidence_id != go_opaque_id("evidence", "task", task_sha):
        fail("task bundle task evidence ID does not bind the exact task text")
    if bundle.get("task_kind_hint") not in TASK_KINDS:
        fail("task bundle task_kind_hint is invalid")
    locality = bundle.get("locality")
    if locality not in LOCALITIES:
        fail("task bundle locality is invalid")
    require_text(bundle.get("observable_hint"), "task bundle.observable_hint")
    allowed_paths = require_array(bundle.get("allowed_paths"), "task bundle.allowed_paths", 0, 12)
    if not all(isinstance(item, str) for item in allowed_paths) or allowed_paths != sorted(set(allowed_paths)):
        fail("task bundle.allowed_paths must be unique sorted strings")
    for path in allowed_paths:
        validate_relative_path(path, "task bundle allowed path")
    allowed = set(allowed_paths)
    repository_root = (provenance["episode_root"] / provenance["worktree"]).resolve()
    source_lines: dict[str, list[str]] = {}
    for relative in allowed_paths:
        candidate = (repository_root / relative).resolve()
        try:
            candidate.relative_to(repository_root)
        except ValueError:
            fail(f"task bundle allowed path escapes the exact episode source: {relative}")
        if not candidate.is_file():
            fail(f"task bundle allowed path is not a source file at the exact episode base: {relative}")
        try:
            source_lines[relative] = candidate.read_text(encoding="utf-8").splitlines()
        except (OSError, UnicodeDecodeError) as exc:
            fail(f"cannot verify model-visible source {relative}: {exc}")

    evidence_items = require_array(bundle.get("evidence"), "task bundle.evidence", 1)
    evidence: dict[str, dict[str, Any]] = {}
    for index, raw in enumerate(evidence_items):
        item = require_object(raw, f"task bundle.evidence[{index}]")
        evidence_id = require_text(item.get("id"), f"task bundle.evidence[{index}].id", 128)
        if evidence_id in evidence:
            fail(f"task bundle repeats evidence ID {evidence_id}")
        kind = item.get("kind")
        if kind not in {"repository_fact", "document_claim", "task_provided"}:
            fail(f"task bundle evidence {evidence_id} has invalid kind")
        require_text(item.get("summary"), f"task bundle evidence {evidence_id}.summary", 1024)
        if kind == "task_provided":
            if (
                evidence_id != task_evidence_id
                or item.get("path", "") not in (None, "")
                or item.get("start_line", 0) != 0
                or item.get("end_line", 0) != 0
                or item.get("anchor_id", "") not in (None, "")
                or item["summary"]
                != "Symptom or requested outcome supplied by the task; not repository truth."
            ):
                fail("task-provided evidence is not isolated from repository evidence")
        else:
            if item.get("path") not in allowed:
                fail(f"task bundle evidence {evidence_id} path is not allowed")
            require_nonnegative_int(item.get("start_line"), f"evidence {evidence_id}.start_line")
            require_nonnegative_int(item.get("end_line"), f"evidence {evidence_id}.end_line")
            if item["start_line"] < 1 or item["end_line"] < item["start_line"]:
                fail(f"task bundle evidence {evidence_id} has invalid line bounds")
            if item["end_line"] > len(source_lines[item["path"]]):
                fail(f"task bundle evidence {evidence_id} exceeds its exact source file")
        evidence[evidence_id] = item
    if task_evidence_id not in evidence:
        fail("task bundle task evidence ID is absent")

    anchors_raw = require_array(bundle.get("anchors"), "task bundle.anchors", 0, 16)
    if not anchors_raw:
        if not explicitly_insufficient or locality != "broad_dynamic":
            fail("a zero-anchor task bundle must be an explicitly insufficient broad result")
    elif not allowed_paths:
        fail("a task bundle with anchors must expose their bounded allowed paths")
    if len(evidence_items) != len(anchors_raw) + 1:
        fail("task bundle evidence cardinality does not match its anchors and exact task evidence")
    anchors: dict[str, dict[str, Any]] = {}
    for index, raw in enumerate(anchors_raw):
        anchor = require_object(raw, f"task bundle.anchors[{index}]")
        anchor_id = require_text(anchor.get("id"), f"task bundle.anchors[{index}].id", 128)
        if anchor_id in anchors or anchor.get("path") not in allowed:
            fail(f"task bundle anchor {anchor_id} is duplicate or outside allowed paths")
        require_text(anchor.get("symbol"), f"task bundle anchor {anchor_id}.symbol", 256)
        role_hints = require_array(
            anchor.get("role_hints", []),
            f"task bundle anchor {anchor_id}.role_hints",
            0,
            len(ANCHOR_ROLES),
        )
        if (
            any(role not in ANCHOR_ROLES for role in role_hints)
            or role_hints != list(dict.fromkeys(role_hints))
        ):
            fail(f"task bundle anchor {anchor_id} has invalid role hints")
        start = require_nonnegative_int(anchor.get("start_line"), f"anchor {anchor_id}.start_line")
        end = require_nonnegative_int(anchor.get("end_line"), f"anchor {anchor_id}.end_line")
        excerpt = require_array(anchor.get("excerpt"), f"anchor {anchor_id}.excerpt", 1, 60)
        if start < 1 or end < start or len(excerpt) != end - start + 1:
            fail(f"task bundle anchor {anchor_id} has inconsistent excerpt bounds")
        for offset, line in enumerate(excerpt):
            line = require_object(line, f"anchor {anchor_id}.excerpt[{offset}]")
            if line.get("line") != start + offset or not isinstance(line.get("text"), str):
                fail(f"task bundle anchor {anchor_id} has non-contiguous source lines")
            if end > len(source_lines[anchor["path"]]) or line["text"] != source_lines[anchor["path"]][start + offset - 1]:
                fail(f"task bundle anchor {anchor_id} excerpt differs from the exact episode source")
        excerpt_sha = go_compact_json_sha(excerpt)
        expected_anchor_id = go_opaque_id(
            "anchor", anchor["path"], anchor["symbol"], str(start), str(end), excerpt_sha
        )
        if anchor_id != expected_anchor_id:
            fail(f"task bundle anchor {anchor_id} ID does not bind its exact excerpt")
        anchor_evidence = require_array(anchor.get("evidence_ids"), f"anchor {anchor_id}.evidence_ids", 1)
        expected_evidence_id = go_opaque_id(
            "evidence",
            repository["state_sha256"],
            anchor["path"],
            str(start),
            str(end),
            excerpt_sha,
        )
        if anchor_evidence != [expected_evidence_id]:
            fail(f"task bundle anchor {anchor_id} evidence ID does not bind its exact excerpt")
        for evidence_id in anchor_evidence:
            item = evidence.get(evidence_id)
            if not item or item.get("anchor_id") != anchor_id or item.get("path") != anchor.get("path"):
                fail(f"task bundle anchor {anchor_id} has mismatched evidence")
            expected_kind = (
                "document_claim"
                if Path(anchor["path"]).suffix.lower() in {".md", ".mdx", ".rst", ".txt"}
                else "repository_fact"
            )
            summary_prefix = "document" if expected_kind == "document_claim" else "source"
            expected_summary = (
                f"Exact repository {summary_prefix} excerpt for {anchor['symbol']} at "
                f"{anchor['path']}:{start}-{end}."
            )
            if item.get("kind") != expected_kind or item.get("summary") != expected_summary:
                fail(f"task bundle anchor {anchor_id} evidence kind or summary is not deterministic")
        anchors[anchor_id] = anchor

    for evidence_id, item in evidence.items():
        anchor_id = item.get("anchor_id")
        if anchor_id and anchor_id not in anchors:
            fail(f"task bundle evidence {evidence_id} references an unknown anchor")

    modules = require_array(bundle.get("modules", []), "task bundle.modules", 0, 12)
    module_source_paths: set[str] = set()
    for index, raw in enumerate(modules):
        module = require_object(raw, f"task bundle.modules[{index}]")
        require_text(module.get("id"), f"task bundle.modules[{index}].id", 128)
        require_text(module.get("path"), f"task bundle.modules[{index}].path", 512)
        require_text(module.get("dir"), f"task bundle.modules[{index}].dir", 512, True)
        source_path = require_text(
            module.get("source_path"),
            f"task bundle.modules[{index}].source_path",
            512,
        )
        validate_relative_path(source_path, f"task bundle.modules[{index}].source_path")
        module_source_paths.add(source_path)
    expected_allowed_paths = {
        anchor["path"] for anchor in anchors.values()
    } | module_source_paths
    if repository.get("identity_source_path"):
        expected_allowed_paths.add(repository["identity_source_path"])
    if allowed_paths != sorted(expected_allowed_paths):
        fail("task bundle allowed paths are not the exact anchor/module/identity source set")

    seen_terms: set[str] = set()
    for index, raw in enumerate(require_array(bundle.get("terms"), "task bundle.terms", 0, 32)):
        term = require_object(raw, f"task bundle.terms[{index}]")
        text = require_text(term.get("text"), f"task bundle.terms[{index}].text", 96)
        normalized = require_text(
            term.get("normalized"),
            f"task bundle.terms[{index}].normalized",
            96,
        )
        if normalized != text.lower() or normalized in seen_terms:
            fail("task bundle terms are not uniquely normalized")
        seen_terms.add(normalized)
        if term.get("id") != go_opaque_id("term", normalized):
            fail("task bundle term ID does not bind its normalized value")
        found = term.get("found")
        if not isinstance(found, bool):
            fail("task bundle term found state is not boolean")
        evidence_ids_for_term = require_array(
            term.get("evidence_ids", []),
            f"task bundle.terms[{index}].evidence_ids",
        )
        if evidence_ids_for_term != sorted(set(evidence_ids_for_term)):
            fail("task bundle term evidence IDs are not unique and sorted")
        if found != bool(evidence_ids_for_term):
            fail("task bundle term found state does not match its evidence")
        if any(
            evidence_id not in evidence
            or not evidence[evidence_id].get("anchor_id")
            or evidence[evidence_id].get("anchor_id") not in anchors
            for evidence_id in evidence_ids_for_term
        ):
            fail("task bundle term references evidence outside retained anchors")
        require_nonnegative_int(term.get("weight", 0), f"task bundle.terms[{index}].weight")

    for index, raw in enumerate(require_array(bundle.get("relations", []), "task bundle.relations")):
        relation = require_object(raw, f"task bundle.relations[{index}]")
        if relation.get("left_anchor_id") not in anchors or relation.get("right_anchor_id") not in anchors:
            fail("task bundle relation references an unknown anchor")
        if relation.get("left_anchor_id") == relation.get("right_anchor_id"):
            fail("task bundle relation is self-referential")
        if relation.get("support_type") != "locally_observed":
            fail("task bundle local relation is not labelled locally_observed")
        for evidence_id in require_array(relation.get("evidence_ids"), "task bundle relation evidence", 1):
            if evidence_id not in evidence:
                fail("task bundle relation references unknown evidence")

    stages = require_array(bundle.get("stages_skipped"), "task bundle.stages_skipped", 1)
    if tuple(stages) != CANONICAL_SKIPPED_STAGES:
        fail("task bundle skipped stages are not the exact ordered dedicated-pipeline set")
    budgets = validate_budget_object(bundle.get("budgets"), "task bundle.budgets")
    if budgets["retained_anchors"] != len(anchors):
        fail("task bundle retained-anchor accounting does not match anchors")
    retained_source_bytes = sum(
        len(line["text"].encode("utf-8")) + 1
        for anchor in anchors.values()
        for line in anchor["excerpt"]
    )
    if budgets["retained_source_bytes"] != retained_source_bytes:
        fail("task bundle retained-source-byte accounting does not match exact anchor excerpts")
    if budgets["read_files"] < len(allowed_paths):
        fail("task bundle read-file accounting is smaller than its model-visible file set")
    metrics = require_object(bundle.get("metrics"), "task bundle.metrics")
    for field in (
        "tracked_files", "git_grep_queries", "ast_parses", "relations_retained",
        "evidence_files_read", "module_files_found", "module_files_read", "module_bytes_read",
        "manifest_files_read", "manifest_bytes_read",
    ):
        require_nonnegative_int(metrics.get(field), f"task bundle.metrics.{field}")
    if (
        metrics["module_files_read"] < len(modules)
        or metrics["module_files_read"] > 12
        or metrics["module_files_found"] < metrics["module_files_read"]
        or metrics["module_bytes_read"] > metrics["manifest_bytes_read"]
        or (metrics["module_files_read"] > 0 and metrics["module_bytes_read"] == 0)
        or metrics["manifest_files_read"] < metrics["module_files_read"]
        or metrics["manifest_files_read"] > budgets["read_files"]
        or metrics["manifest_bytes_read"] > budgets["read_bytes"]
        or metrics["manifest_bytes_read"] < metrics["module_bytes_read"]
        or (metrics["manifest_files_read"] > 0 and metrics["manifest_bytes_read"] == 0)
    ):
        fail("task bundle manifest/module retrieval accounting is inconsistent")
    if (
        metrics["tracked_files"] < len(allowed_paths)
        or budgets["candidate_items_found"] > metrics["tracked_files"]
        or metrics["evidence_files_read"] < len({anchor["path"] for anchor in anchors.values()})
        or metrics["evidence_files_read"] > budgets["evidence_files_considered"]
        or metrics["evidence_files_read"] > budgets["read_files"]
        or metrics["git_grep_queries"] > 12
        or metrics["ast_parses"] > metrics["evidence_files_read"]
    ):
        fail("task bundle local retrieval accounting is inconsistent")
    if budgets.get("file_limit_bound", False) and (
        metrics["evidence_files_read"] >= budgets["evidence_files_considered"]
    ):
        fail("task bundle file-limit flag does not identify skipped selected evidence")
    if metrics["relations_retained"] != len(bundle.get("relations", [])):
        fail("task bundle relation accounting does not match relations")
    for forbidden in ("file_tree", "internal_edges"):
        if forbidden in bundle:
            fail(f"task bundle exposes forbidden full-repository field {forbidden}")
    return bundle


def validate_pack(
    pack: Any,
    bundle: dict[str, Any],
    provenance: dict[str, Any],
    expected_bundle_sha: str,
    status_state: str,
) -> dict[str, Any]:
    pack = require_object(pack, "task pack")
    if pack.get("version") != 1 or pack.get("id") != bundle["id"]:
        fail("task pack header does not match the task bundle")
    if pack.get("bundle_sha256") != expected_bundle_sha:
        fail("task pack bundle SHA does not match the saved bundle")
    validate_repository_object(pack.get("repository"), provenance, "task pack.repository")
    if pack.get("repository") != bundle["repository"]:
        fail("task pack repository identity differs from its bundle")
    if pack.get("locality") != bundle["locality"] or pack.get("stages_skipped") != bundle["stages_skipped"]:
        fail("task pack locality/stage accounting differs from its bundle")
    anchors = require_array(
        pack.get("investigation_anchors"),
        "task pack.investigation_anchors",
        0,
        8,
    )
    bundle_anchors = {item["id"]: item for item in bundle["anchors"]}
    selected: set[str] = set()
    for index, raw in enumerate(anchors):
        anchor = require_object(raw, f"task pack.investigation_anchors[{index}]")
        anchor_id = anchor.get("id")
        source = bundle_anchors.get(anchor_id)
        if not source or anchor_id in selected:
            fail("task pack contains an unknown or duplicate anchor")
        selected.add(anchor_id)
        for field in (
            "path", "symbol", "section", "package", "start_line", "end_line", "excerpt", "evidence_ids",
        ):
            if anchor.get(field) != source.get(field):
                fail(f"task pack anchor {anchor_id} changed locally observed source evidence")
        if anchor.get("role") not in source.get("role_hints", []):
            fail(f"task pack anchor {anchor_id} role is not allowed by local role hints")
        require_text(anchor.get("why"), f"task pack anchor {anchor_id}.why", 1024)
    areas = require_array(
        pack.get("likely_areas"),
        "task pack.likely_areas",
        0 if not anchors else 1,
        3,
    )
    if bool(areas) != bool(anchors):
        fail("task pack likely-area cardinality does not match its visible anchors")
    for raw in areas:
        area = require_object(raw, "task pack likely area")
        require_text(area.get("label"), "task pack likely area.label", 256)
        require_text(area.get("why"), "task pack likely area.why", 1024)
        targets = require_array(area.get("target_ids"), "task pack likely area.target_ids", 1, 8)
        if any(target not in selected for target in targets):
            fail("task pack likely area references an unselected anchor")
    interpretation = require_object(pack.get("task_interpretation"), "task pack.task_interpretation")
    if interpretation.get("task_kind") not in TASK_KINDS:
        fail("task pack interpretation has invalid task kind")
    require_text(interpretation.get("restatement"), "task pack interpretation.restatement", 1024)
    require_text(interpretation.get("observable_or_outcome"), "task pack interpretation.observable", 1024)
    found_terms = [term["text"] for term in bundle["terms"] if term["found"]]
    user_terms = [term["text"] for term in bundle["terms"] if not term["found"]]
    if require_array(
        interpretation.get("repository_terms_found", []),
        "task pack interpretation.repository_terms_found",
    ) != found_terms:
        fail("task pack found-term grounding differs from its bundle")
    if require_array(
        interpretation.get("user_provided_only_terms", []),
        "task pack interpretation.user_provided_only_terms",
    ) != user_terms:
        fail("task pack user-only term grounding differs from its bundle")
    if not isinstance(pack.get("task_observation_concrete"), bool):
        fail("task pack task_observation_concrete must be boolean")
    evidence = {item["id"]: item for item in bundle["evidence"]}
    evidence_ids = set(evidence)
    relation_ids = {item.get("id") for item in bundle.get("relations", [])}
    for index, raw in enumerate(require_array(pack.get("evidence_joins", []), "task pack.evidence_joins", 0, 6)):
        join = require_object(raw, f"task pack.evidence_joins[{index}]")
        if join.get("left_anchor_id") not in selected or join.get("right_anchor_id") not in selected:
            fail("task pack evidence join references an unselected anchor")
        if join.get("support_type") not in SUPPORT_TYPES:
            fail("task pack evidence join has invalid support type")
        join_kind = require_text(join.get("relation_kind"), "task pack evidence join kind", 128)
        relation_id = join.get("relation_id", "")
        if relation_id and relation_id not in relation_ids:
            fail("task pack evidence join references an unknown local relation")
        support_ids = require_array(join.get("support_ids"), "join support IDs", 0)
        if join.get("support_type") == "unresolved":
            if support_ids or relation_id:
                fail("task pack unresolved join must not claim support evidence or a local relation")
        elif not support_ids:
            fail("task pack supported join must cite exact evidence")
        if any(item not in evidence_ids for item in support_ids):
            fail("task pack evidence join references unknown evidence")
        expected_join_id = go_opaque_id(
            "join",
            join["left_anchor_id"],
            join["right_anchor_id"],
            join_kind,
            join["support_type"],
        )
        if join.get("id") != expected_join_id:
            fail("task pack evidence join ID does not bind its endpoints and support semantics")
        require_text(join.get("explanation"), "task pack evidence join explanation", 1536)
        require_text(join.get("scope_non_guarantees"), "task pack evidence join scope", 1024)
    hypotheses = require_array(pack.get("working_hypothesis"), "task pack.working_hypothesis", 1, 3)
    for raw in hypotheses:
        clause = require_object(raw, "task pack hypothesis clause")
        if clause.get("status") not in HYPOTHESIS_STATES:
            fail("task pack hypothesis clause has invalid epistemic label")
        require_text(clause.get("text"), "task pack hypothesis clause text", 1536)
        support_ids = require_array(clause.get("support_ids", []), "hypothesis support IDs")
        if any(
            item not in evidence_ids
            for item in support_ids
        ):
            fail("task pack hypothesis clause references unknown evidence")
        if any(
            item not in relation_ids
            for item in require_array(clause.get("relation_ids", []), "hypothesis relation IDs")
        ):
            fail("task pack hypothesis clause references unknown relation")
        if clause.get("status") == "supported" and not any(
            evidence[item].get("kind") in {"repository_fact", "document_claim"}
            for item in support_ids
        ):
            fail("task pack supported hypothesis lacks exact repository evidence")
    for field in ("reproduce_or_observe",):
        guidance = require_array(pack.get(field), f"task pack.{field}", 1, 4)
        validate_guidance(guidance, evidence, selected, f"task pack.{field}")
    verification = require_object(pack.get("verify"), "task pack.verify")
    require_text(verification.get("effect_to_observe"), "task pack.verify.effect_to_observe", 1024)
    validate_guidance(
        require_array(verification.get("steps"), "task pack.verify.steps", 1, 4),
        evidence,
        selected,
        "task pack.verify.steps",
    )
    for raw in require_array(pack.get("next_probes"), "task pack.next_probes", 1, 3):
        probe = require_object(raw, "task pack next probe")
        if probe.get("action") not in PROBE_ACTIONS:
            fail("task pack next probe has invalid action")
        anchor_ids = require_array(probe.get("anchor_ids"), "probe anchor IDs", 0, 2)
        if probe.get("action") == "search_task_terms":
            if anchor_ids:
                fail("task-term search probe must not invent an anchor")
        elif not anchor_ids:
            fail("anchored task probe must cite at least one selected anchor")
        if any(item not in selected for item in anchor_ids):
            fail("task pack next probe references an unselected anchor")
        require_text(probe.get("text"), "task pack next probe text", 1024)
    if validate_budget_object(pack.get("budgets"), "task pack.budgets") != bundle["budgets"]:
        fail("task pack budgets differ from the local bundle")
    return pack


def validate_guidance(
    items: list[Any],
    evidence: dict[str, dict[str, Any]],
    selected_anchor_ids: set[str],
    label: str,
) -> None:
    for index, raw in enumerate(items):
        guidance = require_object(raw, f"{label}[{index}]")
        authority = guidance.get("authority")
        if authority not in GUIDANCE_AUTHORITIES:
            fail(f"{label}[{index}] has invalid authority")
        require_text(guidance.get("text"), f"{label}[{index}].text", 1536)
        evidence_ids = require_array(
            guidance.get("evidence_ids", []),
            f"{label}[{index}].evidence_ids",
        )
        if any(item not in evidence for item in evidence_ids):
            fail(f"{label}[{index}] references unknown evidence")
        items_by_id = [evidence[item] for item in evidence_ids]
        if any(
            item.get("anchor_id") and item.get("anchor_id") not in selected_anchor_ids
            for item in items_by_id
        ):
            fail(f"{label}[{index}] cites evidence outside visible anchors")

        def is_test_or_example(path: str) -> bool:
            lowered = path.lower()
            return (
                lowered.endswith("_test.go")
                or "/testdata/" in lowered
                or lowered.startswith("testdata/")
                or "/examples/" in lowered
                or lowered.startswith("examples/")
                or "/example/" in lowered
                or lowered.startswith("example/")
            )

        if authority == "missing_evidence":
            valid = not evidence_ids
        elif authority == "task_provided":
            valid = bool(items_by_id) and all(item.get("kind") == "task_provided" for item in items_by_id)
        elif authority == "repository_document":
            valid = bool(items_by_id) and all(item.get("kind") == "document_claim" for item in items_by_id)
        elif authority == "repository_test_or_example":
            valid = bool(items_by_id) and all(
                item.get("kind") != "task_provided" and is_test_or_example(item.get("path", ""))
                for item in items_by_id
            )
        else:
            valid = bool(items_by_id) and all(item.get("kind") == "repository_fact" for item in items_by_id)
        if not valid:
            fail(f"{label}[{index}] authority does not match its exact evidence kind/path")


def task_pack_sufficient(pack: dict[str, Any], attempt_state: str) -> bool:
    _ = attempt_state
    anchors = pack.get("investigation_anchors", [])
    if pack.get("locality") == "broad_dynamic" or len(anchors) < 2:
        return False

    grounded_join = any(
        join.get("support_ids")
        and join.get("support_type")
        in {"locally_observed", "document_supported", "model_hypothesis"}
        for join in pack.get("evidence_joins", [])
    )

    evidence_anchors: dict[str, dict[str, Any]] = {}
    for anchor in anchors:
        if not isinstance(anchor, dict):
            continue
        for evidence_id in anchor.get("evidence_ids", []):
            evidence_anchors[evidence_id] = anchor

    useful_hypothesis = False
    for clause in pack.get("working_hypothesis", []):
        if not isinstance(clause, dict):
            continue
        support_ids = clause.get("support_ids", [])
        if (
            clause.get("status") == "supported"
            and clause.get("relation_ids")
            and support_ids
        ):
            useful_hypothesis = True
            break
        if (
            clause.get("status") == "plausible"
            and len(support_ids) >= 2
            and not str(clause.get("text", "")).startswith(
                "A relationship involving the retained evidence for"
            )
            and not str(clause.get("text", "")).startswith(
                "A relationship in the cited bounded context"
            )
            and len({
                evidence_anchors[evidence_id].get("id")
                for evidence_id in support_ids
                if evidence_id in evidence_anchors
            }) >= 2
        ):
            useful_hypothesis = True
            break
    strong_join_anchors: set[str] = set()
    for join in pack.get("evidence_joins", []):
        if not isinstance(join, dict):
            continue
        if join.get("support_type") == "document_supported" or (
            join.get("support_type") == "locally_observed"
            and join.get("relation_kind") != "shared_exact_task_term"
        ):
            strong_join_anchors.add(join.get("left_anchor_id"))
            strong_join_anchors.add(join.get("right_anchor_id"))

    generic_verification_terms = {
        "test", "tests", "error", "errors", "config", "configuration",
        "context", "package", "example",
    }

    def anchor_matches_found_term(anchor: dict[str, Any], found_terms: Any) -> bool:
        corpus = "\n".join(
            str(anchor.get(field, ""))
            for field in ("path", "symbol", "section")
        ).lower()
        for raw in found_terms:
            if not isinstance(raw, str):
                continue
            term = raw.strip("`'\".,;:()[]{}<>").lower()
            if (
                len(term.encode("utf-8")) >= 4
                and term not in generic_verification_terms
                and term in corpus
            ):
                return True
        return False

    def has_task_relevant_verification() -> bool:
        found_terms = pack.get("task_interpretation", {}).get("repository_terms_found", [])
        for guidance in pack.get("verify", {}).get("steps", []):
            if (
                not isinstance(guidance, dict)
                or guidance.get("authority") == "missing_evidence"
                or not guidance.get("evidence_ids")
            ):
                continue
            if guidance.get("authority") == "task_provided":
                continue
            for evidence_id in guidance["evidence_ids"]:
                anchor = evidence_anchors.get(evidence_id)
                if anchor and (
                    anchor.get("id") in strong_join_anchors
                    or anchor_matches_found_term(anchor, found_terms)
                ):
                    return True
        return False

    grounded_guidance = any(
        isinstance(guidance, dict)
        and guidance.get("authority") != "missing_evidence"
        and bool(guidance.get("evidence_ids"))
        and (
            guidance.get("authority") != "task_provided"
            or pack.get("task_observation_concrete") is True
        )
        for guidance in pack.get("reproduce_or_observe", [])
    )

    if pack.get("locality") == "extension_contribution":
        anchor_by_id = {
            anchor.get("id"): anchor
            for anchor in anchors
            if isinstance(anchor, dict) and anchor.get("id")
        }
        starts = {
            anchor_id
            for anchor_id, anchor in anchor_by_id.items()
            if anchor.get("role") == "integration_boundary"
        }
        targets: set[str] = set()
        for guidance in pack.get("verify", {}).get("steps", []):
            if not isinstance(guidance, dict) or guidance.get("authority") in {
                "missing_evidence", "task_provided",
            }:
                continue
            for evidence_id in guidance.get("evidence_ids", []):
                anchor = evidence_anchors.get(evidence_id)
                if anchor and anchor.get("role") in {
                    "verification_anchor", "representative_implementation",
                }:
                    targets.add(anchor.get("id"))
        adjacent: dict[str, set[str]] = {}
        for join in pack.get("evidence_joins", []):
            if not isinstance(join, dict):
                continue
            actionable = join.get("support_type") in {
                "document_supported", "model_hypothesis",
            } or (
                join.get("support_type") == "locally_observed"
                and join.get("relation_kind") != "shared_exact_task_term"
            )
            if actionable:
                left = join.get("left_anchor_id")
                right = join.get("right_anchor_id")
                adjacent.setdefault(left, set()).add(right)
                adjacent.setdefault(right, set()).add(left)
        seen: set[str] = set()
        frontier = list(starts)
        connected = False
        while frontier:
            current = frontier.pop(0)
            if current in seen:
                continue
            seen.add(current)
            if current in targets:
                connected = True
                break
            frontier.extend(sorted(adjacent.get(current, set()) - seen))
        if len(pack.get("likely_areas", [])) < 2 or not connected:
            return False
    return bool(
        grounded_join
        and useful_hypothesis
        and grounded_guidance
        and has_task_relevant_verification()
    )


def validate_provider_metrics(value: Any, locality: str, label: str) -> dict[str, Any]:
    provider = require_object(value, label)
    call_limits = {"local_exact": 1, "bounded_cross_file": 2, "extension_contribution": 3, "broad_dynamic": 4}
    calls = require_nonnegative_int(provider.get("calls"), f"{label}.calls", 4)
    if calls > call_limits[locality]:
        fail(f"{label}.calls exceeds the approved {locality} policy")
    for field in (
        "transport_attempts", "request_bytes", "response_bytes", "input_tokens", "output_tokens",
        "prompt_cache_hit_tokens", "prompt_cache_miss_tokens", "latency_millis",
    ):
        require_nonnegative_int(provider.get(field), f"{label}.{field}")
    if provider["transport_attempts"] < calls or provider["transport_attempts"] > 4:
        fail(f"{label}.transport_attempts does not match bounded call accounting")
    return provider


def validate_attempt_response_binding(
    attempt: dict[str, Any],
    attempt_state: str,
    provider: dict[str, Any],
    bundle: dict[str, Any],
    pack: dict[str, Any],
) -> None:
    raw_response = attempt.get("raw_response", "")
    omitted_reason = attempt.get("raw_response_omitted_reason", "")
    response_sha = attempt.get("response_sha256", "")
    reduction_error = attempt.get("reduction_error", "")
    warnings = attempt.get("warnings", [])
    if not all(isinstance(item, str) and item for item in require_array(warnings, "task attempt.warnings")):
        fail("task attempt warnings must be non-empty strings")
    for field, value in (
        ("raw_response", raw_response),
        ("raw_response_omitted_reason", omitted_reason),
        ("response_sha256", response_sha),
        ("reduction_error", reduction_error),
    ):
        if not isinstance(value, str):
            fail(f"task attempt.{field} must be text when present")

    if raw_response:
        raw = raw_response.encode("utf-8")
        if len(raw) > 512 * 1024 or omitted_reason:
            fail("task attempt raw response is outside the replayable response contract")
        if provider["response_bytes"] != len(raw) or response_sha != sha256_bytes(raw):
            fail("task attempt raw response bytes do not match their saved identity")
    elif omitted_reason:
        if omitted_reason not in {"size_limit", "secret_like_content"}:
            fail("task attempt raw response omission reason is invalid")
        require_sha(response_sha, "task attempt.response_sha256")
        if provider["response_bytes"] <= 0:
            fail("task attempt omitted response lacks byte accounting")
        if omitted_reason == "size_limit" and provider["response_bytes"] <= 512 * 1024:
            fail("task attempt size omission is below the replayable response bound")
    elif response_sha:
        fail("task attempt response hash is present without replayable or explicitly omitted bytes")

    if attempt_state in {"accepted", "accepted_with_rejections"}:
        if not raw_response or omitted_reason or reduction_error:
            fail("accepted task attempt does not retain its exact replayable raw response")
        try:
            proposal = json.loads(raw_response)
        except json.JSONDecodeError as exc:
            fail(f"accepted task attempt raw response is not valid JSON: {exc}")
        if not isinstance(proposal, dict) or proposal.get("version") != 1:
            fail("accepted task attempt raw response is not a v1 reducer proposal")
        allowed_proposal_fields = {
            "version", "task_interpretation", "likely_areas", "anchors", "evidence_joins",
            "working_hypothesis", "reproduce_or_observe", "verify", "next_probes",
        }
        required_proposal_fields = allowed_proposal_fields - {"evidence_joins"}
        if not required_proposal_fields.issubset(proposal) or not set(proposal).issubset(
            allowed_proposal_fields
        ):
            fail("accepted task attempt raw response is not the strict v1 proposal shape")
        proposed_anchors = require_array(proposal.get("anchors"), "raw proposal anchors", 0, 8)
        proposed_ids = [
            require_object(item, "raw proposal anchor").get("anchor_id")
            for item in proposed_anchors
        ]
        if proposed_ids != [item["id"] for item in pack["investigation_anchors"]] or any(
            item not in {anchor["id"] for anchor in bundle["anchors"]}
            for item in proposed_ids
        ):
            fail("accepted task attempt raw response does not bind the saved visible anchors")
        for field in (
            "likely_areas", "working_hypothesis", "reproduce_or_observe", "next_probes",
        ):
            require_array(proposal.get(field), f"raw proposal {field}", 1)
        verification = require_object(proposal.get("verify"), "raw proposal verify")
        require_array(verification.get("steps"), "raw proposal verify.steps", 1)
        if attempt_state == "accepted" and warnings:
            fail("fully accepted task attempt unexpectedly records reducer warnings")
        if attempt_state == "accepted_with_rejections" and not warnings:
            fail("accepted-with-rejections task attempt omits its reducer warnings")
    elif attempt_state == "rejected":
        if provider["response_bytes"] <= 0 or not response_sha or not reduction_error.strip():
            fail("rejected task attempt lacks response identity or reduction error")
        if not raw_response and not omitted_reason:
            fail("rejected task attempt neither retains nor explicitly omits its response")
    elif attempt_state == "provider_failed":
        if reduction_error != "provider call failed; response was not used":
            fail("provider-failed task attempt has an invalid reduction error")
        if provider["response_bytes"] > 0 and not (raw_response or omitted_reason):
            fail("provider-failed task attempt loses returned response identity")
    else:
        expected_warnings = (
            ["Semantic synthesis was skipped because bounded retrieval retained fewer than three source anchors."]
            if attempt_state == "skipped_insufficient_evidence"
            else []
        )
        if (
            any(provider.values())
            or raw_response
            or omitted_reason
            or response_sha
            or reduction_error
            or warnings != expected_warnings
        ):
            fail("skipped task attempt contains impossible provider/response accounting")


def validate_report_binding(
    report_path: Path,
    manifest_path: Path,
    bundle: dict[str, Any],
    pack: dict[str, Any],
    status: dict[str, Any],
    provenance: dict[str, Any],
    artifact_hashes: dict[str, str],
    exact_workspace: dict[str, Any],
) -> None:
    if report_path.is_symlink() or manifest_path.is_symlink():
        fail("Task Lens report and run manifest must be regular files")
    report_raw = report_path.read_bytes()
    report = require_object(read_json(report_path), "Task Lens report projection")
    manifest = require_object(read_json(manifest_path), "Task Lens run manifest")
    if manifest.get("version") != 3:
        fail("Task Lens run manifest is not the current authorized version")
    if manifest.get("report_sha256") != sha256_bytes(report_raw):
        fail("Task Lens run manifest does not bind the exact report.json bytes")
    format_version = require_nonnegative_int(report.get("format_version"), "report.format_version")
    if format_version < 1 or manifest.get("report_format_version") != format_version:
        fail("Task Lens report format differs from its run manifest")

    repository_root = (provenance["episode_root"] / provenance["worktree"]).resolve()
    repository_state = require_object(
        manifest.get("repository_state"),
        "Task Lens run manifest repository_state",
    )
    if (
        repository_state.get("version") != 2
        or repository_state.get("identity") != str(repository_root)
        or repository_state.get("head") != provenance["base_revision"]
        or repository_state.get("dirty") not in (None, [])
        or repository_state.get("submodules") not in (None, [])
        or manifest.get("analysis_root") != str(repository_root)
    ):
        fail(
            "Task Lens run manifest does not describe the exact clean detached worktree: "
            f"expected root={repository_root}, revision={provenance['base_revision']}; "
            f"got {repository_state!r}, analysis_root={manifest.get('analysis_root')!r}"
        )
    canonical_state = {
        "version": 2,
        "identity": str(repository_root),
        "head": provenance["base_revision"],
        "dirty": [],
    }
    if repository_state.get("submodules"):
        canonical_state["submodules"] = repository_state["submodules"]
    expected_state_sha = sha256_bytes(
        json.dumps(canonical_state, separators=(",", ":")).encode("utf-8")
    )
    if manifest.get("repository_state_sha256") != expected_state_sha:
        fail("Task Lens run manifest repository-state hash is invalid")

    material = require_object(manifest.get("material_inputs"), "Task Lens material inputs")
    expected_artifact_hashes = {
        "task_bundle_sha256": artifact_hashes["task_investigation_bundle.json"],
        "task_attempt_sha256": artifact_hashes["task_investigation_attempt.json"],
        "task_pack_sha256": artifact_hashes["task_investigation.json"],
        "task_status_sha256": artifact_hashes["task_investigation_status.json"],
    }
    if (
        material.get("selected_revision") != provenance["base_revision"]
        or material.get("model_bundle_sha256", "") != ""
        or not isinstance(material.get("input_policy_version"), str)
        or not material["input_policy_version"]
        or not isinstance(material.get("architecture_contract"), int)
        or material["architecture_contract"] <= 0
        or material.get("report_contract") != format_version
    ):
        fail("Task Lens run manifest material inputs do not describe the dedicated pipeline")
    for field, expected in expected_artifact_hashes.items():
        if material.get(field) != expected:
            fail(f"Task Lens run manifest does not bind exact {field}")
    freshness = require_object(manifest.get("freshness"), "Task Lens run manifest freshness")
    if (
        freshness.get("version") != 1
        or freshness.get("state") != "fresh"
        or freshness.get("analyzed_changes") is not False
        or freshness.get("unrelated_changes") is not False
    ):
        fail("Task Lens strict snapshot is not recorded as fresh")

    captured_inputs = require_array(
        manifest.get("captured_inputs"),
        "Task Lens run manifest captured_inputs",
    )
    canonical_inputs: list[dict[str, Any]] = []
    previous_path = ""
    for index, raw in enumerate(sorted(captured_inputs, key=lambda item: item.get("path", ""))):
        item = require_object(raw, f"Task Lens captured input {index}")
        path = validate_relative_path(item.get("path", ""), "Task Lens captured input path")
        if previous_path and path <= previous_path:
            fail("Task Lens captured inputs are not uniquely path-sorted")
        previous_path = path
        expected_id = sha256_bytes(("captured-input-v1\x00" + path).encode("utf-8"))
        if item.get("version") != 1 or item.get("id") != expected_id:
            fail("Task Lens captured input identity is invalid")
        if item.get("kind") not in {"file", "symlink", "missing"}:
            fail("Task Lens captured input kind is invalid")
        stages = require_array(item.get("stages"), "Task Lens captured input stages", 1)
        if stages != sorted(set(stages)) or "report_evidence" not in stages:
            fail("Task Lens captured input stage accounting is invalid")
        canonical = {
            "version": 1,
            "id": expected_id,
            "path": path,
            "kind": item["kind"],
        }
        for field in ("mode", "content_sha256", "owning_module_id", "owning_package"):
            if item.get(field, ""):
                canonical[field] = item[field]
        canonical["stages"] = stages
        canonical_inputs.append(canonical)
    captured_sha = sha256_bytes(
        json.dumps(canonical_inputs, separators=(",", ":")).encode("utf-8")
    )
    if manifest.get("captured_inputs_sha256") != captured_sha:
        fail("Task Lens run manifest captured-input hash is invalid")
    captured_paths = {item["path"] for item in canonical_inputs}
    if not set(bundle["allowed_paths"]).issubset(captured_paths):
        fail("Task Lens run manifest does not bind every model-visible repository file")

    expected_openable = sorted({anchor["path"] for anchor in pack["investigation_anchors"]})
    if manifest.get("openable_paths", []) != expected_openable or manifest.get("components", []) not in ([], None):
        fail("Task Lens run manifest exposes authority outside the task pack")
    if report.get("openable_paths", []) != expected_openable:
        fail("Task Lens report openable paths differ from its run manifest")
    if report.get("captured_revision") != provenance["base_revision"]:
        fail("Task Lens report projection captured the wrong revision")
    workspace = require_object(report.get("task_investigation"), "Task Lens report task projection")
    expected_workspace = {
        "task_id": bundle["id"],
        "repository": bundle["repository"]["identity"],
        "task": bundle["task"]["text"],
        "state": status["state"],
        "sufficient": status["sufficient"],
        "locality": bundle["locality"],
        "stages_skipped": bundle["stages_skipped"],
        "budget": bundle["budgets"],
        "provider": status["provider"],
        "captured_revision": provenance["base_revision"],
    }
    for field, expected in expected_workspace.items():
        if workspace.get(field) != expected:
            fail(f"Task Lens report task projection changed {field}")
    workspace_hash_fields = {
        "bundle_sha256": expected_artifact_hashes["task_bundle_sha256"],
        "attempt_sha256": expected_artifact_hashes["task_attempt_sha256"],
        "pack_sha256": expected_artifact_hashes["task_pack_sha256"],
        "status_sha256": expected_artifact_hashes["task_status_sha256"],
    }
    for field, expected in workspace_hash_fields.items():
        if workspace.get(field) != expected:
            fail(f"Task Lens report workspace does not bind exact {field}")
    projected_anchors = require_array(
        workspace.get("anchors"),
        "Task Lens report projected anchors",
        0,
        8,
    )
    if len(projected_anchors) != len(pack["investigation_anchors"]):
        fail("Task Lens report anchor count differs from its saved pack")
    for projected, source in zip(projected_anchors, pack["investigation_anchors"]):
        projected = require_object(projected, "Task Lens report projected anchor")
        for field in ("path", "symbol", "role", "start_line", "end_line", "why"):
            if projected.get(field) != source.get(field):
                fail(f"Task Lens report anchor projection changed {field}")
    if workspace != exact_workspace:
        fail("Task Lens report workspace differs from exact candidate-binary Go projection")
    report_text = report_raw.decode("utf-8")
    for item in bundle["evidence"]:
        if item["id"] in report_text:
            fail("Task Lens user-facing report exposes an opaque evidence ID")
    for anchor in bundle["anchors"]:
        if anchor["id"] in report_text:
            fail("Task Lens user-facing report exposes an opaque anchor ID")


def validate_production_artifacts(
    run_dir: Path,
    provenance: dict[str, Any],
    validator_binary: Path,
) -> dict[str, Any]:
    paths = artifact_paths(run_dir)
    artifact_directories = {path.parent.resolve() for path in paths.values()}
    if len(artifact_directories) != 1:
        fail("Task Lens artifacts are split across multiple run directories")
    artifact_directory = next(iter(artifact_directories))
    latest = run_dir / "latest"
    if not latest.is_symlink():
        fail(f"Task Lens debug root is missing the expected latest symlink: {latest}")
    latest_target = os.readlink(latest)
    if Path(latest_target).is_absolute() or Path(latest_target).parts != (artifact_directory.name,):
        fail(f"Task Lens latest symlink is not a single relative run basename: {latest_target!r}")
    if latest.resolve() != artifact_directory:
        fail("Task Lens latest symlink does not resolve to the unique artifact run")
    reports = sorted(path for path in run_dir.rglob("report.html") if path.is_file())
    report_json = sorted(path for path in run_dir.rglob("report.json") if path.is_file())
    run_manifests = sorted(path for path in run_dir.rglob("run_manifest.json") if path.is_file())
    if len(reports) != 1 or len(report_json) != 1 or len(run_manifests) != 1:
        fail("Task Lens output must have one report.html, report.json, and run_manifest.json")
    exact_workspace = exact_task_workspace_replay(
        validator_binary,
        paths,
        report_json[0],
    )
    artifact_hashes = {name: sha256_file(path) for name, path in paths.items()}
    parsed = {name: read_json(path) for name, path in paths.items()}
    status = require_object(parsed["task_investigation_status.json"], "task status")
    attempt = require_object(parsed["task_investigation_attempt.json"], "task attempt")
    state = status.get("state")
    if status.get("version") != 1 or state not in {"accepted", "accepted_partial", "partial_local"}:
        fail("task status has an unsupported schema version or state")
    sufficient = status.get("sufficient")
    if not isinstance(sufficient, bool):
        fail("task status sufficient must be boolean")
    if status.get("captured_revision") != provenance["base_revision"] or status.get("tree_hash") != provenance["tree_hash"]:
        fail("task status revision/tree does not match the episode base")
    task_text = prompt_safe_task_text(
        (provenance["episode_root"] / "task.md").read_text(encoding="utf-8")
    )
    bundle = validate_bundle(
        parsed["task_investigation_bundle.json"],
        provenance,
        task_text,
        state == "partial_local" and not sufficient,
    )
    bundle_sha = go_compact_json_sha(bundle)
    if attempt.get("version") != 1 or attempt.get("bundle_sha256") != bundle_sha:
        fail("task attempt does not bind the saved bundle")
    if attempt.get("prompt_version") != "task-investigation-pack-json-v1":
        fail("task attempt prompt version is not the frozen v0 contract")
    require_sha(attempt.get("prompt_sha256"), "task attempt.prompt_sha256")
    attempt_state = attempt.get("state")
    if attempt_state not in ATTEMPT_STATUS_STATES:
        fail("task attempt has an invalid state")
    expected_status_state, expects_provider = ATTEMPT_STATUS_STATES[attempt_state]
    if state != expected_status_state:
        fail("task attempt state does not map to the saved status state")
    pack = validate_pack(parsed["task_investigation.json"], bundle, provenance, bundle_sha, state)
    if status.get("task_id") != bundle["id"] or pack.get("id") != status.get("task_id"):
        fail("task status ID does not bind bundle and pack")
    if status.get("bundle_sha256") != bundle_sha:
        fail("task status bundle hash does not match bundle content")
    if status.get("attempt_sha256") != sha256_file(paths["task_investigation_attempt.json"]):
        fail("task status attempt hash does not match attempt artifact bytes")
    if status.get("pack_sha256") != sha256_file(paths["task_investigation.json"]):
        fail("task status pack hash does not match pack artifact bytes")
    locality = status.get("locality")
    if locality != bundle["locality"] or pack["locality"] != locality:
        fail("task locality differs across status, bundle, and pack")
    if status.get("stages_skipped") != bundle["stages_skipped"]:
        fail("task status stage accounting differs from bundle")
    provider = validate_provider_metrics(status.get("provider"), locality, "task status.provider")
    if validate_provider_metrics(attempt.get("provider"), locality, "task attempt.provider") != provider:
        fail("task status provider metrics differ from attempt metrics")
    if provider["calls"] != int(expects_provider):
        fail("task attempt state does not match provider call accounting")
    if expects_provider and provider["request_bytes"] <= 0:
        fail("provider-backed task attempt has no request-byte accounting")
    validate_attempt_response_binding(attempt, attempt_state, provider, bundle, pack)
    budgets = validate_budget_object(status.get("budgets"), "task status.budgets")
    if budgets != bundle["budgets"] or pack["budgets"] != budgets:
        fail("task status budgets differ from bundle/pack")
    expected_sufficient = exact_workspace.get("sufficient")
    if not isinstance(expected_sufficient, bool):
        fail("exact candidate-binary Task Lens projection omits sufficiency")
    if sufficient != expected_sufficient:
        fail("task status sufficiency differs from exact candidate-binary replay")
    pack_warnings = require_array(pack.get("warnings", []), "task pack.warnings", 0, 1)
    if sufficient:
        expected_pack_warnings: list[str] = []
    elif attempt_state in {"accepted", "accepted_with_rejections"}:
        expected_pack_warnings = [
            "This model-edited pack remains partial because the bounded evidence does not support a complete, actionable task lens."
        ]
    else:
        expected_pack_warnings = [
            "This deterministic local pack remains partial because the bounded evidence does not support a complete, actionable task lens."
        ]
    if pack_warnings != expected_pack_warnings:
        fail("task pack warning does not match content-based sufficiency and provenance")
    expected_status_warnings = list(dict.fromkeys(attempt.get("warnings", []) + pack_warnings))
    if status.get("warnings", []) != expected_status_warnings:
        fail("task status warnings do not match the attempt and finalized pack")
    for forbidden in FORBIDDEN_GENERIC_ARTIFACTS:
        if (artifact_directory / forbidden).exists() or (artifact_directory / forbidden).is_symlink():
            fail(f"Task Lens cheap exit emitted forbidden generic artifact {forbidden}")
    validate_report_binding(
        report_json[0],
        run_manifests[0],
        bundle,
        pack,
        status,
        provenance,
        artifact_hashes,
        exact_workspace,
    )
    if SECRET_RE.search(b"".join(path.read_bytes() for path in paths.values())):
        fail("Task Lens artifacts contain an obvious credential pattern")
    return {
        "state": state,
        "sufficient": sufficient,
        "task_id": status.get("task_id"),
        "locality": status.get("locality"),
        "provider": provider,
        "budgets": budgets,
        "metrics": bundle["metrics"],
        "stages_skipped": bundle["stages_skipped"],
        "artifact_paths": {name: str(path.relative_to(run_dir)) for name, path in paths.items()},
        "artifact_run_dir": str(artifact_directory.relative_to(run_dir.resolve())),
        "latest_target": latest_target,
        "report_path": str(reports[0].relative_to(run_dir)) if len(reports) == 1 else None,
    }


def inventory_files(
    root: Path,
    excluded_paths: set[str] | None = None,
    allowed_symlinks: dict[str, str] | None = None,
) -> list[dict[str, Any]]:
    excluded_paths = excluded_paths or set()
    allowed_symlinks = allowed_symlinks or {}
    items = []
    for path in sorted(root.rglob("*")):
        relative = path.relative_to(root).as_posix()
        if path.is_symlink():
            target = os.readlink(path)
            if allowed_symlinks.get(relative) != target:
                fail(f"sealed trees contain an unexpected symlink: {path} -> {target}")
            raw_target = target.encode("utf-8", "surrogateescape")
            items.append(
                {
                    "path": relative,
                    "kind": "symlink",
                    "target": target,
                    "bytes": len(raw_target),
                    "sha256": sha256_bytes(raw_target),
                }
            )
            continue
        if not path.is_file():
            continue
        if relative in excluded_paths:
            continue
        items.append(
            {
                "path": relative,
                "kind": "file",
                "bytes": path.stat().st_size,
                "sha256": sha256_file(path),
            }
        )
    return items


def verify_file_inventory(
    root: Path,
    items: Any,
    excluded_paths: set[str] | None = None,
) -> None:
    excluded_paths = {"SEALED.json"} if excluded_paths is None else excluded_paths
    if not isinstance(items, list):
        fail(f"invalid sealed inventory under {root}")
    expected_paths: set[str] = set()
    for item in items:
        if not isinstance(item, dict) or not isinstance(item.get("path"), str):
            fail(f"invalid sealed inventory entry under {root}")
        relative = validate_relative_path(item["path"], "sealed path")
        path = root / relative
        if item.get("kind") == "symlink":
            if not path.is_symlink() or os.readlink(path) != item.get("target"):
                fail(f"sealed symlink changed: {path}")
            raw_target = os.readlink(path).encode("utf-8", "surrogateescape")
            if len(raw_target) != item.get("bytes") or sha256_bytes(raw_target) != item.get("sha256"):
                fail(f"sealed symlink target hash changed: {path}")
        elif item.get("kind") == "file":
            if not path.is_file() or path.is_symlink():
                fail(f"sealed file is missing or not regular: {path}")
            if path.stat().st_size != item.get("bytes") or sha256_file(path) != item.get("sha256"):
                fail(f"sealed file changed: {path}")
        else:
            fail(f"sealed inventory has an invalid kind for {path}")
        expected_paths.add(relative)
    actual = {
        path.relative_to(root).as_posix()
        for path in root.rglob("*")
        if (path.is_file() or path.is_symlink())
        and path.relative_to(root).as_posix() not in excluded_paths
    }
    if actual != expected_paths:
        fail(f"sealed attempt file set changed under {root}")


def seal_attempt(
    root: Path,
    phase: str,
    episode_root: Path,
    attempt_dir: Path,
    provenance: dict[str, Any],
    validator_binary: Path | None = None,
) -> dict[str, Any]:
    attempt_record = validate_harness_attempt(root, phase, attempt_dir, provenance)
    if validator_binary is None:
        if phase == "holdout":
            validator_binary = root / "holdout/inputs/repomap"
        elif (root / "freeze/repomap").is_file():
            validator_binary = root / "freeze/repomap"
        else:
            fail("development seal requires the exact candidate validator binary")
    validator_binary = resolve_existing(validator_binary, "file")
    if sha256_file(validator_binary) != attempt_record["binary_sha256"]:
        fail("exact Task Lens validator binary differs from the launched binary")
    seal_path = attempt_dir / "SEALED.json"
    if seal_path.exists():
        validate_production_artifacts(attempt_dir / "run", provenance, validator_binary)
        seal = read_json(seal_path)
        verify_file_inventory(attempt_dir, seal.get("files"))
        return seal
    run_dir = attempt_dir / "run"
    summary = validate_production_artifacts(run_dir, provenance, validator_binary)
    metrics = {
        "version": VERSION,
        "episode_id": provenance["episode_id"],
        "phase": phase,
        "wall_millis": attempt_record.get("wall_millis"),
        "return_code": attempt_record.get("return_code"),
        "locality": summary["locality"],
        "provider": summary["provider"],
        "budgets": summary["budgets"],
        "local_retrieval": summary["metrics"],
        "stages_skipped": summary["stages_skipped"],
    }
    write_json_new(attempt_dir / "METRICS.json", metrics)
    files = inventory_files(
        attempt_dir,
        {"SEALED.json"},
        {"run/latest": summary["latest_target"]},
    )
    artifact_hashes = {
        name: sha256_file(run_dir / relative)
        for name, relative in summary["artifact_paths"].items()
    }
    seal = {
        "version": VERSION,
        "phase": phase,
        "episode_id": provenance["episode_id"],
        "base_revision": provenance["base_revision"],
        "tree_hash": provenance["tree_hash"],
        "task_sha256": provenance["task_sha256"],
        "state": summary["state"],
        "sufficient": summary["sufficient"],
        "artifact_sha256": artifact_hashes,
        "files": files,
        "sealed_at": utc_now(),
    }
    if phase == "holdout":
        freeze = verify_freeze(root, require_frozen_runner=True)
        seal["freeze_manifest_sha256"] = sha256_file(root / "FREEZE_MANIFEST.json")
        seal["frozen_binary_sha256"] = freeze["binary"]["sha256"]
    write_json_new(seal_path, seal, 0o444)
    for item in files:
        if item["kind"] == "file":
            os.chmod(attempt_dir / item["path"], 0o444)
    return seal


def validate_harness_attempt(
    root: Path,
    phase: str,
    attempt_dir: Path,
    provenance: dict[str, Any],
    expected_binary_sha: str | None = None,
) -> dict[str, Any]:
    if phase == "holdout":
        frozen_binary_sha = verify_freeze(root, require_frozen_runner=True)["binary"]["sha256"]
        if expected_binary_sha is not None and expected_binary_sha != frozen_binary_sha:
            fail("requested holdout binary SHA differs from the freeze")
        expected_binary_sha = frozen_binary_sha
    started_path = attempt_dir / "ATTEMPT_STARTED.json"
    attempt_path = attempt_dir / "HARNESS_ATTEMPT.json"
    started = require_object(read_json(started_path), "harness attempt start")
    record = require_object(read_json(attempt_path), "harness attempt record")
    if started.get("version") != VERSION or record.get("version") != VERSION:
        fail("harness attempt version is invalid")
    expected = {
        "phase": phase,
        "episode_id": provenance["episode_id"],
        "binary_sha256": expected_binary_sha or record.get("binary_sha256"),
        "task_sha256": provenance["task_sha256"],
        "base_revision": provenance["base_revision"],
        "tree_hash": provenance["tree_hash"],
    }
    for field, value in expected.items():
        if started.get(field) != value or record.get(field) != value:
            fail(f"harness attempt {field} is not bound to its launch/provenance")
    require_sha(record.get("binary_sha256"), "harness attempt binary SHA")
    nonce = started.get("launch_nonce")
    if not isinstance(nonce, str) or not re.fullmatch(r"[0-9a-f]{32}", nonce):
        fail("harness attempt launch nonce is invalid")
    if record.get("launch_nonce") != nonce or record.get("started_record_sha256") != sha256_file(started_path):
        fail("harness attempt record is not bound to its pre-launch record")
    if record.get("one_process_invocation") is not True or record.get("semantic_retry") is not False:
        fail("harness attempt does not attest one process with no semantic retry")
    if record.get("return_code") != 0:
        fail("only a successful Task Lens process may be sealed")
    offline = record.get("offline")
    if not isinstance(offline, bool) or (phase == "holdout" and offline):
        fail("holdout attempts may not use offline development mode")
    expected_command = [
        "repomap", "investigate", "repo", "--task-file", "task.md", "--no-open", "--no-serve",
        "--debug-dir", "run", "--strict-snapshot",
    ]
    if offline:
        expected_command.append("--offline")
    if record.get("command") != expected_command:
        fail("harness attempt command differs from the one-process protocol")
    require_nonnegative_int(record.get("wall_millis"), "harness attempt wall_millis")
    require_text(record.get("started_at"), "harness attempt started_at", 64)
    require_text(record.get("finished_at"), "harness attempt finished_at", 64)
    return record


def run_phase(args: argparse.Namespace) -> None:
    root = Path(args.root).expanduser().resolve()
    prepared, manifest, phase_root = load_phase(root, args.phase)
    source_repo = resolve_existing(prepared["source_repository"], "dir")
    ensure_git_repository(source_repo)
    episodes = episode_map(manifest)
    selected = [args.episode] if args.episode else list(episodes)
    unknown = [episode_id for episode_id in selected if episode_id not in episodes]
    if unknown:
        fail(f"unknown {args.phase} episode: {unknown[0]}")

    if args.phase == "holdout":
        if getattr(args, "offline", False):
            fail("holdout attempts may not use --offline")
        reject_holdout_gold_env()
        assert_no_holdout_gold(root)
        freeze = verify_freeze(root, require_frozen_runner=True)
        binary = phase_root / "inputs/repomap"
        if sha256_file(binary) != freeze["binary"]["sha256"]:
            fail("holdout binary does not match the frozen binary")
    else:
        if not args.binary:
            fail("--binary is required for development runs")
        binary = resolve_existing(args.binary, "file")
    if not os.access(binary, os.X_OK):
        fail(f"Task Lens binary is not executable: {binary}")

    completed = 0
    for episode_id in selected:
        episode_root = phase_root / episode_id
        provenance = verify_episode_source(
            root, source_repo, args.phase, episodes[episode_id], episode_root, manifest["repository"]
        )
        if args.phase == "holdout":
            attempt_dir = holdout_attempt(episode_root)
            if attempt_dir.exists():
                if (attempt_dir / "SEALED.json").is_file() and not args.episode:
                    continue
                fail(f"holdout episode {episode_id} has already consumed its one attempt")
        else:
            attempt_dir = next_dev_attempt(episode_root)
        attempt_dir.mkdir(parents=True, exist_ok=False)
        run_dir = attempt_dir / "run"
        execution_dir = attempt_dir / "execution"
        run_dir.mkdir()
        execution_dir.mkdir()
        command = [
            str(binary),
            "investigate",
            str(episode_root / provenance["worktree"]),
            "--task-file",
            str(episode_root / "task.md"),
            "--no-open",
            "--no-serve",
            "--debug-dir",
            str(run_dir),
            "--strict-snapshot",
        ]
        offline = bool(getattr(args, "offline", False))
        if offline:
            command.append("--offline")
        started = utc_now()
        launch = {
            "version": VERSION,
            "phase": args.phase,
            "episode_id": episode_id,
            "binary_sha256": sha256_file(binary),
            "task_sha256": provenance["task_sha256"],
            "base_revision": provenance["base_revision"],
            "tree_hash": provenance["tree_hash"],
            "launch_nonce": secrets.token_hex(16),
            "started_at": started,
        }
        started_path = attempt_dir / "ATTEMPT_STARTED.json"
        write_json_new(started_path, launch, 0o444)
        start = time.monotonic()
        with (attempt_dir / "stdout.txt").open("xb") as stdout_handle, (
            attempt_dir / "stderr.txt"
        ).open("xb") as stderr_handle:
            result = run_command(
                command,
                cwd=execution_dir,
                check=False,
                stdout=stdout_handle,
                stderr=stderr_handle,
            )
        elapsed = int((time.monotonic() - start) * 1000)
        record = {
            "version": VERSION,
            "phase": args.phase,
            "episode_id": episode_id,
            "one_process_invocation": True,
            "semantic_retry": False,
            "offline": offline,
            "launch_nonce": launch["launch_nonce"],
            "started_record_sha256": sha256_file(started_path),
            "binary_sha256": sha256_file(binary),
            "task_sha256": provenance["task_sha256"],
            "base_revision": provenance["base_revision"],
            "tree_hash": provenance["tree_hash"],
            "command": [
                "repomap",
                "investigate",
                "repo",
                "--task-file",
                "task.md",
                "--no-open",
                "--no-serve",
                "--debug-dir",
                "run",
                "--strict-snapshot",
            ],
            "started_at": started,
            "finished_at": utc_now(),
            "wall_millis": elapsed,
            "return_code": result.returncode,
        }
        if offline:
            record["command"].append("--offline")
        write_json_new(attempt_dir / "HARNESS_ATTEMPT.json", record)
        verify_episode_source(
            root, source_repo, args.phase, episodes[episode_id], episode_root, manifest["repository"]
        )
        sealed = False
        try:
            seal_attempt(
                root,
                args.phase,
                episode_root,
                attempt_dir,
                provenance,
                binary,
            )
            sealed = True
        except HarnessError:
            append_experiment(
                root,
                {
                    "version": VERSION,
                    "phase": args.phase,
                    "episode_id": episode_id,
                    "attempt": str(attempt_dir.relative_to(root)),
                    "binary_sha256": record["binary_sha256"],
                    "base_revision": provenance["base_revision"],
                    "tree_hash": provenance["tree_hash"],
                    "wall_millis": elapsed,
                    "return_code": result.returncode,
                    "sealed": False,
                    "semantic_retry": False,
                    "finished_at": record["finished_at"],
                },
            )
            if result.returncode == 0:
                raise
        else:
            append_experiment(
                root,
                {
                    "version": VERSION,
                    "phase": args.phase,
                    "episode_id": episode_id,
                    "attempt": str(attempt_dir.relative_to(root)),
                    "binary_sha256": record["binary_sha256"],
                    "base_revision": provenance["base_revision"],
                    "tree_hash": provenance["tree_hash"],
                    "wall_millis": elapsed,
                    "return_code": result.returncode,
                    "sealed": True,
                    "semantic_retry": False,
                    "finished_at": record["finished_at"],
                },
            )
        if result.returncode != 0:
            detail = "output was sealed" if sealed else "output was incomplete and remains unsealed"
            fail(f"Task Lens attempt failed for {episode_id} with exit {result.returncode}; {detail}")
        completed += 1

    if args.phase == "holdout":
        maybe_seal_holdout(root)
    append_log(root, f"ran {completed} {args.phase} Task Lens attempt(s); every completed output was sealed")
    refresh_audit(root)


def latest_dev_attempt(episode_root: Path) -> Path:
    attempts_root = episode_root / "attempts"
    attempts = sorted(path for path in attempts_root.iterdir() if path.is_dir()) if attempts_root.is_dir() else []
    if not attempts:
        fail(f"development episode has no attempt: {episode_root.name}")
    return attempts[-1]


def seal_phase(args: argparse.Namespace) -> None:
    root = Path(args.root).expanduser().resolve()
    if args.phase == "holdout":
        reject_holdout_gold_env()
        assert_no_holdout_gold(root)
    prepared, manifest, phase_root = load_phase(root, args.phase)
    validator_binary: Path | None = None
    if args.phase == "dev":
        if not getattr(args, "binary", None):
            fail("--binary is required when sealing development attempts")
        validator_binary = resolve_existing(args.binary, "file")
    source_repo = resolve_existing(prepared["source_repository"], "dir")
    episodes = episode_map(manifest)
    selected = [args.episode] if args.episode else list(episodes)
    for episode_id in selected:
        if episode_id not in episodes:
            fail(f"unknown {args.phase} episode: {episode_id}")
        episode_root = phase_root / episode_id
        provenance = verify_episode_source(
            root, source_repo, args.phase, episodes[episode_id], episode_root, manifest["repository"]
        )
        attempt = latest_dev_attempt(episode_root) if args.phase == "dev" else holdout_attempt(episode_root)
        if not attempt.is_dir():
            fail(f"episode has no attempt to seal: {episode_id}")
        if args.phase == "holdout" and not (attempt / "SEALED.json").is_file():
            fail(f"holdout seal command only verifies an existing launch-time seal: {episode_id}")
        seal_attempt(
            root,
            args.phase,
            episode_root,
            attempt,
            provenance,
            validator_binary,
        )
    if args.phase == "holdout":
        maybe_seal_holdout(root)
    append_log(root, f"verified {args.phase} episode seals")
    refresh_audit(root)


def verify_episode_seal(attempt: Path) -> dict[str, Any]:
    seal_path = attempt / "SEALED.json"
    if not seal_path.is_file():
        fail(f"episode attempt is not sealed: {attempt}")
    seal = read_json(seal_path)
    verify_file_inventory(attempt, seal.get("files"))
    return seal


def collect_implementation_inventory(repo: Path, excluded_root: Path) -> list[dict[str, Any]]:
    tracked_result = run_command(
        ("git", "-C", str(repo), "ls-files", "-z", "--cached"),
        check=True,
    )
    tracked = {
        raw.decode("utf-8", "surrogateescape")
        for raw in tracked_result.stdout.split(b"\0")
        if raw
    }
    result = run_command(
        ("git", "-C", str(repo), "ls-files", "-z", "--cached", "--others", "--exclude-standard"),
        check=True,
    )
    repository_root = repo.resolve()
    excluded = excluded_root.resolve()
    try:
        excluded_relative = excluded.relative_to(repository_root)
    except ValueError:
        excluded_relative = None
    items = []
    for raw in result.stdout.split(b"\0"):
        if not raw:
            continue
        relative = raw.decode("utf-8", "surrogateescape")
        relative_path = Path(relative)
        if excluded_relative is not None and (
            relative_path == excluded_relative or excluded_relative in relative_path.parents
        ):
            continue
        path = repo / relative
        if path.is_symlink():
            target = os.readlink(path).encode("utf-8", "surrogateescape")
            items.append(
                {
                    "path": relative,
                    "kind": "symlink",
                    "bytes": len(target),
                    "sha256": sha256_bytes(target),
                    "tracked": relative in tracked,
                }
            )
        elif path.is_file():
            items.append(
                {
                    "path": relative,
                    "kind": "file",
                    "bytes": path.stat().st_size,
                    "sha256": sha256_file(path),
                    "tracked": relative in tracked,
                }
            )
        else:
            fail(f"implementation inventory path is unavailable: {path}")
    return sorted(items, key=lambda item: item["path"])


def copy_implementation_snapshot(repo: Path, destination: Path, items: Sequence[dict[str, Any]]) -> None:
    """Copy the exact tracked-plus-untracked working-tree bytes selected for freeze."""
    destination.mkdir(parents=True, exist_ok=False)
    for item in items:
        relative = validate_relative_path(item.get("path", ""), "implementation snapshot path")
        source = repo / relative
        target = destination / relative
        target.parent.mkdir(parents=True, exist_ok=True)
        if item.get("kind") == "symlink":
            os.symlink(os.readlink(source), target)
        elif item.get("kind") == "file":
            copy_new(source, target, 0o444)
        else:
            fail(f"implementation inventory has an invalid kind for {relative}")


def verify_implementation_snapshot(
    root: Path,
    implementation: dict[str, Any],
) -> list[dict[str, Any]]:
    inventory_reference = require_object(
        implementation.get("source_inventory"), "freeze source inventory"
    )
    inventory = require_object(
        read_json(root / inventory_reference["path"]), "frozen source inventory"
    )
    if inventory.get("version") != VERSION:
        fail("frozen source inventory has an invalid version")
    items = require_array(inventory.get("files"), "frozen source inventory files", 1)
    snapshot = require_object(implementation.get("source_snapshot"), "freeze source snapshot")
    snapshot_root = root / validate_relative_path(snapshot.get("path", ""), "source snapshot path")
    if snapshot_root.is_symlink() or not snapshot_root.is_dir():
        fail(f"frozen implementation source snapshot is missing: {snapshot_root}")
    if snapshot.get("file_count") != len(items):
        fail("frozen implementation source snapshot count differs from its inventory")

    expected_paths: set[str] = set()
    for raw in items:
        item = require_object(raw, "frozen source inventory entry")
        relative = validate_relative_path(item.get("path", ""), "frozen implementation path")
        if not isinstance(item.get("tracked"), bool):
            fail(f"frozen source inventory tracked flag is invalid: {relative}")
        path = snapshot_root / relative
        expected_bytes = require_nonnegative_int(item.get("bytes"), f"frozen source bytes for {relative}")
        expected_sha = require_sha(item.get("sha256"), f"frozen source SHA for {relative}")
        if item.get("kind") == "symlink":
            if not path.is_symlink():
                fail(f"frozen implementation symlink is missing: {path}")
            target = os.readlink(path).encode("utf-8", "surrogateescape")
            if len(target) != expected_bytes or sha256_bytes(target) != expected_sha:
                fail(f"frozen implementation symlink changed: {path}")
        elif item.get("kind") == "file":
            if path.is_symlink() or not path.is_file():
                fail(f"frozen implementation file is missing: {path}")
            if path.stat().st_size != expected_bytes or sha256_file(path) != expected_sha:
                fail(f"frozen implementation file changed: {path}")
        else:
            fail(f"frozen source inventory has an invalid kind for {relative}")
        expected_paths.add(relative)

    actual_paths = {
        path.relative_to(snapshot_root).as_posix()
        for path in snapshot_root.rglob("*")
        if path.is_file() or path.is_symlink()
    }
    if actual_paths != expected_paths:
        fail("frozen implementation source snapshot has an unledgered or missing path")
    return items


def collect_contract_files(repo: Path, specifications: Sequence[str]) -> list[Path]:
    if not specifications:
        fail("at least one --contract path is required")
    files: dict[Path, Path] = {}
    for specification in specifications:
        candidate = (repo / specification).resolve() if not Path(specification).is_absolute() else Path(specification).resolve()
        try:
            relative = candidate.relative_to(repo)
        except ValueError:
            fail(f"contract path is outside the implementation repository: {candidate}")
        if candidate.is_symlink():
            fail(f"contract path may not be a symlink: {candidate}")
        if candidate.is_file():
            files[relative] = candidate
        elif candidate.is_dir():
            for path in candidate.rglob("*"):
                if path.is_symlink():
                    fail(f"contract tree may not contain symlinks: {path}")
                if path.is_file():
                    files[path.relative_to(repo)] = path
        else:
            fail(f"contract path does not exist: {candidate}")
    return [files[key] for key in sorted(files, key=lambda path: path.as_posix())]


def validate_budgets(path: Path) -> dict[str, Any]:
    budgets = read_json(path)
    required = {
        "initial_lexical_symbol_candidates": 40,
        "retained_anchors_before_review": 16,
        "visible_anchors_min": 3,
        "visible_anchors_max": 8,
        "source_document_files_read": 12,
        "retained_source_document_bytes": 131072,
        "gopls_queries": 12,
        "named_frontier_expansions": 2,
        "deterministic_local_seconds": 10,
        "model_calls_hard_max": 4,
    }
    if not isinstance(budgets, dict):
        fail("budget contract must be a JSON object")
    for field, maximum in required.items():
        value = budgets.get(field)
        if not isinstance(value, int) or isinstance(value, bool) or value != maximum:
            fail(f"budget contract {field} must equal the approved v0 value {maximum}")
    calls = budgets.get("model_calls_by_locality")
    if calls != {"bounded_cross_file": 2, "extension_contribution": 3, "local_exact": 1}:
        fail("budget contract has invalid per-locality model-call limits")
    return budgets


def parse_owner_checksums(path: Path) -> dict[str, str]:
    expected_names = {
        "CODEX_TASK_LENS_V0_PROMPT.md", "DEV_SET.json", "HOLDOUT_SET.json", "README.md"
    }
    values: dict[str, str] = {}
    for line_number, line in enumerate(path.read_text(encoding="utf-8").splitlines(), 1):
        match = re.fullmatch(r"([0-9a-f]{64})  ([A-Za-z0-9_.-]+)", line)
        if not match or match.group(2) in values:
            fail(f"invalid owner checksum manifest line {line_number}: {path}")
        values[match.group(2)] = match.group(1)
    if set(values) != expected_names:
        fail("owner checksum manifest does not name the exact supplied four-file bundle")
    return values


def verify_owner_checksums(path: Path) -> dict[str, str]:
    values = parse_owner_checksums(path)
    for name, expected in values.items():
        source = path.parent / name
        if not source.is_file() or sha256_file(source) != expected:
            fail(f"owner-supplied input checksum mismatch: {source}")
    return values


def validate_completed_dev_evaluation(path: Path, episode_ids: Sequence[str]) -> bytes:
    try:
        raw = path.read_bytes()
        text = raw.decode("utf-8")
    except (OSError, UnicodeDecodeError) as exc:
        fail(f"cannot read completed development evaluation {path}: {exc}")
    if len(raw) < 100 or len(raw) > 512 * 1024:
        fail("DEV_EVALUATION.md must be a completed bounded development report before freeze")
    if re.search(r"(?im)^\s*status\s*:\s*(?:not[_ -]?run|pending|incomplete)\s*$", text):
        fail("DEV_EVALUATION.md is still a placeholder; complete development evaluation before freeze")
    missing = [episode_id for episode_id in episode_ids if episode_id not in text]
    if missing:
        fail("DEV_EVALUATION.md does not cover every development episode: " + ", ".join(missing))
    return raw


def validate_live_development_attempt(
    root: Path,
    episode: dict[str, Any],
    attempt: Path,
    provenance: dict[str, Any],
    candidate_binary: Path,
) -> tuple[dict[str, Any], dict[str, Any], dict[str, Any]]:
    candidate_binary_sha = sha256_file(candidate_binary)
    seal = verify_episode_seal(attempt)
    attempt_record = validate_harness_attempt(
        root, "dev", attempt, provenance, candidate_binary_sha
    )
    summary = validate_production_artifacts(
        attempt / "run",
        provenance,
        candidate_binary,
    )
    if attempt_record.get("offline") is not False:
        fail(f"development freeze requires a non-offline latest attempt: {episode['id']}")
    if summary.get("provider", {}).get("calls") != 1:
        fail(f"development freeze requires a live provider call in the latest attempt: {episode['id']}")
    expected_seal = {
        "phase": "dev",
        "episode_id": episode["id"],
        "base_revision": episode["base_revision"],
        "tree_hash": provenance["tree_hash"],
        "task_sha256": provenance["task_sha256"],
        "state": summary["state"],
        "sufficient": summary["sufficient"],
    }
    if any(seal.get(field) != value for field, value in expected_seal.items()):
        fail(f"development seal provenance/result mismatch for {episode['id']}")
    return seal, attempt_record, summary


def freeze(args: argparse.Namespace) -> None:
    root = Path(args.root).expanduser().resolve()
    init_templates(root)
    if (root / "FREEZE_MANIFEST.json").exists() or (root / "freeze").exists():
        fail("freeze already exists; a frozen experiment is immutable")
    implementation_repo = resolve_existing(args.implementation_repo, "dir")
    binary = resolve_existing(args.binary, "file")
    owner_prompt = resolve_existing(args.owner_prompt, "file")
    owner_checksums_path = resolve_existing(args.owner_checksums, "file")
    budgets_path = resolve_existing(args.budgets, "file")
    dev_manifest_path = resolve_existing(args.dev_manifest, "file")
    holdout_manifest_path = resolve_existing(args.holdout_manifest, "file")
    ensure_git_repository(implementation_repo)
    dev_manifest = validate_manifest(dev_manifest_path, "dev")
    validate_manifest(holdout_manifest_path, "holdout")
    validate_budgets(budgets_path)
    owner_checksums = verify_owner_checksums(owner_checksums_path)
    owner_directory = owner_checksums_path.parent
    supplied_paths = {
        "CODEX_TASK_LENS_V0_PROMPT.md": owner_prompt,
        "DEV_SET.json": dev_manifest_path,
        "HOLDOUT_SET.json": holdout_manifest_path,
    }
    for name, supplied in supplied_paths.items():
        if sha256_file(supplied) != owner_checksums[name]:
            fail(f"freeze input is not the exact owner-supplied {name}: {supplied}")
    reject_holdout_gold_env()
    assert_no_holdout_gold(root)

    prepared_record, prepared_dev, dev_root = load_phase(root, "dev")
    if sha256_file(dev_manifest_path) != sha256_file(dev_root / "DEV_SET.json"):
        fail("development manifest differs from the prepared development set")
    if [item["id"] for item in prepared_dev["episodes"]] != [item["id"] for item in dev_manifest["episodes"]]:
        fail("prepared development episodes differ from the freeze manifest")
    dev_source_repo = resolve_existing(prepared_record["source_repository"], "dir")
    dev_seals = []
    for episode in dev_manifest["episodes"]:
        episode_root = dev_root / episode["id"]
        provenance = verify_episode_source(
            root, dev_source_repo, "dev", episode, episode_root, dev_manifest["repository"]
        )
        attempt = latest_dev_attempt(episode_root)
        seal, attempt_record, _summary = validate_live_development_attempt(
            root, episode, attempt, provenance, binary
        )
        dev_seals.append(
            {
                "episode_id": episode["id"],
                "attempt": str(attempt.relative_to(root)),
                "seal_sha256": sha256_file(attempt / "SEALED.json"),
                "binary_sha256": attempt_record["binary_sha256"],
                "task_sha256": provenance["task_sha256"],
                "base_revision": provenance["base_revision"],
                "tree_hash": provenance["tree_hash"],
            }
        )
    dev_evaluation_path = root / "DEV_EVALUATION.md"
    validate_completed_dev_evaluation(
        dev_evaluation_path, [episode["id"] for episode in dev_manifest["episodes"]]
    )
    empty_directory(root / "holdout", "holdout directory before freeze")
    if not os.access(binary, os.X_OK):
        fail(f"freeze binary is not executable: {binary}")
    if SECRET_RE.search(binary.read_bytes() if binary.stat().st_size < 16 * 1024 * 1024 else b""):
        fail("freeze binary contains an obvious credential pattern")

    freeze_root = root / "freeze"
    freeze_root.mkdir()
    copy_new(binary, freeze_root / "repomap", 0o555)
    runner_source = Path(__file__).resolve()
    evaluator_source = runner_source.with_name("task_lens_eval.py")
    if not evaluator_source.is_file():
        fail(f"Task Lens evaluator is missing beside the harness: {evaluator_source}")
    copy_new(runner_source, freeze_root / "harness/task_lens_harness.py", 0o555)
    copy_new(evaluator_source, freeze_root / "harness/task_lens_eval.py", 0o555)
    copy_new(owner_prompt, freeze_root / "OWNER_PROMPT.md", 0o444)
    copy_new(owner_checksums_path, freeze_root / "OWNER_INPUTS.sha256", 0o444)
    copy_new(owner_directory / "README.md", freeze_root / "OWNER_README.md", 0o444)
    copy_new(budgets_path, freeze_root / "BUDGETS.json", 0o444)
    copy_new(dev_manifest_path, freeze_root / "DEV_SET.json", 0o444)
    copy_new(holdout_manifest_path, freeze_root / "HOLDOUT_SET.json", 0o444)
    copy_new(dev_evaluation_path, freeze_root / "DEV_EVALUATION.md", 0o444)

    task_entries = []
    for episode in dev_manifest["episodes"]:
        source = dev_root / episode["id"] / "task.md"
        destination = freeze_root / "tasks/dev" / f"{episode['id']}.md"
        copy_new(source, destination, 0o444)
        task_entries.append(
            {
                "phase": "dev",
                "episode_id": episode["id"],
                "path": destination.relative_to(root).as_posix(),
                "sha256": sha256_file(destination),
                "base_revision": episode["base_revision"],
                "tree_hash": git_output(dev_source_repo, "rev-parse", f"{episode['base_revision']}^{{tree}}"),
            }
        )
    holdout_manifest = validate_manifest(holdout_manifest_path, "holdout")
    for episode in holdout_manifest["episodes"]:
        resolved = git_output(dev_source_repo, "rev-parse", "--verify", f"{episode['base_revision']}^{{commit}}")
        if resolved != episode["base_revision"]:
            fail(f"holdout base revision is unavailable at freeze: {episode['id']}")
        holdout_tree = git_output(dev_source_repo, "rev-parse", f"{episode['base_revision']}^{{tree}}")
        destination = freeze_root / "tasks/holdout" / f"{episode['id']}.md"
        write_new(destination, (episode["task"].rstrip() + "\n").encode("utf-8"), 0o444)
        task_entries.append(
            {
                "phase": "holdout",
                "episode_id": episode["id"],
                "path": destination.relative_to(root).as_posix(),
                "sha256": sha256_file(destination),
                "base_revision": episode["base_revision"],
                "tree_hash": holdout_tree,
            }
        )
    write_json_new(freeze_root / "TASKS.json", {"version": VERSION, "tasks": task_entries}, 0o444)

    diff = run_command(
        ("git", "-C", str(implementation_repo), "diff", "--binary", "--full-index", "HEAD", "--"),
        check=True,
    ).stdout
    if SECRET_RE.search(diff):
        fail("implementation diff contains an obvious credential pattern")
    write_new(freeze_root / "IMPLEMENTATION.diff", diff, 0o444)
    inventory = collect_implementation_inventory(implementation_repo, root)
    write_json_new(freeze_root / "SOURCE_INVENTORY.json", {"version": VERSION, "files": inventory}, 0o444)
    copy_implementation_snapshot(implementation_repo, freeze_root / "implementation", inventory)
    verify_implementation_snapshot(
        root,
        {
            "source_inventory": {"path": "freeze/SOURCE_INVENTORY.json"},
            "source_snapshot": {
                "path": "freeze/implementation",
                "file_count": len(inventory),
            },
        },
    )

    contract_entries = []
    for source in collect_contract_files(implementation_repo, args.contract):
        relative = source.relative_to(implementation_repo)
        # Keep standalone contract snapshots outside Go's ./... package walk.
        # They are exact source bytes for protocol replay, not buildable packages.
        destination = freeze_root / "_contracts" / relative
        copy_new(source, destination, 0o444)
        contract_entries.append(
            {"path": relative.as_posix(), "bytes": source.stat().st_size, "sha256": sha256_file(source)}
        )
    write_json_new(
        freeze_root / "CONTRACTS.json",
        {"version": VERSION, "files": contract_entries},
        0o444,
    )

    head = git_output(implementation_repo, "rev-parse", "HEAD")
    tree = git_output(implementation_repo, "rev-parse", "HEAD^{tree}")
    manifest = {
        "version": VERSION,
        "state": "frozen",
        "frozen_at": utc_now(),
        "answer_quality_comparison": "answer_A_B_not_run",
        "implementation": {
            "repository": str(implementation_repo),
            "head": head,
            "head_tree": tree,
            "diff": {
                "path": "freeze/IMPLEMENTATION.diff",
                "sha256": sha256_file(freeze_root / "IMPLEMENTATION.diff"),
            },
            "source_inventory": {
                "path": "freeze/SOURCE_INVENTORY.json",
                "sha256": sha256_file(freeze_root / "SOURCE_INVENTORY.json"),
            },
            "source_snapshot": {
                "path": "freeze/implementation",
                "file_count": len(inventory),
            },
            "contracts": {
                "path": "freeze/CONTRACTS.json",
                "sha256": sha256_file(freeze_root / "CONTRACTS.json"),
                "file_count": len(contract_entries),
            },
        },
        "binary": {"path": "freeze/repomap", "sha256": sha256_file(freeze_root / "repomap")},
        "harness": {
            "runner": {
                "path": "freeze/harness/task_lens_harness.py",
                "sha256": sha256_file(freeze_root / "harness/task_lens_harness.py"),
            },
            "evaluator": {
                "path": "freeze/harness/task_lens_eval.py",
                "sha256": sha256_file(freeze_root / "harness/task_lens_eval.py"),
            },
        },
        "inputs": {
            "owner_prompt": {
                "path": "freeze/OWNER_PROMPT.md",
                "sha256": sha256_file(freeze_root / "OWNER_PROMPT.md"),
            },
            "owner_checksums": {
                "path": "freeze/OWNER_INPUTS.sha256",
                "sha256": sha256_file(freeze_root / "OWNER_INPUTS.sha256"),
            },
            "owner_readme": {
                "path": "freeze/OWNER_README.md",
                "sha256": sha256_file(freeze_root / "OWNER_README.md"),
            },
            "budgets": {
                "path": "freeze/BUDGETS.json",
                "sha256": sha256_file(freeze_root / "BUDGETS.json"),
            },
            "dev_manifest": {
                "path": "freeze/DEV_SET.json",
                "sha256": sha256_file(freeze_root / "DEV_SET.json"),
            },
            "holdout_manifest": {
                "path": "freeze/HOLDOUT_SET.json",
                "sha256": sha256_file(freeze_root / "HOLDOUT_SET.json"),
            },
            "tasks": {
                "path": "freeze/TASKS.json",
                "sha256": sha256_file(freeze_root / "TASKS.json"),
            },
        },
        "development_seals": dev_seals,
        "development_evaluation": {
            "path": "freeze/DEV_EVALUATION.md",
            "sha256": sha256_file(freeze_root / "DEV_EVALUATION.md"),
        },
        "holdout_gold_present_at_freeze": False,
    }
    manifest_path = root / "FREEZE_MANIFEST.json"
    write_json_new(manifest_path, manifest, 0o444)
    write_new(
        freeze_root / "FREEZE_MANIFEST.sha256",
        f"{sha256_file(manifest_path)}  FREEZE_MANIFEST.json\n".encode("ascii"),
        0o444,
    )
    append_log(root, f"froze Task Lens binary and {len(contract_entries)} prompt/schema/policy contract files")
    refresh_audit(root)


def verify_frozen_phase_sidecars(
    root: Path,
    phase: str,
    manifest: dict[str, Any],
    frozen_manifest_sha: str,
    task_entries: Sequence[dict[str, Any]],
    *,
    required: bool,
) -> dict[str, dict[str, Any]]:
    phase_root = root / phase
    prepared_path = phase_root / "PREPARED.json"
    prepared_sidecar = phase_root / "PREPARED.sha256"
    if not prepared_path.exists() and not prepared_sidecar.exists():
        if required:
            fail(f"frozen {phase} evidence is missing its preparation record")
        return {}
    if not prepared_path.is_file() or not prepared_sidecar.is_file():
        fail(f"{phase} preparation record/sidecar is incomplete")
    parts = prepared_sidecar.read_text(encoding="ascii").split()
    if (
        len(parts) != 2
        or parts[0] != sha256_file(prepared_path)
        or parts[1] != "PREPARED.json"
    ):
        fail(f"{phase} preparation sidecar changed after freeze")
    prepared = require_object(read_json(prepared_path), f"{phase} preparation record")
    if prepared.get("version") != VERSION or prepared.get("phase") != phase:
        fail(f"{phase} preparation record has an invalid header")
    expected_ids = [episode["id"] for episode in manifest["episodes"]]
    if prepared.get("episode_ids") != expected_ids:
        fail(f"{phase} preparation record differs from the frozen episode set")
    local_manifest = phase_root / ("DEV_SET.json" if phase == "dev" else "inputs/HOLDOUT_SET.json")
    if not local_manifest.is_file() or sha256_file(local_manifest) != frozen_manifest_sha:
        fail(f"{phase} prepared manifest differs from the frozen manifest")
    if prepared.get("manifest_sha256") != frozen_manifest_sha:
        fail(f"{phase} preparation record is not bound to the frozen manifest")
    if phase == "holdout":
        if prepared.get("freeze_manifest_sha256") != sha256_file(root / "FREEZE_MANIFEST.json"):
            fail("holdout preparation sidecar is not bound to the current freeze")
        frozen_binary_sha = require_sha(
            read_json(root / "FREEZE_MANIFEST.json").get("binary", {}).get("sha256"),
            "holdout preparation frozen binary SHA",
        )
        if prepared.get("frozen_binary_sha256") != frozen_binary_sha:
            fail("holdout preparation sidecar is not bound to the frozen binary")

    entries = {
        entry["episode_id"]: entry
        for entry in task_entries
        if entry.get("phase") == phase
    }
    if set(entries) != set(expected_ids):
        fail(f"frozen task ledger does not uniquely cover {phase} sidecars")
    provenances: dict[str, dict[str, Any]] = {}
    for episode in manifest["episodes"]:
        episode_id = episode["id"]
        episode_root = phase_root / episode_id
        provenance = load_provenance(episode_root)
        expected_entry = entries[episode_id]
        expected_fields = {
            "version": VERSION,
            "phase": phase,
            "episode_id": episode_id,
            "manifest_repository": manifest["repository"],
            "base_revision": episode["base_revision"],
            "tree_hash": expected_entry["tree_hash"],
            "task_sha256": expected_entry["sha256"],
            "worktree": "worktree/repo",
            "source_only_export": "source/repo",
        }
        if any(provenance.get(field) != value for field, value in expected_fields.items()):
            fail(f"{phase} provenance differs from the frozen task ledger: {episode_id}")
        task_path = episode_root / "task.md"
        if task_path.is_symlink() or not task_path.is_file() or sha256_file(task_path) != expected_entry["sha256"]:
            fail(f"{phase} task sidecar differs from the frozen task ledger: {episode_id}")
        frozen_task_path = root / validate_relative_path(expected_entry["path"], "frozen task path")
        if task_path.read_bytes() != frozen_task_path.read_bytes():
            fail(f"{phase} task bytes differ from their frozen copy: {episode_id}")
        provenances[episode_id] = provenance
    return provenances


def verify_frozen_development_seals(
    root: Path,
    manifest: dict[str, Any],
    freeze: dict[str, Any],
    provenances: dict[str, dict[str, Any]],
) -> None:
    records = freeze.get("development_seals")
    expected_ids = [episode["id"] for episode in manifest["episodes"]]
    if not isinstance(records, list) or not all(isinstance(record, dict) for record in records):
        fail("freeze development seals must be an object list")
    if [record.get("episode_id") for record in records] != expected_ids:
        fail("freeze development seals do not cover the exact frozen development set")
    binary_sha = require_sha(freeze.get("binary", {}).get("sha256"), "frozen binary SHA")
    frozen_binary = resolve_existing(root / freeze["binary"]["path"], "file")
    if sha256_file(frozen_binary) != binary_sha:
        fail("frozen validator binary hash changed")
    for episode, raw in zip(manifest["episodes"], records):
        record = require_object(raw, "freeze development seal entry")
        episode_id = episode["id"]
        relative = validate_relative_path(record.get("attempt", ""), "frozen development attempt path")
        attempt = root / relative
        expected_parent = root / "dev" / episode_id / "attempts"
        if attempt.parent != expected_parent or attempt != latest_dev_attempt(expected_parent.parent):
            fail(f"freeze does not select the latest development attempt: {episode_id}")
        seal, attempt_record, _summary = validate_live_development_attempt(
            root, episode, attempt, provenances[episode_id], frozen_binary
        )
        seal_path = attempt / "SEALED.json"
        expected = {
            "seal_sha256": sha256_file(seal_path),
            "binary_sha256": attempt_record["binary_sha256"],
            "task_sha256": provenances[episode_id]["task_sha256"],
            "base_revision": episode["base_revision"],
            "tree_hash": provenances[episode_id]["tree_hash"],
        }
        if any(record.get(field) != value for field, value in expected.items()):
            fail(f"frozen development seal ledger changed: {episode_id}")
        if seal.get("episode_id") != episode_id:
            fail(f"frozen development seal episode ID changed: {episode_id}")


def verify_rendered_review_episode_links(root: Path, holdout_manifest: dict[str, Any]) -> None:
    if not (root / "evaluation/EVALUATION_RESULT.json").is_file():
        return
    review_path = root / "review/index.html"
    if not review_path.is_file():
        fail("rendered evaluation is missing its review index")
    review = review_path.read_text(encoding="utf-8")
    for episode in holdout_manifest["episodes"]:
        episode_id = episode["id"]
        expected_links = (
            f'../holdout/{episode_id}/task.md',
            f'../holdout/{episode_id}/PROVENANCE.json',
        )
        if any(link not in review for link in expected_links):
            fail(f"rendered review is missing frozen task/provenance links: {episode_id}")


def verify_freeze(root: Path, require_frozen_runner: bool = False) -> dict[str, Any]:
    manifest_path = root / "FREEZE_MANIFEST.json"
    sidecar = root / "freeze/FREEZE_MANIFEST.sha256"
    if not manifest_path.is_file() or not sidecar.is_file():
        fail("Task Lens experiment is not frozen")
    parts = sidecar.read_text(encoding="ascii").split()
    if len(parts) != 2 or parts[0] != sha256_file(manifest_path):
        fail("freeze manifest SHA-256 does not match")
    manifest = read_json(manifest_path)
    if not isinstance(manifest, dict) or manifest.get("state") != "frozen":
        fail("invalid freeze manifest")
    references = [manifest.get("binary"), manifest.get("development_evaluation")]
    implementation = manifest.get("implementation", {})
    references.extend(
        implementation.get(name)
        for name in ("diff", "source_inventory", "contracts")
    )
    references.extend(manifest.get("inputs", {}).values())
    harness = require_object(manifest.get("harness"), "freeze harness")
    references.extend(harness.values())
    for reference in references:
        if not isinstance(reference, dict):
            fail("freeze manifest has an invalid artifact reference")
        path = root / validate_relative_path(reference.get("path", ""), "freeze path")
        expected = reference.get("sha256")
        if not path.is_file() or not isinstance(expected, str) or sha256_file(path) != expected:
            fail(f"frozen artifact changed or is missing: {path}")
    runner = root / validate_relative_path(harness["runner"]["path"], "frozen harness path")
    if require_frozen_runner and Path(__file__).resolve() != runner.resolve():
        fail(f"holdout commands must execute the frozen harness: {runner}")

    owner_values = parse_owner_checksums(root / manifest["inputs"]["owner_checksums"]["path"])
    frozen_owner_files = {
        "CODEX_TASK_LENS_V0_PROMPT.md": root / manifest["inputs"]["owner_prompt"]["path"],
        "DEV_SET.json": root / manifest["inputs"]["dev_manifest"]["path"],
        "HOLDOUT_SET.json": root / manifest["inputs"]["holdout_manifest"]["path"],
        "README.md": root / manifest["inputs"]["owner_readme"]["path"],
    }
    for name, frozen_path in frozen_owner_files.items():
        if sha256_file(frozen_path) != owner_values[name]:
            fail(f"frozen owner input differs from the supplied checksum manifest: {name}")

    verify_implementation_snapshot(root, implementation)

    contracts = read_json(root / implementation["contracts"]["path"])
    contract_entries = require_array(contracts.get("files"), "frozen contract ledger", 1)
    if implementation["contracts"].get("file_count") != len(contract_entries):
        fail("frozen contract ledger count differs from the freeze manifest")
    expected_contract_paths: set[str] = set()
    for entry in contract_entries:
        entry = require_object(entry, "frozen contract entry")
        relative = validate_relative_path(entry.get("path", ""), "frozen contract source path")
        frozen_path = root / "freeze/_contracts" / relative
        if frozen_path.is_symlink() or not frozen_path.is_file():
            fail(f"frozen contract is missing or not regular: {frozen_path}")
        if frozen_path.stat().st_size != entry.get("bytes") or sha256_file(frozen_path) != entry.get("sha256"):
            fail(f"frozen contract changed: {frozen_path}")
        expected_contract_paths.add(relative)
    actual_contract_paths = {
        path.relative_to(root / "freeze/_contracts").as_posix()
        for path in (root / "freeze/_contracts").rglob("*")
        if path.is_file() or path.is_symlink()
    }
    if actual_contract_paths != expected_contract_paths:
        fail("frozen contract tree has an unledgered or missing file")

    task_ledger = read_json(root / manifest["inputs"]["tasks"]["path"])
    task_entries = require_array(task_ledger.get("tasks"), "frozen task ledger", 1)
    expected_task_paths: set[str] = set()
    for entry in task_entries:
        entry = require_object(entry, "frozen task entry")
        if entry.get("phase") not in {"dev", "holdout"} or not EPISODE_ID_RE.fullmatch(entry.get("episode_id", "")):
            fail("frozen task ledger has an invalid phase or episode ID")
        if not REVISION_RE.fullmatch(entry.get("base_revision", "")) or not REVISION_RE.fullmatch(
            entry.get("tree_hash", "")
        ):
            fail("frozen task ledger has an invalid base revision or tree hash")
        task_path = root / validate_relative_path(entry.get("path", ""), "frozen task path")
        if task_path.is_symlink() or not task_path.is_file() or sha256_file(task_path) != entry.get("sha256"):
            fail(f"frozen task changed: {task_path}")
        expected_task_paths.add(task_path.relative_to(root / "freeze/tasks").as_posix())
    actual_task_paths = {
        path.relative_to(root / "freeze/tasks").as_posix()
        for path in (root / "freeze/tasks").rglob("*")
        if path.is_file() or path.is_symlink()
    }
    if actual_task_paths != expected_task_paths:
        fail("frozen task tree has an unledgered or missing file")

    frozen_dev = validate_manifest(root / manifest["inputs"]["dev_manifest"]["path"], "dev")
    frozen_holdout = validate_manifest(root / manifest["inputs"]["holdout_manifest"]["path"], "holdout")
    expected_task_keys = {
        *(('dev', episode['id']) for episode in frozen_dev['episodes']),
        *(('holdout', episode['id']) for episode in frozen_holdout['episodes']),
    }
    actual_task_keys = {(entry["phase"], entry["episode_id"]) for entry in task_entries}
    if actual_task_keys != expected_task_keys:
        fail("frozen task ledger does not cover the exact development and holdout manifests")
    manifest_episodes = {("dev", episode["id"]): episode for episode in frozen_dev["episodes"]}
    manifest_episodes.update(
        {("holdout", episode["id"]): episode for episode in frozen_holdout["episodes"]}
    )
    for entry in task_entries:
        episode = manifest_episodes[(entry["phase"], entry["episode_id"])]
        if entry["base_revision"] != episode["base_revision"]:
            fail(f"frozen task base revision differs from its manifest: {entry['episode_id']}")
    for episode in frozen_holdout["episodes"]:
        entry = next(item for item in task_entries if item["phase"] == "holdout" and item["episode_id"] == episode["id"])
        expected = (episode["task"].rstrip() + "\n").encode("utf-8")
        if (root / entry["path"]).read_bytes() != expected:
            fail(f"frozen holdout task text differs from HOLDOUT_SET: {episode['id']}")
    validate_completed_dev_evaluation(
        root / manifest["development_evaluation"]["path"],
        [episode["id"] for episode in frozen_dev["episodes"]],
    )
    dev_provenances = verify_frozen_phase_sidecars(
        root,
        "dev",
        frozen_dev,
        manifest["inputs"]["dev_manifest"]["sha256"],
        task_entries,
        required=True,
    )
    verify_frozen_development_seals(root, frozen_dev, manifest, dev_provenances)
    verify_frozen_phase_sidecars(
        root,
        "holdout",
        frozen_holdout,
        manifest["inputs"]["holdout_manifest"]["sha256"],
        task_entries,
        required=False,
    )
    verify_rendered_review_episode_links(root, frozen_holdout)
    return manifest


def frozen_task_entry(root: Path, phase: str, episode_id: str) -> dict[str, Any]:
    manifest = verify_freeze(root, require_frozen_runner=(phase == "holdout"))
    ledger = read_json(root / manifest["inputs"]["tasks"]["path"])
    matches = [
        entry
        for entry in ledger.get("tasks", [])
        if entry.get("phase") == phase and entry.get("episode_id") == episode_id
    ]
    if len(matches) != 1:
        fail(f"frozen task ledger has no unique {phase} task for {episode_id}")
    return matches[0]


def maybe_seal_holdout(root: Path) -> bool:
    reject_holdout_gold_env()
    assert_no_holdout_gold(root)
    if (root / "holdout/HOLDOUT_SEAL.json").exists():
        verify_holdout_seal(root)
        return True
    prepared, manifest, phase_root = load_phase(root, "holdout")
    source_repo = resolve_existing(prepared["source_repository"], "dir")
    freeze = verify_freeze(root, require_frozen_runner=True)
    freeze_manifest_sha = sha256_file(root / "FREEZE_MANIFEST.json")
    episodes = []
    for episode in manifest["episodes"]:
        attempt = phase_root / episode["id"] / "attempt"
        if not (attempt / "SEALED.json").is_file():
            return False
        episode_root = phase_root / episode["id"]
        provenance = verify_episode_source(
            root, source_repo, "holdout", episode, episode_root, manifest["repository"]
        )
        seal = seal_attempt(root, "holdout", episode_root, attempt, provenance)
        if seal.get("freeze_manifest_sha256") != freeze_manifest_sha:
            fail(f"holdout episode {episode['id']} was not bound to this freeze")
        task_entry = frozen_task_entry(root, "holdout", episode["id"])
        expected_fields = {
            "phase": "holdout",
            "episode_id": episode["id"],
            "base_revision": episode["base_revision"],
            "tree_hash": provenance["tree_hash"],
            "task_sha256": task_entry["sha256"],
            "frozen_binary_sha256": freeze["binary"]["sha256"],
        }
        if any(seal.get(field) != value for field, value in expected_fields.items()):
            fail(f"holdout episode seal provenance mismatch for {episode['id']}")
        episodes.append(
            {
                "episode_id": episode["id"],
                "base_revision": episode["base_revision"],
                "tree_hash": provenance["tree_hash"],
                "task_sha256": task_entry["sha256"],
                "seal_path": f"holdout/{episode['id']}/attempt/SEALED.json",
                "seal_sha256": sha256_file(attempt / "SEALED.json"),
            }
        )
    global_seal = {
        "version": VERSION,
        "state": "holdout_sealed_gold_locked",
        "freeze_manifest_sha256": freeze_manifest_sha,
        "holdout_manifest_sha256": sha256_file(phase_root / "inputs/HOLDOUT_SET.json"),
        "cheap_exit_expectations_sha256": sha256_file(cheap_exit_expectation_paths(root)[0]),
        "episodes": episodes,
        "answer_quality_comparison": "answer_A_B_not_run",
        "sealed_at": utc_now(),
    }
    seal_path = phase_root / "HOLDOUT_SEAL.json"
    write_json_new(seal_path, global_seal, 0o444)
    write_new(
        phase_root / "HOLDOUT_SEAL.sha256",
        f"{sha256_file(seal_path)}  HOLDOUT_SEAL.json\n".encode("ascii"),
        0o444,
    )
    append_log(root, "sealed every holdout pack/status; historical gold remains locked")
    return True


def verify_holdout_seal(root: Path) -> dict[str, Any]:
    freeze = verify_freeze(root, require_frozen_runner=True)
    seal_path = root / "holdout/HOLDOUT_SEAL.json"
    sidecar = root / "holdout/HOLDOUT_SEAL.sha256"
    if not seal_path.is_file() or not sidecar.is_file():
        fail("holdout is not fully sealed; gold must remain locked")
    parts = sidecar.read_text(encoding="ascii").split()
    if len(parts) != 2 or parts[0] != sha256_file(seal_path):
        fail("holdout seal SHA-256 does not match")
    seal = read_json(seal_path)
    if seal.get("state") != "holdout_sealed_gold_locked":
        fail("invalid holdout seal state")
    if seal.get("freeze_manifest_sha256") != sha256_file(root / "FREEZE_MANIFEST.json"):
        fail("holdout seal is not bound to the current freeze")
    verify_cheap_exit_expectations(root)
    if seal.get("cheap_exit_expectations_sha256") != sha256_file(
        cheap_exit_expectation_paths(root)[0]
    ):
        fail("holdout seal is not bound to the predeclared cheap-exit expectations")
    manifest_path = root / "holdout/inputs/HOLDOUT_SET.json"
    binary_path = root / "holdout/inputs/repomap"
    if sha256_file(manifest_path) != seal.get("holdout_manifest_sha256"):
        fail("sealed holdout manifest changed")
    if sha256_file(manifest_path) != freeze["inputs"]["holdout_manifest"]["sha256"]:
        fail("holdout input manifest is not the frozen manifest")
    if sha256_file(binary_path) != freeze["binary"]["sha256"]:
        fail("holdout input binary is not the frozen binary")
    holdout_manifest = validate_manifest(manifest_path, "holdout")
    sealed_episodes = seal.get("episodes")
    if not isinstance(sealed_episodes, list) or [item.get("episode_id") for item in sealed_episodes] != [
        item["id"] for item in holdout_manifest["episodes"]
    ]:
        fail("global holdout seal does not cover the exact frozen episode set")
    for episode, expected_episode in zip(sealed_episodes, holdout_manifest["episodes"]):
        seal_file = root / validate_relative_path(episode.get("seal_path", ""), "episode seal path")
        if sha256_file(seal_file) != episode.get("seal_sha256"):
            fail(f"holdout episode seal changed: {seal_file}")
        episode_seal = verify_episode_seal(seal_file.parent)
        task_entry = frozen_task_entry(root, "holdout", expected_episode["id"])
        expected = {
            "phase": "holdout",
            "episode_id": expected_episode["id"],
            "base_revision": expected_episode["base_revision"],
            "tree_hash": task_entry["tree_hash"],
            "task_sha256": task_entry["sha256"],
            "freeze_manifest_sha256": seal["freeze_manifest_sha256"],
            "frozen_binary_sha256": freeze["binary"]["sha256"],
        }
        if any(episode_seal.get(field) != value for field, value in expected.items()):
            fail(f"holdout episode seal differs from frozen provenance: {expected_episode['id']}")
        if episode.get("tree_hash") != task_entry["tree_hash"] or episode.get("task_sha256") != task_entry["sha256"]:
            fail(f"global holdout seal differs from the frozen task ledger: {expected_episode['id']}")
    return seal


def cheap_exit_expectation_paths(root: Path) -> tuple[Path, Path]:
    return (
        root / CHEAP_EXIT_EXPECTATIONS,
        root / CHEAP_EXIT_EXPECTATIONS_SHA256,
    )


def declare_cheap_exits(args: argparse.Namespace) -> None:
    """Seal expected cheap exits before holdout execution or historical gold access."""
    root = Path(args.root).expanduser().resolve()
    freeze = verify_freeze(root, require_frozen_runner=True)
    reject_holdout_gold_env()
    assert_no_holdout_gold(root)
    if any((root / "holdout").iterdir()):
        fail("cheap-exit expectations must be declared before holdout preparation")
    declaration_path, sidecar_path = cheap_exit_expectation_paths(root)
    if declaration_path.exists() or sidecar_path.exists():
        fail("cheap-exit expectations have already been declared and are immutable")

    requested = args.episode or []
    if not requested:
        fail("at least one --episode must predeclare an expected cheap exit")
    if len(requested) != len(set(requested)):
        fail("cheap-exit expectation contains a duplicate episode ID")
    holdout_manifest_path = root / freeze["inputs"]["holdout_manifest"]["path"]
    holdout_manifest = validate_manifest(holdout_manifest_path, "holdout")
    expected_ids = [episode["id"] for episode in holdout_manifest["episodes"]]
    unknown = sorted(set(requested) - set(expected_ids))
    if unknown:
        fail(f"cheap-exit expectation names an unknown holdout episode: {unknown[0]}")

    declared = set(requested)
    declaration = {
        "version": VERSION,
        "state": "cheap_exit_predeclared_holdout_unstarted",
        "declared_at": utc_now(),
        "freeze_manifest_sha256": sha256_file(root / "FREEZE_MANIFEST.json"),
        "holdout_manifest_sha256": sha256_file(holdout_manifest_path),
        "answer_quality_comparison": "answer_A_B_not_run",
        "episodes": [
            {
                "episode_id": episode_id,
                "cheap_exit_expected": episode_id in declared,
            }
            for episode_id in expected_ids
        ],
    }
    write_json_new(declaration_path, declaration, 0o444)
    write_new(
        sidecar_path,
        f"{sha256_file(declaration_path)}  {CHEAP_EXIT_EXPECTATIONS}\n".encode("ascii"),
        0o444,
    )
    append_log(root, "predeclared expected cheap-exit episodes before holdout preparation")
    refresh_audit(root)


def verify_cheap_exit_expectations(root: Path) -> dict[str, Any]:
    freeze = verify_freeze(root, require_frozen_runner=True)
    declaration_path, sidecar_path = cheap_exit_expectation_paths(root)
    if not declaration_path.is_file() or not sidecar_path.is_file():
        fail("cheap-exit expectations must be predeclared before holdout preparation or gold unlock")
    parts = sidecar_path.read_text(encoding="ascii").split()
    if (
        len(parts) != 2
        or parts[0] != sha256_file(declaration_path)
        or parts[1] != CHEAP_EXIT_EXPECTATIONS
    ):
        fail("cheap-exit expectation SHA-256 does not match")
    declaration = read_json(declaration_path)
    if (
        not isinstance(declaration, dict)
        or declaration.get("version") != VERSION
        or declaration.get("state") != "cheap_exit_predeclared_holdout_unstarted"
        or declaration.get("answer_quality_comparison") != "answer_A_B_not_run"
    ):
        fail("invalid cheap-exit expectation declaration")
    if declaration.get("freeze_manifest_sha256") != sha256_file(root / "FREEZE_MANIFEST.json"):
        fail("cheap-exit expectations are not bound to the current freeze")
    holdout_manifest_path = root / freeze["inputs"]["holdout_manifest"]["path"]
    if declaration.get("holdout_manifest_sha256") != sha256_file(holdout_manifest_path):
        fail("cheap-exit expectations are not bound to the frozen holdout manifest")
    rows = declaration.get("episodes")
    holdout_manifest = validate_manifest(holdout_manifest_path, "holdout")
    expected_ids = [episode["id"] for episode in holdout_manifest["episodes"]]
    if not isinstance(rows, list) or [row.get("episode_id") for row in rows if isinstance(row, dict)] != expected_ids:
        fail("cheap-exit expectations do not cover the exact frozen holdout in order")
    if not any(row.get("cheap_exit_expected") is True for row in rows):
        fail("cheap-exit expectations must predeclare at least one episode")
    for row in rows:
        if set(row) != {"episode_id", "cheap_exit_expected"} or not isinstance(
            row.get("cheap_exit_expected"), bool
        ):
            fail(f"invalid cheap-exit expectation for episode {row.get('episode_id')}")
    return declaration


def gold_inventory(path: Path) -> list[dict[str, Any]]:
    files = []
    for item in sorted(path.rglob("*")):
        if item.is_symlink():
            fail(f"gold package may not contain symlinks: {item}")
        if item.is_file():
            files.append(
                {
                    "path": item.relative_to(path).as_posix(),
                    "kind": "file",
                    "bytes": item.stat().st_size,
                    "sha256": sha256_file(item),
                }
            )
    if not files:
        fail(f"gold package contains no files: {path}")
    return files


def unlock_gold(args: argparse.Namespace) -> None:
    root = Path(args.root).expanduser().resolve()
    seal = verify_holdout_seal(root)
    cheap_exit_declaration = verify_cheap_exit_expectations(root)
    cheap_exit_by_episode = {
        row["episode_id"]: row["cheap_exit_expected"]
        for row in cheap_exit_declaration["episodes"]
    }
    gold = resolve_existing(args.gold_dir, "dir")
    assert_no_holdout_gold(root, allowed_gold=gold)
    try:
        gold.relative_to(root / "holdout")
        fail("gold package must remain outside the sealed holdout tree")
    except ValueError:
        pass
    evaluation = root / "evaluation"
    if evaluation.exists():
        fail(f"evaluation directory already exists: {evaluation}")
    inventory = gold_inventory(gold)
    evaluation.mkdir()
    copied = evaluation / "gold"
    shutil.copytree(gold, copied)
    verify_file_inventory(copied, inventory, set())
    for item in inventory:
        os.chmod(copied / item["path"], 0o444)
    write_json_new(
        evaluation / "GOLD_UNLOCK.json",
        {
            "version": VERSION,
            "unlocked_at": utc_now(),
            "holdout_seal_sha256": sha256_file(root / "holdout/HOLDOUT_SEAL.json"),
            "cheap_exit_expectations_sha256": sha256_file(
                cheap_exit_expectation_paths(root)[0]
            ),
            "source_path": str(gold),
            "files": inventory,
        },
        0o444,
    )
    scorecard = {
        "version": VERSION,
        "outcome": None,
        "technical_result": None,
        "product_result": None,
        "investment_result": None,
        "single_strongest_demonstrated_value": None,
        "single_main_blocker": None,
        "recommended_next_step": None,
        "previous_benchmark_defects": {
            defect: {
                "reproduced": None,
                "fixed": None,
                "regression_test": None,
                "residual_risk": None,
            }
            for defect in BENCHMARK_DEFECTS
        },
        "task_lens_contract": {field: None for field in CONTRACT_SECTIONS},
        "development": {
            **{field: None for field in DEVELOPMENT_SECTIONS},
            "no_episode_specific_production_rules": None,
        },
        "product_comparison": {field: None for field in PRODUCT_COMPARISON_SECTIONS},
        "product_findings": {field: None for field in PRODUCT_FINDING_QUESTIONS},
        "verification": {"focused_checks": [], "broad_checks_skipped": []},
        "final_recommendation": None,
        "answer_quality_comparison": "answer_A_B_not_run",
        "episodes": [
            {
                "id": episode["episode_id"],
                "scores": {dimension: None for dimension in SCORE_DIMENSIONS},
                "cheap_exit_expected": cheap_exit_by_episode[episode["episode_id"]],
                "gold_comparison": None,
                "generic_report_comparison": None,
                "unsupported_claims": [],
                "major_unsupported_claims": [],
                "irrelevant_anchors": [],
                "important_missing_anchors": [],
                "failure_notes": [],
            }
            for episode in seal["episodes"]
        ],
    }
    write_json_new(evaluation / "SCORECARD.json", scorecard)
    append_log(root, "unlocked and copied historical gold only after the global holdout seal")
    refresh_audit(root)


def completed_scorecard(root: Path, scores_path: Path) -> tuple[dict[str, Any], dict[str, Any]]:
    seal = verify_holdout_seal(root)
    cheap_exit_declaration = verify_cheap_exit_expectations(root)
    cheap_exit_by_episode = {
        row["episode_id"]: row["cheap_exit_expected"]
        for row in cheap_exit_declaration["episodes"]
    }
    unlock = read_json(root / "evaluation/GOLD_UNLOCK.json")
    if unlock.get("holdout_seal_sha256") != sha256_file(root / "holdout/HOLDOUT_SEAL.json"):
        fail("gold unlock is not bound to the current holdout seal")
    if unlock.get("cheap_exit_expectations_sha256") != sha256_file(
        cheap_exit_expectation_paths(root)[0]
    ):
        fail("gold unlock is not bound to the predeclared cheap-exit expectations")
    verify_file_inventory(root / "evaluation/gold", unlock.get("files"), set())
    scorecard = read_json(scores_path)
    if scorecard.get("version") != VERSION:
        fail("unsupported scorecard version")
    if scorecard.get("outcome") not in ("passed", "partial", "failed"):
        fail("scorecard outcome must be passed, partial, or failed")
    text_fields = (
        "technical_result",
        "product_result",
        "investment_result",
        "single_strongest_demonstrated_value",
        "single_main_blocker",
        "recommended_next_step",
    )
    for field in text_fields:
        value = scorecard.get(field)
        if not isinstance(value, str) or not value.strip() or "\n" in value:
            fail(f"scorecard {field} must be one non-empty line")
    if scorecard.get("answer_quality_comparison") != "answer_A_B_not_run":
        fail("scorecard must report answer_A_B_not_run without independently sealed sessions")
    defects = require_object(scorecard.get("previous_benchmark_defects"), "scorecard.previous_benchmark_defects")
    if set(defects) != set(BENCHMARK_DEFECTS):
        fail("scorecard previous benchmark defect audit is incomplete")
    for name, raw in defects.items():
        defect = require_object(raw, f"scorecard defect {name}")
        if set(defect) != {"reproduced", "fixed", "regression_test", "residual_risk"}:
            fail(f"scorecard defect {name} has an invalid shape")
        if not isinstance(defect["reproduced"], bool) or not isinstance(defect["fixed"], bool):
            fail(f"scorecard defect {name} reproduced/fixed must be boolean")
        require_text(defect["regression_test"], f"scorecard defect {name}.regression_test", 2048)
        require_text(defect["residual_risk"], f"scorecard defect {name}.residual_risk", 2048)
    validate_scorecard_text_object(scorecard, "task_lens_contract", CONTRACT_SECTIONS)
    development = validate_scorecard_text_object(scorecard, "development", DEVELOPMENT_SECTIONS, allow_extra=True)
    if set(development) != set(DEVELOPMENT_SECTIONS) | {"no_episode_specific_production_rules"}:
        fail("scorecard.development has an invalid shape")
    if development.get("no_episode_specific_production_rules") is not True:
        fail("scorecard must explicitly confirm no episode-specific production rules")
    validate_scorecard_text_object(scorecard, "product_comparison", PRODUCT_COMPARISON_SECTIONS)
    validate_scorecard_text_object(scorecard, "product_findings", PRODUCT_FINDING_QUESTIONS)
    verification = require_object(scorecard.get("verification"), "scorecard.verification")
    if set(verification) != {"focused_checks", "broad_checks_skipped"}:
        fail("scorecard verification section has an invalid shape")
    for field in ("focused_checks", "broad_checks_skipped"):
        values = require_array(verification.get(field), f"scorecard.verification.{field}", 1)
        if not all(isinstance(item, str) and item.strip() for item in values):
            fail(f"scorecard.verification.{field} must contain non-empty strings")
    if scorecard.get("final_recommendation") not in FINAL_RECOMMENDATIONS:
        fail("scorecard final recommendation is not one of the three approved outcomes")
    if scorecard.get("recommended_next_step") != scorecard.get("final_recommendation"):
        fail("scorecard recommended_next_step must exactly equal final_recommendation")
    rows = scorecard.get("episodes")
    if not isinstance(rows, list):
        fail("scorecard episodes must be an array")
    expected_ids = [episode["episode_id"] for episode in seal["episodes"]]
    if [row.get("id") for row in rows if isinstance(row, dict)] != expected_ids:
        fail("scorecard episode IDs/order do not match the sealed holdout")
    for row in rows:
        scores = row.get("scores")
        if not isinstance(scores, dict) or set(scores) != set(SCORE_DIMENSIONS):
            fail(f"episode {row.get('id')}: score dimensions do not match the v0 contract")
        for dimension, value in scores.items():
            if not isinstance(value, int) or isinstance(value, bool) or value < 0 or value > 4:
                fail(f"episode {row.get('id')}: {dimension} must be an integer from 0 to 4")
        if row.get("cheap_exit_expected") != cheap_exit_by_episode[row["id"]]:
            fail(
                f"episode {row.get('id')}: cheap_exit_expected differs from the sealed pre-gold declaration"
            )
        require_text(row.get("gold_comparison"), f"episode {row.get('id')}.gold_comparison", 4096)
        require_text(
            row.get("generic_report_comparison"),
            f"episode {row.get('id')}.generic_report_comparison",
            4096,
        )
        for field in (
            "unsupported_claims",
            "major_unsupported_claims",
            "irrelevant_anchors",
            "important_missing_anchors",
            "failure_notes",
        ):
            values = row.get(field)
            if not isinstance(values, list) or not all(isinstance(item, str) for item in values):
                fail(f"episode {row.get('id')}: {field} must be an array of strings")
    return scorecard, seal


def validate_scorecard_text_object(
    scorecard: dict[str, Any],
    field: str,
    expected_fields: Sequence[str],
    allow_extra: bool = False,
) -> dict[str, Any]:
    value = require_object(scorecard.get(field), f"scorecard.{field}")
    expected = set(expected_fields)
    if (not allow_extra and set(value) != expected) or not expected.issubset(value):
        fail(f"scorecard.{field} has an invalid shape")
    for name in expected_fields:
        require_text(value.get(name), f"scorecard.{field}.{name}", 4096)
    return value


def holdout_metrics(root: Path, episode_id: str) -> dict[str, Any]:
    return read_json(root / "holdout" / episode_id / "attempt/METRICS.json")


def threshold_summary(root: Path, scorecard: dict[str, Any]) -> dict[str, Any]:
    rows = scorecard["episodes"]
    localization = sum(row["scores"]["subsystem_localization"] >= 3 for row in rows)
    files = sum(row["scores"]["must_read_file_recall"] >= 3 for row in rows)
    joins = sum(row["scores"]["causal_evidence_join_quality"] >= 3 for row in rows)
    verification = sum(row["scores"]["verification_usefulness"] >= 3 for row in rows)
    major_unsupported = sum(len(row["major_unsupported_claims"]) for row in rows)
    cheap_rows = [row for row in rows if row["cheap_exit_expected"]]
    cheap_over_budget = []
    for row in cheap_rows:
        provider = holdout_metrics(root, row["id"]).get("provider", {})
        calls = provider.get("calls") if isinstance(provider, dict) else None
        if not isinstance(calls, int) or calls > 1:
            cheap_over_budget.append(row["id"])
    return {
        "subsystem_localization_at_least_3": {"count": localization, "target": 4, "passed": localization >= 4},
        "must_read_file_recall_at_least_3": {"count": files, "target": 4, "passed": files >= 4},
        "causal_evidence_join_quality_at_least_3": {"count": joins, "target": 3, "passed": joins >= 3},
        "verification_usefulness_at_least_3": {"count": verification, "target": 4, "passed": verification >= 4},
        "major_unsupported_claims": {"count": major_unsupported, "target": 0, "passed": major_unsupported == 0},
        "cheap_exit": {
            "episodes": len(cheap_rows),
            "target_episodes": 3,
            "over_one_model_call": cheap_over_budget,
            "passed": len(cheap_rows) >= 3 and not cheap_over_budget,
        },
        "captured_revision": {"passed": True},
        "neutral_basename": {"passed": True},
    }


def holdout_target_passed(thresholds: dict[str, Any]) -> bool:
    return all(result.get("passed") is True for result in thresholds.values())


def validate_evaluation_outcome(outcome: str, thresholds: dict[str, Any]) -> bool:
    target_passed = holdout_target_passed(thresholds)
    if outcome == "passed" and not target_passed:
        failed = ", ".join(
            name for name, result in thresholds.items() if result.get("passed") is not True
        )
        fail(
            "scorecard outcome cannot be passed unless every frozen holdout target, "
            f"including the zero-major-unsupported-claim gate, is met; not met: {failed}"
        )
    return target_passed


def render_evaluation(args: argparse.Namespace) -> None:
    root = Path(args.root).expanduser().resolve()
    scores_path = resolve_existing(args.scores, "file")
    scorecard, seal = completed_scorecard(root, scores_path)
    thresholds = threshold_summary(root, scorecard)
    target_passed = validate_evaluation_outcome(scorecard["outcome"], thresholds)
    evaluation_result = {
        "version": VERSION,
        "rendered_at": utc_now(),
        "holdout_seal_sha256": sha256_file(root / "holdout/HOLDOUT_SEAL.json"),
        "scorecard_sha256": sha256_file(scores_path),
        "dimensions": list(SCORE_DIMENSIONS),
        "opaque_average_computed": False,
        "holdout_target_passed": target_passed,
        "thresholds": thresholds,
    }
    result_path = root / "evaluation/EVALUATION_RESULT.json"
    if result_path.exists():
        fail("evaluation has already been rendered; frozen scores are immutable")
    write_json_new(result_path, evaluation_result, 0o444)
    os.chmod(scores_path, 0o444)
    render_markdown_outputs(root, scorecard, seal, thresholds)
    render_review(root, scorecard, seal, thresholds)
    append_log(root, "rendered separate 0–4 holdout dimensions; no opaque average was computed")
    refresh_audit(root)


def render_markdown_outputs(
    root: Path,
    scorecard: dict[str, Any],
    seal: dict[str, Any],
    thresholds: dict[str, Any],
) -> None:
    score_header = "| Episode | " + " | ".join(label.replace("_", " ") for label in SCORE_DIMENSIONS) + " |"
    score_rule = "|---|" + "---:|" * len(SCORE_DIMENSIONS)
    score_rows = [
        f"| {row['id']} | " + " | ".join(str(row["scores"][dimension]) for dimension in SCORE_DIMENSIONS) + " |"
        for row in scorecard["episodes"]
    ]
    lines = [
        "# Holdout evaluation",
        "",
        f"Outcome: {scorecard['outcome']}",
        "",
        "Scores are reported separately on the approved 0–4 dimensions. No average is computed.",
        "",
        score_header,
        score_rule,
        *score_rows,
    ]
    for row in scorecard["episodes"]:
        lines.extend(
            [
                "",
                f"## {row['id']}",
                "",
                f"Gold comparison: {row['gold_comparison']}",
                "",
                f"Generic-report comparison: {row['generic_report_comparison']}",
                "",
                "Unsupported claims: " + list_or_none(row["unsupported_claims"]),
                "",
                "Major unsupported claims: " + list_or_none(row["major_unsupported_claims"]),
                "",
                "Irrelevant anchors: " + list_or_none(row["irrelevant_anchors"]),
                "",
                "Important missing anchors: " + list_or_none(row["important_missing_anchors"]),
                "",
                "Failure notes: " + list_or_none(row["failure_notes"]),
            ]
        )
    lines.extend(["", "## Holdout targets", ""])
    for name, result in thresholds.items():
        lines.append(f"- {name.replace('_', ' ')}: {'passed' if result['passed'] else 'not met'}")
    lines.extend(["", "Answer quality comparison: `answer_A_B_not_run`.", ""])
    replace_text(root / "HOLDOUT_EVALUATION.md", "\n".join(lines))

    findings_lines = ["# Product findings", "", f"Outcome: {scorecard['outcome']}", ""]
    for index, field in enumerate(PRODUCT_FINDING_QUESTIONS, 1):
        findings_lines.extend([f"{index}. **{field.replace('_', ' ').capitalize()}**", "", scorecard["product_findings"][field], ""])
    findings_lines.extend(
        [
            f"Strongest demonstrated value: {scorecard['single_strongest_demonstrated_value']}",
            "",
            f"Main blocker: {scorecard['single_main_blocker']}",
        ]
    )
    findings = "\n".join(findings_lines)
    replace_text(root / "PRODUCT_FINDINGS.md", findings)

    dev_rows = development_rows(root)
    development_lines = [
        "# Development evaluation",
        "",
        f"Changes during development: {scorecard['development']['changes_during_development']}",
        "",
        f"Generic versus fixture-only: {scorecard['development']['generic_vs_fixture_only']}",
        "",
        f"Failures shaping the contract: {scorecard['development']['failures_shaping_contract']}",
        "",
        "Episode-specific production rules: none (supervisor-confirmed).",
        "",
        "| Episode | attempts | selected state | locality | model calls | wall ms |",
        "|---|---:|---|---|---:|---:|",
    ]
    development_lines.extend(
        f"| {row['id']} | {row['attempts']} | {row['state']} | {row['locality']} | {row['calls']} | {row['wall_millis']} |"
        for row in dev_rows
    )
    replace_text(root / "DEV_EVALUATION.md", "\n".join(development_lines))

    report_path = root / "SUPERVISOR_REPORT.md"
    report_lines = [
        f"outcome: {scorecard['outcome']}",
        f"technical result: {scorecard['technical_result']}",
        f"product result: {scorecard['product_result']}",
        f"investment result: {scorecard['investment_result']}",
        f"development episodes used: {len(dev_rows)}",
        f"holdout episodes evaluated: {len(seal['episodes'])}",
        f"single strongest demonstrated value: {scorecard['single_strongest_demonstrated_value']}",
        f"single main blocker: {scorecard['single_main_blocker']}",
        f"recommended next step: {scorecard['recommended_next_step']}",
        "",
        "## Previous benchmark defects",
        "",
    ]
    for defect in BENCHMARK_DEFECTS:
        value = scorecard["previous_benchmark_defects"][defect]
        report_lines.extend(
            [
                f"### {defect.replace('_', ' ').capitalize()}",
                "",
                f"- Reproduced: {'yes' if value['reproduced'] else 'no'}",
                f"- Fixed: {'yes' if value['fixed'] else 'no'}",
                f"- Regression test: {value['regression_test']}",
                f"- Residual risk: {value['residual_risk']}",
                "",
            ]
        )
    report_lines.extend(["## Task Lens contract", ""])
    report_lines.extend(
        f"- {field.replace('_', ' ').capitalize()}: {scorecard['task_lens_contract'][field]}"
        for field in CONTRACT_SECTIONS
    )
    report_lines.extend(
        [
            "",
            "## Cheap exit",
            "",
            "| Episode | locality | stages skipped | model calls | local actions | wall ms | sufficient |",
            "|---|---|---|---:|---|---:|---|",
        ]
    )
    for episode in seal["episodes"]:
        metrics = holdout_metrics(root, episode["episode_id"])
        local = metrics.get("local_retrieval", {})
        actions = (
            f"grep={local.get('git_grep_queries', 0)}, AST={local.get('ast_parses', 0)}, "
            f"gopls={metrics.get('budgets', {}).get('gopls_queries', 0)}, "
            f"frontiers={metrics.get('budgets', {}).get('frontier_expansions', 0)}"
        )
        report_lines.append(
            f"| {episode['episode_id']} | {metrics.get('locality')} | "
            f"{', '.join(metrics.get('stages_skipped', []))} | {metrics.get('provider', {}).get('calls')} | "
            f"{actions} | {metrics.get('wall_millis')} | {str(read_json(root / episode['seal_path']).get('sufficient')).lower()} |"
        )
    report_lines.extend(
        [
            "",
            "## Development set",
            "",
            f"- What changed: {scorecard['development']['changes_during_development']}",
            f"- Generic versus fixture-only: {scorecard['development']['generic_vs_fixture_only']}",
            f"- Failures that shaped the contract: {scorecard['development']['failures_shaping_contract']}",
            "- Episode-specific production rules: none (supervisor-confirmed).",
            "",
            "## Holdout results",
            "",
            score_header,
            score_rule,
            *score_rows,
            "",
            "No average was computed.",
            "",
            "## Product comparison",
            "",
        ]
    )
    report_lines.extend(
        f"- {field.replace('_', ' ').capitalize()}: {scorecard['product_comparison'][field]}"
        for field in PRODUCT_COMPARISON_SECTIONS
    )
    report_lines.extend(
        [
            "- Independent answer-quality A/B: answer_A_B_not_run.",
            "",
            "## Model/resource accounting",
            "",
            "| Episode | calls | input | output | cache hit | cache miss | request B | response B | provider ms | wall ms | gopls | grep | AST | bound budgets |",
            "|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---|",
        ]
    )
    for episode in seal["episodes"]:
        metrics = holdout_metrics(root, episode["episode_id"])
        provider = metrics.get("provider", {})
        budget = metrics.get("budgets", {})
        local = metrics.get("local_retrieval", {})
        bound = ", ".join(key for key, value in budget.items() if key.endswith("_bound") and value) or "none"
        report_lines.append(
            f"| {episode['episode_id']} | {provider.get('calls')} | {provider.get('input_tokens')} | "
            f"{provider.get('output_tokens')} | {provider.get('prompt_cache_hit_tokens')} | "
            f"{provider.get('prompt_cache_miss_tokens')} | {provider.get('request_bytes')} | "
            f"{provider.get('response_bytes')} | {provider.get('latency_millis')} | {metrics.get('wall_millis')} | "
            f"{budget.get('gopls_queries')} | {local.get('git_grep_queries')} | {local.get('ast_parses')} | {bound} |"
        )
    report_lines.extend(["", "## Product findings", ""])
    for index, field in enumerate(PRODUCT_FINDING_QUESTIONS, 1):
        report_lines.append(f"{index}. {scorecard['product_findings'][field]}")
    report_lines.extend(["", "## Verification", "", "Focused checks:", ""])
    report_lines.extend(f"- {item}" for item in scorecard["verification"]["focused_checks"])
    report_lines.extend(["", "Broad/slow checks skipped:", ""])
    report_lines.extend(f"- {item}" for item in scorecard["verification"]["broad_checks_skipped"])
    report_lines.extend(
        [
            "",
            "## Final recommendation",
            "",
            scorecard["final_recommendation"],
            "",
            "answer quality comparison: answer_A_B_not_run",
            "",
            "Supervisor report:",
            str(report_path),
            "",
            "User review:",
            review_server_command(root),
            review_url(),
        ]
    )
    report = "\n".join(report_lines)
    replace_text(report_path, report)

    walkthrough = [
        "# Task Lens v0 walkthrough",
        "",
        "1. Read the [harness audit](HARNESS_AUDIT.md) and [freeze manifest](FREEZE_MANIFEST.json).",
        "2. Review development outcomes in [DEV_EVALUATION.md](DEV_EVALUATION.md).",
        "3. Review separate holdout scores in [HOLDOUT_EVALUATION.md](HOLDOUT_EVALUATION.md).",
        "4. Open the static [episode comparison](review/index.html).",
        "5. Read [product findings](PRODUCT_FINDINGS.md) and the [supervisor report](SUPERVISOR_REPORT.md).",
        "",
        "Per holdout episode the review links the exact task, provenance, Task Lens report/artifacts, metrics, seal, and post-seal gold comparison.",
        "",
        "Independent answer A/B: `answer_A_B_not_run`.",
    ]
    replace_text(root / "WALKTHROUGH.md", "\n".join(walkthrough))


def list_or_none(values: Sequence[str]) -> str:
    return "; ".join(values) if values else "none recorded"


def development_rows(root: Path) -> list[dict[str, Any]]:
    manifest = validate_manifest(root / "dev/DEV_SET.json", "dev")
    rows = []
    for episode in manifest["episodes"]:
        episode_root = root / "dev" / episode["id"]
        attempts = sorted(path for path in (episode_root / "attempts").iterdir() if path.is_dir())
        selected = attempts[-1]
        seal = verify_episode_seal(selected)
        metrics = read_json(selected / "METRICS.json")
        rows.append(
            {
                "id": episode["id"],
                "attempts": len(attempts),
                "state": seal.get("state"),
                "locality": metrics.get("locality"),
                "calls": metrics.get("provider", {}).get("calls"),
                "wall_millis": metrics.get("wall_millis"),
            }
        )
    return rows


def replace_text(path: Path, text: str) -> None:
    temporary = path.with_name(path.name + ".tmp")
    with temporary.open("w", encoding="utf-8") as handle:
        handle.write(text.rstrip() + "\n")
    os.replace(temporary, path)


def review_url() -> str:
    return "http://127.0.0.1:8767/review/"


def review_server_command(root: Path) -> str:
    return f"python3 -m http.server 8767 --bind 127.0.0.1 --directory {root}"


def initial_review_html(root: Path) -> str:
    return f"""<!doctype html>
<html lang=\"en\"><head><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width,initial-scale=1\">
<title>Task Lens v0 review</title><style>
body{{font:16px/1.5 system-ui,sans-serif;max-width:960px;margin:3rem auto;padding:0 1rem;color:#17202a}}
code{{background:#eef2f4;padding:.15rem .35rem;border-radius:.25rem}} a{{color:#075985;overflow-wrap:anywhere}} .muted{{color:#59636e}}
</style></head><body><h1>Task Lens v0 review</h1>
<p>Status: harness initialized; product evaluation has not run.</p>
<p><a href=\"../SUPERVISOR_REPORT.md\">Supervisor report</a> · <a href=\"../HARNESS_AUDIT.md\">Harness audit</a></p>
<p class=\"muted\">Serve with <code>{html.escape(review_server_command(root))}</code>.</p></body></html>\n"""


def render_review(
    root: Path,
    scorecard: dict[str, Any],
    seal: dict[str, Any],
    thresholds: dict[str, Any],
) -> None:
    cards = []
    rows = {row["id"]: row for row in scorecard["episodes"]}
    gold_files = read_json(root / "evaluation/GOLD_UNLOCK.json").get("files", [])
    for episode in seal["episodes"]:
        episode_id = episode["episode_id"]
        row = rows[episode_id]
        episode_root = root / "holdout" / episode_id
        attempt_root = episode_root / "attempt"
        paths = artifact_paths(attempt_root / "run")
        pack = read_json(paths["task_investigation.json"])
        metrics = holdout_metrics(root, episode_id)
        report_matches = sorted(path for path in (attempt_root / "run").rglob("report.html") if path.is_file())
        report_link = "../" + report_matches[0].relative_to(root).as_posix()
        task_text = (episode_root / "task.md").read_text(encoding="utf-8").strip()
        scores = "".join(
            f"<li>{html.escape(dimension.replace('_', ' '))}: <strong>{row['scores'][dimension]}</strong></li>"
            for dimension in SCORE_DIMENSIONS
        )
        anchors = []
        for anchor in pack.get("investigation_anchors", []):
            excerpt = "\n".join(f"{line['line']:>5}  {line['text']}" for line in anchor.get("excerpt", []))
            anchors.append(
                "<article class=\"anchor\"><h4>"
                + html.escape(f"{anchor.get('path')} — {anchor.get('symbol')}")
                + "</h4><p><strong>Role:</strong> "
                + html.escape(str(anchor.get("role")))
                + "</p><p>"
                + html.escape(str(anchor.get("why")))
                + "</p><pre>"
                + html.escape(excerpt)
                + "</pre></article>"
            )
        hypothesis = "".join(
            f"<li><strong>{html.escape(str(item.get('status')))}</strong>: {html.escape(str(item.get('text')))}</li>"
            for item in pack.get("working_hypothesis", [])
        )
        reproduce = "".join(
            f"<li><strong>{html.escape(str(item.get('authority')))}</strong>: {html.escape(str(item.get('text')))}</li>"
            for item in pack.get("reproduce_or_observe", [])
        )
        verify = "".join(
            f"<li><strong>{html.escape(str(item.get('authority')))}</strong>: {html.escape(str(item.get('text')))}</li>"
            for item in pack.get("verify", {}).get("steps", [])
        )
        matching_gold = [item["path"] for item in gold_files if episode_id in item.get("path", "")]
        gold_links = "".join(
            f"<li><a href=\"../evaluation/gold/{html.escape(path, quote=True)}\">{html.escape(path)}</a></li>"
            for path in matching_gold
        ) or "<li>No episode-labelled gold filename; use the complete gold directory link above.</li>"
        failures = "".join(f"<li>{html.escape(item)}</li>" for item in row["failure_notes"]) or "<li>None recorded.</li>"
        provider = metrics.get("provider", {})
        local = metrics.get("local_retrieval", {})
        cards.append(
            f"<section><h2>{html.escape(episode_id)}</h2>"
            f"<p><a href=\"../holdout/{episode_id}/task.md\">Task packet</a> · "
            f"<a href=\"../holdout/{episode_id}/PROVENANCE.json\">Provenance</a> · "
            f"<a href=\"../holdout/{episode_id}/attempt/SEALED.json\">Seal</a> · "
            f"<a href=\"{html.escape(report_link, quote=True)}\">Task Lens report</a> · "
            f"<a href=\"../holdout/{episode_id}/attempt/run/\">Artifacts</a></p>"
            f"<h3>Task</h3><p>{html.escape(task_text)}</p>"
            f"<h3>Separate 0–4 scores</h3><ul class=\"scores\">{scores}</ul>"
            f"<h3>Exact anchors and source</h3>{''.join(anchors)}"
            f"<h3>Working hypothesis</h3><ul>{hypothesis}</ul>"
            f"<h3>Reproduce or observe</h3><ul>{reproduce}</ul>"
            f"<h3>Verify</h3><p>{html.escape(str(pack.get('verify', {}).get('effect_to_observe')))}</p><ul>{verify}</ul>"
            f"<h3>Historical gold comparison</h3><p>{html.escape(row['gold_comparison'])}</p><ul>{gold_links}</ul>"
            f"<h3>Generic report comparison</h3><p>{html.escape(row['generic_report_comparison'])}</p>"
            f"<h3>Cost</h3><p>calls={provider.get('calls')}, input={provider.get('input_tokens')}, "
            f"output={provider.get('output_tokens')}, cache hit={provider.get('prompt_cache_hit_tokens')}, "
            f"provider ms={provider.get('latency_millis')}, wall ms={metrics.get('wall_millis')}, "
            f"grep={local.get('git_grep_queries')}, AST={local.get('ast_parses')}.</p>"
            f"<h3>Failure notes</h3><ul>{failures}</ul></section>"
        )
    target_items = "".join(
        f"<li class=\"{'pass' if value['passed'] else 'fail'}\">{html.escape(name.replace('_', ' '))}: "
        f"{'passed' if value['passed'] else 'not met'}</li>"
        for name, value in thresholds.items()
    )
    document = f"""<!doctype html>
<html lang=\"en\"><head><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width,initial-scale=1\">
<title>Task Lens v0 review</title><style>
:root{{--ink:#17202a;--muted:#59636e;--line:#d8dee4;--panel:#f7f9fa;--pass:#166534;--fail:#991b1b}}
body{{font:16px/1.5 system-ui,sans-serif;max-width:1100px;margin:2.5rem auto;padding:0 1rem;color:var(--ink)}}
	header,section{{border:1px solid var(--line);border-radius:.7rem;padding:1rem 1.25rem;margin:1rem 0;background:white}}
	.anchor{{background:var(--panel);padding:.75rem;margin:.75rem 0;border-radius:.5rem}} pre{{overflow:auto;background:#111827;color:#f8fafc;padding:.75rem;border-radius:.4rem}}
.scores{{columns:2;list-style:none;padding:0}} .pass{{color:var(--pass)}} .fail{{color:var(--fail)}}
a{{color:#075985;overflow-wrap:anywhere}} h1,h2,h3,h4{{overflow-wrap:anywhere}} code{{background:var(--panel);padding:.15rem .35rem;border-radius:.25rem}} .muted{{color:var(--muted)}}
@media (max-width:640px){{body{{margin:1rem auto}} header,section{{padding:.8rem}} .scores{{columns:1}}}}
</style></head><body><header><h1>Task Lens v0 holdout review</h1>
<p>Outcome: <strong>{html.escape(scorecard['outcome'])}</strong>. Scores remain separate; no average was computed.</p>
<p><a href=\"../SUPERVISOR_REPORT.md\">Supervisor report</a> · <a href=\"../HOLDOUT_EVALUATION.md\">Evaluation</a> · <a href=\"../HARNESS_AUDIT.md\">Harness audit</a> · <a href=\"../evaluation/gold/\">Historical gold</a></p>
<p class=\"muted\">Answer quality comparison: <code>answer_A_B_not_run</code>.</p></header>
<section><h2>Frozen holdout targets</h2><ul>{target_items}</ul></section>
{''.join(cards)}
</body></html>\n"""
    replace_text(root / "review/index.html", document)


def harness_audit_text(root: Path, state: str) -> str:
    return (
        "# Task Lens v0 harness audit\n\n"
        f"State: {state}\n\n"
        "- Every Git subprocess runs with all ambient `GIT_*` variables removed; the harness refuses identity/worktree-affecting Git variables (`GIT_PAGER` is harmless and allowed at entry).\n"
        "- Each episode uses a real detached worktree at its exact 40-hex base revision and neutral basename `repo`.\n"
        "- HEAD, tree hash, cleanliness, detachment, and later-added/explicitly-forbidden path absence are asserted.\n"
        "- Each episode also has a separately archived `.git`-free source-only export.\n"
        "- The checked-in owner prompt, README, task manifests, and checksum ledger must match their exact approved SHA-256 values.\n"
        "- Freeze preserves the tracked-plus-untracked implementation bytes in a deterministic content snapshot, excluding this experiment root.\n"
        "- Freeze requires each latest development attempt to be live, non-offline, candidate-binary-bound, and sealed; it also hashes the completed development evaluation and frozen harness/evaluator.\n"
        "- Every freeze verification replays development seals and checks prepared task/provenance sidecars against the frozen ledger.\n"
        "- Holdout preparation rejects Downloads and legacy benchmark paths and copies only the frozen binary and frozen `HOLDOUT_SET.json` as non-repository inputs.\n"
        "- A holdout attempt lock is consumed before the one CLI process starts; retries and semantic retries are not available.\n"
        "- Every artifact quartet is replayed in a temporary directory through the exact launched candidate/frozen binary; stable prompt, reducer/finalizer, sufficiency, and the complete report workspace must reproduce.\n"
        "- Pack, status, raw accepted response, supporting artifacts, metrics, stdout, stderr, report, and run-manifest four-artifact bindings are content-validated and SHA-256 sealed.\n"
        "- Manifest and evidence reads share one total budget; skipped stages are ordered exactly; legacy generic artifacts and invalid opaque IDs are rejected.\n"
        "- Provider attempt status and content sufficiency are independent; zero-anchor results are restricted to explicit broad/negative controls.\n"
        "- Expected cheap-exit episodes are declared and SHA-256 sealed before holdout preparation or historical gold access; the scorecard cannot redefine them.\n"
        "- Gold unlock verifies the global seal first and writes an immutable gold inventory.\n"
        "- A `passed` outcome requires every computed holdout target, including zero major unsupported claims; the final recommendation is the one recommended next step.\n"
        "- Subjective dimensions remain separate 0–4 scores; no opaque average is calculated.\n"
        "- Answer A/B is recorded as `answer_A_B_not_run`; the harness does not simulate independent sessions.\n\n"
        f"Review root: `{root}`\n"
    )


def protocol_state(root: Path) -> str:
    if (root / "evaluation/EVALUATION_RESULT.json").is_file():
        return "evaluation_rendered"
    if (root / "evaluation/GOLD_UNLOCK.json").is_file():
        return "gold_unlocked_after_seal"
    if (root / "holdout/HOLDOUT_SEAL.json").is_file():
        return "holdout_sealed_gold_locked"
    if (root / "holdout/PREPARED.json").is_file():
        return "holdout_prepared"
    if cheap_exit_expectation_paths(root)[0].is_file():
        return "cheap_exit_predeclared_holdout_unstarted"
    if (root / "FREEZE_MANIFEST.json").is_file():
        return "frozen"
    if (root / "dev/PREPARED.json").is_file():
        return "development_prepared"
    return "initialized"


def refresh_audit(root: Path) -> None:
    replace_text(root / "HARNESS_AUDIT.md", harness_audit_text(root, protocol_state(root)))


def print_review(args: argparse.Namespace) -> None:
    root = Path(args.root).expanduser().resolve()
    if not (root / "review/index.html").is_file():
        fail(f"review has not been initialized: {root}")
    if (root / "FREEZE_MANIFEST.json").is_file():
        verify_freeze(root)
    print("Supervisor report:")
    print(root / "SUPERVISOR_REPORT.md")
    print()
    print("User review:")
    print(review_server_command(root))
    print(review_url())


def self_test_zero_anchor_contract(provenance: dict[str, Any]) -> None:
    task_text = "Search for a symbol that is absent."
    task_sha = sha256_bytes(task_text.encode("utf-8"))
    state_sha = sha256_bytes(
        json.dumps(
            {"version": 2, "head": provenance["base_revision"], "dirty": []},
            separators=(",", ":"),
        ).encode("utf-8")
    )
    repository = {
        "identity": "example.invalid/source",
        "display_name": "repo",
        "revision": provenance["base_revision"],
        "tree_hash": provenance["tree_hash"],
        "state_sha256": state_sha,
        "identity_source": "root_module",
        "identity_source_path": "go.mod",
    }
    task_evidence_id = go_opaque_id("evidence", "task", task_sha)
    manifest_bytes = len(
        (provenance["episode_root"] / provenance["worktree"] / "go.mod").read_bytes()
    )
    budgets = {
        "initial_candidates": 0,
        "candidate_items_found": 0,
        "retained_anchors": 0,
        "anchor_items_found": 0,
        "evidence_files_considered": 0,
        "read_files": 1,
        "read_bytes": manifest_bytes,
        "retained_source_bytes": 0,
        "gopls_queries": 0,
        "frontier_expansions": 0,
        "local_wall_millis": 1,
        "candidate_limit_bound": False,
        "anchor_limit_bound": False,
        "file_limit_bound": False,
        "byte_limit_bound": False,
        "retained_byte_limit_bound": False,
        "time_limit_bound": False,
    }
    bundle = {
        "version": 1,
        "id": go_opaque_id(
            "task",
            repository["identity"],
            repository["revision"],
            repository["state_sha256"],
            task_sha,
        ),
        "repository": repository,
        "task": {"text": task_text, "evidence_id": task_evidence_id},
        "task_kind_hint": "unknown",
        "observable_hint": task_text,
        "locality": "broad_dynamic",
        "terms": [],
        "modules": [{
            "id": "module-root",
            "path": "example.invalid/source",
            "dir": ".",
            "source_path": "go.mod",
        }],
        "anchors": [],
        "evidence": [{
            "id": task_evidence_id,
            "kind": "task_provided",
            "summary": "Symptom or requested outcome supplied by the task; not repository truth.",
        }],
        "allowed_paths": ["go.mod"],
        "stages_skipped": list(CANONICAL_SKIPPED_STAGES),
        "budgets": budgets,
        "metrics": {
            "tracked_files": 2,
            "git_grep_queries": 0,
            "ast_parses": 0,
            "relations_retained": 0,
            "evidence_files_read": 0,
            "module_files_found": 1,
            "module_files_read": 1,
            "module_bytes_read": manifest_bytes,
            "manifest_files_read": 1,
            "manifest_bytes_read": manifest_bytes,
        },
    }
    validated = validate_bundle(bundle, provenance, task_text, True)
    bundle_sha = go_compact_json_sha(validated)
    pack = {
        "version": 1,
        "id": bundle["id"],
        "bundle_sha256": bundle_sha,
        "repository": repository,
        "locality": "broad_dynamic",
        "stages_skipped": list(CANONICAL_SKIPPED_STAGES),
        "task_interpretation": {
            "task_kind": "unknown",
            "restatement": task_text,
            "observable_or_outcome": task_text,
        },
        "task_observation_concrete": False,
        "likely_areas": [],
        "investigation_anchors": [],
        "working_hypothesis": [{
            "status": "unresolved",
            "text": "No exact bounded repository anchor was retained.",
            "support_ids": [],
            "relation_ids": [],
        }],
        "reproduce_or_observe": [{
            "authority": "missing_evidence",
            "text": "Obtain exact repository evidence before choosing an action.",
            "evidence_ids": [],
        }],
        "verify": {
            "effect_to_observe": task_text,
            "steps": [{
                "authority": "missing_evidence",
                "text": "Obtain exact repository evidence before choosing a verification step.",
                "evidence_ids": [],
            }],
        },
        "next_probes": [{
            "action": "search_task_terms",
            "anchor_ids": [],
            "text": "Search the tracked repository for one exact task term.",
        }],
        "budgets": budgets,
    }
    validate_pack(pack, validated, provenance, bundle_sha, "partial_local")
    if task_pack_sufficient(pack, "skipped_insufficient_evidence"):
        fail("self-test: zero-anchor broad_dynamic negative control claimed sufficiency")


def self_test_extension_sufficiency_contract() -> None:
    pack = {
        "locality": "extension_contribution",
        "investigation_anchors": [
            {
                "id": "anchor_integration",
                "path": "extension.go",
                "symbol": "RegisterExtension",
                "role": "integration_boundary",
                "evidence_ids": ["evidence_a"],
            },
            {
                "id": "anchor_verification",
                "path": "extension_test.go",
                "symbol": "TestExtension",
                "role": "verification_anchor",
                "evidence_ids": ["evidence_b"],
            },
        ],
        "likely_areas": [{"label": "integration"}, {"label": "verification"}],
        "evidence_joins": [{
            "left_anchor_id": "anchor_integration",
            "right_anchor_id": "anchor_verification",
            "support_type": "model_hypothesis",
            "relation_kind": "possible_extension_path",
            "support_ids": ["evidence_a", "evidence_b"],
        }],
        "working_hypothesis": [{
            "status": "supported",
            "text": "The retained integration relation reaches the verification target.",
            "support_ids": ["evidence_a", "evidence_b"],
            "relation_ids": ["relation_ab"],
        }],
        "task_interpretation": {"repository_terms_found": []},
        "task_observation_concrete": False,
        "reproduce_or_observe": [{
            "authority": "repository_observation",
            "evidence_ids": ["evidence_a"],
        }],
        "verify": {"steps": [{
            "authority": "repository_test_or_example",
            "evidence_ids": ["evidence_b"],
        }]},
    }
    if task_pack_sufficient(pack, "accepted"):
        fail("self-test: extension sufficiency accepted a model-only join")
    pack["evidence_joins"][0].update(
        support_type="locally_observed",
        relation_kind="shared_exact_task_term",
    )
    if task_pack_sufficient(pack, "accepted"):
        fail("self-test: extension sufficiency accepted only a shared task-term join")
    pack["evidence_joins"][0]["relation_kind"] = "calls"
    if not task_pack_sufficient(pack, "accepted"):
        fail("self-test: extension sufficiency rejected actionable local evidence")
    pack["investigation_anchors"].append({
        "id": "anchor_unrelated_test",
        "path": "config_unrelated_test.go",
        "symbol": "TestUnrelatedConfig",
        "role": "verification_anchor",
        "evidence_ids": ["evidence_c"],
    })
    pack["verify"]["steps"][0]["evidence_ids"] = ["evidence_c"]
    pack["task_interpretation"]["repository_terms_found"] = ["test", "config"]
    if task_pack_sufficient(pack, "accepted"):
        fail("self-test: unrelated verification passed on generic task terms")


def self_test() -> None:
    self_test_extension_sufficiency_contract()
    with tempfile.TemporaryDirectory(prefix="repomap-task-lens-harness-") as temporary:
        base = Path(temporary)
        source = base / "source"
        implementation = base / "implementation"
        source.mkdir()
        implementation.mkdir()
        for repo in (source, implementation):
            git(repo, "init", "-q")
            git(repo, "config", "user.email", "harness@example.invalid")
            git(repo, "config", "user.name", "Harness Check")

        (source / "go.mod").write_text("module example.invalid/source\n\ngo 1.22\n", encoding="utf-8")
        (source / "base.go").write_text("package source\n", encoding="utf-8")
        git(source, "add", "go.mod", "base.go")
        git(source, "commit", "-q", "-m", "base")
        first = git_output(source, "rev-parse", "HEAD")
        (source / "later.go").write_text("package source\n", encoding="utf-8")
        git(source, "add", "later.go")
        git(source, "commit", "-q", "-m", "later")
        second = git_output(source, "rev-parse", "HEAD")

        (implementation / "contract.txt").write_text("prompt schema policy\n", encoding="utf-8")
        git(implementation, "add", "contract.txt")
        git(implementation, "commit", "-q", "-m", "implementation")
        (implementation / "untracked_impl.py").write_text(
            "def frozen_untracked_source():\n    return 'preserved'\n", encoding="utf-8"
        )

        owner_inputs = base / "owner-inputs"
        owner_inputs.mkdir()
        dev_manifest = owner_inputs / "DEV_SET.json"
        holdout_manifest = owner_inputs / "HOLDOUT_SET.json"
        budgets = base / "BUDGETS.json"
        prompt = owner_inputs / "CODEX_TASK_LENS_V0_PROMPT.md"
        owner_readme = owner_inputs / "README.md"
        owner_checksums = owner_inputs / "MANIFEST.sha256"
        tasks = base / "tasks"
        tasks.mkdir()
        (tasks / "dev_case.md").write_text(
            "# Synthetic task packet\n\n"
            "## Prompt-safe task\n\n"
            "Find the base package.\n\n"
            "## Benchmark rules\n\n"
            "This text must not enter the Task Lens bundle.\n",
            encoding="utf-8",
        )
        dev_manifest.write_bytes(
            canonical_json(
                {
                    "repository": "example.invalid/source",
                    "role": "development_set",
                    "may_use_historical_gold": True,
                    "episodes": [{"id": "dev_case", "base_revision": first}],
                }
            )
        )
        holdout_manifest.write_bytes(
            canonical_json(
                {
                    "repository": "example.invalid/source",
                    "role": "frozen_holdout",
                    "may_use_historical_gold_before_seal": False,
                    "episodes": [
                        {"id": "holdout_case", "base_revision": second, "task": "Find the source package."}
                    ],
                }
            )
        )
        budgets.write_bytes(
            canonical_json(
                {
                    "initial_lexical_symbol_candidates": 40,
                    "retained_anchors_before_review": 16,
                    "visible_anchors_min": 3,
                    "visible_anchors_max": 8,
                    "source_document_files_read": 12,
                    "retained_source_document_bytes": 131072,
                    "gopls_queries": 12,
                    "named_frontier_expansions": 2,
                    "deterministic_local_seconds": 10,
                    "model_calls_hard_max": 4,
                    "model_calls_by_locality": {
                        "bounded_cross_file": 2,
                        "extension_contribution": 3,
                        "local_exact": 1,
                    },
                }
            )
        )
        prompt.write_text("Task Lens test prompt.\n", encoding="utf-8")
        owner_readme.write_text("Task Lens synthetic owner bundle.\n", encoding="utf-8")
        owner_checksums.write_text(
            "".join(
                f"{sha256_file(owner_inputs / name)}  {name}\n"
                for name in (
                    "CODEX_TASK_LENS_V0_PROMPT.md",
                    "DEV_SET.json",
                    "HOLDOUT_SET.json",
                    "README.md",
                )
            ),
            encoding="ascii",
        )
        fake = base / "repomap"
        fake.write_text(fake_binary_source(), encoding="utf-8")
        fake.chmod(0o755)
        root = implementation / "experiment-output"

        prepare_phase(
            argparse.Namespace(
                root=str(root), phase="dev", source_repo=str(source), manifest=str(dev_manifest), tasks_dir=str(tasks)
            )
        )
        provenance = read_json(root / "dev/dev_case/PROVENANCE.json")
        if (root / "dev/dev_case/worktree/repo/later.go").exists():
            fail("self-test: later file leaked into base worktree")
        if provenance["checks"]["neutral_basename"] != "repo":
            fail("self-test: neutral basename was not recorded")
        self_test_zero_anchor_contract(load_provenance(root / "dev/dev_case"))
        run_phase(argparse.Namespace(root=str(root), phase="dev", binary=str(fake), episode=None))
        dev_attempt = root / "dev/dev_case/attempts/001"
        if read_json(dev_attempt / "run/run/task_investigation_attempt.json")["state"] != "skipped_insufficient_evidence":
            fail("self-test: sparse non-offline skip state was not exercised")
        if read_json(dev_attempt / "SEALED.json")["state"] != "partial_local":
            fail("self-test: sparse skip did not map to partial_local")
        freeze_args = argparse.Namespace(
            root=str(root),
            implementation_repo=str(implementation),
            binary=str(fake),
            owner_prompt=str(prompt),
            owner_checksums=str(owner_checksums),
            budgets=str(budgets),
            dev_manifest=str(dev_manifest),
            holdout_manifest=str(holdout_manifest),
            contract=["contract.txt"],
        )
        try:
            freeze(freeze_args)
        except HarnessError as exc:
            if "live provider call" not in str(exc):
                raise
        else:
            fail("self-test: freeze accepted a development attempt without a live provider call")
        (root / "dev/dev_case/FORCE_LIVE").write_text("live\n", encoding="utf-8")
        run_phase(argparse.Namespace(root=str(root), phase="dev", binary=str(fake), episode=None))
        if read_json(root / "dev/dev_case/attempts/002/SEALED.json")["state"] != "accepted_partial":
            fail("self-test: live development attempt was not selected after sparse rejection")
        try:
            freeze(freeze_args)
        except HarnessError as exc:
            if "DEV_EVALUATION.md" not in str(exc):
                raise
        else:
            fail("self-test: freeze accepted the placeholder development evaluation")
        replace_text(
            root / "DEV_EVALUATION.md",
            "# Development evaluation\n\n"
            "Status: complete\n\n"
            "| Episode | Result |\n| --- | --- |\n"
            "| dev_case | Live synthetic provider attempt sealed against the candidate binary. |\n",
        )
        freeze(freeze_args)
        frozen_untracked = root / "freeze/implementation/untracked_impl.py"
        if frozen_untracked.read_text(encoding="utf-8") != (implementation / "untracked_impl.py").read_text(
            encoding="utf-8"
        ):
            fail("self-test: untracked implementation source bytes were not frozen")
        inventory = read_json(root / "freeze/SOURCE_INVENTORY.json")["files"]
        if not any(item.get("path") == "untracked_impl.py" and item.get("tracked") is False for item in inventory):
            fail("self-test: untracked implementation source was not identified in the freeze ledger")
        excluded_prefix = root.relative_to(implementation).as_posix() + "/"
        if any(item.get("path", "").startswith(excluded_prefix) for item in inventory):
            fail("self-test: experiment output leaked into the frozen implementation snapshot")

        original_snapshot = frozen_untracked.read_bytes()
        os.chmod(frozen_untracked, 0o644)
        frozen_untracked.write_bytes(b"tampered\n")
        try:
            verify_freeze(root)
        except HarnessError as exc:
            if "implementation file changed" not in str(exc):
                raise
        else:
            fail("self-test: freeze verification accepted a changed source snapshot")
        frozen_untracked.write_bytes(original_snapshot)
        os.chmod(frozen_untracked, 0o444)

        provenance_sidecar = root / "dev/dev_case/PROVENANCE.sha256"
        original_sidecar = provenance_sidecar.read_bytes()
        os.chmod(provenance_sidecar, 0o644)
        provenance_sidecar.write_text("0" * 64 + "  PROVENANCE.json\n", encoding="ascii")
        try:
            verify_freeze(root)
        except HarnessError as exc:
            if "provenance changed after preparation" not in str(exc):
                raise
        else:
            fail("self-test: freeze verification accepted a changed development provenance sidecar")
        provenance_sidecar.write_bytes(original_sidecar)
        os.chmod(provenance_sidecar, 0o444)
        verify_freeze(root)
        frozen_harness = root / "freeze/harness/task_lens_harness.py"
        frozen_evaluator = root / "freeze/harness/task_lens_eval.py"
        holdout_prepare_command = (
            sys.executable,
            str(frozen_harness),
            "prepare",
            "--root",
            str(root),
            "--phase",
            "holdout",
            "--source-repo",
            str(source),
            "--manifest",
            str(holdout_manifest),
        )
        undeclared_prepare = run_command(holdout_prepare_command, check=False, text=True)
        if (
            undeclared_prepare.returncode == 0
            or "cheap-exit expectations must be predeclared" not in undeclared_prepare.stderr
        ):
            fail("self-test: holdout preparation ran before cheap-exit expectations were declared")
        unknown_cheap_exit = run_command(
            (
                sys.executable,
                str(frozen_evaluator),
                "declare-cheap-exits",
                "--root",
                str(root),
                "--episode",
                "unknown_case",
            ),
            check=False,
            text=True,
        )
        if unknown_cheap_exit.returncode == 0 or "unknown holdout episode" not in unknown_cheap_exit.stderr:
            fail("self-test: an unknown cheap-exit episode was accepted")
        run_command(
            (
                sys.executable,
                str(frozen_evaluator),
                "declare-cheap-exits",
                "--root",
                str(root),
                "--episode",
                "holdout_case",
            )
        )
        run_command(holdout_prepare_command)
        gold = base / "gold"
        gold.mkdir()
        (gold / "holdout_case.md").write_text("Historical expected context.\n", encoding="utf-8")
        premature_unlock = run_command(
            (
                sys.executable,
                str(frozen_evaluator),
                "unlock-gold",
                "--root",
                str(root),
                "--gold-dir",
                str(gold),
            ),
            check=False,
            text=True,
        )
        if premature_unlock.returncode == 0 or "not fully sealed" not in premature_unlock.stderr:
            fail("self-test: gold unlocked before the global holdout seal")
        run_command(
            (sys.executable, str(frozen_harness), "run", "--root", str(root), "--phase", "holdout")
        )
        holdout_attempt = root / "holdout/holdout_case/attempt"
        if read_json(holdout_attempt / "run/run/task_investigation_attempt.json")["state"] != "accepted_with_rejections":
            fail("self-test: accepted-with-rejections state was not exercised")
        if read_json(holdout_attempt / "SEALED.json")["state"] != "accepted_partial":
            fail("self-test: accepted-with-rejections did not map to accepted_partial")
        if not any(
            item.get("path") == "run/latest" and item.get("kind") == "symlink"
            for item in read_json(holdout_attempt / "SEALED.json")["files"]
        ):
            fail("self-test: the production latest symlink was not sealed")
        run_command(
            (sys.executable, str(frozen_harness), "seal", "--root", str(root), "--phase", "holdout")
        )
        retry = run_command(
            (
                sys.executable,
                str(frozen_harness),
                "run",
                "--root",
                str(root),
                "--phase",
                "holdout",
                "--episode",
                "holdout_case",
            ),
            check=False,
            text=True,
        )
        if retry.returncode == 0 or "already consumed" not in retry.stderr:
            fail("self-test: a second holdout attempt was allowed")
        run_command(
            (
                sys.executable,
                str(frozen_evaluator),
                "unlock-gold",
                "--root",
                str(root),
                "--gold-dir",
                str(gold),
            )
        )
        scorecard_path = root / "evaluation/SCORECARD.json"
        scorecard = read_json(scorecard_path)
        if scorecard["episodes"][0]["cheap_exit_expected"] is not True:
            fail("self-test: scorecard did not inherit the pre-gold cheap-exit declaration")
        scorecard.update(
            {
                "outcome": "partial",
                "technical_result": "synthetic_protocol_passed",
                "product_result": "not_applicable_to_synthetic_fixture",
                "investment_result": "not_applicable_to_synthetic_fixture",
                "single_strongest_demonstrated_value": "sealed exact-revision execution",
                "single_main_blocker": "synthetic fixture does not measure product quality",
                "recommended_next_step": "run one cross-repository task holdout",
                "previous_benchmark_defects": {
                    defect: {
                        "reproduced": True,
                        "fixed": True,
                        "regression_test": "synthetic protocol regression",
                        "residual_risk": "synthetic fixtures do not measure external product quality",
                    }
                    for defect in BENCHMARK_DEFECTS
                },
                "task_lens_contract": {
                    field: "synthetic contract coverage"
                    for field in CONTRACT_SECTIONS
                },
                "development": {
                    "changes_during_development": "synthetic fixture only",
                    "generic_vs_fixture_only": "no production behavior is exercised",
                    "failures_shaping_contract": "schema drift is caught by the harness",
                    "no_episode_specific_production_rules": True,
                },
                "product_comparison": {
                    field: "not applicable to the synthetic protocol fixture"
                    for field in PRODUCT_COMPARISON_SECTIONS
                },
                "product_findings": {
                    field: "requires the real frozen holdout"
                    for field in PRODUCT_FINDING_QUESTIONS
                },
                "verification": {
                    "focused_checks": ["synthetic detached-worktree and seal regression"],
                    "broad_checks_skipped": ["external repository product evaluation"],
                },
                "final_recommendation": "run one cross-repository task holdout",
            }
        )
        scorecard["episodes"][0]["scores"] = {dimension: 3 for dimension in SCORE_DIMENSIONS}
        scorecard["episodes"][0]["cheap_exit_expected"] = True
        scorecard["episodes"][0]["gold_comparison"] = "synthetic gold remains post-seal evaluator input"
        scorecard["episodes"][0]["generic_report_comparison"] = "not applicable to the synthetic fixture"
        replace_json(scorecard_path, scorecard)

        scorecard["episodes"][0]["cheap_exit_expected"] = False
        replace_json(scorecard_path, scorecard)
        changed_cheap_exit = run_command(
            (
                sys.executable,
                str(frozen_evaluator),
                "evaluate",
                "--root",
                str(root),
                "--scores",
                str(scorecard_path),
            ),
            check=False,
            text=True,
        )
        if changed_cheap_exit.returncode == 0 or "differs from the sealed pre-gold declaration" not in changed_cheap_exit.stderr:
            fail("self-test: scorecard changed a predeclared cheap-exit expectation")
        scorecard["episodes"][0]["cheap_exit_expected"] = True

        scorecard["recommended_next_step"] = "a different next step"
        replace_json(scorecard_path, scorecard)
        mismatched_recommendation = run_command(
            (
                sys.executable,
                str(frozen_evaluator),
                "evaluate",
                "--root",
                str(root),
                "--scores",
                str(scorecard_path),
            ),
            check=False,
            text=True,
        )
        if mismatched_recommendation.returncode == 0 or "must exactly equal" not in mismatched_recommendation.stderr:
            fail("self-test: mismatched recommendation fields were accepted")
        scorecard["recommended_next_step"] = scorecard["final_recommendation"]

        try:
            validate_evaluation_outcome(
                "passed",
                {
                    "all_other_targets": {"passed": True},
                    "major_unsupported_claims": {"passed": False},
                },
            )
        except HarnessError as exc:
            if "zero-major-unsupported-claim gate" not in str(exc):
                raise
        else:
            fail("self-test: major unsupported claims did not block a passed outcome")

        scorecard["outcome"] = "passed"
        replace_json(scorecard_path, scorecard)
        false_pass = run_command(
            (
                sys.executable,
                str(frozen_evaluator),
                "evaluate",
                "--root",
                str(root),
                "--scores",
                str(scorecard_path),
            ),
            check=False,
            text=True,
        )
        if false_pass.returncode == 0 or "outcome cannot be passed" not in false_pass.stderr:
            fail("self-test: a passed outcome ignored failed computed holdout targets")
        scorecard["outcome"] = "partial"
        replace_json(scorecard_path, scorecard)
        run_command(
            (
                sys.executable,
                str(frozen_evaluator),
                "evaluate",
                "--root",
                str(root),
                "--scores",
                str(scorecard_path),
            )
        )
        evaluation_result = read_json(root / "evaluation/EVALUATION_RESULT.json")
        if (
            evaluation_result.get("dimensions") != list(SCORE_DIMENSIONS)
            or evaluation_result.get("opaque_average_computed") is not False
            or scorecard.get("answer_quality_comparison") != "answer_A_B_not_run"
        ):
            fail("self-test: evaluation collapsed scores or invented an answer-quality A/B")
        verify_freeze(root)
        supervisor = (root / "SUPERVISOR_REPORT.md").read_text(encoding="utf-8")
        if not supervisor.startswith("outcome: partial\n") or supervisor.count("recommended next step:") != 1:
            fail("self-test: supervisor report convention changed")

        env = os.environ.copy()
        env["GIT_DIR"] = str(source / ".git")
        result = subprocess.run(
            (sys.executable, str(Path(__file__).resolve()), "review", "--root", str(root)),
            env=env,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            check=False,
        )
        if result.returncode == 0 or b"ambient Git environment is forbidden" not in result.stderr:
            fail("self-test: ambient Git environment was not rejected")
        print("OK: Task Lens harness synthetic detached-worktree/freeze/holdout checks passed")


def self_test_proposal_from_pack(pack: dict[str, Any]) -> dict[str, Any]:
    interpretation = pack["task_interpretation"]
    proposal = {
        "version": 1,
        "task_interpretation": {
            "restatement": interpretation["restatement"],
            "task_kind": interpretation["task_kind"],
            "observable_or_outcome": interpretation["observable_or_outcome"],
        },
        "likely_areas": pack["likely_areas"],
        "anchors": [
            {"anchor_id": anchor["id"], "role": anchor["role"], "why": anchor["why"]}
            for anchor in pack["investigation_anchors"]
        ],
        "evidence_joins": [
            {
                key: value
                for key, value in join.items()
                if key != "id"
            }
            for join in pack.get("evidence_joins", [])
        ],
        "working_hypothesis": pack["working_hypothesis"],
        "reproduce_or_observe": pack["reproduce_or_observe"],
        "verify": pack["verify"],
        "next_probes": pack["next_probes"],
    }
    return proposal


def self_test_replace_json(path: Path, value: Any) -> None:
    os.chmod(path, 0o644)
    replace_json(path, value)


def self_test_rebind_report_and_manifest(
    run_dir: Path,
    binary: Path,
    *,
    replay_workspace: bool,
) -> None:
    paths = artifact_paths(run_dir)
    report_path = next(path for path in run_dir.rglob("report.json") if path.is_file())
    manifest_path = next(path for path in run_dir.rglob("run_manifest.json") if path.is_file())
    report = require_object(read_json(report_path), "mutation report")
    workspace = require_object(report.get("task_investigation"), "mutation Task Lens workspace")
    if replay_workspace:
        workspace = exact_task_workspace_replay(binary, paths, report_path)
        report["task_investigation"] = workspace
    hashes = {name: sha256_file(path) for name, path in paths.items()}
    workspace.update({
        "bundle_sha256": hashes["task_investigation_bundle.json"],
        "attempt_sha256": hashes["task_investigation_attempt.json"],
        "pack_sha256": hashes["task_investigation.json"],
        "status_sha256": hashes["task_investigation_status.json"],
    })
    self_test_replace_json(report_path, report)
    manifest = require_object(read_json(manifest_path), "mutation run manifest")
    material = require_object(manifest.get("material_inputs"), "mutation material inputs")
    material.update({
        "task_bundle_sha256": hashes["task_investigation_bundle.json"],
        "task_attempt_sha256": hashes["task_investigation_attempt.json"],
        "task_pack_sha256": hashes["task_investigation.json"],
        "task_status_sha256": hashes["task_investigation_status.json"],
    })
    manifest["report_sha256"] = sha256_file(report_path)
    self_test_replace_json(manifest_path, manifest)


def self_test_expect_artifact_rejection(
    run_dir: Path,
    provenance: dict[str, Any],
    binary: Path,
    label: str,
) -> None:
    try:
        validate_production_artifacts(run_dir, provenance, binary)
    except HarnessError:
        return
    fail(f"real binary smoke: {label} mutation was accepted")


def real_binary_smoke(binary_path: str) -> None:
    binary = resolve_existing(binary_path, "file")
    if not os.access(binary, os.X_OK):
        fail(f"real smoke binary is not executable: {binary}")
    with tempfile.TemporaryDirectory(prefix="repomap-task-lens-real-smoke-") as temporary:
        base = Path(temporary)
        source = base / "source"
        tasks = base / "tasks"
        source.mkdir()
        tasks.mkdir()
        git(source, "init", "-q")
        git(source, "config", "user.email", "harness@example.invalid")
        git(source, "config", "user.name", "Harness Check")
        (source / "go.mod").write_text(
            "module example.invalid/tasklenssmoke\n\ngo 1.22\n",
            encoding="utf-8",
        )
        (source / "greet.go").write_text(
            "package tasklenssmoke\n\n"
            "const greeting = \"hello\"\n\n"
            "func Greet() string {\n"
            "\treturn greeting\n"
            "}\n",
            encoding="utf-8",
        )
        (source / "greet_test.go").write_text(
            "package tasklenssmoke\n\n"
            "import \"testing\"\n\n"
            "func TestGreet(t *testing.T) {\n"
            "\tif got := Greet(); got != \"hello\" {\n"
            "\t\tt.Fatalf(\"Greet() = %q\", got)\n"
            "\t}\n"
            "}\n",
            encoding="utf-8",
        )
        git(source, "add", "go.mod", "greet.go", "greet_test.go")
        git(source, "commit", "-q", "-m", "smoke fixture")
        revision = git_output(source, "rev-parse", "HEAD")
        manifest = base / "DEV_SET.json"
        manifest.write_bytes(
            canonical_json(
                {
                    "repository": "example.invalid/tasklenssmoke",
                    "role": "development_set",
                    "may_use_historical_gold": True,
                    "episodes": [{"id": "offline_smoke", "base_revision": revision}],
                }
            )
        )
        (tasks / "offline_smoke.md").write_text(
            "Find where Greet returns its greeting and how the test verifies the output.\n",
            encoding="utf-8",
        )
        root = base / "run"
        prepare_phase(
            argparse.Namespace(
                root=str(root),
                phase="dev",
                source_repo=str(source),
                manifest=str(manifest),
                tasks_dir=str(tasks),
            )
        )
        run_phase(
            argparse.Namespace(
                root=str(root),
                phase="dev",
                binary=str(binary),
                episode=None,
                offline=True,
            )
        )
        attempt_root = root / "dev/offline_smoke/attempts/001"
        seal = read_json(attempt_root / "SEALED.json")
        artifacts = artifact_paths(attempt_root / "run")
        attempt = read_json(artifacts["task_investigation_attempt.json"])
        status = read_json(artifacts["task_investigation_status.json"])
        if attempt.get("state") != "skipped_offline" or status.get("state") != "partial_local":
            fail("real binary smoke did not exercise skipped_offline -> partial_local")
        if (
            seal.get("state") != "partial_local"
            or seal.get("sufficient") is not True
            or status.get("sufficient") is not True
        ):
            fail("real binary smoke did not preserve content-based sufficiency for the offline local pack")

        provenance = load_provenance(root / "dev/offline_smoke")
        source_run = attempt_root / "run"
        with tempfile.TemporaryDirectory(prefix="repomap-task-lens-mutations-") as mutations:
            mutation_root = Path(mutations)

            accepted_run = mutation_root / "accepted"
            shutil.copytree(source_run, accepted_run, symlinks=True)
            accepted_paths = artifact_paths(accepted_run)
            accepted_pack = require_object(
                read_json(accepted_paths["task_investigation.json"]),
                "accepted mutation pack",
            )
            if accepted_pack.get("warnings", []) != []:
                fail("real binary smoke needs a sufficient warning-free pack for accepted replay")
            proposal = self_test_proposal_from_pack(accepted_pack)
            raw_response = json.dumps(proposal, ensure_ascii=False, separators=(",", ":"))
            accepted_attempt = require_object(
                read_json(accepted_paths["task_investigation_attempt.json"]),
                "accepted mutation attempt",
            )
            accepted_provider = {
                "calls": 1,
                "transport_attempts": 1,
                "request_bytes": 64,
                "response_bytes": len(raw_response.encode("utf-8")),
                "input_tokens": 8,
                "output_tokens": 8,
                "prompt_cache_hit_tokens": 0,
                "prompt_cache_miss_tokens": 8,
                "latency_millis": 1,
            }
            accepted_attempt.update({
                "state": "accepted",
                "warnings": [],
                "provider": accepted_provider,
                "response_sha256": sha256_bytes(raw_response.encode("utf-8")),
                "raw_response": raw_response,
            })
            accepted_attempt.pop("raw_response_omitted_reason", None)
            accepted_attempt.pop("reduction_error", None)
            self_test_replace_json(
                accepted_paths["task_investigation_attempt.json"],
                accepted_attempt,
            )
            accepted_status = require_object(
                read_json(accepted_paths["task_investigation_status.json"]),
                "accepted mutation status",
            )
            accepted_status.update({
                "state": "accepted",
                "provider": accepted_provider,
                "attempt_sha256": sha256_file(
                    accepted_paths["task_investigation_attempt.json"]
                ),
                "pack_sha256": sha256_file(accepted_paths["task_investigation.json"]),
                "warnings": [],
            })
            self_test_replace_json(
                accepted_paths["task_investigation_status.json"],
                accepted_status,
            )
            self_test_rebind_report_and_manifest(
                accepted_run,
                binary,
                replay_workspace=True,
            )
            validate_production_artifacts(accepted_run, provenance, binary)

            forged_run = mutation_root / "forged-response"
            shutil.copytree(accepted_run, forged_run, symlinks=True)
            forged_paths = artifact_paths(forged_run)
            forged_attempt = require_object(
                read_json(forged_paths["task_investigation_attempt.json"]),
                "forged mutation attempt",
            )
            forged_proposal = require_object(
                json.loads(forged_attempt["raw_response"]),
                "forged raw proposal",
            )
            forged_proposal["task_interpretation"]["restatement"] = (
                "FORGED DIFFERENT INTERPRETATION"
            )
            forged_raw = json.dumps(
                forged_proposal,
                ensure_ascii=False,
                separators=(",", ":"),
            )
            forged_attempt["raw_response"] = forged_raw
            forged_attempt["response_sha256"] = sha256_bytes(forged_raw.encode("utf-8"))
            forged_attempt["provider"]["response_bytes"] = len(forged_raw.encode("utf-8"))
            self_test_replace_json(
                forged_paths["task_investigation_attempt.json"],
                forged_attempt,
            )
            forged_status = require_object(
                read_json(forged_paths["task_investigation_status.json"]),
                "forged mutation status",
            )
            forged_status["provider"] = forged_attempt["provider"]
            forged_status["attempt_sha256"] = sha256_file(
                forged_paths["task_investigation_attempt.json"]
            )
            self_test_replace_json(
                forged_paths["task_investigation_status.json"],
                forged_status,
            )
            self_test_rebind_report_and_manifest(
                forged_run,
                binary,
                replay_workspace=False,
            )
            self_test_expect_artifact_rejection(
                forged_run,
                provenance,
                binary,
                "rehashed raw-response restatement",
            )

            report_swap_run = mutation_root / "report-swap"
            shutil.copytree(source_run, report_swap_run, symlinks=True)
            report_path = next(
                path for path in report_swap_run.rglob("report.json") if path.is_file()
            )
            swapped_report = require_object(read_json(report_path), "swapped report")
            swapped_report["task_investigation"]["interpretation"]["restatement"] = (
                "FORGED REPORT PROJECTION"
            )
            self_test_replace_json(report_path, swapped_report)
            manifest_path = next(
                path for path in report_swap_run.rglob("run_manifest.json") if path.is_file()
            )
            swapped_manifest = require_object(read_json(manifest_path), "swapped manifest")
            swapped_manifest["report_sha256"] = sha256_file(report_path)
            self_test_replace_json(manifest_path, swapped_manifest)
            self_test_expect_artifact_rejection(
                report_swap_run,
                provenance,
                binary,
                "rehashed report projection",
            )

            artifact_swap_run = mutation_root / "artifact-swap"
            shutil.copytree(source_run, artifact_swap_run, symlinks=True)
            artifact_report_path = next(
                path for path in artifact_swap_run.rglob("report.json") if path.is_file()
            )
            artifact_report = require_object(
                read_json(artifact_report_path),
                "artifact swap report",
            )
            artifact_report["task_investigation"]["pack_sha256"] = "0" * 64
            self_test_replace_json(artifact_report_path, artifact_report)
            artifact_manifest_path = next(
                path for path in artifact_swap_run.rglob("run_manifest.json") if path.is_file()
            )
            artifact_manifest = require_object(
                read_json(artifact_manifest_path),
                "artifact swap manifest",
            )
            artifact_manifest["material_inputs"]["task_pack_sha256"] = "0" * 64
            artifact_manifest["report_sha256"] = sha256_file(artifact_report_path)
            self_test_replace_json(artifact_manifest_path, artifact_manifest)
            self_test_expect_artifact_rejection(
                artifact_swap_run,
                provenance,
                binary,
                "report/artifact hash swap",
            )

            prompt_swap_run = mutation_root / "prompt-sha"
            shutil.copytree(source_run, prompt_swap_run, symlinks=True)
            prompt_paths = artifact_paths(prompt_swap_run)
            prompt_attempt = require_object(
                read_json(prompt_paths["task_investigation_attempt.json"]),
                "prompt mutation attempt",
            )
            prompt_attempt["prompt_sha256"] = "0" * 64
            self_test_replace_json(
                prompt_paths["task_investigation_attempt.json"],
                prompt_attempt,
            )
            prompt_status = require_object(
                read_json(prompt_paths["task_investigation_status.json"]),
                "prompt mutation status",
            )
            prompt_status["attempt_sha256"] = sha256_file(
                prompt_paths["task_investigation_attempt.json"]
            )
            self_test_replace_json(
                prompt_paths["task_investigation_status.json"],
                prompt_status,
            )
            self_test_rebind_report_and_manifest(
                prompt_swap_run,
                binary,
                replay_workspace=False,
            )
            self_test_expect_artifact_rejection(
                prompt_swap_run,
                provenance,
                binary,
                "rehashed stable-prompt identity",
            )
        print("OK: current repomap binary passed the offline Task Lens artifact/seal smoke")


def fake_binary_source() -> str:
    return r'''#!/usr/bin/env python3
import hashlib
import json
import subprocess
import sys
from pathlib import Path

if sys.argv[1:3] == ["dev", "render-report"]:
    replay_report = Path(sys.argv[3]) / "report.json" if len(sys.argv) == 4 else None
    if replay_report is None or not replay_report.is_file():
        raise SystemExit(2)
    print(f"Report: {replay_report.parent / 'report.html'}")
    raise SystemExit(0)

def opaque_id(kind, *parts):
    digest = hashlib.sha256()
    digest.update(("repomap-task-lens-v1\0" + kind).encode())
    for part in parts:
        digest.update(("\0" + part).encode())
    return kind + "-" + digest.hexdigest()[:20]

def compact_sha(value):
    raw = json.dumps(value, ensure_ascii=False, separators=(",", ":"))
    raw = raw.replace("&", "\\u0026").replace("<", "\\u003c").replace(">", "\\u003e")
    raw = raw.replace("\u2028", "\\u2028").replace("\u2029", "\\u2029")
    return hashlib.sha256(raw.encode()).hexdigest()

repo = Path(sys.argv[2])
debug = Path(sys.argv[sys.argv.index("--debug-dir") + 1]) / "run"
debug.mkdir(parents=True)
(debug.parent / "latest").symlink_to(debug.name)
revision = subprocess.check_output(["git", "-C", str(repo), "rev-parse", "HEAD"], text=True).strip()
tree = subprocess.check_output(["git", "-C", str(repo), "rev-parse", "HEAD^{tree}"], text=True).strip()
task_file = Path(sys.argv[sys.argv.index("--task-file") + 1])
task_text = task_file.read_text(encoding="utf-8").replace("\r\n", "\n").strip()
heading = "## Prompt-safe task"
start = task_text.find(heading)
if start >= 0:
    task_text = task_text[start + len(heading):]
    next_heading = task_text.find("\n## ")
    if next_heading >= 0:
        task_text = task_text[:next_heading]
    task_text = task_text.strip()
force_live = (repo.parent.parent / "FORCE_LIVE").is_file()
skip = ("base package" in task_text.lower() and not force_live) or "--offline" in sys.argv
anchor_count = 1 if skip else 3
repository = {
    "identity": "example.invalid/source",
    "display_name": "repo",
    "revision": revision,
    "tree_hash": tree,
    "state_sha256": hashlib.sha256(json.dumps(
        {"version": 2, "head": revision, "dirty": []}, separators=(",", ":")
    ).encode()).hexdigest(),
    "identity_source": "root_module",
    "identity_source_path": "go.mod",
}
task_sha = hashlib.sha256(task_text.encode()).hexdigest()
task_id = opaque_id(
    "task", repository["identity"], revision, repository["state_sha256"], task_sha
)
task_evidence_id = opaque_id("evidence", "task", task_sha)
stages = [
    "architecture_synthesis",
    "generic_orientation",
    "guided_tour",
    "mechanism_opportunity",
    "paved_paths",
    "repository_study_map",
    "runtime_surface_discovery",
]
budgets = {
    "initial_candidates": 2,
    "candidate_items_found": 2,
    "retained_anchors": anchor_count,
    "anchor_items_found": anchor_count,
    "evidence_files_considered": 2,
    "read_files": 2,
    "read_bytes": len((repo / "base.go").read_bytes()) + len((repo / "go.mod").read_bytes()),
    "retained_source_bytes": 0,
    "gopls_queries": 0,
    "frontier_expansions": 0,
    "local_wall_millis": 1,
    "candidate_limit_bound": False,
    "anchor_limit_bound": False,
    "file_limit_bound": False,
    "byte_limit_bound": False,
    "retained_byte_limit_bound": False,
    "time_limit_bound": False,
}
anchors = [
    {
        "id": "anchor-base-package",
        "path": "base.go",
        "symbol": "package source",
        "role_hints": ["symptom_site"],
        "start_line": 1,
        "end_line": 1,
        "excerpt": [{"line": 1, "text": "package source"}],
        "evidence_ids": ["evidence-base-package"],
    },
    {
        "id": "anchor-root-module",
        "path": "go.mod",
        "symbol": "module example.invalid/source",
        "role_hints": ["public_or_cli_entry"],
        "start_line": 1,
        "end_line": 1,
        "excerpt": [{"line": 1, "text": "module example.invalid/source"}],
        "evidence_ids": ["evidence-root-module"],
    },
    {
        "id": "anchor-go-version",
        "path": "go.mod",
        "symbol": "go 1.22",
        "role_hints": ["verification_anchor"],
        "start_line": 3,
        "end_line": 3,
        "excerpt": [{"line": 3, "text": "go 1.22"}],
        "evidence_ids": ["evidence-go-version"],
    },
]
anchors = anchors[:anchor_count]
for anchor in anchors:
    excerpt_sha = compact_sha(anchor["excerpt"])
    anchor["id"] = opaque_id(
        "anchor",
        anchor["path"],
        anchor["symbol"],
        str(anchor["start_line"]),
        str(anchor["end_line"]),
        excerpt_sha,
    )
    anchor["evidence_ids"] = [opaque_id(
        "evidence",
        repository["state_sha256"],
        anchor["path"],
        str(anchor["start_line"]),
        str(anchor["end_line"]),
        excerpt_sha,
    )]
grounded_term = "base" if "base" in task_text.lower() else "source"
grounded_term_evidence = sorted({
    evidence_id
    for anchor in anchors
    if grounded_term in (anchor["path"] + "\n" + anchor["symbol"]).lower()
    for evidence_id in anchor["evidence_ids"]
})
terms = [{
    "id": opaque_id("term", grounded_term),
    "text": grounded_term,
    "normalized": grounded_term,
    "found": True,
    "evidence_ids": grounded_term_evidence,
    "weight": 2,
}]
base_anchor_id = anchors[0]["id"]
base_evidence_id = anchors[0]["evidence_ids"][0]
budgets["retained_source_bytes"] = sum(
    len(line["text"].encode()) + 1 for anchor in anchors for line in anchor["excerpt"]
)
evidence = [{
    "id": task_evidence_id,
    "kind": "task_provided",
    "summary": "Symptom or requested outcome supplied by the task; not repository truth.",
}]
for anchor in anchors:
    evidence.append({
        "id": anchor["evidence_ids"][0],
        "kind": "repository_fact",
        "path": anchor["path"],
        "start_line": anchor["start_line"],
        "end_line": anchor["end_line"],
        "anchor_id": anchor["id"],
        "summary": (
            f"Exact repository source excerpt for {anchor['symbol']} at "
            f"{anchor['path']}:{anchor['start_line']}-{anchor['end_line']}."
        ),
    })
provider = {
    "calls": 0 if skip else 1,
    "transport_attempts": 0 if skip else 1,
    "request_bytes": 0 if skip else 64,
    "response_bytes": 0 if skip else 64,
    "input_tokens": 0 if skip else 8,
    "output_tokens": 0 if skip else 8,
    "prompt_cache_hit_tokens": 0,
    "prompt_cache_miss_tokens": 0 if skip else 8,
    "latency_millis": 0 if skip else 1,
}
bundle = {
    "version": 1,
    "id": task_id,
    "repository": repository,
    "task": {"text": task_text, "evidence_id": task_evidence_id},
    "task_kind_hint": "unknown",
    "locality": "local_exact",
    "observable_hint": "Find the synthetic package evidence.",
    "terms": terms,
    "modules": [{
        "id": "module-root",
        "path": "example.invalid/source",
        "dir": ".",
        "source_path": "go.mod",
    }],
    "allowed_paths": sorted({anchor["path"] for anchor in anchors} | {"go.mod"}),
    "evidence": evidence,
    "anchors": anchors,
    "relations": [],
    "stages_skipped": stages,
    "budgets": budgets,
    "metrics": {
        "tracked_files": len(subprocess.check_output(
            ["git", "-C", str(repo), "ls-files"], text=True
        ).splitlines()),
        "git_grep_queries": 1,
        "ast_parses": 1,
        "relations_retained": 0,
        "evidence_files_read": 2,
        "module_files_found": 1,
        "module_files_read": 1,
        "module_bytes_read": len((repo / "go.mod").read_bytes()),
        "manifest_files_read": 1,
        "manifest_bytes_read": len((repo / "go.mod").read_bytes()),
    },
}
compact = json.dumps(bundle, ensure_ascii=False, separators=(",", ":"))
compact = compact.replace("&", "\\u0026").replace("<", "\\u003c").replace(">", "\\u003e")
compact = compact.replace("\u2028", "\\u2028").replace("\u2029", "\\u2029")
bundle_sha = hashlib.sha256(compact.encode()).hexdigest()
attempt_state = (
    "skipped_offline" if "--offline" in sys.argv
    else "skipped_insufficient_evidence" if skip
    else "accepted_with_rejections"
)
roles = ["symptom_site", "public_or_cli_entry", "verification_anchor"]
raw_proposal = {
    "version": 1,
    "task_interpretation": {
        "restatement": "Locate the synthetic package evidence.",
        "task_kind": "unknown",
        "observable_or_outcome": "The package and module declarations are visible.",
    },
    "likely_areas": [{
        "label": "synthetic root package",
        "why": "The retained anchors identify the root package and module.",
        "target_ids": [anchor["id"] for anchor in anchors],
    }],
    "anchors": [
        {
            "anchor_id": anchor["id"],
            "role": role,
            "why": "Exact retained repository evidence for the task.",
        }
        for anchor, role in zip(anchors, roles)
    ],
    "evidence_joins": [{
        "left_anchor_id": base_anchor_id,
        "right_anchor_id": base_anchor_id,
        "relation_kind": "invalid_self_relation",
        "support_type": "unresolved",
        "support_ids": [],
        "explanation": "This optional synthetic self-relation must be rejected.",
        "scope_non_guarantees": "No runtime relation is asserted.",
    }],
    "working_hypothesis": [{
        "status": "supported",
        "text": "The retained declaration is the exact local investigation point.",
        "support_ids": [base_evidence_id],
        "relation_ids": [],
    }],
    "reproduce_or_observe": [{
        "text": "Inspect the retained base package declaration.",
        "authority": "repository_observation",
        "evidence_ids": [base_evidence_id],
    }],
    "verify": {
        "effect_to_observe": "The exact base revision remains bound to the pack.",
        "steps": [{
            "text": "Compare the retained package declaration with the sealed revision.",
            "authority": "repository_observation",
            "evidence_ids": [base_evidence_id],
        }],
    },
    "next_probes": [{
        "action": "inspect_symbol",
        "anchor_ids": [base_anchor_id],
        "text": "Inspect the exact package declaration.",
    }],
}
raw_response = "" if skip else json.dumps(raw_proposal, separators=(",", ":"))
if raw_response:
    provider["response_bytes"] = len(raw_response.encode())
attempt_warnings = (
    ["Semantic synthesis was skipped because bounded retrieval retained fewer than three source anchors."]
    if attempt_state == "skipped_insufficient_evidence"
    else ["Evidence join 1 was rejected locally: invalid content."]
    if attempt_state == "accepted_with_rejections"
    else []
)
attempt = {
    "version": 1,
    "bundle_sha256": bundle_sha,
    "prompt_version": "task-investigation-pack-json-v1",
    "prompt_sha256": hashlib.sha256(b"synthetic prompt").hexdigest(),
    "state": attempt_state,
    "warnings": attempt_warnings,
    "provider": provider,
}
if raw_response:
    attempt["response_sha256"] = hashlib.sha256(raw_response.encode()).hexdigest()
    attempt["raw_response"] = raw_response
pack_anchors = []
for anchor, role in zip(anchors, roles):
    pack_anchor = dict(anchor)
    pack_anchor.update({"role": role, "why": "Exact synthetic evidence for the protocol fixture."})
    pack_anchors.append(pack_anchor)
pack = {
    "version": 1,
    "id": task_id,
    "bundle_sha256": bundle_sha,
    "repository": repository,
    "locality": "local_exact",
    "stages_skipped": stages,
    "investigation_anchors": pack_anchors,
    "likely_areas": [{
        "label": "synthetic root package",
        "why": "The retained anchors identify the root package and module.",
        "target_ids": [anchor["id"] for anchor in anchors],
    }],
    "task_interpretation": {
        "task_kind": "unknown",
        "restatement": "Locate the synthetic package evidence.",
        "observable_or_outcome": "The package and module declarations are visible.",
        "repository_terms_found": [grounded_term],
        "user_provided_only_terms": [],
    },
    "task_observation_concrete": False,
    "evidence_joins": [],
    "working_hypothesis": [{
        "status": "supported",
        "text": "The base file belongs to the declared root module.",
        "support_ids": [base_evidence_id],
        "relation_ids": [],
    }],
    "reproduce_or_observe": [{
        "authority": "repository_observation",
        "text": "Inspect the retained base package declaration.",
        "evidence_ids": [base_evidence_id],
    }],
    "verify": {
        "effect_to_observe": "The exact base revision remains bound to the pack.",
        "steps": [{
            "authority": "repository_observation",
            "text": "Compare the retained package declaration with the sealed revision.",
            "evidence_ids": [base_evidence_id],
        }],
    },
    "next_probes": [{
        "action": "inspect_symbol",
        "anchor_ids": [base_anchor_id],
        "text": "Inspect the exact package declaration.",
    }],
    "budgets": budgets,
}
pack_sufficient = not skip
pack["warnings"] = [] if pack_sufficient else [
    "This deterministic local pack remains partial because the bounded evidence does not support a complete, actionable task lens."
]

def encode(value):
    return (json.dumps(value) + "\n").encode()

bundle_bytes = encode(bundle)
attempt_bytes = encode(attempt)
pack_bytes = encode(pack)
status = {
    "version": 1,
    "state": "partial_local" if skip else "accepted_partial",
    "sufficient": pack_sufficient,
    "task_id": task_id,
    "captured_revision": revision,
    "tree_hash": tree,
    "bundle_sha256": bundle_sha,
    "attempt_sha256": hashlib.sha256(attempt_bytes).hexdigest(),
    "pack_sha256": hashlib.sha256(pack_bytes).hexdigest(),
    "locality": "local_exact",
    "provider": provider,
    "budgets": budgets,
    "stages_skipped": stages,
    "warnings": list(dict.fromkeys(attempt_warnings + pack["warnings"])),
}
status_bytes = encode(status)
(debug / "task_investigation_bundle.json").write_bytes(bundle_bytes)
(debug / "task_investigation_attempt.json").write_bytes(attempt_bytes)
(debug / "task_investigation.json").write_bytes(pack_bytes)
(debug / "task_investigation_status.json").write_bytes(status_bytes)
openable_paths = sorted({anchor["path"] for anchor in pack_anchors})
report_anchors = [{
    "path": anchor["path"],
    "symbol": anchor["symbol"],
    "role": anchor["role"],
    "start_line": anchor["start_line"],
    "end_line": anchor["end_line"],
    "why": anchor["why"],
    "source": {"path": anchor["path"], "start_line": anchor["start_line"], "lines": anchor["excerpt"]},
} for anchor in pack_anchors]
report = {
    "format_version": 26,
    "captured_revision": revision,
    "openable_paths": openable_paths,
    "components": [],
    "task_investigation": {
        "task_id": task_id,
        "repository": repository["identity"],
        "task": task_text,
        "state": status["state"],
        "sufficient": status["sufficient"],
        "locality": bundle["locality"],
        "interpretation": pack["task_interpretation"],
        "likely_areas": [],
        "anchors": report_anchors,
        "working_hypothesis": [],
        "reproduce_or_observe": [],
        "verify": {"effect_to_observe": pack["verify"]["effect_to_observe"], "steps": []},
        "stages_skipped": stages,
        "budget": budgets,
        "provider": provider,
        "captured_revision": revision,
        "warnings": status["warnings"],
        "bundle_sha256": hashlib.sha256(bundle_bytes).hexdigest(),
        "attempt_sha256": hashlib.sha256(attempt_bytes).hexdigest(),
        "pack_sha256": hashlib.sha256(pack_bytes).hexdigest(),
        "status_sha256": hashlib.sha256(status_bytes).hexdigest(),
    },
}
report_bytes = encode(report)
(debug / "report.json").write_bytes(report_bytes)
(debug / "report.html").write_text("<!doctype html><title>Task Lens</title>\n", encoding="utf-8")

captured_inputs = []
for file_path in bundle["allowed_paths"]:
    stages_for_input = ["report_evidence"]
    if file_path in {"go.mod", "go.sum", "go.work", "go.work.sum"}:
        stages_for_input = ["go_build", "report_evidence"]
    captured_inputs.append({
        "version": 1,
        "id": hashlib.sha256(("captured-input-v1\0" + file_path).encode()).hexdigest(),
        "path": file_path,
        "kind": "file",
        "mode": "100644",
        "content_sha256": hashlib.sha256((repo / file_path).read_bytes()).hexdigest(),
        "stages": stages_for_input,
    })
captured_inputs.sort(key=lambda item: item["path"])
repository_state = {
    "version": 2,
    "identity": str(repo.resolve()),
    "head": revision,
    "dirty": [],
}
manifest = {
    "version": 3,
    "repository_state": repository_state,
    "analysis_root": str(repo.resolve()),
    "repository_state_sha256": compact_sha(repository_state),
    "report_sha256": hashlib.sha256(report_bytes).hexdigest(),
    "report_format_version": report["format_version"],
    "openable_paths": openable_paths,
    "components": [],
    "captured_inputs": captured_inputs,
    "captured_inputs_sha256": compact_sha(captured_inputs),
    "freshness": {
        "version": 1,
        "state": "fresh",
        "analyzed_changes": False,
        "unrelated_changes": False,
        "compared_at": "2026-01-01T00:00:00Z",
    },
    "material_inputs": {
        "selected_revision": revision,
        "input_policy_version": "captured-inputs-v1",
        "architecture_contract": 1,
        "report_contract": report["format_version"],
        "task_bundle_sha256": hashlib.sha256(bundle_bytes).hexdigest(),
        "task_attempt_sha256": hashlib.sha256(attempt_bytes).hexdigest(),
        "task_pack_sha256": hashlib.sha256(pack_bytes).hexdigest(),
        "task_status_sha256": hashlib.sha256(status_bytes).hexdigest(),
    },
}
(debug / "run_manifest.json").write_bytes(encode(manifest))
'''


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description=__doc__)
    subparsers = parser.add_subparsers(dest="command", required=True)

    init = subparsers.add_parser("init", help="initialize the review bundle without running an episode")
    init.add_argument("--root", required=True)
    init.set_defaults(func=lambda args: init_templates(Path(args.root).expanduser().resolve()))

    prepare = subparsers.add_parser("prepare", help="prepare exact detached worktrees and Git-free exports")
    prepare.add_argument("--root", required=True)
    prepare.add_argument("--phase", choices=("dev", "holdout"), required=True)
    prepare.add_argument("--source-repo", required=True)
    prepare.add_argument("--manifest", required=True)
    prepare.add_argument("--tasks-dir")
    prepare.set_defaults(func=prepare_phase)

    run = subparsers.add_parser("run", help="run and seal Task Lens attempts")
    run.add_argument("--root", required=True)
    run.add_argument("--phase", choices=("dev", "holdout"), required=True)
    run.add_argument("--binary")
    run.add_argument("--episode")
    run.add_argument("--offline", action="store_true", help="development-only local partial smoke")
    run.set_defaults(func=run_phase)

    seal = subparsers.add_parser("seal", help="validate and seal already-produced attempts")
    seal.add_argument("--root", required=True)
    seal.add_argument("--phase", choices=("dev", "holdout"), required=True)
    seal.add_argument("--episode")
    seal.add_argument("--binary", help="exact candidate binary required for development sealing")
    seal.set_defaults(func=seal_phase)

    freeze_parser = subparsers.add_parser("freeze", help="freeze implementation, contracts, budgets, tasks, and binary")
    freeze_parser.add_argument("--root", required=True)
    freeze_parser.add_argument("--implementation-repo", required=True)
    freeze_parser.add_argument("--binary", required=True)
    freeze_parser.add_argument("--owner-prompt", required=True)
    freeze_parser.add_argument("--owner-checksums", required=True)
    freeze_parser.add_argument("--budgets", required=True)
    freeze_parser.add_argument("--dev-manifest", required=True)
    freeze_parser.add_argument("--holdout-manifest", required=True)
    freeze_parser.add_argument("--contract", action="append", default=[])
    freeze_parser.set_defaults(func=freeze)

    cheap_exit = subparsers.add_parser(
        "declare-cheap-exits",
        help="seal expected cheap-exit episode IDs before holdout execution and gold access",
    )
    cheap_exit.add_argument("--root", required=True)
    cheap_exit.add_argument("--episode", action="append", required=True)
    cheap_exit.set_defaults(func=declare_cheap_exits)

    unlock = subparsers.add_parser("unlock-gold", help="copy and inventory gold only after the global holdout seal")
    unlock.add_argument("--root", required=True)
    unlock.add_argument("--gold-dir", required=True)
    unlock.set_defaults(func=unlock_gold)

    evaluate = subparsers.add_parser("evaluate", help="validate supervisor scores and render the review bundle")
    evaluate.add_argument("--root", required=True)
    evaluate.add_argument("--scores", required=True)
    evaluate.set_defaults(func=render_evaluation)

    review = subparsers.add_parser("review", help="print the stable supervisor and localhost review convention")
    review.add_argument("--root", required=True)
    review.set_defaults(func=print_review)

    check = subparsers.add_parser("self-test", help="run a synthetic no-network protocol regression")
    check.set_defaults(func=lambda _args: self_test())

    real_smoke = subparsers.add_parser(
        "real-smoke",
        help="run the current repomap binary once in offline development mode",
    )
    real_smoke.add_argument("--binary", required=True)
    real_smoke.set_defaults(func=lambda args: real_binary_smoke(args.binary))
    return parser


def main() -> int:
    try:
        reject_ambient_git_env()
        args = build_parser().parse_args()
        args.func(args)
        return 0
    except HarnessError as exc:
        print(f"task-lens-harness: error: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
