package pythontarget

import (
	"fmt"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/corpus"
)

const (
	pythonCallableHypothesis         = "defines a Python callable entry point"
	pythonModuleHypothesis           = "provides a runnable Python module"
	pythonMainGuardHypothesis        = "contains a Python main guard"
	pythonExecutableScriptHypothesis = "is an executable Python script"
	pythonBoundObjectHypothesis      = "defines a Python bound-object entry point"
	pythonConsoleScriptHypothesis    = "implements a declared Python console script"
	pythonGUIScriptHypothesis        = "implements a declared Python GUI script"
	pythonPackageMainHypothesis      = "provides a Python package main module"
	pythonDeclaredPackageHypothesis  = "provides a declared Python import package"
	pythonPublicPackageHypothesis    = "public Python package surface"
)

// FileCandidates projects the exact files in a Python target catalog onto the
// shared initial-scout contract. It deliberately exposes no Python target
// identity: executable evidence is attached to each exact Root.Path, while
// library evidence is attached to the target's sealed SourceRefs. The latter
// is normally its exact packaging basis and deliberately is not every module
// or package in the project inventory.
func FileCandidates(
	repository *corpus.Corpus,
	catalog Catalog,
) ([]analysistarget.FileCandidate, error) {
	if _, err := validateFileProjectionInputs(repository, catalog); err != nil {
		return nil, err
	}
	return fileCandidatesFromValidated(repository, catalog)
}

// FileCandidatesWithResolver projects one validated catalog once for ordinary
// orchestration and returns both the public scout rows and the private exact
// resolver. Keeping the two views in one handoff avoids immediately repeating
// catalog/corpus validation after target discovery.
func FileCandidatesWithResolver(
	repository *corpus.Corpus,
	catalog Catalog,
) ([]analysistarget.FileCandidate, FileTargetResolver, error) {
	snapshot, err := validateFileProjectionInputs(repository, catalog)
	if err != nil {
		return nil, FileTargetResolver{}, err
	}
	candidates, err := fileCandidatesFromValidated(repository, catalog)
	if err != nil {
		return nil, FileTargetResolver{}, err
	}
	resolver, err := fileTargetResolverFromValidated(catalog, snapshot)
	if err != nil {
		return nil, FileTargetResolver{}, err
	}
	return candidates, resolver, nil
}

func validateFileProjectionInputs(
	repository *corpus.Corpus,
	catalog Catalog,
) (corpus.Snapshot, error) {
	if repository == nil {
		return corpus.Snapshot{}, fmt.Errorf("python target file projection: repository corpus is required")
	}
	snapshot, err := repository.Snapshot().Owned()
	if err != nil {
		return corpus.Snapshot{}, fmt.Errorf("python target file projection: repository corpus: %w", err)
	}
	if err := catalog.Validate(); err != nil {
		return corpus.Snapshot{}, fmt.Errorf("python target file projection: catalog: %w", err)
	}
	if err := validateCatalogCorpus(catalog, repository); err != nil {
		return corpus.Snapshot{}, fmt.Errorf("python target file projection: corpus binding: %w", err)
	}
	return snapshot, nil
}

