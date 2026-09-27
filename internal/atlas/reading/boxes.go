package reading

import (
	"context"
	"fmt"
	"path"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/destinations"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/atlas/table"
	"github.com/dvordrova/repomap/internal/programindex"
)

// boxState is one drawn part of a target and its exact declaration members.
type boxState struct {
	targetID string
	symbols  map[string]bool // explicit declaration membership
	id       string
	dir      string
	title    string
	// line is the part's description; empty is the no-description state.
	line  string
	files []string // file place IDs holding its declarations
	// rows are the files placed in it whole: its file endpoints. sources
	// are those and the split files whose units it holds: its directory,
	// its test fact, its description and its area dirs come from them.
	rows, sources []string
	// unitIDs are the units it holds; units counts them.
	unitIDs []string
	units   int
	open    bool
	// core is the model's: the program exists for this part. forTests is a
	// fact: every file placed in the part is test code. unreached is a fact
	// too: its program never runs any of it (runsNothing).
	core, forTests, unreached bool
	test                      bool
	role                      string
}

// offCanvas is a part kept in the atlas and not drawn: one made only of test
// code, or one its program never runs. It is not described, not grouped
// into an area, not asked for a core role or keys, and draws no arrow.
func (box *boxState) offCanvas() bool {
	return box.forTests || box.unreached
}

// overviewKeys chooses the declarations the overview describes, before any
// part is drawn: each file of a target shows up to three of the keys the
// selection found in it, the documented ones first. A declaration shared by targets is chosen once. A
// file without a selected key shows ranked keys, which the selection did not
// choose and which are therefore not described.
func (r *reader) overviewKeys() map[string]bool {
	selected := make(map[string]bool)
	for _, target := range r.opts.Targets {
		for _, place := range r.opts.Graph.Places {
			if place.Kind != atlas.PlaceFile || !contains(place.TargetIDs, target.ID) {
				continue
			}
			file := atlas.File{Path: place.Path}
			for _, decl := range place.File.Decls {
				symbol := atlas.Symbol{ID: r.symbolID(place.Path, decl.LineNo, decl.Name), Name: decl.Name, Doc: decl.Doc, LineNo: decl.LineNo}
				if line, ok := r.symbolLine[symbol.ID]; ok {
					symbol.Line = line.value
				}
				symbol.Key = contains(r.keys[place.ID], symbol.ID)
				file.Symbols = append(file.Symbols, symbol)
			}
			for _, key := range modelKeys([]atlas.File{file}) {
				if contains(r.keys[r.places[key.SymbolID].Parent], key.SymbolID) {
					selected[key.SymbolID] = true
				}
			}
		}
	}
	return selected
}

// boxesOfTarget lists the boxes holding files of one target, by ID.
func (r *reader) boxesOfTarget(targetID string) []*boxState {
	var result []*boxState
	for _, owner := range r.boxes {
		if owner.targetID != "" && owner.targetID != targetID {
			continue
		}
		for _, fileID := range owner.files {
			if contains(r.places[fileID].TargetIDs, targetID) {
				result = append(result, owner)
				break
			}
		}
	}
	sort.Slice(result, func(i, j int) bool { return compactIDLess(result[i].id, result[j].id) })
	return result
}

func (r *reader) targetFiles(owner *boxState, targetID string) int {
	if owner == nil || owner.targetID != "" && owner.targetID != targetID {
		return 0
	}
	count := 0
	for _, fileID := range owner.files {
		if contains(r.places[fileID].TargetIDs, targetID) {
			count++
		}
	}
	return count
}

func (r *reader) summary(owner *boxState, targetID string) lines.BoxSummary {
	return lines.BoxSummary{ID: owner.id, Title: owner.title, Line: owner.line, Files: r.targetFiles(owner, targetID)}
}

// arrowState is one box-to-box arrow of one target.
type arrowState struct {
	id        string
	from, to  string
	calls     int
	witnesses map[string]int
	// order is, per witness, the first call site that makes it and the
	// declaration of the function it reaches: the source order that breaks
	// a tie of counts.
	order    map[string]witnessOrder
	sentence string
	drawn    bool
}

// witnessOrder is where a witness first appears in the source: its call,
// the store that put the function it reaches into the field the call reads
// (a command table's row), and that function's declaration.
type witnessOrder struct{ call, store, callee sourceSite }

// sourceSite is a position in source order: by file, then line, then column.
// An unknown site comes after every known one.
type sourceSite struct {
	path         string
	line, column int
}

