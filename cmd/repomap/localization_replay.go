package main

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/dvordrova/repomap/internal/report"
)

type architectureLocalizationReplayFunc func(string, string) ([]byte, error)

func runLocalizationReplayCLI(args []string, stdout io.Writer) error {
	return runLocalizationReplayCLIWith(
		args,
		stdout,
		report.ReplayArchitectureLocalizationRussianFile,
	)
}

func runLocalizationReplayCLIWith(
	args []string,
	stdout io.Writer,
	replay architectureLocalizationReplayFunc,
) error {
	if len(args) != 2 ||
		args[0] == "" ||
		args[1] == "" ||
		strings.HasPrefix(args[0], "-") ||
		strings.HasPrefix(args[1], "-") {
		return fmt.Errorf("usage: repomap dev localization-replay <run-dir> <projection.json>")
	}
	runDir, err := filepath.Abs(args[0])
	if err != nil {
		return fmt.Errorf("localization replay: resolve run dir: %w", err)
	}
	projectionPath, err := filepath.Abs(args[1])
	if err != nil {
		return fmt.Errorf("localization replay: resolve projection: %w", err)
	}
	encoded, err := replay(runDir, projectionPath)
	if err != nil {
		return err
	}
	if len(encoded) == 0 {
		return fmt.Errorf("localization replay: empty result")
	}
	if _, err := stdout.Write(encoded); err != nil {
		return fmt.Errorf("localization replay: write result: %w", err)
	}
	if _, err := stdout.Write([]byte{'\n'}); err != nil {
		return fmt.Errorf("localization replay: write result terminator: %w", err)
	}
	return nil
}
