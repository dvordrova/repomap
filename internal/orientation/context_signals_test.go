package orientation

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/llm"
)

func signalInput() orientationReading {
	return orientationReading{Task: contextTask, Target: targetWire{Ref: "t1"}, Records: []orientationRecord{
		{Ref: "d1", Layer: "native", Kind: "seed_call", Value: map[string]any{"seed": map[string]any{"ref": "t1.n1", "symbol": "begin"}, "position": 2, "call": []any{"begin", "->", "store"}, "evidence": map[string]any{"e1": []any{"original-use", 17}}, "related_facts": []any{map[string]any{"ref": "a2", "value": "second exact value"}}}, Sources: []string{"a1", "a2", "t1.n1"}},
		{Ref: "d2", Layer: "model_interpretation", Kind: "observation", Value: map[string]any{"text": "Prior prose", "source_inventory": []string{"a9"}}, Sources: []string{"a9"}},
		{Ref: "d3", Layer: "native", Kind: "seed_call", Value: map[string]any{"seed": map[string]any{"ref": "t1.n1", "symbol": "begin"}, "position": 3, "call": []any{"begin", "->", "flush"}, "evidence": map[string]any{"e2": []any{"distinct-use", 24}}}, Sources: []string{"t1.n1"}},
	}}
}
func completeSignals(responsibilities, interfaces, launch, uncertainty string) []byte {
	return []byte(`{"scope":"w1","signals":{"responsibilities":` + responsibilities + `,"interfaces":` + interfaces + `,"launch_requirements":` + launch + `,"uncertainty":` + uncertainty + `}}`)
}

const supportedSignal = `{"basis":["d1"],"text":"Offers the observed interface.","status":"supported","supports":["d1"]}`

func TestContextSignalsKeepMalformedKnownRowsIndependentFromUnknownMetadata(t *testing.T) {
	for _, bad := range []string{
		`{"basis":null,"basis":["d1"],"text":"Bad.","status":"supported","supports":["d1"]}`,
		`{"basis":["outside"],"basis":["d1"],"text":"Bad.","status":"supported","supports":["d1"]}`,
		`{"basis":["d1"],"text":42,"status":"supported","supports":["d1"]}`,
		`{"basis":["d1"],"text":"Bad.","status":"supported","supports":["d2"]}`,
		`{"basis":["d1"],"text":"Bad.","status":"supported","supports":["outside"]}`,
		`{"basis":["outside"],"text":"Bad.","status":"supported","supports":["d1"]}`,
	} {
		for _, rows := range []string{`[` + bad + `,` + supportedSignal + `]`, `[` + supportedSignal + `,` + bad + `]`} {
			got, err := decodeOrientationContext(completeSignals(rows, `[]`, `[]`, `[]`), signalInput())
			if err != nil || len(got.Observations) != 1 || len(got.Rejected) != 1 || !got.permits("roles") || got.Observations[0].Text != "Offers the observed interface." {
				t.Fatalf("known bad row repaired or neighbour lost: %s %+v %v", rows, got, err)
			}
		}
	}
	valid := completeSignals(`[`+supportedSignal+`,{"basis":["outside"],"text":42,"supports":["outside"]},42]`, `[]`, `[]`, `[]`)
	wrapped := append(append([]byte(`{"result":`), valid...), []byte(`,"extra":{"scope":42,"reading":"metadata"},"unknown":{"scope":"outside","signals":42}}`)...)
	got, err := decodeOrientationContext(wrapped, signalInput())
	if err != nil || len(got.Rejected) != 0 || len(got.Observations) != 1 {
		t.Fatalf("unknown rows/metadata poisoned known signal: %+v %v", got, err)
	}
}

func TestContextQuestionsMissingAndConflictingOccurrencesRefuseOnlyDependents(t *testing.T) {
	good := completeSignals(`[`+supportedSignal+`]`, `[]`, `[]`, `[]`)
	for _, bad := range []string{
		`{"scope":"w1","signals":{"responsibilities":[],"interfaces":[],"uncertainty":[]}}`,
		`{"scope":"w1","signals":{"responsibilities":[],"interfaces":[],"launch_requirements":42,"uncertainty":[]}}`,
	} {
		raw := []byte(`{"left":` + string(good) + `,"right":` + bad + `}`)
		got, err := decodeOrientationContext(raw, signalInput())
		// The responsibilities difference independently conflicts. Interfaces and
		// uncertainty survive; launch is not filled from the other full answer.
		if err != nil || got.Questions["launch_requirements"] || !got.Questions["interfaces"] || got.permits("run_recipe") || got.permits("repository") {
			t.Fatalf("known question repaired across wrappers: %+v %v", got, err)
		}
	}
	raw := []byte(`{"scope":"w1","signals":{"responsibilities":[` + supportedSignal + `],"interfaces":[],"launch_requirements":[],"launch_requirements":42,"uncertainty":[]}}`)
	got, err := decodeOrientationContext(raw, signalInput())
	if err != nil || !got.permits("roles") || got.permits("run_recipe") || len(got.Observations) != 1 {
		t.Fatalf("smallest question dependency lost: %+v %v", got, err)
	}
	// A nested explicit owning scope cannot hide a malformed decision under a
	// valid outer wrapper; unaddressed metadata in the previous test is harmless.
	raw = []byte(`{"outer":` + string(good) + `,"nested":{"scope":"w1","signals":42}}`)
	if _, err := decodeOrientationContext(raw, signalInput()); err == nil {
		t.Fatal("known malformed nested scope hidden")
	}
}

