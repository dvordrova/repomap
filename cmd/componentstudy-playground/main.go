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
	"sort"
	"strings"
	"syscall"
	"time"

	goplsanalyzer "github.com/dvordrova/repomap/internal/analyzer/golang/gopls"
	"github.com/dvordrova/repomap/internal/componentstudy"
	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/report"
)

const (
	artifactVersion       = 1
	maxPackageFiles       = 16
	maxSymbolFiles        = 4
	maxSymbolsPerFile     = 4
	maxSeedSymbols        = 16
	maxSeedEvidence       = 16
	maxPlannerBundleBytes = 12 * 1024
)

type config struct {
	runDir                string
	component             string
	anchor                string
	goal                  string
	outDir                string
	live                  bool
	responseFile          string
	responsePromptVersion string
}

type authorizedRun struct {
	manifest           report.RunManifest
	data               report.ReportData
	analysisRoot       string
	component          report.Component
	componentAuthority report.ComponentAuthority
	anchor             report.AnchorGroup
	anchorAuthority    report.AnchorAuthority
	openable           map[string]struct{}
}

type caseArtifact struct {
	Version               int                   `json:"version"`
	Mode                  string                `json:"mode"`
	RunDir                string                `json:"run_dir"`
	AnalysisRoot          string                `json:"analysis_root"`
	RepoName              string                `json:"repo_name"`
	ReportSHA256          string                `json:"report_sha256"`
	RepositorySHA256      string                `json:"repository_state_sha256"`
	ComponentQuery        string                `json:"component_query"`
	ReportComponentID     string                `json:"report_component_id"`
	PlannerComponentID    string                `json:"planner_component_id"`
	ComponentName         string                `json:"component_name"`
	AnchorPath            string                `json:"anchor_path"`
	ReportAnchorID        string                `json:"report_anchor_id"`
	Goal                  string                `json:"goal"`
	PlannerGoalID         string                `json:"planner_goal_id"`
	Budget                componentstudy.Budget `json:"budget"`
	ProviderModel         string                `json:"provider_model"`
	PlannerPromptVersion  string                `json:"planner_prompt_version"`
	ResponseFile          string                `json:"response_file,omitempty"`
	ResponsePromptVersion string                `json:"response_prompt_version,omitempty"`
	ProviderRequestCount  int                   `json:"provider_request_count"`
}

