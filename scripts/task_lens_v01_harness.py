#!/usr/bin/env python3
"""One-shot preparation, execution, and sealing for Task Lens v0.1 episodes.

The harness intentionally never executes commands from the target repository.
It launches one ``repomap investigate`` process per final episode, records that
attempt before launch, and refuses retries.  A post-calibration route plan is
frozen before final execution.  Episodes selected for zero-call run without
``--offline`` and must prove a genuine local result; all others run offline and
must remain honest, insufficient local partials.
"""

from __future__ import annotations

import argparse
import datetime as dt
import hashlib
import json
import os
import re
import secrets
import shutil
import subprocess
import sys
import time
from pathlib import Path
from typing import Any, Iterable, Mapping, Sequence


VERSION = 1
CONFIG_NAME = "FINAL_RUN_CONFIG.json"
CONFIG_SEAL_NAME = "FINAL_RUN_CONFIG.sha256"
INVENTORY_NAME = "SHA256SUMS"
QUARTET = (
    "task_investigation_bundle.json",
    "task_investigation_attempt.json",
    "task_investigation.json",
    "task_investigation_status.json",
)
REQUIRED_RUN_FILES = QUARTET + (
    "retrieval_trace.json",
    "retrieval_trace.md",
    "metadata.json",
)
PROJECTION_FILES = (
    "role_contract.json",
    "role_coverage.json",
    "verification_frontier.json",
    "cheap_exit_decision.json",
    "source_scopes.json",
)
PROVIDER_FIELDS = (
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
PROVIDER_ENV_PREFIXES = ("REPOMAP_LLM_", "DEEPSEEK_")
CHEAP_EXIT_GATES = {
    "unambiguous_area",
    "all_key_roles",
    "decisive_locally_observed_relation",
    "exact_verification_anchor_or_effect",
    "no_unresolved_competing_hypothesis",
}
CHEAP_EXIT_SUCCESS_REASON = "all deterministic cheap-exit gates passed"
FORBIDDEN_TRACE_KEYS = {
    "gold",
    "gold_assessment",
    "gold_loss_stage",
    "gold_candidate_id",
    "gold_anchor_id",
    "gold_loss_detail",
    "development_gold",
    "supervisor_gold",
}
EPISODE_ID_RE = re.compile(r"^[a-z0-9][a-z0-9_-]*$")
REVISION_RE = re.compile(r"^[0-9a-f]{40}$")
SHA256_RE = re.compile(r"^[0-9a-f]{64}$")
SECRET_RE = re.compile(
    rb"(?:(?<![A-Za-z0-9])sk-[A-Za-z0-9_-]{16,}|"
    rb"gh[pousr]_[A-Za-z0-9]{20,}|AKIA[0-9A-Z]{16})"
)
CANONICAL_EPISODES = (
    (
        "openapi_example_tag_parsing",
        "84fadc7a86fa097f240d5dafda3d86f7a784b3ec",
        "primary_regression",
        False,
    ),
    (
        "openapi_required_nullable_semantics",
        "c8a6e9dcd907314a613b2f8ff325ba3f01372774",
        "primary_regression",
        False,
    ),
    (
        "accept_header_wrong_status",
        "4bc28418acc1e8523276d0da2d6581fd98393106",
        "primary_regression",
        True,
    ),
    (
        "nil_body_validation_panic",
        "29023c565e324759e5a50e90583f4afdcdca11e4",
        "primary_regression",
        True,
    ),
    (
        "httperror_pointer_value",
        "fbf3dcfcaa69bdee8a17858a8f82656a4a8485d0",
        "primary_regression",
        False,
    ),
    (
        "multi_module_release_script",
        "62113a7cff6210c0db16ed51e003d26043398cf2",
        "primary_regression",
        False,
    ),
    (
        "openapi_disable_messages_config",
        "5f914dc7beb960bef84dede11960744a0a96a3c1",
        "cheap_exit_only",
        True,
    ),
)
CANONICAL_CONSTRAINTS = {
    "semantic_retries": 0,
    "maximum_model_calls_per_episode": 1,
    "maximum_total_fresh_model_calls": 8,
    "provider_model": "deepseek-v4-flash",
    "provider_endpoint": "https://api.deepseek.com/chat/completions",
    "target_repository_commands": 0,
}


class HarnessError(RuntimeError):
    """Raised when a final-run invariant is not satisfied."""


def fail(message: str) -> None:
    raise HarnessError(message)


def utc_now() -> str:
    return dt.datetime.now(dt.timezone.utc).replace(microsecond=0).isoformat()


def canonical_json(value: Any) -> bytes:
    return (
        json.dumps(value, indent=2, sort_keys=True, ensure_ascii=False) + "\n"
    ).encode("utf-8")


def sha256_bytes(raw: bytes) -> str:
    return hashlib.sha256(raw).hexdigest()


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    try:
        with path.open("rb") as source:
            for block in iter(lambda: source.read(1024 * 1024), b""):
                digest.update(block)
    except OSError as exc:
        fail(f"cannot hash {path}: {exc}")
    return digest.hexdigest()


def go_compact_json_sha256(value: Any) -> str:
    """Match encoding/json Marshal for an object decoded in Go field order."""
    raw = json.dumps(value, ensure_ascii=False, separators=(",", ":"))
    raw = raw.replace("&", "\\u0026").replace("<", "\\u003c").replace(">", "\\u003e")
    raw = raw.replace("\u2028", "\\u2028").replace("\u2029", "\\u2029")
    return sha256_bytes(raw.encode("utf-8"))


def _reject_duplicate_keys(pairs: Sequence[tuple[str, Any]]) -> dict[str, Any]:
    result: dict[str, Any] = {}
    for key, value in pairs:
        if key in result:
            fail(f"duplicate JSON key: {key}")
        result[key] = value
    return result


def read_json(path: Path) -> dict[str, Any]:
    try:
        value = json.loads(
            path.read_text(encoding="utf-8"),
            object_pairs_hook=_reject_duplicate_keys,
        )
    except (OSError, UnicodeError, json.JSONDecodeError) as exc:
        fail(f"cannot read JSON {path}: {exc}")
    if not isinstance(value, dict):
        fail(f"expected a JSON object in {path}")
    return value


def write_new(path: Path, raw: bytes, mode: int = 0o644) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    try:
        descriptor = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, mode)
    except FileExistsError:
        fail(f"refusing to overwrite existing artifact: {path}")
    try:
        with os.fdopen(descriptor, "wb") as output:
            output.write(raw)
            output.flush()
            os.fsync(output.fileno())
    except BaseException:
        try:
            path.unlink()
        except FileNotFoundError:
            pass
        raise


def write_json_new(path: Path, value: Any, mode: int = 0o644) -> None:
    write_new(path, canonical_json(value), mode)


def copy_new(source: Path, destination: Path, mode: int) -> None:
    destination.parent.mkdir(parents=True, exist_ok=True)
    try:
        descriptor = os.open(destination, os.O_WRONLY | os.O_CREAT | os.O_EXCL, mode)
    except FileExistsError:
        fail(f"refusing to overwrite existing input: {destination}")
    try:
        with source.open("rb") as input_file, os.fdopen(descriptor, "wb") as output:
            shutil.copyfileobj(input_file, output)
            output.flush()
            os.fsync(output.fileno())
        os.chmod(destination, mode)
    except BaseException:
        try:
            destination.unlink()
        except FileNotFoundError:
            pass
        raise


def sanitized_env() -> dict[str, str]:
    return {key: value for key, value in os.environ.items() if not key.startswith("GIT_")}


def final_attempt_env() -> dict[str, str]:
    """Remove provider configuration so a stale zero-call route cannot spend a call."""
    return {
        key: value
        for key, value in sanitized_env().items()
        if not key.startswith(PROVIDER_ENV_PREFIXES)
    }


