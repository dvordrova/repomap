package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/dvordrova/repomap/internal/snapshot"
)

// Long-horizon program Phase 0: the 37-repo acceptance matrix as the release
// harness. `repomap dev corpus <root> [--matrix <json>]` reads every
// per-repository run directory and emits the acceptance matrix facts:
// revision, verified publication readiness, stage states, Architecture
// source/coverage, Study card counts, duplicate reading pairs, surface
// counts, provider accounting, and timing).

type corpusMatrix struct {
	Repositories []corpusRepo `json:"repositories"`
}

type corpusRepo struct {
	Repository string `json:"repository"`
	Kind       string `json:"kind"`
	Tier       string `json:"tier"`
	Archetype  string `json:"archetype"`
	RunID      string `json:"run_id,omitempty"`
}

type corpusRunFacts struct {
	Repository            string               `json:"repository"`
	RunID                 string               `json:"run_id"`
	PublicationStatus     publicationReadiness `json:"publication_status"`
	PublicationReasons    []publicationReason  `json:"publication_reasons,omitempty"`
	Seconds               float64              `json:"seconds"`
	Revision              string               `json:"revision"`
	ReportPublished       bool                 `json:"report_published"`
	ReportHTMLPublished   bool                 `json:"report_html_published"`
	ManifestPublished     bool                 `json:"manifest_published"`
	StageStates           map[string]string    `json:"stage_states"`
	ArchSource            string               `json:"architecture_source"`
	ArchOutcome           string               `json:"architecture_outcome"`
	ArchComponents        int                  `json:"architecture_components"`
	StudyState            string               `json:"study_state"`
	StudyCards            int                  `json:"study_cards"`
	DuplicateReadingPairs int                  `json:"duplicate_reading_pairs"`
	Surfaces              int                  `json:"surfaces"`
	Associations          int                  `json:"associations"`
	EntryHandoffGroups    int                  `json:"entry_handoff_groups"`
	EntryHandoffs         int                  `json:"entry_handoffs"`
	ProviderCalls         int                  `json:"provider_calls"`
	ProviderLatencyMS     int64                `json:"provider_latency_ms"`
	ExternalRequestBytes  int                  `json:"external_request_bytes"`
}

func runCorpusCLI(args []string, stdout io.Writer) error {
	root := ""
	matrixPath := ""
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--matrix":
			index++
			if index >= len(args) || args[index] == "" {
				return fmt.Errorf("corpus: --matrix requires a path")
			}
			matrixPath = args[index]
		default:
			if root == "" {
				root = args[index]
			} else {
				return fmt.Errorf("usage: repomap dev corpus <root> [--matrix <json>]")
			}
		}
	}
	if root == "" {
		return fmt.Errorf("usage: repomap dev corpus <root> [--matrix <json>]")
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("corpus: resolve root: %w", err)
	}
	matrix := corpusMatrix{}
	if matrixPath != "" {
		raw, err := os.ReadFile(matrixPath)
		if err != nil {
			return fmt.Errorf("corpus: read matrix: %w", err)
		}
		if err := json.Unmarshal(raw, &matrix); err != nil {
			return fmt.Errorf("corpus: decode matrix: %w", err)
		}
	}
	repos := matrix.Repositories
	if len(repos) == 0 {
		// Fallback: derive the corpus layout from the run directory names.
		repos, err = discoverCorpusRepos(absRoot)
		if err != nil {
			return err
		}
	}
	if len(repos) == 0 {
		return fmt.Errorf("corpus: no repositories found")
	}
	if err := validateCorpusRepoSet(repos); err != nil {
		return err
	}
	selectedRuns := make([]corpusRunSelection, len(repos))
	seenRunDirs := make(map[string]string, len(repos))
	for index, repo := range repos {
		selected, resolveErr := resolveCorpusRun(absRoot, repo)
		if errors.Is(resolveErr, errCorpusRunMissing) {
			continue
		}
		if resolveErr != nil {
			return resolveErr
		}
		if prior, duplicate := seenRunDirs[selected.RunDir]; duplicate {
			return fmt.Errorf(
				"corpus: repositories %q and %q resolve to the same run %q",
				prior, repo.Repository, selected.RunID,
			)
		}
		seenRunDirs[selected.RunDir] = repo.Repository
		selectedRuns[index] = selected
	}
	var facts []corpusRunFacts
	failed := 0
	for index, repo := range repos {
		selected := selectedRuns[index]
		if selected.RunDir == "" {
			f := corpusRunFacts{
				Repository:         repo.Repository,
				RunID:              repo.RunID,
				PublicationStatus:  publicationFailed,
				PublicationReasons: []publicationReason{publicationReasonArtifactsInvalid},
				StageStates:        map[string]string{},
			}
			facts = append(facts, f)
			printCorpusFact(stdout, f)
			failed++
			continue
		}
		f := collectCorpusRunFacts(repo, selected.RunDir)
		f.RunID = selected.RunID
		facts = append(facts, f)
		printCorpusFact(stdout, f)
		if f.PublicationStatus == publicationFailed {
			failed++
		}
	}
	if err := writeCorpusMatrix(absRoot, facts); err != nil {
		return err
	}
	if failed > 0 {
		return fmt.Errorf("corpus: %d publication(s) failed integrity", failed)
	}
	return nil
}

