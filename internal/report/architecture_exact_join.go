package report

import (
	"fmt"
	"sort"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/evidence"
)

// exactCanvasDeclarationTarget is a neutral exact Canvas declaration used by
// source-navigation consumers. It is not entry-handoff or Mechanism authority.
type exactCanvasDeclarationTarget struct {
	Label  string
	Path   string
	Line   int
	Column int
	Symbol string
}

func architectureDeclarationLocationsCompatible(left, right evidence.Location) bool {
	if left.Path != right.Path || left.Line != right.Line {
		return false
	}
	return left.Column == 0 || right.Column == 0 || left.Column == right.Column
}

// architectureComponentIDsForExactDeclaration joins one exact repository
// declaration to every accepted non-remainder Architecture component that
// carries it. The exact symbol-member join is authoritative. Only when that
// join has no owner may the declaration path fall back through the exact
// RepositoryGraph package-file inventory to an exact Canvas package member.
//
// This helper is intentionally product-neutral: an entry handoff and a Study
// mechanism may reuse the same Architecture ownership evidence, but neither
// can create ownership through names, basenames, directory prefixes, model
// prose, structural edges, or presentation order.
func architectureComponentIDsForExactDeclaration(
	canvas *ArchitectureCanvas,
	repositoryGraph *RepositoryGraph,
	symbol string,
	location evidence.Location,
) []componentmap.ComponentID {
	if canvas == nil || symbol == "" || !validGroundingLocation(location) {
		return []componentmap.ComponentID{}
	}

	var declarationOwners []componentmap.ComponentID
	for _, component := range canvas.Components {
		if component.ID == "" ||
			(canvas.LocalRemainderComponentID != "" && component.ID == canvas.LocalRemainderComponentID) {
			continue
		}
		if architectureComponentHasExactDeclaration(component, symbol, location) {
			declarationOwners = append(declarationOwners, component.ID)
		}
	}
	declarationOwners = sortedArchitectureComponentIDs(declarationOwners)
	if len(declarationOwners) > 0 {
		return declarationOwners
	}

	return architectureComponentIDsForExactPackageMember(
		canvas,
		repositoryGraph,
		location.Path,
	)
}

func architectureComponentHasExactDeclaration(
	component ArchitectureComponent,
	symbol string,
	location evidence.Location,
) bool {
	for _, members := range [][]componentmap.Candidate{component.Members, component.SharedMembers} {
		for _, candidate := range members {
			if architectureCandidateHasExactDeclaration(candidate, symbol, location) {
				return true
			}
		}
	}
	return false
}

func architectureCandidateHasExactDeclaration(
	candidate componentmap.Candidate,
	symbol string,
	location evidence.Location,
) bool {
	if candidate.ID.Kind != componentmap.MemberSymbol {
		return false
	}
	for _, fact := range candidate.Facts {
		if fact.Kind == componentmap.FactDeclaration && fact.Value == symbol &&
			fact.Location != nil &&
			architectureDeclarationLocationsCompatible(*fact.Location, location) {
			return true
		}
	}
	return false
}

// architectureComponentIDsForExactPackageMember is the bounded fallback for
// an exact repository declaration that was not retained as a Canvas symbol.
// It performs only these backend-owned equality joins:
//
//	declaration path == RepositoryGraph.PackageInfo.Files item
//	package canonical path == accepted Canvas package declaration fact
//
// Every exact non-remainder owner is returned so 0/1/N ownership stays
// explicit. Package-directory prefixes, basenames, symbols, component names,
// model prose, structural edges, and sorted-first selection never participate.
func architectureComponentIDsForExactPackageMember(
	canvas *ArchitectureCanvas,
	repositoryGraph *RepositoryGraph,
	declarationPath string,
) []componentmap.ComponentID {
	if canvas == nil || repositoryGraph == nil || declarationPath == "" {
		return []componentmap.ComponentID{}
	}
	packages := make(map[string]struct{})
	for _, pkg := range repositoryGraph.Packages {
		if pkg.CanonicalPath == "" {
			continue
		}
		for _, filePath := range pkg.Files {
			if filePath == declarationPath {
				packages[pkg.CanonicalPath] = struct{}{}
				break
			}
		}
	}
	if len(packages) == 0 {
		return []componentmap.ComponentID{}
	}

	var result []componentmap.ComponentID
	for _, component := range canvas.Components {
		if component.ID == "" ||
			(canvas.LocalRemainderComponentID != "" && component.ID == canvas.LocalRemainderComponentID) {
			continue
		}
		matched := false
		for _, members := range [][]componentmap.Candidate{component.Members, component.SharedMembers} {
			for _, candidate := range members {
				if architectureCandidateHasExactPackage(candidate, packages) {
					matched = true
					break
				}
			}
			if matched {
				break
			}
		}
		if matched {
			result = append(result, component.ID)
		}
	}
	return sortedArchitectureComponentIDs(result)
}