func TestSignalsSeparateFullProvenanceFromSupportAndKeepEveryChosenNativeValue(t *testing.T) {
	input := signalInput()
	raw := completeSignals(`[{"basis":["d2"],"text":"Earlier grouping is interpreted.","status":"interpretation","supports":[]}]`, `[]`, `[`+supportedSignal+`,{"basis":["d3"],"text":"A distinct call follows.","status":"supported","supports":["d3"]}]`, `[]`)
	got, err := decodeOrientationContext(raw, input)
	if err != nil {
		t.Fatal(err)
	}
	if !sameOrientationRecords(input.Records, got.Provenance) || len(got.Observations) != 3 || len(got.Observations[0].Sources) != 0 || len(got.Observations[0].Native) != 0 {
		t.Fatalf("provenance promoted to native support: %+v", got)
	}
	cat := newCatalog()
	cat.targets["t1"] = "t1"
	for _, ref := range []string{"a1", "a2"} {
		cat.facts[ref] = factEntry{id: ref, kind: facts.KindManifest, byTarget: map[string]string{"t1": ref}}
		cat.evidence[ref] = map[string]any{"ref": ref, "value": "actual " + ref}
	}
	cat.subjects["t1.n1"] = subjectEntry{id: "n1", targetRef: "t1"}
	cat.evidence["t1.n1"] = map[string]any{"ref": "t1.n1", "symbol": "begin"}
	final, err := finalContextWire("fixture", []targetWire{{Ref: "t1"}}, got.forSection("run_recipe"), cat, "run_recipe")
	if err != nil {
		t.Fatal(err)
	}
	var body struct {
		Native       []orientationRecord    `json:"native_support"`
		Sources      []struct{ Ref string } `json:"citation_sources"`
		Observations []orientationObservation
	}
	if err := json.Unmarshal(final, &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Native) != 2 || body.Native[0].Ref != "s1" || body.Native[1].Ref != "s2" || len(body.Sources) != 3 {
		t.Fatalf("distinct uses lost: %s", final)
	}
	for i, wanted := range []orientationRecord{input.Records[0], input.Records[2]} {
		actual := body.Native[i]
		actual.Ref = wanted.Ref
		a, _ := encodeWire(actual)
		b, _ := encodeWire(wanted)
		if !bytes.Equal(a, b) {
			t.Fatalf("native call/related values clipped: %s / %s", a, b)
		}
	}
	if !slices.Equal(body.Observations[0].SupportRecords, []string{"s1"}) || !slices.Equal(body.Observations[1].SupportRecords, []string{"s2"}) {
		t.Fatal("chosen use associations lost")
	}
	global := contextSignalRecords([]struct {
		Target  targetWire
		Context orientationContext
	}{{input.Target, got}})
	native, interpretations := 0, 0
	for _, r := range global {
		if r.Layer == "native" {
			native++
		} else {
			interpretations++
			if len(r.Sources) != 0 {
				t.Fatal("global model signal inherits union")
			}
		}
	}
	if native != 2 || interpretations != 3 {
		t.Fatal("global did not retain all accepted signals and selected original support")
	}
	// A complete acknowledged empty window remains explicitly empty, not a
	// claim that other windows/targets lack work; the nonempty peer is kept.
	empty, err := decodeOrientationContext(completeSignals(`[]`, `[]`, `[]`, `[]`), input)
	if err != nil {
		t.Fatal(err)
	}
	emptyGlobal := contextSignalRecords([]struct {
		Target  targetWire
		Context orientationContext
	}{{input.Target, empty}, {input.Target, got}})
	if !reflect.DeepEqual(global, emptyGlobal) {
		t.Fatal("empty window establishes invented absence or erases peer")
	}
}

type signalFailureProvider struct {
	*partitionProvider
	phase string
	kind  llm.ResourceLimitKind
}

func (p *signalFailureProvider) Prepare(prompt llm.Prompt, bounds llm.Limits) (llm.Prepared, error) {
	var wire struct{ Task, Result string }
	_ = json.Unmarshal([]byte(prompt.User), &wire)
	matched := p.phase == "overview" && wire.Task == "" || p.phase == "context" && wire.Task == contextTask || p.phase == "final" && wire.Result == "roles"
	if matched {
		return llm.Prepared{}, llm.NewResourceLimitError(llm.ResourceLimitError{Kind: p.kind})
	}
	return p.partitionProvider.Prepare(prompt, bounds)
}
func TestOrientationPreparationOutputAndResponseFailuresRemainFatalAtEveryPhase(t *testing.T) {
	for _, phase := range []string{"overview", "context", "final"} {
		for _, kind := range []llm.ResourceLimitKind{llm.ResourceLimitOutputTokens, llm.ResourceLimitResponseBytes} {
			t.Run(phase+"/"+string(kind), func(t *testing.T) {
				f := newFixture(t)
				p := &signalFailureProvider{partitionProvider: &partitionProvider{t: t, refs: f.refs(t)}, phase: phase, kind: kind}
				_, _, err := Run(t.Context(), llm.Executor{}, p, f.input)
				var resource *llm.ResourceLimitError
				if !errors.As(err, &resource) || resource.Kind != kind {
					t.Fatalf("Prepare failure swallowed/refined: %v", err)
				}
				if phase != "final" && p.calls != 0 {
					t.Fatalf("Prepare failed but %d completions ran", p.calls)
				}
			})
		}
	}
}

