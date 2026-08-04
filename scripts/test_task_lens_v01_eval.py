#!/usr/bin/env python3
"""Focused tests for the standalone Task Lens v0.1 evaluator."""

from __future__ import annotations

import hashlib
import json
import os
import sys
import tempfile
import unittest
from pathlib import Path

sys.dont_write_bytecode = True

from task_lens_v01_eval import (
    Audit,
    CHEAP_EXIT_IDS,
    EPISODES,
    EpisodeSpec,
    EvaluationError,
    PRIMARY_IDS,
    REQUIRED_RENDERED_REPORTS,
    SCORE_DIMENSIONS,
    SUPERVISOR_OPENING_LABELS,
    V0_PROVIDER_ENDPOINT,
    V0_PROVIDER_MODEL,
    _is_offline_partial_context,
    _is_real_cheap_exit,
    _safe_relative_path,
    _validate_cheap_exit,
    _validate_execution_protocol,
    _validate_provider_identity,
    _validate_rendered_reports,
    _validate_source_scope,
    _validate_trace,
    build_gold_loss_ledger,
    compute_thresholds,
    go_compact_json_sha256,
    sha256_file,
    validate_episode,
)


class SourceScopeTest(unittest.TestCase):
    def test_partial_scope_prohibits_negative_claims(self) -> None:
        audit = Audit()
        _validate_source_scope(
            {
                "scope_kind": "partial_window",
                "scope_start": 10,
                "scope_end": 20,
                "source_total_lines": 80,
                "truncated": True,
                "truncation_reason": "per-anchor byte budget",
                "task_matches_outside_window": True,
                "negative_claims_allowed": True,
                "negative_evidence_basis": "",
            },
            "fixture",
            audit,
        )
        self.assertTrue(any("must prohibit negative claims" in item for item in audit.errors))

    def test_complete_scope_accepts_bounded_negative_evidence(self) -> None:
        audit = Audit()
        _validate_source_scope(
            {
                "scope_kind": "complete_file",
                "scope_start": 1,
                "scope_end": 69,
                "source_total_lines": 69,
                "truncated": False,
                "truncation_reason": "",
                "task_matches_outside_window": False,
                "negative_claims_allowed": True,
                "negative_evidence_basis": "complete_scope",
            },
            "fixture",
            audit,
        )
        self.assertEqual(audit.errors, [])


class HashAndPathTest(unittest.TestCase):
    def test_go_hash_uses_encoding_json_html_escaping(self) -> None:
        value = {"text": "a<b&c>"}
        expected = hashlib.sha256(b'{"text":"a\\u003cb\\u0026c\\u003e"}').hexdigest()
        self.assertEqual(go_compact_json_sha256(value), expected)

    def test_inventory_path_rejects_parent_traversal(self) -> None:
        with self.assertRaises(EvaluationError):
            _safe_relative_path("baseline/../secret", Path("MANIFEST.sha256"))


