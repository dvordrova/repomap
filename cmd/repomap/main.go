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
	"github.com/dvordrova/repomap/internal/atlasstudy"
	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/guidedtour"
	"github.com/dvordrova/repomap/internal/localization"
	"github.com/dvordrova/repomap/internal/modelresearch"
	"github.com/dvordrova/repomap/internal/orient"
	"github.com/dvordrova/repomap/internal/pavedpath"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/reportserver"
	"github.com/dvordrova/repomap/internal/secretscan"
	"github.com/dvordrova/repomap/internal/semanticdiscovery"
	"github.com/dvordrova/repomap/internal/snapshot"
	"github.com/dvordrova/repomap/internal/surfacediscovery"
	"github.com/dvordrova/repomap/internal/tasklens"
	"github.com/dvordrova/repomap/internal/themestudy"
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
	if len(os.Args) >= 2 && !strings.HasPrefix(os.Args[1], "-") && os.Args[1] != "investigate" && os.Args[1] != "orient" && os.Args[1] != "doctor" && os.Args[1] != "serve" && os.Args[1] != "cache" && os.Args[1] != "dev" {
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
	case "cache":
		if err := runCache(os.Args[2:], os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "dev":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Usage: repomap dev render-report <run-dir> [--repo <repo>] | theme-study-response-replay --run-dir <copied-run> --request-sha256 <sha> --response <file> --response-sha256 <sha> [--stage scout|adjudication] | theme-study-scout-request-rebuild --run-dir <copied-run> [--repo <repo>] | theme-study-response-mock --run-dir <copied-run> [--stage scout|adjudication] [--out <file>] | localization-check <run-dir> | localization-replay <run-dir> <projection.json> | localization-stage <run-dir> [<projection.json>] | localization-record <run-dir> [<projection.json>] | v32-refresh --run-dir <copied-run-dir> --repo <repo> [--reuse-study | --operate-only | --replay-saved] | fresh-repo-onboarding --run-dir <run-dir> [--repo <repo> [--replan-saved] | --replay-saved] | guided-tour <run-dir> | guided-tour-fanout <run-dir> | guided-tour-experiment <run-dir> | semantic-discovery <run-dir> | semantic-discovery-experiment <run-dir> | golden-mechanism <run-dir> [--probe-only] | golden-mechanism-v01 <run-dir> [--replay-old] | golden-mechanism-v02 <run-dir> [--prepare | --replay] | golden-mechanism-v03 <run-dir> [--replay] | golden-mechanism-v1 <run-dir> [--prepare | --replay] | chi-request-dispatch <run-dir> [--prepare | --replay-response | --replay] | mechanism-v1 <run-dir> [--replay] | mechanism-study-experiment --repo <repo> --root-path <path> --root-line <line> --root-symbol <exact-symbol> --label <label> --question <question> --out <directory> [--request-only] | review-cockpit --caddy-run <run-dir> --chi-run <run-dir> --out <output-dir> | prompt-versions | corpus <root> [--matrix <json>]")
			os.Exit(2)
		}
		switch os.Args[2] {
		case "theme-study-response-replay":
			if err := runThemeStudyResponseReplayCLI(os.Args[3:], os.Stdout); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		case "theme-study-scout-request-rebuild":
			if err := runThemeStudyScoutRequestRebuildCLI(os.Args[3:], os.Stdout); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		case "theme-study-response-mock":
			if err := runThemeStudyResponseMockCLI(os.Args[3:], os.Stdout); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		case "v32-refresh":
			if err := runV32RefreshCLI(os.Args[3:], os.Stdout, os.Stderr); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		case "render-report":
			if err := runRenderReportCLI(os.Args[3:], os.Stdout); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		case "ui":
			if err := runDevUICLI(os.Args[3:], os.Stdout); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		case "localization-check":
			if err := runLocalizationCheckCLI(os.Args[3:], os.Stdout); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		case "localization-replay":
			if err := runLocalizationReplayCLI(os.Args[3:], os.Stdout); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		case "localization-stage":
			if err := runLocalizationStageCLI(os.Args[3:], os.Stdout); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		case "localization-record":
			if err := runLocalizationRecordCLI(os.Args[3:], os.Stdout); err != nil {
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
		case "corpus":
			if err := runCorpusCLI(os.Args[3:], os.Stdout); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
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
		case "mechanism-study-experiment":
			if err := runMechanismStudyExperimentCLI(os.Args[3:], os.Stdout, os.Stderr); err != nil {
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
	output := newRunOutput(writer)
	if errors.Is(err, context.Canceled) {
		output.State("Run", "canceled")
		return
	}
	output.Error("run failed", err.Error())
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
		"{\"orientation_json\":%q,\"source_json\":%q,\"symbol_json\":%q,\"symbol_tagged\":%q,\"localization\":%q,\"localization_request\":%q,\"guided_tour\":%q,\"guided_tour_leaf\":%q,\"guided_tour_fan_in\":%q,\"semantic_opportunity\":%q,\"semantic_leaf\":%q,\"semantic_fan_in\":%q,\"semantic_monolithic\":%q,\"golden_mechanism\":%q,\"repository_onboarding_editor\":%q,\"repository_brief_shape\":%q,\"study_direction_candidates\":%q,\"reading_pack_review\":%q,\"paved_paths\":%q,\"task_investigation\":%q}\n",
		deepseek.OrientationPromptVersionJSON,
		deepseek.SourcePromptVersionJSON,
		deepseek.SymbolPromptVersionJSON,
		deepseek.SymbolPromptVersionTagged,
		localization.PromptVersion,
		deepseek.LocalizationRequestVersion,
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
				"repomap: reused cached orientation response of %d bytes (original call: %d ms, %s); %d candidate direction(s) accepted, %d rejected\n",
				event.ResponseBytes,
				event.LatencyMillis,
				formatTokenUsage(event.InputTokens, event.OutputTokens),
				event.CandidateCount,
				event.RejectedCount,
			)
			break
		}
		fmt.Fprintf(
			writer,
			"repomap: orientation received %d bytes in %d ms (%s); %d candidate direction(s) accepted, %d rejected\n",
			event.ResponseBytes,
			event.LatencyMillis,
			formatTokenUsage(event.InputTokens, event.OutputTokens),
			event.CandidateCount,
			event.RejectedCount,
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
		ctx:                         ctx,
		stdout:                      os.Stdout,
		stderr:                      os.Stderr,
		openReport:                  openReport,
		serveReport:                 reportserver.Serve,
		captureRepo:                 freshness.CaptureRepository,
		newStudyInvestigationClient: defaultStudyInvestigationClientFactory,
	})
}