// These are original complete ordinary request windows, not selected fragments
// or a fake corpus. All original records retain their order and values.
func TestSavedCompleteAirflowContextsKeepOriginalEvidenceAndPreparedEnvelopes(t *testing.T) {
	for _, fixture := range []struct {
		Name, SHA        string
		Records, Sources int
		ModelOnly        bool
	}{
		{"airflow-t29-original-context", "866c471eeac283326cc7a01fad9b9459fa17fb6817a3163dfcf5b3b42dd81ecc", 8950, 8944, false},
		{"airflow-t29-model-context", "5f9b0d7b24644170665470e0ec19b828e02b8ae41833279baff719db7bda8697", 34, 8944, true},
		{"airflow-t70-original-context", "f7d8964d312fbbac4ed093c9253c5df3f5966c25f70af51206f42beba162a7eb", 89, 88, false},
	} {
		t.Run(fixture.Name, func(t *testing.T) {
			f, err := os.Open(filepath.Join("testdata", fixture.Name+".json.gz"))
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			gz, err := gzip.NewReader(f)
			if err != nil {
				t.Fatal(err)
			}
			defer gz.Close()
			raw, err := io.ReadAll(gz)
			if err != nil {
				t.Fatal(err)
			}
			sum := sha256.Sum256(raw)
			if hex.EncodeToString(sum[:]) != fixture.SHA {
				t.Fatal("complete original saved input changed")
			}
			var saved struct{ Input orientationReading }
			if err := json.Unmarshal(raw, &saved); err != nil {
				t.Fatal(err)
			}
			input := saved.Input
			input.Task = contextTask
			refs := map[string]bool{}
			for _, r := range input.Records {
				for _, ref := range r.Sources {
					refs[ref] = true
				}
			}
			if len(input.Records) != fixture.Records || len(refs) != fixture.Sources {
				t.Fatal("complete original record/source inventory lost")
			}
			if fixture.ModelOnly {
				got, err := decodeOrientationContext(completeSignals(`[`+supportedSignal+`]`, `[]`, `[]`, `[]`), input)
				if err != nil || len(got.Observations) != 0 || len(got.Rejected) != 1 || !sameOrientationRecords(input.Records, got.Provenance) {
					t.Fatalf("model union became source evidence: %+v %v", got, err)
				}
			}
			// Fail closed if any HTTP path is invoked; only local preparation and a
			// request-bound preset can execute in this proof.
			httpCalls := 0
			client := &deepseek.Client{APIKey: "local-preset", Model: "deepseek-v4-flash", Endpoint: "https://api.deepseek.com/chat/completions", ContextTokens: 1000000, MaxTokens: llm.DefaultMaxOutputTokens, HTTPClient: &http.Client{Transport: signalNoHTTP{&httpCalls}}}
			p := &preparedPartitionProvider{partitionProvider: &partitionProvider{t: t}, client: client}
			fxt := newFixture(t)
			reading, rejected, err := readOrientationContext(t.Context(), llm.Executor{}, p, fxt.input, groupDigests(fxt.input.Groups), input.Target, input.Records)
			if err != nil || len(rejected) != 0 || !reading.permits("repository") || !sameOrientationRecords(reading.Provenance, input.Records) || !sameOrientationRecords(input.Records, p.seen[input.Target.Ref]) {
				t.Fatalf("complete native reader failure: %v rejected=%d originals=%d seen=%d", err, len(rejected), len(reading.Provenance), len(p.seen[input.Target.Ref]))
			}
			originals := map[string]orientationRecord{}
			for _, record := range input.Records {
				originals[record.Ref] = record
			}
			var chosen []orientationRecord
			chosenSeen := map[string]bool{}
			for _, obs := range reading.Observations {
				for _, r := range obs.Native {
					wanted, known := originals[r.Ref]
					a, _ := encodeWire(r)
					b, _ := encodeWire(wanted)
					if !known || !bytes.Equal(a, b) || r.Layer != "native" {
						t.Fatal("selected native record changed original value/source/order")
					}
					if !chosenSeen[r.Ref] {
						chosenSeen[r.Ref] = true
						chosen = append(chosen, r)
					}
				}
			}
			cat := catalogFromOriginalRecords(t, input)
			wire, err := finalContextWire("airflow", []targetWire{input.Target}, reading.forSection("roles"), cat, "roles")
			if err != nil {
				t.Fatal(err)
			}
			if err := requestFits(p, llm.Prompt{System: sectionPrompt("roles"), User: string(wire), ResponseFormatJSON: true, ResponseExample: sectionExample("roles")}); err != nil {
				t.Fatalf("selected complete native support final floor: %v", err)
			}
			var final struct {
				Native  []orientationRecord `json:"native_support"`
				Sources []struct {
					Ref, Kind string
					Targets   []string
					Evidence  json.RawMessage
				} `json:"citation_sources"`
			}
			if err := json.Unmarshal(wire, &final); err != nil {
				t.Fatal(err)
			}
			if len(final.Native) != len(chosen) {
				t.Fatal("selected original native values not complete in final")
			}
			for i, record := range final.Native {
				record.Ref = chosen[i].Ref
				a, _ := encodeWire(record)
				b, _ := encodeWire(chosen[i])
				if !bytes.Equal(a, b) {
					t.Fatal("final selected native original value/order altered")
				}
			}
			for _, source := range final.Sources {
				evidence, known := cat.evidence[source.Ref]
				if !known {
					t.Fatal("final invented source metadata")
				}
				expected, _ := encodeWire(evidence)
				if !bytes.Equal(source.Evidence, expected) {
					t.Fatal("native citation original evidence lost")
				}
				if entry, ok := cat.facts[source.Ref]; ok {
					kind := string(entry.kind)
					if entry.export {
						kind = exportKind
					}
					if source.Kind != kind {
						t.Fatal("native source kind invented")
					}
				}
			}
			if httpCalls != 0 {
				t.Fatal("probe invoked HTTP")
			}
			t.Logf("original_records=%d provenance_sources=%d accepted_signal_rows=%d native_selected_final_bytes=%d completed_read_leaves=%d HTTP=%d", len(input.Records), len(refs), len(reading.Observations), len(wire), p.calls, httpCalls)
		})
	}
}

type signalNoHTTP struct{ calls *int }

func (s signalNoHTTP) RoundTrip(*http.Request) (*http.Response, error) {
	*s.calls++
	return nil, fmt.Errorf("HTTP forbidden in saved complete window probe")
}

