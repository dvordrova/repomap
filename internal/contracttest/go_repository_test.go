package contracttest

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/gocoreobject"
	"github.com/dvordrova/repomap/internal/godynamichandoff"
	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/programindex/goadapter"
	"github.com/dvordrova/repomap/internal/snapshot"
	"github.com/dvordrova/repomap/internal/surfacediscovery"
)

const (
	goFixtureRootPackage             = "example.com/repomap/cumulative-go-fixture"
	goFixtureAppPackage              = goFixtureRootPackage + "/cmd/app"
	goFixturePublishedExamplePackage = "example.com/repomap/cumulative-go-published-example"
)

type goFixtureAuthorities struct {
	target   analysistarget.Target
	origins  []gofacts.PackageOrigin
	direct   surfacediscovery.DirectCallIndex
	external surfacediscovery.ExternalCallIndex
	core     gocoreobject.Index
	dynamic  godynamichandoff.Index
}

func TestCumulativeGoRepositoryDiscoveryAndProgramIndexContract(t *testing.T) {
	t.Setenv("CGO_ENABLED", "0")
	t.Setenv("GOTOOLCHAIN", "local")
	t.Setenv("GOWORK", "off")
	repositoryPath, repository := materializeFixtureRepository(t, "go")
	writePublishedGoFixtureModule(t, repositoryPath)
	authorities := analyzeGoFixture(t, repositoryPath, repository, goFixtureAppPackage, "cumulative-go-contract")
	assertUnusedPrivateMethodHasNoDanglingDirectNode(t, authorities)
	unretainedProducerResultID := assertGoUnretainedProducerReceiverAuthority(t, authorities)

	index, err := goadapter.Build(
		repository,
		authorities.target,
		authorities.origins,
		authorities.direct,
		authorities.external,
		authorities.core,
		authorities.dynamic,
	)
	if err != nil {
		t.Fatalf("build Go ProgramIndex: %v", err)
	}
	assertProgramIndexRoundTrip(t, index)
	if index.Target.Language != "go" || index.Target.Selector == "" || index.Target.Name != goFixtureAppPackage {
		t.Fatalf("Go ProgramIndex target = %#v", index.Target)
	}
	if !programIndexHasObject(index, programindex.ObjectMethod, "recreateStore") {
		t.Fatal("Go ProgramIndex omitted unused private method recreateStore")
	}
	assertGoNeutralBoundaryPatterns(t, index)
	assertGoExternalEventAndStoragePatterns(t, index)
	assertGoChainedCallAndCallbackTraversal(t, index)
	assertGoUnretainedCallbackSourceArgument(t, index)
	assertGoUnretainedProducerReceiverProjection(t, index, unretainedProducerResultID)

	publishedAuthorities := analyzeGoFixture(
		t,
		repositoryPath,
		repository,
		goFixturePublishedExamplePackage,
		"cumulative-go-published-contract",
	)
	publishedIndex, err := goadapter.Build(
		repository,
		publishedAuthorities.target,
		publishedAuthorities.origins,
		publishedAuthorities.direct,
		publishedAuthorities.external,
		publishedAuthorities.core,
		publishedAuthorities.dynamic,
	)
	if err != nil {
		t.Fatalf("build nested-module Go ProgramIndex: %v", err)
	}
	assertProgramIndexRoundTrip(t, publishedIndex)
	assertPublishedRootImportRemainsExternal(t, publishedAuthorities, publishedIndex)
}

