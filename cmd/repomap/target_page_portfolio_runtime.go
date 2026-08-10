package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/snapshot"
)

// targetPublishedRun retains only the exact local authority needed for the
// provider-free final portfolio render. It is live-run-only; the shared sealed
// portfolio and each run manifest are the persisted authority.
type targetPublishedRun struct {
	RunID                 string
	RunDir                string
	Target                analysistarget.Target
	Authority             report.RunAuthority
	RepositoryStateSHA256 string
	SelectedRevision      string
	GoTarget              string
	SourceEpisodeJSON     []byte
	GitLabURL             string
	GitHubURL             string
}

type targetPageRunSet struct {
	Outcomes []snapshot.TargetPageOutcome
	Ready    []targetPublishedRun
}

type siblingTargetRunner func(
	snapshot.Snapshot,
	snapshot.TargetRunProjection,
	string,
) (targetPublishedRun, error)

// collectTargetPageRuns restores container order, reuses the already
// successful default run, and invokes runner exactly once for every additional
// selected target. An additional target failure is represented only by the
// closed unavailable code in persisted state; its detailed error remains the
// caller's local diagnostic.
func collectTargetPageRuns(
	container snapshot.TargetRunContainer,
	defaultRun targetPublishedRun,
	runIDFor func(snapshot.TargetRunProjection) string,
	runner siblingTargetRunner,
	output *runOutput,
	onUnavailable func(snapshot.TargetRunProjection, error),
) (targetPageRunSet, error) {
	if err := container.Validate(); err != nil {
		return targetPageRunSet{}, err
	}
	if defaultRun.Target.Ref != container.DefaultTargetRef ||
		defaultRun.RunID == "" || defaultRun.RunDir == "" ||
		filepath.Base(defaultRun.RunDir) != defaultRun.RunID {
		return targetPageRunSet{}, fmt.Errorf("target page portfolio: default run does not match container authority")
	}
	if err := snapshot.ValidateTargetPageRunID(defaultRun.RunID); err != nil {
		return targetPageRunSet{}, err
	}
	if runIDFor == nil || runner == nil {
		return targetPageRunSet{}, fmt.Errorf("target page portfolio: sibling runner is required")
	}

	result := targetPageRunSet{
		Outcomes: make([]snapshot.TargetPageOutcome, 0, len(container.Targets)),
		Ready:    make([]targetPublishedRun, 0, len(container.Targets)),
	}
	for _, projection := range container.Targets {
		if projection.Target.Ref == container.DefaultTargetRef {
			result.Outcomes = append(result.Outcomes, snapshot.TargetPageOutcome{
				TargetRef: projection.Target.Ref,
				State:     snapshot.TargetPageReady,
				RunID:     defaultRun.RunID,
			})
			result.Ready = append(result.Ready, defaultRun)
			continue
		}

		runID := runIDFor(projection)
		if err := snapshot.ValidateTargetPageRunID(runID); err != nil {
			return targetPageRunSet{}, err
		}
		consoleTarget := targetPageConsoleContext{
			DisplayPath: projection.DisplayPath,
			PackagePath: projection.Target.PackagePath,
			RunID:       runID,
			Role:        "sibling",
		}
		output.TargetPage("started", consoleTarget)
		scoped, err := container.ScopedSnapshot(projection.Target.Ref)
		if err == nil {
			var published targetPublishedRun
			published, err = runner(scoped, projection, runID)
			if err == nil && (published.RunID != runID ||
				published.Target.Ref != projection.Target.Ref || published.RunDir == "" ||
				filepath.Base(published.RunDir) != runID ||
				filepath.Dir(published.RunDir) != filepath.Dir(defaultRun.RunDir) ||
				published.RepositoryStateSHA256 != defaultRun.RepositoryStateSHA256 ||
				published.SelectedRevision != defaultRun.SelectedRevision ||
				published.GoTarget != defaultRun.GoTarget) {
				err = fmt.Errorf("target page portfolio: sibling run returned mismatched authority")
			}
			if err == nil {
				output.TargetPage("complete", consoleTarget)
				result.Outcomes = append(result.Outcomes, snapshot.TargetPageOutcome{
					TargetRef: projection.Target.Ref,
					State:     snapshot.TargetPageReady,
					RunID:     runID,
				})
				result.Ready = append(result.Ready, published)
				continue
			}
		}
		output.TargetPage("unavailable", consoleTarget)
		if onUnavailable != nil {
			onUnavailable(projection, err)
		}
		result.Outcomes = append(result.Outcomes, snapshot.TargetPageOutcome{
			TargetRef:       projection.Target.Ref,
			State:           snapshot.TargetPageUnavailable,
			UnavailableCode: snapshot.TargetPageUnavailableTargetRunFailed,
		})
	}
	return result, nil
}

