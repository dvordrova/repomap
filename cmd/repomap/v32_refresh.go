package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"reflect"
	"sync"
	"syscall"
	"time"

	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/semanticdiscovery"
	"github.com/dvordrova/repomap/internal/studymap"
)

func runV32RefreshCLI(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("v32-refresh", flag.ContinueOnError)
	flags.SetOutput(stderr)
	runDir := flags.String("run-dir", "", "copied saved run directory to enrich")
	repoRoot := flags.String("repo", "", "exact repository root captured by the run")
	reuseStudy := flags.Bool("reuse-study", false, "reuse the saved v3.1 Brief and directions, then run only v3.2 reviews")
	operateOnly := flags.Bool("operate-only", false, "keep an already reviewed Study Map and refresh only operational artifacts")
	replaySaved := flags.Bool("replay-saved", false, "locally revalidate saved split Study and Paved Path responses without a provider call")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *runDir == "" || *repoRoot == "" || flags.NArg() != 0 {
		return fmt.Errorf("usage: repomap dev v32-refresh --run-dir <copied-run-dir> --repo <repo> [--reuse-study | --operate-only | --replay-saved]")
	}
	selectedModes := 0
	for _, selected := range []bool{*reuseStudy, *operateOnly, *replaySaved} {
		if selected {
			selectedModes++
		}
	}
	if selectedModes > 1 {
		return fmt.Errorf("v32 refresh: --reuse-study, --operate-only, and --replay-saved are mutually exclusive")
	}
	absRun, err := filepath.Abs(*runDir)
	if err != nil {
		return err
	}
	absRepo, err := filepath.Abs(*repoRoot)
	if err != nil {
		return err
	}
	var client *deepseek.Client
	if !*replaySaved {
		client, err = deepseek.NewFromEnv()
		if err != nil {
			return fmt.Errorf("v32 refresh: provider configuration: %w", err)
		}
		var progressMu sync.Mutex
		client.OnWait = func(progress deepseek.WaitProgress) {
			progressMu.Lock()
			defer progressMu.Unlock()
			fmt.Fprintf(stderr, "repomap: %s still running after %s (Ctrl-C to cancel)\n",
				progress.Stage, progress.Elapsed.Round(time.Second))
		}
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	manifest, err := report.ReadRunManifest(absRun)
	if err != nil {
		return fmt.Errorf("v32 refresh: verify saved run authority: %w", err)
	}
	if manifest.AnalysisRoot != absRepo {
		return fmt.Errorf("v32 refresh: --repo does not match the saved analysis root")
	}
	var studyErr error
	var operateErr error
	if *replaySaved {
		fmt.Fprintln(stderr, "repomap: locally replaying saved Study and Paved Path responses")
		_, studyErr = replaySavedStudyMapV32(absRun)
		_, operateErr = replaySavedPavedPathsV32(absRun)
	} else if *operateOnly {
		if err := validateReviewedStudyMapV32(absRun); err != nil {
			return fmt.Errorf("v32 refresh: operate-only requires a reviewed v3.2 Study Map: %w", err)
		}
		fmt.Fprintln(stderr, "repomap: keeping the already reviewed Study Map")
	} else if *reuseStudy {
		fmt.Fprintln(stderr, "repomap: replaying the saved Study Map through bounded source reviews")
		studyErr = refreshSavedStudyMapV32(ctx, absRun, client)
	} else {
		fmt.Fprintln(stderr, "repomap: running split Brief, candidate, and Reading Pack stages")
		_, studyErr = prepareStudyMap(ctx, absRun, absRepo, client)
	}
	if terminalErr := v32RefreshResourceLimitError("Study", studyErr); terminalErr != nil {
		return terminalErr
	}
	if studyErr != nil {
		fmt.Fprintf(stderr, "warning: %v; the copied report will not publish an unreviewed Study Map\n", studyErr)
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	if !*replaySaved {
		fmt.Fprintln(stderr, "repomap: collecting bounded operational evidence and editing Paved Paths")
		operateStatus, prepareErr := preparePavedPaths(
			ctx,
			absRun,
			absRepo,
			client,
			&manifest.RepositoryState,
		)
		operateErr = prepareErr
		if terminalErr := v32RefreshResourceLimitError("Paved Paths", operateErr); terminalErr != nil {
			return terminalErr
		}
		if operateErr != nil {
			fmt.Fprintf(stderr, "warning: %v; retained %d exact operational landmark(s)\n",
				operateErr, operateStatus.Landmarks)
		}
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	reportData, err := report.ReadRunDir(absRun)
	if err != nil {
		return fmt.Errorf("v32 refresh: read enriched report inputs: %w", err)
	}
	currentRepository, err := freshness.CaptureRepository(ctx, absRepo)
	if err != nil {
		return fmt.Errorf("v32 refresh: capture repository after enrichment: %w", err)
	}
	authority, err := report.ConfirmRunAuthorityScoped(
		ctx,
		manifest.AnalysisRoot,
		manifest.RepositoryState,
		currentRepository,
		report.CapturedInputPaths(reportData),
		false,
	)
	if err != nil {
		return fmt.Errorf("v32 refresh: confirm enriched report authority: %w", err)
	}
	if err := report.GenerateAuthorized(absRun, authority); err != nil {
		return fmt.Errorf("v32 refresh: render authorized report: %w", err)
	}
	reportPath := filepath.Join(absRun, "report.html")
	fmt.Fprintf(stdout, "%s\n", reportPath)
	return errors.Join(studyErr, operateErr)
}

func v32RefreshResourceLimitError(stage string, err error) error {
	if !isSemanticResourceLimit(err) {
		return nil
	}
	return fmt.Errorf("v32 refresh: %s resource limit: %w", stage, err)
}

func validateReviewedStudyMapV32(runDir string) error {
	raw, err := readV32ReplayRaw(filepath.Join(runDir, studymap.RecordFile))
	if err != nil {
		return err
	}
	record, err := studymap.DecodeRecord(raw)
	if err != nil {
		return err
	}
	bundleSHA, err := studymap.BundleHash(record.Bundle)
	if err != nil {
		return err
	}
	var attempt studyMapAttempt
	if err := readV32ReplayJSON(filepath.Join(runDir, studymap.AttemptFile), &attempt); err != nil {
		return err
	}
	if attempt.Version != 2 || attempt.ValidationState != "accepted" ||
		attempt.BundleSHA256 != bundleSHA ||
		(attempt.PromptVersion != "repository-study-map-split-v2" &&
			attempt.PromptVersion != "repository-study-map-v31-replay-plus-reviews-v2") {
		return fmt.Errorf("saved Study Map is not bound to an accepted v3.2 review attempt")
	}
	brief, directions, err := loadBoundStudyMapInputs(runDir, record.Bundle, bundleSHA)
	if err != nil {
		return err
	}
	reviews, _, _, err := loadBoundStudyMapReviews(
		runDir,
		record.Bundle,
		directions,
		bundleSHA,
	)
	if err != nil {
		return err
	}
	rebuilt, _, err := studymap.BuildReviewedRecord(record.Bundle, brief, directions, reviews)
	if err != nil || !equalV32Projection(rebuilt, record) {
		return fmt.Errorf("saved Study Map does not rebuild from its reviewed v3.2 inputs")
	}
	var status studyMapStatus
	if err := readV32ReplayJSON(filepath.Join(runDir, studymap.StatusFile), &status); err != nil {
		return err
	}
	if status.State != "published" || status.Selected != len(record.Directions) {
		return fmt.Errorf("saved Study Map does not have a matching published v3.2 status")
	}
	return nil
}

func refreshSavedStudyMapV32(
	ctx context.Context,
	runDir string,
	provider semanticDiscoveryEditor,
) (returnErr error) {
	recordPath := filepath.Join(runDir, studymap.RecordFile)
	raw, err := os.ReadFile(recordPath)
	if err != nil {
		return fmt.Errorf("v32 refresh: read saved Study Map: %w", err)
	}
	saved, err := studymap.DecodeRecord(raw)
	if err != nil {
		return fmt.Errorf("v32 refresh: decode saved Study Map: %w", err)
	}
	var sourceAttempt studyMapAttempt
	if err := readV32ReplayJSON(filepath.Join(runDir, studymap.AttemptFile), &sourceAttempt); err != nil {
		return fmt.Errorf("v32 refresh: read saved Study Map attempt: %w", err)
	}
	bundleSHA, err := studymap.BundleHash(saved.Bundle)
	if err != nil {
		return err
	}
	if sourceAttempt.Version != 1 ||
		sourceAttempt.PromptVersion != semanticdiscovery.StudyMapPromptVersion ||
		sourceAttempt.BundleSHA256 != bundleSHA || sourceAttempt.ValidationState != "accepted" {
		return fmt.Errorf("v32 refresh: saved Study Map attempt is not bound to the canonical record")
	}
	proposal, err := studymap.DecodeProposal(sourceAttempt.Response)
	if err != nil {
		return fmt.Errorf("v32 refresh: decode saved Study Map proposal: %w", err)
	}
	boundRecord, err := studymap.BuildRecord(saved.Bundle, proposal)
	if err != nil || !reflect.DeepEqual(boundRecord, saved) {
		return fmt.Errorf("v32 refresh: saved Study Map attempt does not rebuild the canonical record")
	}
	if err := writeGoldenJSON(filepath.Join(runDir, studyMapSourceAttemptFile), sourceAttempt); err != nil {
		return fmt.Errorf("v32 refresh: save source Study Map attempt: %w", err)
	}
	// The copied run must never fall back to unreviewed v3.1 copy after the
	// v3.2 guard was requested.
	if err := os.Remove(recordPath); err != nil {
		return err
	}
	started := time.Now()
	reviewed, reduction, stages, reviewErr := reviewSavedStudyMapV32(ctx, runDir, saved, provider)
	status := studyMapStatus{
		Version: studyMapStatusVersion, Anchors: len(saved.Bundle.Anchors),
		Areas: len(saved.Bundle.Areas), Documents: len(saved.Bundle.Documents),
		Mechanisms: len(saved.Bundle.Mechanisms), Candidates: len(saved.Directions),
		Validated: reduction.Reviewed, Selected: reduction.Selected,
		Stages: stages, Metrics: aggregateStudyMapMetrics(stages, reviewErr),
		WallMillis: time.Since(started).Milliseconds(),
	}
	status.ProviderLatencyMillis = status.Metrics.LatencyMillis
	bundleSHA, hashErr := studymap.BundleHash(saved.Bundle)
	if hashErr != nil {
		return hashErr
	}
	attempt := studyMapAttempt{
		Version: 2, PromptVersion: "repository-study-map-v31-replay-plus-reviews-v2",
		BundleSHA256: bundleSHA, Metrics: status.Metrics,
	}
	if reviewErr != nil {
		status.State = "failed"
		status.FailureReason = semanticDiscoveryReason(reviewErr.Error())
		attempt.ValidationState = "rejected"
		attempt.FailureReason = status.FailureReason
		_ = writeGoldenJSON(filepath.Join(runDir, studymap.AttemptFile), attempt)
		_ = writeGoldenJSON(filepath.Join(runDir, studymap.StatusFile), status)
		return reviewErr
	}
	status.State = "published"
	status.RepositoryType = reviewed.RepositoryType
	attempt.ValidationState = "accepted"
	if proposalRaw, marshalErr := json.Marshal(studyMapV32ProposalFromRecord(reviewed)); marshalErr == nil {
		attempt.Response = proposalRaw
	}
	if err := writeGoldenJSON(filepath.Join(runDir, studymap.AttemptFile), attempt); err != nil {
		return err
	}
	if err := writeGoldenJSON(filepath.Join(runDir, studymap.StatusFile), status); err != nil {
		return err
	}
	if err := writeGoldenJSON(recordPath, reviewed); err != nil {
		return err
	}
	return nil
}
