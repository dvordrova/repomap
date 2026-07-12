package orient

import (
	"slices"
	"testing"

	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/flowexplain"
	"github.com/dvordrova/repomap/internal/flowproof"
)

func TestResolvedFilesAreRemovedFromOverviewUnknowns(t *testing.T) {
	report := orientationPart{
		UnverifiedPaths: unverifiedPathList{
			{Path: "cmd/restic/main.go", Reason: "entrypoint needed proof"},
			{Path: "internal/restic/backup.go", Reason: "call target needed proof"},
			{Path: "internal/restic/callback.go", Reason: "callback target needed proof"},
			{Path: "internal/restic/runtime.go", Reason: "runtime target needed proof"},
			{Path: "internal/restic/inferred.go", Reason: "type-inferred target remains partial"},
			{Path: "internal/restic/dynamic.go", Reason: "dynamic target remains unresolved"},
			{Path: "internal/restic/suggested.go", Reason: "model suggestion remains unverified"},
			{Path: "internal/restic/backup_test.go", Reason: "similar path is not exact proof"},
			{Path: "docs/design.md", Reason: "unrelated unknown"},
			{Path: "invented/missing.go", Reason: "fabricated path"},
		},
		CandidateFlows: []flowexplain.CandidateFlow{{
			LocalProof: &flowproof.Session{Proof: flowproof.Proof{
				Slots: []flowproof.Slot{{
					Kind:        flowproof.SlotEntrypoint,
					Status:      flowproof.SlotVerified,
					EvidenceIDs: []string{"entrypoint"},
				}},
				Anchors: []flowproof.Anchor{
					{ID: "entrypoint", Location: reconcileLocation("cmd/restic/main.go", 10)},
					{ID: "static-target", Location: reconcileLocation("internal/restic/backup.go", 20)},
					{ID: "framework-target", Location: reconcileLocation("internal/restic/callback.go", 30)},
					{ID: "runtime-target", Location: reconcileLocation("internal/restic/runtime.go", 40)},
					{ID: "inferred-target", Location: reconcileLocation("internal/restic/inferred.go", 45)},
					{ID: "unresolved-target", Location: reconcileLocation("internal/restic/dynamic.go", 50)},
					{ID: "model-target", Location: reconcileLocation("internal/restic/suggested.go", 60)},
				},
				Transitions: []flowproof.Transition{
					{To: "static-target", Resolution: evidence.ResolutionStatic},
					{To: "framework-target", Resolution: evidence.ResolutionFrameworkRule},
					{To: "runtime-target", Resolution: evidence.ResolutionRuntimeObserved},
					{To: "inferred-target", Resolution: evidence.ResolutionTypeInferred},
					{To: "unresolved-target", Resolution: evidence.ResolutionUnresolved},
					{To: "model-target", Resolution: evidence.ResolutionModelSuggested},
				},
			}},
		}},
	}

	reconcileResolvedUnknownPaths(&report)

	want := unverifiedPathList{
		{Path: "internal/restic/inferred.go", Reason: "type-inferred target remains partial"},
		{Path: "internal/restic/dynamic.go", Reason: "dynamic target remains unresolved"},
		{Path: "internal/restic/suggested.go", Reason: "model suggestion remains unverified"},
		{Path: "internal/restic/backup_test.go", Reason: "similar path is not exact proof"},
		{Path: "docs/design.md", Reason: "unrelated unknown"},
		{Path: "invented/missing.go", Reason: "fabricated path"},
	}
	if !slices.Equal(report.UnverifiedPaths, want) {
		t.Fatalf("unverified paths = %#v, want %#v", report.UnverifiedPaths, want)
	}
}

func reconcileLocation(path string, line int) *evidence.Location {
	return &evidence.Location{Path: path, Line: line}
}
