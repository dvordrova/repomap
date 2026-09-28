package terminology

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/dvordrova/repomap/internal/llm"
)

type testProvider struct {
	mu       sync.Mutex
	calls    int
	prompt   llm.Prompt
	limits   llm.Limits
	complete func(llm.Prepared) (llm.Completion, error)
}

func (*testProvider) State() []byte { return []byte(`{"model":"terms-test"}`) }
func (p *testProvider) Prepare(prompt llm.Prompt, limits llm.Limits) (llm.Prepared, error) {
	p.mu.Lock()
	p.prompt, p.limits = prompt, limits
	p.mu.Unlock()
	raw, err := json.Marshal(map[string]any{"messages": []map[string]string{{"role": "system", "content": prompt.System}, {"role": "user", "content": prompt.User}}, "max_tokens": limits.MaxOutputTokens})
	if err != nil {
		return llm.Prepared{}, err
	}
	return llm.NewPrepared(raw)
}
func (p *testProvider) Complete(_ context.Context, prepared llm.Prepared) (llm.Completion, error) {
	p.mu.Lock()
	p.calls++
	p.mu.Unlock()
	return p.complete(prepared)
}
func inputUser(t *testing.T, prepared llm.Prepared) string {
	t.Helper()
	var wire struct{ Messages []struct{ Content string } }
	if err := json.Unmarshal(prepared.Bytes(), &wire); err != nil {
		t.Fatal(err)
	}
	return wire.Messages[1].Content
}
func completed(raw string) (llm.Completion, error) {
	return llm.Completion{Response: []byte(raw), FinishReason: llm.FinishStop, ChoiceCount: 1, Metrics: llm.Metrics{Attempts: 1}}, nil
}

type acceptedRows struct {
	Rows []map[string]string `json:"rows"`
}

func (acceptedRows) AcceptedRowKeys() []string { return []string{"r1"} }

func TestMainResponseKeepsSoleShapeAndExactSourceScope(t *testing.T) {
	c := NewCollector([]string{"a.py", "b.go", "README.md", "not-sent.py"})
	base := &testProvider{}
	wrapper := c.Wrap(base)
	prompt := llm.Prompt{System: "Original task.", User: `{"context":{"path":"README.md"},"rows":[{"key":"r1","path":"a.py","line":7},{"key":"r2","files":["b.go"],"line":99}]}`, ResponseExample: `{"rows":[{"key":"r1","line":"<computed>"}]}`, ResponseFormatJSON: true, Reasoning: true}
	first, err := llm.Prepare(wrapper, prompt, llm.Limits{MaxOutputTokens: 16000})
	if err != nil {
		t.Fatal(err)
	}
	second, err := llm.Prepare(wrapper, prompt, llm.Limits{MaxOutputTokens: 16000})
	if err != nil || !bytes.Equal(first.Bytes(), second.Bytes()) {
		t.Fatal("unstable preparation")
	}
	if !base.prompt.Reasoning || strings.Count(base.prompt.System, `"rows"`) != 1 || strings.Contains(base.prompt.System, `"terms"`) || strings.Contains(base.prompt.System, `"result"`) {
		t.Fatalf("second response shape: %s", base.prompt.System)
	}
	ctx, err := c.requestContext(first.ResponseContext())
	if err != nil || len(ctx.sources) != 3 {
		t.Fatalf("source catalogue: %+v %v", ctx, err)
	}
	for _, source := range ctx.sources {
		if source.Path == "a.py" && (source.Line != 7 || source.Row != "r1") {
			t.Fatal(source)
		}
		if source.Path == "b.go" && (source.Line != 0 || source.Row != "r2") {
			t.Fatal(source)
		}
	}
	if strings.Contains(string(first.Bytes()), "not-sent.py") || base.calls != 0 {
		t.Fatal("invented source or provider work")
	}
	for _, raw := range []string{`[]`, `{"terms":["domain-owned"]}`} {
		adapted, err := llm.AdaptResponse(wrapper, first.ResponseContext(), first.Bytes(), []byte(raw))
		if err != nil || string(adapted.Domain) != raw || len(adapted.Rejections) != 0 {
			t.Fatalf("owner result rewritten: %+v %v", adapted, err)
		}
	}
}