type defaultRunDeps struct {
	ctx                         context.Context
	stdout                      io.Writer
	stderr                      io.Writer
	openReport                  func(string) error
	serveReport                 func(context.Context, reportserver.Options) error
	captureRepo                 func(context.Context, string) (freshness.RepositoryState, error)
	newStudyInvestigationClient studyInvestigationClientFactory
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

func runDefaultWithDeps(repo string, extraArgs []string, deps defaultRunDeps) (runErr error) {
	fs := flag.NewFlagSet("repomap", flag.ContinueOnError)
	fs.SetOutput(deps.stderr)

	offline := fs.Bool("offline", false, "skip model calls, build local facts/bundles only")
	discoverSurfaces := fs.Bool("discover-surfaces", true, "discover bounded Go runtime surfaces for the report")
	noCache := fs.Bool("no-cache", false, "disable cross-run model response caches")
	noSecrets := fs.Bool("no-secrets", false, "disable credential detection for this run (unsafe)")
	language := fs.String("lang", "en", "report language: en or ru")
	gitLabURLFlag := fs.String("gitlab-url", "", "create a standalone report linked to this GitLab project or host")
	gitHubURLFlag := fs.String("github-url", "", "create a standalone report linked to this GitHub repository or host")
	noOpen := fs.Bool("no-open", false, "do not open the generated HTML report")
	noServe := fs.Bool("no-serve", false, "generate a static report without starting the local server")
	port := fs.Int("port", 0, "local report server port (default: random)")
	debugDir := fs.String("debug-dir", defaultDebugDir(), "directory for debug artifacts")
	strictSnapshot := fs.Bool("strict-snapshot", false, "fail when captured analyzed inputs change before report publication")
	sourceEpisodePath := fs.String("source-episode", "", "render an approved bounded source episode over the generated report")

	if err := fs.Parse(extraArgs); err != nil {
		return err
	}
	humanOutput := newRunOutput(deps.stderr)
	newStudyInvestigationClient := deps.newStudyInvestigationClient
	if newStudyInvestigationClient == nil {
		newStudyInvestigationClient = defaultStudyInvestigationClientFactory
	}
	publicationStateEmitted := false
	defer func() {
		if runErr != nil && !publicationStateEmitted {
			humanOutput.State(
				"Run", "failed",
				"report publication did not complete",
			)
		}
	}()
	if fs.NArg() > 0 {
		if repo != "." || fs.NArg() != 1 {
			return fmt.Errorf("unexpected positional arguments: %s", strings.Join(fs.Args(), " "))
		}
		repo = fs.Arg(0)
	}
	restoreSecretScan := secretscan.SetDisabled(*noSecrets)
	defer restoreSecretScan()
	if *noSecrets {
		humanOutput.Warn(
			"ordinary input credential detection is disabled",
			"mandatory provider-response and persisted-artifact scans remain active",
			"selected tracked source may reach the model provider and debug artifacts",
		)
	}
	if *port < 0 || *port > 65535 {
		return fmt.Errorf("--port must be between 0 and 65535")
	}
	reportLanguage, err := normalizeReportLanguage(*language)
	if err != nil {
		return err
	}
	gitLabURL := strings.TrimSpace(*gitLabURLFlag)
	gitHubURL := strings.TrimSpace(*gitHubURLFlag)
	if gitLabURL != "" && gitHubURL != "" {
		return fmt.Errorf("--gitlab-url and --github-url cannot be combined")
	}
	staticSourceHost := ""
	if gitLabURL != "" {
		staticSourceHost = "GitLab"
	}
	if gitHubURL != "" {
		staticSourceHost = "GitHub"
	}
	if staticSourceHost != "" {
		*noServe = true
		if *sourceEpisodePath != "" {
			return fmt.Errorf("standalone %s reports cannot be combined with --source-episode because they do not embed source", staticSourceHost)
		}
	}
	if *noCache && !*offline {
		humanOutput.State("Cache", "disabled", "cross-run model response reuse: off")
	}
	absRepo, err := filepath.Abs(repo)
	if err != nil {
		return fmt.Errorf("resolve repository path: %w", err)
	}
	repo = absRepo
	if gitLabURL != "" {
		gitLabURL, err = report.ResolveGitLabRepositoryURL(
			gitLabURL,
			snapshot.RepositoryOriginIdentity(repo),
		)
		if err != nil {
			return err
		}
	}
	if gitHubURL != "" {
		gitHubURL, err = report.ResolveGitHubRepositoryURL(
			gitHubURL,
			snapshot.RepositoryOriginIdentity(repo),
		)
		if err != nil {
			return err
		}
	}

	dDir := *debugDir
	if dDir == "" {
		return fmt.Errorf("Atlas-first product runs require a nonempty --debug-dir for report authority")
	}

	runID := debugdump.GenerateRunID(repoRunLabel(repo))
	var sourceEpisodeJSON []byte
	if *sourceEpisodePath != "" {
		sourceEpisodeJSON, err = readSourceEpisodeFile(*sourceEpisodePath)
		if err != nil {
			return err
		}
	}

	ctx := deps.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	captureRepo := deps.captureRepo
	if captureRepo == nil {
		captureRepo = freshness.CaptureRepository
	}
	runDir := filepath.Join(dDir, runID)
	humanOutput.Artifacts(runDir)
	if err := report.RemoveRunManifest(runDir); err != nil {
		return fmt.Errorf("invalidate previous browser report authority: %w", err)
	}
	analysisRoot, err := resolveAnalysisRoot(repo)
	if err != nil {
		return err
	}
	initialState, err := captureRepo(ctx, repo)
	if err != nil {
		return fmt.Errorf("capture repository state before orientation: %w", err)
	}
	if staticSourceHost != "" && repositoryStateHasAnalyzedSubmodule(initialState) {
		return fmt.Errorf("standalone %s reports do not support analyzed submodule source because one repository URL cannot address it", staticSourceHost)
	}
	if sourceEpisodeJSON != nil {
		if err := report.ValidateSourceEpisodeForRevision(sourceEpisodeJSON, initialState.Head); err != nil {
			return fmt.Errorf("validate source episode before orientation: %w", err)
		}
	}

	researchPolicy := modelresearch.DefaultPolicy()
	var directCallIndex *surfacediscovery.DirectCallIndex
	opts := orient.Options{
		RepoPath:                  repo,
		AtlasFirst:                true,
		Offline:                   *offline,
		NoCache:                   *noCache,
		RunID:                     runID,
		DebugDir:                  dDir,
		DumpRedacted:              true,
		RequireArtifacts:          true,
		DiscoverSurfaces:          *discoverSurfaces,
		MaxLLMFiles:               0,
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
		MaxGoPkgs:                 0,
		MaxGoEdges:                0,
		ResearchPolicy:            researchPolicy,
		RepositoryContext:         researchRepositoryContext(initialState, repo),
		DirectCallIndexSink: func(index surfacediscovery.DirectCallIndex) {
			directCallIndex = &index
		},
		EffectiveOptions: debugdump.EffectiveOptions{
			Offline:          *offline,
			NoCache:          *noCache,
			DiscoverSurfaces: *discoverSurfaces,
			NoSecrets:        *noSecrets,
			ReportLanguage:   storedReportLanguage(reportLanguage),
			GitLabURL:        gitLabURL,
			GitHubURL:        gitHubURL,
			NoOpen:           *noOpen,
			NoServe:          *noServe,
			Port:             *port,
			DebugEnabled:     dDir != "",
		},
	}
	opts.Progress = humanOutput.Progress

	_, err = orient.Run(ctx, opts)
	if err != nil {
		return fmt.Errorf("%w\nrequest diagnostics: %s", err, filepath.Join(runDir, "metadata.json"))
	}
	var reportPath string
	var architectureAuthority report.RunAuthority
	if !*offline {
		architectureAuthorityStarted := time.Now()
		humanOutput.Stage("Repository authority", "confirming Architecture inputs")
		architectureReportData, readErr := report.ReadRunDir(runDir)
		if readErr != nil {
			return fmt.Errorf("read captured Architecture inputs: %w", readErr)
		}
		architectureState, captureErr := captureRepo(ctx, repo)
		if captureErr != nil {
			return fmt.Errorf("capture repository state before Architecture: %w", captureErr)
		}
		if staticSourceHost != "" && architectureState.Head != initialState.Head {
			return fmt.Errorf(
				"standalone %s reports require HEAD to remain at the captured commit until report publication",
				staticSourceHost,
			)
		}
		architectureAuthority, err = report.ConfirmRunAuthorityScoped(
			ctx,
			analysisRoot,
			initialState,
			architectureState,
			report.CapturedInputPaths(architectureReportData),
			*strictSnapshot,
		)
		if err != nil {
			return fmt.Errorf("confirm Architecture input authority: %w", err)
		}
		humanOutput.State(
			"Repository authority", "Architecture inputs confirmed",
			fmt.Sprintf(
				"captured inputs: %d",
				len(report.CapturedInputPaths(architectureReportData)),
			),
			formatRunOutputDuration(time.Since(architectureAuthorityStarted).Milliseconds()),
		)
	}
	var architectureOutcome architectureSynthesisOutcome
	var architectureErr error
	if *offline {
		if err := persistArchitectureSynthesisUnavailable(runDir); err != nil {
			return fmt.Errorf("persist offline Architecture state: %w", err)
		}
		humanOutput.State("Architecture", "unavailable", "provider calls: 0", "reason: offline requested")
	} else {
		architectureOutcome, architectureErr = synthesizeArchitectureForRun(
			ctx, runDir, architectureAuthority, humanOutput, *noCache, reportLanguage,
		)
	}
	if diagnosticErr := recordAtlasFirstStageDiagnostic(
		runDir, architectureAtlasFirstDiagnostic(architectureOutcome, architectureErr, *offline),
	); diagnosticErr != nil {
		if architectureErr != nil {
			return errors.Join(architectureErr, diagnosticErr)
		}
		return diagnosticErr
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	if architectureErr != nil {
		if !isPublishableArchitectureFailure(architectureErr) {
			return architectureErr
		}
		if report.IsExactWorkspaceGraphUnavailable(architectureErr) {
			humanOutput.State(
				"Architecture", "unavailable", "provider calls: 0",
				"reason: exact local package graph unavailable",
			)
		} else if architectureOutcome.Failure == nil && !errors.Is(architectureErr, errArchitectureSynthesisRejected) {
			if isArchitectureOutputResourceExhausted(architectureErr) {
				humanOutput.Warn(
					"Architecture grouping unavailable",
					"reason: the model exceeded its response budget",
					"the partial response was not used",
					"the exact local Architecture Canvas remains available",
				)
			} else {
				humanOutput.Warn(
					"Architecture model stage failed",
					"state: failed",
					"the exact local Architecture Canvas remains available",
				)
			}
		}
	}
	reconciliationStarted := time.Now()
	humanOutput.Stage("Repository authority", "reconciling captured inputs")
	var reportData *report.ReportData
	if *offline {
		reportData, err = report.ReadRunDir(runDir)
	} else {
		reportData, err = report.ReadRunDirForAuthorizedArchitecture(runDir, architectureAuthority)
	}
	if err != nil && !(report.IsExactWorkspaceGraphUnavailable(err) && reportData != nil &&
		report.IsExactWorkspaceGraphUnavailable(architectureErr)) {
		return fmt.Errorf("read captured report inputs: %w", err)
	}
	currentState, err := captureRepo(ctx, repo)
	if err != nil {
		return fmt.Errorf("capture repository state after analysis: %w", err)
	}
	if staticSourceHost != "" && currentState.Head != initialState.Head {
		return fmt.Errorf("standalone %s reports require HEAD to remain at the captured commit until report publication", staticSourceHost)
	}
	authority, err := report.ConfirmRunAuthorityScoped(
		ctx, analysisRoot, initialState, currentState, report.CapturedInputPaths(reportData), *strictSnapshot,
	)
	if err != nil {
		return fmt.Errorf("confirm browser report authority: %w", err)
	}
	humanOutput.State(
		"Repository authority", "confirmed",
		fmt.Sprintf("captured inputs: %d", len(report.CapturedInputPaths(reportData))),
		formatRunOutputDuration(time.Since(reconciliationStarted).Milliseconds()),
	)
	if authority.Freshness().State != freshness.FreshnessFresh {
		humanOutput.Warn("repository snapshot is not fresh", "state: "+string(authority.Freshness().State))
	}
	if err := report.PrepareAuthorizedSourceCoverage(ctx, reportData, &authority); err != nil {
		return fmt.Errorf("prepare exact Atlas Study source coverage: %w", err)
	}
	// These deterministic scope/connectivity facts are known before Study.
	// Emit them here so a pre-call Study failure cannot hide already-observed
	// Architecture omissions or Map connectivity loss from the ordinary console.
	humanOutput.ArchitectureScope(
		report.DescribeArchitectureProductScope(reportData.RepositoryGraph),
	)
	humanOutput.MapConnectivity(
		report.DescribeArchitectureStructuralConnectivity(reportData.ArchitectureCanvas),
	)

	var studyOutcome themeStudyRunOutcome
	var studyErr error
	studyCalled := true
	switch {
	case *offline:
		if cleanupErr := resetThemeStudyArtifacts(runDir); cleanupErr != nil {
			studyErr = cleanupErr
		} else {
			studyOutcome = themeStudyRunOutcome{
				State: atlasstudy.ProductStateUnavailable, ProviderSkipped: true,
			}
			humanOutput.State("Study", "unavailable", "provider calls: 0", "reason: offline requested")
		}
	default:
		studyOutcome, studyErr = runThemeStudyForRun(
			ctx, reportData, runDir, dDir, analysisRoot,
			researchRepositoryContext(initialState, repo), researchPolicy,
			*noCache, true, themestudy.Language(reportLanguage), humanOutput,
		)
	}
	if ctxErr := ctx.Err(); ctxErr != nil && studyErr == nil {
		if cleanupErr := resetThemeStudyArtifacts(runDir); cleanupErr != nil {
			studyErr = errors.Join(ctxErr, cleanupErr)
		} else {
			studyErr = ctxErr
		}
	}
	if diagnosticErr := recordAtlasFirstStageDiagnostic(
		runDir, atlasStudyAtlasFirstDiagnostic(studyOutcome, studyErr, studyCalled),
	); diagnosticErr != nil {
		if studyErr != nil {
			return errors.Join(studyErr, diagnosticErr)
		}
		return diagnosticErr
	}
	if studyErr != nil {
		return studyErr
	}

	var investigationOutcome studyInvestigationRunOutcome
	var investigationErr error
	investigationCalled := studyOutcome.PublishedCards > 0
	if investigationCalled {
		repositoryFreshnessSHA256, digestErr := initialState.Digest()
		if digestErr != nil {
			investigationErr = fmt.Errorf("bind Study investigation repository state: %w", digestErr)
		} else {
			investigationOutcome, investigationErr = runStudyInvestigationForRun(
				ctx,
				runDir,
				directCallIndex,
				initialState.Head,
				repositoryFreshnessSHA256,
				humanOutput,
				newStudyInvestigationClient,
			)
		}
	}
	if diagnosticErr := recordAtlasFirstStageDiagnostic(
		runDir,
		studyInvestigationAtlasFirstDiagnostic(investigationOutcome, investigationCalled),
	); diagnosticErr != nil {
		if investigationErr != nil {
			return errors.Join(investigationErr, diagnosticErr)
		}
		return diagnosticErr
	}
	if investigationErr != nil {
		return investigationErr
	}

	reconciliationContext, releaseReconciliation := studyInvestigationPublicationContext(
		ctx,
		investigationOutcome.Status,
	)
	defer releaseReconciliation()
	if studyOutcome.SemanticCalls > 0 || investigationOutcome.SemanticCalls > 0 || investigationCalled {
		studyReconciliationStarted := time.Now()
		postStudyState, captureErr := captureRepo(reconciliationContext, repo)
		if captureErr != nil {
			return fmt.Errorf("capture repository state after Atlas Study: %w", captureErr)
		}
		if investigationCalled {
			if err := validateStudyInvestigationRepositoryFreshness(initialState, postStudyState); err != nil {
				return err
			}
		}
		if staticSourceHost != "" && postStudyState.Head != initialState.Head {
			return fmt.Errorf(
				"standalone %s reports require HEAD to remain at the captured commit until report publication",
				staticSourceHost,
			)
		}
		authority, err = report.ConfirmRunAuthorityScoped(
			reconciliationContext, analysisRoot, initialState, postStudyState,
			report.CapturedInputPaths(reportData), *strictSnapshot,
		)
		if err != nil {
			return fmt.Errorf("confirm browser report authority after Atlas Study: %w", err)
		}
		if err := report.PrepareAuthorizedSourceCoverage(reconciliationContext, reportData, &authority); err != nil {
			return fmt.Errorf("reprepare exact source coverage after Atlas Study: %w", err)
		}
		if investigationOutcome.Status.MechanismCount > 0 {
			if err := report.PrepareAuthorizedStudyInvestigationSourceCoverage(
				reconciliationContext,
				reportData,
				&authority,
				investigationOutcome.ReportInput,
			); err != nil {
				return fmt.Errorf("prepare exact Study investigation source coverage: %w", err)
			}
		}
		humanOutput.State(
			"Repository authority", "reconfirmed after Study",
			fmt.Sprintf("captured inputs: %d", len(report.CapturedInputPaths(reportData))),
			formatRunOutputDuration(time.Since(studyReconciliationStarted).Milliseconds()),
		)
	}
	if err := finalizeAtlasFirstStageDiagnostics(runDir); err != nil {
		return err
	}
	generateAuthorizedReport := func() error {
		if sourceEpisodeJSON != nil {
			return report.GenerateAuthorizedWithSourceEpisode(
				runDir,
				authority,
				sourceEpisodeJSON,
			)
		}
		if gitLabURL != "" {
			return report.GenerateAuthorizedGitLab(runDir, authority, gitLabURL)
		}
		if gitHubURL != "" {
			return report.GenerateAuthorizedGitHub(runDir, authority, gitHubURL)
		}
		return report.GenerateAuthorized(runDir, authority)
	}
	reportStarted := time.Now()
	if reportLanguage == "ru" && !opts.AtlasFirst {
		localizationData, preparedLocalization, preparationErr :=
			preparePresentationLocalizationForRun(
				runDir,
				reportData,
				sourceEpisodeJSON,
			)
		if *offline {
			var prepared *report.PreparedPresentationLocalization
			if preparationErr == nil {
				prepared = &preparedLocalization
			}
			if err := markPresentationLocalizationUnavailable(
				runDir,
				report.LocalizationFailureOfflineRequested,
				prepared,
			); err != nil {
				fmt.Fprintln(
					deps.stderr,
					"warning: Russian localization status could not be saved; Russian product UI will retain canonical English model prose",
				)
			}
			fmt.Fprintln(
				deps.stderr,
				"warning: Russian localization was requested in offline mode; Russian product UI will show canonical English model prose",
			)
		} else {
			fmt.Fprintln(
				deps.stderr,
				"repomap: translating the complete bounded presentation inventory from canonical English to Russian",
			)
			localizationStarted := time.Now()
			var localizationOutcome presentationLocalizationOutcome
			var localizationErr error
			if preparationErr != nil {
				localizationOutcome, localizationErr =
					recordPresentationLocalizationPreparationFailure(
						runDir,
						preparationErr,
					)
			} else {
				localizationOutcome, localizationErr =
					localizePreparedPresentationForRun(
						ctx,
						runDir,
						filepath.Join(dDir, presentationLocalizationCacheDir),
						*noCache,
						deps.stderr,
						localizationData,
						preparedLocalization,
					)
			}
			if localizationErr != nil {
				if isSemanticResourceLimit(localizationErr) {
					return localizationErr
				}
				fmt.Fprintln(
					deps.stderr,
					"warning: Russian localization status could not be saved; Russian product UI will retain canonical English model prose",
				)
			} else {
				for _, batch := range localizationOutcome.Batches {
					cacheState := "miss"
					if batch.CacheHit {
						cacheState = "hit"
					}
					fmt.Fprintf(
						deps.stderr,
						"repomap: localization batch %d/%d: fields=%d predicted_output=%d bytes cache=%s request=%d response=%d tokens=%d/%d semantic_calls=%d transport_attempts=%d unrequested=%d\n",
						batch.Index+1,
						batch.Count,
						batch.FieldCount,
						batch.PredictedOutputBytes,
						cacheState,
						batch.RequestBytes,
						batch.ResponseBytes,
						batch.InputTokens,
						batch.OutputTokens,
						batch.ProviderCalls,
						batch.Attempts,
						batch.UnrequestedTranslations,
					)
				}
				if localizationOutcome.CacheCorrupt {
					fmt.Fprintln(
						deps.stderr,
						"warning: ignored an invalid localization cache entry and recomputed the projection",
					)
				}
				if localizationOutcome.CacheWriteErr {
					fmt.Fprintln(
						deps.stderr,
						"warning: Russian localization cache entry could not be saved; the valid per-run projection remains available",
					)
				}
				switch localizationOutcome.State {
				case report.PresentationLocalizationSucceeded:
					writePresentationLocalizationUnrequestedWarning(deps.stderr, localizationOutcome)
					if localizationOutcome.CacheHit {
						fmt.Fprintf(
							deps.stderr,
							"repomap: reused cached Russian presentation translation in %d ms\n",
							time.Since(localizationStarted).Milliseconds(),
						)
					} else {
						fmt.Fprintf(
							deps.stderr,
							"repomap: Russian presentation translation received %d bytes from %d request bytes in %d ms (%s; %d semantic call(s), %d transport attempt(s))\n",
							localizationOutcome.ResponseBytes,
							localizationOutcome.RequestBytes,
							time.Since(localizationStarted).Milliseconds(),
							formatTokenUsage(
								localizationOutcome.InputTokens,
								localizationOutcome.OutputTokens,
							),
							localizationOutcome.ProviderCalls,
							localizationOutcome.Attempts,
						)
					}
				default:
					writePresentationLocalizationFailureWarning(
						deps.stderr,
						localizationOutcome,
						time.Since(localizationStarted).Milliseconds(),
					)
				}
			}
		}
		if !*offline {
			// Translation can be a long external call. Reconcile the
			// captured inputs again before the final render so authority
			// confirmed before translation is never silently reused after
			// the repository has changed.
			localizationReconciliationStarted := time.Now()
			postLocalizationState, captureErr := captureRepo(ctx, repo)
			if captureErr != nil {
				return fmt.Errorf(
					"capture repository state after presentation localization: %w",
					captureErr,
				)
			}
			if staticSourceHost != "" &&
				postLocalizationState.Head != initialState.Head {
				return fmt.Errorf(
					"standalone %s reports require HEAD to remain at the captured commit until report publication",
					staticSourceHost,
				)
			}
			authority, err = report.ConfirmRunAuthorityScoped(
				ctx,
				analysisRoot,
				initialState,
				postLocalizationState,
				report.CapturedInputPaths(reportData),
				*strictSnapshot,
			)
			if err != nil {
				return fmt.Errorf(
					"confirm browser report authority after presentation localization: %w",
					err,
				)
			}
			fmt.Fprintf(
				deps.stderr,
				"repomap: reconciled %d captured input(s) after presentation localization in %d ms\n",
				len(report.CapturedInputPaths(reportData)),
				time.Since(localizationReconciliationStarted).Milliseconds(),
			)
		}
	}
	humanOutput.Stage("Report", "generating authorized Atlas-first workspace")
	if err := generateAuthorizedReport(); err != nil {
		return fmt.Errorf("generate authorized browser report: %w", err)
	}
	humanOutput.State(
		"Report", "generated",
		formatRunOutputDuration(time.Since(reportStarted).Milliseconds()),
	)
	reportPath = filepath.Join(runDir, "report.html")
	humanOutput.Stage("Report", "path: "+reportPath)
	if staticSourceHost != "" {
		humanOutput.Stage(
			"Report",
			fmt.Sprintf("standalone host: %s", staticSourceHost),
			"captured revision: "+initialState.Head,
			"remote availability is not checked; ensure the captured commit is pushed before sharing",
		)
		if len(initialState.Dirty) != 0 {
			humanOutput.Warn(
				"report contains stable local changes",
				fmt.Sprintf("changed inputs: %d", len(initialState.Dirty)),
				"changed source paths are local-only",
			)
		}
	}
	publication, err := assessRunPublication(runDir)
	if err != nil {
		return fmt.Errorf("verify generated report publication: %w", err)
	}
	linkLatest(dDir, runDir, runOutputWarningSink{
		output: humanOutput, summary: "could not update latest report link",
	})
	publicationDetails := append([]string{"report: " + reportPath}, publication.consoleDetails()...)
	humanOutput.State("Run", publication.consoleState(), publicationDetails...)
	publicationStateEmitted = true
	if studyInvestigationCanceled(investigationOutcome.Status) && ctx.Err() != nil {
		// The optional stage consumed the cancellation into a durable failed/
		// partial status. The report is now safely published; do not start an
		// interactive server under the already-canceled caller context.
		return nil
	}

	interactiveReport := reportPath != ""
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
				humanOutput.Stage("Server", fmt.Sprintf(format, args...))
			},
			OnReady: func(url string) error {
				url = reportMapURL(url)
				humanOutput.State("Server", "ready", "url: "+url, "Ctrl-C to stop")
				if !*noOpen && deps.openReport != nil {
					if err := deps.openReport(url); err != nil {
						humanOutput.Warn("could not open report", err.Error())
					}
				}
				return nil
			},
		})
	}
	if interactiveReport && !*noOpen && deps.openReport != nil {
		if err := deps.openReport(reportPath); err != nil {
			humanOutput.Warn("could not open report", err.Error())
		}
	}
	return nil
}