func (a sourceSite) compare(b sourceSite) int {
	switch {
	case (a.path == "") != (b.path == ""):
		if a.path == "" {
			return 1
		}
		return -1
	case a.path != b.path:
		return strings.Compare(a.path, b.path)
	case a.line != b.line:
		return a.line - b.line
	default:
		return a.column - b.column
	}
}

// observe counts one witness of the arrow where it first appears.
func (arrow *arrowState) observe(key string, at witnessOrder) {
	arrow.witnesses[key]++
	if arrow.order == nil {
		arrow.order = make(map[string]witnessOrder)
	}
	order, seen := arrow.order[key]
	if !seen || at.call.compare(order.call) < 0 {
		order.call = at.call
	}
	if !seen || at.store.compare(order.store) < 0 {
		order.store = at.store
	}
	if !seen || at.callee.compare(order.callee) < 0 {
		order.callee = at.callee
	}
	arrow.order[key] = order
}

func (r *reader) foldArrows() {
	r.arrows = make(map[string][]*arrowState)
	for _, target := range r.opts.Targets {
		byPair := make(map[[2]string]*arrowState)
		nativePairs := map[[2]string]bool{}
		if r.designBoxOf != nil {
			for _, place := range r.opts.Graph.Places {
				if place.Symbol == nil || !contains(place.TargetIDs, target.ID) {
					continue
				}
				for _, call := range place.Symbol.Calls {
					for _, callee := range call.CalleeIDs {
						nativePairs[[2]string{r.boxFor(target.ID, place.ID), r.boxFor(target.ID, callee)}] = true
					}
				}
			}
		}
		for _, edge := range r.opts.Graph.Edges {
			from, to := r.boxFor(target.ID, edge.From), r.boxFor(target.ID, edge.To)
			if edge.Kind == "calls" && nativePairs[[2]string{from, to}] {
				continue // exact observations below own these counts
			}
			if from == "" || to == "" || from == to {
				continue
			}
			// A directory edge may name a box that holds none of this
			// target's files; the arrow belongs to another target's map.
			if r.targetFiles(r.boxes[from], target.ID) == 0 || r.targetFiles(r.boxes[to], target.ID) == 0 {
				continue
			}
			key := [2]string{from, to}
			arrow, ok := byPair[key]
			if !ok {
				arrow = &arrowState{from: from, to: to, witnesses: make(map[string]int)}
				byPair[key] = arrow
			}
			arrow.calls += edge.Count
			for _, witness := range edge.Witnesses {
				arrow.observe(witness.Caller+"\x00"+witness.Callee+"\x00"+witness.Kind, witnessOrder{call: sourceSite{path: witness.Path, line: witness.LineNo}})
			}
		}
		// Declaration observations retain collaborations within one file and
		// across files split between several parts. File edges cannot locate
		// these endpoints and must not guess an owner.
		if r.designBoxOf != nil {
			for _, place := range r.opts.Graph.Places {
				if place.Symbol == nil || !contains(place.TargetIDs, target.ID) {
					continue
				}
				from := r.boxFor(target.ID, place.ID)
				for _, call := range place.Symbol.Calls {
					for _, callee := range call.CalleeIDs {
						to := r.boxFor(target.ID, callee)
						if from == "" || to == "" || from == to {
							continue
						}
						key := [2]string{from, to}
						arrow := byPair[key]
						if arrow == nil {
							arrow = &arrowState{from: from, to: to, witnesses: map[string]int{}}
							byPair[key] = arrow
						}
						arrow.calls++
						at := witnessOrder{call: sourceSite{path: place.Path, line: call.Line, column: call.Column}}
						for _, store := range call.Stores {
							if store.CalleeID == callee {
								at.store = sourceSite{path: store.Path, line: store.LineNo, column: store.Column}
							}
						}
						if target, ok := r.places[callee]; ok {
							at.callee = sourceSite{path: target.Path, line: target.LineNo, column: target.Column}
						}
						arrow.observe(place.Symbol.Decl.Name+"\x00"+r.calleeName(call, callee)+"\x00"+call.Kind, at)
					}
				}
			}
		}
		var arrows []*arrowState
		for _, arrow := range byPair {
			arrows = append(arrows, arrow)
		}
		sort.Slice(arrows, func(i, j int) bool {
			if arrows[i].from != arrows[j].from {
				return arrows[i].from < arrows[j].from
			}
			return arrows[i].to < arrows[j].to
		})
		for _, arrow := range arrows {
			arrow.drawn = true
		}
		r.arrows[target.ID] = arrows
	}
}

