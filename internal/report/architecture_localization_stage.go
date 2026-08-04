package report

import (
	"context"
	"fmt"

	"github.com/dvordrova/repomap/internal/localization"
	"github.com/dvordrova/repomap/internal/secretscan"
)

type architectureLocalizationStageProvider func(
	context.Context,
	localization.Prompt,
) ([]byte, error)

// BuildArchitectureLocalizationRussianPrompt returns the exact deterministic
// provider-neutral prompt for one current eligible English Architecture
// Canvas. It performs no provider call and writes no artifact.
func BuildArchitectureLocalizationRussianPrompt(runDir string) ([]byte, error) {
	prepared, err := prepareArchitectureLocalizationRussian(runDir)
	if err != nil {
		return nil, err
	}
	_, encoded, err := buildArchitectureLocalizationRussianPrompt(prepared)
	return encoded, err
}

// ReplayArchitectureLocalizationRussianStageFile runs the provider-free stage
// with one explicit bounded local response file. The file adapter never opens
// a network connection and the run directory remains unchanged.
func ReplayArchitectureLocalizationRussianStageFile(
	ctx context.Context,
	runDir,
	responsePath string,
) ([]byte, error) {
	response, err := readArchitectureLocalizationProjectionFile(responsePath)
	if err != nil {
		return nil, err
	}
	return replayArchitectureLocalizationRussianStage(
		ctx,
		runDir,
		func(context.Context, localization.Prompt) ([]byte, error) {
			return append([]byte(nil), response...), nil
		},
	)
}

func replayArchitectureLocalizationRussianStage(
	ctx context.Context,
	runDir string,
	provider architectureLocalizationStageProvider,
) ([]byte, error) {
	if provider == nil {
		return nil, fmt.Errorf("architecture localization stage: provider is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	prepared, err := prepareArchitectureLocalizationRussian(runDir)
	if err != nil {
		return nil, err
	}
	prompt, _, err := buildArchitectureLocalizationRussianPrompt(prepared)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	response, err := provider(ctx, prompt)
	if contextErr := ctx.Err(); contextErr != nil {
		return nil, contextErr
	}
	if err != nil {
		return nil, fmt.Errorf("architecture localization stage: provider response unavailable")
	}
	if len(response) == 0 || len(response) > maxArchitectureLocalizationArtifactBytes {
		return nil, fmt.Errorf("architecture localization stage: response exceeds its byte limit")
	}
	projection, err := localization.DecodeRussianProviderResponse(
		prepared.canonical,
		prepared.input,
		response,
	)
	if err != nil {
		return nil, err
	}
	return replayPreparedArchitectureLocalizationRussian(prepared, projection)
}

func buildArchitectureLocalizationRussianPrompt(
	prepared preparedArchitectureLocalizationRussian,
) (localization.Prompt, []byte, error) {
	if kind, found := architectureLocalizationCredential(
		prepared.canonical,
		prepared.input,
		localization.Projection{},
	); found {
		return localization.Prompt{}, nil, fmt.Errorf(
			"architecture localization stage: prompt input contains an obvious %s",
			kind,
		)
	}
	prompt, err := localization.BuildRussianPrompt(
		prepared.canonical,
		prepared.input,
	)
	if err != nil {
		return localization.Prompt{}, nil, fmt.Errorf(
			"architecture localization stage: build prompt: %w",
			err,
		)
	}
	encoded, err := localization.MarshalPrompt(prompt)
	if err != nil {
		return localization.Prompt{}, nil, fmt.Errorf(
			"architecture localization stage: encode prompt: %w",
			err,
		)
	}
	if kind, found := secretscan.DetectAlways(string(encoded)); found {
		return localization.Prompt{}, nil, fmt.Errorf(
			"architecture localization stage: prompt contains an obvious %s",
			kind,
		)
	}
	return prompt, encoded, nil
}
