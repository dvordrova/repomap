package jstsproject

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/gitfiles"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/programindex/adaptertest"
)

func TestHelperUsesPreparedLocalCompilerAndBindsSourceBytes(t *testing.T) {
	root := preparedTempProject(t)
	tracked := []string{
		"package.json", "postcss.config.js", "src/excluded.ts", "src/main.ts",
		"src/collisions.ts", "src/lookalike.js", "src/one.ts", "src/two.ts", "src/view.tsx", "src/widget.jsx", "tsconfig.json",
	}
	listing := gitfiles.Listing{Paths: tracked, RegularPaths: tracked}
	firstCorpus, err := corpus.New(context.Background(), root, listing)
	if err != nil {
		t.Fatal(err)
	}
	first, firstIndex, firstCatalog, err := Build(context.Background(), firstCorpus, root)
	firstCorpus.Close()
	if err != nil {
		t.Fatal(err)
	}
	if first.Project.ModuleResolution != "bundler" || first.Project.Language != "typescript" {
		t.Fatalf("project = %#v", first.Project)
	}
	wantFiles := []string{"postcss.config.js", "src/collisions.ts", "src/lookalike.js", "src/main.ts", "src/one.ts", "src/two.ts", "src/view.tsx", "src/widget.jsx"}
	gotFiles := make([]string, len(first.Files))
	for index, file := range first.Files {
		gotFiles[index] = file.Path
	}
	if strings.Join(gotFiles, "\n") != strings.Join(wantFiles, "\n") {
		t.Fatalf("project-selected deterministic files = %#v, want %#v", gotFiles, wantFiles)
	}
	if !hasExactImport(first, "@/one", "src/one.ts") || !hasExactImport(first, "@/two", "src/two.ts") {
		t.Fatalf("path-alias imports were not resolved exactly: %#v", first.Imports)
	}
	qualifiedSame := map[string]bool{}
	for _, declaration := range first.Declarations {
		if declaration.Name == "same" {
			qualifiedSame[declaration.QualifiedName] = true
		}
	}
	if !qualifiedSame["src/one#same"] || !qualifiedSame["src/two#same"] || len(qualifiedSame) != 2 {
		t.Fatalf("same-name qualified declarations were merged: %#v", qualifiedSame)
	}
	if len(first.Surfaces) != 0 {
		t.Fatalf("framework-name lookalikes gained product-surface authority: %#v", first.Surfaces)
	}
	foundExactTS := false
	foundCompleteExpression := false
	for _, call := range first.Calls {
		if call.Location.Path == "src/main.ts" && call.Resolution == "exact" {
			foundExactTS = true
		}
		if strings.HasPrefix(call.Expression, `["abc",`) {
			foundCompleteExpression = true
			if len(call.Expression) <= 512 || call.Expression != strings.TrimSpace(call.Expression) ||
				!strings.HasSuffix(call.Expression, `"end"].join`) {
				t.Fatalf("complete call expression was clipped or malformed: %q", call.Expression)
			}
		}
	}
	if !foundExactTS {
		t.Fatalf("TypeScript calls = %#v", first.Calls)
	}
	if !foundCompleteExpression {
		t.Fatalf("long call expression was not retained: %#v", first.Calls)
	}
	assertReceiverlessObjectMethodProjection(t, first, firstIndex)
	assertEveryMethodHasExactTypeReceiver(t, firstIndex)
	assertCompilerResolvedInvocationProjection(t, first, firstIndex)
	for _, dependency := range firstCatalog.Dependencies {
		if dependency.PackagePath == javascriptPlatform {
			t.Fatalf("JavaScript platform authority leaked into dependency catalog: %#v", dependency)
		}
	}
	foundUnresolvedDynamic := false
	foundUnresolvedPropertyNameCollision := false
	for _, call := range first.Calls {
		if call.Location.Path == "src/lookalike.js" && call.Resolution == "unresolved" {
			foundUnresolvedDynamic = true
		}
		if call.Location.Path == "src/lookalike.js" && strings.Contains(call.Expression, ".test") {
			if call.Resolution != "unresolved" || len(call.CalleeRefs) != 0 {
				t.Fatalf("property-name collision invented call targets: %#v", call)
			}
			foundUnresolvedPropertyNameCollision = true
		}
		if strings.HasSuffix(call.Location.Path, ".js") || strings.HasSuffix(call.Location.Path, ".jsx") {
			if call.Resolution == "exact" {
				t.Fatalf("JavaScript-origin call retained exact authority: %#v", call)
			}
		}
	}
	if !foundUnresolvedDynamic {
		t.Fatalf("dynamic JavaScript call frontier was not retained: %#v", first.Calls)
	}
	if !foundUnresolvedPropertyNameCollision {
		t.Fatalf("property-name collision frontier was not retained: %#v", first.Calls)
	}
	for _, relation := range firstIndex.Relations {
		if relation.Location != nil && relation.Location.Path == "postcss.config.js" && relation.Kind == "calls" {
			if relation.Resolution != "alternatives" || len(relation.Witnesses) != 1 || relation.Witnesses[0].Kind != "javascript_call_candidate" {
				t.Fatalf("JavaScript ProgramIndex authority = %#v", relation)
			}
		}
	}

	writeTestFile(t, root, "src/main.ts", "export function hello(): string { return \"changed\" }\nhello()\n")
	secondCorpus, err := corpus.New(context.Background(), root, listing)
	if err != nil {
		t.Fatal(err)
	}
	second, secondIndex, _, err := Build(context.Background(), secondCorpus, root)
	secondCorpus.Close()
	if err != nil {
		t.Fatal(err)
	}
	if first.CorpusSHA256 != second.CorpusSHA256 {
		t.Fatal("path-only corpus identity unexpectedly changed")
	}
	if first.SourceSHA256 == second.SourceSHA256 || first.SHA256 == second.SHA256 || firstIndex.SourceSHA256 == secondIndex.SourceSHA256 {
		t.Fatal("source-byte change did not alter adapter/index identity")
	}

	writeTestFile(t, root, "tsconfig.json", `{"include":["src/**/*"],"exclude":["src/excluded.ts"],"compilerOptions":{"allowJs":true,"module":"ESNext","moduleResolution":"bundler","jsx":"preserve","baseUrl":".","paths":{"@/*":["./src/*"],"#/*":["./src/*"]},"strict":true}}`)
	thirdCorpus, err := corpus.New(context.Background(), root, listing)
	if err != nil {
		t.Fatal(err)
	}
	third, thirdIndex, _, err := Build(context.Background(), thirdCorpus, root)
	thirdCorpus.Close()
	if err != nil {
		t.Fatal(err)
	}
	if second.SourceSHA256 != third.SourceSHA256 {
		t.Fatal("configuration-only change unexpectedly altered selected source-byte identity")
	}
	if second.SHA256 == third.SHA256 || secondIndex.ScenarioSHA256 == thirdIndex.ScenarioSHA256 {
		t.Fatal("material project-configuration change did not alter semantic scenario identity")
	}
}

