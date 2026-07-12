package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	goplsanalyzer "github.com/dvordrova/repomap/internal/analyzer/golang/gopls"
	"github.com/dvordrova/repomap/internal/componentprobe"
	"github.com/dvordrova/repomap/internal/componentstudy"
	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/report"
)

const (
	artifactVersion     = 1
	maxStudyBundleBytes = 2 * 1024 * 1024
	maxPlanBytes        = 512 * 1024
	maxProbeBundleBytes = 4 * 1024 * 1024
	commandTimeout      = 30 * time.Second
	maxCallers          = 6
	maxCallees          = 6
)

type config struct {
	runDir          string
	studyBundlePath string
	planPath        string
	previousProbe   string
	frontierID      string
	outDir          string
}

type caseArtifact struct {
	Version                 int    `json:"version"`
	ProbeRound              int    `json:"probe_round"`
	InputAuthority          string `json:"input_authority"`
	RunDir                  string `json:"run_dir"`
	AnalysisRoot            string `json:"analysis_root"`
	StudyBundlePath         string `json:"study_bundle_path,omitempty"`
	PlanPath                string `json:"plan_path,omitempty"`
	PreviousProbePath       string `json:"previous_probe_path,omitempty"`
	PreviousBundleSHA256    string `json:"previous_bundle_sha256,omitempty"`
	AcceptedFrontierID      string `json:"accepted_frontier_id,omitempty"`
	ReportSHA256            string `json:"report_sha256"`
	RepositoryStateSHA256   string `json:"repository_state_sha256"`
	RepoName                string `json:"repo_name"`
	GoalID                  string `json:"goal_id"`
	ComponentID             string `json:"component_id"`
	ComponentName           string `json:"component_name"`
	PrimaryQuestionID       string `json:"primary_question_id"`
	SelectedFileCount       int    `json:"selected_file_count"`
	SelectedSymbolCount     int    `json:"selected_symbol_count"`
	Analyzer                string `json:"analyzer"`
	MaxCallers              int    `json:"max_callers"`
	MaxCallees              int    `json:"max_callees"`
	CommandTimeoutMillis    int64  `json:"command_timeout_ms"`
	ExternalRequestCount    int    `json:"external_request_count"`
	SelectedPathsAuthorized bool   `json:"selected_paths_authorized"`
}

type readinessArtifact struct {
	Version              int                   `json:"version"`
	ProbeRound           int                   `json:"probe_round"`
	Status               componentprobe.Status `json:"status"`
	NextAction           string                `json:"next_action"`
	ReadyForTeacher      bool                  `json:"ready_for_teacher"`
	NeedsFrontierProbe   bool                  `json:"needs_frontier_probe"`
	Partial              bool                  `json:"partial"`
	ProbeBudgetExhausted bool                  `json:"probe_budget_exhausted"`
	Blocked              bool                  `json:"blocked"`
	PrimaryQuestionID    string                `json:"primary_question_id"`
	SymbolProbeIDs       []string              `json:"symbol_probe_ids"`
	FrontierIDs          []string              `json:"frontier_ids"`
	CallsiteWindowCount  int                   `json:"callsite_window_count"`
	WarningCount         int                   `json:"warning_count"`
}

type metricsArtifact struct {
	Version                int              `json:"version"`
	DurationsMillis        map[string]int64 `json:"durations_ms"`
	Counts                 map[string]int   `json:"counts"`
	StudyBundleJSONBytes   int              `json:"study_bundle_json_bytes"`
	PlanJSONBytes          int              `json:"plan_json_bytes"`
	PreviousProbeJSONBytes int              `json:"previous_probe_json_bytes"`
	ProbeBundleJSONBytes   int              `json:"probe_bundle_json_bytes"`
	ExternalRequestCount   int              `json:"external_request_count"`
}

