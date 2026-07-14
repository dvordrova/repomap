package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	analysis "github.com/dvordrova/repomap/internal/analyzer"
	goplsanalyzer "github.com/dvordrova/repomap/internal/analyzer/golang/gopls"
	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/sourcecard"
	"github.com/dvordrova/repomap/internal/sourceexplain"
	"github.com/dvordrova/repomap/internal/symbol"
	"github.com/dvordrova/repomap/internal/testevidence"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	var (
		repoPath       = flag.String("repo", ".", "path to a Go repository")
		symbolQuery    = flag.String("symbol", "", "exact gopls workspace symbol name")
		outDir         = flag.String("out-dir", "tmp/symbol-playground", "artifact output directory")
		callDeepSeek   = flag.Bool("deepseek", false, "send the bounded symbol bundle to DeepSeek")
		buildSource    = flag.Bool("source", false, "build bounded source assessment artifacts without a model call")
		callSource     = flag.Bool("deepseek-source", false, "ask DeepSeek to assess the bounded target source")
		goplsBinary    = flag.String("gopls", "gopls", "gopls binary")
		commandTimeout = flag.Duration("command-timeout", 2*time.Minute, "timeout for each gopls command")
		maxCandidates  = flag.Int("max-candidates", 12, "maximum fuzzy candidates in the model bundle")
		maxIncoming    = flag.Int("max-incoming", 30, "maximum incoming calls in the model bundle")
		maxOutgoing    = flag.Int("max-outgoing", 30, "maximum outgoing calls in the model bundle")
		responseFormat = flag.String("format", "tagged", "DeepSeek response format: json or tagged")
	)
	flag.Parse()

	if *symbolQuery == "" {
		return fmt.Errorf("--symbol is required and must exactly match one gopls symbol name")
	}
	if *outDir == "" {
		return fmt.Errorf("--out-dir is required")
	}
	if *responseFormat != "json" && *responseFormat != "tagged" {
		return fmt.Errorf("--format must be json or tagged")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	analyzer := goplsanalyzer.New(goplsanalyzer.Options{
		Binary:         *goplsBinary,
		MaxSymbols:     max(*maxCandidates, 40),
		MaxCallRoots:   1,
		CommandTimeout: *commandTimeout,
	})
	graph, err := analyzer.Analyze(ctx, analysis.Request{RepoPath: *repoPath, Query: *symbolQuery})
	if err != nil {
		return err
	}

	bundle, err := symbol.Build(graph, symbol.Options{
		MaxCandidates:    *maxCandidates,
		MaxIncomingCalls: *maxIncoming,
		MaxOutgoingCalls: *maxOutgoing,
	})
	if err != nil {
		return err
	}
	graphJSON, err := json.MarshalIndent(graph, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal evidence graph: %w", err)
	}
	bundleJSON, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal symbol bundle: %w", err)
	}
	promptClient, err := deepseek.NewPromptFromEnv()
	if err != nil {
		return err
	}
	promptVersion := deepseek.SymbolPromptVersionJSON
	promptJSON, err := promptClient.SymbolPromptJSON(bundleJSON)
	if *responseFormat == "tagged" {
		promptVersion = deepseek.SymbolPromptVersionTagged
		promptJSON, err = promptClient.SymbolTaggedPromptJSON(bundleJSON)
	}
	if err != nil {
		return err
	}
	metadata := experimentMetadata{
		PromptVersion:  promptVersion,
		ResponseFormat: *responseFormat,
		Model:          promptClient.Model,
		BundleBytes:    len(bundleJSON),
		RequestBytes:   len(promptJSON),
	}

	if err := os.MkdirAll(*outDir, 0o700); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	artifacts := []artifact{
		{name: "evidence_graph.json", data: graphJSON},
		{name: "symbol_bundle.json", data: bundleJSON},
		{name: "deepseek_request.redacted.json", data: promptJSON},
	}
	var sourceWorkflow preparedSource
	if *buildSource || *callSource {
		sourceWorkflow, err = prepareSource(graph.RepoPath, bundle, promptClient)
		if err != nil {
			return err
		}
		metadata.SourcePromptVersion = deepseek.SourcePromptVersionJSON
		metadata.SourceBundleBytes = len(sourceWorkflow.bundleJSON)
		metadata.SourceRequestBytes = len(sourceWorkflow.promptJSON)
		artifacts = append(artifacts,
			artifact{name: "source_card.json", data: sourceWorkflow.cardJSON},
			artifact{name: "source_assessment_bundle.json", data: sourceWorkflow.bundleJSON},
			artifact{name: "deepseek_source_request.redacted.json", data: sourceWorkflow.promptJSON},
		)
	}
	for _, artifact := range artifacts {
		if err := writeArtifact(*outDir, artifact.name, artifact.data); err != nil {
			return err
		}
	}
	if err := writeExperimentMetadata(*outDir, metadata); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "resolved %s at %s:%d\n",
		bundle.Target.Entity.Name,
		bundle.Target.Entity.Location.Path,
		bundle.Target.Entity.Location.Line,
	)
	fmt.Fprintf(os.Stderr, "bundle: %d bytes, prompt request: %d bytes, incoming calls: %d, outgoing calls: %d\n",
		len(bundleJSON), len(promptJSON), len(bundle.IncomingCalls), len(bundle.OutgoingCalls))
	if *buildSource || *callSource {
		fmt.Fprintf(os.Stderr, "source card: %s:%d-%d, %d bytes, stop=%s\n",
			sourceWorkflow.card.Target.Path,
			sourceWorkflow.card.Window.StartLine,
			sourceWorkflow.card.Window.EndLine,
			sourceWorkflow.card.Window.IncludedBytes,
			sourceWorkflow.card.Window.StopReason,
		)
		fmt.Fprintf(os.Stderr, "source assessment: %d questions, %d-byte bundle, %d-byte prompt request\n",
			len(sourceWorkflow.bundle.Questions), len(sourceWorkflow.bundleJSON), len(sourceWorkflow.promptJSON))
	}
	fmt.Fprintf(os.Stderr, "wrote prompt-only artifacts to %s\n", *outDir)

	if !*callDeepSeek && !*callSource {
		return nil
	}

	client, err := deepseek.NewFromEnv()
	if err != nil {
		return err
	}
	if *callDeepSeek {
		if err := runSymbolExplanation(ctx, client, bundle, bundleJSON, *responseFormat, *outDir); err != nil {
			return err
		}
	}
	if *callSource {
		explainer := sourceexplain.NewService(client)
		sourceStarted := time.Now()
		explanation, err := runSourceExplanation(ctx, explainer, sourceWorkflow.bundle, *outDir)
		metadata.recordSourceTiming(sourceStarted, time.Now())
		if metadataErr := writeExperimentMetadata(*outDir, metadata); metadataErr != nil {
			return metadataErr
		}
		if err != nil {
			return err
		}
		if explanation.Parsed.Report.NextAction.Operation == sourceexplain.OperationFindTests {
			testBundle, err := testevidence.Collect(
				ctx,
				analyzer,
				graph.RepoPath,
				bundle,
				sourceWorkflow.bundle,
				explanation.Parsed.Report,
				testevidence.Options{},
			)
			if err != nil {
				return fmt.Errorf("execute find_tests action: %w", err)
			}
			testBundleJSON, err := json.MarshalIndent(testBundle, "", "  ")
			if err != nil {
				return fmt.Errorf("marshal test evidence: %w", err)
			}
			if err := writeArtifact(*outDir, "test_evidence.json", testBundleJSON); err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "find_tests: %d searches, %d test references, %d warnings\n",
				len(testBundle.Searches), len(testBundle.References), len(testBundle.Warnings))
			fmt.Fprintf(os.Stderr, "wrote %s\n", filepath.Join(*outDir, "test_evidence.json"))
		}
	}
	return nil
}

