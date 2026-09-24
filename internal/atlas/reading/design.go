package reading

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"path"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/atlas/table"
	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/modeldiag"
)

//go:embed prompts/design_parts.md
var designPartsPrompt string

//go:embed prompts/design_areas.md
var designAreasPrompt string

// A design unit is what a reader recognizes in the source: one function with
// its package, name, signature and documentation, or one type with its
// package, name and methods. A part and an area are proposed first as a
// closed catalogue; units and parts are then assigned by closed choice.
type designUnit struct {
	id      string   // row identity: the function's or type's symbol place
	members []string // symbol places the unit holds: itself and its methods
	row     table.Row
	calls   map[string]bool // other units it calls
}

type designProposal struct {
	Title   string `json:"title"`
	Purpose string `json:"purpose"`
}

type designProposals struct {
	Groups []designProposal `json:"groups"`
}

// MarshalJSON writes proposals in their wire shape: titles as keys, in order.
func (proposals designProposals) MarshalJSON() ([]byte, error) {
	var out bytes.Buffer
	out.WriteString(`{"groups":{`)
	for i, group := range proposals.Groups {
		if i > 0 {
			out.WriteByte(',')
		}
		title, _ := json.Marshal(group.Title)
		purpose, _ := json.Marshal(group.Purpose)
		out.Write(title)
		out.WriteByte(':')
		out.Write(purpose)
	}
	out.WriteString(`}}`)
	return out.Bytes(), nil
}