// calleeName is what an arrow's witness names at the far end of a call. A
// call through a function value writes a field or a variable (proc); the
// function the fact found stored there is what the arrow reaches, so the
// witness names it (getCommand), one per stored function.
func (r *reader) calleeName(call atlas.SymbolCall, callee string) string {
	if call.Dispatch != programindex.DispatchFunctionValue {
		return call.Name
	}
	if place, ok := r.places[callee]; ok && place.Symbol != nil && place.Symbol.Decl.Name != "" {
		return place.Symbol.Decl.Name
	}
	return call.Name
}

func (r *reader) boxOfPlace(placeID string) string {
	if boxID, ok := r.boxOf[placeID]; ok {
		return boxID
	}
	if place, ok := r.places[placeID]; ok && place.Kind == atlas.PlaceDirectory {
		if _, ok := r.boxes[place.Path]; ok {
			return place.Path
		}
	}
	return ""
}

func (arrow *arrowState) topWitnesses() []atlas.Witness {
	ranked := arrow.rankedWitnesses()
	return ranked[:min(len(ranked), 3)]
}

// rankedWitnesses are every witness of the arrow, most observed first. A tie
// goes in source order: the call written first; among the functions one call
// reaches through a field, the one stored there first (the command table's
// first row); then the one declared first. The alphabet would favour names
// that begin early: the dispatcher's arrow to Redis's string commands named
// append, decr and decrby and hid get and set.
func (arrow *arrowState) rankedWitnesses() []atlas.Witness {
	type pair struct {
		key   string
		count int
		order witnessOrder
	}
	pairs := make([]pair, 0, len(arrow.witnesses))
	for key, count := range arrow.witnesses {
		pairs = append(pairs, pair{key, count, arrow.order[key]})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].count != pairs[j].count {
			return pairs[i].count > pairs[j].count
		}
		a, b := pairs[i].order, pairs[j].order
		for _, c := range []int{a.call.compare(b.call), a.store.compare(b.store), a.callee.compare(b.callee)} {
			if c != 0 {
				return c < 0
			}
		}
		return pairs[i].key < pairs[j].key
	})
	result := make([]atlas.Witness, 0, len(pairs))
	for _, p := range pairs {
		parts := strings.SplitN(p.key, "\x00", 3)
		witness := atlas.Witness{Caller: parts[0], Callee: parts[1]}
		if len(parts) == 3 {
			witness.Kind = parts[2]
		}
		result = append(result, witness)
	}
	return result
}

