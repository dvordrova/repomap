package pavedpath

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCollectRejectsRootEscape(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	if _, err := Collect(repo, "fixture", []string{"../outside.md"}); err == nil {
		t.Fatal("Collect accepted a parent-relative allowed path")
	}

	outside := filepath.Join(t.TempDir(), "outside.md")
	writeCollectorFixture(t, outside, "# Run\n", 0o644)
	if err := os.Symlink(outside, filepath.Join(repo, "README.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := Collect(repo, "fixture", []string{"README.md"}); err == nil {
		t.Fatal("Collect followed an allowed symlink outside the repository root")
	}
}

func TestCollectEnforcesBudgets(t *testing.T) {
	t.Parallel()

	t.Run("file count", func(t *testing.T) {
		repo := t.TempDir()
		allowed := make([]string, 0, collectorMaxFiles+5)
		for index := range collectorMaxFiles + 5 {
			name := fmt.Sprintf("scripts/check-%02d.sh", index)
			writeCollectorFixture(t, filepath.Join(repo, name), "#!/bin/sh\ngo test ./...\n", 0o755)
			allowed = append(allowed, name)
		}
		bundle, err := Collect(repo, "fixture", allowed)
		if err != nil {
			t.Fatal(err)
		}
		if bundle.Stats.ReadFiles != collectorMaxFiles || !bundle.Stats.Truncated {
			t.Fatalf("file budget stats = %+v", bundle.Stats)
		}
		if len(bundle.Evidence) > MaxEvidence {
			t.Fatalf("evidence count = %d, want <= %d", len(bundle.Evidence), MaxEvidence)
		}
	})

	t.Run("per-file and aggregate bytes", func(t *testing.T) {
		repo := t.TempDir()
		fileCount := collectorMaxBytes/collectorMaxFileBytes + 1
		allowed := make([]string, 0, fileCount)
		content := "# Run\n\n```sh\ngo test ./...\n```\n" + strings.Repeat("# bounded filler\n", 4_000)
		for index := range fileCount {
			name := fmt.Sprintf("docs/run-%02d.md", index)
			writeCollectorFixture(t, filepath.Join(repo, name), content, 0o644)
			allowed = append(allowed, name)
		}
		bundle, err := Collect(repo, "fixture", allowed)
		if err != nil {
			t.Fatal(err)
		}
		if bundle.Stats.ReadBytes > collectorMaxBytes || bundle.Stats.ReadFiles > collectorMaxFiles {
			t.Fatalf("byte budget stats = %+v", bundle.Stats)
		}
		if bundle.Stats.ReadBytes != collectorMaxBytes || !bundle.Stats.Truncated {
			t.Fatalf("aggregate budget was not exhausted deterministically: %+v", bundle.Stats)
		}
	})
}

func TestCollectMakeTargets(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	writeCollectorFixture(t, filepath.Join(repo, "Makefile"), `.PHONY: test clean
test:
	go test ./...

clean:
	rm -rf build
`, 0o644)
	bundle, err := Collect(repo, "fixture", []string{"Makefile"})
	if err != nil {
		t.Fatal(err)
	}
	testEvidence := evidenceWithCommand(t, bundle, "make test")
	if testEvidence.Target != "test" || testEvidence.Commands[0].Basis != CommandStructural ||
		!testEvidence.Commands[0].SafeToCopy {
		t.Fatalf("test target evidence = %+v", testEvidence)
	}
	cleanEvidence := evidenceWithCommand(t, bundle, "make clean")
	if cleanEvidence.Commands[0].SafeToCopy {
		t.Fatalf("destructive target was marked safe: %+v", cleanEvidence)
	}
}

func TestCollectMakeSafetyInspectsWholeTarget(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	body := "verify:\n\tgo test ./...\n" + strings.Repeat("\t# harmless filler\n", collectorMaxExcerpt+2) +
		"\trm -rf generated\n"
	writeCollectorFixture(t, filepath.Join(repo, "Makefile"), body, 0o644)
	bundle, err := Collect(repo, "fixture", []string{"Makefile"})
	if err != nil {
		t.Fatal(err)
	}
	item := evidenceWithCommand(t, bundle, "make verify")
	if item.Commands[0].SafeToCopy {
		t.Fatal("target with destructive action beyond display excerpt was copy-enabled")
	}
}

func TestCollectFencedCommand(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	writeCollectorFixture(
		t,
		filepath.Join(repo, "README.md"),
		"# Fixture\n\n## Run the checks\n\n```sh\ngo test ./...\n```\n",
		0o644,
	)
	bundle, err := Collect(repo, "fixture", []string{"README.md"})
	if err != nil {
		t.Fatal(err)
	}
	item := evidenceWithCommand(t, bundle, "go test ./...")
	if item.Commands[0].Basis != CommandExact || !item.Commands[0].SafeToCopy {
		t.Fatalf("fenced command evidence = %+v", item)
	}
	if item.StartLine != 6 || item.EndLine != 6 {
		t.Fatalf("fenced command lines = %d-%d, want 6-6", item.StartLine, item.EndLine)
	}
}

func TestCollectExecutableScript(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	writeCollectorFixture(t, filepath.Join(repo, "scripts/check.sh"), "#!/bin/sh\ngo test ./...\n", 0o755)
	bundle, err := Collect(repo, "fixture", []string{"scripts/check.sh"})
	if err != nil {
		t.Fatal(err)
	}
	item := evidenceWithCommand(t, bundle, "./scripts/check.sh")
	if !item.Executable || item.Role != RoleRepositoryScript || !item.Commands[0].SafeToCopy {
		t.Fatalf("script evidence = %+v", item)
	}
}

func TestCollectEnvironmentRedactsEveryValue(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	writeCollectorFixture(t, filepath.Join(repo, ".env.example"), "PORT=8080\nAPI_TOKEN=do-not-persist\n", 0o644)
	bundle, err := Collect(repo, "fixture", []string{".env.example"})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(raw)
	for _, forbidden := range []string{"8080", "do-not-persist"} {
		if strings.Contains(encoded, forbidden) {
			t.Fatalf("bundle persisted environment value %q: %s", forbidden, encoded)
		}
	}
	redactedValues := 0
	for _, item := range bundle.Evidence {
		if len(item.Excerpt) == 1 && strings.HasSuffix(item.Excerpt[0], "=<redacted>") {
			redactedValues++
		}
	}
	if redactedValues != 2 || bundle.Stats.Redactions != 2 {
		t.Fatalf("environment redaction result = %s, stats=%+v", encoded, bundle.Stats)
	}
}

func TestCollectDoesNotPersistSecrets(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	writeCollectorFixture(
		t,
		filepath.Join(repo, "README.md"),
		"# Setup\n\n```sh\n"+
			"export API_KEY=super-secret-literal\n"+
			"tool --bearer-token=another-secret-literal\n"+
			"curl -H \"Authorization: Bearer bearer-secret\" http://localhost:8080/\n"+
			"go test ./...\n```\n\n"+
			"# secret-access-key: documented-secret-placeholder\n"+
			"-----BEGIN PRIVATE KEY-----\nprivate-key-payload\n-----END PRIVATE KEY-----\n",
		0o644,
	)
	bundle, err := Collect(repo, "fixture", []string{"README.md"})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	encoded := strings.ToLower(string(raw))
	for _, forbidden := range []string{
		"super-secret-literal", "another-secret-literal", "bearer-secret", "documented-secret-placeholder", "private-key-payload", "authorization: bearer",
	} {
		if strings.Contains(encoded, forbidden) {
			t.Fatalf("bundle persisted %q: %s", forbidden, encoded)
		}
	}
	if bundle.Stats.Redactions < 5 {
		t.Fatalf("redactions = %d, want at least 5", bundle.Stats.Redactions)
	}
}

func TestCollectRedactsBasicAuthAndPreservesExcerptCoordinates(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	writeCollectorFixture(t, filepath.Join(repo, "README.md"),
		"# Run\n\n```sh\ngo test ./...\n\ncurl -u alice:hunter2 http://localhost:8080/\ngo vet ./...\n```\n", 0o644)
	bundle, err := Collect(repo, "fixture", []string{"README.md"})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "alice:hunter2") {
		t.Fatalf("basic auth leaked into bundle: %s", raw)
	}
	for _, item := range bundle.Evidence {
		if item.EndLine-item.StartLine+1 != len(item.Excerpt) {
			t.Fatalf("excerpt coordinates shifted: %+v", item)
		}
	}
}

