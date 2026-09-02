package contracttest

import (
	"testing"

	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/programindex/adaptertest"
	"github.com/dvordrova/repomap/internal/pythondependencies"
	"github.com/dvordrova/repomap/internal/pythonprogramindex"
	"github.com/dvordrova/repomap/internal/pythontarget"
)

const pythonFixtureSelector = "python:.:script:repomap-fixture"

func TestCumulativePythonRepositoryDiscoveryAndProgramIndexContract(t *testing.T) {
	_, repository := materializeFixtureRepository(t, "python")
	catalog, err := pythontarget.Discover(t.Context(), repository)
	if err != nil {
		t.Fatalf("discover cumulative Python fixture: %v", err)
	}
	if err := catalog.Validate(); err != nil {
		t.Fatalf("validate cumulative Python target catalog: %v", err)
	}
	target := pythonFixtureTarget(t, catalog)
	indexes, err := pythonprogramindex.BuildMany(t.Context(), repository, []pythontarget.Target{target})
	if err != nil {
		t.Fatalf("build Python ProgramIndex: %v", err)
	}
	if len(indexes) != 1 {
		t.Fatalf("Python ProgramIndex count = %d, want one exact target", len(indexes))
	}
	index := indexes[0]
	assertProgramIndexRoundTrip(t, index)
	if index.Target.Language != "python" || index.Target.Selector != pythonFixtureSelector {
		t.Fatalf("Python ProgramIndex target = %#v", index.Target)
	}
	if len(index.Target.Seeds) != 1 || index.Target.Seeds[0].Kind != programindex.SeedCallable {
		t.Fatalf("Python script target seeds = %#v, want one exact callable seed", index.Target.Seeds)
	}
	seed := programIndexObjectByID(index, index.Target.Seeds[0].ObjectID)
	if seed.ID == "" || seed.Kind != programindex.ObjectFunction || seed.Name != "main" {
		t.Fatalf("Python script seed object = %#v, want exact main function", seed)
	}
	assertCumulativePythonSemanticFacts(t, index)

	fixturePackage := programIndexObjectNamed(
		t, index, programindex.ObjectPackage, "fixture_app", "src/fixture_app/__init__.py",
	)
	testsPackage := programIndexObjectNamed(
		t, index, programindex.ObjectModule, "tests.__init__", "tests/__init__.py",
	)
	consumer := programIndexObjectNamed(
		t, index, programindex.ObjectModule, "tests.test_facade", "tests/test_facade.py",
	)
	assertExactPythonImportBoundary(
		t, index, consumer.ID, fixturePackage.ID, "fixture_app.main",
	)
	assertExactPythonImportBoundary(
		t, index, consumer.ID, testsPackage.ID, "tests.runtime",
	)

	dependencyCatalog, err := pythondependencies.Build(index)
	if err != nil {
		t.Fatalf("build cumulative Python dependency catalog: %v", err)
	}
	if dependencyCatalog.Coverage.State != dependencies.CoverageComplete ||
		len(dependencyCatalog.Coverage.Omissions) != 0 {
		t.Fatalf("Python dependency coverage = %#v, want complete", dependencyCatalog.Coverage)
	}
}

