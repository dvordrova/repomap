package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unicode"

	goplsanalyzer "github.com/dvordrova/repomap/internal/analyzer/golang/gopls"
	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/guidedtour"
	"github.com/dvordrova/repomap/internal/modelresearch"
	"github.com/dvordrova/repomap/internal/orient"
	"github.com/dvordrova/repomap/internal/pavedpath"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/reportserver"
	"github.com/dvordrova/repomap/internal/semanticdiscovery"
	"github.com/dvordrova/repomap/internal/tasklens"
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
			writeDefaultRunError(os.Stderr, err)
			os.Exit(defaultRunExitCode(err))
		}
		return
	}

	// repomap <repo> [flags]
	if len(os.Args) >= 2 && !strings.HasPrefix(os.Args[1], "-") && os.Args[1] != "investigate" && os.Args[1] != "orient" && os.Args[1] != "doctor" && os.Args[1] != "serve" && os.Args[1] != "dev" {
		if err := runDefault(os.Args[1], os.Args[2:]); err != nil {
			writeDefaultRunError(os.Stderr, err)
			os.Exit(defaultRunExitCode(err))
		}
		return
	}

	// repomap [flags] analyses the current directory.
	if strings.HasPrefix(os.Args[1], "-") {
		if err := runDefault(".", os.Args[1:]); err != nil {
			writeDefaultRunError(os.Stderr, err)
			os.Exit(defaultRunExitCode(err))
		}
		return
	}

	switch os.Args[1] {
	case "investigate":
		if err := runInvestigate(os.Args[2:], investigateDependencies{}); err != nil {
			writeDefaultRunError(os.Stderr, err)
			os.Exit(defaultRunExitCode(err))
		}
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
			fmt.Fprintln(os.Stderr, "Usage: repomap dev render-report <run-dir> | v32-refresh --run-dir <copied-run-dir> --repo <repo> [--reuse-study | --operate-only | --replay-saved] | fresh-repo-onboarding --run-dir <run-dir> [--repo <repo> [--replan-saved] | --replay-saved] | guided-tour <run-dir> | guided-tour-fanout <run-dir> | guided-tour-experiment <run-dir> | semantic-discovery <run-dir> | semantic-discovery-experiment <run-dir> | golden-mechanism <run-dir> [--probe-only] | golden-mechanism-v01 <run-dir> [--replay-old] | golden-mechanism-v02 <run-dir> [--prepare | --replay] | golden-mechanism-v03 <run-dir> [--replay] | golden-mechanism-v1 <run-dir> [--prepare | --replay] | chi-request-dispatch <run-dir> [--prepare | --replay-response | --replay] | mechanism-v1 <run-dir> [--replay] | review-cockpit --caddy-run <run-dir> --chi-run <run-dir> --out <output-dir> | prompt-versions")
			os.Exit(2)
		}
		switch os.Args[2] {
		case "v32-refresh":
			if err := runV32RefreshCLI(os.Args[3:], os.Stdout, os.Stderr); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		case "render-report":
			if len(os.Args) < 4 {
				fmt.Fprintf(os.Stderr, "Usage: repomap dev render-report <.repomap-runs/<run-id>>\n")
				os.Exit(2)
			}
			if err := runRenderReport(os.Args[3]); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		case "fresh-repo-onboarding":
			if err := runFreshRepoOnboardingCLI(os.Args[3:], os.Stdout, os.Stderr); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		case "prompt-versions":
			if len(os.Args) != 3 {
				fmt.Fprintln(os.Stderr, "Usage: repomap dev prompt-versions")
				os.Exit(2)
			}
			printPromptVersions(os.Stdout)
		case "guided-tour":
			if len(os.Args) != 4 {
				fmt.Fprintln(os.Stderr, "Usage: repomap dev guided-tour <run-dir>")
				os.Exit(2)
			}
			if err := runGuidedTour(os.Args[3], os.Stderr); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		case "guided-tour-experiment":
			if len(os.Args) != 4 {
				fmt.Fprintln(os.Stderr, "Usage: repomap dev guided-tour-experiment <run-dir>")
				os.Exit(2)
			}
			if err := runGuidedTourExperiment(os.Args[3], os.Stderr); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		case "guided-tour-fanout":
			if len(os.Args) != 4 {
				fmt.Fprintln(os.Stderr, "Usage: repomap dev guided-tour-fanout <run-dir>")
				os.Exit(2)
			}
			if err := runGuidedTourFanout(os.Args[3], os.Stderr); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		case "semantic-discovery":
			if len(os.Args) != 4 {
				fmt.Fprintln(os.Stderr, "Usage: repomap dev semantic-discovery <run-dir>")
				os.Exit(2)
			}
			if err := runSemanticDiscovery(os.Args[3], os.Stderr); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		case "semantic-discovery-experiment":
			if len(os.Args) != 4 {
				fmt.Fprintln(os.Stderr, "Usage: repomap dev semantic-discovery-experiment <run-dir>")
				os.Exit(2)
			}
			if err := runSemanticDiscoveryExperiment(os.Args[3], os.Stderr); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		case "golden-mechanism":
			if err := runGoldenMechanismCLI(os.Args[3:], os.Stdout, os.Stderr); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		case "golden-mechanism-v01":
			if err := runGoldenMechanismV01CLI(os.Args[3:], os.Stdout, os.Stderr); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		case "golden-mechanism-v02":
			if err := runGoldenMechanismV02CLI(os.Args[3:], os.Stdout, os.Stderr); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		case "golden-mechanism-v03":
			if err := runGoldenMechanismV03CLI(os.Args[3:], os.Stdout, os.Stderr); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		case "golden-mechanism-v1":
			if err := runGoldenMechanismV1CLI(os.Args[3:], os.Stdout, os.Stderr); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		case "mechanism-v1":
			if err := runMechanismV1CLI(os.Args[3:], os.Stdout); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		case "chi-request-dispatch":
			if err := runChiRequestDispatchCLI(os.Args[3:], os.Stdout, os.Stderr); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		case "review-cockpit":
			if err := runReviewCockpitCLI(os.Args[3:], os.Stdout); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		default:
			fmt.Fprintf(os.Stderr, "unknown dev command: %s\n", os.Args[2])
			os.Exit(2)
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		os.Exit(2)
	}
}