func publishTargetPagePortfolio(
	repo string,
	extraArgs []string,
	deps defaultRunDeps,
	container snapshot.TargetRunContainer,
	defaultRun targetPublishedRun,
	output *runOutput,
) (publicationAssessment, error) {
	runSet, err := collectTargetPageRuns(
		container,
		defaultRun,
		func(projection snapshot.TargetRunProjection) string {
			return debugdump.GenerateRunID(repoRunLabel(repo) + "-" + projection.DisplayPath)
		},
		func(
			scoped snapshot.Snapshot,
			projection snapshot.TargetRunProjection,
			runID string,
		) (targetPublishedRun, error) {
			childDeps := deps
			childDeps.precomputedSnapshot = &scoped
			childDeps.runIDOverride = runID
			childDeps.siblingTargetRun = true
			childDeps.expectedRepositoryStateSHA256 = defaultRun.RepositoryStateSHA256
			var published targetPublishedRun
			childDeps.publishedTargetSink = func(result targetPublishedRun) {
				published = result
			}
			if err := runDefaultWithDeps(repo, extraArgs, childDeps); err != nil {
				return targetPublishedRun{}, err
			}
			return published, nil
		},
		output,
		func(projection snapshot.TargetRunProjection, runErr error) {
			if output != nil {
				output.Warn(
					"Target page unavailable",
					"target: "+projection.DisplayPath,
					"reason: "+runErr.Error(),
				)
			}
		},
	)
	if err != nil {
		return failedPublication(), err
	}
	portfolio, err := snapshot.BuildTargetPagePortfolio(container, runSet.Outcomes)
	if err != nil {
		return failedPublication(), err
	}
	finalize := deps.finalizeTargetPages
	if finalize == nil {
		finalize = finalizeTargetPageRuns
	}
	if err := finalize(container, portfolio, runSet.Ready); err != nil {
		return failedPublication(), err
	}
	var defaultAssessment publicationAssessment
	defaultVerified := false
	for _, published := range runSet.Ready {
		assessment, assessErr := assessRunPublication(published.RunDir)
		if assessErr != nil {
			return failedPublication(), fmt.Errorf(
				"verify final target page %s: %w", published.Target.PackageDir, assessErr,
			)
		}
		if published.Target.Ref == container.DefaultTargetRef {
			defaultAssessment = assessment
			defaultVerified = true
		}
	}
	if !defaultVerified {
		return failedPublication(), fmt.Errorf("target page portfolio: finalized default page is unavailable")
	}
	return defaultAssessment, nil
}