def run_command(
    argv: Sequence[str],
    *,
    cwd: Path | None = None,
    check: bool = True,
) -> subprocess.CompletedProcess[str]:
    result = subprocess.run(
        list(argv),
        cwd=str(cwd) if cwd else None,
        env=sanitized_env(),
        check=False,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )
    if check and result.returncode != 0:
        fail(
            f"command failed ({result.returncode}): {' '.join(argv)}: "
            f"{result.stderr.strip()}"
        )
    return result


def git(repo: Path, *args: str, check: bool = True) -> subprocess.CompletedProcess[str]:
    return run_command(("git", "-C", str(repo), *args), check=check)


def git_output(repo: Path, *args: str) -> str:
    return git(repo, *args).stdout.strip()


def resolve_file(value: str | Path, label: str) -> Path:
    unresolved = Path(value).expanduser()
    if unresolved.is_symlink():
        fail(f"{label} must not be a symlink: {unresolved}")
    path = unresolved.resolve()
    if not path.is_file():
        fail(f"{label} must be a regular file: {path}")
    return path


def resolve_dir(value: str | Path, label: str) -> Path:
    unresolved = Path(value).expanduser()
    if unresolved.is_symlink():
        fail(f"{label} must not be a symlink: {unresolved}")
    path = unresolved.resolve()
    if not path.is_dir():
        fail(f"{label} must be a directory: {path}")
    return path


def safe_relative_path(raw: str, label: str) -> Path:
    path = Path(raw)
    if (
        not raw
        or "\x00" in raw
        or "\r" in raw
        or "\n" in raw
        or path.is_absolute()
        or any(part in {"", ".", ".."} for part in path.parts)
    ):
        fail(f"{label} is not a safe relative path: {raw!r}")
    return path


def resolve_under(root: Path, relative: Path, kind: str, label: str) -> Path:
    root = root.resolve()
    current = root
    for part in relative.parts:
        current = current / part
        if current.is_symlink():
            fail(f"{label} contains a symlink component: {current}")
    try:
        current.resolve(strict=True).relative_to(root)
    except (OSError, ValueError) as exc:
        fail(f"{label} is not a contained existing path: {current}: {exc}")
    if kind == "file" and not current.is_file():
        fail(f"{label} must be a regular file: {current}")
    if kind == "dir" and not current.is_dir():
        fail(f"{label} must be a directory: {current}")
    return current


def prompt_safe_task_text(raw: str) -> str:
    text = raw.replace("\r\n", "\n").strip()
    heading = "## Prompt-safe task"
    start = text.find(heading)
    if start >= 0:
        body = text[start + len(heading) :]
        next_heading = body.find("\n## ")
        if next_heading >= 0:
            body = body[:next_heading]
        text = body.strip()
    if not text:
        fail("task source has no prompt-safe task text")
    if len(text.encode("utf-8")) > 32 * 1024 or "\x00" in text:
        fail("task source is outside the bounded task size")
    return text + "\n"


def load_manifest(path: Path) -> dict[str, Any]:
    manifest = read_json(path)
    raw_episodes = manifest.get("episodes")
    if not isinstance(raw_episodes, list) or not raw_episodes:
        fail("development manifest must contain episodes")
    canonical = manifest.get("kind") == "task_lens_v01_known_development"
    if canonical:
        if (
            manifest.get("version") != VERSION
            or manifest.get("fresh_generalization") is not False
            or manifest.get("primary_episode_count") != 6
            or manifest.get("cheap_exit_calibration_count") != 1
            or manifest.get("constraints") != CANONICAL_CONSTRAINTS
            or len(raw_episodes) != len(CANONICAL_EPISODES)
        ):
            fail("the canonical Task Lens v0.1 development manifest header is invalid")
    seen: set[str] = set()
    episodes: list[dict[str, Any]] = []
    for index, raw in enumerate(raw_episodes):
        if not isinstance(raw, dict):
            fail(f"manifest episode {index} is not an object")
        episode_id = raw.get("episode_id")
        revision = raw.get("base_revision")
        cheap = raw.get("cheap_exit_target")
        if not isinstance(episode_id, str) or not EPISODE_ID_RE.fullmatch(episode_id):
            fail(f"manifest episode {index} has an invalid episode_id")
        if episode_id in seen:
            fail(f"duplicate manifest episode: {episode_id}")
        seen.add(episode_id)
        if not isinstance(revision, str) or not REVISION_RE.fullmatch(revision):
            fail(f"manifest episode {episode_id} has an invalid base_revision")
        if not isinstance(cheap, bool):
            fail(f"manifest episode {episode_id} has no cheap_exit_target boolean")
        expected_attempt = f"episodes/{episode_id}/final/attempt"
        if raw.get("final_attempt", expected_attempt) != expected_attempt:
            fail(f"manifest episode {episode_id} has a non-canonical final_attempt")
        episodes.append(dict(raw))
    if canonical:
        observed = [
            (
                item.get("episode_id"),
                item.get("base_revision"),
                item.get("evaluation_scope"),
                item.get("cheap_exit_target"),
            )
            for item in episodes
        ]
        if observed != list(CANONICAL_EPISODES):
            fail("canonical Task Lens v0.1 episode IDs, revisions, scopes, or modes changed")
    result = dict(manifest)
    result["episodes"] = episodes
    return result


def find_task_source(tasks_root: Path, episode_id: str) -> Path:
    candidates = (
        tasks_root / f"{episode_id}.md",
        tasks_root / episode_id / "task.md",
        tasks_root / "holdout" / f"{episode_id}.md",
        tasks_root / "dev" / f"{episode_id}.md",
        tasks_root / "holdout" / episode_id / "task.md",
        tasks_root / "dev" / episode_id / "task.md",
    )
    found = [path for path in candidates if path.is_file() and not path.is_symlink()]
    if len(found) != 1:
        rendered = ", ".join(str(path) for path in candidates)
        fail(f"episode {episode_id}: expected exactly one task source among {rendered}")
    return found[0]


def frozen_route_plan(
    manifest: Mapping[str, Any],
    zero_call_episode_ids: Iterable[str],
) -> dict[str, Any]:
    manifest_ids = [item["episode_id"] for item in manifest["episodes"]]
    if isinstance(zero_call_episode_ids, (str, bytes)):
        fail("zero-call episode selection must be a sequence of episode IDs")
    requested = list(zero_call_episode_ids)
    if any(not isinstance(item, str) or not item for item in requested):
        fail("zero-call episode selection contains an invalid ID")
    if len(requested) != len(set(requested)):
        fail("zero-call episode selection contains duplicate IDs")
    unknown = sorted(set(requested) - set(manifest_ids))
    if unknown:
        fail("zero-call episode selection contains unknown IDs: " + ", ".join(unknown))
    selected_set = set(requested)
    selected = [episode_id for episode_id in manifest_ids if episode_id in selected_set]
    selected_set = set(selected)
    return {
        "version": VERSION,
        "kind": "task_lens_v01_frozen_route_plan",
        "selection_source": "explicit_post_calibration",
        "zero_call_episode_ids": selected,
        "offline_partial_episode_ids": [
            episode_id for episode_id in manifest_ids if episode_id not in selected_set
        ],
    }


