package targetportfolio

import "fmt"

const promptSystem = `You select the useful product analysis targets for one Go repository from an exact prefiltered candidate surface.
Choose one default starting target and an unordered non-empty portfolio of targets worth offering in the same repository report. A target is either an executable application or an importable library package. Each target carries exact declaration labels grouped by kind: executable targets include all build-selected non-test package declarations, while library targets include only exported API declarations. Method names are receiver-qualified. Use these labels with the display path to distinguish product behavior from generators, previews, experiments and other development tools; the labels are identifiers, not source or runtime evidence. Include multiple real products or public library APIs when the supplied target evidence supports that interpretation; omit implementation-only, fixture, example, test, generator, and development-tool targets. Do not impose a fixed portfolio size and do not pad the result.
The candidate surface contains every exact executable package and only library packages that are the exact root package of their Go module. Public non-root library packages are intentionally absent: they remain supporting code inside their module-root or executable analysis and must not become sibling report scopes.
For a library target, symbols:[] means that no public API is advertised. Such a target is ineligible: never return it as default_ref and never include it in target_refs. Executable targets remain eligible based on their supplied executable evidence.
Each selected ref becomes a separate top-level report scope in the left navigation, with its own Architecture canvas and Study content; selecting it switches the whole report scope. Select the smallest non-redundant set of independent products or downstream-consumed library surfaces. This is not package coverage: supporting implementation packages are already analyzed inside their owning target and must not be returned separately. Returning only the default is correct when no independent sibling target exists.
Use only supplied t* refs. The default_ref must also appear exactly once in target_refs. Return refs only: no paths, names, explanations, scores, ranks, categories, or prose. Return exactly one JSON object and no markdown.`

const promptUserShape = `Response JSON object fields: version (integer 3), request_ref (exact string %q), default_ref (one eligible supplied t* ref), target_refs (a non-empty JSON array of eligible supplied t* refs).
Return exactly those four fields. target_refs is an unordered set; array order carries no authority.
Exact bounded target catalog JSON:
%s`

var PromptVersion = "target-portfolio-prompt-" + shortSHA256(promptSystem+promptUserShape)

// ProviderVisibleJSON returns the sole facts bytes permitted to cross the
// provider boundary. Canonical module/package identities and exact roots stay
// in Compilation's private authority.
func ProviderVisibleJSON(compilation Compilation) ([]byte, error) {
	if err := validateCompilation(compilation); err != nil {
		return nil, err
	}
	return append([]byte(nil), compilation.wire...), nil
}

func BuildPrompt(compilation Compilation) (Prompt, error) {
	wire, err := ProviderVisibleJSON(compilation)
	if err != nil {
		return Prompt{}, err
	}
	return Prompt{
		Version: PromptVersion,
		System:  promptSystem,
		User:    fmt.Sprintf(promptUserShape, compilation.Request.RequestRef, wire),
	}, nil
}

func shortSHA256(value string) string {
	return sha256Hex([]byte(value))[:12]
}