func assertCumulativePythonSemanticFacts(t *testing.T, index programindex.Index) {
	t.Helper()
	main := programIndexObjectNamed(
		t, index, programindex.ObjectFunction, "main", "src/fixture_app/cli.py",
	)
	app := programIndexObjectNamed(
		t, index, programindex.ObjectVariable, "app", "src/fixture_app/cli.py",
	)
	getLevel := programIndexObjectNamed(
		t, index, programindex.ObjectFunction, "get_level", "src/fixture_app/cli.py",
	)
	dynamicLevel := programIndexObjectNamed(
		t, index, programindex.ObjectFunction, "dynamic_level", "src/fixture_app/cli.py",
	)
	dynamicPath := programIndexObjectNamed(
		t, index, programindex.ObjectVariable, "dynamic_path", "src/fixture_app/cli.py",
	)
	reassignedLevel := programIndexObjectNamed(
		t, index, programindex.ObjectFunction, "reassigned_level", "src/fixture_app/cli.py",
	)
	retrieveLevel := programIndexObjectNamed(
		t, index, programindex.ObjectFunction, "retrieve_level", "src/fixture_app/levels.py",
	)
	fetchLevel := programIndexObjectNamed(
		t, index, programindex.ObjectFunction, "fetch_level", "src/fixture_app/levels.py",
	)
	eventsModule := programIndexObjectNamed(
		t, index, programindex.ObjectModule, "fixture_app.events", "src/fixture_app/events.py",
	)
	consumer := programIndexObjectNamed(
		t, index, programindex.ObjectVariable, "consumer", "src/fixture_app/events.py",
	)
	handleOrder := programIndexObjectNamed(
		t, index, programindex.ObjectFunction, "handle_order", "src/fixture_app/events.py",
	)
	subscribeDynamic := programIndexObjectNamed(
		t, index, programindex.ObjectFunction, "subscribe_dynamic", "src/fixture_app/events.py",
	)
	subscribeDirect := programIndexObjectNamed(
		t, index, programindex.ObjectFunction, "subscribe_direct", "src/fixture_app/events.py",
	)
	runtimeConsumer := programIndexObjectNamed(
		t, index, programindex.ObjectVariable, "runtime_consumer", "src/fixture_app/events.py",
	)
	topic := programIndexObjectNamed(
		t, index, programindex.ObjectVariable, "topic", "src/fixture_app/events.py",
	)
	callback := programIndexObjectNamed(
		t, index, programindex.ObjectVariable, "callback", "src/fixture_app/events.py",
	)
	bindDuplicateCallbacks := programIndexObjectNamed(
		t, index, programindex.ObjectFunction, "bind_duplicate_callbacks", "src/fixture_app/events.py",
	)

	fastAPI := programIndexExternalObjectNamed(t, index, "fastapi.FastAPI")
	uvicornRun := programIndexExternalObjectNamed(t, index, "uvicorn.run")
	httpxGet := programIndexExternalObjectNamed(t, index, "httpx.get")
	kafkaConsumer := programIndexExternalObjectNamed(t, index, "kafka.KafkaConsumer")
	for _, external := range []programindex.Object{fastAPI, uvicornRun, httpxGet, kafkaConsumer} {
		if external.External == nil || external.External.AuthorityKind != programindex.ExternalAuthorityPackage {
			t.Fatalf("cumulative Python package authority = %#v", external)
		}
	}

	route := pythonRelation(
		t, index, programindex.RelationDecorates, getLevel.ID, "", programindex.ResolutionUnresolved,
	)
	routePattern := singlePythonPattern(t, route)
	if routePattern.Form != programindex.PatternDecoratorCall || routePattern.Selector != "get" ||
		routePattern.ReceiverID != app.ID ||
		routePattern.ReceiverOriginResolution != programindex.ResolutionAlternatives ||
		!sameSingleID(routePattern.ReceiverOriginIDs, fastAPI.ID) ||
		routePattern.ReceiverOriginsObserved != 1 || routePattern.ReceiverOriginsOmitted != 0 {
		t.Fatalf("cumulative Python HTTP decorator pattern = %#v", routePattern)
	}
	routePath := pythonPatternArgument(t, routePattern, 1)
	if routePath.Kind != programindex.PatternLiteralString ||
		routePath.Value != "/api/level/{level_id}" || routePath.ObjectsObserved != 0 ||
		len(routePath.ObjectIDs) != 0 {
		t.Fatalf("cumulative Python HTTP route path = %#v", routePath)
	}

	dynamicRoute := pythonRelation(
		t, index, programindex.RelationDecorates, dynamicLevel.ID, "", programindex.ResolutionUnresolved,
	)
	dynamicRoutePattern := singlePythonPattern(t, dynamicRoute)
	dynamicRoutePath := pythonPatternArgument(t, dynamicRoutePattern, 1)
	if dynamicRoutePattern.Selector != "get" || dynamicRoutePath.Kind != programindex.PatternDynamic ||
		dynamicRoutePath.Resolution != programindex.ResolutionAlternatives ||
		!sameSingleID(dynamicRoutePath.ObjectIDs, dynamicPath.ID) ||
		dynamicRoutePath.ValueCandidatesObserved != 1 || dynamicRoutePath.ValueCandidatesOmitted != 0 ||
		len(dynamicRoutePath.ValueCandidates) != 1 {
		t.Fatalf("cumulative Python initializer-backed route = %#v", dynamicRoutePattern)
	}
	dynamicValue := dynamicRoutePath.ValueCandidates[0]
	if dynamicValue.Kind != programindex.PatternLiteralString || dynamicValue.Value != "/api/dynamic" ||
		dynamicValue.Resolution != programindex.PatternValuePossible ||
		dynamicValue.SourceKind != programindex.PatternValueSourceInitializer ||
		!sameSingleID(dynamicValue.SourceObjectIDs, dynamicPath.ID) ||
		dynamicValue.SourceObjectsObserved != 1 || dynamicValue.SourceObjectsOmitted != 0 {
		t.Fatalf("cumulative Python initializer value = %#v", dynamicValue)
	}

	reassignedRoute := pythonRelation(
		t, index, programindex.RelationDecorates, reassignedLevel.ID, "", programindex.ResolutionUnresolved,
	)
	reassignedRoutePath := pythonPatternArgument(t, singlePythonPattern(t, reassignedRoute), 1)
	if reassignedRoutePath.Kind != programindex.PatternDynamic ||
		reassignedRoutePath.ValueCandidatesObserved != 0 || reassignedRoutePath.ValueCandidatesOmitted != 0 ||
		len(reassignedRoutePath.ValueCandidates) != 0 || len(reassignedRoutePath.ObjectIDs) != 1 {
		t.Fatalf("cumulative Python reassignment did not fail closed = %#v", reassignedRoutePath)
	}
	reassignedSource := programIndexObjectByID(index, reassignedRoutePath.ObjectIDs[0])
	if reassignedSource.ID == "" || reassignedSource.Kind != programindex.ObjectVariable ||
		reassignedSource.Name != "reassigned_path" || reassignedSource.Location == nil ||
		reassignedSource.Location.Path != "src/fixture_app/cli.py" || reassignedSource.Location.Line != 57 {
		t.Fatalf("cumulative Python reassigned source = %#v", reassignedSource)
	}

	bootstrap := pythonRelation(
		t, index, programindex.RelationInvokesExternal, main.ID, uvicornRun.ID,
		programindex.ResolutionAlternatives,
	)
	bootstrapPattern := singlePythonPattern(t, bootstrap)
	bootstrapApp := pythonPatternArgument(t, bootstrapPattern, 1)
	if bootstrapPattern.Form != programindex.PatternCall || bootstrapPattern.Selector != "run" ||
		bootstrapApp.Kind != programindex.PatternDynamic ||
		bootstrapApp.Resolution != programindex.ResolutionAlternatives ||
		!sameSingleID(bootstrapApp.ObjectIDs, app.ID) || bootstrapApp.ObjectsObserved != 1 ||
		bootstrapApp.ObjectsOmitted != 0 {
		t.Fatalf("cumulative Python server bootstrap pattern = %#v", bootstrapPattern)
	}

	assertPythonRelation(
		t, index, programindex.RelationCalls, getLevel.ID, retrieveLevel.ID,
		programindex.ResolutionAlternatives,
	)
	assertPythonRelation(
		t, index, programindex.RelationPassesCallback, getLevel.ID, fetchLevel.ID,
		programindex.ResolutionAlternatives,
	)
	assertPythonRelation(
		t, index, programindex.RelationCalls, handleOrder.ID, retrieveLevel.ID,
		programindex.ResolutionAlternatives,
	)
	assertPythonRelation(
		t, index, programindex.RelationPassesCallback, handleOrder.ID, fetchLevel.ID,
		programindex.ResolutionAlternatives,
	)

	loaderCall := pythonRelation(
		t, index, programindex.RelationCalls, retrieveLevel.ID, "", programindex.ResolutionUnresolved,
	)
	loaderPattern := singlePythonPattern(t, loaderCall)
	if loaderPattern.Selector != "loader" || len(loaderCall.ToIDs) != 0 ||
		loaderCall.TargetsObserved != 1 || loaderCall.TargetsOmitted != 1 {
		t.Fatalf("cumulative Python unresolved callback invocation = %#v", loaderCall)
	}

	outbound := pythonRelation(
		t, index, programindex.RelationInvokesExternal, fetchLevel.ID, httpxGet.ID,
		programindex.ResolutionAlternatives,
	)
	outboundPattern := singlePythonPattern(t, outbound)
	outboundURL := pythonPatternArgument(t, outboundPattern, 1)
	if outboundPattern.Selector != "get" || outboundURL.Kind != programindex.PatternStringTemplate ||
		len(outboundURL.Parts) != 2 ||
		outboundURL.Parts[0].Kind != programindex.PatternPartLiteral ||
		outboundURL.Parts[0].Text != "https://catalog.example/levels/" ||
		outboundURL.Parts[1].Kind != programindex.PatternPartHole {
		t.Fatalf("cumulative Python outbound HTTP pattern = %#v", outboundPattern)
	}

	subscription := pythonRelation(
		t, index, programindex.RelationCalls, eventsModule.ID, "", programindex.ResolutionUnresolved,
	)
	subscriptionPattern := singlePythonPattern(t, subscription)
	if subscriptionPattern.Selector != "subscribe" || subscriptionPattern.ReceiverID != consumer.ID ||
		subscriptionPattern.ReceiverOriginResolution != programindex.ResolutionAlternatives ||
		!sameSingleID(subscriptionPattern.ReceiverOriginIDs, kafkaConsumer.ID) ||
		subscriptionPattern.ReceiverOriginsObserved != 1 ||
		subscriptionPattern.ReceiverOriginsOmitted != 0 {
		t.Fatalf("cumulative Python consumer subscription pattern = %#v", subscriptionPattern)
	}
	subscriptionTopic := pythonPatternArgument(t, subscriptionPattern, 1)
	subscriptionHandler := pythonPatternArgument(t, subscriptionPattern, 2)
	if subscriptionTopic.Kind != programindex.PatternLiteralString ||
		subscriptionTopic.Value != "orders.created" ||
		subscriptionHandler.Kind != programindex.PatternDynamic ||
		subscriptionHandler.Resolution != programindex.ResolutionAlternatives ||
		!sameSingleID(subscriptionHandler.ObjectIDs, handleOrder.ID) ||
		subscriptionHandler.ObjectsObserved != 1 || subscriptionHandler.ObjectsOmitted != 0 {
		t.Fatalf("cumulative Python consumer subscription arguments = %#v", subscriptionPattern.Arguments)
	}
	assertPythonRelation(
		t, index, programindex.RelationPassesCallback, eventsModule.ID, handleOrder.ID,
		programindex.ResolutionAlternatives,
	)

	dynamicSubscription := pythonRelation(
		t, index, programindex.RelationCalls, subscribeDynamic.ID, "", programindex.ResolutionUnresolved,
	)
	dynamicPattern := singlePythonPattern(t, dynamicSubscription)
	if dynamicPattern.Selector != "subscribe" || dynamicPattern.ReceiverID != runtimeConsumer.ID ||
		len(dynamicPattern.ReceiverOriginIDs) != 0 || dynamicPattern.ReceiverOriginsObserved != 0 ||
		dynamicPattern.ReceiverOriginsOmitted != 0 {
		t.Fatalf("cumulative Python dynamic subscription receiver = %#v", dynamicPattern)
	}
	dynamicTopic := pythonPatternArgument(t, dynamicPattern, 1)
	dynamicCallback := pythonPatternArgument(t, dynamicPattern, 2)
	if dynamicTopic.Kind != programindex.PatternDynamic ||
		dynamicTopic.Resolution != programindex.ResolutionAlternatives ||
		!sameSingleID(dynamicTopic.ObjectIDs, topic.ID) || dynamicTopic.ObjectsObserved != 1 ||
		dynamicTopic.ObjectsOmitted != 0 ||
		dynamicCallback.Kind != programindex.PatternDynamic ||
		dynamicCallback.Resolution != programindex.ResolutionAlternatives ||
		!sameSingleID(dynamicCallback.ObjectIDs, callback.ID) || dynamicCallback.ObjectsObserved != 1 ||
		dynamicCallback.ObjectsOmitted != 0 {
		t.Fatalf("cumulative Python dynamic subscription arguments = %#v", dynamicPattern.Arguments)
	}
	for _, relation := range index.Relations {
		if relation.Kind == programindex.RelationPassesCallback && relation.FromID == subscribeDynamic.ID {
			t.Fatalf("dynamic callback parameter gained callable authority: %#v", relation)
		}
	}

	directFactory := pythonRelation(
		t, index, programindex.RelationInvokesExternal, subscribeDirect.ID, kafkaConsumer.ID,
		programindex.ResolutionAlternatives,
	)
	directFactoryPattern := singlePythonPattern(t, directFactory)
	if directFactoryPattern.Selector != "KafkaConsumer" || directFactoryPattern.ResultID == "" ||
		directFactoryPattern.Location == nil ||
		directFactoryPattern.Location.Path != "src/fixture_app/events.py" ||
		directFactoryPattern.Location.Line != 21 || directFactoryPattern.Location.Column != 5 {
		t.Fatalf("direct Python factory pattern = %#v", directFactoryPattern)
	}
	directResult := programIndexObjectByID(index, directFactoryPattern.ResultID)
	if directResult.ID == "" || directResult.Kind != programindex.ObjectVariable ||
		directResult.Name != "call result" || directResult.Location == nil ||
		directResult.Location.Path != "src/fixture_app/events.py" ||
		directResult.Location.Line != 21 || directResult.Location.Column != 5 {
		t.Fatalf("direct Python factory result object = %#v", directResult)
	}
	callResults := 0
	for _, object := range index.Objects {
		if object.Kind == programindex.ObjectVariable && object.Name == "call result" {
			callResults++
		}
	}
	if callResults != 1 {
		t.Fatalf("Python synthetic call-result objects = %d, want only the directly consumed factory result", callResults)
	}

	adaptertest.AssertRegistration(t, index, adaptertest.Registration{
		Name: "Python decorator registration",
		Registration: adaptertest.Relation{
			Kind: programindex.RelationDecorates, FromID: getLevel.ID, Resolution: programindex.ResolutionUnresolved,
			Path: "src/fixture_app/cli.py", Line: 17,
			TargetsObserved: 1, TargetsOmitted: 1, WitnessesObserved: 1, WitnessesOmitted: 0,
			PatternsObserved: 1, PatternsOmitted: 0,
			Patterns: []adaptertest.Pattern{{
				Form: programindex.PatternDecoratorCall, Selector: "get", ReceiverID: app.ID,
				Path: "src/fixture_app/cli.py", Line: 17,
				ReceiverOrigins: adaptertest.ObjectAuthority{
					IDs: []string{fastAPI.ID}, Resolution: programindex.ResolutionAlternatives, Observed: 1,
				},
				Observed: 1, Arguments: []adaptertest.Argument{{
					Position: 1, Kind: programindex.PatternLiteralString, Value: "/api/level/{level_id}",
				}},
			}},
		},
		RequireComplete: true,
	})
	adaptertest.AssertRegistration(t, index, adaptertest.Registration{
		Name: "Python callback registration",
		Registration: adaptertest.Relation{
			Kind: programindex.RelationCalls, FromID: eventsModule.ID, Resolution: programindex.ResolutionUnresolved,
			Invocation: "direct", Path: "src/fixture_app/events.py", Line: 13,
			TargetsObserved: 1, TargetsOmitted: 1, WitnessesObserved: 1, WitnessesOmitted: 0,
			PatternsObserved: 1, PatternsOmitted: 0,
			Patterns: []adaptertest.Pattern{{
				Form: programindex.PatternCall, Selector: "subscribe", ReceiverID: consumer.ID,
				Path: "src/fixture_app/events.py", Line: 13,
				ReceiverOrigins: adaptertest.ObjectAuthority{
					IDs: []string{kafkaConsumer.ID}, Resolution: programindex.ResolutionAlternatives, Observed: 1,
				},
				Observed: 2, Arguments: []adaptertest.Argument{
					{Position: 1, Kind: programindex.PatternLiteralString, Value: "orders.created"},
					{Position: 2, Kind: programindex.PatternDynamic, Objects: adaptertest.ObjectAuthority{
						IDs: []string{handleOrder.ID}, Resolution: programindex.ResolutionAlternatives, Observed: 1,
					}},
				},
			}},
		},
		Callbacks: []adaptertest.Callback{{
			ArgumentPosition: 2,
			Relation: adaptertest.Relation{
				Kind: programindex.RelationPassesCallback, FromID: eventsModule.ID, ToIDs: []string{handleOrder.ID},
				Resolution: programindex.ResolutionAlternatives, Path: "src/fixture_app/events.py", Line: 13,
				TargetsObserved: 1, WitnessesObserved: 1,
			},
		}},
		RequireComplete: true,
	})
	adaptertest.AssertRegistration(t, index, adaptertest.Registration{
		Name: "Python template output call",
		Registration: adaptertest.Relation{
			Kind: programindex.RelationInvokesExternal, FromID: fetchLevel.ID, ToIDs: []string{httpxGet.ID},
			Resolution: programindex.ResolutionAlternatives, Invocation: "direct",
			Path: "src/fixture_app/levels.py", Line: 5,
			TargetsObserved: 1, WitnessesObserved: 1, PatternsObserved: 1,
			Patterns: []adaptertest.Pattern{{
				Form: programindex.PatternCall, Selector: "get", Observed: 1,
				Path: "src/fixture_app/levels.py", Line: 5,
				Arguments: []adaptertest.Argument{{
					Position: 1, Kind: programindex.PatternStringTemplate,
					Parts: []programindex.PatternPart{
						{Kind: programindex.PatternPartLiteral, Text: "https://catalog.example/levels/"},
						{Kind: programindex.PatternPartHole},
					},
				}},
			}},
		},
		RequireComplete: true,
	})
	adaptertest.AssertRegistration(t, index, adaptertest.Registration{
		Name: "Python dynamic callback frontier",
		Registration: adaptertest.Relation{
			Kind: programindex.RelationCalls, FromID: subscribeDynamic.ID, Resolution: programindex.ResolutionUnresolved,
			Invocation: "direct", Path: "src/fixture_app/events.py", Line: 17,
			TargetsObserved: 1, TargetsOmitted: 1, WitnessesObserved: 1, PatternsObserved: 1,
			Patterns: []adaptertest.Pattern{{
				Form: programindex.PatternCall, Selector: "subscribe", ReceiverID: runtimeConsumer.ID, Observed: 2,
				Path: "src/fixture_app/events.py", Line: 17,
				Arguments: []adaptertest.Argument{
					{Position: 1, Kind: programindex.PatternDynamic, Objects: adaptertest.ObjectAuthority{
						IDs: []string{topic.ID}, Resolution: programindex.ResolutionAlternatives, Observed: 1,
					}},
					{Position: 2, Kind: programindex.PatternDynamic, Objects: adaptertest.ObjectAuthority{
						IDs: []string{callback.ID}, Resolution: programindex.ResolutionAlternatives, Observed: 1,
					}},
				},
			}},
		},
		RequireComplete: true,
	})
	adaptertest.AssertRegistration(t, index, adaptertest.Registration{
		Name: "Python direct-result callback registration",
		Registration: adaptertest.Relation{
			Kind: programindex.RelationCalls, FromID: subscribeDirect.ID,
			Resolution: programindex.ResolutionUnresolved, Invocation: "direct",
			Path: "src/fixture_app/events.py", Line: 21,
			TargetsObserved: 1, TargetsOmitted: 1, WitnessesObserved: 1, PatternsObserved: 1,
			Patterns: []adaptertest.Pattern{{
				Form: programindex.PatternCall, Selector: "subscribe", ReceiverID: directResult.ID,
				Path: "src/fixture_app/events.py", Line: 21, Column: 5,
				Observed: 2,
				Arguments: []adaptertest.Argument{
					{Position: 1, Kind: programindex.PatternLiteralString, Value: "orders.direct"},
					{Position: 2, Kind: programindex.PatternDynamic, Objects: adaptertest.ObjectAuthority{
						IDs: []string{handleOrder.ID}, Resolution: programindex.ResolutionAlternatives, Observed: 1,
					}},
				},
			}},
		},
		Callbacks: []adaptertest.Callback{{
			ArgumentPosition: 2,
			Relation: adaptertest.Relation{
				Kind: programindex.RelationPassesCallback, FromID: subscribeDirect.ID,
				ToIDs: []string{handleOrder.ID}, Resolution: programindex.ResolutionAlternatives,
				Path: "src/fixture_app/events.py", Line: 21,
				TargetsObserved: 1, WitnessesObserved: 1,
			},
		}},
		RequireComplete: true,
	})
	adaptertest.AssertRegistration(t, index, adaptertest.Registration{
		Name: "Python duplicate callback arguments",
		Registration: adaptertest.Relation{
			Kind: programindex.RelationCalls, FromID: bindDuplicateCallbacks.ID,
			Resolution: programindex.ResolutionUnresolved, Invocation: "direct",
			Path: "src/fixture_app/events.py", Line: 25,
			TargetsObserved: 1, TargetsOmitted: 1, WitnessesObserved: 1, PatternsObserved: 1,
			Patterns: []adaptertest.Pattern{{
				Form: programindex.PatternCall, Selector: "bind_pair", ReceiverID: consumer.ID,
				Path: "src/fixture_app/events.py", Line: 25,
				ReceiverOrigins: adaptertest.ObjectAuthority{
					IDs: []string{kafkaConsumer.ID}, Resolution: programindex.ResolutionAlternatives, Observed: 1,
				},
				Observed: 2,
				Arguments: []adaptertest.Argument{
					{Position: 1, Kind: programindex.PatternDynamic, Objects: adaptertest.ObjectAuthority{
						IDs: []string{handleOrder.ID}, Resolution: programindex.ResolutionAlternatives, Observed: 1,
					}},
					{Position: 2, Kind: programindex.PatternDynamic, Objects: adaptertest.ObjectAuthority{
						IDs: []string{handleOrder.ID}, Resolution: programindex.ResolutionAlternatives, Observed: 1,
					}},
				},
			}},
		},
		Callbacks: []adaptertest.Callback{
			{ArgumentPosition: 1, Relation: adaptertest.Relation{
				Kind: programindex.RelationPassesCallback, FromID: bindDuplicateCallbacks.ID,
				ToIDs: []string{handleOrder.ID}, Resolution: programindex.ResolutionAlternatives,
				Path: "src/fixture_app/events.py", Line: 25, TargetsObserved: 1, WitnessesObserved: 1,
			}},
			{ArgumentPosition: 2, Relation: adaptertest.Relation{
				Kind: programindex.RelationPassesCallback, FromID: bindDuplicateCallbacks.ID,
				ToIDs: []string{handleOrder.ID}, Resolution: programindex.ResolutionAlternatives,
				Path: "src/fixture_app/events.py", Line: 25, TargetsObserved: 1, WitnessesObserved: 1,
			}},
		},
		RequireComplete: true,
	})
}

