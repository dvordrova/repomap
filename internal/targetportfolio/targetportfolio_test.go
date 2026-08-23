package targetportfolio

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/secretscan"
)

func TestCompileAndResolveFilePortfolio(t *testing.T) {
	snapshot := testSnapshot(t, []string{
		"README.md", "cmd/tool/main.go", "pkg/client/client.go", "scripts/preview.py",
	})
	candidates := []Candidate{
		{FileRef: "f4", Hypotheses: []string{"development preview script"}},
		{FileRef: "f2", Hypotheses: []string{"declared CLI command", "runnable application", "declared CLI command"}},
		{FileRef: "f3", Hypotheses: []string{"downstream-consumed library"}},
	}
	compilation, err := Compile(snapshot, candidates)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	wire, err := ProviderVisibleJSON(compilation)
	if err != nil {
		t.Fatalf("ProviderVisibleJSON: %v", err)
	}
	if len(wire) > MaxRequestBytes || sha256Hex(wire) != compilation.RequestSHA256 {
		t.Fatalf("wire identity = %d/%s", len(wire), compilation.RequestSHA256)
	}
	var request Request
	if err := json.Unmarshal(wire, &request); err != nil {
		t.Fatal(err)
	}
	if len(request.Candidates) != 3 || request.Candidates[0].FileRef != "f2" ||
		request.Candidates[0].Path != "cmd/tool/main.go" ||
		!slices.Equal(request.Candidates[0].Hypotheses, []string{"declared CLI command", "runnable application"}) {
		t.Fatalf("canonical candidates = %#v", request.Candidates)
	}
	for _, forbidden := range []string{
		`"version"`, `"request_ref"`, `"target_ref"`, `"claim"`, `"basis"`,
		`"native_ref"`, `"identity_ref"`, snapshot.Ref, snapshot.SHA256,
	} {
		if bytes.Contains(wire, []byte(forbidden)) {
			t.Fatalf("private/obsolete field %q leaked in %s", forbidden, wire)
		}
	}

	state, err := ExecutionState(compilation)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{
		`"prompt_version"`, `"preparation_version"`, `"response_schema_version"`,
		`"corpus_bytes_sha256"`, `"candidate_bytes_sha256"`, `"request_bytes_sha256"`,
	} {
		if !bytes.Contains(state, []byte(field)) {
			t.Fatalf("execution state lacks %s: %s", field, state)
		}
	}

	prompt, err := BuildPrompt(compilation)
	if err != nil {
		t.Fatal(err)
	}
	if prompt.Version != PromptVersion || !strings.Contains(prompt.User, string(wire)) ||
		!strings.Contains(prompt.System, "Selection is positive") ||
		!strings.Contains(prompt.System, "credible starting file") ||
		!strings.Contains(prompt.System, "do not receive file contents") ||
		!strings.Contains(prompt.System, "Omit every file that is not positively supported") ||
		!strings.Contains(prompt.System, "producer provenance, not evidence") ||
		!strings.Contains(prompt.User, "exactly one JSON object with these two fields") ||
		!strings.Contains(prompt.User, `{"default_file_ref":null,"target_file_refs":[]}`) ||
		!strings.Contains(prompt.User, "End of quoted candidate JSON") ||
		strings.Contains(prompt.User, "request_ref") || strings.Contains(prompt.System, "Go repository") ||
		strings.Contains(strings.ToLower(prompt.System), "surface") ||
		strings.Contains(strings.ToLower(prompt.System), "unlikely") ||
		strings.Contains(strings.ToLower(prompt.User), "unlikely") {
		t.Fatalf("prompt contract = %#v", prompt)
	}

	defaultRef := corpus.FileID("f2")
	raw, err := json.Marshal(Response{
		DefaultFileRef: &defaultRef, TargetFileRefs: []corpus.FileID{"f3", "f2"},
	})
	if err != nil {
		t.Fatal(err)
	}
	selection, err := ResolveResponse(compilation, raw)
	if err != nil {
		t.Fatalf("ResolveResponse: %v", err)
	}
	if selection.Default == nil || selection.Default.FileRef != "f2" ||
		!slices.Equal(candidateRefs(selection.Targets), []corpus.FileID{"f2", "f3"}) ||
		!slices.Equal(candidateRefs(selection.Unclassified), []corpus.FileID{"f4"}) {
		t.Fatalf("selection = %#v", selection)
	}
	selection.Default.Hypotheses[0] = "mutated"
	selection.Targets[0].Hypotheses[0] = "mutated"
	selection.Unclassified[0].Hypotheses[0] = "mutated"
	again, err := ResolveResponse(compilation, raw)
	if err != nil || again.Default == nil || slices.Contains(again.Default.Hypotheses, "mutated") ||
		slices.Contains(again.Targets[0].Hypotheses, "mutated") ||
		slices.Contains(again.Unclassified[0].Hypotheses, "mutated") {
		t.Fatalf("selection mutated compilation authority: %#v / %v", again, err)
	}
}

