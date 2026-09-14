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
	if _, err := decodeDesign([]byte(`{"groups":[{"title":"Wrapper","purpose":"Wraps.","members":["r1"]}]}`), items, "areas"); err == nil {
		t.Fatal("one-part wrapper accepted")
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
