package entrycall

import (
	"fmt"

	"github.com/dvordrova/repomap/internal/secretscan"
)

const promptSystem = `You identify the smallest useful direct-call family set that shows how work enters or concentrates below each supplied analysis root, and classify exact bounded syntax candidates as CLI commands, HTTP routes, or scheduled jobs when their advertised facts establish the closed shape.
The input is a bounded exact static-call projection. witness_count is the number of exact source callsites compacted into one caller-to-callee family; it does not prove runtime frequency. Frontier counts describe facts omitted by local bounds or calls that could not be statically identified.
For every supplied root, return any useful subset of its advertised f* refs forming one rooted directed subgraph. Every advertised family is already within the per-root response bound, so include all useful rooted families; do not stop at an arbitrary count. A selected family is rooted only when its caller is the supplied root or is the callee of another selected family reachable from that root by following caller-to-callee direction. Include every advertised connector family needed to make a deeper selected family reachable; otherwise omit that deeper family. Prefer high-signal registration, dispatch, server-start, command, worker, or scheduling families. Generic setup, logging, environment, and error helpers are supporting details unless they are needed to connect a selected family to the root. An empty selection is valid when labels do not establish a useful entry mechanism.
Surface candidates are structural hints, not framework or runtime proof. Family selection and surface proposals have independent bounds. Examine every advertised surface candidate and return one proposal for every candidate whose exact facts establish a CLI command, HTTP route registration, or scheduled job. Do not stop after an arbitrary number of surface proposals. Return at most one proposal for an advertised candidate, and omit uncertain candidates. For a keyed_composite CLI command, use the advertised CLI kind ref, bind exactly one string fact to the identity slot, and optionally bind one callable fact to the handler slot. If callable facts exist but the handler is omitted, require another descriptive string fact establishing a parent command; one string setting paired with an unbound factory or helper callable is not a command. For a direct_call HTTP route, use the advertised HTTP kind ref and bind exactly one string fact beginning with / to path. Method and handler are optional: bind at most one advertised string or token fact to method and at most one advertised callable fact to handler. When handler is omitted, the backend attaches it only if the candidate has exactly one callable fact; zero or multiple callable facts remain unbound. A token method fact must case-insensitively equal CONNECT, DELETE, GET, HEAD, OPTIONS, PATCH, POST, PUT, or TRACE; a string method fact may preserve an exact custom HTTP method token. Publish a path-only descriptor only when the candidate semantics establish an HTTP route registration. Middleware, filters, static-filesystem calls, mounts, and ordinary two-string helper calls are not routes. Never interpret or split an action string such as METHOD:Action into an HTTP method or callback; bind method or handler only from its own advertised fact of the required kind. For a direct_call scheduled job, use the advertised scheduled-job kind ref, bind exactly one string fact to identity, optionally bind one callable fact to handler, and do not bind method or path. Prefer an advertised stable job name as identity; only when no stable job name is advertised may the exact schedule string be the identity. If callable facts exist but the handler is omitted, require another exact string fact beyond identity that establishes the time- or schedule-driven registration; one schedule string paired with an unbound callback is not enough. A scheduled job must be a time- or schedule-driven registration. Generic callbacks, event handlers, worker starts, and lifecycle hooks are not scheduled jobs. Do not use other form, kind, slot, or fact combinations.
Use refs only. Do not return request_ref; request identity is backend-owned and bound locally to the exact request. Return no labels, values, paths, methods, handlers, source, identifiers, scores, explanations, endpoint claims, framework claims, parent refs, links, order, or prose. Response order carries no authority. Return exactly one JSON object, every root exactly once, and surface_proposals as an array even when empty.`

const promptUserShape = `Response schema: {"version":4,"entries":[{"root_ref":"r1","family_refs":["f1"]}],"surface_proposals":[{"candidate_ref":"c1","kind_ref":"k2","bindings":[{"slot_ref":"s2","fact_ref":"v1"},{"slot_ref":"s3","fact_ref":"v2"},{"slot_ref":"s4","fact_ref":"v3"}]}]}
Exact bounded request JSON:
%s`

const PromptVersion = "entry-call-compression-prompt-a5f294daff33"

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
