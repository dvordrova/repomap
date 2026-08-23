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
	"github.com/dvordrova/repomap/internal/lexicalhints"
)

const preparationContract = "one atomic request; complete canonical corpus FileID-to-path authority encoded as a lossless path-component tree with FileID string leaves; complete current bytes of every tracked regular README; sparse capped lexical substring counts from lexicalhints v1; source bytes and local scan omissions excluded; no semantic filtering, truncation, chunking, or partial result-v3"

// HasReadmeFiles is the cheap metadata-only applicability check used before
// the local lexical scan. Compile repeats the authoritative check while
// building its exact request.
func HasReadmeFiles(repository *corpus.Corpus) bool {
	if repository == nil {
		return false
	}
	for _, entry := range repository.Entries() {
		if isReadmePath(entry.Path) {
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
	lexical lexicalhints.Result,
) (Compilation, error) {
	if err := validateRepoName(repoName); err != nil {
		return Compilation{}, err
	}
	if repository == nil {
		return Compilation{}, fmt.Errorf("readme target scout: repository corpus is required")
	}
	if lexical.CorpusRef == "" || lexical.CorpusRef != repository.Ref() {
		return Compilation{}, fmt.Errorf("readme target scout: lexical hints do not belong to this repository corpus")
	}
	if _, err := lexical.Model.CanonicalJSON(); err != nil {
		return Compilation{}, fmt.Errorf("readme target scout: lexical hints: %w", err)
	}
	snapshot := repository.Snapshot()
	if err := snapshot.Validate(); err != nil {
		return Compilation{}, fmt.Errorf("readme target scout: repository corpus: %w", err)
	}
	authority := make(map[corpus.FileID]string, len(snapshot.Entries))
	readmes := make([]RequestReadme, 0)
	for _, entry := range snapshot.Entries {
		authority[entry.ID] = entry.Path
		if !isReadmePath(entry.Path) {
			continue
		}
		content, err := repository.ReadFile(entry.ID, corpus.MaxReadBytes)
		if err != nil {
			return Compilation{}, fmt.Errorf("readme target scout: read complete README %s: %w", entry.ID, err)
		}
		if content.Truncated {
			return Compilation{}, fmt.Errorf(
				"readme target scout: README %q exceeds the %d-byte complete-read limit; no provider request was made because partial README analysis is forbidden",
				entry.Path, corpus.MaxReadBytes,
			)
		}
		if !utf8.Valid(content.Bytes) {
			return Compilation{}, fmt.Errorf("readme target scout: README %q is not valid UTF-8; no provider request was made", entry.Path)
		}
		readmes = append(readmes, RequestReadme{
			FileRef: entry.ID, Path: entry.Path, Content: string(content.Bytes),
		})
	}
	if len(readmes) == 0 {
		compilation := Compilation{
			Version: CompilationVersion, State: StateNotApplicable,
			Reason: NoReadmeFiles, corpusRef: repository.Ref(),
		}
		compilation.seal = compilationSeal(compilation)
		return compilation, nil
	}
	fileTree, err := buildFileTree(snapshot.Entries)
	if err != nil {
		return Compilation{}, fmt.Errorf("readme target scout: build complete file tree: %w", err)
	}
	for fileRef := range lexical.Model.ByFile {
		if _, known := authority[fileRef]; !known {
			return Compilation{}, fmt.Errorf("readme target scout: lexical hints cite a file outside the corpus")
		}
	}
	request := Request{
		RepoName: repoName, FileCount: len(snapshot.Entries), FileTree: cloneFileTree(fileTree),
		GrepStats: cloneGrepStats(lexical.Model.ByFile), Readmes: readmes,
	}
	wire, err := json.Marshal(request)
	if err != nil {
		return Compilation{}, fmt.Errorf("readme target scout: encode complete request: %w", err)
	}
	if len(wire) > MaxRequestBytes {
		return Compilation{}, fmt.Errorf(
			"readme target scout: complete README + lossless file-tree + grep-stats request is %d bytes, reliable atomic limit is %d; no provider request was made and an explicitly approved semantic partition or chunked repository-index contract is required",
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
		compilation.RequestSHA256 == "" || compilation.corpusRef == "" || len(compilation.Request.Readmes) == 0 ||
		compilation.Request.FileCount != len(compilation.authority) || compilation.Request.FileTree == nil ||
		compilation.Request.GrepStats == nil {
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
	if _, err := (lexicalhints.Model{
		Version: lexicalhints.Version,
		ByFile:  compilation.Request.GrepStats,
	}).CanonicalJSON(); err != nil {
		return fmt.Errorf("readme target scout: invalid lexical hints: %w", err)
	}
	for fileRef := range compilation.Request.GrepStats {
		if _, known := compilation.authority[fileRef]; !known {
			return fmt.Errorf("readme target scout: lexical hints authority mismatch")
		}
	}
	seenReadmes := make(map[corpus.FileID]struct{}, len(compilation.Request.Readmes))
	for _, readme := range compilation.Request.Readmes {
		if readme.FileRef == "" || compilation.authority[readme.FileRef] != readme.Path ||
			!isReadmePath(readme.Path) || !utf8.ValidString(readme.Content) {
			return fmt.Errorf("readme target scout: invalid complete README row")
		}
		if _, duplicate := seenReadmes[readme.FileRef]; duplicate {
			return fmt.Errorf("readme target scout: duplicate README FileID")
		}
		seenReadmes[readme.FileRef] = struct{}{}
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

func cloneGrepStats(source map[corpus.FileID]map[string]uint8) map[corpus.FileID]map[string]uint8 {
	result := make(map[corpus.FileID]map[string]uint8, len(source))
	for fileRef, sourceCounts := range source {
		counts := make(map[string]uint8, len(sourceCounts))
		for term, count := range sourceCounts {
			counts[term] = count
		}
		result[fileRef] = counts
	}
	return result
}

func compilationSeal(compilation Compilation) string {
	return sha256Hex([]byte(strings.Join([]string{
		"readme-target-scout-compilation-v3", compilation.corpusRef,
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

func containsControl(value string) bool {
	for _, character := range value {
		if unicode.IsControl(character) {
			return true
		}
	}
	return false
}
