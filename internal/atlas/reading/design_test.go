package reading

import (
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/llm"
)

func TestDesignSplitsOneFileAndJoinsAcrossDirectories(t *testing.T) {
	graph := knowledgeGraph(t)
	// Main and help live beside each other; Z lives elsewhere. The accepted
	// design joins Main with Z, leaving help as a separate collaborator.
	provider := &tableProvider{designFor: func(mode string, items []designItem) designResult {
		result := designResult{Groups: []designGroup{}}
		if mode == "areas" {
			return result
		}
		joint := designGroup{Title: "Application", Purpose: "Coordinates the work."}
		for _, item := range items {
			if item.Name == "Main" || item.Name == "Z" {
				joint.Members = append(joint.Members, item.Ref)
			} else {
				result.Groups = append(result.Groups, designGroup{Title: item.Name, Purpose: "Provides an independent collaborator.", Members: []string{item.Ref}})
			}
		}
		result.Groups = append(result.Groups, joint)
		return result
	}}
	// Put the helper inside Main's file while preserving its exact declaration.
	for i := range graph.Places {
		p := &graph.Places[i]
		if p.File != nil && p.Path == "pkg/a/x.go" {
			p.File.Decls = append(p.File.Decls, atlas.Decl{Name: "Separate", Kind: "function", LineNo: 30, ObjectID: "separate"})
		}
	}
	result, err := Read(t.Context(), readOptions(t, graph, provider, ""))
	if err != nil {
		t.Fatal(err)
	}
	var application, helper atlas.Box
	for _, box := range result.Atlas.Targets[0].Boxes {
		if box.Title == "Application" {
			application = box
		}
		if box.Title == "Separate" {
			helper = box
		}
	}
	if len(application.Files) != 2 || len(helper.Files) != 1 || helper.Files[0].Path != "pkg/a/x.go" || len(helper.Files[0].Symbols) != 1 {
		t.Fatalf("source layout overrode the design: application=%+v helper=%+v", application, helper)
	}
	if len(result.Atlas.Targets[0].Zones) != 0 {
		t.Fatal("invented a wrapping area")
	}
	if err := atlas.Validate(result.Atlas); err != nil {
		t.Fatal(err)
	}
}

func TestDesignSavedCompleteWindow(t *testing.T) {
	filename := os.Getenv("REPOMAP_DESIGN_PROBE_INPUT")
	if filename == "" {
		t.Skip("saved ordinary window probe")
	}
	input, err := LoadInput(filename)
	if err != nil {
		t.Fatal(err)
	}
	r := &reader{opts: input.Options(), places: map[string]atlas.Place{}}
	for _, place := range input.Graph.Places {
		r.places[place.ID] = place
	}
	for _, target := range input.Targets {
		items, docs := r.designInputs(target.ID)
		count := 0
		for _, file := range input.Graph.Places {
			if file.File != nil && contains(file.TargetIDs, target.ID) {
				count += len(file.File.Decls)
			}
		}
		if len(items) != count {
			t.Fatalf("sampled declarations: %d of %d", len(items), count)
		}
		call, err := designCallFor(items, docs, "parts")
		if err != nil {
			t.Fatal(err)
		}
		provider := &deepseek.Client{HTTPClient: &http.Client{}, Endpoint: "https://api.deepseek.com/chat/completions", Model: "deepseek-v4-flash", Auth: "none", MaxTokens: llm.DefaultMaxOutputTokens}
		prepared, err := llm.Prepare(provider, call.Prompt, call.Limits)
		if err != nil {
			t.Fatal(err)
		}
		var members []string
		for _, item := range items {
			members = append(members, item.Ref)
		}
		raw, _ := json.Marshal(designResult{Groups: []designGroup{{Title: "Probe", Purpose: "Checks complete reference restoration.", Members: members}}})
		value, err := call.DecodeValidate(raw)
		if err != nil || len(value.Groups[0].Members) != count {
			t.Fatal("complete response refused", err)
		}
		if out := os.Getenv("REPOMAP_DESIGN_PROBE_OUTPUT"); out != "" {
			if err := os.WriteFile(out, prepared.Bytes(), 0600); err != nil {
				t.Fatal(err)
			}
		}
		t.Logf("%s: %d declarations, %d author contexts, %d prepared bytes, sha256 %x; all refs accepted, no provider calls", target.Name, count, len(docs), prepared.Len(), sha256.Sum256(prepared.Bytes()))
	}
}

