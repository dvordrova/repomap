package orientation

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/llm"
)

type partitionProvider struct {
	t                     *testing.T
	mu                    sync.Mutex
	bound                 int
	maxRecords            int
	refuseTarget          string
	refuseSection         string
	outputRefusal         bool
	recipeNeedsSingle     bool
	recipeAlwaysOversized bool
	refuseRefinement      bool
	finishLength          bool
	seen                  map[string][]orientationRecord
	calls                 int
	refs                  refLookup
}

// Use the ordinary provider encoder and byte reservation; only completion is
// replaced by a closed request-bound preset. There is no sizing HTTP call.
type preparedPartitionProvider struct {
	*partitionProvider
	client *deepseek.Client
}

func (p *preparedPartitionProvider) Prepare(prompt llm.Prompt, limits llm.Limits) (llm.Prepared, error) {
	return p.client.Prepare(prompt, limits)
}
func (p *preparedPartitionProvider) Complete(ctx context.Context, prepared llm.Prepared) (llm.Completion, error) {
	var body struct {
		MaxTokens int `json:"max_tokens"`
		Messages  []struct{ Role, Content string }
	}
	if err := json.Unmarshal(prepared.Bytes(), &body); err != nil {
		return llm.Completion{}, err
	}
	for _, message := range body.Messages {
		if message.Role == "user" {
			var wire struct{ Task string }
			_ = json.Unmarshal([]byte(message.Content), &wire)
			if wire.Task == contextTask && body.MaxTokens != min(llm.DefaultMaxOutputTokens, p.client.MaxTokens) {
				return llm.Completion{}, fmt.Errorf("complete context inherited short final output allowance: %d", body.MaxTokens)
			}
			local, err := llm.NewPrepared([]byte(message.Content))
			if err != nil {
				return llm.Completion{}, err
			}
			return p.partitionProvider.Complete(ctx, local)
		}
	}
	return llm.Completion{}, fmt.Errorf("no prepared user message")
}

func TestFinalContextCitationsKeepNaturalTargetOrder(t *testing.T) {
	cat := newCatalog()
	cat.facts["a1"] = factEntry{kind: facts.Kind("manifest"), byTarget: map[string]string{"t10": "a1", "t2": "a1", "t1": "a1"}}
	wire, err := finalContextWire("fixture", nil, []orientationObservation{{Text: "Shared build evidence.", Sources: []string{"a1"}}}, cat, "repository")
	if err != nil {
		t.Fatal(err)
	}
	var body struct {
		Sources []struct{ Targets []string } `json:"citation_sources"`
	}
	if err := json.Unmarshal(wire, &body); err != nil || len(body.Sources) != 1 || !slices.Equal(body.Sources[0].Targets, []string{"t1", "t2", "t10"}) {
		t.Fatalf("citation target order: %s / %v", wire, err)
	}
}