def validate_frozen_route_plan(
    config: Mapping[str, Any],
    manifest: Mapping[str, Any],
) -> tuple[dict[str, Any], set[str]]:
    route_plan = config.get("route_plan")
    if not isinstance(route_plan, dict) or set(route_plan) != {
        "version",
        "kind",
        "selection_source",
        "zero_call_episode_ids",
        "offline_partial_episode_ids",
    }:
        fail("final-run config has no canonical frozen route plan")
    if (
        route_plan.get("version") != VERSION
        or route_plan.get("kind") != "task_lens_v01_frozen_route_plan"
        or route_plan.get("selection_source") != "explicit_post_calibration"
    ):
        fail("frozen route plan header is invalid")
    zero_ids = route_plan.get("zero_call_episode_ids")
    offline_ids = route_plan.get("offline_partial_episode_ids")
    if (
        not isinstance(zero_ids, list)
        or not isinstance(offline_ids, list)
        or any(not isinstance(item, str) for item in zero_ids + offline_ids)
        or len(zero_ids) != len(set(zero_ids))
        or len(offline_ids) != len(set(offline_ids))
        or set(zero_ids) & set(offline_ids)
    ):
        fail("frozen route plan contains invalid or duplicate episode IDs")
    manifest_ids = [item["episode_id"] for item in manifest["episodes"]]
    zero_set = set(zero_ids)
    expected_zero = [episode_id for episode_id in manifest_ids if episode_id in zero_set]
    expected_offline = [episode_id for episode_id in manifest_ids if episode_id not in zero_set]
    if zero_ids != expected_zero or offline_ids != expected_offline:
        fail("frozen route plan is not an ordered exact partition of the manifest")
    route_hash = config.get("route_plan_sha256")
    if (
        not isinstance(route_hash, str)
        or SHA256_RE.fullmatch(route_hash) is None
        or route_hash != sha256_bytes(canonical_json(route_plan))
    ):
        fail("frozen route plan hash does not match FINAL_RUN_CONFIG.json")
    return route_plan, zero_set


def ensure_git_repository(repo: Path) -> None:
    if git_output(repo, "rev-parse", "--is-inside-work-tree") != "true":
        fail(f"not a Git worktree: {repo}")


def verify_worktree(repo: Path, revision: str, tree_hash: str) -> None:
    if repo.name != "repo":
        fail(f"episode worktree basename must be neutral 'repo': {repo}")
    ensure_git_repository(repo)
    if git_output(repo, "rev-parse", "HEAD") != revision:
        fail(f"episode worktree revision changed: {repo}")
    if git_output(repo, "rev-parse", "HEAD^{tree}") != tree_hash:
        fail(f"episode worktree tree changed: {repo}")
    symbolic = git(repo, "symbolic-ref", "-q", "HEAD", check=False)
    if symbolic.returncode != 1:
        fail(f"episode worktree must remain at detached HEAD: {repo}")
    if git_output(repo, "status", "--porcelain", "--untracked-files=all"):
        fail(f"episode worktree is not clean: {repo}")


def prepare_experiment(
    root: Path,
    manifest_path: Path,
    source_repo: Path,
    tasks_root: Path,
    binary: Path,
    zero_call_episode_ids: Iterable[str],
) -> dict[str, Any]:
    root = root.expanduser().resolve()
    manifest_path = resolve_file(manifest_path, "development manifest")
    source_repo = resolve_dir(source_repo, "source repository")
    tasks_root = resolve_dir(tasks_root, "task source root")
    binary = resolve_file(binary, "candidate binary")
    if not os.access(binary, os.X_OK):
        fail(f"candidate binary is not executable: {binary}")
    ensure_git_repository(source_repo)
    manifest = load_manifest(manifest_path)
    route_plan = frozen_route_plan(manifest, zero_call_episode_ids)
    zero_call_ids = set(route_plan["zero_call_episode_ids"])

    config_path = root / CONFIG_NAME
    config_seal = root / CONFIG_SEAL_NAME
    frozen_binary = root / "inputs" / "repomap"
    frozen_manifest = root / "inputs" / "DEVELOPMENT_SET.json"
    for target in (config_path, config_seal, frozen_binary, frozen_manifest):
        if target.exists() or target.is_symlink():
            fail(f"prepared final-run input already exists: {target}")

    planned: list[dict[str, Any]] = []
    for raw in manifest["episodes"]:
        episode_id = raw["episode_id"]
        episode_dir = root / "episodes" / episode_id
        if episode_dir.exists() or episode_dir.is_symlink():
            fail(f"episode staging directory already exists: {episode_dir}")
        resolved_revision = git_output(
            source_repo,
            "rev-parse",
            "--verify",
            f"{raw['base_revision']}^{{commit}}",
        )
        if resolved_revision != raw["base_revision"]:
            fail(f"episode {episode_id}: base revision did not resolve exactly")
        tree_hash = git_output(source_repo, "rev-parse", f"{resolved_revision}^{{tree}}")
        task_source = find_task_source(tasks_root, episode_id)
        task_text = prompt_safe_task_text(task_source.read_text(encoding="utf-8"))
        planned.append(
            {
                "episode_id": episode_id,
                "base_revision": resolved_revision,
                "tree_hash": tree_hash,
                "cheap_exit_target": raw["cheap_exit_target"],
                "run_mode": "zero_call" if episode_id in zero_call_ids else "offline_partial",
                "task_text": task_text,
            }
        )

    root.mkdir(parents=True, exist_ok=True)
    copy_new(binary, frozen_binary, 0o555)
    copy_new(manifest_path, frozen_manifest, 0o444)
    for item in planned:
        episode_dir = root / "episodes" / item["episode_id"]
        worktree = episode_dir / "worktree" / "repo"
        worktree.parent.mkdir(parents=True, exist_ok=False)
        git(source_repo, "worktree", "add", "--detach", str(worktree), item["base_revision"])
        verify_worktree(worktree, item["base_revision"], item["tree_hash"])
        task_path = episode_dir / "task.md"
        write_new(task_path, item["task_text"].encode("utf-8"), 0o444)
        item["task_sha256"] = sha256_file(task_path)
        item["task_path"] = f"episodes/{item['episode_id']}/task.md"
        item["repository_path"] = f"episodes/{item['episode_id']}/worktree/repo"
        del item["task_text"]

    config = {
        "version": VERSION,
        "kind": "task_lens_v01_final_run_config",
        "known_development_only": True,
        "semantic_retries": 0,
        "maximum_process_invocations_per_episode": 1,
        "target_repository_commands_executed": 0,
        "provider_environment_disabled": True,
        "route_plan": route_plan,
        "route_plan_sha256": sha256_bytes(canonical_json(route_plan)),
        "source_repository": str(source_repo),
        "manifest_path": "inputs/DEVELOPMENT_SET.json",
        "manifest_sha256": sha256_file(frozen_manifest),
        "binary_path": "inputs/repomap",
        "binary_sha256": sha256_file(frozen_binary),
        "episodes": planned,
        "prepared_at": utc_now(),
    }
    write_json_new(config_path, config, 0o444)
    write_new(
        config_seal,
        f"{sha256_file(config_path)}  {CONFIG_NAME}\n".encode("ascii"),
        0o444,
    )
    return config


def _verify_sidecar(path: Path, sidecar: Path) -> None:
    try:
        fields = sidecar.read_text(encoding="ascii").split()
    except (OSError, UnicodeError) as exc:
        fail(f"cannot read config seal {sidecar}: {exc}")
    if len(fields) != 2 or fields[0] != sha256_file(path) or fields[1] != path.name:
        fail(f"prepared config seal does not match {path}")


