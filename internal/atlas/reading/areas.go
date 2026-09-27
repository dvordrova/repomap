package reading

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/atlas/table"
	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/modeldiag"
)

// minAreaParts is how many drawn parts that are not test code a target needs
// before grouping them into areas is a decision.
const minAreaParts = 3

// zoneState is one area of a target and the parts it holds.
type zoneState struct {
	id    string
	title string
	line  string
	boxes []string
}

// areaPartRow is one part as the areas request shows it.
type areaPartRow struct {
	Ref         string   `json:"ref"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Dirs        []string `json:"dirs"`
	Units       int      `json:"units"`
}

type areasInput struct {
	Task  string        `json:"task"`
	Parts []areaPartRow `json:"parts"`
	Calls []string      `json:"calls,omitempty"`
}

// areasGroup is one element of an areas answer, read on its own.
type areasGroup struct {
	Name      string
	Parts     []string
	Malformed bool
}

type areasAnswer struct {
	Areas []areasGroup
}

func (answer areasAnswer) MarshalJSON() ([]byte, error) {
	type area struct {
		Name  string   `json:"name"`
		Parts []string `json:"parts"`
	}
	areas := make([]area, 0, len(answer.Areas))
	for _, a := range answer.Areas {
		areas = append(areas, area{Name: a.Name, Parts: a.Parts})
	}
	return json.Marshal(struct {
		Areas []area `json:"areas"`
	}{areas})
}

// decodeAreas reads {"areas":[{"name":…,"parts":[…]}]} over the part refs
// one request listed; "parts" may also be one string of refs. An empty list
// says every part stands alone, and each area is read on its own. An answer
// is refused whole when it is not JSON, has no areas list, or lists areas
// none of which holds a listed part (every ref unknown, such as a part's
// name, or every area without a name).
func decodeAreas(raw []byte, listed []string) (areasAnswer, error) {
	var envelope struct {
		Areas json.RawMessage `json:"areas"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return areasAnswer{}, fmt.Errorf("areas: the answer is not a JSON object")
	}
	var elements []json.RawMessage
	if len(envelope.Areas) == 0 || json.Unmarshal(envelope.Areas, &elements) != nil {
		return areasAnswer{}, fmt.Errorf("areas: the answer has no areas list")
	}
	answer := areasAnswer{Areas: make([]areasGroup, 0, len(elements))}
	for _, element := range elements {
		var area struct {
			Name  string          `json:"name"`
			Parts json.RawMessage `json:"parts"`
		}
		if err := json.Unmarshal(element, &area); err != nil {
			answer.Areas = append(answer.Areas, areasGroup{Malformed: true})
			continue
		}
		parts, ok := refList(area.Parts)
		if !ok {
			answer.Areas = append(answer.Areas, areasGroup{Malformed: true})
			continue
		}
		answer.Areas = append(answer.Areas, areasGroup{Name: cleanText(area.Name), Parts: parts})
	}
	if len(answer.Areas) > 0 && !validateAreas(answer, listed).holds {
		return areasAnswer{}, fmt.Errorf("areas: no area holds a listed part")
	}
	return answer, nil
}

// validAreas is a validated areas answer: a closed split of the listed
// parts. A part in two areas or in none stands alone; an area of fewer than
// two parts is that part, not an area. An area given twice, with the same
// name ignoring case and the same set of listed parts, is one area.
type validAreas struct {
	names   []string
	parts   [][]string
	notes   []string
	unknown []string
	// holds says a named area holds a listed part, even one that then
	// stands alone.
	holds bool
}

func validateAreas(answer areasAnswer, listed []string) validAreas {
	known := make(map[string]bool, len(listed))
	for _, ref := range listed {
		known[ref] = true
	}
	holders := map[string][]int{}
	var names []string
	var sets [][]string // each read area's listed parts, sorted
	var result validAreas
	for position, area := range answer.Areas {
		if area.Malformed || area.Name == "" {
			result.notes = append(result.notes, fmt.Sprintf("area %d has no name or no list of parts", position+1))
			continue
		}
		var seen []string
		for _, ref := range area.Parts {
			ref = strings.TrimSpace(ref)
			switch {
			case !known[ref]:
				result.unknown = appendUnique(result.unknown, ref)
			case !slices.Contains(seen, ref):
				seen = append(seen, ref)
			}
		}
		set := slices.Clone(seen)
		sort.Strings(set)
		if earlier := sameGroup(names, sets, area.Name, set); earlier >= 0 {
			result.notes = append(result.notes, fmt.Sprintf("area %d repeats area %q, drawn once", position+1, names[earlier]))
			continue
		}
		index := len(names)
		names = append(names, area.Name)
		sets = append(sets, set)
		for _, ref := range seen {
			holders[ref] = append(holders[ref], index)
			result.holds = true
		}
	}
	members := make([][]string, len(names))
	for _, ref := range listed {
		switch len(holders[ref]) {
		case 1:
			members[holders[ref][0]] = append(members[holders[ref][0]], ref)
		case 0:
		default:
			result.notes = append(result.notes, fmt.Sprintf("%s is in two areas and stands alone", ref))
		}
	}
	for i, name := range names {
		if len(members[i]) < 2 {
			if len(members[i]) == 1 {
				result.notes = append(result.notes, fmt.Sprintf("area %q holds one part, drawn as that part", name))
			}
			continue
		}
		result.names = append(result.names, name)
		result.parts = append(result.parts, members[i])
	}
	return result
}

