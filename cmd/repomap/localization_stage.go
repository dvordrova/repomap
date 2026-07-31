package main

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/dvordrova/repomap/internal/report"
)

type architectureLocalizationPromptFunc func(string) ([]byte, error)

type architectureLocalizationStageReplayFunc func(
	context.Context,
	string,
	string,
) ([]byte, error)

func runLocalizationStageCLI(args []string, stdout io.Writer) error {
	return runLocalizationStageCLIWith(
		context.Background(),
		args,
		stdout,
		report.BuildArchitectureLocalizationRussianPrompt,
		report.ReplayArchitectureLocalizationRussianStageFile,
	)
}

func runLocalizationStageCLIWith(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	buildPrompt architectureLocalizationPromptFunc,
	replay architectureLocalizationStageReplayFunc,
) error {
	if len(args) < 1 ||
		len(args) > 2 ||
		args[0] == "" ||
		strings.HasPrefix(args[0], "-") ||
		(len(args) == 2 && (args[1] == "" || strings.HasPrefix(args[1], "-"))) {
		return fmt.Errorf(
			"usage: repomap dev localization-stage <run-dir> [<projection.json>]",
		)
	}
	runDir, err := filepath.Abs(args[0])
	if err != nil {
		return fmt.Errorf("localization stage: resolve run dir: %w", err)
	}
	var encoded []byte
	if len(args) == 1 {
		encoded, err = buildPrompt(runDir)
	} else {
		responsePath, pathErr := filepath.Abs(args[1])
		if pathErr != nil {
			return fmt.Errorf("localization stage: resolve response: %w", pathErr)
		}
		encoded, err = replay(ctx, runDir, responsePath)
	}
	if err != nil {
		return err
	}
	if len(encoded) == 0 {
		return fmt.Errorf("localization stage: empty result")
	}
	if _, err := stdout.Write(encoded); err != nil {
		return fmt.Errorf("localization stage: write result: %w", err)
	}
	if _, err := stdout.Write([]byte{'\n'}); err != nil {
		return fmt.Errorf("localization stage: write result terminator: %w", err)
	}
	return nil
}
