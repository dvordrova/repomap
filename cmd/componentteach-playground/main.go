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
	"syscall"
	"time"

	"github.com/dvordrova/repomap/internal/componentprobe"
	"github.com/dvordrova/repomap/internal/componentteach"
	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/secretscan"
)

const (
	artifactVersion     = 1
	maxProbeBundleBytes = 4 * 1024 * 1024
	maxResponseBytes    = 4 * 1024 * 1024
)

type config struct {
	runDir                string
	round1Path            string
	round2Path            string
	outDir                string
	live                  bool
	responseFile          string
	responsePromptVersion string
}

type caseArtifact struct {
	Version               int                   `json:"version"`
	Mode                  string                `json:"mode"`
	RunDir                string                `json:"run_dir"`
	AnalysisRoot          string                `json:"analysis_root"`
	ReportSHA256          string                `json:"report_sha256"`
	RepositorySHA256      string                `json:"repository_state_sha256"`
	Round1Path            string                `json:"round_1_path"`
	Round1SHA256          string                `json:"round_1_sha256"`
	Round2Path            string                `json:"round_2_path,omitempty"`
	Round2SHA256          string                `json:"round_2_sha256,omitempty"`
	GoalID                string                `json:"goal_id"`
	ComponentID           string                `json:"component_id"`
	ComponentName         string                `json:"component_name"`
	PrimaryQuestionID     string                `json:"primary_question_id"`
	Budget                componentteach.Budget `json:"budget"`
	ProviderModel         string                `json:"provider_model"`
	TeacherPromptVersion  string                `json:"teacher_prompt_version"`
	ResponseFile          string                `json:"response_file,omitempty"`
	ResponsePromptVersion string                `json:"response_prompt_version,omitempty"`
	ProviderRequestCount  int                   `json:"provider_request_count"`
}

type metricsArtifact struct {
	Version                 int              `json:"version"`
	DurationsMillis         map[string]int64 `json:"durations_ms"`
	Counts                  map[string]int   `json:"counts"`
	Round1JSONBytes         int              `json:"round_1_json_bytes"`
	Round2JSONBytes         int              `json:"round_2_json_bytes"`
	TeacherBundleJSONBytes  int              `json:"teacher_bundle_json_bytes"`
	TeacherIndexJSONBytes   int              `json:"teacher_index_json_bytes"`
	SelectionTraceJSONBytes int              `json:"selection_trace_json_bytes"`
	ExternalRequestBytes    int              `json:"external_request_bytes"`
	ProviderResponseBytes   int              `json:"provider_response_bytes"`
	ProviderRequestCount    int              `json:"provider_request_count"`
}