func areasCall(input areasInput) (llm.Call[areasAnswer], error) {
	raw, err := json.Marshal(input)
	if err != nil {
		return llm.Call[areasAnswer]{}, err
	}
	listed := make([]string, len(input.Parts))
	for i, part := range input.Parts {
		listed[i] = part.Ref
	}
	return llm.Call[areasAnswer]{
		State: []byte(designAreasTask),
		Prompt: llm.Prompt{System: designAreasPrompt, User: string(raw), ResponseFormatJSON: true, NoResponseAdjunct: true,
			ResponseExample: `{"areas":[{"name":"Game rules","parts":["p1","p4"]}]}`},
		Limits:         llm.Limits{MaxRequestBytes: llm.SemanticRecordByteLimit, MaxResponseBytes: llm.ProviderResponseByteLimit, MaxOutputTokens: designOutputTokens(len(input.Parts))},
		DecodeValidate: func(raw []byte) (areasAnswer, error) { return decodeAreas(raw, listed) },
	}, nil
}

// areasInput lists a target's drawn parts that are not test code, with the
// exact call sites between them, once per distinct pair of parts.
func (r *reader) areasInput(targetID string, parts []*boxState) areasInput {
	input := areasInput{Task: designAreasTask}
	listed := map[string]bool{}
	for _, box := range parts {
		listed[box.id] = true
		var dirs []string
		for _, ref := range box.sources {
			dirs = appendUnique(dirs, path.Dir(r.places[ref].Path))
		}
		sort.Strings(dirs)
		input.Parts = append(input.Parts, areaPartRow{Ref: box.id, Name: box.title, Description: box.line, Dirs: dirs, Units: box.units})
	}
	counts := map[[2]string]int{}
	membership := r.designBoxOf[targetID]
	for _, box := range parts {
		ids := make([]string, 0, len(box.symbols))
		for id := range box.symbols {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			symbol := r.places[id].Symbol
			if symbol == nil {
				continue
			}
			for _, call := range symbol.Calls {
				if call.Kind != "calls" || call.Resolution != "exact" {
					continue
				}
				var reached []string
				for _, callee := range call.CalleeIDs {
					if to := membership[callee]; listed[to] && to != box.id && !slices.Contains(reached, to) {
						reached = append(reached, to)
					}
				}
				for _, to := range reached {
					counts[[2]string{box.id, to}]++
				}
			}
		}
	}
	input.Calls = pairCounts(counts, listed)
	return input
}

