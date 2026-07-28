package semanticmap

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	analysis "github.com/dvordrova/repomap/internal/analyzer"
	"github.com/dvordrova/repomap/internal/analyzer/golang/gopls"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/reporead"
)

const (
	goSelectionVersion            = "semantic-map-go-selector-v1"
	goSelectionMaxQuestionBytes   = 512
	goSelectionMaxQueryTerms      = 3
	goSelectionMaxContentTerms    = 12
	goSelectionMaxHitsPerTerm     = 32
	goSelectionMaxCandidates      = 96
	goSelectionMaxDiscoveryItems  = 64
	goSelectionMaxExactItems      = 32
	goSelectionMaxExactAnalyses   = 3
	goSelectionMaxExpansionHops   = 2
	goSelectionMaxCallEndpoints   = 64
	goSelectionMaxResolveAttempts = 24
	goSelectionMaxResolveResults  = 8
	goSelectionMaxRankReasons     = 8
	goSelectionMaxProvenance      = 4
	goSelectionMaxWarnings        = 8
	goSelectionMaxAnalyzerText    = 240
	goSelectionMaxSlices          = 12
	goSelectionMaxSourceBytes     = 24 << 10
	goSelectionMaxAnchorBytes     = 3 << 10
	goSelectionMaxSourceFileBytes = 4 << 20
	goSelectionMaxSliceLines      = 201
	goSelectionMaxPathBytes       = 240
	goSelectionCommandTimeout     = 30 * time.Second
	goSelectionGitTimeout         = 10 * time.Second
)

var goSelectionStopWords = map[string]struct{}{
	"a": {}, "an": {}, "and": {}, "are": {}, "as": {}, "at": {}, "be": {},
	"before": {}, "by": {}, "can": {}, "does": {}, "for": {}, "from": {},
	"how": {}, "in": {}, "into": {}, "is": {}, "it": {}, "new": {}, "not": {},
	"of": {}, "old": {}, "on": {}, "one": {}, "or": {}, "that": {}, "the": {},
	"this": {}, "to": {}, "what": {}, "when": {}, "where": {}, "which": {},
	"without": {}, "with": {},
}

// GoSourceSelectionOptions is the complete input to the experiment. The
// selector deliberately accepts no curated path, packet, sidecar, or answer
// vocabulary.
type GoSourceSelectionOptions struct {
	RepositoryPath   string
	ExpectedRevision string
	Question         string
	GoplsBinary      string
}

type GoSelectionRepository struct {
	Name     string `json:"name"`
	Revision string `json:"revision"`
}

type GoSelectionLimits struct {
	QueryTerms       int `json:"query_terms"`
	HitsPerTerm      int `json:"hits_per_term"`
	Candidates       int `json:"candidates"`
	ExactAnalyses    int `json:"exact_analyses"`
	ExpansionHops    int `json:"expansion_hops"`
	CallEndpoints    int `json:"call_endpoints"`
	SourceSlices     int `json:"source_slices"`
	SourceTextBytes  int `json:"source_text_bytes"`
	SourceSliceLines int `json:"source_slice_lines"`
}

type GoSelectionQuery struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

type GoSelectionCandidate struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Kind         string   `json:"kind"`
	Path         string   `json:"path"`
	Line         int      `json:"line"`
	Column       int      `json:"column"`
	QueryTermIDs []string `json:"query_term_ids"`
	Score        int      `json:"score"`
}

type GoSelectionSymbol struct {
	ID                 string   `json:"id"`
	Name               string   `json:"name"`
	Kind               string   `json:"kind"`
	Path               string   `json:"path"`
	StartLine          int      `json:"start_line"`
	StartColumn        int      `json:"start_column"`
	EndLine            int      `json:"end_line"`
	EndColumn          int      `json:"end_column"`
	Distance           int      `json:"distance"`
	SelectionReasonIDs []string `json:"selection_reason_ids"`
}

type GoSelectionCall struct {
	ID             string `json:"id"`
	CallerSymbolID string `json:"caller_symbol_id"`
	CalleeSymbolID string `json:"callee_symbol_id"`
	Path           string `json:"path"`
	StartLine      int    `json:"start_line"`
	StartColumn    int    `json:"start_column"`
	EndLine        int    `json:"end_line"`
	EndColumn      int    `json:"end_column"`
}

type GoSelectionProvenance struct {
	Provider         string   `json:"provider"`
	Version          string   `json:"version"`
	CollectorVersion string   `json:"collector_version"`
	Operations       []string `json:"operations"`
}

type GoSourceSelectionTrace struct {
	Version            string                 `json:"version"`
	Repository         GoSelectionRepository  `json:"repository"`
	Question           string                 `json:"question"`
	SeedDeclarationIDs []string               `json:"seed_declaration_ids,omitempty"`
	Limits             GoSelectionLimits      `json:"limits"`
	QueryTerms         []GoSelectionQuery     `json:"query_terms"`
	Candidates         []GoSelectionCandidate `json:"candidates"`
	SelectedSymbols    []GoSelectionSymbol    `json:"selected_symbols"`
	ExactCalls         []GoSelectionCall      `json:"exact_calls"`
	Provenance         GoSelectionProvenance  `json:"provenance"`
	Coverage           string                 `json:"coverage"`
	Warnings           []string               `json:"warnings"`
}

type GoSourceSelectionSlice struct {
	Path               string   `json:"path"`
	StartLine          int      `json:"start_line"`
	EndLine            int      `json:"end_line"`
	Text               string   `json:"text"`
	Truncated          bool     `json:"truncated,omitempty"`
	EnclosingSymbolID  string   `json:"enclosing_symbol_id"`
	SelectionReasonIDs []string `json:"selection_reason_ids"`
}

type GoSourceSelectionPacket struct {
	Version            string                   `json:"version"`
	Repository         GoSelectionRepository    `json:"repository"`
	Question           string                   `json:"question"`
	SeedDeclarationIDs []string                 `json:"seed_declaration_ids,omitempty"`
	Coverage           string                   `json:"coverage"`
	SourceSlices       []GoSourceSelectionSlice `json:"source_slices"`
}

type goSelectionAnalyzer interface {
	Analyze(context.Context, analysis.Request) (evidence.Graph, error)
	ResolveLocation(context.Context, analysis.LocationRequest) (analysis.LocationResolution, error)
	AnalyzeExactSymbol(context.Context, analysis.ExactSymbolRequest) (evidence.Graph, error)
}

type goSelectionCandidateState struct {
	entity   evidence.Entity
	queryIDs []string
	score    int
}

type goSelectionNode struct {
	entity           evidence.Entity
	distance         int
	parentKey        string
	parentEdgeKey    string
	root             bool
	seedID           string
	previewTruncated bool
}

type goSelectionEdge struct {
	from       string
	to         string
	location   evidence.Location
	provenance evidence.Provenance
}

type goSelectionFrontier struct {
	entity   evidence.Entity
	distance int
}

// SelectGoQuestionSources runs the experiment against one clean, pinned Go
// checkout. It returns deterministic projections rather than analyzer graphs,
// which contain machine-specific paths and build details.
func SelectGoQuestionSources(
	ctx context.Context,
	opts GoSourceSelectionOptions,
) (GoSourceSelectionTrace, GoSourceSelectionPacket, error) {
	adapter := gopls.New(gopls.Options{
		Binary:          opts.GoplsBinary,
		MaxSymbols:      goSelectionMaxHitsPerTerm,
		MaxCallRoots:    1,
		MaxCallers:      10,
		MaxCallees:      10,
		CommandTimeout:  goSelectionCommandTimeout,
		IncludeExternal: false,
	})
	return selectGoQuestionSources(ctx, opts, adapter)
}

// SelectGoAnchoredQuestionSources starts from declarations already selected by
// topic discovery. It never runs workspace_symbol; the question only ranks
// bounded exact expansion around the retained anchors.
func SelectGoAnchoredQuestionSources(
	ctx context.Context,
	opts GoSourceSelectionOptions,
	anchors []GoTopicDeclaration,
) (GoSourceSelectionTrace, GoSourceSelectionPacket, error) {
	adapter := gopls.New(gopls.Options{
		Binary:          opts.GoplsBinary,
		MaxSymbols:      goSelectionMaxHitsPerTerm,
		MaxCallRoots:    1,
		MaxCallers:      10,
		MaxCallees:      10,
		CommandTimeout:  goSelectionCommandTimeout,
		IncludeExternal: false,
	})
	return selectGoAnchoredQuestionSources(ctx, opts, anchors, adapter)
}

