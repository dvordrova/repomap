package main

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/pythontarget"
)

type repositoryLanguageEvidence struct {
	Go                   bool
	Python               bool
	JavaScriptTypeScript bool
}

// repositoryLanguages is a cheap routing fact over the one shared corpus. A
// lone .go file is not Go-project authority: Python repositories commonly
// carry generated helpers and examples. The Go main path still handles a pure
// Go tree without go.mod; for a multi-language dispatch we require an explicit
// Go module boundary before spending the shared portfolio on Go candidates.
// Language adapters still own exact target discovery.
func repositoryLanguages(repository *corpus.Corpus) repositoryLanguageEvidence {
	if repository == nil {
		return repositoryLanguageEvidence{}
	}
	var evidence repositoryLanguageEvidence
	entries := repository.Entries()
	for _, entry := range repository.Entries() {
		switch {
		case entry.Path == "go.mod" || strings.HasSuffix(entry.Path, "/go.mod"):
			evidence.Go = true
		case path.Ext(entry.Path) == ".py" || pythonManifestPath(entry.Path):
			evidence.Python = true
		}
	}
	evidence.JavaScriptTypeScript = hasJSTSProjectEvidence(entries)
	return evidence
}

func hasJSTSProjectEvidence(entries []corpus.Entry) bool {
	manifestPaths := make([]string, 0)
	for _, entry := range entries {
		if path.Base(entry.Path) == "package.json" {
			manifestPaths = append(manifestPaths, entry.Path)
		}
	}
	if len(manifestPaths) == 0 {
		return false
	}
	for _, manifestPath := range manifestPaths {
		projectDir := path.Dir(manifestPath)
		for _, entry := range entries {
			if !jsTSProgramSourcePath(entry.Path) || (projectDir != "." && !strings.HasPrefix(entry.Path, projectDir+"/")) {
				continue
			}
			ownedByNestedPackage := false
			for _, otherManifest := range manifestPaths {
				otherDir := path.Dir(otherManifest)
				if otherDir != projectDir && strings.HasPrefix(otherDir, projectDir+"/") && strings.HasPrefix(entry.Path, otherDir+"/") {
					ownedByNestedPackage = true
					break
				}
			}
			if !ownedByNestedPackage {
				return true
			}
		}
	}
	return false
}

func jsTSProgramSourcePath(filePath string) bool {
	switch path.Ext(filePath) {
	case ".ts", ".tsx", ".mts", ".cts", ".js", ".jsx", ".mjs", ".cjs":
		return true
	default:
		return false
	}
}

func pythonManifestPath(filePath string) bool {
	base := path.Base(filePath)
	if strings.HasPrefix(base, "requirements") && strings.HasSuffix(base, ".txt") {
		return true
	}
	switch base {
	case "pyproject.toml", "setup.cfg", "setup.py", "requirements.txt", "Pipfile", "tox.ini":
		return true
	default:
		return false
	}
}

func resolvePythonTargetOverride(
	catalog pythontarget.Catalog,
	resolver pythontarget.FileTargetResolver,
	override string,
) (pythontarget.Target, error) {
	matches := make(map[string]pythontarget.Target)
	for _, target := range catalog.Entries {
		if pythonTargetMatchesOverride(target, override) {
			matches[target.Ref] = target
		}
	}
	if len(matches) == 1 {
		for _, target := range matches {
			return target, nil
		}
	}
	if len(matches) == 0 {
		if derived, ok, err := resolver.ResolveSelector(override); err != nil {
			return pythontarget.Target{}, err
		} else if ok {
			return derived, nil
		}
		choices, choicesErr := pythonExactTargetChoices(catalog, resolver)
		if choicesErr != nil {
			return pythontarget.Target{}, choicesErr
		}
		return pythontarget.Target{}, fmt.Errorf(
			"--target %q is not an eligible exact Python target; use one exact selector: %s",
			override, choices,
		)
	}
	refs := make([]string, 0, len(matches))
	for _, target := range matches {
		refs = append(refs, target.Selector)
	}
	sort.Strings(refs)
	return pythontarget.Target{}, fmt.Errorf(
		"--target %q is ambiguous; use one exact Python selector: %s",
		override, strings.Join(refs, ", "),
	)
}

func pythonExactTargetChoices(
	catalog pythontarget.Catalog,
	resolver pythontarget.FileTargetResolver,
) (string, error) {
	const moduleLimit = 12
	parts := make([]string, 0, 2)
	if native := pythonTargetChoices(catalog); native != "" {
		parts = append(parts, "native "+native)
	}
	modules, total, err := resolver.ModuleExecutionChoices(moduleLimit)
	if err != nil {
		return "", err
	}
	if len(modules) > 0 {
		choices := make([]string, 0, len(modules)+1)
		for _, choice := range modules {
			choices = append(choices, fmt.Sprintf("%s (%s)", choice.Selector, choice.Path))
		}
		if total > len(modules) {
			choices = append(choices, fmt.Sprintf("... and %d more module selectors", total-len(modules)))
		}
		parts = append(parts, "module "+strings.Join(choices, ", "))
	}
	if len(parts) == 0 {
		return "no exact Python selectors were discovered", nil
	}
	return strings.Join(parts, "; "), nil
}

func pythonTargetMatchesOverride(target pythontarget.Target, override string) bool {
	return override == target.Ref || override == target.IdentityRef || override == target.Selector ||
		override == target.DisplayName || override == target.ProjectDir ||
		pythonTargetHasPath(target, override)
}

func pythonTargetHasPath(target pythontarget.Target, wanted string) bool {
	for _, root := range target.Roots {
		if root.Path == wanted {
			return true
		}
	}
	return false
}

func pythonTargetChoices(catalog pythontarget.Catalog) string {
	const limit = 12
	choices := make([]string, 0, min(len(catalog.Entries), limit)+1)
	for index, target := range catalog.Entries {
		if index == limit {
			choices = append(choices, fmt.Sprintf("... and %d more", len(catalog.Entries)-limit))
			break
		}
		choices = append(choices, fmt.Sprintf("%s (%s; %s)", target.DisplayName, target.Kind, target.Selector))
	}
	return strings.Join(choices, ", ")
}
