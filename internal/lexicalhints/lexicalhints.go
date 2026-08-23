// Package lexicalhints extracts small, deterministic lexical signals from the
// tracked repository corpus. It never exposes source bytes or host paths: the
// provider-facing model contains only closed terms, corpus-local file refs,
// and capped occurrence counts.
package lexicalhints

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"sync"
	"sync/atomic"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/corpus"
)

const (
	// Version changes whenever matching, filtering, or wire semantics
	// change. Domain cubes can include it in their own cache identity.
	Version = 1

	MaxWorkers              = 4
	MaxFileBytes            = int64(4 << 20)
	MaxCanonicalBytes       = 128 << 20
	CountCap          uint8 = 255
)

var closedTerms = [...]string{
	"config",
	"dao",
	"sql",
	"request",
	"response",
	"client",
	"route",
	"worker",
	"kafka",
}

// ScannerState is the complete stable configuration of this extractor.
type ScannerState struct {
	Version           int      `json:"version"`
	Terms             []string `json:"terms"`
	MaxWorkers        int      `json:"max_workers"`
	MaxFileBytes      int64    `json:"max_file_bytes"`
	MaxCanonicalBytes int      `json:"max_canonical_bytes"`
	CountCap          uint8    `json:"count_cap"`
}

// State returns an independently owned description of the extractor rules.
func State() ScannerState {
	return ScannerState{
		Version:           Version,
		Terms:             append([]string(nil), closedTerms[:]...),
		MaxWorkers:        MaxWorkers,
		MaxFileBytes:      MaxFileBytes,
		MaxCanonicalBytes: MaxCanonicalBytes,
		CountCap:          CountCap,
	}
}

// Model is safe to append directly to a provider request. It deliberately has
// no repository paths, source snippets, or local diagnostics. Counts are exact
// through 254; 255 means 255 or more occurrences.
type Model struct {
	Version int                                `json:"version"`
	ByFile  map[corpus.FileID]map[string]uint8 `json:"by_file"`
}

// CanonicalJSON validates the closed sparse shape and returns its deterministic
// compact JSON encoding. encoding/json sorts both FileID and term map keys.
func (model Model) CanonicalJSON() ([]byte, error) {
	if err := model.validate(); err != nil {
		return nil, err
	}
	wire, err := json.Marshal(model)
	if err != nil {
		return nil, fmt.Errorf("lexical hints: encode model: %w", err)
	}
	if len(wire) > MaxCanonicalBytes {
		return nil, fmt.Errorf("lexical hints: model exceeds %d bytes", MaxCanonicalBytes)
	}
	return wire, nil
}

func (model Model) validate() error {
	if model.Version != Version || model.ByFile == nil || len(model.ByFile) > corpus.MaxFiles {
		return fmt.Errorf("lexical hints: invalid model identity")
	}
	allowed := make(map[string]struct{}, len(closedTerms))
	for _, term := range closedTerms {
		allowed[term] = struct{}{}
	}
	for fileRef, counts := range model.ByFile {
		if !validFileRef(fileRef) || len(counts) == 0 || len(counts) > len(closedTerms) {
			return fmt.Errorf("lexical hints: invalid file row %q", fileRef)
		}
		for term, count := range counts {
			if _, ok := allowed[term]; !ok || count == 0 {
				return fmt.Errorf("lexical hints: invalid count for %q in %q", term, fileRef)
			}
		}
	}
	return nil
}

func validFileRef(fileRef corpus.FileID) bool {
	value := string(fileRef)
	if len(value) < 2 || value[0] != 'f' || value[1] == '0' {
		return false
	}
	number := 0
	for _, character := range value[1:] {
		if character < '0' || character > '9' {
			return false
		}
		number = number*10 + int(character-'0')
		if number > corpus.MaxFiles {
			return false
		}
	}
	return number > 0
}

