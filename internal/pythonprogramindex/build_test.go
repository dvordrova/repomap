package pythonprogramindex

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/gitfiles"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/pythontarget"
)

func TestBuildKeepsCallRegistrationPathAndHandlerPattern(t *testing.T) {
	repository := pythonCorpus(t, map[string]string{
		"pyproject.toml": `[project]
name = "actual-call-route-boundary"
version = "1.0.0"
`,
		"app/__init__.py": "",
		"app/main.py": `from routerlib import Router

router = Router()

def get_items():
    return []

router.get("/api/items", get_items)
`,
	})
	target := targetOfKind(t, repository, pythontarget.KindLibrary)
	indexes, err := BuildMany(context.Background(), repository, []pythontarget.Target{target})
	if err != nil {
		t.Fatalf("BuildMany: %v", err)
	}
	if len(indexes) != 1 {
		t.Fatalf("indexes = %d", len(indexes))
	}
	handler := objectNamed(t, indexes[0], programindex.ObjectFunction, "get_items", "app/main.py")
	found := false
	for _, relation := range indexes[0].Relations {
		for _, pattern := range relation.Patterns {
			if pattern.Form != programindex.PatternCall || pattern.Selector != "get" {
				continue
			}
			pathFound, handlerFound := false, false
			for _, argument := range pattern.Arguments {
				if argument.Kind == programindex.PatternLiteralString && argument.Value == "/api/items" {
					pathFound = true
				}
				if slices.Contains(argument.ObjectIDs, handler.ID) {
					handlerFound = true
				}
			}
			if pathFound && handlerFound {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("call registration lost its neutral path or exact handler argument")
	}
}

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

func TestBuildInputSealsToTheSameIndexAsBuildMany(t *testing.T) {
	repository := pythonCorpus(t, map[string]string{
		"pyproject.toml": `[project]
name = "atomic-python-adapter"
version = "1.0.0"
`,
		"src/app/__init__.py": "",
		"src/app/main.py": `import httpx

BASE = "/api"

def run(client):
    return client.get(BASE + "/items")
`,
	})
	target := targetOfKind(t, repository, pythontarget.KindLibrary)
	input, err := BuildInput(context.Background(), repository, target)
	if err != nil {
		t.Fatalf("BuildInput: %v", err)
	}
	atomic, err := programindex.New(input)
	if err != nil {
		t.Fatalf("programindex.New(BuildInput): %v", err)
	}
	legacyBatch, err := BuildMany(context.Background(), repository, []pythontarget.Target{target})
	if err != nil {
		t.Fatalf("BuildMany: %v", err)
	}
	if len(legacyBatch) != 1 || !reflect.DeepEqual(atomic, legacyBatch[0]) {
		t.Fatalf("atomic Python adapter drifted from the established projection")
	}
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

class _HiddenWorker:
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
	hiddenWorker := objectNamed(t, index, programindex.ObjectType, "_HiddenWorker", "pkg/service.py")
	var execute, hiddenExecute programindex.Object
	for _, object := range index.Objects {
		if object.Kind != programindex.ObjectMethod || object.Name != "execute" {
			continue
		}
		switch object.OwnerID {
		case worker.ID:
			execute = object
		case hiddenWorker.ID:
			hiddenExecute = object
		}
	}
	if execute.ID == "" || hiddenExecute.ID == "" {
		t.Fatal("owned public-looking methods not found")
	}
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
	if external.External == nil || external.External.PackagePath != "json" ||
		external.External.AuthorityKind != programindex.ExternalAuthorityPlatform ||
		external.External.Receiver != "" || external.External.Name != "dumps" {
		t.Fatalf("external symbol authority = %#v, want json.dumps", external.External)
	}
	assertPythonPublicIdentity(t, run, "pkg.service.run")
	assertPythonPublicIdentity(t, external, "json.dumps")
	if len(hidden.SymbolLinkIdentities) != 0 {
		t.Fatalf("internal binding received cross-target identity: %#v", hidden.SymbolLinkIdentities)
	}
	if len(hiddenExecute.SymbolLinkIdentities) != 0 {
		t.Fatalf("member of private owner received cross-target identity: %#v", hiddenExecute.SymbolLinkIdentities)
	}
	assertAlternativeRelation(t, index, programindex.RelationInvokesExternal, run.ID, external.ID)

	invoke := objectNamed(t, index, programindex.ObjectFunction, "invoke", "pkg/service.py")
	assertUnresolvedFrom(t, index, programindex.RelationCalls, invoke.ID)
}

func TestCompileParserViewRejectsMissingOrInvalidExternalAuthorityKind(t *testing.T) {
	for _, test := range []struct {
		name     string
		external *programindex.ExternalSymbol
	}{
		{name: "missing"},
		{name: "empty", external: &programindex.ExternalSymbol{PackagePath: "httpx", Name: "get"}},
		{name: "unknown", external: &programindex.ExternalSymbol{
			AuthorityKind: "runtime", PackagePath: "httpx", Name: "get",
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := compileParserView(parserViewResult{Objects: []parsedObject{{
				SourceRef: "external:httpx.get", Kind: string(programindex.ObjectExternalSymbol),
				Name: "httpx.get", Visibility: string(programindex.VisibilityUnknown), External: test.external,
			}}}, map[string]struct{}{})
			if err == nil || !strings.Contains(err.Error(), "external authority kind") {
				t.Fatalf("compileParserView error = %v", err)
			}
		})
	}
}

func TestBuildKeepsWildcardImportModuleAsExactDependencyBoundary(t *testing.T) {
	repository := pythonCorpus(t, map[string]string{
		"pyproject.toml": `[project]
name = "wildcard-demo"
version = "1.0.0"
`,
		"pkg/__init__.py": "",
		"pkg/resources.py": `class Resource:
    pass
`,
		"pkg/main.py": `from .resources import *

def main():
    return Resource()
`,
	})
	target := targetOfKind(t, repository, pythontarget.KindLibrary)

	index, err := buildOneForTest(context.Background(), repository, target)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	mainModule := objectNamed(t, index, programindex.ObjectModule, "pkg.main", "pkg/main.py")
	resourcesModule := objectNamed(t, index, programindex.ObjectModule, "pkg.resources", "pkg/resources.py")
	resource := objectNamed(t, index, programindex.ObjectType, "Resource", "pkg/resources.py")

	found := false
	for _, relation := range index.Relations {
		if relation.Kind != programindex.RelationImports || relation.FromID != mainModule.ID {
			continue
		}
		if relation.Resolution != programindex.ResolutionExact ||
			len(relation.ToIDs) != 1 || relation.ToIDs[0] != resourcesModule.ID ||
			len(relation.Witnesses) != 1 || relation.Witnesses[0].Kind != "wildcard_import" ||
			relation.Witnesses[0].Detail != "pkg.resources" {
			t.Fatalf("wildcard import relation = %#v", relation)
		}
		found = true
	}
	if !found {
		t.Fatal("wildcard import did not retain its exact module boundary")
	}
	for _, relation := range index.Relations {
		if relation.Kind == programindex.RelationImports && relation.FromID == mainModule.ID &&
			len(relation.ToIDs) == 1 && relation.ToIDs[0] == resource.ID {
			t.Fatalf("wildcard import invented an exact imported declaration: %#v", relation)
		}
	}
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

func TestBuildKeepsNestedCallWitnessesAtExactCalleeTokens(t *testing.T) {
	repository := pythonCorpus(t, map[string]string{
		"pyproject.toml": `[project]
name = "nested-call-demo"
version = "1.0.0"
`,
		"nested_call_demo.py": `def sanitize_text(value):
    return " ".join(str(value or "").replace("\r", " ").replace("\n", " ").split())
`,
	})
	target := targetOfKind(t, repository, pythontarget.KindLibrary)
	index, err := buildOneForTest(context.Background(), repository, target)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if index.Coverage.WitnessesOmitted != 0 {
		t.Fatalf("nested call witnesses omitted = %d", index.Coverage.WitnessesOmitted)
	}

	sanitize := objectNamed(
		t, index, programindex.ObjectFunction, "sanitize_text", "nested_call_demo.py",
	)
	columnsByCallee := make(map[string][]int)
	for _, relation := range index.Relations {
		if relation.Kind != programindex.RelationCalls || relation.FromID != sanitize.ID {
			continue
		}
		if relation.WitnessesOmitted != 0 || relation.WitnessesObserved != len(relation.Witnesses) {
			t.Fatalf("nested call relation lost witness authority: %#v", relation)
		}
		for _, witness := range relation.Witnesses {
			if witness.Location == nil || witness.Location.Path != "nested_call_demo.py" ||
				witness.Location.Line != 2 {
				t.Fatalf("nested call witness has no exact source location: %#v", witness)
			}
			columnsByCallee[witness.Detail] = append(
				columnsByCallee[witness.Detail], witness.Location.Column,
			)
		}
	}
	for callee := range columnsByCallee {
		sort.Ints(columnsByCallee[callee])
	}
	want := map[string][]int{
		"join": {16}, "str": {21}, "replace": {38, 57}, "split": {76},
	}
	if !reflect.DeepEqual(columnsByCallee, want) {
		t.Fatalf("nested call callee columns = %#v, want %#v", columnsByCallee, want)
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

func TestBuildResolvesProjectRelativeSrcImportsAndRetainsNeutralDecoratorPattern(t *testing.T) {
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
	app := objectNamed(t, index, programindex.ObjectVariable, "app", "src/main.py")
	fastAPI := objectNamed(t, index, programindex.ObjectExternalSymbol, "fastapi.FastAPI", "")
	assertExactRelation(t, index, programindex.RelationImports, mainModule.ID, load.ID)
	assertAlternativeRelation(t, index, programindex.RelationCalls, healthcheck.ID, load.ID)
	assertExternalCallExpression(t, index, "fastapi.FastAPI", "FastAPI")
	for _, object := range index.Objects {
		if object.Kind == programindex.ObjectExternalSymbol && strings.HasPrefix(object.Name, "src.") {
			t.Fatalf("project-local src import became external: %#v", object)
		}
	}
	foundDecoratorPattern := false
	for _, relation := range index.Relations {
		if relation.Kind != programindex.RelationDecorates || relation.FromID != healthcheck.ID || relation.Location == nil ||
			relation.Location.Path != "src/main.py" {
			continue
		}
		for _, witness := range relation.Witnesses {
			if witness.Detail == "app.get" {
				foundDecoratorPattern = true
			}
		}
		if relation.PatternsObserved != 1 || relation.PatternsOmitted != 0 || len(relation.Patterns) != 1 {
			t.Fatalf("decorator pattern coverage = %#v", relation)
		}
		pattern := relation.Patterns[0]
		if pattern.Form != programindex.PatternDecoratorCall || pattern.Selector != "get" ||
			pattern.ReceiverID != app.ID || pattern.ReceiverOriginResolution != programindex.ResolutionAlternatives ||
			!reflect.DeepEqual(pattern.ReceiverOriginIDs, []string{fastAPI.ID}) ||
			pattern.ReceiverOriginsObserved != 1 || pattern.ReceiverOriginsOmitted != 0 {
			t.Fatalf("decorator receiver pattern = %#v", pattern)
		}
		if pattern.ArgumentsObserved != 1 || pattern.ArgumentsOmitted != 0 || len(pattern.Arguments) != 1 ||
			pattern.Arguments[0].Position != 1 || pattern.Arguments[0].Kind != programindex.PatternLiteralString ||
			pattern.Arguments[0].Value != "/healthcheck" {
			t.Fatalf("decorator argument pattern = %#v", pattern.Arguments)
		}
	}
	if !foundDecoratorPattern {
		t.Fatal("route decorator lost its neutral pattern")
	}
}

func TestBuildRetainsGenericReceiverOriginAndFStringTemplate(t *testing.T) {
	repository := pythonCorpus(t, map[string]string{
		"pyproject.toml": `[project]
name = "generic-patterns"
version = "1.0.0"
`,
		"generic/__init__.py": "",
		"generic/service.py": `import runtime
from toolkit import Maker

receiver = Maker()
segment = "anything"

@receiver.publish(f"/items/{segment}", mode=segment)
def handler():
    return None

def launch():
    runtime.start("generic.service:receiver", mode=segment)

def expand(values, options):
    runtime.start(*values, **options)
`,
	})
	target := targetOfKind(t, repository, pythontarget.KindLibrary)
	index, err := buildOneForTest(context.Background(), repository, target)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	handler := objectNamed(t, index, programindex.ObjectFunction, "handler", "generic/service.py")
	receiver := objectNamed(t, index, programindex.ObjectVariable, "receiver", "generic/service.py")
	maker := objectNamed(t, index, programindex.ObjectExternalSymbol, "toolkit.Maker", "")
	segment := objectNamed(t, index, programindex.ObjectVariable, "segment", "generic/service.py")

	var pattern programindex.RelationPattern
	for _, relation := range index.Relations {
		if relation.Kind == programindex.RelationDecorates && relation.FromID == handler.ID && len(relation.Patterns) == 1 {
			pattern = relation.Patterns[0]
			break
		}
	}
	if pattern.ID == "" {
		t.Fatal("generic decorator pattern not found")
	}
	if pattern.Selector != "publish" || pattern.ReceiverID != receiver.ID ||
		pattern.ReceiverOriginResolution != programindex.ResolutionAlternatives ||
		!reflect.DeepEqual(pattern.ReceiverOriginIDs, []string{maker.ID}) {
		t.Fatalf("generic receiver origin = %#v", pattern)
	}
	if pattern.ArgumentsObserved != 2 || pattern.ArgumentsOmitted != 0 || len(pattern.Arguments) != 2 {
		t.Fatalf("generic arguments coverage = %#v", pattern)
	}
	positional := pattern.Arguments[0]
	if positional.Position != 1 || positional.Kind != programindex.PatternStringTemplate ||
		!reflect.DeepEqual(positional.Parts, []programindex.PatternPart{
			{Kind: programindex.PatternPartLiteral, Text: "/items/"},
			{Kind: programindex.PatternPartHole},
		}) {
		t.Fatalf("f-string template = %#v", positional)
	}
	keyword := pattern.Arguments[1]
	if keyword.Keyword != "mode" || keyword.Kind != programindex.PatternDynamic ||
		keyword.Resolution != programindex.ResolutionAlternatives ||
		!reflect.DeepEqual(keyword.ObjectIDs, []string{segment.ID}) {
		t.Fatalf("keyword authority = %#v", keyword)
	}

	launch := objectNamed(t, index, programindex.ObjectFunction, "launch", "generic/service.py")
	runtimeStart := objectNamed(t, index, programindex.ObjectExternalSymbol, "runtime.start", "")
	foundCall := false
	for _, relation := range index.Relations {
		if relation.Kind != programindex.RelationInvokesExternal || relation.FromID != launch.ID ||
			len(relation.ToIDs) != 1 || relation.ToIDs[0] != runtimeStart.ID || len(relation.Patterns) != 1 {
			continue
		}
		call := relation.Patterns[0]
		if call.Form != programindex.PatternCall || call.Selector != "start" ||
			call.ArgumentsObserved != 2 || call.ArgumentsOmitted != 0 || len(call.Arguments) != 2 ||
			call.Arguments[0].Kind != programindex.PatternLiteralString ||
			call.Arguments[0].Value != "generic.service:receiver" {
			t.Fatalf("ordinary call pattern = %#v", call)
		}
		foundCall = true
	}
	if !foundCall {
		t.Fatal("ordinary external call pattern not found")
	}

	expand := objectNamed(t, index, programindex.ObjectFunction, "expand", "generic/service.py")
	foundOmission := false
	for _, relation := range index.Relations {
		if relation.Kind != programindex.RelationInvokesExternal || relation.FromID != expand.ID ||
			len(relation.Patterns) != 1 {
			continue
		}
		call := relation.Patterns[0]
		if call.ArgumentsObserved != 2 || call.ArgumentsOmitted != 2 || len(call.Arguments) != 0 {
			t.Fatalf("expanded argument frontier = %#v", call)
		}
		foundOmission = true
	}
	if !foundOmission {
		t.Fatal("expanded argument frontier not found")
	}
}

func TestBuildRetainsInitializerValueCandidatesAndFailsClosedAfterReassignment(t *testing.T) {
	repository := pythonCorpus(t, map[string]string{
		"pyproject.toml": `[project]
name = "initializer-values"
version = "1.0.0"
`,
		"values/__init__.py": "",
		"values/routes.py": `from routing import Router

router = Router()

literal_path = "/api/dynamic"
@router.get(literal_path)
def literal_handler():
    return None

segment = "items"
template_path = f"/api/{segment}"
@router.get(template_path)
def template_handler():
    return None

reassigned_path = "/api/before"
reassigned_path = "/api/after"
@router.get(reassigned_path)
def reassigned_handler():
    return None
`,
	})
	target := targetOfKind(t, repository, pythontarget.KindLibrary)
	index, err := buildOneForTest(context.Background(), repository, target)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	argumentForHandler := func(name string) programindex.PatternArgument {
		t.Helper()
		handler := objectNamed(t, index, programindex.ObjectFunction, name, "values/routes.py")
		for _, relation := range index.Relations {
			if relation.Kind == programindex.RelationDecorates && relation.FromID == handler.ID &&
				len(relation.Patterns) == 1 && len(relation.Patterns[0].Arguments) == 1 {
				return relation.Patterns[0].Arguments[0]
			}
		}
		t.Fatalf("decorator argument for %q not found", name)
		return programindex.PatternArgument{}
	}
	assertSource := func(argument programindex.PatternArgument, wantLine int) {
		t.Helper()
		if len(argument.ObjectIDs) != 1 || len(argument.ValueCandidates) != 1 ||
			!reflect.DeepEqual(argument.ValueCandidates[0].SourceObjectIDs, argument.ObjectIDs) {
			t.Fatalf("initializer source binding = %#v", argument)
		}
		source := objectByID(t, index, argument.ValueCandidates[0].SourceObjectIDs[0])
		if source.Kind != programindex.ObjectVariable || source.Location == nil ||
			source.Location.Path != "values/routes.py" || source.Location.Line != wantLine {
			t.Fatalf("initializer source object = %#v, want line %d", source, wantLine)
		}
	}

	literal := argumentForHandler("literal_handler")
	if literal.Kind != programindex.PatternDynamic ||
		literal.Resolution != programindex.ResolutionAlternatives ||
		literal.ValueCandidatesObserved != 1 || literal.ValueCandidatesOmitted != 0 ||
		len(literal.ValueCandidates) != 1 ||
		literal.ValueCandidates[0].Kind != programindex.PatternLiteralString ||
		literal.ValueCandidates[0].Value != "/api/dynamic" ||
		literal.ValueCandidates[0].Resolution != programindex.PatternValuePossible ||
		literal.ValueCandidates[0].SourceKind != programindex.PatternValueSourceInitializer {
		t.Fatalf("literal initializer candidate = %#v", literal)
	}
	assertSource(literal, 5)

	template := argumentForHandler("template_handler")
	if template.Kind != programindex.PatternDynamic || template.ValueCandidatesObserved != 1 ||
		len(template.ValueCandidates) != 1 ||
		template.ValueCandidates[0].Kind != programindex.PatternStringTemplate ||
		template.ValueCandidates[0].Resolution != programindex.PatternValuePossible ||
		!reflect.DeepEqual(template.ValueCandidates[0].Parts, []programindex.PatternPart{
			{Kind: programindex.PatternPartLiteral, Text: "/api/"},
			{Kind: programindex.PatternPartHole},
		}) {
		t.Fatalf("template initializer candidate = %#v", template)
	}
	assertSource(template, 11)

	reassigned := argumentForHandler("reassigned_handler")
	if reassigned.Kind != programindex.PatternDynamic || reassigned.ValueCandidatesObserved != 0 ||
		reassigned.ValueCandidatesOmitted != 0 || len(reassigned.ValueCandidates) != 0 ||
		len(reassigned.ObjectIDs) != 1 {
		t.Fatalf("reassigned initializer did not fail closed: %#v", reassigned)
	}
	reassignedSource := objectByID(t, index, reassigned.ObjectIDs[0])
	if reassignedSource.Location == nil || reassignedSource.Location.Line != 17 {
		t.Fatalf("reassigned argument source = %#v, want latest structural binding on line 17", reassignedSource)
	}
}

func TestBuildRetainsNeutralCallPatternsPastFormerLocalThresholds(t *testing.T) {
	const (
		formerArgumentLimit = 128
		formerPartLimit     = 64
		formerTextLimit     = 16 * 1024
	)
	arguments := make([]string, formerArgumentLimit+1)
	for position := range arguments {
		arguments[position] = fmt.Sprintf("%q", fmt.Sprintf("arg-%d", position))
	}
	var template strings.Builder
	for position := 0; position < formerPartLimit+1; position++ {
		_, _ = fmt.Fprintf(&template, "/{value%d}", position)
	}
	longLiteral := strings.Repeat("x", formerTextLimit+1)
	longSelector := strings.Repeat("s", formerTextLimit+1)

	var source strings.Builder
	source.WriteString("import runtime\n\n")
	for position := 0; position < formerPartLimit+1; position++ {
		_, _ = fmt.Fprintf(&source, "value%d = %d\n", position, position)
	}
	source.WriteString("\ndef emit():\n")
	_, _ = fmt.Fprintf(&source, "    runtime.many(%s)\n", strings.Join(arguments, ", "))
	_, _ = fmt.Fprintf(&source, "    runtime.template(f%q)\n", template.String())
	_, _ = fmt.Fprintf(&source, "    runtime.literal(%q)\n", longLiteral)
	_, _ = fmt.Fprintf(&source, "    runtime.%s(\"/computed\")\n", longSelector)

	repository := pythonCorpus(t, map[string]string{
		"pyproject.toml": `[project]
name = "complete-patterns"
version = "1.0.0"
`,
		"complete/__init__.py": "",
		"complete/source.py":   source.String(),
	})
	target := targetOfKind(t, repository, pythontarget.KindLibrary)
	index, err := buildOneForTest(context.Background(), repository, target)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	emit := objectNamed(t, index, programindex.ObjectFunction, "emit", "complete/source.py")
	patterns := make(map[string]programindex.RelationPattern)
	for _, relation := range index.Relations {
		if relation.Kind != programindex.RelationInvokesExternal || relation.FromID != emit.ID ||
			len(relation.Patterns) != 1 {
			continue
		}
		pattern := relation.Patterns[0]
		if relation.PatternsObserved != 1 || relation.PatternsOmitted != 0 ||
			pattern.ArgumentsOmitted != 0 {
			t.Fatalf("pattern was locally truncated: %#v", relation)
		}
		patterns[pattern.Selector] = pattern
	}
	if many := patterns["many"]; many.ID == "" || many.ArgumentsObserved != formerArgumentLimit+1 ||
		len(many.Arguments) != formerArgumentLimit+1 {
		t.Fatalf("complete positional arguments = %#v", many)
	}
	if value := patterns["template"]; value.ID == "" || value.ArgumentsObserved != 1 ||
		len(value.Arguments) != 1 || value.Arguments[0].Kind != programindex.PatternStringTemplate ||
		len(value.Arguments[0].Parts) != 2*formerPartLimit+2 {
		t.Fatalf("complete template parts = %#v", value)
	}
	if value := patterns["literal"]; value.ID == "" || len(value.Arguments) != 1 ||
		value.Arguments[0].Kind != programindex.PatternLiteralString ||
		value.Arguments[0].Value != longLiteral {
		t.Fatalf("complete literal = %#v", value)
	}
	if value := patterns[longSelector]; value.ID == "" || value.Selector != longSelector {
		t.Fatalf("complete selector = %#v", value)
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

func TestBuildOmitsSignatureLiteralsButRetainsDecoratorArguments(t *testing.T) {
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
	for _, literal := range []string{"PRIVATE_ROUTE_LITERAL", "DECORATOR_TOKEN_LITERAL"} {
		if !strings.Contains(string(wire), literal) {
			t.Fatalf("neutral decorator pattern lost literal argument %q", literal)
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

func TestSelectedTargetReadsCompleteModulePastFormerLocalLimit(t *testing.T) {
	const formerModuleLimit = 2 << 20
	largeSource := "# " + strings.Repeat("x", formerModuleLimit+1) + "\n"
	repository := pythonCorpus(t, map[string]string{
		"pyproject.toml": `[project]
name = "large-module"
version = "1.0.0"
`,
		"large_module/__init__.py": largeSource,
	})
	catalog, err := pythontarget.DiscoverWithOptions(context.Background(), repository, pythontarget.Options{
		MaxFileBytes:  corpus.MaxReadBytes,
		MaxTotalBytes: corpus.MaxReadBytes,
	})
	if err != nil {
		t.Fatalf("DiscoverWithOptions: %v", err)
	}
	var target pythontarget.Target
	for _, candidate := range catalog.Entries {
		if candidate.Kind == pythontarget.KindLibrary {
			target = candidate
			break
		}
	}
	if target.Ref == "" {
		t.Fatal("large library target not discovered")
	}

	sentinel := errors.New("request inspected")
	_, err = buildMany(context.Background(), repository, []pythontarget.Target{target}, func(_ context.Context, request parserRequest) (parserResponse, error) {
		for _, source := range request.Sources {
			if source.Path != "large_module/__init__.py" {
				continue
			}
			decoded, decodeErr := base64.StdEncoding.DecodeString(source.Content)
			if decodeErr != nil {
				t.Fatalf("DecodeString: %v", decodeErr)
			}
			if string(decoded) != largeSource {
				t.Fatalf("parser source bytes = %d, want complete %d", len(decoded), len(largeSource))
			}
			return parserResponse{}, sentinel
		}
		t.Fatal("large source was not sent to parser")
		return parserResponse{}, sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("buildMany error = %v, want inspection sentinel", err)
	}
}

func TestParserTransportAndStructuralCollectionsHaveNoLocalCaps(t *testing.T) {
	for _, forbidden := range []string{
		"MAX_OBJECTS", "MAX_RELATIONS", "object limit exceeded", "relation limit exceeded",
	} {
		if strings.Contains(parserHelper, forbidden) {
			t.Fatalf("embedded parser still contains local structural cap %q", forbidden)
		}
	}

	const formerInjectedThreshold = 4
	encoded := append(
		bytes.Repeat([]byte(" "), formerInjectedThreshold+1),
		[]byte(`{"python_version":"3.14.0","views":[{"objects":[],"relations":[]}]}`)...,
	)
	response, err := decodeParserResponse(encoded)
	if err != nil {
		t.Fatalf("decodeParserResponse past former injected threshold: %v", err)
	}
	if len(response.Views) != 1 {
		t.Fatalf("parser views = %d, want 1", len(response.Views))
	}

	var source strings.Builder
	for index := 0; index < formerInjectedThreshold+2; index++ {
		if index+1 < formerInjectedThreshold+2 {
			_, _ = fmt.Fprintf(&source, "def f%d():\n    return f%d()\n\n", index, index+1)
		} else {
			_, _ = fmt.Fprintf(&source, "def f%d():\n    return %d\n", index, index)
		}
	}
	parsed, err := runParser(context.Background(), parserRequest{
		Sources: []parserSource{{
			Path: "many.py", Content: base64.StdEncoding.EncodeToString([]byte(source.String())),
		}},
		Views: []parserView{{
			Files: []parserFile{{
				SourceRef: "module:many", Path: "many.py", Name: "many",
			}},
			Packages: []parserPackage{},
		}},
	})
	if err != nil {
		t.Fatalf("runParser generated collection: %v", err)
	}
	if len(parsed.Views) != 1 ||
		len(parsed.Views[0].Objects) <= formerInjectedThreshold ||
		len(parsed.Views[0].Relations) <= formerInjectedThreshold {
		t.Fatalf(
			"generated parser collections were not retained beyond injected threshold %d: %#v",
			formerInjectedThreshold, parsed.Views,
		)
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

func assertPythonPublicIdentity(t *testing.T, object programindex.Object, display string) {
	t.Helper()
	for _, identity := range object.SymbolLinkIdentities {
		if identity.Domain == "python_public_symbol_v1" && identity.Display == display &&
			strings.HasPrefix(identity.Key, "symbol-link-") {
			return
		}
	}
	t.Fatalf("object %q lacks Python public identity %q: %#v", object.Name, display, object.SymbolLinkIdentities)
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
