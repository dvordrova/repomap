package terminology

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/modeldiag"
)

type testProvider struct {
	response []byte
	calls    int
	prompt   llm.Prompt
	limits   llm.Limits
	secret   string
}

func (*testProvider) State() []byte { return []byte(`{"model":"test"}`) }
func (p *testProvider) Prepare(prompt llm.Prompt, limits llm.Limits) (llm.Prepared, error) {
	p.prompt, p.limits = prompt, limits
	raw, err := json.Marshal(map[string]any{"messages": []map[string]string{{"role": "system", "content": prompt.System}, {"role": "user", "content": prompt.User}}})
	if err != nil {
		return llm.Prepared{}, err
	}
	return llm.NewPrepared(raw)
}
func (p *testProvider) Complete(context.Context, llm.Prepared) (llm.Completion, error) {
	p.calls++
	return llm.Completion{Response: p.response, FinishReason: llm.FinishStop, ChoiceCount: 1, Metrics: llm.Metrics{Attempts: 1}}, nil
}

func domainForTest(provider llm.Provider, request, response []byte) ([]byte, error) {
	adapted, err := llm.AdaptResponse(provider, request, response)
	return adapted.Domain, err
}
func acceptForTest(provider llm.Provider, request, response []byte) {
	acceptRowsForTest(provider, request, response, nil)
}
func acceptRowsForTest(provider llm.Provider, request, response []byte, rows []string) {
	adapted, err := llm.AdaptResponse(provider, request, response)
	if err == nil {
		adapted.Accepted(rows)
	}
}

