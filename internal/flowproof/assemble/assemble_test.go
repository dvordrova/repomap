package assemble

import (
	"context"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/experiment/surfacediscovery"
	"github.com/dvordrova/repomap/internal/flowexplain"
	"github.com/dvordrova/repomap/internal/flowproof"
	"github.com/dvordrova/repomap/internal/gofacts"
)

func TestAttachSeedsPartialProcessTraceFromUniqueExactEntry(t *testing.T) {
	t.Parallel()

	entry := processEntrySurface("entry-app", surfacediscovery.AvailabilityAvailable)
	route := routeSurface("route-health")
	route.Quality.Reachability = surfacediscovery.SurfaceQualityStatic
	route.WrapperChain = []surfacediscovery.Wrapper{{
		Symbol: surfacediscovery.Symbol{
			ID: "example.com/app.registerRoutes", Name: "registerRoutes",
			Location: surfacediscovery.Location{Path: "internal/routes.go", Line: 20},
		},
		Callsite: surfacediscovery.Location{Path: "cmd/app/main.go", Line: 14},
		Origin:   "repository",
	}}
	flows := []flowexplain.CandidateFlow{{
		Name: "Health request", FlowType: flowexplain.FlowTypeRequest,
		LikelyEntrypoint: "cmd/app/main.go",
	}}

	Attach(context.Background(), t.TempDir(), flows, Input{
		Surfaces: []surfacediscovery.TriggerRecord{entry, route},
	})

	session := flows[0].LocalProof
	if session == nil {
		t.Fatal("exact available process entry did not seed a proof")
	}
	proof := session.Proof
	if proof.Archetype != flowproof.ArchetypeProcess || proof.SeedSurfaceID != entry.ID ||
		proof.TraceQuality != flowproof.TraceQualityPartial || proof.CurrentFrontier == "" {
		t.Fatalf("process proof identity/quality = %#v", proof)
	}
	entrypoint, _ := proof.Slot(flowproof.SlotEntrypoint)
	if entrypoint.Status != flowproof.SlotVerified || len(entrypoint.EvidenceIDs) != 1 ||
		entrypoint.EvidenceIDs[0] != entry.ID {
		t.Fatalf("entrypoint slot = %#v", entrypoint)
	}
	if _, ok := proof.Anchor(route.ID); !ok {
		t.Fatalf("same-executable request route was not retained: %#v", proof.Anchors)
	}
	if len(proof.TraceEvidenceSurfaceIDs) != 1 || proof.TraceEvidenceSurfaceIDs[0] != route.ID {
		t.Fatalf("trace evidence surfaces = %v, want only %q", proof.TraceEvidenceSurfaceIDs, route.ID)
	}
	if len(proof.Transitions) == 0 || proof.Transitions[0].Evidence.Path != "cmd/app/main.go" {
		t.Fatalf("wrapper evidence was not retained exactly: %#v", proof.Transitions)
	}
	if session.Stop == nil || session.Stop.Reason != flowproof.StopNoTask {
		t.Fatalf("partial process proof stop = %#v", session.Stop)
	}
}

func TestAttachPrefersServerStartForOperationalProcessTrace(t *testing.T) {
	t.Parallel()

	entry := processEntrySurface("entry-app", surfacediscovery.AvailabilityAvailable)
	route := routeSurface("route-health")
	server := serverSurface("server-start")
	flows := []flowexplain.CandidateFlow{{
		Name: "Application startup", FlowType: flowexplain.FlowTypeOperational,
		LikelyEntrypoint: "cmd/app/main.go",
	}}

	Attach(context.Background(), t.TempDir(), flows, Input{
		Surfaces: []surfacediscovery.TriggerRecord{entry, route, server},
	})

	proof := flows[0].LocalProof.Proof
	if _, ok := proof.Anchor(server.ID); !ok {
		t.Fatalf("operational proof did not prefer server start: %#v", proof.Anchors)
	}
	if _, ok := proof.Anchor(route.ID); ok {
		t.Fatalf("operational proof also selected request route: %#v", proof.Anchors)
	}
}

func TestAttachRetainsExactDirectSurfaceCall(t *testing.T) {
	t.Parallel()

	entry := processEntrySurface("entry-app", surfacediscovery.AvailabilityAvailable)
	server := serverSurface("server-start")
	server.Quality.Reachability = surfacediscovery.SurfaceQualityStatic
	directSite := surfacediscovery.Location{Path: "cmd/app/main.go", Line: 25}
	server.RegistrationSite = directSite
	server.ServerStartSite = &directSite
	flows := []flowexplain.CandidateFlow{{
		Name: "Application startup", FlowType: flowexplain.FlowTypeOperational,
		LikelyEntrypoint: "cmd/app/main.go",
	}}

	Attach(context.Background(), t.TempDir(), flows, Input{
		Surfaces: []surfacediscovery.TriggerRecord{entry, server},
	})

	proof := flows[0].LocalProof.Proof
	if len(proof.Transitions) != 1 || proof.Transitions[0].From != entry.ID ||
		proof.Transitions[0].To != server.ID || proof.Transitions[0].Relation != evidence.RelationCalls ||
		proof.Transitions[0].Evidence.Path != "cmd/app/main.go" {
		t.Fatalf("exact direct surface transition = %#v", proof.Transitions)
	}
}