func TestFinalContextCitationsKeepFirstUseAndCompleteEvidence(t *testing.T) {
	cat := newCatalog()
	var sources, wanted []string
	for i := 600; i > 0; i-- {
		ref := fmt.Sprintf("a%d", i)
		cat.facts[ref] = factEntry{kind: facts.KindManifest, export: i == 600, byTarget: map[string]string{"t10": ref, "t2": ref, "t1": ref}}
		cat.evidence[ref] = map[string]any{"ref": ref, "value": "source-specific command " + ref, "anchor": fmt.Sprintf("Makefile:%d", i)}
		sources = append(sources, ref, "unknown", ref)
		wanted = append(wanted, ref)
	}
	cat.subjects["t2.n1"] = subjectEntry{id: "n1", targetRef: "t2"}
	cat.evidence["t2.n1"] = map[string]any{"ref": "t2.n1", "name": "run", "anchor": "main.py:8"}
	wanted = append(wanted, "t2.n1")
	observations := []orientationObservation{{Text: "Complete first reading.", Sources: sources}, {Text: "Second reading.", Sources: []string{"unknown", "a1", "t2.n1", "a600", "t2.n1"}}}
	wire, err := finalContextWire("fixture", nil, observations, cat, "roles")
	if err != nil {
		t.Fatal(err)
	}
	var body struct {
		Observations []orientationObservation `json:"observations"`
		Sources      []struct {
			Ref      string         `json:"ref"`
			Kind     string         `json:"kind"`
			Targets  []string       `json:"targets"`
			Evidence map[string]any `json:"evidence"`
		} `json:"citation_sources"`
	}
	if err := json.Unmarshal(wire, &body); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(body.Observations, observations) || len(body.Sources) != len(wanted) {
		t.Fatal("original observations or complete source inventory changed")
	}
	for i, row := range body.Sources {
		kind, targets := string(facts.KindManifest), []string{"t1", "t2", "t10"}
		if i == 0 {
			kind = exportKind
		}
		if i == len(wanted)-1 {
			kind, targets = "seed", []string{"t2"}
		}
		if row.Ref != wanted[i] || row.Kind != kind || !slices.Equal(row.Targets, targets) || !reflect.DeepEqual(row.Evidence, cat.evidence[wanted[i]]) {
			t.Fatalf("first-use citation %d lost original value/owner/order: %+v", i, row)
		}
	}
}

func TestFinalContextCitationsRetainDistinctCompleteNativeCommands(t *testing.T) {
	root := "/private/tmp/fixture-checkout"
	builder := newRequestBuilder(Input{RepositoryRoot: root})
	command := "cd " + root + " && ./configure " + strings.Repeat("--enable-feature ", 12)
	first := builder.factWire(facts.Fact{ID: "a1", Kind: facts.KindManifest, Key: "AS_AUTORECONFIG", Value: command, Anchor: &facts.Anchor{Path: "Makefile", Line: 8}})
	second := builder.factWire(facts.Fact{ID: "a2", Kind: facts.KindManifest, Key: "clean", Value: "rm -f sqlite3", Anchor: &facts.Anchor{Path: "Makefile", Line: 16}})
	cat := newCatalog()
	for _, row := range []factWire{first, second} {
		cat.facts[row.Ref] = factEntry{id: row.Ref, kind: facts.KindManifest, byTarget: map[string]string{"t1": row.Ref}}
	}
	bindContextEvidence(cat, overviewRequest{Facts: []factWire{first, second}})
	wire, err := finalContextWire("fixture", []targetWire{{Ref: "t1"}}, []orientationObservation{{Text: "Configure before building; cleaning is a separate task.", Sources: []string{"a1", "a2"}}}, cat, "run_recipe")
	if err != nil {
		t.Fatal(err)
	}
	var body struct {
		Sources []struct {
			Ref      string
			Evidence factWire
		} `json:"citation_sources"`
	}
	if err := json.Unmarshal(wire, &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Sources) != 2 || body.Sources[0].Evidence.Value != strings.Replace(command, root, ".", 1) || body.Sources[0].Evidence.Anchor != first.Anchor || body.Sources[0].Evidence.Anchor == "" || body.Sources[1].Evidence.Value != "rm -f sqlite3" || body.Sources[1].Evidence.Anchor != second.Anchor || body.Sources[0].Evidence.Anchor == body.Sources[1].Evidence.Anchor {
		t.Fatalf("same-kind choices lost source-specific command/anchor evidence: %s", wire)
	}
}

