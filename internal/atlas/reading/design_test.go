package reading

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/llm"
)

func boxesByTitle(result Result) map[string]atlas.Box {
	boxes := map[string]atlas.Box{}
	for _, box := range result.Atlas.Targets[0].Boxes {
		boxes[box.Title] = box
	}
	return boxes
}

func boxFiles(box atlas.Box) string {
	var files []string
	for _, file := range box.Files {
		files = append(files, file.Path)
	}
	return strings.Join(files, " ")
}

// A proposed part is drawn from the units that chose it, across directories.
func TestDesignJoinsUnitsAcrossDirectories(t *testing.T) {
	graph := knowledgeGraph(t)
	provider := &tableProvider{
		designFor: func(mode string, _ []map[string]any) designProposals {
			if mode == "areas" {
				return designProposals{Groups: []designProposal{}}
			}
			return designProposals{Groups: []designProposal{
				{Title: "Application", Purpose: "Coordinates the work."},
				{Title: "Generation", Purpose: "Generates values."},
			}}
		},
		zoneFor: func(_ string, row map[string]any) string {
			if row["name"] == "Gen" {
				return "Generation"
			}
			return "Application"
		},
	}
	result, err := Read(t.Context(), readOptions(t, graph, provider, ""))
	if err != nil {
		t.Fatal(err)
	}
	boxes := boxesByTitle(result)
	if boxFiles(boxes["Application"]) != "pkg/a/x.go pkg/a/y.go pkg/b/z.go" || boxFiles(boxes["Generation"]) != "pkg/b/gen.go" {
		t.Fatalf("parts do not follow the assignment: %+v", boxes)
	}
	if len(result.Atlas.Targets[0].Zones) != 0 {
		t.Fatal("invented a wrapping area")
	}
	if err := atlas.Validate(result.Atlas); err != nil {
		t.Fatal(err)
	}
}

// A unit that chose no part stays in its source file's inventory; an unchosen
// part is not drawn.
func TestDesignUnassignedUnitsStaySourceInventory(t *testing.T) {
	graph := knowledgeGraph(t)
	provider := &tableProvider{
		designFor: func(mode string, _ []map[string]any) designProposals {
			return designProposals{Groups: []designProposal{{Title: "Unused", Purpose: "Nobody chooses it."}}}
		},
		zoneFor: func(string, map[string]any) string { return "" },
	}
	result, err := Read(t.Context(), readOptions(t, graph, provider, ""))
	if err != nil {
		t.Fatal(err)
	}
	boxes := boxesByTitle(result)
	if _, drawn := boxes["Unused"]; drawn {
		t.Fatal("drew a part no unit chose")
	}
	if _, kept := boxes["pkg/a/x.go"]; !kept {
		t.Fatalf("unassigned declarations lost their source file: %+v", boxes)
	}
}

// Areas are proposed over drawn parts and hold only the parts that chose them.
func TestDesignAreasHoldTheirChosenParts(t *testing.T) {
	graph := knowledgeGraph(t)
	provider := &tableProvider{
		designFor: func(mode string, input []map[string]any) designProposals {
			if mode == "areas" {
				return designProposals{Groups: []designProposal{{Title: "Serving", Purpose: "Serves the work."}}}
			}
			return designProposals{Groups: []designProposal{
				{Title: "pkg/a", Purpose: "Runs."}, {Title: "pkg/b", Purpose: "Holds values."},
			}}
		},
		zoneFor: func(column string, row map[string]any) string {
			if column == "area" {
				if row["title"] == "pkg/a" {
					return "Serving"
				}
				return ""
			}
			return row["package"].(string)
		},
	}
	result, err := Read(t.Context(), readOptions(t, graph, provider, ""))
	if err != nil {
		t.Fatal(err)
	}
	target := result.Atlas.Targets[0]
	boxes := boxesByTitle(result)
	if len(target.Zones) != 1 || len(target.Zones[0].BoxIDs) != 1 || target.Zones[0].BoxIDs[0] != boxes["pkg/a"].ID || boxes["pkg/b"].ZoneID != "" {
		t.Fatalf("areas: %+v boxes %+v", target.Zones, boxes)
	}
}

