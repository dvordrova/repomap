package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
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
) (architectureSynthesisOutcome, error) {
	state, err := freshness.CaptureRepository(ctx, repositoryPath)
	if err != nil {
		return architectureSynthesisOutcome{}, fmt.Errorf("architecture synthesis: capture repository revision: %w", err)
	}
	revision, err := state.Digest()
	if err != nil {
		return architectureSynthesisOutcome{}, fmt.Errorf("architecture synthesis: repository revision: %w", err)
	}
	client, err := deepseek.NewFromEnv()
	if err != nil {
		return architectureSynthesisOutcome{}, fmt.Errorf("architecture synthesis: provider configuration: %w", err)
	}
	outcome, err := prepareArchitectureSynthesis(
		ctx,
		runDir,
		revision,
		"openai-compatible/"+client.Auth,
		client.Model,
		client,
	)
	status := architectureSynthesisStatus(outcome, err)
	if statusErr := writeArchitectureSynthesisStatus(runDir, status); statusErr != nil {
		if err != nil {
			return outcome, errors.Join(err, statusErr)
		}
		return outcome, statusErr
	}
	if err != nil {
		return outcome, err
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
		fmt.Fprintf(stderr, "repomap: architecture synthesis downgraded to local fallback: %s\n", outcome.FallbackReason)
	}
	return outcome, nil
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
	if input.CandidateBundle.GroundingMode == componentmap.GroundingPackages {
		packageCount := 0
		for _, candidate := range input.CandidateBundle.Candidates {
			if candidate.ID.Kind == componentmap.MemberPackage {
				packageCount++
			}
		}
		if packageCount < 2 {
			return architectureSynthesisOutcome{}, fmt.Errorf("architecture synthesis: insufficient package evidence for a useful landscape")
		}
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
	cacheKey, err := componentmap.SynthesisCacheKey(repositoryRevision, bundle)
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
	outcome := architectureSynthesisOutcome{InputBytes: len(prompt.System) + len(prompt.User)}
	started := time.Now()
	raw, err := provider.SynthesizeComponentLandscape(ctx, prompt)
	latency := time.Since(started)
	outcome.LatencyMillis = latency.Milliseconds()
	if err != nil {
		return outcome, fmt.Errorf("architecture synthesis: provider call: %w", err)
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
	outcome = architectureSynthesisOutcome{
		InputBytes:     result.Record.Call.Metadata.InputBytes,
		LatencyMillis:  result.Record.Call.Metadata.LatencyMillis,
		FallbackReason: result.Landscape.FallbackReason,
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
	return outcome, nil
}

func architectureSynthesisDiagnosticCodes(diagnostics []componentmap.Diagnostic) []string {
	const maxCodes = 4

	codes := make([]string, 0, min(len(diagnostics), maxCodes))
	seen := make(map[string]struct{}, len(diagnostics))
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == "" {
			continue
		}
		if _, exists := seen[diagnostic.Code]; exists {
			continue
		}
		seen[diagnostic.Code] = struct{}{}
		codes = append(codes, diagnostic.Code)
		if len(codes) == maxCodes {
			break
		}
	}
	return codes
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

func architectureSynthesisStatus(
	outcome architectureSynthesisOutcome,
	synthesisErr error,
) report.ArchitectureSynthesisStatus {
	status := report.ArchitectureSynthesisStatus{
		Version:       report.ArchitectureSynthesisStatusVersion,
		PromptBytes:   outcome.InputBytes,
		LatencyMillis: outcome.LatencyMillis,
	}
	if synthesisErr == nil {
		if outcome.Cached {
			status.State = report.ArchitectureSynthesisCached
		} else {
			status.State = report.ArchitectureSynthesisSucceeded
			status.ProviderRequestCount = 1
		}
		return status
	}

	status.State = report.ArchitectureSynthesisFailed
	if outcome.LatencyMillis > 0 {
		status.ProviderRequestCount = 1
	}
	message := synthesisErr.Error()
	switch {
	case strings.Contains(message, "response content is empty"):
		status.ErrorCode = "empty_response"
	case strings.Contains(message, "unusable"), strings.Contains(message, "validate response"):
		status.ErrorCode = "invalid_response"
	default:
		status.ErrorCode = "provider_error"
	}
	return status
}

func writeArchitectureSynthesisStatus(
	runDir string,
	status report.ArchitectureSynthesisStatus,
) error {
	if err := status.Validate(); err != nil {
		return fmt.Errorf("architecture synthesis: status: %w", err)
	}
	data, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return fmt.Errorf("architecture synthesis: encode status: %w", err)
	}
	data = append(data, '\n')
	return writeArchitectureSynthesisRecord(
		filepath.Join(runDir, report.ArchitectureSynthesisStatusFile),
		data,
	)
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