// Coverage is local-only diagnostic accounting. Excluded and unreadable files
// do not make a repository scan fail, but they are never hidden from local
// observability.
type Coverage struct {
	TrackedFiles         int `json:"tracked_files"`
	ScannedFiles         int `json:"scanned_files"`
	PathExcludedFiles    int `json:"path_excluded_files"`
	OversizeOmissions    int `json:"oversize_omissions"`
	BinaryOmissions      int `json:"binary_omissions"`
	InvalidUTF8Omissions int `json:"invalid_utf8_omissions"`
	ReadOmissions        int `json:"read_omissions"`
}

// Result keeps provider-safe evidence and local coverage structurally
// separate. Only Model should be placed into a provider request.
type Result struct {
	CorpusRef string   `json:"-"`
	Model     Model    `json:"model"`
	Coverage  Coverage `json:"-"`
}

type fileScan struct {
	counts      [len(closedTerms)]uint8
	disposition disposition
}

type disposition uint8

const (
	dispositionScanned disposition = iota + 1
	dispositionPathExcluded
	dispositionOversize
	dispositionBinary
	dispositionInvalidUTF8
	dispositionReadOmission
)

// Scan reads eligible current working-tree files from repository exactly once
// with at most four workers. Per-file races and read failures become coverage
// omissions; cancellation is the only scan-wide runtime failure.
func Scan(ctx context.Context, repository *corpus.Corpus) (Result, error) {
	if repository == nil {
		return Result{}, fmt.Errorf("lexical hints: repository corpus is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}

	entries := repository.Entries()
	results := make([]fileScan, len(entries))
	workerCount := len(entries)
	if workerCount > MaxWorkers {
		workerCount = MaxWorkers
	}

	var next atomic.Int64
	next.Store(-1)
	var workers sync.WaitGroup
	workers.Add(workerCount)
	for worker := 0; worker < workerCount; worker++ {
		go func() {
			defer workers.Done()
			for {
				if ctx.Err() != nil {
					return
				}
				index := int(next.Add(1))
				if index >= len(entries) {
					return
				}
				results[index] = scanFile(repository, entries[index])
			}
		}()
	}
	workers.Wait()
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}

	result := assemble(entries, results)
	result.CorpusRef = repository.Ref()
	return result, nil
}

func scanFile(repository *corpus.Corpus, entry corpus.Entry) fileScan {
	if excludePath(entry.Path) {
		return fileScan{disposition: dispositionPathExcluded}
	}
	content, err := repository.ReadFile(entry.ID, MaxFileBytes)
	if err != nil {
		return fileScan{disposition: dispositionReadOmission}
	}
	if content.Truncated {
		return fileScan{disposition: dispositionOversize}
	}
	if looksBinary(content.Bytes) {
		return fileScan{disposition: dispositionBinary}
	}
	if !utf8.Valid(content.Bytes) {
		return fileScan{disposition: dispositionInvalidUTF8}
	}
	return fileScan{
		counts:      defaultMatcher.match(content.Bytes),
		disposition: dispositionScanned,
	}
}

func assemble(entries []corpus.Entry, scans []fileScan) Result {
	coverage := Coverage{TrackedFiles: len(entries)}
	for _, scan := range scans {
		switch scan.disposition {
		case dispositionScanned:
			coverage.ScannedFiles++
		case dispositionPathExcluded:
			coverage.PathExcludedFiles++
		case dispositionOversize:
			coverage.OversizeOmissions++
		case dispositionBinary:
			coverage.BinaryOmissions++
		case dispositionInvalidUTF8:
			coverage.InvalidUTF8Omissions++
		case dispositionReadOmission:
			coverage.ReadOmissions++
		}
	}

	model := Model{
		Version: Version,
		ByFile:  make(map[corpus.FileID]map[string]uint8),
	}
	for fileIndex, scan := range scans {
		var counts map[string]uint8
		for termIndex, count := range scan.counts {
			if count == 0 {
				continue
			}
			if counts == nil {
				counts = make(map[string]uint8)
			}
			counts[closedTerms[termIndex]] = count
		}
		if len(counts) > 0 {
			model.ByFile[entries[fileIndex].ID] = counts
		}
	}
	return Result{Model: model, Coverage: coverage}
}

func excludePath(filePath string) bool {
	lower := strings.ToLower(filePath)
	for _, component := range strings.Split(lower, "/") {
		switch component {
		case "vendor", "node_modules", "third_party":
			return true
		}
	}
	base := path.Base(lower)
	if isLockFile(base) || strings.HasSuffix(base, ".min.js") ||
		strings.HasSuffix(base, ".min.css") || strings.HasSuffix(base, ".js.map") ||
		strings.HasSuffix(base, ".css.map") {
		return true
	}
	switch path.Ext(base) {
	case ".7z", ".a", ".avi", ".bmp", ".class", ".dll", ".dylib",
		".eot", ".exe", ".flac", ".gif", ".gz", ".ico", ".jar",
		".jpeg", ".jpg", ".mov", ".mp3", ".mp4", ".o", ".otf",
		".pdf", ".png", ".so", ".tar", ".tif", ".tiff", ".ttf",
		".wav", ".webm", ".webp", ".woff", ".woff2", ".xz", ".zip":
		return true
	default:
		return false
	}
}

func isLockFile(base string) bool {
	if strings.HasSuffix(base, ".lock") {
		return true
	}
	switch base {
	case "bun.lockb", "go.sum", "package-lock.json", "packages.lock.json",
		"pnpm-lock.yaml", "yarn.lock":
		return true
	default:
		return false
	}
}

func looksBinary(data []byte) bool {
	if bytes.IndexByte(data, 0) >= 0 {
		return true
	}
	if len(data) == 0 {
		return false
	}
	controls := 0
	for _, value := range data {
		if value < 0x20 && value != '\t' && value != '\n' && value != '\r' && value != '\f' {
			controls++
		}
	}
	return controls*100 > len(data)
}

type matcherNode struct {
	next    [128]int
	failure int
	outputs []int
}

type matcher struct {
	nodes []matcherNode
}

var defaultMatcher = newMatcher(closedTerms[:])

func newMatcher(terms []string) matcher {
	result := matcher{nodes: []matcherNode{{}}}
	for termIndex, term := range terms {
		state := 0
		for _, value := range []byte(term) {
			next := result.nodes[state].next[value]
			if next == 0 {
				result.nodes = append(result.nodes, matcherNode{})
				next = len(result.nodes) - 1
				result.nodes[state].next[value] = next
			}
			state = next
		}
		result.nodes[state].outputs = append(result.nodes[state].outputs, termIndex)
	}

	queue := make([]int, 0, len(result.nodes))
	for value := 0; value < len(result.nodes[0].next); value++ {
		if child := result.nodes[0].next[value]; child != 0 {
			queue = append(queue, child)
		}
	}
	for len(queue) > 0 {
		state := queue[0]
		queue = queue[1:]
		for value := 0; value < len(result.nodes[state].next); value++ {
			child := result.nodes[state].next[value]
			if child == 0 {
				continue
			}
			queue = append(queue, child)
			failure := result.nodes[state].failure
			for failure != 0 && result.nodes[failure].next[value] == 0 {
				failure = result.nodes[failure].failure
			}
			if candidate := result.nodes[failure].next[value]; candidate != child {
				result.nodes[child].failure = candidate
			}
			inherited := result.nodes[result.nodes[child].failure].outputs
			result.nodes[child].outputs = append(result.nodes[child].outputs, inherited...)
		}
	}
	return result
}

func (matcher matcher) match(data []byte) [len(closedTerms)]uint8 {
	var counts [len(closedTerms)]uint8
	state := 0
	for _, raw := range data {
		value := asciiLower(raw)
		if value >= 128 {
			state = 0
			continue
		}
		for state != 0 && matcher.nodes[state].next[value] == 0 {
			state = matcher.nodes[state].failure
		}
		state = matcher.nodes[state].next[value]
		for _, termIndex := range matcher.nodes[state].outputs {
			if counts[termIndex] < CountCap {
				counts[termIndex]++
			}
		}
	}
	return counts
}

func asciiLower(value byte) byte {
	if value >= 'A' && value <= 'Z' {
		return value + ('a' - 'A')
	}
	return value
}