func writeDefaultRunError(writer io.Writer, err error) {
	if errors.Is(err, context.Canceled) {
		fmt.Fprintln(writer, "repomap: canceled")
		return
	}
	fmt.Fprintln(writer, err)
}

func defaultRunExitCode(err error) int {
	if errors.Is(err, context.Canceled) {
		return 130
	}
	return 1
}

func printPromptVersions(writer io.Writer) {
	fmt.Fprintf(
		writer,
		"{\"orientation_json\":%q,\"source_json\":%q,\"symbol_json\":%q,\"symbol_tagged\":%q,\"guided_tour\":%q,\"guided_tour_leaf\":%q,\"guided_tour_fan_in\":%q,\"semantic_opportunity\":%q,\"semantic_leaf\":%q,\"semantic_fan_in\":%q,\"semantic_monolithic\":%q,\"golden_mechanism\":%q,\"repository_onboarding_editor\":%q,\"repository_brief_shape\":%q,\"study_direction_candidates\":%q,\"reading_pack_review\":%q,\"paved_paths\":%q,\"task_investigation\":%q}\n",
		deepseek.OrientationPromptVersionJSON,
		deepseek.SourcePromptVersionJSON,
		deepseek.SymbolPromptVersionJSON,
		deepseek.SymbolPromptVersionTagged,
		guidedtour.PromptVersion,
		guidedtour.LeafPromptVersion,
		guidedtour.FanInPromptVersion,
		semanticdiscovery.OpportunityPromptVersion,
		semanticdiscovery.LeafPromptVersion,
		semanticdiscovery.FanInPromptVersion,
		semanticdiscovery.MonolithicPromptVersion,
		semanticdiscovery.GoldenMechanismPromptVersion,
		semanticdiscovery.OnboardingEditorPromptVersion,
		semanticdiscovery.StudyBriefPromptVersion,
		semanticdiscovery.StudyCandidatesPromptVersion,
		semanticdiscovery.ReadingPackReviewPromptVersion,
		pavedpath.PromptVersion,
		tasklens.PromptVersion,
	)
}

func linkLatest(debugDir, runDir string, stderr io.Writer) {
	latest := filepath.Join(debugDir, "latest")
	os.Remove(latest)
	if err := os.Symlink(filepath.Base(runDir), latest); err != nil {
		fmt.Fprintf(stderr, "warning: could not create latest symlink: %v\n", err)
	}
}

func formatTokenUsage(inputTokens, outputTokens int) string {
	if inputTokens == 0 && outputTokens == 0 {
		return "tokens unavailable"
	}
	return fmt.Sprintf("%d input / %d output tokens", inputTokens, outputTokens)
}