def load_config(root: Path) -> tuple[dict[str, Any], dict[str, dict[str, Any]]]:
    root = root.expanduser().resolve()
    config_path = resolve_under(root, Path(CONFIG_NAME), "file", "final-run config")
    sidecar = resolve_under(
        root,
        Path(CONFIG_SEAL_NAME),
        "file",
        "final-run config seal",
    )
    _verify_sidecar(config_path, sidecar)
    config = read_json(config_path)
    if (
        config.get("version") != VERSION
        or config.get("kind") != "task_lens_v01_final_run_config"
        or config.get("known_development_only") is not True
        or config.get("semantic_retries") != 0
        or config.get("maximum_process_invocations_per_episode") != 1
        or config.get("target_repository_commands_executed") != 0
        or config.get("provider_environment_disabled") is not True
    ):
        fail("final-run config header is invalid")
    binary_relative = safe_relative_path(str(config.get("binary_path", "")), "binary_path")
    manifest_relative = safe_relative_path(str(config.get("manifest_path", "")), "manifest_path")
    if binary_relative.as_posix() != "inputs/repomap":
        fail("frozen candidate binary must use canonical inputs/repomap path")
    if manifest_relative.as_posix() != "inputs/DEVELOPMENT_SET.json":
        fail("frozen manifest must use canonical inputs/DEVELOPMENT_SET.json path")
    binary = resolve_under(root, binary_relative, "file", "frozen candidate binary")
    manifest_path = resolve_under(
        root,
        manifest_relative,
        "file",
        "frozen development manifest",
    )
    if sha256_file(binary) != config.get("binary_sha256"):
        fail("frozen candidate binary changed after preparation")
    if sha256_file(manifest_path) != config.get("manifest_sha256"):
        fail("frozen development manifest changed after preparation")
    manifest = load_manifest(manifest_path)
    _, zero_call_ids = validate_frozen_route_plan(config, manifest)
    manifest_by_id = {item["episode_id"]: item for item in manifest["episodes"]}

    raw_episodes = config.get("episodes")
    if not isinstance(raw_episodes, list):
        fail("final-run config episodes are missing")
    episodes: dict[str, dict[str, Any]] = {}
    for raw in raw_episodes:
        if not isinstance(raw, dict) or not isinstance(raw.get("episode_id"), str):
            fail("final-run config contains an invalid episode")
        episode_id = raw["episode_id"]
        if episode_id in episodes or episode_id not in manifest_by_id:
            fail(f"final-run config episode set is invalid: {episode_id}")
        manifest_item = manifest_by_id[episode_id]
        expected_mode = "zero_call" if episode_id in zero_call_ids else "offline_partial"
        if (
            raw.get("base_revision") != manifest_item["base_revision"]
            or raw.get("cheap_exit_target") != manifest_item["cheap_exit_target"]
            or raw.get("run_mode") != expected_mode
            or raw.get("task_path") != f"episodes/{episode_id}/task.md"
            or raw.get("repository_path")
            != f"episodes/{episode_id}/worktree/repo"
            or not isinstance(raw.get("tree_hash"), str)
            or not REVISION_RE.fullmatch(raw["tree_hash"])
            or not isinstance(raw.get("task_sha256"), str)
            or not SHA256_RE.fullmatch(raw["task_sha256"])
        ):
            fail(f"final-run config binding is invalid for {episode_id}")
        episodes[episode_id] = raw
    if set(episodes) != set(manifest_by_id):
        fail("final-run config and manifest episode sets differ")
    return config, episodes


def verify_prepared_episode(root: Path, episode: Mapping[str, Any]) -> tuple[Path, Path]:
    task_relative = safe_relative_path(str(episode.get("task_path", "")), "task_path")
    repo_relative = safe_relative_path(
        str(episode.get("repository_path", "")),
        "repository_path",
    )
    task_path = resolve_under(root, task_relative, "file", "canonical episode task")
    repo = resolve_under(root, repo_relative, "dir", "episode worktree")
    if sha256_file(task_path) != episode.get("task_sha256"):
        fail(f"episode task changed after preparation: {task_path}")
    verify_worktree(repo, str(episode["base_revision"]), str(episode["tree_hash"]))
    return task_path, repo


def canonical_command(offline: bool) -> list[str]:
    command = [
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
    ]
    if offline:
        command.append("--offline")
    return command


def _invoke_cli(
    argv: Sequence[str],
    attempt_dir: Path,
    timeout_seconds: int,
) -> tuple[int, bool, str | None]:
    timed_out = False
    launch_error: str | None = None
    return_code = -127
    with (attempt_dir / "stdout.txt").open("xb") as stdout, (
        attempt_dir / "stderr.txt"
    ).open("xb") as stderr:
        try:
            result = subprocess.run(
                list(argv),
                cwd=str(attempt_dir),
                env=final_attempt_env(),
                check=False,
                stdout=stdout,
                stderr=stderr,
                timeout=timeout_seconds,
            )
            return_code = result.returncode
        except subprocess.TimeoutExpired:
            timed_out = True
            return_code = -124
            launch_error = f"process exceeded {timeout_seconds} seconds"
        except OSError as exc:
            launch_error = f"cannot launch candidate binary: {exc}"
    return return_code, timed_out, launch_error


def _require_object(value: Any, label: str) -> dict[str, Any]:
    if not isinstance(value, dict):
        fail(f"{label} must be a JSON object")
    return value


def _provider_metrics(*documents: Mapping[str, Any]) -> dict[str, int]:
    found: list[dict[str, int]] = []
    for index, document in enumerate(documents):
        provider = document.get("provider")
        if not isinstance(provider, dict):
            fail(f"provider accounting is missing from artifact {index + 1}")
        normalized: dict[str, int] = {}
        for field in PROVIDER_FIELDS:
            value = provider.get(field)
            if isinstance(value, bool) or not isinstance(value, int) or value < 0:
                fail(f"provider.{field} must be a non-negative integer")
            normalized[field] = value
        found.append(normalized)
    if any(value != found[0] for value in found[1:]):
        fail("provider accounting differs across saved artifacts")
    return found[0]


def _discover_run_dir(attempt_dir: Path) -> tuple[Path, str]:
    run_root = attempt_dir / "run"
    if not run_root.is_dir() or run_root.is_symlink():
        fail("attempt run root must be a regular directory")
    latest = run_root / "latest"
    if not latest.is_symlink():
        fail("run/latest must be a relative symlink")
    target = os.readlink(latest)
    target_path = Path(target)
    if target_path.is_absolute() or len(target_path.parts) != 1 or target_path.parts[0] in {"", ".", ".."}:
        fail(f"run/latest has an unsafe target: {target!r}")
    run_dir = run_root / target_path
    if not run_dir.is_dir() or run_dir.is_symlink():
        fail("run/latest does not select a regular run directory")
    candidates = {
        path.parent.resolve()
        for name in QUARTET
        for path in run_root.rglob(name)
        if path.is_file() and not path.is_symlink()
    }
    if candidates != {run_dir.resolve()}:
        fail("attempt must contain exactly one canonical artifact run")
    for name in REQUIRED_RUN_FILES:
        path = run_dir / name
        if not path.is_file() or path.is_symlink():
            fail(f"canonical run is missing regular artifact {name}")
    return run_dir, target


def _assert_trace_has_no_gold(value: Any, location: str = "trace") -> None:
    if isinstance(value, dict):
        for key, nested in value.items():
            if key.lower() in FORBIDDEN_TRACE_KEYS:
                fail(f"raw retrieval trace contains evaluation-only key {location}.{key}")
            _assert_trace_has_no_gold(nested, f"{location}.{key}")
    elif isinstance(value, list):
        for index, nested in enumerate(value):
            _assert_trace_has_no_gold(nested, f"{location}[{index}]")


def _cheap_exit_projection(
    bundle: Mapping[str, Any],
    pack: Mapping[str, Any],
    status: Mapping[str, Any],
) -> dict[str, Any]:
    value = _require_object(bundle.get("cheap_exit"), "bundle cheap_exit")
    if pack.get("cheap_exit") != value or status.get("cheap_exit") != value:
        fail("cheap-exit projections disagree across bundle, pack, and status")
    if not isinstance(value.get("eligible"), bool):
        fail("cheap_exit.eligible must be boolean")
    gates = value.get("gates")
    if not isinstance(gates, list) or len(gates) != len(CHEAP_EXIT_GATES):
        fail("cheap_exit.gates must contain exactly five entries")
    names: set[str] = set()
    for gate in gates:
        if (
            not isinstance(gate, dict)
            or not isinstance(gate.get("gate"), str)
            or not isinstance(gate.get("passed"), bool)
            or not isinstance(gate.get("reason"), str)
            or not gate["reason"].strip()
            or gate["gate"] in names
        ):
            fail("cheap_exit.gates contains a malformed or duplicate gate")
        names.add(gate["gate"])
    if names != CHEAP_EXIT_GATES:
        fail("cheap-exit decision does not contain the five deterministic gates")
    reasons = value.get("reasons")
    if not isinstance(reasons, list) or any(
        not isinstance(reason, str) or not reason.strip() for reason in reasons
    ):
        fail("cheap_exit.reasons must contain only non-empty strings")
    if value["eligible"] != all(gate["passed"] for gate in gates):
        fail("cheap_exit eligibility does not equal the five deterministic gates")
    if value["eligible"] and reasons != [CHEAP_EXIT_SUCCESS_REASON]:
        fail("eligible cheap-exit decision must record the deterministic success reason")
    if not value["eligible"] and not reasons:
        fail("ineligible cheap-exit decision must record exact failure reasons")
    return value


