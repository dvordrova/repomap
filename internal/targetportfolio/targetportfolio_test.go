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
	if prompt.Version != PromptVersion || !strings.Contains(prompt.User, string(wire)) ||
		!strings.Contains(prompt.System, "Selection is positive") ||
		!strings.Contains(prompt.System, "credible starting file") ||
		!strings.Contains(prompt.System, "do not receive file contents") ||
		!strings.Contains(prompt.System, "Omit every file that is neither exact required target authority nor positively supported") ||
		!strings.Contains(prompt.System, "producer provenance, not evidence") ||
		!strings.Contains(prompt.User, "exactly one JSON object with these two fields") ||
		!strings.Contains(prompt.User, `{"default_file_ref":null,"target_file_refs":[]}`) ||
		!strings.Contains(prompt.User, "set-valued selection") ||
		!strings.Contains(prompt.User, "End of quoted classification-batch JSON") ||
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

func TestCompileWithExecutableAuthorityCanonicalizesAndBindsExactRefs(t *testing.T) {
	snapshot := testSnapshot(t, []string{"cmd/main.go", "pkg/client.go", "worker/main.go"})
	left, err := CompileWithExecutableAuthority(snapshot, []Candidate{
		{FileRef: "f3", Hypotheses: []string{"worker"}},
		{FileRef: "f2", Hypotheses: []string{"primary executable according to confident prose"}},
		{FileRef: "f1", Hypotheses: []string{"library according to misleading prose"}},
	}, []corpus.FileID{"f3", "f1", "f3", "f1"})
	if err != nil {
		t.Fatal(err)
	}
	right, err := CompileWithExecutableAuthority(snapshot, []Candidate{
		{FileRef: "f1", Hypotheses: []string{"library according to misleading prose"}},
		{FileRef: "f2", Hypotheses: []string{"primary executable according to confident prose"}},
		{FileRef: "f3", Hypotheses: []string{"worker"}},
	}, []corpus.FileID{"f1", "f3"})
	if err != nil {
		t.Fatal(err)
	}
	if left.Request.ExecutableFileRefs == nil ||
		!slices.Equal(*left.Request.ExecutableFileRefs, []corpus.FileID{"f1", "f3"}) {
		t.Fatalf("canonical executable authority = %#v", left.Request.ExecutableFileRefs)
	}
	if batches, err := classificationBatches(left); err != nil || len(batches) != 1 {
		t.Fatalf("executable classification batch = %d / %v", len(batches), err)
	}
	leftWire, err := ProviderVisibleJSON(left)
	if err != nil {
		t.Fatal(err)
	}
	rightWire, err := ProviderVisibleJSON(right)
	if err != nil {
		t.Fatal(err)
	}
	leftState, err := ExecutionState(left)
	if err != nil {
		t.Fatal(err)
	}
	rightState, err := ExecutionState(right)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(leftWire, rightWire) || !bytes.Equal(leftState, rightState) ||
		left.RequestSHA256 != right.RequestSHA256 ||
		!bytes.Contains(leftWire, []byte(`"executable_file_refs":["f1","f3"]`)) {
		t.Fatalf("authority permutation changed compilation:\n%s\n%s\n%s\n%s", leftWire, rightWire, leftState, rightState)
	}
	prompt, err := BuildPrompt(left)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt.System, "complete closed subset") ||
		!strings.Contains(prompt.System, "library-only subset or library default") ||
		!strings.Contains(prompt.User, "exact local authority") ||
		!strings.Contains(prompt.User, "legitimate empty selection") {
		t.Fatalf("executable-authority prompt contract = %#v", prompt)
	}

	defaultRef := corpus.FileID("f1")
	selection, err := ResolveResponse(left, mustResponse(t, &defaultRef, []corpus.FileID{"f1", "f2"}))
	if err != nil || selection.Default == nil || selection.Default.FileRef != "f1" {
		t.Fatalf("exact authority lost to misleading prose: %#v / %v", selection, err)
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
	if !strings.Contains(prompt.System, "Include every required ref in `target_file_refs`") ||
		!strings.Contains(prompt.System, "Do not suppress one language because another language is present") ||
		!strings.Contains(prompt.User, "include every member") {
		t.Fatalf("required-target prompt contract = %#v", prompt)
	}

	for name, raw := range map[string][]byte{
		"empty":       []byte(`{"default_file_ref":null,"target_file_refs":[]}`),
		"one missing": mustResponse(t, fileIDPointer("f1"), []corpus.FileID{"f1"}),
		"unknowns":    mustResponse(t, fileIDPointer("f1"), []corpus.FileID{"f1", "foreign"}),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ResolveResponse(compilation, raw); err == nil ||
				!strings.Contains(err.Error(), "omits exact required target authority") {
				t.Fatalf("authority-losing response error = %v", err)
			}
		})
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