func TestRepeatedMalformedSignalExtraMetadataDoesNotPoisonIndependentQuestion(t *testing.T) {
	bad := `{"basis":["d1"],"text":42,"status":"supported","supports":["d1"]}`
	extra := `{"basis":["d1"],"text":42,"status":"supported","supports":["d1"],"metadata":{"note":"irrelevant"}}`
	left := completeSignals(`[`+supportedSignal+`,`+bad+`]`, `[]`, `[]`, `[]`)
	right := completeSignals(`[`+supportedSignal+`,`+extra+`]`, `[]`, `[]`, `[]`)
	got, err := decodeOrientationContext([]byte(`{"left":`+string(left)+`,"right":`+string(right)+`}`), signalInput())
	if err != nil || !got.permits("roles") || len(got.Observations) != 1 || len(got.Rejected) != 2 {
		t.Fatalf("invalid raw metadata became question conflict: %+v %v", got, err)
	}
}

func TestNestedFinalKnownConflictsCannotHideBehindDirectAnswer(t *testing.T) {
	f := newFixture(t)
	_, cat, _ := buildOverview(f.input)
	refs := f.refs(t)
	first := fmt.Sprintf(`{"roles":[{"target":%q,"role":"First","purpose":"First work.","refs":[%q]},{"target":%q,"role":"Peer","purpose":"Peer work.","refs":[%q]}]}`, refs.target("alpha"), refs.fact("entrypoint"), refs.target("beta"), refs.subject("beta", "inbound"))
	second := fmt.Sprintf(`{"roles":[{"target":%q,"role":"Other","purpose":"Different work.","refs":[%q]}]}`, refs.target("alpha"), refs.fact("entrypoint"))
	for _, pair := range [][2]string{{first, second}, {second, first}} {
		var root map[string]json.RawMessage
		_ = json.Unmarshal([]byte(pair[0]), &root)
		root["result"] = json.RawMessage(pair[1])
		raw, _ := encodeWire(root)
		got, err := normalizeSection(raw, cat, "roles")
		if err != nil || len(got.roles) != 1 || got.roles[0].TargetID != f.targetID("beta") {
			t.Fatalf("direct wrapper hid known conflict: %+v %v", got, err)
		}
	}
	for _, raw := range []string{`[` + first + `]`, `{"results":[` + first + `,42,{"metadata":"extra"}]}`} {
		got, err := normalizeSection([]byte(raw), cat, "roles")
		if err != nil || len(got.roles) != 2 || len(got.rejected) != 0 {
			t.Fatalf("harmless array wrapper refused: %+v %v", got, err)
		}
	}
	for _, raw := range []string{`[` + first + `,` + second + `]`, `{"results":[` + first + `,` + second + `]}`} {
		got, err := normalizeSection([]byte(raw), cat, "roles")
		if err != nil || len(got.roles) != 1 || got.roles[0].TargetID != f.targetID("beta") {
			t.Fatalf("array wrapper hides conflicting known role: %+v %v", got, err)
		}
	}
	good := fmt.Sprintf(`{"summary":"One useful summary.","summary_refs":[%q],"main_flow_target":%q}`, refs.fact("entrypoint"), refs.target("alpha"))
	other := fmt.Sprintf(`{"summary":"Another useful summary.","summary_refs":[%q],"main_flow_target":%q}`, refs.fact("entrypoint"), refs.target("alpha"))
	for _, pair := range [][2]string{{good, other}, {other, good}} {
		var root map[string]json.RawMessage
		_ = json.Unmarshal([]byte(pair[0]), &root)
		root["result"] = json.RawMessage(pair[1])
		raw, _ := encodeWire(root)
		got, err := normalizeSection(raw, cat, "repository")
		if err != nil || got.summary != "" || got.flowTarget != f.targetID("alpha") {
			t.Fatalf("summary conflict hidden or independent target lost: %+v %v", got, err)
		}
	}
	var root map[string]json.RawMessage
	_ = json.Unmarshal([]byte(first), &root)
	root["extra"] = json.RawMessage(`{"roles":[{"target":"outside","role":42,"refs":["outside"]}],"metadata":{"note":"harmless"}}`)
	raw, _ := encodeWire(root)
	got, err := normalizeSection(raw, cat, "roles")
	if err != nil || len(got.roles) != 2 || len(got.rejected) != 0 {
		t.Fatalf("unknown wrapper poisoned known rows: %+v %v", got, err)
	}
}

type emptySignalProvider struct{ *partitionProvider }

func (p *emptySignalProvider) Complete(ctx context.Context, prepared llm.Prepared) (llm.Completion, error) {
	var wire struct{ Task, Result string }
	_ = json.Unmarshal(prepared.Bytes(), &wire)
	if wire.Result == "roles" || wire.Result == "run_recipe" {
		p.calls++
		raw, _ := encodeWire(map[string]any{wire.Result: []any{}})
		return llm.Completion{Response: raw, FinishReason: llm.FinishStop, ChoiceCount: 1}, nil
	}
	if wire.Task == contextTask {
		p.calls++
		return llm.Completion{Response: completeSignals(`[]`, `[]`, `[]`, `[]`), FinishReason: llm.FinishStop, ChoiceCount: 1}, nil
	}
	return p.partitionProvider.Complete(ctx, prepared)
}
func TestAcknowledgedEmptyQuestionsRemainExplicitWithoutEmptyGlobalFatal(t *testing.T) {
	f := newFixture(t)
	p := &emptySignalProvider{&partitionProvider{t: t, refs: f.refs(t)}}
	result, rejected, err := Run(t.Context(), llm.Executor{}, p, f.input)
	if err != nil || result.Summary != "" || len(result.Roles) != 0 || len(rejected) == 0 {
		t.Fatalf("empty local signal became fatal/global absence: %+v %+v %v", result, rejected, err)
	}
	if !slices.ContainsFunc(rejected, func(row RejectedRow) bool {
		return row.Section == "repository" && strings.Contains(row.Reason, "without selected")
	}) {
		t.Fatal("global missing interpretation not explicit")
	}
}

