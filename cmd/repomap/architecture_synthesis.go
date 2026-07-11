package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/report"
)

const architectureSynthesisCacheDirectory = ".component-synthesis"

type componentLandscapeSynthesizer interface {
	SynthesizeComponentLandscape(context.Context, componentmap.SynthesisPrompt) ([]byte, error)
}

type architectureSynthesisOutcome struct {
	Cached         bool
	InputBytes     int
	LatencyMillis  int64
	FallbackReason componentmap.FallbackReason
}

func synthesizeArchitectureForRun(
	ctx context.Context,
	runDir string,
	repositoryPath string,
	stderr io.Writer,
) error {
	state, err := freshness.CaptureRepository(ctx, repositoryPath)
	if err != nil {
		return fmt.Errorf("architecture synthesis: capture repository revision: %w", err)
	}
	revision, err := state.Digest()
	if err != nil {
		return fmt.Errorf("architecture synthesis: repository revision: %w", err)
	}
	client, err := deepseek.NewFromEnv()
	if err != nil {
		return fmt.Errorf("architecture synthesis: provider configuration: %w", err)
	}
	outcome, err := prepareArchitectureSynthesis(
		ctx,
		runDir,
		revision,
		"openai-compatible/"+client.Auth,
		client.Model,
		client,
	)
	if err != nil {
		return err
	}
	cacheLabel := ""
	if outcome.Cached {
		cacheLabel = ", cached"
	}
	fmt.Fprintf(
		stderr,
		"repomap: architecture synthesis %d-byte prompt in %d ms%s\n",
		outcome.InputBytes,
		outcome.LatencyMillis,
		cacheLabel,
	)
	if outcome.FallbackReason != "" {
		fmt.Fprintf(stderr, "warning: architecture synthesis used %s fallback\n", outcome.FallbackReason)
	}
	return nil
}

func prepareArchitectureSynthesis(
	ctx context.Context,
	runDir string,
	repositoryRevision string,
	profile string,
	model string,
	provider componentLandscapeSynthesizer,
) (architectureSynthesisOutcome, error) {
	data, err := report.ReadRunDir(runDir)
	if err != nil {
		return architectureSynthesisOutcome{}, fmt.Errorf("architecture synthesis: read saved run: %w", err)
	}
	input, err := report.BuildArchitectureCanvasInput(data)
	if err != nil {
		return architectureSynthesisOutcome{}, fmt.Errorf("architecture synthesis: build candidates: %w", err)
	}
	return ensureArchitectureSynthesis(
		ctx,
		input.CandidateBundle,
		runDir,
		repositoryRevision,
		profile,
		model,
		provider,
	)
}

func ensureArchitectureSynthesis(
	ctx context.Context,
	bundle componentmap.CandidateBundle,
	runDir string,
	repositoryRevision string,
	profile string,
	model string,
	provider componentLandscapeSynthesizer,
) (architectureSynthesisOutcome, error) {
	if provider == nil {
		return architectureSynthesisOutcome{}, fmt.Errorf("architecture synthesis: provider is required")
	}
	cacheKey, err := componentmap.SynthesisCacheKey(repositoryRevision)
	if err != nil {
		return architectureSynthesisOutcome{}, err
	}
	runPath := filepath.Join(runDir, report.ArchitectureSynthesisFile)
	cachePath := filepath.Join(
		filepath.Dir(runDir),
		architectureSynthesisCacheDirectory,
		cacheKey+".json",
	)

	for _, candidate := range []struct {
		path      string
		copyToRun bool
	}{{path: runPath}, {path: cachePath, copyToRun: true}} {
		saved, readErr := os.ReadFile(candidate.path)
		if readErr == nil {
			outcome, replayErr := replayArchitectureSynthesisOutcome(bundle, repositoryRevision, saved)
			if replayErr != nil {
				return architectureSynthesisOutcome{}, fmt.Errorf(
					"architecture synthesis: reject saved record %s without another provider call: %w",
					candidate.path,
					replayErr,
				)
			}
			outcome.Cached = true
			if candidate.copyToRun {
				if err := writeArchitectureSynthesisRecord(runPath, saved); err != nil {
					return architectureSynthesisOutcome{}, err
				}
			}
			return outcome, nil
		}
		if !errors.Is(readErr, os.ErrNotExist) {
			return architectureSynthesisOutcome{}, fmt.Errorf(
				"architecture synthesis: read saved record %s: %w",
				candidate.path,
				readErr,
			)
		}
	}

	prompt, err := componentmap.BuildSynthesisPrompt(bundle)
	if err != nil {
		return architectureSynthesisOutcome{}, err
	}
	started := time.Now()
	raw, err := provider.SynthesizeComponentLandscape(ctx, prompt)
	latency := time.Since(started)
	if err != nil {
		return architectureSynthesisOutcome{}, fmt.Errorf("architecture synthesis: provider call: %w", err)
	}
	result, err := componentmap.RecordSynthesisResponse(
		bundle,
		repositoryRevision,
		profile,
		model,
		latency,
		raw,
	)
	if err != nil {
		return architectureSynthesisOutcome{}, fmt.Errorf("architecture synthesis: validate response: %w", err)
	}
	saved, err := json.MarshalIndent(result.Record, "", "  ")
	if err != nil {
		return architectureSynthesisOutcome{}, fmt.Errorf("architecture synthesis: encode record: %w", err)
	}
	saved = append(saved, '\n')
	if err := writeArchitectureSynthesisRecord(cachePath, saved); err != nil {
		return architectureSynthesisOutcome{}, err
	}
	if err := writeArchitectureSynthesisRecord(runPath, saved); err != nil {
		return architectureSynthesisOutcome{}, err
	}
	return architectureSynthesisOutcome{
		InputBytes:     result.Record.Call.Metadata.InputBytes,
		LatencyMillis:  result.Record.Call.Metadata.LatencyMillis,
		FallbackReason: result.Landscape.FallbackReason,
	}, nil
}

func replayArchitectureSynthesisOutcome(
	bundle componentmap.CandidateBundle,
	repositoryRevision string,
	saved []byte,
) (architectureSynthesisOutcome, error) {
	landscape, err := componentmap.ReplaySynthesis(bundle, repositoryRevision, saved)
	if err != nil {
		return architectureSynthesisOutcome{}, err
	}
	var record componentmap.SynthesisRecord
	if err := json.Unmarshal(saved, &record); err != nil {
		return architectureSynthesisOutcome{}, err
	}
	return architectureSynthesisOutcome{
		InputBytes:     record.Call.Metadata.InputBytes,
		LatencyMillis:  record.Call.Metadata.LatencyMillis,
		FallbackReason: landscape.FallbackReason,
	}, nil
}

func writeArchitectureSynthesisRecord(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("architecture synthesis: create record directory: %w", err)
	}
	temporary, err := os.CreateTemp(dir, ".repomap-architecture-synthesis-")
	if err != nil {
		return fmt.Errorf("architecture synthesis: create temporary record: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return fmt.Errorf("architecture synthesis: protect temporary record: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		return fmt.Errorf("architecture synthesis: write temporary record: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("architecture synthesis: close temporary record: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("architecture synthesis: replace saved record: %w", err)
	}
	return nil
}