type metricsArtifact struct {
	Version                    int              `json:"version"`
	DurationsMillis            map[string]int64 `json:"durations_ms"`
	Counts                     map[string]int   `json:"counts"`
	SeedJSONBytes              int              `json:"seed_json_bytes"`
	BundleJSONBytes            int              `json:"bundle_json_bytes"`
	EstimatedPlannerModelBytes int              `json:"estimated_planner_model_bytes"`
	ExternalRequestBytes       int              `json:"external_request_bytes"`
	ProviderResponseBytes      int              `json:"provider_response_bytes"`
	ProviderRequestCount       int              `json:"provider_request_count"`
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
	if err := clearPlannerOutcome(cfg.outDir, cfg.responseFile); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	totalStarted := time.Now()

	run, err := loadAuthorizedRun(cfg)
	if err != nil {
		return err
	}
	freshnessStarted := time.Now()
	currentRepository, err := freshness.CaptureRepository(ctx, run.manifest.RepositoryState.Identity)
	if err != nil {
		return fmt.Errorf("componentstudy-playground: capture current repository state: %w", err)
	}
	if err := run.manifest.VerifyRepositoryState(currentRepository); err != nil {
		return fmt.Errorf("componentstudy-playground: reconcile saved run: %w", err)
	}
	freshnessMillis := time.Since(freshnessStarted).Milliseconds()

	terms := rankTerms(run.component.Name, cfg.goal)
	packageStarted := time.Now()
	packageScope, packageFiles, err := collectPackageScope(ctx, run, terms)
	if err != nil {
		return err
	}
	packageMillis := time.Since(packageStarted).Milliseconds()

	symbolStarted := time.Now()
	resolver := goplsanalyzer.New(goplsanalyzer.Options{CommandTimeout: 30 * time.Second})
	symbolCatalog, symbols, err := collectSymbolCatalog(ctx, resolver, run, packageFiles, terms)
	if err != nil {
		return err
	}
	symbolMillis := time.Since(symbolStarted).Milliseconds()

	seed, budget := buildSeed(cfg, run, packageScope, packageFiles, symbols)
	bundle, trace, err := componentstudy.Build(seed, budget)
	if err != nil {
		return fmt.Errorf("componentstudy-playground: build planner bundle: %w", err)
	}

	seedJSON, err := json.Marshal(seed)
	if err != nil {
		return fmt.Errorf("componentstudy-playground: measure seed: %w", err)
	}
	bundleJSON, err := json.Marshal(bundle)
	if err != nil {
		return fmt.Errorf("componentstudy-playground: measure bundle: %w", err)
	}
	planner, err := preparePlanner(cfg.live, bundleJSON)
	if err != nil {
		return err
	}
	mode := "preview"
	requestCount := 0
	if cfg.live {
		mode = "live"
		requestCount = 1
	} else if cfg.responseFile != "" {
		mode = "replay"
	}
	caseData := caseArtifact{
		Version:               artifactVersion,
		Mode:                  mode,
		RunDir:                cfg.runDir,
		AnalysisRoot:          run.analysisRoot,
		RepoName:              seed.RepoName,
		ReportSHA256:          run.manifest.ReportSHA256,
		RepositorySHA256:      run.manifest.RepositoryStateSHA256,
		ComponentQuery:        cfg.component,
		ReportComponentID:     run.component.ID,
		PlannerComponentID:    seed.Component.ID,
		ComponentName:         seed.Component.Name,
		AnchorPath:            run.anchor.Path,
		ReportAnchorID:        run.anchor.ID,
		Goal:                  seed.Goal.Objective,
		PlannerGoalID:         seed.Goal.ID,
		Budget:                budget,
		ProviderModel:         planner.Client.Model,
		PlannerPromptVersion:  deepseek.ComponentPlanPromptVersionJSON,
		ResponseFile:          cfg.responseFile,
		ResponsePromptVersion: cfg.responsePromptVersion,
		ProviderRequestCount:  requestCount,
	}
	metrics := metricsArtifact{
		Version: artifactVersion,
		DurationsMillis: map[string]int64{
			"repository_freshness": freshnessMillis,
			"go_list":              packageMillis,
			"gopls_symbols":        symbolMillis,
			"total":                time.Since(totalStarted).Milliseconds(),
		},
		Counts: map[string]int{
			"manifest_openable_paths":  len(run.manifest.OpenablePaths),
			"repository_dirty_files":   len(currentRepository.Dirty),
			"component_anchors":        len(run.componentAuthority.Anchors),
			"package_files_discovered": len(packageScope.Files),
			"package_files_included":   len(packageFiles),
			"symbol_files_queried":     len(symbolCatalog.Files),
			"symbol_candidates":        len(symbols),
			"seed_evidence":            len(seed.Evidence),
			"bundle_files":             len(bundle.Files),
			"bundle_symbols":           len(bundle.Symbols),
			"bundle_evidence":          len(bundle.Evidence),
		},
		SeedJSONBytes:              len(seedJSON),
		BundleJSONBytes:            len(bundleJSON),
		EstimatedPlannerModelBytes: trace.EstimatedModelBytes,
		ExternalRequestBytes:       len(planner.RequestJSON),
		ProviderRequestCount:       requestCount,
	}

	artifacts := []struct {
		name  string
		value any
	}{
		{name: "case.json", value: caseData},
		{name: "seed.json", value: seed},
		{name: "package_scope.json", value: packageScope},
		{name: "symbol_catalog.json", value: symbolCatalog},
		{name: "selection_trace.json", value: trace},
		{name: filepath.Join("planner", "bundle.json"), value: bundle},
		{name: filepath.Join("planner", "request.redacted.json"), value: json.RawMessage(planner.RequestJSON)},
		{name: "metrics.json", value: metrics},
	}
	for _, artifact := range artifacts {
		if err := writeJSONArtifact(cfg.outDir, artifact.name, artifact.value); err != nil {
			return err
		}
	}

	if !cfg.live && cfg.responseFile == "" {
		fmt.Fprintf(stdout, "%s\n", filepath.Join(cfg.outDir, "planner", "bundle.json"))
		fmt.Fprintf(stderr,
			"componentstudy-playground: previewed %s / %s with %d package files, %d symbols, %d-byte bundle; no model request\n",
			run.component.Name,
			run.anchor.Path,
			len(packageFiles),
			len(symbols),
			len(bundleJSON),
		)
		return nil
	}

	var planErr error
	if cfg.responseFile != "" {
		raw, err := readPlannerResponse(cfg.responseFile)
		if err != nil {
			return err
		}
		planner, planErr = replayPlanner(planner, bundle, raw)
		metrics.DurationsMillis["replay_parse"] = planner.DurationMillis
	} else {
		planner, planErr = executePlanner(ctx, planner, bundle)
		metrics.DurationsMillis["provider"] = planner.DurationMillis
	}
	metrics.DurationsMillis["total"] = time.Since(totalStarted).Milliseconds()
	metrics.ProviderResponseBytes = len(planner.RawResponse)
	if len(planner.RawResponse) > 0 {
		if err := writeArtifact(cfg.outDir, filepath.Join("planner", "response.raw.txt"), planner.RawResponse); err != nil {
			return err
		}
	}
	if planErr != nil {
		_ = writeJSONArtifact(cfg.outDir, filepath.Join("planner", "error.json"), map[string]string{"error": planErr.Error()})
		_ = writeJSONArtifact(cfg.outDir, "metrics.json", metrics)
		return planErr
	}
	if err := writeJSONArtifact(cfg.outDir, filepath.Join("planner", "plan.json"), planner.Result.Plan); err != nil {
		return err
	}
	if err := writeJSONArtifact(cfg.outDir, filepath.Join("planner", "parse_warnings.json"), planner.Result.Diagnostics); err != nil {
		return err
	}
	if err := writeJSONArtifact(cfg.outDir, "metrics.json", metrics); err != nil {
		return err
	}

	fmt.Fprintf(stdout, "%s\n", filepath.Join(cfg.outDir, "planner", "plan.json"))
	fmt.Fprintf(stderr,
		"componentstudy-playground: %s %s / %s with %d selected files, %d selected symbols, %d questions in %d ms\n",
		mode,
		run.component.Name,
		run.anchor.Path,
		len(planner.Result.Plan.SelectedFiles),
		len(planner.Result.Plan.SelectedSymbols),
		len(planner.Result.Plan.Questions),
		planner.DurationMillis,
	)
	return nil
}

