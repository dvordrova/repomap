package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/snapshot"
)

const maxTargetPageRecoveryMetadataBytes = 4 << 20

type targetPageRecoveryRunFlags []string

func (values *targetPageRecoveryRunFlags) String() string {
	return strings.Join(*values, ",")
}

func (values *targetPageRecoveryRunFlags) Set(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("sibling run directory is empty")
	}
	*values = append(*values, value)
	return nil
}

func runFinalizeTargetPagesCLI(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("finalize-target-pages", flag.ContinueOnError)
	fs.SetOutput(stderr)
	defaultRunDir := fs.String("run-dir", "", "default target run directory")
	sourceEpisodePath := fs.String("source-episode", "", "approved source episode used by the original run")
	var siblingRunDirs targetPageRecoveryRunFlags
	fs.Var(&siblingRunDirs, "sibling-run", "successful sibling target run directory (repeat for every sibling)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 || strings.TrimSpace(*defaultRunDir) == "" {
		return fmt.Errorf("usage: repomap dev finalize-target-pages --run-dir DEFAULT --sibling-run RUN [--sibling-run RUN ...]")
	}
	var sourceEpisodeJSON []byte
	var err error
	if strings.TrimSpace(*sourceEpisodePath) != "" {
		sourceEpisodeJSON, err = readSourceEpisodeFile(*sourceEpisodePath)
		if err != nil {
			return err
		}
	}
	portfolio, runs, err := recoverExistingTargetPageRuns(
		context.Background(), *defaultRunDir, siblingRunDirs, sourceEpisodeJSON,
	)
	if err != nil {
		return err
	}
	exactDefaultRunDir, err := exactExistingTargetPageRunDir(*defaultRunDir)
	if err != nil {
		return err
	}
	containerRaw, _, err := readTargetPageRunFile(
		exactDefaultRunDir,
		snapshot.TargetRunContainerArtifactFilename,
		snapshot.MaxTargetRunContainerBytes,
	)
	if err != nil {
		return err
	}
	container, err := snapshot.DecodeTargetRunContainer(containerRaw)
	if err != nil {
		return err
	}
	if err := finalizeTargetPageRuns(container, portfolio, runs); err != nil {
		return err
	}
	for _, run := range runs {
		if _, err := assessRunPublication(run.RunDir); err != nil {
			return fmt.Errorf("verify recovered target page %s: %w", run.Target.PackageDir, err)
		}
	}
	linkLatest(filepath.Dir(exactDefaultRunDir), exactDefaultRunDir, stderr)
	fmt.Fprintf(stdout, "Finalized target pages: %d\nReport: %s/report.html\n", len(runs), exactDefaultRunDir)
	return nil
}

