package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/reportserver"
	"github.com/dvordrova/repomap/internal/secretscan"
	"github.com/dvordrova/repomap/internal/tasklens"
)

type taskInvestigationProvider interface {
	TaskInvestigationPromptJSON(tasklens.Bundle) ([]byte, error)
	InvestigateTaskMeasured(context.Context, tasklens.Bundle) (modelProviderResult, error)
}

// modelProviderResult mirrors the stable metrics used by this command while
// the concrete DeepSeek client remains the reference implementation.
type modelProviderResult struct {
	Content               []byte
	Attempts              int
	RequestBytes          int
	InputTokens           int
	OutputTokens          int
	PromptCacheHitTokens  int
	PromptCacheMissTokens int
}

type deepSeekTaskProvider struct {
	client *deepseek.Client
}

func (provider deepSeekTaskProvider) TaskInvestigationPromptJSON(bundle tasklens.Bundle) ([]byte, error) {
	return provider.client.TaskInvestigationPromptJSON(bundle)
}

func (provider deepSeekTaskProvider) InvestigateTaskMeasured(
	ctx context.Context,
	bundle tasklens.Bundle,
) (modelProviderResult, error) {
	result, err := provider.client.InvestigateTaskMeasured(ctx, bundle)
	return modelProviderResult{
		Content: result.Content, Attempts: result.Attempts, RequestBytes: result.RequestBytes,
		InputTokens: result.InputTokens, OutputTokens: result.OutputTokens,
		PromptCacheHitTokens:  result.PromptCacheHitTokens,
		PromptCacheMissTokens: result.PromptCacheMissTokens,
	}, err
}

type investigateDependencies struct {
	ctx          context.Context
	stdout       io.Writer
	stderr       io.Writer
	newProvider  func(bool) (taskInvestigationProvider, deepseek.EffectiveConfig, error)
	captureRepo  func(context.Context, string) (freshness.RepositoryState, error)
	finalizePack func(tasklens.Pack, string) (tasklens.Pack, bool)
	serveReport  func(context.Context, reportserver.Options) error
	openReport   func(string) error
}