def _source_scope_projection(
    bundle: Mapping[str, Any],
    trace: Mapping[str, Any],
) -> list[dict[str, Any]]:
    anchors = bundle.get("anchors")
    scopes = trace.get("source_scopes")
    if not isinstance(anchors, list) or not isinstance(scopes, list):
        fail("bundle anchors or retrieval trace source_scopes are missing")
    expected: dict[str, Any] = {}
    for anchor in anchors:
        if not isinstance(anchor, dict) or not isinstance(anchor.get("id"), str):
            fail("bundle contains an invalid anchor")
        if anchor["id"] in expected:
            fail("bundle contains duplicate anchor IDs")
        scope = anchor.get("source_scope")
        if not isinstance(scope, dict):
            fail("bundle anchor has no source_scope")
        expected[anchor["id"]] = scope
    observed: dict[str, Any] = {}
    for item in scopes:
        if not isinstance(item, dict):
            fail("retrieval trace contains an invalid source scope")
        anchor_id = item.get("anchor_id")
        scope = item.get("scope")
        if not isinstance(anchor_id, str) or not isinstance(scope, dict) or anchor_id in observed:
            fail("retrieval trace source scope is not uniquely bound to an anchor")
        observed[anchor_id] = scope
    if observed != expected:
        fail("retrieval trace source scopes do not exactly match bundle anchors")
    return [dict(item) for item in scopes]


def validate_run_artifacts(
    attempt_dir: Path,
    episode: Mapping[str, Any],
    task_path: Path,
    offline: bool,
) -> dict[str, Any]:
    run_dir, latest_target = _discover_run_dir(attempt_dir)
    documents = {name: read_json(run_dir / name) for name in QUARTET}
    bundle = documents[QUARTET[0]]
    attempt = documents[QUARTET[1]]
    pack = documents[QUARTET[2]]
    status = documents[QUARTET[3]]
    trace = read_json(run_dir / "retrieval_trace.json")
    metadata = read_json(run_dir / "metadata.json")
    _assert_trace_has_no_gold(trace)
    try:
        trace_markdown = (run_dir / "retrieval_trace.md").read_text(encoding="utf-8")
    except (OSError, UnicodeError) as exc:
        fail(f"cannot read retrieval trace markdown: {exc}")
    for line in trace_markdown.splitlines():
        lowered = line.strip().lower()
        if "gold assessment:" in lowered and lowered != "- gold assessment: not applied":
            fail("raw retrieval trace markdown contains an applied gold assessment")
        if any(key in lowered for key in FORBIDDEN_TRACE_KEYS - {"gold"}):
            fail("raw retrieval trace markdown contains evaluation-only gold metadata")

    bundle_sha = go_compact_json_sha256(bundle)
    for label, document in (("attempt", attempt), ("pack", pack), ("status", status)):
        if document.get("bundle_sha256") != bundle_sha:
            fail(f"{label} does not bind the canonical bundle hash")
    if status.get("attempt_sha256") != sha256_file(run_dir / QUARTET[1]):
        fail("status attempt hash does not match artifact bytes")
    if status.get("pack_sha256") != sha256_file(run_dir / QUARTET[2]):
        fail("status pack hash does not match artifact bytes")
    if status.get("retrieval_trace_sha256") != sha256_file(run_dir / "retrieval_trace.json"):
        fail("status retrieval trace hash does not match artifact bytes")
    if status.get("retrieval_trace_markdown_sha256") != sha256_file(
        run_dir / "retrieval_trace.md"
    ):
        fail("status retrieval trace markdown hash does not match artifact bytes")

    repository = _require_object(bundle.get("repository"), "bundle repository")
    if (
        repository.get("revision") != episode["base_revision"]
        or repository.get("tree_hash") != episode["tree_hash"]
        or status.get("captured_revision") != episode["base_revision"]
        or status.get("tree_hash") != episode["tree_hash"]
    ):
        fail("saved artifacts do not bind the prepared repository revision/tree")
    pack_repository = _require_object(pack.get("repository"), "pack repository")
    if (
        pack_repository.get("revision") != episode["base_revision"]
        or pack_repository.get("tree_hash") != episode["tree_hash"]
        or pack_repository != repository
    ):
        fail("pack repository identity differs from the canonical bundle/prepared episode")
    task = _require_object(bundle.get("task"), "bundle task")
    task_text = task.get("text")
    if not isinstance(task_text, str) or task_text.strip() != task_path.read_text(
        encoding="utf-8"
    ).strip():
        fail("bundle task text differs from canonical episodes/<id>/task.md")
    task_id = bundle.get("id")
    if (
        not isinstance(task_id, str)
        or not task_id
        or pack.get("id") != task_id
        or status.get("task_id") != task_id
    ):
        fail("bundle, pack, and status task IDs disagree")

    provider = _provider_metrics(attempt, status)
    if any(provider.values()):
        fail("final v0.1 harness accepts only zero-provider-call outputs")
    provider_request_count = metadata.get("provider_request_count")
    if provider_request_count is not None and (
        isinstance(provider_request_count, bool)
        or not isinstance(provider_request_count, int)
        or provider_request_count != 0
    ):
        fail("run metadata reports a provider request for a zero-call final output")
    effective_options = _require_object(
        metadata.get("effective_options"),
        "run metadata effective_options",
    )
    if (
        metadata.get("run_id") != latest_target
        or metadata.get("command") != "investigate"
        or metadata.get("model") != CANONICAL_CONSTRAINTS["provider_model"]
        or metadata.get("endpoint") != CANONICAL_CONSTRAINTS["provider_endpoint"]
        or effective_options.get("offline") is not offline
        or effective_options.get("no_open") is not True
        or effective_options.get("no_serve") is not True
        or effective_options.get("debug_enabled") is not True
    ):
        fail("run metadata does not bind the canonical one-process CLI invocation")

    cheap_exit = _cheap_exit_projection(bundle, pack, status)
    role_contract = _require_object(bundle.get("role_contract"), "bundle role_contract")
    role_coverage = _require_object(bundle.get("role_coverage"), "bundle role_coverage")
    verification = _require_object(
        bundle.get("verification_frontier"),
        "bundle verification_frontier",
    )
    if (
        trace.get("role_coverage") != role_coverage
        or pack.get("role_contract") != role_contract
        or pack.get("role_coverage") != role_coverage
        or trace.get("verification_frontier") != verification
        or pack.get("verification_frontier") != verification
    ):
        fail("role or verification projections disagree across saved artifacts")
    source_scopes = _source_scope_projection(bundle, trace)

    if offline:
        if episode.get("run_mode") != "offline_partial":
            fail("offline launch is not allowed for a planned zero-call episode")
        if (
            cheap_exit.get("eligible") is not False
            or cheap_exit.get("route") != "single_synthesis_call"
            or attempt.get("state") != "skipped_offline"
            or status.get("state") != "partial_local"
            or status.get("sufficient") is not False
        ):
            fail("offline non-cheap result is not an honest insufficient local partial")
    else:
        if episode.get("run_mode") != "zero_call":
            fail("non-offline launch is only allowed for a planned zero-call episode")
        if (
            cheap_exit.get("eligible") is not True
            or cheap_exit.get("route") != "zero_call"
            or attempt.get("state") != "skipped_local_complete"
            or status.get("state") != "accepted_local_complete"
            or status.get("sufficient") is not True
        ):
            fail("cheap-exit target did not produce a genuine sufficient zero-call result")
        if any(gate.get("passed") is not True for gate in cheap_exit["gates"]):
            fail("genuine zero-call result contains a failing cheap-exit gate")

    return {
        "run_dir": run_dir,
        "latest_target": latest_target,
        "bundle": bundle,
        "attempt": attempt,
        "pack": pack,
        "status": status,
        "trace": trace,
        "provider": provider,
        "role_contract": role_contract,
        "role_coverage": role_coverage,
        "verification_frontier": verification,
        "cheap_exit": cheap_exit,
        "source_scopes": source_scopes,
    }


