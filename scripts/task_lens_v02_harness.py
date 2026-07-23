#!/usr/bin/env python3
"""One-shot preparation, execution, and sealing for Task Lens v0.2.

The v0.2 run is deliberately limited to the six known-development primary
episodes.  Its route partition is inherited from the pinned v0.1 final-run
configuration: ``nil_body_validation_panic`` is the only non-offline zero-call
episode and the other five episodes are honest offline partials.

The harness never executes a command from the target repository.  It freezes a
private bare copy of the supplied Git object store, disables hooks and
fsmonitor for every Git invocation, launches one ``repomap investigate``
process per final episode, and refuses retries.
"""

from __future__ import annotations

import argparse
import os
import re
import secrets
import subprocess
import sys
import time
from pathlib import Path
from typing import Any, Mapping, Sequence

import task_lens_v01_harness as core


VERSION = 2
CONFIG_NAME = "FINAL_RUN_CONFIG.json"
CONFIG_SEAL_NAME = "FINAL_RUN_CONFIG.sha256"
INVENTORY_NAME = core.INVENTORY_NAME
SOURCE_STORE_PATH = "inputs/source.git"
V01_CONFIG_PATH = "inputs/V01_FINAL_RUN_CONFIG.json"

MANIFEST_KIND = "task_lens_v02_known_development"
ROUTE_PLAN_KIND = "task_lens_v02_frozen_route_plan"
CONFIG_KIND = "task_lens_v02_final_run_config"
ATTEMPT_SEAL_KIND = "task_lens_v02_final_attempt_seal"
ROUTE_SELECTION_SOURCE = "frozen_v01_same_six_partition"

V01_CONFIG_SHA256 = "f5c0439e82c1f921c84d065e21c634e1fd4eb9d7b0b5350ef845a77521c5b4ee"
V01_BINARY_SHA256 = "30780fde698e48822d2d07653e43036610aee5283ccb3c57537c7d2792a0050f"
V01_ROUTE_PLAN_SHA256 = "7504f07cda716a862eb3276df3d9ddf980087ae400c071f19f4bfa78d0e5f652"
V01_CALIBRATION_EPISODE = "openapi_disable_messages_config"
V01_ZERO_CALL_PRIMARY = "nil_body_validation_panic"

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
)

CANONICAL_CONSTRAINTS = {
    "semantic_retries": 0,
    "maximum_model_calls_per_episode": 1,
    "maximum_total_fresh_model_calls": 6,
    "provider_model": "deepseek-v4-flash",
    "provider_endpoint": "https://api.deepseek.com/chat/completions",
    "target_repository_commands": 0,
}

HarnessError = core.HarnessError
fail = core.fail
utc_now = core.utc_now
canonical_json = core.canonical_json
sha256_bytes = core.sha256_bytes
sha256_file = core.sha256_file
read_json = core.read_json
write_new = core.write_new
write_json_new = core.write_json_new
copy_new = core.copy_new
resolve_file = core.resolve_file
resolve_dir = core.resolve_dir
safe_relative_path = core.safe_relative_path
resolve_under = core.resolve_under
prompt_safe_task_text = core.prompt_safe_task_text
canonical_command = core.canonical_command
validate_run_artifacts = core.validate_run_artifacts
inventory_files = core.inventory_files
_parse_sha256_inventory = core._parse_sha256_inventory
_assert_no_obvious_secret = core._assert_no_obvious_secret


_GIT_ENV_NAMES = {
    "GIT_DIR",
    "GIT_WORK_TREE",
    "GIT_INDEX_FILE",
    "GIT_COMMON_DIR",
    "GIT_OBJECT_DIRECTORY",
    "GIT_ALTERNATE_OBJECT_DIRECTORIES",
    "GIT_CONFIG",
    "GIT_CONFIG_COUNT",
    "GIT_CONFIG_PARAMETERS",
    "GIT_CONFIG_SYSTEM",
    "GIT_CONFIG_GLOBAL",
    "GIT_CONFIG_NOSYSTEM",
    "GIT_EXTERNAL_DIFF",
    "GIT_PAGER",
    "PAGER",
}