// readArrows asks one sentence per drawn arrow, each box pair once across
// targets.
func (r *reader) readArrows(ctx context.Context) error {
	r.foldArrows()
	def := lines.Arrows()
	seen := make(map[string]*arrowState)
	ids := make(map[string]string)
	var rows []table.Row
	var order []*arrowState
	for _, target := range r.opts.Targets {
		for _, arrow := range r.arrows[target.ID] {
			if !arrow.drawn {
				continue
			}
			key := arrow.from + "\x00" + arrow.to
			if ids[key] == "" {
				ids[key] = fmt.Sprintf("x%d", len(ids)+1)
			}
			arrow.id = ids[key]
			if !r.boxes[arrow.from].open || !r.boxes[arrow.to].open {
				continue
			}
			// An arrow made only of import edges has no witness call to
			// describe; asked anyway, the model invents one (Morfeu arrows
			// r7 and r10 became "store or fetch cached data"). Such an
			// arrow keeps its fallback sentence and costs no row.
			if len(arrow.witnesses) == 0 {
				continue
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = arrow
			from, to := r.summary(r.boxes[arrow.from], target.ID), r.summary(r.boxes[arrow.to], target.ID)
			rows = append(rows, lines.ArrowRow(arrow.id, from, to, arrow.topWitnesses(), arrow.calls))
			order = append(order, arrow)
		}
	}
	r.opts.Stage(def.Stage, fmt.Sprintf("%d drawn arrows between boxes", len(rows)))
	answers, err := r.runTable(ctx, def, 1, rows)
	if err != nil {
		return err
	}
	sentences := make(map[string]string, len(rows))
	for i, arrow := range order {
		if answer := answers[i]; answer.answer != nil {
			sentences[arrow.from+"\x00"+arrow.to] = answer.answer["sentence"]
		}
	}
	for _, target := range r.opts.Targets {
		for _, arrow := range r.arrows[target.ID] {
			if id := ids[arrow.from+"\x00"+arrow.to]; id != "" {
				arrow.id = id
			}
			if sentence, ok := sentences[arrow.from+"\x00"+arrow.to]; ok {
				arrow.sentence = sentence
				continue
			}
			arrow.sentence = lines.FallbackSentence(r.summary(r.boxes[arrow.from], target.ID), r.summary(r.boxes[arrow.to], target.ID), arrow.rankedWitnesses())
		}
	}
	r.reportStage(def.Stage)
	return nil
}

// readSymbols first selects roles from every candidate, then writes only the
// explanations used by the overview. Closing a presentation scope does not
// erase activation, integration or key-symbol decisions.
func (r *reader) readSymbols(ctx context.Context) error {
	r.symbolSelections = make(map[string]*Knowledge)
	var order []atlas.Place
	var rows, typeRows []table.Row
	var typeOrder []atlas.Place
	for _, place := range r.opts.Graph.Places {
		if place.Kind != atlas.PlaceSymbol || !place.Symbol.Candidate {
			continue
		}
		file := r.places[place.Parent]
		if file.File == nil || file.File.Generated {
			continue
		}
		if place.Symbol.Decl.Kind == "type" && len(place.Symbol.Members) == 0 && place.Symbol.Decl.Doc == "" {
			continue
		}
		if place.Symbol.Decl.Kind == "type" {
			row := lines.TypeRow(place)
			typeRows = append(typeRows, row)
			typeOrder = append(typeOrder, place)
		} else {
			fileLine, _ := r.Line(place.Parent)
			row := lines.SymbolRow(place, fileLine)
			rows = append(rows, row)
			order = append(order, place)
		}
	}
	r.opts.Stage(lines.StageSymbols, fmt.Sprintf("selecting key declarations, activations and outgoing calls: %d candidates; no descriptions yet", len(rows)+len(typeRows)))
	// Functions and types are selected at once: neither reads the other.
	// The types round runs on its own view, joined after the functions.
	selecting, cancel := context.WithCancel(ctx)
	defer cancel()
	types := r.view(nil)
	var typeAnswers []rowAnswer
	var typeErr error
	typesDone := make(chan struct{})
	go func() {
		defer close(typesDone)
		typeAnswers, typeErr = types.runTable(selecting, lines.SymbolSelection(true), 2, typeRows)
	}()
	answers, err := r.runTable(selecting, lines.SymbolSelection(false), 1, rows)
	if err != nil {
		cancel()
	}
	<-typesDone
	if err != nil {
		return err
	}
	if typeErr != nil {
		return typeErr
	}
	r.joinView(types)
	order = append(order, typeOrder...)
	answers = append(answers, typeAnswers...)
	type marked struct {
		id   string
		rank int
	}
	byFile := make(map[string][]marked)
	for i, place := range order {
		answer := answers[i]
		if answer.answer == nil {
			continue
		}
		subject := place.Symbol.Decl.ObjectID
		if subject == "" {
			subject = place.ID
		}
		unlock := r.lock()
		r.symbolSelections[subject] = r.knowledge[place.ID]
		delete(r.knowledge, place.ID)
		unlock()
		if answer.answer["key_symbol"] == "yes" {
			r.selectedKeys[place.ID] = true
			byFile[place.Parent] = append(byFile[place.Parent], marked{id: place.ID, rank: place.Symbol.Rank})
		}
	}
	for fileID, list := range byFile {
		sort.SliceStable(list, func(i, j int) bool { return list[i].rank < list[j].rank })
		if len(list) > lines.MaxKeysPerFile {
			list = list[:lines.MaxKeysPerFile]
		}
		for _, item := range list {
			r.keys[fileID] = append(r.keys[fileID], item.id)
		}
	}
	// Use the existing overview key selection, before model wording can affect
	// its order. Shared declarations receive one caption across their owners.
	selected := r.overviewKeys()
	rows, typeRows = nil, nil
	order, typeOrder = nil, nil
	for _, place := range r.opts.Graph.Places {
		if !selected[place.ID] {
			continue
		}
		if place.Symbol.Decl.Kind == "type" {
			typeRows = append(typeRows, lines.TypeRow(place))
			typeOrder = append(typeOrder, place)
		} else {
			fileLine, _ := r.Line(place.Parent)
			rows = append(rows, lines.SymbolRow(place, fileLine))
			order = append(order, place)
		}
	}
	r.opts.Stage(lines.StageSymbols, fmt.Sprintf("describing %d overview declarations (%d types); all original sources remain available to questions", len(rows)+len(typeRows), len(typeRows)))
	answers, err = r.describeDeclarations(ctx,
		declarationTable{def: lines.Symbols(), round: 3, aliasRound: 5, places: order, rows: rows},
		declarationTable{def: lines.Types(), round: 4, aliasRound: 6, places: typeOrder, rows: typeRows})
	if err != nil {
		return err
	}
	order = append(order, typeOrder...)
	for i, place := range order {
		// Without captions a symbol whose name needs an alias is asked the
		// alias alone: it has no line to show.
		if line, asked := answers[i].answer["line"]; asked {
			// A type's prose line keeps its paragraphs in the answer, but the
			// atlas shows it as one line: a newline or tab there is form, and
			// it would fail the atlas and group index validation.
			r.symbolLine[place.ID] = cell{value: table.OneLine(line), source: answers[i].source}
		}
	}
	r.reportStage(lines.StageSymbols)
	return nil
}

// declarationTable is one description table of overview declarations: its
// rows beside their places, and the rounds of its two request shapes.
type declarationTable struct {
	def               table.Definition
	round, aliasRound int
	places            []atlas.Place
	rows              []table.Row
}

// describeDeclarations asks the description tables of the overview
// declarations and returns their answers in table and row order. Only a name
// that lines.NeedsAlias is asked its English alias, with or without captions
// (owner decision 2026-09-26). Columns belong to a request, so those rows go
// to the complete table in its aliasRound, and the others to the table
// without the alias in its round, in requests of their own. The others keep
// their other cells and decisions: without captions an English function is
// still asked nothing, and with captions a row loses only the alias cell. An
// English type is then asked alike in both modes. No request reads another's
// answer, so all are asked at once, each on its own view, and their tables
// and counts join in step order. The first failure cancels the others and is
// the error returned; the cancellations it causes are not.
func (r *reader) describeDeclarations(ctx context.Context, tables ...declarationTable) ([]rowAnswer, error) {
	type part struct {
		def   table.Definition
		round int
		at    []int
		rows  []table.Row
		view  *reader
		got   []rowAnswer
	}
	var parts []*part
	total := 0
	for _, described := range tables {
		plain := &part{def: withoutAlias(described.def), round: described.round}
		named := &part{def: described.def, round: described.aliasRound}
		for i, place := range described.places {
			into := plain
			if lines.NeedsAlias(place.Symbol.Decl.Name) {
				into = named
			}
			into.at = append(into.at, total+i)
			into.rows = append(into.rows, described.rows[i])
		}
		total += len(described.rows)
		parts = append(parts, plain, named)
	}
	asking, cancel := context.WithCancel(ctx)
	defer cancel()
	var failed sync.Once
	var failure error
	var wg sync.WaitGroup
	for _, p := range parts {
		p.view = r.view(nil)
		wg.Go(func() {
			var err error
			if p.got, err = p.view.runTable(asking, p.def, p.round, p.rows); err != nil {
				failed.Do(func() {
					failure = err
					cancel()
				})
			}
		})
	}
	wg.Wait()
	if failure != nil {
		return nil, failure
	}
	answers := make([]rowAnswer, total)
	for _, p := range parts {
		r.joinView(p.view)
		for j, i := range p.at {
			answers[i] = p.got[j]
		}
	}
	return answers, nil
}

// boundaryState is one accepted fact or candidate awaiting its own review.
type boundaryState struct {
	uses  []atlas.DestinationUse
	place atlas.Place
	line  string
	// name is an entry's chosen words, restored as written.
	name        string
	kind        string
	destination string
	address     string
	basis       string
}

// readBoundaries is the one semantic owner of candidate runtime relationships.
// Source facts survive refused prose; a refused candidate gains no boundary.
func (r *reader) readBoundaries(ctx context.Context) error {
	r.boundaries = make(map[string]*boundaryState)
	owners := make(map[string]atlas.Place)
	for _, place := range r.opts.Graph.Places {
		if place.Symbol != nil {
			owners[place.ID] = place
			if place.Symbol.Decl.ObjectID != "" {
				owners[place.Symbol.Decl.ObjectID] = place
			}
		}
		if place.Kind != atlas.PlaceBoundary {
			continue
		}
		state := &boundaryState{place: place, line: place.Given, kind: place.Boundary.GivenKind}
		if place.Boundary.GivenKind == atlas.BoundaryClientRequest {
			state.basis = "dispatch"
			if len(place.Boundary.Values) == 1 {
				state.address = place.Boundary.Values[0]
			}
		}
		r.boundaries[place.ID] = state
	}
	publishes := r.applyAPIRoles()
	publishes = append(publishes, r.bindInterpretedBoundaries()...)
	tracer := NewDestinationReader(r.opts.Graph.Places)
	for _, state := range r.boundaries {
		facts := state.place.Boundary
		if facts.Direction != atlas.DirectionOut {
			continue
		}
		owner := boundaryOwner(facts, owners)
		if owner.Symbol == nil {
			continue
		}
		for _, call := range owner.Symbol.Calls {
			if call.Line == state.place.LineNo && call.Column == state.place.Column {
				state.uses = append(state.uses, tracer.Read(owner, call)...)
			}
		}
		state.uses = canonicalDestinationUses(state.uses)
		if len(state.uses) == 1 && state.uses[0].Address != "" {
			state.address = state.uses[0].Address
		}
	}
	var ids []string
	for id := range r.boundaries {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return compactIDLess(ids[i], ids[j]) })
	r.opts.Stage(lines.StageBoundaries, fmt.Sprintf("reviewing runtime relationships: %d source candidates and facts", len(ids)))
	// Every boundary here has its kind: the facts and the symbol roles gave
	// it. The table explains it, and for an outgoing one chooses the
	// destination and the address among the observed values.
	for mode := 0; mode < 2; mode++ {
		outgoing, fixed := mode == 1, true
		def := lines.FixedBoundaries(outgoing)
		// The rows of one declaration share one window: the declaration, its
		// source context and the destination catalogue are sent once, and a
		// row names the declaration by owner_ref. Rows without a declaration
		// share a window without owners.
		type ownerGroup struct {
			owner  atlas.Place
			states []*boundaryState
		}
		byOwner := make(map[string]*ownerGroup)
		var keys []string
		for _, id := range ids {
			state := r.boundaries[id]
			// A candidate an earlier mode refused is gone; an accepted
			// operation already owns its incoming interpretation.
			if state == nil || state.place.Boundary.Source == "model" && state.place.Boundary.Direction == atlas.DirectionIn {
				continue
			}
			facts := state.place.Boundary
			isOutgoing := facts.Direction == atlas.DirectionOut && facts.GivenKind != atlas.BoundaryConfig && facts.GivenKind != atlas.BoundaryOther
			if isOutgoing != outgoing || (facts.GivenKind != "") != fixed {
				continue
			}
			r.places[id] = state.place
			// Without captions an incoming row asks only its entry's name:
			// a row with no words to name it by asks nothing and keeps its
			// given line.
			if !outgoing && r.opts.NoCaptions && len(lines.EntryWords(state.place)) == 0 {
				continue
			}
			owner := boundaryOwner(facts, owners)
			// An entry is named from its own words, and its handler's calls
			// near a registration elsewhere say nothing about them: entries
			// pack into shared windows without an owner, one window per
			// handler cost Redis 98 requests for 97 names.
			if len(lines.EntryWords(state.place)) > 0 {
				owner = atlas.Place{}
			}
			key := ""
			if owner.Symbol != nil {
				key = owner.ID
			}
			group, known := byOwner[key]
			if !known {
				group = &ownerGroup{owner: owner}
				byOwner[key] = group
				keys = append(keys, key)
			}
			group.states = append(group.states, state)
		}
		sort.Strings(keys)
		var groups rowGroups
		var order []*boundaryState
		addressValues := make(map[string][]lines.BoundaryAddress)
		catalogs := make(map[string][]destinations.Entry)
		for _, key := range keys {
			group := byOwner[key]
			ownerRef := ""
			var original []atlas.Place
			if group.owner.Symbol != nil {
				ownerRef = "o1"
				original = append(original, group.owner)
			}
			var rows []table.Row
			var rowLines []int
			var catalog []destinations.Entry
			if outgoing {
				catalog = destinations.Catalog(r.targetDependencies(group.states))
			}
			for _, state := range group.states {
				addresses := lines.BoundaryAddresses(state.place, original...)
				if len(state.uses) > 0 {
					addresses = destinationAddresses(state.uses)
				}
				// The code knows the address of a native HTTP fact with one
				// value and of a call whose traced chains end in one value;
				// such a row has no address decision.
				askAddress := outgoing && state.address == ""
				row := lines.BoundaryRow(state.place, ownerRef, addresses, askAddress)
				if len(state.uses) > 0 {
					row.Fields = append(row.Fields, table.Field{Name: "destination_chains", Value: destinationEvidence(state.uses)})
				}
				rows = append(rows, row)
				rowLines = append(rowLines, state.place.LineNo)
				if askAddress {
					addressValues[state.place.ID] = addresses
				}
				catalogs[state.place.ID] = catalog
				order = append(order, state)
			}
			var shared []table.Field
			if group.owner.Symbol != nil {
				var sourceContext []table.Field
				if outgoing {
					sourceContext = lines.BoundarySourceContext(group.states[0].place, group.owner, r.places, owners)
				}
				shared = append(shared, lines.BoundaryOwnerContext(lines.BoundaryOwner(ownerRef, group.owner, rowLines, sourceContext)))
			}
			if outgoing {
				shared = append(shared, lines.DestinationFields(catalog)...)
			}
			groups = append(groups, rowGroup{shared: shared, rows: rows})
		}
		answers, err := r.runTableGroups(ctx, def, mode+1, groups, nil)
		if err != nil {
			return err
		}
		for i, state := range order {
			answer := answers[i].answer
			if answer == nil || (!fixed && answer["decision"] != "boundary") {
				if state.place.Boundary.GivenKind == "" {
					delete(r.boundaries, state.place.ID)
				}
				continue
			}
			// A cell refused alone keeps the fact's given line, the handler's
			// own name and no destination or address.
			if line, ok := answer["line"]; ok {
				state.line = line
			}
			if cell, ok := answer["name"]; ok {
				state.name = lines.EntryName(lines.EntryWords(state.place), cell)
			}
			if !fixed {
				state.kind = answer["kind"]
			}
			if outgoing {
				state.destination = destinationChoice(def, catalogs[state.place.ID], answer["destination"])
				if !fixed && state.basis == "" {
					state.basis = answer["basis"]
					if state.basis == "remote_client_instance" {
						state.basis = "configuration"
					}
				}
				if state.address == "" {
					for _, address := range addressValues[state.place.ID] {
						if address.Ref == answer["address"] {
							state.address = address.Value
							break
						}
					}
				}
			}
		}
	}
	r.reportStage(lines.StageBoundaries)
	return r.joinPublishes(ctx, publishes)
}

