package deepseek

import (
	"context"
	"encoding/json"
	"fmt"
)

const (
	SymbolPromptVersionJSON   = "symbol-json-v3"
	SymbolPromptVersionTagged = "symbol-tagged-v2"
)

func (c *Client) SymbolPromptJSON(bundleJSON []byte) ([]byte, error) {
	systemPrompt, userPrompt, err := buildSymbolPrompts(bundleJSON)
	if err != nil {
		return nil, err
	}
	return c.FlowExplainPromptJSON(userPrompt, systemPrompt)
}

func (c *Client) ExplainSymbol(ctx context.Context, bundleJSON []byte) ([]byte, error) {
	systemPrompt, userPrompt, err := buildSymbolPrompts(bundleJSON)
	if err != nil {
		return nil, err
	}
	return c.flowExplain(ctx, userPrompt, systemPrompt, true, false)
}

func (c *Client) SymbolTaggedPromptJSON(bundleJSON []byte) ([]byte, error) {
	systemPrompt, userPrompt, err := buildTaggedSymbolPrompts(bundleJSON)
	if err != nil {
		return nil, err
	}
	return c.flowExplainPromptText(userPrompt, systemPrompt)
}

func (c *Client) ExplainSymbolTagged(ctx context.Context, bundleJSON []byte) ([]byte, error) {
	systemPrompt, userPrompt, err := buildTaggedSymbolPrompts(bundleJSON)
	if err != nil {
		return nil, err
	}
	return c.flowExplain(ctx, userPrompt, systemPrompt, false, false)
}

func buildSymbolPrompts(bundleJSON []byte) (string, string, error) {
	if !json.Valid(bundleJSON) {
		return "", "", fmt.Errorf("deepseek: symbol bundle is not valid json")
	}

	systemPrompt := `You investigate one selected Go symbol using only a supplied evidence bundle.

Static call edges are not runtime truth. Names may support inference but do not prove semantics. Never invent evidence IDs, paths, tests, execution order, or source behavior. Every substantive interpretation must cite evidence IDs present in the bundle. Return one valid JSON object only.`

	userPrompt := `Analyze the uniquely resolved target symbol. Local code already owns the target identity and structural caller/callee facts; do not copy them into the response.

Return JSON with exactly this shape:
{
  "summary": {
    "statement": "concise interpretation",
    "evidence_ids": ["resolution-001"],
    "confidence": 0.0
  },
  "responsibility": {
    "statement": "likely responsibility, explicitly inferred from names and static edges",
    "evidence_ids": ["resolution-001"],
    "confidence": 0.0
  },
  "read_evidence_ids": ["resolution-001", "call-out-001"],
  "test_evidence_ids": [],
  "unknowns": ["one fact not established by the bundle"],
  "next_queries": [
    {"query": "concrete local query", "reason": "why it reduces uncertainty"}
  ],
  "warnings": ["important limitation"]
}

Rules:
1. SUMMARY and RESPONSIBILITY are inference. Each statement must explicitly use cautious language such as "likely", "suggests", or "based on static names/edges". Confidence must be between 0 and 0.75.
2. Use only evidence IDs present in the bundle.
3. read_evidence_ids and test_evidence_ids contain evidence IDs, not paths. Local code resolves paths and structural roles.
4. test_evidence_ids may cite only evidence for an explicit *_test.go path. If tests are absent, keep it empty and add a next query to find tests.
5. Do not emit target, callers, or callees. Local code reconstructs those deterministic facts.
6. Describe only what symbol names and static edges suggest. A callee name may support a likely interpretation, but does not prove that validation, persistence, error handling, or any other behavior occurs.
7. Do not claim runtime paths, execution order, frequency, side effects, source bodies, parameters, return values, errors, or domain semantics unless explicitly established by supplied evidence.
8. Incoming callers are alternatives unless supplied edges connect them. Outgoing callees are an unordered set.
9. Return exactly the documented fields, without Markdown or surrounding prose.

SYMBOL EVIDENCE BUNDLE:
` + string(bundleJSON)

	return systemPrompt, userPrompt, nil
}

func buildTaggedSymbolPrompts(bundleJSON []byte) (string, string, error) {
	if !json.Valid(bundleJSON) {
		return "", "", fmt.Errorf("deepseek: symbol bundle is not valid json")
	}

	systemPrompt := `You investigate one selected Go symbol using only a supplied evidence bundle. Static call edges are not runtime truth. Names may support inference but do not prove semantics. Never invent evidence IDs, paths, tests, execution order, or source behavior. Return only the requested KEY: VALUE lines.`
	userPrompt := `Analyze the uniquely resolved target symbol. Do not return JSON, Markdown, bullets, or prose outside the tagged lines.

Use this line-oriented format:
SUMMARY: concise interpretation
SUMMARY_EVIDENCE: comma-separated evidence IDs
SUMMARY_CONFIDENCE: number from 0 to 0.75
RESPONSIBILITY: likely responsibility, explicitly inferred from names and static edges
RESPONSIBILITY_EVIDENCE: comma-separated evidence IDs
RESPONSIBILITY_CONFIDENCE: number from 0 to 0.75
READ: one evidence ID
READ: another evidence ID
TEST: evidence ID for an explicit *_test.go file, only when present
UNKNOWN: one fact not established by the bundle
NEXT_QUERY: concrete local query || reason it reduces uncertainty
WARNING: important limitation

Rules:
1. SUMMARY and RESPONSIBILITY are inference, never static facts. Each statement must explicitly say "likely", "suggests", or "based on static names/edges".
2. Use only evidence IDs present in the bundle.
3. READ and TEST contain evidence IDs, not paths. Local code resolves paths and roles.
4. Emit zero or more READ, TEST, UNKNOWN, NEXT_QUERY, and WARNING lines.
5. Do not emit TARGET, CALLER, or CALLEE lines; local code already owns structural facts.
6. A callee name may support a likely interpretation, but does not prove that validation, persistence, error handling, or any other behavior occurs.
7. Do not construct runtime paths or order sibling call edges.
8. If tests are absent, emit no TEST line and add a NEXT_QUERY to find tests.

SYMBOL EVIDENCE BUNDLE:
` + string(bundleJSON)

	return systemPrompt, userPrompt, nil
}
