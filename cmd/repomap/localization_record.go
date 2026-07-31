package main

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/localization"
	"github.com/dvordrova/repomap/internal/report"
)

type localizationPromptClientFactory func() (*deepseek.Client, error)

type architectureLocalizationRecordReplayFunc func(
	context.Context,
	string,
	string,
	func(localization.Prompt) (deepseek.LocalizationRequestEvidence, error),
) ([]byte, error)

func runLocalizationRecordCLI(args []string, stdout io.Writer) error {
	return runLocalizationRecordCLIWith(
		context.Background(),
		args,
		stdout,
		deepseek.NewPromptFromEnv,
		func(
			ctx context.Context,
			runDir,
			responsePath string,
			buildRequest func(
				localization.Prompt,
			) (deepseek.LocalizationRequestEvidence, error),
		) ([]byte, error) {
			return report.ReplayArchitectureLocalizationRussianRecordFile(
				ctx,
				runDir,
				responsePath,
				buildRequest,
			)
		},
	)
}

func runLocalizationRecordCLIWith(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	newClient localizationPromptClientFactory,
	replay architectureLocalizationRecordReplayFunc,
) error {
	if len(args) < 1 ||
		len(args) > 2 ||
		args[0] == "" ||
		strings.HasPrefix(args[0], "-") ||
		(len(args) == 2 && (args[1] == "" || strings.HasPrefix(args[1], "-"))) {
		return fmt.Errorf(
			"usage: repomap dev localization-record <run-dir> [<projection.json>]",
		)
	}
	runDir, err := filepath.Abs(args[0])
	if err != nil {
		return fmt.Errorf("localization record: resolve run dir: %w", err)
	}
	responsePath := ""
	if len(args) == 2 {
		responsePath, err = filepath.Abs(args[1])
		if err != nil {
			return fmt.Errorf("localization record: resolve response: %w", err)
		}
	}
	if newClient == nil {
		return fmt.Errorf("localization record: prompt client factory is required")
	}
	client, err := newClient()
	if err != nil {
		return fmt.Errorf(
			"localization record: provider identity configuration was rejected; check REPOMAP_LLM_* or DEEPSEEK_* settings",
		)
	}
	if client == nil {
		return fmt.Errorf("localization record: prompt client is required")
	}
	if replay == nil {
		return fmt.Errorf("localization record: replay is required")
	}
	encoded, err := replay(
		ctx,
		runDir,
		responsePath,
		client.BuildLocalizationRequest,
	)
	if err != nil {
		return err
	}
	if len(encoded) == 0 {
		return fmt.Errorf("localization record: empty result")
	}
	if _, err := stdout.Write(encoded); err != nil {
		return fmt.Errorf("localization record: write result: %w", err)
	}
	if _, err := stdout.Write([]byte{'\n'}); err != nil {
		return fmt.Errorf("localization record: write result terminator: %w", err)
	}
	return nil
}