func writeProgress(writer io.Writer, event orient.ProgressEvent) {
	switch event.Stage {
	case orient.ProgressSnapshotStarted:
		fmt.Fprintf(writer, "repomap: collecting tracked repository facts from %s\n", event.RepoPath)
	case orient.ProgressSnapshotReady:
		fmt.Fprintf(
			writer,
			"repomap: repository facts ready: %d tracked file(s) in %d ms\n",
			event.FileCount,
			event.LatencyMillis,
		)
	case orient.ProgressBundleReady:
		fmt.Fprintf(
			writer,
			"repomap: compact local context %d bytes across %d candidate file(s) in %d ms\n",
			event.BundleBytes,
			event.CandidateCount,
			event.LatencyMillis,
		)
	case orient.ProgressSurfaceStarted:
		fmt.Fprintln(writer, "repomap: discovering local Go runtime surfaces")
	case orient.ProgressSurfacePhase:
		elapsed := (time.Duration(event.LatencyMillis) * time.Millisecond).Round(time.Second)
		switch event.PhaseState {
		case "started":
			fmt.Fprintf(writer, "repomap: surface discovery phase %s: %s\n", event.Phase, event.Activity)
		case "completed":
			fmt.Fprintf(writer, "repomap: surface discovery phase %s completed in %d ms", event.Phase, event.LatencyMillis)
			if event.TotalCount > 0 {
				fmt.Fprintf(writer, " (%d/%d)", event.CompletedCount, event.TotalCount)
			}
			fmt.Fprintln(writer)
		default:
			fmt.Fprintf(writer, "repomap: surface discovery phase %s: %d/%d after %s\n",
				event.Phase, event.CompletedCount, event.TotalCount, elapsed)
		}
	case orient.ProgressSurfaceWaiting, orient.ProgressPlanningWaiting:
		fmt.Fprintf(
			writer,
			"repomap: %s still running after %s (Ctrl-C to cancel)\n",
			event.Activity,
			(time.Duration(event.LatencyMillis) * time.Millisecond).Round(time.Second),
		)
	case orient.ProgressSurfaceReady:
		fmt.Fprintf(
			writer,
			"repomap: discovered %d local runtime surface(s) in %d ms\n",
			event.SurfaceCount,
			event.LatencyMillis,
		)
	case orient.ProgressSurfaceFailed:
		fmt.Fprintf(writer, "repomap: warning: %s\n", event.Warning)
	case orient.ProgressModelRequest:
		fmt.Fprintf(writer, "repomap: prepared %d-byte orientation request for %s\n", event.RequestBytes, event.Model)
	case orient.ProgressProviderWaiting:
		fmt.Fprintf(
			writer,
			"repomap: %s from %s still running after %s (Ctrl-C to cancel)\n",
			event.Activity,
			event.Model,
			(time.Duration(event.LatencyMillis) * time.Millisecond).Round(time.Second),
		)
	case orient.ProgressOrientationDone:
		if event.Cached {
			fmt.Fprintf(
				writer,
				"repomap: reused cached orientation response of %d bytes (original call: %d ms, %s); validated %d candidate direction(s)\n",
				event.ResponseBytes,
				event.LatencyMillis,
				formatTokenUsage(event.InputTokens, event.OutputTokens),
				event.CandidateCount,
			)
			break
		}
		fmt.Fprintf(
			writer,
			"repomap: orientation received %d bytes in %d ms (%s); validated %d candidate direction(s)\n",
			event.ResponseBytes,
			event.LatencyMillis,
			formatTokenUsage(event.InputTokens, event.OutputTokens),
			event.CandidateCount,
		)
	case orient.ProgressResearchPrepared:
		fmt.Fprintf(
			writer,
			"repomap: targeted research prepared %d evidence item(s) from %d locally inspected file(s)\n",
			event.EvidenceCount,
			event.FileCount,
		)
	case orient.ProgressResearchDone:
		if event.Cached {
			fmt.Fprintf(
				writer,
				"repomap: reused cached targeted-research response of %d bytes from an original %d-byte request (original call: %d ms, %s); %d validated, %d rejected, %d new grounded fact(s)\n",
				event.ResponseBytes,
				event.RequestBytes,
				event.LatencyMillis,
				formatTokenUsage(event.InputTokens, event.OutputTokens),
				event.FindingCount,
				event.RejectedCount,
				event.NewFactCount,
			)
			break
		}
		fmt.Fprintf(
			writer,
			"repomap: targeted research %s: received %d bytes from a %d-byte request in %d ms (%s); %d validated, %d rejected, %d new grounded fact(s)\n",
			event.Activity,
			event.ResponseBytes,
			event.RequestBytes,
			event.LatencyMillis,
			formatTokenUsage(event.InputTokens, event.OutputTokens),
			event.FindingCount,
			event.RejectedCount,
			event.NewFactCount,
		)
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

func readSourceEpisodeFile(filePath string) ([]byte, error) {
	info, err := os.Lstat(filePath)
	if err != nil {
		return nil, fmt.Errorf("inspect source episode %q: %w", filePath, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("source episode %q must be a regular file; symlinks are not allowed", filePath)
	}
	if info.Size() < 0 || info.Size() > report.MaxSourceEpisodeBytes {
		return nil, fmt.Errorf("source episode %q exceeds the %d-byte limit", filePath, report.MaxSourceEpisodeBytes)
	}

	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open source episode %q: %w", filePath, err)
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat opened source episode %q: %w", filePath, err)
	}
	if !openedInfo.Mode().IsRegular() || !os.SameFile(info, openedInfo) {
		return nil, fmt.Errorf("source episode %q changed before it could be read", filePath)
	}
	if openedInfo.Size() < 0 || openedInfo.Size() > report.MaxSourceEpisodeBytes {
		return nil, fmt.Errorf("source episode %q exceeds the %d-byte limit", filePath, report.MaxSourceEpisodeBytes)
	}

	data, err := io.ReadAll(io.LimitReader(file, report.MaxSourceEpisodeBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read source episode %q: %w", filePath, err)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("source episode %q is empty", filePath)
	}
	if len(data) > report.MaxSourceEpisodeBytes {
		return nil, fmt.Errorf("source episode %q exceeds the %d-byte limit", filePath, report.MaxSourceEpisodeBytes)
	}
	return data, nil
}

func runDefaultWithDeps(repo string, extraArgs []string, deps defaultRunDeps) error {
	fs := flag.NewFlagSet("repomap", flag.ContinueOnError)
	fs.SetOutput(deps.stderr)

	jsonOut := fs.Bool("json", false, "print combined JSON report instead of text")
	offline := fs.Bool("offline", false, "skip model calls, build local facts/bundles only")
	flows := fs.Int("flows", 0, "number of top candidate directions to expand after orientation")
	discoverSurfaces := fs.Bool("discover-surfaces", true, "discover bounded Go runtime surfaces for the report")
	guidedTour := fs.Bool("guided-tour", true, "add an optional model-edited guided tour to the existing architecture map")
	noSearch := fs.Bool("no-search", false, "omit Super Search from the generated report")
	noDebug := fs.Bool("no-debug", false, "disable debug artifact writing")
	noOpen := fs.Bool("no-open", false, "do not open the generated HTML report")
	noServe := fs.Bool("no-serve", false, "generate a static report without starting the local server")
	port := fs.Int("port", 0, "local report server port (default: random)")
	debugDir := fs.String("debug-dir", defaultDebugDir(), "directory for debug artifacts")
	dumpLLM := fs.Bool("dump-llm", false, "dump LLM request/response to debug dir")
	previewRequest := fs.Bool("preview-request", false, "print the exact redacted LLM request without sending it")
	strictSnapshot := fs.Bool("strict-snapshot", false, "fail when captured analyzed inputs change before report publication")
	sourceEpisodePath := fs.String("source-episode", "", "render an approved bounded source episode over the generated report")
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
	var sourceEpisodeJSON []byte
	if *sourceEpisodePath != "" {
		if !artifactRun {
			return fmt.Errorf("--source-episode requires a generated report run")
		}
		sourceEpisodeJSON, err = readSourceEpisodeFile(*sourceEpisodePath)
		if err != nil {
			return err
		}
	}
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
		if sourceEpisodeJSON != nil {
			if err := report.ValidateSourceEpisodeForRevision(sourceEpisodeJSON, initialState.Head); err != nil {
				return fmt.Errorf("validate source episode before orientation: %w", err)
			}
		}
	}

	researchPolicy := modelresearch.DefaultPolicy()
	opts := orient.Options{
		RepoPath:                  repo,
		LLMRequestOnly:            *previewRequest,
		OutputJSON:                *jsonOut,
		Offline:                   *offline,
		FlowCount:                 *flows,
		RunID:                     runID,
		DebugDir:                  dDir,
		DumpLLM:                   *dumpLLM,
		DumpRedacted:              true,
		RequireArtifacts:          dDir != "" && !*previewRequest,
		DiscoverSurfaces:          *discoverSurfaces && artifactRun,
		MaxLLMFiles:               researchPolicy.Orientation.MaxFiles,
		MaxOrientationBundleBytes: researchPolicy.Orientation.MaxRequestBytes - (16 << 10),
		MaxLocalDirectionFiles:    20,
		MaxLLMEdges:               60,
		MaxLLMModules:             20,
		MaxLLMEntrypoints:         20,
		MaxLLMSignals:             30,
		MaxLLMSignalsPerFile:      3,
		MaxReadmeBytes:            40000,
		MaxReadmeLLMBytes:         6000,
		MaxTreeLines:              800,
		MaxInterestingFiles:       400,
		MaxGoPkgs:                 600,
		MaxGoEdges:                1000,
		ResearchPolicy:            researchPolicy,
		RepositoryContext:         researchRepositoryContext(initialState, repo),
		EffectiveOptions: debugdump.EffectiveOptions{
			Offline:          *offline,
			FlowCount:        *flows,
			DiscoverSurfaces: *discoverSurfaces && artifactRun,
			GuidedTour:       *guidedTour && !*offline,
			NoSearch:         *noSearch,
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
			writeProgress(deps.stderr, event)
		}
	}

	output, err := orient.Run(ctx, opts)
	if err != nil {
		if artifactRun {
			return fmt.Errorf("%w\nrequest diagnostics: %s", err, filepath.Join(runDir, "metadata.json"))
		}
		return err
	}

	runOptionalModelStages := !*offline
	if artifactRun && runOptionalModelStages {
		earlyReportData, readErr := report.ReadRunDir(runDir)
		if readErr != nil {
			return fmt.Errorf("read source catalog preflight inputs: %w", readErr)
		}
		if !sourceCatalogPathsAreRegular(
			ctx,
			initialState,
			analysisRoot,
			earlyReportData.OpenablePaths,
		) {
			runOptionalModelStages = false
			fmt.Fprintln(
				deps.stderr,
				"warning: authorized source catalog is unavailable; skipping optional model stages and publishing a view-only report",
			)
		}
	}

	var reportPath string
	if artifactRun {
		if runOptionalModelStages {
			architectureStarted := time.Now()
			fmt.Fprintln(deps.stderr, "repomap: synthesizing bounded architecture grouping")
			if _, err := synthesizeArchitectureForRun(ctx, runDir, deps.stderr); err != nil {
				if ctxErr := ctx.Err(); ctxErr != nil {
					return ctxErr
				}
				fmt.Fprintf(deps.stderr, "warning: %v; architecture map will be unavailable (after %d ms)\n", err, time.Since(architectureStarted).Milliseconds())
			}
		}
		if runOptionalModelStages && *guidedTour {
			guidedStarted := time.Now()
			outcome, guidedErr := editGuidedTourForRun(ctx, runDir, deps.stderr)
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			if guidedErr != nil {
				fmt.Fprintf(
					deps.stderr,
					"warning: %v; report will keep the full architecture map without a guided tour (after %d ms)\n",
					guidedErr,
					time.Since(guidedStarted).Milliseconds(),
				)
			} else if outcome.Skipped {
				// No saved flow or direction currently satisfies the Guided
				// Tour publication contract. This is an expected no-call
				// presentation outcome, not a product warning.
			} else if outcome.Cached {
				fmt.Fprintf(
					deps.stderr,
					"repomap: reused cached guided tour response of %d bytes for a %d-byte request\n",
					outcome.ResponseBytes,
					outcome.RequestBytes,
				)
			} else {
				fmt.Fprintf(
					deps.stderr,
					"repomap: guided tour accepted %d response bytes from a %d-byte request in %d ms (%s)\n",
					outcome.ResponseBytes,
					outcome.RequestBytes,
					outcome.LatencyMillis,
					formatTokenUsage(outcome.InputTokens, outcome.OutputTokens),
				)
			}
		}
		if runOptionalModelStages {
			semanticStarted := time.Now()
			fmt.Fprintln(deps.stderr, "repomap: selecting bounded source-backed onboarding paths from saved facts")
			freshResult, semanticErr := editFreshRepoMechanismForRun(
				ctx,
				runDir,
				repo,
				deps.stderr,
			)
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			if semanticErr != nil {
				fmt.Fprintf(
					deps.stderr,
					"warning: %v; report will keep its code-first fallback (after %d ms)\n",
					semanticErr,
					time.Since(semanticStarted).Milliseconds(),
				)
			} else if freshResult.Status.State == "published" {
				fmt.Fprintf(
					deps.stderr,
					"repomap: published %d canonical onboarding path(s) from %d proposed question(s) and %d candidate probe(s) in %d ms\n",
					len(freshResult.Status.PublishedMechanisms),
					freshResult.Status.QuestionsProposed,
					len(freshResult.Status.Attempts),
					time.Since(semanticStarted).Milliseconds(),
				)
			} else {
				fmt.Fprintf(
					deps.stderr,
					"repomap: no bounded mechanism passed local publication checks; keeping the code-first fallback (after %d ms)\n",
					time.Since(semanticStarted).Milliseconds(),
				)
			}
		}
		if runOptionalModelStages {
			studyStarted := time.Now()
			fmt.Fprintln(deps.stderr, "repomap: editing a bounded repository brief and study map")
			studyStatus, studyErr := editStudyMapForRun(ctx, runDir, repo, deps.stderr)
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			if studyErr != nil {
				fmt.Fprintf(
					deps.stderr,
					"warning: %v; report will keep its existing overview and code paths (after %d ms)\n",
					studyErr,
					time.Since(studyStarted).Milliseconds(),
				)
			} else {
				fmt.Fprintf(
					deps.stderr,
					"repomap: selected %d source-backed study direction(s) from %d proposed candidate(s) in %d ms (%s)\n",
					studyStatus.Selected,
					studyStatus.Candidates,
					time.Since(studyStarted).Milliseconds(),
					formatTokenUsage(studyStatus.Metrics.InputTokens, studyStatus.Metrics.OutputTokens),
				)
			}
		}
		if runOptionalModelStages {
			operateStarted := time.Now()
			fmt.Fprintln(deps.stderr, "repomap: collecting exact repository-owned ways to run and verify")
			operateStatus, operateErr := editPavedPathsForRun(ctx, runDir, repo, deps.stderr)
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			if operateErr != nil {
				fmt.Fprintf(
					deps.stderr,
					"warning: %v; report will keep %d exact operational landmark(s) when available (after %d ms)\n",
					operateErr,
					operateStatus.Landmarks,
					time.Since(operateStarted).Milliseconds(),
				)
			} else {
				fmt.Fprintf(
					deps.stderr,
					"repomap: published %d paved path(s) and %d exact operational landmark(s) from %d bounded item(s) in %d ms (%s)\n",
					operateStatus.Paths,
					operateStatus.Landmarks,
					operateStatus.Evidence,
					time.Since(operateStarted).Milliseconds(),
					formatTokenUsage(operateStatus.Metrics.InputTokens, operateStatus.Metrics.OutputTokens),
				)
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
		var generateErr error
		if sourceEpisodeJSON != nil {
			generateErr = report.GenerateAuthorizedWithSourceEpisode(runDir, authority, sourceEpisodeJSON)
		} else {
			generateErr = report.GenerateAuthorized(runDir, authority)
		}
		if generateErr != nil {
			return fmt.Errorf("generate authorized browser report: %w", generateErr)
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
			SourceEpisodeJSON:   sourceEpisodeJSON,
			Logf: func(format string, args ...any) {
				fmt.Fprintf(deps.stderr, "repomap: "+format+"\n", args...)
			},
			OnReady: func(url string) error {
				url = reportOverviewURL(url)
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
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	return runServeWithDeps(ctx, args, os.Stderr, openReport, reportserver.Serve)
}

func runServeWithDeps(
	ctx context.Context,
	args []string,
	stderr io.Writer,
	open func(string) error,
	serve func(context.Context, reportserver.Options) error,
) error {
	if stderr == nil {
		stderr = io.Discard
	}
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	runsDir := fs.String("debug-dir", defaultDebugDir(), "directory containing saved report runs")
	runID := fs.String("run", "", "saved run to open (default: latest)")
	port := fs.Int("port", 0, "local report server port (default: random)")
	noOpen := fs.Bool("no-open", false, "do not open the report in a browser")
	sourceEpisodePath := fs.String("source-episode", "", "render an approved bounded source episode over the selected run")
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

	var sourceEpisodeJSON []byte
	if *sourceEpisodePath != "" {
		if strings.TrimSpace(*runID) == "" {
			return fmt.Errorf("serve: --source-episode requires --run")
		}
		var err error
		sourceEpisodeJSON, err = readSourceEpisodeFile(*sourceEpisodePath)
		if err != nil {
			return fmt.Errorf("serve: %w", err)
		}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	localAnalyzer := newReportAnalyzer()
	return serve(ctx, reportserver.Options{
		RunsDir:             *runsDir,
		InitialRunID:        *runID,
		Port:                *port,
		LocationResolver:    localAnalyzer,
		ExactSymbolAnalyzer: localAnalyzer,
		ReferenceFinder:     localAnalyzer,
		SourceEpisodeJSON:   sourceEpisodeJSON,
		Logf: func(format string, args ...any) {
			fmt.Fprintf(stderr, "repomap: "+format+"\n", args...)
		},
		OnReady: func(url string) error {
			url = reportOverviewURL(url)
			fmt.Fprintf(stderr, "Serving reports: %s (press Ctrl-C to stop)\n", url)
			if !*noOpen && open != nil {
				if err := open(url); err != nil {
					fmt.Fprintf(stderr, "warning: could not open report: %v\n", err)
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
		CommandTimeout: goplsanalyzer.DefaultCommandTimeout,
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

	researchPolicy := modelresearch.DefaultPolicy()
	opts := orient.Options{
		RepoPath:                  *repo,
		SnapshotOnly:              *snapshotOnly,
		LLMBundleOnly:             *llmBundleOnly,
		LLMRequestOnly:            *llmRequestOnly,
		OutputJSON:                true,
		FlowCount:                 *explainFlows,
		FlowBundlesOnly:           *flowBundlesOnly,
		RunID:                     runID,
		DebugDir:                  dDir,
		DumpLLM:                   *dumpLLM,
		DumpRedacted:              true,
		RequireArtifacts:          dDir != "",
		MaxLLMFiles:               *maxLLMFiles,
		MaxOrientationBundleBytes: researchPolicy.Orientation.MaxRequestBytes - (16 << 10),
		MaxLocalDirectionFiles:    20,
		MaxLLMEdges:               500,
		MaxLLMSignals:             80,
		MaxLLMSignalsPerFile:      3,
		MaxLLMModules:             40,
		MaxLLMEntrypoints:         40,
		MaxReadmeBytes:            40000,
		MaxReadmeLLMBytes:         12000,
		MaxTreeLines:              800,
		MaxInterestingFiles:       400,
		MaxGoPkgs:                 600,
		MaxGoEdges:                1000,
		ResearchPolicy:            researchPolicy,
		RepositoryContext:         researchRepositoryContext(initialState, *repo),
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

const (
	maxEarlySourceCatalogPaths          = 4096
	maxEarlySourceCatalogPathBytes      = 4096
	maxEarlySourceCatalogTotalPathBytes = 64 * 1024
)

// sourceCatalogPathsAreRegular is a bounded, kind-only preflight. It neither
// follows symlinks nor reads file contents; final captured-input reconciliation
// remains the TOCTOU authority before report publication.
func sourceCatalogPathsAreRegular(
	ctx context.Context,
	state freshness.RepositoryState,
	analysisRoot string,
	sourcePaths []string,
) bool {
	if len(sourcePaths) > maxEarlySourceCatalogPaths ||
		len(state.Dirty) > maxEarlySourceCatalogPaths {
		return false
	}

	repositoryPrefix, err := filepath.Rel(state.Identity, analysisRoot)
	if err != nil || filepath.IsAbs(repositoryPrefix) ||
		repositoryPrefix == ".." ||
		strings.HasPrefix(repositoryPrefix, ".."+string(filepath.Separator)) {
		return false
	}
	if repositoryPrefix == "." {
		repositoryPrefix = ""
	} else {
		repositoryPrefix = filepath.ToSlash(repositoryPrefix)
	}

	repositoryPaths := make([]string, 0, len(sourcePaths))
	requested := make(map[string]struct{}, len(sourcePaths))
	totalPathBytes := 0
	for _, sourcePath := range sourcePaths {
		if len(sourcePath) > maxEarlySourceCatalogPathBytes ||
			sourcePath == "" || sourcePath == "." ||
			!fs.ValidPath(sourcePath) || path.Clean(sourcePath) != sourcePath ||
			strings.ContainsRune(sourcePath, '\\') {
			return false
		}
		for _, character := range sourcePath {
			if unicode.IsControl(character) {
				return false
			}
		}
		repositoryPath := sourcePath
		if repositoryPrefix != "" {
			repositoryPath = path.Join(repositoryPrefix, sourcePath)
		}
		if _, duplicate := requested[repositoryPath]; duplicate {
			return false
		}
		requested[repositoryPath] = struct{}{}
		repositoryPaths = append(repositoryPaths, repositoryPath)
		totalPathBytes += len(repositoryPath) + len(":(literal)")
		if totalPathBytes > maxEarlySourceCatalogTotalPathBytes {
			return false
		}
	}

	dirtyKinds := make(map[string]freshness.FileKind, len(repositoryPaths))
	for _, dirty := range state.Dirty {
		if _, included := requested[dirty.Path]; included {
			dirtyKinds[dirty.Path] = dirty.Kind
		}
		if dirty.FromPath != "" {
			if _, included := requested[dirty.FromPath]; included {
				dirtyKinds[dirty.FromPath] = freshness.FileMissing
			}
		}
	}
	cleanPaths := make([]string, 0, len(repositoryPaths))
	for _, repositoryPath := range repositoryPaths {
		if kind, dirty := dirtyKinds[repositoryPath]; dirty {
			if kind != freshness.FileRegular {
				return false
			}
			continue
		}
		cleanPaths = append(cleanPaths, repositoryPath)
	}
	return committedSourceCatalogPathsAreRegular(ctx, state, cleanPaths, totalPathBytes)
}

func committedSourceCatalogPathsAreRegular(
	ctx context.Context,
	state freshness.RepositoryState,
	repositoryPaths []string,
	totalPathBytes int,
) bool {
	if len(repositoryPaths) == 0 {
		return true
	}
	args := []string{
		"--no-pager",
		"-c", "core.fsmonitor=false",
		"-c", "core.hooksPath=" + os.DevNull,
		"-C", state.Identity,
		"ls-tree", "-z", state.Head, "--",
	}
	expected := make(map[string]struct{}, len(repositoryPaths))
	for _, repositoryPath := range repositoryPaths {
		args = append(args, ":(literal)"+repositoryPath)
		expected[repositoryPath] = struct{}{}
	}
	command := exec.CommandContext(ctx, "git", args...)
	command.Env = sourceCatalogGitEnvironment(os.Environ())
	stdout, err := command.StdoutPipe()
	if err != nil || command.Start() != nil {
		return false
	}
	outputLimit := int64(totalPathBytes + len(repositoryPaths)*128 + 1)
	output, readErr := io.ReadAll(io.LimitReader(stdout, outputLimit))
	_, drainErr := io.Copy(io.Discard, stdout)
	waitErr := command.Wait()
	if readErr != nil || drainErr != nil || waitErr != nil || int64(len(output)) >= outputLimit {
		return false
	}
	for len(output) > 0 {
		end := bytes.IndexByte(output, 0)
		if end < 0 {
			return false
		}
		record := output[:end]
		output = output[end+1:]
		header, rawPath, found := bytes.Cut(record, []byte{'\t'})
		fields := bytes.Fields(header)
		repositoryPath := string(rawPath)
		if !found || len(fields) != 3 || !bytes.HasPrefix(fields[0], []byte("100")) {
			return false
		}
		if _, included := expected[repositoryPath]; !included {
			return false
		}
		delete(expected, repositoryPath)
	}
	return len(expected) == 0
}

func sourceCatalogGitEnvironment(environment []string) []string {
	result := make([]string, 0, len(environment)+6)
	for _, entry := range environment {
		name, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(name, "GIT_") || name == "PAGER" {
			continue
		}
		result = append(result, entry)
	}
	return append(
		result,
		"GIT_OPTIONAL_LOCKS=0",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_SYSTEM="+os.DevNull,
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_PAGER=cat",
		"PAGER=cat",
	)
}

func researchRepositoryContext(state freshness.RepositoryState, repo string) modelresearch.RepositoryContext {
	identity := state.Identity
	if identity == "" {
		identity, _ = filepath.Abs(repo)
	}
	revision := state.Head
	if revision == "" {
		revision = "unknown"
	}
	return modelresearch.RepositoryContext{
		Identity: identity, Revision: revision, Scenario: "go-default",
	}
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

func reportOverviewURL(location string) string {
	base, _, _ := strings.Cut(location, "#")
	return base + "#/overview"
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

func runGuidedTour(runDir string, stderr io.Writer) error {
	absDir, err := filepath.Abs(runDir)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	outcome, err := editGuidedTourForRun(ctx, absDir, stderr)
	if err != nil {
		return err
	}
	if err := report.Generate(absDir); err != nil {
		return err
	}
	if outcome.Cached {
		fmt.Fprintf(stderr, "repomap: reused cached guided tour (%d response bytes)\n", outcome.ResponseBytes)
	} else {
		fmt.Fprintf(stderr, "repomap: guided tour accepted in %d ms\n", outcome.LatencyMillis)
	}
	fmt.Printf("Report: %s/report.html\n", absDir)
	return nil
}

func printUsage() {
	fmt.Fprintf(os.Stderr, "Usage: repomap [repo] [flags]\n")
	fmt.Fprintf(os.Stderr, "       repomap investigate <repo> --task-file <task.md> [flags]\n")
	fmt.Fprintf(os.Stderr, "       repomap doctor llm [--check]\n")
	fmt.Fprintf(os.Stderr, "       repomap serve [--run RUN_ID] [--source-episode PATH] [--port PORT]\n")
	fmt.Fprintf(os.Stderr, "       repomap orient --repo <repo> [flags]\n")
	fmt.Fprintf(os.Stderr, "\nFlags:\n")
	fmt.Fprintf(os.Stderr, "  --json          output JSON instead of text\n")
	fmt.Fprintf(os.Stderr, "  --offline       skip model calls, local facts only\n")
	fmt.Fprintf(os.Stderr, "  --flows N       expand top N directions after orientation (default 0)\n")
	fmt.Fprintf(os.Stderr, "  --discover-surfaces discover bounded Go runtime surfaces (default true)\n")
	fmt.Fprintf(os.Stderr, "  --guided-tour   add an optional guided tour to the architecture map (default true)\n")
	fmt.Fprintf(os.Stderr, "  --no-search     omit Super Search from the generated report\n")
	fmt.Fprintf(os.Stderr, "  --no-debug      disable debug artifact writing\n")
	fmt.Fprintf(os.Stderr, "  --no-open       do not open the generated HTML report\n")
	fmt.Fprintf(os.Stderr, "  --no-serve      generate a static report without starting the local server\n")
	fmt.Fprintf(os.Stderr, "  --port PORT     local report server port (default random)\n")
	fmt.Fprintf(os.Stderr, "  --debug-dir DIR debug artifact directory (default user cache)\n")
	fmt.Fprintf(os.Stderr, "  --dump-llm      dump LLM request/response in debug dir\n")
	fmt.Fprintf(os.Stderr, "  --preview-request print exact redacted request without an API call\n")
	fmt.Fprintf(os.Stderr, "  --strict-snapshot fail if captured analyzed inputs change during the run\n")
	fmt.Fprintf(os.Stderr, "  --source-episode PATH render one approved bounded source episode over the generated report\n")
	fmt.Fprintf(os.Stderr, "  --help, -h      show this help\n")
	fmt.Fprintf(os.Stderr, "  --version       show version\n")
	fmt.Fprintf(os.Stderr, "\nEnvironment:\n")
	fmt.Fprintf(os.Stderr, "  REPOMAP_LLM_ENDPOINT full OpenAI-compatible chat/completions URL\n")
	fmt.Fprintf(os.Stderr, "  REPOMAP_LLM_MODEL\n")
	fmt.Fprintf(os.Stderr, "  REPOMAP_LLM_API_KEY (for bearer auth)\n")
	fmt.Fprintf(os.Stderr, "  REPOMAP_LLM_AUTH    bearer (default) or none\n")
	fmt.Fprintf(os.Stderr, "  REPOMAP_LLM_TIMEOUT (default 10m)\n")
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