func TestCollectTrimsMarkdownFenceFromLocalEndpoint(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	writeCollectorFixture(t, filepath.Join(repo, "README.md"),
		"Open `http://localhost:3333/` in a browser.\n", 0o644)
	bundle, err := Collect(repo, "fixture", []string{"README.md"})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range bundle.Evidence {
		if item.Role == RoleEndpoint {
			if item.Endpoint != "http://localhost:3333/" {
				t.Fatalf("endpoint = %q", item.Endpoint)
			}
			return
		}
	}
	t.Fatal("local endpoint evidence missing")
}

func TestCollectReservesStructuralEvidenceAndAvoidsIncompleteDerivedCommands(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	var readme strings.Builder
	readme.WriteString("# Commands\n")
	for index := range MaxEvidence + 20 {
		fmt.Fprintf(&readme, "```sh\ngo test ./pkg/%d\n```\n", index)
	}
	writeCollectorFixture(t, filepath.Join(repo, "README.md"), readme.String(), 0o644)
	writeCollectorFixture(t, filepath.Join(repo, "Makefile"), "build:\n\tgo build ./...\n", 0o644)
	writeCollectorFixture(t, filepath.Join(repo, "justfile"), "serve port:\n    go run ./cmd/server --port {{port}}\n", 0o644)
	writeCollectorFixture(t, filepath.Join(repo, "containers/api/Dockerfile"), "FROM scratch\n", 0o644)
	bundle, err := Collect(repo, "fixture", []string{
		"README.md", "Makefile", "justfile", "containers/api/Dockerfile",
	})
	if err != nil {
		t.Fatal(err)
	}
	evidenceWithCommand(t, bundle, "make build")
	for _, item := range bundle.Evidence {
		for _, command := range item.Commands {
			if command.Value == "just serve" || strings.HasPrefix(command.Value, "docker build -f containers/") {
				t.Fatalf("collector invented incomplete structural command: %+v", command)
			}
		}
	}
}