func parseConfig(args []string, stderr io.Writer) (config, error) {
	var cfg config
	flags := flag.NewFlagSet("componentstudy-playground", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&cfg.runDir, "run-dir", "", "verified repomap v2 run directory")
	flags.StringVar(&cfg.component, "component", "", "component id or unique component name")
	flags.StringVar(&cfg.anchor, "anchor", "", "repository-relative component anchor path")
	flags.StringVar(&cfg.goal, "goal", "", "one onboarding question for this component")
	flags.StringVar(&cfg.outDir, "out-dir", "", "directory for local preview artifacts")
	flags.BoolVar(&cfg.live, "live", false, "make one configured component-planning model request")
	flags.StringVar(&cfg.responseFile, "response-file", "", "replay one saved raw planner response without a model call")
	flags.StringVar(&cfg.responsePromptVersion, "response-prompt-version", "", "prompt version that produced --response-file")
	if err := flags.Parse(args); err != nil {
		return config{}, err
	}
	if flags.NArg() != 0 {
		return config{}, fmt.Errorf("componentstudy-playground: unexpected positional arguments")
	}
	if cfg.live && cfg.responseFile != "" {
		return config{}, fmt.Errorf("componentstudy-playground: --live and --response-file are mutually exclusive")
	}
	if cfg.responseFile == "" && cfg.responsePromptVersion != "" {
		return config{}, fmt.Errorf("componentstudy-playground: --response-prompt-version requires --response-file")
	}
	if cfg.runDir == "" || cfg.component == "" || cfg.anchor == "" || cfg.goal == "" || cfg.outDir == "" {
		return config{}, fmt.Errorf("componentstudy-playground: --run-dir, --component, --anchor, --goal, and --out-dir are required")
	}
	absRunDir, err := filepath.Abs(cfg.runDir)
	if err != nil {
		return config{}, fmt.Errorf("componentstudy-playground: resolve run directory: %w", err)
	}
	absOutDir, err := filepath.Abs(cfg.outDir)
	if err != nil {
		return config{}, fmt.Errorf("componentstudy-playground: resolve output directory: %w", err)
	}
	cfg.runDir = filepath.Clean(absRunDir)
	cfg.outDir = filepath.Clean(absOutDir)
	if cfg.responseFile != "" {
		absResponseFile, err := filepath.Abs(cfg.responseFile)
		if err != nil {
			return config{}, fmt.Errorf("componentstudy-playground: resolve response file: %w", err)
		}
		cfg.responseFile = filepath.Clean(absResponseFile)
		cfg.responsePromptVersion = cleanText(cfg.responsePromptVersion, 128)
		if cfg.responsePromptVersion == "" {
			cfg.responsePromptVersion = "unknown"
		}
	}
	cfg.component = strings.TrimSpace(cfg.component)
	if cfg.component == "" {
		return config{}, fmt.Errorf("componentstudy-playground: --component must contain visible text")
	}
	cfg.goal = cleanText(cfg.goal, 1024)
	if cfg.goal == "" {
		return config{}, fmt.Errorf("componentstudy-playground: --goal must contain visible text")
	}
	cfg.anchor, err = cleanRepoPath(cfg.anchor)
	if err != nil {
		return config{}, fmt.Errorf("componentstudy-playground: --anchor: %w", err)
	}
	return cfg, nil
}

