package deepseek

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dvordrova/repomap/internal/sourcecard"
	"github.com/dvordrova/repomap/internal/sourceexplain"
)

const SourcePromptVersionJSON = "source-assessment-json-v4"

func (c *Client) SourcePromptJSON(bundleJSON []byte) ([]byte, error) {
	systemPrompt, userPrompt, err := buildSourcePrompts(bundleJSON)
	if err != nil {
		return nil, err
	}
	return c.FlowExplainPromptJSON(userPrompt, systemPrompt)
}

func (c *Client) AssessSource(ctx context.Context, bundleJSON []byte) ([]byte, error) {
	systemPrompt, userPrompt, err := buildSourcePrompts(bundleJSON)
	if err != nil {
		return nil, err
	}
	return c.flowExplain(ctx, userPrompt, systemPrompt, true, false)
}

func buildSourcePrompts(bundleJSON []byte) (string, string, error) {
	canonicalJSON, err := canonicalSourceBundle(bundleJSON)
	if err != nil {
		return "", "", err
	}

	systemPrompt := `You assess locally seeded questions about one Go function using only the supplied bounded source lines.

Source shows written control flow, not runtime reachability, test coverage, or the internal behavior of a callee. A callee name is a navigation hypothesis, not proof of its semantics. Never emit paths, source text, line numbers, confidence, evidence levels, commands, or identifiers absent from the bundle. Return one valid JSON object only.`

	userPrompt := `Assess every question in the source assessment bundle.

Return JSON with exactly this shape:
{
  "assessments": [
    {
      "question_id": "exact question id from the bundle",
      "verdict": "shown | not_shown | ambiguous",
      "source_evidence_ids": ["source evidence id allowed by that question"]
    }
  ],
  "unknowns": [
    {
      "kind": "callee_behavior | test_coverage | runtime_reachability | dynamic_calls | build_variants",
      "anchor_evidence_id": "target or call evidence id from the bundle"
    }
  ],
  "next_action_id": "one exact id from allowed_actions"
}

Rules:
1. Emit exactly one assessment for every question id, without duplicates.
2. Use "shown" only when the cited source lines directly show the anchored source operation. A shown assessment must cite its anchor_source_evidence_id. When a call result is used by a separate guard or returned comparison, cite both the call anchor and that result-use line; the anchor alone is ambiguous.
3. source_evidence_ids must be a subset of that question's candidate_source_evidence_ids. An evidence id that belongs to another question is invalid even when it exists elsewhere in the bundle.
4. The supplied source is a lexical window, not a parsed complete function body. It cannot prove absence. Use "ambiguous", never "not_shown", when an operation is not directly shown.
5. Match the predicate to written syntax, not the callee name alone. validates_input requires a guard, conditional, or returned nil comparison using the call result. maps_error requires returning the mapped result on a shown error path. delegates_operation supports only that the call is made at the operation step. fills_response requires response data in the shown call. Persistence and I/O names do not prove runtime side effects.
6. Source evidence cannot establish test coverage, runtime reachability, dynamic dispatch, callee internals, or behavior under another build variant. Always include both {"kind":"test_coverage","anchor_evidence_id":"<target evidence id>"} and {"kind":"runtime_reachability","anchor_evidence_id":"<target evidence id>"} in unknowns, using the exact target evidence id from the bundle.
7. Choose exactly one next_action_id from allowed_actions. Prefer find_tests after source observations are established; prefer read_callee when a claim depends on the callee's internal behavior.
8. Do not copy the target, questions, allowed actions, paths, source text, or line numbers into the response.
9. Return only the documented JSON fields, without Markdown or surrounding prose.

SOURCE ASSESSMENT BUNDLE:
` + string(canonicalJSON)

	return systemPrompt, userPrompt, nil
}

func canonicalSourceBundle(bundleJSON []byte) ([]byte, error) {
	if !json.Valid(bundleJSON) {
		return nil, fmt.Errorf("llm: source assessment bundle is not valid json")
	}
	var bundle sourceexplain.Bundle
	if err := json.Unmarshal(bundleJSON, &bundle); err != nil {
		return nil, fmt.Errorf("llm: decode source assessment bundle: %w", err)
	}
	if err := bundle.Validate(); err != nil {
		return nil, fmt.Errorf("llm: invalid source assessment bundle: %w", err)
	}
	if err := sourcecard.ValidateLinesForRemote(bundle.Source.Lines); err != nil {
		return nil, fmt.Errorf("llm: unsafe source assessment bundle: %w", err)
	}
	canonicalJSON, err := json.Marshal(bundle)
	if err != nil {
		return nil, fmt.Errorf("llm: marshal source assessment bundle: %w", err)
	}
	return canonicalJSON, nil
}
