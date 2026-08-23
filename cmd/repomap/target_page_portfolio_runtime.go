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
	GoTargetSource        string
	GoTargetBaseline      string
	GitLabURL             string
	GitHubURL             string
}

type targetPageRunSet struct {
	Outcomes         []snapshot.TargetPageOutcome
	Ready            []targetPublishedRun
	AttemptedRunDirs []string
	CompletedTargets []targetPageConsoleContext
}

type siblingTargetRunner func(
	snapshot.Snapshot,
	snapshot.TargetRunProjection,
	string,
) (targetPublishedRun, error)

// collectTargetPageRuns restores container order, reuses the already
// successful default run, and invokes runner exactly once for every additional
// selected target. Every selected Go target is part of the ordinary default
// portfolio, so one failed page fails the complete publication.
func collectTargetPageRuns(
	container snapshot.TargetRunContainer,
	defaultRun targetPublishedRun,
	runIDFor func(snapshot.TargetRunProjection) string,
	runner siblingTargetRunner,
	output *runOutput,
	onFailure func(snapshot.TargetRunProjection, error),
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
		Outcomes:         make([]snapshot.TargetPageOutcome, 0, len(container.Targets)),
		Ready:            make([]targetPublishedRun, 0, len(container.Targets)),
		AttemptedRunDirs: []string{defaultRun.RunDir},
		CompletedTargets: make([]targetPageConsoleContext, 0, len(container.Targets)-1),
	}
	for _, projection := range container.Targets {
		if projection.Target.Ref == container.DefaultTargetRef {
			result.Outcomes = append(result.Outcomes, snapshot.TargetPageOutcome{
				TargetRef: projection.Target.Ref,
				RunID:     defaultRun.RunID,
			})
			result.Ready = append(result.Ready, defaultRun)
			continue
		}

		runID := runIDFor(projection)
		if err := snapshot.ValidateTargetPageRunID(runID); err != nil {
			return targetPageRunSet{}, err
		}
		result.AttemptedRunDirs = append(result.AttemptedRunDirs, filepath.Join(filepath.Dir(defaultRun.RunDir), runID))
		consoleTarget := targetPageConsoleContext{
			DisplayPath: projection.DisplayPath,
			Scope:       analysisTargetSubject(projection.Target),
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
				published.GoTarget != defaultRun.GoTarget ||
				published.GoTargetSource != defaultRun.GoTargetSource ||
				published.GoTargetBaseline != defaultRun.GoTargetBaseline ||
				published.GitLabURL != defaultRun.GitLabURL ||
				published.GitHubURL != defaultRun.GitHubURL) {
				err = fmt.Errorf("target page portfolio: sibling run returned mismatched authority")
			}
			if err == nil {
				result.CompletedTargets = append(result.CompletedTargets, consoleTarget)
				result.Outcomes = append(result.Outcomes, snapshot.TargetPageOutcome{
					TargetRef: projection.Target.Ref,
					RunID:     runID,
				})
				result.Ready = append(result.Ready, published)
				continue
			}
		}
		output.TargetPage("failed", consoleTarget)
		if onFailure != nil {
			onFailure(projection, err)
		}
		return result, fmt.Errorf(
			"target page %s failed: %w", projection.DisplayPath, err,
		)
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
) (report.PublicationAssessment, error) {
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
					"Target page failed",
					"target: "+projection.DisplayPath,
					"reason: "+runErr.Error(),
				)
			}
		},
	)
	if err != nil {
		markTargetPagesFailed(output, runSet.CompletedTargets)
		runDirs := append([]string{defaultRun.RunDir}, runSet.AttemptedRunDirs...)
		return report.FailedPublicationAssessment(), errors.Join(err, quarantineTargetPagePublication(runDirs))
	}
	quarantineOnFailure := func(runErr error) (report.PublicationAssessment, error) {
		markTargetPagesFailed(output, runSet.CompletedTargets)
		return report.FailedPublicationAssessment(), errors.Join(runErr, quarantineTargetPagePublication(runSet.AttemptedRunDirs))
	}
	portfolio, err := snapshot.BuildTargetPagePortfolio(container, runSet.Outcomes)
	if err != nil {
		return quarantineOnFailure(err)
	}
	finalize := deps.finalizeTargetPages
	if finalize == nil {
		finalize = finalizeTargetPageRuns
	}
	if err := finalize(container, portfolio, runSet.Ready); err != nil {
		return quarantineOnFailure(err)
	}
	var defaultAssessment report.PublicationAssessment
	defaultVerified := false
	for _, published := range runSet.Ready {
		assessment, assessErr := report.AssessRunPublication(published.RunDir)
		if assessErr != nil {
			return quarantineOnFailure(fmt.Errorf(
				"verify final target page %s: %w", published.Target.DisplayPath(), assessErr,
			))
		}
		if published.Target.Ref == container.DefaultTargetRef {
			defaultAssessment = assessment
			defaultVerified = true
		}
	}
	if !defaultVerified {
		return quarantineOnFailure(fmt.Errorf("target page portfolio: finalized default page is missing"))
	}
	for _, consoleTarget := range runSet.CompletedTargets {
		output.TargetPage("complete", consoleTarget)
	}
	return defaultAssessment, nil
}