func loadAuthorizedRun(cfg config) (authorizedRun, error) {
	manifest, err := report.ReadRunManifest(cfg.runDir)
	if err != nil {
		return authorizedRun{}, fmt.Errorf("componentstudy-playground: verify run: %w", err)
	}
	analysisRoot, err := manifest.ResolveAnalysisRoot()
	if err != nil {
		return authorizedRun{}, fmt.Errorf("componentstudy-playground: resolve analysis root: %w", err)
	}
	reportJSON, err := os.ReadFile(filepath.Join(cfg.runDir, "report.json"))
	if err != nil {
		return authorizedRun{}, fmt.Errorf("componentstudy-playground: read report: %w", err)
	}
	if err := manifest.VerifyReportJSON(reportJSON); err != nil {
		return authorizedRun{}, fmt.Errorf("componentstudy-playground: reverify report: %w", err)
	}
	var data report.ReportData
	if err := json.Unmarshal(reportJSON, &data); err != nil {
		return authorizedRun{}, fmt.Errorf("componentstudy-playground: decode report: %w", err)
	}

	component, err := chooseComponent(data.Components, cfg.component)
	if err != nil {
		return authorizedRun{}, err
	}
	componentAuthority, ok := findComponentAuthority(manifest.Components, component.ID)
	if !ok {
		return authorizedRun{}, fmt.Errorf("componentstudy-playground: component %q is absent from run authority", component.Name)
	}
	anchor, ok := findAnchor(component.AnchorGroups, cfg.anchor)
	if !ok {
		return authorizedRun{}, fmt.Errorf("componentstudy-playground: %q is not an anchor of component %q", cfg.anchor, component.Name)
	}
	anchorAuthority, ok := findAnchorAuthority(componentAuthority.Anchors, anchor.ID, cfg.anchor)
	if !ok {
		return authorizedRun{}, fmt.Errorf("componentstudy-playground: anchor %q is absent from run authority", cfg.anchor)
	}
	if !anchorAuthority.CanListSymbols {
		return authorizedRun{}, fmt.Errorf("componentstudy-playground: anchor %q is not authorized for symbol listing", cfg.anchor)
	}
	openable := make(map[string]struct{}, len(manifest.OpenablePaths))
	for _, path := range manifest.OpenablePaths {
		openable[path] = struct{}{}
	}
	if _, ok := openable[cfg.anchor]; !ok {
		return authorizedRun{}, fmt.Errorf("componentstudy-playground: anchor %q is not openable", cfg.anchor)
	}
	info, err := os.Stat(filepath.Join(analysisRoot, filepath.FromSlash(cfg.anchor)))
	if err != nil {
		return authorizedRun{}, fmt.Errorf("componentstudy-playground: inspect anchor: %w", err)
	}
	if !info.Mode().IsRegular() {
		return authorizedRun{}, fmt.Errorf("componentstudy-playground: anchor %q is not a regular file", cfg.anchor)
	}

	return authorizedRun{
		manifest:           manifest,
		data:               data,
		analysisRoot:       analysisRoot,
		component:          component,
		componentAuthority: componentAuthority,
		anchor:             anchor,
		anchorAuthority:    anchorAuthority,
		openable:           openable,
	}, nil
}

