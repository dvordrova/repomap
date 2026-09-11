package readmetargetscout

import (
	"encoding/json"
	"fmt"
	"sort"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/corpus"
)

// GuidanceDocument is one exact repository-authored document captured by the
// first-layer compilation. Corpus FileIDs deliberately stop at the classifier
// boundary: later domain cubes receive fresh request-local refs and need only
// the canonical path, kind, and already-read bytes.
type GuidanceDocument struct {
	Path    string       `json:"path"`
	Kind    GuidanceKind `json:"kind"`
	Content string       `json:"content"`
}

// GuidanceSnapshot is the independently owned, transaction-local handoff of
// the complete guidance bytes already read by Compile. SHA256 binds only the
// canonical path/kind/content rows, not the classifier's unrelated file tree.
// The snapshot is not a persisted artifact.
type GuidanceSnapshot struct {
	SHA256    string             `json:"sha256"`
	Documents []GuidanceDocument `json:"documents"`
}

// GuidanceSnapshot returns an independently owned documents-only snapshot
// from one sealed ready compilation. It never rereads the repository.
func (compilation Compilation) GuidanceSnapshot() (GuidanceSnapshot, error) {
	if err := validateReadyCompilation(compilation); err != nil {
		return GuidanceSnapshot{}, err
	}
	documents := make([]GuidanceDocument, 0, len(compilation.Request.GuidanceDocuments))
	for _, document := range compilation.Request.GuidanceDocuments {
		documents = append(documents, GuidanceDocument{
			Path: document.Path, Kind: document.Kind, Content: document.Content,
		})
	}
	sort.Slice(documents, func(i, j int) bool {
		if documents[i].Path != documents[j].Path {
			return documents[i].Path < documents[j].Path
		}
		return documents[i].Kind < documents[j].Kind
	})
	digest, err := guidanceSnapshotDigest(documents)
	if err != nil {
		return GuidanceSnapshot{}, err
	}
	snapshot := GuidanceSnapshot{SHA256: digest, Documents: documents}
	if err := snapshot.Validate(); err != nil {
		return GuidanceSnapshot{}, err
	}
	return snapshot, nil
}

// Snapshot validates the handoff and returns an independently owned copy.
func (snapshot GuidanceSnapshot) Snapshot() (GuidanceSnapshot, error) {
	if err := snapshot.Validate(); err != nil {
		return GuidanceSnapshot{}, err
	}
	return GuidanceSnapshot{
		SHA256:    snapshot.SHA256,
		Documents: append([]GuidanceDocument(nil), snapshot.Documents...),
	}, nil
}

// Validate accepts the zero value as the no-guidance transaction state.
func (snapshot GuidanceSnapshot) Validate() error {
	if len(snapshot.Documents) == 0 {
		if snapshot.SHA256 != "" {
			return fmt.Errorf("README file classifier: empty guidance snapshot has a digest")
		}
		return nil
	}
	if snapshot.SHA256 == "" {
		return fmt.Errorf("README file classifier: guidance snapshot digest is missing")
	}
	previousPath := ""
	for index, document := range snapshot.Documents {
		kind, known := guidanceKind(document.Path)
		if validateRepoPath(document.Path) != nil || !known || kind != document.Kind ||
			!utf8.ValidString(document.Content) {
			return fmt.Errorf("README file classifier: guidance snapshot document %d is invalid", index)
		}
		if index > 0 && document.Path <= previousPath {
			return fmt.Errorf("README file classifier: guidance snapshot documents are not canonical")
		}
		previousPath = document.Path
	}
	want, err := guidanceSnapshotDigest(snapshot.Documents)
	if err != nil {
		return err
	}
	if snapshot.SHA256 != want {
		return fmt.Errorf("README file classifier: guidance snapshot digest mismatch")
	}
	return nil
}

func guidanceSnapshotDigest(documents []GuidanceDocument) (string, error) {
	raw, err := json.Marshal(documents)
	if err != nil {
		return "", fmt.Errorf("README file classifier: encode guidance snapshot: %w", err)
	}
	return sha256Hex(raw), nil
}

// SnapshotAgainstCorpus validates that result is an accepted, canonical entry
// catalog for repository and returns an independently owned copy. It resolves
// only sealed corpus metadata; it never reads repository file contents.
func (result Result) SnapshotAgainstCorpus(repository *corpus.Corpus) (Result, error) {
	if repository == nil {
		return nil, fmt.Errorf("README file classifier: repository corpus is required for result handoff")
	}
	if err := repository.Snapshot().Validate(); err != nil {
		return nil, fmt.Errorf("README file classifier: result handoff corpus: %w", err)
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
		if !isCandidateEntryPath(info.Entry.Path) {
			return nil, fmt.Errorf("README file classifier: result handoff retained a non-candidate file")
		}
		if len(file.Classifications) != 1 || !validFileClass(file.Classifications[0].Class) {
			return nil, fmt.Errorf("README file classifier: result handoff has invalid classifications")
		}
		hypotheses := append([]string(nil), file.Classifications[0].Hypotheses...)
		if len(hypotheses) == 0 {
			return nil, fmt.Errorf("README file classifier: result handoff has invalid hypotheses")
		}
		for hypothesisIndex, hypothesis := range hypotheses {
			if !validHypothesis(hypothesis) {
				return nil, fmt.Errorf("README file classifier: result handoff contains an invalid hypothesis")
			}
			if hypothesisIndex > 0 && hypothesis <= hypotheses[hypothesisIndex-1] {
				return nil, fmt.Errorf("README file classifier: result handoff hypotheses are not in canonical order")
			}
		}
		owned[fileIndex] = ClassifiedFile{
			FileRef:         file.FileRef,
			Classifications: []Classification{{Class: ClassTargetEntry, Hypotheses: hypotheses}},
		}
	}
	return owned, nil
}