func TestFinalCitationKnownOriginalButUnselectedRemainsClosed(t *testing.T) {
	f := newFixture(t)
	wire, cat, _ := buildOverview(f.input)
	refs := f.refs(t)
	observation := orientationObservation{Text: "Selected actual source.", Sources: []string{refs.fact("entrypoint")}}
	local := contextCatalog(cat, wire.Targets[:1], []orientationObservation{observation})
	raw := fmt.Sprintf(`{"roles":[{"target":%q,"role":"Backend","purpose":"Serves work.","refs":[%q]}]}`, wire.Targets[0].Ref, refs.fact("route"))
	got, err := normalizeSection([]byte(raw), local, "roles")
	if err == nil || len(got.roles) != 0 {
		t.Fatalf("known but unadvertised source promoted: %+v %v", got, err)
	}
}

func TestRepeatedSignalOrderIsHarmlessAndFirstOrderIsPreserved(t *testing.T) {
	second := `{"basis":["d3"],"text":"Another observed call.","status":"supported","supports":["d3"]}`
	left := completeSignals(`[`+supportedSignal+`,`+second+`]`, `[]`, `[]`, `[]`)
	right := completeSignals(`[`+second+`,`+supportedSignal+`]`, `[]`, `[]`, `[]`)
	got, err := decodeOrientationContext([]byte(`{"left":`+string(left)+`,"right":`+string(right)+`}`), signalInput())
	if err != nil || !got.permits("repository") || len(got.Rejected) != 0 || len(got.Observations) != 2 || got.Observations[0].SupportRecords[0] != "d1" || got.Observations[1].SupportRecords[0] != "d3" {
		t.Fatalf("presentation order becomes whole-question conflict: %+v %v", got, err)
	}
}

// Reconstruct only the original advertised native source metadata and values.
// This helper never guesses kind/owner from a ref prefix or manufactures facts.
func catalogFromOriginalRecords(t *testing.T, input orientationReading) catalog {
	t.Helper()
	cat := newCatalog()
	cat.targets[input.Target.Ref] = input.Target.Ref
	for _, record := range input.Records {
		for _, fact := range nativeRecordFacts(record) {
			entry := factEntry{id: fact.Ref, kind: facts.Kind(fact.Kind), export: fact.Kind == exportKind, byTarget: map[string]string{}}
			for _, target := range fact.Targets {
				entry.byTarget[target] = fact.Ref
			}
			cat.facts[fact.Ref] = entry
			cat.evidence[fact.Ref] = fact
		}
		if record.Layer != "native" {
			continue
		}
		var header memberRow
		raw, err := encodeWire(record.Value)
		if err != nil {
			t.Fatal(err)
		}
		switch record.Kind {
		case "seed":
			err = json.Unmarshal(raw, &header)
		case "seed_call", "seed_evidence":
			var value struct{ Seed memberRow }
			err = json.Unmarshal(raw, &value)
			header = value.Seed
		default:
			continue
		}
		if err != nil || header.Ref == "" {
			t.Fatal("original native seed header missing")
		}
		target, _, ok := strings.Cut(header.Ref, ".")
		if !ok {
			t.Fatal("original seed ownership missing")
		}
		cat.subjects[header.Ref] = subjectEntry{id: header.Ref, targetRef: target}
		cat.evidence[header.Ref] = header
	}
	return cat
}

func TestSelectedNativeRelatedManifestRemainsUsableWithoutLinksOnlyPromotion(t *testing.T) {
	primary := factWire{Ref: "a1", Kind: string(facts.KindConfigRead), Targets: []string{"t1"}, Value: "CONFIG", Links: []string{"a2", "a99"}}
	related := factWire{Ref: "a2", Kind: string(facts.KindManifest), Targets: []string{"t1"}, Anchor: "Makefile:22", Value: "make run INPUT=<input>"}
	input := orientationReading{Task: contextTask, Target: targetWire{Ref: "t1"}, Records: []orientationRecord{{Ref: "d1", Layer: "native", Kind: "fact", Value: struct {
		Fact    factWire   `json:"fact"`
		Related []factWire `json:"related_facts"`
	}{primary, []factWire{related}}, Sources: []string{"a1"}}}}
	got, err := decodeOrientationContext(completeSignals(`[]`, `[]`, `[`+supportedSignal+`]`, `[]`), input)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got.Observations[0].Sources, []string{"a1", "a2"}) || !slices.Equal(got.Observations[0].Native[0].Sources, []string{"a1"}) {
		t.Fatal("native sources mutated or related source unavailable")
	}
	cat := catalogFromOriginalRecords(t, input)
	local := contextCatalog(cat, []targetWire{input.Target}, got.Observations)
	raw := []byte(`{"run_recipe":[{"target":"t1","command":"make run INPUT=<input>","cwd":".","note":"Supply INPUT.","refs":["a2"]}]}`)
	normalized, err := normalizeSection(raw, local, "run_recipe")
	if err != nil || len(normalized.recipe) != 1 {
		t.Fatalf("actual related launch source unavailable: %+v %v", normalized, err)
	}
	if _, exists := local.facts["a99"]; exists {
		t.Fatal("link-only value became native evidence")
	}
	final, err := finalContextWire("fixture", []targetWire{input.Target}, got.Observations, cat, "run_recipe")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(final, []byte(`"kind":"manifest"`)) || !bytes.Contains(final, []byte(`"anchor":"Makefile:22"`)) {
		t.Fatal("actual kind/source value lost")
	}
}

