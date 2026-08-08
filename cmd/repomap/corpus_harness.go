package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
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
}

type corpusRunFacts struct {
	Repository            string               `json:"repository"`
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
	var facts []corpusRunFacts
	failed := 0
	for _, repo := range repos {
		runDir := latestCorpusRun(absRoot, repo.Repository)
		if runDir == "" {
			f := corpusRunFacts{
				Repository:         repo.Repository,
				PublicationStatus:  publicationFailed,
				PublicationReasons: []publicationReason{publicationReasonArtifactsInvalid},
				StageStates:        map[string]string{},
			}
			facts = append(facts, f)
			printCorpusFact(stdout, f)
			failed++
			continue
		}
		f := collectCorpusRunFacts(repo, runDir)
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
	fmt.Fprintf(stdout, "%s	%s	%s	report=%t	html=%t	arch=%s/%d	study=%s/%d	dup=%d	sec=%.0f	surf=%d	assoc=%d	entry_groups=%d	entry_handoffs=%d	reasons=%s\n",
		f.Repository, f.PublicationStatus, f.ArchOutcome, f.ReportPublished, f.ReportHTMLPublished,
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
		repos = append(repos, corpusRepo{Repository: name, Kind: kind})
	}
	sort.Slice(repos, func(i, j int) bool { return repos[i].Repository < repos[j].Repository })
	return repos, nil
}

// latestCorpusRun resolves the newest run directory for one repository,
// preferring a fresh timestamped run over runs/latest. The run-tree layout
// prefixes each repository with its kind (cli__ or service__).
func latestCorpusRun(root, repository string) string {
	rel := strings.ReplaceAll(repository, "/", "__")
	direct := filepath.Join(root, rel, "runs")
	if info, err := os.Stat(direct); err == nil && info.IsDir() {
		return newestRunIn(direct)
	}
	kind := "cli"
	serviceDir := filepath.Join(root, "service__"+rel, "runs")
	if info, err := os.Stat(serviceDir); err == nil && info.IsDir() {
		return newestRunIn(serviceDir)
	}
	cliDir := filepath.Join(root, kind+"__"+rel, "runs")
	return newestRunIn(cliDir)
}

func newestRunIn(runsDir string) string {
	entries, err := os.ReadDir(runsDir)
	if err != nil {
		return ""
	}
	var dirs []string
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), "20") {
			dirs = append(dirs, entry.Name())
		}
	}
	sort.Strings(dirs)
	if len(dirs) == 0 {
		return ""
	}
	return filepath.Join(runsDir, dirs[len(dirs)-1])
}

func collectCorpusRunFacts(repo corpusRepo, runDir string) corpusRunFacts {
	f := corpusRunFacts{
		Repository:  repo.Repository,
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
