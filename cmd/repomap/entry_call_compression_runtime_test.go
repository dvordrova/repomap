package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/entrycall"
	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/modelresearch"
	"github.com/dvordrova/repomap/internal/report"
)

type entryCallCompressionRuntimeProvider struct {
	request          entrycall.Request
	calls            int
	invalid          bool
	partial          bool
	selectFirstRoute bool
}

func (provider *entryCallCompressionRuntimeProvider) EntryCallCompressionPromptJSON(
	prompt entrycall.Prompt,
) ([]byte, error) {
	const marker = "Exact bounded request JSON:\n"
	position := strings.LastIndex(prompt.User, marker)
	if position < 0 {
		return nil, errors.New("missing exact entry-call request marker")
	}
	if err := json.Unmarshal([]byte(prompt.User[position+len(marker):]), &provider.request); err != nil {
		return nil, err
	}
	return []byte(`{"exact":"entry-call-envelope"}`), nil
}

func (provider *entryCallCompressionRuntimeProvider) EntryCallCompressionBodyMeasured(
	_ context.Context,
	_ []byte,
) (modelresearch.ProviderResult, error) {
	provider.calls++
	if provider.invalid {
		return modelresearch.ProviderResult{Content: []byte(`{"version":3,"request_ref":"wrong","entries":[],"surface_proposals":[]}`), Attempts: 1}, nil
	}
	entries := make([]entrycall.ResponseEntry, 0, len(provider.request.Entries))
	for _, requestEntry := range provider.request.Entries {
		refs := make([]string, 0, len(requestEntry.Families))
		for position, family := range requestEntry.Families {
			if provider.partial && position == 0 {
				continue
			}
			refs = append(refs, family.Ref)
		}
		entries = append(entries, entrycall.ResponseEntry{RootRef: requestEntry.Ref, FamilyRefs: refs})
	}
	proposals := []entrycall.ResponseSurfaceProposal{}
	if provider.selectFirstRoute && len(provider.request.SurfaceCatalog.Candidates) > 0 {
		candidate := provider.request.SurfaceCatalog.Candidates[0]
		bindings := []entrycall.ResponseSurfaceBinding{}
		for _, fact := range candidate.Facts {
			slot := ""
			switch fact.Kind {
			case entrycall.SurfaceFactToken:
				slot = entrycall.SurfaceSlotRefMethod
			case entrycall.SurfaceFactString:
				slot = entrycall.SurfaceSlotRefPath
			case entrycall.SurfaceFactCallable:
				slot = entrycall.SurfaceSlotRefHandler
			}
			if slot != "" {
				bindings = append(bindings, entrycall.ResponseSurfaceBinding{SlotRef: slot, FactRef: fact.Ref})
			}
		}
		proposals = append(proposals, entrycall.ResponseSurfaceProposal{
			CandidateRef: candidate.Ref, KindRef: entrycall.SurfaceKindRefHTTPRoute, Bindings: bindings,
		})
	}
	response, err := json.Marshal(entrycall.Response{
		Version: entrycall.ResultVersion, RequestRef: provider.request.RequestRef, Entries: entries,
		SurfaceProposals: proposals,
	})
	if err != nil {
		return modelresearch.ProviderResult{}, err
	}
	return modelresearch.ProviderResult{Content: response, Attempts: 1, ResponseBytes: len(response)}, nil
}

