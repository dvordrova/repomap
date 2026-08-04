#!/usr/bin/env python3
"""Focused integration tests for the one-shot Task Lens v0.2 harness."""

from __future__ import annotations

import json
import os
import stat
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from unittest import mock


sys.dont_write_bytecode = True
sys.path.insert(0, str(Path(__file__).resolve().parent))

import task_lens_v02_harness as harness  # noqa: E402
from test_task_lens_v01_harness import FAKE_CLI  # noqa: E402


REAL_CANONICAL_EPISODES = harness.CANONICAL_EPISODES


class TaskLensV02CanonicalContractTest(unittest.TestCase):
    def test_contract_is_exactly_the_six_primary_episodes(self) -> None:
        self.assertEqual(
            [item[0] for item in REAL_CANONICAL_EPISODES],
            [
                "openapi_example_tag_parsing",
                "openapi_required_nullable_semantics",
                "accept_header_wrong_status",
                "nil_body_validation_panic",
                "httperror_pointer_value",
                "multi_module_release_script",
            ],
        )
        self.assertEqual(
            {item[2] for item in REAL_CANONICAL_EPISODES},
            {"primary_regression"},
        )
        self.assertNotIn(
            harness.V01_CALIBRATION_EPISODE,
            {item[0] for item in REAL_CANONICAL_EPISODES},
        )
        self.assertEqual(harness.VERSION, 2)
        self.assertEqual(harness.MANIFEST_KIND, "task_lens_v02_known_development")
        self.assertEqual(harness.CONFIG_KIND, "task_lens_v02_final_run_config")
        self.assertEqual(
            harness.ATTEMPT_SEAL_KIND,
            "task_lens_v02_final_attempt_seal",
        )