func TestSafeToCopyFailsClosed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		command string
		source  string
		want    bool
	}{
		{name: "go test", command: "go test ./...", want: true},
		{name: "make check", command: "make check", want: true},
		{name: "local run", command: "go run ./cmd/server", want: true},
		{name: "simple checked script", command: "./scripts/check.sh", source: "#!/bin/sh\ngo test ./...", want: true},
		{name: "multiline", command: "go test ./...\ngo vet ./...", want: false},
		{name: "chain", command: "go test ./... && go vet ./...", want: false},
		{name: "redirection", command: "go test ./... > result.txt", want: false},
		{name: "substitution", command: "echo $(date)", want: false},
		{name: "sudo", command: "sudo go test ./...", want: false},
		{name: "destructive", command: "rm -rf build", want: false},
		{name: "restore", command: "restic restore latest", want: false},
		{name: "placeholder", command: "go run <package>", want: false},
		{name: "shell wildcard", command: "go run *.go", want: false},
		{name: "remote write", command: "kubectl apply -f deploy.yml", want: false},
		{name: "credential", command: "tool --token secret", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := safeToCopy(test.command, test.source); got != test.want {
				t.Fatalf("safeToCopy(%q) = %v, want %v", test.command, got, test.want)
			}
		})
	}
}

