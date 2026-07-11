package sourcecard

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/evidence"
)

func TestReadCompleteMethodWindow(t *testing.T) {
	t.Parallel()

	repo := writeRepo(t, `package sample

func (s *server) Put(value string) error {
	if value == "" {
		return errEmpty
	}
	return s.store(value)
}

func (s *server) Delete() {}
`)
	card, err := Read(requestFor(repo, "server.Put", 3), Limits{})
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	if card.Window.StartLine != 3 || card.Window.EndLine != 9 {
		t.Fatalf("window = %d..%d, want 3..9", card.Window.StartLine, card.Window.EndLine)
	}
	if card.Window.StopReason != StopNextTopLevelFunc || card.Window.Truncated {
		t.Fatalf("window stop = %q truncated=%v", card.Window.StopReason, card.Window.Truncated)
	}
	if got := card.Lines[1]; got.EvidenceID != "source-4" || got.Line != 4 {
		t.Fatalf("second line = %#v", got)
	}
	if card.Target.Path != "pkg/sample.go" {
		t.Fatalf("target path = %q", card.Target.Path)
	}
	if len(card.FileSHA256) != 64 {
		t.Fatalf("file sha length = %d", len(card.FileSHA256))
	}
}

func TestReadStopsAtEndOfFile(t *testing.T) {
	t.Parallel()

	repo := writeRepo(t, "package sample\n\nfunc Work() {\n}\n")
	card, err := Read(requestFor(repo, "Work", 3), Limits{})
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if card.Window.StopReason != StopEndOfFile || card.Window.EndLine != 4 {
		t.Fatalf("window = %#v", card.Window)
	}
}

func TestReadAppliesWindowLimits(t *testing.T) {
	t.Parallel()

	repo := writeRepo(t, "package sample\n\nfunc Work() {\n\tone()\n\ttwo()\n\tthree()\n}\n")
	tests := []struct {
		name       string
		limits     Limits
		stopReason StopReason
		warning    string
	}{
		{
			name:       "line limit",
			limits:     Limits{MaxWindowLines: 2},
			stopReason: StopLineLimit,
			warning:    "window.line_limit",
		},
		{
			name:       "byte limit",
			limits:     Limits{MaxWindowBytes: len("func Work() {") + 1},
			stopReason: StopByteLimit,
			warning:    "window.byte_limit",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			card, err := Read(requestFor(repo, "Work", 3), test.limits)
			if err != nil {
				t.Fatalf("Read() error = %v", err)
			}
			if card.Window.StopReason != test.stopReason || !card.Window.Truncated {
				t.Fatalf("window = %#v", card.Window)
			}
			if !hasWarning(card, test.warning) {
				t.Fatalf("warnings = %#v, want %q", card.Warnings, test.warning)
			}
		})
	}
}

func TestReadTruncatesLongLineWithWarning(t *testing.T) {
	t.Parallel()

	repo := writeRepo(t, "package sample\n\nfunc Work() {\n\t"+strings.Repeat("é", 20)+"\n}\n")
	card, err := Read(requestFor(repo, "Work", 3), Limits{MaxLineBytes: 9})
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if !card.Lines[1].Truncated || len(card.Lines[1].Text) > 9 || !hasWarning(card, "line.truncated") {
		t.Fatalf("line = %#v warnings = %#v", card.Lines[1], card.Warnings)
	}
}

func TestReadIsDeterministicAndDoesNotLeakRoot(t *testing.T) {
	t.Parallel()

	repo := writeRepo(t, "package sample\n\nfunc Work() {}\n")
	first, err := Read(requestFor(repo, "Work", 3), Limits{})
	if err != nil {
		t.Fatal(err)
	}
	second, err := Read(requestFor(repo, "Work", 3), Limits{})
	if err != nil {
		t.Fatal(err)
	}
	firstJSON, _ := json.Marshal(first)
	secondJSON, _ := json.Marshal(second)
	if string(firstJSON) != string(secondJSON) {
		t.Fatalf("cards differ:\n%s\n%s", firstJSON, secondJSON)
	}
	if strings.Contains(string(firstJSON), repo) {
		t.Fatalf("card leaked absolute repository root: %s", firstJSON)
	}
}