type errorArtifact struct {
	Version int    `json:"version"`
	Stage   string `json:"stage"`
	Error   string `json:"error"`
}

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	cfg, err := parseConfig(args, stderr)
	if err != nil {
		return err
	}
	if err := removeStaleError(cfg.outDir); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	totalStarted := time.Now()

	manifest, analysisRoot, currentRepository, freshnessMillis, err := loadAuthorizedRun(ctx, cfg.runDir)
	if err != nil {
		return err
	}

	inputsStarted := time.Now()
	caseData := caseArtifact{
		Version:                 artifactVersion,
		RunDir:                  cfg.runDir,
		AnalysisRoot:            analysisRoot,
		ReportSHA256:            manifest.ReportSHA256,
		RepositoryStateSHA256:   manifest.RepositoryStateSHA256,
		Analyzer:                "gopls",
		MaxCallers:              maxCallers,
		MaxCallees:              maxCallees,
		CommandTimeoutMillis:    commandTimeout.Milliseconds(),
		ExternalRequestCount:    0,
		SelectedPathsAuthorized: true,
	}
	metrics := metricsArtifact{
		Version: artifactVersion,
		DurationsMillis: map[string]int64{
			"repository_freshness": freshnessMillis,
		},
		Counts: map[string]int{
			"manifest_openable_paths": len(manifest.OpenablePaths),
			"repository_dirty_files":  len(currentRepository.Dirty),
		},
		ExternalRequestCount: 0,
	}

	var (
		study componentstudy.Bundle
		plan  componentstudy.Plan
		prior componentprobe.Bundle
	)
	if cfg.previousProbe != "" {
		caseData.ProbeRound = componentprobe.RoundFrontier
		caseData.InputAuthority = "prior_probe_frontier"
		caseData.PreviousProbePath = cfg.previousProbe
		caseData.AcceptedFrontierID = cfg.frontierID
		previousJSON, readErr := readJSONArtifact(cfg.previousProbe, maxProbeBundleBytes, &prior)
		if readErr != nil {
			return fmt.Errorf("componentprobe-playground: read previous probe: %w", readErr)
		}
		if validateErr := prior.Validate(); validateErr != nil {
			return fmt.Errorf("componentprobe-playground: validate previous probe: %w", validateErr)
		}
		if prior.Round != componentprobe.RoundInitial || len(prior.SymbolProbes) == 0 {
			return fmt.Errorf("componentprobe-playground: previous probe must be a usable round-1 bundle")
		}
		previousDigest, digestErr := componentprobe.SHA256(prior)
		if digestErr != nil {
			return fmt.Errorf("componentprobe-playground: digest previous probe: %w", digestErr)
		}
		caseData.PreviousBundleSHA256 = previousDigest
		caseData.RepoName = prior.SymbolProbes[0].Structural.RepoName
		caseData.GoalID = prior.Focus.Goal.ID
		caseData.ComponentID = prior.Focus.Component.ID
		caseData.ComponentName = prior.Focus.Component.Name
		caseData.PrimaryQuestionID = prior.Focus.PrimaryQuestion.ID
		caseData.SelectedFileCount = 1
		caseData.SelectedSymbolCount = 1
		metrics.PreviousProbeJSONBytes = len(previousJSON)
		metrics.Counts["selected_files"] = 1
		metrics.Counts["selected_symbols"] = 1
	} else {
		caseData.ProbeRound = componentprobe.RoundInitial
		caseData.InputAuthority = "run_manifest_and_normalized_plan"
		caseData.StudyBundlePath = cfg.studyBundlePath
		caseData.PlanPath = cfg.planPath
		studyJSON, readErr := readJSONArtifact(cfg.studyBundlePath, maxStudyBundleBytes, &study)
		if readErr != nil {
			return fmt.Errorf("componentprobe-playground: read study bundle: %w", readErr)
		}
		if validateErr := study.Validate(); validateErr != nil {
			return fmt.Errorf("componentprobe-playground: validate study bundle: %w", validateErr)
		}
		planJSON, readErr := readJSONArtifact(cfg.planPath, maxPlanBytes, &plan)
		if readErr != nil {
			return fmt.Errorf("componentprobe-playground: read plan: %w", readErr)
		}
		if validateErr := plan.Validate(study); validateErr != nil {
			return fmt.Errorf("componentprobe-playground: validate plan: %w", validateErr)
		}
		if authorizeErr := authorizeSelectedPaths(analysisRoot, manifest.OpenablePaths, plan); authorizeErr != nil {
			return authorizeErr
		}
		caseData.RepoName = study.RepoName
		caseData.GoalID = study.Goal.ID
		caseData.ComponentID = study.Component.ID
		caseData.ComponentName = study.Component.Name
		caseData.PrimaryQuestionID = plan.PrimaryQuestionID
		caseData.SelectedFileCount = len(plan.SelectedFiles)
		caseData.SelectedSymbolCount = len(plan.SelectedSymbols)
		metrics.StudyBundleJSONBytes = len(studyJSON)
		metrics.PlanJSONBytes = len(planJSON)
		metrics.Counts["selected_files"] = len(plan.SelectedFiles)
		metrics.Counts["selected_symbols"] = len(plan.SelectedSymbols)
	}
	metrics.DurationsMillis["load_and_authorize"] = time.Since(inputsStarted).Milliseconds()
	if err := writeJSONArtifact(cfg.outDir, "case.json", caseData); err != nil {
		return err
	}

	provider := goplsanalyzer.New(goplsanalyzer.Options{
		MaxCallers:     maxCallers,
		MaxCallees:     maxCallees,
		CommandTimeout: commandTimeout,
	})
	collectStarted := time.Now()
	var bundle componentprobe.Bundle
	var collectErr error
	if cfg.previousProbe != "" {
		bundle, collectErr = componentprobe.CollectFrontier(
			ctx, provider, analysisRoot, prior, cfg.frontierID, componentprobe.Options{},
		)
	} else {
		bundle, collectErr = componentprobe.Collect(
			ctx, provider, analysisRoot, study, plan, componentprobe.Options{},
		)
	}
	metrics.DurationsMillis["collect"] = time.Since(collectStarted).Milliseconds()
	metrics.DurationsMillis["total"] = time.Since(totalStarted).Milliseconds()
	if collectErr != nil {
		writeErr := writeJSONArtifact(cfg.outDir, filepath.Join("probe", "error.json"), errorArtifact{
			Version: artifactVersion,
			Stage:   "collect",
			Error:   collectErr.Error(),
		})
		if writeErr != nil {
			return errors.Join(collectErr, writeErr)
		}
	}

	if bundle.Version != 0 {
		if bundle.Round == componentprobe.RoundFrontier {
			if validateErr := bundle.ValidateAgainst(prior); validateErr != nil {
				return fmt.Errorf("componentprobe-playground: validate frontier round: %w", validateErr)
			}
		}
		bundleJSON, marshalErr := json.Marshal(bundle)
		if marshalErr != nil {
			return fmt.Errorf("componentprobe-playground: measure probe bundle: %w", marshalErr)
		}
		metrics.ProbeBundleJSONBytes = len(bundleJSON)
		metrics.Counts["symbol_probes"] = len(bundle.SymbolProbes)
		metrics.Counts["callsite_windows"] = len(bundle.CallsiteWindows)
		metrics.Counts["frontier_candidates"] = len(bundle.Frontier)
		metrics.Counts["probe_warnings"] = len(bundle.Warnings)
		if err := writeJSONArtifact(cfg.outDir, filepath.Join("probe", "bundle.json"), bundle); err != nil {
			return err
		}
		if err := writeJSONArtifact(
			cfg.outDir,
			filepath.Join("probe", "readiness.json"),
			readinessFromBundle(bundle),
		); err != nil {
			return err
		}
	}
	if err := writeJSONArtifact(cfg.outDir, "metrics.json", metrics); err != nil {
		return err
	}
	if collectErr != nil {
		return fmt.Errorf("componentprobe-playground: collect round %d: %w", caseData.ProbeRound, collectErr)
	}

	bundlePath := filepath.Join(cfg.outDir, "probe", "bundle.json")
	fmt.Fprintln(stdout, bundlePath)
	fmt.Fprintf(
		stderr,
		"componentprobe-playground: round %d %s with %d symbol probes, %d callsite windows, and %d frontier candidates in %d ms; no model request\n",
		bundle.Round,
		bundle.Status,
		len(bundle.SymbolProbes),
		len(bundle.CallsiteWindows),
		len(bundle.Frontier),
		metrics.DurationsMillis["collect"],
	)
	return nil
}