func architectureCandidateHasExactPackage(
	candidate componentmap.Candidate,
	packages map[string]struct{},
) bool {
	if candidate.ID.Kind != componentmap.MemberPackage {
		return false
	}
	for _, fact := range candidate.Facts {
		if fact.Kind != componentmap.FactDeclaration {
			continue
		}
		if _, exists := packages[fact.Value]; exists {
			return true
		}
	}
	return false
}

func sortedArchitectureComponentIDs(
	values []componentmap.ComponentID,
) []componentmap.ComponentID {
	seen := make(map[componentmap.ComponentID]struct{}, len(values))
	for _, value := range values {
		if value != "" {
			seen[value] = struct{}{}
		}
	}
	result := make([]componentmap.ComponentID, 0, len(seen))
	for value := range seen {
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func processEntryMemberIDs(anchor *componentmap.BehaviorAnchor) []componentmap.MemberID {
	if anchor == nil {
		return nil
	}
	seen := make(map[componentmap.MemberID]struct{}, len(anchor.MemberIDs))
	for _, memberID := range anchor.MemberIDs {
		if memberID.Kind != componentmap.MemberSymbol || memberID.Value == "" {
			continue
		}
		seen[memberID] = struct{}{}
	}
	result := make([]componentmap.MemberID, 0, len(seen))
	for memberID := range seen {
		result = append(result, memberID)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Kind != result[j].Kind {
			return result[i].Kind < result[j].Kind
		}
		return result[i].Value < result[j].Value
	})
	return result
}

func memberIDEquals(left, right componentmap.MemberID) bool {
	return left.Kind == right.Kind && left.Value == right.Value
}

func containsMemberID(values []componentmap.MemberID, target componentmap.MemberID) bool {
	for _, value := range values {
		if memberIDEquals(value, target) {
			return true
		}
	}
	return false
}

func localRelationHasScenario(relation componentmap.LocalRelation, scenarioID string) bool {
	if scenarioID == "" {
		return false
	}
	for _, scenario := range relation.Scenarios {
		if scenario.ID == scenarioID {
			return true
		}
	}
	return false
}

// exactCanvasDeclarationTargetForMemberID restores one exact declaration only
// when accepted non-remainder Canvas membership makes it unambiguous. It is a
// source-navigation join and cannot establish an entry handoff or Mechanism.
func exactCanvasDeclarationTargetForMemberID(
	canvas *ArchitectureCanvas,
	memberID componentmap.MemberID,
) *exactCanvasDeclarationTarget {
	if canvas == nil || memberID.Kind != componentmap.MemberSymbol || memberID.Value == "" {
		return nil
	}
	var result *exactCanvasDeclarationTarget
	resultKey := ""
	for _, component := range canvas.Components {
		if canvas.LocalRemainderComponentID != "" && component.ID == canvas.LocalRemainderComponentID {
			continue
		}
		for _, members := range [][]componentmap.Candidate{component.Members, component.SharedMembers} {
			for _, member := range members {
				if !memberIDEquals(member.ID, memberID) {
					continue
				}
				for _, fact := range member.Facts {
					if fact.Kind != componentmap.FactDeclaration || fact.Value == "" ||
						fact.Location == nil || !validGroundingLocation(*fact.Location) {
						continue
					}
					candidate := &exactCanvasDeclarationTarget{
						Label:  fact.Value,
						Path:   fact.Location.Path,
						Line:   fact.Location.Line,
						Column: fact.Location.Column,
						Symbol: fact.Value,
					}
					key := exactCanvasDeclarationTargetKey(candidate)
					if result == nil {
						result, resultKey = candidate, key
						continue
					}
					if key != resultKey {
						return nil
					}
				}
			}
		}
	}
	return result
}

func exactCanvasDeclarationTargetKey(target *exactCanvasDeclarationTarget) string {
	if target == nil {
		return ""
	}
	return fmt.Sprintf(
		"%s\x00%s\x00%d\x00%d\x00%s",
		target.Label,
		target.Path,
		target.Line,
		target.Column,
		target.Symbol,
	)
}