func assertGoUnretainedCallbackSourceArgument(t *testing.T, index programindex.Index) {
	t.Helper()
	const sourcePath = "cmd/app/main.go"
	const callLine = 77
	foundTransfer := false
	for _, relation := range index.Relations {
		if relation.Location == nil || relation.Location.Path != sourcePath ||
			relation.Location.Line != callLine {
			continue
		}
		if relation.Kind == programindex.RelationCalls {
			t.Fatalf("unreachable callback owner unexpectedly entered the target-root direct graph: %#v", relation)
		}
		if relation.Kind != programindex.RelationPassesCallback {
			continue
		}
		foundTransfer = true
		if relation.Resolution != programindex.ResolutionExact || len(relation.ToIDs) != 1 ||
			relation.Invocation != "callback_transfer:synchronous" || relation.SourceArgumentID != "" ||
			relation.TargetsObserved != 1 || relation.TargetsOmitted != 0 {
			t.Fatalf("unretained-owner callback transfer = %#v", relation)
		}
	}
	if !foundTransfer {
		t.Fatal("ProgramIndex omitted the exact callback transfer whose owning call is outside direct traversal")
	}
}

func assertGoUnretainedProducerReceiverAuthority(
	t *testing.T,
	authorities goFixtureAuthorities,
) string {
	t.Helper()
	const sourcePath = "cmd/app/main.go"
	const consumerLine = 69
	resultID := ""
	for _, family := range authorities.external.Families {
		if family.Target.PackagePath != "net/http" || family.Target.Receiver != "HandlerFunc" ||
			family.Target.Name != "ServeHTTP" {
			continue
		}
		for _, pattern := range family.Patterns {
			if pattern.Callsite.Path != sourcePath || pattern.Callsite.Line != consumerLine {
				continue
			}
			if len(pattern.ReceiverResultIDs) != 1 || pattern.ReceiversObserved != 1 ||
				pattern.ReceiversOmitted != 0 {
				t.Fatalf("cumulative unretained-producer consumer authority = %#v", pattern)
			}
			resultID = pattern.ReceiverResultIDs[0]
		}
	}
	if resultID == "" {
		t.Fatal("cumulative Go fixture omitted the program-wide chained external consumer")
	}
	for _, edge := range authorities.direct.Edges {
		for _, pattern := range edge.Patterns {
			if pattern.ResultID == resultID {
				t.Fatalf("target-root direct graph unexpectedly retained the unreachable producer: %#v", pattern)
			}
		}
	}
	return resultID
}

func assertGoUnretainedProducerReceiverProjection(
	t *testing.T,
	index programindex.Index,
	resultID string,
) {
	t.Helper()
	const sourcePath = "cmd/app/main.go"
	const consumerLine = 69
	objects := make(map[string]programindex.Object, len(index.Objects))
	for _, object := range index.Objects {
		objects[object.ID] = object
		if object.SourceRef == resultID {
			t.Fatalf("unretained producer gained a synthetic call-result object: %#v", object)
		}
	}
	found := false
	for _, relation := range index.Relations {
		if relation.Kind != programindex.RelationInvokesExternal || len(relation.ToIDs) != 1 {
			continue
		}
		target := objects[relation.ToIDs[0]]
		if target.External == nil || target.External.PackagePath != "net/http" ||
			target.External.Receiver != "HandlerFunc" || target.External.Name != "ServeHTTP" {
			continue
		}
		if target.External.AuthorityKind != programindex.ExternalAuthorityPlatform {
			t.Fatalf("net/http receiver authority = %#v", target.External)
		}
		for _, pattern := range relation.Patterns {
			if pattern.Location == nil || pattern.Location.Path != sourcePath ||
				pattern.Location.Line != consumerLine {
				continue
			}
			found = true
			if pattern.ReceiverID != "" || len(pattern.ReceiverOriginIDs) != 0 ||
				pattern.ReceiverOriginResolution != programindex.ResolutionUnresolved ||
				pattern.ReceiverOriginsObserved != 1 || pattern.ReceiverOriginsOmitted != 1 {
				t.Fatalf("unretained producer receiver frontier = %#v", pattern)
			}
		}
	}
	if !found {
		t.Fatal("ProgramIndex omitted the chained external consumer with an unretained local producer")
	}
}