func parseConfig(args []string, stderr io.Writer) (config, error) {
	var cfg config
	flags := flag.NewFlagSet("componentprobe-playground", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&cfg.runDir, "run-dir", "", "verified repomap v2 run directory")
	flags.StringVar(&cfg.studyBundlePath, "study-bundle", "", "bounded component-study bundle.json")
	flags.StringVar(&cfg.planPath, "plan", "", "locally normalized component-study plan.json")
	flags.StringVar(&cfg.previousProbe, "previous-probe", "", "validated round-1 probe bundle for one frontier continuation")
	flags.StringVar(&cfg.frontierID, "frontier-id", "", "opaque frontier id from --previous-probe")
	flags.StringVar(&cfg.outDir, "out-dir", "", "directory for local probe artifacts")
	if err := flags.Parse(args); err != nil {
		return config{}, err
	}
	if flags.NArg() != 0 {
		return config{}, fmt.Errorf("componentprobe-playground: unexpected positional arguments")
	}
	if cfg.runDir == "" || cfg.outDir == "" {
		return config{}, fmt.Errorf(
			"componentprobe-playground: --run-dir and --out-dir are required",
		)
	}
	frontierMode := cfg.previousProbe != "" || cfg.frontierID != ""
	if frontierMode {
		if cfg.previousProbe == "" || strings.TrimSpace(cfg.frontierID) == "" {
			return config{}, fmt.Errorf("componentprobe-playground: --previous-probe and --frontier-id are required together")
		}
		if cfg.studyBundlePath != "" || cfg.planPath != "" {
			return config{}, fmt.Errorf("componentprobe-playground: frontier mode does not accept --study-bundle or --plan")
		}
	} else if cfg.studyBundlePath == "" || cfg.planPath == "" {
		return config{}, fmt.Errorf("componentprobe-playground: --study-bundle and --plan are required for round 1")
	}
	paths := []struct {
		label string
		value *string
	}{
		{label: "run directory", value: &cfg.runDir},
		{label: "study bundle", value: &cfg.studyBundlePath},
		{label: "plan", value: &cfg.planPath},
		{label: "previous probe", value: &cfg.previousProbe},
		{label: "output directory", value: &cfg.outDir},
	}
	for _, item := range paths {
		if *item.value == "" {
			continue
		}
		absolute, err := filepath.Abs(*item.value)
		if err != nil {
			return config{}, fmt.Errorf(
				"componentprobe-playground: resolve %s: %w",
				item.label,
				err,
			)
		}
		*item.value = filepath.Clean(absolute)
	}
	cfg.frontierID = strings.TrimSpace(cfg.frontierID)
	return cfg, nil
}

