package targetportfolio

import "fmt"

const promptSystem = `You select the useful product analysis targets for one Go repository from an exact prefiltered candidate surface.
Choose one default starting target and an unordered non-empty portfolio of targets worth offering in the same repository report. A target is either one executable application or one aggregate Go-module library surface. Every target carries exact package-grouped declaration labels: an executable has exactly one build-selected non-test package, while a module library groups the exported API declarations of all of its externally importable non-main packages. Method names are receiver-qualified. Package display paths and declaration labels are identifiers, not source or runtime evidence. Use them to distinguish product behavior from generators, previews, experiments and other development tools; include multiple real executables or module libraries when the supplied evidence supports that interpretation, and omit fixture, example, test, generator, and development-tool executables. Do not impose a fixed portfolio size and do not pad the result.
The candidate surface already contains every exact executable package and at most one aggregate library target per exact Go module. A module library's package groups are evidence for that one target: never split them into sibling targets and never treat package coverage as portfolio coverage. Internal and otherwise non-importable packages are not advertised as public module API.
For a module-library target, packages:[] means that no public API is advertised. Such a target is ineligible: never return it as default_ref and never include it in target_refs. Executable targets remain eligible based on their supplied executable evidence.
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