func (p *partitionProvider) State() []byte {
	return []byte(`{"provider":"orientation-partition-preset-v1"}`)
}
func (p *partitionProvider) Prepare(prompt llm.Prompt, limits llm.Limits) (llm.Prepared, error) {
	var wire struct {
		Task         string
		Records      []orientationRecord
		Observations []orientationObservation
		Result       string
	}
	_ = json.Unmarshal([]byte(prompt.User), &wire)
	if (p.recipeAlwaysOversized || p.recipeNeedsSingle && len(wire.Observations) > 1) && wire.Result == "run_recipe" || wire.Task == "" && !p.outputRefusal || p.bound > 0 && len(prompt.User) > p.bound || p.maxRecords > 0 && wire.Task == contextTask && len(wire.Records) > p.maxRecords {
		return llm.Prepared{}, llm.NewResourceLimitError(llm.ResourceLimitError{Kind: llm.ResourceLimitContextTokens})
	}
	return llm.NewPrepared([]byte(prompt.User))
}
func (p *partitionProvider) Complete(ctx context.Context, prepared llm.Prepared) (completion llm.Completion, err error) {
	defer func() {
		if err != nil && p.t != nil {
			p.t.Logf("request-bound preset refusal: %v", err)
		}
	}()
	var wire struct {
		Task         string
		Target       targetWire
		Targets      []targetWire
		Records      []orientationRecord
		Observations []orientationObservation
		Result       string
		Scope        orientationScope
	}
	if err := json.Unmarshal(prepared.Bytes(), &wire); err != nil {
		return llm.Completion{}, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	if wire.Task == "" {
		if p.finishLength {
			return llm.Completion{Response: []byte(`{"roles":[`), FinishReason: llm.FinishLength, ChoiceCount: 1}, nil
		}
		return llm.Completion{}, llm.NewResourceLimitError(llm.ResourceLimitError{Kind: llm.ResourceLimitOutputTokens})
	}
	var response any
	if wire.Task == contextTask {
		_, wire.Records = expandContextWire(p.t, prepared.Bytes())
		if len(wire.Records) == 0 || wire.Scope.First != wire.Records[0].Ref || wire.Scope.Last != wire.Records[len(wire.Records)-1].Ref {
			return llm.Completion{}, fmt.Errorf("advertised scope does not name actual prepared leaf")
		}
		if p.bound > 0 && prepared.Len() > p.bound || p.maxRecords > 0 && len(wire.Records) > p.maxRecords {
			return llm.Completion{}, fmt.Errorf("oversized leaf reached transport")
		}
		if wire.Target.Ref == p.refuseTarget && p.refuseTarget != "" || p.refuseRefinement && len(wire.Records) > 0 && wire.Records[0].Kind == "observation" {
			response = map[string]any{"observations": []any{}}
		} else {
			if p.seen == nil {
				p.seen = map[string][]orientationRecord{}
			}
			p.seen[wire.Target.Ref] = append(p.seen[wire.Target.Ref], wire.Records...)
			late := false
			for _, record := range wire.Records {
				raw, _ := encodeWire(record.Value)
				late = late || strings.Contains(string(raw), "FINAL_CONNECTION")
			}
			text := "Complete contextual reading of all advertised native records and model interpretations."
			if late {
				text = "FINAL_CONNECTION is present beside the other supported work; a required input remains required."
			}
			response = signalPreset(wire.Records, text, p.refs)
		}
	} else if wire.Task == "repomap.orientation.final.v1" {
		if wire.Result == p.refuseSection {
			return llm.Completion{Response: []byte(`{"broken":`), FinishReason: llm.FinishStop, ChoiceCount: 1}, nil
		}
		switch wire.Result {
		case "roles":
			ref := wire.Targets[0].Ref
			source := p.refs.subject("beta", "inbound")
			if ref == p.refs.target("alpha") {
				source = p.refs.fact("entrypoint")
			}
			advertised := false
			for _, obs := range wire.Observations {
				advertised = advertised || slices.Contains(obs.Sources, source)
			}
			if !advertised {
				return llm.Completion{}, fmt.Errorf("unadvertised preset role source")
			}
			response = map[string]any{"roles": []any{map[string]any{"target": ref, "role": "Supported component", "purpose": "Performs the supported work.", "refs": []string{source}}}}
		case "run_recipe":
			if wire.Targets[0].Ref == p.refs.target("alpha") {
				response = map[string]any{"run_recipe": []any{map[string]any{"target": wire.Targets[0].Ref, "command": "go run . <input>", "cwd": "alpha", "note": "Supply the required input and PORT.", "refs": []string{p.refs.fact("entrypoint"), p.refs.fact("config")}}}}
			} else {
				response = map[string]any{"run_recipe": []any{}}
			}
		default:
			response = map[string]any{"summary": "The repository performs the supported work.", "summary_refs": []string{p.refs.fact("entrypoint")}, "main_flow_target": p.refs.target("alpha")}
		}
	} else {
		return llm.Completion{}, fmt.Errorf("unknown task")
	}
	raw, err := encodeWire(response)
	return llm.Completion{Response: raw, FinishReason: llm.FinishStop, ChoiceCount: 1}, err
}

func TestOrientationOwnerPartitionsCompleteEvidenceAndKeepsIndependentDecisions(t *testing.T) {
	for _, refused := range []string{"", "run_recipe", "repository"} {
		t.Run(refused, func(t *testing.T) {
			fixture := newFixture(t)
			p := &partitionProvider{t: t, maxRecords: 2, refs: fixture.refs(t), refuseSection: refused}
			result, rejected, err := Run(t.Context(), llm.Executor{}, p, fixture.input)
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Roles) != 2 {
				t.Fatalf("independent roles lost: %+v", result.Roles)
			}
			if refused != "run_recipe" && (len(result.RunRecipe) != 1 || result.RunRecipe[0].Command != "go run . <input>" || !strings.Contains(result.RunRecipe[0].Note, "PORT")) {
				t.Fatalf("usable invocation lost: %+v", result.RunRecipe)
			}
			if refused == "" && (result.Summary == "" || len(result.MainFlow.Steps) != 2 || len(rejected) > 0) {
				t.Fatalf("global decision failed: %+v %+v", result, rejected)
			}
			if refused == "repository" && (result.Summary != "" || len(result.RunRecipe) != 1 || len(rejected) != 1) {
				t.Fatalf("global refusal lost neighbour sections")
			}
			overview, _, err := buildOverview(fixture.input)
			if err != nil {
				t.Fatal(err)
			}
			for _, target := range overview.Targets {
				expected := orientationRecords(overview, target)
				seen := p.seen[target.Ref]
				if !sameOrientationRecords(expected, seen) {
					t.Fatalf("target %s did not read complete exact records: got=%d want=%d", target.Ref, len(seen), len(expected))
				}
			}
		})
	}
}

