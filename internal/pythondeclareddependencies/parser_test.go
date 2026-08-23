package pythondeclareddependencies

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/dependencydeclaration"
)

func TestRequirementParserRetainsIdentityAndRedactsLocator(t *testing.T) {
	t.Parallel()
	parsed, err := parseRequirementExpression(
		`Example_Pkg[socks] @ https://user:secret@example.com/archive.whl?token=private ; python_version < "3.13"`,
		"requirements.txt", "",
	)
	if err != nil {
		t.Fatalf("parseRequirementExpression() error = %v", err)
	}
	if parsed.name != "Example_Pkg" || parsed.normalized != "example-pkg" ||
		parsed.locator.Kind != dependencydeclaration.LocatorURL || parsed.locator.Host != "example.com" ||
		!parsed.conditional {
		t.Fatalf("parsed requirement = %#v", parsed)
	}
	rendered := fmt.Sprintf("%#v", parsed)
	if strings.Contains(rendered, "secret") || strings.Contains(rendered, "private") ||
		strings.Contains(rendered, "user") {
		t.Fatalf("parsed locator leaked credentials: %s", rendered)
	}

	hashed, err := parseRequirementExpression(
		"fastapi>=0.100 --hash=sha256:deadbeef --hash sha256:cafe", "requirements.txt", "",
	)
	if err != nil || hashed.specifier != ">=0.100" {
		t.Fatalf("hashed requirement = %#v, %v", hashed, err)
	}
	if _, err := parseRequirementExpression("https://example.com/pkg.whl", "requirements.txt", ""); err == nil {
		t.Fatal("bare URL acquired a package identity")
	}
}

func TestPyprojectParsesPEP621AndPoetryWithoutCredentialPayload(t *testing.T) {
	t.Parallel()
	content := []byte(`
[project]
dependencies = ["fastapi>=0.100", "PyYAML"]
dynamic = ["optional-dependencies"]

[project.optional-dependencies]
test = ["pytest>=8"]

[dependency-groups]
lint = ["ruff>=0.6", { include-group = "test" }]

[build-system]
requires = ["setuptools>=70"]

[tool.poetry.dependencies]
python = ">=3.12"
httpx = "^0.27"
redis = { version = "^5", optional = true }
private-client = { url = "https://user:secret@packages.example/simple?token=private" }

[tool.poetry.group.worker.dependencies]
celery = "^5"
`)
	source := &sourceDraft{
		key: "source-f1-pyproject", entry: corpus.Entry{ID: "f1", Path: "pyproject.toml"},
		format: formatPyproject, state: dependencydeclaration.SourceParsed, content: content,
		digest: textSHA256(string(content)),
	}
	state := &builder{
		projectDir: "", sources: map[string]*sourceDraft{source.key: source},
		frontierSeen: make(map[string]struct{}),
	}
	if err := state.parsePyproject(source); err != nil {
		t.Fatalf("parsePyproject() error = %v", err)
	}
	byName := make(map[string]dependencydeclaration.StatementInput)
	for _, statement := range state.statements {
		byName[statement.Name] = statement
	}
	for _, name := range []string{"fastapi", "PyYAML", "pytest", "ruff", "setuptools", "httpx", "redis", "private-client", "celery"} {
		if _, ok := byName[name]; !ok {
			t.Errorf("statement %q missing from %#v", name, byName)
		}
	}
	if byName["pytest"].Role != dependencydeclaration.RoleOptional || byName["pytest"].Group != "test" ||
		byName["setuptools"].Role != dependencydeclaration.RoleBuild ||
		byName["redis"].Role != dependencydeclaration.RoleOptional ||
		byName["celery"].Role != dependencydeclaration.RoleUnspecified || byName["celery"].Group != "worker" {
		t.Fatalf("role projection = %#v", byName)
	}
	private := byName["private-client"]
	if private.Locator.Kind != dependencydeclaration.LocatorURL || private.Locator.Host != "packages.example" ||
		private.Specifier != "" {
		t.Fatalf("private locator = %#v", private)
	}
	encoded, err := json.Marshal(state.statements)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "secret") || strings.Contains(string(encoded), "private?") ||
		strings.Contains(string(encoded), "token=") {
		t.Fatalf("pyproject statements leaked credentials: %s", encoded)
	}
	foundDynamic := false
	foundGroupBoundary := false
	for _, frontier := range state.frontiers {
		if frontier.Reason == dependencydeclaration.FrontierDynamicDeclaration {
			foundDynamic = true
		}
		if frontier.Section == "dependency-groups.lint" &&
			frontier.Reason == dependencydeclaration.FrontierUnsupportedShape {
			foundGroupBoundary = true
		}
	}
	if !foundDynamic {
		t.Fatal("dynamic PEP 621 declaration was silently dropped")
	}
	if !foundGroupBoundary {
		t.Fatal("PEP 735 include-group was silently dropped")
	}
}

func TestRequirementsCandidateStaysInsideSelectedProject(t *testing.T) {
	t.Parallel()
	state := &builder{
		projectDir:   "services/api",
		excludedDirs: []string{"services/api/vendor/other-project"},
	}
	for _, candidate := range []string{
		"services/api/requirements.txt",
		"services/api/config/requirements-dev.txt",
		"services/api/requirements/base.txt",
		"services/api/requirements/dev/test.txt",
	} {
		if !state.requirementsCandidate(candidate) {
			t.Errorf("candidate %q was not discovered", candidate)
		}
	}
	for _, rejected := range []string{
		"services/other/requirements.txt",
		"services/api/vendor/other-project/requirements.txt",
		"services/api/docs/notes.txt",
	} {
		if state.requirementsCandidate(rejected) {
			t.Errorf("out-of-scope candidate %q was discovered", rejected)
		}
	}
}

func TestLogicalRequirementsPreserveIncludeAndContinuationLocation(t *testing.T) {
	t.Parallel()
	lines, err := logicalRequirements([]byte("-r constraints/base.txt\nhttpx==0.27 \\\n  --hash=sha256:deadbeef\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 3 || lines[0].line != 1 || lines[1].line != 2 || lines[1].incomplete {
		t.Fatalf("logical lines = %#v", lines)
	}
	kind, argument, ok := parseIncludeDirective(lines[0].text)
	if !ok || kind != dependencydeclaration.IncludeRequirement || argument != "constraints/base.txt" {
		t.Fatalf("include = %q %q %v", kind, argument, ok)
	}
}
