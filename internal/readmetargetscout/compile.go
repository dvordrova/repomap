package readmetargetscout

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"reflect"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/corpus"
)

const preparationContract = "complete canonical corpus FileID-to-path authority restricted to candidate entry files: no prose file and no .claude, .github or .vscode tree; complete current bytes of every tracked regular README and AGENTS.md; aggregate compilation never size-rejected; deterministic guidance-group by file-group product covers every guidance byte against every candidate file through lossless path-component-tree requests; former packing windows never reject an indivisible document or file row; an indivisible prepared request is terminal only after crossing the shared semantic-record envelope; no semantic filtering, truncation, prefix selection, or partial result-v9"

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

// Compile captures the complete aggregate evidence authority. It never drops
// or rejects repository facts merely because their combined encoding is larger
// than one provider request; Batches creates the exhaustive bounded exchange.
// Only candidate entry files enter the authority: prose files and the
// excluded configuration trees can never be the entry the guidance names.
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
		if kind, ok := guidanceKind(entry.Path); ok {
			content, err := repository.ReadFileAll(entry.ID)
			if err != nil {
				return Compilation{}, fmt.Errorf("readme target scout: read complete repository guidance %s: %w", entry.ID, err)
			}
			if !utf8.Valid(content.Bytes) {
				return Compilation{}, fmt.Errorf("readme target scout: repository guidance %q is not valid UTF-8; no provider request was made", entry.Path)
			}
			documents = append(documents, RequestGuidanceDocument{
				FileRef: entry.ID, Path: entry.Path, Kind: kind, Content: string(content.Bytes),
			})
			continue
		}
		if isCandidateEntryPath(entry.Path) {
			authority[entry.ID] = entry.Path
		}
	}
	if len(documents) == 0 {
		return notApplicableCompilation(repository, NoGuidanceFiles), nil
	}
	if len(authority) == 0 {
		return notApplicableCompilation(repository, NoCandidateFiles), nil
	}
	fileTree, err := buildFileTree(authorityEntries(authority))
	if err != nil {
		return Compilation{}, fmt.Errorf("readme target scout: build candidate file tree: %w", err)
	}
	request := Request{
		RepoName: repoName, FileCount: len(authority), FileTree: cloneFileTree(fileTree),
		GuidanceDocuments: documents,
	}
	wire, err := json.Marshal(request)
	if err != nil {
		return Compilation{}, fmt.Errorf("readme target scout: encode complete request: %w", err)
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

func notApplicableCompilation(repository *corpus.Corpus, reason NotApplicableReason) Compilation {
	compilation := Compilation{
		Version: CompilationVersion, State: StateNotApplicable,
		Reason: reason, corpusRef: repository.Ref(),
	}
	compilation.seal = compilationSeal(compilation)
	return compilation
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
		return fmt.Errorf("readme target scout: candidate file tree: %w", err)
	}
	if len(treeAuthority) != len(compilation.authority) {
		return fmt.Errorf("readme target scout: candidate file tree authority mismatch")
	}
	for id, filePath := range treeAuthority {
		if id == "" || validateRepoPath(filePath) != nil || compilation.authority[id] != filePath {
			return fmt.Errorf("readme target scout: candidate file tree authority mismatch")
		}
		if !isCandidateEntryPath(filePath) {
			return fmt.Errorf("readme target scout: candidate file tree contains a non-candidate path")
		}
	}
	seenDocuments := make(map[corpus.FileID]struct{}, len(compilation.Request.GuidanceDocuments))
	for _, document := range compilation.Request.GuidanceDocuments {
		kind, ok := guidanceKind(document.Path)
		if document.FileRef == "" || validateRepoPath(document.Path) != nil ||
			!ok || kind != document.Kind || !utf8.ValidString(document.Content) {
			return fmt.Errorf("readme target scout: invalid complete guidance row")
		}
		if _, duplicate := seenDocuments[document.FileRef]; duplicate {
			return fmt.Errorf("readme target scout: duplicate guidance FileID")
		}
		if _, candidate := compilation.authority[document.FileRef]; candidate {
			return fmt.Errorf("readme target scout: guidance row is also a candidate file")
		}
		seenDocuments[document.FileRef] = struct{}{}
	}
	wire, err := json.Marshal(compilation.Request)
	if err != nil {
		return fmt.Errorf("readme target scout: encode complete request: %w", err)
	}
	if !reflect.DeepEqual(wire, compilation.wire) || compilation.RequestSHA256 != sha256Hex(wire) {
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

// authorityEntries lists one exact dictionary in canonical path order for the
// lossless tree builder.
func authorityEntries(authority map[corpus.FileID]string) []corpus.Entry {
	entries := make([]corpus.Entry, 0, len(authority))
	for fileRef, filePath := range authority {
		entries = append(entries, corpus.Entry{ID: fileRef, Path: filePath})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Path != entries[j].Path {
			return entries[i].Path < entries[j].Path
		}
		return entries[i].ID < entries[j].ID
	})
	return entries
}

func compilationSeal(compilation Compilation) string {
	return sha256Hex([]byte(strings.Join([]string{
		"readme-target-scout-compilation-v8", compilation.corpusRef,
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

// excludedTreeComponents name editor and agent configuration trees. Their
// files are never launched, built, deployed or imported as a repository
// product, so they are outside the candidate authority.
var excludedTreeComponents = map[string]bool{".claude": true, ".github": true, ".vscode": true}

// isCandidateEntryPath reports whether a tracked regular file could be the
// entry the guidance names: code or a manifest outside prose and outside the
// excluded configuration trees. It bounds request preparation; it never
// establishes a role.
func isCandidateEntryPath(value string) bool {
	if isProseEvidencePath(value) {
		return false
	}
	for _, component := range strings.Split(value, "/") {
		if excludedTreeComponents[component] {
			return false
		}
	}
	return true
}

func isProseEvidencePath(value string) bool {
	if isReadmePath(value) {
		return true
	}
	name := strings.ToLower(path.Base(value))
	for _, prefix := range []string{
		"license", "copying", "changelog", "changes", "contributing", "code_of_conduct",
	} {
		if name == prefix || strings.HasPrefix(name, prefix+".") {
			return true
		}
	}
	switch strings.ToLower(path.Ext(name)) {
	case ".md", ".markdown", ".mdown", ".mkd", ".mkdn",
		".rst", ".rest", ".txt", ".textile", ".rdoc", ".org",
		".creole", ".mediawiki", ".wiki", ".adoc", ".asciidoc":
		return true
	default:
		return false
	}
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
