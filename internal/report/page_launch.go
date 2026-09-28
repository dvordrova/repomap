package report

import (
	"encoding/json"

	"github.com/dvordrova/repomap/internal/groupindex"
)

// pageLaunch is the Inputs reading's fold "How these were found"
// (GroupsIndex Launch): each launch function that holds inputs, by the
// chain of calls from where the program starts; each outside symbol's
// idiom line (model); the calls that may declare an input and were not
// decided; the calls the code cannot follow; and how many other functions
// the walk reached that declare none, as a count only.
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

type pageLaunchUnsure struct {
	Function int    `json:"function"`
	Symbol   string `json:"symbol"`
	Reason   string `json:"reason"`
	Line     int    `json:"line"`
	Href     string `json:"href,omitempty"`
	Open     string `json:"open,omitempty"`
}

type pageLaunchClosed struct {
	Function int        `json:"function"`
	Sites    []pageSite `json:"sites"`
}

type pageSite struct {
	Line int    `json:"line"`
	Href string `json:"href,omitempty"`
	Open string `json:"open,omitempty"`
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
	chain := func(function groupindex.LaunchFunction) []int {
		var subjects []string
		for seen := 0; seen < 64; seen++ {
			subjects = append([]string{function.SubjectID}, subjects...)
			if function.Via < 0 {
				break
			}
			from := index.StructuralEdges[function.Via].FromSubjectID
			position, ok := at[from]
			if !ok {
				subjects = append([]string{from}, subjects...)
				break
			}
			function = launch.Functions[position]
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
				anchor := builder.links.anchor(call.Location.Path, call.Location.Line, call.Location.Column)
				result.Unsure = append(result.Unsure, pageLaunchUnsure{Function: decls.of(function.SubjectID), Symbol: call.Symbol, Reason: call.Reason, Line: call.Location.Line, Href: anchor.Href, Open: anchor.Open})
			}
		case "closed":
			closed := pageLaunchClosed{Function: decls.of(function.SubjectID)}
			for _, position := range function.Closed {
				edge := index.StructuralEdges[position]
				site := pageSite{}
				if ref, ok := builder.subject(index.Target.ID, edge.ToSubjectID); ok && ref.subject.Pattern != nil && ref.subject.Pattern.Location != nil {
					location := ref.subject.Pattern.Location
					anchor := builder.links.anchor(location.Path, location.Line, location.Column)
					site = pageSite{Line: location.Line, Href: anchor.Href, Open: anchor.Open}
				}
				closed.Sites = append(closed.Sites, site)
			}
			result.Closed = append(result.Closed, closed)
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
