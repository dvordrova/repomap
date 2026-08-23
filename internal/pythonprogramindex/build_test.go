package pythonprogramindex

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/gitfiles"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/pythontarget"
)

func buildOneForTest(
	ctx context.Context,
	repository *corpus.Corpus,
	target pythontarget.Target,
) (programindex.Index, error) {
	indexes, err := BuildMany(ctx, repository, []pythontarget.Target{target})
	if err != nil {
		return programindex.Index{}, err
	}
	return indexes[0], nil
}

func TestBuildKeepsMutablePythonBindingsAsFrontiers(t *testing.T) {
	repository := pythonCorpus(t, map[string]string{
		"pyproject.toml": `[project]
name = "demo"
version = "1.0.0"
`,
		"pkg/__init__.py": "",
		"pkg/decorators.py": `def decorate(fn):
    return fn
`,
		"pkg/tasks.py": `def callback():
    return 1
`,
		"pkg/service.py": `import json as codec
from .decorators import decorate as mark
from .tasks import callback as cb

class Base:
    pass

class Worker(Base):
    def execute(self):
        return run()

@mark
def run():
    cb()
    return codec.dumps({"ok": True})

def invoke(fn):
    return fn()

def main():
    invoke(cb)

factory = lambda: Worker()
_hidden = 1
`,
	})
	target := targetOfKind(t, repository, pythontarget.KindLibrary)

	index, err := buildOneForTest(context.Background(), repository, target)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if err := index.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	assertTargetCorpusRefs(t, index, target)
	if len(index.Target.Seeds) != 0 {
		t.Fatalf("library target invented launch seeds: %#v", index.Target.Seeds)
	}

	service := objectNamed(t, index, programindex.ObjectModule, "pkg.service", "pkg/service.py")
	decorate := objectNamed(t, index, programindex.ObjectFunction, "decorate", "pkg/decorators.py")
	callback := objectNamed(t, index, programindex.ObjectFunction, "callback", "pkg/tasks.py")
	run := objectNamed(t, index, programindex.ObjectFunction, "run", "pkg/service.py")
	main := objectNamed(t, index, programindex.ObjectFunction, "main", "pkg/service.py")
	base := objectNamed(t, index, programindex.ObjectType, "Base", "pkg/service.py")
	worker := objectNamed(t, index, programindex.ObjectType, "Worker", "pkg/service.py")
	execute := objectNamed(t, index, programindex.ObjectMethod, "execute", "pkg/service.py")
	hidden := objectNamed(t, index, programindex.ObjectVariable, "_hidden", "pkg/service.py")
	_ = objectOfKindAtPath(t, index, programindex.ObjectLambda, "pkg/service.py")
	if hidden.Visibility != programindex.VisibilityInternal {
		t.Fatalf("_hidden visibility = %q, want internal", hidden.Visibility)
	}
	if execute.OwnerID != worker.ID || execute.ContainerID != worker.ID {
		t.Fatalf("method ownership = owner %q container %q, want Worker %q", execute.OwnerID, execute.ContainerID, worker.ID)
	}

	assertExactRelation(t, index, programindex.RelationImports, service.ID, callback.ID)
	assertAlternativeRelation(t, index, programindex.RelationCalls, run.ID, callback.ID)
	assertAlternativeRelation(t, index, programindex.RelationDecorates, run.ID, decorate.ID)
	assertAlternativeRelation(t, index, programindex.RelationImplements, worker.ID, base.ID)
	assertAlternativeRelation(t, index, programindex.RelationPassesCallback, main.ID, callback.ID)
	assertExactRelation(t, index, programindex.RelationContains, worker.ID, execute.ID)

	external := objectNamed(t, index, programindex.ObjectExternalSymbol, "json.dumps", "")
	if external.Visibility != programindex.VisibilityUnknown {
		t.Fatalf("external visibility = %q, want unknown", external.Visibility)
	}
	assertAlternativeRelation(t, index, programindex.RelationInvokesExternal, run.ID, external.ID)

	invoke := objectNamed(t, index, programindex.ObjectFunction, "invoke", "pkg/service.py")
	assertUnresolvedFrom(t, index, programindex.RelationCalls, invoke.ID)
}

