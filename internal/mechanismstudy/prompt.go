package mechanismstudy

import (
	"encoding/json"
	"fmt"
)

const promptSystem = `You are identifying an unordered set of zero or more useful mechanisms inside bounded exact direct-static call graphs for supplied context cards.
Each card already contains the complete graph you may cite. Return mechanism candidates directly; do not choose a repository action, rank a next step, request expansion, or invent a root.
A qualifying shape is necessary but not sufficient: a mechanism must be one useful coherent simple directed path containing at least two consecutive edges and three distinct nodes, and at least one supplied direct reading root_node_ref must lie on it.
When a card has target_root_refs, the connected path reconstructed from edge_refs must have its first edge's caller_ref in target_root_refs, include every advertised connector edge from that target root through a supplied reading root_node_ref, and cross that reading. A local suffix that begins after the target root is invalid. Cards without target_root_refs retain the reading-local rule above.
Within one mechanism candidate, select exactly one branch: no selected node may have more than one selected incoming edge or more than one selected outgoing edge. Never union alternative continuations into one candidate; return independently useful alternatives as separate mechanisms within the existing limit.
Inspect the whole card before responding. Return the smallest set of at most three distinct paths that directly answers the card question. Do not enumerate every qualifying second-hop path or use input order as relevance. Logging, generic helper, default-value, and environment-lookup paths are supporting fragments unless the question directly asks about them. Prefer an empty mechanisms array when the labels and exact graph do not establish a useful distinction.
Use only advertised e* refs. Each candidate contains edge_refs only: return no labels, explanations, node refs, reading refs, endpoints, scores, ranks, actions, or prose. Edge-ref order and candidate-array order carry no meaning; the backend reconstructs and canonically orders exact paths, reading ties, endpoints, and invocation display.
Static calls do not prove runtime execution, frequency, causality beyond an edge, or completeness beyond the typed frontier.
Return exactly one JSON object and no markdown. Return every requested card exactly once.`

const promptUserShape = `Response schema: {"version":2,"catalog_ref":"mc-...","catalog_sha256":"...","request_ref":"q1","cards":[{"card_ref":"t1","mechanisms":[{"edge_refs":["e1","e2"]}]}]}
Return zero to three mechanisms per card and two to eight distinct advertised edge refs per mechanism. The response must contain no human-readable prose.
Exact request bundle JSON:
%s`

var PromptVersion = "mechanism-study-prompt-" + shortSHA256(promptSystem+promptUserShape)

// BuildPrompt embeds the exact bounded wire bytes. It never adds private
// catalog authority, paths, canonical IDs, source, or a follow-up action.
func BuildPrompt(batch RequestBatch) (Prompt, error) {
	if err := validateRequestBatchWire(batch); err != nil {
		return Prompt{}, err
	}
	return Prompt{
		Version: PromptVersion,
		System:  promptSystem,
		User:    fmt.Sprintf(promptUserShape, batch.WireJSON),
	}, nil
}

func shortSHA256(value string) string {
	return sha256Hex([]byte(value))[:12]
}

// ProviderVisibleJSON returns the exact bytes for one independent provider
// call. The copy makes the boundary explicit to callers and tests.
func ProviderVisibleJSON(batch RequestBatch) ([]byte, error) {
	if err := validateRequestBatchWire(batch); err != nil {
		return nil, err
	}
	return append([]byte(nil), batch.WireJSON...), nil
}

func validateRequestBatchWire(batch RequestBatch) error {
	if err := batch.Request.Validate(); err != nil {
		return err
	}
	encoded, err := json.Marshal(batch.Request)
	if err != nil {
		return fmt.Errorf("mechanism study: encode request wire: %w", err)
	}
	if len(encoded) > MaxRequestBytes || batch.WireJSON != string(encoded) ||
		batch.WireSHA256 != sha256Hex(encoded) || batch.sealed != requestBatchSeal(batch.WireSHA256) {
		return fmt.Errorf("mechanism study: request wire binding mismatch")
	}
	return nil
}