func pythonFixtureTarget(t *testing.T, catalog pythontarget.Catalog) pythontarget.Target {
	t.Helper()
	for _, target := range catalog.Entries {
		if target.Selector == pythonFixtureSelector {
			return target
		}
	}
	t.Fatalf("Python fixture target %q is absent from catalog %#v", pythonFixtureSelector, catalog.Entries)
	return pythontarget.Target{}
}

func programIndexObjectByID(index programindex.Index, id string) programindex.Object {
	for _, object := range index.Objects {
		if object.ID == id {
			return object
		}
	}
	return programindex.Object{}
}

func programIndexObjectNamed(
	t *testing.T,
	index programindex.Index,
	kind programindex.ObjectKind,
	name string,
	path string,
) programindex.Object {
	t.Helper()
	for _, object := range index.Objects {
		if object.Kind == kind && object.Name == name && object.Location != nil &&
			object.Location.Path == path {
			return object
		}
	}
	t.Fatalf("Python ProgramIndex has no %s %q at %q", kind, name, path)
	return programindex.Object{}
}

func assertExactPythonImportBoundary(
	t *testing.T,
	index programindex.Index,
	fromID string,
	toID string,
	detail string,
) {
	t.Helper()
	for _, relation := range index.Relations {
		if relation.Kind != programindex.RelationImports || relation.FromID != fromID ||
			relation.Resolution != programindex.ResolutionExact || len(relation.ToIDs) != 1 ||
			relation.ToIDs[0] != toID {
			continue
		}
		for _, witness := range relation.Witnesses {
			if witness.Kind == "from_import_module_boundary" && witness.Detail == detail {
				return
			}
		}
	}
	t.Fatalf("Python ProgramIndex has no exact module boundary %q from %q to %q", detail, fromID, toID)
}