// destinationChoice reads a destination cell: the system a d* ref names in
// the window's catalogue, or the text the model wrote after the free prefix.
// A new atlas thus stores the canonical system name; the report folds only
// older free text onto it.
func destinationChoice(def table.Definition, catalog []destinations.Entry, cell string) string {
	for _, column := range def.Columns {
		if column.Name != "destination" {
			continue
		}
		if text, ok := table.IsFree(column, cell); ok {
			return strings.TrimSpace(text)
		}
	}
	return destinations.Value(catalog, cell)
}

// targetDependencies are the external packages imported by the targets the
// rows belong to, the evidence the destination catalogue is annotated with.
func (r *reader) targetDependencies(states []*boundaryState) []string {
	wanted := make(map[string]bool)
	for _, state := range states {
		for _, id := range state.place.TargetIDs {
			wanted[id] = true
		}
	}
	seen := make(map[string]bool)
	var result []string
	for _, target := range r.opts.Targets {
		if !wanted[target.ID] {
			continue
		}
		for _, dependency := range target.Dependencies {
			if !seen[dependency] {
				seen[dependency] = true
				result = append(result, dependency)
			}
		}
	}
	sort.Strings(result)
	return result
}

// Native SubjectID names the shared compiler-located declaration even when
// the row's original ObjectID belongs to another target's native view.
func boundaryOwner(facts *atlas.BoundaryFacts, owners map[string]atlas.Place) atlas.Place {
	if owner := owners[facts.SubjectID]; owner.Symbol != nil {
		return owner
	}
	return owners[facts.ObjectID]
}