func TestOrientationTargetContextRefusalKeepsSuccessfulTargetAndRefusesPartialSummary(t *testing.T) {
	f := newFixture(t)
	p := &partitionProvider{t: t, maxRecords: 2, refs: f.refs(t), refuseTarget: f.targetID("alpha")}
	result, rejected, err := Run(t.Context(), llm.Executor{}, p, f.input)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Roles) != 1 || result.Roles[0].TargetID != f.targetID("beta") || result.Summary != "" || len(result.MainFlow.Steps) != 0 || len(rejected) == 0 {
		t.Fatalf("context refusal invented or lost output: %+v %+v", result, rejected)
	}
}

func TestOrientationOutputEnvelopeUsesOwnerDecomposition(t *testing.T) {
	f := newFixture(t)
	p := &partitionProvider{t: t, maxRecords: 2, refs: f.refs(t), outputRefusal: true}
	result, rejected, err := Run(t.Context(), llm.Executor{}, p, f.input)
	if err != nil || len(result.Roles) != 2 || result.Summary == "" || len(rejected) > 0 {
		t.Fatalf("output overflow retained aggregate refusal: %v %+v %+v", err, result, rejected)
	}
}

func TestOrientationContextAndFinalDecisionsRefuseConflictingOccurrences(t *testing.T) {
	f := newFixture(t)
	wire, cat, _ := buildOverview(f.input)
	target := wire.Targets[0]
	input := orientationReading{Task: contextTask, Target: target, Records: []orientationRecord{{Ref: "d1"}, {Ref: "d2"}}}
	for _, raw := range []string{`{"scope":"w1","reading":"","reading":"A"}`, `{"scope":"unknown","scope":"w1","reading":"A"}`} {
		if _, err := decodeOrientationContext([]byte(raw), input); err == nil {
			t.Fatalf("accepted conflicting context: %s", raw)
		}
	}
	ref := f.refs(t).fact("entrypoint")
	for _, raw := range []string{`{}`, `{"roles":[]}`, fmt.Sprintf(`{"roles":[{"target":%q,"role":"A","role":"B","refs":[%q]}]}`, target.Ref, ref), fmt.Sprintf(`{"roles":[{"target":%q,"role":"A","refs":[%q]}],"roles":[{"target":%q,"role":"B","refs":[%q]}]}`, target.Ref, ref, target.Ref, ref)} {
		closed := cat
		closed.targets = map[string]string{target.Ref: cat.targets[target.Ref]}
		got, err := normalizeSection([]byte(raw), closed, "roles")
		if err == nil || len(got.roles) > 0 {
			t.Fatalf("accepted missing/conflicting known role: %s %+v %v", raw, got, err)
		}
	}
	raw := fmt.Sprintf(`{"result":{"roles":[{"TARGET":%q,"ROLE":" A ","role":"A","refs":%q,"extra":true}]} ,"metadata":{"x":1}}`, strings.ToUpper(target.Ref), ref)
	got, err := normalizeSection([]byte(raw), cat, "roles")
	// This catalogue includes both targets; the untouched target is explicit.
	if err != nil || len(got.roles) != 1 || got.roles[0].Role != "A" || len(got.rejected) != 1 {
		t.Fatalf("harmless wrapped form refused: %+v %v", got, err)
	}
}

