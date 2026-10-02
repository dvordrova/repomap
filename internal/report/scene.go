package report

import (
	"cmp"
	"encoding/json"
	"html/template"
	"slices"
	"strconv"
	"strings"
)

// SceneFacts are the facts the scene canvas draws (REPORT.md "Scene
// model"), read once when the report is assembled off the same system map
// the page draws and saved in report.json. The page only shows them (owner,
// 2026-10-01: "у html должна быть простая задача — вот данные,
// показываю"). Every ID is the page's.
type SceneFacts struct {
	// Inputs is where each input takes effect.
	Inputs map[string]SceneInput `json:"inputs"`
	// Systems is who calls each outside system.
	Systems map[string]SceneSystem `json:"systems"`
	// Calls is, by part, the outside systems it calls and the declarations
	// making each call.
	Calls map[string][]SceneCall `json:"calls"`
	// ProgramPairs is one record per directed pair of programs a relation
	// joins.
	ProgramPairs []SceneProgramPair `json:"program_pairs"`
	// Reaching is, by part and outside system, the inputs whose saved path
	// reaches it, in the page's order: a relation listing the input among
	// its operations starts or ends at the part, or at the system or one of
	// its call records. No entry when no input reaches it.
	Reaching map[string][]string `json:"reaching"`
}

// SceneInput is where an input takes effect: its handler's part (Handled),
// else the parts its code takes it in ("declared in", "looked up in"),
// else, Parts empty, its Program.
type SceneInput struct {
	// Kind is one of sceneInputKinds.
	Kind    string   `json:"kind"`
	Program string   `json:"program"`
	Parts   []string `json:"parts"`
	Handled bool     `json:"handled"`
	// Handler is the declaration it is handled by, when Handled and known.
	Handler *SceneSource `json:"handler,omitempty"`
}

// SceneSystem is an outside system: Kind (one of sceneSystemKinds) the kind
// most of its calls' facts give it; Parts the parts whose calls reach it;
// Programs the programs calling it.
type SceneSystem struct {
	Kind     string   `json:"kind"`
	Parts    []string `json:"parts"`
	Programs []string `json:"programs"`
}

// SceneCall is one outside call of a part: the system and the declaration
// making the call, nil when no call to it names one.
type SceneCall struct {
	System string       `json:"system"`
	Caller *SceneSource `json:"caller,omitempty"`
}

// SceneProgramPair is a directed pair of programs: Runtime when a relation
// between them is an operation (a part reaching the other program's input
// counting for that program, and a call through an outside system the other
// program's input serves counting as its caller's), else code use only.
// Relations are the positions, in the system map's relations, of the
// relations it stands for.
type SceneProgramPair struct {
	From      string `json:"from"`
	To        string `json:"to"`
	Runtime   bool   `json:"runtime"`
	Relations []int  `json:"relations"`
}

// SceneSource is a declaration's place, whatever links a page gives it:
// its file and line, as a part's declarations on the page carry them
// (pageNodeSymbol Path and Line).
type SceneSource struct {
	Path string `json:"path"`
	Line int    `json:"line"`
}

// The kinds the canvas marks an input and an outside system with.
var (
	sceneInputKinds  = []string{"request", "command", "setting", "scheduled", "continuous", "interaction", "consumer", "extension", "entry"}
	sceneSystemKinds = []string{"database", "request", "sdk", "queue", "started", "other"}
)

func sceneInputKind(activation string) string {
	switch activation {
	case "background":
		return "continuous"
	case "queue_consumer":
		return "consumer"
	}
	if slices.Contains(sceneInputKinds, activation) {
		return activation
	}
	return "entry"
}

func sceneSystemKind(kind string) string {
	if slices.Contains(sceneSystemKinds, kind) {
		return kind
	}
	return "other"
}

// deriveScene reads the scene's facts off the page the report draws, its
// declarations keyed by their places (pageLinks keyed), so the facts hold
// whatever links a later render gives the page.
func deriveScene(data *ReportData) (*SceneFacts, error) {
	keyed := *data
	keyed.sceneKeys = true
	keyed.GitHubSourceLinks, keyed.GitLabSourceLinks, keyed.SourceIDs, keyed.UnavailableSourcePaths = nil, nil, nil, nil
	view, err := buildPageView(&keyed, "", renderPayloadLocalRoots(&keyed, nil))
	if err != nil {
		return nil, err
	}
	return sceneOf(view.SystemMap(), sceneSourceOf), nil
}

// sceneKey is a place as the keyed page writes it (pageLinks.anchor).
func sceneKey(path string, line, column int) string {
	return path + ":" + strconv.Itoa(line) + ":" + strconv.Itoa(column)
}

