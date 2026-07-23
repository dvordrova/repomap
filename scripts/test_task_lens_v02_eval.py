#!/usr/bin/env python3
"""Focused tests for the Task Lens v0.2 evaluator/report boundary."""

from __future__ import annotations

import json
import sys
import tempfile
import unittest
from pathlib import Path
from unittest import mock

sys.dont_write_bytecode = True

import task_lens_v02_eval as evaluator


class ScoreFixture:
    @staticmethod
    def document() -> dict[str, object]:
        value = evaluator.scores_template()
        value["recommended_next_experiment"] = "prepare one untouched holdout"
        for episode in value["episodes"]:  # type: ignore[index]
            episode["scores"] = {  # type: ignore[index]
                dimension: 4 for dimension in evaluator.SCORE_DIMENSIONS
            }
            episode["decisive_key_roles_present"] = True  # type: ignore[index]
            episode["gold_loss_stage"] = "never_generated"  # type: ignore[index]
            episode["gold_loss_detail"] = "no exact gold identity was generated"  # type: ignore[index]
            episode["useful"] = True  # type: ignore[index]
        return value

    @staticmethod
    def write(path: Path, value: dict[str, object]) -> None:
        path.write_text(json.dumps(value, indent=2) + "\n", encoding="utf-8")


class EpisodeAndKindContractTest(unittest.TestCase):
    def test_v02_contains_exactly_six_primary_episodes(self) -> None:
        self.assertEqual(len(evaluator.EPISODES), 6)
        self.assertEqual(set(evaluator.PRIMARY_IDS), set(evaluator.EPISODE_BY_ID))
        self.assertNotIn("openapi_disable_messages_config", evaluator.PRIMARY_IDS)

    def test_v01_score_kind_is_rejected(self) -> None:
        value = ScoreFixture.document()
        value["kind"] = "task_lens_v01_supervisor_scores"
        with tempfile.TemporaryDirectory() as temporary:
            path = Path(temporary) / "scores.json"
            ScoreFixture.write(path, value)
            with self.assertRaisesRegex(evaluator.EvaluationError, "v0.1 score kinds"):
                evaluator.load_scores(path)

    def test_seventh_calibration_score_is_rejected(self) -> None:
        value = ScoreFixture.document()
        value["episodes"].append(  # type: ignore[union-attr]
            {
                "id": "openapi_disable_messages_config",
                "evaluation_scope": "cheap_exit_only",
                "scores": {},
            }
        )
        with tempfile.TemporaryDirectory() as temporary:
            path = Path(temporary) / "scores.json"
            ScoreFixture.write(path, value)
            with self.assertRaisesRegex(evaluator.EvaluationError, "exactly six"):
                evaluator.load_scores(path)

    def test_valid_v02_scores_load(self) -> None:
        value = ScoreFixture.document()
        with tempfile.TemporaryDirectory() as temporary:
            path = Path(temporary) / "scores.json"
            ScoreFixture.write(path, value)
            loaded = evaluator.load_scores(path)
        self.assertEqual(loaded["kind"], "task_lens_v02_supervisor_scores")


class BaselineAllowlistTest(unittest.TestCase):
    def test_only_primary_attempt_closure_is_allowed(self) -> None:
        episode_id = evaluator.PRIMARY_IDS[0]
        self.assertTrue(
            evaluator._baseline_path_allowed(
                f"episodes/{episode_id}/final/attempt/run/run-1/retrieval_trace.json"
            )
        )
        self.assertTrue(
            evaluator._baseline_path_allowed(f"episodes/{episode_id}/task.md")
        )
        self.assertFalse(
            evaluator._baseline_path_allowed(
                f"episodes/{episode_id}/worktree/repo/openapi.go"
            )
        )
        self.assertFalse(
            evaluator._baseline_path_allowed("screenshots/review-desktop.jpg")
        )
        self.assertFalse(
            evaluator._baseline_path_allowed("baseline/holdout/HOLDOUT_SEAL.json")
        )
        self.assertFalse(
            evaluator._baseline_path_allowed(
                "episodes/openapi_disable_messages_config/final/attempt/SEALED.json"
            )
        )
        self.assertFalse(
            evaluator._baseline_path_allowed(
                f"episodes/{episode_id}/final/attempt/baseline/holdout/result.json"
            )
        )