type preparedSource struct {
	card       sourcecard.Card
	bundle     sourceexplain.Bundle
	cardJSON   []byte
	bundleJSON []byte
	promptJSON []byte
}

type artifact struct {
	name string
	data []byte
}

type experimentMetadata struct {
	PromptVersion       string `json:"prompt_version"`
	SourcePromptVersion string `json:"source_prompt_version,omitempty"`
	ResponseFormat      string `json:"response_format"`
	Model               string `json:"model"`
	BundleBytes         int    `json:"bundle_bytes"`
	RequestBytes        int    `json:"request_bytes"`
	SourceBundleBytes   int    `json:"source_bundle_bytes,omitempty"`
	SourceRequestBytes  int    `json:"source_request_bytes,omitempty"`
	SourceCapturedAt    string `json:"source_captured_at,omitempty"`
	SourceLatencyMillis *int64 `json:"source_latency_ms,omitempty"`
}

func writeExperimentMetadata(outDir string, metadata experimentMetadata) error {
	experimentJSON, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal prompt experiment metadata: %w", err)
	}
	return writeArtifact(outDir, "prompt_experiment.json", experimentJSON)
}

func (m *experimentMetadata) recordSourceTiming(started, finished time.Time) {
	latency := finished.Sub(started).Milliseconds()
	if latency < 0 {
		latency = 0
	}
	m.SourceCapturedAt = started.UTC().Format(time.RFC3339)
	m.SourceLatencyMillis = &latency
}

type sourceExplainer interface {
	Explain(ctx context.Context, bundle sourceexplain.Bundle) (sourceexplain.Explanation, error)
}