func recoverExistingTargetPageRuns(
	ctx context.Context,
	defaultRunDir string,
	siblingRunDirs []string,
	sourceEpisodeJSON []byte,
) (snapshot.TargetPagePortfolio, []targetPublishedRun, error) {
	defaultRunDir, err := exactExistingTargetPageRunDir(defaultRunDir)
	if err != nil {
		return snapshot.TargetPagePortfolio{}, nil, err
	}
	containerRaw, found, err := readTargetPageRunFile(
		defaultRunDir,
		snapshot.TargetRunContainerArtifactFilename,
		snapshot.MaxTargetRunContainerBytes,
	)
	if err != nil {
		return snapshot.TargetPagePortfolio{}, nil, err
	}
	if !found {
		return snapshot.TargetPagePortfolio{}, nil, fmt.Errorf("target page recovery: default run has no target run container")
	}
	container, err := snapshot.DecodeTargetRunContainer(containerRaw)
	if err != nil {
		return snapshot.TargetPagePortfolio{}, nil, err
	}

	portfolioRaw, portfolioFound, err := readTargetPageRunFile(
		defaultRunDir,
		snapshot.TargetPagePortfolioArtifactFilename,
		snapshot.MaxTargetPagePortfolioBytes,
	)
	if err != nil {
		return snapshot.TargetPagePortfolio{}, nil, err
	}
	var portfolio snapshot.TargetPagePortfolio
	runDirs := make([]string, 0, len(container.Targets))
	if portfolioFound {
		portfolio, err = snapshot.DecodeTargetPagePortfolio(portfolioRaw)
		if err != nil {
			return snapshot.TargetPagePortfolio{}, nil, err
		}
		if err := portfolio.ValidateAgainstContainer(container); err != nil {
			return snapshot.TargetPagePortfolio{}, nil, err
		}
		if len(siblingRunDirs) != 0 {
			expectedSiblings := make(map[string]struct{})
			for _, page := range portfolio.Targets {
				if page.State == snapshot.TargetPageReady && !page.Default {
					expectedSiblings[page.RunID] = struct{}{}
				}
			}
			if len(siblingRunDirs) != len(expectedSiblings) {
				return snapshot.TargetPagePortfolio{}, nil, fmt.Errorf("target page recovery: supplied siblings do not match the sealed portfolio")
			}
			seenSiblings := make(map[string]struct{}, len(siblingRunDirs))
			for _, sibling := range siblingRunDirs {
				exactSibling, exactErr := exactExistingTargetPageRunDir(sibling)
				if exactErr != nil {
					return snapshot.TargetPagePortfolio{}, nil, exactErr
				}
				runID := filepath.Base(exactSibling)
				if filepath.Dir(exactSibling) != filepath.Dir(defaultRunDir) {
					return snapshot.TargetPagePortfolio{}, nil, fmt.Errorf("target page recovery: every run must be a sibling of the default run")
				}
				if _, expected := expectedSiblings[runID]; !expected {
					return snapshot.TargetPagePortfolio{}, nil, fmt.Errorf("target page recovery: supplied sibling is absent from the sealed portfolio")
				}
				if _, duplicate := seenSiblings[runID]; duplicate {
					return snapshot.TargetPagePortfolio{}, nil, fmt.Errorf("target page recovery: duplicate supplied sibling")
				}
				seenSiblings[runID] = struct{}{}
			}
		}
		for _, page := range portfolio.Targets {
			if page.State == snapshot.TargetPageReady {
				runDirs = append(runDirs, filepath.Join(filepath.Dir(defaultRunDir), page.RunID))
			}
		}
	} else {
		runDirs = append(runDirs, defaultRunDir)
		runDirs = append(runDirs, siblingRunDirs...)
		if len(runDirs) != len(container.Targets) {
			return snapshot.TargetPagePortfolio{}, nil, fmt.Errorf(
				"target page recovery: provide exactly %d --sibling-run value(s); no target is guessed from the runs directory",
				len(container.Targets)-1,
			)
		}
	}

	projectionByRef := make(map[string]snapshot.TargetRunProjection, len(container.Targets))
	for _, projection := range container.Targets {
		projectionByRef[projection.Target.Ref] = projection
	}
	runsByRef := make(map[string]targetPublishedRun, len(runDirs))
	currentByRepository := make(map[string]freshness.RepositoryState)
	var commonRepositorySHA256 string
	var commonRevision string
	for _, candidate := range runDirs {
		runDir, err := exactExistingTargetPageRunDir(candidate)
		if err != nil {
			return snapshot.TargetPagePortfolio{}, nil, err
		}
		if filepath.Dir(runDir) != filepath.Dir(defaultRunDir) {
			return snapshot.TargetPagePortfolio{}, nil, fmt.Errorf("target page recovery: every run must be a sibling of the default run")
		}
		metadataRaw, found, err := readTargetPageRunFile(runDir, "metadata.json", maxTargetPageRecoveryMetadataBytes)
		if err != nil {
			return snapshot.TargetPagePortfolio{}, nil, err
		}
		if !found {
			return snapshot.TargetPagePortfolio{}, nil, fmt.Errorf("target page recovery: run %s has no metadata", filepath.Base(runDir))
		}
		var metadata debugdump.RunMeta
		if err := json.Unmarshal(metadataRaw, &metadata); err != nil {
			return snapshot.TargetPagePortfolio{}, nil, fmt.Errorf("target page recovery: decode metadata for run %s", filepath.Base(runDir))
		}
		projection, selected := projectionByRef[metadata.AnalysisTargetRef]
		if !selected || metadata.RunID != filepath.Base(runDir) ||
			metadata.AnalysisTargetPackage != projection.Target.PackagePath {
			return snapshot.TargetPagePortfolio{}, nil, fmt.Errorf("target page recovery: run %s does not match one selected target", filepath.Base(runDir))
		}
		if runDir == defaultRunDir && metadata.AnalysisTargetRef != container.DefaultTargetRef {
			return snapshot.TargetPagePortfolio{}, nil, fmt.Errorf("target page recovery: --run-dir is not the selected default target")
		}
		if metadata.EffectiveOptions.SourceEpisode != (len(sourceEpisodeJSON) != 0) {
			return snapshot.TargetPagePortfolio{}, nil, fmt.Errorf(
				"target page recovery: run %s source-episode mode differs; pass the original --source-episode file",
				metadata.RunID,
			)
		}
		if _, duplicate := runsByRef[metadata.AnalysisTargetRef]; duplicate {
			return snapshot.TargetPagePortfolio{}, nil, fmt.Errorf("target page recovery: duplicate run for one selected target")
		}
		seed, err := report.ReadRunManifestAuthoritySeed(runDir)
		if err != nil {
			return snapshot.TargetPagePortfolio{}, nil, fmt.Errorf("target page recovery: read run %s authority: %w", metadata.RunID, err)
		}
		if commonRepositorySHA256 == "" {
			commonRepositorySHA256 = seed.RepositoryStateSHA256
			commonRevision = seed.SelectedRevision
		} else if seed.RepositoryStateSHA256 != commonRepositorySHA256 || seed.SelectedRevision != commonRevision {
			return snapshot.TargetPagePortfolio{}, nil, fmt.Errorf("target page recovery: sibling repository authorities differ")
		}
		current, captured := currentByRepository[seed.RepositoryIdentity]
		if !captured {
			current, err = freshness.CaptureRepository(ctx, seed.RepositoryIdentity)
			if err != nil {
				return snapshot.TargetPagePortfolio{}, nil, fmt.Errorf("target page recovery: capture current repository: %w", err)
			}
			currentByRepository[seed.RepositoryIdentity] = current
		}
		authority, err := report.ReconfirmRunManifestAuthority(
			ctx, runDir, current, metadata.EffectiveOptions.StrictSnapshot,
		)
		if err != nil {
			return snapshot.TargetPagePortfolio{}, nil, fmt.Errorf("target page recovery: reconfirm run %s: %w", metadata.RunID, err)
		}
		if metadata.EffectiveOptions.GitLabURL != "" && metadata.EffectiveOptions.GitHubURL != "" {
			return snapshot.TargetPagePortfolio{}, nil, fmt.Errorf("target page recovery: run %s has conflicting source hosts", metadata.RunID)
		}
		runsByRef[metadata.AnalysisTargetRef] = targetPublishedRun{
			RunID:                 metadata.RunID,
			RunDir:                runDir,
			Target:                projection.Target,
			Authority:             authority,
			RepositoryStateSHA256: seed.RepositoryStateSHA256,
			SelectedRevision:      seed.SelectedRevision,
			GoTarget:              metadata.EffectiveOptions.GoTarget,
			SourceEpisodeJSON:     append([]byte(nil), sourceEpisodeJSON...),
			GitLabURL:             metadata.EffectiveOptions.GitLabURL,
			GitHubURL:             metadata.EffectiveOptions.GitHubURL,
		}
	}

	runs := make([]targetPublishedRun, 0, len(runsByRef))
	outcomes := make([]snapshot.TargetPageOutcome, 0, len(container.Targets))
	for _, projection := range container.Targets {
		run, ready := runsByRef[projection.Target.Ref]
		if portfolioFound {
			page := portfolio.Targets[len(outcomes)]
			if page.State == snapshot.TargetPageUnavailable {
				outcomes = append(outcomes, snapshot.TargetPageOutcome{
					TargetRef:       projection.Target.Ref,
					State:           snapshot.TargetPageUnavailable,
					UnavailableCode: snapshot.TargetPageUnavailableTargetRunFailed,
				})
				continue
			}
			if !ready || run.RunID != page.RunID {
				return snapshot.TargetPagePortfolio{}, nil, fmt.Errorf("target page recovery: sealed ready run is unavailable")
			}
		}
		if !ready {
			return snapshot.TargetPagePortfolio{}, nil, fmt.Errorf("target page recovery: selected target run is missing")
		}
		runs = append(runs, run)
		outcomes = append(outcomes, snapshot.TargetPageOutcome{
			TargetRef: projection.Target.Ref,
			State:     snapshot.TargetPageReady,
			RunID:     run.RunID,
		})
	}
	if _, ready := runsByRef[container.DefaultTargetRef]; !ready {
		return snapshot.TargetPagePortfolio{}, nil, fmt.Errorf("target page recovery: default target run is unavailable")
	}
	if !portfolioFound {
		portfolio, err = snapshot.BuildTargetPagePortfolio(container, outcomes)
		if err != nil {
			return snapshot.TargetPagePortfolio{}, nil, err
		}
	}
	return portfolio, runs, nil
}

func exactExistingTargetPageRunDir(runDir string) (string, error) {
	abs, err := filepath.Abs(strings.TrimSpace(runDir))
	if err != nil {
		return "", fmt.Errorf("target page recovery: resolve run directory: %w", err)
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return "", fmt.Errorf("target page recovery: inspect run directory: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 ||
		snapshot.ValidateTargetPageRunID(filepath.Base(abs)) != nil {
		return "", fmt.Errorf("target page recovery: run directory must be an existing real safe-ID directory")
	}
	return abs, nil
}