func printCorpusFact(stdout io.Writer, f corpusRunFacts) {
	reasons := make([]string, 0, len(f.PublicationReasons))
	for _, reason := range f.PublicationReasons {
		reasons = append(reasons, string(reason))
	}
	fmt.Fprintf(stdout, "%s	run=%s	%s	%s	report=%t	html=%t	arch=%s/%d	study=%s/%d	dup=%d	sec=%.0f	surf=%d	assoc=%d	entry_groups=%d	entry_handoffs=%d	reasons=%s\n",
		f.Repository, f.RunID, f.PublicationStatus, f.ArchOutcome, f.ReportPublished, f.ReportHTMLPublished,
		f.ArchSource, f.ArchComponents, f.StudyState, f.StudyCards,
		f.DuplicateReadingPairs, f.Seconds, f.Surfaces, f.Associations,
		f.EntryHandoffGroups, f.EntryHandoffs,
		strings.Join(reasons, ","))
}

func discoverCorpusRepos(root string) ([]corpusRepo, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("corpus: read root: %w", err)
	}
	var repos []corpusRepo
	for _, entry := range entries {
		if !entry.IsDir() || (!strings.HasPrefix(entry.Name(), "cli__") && !strings.HasPrefix(entry.Name(), "service__")) {
			continue
		}
		name := strings.ReplaceAll(entry.Name(), "__", "/")
		kind := "cli"
		if strings.HasPrefix(entry.Name(), "service__") {
			kind = "service"
		}
		repo := corpusRepo{Repository: name, Kind: kind}
		selected, resolveErr := resolveCorpusRun(root, repo)
		if resolveErr != nil {
			return nil, resolveErr
		}
		repo.RunID = selected.RunID
		repos = append(repos, repo)
	}
	sort.Slice(repos, func(i, j int) bool { return repos[i].Repository < repos[j].Repository })
	return repos, nil
}

var errCorpusRunMissing = errors.New("corpus run is missing")

type corpusRunSelection struct {
	RunID  string
	RunDir string
}

// latestCorpusRun is retained for internal callers that only have a legacy
// repository identity. "Latest" now means the exact runs/latest authority;
// it never scans timestamped siblings.
func latestCorpusRun(root, repository string) string {
	selected, err := resolveCorpusRun(root, corpusRepo{Repository: repository})
	if err != nil {
		return ""
	}
	return selected.RunDir
}

// resolveCorpusRun restores one exact launcher-owned run identity. New matrix
// rows carry run_id. A legacy row may use only the exact runs/latest symlink,
// which ordinary publication keeps on the selected default page. Timestamps
// are never used because a newer multi-target sibling is not the default run.
func resolveCorpusRun(root string, repo corpusRepo) (corpusRunSelection, error) {
	runsDir, err := corpusRunsDir(root, repo)
	if err != nil {
		if errors.Is(err, errCorpusRunMissing) && repo.RunID != "" {
			return corpusRunSelection{}, fmt.Errorf(
				"corpus: repository %q has no run root for exact run %q",
				repo.Repository, repo.RunID,
			)
		}
		return corpusRunSelection{}, err
	}
	runID := repo.RunID
	if runID == "" {
		runID, err = corpusLatestRunID(runsDir)
		if err != nil {
			return corpusRunSelection{}, err
		}
	}
	if err := snapshot.ValidateTargetPageRunID(runID); err != nil {
		return corpusRunSelection{}, fmt.Errorf(
			"corpus: repository %q has invalid run_id: %w",
			repo.Repository, err,
		)
	}
	runDir := filepath.Join(runsDir, runID)
	if err := validateConfinedCorpusRunDir(runsDir, runDir, runID); err != nil {
		return corpusRunSelection{}, fmt.Errorf(
			"corpus: repository %q exact run %q: %w",
			repo.Repository, runID, err,
		)
	}
	if err := validateCorpusRunMetadataIdentity(runDir, runID); err != nil {
		return corpusRunSelection{}, fmt.Errorf(
			"corpus: repository %q exact run %q: %w",
			repo.Repository, runID, err,
		)
	}
	return corpusRunSelection{RunID: runID, RunDir: runDir}, nil
}

