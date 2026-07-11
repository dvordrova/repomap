package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/dvordrova/repomap/internal/investigation"
	"github.com/dvordrova/repomap/internal/orient"
)

const maxSavedInputBytes = 32 << 20

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
	repoPath, err := canonicalRepositoryPath(ctx, cfg.repoPath)
	if err != nil {
		return investigation.Session{}, false, false, err
	}
	revision, err := repositoryRevision(ctx, repoPath)
	if err != nil {
		return investigation.Session{}, false, false, err
	}

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
	var session investigation.Session
	if err := json.Unmarshal(data, &session); err != nil {
		return investigation.Session{}, false, false, fmt.Errorf("decode investigation session: %w", err)
	}
	if err := session.Validate(); err != nil {
		return investigation.Session{}, false, false, fmt.Errorf("invalid investigation session: %w", err)
	}
	if !filepath.IsAbs(session.Repository.Path) {
		return investigation.Session{}, false, false, fmt.Errorf("saved investigation session has a non-canonical repository path")
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
	root, err := gitOutput(ctx, repoPath, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(strings.TrimSpace(string(root)))
	if err != nil {
		return "", fmt.Errorf("resolve repository root: %w", err)
	}
	absolute, err := filepath.Abs(resolved)
	if err != nil {
		return "", fmt.Errorf("resolve repository root: %w", err)
	}
	return filepath.Clean(absolute), nil
}

// repositoryRevision is intentionally a coarse M2 freshness marker. M4 still
// needs content hashes for dirty files and analyzer/build metadata.
func repositoryRevision(ctx context.Context, repoPath string) (string, error) {
	head, err := gitOutput(ctx, repoPath, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	status, err := gitOutput(ctx, repoPath, "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return "", err
	}
	revision := strings.TrimSpace(string(head))
	if len(status) > 0 {
		revision += "-dirty"
	}
	return revision, nil
}

func gitOutput(ctx context.Context, repoPath string, args ...string) ([]byte, error) {
	commandArgs := append([]string{"-C", repoPath}, args...)
	command := exec.CommandContext(ctx, "git", commandArgs...)
	output, err := command.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return output, nil
}
