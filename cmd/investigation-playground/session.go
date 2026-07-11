package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	goplsanalyzer "github.com/dvordrova/repomap/internal/analyzer/golang/gopls"
	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/investigation"
	"github.com/dvordrova/repomap/internal/memory"
	"github.com/dvordrova/repomap/internal/orient"
	"github.com/dvordrova/repomap/internal/sourcecard"
	"github.com/dvordrova/repomap/internal/sourceexplain"
	"github.com/dvordrova/repomap/internal/symbol"
	"github.com/dvordrova/repomap/internal/testevidence"
)

const maxSavedInputBytes = 32 << 20

const investigationFactCollector = "investigation-facts"

type loadedSession struct {
	Session investigation.Session
	Native  bool
	Changes []freshness.Difference
}

func prepareSession(ctx context.Context, cfg config) (investigation.Session, bool, bool, error) {
	cfg.resumePath = strings.TrimSpace(cfg.resumePath)
	cfg.symbolQuery = strings.TrimSpace(cfg.symbolQuery)
	cfg.orientationJSON = strings.TrimSpace(cfg.orientationJSON)
	cfg.flowID = strings.TrimSpace(cfg.flowID)
	if cfg.resumePath != "" {
		return prepareResumedSession(ctx, cfg)
	}
	if cfg.continueRun {
		return investigation.Session{}, false, false, fmt.Errorf("--continue requires --resume")
	}
	if cfg.finish {
		return investigation.Session{}, false, false, fmt.Errorf("--finish requires --resume")
	}
	if cfg.symbolQuery == "" {
		return investigation.Session{}, false, false, fmt.Errorf("--symbol is required")
	}
	if (cfg.orientationJSON == "") != (cfg.flowID == "") {
		return investigation.Session{}, false, false, fmt.Errorf("--orientation-json and --flow-id must be used together")
	}
	repository, err := freshness.CaptureRepository(ctx, cfg.repoPath)
	if err != nil {
		return investigation.Session{}, false, false, err
	}
	revision, err := repository.Digest()
	if err != nil {
		return investigation.Session{}, false, false, err
	}
	repoPath := repository.Identity

	goal := investigation.Goal{Text: "understand " + cfg.symbolQuery}
	var origin *investigation.Origin
	if cfg.orientationJSON != "" {
		reportJSON, err := readBoundedFile("orientation report", cfg.orientationJSON)
		if err != nil {
			return investigation.Session{}, false, false, err
		}
		selected, err := orient.SelectFlow(reportJSON, cfg.flowID)
		if err != nil {
			return investigation.Session{}, false, false, err
		}
		repoName := filepath.Base(repoPath)
		if selected.RepoName != repoName {
			return investigation.Session{}, false, false, fmt.Errorf("orientation report is for repository %q, not %q", selected.RepoName, repoName)
		}
		goal.Text = fmt.Sprintf("investigate flow %q through %s", selected.FlowName, cfg.symbolQuery)
		origin = &investigation.Origin{
			Kind:             investigation.OriginOrientationFlow,
			Status:           investigation.OriginCandidate,
			ReportSHA256:     selected.ReportSHA256,
			RepoName:         selected.RepoName,
			FlowID:           selected.FlowID,
			FlowName:         selected.FlowName,
			AcceptedRevision: revision,
		}
	}

	session, _, err := investigation.Reduce(investigation.Session{}, investigation.Event{
		Kind: investigation.EventStarted,
		Start: &investigation.StartInput{
			Goal:       goal,
			Repository: investigation.Repository{Path: repoPath, Revision: revision},
			Focus:      investigation.Focus{Kind: investigation.FocusSymbol, Symbol: cfg.symbolQuery},
			Origin:     origin,
		},
	})
	return session, false, false, err
}