def _write_projections(attempt_dir: Path, episode_id: str, summary: Mapping[str, Any]) -> None:
    write_json_new(attempt_dir / "role_contract.json", summary["role_contract"])
    write_json_new(attempt_dir / "role_coverage.json", summary["role_coverage"])
    write_json_new(
        attempt_dir / "verification_frontier.json",
        summary["verification_frontier"],
    )
    write_json_new(attempt_dir / "cheap_exit_decision.json", summary["cheap_exit"])
    write_json_new(
        attempt_dir / "source_scopes.json",
        {
            "version": VERSION,
            "episode_id": episode_id,
            "source_scopes": summary["source_scopes"],
        },
    )


def validate_harness_attempt(
    config: Mapping[str, Any],
    episode: Mapping[str, Any],
    attempt_dir: Path,
) -> dict[str, Any]:
    started_path = attempt_dir / "ATTEMPT_STARTED.json"
    harness_path = attempt_dir / "HARNESS_ATTEMPT.json"
    started = read_json(started_path)
    harness = read_json(harness_path)
    offline = episode.get("run_mode") == "offline_partial"
    expected = {
        "version": VERSION,
        "phase": "development_final",
        "final": True,
        "episode_id": episode.get("episode_id"),
        "run_mode": episode.get("run_mode"),
        "offline": offline,
        "one_process_invocation": True,
        "semantic_retry": False,
        "provider_environment_disabled": True,
        "base_revision": episode.get("base_revision"),
        "tree_hash": episode.get("tree_hash"),
        "task_sha256": episode.get("task_sha256"),
        "binary_sha256": config.get("binary_sha256"),
        "manifest_sha256": config.get("manifest_sha256"),
        "route_plan_sha256": config.get("route_plan_sha256"),
        "command": canonical_command(offline),
    }
    for field, value in expected.items():
        if started.get(field) != value or harness.get(field) != value:
            fail(f"harness attempt {field} is not bound to the frozen launch plan")
    nonce = started.get("launch_nonce")
    if not isinstance(nonce, str) or re.fullmatch(r"[0-9a-f]{32}", nonce) is None:
        fail("harness attempt launch nonce is invalid")
    if harness.get("launch_nonce") != nonce:
        fail("harness attempt launch nonce changed after process execution")
    if set(started) != set(expected) | {"launch_nonce", "started_at"}:
        fail("pre-launch attempt record contains unexpected or missing fields")
    if (
        harness.get("started_record_sha256") != sha256_file(started_path)
        or harness.get("return_code") != 0
        or harness.get("timed_out") is not False
        or "launch_error" in harness
    ):
        fail("only a successful, non-timeout final process may be sealed")
    wall_millis = harness.get("wall_millis")
    if isinstance(wall_millis, bool) or not isinstance(wall_millis, int) or wall_millis < 0:
        fail("harness attempt wall_millis must be a non-negative integer")
    for field in ("started_at", "finished_at"):
        value = harness.get(field)
        if not isinstance(value, str) or not value.strip():
            fail(f"harness attempt {field} is missing")
    if harness.get("started_at") != started.get("started_at"):
        fail("harness attempt start time changed after process execution")
    if harness.get("target_repository_commands_executed") != 0:
        fail("harness attempt reports target repository command execution")
    allowed_harness_fields = set(started) | {
        "started_record_sha256",
        "return_code",
        "timed_out",
        "wall_millis",
        "finished_at",
        "target_repository_commands_executed",
    }
    if set(harness) != allowed_harness_fields:
        fail("completed harness attempt contains unexpected or missing fields")
    return harness


def inventory_files(
    root: Path,
    *,
    excluded: Iterable[str] = (),
    allowed_symlinks: Mapping[str, str] | None = None,
) -> list[dict[str, Any]]:
    excluded_set = set(excluded)
    allowed_symlinks = allowed_symlinks or {}
    items: list[dict[str, Any]] = []
    for path in sorted(root.rglob("*")):
        relative = path.relative_to(root).as_posix()
        safe_relative_path(relative, "final attempt inventory path")
        if relative in excluded_set:
            continue
        if path.is_symlink():
            target = os.readlink(path)
            if allowed_symlinks.get(relative) != target:
                fail(f"final attempt contains an unexpected symlink: {relative} -> {target}")
            raw = target.encode("utf-8", "surrogateescape")
            items.append(
                {
                    "path": relative,
                    "kind": "symlink",
                    "target": target,
                    "bytes": len(raw),
                    "sha256": sha256_bytes(raw),
                }
            )
        elif path.is_file():
            items.append(
                {
                    "path": relative,
                    "kind": "file",
                    "bytes": path.stat().st_size,
                    "sha256": sha256_file(path),
                }
            )
        elif not path.is_dir():
            fail(f"final attempt contains an unsupported filesystem entry: {relative}")
    return items


def _write_sha256_inventory(
    attempt_dir: Path,
    latest_target: str,
) -> tuple[Path, int]:
    items = inventory_files(
        attempt_dir,
        excluded={"SEALED.json", INVENTORY_NAME},
        allowed_symlinks={"run/latest": latest_target},
    )
    regular = [item for item in items if item["kind"] == "file"]
    lines = [f"{item['sha256']}  {item['path']}" for item in regular]
    inventory_path = attempt_dir / INVENTORY_NAME
    write_new(inventory_path, ("\n".join(lines) + "\n").encode("utf-8"), 0o444)
    return inventory_path, len(regular)


def _assert_no_obvious_secret(attempt_dir: Path) -> None:
    for path in attempt_dir.rglob("*"):
        if path.is_file() and not path.is_symlink() and SECRET_RE.search(path.read_bytes()):
            fail(f"refusing to seal an obvious credential pattern in {path}")


def seal_attempt(
    root: Path,
    config: Mapping[str, Any],
    episode: Mapping[str, Any],
    attempt_dir: Path,
    task_path: Path,
    offline: bool,
) -> dict[str, Any]:
    if (attempt_dir / "SEALED.json").exists():
        fail(f"final attempt is already sealed: {attempt_dir}")
    harness = validate_harness_attempt(config, episode, attempt_dir)
    summary = validate_run_artifacts(attempt_dir, episode, task_path, offline)
    _write_projections(attempt_dir, str(episode["episode_id"]), summary)
    metrics = {
        "version": VERSION,
        "phase": "development_final",
        "episode_id": episode["episode_id"],
        "run_mode": episode["run_mode"],
        "route_plan_sha256": config["route_plan_sha256"],
        "wall_millis": harness["wall_millis"],
        "return_code": harness["return_code"],
        "provider": summary["provider"],
        "budgets": summary["bundle"].get("budgets", {}),
        "local_retrieval": summary["bundle"].get("metrics", {}),
        "stages_skipped": summary["bundle"].get("stages_skipped", []),
        "cheap_exit": summary["cheap_exit"],
        "target_repository_commands_executed": 0,
        "provider_environment_disabled": True,
    }
    write_json_new(attempt_dir / "METRICS.json", metrics)
    _assert_no_obvious_secret(attempt_dir)
    inventory_path, inventory_entries = _write_sha256_inventory(
        attempt_dir,
        summary["latest_target"],
    )
    files = inventory_files(
        attempt_dir,
        excluded={"SEALED.json"},
        allowed_symlinks={"run/latest": summary["latest_target"]},
    )
    written_inventory = _parse_sha256_inventory(inventory_path)
    sealed_regular = {
        item["path"]: item["sha256"]
        for item in files
        if item["kind"] == "file" and item["path"] != INVENTORY_NAME
    }
    if written_inventory != sealed_regular:
        fail("final attempt changed while its SHA-256 inventory was being sealed")
    run_dir = summary["run_dir"]
    artifact_hashes = {
        name: sha256_file(run_dir / name)
        for name in REQUIRED_RUN_FILES
    }
    projection_hashes = {
        name: sha256_file(attempt_dir / name)
        for name in PROJECTION_FILES
    }
    status = summary["status"]
    seal = {
        "version": VERSION,
        "kind": "task_lens_v01_final_attempt_seal",
        "phase": "development_final",
        "episode_id": episode["episode_id"],
        "base_revision": episode["base_revision"],
        "tree_hash": episode["tree_hash"],
        "task_sha256": episode["task_sha256"],
        "binary_sha256": config["binary_sha256"],
        "manifest_sha256": config["manifest_sha256"],
        "route_plan_sha256": config["route_plan_sha256"],
        "run_mode": episode["run_mode"],
        "offline": offline,
        "state": status.get("state"),
        "sufficient": status.get("sufficient"),
        "provider_calls": summary["provider"]["calls"],
        "run_id": summary["latest_target"],
        "artifact_sha256": artifact_hashes,
        "projection_sha256": projection_hashes,
        "harness_attempt_sha256": sha256_file(attempt_dir / "HARNESS_ATTEMPT.json"),
        "metrics_sha256": sha256_file(attempt_dir / "METRICS.json"),
        "sha256_inventory": {
            "path": INVENTORY_NAME,
            "sha256": sha256_file(inventory_path),
            "regular_file_entries": inventory_entries,
        },
        "files": files,
        "semantic_retry": False,
        "one_process_invocation": True,
        "provider_environment_disabled": True,
        "target_repository_commands_executed": 0,
        "sealed_at": utc_now(),
    }
    write_json_new(attempt_dir / "SEALED.json", seal, 0o444)
    for item in files:
        if item["kind"] == "file":
            os.chmod(attempt_dir / item["path"], 0o444)
    return seal


