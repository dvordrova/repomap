package report

import (
	"encoding/json"
	"sort"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

// pagePathStep is one declaration on an input's path as its reading lists
// it: a link into its code (Source is its place, for a tooltip only), the
// part it stands in, and how deep it is under the handler.
type pagePathStep struct {
	Name      string `json:"name"`
	Href      string `json:"href,omitempty"`
	Open      string `json:"open,omitempty"`
	Source    string `json:"source,omitempty"`
	NoSource  bool   `json:"no_source,omitempty"`
	Part      string `json:"part,omitempty"`
	PartTitle string `json:"part_title,omitempty"`
	Depth     int    `json:"depth,omitempty"`
	Possible  bool   `json:"possible,omitempty"`
	Read      bool   `json:"read,omitempty"`
}

// pageSharedPath is a dispatch site whose alternatives include the input's
// handler: every input handled by one of those alternatives is dispatched
// there, so the reading names it once, with the declaration it dispatches
// through and how many alternatives it chooses between. The path by which
// an input reaches that declaration is not established here: a route chosen
// by length from the program's entry ran Redis's main → aeMain → beforeSleep
// → call, which a benchmark reader was offered as GET's path, and no route
// is shown instead.
type pageSharedPath struct {
	Inputs  int    `json:"inputs"`
	All     bool   `json:"all,omitempty"`
	Through string `json:"through"`
	Of      int    `json:"of"`
}

// pageInputPath is an input's path for its reading: the dispatch sites it
// shares with other inputs, then its own steps from its handler.
type pageInputPath struct {
	Shared []pageSharedPath `json:"shared,omitempty"`
	Own    []pagePathStep   `json:"own,omitempty"`
}

// pathStep is a declaration as a step of an input's path.
func (builder *pageBuilder) pathStep(targetID, subject string, part func(string) (string, string)) pagePathStep {
	step := pagePathStep{Name: subject}
	if ref, known := builder.subject(targetID, subject); known {
		name, anchor := builder.subjectDisplay(ref.subject)
		if name != "" {
			step.Name = name
		}
		if anchor != nil {
			step.Href, step.Open, step.Source, step.NoSource = anchor.Href, anchor.Open, anchor.Text, anchor.NoSource
		}
	}
	step.Part, step.PartTitle = part(subject)
	return step
}

// inputPath is an input's path for its reading. Shared: each dispatch site
// whose alternatives hold the handler and handle at least one other input,
// named by the declaration it dispatches through. Own: the handler's witness
// tree, every reached part's shortest witness merged into one tree, a
// callee under its caller, the branches in the order their parts were
// reached; nothing in it claims an order of execution between branches.
func (builder *pageBuilder) inputPath(index *groupindex.Index, operation groupindex.Operation, reached []string, firstInGroup map[string]string,
	parents map[string]groupindex.StructuralEdge, part func(string) (string, string)) string {
	var path pageInputPath
	targetID := index.Target.ID
	handled := map[string]int{}
	for _, other := range index.Operations {
		handled[other.SubjectID]++
	}
	through := map[string]bool{}
	for _, site := range builder.dispatch(targetID).sites {
		if through[site.from] || !containsString(site.targets, operation.SubjectID) {
			continue
		}
		inputs := 0
		for _, target := range site.targets {
			inputs += handled[target]
		}
		if inputs < 2 {
			continue
		}
		through[site.from] = true
		path.Shared = append(path.Shared, pageSharedPath{Inputs: inputs, All: inputs == len(index.Operations), Of: len(site.targets),
			Through: builder.pathStep(targetID, site.from, part).Name})
	}
	// The witness tree: the chain from the handler to each reached part's
	// first declaration, the branch toward an earlier reached part first.
	rank := map[string]int{}
	children := map[string][]string{}
	destinations := make([]string, 0, len(firstInGroup))
	for node := range firstInGroup {
		destinations = append(destinations, node)
	}
	order := map[string]int{}
	for i, node := range reached {
		order[node] = i
	}
	sort.Slice(destinations, func(i, j int) bool {
		left, leftReached := order[destinations[i]]
		right, rightReached := order[destinations[j]]
		if leftReached != rightReached {
			return leftReached
		}
		if left != right {
			return left < right
		}
		return destinations[i] < destinations[j]
	})
	for position, node := range destinations {
		var chain []string
		for at := firstInGroup[node]; ; {
			chain = append(chain, at)
			if at == operation.SubjectID {
				break
			}
			edge, ok := parents[at]
			if !ok {
				chain = nil
				break
			}
			at = edge.FromSubjectID
		}
		for i, subject := range chain {
			if previous, ranked := rank[subject]; !ranked || position < previous {
				rank[subject] = position
			}
			if i+1 < len(chain) && !containsString(children[chain[i+1]], subject) {
				children[chain[i+1]] = append(children[chain[i+1]], subject)
			}
		}
	}
	if _, reachedAny := rank[operation.SubjectID]; reachedAny || len(path.Shared) > 0 {
		var walk func(subject string, depth int)
		walk = func(subject string, depth int) {
			step := builder.pathStep(targetID, subject, part)
			step.Depth = depth
			if edge, ok := parents[subject]; ok && subject != operation.SubjectID {
				step.Possible = edge.Resolution != programindex.ResolutionExact
				step.Read = edge.RelationKind == programindex.RelationReads
			}
			path.Own = append(path.Own, step)
			next := children[subject]
			sort.SliceStable(next, func(i, j int) bool { return rank[next[i]] < rank[next[j]] })
			for _, child := range next {
				walk(child, depth+1)
			}
		}
		walk(operation.SubjectID, 0)
	}
	if len(path.Shared) == 0 && len(path.Own) <= 1 {
		return ""
	}
	raw, err := json.Marshal(path)
	if err != nil {
		return ""
	}
	return string(raw)
}

// remapInputPath renames the parts an input's path names, as the map's
// node IDs are renamed when its maps are scoped or joined.
func remapInputPath(raw string, rename func(string) string) string {
	if raw == "" {
		return raw
	}
	var path pageInputPath
	if json.Unmarshal([]byte(raw), &path) != nil {
		return raw
	}
	steps := func(list []pagePathStep) {
		for i := range list {
			if list[i].Part != "" {
				list[i].Part = rename(list[i].Part)
			}
		}
	}
	steps(path.Own)
	encoded, err := json.Marshal(path)
	if err != nil {
		return raw
	}
	return string(encoded)
}