func TestHelperRetainsNeutralCallPatternsPastFormerLocalThresholds(t *testing.T) {
	const (
		formerArgumentLimit = 128
		formerObjectLimit   = 64
		formerTextLimit     = 16 * 1024
	)
	root := preparedTempProject(t)
	var source strings.Builder
	source.WriteString("const transport: Record<string, (...args: unknown[]) => void> = {}\n")
	for index := 0; index < formerObjectLimit+1; index++ {
		_, _ = fmt.Fprintf(&source, "const value%d = String(%d)\n", index, index)
	}
	arguments := make([]string, formerArgumentLimit+1)
	for index := range arguments {
		arguments[index] = fmt.Sprintf("\"arg-%d\"", index)
	}
	_, _ = fmt.Fprintf(&source, "transport.many(%s)\n", strings.Join(arguments, ","))
	source.WriteString("transport.template(`")
	for index := 0; index < formerObjectLimit+1; index++ {
		_, _ = fmt.Fprintf(&source, "/${value%d}", index)
	}
	source.WriteString("`)\n")
	longLiteral := strings.Repeat("x", formerTextLimit+1)
	longSelector := strings.Repeat("s", formerTextLimit+1)
	_, _ = fmt.Fprintf(&source, "transport.literal(%q)\n", longLiteral)
	_, _ = fmt.Fprintf(&source, "transport[%q](\"/computed\")\n", longSelector)
	writeTestFile(t, root, "src/bounds.ts", source.String())

	tracked := []string{"package.json", "src/bounds.ts", "tsconfig.json"}
	repository, err := corpus.New(
		context.Background(), root,
		gitfiles.Listing{Paths: tracked, RegularPaths: tracked},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	result, index, _, err := Build(context.Background(), repository, root)
	if err != nil {
		t.Fatal(err)
	}

	calls := make(map[string]Call)
	for _, call := range result.Calls {
		calls[call.Expression] = call
	}
	many := calls["transport.many"]
	if many.Pattern == nil || many.Pattern.ArgumentsObserved != formerArgumentLimit+1 ||
		len(many.Pattern.Arguments) != formerArgumentLimit+1 {
		t.Fatalf("complete arguments = %#v", many.Pattern)
	}
	template := calls["transport.template"]
	if template.Pattern == nil || len(template.Pattern.Arguments) != 1 ||
		template.Pattern.Arguments[0].Kind != "string_template" ||
		template.Pattern.Arguments[0].ObjectsObserved != formerObjectLimit+1 ||
		len(template.Pattern.Arguments[0].ObjectRefs) != formerObjectLimit+1 ||
		len(template.Pattern.Arguments[0].Parts) != 2*formerObjectLimit+2 {
		t.Fatalf("complete template/object refs = %#v", template.Pattern)
	}
	literal := calls["transport.literal"]
	if literal.Pattern == nil || len(literal.Pattern.Arguments) != 1 ||
		literal.Pattern.Arguments[0].Kind != "literal_string" || literal.Pattern.Arguments[0].Value != longLiteral {
		t.Fatalf("complete literal = %#v", literal.Pattern)
	}
	foundLongSelector := false
	for _, call := range result.Calls {
		if strings.HasPrefix(call.Expression, "transport[") && call.PatternsObserved == 1 && call.Pattern != nil &&
			call.Pattern.Selector == longSelector {
			foundLongSelector = true
		}
	}
	if !foundLongSelector {
		t.Fatal("long selector was clipped or omitted")
	}

	for _, relation := range index.Relations {
		if relation.SourceRef == "program:"+many.Ref {
			if len(relation.Patterns) != 1 || relation.Patterns[0].ArgumentsObserved != formerArgumentLimit+1 ||
				len(relation.Patterns[0].Arguments) != formerArgumentLimit+1 || relation.Patterns[0].ArgumentsOmitted != 0 {
				t.Fatalf("complete ProgramIndex arguments = %#v", relation)
			}
		}
		if relation.SourceRef == "program:"+template.Ref {
			argument := relation.Patterns[0].Arguments[0]
			if argument.Kind != programindex.PatternStringTemplate || argument.ObjectsObserved != formerObjectLimit+1 ||
				len(argument.ObjectIDs) != formerObjectLimit+1 || argument.ObjectsOmitted != 0 ||
				len(argument.Parts) != 2*formerObjectLimit+2 {
				t.Fatalf("complete ProgramIndex object refs = %#v", relation)
			}
		}
	}
}

func TestCumulativeJSTSActualToFormalValueProvenance(t *testing.T) {
	root := preparedCompilerProject(t)
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve JSTS contract-test source path")
	}
	fixtureRoot := filepath.Join(filepath.Dir(filename), "..", "..", "testdata", "repositories", "jsts")
	tracked := []string{"package.json", "shared/contracts.ts", "src/ambiguity.tsx", "src/platform.ts", "src/server.ts", "tsconfig.json"}
	for _, relative := range tracked {
		contents, err := os.ReadFile(filepath.Join(fixtureRoot, filepath.FromSlash(relative)))
		if err != nil {
			t.Fatalf("read cumulative JSTS fixture %s: %v", relative, err)
		}
		writeTestFile(t, root, relative, string(contents))
	}
	materializeCumulativeJSTSDependencyTypes(t, root)
	repository, err := corpus.New(
		context.Background(), root,
		gitfiles.Listing{Paths: tracked, RegularPaths: tracked},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	result, index, _, err := Build(context.Background(), repository, root)
	if err != nil {
		t.Fatal(err)
	}
	assertCumulativeJSTSActualToFormalValueProvenance(t, result, index)
}

func TestCumulativeJSTSRepositoryCompilerAndProgramIndexContract(t *testing.T) {
	root := preparedCompilerProject(t)
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve JSTS contract-test source path")
	}
	fixtureRoot := filepath.Join(filepath.Dir(filename), "..", "..", "testdata", "repositories", "jsts")
	tracked := []string{"package.json", "shared/contracts.ts", "src/ambiguity.tsx", "src/platform.ts", "src/server.ts", "tsconfig.json"}
	for _, relative := range tracked {
		contents, err := os.ReadFile(filepath.Join(fixtureRoot, filepath.FromSlash(relative)))
		if err != nil {
			t.Fatalf("read cumulative JSTS fixture %s: %v", relative, err)
		}
		writeTestFile(t, root, relative, string(contents))
	}
	materializeCumulativeJSTSDependencyTypes(t, root)
	repository, err := corpus.New(
		context.Background(), root,
		gitfiles.Listing{Paths: tracked, RegularPaths: tracked},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()

	result, index, catalog, err := Build(context.Background(), repository, root)
	if err != nil {
		t.Fatal(err)
	}
	if result.Version != Version || result.HelperVersion != HelperVersion ||
		result.Project.Language != "typescript" || result.Project.Name != "repomap-cumulative-jsts-fixture" {
		t.Fatalf("cumulative JSTS producer identity = %#v", result.Project)
	}
	if err := ValidateProgramIndex(result, index); err != nil {
		t.Fatalf("validate cumulative JSTS ProgramIndex: %v", err)
	}
	encoded, err := programindex.Encode(index)
	if err != nil {
		t.Fatalf("encode cumulative JSTS ProgramIndex: %v", err)
	}
	if _, err := programindex.Decode(encoded); err != nil {
		t.Fatalf("round-trip cumulative JSTS ProgramIndex: %v", err)
	}

	calls := make(map[string]Call, len(result.Calls))
	for _, call := range result.Calls {
		calls[call.Expression] = call
	}
	for expression, externalName := range map[string]string{
		"new Date": "Date", "new Image": "Image", "new Promise": "Promise",
	} {
		call, exists := calls[expression]
		if !exists || call.Invocation != "construct" || call.Resolution != "exact" ||
			call.ExternalPackage != javascriptPlatform || call.ExternalName != externalName ||
			len(call.CalleeRefs) != 0 {
			t.Fatalf("cumulative platform constructor %q = %#v", expression, call)
		}
	}
	for expression, target := range map[string]struct {
		receiver string
		name     string
	}{
		"this.context.beginPath": {receiver: "CanvasRenderingContext2D", name: "beginPath"},
		"this.context.moveTo":    {receiver: "CanvasRenderingContext2D", name: "moveTo"},
		"this.context.lineTo":    {receiver: "CanvasRenderingContext2D", name: "lineTo"},
		"this.context.stroke":    {receiver: "CanvasRenderingContext2D", name: "stroke"},
		"canvas.getContext":      {receiver: "HTMLCanvasElement", name: "getContext"},
		"Math.min":               {receiver: "Math", name: "min"},
		"console.log":            {receiver: "Console", name: "log"},
	} {
		call, exists := calls[expression]
		if !exists || call.Invocation != "call" || call.Resolution != "exact" ||
			call.ExternalPackage != javascriptPlatform || call.ExternalReceiver != target.receiver ||
			call.ExternalName != target.name || len(call.CalleeRefs) != 0 {
			t.Fatalf("cumulative platform call %q = %#v, want %#v", expression, call, target)
		}
	}
	localConstructor := calls["new LevelDrawer"]
	if localConstructor.Invocation != "construct" || localConstructor.Resolution != "exact" ||
		localConstructor.ExternalPackage != "" || len(localConstructor.CalleeRefs) != 1 {
		t.Fatalf("cumulative local constructor = %#v", localConstructor)
	}
	localTyped := calls["new localDateConstructor"]
	if localTyped.Invocation != "construct" || localTyped.Resolution != "unresolved" ||
		localTyped.ExternalPackage != "" || len(localTyped.CalleeRefs) != 0 {
		t.Fatalf("cumulative local typed-constructor frontier = %#v", localTyped)
	}
	stateSetter := calls["setSliderValue"]
	if stateSetter.Invocation != "call" || stateSetter.Resolution != "unresolved" ||
		stateSetter.ExternalPackage != "" || len(stateSetter.CalleeRefs) != 0 {
		t.Fatalf("destructured state setter gained enclosing-function call authority: %#v", stateSetter)
	}
	animate := calls["animate"]
	if animate.Invocation != "call" || animate.Resolution != "exact" ||
		animate.ExternalPackage != "" || len(animate.CalleeRefs) != 1 {
		t.Fatalf("direct local function lost exact call authority: %#v", animate)
	}
	var literalPatternCall, templatePatternCall, dynamicPatternCall Call
	for _, call := range result.Calls {
		if call.Pattern == nil {
			continue
		}
		switch {
		case call.Expression == "neutralTransport.get" && len(call.Pattern.Arguments) == 1 &&
			call.Pattern.Arguments[0].Kind == "literal_string":
			literalPatternCall = call
		case call.Expression == "neutralTransport.get" && len(call.Pattern.Arguments) == 1 &&
			call.Pattern.Arguments[0].Kind == "string_template":
			templatePatternCall = call
		case call.Expression == "neutralTransport.post":
			dynamicPatternCall = call
		}
	}
	if literalPatternCall.Pattern == nil || literalPatternCall.Pattern.Selector != "get" ||
		literalPatternCall.Pattern.ReceiverRef == "" || literalPatternCall.Pattern.ArgumentsObserved != 1 ||
		literalPatternCall.Pattern.Arguments[0].Value != "/api/levels" {
		t.Fatalf("neutral literal call pattern = %#v", literalPatternCall)
	}
	if templatePatternCall.Pattern == nil || templatePatternCall.Pattern.Selector != "get" ||
		templatePatternCall.Pattern.ArgumentsObserved != 1 ||
		len(templatePatternCall.Pattern.Arguments[0].Parts) != 2 ||
		templatePatternCall.Pattern.Arguments[0].Parts[0] != (CallPatternPart{Kind: "literal", Text: "/api/level/"}) ||
		templatePatternCall.Pattern.Arguments[0].Parts[1] != (CallPatternPart{Kind: "hole"}) {
		t.Fatalf("neutral template call pattern = %#v", templatePatternCall)
	}
	if dynamicPatternCall.Pattern == nil || dynamicPatternCall.Pattern.Selector != "post" ||
		dynamicPatternCall.Pattern.ArgumentsObserved != 2 || len(dynamicPatternCall.Pattern.Arguments) != 2 ||
		dynamicPatternCall.Pattern.Arguments[0].Kind != "literal_string" ||
		dynamicPatternCall.Pattern.Arguments[0].Value != "/api/level/run" ||
		dynamicPatternCall.Pattern.Arguments[1].Kind != "dynamic" ||
		dynamicPatternCall.Pattern.Arguments[1].Value != "" {
		t.Fatalf("neutral dynamic call pattern = %#v", dynamicPatternCall)
	}
	computedPatternCall := calls["dynamicTransport[levelId]"]
	if computedPatternCall.Ref == "" || computedPatternCall.PatternsObserved != 1 || computedPatternCall.Pattern != nil {
		t.Fatalf("computed selector invented pattern authority: %#v", computedPatternCall)
	}
	declarationRefs := make(map[string]string)
	declarationRefsByName := make(map[string][]string)
	declarationKindByRef := make(map[string]string)
	for _, declaration := range result.Declarations {
		declarationKindByRef[declaration.Ref] = declaration.Kind
		declarationRefsByName[declaration.Location.Path+"\x00"+declaration.Name] = append(
			declarationRefsByName[declaration.Location.Path+"\x00"+declaration.Name],
			declaration.Ref,
		)
		if declaration.Location.Path == "src/server.ts" {
			declarationRefs[declaration.Name] = declaration.Ref
		}
	}
	refsForName := func(filePath, name string) []string {
		t.Helper()
		refs := append([]string(nil), declarationRefsByName[filePath+"\x00"+name]...)
		sort.Strings(refs)
		return refs
	}
	ambiguityCallerRefs := refsForName("src/ambiguity.tsx", "registerMixedOrderCallback")
	ambiguousRouteCallerRefs := refsForName("src/ambiguity.tsx", "registerAmbiguousExpress")
	mixedCallbackRefs := refsForName("src/ambiguity.tsx", "mixedOrderCallback")
	requestMiddlewareRefs := refsForName("src/ambiguity.tsx", "requestMiddleware")
	ambiguousHandlerRefs := refsForName("src/ambiguity.tsx", "ambiguousHandler")
	mixedQueryOwnerRefs := refsForName("src/ambiguity.tsx", "mixedQueryKey")
	fetchMethodOwnerRefs := refsForName("src/ambiguity.tsx", "fetchMethodAuthority")
	if len(ambiguityCallerRefs) != 1 || len(ambiguousRouteCallerRefs) != 1 || len(mixedCallbackRefs) != 2 ||
		len(requestMiddlewareRefs) != 1 || len(ambiguousHandlerRefs) < 2 ||
		len(mixedQueryOwnerRefs) != 1 || len(fetchMethodOwnerRefs) != 1 {
		t.Fatalf("cumulative alternative declarations: consumer=%#v route=%#v mixed=%#v middleware=%#v handler=%#v",
			ambiguityCallerRefs, ambiguousRouteCallerRefs, mixedCallbackRefs, requestMiddlewareRefs, ambiguousHandlerRefs)
	}
	sharedContractRefs := []string{}
	for _, contract := range result.Contracts {
		if contract.Kind == "shared_type" || strings.HasPrefix(contract.Location.Path, "shared/") {
			sharedContractRefs = append(sharedContractRefs, contract.Ref)
		}
	}
	sort.Strings(sharedContractRefs)
	var sharedContractSurface Surface
	for _, surface := range result.Surfaces {
		if surface.Ref == "surface:shared-contracts" {
			sharedContractSurface = surface
			break
		}
	}
	if sharedContractSurface.Ref == "" || sharedContractSurface.Location.Path != "shared/contracts.ts" ||
		strings.Join(sharedContractSurface.EvidenceRefs, "\x00") != strings.Join(sharedContractRefs, "\x00") {
		t.Fatalf("shared-contract predicate drifted: surface=%#v contracts=%#v", sharedContractSurface, sharedContractRefs)
	}
	var mixedQueryCall Call
	for _, call := range result.Calls {
		if call.CallerRef == mixedQueryOwnerRefs[0] && call.Expression == "useQuery" {
			mixedQueryCall = call
			break
		}
	}
	if mixedQueryCall.Ref == "" || mixedQueryCall.ExternalPackage != "@tanstack/react-query" || mixedQueryCall.Resolution != "exact" {
		t.Fatalf("mixed query-key call lacks exact imported call authority: %#v", mixedQueryCall)
	}
	for _, contract := range result.Contracts {
		if contract.Kind == "query_key" && contract.Location.Path == "src/ambiguity.tsx" {
			t.Fatalf("mixed static/dynamic query key became a partial exact contract: %#v", contract)
		}
	}
	for _, name := range []string{
		"loadFeaturedProduct", "getFeaturedProduct", "recordOrder", "handleOrder", "startServer",
		"registerOrderConsumer", "registerRuntimeOrderHandler", "registerDirectOrderConsumer",
	} {
		if declarationRefs[name] == "" {
			t.Fatalf("cumulative callback declaration %q missing: %#v", name, result.Declarations)
		}
	}
	findPatternCall := func(callerRef, expression, firstLiteral string) Call {
		t.Helper()
		for _, call := range result.Calls {
			if call.CallerRef != callerRef || call.Expression != expression || call.Pattern == nil || len(call.Pattern.Arguments) == 0 {
				continue
			}
			first := call.Pattern.Arguments[0]
			if firstLiteral == "" || first.Kind == "literal_string" && first.Value == firstLiteral {
				return call
			}
		}
		return Call{}
	}
	assertPatternCallback := func(call Call, originRef, callbackRef string) {
		t.Helper()
		if call.Ref == "" || call.Pattern == nil || call.Pattern.ReceiverRef == "" ||
			call.Pattern.ReceiverOriginResolution != "exact" || call.Pattern.ReceiverOriginsObserved != 1 ||
			len(call.Pattern.ReceiverOriginRefs) != 1 || call.Pattern.ReceiverOriginRefs[0] != originRef ||
			len(call.Pattern.Arguments) < 2 {
			t.Fatalf("receiver-origin callback pattern = %#v, want origin %q", call, originRef)
		}
		callback := call.Pattern.Arguments[1]
		if callback.Position != 2 || callback.Resolution != "exact" || callback.ObjectsObserved != 1 ||
			len(callback.ObjectRefs) != 1 || callback.ObjectRefs[0] != callbackRef {
			t.Fatalf("callback argument authority = %#v, want %q", callback, callbackRef)
		}
	}
	ambiguousRouteCall := findPatternCall(
		ambiguousRouteCallerRefs[0], "app.get", "/products/ambiguous",
	)
	if ambiguousRouteCall.Ref == "" || ambiguousRouteCall.Pattern == nil ||
		ambiguousRouteCall.Pattern.ReceiverOriginResolution != "exact" ||
		!reflect.DeepEqual(ambiguousRouteCall.Pattern.ReceiverOriginRefs, []string{
			externalProgramObjectRef("express", "", "default"),
		}) || len(ambiguousRouteCall.Pattern.Arguments) != 3 {
		t.Fatalf("neutral ambiguous Express registration = %#v", ambiguousRouteCall)
	}
	middlewareArgument := ambiguousRouteCall.Pattern.Arguments[1]
	handlerArgument := ambiguousRouteCall.Pattern.Arguments[2]
	if middlewareArgument.Resolution != "exact" || middlewareArgument.ObjectsObserved != 1 ||
		!reflect.DeepEqual(middlewareArgument.ObjectRefs, requestMiddlewareRefs) ||
		handlerArgument.Resolution != "alternatives" ||
		handlerArgument.ObjectsObserved != len(ambiguousHandlerRefs) ||
		!reflect.DeepEqual(handlerArgument.ObjectRefs, ambiguousHandlerRefs) {
		t.Fatalf("neutral Express callback arguments = middleware %#v handler %#v", middlewareArgument, handlerArgument)
	}
	for _, requestPath := range []string{"/products/duplicate-method", "/products/dynamic-method"} {
		fetchCall := findPatternCall(fetchMethodOwnerRefs[0], "fetch", requestPath)
		if fetchCall.Ref == "" || fetchCall.ExternalPackage != javascriptPlatform ||
			fetchCall.Pattern == nil || len(fetchCall.Pattern.Arguments) != 2 ||
			fetchCall.Pattern.Arguments[0].Kind != "literal_string" ||
			fetchCall.Pattern.Arguments[0].Value != requestPath ||
			fetchCall.Pattern.Arguments[1].Kind != "dynamic" {
			t.Fatalf("neutral fetch arguments for %q = %#v", requestPath, fetchCall)
		}
	}
	routeCall := findPatternCall(declarationRefs["startServer"], "app.get", "/products/featured")
	assertPatternCallback(
		routeCall,
		externalProgramObjectRef("express", "", "default"),
		declarationRefs["getFeaturedProduct"],
	)
	consumerCall := findPatternCall(declarationRefs["registerOrderConsumer"], "consumer.subscribe", "orders.created")
	assertPatternCallback(
		consumerCall,
		externalProgramObjectRef("@fixture/kafka-client", "", "createConsumer"),
		declarationRefs["handleOrder"],
	)
	mixedConsumerCall := findPatternCall(ambiguityCallerRefs[0], "consumer.subscribe", "orders.mixed")
	if mixedConsumerCall.Ref == "" || mixedConsumerCall.Pattern == nil || len(mixedConsumerCall.Pattern.Arguments) != 2 ||
		mixedConsumerCall.Pattern.Arguments[1].Resolution != "alternatives" ||
		mixedConsumerCall.Pattern.Arguments[1].ObjectsObserved != len(mixedCallbackRefs) ||
		strings.Join(mixedConsumerCall.Pattern.Arguments[1].ObjectRefs, "\x00") != strings.Join(mixedCallbackRefs, "\x00") {
		t.Fatalf("mixed callable/non-callable callback pattern = %#v, want %#v", mixedConsumerCall, mixedCallbackRefs)
	}
	mixedCallableRef := ""
	for _, ref := range mixedCallbackRefs {
		if declarationKindByRef[ref] == "function" {
			mixedCallableRef = ref
		}
	}
	if mixedCallableRef == "" {
		t.Fatalf("mixed callback lacks callable declaration: %#v", mixedCallbackRefs)
	}
	var dynamicRouteCall Call
	for _, call := range result.Calls {
		if call.CallerRef == declarationRefs["startServer"] && call.Expression == "app.get" &&
			call.Pattern != nil && len(call.Pattern.Arguments) > 0 && call.Pattern.Arguments[0].Kind == "dynamic" {
			dynamicRouteCall = call
			break
		}
	}
	if dynamicRouteCall.Ref == "" || dynamicRouteCall.Pattern.Arguments[0].Kind != "dynamic" ||
		dynamicRouteCall.Pattern.Arguments[0].ObjectsObserved != 1 ||
		len(dynamicRouteCall.Pattern.Arguments[0].ObjectRefs) != 0 {
		t.Fatalf("dynamic route frontier = %#v", dynamicRouteCall)
	}
	runtimeConsumerCall := findPatternCall(declarationRefs["registerRuntimeOrderHandler"], "consumer.subscribe", "")
	if runtimeConsumerCall.Ref == "" || runtimeConsumerCall.Pattern == nil || len(runtimeConsumerCall.Pattern.Arguments) != 2 {
		t.Fatalf("runtime consumer pattern = %#v", runtimeConsumerCall)
	}
	for _, argument := range runtimeConsumerCall.Pattern.Arguments {
		if argument.Kind != "dynamic" || argument.ObjectsObserved != 1 || len(argument.ObjectRefs) != 0 || argument.Resolution != "unresolved" {
			t.Fatalf("runtime consumer unresolved argument = %#v", argument)
		}
	}
	for _, expression := range []string{"localRouterLookalike.get", "localConsumerLookalike.subscribe"} {
		for _, call := range result.Calls {
			if call.Expression == expression && call.Pattern != nil &&
				(call.Pattern.ReceiverOriginsObserved != 0 || len(call.Pattern.ReceiverOriginRefs) != 0) {
				t.Fatalf("local lookalike gained receiver-origin authority: %#v", call)
			}
		}
	}
	computedConsumerFound := false
	for _, call := range result.Calls {
		if call.CallerRef == declarationRefs["registerOrderConsumer"] &&
			strings.Contains(call.Expression, "dynamicConsumer[dynamicSelector]") {
			computedConsumerFound = call.PatternsObserved == 1 && call.Pattern == nil
		}
	}
	if !computedConsumerFound {
		t.Fatal("computed consumer selector was not retained as one omitted observed pattern")
	}
	objectsBySourceRef := make(map[string]programindex.Object, len(index.Objects))
	for _, object := range index.Objects {
		objectsBySourceRef[object.SourceRef] = object
	}
	assertProjectedCallback := func(call Call, callerRef, callbackRef, originRef string) {
		t.Helper()
		origin := objectsBySourceRef[originRef]
		caller := objectsBySourceRef[callerRef]
		callback := objectsBySourceRef[callbackRef]
		if origin.ID == "" || origin.External == nil || caller.ID == "" || callback.ID == "" {
			t.Fatalf("callback ProgramIndex objects = origin %#v caller %#v callback %#v", origin, caller, callback)
		}
		var callRelation, callbackRelation programindex.Relation
		for _, relation := range index.Relations {
			switch relation.SourceRef {
			case "program:" + call.Ref:
				callRelation = relation
			case fmt.Sprintf("callback:%s:%d", call.Ref, 2):
				callbackRelation = relation
			}
		}
		if len(callRelation.Patterns) != 1 || len(callRelation.Patterns[0].ReceiverOriginIDs) != 1 ||
			callRelation.Patterns[0].ReceiverOriginIDs[0] != origin.ID ||
			callRelation.Patterns[0].ReceiverOriginResolution != programindex.ResolutionExact {
			t.Fatalf("projected receiver origin = %#v", callRelation)
		}
		if callbackRelation.Kind != programindex.RelationPassesCallback ||
			callbackRelation.Resolution != programindex.ResolutionExact || callbackRelation.FromID != caller.ID ||
			len(callbackRelation.ToIDs) != 1 || callbackRelation.ToIDs[0] != callback.ID || callbackRelation.Invocation != "" {
			t.Fatalf("projected callback wiring = %#v", callbackRelation)
		}
	}
	assertProjectedCallback(
		routeCall,
		declarationRefs["startServer"],
		declarationRefs["getFeaturedProduct"],
		externalProgramObjectRef("express", "", "default"),
	)
	var mixedCallbackRelation programindex.Relation
	for _, relation := range index.Relations {
		if relation.SourceRef == fmt.Sprintf("callback:%s:%d", mixedConsumerCall.Ref, 2) {
			mixedCallbackRelation = relation
			break
		}
	}
	mixedCallableObject := objectsBySourceRef[mixedCallableRef]
	if mixedCallableObject.ID == "" || mixedCallbackRelation.Kind != programindex.RelationPassesCallback ||
		mixedCallbackRelation.Resolution != programindex.ResolutionAlternatives ||
		len(mixedCallbackRelation.ToIDs) != 1 || mixedCallbackRelation.ToIDs[0] != mixedCallableObject.ID ||
		mixedCallbackRelation.TargetsObserved != len(mixedCallbackRefs) || mixedCallbackRelation.TargetsOmitted != len(mixedCallbackRefs)-1 {
		t.Fatalf("mixed callback projection erased callable authority: relation=%#v callable=%#v", mixedCallbackRelation, mixedCallableObject)
	}
	assertProjectedCallback(
		consumerCall,
		declarationRefs["registerOrderConsumer"],
		declarationRefs["handleOrder"],
		externalProgramObjectRef("@fixture/kafka-client", "", "createConsumer"),
	)
	assertRegistration := func(name string, call Call, callerRef, originRef, callbackRef string) {
		t.Helper()
		caller := objectsBySourceRef[callerRef]
		receiver := objectsBySourceRef[call.Pattern.ReceiverRef]
		origin := objectsBySourceRef[originRef]
		callback := objectsBySourceRef[callbackRef]
		if caller.ID == "" || receiver.ID == "" || origin.ID == "" || callback.ID == "" {
			t.Fatalf("%s objects are incomplete: caller=%#v receiver=%#v origin=%#v callback=%#v",
				name, caller, receiver, origin, callback)
		}
		adaptertest.AssertRegistration(t, index, adaptertest.Registration{
			Name: name,
			Registration: adaptertest.Relation{
				Kind: programindex.RelationCalls, FromID: caller.ID,
				Resolution: programindex.ResolutionUnresolved, Invocation: "call",
				Path: "src/server.ts", Line: call.Location.Line,
				TargetsObserved: 1, TargetsOmitted: 1, WitnessesObserved: 1, PatternsObserved: 1,
				Patterns: []adaptertest.Pattern{{
					Form: programindex.PatternCall, Selector: call.Pattern.Selector, ReceiverID: receiver.ID,
					Path: "src/server.ts", Line: call.Location.Line,
					ReceiverOrigins: adaptertest.ObjectAuthority{
						IDs: []string{origin.ID}, Resolution: programindex.ResolutionExact, Observed: 1,
					},
					Observed: 2,
					Arguments: []adaptertest.Argument{
						{Position: 1, Kind: programindex.PatternLiteralString, Value: call.Pattern.Arguments[0].Value},
						{Position: 2, Kind: programindex.PatternDynamic, Objects: adaptertest.ObjectAuthority{
							IDs: []string{callback.ID}, Resolution: programindex.ResolutionExact, Observed: 1,
						}},
					},
				}},
			},
			Callbacks: []adaptertest.Callback{{
				ArgumentPosition: 2,
				Relation: adaptertest.Relation{
					Kind: programindex.RelationPassesCallback, FromID: caller.ID, ToIDs: []string{callback.ID},
					Resolution: programindex.ResolutionExact, Path: "src/server.ts", Line: call.Location.Line,
					TargetsObserved: 1, WitnessesObserved: 1,
				},
			}},
			RequireComplete: true,
		})
	}
	assertRegistration(
		"TypeScript route callback registration", routeCall,
		declarationRefs["startServer"], externalProgramObjectRef("express", "", "default"),
		declarationRefs["getFeaturedProduct"],
	)
	assertRegistration(
		"TypeScript consumer callback registration", consumerCall,
		declarationRefs["registerOrderConsumer"], externalProgramObjectRef("@fixture/kafka-client", "", "createConsumer"),
		declarationRefs["handleOrder"],
	)
	directCallerRef := declarationRefs["registerDirectOrderConsumer"]
	var directFactoryCall, directConsumerCall Call
	for _, call := range result.Calls {
		if call.CallerRef != directCallerRef || call.Pattern == nil {
			continue
		}
		switch call.Expression {
		case "createConsumer":
			directFactoryCall = call
		case "createConsumer().subscribe":
			directConsumerCall = call
		}
	}
	factoryObjectRef := externalProgramObjectRef("@fixture/kafka-client", "", "createConsumer")
	if directFactoryCall.Ref == "" || directFactoryCall.Pattern.ResultRef == "" ||
		directFactoryCall.Pattern.ReceiverRef != "" || directFactoryCall.Location.Path != "src/server.ts" ||
		directFactoryCall.Location.Line != 81 || directFactoryCall.Location.Column != 3 {
		t.Fatalf("direct TypeScript factory call = %#v", directFactoryCall)
	}
	if directConsumerCall.Ref == "" || directConsumerCall.Pattern.ReceiverRef != directFactoryCall.Pattern.ResultRef ||
		directConsumerCall.Pattern.ReceiverRef == factoryObjectRef ||
		directConsumerCall.Location.Path != "src/server.ts" || directConsumerCall.Location.Line != 81 ||
		directConsumerCall.Location.Column != 3 {
		t.Fatalf("direct TypeScript continuation call = %#v; factory=%#v", directConsumerCall, directFactoryCall)
	}
	directCaller := objectsBySourceRef[directCallerRef]
	directCallback := objectsBySourceRef[declarationRefs["handleOrder"]]
	directFactoryObject := objectsBySourceRef[factoryObjectRef]
	directResult := objectsBySourceRef[directFactoryCall.Pattern.ResultRef]
	if directCaller.ID == "" || directCallback.ID == "" || directFactoryObject.ID == "" ||
		directResult.ID == "" || directResult.Kind != programindex.ObjectVariable ||
		directResult.Name != "call result" || directResult.Location == nil ||
		directResult.Location.Path != "src/server.ts" || directResult.Location.Line != 81 ||
		directResult.Location.Column != 3 || directResult.ID == directFactoryObject.ID {
		t.Fatalf("direct TypeScript result authority: caller=%#v callback=%#v factory=%#v result=%#v",
			directCaller, directCallback, directFactoryObject, directResult)
	}
	callResults := 0
	for _, object := range index.Objects {
		if object.Kind == programindex.ObjectVariable && object.Name == "call result" {
			callResults++
		}
	}
	if callResults != 1 {
		t.Fatalf("TypeScript synthetic call-result objects = %d, want only the directly consumed factory result", callResults)
	}
	directContinuation := adaptertest.Relation{
		Kind: programindex.RelationCalls, FromID: directCaller.ID,
		Resolution: programindex.ResolutionUnresolved, Invocation: "call",
		Path: "src/server.ts", Line: 81,
		TargetsObserved: 1, TargetsOmitted: 1, WitnessesObserved: 1, PatternsObserved: 1,
		Patterns: []adaptertest.Pattern{{
			Form: programindex.PatternCall, Selector: "subscribe",
			Path: "src/server.ts", Line: 81, Column: 3, Observed: 2,
			Arguments: []adaptertest.Argument{
				{Position: 1, Kind: programindex.PatternLiteralString, Value: "orders.direct"},
				{Position: 2, Kind: programindex.PatternDynamic, Objects: adaptertest.ObjectAuthority{
					IDs: []string{directCallback.ID}, Resolution: programindex.ResolutionExact, Observed: 1,
				}},
			},
		}},
	}
	adaptertest.AssertRegistration(t, index, adaptertest.Registration{
		Name: "TypeScript direct factory result continuation",
		Registration: adaptertest.Relation{
			Kind: programindex.RelationInvokesExternal, FromID: directCaller.ID,
			ToIDs: []string{directFactoryObject.ID}, Resolution: programindex.ResolutionExact,
			Invocation: "call", Path: "src/server.ts", Line: 81,
			TargetsObserved: 1, WitnessesObserved: 1, PatternsObserved: 1,
			Patterns: []adaptertest.Pattern{{
				Form: programindex.PatternCall, Selector: "createConsumer", RequireResult: true,
				Path: "src/server.ts", Line: 81, Column: 3,
			}},
		},
		Continuation:        &directContinuation,
		ResultPattern:       0,
		ContinuationPattern: 0,
		RequireComplete:     true,
	})
	directContinuation.Patterns[0].ReceiverID = directResult.ID
	adaptertest.AssertRegistration(t, index, adaptertest.Registration{
		Name:         "TypeScript direct-result callback registration",
		Registration: directContinuation,
		Callbacks: []adaptertest.Callback{{
			ArgumentPosition: 2,
			Relation: adaptertest.Relation{
				Kind: programindex.RelationPassesCallback, FromID: directCaller.ID,
				ToIDs: []string{directCallback.ID}, Resolution: programindex.ResolutionExact,
				Path: "src/server.ts", Line: 81, TargetsObserved: 1, WitnessesObserved: 1,
			},
		}},
		RequireComplete: true,
	})
	assertExactLocalCall := func(callerRef, expression, calleeRef string) Call {
		t.Helper()
		for _, call := range result.Calls {
			if call.CallerRef == callerRef && call.Expression == expression && call.Resolution == "exact" &&
				call.ExternalPackage == "" && len(call.CalleeRefs) == 1 && call.CalleeRefs[0] == calleeRef {
				return call
			}
		}
		t.Fatalf("exact local call %q from %q to %q missing", expression, callerRef, calleeRef)
		return Call{}
	}
	startupCall := calls["startServer"]
	if startupCall.Ref == "" || startupCall.Resolution != "exact" || len(startupCall.CalleeRefs) != 1 ||
		startupCall.CalleeRefs[0] != declarationRefs["startServer"] {
		t.Fatalf("module startup call = %#v", startupCall)
	}
	loadCall := assertExactLocalCall(
		declarationRefs["getFeaturedProduct"],
		"loadFeaturedProduct",
		declarationRefs["loadFeaturedProduct"],
	)
	recordCall := assertExactLocalCall(
		declarationRefs["handleOrder"],
		"recordOrder",
		declarationRefs["recordOrder"],
	)
	var outboundCall Call
	for _, call := range result.Calls {
		if call.CallerRef == declarationRefs["loadFeaturedProduct"] && call.Expression == "axios.get" &&
			call.ExternalPackage == "axios" && call.ExternalExport == "default" &&
			call.ExternalReceiver == "default" && call.ExternalName == "get" && call.Resolution == "exact" {
			outboundCall = call
			break
		}
	}
	if outboundCall.Ref == "" {
		t.Fatal("route-handler chain lost its exact outbound axios call")
	}
	for _, call := range []Call{startupCall, loadCall, recordCall, outboundCall} {
		found := false
		for _, relation := range index.Relations {
			if relation.SourceRef != "program:"+call.Ref || relation.Resolution != programindex.ResolutionExact || len(relation.ToIDs) != 1 {
				continue
			}
			if call.ExternalPackage == "" && relation.Kind == programindex.RelationCalls ||
				call.ExternalPackage != "" && relation.Kind == programindex.RelationInvokesExternal {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("exact chain call %q lacks ProgramIndex relation", call.Expression)
		}
	}
	for _, relation := range index.Relations {
		if relation.Kind == programindex.RelationPassesCallback && relation.FromID == objectsBySourceRef[declarationRefs["registerRuntimeOrderHandler"]].ID {
			t.Fatalf("unresolved runtime callback gained wiring authority: %#v", relation)
		}
	}
	projectedPattern := false
	for _, relation := range index.Relations {
		if relation.SourceRef != "program:"+templatePatternCall.Ref {
			continue
		}
		projectedPattern = true
		if relation.PatternsObserved != 1 || len(relation.Patterns) != 1 || relation.PatternsOmitted != 0 ||
			relation.Patterns[0].Form != programindex.PatternCall || relation.Patterns[0].Selector != "get" ||
			len(relation.Patterns[0].Arguments) != 1 ||
			relation.Patterns[0].Arguments[0].Kind != programindex.PatternStringTemplate {
			t.Fatalf("neutral ProgramIndex call pattern = %#v", relation)
		}
	}
	if !projectedPattern {
		t.Fatalf("neutral ProgramIndex call pattern missing for %q", templatePatternCall.Ref)
	}
	computedOmission := false
	for _, relation := range index.Relations {
		if relation.SourceRef != "program:"+computedPatternCall.Ref {
			continue
		}
		computedOmission = relation.PatternsObserved == 1 && relation.PatternsOmitted == 1 && len(relation.Patterns) == 0
	}
	if !computedOmission {
		t.Fatalf("computed selector omission coverage missing for %q", computedPatternCall.Ref)
	}
	for _, relation := range index.Relations {
		if relation.SourceRef != "program:"+stateSetter.Ref {
			continue
		}
		if relation.Kind != programindex.RelationCalls || relation.Resolution != programindex.ResolutionUnresolved || len(relation.ToIDs) != 0 {
			t.Fatalf("destructured state setter ProgramIndex relation = %#v", relation)
		}
	}
	for _, dependency := range catalog.Dependencies {
		if dependency.PackagePath == javascriptPlatform {
			t.Fatalf("JavaScript platform authority leaked into cumulative dependency catalog: %#v", dependency)
		}
	}
}

func assertCumulativeJSTSActualToFormalValueProvenance(t *testing.T, result Result, index programindex.Index) {
	t.Helper()
	declarations := make(map[string]string)
	for _, declaration := range result.Declarations {
		if declaration.Location.Path == "src/server.ts" {
			declarations[declaration.Name] = declaration.Ref
		}
	}
	for _, name := range []string{"startServer", "startAmbiguousServer", "startReassignedServer"} {
		if declarations[name] == "" {
			t.Fatalf("actual-to-formal declaration %q missing", name)
		}
	}
	var sourceCall, formalUseCall Call
	for _, call := range result.Calls {
		if call.Expression == "startServer" && call.Resolution == "exact" && len(call.CalleeRefs) == 1 &&
			call.CalleeRefs[0] == declarations["startServer"] && call.Pattern != nil &&
			len(call.Pattern.Arguments) == 1 && call.Pattern.Arguments[0].Kind == "literal_string" &&
			call.Pattern.Arguments[0].Value == "/products/runtime" {
			sourceCall = call
		}
		if call.CallerRef == declarations["startServer"] && call.Expression == "app.get" && call.Pattern != nil &&
			len(call.Pattern.Arguments) > 0 && call.Pattern.Arguments[0].Kind == "dynamic" {
			formalUseCall = call
		}
	}
	if sourceCall.Ref == "" || formalUseCall.Ref == "" {
		t.Fatalf("actual-to-formal calls: source=%#v destination=%#v", sourceCall, formalUseCall)
	}
	formalUse := formalUseCall.Pattern.Arguments[0]
	if formalUse.ValueCandidatesObserved != 1 || len(formalUse.ValueCandidates) != 1 {
		t.Fatalf("runtime actual-to-formal candidates = %#v", formalUse)
	}
	candidate := formalUse.ValueCandidates[0]
	if candidate.Kind != "literal_string" || candidate.Value != "/products/runtime" ||
		candidate.Resolution != "possible" || candidate.SourceKind != "actual_argument" ||
		candidate.SourceCallRef != sourceCall.Ref || candidate.SourcePosition != 1 {
		t.Fatalf("runtime actual-to-formal candidate = %#v, source=%#v", candidate, sourceCall)
	}
	for _, negative := range []struct {
		name      string
		calleeRef string
	}{
		{name: "ambiguous incoming actuals", calleeRef: declarations["startAmbiguousServer"]},
		{name: "reassigned formal", calleeRef: declarations["startReassignedServer"]},
	} {
		found := false
		for _, call := range result.Calls {
			if call.CallerRef != negative.calleeRef || call.Expression != "app.get" || call.Pattern == nil ||
				len(call.Pattern.Arguments) == 0 || call.Pattern.Arguments[0].Kind != "dynamic" {
				continue
			}
			found = true
			argument := call.Pattern.Arguments[0]
			if argument.ValueCandidatesObserved != 0 || len(argument.ValueCandidates) != 0 {
				t.Fatalf("%s gained actual-to-formal candidate: %#v", negative.name, argument)
			}
		}
		if !found {
			t.Fatalf("%s dynamic formal-use call missing", negative.name)
		}
	}
	var sourceRelation, formalUseRelation programindex.Relation
	for _, relation := range index.Relations {
		switch relation.SourceRef {
		case "program:" + sourceCall.Ref:
			sourceRelation = relation
		case "program:" + formalUseCall.Ref:
			formalUseRelation = relation
		}
	}
	if len(sourceRelation.Patterns) != 1 || len(sourceRelation.Patterns[0].Arguments) != 1 ||
		len(formalUseRelation.Patterns) != 1 || len(formalUseRelation.Patterns[0].Arguments) == 0 {
		t.Fatalf("actual-to-formal ProgramIndex patterns: source=%#v destination=%#v", sourceRelation, formalUseRelation)
	}
	actualArgument := sourceRelation.Patterns[0].Arguments[0]
	formalUseArgument := formalUseRelation.Patterns[0].Arguments[0]
	if len(formalUseArgument.ValueCandidates) != 1 || formalUseArgument.ValueCandidatesObserved != 1 ||
		formalUseArgument.ValueCandidatesOmitted != 0 {
		t.Fatalf("projected actual-to-formal candidates = %#v", formalUseArgument)
	}
	projectedCandidate := formalUseArgument.ValueCandidates[0]
	if projectedCandidate.Kind != programindex.PatternLiteralString || projectedCandidate.Value != "/products/runtime" ||
		projectedCandidate.Resolution != programindex.PatternValuePossible ||
		projectedCandidate.SourceKind != programindex.PatternValueSourceActualArgument ||
		!reflect.DeepEqual(projectedCandidate.SourceArgumentIDs, []string{actualArgument.ID}) ||
		projectedCandidate.SourceArgumentsObserved != 1 || projectedCandidate.SourceArgumentsOmitted != 0 ||
		len(projectedCandidate.SourceObjectIDs) != 0 || projectedCandidate.SourceObjectsObserved != 0 {
		t.Fatalf("sealed actual-to-formal candidate = %#v, source argument=%#v", projectedCandidate, actualArgument)
	}
}

func TestAxiosCallsProjectNeutralExternalOriginsAndPathArguments(t *testing.T) {
	root := preparedTempProject(t)
	writeTestFile(t, root, "package.json", `{"name":"axios-patterns","dependencies":{"axios":"1.0.0"},"devDependencies":{"typescript":"5.9.3"}}`)
	writeTestFile(t, root, "tsconfig.json", `{"include":["src/main.ts"],"compilerOptions":{"module":"ESNext","moduleResolution":"bundler","strict":true}}`)
	writeTestFile(t, root, "src/main.ts", "import axios from \"axios\"\n"+
		"export async function load(levelId: string, payload: unknown): Promise<void> {\n"+
		"  await axios.get(\"/api/levels\")\n"+
		"  await axios.get(`/api/level/${levelId}`)\n"+
		"  await axios.post(\"/api/level/run\", payload)\n"+
		"}\n")
	tracked := []string{"package.json", "src/main.ts", "tsconfig.json"}
	repository, err := corpus.New(context.Background(), root, gitfiles.Listing{Paths: tracked, RegularPaths: tracked})
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	result, index, _, err := Build(context.Background(), repository, root)
	if err != nil {
		t.Fatal(err)
	}
	wantLiteral := map[string]string{"get": "/api/levels", "post": "/api/level/run"}
	foundLiteral := map[string]bool{}
	foundTemplate := false
	patternCalls := map[string]Call{}
	for _, call := range result.Calls {
		if call.Expression != "axios.get" && call.Expression != "axios.post" {
			continue
		}
		if call.Pattern == nil || call.ExternalPackage != "axios" || call.ExternalExport != "default" ||
			call.ExternalReceiver != "default" || call.ExternalName != call.Pattern.Selector ||
			call.PatternsObserved != 1 || call.Pattern.Selector == "" ||
			call.Pattern.ReceiverRef != "" || call.Pattern.ReceiverOriginsObserved != 0 {
			t.Fatalf("Axios neutral call pattern = %#v", call)
		}
		patternCalls[call.Ref] = call
		first := call.Pattern.Arguments[0]
		if first.Kind == "literal_string" && first.Value == wantLiteral[call.Pattern.Selector] {
			foundLiteral[call.Pattern.Selector] = true
		}
		if call.Pattern.Selector == "get" && first.Kind == "string_template" &&
			len(first.Parts) == 2 && first.Parts[0] == (CallPatternPart{Kind: "literal", Text: "/api/level/"}) &&
			first.Parts[1] == (CallPatternPart{Kind: "hole"}) {
			foundTemplate = true
		}
		if call.Pattern.Selector == "post" && (len(call.Pattern.Arguments) != 2 || call.Pattern.Arguments[1].Kind != "dynamic") {
			t.Fatalf("Axios POST positional arguments = %#v", call.Pattern.Arguments)
		}
	}
	if !foundLiteral["get"] || !foundLiteral["post"] || !foundTemplate || len(patternCalls) != 3 {
		t.Fatalf("Axios neutral pattern coverage = literals=%#v template=%v calls=%#v", foundLiteral, foundTemplate, patternCalls)
	}
	objectsByID := make(map[string]programindex.Object, len(index.Objects))
	for _, object := range index.Objects {
		objectsByID[object.ID] = object
	}
	projectedCalls := 0
	for _, relation := range index.Relations {
		call, ok := patternCalls[strings.TrimPrefix(relation.SourceRef, "program:")]
		if !ok {
			continue
		}
		projectedCalls++
		target := programindex.Object{}
		if len(relation.ToIDs) == 1 {
			target = objectsByID[relation.ToIDs[0]]
		}
		if relation.Kind != programindex.RelationInvokesExternal || relation.Resolution != programindex.ResolutionExact ||
			target.External == nil || target.External.AuthorityKind != programindex.ExternalAuthorityPackage ||
			target.External.PackagePath != "axios" ||
			target.External.Name != call.Pattern.Selector || relation.PatternsObserved != 1 ||
			relation.PatternsOmitted != 0 || len(relation.Patterns) != 1 ||
			relation.Patterns[0].Selector != call.Pattern.Selector {
			t.Fatalf("Axios ProgramIndex pattern = %#v", relation)
		}
	}
	if projectedCalls != len(patternCalls) {
		t.Fatalf("Axios ProgramIndex pattern coverage = %d, want %d", projectedCalls, len(patternCalls))
	}
}

func TestPreparedJavaScriptJSXProjectUsesWeakerAuthority(t *testing.T) {
	root := preparedTempProject(t)
	writeTestFile(t, root, "jsconfig.json", `{"include":["src/**/*.js","src/**/*.jsx"],"compilerOptions":{"allowJs":true,"checkJs":false,"module":"ESNext","moduleResolution":"bundler","jsx":"preserve"}}`)
	tracked := []string{"jsconfig.json", "package.json", "src/lookalike.js", "src/widget.jsx"}
	repository, err := corpus.New(context.Background(), root, gitfiles.Listing{Paths: tracked, RegularPaths: tracked})
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	result, index, _, err := Build(context.Background(), repository, root)
	if err != nil {
		t.Fatal(err)
	}
	if result.Project.Language != "javascript" || index.Target.Language != "javascript" || TargetKind(result) != "library" {
		t.Fatalf("pure JavaScript/JSX project authority = %#v / %#v", result.Project, index.Target)
	}
	for _, call := range result.Calls {
		if call.Resolution == "exact" {
			t.Fatalf("pure JavaScript/JSX call gained exact authority: %#v", call)
		}
	}
}

func TestPreparedPackageBinaryAndDevEntryRemainSeparateCLIAuthorities(t *testing.T) {
	root := preparedTempProject(t)
	writeTestFile(t, root, "package.json", `{
  "name":"opencode",
  "bin":{"opencode":"./bin/opencode"},
  "scripts":{
    "dev":"bun run --conditions=browser ./src/index.ts",
    "dev:temporary":"bun run ./src/temporary.ts"
  },
  "devDependencies":{"typescript":"5.9.3"}
}`)
	writeTestFile(t, root, "tsconfig.json", `{
  "include":["src/index.ts","src/temporary.ts"],
  "compilerOptions":{"module":"ESNext","moduleResolution":"bundler","strict":true}
}`)
	writeTestFile(t, root, "bin/opencode", "#!/usr/bin/env node\n")
	writeTestFile(t, root, "src/index.ts", "export const command = 'opencode'\n")
	writeTestFile(t, root, "src/temporary.ts", "export const temporary = true\n")
	tracked := []string{"bin/opencode", "package.json", "src/index.ts", "src/temporary.ts", "tsconfig.json"}
	repository, err := corpus.New(
		context.Background(), root,
		gitfiles.Listing{Paths: tracked, RegularPaths: tracked, ExecutablePaths: []string{"bin/opencode"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	result, index, _, err := Build(context.Background(), repository, root)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Project.Binaries) != 1 ||
		result.Project.Binaries[0].Command != "opencode" ||
		result.Project.Binaries[0].Path != "bin/opencode" {
		t.Fatalf("package binaries = %#v", result.Project.Binaries)
	}
	cliProductSurface := false
	for _, surface := range result.Surfaces {
		if surface.Kind == SurfaceCLI && surface.Role == SurfaceProduct {
			cliProductSurface = true
		}
		if surface.Kind == SurfaceCLI && len(surface.EntryRefs) != 0 {
			t.Fatalf("CLI surface invented bin-to-source entry refs: %#v", surface)
		}
	}
	if !cliProductSurface {
		t.Fatalf("CLI product surface missing: %#v", result.Surfaces)
	}
	if index.Target.Kind != "application" || len(index.Target.Seeds) != 1 || index.Target.Seeds[0].Kind != programindex.SeedScript {
		t.Fatalf("CLI ProgramTarget = %#v", index.Target)
	}
	seededPath := ""
	for _, object := range index.Objects {
		if object.ID == index.Target.Seeds[0].ObjectID && object.Location != nil {
			seededPath = object.Location.Path
		}
	}
	if seededPath != "src/index.ts" {
		t.Fatalf("CLI runtime seed path = %q, want src/index.ts", seededPath)
	}
}

func TestPreparedNamelessPackageUsesLockfileIdentityWithoutInventingImplicitBin(t *testing.T) {
	root := preparedTempProject(t)
	writeTestFile(t, root, "package.json", `{
  "bin":"./bin/meetup",
  "devDependencies":{"typescript":"5.9.3"}
}`)
	writeTestFile(t, root, "package-lock.json", `{
  "name":"meetup",
  "lockfileVersion":3,
  "packages":{"":{"devDependencies":{"typescript":"5.9.3"}}}
}`)
	writeTestFile(t, root, "bun.lock", "lockfileVersion = 1\n")
	writeTestFile(t, root, "bin/meetup", "#!/usr/bin/env node\n")
	tracked := []string{"bin/meetup", "bun.lock", "package-lock.json", "package.json", "src/main.ts", "tsconfig.json"}
	repository, err := corpus.New(
		context.Background(), root,
		gitfiles.Listing{Paths: tracked, RegularPaths: tracked, ExecutablePaths: []string{"bin/meetup"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()

	result, index, catalog, err := Build(context.Background(), repository, root)
	if err != nil {
		t.Fatal(err)
	}
	if result.Project.Name != "meetup" || result.Project.PackagePath != "meetup" ||
		index.Target.Name != "meetup" || len(catalog.Importers) != 1 || catalog.Importers[0].Name != "meetup" {
		t.Fatalf("lockfile-restored project identity = %#v / %#v / %#v", result.Project, index.Target, catalog.Importers)
	}
	if result.Project.PackageManager != "bun" || result.Project.LockfilePath != "bun.lock" {
		t.Fatalf("package-manager lockfile facts = %#v, want bun/bun.lock", result.Project)
	}
	if len(result.Project.Binaries) != 0 || hasCLIProductSurface(result) {
		t.Fatalf("lockfile name invented package.json string-bin command authority: %#v / %#v", result.Project.Binaries, result.Surfaces)
	}

	fallbackTracked := []string{"bin/meetup", "package.json", "src/main.ts", "tsconfig.json"}
	fallbackRepository, err := corpus.New(
		context.Background(), root,
		gitfiles.Listing{Paths: fallbackTracked, RegularPaths: fallbackTracked, ExecutablePaths: []string{"bin/meetup"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer fallbackRepository.Close()
	fallback, fallbackIndex, fallbackCatalog, err := Build(context.Background(), fallbackRepository, root)
	if err != nil {
		t.Fatal(err)
	}
	if fallback.Project.Name != "root-package" || fallback.Project.PackagePath != "root-package" ||
		fallbackIndex.Target.Name != "root-package" || len(fallbackCatalog.Importers) != 1 ||
		fallbackCatalog.Importers[0].Name != "root-package" {
		t.Fatalf("repository-relative root fallback = %#v / %#v / %#v", fallback.Project, fallbackIndex.Target, fallbackCatalog.Importers)
	}
	if len(fallback.Project.Binaries) != 0 || hasCLIProductSurface(fallback) {
		t.Fatalf("root fallback invented package.json string-bin command authority: %#v / %#v", fallback.Project.Binaries, fallback.Surfaces)
	}
}

func TestNamelessPackageIdentityFallbackIsRepositoryRelative(t *testing.T) {
	if got, err := selectedPackageIdentityName(nil, ".", ""); err != nil || got != "root-package" {
		t.Fatalf("root package fallback = %q (error %v), want root-package", got, err)
	}
	if got, err := selectedPackageIdentityName(nil, "packages/web", ""); err != nil || got != "packages/web" {
		t.Fatalf("nested package fallback = %q (error %v), want packages/web", got, err)
	}
	if got, err := selectedPackageIdentityName(nil, ".", " declared "); err != nil || got != "declared" {
		t.Fatalf("manifest package name = %q (error %v), want declared", got, err)
	}
}

func TestReadPackageManifestRejectsInvalidEnvelope(t *testing.T) {
	for _, content := range []string{
		`null`,
		`{"name":"active"}{"name":"shadow"}`,
	} {
		root := t.TempDir()
		writeTestFile(t, root, "package.json", content)
		repository, err := corpus.New(
			context.Background(), root,
			gitfiles.Listing{Paths: []string{"package.json"}, RegularPaths: []string{"package.json"}},
		)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := readPackageManifest(repository, "package.json"); err == nil ||
			!strings.Contains(err.Error(), "invalid package manifest") {
			t.Fatalf("package manifest %q error = %v", content, err)
		}
		if err := repository.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestManifestAndLockfileReadPastFormerLocalLimit(t *testing.T) {
	const formerManifestLimit = 4 << 20
	padding := strings.Repeat("x", formerManifestLimit+1)
	repository := newTargetScoutCorpus(t, map[string]string{
		"package.json":      `{"name":"large-manifest","padding":"` + padding + `"}`,
		"package-lock.json": `{"name":"large-lockfile","padding":"` + padding + `"}`,
	})
	defer repository.Close()
	manifest, err := readPackageManifest(repository, "package.json")
	if err != nil {
		t.Fatalf("readPackageManifest: %v", err)
	}
	if manifest.Name != "large-manifest" {
		t.Fatalf("manifest name = %q, want large-manifest", manifest.Name)
	}
	if got, err := packageLockProjectName(repository, "package-lock.json"); err != nil || got != "large-lockfile" {
		t.Fatalf("lockfile name = %q (error %v), want large-lockfile", got, err)
	}
}

func TestHelperTransportAndProjectConfigGraphHaveNoLocalCaps(t *testing.T) {
	for _, forbidden := range []string{
		"MAX_INPUT_BYTES", "MAX_CONFIG_BYTES", "MAX_PROJECT_CONFIGS", "MAX_PROJECT_REFERENCE_DEPTH",
		"input exceeds limit", "project config count exceeds limit", "project reference depth exceeds limit",
	} {
		if strings.Contains(nodeHelper, forbidden) {
			t.Fatalf("embedded TypeScript helper still contains local structural cap %q", forbidden)
		}
	}

	const formerInjectedThreshold = 4
	encoded := append(bytes.Repeat([]byte(" "), formerInjectedThreshold+1), []byte(`{}`)...)
	if _, err := decodeHelperOutput(encoded); err != nil {
		t.Fatalf("decodeHelperOutput past former injected threshold: %v", err)
	}

	diagnostic := &boundedBuffer{limit: formerInjectedThreshold}
	written, err := diagnostic.Write([]byte("abcdef"))
	if err != nil || written != 6 {
		t.Fatalf("bounded diagnostic Write = (%d, %v), want drained (6, nil)", written, err)
	}
	written, err = diagnostic.Write([]byte("gh"))
	if err != nil || written != 2 || diagnostic.buffer.String() != "abcd" || !diagnostic.exceeded {
		t.Fatalf(
			"bounded diagnostic second Write = (%d, %v), value=%q exceeded=%t",
			written, err, diagnostic.buffer.String(), diagnostic.exceeded,
		)
	}
}

func TestManifestTypeScriptCompilerPackagesKeepDirectAndNpmAliasAuthority(t *testing.T) {
	manifest := packageManifest{
		Dependencies: map[string]string{
			"runtime":        "1.0.0",
			"typescript-api": "npm:typescript@6.0.3",
		},
		DevDependencies: map[string]string{
			"typescript": "^7.0.2",
			"lookalike":  "npm:not-typescript@1.0.0",
		},
	}
	got := typeScriptCompilerPackageNames(manifest)
	want := []string{"typescript", "typescript-api"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("TypeScript compiler package candidates = %#v, want %#v", got, want)
	}
}

func TestSelectedManifestCompilerDeclarationsDoNotMixWithRootFallback(t *testing.T) {
	selected := packageManifest{DevDependencies: map[string]string{"typescript": "7.0.2"}}
	root := packageManifest{DevDependencies: map[string]string{"typescript-api": "npm:typescript@6.0.3"}}
	got := typeScriptCompilerPackagesForProject(selected, &root)
	if len(got) != 1 || got[0].Name != "typescript" || got[0].ResolutionBase != helperCompilerResolutionProject {
		t.Fatalf("selected compiler candidates = %#v", got)
	}
	if fallback := typeScriptCompilerPackagesForProject(packageManifest{}, &root); len(fallback) != 1 ||
		fallback[0].Name != "typescript-api" || fallback[0].ResolutionBase != helperCompilerResolutionRepositoryRoot {
		t.Fatalf("root compiler fallback candidates = %#v", fallback)
	}
}

func TestRepositoryRootCompilerFallbackCannotBeShadowedBySelectedChild(t *testing.T) {
	repositoryRoot := preparedCompilerProject(t)
	rootNodeModules := filepath.Join(repositoryRoot, "node_modules")
	if err := os.Rename(filepath.Join(rootNodeModules, "typescript"), filepath.Join(rootNodeModules, "typescript-api")); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, repositoryRoot, "package.json", `{
  "name":"workspace",
  "workspaces":["front"],
  "scripts":{"dev":"bun run --cwd front src/main.ts"},
  "devDependencies":{"typescript-api":"npm:typescript@5.9.3"}
}`)
	writeTestFile(t, repositoryRoot, "front/package.json", `{"name":"front"}`)
	writeTestFile(t, repositoryRoot, "front/tsconfig.json", `{
  "include":["src/**/*.ts"],
  "compilerOptions":{"module":"ESNext","moduleResolution":"bundler","strict":true}
}`)
	writeTestFile(t, repositoryRoot, "front/src/main.ts", "export const ready: boolean = true\n")

	// This deliberately has the same install name as the root alias. It is a
	// supported-looking legacy package shape, but not a usable compiler. A
	// child-based require would select it and fail; root-owned fallback
	// authority must resolve from the repository-root manifest instead.
	writeTestFile(t, repositoryRoot, "front/node_modules/typescript-api/package.json", `{"name":"typescript","version":"0.0.0"}`)
	writeTestFile(t, repositoryRoot, "front/node_modules/typescript-api/lib/typescript.js", `module.exports = {}`)

	tracked := []string{
		"front/package.json",
		"front/src/main.ts",
		"front/tsconfig.json",
		"package.json",
	}
	repository, err := corpus.New(
		context.Background(), repositoryRoot,
		gitfiles.Listing{Paths: tracked, RegularPaths: tracked},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()

	result, err := Discover(context.Background(), repository, repositoryRoot)
	if err != nil {
		t.Fatal(err)
	}
	if result.Project.ManifestPath != "front/package.json" || len(result.Files) != 1 || result.Files[0].Path != "front/src/main.ts" {
		t.Fatalf("root-fallback compiler project = %#v; files = %#v", result.Project, result.Files)
	}
}

func TestPreparedCompilerPrefersOneLegacyAPICandidateOverNativeAPI(t *testing.T) {
	root := preparedCompilerProject(t)
	nodeModules := filepath.Join(root, "node_modules")
	if err := os.Rename(filepath.Join(nodeModules, "typescript"), filepath.Join(nodeModules, "typescript-api")); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, root, "node_modules/typescript/package.json", `{
  "name":"typescript",
  "version":"7.0.2",
  "type":"module",
  "exports":{
    "./package.json":"./package.json",
    "./unstable/sync":"./dist/api/sync.js",
    "./unstable/ast":"./dist/ast.js"
  }
}`)
	// These files establish a supported native package shape. They deliberately
	// cannot execute: success therefore proves the one legacy API candidate was
	// selected before native Compiler API startup.
	writeTestFile(t, root, "node_modules/typescript/dist/api/sync.js", `throw new Error("native compiler must not load")`)
	writeTestFile(t, root, "node_modules/typescript/dist/ast.js", `throw new Error("native compiler must not load")`)
	writeTestFile(t, root, "package.json", `{
  "name":"dual-compiler",
  "devDependencies":{
    "typescript":"7.0.2",
    "typescript-api":"npm:typescript@6.0.3"
  }
}`)
	writeTestFile(t, root, "tsconfig.json", `{"include":["src/**/*.ts"],"compilerOptions":{"strict":true}}`)
	writeTestFile(t, root, "src/main.ts", "export const ready: boolean = true\n")
	tracked := []string{"package.json", "src/main.ts", "tsconfig.json"}
	repository, err := corpus.New(
		context.Background(), root,
		gitfiles.Listing{Paths: tracked, RegularPaths: tracked},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	result, _, _, err := Build(context.Background(), repository, root)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Files) != 1 || result.Files[0].Path != "src/main.ts" {
		t.Fatalf("legacy alias compiler files = %#v", result.Files)
	}
}

func TestPreparedCompilerRejectsDistinctLegacyCandidateAmbiguity(t *testing.T) {
	root := preparedTempProject(t)
	writeTestFile(t, root, "node_modules/typescript-api/package.json", `{"name":"typescript","version":"6.0.3"}`)
	writeTestFile(t, root, "node_modules/typescript-api/lib/typescript.js", `module.exports = {}`)
	writeTestFile(t, root, "package.json", `{
  "name":"ambiguous-compiler",
  "devDependencies":{
    "typescript":"5.9.3",
    "typescript-api":"npm:typescript@6.0.3"
  }
}`)
	tracked := []string{"package.json", "src/main.ts", "tsconfig.json"}
	repository, err := corpus.New(
		context.Background(), root,
		gitfiles.Listing{Paths: tracked, RegularPaths: tracked},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	_, err = Discover(context.Background(), repository, root)
	if !errors.Is(err, ErrTypeScriptCompilerUnavailable) ||
		!strings.Contains(err.Error(), "ambiguous supported TypeScript compiler packages: typescript, typescript-api") {
		t.Fatalf("ambiguous compiler error = %v", err)
	}
}

func TestPreparedCompilerLoadsThroughPackageExportsBoundary(t *testing.T) {
	root := preparedTempProject(t)
	manifestPath := filepath.Join(root, "node_modules", "typescript", "package.json")
	encoded, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(encoded, &manifest); err != nil {
		t.Fatal(err)
	}
	// Modern packages may explicitly export package.json while keeping their
	// compatible Compiler API implementation as a package-owned private file.
	// The helper must establish the package root without asking Node to resolve
	// that private subpath through exports.
	manifest["exports"] = map[string]any{
		".":              "./lib/typescript.js",
		"./package.json": "./package.json",
	}
	encoded, err = json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, encoded, 0o600); err != nil {
		t.Fatal(err)
	}

	tracked := []string{"package.json", "src/main.ts", "tsconfig.json"}
	repository, err := corpus.New(context.Background(), root, gitfiles.Listing{Paths: tracked, RegularPaths: tracked})
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	result, _, _, err := Build(context.Background(), repository, root)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Files) != 1 || result.Files[0].Path != "src/main.ts" {
		t.Fatalf("exports-bound compiler files = %#v", result.Files)
	}
}

func TestSolutionConfigKeepsReferencedProjectOptionsSeparate(t *testing.T) {
	root := preparedTempProject(t)
	writeTestFile(t, root, "tsconfig.json", `{
  "files": [],
  "references": [
    {"path": "./config/tsconfig.node.json"},
    {"path": "./config/tsconfig.web.json"}
  ]
}`)
	writeTestFile(t, root, "config/tsconfig.node.json", `{
  "include": ["../server/**/*.ts"],
  "compilerOptions": {"module": "Node16", "moduleResolution": "Node16", "strict": true}
}`)
	writeTestFile(t, root, "config/tsconfig.web.json", `{
  "include": ["../client/**/*.ts"],
  "compilerOptions": {
    "module": "ESNext",
    "moduleResolution": "bundler",
    "paths": {"@/*": ["../client/*"]},
    "strict": true
  }
}`)
	writeTestFile(t, root, "server/main.ts", `import { worker } from "./worker"
export function serve(): number { return worker() }
`)
	writeTestFile(t, root, "server/worker.ts", `export function worker(): number { return 1 }
`)
	writeTestFile(t, root, "client/main.ts", `import { view } from "@/view"
export function render(): string { return view() }
`)
	writeTestFile(t, root, "client/view.ts", `export function view(): string { return "ready" }
`)
	tracked := []string{
		"client/main.ts", "client/view.ts", "config/tsconfig.node.json", "config/tsconfig.web.json",
		"package.json", "server/main.ts", "server/worker.ts", "tsconfig.json",
	}
	repository, err := corpus.New(context.Background(), root, gitfiles.Listing{Paths: tracked, RegularPaths: tracked})
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	result, _, _, err := Build(context.Background(), repository, root)
	if err != nil {
		t.Fatal(err)
	}
	wantFiles := []string{"client/main.ts", "client/view.ts", "server/main.ts", "server/worker.ts"}
	gotFiles := make([]string, len(result.Files))
	for index, file := range result.Files {
		gotFiles[index] = file.Path
	}
	if strings.Join(gotFiles, "\n") != strings.Join(wantFiles, "\n") {
		t.Fatalf("solution files = %#v, want %#v", gotFiles, wantFiles)
	}
	if result.Project.ModuleResolution != "mixed" {
		t.Fatalf("solution module resolution = %q, want mixed", result.Project.ModuleResolution)
	}
	if !hasExactImport(result, "@/view", "client/view.ts") || !hasExactImport(result, "./worker", "server/worker.ts") {
		t.Fatalf("referenced project imports lost leaf options: %#v", result.Imports)
	}
	if len(result.Project.PathAliases) != 1 || result.Project.PathAliases[0].Pattern != "@/*" ||
		strings.Join(result.Project.PathAliases[0].Targets, ",") != "client/*" {
		t.Fatalf("solution path aliases = %#v", result.Project.PathAliases)
	}
}

func TestSolutionConfigRetainsFormerlyCappedSizeCountAndDepth(t *testing.T) {
	const (
		formerConfigBytes = 1 << 20
		configCount       = 1_100
	)
	root := preparedCompilerProject(t)
	writeTestFile(t, root, "package.json", `{"name":"large-solution","devDependencies":{"typescript":"5.9.3"}}`)
	writeTestFile(t, root, "src/main.ts", "export const ready: boolean = true\n")

	tracked := []string{"package.json", "src/main.ts", "tsconfig.json"}
	writeTestFile(t, root, "tsconfig.json", `{"padding":"`+strings.Repeat("x", formerConfigBytes+1)+`","files":[],"references":[{"path":"./configs/c00.json"}]}`)
	for index := 0; index < configCount; index++ {
		configPath := fmt.Sprintf("configs/c%02d.json", index)
		content := `{"files":["../src/main.ts"],"compilerOptions":{"module":"ESNext","moduleResolution":"bundler","strict":true}}`
		if index+1 < configCount {
			content = fmt.Sprintf(`{"files":[],"references":[{"path":"./c%02d.json"}]}`, index+1)
		}
		writeTestFile(t, root, configPath, content)
		tracked = append(tracked, configPath)
	}
	sort.Strings(tracked)
	repository, err := corpus.New(
		context.Background(), root,
		gitfiles.Listing{Paths: tracked, RegularPaths: append([]string(nil), tracked...)},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()

	result, _, _, err := Build(context.Background(), repository, root)
	if err != nil {
		t.Fatalf("Build formerly capped solution graph: %v", err)
	}
	if len(result.Files) != 1 || result.Files[0].Path != "src/main.ts" {
		t.Fatalf("large/deep solution files = %#v, want complete source", result.Files)
	}
}

func TestSolutionConfigIterativeWalkRejectsReferenceCycle(t *testing.T) {
	root := preparedCompilerProject(t)
	writeTestFile(t, root, "package.json", `{"name":"cycle","devDependencies":{"typescript":"5.9.3"}}`)
	writeTestFile(t, root, "src/main.ts", "export const ready = true\n")
	writeTestFile(t, root, "tsconfig.json", `{"files":[],"references":[{"path":"./configs/a.json"}]}`)
	writeTestFile(t, root, "configs/a.json", `{"files":[],"references":[{"path":"./b.json"}]}`)
	writeTestFile(t, root, "configs/b.json", `{"files":["../src/main.ts"],"references":[{"path":"./a.json"}]}`)
	tracked := []string{
		"configs/a.json", "configs/b.json", "package.json", "src/main.ts", "tsconfig.json",
	}
	repository, err := corpus.New(
		context.Background(), root,
		gitfiles.Listing{Paths: tracked, RegularPaths: append([]string(nil), tracked...)},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()

	_, _, _, err = Build(context.Background(), repository, root)
	if err == nil || !strings.Contains(err.Error(), "project reference cycle") {
		t.Fatalf("cycle error = %v", err)
	}
}

func TestNestedPackageKeepsSiblingProjectReferenceAsExternalBoundary(t *testing.T) {
	root := preparedTempProject(t)
	var sharedSource strings.Builder
	sharedSource.WriteString("export class Shared { value = true }\nexport default function defaultShared() { return new Shared() }\nexport function makeShared() { return { shared: new Shared()")
	for index := 0; index < 400; index++ {
		_, _ = fmt.Fprintf(&sharedSource, ", field%d: %d", index, index)
	}
	sharedSource.WriteString(" } }\n")
	files := map[string]string{
		"packages/app/package.json": `{"name":"app"}`,
		"packages/app/src/main.ts": `import defaultShared, { makeShared as make } from "../../shared/src/index"
import * as sharedAPI from "../../shared/src/index"
export const app = make()
export const defaultApp = defaultShared()
export const namespaceApp = sharedAPI.makeShared()
`,
		"packages/app/tsconfig.json": `{
  "include": ["src/**/*.ts"],
  "references": [{"path": "../shared"}],
  "compilerOptions": {"module": "ESNext", "moduleResolution": "bundler", "strict": true}
}`,
		"packages/shared/package.json": `{"name":"shared"}`,
		"packages/shared/src/index.ts": sharedSource.String(),
		"packages/shared/tsconfig.json": `{
  "include": ["src/**/*.ts"],
  "references": [{"path": "./missing-child"}],
  "compilerOptions": {"composite": true, "module": "ESNext", "moduleResolution": "bundler", "strict": true}
}`,
	}
	tracked := []string{"package.json"}
	for filePath, content := range files {
		writeTestFile(t, root, filePath, content)
		tracked = append(tracked, filePath)
	}
	sort.Strings(tracked)
	repository, err := corpus.New(
		context.Background(), root,
		gitfiles.Listing{Paths: tracked, RegularPaths: tracked},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()

	result, err := DiscoverSelected(
		context.Background(), repository, root, "jsts:packages/app/package.json",
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Project.Selector != "jsts:packages/app/package.json" ||
		result.Project.ConfigPath != "packages/app/tsconfig.json" || len(result.Files) != 1 ||
		result.Files[0].Path != "packages/app/src/main.ts" {
		t.Fatalf("package with sibling project reference = project %#v; files %#v", result.Project, result.Files)
	}
	for _, file := range result.Files {
		if strings.HasPrefix(file.Path, "packages/shared/") {
			t.Fatalf("sibling package source leaked into selected package page: %#v", file)
		}
	}
	foundApp := false
	for _, declaration := range result.Declarations {
		if declaration.Name != "app" {
			continue
		}
		foundApp = true
		if declaration.Signature != "" {
			t.Fatalf("unsafe inferred sibling signature was retained: %q", declaration.Signature)
		}
	}
	if !foundApp {
		t.Fatalf("app declaration was not retained: %#v", result.Declarations)
	}
	index, _, err := BuildFromResult(result)
	if err != nil {
		t.Fatal(err)
	}
	assertExactSiblingPackageCalls(t, result, index, "shared", map[string]string{
		"make": "makeShared", "defaultShared": "default", "sharedAPI.makeShared": "makeShared",
	})

	writeTestFile(t, filepath.Dir(root), "outside-project/tsconfig.json", `{"include":["src/**/*.ts"]}`)
	writeTestFile(t, root, "packages/app/tsconfig.json", `{
  "include": ["src/**/*.ts"],
  "references": [{"path": "../../../outside-project"}],
  "compilerOptions": {"module": "ESNext", "moduleResolution": "bundler", "strict": true}
}`)
	_, err = DiscoverSelected(
		context.Background(), repository, root, "jsts:packages/app/package.json",
	)
	if err == nil || !strings.Contains(err.Error(), "unresolved project reference ../../../outside-project") {
		t.Fatalf("outside-repository project reference error = %v", err)
	}
}

func TestRootPackageKeepsNestedProjectReferenceAsExternalBoundary(t *testing.T) {
	root := preparedTempProject(t)
	files := map[string]string{
		"package.json": `{"name":"root","devDependencies":{"typescript":"5.9.3"}}`,
		"src/root.ts":  "export const root = true\n",
		"tsconfig.json": `{
  "include": ["src/**/*.ts"],
  "references": [{"path": "./packages/shared"}],
  "compilerOptions": {"module": "ESNext", "moduleResolution": "bundler", "strict": true}
}`,
		"packages/shared/package.json": `{"name":"shared"}`,
		"packages/shared/src/index.ts": "export const shared = true\n",
		"packages/shared/tsconfig.json": `{
  "include": ["src/**/*.ts"],
  "references": [{"path": "./missing-child"}],
  "compilerOptions": {"composite": true, "module": "ESNext", "moduleResolution": "bundler", "strict": true}
}`,
	}
	tracked := make([]string, 0, len(files))
	for filePath, content := range files {
		writeTestFile(t, root, filePath, content)
		tracked = append(tracked, filePath)
	}
	sort.Strings(tracked)
	repository, err := corpus.New(
		context.Background(), root,
		gitfiles.Listing{Paths: tracked, RegularPaths: tracked},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()

	result, err := DiscoverSelected(context.Background(), repository, root, "jsts:package.json")
	if err != nil {
		t.Fatal(err)
	}
	if result.Project.Selector != "jsts:package.json" || result.Project.ConfigPath != "tsconfig.json" ||
		len(result.Files) != 1 || result.Files[0].Path != "src/root.ts" {
		t.Fatalf("root package with nested project reference = project %#v; files %#v", result.Project, result.Files)
	}
	for _, file := range result.Files {
		if strings.HasPrefix(file.Path, "packages/shared/") {
			t.Fatalf("nested package source leaked into root package page: %#v", file)
		}
	}
}

func TestJavaScriptSiblingPackageCallKeepsAlternativeDispatchAndExactExportIdentity(t *testing.T) {
	root := preparedTempProject(t)
	files := map[string]string{
		"packages/app/package.json":    `{"name":"app"}`,
		"packages/app/src/main.js":     "import { serve as invoke } from \"../../shared/src/index.js\"\nexport function run() { return invoke() }\n",
		"packages/shared/package.json": `{"name":"shared"}`,
		"packages/shared/src/index.js": "function implementation() { return 1 }\nexport { implementation as serve }\n",
	}
	tracked := []string{"package.json"}
	for filePath, content := range files {
		writeTestFile(t, root, filePath, content)
		tracked = append(tracked, filePath)
	}
	sort.Strings(tracked)
	repository, err := corpus.New(context.Background(), root, gitfiles.Listing{Paths: tracked, RegularPaths: tracked})
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()

	app, err := DiscoverSelected(context.Background(), repository, root, "jsts:packages/app/package.json")
	if err != nil {
		t.Fatal(err)
	}
	appIndex, _, err := BuildFromResult(app)
	if err != nil {
		t.Fatal(err)
	}
	var call Call
	for _, candidate := range app.Calls {
		if candidate.Expression == "invoke" {
			call = candidate
			break
		}
	}
	if call.Ref == "" || call.Resolution != "alternatives" || call.ExternalPackage != "shared" ||
		call.ExternalExport != "serve" || call.ExternalName != "serve" {
		t.Fatalf("JavaScript sibling call = %#v", call)
	}
	var relation programindex.Relation
	for _, candidate := range appIndex.Relations {
		if candidate.SourceRef == "program:"+call.Ref {
			relation = candidate
			break
		}
	}
	if relation.Resolution != programindex.ResolutionAlternatives || len(relation.ToIDs) != 1 {
		t.Fatalf("JavaScript sibling ProgramIndex relation = %#v", relation)
	}
	externalIdentity := identityForObjectID(t, appIndex, relation.ToIDs[0], "shared#serve")

	shared, err := DiscoverSelected(context.Background(), repository, root, "jsts:packages/shared/package.json")
	if err != nil {
		t.Fatal(err)
	}
	sharedIndex, _, err := BuildFromResult(shared)
	if err != nil {
		t.Fatal(err)
	}
	var serveRef string
	for _, declaration := range shared.Declarations {
		if declaration.Name == "implementation" {
			serveRef = declaration.Ref
			break
		}
	}
	localIdentity := identityForSourceRef(t, sharedIndex, serveRef, "shared#serve")
	for _, object := range sharedIndex.Objects {
		if object.SourceRef == serveRef && object.Visibility != programindex.VisibilityPublic {
			t.Fatalf("export alias implementation visibility = %q, want public", object.Visibility)
		}
	}
	if externalIdentity.Domain != localIdentity.Domain || externalIdentity.Key != localIdentity.Key {
		t.Fatalf("JavaScript sibling identity = %#v, local %#v", externalIdentity, localIdentity)
	}
}

func TestCredentialBearingManifestValuesNeverEnterHelperOrArtifact(t *testing.T) {
	root := preparedTempProject(t)
	secret := "token-value-123456789"
	writeTestFile(t, root, "package.json", `{"name":"safe","scripts":{"build":"NPM_TOKEN=`+secret+` tsx src/main.ts"},"dependencies":{"safe-package":"git+https://user:`+secret+`@example.invalid/repo.git"},"devDependencies":{"typescript":"5.9.3"}}`)
	listing := gitfiles.Listing{Paths: []string{"package.json", "src/main.ts", "tsconfig.json"}, RegularPaths: []string{"package.json", "src/main.ts", "tsconfig.json"}}
	repository, err := corpus.New(context.Background(), root, listing)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	result, err := Discover(context.Background(), repository, root)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := jsonMarshal(result)
	if err != nil {
		t.Fatal(err)
	}
	request := newHelperRequest(
		[]helperCompilerPackage{{Name: "typescript", ResolutionBase: helperCompilerResolutionProject}},
		nil,
	)
	request.Files = append(request.Files, helperFile{Path: "src/main.ts", FileRef: "f2"})
	requestBytes, err := jsonMarshal(request)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte(secret)) || bytes.Contains(requestBytes, []byte(secret)) {
		t.Fatal("credential-bearing manifest value entered helper input or result")
	}
}

func TestHelperRequestEncodesEmptyPackageBoundariesAsArray(t *testing.T) {
	request := newHelperRequest(
		[]helperCompilerPackage{{Name: "typescript", ResolutionBase: helperCompilerResolutionProject}},
		nil,
	)
	encoded, err := jsonMarshal(request)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range [][]byte{
		[]byte(`"package_boundary_dirs":[]`),
		[]byte(`"package_boundaries":[]`),
		[]byte(`"files":[]`),
		[]byte(`"additional_files":[]`),
	} {
		if !bytes.Contains(encoded, want) {
			t.Fatalf("helper request omitted canonical empty array %s: %s", want, encoded)
		}
	}
}

func TestDiscoverSelectsOneNestedPreparedPackageAndKeepsRepositoryPaths(t *testing.T) {
	preparedRoot := preparedTempProject(t)
	repositoryRoot := t.TempDir()
	projectRoot := filepath.Join(repositoryRoot, "front")
	if err := os.Rename(preparedRoot, projectRoot); err != nil {
		t.Fatal(err)
	}
	projectFiles := []string{
		"package.json", "postcss.config.js", "src/excluded.ts", "src/main.ts",
		"src/lookalike.js", "src/one.ts", "src/two.ts", "src/view.tsx", "src/widget.jsx", "tsconfig.json",
	}
	tracked := make([]string, len(projectFiles))
	for index, filePath := range projectFiles {
		tracked[index] = path.Join("front", filePath)
	}
	repository, err := corpus.New(context.Background(), repositoryRoot, gitfiles.Listing{Paths: tracked, RegularPaths: tracked})
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()

	result, index, catalog, err := Build(context.Background(), repository, repositoryRoot)
	if err != nil {
		t.Fatal(err)
	}
	if result.Project.ManifestPath != "front/package.json" || result.Project.Selector != "jsts:front/package.json" ||
		result.Project.ConfigPath != "front/tsconfig.json" || result.Project.BaseURL != "front" {
		t.Fatalf("nested project identity = %#v", result.Project)
	}
	if len(result.Project.PathAliases) != 1 || strings.Join(result.Project.PathAliases[0].Targets, ",") != "front/src/*" {
		t.Fatalf("nested path aliases = %#v", result.Project.PathAliases)
	}
	for _, file := range result.Files {
		if !strings.HasPrefix(file.Path, "front/") {
			t.Fatalf("nested source escaped repository-relative package path: %#v", file)
		}
	}
	if !hasExactImport(result, "@/one", "front/src/one.ts") || !hasExactImport(result, "@/two", "front/src/two.ts") {
		t.Fatalf("nested project imports lost exact authority: %#v", result.Imports)
	}
	if index.Target.Selector != result.Project.Selector || index.Target.AnchorFileRef != result.Project.ManifestFileRef {
		t.Fatalf("nested ProgramTarget binding = %#v", index.Target)
	}
	if len(catalog.Importers) != 1 || catalog.Importers[0].RepositoryPath != "front" {
		t.Fatalf("nested dependency importer = %#v", catalog.Importers)
	}
}

func TestNestedPackageSharedRepositoryToolDoesNotBecomePackageSource(t *testing.T) {
	repositoryRoot := preparedCompilerProject(t)
	files := map[string]string{
		"package.json": `{
  "name":"workspace",
  "private":true,
  "devDependencies":{"typescript":"5.9.3"}
}`,
		"config/typescript/tsconfig.node.json": `{
  "compilerOptions":{"module":"ESNext","moduleResolution":"bundler","strict":true}
}`,
		"packages/preset/package.json": `{
  "name":"@sample/preset",
  "scripts":{
    "build":"tsx ../../scripts/build-package.ts --pkg-name preset",
    "dev":"tsx ../../scripts/build-package.ts --pkg-name preset --watch",
    "migrate":"tsx tools/migrate.ts"
  },
  "devDependencies":{"typescript":"5.9.3"}
}`,
		"packages/preset/src/index.ts":     "export const preset = true\n",
		"packages/preset/tools/migrate.ts": "export const migrate = true\n",
		"packages/preset/tsconfig.json": `{
  "extends":"../../config/typescript/tsconfig.node.json",
  "include":["src/**/*"]
}`,
		"scripts/build-package.ts": "export const buildPackage = true\n",
	}
	tracked := make([]string, 0, len(files))
	for filePath, content := range files {
		writeTestFile(t, repositoryRoot, filePath, content)
		tracked = append(tracked, filePath)
	}
	sort.Strings(tracked)
	repository, err := corpus.New(
		context.Background(), repositoryRoot,
		gitfiles.Listing{Paths: tracked, RegularPaths: tracked},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()

	result, err := DiscoverSelected(
		context.Background(), repository, repositoryRoot,
		"jsts:packages/preset/package.json",
	)
	if err != nil {
		t.Fatal(err)
	}
	wantFiles := []string{
		"packages/preset/src/index.ts",
		"packages/preset/tools/migrate.ts",
	}
	gotFiles := make([]string, len(result.Files))
	for index, file := range result.Files {
		gotFiles[index] = file.Path
	}
	if strings.Join(gotFiles, "\n") != strings.Join(wantFiles, "\n") {
		t.Fatalf("selected package files = %#v", result.Files)
	}
	scriptRefs := map[string][]string{}
	for _, script := range result.Project.Scripts {
		scriptRefs[script.Name] = script.EntryFileRefs
	}
	if len(scriptRefs["build"]) != 0 || len(scriptRefs["dev"]) != 0 {
		t.Fatalf("shared repository tool gained package entry authority: %#v", scriptRefs)
	}
	if len(scriptRefs["migrate"]) != 1 {
		t.Fatalf("package-owned additional source lost script authority: %#v", scriptRefs)
	}
}

func TestNestedProjectUsesPreparedCompilerOnlyFromAnalyzedRepository(t *testing.T) {
	repositoryRoot := preparedTempProject(t)
	projectRoot := filepath.Join(repositoryRoot, "front")
	if err := os.MkdirAll(projectRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, filePath := range []string{"package.json", "postcss.config.js", "src", "tsconfig.json"} {
		if err := os.Rename(filepath.Join(repositoryRoot, filePath), filepath.Join(projectRoot, filePath)); err != nil {
			t.Fatal(err)
		}
	}
	writeTestFile(t, repositoryRoot, "package.json", `{
  "name":"workspace",
  "workspaces":["front"],
  "scripts":{"dev":"bun run --cwd front src/main.ts"}
}`)
	projectFiles := []string{
		"package.json", "postcss.config.js", "src/excluded.ts", "src/main.ts",
		"src/lookalike.js", "src/one.ts", "src/two.ts", "src/view.tsx", "src/widget.jsx", "tsconfig.json",
	}
	tracked := []string{"package.json"}
	for _, filePath := range projectFiles {
		tracked = append(tracked, path.Join("front", filePath))
	}
	repository, err := corpus.New(context.Background(), repositoryRoot, gitfiles.Listing{Paths: tracked, RegularPaths: tracked})
	if err != nil {
		t.Fatal(err)
	}
	result, _, _, err := Build(context.Background(), repository, repositoryRoot)
	if closeErr := repository.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	if err != nil {
		t.Fatal(err)
	}
	if result.Project.ManifestPath != "front/package.json" || len(result.Files) == 0 {
		t.Fatalf("workspace-root compiler project = %#v; files = %d", result.Project, len(result.Files))
	}

	// The same prepared compiler is deliberately outside this second analyzed
	// repository. Node package lookup can see it through the parent directory,
	// but the helper must reject that authority.
	nestedRepositoryRoot := filepath.Join(repositoryRoot, "outside-check")
	files := map[string]string{
		"package.json":        `{"name":"outside-check","workspaces":["front"],"scripts":{"dev":"bun run --cwd front src/main.ts"}}`,
		"front/package.json":  `{"name":"front","devDependencies":{"typescript":"5.9.3"}}`,
		"front/src/main.ts":   "export const ready = true\n",
		"front/tsconfig.json": `{"include":["src/**/*.ts"],"compilerOptions":{"module":"ESNext","moduleResolution":"bundler"}}`,
	}
	nestedTracked := make([]string, 0, len(files))
	for filePath, content := range files {
		writeTestFile(t, nestedRepositoryRoot, filePath, content)
		nestedTracked = append(nestedTracked, filePath)
	}
	sort.Strings(nestedTracked)
	nestedRepository, err := corpus.New(
		context.Background(),
		nestedRepositoryRoot,
		gitfiles.Listing{Paths: nestedTracked, RegularPaths: nestedTracked},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer nestedRepository.Close()
	_, err = Discover(context.Background(), nestedRepository, nestedRepositoryRoot)
	if !errors.Is(err, ErrTypeScriptCompilerUnavailable) {
		t.Fatalf("outside-repository compiler authority error = %v", err)
	}
}

func TestDiscoverRejectsAmbiguousNestedPackagesBeforeCompilerExecution(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"admin/package.json": `{"name":"admin"}`,
		"admin/main.ts":      "export {}\n",
		"web/package.json":   `{"name":"web"}`,
		"web/main.ts":        "export {}\n",
	}
	tracked := make([]string, 0, len(files))
	for filePath, content := range files {
		writeTestFile(t, root, filePath, content)
		tracked = append(tracked, filePath)
	}
	sort.Strings(tracked)
	repository, err := corpus.New(context.Background(), root, gitfiles.Listing{Paths: tracked, RegularPaths: tracked})
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	_, err = Discover(context.Background(), repository, root)
	if err == nil || !strings.Contains(err.Error(), "multiple nested package projects are ambiguous") {
		t.Fatalf("ambiguous nested discovery error = %v", err)
	}
}

func TestProjectManifestSelectionKeepsWorkspaceRootWithExactOwnedEntry(t *testing.T) {
	manifestPath, projectDir, err := selectProjectManifest([]corpus.Entry{
		{Path: "package.json"},
		{Path: "src/main.ts"},
		{Path: "front/package.json"},
		{Path: "front/src/main.ts"},
	}, "", &packageManifest{
		Workspaces: json.RawMessage(`["front"]`),
		Exports:    json.RawMessage(`"./src/main.ts"`),
		Scripts:    map[string]string{"dev": "bun run --cwd front src/main.ts"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if manifestPath != "package.json" || projectDir != "." {
		t.Fatalf("selected project = %q at %q, want repository root", manifestPath, projectDir)
	}
}

func TestPackageBinaryFactsKeepOnlyCanonicalTrackedOwnedPairs(t *testing.T) {
	owned := map[string]string{
		"bin/opencode": "f2",
		"bin/other":    "f3",
	}
	binaries := packageBinaryFacts(
		"@scope/opencode",
		json.RawMessage(`{
  "opencode": "./bin/opencode",
  "alias": "bin/opencode",
  "escape": "../outside",
  "missing": "bin/missing",
  "generated": "dist/opencode",
  "invalid": {"path":"bin/other"}
}`),
		"packages/opencode",
		owned,
	)
	if len(binaries) != 2 ||
		binaries[0] != (PackageBinary{Command: "alias", Path: "packages/opencode/bin/opencode", FileRef: "f2"}) ||
		binaries[1] != (PackageBinary{Command: "opencode", Path: "packages/opencode/bin/opencode", FileRef: "f2"}) {
		t.Fatalf("package binary facts = %#v", binaries)
	}

	stringForm := packageBinaryFacts(
		"@scope/opencode", json.RawMessage(`"./bin/opencode"`), ".", owned,
	)
	if len(stringForm) != 1 || stringForm[0] != (PackageBinary{Command: "opencode", Path: "bin/opencode", FileRef: "f2"}) {
		t.Fatalf("string package binary facts = %#v", stringForm)
	}
}

func TestPackageBinaryCandidatesDiscardConflictingCommandWithoutDiscardingValidSibling(t *testing.T) {
	candidates := packageBinaryCandidates(
		"sample",
		json.RawMessage(`{"tool":"bin/one","stable":"bin/other","tool":"bin/two"}`),
	)
	if len(candidates) != 1 || candidates[0] != (packageBinaryCandidate{Command: "stable", Path: "bin/other"}) {
		t.Fatalf("conflicting package binary candidates = %#v", candidates)
	}
}

func TestProjectManifestSelectionKeepsWorkspaceRootWithTrackedExtensionlessBinary(t *testing.T) {
	manifestPath, projectDir, err := selectProjectManifest([]corpus.Entry{
		{Path: "package.json"},
		{Path: "bin/tool"},
		{Path: "src/main.ts"},
		{Path: "front/package.json"},
		{Path: "front/src/main.ts"},
	}, "", &packageManifest{
		Name:       "tool",
		Workspaces: json.RawMessage(`["front"]`),
		Bin:        json.RawMessage(`"./bin/tool"`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if manifestPath != "package.json" || projectDir != "." {
		t.Fatalf("selected extensionless-bin project = %q at %q, want repository root", manifestPath, projectDir)
	}
}

func TestProjectManifestSelectionUsesOneExactWorkspaceDelegate(t *testing.T) {
	entries := []corpus.Entry{
		{Path: "package.json"},
		{Path: "src/index.ts"},
		{Path: "front/package.json"},
		{Path: "front/src/main.ts"},
	}
	manifestPath, projectDir, err := selectProjectManifest(entries, "", &packageManifest{
		Workspaces: json.RawMessage(`["front"]`),
		Scripts: map[string]string{
			"dev": "bun run --cwd front --conditions=browser src/index.ts",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if manifestPath != "front/package.json" || projectDir != "front" {
		t.Fatalf("selected delegated project = %q at %q, want front", manifestPath, projectDir)
	}
}

func TestProjectManifestSelectionFailsClosedWithoutOneWorkspaceDelegate(t *testing.T) {
	entries := []corpus.Entry{
		{Path: "package.json"},
		{Path: "admin/package.json"},
		{Path: "admin/src/main.ts"},
		{Path: "front/package.json"},
		{Path: "front/src/main.ts"},
	}
	for _, test := range []struct {
		name    string
		scripts map[string]string
		count   string
	}{
		{name: "zero", scripts: map[string]string{"dev": "bun run build"}, count: "has 0 exact"},
		{name: "multiple", scripts: map[string]string{
			"dev":   "bun run --cwd front dev",
			"start": "bun run --cwd=admin start",
		}, count: "has 2 exact"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := selectProjectManifest(entries, "", &packageManifest{
				Workspaces: json.RawMessage(`["admin","front"]`),
				Scripts:    test.scripts,
			})
			if err == nil || !strings.Contains(err.Error(), test.count) ||
				!strings.Contains(err.Error(), "jsts:admin/package.json") ||
				!strings.Contains(err.Error(), "jsts:front/package.json") {
				t.Fatalf("workspace delegation error = %v", err)
			}
		})
	}
}

func TestProjectManifestExactSelectorBypassesWorkspaceRootPriority(t *testing.T) {
	manifestPath, projectDir, err := selectProjectManifest([]corpus.Entry{
		{Path: "package.json"},
		{Path: "src/main.ts"},
		{Path: "front/package.json"},
		{Path: "front/src/main.ts"},
	}, "jsts:front/package.json", nil)
	if err != nil {
		t.Fatal(err)
	}
	if manifestPath != "front/package.json" || projectDir != "front" {
		t.Fatalf("exact selected project = %q at %q, want front", manifestPath, projectDir)
	}
}

func TestPackageProjectOwnershipUsesPathDepthNotDirectoryNameLength(t *testing.T) {
	candidates := packageProjectCandidates([]corpus.Entry{
		{Path: "package.json"},
		{Path: "z/package.json"},
		{Path: "z/main.ts"},
	})
	if len(candidates) != 2 {
		t.Fatalf("package candidates = %#v", candidates)
	}
	for _, candidate := range candidates {
		switch candidate.manifestPath {
		case "package.json":
			if len(candidate.ownSources) != 0 {
				t.Fatalf("root stole nested source ownership: %#v", candidate.ownSources)
			}
		case "z/package.json":
			if _, ok := candidate.ownSources["z/main.ts"]; !ok {
				t.Fatalf("nested package lost source ownership: %#v", candidate.ownSources)
			}
		}
	}
}

var (
	preparedCompilerOnce   sync.Once
	preparedCompilerSource string
	preparedCompilerErr    error
)

func preparedCompilerPackage() (string, error) {
	preparedCompilerOnce.Do(func() {
		npm, err := exec.LookPath("npm")
		if err != nil {
			preparedCompilerErr = fmt.Errorf("npm is unavailable: %w", err)
			return
		}
		output, err := exec.Command(npm, "root", "-g").Output()
		if err != nil {
			preparedCompilerErr = fmt.Errorf("global npm root is unavailable: %w", err)
			return
		}
		preparedCompilerSource = filepath.Join(strings.TrimSpace(string(output)), "typescript")
		if _, err := os.Stat(filepath.Join(preparedCompilerSource, "lib", "typescript.js")); err != nil {
			preparedCompilerErr = fmt.Errorf("global TypeScript compiler is unavailable: %w", err)
		}
	})
	return preparedCompilerSource, preparedCompilerErr
}

func preparedCompilerProject(t *testing.T) string {
	t.Helper()
	compilerSource, err := preparedCompilerPackage()
	if err != nil {
		t.Skip(err)
	}
	root := t.TempDir()
	compilerTarget := filepath.Join(root, "node_modules", "typescript")
	if err := materializeCompilerTree(compilerSource, compilerTarget); err != nil {
		t.Fatal(err)
	}
	return root
}

func materializeCumulativeJSTSDependencyTypes(t *testing.T, root string) {
	t.Helper()
	writeTestFile(t, root, "node_modules/axios/package.json", `{"name":"axios","version":"1.0.0","types":"index.d.ts"}`)
	writeTestFile(t, root, "node_modules/axios/index.d.ts", `
export interface AxiosResponse<T> { data: T }
declare const axios: {
  get<T = unknown>(url: string): Promise<AxiosResponse<T>>
  post<T = unknown>(url: string, payload: unknown): Promise<AxiosResponse<T>>
  delete<T = unknown>(url: string): Promise<AxiosResponse<T>>
}
export default axios
`)
	writeTestFile(t, root, "node_modules/express/package.json", `{"name":"express","version":"4.21.2","types":"index.d.ts"}`)
	writeTestFile(t, root, "node_modules/express/index.d.ts", `
export interface Request {}
export interface Response { json(value: unknown): void }
export interface Application {
	  get(path: string, ...handlers: Array<(request: Request, response: Response) => void | Promise<void>>): void
	  listen(port: number): void
	}
export default function express(): Application
`)
	writeTestFile(t, root, "node_modules/@fixture/kafka-client/package.json", `{"name":"@fixture/kafka-client","version":"1.0.0","types":"index.d.ts"}`)
	writeTestFile(t, root, "node_modules/@fixture/kafka-client/index.d.ts", `
export interface Consumer {
  subscribe(topic: string, handler: (event: any) => void): void
}
export function createConsumer(): Consumer
`)
	writeTestFile(t, root, "node_modules/@tanstack/react-query/package.json", `{"name":"@tanstack/react-query","version":"5.0.0","types":"index.d.ts"}`)
	writeTestFile(t, root, "node_modules/@tanstack/react-query/index.d.ts", `
export interface QueryOptions {
  queryKey: readonly unknown[]
}
export function useQuery(options: QueryOptions): unknown
`)
	writeTestFile(t, root, "node_modules/wouter/package.json", `{"name":"wouter","version":"3.7.1","types":"index.d.ts"}`)
	writeTestFile(t, root, "node_modules/wouter/index.d.ts", `
export interface RouteProps {
  path?: string
  component?: unknown
}
export function Route(props: RouteProps): unknown
`)
}

func materializeCompilerTree(source, target string) error {
	return filepath.WalkDir(source, func(sourcePath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, sourcePath)
		if err != nil {
			return err
		}
		targetPath := filepath.Join(target, relative)
		if entry.IsDir() {
			return os.MkdirAll(targetPath, 0o755)
		}
		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
			return fmt.Errorf("prepared TypeScript compiler contains unsupported file %q", relative)
		}
		if err := os.Link(sourcePath, targetPath); err == nil {
			return nil
		}
		contents, err := os.ReadFile(sourcePath)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		return os.WriteFile(targetPath, contents, info.Mode().Perm())
	})
}

func preparedTempProject(t *testing.T) string {
	t.Helper()
	root := preparedCompilerProject(t)
	writeTestFile(t, root, "package.json", `{"name":"sample","devDependencies":{"typescript":"5.9.3"}}`)
	writeTestFile(t, root, "tsconfig.json", `{"include":["src/**/*"],"exclude":["src/excluded.ts"],"compilerOptions":{"allowJs":true,"module":"ESNext","moduleResolution":"bundler","jsx":"preserve","baseUrl":".","paths":{"@/*":["./src/*"]},"strict":true}}`)
	var collisions strings.Builder
	for index := 0; index <= programindex.MaxTargetsPerRelation; index++ {
		_, _ = fmt.Fprintf(&collisions, "export namespace Candidate%d { export function test() {} }\n", index)
	}
	writeTestFile(t, root, "src/collisions.ts", collisions.String())
	writeTestFile(t, root, "src/main.ts", "import { same as one } from \"@/one\"\nimport { same as two } from \"@/two\"\nexport class ExactReceiver { statusCode(): number { return 201 } }\nexport class Constructed { constructor() {} }\nexport function createResponse() { return { statusCode(): number { return 200 } }\nexport const response = { get statusCode(): number { return 202 } }\nexport function hello(): string { return \"hello\" }\nexport function platformCalls(canvas: HTMLCanvasElement) {\n  new Constructed()\n  new Date()\n  new Promise<void>((resolve) => resolve())\n  const localDateConstructor: DateConstructor = Date\n  new localDateConstructor()\n  canvas.getContext(\"2d\")\n  Math.min(1, 2)\n  console.log(\"ready\")\n  return new Image()\n}\nhello()\none()\ntwo()\nexport const prompt = ["+strings.Repeat("\"abc\", ", 100)+"\"end\"].join(\"\\n\")\n")
	writeTestFile(t, root, "node_modules/@types/node/index.d.ts", "interface Console { log(message?: any, ...optionalParams: any[]): void }\n")
	writeTestFile(t, root, "src/one.ts", "export function same(): number { return 1 }\n")
	writeTestFile(t, root, "src/two.ts", "export function same(): number { return 2 }\n")
	writeTestFile(t, root, "src/view.tsx", "export function View() { return <main /> }\n")
	writeTestFile(t, root, "src/widget.jsx", "export function Widget() { return <aside /> }\nWidget()\n")
	writeTestFile(t, root, "src/lookalike.js", "const app = { get() {}, listen() {} }\nfunction createRoot() {}\nfunction test() {}\napp.get(\"/not-express\", () => {})\napp.listen()\ncreateRoot()\n/robot/.test(\"robot\")\nconst dynamicName = \"./missing.js\"\nimport(dynamicName)\n")
	writeTestFile(t, root, "src/excluded.ts", "export function excluded(): never { throw new Error(\"excluded\") }\n")
	writeTestFile(t, root, "postcss.config.js", "export function plugin() {}\nplugin()\n")
	return root
}

func hasExactImport(result Result, specifier, resolvedPath string) bool {
	fileRef := ""
	for _, file := range result.Files {
		if file.Path == resolvedPath {
			fileRef = file.FileRef
			break
		}
	}
	for _, value := range result.Imports {
		if value.Specifier == specifier && value.Resolution == "exact" && value.ResolvedFileRef == fileRef && fileRef != "" {
			return true
		}
	}
	return false
}

func assertCompilerResolvedInvocationProjection(t *testing.T, result Result, index programindex.Index) {
	t.Helper()
	type expectedPlatformTarget struct {
		invocation string
		receiver   string
		name       string
	}
	wantPlatform := map[string]expectedPlatformTarget{
		"canvas.getContext": {invocation: "call", receiver: "HTMLCanvasElement", name: "getContext"},
		"Math.min":          {invocation: "call", receiver: "Math", name: "min"},
		"console.log":       {invocation: "call", receiver: "Console", name: "log"},
		"new Date":          {invocation: "construct", name: "Date"},
		"new Image":         {invocation: "construct", name: "Image"},
		"new Promise":       {invocation: "construct", name: "Promise"},
	}
	callsByExpression := make(map[string]Call, len(result.Calls))
	for _, call := range result.Calls {
		if !validInvocation(call.Invocation) {
			t.Fatalf("call has no closed invocation kind: %#v", call)
		}
		callsByExpression[call.Expression] = call
	}
	objectsByID := make(map[string]programindex.Object, len(index.Objects))
	for _, object := range index.Objects {
		objectsByID[object.ID] = object
	}
	relationsBySourceRef := make(map[string]programindex.Relation, len(index.Relations))
	for _, relation := range index.Relations {
		relationsBySourceRef[relation.SourceRef] = relation
	}
	for expression, want := range wantPlatform {
		call, ok := callsByExpression[expression]
		if !ok || call.Resolution != "exact" || call.Invocation != want.invocation || call.ExternalPackage != javascriptPlatform ||
			call.ExternalReceiver != want.receiver || call.ExternalName != want.name || len(call.CalleeRefs) != 0 {
			t.Fatalf("compiler-resolved platform invocation %q = %#v, want %#v", expression, call, want)
		}
		relation, ok := relationsBySourceRef["program:"+call.Ref]
		if !ok || relation.Kind != programindex.RelationInvokesExternal || relation.Resolution != programindex.ResolutionExact ||
			relation.Invocation != want.invocation || len(relation.ToIDs) != 1 {
			t.Fatalf("platform invocation relation %q = %#v", expression, relation)
		}
		target := objectsByID[relation.ToIDs[0]]
		if target.External == nil || target.External.AuthorityKind != programindex.ExternalAuthorityPlatform ||
			target.External.PackagePath != javascriptPlatform || target.External.Receiver != want.receiver || target.External.Name != want.name {
			t.Fatalf("platform invocation target %q = %#v", expression, target)
		}
	}
	if call, ok := callsByExpression["new localDateConstructor"]; !ok || call.Resolution != "unresolved" ||
		call.ExternalPackage != "" || len(call.CalleeRefs) != 0 {
		t.Fatalf("repository-local value typed as a platform constructor gained platform authority: %#v", call)
	}

	constructorCall, ok := callsByExpression["new Constructed"]
	if !ok || constructorCall.Invocation != "construct" || constructorCall.Resolution != "exact" || constructorCall.ExternalPackage != "" || len(constructorCall.CalleeRefs) != 1 {
		t.Fatalf("local constructor invocation = %#v", constructorCall)
	}
	declarationsByRef := make(map[string]Declaration, len(result.Declarations))
	for _, declaration := range result.Declarations {
		declarationsByRef[declaration.Ref] = declaration
	}
	constructor := declarationsByRef[constructorCall.CalleeRefs[0]]
	owner := declarationsByRef[constructor.OwnerRef]
	if constructor.Name != "constructor" || owner.Name != "Constructed" {
		t.Fatalf("local constructor target = %#v, owner %#v", constructor, owner)
	}
	relation := relationsBySourceRef["program:"+constructorCall.Ref]
	if relation.Kind != programindex.RelationCalls || relation.Resolution != programindex.ResolutionExact || relation.Invocation != "construct" || len(relation.ToIDs) != 1 {
		t.Fatalf("local constructor relation = %#v", relation)
	}
}

func assertExactSiblingPackageCalls(
	t *testing.T,
	result Result,
	index programindex.Index,
	packagePath string,
	exportsByExpression map[string]string,
) {
	t.Helper()
	for expression, exportName := range exportsByExpression {
		var call Call
		for _, candidate := range result.Calls {
			if candidate.Expression == expression {
				call = candidate
				break
			}
		}
		if call.Ref == "" || call.Resolution != "exact" || call.ExternalPackage != packagePath ||
			call.ExternalExport != exportName || call.ExternalName == "" || len(call.CalleeRefs) != 0 {
			t.Fatalf("exact sibling call %q = %#v, want package %q export %q", expression, call, packagePath, exportName)
		}
		var relation programindex.Relation
		for _, candidate := range index.Relations {
			if candidate.SourceRef == "program:"+call.Ref {
				relation = candidate
				break
			}
		}
		if relation.Kind != programindex.RelationInvokesExternal ||
			relation.Resolution != programindex.ResolutionExact || len(relation.ToIDs) != 1 {
			t.Fatalf("sibling call relation %q = %#v", expression, relation)
		}
		var target programindex.Object
		for _, object := range index.Objects {
			if object.ID == relation.ToIDs[0] {
				target = object
				break
			}
		}
		foundIdentity := false
		for _, identity := range target.SymbolLinkIdentities {
			if identity.Domain == "jsts_package_export_v1" && identity.Display == packagePath+"#"+exportName {
				foundIdentity = true
			}
		}
		if !foundIdentity {
			t.Fatalf("sibling call target %q lacks package/export identity: %#v", expression, target)
		}
	}
}

func assertReceiverlessObjectMethodProjection(t *testing.T, result Result, index programindex.Index) {
	t.Helper()
	objectsBySourceRef := make(map[string]programindex.Object, len(index.Objects))
	objectsByID := make(map[string]programindex.Object, len(index.Objects))
	for _, object := range index.Objects {
		objectsBySourceRef[object.SourceRef] = object
		objectsByID[object.ID] = object
	}
	classMethodFound := false
	receiverlessOwners := map[programindex.ObjectKind]bool{}
	for _, declaration := range result.Declarations {
		if declaration.Name != "statusCode" {
			continue
		}
		object, ok := objectsBySourceRef[declaration.Ref]
		if !ok {
			t.Fatalf("statusCode declaration %q is absent from ProgramIndex", declaration.Ref)
		}
		owner := objectsByID[object.OwnerID]
		switch owner.Kind {
		case programindex.ObjectType:
			classMethodFound = true
			if object.Kind != programindex.ObjectMethod {
				t.Fatalf("class method lost exact receiver authority: %#v / owner=%#v", object, owner)
			}
		case programindex.ObjectFunction:
			receiverlessOwners[owner.Kind] = true
			if object.Kind != programindex.ObjectFunction {
				t.Fatalf("object-literal callable invented method receiver authority: %#v / owner=%#v", object, owner)
			}
		case programindex.ObjectVariable:
			receiverlessOwners[owner.Kind] = true
			if object.Kind != programindex.ObjectFunction {
				t.Fatalf("object-literal callable invented method receiver authority: %#v / owner=%#v", object, owner)
			}
		}
	}
	if !classMethodFound || !receiverlessOwners[programindex.ObjectFunction] || !receiverlessOwners[programindex.ObjectVariable] {
		t.Fatalf("receiver projection coverage: class=%t receiverless owners=%#v", classMethodFound, receiverlessOwners)
	}
}

func writeTestFile(t *testing.T, root, filePath, content string) {
	t.Helper()
	absolute := filepath.Join(root, filepath.FromSlash(filePath))
	if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(absolute, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func jsonMarshal(value any) ([]byte, error) { return json.Marshal(value) }
