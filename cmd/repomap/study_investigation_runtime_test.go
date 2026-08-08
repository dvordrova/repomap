package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/mechanismstudy"
	"github.com/dvordrova/repomap/internal/modelresearch"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/surfacediscovery"
	"github.com/dvordrova/repomap/internal/themestudy"
)

const (
	studyInvestigationTestRevision  = "0123456789abcdef0123456789abcdef01234567"
	studyInvestigationTestFreshness = "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
)

func TestValidateStudyInvestigationRepositoryFreshnessRejectsAnyDrift(t *testing.T) {
	initial := freshness.RepositoryState{
		Version:  freshness.RepositoryStateVersion,
		Identity: "/repo",
		Head:     studyInvestigationTestRevision,
		Dirty:    []freshness.DirtyFile{},
	}
	if err := validateStudyInvestigationRepositoryFreshness(initial, initial); err != nil {
		t.Fatalf("unchanged repository: %v", err)
	}

	changed := initial
	changed.Dirty = []freshness.DirtyFile{{
		Status:        "modified",
		Path:          "unrelated.txt",
		Kind:          freshness.FileRegular,
		Mode:          "100644",
		ContentSHA256: strings.Repeat("a", 64),
	}}
	if err := validateStudyInvestigationRepositoryFreshness(initial, changed); err == nil ||
		!strings.Contains(err.Error(), "repository changed") {
		t.Fatalf("drift error = %v", err)
	}
}

type studyInvestigationRuntimeProvider struct {
	requests    []mechanismstudy.Request
	calls       int
	failCall    int
	failErr     error
	attempts    int
	failContent []byte
}

func (provider *studyInvestigationRuntimeProvider) MechanismStudyPromptJSON(
	prompt mechanismstudy.Prompt,
) ([]byte, error) {
	const marker = "Exact request bundle JSON:\n"
	position := strings.LastIndex(prompt.User, marker)
	if position < 0 {
		return nil, errors.New("missing exact request marker")
	}
	var request mechanismstudy.Request
	if err := json.Unmarshal([]byte(prompt.User[position+len(marker):]), &request); err != nil {
		return nil, err
	}
	provider.requests = append(provider.requests, request)
	return json.Marshal(struct {
		RequestRef string `json:"request_ref"`
	}{RequestRef: request.RequestRef})
}

func (provider *studyInvestigationRuntimeProvider) MechanismStudyBodyMeasured(
	_ context.Context,
	_ []byte,
) (modelresearch.ProviderResult, error) {
	provider.calls++
	if provider.failCall == provider.calls {
		failure := provider.failErr
		if failure == nil {
			failure = errors.New("fixture provider failed")
		}
		return modelresearch.ProviderResult{
			Content: provider.failContent, Attempts: provider.attempts,
			ResponseBytes: len(provider.failContent),
		}, failure
	}
	request := provider.requests[provider.calls-1]
	response := mechanismstudy.Response{
		Version:    mechanismstudy.ResultVersion,
		CatalogRef: request.CatalogRef, CatalogSHA256: request.CatalogSHA256,
		RequestRef: request.RequestRef,
		Cards:      make([]mechanismstudy.ResponseCard, 0, len(request.Cards)),
	}
	for _, card := range request.Cards {
		response.Cards = append(response.Cards, mechanismstudy.ResponseCard{
			CardRef:    card.Ref,
			Mechanisms: studyInvestigationRuntimeCandidates(card),
		})
	}
	raw, err := json.Marshal(response)
	if err != nil {
		return modelresearch.ProviderResult{}, err
	}
	return modelresearch.ProviderResult{
		Content: raw, Attempts: 1, ResponseBytes: len(raw),
	}, nil
}