func TestResolveResponseRequiresExactTwoFieldSchema(t *testing.T) {
	compilation := testCompilation(t)
	valid := func(defaultRef string, targets []string) []byte {
		refs := make([]corpus.FileID, len(targets))
		for index, value := range targets {
			refs[index] = corpus.FileID(value)
		}
		fileRef := corpus.FileID(defaultRef)
		raw, err := json.Marshal(Response{DefaultFileRef: &fileRef, TargetFileRefs: refs})
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	tests := map[string][]byte{
		"malformed":          []byte(`{"default_file_ref":`),
		"null object":        []byte(`null`),
		"empty object":       []byte(`{}`),
		"missing targets":    []byte(`{"default_file_ref":"f1"}`),
		"null targets":       []byte(`{"default_file_ref":"f1","target_file_refs":null}`),
		"empty targets":      []byte(`{"default_file_ref":"f1","target_file_refs":[]}`),
		"missing default":    []byte(`{"target_file_refs":[]}`),
		"old version":        []byte(`{"version":1,"default_file_ref":"f1","target_file_refs":["f1"]}`),
		"old request ref":    []byte(`{"request_ref":"q1","default_file_ref":"f1","target_file_refs":["f1"]}`),
		"old target ref":     []byte(`{"default_file_ref":"f1","target_file_refs":["f1"],"target_ref":"t1"}`),
		"old unlikely field": []byte(`{"default_file_ref":"f1","target_file_refs":["f1"],"unlikely_file_refs":[]}`),
		"extra field":        []byte(`{"default_file_ref":"f1","target_file_refs":["f1"],"reason":"no"}`),
		"trailing value":     append(valid("f1", []string{"f1"}), []byte(` {}`)...),
		"unknown default":    valid("f99", []string{"f1"}),
		"unknown target":     valid("f1", []string{"f1", "f99"}),
		"duplicate target":   valid("f1", []string{"f1", "f2", "f2"}),
		"default omitted":    valid("f1", []string{"f2"}),
	}
	for name, raw := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := ResolveResponse(compilation, raw); err == nil {
				t.Fatalf("accepted invalid response: %s", raw)
			}
		})
	}
	selection, err := ResolveResponse(compilation, valid("f2", []string{"f3", "f2"}))
	if err != nil {
		t.Fatalf("valid positive response: %v", err)
	}
	if selection.Default == nil || selection.Default.FileRef != "f2" ||
		!slices.Equal(candidateRefs(selection.Targets), []corpus.FileID{"f2", "f3"}) ||
		!slices.Equal(candidateRefs(selection.Unclassified), []corpus.FileID{"f1"}) {
		t.Fatalf("canonical positive selection = %#v", selection)
	}
	emptySelection, err := ResolveResponse(compilation, []byte(`{"default_file_ref":null,"target_file_refs":[]}`))
	if err != nil || emptySelection.Default != nil || len(emptySelection.Targets) != 0 ||
		!slices.Equal(candidateRefs(emptySelection.Unclassified), []corpus.FileID{"f1", "f2", "f3"}) {
		t.Fatalf("valid empty positive selection = %#v / %v", emptySelection, err)
	}
	oversized := bytes.Repeat([]byte{'x'}, MaxResponseBytes+1)
	if _, err := ResolveResponse(compilation, oversized); err == nil {
		t.Fatal("accepted oversized response")
	}
}

func TestCompileRequiresAlreadyMergedExactCorpusCandidates(t *testing.T) {
	snapshot := testSnapshot(t, []string{"a.py", "b.py"})
	tests := map[string][]Candidate{
		"empty":          {},
		"unknown file":   {{FileRef: "f3", Hypotheses: []string{"candidate"}}},
		"duplicate file": {{FileRef: "f1", Hypotheses: []string{"one"}}, {FileRef: "f1", Hypotheses: []string{"two"}}},
		"no hypotheses":  {{FileRef: "f1", Hypotheses: nil}},
		"blank":          {{FileRef: "f1", Hypotheses: []string{" "}}},
		"control":        {{FileRef: "f1", Hypotheses: []string{"bad\nvalue"}}},
		"host absolute":  {{FileRef: "f1", Hypotheses: []string{"/Users/me/project"}}},
	}
	for name, candidates := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := Compile(snapshot, candidates); err == nil {
				t.Fatalf("Compile accepted %#v", candidates)
			}
		})
	}
}