func TestDesignProposalsNeedTitlesAndPurposes(t *testing.T) {
	result, err := decodeDesignProposals([]byte(`{"groups":{" HTTP\nAPI ":"Receives\r\nrequests."}}`))
	if err != nil || len(result.Groups) != 1 || result.Groups[0].Title != "HTTP API" || result.Groups[0].Purpose != "Receives requests." {
		t.Fatalf("proposals: %+v %v", result, err)
	}
	if _, err := decodeDesignProposals([]byte(`{"groups":{"Only":""}}`)); err == nil {
		t.Fatal("accepted a proposal without a purpose")
	}
}

// The zone tables are recorded under their own stages.
func TestDesignAssignmentTablesHaveTheirOwnStages(t *testing.T) {
	if lines.ZoneParts().Stage == lines.StageZones || lines.ZoneAreas().Stage == lines.StageZones {
		t.Fatal("assignment tables share the proposal stage")
	}
}

// fakeClassifier answers closed tables in the decision-model form: every
// part question chooses the part named after the row's package.
type fakeClassifier struct{ tables []string }

func (*fakeClassifier) State() []byte { return []byte(`{"model":"classifier"}`) }
func (*fakeClassifier) Prepare(prompt llm.Prompt, _ llm.Limits) (llm.Prepared, error) {
	return llm.NewPrepared([]byte(prompt.User))
}
func (c *fakeClassifier) Complete(_ context.Context, prepared llm.Prepared) (llm.Completion, error) {
	var body struct {
		State struct {
			Context map[string]any `json:"context"`
		} `json:"state"`
		Questions map[string]struct {
			Instructions struct {
				Row map[string]any `json:"row"`
			} `json:"instructions"`
		} `json:"questions"`
	}
	if err := json.Unmarshal(prepared.Bytes(), &body); err != nil {
		return llm.Completion{}, err
	}
	answers := map[string]any{}
	for key, question := range body.Questions {
		column := key[strings.Index(key, "|")+1:]
		c.tables = append(c.tables, column)
		// The model sees each part by its title and answers with it.
		choice := zoneTitle(column, question.Instructions.Row, body.State.Context)
		answers[key] = map[string]any{"type": "choice", "choice": choice, "confidence": 0.9, "probabilities": map[string]float64{choice: 0.9}}
	}
	raw, err := json.Marshal(map[string]any{"answers": answers})
	return llm.Completion{Response: raw, FinishReason: llm.FinishStop, ChoiceCount: 1, Metrics: llm.Metrics{Attempts: 1}}, err
}

func TestClosedZoneTablesGoToTheClassifier(t *testing.T) {
	graph := knowledgeGraph(t)
	classifier := &fakeClassifier{}
	opts := readOptions(t, graph, &tableProvider{}, "")
	opts.Classifier = classifier
	result, err := Read(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	boxes := boxesByTitle(result)
	if boxFiles(boxes["pkg/a"]) != "pkg/a/x.go pkg/a/y.go" || len(classifier.tables) == 0 {
		t.Fatalf("classifier did not assign the parts: %v %+v", classifier.tables, boxes)
	}
	if !slices.Contains(classifier.tables, "part") {
		t.Fatalf("part assignment did not reach the classifier: %v", classifier.tables)
	}
}

func zoneTitle(column string, row, context map[string]any) string {
	ref := zoneRef(nil, column, row, context)
	entries, _ := context[column+"s"].([]any)
	for _, entry := range entries {
		if item, _ := entry.(map[string]any); item != nil && item["ref"] == ref {
			return fmt.Sprint(item["title"])
		}
	}
	return "none of these"
}

// A catalogue is refused whole, never shrunk: every unit is assigned among
// its groups. A live answer once lost 13 of 14 titles to a misnamed field.
func TestDesignProposalRefusesABrokenCatalogue(t *testing.T) {
	for _, raw := range []string{
		`{"groups":{"Entry":"Starts.","entry":"Starts again."}}`,
		`{"groups":{"Entry":"Starts.","Corpus":""}}`,
		`{"groups":[{"title":"Entry","purpose":"Starts."}]}`,
	} {
		if _, err := decodeDesignProposals([]byte(raw)); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	result, err := decodeDesignProposals([]byte(`{"groups":{"Entry":"Starts.","Corpus":"Reads files."}}`))
	if err != nil || len(result.Groups) != 2 || result.Groups[1].Title != "Corpus" {
		t.Fatalf("result %+v %v", result, err)
	}
}