func finalizeTargetPageRuns(
	container snapshot.TargetRunContainer,
	portfolio snapshot.TargetPagePortfolio,
	runs []targetPublishedRun,
) error {
	containerJSON, err := container.CanonicalJSON()
	if err != nil {
		return err
	}
	portfolioJSON, err := portfolio.CanonicalJSON()
	if err != nil {
		return err
	}
	alreadyFinalized, err := targetPageRunsAlreadyFinalized(
		container, portfolio, runs, containerJSON, portfolioJSON,
	)
	if err != nil {
		return err
	}
	if alreadyFinalized {
		return nil
	}
	for _, run := range runs {
		writer, writerErr := debugdump.OpenWriter(run.RunDir, true)
		if writerErr != nil {
			return fmt.Errorf("target page portfolio: open run %s: %w", run.RunID, writerErr)
		}
		writeErr := writer.WriteValidatedFile(
			snapshot.TargetRunContainerArtifactFilename,
			containerJSON,
			func(raw []byte) error {
				decoded, decodeErr := snapshot.DecodeTargetRunContainer(raw)
				if decodeErr != nil {
					return decodeErr
				}
				if decoded.SHA256 != container.SHA256 || decoded.CatalogRef != container.CatalogRef {
					return fmt.Errorf("target run container: recovery binding mismatch")
				}
				return nil
			},
		)
		if writeErr == nil {
			writeErr = writer.WriteValidatedFile(
				snapshot.TargetPagePortfolioArtifactFilename,
				portfolioJSON,
				func(raw []byte) error {
					decoded, decodeErr := snapshot.DecodeTargetPagePortfolio(raw)
					if decodeErr != nil {
						return decodeErr
					}
					return decoded.ValidateAgainstContainer(container)
				},
			)
		}
		closeErr := writer.Close()
		if writeErr != nil {
			return fmt.Errorf("target page portfolio: bind run %s: %w", run.RunID, writeErr)
		}
		if closeErr != nil {
			return fmt.Errorf("target page portfolio: close run %s: %w", run.RunID, closeErr)
		}
	}

	authorities := make([]snapshot.TargetPageSiblingAuthority, 0, len(runs))
	for _, run := range runs {
		navigation, navErr := targetNavigationForRun(container, portfolio, run.Target.Ref)
		if navErr != nil {
			return navErr
		}
		if err := run.generateWithTargetNavigation(navigation); err != nil {
			return fmt.Errorf("target page portfolio: render run %s: %w", run.RunID, err)
		}
		manifest, err := report.ReadRunManifest(run.RunDir)
		if err != nil {
			return fmt.Errorf("target page portfolio: verify run %s: %w", run.RunID, err)
		}
		if manifest.RepositoryStateSHA256 != run.RepositoryStateSHA256 ||
			manifest.MaterialInputs.SelectedRevision != run.SelectedRevision {
			return fmt.Errorf("target page portfolio: run %s repository authority mismatch", run.RunID)
		}
		authorities = append(authorities, snapshot.TargetPageSiblingAuthority{
			RunID:                             run.RunID,
			AnalysisTargetRef:                 manifest.MaterialInputs.AnalysisTargetRef,
			TargetRunContainerArtifactSHA256:  manifest.MaterialInputs.TargetRunContainerSHA256,
			TargetPagePortfolioArtifactSHA256: manifest.MaterialInputs.TargetPagePortfolioSHA256,
		})
	}
	if err := portfolio.ValidateSiblingAuthorities(container, authorities); err != nil {
		return err
	}
	return nil
}