func fileCandidatesFromValidated(
	repository *corpus.Corpus,
	catalog Catalog,
) ([]analysistarget.FileCandidate, error) {
	raw := make([]analysistarget.FileCandidate, 0)
	for _, target := range catalog.Entries {
		if resolverOnlyModuleExecutionTarget(target) {
			continue
		}
		switch target.Kind {
		case KindExecutable:
			basisHypotheses, err := executableBasisHypotheses(target.Basis)
			if err != nil {
				return nil, err
			}
			for _, root := range target.Roots {
				rootHypothesis, err := executableRootHypothesis(root.Kind)
				if err != nil {
					return nil, err
				}
				fileRef, err := exactCandidateFileRef(repository, root.Path)
				if err != nil {
					return nil, err
				}
				hypotheses := make([]string, 0, 1+len(basisHypotheses))
				hypotheses = append(hypotheses, rootHypothesis)
				hypotheses = append(hypotheses, basisHypotheses...)
				raw = append(raw, analysistarget.FileCandidate{
					FileRef: fileRef, Hypotheses: hypotheses,
				})
			}

		case KindLibrary:
			libraryHypotheses, err := libraryTargetHypotheses(target.Basis)
			if err != nil {
				return nil, err
			}
			for _, fileRef := range target.SourceRefs {
				raw = append(raw, analysistarget.FileCandidate{
					FileRef:    fileRef,
					Hypotheses: append([]string(nil), libraryHypotheses...),
				})
			}

		default:
			return nil, fmt.Errorf(
				"python target file candidates: unsupported target kind %q", target.Kind,
			)
		}
	}

	result, err := analysistarget.MergeFileCandidates(repository.Snapshot(), raw)
	if err != nil {
		return nil, fmt.Errorf("python target file candidates: merge: %w", err)
	}
	return result, nil
}

func libraryTargetHypotheses(values []Basis) ([]string, error) {
	declared := false
	for _, value := range values {
		if value.Kind != BasisImportPackage {
			return nil, fmt.Errorf(
				"python target file candidates: unsupported library basis kind %q", value.Kind,
			)
		}
		declared = true
	}
	if !declared {
		return nil, fmt.Errorf("python target file candidates: library target has no import-package basis")
	}
	return []string{pythonDeclaredPackageHypothesis, pythonPublicPackageHypothesis}, nil
}

func exactCandidateFileRef(repository *corpus.Corpus, filePath string) (corpus.FileID, error) {
	fileRef, ok := repository.ID(filePath)
	if !ok {
		return "", fmt.Errorf(
			"python target file candidates: exact source path %q is absent from the corpus",
			filePath,
		)
	}
	return fileRef, nil
}

func executableRootHypothesis(kind RootKind) (string, error) {
	switch kind {
	case RootCallable:
		return pythonCallableHypothesis, nil
	case RootModule:
		return pythonModuleHypothesis, nil
	case RootMainGuard:
		return pythonMainGuardHypothesis, nil
	case RootScriptFile:
		return pythonExecutableScriptHypothesis, nil
	case RootBoundObject:
		return pythonBoundObjectHypothesis, nil
	default:
		return "", fmt.Errorf(
			"python target file candidates: unsupported executable root kind %q", kind,
		)
	}
}

func executableBasisHypotheses(values []Basis) ([]string, error) {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		var hypothesis string
		switch value.Kind {
		case BasisPEP621Script, BasisPoetryScript, BasisSetupCFGScript, BasisSetupPYScript:
			hypothesis = pythonConsoleScriptHypothesis
		case BasisPEP621GUIScript, BasisSetupCFGGUIScript, BasisSetupPYGUIScript:
			hypothesis = pythonGUIScriptHypothesis
		case BasisPackageMain:
			hypothesis = pythonPackageMainHypothesis
		case BasisNameMainGuard:
			hypothesis = pythonMainGuardHypothesis
		case BasisPythonShebang:
			hypothesis = pythonExecutableScriptHypothesis
		case BasisModuleExecutionView:
			continue
		case BasisImportPackage:
			hypothesis = pythonDeclaredPackageHypothesis
		default:
			return nil, fmt.Errorf(
				"python target file candidates: unsupported executable basis kind %q", value.Kind,
			)
		}
		if _, duplicate := seen[hypothesis]; duplicate {
			continue
		}
		seen[hypothesis] = struct{}{}
		result = append(result, hypothesis)
	}
	return result, nil
}

func resolverOnlyModuleExecutionTarget(target Target) bool {
	return len(target.Roots) == 1 && target.Roots[0].Kind == RootModuleExecution &&
		len(target.Basis) == 1 && target.Basis[0].Kind == BasisModuleExecutionView
}