class TaskLensV02HarnessTest(unittest.TestCase):
    def setUp(self) -> None:
        self.temporary = tempfile.TemporaryDirectory(prefix="task-lens-v02-harness-")
        self.base = Path(self.temporary.name)
        self.source_repo = self.base / "source"
        self.source_repo.mkdir()
        self._git(self.source_repo, "init")
        (self.source_repo / "main.go").write_text(
            "package synthetic\n\nfunc Value() int { return 1 }\n",
            encoding="utf-8",
        )
        (self.source_repo / "main_test.go").write_text(
            "package synthetic\n\nfunc TestSynthetic() {}\n",
            encoding="utf-8",
        )
        self._git(self.source_repo, "add", "main.go", "main_test.go")
        self._git(
            self.source_repo,
            "-c",
            "user.name=Synthetic Test",
            "-c",
            "user.email=synthetic@example.invalid",
            "commit",
            "-m",
            "synthetic base",
        )
        self.revision = self._git(
            self.source_repo,
            "rev-parse",
            "HEAD",
        ).stdout.strip()

        self.synthetic_episodes = tuple(
            (episode_id, self.revision, evaluation_scope, cheap_exit_target)
            for (
                episode_id,
                _,
                evaluation_scope,
                cheap_exit_target,
            ) in REAL_CANONICAL_EPISODES
        )
        self.tasks = self.base / "tasks"
        self.tasks.mkdir()
        for episode_id, _, _, _ in self.synthetic_episodes:
            task_dir = self.tasks / episode_id
            task_dir.mkdir()
            (task_dir / "task.md").write_text(
                "# Fixture metadata\n\n"
                "## Prompt-safe task\n\n"
                f"Locate the {episode_id} configuration handoff and focused test.\n\n"
                "## Gold\n\nThis must never enter the frozen task.\n",
                encoding="utf-8",
            )

        self.binary = self.base / "candidate-repomap"
        self.binary.write_text(FAKE_CLI, encoding="utf-8")
        self.binary.chmod(self.binary.stat().st_mode | stat.S_IXUSR)
        self.invocations = self.base / "invocations.jsonl"

        self.manifest_path = self.base / "DEVELOPMENT_SET.json"
        self._write_manifest(self.manifest_path)

        self.v01_binary_sha = "0" * 64
        self.v01_route_plan = {
            "version": 1,
            "kind": "task_lens_v01_frozen_route_plan",
            "selection_source": "explicit_post_calibration",
            "zero_call_episode_ids": [
                harness.V01_ZERO_CALL_PRIMARY,
                harness.V01_CALIBRATION_EPISODE,
            ],
            "offline_partial_episode_ids": [
                item[0]
                for item in self.synthetic_episodes
                if item[0] != harness.V01_ZERO_CALL_PRIMARY
            ],
        }
        self.v01_route_sha = harness.sha256_bytes(
            harness.canonical_json(self.v01_route_plan)
        )
        self.v01_config_path = self.base / "V01_FINAL_RUN_CONFIG.json"
        self._write_v01_config(self.v01_config_path)
        self.v01_config_sha = harness.sha256_file(self.v01_config_path)

        self.patchers = [
            mock.patch.object(
                harness,
                "CANONICAL_EPISODES",
                self.synthetic_episodes,
            ),
            mock.patch.object(
                harness,
                "V01_BINARY_SHA256",
                self.v01_binary_sha,
            ),
            mock.patch.object(
                harness,
                "V01_ROUTE_PLAN_SHA256",
                self.v01_route_sha,
            ),
            mock.patch.object(
                harness,
                "V01_CONFIG_SHA256",
                self.v01_config_sha,
            ),
        ]
        for patcher in self.patchers:
            patcher.start()

    def tearDown(self) -> None:
        for patcher in reversed(self.patchers):
            patcher.stop()
        self.temporary.cleanup()

    @staticmethod
    def _git(repo: Path, *args: str) -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            ["git", "-C", str(repo), *args],
            check=True,
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
        )

    def _write_manifest(self, path: Path) -> None:
        path.write_text(
            json.dumps(
                {
                    "version": 2,
                    "kind": "task_lens_v02_known_development",
                    "fresh_generalization": False,
                    "primary_episode_count": 6,
                    "cheap_exit_calibration_count": 0,
                    "episodes": [
                        {
                            "episode_id": episode_id,
                            "base_revision": revision,
                            "evaluation_scope": evaluation_scope,
                            "cheap_exit_target": cheap_exit_target,
                            "final_attempt": (
                                f"episodes/{episode_id}/final/attempt"
                            ),
                        }
                        for (
                            episode_id,
                            revision,
                            evaluation_scope,
                            cheap_exit_target,
                        ) in self.synthetic_episodes
                    ],
                    "constraints": harness.CANONICAL_CONSTRAINTS,
                },
                indent=2,
                sort_keys=True,
            )
            + "\n",
            encoding="utf-8",
        )

    def _write_v01_config(self, path: Path) -> None:
        path.write_text(
            json.dumps(
                {
                    "version": 1,
                    "kind": "task_lens_v01_final_run_config",
                    "binary_sha256": self.v01_binary_sha,
                    "route_plan": self.v01_route_plan,
                    "route_plan_sha256": self.v01_route_sha,
                },
                indent=2,
                sort_keys=True,
            )
            + "\n",
            encoding="utf-8",
        )

    def _prepare(self, name: str = "run") -> tuple[Path, dict, dict[str, dict]]:
        root = self.base / name
        harness.prepare_experiment(
            root,
            self.manifest_path,
            self.source_repo,
            self.tasks,
            self.binary,
            self.v01_config_path,
        )
        config, episodes = harness.load_config(root)
        return root, config, episodes

    def test_manifest_rejects_v01_kind_or_calibration_episode(self) -> None:
        value = json.loads(self.manifest_path.read_text(encoding="utf-8"))
        value["kind"] = "task_lens_v01_known_development"
        invalid_kind = self.base / "invalid-kind.json"
        invalid_kind.write_text(json.dumps(value), encoding="utf-8")
        with self.assertRaisesRegex(harness.HarnessError, "v0.2.*header"):
            harness.load_manifest(invalid_kind)

        value["kind"] = harness.MANIFEST_KIND
        value["episodes"].append(
            {
                "episode_id": harness.V01_CALIBRATION_EPISODE,
                "base_revision": self.revision,
                "evaluation_scope": "cheap_exit_only",
                "cheap_exit_target": True,
                "final_attempt": (
                    "episodes/openapi_disable_messages_config/final/attempt"
                ),
            }
        )
        value["cheap_exit_calibration_count"] = 1
        invalid_calibration = self.base / "invalid-calibration.json"
        invalid_calibration.write_text(json.dumps(value), encoding="utf-8")
        with self.assertRaisesRegex(harness.HarnessError, "v0.2.*header"):
            harness.load_manifest(invalid_calibration)

    def test_task_source_is_only_episode_task_and_holdout_is_rejected(self) -> None:
        episode_id = self.synthetic_episodes[0][0]
        expected = (self.tasks / episode_id / "task.md").resolve()
        self.assertEqual(
            harness.find_task_source(self.tasks, episode_id),
            expected,
        )

        (self.tasks / "flat_only.md").write_text("flat task\n", encoding="utf-8")
        with self.assertRaisesRegex(harness.HarnessError, "exactly.*task.md"):
            harness.find_task_source(self.tasks, "flat_only")

        holdout_root = self.base / "holdout" / "tasks"
        holdout_task = holdout_root / episode_id
        holdout_task.mkdir(parents=True)
        (holdout_task / "task.md").write_text("forbidden\n", encoding="utf-8")
        with self.assertRaisesRegex(harness.HarnessError, "holdout"):
            harness.find_task_source(holdout_root, episode_id)

    def test_prepare_freezes_exact_route_and_self_contained_source(self) -> None:
        fsmonitor_marker = self.base / "source-fsmonitor-ran"
        fsmonitor = self.base / "source-fsmonitor.sh"
        fsmonitor.write_text(
            "#!/bin/sh\n"
            f"printf ran > {fsmonitor_marker}\n"
            "exit 0\n",
            encoding="utf-8",
        )
        fsmonitor.chmod(0o755)
        self._git(
            self.source_repo,
            "config",
            "core.fsmonitor",
            str(fsmonitor),
        )
        hook_marker = self.base / "source-hook-ran"
        hook = self.source_repo / ".git" / "hooks" / "post-checkout"
        hook.write_text(
            "#!/bin/sh\n"
            f"printf ran > {hook_marker}\n",
            encoding="utf-8",
        )
        hook.chmod(0o755)

        root, config, episodes = self._prepare()
        self.assertEqual(config["kind"], harness.CONFIG_KIND)
        self.assertEqual(config["version"], 2)
        self.assertEqual(config["source_repository_path"], "inputs/source.git")
        self.assertEqual(
            config["route_plan"]["selection_source"],
            "frozen_v01_same_six_partition",
        )
        self.assertEqual(
            config["route_plan"]["zero_call_episode_ids"],
            [harness.V01_ZERO_CALL_PRIMARY],
        )
        self.assertEqual(
            config["route_plan"]["offline_partial_episode_ids"],
            [
                item[0]
                for item in self.synthetic_episodes
                if item[0] != harness.V01_ZERO_CALL_PRIMARY
            ],
        )
        self.assertEqual(len(episodes), 6)
        self.assertEqual(
            episodes[harness.V01_ZERO_CALL_PRIMARY]["run_mode"],
            "zero_call",
        )
        self.assertTrue(
            all(
                episode["run_mode"] == "offline_partial"
                for episode_id, episode in episodes.items()
                if episode_id != harness.V01_ZERO_CALL_PRIMARY
            )
        )
        self.assertFalse(fsmonitor_marker.exists())
        self.assertFalse(hook_marker.exists())

        source_store = root / "inputs" / "source.git"
        self.assertEqual(
            harness.git_output(source_store, "rev-parse", "--is-bare-repository"),
            "true",
        )
        self.assertEqual(harness.git_output(source_store, "remote"), "")
        self.assertFalse(
            (source_store / "objects" / "info" / "alternates").exists()
        )
        worktree = (
            root
            / "episodes"
            / self.synthetic_episodes[0][0]
            / "worktree"
            / "repo"
        )
        git_pointer = (worktree / ".git").read_text(encoding="utf-8")
        self.assertIn(str(source_store), git_pointer)
        self.assertNotIn(str(self.source_repo / ".git"), git_pointer)

        # Install malicious local Git integration after preparation.  Verify
        # must still override it rather than executing target-owned helpers.
        frozen_fsmonitor_marker = self.base / "frozen-fsmonitor-ran"
        frozen_fsmonitor = self.base / "frozen-fsmonitor.sh"
        frozen_fsmonitor.write_text(
            "#!/bin/sh\n"
            f"printf ran > {frozen_fsmonitor_marker}\n"
            "exit 0\n",
            encoding="utf-8",
        )
        frozen_fsmonitor.chmod(0o755)
        self._git(
            worktree,
            "config",
            "core.fsmonitor",
            str(frozen_fsmonitor),
        )
        harness.verify_worktree(
            worktree,
            episodes[self.synthetic_episodes[0][0]]["base_revision"],
            episodes[self.synthetic_episodes[0][0]]["tree_hash"],
        )
        self.assertFalse(frozen_fsmonitor_marker.exists())

        moved_source = self.base / "source-moved-after-freeze"
        self.source_repo.rename(moved_source)
        reloaded, reloaded_episodes = harness.load_config(root)
        self.assertEqual(reloaded["source_repository_path"], "inputs/source.git")
        harness.verify_prepared_episode(
            root,
            reloaded_episodes[self.synthetic_episodes[-1][0]],
        )

    def test_candidate_must_differ_from_v01_binary(self) -> None:
        with mock.patch.object(
            harness,
            "V01_BINARY_SHA256",
            harness.sha256_file(self.binary),
        ):
            with self.assertRaisesRegex(harness.HarnessError, "byte-identical"):
                harness.prepare_experiment(
                    self.base / "same-binary",
                    self.manifest_path,
                    self.source_repo,
                    self.tasks,
                    self.binary,
                    self.v01_config_path,
                )

    def test_tampered_v01_config_is_rejected_before_staging(self) -> None:
        value = json.loads(self.v01_config_path.read_text(encoding="utf-8"))
        value["route_plan"]["zero_call_episode_ids"] = [
            harness.V01_ZERO_CALL_PRIMARY
        ]
        self.v01_config_path.write_text(json.dumps(value), encoding="utf-8")
        with self.assertRaisesRegex(harness.HarnessError, "pinned baseline"):
            harness.prepare_experiment(
                self.base / "tampered-route",
                self.manifest_path,
                self.source_repo,
                self.tasks,
                self.binary,
                self.v01_config_path,
            )
        self.assertFalse((self.base / "tampered-route").exists())

    def test_zero_call_attempt_and_seal_use_v02_kinds(self) -> None:
        root, config, episodes = self._prepare()
        episode = episodes[harness.V01_ZERO_CALL_PRIMARY]
        with mock.patch.dict(
            os.environ,
            {"FAKE_INVOCATIONS": str(self.invocations)},
            clear=False,
        ):
            seal = harness.run_episode(
                root,
                config,
                episode,
                timeout_seconds=30,
            )
            verified = harness.verify_seal(root, config, episode)

        attempt_dir = (
            root
            / "episodes"
            / harness.V01_ZERO_CALL_PRIMARY
            / "final"
            / "attempt"
        )
        started = json.loads(
            (attempt_dir / "ATTEMPT_STARTED.json").read_text(encoding="utf-8")
        )
        completed = json.loads(
            (attempt_dir / "HARNESS_ATTEMPT.json").read_text(encoding="utf-8")
        )
        self.assertEqual(started["version"], 2)
        self.assertEqual(completed["version"], 2)
        self.assertTrue(started["git_environment_isolated"])
        self.assertEqual(seal["kind"], harness.ATTEMPT_SEAL_KIND)
        self.assertEqual(seal["version"], 2)
        self.assertTrue(seal["git_environment_isolated"])
        self.assertEqual(verified, seal)
        invocations = [
            json.loads(line)
            for line in self.invocations.read_text(encoding="utf-8").splitlines()
        ]
        self.assertEqual(len(invocations), 1)
        self.assertNotIn("--offline", invocations[0])


if __name__ == "__main__":
    unittest.main()
