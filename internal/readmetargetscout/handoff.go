package readmetargetscout

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/corpus"
)

// SnapshotAgainstCorpus validates that result is an accepted, canonical role
// catalog for repository and returns an independently owned copy. It resolves
// only sealed corpus metadata; it never reads repository file contents.
func (result Result) SnapshotAgainstCorpus(repository *corpus.Corpus) (Result, error) {
	if repository == nil {
		return nil, fmt.Errorf("README file classifier: repository corpus is required for result handoff")
	}
	if err := repository.Snapshot().Validate(); err != nil {
		return nil, fmt.Errorf("README file classifier: result handoff corpus: %w", err)
	}
	if len(result) > MaxClassifiedFiles {
		return nil, fmt.Errorf("README file classifier: result handoff contains too many classified files")
	}

	owned := make(Result, len(result))
	previousPath := ""
	for fileIndex, file := range result {
		info, known := repository.Info(file.FileRef)
		if file.FileRef == "" || !known {
			return nil, fmt.Errorf("README file classifier: result handoff cites unknown file_ref")
		}
		if fileIndex > 0 && info.Entry.Path <= previousPath {
			return nil, fmt.Errorf("README file classifier: result handoff files are not in canonical order")
		}
		previousPath = info.Entry.Path
		if len(file.Classifications) == 0 || len(file.Classifications) > MaxClassificationsPerFile {
			return nil, fmt.Errorf("README file classifier: result handoff has invalid classifications")
		}

		classifications := make([]Classification, len(file.Classifications))
		var previousClass FileClass
		for classIndex, classification := range file.Classifications {
			if !validFileClass(classification.Class) {
				return nil, fmt.Errorf("README file classifier: result handoff contains unknown file class")
			}
			if classIndex > 0 && classification.Class <= previousClass {
				return nil, fmt.Errorf("README file classifier: result handoff classes are not in canonical order")
			}
			previousClass = classification.Class
			if isProseEvidencePath(info.Entry.Path) && classification.Class != ClassDocumentation {
				return nil, fmt.Errorf("README file classifier: result handoff retained a non-documentation prose role")
			}
			if len(classification.Hypotheses) == 0 ||
				len(classification.Hypotheses) > MaxHypothesesPerClassification {
				return nil, fmt.Errorf("README file classifier: result handoff has invalid hypotheses")
			}

			hypotheses := append([]string(nil), classification.Hypotheses...)
			for hypothesisIndex, hypothesis := range hypotheses {
				if hypothesis == "" || hypothesis != strings.TrimSpace(hypothesis) ||
					len(hypothesis) > MaxHypothesisBytes || !utf8.ValidString(hypothesis) ||
					containsControl(hypothesis) {
					return nil, fmt.Errorf("README file classifier: result handoff contains an invalid hypothesis")
				}
				if hypothesisIndex > 0 && hypothesis <= hypotheses[hypothesisIndex-1] {
					return nil, fmt.Errorf("README file classifier: result handoff hypotheses are not in canonical order")
				}
			}
			classifications[classIndex] = Classification{
				Class: classification.Class, Hypotheses: hypotheses,
			}
		}
		owned[fileIndex] = ClassifiedFile{
			FileRef: file.FileRef, Classifications: classifications,
		}
	}
	return owned, nil
}