func selectGoQuestionSources(
	ctx context.Context,
	opts GoSourceSelectionOptions,
	adapter goSelectionAnalyzer,
) (GoSourceSelectionTrace, GoSourceSelectionPacket, error) {
	repoPath, repository, err := validateGoSelectionInput(opts)
	if err != nil {
		return GoSourceSelectionTrace{}, GoSourceSelectionPacket{}, err
	}
	queries, contentTerms, err := goSelectionQueries(opts.Question)
	if err != nil {
		return GoSourceSelectionTrace{}, GoSourceSelectionPacket{}, err
	}

	candidates, analyzerVersion, err := discoverGoSelectionCandidates(
		ctx,
		adapter,
		repoPath,
		queries,
		contentTerms,
	)
	if err != nil {
		return GoSourceSelectionTrace{}, GoSourceSelectionPacket{}, err
	}
	if len(candidates) == 0 {
		return GoSourceSelectionTrace{}, GoSourceSelectionPacket{}, fmt.Errorf(
			"go source selection: no callable repository symbol matched the question",
		)
	}

	root := candidates[0].entity
	nodes, edges, exactVersion, err := expandGoSelection(
		ctx,
		adapter,
		repoPath,
		root,
		contentTerms,
	)
	if err != nil {
		return GoSourceSelectionTrace{}, GoSourceSelectionPacket{}, err
	}
	if analyzerVersion == "" {
		analyzerVersion = exactVersion
	}
	selected, err := selectGoSelectionNodes(ctx, adapter, repoPath, nodes, edges, root, contentTerms)
	if err != nil {
		return GoSourceSelectionTrace{}, GoSourceSelectionPacket{}, err
	}
	selectedEdges := projectGoSelectionEdges(edges, selected)
	packet, retained, retainedEdges, err := buildGoSelectionPacket(
		repoPath,
		repository,
		opts.Question,
		selected,
		selectedEdges,
	)
	if err != nil {
		return GoSourceSelectionTrace{}, GoSourceSelectionPacket{}, err
	}
	if err := validateGoSelectionCheckout(repoPath, opts.ExpectedRevision); err != nil {
		return GoSourceSelectionTrace{}, GoSourceSelectionPacket{}, err
	}
	trace := buildGoSelectionTrace(
		repository,
		opts.Question,
		queries,
		candidates,
		retained,
		retainedEdges,
		analyzerVersion,
	)
	return trace, packet, nil
}

func selectGoAnchoredQuestionSources(
	ctx context.Context,
	opts GoSourceSelectionOptions,
	anchors []GoTopicDeclaration,
	adapter goSelectionAnalyzer,
) (GoSourceSelectionTrace, GoSourceSelectionPacket, error) {
	repoPath, repository, err := validateGoSelectionInput(opts)
	if err != nil {
		return GoSourceSelectionTrace{}, GoSourceSelectionPacket{}, err
	}
	queries, contentTerms, err := goSelectionQueries(opts.Question)
	if err != nil {
		return GoSourceSelectionTrace{}, GoSourceSelectionPacket{}, err
	}
	if len(anchors) < goTopicMinSupportSymbols ||
		len(anchors) > goTopicMaxSupportSymbols {
		return GoSourceSelectionTrace{}, GoSourceSelectionPacket{}, fmt.Errorf(
			"go source selection: anchor count must be between %d and %d",
			goTopicMinSupportSymbols,
			goTopicMaxSupportSymbols,
		)
	}

	roots := make([]goSelectionNode, 0, len(anchors))
	seedIDs := make([]string, 0, len(anchors))
	seenIDs := make(map[string]struct{}, goTopicMaxSupportSymbols)
	seenEntities := make(map[string]struct{}, goTopicMaxSupportSymbols)
	for _, anchor := range anchors {
		if _, duplicate := seenIDs[anchor.ID]; duplicate {
			return GoSourceSelectionTrace{}, GoSourceSelectionPacket{}, fmt.Errorf(
				"go source selection: duplicate anchor %q",
				anchor.ID,
			)
		}
		entity, truncated, err := resolveGoSelectionAnchor(repoPath, anchor)
		if err != nil {
			return GoSourceSelectionTrace{}, GoSourceSelectionPacket{}, fmt.Errorf(
				"go source selection: resolve anchor %s: %w",
				anchor.ID,
				err,
			)
		}
		key := goSelectionEntityKey(entity)
		if _, duplicate := seenEntities[key]; duplicate {
			return GoSourceSelectionTrace{}, GoSourceSelectionPacket{}, fmt.Errorf(
				"go source selection: anchors resolve to one declaration",
			)
		}
		seenIDs[anchor.ID] = struct{}{}
		seenEntities[key] = struct{}{}
		seedIDs = append(seedIDs, anchor.ID)
		roots = append(roots, goSelectionNode{
			entity:           entity,
			root:             true,
			seedID:           anchor.ID,
			previewTruncated: truncated,
		})
	}

	nodes, edges, analyzerVersion, err := expandGoSelectionRoots(
		ctx,
		adapter,
		repoPath,
		roots,
		contentTerms,
	)
	if err != nil {
		return GoSourceSelectionTrace{}, GoSourceSelectionPacket{}, err
	}
	selected, err := selectGoSelectionNodes(
		ctx,
		adapter,
		repoPath,
		nodes,
		edges,
		roots[0].entity,
		contentTerms,
	)
	if err != nil {
		return GoSourceSelectionTrace{}, GoSourceSelectionPacket{}, err
	}
	selectedEdges := projectGoSelectionEdges(edges, selected)
	packet, retained, retainedEdges, err := buildGoSelectionPacket(
		repoPath,
		repository,
		opts.Question,
		selected,
		selectedEdges,
	)
	if err != nil {
		return GoSourceSelectionTrace{}, GoSourceSelectionPacket{}, err
	}
	if err := validateGoSelectionCheckout(repoPath, opts.ExpectedRevision); err != nil {
		return GoSourceSelectionTrace{}, GoSourceSelectionPacket{}, err
	}
	trace := buildGoSelectionTrace(
		repository,
		opts.Question,
		queries,
		nil,
		retained,
		retainedEdges,
		analyzerVersion,
	)
	trace.SeedDeclarationIDs = append([]string(nil), seedIDs...)
	trace.Coverage = "anchor_seeded_non_exhaustive"
	trace.Provenance.Operations = []string{
		"ast_anchor_resolution",
		"call_hierarchy",
		"document_symbols",
	}
	trace.Warnings = []string{
		"all topic support declarations are retained as exact source anchors",
		"exact call expansion is capped and may omit relationships for retained anchors",
		"source previews are bounded and may truncate long anchor bodies",
	}
	packet.SeedDeclarationIDs = append([]string(nil), seedIDs...)
	packet.Coverage = trace.Coverage
	return trace, packet, nil
}

func validateGoSelectionInput(
	opts GoSourceSelectionOptions,
) (string, GoSelectionRepository, error) {
	if opts.RepositoryPath == "" {
		return "", GoSelectionRepository{}, fmt.Errorf("go source selection: repository path is required")
	}
	if opts.Question == "" ||
		len(opts.Question) > goSelectionMaxQuestionBytes ||
		!utf8.ValidString(opts.Question) ||
		strings.ContainsRune(opts.Question, 0) {
		return "", GoSelectionRepository{}, fmt.Errorf(
			"go source selection: question must be valid UTF-8 within %d bytes",
			goSelectionMaxQuestionBytes,
		)
	}
	if len(opts.ExpectedRevision) != 40 || !lowerHex(opts.ExpectedRevision) {
		return "", GoSelectionRepository{}, fmt.Errorf(
			"go source selection: expected revision must be a lowercase 40-byte commit",
		)
	}
	absolute, err := filepath.Abs(opts.RepositoryPath)
	if err != nil {
		return "", GoSelectionRepository{}, fmt.Errorf("go source selection: resolve repository: %w", err)
	}
	repoPath, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", GoSelectionRepository{}, fmt.Errorf("go source selection: resolve repository symlinks: %w", err)
	}
	revision, err := runGit(repoPath, "rev-parse", "HEAD")
	if err != nil {
		return "", GoSelectionRepository{}, err
	}
	if revision != opts.ExpectedRevision {
		return "", GoSelectionRepository{}, fmt.Errorf(
			"go source selection: revision is %s, want %s",
			revision,
			opts.ExpectedRevision,
		)
	}
	if err := validateGoSelectionCheckout(repoPath, opts.ExpectedRevision); err != nil {
		return "", GoSelectionRepository{}, err
	}
	return repoPath, GoSelectionRepository{
		Name:     filepath.Base(repoPath),
		Revision: revision,
	}, nil
}