func TestActualInitialOutputRefusalCannotSoftenLaterPreparationFailure(t *testing.T) {
	for _, phase := range []string{"context", "final"} {
		for _, kind := range []llm.ResourceLimitKind{llm.ResourceLimitOutputTokens, llm.ResourceLimitResponseBytes} {
			t.Run(phase+"/"+string(kind), func(t *testing.T) {
				f := newFixture(t)
				p := &signalFailureProvider{partitionProvider: &partitionProvider{t: t, refs: f.refs(t), outputRefusal: true, finishLength: true}, phase: phase, kind: kind}
				_, _, err := Run(t.Context(), llm.Executor{}, p, f.input)
				var resource *llm.ResourceLimitError
				if !errors.As(err, &resource) || resource.Kind != kind {
					t.Fatalf("stale initial completion proof softened new Prepare error: %v", err)
				}
				if phase == "context" && p.calls != 1 {
					t.Fatalf("failed context phase transported %d calls after initial", p.calls-1)
				}
				if phase == "final" && p.calls != 2 {
					t.Fatalf("failed final phase transported or read next target: %d", p.calls)
				}
			})
		}
	}
}

type partialSignalProvider struct {
	*partitionProvider
	response []byte
}

func (p *partialSignalProvider) Complete(_ context.Context, _ llm.Prepared) (llm.Completion, error) {
	p.calls++
	return llm.Completion{Response: p.response, FinishReason: llm.FinishStop, ChoiceCount: 1}, nil
}
func TestSignalRowRefusalKeepsExactLiveWarmResponseAfterCacheClear(t *testing.T) {
	input := signalInput()
	f := newFixture(t)
	bad := `{"basis":["d1"],"text":42,"status":"supported","supports":["d1"]}`
	raw := completeSignals(`[`+bad+`,`+supportedSignal+`]`, `[]`, `[]`, `[]`)
	p := &partialSignalProvider{partitionProvider: &partitionProvider{}, response: raw}
	root := t.TempDir()
	writer, err := debugdump.NewWriter(root, "orientation")
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	executor := debugdump.BindStage(llm.Executor{RootDir: root, Enabled: true, Observer: debugdump.NewSemanticObserver(writer)}, StageName)
	for range 2 {
		value, rejected, err := readOrientationContext(t.Context(), executor, p, f.input, groupDigests(f.input.Groups), input.Target, input.Records)
		if err != nil || len(rejected) != 1 || len(value.Observations) != 1 || !value.permits("repository") || !bytes.Equal(rejected[0].Raw, []byte(bad)) {
			t.Fatalf("partial exact response/independent signal lost: %+v %+v %v", value, rejected, err)
		}
	}
	if p.calls != 1 {
		t.Fatalf("warm context did not use exact accepted request: %d", p.calls)
	}
	if err := os.RemoveAll(filepath.Join(root, llm.CacheDirectoryName)); err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(writer.BaseDir, writer.RunID)
	journals, err := filepath.Glob(filepath.Join(runDir, debugdump.SemanticExchangesDir, "*", debugdump.SemanticExchangeMetaFile))
	if err != nil || len(journals) != 2 {
		t.Fatal("partial exchange journals lost")
	}
	for _, journal := range journals {
		body, err := os.ReadFile(journal)
		if err != nil {
			t.Fatal(err)
		}
		var record debugdump.SemanticExchangeRecord
		if err := json.Unmarshal(body, &record); err != nil {
			t.Fatal(err)
		}
		saved, err := os.ReadFile(filepath.Join(filepath.Dir(journal), record.Response.File))
		if err != nil || !bytes.Equal(saved, raw) {
			t.Fatalf("cache clear erased exact rejected payload: %v", err)
		}
	}
	rows, err := os.ReadFile(filepath.Join(runDir, "rejected.jsonl"))
	if err != nil || !bytes.Contains(rows, []byte(`"response_ref"`)) {
		t.Fatalf("partial response pointer missing: %v", err)
	}
}

func TestRefusedSignalGapsReachFinalAndGlobalWithoutBecomingClaimsOrProof(t *testing.T) {
	input := signalInput()
	invalid := `{"basis":["d1"],"text":42,"status":"supported","supports":["d1"]}`
	valid := `{"basis":["d3"],"text":"The supplied call is interpreted.","status":"interpretation","supports":[]}`
	got, err := decodeOrientationContext(completeSignals(`[]`, `[]`, `[`+invalid+`,`+valid+`]`, `[]`), input)
	if err != nil || !got.permits("run_recipe") || len(got.Observations) != 1 || len(got.Gaps) != 1 {
		t.Fatalf("partial gap incorrectly repaired or wholequestion refused: %+v %v", got, err)
	}
	gap := got.Gaps[0]
	if gap.Question != "launch_requirements" || gap.Target != "t1" || gap.Scope.First != "d1" || gap.Scope.Last != "d3" || !reflect.DeepEqual(gap.Basis, []orientationBasis{{"d1", "native"}}) || !reflect.DeepEqual(gap.Supports, []orientationBasis{{"d1", "native"}}) {
		t.Fatalf("original closed refusal address lost: %+v", gap)
	}
	cat := newCatalog()
	cat.targets["t1"] = "t1"
	wire, err := finalContextWire("fixture", []targetWire{input.Target}, got.forSection("run_recipe"), cat, "run_recipe", got.gapsForSection("run_recipe")...)
	if err != nil {
		t.Fatal(err)
	}
	var final struct {
		Gaps    []orientationGap    `json:"reader_gaps"`
		Native  []orientationRecord `json:"native_support"`
		Sources []json.RawMessage   `json:"citation_sources"`
	}
	if err := json.Unmarshal(wire, &final); err != nil {
		t.Fatal(err)
	}
	if len(final.Gaps) != 1 || len(final.Native) != 0 || len(final.Sources) != 0 || !reflect.DeepEqual(final.Gaps[0], gap) {
		t.Fatalf("gap became supportingclaim or disappeared: %s", wire)
	}
	if len(got.gapsForSection("roles")) != 0 {
		t.Fatal("launch-only gap overwrites independent role question")
	}
	global := contextSignalRecords([]struct {
		Target  targetWire
		Context orientationContext
	}{{input.Target, got}})
	if len(global) != 2 || global[0].Kind != "reader_gaps" || global[0].Layer != "model_interpretation" || len(global[0].Sources) != 0 {
		t.Fatal("global silently loses gap or promotes its attemptedsource")
	}
	reread, err := decodeOrientationContext(completeSignals(`[{"basis":["d1"],"text":"The gap proves a command.","status":"supported","supports":["d1"]}]`, `[]`, `[]`, `[]`), orientationReading{Task: contextTask, Records: global})
	if err != nil || len(reread.Observations) != 0 || len(reread.Rejected) != 1 {
		t.Fatalf("a refused source address became native proof globally: %+v %v", reread, err)
	}
}

