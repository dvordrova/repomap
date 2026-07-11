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
	"github.com/dvordrova/repomap/internal/envfile"
	"github.com/dvordrova/repomap/internal/symbol"
)

func main() {
	_ = envfile.Load(".env")
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
	experimentJSON, err := json.MarshalIndent(struct {
		PromptVersion  string `json:"prompt_version"`
		ResponseFormat string `json:"response_format"`
		Model          string `json:"model"`
		BundleBytes    int    `json:"bundle_bytes"`
		RequestBytes   int    `json:"request_bytes"`
	}{
		PromptVersion: promptVersion, ResponseFormat: *responseFormat, Model: promptClient.Model,
		BundleBytes: len(bundleJSON), RequestBytes: len(promptJSON),
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal prompt experiment metadata: %w", err)
	}

	if err := os.MkdirAll(*outDir, 0o700); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	artifacts := []struct {
		name string
		data []byte
	}{
		{name: "evidence_graph.json", data: graphJSON},
		{name: "symbol_bundle.json", data: bundleJSON},
		{name: "deepseek_request.redacted.json", data: promptJSON},
		{name: "prompt_experiment.json", data: experimentJSON},
	}
	for _, artifact := range artifacts {
		if err := writeArtifact(*outDir, artifact.name, artifact.data); err != nil {
			return err
		}
	}

	fmt.Fprintf(os.Stderr, "resolved %s at %s:%d\n",
		bundle.Target.Entity.Name,
		bundle.Target.Entity.Location.Path,
		bundle.Target.Entity.Location.Line,
	)
	fmt.Fprintf(os.Stderr, "bundle: %d bytes, prompt request: %d bytes, incoming calls: %d, outgoing calls: %d\n",
		len(bundleJSON), len(promptJSON), len(bundle.IncomingCalls), len(bundle.OutgoingCalls))
	fmt.Fprintf(os.Stderr, "wrote prompt-only artifacts to %s\n", *outDir)

	if !*callDeepSeek {
		return nil
	}

	client, err := deepseek.NewFromEnv()
	if err != nil {
		return err
	}
	var raw []byte
	if *responseFormat == "tagged" {
		raw, err = client.ExplainSymbolTagged(ctx, bundleJSON)
	} else {
		raw, err = client.ExplainSymbol(ctx, bundleJSON)
	}
	if err != nil {
		return err
	}
	if err := writeArtifact(*outDir, "deepseek_response.raw.txt", raw); err != nil {
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
	if err := writeArtifact(*outDir, "symbol_report.json", reportJSON); err != nil {
		return err
	}
	warningsJSON, err := json.MarshalIndent(parsed.Warnings, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal parse warnings: %w", err)
	}
	if err := writeArtifact(*outDir, "symbol_parse_warnings.json", warningsJSON); err != nil {
		return err
	}
	evaluation := symbol.Evaluate(parsed)
	evaluationJSON, err := json.MarshalIndent(evaluation, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal prompt evaluation: %w", err)
	}
	if err := writeArtifact(*outDir, "symbol_evaluation.json", evaluationJSON); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "normalized DeepSeek response with %d parser warnings; contract score %d/%d\n", len(parsed.Warnings), evaluation.Score, evaluation.MaxScore)
	fmt.Fprintf(os.Stderr, "wrote %s\n", filepath.Join(*outDir, "symbol_report.json"))
	return nil
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