func writeStudyMapCompletion(
	writer io.Writer,
	status studyMapStatus,
	elapsed time.Duration,
) {
	switch status.FailureReason {
	case string(studyMapNoSupportedSourceAdapter),
		string(studyMapNoEligibleSourceFunctions):
		fmt.Fprintf(
			writer,
			"repomap: decision study: published=0 provider_calls=0 reason=%s (after %d ms)\n",
			status.FailureReason,
			elapsed.Milliseconds(),
		)
		return
	}
	fmt.Fprintf(
		writer,
		"repomap: selected %d source-backed study direction(s) from %d proposed candidate(s) in %d ms (%s)\n",
		status.Selected,
		status.Candidates,
		elapsed.Milliseconds(),
		formatTokenUsage(status.Metrics.InputTokens, status.Metrics.OutputTokens),
	)
}

func repositoryStateHasAnalyzedSubmodule(state freshness.RepositoryState) bool {
	for _, submodule := range state.Submodules {
		if submodule.IncludedInAnalysis {
			return true
		}
	}
	return false
}

func writePresentationLocalizationFailureWarning(
	stderr io.Writer,
	outcome presentationLocalizationOutcome,
	elapsedMillis int64,
) {
	unsafeAttribution := ""
	if outcome.UnsafeKind != "" {
		unsafeAttribution = fmt.Sprintf(
			" unsafe_kind=%s translation_index=%d",
			outcome.UnsafeKind,
			outcome.TranslationIndex,
		)
	}
	fmt.Fprintf(
		stderr,
		"warning: Russian localization failed (%s stage=%s validation=%s batches=%d attempted=%d completed=%d failed_batch=%d%s); Russian product UI will show canonical English model prose (after %d ms)\n",
		outcome.ReasonCode,
		outcome.FailureStage,
		outcome.ValidationCode,
		outcome.BatchTotal,
		outcome.BatchAttempted,
		outcome.BatchCompleted,
		outcome.FailedBatch,
		unsafeAttribution,
		elapsedMillis,
	)
}

