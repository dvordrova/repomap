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
		if err := runReadConfigured(t.Context(), args, io.Discard, factory, false); err != nil {
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
		refs := refusedResponseRefs(t, run)
		if len(refs) == 0 {
			t.Errorf("%s: rejected.jsonl names no refused response", filepath.Base(run))
		}
		for _, ref := range refs {
			body, err := os.ReadFile(resolveResponseRef(t, run, ref))
			if err != nil || !bytes.Equal(body, provider.answer) {
				t.Errorf("%s: after cache clear %s does not lead to the refused answer: %q, %v", filepath.Base(run), ref, body, err)
			}
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
	kind, from, label, path string
	refused                 bool
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
			links = append(links, exchangeLink{kind: "journal", from: journal, label: label, refused: refused,
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
			links = append(links, exchangeLink{kind: "table", from: ref, label: label, refused: window.Source == atlas.SourceGiven,
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

// refusedResponseRefs lists the response_ref of every rejected.jsonl row of
// the refused table.
func refusedResponseRefs(t *testing.T, run string) []string {
	t.Helper()
	file, err := os.Open(filepath.Join(run, "rejected.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	var refs []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var row struct {
			Stage       string `json:"stage"`
			ResponseRef string `json:"response_ref"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &row); err != nil {
			t.Fatal(err)
		}
		if row.Stage == lines.StageFiles && row.ResponseRef != "" {
			refs = append(refs, row.ResponseRef)
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
