package entrycall

import (
	"fmt"

	"github.com/dvordrova/repomap/internal/secretscan"
)

const promptSystem = `You identify the smallest useful direct-call family set that shows how work enters or concentrates below each supplied analysis root.
The input is a bounded exact static-call projection. witness_count is the number of exact source callsites compacted into one caller-to-callee family; it does not prove runtime frequency. Frontier counts describe facts omitted by local bounds or calls that could not be statically identified.
For every supplied root, return zero to twelve advertised f* refs forming one rooted directed subgraph. A selected family is rooted only when its caller is the supplied root or is the callee of another selected family reachable from that root by following caller-to-callee direction. Include every advertised connector family needed to make a deeper selected family reachable; otherwise omit that deeper family. Prefer high-signal registration, dispatch, server-start, command, worker, or scheduling families. Generic setup, logging, environment, and error helpers are supporting details unless they are needed to connect a selected family to the root. An empty selection is valid when labels do not establish a useful entry mechanism.
Use refs only. Return no labels, paths, source, identifiers, scores, explanations, endpoint claims, framework claims, or prose. Response order carries no authority. Return exactly one JSON object and every root exactly once.`

const promptUserShape = `Response schema: {"version":2,"request_ref":"q-...","entries":[{"root_ref":"r1","family_refs":["f1"]}]}
Exact bounded request JSON:
%s`

const PromptVersion = "entry-call-compression-prompt-c53df8bbacc9"

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
