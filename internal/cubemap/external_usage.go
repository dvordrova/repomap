package cubemap

import (
	"fmt"
	"reflect"
	"sort"

	"github.com/dvordrova/repomap/internal/surfacediscovery"
)

type usageCandidate struct {
	ref                  string
	node                 surfacediscovery.DirectCallNode
	dependencyRefs       []string
	familiesByDependency map[string][]surfacediscovery.ExternalCallFamily
}

// discoverExternalUsages joins exact external SSA call families to the
// model-selected dependency rows. It performs no package, operation, or
// framework classification and never rereads repository source.
func discoverExternalUsages(
	index surfacediscovery.DirectCallIndex,
	externalIndex surfacediscovery.ExternalCallIndex,
	selectedDependencies []dependencyCandidate,
) ([]usageCandidate, int, error) {
	dependenciesByPackage := make(map[string][]dependencyCandidate)
	for _, dependency := range selectedDependencies {
		dependenciesByPackage[dependency.value.PackagePath] = append(
			dependenciesByPackage[dependency.value.PackagePath], dependency,
		)
	}
	externalCallers := make(map[string]surfacediscovery.DirectCallNode, len(externalIndex.Callers))
	for _, caller := range externalIndex.Callers {
		externalCallers[caller.ID] = caller
	}
	byCaller := make(map[string]*usageCandidate)
	matchedFamilies := 0
	for _, family := range externalIndex.Families {
		dependenciesForPackage := dependenciesByPackage[family.Target.PackagePath]
		if len(dependenciesForPackage) == 0 {
			continue
		}
		if len(dependenciesForPackage) != 1 {
			return nil, 0, fmt.Errorf(
				"cubemap: external package %q has ambiguous selected dependency authority",
				family.Target.PackagePath,
			)
		}
		externalCaller, exists := externalCallers[family.CallerID]
		if !exists {
			return nil, 0, fmt.Errorf("cubemap: external call family has no exact caller")
		}
		graphCaller, exists := index.Node(family.CallerID)
		if !exists || !reflect.DeepEqual(graphCaller, externalCaller) {
			return nil, 0, fmt.Errorf("cubemap: external call caller does not match direct-call authority")
		}
		dependency := dependenciesForPackage[0]
		candidate := byCaller[family.CallerID]
		if candidate == nil {
			candidate = &usageCandidate{
				node: graphCaller, familiesByDependency: make(map[string][]surfacediscovery.ExternalCallFamily),
			}
			byCaller[family.CallerID] = candidate
		}
		if _, exists := candidate.familiesByDependency[dependency.ref]; !exists {
			candidate.dependencyRefs = append(candidate.dependencyRefs, dependency.ref)
		}
		family.Callsites = append([]surfacediscovery.Location(nil), family.Callsites...)
		candidate.familiesByDependency[dependency.ref] = append(
			candidate.familiesByDependency[dependency.ref], family,
		)
		matchedFamilies++
	}

	result := make([]usageCandidate, 0, len(byCaller))
	for _, candidate := range byCaller {
		sort.Strings(candidate.dependencyRefs)
		for dependencyRef := range candidate.familiesByDependency {
			sort.Slice(candidate.familiesByDependency[dependencyRef], func(i, j int) bool {
				return candidate.familiesByDependency[dependencyRef][i].ID <
					candidate.familiesByDependency[dependencyRef][j].ID
			})
		}
		result = append(result, *candidate)
	}
	sort.Slice(result, func(i, j int) bool {
		return symbolKey(symbolFromNode(result[i].node)) < symbolKey(symbolFromNode(result[j].node))
	})
	return result, matchedFamilies, nil
}
