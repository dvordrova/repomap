package integrationusage

import (
	"fmt"
	"sort"

	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/integrationdependency"
	"github.com/dvordrova/repomap/internal/programindex"
)

const (
	goStaticCallWitness            = "go_external_static_call"
	goDeclaredInterfaceCallWitness = "go_declared_interface_dispatch"
)

// prepareGo interprets only facts that the Go adapter sealed explicitly:
// exact invokes_external relations, typed external-symbol package authority,
// exact caller containment, typed SSA witness kinds, and exact importers from
// the selected dependency result. Unresolved call frontiers remain coverage;
// they are never promoted into operations.
func prepareGo(
	index programindex.Index,
	selected integrationdependency.Result,
) (string, preparedCandidates, Coverage, error) {
	var empty preparedCandidates
	if err := index.Validate(); err != nil {
		return "", empty, Coverage{}, fmt.Errorf("integration usage: ProgramIndex: %w", err)
	}
	if index.Target.Language != "go" {
		return "", empty, Coverage{}, fmt.Errorf(
			"integration usage: Go adapter received language %q", index.Target.Language,
		)
	}
	// The Go adapter builds invokes_external from its independently complete
	// external-call ledger. Global ProgramIndex omissions belong to other call
	// and dynamic-handoff shapes; they remain upstream frontier accounting but
	// cannot erase an exact external relation retained below. Every advertised
	// operation still requires exact endpoints and typed witnesses.
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
		if dependency.Language != "go" {
			return "", empty, Coverage{}, fmt.Errorf(
				"integration usage: selected dependency %q has language %q", dependency.ID, dependency.Language,
			)
		}
		if dependency.PackagePath == "" {
			return "", empty, Coverage{}, fmt.Errorf(
				"integration usage: selected Go dependency %q has no package path", dependency.ID,
			)
		}
		for _, importer := range value.Importers {
			if importer.Language != "go" {
				return "", empty, Coverage{}, fmt.Errorf(
					"integration usage: selected dependency %q has non-Go importer", dependency.ID,
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
		if relation.Resolution != programindex.ResolutionExact || len(relation.ToIDs) != 1 ||
			relation.TargetsObserved != 1 || relation.TargetsOmitted != 0 {
			return "", empty, Coverage{}, fmt.Errorf(
				"integration usage: Go external relation %q is neither exact nor an explicit unresolved frontier",
				relation.ID,
			)
		}
		coverage.ExactExternalRelations++
		externalSymbol, ok := objects[relation.ToIDs[0]]
		if !ok || externalSymbol.Kind != programindex.ObjectExternalSymbol || externalSymbol.External == nil {
			return "", empty, Coverage{}, fmt.Errorf(
				"integration usage: Go external relation %q has no typed external-symbol authority", relation.ID,
			)
		}
		if relation.WitnessesOmitted != relation.WitnessesObserved-len(relation.Witnesses) ||
			len(relation.Witnesses) == 0 {
			return "", empty, Coverage{}, fmt.Errorf(
				"integration usage: Go external relation %q has invalid witness coverage", relation.ID,
			)
		}
		caller, ok := objects[relation.FromID]
		if !ok || caller.Location == nil || caller.Name == "" {
			return "", empty, Coverage{}, fmt.Errorf(
				"integration usage: Go external relation %q has no exact caller declaration", relation.ID,
			)
		}
		pkg, err := goPackageFor(caller.ID, objects)
		if err != nil {
			return "", empty, Coverage{}, fmt.Errorf(
				"integration usage: Go external relation %q caller: %w", relation.ID, err,
			)
		}
		dependency, matched, err := matchGoDependency(
			externalSymbol.External.PackagePath, pkg, dependencyCandidates,
		)
		if err != nil {
			return "", empty, Coverage{}, fmt.Errorf(
				"integration usage: Go external relation %q: %w", relation.ID, err,
			)
		}
		if !matched {
			coverage.OutOfScopeCandidates += relation.WitnessesObserved
			continue
		}
		coverage.CallsiteCandidatesOmitted += relation.WitnessesOmitted
		for witnessIndex, witness := range relation.Witnesses {
			if (witness.Kind != goStaticCallWitness && witness.Kind != goDeclaredInterfaceCallWitness) ||
				witness.Location == nil || witness.SourceExpression != "" {
				return "", empty, Coverage{}, fmt.Errorf(
					"integration usage: Go external relation %q has unsupported typed witness %d",
					relation.ID, witnessIndex,
				)
			}
			operation := Operation{
				Language: "go", DependencyID: dependency.selected.Dependency.ID,
				RelationID: relation.ID, WitnessIndex: witnessIndex,
				CallerID: caller.ID, CallerKind: caller.Kind, CallerName: caller.Name,
				CallerLocation: *caller.Location, Callsite: *witness.Location,
				CanonicalCallee: externalSymbol.Name, ExternalSymbolID: externalSymbol.ID,
				Invocation: relation.Invocation, Authority: AuthorityExactExternalSymbol,
			}
			operations = append(operations, operationCandidate{
				dependencyRef: dependency.ref, operation: operation,
			})
			dependenciesWithOperations[dependency.selected.Dependency.ID] = struct{}{}
		}
	}
	if coverage.CallsiteCandidatesObserved != len(operations)+coverage.OutOfScopeCandidates+
		coverage.CallsiteCandidatesOmitted {
		return "", empty, Coverage{}, fmt.Errorf("integration usage: Go callsite coverage is not a complete partition")
	}
	if len(operations) > MaxAdvertisedOperations {
		return "", empty, Coverage{}, fmt.Errorf(
			"integration usage: %d operations exceed the complete run bound %d",
			len(operations), MaxAdvertisedOperations,
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

func goPackageFor(
	objectID string,
	objects map[string]programindex.Object,
) (programindex.Object, error) {
	seen := make(map[string]struct{})
	for objectID != "" {
		if _, duplicate := seen[objectID]; duplicate {
			return programindex.Object{}, fmt.Errorf("object containment contains a cycle")
		}
		seen[objectID] = struct{}{}
		object, ok := objects[objectID]
		if !ok {
			return programindex.Object{}, fmt.Errorf("unknown object %q", objectID)
		}
		if object.Kind == programindex.ObjectPackage {
			return object, nil
		}
		if object.ContainerID != "" {
			objectID = object.ContainerID
		} else {
			objectID = object.OwnerID
		}
	}
	return programindex.Object{}, fmt.Errorf("object has no exact package container")
}

func matchGoDependency(
	packagePath string,
	callerPackage programindex.Object,
	candidates []dependencyCandidate,
) (dependencyCandidate, bool, error) {
	matches := make([]dependencyCandidate, 0, 1)
	for _, candidate := range candidates {
		if len(candidate.keys) != 1 || candidate.keys[0] != packagePath ||
			!goPackageMatchesImporters(callerPackage, candidate.selected.Importers) {
			continue
		}
		matches = append(matches, candidate)
	}
	if len(matches) == 0 {
		return dependencyCandidate{}, false, nil
	}
	if len(matches) != 1 {
		return dependencyCandidate{}, false, fmt.Errorf(
			"external package %q has ambiguous dependency authority", packagePath,
		)
	}
	return matches[0], true, nil
}

func goPackageMatchesImporters(pkg programindex.Object, importers []dependencies.Importer) bool {
	if pkg.Kind != programindex.ObjectPackage {
		return false
	}
	for _, importer := range importers {
		if importer.Language == "go" && importer.PackagePath == pkg.Name {
			return true
		}
	}
	return false
}