func TestReadRejectsInvalidRequest(t *testing.T) {
	t.Parallel()

	repo := writeRepo(t, "package sample\n\nfunc Work() {}\n")
	tests := []struct {
		name    string
		request Request
	}{
		{name: "empty path", request: requestForPath(repo, "", "Work", 3)},
		{name: "absolute path", request: requestForPath(repo, filepath.Join(repo, "pkg/sample.go"), "Work", 3)},
		{name: "traversal", request: requestForPath(repo, "../sample.go", "Work", 3)},
		{name: "non go", request: requestForPath(repo, "pkg/sample.txt", "Work", 3)},
		{name: "bad line", request: requestFor(repo, "Work", 30)},
		{name: "wrong name", request: requestFor(repo, "Other", 3)},
		{name: "wrong kind", request: func() Request {
			request := requestFor(repo, "Work", 3)
			request.Target.Kind = evidence.EntityType
			return request
		}()},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := Read(test.request, Limits{}); err == nil {
				t.Fatal("Read() error = nil")
			}
		})
	}
}

func TestReadRejectsOversizedFile(t *testing.T) {
	t.Parallel()

	repo := writeRepo(t, "package sample\n\nfunc Work() {}\n")
	if _, err := Read(requestFor(repo, "Work", 3), Limits{MaxFileBytes: 8}); err == nil {
		t.Fatal("Read() error = nil")
	}
}

func TestReadRejectsSymlinkEscapeAndAllowsContainedSymlink(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "pkg"), 0o700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.go")
	if err := os.WriteFile(outside, []byte("package outside\n\nfunc Work() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	escape := filepath.Join(repo, "pkg", "escape.go")
	if err := os.Symlink(outside, escape); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := Read(requestForPath(repo, "pkg/escape.go", "Work", 3), Limits{}); err == nil {
		t.Fatal("Read() accepted symlink escape")
	}

	inside := filepath.Join(repo, "pkg", "inside.go")
	if err := os.WriteFile(inside, []byte("package sample\n\nfunc Work() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(repo, "pkg", "link.go")
	if err := os.Symlink(inside, link); err != nil {
		t.Fatal(err)
	}
	card, err := Read(requestForPath(repo, "pkg/link.go", "Work", 3), Limits{})
	if err != nil {
		t.Fatalf("Read() contained symlink error = %v", err)
	}
	if card.Target.Path != "pkg/link.go" {
		t.Fatalf("target path = %q", card.Target.Path)
	}
}

func TestValidateForRemoteRejectsCredentialMarkers(t *testing.T) {
	t.Parallel()

	card := validCard()
	tests := []struct {
		name string
		text string
	}{
		{name: "private key", text: `const key = "-----BEGIN RSA PRIVATE KEY-----"`},
		{name: "bearer", text: `const auth = "Bearer abcdefghijklmnop"`},
		{name: "secret key", text: `const key = "sk-abcdefghijklmnop"`},
		{name: "credential assignment", text: `apiKey := "abcdefghijklmnop"`},
		{name: "underscore api key", text: `const API_KEY = "abcdefghijklmnop"`},
		{name: "client secret", text: `ClientSecret := "abcdefghijklmnop"`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			candidate := card
			candidate.Lines = append([]Line{}, card.Lines...)
			candidate.Lines[0].Text = test.text
			if err := ValidateForRemote(candidate); err == nil {
				t.Fatal("ValidateForRemote() error = nil")
			}
		})
	}
	if err := ValidateForRemote(card); err != nil {
		t.Fatalf("ValidateForRemote() safe card error = %v", err)
	}
}

func writeRepo(t *testing.T, source string) string {
	t.Helper()
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "pkg"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "pkg", "sample.go"), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	return repo
}

func requestFor(repo, name string, line int) Request {
	return requestForPath(repo, "pkg/sample.go", name, line)
}

func requestForPath(repo, path, name string, line int) Request {
	return Request{
		RepoPath:         repo,
		TargetEvidenceID: "resolution-001",
		Target: evidence.Entity{
			ID:   "entity-target",
			Kind: evidence.EntityMethod,
			Name: name,
			Location: &evidence.Location{
				Path:   path,
				Line:   line,
				Column: 1,
			},
		},
	}
}

func validCard() Card {
	return Card{
		Version:  Version,
		Language: "go",
		RepoName: "repo",
		Target: Target{
			EvidenceID: "resolution-001",
			EntityID:   "entity-target",
			Name:       "Work",
			Kind:       evidence.EntityFunction,
			Path:       "pkg/sample.go",
			Line:       3,
		},
		FileSHA256: strings.Repeat("a", 64),
		Window: Window{
			StartLine:     3,
			EndLine:       3,
			IncludedBytes: len("func Work() {}"),
			StopReason:    StopEndOfFile,
		},
		Lines:    []Line{{EvidenceID: "source-3", Line: 3, Text: "func Work() {}"}},
		Warnings: []Warning{},
	}
}

func hasWarning(card Card, code string) bool {
	for _, warning := range card.Warnings {
		if warning.Code == code {
			return true
		}
	}
	return false
}
