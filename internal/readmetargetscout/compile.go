package readmetargetscout

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"reflect"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/corpus"
)

const preparationContract = "one atomic request; complete canonical corpus FileID-to-path authority encoded as a lossless path-component tree with FileID string leaves; complete current bytes of every tracked regular README and AGENTS.md guidance document; all other source bytes excluded; no semantic filtering, truncation, chunking, or partial result-v5"

// HasGuidanceFiles is the cheap metadata-only applicability check. Compile
// repeats the authoritative check while building its exact request.
func HasGuidanceFiles(repository *corpus.Corpus) bool {
	if repository == nil {
		return false
	}
	for _, entry := range repository.Entries() {
		if _, ok := guidanceKind(entry.Path); ok {
			return true
		}
	}
	return false
}

// Compile builds one atomic model request. If complete evidence cannot fit the
// explicit provider-safe envelope, it fails before provider execution with no
// partial request or semantic result.
func Compile(
	repoName string,
	repository *corpus.Corpus,
) (Compilation, error) {
	if err := validateRepoName(repoName); err != nil {
		return Compilation{}, err
	}
	if repository == nil {
		return Compilation{}, fmt.Errorf("readme target scout: repository corpus is required")
	}
	snapshot := repository.Snapshot()
	if err := snapshot.Validate(); err != nil {
		return Compilation{}, fmt.Errorf("readme target scout: repository corpus: %w", err)
	}
	authority := make(map[corpus.FileID]string, len(snapshot.Entries))
	documents := make([]RequestGuidanceDocument, 0)
	for _, entry := range snapshot.Entries {
		authority[entry.ID] = entry.Path
		kind, ok := guidanceKind(entry.Path)
		if !ok {
			continue
		}
		content, err := repository.ReadFile(entry.ID, corpus.MaxReadBytes)
		if err != nil {
			return Compilation{}, fmt.Errorf("readme target scout: read complete repository guidance %s: %w", entry.ID, err)
		}
		if content.Truncated {
			return Compilation{}, fmt.Errorf(
				"readme target scout: repository guidance %q exceeds the %d-byte complete-read limit; no provider request was made because partial guidance analysis is forbidden",
				entry.Path, corpus.MaxReadBytes,
			)
		}
		if !utf8.Valid(content.Bytes) {
			return Compilation{}, fmt.Errorf("readme target scout: repository guidance %q is not valid UTF-8; no provider request was made", entry.Path)
		}
		documents = append(documents, RequestGuidanceDocument{
			FileRef: entry.ID, Path: entry.Path, Kind: kind, Content: string(content.Bytes),
		})
	}
	if len(documents) == 0 {
		compilation := Compilation{
			Version: CompilationVersion, State: StateNotApplicable,
			Reason: NoGuidanceFiles, corpusRef: repository.Ref(),
		}
		compilation.seal = compilationSeal(compilation)
		return compilation, nil
	}
	fileTree, err := buildFileTree(snapshot.Entries)
	if err != nil {
		return Compilation{}, fmt.Errorf("readme target scout: build complete file tree: %w", err)
	}
	request := Request{
		RepoName: repoName, FileCount: len(snapshot.Entries), FileTree: cloneFileTree(fileTree),
		GuidanceDocuments: documents,
	}
	wire, err := json.Marshal(request)
	if err != nil {
		return Compilation{}, fmt.Errorf("readme target scout: encode complete request: %w", err)
	}
	if len(wire) > MaxRequestBytes {
		return Compilation{}, fmt.Errorf(
			"readme target scout: complete guidance + lossless file-tree request is %d bytes, reliable atomic limit is %d; no provider request was made and an explicitly approved semantic partition or chunked repository-index contract is required",
			len(wire), MaxRequestBytes,
		)
	}
	compilation := Compilation{
		Version: CompilationVersion, State: StateReady, Request: request,
		RequestSHA256: sha256Hex(wire), wire: append([]byte(nil), wire...),
		authority: cloneDictionary(authority), corpusRef: repository.Ref(),
	}
	compilation.seal = compilationSeal(compilation)
	if err := validateReadyCompilation(compilation); err != nil {
		return Compilation{}, err
	}
	return compilation, nil
}