func TestBuildKeepsDynamicOperationsAsFrontiers(t *testing.T) {
	repository := pythonCorpus(t, map[string]string{
		"pyproject.toml": `[project]
name = "dynamic-demo"
version = "1.0.0"
`,
		"dynamic_pkg/__init__.py": "",
		"dynamic_pkg/runtime.py": `import importlib

def dynamic(name, obj, monkeypatch):
    module = importlib.import_module(name)
    value = getattr(module, name)
    setattr(obj, name, value)
    monkeypatch.setattr(obj, name, value)
    return obj.handler()
`,
	})
	target := targetOfKind(t, repository, pythontarget.KindLibrary)
	index, err := buildOneForTest(context.Background(), repository, target)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	dynamic := objectNamed(t, index, programindex.ObjectFunction, "dynamic", "dynamic_pkg/runtime.py")

	for _, kind := range []programindex.RelationKind{
		programindex.RelationImports,
		programindex.RelationReads,
		programindex.RelationWrites,
		programindex.RelationCalls,
	} {
		relation := unresolvedFrom(t, index, kind, dynamic.ID)
		if relation.Location == nil || relation.Location.Path != "dynamic_pkg/runtime.py" ||
			relation.TargetsObserved < 1 || relation.WitnessesObserved < 1 || len(relation.Witnesses) == 0 {
			t.Fatalf("unresolved %s lost frontier evidence: %#v", kind, relation)
		}
	}
}

func TestBuildRetainsCallableAliasesAndClosedLocalDynamicImport(t *testing.T) {
	repository := pythonCorpus(t, map[string]string{
		"pyproject.toml": `[project]
name = "alias-demo"
version = "1.0.0"
`,
		"alias_pkg/__init__.py": "",
		"alias_pkg/hooks.py": `def decorate(value):
    return value
`,
		"alias_pkg/plugin.py": `def callback():
    return 1
`,
		"alias_pkg/runtime.py": `import importlib
from .hooks import decorate
from .plugin import callback

decorator_alias = decorate
callback_alias = callback

@decorator_alias
def run():
    return callback_alias()

def patch(monkeypatch, obj):
    installed = callback_alias
    monkeypatch.setattr(obj, "handler", installed)

def load_local():
    return importlib.import_module("alias_pkg.plugin")

def load_unknown(name):
    return importlib.import_module(name)

def load_missing_literal():
    return importlib.import_module("third_party_plugin")

def load_through_impostor(importlib):
    return importlib.import_module("alias_pkg.plugin")

def load_through_arbitrary_member(loader):
    return loader.import_module("alias_pkg.plugin")
`,
	})
	target := targetOfKind(t, repository, pythontarget.KindLibrary)
	index, err := buildOneForTest(context.Background(), repository, target)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if err := index.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	run := objectNamed(t, index, programindex.ObjectFunction, "run", "alias_pkg/runtime.py")
	decorate := objectNamed(t, index, programindex.ObjectFunction, "decorate", "alias_pkg/hooks.py")
	callback := objectNamed(t, index, programindex.ObjectFunction, "callback", "alias_pkg/plugin.py")
	patch := objectNamed(t, index, programindex.ObjectFunction, "patch", "alias_pkg/runtime.py")
	loadLocal := objectNamed(t, index, programindex.ObjectFunction, "load_local", "alias_pkg/runtime.py")
	loadUnknown := objectNamed(t, index, programindex.ObjectFunction, "load_unknown", "alias_pkg/runtime.py")
	loadMissingLiteral := objectNamed(t, index, programindex.ObjectFunction, "load_missing_literal", "alias_pkg/runtime.py")
	loadThroughImpostor := objectNamed(t, index, programindex.ObjectFunction, "load_through_impostor", "alias_pkg/runtime.py")
	loadThroughArbitraryMember := objectNamed(t, index, programindex.ObjectFunction, "load_through_arbitrary_member", "alias_pkg/runtime.py")
	plugin := objectNamed(t, index, programindex.ObjectModule, "alias_pkg.plugin", "alias_pkg/plugin.py")

	assertAlternativeWitness(t, index, programindex.RelationDecorates, run.ID, decorate.ID, "alias_pkg.hooks.decorate -> decorate")
	assertAlternativeWitness(t, index, programindex.RelationCalls, run.ID, callback.ID, "alias_pkg.plugin.callback -> callback")
	assertAlternativeWitnessExpression(t, index, programindex.RelationCalls, run.ID, callback.ID, "callback_alias")
	assertAlternativeWitness(t, index, programindex.RelationPassesCallback, patch.ID, callback.ID, "installed -> callback")
	assertExactRelation(t, index, programindex.RelationImports, loadLocal.ID, plugin.ID)
	assertUnresolvedFrom(t, index, programindex.RelationImports, loadUnknown.ID)
	assertUnresolvedFrom(t, index, programindex.RelationImports, loadMissingLiteral.ID)
	for _, caller := range []programindex.Object{loadThroughImpostor, loadThroughArbitraryMember} {
		assertUnresolvedFrom(t, index, programindex.RelationCalls, caller.ID)
		for _, relation := range index.Relations {
			if relation.Kind == programindex.RelationImports && relation.FromID == caller.ID {
				t.Fatalf("ordinary .import_module call became import authority: %#v", relation)
			}
		}
	}
	for _, object := range index.Objects {
		if object.Kind == programindex.ObjectExternalSymbol && object.Name == "third_party_plugin" {
			t.Fatalf("unknown dynamic-import literal became invented authority: %#v", object)
		}
	}
}