func TestExecutableAuthorityRejectsLibraryOnlyPositiveAuthorityLoss(t *testing.T) {
	snapshot := testSnapshot(t, []string{"cmd/main.go", "pkg/client.go", "worker/main.go"})
	compilation, err := CompileWithExecutableAuthority(snapshot, []Candidate{
		{FileRef: "f1", Hypotheses: []string{"command"}},
		{FileRef: "f2", Hypotheses: []string{"importable library"}},
		{FileRef: "f3", Hypotheses: []string{"worker"}},
	}, []corpus.FileID{"f1", "f3"})
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]struct {
		defaultRef corpus.FileID
		targets    []corpus.FileID
	}{
		"library only":                    {defaultRef: "f2", targets: []corpus.FileID{"f2"}},
		"library default with executable": {defaultRef: "f2", targets: []corpus.FileID{"f1", "f2"}},
		"unknowns filter to library only": {defaultRef: "f2", targets: []corpus.FileID{"foreign", "f2"}},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := ResolveResponse(
				compilation,
				mustResponse(t, &test.defaultRef, test.targets),
			); err == nil {
				t.Fatalf("accepted authority-losing response: default=%s targets=%v", test.defaultRef, test.targets)
			}
		})
	}
	defaultRef := corpus.FileID("f3")
	selection, err := ResolveResponse(
		compilation,
		mustResponse(t, &defaultRef, []corpus.FileID{"f2", "f3"}),
	)
	if err != nil || selection.Default == nil || selection.Default.FileRef != "f3" ||
		!slices.Equal(candidateRefs(selection.Targets), []corpus.FileID{"f2", "f3"}) {
		t.Fatalf("executable default with supporting library = %#v / %v", selection, err)
	}
	empty, err := ResolveResponse(
		compilation,
		[]byte(`{"default_file_ref":null,"target_file_refs":[]}`),
	)
	if err != nil || empty.Default != nil || len(empty.Targets) != 0 || len(empty.Unclassified) != 3 {
		t.Fatalf("legitimate empty selection = %#v / %v", empty, err)
	}
}

func TestCompileWithEmptyExecutableAuthorityPreservesLibraryOnlyContract(t *testing.T) {
	snapshot := testSnapshot(t, []string{"pkg/client.go"})
	bound, err := CompileWithExecutableAuthority(snapshot, []Candidate{
		{FileRef: "f1", Hypotheses: []string{"importable library"}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	wire, err := ProviderVisibleJSON(bound)
	if err != nil {
		t.Fatal(err)
	}
	if bound.Request.ExecutableFileRefs == nil || *bound.Request.ExecutableFileRefs == nil ||
		len(*bound.Request.ExecutableFileRefs) != 0 ||
		!bytes.Contains(wire, []byte(`"executable_file_refs":[]`)) {
		t.Fatalf("empty exact authority was not preserved: request=%#v wire=%s", bound.Request, wire)
	}
	empty, err := ResolveResponse(bound, []byte(`{"default_file_ref":null,"target_file_refs":[]}`))
	if err != nil || empty.Default != nil || len(empty.Targets) != 0 || len(empty.Unclassified) != 1 {
		t.Fatalf("library-only empty selection = %#v / %v", empty, err)
	}

	generic, err := Compile(snapshot, []Candidate{
		{FileRef: "f1", Hypotheses: []string{"importable library"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	genericWire, _ := ProviderVisibleJSON(generic)
	boundState, _ := ExecutionState(bound)
	genericState, _ := ExecutionState(generic)
	if generic.Request.ExecutableFileRefs != nil || bytes.Contains(genericWire, []byte("executable_file_refs")) ||
		bytes.Equal(boundState, genericState) || bound.RequestSHA256 == generic.RequestSHA256 {
		t.Fatalf("bound empty authority collapsed into generic compilation:\nbound=%s\ngeneric=%s", wire, genericWire)
	}
}

func TestExecutableAuthorityIsCurrentRequestBoundAndTamperEvident(t *testing.T) {
	snapshot := testSnapshot(t, []string{"cmd/main.go", "pkg/client.go", "worker/main.go"})
	candidates := []Candidate{
		{FileRef: "f1", Hypotheses: []string{"command"}},
		{FileRef: "f2", Hypotheses: []string{"library"}},
	}
	if _, err := CompileWithExecutableAuthority(snapshot, candidates, []corpus.FileID{"f3"}); err == nil {
		t.Fatal("accepted corpus-current ref outside the current candidate authority")
	}
	if _, err := CompileWithExecutableAuthority(snapshot, candidates, []corpus.FileID{"stale"}); err == nil {
		t.Fatal("accepted stale executable authority ref")
	}

	left, err := CompileWithExecutableAuthority(snapshot, candidates, []corpus.FileID{"f1"})
	if err != nil {
		t.Fatal(err)
	}
	right, err := CompileWithExecutableAuthority(snapshot, candidates, []corpus.FileID{"f2"})
	if err != nil {
		t.Fatal(err)
	}
	leftWire, _ := ProviderVisibleJSON(left)
	rightWire, _ := ProviderVisibleJSON(right)
	leftState, _ := ExecutionState(left)
	rightState, _ := ExecutionState(right)
	if bytes.Equal(leftWire, rightWire) || bytes.Equal(leftState, rightState) ||
		left.RequestSHA256 == right.RequestSHA256 {
		t.Fatalf("material authority change reused identity:\n%s\n%s", leftWire, rightWire)
	}

	(*left.Request.ExecutableFileRefs)[0] = "f2"
	if _, err := ProviderVisibleJSON(left); err == nil {
		t.Fatal("accepted visible executable authority tampering")
	}
	left, err = CompileWithExecutableAuthority(snapshot, candidates, []corpus.FileID{"f1"})
	if err != nil {
		t.Fatal(err)
	}
	left.executableFileRefs[0] = "f2"
	if _, err := ExecutionState(left); err == nil {
		t.Fatal("accepted private executable authority tampering")
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
