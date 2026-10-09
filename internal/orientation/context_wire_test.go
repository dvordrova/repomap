package orientation

import (
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/llm"
)

// Expansion exists only in this representation test. The ordinary decoder
// derives provenance from the original logical item, never from model aliases.
func expandContextWire(t *testing.T, raw []byte) (contextReadingWire, []orientationRecord) {
	t.Helper()
	var wire contextReadingWire
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	lookup := func(cat *contextViewCatalogue, layer string) map[string]json.RawMessage {
		views := map[string]json.RawMessage{}
		if cat == nil {
			return views
		}
		if cat.Layer != layer {
			t.Fatalf("catalogue layer %q, want %q", cat.Layer, layer)
		}
		for _, view := range cat.Views {
			var identity struct{ Ref string }
			if err := json.Unmarshal(view, &identity); err != nil || identity.Ref == "" {
				t.Fatalf("invalid closed view: %s / %v", view, err)
			}
			if _, exists := views[identity.Ref]; exists {
				t.Fatalf("repeated catalogue identity %q", identity.Ref)
			}
			views[identity.Ref] = view
		}
		return views
	}
	groups := lookup(wire.Groups, "model_interpretation")
	targets := lookup(wire.Targets, "native")
	seeds := lookup(wire.SeedHeaders, "native")
	rows := append([]orientationRecord(nil), wire.Records...)
	for i := range rows {
		row := &rows[i]
		fields := map[string]map[string]json.RawMessage{}
		switch row.Kind {
		case "connection":
			fields = map[string]map[string]json.RawMessage{"from_group": groups, "to_group": groups, "from_target": targets, "to_target": targets}
		case "seed_call", "seed_evidence":
			fields = map[string]map[string]json.RawMessage{"seed": seeds}
		default:
			continue
		}
		encoded, err := encodeWire(row.Value)
		if err != nil {
			t.Fatal(err)
		}
		var value map[string]json.RawMessage
		if err := json.Unmarshal(encoded, &value); err != nil {
			t.Fatal(err)
		}
		for field, views := range fields {
			var ref string
			if err := json.Unmarshal(value[field+"_ref"], &ref); err != nil {
				t.Fatalf("missing local ref %s: %v", field, err)
			}
			full, exists := views[ref]
			if !exists {
				t.Fatalf("view %q missing from actual leaf", ref)
			}
			value[field] = full
			delete(value, field+"_ref")
		}
		row.Value = value
	}
	return wire, rows
}

func contextWireFixture() (overviewRequest, targetWire) {
	first := targetWire{Ref: "t1", Name: "CLI", Language: "c", Kind: "executable", Root: ".", Manifest: "Makefile"}
	second := targetWire{Ref: "t2", Name: "Engine", Language: "c", Kind: "library", Root: ".", Manifest: "Makefile"}
	return overviewRequest{
		Repository: "fixture", Targets: []targetWire{first, second},
		Facts: []factWire{{Ref: "a1", Kind: "manifest", Targets: []string{"t1"}, Anchor: "Makefile:9", Key: "portable", Value: strings.Repeat("-Wextra ", 25)}},
		Groups: []groupWire{
			{Ref: "t1.g1", Target: "t1", Lane: "triggers", Title: "Dispatch", Summary: "Routes commands.", MemberCount: 2},
			{Ref: "t1.g2", Target: "t1", Lane: "helpers", Title: "Dispatch", Summary: "Routes commands.", MemberCount: 2},
			{Ref: "t2.g1", Target: "t2", Lane: "data", Title: "Storage", Summary: "Stores pages.", MemberCount: 3},
		},
		Connections: []connectionWire{
			{From: "t1.g1", To: "t2.g1", Kind: "calls", Labels: []string{"main calls store"}, Sentences: []string{"Command execution uses storage."}},
			{From: "t1.g2", To: "t2.g1", Kind: "calls", Labels: []string{"helper calls store"}},
		},
		Seeds: []memberRow{{Ref: "t1.n1", Name: "main", Kind: "function", Anchor: "cli.c:7", Signature: "int main(int argc, char **argv)",
			Calls:    []any{[]any{"store@8", map[string]any{"evidence_refs": []string{"e1"}, "values": []string{"--database"}}}, "cleanup@9"},
			Evidence: map[string][]any{"e1": {map[string]any{"anchor": "cli.c:8", "value": "argv[1]"}}, "unused": {"complete independent source"}}}},
	}, first
}