func TestCollectIsDeterministic(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	writeCollectorFixture(t, filepath.Join(repo, "README.md"), "# Test\n\n```sh\ngo test ./...\n```\n", 0o644)
	writeCollectorFixture(t, filepath.Join(repo, "Makefile"), "build:\n\tgo build ./...\n", 0o644)
	writeCollectorFixture(t, filepath.Join(repo, "scripts/check.sh"), "#!/bin/sh\ngo vet ./...\n", 0o755)

	first, err := Collect(repo, "fixture", []string{"scripts/check.sh", "README.md", "Makefile", "README.md"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := Collect(repo, "fixture", []string{"Makefile", "README.md", "scripts/check.sh"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("Collect is not deterministic:\nfirst:  %#v\nsecond: %#v", first, second)
	}
}

func TestCollectStructuralSources(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	writeCollectorFixture(t, filepath.Join(repo, "Taskfile.yml"), "version: '3'\ntasks:\n  verify:\n    cmds:\n      - go test ./...\n", 0o644)
	writeCollectorFixture(t, filepath.Join(repo, "justfile"), "serve:\n    go run ./cmd/server\n", 0o644)
	writeCollectorFixture(t, filepath.Join(repo, "package.json"), `{"packageManager":"pnpm@9", "scripts":{"test":"node test.js"}}`, 0o644)
	writeCollectorFixture(t, filepath.Join(repo, "compose.yml"), "services:\n  api:\n    image: fixture:latest\n", 0o644)
	writeCollectorFixture(t, filepath.Join(repo, "Dockerfile"), "FROM scratch\n", 0o644)

	bundle, err := Collect(repo, "fixture", []string{
		"Dockerfile", "compose.yml", "package.json", "justfile", "Taskfile.yml",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{
		"task verify", "just serve", "pnpm run test", "docker compose up api", "docker build .",
	} {
		evidenceWithCommand(t, bundle, command)
	}
}

func TestCollectPackageScriptUsesExactScriptsMemberSource(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	writeCollectorFixture(t, filepath.Join(repo, "package.json"), `{
  "test": "not a package script",
  "scripts": {
    "test": "node test.js"
  }
}`, 0o644)
	bundle, err := Collect(repo, "fixture", []string{"package.json"})
	if err != nil {
		t.Fatal(err)
	}
	item := evidenceWithCommand(t, bundle, "npm run test")
	if item.StartLine != 4 || item.EndLine != 4 {
		t.Fatalf("package script lines = %d-%d, want 4-4", item.StartLine, item.EndLine)
	}
	if got := strings.Join(item.Excerpt, "\n"); got != `    "test": "node test.js"` {
		t.Fatalf("package script excerpt = %q", got)
	}
}

func TestCollectPackageScriptRejectsDuplicateJSONSource(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	writeCollectorFixture(
		t,
		filepath.Join(repo, "package.json"),
		`{"scripts":{"test":"node first.js","test":"node second.js"}}`,
		0o644,
	)
	bundle, err := Collect(repo, "fixture", []string{"package.json"})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range bundle.Evidence {
		for _, command := range item.Commands {
			if command.Value == "npm run test" {
				t.Fatalf("duplicate package script produced exact evidence: %+v", item)
			}
		}
	}
}

func TestCollectGenericRepositoryCLIAndRedactedPackageScript(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	writeCollectorFixture(
		t,
		filepath.Join(repo, "README.md"),
		"# Run\n\n```sh\nfixture validate --config config.yml\n```\n",
		0o644,
	)
	writeCollectorFixture(
		t,
		filepath.Join(repo, "package.json"),
		`{"scripts":{"test":"API_TOKEN=package-secret node test.js"}}`,
		0o644,
	)
	bundle, err := Collect(repo, "fixture", []string{"package.json", "README.md"})
	if err != nil {
		t.Fatal(err)
	}
	evidenceWithCommand(t, bundle, "fixture validate --config config.yml")
	raw, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "package-secret") {
		t.Fatalf("package script secret persisted: %s", raw)
	}
	packageItem := evidenceWithCommand(t, bundle, "npm run test")
	if !packageItem.Redacted || packageItem.Commands[0].SafeToCopy {
		t.Fatalf("redacted package script = %+v", packageItem)
	}
}

func TestTruncatedStructuralFileNeverAuthorizesCopy(t *testing.T) {
	t.Parallel()

	lines := []collectedLine{
		{number: 1, text: "verify:"},
		{number: 2, text: "\tgo test ./..."},
	}
	items := parseMakefile("Makefile", lines, true)
	if len(items) != 1 || len(items[0].Commands) != 1 {
		t.Fatalf("items = %#v", items)
	}
	if items[0].Commands[0].SafeToCopy {
		t.Fatal("truncated Makefile authorized a structural command for copying")
	}
}

func writeCollectorFixture(t *testing.T, name, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func evidenceWithCommand(t *testing.T, bundle Bundle, value string) Evidence {
	t.Helper()
	for _, item := range bundle.Evidence {
		for _, command := range item.Commands {
			if command.Value == value {
				return item
			}
		}
	}
	t.Fatalf("command %q not found in evidence: %+v", value, bundle.Evidence)
	return Evidence{}
}
