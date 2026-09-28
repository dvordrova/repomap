package report

import (
	"encoding/json"
	"slices"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

// pageDecl is one declaration an input's reading names: a link into its
// code (Source is its place, for a tooltip only) and the drawn part it
// stands in, where a plain click reads it.
type pageDecl struct {
	Name     string `json:"name"`
	Href     string `json:"href,omitempty"`
	Open     string `json:"open,omitempty"`
	Source   string `json:"source,omitempty"`
	NoSource bool   `json:"no_source,omitempty"`
	Part     string `json:"part,omitempty"`
}

// pageCall is one relation the reading lists: the caller's and the callee's
// positions in Decls, and flags (callPossible, callRead, callIntegration).
type pageCall [3]int

const (
	callPossible    = 1
	callRead        = 2
	callIntegration = 4
)

// pageDispatched is a dispatch site whose alternatives hold the input's
// handler: the input is dispatched there, one of Of. Handlers counts the
// alternatives that are an input's handler and Shared the handlers that
// several of the Inputs dispatched there share, so each count says what it
// counts: Redis's call is one of 94 handlers, and 95 inputs are dispatched
// there because sinterCommand handles sinter and smembers. ReachedFrom are
// the inputs whose reach holds the site's declaration; the input's reading
// does not list them (a reader took them for its own route), the site's
// does.
type pageDispatched struct {
	Site        int                 `json:"site"`
	Of          int                 `json:"of"`
	Handlers    int                 `json:"handlers"`
	Inputs      int                 `json:"inputs"`
	Shared      []pageSharedHandler `json:"shared,omitempty"`
	All         bool                `json:"all,omitempty"`
	ReachedFrom []string            `json:"reached_from,omitempty"`
}

// pageSharedHandler is a handler of several inputs dispatched at one site,
// with those inputs' nodes in operation order.
type pageSharedHandler struct {
	Handler int      `json:"handler"`
	Inputs  []string `json:"inputs"`
}

// siteHandlers counts a dispatch site's alternatives that are an input's
// handler and lists each handler several inputs dispatched there share.
func siteHandlers(index *groupindex.Index, site groupindex.DispatchSite, decls *pathDecls, inputNode func(string) string) (int, []pageSharedHandler) {
	handler := make(map[string]bool, len(index.Operations))
	for _, operation := range index.Operations {
		handler[operation.SubjectID] = true
	}
	count := 0
	for _, alternative := range site.Alternatives {
		if handler[alternative] {
			count++
		}
	}
	dispatched := make(map[string]bool, len(site.OperationIDs))
	for _, id := range site.OperationIDs {
		dispatched[id] = true
	}
	var order []string
	inputs := map[string][]string{}
	for _, operation := range index.Operations {
		if !dispatched[operation.ID] {
			continue
		}
		if _, seen := inputs[operation.SubjectID]; !seen {
			order = append(order, operation.SubjectID)
		}
		inputs[operation.SubjectID] = append(inputs[operation.SubjectID], inputNode(operation.ID))
	}
	var shared []pageSharedHandler
	for _, subject := range order {
		if len(inputs[subject]) > 1 {
			shared = append(shared, pageSharedHandler{Handler: decls.of(subject), Inputs: inputs[subject]})
		}
	}
	return count, shared
}

// pageReaches is a dispatch site the input's own code reaches, with every
// call of its reach on a route to it.
type pageReaches struct {
	Site   int        `json:"site"`
	Inputs int        `json:"inputs"`
	Calls  []pageCall `json:"calls"`
}

// pageInputPart is a part (or an outside call's tile, or a matched peer)
// the input's reach enters: its depth, every call into it from a part
// reached earlier, and how many other calls enter it on this path.
type pageInputPart struct {
	Part string `json:"part"`
	// Title names the part where the map does not draw it.
	Title   string     `json:"title,omitempty"`
	Depth   int        `json:"depth"`
	Entered []pageCall `json:"entered,omitempty"`
	Others  int        `json:"others,omitempty"`
}

// pageInputPath is an input's reading of its reach (GroupsIndex's Reach and
// dispatch sites): where it is dispatched from, the sites its code reaches,
// the inputs it registers or is registered by, and the parts it enters in
// depth order. The page projects saved data; it walks no code.
type pageInputPath struct {
	Dispatched   []pageDispatched `json:"dispatched,omitempty"`
	Reaches      []pageReaches    `json:"reaches,omitempty"`
	Registers    []string         `json:"registers,omitempty"`
	RegisteredBy []string         `json:"registered_by,omitempty"`
	Parts        []pageInputPart  `json:"parts,omitempty"`
	Decls        []pageDecl       `json:"decls,omitempty"`
}

// pathDecls collects the declarations a reading names, each once.
type pathDecls struct {
	builder  *pageBuilder
	targetID string
	part     func(string) string
	list     []pageDecl
	position map[string]int
}

func (builder *pageBuilder) pathDecls(targetID string, part func(string) string) *pathDecls {
	return &pathDecls{builder: builder, targetID: targetID, part: part, position: map[string]int{}}
}

func (decls *pathDecls) of(subject string) int {
	if position, known := decls.position[subject]; known {
		return position
	}
	decl := pageDecl{Name: subject}
	if ref, known := decls.builder.subject(decls.targetID, subject); known {
		name, anchor := decls.builder.subjectDisplay(ref.subject)
		if name != "" {
			decl.Name = name
		}
		if anchor != nil {
			decl.Href, decl.Open, decl.Source, decl.NoSource = anchor.Href, anchor.Open, anchor.Text, anchor.NoSource
		}
	}
	decl.Part = decls.part(subject)
	return decls.add(subject, decl)
}

// add names a declaration that is no subject of this program (an outside
// call, a matched peer) under its own key.
func (decls *pathDecls) add(key string, decl pageDecl) int {
	if position, known := decls.position[key]; known {
		return position
	}
	decls.position[key] = len(decls.list)
	decls.list = append(decls.list, decl)
	return len(decls.list) - 1
}

// call is one followed relation of the reach as the reading lists it.
func (decls *pathDecls) call(edge groupindex.StructuralEdge) pageCall {
	flags := 0
	if edge.Resolution != programindex.ResolutionExact {
		flags |= callPossible
	}
	if edge.RelationKind == programindex.RelationReads {
		flags |= callRead
	}
	return pageCall{decls.of(edge.FromSubjectID), decls.of(edge.ToSubjectID), flags}
}

// appendCall lists a call once: two sites of one caller calling one callee
// read as the same line, which the reading writes without line numbers.
func appendCall(calls []pageCall, call pageCall) []pageCall {
	if slices.Contains(calls, call) {
		return calls
	}
	return append(calls, call)
}

// inputPath projects an input's saved reach for its reading. nodeOf names
// a group's node on the map, inputNode an operation's; extra are the tiles
// and peers the reach enters beyond the parts, already built.
func (builder *pageBuilder) inputPath(index *groupindex.Index, operation groupindex.Operation, reach groupindex.Reach,
	decls *pathDecls, nodeOf func(string) string, inputNode func(string) string, extra []pageInputPart) string {
	var path pageInputPath
	seen := map[string]bool{}
	for _, site := range index.Dispatch {
		if seen[site.FromSubjectID] || !slices.Contains(site.OperationIDs, operation.ID) {
			continue
		}
		seen[site.FromSubjectID] = true
		dispatched := pageDispatched{Site: decls.of(site.FromSubjectID), Of: len(site.Alternatives), Inputs: len(site.OperationIDs), All: len(site.OperationIDs) == len(index.Operations)}
		dispatched.Handlers, dispatched.Shared = siteHandlers(index, site, decls, inputNode)
		for _, reached := range site.ReachedFrom {
			dispatched.ReachedFrom = append(dispatched.ReachedFrom, inputNode(reached.OperationID))
		}
		path.Dispatched = append(path.Dispatched, dispatched)
	}
	for _, site := range index.Dispatch {
		for _, reached := range site.ReachedFrom {
			if reached.OperationID != operation.ID {
				continue
			}
			entry := pageReaches{Site: decls.of(site.FromSubjectID), Inputs: len(site.OperationIDs), Calls: []pageCall{}}
			for _, edge := range reached.Edges {
				entry.Calls = appendCall(entry.Calls, decls.call(index.StructuralEdges[edge]))
			}
			path.Reaches = append(path.Reaches, entry)
		}
	}
	for _, edge := range reach.HandsOver {
		for _, other := range index.Operations {
			if other.SubjectID == index.StructuralEdges[edge].ToSubjectID && other.ID != operation.ID && !slices.Contains(path.Registers, inputNode(other.ID)) {
				path.Registers = append(path.Registers, inputNode(other.ID))
			}
		}
	}
	for _, id := range reach.HandedOverBy {
		path.RegisteredBy = append(path.RegisteredBy, inputNode(id))
	}
	titles := make(map[string]string, len(index.Groups))
	for _, group := range index.Groups {
		titles[group.ID] = group.Title
	}
	for _, group := range reach.Groups {
		part := pageInputPart{Part: nodeOf(group.GroupID), Title: titles[group.GroupID], Depth: group.Depth, Others: group.Others}
		for _, witness := range group.Entered {
			part.Entered = appendCall(part.Entered, decls.call(index.StructuralEdges[witness.Edge]))
		}
		path.Parts = append(path.Parts, part)
	}
	path.Parts = append(path.Parts, extra...)
	path.Decls = decls.list
	entered := false
	for _, part := range path.Parts {
		entered = entered || len(part.Entered) > 0
	}
	if len(path.Dispatched) == 0 && len(path.Reaches) == 0 && len(path.Registers) == 0 && len(path.RegisteredBy) == 0 && !entered {
		return ""
	}
	raw, err := json.Marshal(path)
	if err != nil {
		return ""
	}
	return string(raw)
}

// remapInputPath renames the map nodes an input's path names, as the map's
// node IDs are renamed when its maps are scoped or joined. Two tiles folded
// into one keep the first entry.
func remapInputPath(raw string, rename func(string) string) string {
	if raw == "" {
		return raw
	}
	var path pageInputPath
	if json.Unmarshal([]byte(raw), &path) != nil {
		return raw
	}
	path.renameNodes(rename)
	encoded, err := json.Marshal(path)
	if err != nil {
		return raw
	}
	return string(encoded)
}

func (path *pageInputPath) renameNodes(rename func(string) string) {
	ids := func(list []string) {
		for i := range list {
			list[i] = rename(list[i])
		}
	}
	for i := range path.Decls {
		if path.Decls[i].Part != "" {
			path.Decls[i].Part = rename(path.Decls[i].Part)
		}
	}
	for i := range path.Dispatched {
		ids(path.Dispatched[i].ReachedFrom)
		for j := range path.Dispatched[i].Shared {
			ids(path.Dispatched[i].Shared[j].Inputs)
		}
	}
	ids(path.Registers)
	ids(path.RegisteredBy)
	parts := path.Parts[:0]
	seen := map[string]bool{}
	for _, part := range path.Parts {
		part.Part = rename(part.Part)
		if seen[part.Part] {
			continue
		}
		seen[part.Part] = true
		parts = append(parts, part)
	}
	path.Parts = parts
}

// pageSiteReadings are the dispatch sites declared in a part, read with
// their declaration: the inputs dispatched there and the inputs whose own
// code reaches the site, each with its calls to it.
type pageSiteReadings struct {
	Sites []pageSiteReading `json:"sites"`
	Decls []pageDecl        `json:"decls"`
}

type pageSiteReading struct {
	Site        int                 `json:"site"`
	Of          int                 `json:"of"`
	Handlers    int                 `json:"handlers"`
	Inputs      int                 `json:"inputs"`
	Shared      []pageSharedHandler `json:"shared,omitempty"`
	ReachedFrom []pageSiteInput     `json:"reached_from,omitempty"`
}

type pageSiteInput struct {
	Input string     `json:"input"`
	Calls []pageCall `json:"calls"`
}

// siteReadings are the dispatch sites of a group's declarations that
// dispatch an input, for their declarations' readings.
func (builder *pageBuilder) siteReadings(index *groupindex.Index, group groupindex.Group, decls *pathDecls, inputNode func(string) string) string {
	var readings pageSiteReadings
	seen := map[string]bool{}
	for _, site := range index.Dispatch {
		if len(site.OperationIDs) == 0 || seen[site.FromSubjectID] || !slices.Contains(group.MemberSubjectIDs, site.FromSubjectID) {
			continue
		}
		seen[site.FromSubjectID] = true
		reading := pageSiteReading{Site: decls.of(site.FromSubjectID), Of: len(site.Alternatives), Inputs: len(site.OperationIDs)}
		reading.Handlers, reading.Shared = siteHandlers(index, site, decls, inputNode)
		for _, reached := range site.ReachedFrom {
			input := pageSiteInput{Input: inputNode(reached.OperationID), Calls: []pageCall{}}
			for _, edge := range reached.Edges {
				input.Calls = appendCall(input.Calls, decls.call(index.StructuralEdges[edge]))
			}
			reading.ReachedFrom = append(reading.ReachedFrom, input)
		}
		readings.Sites = append(readings.Sites, reading)
	}
	if len(readings.Sites) == 0 {
		return ""
	}
	readings.Decls = decls.list
	raw, err := json.Marshal(readings)
	if err != nil {
		return ""
	}
	return string(raw)
}

// remapSiteReadings renames the input and part nodes a part's site readings
// name.
func remapSiteReadings(raw string, rename func(string) string) string {
	if raw == "" {
		return raw
	}
	var readings pageSiteReadings
	if json.Unmarshal([]byte(raw), &readings) != nil {
		return raw
	}
	for i := range readings.Decls {
		if readings.Decls[i].Part != "" {
			readings.Decls[i].Part = rename(readings.Decls[i].Part)
		}
	}
	for i := range readings.Sites {
		for j := range readings.Sites[i].ReachedFrom {
			readings.Sites[i].ReachedFrom[j].Input = rename(readings.Sites[i].ReachedFrom[j].Input)
		}
		for j := range readings.Sites[i].Shared {
			for k := range readings.Sites[i].Shared[j].Inputs {
				readings.Sites[i].Shared[j].Inputs[k] = rename(readings.Sites[i].Shared[j].Inputs[k])
			}
		}
	}
	encoded, err := json.Marshal(readings)
	if err != nil {
		return raw
	}
	return string(encoded)
}