func TestOrientationCompleteScopeKeepsInventoryAndRefusesConflictingDecisions(t *testing.T) {
	input := orientationReading{Records: []orientationRecord{
		{Ref: "d1", Layer: "native", Kind: "fact", Value: map[string]any{"command": "cd \".\" && ./configure"}, Sources: []string{"a1", "a2"}},
		{Ref: "d9", Layer: "model_interpretation", Value: map[string]any{"text": "Earlier reading", "source_inventory": []string{"a9"}}, Sources: []string{"a9"}},
	}}
	raw, _ := encodeWire(signalPreset(input.Records, "Configure with cd \".\" && ./configure. Backslash \\.\n", refLookup{}))
	got, err := decodeOrientationContext(raw, input)
	if err != nil || len(got.Observations) != 4 || !sameOrientationRecords(input.Records, got.Provenance) {
		t.Fatalf("typed context lost full provenance: %+v %v", got, err)
	}
	if !slices.Equal(got.Observations[0].Sources, []string{"a1", "a2"}) || len(got.Observations[0].Native) != 1 {
		t.Fatalf("support selection promoted model inventory: %+v", got)
	}
	for _, raw := range []string{`{}`, `{"scope":"w1"}`, `{"reading":"Reading."}`, `{"scope":"unknown","signals":{}}`, `{"scope":"w1","reading":"Old shape."}`, `{"scope":"w1","scope":"w2","signals":{}}`, `{"scope":"w1","scope":42,"signals":{}}`, `{"scope":"w1","signals":42}`} {
		if _, err := decodeOrientationContext([]byte(raw), input); err == nil {
			t.Fatalf("invalid scope/old protocol accepted: %s", raw)
		}
	}
	repeat := append(append([]byte(`{"left":`), raw...), []byte(`,"right":`)...)
	repeat = append(repeat, raw...)
	repeat = append(repeat, []byte(`,"metadata":{"reading":"harmless"}}`)...)
	if got, err := decodeOrientationContext(repeat, input); err != nil || len(got.Observations) != 4 {
		t.Fatalf("identical wrapped repeat refused: %+v %v", got, err)
	}
}

