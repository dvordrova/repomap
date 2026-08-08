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
