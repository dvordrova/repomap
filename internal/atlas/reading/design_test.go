package reading

import (
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
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
	result, err := decodeDesignProposals([]byte(`{"groups":[{"title":" HTTP\nAPI ","purpose":"Receives\r\nrequests."},{"title":"http api","purpose":"Duplicate."},{"title":"Untitled"}]}`))
	if err != nil || len(result.Groups) != 1 || result.Groups[0].Title != "HTTP API" || result.Groups[0].Purpose != "Receives requests." {
		t.Fatalf("proposals: %+v %v", result, err)
	}
	if _, err := decodeDesignProposals([]byte(`{"groups":[{"title":"Only"}]}`)); err == nil {
		t.Fatal("accepted a proposal without a purpose")
	}
}

// The zone tables are recorded under their own stages.
func TestDesignAssignmentTablesHaveTheirOwnStages(t *testing.T) {
	if lines.ZoneParts().Stage == lines.StageZones || lines.ZoneAreas().Stage == lines.StageZones {
		t.Fatal("assignment tables share the proposal stage")
	}
}