func studyInvestigationRuntimeCandidates(card mechanismstudy.Card) []mechanismstudy.Candidate {
	roots := make(map[string]struct{})
	for _, reading := range card.Readings {
		if reading.RootNodeRef != "" {
			roots[reading.RootNodeRef] = struct{}{}
		}
	}
	for _, first := range card.Edges {
		for _, second := range card.Edges {
			if first.Ref == second.Ref || first.CalleeRef != second.CallerRef ||
				first.CallerRef == second.CalleeRef {
				continue
			}
			if _, tied := roots[first.CallerRef]; !tied {
				if _, tied = roots[first.CalleeRef]; !tied {
					if _, tied = roots[second.CalleeRef]; !tied {
						continue
					}
				}
			}
			return []mechanismstudy.Candidate{{EdgeRefs: []string{second.Ref, first.Ref}}}
		}
	}
	return []mechanismstudy.Candidate{}
}

func TestStudyInvestigationZeroGraphWritesCompletePreparedFamilyWithoutProvider(t *testing.T) {
	runDir, _, _ := studyInvestigationRuntimeFixture(t, 2)
	factoryCalls := 0
	outcome, err := runStudyInvestigationForRun(
		context.Background(),
		runDir,
		nil,
		studyInvestigationTestRevision,
		studyInvestigationTestFreshness,
		newRunOutput(io.Discard),
		func() (studyInvestigationClient, error) {
			factoryCalls++
			return nil, errors.New("provider must not be configured")
		},
	)
	if err != nil {
		t.Fatalf("runStudyInvestigationForRun: %v", err)
	}
	if factoryCalls != 0 || outcome.Status.State != mechanismstudy.StatusComplete ||
		outcome.Status.ProviderCallCount != 0 || outcome.Status.PreparedCardCount != 2 ||
		len(outcome.ReportInput.Cards) != 2 {
		t.Fatalf("zero-graph outcome = %#v; factory calls=%d", outcome, factoryCalls)
	}
	for _, card := range outcome.ReportInput.Cards {
		if card.Outcome != "prepared_investigation" || len(card.Mechanisms) != 0 {
			t.Fatalf("prepared report card = %#v", card)
		}
	}
	assertStudyInvestigationArtifacts(t, runDir)
}

func TestStudyInvestigationRetainsAcceptedPrefixWhenSecondBatchFails(t *testing.T) {
	runDir, index, _ := studyInvestigationRuntimeFixture(t, 5)
	provider := &studyInvestigationRuntimeProvider{failCall: 2, attempts: 1}
	factoryCalls := 0
	outcome, err := runStudyInvestigationForRun(
		context.Background(),
		runDir,
		index,
		studyInvestigationTestRevision,
		studyInvestigationTestFreshness,
		newRunOutput(io.Discard),
		func() (studyInvestigationClient, error) {
			factoryCalls++
			return provider, nil
		},
	)
	if err != nil {
		t.Fatalf("runStudyInvestigationForRun: %v", err)
	}
	if factoryCalls != 1 || provider.calls != 2 ||
		outcome.Status.State != mechanismstudy.StatusPartial ||
		outcome.Status.AcceptedBatchCount != 1 || outcome.Status.FailedBatchCount != 1 ||
		outcome.Status.MechanismCardCount != 4 || outcome.Status.PreparedCardCount != 1 ||
		outcome.SemanticCalls != 2 || outcome.TransportAttempts != 2 {
		t.Fatalf("partial outcome = %#v; provider=%#v factory=%d", outcome.Status, provider, factoryCalls)
	}
	mechanismCards := 0
	for _, card := range outcome.ReportInput.Cards {
		if card.Outcome == "mechanism" {
			mechanismCards++
			if len(card.Mechanisms) != 1 || len(card.Mechanisms[0].Nodes) != 3 ||
				len(card.Mechanisms[0].Edges) != 2 {
				t.Fatalf("published mechanism card = %#v", card)
			}
		}
	}
	if mechanismCards != 4 {
		t.Fatalf("published mechanism cards = %d, want 4", mechanismCards)
	}
	assertStudyInvestigationArtifacts(t, runDir)
}

