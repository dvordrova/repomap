// Package opflows turns bounded deterministic source signals into operational
// orientation candidates. It does not infer runtime execution or ordering.
package opflows

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/sourcesignals"
)

const (
	maxCandidates         = 5
	maxSignalsPerCategory = 10
	maxOpenFiles          = 5
	maxEvidenceItems      = 3
	strongSignalWeight    = 35
)

var operationalCategories = []string{
	"background_loop",
	"admin_maintenance",
	"threshold_limit",
	"consensus_state",
	"storage_durability",
}

var operationalCategoryLabels = map[string]string{
	"background_loop":    "Background loop",
	"admin_maintenance":  "Administrative maintenance",
	"threshold_limit":    "Threshold enforcement",
	"consensus_state":    "Consensus state transition",
	"storage_durability": "Storage durability",
}

// Discover groups local source signals into bounded operational-flow
// candidates. Returned candidates are hypotheses grounded only in the supplied
// static signals; callers remain responsible for allowlisting their paths.
func Discover(
	signals []sourcesignals.Signal,
	entrypoints []gofacts.Entrypoint,
) ([]gofacts.OrientationCandidate, []string) {
	// Entrypoints are intentionally not assigned without static reachability
	// evidence. The parameter remains part of the approved discovery contract.
	_ = entrypoints
	groups := make(map[string][]sourcesignals.Signal, len(operationalCategories))
	warnings := make([]string, 0)
	for index, signal := range signals {
		normalized, ok := normalizeSignal(signal)
		if !ok {
			warnings = append(warnings, fmt.Sprintf(
				"operational flow discovery: skipped malformed source signal at index %d",
				index,
			))
			continue
		}
		if _, ok := operationalCategoryLabels[normalized.Category]; !ok {
			continue
		}
		groups[normalized.Category] = append(groups[normalized.Category], normalized)
	}

	candidates := make([]gofacts.OrientationCandidate, 0, len(operationalCategories))
	for _, category := range operationalCategories {
		categorySignals := strongestSignals(groups[category], maxSignalsPerCategory)
		if !meetsEvidenceThreshold(categorySignals) {
			continue
		}
		files := distinctFiles(categorySignals)
		priority := 2 + min((len(categorySignals)+1)/3, 3)
		if len(files) == 1 {
			priority--
		}
		candidates = append(candidates, gofacts.OrientationCandidate{
			Name:      candidateName(category, categorySignals[0]),
			Kind:      gofacts.OrientationKindSignalFlow,
			OpenFiles: files,
			Why:       candidateWhy(category, categorySignals),
			Priority:  priority,
		})
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Priority != candidates[j].Priority {
			return candidates[i].Priority > candidates[j].Priority
		}
		if candidates[i].Name != candidates[j].Name {
			return candidates[i].Name < candidates[j].Name
		}
		return candidates[i].Kind < candidates[j].Kind
	})
	if len(candidates) > maxCandidates {
		candidates = candidates[:maxCandidates]
	}
	return candidates, warnings
}

func normalizeSignal(signal sourcesignals.Signal) (sourcesignals.Signal, bool) {
	signal.Category = strings.TrimSpace(signal.Category)
	signal.Path = filepath.ToSlash(filepath.Clean(strings.TrimSpace(signal.Path)))
	hasEvidence := strings.TrimSpace(signal.Match) != "" ||
		strings.TrimSpace(signal.Snippet) != "" ||
		strings.TrimSpace(signal.Reason) != ""
	if signal.Category == "" || signal.Path == "." ||
		!filepath.IsLocal(filepath.FromSlash(signal.Path)) ||
		signal.Line <= 0 || signal.Weight <= 0 || !hasEvidence {
		return sourcesignals.Signal{}, false
	}
	return signal, true
}

func strongestSignals(signals []sourcesignals.Signal, limit int) []sourcesignals.Signal {
	result := append([]sourcesignals.Signal(nil), signals...)
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Weight != result[j].Weight {
			return result[i].Weight > result[j].Weight
		}
		if result[i].Penalty != result[j].Penalty {
			return result[i].Penalty < result[j].Penalty
		}
		if result[i].Path != result[j].Path {
			return result[i].Path < result[j].Path
		}
		return result[i].Line < result[j].Line
	})
	if len(result) > limit {
		result = result[:limit]
	}
	return result
}

func meetsEvidenceThreshold(signals []sourcesignals.Signal) bool {
	if len(signals) == 0 {
		return false
	}
	if signals[0].Weight >= strongSignalWeight {
		return true
	}
	return len(signals) >= 2 && len(distinctFiles(signals)) >= 2
}

func distinctFiles(signals []sourcesignals.Signal) []string {
	seen := make(map[string]struct{}, len(signals))
	files := make([]string, 0, min(len(signals), maxOpenFiles))
	for _, signal := range signals {
		if _, ok := seen[signal.Path]; ok {
			continue
		}
		seen[signal.Path] = struct{}{}
		files = append(files, signal.Path)
		if len(files) == maxOpenFiles {
			break
		}
	}
	return files
}

func candidateName(category string, signal sourcesignals.Signal) string {
	label := operationalCategoryLabels[category]
	detail := strings.TrimSpace(signal.Reason)
	if detail == "" {
		detail = strings.TrimSpace(signal.Match)
	}
	detail = strings.Join(strings.Fields(detail), " ")
	if detail == "" {
		return label
	}
	const maxRunes = 60
	runes := []rune(detail)
	if len(runes) > maxRunes {
		detail = string(runes[:maxRunes]) + "…"
	}
	return label + " — " + detail
}

func candidateWhy(category string, signals []sourcesignals.Signal) string {
	items := make([]string, 0, min(len(signals), maxEvidenceItems))
	for _, signal := range signals {
		items = append(items, compactEvidence(signal))
		if len(items) == maxEvidenceItems {
			break
		}
	}
	return fmt.Sprintf(
		"operational flow discovered from %d %s source signals across %d files: %s",
		len(signals),
		strings.ReplaceAll(category, "_", " "),
		len(distinctFiles(signals)),
		strings.Join(items, "; "),
	)
}

func compactEvidence(signal sourcesignals.Signal) string {
	value := strings.TrimSpace(signal.Match)
	if value == "" {
		value = strings.TrimSpace(signal.Reason)
	}
	if value == "" {
		return "matched static source pattern"
	}
	value = strings.Join(strings.Fields(value), " ")
	const maxRunes = 80
	runes := []rune(value)
	if len(runes) > maxRunes {
		value = string(runes[:maxRunes]) + "…"
	}
	return fmt.Sprintf("matched %q", value)
}
