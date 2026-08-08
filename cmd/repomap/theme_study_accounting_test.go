package main

import (
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlasstudy"
	"github.com/dvordrova/repomap/internal/debugdump"
)

func TestThemeStudyCompletionExplainsCoProjection(t *testing.T) {
	details := strings.Join(themeStudyCompletionDetails(themeStudyRunOutcome{
		PublishedCards: 5, ScoutAccepted: 6, AdjAccepted: 6, CoProjected: 1,
	}), "\n")
	for _, want := range []string{
		"theme cards: 5",
		"scout 6/6 · adjudication 6/6",
		"equivalent themes merged into existing cards: 1 · alternate readings retained",
	} {
		if !strings.Contains(details, want) {
			t.Fatalf("completion details missing %q:\n%s", want, details)
		}
	}
}

func TestThemeStudyProviderAccountingAggregatesOnlyCurrentLiveCalls(t *testing.T) {
	liveScout := themeScoutStageOutcome{
		State: atlasstudy.ProductStateAccepted, ScoutAccepted: 2,
		SemanticCalls: 1, RequestBytes: 100, ResponseBytes: 40,
		InputTokens: 11, OutputTokens: 5, TransportAttempts: 2, LatencyMillis: 7,
	}
	cachedScout := liveScout
	cachedScout.Cached = true
	cachedScout.SemanticCalls = 0
	cachedScout.RequestBytes = 900
	cachedScout.ResponseBytes = 400
	cachedScout.InputTokens = 110
	cachedScout.OutputTokens = 50
	cachedScout.TransportAttempts = 0
	cachedScout.LatencyMillis = 70

	liveAdjudication := themeAdjStageOutcome{
		State: atlasstudy.ProductStateAccepted, AdjAccepted: 1,
		SemanticCalls: 1, RequestBytes: 300, ResponseBytes: 60,
		InputTokens: 13, OutputTokens: 8, TransportAttempts: 3, LatencyMillis: 9,
	}
	cachedAdjudication := liveAdjudication
	cachedAdjudication.Cached = true
	cachedAdjudication.SemanticCalls = 0
	cachedAdjudication.RequestBytes = 800
	cachedAdjudication.ResponseBytes = 600
	cachedAdjudication.InputTokens = 130
	cachedAdjudication.OutputTokens = 80
	cachedAdjudication.TransportAttempts = 0
	cachedAdjudication.LatencyMillis = 90

	failedAdjudication := themeAdjStageOutcome{
		State: atlasstudy.ProductStateFailed, FailureCode: atlasstudy.FailureProvider,
		SemanticCalls: 1, RequestBytes: 250, ResponseBytes: 17,
		InputTokens: 19, OutputTokens: 2, TransportAttempts: 4, LatencyMillis: 12,
	}

	tests := []struct {
		name         string
		scout        themeScoutStageOutcome
		adjudication themeAdjStageOutcome
		state        atlasstudy.ProductState
		cached       bool
		calls        int
		request      int
		response     int
		inputTokens  int
		outputTokens int
		attempts     int
		latency      int64
		diagnostic   string
	}{
		{
			name: "live_live", scout: liveScout, adjudication: liveAdjudication,
			state: atlasstudy.ProductStateAccepted,
			calls: 2, request: 400, response: 100, inputTokens: 24,
			outputTokens: 13, attempts: 5, latency: 16, diagnostic: "accepted",
		},
		{
			name: "cache_live", scout: cachedScout, adjudication: liveAdjudication,
			state: atlasstudy.ProductStateAccepted,
			calls: 1, request: 300, response: 60, inputTokens: 13,
			outputTokens: 8, attempts: 3, latency: 9, diagnostic: "accepted",
		},
		{
			name: "live_cache", scout: liveScout, adjudication: cachedAdjudication,
			state: atlasstudy.ProductStateAccepted,
			calls: 1, request: 100, response: 40, inputTokens: 11,
			outputTokens: 5, attempts: 2, latency: 7, diagnostic: "accepted",
		},
		{
			name: "cache_cache", scout: cachedScout, adjudication: cachedAdjudication,
			state: atlasstudy.ProductStateAccepted, cached: true,
			diagnostic: "cache_hit",
		},
		{
			name: "adjudication_failure", scout: liveScout, adjudication: failedAdjudication,
			state: atlasstudy.ProductStateFailed,
			calls: 2, request: 350, response: 57, inputTokens: 30,
			outputTokens: 7, attempts: 6, latency: 19, diagnostic: "failed",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			outcome := themeOutcomeWithAdjudication(
				themeOutcomeFromScout(test.scout), test.adjudication,
			)
			outcome.State = test.state
			outcome.FailureCode = test.adjudication.FailureCode
			if outcome.Cached != test.cached || outcome.SemanticCalls != test.calls ||
				outcome.RequestBytes != test.request || outcome.ResponseBytes != test.response ||
				outcome.InputTokens != test.inputTokens || outcome.OutputTokens != test.outputTokens ||
				outcome.TransportAttempts != test.attempts || outcome.LatencyMillis != test.latency {
				t.Fatalf("aggregate = %#v", outcome)
			}

			diagnostic := atlasStudyAtlasFirstDiagnostic(outcome, nil, true)
			if diagnostic.State != test.diagnostic ||
				diagnostic.RequestBytes != outcome.RequestBytes ||
				diagnostic.SemanticCalls != outcome.SemanticCalls ||
				diagnostic.TransportAttempts != outcome.TransportAttempts ||
				diagnostic.LatencyMillis != outcome.LatencyMillis {
				t.Fatalf("diagnostic = %#v, aggregate = %#v", diagnostic, outcome)
			}

			runDir := t.TempDir()
			writer, err := debugdump.OpenWriter(runDir, true)
			if err != nil {
				t.Fatal(err)
			}
			if err := writer.WriteMetadata(debugdump.RunMeta{
				RunID: "theme-accounting", Command: "atlas-first",
			}); err != nil {
				t.Fatal(err)
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			if err := recordAtlasFirstStageDiagnostic(runDir, diagnostic); err != nil {
				t.Fatal(err)
			}
			metadata := readAtlasFirstMetadataFixture(t, runDir)
			if metadata.ProviderRequestCount != outcome.SemanticCalls ||
				metadata.ExternalRequestBytes != outcome.RequestBytes ||
				len(metadata.RequestAttempts) != 1 {
				t.Fatalf("metadata = %#v, aggregate = %#v", metadata, outcome)
			}
			attempt := metadata.RequestAttempts[0]
			if attempt.ProviderCallCount != outcome.SemanticCalls ||
				attempt.RequestBytes != outcome.RequestBytes ||
				attempt.TransportAttemptCount != outcome.TransportAttempts {
				t.Fatalf("metadata attempt = %#v, aggregate = %#v", attempt, outcome)
			}
			if test.cached {
				if attempt.LatencyMillis != nil || metadata.ProviderLatencyMillis != nil {
					t.Fatalf("cached metadata exposes historical latency: %#v", metadata)
				}
			} else if attempt.LatencyMillis == nil ||
				*attempt.LatencyMillis != outcome.LatencyMillis ||
				metadata.ProviderLatencyMillis == nil ||
				*metadata.ProviderLatencyMillis != outcome.LatencyMillis {
				t.Fatalf("metadata latency = %#v, aggregate = %#v", metadata, outcome)
			}
		})
	}
}