func prepareForTest(t *testing.T, c *Collector, input string) (llm.Provider, []byte) {
	t.Helper()
	wrapped := c.Wrap(&testProvider{})
	prepared, err := llm.Prepare(wrapped, llm.Prompt{ResponseExample: `{"answer":"<computed answer>"}`, System: "Return the original table.", User: input}, llm.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	return wrapped, prepared.Bytes()
}
func termJSON(name, explanation, source string) map[string]any {
	return map[string]any{"name": name, "explanation": explanation, "sources": []string{source}}
}
func responseJSON(result any, terms ...map[string]any) []byte {
	if terms == nil {
		terms = []map[string]any{}
	}
	raw, _ := json.Marshal(map[string]any{"result": result, "terms": terms})
	return raw
}
func sourceRef(t *testing.T, c *Collector, request []byte, path, row string) string {
	t.Helper()
	ctx, err := c.requestContext(request)
	if err != nil {
		t.Fatal(err)
	}
	for ref, source := range ctx.sources {
		if source.Path == path && source.Row == row {
			return ref
		}
	}
	t.Fatalf("missing exact source %s / %s", path, row)
	return ""
}

func TestPrepareCatalogueUsesOnlyExistingSafeExactPathStrings(t *testing.T) {
	c := NewCollector([]string{"src/a.py", "src/b.go", "docs/readme.md", "not-sent.py", "/host/private", "../outside"})
	base := &testProvider{secret: "private-provider-key"}
	wrapped := c.Wrap(base)
	input := `{"context":{"path":"docs/readme.md","description":"Stocks API."},"rows":[{"key":"r1","path":"src/a.py","line":7,"name":"시가 총액","note":"src/b.go is a prose mention"},{"key":"r2","files":["src/b.go"],"line":99}]}`
	prompt := llm.Prompt{ResponseExample: `{"answer":"<computed answer>"}`, System: "Original instructions.", User: input, Reasoning: true}
	limits := llm.Limits{MaxRequestBytes: 12345, MaxOutputTokens: 678}
	first, err := llm.Prepare(wrapped, prompt, limits)
	if err != nil {
		t.Fatal(err)
	}
	second, err := llm.Prepare(wrapped, prompt, limits)
	if err != nil || !bytes.Equal(first.Bytes(), second.Bytes()) {
		t.Fatal("repeated preparation is nondeterministic")
	}
	if base.calls != 0 || len(c.Snapshot()) != 0 {
		t.Fatal("preparation made a call or collected unaccepted terms")
	}
	if base.limits != limits || !base.prompt.Reasoning || !strings.Contains(base.prompt.System, "Original instructions.") {
		t.Fatal("wrapper changed owning cube settings")
	}
	if prompt.ResponseFormatJSON || !base.prompt.ResponseFormatJSON {
		t.Fatal("an original non-object result did not enable JSON mode for the object envelope")
	}
	ctx, err := c.requestContext(first.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if len(ctx.sources) != 3 {
		t.Fatalf("catalogue: %+v", ctx.sources)
	}
	for _, source := range ctx.sources {
		switch source.Path {
		case "src/a.py":
			if source.Line != 7 || source.Row != "r1" {
				t.Fatalf("lost explicit location: %+v", source)
			}
		case "src/b.go":
			if source.Line != 0 || source.Row != "r2" {
				t.Fatalf("invented array-item line: %+v", source)
			}
		case "docs/readme.md":
			if source.Line != 0 || source.Row != "" {
				t.Fatalf("lost shared file source: %+v", source)
			}
		default:
			t.Fatalf("invented source: %+v", source)
		}
	}
	for _, absent := range []string{"not-sent.py", "/host/private", "../outside", base.secret} {
		if bytes.Contains(first.Bytes(), []byte(absent)) || bytes.Contains(wrapped.State(), []byte(absent)) {
			t.Fatalf("wrapper added absent/private material: %s", absent)
		}
	}
	if !strings.HasPrefix(base.prompt.User, input+catalogDelimiter) {
		t.Fatal("owning input was rewritten")
	}
	if strings.Count(base.prompt.User, catalogDelimiter) != 1 || strings.Count(base.prompt.System, "# Response envelope and repository terminology") != 1 {
		t.Fatal("preparation applied the envelope more than once")
	}
}

func TestCollectorPreservesDomainResultAndWarmsMetadataWithoutExtraCalls(t *testing.T) {
	paths := []string{"api.py"}
	c := NewCollector(paths)
	base := &testProvider{}
	wrapped := c.Wrap(base)
	call := llm.Call[map[string]string]{State: []byte(`{"contract":"test.v1"}`), Prompt: llm.Prompt{ResponseExample: `{"answer":"<computed answer>"}`, System: "Original shape: a JSON object.", User: `{"path":"api.py","line":17,"name":"시가 총액"}`}, Limits: llm.Limits{MaxRequestBytes: 1 << 20, MaxResponseBytes: 1 << 20, MaxOutputTokens: 1000}}
	prepared, err := llm.Prepare(wrapped, call.Prompt, call.Limits)
	if err != nil {
		t.Fatal(err)
	}
	ref := sourceRef(t, c, prepared.Bytes(), "api.py", "")
	base.response = responseJSON(map[string]string{"answer": "시가 총액 describes market capitalization."}, termJSON("시가 총액", "The market-value measure returned here.", ref))
	executor := llm.Executor{Enabled: true, RootDir: t.TempDir()}
	result, err := llm.ExecuteJSON(context.Background(), executor, wrapped, call)
	if err != nil || base.calls != 1 || result.Value["answer"] != "시가 총액 describes market capitalization." {
		t.Fatalf("domain result changed: %+v / %v", result.Value, err)
	}
	first := c.Snapshot()
	if len(first) != 1 || first[0].Sources[0] != (Source{Path: "api.py", Line: 17}) || len(first[0].Origins) != 1 {
		t.Fatalf("missing same-call metadata: %+v", first)
	}
	warmCollector := NewCollector(paths)
	warm, err := llm.ExecuteJSON(context.Background(), executor, warmCollector.Wrap(base), call)
	if err != nil || !warm.Cached || base.calls != 1 || !reflect.DeepEqual(first, warmCollector.Snapshot()) {
		t.Fatalf("cache lost metadata or made another call: %+v / %v", warmCollector.Snapshot(), err)
	}
	if !bytes.Equal(result.Response, base.response) {
		t.Fatal("cache outcome lost raw outer response")
	}
	if value, err := domainForTest(warmCollector.Wrap(base), result.Request, result.Response); err != nil || bytes.Contains(value, []byte(`"terms"`)) {
		t.Fatalf("domain did not get its own format: %s / %v", value, err)
	}
}

func TestBaseReplayRefreshesWrappedDomainAndTerminologyTogether(t *testing.T) {
	c := NewCollector([]string{"api.py"})
	base := &testProvider{}
	wrapped := c.Wrap(base)
	if !bytes.Equal(wrapped.State(), base.State()) {
		t.Fatal("adjunct changed transport/cache identity")
	}
	call := llm.Call[map[string]string]{State: []byte(`{"contract":"replay-test.v1"}`),
		Prompt: llm.Prompt{ResponseExample: `{"answer":"<computed answer>"}`, User: `{"path":"api.py","name":"API"}`},
		Limits: llm.Limits{MaxRequestBytes: 1 << 20, MaxResponseBytes: 1 << 20, MaxOutputTokens: 1000}}
	base.response = responseJSON(map[string]string{"answer": "API originally returned a row."}, termJSON("API", "The original explanation.", "g1"))
	executor := llm.Executor{Enabled: true, RootDir: t.TempDir()}
	first, err := llm.ExecuteJSON(context.Background(), executor, wrapped, call)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := llm.NewPrepared(first.Request)
	if err != nil {
		t.Fatal(err)
	}
	base.response = responseJSON(map[string]string{"answer": "API now returns a table."}, termJSON("API", "The refreshed explanation.", "g1"))
	replay, err := llm.ReplayJSON(context.Background(), executor, base, prepared)
	if err != nil || replay.CacheKey != first.CacheKey {
		t.Fatalf("base replay refreshed another cache entry: %s / %s / %v", replay.CacheKey, first.CacheKey, err)
	}
	fresh := NewCollector([]string{"api.py"})
	accepted, err := llm.ExecuteJSON(context.Background(), executor, fresh.Wrap(base), call)
	if err != nil || !accepted.Cached || base.calls != 2 || accepted.Value["answer"] != "API now returns a table." {
		t.Fatalf("ordinary reuse missed replay: value=%v cached=%v calls=%d err=%v", accepted.Value, accepted.Cached, base.calls, err)
	}
	terms := fresh.Snapshot()
	if len(terms) != 1 || terms[0].Explanation != "The refreshed explanation." || !reflect.DeepEqual(terms[0].Origins, c.Snapshot()[0].Origins) {
		t.Fatalf("replayed domain did not carry refreshed same-request terms: %+v", terms)
	}
}

func TestRowMemoRestoresOnlyAcceptedResultRowsAndSourceOwnership(t *testing.T) {
	paths := []string{"a.py", "b.py", "README.md"}
	c := NewCollector(paths)
	_, request := prepareForTest(t, c, `{"context":{"path":"README.md","description":"Shared context."},"rows":[{"key":"r1","path":"a.py","name":"Tokenizer"},{"key":"r2","path":"b.py","name":"Storage"}]}`)
	a, b, shared := sourceRef(t, c, request, "a.py", "r1"), sourceRef(t, c, request, "b.py", "r2"), sourceRef(t, c, request, "README.md", "")
	result := map[string]any{"overview": "Global note explains the glossary.", "rows": []map[string]string{{"key": "r1", "line": "Tokenizer uses Shared context."}, {"key": "r2", "line": "Storage and Tokenizer differ."}}}
	kept := termJSON("Tokenizer", "Its first source-backed meaning.", a)
	kept["sources"] = []string{a, shared, a, "g999"}
	response := responseJSON(result, kept,
		termJSON("Storage", "The other row's meaning.", b),
		termJSON("Tokenizer", "The second row's meaning despite a shared name.", b),
		termJSON("Global note", "A global-only explanation.", shared),
		termJSON("Storage", "A mismatched citation from another row.", a))
	// This collector never prepared the batch. The original request is enough.
	restored := NewCollector(paths)
	wrapped := restored.Wrap(&testProvider{})
	if _, err := domainForTest(wrapped, request, response); err != nil {
		t.Fatal(err)
	}
	if len(restored.Snapshot()) != 0 {
		t.Fatal("unwrapping collected metadata before domain acceptance")
	}
	acceptRowsForTest(wrapped, request, response, []string{"r1"})
	terms := restored.Snapshot()
	if len(terms) != 1 || terms[0].Explanation != "Its first source-backed meaning." || len(terms[0].Sources) != 2 {
		t.Fatalf("stale sibling/global metadata leaked through row reuse: %+v", terms)
	}
	acceptRowsForTest(wrapped, request, response, []string{})
	if !reflect.DeepEqual(terms, restored.Snapshot()) {
		t.Fatal("empty accepted row set admitted terms")
	}
	full := NewCollector(paths)
	acceptForTest(full.Wrap(&testProvider{}), request, response)
	if len(full.Snapshot()) != 4 {
		t.Fatalf("whole accepted call lost supported contextual terms: %+v", full.Snapshot())
	}
}

func TestTermsFilterUnknownRefsDeduplicateExactlyAndKeepDifferentMeanings(t *testing.T) {
	c := NewCollector([]string{"api.py"})
	wrapped, request := prepareForTest(t, c, `{"path":"api.py","name":"API"}`)
	ref := sourceRef(t, c, request, "api.py", "")
	first := termJSON("API", "The local public interface.", ref)
	first["sources"] = []string{ref, ref, "g999"}
	second := termJSON("API", "A distinct contextual meaning.", ref)
	unsupported := termJSON("API", "Unknown-only evidence.", "g999")
	response := responseJSON(map[string]string{"value": "API"}, first, first, second, unsupported)
	if _, err := domainForTest(wrapped, request, response); err != nil {
		t.Fatal(err)
	}
	var group sync.WaitGroup
	for i := 0; i < 20; i++ {
		group.Add(1)
		go func() { defer group.Done(); acceptForTest(wrapped, request, response) }()
	}
	group.Wait()
	terms := c.Snapshot()
	if len(terms) != 2 {
		t.Fatalf("identity confused meanings or unsupported refs: %+v", terms)
	}
	for _, term := range terms {
		if len(term.Sources) != 1 || len(term.Origins) != 1 {
			t.Fatalf("replay duplicated provenance: %+v", term)
		}
	}
	terms[0].Sources[0].Path = "changed"
	terms[0].Origins[0].Row = "changed"
	if strings.Contains(fmt.Sprint(c.Snapshot()), "changed") {
		t.Fatal("snapshot aliases collector memory")
	}
}

func TestRowMemoPartitionKeepsOriginalTermVariant(t *testing.T) {
	paths := []string{"a.py", "b.py", "c.py"}
	cold := NewCollector(paths)
	wrapped, request := prepareForTest(t, cold, `{"rows":[{"key":"r1","path":"a.py"},{"key":"r2","path":"b.py"},{"key":"r3","path":"c.py"}]}`)
	term := termJSON("Interaction", "A user action.", sourceRef(t, cold, request, "a.py", "r1"))
	term["sources"] = []string{sourceRef(t, cold, request, "a.py", "r1"), sourceRef(t, cold, request, "b.py", "r2"), sourceRef(t, cold, request, "c.py", "r3")}
	response := responseJSON(map[string]any{"rows": []map[string]string{
		{"key": "r1", "line": "Interaction"}, {"key": "r2", "line": "Interaction"}, {"key": "r3", "line": "Interaction"},
	}}, term)
	// r3 is refused in both paths; its source must stay absent even though it
	// belongs to the original term's identity.
	acceptRowsForTest(wrapped, request, response, []string{"r1", "r2"})
	warm := NewCollector(paths)
	for _, row := range []string{"r2", "r1", "r2"} {
		acceptRowsForTest(warm.Wrap(&testProvider{}), request, response, []string{row})
	}
	first, second := cold.Snapshot(), warm.Snapshot()
	if len(first) != 1 || !reflect.DeepEqual(first, second) || !reflect.DeepEqual(first[0].Sources, []Source{{Path: "a.py"}, {Path: "b.py"}}) || len(first[0].Origins) != 2 {
		t.Fatalf("row reuse changed the original accepted term: cold=%+v warm=%+v", first, second)
	}
}

func TestSharedEvidenceRetainsExactAcceptedAnswerRowOrigins(t *testing.T) {
	paths := []string{"README.md"}
	preparedBy := NewCollector(paths)
	_, request := prepareForTest(t, preparedBy, `{"context":{"path":"README.md"},"rows":[{"key":"r1","question":"Prices?"},{"key":"r2","question":"Symbols?"}]}`)
	ref := sourceRef(t, preparedBy, request, "README.md", "")
	response := responseJSON(map[string]any{
		"overview": "Global note.",
		"rows": []map[string]string{
			{"key": "r1", "answer": "OHLCV uses a DataFrame."},
			{"key": "r2", "answer": "Ticker uses a DataFrame."},
		},
	}, termJSON("OHLCV", "Per-period prices and traded volume.", ref),
		termJSON("Ticker", "A listed instrument's code.", ref),
		termJSON("DataFrame", "A labelled data table.", ref),
		termJSON("Global note", "A shared introductory note.", ref))
	requestSHA := fmt.Sprintf("%x", sha256.Sum256(request))
	for _, test := range []struct {
		name string
		rows []string
		want map[string][]Origin
	}{
		{"whole response", nil, map[string][]Origin{
			"OHLCV":       {{RequestSHA256: requestSHA, Row: "r1"}},
			"Ticker":      {{RequestSHA256: requestSHA, Row: "r2"}},
			"DataFrame":   {{RequestSHA256: requestSHA, Row: "r1"}, {RequestSHA256: requestSHA, Row: "r2"}},
			"Global note": {{RequestSHA256: requestSHA, Row: ""}},
		}},
		{"one recalled row", []string{"r2"}, map[string][]Origin{
			"Ticker":    {{RequestSHA256: requestSHA, Row: "r2"}},
			"DataFrame": {{RequestSHA256: requestSHA, Row: "r2"}},
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			collector := NewCollector(paths)
			adapted, err := llm.AdaptResponse(collector.Wrap(&testProvider{}), request, response)
			if err != nil || len(adapted.Rejections) != 0 {
				t.Fatalf("valid terms were not adapted: %+v / %v", adapted.Rejections, err)
			}
			// Repeated memo acceptance uses the same parsed response and cannot
			// add a sibling row, a global origin, or duplicate provenance.
			adapted.Accepted(test.rows)
			adapted.Accepted(test.rows)
			got := make(map[string][]Origin)
			for _, candidate := range collector.Snapshot() {
				got[candidate.Name] = candidate.Origins
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("answer-row provenance = %+v, want %+v", got, test.want)
			}
			snapshot := collector.Snapshot()
			snapshot[0].Origins[0].RequestSHA256 = "changed request"
			snapshot[0].Origins[0].Row = "changed row"
			for _, candidate := range collector.Snapshot() {
				if !reflect.DeepEqual(candidate.Origins, test.want[candidate.Name]) {
					t.Fatal("snapshot aliases accepted response origins")
				}
			}
		})
	}
}

func TestSharedSourcesUseOnlyExplicitClosedResultRows(t *testing.T) {
	paths := []string{"finance.py", "river.py", "shared.py", "unknown.py", "empty.py", "malformed.py"}
	collector := NewCollector(paths)
	_, request := prepareForTest(t, collector, `{"context":{"candidates":[
		{"path":"finance.py","line":7,"result_rows":["r1"]},
		{"path":"river.py","line":8,"result_rows":["r2"]},
		{"path":"shared.py","line":9,"result_rows":["r2","r1","r1","unknown"]},
		{"path":"unknown.py","result_rows":["unknown"]},
		{"path":"empty.py","result_rows":[]},
		{"path":"malformed.py","result_rows":"r1"}
	]},"rows":[{"key":"r1","question":"Finance?"},{"key":"r2","question":"Geography?"}]}`)
	ctx, err := collector.requestContext(request)
	if err != nil || len(ctx.sources) != 4 {
		t.Fatalf("unexpected explicit source memberships: %+v / %v", ctx.sources, err)
	}
	for _, source := range ctx.sources {
		if source.Row != "r1" && source.Row != "r2" {
			t.Fatalf("unsupported membership gained shared authority: %+v", source)
		}
	}
	finance := sourceRef(t, collector, request, "finance.py", "r1")
	river := sourceRef(t, collector, request, "river.py", "r2")
	shared1 := sourceRef(t, collector, request, "shared.py", "r1")
	shared2 := sourceRef(t, collector, request, "shared.py", "r2")
	sharedTerm := termJSON("Record", "A returned data row.", shared1)
	sharedTerm["sources"] = []string{shared2, shared1}
	response := responseJSON(map[string]any{"rows": []map[string]string{
		{"key": "r1", "answer": "A bank supplies a Record."},
		{"key": "r2", "answer": "A bank borders the river in a Record."},
	}}, termJSON("bank", "A financial institution.", finance),
		termJSON("bank", "The land beside a river.", river), sharedTerm)
	requestSHA := fmt.Sprintf("%x", sha256.Sum256(request))
	for _, rows := range [][]string{nil, {"r1"}} {
		restored := NewCollector(paths)
		adapted, err := llm.AdaptResponse(restored.Wrap(&testProvider{}), request, response)
		if err != nil || len(adapted.Rejections) != 0 {
			t.Fatalf("supported row-scoped sources rejected: %+v / %v", adapted.Rejections, err)
		}
		adapted.Accepted(rows)
		terms := restored.Snapshot()
		wantCount := 3
		if rows != nil {
			wantCount = 2
		}
		if len(terms) != wantCount {
			t.Fatalf("wrong recalled meanings for %v: %+v", rows, terms)
		}
		for _, term := range terms {
			want := []Origin{{RequestSHA256: requestSHA, Row: "r1"}}
			if term.Explanation == "The land beside a river." {
				want[0].Row = "r2"
			} else if term.Name == "Record" && rows == nil {
				want = append(want, Origin{RequestSHA256: requestSHA, Row: "r2"})
			}
			if !reflect.DeepEqual(term.Origins, want) {
				t.Fatalf("same spelling crossed explicit source memberships: %+v; want %+v", term, want)
			}
		}
	}
}

func TestResultEnvelopeRejectsMissingAndMalformedDomain(t *testing.T) {
	c := NewCollector([]string{"api.py"})
	wrapped, request := prepareForTest(t, c, `{"path":"api.py"}`)
	for _, raw := range []string{`{"answer":"API"}`, `{"terms":[]}`, `{"result":null,"terms":[]}`, `{"result":{"answer":"API"},"terms":[}`} {
		if _, err := llm.AdaptResponse(wrapped, request, []byte(raw)); err == nil {
			t.Fatalf("accepted a missing or malformed result envelope: %s", raw)
		}
	}
}

func TestMalformedOptionalTermsKeepGoodDomainAndValidSiblings(t *testing.T) {
	valid := `{"name":"HTTP","explanation":"A request protocol.","sources":["g1"]}`
	cases := map[string]string{
		"absent":             ``,
		"null":               `,"terms":null`,
		"object":             `,"terms":{}`,
		"string":             `,"terms":"wrong"`,
		"null member":        `,"terms":[null,` + valid + `]`,
		"wrong fields":       `,"terms":[{"name":42,"sources":[]},` + valid + `]`,
		"unknown field":      `,"terms":[{"name":"HTTP","explanation":"Wrong field.","sources":["g1"],"translation":"preserve"},` + valid + `]`,
		"empty explanation":  `,"terms":[{"name":"HTTP","explanation":"","sources":["g1"]},` + valid + `]`,
		"wrong source shape": `,"terms":[{"name":"HTTP","explanation":"Wrong shape.","sources":"g1"},` + valid + `]`,
		"unknown envelope":   `,"terms":[` + valid + `],"unexpected":true`,
	}
	for name, suffix := range cases {
		t.Run(name, func(t *testing.T) {
			c := NewCollector([]string{"api.py"})
			base := &testProvider{response: []byte(`{"result":{"answer":"HTTP works."}` + suffix + `}`)}
			var events []llm.Event
			executor := llm.Executor{Enabled: true, RootDir: t.TempDir(), Observer: llm.ObserverFunc(func(event llm.Event) error { events = append(events, event); return nil })}
			call := llm.Call[map[string]string]{State: []byte(`{"test":"optional"}`), Prompt: llm.Prompt{ResponseExample: `{"answer":"<computed>"}`, User: `{"path":"api.py"}`}, Limits: llm.Limits{MaxRequestBytes: 100000, MaxResponseBytes: 100000, MaxOutputTokens: 1000}}
			for i := 0; i < 2; i++ {
				value, err := llm.ExecuteJSON(t.Context(), executor, c.Wrap(base), call)
				if err != nil || value.Value["answer"] != "HTTP works." || value.Cached != (i == 1) || !bytes.Equal(value.Response, base.response) {
					t.Fatalf("metadata refusal discarded or rewrote good domain: %+v / %v", value, err)
				}
				if len(events) != i+1 || len(events[i].ResponseRejections) == 0 || events[i].Failure != llm.FailureNone {
					t.Fatalf("missing metadata diagnostics or duplicate/failure event: %+v", events)
				}
			}
			want := 0
			if strings.Contains(suffix, valid) {
				want = 1
			}
			if base.calls != 1 || len(c.Snapshot()) != want {
				t.Fatalf("calls=%d candidates=%+v", base.calls, c.Snapshot())
			}
		})
	}
}

func TestOnlyComputedAnswerTermsSurviveAndCacheWithoutExtraCalls(t *testing.T) {
	c := NewCollector([]string{"api.py"})
	base := &testProvider{}
	wrapped := c.Wrap(base)
	call := llm.Call[map[string]string]{State: []byte(`{"contract":"result-terms.v2"}`), Prompt: llm.Prompt{ResponseExample: `{"answer":"<computed answer>"}`, User: `{"path":"api.py","name":"InputOnly","signature":"(newCode: string) => void"}`}, Limits: llm.Limits{MaxRequestBytes: 1 << 20, MaxResponseBytes: 1 << 20, MaxOutputTokens: 1000}}
	valid := termJSON("HTTP", "The supported protocol.", "g1")
	unsupported := []map[string]any{
		termJSON("InputOnly", "An input name never used in the computed answer.", "g1"),
		termJSON("antd", "A name absent from the answer.", "g1"),
		termJSON("API", "Unsupported word substring.", "g1"),
		termJSON("HTTP", "Unsupported source ref.", "g999"),
		termJSON(" HTTP ", "A bad name is not trimmed into authority.", "g1"),
		termJSON("", "An empty name has no occurrence.", "g1"),
		termJSON("http", "Case is not repaired into a known name.", "g1"),
	}
	answer := map[string]string{"answer": "HTTP carries the CAPITAL value."}
	base.response = responseJSON(answer, append(unsupported, valid)...)
	executor := llm.Executor{Enabled: true, RootDir: t.TempDir()}
	result, err := llm.ExecuteJSON(context.Background(), executor, wrapped, call)
	if err != nil || !reflect.DeepEqual(result.Value, answer) || base.calls != 1 {
		t.Fatalf("optional unsupported refs invalidated the computed answer: %+v / %v", result.Value, err)
	}
	terms := c.Snapshot()
	if len(terms) != 1 || terms[0].Explanation != "The supported protocol." {
		t.Fatalf("unsupported refs gained authority or removed valid siblings: %+v", terms)
	}
	warmCollector := NewCollector([]string{"api.py"})
	warm, err := llm.ExecuteJSON(context.Background(), executor, warmCollector.Wrap(base), call)
	if err != nil || !warm.Cached || base.calls != 1 || !reflect.DeepEqual(warmCollector.Snapshot(), terms) {
		t.Fatalf("warm reuse changed optional filtering or made another call: %+v / %v", warmCollector.Snapshot(), err)
	}
}

func TestExplicitResultRowUsesOnlyAdvertisedSourceRowRefs(t *testing.T) {
	paths := []string{"prices.py", "value.py"}
	c := NewCollector(paths)
	_, request := prepareForTest(t, c, `{"rows":[{"key":"r1","path":"prices.py"},{"key":"r2","path":"value.py"}]}`)
	prices := sourceRef(t, c, request, "prices.py", "r1")
	value := sourceRef(t, c, request, "value.py", "r2")
	result, _ := jsonValue([]byte(`{"questions":[{"key":"q1","selections":[{"row":"r1","anchors":["a1"],"why":"OHLCV describes the returned price fields."},{"row":"r2","why":"Market capitalization measures equity value."},{"row":"r999","why":"Unbound appears at an unknown row."}]}]}`))
	response := responseJSON(result,
		termJSON("OHLCV", "The named price and volume fields.", prices),
		termJSON("Market capitalization", "The market value of equity.", value),
		termJSON("Unbound", "An unsupported row must not acquire authority.", prices))
	wrapped := c.Wrap(&testProvider{})
	if _, err := domainForTest(wrapped, request, response); err != nil {
		t.Fatal(err)
	}
	acceptRowsForTest(wrapped, request, response, []string{"r1"})
	terms := c.Snapshot()
	if len(terms) != 1 || terms[0].Name != "OHLCV" || terms[0].Sources[0].Path != "prices.py" {
		t.Fatalf("known explicit row did not supersede its outer question key: %+v", terms)
	}
	full := NewCollector(paths)
	acceptForTest(full.Wrap(&testProvider{}), request, response)
	if len(full.Snapshot()) != 2 {
		t.Fatalf("unknown row acquired source authority: %+v", full.Snapshot())
	}
}

func TestExactCurrentDelimiterSelectsEnvelopeWithoutOldAdapters(t *testing.T) {
	c := NewCollector([]string{"api.py"})
	wrapped, request := prepareForTest(t, c, `{"path":"api.py"}`)
	response := responseJSON(map[string]string{"answer": "HTTP carries requests."}, termJSON("HTTP", "The web request protocol.", "g1"))
	if _, err := domainForTest(wrapped, request, response); err != nil {
		t.Fatal(err)
	}
	wrongContract := bytes.ReplaceAll(request, []byte(adjunctVersion), []byte("unsupported contract"))
	if _, err := domainForTest(wrapped, wrongContract, response); err == nil {
		t.Fatal("current delimiter accepted a different contract")
	}
	old := bytes.ReplaceAll(request, []byte("REPOMAP_TERMINOLOGY_CATALOG_V3"), []byte("REPOMAP_TERMINOLOGY_CATALOG_V2"))
	old = bytes.ReplaceAll(old, []byte(adjunctVersion), []byte("repomap.terminology.adjunct.v2"))
	adapted, err := llm.AdaptResponse(wrapped, old, response)
	if err != nil || !bytes.Equal(adapted.Domain, response) || adapted.Accept != nil || len(adapted.Rejections) != 0 {
		t.Fatal("an old delimiter acquired an implicit unwrapping or metadata adapter")
	}
}

func TestSourceRowsCannotClaimUnrelatedResultRowOccurrences(t *testing.T) {
	c := NewCollector([]string{"first.py", "second.py"})
	_, request := prepareForTest(t, c, `{"rows":[{"key":"r1","path":"first.py","name":"API"},{"key":"r2","path":"second.py","name":"CLI"}]}`)
	firstRef := sourceRef(t, c, request, "first.py", "r1")
	secondRef := sourceRef(t, c, request, "second.py", "r2")
	term := termJSON("CLI", "The command interface in the second row.", secondRef)
	term["sources"] = []string{firstRef, secondRef}
	response := responseJSON(map[string]any{"rows": []map[string]any{{"key": "r1", "answer": "API is available."}, {"key": "r2", "nested": map[string]any{"details": []string{"The CLI is available."}}}}}, term)
	fresh := NewCollector([]string{"first.py", "second.py"})
	wrapped := fresh.Wrap(&testProvider{})
	if _, err := domainForTest(wrapped, request, response); err != nil {
		t.Fatal(err)
	}
	acceptRowsForTest(wrapped, request, response, []string{"r1"})
	if len(fresh.Snapshot()) != 0 {
		t.Fatal("an unrelated source claimed ownership of a recalled result row")
	}
	acceptRowsForTest(wrapped, request, response, []string{"r2"})
	terms := fresh.Snapshot()
	if len(terms) != 1 || len(terms[0].Sources) != 1 || terms[0].Sources[0].Path != "second.py" {
		t.Fatalf("actual row mention or its source was lost: %+v", terms)
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
		{"Custom Matchers", "Matcher", false},
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

func TestTermMentionAcceptsLatinNameBesideKoreanParticle(t *testing.T) {
	c := NewCollector([]string{"docs/custom-matcher.md"})
	wrapped, request := prepareForTest(t, c, `{"documents":[{"path":"docs/custom-matcher.md","content":"커스텀 Matcher를 사용합니다.\nCustom Matchers"}]}`)
	ref := sourceRef(t, c, request, "docs/custom-matcher.md", "")
	result := map[string]string{"summary": "커스텀 Matcher를 사용합니다.\nCustom Matchers"}
	response := responseJSON(result, termJSON("Matcher", "The custom matching interface described in this document.", ref))
	unwrapped, err := domainForTest(wrapped, request, response)
	want, _ := json.Marshal(result)
	if err != nil || !bytes.Equal(unwrapped, want) {
		t.Fatalf("exact Korean-context mention rejected or original result changed: %s / %v", unwrapped, err)
	}
	acceptForTest(wrapped, request, response)
	terms := c.Snapshot()
	if len(terms) != 1 || terms[0].Name != "Matcher" || len(terms[0].Sources) != 1 || terms[0].Sources[0].Path != "docs/custom-matcher.md" {
		t.Fatalf("accepted exact mention lost its closed source: %+v", terms)
	}
}

func TestRejectedDomainDoesNotCollectAndCurrentCorpusFiltersOldSources(t *testing.T) {
	c := NewCollector([]string{"api.py"})
	base := &testProvider{}
	wrapped := c.Wrap(base)
	call := llm.Call[map[string]string]{Prompt: llm.Prompt{ResponseExample: `{"answer":"<computed answer>"}`, User: `{"path":"api.py","name":"API"}`}, Limits: llm.Limits{MaxRequestBytes: 1 << 20, MaxResponseBytes: 1 << 20, MaxOutputTokens: 1000}, Validate: func(map[string]string) error { return fmt.Errorf("domain rejected") }}
	prepared, err := llm.Prepare(wrapped, call.Prompt, call.Limits)
	if err != nil {
		t.Fatal(err)
	}
	base.response = responseJSON(map[string]string{"answer": "API"}, termJSON("API", "An interface.", "g1"))
	if _, err := llm.ExecuteJSON(context.Background(), llm.Executor{}, wrapped, call); err == nil || !strings.Contains(err.Error(), "domain rejected") || base.calls != 1 || len(c.Snapshot()) != 0 {
		t.Fatal("domain refusal collected metadata")
	}
	current := NewCollector(nil)
	currentWrapped := current.Wrap(base)
	if _, err := domainForTest(currentWrapped, prepared.Bytes(), base.response); err != nil {
		t.Fatal(err)
	}
	acceptForTest(currentWrapped, prepared.Bytes(), base.response)
	if len(current.Snapshot()) != 0 {
		t.Fatal("a source outside the current corpus gained glossary authority")
	}
}

func TestSavedCatalogueCannotInventPathLineOrRow(t *testing.T) {
	c := NewCollector([]string{"api.py", "not-sent.py"})
	wrapped, request := prepareForTest(t, c, `{"rows":[{"key":"r1","path":"api.py","line":9,"name":"API"}]}`)
	response := responseJSON(map[string]string{"answer": "API"}, termJSON("API", "An interface.", "g1"))
	if _, err := domainForTest(wrapped, request, response); err != nil {
		t.Fatalf("original catalogue was rejected: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(request, &body); err != nil {
		t.Fatal(err)
	}
	messages := body["messages"].([]any)
	user := messages[1].(map[string]any)
	content := user["content"].(string)
	start := strings.LastIndex(content, catalogDelimiter) + len(catalogDelimiter)
	var originalCatalog sourceCatalog
	if err := json.Unmarshal([]byte(content[start:]), &originalCatalog); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []catalogSource{
		{Ref: "g1", Path: "not-sent.py", Line: 0, Row: "r1"},
		{Ref: "g1", Path: "api.py", Line: 10, Row: "r1"},
		{Ref: "g1", Path: "api.py", Line: 9, Row: "r2"},
	} {
		catalogue, _ := json.Marshal(sourceCatalog{Version: adjunctVersion, Sources: []catalogSource{invalid}, ResponseContract: strings.TrimSpace(responseContract), ResponseExample: originalCatalog.ResponseExample})
		user["content"] = content[:start] + string(catalogue)
		tampered, _ := json.Marshal(body)
		if _, err := domainForTest(wrapped, tampered, response); err == nil {
			t.Fatalf("invented source metadata was accepted: %+v", invalid)
		}
	}
}

func TestNoSourceModeKeepsExactOriginalDomainShapes(t *testing.T) {
	c := NewCollector(nil)
	wrapped, request := prepareForTest(t, c, `{"question":"Classify this name without source evidence."}`)
	for _, result := range []any{map[string]any{"nested": []any{true, nil}}, []any{"one", "two"}, "choice", nil} {
		response, _ := json.Marshal(result)
		unwrapped, err := domainForTest(wrapped, request, response)
		want, _ := json.Marshal(result)
		if err != nil || !bytes.Equal(unwrapped, want) {
			t.Fatalf("empty adjunct changed domain shape: %s / %v", unwrapped, err)
		}
		acceptForTest(wrapped, request, response)
	}
	if len(c.Snapshot()) != 0 {
		t.Fatal("empty evidence manufactured a glossary")
	}
}

func TestArrayAnswerOwnerKeepsItsSchemaWithoutGenericTableExample(t *testing.T) {
	c := NewCollector([]string{"api.py"})
	base := &testProvider{}
	wrapped := c.Wrap(base)
	owner := llm.Prompt{ResponseExample: `[{"file_ref":"<supplied ref>","classifications":[]}]`, System: "Classify the supplied file references. Each file has file_ref and classifications.", User: `{"path":"api.py","file_ref":"f1"}`}
	call := llm.Call[[]map[string]any]{Prompt: owner, Limits: llm.Limits{MaxRequestBytes: 1 << 20, MaxResponseBytes: 1 << 20, MaxOutputTokens: 1000}}
	answer := []map[string]any{{"file_ref": "f1", "classifications": []any{map[string]any{"class": "documentation", "hypotheses": []string{"The guidance documents this source."}}}}}
	// A misplaced table wrapper stays invalid for the owning array decoder.
	base.response = responseJSON(map[string]any{"rows": answer})
	if _, err := llm.ExecuteJSON(context.Background(), llm.Executor{}, wrapped, call); err == nil {
		t.Fatal("an array answer was repaired from a table-shaped object")
	}
	base.response = responseJSON(answer)
	result, err := llm.ExecuteJSON(context.Background(), llm.Executor{}, wrapped, call)
	if err != nil || len(result.Value) != 1 || result.Value[0]["file_ref"] != "f1" || base.calls != 2 {
		t.Fatalf("the owner's direct array answer did not survive the envelope: %+v / %v", result.Value, err)
	}
	if !strings.Contains(base.prompt.System, owner.System+"\n\n"+strings.TrimSpace(adjunctPrompt)) || !strings.HasPrefix(base.prompt.User, owner.User+catalogDelimiter) {
		t.Fatal("the owning array schema was rewritten")
	}
	if strings.Contains(adjunctPrompt+responseContract, `"rows":`) {
		t.Fatal("the common adjunct still imposes a table example on non-table owners")
	}
	var catalog sourceCatalog
	suffix := strings.Split(base.prompt.User, catalogDelimiter)[1]
	if err := json.Unmarshal([]byte(suffix), &catalog); err != nil {
		t.Fatal(err)
	}
	var example struct{ Result json.RawMessage }
	if err := json.Unmarshal(catalog.ResponseExample, &example); err != nil || len(example.Result) == 0 || example.Result[0] != '[' {
		t.Fatalf("final envelope example lost the owner's array: %s / %v", catalog.ResponseExample, err)
	}
}

func TestPrepareRequiresOwnerResponseExampleAndUsesItsExactContainers(t *testing.T) {
	base := &testProvider{}
	wrapped := NewCollector([]string{"api.py"}).Wrap(base)
	for _, invalid := range []string{"", "not JSON", "{} []"} {
		if _, err := llm.Prepare(wrapped, llm.Prompt{ResponseExample: invalid, User: `{"path":"api.py"}`}, llm.Limits{}); err == nil {
			t.Fatalf("missing or invalid owner shape was guessed: %q", invalid)
		}
	}
	for _, owner := range []string{`{"rows":[{"key":"r1","activation":"<computed>","operation":"<computed>"}]}`, `{"reviews":[{"intent":"<supplied>","questions":[]}]}`, `{"overview":"<computed>","sources":[]}`} {
		if _, err := llm.Prepare(wrapped, llm.Prompt{ResponseExample: owner, User: `{"path":"api.py"}`}, llm.Limits{}); err != nil {
			t.Fatal(err)
		}
		var catalog sourceCatalog
		if err := json.Unmarshal([]byte(strings.Split(base.prompt.User, catalogDelimiter)[1]), &catalog); err != nil {
			t.Fatal(err)
		}
		var example struct {
			Result any   `json:"result"`
			Terms  []any `json:"terms"`
		}
		var want any
		if err := json.Unmarshal(catalog.ResponseExample, &example); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal([]byte(owner), &want); err != nil || !reflect.DeepEqual(example.Result, want) || example.Terms == nil || len(example.Terms) != 0 {
			t.Fatalf("owner's computed-answer container was changed: %s / %v", catalog.ResponseExample, err)
		}
	}
}

func TestNoSourcesPassExactOriginalPromptWithoutAnyProtocol(t *testing.T) {
	c := NewCollector([]string{"api.py"})
	base := &testProvider{}
	wrapped := c.Wrap(base)
	for _, input := range []string{`{"choice":"f1"}`, "Choose a closed file.\nEvidence:\n{\"path\":\"api.py\"}"} {
		owner := llm.Prompt{System: "Choose directly.", User: input, Reasoning: true}
		prepared, err := llm.Prepare(wrapped, owner, llm.Limits{})
		if err != nil {
			t.Fatal(err)
		}
		actualPrompt := base.prompt
		expected, err := llm.Prepare(base, owner, llm.Limits{})
		if err != nil || !bytes.Equal(prepared.Bytes(), expected.Bytes()) || !reflect.DeepEqual(actualPrompt, base.prompt) {
			t.Fatal("source-free bytes changed")
		}
		for _, raw := range []string{`{"result":"an original domain field","terms":42}`, `[{"answer":"direct"}]`} {
			adapted, err := llm.AdaptResponse(wrapped, prepared.Bytes(), []byte(raw))
			if err != nil || string(adapted.Domain) != raw || adapted.Accept != nil || len(adapted.Rejections) != 0 {
				t.Fatalf("inspected or repaired unwrapped answer: %+v / %v", adapted, err)
			}
		}
	}
}

func TestSourceBackedRequestIsDeterministicOwnerEnvelope(t *testing.T) {
	base := &testProvider{}
	collector := NewCollector([]string{"api.py"})
	owner := llm.Prompt{System: "Compute the table answer.", User: `{"rows":[{"key":"r1","path":"api.py","line":7,"name":"HTTP"}]}`, ResponseExample: `{"rows":[{"key":"r1","line":"<computed answer>"}]}`, Reasoning: true}
	limits := llm.Limits{MaxRequestBytes: 100000, MaxResponseBytes: 100000, MaxOutputTokens: 1000}
	current, err := llm.Prepare(collector.Wrap(base), owner, limits)
	if err != nil {
		t.Fatal(err)
	}
	currentPrompt := base.prompt
	// Preparation remains reproducible from the owner schema and closed sources.
	example, _ := json.Marshal(struct {
		Result json.RawMessage `json:"result"`
		Terms  []termWire      `json:"terms"`
	}{json.RawMessage(owner.ResponseExample), []termWire{}})
	catalog, _ := json.Marshal(struct {
		Version          string          `json:"version"`
		Sources          []catalogSource `json:"sources"`
		ResponseContract string          `json:"response_contract"`
		ResponseExample  json.RawMessage `json:"response_example"`
	}{adjunctVersion, []catalogSource{{Ref: "g1", Path: "api.py", Line: 7, Row: "r1"}}, strings.TrimSpace(responseContract), example})
	previous := owner
	previous.System += "\n\n" + strings.TrimSpace(adjunctPrompt)
	previous.User += catalogDelimiter + string(catalog)
	previous.ResponseFormatJSON = true
	previous.ResponseExample = ""
	expected, err := llm.Prepare(base, previous, limits)
	if err != nil || !bytes.Equal(current.Bytes(), expected.Bytes()) || !reflect.DeepEqual(currentPrompt, base.prompt) {
		t.Fatalf("source-backed prepared bytes or provider controls changed: %v", err)
	}
}

func TestNoSourceArrayAndObjectCacheReplayKeepDirectAnswers(t *testing.T) {
	for _, array := range []bool{false, true} {
		t.Run(fmt.Sprint("array=", array), func(t *testing.T) {
			makeAnswer := func(value string) []byte {
				var answer any = map[string]string{"answer": value}
				if array {
					answer = []any{answer}
				}
				raw, _ := json.Marshal(answer)
				return raw
			}
			base := &testProvider{response: makeAnswer("first")}
			collector := NewCollector([]string{"not-sent.py"})
			wrapped := collector.Wrap(base)
			call := llm.Call[json.RawMessage]{State: []byte(`{"contract":"no-source-mode-test.v1"}`), Prompt: llm.Prompt{System: "Compute the answer directly.", User: `{"question":"Choose a closed option."}`, ResponseExample: string(makeAnswer("<computed answer>")), ResponseFormatJSON: !array, Reasoning: true}, Limits: llm.Limits{MaxRequestBytes: 100000, MaxResponseBytes: 100000, MaxOutputTokens: 1000}, Validate: func(raw json.RawMessage) error {
				if (raw[0] == '[') != array {
					return fmt.Errorf("wrong owning answer type")
				}
				return nil
			}}
			executor := llm.Executor{RootDir: t.TempDir(), Enabled: true}
			first, err := llm.ExecuteJSON(context.Background(), executor, wrapped, call)
			if err != nil || base.calls != 1 || !bytes.Equal(first.Value, makeAnswer("first")) || len(collector.Snapshot()) != 0 {
				t.Fatalf("direct original answer was changed or collected terms: %+v / %v", first, err)
			}
			if base.prompt.ResponseFormatJSON != !array || !base.prompt.Reasoning || strings.Contains(base.prompt.System, "# Response envelope and repository terminology") {
				t.Fatal("no-source mode changed the owner's provider controls or system contract")
			}
			warmCollector := NewCollector(nil)
			warm, err := llm.ExecuteJSON(context.Background(), executor, warmCollector.Wrap(base), call)
			if err != nil || !warm.Cached || base.calls != 1 {
				t.Fatalf("no-source warm reuse changed the request: %+v / %v", warm, err)
			}
			base.response = makeAnswer("replayed")
			prepared, err := llm.NewPrepared(first.Request)
			if err != nil {
				t.Fatal(err)
			}
			replay, err := llm.ReplayJSON(context.Background(), executor, base, prepared)
			if err != nil || replay.CacheKey != first.CacheKey {
				t.Fatalf("base replay refreshed a different no-source entry: %v", err)
			}
			refreshed, err := llm.ExecuteJSON(context.Background(), executor, warmCollector.Wrap(base), call)
			if err != nil || !refreshed.Cached || base.calls != 2 || !bytes.Equal(refreshed.Value, makeAnswer("replayed")) || len(warmCollector.Snapshot()) != 0 {
				t.Fatalf("no-source reuse missed replay or manufactured metadata: %+v / %v", refreshed, err)
			}
		})
	}
}

func TestOptionalMetadataRejectionUsesOrdinaryJournalAndRawCache(t *testing.T) {
	for _, buffered := range []bool{false, true} {
		t.Run(fmt.Sprint("buffered=", buffered), func(t *testing.T) {
			root := t.TempDir()
			writer, err := debugdump.NewWriter(root, "metadata")
			if err != nil {
				t.Fatal(err)
			}
			defer writer.Close()
			var observer *debugdump.SemanticObserver
			if buffered {
				observer = debugdump.NewSemanticObserver(nil)
			} else {
				observer = debugdump.NewSemanticObserver(writer)
			}
			executor := debugdump.BindStage(llm.Executor{Enabled: true, RootDir: root, Observer: observer}, "atlas_files")
			base := &testProvider{response: []byte(`{"result":{"answer":"HTTP works."},"terms":[{"name":42}]}`)}
			collector := NewCollector([]string{"api.py"})
			call := llm.Call[map[string]string]{State: []byte(`{"test":"rejection-journal"}`), Prompt: llm.Prompt{User: `{"path":"api.py"}`, ResponseExample: `{"answer":"<computed>"}`}, Limits: llm.Limits{MaxRequestBytes: 100000, MaxResponseBytes: 100000, MaxOutputTokens: 1000}}
			var last llm.Outcome[map[string]string]
			for i := 0; i < 2; i++ {
				last, err = llm.ExecuteJSON(t.Context(), executor, collector.Wrap(base), call)
				if err != nil || last.Value["answer"] != "HTTP works." || last.Cached != (i == 1) {
					t.Fatalf("good domain rejected: %+v / %v", last, err)
				}
			}
			if buffered {
				observer.Flush(writer)
			}
			if base.calls != 1 || len(collector.Snapshot()) != 0 {
				t.Fatal("bad metadata collected or good domain repeated")
			}
			cached, found, err := llm.CachedExchange(root, last.CacheKey)
			if err != nil || !found || !bytes.Equal(cached.Response, base.response) {
				t.Fatal("raw cache was repaired or good domain was not cached")
			}
			runDir := filepath.Join(root, "metadata")
			rows, err := modeldiag.Read(runDir)
			if err != nil || len(rows) != 2 {
				t.Fatalf("rejected.jsonl missing: %+v / %v", rows, err)
			}
			for i, row := range rows {
				if row.Stage != "atlas_files" || row.Kind != "terminology_metadata_rejected" || row.Count != 1 || row.Reason == "" || row.ResponseRef == "" {
					t.Fatalf("bad rejection row: %+v", row)
				}
				raw, err := os.ReadFile(filepath.Join(runDir, row.ResponseRef))
				if err != nil {
					t.Fatal(err)
				}
				var record struct {
					State             string `json:"state"`
					SemanticCalls     int    `json:"semantic_calls"`
					Request, Response struct {
						OriginalSHA256 string `json:"original_sha256"`
					}
				}
				if err := json.Unmarshal(raw, &record); err != nil {
					t.Fatal(err)
				}
				wantCalls, wantState := 1, "accepted"
				if i == 1 {
					wantCalls, wantState = 0, "cache_hit"
				}
				if record.SemanticCalls != wantCalls || record.State != wantState || record.Request.OriginalSHA256 != fmt.Sprintf("%x", sha256.Sum256(last.Request)) || record.Response.OriginalSHA256 != fmt.Sprintf("%x", sha256.Sum256(base.response)) {
					t.Fatalf("metadata became another live/failing call or lost exact refs: %+v", record)
				}
			}
		})
	}
}
