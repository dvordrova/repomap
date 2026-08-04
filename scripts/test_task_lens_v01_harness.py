#!/usr/bin/env python3
"""Synthetic integration tests for the one-shot Task Lens v0.1 harness."""

from __future__ import annotations

import json
import os
import stat
import subprocess
import sys
import tempfile
import types
import unittest
from pathlib import Path
from unittest import mock


sys.path.insert(0, str(Path(__file__).resolve().parent))

from task_lens_v01_harness import (  # noqa: E402
    HarnessError,
    command_run,
    find_task_source,
    final_attempt_env,
    frozen_route_plan,
    load_config,
    prepare_experiment,
    run_episode,
    verify_seal,
)
from task_lens_v01_eval import EpisodeSpec, validate_episode  # noqa: E402


FAKE_CLI = r'''#!/usr/bin/env python3
import hashlib
import json
import os
import subprocess
import sys
from pathlib import Path


def write_json(path, value):
    path.write_text(
        json.dumps(value, ensure_ascii=False, separators=(",", ":")),
        encoding="utf-8",
    )


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def bundle_digest(value):
    raw = json.dumps(value, ensure_ascii=False, separators=(",", ":"))
    raw = raw.replace("&", "\\u0026").replace("<", "\\u003c").replace(">", "\\u003e")
    raw = raw.replace("\u2028", "\\u2028").replace("\u2029", "\\u2029")
    return hashlib.sha256(raw.encode("utf-8")).hexdigest()


counter = Path(os.environ["FAKE_INVOCATIONS"])
with counter.open("a", encoding="utf-8") as output:
    output.write(json.dumps(sys.argv[1:]) + "\n")

if len(sys.argv) < 3 or sys.argv[1] != "investigate":
    raise SystemExit(2)
repo = Path(sys.argv[2])
task_path = Path(sys.argv[sys.argv.index("--task-file") + 1])
debug_root = Path(sys.argv[sys.argv.index("--debug-dir") + 1])
offline = "--offline" in sys.argv
fail_episode = os.environ.get("FAKE_FAIL_EPISODE")
if fail_episode and fail_episode in str(task_path):
    raise SystemExit(9)
revision = subprocess.check_output(
    ["git", "-C", str(repo), "rev-parse", "HEAD"], text=True
).strip()
tree_hash = subprocess.check_output(
    ["git", "-C", str(repo), "rev-parse", "HEAD^{tree}"], text=True
).strip()

run_id = "synthetic-run"
run_dir = debug_root / run_id
run_dir.mkdir(parents=True)
(debug_root / "latest").symlink_to(run_id)

scope = {
    "scope_kind": "complete_enclosing_symbol",
    "scope_start": 1,
    "scope_end": 2,
    "source_total_lines": 2,
    "truncated": False,
    "truncation_reason": "",
    "task_matches_outside_window": False,
    "negative_claims_allowed": True,
    "negative_evidence_basis": "complete_scope",
}
role_contract = {
    "profile": "configuration_propagation_bug",
    "key": [{"role": "config_copy", "minimum_anchors": 1}],
    "supporting": [],
    "optional": [],
}
role_coverage = {
    "profile": "configuration_propagation_bug",
    "key": [
        {
            "role": "config_copy",
            "minimum_anchors": 1,
            "represented": not offline,
            "anchor_ids": ["anchor-1"] if not offline else [],
        }
    ],
    "supporting": [],
    "optional": [],
}
frontier = {
    "decisive_anchor_id": "anchor-1" if not offline else "",
    "anchors": [
        {
            "id": "verify-1",
            "authority": "exact_existing_test",
            "anchor_id": "anchor-1",
            "path": "main_test.go",
            "symbol": "TestSynthetic",
            "text": "the focused test observes the effect",
            "evidence_ids": ["evidence-1"],
        }
    ] if not offline else [],
}
gate_names = [
    "unambiguous_area",
    "all_key_roles",
    "decisive_locally_observed_relation",
    "exact_verification_anchor_or_effect",
    "no_unresolved_competing_hypothesis",
]
gates = []
for index, name in enumerate(gate_names):
    passed = not offline or index != 1
    gates.append({"gate": name, "passed": passed, "reason": "synthetic reason"})
cheap_exit = {
    "eligible": not offline,
    "route": "single_synthesis_call" if offline else "zero_call",
    "gates": gates,
    "reasons": ["missing key role"] if offline else ["all deterministic cheap-exit gates passed"],
}
provider = {
    "calls": 0,
    "transport_attempts": 0,
    "request_bytes": 0,
    "response_bytes": 0,
    "input_tokens": 0,
    "output_tokens": 0,
    "prompt_cache_hit_tokens": 0,
    "prompt_cache_miss_tokens": 0,
    "latency_millis": 0,
}
bundle = {
    "version": 2,
    "id": "task-synthetic",
    "repository": {"revision": revision, "tree_hash": tree_hash},
    "task": {"text": task_path.read_text(encoding="utf-8")},
    "anchors": [{"id": "anchor-1", "source_scope": scope}],
    "task_profile": "configuration_propagation_bug",
    "role_contract": role_contract,
    "role_coverage": role_coverage,
    "verification_frontier": frontier,
    "decisive_relation_id": "relation-1",
    "cheap_exit": cheap_exit,
    "budgets": {"max_anchors": 8},
    "metrics": {"candidate_count": 1},
    "stages_skipped": ["synthesis"] if not offline else [],
}
bundle_sha = bundle_digest(bundle)
attempt = {
    "version": 2,
    "bundle_sha256": bundle_sha,
    "prompt_version": "task-investigation-pack-json-v2",
    "state": "skipped_offline" if offline else "skipped_local_complete",
    "provider": provider,
}
pack = {
    "version": 2,
    "id": bundle["id"],
    "bundle_sha256": bundle_sha,
    "repository": {"revision": revision, "tree_hash": tree_hash},
    "task_profile": "configuration_propagation_bug",
    "role_contract": role_contract,
    "role_coverage": role_coverage,
    "verification_frontier": frontier,
    "decisive_relation_id": "relation-1",
    "cheap_exit": cheap_exit,
    "investigation_anchors": [{"id": "anchor-1"}],
    "working_hypothesis": [],
    "likely_areas": [],
    "evidence_joins": [],
}
trace = {
    "version": 1,
    "task_kind": "configuration",
    "task_profile": "configuration_propagation_bug",
    "task_terms": [
        {"text": "configuration", "normalized": "configuration", "found": True, "weight": 4},
        {"text": "test", "normalized": "test", "found": True, "weight": 4},
    ],
    "candidates_before_ranking": [
        {
            "id": "anchor-1",
            "stage": "initial",
            "discovery_order": 1,
            "path": "main.go",
            "symbol": "Value",
            "roles": ["config_copy"],
            "score": 10,
            "score_components": [
                {"kind": "direct_task_term_match", "value": 4},
                {"kind": "missing_role_fit", "value": 6},
            ],
        },
        {
            "id": "candidate-2",
            "stage": "initial",
            "discovery_order": 2,
            "path": "main.go",
            "symbol": "Adjacent",
            "roles": [],
            "score": 1,
            "score_components": [{"kind": "production_relevance", "value": 1}],
        },
    ],
    "relationships": [
        {
            "id": "relation-1",
            "left_candidate_id": "anchor-1",
            "right_candidate_id": "candidate-2",
            "kind": "config_applied",
            "support_type": "locally_observed",
            "evidence_ids": ["evidence-1"],
            "scope": "complete symbol",
            "non_guarantees": "does not establish runtime reachability",
        }
    ],
    "selected_anchors": [
        {
            "candidate_id": "anchor-1",
            "anchor_id": "anchor-1",
            "rank": 1,
            "reason": "reserved for a missing key role",
        }
    ],
    "dropped_anchors": [
        {"candidate_id": "candidate-2", "reason": "lower missing-role fit"}
    ],
    "source_scopes": [{"anchor_id": "anchor-1", "scope": scope}],
    "role_coverage": role_coverage,
    "verification_frontier": frontier,
    "budgets": {"frontier_expansions": 1},
    "limits": [
        {
            "name": "anchor_limit",
            "limit": 16,
            "observed": 2,
            "applied": False,
            "caused_loss": False,
        }
    ],
}
if os.environ.get("FAKE_TRACE_GOLD") == "1":
    trace["gold_assessment"] = {"disposition": "present_before_ranking"}

write_json(run_dir / "task_investigation_bundle.json", bundle)
write_json(run_dir / "task_investigation_attempt.json", attempt)
write_json(run_dir / "task_investigation.json", pack)
write_json(run_dir / "retrieval_trace.json", trace)
(run_dir / "retrieval_trace.md").write_text("# Synthetic retrieval trace\n", encoding="utf-8")
write_json(
    run_dir / "metadata.json",
    {
        "run_id": run_id,
        "command": "investigate",
        "model": "deepseek-v4-flash",
        "endpoint": "https://api.deepseek.com/chat/completions",
        "provider_request_count": 0,
        "effective_options": {
            "offline": offline,
            "no_open": True,
            "no_serve": True,
            "debug_enabled": True,
        },
    },
)
status = {
    "version": 2,
    "task_id": bundle["id"],
    "bundle_sha256": bundle_sha,
    "attempt_sha256": digest(run_dir / "task_investigation_attempt.json"),
    "pack_sha256": digest(run_dir / "task_investigation.json"),
    "retrieval_trace_sha256": digest(run_dir / "retrieval_trace.json"),
    "retrieval_trace_markdown_sha256": digest(run_dir / "retrieval_trace.md"),
    "captured_revision": revision,
    "tree_hash": tree_hash,
    "state": "partial_local" if offline else "accepted_local_complete",
    "sufficient": not offline,
    "provider": provider,
    "cheap_exit": cheap_exit,
}
write_json(run_dir / "task_investigation_status.json", status)
'''


