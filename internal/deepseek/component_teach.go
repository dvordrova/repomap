package deepseek

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/dvordrova/repomap/internal/componentteach"
)

// ComponentTeachPromptVersionJSON identifies the first bounded component
// teaching prompt and response contract.
const ComponentTeachPromptVersionJSON = "component-teach-json-v2"

// ComponentTeacher adapts the purpose-specific DeepSeek client method to the
// provider-neutral port consumed by componentteach.Service.
type ComponentTeacher struct {
	Client *Client
}

var _ componentteach.Teacher = (*ComponentTeacher)(nil)

func NewComponentTeacher(client *Client) *ComponentTeacher {
	return &ComponentTeacher{Client: client}
}

func (t *ComponentTeacher) Teach(ctx context.Context, bundleJSON []byte) ([]byte, error) {
	if t == nil || t.Client == nil {
		return nil, fmt.Errorf("llm: component teacher client is required")
	}
	return t.Client.TeachComponent(ctx, bundleJSON)
}

// TeacherPromptJSON returns the exact OpenAI-compatible request body used by
// TeachComponent without making a provider call.
func (c *Client) TeacherPromptJSON(bundleJSON []byte) ([]byte, error) {
	systemPrompt, userPrompt, err := buildComponentTeachPrompts(bundleJSON)
	if err != nil {
		return nil, err
	}
	request := c.flowExplainRequest(userPrompt, systemPrompt, true)
	request.Temperature = float64Pointer(0)
	return json.Marshal(request)
}

// TeachComponent asks the provider to explain only the selected primary
// question from the bounded, locally grounded teaching bundle.
func (c *Client) TeachComponent(ctx context.Context, bundleJSON []byte) ([]byte, error) {
	body, err := c.TeacherPromptJSON(bundleJSON)
	if err != nil {
		return nil, err
	}

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			backoff := backoffDuration(attempt)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}

		result, shouldRetry, err := doChat(
			ctx,
			c.HTTPClient,
			c.Endpoint,
			c.APIKey,
			c.Auth,
			body,
			false,
		)
		if err == nil {
			return result, nil
		}
		lastErr = err
		if !shouldRetry {
			return nil, annotateResourceLimit(err, "component_teach", c.MaxTokens)
		}
	}

	return nil, fmt.Errorf("retries exhausted (%d attempts): %w", maxRetries+1, lastErr)
}