func writePresentationLocalizationUnrequestedWarning(
	stderr io.Writer,
	outcome presentationLocalizationOutcome,
) {
	if outcome.UnrequestedTranslations <= 0 {
		return
	}
	fmt.Fprintf(
		stderr,
		"warning: ignored %d unrequested trailing Russian localization translation(s); requested translations were validated and applied\n",
		outcome.UnrequestedTranslations,
	)
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
			url = reportMapURL(url)
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
	fmt.Fprintln(stdout, "max_tokens_override: REPOMAP_LLM_MAX_TOKENS")
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

func reportMapURL(location string) string {
	base, _, _ := strings.Cut(location, "#")
	return base + "#/map"
}

func runRenderReportCLI(args []string, stdout io.Writer) error {
	return runRenderReportCLIWith(args, stdout, renderReportDependencies{
		captureRepo:            freshness.CaptureRepository,
		readAuthoritySeed:      report.ReadRunManifestAuthoritySeed,
		confirmAuthorityScoped: report.ConfirmRunAuthorityScoped,
		generate:               report.Generate,
		generateAuthorized:     report.GenerateAuthorized,
		resolveAnalysisRoot:    resolveAnalysisRoot,
	})
}

type renderReportDependencies struct {
	captureRepo            func(context.Context, string) (freshness.RepositoryState, error)
	readAuthoritySeed      func(string) (report.RunManifestAuthoritySeed, error)
	confirmAuthorityScoped func(context.Context, string, freshness.RepositoryState, freshness.RepositoryState, []string, bool) (report.RunAuthority, error)
	generate               func(string) error
	generateAuthorized     func(string, report.RunAuthority) error
	resolveAnalysisRoot    func(string) (string, error)
}

func runRenderReportCLIWith(args []string, stdout io.Writer, deps renderReportDependencies) error {
	if len(args) != 1 && !(len(args) == 3 && args[1] == "--repo" && args[2] != "") {
		return fmt.Errorf("usage: repomap dev render-report <run-dir> [--repo <repo>]")
	}
	if stdout == nil {
		return fmt.Errorf("render report: stdout is required")
	}
	if deps.captureRepo == nil || deps.readAuthoritySeed == nil || deps.confirmAuthorityScoped == nil || deps.generate == nil ||
		deps.generateAuthorized == nil || deps.resolveAnalysisRoot == nil {
		return fmt.Errorf("render report: dependencies are not configured")
	}
	runDir := args[0]
	absDir, err := filepath.Abs(runDir)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}
	if len(args) == 1 {
		if err := deps.generate(absDir); err != nil {
			return err
		}
	} else {
		repo := args[2]
		seed, err := deps.readAuthoritySeed(absDir)
		if err != nil {
			return fmt.Errorf("render report: read authority seed: %w", err)
		}
		analysisRoot, err := deps.resolveAnalysisRoot(repo)
		if err != nil {
			return fmt.Errorf("render report: resolve repository authority: %w", err)
		}
		ctx := context.Background()
		before, err := deps.captureRepo(ctx, repo)
		if err != nil {
			return fmt.Errorf("render report: capture repository before authority confirmation: %w", err)
		}
		after, err := deps.captureRepo(ctx, repo)
		if err != nil {
			return fmt.Errorf("render report: capture repository after authority confirmation: %w", err)
		}
		if seed.RepositoryIdentity != before.Identity || seed.AnalysisRoot != analysisRoot ||
			seed.SelectedRevision != before.Head {
			return fmt.Errorf("render report: copied run authority does not match --repo")
		}
		authority, err := deps.confirmAuthorityScoped(
			ctx, analysisRoot, before, after, seed.CapturedInputPaths, true,
		)
		if err != nil {
			return fmt.Errorf("render report: confirm repository authority: %w", err)
		}
		if err := deps.generateAuthorized(absDir, authority); err != nil {
			return err
		}
	}
	fmt.Fprintf(stdout, "Report: %s/report.html\n", absDir)
	return nil
}