func TestAttachKeepsEntryOnlyProcessProofWithExplicitFrontier(t *testing.T) {
	t.Parallel()

	entry := processEntrySurface("entry-app", surfacediscovery.AvailabilityAvailable)
	flows := []flowexplain.CandidateFlow{{
		Name: "Process lifecycle", LikelyEntrypoint: "cmd/app/main.go",
	}}
	Attach(context.Background(), t.TempDir(), flows, Input{
		Surfaces: []surfacediscovery.TriggerRecord{entry},
	})

	if flows[0].LocalProof == nil {
		t.Fatal("entry-only process proof was discarded")
	}
	proof := flows[0].LocalProof.Proof
	if len(proof.Anchors) != 1 || len(proof.Transitions) != 0 ||
		proof.TraceQuality != flowproof.TraceQualityPartial ||
		!strings.Contains(proof.CurrentFrontier, "downstream runtime handoff") {
		t.Fatalf("entry-only process proof = %#v", proof)
	}
}

func TestAttachDoesNotPromoteRuntimeActivityOrUnavailableEntry(t *testing.T) {
	t.Parallel()

	activity := processEntrySurface("entry-app", surfacediscovery.AvailabilityAvailable)
	flows := []flowexplain.CandidateFlow{
		{
			Name: "Aggregated background work", LikelyEntrypoint: "cmd/app/main.go",
			CandidateBasis: flowexplain.CandidateBasisRuntimeActivity,
		},
		{
			Name: "Aggregated source signals", LikelyEntrypoint: "cmd/app/main.go",
			CandidateBasis: flowexplain.CandidateBasisSourceSignalAggregate,
		},
		{Name: "Unavailable process", LikelyEntrypoint: "cmd/broken/main.go"},
	}
	unavailable := processEntrySurface("entry-broken", surfacediscovery.AvailabilityUnavailable)
	unavailable.ProcessEntrypoint.Location.Path = "cmd/broken/main.go"
	unavailable.SurfaceRole = surfacediscovery.SurfaceRoleRejected
	unavailable.TraceReadiness = surfacediscovery.TraceReadinessRejected

	Attach(context.Background(), t.TempDir(), flows, Input{
		Surfaces: []surfacediscovery.TriggerRecord{activity, unavailable},
	})

	for index, flow := range flows {
		if flow.LocalProof != nil {
			t.Fatalf("flow[%d] unexpectedly became a process trace: %#v", index, flow.LocalProof)
		}
	}
}

func TestBuildDescriptorProofKeepsConsumerFrontier(t *testing.T) {
	t.Parallel()

	descriptorLocation := surfacediscovery.Location{Path: "internal/routes.go", Line: 44}
	descriptor := surfacediscovery.TriggerRecord{
		ID: "descriptor-health", Kind: "http_route_descriptor",
		SurfaceRole:    surfacediscovery.SurfaceRoleDescriptor,
		TraceReadiness: surfacediscovery.TraceReadinessPartial,
		Availability:   surfacediscovery.AvailabilityAvailable,
		Identity:       surfacediscovery.Identity{Path: surfacediscovery.Value{Known: true, Text: "/health"}},
		DescriptorSite: &descriptorLocation,
		DynamicFrontier: []surfacediscovery.Frontier{{
			Kind:   "route_provider_dispatch_candidate",
			Detail: "exact descriptor found; runtime consumer registration remains unresolved",
		}},
	}

	proof, ok := BuildDescriptorProof("descriptor-flow", "Health descriptor", descriptor)
	if !ok {
		t.Fatal("exact descriptor did not seed a partial proof")
	}
	if proof.SeedSurfaceID != descriptor.ID || proof.TraceQuality != flowproof.TraceQualityPartial ||
		!strings.Contains(proof.CurrentFrontier, "consumer registration") {
		t.Fatalf("descriptor proof = %#v", proof)
	}
	trigger, _ := proof.Slot(flowproof.SlotTrigger)
	if trigger.Status != flowproof.SlotPartial || len(trigger.EvidenceIDs) != 1 ||
		trigger.EvidenceIDs[0] != descriptor.ID {
		t.Fatalf("descriptor trigger slot = %#v", trigger)
	}
}