func TestDesignRefusalPreservesIndependentGroupsWithoutRepair(t *testing.T) {
	items := []designItem{{Ref: "r1"}, {Ref: "r2"}, {Ref: "r3"}, {Ref: "r4"}}
	result, err := decodeDesign([]byte(`{"groups":[{"title":"Accepted","purpose":"Reads.","members":["r1","outside"]},{"title":"Conflict","purpose":"Writes.","members":["r1","r2"]},{"title":"Malformed","members":["r3"]},{"title":"Independent","purpose":"Runs.","members":["r4"]}]}`), items, "parts")
	if err != nil || len(result.Groups) != 1 || len(result.Notes) != 7 || result.Groups[0].Members[0] != "r4" {
		t.Fatalf("invalid independent validation: %+v %v", result, err)
	}
	for _, group := range result.Groups {
		for _, ref := range group.Members {
			if ref == "r1" || ref == "r2" || ref == "r3" {
				t.Fatal("repaired a refused membership")
			}
		}
	}
}

func TestDesignAreaMembershipIsNotACountHeuristic(t *testing.T) {
	items := []designItem{{Ref: "r1"}, {Ref: "r2"}, {Ref: "r3"}}
	result, err := decodeDesign([]byte(`{"groups":[{"title":"API surface","purpose":"Exposes the service boundary.","members":["r1","r1","outside"]}]}`), items, "areas")
	if err != nil || len(result.Groups) != 1 || len(result.Groups[0].Members) != 1 || result.Groups[0].Members[0] != "r1" || len(result.Notes) != 1 {
		t.Fatalf("valid single-part area refused or unknown membership repaired: %+v %v", result, err)
	}
	result, err = decodeDesign([]byte(`{"groups":[{"title":"Single","purpose":"Owns the API.","members":["r1"]},{"title":"Conflict","purpose":"Claims the same part.","members":["r1","r2"]},{"title":"Independent","purpose":"Owns storage.","members":["r3"]}]}`), items, "areas")
	if err != nil || len(result.Groups) != 1 || result.Groups[0].Members[0] != "r3" || len(result.Notes) != 2 {
		t.Fatalf("singleton escaped conflict validation: %+v %v", result, err)
	}
	if _, err := decodeDesign([]byte(`{"groups":[{"title":"Unknown","purpose":"Has no known member.","members":["outside"]}]}`), items, "areas"); err == nil {
		t.Fatal("area without any known part accepted")
	}
}

func TestDesignAcceptsExplicitAbstention(t *testing.T) {
	for _, mode := range []string{"parts", "merge", "areas"} {
		t.Run(mode, func(t *testing.T) {
			result, err := decodeDesign([]byte(`{"groups":[]}`), []designItem{{Ref: "r1"}}, mode)
			if err != nil || len(result.Groups) != 0 {
				t.Fatalf("explicit ungrouped decision refused: %+v %v", result, err)
			}
			if _, err := decodeDesign([]byte(`{"groups":[{"title":"Unknown","purpose":"Reads.","members":["outside"]}]}`), []designItem{{Ref: "r1"}}, mode); err == nil {
				t.Fatal("wholly invalid response became an accepted abstention")
			}
		})
	}
}