func prepareSource(repoPath string, structural symbol.Bundle, promptClient *deepseek.Client) (preparedSource, error) {
	card, err := sourcecard.Read(sourcecard.Request{
		RepoPath:         repoPath,
		TargetEvidenceID: structural.Target.EvidenceID,
		Target:           structural.Target.Entity,
	}, sourcecard.Limits{})
	if err != nil {
		return preparedSource{}, fmt.Errorf("collect target source card: %w", err)
	}
	if err := sourcecard.ValidateForRemote(card); err != nil {
		return preparedSource{}, fmt.Errorf("validate target source card: %w", err)
	}
	bundle, err := sourceexplain.Build(structural, card)
	if err != nil {
		return preparedSource{}, fmt.Errorf("build source assessment bundle: %w", err)
	}
	cardJSON, err := json.MarshalIndent(card, "", "  ")
	if err != nil {
		return preparedSource{}, fmt.Errorf("marshal source card: %w", err)
	}
	bundleJSON, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return preparedSource{}, fmt.Errorf("marshal source assessment bundle: %w", err)
	}
	promptJSON, err := promptClient.SourcePromptJSON(bundleJSON)
	if err != nil {
		return preparedSource{}, err
	}
	return preparedSource{
		card:       card,
		bundle:     bundle,
		cardJSON:   cardJSON,
		bundleJSON: bundleJSON,
		promptJSON: promptJSON,
	}, nil
}

func runSymbolExplanation(ctx context.Context, client *deepseek.Client, bundle symbol.Bundle, bundleJSON []byte, responseFormat, outDir string) error {
	var raw []byte
	var err error
	if responseFormat == "tagged" {
		raw, err = client.ExplainSymbolTagged(ctx, bundleJSON)
	} else {
		raw, err = client.ExplainSymbol(ctx, bundleJSON)
	}
	if err != nil {
		return err
	}
	if err := writeArtifact(outDir, "deepseek_response.raw.txt", raw); err != nil {
		return err
	}

	parsed, err := symbol.ParseReport(bundle, raw)
	if err != nil {
		return fmt.Errorf("parse DeepSeek symbol report: %w", err)
	}
	reportJSON, err := json.MarshalIndent(parsed.Report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal symbol report: %w", err)
	}
	if err := writeArtifact(outDir, "symbol_report.json", reportJSON); err != nil {
		return err
	}
	warningsJSON, err := json.MarshalIndent(parsed.Warnings, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal parse warnings: %w", err)
	}
	if err := writeArtifact(outDir, "symbol_parse_warnings.json", warningsJSON); err != nil {
		return err
	}
	evaluation := symbol.Evaluate(parsed)
	evaluationJSON, err := json.MarshalIndent(evaluation, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal prompt evaluation: %w", err)
	}
	if err := writeArtifact(outDir, "symbol_evaluation.json", evaluationJSON); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "normalized DeepSeek response with %d parser warnings; contract score %d/%d\n", len(parsed.Warnings), evaluation.Score, evaluation.MaxScore)
	fmt.Fprintf(os.Stderr, "wrote %s\n", filepath.Join(outDir, "symbol_report.json"))
	return nil
}

func runSourceExplanation(ctx context.Context, explainer sourceExplainer, bundle sourceexplain.Bundle, outDir string) (sourceexplain.Explanation, error) {
	explanation, err := explainer.Explain(ctx, bundle)
	if len(explanation.Raw) > 0 {
		if writeErr := writeArtifact(outDir, "deepseek_source_response.raw.txt", explanation.Raw); writeErr != nil {
			return sourceexplain.Explanation{}, writeErr
		}
	}
	if err != nil {
		return explanation, fmt.Errorf("assess source with DeepSeek: %w", err)
	}
	reportJSON, err := json.MarshalIndent(explanation.Parsed.Report, "", "  ")
	if err != nil {
		return sourceexplain.Explanation{}, fmt.Errorf("marshal source report: %w", err)
	}
	if err := writeArtifact(outDir, "source_report.json", reportJSON); err != nil {
		return sourceexplain.Explanation{}, err
	}
	warningsJSON, err := json.MarshalIndent(explanation.Parsed.Warnings, "", "  ")
	if err != nil {
		return sourceexplain.Explanation{}, fmt.Errorf("marshal source parse warnings: %w", err)
	}
	if err := writeArtifact(outDir, "source_parse_warnings.json", warningsJSON); err != nil {
		return sourceexplain.Explanation{}, err
	}
	evaluationJSON, err := json.MarshalIndent(explanation.Evaluation, "", "  ")
	if err != nil {
		return sourceexplain.Explanation{}, fmt.Errorf("marshal source evaluation: %w", err)
	}
	if err := writeArtifact(outDir, "source_evaluation.json", evaluationJSON); err != nil {
		return sourceexplain.Explanation{}, err
	}
	fmt.Fprintf(os.Stderr, "normalized DeepSeek source response with %d parser warnings; contract score %d/%d\n",
		len(explanation.Parsed.Warnings), explanation.Evaluation.Score, explanation.Evaluation.MaxScore)
	fmt.Fprintf(os.Stderr, "wrote %s\n", filepath.Join(outDir, "source_report.json"))
	return explanation, nil
}

func writeArtifact(dir, name string, data []byte) error {
	path := filepath.Join(dir, name)
	temporaryPath := path + ".tmp"
	data = append(append([]byte{}, data...), '\n')
	if err := os.WriteFile(temporaryPath, data, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("rename %s: %w", name, err)
	}
	return nil
}