func runGit(repoPath string, args ...string) (string, error) {
	commandCtx, cancel := context.WithTimeout(context.Background(), goSelectionGitTimeout)
	defer cancel()
	commandArgs := []string{
		"--no-pager",
		"-c", "core.fsmonitor=false",
		"-c", "core.hooksPath=" + os.DevNull,
		"-C", repoPath,
	}
	commandArgs = append(commandArgs, args...)
	command := exec.CommandContext(commandCtx, "git", commandArgs...)
	command.Env = append(
		filterGitEnvironment(os.Environ()),
		"GIT_OPTIONAL_LOCKS=0",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_SYSTEM="+os.DevNull,
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_PAGER=cat",
		"PAGER=cat",
	)
	output, err := command.Output()
	if commandCtx.Err() != nil {
		return "", fmt.Errorf(
			"go source selection: git %s timed out after %s",
			strings.Join(args, " "),
			goSelectionGitTimeout,
		)
	}
	if err != nil {
		return "", fmt.Errorf("go source selection: git %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(output)), nil
}

func validateGoSelectionCheckout(repoPath, expectedRevision string) error {
	revision, err := runGit(repoPath, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	if revision != expectedRevision {
		return fmt.Errorf(
			"go source selection: revision is %s, want %s",
			revision,
			expectedRevision,
		)
	}
	for _, args := range [][]string{
		{"diff", "--quiet", "--no-ext-diff", "HEAD", "--"},
		{"diff", "--cached", "--quiet", "--no-ext-diff", "HEAD", "--"},
	} {
		clean, err := runGitQuiet(repoPath, args...)
		if err != nil {
			return err
		}
		if !clean {
			return fmt.Errorf("go source selection: tracked repository state must be clean")
		}
	}
	untracked, err := runGitHasOutput(
		repoPath,
		"ls-files",
		"--others",
		"--exclude-standard",
		"-z",
		"--",
	)
	if err != nil {
		return err
	}
	if untracked {
		return fmt.Errorf("go source selection: untracked repository state must be clean")
	}
	return nil
}

func runGitQuiet(repoPath string, args ...string) (bool, error) {
	commandCtx, cancel := context.WithTimeout(context.Background(), goSelectionGitTimeout)
	defer cancel()
	commandArgs := []string{
		"--no-pager",
		"-c", "core.fsmonitor=false",
		"-c", "core.hooksPath=" + os.DevNull,
		"-C", repoPath,
	}
	commandArgs = append(commandArgs, args...)
	command := exec.CommandContext(commandCtx, "git", commandArgs...)
	command.Env = append(
		filterGitEnvironment(os.Environ()),
		"GIT_OPTIONAL_LOCKS=0",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_SYSTEM="+os.DevNull,
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_PAGER=cat",
		"PAGER=cat",
	)
	err := command.Run()
	if commandCtx.Err() != nil {
		return false, fmt.Errorf(
			"go source selection: git %s timed out after %s",
			strings.Join(args, " "),
			goSelectionGitTimeout,
		)
	}
	if err == nil {
		return true, nil
	}
	if exitError, ok := err.(*exec.ExitError); ok && exitError.ExitCode() == 1 {
		return false, nil
	}
	return false, fmt.Errorf("go source selection: git %s: %w", strings.Join(args, " "), err)
}

func runGitHasOutput(repoPath string, args ...string) (bool, error) {
	commandCtx, cancel := context.WithTimeout(context.Background(), goSelectionGitTimeout)
	defer cancel()
	commandArgs := []string{
		"--no-pager",
		"-c", "core.fsmonitor=false",
		"-c", "core.hooksPath=" + os.DevNull,
		"-C", repoPath,
	}
	commandArgs = append(commandArgs, args...)
	command := exec.CommandContext(commandCtx, "git", commandArgs...)
	command.Env = append(
		filterGitEnvironment(os.Environ()),
		"GIT_OPTIONAL_LOCKS=0",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_SYSTEM="+os.DevNull,
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_PAGER=cat",
		"PAGER=cat",
	)
	stdout, err := command.StdoutPipe()
	if err != nil {
		return false, fmt.Errorf("go source selection: git %s: %w", strings.Join(args, " "), err)
	}
	if err := command.Start(); err != nil {
		return false, fmt.Errorf("go source selection: git %s: %w", strings.Join(args, " "), err)
	}
	var firstByte [1]byte
	count, readErr := stdout.Read(firstByte[:])
	if count > 0 {
		cancel()
		_ = command.Wait()
		return true, nil
	}
	waitErr := command.Wait()
	if commandCtx.Err() != nil {
		return false, fmt.Errorf(
			"go source selection: git %s timed out after %s",
			strings.Join(args, " "),
			goSelectionGitTimeout,
		)
	}
	if readErr != nil && readErr != io.EOF {
		return false, fmt.Errorf(
			"go source selection: read git %s output: %w",
			strings.Join(args, " "),
			readErr,
		)
	}
	if waitErr != nil {
		return false, fmt.Errorf("go source selection: git %s: %w", strings.Join(args, " "), waitErr)
	}
	return false, nil
}

func filterGitEnvironment(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		name, _, _ := strings.Cut(value, "=")
		switch name {
		case "GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_COMMON_DIR",
			"GIT_OBJECT_DIRECTORY", "GIT_ALTERNATE_OBJECT_DIRECTORIES",
			"GIT_CONFIG", "GIT_CONFIG_COUNT", "GIT_CONFIG_PARAMETERS",
			"GIT_CONFIG_SYSTEM", "GIT_CONFIG_GLOBAL", "GIT_CONFIG_NOSYSTEM",
			"GIT_EXTERNAL_DIFF", "GIT_PAGER", "PAGER":
			continue
		}
		if strings.HasPrefix(name, "GIT_CONFIG_KEY_") ||
			strings.HasPrefix(name, "GIT_CONFIG_VALUE_") {
			continue
		}
		result = append(result, value)
	}
	return result
}

func lowerHex(value string) bool {
	for _, character := range value {
		if (character < '0' || character > '9') &&
			(character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func goSelectionQueries(question string) ([]GoSelectionQuery, []string, error) {
	words := strings.FieldsFunc(strings.ToLower(question), func(character rune) bool {
		return !unicode.IsLetter(character) && !unicode.IsDigit(character)
	})
	content := make([]string, 0, goSelectionMaxContentTerms)
	seen := make(map[string]struct{}, goSelectionMaxContentTerms)
	for _, word := range words {
		if len([]rune(word)) < 4 {
			continue
		}
		if _, stop := goSelectionStopWords[word]; stop {
			continue
		}
		word = normalizeGoSelectionTerm(word)
		if word == "" {
			continue
		}
		if _, duplicate := seen[word]; duplicate {
			continue
		}
		seen[word] = struct{}{}
		content = append(content, word)
		if len(content) == goSelectionMaxContentTerms {
			break
		}
	}
	if len(content) == 0 {
		return nil, nil, fmt.Errorf("go source selection: question has no usable content terms")
	}

	queryText := make([]string, 0, goSelectionMaxQueryTerms)
	if len(content) == 1 {
		queryText = append(queryText, content[0])
	} else {
		anchor := content[0]
		for _, term := range content[1:] {
			queryText = append(queryText, anchor+" "+term)
			if len(queryText) == goSelectionMaxQueryTerms {
				break
			}
		}
		for _, term := range content {
			if len(queryText) == goSelectionMaxQueryTerms {
				break
			}
			queryText = appendUniqueString(queryText, term)
		}
	}
	queries := make([]GoSelectionQuery, len(queryText))
	for index, text := range queryText {
		queries[index] = GoSelectionQuery{
			ID:   fmt.Sprintf("q%d", index+1),
			Text: text,
		}
	}
	return queries, content, nil
}

func normalizeGoSelectionTerm(value string) string {
	runes := []rune(value)
	if len(runes) > 5 && strings.HasSuffix(value, "ing") {
		runes = runes[:len(runes)-3]
		if len(runes) >= 2 && runes[len(runes)-1] == runes[len(runes)-2] {
			runes = runes[:len(runes)-1]
		}
	}
	if len(runes) > 5 && strings.HasSuffix(string(runes), "ed") {
		runes = runes[:len(runes)-2]
	}
	if len(runes) > 6 {
		runes = runes[:6]
	}
	return string(runes)
}

func appendUniqueString(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func discoverGoSelectionCandidates(
	ctx context.Context,
	adapter goSelectionAnalyzer,
	repoPath string,
	queries []GoSelectionQuery,
	contentTerms []string,
) ([]goSelectionCandidateState, string, error) {
	byKey := make(map[string]*goSelectionCandidateState, goSelectionMaxCandidates)
	version := ""
	for _, query := range queries {
		graph, err := adapter.Analyze(ctx, analysis.Request{
			RepoPath: repoPath,
			Query:    query.Text,
		})
		if err != nil {
			return nil, "", fmt.Errorf("go source selection: query %q: %w", query.Text, err)
		}
		if err := validateGoSelectionAnalyzerGraph(
			graph,
			goSelectionMaxDiscoveryItems,
		); err != nil {
			return nil, "", fmt.Errorf("go source selection: query %q: %w", query.Text, err)
		}
		if version == "" {
			version = graphProviderVersion(graph)
		}
		queryHits := make(map[string]struct{}, goSelectionMaxHitsPerTerm)
		for _, relation := range graph.Relations {
			if relation.Kind != evidence.RelationMatchesQuery {
				continue
			}
			entity, ok := graphEntity(graph, relation.To)
			if !ok || !goSelectionCallable(entity) || !goSelectionRepositoryEntity(entity) {
				continue
			}
			key := goSelectionEntityKey(entity)
			if _, exists := queryHits[key]; !exists {
				if len(queryHits) == goSelectionMaxHitsPerTerm {
					continue
				}
				queryHits[key] = struct{}{}
			}
			if existing, ok := byKey[key]; ok {
				existing.queryIDs = appendUniqueString(existing.queryIDs, query.ID)
				continue
			}
			if len(byKey) == goSelectionMaxCandidates {
				continue
			}
			copyEntity := cloneGoSelectionEntity(entity)
			byKey[key] = &goSelectionCandidateState{
				entity:   copyEntity,
				queryIDs: []string{query.ID},
			}
		}
	}

	result := make([]goSelectionCandidateState, 0, goSelectionMaxCandidates)
	for _, candidate := range byKey {
		sort.Strings(candidate.queryIDs)
		candidate.score = scoreGoSelectionCandidate(candidate.entity, candidate.queryIDs, contentTerms)
		result = append(result, *candidate)
	}
	sort.Slice(result, func(i, j int) bool {
		left, right := result[i], result[j]
		if left.score != right.score {
			return left.score > right.score
		}
		if len(left.queryIDs) != len(right.queryIDs) {
			return len(left.queryIDs) > len(right.queryIDs)
		}
		if goSelectionTestPath(left.entity.Location.Path) != goSelectionTestPath(right.entity.Location.Path) {
			return !goSelectionTestPath(left.entity.Location.Path)
		}
		return goSelectionEntityKey(left.entity) < goSelectionEntityKey(right.entity)
	})
	return result, version, nil
}

func scoreGoSelectionCandidate(
	entity evidence.Entity,
	queryIDs []string,
	contentTerms []string,
) int {
	name := goSelectionCallableName(entity.Name)
	nameTerms := splitGoSelectionName(name)
	score := 40 + len(queryIDs)*5
	if !goSelectionTestPath(entity.Location.Path) {
		score += 10
	}
	for _, term := range contentTerms {
		for _, nameTerm := range nameTerms {
			switch {
			case nameTerm == term:
				score += 30
			case strings.HasPrefix(nameTerm, term) || strings.HasPrefix(term, nameTerm):
				score += 10
			}
		}
		if name == term {
			score += 15
		}
	}
	return score
}

func splitGoSelectionName(value string) []string {
	var tokens []string
	var current []rune
	flush := func() {
		if len(current) == 0 {
			return
		}
		tokens = append(tokens, strings.ToLower(string(current)))
		current = nil
	}
	for index, character := range []rune(value) {
		if !unicode.IsLetter(character) && !unicode.IsDigit(character) {
			flush()
			continue
		}
		if index > 0 && unicode.IsUpper(character) && len(current) > 0 {
			flush()
		}
		current = append(current, character)
	}
	flush()
	return tokens
}

func expandGoSelection(
	ctx context.Context,
	adapter goSelectionAnalyzer,
	repoPath string,
	root evidence.Entity,
	contentTerms []string,
) ([]goSelectionNode, []goSelectionEdge, string, error) {
	return expandGoSelectionRoots(
		ctx,
		adapter,
		repoPath,
		[]goSelectionNode{{entity: root, root: true}},
		contentTerms,
	)
}

func expandGoSelectionRoots(
	ctx context.Context,
	adapter goSelectionAnalyzer,
	repoPath string,
	roots []goSelectionNode,
	contentTerms []string,
) ([]goSelectionNode, []goSelectionEdge, string, error) {
	if len(roots) == 0 {
		return nil, nil, "", fmt.Errorf("go source selection: exact roots are required")
	}
	nodes := make(map[string]goSelectionNode, goSelectionMaxCallEndpoints)
	frontier := make([]goSelectionFrontier, 0, len(roots))
	anchoredByPosition := make(map[string]evidence.Entity, len(roots))
	for _, root := range roots {
		key := goSelectionEntityKey(root.entity)
		if key == "" {
			return nil, nil, "", fmt.Errorf("go source selection: exact root is invalid")
		}
		nodes[key] = root
		frontier = append(frontier, goSelectionFrontier{entity: root.entity})
		if root.seedID != "" {
			anchoredByPosition[goSelectionPhysicalKey(root.entity)] = root.entity
		}
	}
	primary := roots[0].entity
	visited := make(map[string]struct{}, goSelectionMaxExactAnalyses)
	edges := make([]goSelectionEdge, 0, goSelectionMaxCallEndpoints)
	version := ""

	for len(frontier) > 0 && len(visited) < goSelectionMaxExactAnalyses {
		sort.Slice(frontier, func(i, j int) bool {
			return goSelectionFrontierLess(frontier[i], frontier[j], primary, contentTerms)
		})
		current := frontier[0]
		frontier = frontier[1:]
		key := goSelectionEntityKey(current.entity)
		if _, ok := visited[key]; ok || current.distance > goSelectionMaxExpansionHops {
			continue
		}
		visited[key] = struct{}{}
		graph, err := adapter.AnalyzeExactSymbol(ctx, analysis.ExactSymbolRequest{
			RepoPath: repoPath,
			Symbol:   current.entity,
		})
		if err != nil {
			return nil, nil, "", fmt.Errorf(
				"go source selection: exact symbol %s: %w",
				current.entity.Name,
				err,
			)
		}
		if err := validateGoSelectionAnalyzerGraph(
			graph,
			goSelectionMaxExactItems,
		); err != nil {
			return nil, nil, "", fmt.Errorf(
				"go source selection: exact symbol %s: %w",
				current.entity.Name,
				err,
			)
		}
		if version == "" {
			version = graphProviderVersion(graph)
		}
		for _, relation := range graph.Relations {
			if relation.Kind != evidence.RelationCalls {
				continue
			}
			from, fromOK := graphEntity(graph, relation.From)
			to, toOK := graphEntity(graph, relation.To)
			if !fromOK || !toOK ||
				!goSelectionCallable(from) || !goSelectionCallable(to) ||
				!goSelectionRepositoryEntity(from) || !goSelectionRepositoryEntity(to) {
				continue
			}
			if anchored, ok := anchoredByPosition[goSelectionPhysicalKey(from)]; ok {
				from = anchored
			}
			if anchored, ok := anchoredByPosition[goSelectionPhysicalKey(to)]; ok {
				to = anchored
			}
			callsite, provenance, ok := goSelectionCallsite(relation)
			if !ok || !goSelectionCanonicalPath(callsite.Path) {
				continue
			}
			fromKey := goSelectionEntityKey(from)
			toKey := goSelectionEntityKey(to)
			fromAllowed := goSelectionEndpointAllowed(nodes, fromKey, key, current.distance)
			toAllowed := goSelectionEndpointAllowed(nodes, toKey, key, current.distance)
			if !fromAllowed || !toAllowed {
				continue
			}
			candidateEdge := goSelectionEdge{
				from:       fromKey,
				to:         toKey,
				location:   callsite,
				provenance: provenance,
			}
			if len(edges) == goSelectionMaxCallEndpoints &&
				!hasGoSelectionEdge(edges, candidateEdge) {
				continue
			}
			edges = appendUniqueGoSelectionEdge(edges, candidateEdge)
			parentEdgeKey := goSelectionEdgeKey(candidateEdge)
			for _, neighbor := range []evidence.Entity{from, to} {
				neighborKey := goSelectionEntityKey(neighbor)
				if neighborKey == key {
					continue
				}
				existing, exists := nodes[neighborKey]
				distance := current.distance + 1
				if distance > goSelectionMaxExpansionHops && !exists {
					continue
				}
				if !exists && len(nodes) == goSelectionMaxCallEndpoints {
					continue
				}
				if !exists || distance < existing.distance {
					nodes[neighborKey] = goSelectionNode{
						entity:        cloneGoSelectionEntity(neighbor),
						distance:      distance,
						parentKey:     key,
						parentEdgeKey: parentEdgeKey,
					}
				}
				if current.distance < goSelectionMaxExpansionHops {
					frontier = appendGoSelectionFrontier(frontier, goSelectionFrontier{
						entity:   cloneGoSelectionEntity(neighbor),
						distance: distance,
					})
				}
			}
		}
	}

	nodeValues := make([]goSelectionNode, 0, goSelectionMaxCallEndpoints)
	for _, node := range nodes {
		nodeValues = append(nodeValues, node)
	}
	sort.Slice(nodeValues, func(i, j int) bool {
		return goSelectionEntityKey(nodeValues[i].entity) < goSelectionEntityKey(nodeValues[j].entity)
	})
	sortGoSelectionEdges(edges)
	return nodeValues, edges, version, nil
}

func goSelectionFrontierLess(
	left, right goSelectionFrontier,
	root evidence.Entity,
	contentTerms []string,
) bool {
	leftOverlap := goSelectionNameOverlap(left.entity.Name, contentTerms)
	rightOverlap := goSelectionNameOverlap(right.entity.Name, contentTerms)
	if leftOverlap != rightOverlap {
		return leftOverlap > rightOverlap
	}
	leftSamePath := left.entity.Location.Path == root.Location.Path
	rightSamePath := right.entity.Location.Path == root.Location.Path
	if leftSamePath != rightSamePath {
		return leftSamePath
	}
	if left.distance != right.distance {
		return left.distance < right.distance
	}
	if goSelectionTestPath(left.entity.Location.Path) != goSelectionTestPath(right.entity.Location.Path) {
		return !goSelectionTestPath(left.entity.Location.Path)
	}
	return goSelectionEntityKey(left.entity) < goSelectionEntityKey(right.entity)
}

func appendGoSelectionFrontier(
	values []goSelectionFrontier,
	value goSelectionFrontier,
) []goSelectionFrontier {
	key := goSelectionEntityKey(value.entity)
	for _, existing := range values {
		if goSelectionEntityKey(existing.entity) == key {
			return values
		}
	}
	return append(values, value)
}

func selectGoSelectionNodes(
	ctx context.Context,
	adapter goSelectionAnalyzer,
	repoPath string,
	nodes []goSelectionNode,
	edges []goSelectionEdge,
	root evidence.Entity,
	contentTerms []string,
) ([]goSelectionNode, error) {
	degree := make(map[string]int, goSelectionMaxCallEndpoints)
	for _, edge := range edges {
		degree[edge.from]++
		degree[edge.to]++
	}
	sort.Slice(nodes, func(i, j int) bool {
		left, right := nodes[i], nodes[j]
		if left.root != right.root {
			return left.root
		}
		leftOverlap := goSelectionNameOverlap(left.entity.Name, contentTerms)
		rightOverlap := goSelectionNameOverlap(right.entity.Name, contentTerms)
		if leftOverlap != rightOverlap {
			return leftOverlap > rightOverlap
		}
		leftSamePath := left.entity.Location.Path == root.Location.Path
		rightSamePath := right.entity.Location.Path == root.Location.Path
		if leftSamePath != rightSamePath {
			return leftSamePath
		}
		if left.distance != right.distance {
			return left.distance < right.distance
		}
		leftDegree := degree[goSelectionEntityKey(left.entity)]
		rightDegree := degree[goSelectionEntityKey(right.entity)]
		if leftDegree != rightDegree {
			return leftDegree > rightDegree
		}
		return goSelectionEntityKey(left.entity) < goSelectionEntityKey(right.entity)
	})

	selected := make([]goSelectionNode, 0, goSelectionMaxSlices)
	selectedKeys := make(map[string]struct{}, goSelectionMaxSlices)
	attempts := 0
	for progress := true; progress && len(selected) < goSelectionMaxSlices; {
		progress = false
		for _, node := range nodes {
			if len(selected) == goSelectionMaxSlices {
				break
			}
			key := goSelectionEntityKey(node.entity)
			if _, exists := selectedKeys[key]; exists {
				continue
			}
			if !node.root {
				if _, parentSelected := selectedKeys[node.parentKey]; !parentSelected {
					continue
				}
			}
			if node.seedID == "" {
				if attempts == goSelectionMaxResolveAttempts {
					break
				}
				attempts++
				resolved, err := resolveGoSelectionRange(
					ctx,
					adapter,
					repoPath,
					node.entity,
					contentTerms,
				)
				if err != nil {
					continue
				}
				node.entity = resolved
			}
			selected = append(selected, node)
			selectedKeys[key] = struct{}{}
			progress = true
		}
	}
	if len(selected) == 0 || !selected[0].root {
		return nil, fmt.Errorf("go source selection: selected root has no exact source scope")
	}
	return selected, nil
}

func resolveGoSelectionRange(
	ctx context.Context,
	adapter goSelectionAnalyzer,
	repoPath string,
	entity evidence.Entity,
	contentTerms []string,
) (evidence.Entity, error) {
	result, err := adapter.ResolveLocation(ctx, analysis.LocationRequest{
		RepoPath:      repoPath,
		Location:      *entity.Location,
		MaxCandidates: 8,
		RankTerms:     contentTerms,
	})
	if err != nil {
		return evidence.Entity{}, err
	}
	if err := validateGoSelectionResolution(result); err != nil {
		return evidence.Entity{}, err
	}
	for _, candidate := range result.Candidates {
		location := candidate.Entity.Location
		if !candidate.Investigable || location == nil ||
			location.Path != entity.Location.Path ||
			location.Line != entity.Location.Line ||
			location.Column != entity.Location.Column ||
			goSelectionCallableName(candidate.Entity.Name) != goSelectionCallableName(entity.Name) {
			continue
		}
		if location.EndLine < location.Line ||
			location.EndLine-location.Line+1 > goSelectionMaxSliceLines {
			return evidence.Entity{}, fmt.Errorf("source scope exceeds line budget")
		}
		fullRange, err := resolveGoFunctionRange(repoPath, entity)
		if err != nil {
			return evidence.Entity{}, err
		}
		resolved := cloneGoSelectionEntity(entity)
		resolved.Location = &fullRange
		return resolved, nil
	}
	return evidence.Entity{}, fmt.Errorf("exact source scope was not resolved")
}

func resolveGoSelectionAnchor(
	repoPath string,
	anchor GoTopicDeclaration,
) (evidence.Entity, bool, error) {
	if anchor.ID == "" ||
		!goSelectionCanonicalPath(anchor.Path) ||
		(anchor.Kind != "function" && anchor.Kind != "method") ||
		!goTopicScalar(anchor.Name, goTopicMaxTextBytes, false) ||
		!goTopicScalar(anchor.Receiver, goTopicMaxTextBytes, true) ||
		!goTopicScalar(anchor.Signature, goTopicMaxTextBytes, false) {
		return evidence.Entity{}, false, fmt.Errorf("anchor declaration is invalid")
	}
	if err := verifyGoSelectionTracked(repoPath, anchor.Path); err != nil {
		return evidence.Entity{}, false, err
	}
	reader, err := reporead.New(repoPath)
	if err != nil {
		return evidence.Entity{}, false, err
	}
	defer reader.Close()
	content, err := reader.ReadFileNoSymlinks(anchor.Path, goTopicMaxSourceFileBytes)
	if err != nil {
		return evidence.Entity{}, false, err
	}
	if content.Truncated {
		return evidence.Entity{}, false, fmt.Errorf(
			"anchor source exceeds %d bytes",
			goTopicMaxSourceFileBytes,
		)
	}

	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(
		fileSet,
		anchor.Path,
		content.Bytes,
		parser.SkipObjectResolution,
	)
	if err != nil {
		return evidence.Entity{}, false, fmt.Errorf("parse anchor source: %w", err)
	}
	var matched *ast.FuncDecl
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Name == nil || function.Name.Name != anchor.Name {
			continue
		}
		kind := "function"
		receiver := ""
		if function.Recv != nil && len(function.Recv.List) > 0 {
			kind = "method"
			receiver, err = goTopicFormatNode(fileSet, function.Recv.List[0].Type)
			if err != nil {
				return evidence.Entity{}, false, err
			}
		}
		signature, err := goTopicFormatNode(fileSet, function.Type)
		if err != nil {
			return evidence.Entity{}, false, err
		}
		if kind != anchor.Kind ||
			receiver != anchor.Receiver ||
			signature != anchor.Signature {
			continue
		}
		if matched != nil {
			return evidence.Entity{}, false, fmt.Errorf("anchor declaration is ambiguous")
		}
		matched = function
	}
	if matched == nil {
		return evidence.Entity{}, false, fmt.Errorf("anchor declaration no longer matches source")
	}

	start := fileSet.Position(matched.Name.Pos())
	end := fileSet.Position(matched.End())
	if start.Line <= 0 || start.Column <= 0 || end.Line < start.Line {
		return evidence.Entity{}, false, fmt.Errorf("anchor declaration range is invalid")
	}
	lines := bytes.Split(content.Bytes, []byte("\n"))
	maxEndLine := min(end.Line, start.Line+goSelectionMaxSliceLines-1)
	previewEndLine := start.Line - 1
	previewBytes := 0
	for line := start.Line; line <= maxEndLine && line <= len(lines); line++ {
		lineBytes := len(lines[line-1]) + 1
		if previewBytes+lineBytes > goSelectionMaxAnchorBytes {
			break
		}
		previewBytes += lineBytes
		previewEndLine = line
	}
	if previewEndLine < start.Line {
		return evidence.Entity{}, false, fmt.Errorf(
			"anchor first source line exceeds %d bytes",
			goSelectionMaxAnchorBytes,
		)
	}
	truncated := previewEndLine < end.Line
	endColumn := end.Column
	if truncated {
		endColumn = len(lines[previewEndLine-1]) + 1
	}
	location := evidence.Location{
		Path:      anchor.Path,
		Line:      start.Line,
		Column:    start.Column,
		EndLine:   previewEndLine,
		EndColumn: endColumn,
	}
	kind := evidence.EntityFunction
	if anchor.Kind == "method" {
		kind = evidence.EntityMethod
	}
	entity := evidence.Entity{
		Kind:     kind,
		Name:     anchor.Name,
		Language: "go",
		Scope:    evidence.SourceScopeRepository,
		Location: &location,
	}
	entity.ID = goSelectionEntityKey(entity)
	return entity, truncated, nil
}

func resolveGoFunctionRange(
	repoPath string,
	entity evidence.Entity,
) (evidence.Location, error) {
	if entity.Location == nil || !goSelectionCanonicalPath(entity.Location.Path) {
		return evidence.Location{}, fmt.Errorf("function range requires a canonical source location")
	}
	if err := verifyGoSelectionTracked(repoPath, entity.Location.Path); err != nil {
		return evidence.Location{}, err
	}
	reader, err := reporead.New(repoPath)
	if err != nil {
		return evidence.Location{}, err
	}
	defer reader.Close()
	content, err := reader.ReadFileNoSymlinks(
		entity.Location.Path,
		goSelectionMaxSourceFileBytes,
	)
	if err != nil {
		return evidence.Location{}, err
	}
	if content.Truncated {
		return evidence.Location{}, fmt.Errorf(
			"Go source exceeds %d bytes",
			goSelectionMaxSourceFileBytes,
		)
	}
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(
		fileSet,
		entity.Location.Path,
		content.Bytes,
		parser.SkipObjectResolution,
	)
	if err != nil {
		return evidence.Location{}, fmt.Errorf("parse selected Go source: %w", err)
	}
	name := goSelectionCallableName(entity.Name)
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Name == nil || function.Name.Name != name {
			continue
		}
		start := fileSet.Position(function.Name.Pos())
		if start.Line != entity.Location.Line || start.Column != entity.Location.Column {
			continue
		}
		end := fileSet.Position(function.End())
		if end.Line < start.Line ||
			end.Line-start.Line+1 > goSelectionMaxSliceLines {
			return evidence.Location{}, fmt.Errorf(
				"selected function range exceeds %d lines",
				goSelectionMaxSliceLines,
			)
		}
		return evidence.Location{
			Path:      entity.Location.Path,
			Line:      start.Line,
			Column:    start.Column,
			EndLine:   end.Line,
			EndColumn: end.Column,
		}, nil
	}
	return evidence.Location{}, fmt.Errorf("selected symbol is not a concrete Go function declaration")
}

func projectGoSelectionEdges(
	edges []goSelectionEdge,
	selected []goSelectionNode,
) []goSelectionEdge {
	known := make(map[string]struct{}, goSelectionMaxSlices)
	for _, node := range selected {
		known[goSelectionEntityKey(node.entity)] = struct{}{}
	}
	result := make([]goSelectionEdge, 0, goSelectionMaxCallEndpoints)
	for _, edge := range edges {
		if _, ok := known[edge.from]; !ok {
			continue
		}
		if _, ok := known[edge.to]; !ok {
			continue
		}
		result = append(result, edge)
	}
	sortGoSelectionEdges(result)
	return result
}

func buildGoSelectionPacket(
	repoPath string,
	repository GoSelectionRepository,
	question string,
	selected []goSelectionNode,
	edges []goSelectionEdge,
) (GoSourceSelectionPacket, []goSelectionNode, []goSelectionEdge, error) {
	reader, err := reporead.New(repoPath)
	if err != nil {
		return GoSourceSelectionPacket{}, nil, nil, err
	}
	defer reader.Close()

	slices := make([]GoSourceSelectionSlice, 0, goSelectionMaxSlices)
	retained := make([]goSelectionNode, 0, goSelectionMaxSlices)
	retainedKeys := make(map[string]struct{}, goSelectionMaxSlices)
	totalBytes := 0
	for _, node := range selected {
		location := node.entity.Location
		if location == nil || !goSelectionCanonicalPath(location.Path) {
			continue
		}
		if !node.root {
			if _, parentRetained := retainedKeys[node.parentKey]; !parentRetained {
				continue
			}
		}
		if err := verifyGoSelectionTracked(repoPath, location.Path); err != nil {
			continue
		}
		content, err := reader.ReadFileNoSymlinks(location.Path, goSelectionMaxSourceFileBytes)
		if err != nil || content.Truncated {
			continue
		}
		text, err := goSelectionLineRange(content.Bytes, location.Line, location.EndLine)
		if err != nil || totalBytes+len(text) > goSelectionMaxSourceBytes {
			continue
		}
		slices = append(slices, GoSourceSelectionSlice{
			Path:              location.Path,
			StartLine:         location.Line,
			EndLine:           location.EndLine,
			Text:              text,
			Truncated:         node.previewTruncated,
			EnclosingSymbolID: goSelectionEntityKey(node.entity),
		})
		retained = append(retained, node)
		retainedKeys[goSelectionEntityKey(node.entity)] = struct{}{}
		totalBytes += len(text)
	}
	if len(slices) == 0 || !retained[0].root {
		return GoSourceSelectionPacket{}, nil, nil, fmt.Errorf(
			"go source selection: root source does not fit the packet budget",
		)
	}
	retainedEdges := projectGoSelectionEdges(edges, retained)
	edgeIDs := goSelectionEdgeIDs(retainedEdges)
	for index, node := range retained {
		slices[index].SelectionReasonIDs = goSelectionNodeReasons(
			node,
			retainedEdges,
			edgeIDs,
		)
	}
	return GoSourceSelectionPacket{
		Version:      goSelectionVersion,
		Repository:   repository,
		Question:     question,
		Coverage:     "selected_symbol_targets_only_non_exhaustive",
		SourceSlices: slices,
	}, retained, retainedEdges, nil
}

func buildGoSelectionTrace(
	repository GoSelectionRepository,
	question string,
	queries []GoSelectionQuery,
	candidates []goSelectionCandidateState,
	selected []goSelectionNode,
	edges []goSelectionEdge,
	analyzerVersion string,
) GoSourceSelectionTrace {
	traceCandidates := make([]GoSelectionCandidate, len(candidates))
	for index, candidate := range candidates {
		location := candidate.entity.Location
		traceCandidates[index] = GoSelectionCandidate{
			ID:           goSelectionEntityKey(candidate.entity),
			Name:         candidate.entity.Name,
			Kind:         string(candidate.entity.Kind),
			Path:         location.Path,
			Line:         location.Line,
			Column:       location.Column,
			QueryTermIDs: append([]string(nil), candidate.queryIDs...),
			Score:        candidate.score,
		}
	}
	edgeIDs := goSelectionEdgeIDs(edges)
	traceSymbols := make([]GoSelectionSymbol, len(selected))
	for index, node := range selected {
		location := node.entity.Location
		traceSymbols[index] = GoSelectionSymbol{
			ID:                 goSelectionEntityKey(node.entity),
			Name:               node.entity.Name,
			Kind:               string(node.entity.Kind),
			Path:               location.Path,
			StartLine:          location.Line,
			StartColumn:        location.Column,
			EndLine:            location.EndLine,
			EndColumn:          location.EndColumn,
			Distance:           node.distance,
			SelectionReasonIDs: goSelectionNodeReasons(node, edges, edgeIDs),
		}
	}
	traceEdges := make([]GoSelectionCall, len(edges))
	for index, edge := range edges {
		traceEdges[index] = GoSelectionCall{
			ID:             edgeIDs[goSelectionEdgeKey(edge)],
			CallerSymbolID: edge.from,
			CalleeSymbolID: edge.to,
			Path:           edge.location.Path,
			StartLine:      edge.location.Line,
			StartColumn:    edge.location.Column,
			EndLine:        edge.location.EndLine,
			EndColumn:      edge.location.EndColumn,
		}
	}
	return GoSourceSelectionTrace{
		Version:    goSelectionVersion,
		Repository: repository,
		Question:   question,
		Limits: GoSelectionLimits{
			QueryTerms:       goSelectionMaxQueryTerms,
			HitsPerTerm:      goSelectionMaxHitsPerTerm,
			Candidates:       goSelectionMaxCandidates,
			ExactAnalyses:    goSelectionMaxExactAnalyses,
			ExpansionHops:    goSelectionMaxExpansionHops,
			CallEndpoints:    goSelectionMaxCallEndpoints,
			SourceSlices:     goSelectionMaxSlices,
			SourceTextBytes:  goSelectionMaxSourceBytes,
			SourceSliceLines: goSelectionMaxSliceLines,
		},
		QueryTerms:      queries,
		Candidates:      traceCandidates,
		SelectedSymbols: traceSymbols,
		ExactCalls:      traceEdges,
		Provenance: GoSelectionProvenance{
			Provider:         "gopls",
			Version:          analyzerVersion,
			CollectorVersion: goSelectionVersion,
			Operations:       []string{"workspace_symbol", "call_hierarchy", "document_symbols"},
		},
		Coverage: "selected_symbol_targets_only_non_exhaustive",
		Warnings: []string{
			"workspace symbol matches are capped per query before deterministic local ranking",
			"call coverage contains only bounded exact analyses and selected repository-local endpoints",
		},
	}
}

func goSelectionNodeReasons(
	node goSelectionNode,
	edges []goSelectionEdge,
	edgeIDs map[string]string,
) []string {
	if node.root {
		if node.seedID != "" {
			return []string{"seed", "anchor:" + node.seedID}
		}
		return []string{"seed"}
	}
	if node.parentEdgeKey != "" {
		if id := edgeIDs[node.parentEdgeKey]; id != "" {
			return []string{id}
		}
	}
	key := goSelectionEntityKey(node.entity)
	reasons := make([]string, 0, 4)
	for _, edge := range edges {
		if edge.from == key || edge.to == key {
			if id := edgeIDs[goSelectionEdgeKey(edge)]; id != "" {
				reasons = append(reasons, id)
			}
		}
	}
	sort.Strings(reasons)
	if len(reasons) > 4 {
		reasons = reasons[:4]
	}
	return reasons
}

func goSelectionEdgeIDs(edges []goSelectionEdge) map[string]string {
	result := make(map[string]string, goSelectionMaxCallEndpoints)
	for index, edge := range edges {
		result[goSelectionEdgeKey(edge)] = fmt.Sprintf("e%d", index+1)
	}
	return result
}

func goSelectionLineRange(data []byte, startLine, endLine int) (string, error) {
	if startLine <= 0 || endLine < startLine ||
		endLine-startLine+1 > goSelectionMaxSliceLines {
		return "", fmt.Errorf("invalid source line range")
	}
	lines := bytes.Split(data, []byte("\n"))
	if endLine > len(lines) {
		return "", fmt.Errorf("source line range exceeds file")
	}
	selected := bytes.Join(lines[startLine-1:endLine], []byte("\n"))
	selected = append(selected, '\n')
	if !utf8.Valid(selected) || bytes.IndexByte(selected, 0) >= 0 {
		return "", fmt.Errorf("source range is not valid UTF-8 text")
	}
	return string(selected), nil
}

func verifyGoSelectionTracked(repoPath, sourcePath string) error {
	if !goSelectionCanonicalPath(sourcePath) {
		return fmt.Errorf("source path is not canonical")
	}
	output, err := runGit(
		repoPath,
		"ls-files",
		"-v",
		"-z",
		"--error-unmatch",
		"--",
		":(literal)"+sourcePath,
	)
	if err != nil {
		return err
	}
	if output != "H "+sourcePath+"\x00" {
		return fmt.Errorf("source path is not one exact ordinary tracked file")
	}
	return nil
}

func goSelectionCanonicalPath(value string) bool {
	return value != "" &&
		len(value) <= goSelectionMaxPathBytes &&
		utf8.ValidString(value) &&
		!strings.ContainsRune(value, 0) &&
		!strings.Contains(value, `\`) &&
		!strings.Contains(value, ":") &&
		!path.IsAbs(value) &&
		path.Clean(value) == value &&
		value != "." &&
		value != ".." &&
		!strings.HasPrefix(value, "../")
}

func graphEntity(graph evidence.Graph, id string) (evidence.Entity, bool) {
	limit := min(len(graph.Entities), goSelectionMaxDiscoveryItems)
	for _, entity := range graph.Entities[:limit] {
		if entity.ID == id {
			return entity, true
		}
	}
	return evidence.Entity{}, false
}

func validateGoSelectionAnalyzerGraph(graph evidence.Graph, itemLimit int) error {
	if len(graph.Entities) > itemLimit || len(graph.Relations) > itemLimit {
		return fmt.Errorf("exceeded the bounded analyzer graph")
	}
	if len(graph.Warnings) > goSelectionMaxWarnings {
		return fmt.Errorf("analyzer warnings exceed the processing budget")
	}
	for _, warning := range graph.Warnings {
		if !goSelectionAnalyzerText(warning, true) {
			return fmt.Errorf("analyzer warning exceeds the scalar budget")
		}
	}
	for _, entity := range graph.Entities {
		if !goSelectionAnalyzerText(entity.ID, false) ||
			!goSelectionAnalyzerText(entity.Name, false) ||
			!goSelectionAnalyzerText(entity.Language, true) {
			return fmt.Errorf("analyzer entity exceeds the scalar budget")
		}
		if entity.Location != nil && !goSelectionAnalyzerLocation(*entity.Location) {
			return fmt.Errorf("analyzer entity location exceeds the scalar budget")
		}
	}
	for _, relation := range graph.Relations {
		if !goSelectionAnalyzerText(relation.From, false) ||
			!goSelectionAnalyzerText(relation.To, false) {
			return fmt.Errorf("analyzer relation endpoint exceeds the scalar budget")
		}
		if len(relation.Provenance) > goSelectionMaxProvenance {
			return fmt.Errorf("analyzer provenance exceeds the processing budget")
		}
		for _, provenance := range relation.Provenance {
			if !goSelectionAnalyzerProvenance(provenance) {
				return fmt.Errorf("analyzer provenance exceeds the scalar budget")
			}
		}
		if len(relation.Scenarios) > goSelectionMaxProvenance {
			return fmt.Errorf("analyzer relation scenarios exceed the processing budget")
		}
		for _, scenario := range relation.Scenarios {
			if !goSelectionAnalyzerText(scenario, false) {
				return fmt.Errorf("analyzer relation scenario exceeds the scalar budget")
			}
		}
	}
	return nil
}

func validateGoSelectionResolution(result analysis.LocationResolution) error {
	if len(result.Candidates) > goSelectionMaxResolveResults {
		return fmt.Errorf("location candidates exceed the processing budget")
	}
	if len(result.Warnings) > goSelectionMaxWarnings {
		return fmt.Errorf("location warnings exceed the processing budget")
	}
	for _, warning := range result.Warnings {
		if !goSelectionAnalyzerText(warning, true) {
			return fmt.Errorf("location warning exceeds the scalar budget")
		}
	}
	for _, candidate := range result.Candidates {
		if !goSelectionAnalyzerText(candidate.Entity.ID, false) ||
			!goSelectionAnalyzerText(candidate.Entity.Name, false) ||
			!goSelectionAnalyzerText(candidate.Entity.Language, true) ||
			!goSelectionAnalyzerText(candidate.Match, true) {
			return fmt.Errorf("location candidate exceeds the scalar budget")
		}
		if candidate.Entity.Location != nil &&
			!goSelectionAnalyzerLocation(*candidate.Entity.Location) {
			return fmt.Errorf("location candidate path exceeds the scalar budget")
		}
		if len(candidate.RankReasons) > goSelectionMaxRankReasons {
			return fmt.Errorf("location candidate reasons exceed the processing budget")
		}
		for _, reason := range candidate.RankReasons {
			if !goSelectionAnalyzerText(reason, true) {
				return fmt.Errorf("location candidate reason exceeds the scalar budget")
			}
		}
	}
	return nil
}

func goSelectionAnalyzerProvenance(value evidence.Provenance) bool {
	return goSelectionAnalyzerText(value.Provider, false) &&
		goSelectionAnalyzerText(value.Version, true) &&
		goSelectionAnalyzerText(value.Operation, false) &&
		goSelectionAnalyzerText(value.Detail, true) &&
		(value.Location == nil || goSelectionAnalyzerLocation(*value.Location))
}

func goSelectionAnalyzerLocation(value evidence.Location) bool {
	return goSelectionAnalyzerText(value.Path, false)
}

func goSelectionAnalyzerText(value string, allowEmpty bool) bool {
	if len(value) > goSelectionMaxAnalyzerText || (!allowEmpty && value == "") {
		return false
	}
	return utf8.ValidString(value) && !strings.ContainsRune(value, 0)
}

func graphProviderVersion(graph evidence.Graph) string {
	limit := min(len(graph.Relations), goSelectionMaxDiscoveryItems)
	for _, relation := range graph.Relations[:limit] {
		for _, provenance := range relation.Provenance {
			if provenance.Provider == "gopls" && provenance.Version != "" {
				return provenance.Version
			}
		}
	}
	return ""
}

func goSelectionCallsite(
	relation evidence.Relation,
) (evidence.Location, evidence.Provenance, bool) {
	for _, provenance := range relation.Provenance {
		if provenance.Provider == "gopls" &&
			provenance.Operation == "call_hierarchy" &&
			provenance.Location != nil {
			return *provenance.Location, provenance, true
		}
	}
	return evidence.Location{}, evidence.Provenance{}, false
}

func appendUniqueGoSelectionEdge(
	edges []goSelectionEdge,
	edge goSelectionEdge,
) []goSelectionEdge {
	key := goSelectionEdgeKey(edge)
	for _, existing := range edges {
		if goSelectionEdgeKey(existing) == key {
			return edges
		}
	}
	return append(edges, edge)
}

func hasGoSelectionEdge(edges []goSelectionEdge, edge goSelectionEdge) bool {
	key := goSelectionEdgeKey(edge)
	for _, existing := range edges {
		if goSelectionEdgeKey(existing) == key {
			return true
		}
	}
	return false
}

func goSelectionEndpointAllowed(
	nodes map[string]goSelectionNode,
	endpointKey string,
	currentKey string,
	currentDistance int,
) bool {
	if endpointKey == currentKey {
		return true
	}
	if _, exists := nodes[endpointKey]; exists {
		return true
	}
	return currentDistance+1 <= goSelectionMaxExpansionHops &&
		len(nodes) < goSelectionMaxCallEndpoints
}

func sortGoSelectionEdges(edges []goSelectionEdge) {
	sort.Slice(edges, func(i, j int) bool {
		return goSelectionEdgeKey(edges[i]) < goSelectionEdgeKey(edges[j])
	})
}

func goSelectionEdgeKey(edge goSelectionEdge) string {
	location := edge.location
	return fmt.Sprintf(
		"%s\x00%s\x00%s\x00%09d\x00%09d\x00%09d\x00%09d",
		edge.from,
		edge.to,
		location.Path,
		location.Line,
		location.Column,
		location.EndLine,
		location.EndColumn,
	)
}

func goSelectionEntityKey(entity evidence.Entity) string {
	location := entity.Location
	if location == nil {
		return ""
	}
	return fmt.Sprintf(
		"%s:%s:%d:%d:%s",
		entity.Kind,
		location.Path,
		location.Line,
		location.Column,
		goSelectionCallableName(entity.Name),
	)
}

func goSelectionPhysicalKey(entity evidence.Entity) string {
	location := entity.Location
	if location == nil {
		return ""
	}
	return fmt.Sprintf(
		"%s:%d:%d:%s",
		location.Path,
		location.Line,
		location.Column,
		goSelectionCallableName(entity.Name),
	)
}

func cloneGoSelectionEntity(entity evidence.Entity) evidence.Entity {
	copyEntity := entity
	if entity.Location != nil {
		location := *entity.Location
		copyEntity.Location = &location
	}
	return copyEntity
}

func goSelectionCallableName(value string) string {
	if separator := strings.LastIndexByte(value, '.'); separator >= 0 {
		return value[separator+1:]
	}
	return value
}

func goSelectionCallable(entity evidence.Entity) bool {
	return entity.Location != nil &&
		(entity.Kind == evidence.EntityFunction || entity.Kind == evidence.EntityMethod)
}

func goSelectionRepositoryEntity(entity evidence.Entity) bool {
	return goSelectionCallable(entity) &&
		len(entity.ID) <= 512 &&
		len(entity.Name) <= 240 &&
		utf8.ValidString(entity.Name) &&
		!strings.ContainsRune(entity.Name, 0) &&
		(entity.Scope == "" || entity.Scope == evidence.SourceScopeRepository) &&
		goSelectionCanonicalPath(entity.Location.Path)
}

func goSelectionNameOverlap(name string, contentTerms []string) int {
	score := 0
	for _, nameTerm := range splitGoSelectionName(goSelectionCallableName(name)) {
		for _, contentTerm := range contentTerms {
			if nameTerm == contentTerm {
				score += 3
			} else if strings.HasPrefix(nameTerm, contentTerm) ||
				strings.HasPrefix(contentTerm, nameTerm) {
				score++
			}
		}
	}
	return score
}

func goSelectionTestPath(value string) bool {
	lower := strings.ToLower(value)
	if strings.HasSuffix(lower, "_test.go") {
		return true
	}
	for _, segment := range strings.Split(lower, "/") {
		switch segment {
		case "test", "tests", "testdata", "benchmark", "benchmarks":
			return true
		}
	}
	return false
}

// EncodeGoSourceSelection provides one canonical JSON representation for
// byte-identical replay checks.
func EncodeGoSourceSelection(value any) ([]byte, error) {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(encoded, '\n'), nil
}