func loadAuthorizedRun(
	ctx context.Context,
	runDir string,
) (report.RunManifest, string, freshness.RepositoryState, int64, error) {
	manifest, err := report.ReadRunManifest(runDir)
	if err != nil {
		return report.RunManifest{}, "", freshness.RepositoryState{}, 0,
			fmt.Errorf("componentprobe-playground: verify run: %w", err)
	}
	analysisRoot, err := manifest.ResolveAnalysisRoot()
	if err != nil {
		return report.RunManifest{}, "", freshness.RepositoryState{}, 0,
			fmt.Errorf("componentprobe-playground: resolve analysis root: %w", err)
	}
	started := time.Now()
	current, err := freshness.CaptureRepository(ctx, manifest.RepositoryState.Identity)
	if err != nil {
		return report.RunManifest{}, "", freshness.RepositoryState{}, 0,
			fmt.Errorf("componentprobe-playground: capture current repository state: %w", err)
	}
	if err := manifest.VerifyRepositoryState(current); err != nil {
		return report.RunManifest{}, "", freshness.RepositoryState{}, 0,
			fmt.Errorf("componentprobe-playground: reconcile saved run: %w", err)
	}
	return manifest, analysisRoot, current, time.Since(started).Milliseconds(), nil
}

func authorizeSelectedPaths(
	analysisRoot string,
	openablePaths []string,
	plan componentstudy.Plan,
) error {
	openable := make(map[string]struct{}, len(openablePaths))
	for _, path := range openablePaths {
		openable[path] = struct{}{}
	}
	selected := make(map[string]struct{}, len(plan.SelectedFiles)+len(plan.SelectedSymbols))
	for _, file := range plan.SelectedFiles {
		selected[file.Path] = struct{}{}
	}
	for _, symbol := range plan.SelectedSymbols {
		selected[symbol.Path] = struct{}{}
	}
	paths := make([]string, 0, len(selected))
	for path := range selected {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		if _, ok := openable[path]; !ok {
			return fmt.Errorf("componentprobe-playground: selected path %q is not authorized by the run manifest", path)
		}
		info, err := os.Stat(filepath.Join(analysisRoot, filepath.FromSlash(path)))
		if err != nil {
			return fmt.Errorf("componentprobe-playground: inspect selected path %q: %w", path, err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("componentprobe-playground: selected path %q is not a regular file", path)
		}
	}
	return nil
}

func readinessFromBundle(bundle componentprobe.Bundle) readinessArtifact {
	readiness := readinessArtifact{
		Version:             artifactVersion,
		ProbeRound:          bundle.Round,
		Status:              bundle.Status,
		PrimaryQuestionID:   bundle.Focus.PrimaryQuestion.ID,
		SymbolProbeIDs:      make([]string, 0, len(bundle.SymbolProbes)),
		FrontierIDs:         make([]string, 0, len(bundle.Frontier)),
		CallsiteWindowCount: len(bundle.CallsiteWindows),
		WarningCount:        len(bundle.Warnings),
	}
	for _, probe := range bundle.SymbolProbes {
		readiness.SymbolProbeIDs = append(readiness.SymbolProbeIDs, probe.ID)
	}
	for _, frontier := range bundle.Frontier {
		readiness.FrontierIDs = append(readiness.FrontierIDs, frontier.ID)
	}
	if bundle.Round == componentprobe.RoundFrontier {
		readiness.ProbeBudgetExhausted = true
		switch bundle.Status {
		case componentprobe.StatusConnected:
			readiness.NextAction = "teach"
			readiness.ReadyForTeacher = true
		case componentprobe.StatusFrontier:
			readiness.NextAction = "teach_partial"
			readiness.ReadyForTeacher = true
			readiness.Partial = true
		case componentprobe.StatusBlocked:
			readiness.NextAction = "stop"
			readiness.Blocked = true
		}
		return readiness
	}
	switch bundle.Status {
	case componentprobe.StatusConnected:
		readiness.NextAction = "teach"
		readiness.ReadyForTeacher = true
	case componentprobe.StatusFrontier:
		readiness.NextAction = "probe_frontier"
		readiness.NeedsFrontierProbe = true
	case componentprobe.StatusBlocked:
		readiness.NextAction = "stop"
		readiness.Blocked = true
	}
	return readiness
}

func readJSONArtifact(path string, limit int64, target any) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("not a regular file")
	}
	if info.Size() <= 0 || info.Size() > limit {
		return nil, fmt.Errorf("size must be between 1 and %d bytes", limit)
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	if len(data) == 0 || int64(len(data)) > limit {
		return nil, fmt.Errorf("size must be between 1 and %d bytes", limit)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, fmt.Errorf("multiple json values")
		}
		return nil, fmt.Errorf("trailing data: %w", err)
	}
	return data, nil
}

func writeJSONArtifact(root, name string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("componentprobe-playground: marshal %s: %w", name, err)
	}
	return writeArtifact(root, name, append(data, '\n'))
}

func writeArtifact(root, name string, data []byte) error {
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("componentprobe-playground: create artifact directory: %w", err)
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, data, 0o600); err != nil {
		return fmt.Errorf("componentprobe-playground: write %s: %w", name, err)
	}
	if err := os.Rename(temporary, path); err != nil {
		return fmt.Errorf("componentprobe-playground: publish %s: %w", name, err)
	}
	return nil
}

func removeStaleError(root string) error {
	path := filepath.Join(root, "probe", "error.json")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("componentprobe-playground: remove stale probe error: %w", err)
	}
	return nil
}