func chooseComponent(components []report.Component, query string) (report.Component, error) {
	query = strings.TrimSpace(query)
	for _, component := range components {
		if component.ID == query || component.Name == query {
			return component, nil
		}
	}
	lowerQuery := strings.ToLower(query)
	var matches []report.Component
	for _, component := range components {
		if strings.EqualFold(component.Name, query) || strings.Contains(strings.ToLower(component.Name), lowerQuery) {
			matches = append(matches, component)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		names := make([]string, 0, len(matches))
		for _, component := range matches {
			names = append(names, component.Name)
		}
		sort.Strings(names)
		return report.Component{}, fmt.Errorf("componentstudy-playground: component query %q is ambiguous: %s", query, strings.Join(names, ", "))
	}
	available := make([]string, 0, len(components))
	for _, component := range components {
		available = append(available, component.Name)
	}
	sort.Strings(available)
	return report.Component{}, fmt.Errorf("componentstudy-playground: component %q not found; available: %s", query, strings.Join(available, ", "))
}

func findComponentAuthority(components []report.ComponentAuthority, id string) (report.ComponentAuthority, bool) {
	for _, component := range components {
		if component.ID == id {
			return component, true
		}
	}
	return report.ComponentAuthority{}, false
}

func findAnchor(anchors []report.AnchorGroup, path string) (report.AnchorGroup, bool) {
	for _, anchor := range anchors {
		if anchor.Path == path {
			return anchor, true
		}
	}
	return report.AnchorGroup{}, false
}

func findAnchorAuthority(anchors []report.AnchorAuthority, id, path string) (report.AnchorAuthority, bool) {
	for _, anchor := range anchors {
		if anchor.ID == id && anchor.Path == path {
			return anchor, true
		}
	}
	return report.AnchorAuthority{}, false
}

func writeJSONArtifact(root, name string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("componentstudy-playground: marshal %s: %w", name, err)
	}
	return writeArtifact(root, name, append(data, '\n'))
}

func clearPlannerOutcome(root, preserve string) error {
	for _, name := range []string{
		"error.json",
		"parse_warnings.json",
		"plan.json",
		"response.raw.txt",
	} {
		path := filepath.Join(root, "planner", name)
		if preserve != "" && filepath.Clean(path) == filepath.Clean(preserve) {
			continue
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("componentstudy-playground: remove stale %s: %w", name, err)
		}
	}
	return nil
}

func readPlannerResponse(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("componentstudy-playground: read replay response: %w", err)
	}
	const maxReplayResponseBytes = 4 * 1024 * 1024
	if len(data) == 0 || len(data) > maxReplayResponseBytes {
		return nil, fmt.Errorf("componentstudy-playground: replay response must be between 1 and %d bytes", maxReplayResponseBytes)
	}
	return data, nil
}

func writeArtifact(root, name string, data []byte) error {
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("componentstudy-playground: create artifact directory: %w", err)
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, data, 0o600); err != nil {
		return fmt.Errorf("componentstudy-playground: write %s: %w", name, err)
	}
	if err := os.Rename(temporary, path); err != nil {
		return fmt.Errorf("componentstudy-playground: publish %s: %w", name, err)
	}
	return nil
}