func TestSeparateGlossaryKeepsAcceptedRowsMainOriginsAndWarmCache(t *testing.T) {
	base := &testProvider{}
	main := `{"rows":[{"key":"r1","line":"OHLCV gives market values."},{"key":"r2","line":"BadTerm belongs to a refused row."}]}`
	base.complete = func(prepared llm.Prepared) (llm.Completion, error) {
		user := inputUser(t, prepared)
		if answer, ok := glossaryAnswer(user, written("OHLCV", "BadTerm"), explainedBy(map[string]string{"OHLCV": "Open, high, low, close and volume market values."})); ok {
			if strings.Contains(user, "BadTerm") {
				t.Fatal("refused prose entered glossary")
			}
			return completed(answer)
		}
		return completed(main)
	}
	call := llm.Call[acceptedRows]{State: []byte(`{"contract":"test.rows.v1"}`), Limits: llm.Limits{MaxRequestBytes: 1 << 20, MaxResponseBytes: 1 << 20, MaxOutputTokens: 16000}, Prompt: llm.Prompt{User: `{"rows":[{"key":"r1","path":"api.py"},{"key":"r2","path":"bad.py"}]}`, ResponseExample: `{"rows":[{"key":"r1","line":"<computed>"}]}`}}
	executor := llm.Executor{Enabled: true, RootDir: t.TempDir()}
	var want []Candidate
	for run := 0; run < 2; run++ {
		collector := NewCollector([]string{"api.py", "bad.py"})
		wrapper := collector.Wrap(base)
		outcome, err := llm.ExecuteJSON(t.Context(), executor, wrapper, call)
		if err != nil {
			t.Fatal(err)
		}
		if string(outcome.Response) != main || len(collector.Snapshot()) != 0 || len(collector.pending) != 1 {
			t.Fatal("main result or accepted scope changed")
		}
		if err := collector.Generate(t.Context(), executor, wrapper, everyConcept(), ""); err != nil {
			t.Fatal(err)
		}
		got := collector.Snapshot()
		hash := sha256.Sum256(outcome.Request)
		if len(got) != 1 || !reflect.DeepEqual(got[0].Origins, []Origin{{RequestSHA256: hex.EncodeToString(hash[:]), Row: "r1"}}) || !reflect.DeepEqual(got[0].Sources, []Source{{Path: "api.py"}}) {
			t.Fatalf("wrong main provenance: %+v", got)
		}
		if run == 0 {
			want = got
		} else if !reflect.DeepEqual(want, got) || base.calls != 3 {
			t.Fatalf("warm glossary changed or re-bought: %+v calls=%d", got, base.calls)
		}
	}
}

func TestParallelWindowsKeepLocalOwnershipAcrossWarmAndMemoCollection(t *testing.T) {
	base := &testProvider{}
	base.complete = func(prepared llm.Prepared) (llm.Completion, error) {
		if strings.Contains(inputUser(t, prepared), "REPOMAP_PROSE_SOURCES_V1") {
			t.Fatal("local provenance entered the request")
		}
		return completed(`{"rows":[{"key":"r1","line":"AcceptedConcept is explained."},{"key":"r2","line":"RefusedConcept must not survive."}]}`)
	}
	var paths []string
	var calls []llm.Call[acceptedRows]
	for i := 0; i < 8; i++ {
		name := fmt.Sprintf("module%d.py", i)
		paths = append(paths, name)
		calls = append(calls, llm.Call[acceptedRows]{State: []byte(`{"contract":"parallel-prose"}`),
			Prompt: llm.Prompt{User: fmt.Sprintf(`{"rows":[{"key":"r1","path":%q,"line":%d},{"key":"r2","path":"refused.py"}]}`, name, i+3)},
			Limits: llm.Limits{MaxRequestBytes: 100000, MaxResponseBytes: 100000, MaxOutputTokens: llm.DefaultMaxOutputTokens}})
	}
	paths = append(paths, "refused.py")
	executor := llm.Executor{Enabled: true, RootDir: t.TempDir(), BatchConcurrency: 4}
	var cachedKeys []string
	var want map[string]proseSource
	for run := 0; run < 2; run++ {
		collector := NewCollector(paths)
		outcomes := llm.ExecuteJSONEach(t.Context(), executor, collector.Wrap(base), calls)
		for i, outcome := range outcomes {
			if outcome.Err != nil || outcome.Outcome.Cached != (run == 1) {
				t.Fatalf("window %d run %d: %+v", i, run, outcome)
			}
			if run == 0 {
				cachedKeys = append(cachedKeys, outcome.Outcome.CacheKey)
			}
		}
		if len(collector.pending) != 8 {
			t.Fatalf("parallel windows overwrote or borrowed another window: %+v", collector.pending)
		}
		for _, prose := range collector.pending {
			if !reflect.DeepEqual(prose.Texts, []string{"AcceptedConcept is explained."}) || len(prose.Sources) != 1 || prose.Sources[0].Path == "refused.py" || prose.Origin.Row != "r1" {
				t.Fatalf("refused neighbour gained prose/source authority: %+v", prose)
			}
		}
		if run == 0 {
			want = collector.pending
		} else if !reflect.DeepEqual(collector.pending, want) || base.calls != 8 {
			t.Fatal("warm collection changed ownership or bought more completions")
		}
	}
	// Entity memo reuse has only the shared exchange and original local context;
	// the current collector has never seen any Prepare call for these windows.
	memo := NewCollector(paths)
	for _, key := range cachedKeys {
		exchange, found, err := llm.CachedExchange(executor.RootDir, key)
		if err != nil || !found {
			t.Fatal("original exchange unavailable")
		}
		adapted, err := llm.AdaptResponse(memo.Wrap(base), exchange.ResponseContext, exchange.Request, exchange.Response)
		if err != nil {
			t.Fatal(err)
		}
		adapted.Accepted([]string{"r1"})
	}
	if !reflect.DeepEqual(memo.pending, want) || base.calls != 8 {
		t.Fatal("fresh memo collection lost original row ownership")
	}
}