func TestContextWireSharesOnlyCompleteLocalViews(t *testing.T) {
	overview, target := contextWireFixture()
	records := orientationRecords(overview, target)
	item := orientationReading{contextTask, target, records}
	before, _ := encodeWire(item)
	raw, err := encodeContextReading(item)
	if err != nil {
		t.Fatal(err)
	}
	wire, expanded := expandContextWire(t, raw)
	if !sameOrientationRecords(records, expanded) {
		t.Fatal("shared representation erased an original record value, use, layer or source")
	}
	after, _ := encodeWire(item)
	if string(before) != string(after) {
		t.Fatal("packing mutated the original logical input")
	}
	if wire.Scope != (orientationScope{"w1", "all_supplied_records", records[0].Ref, records[len(records)-1].Ref}) || wire.Task != item.Task || wire.Target != item.Target {
		t.Fatal("scope, task or owning target changed")
	}
	if len(wire.Groups.Views) != 3 || len(wire.Targets.Views) != 2 || len(wire.SeedHeaders.Views) != 1 {
		t.Fatal("different identities merged or identical full views duplicated")
	}
	for _, record := range records {
		if record.Kind != "connection" && record.Kind != "seed_call" && record.Kind != "seed_evidence" {
			continue
		}
		// Each real split leaf has no standalone endpoint/seed definition row.
		leaf := orientationReading{contextTask, target, []orientationRecord{record}}
		encoded, err := encodeContextReading(leaf)
		if err != nil {
			t.Fatal(err)
		}
		local, expanded := expandContextWire(t, encoded)
		if !sameOrientationRecords(leaf.Records, expanded) {
			t.Fatalf("leaf %s lost context", record.Ref)
		}
		if record.Kind == "connection" && (len(local.Targets.Views) != 2 || len(local.Groups.Views) != 2) {
			t.Fatal("cross-target endpoint-only leaf lost complete endpoints")
		}
		if record.Kind != "connection" && len(local.SeedHeaders.Views) != 1 {
			t.Fatal("split call/evidence leaf lacks its complete declaration")
		}
	}
}

func TestContextWireRefusesConflictingSameIdentityWithoutErasure(t *testing.T) {
	overview, target := contextWireFixture()
	overview.Groups[1].Ref = overview.Groups[0].Ref
	overview.Groups[1].Summary = "A different interpretation of the same identity."
	// Direct native view values make the collision explicit instead of first
	// erasing it through orientationRecords' already-closed group lookup.
	value := map[string]any{"connection": overview.Connections[0], "from_group": overview.Groups[0], "to_group": overview.Groups[1], "from_target": target, "to_target": target}
	_, err := encodeContextReading(orientationReading{contextTask, target, []orientationRecord{{Ref: "d1", Layer: "model_interpretation", Kind: "connection", Value: value}}})
	if err == nil || !strings.Contains(err.Error(), "conflicting") {
		t.Fatalf("same-ID differing view silently erased: %v", err)
	}
}

type contextWireNoHTTP struct{ t *testing.T }

func (transport contextWireNoHTTP) RoundTrip(*http.Request) (*http.Response, error) {
	transport.t.Error("request preparation made an HTTP call")
	return nil, fmt.Errorf("HTTP is forbidden in this preparation test")
}