// The local preset selects actual supplied native records. It acknowledges all
// records separately; it never manufactures support from a model source union.
func signalPreset(records []orientationRecord, text string, refs refLookup) map[string]any {
	var support []string
	var wanted []string
	if refs.fixture != nil {
		wanted = []string{refs.fact("entrypoint"), refs.fact("config"), refs.subject("beta", "inbound")}
	}
	for _, record := range records {
		if record.Layer == "native" && record.Value != nil && len(record.Sources) > 0 && slices.ContainsFunc(record.Sources, func(ref string) bool { return slices.Contains(wanted, ref) }) {
			support = append(support, record.Ref)
		}
	}
	if len(support) == 0 {
		for _, record := range records {
			if record.Layer == "native" && record.Value != nil && len(record.Sources) > 0 {
				support = append(support, record.Ref)
				break
			}
		}
	}
	status := "interpretation"
	if len(support) > 0 {
		status = "supported"
	}
	questions := map[string]any{}
	for _, q := range contextQuestions {
		questions[q] = []any{map[string]any{"basis": []string{records[0].Ref}, "text": text, "status": status, "supports": support}}
	}
	return map[string]any{"scope": "w1", "signals": questions}
}

func TestOrientationReadingKeepsCrossTargetEndpointContextAndSeedCallEvidence(t *testing.T) {
	f := newFixture(t)
	wire, _, err := buildOverview(f.input)
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range wire.Targets {
		records := orientationRecords(wire, target)
		connections, calls := 0, 0
		for _, record := range records {
			raw, _ := encodeWire(record.Value)
			if record.Kind == "connection" {
				connections++
				for _, field := range []string{"from_group", "to_group", "from_target", "to_target"} {
					if !strings.Contains(string(raw), `"`+field+`"`) {
						t.Fatalf("lost closed endpoint %s", field)
					}
				}
			}
			if record.Kind == "seed_call" {
				calls++
				if !strings.Contains(string(raw), `"position":1`) || !strings.Contains(string(raw), "Apply") {
					t.Fatalf("lost ordered native call: %s", raw)
				}
			}
		}
		if connections == 0 || calls == 0 {
			t.Fatalf("missing source records")
		}
	}
}

func TestSavedCompleteSQLiteOrientationWindowDecomposesBeforeTransport(t *testing.T) {
	f := newFixture(t)
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
	p := &preparedPartitionProvider{partitionProvider: &partitionProvider{t: t, refs: f.refs(t)}, client: &deepseek.Client{HTTPClient: &http.Client{}, APIKey: "request-bound-local-preset", Endpoint: "https://api.deepseek.com/chat/completions", Model: "deepseek-v4-flash", ContextTokens: 1000000, MaxTokens: llm.DefaultMaxOutputTokens}}
	cat := newCatalog()
	for _, target := range overview.Targets {
		cat.targets[target.Ref] = target.Ref
	}
	for _, row := range overview.Facts {
		entry := factEntry{id: row.Ref, kind: facts.Kind(row.Kind), export: row.Kind == exportKind, byTarget: map[string]string{}}
		for _, target := range row.Targets {
			entry.byTarget[target] = row.Ref
		}
		cat.facts[row.Ref] = entry
	}
	for _, row := range overview.Seeds {
		target, _, _ := strings.Cut(row.Ref, ".")
		cat.subjects[row.Ref] = subjectEntry{id: row.Ref, targetRef: target}
	}
	bindContextEvidence(cat, overview)
	if err := requestFits(p, llm.Prompt{System: overviewPrompt, User: string(raw), ResponseFormatJSON: true, ResponseExample: overviewExample}); !isEnvelopeRefusal(err) {
		t.Fatalf("complete saved window did not exceed real byte envelope: %v", err)
	}
	for _, target := range overview.Targets {
		records := orientationRecords(overview, target)
		observations, rejected, err := readOrientationContext(t.Context(), llm.Executor{}, p, f.input, groupDigests(f.input.Groups), target, records)
		if err != nil || len(rejected) > 0 || len(observations.Observations) == 0 {
			t.Fatalf("saved complete window does not fit: %v %+v", err, rejected)
		}
		final, err := finalContextWire(overview.Repository, []targetWire{target}, observations.forSection("roles"), cat, "roles")
		if err != nil {
			t.Fatal(err)
		}
		if err := requestFits(p, llm.Prompt{System: sectionPrompt("roles"), User: string(final), ResponseFormatJSON: true, ResponseExample: sectionExample("roles")}); err != nil {
			t.Fatalf("actual prepared final window does not fit: %v", err)
		}
		t.Logf("target=%s original_records=%d final_wire_bytes=%d", target.Ref, len(records), len(final))
		if !sameOrientationRecords(records, p.seen[target.Ref]) {
			t.Fatalf("saved records not complete: target=%s got=%d want=%d", target.Ref, len(p.seen[target.Ref]), len(records))
		}
		var originalRefs, covered []string
		for _, record := range records {
			originalRefs = unionRefs(originalRefs, record.Sources)
		}
		for _, record := range observations.Provenance {
			covered = unionRefs(covered, record.Sources)
		}
		slices.Sort(originalRefs)
		slices.Sort(covered)
		if !slices.Equal(originalRefs, covered) {
			t.Fatal("native citation provenance lost")
		}
	}
}

