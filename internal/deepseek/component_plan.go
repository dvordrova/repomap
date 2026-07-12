package deepseek

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dvordrova/repomap/internal/componentstudy"
)

// ComponentPlanPromptVersionJSON identifies the first bounded component
// research-planning prompt and response contract.
const ComponentPlanPromptVersionJSON = "component-plan-json-v3"

// ComponentPlanner adapts the purpose-specific DeepSeek client method to the
// provider-neutral port consumed by componentstudy.Service.
type ComponentPlanner struct {
	Client *Client
}

var _ componentstudy.Planner = (*ComponentPlanner)(nil)

func NewComponentPlanner(client *Client) *ComponentPlanner {
	return &ComponentPlanner{Client: client}
}

func (p *ComponentPlanner) Plan(ctx context.Context, bundleJSON []byte) ([]byte, error) {
	if p == nil || p.Client == nil {
		return nil, fmt.Errorf("llm: component planner client is required")
	}
	return p.Client.PlanComponent(ctx, bundleJSON)
}

// ComponentPlanPromptJSON returns the exact OpenAI-compatible request body
// used by PlanComponent without making a provider call.
func (c *Client) ComponentPlanPromptJSON(bundleJSON []byte) ([]byte, error) {
	systemPrompt, userPrompt, err := buildComponentPlanPrompts(bundleJSON)
	if err != nil {
		return nil, err
	}
	return c.FlowExplainPromptJSON(userPrompt, systemPrompt)
}

// PlanComponent asks the provider only to choose the next bounded research
// questions and evidence candidates. Conclusions are left to a later cube.
func (c *Client) PlanComponent(ctx context.Context, bundleJSON []byte) ([]byte, error) {
	systemPrompt, userPrompt, err := buildComponentPlanPrompts(bundleJSON)
	if err != nil {
		return nil, err
	}
	return c.flowExplain(ctx, userPrompt, systemPrompt, true, false)
}

func buildComponentPlanPrompts(bundleJSON []byte) (string, string, error) {
	canonicalJSON, err := canonicalComponentStudyBundle(bundleJSON)
	if err != nil {
		return "", "", err
	}

	systemPrompt := `You plan the next small research step for one selected component in an unfamiliar Go repository using only the supplied bounded candidate bundle.

Do not explain how the component works and do not answer the research objective. Static and navigation evidence is not runtime truth. Your job is only to frame uncertainty, choose a tiny evidence slice, and ask grounded questions. Never invent candidate IDs, paths, symbols, behavior, execution order, or conclusions. Return one valid JSON object only.`

	userPrompt := `Produce a bounded component research plan, not a component explanation.

Return JSON with exactly this shape:
{
  "version": 2,
  "framing": "short statement of the research scope and current uncertainty, without answering it",
  "questions": [
    {
      "id": "q-lifecycle",
      "question": "one concrete question for the next local inspection",
      "why": "how answering it reduces uncertainty",
      "evidence_ids": ["exact candidate id from the bundle"]
    }
	  ],
  "primary_question_id": "q-lifecycle",
	  "selected_files": ["exact file candidate id"],
  "selected_symbols": ["exact symbol candidate id"],
  "unknowns": ["fact the bounded bundle cannot establish"],
  "warnings": ["important evidence limitation"]
}

Rules:
1. Ask 2-4 concrete research questions. Cover the explicit clauses of the research objective before optional side topics. Questions are the frontier; do not answer them and do not state a guessed lifecycle or architecture as fact.
2. question.id is a new plan-local opaque ID such as "q-lifecycle". Every question must cite at least one copied evidence_id present in anchors, files, symbols, or evidence. The component id and purpose frame the task but are prior model orientation, so they cannot ground a question. Candidate IDs support why to inspect something; they do not prove the answer.
3. primary_question_id must copy exactly one returned question id. It is the single question to investigate now; remaining questions are backlog. selected_files contains at most 2 exact IDs from files and selected_symbols contains at most 3 exact IDs from symbols. Use strings only, never objects. Every selected file and symbol must help answer the primary question, and every file or symbol ID cited by the primary question must appear in the matching selected list. Backlog questions may cite unselected candidates for later steps.
4. Structural references in the response must be opaque candidate IDs only. Do not copy file paths, package paths, symbol names, source locations, or repository text into selected_files or selected_symbols.
5. Choose the smallest slice that can discriminate between plausible explanations. A lower rank is the local collector's stronger navigation preference, not proof; consider it together with reason and certainty. Prefer candidates that connect the selected anchor to construction, lifecycle, boundaries, failure handling, tests, or observability when the supplied goal makes them relevant.
6. Treat certainty literally: hypothesis, possible, and navigation are leads; static supports only the relation visible under the recorded analysis conditions; observed and verified still prove only the fact stated in that candidate.
7. Do not claim runtime reachability, call order, side effects, configuration precedence, test coverage, or failure behavior unless a later inspection establishes it. Put missing proof in unknowns.
8. Use only the documented JSON fields, without Markdown or surrounding prose.

COMPONENT RESEARCH CANDIDATE BUNDLE:
` + string(canonicalJSON)

	return systemPrompt, userPrompt, nil
}

func canonicalComponentStudyBundle(bundleJSON []byte) ([]byte, error) {
	if !json.Valid(bundleJSON) {
		return nil, fmt.Errorf("llm: component study bundle is not valid json")
	}
	var bundle componentstudy.Bundle
	if err := json.Unmarshal(bundleJSON, &bundle); err != nil {
		return nil, fmt.Errorf("llm: decode component study bundle: %w", err)
	}
	if err := bundle.Validate(); err != nil {
		return nil, fmt.Errorf("llm: invalid component study bundle: %w", err)
	}
	canonicalJSON, err := json.Marshal(bundle)
	if err != nil {
		return nil, fmt.Errorf("llm: marshal component study bundle: %w", err)
	}
	return canonicalJSON, nil
}