// Interpretation adds candidate source calls before the boundary review. A
// selected call is not yet an accepted SDK relationship. Source columns and
// native call identities distinguish calls sharing a line or declaration.
func (r *reader) bindInterpretedBoundaries() []*boundaryState {
	var publishes []*boundaryState
	if r.boundaryIDs == nil {
		r.boundaryIDs = make(map[string]string)
	}
	boundaryID := func(source string) string {
		if id := r.boundaryIDs[source]; id != "" {
			return id
		}
		id := r.compactID("b", &r.nextBoundary)
		r.boundaryIDs[source] = id
		return id
	}
	for _, place := range r.opts.Graph.Places {
		if place.Symbol == nil {
			continue
		}
		// A target whose program never runs the declaration does not make
		// the calls it writes.
		targets := runningTargets(place)
		if len(targets) == 0 {
			continue
		}
		decl := place.Symbol.Decl
		for _, call := range place.Symbol.Calls {
			// A call to a symbol that talks to another system is that
			// outgoing boundary at every site; a call to a symbol that
			// publishes is the listener at every site.
			if call.Line < 1 || call.Kind != string(programindex.RelationInvokesExternal) || call.API == nil {
				continue
			}
			role := r.api[apiName(*call.API)]
			if role.talks == "" && !role.publishes {
				continue
			}
			claimed := false
			for _, existing := range r.boundaries {
				p := existing.place
				if p.Boundary.Source != "model" && p.Path == place.Path && p.LineNo == call.Line && p.Boundary.Direction == atlas.DirectionOut {
					// Every native source counts ("fact" from route/config/http
					// facts, "external_call" from SDK observations); only the
					// model's own interpreted boundaries are not facts. A fact
					// with a column claims exactly its own call, so two calls on
					// one line stay apart. A fact without a column, such as an
					// SDK boundary from an external_call observation, claims the
					// whole line: Morfeu 20260911-152759 and 171727 otherwise
					// reviewed the same call twice (bnd:internal/broker/
					// client.go:166:sdk beside out:…PublicarComConfirm:166:42)
					// and listed it twice.
					if p.Column == 0 || p.Column == call.Column {
						claimed = true
						break
					}
				}
			}
			if claimed {
				continue
			}
			direction, kind := atlas.DirectionOut, role.talks
			if role.publishes {
				direction, kind = atlas.DirectionIn, atlas.BoundaryListenAddress
			}
			id := boundaryID(fmt.Sprintf("%s\x00%s\x00%d\x00%d\x00%s\x00%s\x00%s", direction, place.ID, call.Line, call.Column, call.Kind, call.Name, call.Resolution))
			p := atlas.Place{ID: id, Kind: atlas.PlaceBoundary, Path: place.Path, LineNo: call.Line, Column: call.Column,
				Parent: place.Parent, TargetIDs: slices.Clone(targets), Boundary: &atlas.BoundaryFacts{
					Source: "model", ObjectID: decl.ObjectID, Caller: decl.Name, CallerDoc: decl.Doc, External: call.Name,
					Values: append([]string{}, call.Values...), Direction: direction, GivenKind: kind}}
			state := &boundaryState{place: p, kind: kind}
			if role.publishes {
				state.address = publishAddress(call.Values)
				publishes = append(publishes, state)
			}
			r.boundaries[id] = state
		}
	}
	return publishes
}

