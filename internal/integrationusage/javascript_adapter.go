package integrationusage

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/integrationdependency"
	"github.com/dvordrova/repomap/internal/programindex"
)

const (
	typeScriptCallWitness          = "typescript_call"
	javaScriptCallCandidateWitness = "javascript_call_candidate"
)

// prepareJavaScriptTypeScript consumes only external-call authority sealed by
// the project-aware adapter. TypeScript checker resolution may establish one
// exact external symbol. JavaScript and dynamic TypeScript candidates remain
// alternatives or unresolved frontiers and are never promoted locally.
func prepareJavaScriptTypeScript(
	index programindex.Index,
	selected integrationdependency.Result,
) (string, preparedCandidates, Coverage, error) {
	var empty preparedCandidates
	if err := index.Validate(); err != nil {
		return "", empty, Coverage{}, fmt.Errorf("integration usage: ProgramIndex: %w", err)
	}
	if index.Target.Language != "javascript" && index.Target.Language != "typescript" {
		return "", empty, Coverage{}, fmt.Errorf(
			"integration usage: JavaScript/TypeScript adapter received language %q",
			index.Target.Language,
		)
	}
	if index.Coverage.ObjectsOmitted != 0 || index.Coverage.RelationsOmitted != 0 ||
		index.Coverage.WitnessesOmitted != 0 {
		return "", empty, Coverage{}, fmt.Errorf(
			"integration usage: ProgramIndex authority is incomplete (%d objects, %d relations, %d witnesses omitted)",
			index.Coverage.ObjectsOmitted, index.Coverage.RelationsOmitted,
			index.Coverage.WitnessesOmitted,
		)
	}
	if err := selected.Validate(); err != nil {
		return "", empty, Coverage{}, fmt.Errorf("integration usage: integration dependencies: %w", err)
	}
	if selected.Coverage.Omitted != 0 {
		return "", empty, Coverage{}, fmt.Errorf(
			"integration usage: integration dependency authority omitted %d candidates",
			selected.Coverage.Omitted,
		)
	}
	dependenciesSHA256, err := selected.ArtifactSHA256()
	if err != nil {
		return "", empty, Coverage{}, fmt.Errorf("integration usage: integration dependency identity: %w", err)
	}

	dependencyCandidates := make([]dependencyCandidate, 0, len(selected.Dependencies))
	for position, value := range selected.Dependencies {
		dependency := value.Dependency
		if dependency.Language != index.Target.Language {
			return "", empty, Coverage{}, fmt.Errorf(
				"integration usage: selected dependency %q has language %q, want %q",
				dependency.ID, dependency.Language, index.Target.Language,
			)
		}
		if !validJavaScriptPackagePath(dependency.PackagePath) {
			return "", empty, Coverage{}, fmt.Errorf(
				"integration usage: selected dependency %q has invalid JavaScript package path %q",
				dependency.ID, dependency.PackagePath,
			)
		}
		for _, importer := range value.Importers {
			if importer.Language != index.Target.Language {
				return "", empty, Coverage{}, fmt.Errorf(
					"integration usage: selected dependency %q has importer language %q, want %q",
					dependency.ID, importer.Language, index.Target.Language,
				)
			}
		}
		dependencyCandidates = append(dependencyCandidates, dependencyCandidate{
			ref: fmt.Sprintf("d%d", position+1), selected: cloneSelectedDependency(value),
			keys: []string{dependency.PackagePath},
		})
	}

	objects := make(map[string]programindex.Object, len(index.Objects))
	for _, object := range index.Objects {
		objects[object.ID] = object
	}
	coverage := Coverage{DependenciesObserved: len(dependencyCandidates)}
	operations := make([]operationCandidate, 0)
	dependenciesWithOperations := make(map[string]struct{})
	for _, relation := range index.Relations {
		if relation.Kind != programindex.RelationInvokesExternal {
			continue
		}
		coverage.ExternalRelationsObserved++
		coverage.CallsiteCandidatesObserved += relation.WitnessesObserved
		if relation.Resolution == programindex.ResolutionUnresolved {
			coverage.UnresolvedRuntimeRelations++
			coverage.CallsiteCandidatesOmitted += relation.WitnessesObserved
			continue
		}
		if (relation.Resolution != programindex.ResolutionExact &&
			relation.Resolution != programindex.ResolutionAlternatives) ||
			len(relation.ToIDs) != 1 || relation.TargetsObserved != 1 || relation.TargetsOmitted != 0 {
			return "", empty, Coverage{}, fmt.Errorf(
				"integration usage: JavaScript/TypeScript external relation %q has invalid resolution authority",
				relation.ID,
			)
		}
		if relation.Resolution == programindex.ResolutionExact {
			coverage.ExactExternalRelations++
		} else {
			coverage.UnresolvedRuntimeRelations++
		}
		externalSymbol, ok := objects[relation.ToIDs[0]]
		if !ok || externalSymbol.Kind != programindex.ObjectExternalSymbol || externalSymbol.External == nil ||
			!validJavaScriptPackagePath(externalSymbol.External.PackagePath) {
			return "", empty, Coverage{}, fmt.Errorf(
				"integration usage: JavaScript/TypeScript external relation %q has no typed package authority",
				relation.ID,
			)
		}
		caller, ok := objects[relation.FromID]
		if !ok || caller.Location == nil || caller.Name == "" {
			return "", empty, Coverage{}, fmt.Errorf(
				"integration usage: JavaScript/TypeScript external relation %q has no exact caller declaration",
				relation.ID,
			)
		}
		if relation.WitnessesOmitted != 0 || len(relation.Witnesses) == 0 ||
			relation.WitnessesObserved != len(relation.Witnesses) {
			return "", empty, Coverage{}, fmt.Errorf(
				"integration usage: JavaScript/TypeScript external relation %q has incomplete callsite authority",
				relation.ID,
			)
		}
		dependency, matched, err := matchJavaScriptDependency(
			externalSymbol.External.PackagePath, dependencyCandidates,
		)
		if err != nil {
			return "", empty, Coverage{}, fmt.Errorf(
				"integration usage: JavaScript/TypeScript external relation %q: %w", relation.ID, err,
			)
		}
		if !matched {
			coverage.OutOfScopeCandidates += relation.WitnessesObserved
			continue
		}
		for witnessIndex, witness := range relation.Witnesses {
			if !validJavaScriptTypeScriptExternalWitness(
				index.Target.Language, relation.Resolution, witness,
			) {
				return "", empty, Coverage{}, fmt.Errorf(
					"integration usage: JavaScript/TypeScript external relation %q has unsupported witness %d",
					relation.ID, witnessIndex,
				)
			}
			authority := AuthoritySyntacticUnresolved
			if relation.Resolution == programindex.ResolutionExact {
				authority = AuthorityExactExternalSymbol
			}
			operation := Operation{
				Language: index.Target.Language, DependencyID: dependency.selected.Dependency.ID,
				RelationID: relation.ID, WitnessIndex: witnessIndex,
				CallerID: caller.ID, CallerKind: caller.Kind, CallerName: caller.Name,
				CallerLocation: *caller.Location, Callsite: *witness.Location,
				CallExpression: witness.SourceExpression, CanonicalCallee: externalSymbol.Name,
				ExternalSymbolID: externalSymbol.ID, Invocation: relation.Invocation,
				Authority: authority,
			}
			operations = append(operations, operationCandidate{
				dependencyRef: dependency.ref, operation: operation,
			})
			dependenciesWithOperations[dependency.selected.Dependency.ID] = struct{}{}
		}
	}
	if coverage.CallsiteCandidatesObserved != len(operations)+coverage.OutOfScopeCandidates+
		coverage.CallsiteCandidatesOmitted {
		return "", empty, Coverage{}, fmt.Errorf(
			"integration usage: JavaScript/TypeScript callsite coverage is not a complete partition",
		)
	}
	sort.Slice(operations, func(left, right int) bool {
		return operationLess(operations[left].operation, operations[right].operation)
	})
	for position := range operations {
		operations[position].ref = fmt.Sprintf("o%d", position+1)
	}
	coverage.DependenciesWithOperations = len(dependenciesWithOperations)
	coverage.OperationsAdvertised = len(operations)
	coverage.ModelCalled = len(operations) > 0
	return dependenciesSHA256, preparedCandidates{
		dependencies: dependencyCandidates, operations: operations,
	}, coverage, nil
}