def isolated_environment(*, disable_provider: bool = False) -> dict[str, str]:
    """Return a Git- and provider-isolated child-process environment."""

    result: dict[str, str] = {}
    for name, value in os.environ.items():
        if (
            name in _GIT_ENV_NAMES
            or name.startswith("GIT_CONFIG_KEY_")
            or name.startswith("GIT_CONFIG_VALUE_")
        ):
            continue
        if disable_provider and name.startswith(core.PROVIDER_ENV_PREFIXES):
            continue
        result[name] = value
    result.update(
        {
            "GIT_OPTIONAL_LOCKS": "0",
            "GIT_CONFIG_NOSYSTEM": "1",
            "GIT_CONFIG_SYSTEM": os.devnull,
            "GIT_CONFIG_GLOBAL": os.devnull,
            "GIT_PAGER": "cat",
            "PAGER": "cat",
        }
    )
    return result


def final_attempt_env() -> dict[str, str]:
    """Disable provider credentials and all ambient Git configuration."""

    return isolated_environment(disable_provider=True)


def run_command(
    argv: Sequence[str],
    *,
    cwd: Path | None = None,
    check: bool = True,
) -> subprocess.CompletedProcess[str]:
    result = subprocess.run(
        list(argv),
        cwd=str(cwd) if cwd else None,
        env=isolated_environment(),
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


def _isolated_git_argv(repo: Path | None, args: Sequence[str]) -> list[str]:
    command = [
        "git",
        "--no-pager",
        "-c",
        "core.fsmonitor=false",
        "-c",
        f"core.hooksPath={os.devnull}",
    ]
    if repo is not None:
        command.extend(("-C", str(repo)))
    command.extend(args)
    return command


def git(
    repo: Path,
    *args: str,
    check: bool = True,
) -> subprocess.CompletedProcess[str]:
    return run_command(_isolated_git_argv(repo, args), check=check)


def git_output(repo: Path, *args: str) -> str:
    return git(repo, *args).stdout.strip()


def ensure_worktree_repository(repo: Path) -> None:
    if git_output(repo, "rev-parse", "--is-inside-work-tree") != "true":
        fail(f"not a Git worktree: {repo}")


def ensure_bare_source_store(repo: Path) -> None:
    if git_output(repo, "rev-parse", "--is-bare-repository") != "true":
        fail(f"not a frozen bare Git object store: {repo}")


def verify_worktree(
    repo: Path,
    revision: str,
    tree_hash: str,
    expected_source_store: Path | None = None,
) -> None:
    if repo.name != "repo":
        fail(f"episode worktree basename must be neutral 'repo': {repo}")
    if expected_source_store is not None:
        git_pointer = repo / ".git"
        if not git_pointer.is_file() or git_pointer.is_symlink():
            fail(f"episode worktree has no regular Git pointer: {repo}")
        try:
            prefix, raw_git_dir = git_pointer.read_text(encoding="utf-8").strip().split(
                ": ",
                1,
            )
            git_dir = Path(raw_git_dir).resolve(strict=True)
            git_dir.relative_to(expected_source_store.resolve() / "worktrees")
        except (OSError, UnicodeError, ValueError) as exc:
            fail(
                f"episode worktree is not bound to the frozen local source store: "
                f"{repo}: {exc}"
            )
        if prefix != "gitdir":
            fail(f"episode worktree has an invalid Git pointer: {repo}")
    ensure_worktree_repository(repo)
    if git_output(repo, "rev-parse", "HEAD") != revision:
        fail(f"episode worktree revision changed: {repo}")
    if git_output(repo, "rev-parse", "HEAD^{tree}") != tree_hash:
        fail(f"episode worktree tree changed: {repo}")
    symbolic = git(repo, "symbolic-ref", "-q", "HEAD", check=False)
    if symbolic.returncode != 1:
        fail(f"episode worktree must remain at detached HEAD: {repo}")
    if git_output(repo, "status", "--porcelain", "--untracked-files=all"):
        fail(f"episode worktree is not clean: {repo}")


def _clone_bare_source(source_repo: Path, destination: Path) -> None:
    destination.parent.mkdir(parents=True, exist_ok=True)
    command = _isolated_git_argv(
        None,
        (
            "clone",
            "--bare",
            "--local",
            "--no-hardlinks",
            str(source_repo),
            str(destination),
        ),
    )
    run_command(command)
    ensure_bare_source_store(destination)
    # The frozen object store is self-contained; no future operation may fall
    # back to the mutable or ephemeral source path.
    git(destination, "remote", "remove", "origin", check=False)


def _contains_holdout_component(path: Path) -> bool:
    return any(part.casefold() == "holdout" for part in path.parts)


def find_task_source(tasks_root: Path, episode_id: str) -> Path:
    tasks_root = resolve_dir(tasks_root, "task source root")
    if _contains_holdout_component(tasks_root):
        fail(f"task source root must not contain a holdout component: {tasks_root}")
    candidate = tasks_root / episode_id / "task.md"
    if _contains_holdout_component(candidate):
        fail(f"episode {episode_id}: task source must not use a holdout path")
    if not candidate.is_file() or candidate.is_symlink():
        fail(
            f"episode {episode_id}: expected exactly "
            f"<tasks-root>/{episode_id}/task.md"
        )
    try:
        candidate.resolve(strict=True).relative_to(tasks_root)
    except (OSError, ValueError) as exc:
        fail(f"episode {episode_id}: task source escapes tasks root: {exc}")
    return candidate


def load_manifest(path: Path) -> dict[str, Any]:
    manifest = read_json(path)
    raw_episodes = manifest.get("episodes")
    if (
        manifest.get("version") != VERSION
        or manifest.get("kind") != MANIFEST_KIND
        or manifest.get("fresh_generalization") is not False
        or manifest.get("primary_episode_count") != len(CANONICAL_EPISODES)
        or manifest.get("cheap_exit_calibration_count") != 0
        or manifest.get("constraints") != CANONICAL_CONSTRAINTS
        or not isinstance(raw_episodes, list)
        or len(raw_episodes) != len(CANONICAL_EPISODES)
    ):
        fail("the canonical Task Lens v0.2 development manifest header is invalid")

    episodes: list[dict[str, Any]] = []
    seen: set[str] = set()
    for index, raw in enumerate(raw_episodes):
        if not isinstance(raw, dict):
            fail(f"manifest episode {index} is not an object")
        episode_id = raw.get("episode_id")
        revision = raw.get("base_revision")
        cheap = raw.get("cheap_exit_target")
        if (
            not isinstance(episode_id, str)
            or core.EPISODE_ID_RE.fullmatch(episode_id) is None
        ):
            fail(f"manifest episode {index} has an invalid episode_id")
        if episode_id in seen:
            fail(f"duplicate manifest episode: {episode_id}")
        seen.add(episode_id)
        if not isinstance(revision, str) or core.REVISION_RE.fullmatch(revision) is None:
            fail(f"manifest episode {episode_id} has an invalid base_revision")
        if not isinstance(cheap, bool):
            fail(f"manifest episode {episode_id} has no cheap_exit_target boolean")
        expected_attempt = f"episodes/{episode_id}/final/attempt"
        if raw.get("final_attempt") != expected_attempt:
            fail(f"manifest episode {episode_id} has a non-canonical final_attempt")
        episodes.append(dict(raw))

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
        fail("canonical Task Lens v0.2 episode IDs, revisions, scopes, or modes changed")
    result = dict(manifest)
    result["episodes"] = episodes
    return result


def _validate_v01_route_source(path: Path) -> dict[str, Any]:
    path = resolve_file(path, "pinned v0.1 final-run config")
    if sha256_file(path) != V01_CONFIG_SHA256:
        fail("supplied v0.1 final-run config does not match the pinned baseline")
    config = read_json(path)
    route_plan = config.get("route_plan")
    if (
        config.get("version") != 1
        or config.get("kind") != "task_lens_v01_final_run_config"
        or config.get("binary_sha256") != V01_BINARY_SHA256
        or config.get("route_plan_sha256") != V01_ROUTE_PLAN_SHA256
        or not isinstance(route_plan, dict)
        or route_plan.get("kind") != "task_lens_v01_frozen_route_plan"
        or sha256_bytes(canonical_json(route_plan)) != V01_ROUTE_PLAN_SHA256
    ):
        fail("supplied v0.1 final-run config has an invalid pinned route binding")

    primary_ids = [item[0] for item in CANONICAL_EPISODES]
    expected_zero = [V01_ZERO_CALL_PRIMARY, V01_CALIBRATION_EPISODE]
    expected_offline = [
        episode_id
        for episode_id in primary_ids
        if episode_id != V01_ZERO_CALL_PRIMARY
    ]
    if (
        route_plan.get("zero_call_episode_ids") != expected_zero
        or route_plan.get("offline_partial_episode_ids") != expected_offline
    ):
        fail("v0.1 route plan is not the pinned same-six baseline partition")
    return config


def frozen_route_plan() -> dict[str, Any]:
    primary_ids = [item[0] for item in CANONICAL_EPISODES]
    if V01_ZERO_CALL_PRIMARY not in primary_ids:
        fail("canonical v0.2 episode set has no pinned v0.1 zero-call episode")
    return {
        "version": VERSION,
        "kind": ROUTE_PLAN_KIND,
        "selection_source": ROUTE_SELECTION_SOURCE,
        "baseline_v01_config_sha256": V01_CONFIG_SHA256,
        "baseline_v01_route_plan_sha256": V01_ROUTE_PLAN_SHA256,
        "zero_call_episode_ids": [V01_ZERO_CALL_PRIMARY],
        "offline_partial_episode_ids": [
            episode_id
            for episode_id in primary_ids
            if episode_id != V01_ZERO_CALL_PRIMARY
        ],
        "excluded_v01_episode_ids": [V01_CALIBRATION_EPISODE],
    }


def validate_frozen_route_plan(
    config: Mapping[str, Any],
    manifest: Mapping[str, Any],
) -> tuple[dict[str, Any], set[str]]:
    route_plan = config.get("route_plan")
    expected = frozen_route_plan()
    if route_plan != expected:
        fail("final-run config has no canonical frozen v0.2 route plan")
    route_hash = config.get("route_plan_sha256")
    if (
        not isinstance(route_hash, str)
        or core.SHA256_RE.fullmatch(route_hash) is None
        or route_hash != sha256_bytes(canonical_json(expected))
    ):
        fail("frozen route plan hash does not match FINAL_RUN_CONFIG.json")
    manifest_ids = [item["episode_id"] for item in manifest["episodes"]]
    routed = set(expected["zero_call_episode_ids"]) | set(
        expected["offline_partial_episode_ids"]
    )
    if routed != set(manifest_ids):
        fail("frozen route plan is not an exact partition of the manifest")
    expected_offline = [
        episode_id
        for episode_id in manifest_ids
        if episode_id not in set(expected["zero_call_episode_ids"])
    ]
    if expected["offline_partial_episode_ids"] != expected_offline:
        fail("frozen route plan order differs from the canonical manifest")
    return dict(expected), set(expected["zero_call_episode_ids"])


def prepare_experiment(
    root: Path,
    manifest_path: Path,
    source_repo: Path,
    tasks_root: Path,
    binary: Path,
    v01_config_path: Path,
) -> dict[str, Any]:
    root = root.expanduser().resolve()
    manifest_path = resolve_file(manifest_path, "development manifest")
    source_repo = resolve_dir(source_repo, "source repository")
    tasks_root = resolve_dir(tasks_root, "task source root")
    if _contains_holdout_component(tasks_root):
        fail(f"task source root must not contain a holdout component: {tasks_root}")
    binary = resolve_file(binary, "candidate binary")
    if not os.access(binary, os.X_OK):
        fail(f"candidate binary is not executable: {binary}")
    candidate_sha = sha256_file(binary)
    if candidate_sha == V01_BINARY_SHA256:
        fail("candidate binary is byte-identical to the pinned v0.1 binary")
    ensure_worktree_repository(source_repo)

    manifest = load_manifest(manifest_path)
    v01_config_path = resolve_file(v01_config_path, "pinned v0.1 final-run config")
    _validate_v01_route_source(v01_config_path)
    route_plan = frozen_route_plan()
    zero_call_ids = set(route_plan["zero_call_episode_ids"])

    config_path = root / CONFIG_NAME
    config_seal = root / CONFIG_SEAL_NAME
    frozen_binary = root / "inputs" / "repomap"
    frozen_manifest = root / "inputs" / "DEVELOPMENT_SET.json"
    frozen_v01_config = root / V01_CONFIG_PATH
    frozen_source = root / SOURCE_STORE_PATH
    for target in (
        config_path,
        config_seal,
        frozen_binary,
        frozen_manifest,
        frozen_v01_config,
        frozen_source,
    ):
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
                "run_mode": (
                    "zero_call" if episode_id in zero_call_ids else "offline_partial"
                ),
                "task_text": task_text,
            }
        )

    root.mkdir(parents=True, exist_ok=True)
    copy_new(binary, frozen_binary, 0o555)
    copy_new(manifest_path, frozen_manifest, 0o444)
    copy_new(v01_config_path, frozen_v01_config, 0o444)
    _clone_bare_source(source_repo, frozen_source)

    for item in planned:
        if (
            git_output(
                frozen_source,
                "rev-parse",
                "--verify",
                f"{item['base_revision']}^{{commit}}",
            )
            != item["base_revision"]
            or git_output(
                frozen_source,
                "rev-parse",
                f"{item['base_revision']}^{{tree}}",
            )
            != item["tree_hash"]
        ):
            fail(
                f"episode {item['episode_id']}: frozen source lost the exact revision/tree"
            )
        episode_dir = root / "episodes" / item["episode_id"]
        worktree = episode_dir / "worktree" / "repo"
        worktree.parent.mkdir(parents=True, exist_ok=False)
        git(
            frozen_source,
            "worktree",
            "add",
            "--detach",
            str(worktree),
            item["base_revision"],
        )
        verify_worktree(
            worktree,
            item["base_revision"],
            item["tree_hash"],
            frozen_source,
        )
        task_path = episode_dir / "task.md"
        write_new(task_path, item["task_text"].encode("utf-8"), 0o444)
        item["task_sha256"] = sha256_file(task_path)
        item["task_path"] = f"episodes/{item['episode_id']}/task.md"
        item["repository_path"] = f"episodes/{item['episode_id']}/worktree/repo"
        del item["task_text"]

    config = {
        "version": VERSION,
        "kind": CONFIG_KIND,
        "known_development_only": True,
        "semantic_retries": 0,
        "maximum_process_invocations_per_episode": 1,
        "target_repository_commands_executed": 0,
        "provider_environment_disabled": True,
        "git_environment_isolated": True,
        "route_plan": route_plan,
        "route_plan_sha256": sha256_bytes(canonical_json(route_plan)),
        "baseline_v01_config_path": V01_CONFIG_PATH,
        "baseline_v01_config_sha256": V01_CONFIG_SHA256,
        "source_repository_path": SOURCE_STORE_PATH,
        "source_repository_kind": "frozen_bare_git_object_store",
        "source_repository_origin": str(source_repo),
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
        or config.get("kind") != CONFIG_KIND
        or config.get("known_development_only") is not True
        or config.get("semantic_retries") != 0
        or config.get("maximum_process_invocations_per_episode") != 1
        or config.get("target_repository_commands_executed") != 0
        or config.get("provider_environment_disabled") is not True
        or config.get("git_environment_isolated") is not True
        or config.get("source_repository_kind")
        != "frozen_bare_git_object_store"
    ):
        fail("final-run config header is invalid")

    binary_relative = safe_relative_path(
        str(config.get("binary_path", "")),
        "binary_path",
    )
    manifest_relative = safe_relative_path(
        str(config.get("manifest_path", "")), "manifest_path"
    )
    source_relative = safe_relative_path(
        str(config.get("source_repository_path", "")),
        "source_repository_path",
    )
    v01_relative = safe_relative_path(
        str(config.get("baseline_v01_config_path", "")),
        "baseline_v01_config_path",
    )
    if (
        binary_relative.as_posix() != "inputs/repomap"
        or manifest_relative.as_posix() != "inputs/DEVELOPMENT_SET.json"
        or source_relative.as_posix() != SOURCE_STORE_PATH
        or v01_relative.as_posix() != V01_CONFIG_PATH
    ):
        fail("final-run config uses a non-canonical frozen input path")

    binary = resolve_under(root, binary_relative, "file", "frozen candidate binary")
    manifest_path = resolve_under(
        root,
        manifest_relative,
        "file",
        "frozen development manifest",
    )
    source_store = resolve_under(
        root,
        source_relative,
        "dir",
        "frozen source object store",
    )
    v01_config_path = resolve_under(
        root,
        v01_relative,
        "file",
        "frozen v0.1 final-run config",
    )
    if (
        sha256_file(binary) != config.get("binary_sha256")
        or config.get("binary_sha256") == V01_BINARY_SHA256
    ):
        fail("frozen candidate binary changed or equals the v0.1 binary")
    if sha256_file(manifest_path) != config.get("manifest_sha256"):
        fail("frozen development manifest changed after preparation")
    if (
        config.get("baseline_v01_config_sha256") != V01_CONFIG_SHA256
        or sha256_file(v01_config_path) != V01_CONFIG_SHA256
    ):
        fail("frozen v0.1 final-run config changed after preparation")
    _validate_v01_route_source(v01_config_path)
    ensure_bare_source_store(source_store)
    if git_output(source_store, "remote"):
        fail("frozen source object store unexpectedly retains a remote")
    if (source_store / "objects" / "info" / "alternates").exists():
        fail("frozen source object store unexpectedly depends on alternate objects")

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
            or core.REVISION_RE.fullmatch(raw["tree_hash"]) is None
            or not isinstance(raw.get("task_sha256"), str)
            or core.SHA256_RE.fullmatch(raw["task_sha256"]) is None
            or git_output(
                source_store,
                "rev-parse",
                f"{raw['base_revision']}^{{tree}}",
            )
            != raw["tree_hash"]
        ):
            fail(f"final-run config binding is invalid for {episode_id}")
        episodes[episode_id] = raw
    if list(episodes) != [item["episode_id"] for item in manifest["episodes"]]:
        fail("final-run config and manifest episode order or sets differ")
    return config, episodes