func TestBuildResolvesProjectRelativeSrcImportsAndKeepsOnlyDecoratorCallee(t *testing.T) {
	repository := pythonCorpus(t, map[string]string{
		"pyproject.toml": `[project]
name = "src-app"
version = "1.0.0"

[tool.setuptools]
package-dir = {"" = "src"}
`,
		"src/__init__.py": "",
		"src/config.py":   "def load():\n    return 1\n",
		"src/main.py": `from src.config import load
from fastapi import FastAPI

app = FastAPI()

@app.get("/healthcheck")
def healthcheck():
    return load()
`,
	})
	target := targetOfKind(t, repository, pythontarget.KindLibrary)
	index, err := buildOneForTest(context.Background(), repository, target)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	mainModule := objectNamed(t, index, programindex.ObjectModule, "main", "src/main.py")
	load := objectNamed(t, index, programindex.ObjectFunction, "load", "src/config.py")
	healthcheck := objectNamed(t, index, programindex.ObjectFunction, "healthcheck", "src/main.py")
	assertExactRelation(t, index, programindex.RelationImports, mainModule.ID, load.ID)
	assertAlternativeRelation(t, index, programindex.RelationCalls, healthcheck.ID, load.ID)
	assertExternalCallExpression(t, index, "fastapi.FastAPI", "FastAPI")
	for _, object := range index.Objects {
		if object.Kind == programindex.ObjectExternalSymbol && strings.HasPrefix(object.Name, "src.") {
			t.Fatalf("project-local src import became external: %#v", object)
		}
	}
	foundDecoratorCallee := false
	for _, relation := range index.Relations {
		if relation.Kind != programindex.RelationDecorates || relation.Location == nil ||
			relation.Location.Path != "src/main.py" {
			continue
		}
		for _, witness := range relation.Witnesses {
			if witness.Detail == "app.get" {
				foundDecoratorCallee = true
			}
			if strings.Contains(witness.Detail, "/healthcheck") {
				t.Fatalf("route literal persisted in decorator witness: %#v", witness)
			}
		}
	}
	if !foundDecoratorCallee {
		t.Fatal("route decorator lost its structural callee name")
	}
}

func TestBuildFrameworkNeutralModuleExecutionViewSeedsExactModule(t *testing.T) {
	repository := pythonCorpus(t, map[string]string{
		"pyproject.toml": `[project]
name = "application-seed"
version = "1.0.0"
`,
		"app.py": `from runtime import Whatever

app = Whatever()
`,
	})
	catalog, err := pythontarget.Discover(context.Background(), repository)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	resolver, err := pythontarget.NewFileTargetResolver(repository, catalog)
	if err != nil {
		t.Fatalf("NewFileTargetResolver: %v", err)
	}
	appRef, ok := repository.ID("app.py")
	if !ok {
		t.Fatal("app.py is absent from corpus")
	}
	target, err := resolver.ResolveOne(appRef)
	if err != nil {
		t.Fatalf("ResolveOne: %v", err)
	}
	index, err := buildOneForTest(context.Background(), repository, target)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(index.Target.Seeds) != 1 {
		t.Fatalf("module-execution target seeds = %#v, want one exact module", index.Target.Seeds)
	}
	kinds := make(map[programindex.SeedKind]programindex.Object)
	for _, seed := range index.Target.Seeds {
		kinds[seed.Kind] = objectByID(t, index, seed.ObjectID)
	}
	if kinds[programindex.SeedModule].Kind != programindex.ObjectModule ||
		kinds[programindex.SeedModule].Name != "app" {
		t.Fatalf("containing module seed = %#v", kinds[programindex.SeedModule])
	}
}