class RenderedReportContractTest(unittest.TestCase):
    recommendation = "run one untouched cross-repository holdout"

    def _files(self, root: Path) -> dict[Path, str]:
        files = {root / relative: "placeholder\n" for relative in REQUIRED_RENDERED_REPORTS}
        opening = [
            f"{label} {self.recommendation if index == 9 else 'value'}"
            for index, label in enumerate(SUPERVISOR_OPENING_LABELS)
        ]
        sections = [
            (
                "The preserved v0 attempts predate `retrieval_trace.json` and "
                "`retrieval_trace.md`."
            ),
            "## What v0 got right",
            "## Root-cause audit",
            *(f"### {episode.episode_id}" for episode in EPISODES),
            "## Source-scope contract",
            "## Completion and verification",
            "## Cheap exit",
            "## Before and after",
            "## Model/resource accounting",
            "## Product decision",
        ]
        files[root / "SUPERVISOR_REPORT.md"] = "\n".join([*opening, "", *sections])
        files[root / "RETRIEVAL_FAILURES.md"] = (
            "The preserved v0 attempts predate `retrieval_trace.json` and "
            "`retrieval_trace.md`.\n"
        )
        artifacts = (
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
        )
        cards = "".join(
            f'<article id="{episode.episode_id}"></article>'
            for episode in EPISODES
        )
        projection_links = "".join(
            (
                f'<a href="../episodes/{episode.episode_id}/final/attempt/{artifact}">'
                f"{artifact}</a>"
            )
            for episode in EPISODES
            for artifact in (
                "role_contract.json",
                "role_coverage.json",
                "verification_frontier.json",
                "cheap_exit_decision.json",
                "source_scopes.json",
            )
        )
        links = "".join(f'<a href="{artifact}">{artifact}</a>' for artifact in artifacts)
        files[root / "review" / "index.html"] = (
            f"<!doctype html>{cards}{links}{projection_links}"
        )
        return files

    def test_exact_ten_line_opening_and_static_review_contract(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            _validate_rendered_reports(root, self._files(root), self.recommendation)

    def test_second_recommended_next_step_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            files = self._files(root)
            files[root / "SUPERVISOR_REPORT.md"] += (
                f"\n{SUPERVISOR_OPENING_LABELS[-1]} do something else\n"
            )
            with self.assertRaises(EvaluationError):
                _validate_rendered_reports(root, files, self.recommendation)

    def test_executable_review_content_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            files = self._files(root)
            files[root / "review" / "index.html"] += "<script>alert(1)</script>"
            with self.assertRaises(EvaluationError):
                _validate_rendered_reports(root, files, self.recommendation)

    def test_missing_v0_trace_provenance_limit_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            files = self._files(root)
            files[root / "RETRIEVAL_FAILURES.md"] = "placeholder\n"
            with self.assertRaises(EvaluationError):
                _validate_rendered_reports(root, files, self.recommendation)


class ThresholdTest(unittest.TestCase):
    def _scores(self) -> dict[str, dict[str, object]]:
        result: dict[str, dict[str, object]] = {}
        for episode in EPISODES:
            result[episode.episode_id] = {
                "id": episode.episode_id,
                "scores": (
                    {dimension: 4 for dimension in SCORE_DIMENSIONS}
                    if episode.episode_id in PRIMARY_IDS
                    else {}
                ),
                "decisive_key_roles_present": True,
                "major_unsupported_claims": [],
                "absence_claims_from_incomplete_scope": [],
                "clipped_before_known_task_match_without_partial_window": [],
                "useful": episode.episode_id in CHEAP_EXIT_IDS,
            }
        return result

    def _validation(self) -> dict[str, dict[str, object]]:
        result: dict[str, dict[str, object]] = {}
        for episode in EPISODES:
            zero = episode.episode_id in CHEAP_EXIT_IDS
            result[episode.episode_id] = {
                "artifact_valid": True,
                "provider": {"calls": 0 if zero else 1},
                "offline": False,
                "status": {"sufficient": zero},
                "cheap_exit": {
                    "eligible": zero,
                    "route": "zero_call" if zero else "single_synthesis_call",
                },
            }
        return result

    def test_fixed_gates_pass_without_an_opaque_average(self) -> None:
        thresholds = compute_thresholds(self._scores(), self._validation())
        self.assertTrue(all(value["passed"] for value in thresholds.values()))
        self.assertEqual(thresholds["must_read_file_recall_at_least_3"]["denominator"], 6)
        self.assertEqual(thresholds["useful_zero_call_packs"]["denominator"], 3)

    def test_calibration_episode_cannot_inflate_primary_denominator(self) -> None:
        scores = self._scores()
        scores[PRIMARY_IDS[0]]["scores"]["must_read_file_recall"] = 2  # type: ignore[index]
        thresholds = compute_thresholds(scores, self._validation())
        gate = thresholds["must_read_file_recall_at_least_3"]
        self.assertEqual(gate["count"], 5)
        self.assertEqual(gate["denominator"], 6)
        self.assertTrue(gate["passed"])

    def test_provider_call_is_required_for_a_counted_cheap_exit(self) -> None:
        validation = self._validation()
        validation[CHEAP_EXIT_IDS[0]]["provider"] = {"calls": 1}
        thresholds = compute_thresholds(self._scores(), validation)
        self.assertEqual(thresholds["useful_zero_call_packs"]["count"], 2)

    def test_eligibility_is_required_for_a_counted_cheap_exit(self) -> None:
        validation = self._validation()
        validation[CHEAP_EXIT_IDS[0]]["cheap_exit"] = {
            "eligible": False,
            "route": "zero_call",
        }
        thresholds = compute_thresholds(self._scores(), validation)
        self.assertEqual(thresholds["useful_zero_call_packs"]["count"], 2)


class ExecutionProtocolTest(unittest.TestCase):
    @staticmethod
    def _zero_provider() -> dict[str, int]:
        return {
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

    @staticmethod
    def _cheap_exit(eligible: bool, route: str) -> dict[str, object]:
        return {
            "eligible": eligible,
            "route": route,
            "gates": [
                {"gate": name, "passed": eligible, "reason": "bounded local evidence"}
                for name in (
                    "unambiguous_area",
                    "all_key_roles",
                    "decisive_locally_observed_relation",
                    "exact_verification_anchor_or_effect",
                    "no_unresolved_competing_hypothesis",
                )
            ],
            "reasons": [] if eligible else ["synthesis was intentionally deferred"],
        }

    def test_exact_offline_partial_allows_deferred_single_synthesis_call(self) -> None:
        provider = self._zero_provider()
        harness = {"offline": True}
        attempt = {"state": "skipped_offline"}
        status = {"state": "partial_local", "sufficient": False}
        cheap_exit = self._cheap_exit(False, "single_synthesis_call")
        self.assertTrue(_is_offline_partial_context(harness, attempt, status, provider))

        audit = Audit()
        validated = _validate_cheap_exit(
            cheap_exit,
            0,
            "fixture",
            audit,
            allow_deferred_single_call=True,
        )
        _validate_execution_protocol(
            harness,
            attempt,
            status,
            provider,
            validated,
            "fixture",
            audit,
        )
        self.assertEqual(audit.errors, [])

    def test_offline_zero_call_decision_is_rejected(self) -> None:
        audit = Audit()
        _validate_execution_protocol(
            {"offline": True},
            {"state": "skipped_offline"},
            {"state": "partial_local", "sufficient": False},
            self._zero_provider(),
            {"eligible": True, "route": "zero_call"},
            "fixture",
            audit,
        )
        self.assertTrue(any("ineligible single_synthesis_call" in item for item in audit.errors))

    def test_non_offline_single_synthesis_route_still_requires_one_call(self) -> None:
        audit = Audit()
        _validate_cheap_exit(
            self._cheap_exit(False, "single_synthesis_call"),
            0,
            "fixture",
            audit,
        )
        self.assertTrue(any("recorded 0 provider calls" in item for item in audit.errors))

    def test_missing_metadata_count_is_allowed_only_when_all_counters_are_zero(self) -> None:
        metadata = {"model": V0_PROVIDER_MODEL, "endpoint": V0_PROVIDER_ENDPOINT}
        audit = Audit()
        _validate_provider_identity(metadata, self._zero_provider(), "fixture", audit)
        self.assertEqual(audit.errors, [])

        nonzero = self._zero_provider()
        nonzero.update({"calls": 1, "transport_attempts": 1, "request_bytes": 10})
        _validate_provider_identity(metadata, nonzero, "fixture", audit)
        self.assertTrue(any("provider_request_count is invalid" in item for item in audit.errors))

    def test_cheap_report_predicate_requires_route_eligibility_and_non_offline_status(self) -> None:
        entry = {
            "useful": True,
            "provider": {"calls": 0},
            "cheap_exit": {"eligible": True, "route": "zero_call"},
            "offline": False,
            "status": {"sufficient": True},
        }
        self.assertTrue(_is_real_cheap_exit(entry))
        entry["cheap_exit"] = {"eligible": False, "route": "zero_call"}
        self.assertFalse(_is_real_cheap_exit(entry))


class GoldLedgerTest(unittest.TestCase):
    def test_raw_trace_rejects_embedded_development_gold(self) -> None:
        audit = Audit()
        _validate_trace(
            {"gold_assessment": {"disposition": "never_generated"}},
            "fixture",
            audit,
        )
        self.assertTrue(any("mutated with development gold" in item for item in audit.errors))

    def test_evaluation_ledger_binds_immutable_raw_trace_hashes(self) -> None:
        scores = {}
        validation = {}
        for episode in EPISODES:
            scores[episode.episode_id] = {
                "gold_loss_stage": "never_generated",
                "gold_candidate_id": None,
                "gold_anchor_id": None,
                "gold_loss_detail": "the decisive gold anchor was not generated",
            }
            validation[episode.episode_id] = {
                "trace": {
                    "candidate_ids": [],
                    "selected_anchor_ids": [],
                    "sha256": "a" * 64,
                    "json_path": f"episodes/{episode.episode_id}/final/attempt/run/run-1/retrieval_trace.json",
                }
            }
        ledger = build_gold_loss_ledger(scores, validation)
        self.assertFalse(ledger["production_trace_mutated"])
        self.assertEqual(len(ledger["entries"]), 7)
        self.assertTrue(all(not entry["production_trace_mutated"] for entry in ledger["entries"]))

    def test_gold_identity_must_exist_in_bound_raw_trace(self) -> None:
        scores = {}
        validation = {}
        for episode in EPISODES:
            scores[episode.episode_id] = {
                "gold_loss_stage": "never_generated",
                "gold_candidate_id": None,
                "gold_anchor_id": None,
                "gold_loss_detail": "not generated",
            }
            validation[episode.episode_id] = {
                "trace": {
                    "candidate_ids": [],
                    "selected_anchor_ids": [],
                    "sha256": "a" * 64,
                    "json_path": "trace.json",
                }
            }
        first = EPISODES[0].episode_id
        scores[first]["gold_loss_stage"] = "present_before_ranking"
        scores[first]["gold_candidate_id"] = "candidate-not-in-trace"
        with self.assertRaises(EvaluationError):
            build_gold_loss_ledger(scores, validation)


class EpisodeValidationTest(unittest.TestCase):
    def test_valid_sealed_zero_call_episode(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            spec = EpisodeSpec("fixture", "a" * 40, cheap_exit_target=True)
            episode_dir = root / "episodes" / spec.episode_id
            attempt_dir = episode_dir / "final" / "attempt"
            run_dir = attempt_dir / "run" / "run-1"
            run_dir.mkdir(parents=True)
            (attempt_dir / "run" / "latest").symlink_to("run-1")
            task_text = "Verify that a copied option reaches its effective field."
            (episode_dir / "task.md").write_text(task_text + "\n", encoding="utf-8")

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
            repository = {"revision": spec.base_revision, "tree_hash": "b" * 40}
            bundle = {
                "version": 2,
                "id": "task-fixture",
                "repository": repository,
                "task": {"text": task_text},
                "anchors": [{"id": "anchor-1"}],
            }
            cheap_exit = {
                "eligible": True,
                "route": "zero_call",
                "gates": [
                    {"gate": name, "passed": True, "reason": "exact local evidence"}
                    for name in (
                        "unambiguous_area",
                        "all_key_roles",
                        "decisive_locally_observed_relation",
                        "exact_verification_anchor_or_effect",
                        "no_unresolved_competing_hypothesis",
                    )
                ],
                "reasons": [],
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
                        "anchor_ids": ["anchor-1"],
                        "represented": True,
                    }
                ],
                "supporting": [],
                "optional": [],
            }
            frontier = {
                "decisive_anchor_id": "anchor-1",
                "anchors": [
                    {
                        "id": "verify-1",
                        "authority": "exact_existing_test",
                        "anchor_id": "anchor-1",
                        "path": "config_test.go",
                        "symbol": "TestConfig",
                        "text": "effective field preserves the option",
                        "evidence_ids": ["evidence-1"],
                    }
                ],
                "fixture": None,
                "command_or_effect": None,
            }
            bundle.update({
                "task_profile": "configuration_propagation_bug",
                "role_contract": role_contract,
                "role_coverage": role_coverage,
                "verification_frontier": frontier,
                "decisive_relation_id": "relation-1",
                "cheap_exit": cheap_exit,
            })
            bundle_hash = go_compact_json_sha256(bundle)
            trace = {
                "version": 1,
                "task_kind": "configuration",
                "task_profile": "configuration_propagation_bug",
                "task_terms": [
                    {"text": "option", "normalized": "option", "found": True, "weight": 4},
                    {"text": "field", "normalized": "field", "found": True, "weight": 4},
                ],
                "candidates_before_ranking": [
                    {
                        "id": "anchor-1",
                        "stage": "initial",
                        "discovery_order": 1,
                        "path": "config.go",
                        "symbol": "ApplyConfig",
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
                        "path": "adjacent.go",
                        "symbol": "Adjacent",
                        "roles": [],
                        "score": 1,
                        "score_components": [
                            {"kind": "production_relevance", "value": 1},
                        ],
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
                "source_scopes": [
                    {
                        "anchor_id": "anchor-1",
                        "scope": {
                            "scope_kind": "complete_enclosing_symbol",
                            "scope_start": 5,
                            "scope_end": 12,
                            "source_total_lines": 30,
                            "truncated": False,
                            "truncation_reason": "",
                            "task_matches_outside_window": False,
                            "negative_claims_allowed": True,
                            "negative_evidence_basis": "complete_scope",
                        },
                    }
                ],
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
            attempt = {
                "version": 2,
                "bundle_sha256": bundle_hash,
                "prompt_version": "task-investigation-pack-json-v2",
                "state": "skipped_local_complete",
                "provider": provider,
            }
            pack = {
                "version": 2,
                "id": bundle["id"],
                "bundle_sha256": bundle_hash,
                "repository": repository,
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
            self._write_json(run_dir / "task_investigation_bundle.json", bundle)
            self._write_json(run_dir / "task_investigation_attempt.json", attempt)
            self._write_json(run_dir / "task_investigation.json", pack)
            status = {
                "version": 2,
                "state": "accepted_local_complete",
                "sufficient": True,
                "task_id": bundle["id"],
                "bundle_sha256": bundle_hash,
                "attempt_sha256": sha256_file(run_dir / "task_investigation_attempt.json"),
                "pack_sha256": sha256_file(run_dir / "task_investigation.json"),
                "captured_revision": spec.base_revision,
                "provider": provider,
                "cheap_exit": cheap_exit,
            }
            self._write_json(run_dir / "task_investigation_status.json", status)
            self._write_json(run_dir / "metadata.json", {
                "model": "deepseek-v4-flash",
                "endpoint": "https://api.deepseek.com/chat/completions",
                "provider_request_count": 0,
            })
            self._write_json(run_dir / "retrieval_trace.json", trace)
            (run_dir / "retrieval_trace.md").write_text("# Retrieval trace\n", encoding="utf-8")
            status["retrieval_trace_sha256"] = sha256_file(run_dir / "retrieval_trace.json")
            status["retrieval_trace_markdown_sha256"] = sha256_file(run_dir / "retrieval_trace.md")
            self._write_json(run_dir / "task_investigation_status.json", status)
            self._write_json(run_dir / "task_role_contract.json", role_contract)
            self._write_json(run_dir / "verification_frontier.json", frontier)
            self._write_json(run_dir / "cheap_exit_decision.json", cheap_exit)
            self._write_json(attempt_dir / "HARNESS_ATTEMPT.json", {
                "episode_id": spec.episode_id,
                "base_revision": spec.base_revision,
                "phase": "development_final",
                "final": True,
                "semantic_retry": False,
                "one_process_invocation": True,
                "offline": False,
                "return_code": 0,
                "wall_millis": 15,
            })
            self._write_json(attempt_dir / "METRICS.json", {"provider": provider, "wall_millis": 15})

            inventory = []
            for path in sorted(item for item in attempt_dir.rglob("*") if item.is_file() and not item.is_symlink()):
                inventory.append({
                    "path": path.relative_to(attempt_dir).as_posix(),
                    "kind": "file",
                    "bytes": path.stat().st_size,
                    "sha256": sha256_file(path),
                })
            latest = attempt_dir / "run" / "latest"
            target = os.readlink(latest)
            inventory.append({
                "path": latest.relative_to(attempt_dir).as_posix(),
                "kind": "symlink",
                "bytes": len(target.encode("utf-8")),
                "target": target,
                "sha256": hashlib.sha256(target.encode("utf-8")).hexdigest(),
            })
            seal = {
                "version": 2,
                "episode_id": spec.episode_id,
                "base_revision": spec.base_revision,
                "state": status["state"],
                "sufficient": status["sufficient"],
                "artifact_sha256": {
                    name: sha256_file(run_dir / name)
                    for name in (
                        "task_investigation_bundle.json",
                        "task_investigation_attempt.json",
                        "task_investigation.json",
                        "task_investigation_status.json",
                    )
                },
                "files": inventory,
            }
            self._write_json(attempt_dir / "SEALED.json", seal)

            result = validate_episode(root, spec)
            self.assertTrue(result["artifact_valid"], result["errors"])
            self.assertEqual(result["provider"]["calls"], 0)
            self.assertEqual(result["cheap_exit"]["route"], "zero_call")

    @staticmethod
    def _write_json(path: Path, value: object) -> None:
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(json.dumps(value, indent=2) + "\n", encoding="utf-8")


if __name__ == "__main__":
    unittest.main()