func runInvestigate(args []string, deps investigateDependencies) error {
	fs := flag.NewFlagSet("investigate", flag.ContinueOnError)
	stderr := deps.stderr
	if stderr == nil {
		stderr = os.Stderr
	}
	stdout := deps.stdout
	if stdout == nil {
		stdout = os.Stdout
	}
	fs.SetOutput(stderr)
	taskFile := fs.String("task-file", "", "path to one task description")
	debugDir := fs.String("debug-dir", defaultDebugDir(), "directory for Task Lens run artifacts")
	noOpen := fs.Bool("no-open", false, "do not open the generated report")
	noServe := fs.Bool("no-serve", false, "generate a static report without starting the local server")
	port := fs.Int("port", 0, "local report server port (default: random)")
	offline := fs.Bool("offline", false, "skip the compact synthesis call")
	strictSnapshot := fs.Bool("strict-snapshot", true, "fail when captured source changes during investigation")
	// Keep the product-facing form `investigate <repo> --task-file ...` while
	// still accepting the standard flags-first spelling.
	repositoryArg := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		repositoryArg = args[0]
		args = args[1:]
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if repositoryArg == "" && fs.NArg() == 1 {
		repositoryArg = fs.Arg(0)
	} else if fs.NArg() != 0 {
		return fmt.Errorf("usage: repomap investigate <repository> --task-file <task.md>")
	}
	if repositoryArg == "" {
		return fmt.Errorf("usage: repomap investigate <repository> --task-file <task.md>")
	}
	if strings.TrimSpace(*taskFile) == "" {
		return fmt.Errorf("investigate: --task-file is required")
	}
	if strings.TrimSpace(*debugDir) == "" {
		return fmt.Errorf("investigate: --debug-dir is required for replayable artifacts")
	}
	if *port < 0 || *port > 65535 {
		return fmt.Errorf("investigate: --port must be between 0 and 65535")
	}
	repo, err := filepath.Abs(repositoryArg)
	if err != nil {
		return fmt.Errorf("investigate: resolve repository path: %w", err)
	}
	taskPath, err := filepath.Abs(*taskFile)
	if err != nil {
		return fmt.Errorf("investigate: resolve task path: %w", err)
	}
	rawTask, err := readBoundedTaskFile(taskPath)
	if err != nil {
		return fmt.Errorf("investigate: read task file: %w", err)
	}
	taskText, err := tasklens.ParseTaskText(rawTask)
	if err != nil {
		return err
	}

	ctx := deps.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	captureRepo := deps.captureRepo
	if captureRepo == nil {
		captureRepo = freshness.CaptureRepository
	}
	initialState, err := captureRepo(ctx, repo)
	if err != nil {
		return fmt.Errorf("investigate: capture repository state before retrieval: %w", err)
	}
	fmt.Fprintln(stderr, "repomap: collecting bounded task evidence")
	collected, err := tasklens.CollectWithTrace(ctx, tasklens.CollectOptions{
		RepositoryPath: repo, TaskText: taskText,
	})
	if err != nil {
		return err
	}
	bundle := collected.Bundle
	retrievalTrace := collected.Trace
	if bundle.Repository.Revision != initialState.Head {
		return fmt.Errorf("investigate: collector revision does not match the requested worktree")
	}
	initialStateSHA, err := tasklens.RepositoryStateSHA(initialState)
	if err != nil {
		return fmt.Errorf("investigate: identify initial repository state: %w", err)
	}
	if bundle.Repository.StateSHA256 != initialStateSHA {
		return fmt.Errorf("investigate: collector state does not match the requested worktree snapshot")
	}
	sparseEvidence := len(bundle.Anchors) < tasklens.PreferredMinVisibleAnchors
	finalizePack := deps.finalizePack
	if finalizePack == nil {
		finalizePack = report.FinalizeTaskInvestigationPack
	}
	var proposal tasklens.Proposal
	var pack tasklens.Pack
	packReady := false
	localComplete := false
	if bundle.CheapExit.Eligible {
		proposal, pack, localComplete, err = previewLocalTaskInvestigationPack(bundle, finalizePack)
		if err != nil {
			return fmt.Errorf("investigate: preview local-complete pack: %w", err)
		}
		packReady = localComplete
	}
	skipSynthesis := *offline || sparseEvidence || localComplete

	var provider taskInvestigationProvider
	var providerConfig deepseek.EffectiveConfig
	var promptJSON []byte
	stablePromptJSON, err := tasklens.StablePromptJSON(bundle)
	if err != nil {
		return fmt.Errorf("investigate: build stable semantic prompt: %w", err)
	}
	if skipSynthesis {
		providerConfig = deepseek.DefaultTaskInvestigationConfig()
		promptJSON = stablePromptJSON
	} else {
		newProvider := deps.newProvider
		if newProvider == nil {
			newProvider = func(bool) (taskInvestigationProvider, deepseek.EffectiveConfig, error) {
				client, clientErr := deepseek.NewFromEnv()
				if clientErr != nil {
					return nil, deepseek.EffectiveConfig{}, clientErr
				}
				client.OnWait = func(progress deepseek.WaitProgress) {
					fmt.Fprintf(stderr, "repomap: task synthesis still running (%s)\n", progress.Elapsed.Round(time.Second))
				}
				config := client.EffectiveConfig()
				config.MaxTokens = client.TaskInvestigationMaxTokens()
				return deepSeekTaskProvider{client: client}, config, nil
			}
		}
		provider, providerConfig, err = newProvider(false)
		if err == nil {
			promptJSON, err = provider.TaskInvestigationPromptJSON(bundle)
		}
	}
	if err != nil {
		return fmt.Errorf("investigate: prepare semantic prompt: %w", err)
	}

	bundleSHA, err := tasklens.BundleHash(bundle)
	if err != nil {
		return err
	}
	attempt := tasklens.Attempt{
		Version: tasklens.AttemptVersion, BundleSHA256: bundleSHA,
		PromptVersion: tasklens.PromptVersion, PromptSHA256: tasklens.SHA256(stablePromptJSON),
	}
	packState := "accepted"
	if skipSynthesis {
		if localComplete {
			attempt.State = "skipped_local_complete"
			packState = "accepted_local_complete"
		} else if *offline {
			attempt.State = "skipped_offline"
		} else {
			attempt.State = "skipped_insufficient_evidence"
			attempt.Warnings = append(attempt.Warnings,
				tasklens.AttemptWarningSparseEvidence)
		}
		if !localComplete {
			packState = "partial_local"
		}
		if !packReady {
			proposal, err = tasklens.LocalProposal(bundle)
		}
	} else {
		fmt.Fprintln(stderr, "repomap: editing one compact task investigation")
		callStarted := time.Now()
		result, callErr := provider.InvestigateTaskMeasured(ctx, bundle)
		requestBytes := result.RequestBytes
		if requestBytes == 0 && result.Attempts > 0 {
			requestBytes = len(promptJSON) * result.Attempts
		}
		attempt.Provider = tasklens.ProviderMetrics{
			Calls: 1, TransportAttempts: result.Attempts, RequestBytes: requestBytes,
			ResponseBytes: len(result.Content), InputTokens: result.InputTokens,
			OutputTokens: result.OutputTokens, PromptCacheHitTokens: result.PromptCacheHitTokens,
			PromptCacheMissTokens: result.PromptCacheMissTokens,
			LatencyMillis:         time.Since(callStarted).Milliseconds(),
		}
		recordTaskResponse(&attempt, result.Content)
		if callErr != nil {
			attempt.State = "provider_failed"
			attempt.ReductionError = tasklens.ReductionErrorProviderFailed
			attempt.Warnings = append(attempt.Warnings, tasklens.AttemptWarningProviderFailed)
			packState = "partial_local"
			proposal, err = tasklens.LocalProposal(bundle)
		} else {
			if attempt.RawResponseOmittedReason != "" {
				err = fmt.Errorf("%s", tasklens.RawResponseOmissionReductionError(attempt.RawResponseOmittedReason))
			} else {
				proposal, err = tasklens.DecodeProposal(result.Content)
			}
			if err == nil {
				var reductionWarnings []string
				pack, reductionWarnings, err = tasklens.ReduceProposal(bundle, proposal)
				if len(reductionWarnings) > 0 {
					attempt.Warnings = append(attempt.Warnings, reductionWarnings...)
				}
				if err == nil {
					packReady = true
					if len(reductionWarnings) > 0 {
						attempt.State = "accepted_with_rejections"
						packState = "accepted_partial"
					} else {
						attempt.State = "accepted"
					}
				}
			}
			if err != nil {
				attempt.State = "rejected"
				attempt.ReductionError = err.Error()
				attempt.Warnings = append(attempt.Warnings, tasklens.AttemptWarningResponseRejected)
				packState = "partial_local"
				proposal, err = tasklens.LocalProposal(bundle)
			}
		}
	}
	if err != nil {
		return err
	}
	if !packReady {
		pack, err = tasklens.BuildPack(bundle, proposal)
		if err != nil {
			return err
		}
	}
	pack, sufficient := finalizePack(pack, attempt.State)

	runID := debugdump.GenerateRunID("task-lens-" + bundle.ID)
	writer, err := debugdump.NewWriter(*debugDir, runID, true)
	if err != nil {
		return err
	}
	defer writer.Close()
	bundleBytes, err := marshalTaskArtifact(bundle)
	if err != nil {
		return err
	}
	attemptBytes, err := marshalTaskArtifact(attempt)
	if err != nil {
		return err
	}
	packBytes, err := marshalTaskArtifact(pack)
	if err != nil {
		return err
	}
	traceBytes, err := marshalTaskArtifact(retrievalTrace)
	if err != nil {
		return err
	}
	traceMarkdown, err := tasklens.RenderRetrievalTraceMarkdown(retrievalTrace)
	if err != nil {
		return fmt.Errorf("investigate: render retrieval trace: %w", err)
	}
	traceMarkdownBytes := []byte(traceMarkdown)
	for name, data := range map[string][]byte{
		tasklens.BundleFile:        bundleBytes,
		tasklens.AttemptFile:       attemptBytes,
		tasklens.PackFile:          packBytes,
		tasklens.TraceJSONFile:     traceBytes,
		tasklens.TraceMarkdownFile: traceMarkdownBytes,
	} {
		if err := writer.WriteFile(name, data); err != nil {
			return err
		}
	}
	status := tasklens.Status{
		Version: tasklens.StatusVersion, State: packState,
		Sufficient: sufficient,
		TaskID:     pack.ID, BundleSHA256: bundleSHA, AttemptSHA256: tasklens.SHA256(attemptBytes),
		PackSHA256:                   tasklens.SHA256(packBytes),
		RetrievalTraceSHA256:         tasklens.SHA256(traceBytes),
		RetrievalTraceMarkdownSHA256: tasklens.SHA256(traceMarkdownBytes),
		CapturedRevision:             bundle.Repository.Revision,
		TreeHash:                     bundle.Repository.TreeHash, Locality: bundle.Locality,
		StagesSkipped: append([]string(nil), bundle.StagesSkipped...), Provider: attempt.Provider,
		CheapExit: bundle.CheapExit,
		Budgets:   bundle.Budgets, Warnings: uniqueTaskWarnings(attempt.Warnings, pack.Warnings),
	}
	statusBytes, err := marshalTaskArtifact(status)
	if err != nil {
		return err
	}
	if err := writer.WriteFile(tasklens.StatusFile, statusBytes); err != nil {
		return err
	}
	latency := attempt.Provider.LatencyMillis
	metadata := debugdump.RunMeta{
		RunID: runID, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		RepoName: bundle.Repository.Identity, RepoPath: repo, Command: "investigate",
		Model: providerConfig.Model, Endpoint: providerConfig.Endpoint,
		PromptVersion: tasklens.PromptVersion, CompactContextBytes: len(promptJSON),
		ExternalRequestBytes: attempt.Provider.RequestBytes,
		ProviderRequestCount: attempt.Provider.Calls, ProviderLatencyMillis: &latency,
		AuthMode: providerConfig.AuthMode, TimeoutMillis: providerConfig.Timeout.Milliseconds(),
		MaxTokens: providerConfig.MaxTokens,
		EffectiveOptions: debugdump.EffectiveOptions{
			Offline: *offline, NoOpen: *noOpen, NoServe: *noServe, Port: *port, DebugEnabled: true,
		},
		RequestAttempts: []debugdump.RequestAttempt{{
			Stage: "task_investigation", State: attempt.State,
			RequestBytes: attempt.Provider.RequestBytes, ProviderCallCount: attempt.Provider.Calls,
			LatencyMillis: &latency,
		}},
	}
	if err := writer.WriteMetadata(metadata); err != nil {
		return err
	}

	reportData, err := report.ReadRunDir(writer.RunDir())
	if err != nil {
		return fmt.Errorf("investigate: read Task Lens report inputs: %w", err)
	}
	if err := requireTaskInvestigationReport(reportData); err != nil {
		return err
	}
	currentState, err := captureRepo(ctx, repo)
	if err != nil {
		return fmt.Errorf("investigate: capture repository state after retrieval: %w", err)
	}
	analysisRoot, err := resolveAnalysisRoot(repo)
	if err != nil {
		return err
	}
	authority, err := report.ConfirmRunAuthorityScoped(
		ctx, analysisRoot, initialState, currentState, report.CapturedInputPaths(reportData), *strictSnapshot,
	)
	if err != nil {
		return fmt.Errorf("investigate: confirm report authority: %w", err)
	}
	if err := report.GenerateAuthorized(writer.RunDir(), authority); err != nil {
		return fmt.Errorf("investigate: generate authorized report: %w", err)
	}
	reportPath := filepath.Join(writer.RunDir(), "report.html")
	linkLatest(*debugDir, writer.RunDir(), stderr)
	fmt.Fprintf(stderr, "Task investigation: %s\n", writer.RunDir())
	fmt.Fprintf(stderr, "Report: %s\n", reportPath)
	fmt.Fprintf(stderr, "Task route: #/investigate/%s\n", pack.ID)
	fmt.Fprintf(stdout, "%s\n", writer.RunDir())

	serveReport := deps.serveReport
	if serveReport == nil {
		serveReport = reportserver.Serve
	}
	open := deps.openReport
	if open == nil {
		open = openReport
	}
	if !*noServe {
		serveCtx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
		defer cancel()
		localAnalyzer := newReportAnalyzer()
		return serveReport(serveCtx, reportserver.Options{
			RunsDir: *debugDir, InitialRunID: runID, Port: *port,
			LocationResolver: localAnalyzer, ExactSymbolAnalyzer: localAnalyzer, ReferenceFinder: localAnalyzer,
			Logf: func(format string, args ...any) { fmt.Fprintf(stderr, "repomap: "+format+"\n", args...) },
			OnReady: func(location string) error {
				location = reportTaskURL(location, pack.ID)
				fmt.Fprintf(stderr, "Serving reports: %s (press Ctrl-C to stop)\n", location)
				if !*noOpen {
					return open(location)
				}
				return nil
			},
		})
	}
	if !*noOpen {
		return open(reportPath + "#/investigate/" + pack.ID)
	}
	return nil
}