func TestBuildDoesNotPersistSignatureOrDecoratorLiterals(t *testing.T) {
	repository := pythonCorpus(t, map[string]string{
		"pyproject.toml": `[project]
name = "literal-safety"
version = "1.0.0"
`,
		"safe_pkg/__init__.py": "",
		"safe_pkg/api.py": `from framework import route

@route("PRIVATE_ROUTE_LITERAL", token="DECORATOR_TOKEN_LITERAL")
def endpoint(secret="DEFAULT_SECRET_LITERAL", *, api_key: "ANNOTATION_LITERAL" = "KEY_LITERAL") -> "RETURN_LITERAL":
    return secret

handler = lambda token="LAMBDA_DEFAULT_LITERAL": token

class Store(Generic["CLASS_BASE_LITERAL"]):
    pass
`,
	})
	target := targetOfKind(t, repository, pythontarget.KindLibrary)
	index, err := buildOneForTest(context.Background(), repository, target)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	endpoint := objectNamed(t, index, programindex.ObjectFunction, "endpoint", "safe_pkg/api.py")
	if endpoint.Signature != "endpoint(secret, *, api_key)" {
		t.Fatalf("endpoint signature = %q, want structural parameters only", endpoint.Signature)
	}
	lambda := objectOfKindAtPath(t, index, programindex.ObjectLambda, "safe_pkg/api.py")
	if lambda.Signature != "lambda token" {
		t.Fatalf("lambda signature = %q, want structural parameters only", lambda.Signature)
	}
	store := objectNamed(t, index, programindex.ObjectType, "Store", "safe_pkg/api.py")
	if store.Signature != "class Store(Generic)" {
		t.Fatalf("class signature = %q, want structural base name only", store.Signature)
	}

	wire, err := programindex.Encode(index)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	for _, literal := range []string{
		"PRIVATE_ROUTE_LITERAL",
		"DECORATOR_TOKEN_LITERAL",
		"DEFAULT_SECRET_LITERAL",
		"ANNOTATION_LITERAL",
		"KEY_LITERAL",
		"RETURN_LITERAL",
		"LAMBDA_DEFAULT_LITERAL",
		"CLASS_BASE_LITERAL",
	} {
		if strings.Contains(string(wire), literal) {
			t.Fatalf("persistent program index contains source literal %q", literal)
		}
	}
}

func TestBuildVisitsNestedCallExpressionsOnce(t *testing.T) {
	repository := pythonCorpus(t, map[string]string{
		"pyproject.toml": `[project]
name = "nested-calls"
version = "1.0.0"
`,
		"nested_calls/__init__.py": "",
		"nested_calls/calls.py": `def outer(value):
    return value

def middle(value):
    return value

def leaf():
    return 1

def sample():
    return outer(middle(leaf()))
`,
	})
	target := targetOfKind(t, repository, pythontarget.KindLibrary)
	index, err := buildOneForTest(context.Background(), repository, target)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	sample := objectNamed(t, index, programindex.ObjectFunction, "sample", "nested_calls/calls.py")
	calls := 0
	for _, relation := range index.Relations {
		if relation.Kind != programindex.RelationCalls || relation.FromID != sample.ID {
			continue
		}
		calls++
		if relation.Resolution != programindex.ResolutionAlternatives || len(relation.ToIDs) != 1 ||
			relation.TargetsOmitted != 0 || relation.WitnessesObserved != 1 || len(relation.Witnesses) != 1 {
			t.Fatalf("nested call was visited more than once or promoted to exact: %#v", relation)
		}
	}
	if calls != 3 {
		t.Fatalf("sample call relations = %d, want outer, middle, and leaf once each", calls)
	}
}

func TestBuildKeepsDefinitionHeaderCallsInTheDefiningScope(t *testing.T) {
	repository := pythonCorpus(t, map[string]string{
		"pyproject.toml": `[project]
name = "definition-headers"
version = "1.0.0"
`,
		"definition_headers/__init__.py": "",
		"definition_headers/model.py": `def make():
    return object()

def annotate():
    return object

def sample(value: annotate() = make(), *, flag=make()) -> annotate():
    return value

handler = lambda value=make(): value

class Child(make(), metaclass=make()):
    pass
`,
	})
	target := targetOfKind(t, repository, pythontarget.KindLibrary)
	index, err := buildOneForTest(context.Background(), repository, target)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	module := objectNamed(
		t, index, programindex.ObjectModule,
		"definition_headers.model", "definition_headers/model.py",
	)
	headerCalls := 0
	for _, relation := range index.Relations {
		if relation.Kind != programindex.RelationCalls || relation.FromID != module.ID ||
			relation.Location == nil || relation.Location.Path != "definition_headers/model.py" {
			continue
		}
		headerCalls++
		if relation.Resolution != programindex.ResolutionAlternatives || len(relation.ToIDs) != 1 ||
			relation.TargetsOmitted != 0 || relation.WitnessesObserved != 1 || len(relation.Witnesses) != 1 {
			t.Fatalf("definition-header call lost its possible-target witness: %#v", relation)
		}
	}
	if headerCalls != 7 {
		t.Fatalf("definition-header calls = %d, want two defaults, two annotations, one lambda default, base, and metaclass", headerCalls)
	}
}