class ThresholdTest(unittest.TestCase):
    @staticmethod
    def _entries() -> tuple[
        dict[str, dict[str, object]],
        dict[str, dict[str, object]],
        dict[str, dict[str, object]],
    ]:
        score_document = ScoreFixture.document()
        scores = {
            entry["id"]: entry  # type: ignore[index]
            for entry in score_document["episodes"]  # type: ignore[index]
        }
        baseline: dict[str, dict[str, object]] = {}
        validation: dict[str, dict[str, object]] = {}
        for episode_id in evaluator.PRIMARY_IDS:
            before = {dimension: 4 for dimension in evaluator.SCORE_DIMENSIONS}
            if episode_id in evaluator.IMPROVEMENT_TARGET_IDS:
                before["verification_usefulness"] = 2
            baseline[episode_id] = {"scores": before}
            zero = episode_id in evaluator.ZERO_CALL_BASELINE_IDS
            validation[episode_id] = {
                "artifact_valid": True,
                "provider": {"calls": 0},
                "offline": not zero,
                "status": {"sufficient": zero},
                "cheap_exit": {
                    "eligible": zero,
                    "route": "zero_call" if zero else "single_synthesis_call",
                },
            }
        return scores, validation, baseline

    def test_declared_v02_gates_pass(self) -> None:
        scores, validation, baseline = self._entries()
        thresholds = evaluator.compute_thresholds(scores, validation, baseline)
        self.assertTrue(all(gate["passed"] for gate in thresholds.values()))
        self.assertEqual(
            thresholds["verification_usefulness_at_least_3"]["denominator"],
            6,
        )
        self.assertEqual(
            thresholds["useful_zero_call_same_six"]["denominator"],
            1,
        )

    def test_verification_four_of_six_is_fixed(self) -> None:
        scores, validation, baseline = self._entries()
        for episode_id in evaluator.PRIMARY_IDS[:3]:
            scores[episode_id]["scores"]["verification_usefulness"] = 2  # type: ignore[index]
            baseline[episode_id]["scores"]["verification_usefulness"] = 2  # type: ignore[index]
        thresholds = evaluator.compute_thresholds(scores, validation, baseline)
        gate = thresholds["verification_usefulness_at_least_3"]
        self.assertEqual(gate["count"], 3)
        self.assertFalse(gate["passed"])

    def test_existing_score_at_or_above_three_cannot_regress(self) -> None:
        scores, validation, baseline = self._entries()
        episode_id = evaluator.PRIMARY_IDS[0]
        scores[episode_id]["scores"]["subsystem_localization"] = 3  # type: ignore[index]
        thresholds = evaluator.compute_thresholds(scores, validation, baseline)
        gate = thresholds["no_v01_score_regression_at_or_above_3"]
        self.assertFalse(gate["passed"])
        self.assertEqual(gate["regressions"][0]["dimension"], "subsystem_localization")

    def test_false_exact_anchor_fails_independently(self) -> None:
        scores, validation, baseline = self._entries()
        scores[evaluator.PRIMARY_IDS[0]][
            "false_exact_or_unrelated_verification_anchors"
        ] = ["unrelated TestFoo"]
        thresholds = evaluator.compute_thresholds(scores, validation, baseline)
        self.assertFalse(
            thresholds["false_exact_or_unrelated_verification_anchors"]["passed"]
        )

    def test_same_six_zero_call_must_remain_real(self) -> None:
        scores, validation, baseline = self._entries()
        episode_id = evaluator.ZERO_CALL_BASELINE_IDS[0]
        validation[episode_id]["offline"] = True
        thresholds = evaluator.compute_thresholds(scores, validation, baseline)
        self.assertFalse(thresholds["useful_zero_call_same_six"]["passed"])


