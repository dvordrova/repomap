package reading

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/destinations"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/atlas/table"
)

// boxState is one box before it is written per target: a directory, or a
// directory split by a title the model started.
type boxState struct {
	id     string
	dir    string
	title  string
	line   string
	files  []string // file place IDs
	top    bool
	open   bool
	zoneID map[string]string // per target
}

// assignBoxes turns the file rows' box choices into boxes, once, after the
// file rounds. A title started by fewer than two files was already cancelled.
func (r *reader) assignBoxes() {
	r.boxOf = make(map[string]string)
	r.boxes = make(map[string]*boxState)
	box := func(id, dir, title string) *boxState {
		if existing, ok := r.boxes[id]; ok {
			return existing
		}
		created := &boxState{id: id, dir: dir, title: title, zoneID: make(map[string]string)}
		if dirPlace, ok := r.places[atlas.DirectoryID(dir)]; ok {
			created.top = dirPlace.Directory.TopBox
			if line, ok := r.lines[dirPlace.ID]; ok && id == dir {
				created.line = line.value
			} else {
				created.line = dirPlace.Given
			}
			if title == "" {
				if t, ok := r.titles[dirPlace.ID]; ok {
					created.title = t.value
				} else {
					created.title = directoryTitle(dir)
				}
			}
		}
		r.boxes[id] = created
		return created
	}
	for _, place := range r.opts.Graph.Places {
		if place.Kind != atlas.PlaceFile {
			continue
		}
		dir := parentDir(place.Path)
		boxID, boxDir, title := dir, dir, ""
		switch choice := r.boxChoice[place.ID]; {
		case choice == "" || choice == lines.BoxHere:
		default:
			if newTitle, ok := table.IsFree(lines.Files().Columns[1], choice); ok {
				boxID, title = dir+"#"+atlas.Slug(newTitle), newTitle
			} else if _, ok := r.places[atlas.DirectoryID(choice)]; ok {
				boxID, boxDir = choice, choice
			}
		}
		owner := box(boxID, boxDir, title)
		if title != "" && owner.line == "" {
			owner.line = r.lines[place.ID].value
		}
		owner.files = append(owner.files, place.ID)
		r.boxOf[place.ID] = boxID
	}
	for _, owner := range r.boxes {
		sort.Strings(owner.files)
		// A box is open when its directory was, or when no budget asked.
		owner.open = !r.budget
		if open, decided := r.openDirs[atlas.DirectoryID(owner.dir)]; decided {
			owner.open = open
		}
	}
}

// boxesOfTarget lists the boxes holding files of one target, by ID.
func (r *reader) boxesOfTarget(targetID string) []*boxState {
	var result []*boxState
	for _, owner := range r.boxes {
		for _, fileID := range owner.files {
			if contains(r.places[fileID].TargetIDs, targetID) {
				result = append(result, owner)
				break
			}
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].id < result[j].id })
	return result
}

