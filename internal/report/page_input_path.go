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

// pageSharedPath is the chain from the program's entry to a dispatch site
// whose alternatives include the input's handler: every input handled by
// one of those alternatives shares it, so the reading shows it once, folded
// into one box, before the input's own steps.
type pageSharedPath struct {
	Inputs  int            `json:"inputs"`
	All     bool           `json:"all,omitempty"`
	Through string         `json:"through"`
	Of      int            `json:"of"`
	Steps   []pagePathStep `json:"steps"`
}

// pageInputPath is an input's path for its reading: the chains it shares
// with other inputs, then its own steps from its handler.
type pageInputPath struct {
	Shared []pageSharedPath `json:"shared,omitempty"`
	Own    []pagePathStep   `json:"own,omitempty"`
}

type entryEdge struct {
	to       string
	possible bool
	location *programindex.Location
}

// entryGraph is a target's calls as the chain from its entry follows them:
// the calls the adapter resolved, and the calls it left open whose stored
// ends the index names (Redis's aeProcessEvents calls fe->rfileProc, which
// readQueryFromClient is stored in).
type entryGraph struct {
	seeds []string
	adj   map[string][]entryEdge
	// chains is each dispatch site's chain, by relation.
	chains map[string][]entryStep
}

type entryStep struct {
	subject  string
	possible bool
}

func newEntryGraph(index *groupindex.Index) *entryGraph {
	graph := &entryGraph{adj: map[string][]entryEdge{}, chains: map[string][]entryStep{}}
	seen := map[[2]string]bool{}
	add := func(from, to string, possible bool, location *programindex.Location) {
		if from == "" || to == "" || from == to || seen[[2]string{from, to}] {
			return
		}
		seen[[2]string{from, to}] = true
		graph.adj[from] = append(graph.adj[from], entryEdge{to: to, possible: possible, location: location})
	}
	for from, edges := range executionAdjacency(index) {
		for _, edge := range edges {
			add(from, edge.ToSubjectID, edge.Resolution != programindex.ResolutionExact, edge.Location)
		}
	}
	for _, connection := range index.Connections {
		if connection.SourceKind == "native_"+string(programindex.RelationCalls) && connection.From.TargetID == connection.To.TargetID {
			add(connection.FromSubjectID, connection.ToSubjectID, connection.SupportResolution != programindex.PatternValueExact, connection.FromLocation)
		}
	}
	for from := range graph.adj {
		edges := graph.adj[from]
		sort.SliceStable(edges, func(i, j int) bool {
			switch left, right := edges[i].location, edges[j].location; {
			case locationBeforeInFile(left, right):
				return true
			case locationBeforeInFile(right, left):
				return false
			}
			return edges[i].to < edges[j].to
		})
	}
	for _, seed := range index.Target.Seeds {
		if seed.ObjectID != "" {
			graph.seeds = append(graph.seeds, seed.ObjectID)
		}
	}
	sort.Strings(graph.seeds)
	return graph
}

// chain is the shortest chain of calls from the program's entry to a
// dispatch site's declaration that passes through none of the declarations
// the site chooses between: through one of them it would lead through the
// very handlers it dispatches to (Redis's main → loadAppendOnlyFile →
// execCommand → call). A declaration the entry does not reach is its own
// chain.
func (graph *entryGraph) chain(site dispatchRelation) []entryStep {
	if chain, cached := graph.chains[site.id]; cached {
		return chain
	}
	avoid := map[string]bool{}
	for _, target := range site.targets {
		avoid[target] = true
	}
	type visit struct {
		from     string
		possible bool
	}
	parent := map[string]visit{}
	seen := map[string]bool{}
	var queue []string
	for _, seed := range graph.seeds {
		if !avoid[seed] && !seen[seed] {
			seen[seed] = true
			queue = append(queue, seed)
		}
	}
	for len(queue) > 0 && !seen[site.from] {
		current := queue[0]
		queue = queue[1:]
		for _, edge := range graph.adj[current] {
			if avoid[edge.to] || seen[edge.to] {
				continue
			}
			seen[edge.to] = true
			parent[edge.to] = visit{current, edge.possible}
			queue = append(queue, edge.to)
		}
	}
	chain := []entryStep{{subject: site.from}}
	if seen[site.from] {
		chain = nil
		for at := site.from; ; {
			step := entryStep{subject: at}
			up, has := parent[at]
			if has {
				step.possible = up.possible
			}
			chain = append(chain, step)
			if !has {
				break
			}
			at = up.from
		}
		for left, right := 0, len(chain)-1; left < right; left, right = left+1, right-1 {
			chain[left], chain[right] = chain[right], chain[left]
		}
	}
	graph.chains[site.id] = chain
	return chain
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

// inputPath is an input's path for its reading. Shared: for each dispatch
// site whose alternatives hold the handler and handle at least one other
// input, the chain from the entry to the site. Own: the handler's witness
// tree, every reached part's shortest witness merged into one tree, a
// callee under its caller, the branches in the order their parts were
// reached; nothing in it claims an order of execution between branches.
func (builder *pageBuilder) inputPath(index *groupindex.Index, graph *entryGraph, operation groupindex.Operation, reached []string, firstInGroup map[string]string,
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
		shared := pageSharedPath{Inputs: inputs, All: inputs == len(index.Operations), Of: len(site.targets)}
		for _, step := range graph.chain(site) {
			item := builder.pathStep(targetID, step.subject, part)
			item.Possible = step.possible
			shared.Steps = append(shared.Steps, item)
		}
		shared.Through = shared.Steps[len(shared.Steps)-1].Name
		path.Shared = append(path.Shared, shared)
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
	for i := range path.Shared {
		steps(path.Shared[i].Steps)
	}
	steps(path.Own)
	encoded, err := json.Marshal(path)
	if err != nil {
		return raw
	}
	return string(encoded)
}
