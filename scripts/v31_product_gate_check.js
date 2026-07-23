#!/usr/bin/env node
"use strict";

// This checker is deliberately read-only. It evaluates an already materialized
// run; it never invokes repomap, a model provider, or repository analysis.

const fs = require("fs");
const path = require("path");
const vm = require("vm");

const runDirArg = process.argv[2];
const specArg = process.argv[3];
if (!runDirArg || !specArg) {
  process.stderr.write("usage: v31_product_gate_check.js RUN_DIR SPEC_JSON\n");
  process.exit(2);
}

const runDir = path.resolve(runDirArg);
const specPath = path.resolve(specArg);
const failures = [];

function fail(code, detail) {
  failures.push({ code, detail });
}

function requireCondition(condition, code, detail) {
  if (!condition) fail(code, detail);
  return condition;
}

function readJSON(filename, label) {
  try {
    return JSON.parse(fs.readFileSync(filename, "utf8"));
  } catch (error) {
    fail("artifact_unreadable", `${label}: ${error.message}`);
    return null;
  }
}

function readRunJSON(name) {
  return readJSON(path.join(runDir, name), name);
}

function text(value) {
  return typeof value === "string" ? value.trim() : "";
}

function list(value) {
  return Array.isArray(value) ? value : [];
}

function normalizedProse(value) {
  return text(value)
    .toLowerCase()
    .replace(/[^\p{L}\p{N}]+/gu, " ")
    .trim();
}

function isNaturalQuestion(value) {
  const question = text(value);
  if (!question.endsWith("?")) return false;
  if (normalizedProse(question).split(/\s+/).filter(Boolean).length < 5) return false;
  return !/\b(?:artifact|candidate|claim|fact id|opaque id|verdict|semantic pipeline|bounded evidence)\b/i.test(question);
}

function sourceIsConcrete(source) {
  if (!source || !text(source.path) || !text(source.content)) return false;
  if (!Number.isInteger(source.start_line) || !Number.isInteger(source.end_line)) return false;
  if (source.start_line < 1 || source.end_line < source.start_line) return false;
  if (list(source.lines).length === 0 || list(source.highlight_ranges).length === 0) return false;
  return list(source.lines).some((line) =>
    Number.isInteger(line && line.line) && typeof (line && line.text) === "string"
  );
}

// In the current report projection, `sources` is the default visible excerpt.
// `full_function_lines` is only the secondary expansion. Requiring a concrete
// primary source here therefore prevents a full-function/secondary-only pass.
function sourceIsConcreteDefault(source) {
  return sourceIsConcrete(source) && text(source.role) === "primary";
}

const spec = readJSON(specPath, "spec");
const manifest = readRunJSON("run_manifest.json");
const metadata = readRunJSON("metadata.json");
const status = readRunJSON("fresh_repo_demo_status.json");
const report = readRunJSON("report.json");

if (!spec) {
  process.exit(1);
}

requireCondition(spec.version === 1, "spec_version_unsupported", `version=${JSON.stringify(spec.version)}`);
const expectedRevision = text(spec.expected_revision);
const requiredRole = text(spec.required_primary_role);
const requiredAspects = list(spec.required_aspects).map(text).filter(Boolean);
const effectAspect = text(spec.effect_aspect);
const minimumPhases = Number(spec.phase_count && spec.phase_count.min);
const maximumPhases = Number(spec.phase_count && spec.phase_count.max);
const fresh = spec.fresh_run || {};

requireCondition(Boolean(expectedRevision), "spec_expected_revision_missing", "expected_revision is empty");
requireCondition(Boolean(requiredRole), "spec_primary_role_missing", "required_primary_role is empty");
requireCondition(requiredAspects.length > 0, "spec_required_aspects_missing", "required_aspects is empty");
requireCondition(requiredAspects.includes(effectAspect), "spec_effect_aspect_invalid", `effect_aspect=${JSON.stringify(effectAspect)}`);
requireCondition(Number.isInteger(minimumPhases) && Number.isInteger(maximumPhases) && minimumPhases >= 1 && maximumPhases >= minimumPhases,
  "spec_phase_range_invalid", `phase_count=${JSON.stringify(spec.phase_count)}`);

if (manifest) {
  const revision = text(manifest.repository_state && manifest.repository_state.head);
  requireCondition(
    revision === expectedRevision,
    "frozen_revision_mismatch",
    `repository_state.head=${JSON.stringify(revision)}, want ${expectedRevision}`
  );
}

