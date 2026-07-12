package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	goplsanalyzer "github.com/dvordrova/repomap/internal/analyzer/golang/gopls"
	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/orient"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/reportserver"
)

func main() {
	// Handle --help and --version at top level
	if len(os.Args) >= 2 {
		switch os.Args[1] {
		case "--help", "-h", "help":
			printUsage()
			return
		case "--version", "-v":
			fmt.Println("repomap (dev)")
			return
		}
	}

	if len(os.Args) < 2 {
		if err := runDefault(".", nil); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	// repomap <repo> [flags]
	if len(os.Args) >= 2 && !strings.HasPrefix(os.Args[1], "-") && os.Args[1] != "orient" && os.Args[1] != "doctor" && os.Args[1] != "serve" && os.Args[1] != "dev" {
		if err := runDefault(os.Args[1], os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	// repomap [flags] analyses the current directory.
	if strings.HasPrefix(os.Args[1], "-") {
		if err := runDefault(".", os.Args[1:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	switch os.Args[1] {
	case "serve":
		if err := runServe(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "doctor":
		if err := runDoctor(os.Args[2:], os.Stdout, os.Stderr); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "orient":
		if err := runOrient(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "dev":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Usage: repomap dev render-report <run-dir> | prompt-versions")
			os.Exit(2)
		}
		switch os.Args[2] {
		case "render-report":
			if len(os.Args) < 4 {
				fmt.Fprintf(os.Stderr, "Usage: repomap dev render-report <.repomap-runs/<run-id>>\n")
				os.Exit(2)
			}
			if err := runRenderReport(os.Args[3]); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		case "prompt-versions":
			if len(os.Args) != 3 {
				fmt.Fprintln(os.Stderr, "Usage: repomap dev prompt-versions")
				os.Exit(2)
			}
			printPromptVersions(os.Stdout)
		default:
			fmt.Fprintf(os.Stderr, "unknown dev command: %s\n", os.Args[2])
			os.Exit(2)
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		os.Exit(2)
	}
}

func printPromptVersions(writer io.Writer) {
	fmt.Fprintf(
		writer,
		"{\"orientation_json\":%q,\"source_json\":%q,\"symbol_json\":%q,\"symbol_tagged\":%q}\n",
		deepseek.OrientationPromptVersionJSON,
		deepseek.SourcePromptVersionJSON,
		deepseek.SymbolPromptVersionJSON,
		deepseek.SymbolPromptVersionTagged,
	)
}

func linkLatest(debugDir, runDir string, stderr io.Writer) {
	latest := filepath.Join(debugDir, "latest")
	os.Remove(latest)
	if err := os.Symlink(filepath.Base(runDir), latest); err != nil {
		fmt.Fprintf(stderr, "warning: could not create latest symlink: %v\n", err)
	}
}

func runDefault(repo string, extraArgs []string) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	return runDefaultWithDeps(repo, extraArgs, defaultRunDeps{
		ctx:         ctx,
		stdout:      os.Stdout,
		stderr:      os.Stderr,
		openReport:  openReport,
		serveReport: reportserver.Serve,
		captureRepo: freshness.CaptureRepository,
	})
}

type defaultRunDeps struct {
	ctx         context.Context
	stdout      io.Writer
	stderr      io.Writer
	openReport  func(string) error
	serveReport func(context.Context, reportserver.Options) error
	captureRepo func(context.Context, string) (freshness.RepositoryState, error)
}

func runDefaultWithDeps(repo string, extraArgs []string, deps defaultRunDeps) error {
	fs := flag.NewFlagSet("repomap", flag.ContinueOnError)
	fs.SetOutput(deps.stderr)

	jsonOut := fs.Bool("json", false, "print combined JSON report instead of text")
	offline := fs.Bool("offline", false, "skip model calls, build local facts/bundles only")
	flows := fs.Int("flows", 0, "number of top candidate directions to expand after orientation")
	discoverSurfaces := fs.Bool("discover-surfaces", true, "discover bounded Go runtime surfaces for the report")
	noDebug := fs.Bool("no-debug", false, "disable debug artifact writing")
	noOpen := fs.Bool("no-open", false, "do not open the generated HTML report")
	noServe := fs.Bool("no-serve", false, "generate a static report without starting the local server")
	port := fs.Int("port", 0, "local report server port (default: random)")
	debugDir := fs.String("debug-dir", defaultDebugDir(), "directory for debug artifacts")
	dumpLLM := fs.Bool("dump-llm", false, "dump LLM request/response to debug dir")
	previewRequest := fs.Bool("preview-request", false, "print the exact redacted LLM request without sending it")
	strictSnapshot := fs.Bool("strict-snapshot", false, "fail when captured analyzed inputs change before report publication")
	out := fs.String("out", "", "write output to file instead of stdout")

	if err := fs.Parse(extraArgs); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		if repo != "." || fs.NArg() != 1 {
			return fmt.Errorf("unexpected positional arguments: %s", strings.Join(fs.Args(), " "))
		}
		repo = fs.Arg(0)
	}
	if *flows < 0 {
		return fmt.Errorf("--flows cannot be negative")
	}
	if *port < 0 || *port > 65535 {
		return fmt.Errorf("--port must be between 0 and 65535")
	}
	absRepo, err := filepath.Abs(repo)
	if err != nil {
		return fmt.Errorf("resolve repository path: %w", err)
	}
	repo = absRepo

	dDir := *debugDir
	if *noDebug {
		dDir = ""
	}

	var runID string
	artifactRun := dDir != "" && !*previewRequest
	if artifactRun {
		runID = debugdump.GenerateRunID(repoRunLabel(repo))
	}

	ctx := deps.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	captureRepo := deps.captureRepo
	if captureRepo == nil {
		captureRepo = freshness.CaptureRepository
	}
	var (
		runDir       string
		analysisRoot string
		initialState freshness.RepositoryState
	)
	if artifactRun {
		runDir = filepath.Join(dDir, runID)
		if err := report.RemoveRunManifest(runDir); err != nil {
			return fmt.Errorf("invalidate previous browser report authority: %w", err)
		}
		analysisRoot, err = resolveAnalysisRoot(repo)
		if err != nil {
			return err
		}
		initialState, err = captureRepo(ctx, repo)
		if err != nil {
			return fmt.Errorf("capture repository state before orientation: %w", err)
		}
	}

	opts := orient.Options{
		RepoPath:               repo,
		LLMRequestOnly:         *previewRequest,
		OutputJSON:             *jsonOut,
		Offline:                *offline,
		FlowCount:              *flows,
		RunID:                  runID,
		DebugDir:               dDir,
		DumpLLM:                *dumpLLM,
		DumpRedacted:           true,
		RequireArtifacts:       dDir != "" && !*previewRequest,
		DiscoverSurfaces:       *discoverSurfaces && artifactRun,
		MaxLLMFiles:            60,
		MaxLocalDirectionFiles: 20,
		MaxLLMEdges:            60,
		MaxLLMModules:          20,
		MaxLLMEntrypoints:      20,
		MaxLLMSignals:          30,
		MaxLLMSignalsPerFile:   3,
		MaxReadmeBytes:         40000,
		MaxReadmeLLMBytes:      6000,
		MaxTreeLines:           800,
		MaxInterestingFiles:    400,
		MaxGoPkgs:              600,
		MaxGoEdges:             1000,
		EffectiveOptions: debugdump.EffectiveOptions{
			Offline:          *offline,
			FlowCount:        *flows,
			DiscoverSurfaces: *discoverSurfaces && artifactRun,
			DumpLLM:          *dumpLLM,
			OutputJSON:       *jsonOut,
			PreviewRequest:   *previewRequest,
			NoOpen:           *noOpen,
			NoServe:          *noServe,
			Port:             *port,
			DebugEnabled:     dDir != "",
		},
	}
	showProgress := !*jsonOut && *out == "" && !*previewRequest
	if showProgress {
		opts.Progress = func(event orient.ProgressEvent) {
			switch event.Stage {
			case orient.ProgressSnapshotStarted:
				fmt.Fprintf(deps.stderr, "repomap: collecting tracked repository facts from %s\n", event.RepoPath)
			case orient.ProgressSnapshotReady:
				fmt.Fprintf(deps.stderr, "repomap: repository facts ready in %d ms\n", event.LatencyMillis)
			case orient.ProgressBundleReady:
				fmt.Fprintf(
					deps.stderr,
					"repomap: compact local context %d bytes in %d ms\n",
					event.BundleBytes,
					event.LatencyMillis,
				)
			case orient.ProgressSurfaceStarted:
				fmt.Fprintln(deps.stderr, "repomap: discovering local Go runtime surfaces")
			case orient.ProgressSurfaceReady:
				fmt.Fprintf(
					deps.stderr,
					"repomap: discovered %d local runtime surface(s) in %d ms\n",
					event.SurfaceCount,
					event.LatencyMillis,
				)
			case orient.ProgressSurfaceFailed:
				fmt.Fprintf(deps.stderr, "repomap: warning: %s\n", event.Warning)
			case orient.ProgressModelRequest:
				fmt.Fprintf(deps.stderr, "repomap: asking %s with %d-byte request\n", event.Model, event.RequestBytes)
			case orient.ProgressOrientationDone:
				fmt.Fprintf(
					deps.stderr,
					"repomap: validated %d candidate direction(s) in %d ms\n",
					event.CandidateCount,
					event.LatencyMillis,
				)
			}
		}
	}

	output, err := orient.Run(ctx, opts)
	if err != nil {
		if artifactRun {
			return fmt.Errorf("%w\nrequest diagnostics: %s", err, filepath.Join(runDir, "metadata.json"))
		}
		return err
	}

	var reportPath string
	if artifactRun {
		if !*offline {
			architectureStarted := time.Now()
			fmt.Fprintln(deps.stderr, "repomap: synthesizing bounded architecture grouping")
			if _, err := synthesizeArchitectureForRun(ctx, runDir, deps.stderr); err != nil {
				fmt.Fprintf(deps.stderr, "warning: %v; architecture map will be unavailable (after %d ms)\n", err, time.Since(architectureStarted).Milliseconds())
			}
		}
		reconciliationStarted := time.Now()
		reportData, err := report.ReadRunDir(runDir)
		if err != nil {
			return fmt.Errorf("read captured report inputs: %w", err)
		}
		currentState, err := captureRepo(ctx, repo)
		if err != nil {
			return fmt.Errorf("capture repository state after analysis: %w", err)
		}
		authority, err := report.ConfirmRunAuthorityScoped(
			ctx, analysisRoot, initialState, currentState, report.CapturedInputPaths(reportData), *strictSnapshot,
		)
		if err != nil {
			return fmt.Errorf("confirm browser report authority: %w", err)
		}
		fmt.Fprintf(deps.stderr, "repomap: reconciled %d captured input(s) in %d ms\n",
			len(report.CapturedInputPaths(reportData)), time.Since(reconciliationStarted).Milliseconds())
		if authority.Freshness().State != freshness.FreshnessFresh {
			fmt.Fprintf(deps.stderr, "repomap: snapshot freshness: %s\n", authority.Freshness().State)
		}
		reportStarted := time.Now()
		if err := report.GenerateAuthorized(runDir, authority); err != nil {
			return fmt.Errorf("generate authorized browser report: %w", err)
		}
		fmt.Fprintf(deps.stderr, "repomap: generated authorized report in %d ms\n", time.Since(reportStarted).Milliseconds())
		reportPath = filepath.Join(runDir, "report.html")
		fmt.Fprintf(deps.stderr, "Report: %s\n", reportPath)
		linkLatest(dDir, runDir, deps.stderr)
	}

	if *out != "" {
		return os.WriteFile(*out, output, 0o644)
	}

	if _, err := deps.stdout.Write(output); err != nil {
		return fmt.Errorf("write stdout: %w", err)
	}
	if !*previewRequest && (len(output) == 0 || output[len(output)-1] != '\n') {
		fmt.Fprintln(deps.stdout)
	}

	interactiveReport := reportPath != "" && !*jsonOut && *out == ""
	if interactiveReport && !*noServe && deps.serveReport != nil {
		localAnalyzer := newReportAnalyzer()
		return deps.serveReport(ctx, reportserver.Options{
			RunsDir:             dDir,
			InitialRunID:        runID,
			Port:                *port,
			LocationResolver:    localAnalyzer,
			ExactSymbolAnalyzer: localAnalyzer,
			ReferenceFinder:     localAnalyzer,
			Logf: func(format string, args ...any) {
				fmt.Fprintf(deps.stderr, "repomap: "+format+"\n", args...)
			},
			OnReady: func(url string) error {
				fmt.Fprintf(deps.stderr, "Serving reports: %s (press Ctrl-C to stop)\n", url)
				if !*noOpen && deps.openReport != nil {
					if err := deps.openReport(url); err != nil {
						fmt.Fprintf(deps.stderr, "warning: could not open report: %v\n", err)
					}
				}
				return nil
			},
		})
	}
	if interactiveReport && !*noOpen && deps.openReport != nil {
		if err := deps.openReport(reportPath); err != nil {
			fmt.Fprintf(deps.stderr, "warning: could not open report: %v\n", err)
		}
	}
	return nil
}

func runServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	runsDir := fs.String("debug-dir", defaultDebugDir(), "directory containing saved report runs")
	runID := fs.String("run", "", "saved run to open (default: latest)")
	port := fs.Int("port", 0, "local report server port (default: random)")
	noOpen := fs.Bool("no-open", false, "do not open the report in a browser")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("serve: unexpected positional arguments: %s", strings.Join(fs.Args(), " "))
	}
	if *runsDir == "" {
		return fmt.Errorf("serve: report directory is unavailable; pass --debug-dir")
	}
	if *port < 0 || *port > 65535 {
		return fmt.Errorf("serve: --port must be between 0 and 65535")
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	localAnalyzer := newReportAnalyzer()
	return reportserver.Serve(ctx, reportserver.Options{
		RunsDir:             *runsDir,
		InitialRunID:        *runID,
		Port:                *port,
		LocationResolver:    localAnalyzer,
		ExactSymbolAnalyzer: localAnalyzer,
		ReferenceFinder:     localAnalyzer,
		Logf: func(format string, args ...any) {
			fmt.Fprintf(os.Stderr, "repomap: "+format+"\n", args...)
		},
		OnReady: func(url string) error {
			fmt.Fprintf(os.Stderr, "Serving reports: %s (press Ctrl-C to stop)\n", url)
			if !*noOpen {
				if err := openReport(url); err != nil {
					fmt.Fprintf(os.Stderr, "warning: could not open report: %v\n", err)
				}
			}
			return nil
		},
	})
}

func newReportAnalyzer() *goplsanalyzer.Analyzer {
	return goplsanalyzer.New(goplsanalyzer.Options{
		MaxSymbols:     20,
		MaxCallers:     30,
		MaxCallees:     30,
		CommandTimeout: 30 * time.Second,
	})
}

func runOrient(args []string) error {
	fs := flag.NewFlagSet("orient", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	repo := fs.String("repo", "", "path to local git repository")
	snapshotOnly := fs.Bool("snapshot-only", false, "print local snapshot JSON only")
	llmBundleOnly := fs.Bool("llm-bundle-only", false, "print compact LLM bundle (no API call)")
	llmRequestOnly := fs.Bool("llm-request-only", false, "print exact redacted LLM request (no API call)")
	out := fs.String("out", "", "write output to file")
	debugDir := fs.String("debug-dir", "", "directory for debug artifacts")
	dumpLLM := fs.Bool("dump-llm", false, "dump LLM request/response")
	explainFlows := fs.Int("explain-flows", 0, "explain top N candidate flows")
	flowBundlesOnly := fs.Bool("flow-bundles-only", false, "build flow bundles only")
	strictSnapshot := fs.Bool("strict-snapshot", false, "fail when captured analyzed inputs change before report publication")
	maxLLMFiles := fs.Int("max-llm-files", 150, "max files in LLM bundle")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if *explainFlows < 0 {
		return fmt.Errorf("--explain-flows cannot be negative")
	}
	if *repo == "" {
		return fmt.Errorf("--repo is required")
	}
	absRepo, err := filepath.Abs(*repo)
	if err != nil {
		return fmt.Errorf("resolve repository path: %w", err)
	}
	*repo = absRepo

	dDir := *debugDir
	var runID string
	reportArtifacts := dDir != "" && !*snapshotOnly && !*llmBundleOnly && !*llmRequestOnly
	if reportArtifacts {
		runID = debugdump.GenerateRunID(repoRunLabel(*repo))
	}

	ctx := context.Background()
	var (
		runDir       string
		analysisRoot string
		initialState freshness.RepositoryState
	)
	if reportArtifacts {
		runDir = filepath.Join(dDir, runID)
		if err := report.RemoveRunManifest(runDir); err != nil {
			return fmt.Errorf("invalidate previous report authority: %w", err)
		}
		analysisRoot, err = resolveAnalysisRoot(*repo)
		if err != nil {
			return err
		}
		initialState, err = freshness.CaptureRepository(ctx, *repo)
		if err != nil {
			return fmt.Errorf("capture repository state before orientation: %w", err)
		}
	}

	opts := orient.Options{
		RepoPath:               *repo,
		SnapshotOnly:           *snapshotOnly,
		LLMBundleOnly:          *llmBundleOnly,
		LLMRequestOnly:         *llmRequestOnly,
		OutputJSON:             true,
		FlowCount:              *explainFlows,
		FlowBundlesOnly:        *flowBundlesOnly,
		RunID:                  runID,
		DebugDir:               dDir,
		DumpLLM:                *dumpLLM,
		DumpRedacted:           true,
		RequireArtifacts:       dDir != "",
		MaxLLMFiles:            *maxLLMFiles,
		MaxLocalDirectionFiles: 20,
		MaxLLMEdges:            500,
		MaxLLMSignals:          80,
		MaxLLMSignalsPerFile:   3,
		MaxLLMModules:          40,
		MaxLLMEntrypoints:      40,
		MaxReadmeBytes:         40000,
		MaxReadmeLLMBytes:      12000,
		MaxTreeLines:           800,
		MaxInterestingFiles:    400,
		MaxGoPkgs:              600,
		MaxGoEdges:             1000,
	}

	output, err := orient.Run(ctx, opts)
	if err != nil {
		return err
	}

	if reportArtifacts {
		if _, err := synthesizeArchitectureForRun(ctx, runDir, os.Stderr); err != nil {
			fmt.Fprintf(os.Stderr, "warning: %v; architecture map will be unavailable\n", err)
		}
		reportData, err := report.ReadRunDir(runDir)
		if err != nil {
			return fmt.Errorf("read captured report inputs: %w", err)
		}
		currentState, err := freshness.CaptureRepository(ctx, *repo)
		if err != nil {
			return fmt.Errorf("capture repository state after analysis: %w", err)
		}
		authority, err := report.ConfirmRunAuthorityScoped(
			ctx, analysisRoot, initialState, currentState, report.CapturedInputPaths(reportData), *strictSnapshot,
		)
		if err != nil {
			return fmt.Errorf("confirm report authority: %w", err)
		}
		if err := report.GenerateAuthorized(runDir, authority); err != nil {
			return fmt.Errorf("generate authorized report: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Report: %s/report.html\n", runDir)
		linkLatest(dDir, runDir, os.Stderr)
	}

	if *out != "" {
		return os.WriteFile(*out, output, 0o644)
	}

	if _, err := os.Stdout.Write(output); err != nil {
		return fmt.Errorf("write stdout: %w", err)
	}
	if !*llmRequestOnly && (len(output) == 0 || output[len(output)-1] != '\n') {
		fmt.Println()
	}
	return nil
}

func runDoctor(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] != "llm" {
		return fmt.Errorf("usage: repomap doctor llm [--check]")
	}
	fs := flag.NewFlagSet("doctor llm", flag.ContinueOnError)
	fs.SetOutput(stderr)
	check := fs.Bool("check", false, "send a tiny synthetic JSON request without repository content")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("doctor llm: unexpected positional arguments")
	}

	client, err := deepseek.NewFromEnv()
	if err != nil {
		return fmt.Errorf("doctor llm: %w", err)
	}
	fmt.Fprintf(stdout, "LLM configuration OK\n")
	fmt.Fprintf(stdout, "endpoint: %s\n", client.Endpoint)
	fmt.Fprintf(stdout, "model: %s\n", client.Model)
	fmt.Fprintf(stdout, "auth: %s\n", client.Auth)
	fmt.Fprintf(stdout, "timeout: %s\n", client.HTTPClient.Timeout)
	fmt.Fprintf(stdout, "max_tokens: %d\n", client.MaxTokens)
	if !*check {
		fmt.Fprintln(stdout, "network_check: skipped (use --check)")
		return nil
	}

	if err := client.CheckJSONCompatibility(context.Background()); err != nil {
		return fmt.Errorf("doctor llm: compatibility request failed: %w", err)
	}
	fmt.Fprintln(stdout, "network_check: passed")
	return nil
}

func defaultDebugDir() string {
	cacheDir, err := os.UserCacheDir()
	if err != nil || strings.TrimSpace(cacheDir) == "" {
		return ""
	}
	return filepath.Join(cacheDir, "repomap", "runs")
}

func resolveAnalysisRoot(repositoryPath string) (string, error) {
	resolved, err := filepath.EvalSymlinks(repositoryPath)
	if err != nil {
		return "", fmt.Errorf("resolve analysis root: %w", err)
	}
	absolute, err := filepath.Abs(resolved)
	if err != nil {
		return "", fmt.Errorf("resolve analysis root: %w", err)
	}
	root := filepath.Clean(absolute)
	info, err := os.Stat(root)
	if err != nil {
		return "", fmt.Errorf("inspect analysis root: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("analysis root is not a directory: %s", root)
	}
	return root, nil
}

func repoRunLabel(repo string) string {
	absPath, err := filepath.Abs(repo)
	if err == nil {
		return filepath.Base(absPath)
	}
	return filepath.Base(filepath.Clean(repo))
}

func runRenderReport(runDir string) error {
	absDir, err := filepath.Abs(runDir)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}
	if err := report.Generate(absDir); err != nil {
		return err
	}
	fmt.Printf("Report: %s/report.html\n", absDir)
	return nil
}

func printUsage() {
	fmt.Fprintf(os.Stderr, "Usage: repomap [repo] [flags]\n")
	fmt.Fprintf(os.Stderr, "       repomap doctor llm [--check]\n")
	fmt.Fprintf(os.Stderr, "       repomap serve [--run RUN_ID] [--port PORT]\n")
	fmt.Fprintf(os.Stderr, "       repomap orient --repo <repo> [flags]\n")
	fmt.Fprintf(os.Stderr, "\nFlags:\n")
	fmt.Fprintf(os.Stderr, "  --json          output JSON instead of text\n")
	fmt.Fprintf(os.Stderr, "  --offline       skip model calls, local facts only\n")
	fmt.Fprintf(os.Stderr, "  --flows N       expand top N directions after orientation (default 0)\n")
	fmt.Fprintf(os.Stderr, "  --discover-surfaces discover bounded Go runtime surfaces (default true)\n")
	fmt.Fprintf(os.Stderr, "  --no-debug      disable debug artifact writing\n")
	fmt.Fprintf(os.Stderr, "  --no-open       do not open the generated HTML report\n")
	fmt.Fprintf(os.Stderr, "  --no-serve      generate a static report without starting the local server\n")
	fmt.Fprintf(os.Stderr, "  --port PORT     local report server port (default random)\n")
	fmt.Fprintf(os.Stderr, "  --debug-dir DIR debug artifact directory (default user cache)\n")
	fmt.Fprintf(os.Stderr, "  --dump-llm      dump LLM request/response in debug dir\n")
	fmt.Fprintf(os.Stderr, "  --preview-request print exact redacted request without an API call\n")
	fmt.Fprintf(os.Stderr, "  --strict-snapshot fail if captured analyzed inputs change during the run\n")
	fmt.Fprintf(os.Stderr, "  --help, -h      show this help\n")
	fmt.Fprintf(os.Stderr, "  --version       show version\n")
	fmt.Fprintf(os.Stderr, "\nEnvironment:\n")
	fmt.Fprintf(os.Stderr, "  REPOMAP_LLM_ENDPOINT full OpenAI-compatible chat/completions URL\n")
	fmt.Fprintf(os.Stderr, "  REPOMAP_LLM_MODEL\n")
	fmt.Fprintf(os.Stderr, "  REPOMAP_LLM_API_KEY (for bearer auth)\n")
	fmt.Fprintf(os.Stderr, "  REPOMAP_LLM_AUTH    bearer (default) or none\n")
	fmt.Fprintf(os.Stderr, "  REPOMAP_LLM_TIMEOUT (default 60s)\n")
	fmt.Fprintf(os.Stderr, "  DEEPSEEK_API_KEY    quick setup; defaults to deepseek-v4-flash\n")
	fmt.Fprintf(os.Stderr, "  DEEPSEEK_*          compatibility configuration aliases\n")
	fmt.Fprintf(os.Stderr, "\nExamples:\n")
	fmt.Fprintf(os.Stderr, "  repomap\n")
	fmt.Fprintf(os.Stderr, "  repomap ../etcd\n")
	fmt.Fprintf(os.Stderr, "  repomap ../etcd --offline\n")
	fmt.Fprintf(os.Stderr, "  repomap ../etcd --preview-request > /tmp/repomap-request.json\n")
	fmt.Fprintf(os.Stderr, "  repomap doctor llm --check\n")
	fmt.Fprintf(os.Stderr, "  repomap serve\n")
	fmt.Fprintf(os.Stderr, "  repomap ../etcd --flows 2 --json | jq .\n")
	fmt.Fprintf(os.Stderr, "  repomap --help\n")
}