func corpusRunsDir(root string, repo corpusRepo) (string, error) {
	if strings.TrimSpace(repo.Repository) == "" || repo.Repository != strings.TrimSpace(repo.Repository) {
		return "", fmt.Errorf("corpus: repository identity is empty or padded")
	}
	encoded := strings.ReplaceAll(repo.Repository, "/", "__")
	candidates := []string{filepath.Join(root, encoded, "runs")}
	if repo.Kind != "" {
		candidates = append(candidates, filepath.Join(root, repo.Kind+"__"+encoded, "runs"))
	} else {
		candidates = append(candidates,
			filepath.Join(root, "service__"+encoded, "runs"),
			filepath.Join(root, "cli__"+encoded, "runs"),
		)
	}
	seen := make(map[string]struct{}, len(candidates))
	found := make([]string, 0, 1)
	for _, candidate := range candidates {
		abs, err := filepath.Abs(candidate)
		if err != nil {
			return "", fmt.Errorf("corpus: resolve repository run root: %w", err)
		}
		if _, duplicate := seen[abs]; duplicate {
			continue
		}
		seen[abs] = struct{}{}
		if err := validateCorpusRunsRoot(root, abs); err != nil {
			return "", fmt.Errorf("corpus: repository %q run root: %w", repo.Repository, err)
		}
		info, err := os.Lstat(abs)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("corpus: inspect repository %q run root: %w", repo.Repository, err)
		}
		if !info.IsDir() {
			return "", fmt.Errorf("corpus: repository %q run root is not a directory", repo.Repository)
		}
		found = append(found, abs)
	}
	if len(found) == 0 {
		return "", errCorpusRunMissing
	}
	if len(found) != 1 {
		return "", fmt.Errorf("corpus: repository %q has ambiguous run roots", repo.Repository)
	}
	return found[0], nil
}

func corpusLatestRunID(runsDir string) (string, error) {
	latest := filepath.Join(runsDir, "latest")
	info, err := os.Lstat(latest)
	if errors.Is(err, os.ErrNotExist) {
		return "", errCorpusRunMissing
	}
	if err != nil {
		return "", fmt.Errorf("corpus: inspect runs/latest: %w", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return "", fmt.Errorf("corpus: runs/latest is not an exact run symlink")
	}
	runID, err := os.Readlink(latest)
	if err != nil {
		return "", fmt.Errorf("corpus: read runs/latest: %w", err)
	}
	if filepath.IsAbs(runID) || filepath.Base(runID) != runID {
		return "", fmt.Errorf("corpus: runs/latest does not name one confined run id")
	}
	if err := snapshot.ValidateTargetPageRunID(runID); err != nil {
		return "", fmt.Errorf("corpus: runs/latest has invalid run id: %w", err)
	}
	return runID, nil
}

func validateCorpusRunsRoot(root, runsDir string) error {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(absRoot, runsDir)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path escapes corpus root")
	}
	return nil
}

func validateConfinedCorpusRunDir(runsDir, runDir, runID string) error {
	rel, err := filepath.Rel(runsDir, runDir)
	if err != nil || rel != runID || filepath.Base(rel) != rel {
		return fmt.Errorf("run path escapes repository run root")
	}
	info, err := os.Lstat(runDir)
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("run id is unknown")
	}
	if err != nil {
		return fmt.Errorf("inspect run directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("run path is not a regular directory")
	}
	return nil
}

func validateCorpusRunMetadataIdentity(runDir, runID string) error {
	raw, err := os.ReadFile(filepath.Join(runDir, "metadata.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read metadata identity: %w", err)
	}
	var metadata struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return fmt.Errorf("decode metadata identity: %w", err)
	}
	if metadata.RunID != "" && metadata.RunID != runID {
		return fmt.Errorf("metadata run_id does not match directory")
	}
	return nil
}

func validateCorpusRepoSet(repos []corpusRepo) error {
	seen := make(map[string]struct{}, len(repos))
	for _, repo := range repos {
		if _, duplicate := seen[repo.Repository]; duplicate {
			return fmt.Errorf("corpus: duplicate repository %q", repo.Repository)
		}
		seen[repo.Repository] = struct{}{}
	}
	return nil
}