func runGuidedTour(runDir string, stderr io.Writer) error {
	absDir, err := filepath.Abs(runDir)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	outcome, err := editGuidedTourForRun(ctx, absDir, stderr, false)
	if err != nil {
		return err
	}
	if err := report.Generate(absDir); err != nil {
		return err
	}
	if outcome.Cached {
		fmt.Fprintf(stderr, "repomap: reused cached guided tour (%d response bytes)\n", outcome.ResponseBytes)
	} else {
		fmt.Fprintf(
			stderr,
			"repomap: guided tour accepted in %d ms (%d provider attempt(s))\n",
			outcome.LatencyMillis,
			outcome.RetryCount+1,
		)
	}
	fmt.Printf("Report: %s/report.html\n", absDir)
	return nil
}

func printUsage() {
	fmt.Fprintf(os.Stderr, "Usage: repomap [repo] [flags]\n")
	fmt.Fprintf(os.Stderr, "       repomap investigate <repo> --task-file <task.md> [flags]\n")
	fmt.Fprintf(os.Stderr, "       repomap doctor llm [--check]\n")
	fmt.Fprintf(os.Stderr, "       repomap serve [--run RUN_ID] [--source-episode PATH] [--port PORT]\n")
	fmt.Fprintf(os.Stderr, "       repomap cache clear [--debug-dir DIR]\n")
	fmt.Fprintf(os.Stderr, "\nFlags:\n")
	fmt.Fprintf(os.Stderr, "  --offline       skip model calls, local facts only\n")
	fmt.Fprintf(os.Stderr, "  --discover-surfaces discover bounded Go runtime surfaces (default true)\n")
	fmt.Fprintf(os.Stderr, "  --no-cache      disable cross-run model response caches\n")
	fmt.Fprintf(os.Stderr, "  --no-secrets    disable credential detection for this run (unsafe)\n")
	fmt.Fprintf(os.Stderr, "  --lang LANG     report language: en or ru (default: en)\n")
	fmt.Fprintf(os.Stderr, "  --gitlab-url URL create a standalone report linked to a GitLab project or repository remote host\n")
	fmt.Fprintf(os.Stderr, "  --github-url URL create a standalone report linked to a GitHub repository or remote host\n")
	fmt.Fprintf(os.Stderr, "  --no-open       do not open the generated HTML report\n")
	fmt.Fprintf(os.Stderr, "  --no-serve      generate a static report without starting the local server\n")
	fmt.Fprintf(os.Stderr, "  --port PORT     local report server port (default random)\n")
	fmt.Fprintf(os.Stderr, "  --debug-dir DIR debug artifact directory (default user cache)\n")
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
	fmt.Fprintf(os.Stderr, "  repomap doctor llm --check\n")
	fmt.Fprintf(os.Stderr, "  repomap serve\n")
	fmt.Fprintf(os.Stderr, "  repomap cache clear\n")
	fmt.Fprintf(os.Stderr, "  repomap --help\n")
}