def run_episode(
    root: Path,
    config: Mapping[str, Any],
    episode: Mapping[str, Any],
    timeout_seconds: int,
) -> dict[str, Any]:
    root = root.expanduser().resolve()
    task_path, repo = verify_prepared_episode(root, episode)
    episode_root = root / "episodes" / str(episode["episode_id"])
    final_dir = episode_root / "final"
    if final_dir.is_symlink() or (final_dir.exists() and not final_dir.is_dir()):
        fail(f"episode final path is not a regular directory: {final_dir}")
    attempt_dir = final_dir / "attempt"
    if attempt_dir.exists() or attempt_dir.is_symlink():
        fail(f"episode {episode['episode_id']} has already consumed its one final attempt")
    attempt_dir.mkdir(parents=True, exist_ok=False)
    (attempt_dir / "run").mkdir()

    binary_relative = safe_relative_path(str(config["binary_path"]), "binary_path")
    binary = resolve_under(root, binary_relative, "file", "frozen candidate binary")
    if sha256_file(binary) != config["binary_sha256"]:
        fail("frozen candidate binary changed before launch")
    offline = episode["run_mode"] == "offline_partial"
    actual_command = [
        str(binary),
        "investigate",
        str(repo),
        "--task-file",
        str(task_path),
        "--no-open",
        "--no-serve",
        "--debug-dir",
        str(attempt_dir / "run"),
        "--strict-snapshot",
    ]
    if offline:
        actual_command.append("--offline")
    launch = {
        "version": VERSION,
        "phase": "development_final",
        "final": True,
        "episode_id": episode["episode_id"],
        "run_mode": episode["run_mode"],
        "offline": offline,
        "one_process_invocation": True,
        "semantic_retry": False,
        "provider_environment_disabled": True,
        "base_revision": episode["base_revision"],
        "tree_hash": episode["tree_hash"],
        "task_sha256": episode["task_sha256"],
        "binary_sha256": config["binary_sha256"],
        "manifest_sha256": config["manifest_sha256"],
        "route_plan_sha256": config["route_plan_sha256"],
        "launch_nonce": secrets.token_hex(16),
        "command": canonical_command(offline),
        "started_at": utc_now(),
    }
    started_path = attempt_dir / "ATTEMPT_STARTED.json"
    write_json_new(started_path, launch, 0o444)
    started = time.monotonic()
    return_code, timed_out, launch_error = _invoke_cli(
        actual_command,
        attempt_dir,
        timeout_seconds,
    )
    wall_millis = int((time.monotonic() - started) * 1000)
    record = {
        **launch,
        "started_record_sha256": sha256_file(started_path),
        "return_code": return_code,
        "timed_out": timed_out,
        "wall_millis": wall_millis,
        "finished_at": utc_now(),
        "target_repository_commands_executed": 0,
    }
    if launch_error is not None:
        record["launch_error"] = launch_error
    write_json_new(attempt_dir / "HARNESS_ATTEMPT.json", record)

    if sha256_file(binary) != config["binary_sha256"]:
        fail("frozen candidate binary changed during the final attempt")
    verify_prepared_episode(root, episode)
    if return_code != 0:
        fail(
            f"Task Lens final process failed for {episode['episode_id']} with exit "
            f"{return_code}; the one attempt remains consumed and unsealed"
        )
    return seal_attempt(root, config, episode, attempt_dir, task_path, offline)


def _parse_sha256_inventory(path: Path) -> dict[str, str]:
    result: dict[str, str] = {}
    try:
        lines = path.read_text(encoding="utf-8").splitlines()
    except (OSError, UnicodeError) as exc:
        fail(f"cannot read SHA-256 inventory {path}: {exc}")
    for line_number, line in enumerate(lines, 1):
        match = re.fullmatch(r"([0-9a-f]{64})  (.+)", line)
        if match is None:
            fail(f"invalid SHA-256 inventory line {line_number} in {path}")
        relative = safe_relative_path(match.group(2), "SHA-256 inventory path").as_posix()
        if relative in result:
            fail(f"duplicate SHA-256 inventory path: {relative}")
        result[relative] = match.group(1)
    return result