func markTargetPagesFailed(output *runOutput, targets []targetPageConsoleContext) {
	if output == nil {
		return
	}
	for _, target := range targets {
		output.TargetPage("failed", target)
	}
}

// quarantineTargetPagePublication removes browser authority and gives any
// already-rendered files an explicit failed suffix. Raw analysis artifacts
// remain available for diagnostics, but no partial multi-target publication
// keeps a product-looking report.html/report.json pair.
func quarantineTargetPagePublication(runDirs []string) error {
	seen := make(map[string]struct{}, len(runDirs))
	var result error
	for _, runDir := range runDirs {
		if runDir == "" {
			continue
		}
		clean := filepath.Clean(runDir)
		if _, exists := seen[clean]; exists {
			continue
		}
		seen[clean] = struct{}{}
		if err := report.RemoveRunManifest(clean); err != nil {
			result = errors.Join(result, err)
		}
		for _, name := range []string{"report.html", "report.json"} {
			from := filepath.Join(clean, name)
			to := filepath.Join(clean, name+".failed")
			if err := os.Rename(from, to); err != nil && !os.IsNotExist(err) {
				result = errors.Join(result, fmt.Errorf("quarantine %s: %w", from, err))
			}
		}
	}
	return result
}

func finalizeTargetPageRuns(
	container snapshot.TargetRunContainer,
	portfolio snapshot.TargetPagePortfolio,
	runs []targetPublishedRun,
) error {
	bundleDefault, err := standaloneTargetBundleDefaultRun(container, runs)
	if err != nil {
		return err
	}
	containerJSON, err := container.CanonicalJSON()
	if err != nil {
		return err
	}
	portfolioJSON, err := portfolio.CanonicalJSON()
	if err != nil {
		return err
	}
	alreadyFinalized, err := targetPageRunsAlreadyFinalized(
		container, portfolio, runs, containerJSON, portfolioJSON, bundleDefault,
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

	navigationPages, programTargetByOuterRef, defaultProgramTargetID, err :=
		targetNavigationPagesForRuns(container, portfolio, runs)
	if err != nil {
		return err
	}
	authorities := make([]snapshot.TargetPageSiblingAuthority, 0, len(runs))
	prepared := make([]report.PreparedStandaloneTarget, 0, len(runs))
	for _, run := range runs {
		currentProgramTargetID := programTargetByOuterRef[run.Target.Ref]
		navigation, navErr := targetNavigationForRun(
			navigationPages,
			defaultProgramTargetID,
			currentProgramTargetID,
		)
		if navErr != nil {
			return navErr
		}
		if bundleDefault != nil {
			preparedTarget, prepareErr := run.generatePreparedWithTargetNavigation(navigation)
			if prepareErr != nil {
				return fmt.Errorf("target page portfolio: render hosted run %s: %w", run.RunID, prepareErr)
			}
			prepared = append(prepared, preparedTarget)
		} else if err := run.generateWithTargetNavigation(navigation); err != nil {
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
	if bundleDefault != nil {
		if err := report.WriteStandaloneTargetBundleAtomic(
			bundleDefault.RunDir, container, portfolio, prepared,
		); err != nil {
			return fmt.Errorf("target page portfolio: publish standalone target bundle: %w", err)
		}
		found, inspectErr := inspectExactStandaloneTargetBundle(
			filepath.Join(bundleDefault.RunDir, "report.html"), container, portfolio,
		)
		if inspectErr != nil {
			return fmt.Errorf("target page portfolio: verify standalone target bundle: %w", inspectErr)
		}
		if !found {
			return fmt.Errorf("target page portfolio: standalone target bundle marker is absent after publication")
		}
	}
	return nil
}

// standaloneTargetBundleDefaultRun resolves a multi-target static report.
func standaloneTargetBundleDefaultRun(
	container snapshot.TargetRunContainer,
	runs []targetPublishedRun,
) (*targetPublishedRun, error) {
	if len(container.Targets) <= 1 {
		return nil, nil
	}
	var defaultRun *targetPublishedRun
	for index := range runs {
		run := &runs[index]
		if run.Target.Ref == container.DefaultTargetRef {
			if defaultRun != nil {
				return nil, fmt.Errorf("target page portfolio: duplicate default run")
			}
			defaultRun = run
		}
	}
	if defaultRun == nil {
		return nil, fmt.Errorf("target page portfolio: default run is missing")
	}
	if defaultRun.GitLabURL != "" && defaultRun.GitHubURL != "" {
		return nil, fmt.Errorf("target page portfolio: default run has conflicting source hosts")
	}
	if defaultRun.GitLabURL == "" && defaultRun.GitHubURL == "" {
		return nil, nil
	}
	for _, run := range runs {
		if run.GitLabURL != defaultRun.GitLabURL ||
			run.GitHubURL != defaultRun.GitHubURL {
			return nil, fmt.Errorf("target page portfolio: hosted sibling authority differs")
		}
	}
	owned := *defaultRun
	return &owned, nil
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
	bundleDefault *targetPublishedRun,
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
	if bundleDefault == nil {
		return true, nil
	}
	found, err := inspectExactStandaloneTargetBundle(
		filepath.Join(bundleDefault.RunDir, "report.html"), container, portfolio,
	)
	if err != nil {
		return false, fmt.Errorf("target page portfolio: inspect standalone target bundle: %w", err)
	}
	if !found {
		// The canonical D269 pages may have completed before D286's final atomic
		// replacement. Re-enter the provider-free hosted render in that case.
		return false, nil
	}
	return true, nil
}

func inspectExactStandaloneTargetBundle(
	htmlPath string,
	container snapshot.TargetRunContainer,
	portfolio snapshot.TargetPagePortfolio,
) (bool, error) {
	identity, found, err := report.InspectStandaloneTargetBundleHTML(htmlPath)
	if err != nil || !found {
		return found, err
	}
	defaultIndex := -1
	for index, projection := range container.Targets {
		if projection.Target.Ref == container.DefaultTargetRef {
			defaultIndex = index
			break
		}
	}
	if identity.TargetRunContainerSHA256 != container.SHA256 ||
		identity.TargetPagePortfolioSHA256 != portfolio.SHA256 ||
		identity.DefaultTargetIndex != defaultIndex ||
		identity.TargetCount != len(container.Targets) {
		return true, fmt.Errorf("standalone target bundle authority mismatch")
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

func targetNavigationPagesForRuns(
	container snapshot.TargetRunContainer,
	portfolio snapshot.TargetPagePortfolio,
	runs []targetPublishedRun,
) ([]report.TargetNavigationPage, map[string]string, string, error) {
	if err := portfolio.ValidateAgainstContainer(container); err != nil {
		return nil, nil, "", err
	}
	runsByOuterRef := make(map[string]targetPublishedRun, len(runs))
	for _, run := range runs {
		if _, duplicate := runsByOuterRef[run.Target.Ref]; duplicate {
			return nil, nil, "", fmt.Errorf("target page portfolio: duplicate published target ref")
		}
		runsByOuterRef[run.Target.Ref] = run
	}
	if len(runsByOuterRef) != len(container.Targets) {
		return nil, nil, "", fmt.Errorf("target page portfolio: published runs do not cover selected targets")
	}
	pages := make([]report.TargetNavigationPage, 0, len(container.Targets))
	programTargetByOuterRef := make(map[string]string, len(container.Targets))
	defaultProgramTargetID := ""
	for index, projection := range container.Targets {
		outerPage := portfolio.Targets[index]
		run, found := runsByOuterRef[projection.Target.Ref]
		if !found || run.RunID != outerPage.RunID {
			return nil, nil, "", fmt.Errorf("target page portfolio: published run binding mismatch")
		}
		page, err := report.LoadTargetNavigationPage(run.RunDir, run.RunID)
		if err != nil {
			return nil, nil, "", fmt.Errorf("target page portfolio: program page %s: %w", run.RunID, err)
		}
		pages = append(pages, page)
		programTargetByOuterRef[projection.Target.Ref] = page.ProgramTarget.ID
		if outerPage.Default {
			defaultProgramTargetID = page.ProgramTarget.ID
		}
	}
	if defaultProgramTargetID == "" {
		return nil, nil, "", fmt.Errorf("target page portfolio: default program target is missing")
	}
	return pages, programTargetByOuterRef, defaultProgramTargetID, nil
}

func targetNavigationForRun(
	pages []report.TargetNavigationPage,
	defaultTargetID string,
	currentTargetID string,
) (*report.TargetNavigationPortfolio, error) {
	return report.BuildTargetNavigation(pages, defaultTargetID, currentTargetID)
}

func (run targetPublishedRun) generateWithTargetNavigation(
	navigation *report.TargetNavigationPortfolio,
) error {
	options := report.RenderOptions{TargetNavigation: navigation}
	switch {
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

func (run targetPublishedRun) generatePreparedWithTargetNavigation(
	navigation *report.TargetNavigationPortfolio,
) (report.PreparedStandaloneTarget, error) {
	options := report.RenderOptions{TargetNavigation: navigation}
	switch {
	case run.GitLabURL != "" && run.GitHubURL == "":
		return report.GenerateAuthorizedGitLabPreparedWithOptions(
			run.RunDir, run.Authority, run.GitLabURL, options,
		)
	case run.GitHubURL != "" && run.GitLabURL == "":
		return report.GenerateAuthorizedGitHubPreparedWithOptions(
			run.RunDir, run.Authority, run.GitHubURL, options,
		)
	default:
		return report.PreparedStandaloneTarget{}, fmt.Errorf("hosted target bundle requires exactly one source host")
	}
}