class TaskLensV01HarnessTest(unittest.TestCase):
    def setUp(self) -> None:
        self.temporary = tempfile.TemporaryDirectory(prefix="task-lens-v01-harness-")
        self.base = Path(self.temporary.name)
        self.source_repo = self.base / "source"
        self.source_repo.mkdir()
        self._git("init")
        (self.source_repo / "main.go").write_text(
            "package synthetic\n\nfunc Value() int { return 1 }\n",
            encoding="utf-8",
        )
        self._git("add", "main.go")
        self._git(
            "-c",
            "user.name=Synthetic Test",
            "-c",
            "user.email=synthetic@example.invalid",
            "commit",
            "-m",
            "synthetic base",
        )
        self.revision = self._git("rev-parse", "HEAD").stdout.strip()
        self.tasks = self.base / "tasks"
        self.tasks.mkdir()
        self.binary = self.base / "fake-repomap"
        self.binary.write_text(FAKE_CLI, encoding="utf-8")
        self.binary.chmod(self.binary.stat().st_mode | stat.S_IXUSR)
        self.invocations = self.base / "invocations.jsonl"

    def tearDown(self) -> None:
        self.temporary.cleanup()

    def _git(self, *args: str) -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            ["git", "-C", str(self.source_repo), *args],
            check=True,
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
        )

    def _prepare(
        self,
        *,
        cheap: bool,
        zero_call_route: bool | None = None,
    ) -> tuple[Path, dict, dict]:
        episode_id = "synthetic_zero" if cheap else "synthetic_partial"
        task_source = self.tasks / f"{episode_id}.md"
        task_source.write_text(
            "# Private fixture metadata\n\n"
            "## Prompt-safe task\n\n"
            "Locate the synthetic configuration handoff and its focused test.\n\n"
            "## Gold\n\nThis must never enter task.md.\n",
            encoding="utf-8",
        )
        manifest_path = self.base / "manifest.json"
        manifest_path.write_text(
            json.dumps(
                {
                    "version": 1,
                    "kind": "synthetic_task_lens_manifest",
                    "episodes": [
                        {
                            "episode_id": episode_id,
                            "base_revision": self.revision,
                            "cheap_exit_target": cheap,
                            "final_attempt": f"episodes/{episode_id}/final/attempt",
                        }
                    ],
                }
            ),
            encoding="utf-8",
        )
        root = self.base / ("zero" if cheap else "partial")
        if zero_call_route is None:
            zero_call_route = cheap
        prepare_experiment(
            root,
            manifest_path,
            self.source_repo,
            self.tasks,
            self.binary,
            [episode_id] if zero_call_route else [],
        )
        config, episodes = load_config(root)
        self.assertEqual(
            (root / "episodes" / episode_id / "task.md").read_text(encoding="utf-8"),
            "Locate the synthetic configuration handoff and its focused test.\n",
        )
        return root, config, episodes[episode_id]

    def _environment(self, *, gold: bool = False, fail_episode: str | None = None):
        additions = {"FAKE_INVOCATIONS": str(self.invocations)}
        if gold:
            additions["FAKE_TRACE_GOLD"] = "1"
        if fail_episode is not None:
            additions["FAKE_FAIL_EPISODE"] = fail_episode
        return mock.patch.dict(os.environ, additions, clear=False)

    def _recorded_argv(self) -> list[list[str]]:
        return [
            json.loads(line)
            for line in self.invocations.read_text(encoding="utf-8").splitlines()
        ]

    def test_find_task_source_accepts_preserved_flat_subdirectory_layout(self) -> None:
        holdout = self.tasks / "holdout"
        development = self.tasks / "dev"
        holdout.mkdir()
        development.mkdir()
        holdout_task = holdout / "former_holdout.md"
        development_task = development / "cheap_calibration.md"
        holdout_task.write_text("holdout task\n", encoding="utf-8")
        development_task.write_text("development task\n", encoding="utf-8")

        self.assertEqual(
            find_task_source(self.tasks, "former_holdout"), holdout_task
        )
        self.assertEqual(
            find_task_source(self.tasks, "cheap_calibration"), development_task
        )

    def test_genuine_zero_call_is_non_offline_sealed_and_cannot_retry(self) -> None:
        root, config, episode = self._prepare(cheap=True)
        with self._environment():
            seal = run_episode(root, config, episode, timeout_seconds=30)
            verify_seal(root, config, episode)
            evaluation = validate_episode(
                root,
                EpisodeSpec(
                    episode["episode_id"],
                    episode["base_revision"],
                    cheap_exit_target=True,
                ),
            )
            with self.assertRaisesRegex(HarnessError, "consumed"):
                run_episode(root, config, episode, timeout_seconds=30)

        attempt_dir = root / "episodes" / episode["episode_id"] / "final" / "attempt"
        harness = json.loads((attempt_dir / "HARNESS_ATTEMPT.json").read_text())
        argv = self._recorded_argv()
        self.assertEqual(len(argv), 1)
        self.assertNotIn("--offline", argv[0])
        self.assertFalse(harness["offline"])
        self.assertEqual(harness["command"][-1], "--strict-snapshot")
        self.assertEqual(seal["run_mode"], "zero_call")
        self.assertEqual(seal["provider_calls"], 0)
        self.assertTrue(seal["sufficient"])
        self.assertTrue(evaluation["artifact_valid"], evaluation["errors"])
        self.assertEqual(
            set(seal["projection_sha256"]),
            {
                "role_contract.json",
                "role_coverage.json",
                "verification_frontier.json",
                "cheap_exit_decision.json",
                "source_scopes.json",
            },
        )

    def test_noncheap_episode_is_offline_honest_partial(self) -> None:
        root, config, episode = self._prepare(cheap=False)
        with self._environment():
            seal = run_episode(root, config, episode, timeout_seconds=30)
            verify_seal(root, config, episode)
            evaluation = validate_episode(
                root,
                EpisodeSpec(
                    episode["episode_id"],
                    episode["base_revision"],
                    cheap_exit_target=False,
                ),
            )

        attempt_dir = root / "episodes" / episode["episode_id"] / "final" / "attempt"
        harness = json.loads((attempt_dir / "HARNESS_ATTEMPT.json").read_text())
        metrics = json.loads((attempt_dir / "METRICS.json").read_text())
        self.assertEqual(len(self._recorded_argv()), 1)
        self.assertIn("--offline", self._recorded_argv()[0])
        self.assertTrue(harness["offline"])
        self.assertEqual(harness["command"][-1], "--offline")
        self.assertEqual(seal["run_mode"], "offline_partial")
        self.assertEqual(seal["state"], "partial_local")
        self.assertFalse(seal["sufficient"])
        self.assertEqual(metrics["cheap_exit"]["route"], "single_synthesis_call")
        self.assertTrue(all(value == 0 for value in metrics["provider"].values()))
        self.assertTrue(evaluation["artifact_valid"], evaluation["errors"])

    def test_explicit_route_can_freeze_non_target_as_genuine_zero_call(self) -> None:
        root, config, episode = self._prepare(
            cheap=False,
            zero_call_route=True,
        )
        self.assertFalse(episode["cheap_exit_target"])
        self.assertEqual(episode["run_mode"], "zero_call")
        self.assertEqual(
            config["route_plan"]["selection_source"],
            "explicit_post_calibration",
        )
        self.assertEqual(
            config["route_plan"]["zero_call_episode_ids"],
            [episode["episode_id"]],
        )
        with self._environment():
            seal = run_episode(root, config, episode, timeout_seconds=30)
            verify_seal(root, config, episode)
            evaluation = validate_episode(
                root,
                EpisodeSpec(
                    episode["episode_id"],
                    episode["base_revision"],
                    cheap_exit_target=False,
                ),
            )

        harness_path = (
            root
            / "episodes"
            / episode["episode_id"]
            / "final"
            / "attempt"
            / "HARNESS_ATTEMPT.json"
        )
        harness = json.loads(harness_path.read_text(encoding="utf-8"))
        self.assertNotIn("--offline", self._recorded_argv()[0])
        self.assertFalse(harness["offline"])
        self.assertEqual(
            harness["route_plan_sha256"],
            config["route_plan_sha256"],
        )
        self.assertEqual(seal["route_plan_sha256"], config["route_plan_sha256"])
        self.assertTrue(evaluation["artifact_valid"], evaluation["errors"])

    def test_route_plan_rejects_duplicate_and_unknown_episode_ids(self) -> None:
        manifest = {
            "episodes": [
                {"episode_id": "one", "cheap_exit_target": True},
                {"episode_id": "two", "cheap_exit_target": False},
            ]
        }
        with self.assertRaisesRegex(HarnessError, "duplicate IDs"):
            frozen_route_plan(manifest, ["one", "one"])
        with self.assertRaisesRegex(HarnessError, "unknown IDs: missing"):
            frozen_route_plan(manifest, ["missing"])
        all_offline = frozen_route_plan(manifest, [])
        self.assertEqual(all_offline["zero_call_episode_ids"], [])
        self.assertEqual(all_offline["offline_partial_episode_ids"], ["one", "two"])

    def test_final_attempt_environment_cannot_fall_through_to_provider(self) -> None:
        with mock.patch.dict(
            os.environ,
            {
                "REPOMAP_LLM_ENDPOINT": "https://provider.invalid/chat/completions",
                "REPOMAP_LLM_API_KEY": "must-not-reach-final-process",
                "DEEPSEEK_API_KEY": "must-not-reach-final-process",
                "FAKE_SAFE_MARKER": "preserved",
            },
            clear=False,
        ):
            environment = final_attempt_env()
        self.assertNotIn("REPOMAP_LLM_ENDPOINT", environment)
        self.assertNotIn("REPOMAP_LLM_API_KEY", environment)
        self.assertNotIn("DEEPSEEK_API_KEY", environment)
        self.assertEqual(environment["FAKE_SAFE_MARKER"], "preserved")

    def test_gold_in_raw_trace_consumes_attempt_without_sealing_or_retry(self) -> None:
        root, config, episode = self._prepare(cheap=True)
        with self._environment(gold=True):
            with self.assertRaisesRegex(HarnessError, "evaluation-only key"):
                run_episode(root, config, episode, timeout_seconds=30)
            with self.assertRaisesRegex(HarnessError, "consumed"):
                run_episode(root, config, episode, timeout_seconds=30)

        attempt_dir = root / "episodes" / episode["episode_id"] / "final" / "attempt"
        self.assertEqual(len(self._recorded_argv()), 1)
        self.assertTrue((attempt_dir / "ATTEMPT_STARTED.json").is_file())
        self.assertTrue((attempt_dir / "HARNESS_ATTEMPT.json").is_file())
        self.assertFalse((attempt_dir / "SEALED.json").exists())

    def test_verify_detects_post_seal_file_tampering(self) -> None:
        root, config, episode = self._prepare(cheap=True)
        with self._environment():
            run_episode(root, config, episode, timeout_seconds=30)
        stdout = (
            root
            / "episodes"
            / episode["episode_id"]
            / "final"
            / "attempt"
            / "stdout.txt"
        )
        stdout.chmod(0o644)
        with stdout.open("ab") as output:
            output.write(b"tampered\n")
        with self.assertRaisesRegex(HarnessError, "sealed file changed"):
            verify_seal(root, config, episode)

    def test_run_all_continues_after_failure_without_retrying_consumed_attempt(self) -> None:
        episode_ids = ("first_case", "second_case")
        for episode_id in episode_ids:
            (self.tasks / f"{episode_id}.md").write_text(
                f"Investigate {episode_id} configuration behavior.\n",
                encoding="utf-8",
            )
        manifest_path = self.base / "multi-manifest.json"
        manifest_path.write_text(
            json.dumps(
                {
                    "version": 1,
                    "kind": "synthetic_task_lens_manifest",
                    "episodes": [
                        {
                            "episode_id": episode_id,
                            "base_revision": self.revision,
                            "cheap_exit_target": True,
                            "final_attempt": f"episodes/{episode_id}/final/attempt",
                        }
                        for episode_id in episode_ids
                    ],
                }
            ),
            encoding="utf-8",
        )
        root = self.base / "multi"
        prepare_experiment(
            root,
            manifest_path,
            self.source_repo,
            self.tasks,
            self.binary,
            episode_ids,
        )
        args = types.SimpleNamespace(
            root=str(root),
            episode=None,
            all=True,
            timeout_seconds=30,
        )
        with self._environment(fail_episode="first_case"):
            with self.assertRaisesRegex(HarnessError, "1 failed or unsealed"):
                command_run(args)
            self.assertEqual(len(self._recorded_argv()), 2)
            self.assertFalse(
                (root / "episodes/first_case/final/attempt/SEALED.json").exists()
            )
            self.assertTrue(
                (root / "episodes/second_case/final/attempt/SEALED.json").is_file()
            )
            with self.assertRaisesRegex(HarnessError, "1 failed or unsealed"):
                command_run(args)
            self.assertEqual(len(self._recorded_argv()), 2)


if __name__ == "__main__":
    unittest.main()