func previewLocalTaskInvestigationPack(
	bundle tasklens.Bundle,
	finalizePack func(tasklens.Pack, string) (tasklens.Pack, bool),
) (tasklens.Proposal, tasklens.Pack, bool, error) {
	proposal, err := tasklens.LocalProposal(bundle)
	if err != nil {
		return tasklens.Proposal{}, tasklens.Pack{}, false, err
	}
	pack, err := tasklens.BuildPack(bundle, proposal)
	if err != nil {
		return tasklens.Proposal{}, tasklens.Pack{}, false, err
	}
	pack, sufficient := finalizePack(pack, "skipped_local_complete")
	return proposal, pack, sufficient, nil
}

func readBoundedTaskFile(taskPath string) ([]byte, error) {
	info, err := os.Lstat(taskPath)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > tasklens.MaxTaskBytes {
		return nil, fmt.Errorf("task file is not a bounded regular file")
	}
	file, err := os.Open(taskPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil || !openedInfo.Mode().IsRegular() || !os.SameFile(info, openedInfo) {
		return nil, fmt.Errorf("task file changed while it was opened")
	}
	raw, err := io.ReadAll(io.LimitReader(file, tasklens.MaxTaskBytes+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > tasklens.MaxTaskBytes {
		return nil, fmt.Errorf("task file exceeds the bounded size")
	}
	return raw, nil
}

func recordTaskResponse(attempt *tasklens.Attempt, content []byte) {
	if attempt == nil || len(content) == 0 {
		return
	}
	attempt.ResponseSHA256 = tasklens.SHA256(content)
	switch {
	case len(content) > tasklens.MaxSavedRawResponseBytes:
		attempt.RawResponseOmittedReason = tasklens.RawResponseOmittedSize
		attempt.Warnings = append(attempt.Warnings, tasklens.AttemptWarningResponseSize)
	case func() bool { _, found := secretscan.Detect(string(content)); return found }():
		attempt.RawResponseOmittedReason = tasklens.RawResponseOmittedSecret
		attempt.Warnings = append(attempt.Warnings, tasklens.AttemptWarningResponseSecret)
	default:
		attempt.RawResponse = string(content)
	}
}

func requireTaskInvestigationReport(data *report.ReportData) error {
	if data == nil || data.TaskInvestigation == nil {
		if data != nil {
			for _, warning := range data.Warnings {
				if strings.HasPrefix(warning, "task investigation unavailable:") {
					return fmt.Errorf(
						"investigate: generated Task Lens artifacts failed report projection: %s",
						strings.TrimSpace(strings.TrimPrefix(warning, "task investigation unavailable:")),
					)
				}
			}
		}
		return fmt.Errorf("investigate: generated Task Lens artifacts did not produce a report projection")
	}
	return nil
}

func marshalTaskArtifact(value any) ([]byte, error) {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("investigate: encode artifact: %w", err)
	}
	raw = append(raw, '\n')
	if len(raw) > tasklens.MaxArtifactBytes {
		return nil, fmt.Errorf("investigate: artifact is outside the bounded saved size")
	}
	if kind, found := secretscan.Detect(string(raw)); found {
		return nil, fmt.Errorf("investigate: artifact rejected because %s was detected", kind)
	}
	return raw, nil
}

func reportTaskURL(location, taskID string) string {
	base, _, _ := strings.Cut(location, "#")
	return base + "#/investigate/" + taskID
}

func uniqueTaskWarnings(groups ...[]string) []string {
	seen := make(map[string]struct{})
	var result []string
	for _, group := range groups {
		for _, warning := range group {
			if _, duplicate := seen[warning]; duplicate {
				continue
			}
			seen[warning] = struct{}{}
			result = append(result, warning)
		}
	}
	return result
}