func TestMalformedRefArraysRetainKnownAddressOnlyForRefusal(t *testing.T) {
	for _, bad := range []string{
		`{"basis":["d1",42],"text":"Attempted work.","status":"interpretation","supports":[]}`,
		`{"basis":["outside"],"text":"Attempted work.","status":"supported","supports":[42,"d1"]}`,
	} {
		got, err := decodeOrientationContext(completeSignals(`[`+bad+`,`+supportedSignal+`]`, `[]`, `[]`, `[]`), signalInput())
		if err != nil || len(got.Observations) != 1 || len(got.Rejected) != 1 || len(got.Gaps) != 1 || len(got.Gaps[0].Basis)+len(got.Gaps[0].Supports) != 1 {
			t.Fatalf("malformed known ref list silently disappeared/repaired: %+v %v", got, err)
		}
	}
	unknown := `{"basis":["outside",42],"text":42,"supports":["outside",42]}`
	got, err := decodeOrientationContext(completeSignals(`[`+unknown+`,`+supportedSignal+`]`, `[]`, `[]`, `[]`), signalInput())
	if err != nil || len(got.Observations) != 1 || len(got.Rejected) != 0 {
		t.Fatalf("unknown-only malformed metadata poisons neighbour: %+v %v", got, err)
	}
}

type carryGapProvider struct {
	*partitionProvider
	alpha       string
	originalGap orientationGap
	checked     bool
}

func (p *carryGapProvider) Complete(ctx context.Context, prepared llm.Prepared) (llm.Completion, error) {
	var wire struct {
		Task   string
		Target targetWire
		Result string
		Gaps   []orientationGap `json:"reader_gaps"`
	}
	if err := json.Unmarshal(prepared.Bytes(), &wire); err != nil {
		return llm.Completion{}, err
	}
	if wire.Result == "repository" {
		p.checked = true
		if len(wire.Gaps) != 1 || !reflect.DeepEqual(wire.Gaps[0], p.originalGap) {
			return llm.Completion{}, fmt.Errorf("original target gap lost/altered in final repository decision")
		}
		return llm.Completion{Response: []byte(`{"summary":"","summary_refs":[],"main_flow_target":""}`), FinishReason: llm.FinishStop, ChoiceCount: 1}, nil
	}
	completion, err := p.partitionProvider.Complete(ctx, prepared)
	if err != nil || wire.Task != contextTask {
		return completion, err
	}
	if wire.Target.Ref == "" {
		completion.Response = completeSignals(`[]`, `[]`, `[]`, `[]`)
		return completion, nil
	}
	if wire.Target.Ref == p.alpha {
		_, records := expandContextWire(p.t, prepared.Bytes())
		ref := ""
		for _, record := range records {
			if record.Layer == "native" && len(record.Sources) > 0 {
				ref = record.Ref
				break
			}
		}
		if ref == "" {
			return llm.Completion{}, fmt.Errorf("missing original source in real fixture")
		}
		var response struct {
			Scope   string                       `json:"scope"`
			Signals map[string][]json.RawMessage `json:"signals"`
		}
		if err := json.Unmarshal(completion.Response, &response); err != nil {
			return llm.Completion{}, err
		}
		bad := json.RawMessage(fmt.Sprintf(`{"basis":[%q],"text":42,"status":"supported","supports":[%q]}`, ref, ref))
		response.Signals["responsibilities"] = append(response.Signals["responsibilities"], bad)
		completion.Response, _ = encodeWire(response)
		result, err := decodeOrientationContext(completion.Response, orientationReading{Task: contextTask, Target: wire.Target, Records: records})
		if err != nil || len(result.Gaps) != 1 {
			return llm.Completion{}, fmt.Errorf("fixture partial gap failed")
		}
		p.originalGap = result.Gaps[0]
	}
	return completion, nil
}
func TestOriginalTargetGapSurvivesAcceptedEmptyGlobalInterpretation(t *testing.T) {
	f := newFixture(t)
	refs := f.refs(t)
	p := &carryGapProvider{partitionProvider: &partitionProvider{t: t, refs: refs}, alpha: refs.target("alpha")}
	result, rejected, err := Run(t.Context(), llm.Executor{}, p, f.input)
	if err != nil || len(result.Roles) != 2 || !p.checked || len(rejected) != 1 || result.Summary != "" {
		t.Fatalf("global omission erased original gap or independent role: %+v %+v %v checked=%v", result, rejected, err, p.checked)
	}
}

