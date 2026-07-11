package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	goplsanalyzer "github.com/dvordrova/repomap/internal/analyzer/golang/gopls"
	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/investigation"
	"github.com/dvordrova/repomap/internal/sourceexplain"
	"github.com/dvordrova/repomap/internal/symbol"
	"github.com/dvordrova/repomap/internal/testevidence"
)

type config struct {
	repoPath        string
	symbolQuery     string
	orientationJSON string
	flowID          string
	resumePath      string
	finish          bool
	continueRun     bool
	repoExplicit    bool
	outDir          string
	callDeepSeek    bool
	goplsBinary     string
	commandTimeout  time.Duration
	maxCandidates   int
	maxIncoming     int
	maxOutgoing     int
}

func main() {
	var cfg config
	flag.StringVar(&cfg.repoPath, "repo", ".", "path to a Go repository")
	flag.StringVar(&cfg.symbolQuery, "symbol", "", "exact gopls workspace symbol name (start or redirect)")
	flag.StringVar(&cfg.orientationJSON, "orientation-json", "", "orientation report to hand off from")
	flag.StringVar(&cfg.flowID, "flow-id", "", "selected flow ID from --orientation-json")
	flag.StringVar(&cfg.resumePath, "resume", "", "saved investigation_session.json to resume")
	flag.BoolVar(&cfg.finish, "finish", false, "finish a resumed session that is waiting for the user")
	flag.BoolVar(&cfg.continueRun, "continue", false, "execute the pending capability in a resumed session")
	flag.StringVar(&cfg.outDir, "out-dir", "tmp/investigation-playground", "artifact output directory")
	flag.BoolVar(&cfg.callDeepSeek, "deepseek", false, "execute the source-assessment action with DeepSeek")
	flag.StringVar(&cfg.goplsBinary, "gopls", "gopls", "gopls binary")
	flag.DurationVar(&cfg.commandTimeout, "command-timeout", 2*time.Minute, "timeout for each gopls command")
	flag.IntVar(&cfg.maxCandidates, "max-candidates", 12, "maximum fuzzy symbol candidates")
	flag.IntVar(&cfg.maxIncoming, "max-incoming", 30, "maximum incoming calls")
	flag.IntVar(&cfg.maxOutgoing, "max-outgoing", 30, "maximum outgoing calls")
	flag.Parse()
	flag.Visit(func(value *flag.Flag) {
		if value.Name == "repo" {
			cfg.repoExplicit = true
		}
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cfg config) error {
	if strings.TrimSpace(cfg.outDir) == "" {
		return fmt.Errorf("--out-dir is required")
	}
	session, stopAfterPreparation, preserveRunArtifacts, err := prepareSession(ctx, cfg)
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
	artifacts := runArtifacts{}
	var runErr error
	for !stopAfterPreparation && len(session.Next) == 1 {
		action := session.Next[0]
		if action.Kind == investigation.ActionAwaitUser ||
			(action.Kind == investigation.ActionAssessSource && !cfg.callDeepSeek) {
			break
		}
		if action.Kind == investigation.ActionAssessSource {
			if deepseekClient == nil {
				deepseekClient, err = deepseek.NewFromEnv()
				if err != nil {
					return err
				}
				runner.SourceAssessor = deepseekClient
			}
			artifacts.sourceProvider = "deepseek"
			artifacts.sourceModel = deepseekClient.Model
			artifacts.sourcePromptVersion = deepseek.SourcePromptVersionJSON
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
	preserveRunArtifacts = preserveRunArtifacts && resumeOwnsRunArtifacts(cfg.resumePath, cfg.outDir)
	if err := writeRun(cfg.outDir, session, artifacts, preserveRunArtifacts); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "investigation state: %s, sequence: %d\n", session.State, session.Sequence)
	if len(session.Next) == 1 {
		fmt.Fprintf(os.Stderr, "next action: %s (%s)\n", session.Next[0].Kind, session.Next[0].Reason)
		if session.Next[0].AwaitUser != nil {
			fmt.Fprintf(os.Stderr, "question: %s\n", session.Next[0].AwaitUser.Question)
			fmt.Fprintf(os.Stderr, "choices: %s\n", formatChoices(session.Next[0].AwaitUser.Choices))
		}
	}
	fmt.Fprintf(os.Stderr, "wrote investigation artifacts to %s\n", cfg.outDir)
	return runErr
}

func formatChoices(choices []investigation.UserChoice) string {
	values := make([]string, len(choices))
	for index, choice := range choices {
		values[index] = string(choice)
	}
	return strings.Join(values, ", ")
}

var _ sourceexplain.Assessor = (*deepseek.Client)(nil)