func TestEntryCallCompressionRuntimePersistsAcceptedPartialAccounting(t *testing.T) {
	runDir := newEntryCallCompressionRunDir(t, true)
	provider := &entryCallCompressionRuntimeProvider{partial: true}
	outcome, err := runEntryCallCompressionForRun(
		t.Context(), runDir, entryCallCompressionTestSubstrate(), entryCallCompressionTestState(),
		newRunOutput(nil), func() (entryCallCompressionClient, error) { return provider, nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Status.State != entrycall.StatusAcceptedPartial ||
		outcome.Status.Reason != entrycall.ReasonResponsePartial ||
		outcome.Status.SelectedFamilies != 0 || outcome.Status.RejectedFamilies != 1 {
		t.Fatalf("partial outcome = %+v", outcome)
	}
	resultRaw, err := os.ReadFile(filepath.Join(runDir, entrycall.ResultArtifactFilename))
	if err != nil {
		t.Fatal(err)
	}
	result, err := entrycall.DecodeResult(resultRaw)
	if err != nil || result.SelectedFamilyCount() != 0 || result.RejectedFamilyCount() != 1 {
		t.Fatalf("partial result = %+v, %v", result, err)
	}
	statusRaw, err := os.ReadFile(filepath.Join(runDir, entrycall.StatusArtifactFilename))
	if err != nil {
		t.Fatal(err)
	}
	status, err := entrycall.DecodeStatus(statusRaw)
	if err != nil || status.State != entrycall.StatusAcceptedPartial || status.RejectedFamilies != 1 {
		t.Fatalf("partial status = %+v, %v", status, err)
	}
}

func TestEntryCallCompressionRuntimePersistsOneAcceptedRefsOnlyCall(t *testing.T) {
	runDir := newEntryCallCompressionRunDir(t, true)
	provider := &entryCallCompressionRuntimeProvider{}
	outcome, err := runEntryCallCompressionForRun(
		t.Context(), runDir, entryCallCompressionTestSubstrate(), entryCallCompressionTestState(),
		newRunOutput(nil), func() (entryCallCompressionClient, error) { return provider, nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	if provider.calls != 1 || outcome.Status.State != entrycall.StatusAccepted ||
		outcome.SemanticCalls != 1 || outcome.TransportAttempts != 1 ||
		outcome.Status.AdvertisedFamilies != 2 || outcome.Status.SelectedFamilies != 2 {
		t.Fatalf("provider/outcome = %d/%+v", provider.calls, outcome)
	}
	resultRaw, err := os.ReadFile(filepath.Join(runDir, entrycall.ResultArtifactFilename))
	if err != nil {
		t.Fatal(err)
	}
	result, err := entrycall.DecodeResult(resultRaw)
	if err != nil || result.SelectedFamilyCount() != 2 || result.RepositoryStateSHA256 == "" {
		t.Fatalf("result = %+v, %v", result, err)
	}
	statusRaw, err := os.ReadFile(filepath.Join(runDir, entrycall.StatusArtifactFilename))
	if err != nil {
		t.Fatal(err)
	}
	status, err := entrycall.DecodeStatus(statusRaw)
	if err != nil || status.ResultSHA256 != modelresearch.SHA256(resultRaw) {
		t.Fatalf("status = %+v, %v", status, err)
	}
	entries := readSemanticJournalEntries(t, runDir)
	if len(entries) != 1 || entries[0].record.Stage != debugdump.SemanticStageEntryCallCompression ||
		entries[0].record.State != debugdump.SemanticStateAccepted ||
		entries[0].record.SemanticCalls != 1 || entries[0].record.TransportAttempts != 1 {
		t.Fatalf("semantic entries = %+v", entries)
	}
}

func TestEntryCallCompressionRuntimeAcceptsThirteenAdvertisedFamilies(t *testing.T) {
	runDir := newEntryCallCompressionRunDir(t, true)
	provider := &entryCallCompressionRuntimeProvider{}
	outcome, err := runEntryCallCompressionForRun(
		t.Context(), runDir, entryCallCompressionThirteenFamilySubstrate(), entryCallCompressionTestState(),
		newRunOutput(nil), func() (entryCallCompressionClient, error) { return provider, nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	if provider.calls != 1 || outcome.Status.State != entrycall.StatusAccepted ||
		outcome.Status.AdvertisedFamilies != 13 || outcome.Status.SelectedFamilies != 13 ||
		outcome.Status.RejectedFamilies != 0 {
		t.Fatalf("thirteen-family provider/outcome = %d/%+v", provider.calls, outcome)
	}
	resultRaw, err := os.ReadFile(filepath.Join(runDir, entrycall.ResultArtifactFilename))
	if err != nil {
		t.Fatal(err)
	}
	if result, err := entrycall.DecodeResult(resultRaw); err != nil ||
		result.SelectedFamilyCount() != 13 || result.RejectedFamilyCount() != 0 {
		t.Fatalf("thirteen-family result = %+v, %v", result, err)
	}
}

func TestEntryCallCompressionRuntimeSkipsEmptyBundleWithoutProvider(t *testing.T) {
	runDir := newEntryCallCompressionRunDir(t, false)
	substrate := entryCallCompressionTestSubstrate()
	substrate.Families = []entrycall.ExactFamily{}
	providerCalls := 0
	outcome, err := runEntryCallCompressionForRun(
		t.Context(), runDir, substrate, entryCallCompressionTestState(), newRunOutput(nil),
		func() (entryCallCompressionClient, error) {
			providerCalls++
			return nil, errors.New("provider must not be configured")
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if providerCalls != 0 || outcome.Status.State != entrycall.StatusSkipped ||
		outcome.Status.Reason != entrycall.ReasonNoCandidates || outcome.SemanticCalls != 0 {
		t.Fatalf("provider/outcome = %d/%+v", providerCalls, outcome)
	}
	if _, err := os.Stat(filepath.Join(runDir, entrycall.ResultArtifactFilename)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("empty bundle result artifact err = %v", err)
	}
	statusRaw, err := os.ReadFile(filepath.Join(runDir, entrycall.StatusArtifactFilename))
	if err != nil {
		t.Fatal(err)
	}
	if status, err := entrycall.DecodeStatus(statusRaw); err != nil || status.State != entrycall.StatusSkipped {
		t.Fatalf("status = %+v, %v", status, err)
	}
}

func TestEntryCallCompressionRuntimeCallsSameProviderForSurfaceOnlyBundle(t *testing.T) {
	runDir := newEntryCallCompressionRunDir(t, true)
	substrate := entryCallCompressionTestSubstrate()
	substrate.Families = []entrycall.ExactFamily{}
	substrate.SurfaceCandidates = []entrycall.ExactSurfaceCandidate{{
		ID: "route-candidate", RootNodeID: "main", Form: entrycall.SurfaceCandidateDirectCall,
		Sketch: "GET", Site: entrycall.Location{Path: "routes.go", Line: 8, Column: 2},
		Facts: []entrycall.ExactSurfaceFact{
			{ID: "route-method", Kind: entrycall.SurfaceFactToken, Position: 0, Label: "selector", Value: "GET", Location: entrycall.Location{Path: "routes.go", Line: 8, Column: 2}},
			{ID: "route-path", Kind: entrycall.SurfaceFactString, Position: 1, Label: "argument 1", Value: "/account/:id", Location: entrycall.Location{Path: "routes.go", Line: 8, Column: 6}},
			{ID: "route-handler", Kind: entrycall.SurfaceFactCallable, Position: 2, Label: "argument 2", Value: "getAccount", Location: entrycall.Location{Path: "handlers.go", Line: 20, Column: 1}},
		},
	}}
	substrate.Coverage.SurfaceCandidatesConsidered = 1
	substrate.Coverage.SurfaceCandidatesIndexed = 1
	substrate.Coverage.SurfaceCandidateFactsConsidered = 3
	substrate.Coverage.SurfaceCandidateFactsIndexed = 3
	provider := &entryCallCompressionRuntimeProvider{selectFirstRoute: true}
	outcome, err := runEntryCallCompressionForRun(
		t.Context(), runDir, substrate, entryCallCompressionTestState(), newRunOutput(nil),
		func() (entryCallCompressionClient, error) { return provider, nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	if provider.calls != 1 || outcome.SemanticCalls != 1 || outcome.Status.State != entrycall.StatusAccepted ||
		outcome.Status.AdvertisedFamilies != 0 || outcome.Status.AdvertisedSurfaceCandidates != 1 ||
		outcome.Status.SelectedSurfaces != 1 || outcome.Status.RejectedSurfaces != 0 {
		t.Fatalf("surface-only provider/outcome = %d/%+v", provider.calls, outcome)
	}
	resultRaw, err := os.ReadFile(filepath.Join(runDir, entrycall.ResultArtifactFilename))
	if err != nil {
		t.Fatal(err)
	}
	result, err := entrycall.DecodeResult(resultRaw)
	if err != nil || result.SelectedSurfaceCount() != 1 || result.SurfaceProposals[0].Path == nil ||
		result.SurfaceProposals[0].Path.Text != "/account/:id" {
		t.Fatalf("surface-only result = %+v, %v", result, err)
	}
}

func TestEntryCallCompressionDiagnosticWaitsForSharedFinalization(t *testing.T) {
	runDir := newEntryCallCompressionRunDir(t, true)
	outcome := entryCallCompressionRunOutcome{
		Status:        entrycall.Status{PromptVersion: entrycall.PromptVersion, State: entrycall.StatusAccepted},
		SemanticCalls: 1, TransportAttempts: 1, RequestBytes: 321, LatencyMillis: 9,
	}
	if err := recordEntryCallCompressionDiagnostic(runDir, outcome); err != nil {
		t.Fatal(err)
	}
	metadata, err := readAtlasFirstMetadata(runDir)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.ProviderAccountingComplete || metadata.ProviderRequestCount != 1 ||
		metadata.ExternalRequestBytes != 321 || len(metadata.RequestAttempts) != 1 ||
		metadata.RequestAttempts[0].Stage != debugdump.SemanticStageEntryCallCompression ||
		metadata.RequestAttempts[0].State != "accepted" {
		t.Fatalf("metadata = %+v", metadata)
	}
	if err := finalizeAtlasFirstStageDiagnostics(runDir); err != nil {
		t.Fatal(err)
	}
	metadata, err = readAtlasFirstMetadata(runDir)
	if err != nil {
		t.Fatal(err)
	}
	if !metadata.ProviderAccountingComplete || metadata.ProviderRequestCount != 1 {
		t.Fatalf("finalized metadata = %+v", metadata)
	}
}

func TestEntryCallCompressionCancellationGetsOnlyBoundedPublicationContext(t *testing.T) {
	parent, cancelParent := context.WithCancel(context.Background())
	cancelParent()

	ordinary, cancelOrdinary := entryCallCompressionPublicationContext(
		parent,
		entryCallCompressionRunOutcome{},
	)
	defer cancelOrdinary()
	if !errors.Is(ordinary.Err(), context.Canceled) {
		t.Fatalf("ordinary context error = %v, want parent cancellation", ordinary.Err())
	}

	publication, cancelPublication := entryCallCompressionPublicationContext(
		parent,
		entryCallCompressionRunOutcome{Canceled: true},
	)
	defer cancelPublication()
	if err := publication.Err(); err != nil {
		t.Fatalf("detached bounded publication context starts canceled: %v", err)
	}
	if _, hasDeadline := publication.Deadline(); !hasDeadline {
		t.Fatal("detached publication context has no deadline")
	}
}

func TestRunDefaultBindsAcceptedEntryCallAndKeepsClosedStatusOptional(t *testing.T) {
	for _, test := range []struct {
		name     string
		invalid  bool
		accepted bool
	}{
		{name: "accepted artifacts are report material", accepted: true},
		{name: "closed status stays optional", invalid: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository := atlasFirstAcceptanceRepository(t, "testdata/atlas_first_service")
			semanticProvider := &atlasFirstAcceptanceProvider{
				t: t, repositoryType: "service",
			}
			server := httptest.NewServer(semanticProvider)
			defer server.Close()
			clearLLMEnv(t)
			configureAtlasFirstAcceptanceProvider(t, server.URL)

			entryProvider := &entryCallCompressionRuntimeProvider{invalid: test.invalid}
			runsDir := t.TempDir()
			var stderr strings.Builder
			err := runDefaultWithDeps(
				repository,
				[]string{
					"--debug-dir", runsDir,
					"--lang", "en",
					"--no-cache",
					"--no-open",
					"--no-serve",
				},
				defaultRunDeps{
					ctx: context.Background(), stdout: io.Discard, stderr: &stderr,
					newEntryCallClient: func() (entryCallCompressionClient, error) {
						return entryProvider, nil
					},
					newTargetPortfolioClient: func() (targetPortfolioClient, error) {
						return &targetPortfolioClientStub{response: []byte(`{}`)}, nil
					},
				},
			)
			if err != nil {
				t.Fatalf("runDefaultWithDeps: %v\nstderr:\n%s", err, stderr.String())
			}

			runDir := atlasFirstAcceptanceRunDir(t, runsDir)
			manifest, data := readAtlasFirstAcceptanceRun(t, runDir)
			if _, err := report.ReadRunManifest(runDir); err != nil {
				t.Fatalf("second ReadRunManifest: %v", err)
			}
			statusRaw, err := os.ReadFile(filepath.Join(runDir, entrycall.StatusArtifactFilename))
			if err != nil {
				t.Fatal(err)
			}
			status, err := entrycall.DecodeStatus(statusRaw)
			if err != nil {
				t.Fatal(err)
			}

			if test.accepted {
				if status.State != entrycall.StatusAccepted || entryProvider.calls != 1 ||
					data.EntryCall == nil || len(data.EntryCall.Families) == 0 ||
					manifest.MaterialInputs.EntryCallStatusSHA256 == "" ||
					manifest.MaterialInputs.EntryCallResultSHA256 == "" {
					t.Fatalf(
						"accepted publication status/provider/projection/material = %#v / %d / %#v / %#v",
						status, entryProvider.calls, data.EntryCall, manifest.MaterialInputs,
					)
				}
				openable := make(map[string]struct{}, len(manifest.OpenablePaths))
				for _, sourcePath := range manifest.OpenablePaths {
					openable[sourcePath] = struct{}{}
				}
				captured := make(map[string]struct{}, len(manifest.CapturedInputs))
				for _, input := range manifest.CapturedInputs {
					captured[input.Path] = struct{}{}
				}
				for _, family := range data.EntryCall.Families {
					for _, callsite := range family.Callsites {
						if _, ok := openable[callsite.Path]; !ok {
							t.Fatalf("entry-call callsite %q is not openable", callsite.Path)
						}
						if _, ok := captured[callsite.Path]; !ok {
							t.Fatalf("entry-call callsite %q is not captured", callsite.Path)
						}
					}
				}
			} else {
				if status.State != entrycall.StatusRejected ||
					status.Reason != entrycall.ReasonResponseRejected || data.EntryCall != nil ||
					manifest.MaterialInputs.EntryCallStatusSHA256 != "" ||
					manifest.MaterialInputs.EntryCallResultSHA256 != "" {
					t.Fatalf(
						"closed publication status/projection/material = %#v / %#v / %#v",
						status, data.EntryCall, manifest.MaterialInputs,
					)
				}
				if _, err := os.Lstat(filepath.Join(runDir, entrycall.ResultArtifactFilename)); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("closed entry-call result artifact = %v", err)
				}
			}
		})
	}
}

func newEntryCallCompressionRunDir(t *testing.T, metadata bool) string {
	t.Helper()
	writer, err := debugdump.NewWriter(t.TempDir(), "run", false)
	if err != nil {
		t.Fatal(err)
	}
	if metadata {
		if err := writer.WriteMetadata(debugdump.RunMeta{}); err != nil {
			t.Fatal(err)
		}
	}
	runDir := writer.RunDir()
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return runDir
}

func entryCallCompressionTestState() freshness.RepositoryState {
	return freshness.RepositoryState{
		Version: freshness.RepositoryStateVersion, Identity: "/repo",
		Head: strings.Repeat("a", 40), Dirty: []freshness.DirtyFile{}, Submodules: []freshness.SubmoduleState{},
	}
}

func entryCallCompressionTestSubstrate() *entrycall.Substrate {
	location := func(path string, line int) entrycall.Location {
		return entrycall.Location{Path: path, Line: line, Column: 1}
	}
	return &entrycall.Substrate{
		Version: entrycall.SubstrateVersion, State: entrycall.StateReady,
		Roots: []entrycall.ExactRoot{{NodeID: "main"}},
		Nodes: []entrycall.ExactNode{
			{ID: "main", Label: "main · main", Declaration: location("main.go", 10)},
			{ID: "init", Label: "routers · InitAPI", Declaration: location("routers/router.go", 20)},
			{ID: "router", Label: "web · Router", External: true},
		},
		Families: []entrycall.ExactFamily{
			{ID: "main-init", CallerID: "main", CalleeID: "init", Invocation: entrycall.InvocationSynchronous, WitnessCount: 1, Callsites: []entrycall.Location{location("main.go", 12)}},
			{ID: "init-router", CallerID: "init", CalleeID: "router", Invocation: entrycall.InvocationSynchronous, WitnessCount: 2, Callsites: []entrycall.Location{location("routers/router.go", 22), location("routers/router.go", 23)}},
		},
		Frontiers: []entrycall.ExactFrontier{},
	}
}

func entryCallCompressionThirteenFamilySubstrate() *entrycall.Substrate {
	substrate := entryCallCompressionTestSubstrate()
	location := func(line int) entrycall.Location {
		return entrycall.Location{Path: "main.go", Line: line, Column: 1}
	}
	for index := 0; index < 11; index++ {
		id := fmt.Sprintf("helper-%02d", index)
		substrate.Nodes = append(substrate.Nodes, entrycall.ExactNode{
			ID: id, Label: fmt.Sprintf("main · helper%02d", index), Declaration: location(100 + index),
		})
		substrate.Families = append(substrate.Families, entrycall.ExactFamily{
			ID: "main-" + id, CallerID: "main", CalleeID: id,
			Invocation: entrycall.InvocationSynchronous, WitnessCount: 1,
			Callsites: []entrycall.Location{location(120 + index)},
		})
	}
	return substrate
}