func prepareResumedSession(ctx context.Context, cfg config) (investigation.Session, bool, bool, error) {
	if cfg.repoExplicit {
		return investigation.Session{}, false, false, fmt.Errorf("--repo cannot be combined with --resume; the saved canonical repository path is authoritative")
	}
	if cfg.orientationJSON != "" || cfg.flowID != "" {
		return investigation.Session{}, false, false, fmt.Errorf("orientation flags cannot be combined with --resume")
	}
	if cfg.finish && cfg.symbolQuery != "" {
		return investigation.Session{}, false, false, fmt.Errorf("--finish and --symbol are mutually exclusive when resuming")
	}
	if cfg.continueRun && (cfg.finish || cfg.symbolQuery != "") {
		return investigation.Session{}, false, false, fmt.Errorf("--continue cannot be combined with --finish or --symbol")
	}
	data, err := readBoundedFile("investigation session", cfg.resumePath)
	if err != nil {
		return investigation.Session{}, false, false, err
	}
	loaded, err := loadInvestigationSession(ctx, cfg, data)
	if err != nil {
		return investigation.Session{}, false, false, err
	}
	session := loaded.Session
	if !filepath.IsAbs(session.Repository.Path) {
		return investigation.Session{}, false, false, fmt.Errorf("saved investigation session has a non-canonical repository path")
	}
	if loaded.Native && len(loaded.Changes) > 0 {
		return session, true, false, nil
	}
	revision, err := repositoryRevision(ctx, session.Repository.Path)
	if err != nil {
		return investigation.Session{}, false, false, err
	}
	if revision != session.Repository.Revision {
		next, _, err := investigation.Reduce(session, investigation.Event{
			Kind:     investigation.EventRepositoryChanged,
			Revision: revision,
		})
		return next, true, false, err
	}
	if !loaded.Native && session.Symbol != nil {
		next, _, err := investigation.Reduce(session, investigation.Event{
			Kind:    investigation.EventFactContextChanged,
			Message: "legacy session has no versioned fact freshness context",
		})
		return next, true, false, err
	}
	if cfg.continueRun {
		if err := validateContinue(session, cfg.callDeepSeek); err != nil {
			return investigation.Session{}, false, false, err
		}
	}
	if cfg.finish {
		if !pendingChoice(session, investigation.ChoiceFinish) {
			return investigation.Session{}, false, false, fmt.Errorf("--finish requires a session waiting with the finish choice")
		}
		next, _, err := investigation.Reduce(session, investigation.Event{
			Kind:     investigation.EventFinished,
			ActionID: session.Next[0].ID,
			Message:  "finished by user",
		})
		return next, false, true, err
	}
	if symbolQuery := cfg.symbolQuery; symbolQuery != "" {
		if session.State != investigation.StateWaitingUser ||
			(!pendingChoice(session, investigation.ChoiceReadCallee) && !pendingChoice(session, investigation.ChoiceInspectTests)) {
			return investigation.Session{}, false, false, fmt.Errorf("--symbol can redirect only a session waiting for the next evidence choice")
		}
		if symbolQuery == session.Focus.Symbol {
			return investigation.Session{}, false, false, fmt.Errorf("--symbol must differ from the current focus when resuming")
		}
		next, _, err := investigation.Reduce(session, investigation.Event{
			Kind: investigation.EventRedirected,
			Redirect: &investigation.RedirectInput{
				Goal:  redirectGoal(session, symbolQuery),
				Focus: investigation.Focus{Kind: investigation.FocusSymbol, Symbol: symbolQuery},
			},
		})
		return next, false, false, err
	}
	return session, !cfg.continueRun, true, nil
}

func loadInvestigationSession(ctx context.Context, cfg config, data []byte) (loadedSession, error) {
	var header struct {
		MemoryVersion   *int                      `json:"memory_version"`
		Repository      investigation.Repository  `json:"repository"`
		RepositoryState freshness.RepositoryState `json:"repository_state"`
		FactsRef        json.RawMessage           `json:"facts_ref"`
		ClaimsRef       json.RawMessage           `json:"claims_ref"`
	}
	if err := json.Unmarshal(data, &header); err != nil {
		return loadedSession{}, fmt.Errorf("decode investigation session header: %w", err)
	}
	if header.MemoryVersion == nil {
		var session investigation.Session
		if err := json.Unmarshal(data, &session); err != nil {
			return loadedSession{}, fmt.Errorf("decode investigation session: %w", err)
		}
		if err := session.Validate(); err != nil {
			return loadedSession{}, fmt.Errorf("invalid investigation session: %w", err)
		}
		return loadedSession{Session: session}, nil
	}
	if !filepath.IsAbs(header.Repository.Path) {
		return loadedSession{}, fmt.Errorf("saved investigation session has a non-canonical repository path")
	}
	currentRepository, err := freshness.CaptureRepository(ctx, header.Repository.Path)
	if err != nil {
		return loadedSession{}, err
	}
	current := memory.Current{Repository: currentRepository}
	if len(freshness.CompareRepository(header.RepositoryState, currentRepository)) == 0 && hasJSONReference(header.FactsRef) {
		facts, err := captureInvestigationFactContext(ctx, cfg, currentRepository)
		if err != nil {
			return loadedSession{}, err
		}
		current.Facts = &facts
	}
	if hasJSONReference(header.ClaimsRef) {
		current.Claims = &freshness.ClaimContext{
			Version:          freshness.ClaimContextVersion,
			PromptVersion:    deepseek.SourcePromptVersionJSON,
			ParserVersion:    sourceexplain.ParserVersion,
			EvaluatorVersion: sourceexplain.EvaluationVersion,
		}
	}
	record, err := memory.Load(cfg.resumePath, current)
	if err != nil {
		return loadedSession{}, err
	}
	return loadedSession{Session: record.Session, Native: true, Changes: record.Changes}, nil
}