func (r *reader) targetFiles(owner *boxState, targetID string) int {
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

// zoneState is one part of a target.
type zoneState struct {
	id    string
	title string
	line  string
	boxes []string
}

// readZones names the parts of every target and places its boxes in them:
// the largest top boxes name the parts, the rest choose from the closed
// list, boxes beneath a top box inherit its part.
func (r *reader) readZones(ctx context.Context) error {
	r.zones = make(map[string][]*zoneState)
	r.opts.Stage(lines.StageZones, "naming the parts of every target from its largest boxes")
	for _, target := range r.opts.Targets {
		if err := r.readTargetZones(ctx, target.ID); err != nil {
			return err
		}
	}
	r.reportStage(lines.StageZones)
	return nil
}

func (r *reader) readTargetZones(ctx context.Context, targetID string) error {
	var tops []*boxState
	for _, owner := range r.boxesOfTarget(targetID) {
		if owner.top || strings.Contains(owner.id, "#") {
			tops = append(tops, owner)
		}
	}
	if all := r.boxesOfTarget(targetID); len(tops) < 4 && len(all) >= 4 {
		// A library whose root directory holds files has one top box and
		// everything beneath it; its parts are cut over all of its boxes.
		tops = all
	}
	if len(tops) < 4 {
		// A target of a few boxes needs no parts: each box is its own.
		for _, owner := range tops {
			zone := &zoneState{id: atlas.Slug(owner.title), title: owner.title, line: owner.line, boxes: []string{owner.id}}
			r.zones[targetID] = append(r.zones[targetID], zone)
			owner.zoneID[targetID] = zone.id
		}
		r.inheritZones(targetID, tops)
		return nil
	}
	parts, err := r.partition(ctx, targetID, tops, 1)
	if err != nil {
		return err
	}
	// A part holding more than half of the boxes is not a part, it is the
	// target under one name: client/v3 came back as "Official Go client ·
	// 18 groups". Such a part is partitioned once more on its own.
	for _, part := range parts {
		if len(part.boxes) < 6 || len(part.boxes)*2 <= len(tops) {
			r.zones[targetID] = append(r.zones[targetID], part)
			continue
		}
		inner := make([]*boxState, 0, len(part.boxes))
		for _, boxID := range part.boxes {
			inner = append(inner, r.boxes[boxID])
		}
		sub, err := r.partition(ctx, targetID, inner, 10)
		if err != nil {
			return err
		}
		if len(sub) < 2 {
			r.zones[targetID] = append(r.zones[targetID], part)
			continue
		}
		r.zones[targetID] = append(r.zones[targetID], sub...)
	}
	for _, zone := range r.zones[targetID] {
		for _, boxID := range zone.boxes {
			r.boxes[boxID].zoneID[targetID] = zone.id
		}
	}
	r.inheritZones(targetID, tops)
	// Empty parts vanish; every remaining part gets its sentence.
	kept := r.zones[targetID][:0]
	for _, zone := range r.zones[targetID] {
		if len(zone.boxes) > 0 {
			kept = append(kept, zone)
		}
	}
	r.zones[targetID] = kept
	var rows []table.Row
	for _, zone := range kept {
		var boxes []lines.BoxSummary
		for _, boxID := range zone.boxes {
			boxes = append(boxes, r.summary(r.boxes[boxID], targetID))
		}
		rows = append(rows, lines.ZoneLineRow(zone.id, zone.title, boxes))
	}
	described, err := r.runTableWith(ctx, lines.ZoneLines(), 3, []table.Field{{Name: "question", Value: "lines"}}, rows, nil)
	if err != nil {
		return err
	}
	for i, zone := range kept {
		if answer := described[i]; answer.answer != nil {
			zone.line = answer.answer["line"]
			continue
		}
		var names []string
		for _, boxID := range zone.boxes {
			names = append(names, r.boxes[boxID].title)
			if len(names) == 3 {
				break
			}
		}
		zone.line = fmt.Sprintf("%s: %s", boxCount(len(zone.boxes)), strings.Join(names, ", "))
	}
	return nil
}

// inheritZones gives boxes beneath the reviewed tops the part of their nearest
// placed ancestor. A refused top assignment remains unassigned.
func (r *reader) inheritZones(targetID string, tops []*boxState) {
	asked := make(map[string]bool, len(tops))
	for _, top := range tops {
		asked[top.id] = true
	}
	byID := make(map[string]*zoneState)
	for _, zone := range r.zones[targetID] {
		byID[zone.id] = zone
	}
	for _, owner := range r.boxesOfTarget(targetID) {
		if asked[owner.id] {
			continue
		}
		if _, placed := owner.zoneID[targetID]; placed {
			continue
		}
		dir := owner.dir
		for dir != "." {
			dir = parentDir(dir)
			ancestor, ok := r.boxes[dir]
			if !ok {
				continue
			}
			if zoneID, placed := ancestor.zoneID[targetID]; placed {
				owner.zoneID[targetID] = zoneID
				byID[zoneID].boxes = append(byID[zoneID].boxes, owner.id)
				break
			}
			if asked[ancestor.id] {
				break
			}
		}
	}
}

// arrowState is one box-to-box arrow of one target.
type arrowState struct {
	from, to  string
	calls     int
	witnesses map[string]int
	sentence  string
	drawn     bool
}

func (r *reader) foldArrows() {
	r.arrows = make(map[string][]*arrowState)
	for _, target := range r.opts.Targets {
		byPair := make(map[[2]string]*arrowState)
		for _, edge := range r.opts.Graph.Edges {
			from, to := r.boxOfPlace(edge.From), r.boxOfPlace(edge.To)
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
				arrow.witnesses[witness.Caller+"\x00"+witness.Callee]++
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
		// Up to six outgoing arrows per box are drawn, the busiest first.
		outgoing := make(map[string][]*arrowState)
		for _, arrow := range arrows {
			outgoing[arrow.from] = append(outgoing[arrow.from], arrow)
		}
		for _, list := range outgoing {
			sort.SliceStable(list, func(i, j int) bool { return list[i].calls > list[j].calls })
			for i, arrow := range list {
				arrow.drawn = i < 6
			}
		}
		r.arrows[target.ID] = arrows
	}
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
	type pair struct {
		key   string
		count int
	}
	pairs := make([]pair, 0, len(arrow.witnesses))
	for key, count := range arrow.witnesses {
		pairs = append(pairs, pair{key, count})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].count != pairs[j].count {
			return pairs[i].count > pairs[j].count
		}
		return pairs[i].key < pairs[j].key
	})
	result := make([]atlas.Witness, 0, 3)
	for _, p := range pairs {
		caller, callee, _ := strings.Cut(p.key, "\x00")
		result = append(result, atlas.Witness{Caller: caller, Callee: callee})
		if len(result) == 3 {
			break
		}
	}
	return result
}

