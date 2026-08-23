package analysistarget

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/corpus"
)

// FileCandidate is the only value exchanged by the initial target scouts.
// A scout may think in any language-specific way it needs; the shared result
// only says which repository file it suspects and why.
type FileCandidate struct {
	FileRef    corpus.FileID `json:"file_ref"`
	Hypotheses []string      `json:"hypotheses"`
}

// MergeFileCandidates joins parallel scout results by exact corpus FileID.
// It performs no target grouping, path matching, ranking, or semantic
// interpretation: hypotheses for the same file are simply unioned.
func MergeFileCandidates(
	snapshot corpus.Snapshot,
	outputs ...[]FileCandidate,
) ([]FileCandidate, error) {
	owned, err := snapshot.Owned()
	if err != nil {
		return nil, fmt.Errorf("analysis target hypotheses: corpus: %w", err)
	}

	ordinal := make(map[corpus.FileID]int, len(owned.Entries))
	for index, entry := range owned.Entries {
		ordinal[entry.ID] = index
	}

	byFile := make(map[corpus.FileID]map[string]struct{})
	for outputIndex, output := range outputs {
		for candidateIndex, candidate := range output {
			if _, ok := ordinal[candidate.FileRef]; !ok {
				return nil, fmt.Errorf(
					"analysis target hypotheses: scout %d candidate %d cites unknown file_ref %q",
					outputIndex, candidateIndex, candidate.FileRef,
				)
			}
			if len(candidate.Hypotheses) == 0 {
				return nil, fmt.Errorf(
					"analysis target hypotheses: scout %d candidate %d has no hypotheses",
					outputIndex, candidateIndex,
				)
			}
			set := byFile[candidate.FileRef]
			if set == nil {
				set = make(map[string]struct{})
				byFile[candidate.FileRef] = set
			}
			for _, hypothesis := range candidate.Hypotheses {
				if err := validateFileHypothesis(hypothesis); err != nil {
					return nil, fmt.Errorf(
						"analysis target hypotheses: scout %d candidate %d: %w",
						outputIndex, candidateIndex, err,
					)
				}
				set[hypothesis] = struct{}{}
			}
		}
	}

	result := make([]FileCandidate, 0, len(byFile))
	for fileRef, set := range byFile {
		hypotheses := make([]string, 0, len(set))
		for hypothesis := range set {
			hypotheses = append(hypotheses, hypothesis)
		}
		sort.Strings(hypotheses)
		result = append(result, FileCandidate{
			FileRef: fileRef, Hypotheses: hypotheses,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		return ordinal[result[i].FileRef] < ordinal[result[j].FileRef]
	})
	if result == nil {
		return []FileCandidate{}, nil
	}
	return result, nil
}

func validateFileHypothesis(value string) error {
	if value == "" || value != strings.TrimSpace(value) || !utf8.ValidString(value) {
		return fmt.Errorf("invalid hypothesis")
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return fmt.Errorf("invalid hypothesis")
		}
	}
	return nil
}