func TestStudyInvestigationConfigurationAndCancellationPersistFailedPreparedFamily(t *testing.T) {
	for _, test := range []struct {
		name        string
		context     func() context.Context
		factory     studyInvestigationClientFactory
		wantFactory int
		wantCalls   int
		wantAttempt int
	}{
		{
			name:    "configuration failure",
			context: context.Background,
			factory: func() (studyInvestigationClient, error) {
				return nil, errors.New("fixture configuration failed")
			},
			wantFactory: 1,
		},
		{
			name: "canceled before provider construction",
			context: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			},
			factory: func() (studyInvestigationClient, error) {
				return nil, errors.New("provider must not be constructed")
			},
		},
		{
			name:    "canceled attempted call with no transport attempt",
			context: context.Background,
			factory: func() (studyInvestigationClient, error) {
				return &studyInvestigationRuntimeProvider{
					failCall: 1, failErr: context.Canceled, attempts: 0,
				}, nil
			},
			wantFactory: 1,
			wantCalls:   1,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			runDir, index, _ := studyInvestigationRuntimeFixture(t, 1)
			factoryCalls := 0
			outcome, err := runStudyInvestigationForRun(
				test.context(), runDir, index,
				studyInvestigationTestRevision, studyInvestigationTestFreshness,
				newRunOutput(io.Discard),
				func() (studyInvestigationClient, error) {
					factoryCalls++
					return test.factory()
				},
			)
			if err != nil {
				t.Fatalf("runStudyInvestigationForRun: %v", err)
			}
			if factoryCalls != test.wantFactory || outcome.SemanticCalls != test.wantCalls ||
				outcome.TransportAttempts != test.wantAttempt ||
				outcome.Status.State != mechanismstudy.StatusFailed ||
				outcome.Status.PreparedCardCount != 1 || outcome.Status.MechanismCardCount != 0 ||
				len(outcome.ReportInput.Cards) != 1 ||
				outcome.ReportInput.Cards[0].Outcome != report.StudyInvestigationOutcomePrepared {
				t.Fatalf("outcome = %#v; factory=%d", outcome, factoryCalls)
			}
			assertStudyInvestigationArtifacts(t, runDir)
		})
	}
}

func TestStudyInvestigationCancellationGetsOnlyBoundedPublicationContext(t *testing.T) {
	parent, cancelParent := context.WithCancel(context.Background())
	cancelParent()

	ordinary, cancelOrdinary := studyInvestigationPublicationContext(
		parent,
		mechanismstudy.Status{State: mechanismstudy.StatusFailed},
	)
	defer cancelOrdinary()
	if !errors.Is(ordinary.Err(), context.Canceled) {
		t.Fatalf("ordinary context error = %v, want parent cancellation", ordinary.Err())
	}

	publication, cancelPublication := studyInvestigationPublicationContext(
		parent,
		mechanismstudy.Status{
			State:   mechanismstudy.StatusFailed,
			Batches: []mechanismstudy.BatchExecution{{State: mechanismstudy.BatchCanceled}},
		},
	)
	defer cancelPublication()
	if err := publication.Err(); err != nil {
		t.Fatalf("detached bounded publication context starts canceled: %v", err)
	}
	if _, hasDeadline := publication.Deadline(); !hasDeadline {
		t.Fatal("detached publication context has no deadline")
	}
}