func validateReadyCompilation(compilation Compilation) error {
	if compilation.Version != CompilationVersion || compilation.State != StateReady || compilation.Reason != "" ||
		compilation.RequestSHA256 == "" || compilation.corpusRef == "" || len(compilation.Request.GuidanceDocuments) == 0 ||
		compilation.Request.FileCount != len(compilation.authority) || compilation.Request.FileTree == nil {
		return fmt.Errorf("readme target scout: invalid ready compilation identity")
	}
	if err := validateRepoName(compilation.Request.RepoName); err != nil {
		return err
	}
	treeAuthority, err := fileTreeDictionary(
		compilation.Request.FileTree,
		compilation.Request.FileCount,
	)
	if err != nil {
		return fmt.Errorf("readme target scout: complete file tree: %w", err)
	}
	if len(treeAuthority) != len(compilation.authority) {
		return fmt.Errorf("readme target scout: complete file tree authority mismatch")
	}
	for id, filePath := range treeAuthority {
		if id == "" || validateRepoPath(filePath) != nil || compilation.authority[id] != filePath {
			return fmt.Errorf("readme target scout: complete file tree authority mismatch")
		}
	}
	seenDocuments := make(map[corpus.FileID]struct{}, len(compilation.Request.GuidanceDocuments))
	for _, document := range compilation.Request.GuidanceDocuments {
		kind, ok := guidanceKind(document.Path)
		if document.FileRef == "" || compilation.authority[document.FileRef] != document.Path ||
			!ok || kind != document.Kind || !utf8.ValidString(document.Content) {
			return fmt.Errorf("readme target scout: invalid complete guidance row")
		}
		if _, duplicate := seenDocuments[document.FileRef]; duplicate {
			return fmt.Errorf("readme target scout: duplicate guidance FileID")
		}
		seenDocuments[document.FileRef] = struct{}{}
	}
	wire, err := json.Marshal(compilation.Request)
	if err != nil {
		return fmt.Errorf("readme target scout: encode complete request: %w", err)
	}
	if len(wire) > MaxRequestBytes || !reflect.DeepEqual(wire, compilation.wire) || compilation.RequestSHA256 != sha256Hex(wire) {
		return fmt.Errorf("readme target scout: request wire binding mismatch")
	}
	if compilation.seal != compilationSeal(compilation) {
		return fmt.Errorf("readme target scout: compilation seal mismatch")
	}
	return nil
}

func cloneDictionary(source map[corpus.FileID]string) map[corpus.FileID]string {
	result := make(map[corpus.FileID]string, len(source))
	for id, filePath := range source {
		result[id] = filePath
	}
	return result
}

func compilationSeal(compilation Compilation) string {
	return sha256Hex([]byte(strings.Join([]string{
		"readme-target-scout-compilation-v5", compilation.corpusRef,
		string(compilation.State), string(compilation.Reason), compilation.RequestSHA256,
	}, "\x00")))
}

func sha256Hex(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}

func validateRepoName(value string) error {
	if value == "" || value != strings.TrimSpace(value) || !utf8.ValidString(value) ||
		strings.ContainsAny(value, `/\\`) || containsControl(value) {
		return fmt.Errorf("readme target scout: invalid repository name")
	}
	return nil
}

func validateRepoPath(value string) error {
	if value == "" || value == "." || !utf8.ValidString(value) || containsControl(value) ||
		strings.Contains(value, "\\") || strings.HasPrefix(value, "/") || path.Clean(value) != value ||
		value == ".." || strings.HasPrefix(value, "../") {
		return fmt.Errorf("invalid repository-relative path")
	}
	return nil
}

func isReadmePath(value string) bool {
	name := strings.ToLower(path.Base(value))
	if name == "readme" {
		return true
	}
	if !strings.HasPrefix(name, "readme.") {
		return false
	}
	switch path.Ext(name) {
	case ".md", ".markdown", ".mdown", ".mkd", ".mkdn",
		".rst", ".rest", ".txt", ".textile", ".rdoc", ".org",
		".creole", ".mediawiki", ".wiki", ".adoc", ".asciidoc":
		return true
	default:
		return false
	}
}

func guidanceKind(value string) (GuidanceKind, bool) {
	if isReadmePath(value) {
		return GuidanceReadme, true
	}
	if strings.EqualFold(path.Base(value), "AGENTS.md") {
		return GuidanceAgents, true
	}
	return "", false
}

func containsControl(value string) bool {
	for _, character := range value {
		if unicode.IsControl(character) {
			return true
		}
	}
	return false
}