func collectCorpusRunFacts(repo corpusRepo, runDir string) corpusRunFacts {
	f := corpusRunFacts{
		Repository:  repo.Repository,
		RunID:       filepath.Base(runDir),
		StageStates: map[string]string{},
	}
	metadataPath := filepath.Join(runDir, "metadata.json")
	var createdAt time.Time
	if raw, err := os.ReadFile(metadataPath); err == nil {
		var meta struct {
			RepoName        string `json:"repo_name"`
			ProviderCalls   int    `json:"provider_request_count"`
			ProviderLatency int64  `json:"provider_latency_ms"`
			ExternalBytes   int    `json:"external_request_bytes"`
			CreatedAt       string `json:"created_at"`
			Attempts        []struct {
				Stage string `json:"stage"`
				State string `json:"state"`
			} `json:"request_attempts"`
		}
		if json.Unmarshal(raw, &meta) == nil {
			f.ProviderCalls = meta.ProviderCalls
			f.ProviderLatencyMS = meta.ProviderLatency
			f.ExternalRequestBytes = meta.ExternalBytes
			for _, attempt := range meta.Attempts {
				f.StageStates[attempt.Stage] = attempt.State
			}
			if meta.CreatedAt != "" {
				createdAt, _ = time.Parse(time.RFC3339, meta.CreatedAt)
			}
		}
	}
	reportPath := filepath.Join(runDir, "report.json")
	if info, err := os.Lstat(filepath.Join(runDir, "report.html")); err == nil &&
		info.Mode().IsRegular() && info.Size() > 0 {
		f.ReportHTMLPublished = true
	}
	if info, err := os.Stat(reportPath); err == nil && !createdAt.IsZero() {
		f.Seconds = info.ModTime().Sub(createdAt).Seconds()
	}
	if raw, err := os.ReadFile(reportPath); err == nil {
		f.ReportPublished = true
		var report struct {
			CapturedRevision string `json:"captured_revision"`
			Architecture     struct {
				Source             string `json:"architecture_source"`
				Outcome            string `json:"validation_outcome"`
				Components         []any  `json:"components"`
				EntryHandoffGroups []struct {
					EntryHandoffs []any `json:"entry_handoffs"`
				} `json:"entry_handoff_groups"`
			} `json:"architecture_canvas"`
			Study struct {
				State  string `json:"state"`
				Themes struct {
					Cards []struct {
						Readings []struct {
							Path   string `json:"path"`
							Line   int    `json:"line"`
							Symbol string `json:"symbol"`
						} `json:"readings"`
					} `json:"cards"`
				} `json:"themes"`
			} `json:"atlas_study"`
			DiscoveredSurfaces struct {
				TotalCount int `json:"total_count"`
			} `json:"discovered_surfaces"`
			Associations struct {
				Total int `json:"total"`
			} `json:"architecture_associations"`
		}
		if err := json.Unmarshal(raw, &report); err != nil {
			f.Revision = "unmarshal-error: " + err.Error()
		} else {
			f.Revision = report.CapturedRevision
			f.ArchSource = report.Architecture.Source
			f.ArchOutcome = report.Architecture.Outcome
			f.ArchComponents = len(report.Architecture.Components)
			f.StudyState = report.Study.State
			f.StudyCards = len(report.Study.Themes.Cards)
			f.Surfaces = report.DiscoveredSurfaces.TotalCount
			f.Associations = report.Associations.Total
			f.EntryHandoffGroups = len(report.Architecture.EntryHandoffGroups)
			for _, group := range report.Architecture.EntryHandoffGroups {
				f.EntryHandoffs += len(group.EntryHandoffs)
			}
			f.DuplicateReadingPairs = countDuplicateReadingPairs(report.Study.Themes.Cards)
		}
	}
	if _, err := os.Stat(filepath.Join(runDir, "run_manifest.json")); err == nil {
		f.ManifestPublished = true
	}
	publication, _ := assessRunPublication(runDir)
	f.PublicationStatus = publication.Status
	f.PublicationReasons = publication.Reasons
	return f
}

// countDuplicateReadingPairs counts pairs of cards whose exact public
// reading sets (path,line,symbol) are identical.
func countDuplicateReadingPairs(cards []struct {
	Readings []struct {
		Path   string `json:"path"`
		Line   int    `json:"line"`
		Symbol string `json:"symbol"`
	} `json:"readings"`
}) int {
	seen := map[string]bool{}
	duplicates := 0
	for _, card := range cards {
		var parts []string
		for _, reading := range card.Readings {
			parts = append(parts, fmt.Sprintf("%s:%d:%s", reading.Path, reading.Line, reading.Symbol))
		}
		sort.Strings(parts)
		key := strings.Join(parts, "|")
		if len(parts) == 0 {
			continue
		}
		if seen[key] {
			duplicates++
		}
		seen[key] = true
	}
	return duplicates
}

func writeCorpusMatrix(root string, facts []corpusRunFacts) error {
	outPath := filepath.Join(root, "corpus_acceptance.json")
	raw, err := json.MarshalIndent(facts, "", "  ")
	if err != nil {
		return fmt.Errorf("corpus: encode acceptance matrix: %w", err)
	}
	if err := os.WriteFile(outPath, raw, 0o600); err != nil {
		return fmt.Errorf("corpus: write acceptance matrix: %w", err)
	}
	return nil
}
