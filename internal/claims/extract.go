package claims

import (
	"context"
	"fmt"
	"path"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/secretscan"
)

const (
	// DefaultCommitLimit is the commit window quoted when the caller passes 0.
	DefaultCommitLimit = 20
	// MaxSourceBytes bounds one file read for quotes; larger files are
	// skipped whole rather than partially scanned.
	MaxSourceBytes = int64(1 << 20)

	shortCommitLength = 12
	isoDateLength     = 10
	isoDateLayout     = "2006-01-02"
	hoursPerDay       = 24
)

// TargetRoot lets a file claim carry the id of the target whose root
// contains it. Root is repository-relative; "" or "." is the whole repository.
type TargetRoot struct {
	ID   string
	Root string
}

// Input is everything the deterministic claims stage reads.
type Input struct {
	Revision    string
	RepoPath    string
	Repository  *corpus.Corpus
	Targets     []TargetRoot
	CommitLimit int
}

// Extract quotes README text, docstrings, marker comments, and commit subjects
// and seals them. Same repository state and input produce byte-identical
// results; the only subprocess is git, run against the captured revision.
func Extract(ctx context.Context, input Input) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := input.validate(); err != nil {
		return Result{}, err
	}
	limit := input.CommitLimit
	if limit <= 0 {
		limit = DefaultCommitLimit
	}
	asOf, err := revisionDate(ctx, input.RepoPath, input.Revision)
	if err != nil {
		return Result{}, err
	}
	commits, err := listCommits(ctx, input.RepoPath, input.Revision, limit)
	if err != nil {
		return Result{}, err
	}
	fileDates, err := fileCommitDates(ctx, input.RepoPath, input.Revision)
	if err != nil {
		return Result{}, err
	}

	builder := newBuilder(asOf, input.Targets)
	for _, commit := range commits {
		builder.addCommit(commit)
	}
	for _, entry := range input.Repository.Entries() {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		quotes, err := fileQuotes(input.Repository, entry)
		if err != nil {
			return Result{}, err
		}
		for _, quote := range quotes {
			builder.addFile(entry.Path, quote, fileDates[entry.Path])
		}
	}
	return Seal(Result{
		Revision: input.Revision,
		AsOf:     asOf,
		Claims:   builder.claims,
		Dropped:  builder.dropped,
	})
}

func (input Input) validate() error {
	if strings.TrimSpace(input.Revision) == "" {
		return fmt.Errorf("claims: revision is required")
	}
	if strings.TrimSpace(input.RepoPath) == "" {
		return fmt.Errorf("claims: repository path is required")
	}
	if input.Repository == nil {
		return fmt.Errorf("claims: repository corpus is required")
	}
	for position, target := range input.Targets {
		if strings.TrimSpace(target.ID) == "" {
			return fmt.Errorf("claims: target %d has no id", position)
		}
	}
	return nil
}

// fileQuote is one quote located inside a file before dates are attached.
type fileQuote struct {
	Source Source
	Line   int
	Text   string
}

type fileKind int

const (
	kindOther fileKind = iota
	kindReadme
	kindPython
	kindGo
	kindJSTS
)

func classifyPath(filePath string) fileKind {
	if isReadmePath(filePath) {
		return kindReadme
	}
	switch strings.ToLower(path.Ext(filePath)) {
	case ".py":
		return kindPython
	case ".go":
		return kindGo
	case ".js", ".jsx", ".mjs", ".cjs", ".ts", ".tsx", ".mts", ".cts":
		return kindJSTS
	default:
		return kindOther
	}
}

// fileQuotes reads one corpus entry within MaxSourceBytes and runs the
// scanners for its kind. Oversized and non-UTF-8 files yield nothing.
func fileQuotes(repository *corpus.Corpus, entry corpus.Entry) ([]fileQuote, error) {
	kind := classifyPath(entry.Path)
	if kind == kindOther {
		return nil, nil
	}
	content, err := repository.ReadFile(entry.ID, MaxSourceBytes)
	if err != nil {
		return nil, fmt.Errorf("claims: read %s: %w", entry.Path, err)
	}
	if content.Truncated || !utf8.Valid(content.Bytes) {
		return nil, nil
	}
	lines := splitLines(string(content.Bytes))
	switch kind {
	case kindReadme:
		return tagged(SourceReadme, readmeQuotes(len(content.Bytes), lines)), nil
	case kindPython:
		return append(
			tagged(SourceDocstring, pythonDocstrings(lines)),
			tagged(SourceComment, markerComments(lines, "#"))...,
		), nil
	case kindGo:
		return append(
			tagged(SourceDocstring, goDocComments(lines)),
			tagged(SourceComment, markerComments(lines, "//"))...,
		), nil
	default:
		return append(
			tagged(SourceDocstring, jsDocBlocks(lines)),
			tagged(SourceComment, markerComments(lines, "//"))...,
		), nil
	}
}

func tagged(source Source, quotes []quote) []fileQuote {
	result := make([]fileQuote, 0, len(quotes))
	for _, item := range quotes {
		result = append(result, fileQuote{Source: source, Line: item.Line, Text: item.Text})
	}
	return result
}

// builder accumulates claims, withholding credential-shaped text and
// collapsing exact duplicates so Seal never sees a repeated id.
type builder struct {
	asOf    string
	targets []TargetRoot
	seen    map[string]struct{}
	claims  []Claim
	dropped int
}

func newBuilder(asOf string, targets []TargetRoot) *builder {
	return &builder{asOf: asOf, targets: targets, seen: make(map[string]struct{})}
}

func (b *builder) addCommit(commit commitRecord) {
	b.add(Claim{
		Source: SourceCommit,
		Commit: commit.Hash[:shortCommitLength],
		Text:   commit.Subject,
		Date:   commit.Date,
	}, commit.Hash)
}

func (b *builder) addFile(filePath string, quote fileQuote, date string) {
	claim := Claim{
		Source:   quote.Source,
		Path:     filePath,
		Line:     quote.Line,
		Text:     quote.Text,
		Date:     date,
		TargetID: targetFor(b.targets, filePath),
	}
	if date != "" {
		claim.AgeDays = ageDays(b.asOf, date)
	}
	b.add(claim, fmt.Sprintf("%s:%d", filePath, quote.Line))
}

func (b *builder) add(claim Claim, location string) {
	if claim.Text == "" {
		return
	}
	if _, sensitive := secretscan.DetectPersistenceSensitive(claim.Text); sensitive {
		b.dropped++
		return
	}
	claim.ID = NewClaimID(claim.Source, location, claim.Text)
	if _, duplicate := b.seen[claim.ID]; duplicate {
		return
	}
	b.seen[claim.ID] = struct{}{}
	b.claims = append(b.claims, claim)
}

// targetFor picks the most specific root containing the path.
func targetFor(targets []TargetRoot, filePath string) string {
	bestID, bestLength := "", -1
	for _, target := range targets {
		root := path.Clean(target.Root)
		if root == "." {
			root = ""
		}
		if root != "" && filePath != root && !strings.HasPrefix(filePath, root+"/") {
			continue
		}
		if len(root) > bestLength {
			bestID, bestLength = target.ID, len(root)
		}
	}
	return bestID
}

// ageDays counts whole days from date to asOf; a date after asOf (possible
// across time zones) is clamped to zero.
func ageDays(asOf, date string) int {
	end, errEnd := time.Parse(isoDateLayout, asOf)
	start, errStart := time.Parse(isoDateLayout, date)
	if errEnd != nil || errStart != nil {
		return 0
	}
	days := int(end.Sub(start).Hours() / hoursPerDay)
	if days < 0 {
		return 0
	}
	return days
}