// decodeDesignProposals reads {"groups":{"<title>":"<purpose>"}} in order.
// The catalogue is accepted only whole: every later choice is made among
// these groups, so a malformed or repeated entry refuses the answer rather
// than silently shrinking it. An empty object is an explicit abstention.
func decodeDesignProposals(raw []byte) (designProposals, error) {
	var envelope struct {
		Groups json.RawMessage `json:"groups"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil || len(envelope.Groups) == 0 {
		return designProposals{}, fmt.Errorf("design: response needs a groups object")
	}
	decoder := json.NewDecoder(bytes.NewReader(envelope.Groups))
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return designProposals{}, fmt.Errorf("design: groups must map each title to its purpose")
	}
	space := func(r rune) bool { return unicode.IsSpace(r) || r < 0x20 || r == 0x7f }
	clean := func(text string) string { return strings.Join(strings.FieldsFunc(text, space), " ") }
	seen := map[string]bool{}
	result := designProposals{Groups: []designProposal{}}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return designProposals{}, fmt.Errorf("design: groups must map each title to its purpose")
		}
		title := clean(token.(string))
		var purpose string
		if err := decoder.Decode(&purpose); err != nil {
			return designProposals{}, fmt.Errorf("design: the purpose of %q is not a sentence", title)
		}
		purpose = clean(purpose)
		switch {
		case title == "" || purpose == "":
			return designProposals{}, fmt.Errorf("design: group %d needs a title and a purpose", len(result.Groups)+1)
		case seen[strings.ToLower(title)]:
			return designProposals{}, fmt.Errorf("design: the title %q appears twice", title)
		}
		seen[strings.ToLower(title)] = true
		result.Groups = append(result.Groups, designProposal{Title: title, Purpose: purpose})
	}
	return result, nil
}

// designUnits reads the target's declarations as units. Every declaration
// with a native object belongs to exactly one unit; nothing is sampled.
func (r *reader) designUnits(targetID string) []*designUnit {
	if r.designFiles == nil {
		r.designFiles = map[string]string{}
		r.designSubjects = map[string]string{}
	}
	var units []*designUnit
	byType := map[string]*designUnit{} // dir#type -> unit
	unitOf := map[string]*designUnit{} // symbol place -> unit
	var methods []struct {
		id, key, name string
	}
	for _, file := range r.opts.Graph.Places {
		if file.File == nil || !contains(file.TargetIDs, targetID) {
			continue
		}
		dir := path.Dir(file.Path)
		for _, decl := range file.File.Decls {
			id := r.symbolID(file.Path, decl.LineNo, decl.Name)
			objectID := decl.ObjectID
			if objectID == "" {
				if symbol := r.places[id].Symbol; symbol != nil {
					objectID = symbol.Decl.ObjectID
				}
			}
			if objectID == "" {
				continue
			}
			r.designFiles[id] = file.ID
			r.designSubjects[objectID] = id
			if owner, method, isMember := strings.Cut(decl.Name, "."); decl.Kind == "method" && isMember {
				methods = append(methods, struct{ id, key, name string }{id, dir + "#" + owner, method})
				continue
			}
			unit := &designUnit{id: id, members: []string{id}}
			fields := []table.Field{{Name: "package", Value: dir}, {Name: "name", Value: decl.Name}, {Name: "kind", Value: decl.Kind}}
			if decl.Kind == "type" {
				byType[dir+"#"+decl.Name] = unit
			} else if decl.Signature != "" {
				fields = append(fields, table.Field{Name: "signature", Value: decl.Signature})
			}
			if doc := firstSentence(decl.Doc); doc != "" {
				fields = append(fields, table.Field{Name: "documentation", Value: doc})
			}
			unit.row = table.Row{ID: id, Fields: fields}
			units = append(units, unit)
			unitOf[id] = unit
		}
	}
	// A method belongs to its type; one whose type is not declared here is a
	// function of its own.
	for _, method := range methods {
		unit := byType[method.key]
		if unit == nil {
			unit = &designUnit{id: method.id, members: []string{method.id}, row: table.Row{ID: method.id, Fields: []table.Field{
				{Name: "package", Value: strings.Split(method.key, "#")[0]}, {Name: "name", Value: method.name}, {Name: "kind", Value: "method"},
			}}}
			units = append(units, unit)
		}
		unit.members = append(unit.members, method.id)
		unitOf[method.id] = unit
	}
	for _, unit := range units {
		var names []string
		for _, member := range unit.members[1:] {
			names = append(names, r.places[member].Given)
		}
		if len(names) > 0 {
			unit.row.Fields = append(unit.row.Fields, table.Field{Name: "methods", Value: names})
		}
	}
	for _, unit := range units {
		for _, member := range unit.members {
			symbol := r.places[member].Symbol
			if symbol == nil {
				continue
			}
			for _, call := range symbol.Calls {
				for _, callee := range call.CalleeIDs {
					if other := unitOf[callee]; other != nil && other != unit {
						if unit.calls == nil {
							unit.calls = map[string]bool{}
						}
						unit.calls[other.id] = true
					}
				}
			}
		}
	}
	return units
}

// designOverview is the proposal input for parts: each package once, with the
// names it holds. Documents are README and AGENTS only.
func (r *reader) designOverview(targetID string, units []*designUnit) ([]map[string]any, []table.Field) {
	type pkg struct {
		doc          string
		types, funcs []string
	}
	packages := map[string]*pkg{}
	var order []string
	for _, unit := range units {
		values := map[string]any{}
		for _, field := range unit.row.Fields {
			values[field.Name] = field.Value
		}
		dir := values["package"].(string)
		if packages[dir] == nil {
			packages[dir] = &pkg{}
			order = append(order, dir)
		}
		name := values["name"].(string)
		if values["kind"] == "type" {
			packages[dir].types = append(packages[dir].types, name)
		} else {
			packages[dir].funcs = append(packages[dir].funcs, name)
		}
	}
	for _, file := range r.opts.Graph.Places {
		if file.File != nil && file.File.Doc != "" && contains(file.TargetIDs, targetID) {
			if p := packages[path.Dir(file.Path)]; p != nil && p.doc == "" {
				p.doc = firstSentence(file.File.Doc)
			}
		}
	}
	sort.Strings(order)
	overview := make([]map[string]any, 0, len(order))
	for _, dir := range order {
		entry := map[string]any{"package": dir, "types": packages[dir].types, "functions": packages[dir].funcs}
		if packages[dir].doc != "" {
			entry["documentation"] = packages[dir].doc
		}
		overview = append(overview, entry)
	}
	var docs []table.Field
	for _, place := range r.opts.Graph.Places {
		if place.Document == nil || (len(place.TargetIDs) > 0 && !contains(place.TargetIDs, targetID)) {
			continue
		}
		base := strings.ToUpper(path.Base(place.Path))
		if strings.HasPrefix(base, "README") || strings.HasPrefix(base, "AGENTS") {
			docs = append(docs, table.Field{Name: place.Path, Value: place.Document})
		}
	}
	return overview, docs
}

func firstSentence(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	if end := strings.Index(text, ". "); end >= 0 {
		return text[:end+1]
	}
	return text
}

func designCallFor(mode string, input any, documents []table.Field) (llm.Call[designProposals], error) {
	raw, err := json.Marshal(struct {
		Task      string        `json:"task"`
		Mode      string        `json:"mode"`
		Input     any           `json:"input"`
		Documents []table.Field `json:"author_context,omitempty"`
	}{"repomap.atlas.design.v4", mode, input, documents})
	if err != nil {
		return llm.Call[designProposals]{}, err
	}
	prompt := designPartsPrompt
	if mode == "areas" {
		prompt = designAreasPrompt
	}
	return llm.Call[designProposals]{
		State: []byte("repomap.atlas.design.v4"),
		Prompt: llm.Prompt{System: prompt, User: string(raw), ResponseFormatJSON: true,
			ResponseExample: `{"groups":{"Move search":"Chooses a move by exploring legal continuations.","Board state":"Holds the position and applies moves."}}`, NoResponseAdjunct: true},
		Limits:         llm.Limits{MaxRequestBytes: llm.SemanticRecordByteLimit, MaxResponseBytes: llm.ProviderResponseByteLimit, MaxOutputTokens: llm.DefaultMaxOutputTokens},
		DecodeValidate: decodeDesignProposals,
	}, nil
}

// propose asks for one closed catalogue of parts or areas. A refused answer
// leaves nothing to assign; the declarations stay source inventory.
func (r *reader) propose(ctx context.Context, mode string, input any, documents []table.Field) ([]designProposal, error) {
	if r.dry {
		return nil, nil
	}
	build := func(input []any) (llm.Call[designProposals], error) {
		return designCallFor(mode, input[0], documents)
	}
	results, err := llm.ExecuteAdaptiveJSONEachResults(ctx, debugdump.BindStage(r.opts.Executor, lines.StageZones), r.opts.Provider, [][]any{{input}}, build,
		func([]any) ([]any, []any, bool) { return nil, nil, false })
	if err != nil {
		return nil, err
	}
	use := r.use(lines.StageZones)
	var proposals []designProposal
	for _, result := range results {
		r.designRound++
		window := table.Window{Stage: lines.StageZones, Round: r.designRound}
		responseRef := path.Join(atlas.TablesDir, r.windowFileName(window, "response.ref.json"))
		call, err := build(result.Item)
		if err != nil {
			return nil, err
		}
		use.Windows++
		if result.Outcome.Cached {
			use.Cached++
		} else {
			use.Live++
		}
		for _, item := range []struct {
			name string
			data []byte
		}{
			{"prompt.md", []byte(call.Prompt.System)}, {"input.json", []byte(call.Prompt.User)},
			{"request.json", result.Outcome.Request}, {"response.json", result.Outcome.Response},
		} {
			if len(item.data) == 0 {
				continue
			}
			if err := r.writeWindowFile(window, item.name, item.data); err != nil {
				return nil, err
			}
		}
		if result.Err != nil {
			use.Rejected++
			r.rejected = append(r.rejected, modeldiag.Row{Stage: lines.StageZones, Kind: "window_rejected", Count: 1, Reason: result.Err.Error(), ResponseRef: responseRef})
			fmt.Fprintf(&r.tables, "## %s · %s · round %d\n\nProposal unavailable: %s\n\n", lines.StageZones, mode, r.designRound, result.Err)
			continue
		}
		raw, err := json.MarshalIndent(result.Outcome.Value, "", "  ")
		if err != nil {
			return nil, err
		}
		if err := r.writeWindowFile(window, "result.json", raw); err != nil {
			return nil, err
		}
		fmt.Fprintf(&r.tables, "## %s · %s · round %d\n\n%s\n\n", lines.StageZones, mode, r.designRound, raw)
		proposals = append(proposals, result.Outcome.Value.Groups...)
	}
	return proposals, nil
}

// catalogue renders proposals as the closed context of an assignment table.
func catalogue(name string, proposals []designProposal) []table.Field {
	entries := make([]map[string]any, len(proposals))
	options := make([]string, 0, len(proposals)+1)
	for i, proposal := range proposals {
		ref := fmt.Sprintf("c%d", i+1)
		entries[i] = map[string]any{"ref": ref, "title": proposal.Title, "purpose": proposal.Purpose}
		options = append(options, ref)
	}
	options = append(options, lines.ZoneNone)
	return []table.Field{{Name: name + "s", Value: entries}, {Name: name + "_options", Value: options}}
}

func chosen(answer rowAnswer, column string, proposals []designProposal) int {
	if answer.answer == nil {
		return -1
	}
	var position int
	if _, err := fmt.Sscanf(answer.answer[column], "c%d", &position); err != nil || position < 1 || position > len(proposals) {
		return -1
	}
	return position - 1
}

func (r *reader) readDesign(ctx context.Context) error {
	r.started[lines.StageZones] = time.Now()
	r.opts.Stage(lines.StageZones, "proposing parts and areas, then assigning functions, types and parts to them")
	r.boxes = map[string]*boxState{}
	r.designBoxOf = map[string]map[string]string{}
	r.zones = map[string][]*zoneState{}
	for _, target := range r.opts.Targets {
		units := r.designUnits(target.ID)
		overview, docs := r.designOverview(target.ID, units)
		parts, err := r.propose(ctx, "parts", overview, docs)
		if err != nil {
			return err
		}
		membership := map[string]string{}
		r.designBoxOf[target.ID] = membership
		partOf := make([]int, len(units))
		for i := range partOf {
			partOf[i] = -1
		}
		if len(parts) > 0 && len(units) > 0 {
			unitByID := map[string]*designUnit{}
			rows := make([]table.Row, len(units))
			for i, unit := range units {
				unitByID[unit.id] = unit
				rows[i] = unit.row
				var calls []string
				for other := range unit.calls {
					calls = append(calls, r.places[other].Given)
				}
				sort.Strings(calls)
				if len(calls) > 0 {
					rows[i].Fields = append(append([]table.Field(nil), rows[i].Fields...), table.Field{Name: "calls", Value: calls})
				}
			}
			answers, err := r.runTableWith(ctx, lines.ZoneParts(), 1, catalogue("part", parts), rows, nil)
			if err != nil {
				return err
			}
			for i := range units {
				partOf[i] = chosen(answers[i], "part", parts)
			}
		}
		// A part is drawn only when at least one unit chose it.
		boxOfPart := map[int]string{}
		var drawn []int
		for position := range parts {
			var ids []string
			for i, unit := range units {
				if partOf[i] == position {
					ids = append(ids, unit.members...)
				}
			}
			if len(ids) == 0 {
				continue
			}
			r.addDesignBox(target.ID, designItem{Name: parts[position].Title, Purpose: parts[position].Purpose, IDs: ids}, membership)
			boxOfPart[position] = membership[ids[0]]
			drawn = append(drawn, position)
		}
		// Missing decisions remain source inventory, not a directory-derived
		// architectural claim. Every unassigned declaration stays available.
		for _, file := range r.opts.Graph.Places {
			if file.File == nil || !contains(file.TargetIDs, target.ID) {
				continue
			}
			var ids []string
			for _, decl := range file.File.Decls {
				id := r.symbolID(file.Path, decl.LineNo, decl.Name)
				if membership[id] == "" {
					ids = append(ids, id)
				}
			}
			if len(ids) > 0 || len(file.File.Decls) == 0 {
				if len(ids) == 0 {
					ids = []string{file.ID}
				}
				r.addDesignBox(target.ID, designItem{Name: file.Path, Purpose: "Declarations from this source file.", IDs: ids}, membership)
				r.boxes[membership[ids[0]]].inventory = true
			}
			// A file endpoint is unambiguous only when all its declarations
			// actually belong to one part. Never choose an arbitrary owner.
			owners := map[string]bool{}
			for _, decl := range file.File.Decls {
				owners[membership[r.symbolID(file.Path, decl.LineNo, decl.Name)]] = true
			}
			if len(owners) == 1 {
				for id := range owners {
					membership[file.ID] = id
				}
			}
		}
		if len(drawn) == 0 {
			continue
		}
		if err := r.readAreas(ctx, target.ID, units, partOf, parts, drawn, boxOfPart); err != nil {
			return err
		}
	}
	r.reportStage(lines.StageZones)
	r.reportStage(lines.StageZoneParts)
	r.reportStage(lines.StageZoneAreas)
	return nil
}

// readAreas proposes areas over the drawn parts and assigns each part to one.
func (r *reader) readAreas(ctx context.Context, targetID string, units []*designUnit, partOf []int, parts []designProposal, drawn []int, boxOfPart map[int]string) error {
	unitPart := map[string]int{}
	for i, unit := range units {
		unitPart[unit.id] = partOf[i]
	}
	calls := map[int]map[int]bool{}
	for i, unit := range units {
		for other := range unit.calls {
			if to, ok := unitPart[other]; ok && to >= 0 && partOf[i] >= 0 && to != partOf[i] {
				if calls[partOf[i]] == nil {
					calls[partOf[i]] = map[int]bool{}
				}
				calls[partOf[i]][to] = true
			}
		}
	}
	input := make([]map[string]any, 0, len(drawn))
	rows := make([]table.Row, 0, len(drawn))
	for _, position := range drawn {
		var calling []string
		for to := range calls[position] {
			calling = append(calling, parts[to].Title)
		}
		sort.Strings(calling)
		input = append(input, map[string]any{"title": parts[position].Title, "purpose": parts[position].Purpose})
		fields := []table.Field{{Name: "title", Value: parts[position].Title}, {Name: "purpose", Value: parts[position].Purpose}}
		if len(calling) > 0 {
			fields = append(fields, table.Field{Name: "calls", Value: calling})
		}
		rows = append(rows, table.Row{ID: boxOfPart[position], Fields: fields})
	}
	areas, err := r.propose(ctx, "areas", input, nil)
	if err != nil || len(areas) == 0 {
		return err
	}
	answers, err := r.runTableWith(ctx, lines.ZoneAreas(), 1, catalogue("area", areas), rows, nil)
	if err != nil {
		return err
	}
	zones := make([]*zoneState, len(areas))
	for i, position := range drawn {
		area := chosen(answers[i], "area", areas)
		if area < 0 {
			continue
		}
		if zones[area] == nil {
			zones[area] = &zoneState{id: r.compactID("z", &r.nextZone), title: areas[area].Title, line: areas[area].Purpose}
			r.zones[targetID] = append(r.zones[targetID], zones[area])
		}
		box := boxOfPart[position]
		zones[area].boxes = append(zones[area].boxes, box)
		r.boxes[box].zoneID[targetID] = zones[area].id
	}
	return nil
}

// designItem is what addDesignBox draws: a titled set of symbol places.
type designItem struct {
	Name    string
	Purpose string
	IDs     []string
}

func (r *reader) addDesignBox(targetID string, part designItem, membership map[string]string) {
	id := r.compactID("p", &r.nextPart)
	box := &boxState{id: id, targetID: targetID, title: part.Name, line: part.Purpose, open: true, dir: ".", zoneID: map[string]string{}, symbols: map[string]bool{}}
	for _, member := range part.IDs {
		membership[member] = id
		box.symbols[member] = true
		fileID := r.places[member].Parent
		if r.places[member].File != nil {
			fileID = member
		}
		// Non-candidate declarations still have their indexed source file.
		if fileID == "" {
			fileID = r.designFiles[member]
		}
		if fileID != "" && !slices.Contains(box.files, fileID) {
			box.files = append(box.files, fileID)
		}
	}
	sort.Strings(box.files)
	if len(box.files) > 0 {
		box.dir = path.Dir(r.places[box.files[0]].Path)
	}
	r.boxes[id] = box
}

func (r *reader) boxFor(targetID, placeID string) string {
	if owners, ok := r.designBoxOf[targetID]; ok {
		return owners[placeID]
	}
	return r.boxOfPlace(placeID)
}

func (r *reader) boundaryBox(targetID string, place atlas.Place) string {
	if place.Boundary != nil {
		if id := r.boxFor(targetID, place.Boundary.SubjectID); id != "" {
			return id
		}
		if symbol := r.designSubjects[place.Boundary.ObjectID]; symbol != "" {
			if id := r.boxFor(targetID, symbol); id != "" {
				return id
			}
		}
	}
	return r.boxFor(targetID, place.Parent)
}
