package run

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/groupindex"
)

// formatGroupShape prints what the grouping actually produced, not just how
// many groups there are. A group count alone cannot distinguish seven readable
// groups from six readable ones plus a bucket holding a third of the target,
// and a bucket is what routes no attention on the page.
func formatGroupShape(index groupindex.Index) []string {
	members := make(map[string]struct{}, len(index.Subjects))
	byLane := make(map[groupindex.Lane]int, 3)
	largest, largestTitle := 0, ""
	for _, group := range index.Groups {
		byLane[group.Lane]++
		for _, member := range group.MemberSubjectIDs {
			members[member] = struct{}{}
		}
		if len(group.MemberSubjectIDs) > largest {
			largest, largestTitle = len(group.MemberSubjectIDs), group.Title
		}
	}
	details := []string{
		fmt.Sprintf("groups: %d (%s)", len(index.Groups), formatGroupLanes(byLane)),
		"subjects in a group: " + formatCoverageFraction(len(members), len(index.Subjects)),
	}
	if largest > 0 {
		details = append(details, fmt.Sprintf(
			"largest group: %s, %s of this target",
			largestTitle, formatCoverageFraction(largest, len(index.Subjects)),
		))
	}
	return append(details, fmt.Sprintf("local connections: %d", len(index.Connections)))
}

func formatGroupLanes(byLane map[groupindex.Lane]int) string {
	lanes := make([]string, 0, len(byLane))
	for lane, count := range byLane {
		lanes = append(lanes, fmt.Sprintf("%s %d", lane, count))
	}
	sort.Strings(lanes)
	if len(lanes) == 0 {
		return "no lanes"
	}
	return strings.Join(lanes, ", ")
}