// readArrows asks one sentence per drawn arrow, each box pair once across
// targets.
func (r *reader) readArrows(ctx context.Context) error {
	r.foldArrows()
	def := lines.Arrows()
	seen := make(map[string]*arrowState)
	var rows []table.Row
	var order []*arrowState
	for _, target := range r.opts.Targets {
		for _, arrow := range r.arrows[target.ID] {
			if !arrow.drawn || !r.boxes[arrow.from].open || !r.boxes[arrow.to].open {
				continue
			}
			// An arrow made only of import edges has no witness call to
			// describe; asked anyway, the model invents one (Morfeu arrows
			// r7 and r10 became "store or fetch cached data"). Such an
			// arrow keeps its fallback sentence and costs no row.
			if len(arrow.witnesses) == 0 {
				continue
			}
			key := arrow.from + "\x00" + arrow.to
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = arrow
			from, to := r.summary(r.boxes[arrow.from], target.ID), r.summary(r.boxes[arrow.to], target.ID)
			rows = append(rows, lines.ArrowRow(key, from, to, arrow.topWitnesses(), arrow.calls))
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
			if sentence, ok := sentences[arrow.from+"\x00"+arrow.to]; ok {
				arrow.sentence = sentence
				continue
			}
			arrow.sentence = lines.FallbackSentence(r.summary(r.boxes[arrow.from], target.ID), r.summary(r.boxes[arrow.to], target.ID), arrow.topWitnesses())
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
		id := "selection:" + place.ID
		selection := place
		selection.ID = id
		symbol := *place.Symbol
		selection.Symbol = &symbol
		if symbol.Decl.ObjectID == "" {
			symbol.Decl.ObjectID = place.ID
		}
		r.places[id] = selection
		if place.Symbol.Decl.Kind == "type" {
			row := lines.TypeRow(place)
			row.ID = id
			typeRows = append(typeRows, row)
			typeOrder = append(typeOrder, place)
		} else {
			fileLine, _ := r.Line(place.Parent)
			row := lines.SymbolRow(place, fileLine)
			row.ID = id
			rows = append(rows, row)
			order = append(order, place)
		}
	}
	r.opts.Stage(lines.StageSymbols, fmt.Sprintf("selecting key declarations, activations and outgoing calls: %d candidates; no descriptions yet", len(rows)+len(typeRows)))
	answers, err := r.runTable(ctx, lines.SymbolSelection(false), 1, rows)
	if err != nil {
		return err
	}
	typeAnswers, err := r.runTable(ctx, lines.SymbolSelection(true), 2, typeRows)
	if err != nil {
		return err
	}
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
		r.symbolSelections[subject] = r.knowledge["selection:"+place.ID]
		for _, ref := range strings.Fields(answer.answer["outbound"]) {
			n, err := strconv.Atoi(strings.TrimPrefix(ref, "c"))
			if err == nil && n > 0 && n <= len(place.Symbol.Calls) {
				r.outbound[place.ID] = append(r.outbound[place.ID], place.Symbol.Calls[n-1])
			}
		}
		if activation := answer.answer["activation"]; activation != "" && activation != "none" && activation != "unassessed" {
			r.operations[place.ID] = [3]string{activation}
		}
		if answer.answer["key_symbol"] == "yes" {
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
	r.assignBoxes()
	// Use the existing overview key selection, before model wording can affect
	// its order. Shared declarations receive one caption across their owners.
	selected := make(map[string]bool)
	for _, target := range r.opts.Targets {
		for _, box := range r.target(target).Boxes {
			for _, key := range box.Keys {
				place := r.places[key.SymbolID]
				if contains(r.keys[place.Parent], key.SymbolID) {
					selected[key.SymbolID] = true
				}
			}
		}
	}
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
	answers, err = r.runTable(ctx, lines.Symbols(), 3, rows)
	if err != nil {
		return err
	}
	typeAnswers, err = r.runTable(ctx, lines.Types(), 4, typeRows)
	if err != nil {
		return err
	}
	order = append(order, typeOrder...)
	answers = append(answers, typeAnswers...)
	for i, place := range order {
		if answers[i].answer != nil {
			r.symbolLine[place.ID] = cell{value: answers[i].answer["line"], source: answers[i].source}
		}
	}
	r.reportStage(lines.StageSymbols)
	return nil
}

// boundaryState is one accepted fact or candidate awaiting its own review.
type boundaryState struct {
	uses        []atlas.DestinationUse
	place       atlas.Place
	line        string
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
		if place.Boundary.GivenKind == atlas.BoundaryHTTPClient {
			state.basis = "dispatch"
			if len(place.Boundary.Values) == 1 {
				state.address = place.Boundary.Values[0]
			}
		}
		r.boundaries[place.ID] = state
	}
	r.bindInterpretedBoundaries()
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
	sort.Strings(ids)
	r.opts.Stage(lines.StageBoundaries, fmt.Sprintf("reviewing runtime relationships: %d source candidates and facts", len(ids)))
	for mode := 0; mode < 4; mode++ {
		outgoing, fixed := mode%2 == 1, mode < 2
		def := lines.Boundaries(outgoing)
		if fixed {
			def = lines.FixedBoundaries(outgoing)
		}
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
			// An accepted operation already owns its incoming interpretation.
			if strings.HasPrefix(id, "in:") {
				continue
			}
			facts := state.place.Boundary
			isOutgoing := facts.Direction == atlas.DirectionOut && facts.GivenKind != atlas.BoundaryConfig && facts.GivenKind != atlas.BoundaryOther
			if isOutgoing != outgoing || (facts.GivenKind != "") != fixed {
				continue
			}
			owner := boundaryOwner(facts, owners)
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
			r.places[id] = state.place
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
			state.line = answer["line"]
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
	return nil
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
func (r *reader) bindInterpretedBoundaries() {
	nativeRoutes := make(map[string]bool)
	for _, place := range r.opts.Graph.Places {
		if b := place.Boundary; b != nil && b.SubjectID != "" && b.Source == "fact" && b.Direction == atlas.DirectionIn && b.GivenKind == atlas.BoundaryHTTPServer && b.Method != "" {
			nativeRoutes[b.SubjectID] = true
		}
	}
	for _, place := range r.opts.Graph.Places {
		if place.Symbol == nil {
			continue
		}
		decl := place.Symbol.Decl
		if operation := r.operations[place.ID]; operation[0] == "request" && !nativeRoutes[place.ID] {
			id := "in:" + place.ID
			p := atlas.Place{ID: id, Kind: atlas.PlaceBoundary, Path: place.Path, LineNo: place.LineNo, Column: decl.Column,
				Parent: place.Parent, TargetIDs: append([]string(nil), place.TargetIDs...), Boundary: &atlas.BoundaryFacts{
					Source: "model", ObjectID: decl.ObjectID, Caller: decl.Name, CallerDoc: decl.Doc, External: decl.Name,
					Values: []string{}, Direction: atlas.DirectionIn}}
			r.boundaries[id] = &boundaryState{place: p, line: operation[2], kind: atlas.BoundaryOther}
		}
		for _, call := range r.outbound[place.ID] {
			if call.Line < 1 {
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
			identity, _ := json.Marshal(struct {
				Kind, Name, Invocation, Resolution string
				Callees                            []string
			}{call.Kind, call.Name, call.Invocation, call.Resolution, call.CalleeIDs})
			id := fmt.Sprintf("out:%s:%d:%d:%s", place.ID, call.Line, call.Column, digest(identity))
			p := atlas.Place{ID: id, Kind: atlas.PlaceBoundary, Path: place.Path, LineNo: call.Line, Column: call.Column,
				Parent: place.Parent, TargetIDs: append([]string(nil), place.TargetIDs...), Boundary: &atlas.BoundaryFacts{
					Source: "model", ObjectID: decl.ObjectID, Caller: decl.Name, CallerDoc: decl.Doc, External: call.Name,
					Values: append([]string{}, call.Values...), Direction: atlas.DirectionOut}}
			r.boundaries[id] = &boundaryState{place: p}
		}
	}
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
		if contains(r.places[fileID].TargetIDs, targetID) && contains(r.opts.Graph.Seeds, fileID) {
			in = true
		}
	}
	for _, state := range r.boundaries {
		if r.boxOf[state.place.Parent] != owner.id || !contains(state.place.TargetIDs, targetID) {
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
		if boxID, ok := r.boxOf[seed]; ok && !contains(start, boxID) {
			start = append(start, boxID)
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

// partition names the parts of a set of boxes in one row and lets every box
// choose its part from that closed list. It returns the parts with their
// boxes; empty parts are dropped. Only the final parts assign box zones.
func (r *reader) partition(ctx context.Context, targetID string, tops []*boxState, round int) ([]*zoneState, error) {
	sort.SliceStable(tops, func(i, j int) bool {
		a, b := r.targetFiles(tops[i], targetID), r.targetFiles(tops[j], targetID)
		if a != b {
			return a > b
		}
		return tops[i].id < tops[j].id
	})
	largest := tops
	if len(largest) > lines.WindowRows {
		largest = largest[:lines.WindowRows]
	}
	want := lines.WantZones(len(largest))
	summaries := make([]lines.BoxSummary, 0, len(largest))
	for _, owner := range largest {
		summaries = append(summaries, r.summary(owner, targetID))
	}
	named, err := r.runTableWith(ctx, lines.ZoneNames(want), round, []table.Field{{Name: "question", Value: "names"}, {Name: "want", Value: want}},
		[]table.Row{lines.ZoneNamesRow(targetID, summaries)}, func(answers table.Answers) error {
			distinct := make(map[string]struct{})
			for i := 0; i < want; i++ {
				distinct[strings.ToLower(strings.TrimSpace(lines.PartName(answers[0], i)))] = struct{}{}
			}
			if len(distinct) != want {
				return fmt.Errorf("%d distinct parts, want %d", len(distinct), want)
			}
			return nil
		})
	if err != nil {
		return nil, err
	}
	var titles []string
	var parts []*zoneState
	byKey := make(map[string]*zoneState)
	addPart := func(title string) *zoneState {
		key := strings.ToLower(strings.TrimSpace(title))
		if zone, ok := byKey[key]; ok {
			return zone
		}
		zone := &zoneState{id: atlas.Slug(title), title: strings.TrimSpace(title)}
		byKey[key] = zone
		titles = append(titles, zone.title)
		parts = append(parts, zone)
		return zone
	}
	if named[0].answer != nil {
		for i := 0; i < want; i++ {
			addPart(lines.PartName(named[0].answer, i))
		}
	} else {
		for _, owner := range largest[:want] {
			addPart(owner.title)
		}
	}
	rows := make([]table.Row, 0, len(tops))
	for _, owner := range tops {
		rows = append(rows, lines.ZoneBoxRow(r.summary(owner, targetID)))
	}
	assigned, err := r.runTableWith(ctx, lines.ZoneAssign(titles), round+1, []table.Field{{Name: "question", Value: "assign"}, {Name: "parts", Value: titles}}, rows, nil)
	if err != nil {
		return nil, err
	}
	for i, owner := range tops {
		if answer := assigned[i]; answer.answer != nil {
			zone := addPart(answer.answer["part"])
			zone.boxes = append(zone.boxes, owner.id)
			continue
		}
		if r.dry || named[0].answer == nil {
			if zone, ok := byKey[strings.ToLower(strings.TrimSpace(owner.title))]; ok {
				zone.boxes = append(zone.boxes, owner.id)
			}
		}
	}
	kept := parts[:0]
	var empty []string
	for _, part := range parts {
		if len(part.boxes) > 0 {
			kept = append(kept, part)
		} else {
			empty = append(empty, part.title)
		}
	}
	// A named part no box chose vanishes; the journal says which, so a
	// name the model invented and then abandoned is visible in the run.
	if len(empty) > 0 && r.opts.State != nil {
		r.opts.State(lines.StageZones, "ready", fmt.Sprintf("target %s: %d named parts held no box after assignment and were dropped: %s", targetID, len(empty), strings.Join(empty, ", ")))
	}
	return kept, nil
}

func boxCount(n int) string {
	if n == 1 {
		return "1 box"
	}
	return fmt.Sprintf("%d boxes", n)
}
