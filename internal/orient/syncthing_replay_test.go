package orient

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/dvordrova/repomap/internal/experiment/surfacediscovery"
	"github.com/dvordrova/repomap/internal/flowexplain"
	"github.com/dvordrova/repomap/internal/flowproof"
	flowproofassemble "github.com/dvordrova/repomap/internal/flowproof/assemble"
)

func TestReplaySavedSyncthingOrientationSeedsPartialTracesWithoutProvider(t *testing.T) {
	t.Parallel()

	encoded, err := os.ReadFile("testdata/syncthing_orientation_report.json")
	if err != nil {
		t.Fatal(err)
	}
	var report orientationPart
	if err := json.Unmarshal(encoded, &report); err != nil {
		t.Fatal(err)
	}
	if len(report.CandidateFlows) != 6 || len(report.ResearchQuestions) != 2 {
		t.Fatalf("saved response shape = %d flows, %d questions", len(report.CandidateFlows), len(report.ResearchQuestions))
	}

	surfaces := []surfacediscovery.TriggerRecord{
		replayProcessSurface("primary", "cmd/syncthing/main.go", "cmd/syncthing", surfacediscovery.AvailabilityUnavailable),
		replayProcessSurface("discovery", "cmd/stdiscosrv/main.go", "cmd/stdiscosrv", surfacediscovery.AvailabilityAvailable),
		replayServerSurface("discovery-server", "cmd/stdiscosrv/main.go", "cmd/stdiscosrv", 180),
		replayProcessSurface("relay", "cmd/strelaysrv/main.go", "cmd/strelaysrv", surfacediscovery.AvailabilityAvailable),
		replayServerSurface("relay-server", "cmd/strelaysrv/main.go", "cmd/strelaysrv", 170),
		replayProcessSurface("crash", "cmd/infra/stcrashreceiver/main.go", "cmd/infra/stcrashreceiver", surfacediscovery.AvailabilityAvailable),
		replayServerSurface("crash-server", "cmd/infra/stcrashreceiver/main.go", "cmd/infra/stcrashreceiver", 108),
		replayProcessSurface("dev-tool", "cmd/dev/stvanity/main.go", "cmd/dev/stvanity", surfacediscovery.AvailabilityAvailable),
	}
	attachLocalFlowProofs(context.Background(), t.TempDir(), &report, flowproofassemble.Input{
		Surfaces: surfaces, ProofBudget: flowproof.DefaultBudget(),
	})

	proofs := make(map[string]*flowproof.Session)
	for _, flow := range report.CandidateFlows {
		proofs[flowexplain.GenerateFlowID(flow.Name)] = flow.LocalProof
	}
	for _, id := range []string{
		"discovery-server-stdiscosrv-operation",
		"relay-server-strelaysrv-operation",
		"crash-report-receiver-background-maintenance",
	} {
		session := proofs[id]
		if session == nil || session.Proof.Archetype != flowproof.ArchetypeProcess ||
			session.Proof.TraceQuality != flowproof.TraceQualityPartial ||
			len(session.Proof.Transitions) != 1 || session.Proof.CurrentFrontier == "" {
			t.Fatalf("replayed partial trace %q = %#v", id, session)
		}
	}
	for _, id := range []string{
		"syncthing-daemon-startup-and-continuous-sync",
		"rest-api-request-handling",
		"background-loop-periodic-ticker-created",
	} {
		if proofs[id] != nil {
			t.Fatalf("unavailable or aggregate direction %q became a top-level trace", id)
		}
	}
	if got := savedFlowProofIDs(report.CandidateFlows); len(got) != 3 {
		t.Fatalf("saved trace IDs = %v, want three real FlowProof sessions", got)
	}
}

func replayProcessSurface(id, path, owner, availability string) surfacediscovery.TriggerRecord {
	role := surfacediscovery.SurfaceRoleEntrySurface
	readiness := surfacediscovery.TraceReadinessPartial
	if availability == surfacediscovery.AvailabilityUnavailable {
		role = surfacediscovery.SurfaceRoleRejected
		readiness = surfacediscovery.TraceReadinessRejected
	}
	return surfacediscovery.TriggerRecord{
		ID: id, Kind: "process_entry", Resolution: "exact", ScenarioID: "go-default",
		SurfaceRole: role, TraceReadiness: readiness, Availability: availability,
		OwningExecutable: owner,
		ProcessEntrypoint: surfacediscovery.Symbol{
			ID: owner + ".main", Name: "main", Package: owner,
			Location: surfacediscovery.Location{Path: path, Line: 1},
		},
	}
}

func replayServerSurface(id, path, owner string, line int) surfacediscovery.TriggerRecord {
	location := surfacediscovery.Location{Path: path, Line: line}
	return surfacediscovery.TriggerRecord{
		ID: id, Kind: "http_server", Resolution: "partial", ScenarioID: "go-default",
		SurfaceRole:    surfacediscovery.SurfaceRoleEntrySurface,
		TraceReadiness: surfacediscovery.TraceReadinessPartial,
		Availability:   surfacediscovery.AvailabilityAvailable, OwningExecutable: owner,
		RegistrationSite: location, ServerStartSite: &location,
		ProcessEntrypoint: surfacediscovery.Symbol{
			ID: owner + ".main", Name: "main", Package: owner,
			Location: surfacediscovery.Location{Path: path, Line: 1},
		},
		Identity: surfacediscovery.Identity{Name: "HTTP server"},
		Handler:  surfacediscovery.Value{Known: true, Text: "net/http.Server"},
		Quality:  surfacediscovery.SurfaceQuality{Reachability: surfacediscovery.SurfaceQualityStatic},
	}
}