func assertGoChainedCallAndCallbackTraversal(t *testing.T, index programindex.Index) {
	t.Helper()
	objects := make(map[string]programindex.Object, len(index.Objects))
	objectIDs := make(map[string]string)
	for _, object := range index.Objects {
		objects[object.ID] = object
		objectIDs[object.Name] = object.ID
	}
	registerID := objectIDs["registerProductRoutes"]
	handleID := objectIDs["HandleFunc"]
	methodsID := objectIDs["Methods"]
	if registerID == "" || handleID == "" || methodsID == "" {
		t.Fatalf("cumulative chained-call objects = register:%q handle:%q methods:%q", registerID, handleID, methodsID)
	}
	var handlePatterns, methodsPatterns []programindex.RelationPattern
	for _, relation := range index.Relations {
		if relation.Kind != programindex.RelationCalls || relation.FromID != registerID ||
			relation.Resolution != programindex.ResolutionExact || len(relation.ToIDs) != 1 {
			continue
		}
		switch relation.ToIDs[0] {
		case handleID:
			handlePatterns = append(handlePatterns, relation.Patterns...)
		case methodsID:
			methodsPatterns = append(methodsPatterns, relation.Patterns...)
		}
	}
	if len(handlePatterns) != 5 || len(methodsPatterns) != 5 {
		t.Fatalf("cumulative chained patterns = HandleFunc:%d Methods:%d", len(handlePatterns), len(methodsPatterns))
	}
	handlerByPath := map[string]string{
		"/products": objectIDs["listProductsHandler"],
		"/product":  objectIDs["createProductHandler"],
	}
	resultByLine := make(map[int]string, len(handlePatterns))
	handlerArgumentIDs := make(map[string]string, len(handlePatterns))
	for _, pattern := range handlePatterns {
		if pattern.ResultID == "" || len(pattern.Arguments) != 2 ||
			pattern.Arguments[0].Kind != programindex.PatternLiteralString ||
			pattern.Arguments[1].Resolution != programindex.ResolutionExact ||
			len(pattern.Arguments[1].ObjectIDs) != 1 {
			t.Fatalf("cumulative HandleFunc result/callback pattern = %#v", pattern)
		}
		result := objects[pattern.ResultID]
		if result.Kind != programindex.ObjectVariable || result.Location == nil {
			t.Fatalf("cumulative HandleFunc result object = %#v", result)
		}
		resultByLine[result.Location.Line] = pattern.ResultID
		handlerArgumentIDs[pattern.Arguments[1].ObjectIDs[0]] = pattern.Arguments[1].ID
		if want := handlerByPath[pattern.Arguments[0].Value]; want != "" && pattern.Arguments[1].ObjectIDs[0] != want {
			t.Fatalf("cumulative HandleFunc handler for %q = %q, want %q", pattern.Arguments[0].Value, pattern.Arguments[1].ObjectIDs[0], want)
		}
	}
	methods := map[string]bool{"GET": true, "POST": true, "PUT": true, "DELETE": true}
	for _, pattern := range methodsPatterns {
		if pattern.ReceiverID == "" || len(pattern.Arguments) != 1 ||
			pattern.Arguments[0].Kind != programindex.PatternLiteralString || !methods[pattern.Arguments[0].Value] {
			t.Fatalf("cumulative Methods receiver/literal pattern = %#v", pattern)
		}
		result := objects[pattern.ReceiverID]
		if result.Location == nil || resultByLine[result.Location.Line] != pattern.ReceiverID {
			t.Fatalf("cumulative Methods receiver has no same-line HandleFunc result = %#v", pattern)
		}
	}
	callbackJoins := 0
	for _, relation := range index.Relations {
		if relation.Kind != programindex.RelationPassesCallback || relation.FromID != registerID ||
			relation.Resolution != programindex.ResolutionExact || len(relation.ToIDs) != 1 {
			continue
		}
		argumentID, expected := handlerArgumentIDs[relation.ToIDs[0]]
		if !expected {
			continue
		}
		callbackJoins++
		if relation.SourceArgumentID != argumentID {
			t.Fatalf("cumulative callback source argument = %q, want %q: %#v", relation.SourceArgumentID, argumentID, relation)
		}
	}
	if callbackJoins != len(handlerArgumentIDs) {
		t.Fatalf("cumulative callback source joins = %d, want %d", callbackJoins, len(handlerArgumentIDs))
	}
	for _, pair := range [][2]string{
		{"listProductsHandler", "listProducts"},
		{"createProductHandler", "createProduct"},
		{"getProductHandler", "getProduct"},
		{"updateProductHandler", "updateProduct"},
		{"deleteProductHandler", "deleteProduct"},
	} {
		fromID, toID := objectIDs[pair[0]], objectIDs[pair[1]]
		found := false
		for _, relation := range index.Relations {
			if relation.Kind == programindex.RelationCalls && relation.FromID == fromID &&
				relation.Resolution == programindex.ResolutionExact && len(relation.ToIDs) == 1 && relation.ToIDs[0] == toID {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("cumulative callback body call %s -> %s is absent", pair[0], pair[1])
		}
	}
}

func assertGoNeutralBoundaryPatterns(t *testing.T, index programindex.Index) {
	t.Helper()
	objects := make(map[string]programindex.Object, len(index.Objects))
	objectIDs := make(map[string]string)
	for _, object := range index.Objects {
		objects[object.ID] = object
		if object.Kind == programindex.ObjectFunction {
			objectIDs[object.Name] = object.ID
		}
	}
	handlerID := objectIDs["getLevel"]
	if handlerID == "" {
		t.Fatal("cumulative Go fixture omitted exact HTTP handler object")
	}
	relationFor := func(name string) programindex.Relation {
		t.Helper()
		for _, relation := range index.Relations {
			if relation.Kind != programindex.RelationInvokesExternal || len(relation.ToIDs) != 1 {
				continue
			}
			target := objects[relation.ToIDs[0]]
			if target.External != nil && target.External.PackagePath == "net/http" &&
				target.External.Name == name {
				if target.External.AuthorityKind != programindex.ExternalAuthorityPlatform {
					t.Fatalf("net/http.%s authority = %#v", name, target.External)
				}
				return relation
			}
		}
		t.Fatalf("cumulative Go fixture omitted net/http.%s", name)
		return programindex.Relation{}
	}
	client := relationFor("Get")
	if len(client.Patterns) != 1 || client.Patterns[0].Selector != "Get" ||
		len(client.Patterns[0].Arguments) != 1 ||
		client.Patterns[0].Arguments[0].Kind != programindex.PatternLiteralString ||
		client.Patterns[0].Arguments[0].Value != "/api/levels" {
		t.Fatalf("cumulative Go client pattern = %#v", client)
	}
	route := relationFor("HandleFunc")
	if len(route.Patterns) != 1 || len(route.Patterns[0].Arguments) != 2 ||
		route.Patterns[0].Arguments[0].Value != "/api/levels" ||
		route.Patterns[0].Arguments[1].Resolution != programindex.ResolutionExact ||
		len(route.Patterns[0].Arguments[1].ObjectIDs) != 1 ||
		route.Patterns[0].Arguments[1].ObjectIDs[0] != handlerID {
		t.Fatalf("cumulative Go route pattern = %#v", route)
	}
	bootstrap := relationFor("ListenAndServe")
	if len(bootstrap.Patterns) != 1 || len(bootstrap.Patterns[0].Arguments) != 2 ||
		bootstrap.Patterns[0].Arguments[0].Value != ":8080" ||
		bootstrap.Patterns[0].Arguments[1].Kind != programindex.PatternDynamic {
		t.Fatalf("cumulative Go bootstrap frontier pattern = %#v", bootstrap)
	}

	registerID := objectIDs["registerLevelConsumer"]
	subscribeID := objectIDs["Subscribe"]
	consumerID := objectIDs["consumeLevel"]
	if registerID == "" || subscribeID == "" || consumerID == "" {
		t.Fatalf("cumulative Go consumer objects = register:%q subscribe:%q consumer:%q",
			registerID, subscribeID, consumerID)
	}
	var subscribeCalls, callbackTransfers []programindex.Relation
	for _, relation := range index.Relations {
		if relation.FromID != registerID || relation.Resolution != programindex.ResolutionExact ||
			len(relation.ToIDs) != 1 {
			continue
		}
		switch {
		case relation.Kind == programindex.RelationCalls && relation.ToIDs[0] == subscribeID:
			subscribeCalls = append(subscribeCalls, relation)
		case relation.Kind == programindex.RelationPassesCallback && relation.ToIDs[0] == consumerID:
			callbackTransfers = append(callbackTransfers, relation)
		}
	}
	if len(subscribeCalls) != 1 {
		t.Fatalf("cumulative Go Subscribe calls = %#v", subscribeCalls)
	}
	subscribe := subscribeCalls[0]
	if subscribe.PatternsObserved != 1 || len(subscribe.Patterns) != 1 ||
		subscribe.Patterns[0].Selector != "Subscribe" || len(subscribe.Patterns[0].Arguments) != 2 ||
		subscribe.Patterns[0].Arguments[0].Kind != programindex.PatternLiteralString ||
		subscribe.Patterns[0].Arguments[0].Value != "levels.requested" ||
		subscribe.Patterns[0].Arguments[1].Kind != programindex.PatternDynamic ||
		subscribe.Patterns[0].Arguments[1].Resolution != programindex.ResolutionExact ||
		len(subscribe.Patterns[0].Arguments[1].ObjectIDs) != 1 ||
		subscribe.Patterns[0].Arguments[1].ObjectIDs[0] != consumerID {
		t.Fatalf("cumulative Go Subscribe neutral pattern = %#v", subscribe)
	}
	if len(callbackTransfers) != 1 || callbackTransfers[0].Invocation != "callback_transfer:synchronous" ||
		callbackTransfers[0].SourceArgumentID != subscribe.Patterns[0].Arguments[1].ID ||
		callbackTransfers[0].TargetsObserved != 1 || callbackTransfers[0].TargetsOmitted != 0 {
		t.Fatalf("cumulative Go callback transfer = %#v", callbackTransfers)
	}
}

func assertGoExternalEventAndStoragePatterns(
	t *testing.T,
	index programindex.Index,
) {
	t.Helper()
	objects := make(map[string]programindex.Object, len(index.Objects))
	for _, object := range index.Objects {
		objects[object.ID] = object
	}
	want := map[string]struct {
		packagePath string
		name        string
	}{
		"event":   {packagePath: "os/signal", name: "Notify"},
		"storage": {packagePath: "os", name: "Create"},
	}
	patternIDs := make(map[string]string, len(want))
	for _, relation := range index.Relations {
		if relation.Kind != programindex.RelationInvokesExternal || len(relation.ToIDs) != 1 {
			continue
		}
		target := objects[relation.ToIDs[0]]
		if target.External == nil {
			continue
		}
		for key, expected := range want {
			if target.External.PackagePath != expected.packagePath || target.External.Name != expected.name {
				continue
			}
			if target.External.AuthorityKind != programindex.ExternalAuthorityPlatform {
				t.Fatalf("cumulative Go %s platform authority = %#v", key, target.External)
			}
			if patternIDs[key] != "" {
				t.Fatalf("cumulative Go fixture duplicated %s external relation", key)
			}
			if relation.Resolution != programindex.ResolutionExact || relation.TargetsObserved != 1 ||
				relation.TargetsOmitted != 0 || relation.WitnessesObserved != 1 ||
				relation.WitnessesOmitted != 0 || len(relation.Patterns) != 1 {
				t.Fatalf("cumulative Go %s external authority = %#v", key, relation)
			}
			pattern := relation.Patterns[0]
			if pattern.Selector != expected.name || pattern.Form != programindex.PatternCall ||
				pattern.Location == nil || pattern.Location.Path != "internal/storefixture/fixtures.go" ||
				pattern.ArgumentsOmitted != 0 || pattern.ArgumentsObserved != len(pattern.Arguments) {
				t.Fatalf("cumulative Go %s neutral pattern = %#v", key, pattern)
			}
			switch key {
			case "event":
				if len(pattern.Arguments) != 2 ||
					pattern.Arguments[0].Kind != programindex.PatternDynamic ||
					pattern.Arguments[1].Kind != programindex.PatternDynamic {
					t.Fatalf("cumulative Go signal registration arguments = %#v", pattern.Arguments)
				}
			case "storage":
				if pattern.ArgumentsObserved != 1 || pattern.ArgumentsOmitted != 0 ||
					len(pattern.Arguments) != 1 ||
					pattern.Arguments[0].Kind != programindex.PatternLiteralString ||
					pattern.Arguments[0].Value != "fixture-state.db" {
					t.Fatalf("cumulative Go storage arguments = %#v", pattern.Arguments)
				}
			}
			patternIDs[key] = pattern.ID
		}
	}
	for key := range want {
		if patternIDs[key] == "" {
			t.Fatalf("cumulative Go fixture omitted exact external %s pattern", key)
		}
	}
}

func analyzeGoFixture(
	t *testing.T,
	repositoryPath string,
	repository *corpus.Corpus,
	packagePath string,
	_ string,
) goFixtureAuthorities {
	t.Helper()
	deferred, err := snapshot.BuildContext(t.Context(), snapshot.Options{
		RepoPath: repositoryPath, GoTarget: runtime.GOOS + "/" + runtime.GOARCH,
		RepositoryCorpus: repository,
	})
	if err != nil {
		t.Fatalf("build cumulative Go fixture target catalog: %v", err)
	}
	targetRef := ""
	if deferred.TargetCatalog != nil {
		for _, entry := range deferred.TargetCatalog.Entries {
			if entry.Candidate.Target.PackagePath == packagePath {
				targetRef = entry.Candidate.Target.Ref
				break
			}
		}
	}
	if targetRef == "" {
		t.Fatalf("cumulative Go fixture target %q is absent", packagePath)
	}
	scoped, err := snapshot.ScopeAnalysisTarget(deferred, targetRef)
	if err != nil {
		t.Fatalf("scope cumulative Go fixture target: %v", err)
	}
	input, err := goadapter.AnalysisInput(scoped.GoFacts, scoped.AnalysisTarget)
	if err != nil {
		t.Fatalf("bind cumulative Go fixture analysis input: %v", err)
	}
	options := surfacediscovery.DefaultOptions(repositoryPath, runtime.GOOS+"/"+runtime.GOARCH)
	options.CaptureExternalCallIndex = true
	options.CaptureCoreObjectIndex = true
	options.CaptureDynamicHandoffIndex = true
	result, err := surfacediscovery.AnalyzeContextWithInput(t.Context(), options, input)
	if err != nil {
		t.Fatalf("analyze cumulative Go fixture: %v", err)
	}
	if result.DirectCallIndex == nil || result.ExternalCallIndex == nil ||
		result.CoreObjectIndex == nil || result.DynamicHandoffIndex == nil {
		t.Fatalf("Go fixture analysis omitted producer authority: %#v", result)
	}
	authorities := goFixtureAuthorities{
		target:   scoped.AnalysisTarget.Snapshot(),
		origins:  append([]gofacts.PackageOrigin(nil), scoped.GoFacts.PackageOrigins...),
		direct:   result.DirectCallIndex.Snapshot(),
		external: result.ExternalCallIndex.Snapshot(),
		core:     result.CoreObjectIndex.Snapshot(),
		dynamic:  result.DynamicHandoffIndex.Snapshot(),
	}
	if authorities.target.Ref == "" || authorities.direct.SHA256 == "" ||
		authorities.external.SHA256 == "" || authorities.core.SHA256 == "" ||
		authorities.dynamic.SHA256 == "" {
		t.Fatalf("Go fixture analysis omitted producer authority: %#v", authorities)
	}
	return authorities
}

func writePublishedGoFixtureModule(t *testing.T, repositoryPath string) {
	t.Helper()
	publishedRoot := filepath.Join(filepath.Dir(repositoryPath), "published-root")
	if err := os.MkdirAll(publishedRoot, 0o700); err != nil {
		t.Fatalf("create published Go fixture module: %v", err)
	}
	files := map[string]string{
		"go.mod": "module " + goFixtureRootPackage + "\n\ngo 1.22\n",
		"root.go": `package cumulativegofixture

func PublishedRoot() string {
	return "published root"
}
`,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(publishedRoot, name), []byte(content), 0o600); err != nil {
			t.Fatalf("write published Go fixture module %s: %v", name, err)
		}
	}
}