func buildComponentTeachPrompts(bundleJSON []byte) (string, string, error) {
	canonicalJSON, err := canonicalComponentTeachBundle(bundleJSON)
	if err != nil {
		return "", "", err
	}

	systemPrompt := `You are a senior Go engineer teaching one bounded repository question from supplied local evidence.

Answer only the primary question. Treat every support basis literally and never bridge a missing relation with plausibility. Use only supplied evidence and unresolved frontier hints and IDs. Do not emit repository paths, line numbers, shell commands, source locations, or invented runtime behavior. Return one valid JSON object only.`

	userPrompt := `Teach only the supplied primary question. Produce a compact, challengeable explanation rather than a repository overview.

Return JSON with exactly this shape:
{
  "version": 1,
  "primary_question_id": "exact primary question id from the bundle",
  "mental_model": [
    {"text": "one bounded explanation item", "evidence_ids": ["exact evidence id"], "frontier_ids": []}
  ],
  "lifecycle_steps": [
    {"text": "one written or static step without invented runtime order", "evidence_ids": ["exact evidence id"], "frontier_ids": []}
  ],
  "boundaries": [
    {"text": "one boundary shown by the supplied evidence", "evidence_ids": ["exact evidence id"], "frontier_ids": []}
  ],
  "design_notes": [
    {"text": "one evidence-bounded design observation", "evidence_ids": ["exact evidence id"], "frontier_ids": []}
  ],
  "failures_and_observability": [
    {"text": "one shown failure or observability fact, or an explicit limitation", "evidence_ids": ["exact evidence id"], "frontier_ids": []}
  ],
  "tests_and_checks": [
    {"text": "what a supplied test reference is useful to inspect, not what it proves", "evidence_ids": ["exact evidence id"], "frontier_ids": []}
  ],
  "unknowns": [
    {"text": "one unresolved behavior", "evidence_ids": ["exact evidence id"], "frontier_ids": ["exact unresolved frontier id"]}
  ],
  "next_dive": [
    {"text": "one bounded next research target, without a command", "evidence_ids": ["exact evidence id"], "frontier_ids": ["exact unresolved frontier id"]}
  ]
}

Rules:
1. Copy primary_question_id exactly and answer only that question. The goal and component purpose frame the question but do not prove repository behavior.
2. Every returned item must cite at least one exact evidence id from evidence. Copy IDs only; never invent, shorten, or transform them. frontier_ids may additionally contain only exact IDs from unresolved_frontier_ids. A frontier ID and its unresolved_frontiers hint are navigation leads, never support for a claim.
3. Do not emit item IDs. They are assigned locally after parsing. Use only the documented JSON fields, without Markdown or surrounding prose.
4. Treat orientation_hypothesis only as a prior suggestion to investigate. It cannot establish a mental model, lifecycle, boundary, design decision, failure behavior, or test behavior.
5. static_active_build establishes only the named direct static relation under the recorded active build. It does not establish runtime reachability, frequency, ordering, concurrency, side effects, dynamic dispatch, or behavior under another build.
6. Bounded caller/callee evidence cannot prove absence. Never say "only", "never", or "not in the call chain" from a missing static relation; state what the supplied static slice connects and what it does not establish.
7. source_supported establishes only behavior directly written in the supplied bounded source content. It does not establish that the code runs, the internals of a callee, complete function behavior outside a truncated slice, or runtime ordering across slices.
8. Inspect all supplied source_slice and callsite_slice content before declaring code or a function body missing; one body may be split across several evidence items.
9. test_navigation_only identifies a place worth inspecting. It does not establish that a test executes the path, asserts the claimed behavior, passes, or covers production runtime. Do not upgrade it to test-supported evidence.
10. No runtime-observed evidence is supplied by this contract. Any claim requiring actual execution, timing, failure convergence, goroutine scheduling, I/O completion, or production behavior must remain an unknown unless the written source itself supports a narrower statement.
11. Do not fill every section for appearance. Use an empty array when the supplied evidence cannot support that section. Keep written/static sequence separate from observed runtime order.
12. When the evidence stops at an unresolved frontier, state the missing link in unknowns and cite both the nearest evidence id and the frontier id. Use that frontier's supplied name, direction, kind, entity_kind, and support_basis only to choose and describe the next bounded research target. Do not claim the hinted target executes, is connected at runtime, or establishes what lies across the frontier. A hint with navigation_only=true is only a place to inspect. next_dive may select a frontier only when its ID remains in both unresolved_frontier_ids and unresolved_frontiers in this final bundle.
13. Never copy or reconstruct repository paths, line numbers, columns, source locations, or commands in text. Do not refer to evidence by an invented filename or location; the local UI resolves IDs separately.
14. Keep text concise and useful to an engineer. Paraphrase only what the cited evidence supports; do not quote large source fragments.

COMPONENT TEACHING BUNDLE JSON:
` + string(canonicalJSON)

	return systemPrompt, userPrompt, nil
}

func canonicalComponentTeachBundle(bundleJSON []byte) ([]byte, error) {
	if !json.Valid(bundleJSON) {
		return nil, fmt.Errorf("llm: component teaching bundle is not valid json")
	}
	var bundle componentteach.Bundle
	if err := json.Unmarshal(bundleJSON, &bundle); err != nil {
		return nil, fmt.Errorf("llm: decode component teaching bundle: %w", err)
	}
	if err := bundle.Validate(); err != nil {
		return nil, fmt.Errorf("llm: invalid component teaching bundle: %w", err)
	}
	canonicalJSON, err := json.Marshal(bundle)
	if err != nil {
		return nil, fmt.Errorf("llm: marshal component teaching bundle: %w", err)
	}
	return canonicalJSON, nil
}
