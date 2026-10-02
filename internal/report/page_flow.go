package report

import (
	"cmp"
	"slices"
	"strings"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

// pageFlowCall is one call a declaration makes, as its flow reads it (owner,
// 2026-09-29, the designer's flow v2): its calls stand in the order they
// are written (the first call site's line, then column), then under each
// part once (groupFlowByPart), each callee once with every place it is
// called; a dispatch site that calls one of several declarations is one
// call. The reading column renders them in this order, a call opening in
// place to its callee's own flow; it sorts nothing.
//
//   - Decl is the callee in the reading's declarations; Name a call into
//     code the report names no declaration for (a library's fork or write),
//     a plain row.
//   - One are, for a dispatch site, the declarations it calls one of.
//   - Macro is a call as the code writes it when a macro's expansion makes
//     it (the C adapter's macro_expansion witness and the call's selector,
//     the outermost macro name at the use): one row for the macro, never
//     its expansion's internals (`assert`, not __assert_rtn and
//     __builtin_expect). Its Decl or One are the repository declarations
//     its expansion calls (redisAssert's _redisAssert, dictHashKey's hash
//     functions), Every saying One are all called rather than one of them;
//     with none it is a plain row, Lib the header of what it calls when the
//     macro is not the repository's. A compiler builtin (the adapter's
//     `builtin` package) is never a call of its own.
//   - Helper marks a call the column may fold under its step's "+ helpers"
//     (32-flow.js folds them only when more than three): the callee is a
//     declaration the helper question decided serves the work of others,
//     and it stands in a part most of the program's parts call into, never
//     the caller's own. A call into the caller's own part is its work and
//     stays even when that part is widely called (owner, 2026-09-29:
//     rdbLoad's rdbLoadType, expireGenericCommand's setExpire and deleteKey,
//     then processCommand's lookupCommand and queueMultiCommand, had waited
//     behind the fold); so does a helper into any other part, whose part
//     says what it is for.
type pageFlowCall struct {
	Decl *int   `json:"decl,omitempty"`
	Name string `json:"name,omitempty"`
	// Lib is the library a named call goes to ("sys/wait.h" for wait3).
	Lib      string            `json:"lib,omitempty"`
	One      []int             `json:"one,omitempty"`
	Kind     string            `json:"kind,omitempty"`
	Possible bool              `json:"possible,omitempty"`
	Helper   bool              `json:"helper,omitempty"`
	Sites    []pageReadingSite `json:"sites,omitempty"`
	Macro    string            `json:"macro,omitempty"`
	Every    bool              `json:"every,omitempty"`
	// Unresolved marks a call no implementation is established for, named
	// as written where it is written (calledName): etcd's
	// local_request_Election_Campaign_0 calling server.Campaign, an
	// interface's method, at gw/v3election.pb.gw.go:62.
	Unresolved bool `json:"unresolved,omitempty"`
	// Implements is, for a call through an interface whose value no
	// observed flow gives, the interface's method as the code calls it
	// (ProgramIndex Relation.Basis "implements", GO): its Decl or One are
	// the methods the repository's types implementing it declare, known by
	// method set, never a traced call (etcd's server.Campaign:
	// electionServer, electionProxy and UnimplementedElectionServer).
	Implements string `json:"implements,omitempty"`
	// Launch marks a call starting a program the code does not name
	// (unnamedLaunch): its function's reading says so, where no outside
	// system stands for it.
	Launch bool `json:"launch,omitempty"`
	// Arity is, for a definition's call of itself written in one arity that
	// its argument count sends to another (a Clojure `arity` witness), the
	// parameters of the arity called: "[board player opts]". Its reading
	// says it calls that form, not itself.
	Arity string `json:"arity,omitempty"`
}

// builtinPackage is the package the C adapter gives a compiler builtin
// (`__builtin_expect`, C: "`__builtin_*` calls belong to the platform").
const builtinPackage = "builtin"

// pageMacroCall is a call a macro's expansion makes: the macro as written
// at its use, and whether the macro is the repository's own (its body's
// spelling is in the repository).
type pageMacroCall struct {
	name string
	own  bool
}

// macroCalls are a program's calls made by a macro's expansion, by relation
// ID, built once from its ProgramIndex.
func (builder *pageBuilder) macroCalls(targetID string) map[string]pageMacroCall {
	if builder.macros == nil {
		builder.macros = map[string]map[string]pageMacroCall{}
	}
	if cached, done := builder.macros[targetID]; done {
		return cached
	}
	calls := map[string]pageMacroCall{}
	if builder.data != nil && builder.data.ProgramPortfolio != nil {
		for _, entry := range builder.data.ProgramPortfolio.Entries {
			if entry.Target.ID != targetID {
				continue
			}
			for _, relation := range entry.Relations {
				if len(relation.Patterns) == 0 || relation.Patterns[0].Selector == "" {
					continue
				}
				for _, witness := range relation.Witnesses {
					if witness.Kind == "macro_expansion" {
						calls[relation.ID] = pageMacroCall{name: relation.Patterns[0].Selector, own: witness.Location != nil}
						break
					}
				}
			}
		}
	}
	builder.macros[targetID] = calls
	return calls
}

// arityCalls are a program's calls of another arity of the definition
// making them, by relation ID: the parameters of the arity called (the
// relation's `arity` witness), built once from its ProgramIndex.
func (builder *pageBuilder) arityCalls(targetID string) map[string]string {
	if builder.arities == nil {
		builder.arities = map[string]map[string]string{}
	}
	if cached, done := builder.arities[targetID]; done {
		return cached
	}
	calls := map[string]string{}
	if builder.data != nil && builder.data.ProgramPortfolio != nil {
		for _, entry := range builder.data.ProgramPortfolio.Entries {
			if entry.Target.ID != targetID {
				continue
			}
			for _, relation := range entry.Relations {
				for _, witness := range relation.Witnesses {
					if witness.Kind == "arity" && witness.Detail != "" {
						calls[relation.ID] = witness.Detail
					}
				}
			}
		}
	}
	builder.arities[targetID] = calls
	return calls
}

// flowKinds are the relations a flow reads as calls.
var flowKinds = map[programindex.RelationKind]bool{
	programindex.RelationCalls: true, programindex.RelationExecutes: true,
	programindex.RelationPassesCallback: true, programindex.RelationInvokesExternal: true,
}

// pageFlowIndex is one program's calls by caller, in the order they are
// written, and the parts most of its parts call into.
type pageFlowIndex struct {
	byCaller map[string][]int
	shared   map[string]bool
	// launches are the sites of the calls starting a program the code
	// does not name (unnamedLaunch).
	launches map[programindex.Location]bool
}

// flowIndex builds a program's flow index once.
func (builder *pageBuilder) flowIndex(index *groupindex.Index) *pageFlowIndex {
	if builder.flows == nil {
		builder.flows = map[string]*pageFlowIndex{}
	}
	if cached := builder.flows[index.Target.ID]; cached != nil {
		return cached
	}
	flow := &pageFlowIndex{byCaller: map[string][]int{}, shared: map[string]bool{}, launches: map[programindex.Location]bool{}}
	for _, call := range index.Outbound {
		if unnamedLaunch(call) {
			flow.launches[call.Location] = true
		}
	}
	groupOf := builder.edgesBetweenGroups(*index).groupOf
	callers := map[string]map[string]bool{}
	for position, edge := range index.StructuralEdges {
		if edge.Role != groupindex.EdgeRelationTarget || !flowKinds[edge.RelationKind] || edge.Resolution == programindex.ResolutionUnresolved {
			continue
		}
		flow.byCaller[edge.FromSubjectID] = append(flow.byCaller[edge.FromSubjectID], position)
		from, to := groupOf[edge.FromSubjectID], groupOf[edge.ToSubjectID]
		if from != "" && to != "" && from != to {
			if callers[to] == nil {
				callers[to] = map[string]bool{}
			}
			callers[to][from] = true
		}
	}
	// "Most of the program" is decided from its parts: a part more than
	// half of the others call into (Redis's Server core state, called from
	// 18 of 21 parts), never by its name.
	for group, from := range callers {
		if 2*len(from) > len(index.Groups)-1 {
			flow.shared[group] = true
		}
	}
	for caller, edges := range flow.byCaller {
		slices.SortStableFunc(edges, func(a, b int) int {
			return compareSites(index.StructuralEdges[a].Location, index.StructuralEdges[b].Location)
		})
		flow.byCaller[caller] = edges
	}
	builder.flows[index.Target.ID] = flow
	return flow
}

// compareSites orders call sites as they are written: file, line, column;
// a call with no place after those with one.
func compareSites(a, b *programindex.Location) int {
	switch {
	case a == nil && b == nil:
		return 0
	case a == nil:
		return 1
	case b == nil:
		return -1
	}
	return cmp.Or(strings.Compare(a.Path, b.Path), cmp.Compare(a.Line, b.Line), cmp.Compare(a.Column, b.Column))
}

// flowOf is a declaration's calls in the order they are written. declare
// names a callee in the reading (its position, or -1 when the report names
// no declaration for it); partOf is the part a declaration stands in.
//
// With a case's lines (Operation.Branch), it is what those lines call, as
// an input a case declares is handled there (GroupsIndex branch.go): what
// litestream's case "replicate" does is its lines of Main.Run, never the
// other cases'.
func (builder *pageBuilder) flowOf(index *groupindex.Index, callerID string, declare func(string) int, within *programindex.LineRange) []pageFlowCall {
	flow := builder.flowIndex(index)
	groupOf := builder.edgesBetweenGroups(*index).groupOf
	macros := builder.macroCalls(index.Target.ID)
	arities := builder.arityCalls(index.Target.ID)
	var calls []pageFlowCall
	at := map[string]int{}
	byRelation := map[string]int{}
	// A macro's row: the repository declarations its expansion calls, and
	// whether a dispatch site makes them one of several.
	into := map[int][]int{}
	dispatched := map[int]bool{}
	first := map[int]string{}
	for _, position := range flow.byCaller[callerID] {
		edge := index.StructuralEdges[position]
		if within != nil && (edge.Location == nil || edge.Location.Line < within.Line || edge.Location.Line > within.EndLine) {
			continue
		}
		var site *pageReadingSite
		if edge.Location != nil {
			anchor := builder.links.anchor(edge.Location.Path, edge.Location.Line, edge.Location.Column)
			site = &pageReadingSite{At: anchor.Text}
		}
		kind := string(edge.RelationKind)
		if edge.RelationKind == programindex.RelationCalls {
			kind = ""
		}
		possible := edge.Resolution != programindex.ResolutionExact
		var external *programindex.ExternalSymbol
		if ref, known := builder.subject(index.Target.ID, edge.ToSubjectID); known && ref.subject.Object != nil {
			external = ref.subject.Object.External
		}
		// A call a macro's expansion makes is the macro as written, once.
		if macro, made := macros[edge.RelationID]; made {
			key := "\x01" + macro.name
			listed, seen := at[key]
			if !seen {
				listed = len(calls)
				at[key] = listed
				calls = append(calls, pageFlowCall{Macro: macro.name, Possible: possible})
			}
			row := &calls[listed]
			if site != nil && !slices.ContainsFunc(row.Sites, func(other pageReadingSite) bool { return other.At == site.At }) {
				row.Sites = append(row.Sites, *site)
			}
			if callee := declare(edge.ToSubjectID); callee >= 0 {
				if !slices.Contains(into[listed], callee) {
					into[listed] = append(into[listed], callee)
				}
				if first[listed] == "" {
					first[listed] = edge.ToSubjectID
				}
				dispatched[listed] = dispatched[listed] || edge.Resolution == programindex.ResolutionAlternatives
			} else if external != nil && !macro.own && row.Lib == "" && external.PackagePath != builtinPackage {
				row.Lib = external.PackagePath
			}
			continue
		}
		// A compiler builtin is never a call of its own.
		if external != nil && external.PackagePath == builtinPackage {
			continue
		}
		// A dispatch site is one call, one of the declarations it calls.
		if edge.Resolution == programindex.ResolutionAlternatives && edge.RelationID != "" {
			if listed, seen := byRelation[edge.RelationID]; seen {
				if callee := declare(edge.ToSubjectID); callee >= 0 && !slices.Contains(calls[listed].One, callee) {
					calls[listed].One = append(calls[listed].One, callee)
				}
				continue
			}
		}
		callee := declare(edge.ToSubjectID)
		arity := arities[edge.RelationID]
		// A call known by the interface's implementations is a row of its
		// own beside a traced call of the same callee, whichever comes first:
		// each site keeps how it is known.
		key := edge.ToSubjectID + "\x00" + kind + "\x00" + arity + "\x00" + edge.Basis
		launch := edge.Location != nil && flow.launches[programindex.Location{Path: edge.Location.Path, Line: edge.Location.Line, Column: max(1, edge.Location.Column)}]
		if listed, seen := at[key]; seen && calls[listed].One == nil {
			if site != nil && !slices.ContainsFunc(calls[listed].Sites, func(other pageReadingSite) bool { return other.At == site.At }) {
				calls[listed].Sites = append(calls[listed].Sites, *site)
			}
			calls[listed].Possible = calls[listed].Possible && possible
			calls[listed].Launch = calls[listed].Launch || launch
			continue
		}
		call := pageFlowCall{Kind: kind, Possible: possible, Launch: launch, Arity: arity}
		if edge.Basis == programindex.BasisImplements {
			call.Implements = builder.implementedName(index.Target.ID, callerID, edge.RelationID)
		}
		if site != nil {
			call.Sites = []pageReadingSite{*site}
		}
		if callee >= 0 {
			call.Decl = &callee
		} else if ref, known := builder.subject(index.Target.ID, edge.ToSubjectID); known {
			call.Name, _ = builder.subjectDisplay(ref.subject)
			if object := ref.subject.Object; object != nil && object.External != nil && object.External.Name != "" {
				call.Name, call.Lib = object.External.Name, object.External.PackagePath
				if object.External.Receiver != "" {
					call.Name = object.External.Receiver + "." + call.Name
				}
			}
		}
		if call.Decl == nil && call.Name == "" {
			continue
		}
		if ref, known := builder.subject(index.Target.ID, edge.ToSubjectID); known && ref.subject.Interpretation != nil && ref.subject.Interpretation.Helper {
			call.Helper = flow.shared[groupOf[edge.ToSubjectID]] && groupOf[edge.ToSubjectID] != groupOf[callerID]
		}
		if edge.Resolution == programindex.ResolutionAlternatives && edge.RelationID != "" {
			byRelation[edge.RelationID] = len(calls)
			if callee >= 0 {
				call.One = []int{callee}
			}
			call.Decl = nil
		}
		at[key] = len(calls)
		calls = append(calls, call)
	}
	// A call no implementation is established for, as saved with no
	// callee: named as written, linked where it is written (the flow's
	// edges hold none of it; the reading had dropped etcd's server.Campaign).
	for _, relation := range builder.callRelations(index.Target.ID)[callerID] {
		if len(relation.ToIDs) > 0 || relation.Resolution != programindex.ResolutionUnresolved {
			continue
		}
		if within != nil && (relation.Location == nil || relation.Location.Line < within.Line || relation.Location.Line > within.EndLine) {
			continue
		}
		name := calledName(relation)
		if name == "" {
			continue
		}
		call := pageFlowCall{Name: name, Possible: true, Unresolved: true}
		if relation.Location != nil {
			anchor := builder.links.anchor(relation.Location.Path, relation.Location.Line, relation.Location.Column)
			call.Sites = []pageReadingSite{{At: anchor.Text, Href: anchor.Href, Open: anchor.Open}}
		}
		key := "\x02" + name
		if listed, seen := at[key]; seen {
			calls[listed].Sites = append(calls[listed].Sites, call.Sites...)
			continue
		}
		at[key] = len(calls)
		calls = append(calls, call)
	}
	// A macro's row calls what its expansion calls: one declaration, the
	// declarations a dispatch site calls one of, or all of several.
	for listed, callees := range into {
		row := &calls[listed]
		row.One, row.Every, row.Possible = callees, !dispatched[listed], dispatched[listed]
		if ref, known := builder.subject(index.Target.ID, first[listed]); known && len(callees) == 1 && ref.subject.Interpretation != nil && ref.subject.Interpretation.Helper {
			row.Helper = flow.shared[groupOf[first[listed]]] && groupOf[first[listed]] != groupOf[callerID]
		}
	}
	// A dispatch site of one declaration is a call of it.
	for i := range calls {
		if len(calls[i].One) == 1 {
			calls[i].Decl, calls[i].One, calls[i].Every = &calls[i].One[0], nil, false
		}
	}
	return calls
}

// implementedName is the interface method a call on the implements basis
// calls, as the code calls it (calledName of its relation).
func (builder *pageBuilder) implementedName(targetID, callerID, relationID string) string {
	for _, relation := range builder.callRelations(targetID)[callerID] {
		if relation.ID == relationID {
			return calledName(relation)
		}
	}
	return ""
}

// groupFlowByPart stands a flow's calls under each part once (reviewer,
// 2026-09-30: redis main had shown "Server lifecycle and cron" four times,
// FreqtradeBot.process "Trading bot core" three): the parts in the order of
// their first call, each part's calls in the order they are written. A
// call's part is its callee's (partOf, by position in the reading), a
// dispatch site's its first declaration's; the calls into code the report
// names no declaration for keep their written order among themselves.
func groupFlowByPart(calls []pageFlowCall, partOf func(int) string) []pageFlowCall {
	if len(calls) < 3 {
		return calls
	}
	part := func(call pageFlowCall) string {
		switch {
		case call.Decl != nil:
			return partOf(*call.Decl)
		case len(call.One) > 0:
			return partOf(call.One[0])
		}
		return "\x00"
	}
	first := map[string]int{}
	for i, call := range calls {
		if _, seen := first[part(call)]; !seen {
			first[part(call)] = i
		}
	}
	grouped := slices.Clone(calls)
	slices.SortStableFunc(grouped, func(a, b pageFlowCall) int { return cmp.Compare(first[part(a)], first[part(b)]) })
	return grouped
}

// waysIn are the ways a request reaches one dispatch site, one per outer
// input in the order of their kinds (requests first), each by its shortest
// route: through a callable it hands over when it hands one over (the
// callable's shortest run to the site), else by its own calls. A site no
// outer input is established for is one way, the site alone.
func (builder *pageBuilder) waysIn(index *groupindex.Index, site groupindex.DispatchSite, decls *pathDecls, inputNode func(string) string, kindOf map[string]string) []pageWay {
	handler := map[string]string{}
	for _, operation := range index.Operations {
		handler[operation.ID] = operation.SubjectID
	}
	best := map[string]pageWay{}
	length := map[string]int{}
	var order []string
	for _, outer := range site.Outer {
		node, root := inputNode(outer.OperationID), handler[outer.OperationID]
		if root == "" {
			continue
		}
		var way pageWay
		score := 0
		if outer.Registered == "" {
			chain := chainOf(index, outer.Edges, root, site.FromSubjectID)
			if chain == nil {
				continue
			}
			way = pageWay{Input: node, Chain: decls.all(chain)}
			// A way by calls alone is taken only when no callable is
			// handed over.
			score = 1_000_000 + len(chain)
		} else {
			passing := index.StructuralEdges[outer.HandOver].FromSubjectID
			by := chainOf(index, outer.Registering, root, passing)
			calls := chainOf(index, outer.Edges, outer.Registered, site.FromSubjectID)
			if by == nil || calls == nil {
				continue
			}
			way = pageWay{Input: node, Chain: decls.all(append([]string{root}, calls...)), Hop: 1, By: decls.all(by)}
			score = len(calls)
		}
		if previous, seen := length[node]; seen && previous <= score {
			continue
		}
		if _, seen := length[node]; !seen {
			order = append(order, node)
		}
		best[node], length[node] = way, score
	}
	slices.SortStableFunc(order, func(a, b string) int { return outerKindRank(kindOf[a]) - outerKindRank(kindOf[b]) })
	var ways []pageWay
	for _, node := range order {
		ways = append(ways, best[node])
	}
	if len(ways) == 0 {
		ways = append(ways, pageWay{Chain: []int{decls.of(site.FromSubjectID)}})
	}
	return ways
}

// all names declarations by their positions in the reading.
func (decls *pathDecls) all(subjects []string) []int {
	positions := make([]int, len(subjects))
	for i, subject := range subjects {
		positions[i] = decls.of(subject)
	}
	return positions
}

// chainOf is the shortest run of the given calls from one declaration to
// another, both included, in call order; nil when they do not join them.
// Calls are taken in the order they are given.
func chainOf(index *groupindex.Index, edges []int, from, to string) []string {
	if from == to {
		return []string{from}
	}
	next := map[string][]string{}
	for _, position := range edges {
		edge := index.StructuralEdges[position]
		next[edge.FromSubjectID] = append(next[edge.FromSubjectID], edge.ToSubjectID)
	}
	previous := map[string]string{from: ""}
	queue := []string{from}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, callee := range next[current] {
			if _, seen := previous[callee]; seen {
				continue
			}
			previous[callee] = current
			if callee == to {
				chain := []string{to}
				for at := current; at != ""; at = previous[at] {
					chain = append([]string{at}, chain...)
				}
				return chain
			}
			queue = append(queue, callee)
		}
	}
	return nil
}

// markWaysFrom names, for each way but the first, the declaration where it
// leaves the first: the one before the first declaration both run
// (syncWithMaster, before createClient), or the way's first when they
// share none.
func markWaysFrom(ways []pageWay) {
	if len(ways) < 2 {
		return
	}
	full := func(way pageWay) []int {
		if way.Hop > 0 {
			return append(slices.Clone(way.By), way.Chain[way.Hop:]...)
		}
		return way.Chain
	}
	first := full(ways[0])
	for i := 1; i < len(ways); i++ {
		route := full(ways[i])
		from := route[0]
		for at, decl := range route {
			if slices.Contains(first, decl) {
				if at > 0 {
					from = route[at-1]
				}
				break
			}
			from = decl
		}
		ways[i].From = &from
	}
}
