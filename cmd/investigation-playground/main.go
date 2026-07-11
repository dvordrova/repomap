package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	goplsanalyzer "github.com/dvordrova/repomap/internal/analyzer/golang/gopls"
	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/envfile"
	"github.com/dvordrova/repomap/internal/investigation"
	"github.com/dvordrova/repomap/internal/sourceexplain"
	"github.com/dvordrova/repomap/internal/symbol"
	"github.com/dvordrova/repomap/internal/testevidence"
)

type config struct {
	repoPath       string
	symbolQuery    string
	outDir         string
	callDeepSeek   bool
	goplsBinary    string
	commandTimeout time.Duration
	maxCandidates  int
	maxIncoming    int
	maxOutgoing    int
}

type runArtifacts struct {
	graphJSON       []byte
	rawSource       []byte
	evaluationJSON  []byte
	parseWarnings   []byte
	deepseekRequest []byte
}

func main() {
	_ = envfile.Load(".env")
	var cfg config
	flag.StringVar(&cfg.repoPath, "repo", ".", "path to a Go repository")
	flag.StringVar(&cfg.symbolQuery, "symbol", "", "exact gopls workspace symbol name")
	flag.StringVar(&cfg.outDir, "out-dir", "tmp/investigation-playground", "artifact output directory")
	flag.BoolVar(&cfg.callDeepSeek, "deepseek", false, "execute the source-assessment action with DeepSeek")
	flag.StringVar(&cfg.goplsBinary, "gopls", "gopls", "gopls binary")
	flag.DurationVar(&cfg.commandTimeout, "command-timeout", 2*time.Minute, "timeout for each gopls command")
	flag.IntVar(&cfg.maxCandidates, "max-candidates", 12, "maximum fuzzy symbol candidates")
	flag.IntVar(&cfg.maxIncoming, "max-incoming", 30, "maximum incoming calls")
	flag.IntVar(&cfg.maxOutgoing, "max-outgoing", 30, "maximum outgoing calls")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cfg config) error {
	if strings.TrimSpace(cfg.symbolQuery) == "" {
		return fmt.Errorf("--symbol is required")
	}
	if strings.TrimSpace(cfg.outDir) == "" {
		return fmt.Errorf("--out-dir is required")
	}
	revision, err := repositoryRevision(ctx, cfg.repoPath)
	if err != nil {
		return err
	}

	analyzer := goplsanalyzer.New(goplsanalyzer.Options{
		Binary:         cfg.goplsBinary,
		MaxSymbols:     max(cfg.maxCandidates, 40),
		MaxCallRoots:   1,
		CommandTimeout: cfg.commandTimeout,
	})
	runner := investigation.Runner{
		Analyzer:        analyzer,
		ReferenceFinder: analyzer,
		SymbolOptions: symbol.Options{
			MaxCandidates:    cfg.maxCandidates,
			MaxIncomingCalls: cfg.maxIncoming,
			MaxOutgoingCalls: cfg.maxOutgoing,
		},
		TestOptions: testevidence.Options{},
	}
	var deepseekClient *deepseek.Client
	if cfg.callDeepSeek {
		deepseekClient, err = deepseek.NewFromEnv()
		if err != nil {
			return err
		}
		runner.SourceAssessor = deepseekClient
	}

	session, _, err := investigation.Reduce(investigation.Session{}, investigation.Event{
		Kind: investigation.EventStarted,
		Start: &investigation.StartInput{
			Goal:       investigation.Goal{Text: "understand " + cfg.symbolQuery},
			Repository: investigation.Repository{Path: cfg.repoPath, Revision: revision},
			Focus:      investigation.Focus{Kind: investigation.FocusSymbol, Symbol: cfg.symbolQuery},
		},
	})
	if err != nil {
		return err
	}
	artifacts := runArtifacts{}
	var runErr error
	for len(session.Next) == 1 {
		action := session.Next[0]
		if action.Kind == investigation.ActionAwaitUser ||
			(action.Kind == investigation.ActionAssessSource && !cfg.callDeepSeek) {
			break
		}
		if action.Kind == investigation.ActionAssessSource && deepseekClient != nil {
			bundleJSON, marshalErr := json.Marshal(action.AssessSource)
			if marshalErr != nil {
				return marshalErr
			}
			artifacts.deepseekRequest, err = deepseekClient.SourcePromptJSON(bundleJSON)
			if err != nil {
				return err
			}
		}
		execution, executeErr := runner.Execute(ctx, session, action)
		if executeErr != nil {
			return executeErr
		}
		if execution.Graph != nil {
			artifacts.graphJSON, err = json.MarshalIndent(execution.Graph, "", "  ")
			if err != nil {
				return err
			}
		}
		if execution.SourceExplanation != nil {
			artifacts.rawSource = append([]byte{}, execution.SourceExplanation.Raw...)
			artifacts.evaluationJSON, err = json.MarshalIndent(execution.SourceExplanation.Evaluation, "", "  ")
			if err != nil {
				return err
			}
			artifacts.parseWarnings, err = json.MarshalIndent(execution.SourceExplanation.Parsed.Warnings, "", "  ")
			if err != nil {
				return err
			}
		}
		if execution.DiagnosticError != nil {
			runErr = fmt.Errorf("%s: %w", action.Kind, execution.DiagnosticError)
		}
		session, _, err = investigation.Reduce(session, execution.Event)
		if err != nil {
			return err
		}
		if session.State == investigation.StateBlocked || session.State == investigation.StateCanceled {
			break
		}
	}
	if err := writeRun(cfg.outDir, session, artifacts); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "investigation state: %s, sequence: %d\n", session.State, session.Sequence)
	if len(session.Next) == 1 {
		fmt.Fprintf(os.Stderr, "next action: %s (%s)\n", session.Next[0].Kind, session.Next[0].Reason)
	}
	fmt.Fprintf(os.Stderr, "wrote investigation artifacts to %s\n", cfg.outDir)
	return runErr
}