func TestDesignWrappedCaptionsKeepTheirMeaning(t *testing.T) {
	result, err := decodeDesign([]byte(`{"groups":[{"title":" HTTP\nAPI ","purpose":"Receives\r\nrequests\tand responds.","members":["r1"]}]}`), []designItem{{Ref: "r1"}}, "parts")
	if err != nil || len(result.Groups) != 1 || result.Groups[0].Title != "HTTP API" || result.Groups[0].Purpose != "Receives requests and responds." {
		t.Fatalf("formatting discarded an understood responsibility: %+v %v", result, err)
	}
}

func TestDesignOmissionsDoNotCountAsRefusedGroups(t *testing.T) {
	for _, empty := range []bool{false, true} {
		provider := &tableProvider{designFor: func(mode string, items []designItem) designResult {
			if mode == "areas" || empty {
				return designResult{Groups: []designGroup{}}
			}
			return designResult{Groups: []designGroup{{Title: "Known", Purpose: "Reads.", Members: []string{items[0].Ref, "outside"}}}}
		}}
		result, err := Read(t.Context(), readOptions(t, knowledgeGraph(t), provider, ""))
		if err != nil {
			t.Fatal(err)
		}
		for _, use := range result.Atlas.Budget.Stages {
			if use.Stage == "atlas_zones" && use.Rejected != 0 {
				t.Fatalf("valid omission/unknown member counted as rejected window: %+v", use)
			}
		}
		for _, note := range result.Rejected {
			if note.Stage == "atlas_zones" && (note.Kind == "group_rejected" || note.Kind == "window_rejected") {
				t.Fatalf("valid omission/unknown member counted as refused group: %+v", note)
			}
		}
		if err := atlas.Validate(result.Atlas); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDesignSinglePartAreaSurvivesReading(t *testing.T) {
	provider := &tableProvider{designFor: func(mode string, items []designItem) designResult {
		group := designGroup{Title: "Application", Purpose: "Provides the application boundary."}
		for _, item := range items {
			group.Members = append(group.Members, item.Ref)
		}
		return designResult{Groups: []designGroup{group}}
	}}
	result, err := Read(t.Context(), readOptions(t, knowledgeGraph(t), provider, ""))
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range result.Atlas.Targets {
		if len(target.Zones) != 1 || len(target.Zones[0].BoxIDs) != 1 || len(target.Boxes) != 1 || target.Boxes[0].ZoneID != target.Zones[0].ID || target.Zones[0].BoxIDs[0] != target.Boxes[0].ID {
			t.Fatalf("single-part area lost on the ordinary reader path: %+v", target)
		}
	}
	if err := atlas.Validate(result.Atlas); err != nil {
		t.Fatal(err)
	}
}

func TestDesignReductionKeepsRelationsAndUsesOnlyLocalRefs(t *testing.T) {
	items := []designItem{{Ref: "r1", Name: "Start", IDs: []string{"internal:start"}, Calls: []designCall{{Kind: "calls", Name: "Load", To: []string{"r2"}, Line: 12, Resolution: "exact"}}}, {Ref: "r2", Name: "Load", IDs: []string{"internal:load"}}}
	parts := designPartContext(items, []designItem{{Name: "Storage", Purpose: "Loads data.", IDs: []string{"internal:load"}}, {Name: "Application", Purpose: "Starts work.", IDs: []string{"internal:start"}}})
	if len(parts[1].Calls) != 1 || parts[1].Calls[0].To[0] != "r1" {
		t.Fatalf("lost cross-part evidence: %+v", parts)
	}
	call, err := designCallFor(parts, nil, "areas")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(call.Prompt.User, "internal:") || !strings.Contains(call.Prompt.User, `"line":12`) || !strings.Contains(call.Prompt.User, `"resolution":"exact"`) {
		t.Fatal("reduction leaked identity or lost native evidence")
	}
	var input struct{ Items []designItem }
	if err := json.Unmarshal([]byte(call.Prompt.User), &input); err != nil || len(input.Items) != 2 {
		t.Fatal("incomplete input", err)
	}
}
