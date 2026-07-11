package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/dvordrova/repomap/internal/symbol"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("symbol-evaluate", flag.ContinueOnError)
	flags.SetOutput(stderr)
	bundlePath := flags.String("bundle", "", "path to a symbol bundle JSON file")
	responsePath := flags.String("response", "", "path to a raw model response")
	outDir := flags.String("out-dir", "", "directory for normalized evaluation artifacts")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("symbol-evaluate: unexpected positional arguments")
	}
	if *bundlePath == "" || *responsePath == "" || *outDir == "" {
		return fmt.Errorf("symbol-evaluate: --bundle, --response, and --out-dir are required")
	}

	bundleJSON, err := os.ReadFile(*bundlePath)
	if err != nil {
		return fmt.Errorf("symbol-evaluate: read bundle: %w", err)
	}
	var bundle symbol.Bundle
	if err := json.Unmarshal(bundleJSON, &bundle); err != nil {
		return fmt.Errorf("symbol-evaluate: decode bundle: %w", err)
	}
	raw, err := os.ReadFile(*responsePath)
	if err != nil {
		return fmt.Errorf("symbol-evaluate: read response: %w", err)
	}
	parsed, err := symbol.ParseReport(bundle, raw)
	if err != nil {
		return fmt.Errorf("symbol-evaluate: parse response: %w", err)
	}
	evaluation := symbol.Evaluate(parsed)

	artifacts := []struct {
		name  string
		value any
	}{
		{name: "symbol_report.json", value: parsed.Report},
		{name: "symbol_parse_warnings.json", value: parsed.Warnings},
		{name: "symbol_evaluation.json", value: evaluation},
	}
	for _, artifact := range artifacts {
		data, err := json.MarshalIndent(artifact.value, "", "  ")
		if err != nil {
			return fmt.Errorf("symbol-evaluate: marshal %s: %w", artifact.name, err)
		}
		if err := writeArtifact(*outDir, artifact.name, data); err != nil {
			return err
		}
	}

	fmt.Fprintf(stdout, "%d/%d\n", evaluation.Score, evaluation.MaxScore)
	fmt.Fprintf(stderr, "normalized %s response with %d parser warnings\n", parsed.Format, len(parsed.Warnings))
	return nil
}

func writeArtifact(dir, name string, data []byte) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("symbol-evaluate: create output directory: %w", err)
	}
	path := filepath.Join(dir, name)
	temporaryPath := path + ".tmp"
	data = append(append([]byte{}, data...), '\n')
	if err := os.WriteFile(temporaryPath, data, 0o600); err != nil {
		return fmt.Errorf("symbol-evaluate: write %s: %w", name, err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("symbol-evaluate: rename %s: %w", name, err)
	}
	return nil
}