func captureInvestigationFactContext(
	ctx context.Context,
	cfg config,
	repository freshness.RepositoryState,
) (freshness.FactContext, error) {
	maxCandidates := cfg.maxCandidates
	if maxCandidates <= 0 {
		maxCandidates = 12
	}
	maxIncoming := cfg.maxIncoming
	if maxIncoming <= 0 {
		maxIncoming = 30
	}
	maxOutgoing := cfg.maxOutgoing
	if maxOutgoing <= 0 {
		maxOutgoing = 30
	}
	analyzerOptions, err := json.Marshal(struct {
		MaxSymbols   int `json:"max_symbols"`
		MaxCallRoots int `json:"max_call_roots"`
	}{
		MaxSymbols:   max(maxCandidates, 40),
		MaxCallRoots: 1,
	})
	if err != nil {
		return freshness.FactContext{}, err
	}
	collectorOptions, err := json.Marshal(struct {
		MaxCandidates int `json:"max_candidates"`
		MaxIncoming   int `json:"max_incoming"`
		MaxOutgoing   int `json:"max_outgoing"`
	}{
		MaxCandidates: maxCandidates,
		MaxIncoming:   maxIncoming,
		MaxOutgoing:   maxOutgoing,
	})
	if err != nil {
		return freshness.FactContext{}, err
	}
	return freshness.CaptureGoFactContext(ctx, repository, freshness.GoOptions{
		GoBinary:         cfg.goBinary,
		GoplsBinary:      cfg.goplsBinary,
		Collector:        investigationFactCollector,
		CollectorVersion: investigationFactCollectorVersion(),
		AnalyzerOptions:  string(analyzerOptions),
		CollectorOptions: string(collectorOptions),
	})
}

func investigationFactCollectorVersion() string {
	return fmt.Sprintf(
		"gopls-%d.symbol-%d.source-%d.assessment-%d.tests-%d",
		goplsanalyzer.CollectorVersion,
		symbol.BundleVersion,
		sourcecard.Version,
		sourceexplain.BundleVersion,
		testevidence.BundleVersion,
	)
}

func hasJSONReference(raw json.RawMessage) bool {
	return len(raw) > 0 && string(raw) != "null"
}

func validateContinue(session investigation.Session, callDeepSeek bool) error {
	if len(session.Next) != 1 {
		return fmt.Errorf("--continue requires one pending capability action")
	}
	switch session.Next[0].Kind {
	case investigation.ActionResolveSymbol, investigation.ActionReadSource, investigation.ActionFindTests:
		return nil
	case investigation.ActionAssessSource:
		if !callDeepSeek {
			return fmt.Errorf("--continue requires --deepseek for a pending assess_source action")
		}
		return nil
	default:
		return fmt.Errorf("--continue cannot execute pending action %q", session.Next[0].Kind)
	}
}

func redirectGoal(session investigation.Session, symbolQuery string) investigation.Goal {
	if session.Origin != nil {
		return investigation.Goal{Text: fmt.Sprintf("investigate flow %q through %s", session.Origin.FlowName, symbolQuery)}
	}
	return investigation.Goal{Text: "understand " + symbolQuery}
}

func pendingChoice(session investigation.Session, choice investigation.UserChoice) bool {
	if len(session.Next) != 1 || session.Next[0].AwaitUser == nil {
		return false
	}
	for _, candidate := range session.Next[0].AwaitUser.Choices {
		if candidate == choice {
			return true
		}
	}
	return false
}

func readBoundedFile(label, path string) ([]byte, error) {
	data, err := readLimitedFile(path, maxSavedInputBytes)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", label, err)
	}
	return data, nil
}

func resumeOwnsRunArtifacts(resumePath, outDir string) bool {
	resumePath = strings.TrimSpace(resumePath)
	if resumePath == "" {
		return false
	}
	resumeInfo, err := os.Stat(resumePath)
	if err != nil {
		return false
	}
	outputInfo, err := os.Stat(filepath.Join(outDir, "investigation_session.json"))
	if err != nil {
		return false
	}
	return os.SameFile(resumeInfo, outputInfo)
}

func readLimitedFile(path string, limit int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("file exceeds %d bytes", limit)
	}
	return data, nil
}

func canonicalRepositoryPath(ctx context.Context, repoPath string) (string, error) {
	state, err := freshness.CaptureRepository(ctx, repoPath)
	if err != nil {
		return "", err
	}
	return state.Identity, nil
}

// repositoryRevision is the stable digest of the canonical root, HEAD, and
// every non-ignored dirty path's current contents.
func repositoryRevision(ctx context.Context, repoPath string) (string, error) {
	state, err := freshness.CaptureRepository(ctx, repoPath)
	if err != nil {
		return "", err
	}
	return state.Digest()
}