// runningTargets are the targets holding a place whose program may run it:
// for a declaration, every target but those that proved it unreached.
func runningTargets(place atlas.Place) []string {
	if place.Symbol == nil || len(place.Symbol.Unreached) == 0 {
		return place.TargetIDs
	}
	var targets []string
	for _, target := range place.TargetIDs {
		if !slices.Contains(place.Symbol.Unreached, target) {
			targets = append(targets, target)
		}
	}
	return targets
}

// An equal terminal identifier is only a candidate for the model to confirm.
// It never establishes that two interfaces are connected.
func operationIdentifier(name string) string {
	parts := strings.FieldsFunc(name, func(r rune) bool { return r == '.' || r == '/' || r == '(' || r == ')' || r == '*' })
	if len(parts) == 0 {
		return name
	}
	return parts[len(parts)-1]
}

// side says which column a box stands in: in when the outside calls into
// it or execution starts there, out when it only calls out, mid otherwise.
func (r *reader) side(owner *boxState, targetID string) string {
	in, out := false, false
	for _, fileID := range owner.files {
		if contains(r.places[fileID].TargetIDs, targetID) && contains(r.opts.Graph.Seeds, fileID) && contains(r.seedBoxes(targetID, fileID), owner.id) {
			in = true
		}
	}
	for _, state := range r.boundaries {
		if r.boundaryBox(targetID, state.place) != owner.id || !contains(state.place.TargetIDs, targetID) {
			continue
		}
		// Configuration describes how this code is parameterized. Reading an
		// environment key does not make the entire module an integration.
		if state.kind == "config" {
			continue
		}
		if state.place.Boundary.Direction == atlas.DirectionIn {
			in = true
		} else {
			out = true
		}
	}
	switch {
	case in:
		return atlas.SideIn
	case out:
		return atlas.SideOut
	default:
		return atlas.SideMid
	}
}

// trace follows the busiest arrows forward from the entry points, up to
// eight boxes.
func (r *reader) trace(targetID string) []string {
	var start []string
	for _, seed := range r.opts.Graph.Seeds {
		if !contains(r.places[seed].TargetIDs, targetID) {
			continue
		}
		for _, boxID := range r.seedBoxes(targetID, seed) {
			if !contains(start, boxID) {
				start = append(start, boxID)
			}
		}
	}
	if len(start) == 0 {
		return []string{}
	}
	sort.Strings(start)
	trace := []string{start[0]}
	visited := map[string]bool{start[0]: true}
	current := start[0]
	for len(trace) < 8 {
		var next *arrowState
		for _, arrow := range r.arrows[targetID] {
			if arrow.from != current || visited[arrow.to] {
				continue
			}
			if next == nil || arrow.calls > next.calls || arrow.calls == next.calls && arrow.to < next.to {
				next = arrow
			}
		}
		if next == nil {
			break
		}
		visited[next.to] = true
		trace = append(trace, next.to)
		current = next.to
	}
	return trace
}

func parentDir(filePath string) string {
	dir := path.Dir(filePath)
	if dir == "" || dir == "/" {
		return "."
	}
	return dir
}
