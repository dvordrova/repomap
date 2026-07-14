package orient

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/flowexplain"
	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/opflows"
	"github.com/dvordrova/repomap/internal/snapshot"
	"github.com/dvordrova/repomap/internal/sourcesignals"
)

const maxMergedOrientationCandidates = 20

func collectOrientationSignals(s snapshot.Snapshot, opts Options) []sourcesignals.Signal {
	signals := sourcesignals.ScanFiles(
		s.FilteredFiles,
		opts.RepoPath,
		sourcesignals.ScanOptions{
			MaxPerFile: opts.MaxLLMSignalsPerFile,
			MaxTotal:   opts.MaxLLMSignals,
		},
	)
	if signals == nil {
		return []sourcesignals.Signal{}
	}
	return signals
}

func discoverOperationalCandidates(
	s *snapshot.Snapshot,
	signals []sourcesignals.Signal,
) []string {
	if s == nil || s.GoFacts == nil {
		return nil
	}
	candidates, warnings := opflows.Discover(signals, s.GoFacts.EntrypointPackages)
	s.GoFacts.OrientationCandidates = mergeOrientationCandidates(
		s.GoFacts.OrientationCandidates,
		candidates,
	)
	return warnings
}

func mergeOrientationCandidates(
	existing []gofacts.OrientationCandidate,
	operational []gofacts.OrientationCandidate,
) []gofacts.OrientationCandidate {
	merged := make([]gofacts.OrientationCandidate, 0, len(existing)+len(operational))
	seen := make(map[string]struct{}, len(existing)+len(operational))
	for _, candidates := range [][]gofacts.OrientationCandidate{existing, operational} {
		for _, candidate := range candidates {
			key := candidate.Kind + "\x00" + candidate.Name + "\x00" + candidate.EntrypointPackage
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			merged = append(merged, candidate)
		}
	}
	sort.SliceStable(merged, func(i, j int) bool {
		if merged[i].Priority != merged[j].Priority {
			return merged[i].Priority > merged[j].Priority
		}
		if merged[i].Kind != merged[j].Kind {
			return merged[i].Kind < merged[j].Kind
		}
		return merged[i].Name < merged[j].Name
	})
	if len(merged) > maxMergedOrientationCandidates {
		merged = merged[:maxMergedOrientationCandidates]
	}
	return merged
}

func mergeOperationalCandidateFlows(
	report *orientationPart,
	candidates []gofacts.OrientationCandidate,
	signals []sourcesignals.Signal,
) {
	if report == nil {
		return
	}
	existing := make(map[string]int, len(report.CandidateFlows))
	for index, flow := range report.CandidateFlows {
		existing[strings.ToLower(strings.TrimSpace(flow.Name))] = index
	}
	for _, candidate := range candidates {
		if candidate.Kind != gofacts.OrientationKindSignalFlow {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(candidate.Name))
		if index, ok := existing[key]; ok {
			flow := &report.CandidateFlows[index]
			flow.FlowType = flowexplain.FlowTypeOperational
			for _, statement := range operationalCandidateEvidence(candidate, signals) {
				if !containsOperationalValue(flow.Evidence, statement) {
					flow.Evidence = append(flow.Evidence, statement)
				}
			}
			continue
		}
		evidence := operationalCandidateEvidence(candidate, signals)
		if len(evidence) == 0 {
			continue
		}
		report.CandidateFlows = append(report.CandidateFlows, flowexplain.CandidateFlow{
			Name:             candidate.Name,
			FlowType:         flowexplain.FlowTypeOperational,
			Trigger:          "local static source-signal threshold was met",
			LikelyEntrypoint: candidate.EntrypointPackage,
			LikelyFiles:      append([]string(nil), candidate.OpenFiles...),
			WhyInteresting:   candidate.Why,
			Evidence:         evidence,
			Confidence:       0.3,
			CandidateBasis:   flowexplain.CandidateBasisSourceSignalAggregate,
		})
		existing[key] = len(report.CandidateFlows) - 1
	}
}

func operationalCandidateEvidence(
	candidate gofacts.OrientationCandidate,
	signals []sourcesignals.Signal,
) []string {
	const maxEvidence = 3
	evidence := make([]string, 0, maxEvidence)
	for _, path := range candidate.OpenFiles {
		for _, signal := range signals {
			if signal.Path != path || !isOperationalSignalCategory(signal.Category) {
				continue
			}
			detail := strings.TrimSpace(signal.Reason)
			if detail == "" {
				detail = strings.ReplaceAll(signal.Category, "_", " ")
			}
			evidence = append(evidence, fmt.Sprintf(
				"%s:%d source_signal %s",
				signal.Path,
				signal.Line,
				detail,
			))
			break
		}
		if len(evidence) == maxEvidence {
			break
		}
	}
	return evidence
}

func isOperationalSignalCategory(category string) bool {
	switch category {
	case "background_loop",
		"admin_maintenance",
		"threshold_limit",
		"consensus_state",
		"storage_durability":
		return true
	default:
		return false
	}
}

func containsOperationalValue(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