func TestLimitedBufferDrainsAndReportsOverflow(t *testing.T) {
	var buffer limitedBuffer
	buffer.limit = 4

	written, err := buffer.Write([]byte("abcdef"))
	if err != nil || written != 6 {
		t.Fatalf("first Write = (%d, %v), want (6, nil)", written, err)
	}
	written, err = buffer.Write([]byte("gh"))
	if err != nil || written != 2 {
		t.Fatalf("second Write = (%d, %v), want (2, nil)", written, err)
	}
	if buffer.String() != "abcd" || !buffer.Exceeded() {
		t.Fatalf("limited buffer = %q exceeded=%t, want capped drained buffer", buffer.String(), buffer.Exceeded())
	}
}

func TestBuildManyParsesSharedInventoryOnceAndPreservesOrder(t *testing.T) {
	repository := pythonCorpus(t, map[string]string{
		"pyproject.toml": `[project]
name = "cli-demo"
version = "1.0.0"

[project.scripts]
first = "cli_pkg.cli:first"
second = "cli_pkg.cli:second"
`,
		"cli_pkg/__init__.py": "",
		"cli_pkg/cli.py": `def first():
    return shared()

def second():
    return shared()

def shared():
    return 1
`,
	})
	catalog, err := pythontarget.Discover(context.Background(), repository)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	executables := make([]pythontarget.Target, 0, 2)
	for _, target := range catalog.Entries {
		if target.Kind == pythontarget.KindExecutable {
			executables = append(executables, target)
		}
	}
	if len(executables) != 2 {
		t.Fatalf("executable targets = %d, want 2", len(executables))
	}
	targets := []pythontarget.Target{executables[1], executables[0]}
	invocations := 0
	indexes, err := buildMany(context.Background(), repository, targets, func(ctx context.Context, request parserRequest) (parserResponse, error) {
		invocations++
		return runParser(ctx, request)
	})
	if err != nil {
		t.Fatalf("buildMany: %v", err)
	}
	if invocations != 1 {
		t.Fatalf("parser invocations = %d, want 1", invocations)
	}
	if len(indexes) != len(targets) {
		t.Fatalf("indexes = %d, want %d", len(indexes), len(targets))
	}
	for position, target := range targets {
		if indexes[position].Target.Name != target.DisplayName {
			t.Fatalf("index %d name = %q, want %q", position, indexes[position].Target.Name, target.DisplayName)
		}
		assertTargetCorpusRefs(t, indexes[position], target)
		if len(indexes[position].Target.Seeds) != 1 {
			t.Fatalf("index %d seeds = %#v, want one exact script callable", position, indexes[position].Target.Seeds)
		}
		seed := objectByID(t, indexes[position], indexes[position].Target.Seeds[0].ObjectID)
		if seed.Name != target.Roots[0].Qualname {
			t.Fatalf("index %d seed = %q, want %q", position, seed.Name, target.Roots[0].Qualname)
		}
	}
	if indexes[0].SourceSHA256 != indexes[1].SourceSHA256 || indexes[0].ScenarioSHA256 != indexes[1].ScenarioSHA256 {
		t.Fatal("shared module inventory did not share source/parser identity")
	}
	if indexes[0].Target.ID == indexes[1].Target.ID {
		t.Fatal("different exact script roots share one target identity")
	}
}

func TestBuildManyKeepsExecutableAndLibrarySemanticsTargetLocal(t *testing.T) {
	repository := pythonCorpus(t, map[string]string{
		"pyproject.toml": `[project]
name = "mixed-demo"
version = "1.0.0"

[project.scripts]
mixed = "mixed.cli:main"
`,
		"mixed/__init__.py": "",
		"mixed/cli.py": `from . import service

def main():
    return service.run()
`,
		"mixed/service.py": "def run():\n    return 1\n",
	})
	catalog, err := pythontarget.Discover(context.Background(), repository)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	var executable, library pythontarget.Target
	for _, target := range catalog.Entries {
		switch target.Kind {
		case pythontarget.KindExecutable:
			executable = target
		case pythontarget.KindLibrary:
			library = target
		}
	}
	if executable.Ref == "" || library.Ref == "" {
		t.Fatalf("mixed project targets missing: executable=%q library=%q", executable.Ref, library.Ref)
	}
	if !reflect.DeepEqual(executable.Modules, library.Modules) {
		t.Fatal("fixture does not exercise a shared module inventory")
	}

	singleExecutable, err := buildMany(context.Background(), repository, []pythontarget.Target{executable}, runParser)
	if err != nil {
		t.Fatalf("build executable: %v", err)
	}
	singleLibrary, err := buildMany(context.Background(), repository, []pythontarget.Target{library}, runParser)
	if err != nil {
		t.Fatalf("build library: %v", err)
	}
	invocations := 0
	batch, err := buildMany(
		context.Background(), repository, []pythontarget.Target{executable, library},
		func(ctx context.Context, request parserRequest) (parserResponse, error) {
			invocations++
			return runParser(ctx, request)
		},
	)
	if err != nil {
		t.Fatalf("build mixed batch: %v", err)
	}
	if invocations != 1 {
		t.Fatalf("parser invocations = %d, want one shared AST parse with two semantic projections", invocations)
	}
	if !reflect.DeepEqual(batch[0], singleExecutable[0]) {
		t.Fatal("executable projection changed when a library view shared its source inventory")
	}
	if !reflect.DeepEqual(batch[1], singleLibrary[0]) {
		t.Fatal("library projection changed when an executable view shared its source inventory")
	}
}