func matchJavaScriptDependency(
	packagePath string,
	candidates []dependencyCandidate,
) (dependencyCandidate, bool, error) {
	matches := make([]dependencyCandidate, 0, 1)
	for _, candidate := range candidates {
		if len(candidate.keys) == 1 && candidate.keys[0] == packagePath &&
			len(candidate.selected.Importers) > 0 {
			matches = append(matches, candidate)
		}
	}
	switch len(matches) {
	case 0:
		return dependencyCandidate{}, false, nil
	case 1:
		return matches[0], true, nil
	default:
		return dependencyCandidate{}, false, fmt.Errorf(
			"external package %q has ambiguous dependency authority", packagePath,
		)
	}
}

func validJavaScriptTypeScriptExternalWitness(
	targetLanguage string,
	resolution programindex.Resolution,
	witness programindex.Witness,
) bool {
	validKind := (witness.Kind == typeScriptCallWitness && targetLanguage == "typescript" &&
		resolution == programindex.ResolutionExact) ||
		(witness.Kind == javaScriptCallCandidateWitness &&
			(targetLanguage == "javascript" || targetLanguage == "typescript") &&
			resolution == programindex.ResolutionAlternatives)
	return validKind && witness.Location != nil &&
		validOptionalLine(witness.SourceExpression, programindex.MaxTextBytes)
}

func validJavaScriptPackagePath(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || strings.ContainsAny(value, "\\\x00\r\n") ||
		strings.HasPrefix(value, ".") || strings.HasSuffix(value, "/") || strings.Contains(value, "//") {
		return false
	}
	if strings.HasPrefix(value, "node:") {
		return len(value) > len("node:")
	}
	parts := strings.Split(value, "/")
	if strings.HasPrefix(value, "@") {
		if len(parts) != 2 || len(parts[0]) < 2 || parts[1] == "" {
			return false
		}
	} else if len(parts) != 1 {
		return false
	}
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return false
		}
		for _, character := range part {
			if character <= ' ' || character == '\u007f' {
				return false
			}
		}
	}
	return true
}
