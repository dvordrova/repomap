package entrycall

import (
	"fmt"

	"github.com/dvordrova/repomap/internal/secretscan"
)

const promptSystem = `You identify the smallest useful direct-call family set that shows how work enters or concentrates below each supplied analysis root, and classify exact bounded syntax candidates as CLI commands or HTTP routes when their advertised facts establish the closed shape.
The input is a bounded exact static-call projection. witness_count is the number of exact source callsites compacted into one caller-to-callee family; it does not prove runtime frequency. Frontier counts describe facts omitted by local bounds or calls that could not be statically identified.
For every supplied root, return zero to twelve advertised f* refs forming one rooted directed subgraph. A selected family is rooted only when its caller is the supplied root or is the callee of another selected family reachable from that root by following caller-to-callee direction. Include every advertised connector family needed to make a deeper selected family reachable; otherwise omit that deeper family. Prefer high-signal registration, dispatch, server-start, command, worker, or scheduling families. Generic setup, logging, environment, and error helpers are supporting details unless they are needed to connect a selected family to the root. An empty selection is valid when labels do not establish a useful entry mechanism.
Surface candidates are structural hints, not framework or runtime proof. Return at most one proposal for an advertised candidate, and omit uncertain candidates. For a keyed_composite CLI command, use the advertised CLI kind ref, bind exactly one string fact to the identity slot, and optionally bind one callable fact to the handler slot. If callable facts exist but the handler is omitted, require another descriptive string fact establishing a parent command; one string setting paired with an unbound factory or helper callable is not a command. For a direct_call HTTP route, use the advertised HTTP kind ref and bind exactly one string or token fact to method, one string fact to path, and one callable fact to handler. Do not use other form, kind, slot, or fact combinations.
Use refs only. Return no labels, values, paths, methods, handlers, source, identifiers, scores, explanations, endpoint claims, framework claims, parent refs, links, order, or prose. Response order carries no authority. Return exactly one JSON object, every root exactly once, and surface_proposals as an array even when empty.`

const promptUserShape = `Response schema: {"version":3,"request_ref":"q-...","entries":[{"root_ref":"r1","family_refs":["f1"]}],"surface_proposals":[{"candidate_ref":"c1","kind_ref":"k2","bindings":[{"slot_ref":"s2","fact_ref":"v1"},{"slot_ref":"s3","fact_ref":"v2"},{"slot_ref":"s4","fact_ref":"v3"}]}]}
Exact bounded request JSON:
%s`

const PromptVersion = "entry-call-compression-prompt-40da4cfef90a"

type Prompt struct {
	Version string
	System  string
	User    string
}

// ProviderVisibleJSON returns the only bytes permitted to cross the provider
// boundary. Exact IDs, repository paths, callsites, and source are held solely
// in Compilation's private authority.
func ProviderVisibleJSON(compilation Compilation) ([]byte, error) {
	if err := validateCompilation(compilation); err != nil {
		return nil, err
	}
	result := append([]byte(nil), compilation.wire...)
	if _, found := secretscan.DetectAlways(string(result)); found {
		return nil, fmt.Errorf("entry call: provider request contains credential-shaped content")
	}
	return result, nil
}

func BuildPrompt(compilation Compilation) (Prompt, error) {
	wire, err := ProviderVisibleJSON(compilation)
	if err != nil {
		return Prompt{}, err
	}
	return Prompt{
		Version: PromptVersion,
		System:  promptSystem,
		User:    fmt.Sprintf(promptUserShape, wire),
	}, nil
}