func TestCompileIsStableAcrossCandidateAndHypothesisPermutation(t *testing.T) {
	snapshot := testSnapshot(t, []string{"a.py", "b.py", "c.py"})
	left, err := Compile(snapshot, []Candidate{
		{FileRef: "f3", Hypotheses: []string{"script", "worker"}},
		{FileRef: "f1", Hypotheses: []string{"application", "application"}},
		{FileRef: "f2", Hypotheses: []string{"library"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	right, err := Compile(snapshot, []Candidate{
		{FileRef: "f2", Hypotheses: []string{"library"}},
		{FileRef: "f3", Hypotheses: []string{"worker", "script", "worker"}},
		{FileRef: "f1", Hypotheses: []string{"application"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	leftWire, _ := ProviderVisibleJSON(left)
	rightWire, _ := ProviderVisibleJSON(right)
	leftState, _ := ExecutionState(left)
	rightState, _ := ExecutionState(right)
	if !bytes.Equal(leftWire, rightWire) || !bytes.Equal(leftState, rightState) ||
		left.RequestSHA256 != right.RequestSHA256 {
		t.Fatalf("permutation changed compilation:\n%s\n%s", leftWire, rightWire)
	}
}

func TestCompleteCandidateSurfaceFailsInsteadOfTruncating(t *testing.T) {
	const count = 3000
	paths := make([]string, count)
	candidates := make([]Candidate, count)
	for index := 0; index < count; index++ {
		paths[index] = fmt.Sprintf("services/service-%04d/main.py", index)
	}
	snapshot := testSnapshot(t, paths)
	for index, entry := range snapshot.Entries {
		candidates[index] = Candidate{
			FileRef:    entry.ID,
			Hypotheses: []string{fmt.Sprintf("independently declared service candidate %04d with a deliberately complete label", index)},
		}
	}
	if _, err := Compile(snapshot, candidates); err == nil || !strings.Contains(err.Error(), "complete candidate request") {
		t.Fatalf("oversized complete surface error = %v", err)
	}
}

func TestProviderBoundaryRejectsVisibleSecretsAndCompilationTampering(t *testing.T) {
	restore := secretscan.SetEnabled(true)
	defer restore()
	snapshot := testSnapshot(t, []string{"main.py"})
	secret := "sk-ABCDEFGHIJKLMNOPQRSTUVWX"
	if _, err := Compile(snapshot, []Candidate{{FileRef: "f1", Hypotheses: []string{secret}}}); err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("visible secret error = %v", err)
	}

	compilation, err := Compile(snapshot, []Candidate{{FileRef: "f1", Hypotheses: []string{"application"}}})
	if err != nil {
		t.Fatal(err)
	}
	compilation.Request.Candidates[0].Path = "invented.py"
	if _, err := ProviderVisibleJSON(compilation); err == nil {
		t.Fatal("provider boundary accepted request tampering")
	}
	compilation, err = Compile(snapshot, []Candidate{{FileRef: "f1", Hypotheses: []string{"application"}}})
	if err != nil {
		t.Fatal(err)
	}
	compilation.candidates[0].Hypotheses[0] = "invented"
	if _, err := ExecutionState(compilation); err == nil {
		t.Fatal("execution state accepted private candidate tampering")
	}
}

func testCompilation(t *testing.T) Compilation {
	t.Helper()
	snapshot := testSnapshot(t, []string{"a.py", "b.py", "c.py"})
	compilation, err := Compile(snapshot, []Candidate{
		{FileRef: "f1", Hypotheses: []string{"application"}},
		{FileRef: "f2", Hypotheses: []string{"worker"}},
		{FileRef: "f3", Hypotheses: []string{"library"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return compilation
}

func testSnapshot(t *testing.T, paths []string) corpus.Snapshot {
	t.Helper()
	canonical := append([]string(nil), paths...)
	sort.Strings(canonical)
	entries := make([]corpus.Entry, len(canonical))
	for index, path := range canonical {
		entries[index] = corpus.Entry{ID: corpus.FileID(fmt.Sprintf("f%d", index+1)), Path: path}
	}
	identity := struct {
		Version int            `json:"version"`
		Entries []corpus.Entry `json:"entries"`
	}{Version: corpus.Version, Entries: entries}
	wire, err := json.Marshal(identity)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(wire)
	sha := hex.EncodeToString(digest[:])
	snapshot := corpus.Snapshot{
		Version: corpus.Version, Ref: "rc-" + sha[:24], SHA256: sha, Entries: entries,
	}
	if err := snapshot.Validate(); err != nil {
		t.Fatalf("test snapshot: %v", err)
	}
	return snapshot
}

func candidateRefs(values []VisibleCandidate) []corpus.FileID {
	result := make([]corpus.FileID, len(values))
	for index, value := range values {
		result[index] = value.FileRef
	}
	return result
}
