package reading

import (
	"context"
	"crypto/sha256"
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

//go:embed prompts/design.md
var designPrompt string

// A design item is a declaration, or an accepted aggregate in a reduction.
// Identity and membership remain local; the provider chooses request-local refs.
type designItem struct {
	Ref          string       `json:"ref"`
	Path         string       `json:"path,omitempty"`
	Name         string       `json:"name"`
	Kind         string       `json:"kind"`
	Signature    string       `json:"signature,omitempty"`
	Doc          string       `json:"author_documentation,omitempty"`
	Purpose      string       `json:"model_interpretation,omitempty"`
	Activation   string       `json:"model_activation,omitempty"`
	Calls        []designCall `json:"observations,omitempty"`
	Declarations []string     `json:"declarations,omitempty"`
	IDs          []string     `json:"-"`
}

type designCall struct {
	Kind       string   `json:"kind"`
	Name       string   `json:"name"`
	Line       int      `json:"line,omitempty"`
	Resolution string   `json:"resolution,omitempty"`
	To         []string `json:"to,omitempty"`
	Values     []string `json:"values,omitempty"`
}

type designGroup struct {
	Title   string   `json:"title"`
	Purpose string   `json:"purpose"`
	Members []string `json:"members"`
}

type designResult struct {
	Groups []designGroup `json:"groups"`
	Notes  []designNote  `json:"notes,omitempty"`
}

type designNote struct {
	Kind   string `json:"kind"`
	Reason string `json:"reason"`
}

func decodeDesignGroup(raw []byte) (designGroup, string) {
	var group designGroup
	if json.Unmarshal(raw, &group) != nil {
		return group, "malformed group"
	}
	// Captions follow the existing table text convention: formatting does
	// not change a membership decision or discard an otherwise valid group.
	space := func(r rune) bool { return unicode.IsSpace(r) || r < 0x20 || r == 0x7f }
	group.Title = strings.Join(strings.FieldsFunc(group.Title, space), " ")
	group.Purpose = strings.Join(strings.FieldsFunc(group.Purpose, space), " ")
	if group.Title == "" || group.Purpose == "" {
		return group, "missing title/purpose"
	}
	return group, ""
}

func decodeDesign(raw []byte, items []designItem, mode string) (designResult, error) {
	var envelope struct {
		Groups []json.RawMessage `json:"groups"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil || envelope.Groups == nil {
		return designResult{}, fmt.Errorf("design: response needs a groups array")
	}
	known, used := map[string]bool{}, map[string]bool{}
	for _, item := range items {
		known[item.Ref] = true
	}
	// Conflicting groups are coupled. Refuse both rather than let response
	// order decide which responsibility acquires a declaration.
	assignments := map[string]int{}
	for _, rawGroup := range envelope.Groups {
		group, reason := decodeDesignGroup(rawGroup)
		if reason != "" {
			continue
		}
		members := map[string]bool{}
		for _, ref := range group.Members {
			if known[ref] {
				members[ref] = true
			}
		}
		for ref := range members {
			assignments[ref]++
		}
	}
	result := designResult{Groups: []designGroup{}}
	for i, rawGroup := range envelope.Groups {
		group, reason := decodeDesignGroup(rawGroup)
		var members []string
		for _, ref := range group.Members {
			if !known[ref] {
				result.Notes = append(result.Notes, designNote{Kind: "unknown_member", Reason: fmt.Sprintf("group %d: unknown member %q discarded", i+1, ref)})
				continue
			}
			if assignments[ref] > 1 {
				reason = "member assigned by conflicting groups"
			}
			if !slices.Contains(members, ref) {
				members = append(members, ref)
			}
		}
		if len(members) == 0 {
			reason = "no known members"
		}
		if reason != "" {
			result.Notes = append(result.Notes, designNote{Kind: "group_rejected", Reason: fmt.Sprintf("group %d refused: %s", i+1, reason)})
			continue
		}
		for _, ref := range members {
			used[ref] = true
		}
		group.Members = members
		result.Groups = append(result.Groups, group)
	}
	if mode != "areas" {
		for _, item := range items {
			if !used[item.Ref] {
				result.Notes = append(result.Notes, designNote{Kind: "ungrouped_input", Reason: "no accepted membership for " + item.Ref})
			}
		}
	}
	if len(result.Groups) == 0 && len(envelope.Groups) != 0 {
		var reasons []string
		for _, note := range result.Notes {
			reasons = append(reasons, note.Reason)
		}
		return designResult{}, fmt.Errorf("design: no valid grouping decision: %s", strings.Join(reasons, "; "))
	}
	return result, nil
}

func designCallFor(items []designItem, documents []table.Field, mode string) (llm.Call[designResult], error) {
	// A provider-sized window has its own closed catalogue. Calls to items
	// outside that window retain their native names and sites; reduction can
	// restore their local endpoints when those parts are reviewed together.
	items = append([]designItem(nil), items...)
	known := map[string]bool{}
	for _, item := range items {
		known[item.Ref] = true
	}
	for i := range items {
		items[i].Calls = append([]designCall(nil), items[i].Calls...)
		for j := range items[i].Calls {
			call := &items[i].Calls[j]
			var refs []string
			for _, ref := range call.To {
				if known[ref] {
					refs = append(refs, ref)
				}
			}
			call.To = refs
		}
	}
	raw, err := json.Marshal(struct {
		Task      string        `json:"task"`
		Mode      string        `json:"mode"`
		Items     []designItem  `json:"items"`
		Documents []table.Field `json:"author_context,omitempty"`
	}{"repomap.atlas.design.v1", mode, items, documents})
	if err != nil {
		return llm.Call[designResult]{}, err
	}
	return llm.Call[designResult]{
		State: []byte("repomap.atlas.design.v1"),
		Prompt: llm.Prompt{System: designPrompt, User: string(raw), ResponseFormatJSON: true,
			ResponseExample: `{"groups":[{"title":"Move search","purpose":"Chooses a move by exploring legal continuations.","members":["r1","r2"]}]}`, NoResponseAdjunct: true},
		Limits:         llm.Limits{MaxRequestBytes: llm.SemanticRecordByteLimit, MaxResponseBytes: llm.ProviderResponseByteLimit, MaxOutputTokens: llm.DefaultMaxOutputTokens},
		DecodeValidate: func(raw []byte) (designResult, error) { return decodeDesign(raw, items, mode) },
	}, nil
}

// Complete aggregates split only for the actual provider envelope. Accepted
// siblings survive. Cross-window parts are subsequently reviewed together.
func (r *reader) askDesign(ctx context.Context, items []designItem, documents []table.Field, mode string) ([]designItem, int, error) {
	if len(items) == 0 || r.dry {
		return nil, 0, nil
	}
	build := func(items []designItem) (llm.Call[designResult], error) { return designCallFor(items, documents, mode) }
	results, err := llm.ExecuteAdaptiveJSONEachResults(ctx, debugdump.BindStage(r.opts.Executor, lines.StageZones), r.opts.Provider, [][]designItem{items}, build,
		func(items []designItem) ([]designItem, []designItem, bool) {
			if len(items) < 2 {
				return nil, nil, false
			}
			mid := len(items) / 2
			return items[:mid], items[mid:], true
		})
	if err != nil {
		return nil, 0, err
	}
	var accepted []designItem
	use := r.use(lines.StageZones)
	for _, result := range results {
		r.designRound++
		window := table.Window{Stage: lines.StageZones, Round: r.designRound}
		responseRef := path.Join(atlas.TablesDir, r.windowFileName(window, "response.ref.json"))
		call, err := build(result.Item)
		if err != nil {
			return nil, 0, err
		}
		use.Rows += len(result.Item)
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
			if len(item.data) > 0 {
				if err := r.writeWindowFile(window, item.name, item.data); err != nil {
					return nil, 0, err
				}
			}
		}
		if result.Err != nil {
			use.Rejected++
			use.Given += len(result.Item)
			r.rejected = append(r.rejected, modeldiag.Row{Stage: lines.StageZones, Kind: "window_rejected", Count: len(result.Item), Reason: result.Err.Error(), ResponseRef: responseRef})
			fmt.Fprintf(&r.tables, "## %s · %s · round %d\n\nGrouping unavailable: %s\n\n", lines.StageZones, mode, r.designRound, result.Err)
			continue
		}
		value := result.Outcome.Value
		refused := false
		for _, note := range value.Notes {
			refused = refused || note.Kind == "group_rejected"
			r.rejected = append(r.rejected, modeldiag.Row{Stage: lines.StageZones, Kind: note.Kind, Count: 1, Reason: note.Reason, ResponseRef: responseRef})
		}
		if refused {
			use.Rejected++
		}
		raw, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return nil, 0, err
		}
		if err := r.writeWindowFile(window, "result.json", raw); err != nil {
			return nil, 0, err
		}
		fmt.Fprintf(&r.tables, "## %s · %s · round %d\n\n%s\n\n", lines.StageZones, mode, r.designRound, raw)
		byRef := map[string]designItem{}
		for _, item := range result.Item {
			byRef[item.Ref] = item
		}
		for _, group := range value.Groups {
			item := designItem{Name: group.Title, Purpose: group.Purpose, Kind: "part"}
			for _, ref := range group.Members {
				member := byRef[ref]
				item.IDs = append(item.IDs, member.IDs...)
				item.Calls = append(item.Calls, member.Calls...)
				if len(member.Declarations) > 0 {
					item.Declarations = append(item.Declarations, member.Declarations...)
				} else {
					item.Declarations = append(item.Declarations, member.Name)
				}
			}
			accepted = append(accepted, item)
		}
	}
	return accepted, len(results), nil
}

func (r *reader) designInputs(targetID string) ([]designItem, []table.Field) {
	if r.designFiles == nil {
		r.designFiles = map[string]string{}
		r.designSubjects = map[string]string{}
	}
	var items []designItem
	refs := map[string]string{}
	for _, file := range r.opts.Graph.Places {
		if file.File == nil || !contains(file.TargetIDs, targetID) {
			continue
		}
		for _, decl := range file.File.Decls {
			id := atlas.SymbolID(file.Path, decl.LineNo, decl.Name)
			r.designFiles[id] = file.ID
			if decl.ObjectID != "" {
				r.designSubjects[decl.ObjectID] = id
			}
			item := designItem{Ref: fmt.Sprintf("r%d", len(items)+1), Path: file.Path, Name: decl.Name, Kind: decl.Kind,
				Signature: decl.Signature, Doc: decl.Doc, Purpose: r.symbolLine[id].value, Activation: r.operations[id][0], IDs: []string{id}}
			refs[id] = item.Ref
			items = append(items, item)
		}
	}
	for i := range items {
		if symbol := r.places[items[i].IDs[0]].Symbol; symbol != nil {
			for _, call := range symbol.Calls {
				row := designCall{Kind: call.Kind, Name: call.Name, Line: call.Line, Resolution: call.Resolution, Values: call.Values}
				for _, id := range call.CalleeIDs {
					if ref := refs[id]; ref != "" {
						row.To = append(row.To, ref)
					}
				}
				items[i].Calls = append(items[i].Calls, row)
			}
			for _, binding := range symbol.Bindings {
				items[i].Calls = append(items[i].Calls, designCall{Kind: "binding", Name: binding.From})
			}
		}
	}
	var docs []table.Field
	for _, place := range r.opts.Graph.Places {
		if place.Document != nil && len(place.TargetIDs) == 0 {
			docs = append(docs, table.Field{Name: place.Path, Value: place.Document})
			continue
		}
		if !contains(place.TargetIDs, targetID) {
			continue
		}
		if place.Document != nil {
			docs = append(docs, table.Field{Name: place.Path, Value: place.Document})
		}
		if place.File != nil && place.File.Doc != "" {
			docs = append(docs, table.Field{Name: place.Path, Value: place.File.Doc})
		}
	}
	return items, docs
}

func designID(targetID string, members []string) string {
	ids := append([]string(nil), members...)
	sort.Strings(ids)
	return fmt.Sprintf("design:%x", sha256.Sum256([]byte(targetID+"\x00"+strings.Join(ids, "\x00"))))
}

func (r *reader) readDesign(ctx context.Context) error {
	r.started[lines.StageZones] = time.Now()
	r.opts.Stage(lines.StageZones, "reading responsibilities and collaboration from declarations, calls and documentation")
	r.boxes = map[string]*boxState{}
	r.designBoxOf = map[string]map[string]string{}
	r.zones = map[string][]*zoneState{}
	for _, target := range r.opts.Targets {
		items, docs := r.designInputs(target.ID)
		parts, windows, err := r.askDesign(ctx, items, docs, "parts")
		if err != nil {
			return err
		}
		// A provider split must not become an architecture boundary. Review
		// the complete accepted parts together until no further merge occurs.
		for windows > 1 && len(parts) > 1 {
			parts = designPartContext(items, parts)
			merged, count, err := r.askDesign(ctx, parts, nil, "merge")
			if err != nil {
				return err
			}
			merged = preserveDesignParts(parts, merged)
			if len(merged) >= len(parts) {
				break
			}
			parts, windows = merged, count
		}
		membership := map[string]string{}
		r.designBoxOf[target.ID] = membership
		for _, part := range parts {
			r.addDesignBox(target.ID, part, membership)
		}
		// Missing decisions remain source inventory, not a directory-derived
		// architectural claim. Every unassigned declaration stays available.
		for _, file := range r.opts.Graph.Places {
			if file.File == nil || !contains(file.TargetIDs, target.ID) {
				continue
			}
			var ids []string
			for _, decl := range file.File.Decls {
				id := atlas.SymbolID(file.Path, decl.LineNo, decl.Name)
				if membership[id] == "" {
					ids = append(ids, id)
				}
			}
			if len(ids) > 0 || len(file.File.Decls) == 0 {
				if len(ids) == 0 {
					ids = []string{file.ID}
				}
				r.addDesignBox(target.ID, designItem{Name: file.Path, Purpose: "Code awaiting architecture grouping.", IDs: ids}, membership)
			}
			// A file endpoint is unambiguous only when all its declarations
			// actually belong to one part. Never choose an arbitrary owner.
			owners := map[string]bool{}
			for _, decl := range file.File.Decls {
				owners[membership[atlas.SymbolID(file.Path, decl.LineNo, decl.Name)]] = true
			}
			if len(owners) == 1 {
				for id := range owners {
					membership[file.ID] = id
				}
			}
		}
		var areaItems []designItem
		for _, part := range designPartContext(items, parts) {
			part.IDs = []string{designID(target.ID, part.IDs)}
			areaItems = append(areaItems, part)
		}
		areas, _, err := r.askDesign(ctx, areaItems, nil, "areas")
		if err != nil {
			return err
		}
		for _, area := range areas {
			zone := &zoneState{id: designID(target.ID, area.IDs), title: area.Name, line: area.Purpose, boxes: area.IDs}
			r.zones[target.ID] = append(r.zones[target.ID], zone)
			for _, id := range area.IDs {
				r.boxes[id].zoneID[target.ID] = zone.id
			}
		}
	}
	r.reportStage(lines.StageZones)
	return nil
}

// Reduction retains the native observations with endpoints rebound to its
// closed part catalogue. Descriptions and file counts alone do not explain
// how the parts collaborate.
func designPartContext(original, parts []designItem) []designItem {
	result := append([]designItem(nil), parts...)
	partOf := map[string]string{}
	for i := range result {
		result[i].Ref = fmt.Sprintf("r%d", i+1)
		result[i].Calls = nil
		for _, id := range result[i].IDs {
			partOf[id] = result[i].Ref
		}
	}
	refPart := map[string]string{}
	originalByID := map[string]designItem{}
	for _, item := range original {
		if len(item.IDs) > 0 {
			refPart[item.Ref] = partOf[item.IDs[0]]
			originalByID[item.IDs[0]] = item
		}
	}
	for i := range result {
		for _, id := range result[i].IDs {
			item := originalByID[id]
			for _, call := range item.Calls {
				copy := call
				copy.To = nil
				for _, ref := range call.To {
					if to := refPart[ref]; to != "" && !slices.Contains(copy.To, to) {
						copy.To = append(copy.To, to)
					}
				}
				result[i].Calls = append(result[i].Calls, copy)
			}
		}
	}
	return result
}

func preserveDesignParts(original, accepted []designItem) []designItem {
	used := map[string]bool{}
	for _, part := range accepted {
		for _, id := range part.IDs {
			used[id] = true
		}
	}
	for _, part := range original {
		if len(part.IDs) > 0 && !used[part.IDs[0]] {
			accepted = append(accepted, part)
		}
	}
	return accepted
}

func (r *reader) addDesignBox(targetID string, part designItem, membership map[string]string) {
	id := designID(targetID, part.IDs)
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
