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
		`"corpus_bytes_sha256"`, `"candidate_bytes_sha256"`,
		`"executable_authority_bound"`, `"executable_file_refs_sha256"`,
		`"required_target_authority_bound"`, `"required_target_file_refs_sha256"`,
		`"request_bytes_sha256"`,
	} {
		if !bytes.Contains(state, []byte(field)) {
			t.Fatalf("execution state lacks %s: %s", field, state)
		}
	}

	prompt, err := BuildPrompt(compilation)
	if err != nil {
		t.Fatal(err)
	}
	if prompt.Version != PromptVersion || !strings.Contains(prompt.User, string(wire)) || strings.Contains(prompt.User, snapshot.SHA256) {
		t.Fatal("prompt is not bound to the exact public request")
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

func TestRequiredTargetAuthorityCannotBeSuppressedByPortfolio(t *testing.T) {
	snapshot := testSnapshot(t, []string{"backend/main.py", "front/package.json", "README.md"})
	compilation, err := CompileWithRequiredTargetAuthority(snapshot, []Candidate{
		{FileRef: "f3", Hypotheses: []string{"repository guidance candidate"}},
		{FileRef: "f2", Hypotheses: []string{"exact JavaScript application project"}},
		{FileRef: "f1", Hypotheses: []string{"exact Python executable target"}},
	}, []corpus.FileID{"f2", "f1", "f2"})
	if err != nil {
		t.Fatal(err)
	}
	if compilation.Request.RequiredTargetFileRefs == nil ||
		!slices.Equal(*compilation.Request.RequiredTargetFileRefs, []corpus.FileID{"f1", "f2"}) {
		t.Fatalf("required target authority = %#v", compilation.Request.RequiredTargetFileRefs)
	}
	wire, err := ProviderVisibleJSON(compilation)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(wire, []byte(`"required_target_file_refs":["f1","f2"]`)) {
		t.Fatalf("required target authority is absent from provider request: %s", wire)
	}
	prompt, err := BuildPrompt(compilation)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt.User, string(wire)) {
		t.Fatal("prompt lost exact authority")
	}

	// An answer that omits a required representative cannot suppress it: the
	// compilation restores it. The guidance candidate the model did not select
	// stays unclassified and is never added.
	for name, test := range map[string]struct {
		raw         []byte
		wantDefault corpus.FileID
		rejected    int
	}{
		"empty":       {raw: []byte(`{"default_file_ref":null,"target_file_refs":[]}`)},
		"absent":      {raw: []byte(`{}`)},
		"one missing": {raw: mustResponse(t, fileIDPointer("f1"), []corpus.FileID{"f1"}), wantDefault: "f1"},
		"unknowns":    {raw: mustResponse(t, fileIDPointer("f1"), []corpus.FileID{"f1", "foreign"}), wantDefault: "f1", rejected: 1},
	} {
		t.Run(name, func(t *testing.T) {
			result, err := resolveResponse(compilation, test.raw)
			if err != nil {
				t.Fatalf("omitting answer refused: %v", err)
			}
			if !slices.Equal(candidateRefs(result.Targets), []corpus.FileID{"f1", "f2"}) ||
				!slices.Equal(candidateRefs(result.Unclassified), []corpus.FileID{"f3"}) {
				t.Fatalf("required authority not restored or guidance promoted: %#v", result.Selection)
			}
			if (test.wantDefault == "") != (result.Default == nil) ||
				(result.Default != nil && result.Default.FileRef != test.wantDefault) {
				t.Fatalf("default = %#v, want %q", result.Default, test.wantDefault)
			}
			if len(result.ResponseRejections()) != test.rejected {
				t.Fatalf("rejections = %+v, want %d", result.ResponseRejections(), test.rejected)
			}
		})
	}
	for _, raw := range []string{`null`, `[]`, `"f1"`, `{"target_file_refs":["f1"]`} {
		if _, err := ResolveResponse(compilation, []byte(raw)); err == nil {
			t.Fatalf("accepted an answer that is not one JSON object: %s", raw)
		}
	}
	selection, err := ResolveResponse(
		compilation,
		mustResponse(t, fileIDPointer("f2"), []corpus.FileID{"f1", "f2", "f3"}),
	)
	if err != nil || selection.Default == nil || selection.Default.FileRef != "f2" ||
		!slices.Equal(candidateRefs(selection.Targets), []corpus.FileID{"f1", "f2", "f3"}) {
		t.Fatalf("complete required selection = %#v / %v", selection, err)
	}
}