if (metadata) {
  if (text(spec.expected_model)) {
    requireCondition(metadata.model === spec.expected_model, "model_mismatch",
      `model=${JSON.stringify(metadata.model)}, want ${JSON.stringify(spec.expected_model)}`);
  }
  if (text(spec.expected_endpoint)) {
    requireCondition(metadata.endpoint === spec.expected_endpoint, "provider_endpoint_mismatch",
      `endpoint=${JSON.stringify(metadata.endpoint)}, want ${JSON.stringify(spec.expected_endpoint)}`);
  }
  const options = metadata.effective_options || {};
  if (typeof fresh.offline === "boolean") {
    requireCondition(options.offline === fresh.offline, "fresh_offline_mode_mismatch",
      `effective_options.offline=${JSON.stringify(options.offline)}, want ${fresh.offline}`);
  }
  if (typeof fresh.preview_request === "boolean") {
    requireCondition(options.preview_request === fresh.preview_request, "fresh_preview_mode_mismatch",
      `effective_options.preview_request=${JSON.stringify(options.preview_request)}, want ${fresh.preview_request}`);
  }
  for (const stage of list(fresh.required_request_stages)) {
    const attempt = list(metadata.request_attempts).find((entry) => entry && entry.stage === stage);
    requireCondition(Boolean(attempt), "fresh_request_stage_missing", `metadata.request_attempts has no ${JSON.stringify(stage)}`);
    if (attempt) {
      requireCondition(Number(attempt.provider_call_count) > 0, "fresh_request_stage_not_provider_backed",
        `${stage}.provider_call_count=${JSON.stringify(attempt.provider_call_count)}`);
    }
  }
}

let published = [];
let startHereID = "";
let publishedStart = null;
let attempt = null;
let mechanism = null;
let semanticArtifact = null;

if (status) {
  requireCondition(
    status.state === fresh.required_status_state,
    "fresh_publication_missing",
    `state=${JSON.stringify(status.state)} failure_reason=${JSON.stringify(status.failure_reason || "")}`
  );
  requireCondition(
    Number(status.total_model_calls) >= Number(fresh.minimum_model_calls),
    "fresh_model_call_provenance_missing",
    `total_model_calls=${JSON.stringify(status.total_model_calls)}, want >=${JSON.stringify(fresh.minimum_model_calls)}`
  );
  requireCondition(
    Number.isInteger(status.questions_proposed) && status.questions_proposed > 0,
    "fresh_question_missing",
    `questions_proposed=${JSON.stringify(status.questions_proposed)}`
  );
  requireCondition(
    Number.isInteger(status.candidates_selected) && status.candidates_selected >= 1 && status.candidates_selected <= Number(spec.maximum_candidates),
    "candidate_limit_or_selection_mismatch",
    `candidates_selected=${JSON.stringify(status.candidates_selected)}, want 1..${JSON.stringify(spec.maximum_candidates)}`
  );
  if (fresh.require_opportunity_provider_call) {
    requireCondition(Boolean(status.opportunity && status.opportunity.provider_call),
      "fresh_opportunity_provider_call_missing", `opportunity=${JSON.stringify(status.opportunity || null)}`);
    requireCondition(status.opportunity && status.opportunity.status === "accepted",
      "fresh_opportunity_not_accepted", `opportunity.status=${JSON.stringify(status.opportunity && status.opportunity.status)}`);
  }
  published = list(status.published_mechanisms);
  if (status.state !== fresh.required_status_state) {
    list(status.attempts).forEach((entry, index) => {
      fail(
        `candidate_${index + 1}_rejected`,
        `candidate=${JSON.stringify(entry && entry.candidate_id)} state=${JSON.stringify(entry && entry.state)} ` +
          `stage=${JSON.stringify(entry && entry.failure_stage || "")} ` +
          `reason=${JSON.stringify(entry && entry.failure_reason || "")}`
      );
      const eligibility = entry && entry.primary_eligibility || {};
      for (const [field, code] of [
        ["input_fact_ids", "input_trigger_fact_missing"],
        ["core_fact_ids", "core_work_fact_missing"],
        ["effect_fact_ids", "observable_effect_fact_missing"],
      ]) {
        if (list(eligibility[field]).length === 0) {
          fail(
            `candidate_${index + 1}_${code}`,
            `${field}=${JSON.stringify(eligibility[field] || [])}; eligibility reasons=${JSON.stringify(list(eligibility.reasons))}`
          );
        }
      }
    });
  }
}

if (report) {
  startHereID = text(report.repository_guide && report.repository_guide.start_here_artifact_id);
}
requireCondition(Boolean(startHereID), "start_here_missing", "repository_guide.start_here_artifact_id is empty");