// targetPageRunsAlreadyFinalized makes recovery idempotent. A fully verified
// portfolio is returned byte-for-byte untouched; a missing or interrupted
// binding falls through to the existing-run writer and authorized rerender.
func targetPageRunsAlreadyFinalized(
	container snapshot.TargetRunContainer,
	portfolio snapshot.TargetPagePortfolio,
	runs []targetPublishedRun,
	containerJSON []byte,
	portfolioJSON []byte,
) (bool, error) {
	authorities := make([]snapshot.TargetPageSiblingAuthority, 0, len(runs))
	for _, run := range runs {
		savedContainer, found, err := readTargetPageRunFile(
			run.RunDir, snapshot.TargetRunContainerArtifactFilename,
			snapshot.MaxTargetRunContainerBytes,
		)
		if err != nil {
			return false, err
		}
		if !found {
			return false, nil
		}
		if !bytes.Equal(savedContainer, containerJSON) {
			return false, fmt.Errorf("target page portfolio: run %s has a different target container", run.RunID)
		}
		savedPortfolio, found, err := readTargetPageRunFile(
			run.RunDir, snapshot.TargetPagePortfolioArtifactFilename,
			snapshot.MaxTargetPagePortfolioBytes,
		)
		if err != nil {
			return false, err
		}
		if !found {
			return false, nil
		}
		if !bytes.Equal(savedPortfolio, portfolioJSON) {
			return false, fmt.Errorf("target page portfolio: run %s has a different target portfolio", run.RunID)
		}
		manifest, err := report.ReadRunManifest(run.RunDir)
		if err != nil {
			// Exact artifacts can be present after a crash immediately before
			// their authorized rerender. Re-enter finalization in that case.
			return false, nil
		}
		if manifest.RepositoryStateSHA256 != run.RepositoryStateSHA256 ||
			manifest.MaterialInputs.SelectedRevision != run.SelectedRevision {
			return false, fmt.Errorf("target page portfolio: run %s repository authority mismatch", run.RunID)
		}
		authorities = append(authorities, snapshot.TargetPageSiblingAuthority{
			RunID:                             run.RunID,
			AnalysisTargetRef:                 manifest.MaterialInputs.AnalysisTargetRef,
			TargetRunContainerArtifactSHA256:  manifest.MaterialInputs.TargetRunContainerSHA256,
			TargetPagePortfolioArtifactSHA256: manifest.MaterialInputs.TargetPagePortfolioSHA256,
		})
	}
	if err := portfolio.ValidateSiblingAuthorities(container, authorities); err != nil {
		return false, err
	}
	return true, nil
}

func readTargetPageRunFile(runDir, name string, maxBytes int) ([]byte, bool, error) {
	root, err := os.OpenRoot(runDir)
	if err != nil {
		return nil, false, fmt.Errorf("target page portfolio: open run %s: %w", filepath.Base(runDir), err)
	}
	defer root.Close()
	file, err := root.Open(name)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("target page portfolio: open %s in run %s: %w", name, filepath.Base(runDir), err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, false, fmt.Errorf("target page portfolio: stat %s in run %s: %w", name, filepath.Base(runDir), err)
	}
	if !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > int64(maxBytes) {
		return nil, false, fmt.Errorf("target page portfolio: %s in run %s is not a bounded regular file", name, filepath.Base(runDir))
	}
	raw, err := io.ReadAll(io.LimitReader(file, int64(maxBytes)+1))
	if err != nil {
		return nil, false, fmt.Errorf("target page portfolio: read %s in run %s: %w", name, filepath.Base(runDir), err)
	}
	if len(raw) > maxBytes {
		return nil, false, fmt.Errorf("target page portfolio: %s in run %s exceeds its byte limit", name, filepath.Base(runDir))
	}
	return raw, true, nil
}

func targetNavigationForRun(
	container snapshot.TargetRunContainer,
	portfolio snapshot.TargetPagePortfolio,
	currentTargetRef string,
) (*report.TargetNavigationPortfolio, error) {
	return report.BuildTargetNavigation(container, portfolio, currentTargetRef)
}

func (run targetPublishedRun) generateWithTargetNavigation(
	navigation *report.TargetNavigationPortfolio,
) error {
	options := report.RenderOptions{TargetNavigation: navigation}
	switch {
	case len(run.SourceEpisodeJSON) != 0:
		return report.GenerateAuthorizedWithSourceEpisodeAndOptions(
			run.RunDir, run.Authority, run.SourceEpisodeJSON, options,
		)
	case run.GitLabURL != "":
		return report.GenerateAuthorizedGitLabWithOptions(
			run.RunDir, run.Authority, run.GitLabURL, options,
		)
	case run.GitHubURL != "":
		return report.GenerateAuthorizedGitHubWithOptions(
			run.RunDir, run.Authority, run.GitHubURL, options,
		)
	default:
		return report.GenerateAuthorizedWithOptions(run.RunDir, run.Authority, options)
	}
}