func sameOrientationRecords(expected, got []orientationRecord) bool {
	if len(expected) != len(got) {
		return false
	}
	normalize := func(records []orientationRecord) map[string]any {
		result := map[string]any{}
		for _, record := range records {
			raw, _ := encodeWire(record)
			var value any
			_ = json.Unmarshal(raw, &value)
			if _, duplicate := result[record.Ref]; duplicate {
				return nil
			}
			result[record.Ref] = value
		}
		return result
	}
	return reflect.DeepEqual(normalize(expected), normalize(got))
}

func TestOrientationFixedRecipeFloorRefusesWithoutResummarizing(t *testing.T) {
	f := newFixture(t)
	p := &partitionProvider{t: t, maxRecords: 2, refs: f.refs(t), recipeNeedsSingle: true}
	result, rejected, err := Run(t.Context(), llm.Executor{}, p, f.input)
	if err != nil || len(result.Roles) != 2 || len(result.RunRecipe) != 0 || len(rejected) == 0 {
		t.Fatalf("recipe fixed floor was not explicit: %v %+v %+v", err, result, rejected)
	}
}
func TestOrientationInvalidKnownRoleCannotBeRepairedByALaterRepeat(t *testing.T) {
	f := newFixture(t)
	_, cat, _ := buildOverview(f.input)
	refs := f.refs(t)
	for _, bad := range []map[string]any{{"target": refs.target("alpha"), "role": "", "refs": []string{refs.fact("entrypoint")}}, {"target": refs.target("alpha"), "role": "First", "refs": []string{"unknown"}}, {"target": refs.target("alpha"), "role": "First"}} {
		raw, _ := encodeWire(map[string]any{"roles": []any{bad, map[string]any{"target": refs.target("alpha"), "role": "Good", "refs": []string{refs.fact("entrypoint")}}, map[string]any{"target": refs.target("beta"), "role": "Client", "refs": []string{refs.subject("beta", "inbound")}}}})
		got, err := normalizeSection(raw, cat, "roles")
		if err != nil || len(got.roles) != 1 || got.roles[0].TargetID != f.targetID("beta") {
			t.Fatalf("repaired a refused decision or lost neighbour: %+v %v", got, err)
		}
	}
}

func TestOrientationNonReductionAndRefusedRefinementKeepCompleteFittingRoleContext(t *testing.T) {
	for _, refuse := range []bool{false, true} {
		t.Run(fmt.Sprint(refuse), func(t *testing.T) {
			f := newFixture(t)
			p := &partitionProvider{t: t, maxRecords: 2, refs: f.refs(t), recipeAlwaysOversized: true, refuseRefinement: refuse}
			result, rejected, err := Run(t.Context(), llm.Executor{}, p, f.input)
			if err != nil || len(result.Roles) != 2 || result.Summary == "" || len(result.RunRecipe) != 0 || len(rejected) == 0 || p.calls > 60 {
				t.Fatalf("unbounded or partial refinement: calls=%d err=%v result=%+v rejected=%+v", p.calls, err, result, rejected)
			}
		})
	}
}
func TestOrientationActualLengthFinishUsesBoundedOwnerDecomposition(t *testing.T) {
	f := newFixture(t)
	p := &partitionProvider{t: t, maxRecords: 2, refs: f.refs(t), outputRefusal: true, finishLength: true}
	result, rejected, err := Run(t.Context(), llm.Executor{}, p, f.input)
	if err != nil || len(result.Roles) != 2 || result.Summary == "" || len(rejected) > 0 {
		t.Fatalf("ordinary output refusal not decomposed: %v %+v %+v", err, result, rejected)
	}
}