if (status && startHereID) {
  publishedStart = published.find((entry) => text(entry && entry.artifact_id) === startHereID) || null;
  requireCondition(Boolean(publishedStart), "start_here_not_published",
    `Start Here ${startHereID} is absent from fresh_repo_demo_status.published_mechanisms`);
  if (publishedStart) {
    requireCondition(publishedStart.role === requiredRole, "start_here_role_mismatch",
      `published role=${JSON.stringify(publishedStart.role)}, want ${JSON.stringify(requiredRole)}`);
    attempt = list(status.attempts).find((entry) =>
      text(entry && entry.candidate_id) === text(publishedStart.candidate_id)
    ) || null;
    requireCondition(Boolean(attempt), "published_attempt_missing",
      `candidate ${JSON.stringify(publishedStart.candidate_id)} has no retained attempt`);
  }
}

if (attempt) {
  requireCondition(attempt.state === "published", "published_attempt_state_mismatch",
    `candidate ${attempt.candidate_id} state=${JSON.stringify(attempt.state)}`);
  if (fresh.require_synthesis_provider_call) {
    requireCondition(Boolean(attempt.synthesis && attempt.synthesis.provider_call),
      "fresh_synthesis_provider_call_missing", `synthesis=${JSON.stringify(attempt.synthesis || null)}`);
    requireCondition(attempt.synthesis && attempt.synthesis.status === "accepted",
      "fresh_synthesis_not_accepted", `synthesis.status=${JSON.stringify(attempt.synthesis && attempt.synthesis.status)}`);
  }
  const eligibility = attempt.primary_eligibility || {};
  requireCondition(eligibility.status === "ready", "primary_eligibility_not_ready",
    `status=${JSON.stringify(eligibility.status)} reasons=${JSON.stringify(list(eligibility.reasons))}`);
  for (const [field, code] of [
    ["input_fact_ids", "input_trigger_fact_missing"],
    ["core_fact_ids", "core_work_fact_missing"],
    ["effect_fact_ids", "observable_effect_fact_missing"],
  ]) {
    requireCondition(list(eligibility[field]).length > 0, code,
      `${field}=${JSON.stringify(eligibility[field] || [])}`);
  }
  const summary = attempt.artifact || {};
  const required = new Set(list(summary.required_answer_aspects));
  const covered = new Set(list(summary.covered_answer_aspects));
  const uncovered = new Set(list(summary.uncovered_answer_aspects));
  for (const aspect of requiredAspects) {
    requireCondition(required.has(aspect), "required_aspect_missing",
      `candidate ${attempt.candidate_id} does not require ${aspect}`);
    requireCondition(covered.has(aspect) && !uncovered.has(aspect), "required_aspect_uncovered",
      `${aspect}: covered=${covered.has(aspect)} uncovered=${uncovered.has(aspect)}`);
  }
}

if (report && startHereID) {
  mechanism = list(report.user_mechanisms).find((entry) => text(entry && entry.artifact_id) === startHereID) || null;
  semanticArtifact = list(report.semantic_artifacts).find((entry) => text(entry && entry.id) === startHereID) || null;
}

requireCondition(Boolean(mechanism) && Boolean(semanticArtifact), "canonical_primary_mechanism_missing",
  `artifact=${JSON.stringify(startHereID)} user_mechanism=${Boolean(mechanism)} semantic_artifact=${Boolean(semanticArtifact)}`);

