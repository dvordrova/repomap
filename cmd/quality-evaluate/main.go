package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/dvordrova/repomap/internal/quality"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("quality-evaluate", flag.ContinueOnError)
	flags.SetOutput(stderr)
	taskPath := flags.String("task", "", "path to a versioned quality task manifest")
	outPath := flags.String("out", "", "path for the replay result JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("quality-evaluate: unexpected positional arguments")
	}
	if strings.TrimSpace(*taskPath) == "" || strings.TrimSpace(*outPath) == "" {
		return fmt.Errorf("quality-evaluate: --task and --out are required")
	}
	if sameCleanPath(*taskPath, *outPath) {
		return fmt.Errorf("quality-evaluate: --out must not overwrite the task manifest")
	}

	loaded, err := quality.Load(*taskPath)
	if err != nil {
		return fmt.Errorf("quality-evaluate: %w", err)
	}
	if err := rejectProtectedOutput(*outPath, loaded); err != nil {
		return err
	}
	result, err := quality.Evaluate(loaded)
	if err != nil {
		return fmt.Errorf("quality-evaluate: evaluate: %w", err)
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("quality-evaluate: marshal result: %w", err)
	}
	if err := writeResult(*outPath, data); err != nil {
		return err
	}

	status := "FAIL"
	if result.Passed {
		status = "PASS"
	}
	fmt.Fprintf(stdout, "%s %s\n", status, result.TaskID)
	if !result.Passed {
		return fmt.Errorf("quality-evaluate: task did not pass; inspect %s", *outPath)
	}
	return nil
}

func sameCleanPath(left, right string) bool {
	leftAbsolute, leftErr := filepath.Abs(left)
	rightAbsolute, rightErr := filepath.Abs(right)
	return leftErr == nil && rightErr == nil && filepath.Clean(leftAbsolute) == filepath.Clean(rightAbsolute)
}

func rejectProtectedOutput(outPath string, loaded quality.LoadedTask) error {
	baseDir := filepath.Dir(loaded.ManifestPath)
	artifacts := loaded.Task.Artifacts
	protected := []struct {
		label string
		path  string
	}{
		{label: "the task manifest", path: loaded.ManifestPath},
		{label: "the orientation_context artifact", path: filepath.Join(baseDir, filepath.FromSlash(artifacts.OrientationContext.Path))},
		{label: "the orientation_response artifact", path: filepath.Join(baseDir, filepath.FromSlash(artifacts.OrientationResponse.Path))},
		{label: "the source_bundle artifact", path: filepath.Join(baseDir, filepath.FromSlash(artifacts.SourceBundle.Path))},
		{label: "the source_response artifact", path: filepath.Join(baseDir, filepath.FromSlash(artifacts.SourceResponse.Path))},
		{label: "the test_evidence artifact", path: filepath.Join(baseDir, filepath.FromSlash(artifacts.TestEvidence.Path))},
	}
	for _, item := range protected {
		same, err := sameFile(outPath, item.path)
		if err != nil {
			return fmt.Errorf("quality-evaluate: inspect --out against %s: %w", item.label, err)
		}
		if same {
			return fmt.Errorf("quality-evaluate: --out must not overwrite %s", item.label)
		}
	}
	return nil
}

func sameFile(left, right string) (bool, error) {
	if sameCleanPath(left, right) {
		return true, nil
	}
	rightInfo, err := os.Stat(right)
	if err != nil {
		return false, fmt.Errorf("stat protected path %q: %w", right, err)
	}
	leftInfo, err := os.Stat(left)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("stat output path %q: %w", left, err)
	}
	return os.SameFile(leftInfo, rightInfo), nil
}

func writeResult(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("quality-evaluate: create output directory: %w", err)
	}
	temporary, err := os.CreateTemp(dir, ".quality-evaluate-*")
	if err != nil {
		return fmt.Errorf("quality-evaluate: create temporary result: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}()

	data = append(append([]byte{}, data...), '\n')
	if _, err := temporary.Write(data); err != nil {
		return fmt.Errorf("quality-evaluate: write result: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("quality-evaluate: close result: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("quality-evaluate: rename result: %w", err)
	}
	return nil
}