func TestOrientationHarmlessRecipeRepeatAndInvalidOptionalPurposeKeepRequiredDecisions(t *testing.T) {
	f := newFixture(t)
	_, cat, _ := buildOverview(f.input)
	refs := f.refs(t)
	row := map[string]any{"target": refs.target("alpha"), "command": "go run .", "cwd": "alpha", "refs": []string{refs.fact("entrypoint")}}
	encoded, _ := encodeWire(row)
	raw := []byte(fmt.Sprintf(`{"run_recipe":[%s],"run_recipe":[%s]}`, encoded, encoded))
	got, err := normalizeSection(raw, cat, "run_recipe")
	if err != nil || len(got.recipe) != 1 {
		t.Fatalf("harmless recipe repeat became two steps: %+v %v", got, err)
	}
	raw, _ = encodeWire(map[string]any{"roles": []any{map[string]any{"target": refs.target("alpha"), "role": "Server", "purpose": "invalid\x00purpose", "refs": []string{refs.fact("entrypoint")}}}})
	got, err = normalizeSection(raw, cat, "roles")
	if err != nil || len(got.roles) != 1 || got.roles[0].Purpose != "" || len(got.rejected) != 2 {
		t.Fatalf("optional purpose refused required role: %+v %v", got, err)
	}
}

func TestOrientationPartialConflictsRetainExactLiveAndWarmResponseAfterCacheClear(t *testing.T) {
	f := newFixture(t)
	refs := f.refs(t)
	raw := []byte(fmt.Sprintf(`{"summary":"First","summary":"Different","summary_refs":[%q],"roles":[{"target":%q,"role":"Server","refs":[%q]}]}`, refs.fact("entrypoint"), refs.target("alpha"), refs.fact("entrypoint")))
	p := &presetProvider{respond: func([]byte) []byte { return raw }}
	root := t.TempDir()
	writer, err := debugdump.NewWriter(root, "orientation")
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	executor := debugdump.BindStage(llm.Executor{RootDir: root, Enabled: true, Observer: debugdump.NewSemanticObserver(writer)}, StageName)
	for range 2 {
		result, rejected, err := Run(t.Context(), executor, p, f.input)
		if err != nil || len(result.Roles) != 1 || len(rejected) != 1 || !strings.Contains(string(rejected[0].Raw), `"summary":"Different"`) {
			t.Fatalf("partial provenance lost: %v %+v", err, rejected)
		}
	}
	if p.completions != 1 {
		t.Fatalf("warm partial answer not reused: %d", p.completions)
	}
	if err := os.RemoveAll(filepath.Join(root, llm.CacheDirectoryName)); err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(writer.BaseDir, writer.RunID)
	journals, err := filepath.Glob(filepath.Join(runDir, debugdump.SemanticExchangesDir, "*", debugdump.SemanticExchangeMetaFile))
	if err != nil || len(journals) != 2 {
		t.Fatalf("exchange journal missing: %v %v", journals, err)
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
		if err != nil || string(saved) != string(raw) {
			t.Fatalf("cache clear erased refused occurrences: %v", err)
		}
	}
	rows, err := os.ReadFile(filepath.Join(runDir, "rejected.jsonl"))
	if err != nil || !strings.Contains(string(rows), `"response_ref"`) {
		t.Fatalf("exact response reference missing: %v", err)
	}
}
