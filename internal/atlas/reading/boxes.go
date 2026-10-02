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
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/atlas/table"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/sourcevalue"
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
	// sources are the files its units are declared in: its directory, its
	// test fact, its description and its area dirs come from them.
	sources []string
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
// part is drawn: each file of a target describes up to three of the keys the
// selection found in it, those with a line or a docstring first, then by
// name. A declaration shared by targets is chosen once.
func (r *reader) overviewKeys() map[string]bool {
	type key struct {
		id, name   string
		documented bool
	}
	selected := make(map[string]bool)
	for _, target := range r.opts.Targets {
		for _, place := range r.opts.Graph.Places {
			if place.Kind != atlas.PlaceFile || !contains(place.TargetIDs, target.ID) {
				continue
			}
			var keys []key
			for _, decl := range place.File.Decls {
				id := r.symbolID(place.Path, decl.LineNo, decl.Name)
				if !contains(r.keys[place.ID], id) {
					continue
				}
				line, lined := r.symbolLine[id]
				keys = append(keys, key{id: id, name: decl.Name, documented: lined && line.value != "" || decl.Doc != ""})
			}
			sort.SliceStable(keys, func(i, j int) bool {
				if keys[i].documented != keys[j].documented {
					return keys[i].documented
				}
				return keys[i].name < keys[j].name
			})
			for _, chosen := range keys[:min(len(keys), 3)] {
				if contains(r.keys[r.places[chosen.id].Parent], chosen.id) {
					selected[chosen.id] = true
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
// line is the joint request's context: the model's line, else the fact's
// given text; written says the model wrote it, and only such a line is the
// boundary's (writtenLine).
type boundaryState struct {
	uses []atlas.DestinationUse
	// through is where an outgoing row's exchange ends (exchangeThrough):
	// its destination key and its destination's ends, where uses are the
	// row's own walk, its address and its chain.
	through []atlas.DestinationUse
	place   atlas.Place
	line    string
	written bool
	// name is an entry's chosen words, restored as written; unnamed marks
	// an entry the model answered no written word names.
	name        string
	unnamed     bool
	kind        string
	destination string
	// destinations are, by program, what an outgoing row reaches as that
	// program's destination is named (destination_groups.go); a program
	// not listed takes destination.
	destinations map[string]string
	// destinationTargets are, by program, the target of this repository's
	// program an outgoing row's destination is (programDestinations).
	destinationTargets map[string]string
	// branch is, for an entry a comparison's case declares, the lines of
	// that case when they call into the program's own code: the entry's
	// handler is the comparing declaration there (bindComparisons).
	branch  [2]int
	address string
	basis   string
	// handlerUnknown marks an entry whose handler is not established: an
	// option a call declares, a value handed over. It stays where its call
	// is written and binds to no part.
	handlerUnknown bool
	// apiSymbol is the outside symbol a boundary the roles made calls, as
	// the atlas_api table names it.
	apiSymbol string
	// outside is the outside package an outgoing boundary's call goes
	// through, when the code names the call at its site; reaching marks a
	// row whose uses walk that call's value (destination_groups.go).
	outside  string
	reaching bool
	// programNotNamed marks a call starting another program that none of
	// its words names; destination then holds no program.
	programNotNamed bool
	// on is the object an incoming boundary's call is made on (declared.go).
	on *atlas.DeclaredOn
	// asWritten is an incoming boundary's registration as the code wrote it
	// (declared.go markDeclaredOn).
	asWritten string
	// element is, for an entry a call's words make, the element of another
	// value its call compares (strcasecmp(argv[1],"always"): element 1 of
	// what sdssplitlen returned at redis.c:1642); valueOf the entry it is
	// a value of (values.go).
	element *valueElement
	valueOf string
	// sameValueAs is, for an entry a call's words make, where the earlier
	// call its call reads the same value as is written; aliasOf the entry
	// it is another spelling of (spellings.go).
	sameValueAs *sourcevalue.Anchor
	aliasOf     string
	// tableRow marks an entry a row of an accepted table makes (inputs.go);
	// rowNeighbours are the neighbouring rows' nearest words, as line and
	// column, which bound the row as written (declared.go).
	tableRow      bool
	rowNeighbours [][2]int
	// names are a handed row's words after its key (handed_tables.go).
	names []string
}

// destinationOf is what the row reaches in one of its programs.
func (state *boundaryState) destinationOf(targetID string) string {
	if name, named := state.destinations[targetID]; named {
		return name
	}
	return state.destination
}

// writtenLine is the boundary's line: only one the model wrote. A fixed
// fact's given text restates the registration ("redis.c.redisCommand.proc
// get in getCommand"), which the entry's name and handler already say.
func (state *boundaryState) writtenLine() string {
	if !state.written {
		return ""
	}
	return state.line
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
	r.applyStored()
	r.applyStarts()
	publishes := r.applyAPIRoles()
	publishes = append(publishes, r.bindInterpretedBoundaries()...)
	publishes = r.dropTestBoundaries(publishes)
	r.foldValues()
	r.foldSpellings()
	r.bindTableRows()
	r.markDeclaredOn()
	r.labelWordsGiven()
	if err := r.readPrograms(ctx); err != nil {
		return err
	}
	talks := make(map[string]string, len(r.api))
	for symbol, role := range r.api {
		talks[symbol] = role.talks
	}
	tracer := NewDestinationReader(r.opts.Graph.Places, DestinationChoices{Arguments: r.arguments, Options: r.optionNames(), Talks: talks})
	for _, state := range r.boundaries {
		facts := state.place.Boundary
		// A started program has no address: the word naming it is its
		// participant.
		if facts.Direction != atlas.DirectionOut || state.kind == atlas.BoundaryRunsProgram {
			continue
		}
		owner := boundaryOwner(facts, owners)
		if owner.Symbol == nil {
			continue
		}
		for _, call := range owner.Symbol.Calls {
			if call.Line == state.place.LineNo && call.Column == state.place.Column {
				// The row's own call is the one its external names or, for a
				// fact naming none (an SQL text on db.Exec), the call at its
				// site whose symbol talks to something: the fact claimed
				// that call's boundary (bindInterpretedBoundaries).
				state.uses = append(state.uses, tracer.Read(owner, call)...)
				if callsExternal(call, facts.External) || facts.External == "" && r.talksOut(call) {
					state.through = append(state.through, r.exchangeThrough(tracer, owner, call, state.kind)...)
					state.outside, state.reaching = call.API.Package, true
				}
			}
		}
		// A fact naming no call whose site holds none that talks, such as
		// the SQL text a fmt.Sprintf formats, is sent by the talking call
		// handed what the call at its site returns (db.Exec(fmt.Sprintf(…))):
		// its destination is that call's, and so is its walk.
		if !state.reaching && facts.External == "" {
			site := sourcevalue.Anchor{Path: state.place.Path, Line: state.place.LineNo, Column: state.place.Column}
			for _, call := range owner.Symbol.Calls {
				if r.talksOut(call) && handsResultOf(call, site) {
					state.uses = tracer.Read(owner, call)
					state.through = r.exchangeThrough(tracer, owner, call, state.kind)
					state.outside, state.reaching = call.API.Package, true
					break
				}
			}
		}
		state.uses = canonicalDestinationUses(state.uses)
		state.through = canonicalDestinationUses(state.through)
	}
	var ids []string
	for id := range r.boundaries {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return compactIDLess(ids[i], ids[j]) })
	r.opts.Stage(lines.StageBoundaries, fmt.Sprintf("reviewing runtime relationships: %d source candidates and facts", len(ids)))
	// Every boundary here has its kind: the facts and the symbol roles gave
	// it. The table explains it, and for an outgoing one chooses the
	// address among the observed values; what an outgoing call reaches is
	// named once per destination (nameDestinations).
	for mode := 0; mode < 2; mode++ {
		outgoing, fixed := mode == 1, true
		def := lines.FixedBoundaries(outgoing)
		// The rows of one declaration share one window: the declaration and
		// its source context are sent once, and a row names the declaration
		// by owner_ref. Rows without a declaration share a window without
		// owners.
		type ownerGroup struct {
			owner  atlas.Place
			states []*boundaryState
		}
		byOwner := make(map[string]*ownerGroup)
		var keys []string
		for _, id := range ids {
			state := r.boundaries[id]
			// A candidate an earlier mode refused is gone; a listener the
			// model's roles made has no words to be named by. An entry the
			// words of a call make is named like any other.
			if state == nil || state.place.Boundary.Source == "model" && state.place.Boundary.Direction == atlas.DirectionIn && len(lines.EntryWords(state.place)) == 0 {
				continue
			}
			// A call starting another program has its one decision, which
			// word names the program, in the program table.
			if state.kind == atlas.BoundaryRunsProgram {
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
			// given line, and an entry whose handler is not established,
			// with one word, is named by that word.
			if words := lines.EntryWords(state.place); !outgoing && r.opts.NoCaptions && (len(words) == 0 || state.handlerUnknown && len(words) == 1) {
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
		// The outside system each package the outgoing rows call through
		// reaches is asked once, and each destination the rows reach is
		// named once (nameDestinations), never per row.
		if outgoing {
			packages := make(targetPackages)
			var states []*boundaryState
			for _, key := range keys {
				for _, state := range byOwner[key].states {
					// A package this repository builds is no outside system:
					// its calls are named by the destination question,
					// where the repository's programs are offered.
					if !r.repositoryPackage(state.outside) {
						packages.add(state)
					}
					states = append(states, state)
				}
			}
			names, err := r.readSystems(ctx, packages.all())
			if err != nil {
				return err
			}
			if err := r.nameDestinations(ctx, states, packages, names, owners); err != nil {
				return err
			}
		}
		var groups rowGroups
		var order []*boundaryState
		addressValues := make(map[string][]lines.BoundaryAddress)
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
			var states []*boundaryState
			for _, state := range group.states {
				addresses := lines.BoundaryAddresses(state.place, original...)
				if len(state.uses) > 0 {
					addresses = destinationAddresses(state.uses)
				}
				// The code knows the address of a native HTTP fact with one
				// value; such a row has no address decision. A walk ending in
				// one value is asked like several: what reached the call is
				// not therefore where it connects (casdoor's LDAP dials ended
				// in "%s:%d", an oss List in an object key's "%s/%s",
				// freqtrade's inspector in the table "trades").
				askAddress := outgoing && state.address == ""
				// Without captions an outgoing row with no address to
				// choose has nothing to decide and is not sent.
				if outgoing && r.opts.NoCaptions && (!askAddress || len(addresses) == 0) {
					continue
				}
				row := lines.BoundaryRow(state.place, ownerRef, addresses, askAddress)
				if outgoing && state.outside != "" {
					row.Fields = append(row.Fields, table.Field{Name: "package", Value: state.outside})
				}
				if len(state.uses) > 0 {
					row.Fields = append(row.Fields, table.Field{Name: "destination_chains", Value: destinationEvidence(state.uses)})
				}
				rows = append(rows, row)
				rowLines = append(rowLines, state.place.LineNo)
				if askAddress {
					addressValues[state.place.ID] = addresses
				}
				states = append(states, state)
			}
			if len(rows) == 0 {
				continue
			}
			var shared []table.Field
			if group.owner.Symbol != nil {
				var sourceContext []table.Field
				if outgoing {
					sourceContext = lines.BoundarySourceContext(states[0].place, group.owner, r.places, owners)
				}
				shared = append(shared, lines.BoundaryOwnerContext(lines.BoundaryOwner(ownerRef, group.owner, rowLines, sourceContext)))
			}
			groups = append(groups, rowGroup{shared: shared, rows: rows})
			order = append(order, states...)
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
			// A cell refused alone leaves the boundary no line (the fact's
			// given text stays the joints' context), the handler's own name
			// and no destination or address.
			if line, ok := answer["line"]; ok {
				state.line, state.written = line, true
			}
			if cell, ok := answer["name"]; ok {
				state.name = lines.EntryName(lines.EntryWords(state.place), cell)
				state.unnamed = state.name == ""
			}
			if !fixed {
				state.kind = answer["kind"]
			}
			if outgoing {
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
	// An entry the model answered no written word names is named by its
	// handler (GroupsIndex), never by a word the code picks over that
	// answer: freqtrade's Query(…, description=…) had been named by its
	// description. One whose handler is not established has no handler to
	// be named by, so it is no entry, journaled (entry_unnamed), and its
	// values stand alone. Asked nothing (one word, without captions) or
	// refused, it is named by the first word its code wrote
	// (lines.FirstEntryWord); its other words, a flag's default and usage,
	// stay its registration as written, never its name.
	dropped := map[string]bool{}
	for _, state := range r.boundaries {
		switch {
		case !state.handlerUnknown || state.name != "":
		case state.unnamed:
			r.noEntryUnnamed(state.place, state.kind)
			dropped[state.place.ID] = true
		default:
			state.name = lines.FirstEntryWord(state.place.Boundary)
		}
	}
	for id := range dropped {
		delete(r.boundaries, id)
	}
	for _, state := range r.boundaries {
		if dropped[state.valueOf] {
			state.valueOf = ""
		}
	}
	// Named, an entry a declaration names again is that entry.
	r.foldRepeated()
	r.reportStage(lines.StageBoundaries)
	return r.joinPublishes(ctx, publishes)
}

// exchangeThrough is where a row's exchange ends: what decides its
// destination key and the destination's ends, never its own address or
// chain (One destination, one name). A call made on what a call of its
// kind returned continues that call's exchange (one exchange, one
// boundary: `inspector.get_columns("trades")` on `inspect(engine)`), so it
// ends where the call that began the exchange ends. A walk ending at a
// field of an object an outside call with a decided argument made goes on
// through that call (Trade.session.bind). A call whose symbol decides no
// argument names what it reaches (an ORM's select, text or update) ends
// where the object it is sent through ends (DestinationReader.Exchange),
// when an outside call made that object; else at its own name.
func (r *reader) exchangeThrough(tracer *DestinationReader, place atlas.Place, call atlas.SymbolCall, kind string) []atlas.DestinationUse {
	own := destinationStep(place, call)
	begun := false
	for seen := map[sourcevalue.Anchor]bool{}; ; {
		receiver := call.ReceiverValue
		if receiver == nil || receiver.Kind != "call_result" || receiver.Anchor == nil || seen[*receiver.Anchor] {
			break
		}
		seen[*receiver.Anchor] = true
		found := false
		for _, candidate := range tracer.callSites[*receiver.Anchor] {
			if candidate.call.API != nil && r.api[apiName(*candidate.call.API)].talks == kind {
				place, call, found, begun = candidate.place, candidate.call, true, true
				break
			}
		}
		if !found {
			break
		}
	}
	tracer.objects = true
	defer func() { tracer.objects = false }()
	uses := tracer.Read(place, call)
	if tracer.chosen(call) == nil {
		if through := tracer.Exchange(place, call, kind); len(through) > 0 {
			uses = through
		}
	}
	if begun {
		for i := range uses {
			uses[i].Steps = append([]atlas.DestinationStep{own}, uses[i].Steps...)
		}
	}
	return uses
}

// destinationChoice reads a destination cell: the system a d* ref names in
// the row's catalogue, or the name the model wrote after the free prefix.
func destinationChoice(def table.Definition, catalog []lines.Destination, cell string) string {
	for _, column := range def.Columns {
		if column.Name != "destination" {
			continue
		}
		if text, ok := table.IsFree(column, cell); ok {
			return strings.TrimSpace(text)
		}
	}
	return lines.DestinationValue(catalog, cell)
}

// handsResultOf reports a call given, as its receiver or an argument, what
// the call at a site returns, directly or as a part of the value.
func handsResultOf(call atlas.SymbolCall, site sourcevalue.Anchor) bool {
	var carries func(value *sourcevalue.Value) bool
	carries = func(value *sourcevalue.Value) bool {
		if value == nil {
			return false
		}
		if value.Kind == "call_result" && value.Anchor != nil && *value.Anchor == site {
			return true
		}
		for i := range value.Parts {
			if carries(&value.Parts[i]) {
				return true
			}
		}
		return false
	}
	if carries(call.ReceiverValue) {
		return true
	}
	for _, argument := range call.SourceArguments {
		if carries(argument.Origin) {
			return true
		}
	}
	return false
}

// talksOut reports a call of an outside symbol whose talks answer sends
// work out: not a file, not another program started.
func (r *reader) talksOut(call atlas.SymbolCall) bool {
	if call.API == nil || call.API.Package == "" {
		return false
	}
	talks := r.api[apiName(*call.API)].talks
	return talks != "" && talks != lines.APIFile && talks != atlas.BoundaryRunsProgram
}

// callsExternal reports whether the call at a boundary's site is the
// boundary's own outside call: the boundary's external is the call as
// named, or ends with the symbol it calls. A query literal formatted where
// the boundary stands (fmt.Sprintf of an SQL text) is not the call that
// reaches anything.
func callsExternal(call atlas.SymbolCall, external string) bool {
	if call.API == nil || call.API.Package == "" || external == "" {
		return false
	}
	symbol := call.API.Name
	if call.API.Receiver != "" {
		symbol = strings.TrimPrefix(call.API.Receiver, "*") + "." + symbol
	}
	return external == call.Name || external == symbol || strings.HasSuffix(external, "."+symbol)
}

// targetPackages are, by target, the outside packages its outgoing rows
// call through.
type targetPackages map[string]map[string]bool

func (packages targetPackages) add(state *boundaryState) {
	if state.outside == "" {
		return
	}
	for _, target := range rowTargets(state) {
		if packages[target] == nil {
			packages[target] = make(map[string]bool)
		}
		packages[target][state.outside] = true
	}
}

// all lists every package once, sorted.
func (packages targetPackages) all() []string {
	var result []string
	for _, byPackage := range packages {
		for pkg := range byPackage {
			result = append(result, pkg)
		}
	}
	sort.Strings(result)
	return slices.Compact(result)
}

// catalog is the closed list one program's destination chooses from: the
// systems the outside packages of that program reach (lines.Destinations).
// It depends on nothing a window's other rows bring, so two calls through
// one package are offered the same choices wherever they are packed.
func (packages targetPackages) catalog(target string, names map[string]string) []lines.Destination {
	named := make(map[string]string)
	for pkg := range packages[target] {
		named[pkg] = names[pkg]
	}
	return lines.Destinations(named)
}

// rowTargets are the targets a row belongs to, sorted and once each.
func rowTargets(state *boundaryState) []string {
	targets := slices.Clone(state.place.TargetIDs)
	slices.Sort(targets)
	return slices.Compact(targets)
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
	// The outside symbol each call site calls: a call on what another call
	// returned names that call by its site.
	calledAt := make(map[sourceSite]string)
	callAt := make(map[sourceSite]atlas.SymbolCall)
	for _, place := range r.opts.Graph.Places {
		if place.Symbol == nil {
			continue
		}
		for _, call := range place.Symbol.Calls {
			if call.Kind == string(programindex.RelationInvokesExternal) && call.API != nil && call.Line > 0 {
				site := sourceSite{place.Path, call.Line, call.Column}
				calledAt[site] = apiName(*call.API)
				callAt[site] = call
			}
		}
	}
	handedOn := r.handedOnExchanges(calledAt, callAt)
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
		inTest := r.testFile(place.Parent)
		for _, call := range place.Symbol.Calls {
			// A call to a symbol that talks to another system is that
			// outgoing boundary at every site; a call to a symbol that
			// publishes is the listener at every site.
			if call.Line < 1 || call.Kind != string(programindex.RelationInvokesExternal) || call.API == nil {
				continue
			}
			symbol := apiName(*call.API)
			role := r.api[symbol]
			running := atlas.Place{Path: place.Path, TargetIDs: targets}
			// A call outside tests that gives words is what its own answer
			// says they become, unless a fact already names the call: an
			// entry is made of its words as the code wrote them, and its
			// handler is not established. A call giving no word to a symbol
			// whose words are an entry at another call may declare one: the
			// launch walk says it is unsure.
			if !inTest && !r.factClaims(place.Path, call.Line, call.Column) {
				if len(call.Values) == 0 && r.entering[symbol] {
					r.recordWordCall(running, decl.ObjectID, call.Line, call.Column, symbol, wordNoWords, "")
				}
				if kind := r.readWordCall(running, place.ID, decl.ObjectID, call.Line, call.Column, symbol, call.Values); kind != "" {
					id := boundaryID(fmt.Sprintf("%s\x00%s\x00%d\x00%d\x00%s\x00%s\x00%s", atlas.DirectionIn, place.ID, call.Line, call.Column, call.Kind, call.Name, call.Resolution))
					r.boundaries[id] = &boundaryState{kind: kind, handlerUnknown: true, element: comparedElement(call.SourceArguments), sameValueAs: cloneAnchor(call.SameValueAs), place: atlas.Place{
						ID: id, Kind: atlas.PlaceBoundary, Path: place.Path, LineNo: call.Line, Column: call.Column,
						Parent: place.Parent, TargetIDs: slices.Clone(targets), Boundary: &atlas.BoundaryFacts{
							Source: "model", ObjectID: decl.ObjectID, Caller: decl.Name, External: call.Name,
							Values: slices.Clone(call.Values), Words: slices.Clone(call.Values), Direction: atlas.DirectionIn, GivenKind: kind}}}
					continue
				}
			}
			// A call reaching a file by its path talks to no other program:
			// it is no outgoing boundary (api_argument.go logs its file).
			if role.talks == "" && !role.publishes || role.talks == lines.APIFile {
				continue
			}
			// One exchange is one boundary (sameExchange): a call on what a
			// call of the same kind returned continues that call's exchange
			// (cmd.Run() on exec.Command's command starts the program it
			// named; select(...).filter(...) builds the statement select
			// began), and a call whose result a call of the same kind is
			// handed is part of that call's (func.sum(...) in select(...)).
			// A command built on either branch is each branch's launch, and
			// the call on it is theirs.
			if role.talks != "" && (r.sameExchange(call.ReceiverValue, role.talks, calledAt) || handedOn[sourceSite{place.Path, call.Line, call.Column}]) {
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
			state := &boundaryState{place: p, kind: kind, apiSymbol: apiName(*call.API)}
			if role.publishes {
				state.address = publishAddress(call.Values)
				publishes = append(publishes, state)
			}
			r.boundaries[id] = state
		}
	}
	return publishes
}

// sameExchange reports a value that is what a call of a symbol answered
// the same talks kind returned: that call's result, or alternatives every
// one of which is such a result. A call made on it continues that call's
// exchange with the same system: it starts, waits for or reads the program
// the call named, or adds to the statement the call began, and the call
// that began it is the one boundary. An alternative from anywhere else
// leaves the value not one.
func (r *reader) sameExchange(value *sourcevalue.Value, kind string, calledAt map[sourceSite]string) bool {
	if value == nil {
		return false
	}
	switch value.Kind {
	case "call_result":
		return value.Anchor != nil && r.api[calledAt[sourceSite{value.Anchor.Path, value.Anchor.Line, value.Anchor.Column}]].talks == kind
	case "alternatives":
		for i := range value.Parts {
			if !r.sameExchange(&value.Parts[i], kind, calledAt) {
				return false
			}
		}
		return len(value.Parts) > 0
	}
	return false
}

// handedOnExchanges are the sites of the calls whose result is handed, as
// written, to a call of a symbol answered the same talks kind: the part
// builds what that call sends (func.count(...) in select(...), a text()
// statement handed to execute), so the call receiving it is the one
// boundary. A call made on such a result is handed on with it
// (func.sum(...).label(...) in select(...) takes func.sum along), and so is
// the chain it continues, through a call on it that has no talks answer of
// its own (func.count(...).label(...), whose label was never answered, took
// func.count along only once this was so): the call it is made on is still
// the part. A call answered another kind ends the chain, whatever carries
// it (requests.get(...).json() in select(...) keeps the request its own).
// Only a value handed whole counts: a field of a result, or a result
// formatted into another value, is not the call's own.
func (r *reader) handedOnExchanges(calledAt map[sourceSite]string, callAt map[sourceSite]atlas.SymbolCall) map[sourceSite]bool {
	handed := make(map[sourceSite]bool)
	type visit struct {
		site sourceSite
		kind string
	}
	visited := make(map[visit]bool)
	var hand func(site sourceSite, kind string)
	hand = func(site sourceSite, kind string) {
		talks := r.api[calledAt[site]].talks
		if visited[visit{site, kind}] || talks != "" && talks != kind {
			return
		}
		visited[visit{site, kind}] = true
		if talks == kind {
			handed[site] = true
		}
		if receiver := callAt[site].ReceiverValue; receiver != nil && receiver.Kind == "call_result" && receiver.Anchor != nil {
			hand(sourceSite{receiver.Anchor.Path, receiver.Anchor.Line, receiver.Anchor.Column}, kind)
		}
	}
	for site, call := range callAt {
		kind := r.api[calledAt[site]].talks
		if kind == "" || kind == lines.APIFile {
			continue
		}
		for _, argument := range call.SourceArguments {
			if origin := argument.Origin; origin != nil && origin.Kind == "call_result" && origin.Anchor != nil {
				if inner := (sourceSite{origin.Anchor.Path, origin.Anchor.Line, origin.Anchor.Column}); inner != site {
					hand(inner, kind)
				}
			}
		}
	}
	return handed
}

// factClaims reports a call a fact boundary already names, in or out: the
// SQL statement a query call sends, a registration at the call. A fact with
// a column names exactly its call, one without a column its whole line.
func (r *reader) factClaims(path string, line, column int) bool {
	for _, existing := range r.boundaries {
		p := existing.place
		if p.Boundary.Source != "model" && p.Path == path && p.LineNo == line && (p.Column == 0 || p.Column == column) {
			return true
		}
	}
	return false
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
		// environment key does not make the entire module an integration,
		// and an option its code declares does not make it the side work
		// comes in at: that entry is bound to no part.
		if state.kind == "config" || state.handlerUnknown {
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

func parentDir(filePath string) string {
	dir := path.Dir(filePath)
	if dir == "" || dir == "/" {
		return "."
	}
	return dir
}