func programIndexExternalObjectNamed(
	t *testing.T,
	index programindex.Index,
	name string,
) programindex.Object {
	t.Helper()
	for _, object := range index.Objects {
		if object.Kind == programindex.ObjectExternalSymbol && object.Name == name {
			return object
		}
	}
	t.Fatalf("Python ProgramIndex has no external symbol %q", name)
	return programindex.Object{}
}

func assertPythonRelation(
	t *testing.T,
	index programindex.Index,
	kind programindex.RelationKind,
	fromID string,
	toID string,
	resolution programindex.Resolution,
) {
	t.Helper()
	_ = pythonRelation(t, index, kind, fromID, toID, resolution)
}

func pythonRelation(
	t *testing.T,
	index programindex.Index,
	kind programindex.RelationKind,
	fromID string,
	toID string,
	resolution programindex.Resolution,
) programindex.Relation {
	t.Helper()
	for _, relation := range index.Relations {
		if relation.Kind != kind || relation.FromID != fromID || relation.Resolution != resolution {
			continue
		}
		if (toID == "" && len(relation.ToIDs) == 0) || sameSingleID(relation.ToIDs, toID) {
			return relation
		}
	}
	t.Fatalf(
		"Python ProgramIndex has no %s relation from %q to %q with %s resolution",
		kind, fromID, toID, resolution,
	)
	return programindex.Relation{}
}

func singlePythonPattern(t *testing.T, relation programindex.Relation) programindex.RelationPattern {
	t.Helper()
	if relation.PatternsObserved != 1 || relation.PatternsOmitted != 0 || len(relation.Patterns) != 1 {
		t.Fatalf("Python relation pattern coverage = %#v", relation)
	}
	return relation.Patterns[0]
}

func pythonPatternArgument(
	t *testing.T,
	pattern programindex.RelationPattern,
	position int,
) programindex.PatternArgument {
	t.Helper()
	for _, argument := range pattern.Arguments {
		if argument.Position == position {
			return argument
		}
	}
	t.Fatalf("Python pattern %q has no positional argument %d: %#v", pattern.ID, position, pattern.Arguments)
	return programindex.PatternArgument{}
}

func sameSingleID(values []string, want string) bool {
	return len(values) == 1 && values[0] == want
}