func TestContextWireSavedSQLiteMeasuresActualPreparedBytesAndKeepsFinalNativeChoices(t *testing.T) {
	file, err := os.Open(filepath.Join("testdata", "sqlite-overview-complete-window.json.gz"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	z, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	raw, err := io.ReadAll(z)
	if err != nil {
		t.Fatal(err)
	}
	var overview overviewRequest
	if err := json.Unmarshal(raw, &overview); err != nil {
		t.Fatal(err)
	}
	if len(overview.Connections) != 5544 || len(overview.Facts) != 730 || len(overview.Groups) != 484 {
		t.Fatal("saved complete source window changed")
	}
	client := &deepseek.Client{HTTPClient: &http.Client{Transport: contextWireNoHTTP{t}}, APIKey: "request-bound-local-preset", Endpoint: "https://api.deepseek.com/chat/completions", Model: "deepseek-v4-flash", ContextTokens: 1000000, MaxTokens: llm.DefaultMaxOutputTokens}
	readingLimits := limits()
	readingLimits.MaxOutputTokens = llm.DefaultMaxOutputTokens
	preparedBytes := func(user []byte) int {
		prepared, err := client.Prepare(llm.Prompt{System: contextPrompt, User: string(user), ResponseFormatJSON: true, NoResponseAdjunct: true}, readingLimits)
		if err == nil {
			return prepared.Len()
		}
		var resource *llm.ResourceLimitError
		if !errors.As(err, &resource) || !resource.ObservedKnown || resource.FinishReason != "utf8_byte_reservation" {
			t.Fatalf("unexpected local preparation error: %v", err)
		}
		// Prepare records the exact encoded body plus the reserved output even
		// for a complete window that cannot be transported as one request.
		return resource.Observed - readingLimits.MaxOutputTokens
	}
	cat := newCatalog()
	for _, target := range overview.Targets {
		cat.targets[target.Ref] = target.Ref
	}
	for _, fact := range overview.Facts {
		entry := factEntry{id: fact.Ref, kind: facts.Kind(fact.Kind), export: fact.Kind == exportKind, byTarget: map[string]string{}}
		for _, target := range fact.Targets {
			entry.byTarget[target] = fact.Ref
		}
		cat.facts[fact.Ref] = entry
	}
	for _, seed := range overview.Seeds {
		target, _, _ := strings.Cut(seed.Ref, ".")
		cat.subjects[seed.Ref] = subjectEntry{id: seed.Ref, targetRef: target}
	}
	bindContextEvidence(cat, overview)
	var global []orientationObservation
	for _, target := range overview.Targets {
		records := orientationRecords(overview, target)
		item := orientationReading{contextTask, target, records}
		old, err := encodeWire(struct {
			Scope orientationScope `json:"scope"`
			orientationReading
		}{orientationScope{"w1", "all_supplied_records", records[0].Ref, records[len(records)-1].Ref}, item})
		if err != nil {
			t.Fatal(err)
		}
		shared, err := encodeContextReading(item)
		if err != nil {
			t.Fatal(err)
		}
		_, expanded := expandContextWire(t, shared)
		if !sameOrientationRecords(records, expanded) {
			t.Fatalf("complete saved native records changed for %s", target.Ref)
		}
		oldBytes, sharedBytes := preparedBytes(old), preparedBytes(shared)
		if sharedBytes >= oldBytes {
			t.Fatalf("exact sharing did not reduce actual provider bytes: %d >= %d", sharedBytes, oldBytes)
		}
		t.Logf("target=%s records=%d user_bytes=%d->%d prepared_bytes=%d->%d", target.Ref, len(records), len(old), len(shared), oldBytes, sharedBytes)
		// Check actual leaf-local definitions under splitting as well as the
		// unsendable complete parent. No record quota enters production code.
		for start := 0; start < len(records); {
			end := min(start+len(records)/8+1, len(records))
			leaf := orientationReading{contextTask, target, records[start:end]}
			packed, err := encodeContextReading(leaf)
			if err != nil {
				t.Fatal(err)
			}
			_, expanded := expandContextWire(t, packed)
			if !sameOrientationRecords(leaf.Records, expanded) {
				t.Fatalf("split saved native records changed for %s", target.Ref)
			}
			if preparedBytes(packed)+readingLimits.MaxOutputTokens > client.ContextTokens {
				t.Fatal("saved leaf does not fit actual prepared envelope")
			}
			start = end
		}
		observation := orientationObservation{Text: "Complete source reading for preparation measurement."}
		for _, record := range records {
			observation.Sources = unionRefs(observation.Sources, record.Sources)
		}
		global = append(global, observation)
		for _, section := range []string{"roles", "run_recipe"} {
			assertContextFinalChoicesPrepared(t, client, overview.Repository, []targetWire{target}, []orientationObservation{observation}, cat, section)
		}
	}
	assertContextFinalChoicesPrepared(t, client, overview.Repository, overview.Targets, global, cat, "repository")
}

func assertContextFinalChoicesPrepared(t *testing.T, client *deepseek.Client, repository string, targets []targetWire, observations []orientationObservation, cat catalog, section string) {
	t.Helper()
	wire, err := finalContextWire(repository, targets, observations, cat, section)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := client.Prepare(llm.Prompt{System: partitionPrompt, User: string(wire), ResponseFormatJSON: true, ResponseExample: sectionExample(section)}, limits())
	if err != nil {
		t.Fatalf("full native final choices do not prepare for %s: %v", section, err)
	}
	var decoded struct {
		Sources []struct {
			Ref      string          `json:"ref"`
			Evidence json.RawMessage `json:"evidence"`
		} `json:"citation_sources"`
	}
	if err := json.Unmarshal(wire, &decoded); err != nil {
		t.Fatal(err)
	}
	var refs []string
	for _, observation := range observations {
		refs = unionRefs(refs, observation.Sources)
	}
	if len(decoded.Sources) != len(refs) {
		t.Fatal("native final choices lost an original source identity")
	}
	for _, source := range decoded.Sources {
		expected, err := encodeWire(cat.evidence[source.Ref])
		if err != nil || string(expected) != string(source.Evidence) {
			t.Fatalf("final source %q lost its complete native evidence", source.Ref)
		}
	}
	t.Logf("final_section=%s targets=%d native_choices=%d user_bytes=%d prepared_bytes=%d", section, len(targets), len(decoded.Sources), len(wire), prepared.Len())
}
