package reading

import (
	"context"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/dvordrova/repomap/internal/atlas"
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
		zone.line = fmt.Sprintf("%d boxes: %s", len(zone.boxes), strings.Join(names, ", "))
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

// readSymbols asks one line and a key flag per candidate symbol of every
// file not closed by an accepted open decision. The code keeps at most
// MaxKeysPerFile keys per file, by rank. Without the model the keys are the
// code's ranking.
func (r *reader) readSymbols(ctx context.Context) error {
	def := lines.Symbols()
	var rows []table.Row
	var order []atlas.Place
	var typeRows []table.Row
	var typeOrder []atlas.Place
	closed := 0
	for _, place := range r.opts.Graph.Places {
		if place.Kind != atlas.PlaceSymbol || !place.Symbol.Candidate {
			continue
		}
		file := r.places[place.Parent]
		if file.File == nil || file.File.Generated {
			continue
		}
		// A bare name and a file hypothesis do not establish what the type
		// means. Retain its source entry without asking for an invented gloss.
		if place.Symbol.Decl.Kind == "type" && len(place.Symbol.Members) == 0 && place.Symbol.Decl.Doc == "" {
			continue
		}
		if r.budget {
			// readFiles also closes files beneath accepted closed directories.
			// Missing or refused decisions never authorize closing their symbols.
			if open, decided := r.openFiles[file.ID]; decided && !open {
				closed++
				continue
			}
		}
		if place.Symbol.Decl.Kind == "type" {
			typeRows = append(typeRows, lines.TypeRow(place))
			typeOrder = append(typeOrder, place)
			continue
		}
		fileLine, _ := r.Line(place.Parent)
		rows = append(rows, lines.SymbolRow(place, fileLine))
		order = append(order, place)
	}
	details := []string{fmt.Sprintf("%d candidate symbols, including %d types with their owned declarations", len(rows)+len(typeRows), len(typeRows))}
	if closed > 0 {
		details = append(details, fmt.Sprintf("symbols in closed files left unasked: %d", closed))
	}
	r.opts.Stage(def.Stage, details...)
	answers, err := r.runTable(ctx, def, 1, rows)
	if err != nil {
		return err
	}
	typeAnswers, err := r.runTable(ctx, lines.Types(), 2, typeRows)
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
		r.symbolLine[place.ID] = cell{value: answer.answer["line"], source: answer.source}
		for _, ref := range strings.Fields(answer.answer["outbound"]) {
			n, err := strconv.Atoi(strings.TrimPrefix(ref, "c"))
			if err == nil && n > 0 && n <= len(place.Symbol.Calls) {
				r.outbound[place.ID] = append(r.outbound[place.ID], place.Symbol.Calls[n-1])
			}
		}
		if activation := answer.answer["activation"]; activation != "" && activation != "none" {
			r.operations[place.ID] = [3]string{activation, answer.answer["operation"]}
		}
		if answer.answer["key_symbol"] == "yes" {
			byFile[place.Parent] = append(byFile[place.Parent], marked{id: place.ID, rank: place.Symbol.Rank})
		}
	}
	for fileID, list := range byFile {
		sort.Slice(list, func(i, j int) bool { return list[i].rank < list[j].rank })
		if len(list) > lines.MaxKeysPerFile {
			list = list[:lines.MaxKeysPerFile]
		}
		for _, item := range list {
			r.keys[fileID] = append(r.keys[fileID], item.id)
		}
	}
	r.reportStage(def.Stage)
	return nil
}

// boundaryState is one boundary after its row.
type boundaryState struct {
	place atlas.Place
	line  string
	kind  string
}

// readBoundaries asks one line per boundary; the kind the code knows wins
// over the model's.
func (r *reader) readBoundaries(ctx context.Context) error {
	def := lines.Boundaries()
	r.boundaries = make(map[string]*boundaryState)
	var rows []table.Row
	var order []atlas.Place
	for _, place := range r.opts.Graph.Places {
		if place.Kind != atlas.PlaceBoundary {
			continue
		}
		state := &boundaryState{place: place, line: place.Given, kind: place.Boundary.GivenKind}
		if state.kind == "" {
			state.kind = atlas.BoundaryOther
		}
		r.boundaries[place.ID] = state
		fileLine, _ := r.Line(place.Parent)
		rows = append(rows, lines.BoundaryRow(place, fileLine))
		order = append(order, place)
	}
	r.opts.Stage(def.Stage, fmt.Sprintf("%d integration points", len(rows)))
	answers, err := r.runTable(ctx, def, 1, rows)
	if err != nil {
		return err
	}
	disagreed := 0
	for i, place := range order {
		answer := answers[i]
		if answer.answer == nil {
			continue
		}
		state := r.boundaries[place.ID]
		state.line = answer.answer["line"]
		if place.Boundary.GivenKind == "" {
			state.kind = answer.answer["kind"]
		} else if answer.answer["kind"] != place.Boundary.GivenKind {
			disagreed++
		}
	}
	if disagreed > 0 {
		r.opts.Stage(def.Stage, fmt.Sprintf("kinds the model answered differently from the code: %d, the code's kept", disagreed))
	}
	r.bindInterpretedBoundaries()
	r.reportStage(def.Stage)
	return nil
}

// Interpretation becomes a boundary on the same declaration, not a second
// analyzer. The selected call retains its source line and literal arguments.
func (r *reader) bindInterpretedBoundaries() {
	claimed := make(map[string]bool)
	key := func(path string, line int, direction string) string {
		return fmt.Sprintf("%s:%d:%s", path, line, direction)
	}
	for _, boundary := range r.boundaries {
		claimed[key(boundary.place.Path, boundary.place.LineNo, boundary.place.Boundary.Direction)] = true
	}
	for _, place := range r.opts.Graph.Places {
		if place.Symbol == nil {
			continue
		}
		decl := place.Symbol.Decl
		add := func(id string, line, column int, direction, kind, external, description string, values []string) {
			if line < 1 || claimed[key(place.Path, line, direction)] {
				return
			}
			p := atlas.Place{ID: id, Kind: atlas.PlaceBoundary, Path: place.Path, LineNo: line, Column: column, Parent: place.Parent, TargetIDs: append([]string(nil), place.TargetIDs...), Boundary: &atlas.BoundaryFacts{Source: "model", ObjectID: decl.ObjectID, Caller: decl.Name, CallerDoc: decl.Doc, External: external, Values: values, Direction: direction}}
			r.boundaries[id] = &boundaryState{place: p, line: description, kind: kind}
		}
		if operation := r.operations[place.ID]; operation[0] == "request" {
			add("in:"+place.ID, place.LineNo, decl.Column, atlas.DirectionIn, atlas.BoundaryOther, decl.Name, operation[2], []string{operation[1], operationIdentifier(decl.Name)})
		}
		for i, call := range r.outbound[place.ID] {
			values := append([]string(nil), call.Values...)
			values = append(values, operationIdentifier(call.Name))
			add(fmt.Sprintf("out:%s:%d", place.ID, i), call.Line, 0, atlas.DirectionOut, atlas.BoundarySDK, call.Name, r.symbolLine[place.ID].value, values)
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
	for _, part := range parts {
		if len(part.boxes) > 0 {
			kept = append(kept, part)
		}
	}
	return kept, nil
}