// readAreas groups each target's described parts into areas by one closed
// split, then describes each area from its parts. Areas take their compact
// IDs in target order. Without a model, or with fewer than three parts, no
// area is drawn; a refused answer draws none and is recorded.
func (r *reader) readAreas(ctx context.Context) error {
	r.zones = map[string][]*zoneState{}
	type asked struct {
		targetID string
		round    int
		parts    []string
		call     llm.Call[areasAnswer]
	}
	var targets []asked
	for position, target := range r.opts.Targets {
		var parts []*boxState
		for _, box := range r.boxesOfTarget(target.ID) {
			if !box.offCanvas() && box.targetID == target.ID {
				parts = append(parts, box)
			}
		}
		if r.dry || len(parts) < minAreaParts {
			continue
		}
		input := r.areasInput(target.ID, parts)
		call, err := areasCall(input)
		if err != nil {
			return err
		}
		refs := make([]string, len(parts))
		for i, box := range parts {
			refs[i] = box.id
		}
		targets = append(targets, asked{targetID: target.ID, round: position + 1, parts: refs, call: call})
	}
	if len(targets) == 0 {
		return nil
	}
	r.started[lines.StageAreas] = time.Now()
	r.opts.Stage(lines.StageAreas, fmt.Sprintf("grouping the parts of %d targets into areas", len(targets)))
	calls := make([]llm.Call[areasAnswer], len(targets))
	for i, target := range targets {
		calls[i] = target.call
	}
	results := llm.ExecuteJSONEach(ctx, debugdump.BindStage(r.opts.Executor, lines.StageAreas), r.opts.Provider, calls)
	if err := ctx.Err(); err != nil {
		return err
	}
	use := r.use(lines.StageAreas)
	type drawn struct {
		targetID string
		round    int
		areas    validAreas
	}
	var accepted []drawn
	for i, result := range results {
		target := targets[i]
		window := table.Window{Stage: lines.StageAreas, Round: target.round}
		use.Windows++
		use.Rows += len(target.parts)
		if result.Outcome.Cached {
			use.Cached++
		} else {
			use.Live++
		}
		// An answer whose refs or areas the validation annotates is recorded
		// below as rejected rows; like a refused one, it stays in the run.
		var areas validAreas
		if result.Err == nil {
			areas = validateAreas(result.Outcome.Value, target.parts)
		}
		annotated := len(areas.unknown) > 0 || len(areas.notes) > 0
		if err := r.writeWindowExchange(window, []byte(calls[i].Prompt.System), []byte(calls[i].Prompt.User), result.Outcome.Request, result.Outcome.Response, result.Err != nil || len(result.Outcome.ResponseRejections) > 0 || annotated); err != nil {
			return err
		}
		responseRef := path.Join(atlas.TablesDir, r.windowFileName(window, "response.ref.json"))
		fmt.Fprintf(&r.tables, "## %s · round %d · window 0 · %s\n\n", lines.StageAreas, target.round, path.Join(atlas.TablesDir, r.windowFileName(window, "request.ref.json")))
		if result.Err != nil {
			use.Rejected++
			// A refusal that left no response has no response ref to name.
			if len(result.Outcome.Response) == 0 {
				responseRef = ""
			}
			r.rejected = append(r.rejected, modeldiag.Row{Stage: lines.StageAreas, Target: target.targetID, Kind: "window_rejected", Count: len(target.parts), Reason: result.Err.Error(), ResponseRef: responseRef})
			fmt.Fprintf(&r.tables, "areas answer refused, no areas drawn: %s\n\n", result.Err)
			continue
		}
		for i, name := range areas.names {
			fmt.Fprintf(&r.tables, "- %s: %s\n", name, strings.Join(areas.parts[i], " "))
		}
		for _, note := range append(append([]string(nil), areas.notes...), unknownNote(areas.unknown)...) {
			fmt.Fprintf(&r.tables, "- %s\n", note)
		}
		if len(areas.unknown) > 0 {
			r.rejected = append(r.rejected, modeldiag.Row{Stage: lines.StageAreas, Target: target.targetID, Kind: "area_unknown_ref", Count: len(areas.unknown), Samples: areas.unknown, ResponseRef: responseRef})
		}
		if len(areas.notes) > 0 {
			r.rejected = append(r.rejected, modeldiag.Row{Stage: lines.StageAreas, Target: target.targetID, Kind: "area_annotation", Count: len(areas.notes), Samples: areas.notes, ResponseRef: responseRef})
		}
		r.tables.WriteString("\n")
		accepted = append(accepted, drawn{targetID: target.targetID, round: target.round, areas: areas})
	}
	// Each area's line comes from its parts' names and lines.
	var describeCalls []llm.Call[description]
	for _, target := range accepted {
		for i, name := range target.areas.names {
			input := describeInput{Task: designDescribeTask, Part: name}
			for _, ref := range target.areas.parts[i] {
				box := r.boxes[ref]
				input.Members = append(input.Members, describeMember{Name: box.title, Line: box.line})
			}
			call, err := describeCall(input)
			if err != nil {
				return err
			}
			describeCalls = append(describeCalls, call)
		}
	}
	described, err := r.describe(ctx, lines.StageAreas, len(r.opts.Targets)+1, describeCalls)
	if err != nil {
		return err
	}
	at := 0
	for _, target := range accepted {
		for i, name := range target.areas.names {
			boxes := slices.Clone(target.areas.parts[i])
			sort.Slice(boxes, func(a, b int) bool { return compactIDLess(boxes[a], boxes[b]) })
			r.zones[target.targetID] = append(r.zones[target.targetID], &zoneState{id: r.compactID("z", &r.nextZone), title: name, line: described[at], boxes: boxes})
			at++
		}
	}
	r.reportStage(lines.StageAreas)
	return nil
}

func unknownNote(unknown []string) []string {
	if len(unknown) == 0 {
		return nil
	}
	return []string{"refs the request did not list, discarded: " + strings.Join(unknown, " ")}
}