func TestCopiedNativeSupportObjectsCannotReplaceClosedSourceChoices(t *testing.T) {
	input := signalInput()
	copied, err := encodeWire(input.Records[0])
	if err != nil {
		t.Fatal(err)
	}
	bad := `{"basis":["d1"],"text":"Copied source is not a choice.","status":"supported","supports":[` + string(copied) + `]}`
	for _, rows := range []string{bad + `,` + supportedSignal, supportedSignal + `,` + bad} {
		got, err := decodeOrientationContext(completeSignals(`[`+rows+`]`, `[]`, `[]`, `[]`), input)
		if err != nil || len(got.Rejected) != 1 || len(got.Observations) != 1 || !got.permits("roles") {
			t.Fatalf("copied object repaired or neighbour lost: %+v %v", got, err)
		}
		if !reflect.DeepEqual(got.Observations[0].Native, input.Records[:1]) || !slices.Equal(got.Observations[0].Sources, input.Records[0].Sources) {
			t.Fatal("closed ref did not retain the exact original native value and sources")
		}
	}
}

// This preset supplies legitimate interpreted signals without native support
// for beta. Preparing its role would be a product bug, not a provider refusal.
type missingOwnCitationProvider struct {
	*partitionProvider
	rolePreparations []string
}

func (p *missingOwnCitationProvider) Prepare(prompt llm.Prompt, limits llm.Limits) (llm.Prepared, error) {
	var wire struct {
		Result  string
		Targets []targetWire
	}
	_ = json.Unmarshal([]byte(prompt.User), &wire)
	if wire.Result == "roles" {
		p.rolePreparations = append(p.rolePreparations, wire.Targets[0].Ref)
		if wire.Targets[0].Ref == p.refs.target("beta") {
			return llm.Prepared{}, fmt.Errorf("role with no closed own citation reached provider preparation")
		}
	}
	return p.partitionProvider.Prepare(prompt, limits)
}
func (p *missingOwnCitationProvider) Complete(ctx context.Context, prepared llm.Prepared) (llm.Completion, error) {
	var wire struct {
		Task    string
		Target  targetWire
		Records []orientationRecord
	}
	_ = json.Unmarshal(prepared.Bytes(), &wire)
	if wire.Task == contextTask && wire.Target.Ref == p.refs.target("beta") {
		_, records := expandContextWire(p.t, prepared.Bytes())
		if p.seen == nil {
			p.seen = map[string][]orientationRecord{}
		}
		p.seen[wire.Target.Ref] = append(p.seen[wire.Target.Ref], records...)
		row := fmt.Sprintf(`[{"basis":[%q],"text":"Interpreted work remains unproved.","status":"interpretation","supports":[]}]`, records[0].Ref)
		return llm.Completion{Response: completeSignals(row, row, row, row), FinishReason: llm.FinishStop, ChoiceCount: 1}, nil
	}
	return p.partitionProvider.Complete(ctx, prepared)
}
func TestMissingClosedOwnCitationsRefusesOnlyRoleBeforePreparation(t *testing.T) {
	f := newFixture(t)
	p := &missingOwnCitationProvider{partitionProvider: &partitionProvider{t: t, refs: f.refs(t)}}
	result, rejected, err := Run(t.Context(), llm.Executor{}, p, f.input)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Roles) != 1 || result.Roles[0].TargetID != f.targetID("alpha") || len(result.RunRecipe) != 1 || result.Summary == "" {
		t.Fatalf("missing beta citation erased independent work: %+v %+v", result, rejected)
	}
	if slices.Contains(p.rolePreparations, p.refs.target("beta")) {
		t.Fatal("unsupported role reached provider")
	}
	if !slices.ContainsFunc(rejected, func(row RejectedRow) bool {
		return row.Section == "roles" && strings.Contains(row.Reason, "required own source citations unavailable") && strings.Contains(string(row.Raw), p.refs.target("beta"))
	}) {
		t.Fatalf("smallest source refusal missing: %+v", rejected)
	}
}
func TestRoleCitationAvailabilityKeepsOriginalEvidenceObligationAndOwnership(t *testing.T) {
	original := newCatalog()
	original.facts["a1"] = factEntry{byTarget: map[string]string{"t1": "a1"}}
	selected := contextCatalog(original, []targetWire{{Ref: "t1"}}, nil)
	if !roleCitationUnavailable(original, selected, "t1") {
		t.Fatal("lost original evidence obligation")
	}
	selected.subjects["t2.n1"] = subjectEntry{targetRef: "t2"}
	selected.facts["a2"] = factEntry{byTarget: map[string]string{}}
	if !roleCitationUnavailable(original, selected, "t1") {
		t.Fatal("borrowed another target or repository-wide source")
	}
	selected.facts["a1"] = original.facts["a1"]
	if roleCitationUnavailable(original, selected, "t1") {
		t.Fatal("valid own native choice unavailable")
	}
	delete(selected.facts, "a1")
	selected.subjects["t1.n1"] = subjectEntry{targetRef: "t1"}
	if roleCitationUnavailable(original, selected, "t1") {
		t.Fatal("valid own native seed unavailable")
	}
	if roleCitationUnavailable(newCatalog(), newCatalog(), "t3") {
		t.Fatal("invented evidence obligation for target with no original fact or seed")
	}
}
func TestFinalSectionPromptsAdvertiseOnlyTheirResponseShape(t *testing.T) {
	for _, section := range []string{"roles", "run_recipe", "repository"} {
		prompt := sectionPrompt(section)
		var shape map[string]json.RawMessage
		parts := strings.Split(prompt, "```json\n")
		if len(parts) != 2 {
			t.Fatalf("%s final prompt has competing response examples", section)
		}
		shapeText := strings.Split(parts[1], "\n```")[0]
		if err := json.Unmarshal([]byte(shapeText), &shape); err != nil {
			t.Fatal(err)
		}
		for _, other := range []string{"roles", "run_recipe", "summary"} {
			_, present := shape[other]
			wanted := other == section || section == "repository" && other == "summary"
			if present != wanted {
				t.Fatalf("%s advertises competing %s decision", section, other)
			}
		}
		if !strings.Contains(prompt, "d* context `basis`") || !strings.Contains(prompt, "citation_sources") {
			t.Fatal("final citation vocabulary is ambiguous")
		}
	}
}