func sceneSourceOf(key string) *SceneSource {
	end := strings.LastIndexByte(key, ':')
	if end < 0 {
		return nil
	}
	middle := strings.LastIndexByte(key[:end], ':')
	if middle <= 0 {
		return nil
	}
	line, lineErr := strconv.Atoi(key[middle+1 : end])
	if _, columnErr := strconv.Atoi(key[end+1:]); lineErr != nil || columnErr != nil {
		return nil
	}
	return &SceneSource{Path: key[:middle], Line: line}
}

// sceneOf reads the facts off a system map, as the canvas reads the page:
// what each record is, the relations folded per pair of drawn things, a call
// tile standing for its system.
func sceneOf(view *pageMap, source func(key string) *SceneSource) *SceneFacts {
	scene := &SceneFacts{Inputs: map[string]SceneInput{}, Systems: map[string]SceneSystem{}, Calls: map[string][]SceneCall{}, ProgramPairs: []SceneProgramPair{}, Reaching: map[string][]string{}}
	if view == nil {
		return scene
	}
	records := view.Nodes
	byID := make(map[string]*pageMapNode, len(records))
	for i := range records {
		if _, seen := byID[records[i].ID]; !seen {
			byID[records[i].ID] = &records[i]
		}
	}
	componentOf := map[string]string{}
	for _, record := range records {
		if record.Branch == "component" && record.Owner != "" {
			componentOf[record.Owner] = record.ID
		}
	}
	program := func(record *pageMapNode) string {
		if record.Activation != "" || record.Branch == "inputs" {
			return componentOf[record.Owner]
		}
		return ""
	}
	// A frame lists its inputs only when it is an Inputs collection.
	children := make(map[string][]string, len(records))
	kind := make(map[string]string, len(records))
	for _, record := range records {
		for _, id := range strings.Fields(record.Children) {
			if child := byID[id]; record.Branch == "inputs" || child == nil || child.Activation == "" {
				children[record.ID] = append(children[record.ID], id)
			}
		}
		kind[record.ID] = sceneRecordKind(record)
	}
	parentOf := map[string]string{}
	for _, record := range records {
		for _, child := range children[record.ID] {
			if byID[child] != nil {
				parentOf[child] = record.ID
			}
		}
	}
	// A call tile stands for its system.
	systemOfCall := map[string]string{}
	for _, record := range records {
		if kind[record.ID] == "system" {
			for _, tile := range children[record.ID] {
				if kind[tile] == "call" {
					systemOfCall[tile] = record.ID
				}
			}
		}
	}
	shown := func(id string) string { return cmp.Or(systemOfCall[id], id) }

	// One edge per directed pair of drawn things and certainty.
	type sceneEdge struct {
		from, to  string
		relations []int
	}
	var edges []*sceneEdge
	folded := map[[3]string]*sceneEdge{}
	labels := make([]string, len(view.Edges))
	for i, relation := range view.Edges {
		labels[i] = relation.Label
		if relation.CallsJSON() != "" {
			labels[i] = relation.Relation()
		}
		from, to := shown(relation.From), shown(relation.To)
		if from == to || byID[from] == nil || byID[to] == nil || kind[from] == "call" || kind[to] == "call" {
			continue
		}
		key := [3]string{from, to, strconv.FormatBool(relation.Possible)}
		edge := folded[key]
		if edge == nil {
			edge = &sceneEdge{from: from, to: to}
			folded[key] = edge
			edges = append(edges, edge)
		}
		edge.relations = append(edge.relations, i)
	}

	// What stands in what, as the canvas contains it: programs, areas,
	// parts and the note by the page's containment; inputs in their
	// program's collection by kind; systems in their Outside frame.
	type sceneNode struct{ kind, parent, program string }
	nodes := map[string]*sceneNode{}
	for _, record := range records {
		switch kind[record.ID] {
		case "program", "area", "part", "note":
			nodes[record.ID] = &sceneNode{kind: kind[record.ID], parent: parentOf[record.ID]}
		}
	}
	var leaves func(id string) []string
	leaves = func(id string) []string {
		if kind[id] == "input" {
			return []string{id}
		}
		var out []string
		for _, child := range children[id] {
			out = append(out, leaves(child)...)
		}
		return out
	}
	for _, record := range records {
		if kind[record.ID] != "inputs" {
			continue
		}
		nodes[record.ID] = &sceneNode{kind: "inputs", program: program(&record)}
		for _, id := range leaves(record.ID) {
			if byID[id] == nil {
				continue
			}
			group := record.ID + "#" + sceneInputKind(byID[id].Activation)
			nodes[group] = &sceneNode{kind: "kind", parent: record.ID, program: program(&record)}
			nodes[id] = &sceneNode{kind: "input", parent: group, program: program(&record)}
		}
	}
	for _, record := range records {
		if kind[record.ID] == "input" && nodes[record.ID] == nil {
			nodes[record.ID] = &sceneNode{kind: "input", program: program(&record)}
		}
	}
	partProgram := func(id string) string {
		at := id
		for nodes[at] != nil && nodes[at].parent != "" {
			at = nodes[at].parent
		}
		if nodes[at] != nil && nodes[at].kind == "program" {
			return at
		}
		return ""
	}
	// Who calls each system: the parts whose arrows go into it, and the
	// programs.
	callers, callingPrograms := map[string][]string{}, map[string][]string{}
	for _, edge := range edges {
		if kind[edge.to] != "system" {
			continue
		}
		if kind[edge.from] == "part" && !slices.Contains(callers[edge.to], edge.from) {
			callers[edge.to] = append(callers[edge.to], edge.from)
		}
		calling := partProgram(edge.from)
		if calling == "" && kind[edge.from] == "program" {
			calling = edge.from
		}
		if calling != "" && !slices.Contains(callingPrograms[edge.to], calling) {
			callingPrograms[edge.to] = append(callingPrograms[edge.to], calling)
		}
	}
	for _, record := range records {
		if kind[record.ID] != "outside" {
			continue
		}
		nodes[record.ID] = &sceneNode{kind: "outside"}
		for _, id := range children[record.ID] {
			if kind[id] == "system" {
				nodes[id] = &sceneNode{kind: "system", parent: record.ID}
			}
		}
	}
	for _, record := range records {
		if kind[record.ID] == "system" && nodes[record.ID] == nil {
			nodes[record.ID] = &sceneNode{kind: "system"}
		}
	}
	for _, node := range nodes {
		if nodes[node.parent] == nil {
			node.parent = ""
		}
	}
	rootOf := func(id string) string {
		at := id
		for nodes[at] != nil && nodes[at].parent != "" {
			at = nodes[at].parent
		}
		return at
	}
	kindOf := func(id string) string {
		if node := nodes[id]; node != nil {
			return node.kind
		}
		return ""
	}

	// Where each input takes effect.
	inputOwner := map[string]string{}
	for _, record := range records {
		if record.InputOwner != "" && byID[record.InputOwner] != nil {
			inputOwner[record.ID] = record.InputOwner
		}
	}
	for i, relation := range view.Edges {
		if from, to := byID[relation.From], byID[relation.To]; from != nil && to != nil && from.Activation != "" && to.Activation == "" && labels[i] == "implemented in" {
			inputOwner[relation.From] = relation.To
		}
	}
	for _, record := range records {
		node := nodes[record.ID]
		if node == nil || node.kind != "input" {
			continue
		}
		if _, done := scene.Inputs[record.ID]; done {
			continue
		}
		input := SceneInput{Kind: sceneInputKind(record.Activation), Program: node.program, Parts: []string{}}
		takenIn, reached := []string{}, []string{}
		for _, edge := range edges {
			if edge.from != record.ID || kindOf(edge.to) != "part" {
				continue
			}
			if !slices.Contains(reached, edge.to) {
				reached = append(reached, edge.to)
			}
			if slices.ContainsFunc(edge.relations, func(i int) bool { return labels[i] == "declared in" || labels[i] == "looked up in" }) && !slices.Contains(takenIn, edge.to) {
				takenIn = append(takenIn, edge.to)
			}
		}
		switch owner := inputOwner[record.ID]; {
		case owner != "" && kindOf(owner) == "part":
			input.Parts, input.Handled = []string{owner}, true
		case len(takenIn) > 0:
			input.Parts = takenIn
		case len(reached) > 0:
			input.Parts, input.Handled = reached, true
		}
		if input.Handled && record.Handler != "" {
			input.Handler = source(declarationKey(&record.HandlerSource))
		}
		scene.Inputs[record.ID] = input
	}

	for _, record := range records {
		if kind[record.ID] != "system" {
			continue
		}
		if _, done := scene.Systems[record.ID]; done {
			continue
		}
		scene.Systems[record.ID] = SceneSystem{Kind: sceneSystemKind(record.DestinationKind),
			Parts: append([]string{}, callers[record.ID]...), Programs: append([]string{}, callingPrograms[record.ID]...)}
	}

	// The systems each part calls, by the declarations making the calls.
	for _, edge := range edges {
		if kind[edge.to] != "system" || kindOf(edge.from) != "part" {
			continue
		}
		var made []string
		for _, i := range edge.relations {
			for _, call := range edgeCalls(view.Edges[i]) {
				made = append(made, call.Caller)
			}
		}
		if len(made) == 0 {
			made = []string{""}
		}
		for _, key := range made {
			call := SceneCall{System: edge.to}
			if key != "" {
				call.Caller = source(key)
			}
			if !slices.ContainsFunc(scene.Calls[edge.from], func(listed SceneCall) bool { return sameSceneCall(listed, call) }) {
				scene.Calls[edge.from] = append(scene.Calls[edge.from], call)
			}
		}
	}

	// The pairs of programs: a program reaching another program's input
	// reaches that program; a call through an outside system another
	// program's input serves (connects_to) is the caller's runtime pair to
	// the served program.
	pairs := map[[2]string]int{}
	add := func(from, to string, edge *sceneEdge, runtime bool) {
		if from == "" || to == "" || from == to || kindOf(from) != "program" || kindOf(to) != "program" {
			return
		}
		at, known := pairs[[2]string{from, to}]
		if !known {
			at = len(scene.ProgramPairs)
			pairs[[2]string{from, to}] = at
			scene.ProgramPairs = append(scene.ProgramPairs, SceneProgramPair{From: from, To: to, Relations: []int{}})
		}
		pair := &scene.ProgramPairs[at]
		pair.Runtime = pair.Runtime || runtime
		for _, i := range edge.relations {
			if !slices.Contains(pair.Relations, i) {
				pair.Relations = append(pair.Relations, i)
			}
		}
	}
	for _, edge := range edges {
		from, to := nodes[edge.from], nodes[edge.to]
		sources, target := []string{rootOf(edge.from)}, rootOf(edge.to)
		if to != nil && to.kind == "input" && to.program != "" && rootOf(edge.from) != to.program {
			target = to.program
		}
		if from != nil && from.kind == "input" && from.program != "" && rootOf(edge.to) != from.program {
			sources = []string{from.program}
		}
		if from != nil && to != nil && from.kind == "system" && to.kind == "input" {
			target = cmp.Or(to.program, target)
			for _, calling := range callingPrograms[edge.from] {
				add(calling, target, edge, true)
			}
			continue
		}
		runtime := slices.ContainsFunc(edge.relations, func(i int) bool { return view.Edges[i].Scope != "structure" })
		for _, from := range sources {
			add(from, target, edge, runtime)
		}
	}

	// The inputs reaching each part and outside system along their saved
	// paths: the relations listing an input among their operations.
	standsFor := map[string][]string{}
	for _, record := range records {
		switch kind[record.ID] {
		case "part":
			standsFor[record.ID] = append(standsFor[record.ID], record.ID)
		case "system":
			standsFor[record.ID] = append(standsFor[record.ID], record.ID)
			for _, id := range strings.Fields(record.Children) {
				if byID[id] != nil && !slices.Contains(standsFor[id], record.ID) {
					standsFor[id] = append(standsFor[id], record.ID)
				}
			}
		}
	}
	paths := map[string][]int{}
	for i, relation := range view.Edges {
		for _, input := range strings.Fields(relation.Operations) {
			paths[input] = append(paths[input], i)
		}
	}
	for _, record := range records {
		if record.Activation == "" {
			continue
		}
		for _, i := range paths[record.ID] {
			for _, end := range []string{view.Edges[i].From, view.Edges[i].To} {
				for _, reached := range standsFor[end] {
					if inputs := scene.Reaching[reached]; len(inputs) == 0 || inputs[len(inputs)-1] != record.ID {
						scene.Reaching[reached] = append(inputs, record.ID)
					}
				}
			}
		}
	}
	return scene
}