func TestBuildRejectsTargetEvidenceFromAnotherCorpus(t *testing.T) {
	original := pythonCorpus(t, map[string]string{
		"demo/__init__.py": "def value():\n    return 1\n",
		"pyproject.toml": `[project]
name = "evidence-demo"
version = "1.0.0"
`,
	})
	target := targetOfKind(t, original, pythontarget.KindLibrary)

	t.Run("ref now names another path", func(t *testing.T) {
		drifted := pythonCorpus(t, map[string]string{
			"demo/__init__.py": "def value():\n    return 1\n",
			"requirements.txt": "requests==1.0.0\n",
		})
		_, err := buildOneForTest(context.Background(), drifted, target)
		if err == nil || !strings.Contains(err.Error(), "resolves to \"requirements.txt\", not \"pyproject.toml\"") {
			t.Fatalf("Build error = %v, want exact corpus path mismatch", err)
		}
	})

	t.Run("ref disappeared", func(t *testing.T) {
		drifted := pythonCorpus(t, map[string]string{
			"demo/__init__.py": "def value():\n    return 1\n",
		})
		_, err := buildOneForTest(context.Background(), drifted, target)
		if err == nil || !strings.Contains(err.Error(), "is outside repository corpus") {
			t.Fatalf("Build error = %v, want unknown corpus ref", err)
		}
	})
}

func TestBuildResolvesModuleAndMainGuardSeedsWithLineOneDeclarations(t *testing.T) {
	repository := pythonCorpus(t, map[string]string{
		"pyproject.toml": `[project]
name = "launch-lines"
version = "1.0.0"
`,
		"pkg/__init__.py": "",
		"pkg/__main__.py": `def declared_on_first_line():
    return 1

if __name__ == "__main__":
    declared_on_first_line()
`,
		"tool.py": `def declared_on_first_line():
    return 1

if __name__ == "__main__":
    declared_on_first_line()
`,
	})
	catalog, err := pythontarget.Discover(context.Background(), repository)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	moduleTarget := pythonTargetBySelector(t, catalog, "python:.:module:pkg")
	guardTarget := pythonTargetBySelector(t, catalog, "python:.:guard:tool")
	indexes, err := BuildMany(context.Background(), repository, []pythontarget.Target{moduleTarget, guardTarget})
	if err != nil {
		t.Fatalf("BuildMany: %v", err)
	}
	if len(indexes) != 2 {
		t.Fatalf("indexes = %d, want 2", len(indexes))
	}

	moduleSeed := indexes[0].Target.Seeds
	if len(moduleSeed) != 1 || moduleSeed[0].Kind != programindex.SeedModule ||
		moduleSeed[0].Location == nil || moduleSeed[0].Location.Line != 1 {
		t.Fatalf("package __main__ seed = %#v, want module launch on line 1", moduleSeed)
	}
	moduleObject := objectByID(t, indexes[0], moduleSeed[0].ObjectID)
	if moduleObject.Kind != programindex.ObjectModule || moduleObject.Location == nil || moduleObject.Location.Line != 1 {
		t.Fatalf("package __main__ seed object = %#v, want unambiguous module declaration", moduleObject)
	}

	guardSeed := indexes[1].Target.Seeds
	if len(guardSeed) != 1 || guardSeed[0].Kind != programindex.SeedMainGuard ||
		guardSeed[0].Location == nil || guardSeed[0].Location.Line != 4 {
		t.Fatalf("main-guard seed = %#v, want launch on guard line 4", guardSeed)
	}
	guardModule := objectByID(t, indexes[1], guardSeed[0].ObjectID)
	if guardModule.Kind != programindex.ObjectModule || guardModule.Location == nil || guardModule.Location.Line != 1 {
		t.Fatalf("main-guard seed object = %#v, want module declaration on line 1", guardModule)
	}
}