func writeRun(dir string, session investigation.Session, artifacts runArtifacts) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	values := map[string]any{
		"investigation_session.json": session,
	}
	if session.Symbol != nil {
		values["symbol_bundle.json"] = session.Symbol
	}
	if session.Source != nil {
		values["source_card.json"] = session.Source
	}
	if session.Assessment != nil {
		values["source_assessment_bundle.json"] = session.Assessment
	}
	if session.SourceReport != nil {
		values["source_report.json"] = session.SourceReport
	}
	if session.Tests != nil {
		values["test_evidence.json"] = session.Tests
	}
	for name, value := range values {
		data, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal %s: %w", name, err)
		}
		if err := writeArtifact(dir, name, data); err != nil {
			return err
		}
	}
	rawArtifacts := map[string][]byte{
		"evidence_graph.json":                   artifacts.graphJSON,
		"deepseek_source_request.redacted.json": artifacts.deepseekRequest,
		"deepseek_source_response.raw.txt":      artifacts.rawSource,
		"source_evaluation.json":                artifacts.evaluationJSON,
		"source_parse_warnings.json":            artifacts.parseWarnings,
	}
	for name, data := range rawArtifacts {
		if len(data) == 0 {
			continue
		}
		if err := writeArtifact(dir, name, data); err != nil {
			return err
		}
	}
	return nil
}

func writeArtifact(dir, name string, data []byte) error {
	path := filepath.Join(dir, name)
	temporary := path + ".tmp"
	data = append(append([]byte{}, data...), '\n')
	if err := os.WriteFile(temporary, data, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}
	if err := os.Rename(temporary, path); err != nil {
		return fmt.Errorf("rename %s: %w", name, err)
	}
	return nil
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

var _ sourceexplain.Assessor = (*deepseek.Client)(nil)