func sameSceneCall(left, right SceneCall) bool {
	if left.System != right.System || (left.Caller == nil) != (right.Caller == nil) {
		return false
	}
	return left.Caller == nil || *left.Caller == *right.Caller
}

// sceneRecordKind is what a system map record is on the canvas
// (web/model.mjs kindOf, 29-operation-view.js category).
func sceneRecordKind(record pageMapNode) string {
	switch record.Branch {
	case "component":
		return "program"
	case "area", "inputs", "outside":
		return record.Branch
	case "communication":
		return "system"
	}
	switch {
	case record.Activation != "":
		return "input"
	case record.ItemKind == "External communication":
		return "call"
	case record.ItemKind == "Component":
		return "note"
	}
	return "part"
}

// scenePage is the saved scene as the page reads it (REPORT.md "Scene
// model").
type scenePage struct {
	Inputs       map[string]SceneInput  `json:"inputs"`
	Systems      map[string]SceneSystem `json:"systems"`
	Calls        map[string][]SceneCall `json:"calls"`
	ProgramPairs []SceneProgramPair     `json:"programPairs"`
	Reaching     map[string][]string    `json:"reaching"`
}

// sceneJSON writes the saved scene for the page as it was saved; empty when
// the report saved none.
func sceneJSON(scene *SceneFacts) (template.JS, error) {
	if scene == nil {
		return "", nil
	}
	raw, err := json.Marshal(scenePage(*scene))
	if err != nil {
		return "", err
	}
	return template.JS(raw), nil
}