type teacherExecution struct {
	Client         *deepseek.Client
	RequestJSON    []byte
	RawResponse    []byte
	Result         componentteach.ParseResult
	DurationMillis int64
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
	if err := clearOutcomes(cfg); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	totalStarted := time.Now()

	manifest, analysisRoot, dirtyCount, freshnessMillis, err := loadAuthorizedRun(ctx, cfg.runDir)
	if err != nil {
		return err
	}
	loadStarted := time.Now()
	var round1 componentprobe.Bundle
	round1JSON, err := readJSONArtifact(cfg.round1Path, maxProbeBundleBytes, &round1)
	if err != nil {
		return fmt.Errorf("componentteach-playground: read round 1: %w", err)
	}
	if err := round1.Validate(); err != nil || round1.Round != componentprobe.RoundInitial {
		if err == nil {
			err = fmt.Errorf("expected round 1")
		}
		return fmt.Errorf("componentteach-playground: validate round 1: %w", err)
	}
	round1Digest, err := componentprobe.SHA256(round1)
	if err != nil {
		return fmt.Errorf("componentteach-playground: digest round 1: %w", err)
	}
	var round2 *componentprobe.Bundle
	var round2JSON []byte
	var round2Digest string
	if cfg.round2Path != "" {
		var value componentprobe.Bundle
		round2JSON, err = readJSONArtifact(cfg.round2Path, maxProbeBundleBytes, &value)
		if err != nil {
			return fmt.Errorf("componentteach-playground: read round 2: %w", err)
		}
		if err := value.ValidateAgainst(round1); err != nil {
			return fmt.Errorf("componentteach-playground: validate round 2: %w", err)
		}
		round2Digest, err = componentprobe.SHA256(value)
		if err != nil {
			return fmt.Errorf("componentteach-playground: digest round 2: %w", err)
		}
		round2 = &value
	}
	loadMillis := time.Since(loadStarted).Milliseconds()

	buildStarted := time.Now()
	budget := componentteach.DefaultBudget()
	bundle, index, trace, err := componentteach.Build(round1, round2, budget)
	if err != nil {
		return fmt.Errorf("componentteach-playground: build teacher bundle: %w", err)
	}
	bundleJSON, err := componentteach.MarshalModelBundle(bundle)
	if err != nil {
		return err
	}
	indexJSON, err := json.Marshal(index)
	if err != nil {
		return fmt.Errorf("componentteach-playground: measure teacher index: %w", err)
	}
	traceJSON, err := json.Marshal(trace)
	if err != nil {
		return fmt.Errorf("componentteach-playground: measure selection trace: %w", err)
	}
	buildMillis := time.Since(buildStarted).Milliseconds()

	execution, err := prepareTeacher(cfg.live, bundleJSON)
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
		Version: artifactVersion, Mode: mode, RunDir: cfg.runDir, AnalysisRoot: analysisRoot,
		ReportSHA256: manifest.ReportSHA256, RepositorySHA256: manifest.RepositoryStateSHA256,
		Round1Path: cfg.round1Path, Round1SHA256: round1Digest,
		Round2Path: cfg.round2Path, Round2SHA256: round2Digest,
		GoalID: round1.Focus.Goal.ID, ComponentID: round1.Focus.Component.ID,
		ComponentName: round1.Focus.Component.Name, PrimaryQuestionID: round1.Focus.PrimaryQuestion.ID,
		Budget: budget, ProviderModel: execution.Client.Model,
		TeacherPromptVersion: deepseek.ComponentTeachPromptVersionJSON,
		ResponseFile:         cfg.responseFile, ResponsePromptVersion: cfg.responsePromptVersion,
		ProviderRequestCount: requestCount,
	}
	metrics := metricsArtifact{
		Version: artifactVersion,
		DurationsMillis: map[string]int64{
			"repository_freshness": freshnessMillis,
			"load_probe_chain":     loadMillis,
			"build_teacher_bundle": buildMillis,
		},
		Counts: map[string]int{
			"repository_dirty_files": dirtyCount,
			"probe_rounds":           1,
			"teacher_evidence":       len(bundle.Evidence),
			"teacher_locators":       len(index.Entries),
			"unresolved_frontiers":   len(bundle.UnresolvedFrontierIDs),
			"selection_decisions":    len(trace.Decisions),
		},
		Round1JSONBytes: len(round1JSON), Round2JSONBytes: len(round2JSON),
		TeacherBundleJSONBytes: len(bundleJSON), TeacherIndexJSONBytes: len(indexJSON),
		SelectionTraceJSONBytes: len(traceJSON), ExternalRequestBytes: len(execution.RequestJSON),
		ProviderRequestCount: requestCount,
	}
	if round2 != nil {
		metrics.Counts["probe_rounds"] = 2
	}
	for _, artifact := range []struct {
		name  string
		value any
	}{
		{"case.json", caseData},
		{filepath.Join("teacher", "bundle.json"), bundle},
		{filepath.Join("teacher", "index.json"), index},
		{filepath.Join("teacher", "selection_trace.json"), trace},
		{filepath.Join("teacher", "request.redacted.json"), json.RawMessage(execution.RequestJSON)},
		{"metrics.json", metrics},
	} {
		if err := writeJSONArtifact(cfg.outDir, artifact.name, artifact.value); err != nil {
			return err
		}
	}

	if !cfg.live && cfg.responseFile == "" {
		metrics.DurationsMillis["total"] = time.Since(totalStarted).Milliseconds()
		_ = writeJSONArtifact(cfg.outDir, "metrics.json", metrics)
		fmt.Fprintln(stdout, filepath.Join(cfg.outDir, "teacher", "bundle.json"))
		fmt.Fprintf(stderr, "componentteach-playground: previewed %d evidence items in a %d-byte model bundle; no model request\n", len(bundle.Evidence), len(bundleJSON))
		return nil
	}

	var teachErr error
	if cfg.responseFile != "" {
		raw, err := readRawResponse(cfg.responseFile)
		if err != nil {
			return err
		}
		execution, teachErr = replayTeacher(execution, bundle, raw)
		metrics.DurationsMillis["replay_parse"] = execution.DurationMillis
	} else {
		execution, teachErr = executeTeacher(ctx, execution, bundle)
		metrics.DurationsMillis["provider"] = execution.DurationMillis
	}
	metrics.DurationsMillis["total"] = time.Since(totalStarted).Milliseconds()
	metrics.ProviderResponseBytes = len(execution.RawResponse)
	if len(execution.RawResponse) > 0 {
		if kind, found := secretscan.Detect(string(execution.RawResponse)); found {
			teachErr = errors.Join(teachErr, fmt.Errorf("componentteach-playground: refusing to store provider response: %s detected", kind))
		} else if err := writeArtifact(cfg.outDir, filepath.Join("teacher", "response.raw.txt"), execution.RawResponse); err != nil {
			return err
		}
	}
	if teachErr != nil {
		_ = writeJSONArtifact(cfg.outDir, filepath.Join("teacher", "error.json"), map[string]string{"error": teachErr.Error()})
		_ = writeJSONArtifact(cfg.outDir, "metrics.json", metrics)
		return teachErr
	}
	if err := writeJSONArtifact(cfg.outDir, filepath.Join("teacher", "report.json"), execution.Result.Report); err != nil {
		return err
	}
	if err := writeJSONArtifact(cfg.outDir, filepath.Join("teacher", "parse_warnings.json"), execution.Result.Diagnostics); err != nil {
		return err
	}
	if err := writeJSONArtifact(cfg.outDir, "metrics.json", metrics); err != nil {
		return err
	}
	fmt.Fprintln(stdout, filepath.Join(cfg.outDir, "teacher", "report.json"))
	fmt.Fprintf(stderr, "componentteach-playground: %s grounded report with %d items in %d ms\n", mode, reportItemCount(execution.Result.Report), execution.DurationMillis)
	return nil
}

