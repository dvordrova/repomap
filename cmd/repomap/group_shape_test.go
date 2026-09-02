package main

import (
	"reflect"
	"testing"

	"github.com/dvordrova/repomap/internal/groupindex"
)

func TestFormatGroupShapeNamesTheBucket(t *testing.T) {
	index := groupindex.Index{
		Subjects: make([]groupindex.Subject, 424),
		Groups: []groupindex.Group{
			{Title: "HTTP service layer", Lane: groupindex.LaneCore, MemberSubjectIDs: repeatIDs("a", 8)},
			{Title: "Simulation rendering", Lane: groupindex.LaneCore, MemberSubjectIDs: repeatIDs("b", 127)},
			{Title: "External dependencies", Lane: groupindex.LaneDependencies, MemberSubjectIDs: repeatIDs("c", 17)},
		},
		Connections: make([]groupindex.Connection, 12),
	}
	want := []string{
		"groups: 3 (core 2, dependencies 1)",
		"subjects in a group: 152/424 (35%)",
		"largest group: Simulation rendering, 127/424 (29%) of this target",
		"local connections: 12",
	}
	if got := formatGroupShape(index); !reflect.DeepEqual(got, want) {
		t.Fatalf("details =\n%#v\nwant\n%#v", got, want)
	}
}

func TestFormatGroupShapeWithoutGroups(t *testing.T) {
	want := []string{"groups: 0 (no lanes)", "subjects in a group: 0/0", "local connections: 0"}
	if got := formatGroupShape(groupindex.Index{}); !reflect.DeepEqual(got, want) {
		t.Fatalf("details = %#v, want %#v", got, want)
	}
}

func repeatIDs(prefix string, count int) []string {
	ids := make([]string, count)
	for position := range ids {
		ids[position] = prefix + string(rune('a'+position%26)) + string(rune('a'+position/26))
	}
	return ids
}