class FrozenBudgetTest(unittest.TestCase):
    @staticmethod
    def _errors(root: Path, budgets: dict[str, int]) -> list[str]:
        run_dir = root / "episodes" / "fixture" / "run"
        run_dir.mkdir(parents=True)
        trace = {
            "budgets": budgets,
            "candidates_before_ranking": [],
            "selected_anchors": [],
            "verification_frontier": {"anchors": []},
        }
        (run_dir / "retrieval_trace.json").write_text(
            json.dumps(trace),
            encoding="utf-8",
        )
        return evaluator._validate_frozen_budgets(
            root,
            {"id": "fixture", "run_dir": "episodes/fixture/run"},
        )

    def test_frozen_budget_increase_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            errors = self._errors(
                root,
                {
                    **evaluator.FROZEN_BUDGETS,
                    "initial_candidates": 41,
                },
            )
        self.assertTrue(any("initial_candidates" in error for error in errors))

    def test_smaller_usage_and_omitted_zero_scan_are_accepted(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            budgets = {
                name: max(0, value - 1)
                for name, value in evaluator.FROZEN_BUDGETS.items()
                if name != "source_scan_bytes"
            }
            errors = self._errors(
                root,
                budgets,
            )
        self.assertEqual(errors, [])


class SupervisorContractTest(unittest.TestCase):
    recommendation = "prepare one untouched holdout"

    def _text(self) -> str:
        opening = [
            f"{label} {self.recommendation if index == 9 else 'value'}"
            for index, label in enumerate(evaluator.SUPERVISOR_OPENING_LABELS)
        ]
        audit = "\n".join(
            f"### {episode_id}\n\n- evidence\n"
            for episode_id in evaluator.PRIMARY_IDS
        )
        return (
            "\n".join(opening)
            + "\n\n# Task Lens v0.2 supervisor report\n\n"
            + audit
        )

    def test_v02_supervisor_is_accepted(self) -> None:
        evaluator._validate_supervisor_text(self._text(), self.recommendation)

    def test_stale_v01_supervisor_title_is_rejected(self) -> None:
        value = self._text().replace(
            "# Task Lens v0.2 supervisor report",
            "# Task Lens v0.1 supervisor report",
        )
        with self.assertRaisesRegex(evaluator.EvaluationError, "stale supervisor"):
            evaluator._validate_supervisor_text(value, self.recommendation)


class ReportSealTest(unittest.TestCase):
    recommendation = "prepare one untouched holdout"

    @staticmethod
    def _supervisor(recommendation: str) -> str:
        opening = [
            f"{label} {recommendation if index == 9 else 'value'}"
            for index, label in enumerate(evaluator.SUPERVISOR_OPENING_LABELS)
        ]
        audit = "\n".join(
            f"### {episode_id}\n\n- evidence\n"
            for episode_id in evaluator.PRIMARY_IDS
        )
        return "\n".join(opening) + "\n\n# Task Lens v0.2 supervisor report\n\n" + audit

    def _root(self, root: Path) -> None:
        evaluation = root / evaluator.EVALUATION_DIR_NAME
        evaluation.mkdir(parents=True)
        (evaluation / "EVALUATION_SEAL.json").write_text("{}\n", encoding="utf-8")
        (root / evaluator.BASELINE_MANIFEST_NAME).write_text("{}\n", encoding="utf-8")
        for relative in evaluator.REQUIRED_RENDERED_REPORTS:
            path = root / relative
            path.parent.mkdir(parents=True, exist_ok=True)
            content = (
                self._supervisor(self.recommendation)
                if relative == "SUPERVISOR_REPORT.md"
                else f"{relative}\n"
            )
            path.write_text(content, encoding="utf-8")
        reports = [
            {
                "path": relative,
                "bytes": (root / relative).stat().st_size,
                "sha256": evaluator.sha256_file(root / relative),
            }
            for relative in sorted(evaluator.REQUIRED_RENDERED_REPORTS)
        ]
        seal = {
            "version": 1,
            "kind": "task_lens_v02_report_seal",
            "evaluation_seal": {
                "path": "evaluation-v02/EVALUATION_SEAL.json",
                "sha256": evaluator.sha256_file(
                    evaluation / "EVALUATION_SEAL.json"
                ),
            },
            "baseline_manifest": {
                "path": evaluator.BASELINE_MANIFEST_NAME,
                "sha256": evaluator.sha256_file(
                    root / evaluator.BASELINE_MANIFEST_NAME
                ),
            },
            "reports": reports,
        }
        evaluator.write_json(root / evaluator.REPORT_SEAL_NAME, seal)
        evaluator.write_text(
            root / evaluator.REPORT_SEAL_SIDECAR,
            (
                f"{evaluator.sha256_file(root / evaluator.REPORT_SEAL_NAME)}  "
                f"{evaluator.REPORT_SEAL_NAME}"
            ),
        )

    def test_report_seal_binds_evaluation_baseline_and_reports(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            self._root(root)
            baseline_sha = evaluator.sha256_file(
                root / evaluator.BASELINE_MANIFEST_NAME
            )
            with (
                mock.patch.object(
                    evaluator,
                    "verify_evaluation_seal",
                    return_value={
                        "result": {
                            "recommended_next_experiment": self.recommendation
                        }
                    },
                ),
                mock.patch.object(
                    evaluator,
                    "verify_baseline",
                    return_value={"manifest_sha256": baseline_sha},
                ),
            ):
                verified = evaluator.verify_report_seal(root)
            self.assertEqual(verified["reports_verified"], 10)

    def test_report_tamper_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            self._root(root)
            (root / "PRODUCT_FINDINGS.md").write_text(
                "tampered\n",
                encoding="utf-8",
            )
            with (
                mock.patch.object(
                    evaluator,
                    "verify_evaluation_seal",
                    return_value={
                        "result": {
                            "recommended_next_experiment": self.recommendation
                        }
                    },
                ),
                mock.patch.object(
                    evaluator,
                    "verify_baseline",
                    return_value={"manifest_sha256": "unused"},
                ),
            ):
                with self.assertRaisesRegex(evaluator.EvaluationError, "mismatch"):
                    evaluator.verify_report_seal(root)


if __name__ == "__main__":
    unittest.main()