func TestAttachKeepsCobraPriorityOverProcessSurface(t *testing.T) {
	t.Parallel()

	flow := flowexplain.CandidateFlow{
		Name: "Serve command", Trigger: "CLI command",
		LikelyEntrypoint: "cmd/app/main.go",
	}
	trace := gofacts.CommandTrace{
		Version: gofacts.CommandTraceVersion, Framework: "cobra",
		EntrypointPackage: "example.com/app/cmd/app", Command: "serve",
		Steps: []gofacts.CommandTraceStep{{
			Symbol: "main", Relation: "entrypoint",
			TargetLocation: evidence.Location{Path: "cmd/app/main.go", Line: 10},
		}, {
			Symbol: "newRootCommand", Relation: "calls",
			CallsiteLocation: &evidence.Location{Path: "cmd/app/main.go", Line: 11},
			TargetLocation:   evidence.Location{Path: "cmd/app/root.go", Line: 20},
		}, {
			Symbol: "newServeCommand", Relation: "registers_command",
			CallsiteLocation: &evidence.Location{Path: "cmd/app/root.go", Line: 30},
			TargetLocation:   evidence.Location{Path: "cmd/app/serve.go", Line: 40},
		}},
	}
	flows := []flowexplain.CandidateFlow{flow}
	Attach(context.Background(), t.TempDir(), flows, Input{
		CommandTraces: []gofacts.CommandTrace{trace},
		Surfaces:      []surfacediscovery.TriggerRecord{processEntrySurface("entry-app", surfacediscovery.AvailabilityAvailable)},
	})
	if flows[0].LocalProof == nil || flows[0].LocalProof.Proof.Archetype != flowproof.ArchetypeCLI {
		t.Fatalf("Cobra did not retain proof priority: %#v", flows[0].LocalProof)
	}
	wantSeed, _, _ := gofacts.CommandSurfaceIdentity(trace)
	if flows[0].LocalProof.Proof.SeedSurfaceID != wantSeed {
		t.Fatalf("CLI seed surface = %q, want %q", flows[0].LocalProof.Proof.SeedSurfaceID, wantSeed)
	}
}

func processEntrySurface(id, availability string) surfacediscovery.TriggerRecord {
	return surfacediscovery.TriggerRecord{
		ID: id, Kind: "process_entry", Resolution: "exact", ScenarioID: "go-default",
		SurfaceRole:    surfacediscovery.SurfaceRoleEntrySurface,
		TraceReadiness: surfacediscovery.TraceReadinessPartial,
		Availability:   availability, OwningExecutable: "cmd/app",
		ProcessEntrypoint: surfacediscovery.Symbol{
			ID: "example.com/app/cmd/app.main", Name: "main", Package: "example.com/app/cmd/app",
			Location: surfacediscovery.Location{Path: "cmd/app/main.go", Line: 10},
		},
	}
}

func routeSurface(id string) surfacediscovery.TriggerRecord {
	return surfacediscovery.TriggerRecord{
		ID: id, Kind: "http_route", Resolution: "exact",
		SurfaceRole:    surfacediscovery.SurfaceRoleEntrySurface,
		TraceReadiness: surfacediscovery.TraceReadinessReady,
		Availability:   surfacediscovery.AvailabilityAvailable, OwningExecutable: "cmd/app",
		Identity:          surfacediscovery.Identity{Method: "GET", Path: surfacediscovery.Value{Known: true, Text: "/health"}},
		RegistrationSite:  surfacediscovery.Location{Path: "internal/routes.go", Line: 30},
		ProcessEntrypoint: processEntrySurface("", surfacediscovery.AvailabilityAvailable).ProcessEntrypoint,
		Handler:           surfacediscovery.Value{Known: true, Text: "example.com/app.health"},
	}
}

func serverSurface(id string) surfacediscovery.TriggerRecord {
	location := surfacediscovery.Location{Path: "internal/server.go", Line: 50}
	return surfacediscovery.TriggerRecord{
		ID: id, Kind: "http_server", Resolution: "exact",
		SurfaceRole:    surfacediscovery.SurfaceRoleEntrySurface,
		TraceReadiness: surfacediscovery.TraceReadinessPartial,
		Availability:   surfacediscovery.AvailabilityAvailable, OwningExecutable: "cmd/app",
		RegistrationSite: location, ServerStartSite: &location,
		ProcessEntrypoint: processEntrySurface("", surfacediscovery.AvailabilityAvailable).ProcessEntrypoint,
		Handler:           surfacediscovery.Value{Known: true, Text: "net/http.Server"},
	}
}
