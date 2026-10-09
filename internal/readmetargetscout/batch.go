package readmetargetscout

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/dvordrova/repomap/internal/corpus"
)

// guidanceBatch is a rectangle of the original document-by-file authority.
// Splitting one axis preserves every pair exactly once; a complete document
// and a candidate leaf are indivisible at this owning boundary.
type guidanceBatch struct {
	compilation Compilation
	err         error
}

func splitGuidanceBatch(item guidanceBatch) (guidanceBatch, guidanceBatch, bool) {
	batch := item.compilation
	documents := batch.Request.GuidanceDocuments
	refs := canonicalAuthorityRefs(batch.authority)
	if len(refs) <= 1 && len(documents) <= 1 {
		return guidanceBatch{}, guidanceBatch{}, false
	}
	// Divide the larger complete encoded axis. Otherwise a shared oversized
	// document catalogue could be repeated for every singleton file before it
	// is finally divided. This is a planning choice only after actual refusal.
	documentBytes, err := json.Marshal(documents)
	if err != nil {
		return guidanceBatch{err: err}, guidanceBatch{err: err}, true
	}
	treeBytes, err := json.Marshal(batch.Request.FileTree)
	if err != nil {
		return guidanceBatch{err: err}, guidanceBatch{err: err}, true
	}
	leftDocs, rightDocs := documents, documents
	leftRefs, rightRefs := refs, refs
	if len(documents) > 1 && (len(refs) <= 1 || len(documentBytes) > len(treeBytes)) {
		at := len(documents) / 2
		leftDocs, rightDocs = documents[:at], documents[at:]
	} else {
		at := len(refs) / 2
		leftRefs, rightRefs = refs[:at], refs[at:]
	}
	left, leftErr := compileBatchSubset(batch, leftDocs, leftRefs)
	right, rightErr := compileBatchSubset(batch, rightDocs, rightRefs)
	return guidanceBatch{left, leftErr}, guidanceBatch{right, rightErr}, true
}

func compileBatchSubset(
	aggregate Compilation,
	documents []RequestGuidanceDocument,
	fileRefs []corpus.FileID,
) (Compilation, error) {
	authority := make(map[corpus.FileID]string, len(fileRefs))
	for _, ref := range fileRefs {
		filePath, known := aggregate.authority[ref]
		if !known {
			return Compilation{}, fmt.Errorf("README file classifier: batch cites unknown file authority")
		}
		authority[ref] = filePath
	}
	if len(authority) == 0 {
		return Compilation{}, fmt.Errorf("README file classifier: batch has no candidate files")
	}
	knownDocuments := make(map[corpus.FileID]string, len(aggregate.Request.GuidanceDocuments))
	for _, document := range aggregate.Request.GuidanceDocuments {
		knownDocuments[document.FileRef] = document.Path
	}
	for _, document := range documents {
		if filePath, known := knownDocuments[document.FileRef]; !known || filePath != document.Path {
			return Compilation{}, fmt.Errorf("README file classifier: batch guidance authority mismatch")
		}
	}
	fileTree, err := buildFileTree(authorityEntries(authority))
	if err != nil {
		return Compilation{}, fmt.Errorf("README file classifier: build batch file tree: %w", err)
	}
	request := Request{
		RepoName: aggregate.Request.RepoName, FileCount: len(authority), FileTree: fileTree,
		GuidanceDocuments: append([]RequestGuidanceDocument(nil), documents...),
	}
	wire, err := json.Marshal(request)
	if err != nil {
		return Compilation{}, fmt.Errorf("README file classifier: encode batch request: %w", err)
	}
	batch := Compilation{
		Version: CompilationVersion, State: StateReady, Request: request,
		RequestSHA256: sha256Hex(wire), wire: append([]byte(nil), wire...),
		authority: authority, corpusRef: aggregate.corpusRef,
	}
	batch.seal = compilationSeal(batch)
	if err := validateReadyCompilation(batch); err != nil {
		return Compilation{}, err
	}
	return batch, nil
}

func canonicalAuthorityRefs(authority map[corpus.FileID]string) []corpus.FileID {
	refs := make([]corpus.FileID, 0, len(authority))
	for ref := range authority {
		refs = append(refs, ref)
	}
	sort.Slice(refs, func(i, j int) bool {
		if authority[refs[i]] != authority[refs[j]] {
			return authority[refs[i]] < authority[refs[j]]
		}
		return refs[i] < refs[j]
	})
	return refs
}