func TestBuildIndexesOnlySelectedTargetModules(t *testing.T) {
	repository := pythonCorpus(t, map[string]string{
		"pyproject.toml": `[project]
name = "root-demo"
version = "1.0.0"
`,
		"root_pkg/__init__.py": "",
		"root_pkg/api.py":      "def included():\n    return 1\n",
		"nested/pyproject.toml": `[project]
name = "nested-demo"
version = "1.0.0"
`,
		"nested/nested_pkg/__init__.py": "",
		"nested/nested_pkg/api.py":      "def excluded():\n    return 2\n",
	})
	catalog, err := pythontarget.Discover(context.Background(), repository)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	var target pythontarget.Target
	for _, candidate := range catalog.Entries {
		if candidate.Kind == pythontarget.KindLibrary && candidate.ProjectDir == "." {
			target = candidate
			break
		}
	}
	if target.Ref == "" {
		t.Fatal("root library target not discovered")
	}
	index, err := buildOneForTest(context.Background(), repository, target)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	for _, object := range index.Objects {
		if object.Name == "excluded" || object.Location != nil && strings.HasPrefix(object.Location.Path, "nested/") {
			t.Fatalf("selected root target contains nested-project object %#v", object)
		}
	}
}

func pythonCorpus(t *testing.T, files map[string]string) *corpus.Corpus {
	t.Helper()
	repositoryPath := t.TempDir()
	paths := make([]string, 0, len(files))
	for path, content := range files {
		absolute := filepath.Join(repositoryPath, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
			t.Fatalf("MkdirAll(%s): %v", path, err)
		}
		if err := os.WriteFile(absolute, []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile(%s): %v", path, err)
		}
		paths = append(paths, path)
	}
	sort.Strings(paths)
	repository, err := corpus.New(context.Background(), repositoryPath, gitfiles.Listing{
		Paths: append([]string(nil), paths...), RegularPaths: append([]string(nil), paths...),
	})
	if err != nil {
		t.Fatalf("corpus.New: %v", err)
	}
	t.Cleanup(func() {
		if err := repository.Close(); err != nil {
			t.Errorf("Corpus.Close: %v", err)
		}
	})
	return repository
}

func targetOfKind(t *testing.T, repository *corpus.Corpus, kind pythontarget.Kind) pythontarget.Target {
	t.Helper()
	catalog, err := pythontarget.Discover(context.Background(), repository)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	for _, target := range catalog.Entries {
		if target.Kind == kind {
			return target
		}
	}
	t.Fatalf("target kind %q not discovered", kind)
	return pythontarget.Target{}
}

func pythonTargetBySelector(t *testing.T, catalog pythontarget.Catalog, selector string) pythontarget.Target {
	t.Helper()
	for _, target := range catalog.Entries {
		if target.Selector == selector {
			return target
		}
	}
	t.Fatalf("Python target selector %q not discovered", selector)
	return pythontarget.Target{}
}

func objectNamed(
	t *testing.T,
	index programindex.Index,
	kind programindex.ObjectKind,
	name string,
	path string,
) programindex.Object {
	t.Helper()
	for _, object := range index.Objects {
		if object.Kind != kind || object.Name != name {
			continue
		}
		if path == "" && object.Location == nil || object.Location != nil && object.Location.Path == path {
			return object
		}
	}
	t.Fatalf("object %s %q at %q not found", kind, name, path)
	return programindex.Object{}
}

func objectByID(t *testing.T, index programindex.Index, id string) programindex.Object {
	t.Helper()
	for _, object := range index.Objects {
		if object.ID == id {
			return object
		}
	}
	t.Fatalf("object id %q not found", id)
	return programindex.Object{}
}

func objectOfKindAtPath(
	t *testing.T,
	index programindex.Index,
	kind programindex.ObjectKind,
	path string,
) programindex.Object {
	t.Helper()
	for _, object := range index.Objects {
		if object.Kind == kind && object.Location != nil && object.Location.Path == path {
			return object
		}
	}
	t.Fatalf("object %s at %q not found", kind, path)
	return programindex.Object{}
}

func assertExactRelation(
	t *testing.T,
	index programindex.Index,
	kind programindex.RelationKind,
	fromID string,
	toID string,
) {
	t.Helper()
	for _, relation := range index.Relations {
		if relation.Kind == kind && relation.FromID == fromID &&
			relation.Resolution == programindex.ResolutionExact && reflect.DeepEqual(relation.ToIDs, []string{toID}) {
			if len(relation.Witnesses) == 0 || relation.WitnessesObserved < 1 {
				t.Fatalf("exact %s relation has no witness: %#v", kind, relation)
			}
			return
		}
	}
	t.Fatalf("exact %s relation %q -> %q not found", kind, fromID, toID)
}