func assertPublishedRootImportRemainsExternal(
	t *testing.T,
	authorities goFixtureAuthorities,
	index programindex.Index,
) {
	t.Helper()
	if authorities.target.PackagePath != goFixturePublishedExamplePackage ||
		authorities.target.ModuleDir != "_examples/published" {
		t.Fatalf("nested-module target = %#v", authorities.target)
	}
	for _, node := range authorities.direct.Nodes {
		if node.Package == goFixtureRootPackage {
			t.Fatalf("published root import became a repository direct-call node: %#v", node)
		}
	}
	foundExternalFamily := false
	for _, family := range authorities.external.Families {
		if family.Target.PackagePath == goFixtureRootPackage && family.Target.Name == "PublishedRoot" {
			foundExternalFamily = true
			break
		}
	}
	if !foundExternalFamily {
		t.Fatalf("published root import is absent from external-call authority: %#v", authorities.external.Families)
	}
	if index.Target.Language != "go" || index.Target.Name != goFixturePublishedExamplePackage ||
		index.Target.Selector != goFixturePublishedExamplePackage {
		t.Fatalf("nested-module ProgramIndex target = %#v", index.Target)
	}
	foundExternalObject := false
	for _, object := range index.Objects {
		if object.Kind == programindex.ObjectPackage && object.Name == goFixtureRootPackage {
			t.Fatalf("published root import became a local ProgramIndex package: %#v", object)
		}
		if object.Kind == programindex.ObjectExternalSymbol && object.External != nil &&
			object.External.PackagePath == goFixtureRootPackage && object.External.Name == "PublishedRoot" {
			if object.External.AuthorityKind != programindex.ExternalAuthorityPackage {
				t.Fatalf("published root import authority = %#v", object.External)
			}
			foundExternalObject = true
		}
	}
	if !foundExternalObject {
		t.Fatalf("published root import is absent from external ProgramIndex objects: %#v", index.Objects)
	}
}

func assertUnusedPrivateMethodHasNoDanglingDirectNode(t *testing.T, authorities goFixtureAuthorities) {
	t.Helper()
	var method gocoreobject.CallableDeclaration
	for _, callable := range authorities.core.Callables {
		if callable.Name == "recreateStore" && strings.HasSuffix(
			callable.Receiver, "/internal/storefixture.fileStoreTestBundle",
		) {
			method = callable
			break
		}
	}
	if method.ID == "" {
		t.Fatal("CoreObjectIndex omitted (*fileStoreTestBundle).recreateStore")
	}
	if method.DirectCallNodeID != "" {
		t.Fatalf("unused private method gained dangling direct-call node %q", method.DirectCallNodeID)
	}
	for _, node := range authorities.direct.Nodes {
		if node.Symbol.Name == "recreateStore" {
			t.Fatalf("unused private method unexpectedly entered DirectCallIndex: %#v", node)
		}
	}
}

func programIndexHasObject(index programindex.Index, kind programindex.ObjectKind, name string) bool {
	for _, object := range index.Objects {
		if object.Kind == kind && object.Name == name {
			return true
		}
	}
	return false
}