// runDevUICLI re-renders a SAVED run directory from the current embedded
// report templates into a standalone report.html — the UI development
// playground. It makes no repository analysis and no provider calls: the
// saved run's report.json is read as-is and re-rendered with today's
// JS/CSS, so a template change is visible after a single rebuild.
//
//	repomap dev ui <run-dir> [--out <file>] [--state <hash>]
//
// The printed URL is the direct fixture URL (with the optional state hash).
func runDevUICLI(args []string, stdout io.Writer) error {
	runDir := ""
	outPath := ""
	stateHash := ""
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--out":
			index++
			if index >= len(args) || args[index] == "" {
				return fmt.Errorf("dev ui: --out requires a file path")
			}
			outPath = args[index]
		case "--state":
			index++
			if index >= len(args) || args[index] == "" {
				return fmt.Errorf("dev ui: --state requires a hash like #/map or #/map?focus=...")
			}
			stateHash = args[index]
		default:
			if runDir == "" {
				runDir = args[index]
			} else {
				return fmt.Errorf("usage: repomap dev ui <run-dir> [--out <file>] [--state <hash>]")
			}
		}
	}
	if runDir == "" {
		return fmt.Errorf("usage: repomap dev ui <run-dir> [--out <file>] [--state <hash>]")
	}
	if stdout == nil {
		return fmt.Errorf("dev ui: stdout is required")
	}
	absDir, err := filepath.Abs(runDir)
	if err != nil {
		return fmt.Errorf("dev ui: resolve run dir: %w", err)
	}
	data, err := report.ReadRunDir(absDir)
	if err != nil {
		return fmt.Errorf("dev ui: read saved run %s: %w", absDir, err)
	}
	if outPath == "" {
		outPath = filepath.Join(absDir, "report.ui-dev.html")
	}
	absOut, err := filepath.Abs(outPath)
	if err != nil {
		return fmt.Errorf("dev ui: resolve out path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(absOut), 0o755); err != nil {
		return fmt.Errorf("dev ui: create out directory: %w", err)
	}
	if err := report.WriteReportHTML(data, absOut); err != nil {
		return fmt.Errorf("dev ui: render from current templates: %w", err)
	}
	url := "file://" + absOut
	if stateHash != "" {
		hash := stateHash
		if !strings.HasPrefix(hash, "#") {
			hash = "#" + hash
		}
		url += hash
	}
	fmt.Fprintln(stdout, url)
	return nil
}
