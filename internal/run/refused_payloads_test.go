package run

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/llm"
)

// refusingTableProvider answers one atlas table with prose its decoder
// refuses and every other table through the ordinary test provider.
type refusingTableProvider struct {
	*terminologyRuntimeProvider
	table  string
	answer []byte

	mu       sync.Mutex
	refused  [][]byte
	accepted int
}

func (p *refusingTableProvider) Complete(ctx context.Context, prepared llm.Prepared) (llm.Completion, error) {
	var message map[string]string
	var input struct {
		Table string `json:"table"`
	}
	if json.Unmarshal(prepared.Bytes(), &message) == nil && json.Unmarshal([]byte(message["user"]), &input) == nil && input.Table == p.table {
		p.mu.Lock()
		p.refused = append(p.refused, prepared.Bytes())
		p.mu.Unlock()
		return llm.Completion{Response: append([]byte(nil), p.answer...), ChoiceCount: 1, FinishReason: llm.FinishStop, Metrics: llm.Metrics{Attempts: 1}}, nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.accepted++
	return p.terminologyRuntimeProvider.Complete(ctx, prepared)
}

// A refused answer is the developer's evidence, not the user's cache
// (owner, 2026-09-26). Its exact bytes stay in the run that asked for them,
// where cache clear leaves them; the cache holds only what accepted answers
// need; and the next run is answered from the cache except for the refused
// window, which it asks again.
func TestRefusedAnswerStaysInItsRunAndOutOfTheCache(t *testing.T) {
	input := readCommandInputFixture(t)
	debugDir := t.TempDir()
	provider := &refusingTableProvider{
		terminologyRuntimeProvider: &terminologyRuntimeProvider{},
		table:                      lines.StageFiles,
		answer:                     []byte("The file row looks fine to me; no JSON today."),
	}
	factory := func() (llm.Provider, error) { return provider, nil }
	read := func(name string) string {
		t.Helper()
		run := filepath.Join(debugDir, name)
		args := []string{input, "--through", "files", "--output", run, "--debug-dir", debugDir}
		if err := runReadConfigured(t.Context(), args, io.Discard, factory, noClosedQuestions, false); err != nil {
			t.Fatalf("%s reading: %v", name, err)
		}
		return run
	}

	cold := read("cold")
	if len(provider.refused) != 1 || provider.accepted == 0 {
		t.Fatalf("fixture must refuse one window and accept others: refused=%d accepted=%d", len(provider.refused), provider.accepted)
	}
	refusedRequest := provider.refused[0]
	cache := filepath.Join(debugDir, llm.CacheDirectoryName)
	owned := acceptedRecordPayloads(t, cache)
	if len(owned) == 0 {
		t.Fatal("no accepted answer reached the cache")
	}
	// refusedPayloads reads every payload a run links for its refused
	// exchange: the journal's request and response, and the window's prompt,
	// input, request and response. All of them are the run's own.
	refusedPayloads := func(run string) map[string][]byte {
		t.Helper()
		bodies := make(map[string][]byte)
		kinds := make(map[string]bool)
		for _, link := range exchangeLinks(t, run) {
			if !link.refused {
				continue
			}
			kinds[link.kind+" "+link.label] = true
			if !strings.HasPrefix(link.path, run+string(filepath.Separator)) {
				t.Errorf("%s links the refused %s outside its run: %s", link.from, link.label, link.path)
			}
			body, err := os.ReadFile(link.path)
			if err != nil {
				t.Fatalf("%s: refused %s is unreadable: %v", link.from, link.label, err)
			}
			if want := map[string][]byte{"request": refusedRequest, "response": provider.answer}[link.label]; want != nil && !bytes.Equal(body, want) {
				t.Errorf("%s links a refused %s other than the exchanged bytes", link.from, link.label)
			}
			bodies[link.from+" "+link.label] = body
		}
		if len(kinds) != 6 {
			t.Fatalf("%s: the refused exchange must be linked by its journal (request, response) and its table (prompt, input, request, response): %v", filepath.Base(run), kinds)
		}
		return bodies
	}
	coldRefused := refusedPayloads(cold)
	// The store holds only what accepted answers need: each payload in it is
	// an accepted record's, or linked by an accepted exchange of the run.
	// Nothing in it belongs only to the refused exchange.
	acceptedLinks := make(map[string]bool)
	for _, link := range exchangeLinks(t, cold) {
		if !link.refused {
			acceptedLinks[link.path] = true
		}
	}
	stored, err := os.ReadDir(filepath.Join(cache, "payloads"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range stored {
		if name := entry.Name(); !owned[name] && !acceptedLinks[filepath.Join(cache, "payloads", name)] {
			t.Errorf("cache payload store holds %s, which no accepted answer owns", name)
		}
		if name := entry.Name(); strings.HasPrefix(name, contentName(refusedRequest)) || strings.HasPrefix(name, contentName(provider.answer)) {
			t.Errorf("cache payload store holds %s, which only the refused answer owns", name)
		}
	}

	accepted := provider.accepted
	warm := read("warm")
	if provider.accepted != accepted || len(provider.refused) != 2 || !bytes.Equal(provider.refused[1], refusedRequest) {
		t.Errorf("warm run: accepted calls %d -> %d, refused calls %d; want every accepted answer from the cache and the refused window asked again",
			accepted, provider.accepted, len(provider.refused))
	}
	warmRefused := refusedPayloads(warm)

	if err := clearPersistentCaches(debugDir, io.Discard); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(cache); !os.IsNotExist(err) {
		t.Fatalf("cache clear left the cache: %v", err)
	}
	for run, before := range map[string]map[string][]byte{cold: coldRefused, warm: warmRefused} {
		refused := 0
		for _, row := range rejectedResponseRefs(t, run) {
			body, err := os.ReadFile(resolveResponseRef(t, run, row.ref))
			if err != nil {
				t.Errorf("%s: after cache clear %s row's %s leads nowhere: %v", filepath.Base(run), row.stage, row.ref, err)
				continue
			}
			if row.stage != lines.StageFiles {
				continue
			}
			refused++
			if !bytes.Equal(body, provider.answer) {
				t.Errorf("%s: after cache clear %s does not lead to the refused answer: %q", filepath.Base(run), row.ref, body)
			}
		}
		if refused == 0 {
			t.Errorf("%s: rejected.jsonl names no refused response", filepath.Base(run))
		}
		for _, link := range exchangeLinks(t, run) {
			if !link.refused {
				continue
			}
			body, err := os.ReadFile(link.path)
			if err != nil || !bytes.Equal(body, before[link.from+" "+link.label]) {
				t.Errorf("%s: after cache clear %s lost the refused %s: %v", filepath.Base(run), link.from, link.label, err)
			}
		}
	}
}

// partlyRefusingTableProvider answers one atlas table like the ordinary test
// provider plus a row nobody asked for: the decoder refuses that row and
// accepts the rest of the answer.
type partlyRefusingTableProvider struct {
	*terminologyRuntimeProvider
	table string

	mu       sync.Mutex
	requests [][]byte
	answer   []byte
	calls    int
}

func (p *partlyRefusingTableProvider) Complete(ctx context.Context, prepared llm.Prepared) (llm.Completion, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	completion, err := p.terminologyRuntimeProvider.Complete(ctx, prepared)
	var message map[string]string
	var input struct {
		Table string `json:"table"`
	}
	if err != nil || json.Unmarshal(prepared.Bytes(), &message) != nil || json.Unmarshal([]byte(message["user"]), &input) != nil || input.Table != p.table {
		return completion, err
	}
	var answer struct {
		Rows []map[string]string `json:"rows"`
	}
	if err := json.Unmarshal(completion.Response, &answer); err != nil {
		return completion, err
	}
	answer.Rows = append(answer.Rows, map[string]string{"key": "p999"})
	if completion.Response, err = json.Marshal(answer); err != nil {
		return completion, err
	}
	p.requests = append(p.requests, prepared.Bytes())
	p.answer = completion.Response
	return completion, nil
}

// An answer accepted with a refused row keeps both lives (owner,
// 2026-09-26): its accepted record stays in the cache and answers the next
// run, and each run that reads it keeps its own copy of what its rejected
// rows point at. After cache clear every rejected row's response_ref still
// leads to the answer, while a wholly accepted answer made no copy.
func TestPartlyRefusedAnswerStaysInItsRunAndInTheCache(t *testing.T) {
	input := readCommandInputFixture(t)
	debugDir := t.TempDir()
	provider := &partlyRefusingTableProvider{terminologyRuntimeProvider: &terminologyRuntimeProvider{}, table: lines.StageFiles}
	factory := func() (llm.Provider, error) { return provider, nil }
	read := func(name string) string {
		t.Helper()
		run := filepath.Join(debugDir, name)
		args := []string{input, "--through", "files", "--output", run, "--debug-dir", debugDir}
		if err := runReadConfigured(t.Context(), args, io.Discard, factory, noClosedQuestions, false); err != nil {
			t.Fatalf("%s reading: %v", name, err)
		}
		return run
	}

	cold := read("cold")
	if len(provider.requests) != 1 {
		t.Fatalf("fixture must answer the %s table once with a refused row: %d", lines.StageFiles, len(provider.requests))
	}
	request, answer := provider.requests[0], provider.answer
	cache := filepath.Join(debugDir, llm.CacheDirectoryName)
	// The accepted record is the cache's as before: it owns its request and
	// response in the shared store.
	owned := acceptedRecordPayloads(t, cache)
	for _, raw := range [][]byte{request, answer} {
		name := contentName(raw) + "json"
		if _, err := os.Stat(filepath.Join(cache, "payloads", name)); !owned[name] || err != nil {
			t.Fatalf("the partly refused answer's accepted record does not own %s in the store: %v", name, err)
		}
	}
	calls := provider.calls
	warm := read("warm")
	if provider.calls != calls {
		t.Fatalf("warm run asked the provider %d more times; want every answer, the partly refused one too, from the cache", provider.calls-calls)
	}
	// payloads reads every payload a run links for the partly refused
	// exchange, all the run's own. Every other exchange was accepted whole
	// and links only the shared store.
	payloads := func(run string) map[string][]byte {
		t.Helper()
		bodies := make(map[string][]byte)
		for _, link := range exchangeLinks(t, run) {
			inRun := strings.HasPrefix(link.path, run+string(filepath.Separator))
			if link.stage != lines.StageFiles {
				if inRun || link.refused {
					t.Errorf("%s links a wholly accepted %s in its run: %s", link.from, link.label, link.path)
				}
				continue
			}
			if !inRun {
				t.Errorf("%s links the partly refused %s outside its run: %s", link.from, link.label, link.path)
			}
			body, err := os.ReadFile(link.path)
			if err != nil {
				t.Fatalf("%s: %s is unreadable: %v", link.from, link.label, err)
			}
			if want := map[string][]byte{"request": request, "response": answer}[link.label]; want != nil && !bytes.Equal(body, want) {
				t.Errorf("%s links a %s other than the exchanged bytes", link.from, link.label)
			}
			bodies[link.from+" "+link.label] = body
		}
		return bodies
	}
	coldBodies := payloads(cold)
	// The cold run exchanged it: its journal links the request and response,
	// its table the prompt, input, request and response. The warm run recalls
	// the accepted rows from the same record and exchanges nothing.
	if len(coldBodies) != 6 {
		t.Fatalf("the partly refused exchange must be linked by its journal (request, response) and its table (prompt, input, request, response): %d links", len(coldBodies))
	}
	// Without its row memos a run asks the same window again, and the
	// accepted record answers that exact request: the hit is unchanged, the
	// same rows are refused again, and this run keeps its own copy too.
	memos, err := filepath.Glob(filepath.Join(cache, "memo-*.json"))
	if err != nil || len(memos) == 0 {
		t.Fatalf("no row memos to forget: %v", err)
	}
	for _, memo := range memos {
		if err := os.Remove(memo); err != nil {
			t.Fatal(err)
		}
	}
	exact := read("exact")
	if provider.calls != calls {
		t.Fatalf("exact run asked the provider %d more times; want the accepted record's hit", provider.calls-calls)
	}
	exactBodies := payloads(exact)
	if len(exactBodies) != 6 {
		t.Fatalf("the exact hit must be linked by its journal (request, response) and its table (prompt, input, request, response): %d links", len(exactBodies))
	}
	hit := false
	for _, link := range exchangeLinks(t, exact) {
		if link.kind != "journal" || link.stage != lines.StageFiles {
			continue
		}
		raw, err := os.ReadFile(link.from)
		if err != nil {
			t.Fatal(err)
		}
		var record debugdump.SemanticExchangeRecord
		if err := json.Unmarshal(raw, &record); err != nil {
			t.Fatal(err)
		}
		hit = hit || record.State == debugdump.SemanticStateCacheHit
	}
	if !hit {
		t.Fatal("the exact run did not journal a cache hit of the accepted record")
	}
	before := map[string]map[string][]byte{cold: coldBodies, warm: payloads(warm), exact: exactBodies}
	// Those are each run's only copies: a wholly accepted answer made none.
	for run := range before {
		copies, err := os.ReadDir(filepath.Join(run, llm.RunPayloadDirectoryName))
		if err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		linked := make(map[string]bool)
		for _, link := range exchangeLinks(t, run) {
			if link.stage == lines.StageFiles {
				linked[filepath.Base(link.path)] = true
			}
		}
		for _, entry := range copies {
			if !linked[entry.Name()] {
				t.Errorf("%s copied %s, which no partly refused exchange links", filepath.Base(run), entry.Name())
			}
		}
	}

	if err := clearPersistentCaches(debugDir, io.Discard); err != nil {
		t.Fatal(err)
	}
	for run, bodies := range before {
		pointers := make(map[string]bool)
		for _, row := range rejectedResponseRefs(t, run) {
			body, err := os.ReadFile(resolveResponseRef(t, run, row.ref))
			if err != nil {
				t.Errorf("%s: after cache clear %s row's %s leads nowhere: %v", filepath.Base(run), row.stage, row.ref, err)
				continue
			}
			if row.stage == lines.StageFiles {
				pointers[strings.SplitN(row.ref, "/", 2)[0]] = true
				if !bytes.Equal(body, answer) {
					t.Errorf("%s: after cache clear %s does not lead to the partly refused answer: %q", filepath.Base(run), row.ref, body)
				}
			}
		}
		// The journal's row names the refused row, once, in each run that
		// exchanged it, live or as an exact hit. The table keeps its window
		// refs, but its reader journals no second row of the same refusal
		// (until 2026-09-29 it did, under the window's response ref).
		if run != warm && (!pointers[debugdump.SemanticExchangesDir] || pointers[atlas.TablesDir]) {
			t.Errorf("%s: the refused row must be named once, from the journal: %v", filepath.Base(run), pointers)
		}
		for _, link := range exchangeLinks(t, run) {
			if link.stage != lines.StageFiles {
				continue
			}
			if body, err := os.ReadFile(link.path); err != nil || !bytes.Equal(body, bodies[link.from+" "+link.label]) {
				t.Errorf("%s: after cache clear %s lost the partly refused %s: %v", filepath.Base(run), link.from, link.label, err)
			}
		}
	}
}

func contentName(raw []byte) string {
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]) + "."
}

// acceptedRecordPayloads names the payload files accepted cache records own.
func acceptedRecordPayloads(t *testing.T, cache string) map[string]bool {
	t.Helper()
	records, err := filepath.Glob(filepath.Join(cache, "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	owned := make(map[string]bool)
	for _, name := range records {
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		var record struct {
			Accepted     bool   `json:"accepted"`
			RequestFile  string `json:"request_file"`
			ResponseFile string `json:"response_file"`
		}
		if json.Unmarshal(raw, &record) == nil && record.Accepted {
			owned[record.RequestFile], owned[record.ResponseFile] = true, true
		}
	}
	return owned
}

type exchangeLink struct {
	kind, from, stage, label, path string
	refused                        bool
}

// exchangeLinks resolves every payload a run links: the semantic journal's
// request and response files and the tables' prompt, input, request and
// response refs.
func exchangeLinks(t *testing.T, run string) []exchangeLink {
	t.Helper()
	var links []exchangeLink
	journals, err := filepath.Glob(filepath.Join(run, debugdump.SemanticExchangesDir, "*", debugdump.SemanticExchangeMetaFile))
	if err != nil {
		t.Fatal(err)
	}
	for _, journal := range journals {
		raw, err := os.ReadFile(journal)
		if err != nil {
			t.Fatal(err)
		}
		var record debugdump.SemanticExchangeRecord
		if err := json.Unmarshal(raw, &record); err != nil {
			t.Fatal(err)
		}
		refused := record.State != debugdump.SemanticStateAccepted && record.State != debugdump.SemanticStateCacheHit
		for label, payload := range map[string]debugdump.SemanticPayloadRecord{"request": record.Request, "response": record.Response} {
			links = append(links, exchangeLink{kind: "journal", from: journal, stage: record.Stage, label: label, refused: refused,
				path: filepath.Clean(filepath.Join(filepath.Dir(journal), filepath.FromSlash(payload.File)))})
		}
	}
	for _, label := range []string{"prompt", "input", "request", "response"} {
		refs, err := filepath.Glob(filepath.Join(run, "tables", "*."+label+".ref.json"))
		if err != nil {
			t.Fatal(err)
		}
		for _, ref := range refs {
			result := strings.TrimSuffix(ref, label+".ref.json") + "result.json"
			raw, err := os.ReadFile(result)
			if err != nil {
				t.Fatal(err)
			}
			var window struct {
				Source string `json:"source"`
			}
			if err := json.Unmarshal(raw, &window); err != nil {
				t.Fatal(err)
			}
			stage, _, _ := strings.Cut(filepath.Base(ref), "-r")
			links = append(links, exchangeLink{kind: "table", from: ref, stage: stage, label: label, refused: window.Source == atlas.SourceGiven,
				path: resolveTableRef(t, ref)})
		}
	}
	return links
}

func resolveTableRef(t *testing.T, ref string) string {
	t.Helper()
	raw, err := os.ReadFile(ref)
	if err != nil {
		t.Fatal(err)
	}
	var link struct {
		File string `json:"file"`
	}
	if err := json.Unmarshal(raw, &link); err != nil || link.File == "" {
		t.Fatalf("%s: %s / %v", ref, raw, err)
	}
	return filepath.Clean(filepath.Join(filepath.Dir(ref), filepath.FromSlash(link.File)))
}

type rejectedRef struct{ stage, ref string }

// rejectedResponseRefs lists the stage and response_ref of every
// rejected.jsonl row that names a response; a run that rejected nothing has
// no file.
func rejectedResponseRefs(t *testing.T, run string) []rejectedRef {
	t.Helper()
	file, err := os.Open(filepath.Join(run, "rejected.jsonl"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	var refs []rejectedRef
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var row struct {
			Stage       string `json:"stage"`
			ResponseRef string `json:"response_ref"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &row); err != nil {
			t.Fatal(err)
		}
		if row.ResponseRef != "" {
			refs = append(refs, rejectedRef{row.Stage, row.ResponseRef})
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return refs
}

// resolveResponseRef follows a rejected.jsonl response_ref, a journal record
// or a table ref, to the file that holds the response bytes.
func resolveResponseRef(t *testing.T, run, ref string) string {
	t.Helper()
	path := filepath.Join(run, filepath.FromSlash(ref))
	if strings.HasSuffix(ref, ".ref.json") {
		return resolveTableRef(t, path)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var record debugdump.SemanticExchangeRecord
	if err := json.Unmarshal(raw, &record); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(filepath.Dir(path), filepath.FromSlash(record.Response.File))
}