func assertAlternativeRelation(
	t *testing.T,
	index programindex.Index,
	kind programindex.RelationKind,
	fromID string,
	toID string,
) {
	t.Helper()
	for _, relation := range index.Relations {
		if relation.Kind == kind && relation.FromID == fromID &&
			relation.Resolution == programindex.ResolutionAlternatives && reflect.DeepEqual(relation.ToIDs, []string{toID}) {
			if len(relation.Witnesses) == 0 || relation.WitnessesObserved < 1 {
				t.Fatalf("alternative %s relation has no witness: %#v", kind, relation)
			}
			return
		}
	}
	t.Fatalf("alternative %s relation %q -> %q not found", kind, fromID, toID)
}

func assertUnresolvedFrom(t *testing.T, index programindex.Index, kind programindex.RelationKind, fromID string) {
	t.Helper()
	_ = unresolvedFrom(t, index, kind, fromID)
}

func assertAlternativeWitness(
	t *testing.T,
	index programindex.Index,
	kind programindex.RelationKind,
	fromID string,
	toID string,
	detail string,
) {
	t.Helper()
	related := make([]programindex.Relation, 0)
	for _, relation := range index.Relations {
		if relation.Kind == kind && relation.FromID == fromID {
			related = append(related, relation)
		}
		if relation.Kind != kind || relation.FromID != fromID ||
			relation.Resolution != programindex.ResolutionAlternatives ||
			!reflect.DeepEqual(relation.ToIDs, []string{toID}) {
			continue
		}
		for _, witness := range relation.Witnesses {
			if witness.Detail == detail {
				return
			}
		}
	}
	t.Fatalf("alternative %s relation %q -> %q with witness %q not found; related: %#v", kind, fromID, toID, detail, related)
}

func assertAlternativeWitnessExpression(
	t *testing.T,
	index programindex.Index,
	kind programindex.RelationKind,
	fromID string,
	toID string,
	expression string,
) {
	t.Helper()
	for _, relation := range index.Relations {
		if relation.Kind != kind || relation.FromID != fromID ||
			relation.Resolution != programindex.ResolutionAlternatives ||
			!reflect.DeepEqual(relation.ToIDs, []string{toID}) {
			continue
		}
		for _, witness := range relation.Witnesses {
			if witness.SourceExpression == expression {
				return
			}
		}
	}
	t.Fatalf("alternative %s relation %q -> %q lost source expression %q", kind, fromID, toID, expression)
}

func assertExternalCallExpression(
	t *testing.T,
	index programindex.Index,
	canonical string,
	expression string,
) {
	t.Helper()
	externalID := objectNamed(t, index, programindex.ObjectExternalSymbol, canonical, "").ID
	for _, relation := range index.Relations {
		if relation.Kind != programindex.RelationInvokesExternal ||
			!reflect.DeepEqual(relation.ToIDs, []string{externalID}) {
			continue
		}
		for _, witness := range relation.Witnesses {
			if witness.SourceExpression == expression {
				return
			}
		}
	}
	t.Fatalf("external call to %q lost source expression %q", canonical, expression)
}

func unresolvedFrom(
	t *testing.T,
	index programindex.Index,
	kind programindex.RelationKind,
	fromID string,
) programindex.Relation {
	t.Helper()
	for _, relation := range index.Relations {
		if relation.Kind == kind && relation.FromID == fromID && relation.Resolution == programindex.ResolutionUnresolved && len(relation.ToIDs) == 0 {
			return relation
		}
	}
	t.Fatalf("unresolved %s relation from %q not found", kind, fromID)
	return programindex.Relation{}
}

func assertTargetCorpusRefs(t *testing.T, index programindex.Index, target pythontarget.Target) {
	t.Helper()
	pathsByRef := make(map[string]string, len(target.Modules)+len(target.Basis))
	for _, module := range target.Modules {
		pathsByRef[string(module.FileID)] = module.Path
	}
	selected := make(map[string]struct{}, len(target.SourceRefs)+len(target.Basis))
	for _, ref := range target.SourceRefs {
		selected[string(ref)] = struct{}{}
	}
	for _, basis := range target.Basis {
		pathsByRef[string(basis.FileID)] = basis.Path
		selected[string(basis.FileID)] = struct{}{}
	}
	want := make([]programindex.TargetSource, 0, len(selected))
	for ref := range selected {
		want = append(want, programindex.TargetSource{FileRef: ref, Path: pathsByRef[ref]})
	}
	sort.Slice(want, func(i, j int) bool { return want[i].FileRef < want[j].FileRef })
	if !reflect.DeepEqual(index.Target.Sources, want) {
		t.Fatalf("target sources = %v, want corpus evidence %v", index.Target.Sources, want)
	}
	if index.Target.AnchorFileRef != string(target.AnchorFileRef) {
		t.Fatalf("target anchor = %q, want corpus ref %q", index.Target.AnchorFileRef, target.AnchorFileRef)
	}
}