let naturalSearchTargetOK = false;
let effectDefaultSourceOK = false;
if (mechanism) {
  requireCondition(mechanism.role === requiredRole, "user_mechanism_role_mismatch",
    `role=${JSON.stringify(mechanism.role)}, want ${JSON.stringify(requiredRole)}`);
  const question = text(mechanism.question);
  requireCondition(isNaturalQuestion(question), "natural_question_invalid", `question=${JSON.stringify(question)}`);
  requireCondition(text(mechanism.answer).length >= 20, "mechanism_answer_missing",
    `answer=${JSON.stringify(text(mechanism.answer))}`);
  if (attempt) {
    requireCondition(normalizedProse(question) === normalizedProse(attempt.question), "selected_question_not_retained",
      `mechanism=${JSON.stringify(question)} attempt=${JSON.stringify(attempt.question)}`);
  }

  const steps = list(mechanism.steps);
  const phases = list(mechanism.phases);
  requireCondition(phases.length >= minimumPhases && phases.length <= maximumPhases,
    "visible_phase_count_out_of_range", `visible phases=${phases.length}, want ${minimumPhases}..${maximumPhases}`);
  const usedStepIndexes = new Set();
  phases.forEach((phase, phaseIndex) => {
    const prefix = `phase_${phaseIndex + 1}`;
    requireCondition(text(phase && phase.title).length > 0 && text(phase && phase.explanation).length >= 20,
      `${prefix}_explanation_missing`,
      `title=${JSON.stringify(text(phase && phase.title))} explanation=${JSON.stringify(text(phase && phase.explanation))}`);
    const sources = list(phase && phase.sources);
    requireCondition(sources.length > 0, `${prefix}_source_missing`, "phase has no source projection");
    sources.forEach((source, sourceIndex) => {
      requireCondition(sourceIsConcrete(source), `${prefix}_source_${sourceIndex + 1}_not_concrete`,
        `path=${JSON.stringify(text(source && source.path))} lines=${list(source && source.lines).length} highlights=${list(source && source.highlight_ranges).length}`);
    });
    const indexes = list(phase && phase.implementation_step_indexes);
    requireCondition(indexes.length > 0, `${prefix}_step_binding_missing`, "phase has no implementation_step_indexes");
    indexes.forEach((stepIndex) => {
      usedStepIndexes.add(stepIndex);
      const step = Number.isInteger(stepIndex) ? steps[stepIndex] : null;
      requireCondition(Boolean(step), `${prefix}_step_binding_invalid`,
        `step_index=${JSON.stringify(stepIndex)} steps=${steps.length}`);
      if (step) {
        requireCondition(list(step.sources).some(sourceIsConcrete), `${prefix}_step_source_missing`,
          `step_index=${stepIndex} has no concrete source`);
      }
    });
  });
  requireCondition(usedStepIndexes.size >= minimumPhases, "meaningful_phase_bindings_below_minimum",
    `distinct implementation steps=${usedStepIndexes.size}, want at least ${minimumPhases}`);

  if (semanticArtifact) {
    const effectStatements = list(semanticArtifact.statements).filter((statement) =>
      list(statement && statement.answer_aspect_ids).includes(effectAspect)
    );
    for (const statement of effectStatements) {
      const statementText = normalizedProse(statement && statement.text);
      const effectStepIndex = steps.findIndex((step) => normalizedProse(step && step.explanation) === statementText);
      if (effectStepIndex < 0) continue;
      const step = steps[effectStepIndex];
      const phase = phases.find((entry) => list(entry && entry.implementation_step_indexes).includes(effectStepIndex));
      if (list(step && step.sources).some(sourceIsConcreteDefault) &&
          list(phase && phase.sources).some(sourceIsConcreteDefault)) {
        effectDefaultSourceOK = true;
        break;
      }
    }
  }
  requireCondition(effectDefaultSourceOK, "observable_effect_default_source_missing",
    `effect aspect ${JSON.stringify(effectAspect)} is not bound to a primary default step and phase excerpt`);

  const index = report.semantic_search;
  if (index && Array.isArray(index.items)) {
    try {
      const assetPath = path.resolve(__dirname, "../internal/report/templates/semantic_search.js");
      const window = { __REPOMAP_SEARCH_TEST__: {} };
      vm.runInNewContext(fs.readFileSync(assetPath, "utf8"), { window });
      const ranked = window.__REPOMAP_SEARCH_TEST__.rankSemanticSearchItems(index.items, question, 12);
      const first = ranked[0];
      const target = first && first.item && first.item.target;
      naturalSearchTargetOK = Boolean(first) && first.complete === true &&
        target && target.kind === "semantic_artifact" && target.artifact_id === startHereID;
      if (!naturalSearchTargetOK) {
        fail("natural_behavior_search_target_missing",
          `question=${JSON.stringify(question)} target=${JSON.stringify(target || null)}, want artifact ${startHereID}`);
      }
    } catch (error) {
      fail("semantic_search_evaluation_failed", error.message);
    }
  } else {
    fail("semantic_search_missing", "report.semantic_search.items is unavailable");
  }
}

if (!mechanism) {
  fail("visible_phase_count_out_of_range", `visible phases=0, want ${minimumPhases}..${maximumPhases}`);
  fail("natural_behavior_search_target_missing", "no canonical primary Mechanism question can route through Search");
}

if (semanticArtifact) {
  requireCondition(semanticArtifact.kind === "mechanism", "start_here_artifact_kind_mismatch",
    `kind=${JSON.stringify(semanticArtifact.kind)}, want "mechanism"`);
}

if (failures.length > 0) {
  failures.forEach((entry) => process.stderr.write(`v3.1 gate: ${entry.code}: ${entry.detail}\n`));
  process.exit(1);
}

process.stdout.write(JSON.stringify({
  version: 1,
  result: "ready_for_owner_review",
  product_success: false,
  owner_review_required: true,
  spec: specPath,
  run_dir: runDir,
  revision: expectedRevision,
  start_here_artifact_id: startHereID,
  question: mechanism.question,
  visible_phases: mechanism.phases.length,
  input_facts: attempt.primary_eligibility.input_fact_ids.length,
  core_facts: attempt.primary_eligibility.core_fact_ids.length,
  effect_facts: attempt.primary_eligibility.effect_fact_ids.length,
  observable_effect_default_source: effectDefaultSourceOK,
  natural_question_search_target: naturalSearchTargetOK ? startHereID : "",
}, null, 2) + "\n");