def verify_prepared_episode(
    root: Path,
    episode: Mapping[str, Any],
) -> tuple[Path, Path]:
    task_relative = safe_relative_path(str(episode.get("task_path", "")), "task_path")
    repo_relative = safe_relative_path(
        str(episode.get("repository_path", "")),
        "repository_path",
    )
    task_path = resolve_under(root, task_relative, "file", "canonical episode task")
    repo = resolve_under(root, repo_relative, "dir", "episode worktree")
    if sha256_file(task_path) != episode.get("task_sha256"):
        fail(f"episode task changed after preparation: {task_path}")
    verify_worktree(
        repo,
        str(episode["base_revision"]),
        str(episode["tree_hash"]),
        root / SOURCE_STORE_PATH,
    )
    return task_path, repo


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


def _write_projections(
    attempt_dir: Path,
    episode_id: str,
    summary: Mapping[str, Any],
) -> None:
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
        "git_environment_isolated": True,
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
        "git_environment_isolated": True,
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
        for name in core.REQUIRED_RUN_FILES
    }
    projection_hashes = {
        name: sha256_file(attempt_dir / name)
        for name in core.PROJECTION_FILES
    }
    status = summary["status"]
    seal = {
        "version": VERSION,
        "kind": ATTEMPT_SEAL_KIND,
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
        "git_environment_isolated": True,
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
        "git_environment_isolated": True,
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


def verify_seal(
    root: Path,
    config: Mapping[str, Any],
    episode: Mapping[str, Any],
) -> dict[str, Any]:
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
        or seal.get("kind") != ATTEMPT_SEAL_KIND
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
        or seal.get("git_environment_isolated") is not True
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
            if (
                target.stat().st_size != item.get("bytes")
                or sha256_file(target) != item.get("sha256")
            ):
                fail(f"sealed file changed: {relative}")
        elif item.get("kind") == "symlink":
            if not target.is_symlink() or os.readlink(target) != item.get("target"):
                fail(f"sealed symlink changed: {relative}")
            if relative != "run/latest":
                fail(f"sealed inventory contains an unexpected symlink: {relative}")
            raw_target = os.readlink(target).encode("utf-8", "surrogateescape")
            if (
                len(raw_target) != item.get("bytes")
                or sha256_bytes(raw_target) != item.get("sha256")
            ):
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
    inventory_record = core._require_object(
        seal.get("sha256_inventory"),
        "seal sha256_inventory",
    )
    if (
        inventory_record.get("path") != INVENTORY_NAME
        or inventory_record.get("sha256") != sha256_file(inventory_path)
        or inventory_record.get("regular_file_entries") != len(inventory)
    ):
        fail("seal SHA-256 inventory binding changed")

    harness = validate_harness_attempt(config, episode, attempt_dir)
    offline = harness.get("offline")
    if not isinstance(offline, bool) or offline != expected_offline:
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
    if (
        not isinstance(artifact_hashes, dict)
        or set(artifact_hashes) != set(core.REQUIRED_RUN_FILES)
    ):
        fail("final seal artifact hash set changed")
    for name in core.REQUIRED_RUN_FILES:
        if artifact_hashes[name] != sha256_file(summary["run_dir"] / name):
            fail(f"final seal artifact binding changed: {name}")
    projection_hashes = seal.get("projection_sha256")
    if (
        not isinstance(projection_hashes, dict)
        or set(projection_hashes) != set(core.PROJECTION_FILES)
    ):
        fail("final seal projection hash set changed")
    for name in core.PROJECTION_FILES:
        if projection_hashes.get(name) != sha256_file(attempt_dir / name):
            fail(f"sealed projection changed: {name}")
    if read_json(attempt_dir / "role_contract.json") != summary["role_contract"]:
        fail("sealed role contract projection changed")
    if read_json(attempt_dir / "role_coverage.json") != summary["role_coverage"]:
        fail("sealed role coverage projection changed")
    if (
        read_json(attempt_dir / "verification_frontier.json")
        != summary["verification_frontier"]
    ):
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
    prepare_experiment(
        Path(args.root),
        Path(args.manifest),
        Path(args.source_repo),
        Path(args.tasks_root),
        Path(args.binary),
        Path(args.route_plan_from_v01),
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
        help="freeze v0.2 inputs, route plan, source objects, and worktrees",
    )
    prepare.add_argument("--root", required=True)
    prepare.add_argument("--manifest", required=True)
    prepare.add_argument("--source-repo", required=True)
    prepare.add_argument("--tasks-root", required=True)
    prepare.add_argument("--binary", required=True)
    prepare.add_argument(
        "--route-plan-from-v01",
        required=True,
        help="pinned v0.1 FINAL_RUN_CONFIG.json supplying the same-six partition",
    )
    prepare.set_defaults(func=command_prepare)

    run = subparsers.add_parser("run", help="consume and seal one final attempt")
    run.add_argument("--root", required=True)
    run.add_argument("--episode")
    run.add_argument("--all", action="store_true")
    run.add_argument("--timeout-seconds", type=int, default=900)
    run.set_defaults(func=command_run)

    verify = subparsers.add_parser("verify", help="verify immutable v0.2 seals")
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
        print(f"task-lens-v02-harness: error: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
