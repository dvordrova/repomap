package report

import (
	"encoding/json"
	"slices"

	"github.com/dvordrova/repomap/internal/groupindex"
)

// pageLaunch is the Inputs reading's fold "How these were found"
// (GroupsIndex Launch): each launch function that holds inputs, by the
// chain of calls from where the program starts; each outside symbol's
// idiom line (model); the functions making calls that may declare an input
// and were not decided; the functions with calls the code cannot follow,
// each with their count; and how many other functions the walk reached
// that declare none, as a count only. A function is named once, by its
// declaration: no line numbers.
type pageLaunch struct {
	Found   []pageLaunchFound  `json:"found,omitempty"`
	Idioms  []pageLaunchIdiom  `json:"idioms,omitempty"`
	Unsure  []pageLaunchUnsure `json:"unsure,omitempty"`
	Closed  []pageLaunchClosed `json:"closed,omitempty"`
	Nothing int                `json:"nothing"`
	Roots   []int              `json:"roots,omitempty"`
	Decls   []pageDecl         `json:"decls"`
}

type pageLaunchFound struct {
	Chain  []int    `json:"chain"`
	Inputs []string `json:"inputs"`
}

type pageLaunchIdiom struct {
	Symbol    string `json:"symbol"`
	Kind      string `json:"kind"`
	Entries   int    `json:"entries"`
	Calls     int    `json:"calls"`
	Functions []int  `json:"functions,omitempty"`
}

// pageLaunchUnsure is a function making undecided calls of one symbol for
// one reason, named once however many such calls it makes (owner,
// 2026-09-29: no line numbers; the name reads its code).
type pageLaunchUnsure struct {
	Function int    `json:"function"`
	Symbol   string `json:"symbol"`
	Reason   string `json:"reason"`
}

// pageLaunchClosed is a function with calls the code cannot follow, and how
// many.
type pageLaunchClosed struct {
	Function int `json:"function"`
	Calls    int `json:"calls"`
}

// launchReading projects a target's launch walk for its Inputs reading.
func (builder *pageBuilder) launchReading(index *groupindex.Index, partOf func(string) string, inputNode func(string) string, shown func(string) bool) string {
	launch := index.Launch
	if len(launch.Functions) == 0 && len(index.Unsure) == 0 && len(index.Idioms) == 0 {
		return ""
	}
	decls := builder.pathDecls(index.Target.ID, partOf)
	result := pageLaunch{}
	at := make(map[string]int, len(launch.Functions))
	for position, function := range launch.Functions {
		at[function.SubjectID] = position
	}
	for _, root := range launch.Roots {
		result.Roots = append(result.Roots, decls.of(root))
	}
	// The walk reached each function from one it had reached before, so the
	// chain of Via edges ends at a root.
	chain := func(function groupindex.LaunchFunction) []int {
		subjects := []string{function.SubjectID}
		for function.Via >= 0 {
			function = launch.Functions[at[index.StructuralEdges[function.Via].FromSubjectID]]
			subjects = append([]string{function.SubjectID}, subjects...)
		}
		positions := make([]int, len(subjects))
		for i, subject := range subjects {
			positions[i] = decls.of(subject)
		}
		return positions
	}
	for _, function := range launch.Functions {
		switch function.Outcome() {
		case "found":
			found := pageLaunchFound{Chain: chain(function)}
			for _, id := range function.Found {
				if shown(id) {
					found.Inputs = append(found.Inputs, inputNode(id))
				}
			}
			if len(found.Inputs) > 0 {
				result.Found = append(result.Found, found)
			}
		case "unsure":
			for _, position := range function.Unsure {
				call := index.Unsure[position]
				unsure := pageLaunchUnsure{Function: decls.of(function.SubjectID), Symbol: call.Symbol, Reason: call.Reason}
				if !slices.Contains(result.Unsure, unsure) {
					result.Unsure = append(result.Unsure, unsure)
				}
			}
		case "closed":
			result.Closed = append(result.Closed, pageLaunchClosed{Function: decls.of(function.SubjectID), Calls: len(function.Closed)})
		default:
			result.Nothing++
		}
	}
	for _, idiom := range index.Idioms {
		row := pageLaunchIdiom{Symbol: idiom.Symbol, Kind: idiom.Kind, Entries: idiom.Entries, Calls: idiom.Calls}
		for _, subject := range idiom.SubjectIDs {
			row.Functions = append(row.Functions, decls.of(subject))
		}
		result.Idioms = append(result.Idioms, row)
	}
	result.Decls = decls.list
	raw, err := json.Marshal(result)
	if err != nil {
		return ""
	}
	return string(raw)
}

// remapLaunch renames the parts and inputs a launch reading names.
func remapLaunch(raw string, rename func(string) string) string {
	if raw == "" {
		return raw
	}
	var launch pageLaunch
	if json.Unmarshal([]byte(raw), &launch) != nil {
		return raw
	}
	for i := range launch.Decls {
		if launch.Decls[i].Part != "" {
			launch.Decls[i].Part = rename(launch.Decls[i].Part)
		}
	}
	for i := range launch.Found {
		for j := range launch.Found[i].Inputs {
			launch.Found[i].Inputs[j] = rename(launch.Found[i].Inputs[j])
		}
	}
	encoded, err := json.Marshal(launch)
	if err != nil {
		return raw
	}
	return string(encoded)
}