func prepareTeacher(live bool, bundleJSON []byte) (teacherExecution, error) {
	var client *deepseek.Client
	var err error
	if live {
		client, err = deepseek.NewFromEnv()
	} else {
		client, err = deepseek.NewPromptFromEnv()
	}
	if err != nil {
		return teacherExecution{}, fmt.Errorf("componentteach-playground: configure teacher: %w", err)
	}
	requestJSON, err := client.TeacherPromptJSON(bundleJSON)
	if err != nil {
		return teacherExecution{}, fmt.Errorf("componentteach-playground: build teacher request: %w", err)
	}
	return teacherExecution{Client: client, RequestJSON: requestJSON}, nil
}

func executeTeacher(ctx context.Context, execution teacherExecution, bundle componentteach.Bundle) (teacherExecution, error) {
	recorder := &recordingTeacher{inner: deepseek.NewComponentTeacher(execution.Client)}
	started := time.Now()
	result, err := componentteach.NewService(recorder).Teach(ctx, bundle)
	execution.DurationMillis = time.Since(started).Milliseconds()
	execution.RawResponse = recorder.raw
	if err != nil {
		return execution, fmt.Errorf("componentteach-playground: teach component: %w", err)
	}
	execution.Result = result
	return execution, nil
}

func replayTeacher(execution teacherExecution, bundle componentteach.Bundle, raw []byte) (teacherExecution, error) {
	started := time.Now()
	result, err := componentteach.ParseReport(bundle, raw)
	execution.DurationMillis = time.Since(started).Milliseconds()
	execution.RawResponse = append([]byte(nil), raw...)
	if err != nil {
		return execution, fmt.Errorf("componentteach-playground: replay teacher: %w", err)
	}
	execution.Result = result
	return execution, nil
}

type recordingTeacher struct {
	inner componentteach.Teacher
	raw   []byte
}

func (r *recordingTeacher) Teach(ctx context.Context, bundleJSON []byte) ([]byte, error) {
	raw, err := r.inner.Teach(ctx, bundleJSON)
	r.raw = append(r.raw[:0], raw...)
	return raw, err
}