def verify_seal(root: Path, config: Mapping[str, Any], episode: Mapping[str, Any]) -> dict[str, Any]:
    root = root.expanduser().resolve()
    task_path, _ = verify_prepared_episode(root, episode)
    attempt_relative = Path("episodes") / str(episode["episode_id"]) / "final" / "attempt"
    attempt_dir = resolve_under(root, attempt_relative, "dir", "final attempt directory")
    seal_path = resolve_under(
        root,
        attempt_relative / "SEALED.json",
        "file",
        "final attempt seal",
    )
    seal = read_json(seal_path)
    expected_offline = episode["run_mode"] == "offline_partial"
    if (
        seal.get("version") != VERSION
        or seal.get("kind") != "task_lens_v01_final_attempt_seal"
        or seal.get("phase") != "development_final"
        or seal.get("episode_id") != episode["episode_id"]
        or seal.get("base_revision") != episode["base_revision"]
        or seal.get("tree_hash") != episode["tree_hash"]
        or seal.get("task_sha256") != episode["task_sha256"]
        or seal.get("binary_sha256") != config["binary_sha256"]
        or seal.get("manifest_sha256") != config["manifest_sha256"]
        or seal.get("route_plan_sha256") != config["route_plan_sha256"]
        or seal.get("run_mode") != episode["run_mode"]
        or seal.get("offline") is not expected_offline
        or seal.get("provider_calls") != 0
        or seal.get("semantic_retry") is not False
        or seal.get("one_process_invocation") is not True
        or seal.get("provider_environment_disabled") is not True
        or seal.get("target_repository_commands_executed") != 0
    ):
        fail(f"final seal binding changed for {episode['episode_id']}")
    raw_files = seal.get("files")
    if not isinstance(raw_files, list):
        fail("final seal has no file inventory")
    seen: set[str] = set()
    for item in raw_files:
        if not isinstance(item, dict) or not isinstance(item.get("path"), str):
            fail("final seal contains an invalid file entry")
        relative = safe_relative_path(item["path"], "sealed path").as_posix()
        if relative in seen:
            fail(f"duplicate sealed path: {relative}")
        seen.add(relative)
        target = attempt_dir / relative
        if item.get("kind") == "file":
            if not target.is_file() or target.is_symlink():
                fail(f"sealed file is missing: {relative}")
            if target.stat().st_size != item.get("bytes") or sha256_file(target) != item.get("sha256"):
                fail(f"sealed file changed: {relative}")
        elif item.get("kind") == "symlink":
            if not target.is_symlink() or os.readlink(target) != item.get("target"):
                fail(f"sealed symlink changed: {relative}")
            if relative != "run/latest":
                fail(f"sealed inventory contains an unexpected symlink: {relative}")
            raw_target = os.readlink(target).encode("utf-8", "surrogateescape")
            if len(raw_target) != item.get("bytes") or sha256_bytes(raw_target) != item.get("sha256"):
                fail(f"sealed symlink target changed: {relative}")
        else:
            fail(f"sealed path has an unsupported kind: {relative}")
    actual = {
        path.relative_to(attempt_dir).as_posix()
        for path in attempt_dir.rglob("*")
        if (path.is_file() or path.is_symlink())
        and path.relative_to(attempt_dir).as_posix() != "SEALED.json"
    }
    if actual != seen:
        fail("sealed final attempt file set changed")

    inventory_path = resolve_under(
        root,
        attempt_relative / INVENTORY_NAME,
        "file",
        "SHA-256 inventory",
    )
    inventory = _parse_sha256_inventory(inventory_path)
    regular = {
        item["path"]: item["sha256"]
        for item in raw_files
        if item.get("kind") == "file" and item.get("path") != INVENTORY_NAME
    }
    if inventory != regular:
        fail("SHA-256 inventory does not exactly bind the sealed regular files")
    inventory_record = _require_object(seal.get("sha256_inventory"), "seal sha256_inventory")
    if (
        inventory_record.get("path") != INVENTORY_NAME
        or inventory_record.get("sha256") != sha256_file(inventory_path)
        or inventory_record.get("regular_file_entries") != len(inventory)
    ):
        fail("seal SHA-256 inventory binding changed")

    harness = validate_harness_attempt(config, episode, attempt_dir)
    offline = harness.get("offline")
    if not isinstance(offline, bool) or offline != (episode["run_mode"] == "offline_partial"):
        fail("sealed harness mode differs from the frozen run plan")
    summary = validate_run_artifacts(attempt_dir, episode, task_path, offline)
    if (
        seal.get("run_id") != summary["latest_target"]
        or seal.get("state") != summary["status"].get("state")
        or seal.get("sufficient") != summary["status"].get("sufficient")
        or seal.get("harness_attempt_sha256")
        != sha256_file(attempt_dir / "HARNESS_ATTEMPT.json")
        or seal.get("metrics_sha256") != sha256_file(attempt_dir / "METRICS.json")
    ):
        fail("final seal result binding changed")
    artifact_hashes = seal.get("artifact_sha256")
    if not isinstance(artifact_hashes, dict) or set(artifact_hashes) != set(REQUIRED_RUN_FILES):
        fail("final seal artifact hash set changed")
    for name in REQUIRED_RUN_FILES:
        if artifact_hashes[name] != sha256_file(summary["run_dir"] / name):
            fail(f"final seal artifact binding changed: {name}")
    projection_hashes = seal.get("projection_sha256")
    if not isinstance(projection_hashes, dict) or set(projection_hashes) != set(PROJECTION_FILES):
        fail("final seal projection hash set changed")
    for name in PROJECTION_FILES:
        expected_hash = projection_hashes.get(name)
        if expected_hash != sha256_file(attempt_dir / name):
            fail(f"sealed projection changed: {name}")
    if read_json(attempt_dir / "role_contract.json") != summary["role_contract"]:
        fail("sealed role contract projection changed")
    if read_json(attempt_dir / "role_coverage.json") != summary["role_coverage"]:
        fail("sealed role coverage projection changed")
    if read_json(attempt_dir / "verification_frontier.json") != summary["verification_frontier"]:
        fail("sealed verification frontier projection changed")
    if read_json(attempt_dir / "cheap_exit_decision.json") != summary["cheap_exit"]:
        fail("sealed cheap-exit projection changed")
    source_projection = read_json(attempt_dir / "source_scopes.json")
    if source_projection.get("source_scopes") != summary["source_scopes"]:
        fail("sealed source-scope projection changed")
    return seal


def selected_episode_ids(
    episodes: Mapping[str, Mapping[str, Any]],
    episode_id: str | None,
    all_episodes: bool,
) -> list[str]:
    if bool(episode_id) == all_episodes:
        fail("select exactly one of --episode or --all")
    if episode_id:
        if episode_id not in episodes:
            fail(f"unknown episode: {episode_id}")
        return [episode_id]
    return list(episodes)


def command_prepare(args: argparse.Namespace) -> None:
    zero_call_episode_ids = [] if args.all_offline else args.zero_call_episode
    prepare_experiment(
        Path(args.root),
        Path(args.manifest),
        Path(args.source_repo),
        Path(args.tasks_root),
        Path(args.binary),
        zero_call_episode_ids,
    )


def command_run(args: argparse.Namespace) -> None:
    root = Path(args.root).expanduser().resolve()
    config, episodes = load_config(root)
    failures: list[str] = []
    for episode_id in selected_episode_ids(episodes, args.episode, args.all):
        try:
            attempt_dir = root / "episodes" / episode_id / "final" / "attempt"
            if args.all and (attempt_dir / "SEALED.json").is_file():
                verify_seal(root, config, episodes[episode_id])
                continue
            run_episode(root, config, episodes[episode_id], args.timeout_seconds)
        except HarnessError as exc:
            if not args.all:
                raise
            failures.append(f"{episode_id}: {exc}")
    if failures:
        fail(
            f"--all finished with {len(failures)} failed or unsealed episode(s); "
            + " | ".join(failures)
        )


def command_verify(args: argparse.Namespace) -> None:
    root = Path(args.root).expanduser().resolve()
    config, episodes = load_config(root)
    for episode_id in selected_episode_ids(episodes, args.episode, args.all):
        verify_seal(root, config, episodes[episode_id])


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description=__doc__)
    subparsers = parser.add_subparsers(dest="command", required=True)

    prepare = subparsers.add_parser(
        "prepare",
        help="freeze the binary/run plan and stage exact detached episode worktrees",
    )
    prepare.add_argument("--root", required=True)
    prepare.add_argument("--manifest", required=True)
    prepare.add_argument("--source-repo", required=True)
    prepare.add_argument("--tasks-root", required=True)
    prepare.add_argument("--binary", required=True)
    routes = prepare.add_mutually_exclusive_group(required=True)
    routes.add_argument(
        "--zero-call-episode",
        action="append",
        metavar="ID",
        help=(
            "episode proven by pre-final local calibration to use the non-offline "
            "zero-call route; repeat for every such episode"
        ),
    )
    routes.add_argument(
        "--all-offline",
        action="store_true",
        help="freeze every episode to an honest offline partial route",
    )
    prepare.set_defaults(func=command_prepare)

    run = subparsers.add_parser("run", help="consume and seal one final attempt per episode")
    run.add_argument("--root", required=True)
    run.add_argument("--episode")
    run.add_argument("--all", action="store_true")
    run.add_argument("--timeout-seconds", type=int, default=900)
    run.set_defaults(func=command_run)

    verify = subparsers.add_parser("verify", help="verify immutable final attempt seals")
    verify.add_argument("--root", required=True)
    verify.add_argument("--episode")
    verify.add_argument("--all", action="store_true")
    verify.set_defaults(func=command_verify)
    return parser


def main() -> int:
    try:
        args = build_parser().parse_args()
        if getattr(args, "timeout_seconds", 1) <= 0:
            fail("--timeout-seconds must be positive")
        args.func(args)
        return 0
    except HarnessError as exc:
        print(f"task-lens-v01-harness: error: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