func TestStudyInvestigationProviderFailureSecretKeepsExactCallJournaledAndRedacted(t *testing.T) {
	runDir, index, _ := studyInvestigationRuntimeFixture(t, 1)
	const secret = "Bearer company-secret-token-value"
	provider := &studyInvestigationRuntimeProvider{
		failCall: 1, attempts: 1, failErr: errors.New("fixture provider failed"),
		failContent: []byte(`{"error":"` + secret + `"}`),
	}
	outcome, err := runStudyInvestigationForRun(
		context.Background(), runDir, index,
		studyInvestigationTestRevision, studyInvestigationTestFreshness,
		newRunOutput(io.Discard),
		func() (studyInvestigationClient, error) { return provider, nil },
	)
	if err == nil || !strings.Contains(err.Error(), "credential scan") ||
		outcome.SemanticCalls != 1 || outcome.TransportAttempts != 1 {
		t.Fatalf("secret provider failure outcome = %#v, err=%v", outcome, err)
	}
	entries := readSemanticJournalEntries(t, runDir)
	if len(entries) != 1 || entries[0].record.Stage != debugdump.SemanticStageMechanismStudy ||
		entries[0].record.State != debugdump.SemanticStateRejected ||
		entries[0].record.ValidationCode != debugdump.SemanticValidationSecret ||
		entries[0].record.SemanticCalls != 1 || entries[0].record.TransportAttempts != 1 {
		t.Fatalf("secret exchange = %#v", entries)
	}
	if strings.Contains(string(entries[0].response), secret) {
		t.Fatal("provider credential leaked into semantic exchange response")
	}
}

func studyInvestigationRuntimeFixture(
	t *testing.T,
	cardCount int,
) (string, *surfacediscovery.DirectCallIndex, surfacediscovery.DirectCallNode) {
	t.Helper()
	repository := t.TempDir()
	if err := os.WriteFile(filepath.Join(repository, "go.mod"), []byte("module example.com/investigation\n\ngo 1.25\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repository, "main.go"), []byte(`package main

func main() { entry() }
func entry() { service() }
func service() { persist() }
func persist() {}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	analysis, err := surfacediscovery.AnalyzeContext(
		context.Background(),
		surfacediscovery.DefaultOptions(repository),
	)
	if err != nil {
		t.Fatalf("AnalyzeContext: %v", err)
	}
	if analysis.DirectCallIndex == nil {
		t.Fatal("AnalyzeContext returned no DirectCallIndex")
	}
	var root surfacediscovery.DirectCallNode
	for _, node := range analysis.DirectCallIndex.Nodes {
		if node.Symbol.Name == "entry" {
			root = node
			break
		}
	}
	if root.ID == "" {
		t.Fatalf("entry node absent: %#v", analysis.DirectCallIndex.Nodes)
	}
	cards := make([]themestudy.ThemeCard, 0, cardCount)
	for ordinal := 1; ordinal <= cardCount; ordinal++ {
		cards = append(cards, themestudy.ThemeCard{
			Ordinal: ordinal, CanonicalID: "fixture-theme-" + string(rune('a'+ordinal-1)),
			FinalTitle: "Bearer authentication", FinalQuestion: "How does startup reach persistence?",
			Readings: []themestudy.Reading{{
				Label: root.Symbol.Name, Symbol: root.Symbol.ID,
				Path: root.Declaration.Path, Line: root.Declaration.Line,
				Fit: themestudy.FitDirect,
			}},
		})
	}
	themesRaw, err := themestudy.EncodeStudyThemes(themestudy.StudyThemes{
		Version: themestudy.StudyThemesVersion, Revision: studyInvestigationTestRevision,
		ScoutSHA256: strings.Repeat("a", 64), AdjSHA256: strings.Repeat("b", 64),
		Cards: cards,
	})
	if err != nil {
		t.Fatalf("EncodeStudyThemes: %v", err)
	}
	writer, err := debugdump.NewWriter(t.TempDir(), "run", false)
	if err != nil {
		t.Fatal(err)
	}
	runDir := writer.RunDir()
	writer.Close()
	if err := os.WriteFile(
		filepath.Join(runDir, themestudy.StudyThemesArtifactFilename),
		themesRaw,
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	return runDir, analysis.DirectCallIndex, root
}

func assertStudyInvestigationArtifacts(t *testing.T, runDir string) {
	t.Helper()
	for _, name := range mechanismstudy.ArtifactFilenames {
		info, err := os.Stat(filepath.Join(runDir, name))
		if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 {
			t.Fatalf("artifact %s: info=%v err=%v", name, info, err)
		}
	}
}