func parseConfig(args []string, stderr io.Writer) (config, error) {
	var cfg config
	flags := flag.NewFlagSet("componentteach-playground", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&cfg.runDir, "run-dir", "", "verified repomap v2 run directory")
	flags.StringVar(&cfg.round1Path, "probe-round1", "", "validated round-1 component probe bundle")
	flags.StringVar(&cfg.round2Path, "probe-round2", "", "optional validated round-2 component probe bundle")
	flags.StringVar(&cfg.outDir, "out-dir", "", "directory for local teacher artifacts")
	flags.BoolVar(&cfg.live, "live", false, "make one configured component-teacher model request")
	flags.StringVar(&cfg.responseFile, "response-file", "", "replay a saved raw teacher response without a model call")
	flags.StringVar(&cfg.responsePromptVersion, "response-prompt-version", "", "prompt version that produced --response-file")
	if err := flags.Parse(args); err != nil {
		return config{}, err
	}
	if flags.NArg() != 0 {
		return config{}, fmt.Errorf("componentteach-playground: unexpected positional arguments")
	}
	if cfg.live && cfg.responseFile != "" {
		return config{}, fmt.Errorf("componentteach-playground: --live and --response-file are mutually exclusive")
	}
	if cfg.responseFile == "" && cfg.responsePromptVersion != "" {
		return config{}, fmt.Errorf("componentteach-playground: --response-prompt-version requires --response-file")
	}
	if cfg.runDir == "" || cfg.round1Path == "" || cfg.outDir == "" {
		return config{}, fmt.Errorf("componentteach-playground: --run-dir, --probe-round1, and --out-dir are required")
	}
	for _, value := range []*string{&cfg.runDir, &cfg.round1Path, &cfg.round2Path, &cfg.outDir, &cfg.responseFile} {
		if *value == "" {
			continue
		}
		absolute, err := filepath.Abs(*value)
		if err != nil {
			return config{}, fmt.Errorf("componentteach-playground: resolve path: %w", err)
		}
		*value = filepath.Clean(absolute)
	}
	if cfg.responseFile != "" && cfg.responsePromptVersion == "" {
		cfg.responsePromptVersion = "unknown"
	}
	return cfg, nil
}

func loadAuthorizedRun(ctx context.Context, runDir string) (report.RunManifest, string, int, int64, error) {
	manifest, err := report.ReadRunManifest(runDir)
	if err != nil {
		return report.RunManifest{}, "", 0, 0, fmt.Errorf("componentteach-playground: verify run: %w", err)
	}
	analysisRoot, err := manifest.ResolveAnalysisRoot()
	if err != nil {
		return report.RunManifest{}, "", 0, 0, fmt.Errorf("componentteach-playground: resolve analysis root: %w", err)
	}
	started := time.Now()
	current, err := freshness.CaptureRepository(ctx, manifest.RepositoryState.Identity)
	if err != nil {
		return report.RunManifest{}, "", 0, 0, fmt.Errorf("componentteach-playground: capture repository state: %w", err)
	}
	if err := manifest.VerifyRepositoryState(current); err != nil {
		return report.RunManifest{}, "", 0, 0, fmt.Errorf("componentteach-playground: reconcile saved run: %w", err)
	}
	return manifest, analysisRoot, len(current.Dirty), time.Since(started).Milliseconds(), nil
}

func readJSONArtifact(path string, limit int64, target any) ([]byte, error) {
	data, err := readBoundedFile(path, limit)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("trailing or multiple json values")
	}
	return data, nil
}

func readRawResponse(path string) ([]byte, error) {
	data, err := readBoundedFile(path, maxResponseBytes)
	if err != nil {
		return nil, fmt.Errorf("componentteach-playground: read response: %w", err)
	}
	return data, nil
}

func readBoundedFile(path string, limit int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("artifact is not a regular file")
	}
	if info.Size() <= 0 || info.Size() > limit {
		return nil, fmt.Errorf("artifact size must be between 1 and %d bytes", limit)
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read bounded artifact: %w", err)
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("artifact exceeds %d bytes", limit)
	}
	return data, nil
}

func clearOutcomes(cfg config) error {
	preserve := ""
	if cfg.responseFile != "" {
		preserve = cfg.responseFile
	}
	for _, name := range []string{"response.raw.txt", "report.json", "parse_warnings.json", "error.json"} {
		path := filepath.Join(cfg.outDir, "teacher", name)
		if preserve != "" && filepath.Clean(path) == filepath.Clean(preserve) {
			continue
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("componentteach-playground: remove stale %s: %w", name, err)
		}
	}
	return nil
}

func writeJSONArtifact(root, name string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("componentteach-playground: marshal %s: %w", name, err)
	}
	return writeArtifact(root, name, append(data, '\n'))
}

func writeArtifact(root, name string, data []byte) error {
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("componentteach-playground: create artifact directory: %w", err)
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, data, 0o600); err != nil {
		return fmt.Errorf("componentteach-playground: write %s: %w", name, err)
	}
	if err := os.Rename(temporary, path); err != nil {
		return fmt.Errorf("componentteach-playground: publish %s: %w", name, err)
	}
	return nil
}

func reportItemCount(report componentteach.Report) int {
	return len(report.MentalModel) + len(report.LifecycleSteps) + len(report.Boundaries) +
		len(report.DesignNotes) + len(report.FailuresAndObservability) + len(report.TestsAndChecks) +
		len(report.Unknowns) + len(report.NextDive)
}
