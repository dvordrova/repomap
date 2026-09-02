package readmetargetscout

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/dvordrova/repomap/internal/corpus"
)

// batches returns a deterministic exhaustive provider-request cover. Guidance
// documents are grouped only when their complete bytes fit together. Every
// guidance group is then paired with a complete disjoint cover of the global
// file authority, so no tracked file or guidance byte disappears at a request
// boundary. Guidance refs are repeated as request-local authority where needed.
func batches(compilation Compilation) ([]Compilation, error) {
	if err := validateReadyCompilation(compilation); err != nil {
		return nil, err
	}
	documentGroups, err := guidanceGroups(compilation)
	if err != nil {
		return nil, err
	}
	refs := canonicalAuthorityRefs(compilation.authority)
	batches := make([]Compilation, 0)
	for _, documents := range documentGroups {
		start := 0
		for start < len(refs) {
			end, batch, err := largestFileWindow(compilation, documents, refs, start)
			if err != nil {
				return nil, err
			}
			if end <= start {
				return nil, fmt.Errorf("README file classifier: bounded file shard made no progress")
			}
			batches = append(batches, batch)
			start = end
		}
	}
	if len(batches) == 0 {
		return nil, fmt.Errorf("README file classifier: exhaustive batching produced no requests")
	}
	return batches, nil
}

func guidanceGroups(compilation Compilation) ([][]RequestGuidanceDocument, error) {
	documents := compilation.Request.GuidanceDocuments
	groups := make([][]RequestGuidanceDocument, 0)
	for start := 0; start < len(documents); {
		low, high := start+1, len(documents)
		best := start
		for low <= high {
			middle := low + (high-low)/2
			candidate, err := compileBatchSubset(compilation, documents[start:middle], nil)
			if err != nil {
				return nil, err
			}
			if len(candidate.wire) <= MaxRequestBytes {
				best = middle
				low = middle + 1
			} else {
				high = middle - 1
			}
		}
		if best == start {
			candidate, err := compileBatchSubset(compilation, documents[start:start+1], nil)
			if err != nil {
				return nil, err
			}
			// Keep the complete document as a singleton. Run checks the exact
			// prepared request against the shared semantic-record envelope.
			_ = candidate
			best = start + 1
		}
		groups = append(groups, append([]RequestGuidanceDocument(nil), documents[start:best]...))
		start = best
	}
	return groups, nil
}

func largestFileWindow(
	compilation Compilation,
	documents []RequestGuidanceDocument,
	refs []corpus.FileID,
	start int,
) (int, Compilation, error) {
	low, high := start+1, len(refs)
	best := start
	var bestBatch Compilation
	for low <= high {
		middle := low + (high-low)/2
		candidate, err := compileBatchSubset(compilation, documents, refs[start:middle])
		if err != nil {
			return start, Compilation{}, err
		}
		if len(candidate.wire) <= MaxRequestBytes {
			best = middle
			bestBatch = candidate
			low = middle + 1
		} else {
			high = middle - 1
		}
	}
	if best == start {
		candidate, err := compileBatchSubset(compilation, documents, refs[start:start+1])
		if err != nil {
			return start, Compilation{}, err
		}
		return start + 1, candidate, nil
	}
	return best, bestBatch, nil
}

func compileBatchSubset(
	aggregate Compilation,
	documents []RequestGuidanceDocument,
	fileRefs []corpus.FileID,
) (Compilation, error) {
	authority := make(map[corpus.FileID]string, len(fileRefs)+len(documents))
	for _, ref := range fileRefs {
		filePath, known := aggregate.authority[ref]
		if !known {
			return Compilation{}, fmt.Errorf("README file classifier: batch cites unknown file authority")
		}
		authority[ref] = filePath
	}
	for _, document := range documents {
		filePath, known := aggregate.authority[document.FileRef]
		if !known || filePath != document.Path {
			return Compilation{}, fmt.Errorf("README file classifier: batch guidance authority mismatch")
		}
		authority[document.FileRef] = filePath
	}
	entries := make([]corpus.Entry, 0, len(authority))
	for fileRef, filePath := range authority {
		entries = append(entries, corpus.Entry{ID: fileRef, Path: filePath})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	fileTree, err := buildFileTree(entries)
	if err != nil {
		return Compilation{}, fmt.Errorf("README file classifier: build batch file tree: %w", err)
	}
	request := Request{
		RepoName: aggregate.Request.RepoName, FileCount: len(entries), FileTree: fileTree,
		ProseFileRefs:     canonicalProseFileRefs(authority),
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