func TestGlossaryReplayUsesUpdatedAcceptedProseAndOriginalRequestOrigin(t *testing.T) {
	base := &testProvider{}
	main := `{"rows":[{"key":"r1","line":"Alpha is the original concept."}]}`
	base.complete = func(prepared llm.Prepared) (llm.Completion, error) {
		user := inputUser(t, prepared)
		if answer, ok := glossaryAnswer(user, written("Alpha", "Beta"), explainedBy(map[string]string{"Alpha": "The original concept.", "Beta": "The updated concept."})); ok {
			return completed(answer)
		}
		return completed(main)
	}
	decider := everyConcept()
	call := llm.Call[acceptedRows]{State: []byte(`{"contract":"replay.prose.v1"}`), Limits: llm.Limits{MaxRequestBytes: 1 << 20, MaxResponseBytes: 1 << 20, MaxOutputTokens: 16000}, Prompt: llm.Prompt{User: `{"rows":[{"key":"r1","path":"api.py"}]}`, ResponseExample: `{"rows":[{"key":"r1","line":"<computed>"}]}`}}
	executor := llm.Executor{Enabled: true, RootDir: t.TempDir()}
	first := NewCollector([]string{"api.py"})
	outcome, err := llm.ExecuteJSON(t.Context(), executor, first.Wrap(base), call)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Generate(t.Context(), executor, base, decider, ""); err != nil {
		t.Fatal(err)
	}
	main = `{"rows":[{"key":"r1","line":"Beta is the updated concept."}]}`
	prepared, err := llm.NewPrepared(outcome.Request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := llm.ReplayJSON(t.Context(), executor, base, prepared); err != nil {
		t.Fatal(err)
	}
	current := NewCollector([]string{"api.py"})
	warm, err := llm.ExecuteJSON(t.Context(), executor, current.Wrap(base), call)
	if err != nil || !warm.Cached {
		t.Fatalf("replay did not replace the original answer: %v %v", warm.Cached, err)
	}
	if err := current.Generate(t.Context(), executor, base, decider, ""); err != nil {
		t.Fatal(err)
	}
	old, got := first.Snapshot(), current.Snapshot()
	if len(got) != 1 || got[0].Name != "Beta" || len(old) != 1 || old[0].Name != "Alpha" || !reflect.DeepEqual(got[0].Origins, old[0].Origins) || base.calls != 6 {
		t.Fatalf("replayed prose kept stale glossary or changed its main origin: old=%+v new=%+v calls=%d", old, got, base.calls)
	}
}

func TestDeferredTableProseExcludesClosedUnusedAndExtraCells(t *testing.T) {
	c := NewCollector([]string{"a.py"})
	wrapper := c.Wrap(&testProvider{})
	prompt := llm.Prompt{User: `{"fill":[{"name":"decision","kind":"choice"},{"name":"line","kind":"prose","when":{"decision":"yes"}},{"name":"alias","kind":"text"}],"rows":[{"key":"r1","path":"a.py"},{"key":"r2","path":"a.py"}]}`}
	prepared, err := llm.Prepare(wrapper, prompt, llm.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	response := []byte(`{"rows":[{"key":"r1","decision":"no","line":"UnusedTerm","alias":"ActiveAlias","extra":"ExtraTerm"},{"key":"r2","decision":"yes","line":"AcceptedTerm has a qualification.\n\nKeep it complete.","alias":"OtherAlias"}]}`)
	adapted, err := llm.AdaptResponse(wrapper, prepared.ResponseContext(), prepared.Bytes(), response)
	if err != nil {
		t.Fatal(err)
	}
	adapted.Accept([]string{"r1", "r2"})
	var texts []string
	for _, item := range c.pending {
		texts = append(texts, item.Texts...)
	}
	joined := strings.Join(texts, "|")
	if len(texts) != 3 || strings.Contains(joined, "UnusedTerm") || strings.Contains(joined, "ExtraTerm") || !strings.Contains(joined, "AcceptedTerm has a qualification.\n\nKeep it complete.") {
		t.Fatalf("glossary inherited unaccepted cells or trimmed prose: %q", texts)
	}
}

func TestLocalSourceAuthoritySurvivesCurrentCorpusAndContextCopies(t *testing.T) {
	old := NewCollector([]string{"a.py", "b.py"})
	prompt := llm.Prompt{User: `{"rows":[{"key":"r1","path":"a.py","line":7},{"key":"r2","path":"b.py","line":9}]}`}
	base := &testProvider{}
	prepared, err := llm.Prepare(old.Wrap(base), prompt, llm.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	current := NewCollector([]string{"a.py"})
	ctx, err := current.requestContext(prepared.ResponseContext())
	if err != nil || len(ctx.sources) != 1 || ctx.sources["g1"].Path != "a.py" || ctx.sources["g1"].Line != 7 {
		t.Fatalf("removed file retained current authority: %+v / %v", ctx, err)
	}
	local := prepared.ResponseContext()
	local[0] = '!'
	ctx, err = current.requestContext(prepared.ResponseContext())
	if err != nil || ctx.sources["g1"].Line != 7 {
		t.Fatal("mutable context escaped Prepared")
	}
	if base.prompt.User != prompt.User || strings.Contains(string(prepared.Bytes()), "REPOMAP_PROSE_SOURCES_V1") {
		t.Fatal("local source context entered provider input")
	}
}

func TestOptionalOutputFailureSplitsCompleteProseAndKeepsSibling(t *testing.T) {
	c := NewCollector([]string{"a.py", "b.py"})
	c.pending["a"] = proseSource{Texts: []string{"Alpha concept."}, Sources: []Source{{Path: "a.py"}}, Origin: Origin{RequestSHA256: strings.Repeat("a", 64), Row: "r1"}}
	c.pending["b"] = proseSource{Texts: []string{"Beta concept."}, Sources: []Source{{Path: "b.py"}}, Origin: Origin{RequestSHA256: strings.Repeat("b", 64), Row: "r2"}}
	base := &testProvider{}
	var seen sync.Map
	base.complete = func(prepared llm.Prepared) (llm.Completion, error) {
		user := inputUser(t, prepared)
		if strings.Contains(user, `"terms":`) {
			answer, _ := glossaryAnswer(user, nil, explainedBy(map[string]string{"Alpha": "The first concept."}))
			return completed(answer)
		}
		if strings.Contains(user, "Alpha") && strings.Contains(user, "Beta") {
			return llm.Completion{}, llm.NewResourceLimitError(llm.ResourceLimitError{Kind: llm.ResourceLimitOutputTokens, Limit: 8000})
		}
		if strings.Contains(user, "Alpha") {
			seen.Store("Alpha", true)
			return completed(`{"names":["Alpha"]}`)
		}
		seen.Store("Beta", true)
		return completed(`{"names":[`)
	}
	if err := c.Generate(t.Context(), llm.Executor{}, base, everyConcept(), ""); err != nil {
		t.Fatal(err)
	}
	count := 0
	seen.Range(func(_, _ any) bool { count++; return true })
	// The refused pair, its two halves and one explanation request.
	if count != 2 || len(c.pending) != 2 || len(c.Snapshot()) != 1 || base.calls != 4 || base.limits.MaxOutputTokens != glossaryOutputTokens {
		t.Fatalf("optional refusal lost sibling: seen=%d %+v", count, c.Snapshot())
	}
}

// The names step keeps each name that the window's prose writes, once.
func TestNamesKeepEachNameTheProseWritesOnce(t *testing.T) {
	call, err := namesCall([]proseSource{{Texts: []string{"커스텀 Matcher를 사용합니다."}, Sources: []Source{{Path: "a.py"}}}, {Texts: []string{"Storage is separate."}, Sources: []Source{{Path: "b.py"}}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := call.DecodeValidate([]byte(`{"names":["Matcher","Storage","Missing","Matcher"," Matcher "]}`))
	if err != nil || !reflect.DeepEqual(got.Names, []string{"Matcher", "Storage"}) {
		t.Fatalf("names: %+v %v", got, err)
	}
	if len(got.Rejections) != 1 || got.Rejections[0].Reason != "the name is not written in the prose" || got.Rejections[0].Count != 1 {
		t.Fatalf("an unwritten name was not journaled: %+v", got.Rejections)
	}
	if empty, err := call.DecodeValidate([]byte(`{"names":[]}`)); err != nil || len(empty.Names) != 0 {
		t.Fatal("a valid empty answer was refused")
	}
	if _, err := call.DecodeValidate([]byte(`{"names":["Missing"]}`)); err == nil {
		t.Fatal("an answer none of whose names the prose writes was accepted")
	}
}

// Code, not the model, attaches every prose row that writes a name; an
// explanation carries the complete sources and origins of all of them.
func TestExplanationRestoresEveryRowThatWritesTheName(t *testing.T) {
	items := []proseSource{
		{Texts: []string{"The OTLP trace collector receives spans."}, Sources: []Source{{Path: "otel.go", Line: 91}, {Path: "main.go", Line: 21}, {Path: "README.md", Line: 62}}, Origin: Origin{RequestSHA256: strings.Repeat("a", 64), Row: "r26"}},
		{Texts: []string{"The OTLP trace collector is configurable."}, Sources: []Source{{Path: "main.go", Line: 16}}, Origin: Origin{RequestSHA256: strings.Repeat("b", 64), Row: "r15"}},
		{Texts: []string{"Unrelated storage."}, Sources: []Source{{Path: "storage.go", Line: 7}}, Origin: Origin{RequestSHA256: strings.Repeat("c", 64), Row: "r1"}},
	}
	names := gatherNames(items, []string{"OTLP trace collector"})
	if len(names) != 1 || !reflect.DeepEqual(names[0].Rows, []int{0, 1}) {
		t.Fatalf("the name's rows: %+v", names)
	}
	call, err := explainCall(items, names)
	if err != nil {
		t.Fatal(err)
	}
	var input struct {
		Prose []struct {
			Ref  string
			Text []string
		}
		Terms []struct {
			Ref, Name string
			Rows      []string
		}
	}
	if json.Unmarshal([]byte(call.Prompt.User), &input) != nil || len(input.Prose) != 2 || len(input.Terms) != 1 || !reflect.DeepEqual(input.Terms[0].Rows, []string{"p1", "p2"}) ||
		strings.Contains(call.Prompt.User, "Unrelated") || strings.Contains(call.Prompt.User, "otel.go") {
		t.Fatalf("the explanation request is not the name's own rows: %s", call.Prompt.User)
	}
	got, err := call.DecodeValidate([]byte(`{"terms":[{"ref":"t1","explanation":"The configured trace destination."},{"ref":"g1","explanation":"A legacy ref."}]}`))
	if err != nil || len(got.Explanations) != 1 || len(got.Rejections) != 1 {
		t.Fatalf("closed term refs: %+v %v", got, err)
	}
	collector := NewCollector([]string{"otel.go", "main.go", "README.md", "storage.go"})
	collector.acceptDefinitions(items, names, got.Explanations)
	definitions := collector.Snapshot()
	wantSources := normalizeSources(append(append([]Source{}, items[0].Sources...), items[1].Sources...))
	if len(definitions) != 1 || !reflect.DeepEqual(definitions[0].Sources, wantSources) ||
		!reflect.DeepEqual(definitions[0].Origins, normalizeOrigins([]Origin{items[0].Origin, items[1].Origin})) {
		t.Fatalf("a row that writes the name lost its sources, or another row lent its own: %+v", definitions)
	}
}

func TestSavedNonTableOwnerProsePathsExcludeTechnicalAndExtraCells(t *testing.T) {
	collector := NewCollector([]string{"a.py", "b.py"})
	prompt := llm.Prompt{User: `{"rows":[{"key":"r1","path":"a.py"},{"key":"r2","path":"b.py"}]}`,
		ProseFields: []string{"questions[].selections[].why"}}
	prepared, err := llm.Prepare(collector.Wrap(&testProvider{}), prompt, llm.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	// A fresh collector uses the original prepared context, without any registry.
	current := NewCollector([]string{"a.py", "b.py"})
	response := []byte(`{"questions":[{"key":"q1","selections":[{"row":"r1","anchors":["a1"],"relevance":"direct","why":"Read the direct context associated with a1.","extra":{"why":"Do not collect extra prose."}},{"row":"r2","anchors":["a2"],"relevance":"context","why":"Rejected source prose."}]}]}`)
	adapted, err := llm.AdaptResponse(current.Wrap(&testProvider{}), prepared.ResponseContext(), prepared.Bytes(), response)
	if err != nil {
		t.Fatal(err)
	}
	adapted.Accepted([]string{"r1"})
	if len(current.pending) != 1 {
		t.Fatalf("source scope: %+v", current.pending)
	}
	for _, item := range current.pending {
		if !reflect.DeepEqual(item.Texts, []string{"Read the direct context associated with a1."}) || item.Origin.Row != "r1" {
			t.Fatalf("owner prose path collected technical/extra cells or blacklisted words: %+v", item)
		}
	}
}

func TestTableEmptyProseIsAnOwnerDescriptorNotAStringBlacklist(t *testing.T) {
	collector := NewCollector([]string{"a.py"})
	provider := collector.Wrap(&testProvider{})
	prompt := llm.Prompt{User: `{"fill":[{"name":"remaining","kind":"prose","empty_value":"none"},{"name":"meaning","kind":"prose"}],"rows":[{"key":"r1","path":"a.py"}]}`}
	prepared, err := llm.Prepare(provider, prompt, llm.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	adapted, err := llm.AdaptResponse(provider, prepared.ResponseContext(), prepared.Bytes(), []byte(`{"rows":[{"key":"r1","remaining":"none","meaning":"none"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	adapted.Accepted([]string{"r1"})
	for _, item := range collector.pending {
		if !reflect.DeepEqual(item.Texts, []string{"none"}) {
			t.Fatalf("ordinary prose spelling was filtered: %+v", item)
		}
	}
	if len(collector.pending) != 1 {
		t.Fatalf("ordinary prose lost: %+v", collector.pending)
	}
}

func TestNamedAndNestedRowsCannotMoveRefusedProse(t *testing.T) {
	for _, test := range []struct{ result, kept string }{
		{`{"reviews":[{"intent":"purpose","reason":"BadReview","questions":[{"key":"structure","why":"BadNested"}]},{"intent":"structure","reason":"GoodReview"}]}`, "structure"},
		{`{"summary":"BadSummary","roles":[{"purpose":"BadRole","key":"roles[1]"},{"purpose":"GoodRole"}],"main_flow":{"title":"BadTitle","steps":[]}}`, "roles[1]"},
		{`{"sources":[{"ref":"e1","text":"BadSource","extra":{"key":"e2","text":"BadNested"}},{"ref":"e2","text":"GoodSource"}]}`, "e2"},
		{`{"files":[{"file_ref":"f1","hypotheses":["BadEntry"],"extra":{"key":"f2","hypotheses":["BadNested"]}},{"file_ref":"f2","hypotheses":["GoodEntry"]}]}`, "f2"},
	} {
		c := NewCollector([]string{"README.md"})
		provider := c.Wrap(&testProvider{})
		prepared, err := llm.Prepare(provider, llm.Prompt{User: `{"path":"README.md"}`, ResponseExample: `{}`}, llm.Limits{})
		if err != nil {
			t.Fatal(err)
		}
		adapted, err := llm.AdaptResponse(provider, prepared.ResponseContext(), prepared.Bytes(), []byte(test.result))
		if err != nil {
			t.Fatal(err)
		}
		adapted.Accepted([]string{test.kept})
		if len(c.pending) != 1 {
			t.Fatalf("accepted row lost: %+v", c.pending)
		}
		for _, item := range c.pending {
			if strings.Contains(strings.Join(item.Texts, " "), "Bad") {
				t.Fatalf("refused text acquired authority: %+v", item)
			}
		}
	}
}

func TestNoSourceModeAndEmptyAcceptancePreserveDomain(t *testing.T) {
	c := NewCollector([]string{"api.py"})
	provider := c.Wrap(&testProvider{})
	for _, raw := range []string{`[]`, `{"terms":["domain-owned"]}`} {
		adapted, err := llm.AdaptResponse(provider, nil, []byte(`{"user":"no catalogue"}`), []byte(raw))
		if err != nil || string(adapted.Domain) != raw || adapted.Accept != nil {
			t.Fatal("no-source owner changed")
		}
	}
	prepared, err := llm.Prepare(provider, llm.Prompt{User: `{"path":"api.py"}`, ResponseExample: `{"answer":"<computed>"}`}, llm.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	adapted, err := llm.AdaptResponse(provider, prepared.ResponseContext(), prepared.Bytes(), []byte(`{"answer":"API"}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(c.pending) != 0 {
		t.Fatal("unaccepted result entered glossary")
	}
	adapted.Accepted([]string{})
	if len(c.pending) != 0 {
		t.Fatal("empty row set authorized glossary")
	}
	adapted.Accepted(nil)
	if len(c.pending) != 1 {
		t.Fatal("unkeyed accepted prose lost")
	}
}

func TestProsePlanningRetainsEveryCompleteOriginalText(t *testing.T) {
	input := []proseSource{{Texts: []string{strings.Repeat("complete original paragraph ", 5000), "Second complete text."}, Sources: []Source{{Path: "a.py", Line: 4}}, Origin: Origin{Row: "r1"}}}
	spec := runSpec[proseSource, namesResult]{call: func(window []proseSource) (llm.Call[namesResult], error) { return namesCall(window, nil) }, split: splitProse}
	windows, err := planWindows(t.Context(), &testProvider{}, input, spec)
	if err != nil || len(windows) != 1 || !reflect.DeepEqual(windows[0], input) {
		t.Fatalf("planning: %d %v", len(windows), err)
	}
	// Splitting is available only after an actual preparation/resource refusal;
	// text size alone does not buy another ordinary request.
	left, right, ok := splitProse(input)
	if !ok {
		t.Fatal("resource refusal could not partition complete texts")
	}
	windows = [][]proseSource{left, right}
	for i, window := range windows {
		if len(window) != 1 || len(window[0].Texts) != 1 || window[0].Texts[0] != input[0].Texts[i] || !reflect.DeepEqual(window[0].Sources, input[0].Sources) || window[0].Origin != input[0].Origin {
			t.Fatal("split changed original evidence")
		}
	}
}

func TestUnicodePhrasesAndScriptBoundaries(t *testing.T) {
	for _, test := range []struct {
		text, name string
		want       bool
	}{
		{"시가 총액 describes a measure.", "시가 총액", true},
		{"주식 API reads data.", "주식", true},
		{"시가총액 is one name.", "총액", false},
		{"CAPITAL labels data.", "API", false},
		{"Use load_userdict.", "userdict", false},
		{"Use jieba.cut(text).", "jieba.cut", true},
		{"Use e\u0301.", "e", false},
		{"커스텀 Matcher를 사용합니다.", "Matcher", true},
		{"사용자Matcher를 사용합니다.", "Matcher", true},
		{"커스텀 Matchers를", "Matcher", true},
		{"HMMish", "HMM", false},
		{"preHMM", "HMM", false},
		{"API文档", "API", true},
		{"中文API", "API", true},
		{"分词API", "分词", true},
		{"APIдоступен", "API", true},
		{"методAPI", "API", true},
		{"漢字한글", "漢字", true},
		{"API2", "API", false},
		{"2API", "API", false},
		{"API\u0301", "API", false},
		{"\u0301API", "API", false},
		{"API_한글", "API", false},
	} {
		if got := mentionsTerm(test.text, test.name); got != test.want {
			t.Errorf("%q in %q: %v", test.name, test.text, got)
		}
	}
}

// Owner decision 2026-09-26: a term occurs in any letter case and with an
// English plural ending, still only as a whole word.
func TestTermOccursInAnyCaseAndEnglishPlural(t *testing.T) {
	for _, test := range []struct {
		text, name string
		want       bool
	}{
		{"Nodes exchange snapshots.", "Snapshot", true},
		{"Take one snapshot.", "Snapshot", true},
		{"SNAPSHOTS are kept.", "snapshot", true},
		{"Two classes load.", "Class", true},
		{"Boxes, matches and hashes.", "box", true},
		{"Boxes, matches and hashes.", "match", true},
		{"Boxes, matches and hashes.", "Hash", true},
		{"Several APIs answer.", "API", true},
		{"İstanbul keeps snapshots.", "snapshot", true},
		{"A good cache.", "Go", false},
		{"The request goes out.", "Go", false},
		{"Snapshotting starts.", "Snapshot", false},
		{"Snapshotss", "Snapshot", false},
		{"snapshots_dir", "Snapshot", false},
		{"Nodes exchange snapshots.", "Snap", false},
		{"One subclass.", "Class", false},
		{"Only HTTPS is served.", "HTTP", false},
		{"The IOS build.", "IO", false},
		{"Plain HTTP and two HTTPs.", "HTTP", true},
	} {
		if got := mentionsTerm(test.text, test.name); got != test.want {
			t.Errorf("%q in %q: %v", test.name, test.text, got)
		}
	}
}

func TestCodeNamesAreDroppedByExactNameAndJournaled(t *testing.T) {
	collector := NewCollector([]string{"cmd/app/main.go", "Makefile"})
	collector.ExcludeCodeNames(CodeEnvironmentKey, "RABBITMQ_URL", "HOME")
	collector.ExcludeCodeNames(CodePackage, "example.com/app/broker", "freqtrade")
	// Real repositories also declare their domain words: a local candle, a
	// class Exchange, a method JSON, an enum member ROI.
	collector.ExcludeCodeNames(CodeDeclaration, "ExchangeWS", "funding_rate", "candle", "Exchange", "JSON", "ROI")
	want := map[string]CodeNameKind{
		"cmd/app/main.go": CodeFile, "main.go": CodeFile, "RABBITMQ_URL": CodeEnvironmentKey,
		"example.com/app/broker": CodePackage, "ExchangeWS": CodeDeclaration, "funding_rate": CodeDeclaration,
	}
	code := collector.codeNames()
	if !reflect.DeepEqual(code, want) {
		t.Fatalf("code names: got %v, want %v", code, want)
	}
	items := []proseSource{{Texts: []string{"Set RABBITMQ_URL in main.go; ExchangeWS streams each candle of the Exchange as JSON with funding_rate and ROI."}, Sources: []Source{{Path: "cmd/app/main.go"}}}}
	call, err := namesCall(items, code)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := namesCall(items, nil)
	if err != nil || !reflect.DeepEqual(call.Prompt, plain.Prompt) {
		t.Fatalf("code names changed the provider request: %+v %v", call.Prompt, err)
	}
	got, err := call.DecodeValidate([]byte(`{"names":["RABBITMQ_URL","main.go","ExchangeWS","funding_rate","candle","Exchange","JSON","ROI","ROI"]}`))
	if err != nil || !reflect.DeepEqual(got.Names, []string{"candle", "Exchange", "JSON", "ROI"}) {
		t.Fatalf("names kept: %+v %v", got, err)
	}
	wantJournal := []llm.ResponseRejection{
		{Kind: "glossary_code_name_omitted", Count: 1, Samples: []string{"names[0]"}, Reason: "name is a code environment key: RABBITMQ_URL"},
		{Kind: "glossary_code_name_omitted", Count: 1, Samples: []string{"names[1]"}, Reason: "name is a code file: main.go"},
		{Kind: "glossary_code_name_omitted", Count: 1, Samples: []string{"names[2]"}, Reason: "name is a code declaration: ExchangeWS"},
		{Kind: "glossary_code_name_omitted", Count: 1, Samples: []string{"names[3]"}, Reason: "name is a code declaration: funding_rate"},
	}
	if !reflect.DeepEqual(got.Rejections, wantJournal) {
		t.Fatalf("journal: %+v", got.Rejections)
	}
	only, err := call.DecodeValidate([]byte(`{"names":["RABBITMQ_URL"]}`))
	if err != nil || len(only.Names) != 0 || len(only.Rejections) != 1 || only.Rejections[0].Kind != "glossary_code_name_omitted" {
		t.Fatalf("a window of only code names must be accepted and keep nothing: %+v %v", only, err)
	}
}

// A provider that reports impossible measurements still gave an answer; the
// executor clamps them and keeps it, so the glossary must not end the run.
func TestGlossaryKeepsAnAnswerWithInvalidMeasurements(t *testing.T) {
	base := &testProvider{}
	base.complete = func(prepared llm.Prepared) (llm.Completion, error) {
		answer, ok := glossaryAnswer(inputUser(t, prepared), written("OHLCV"), explainedBy(map[string]string{"OHLCV": "Open, high, low, close and volume market values."}))
		if !ok {
			return completed(`{"rows":[{"key":"r1","line":"OHLCV gives market values."}]}`)
		}
		return llm.Completion{Response: []byte(answer), FinishReason: llm.FinishStop, ChoiceCount: 1, Metrics: llm.Metrics{Attempts: 0, InputTokens: -1}}, nil
	}
	call := llm.Call[acceptedRows]{State: []byte(`{"contract":"test.rows.v1"}`), Limits: llm.Limits{MaxRequestBytes: 1 << 20, MaxResponseBytes: 1 << 20, MaxOutputTokens: 16000}, Prompt: llm.Prompt{User: `{"rows":[{"key":"r1","path":"api.py"}]}`, ResponseExample: `{"rows":[{"key":"r1","line":"<computed>"}]}`}}
	executor := llm.Executor{Enabled: true, RootDir: t.TempDir()}
	collector := NewCollector([]string{"api.py"})
	wrapper := collector.Wrap(base)
	if _, err := llm.ExecuteJSON(t.Context(), executor, wrapper, call); err != nil {
		t.Fatal(err)
	}
	if err := collector.Generate(t.Context(), executor, wrapper, everyConcept(), ""); err != nil {
		t.Fatalf("an answer with clamped measurements ended the glossary: %v", err)
	}
	if got := collector.Snapshot(); len(got) != 1 || got[0].Name != "OHLCV" {
		t.Fatalf("the accepted term was lost: %+v", got)
	}
}

// The table decoder reads a bare array of rows and one wrapping object as
// the rows, and "None." or an empty cell as the column's empty value; the
// glossary reads the same rows and leaves the same absence out.
func TestTableProseReadsTheRowsTheDecoderReads(t *testing.T) {
	prompt := llm.Prompt{User: `{"fill":[{"name":"line","kind":"prose","empty_value":"none"},{"name":"alias","kind":"text"}],"rows":[{"key":"r1","path":"a.py"},{"key":"r2","path":"a.py"}]}`}
	for name, response := range map[string]string{
		"bare array": `[{"key":"r1","line":"None.","alias":"KeptAlias"},{"key":"r2","line":"LedgerTerm is written once.","alias":""}]`,
		"wrapped":    `{"result":{"rows":[{"key":"r1","line":"NONE","alias":"KeptAlias"},{"key":"r2","line":"LedgerTerm is written once."}]}}`,
	} {
		t.Run(name, func(t *testing.T) {
			c := NewCollector([]string{"a.py"})
			wrapper := c.Wrap(&testProvider{})
			prepared, err := llm.Prepare(wrapper, prompt, llm.Limits{})
			if err != nil {
				t.Fatal(err)
			}
			adapted, err := llm.AdaptResponse(wrapper, prepared.ResponseContext(), prepared.Bytes(), []byte(response))
			if err != nil {
				t.Fatal(err)
			}
			adapted.Accept([]string{"r1", "r2"})
			var texts []string
			for _, item := range c.pending {
				texts = append(texts, item.Texts...)
			}
			joined := strings.Join(texts, "|")
			if !strings.Contains(joined, "KeptAlias") || !strings.Contains(joined, "LedgerTerm is written once.") || strings.Contains(strings.ToLower(joined), "none") {
				t.Fatalf("glossary prose differs from the decoded rows: %q", texts)
			}
		})
	}
}