func TestRequiredTargetAuthorityIsRequestBoundAndTamperEvident(t *testing.T) {
	snapshot := testSnapshot(t, []string{"app.py", "package.json", "README.md"})
	candidates := []Candidate{
		{FileRef: "f1", Hypotheses: []string{"exact Python target"}},
		{FileRef: "f2", Hypotheses: []string{"exact JavaScript target"}},
	}
	if _, err := CompileWithRequiredTargetAuthority(snapshot, candidates, []corpus.FileID{"f3"}); err == nil {
		t.Fatal("accepted corpus-current ref outside the current candidate authority")
	}
	if _, err := CompileWithRequiredTargetAuthority(snapshot, candidates, []corpus.FileID{"stale"}); err == nil {
		t.Fatal("accepted stale required target authority ref")
	}

	left, err := CompileWithRequiredTargetAuthority(snapshot, candidates, []corpus.FileID{"f1"})
	if err != nil {
		t.Fatal(err)
	}
	right, err := CompileWithRequiredTargetAuthority(snapshot, candidates, []corpus.FileID{"f2"})
	if err != nil {
		t.Fatal(err)
	}
	leftWire, _ := ProviderVisibleJSON(left)
	rightWire, _ := ProviderVisibleJSON(right)
	leftState, _ := ExecutionState(left)
	rightState, _ := ExecutionState(right)
	if bytes.Equal(leftWire, rightWire) || bytes.Equal(leftState, rightState) ||
		left.RequestSHA256 == right.RequestSHA256 {
		t.Fatalf("material required-authority change reused identity:\n%s\n%s", leftWire, rightWire)
	}

	(*left.Request.RequiredTargetFileRefs)[0] = "f2"
	if _, err := ProviderVisibleJSON(left); err == nil {
		t.Fatal("accepted visible required target authority tampering")
	}
	left, err = CompileWithRequiredTargetAuthority(snapshot, candidates, []corpus.FileID{"f1"})
	if err != nil {
		t.Fatal(err)
	}
	left.requiredTargetFileRefs[0] = "f2"
	if _, err := ExecutionState(left); err == nil {
		t.Fatal("accepted private required target authority tampering")
	}

	boundEmpty, err := CompileWithRequiredTargetAuthority(snapshot, candidates, nil)
	if err != nil {
		t.Fatal(err)
	}
	generic, err := Compile(snapshot, candidates)
	if err != nil {
		t.Fatal(err)
	}
	boundWire, _ := ProviderVisibleJSON(boundEmpty)
	genericWire, _ := ProviderVisibleJSON(generic)
	boundState, _ := ExecutionState(boundEmpty)
	genericState, _ := ExecutionState(generic)
	if boundEmpty.Request.RequiredTargetFileRefs == nil ||
		len(*boundEmpty.Request.RequiredTargetFileRefs) != 0 ||
		!bytes.Contains(boundWire, []byte(`"required_target_file_refs":[]`)) ||
		generic.Request.RequiredTargetFileRefs != nil ||
		bytes.Contains(genericWire, []byte("required_target_file_refs")) ||
		bytes.Equal(boundState, genericState) || boundEmpty.RequestSHA256 == generic.RequestSHA256 {
		t.Fatalf("bound empty required authority collapsed into generic compilation:\nbound=%s\ngeneric=%s", boundWire, genericWire)
	}
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

func mustResponse(t *testing.T, defaultRef *corpus.FileID, targets []corpus.FileID) []byte {
	t.Helper()
	raw, err := json.Marshal(Response{DefaultFileRef: defaultRef, TargetFileRefs: targets})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func fileIDPointer(value corpus.FileID) *corpus.FileID {
	return &value
}

func candidateRefs(values []VisibleCandidate) []corpus.FileID {
	result := make([]corpus.FileID, len(values))
	for index, value := range values {
		result[index] = value.FileRef
	}
	return result
}
