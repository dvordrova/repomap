package semanticmap

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	analysis "github.com/dvordrova/repomap/internal/analyzer"
	"github.com/dvordrova/repomap/internal/analyzer/python/pyright"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/reporead"
)

const (
	pythonSelectionVersion               = "semantic-map-python-selector-v1"
	pythonSelectionMaxQuestionBytes      = 512
	pythonSelectionMaxQueryTerms         = 3
	pythonSelectionMaxHitsPerQuery       = 64
	pythonSelectionMaxHitUnion           = pythonSelectionMaxQueryTerms * pythonSelectionMaxHitsPerQuery
	pythonSelectionMaxCandidates         = 96
	pythonSelectionMinCandidatesPerQuery = 24
	pythonSelectionMaxRootsPerQuery      = 3
	pythonSelectionMaxRootAnalyses       = pythonSelectionMaxQueryTerms * pythonSelectionMaxRootsPerQuery
	pythonSelectionMaxBridgeAnalyses     = 2
	pythonSelectionMaxExactAnalyses      = pythonSelectionMaxRootAnalyses + pythonSelectionMaxBridgeAnalyses
	pythonSelectionMaxGraphItems         = 96
	pythonSelectionMaxCallEndpoints      = 64
	pythonSelectionMaxResolveResults     = 8
	pythonSelectionMaxWarnings           = 24
	pythonSelectionMaxSlices             = 12
	pythonSelectionMaxRangeCandidates    = 16
	pythonSelectionMaxSourceBytes        = 24 << 10
	pythonSelectionMaxSliceLines         = 201
	pythonSelectionMaxSourceFile         = 4 << 20
	pythonSelectionRequestTimeout        = 30 * time.Second
)

var pythonSelectionFillerWords = map[string]struct{}{
	"become": {}, "becomes": {}, "code": {}, "executable": {}, "execute": {},
	"executes": {}, "execution": {}, "named": {}, "project": {}, "repository": {},
}

type PythonSourceSelectionOptions struct {
	RepositoryPath   string
	ExpectedRevision string
	Question         string
	PyrightBinary    string
}

type PythonSelectionLimits struct {
	QueryTerms       int `json:"query_terms"`
	HitsPerQuery     int `json:"hits_per_query"`
	Candidates       int `json:"candidates"`
	RootsPerQuery    int `json:"roots_per_query"`
	ExactAnalyses    int `json:"exact_analyses"`
	CallEndpoints    int `json:"call_endpoints"`
	SourceSlices     int `json:"source_slices"`
	SourceTextBytes  int `json:"source_text_bytes"`
	SourceSliceLines int `json:"source_slice_lines"`
}

type PythonSelectionQuery struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

type PythonSelectionCandidate struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Container    string   `json:"container,omitempty"`
	Kind         string   `json:"kind"`
	Path         string   `json:"path"`
	Line         int      `json:"line"`
	Column       int      `json:"column"`
	QueryTermIDs []string `json:"query_term_ids"`
	Score        int      `json:"score"`
}

type PythonSelectionSymbol struct {
	ID                 string   `json:"id"`
	Name               string   `json:"name"`
	Kind               string   `json:"kind"`
	Path               string   `json:"path"`
	StartLine          int      `json:"start_line"`
	StartColumn        int      `json:"start_column"`
	EndLine            int      `json:"end_line"`
	EndColumn          int      `json:"end_column"`
	SelectionReasonIDs []string `json:"selection_reason_ids"`
}

type PythonSelectionCall struct {
	ID             string `json:"id"`
	CallerSymbolID string `json:"caller_symbol_id"`
	CalleeSymbolID string `json:"callee_symbol_id"`
	Path           string `json:"path"`
	StartLine      int    `json:"start_line"`
	StartColumn    int    `json:"start_column"`
	EndLine        int    `json:"end_line"`
	EndColumn      int    `json:"end_column"`
}

type PythonSelectionProvenance struct {
	Provider         string   `json:"provider"`
	Version          string   `json:"version"`
	CollectorVersion string   `json:"collector_version"`
	Operations       []string `json:"operations"`
}

type PythonSourceSelectionTrace struct {
	Version             string                     `json:"version"`
	Repository          GoSelectionRepository      `json:"repository"`
	Question            string                     `json:"question"`
	Limits              PythonSelectionLimits      `json:"limits"`
	QueryTerms          []PythonSelectionQuery     `json:"query_terms"`
	Candidates          []PythonSelectionCandidate `json:"candidates"`
	SelectedSymbols     []PythonSelectionSymbol    `json:"selected_symbols"`
	ExactCalls          []PythonSelectionCall      `json:"exact_calls"`
	Provenance          PythonSelectionProvenance  `json:"provenance"`
	Coverage            string                     `json:"coverage"`
	UnresolvedFrontiers []string                   `json:"unresolved_frontiers"`
	Warnings            []string                   `json:"warnings"`
}

type PythonSourceSelectionSlice struct {
	Path               string   `json:"path"`
	StartLine          int      `json:"start_line"`
	EndLine            int      `json:"end_line"`
	Text               string   `json:"text"`
	EnclosingSymbolID  string   `json:"enclosing_symbol_id"`
	SelectionReasonIDs []string `json:"selection_reason_ids"`
}

type PythonSourceSelectionPacket struct {
	Version      string                       `json:"version"`
	Repository   GoSelectionRepository        `json:"repository"`
	Question     string                       `json:"question"`
	Coverage     string                       `json:"coverage"`
	SourceSlices []PythonSourceSelectionSlice `json:"source_slices"`
}

type pythonSelectionAnalyzer interface {
	ResolveLocation(context.Context, analysis.LocationRequest) (analysis.LocationResolution, error)
	AnalyzeExactSymbol(context.Context, analysis.ExactSymbolRequest) (evidence.Graph, error)
}

type pythonSelectionCandidateState struct {
	hit      pyrightWorkspaceHit
	queryIDs []string
	score    int
}

type pythonSelectionNode struct {
	entity         evidence.Entity
	queryIDs       []string
	candidateScore int
	analyzed       bool
	anchor         bool
	mandatory      bool
	dynamic        bool
	degree         int
}

type pythonSelectionEdge struct {
	from     string
	to       string
	location evidence.Location
}

type pythonSelectionSourceVerifier struct {
	repoPath string
	reader   *reporead.Reader
	cache    map[string]error
}

type pythonSelectionInspection struct {
	ctx          context.Context
	analyzer     pythonSelectionAnalyzer
	repoPath     string
	verifier     *pythonSelectionSourceVerifier
	nodes        map[string]pythonSelectionNode
	edges        []pythonSelectionEdge
	analyzedKeys map[string]struct{}
	warnings     []string
	version      string
	analyses     int
}

// SelectPythonQuestionSources proves the smallest Python parity slice: a
// question reaches bounded workspace-symbol candidates, then exact Pyright
// declaration and call evidence. It accepts no curated path, symbol, packet,
// sidecar, or model response.
func SelectPythonQuestionSources(
	ctx context.Context,
	opts PythonSourceSelectionOptions,
) (PythonSourceSelectionTrace, PythonSourceSelectionPacket, error) {
	finder := newPyrightWorkspaceSymbolFinder(opts.PyrightBinary)
	analyzer := pyright.New(pyright.Options{
		Binary:         opts.PyrightBinary,
		MaxIncoming:    12,
		MaxOutgoing:    12,
		MaxReferences:  8,
		RequestTimeout: pythonSelectionRequestTimeout,
	})
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = analyzer.Close(closeCtx)
	}()
	return selectPythonQuestionSources(ctx, opts, finder, analyzer)
}

func selectPythonQuestionSources(
	ctx context.Context,
	opts PythonSourceSelectionOptions,
	finder pythonWorkspaceSymbolFinder,
	analyzer pythonSelectionAnalyzer,
) (PythonSourceSelectionTrace, PythonSourceSelectionPacket, error) {
	repoPath, repository, err := validatePythonSelectionInput(opts)
	if err != nil {
		return PythonSourceSelectionTrace{}, PythonSourceSelectionPacket{}, err
	}
	queries, contentTerms, err := pythonSelectionQueries(repository.Name, opts.Question)
	if err != nil {
		return PythonSourceSelectionTrace{}, PythonSourceSelectionPacket{}, err
	}
	workspaceQueries := make([]pyrightWorkspaceQuery, len(queries))
	for index, query := range queries {
		workspaceQueries[index] = pyrightWorkspaceQuery{ID: query.ID, Text: query.Text}
	}
	discovery, err := finder.Find(ctx, repoPath, workspaceQueries)
	if err != nil {
		return PythonSourceSelectionTrace{}, PythonSourceSelectionPacket{}, fmt.Errorf(
			"python source selection: workspace symbols: %w",
			err,
		)
	}
	candidates, candidateWarnings, err := buildPythonSelectionCandidates(
		discovery.Hits,
		queries,
		contentTerms,
	)
	if err != nil {
		return PythonSourceSelectionTrace{}, PythonSourceSelectionPacket{}, err
	}
	if len(candidates) == 0 {
		return PythonSourceSelectionTrace{}, PythonSourceSelectionPacket{}, fmt.Errorf(
			"python source selection: no callable repository symbol matched the question",
		)
	}
	nodes, edges, analyzerVersion, analysisWarnings, err := inspectPythonSelectionRoots(
		ctx,
		analyzer,
		repoPath,
		queries,
		contentTerms,
		candidates,
	)
	if err != nil {
		return PythonSourceSelectionTrace{}, PythonSourceSelectionPacket{}, err
	}
	if analyzerVersion == "" {
		return PythonSourceSelectionTrace{}, PythonSourceSelectionPacket{}, fmt.Errorf(
			"python source selection: exact Pyright session did not report a version",
		)
	}
	effectiveVersion := discovery.Version
	if effectiveVersion == "" || effectiveVersion == pyrightWorkspaceUnknownVersion {
		effectiveVersion = analyzerVersion
	} else if pythonSelectionVersionNumber(discovery.Version) !=
		pythonSelectionVersionNumber(analyzerVersion) {
		return PythonSourceSelectionTrace{}, PythonSourceSelectionPacket{}, fmt.Errorf(
			"python source selection: workspace and exact sessions disagree on Pyright version %q / %q",
			discovery.Version,
			analyzerVersion,
		)
	}
	selected, err := selectPythonSelectionNodes(nodes, edges, contentTerms)
	if err != nil {
		return PythonSourceSelectionTrace{}, PythonSourceSelectionPacket{}, err
	}
	selected, edges, rangeWarnings, err := enrichPythonSelectionRanges(
		ctx,
		analyzer,
		repoPath,
		selected,
		edges,
		contentTerms,
	)
	if err != nil {
		return PythonSourceSelectionTrace{}, PythonSourceSelectionPacket{}, err
	}
	packet, retained, retainedEdges, err := buildPythonSelectionPacket(
		repoPath,
		repository,
		opts.Question,
		selected,
		edges,
	)
	if err != nil {
		return PythonSourceSelectionTrace{}, PythonSourceSelectionPacket{}, err
	}
	if err := validateGoSelectionCheckout(repoPath, opts.ExpectedRevision); err != nil {
		return PythonSourceSelectionTrace{}, PythonSourceSelectionPacket{}, err
	}
	warnings := append([]string(nil), discovery.Warnings...)
	warnings = append(warnings, candidateWarnings...)
	warnings = append(warnings, analysisWarnings...)
	warnings = append(warnings, rangeWarnings...)
	warnings = append(warnings,
		"workspace symbol matches and exact call neighborhoods are bounded and non-exhaustive",
		"runtime-selected module, class, command-list, and callback targets remain unresolved unless Pyright reports an exact repository declaration",
	)
	warnings = boundedPythonSelectionWarnings(warnings)
	trace := buildPythonSelectionTrace(
		repository,
		opts.Question,
		queries,
		candidates,
		retained,
		retainedEdges,
		effectiveVersion,
		warnings,
	)
	return trace, packet, nil
}

func validatePythonSelectionInput(
	opts PythonSourceSelectionOptions,
) (string, GoSelectionRepository, error) {
	if opts.RepositoryPath == "" {
		return "", GoSelectionRepository{}, fmt.Errorf("python source selection: repository path is required")
	}
	if opts.Question == "" ||
		len(opts.Question) > pythonSelectionMaxQuestionBytes ||
		!utf8.ValidString(opts.Question) ||
		strings.ContainsRune(opts.Question, 0) {
		return "", GoSelectionRepository{}, fmt.Errorf(
			"python source selection: question must be valid UTF-8 within %d bytes",
			pythonSelectionMaxQuestionBytes,
		)
	}
	if len(opts.ExpectedRevision) != 40 || !lowerHex(opts.ExpectedRevision) {
		return "", GoSelectionRepository{}, fmt.Errorf(
			"python source selection: expected revision must be a lowercase 40-byte commit",
		)
	}
	absolute, err := filepath.Abs(opts.RepositoryPath)
	if err != nil {
		return "", GoSelectionRepository{}, fmt.Errorf("python source selection: resolve repository: %w", err)
	}
	repoPath, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", GoSelectionRepository{}, fmt.Errorf("python source selection: resolve repository symlinks: %w", err)
	}
	revision, err := runGit(repoPath, "rev-parse", "HEAD")
	if err != nil {
		return "", GoSelectionRepository{}, err
	}
	if revision != opts.ExpectedRevision {
		return "", GoSelectionRepository{}, fmt.Errorf(
			"python source selection: revision is %s, want %s",
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

func pythonSelectionQueries(
	repositoryName string,
	question string,
) ([]PythonSelectionQuery, []string, error) {
	repositoryTerms := make(map[string]struct{}, 4)
	for _, word := range strings.FieldsFunc(strings.ToLower(repositoryName), func(character rune) bool {
		return !unicode.IsLetter(character) && !unicode.IsDigit(character)
	}) {
		repositoryTerms[normalizePythonSelectionTerm(word)] = struct{}{}
	}
	content := make([]string, 0, pythonSelectionMaxQueryTerms)
	seen := make(map[string]struct{}, pythonSelectionMaxQueryTerms)
	for _, word := range strings.FieldsFunc(strings.ToLower(question), func(character rune) bool {
		return !unicode.IsLetter(character) && !unicode.IsDigit(character)
	}) {
		if len([]rune(word)) < 4 {
			continue
		}
		if _, stop := goSelectionStopWords[word]; stop {
			continue
		}
		if _, filler := pythonSelectionFillerWords[word]; filler {
			continue
		}
		term := normalizePythonSelectionTerm(word)
		if term == "" {
			continue
		}
		if _, repositoryTerm := repositoryTerms[term]; repositoryTerm {
			continue
		}
		if _, duplicate := seen[term]; duplicate {
			continue
		}
		seen[term] = struct{}{}
		content = append(content, term)
		if len(content) == pythonSelectionMaxQueryTerms {
			break
		}
	}
	if len(content) == 0 {
		return nil, nil, fmt.Errorf("python source selection: question has no usable content terms")
	}
	queries := make([]PythonSelectionQuery, len(content))
	for index, term := range content {
		queries[index] = PythonSelectionQuery{
			ID:   fmt.Sprintf("q%d", index+1),
			Text: term,
		}
	}
	return queries, append([]string(nil), content...), nil
}

func normalizePythonSelectionTerm(value string) string {
	normalized := normalizeGoSelectionTerm(value)
	if len(normalized) > 4 && strings.HasSuffix(normalized, "s") {
		normalized = strings.TrimSuffix(normalized, "s")
	}
	return normalized
}

func buildPythonSelectionCandidates(
	hits []pyrightWorkspaceHit,
	queries []PythonSelectionQuery,
	contentTerms []string,
) ([]pythonSelectionCandidateState, []string, error) {
	if len(hits) > pythonSelectionMaxHitUnion {
		return nil, nil, fmt.Errorf("python source selection: workspace hits exceed the processing budget")
	}
	queryIDs := make(map[string]struct{}, pythonSelectionMaxQueryTerms)
	for _, query := range queries {
		queryIDs[query.ID] = struct{}{}
	}
	byKey := make(map[string]*pythonSelectionCandidateState, pythonSelectionMaxHitUnion)
	for _, hit := range hits {
		if _, ok := queryIDs[hit.QueryID]; !ok {
			return nil, nil, fmt.Errorf("python source selection: workspace hit has unknown query")
		}
		if !pythonSelectionWorkspaceHitSafe(hit) {
			return nil, nil, fmt.Errorf("python source selection: workspace hit exceeds the scalar or path budget")
		}
		key := pythonSelectionWorkspaceKey(hit)
		if existing, ok := byKey[key]; ok {
			existing.queryIDs = appendUniqueString(existing.queryIDs, hit.QueryID)
			continue
		}
		copyHit := hit
		byKey[key] = &pythonSelectionCandidateState{
			hit:      copyHit,
			queryIDs: []string{hit.QueryID},
		}
	}
	result := make([]pythonSelectionCandidateState, 0, pythonSelectionMaxHitUnion)
	for _, candidate := range byKey {
		sort.Strings(candidate.queryIDs)
		candidate.score = scorePythonSelectionCandidate(*candidate, contentTerms)
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
		if goSelectionTestPath(left.hit.Path) != goSelectionTestPath(right.hit.Path) {
			return !goSelectionTestPath(left.hit.Path)
		}
		return pythonSelectionWorkspaceKey(left.hit) < pythonSelectionWorkspaceKey(right.hit)
	})
	warnings := []string(nil)
	if len(result) > pythonSelectionMaxCandidates {
		result = capPythonSelectionCandidates(result, queries)
		warnings = append(warnings, fmt.Sprintf(
			"workspace candidates were deterministically truncated to %d",
			pythonSelectionMaxCandidates,
		))
	}
	return result, warnings, nil
}

func capPythonSelectionCandidates(
	candidates []pythonSelectionCandidateState,
	queries []PythonSelectionQuery,
) []pythonSelectionCandidateState {
	selected := make([]pythonSelectionCandidateState, 0, pythonSelectionMaxCandidates)
	seen := make(map[string]struct{}, pythonSelectionMaxCandidates)
	appendCandidate := func(candidate pythonSelectionCandidateState) {
		key := pythonSelectionWorkspaceKey(candidate.hit)
		if _, duplicate := seen[key]; duplicate ||
			len(selected) == pythonSelectionMaxCandidates {
			return
		}
		seen[key] = struct{}{}
		selected = append(selected, candidate)
	}
	for _, query := range queries {
		count := 0
		for _, candidate := range candidates {
			if !containsString(candidate.queryIDs, query.ID) {
				continue
			}
			appendCandidate(candidate)
			count++
			if count == pythonSelectionMinCandidatesPerQuery {
				break
			}
		}
	}
	for _, candidate := range candidates {
		appendCandidate(candidate)
	}
	sort.Slice(selected, func(i, j int) bool {
		left, right := selected[i], selected[j]
		if left.score != right.score {
			return left.score > right.score
		}
		if len(left.queryIDs) != len(right.queryIDs) {
			return len(left.queryIDs) > len(right.queryIDs)
		}
		if goSelectionTestPath(left.hit.Path) != goSelectionTestPath(right.hit.Path) {
			return !goSelectionTestPath(left.hit.Path)
		}
		return pythonSelectionWorkspaceKey(left.hit) < pythonSelectionWorkspaceKey(right.hit)
	})
	return selected
}

func pythonSelectionWorkspaceHitSafe(hit pyrightWorkspaceHit) bool {
	return hit.QueryID != "" &&
		len(hit.QueryID) <= 16 &&
		hit.Name != "" &&
		len(hit.Name) <= goSelectionMaxAnalyzerText &&
		len(hit.Container) <= goSelectionMaxAnalyzerText &&
		utf8.ValidString(hit.Name) &&
		utf8.ValidString(hit.Container) &&
		!strings.ContainsRune(hit.Name, 0) &&
		!strings.ContainsRune(hit.Container, 0) &&
		(hit.Kind == evidence.EntityFunction || hit.Kind == evidence.EntityMethod) &&
		goSelectionCanonicalPath(hit.Path) &&
		strings.HasSuffix(strings.ToLower(hit.Path), ".py") &&
		hit.Line > 0 &&
		hit.Column > 0
}

func pythonSelectionWorkspaceKey(hit pyrightWorkspaceHit) string {
	return fmt.Sprintf(
		"%s:%s:%d:%d:%s",
		hit.Kind,
		hit.Path,
		hit.Line,
		hit.Column,
		hit.Name,
	)
}

func scorePythonSelectionCandidate(
	candidate pythonSelectionCandidateState,
	contentTerms []string,
) int {
	nameTerms := splitGoSelectionName(candidate.hit.Name)
	score := 40 + len(candidate.queryIDs)*10
	if !goSelectionTestPath(candidate.hit.Path) {
		score += 40
	}
	if candidate.hit.Kind == evidence.EntityFunction {
		score += 8
	}
	score += max(0, 10-len(nameTerms)*2)
	for _, term := range contentTerms {
		for _, nameTerm := range nameTerms {
			nameTerm = normalizePythonSelectionTerm(nameTerm)
			switch {
			case nameTerm == term:
				score += 30
			case strings.HasPrefix(nameTerm, term) || strings.HasPrefix(term, nameTerm):
				score += 10
			}
		}
	}
	return score
}

func inspectPythonSelectionRoots(
	ctx context.Context,
	analyzer pythonSelectionAnalyzer,
	repoPath string,
	queries []PythonSelectionQuery,
	contentTerms []string,
	candidates []pythonSelectionCandidateState,
) ([]pythonSelectionNode, []pythonSelectionEdge, string, []string, error) {
	verifier, err := newPythonSelectionSourceVerifier(repoPath)
	if err != nil {
		return nil, nil, "", nil, err
	}
	defer verifier.Close()
	inspection := pythonSelectionInspection{
		ctx:          ctx,
		analyzer:     analyzer,
		repoPath:     repoPath,
		verifier:     verifier,
		nodes:        make(map[string]pythonSelectionNode, pythonSelectionMaxCallEndpoints),
		edges:        make([]pythonSelectionEdge, 0, pythonSelectionMaxCallEndpoints),
		analyzedKeys: make(map[string]struct{}, pythonSelectionMaxExactAnalyses),
		warnings:     make([]string, 0, pythonSelectionMaxWarnings),
	}
	anchors := make(map[string]string, pythonSelectionMaxQueryTerms)
	attemptedQueries := make(map[string][]string, pythonSelectionMaxQueryTerms)
	for _, query := range queries {
		for _, candidate := range pythonSelectionRootCandidatesForQuery(query.ID, candidates) {
			attemptedQueries[query.ID] = appendUniqueString(
				attemptedQueries[query.ID],
				candidate.hit.Name,
			)
			if err := verifier.Verify(candidate.hit.Path); err != nil {
				inspection.warnings = appendUniqueString(
					inspection.warnings,
					fmt.Sprintf(
						"workspace candidate %s at %s:%d was not an ordinary tracked source file",
						candidate.hit.Name,
						candidate.hit.Path,
						candidate.hit.Line,
					),
				)
				continue
			}
			entity, err := resolvePythonSelectionCandidate(
				ctx,
				analyzer,
				repoPath,
				candidate,
				contentTerms,
			)
			if err != nil {
				inspection.warnings = appendUniqueString(
					inspection.warnings,
					fmt.Sprintf(
						"workspace candidate %s at %s:%d was not confirmed by exact document symbols",
						candidate.hit.Name,
						candidate.hit.Path,
						candidate.hit.Line,
					),
				)
				continue
			}
			key := goSelectionEntityKey(entity)
			node := inspection.nodes[key]
			node.entity = cloneGoSelectionEntity(entity)
			node.queryIDs = appendUniqueString(node.queryIDs, query.ID)
			node.candidateScore = max(node.candidateScore, candidate.score)
			node.analyzed = true
			inspection.nodes[key] = node

			edgeBacked, err := inspection.Inspect(entity, false)
			if err != nil {
				return nil, nil, "", nil, fmt.Errorf(
					"python source selection: exact symbol %s: %w",
					entity.Name,
					err,
				)
			}
			if edgeBacked {
				anchors[query.ID] = key
				node = inspection.nodes[key]
				node.anchor = true
				inspection.nodes[key] = node
				break
			}
		}
		if _, confirmed := anchors[query.ID]; !confirmed {
			return nil, nil, "", nil, fmt.Errorf(
				"python source selection: no edge-backed exact root confirmed for query %s from bounded candidates %q",
				query.ID,
				attemptedQueries[query.ID],
			)
		}
	}
	for bridgeAttempt := 0; bridgeAttempt < pythonSelectionMaxBridgeAnalyses &&
		!pythonSelectionAnchorsConnected(anchors, inspection.edges); bridgeAttempt++ {
		key, ok := pythonSelectionBridgeFrontier(
			inspection.nodes,
			inspection.edges,
			inspection.analyzedKeys,
			contentTerms,
		)
		if !ok {
			break
		}
		node := inspection.nodes[key]
		resolved, err := resolvePythonSelectionEntityRange(
			ctx,
			analyzer,
			verifier,
			repoPath,
			node.entity,
			contentTerms,
		)
		if err != nil {
			return nil, nil, "", nil, fmt.Errorf(
				"python source selection: bridge symbol %s at %s:%d-%d: %w",
				node.entity.Name,
				node.entity.Location.Path,
				node.entity.Location.Line,
				node.entity.Location.EndLine,
				err,
			)
		}
		key = inspection.remapNode(key, resolved, anchors)
		if _, err := inspection.Inspect(resolved, true); err != nil {
			return nil, nil, "", nil, fmt.Errorf(
				"python source selection: bridge symbol %s at %s:%d-%d: %w",
				resolved.Name,
				resolved.Location.Path,
				resolved.Location.Line,
				resolved.Location.EndLine,
				err,
			)
		}
	}
	if !pythonSelectionAnchorsConnected(anchors, inspection.edges) {
		return nil, nil, "", nil, fmt.Errorf(
			"python source selection: bounded exact-call frontier did not connect the query roots",
		)
	}
	for key, node := range inspection.nodes {
		node.degree = 0
		inspection.nodes[key] = node
	}
	for _, edge := range inspection.edges {
		from := inspection.nodes[edge.from]
		from.degree++
		inspection.nodes[edge.from] = from
		to := inspection.nodes[edge.to]
		to.degree++
		inspection.nodes[edge.to] = to
	}
	nodeValues := make([]pythonSelectionNode, 0, pythonSelectionMaxCallEndpoints)
	for _, node := range inspection.nodes {
		nodeValues = append(nodeValues, node)
	}
	sort.Slice(nodeValues, func(i, j int) bool {
		return goSelectionEntityKey(nodeValues[i].entity) < goSelectionEntityKey(nodeValues[j].entity)
	})
	sortPythonSelectionEdges(inspection.edges)
	return nodeValues, inspection.edges, inspection.version, inspection.warnings, nil
}

func (inspection *pythonSelectionInspection) Inspect(
	entity evidence.Entity,
	existingEndpointsOnly bool,
) (bool, error) {
	if inspection.analyses >= pythonSelectionMaxExactAnalyses {
		return false, fmt.Errorf("exact analysis budget exhausted")
	}
	if err := inspection.verifier.Verify(entity.Location.Path); err != nil {
		return false, fmt.Errorf("source preflight: %w", err)
	}
	rootKey := goSelectionEntityKey(entity)
	existing := make(map[string]struct{}, pythonSelectionMaxCallEndpoints)
	if existingEndpointsOnly {
		for key := range inspection.nodes {
			existing[key] = struct{}{}
		}
	}
	graph, err := inspection.analyzer.AnalyzeExactSymbol(
		inspection.ctx,
		analysis.ExactSymbolRequest{
			RepoPath: inspection.repoPath,
			Symbol:   entity,
		},
	)
	if err != nil {
		return false, err
	}
	inspection.analyses++
	inspection.analyzedKeys[rootKey] = struct{}{}
	if err := validateGoSelectionAnalyzerGraph(graph, pythonSelectionMaxGraphItems); err != nil {
		return false, err
	}
	graphVersion := pythonSelectionGraphVersion(graph)
	if graphVersion != "" {
		if inspection.version == "" {
			inspection.version = graphVersion
		} else if pythonSelectionVersionNumber(inspection.version) !=
			pythonSelectionVersionNumber(graphVersion) {
			return false, fmt.Errorf("exact analyses disagree on Pyright version")
		}
	}
	dynamic := false
	for _, warning := range graph.Warnings {
		inspection.warnings = appendUniqueString(inspection.warnings, warning)
		if strings.Contains(strings.ToLower(warning), "dynamic") &&
			strings.Contains(strings.ToLower(warning), "unresolved") {
			dynamic = true
		}
	}
	rootNode := inspection.nodes[rootKey]
	if rootNode.entity.Location == nil {
		rootNode.entity = cloneGoSelectionEntity(entity)
	}
	rootNode.dynamic = rootNode.dynamic || dynamic
	rootNode.analyzed = true
	inspection.nodes[rootKey] = rootNode

	graphEntities := make(map[string]evidence.Entity, pythonSelectionMaxGraphItems)
	for _, graphEntity := range graph.Entities {
		graphEntities[graphEntity.ID] = graphEntity
	}
	for _, relation := range graph.Relations {
		if relation.Kind != evidence.RelationCalls {
			continue
		}
		from, fromOK := graphEntities[relation.From]
		to, toOK := graphEntities[relation.To]
		if !fromOK || !toOK ||
			!pythonSelectionRepositoryCallable(from) ||
			!pythonSelectionRepositoryCallable(to) {
			continue
		}
		if to.Kind == evidence.EntityMethod {
			inspection.warnings = appendUniqueString(
				inspection.warnings,
				"Pyright method-target call edges were omitted because runtime dispatch may select a different implementation",
			)
			continue
		}
		callsite, ok := pythonSelectionCallsite(relation)
		if !ok || !goSelectionCanonicalPath(callsite.Path) {
			continue
		}
		if err := inspection.verifier.Verify(from.Location.Path); err != nil {
			return false, fmt.Errorf("caller source preflight: %w", err)
		}
		if err := inspection.verifier.Verify(to.Location.Path); err != nil {
			return false, fmt.Errorf("callee source preflight: %w", err)
		}
		if err := inspection.verifier.Verify(callsite.Path); err != nil {
			return false, fmt.Errorf("callsite source preflight: %w", err)
		}
		fromKey := goSelectionEntityKey(from)
		toKey := goSelectionEntityKey(to)
		if existingEndpointsOnly {
			if _, ok := existing[fromKey]; !ok {
				continue
			}
			if _, ok := existing[toKey]; !ok {
				continue
			}
		}
		if !pythonSelectionEndpointsFit(inspection.nodes, fromKey, toKey) {
			continue
		}
		inspection.mergeEntity(fromKey, from)
		inspection.mergeEntity(toKey, to)
		edge := pythonSelectionEdge{
			from:     fromKey,
			to:       toKey,
			location: callsite,
		}
		if len(inspection.edges) == pythonSelectionMaxCallEndpoints &&
			!hasPythonSelectionEdge(inspection.edges, edge) {
			continue
		}
		inspection.edges = appendUniquePythonSelectionEdge(inspection.edges, edge)
	}
	for _, edge := range inspection.edges {
		if edge.from == rootKey || edge.to == rootKey {
			return true, nil
		}
	}
	return false, nil
}

func (inspection *pythonSelectionInspection) mergeEntity(
	key string,
	entity evidence.Entity,
) {
	node, known := inspection.nodes[key]
	if !known {
		node.entity = cloneGoSelectionEntity(entity)
		inspection.nodes[key] = node
		return
	}
	if entity.Location != nil && node.entity.Location != nil &&
		entity.Location.EndLine-entity.Location.Line >
			node.entity.Location.EndLine-node.entity.Location.Line {
		node.entity = cloneGoSelectionEntity(entity)
	}
	inspection.nodes[key] = node
}

func (inspection *pythonSelectionInspection) remapNode(
	oldKey string,
	entity evidence.Entity,
	anchors map[string]string,
) string {
	newKey := goSelectionEntityKey(entity)
	node := inspection.nodes[oldKey]
	delete(inspection.nodes, oldKey)
	if existing, ok := inspection.nodes[newKey]; ok {
		existing.queryIDs = appendUniqueStrings(existing.queryIDs, node.queryIDs...)
		existing.candidateScore = max(existing.candidateScore, node.candidateScore)
		existing.dynamic = existing.dynamic || node.dynamic
		existing.anchor = existing.anchor || node.anchor
		node = existing
	}
	node.entity = cloneGoSelectionEntity(entity)
	node.analyzed = true
	inspection.nodes[newKey] = node
	for index := range inspection.edges {
		if inspection.edges[index].from == oldKey {
			inspection.edges[index].from = newKey
		}
		if inspection.edges[index].to == oldKey {
			inspection.edges[index].to = newKey
		}
	}
	deduplicated := make([]pythonSelectionEdge, 0, pythonSelectionMaxCallEndpoints)
	for _, edge := range inspection.edges {
		if edge.from != edge.to {
			deduplicated = appendUniquePythonSelectionEdge(deduplicated, edge)
		}
	}
	inspection.edges = deduplicated
	for queryID, key := range anchors {
		if key == oldKey {
			anchors[queryID] = newKey
		}
	}
	if _, ok := inspection.analyzedKeys[oldKey]; ok {
		delete(inspection.analyzedKeys, oldKey)
		inspection.analyzedKeys[newKey] = struct{}{}
	}
	return newKey
}

func pythonSelectionEndpointsFit(
	nodes map[string]pythonSelectionNode,
	fromKey string,
	toKey string,
) bool {
	if len(nodes) > pythonSelectionMaxCallEndpoints {
		return false
	}
	missing := 0
	if _, known := nodes[fromKey]; !known {
		missing++
	}
	if toKey != fromKey {
		if _, known := nodes[toKey]; !known {
			missing++
		}
	}
	return missing <= pythonSelectionMaxCallEndpoints-len(nodes)
}

func pythonSelectionAnchorsConnected(
	anchors map[string]string,
	edges []pythonSelectionEdge,
) bool {
	if len(anchors) == 0 {
		return false
	}
	adjacent := make(map[string][]string, pythonSelectionMaxCallEndpoints)
	for _, edge := range edges {
		adjacent[edge.from] = append(adjacent[edge.from], edge.to)
		adjacent[edge.to] = append(adjacent[edge.to], edge.from)
	}
	start := ""
	for _, key := range anchors {
		start = key
		break
	}
	visited := map[string]struct{}{start: {}}
	frontier := []string{start}
	for len(frontier) > 0 {
		current := frontier[0]
		frontier = frontier[1:]
		for _, next := range adjacent[current] {
			if _, seen := visited[next]; seen {
				continue
			}
			visited[next] = struct{}{}
			frontier = append(frontier, next)
		}
	}
	for _, key := range anchors {
		if _, ok := visited[key]; !ok {
			return false
		}
	}
	return true
}

func pythonSelectionBridgeFrontier(
	nodes map[string]pythonSelectionNode,
	edges []pythonSelectionEdge,
	analyzed map[string]struct{},
	contentTerms []string,
) (string, bool) {
	degree := make(map[string]int, pythonSelectionMaxCallEndpoints)
	for _, edge := range edges {
		degree[edge.from]++
		degree[edge.to]++
	}
	keys := make([]string, 0, pythonSelectionMaxCallEndpoints)
	for key, node := range nodes {
		if _, alreadyAnalyzed := analyzed[key]; alreadyAnalyzed ||
			!pythonSelectionRepositoryCallable(node.entity) {
			continue
		}
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		left, right := nodes[keys[i]], nodes[keys[j]]
		if degree[keys[i]] != degree[keys[j]] {
			return degree[keys[i]] > degree[keys[j]]
		}
		leftOverlap := goSelectionNameOverlap(left.entity.Name, contentTerms)
		rightOverlap := goSelectionNameOverlap(right.entity.Name, contentTerms)
		if leftOverlap != rightOverlap {
			return leftOverlap > rightOverlap
		}
		return keys[i] < keys[j]
	})
	if len(keys) == 0 {
		return "", false
	}
	return keys[0], true
}

func newPythonSelectionSourceVerifier(
	repoPath string,
) (*pythonSelectionSourceVerifier, error) {
	reader, err := reporead.New(repoPath)
	if err != nil {
		return nil, err
	}
	return &pythonSelectionSourceVerifier{
		repoPath: repoPath,
		reader:   reader,
		cache:    make(map[string]error, pythonSelectionMaxCallEndpoints),
	}, nil
}

func (verifier *pythonSelectionSourceVerifier) Verify(sourcePath string) error {
	if result, ok := verifier.cache[sourcePath]; ok {
		return result
	}
	if len(verifier.cache) == pythonSelectionMaxCallEndpoints {
		return fmt.Errorf("source verification budget exhausted")
	}
	err := verifyGoSelectionTracked(verifier.repoPath, sourcePath)
	if err == nil {
		_, err = verifier.reader.ReadFileNoSymlinks(sourcePath, 0)
	}
	verifier.cache[sourcePath] = err
	return err
}

func (verifier *pythonSelectionSourceVerifier) Close() {
	_ = verifier.reader.Close()
}

func pythonSelectionRootCandidatesForQuery(
	queryID string,
	candidates []pythonSelectionCandidateState,
) []pythonSelectionCandidateState {
	result := make([]pythonSelectionCandidateState, 0, pythonSelectionMaxRootsPerQuery)
	for _, includeTests := range []bool{false, true} {
		for _, candidate := range candidates {
			if !containsString(candidate.queryIDs, queryID) ||
				goSelectionTestPath(candidate.hit.Path) != includeTests {
				continue
			}
			result = append(result, candidate)
			if len(result) == pythonSelectionMaxRootsPerQuery {
				return result
			}
		}
	}
	return result
}

func resolvePythonSelectionCandidate(
	ctx context.Context,
	analyzer pythonSelectionAnalyzer,
	repoPath string,
	candidate pythonSelectionCandidateState,
	contentTerms []string,
) (evidence.Entity, error) {
	result, err := analyzer.ResolveLocation(ctx, analysis.LocationRequest{
		RepoPath: repoPath,
		Location: evidence.Location{
			Path:   candidate.hit.Path,
			Line:   candidate.hit.Line,
			Column: candidate.hit.Column,
		},
		MaxCandidates: pythonSelectionMaxResolveResults,
		RankTerms:     append([]string(nil), contentTerms...),
	})
	if err != nil {
		return evidence.Entity{}, fmt.Errorf(
			"python source selection: resolve %s: %w",
			candidate.hit.Name,
			err,
		)
	}
	if err := validateGoSelectionResolution(result); err != nil {
		return evidence.Entity{}, fmt.Errorf(
			"python source selection: resolve %s: %w",
			candidate.hit.Name,
			err,
		)
	}
	for _, resolved := range result.Candidates {
		entity := resolved.Entity
		if !resolved.Investigable ||
			entity.Name != candidate.hit.Name ||
			entity.Kind != candidate.hit.Kind ||
			!pythonSelectionRepositoryCallable(entity) ||
			entity.Location.Path != candidate.hit.Path ||
			entity.Location.Line != candidate.hit.Line ||
			candidate.hit.Column < entity.Location.Column {
			continue
		}
		return cloneGoSelectionEntity(entity), nil
	}
	return evidence.Entity{}, fmt.Errorf(
		"python source selection: Pyright did not confirm workspace symbol %s at %s:%d:%d",
		candidate.hit.Name,
		candidate.hit.Path,
		candidate.hit.Line,
		candidate.hit.Column,
	)
}

func pythonSelectionRepositoryCallable(entity evidence.Entity) bool {
	return goSelectionRepositoryEntity(entity) &&
		entity.Language == "python" &&
		strings.HasSuffix(strings.ToLower(entity.Location.Path), ".py") &&
		entity.Location.EndLine >= entity.Location.Line &&
		entity.Location.EndLine-entity.Location.Line+1 <= pythonSelectionMaxSliceLines
}

func pythonSelectionGraphVersion(graph evidence.Graph) string {
	for _, relation := range graph.Relations {
		for _, provenance := range relation.Provenance {
			if provenance.Provider == "pyright" && provenance.Version != "" {
				return provenance.Version
			}
		}
	}
	return ""
}

func pythonSelectionVersionNumber(value string) string {
	fields := strings.Fields(value)
	for _, field := range fields {
		if field != "" && field[0] >= '0' && field[0] <= '9' {
			return strings.TrimPrefix(field, "v")
		}
	}
	return strings.TrimPrefix(strings.TrimSpace(value), "v")
}

func pythonSelectionCallsite(
	relation evidence.Relation,
) (evidence.Location, bool) {
	for _, provenance := range relation.Provenance {
		if provenance.Provider == "pyright" &&
			strings.HasPrefix(provenance.Operation, "callHierarchy/") &&
			provenance.Location != nil {
			return *provenance.Location, true
		}
	}
	return evidence.Location{}, false
}

func selectPythonSelectionNodes(
	nodes []pythonSelectionNode,
	edges []pythonSelectionEdge,
	contentTerms []string,
) ([]pythonSelectionNode, error) {
	byKey := make(map[string]pythonSelectionNode, pythonSelectionMaxCallEndpoints)
	anchors := make([]string, 0, pythonSelectionMaxQueryTerms)
	selected := make(map[string]struct{}, pythonSelectionMaxSlices)
	for _, node := range nodes {
		key := goSelectionEntityKey(node.entity)
		if len(node.queryIDs) > 0 {
			selected[key] = struct{}{}
		}
		if node.anchor {
			node.mandatory = true
			anchors = append(anchors, key)
		}
		byKey[key] = node
	}
	sort.Strings(anchors)
	for left := 0; left < len(anchors); left++ {
		for right := left + 1; right < len(anchors); right++ {
			for _, key := range pythonSelectionShortestPath(
				anchors[left],
				anchors[right],
				edges,
			) {
				node, ok := byKey[key]
				if !ok {
					return nil, fmt.Errorf(
						"python source selection: mandatory connected spine references an unknown node",
					)
				}
				node.mandatory = true
				byKey[key] = node
				selected[key] = struct{}{}
			}
		}
	}
	degree := make(map[string]int, pythonSelectionMaxCallEndpoints)
	adjacent := make(map[string][]string, pythonSelectionMaxCallEndpoints)
	for _, edge := range edges {
		degree[edge.from]++
		degree[edge.to]++
		adjacent[edge.from] = appendUniqueString(adjacent[edge.from], edge.to)
		adjacent[edge.to] = appendUniqueString(adjacent[edge.to], edge.from)
	}
	for _, anchor := range anchors {
		neighbors := append([]string(nil), adjacent[anchor]...)
		sort.Slice(neighbors, func(i, j int) bool {
			left, right := byKey[neighbors[i]], byKey[neighbors[j]]
			leftOverlap := goSelectionNameOverlap(left.entity.Name, contentTerms)
			rightOverlap := goSelectionNameOverlap(right.entity.Name, contentTerms)
			if leftOverlap != rightOverlap {
				return leftOverlap > rightOverlap
			}
			if degree[neighbors[i]] != degree[neighbors[j]] {
				return degree[neighbors[i]] > degree[neighbors[j]]
			}
			return neighbors[i] < neighbors[j]
		})
		for _, neighbor := range neighbors {
			if _, alreadySelected := selected[neighbor]; alreadySelected {
				continue
			}
			selected[neighbor] = struct{}{}
			break
		}
	}
	result := make([]pythonSelectionNode, 0, pythonSelectionMaxSlices)
	for key := range selected {
		if node, ok := byKey[key]; ok {
			result = append(result, node)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		left, right := result[i], result[j]
		leftRank := pythonSelectionNodeRank(left, contentTerms)
		rightRank := pythonSelectionNodeRank(right, contentTerms)
		if leftRank != rightRank {
			return leftRank > rightRank
		}
		return goSelectionEntityKey(left.entity) < goSelectionEntityKey(right.entity)
	})
	required := append([]pythonSelectionNode(nil), result...)
	result, err := retainPythonSelectionMandatoryNodes(result)
	if err != nil {
		return nil, err
	}
	if err := validatePythonSelectionMandatorySpine(
		required,
		result,
		projectPythonSelectionEdges(edges, result),
		"node selection",
	); err != nil {
		return nil, err
	}
	return result, nil
}

func pythonSelectionShortestPath(
	start string,
	target string,
	edges []pythonSelectionEdge,
) []string {
	adjacent := make(map[string][]string, pythonSelectionMaxCallEndpoints)
	for _, edge := range edges {
		adjacent[edge.from] = appendUniqueString(adjacent[edge.from], edge.to)
		adjacent[edge.to] = appendUniqueString(adjacent[edge.to], edge.from)
	}
	for key := range adjacent {
		sort.Strings(adjacent[key])
	}
	parents := make(map[string]string, pythonSelectionMaxCallEndpoints)
	visited := map[string]struct{}{start: {}}
	frontier := []string{start}
	for len(frontier) > 0 {
		current := frontier[0]
		frontier = frontier[1:]
		if current == target {
			break
		}
		for _, next := range adjacent[current] {
			if _, seen := visited[next]; seen {
				continue
			}
			visited[next] = struct{}{}
			parents[next] = current
			frontier = append(frontier, next)
		}
	}
	if _, ok := visited[target]; !ok {
		return nil
	}
	path := []string{target}
	for current := target; current != start; {
		current = parents[current]
		path = append(path, current)
	}
	slices.Reverse(path)
	return path
}

func retainPythonSelectionMandatoryNodes(
	nodes []pythonSelectionNode,
) ([]pythonSelectionNode, error) {
	if len(nodes) <= pythonSelectionMaxSlices {
		return nodes, nil
	}
	mandatory := pythonSelectionMandatoryCount(nodes)
	if mandatory > pythonSelectionMaxSlices {
		return nil, fmt.Errorf(
			"python source selection: mandatory connected spine has %d nodes; slice limit is %d",
			mandatory,
			pythonSelectionMaxSlices,
		)
	}
	keep := make(map[string]struct{}, pythonSelectionMaxSlices)
	for _, node := range nodes {
		if node.mandatory {
			keep[goSelectionEntityKey(node.entity)] = struct{}{}
		}
	}
	for _, node := range nodes {
		if len(keep) == pythonSelectionMaxSlices {
			break
		}
		keep[goSelectionEntityKey(node.entity)] = struct{}{}
	}
	retained := make([]pythonSelectionNode, 0, pythonSelectionMaxSlices)
	for _, node := range nodes {
		if _, ok := keep[goSelectionEntityKey(node.entity)]; ok {
			retained = append(retained, node)
		}
	}
	return retained, nil
}

func validatePythonSelectionMandatorySpine(
	expected []pythonSelectionNode,
	retained []pythonSelectionNode,
	edges []pythonSelectionEdge,
	stage string,
) error {
	expectedMandatory := pythonSelectionMandatoryCount(expected)
	if expectedMandatory == 0 {
		return fmt.Errorf(
			"python source selection: %s has no mandatory connected spine",
			stage,
		)
	}
	if pythonSelectionMandatoryCount(retained) != expectedMandatory {
		return fmt.Errorf(
			"python source selection: mandatory connected spine was lost during %s",
			stage,
		)
	}
	expectedQueries := make(map[string]struct{}, pythonSelectionMaxQueryTerms)
	for _, node := range expected {
		if !node.anchor {
			continue
		}
		for _, queryID := range node.queryIDs {
			expectedQueries[queryID] = struct{}{}
		}
	}
	anchors := make(map[string]string, pythonSelectionMaxQueryTerms)
	for _, node := range retained {
		if !node.anchor {
			continue
		}
		key := goSelectionEntityKey(node.entity)
		for _, queryID := range node.queryIDs {
			anchors[queryID] = key
		}
	}
	for queryID := range expectedQueries {
		if _, ok := anchors[queryID]; !ok {
			return fmt.Errorf(
				"python source selection: query anchor %s was lost during %s",
				queryID,
				stage,
			)
		}
	}
	if len(anchors) != len(expectedQueries) ||
		!pythonSelectionAnchorsConnected(anchors, edges) {
		return fmt.Errorf(
			"python source selection: mandatory connected spine was disconnected during %s",
			stage,
		)
	}
	return nil
}

func pythonSelectionMandatoryCount(nodes []pythonSelectionNode) int {
	count := 0
	for _, node := range nodes {
		if node.mandatory {
			count++
		}
	}
	return count
}

func enrichPythonSelectionRanges(
	ctx context.Context,
	analyzer pythonSelectionAnalyzer,
	repoPath string,
	nodes []pythonSelectionNode,
	edges []pythonSelectionEdge,
	contentTerms []string,
) ([]pythonSelectionNode, []pythonSelectionEdge, []string, error) {
	verifier, err := newPythonSelectionSourceVerifier(repoPath)
	if err != nil {
		return nil, nil, nil, err
	}
	defer verifier.Close()
	remapped := make(map[string]string, pythonSelectionMaxRangeCandidates)
	byKey := make(map[string]pythonSelectionNode, pythonSelectionMaxRangeCandidates)
	warnings := make([]string, 0, pythonSelectionMaxRangeCandidates)
	for _, node := range nodes {
		oldKey := goSelectionEntityKey(node.entity)
		resolved := cloneGoSelectionEntity(node.entity)
		var err error
		if !node.analyzed {
			resolved, err = resolvePythonSelectionEntityRange(
				ctx,
				analyzer,
				verifier,
				repoPath,
				node.entity,
				contentTerms,
			)
		}
		if err != nil {
			if node.mandatory {
				return nil, nil, nil, fmt.Errorf(
					"python source selection: mandatory connected spine symbol %s did not resolve to a full declaration range: %w",
					node.entity.Name,
					err,
				)
			}
			warnings = appendUniqueString(
				warnings,
				fmt.Sprintf(
					"selected structural neighbor %s at %s:%d was omitted because its full declaration range was not confirmed",
					node.entity.Name,
					node.entity.Location.Path,
					node.entity.Location.Line,
				),
			)
			continue
		}
		node.entity = resolved
		newKey := goSelectionEntityKey(resolved)
		remapped[oldKey] = newKey
		if existing, duplicate := byKey[newKey]; duplicate {
			existing.queryIDs = appendUniqueStrings(existing.queryIDs, node.queryIDs...)
			existing.candidateScore = max(existing.candidateScore, node.candidateScore)
			existing.analyzed = existing.analyzed || node.analyzed
			existing.anchor = existing.anchor || node.anchor
			existing.mandatory = existing.mandatory || node.mandatory
			existing.dynamic = existing.dynamic || node.dynamic
			byKey[newKey] = existing
			continue
		}
		node.degree = 0
		byKey[newKey] = node
	}
	enrichedEdges := make([]pythonSelectionEdge, 0, pythonSelectionMaxCallEndpoints)
	for _, edge := range edges {
		from, fromOK := remapped[edge.from]
		to, toOK := remapped[edge.to]
		if !fromOK || !toOK || from == to {
			continue
		}
		edge.from = from
		edge.to = to
		enrichedEdges = appendUniquePythonSelectionEdge(enrichedEdges, edge)
	}
	sortPythonSelectionEdges(enrichedEdges)
	for _, edge := range enrichedEdges {
		from := byKey[edge.from]
		from.degree++
		byKey[edge.from] = from
		to := byKey[edge.to]
		to.degree++
		byKey[edge.to] = to
	}
	enriched := make([]pythonSelectionNode, 0, pythonSelectionMaxRangeCandidates)
	for _, node := range byKey {
		enriched = append(enriched, node)
	}
	sort.Slice(enriched, func(i, j int) bool {
		left, right := enriched[i], enriched[j]
		leftRank := pythonSelectionNodeRank(left, contentTerms)
		rightRank := pythonSelectionNodeRank(right, contentTerms)
		if leftRank != rightRank {
			return leftRank > rightRank
		}
		return goSelectionEntityKey(left.entity) < goSelectionEntityKey(right.entity)
	})
	enriched, err = retainPythonSelectionMandatoryNodes(enriched)
	if err != nil {
		return nil, nil, nil, err
	}
	enrichedEdges = projectPythonSelectionEdges(enrichedEdges, enriched)
	if err := validatePythonSelectionMandatorySpine(
		nodes,
		enriched,
		enrichedEdges,
		"range enrichment",
	); err != nil {
		return nil, nil, nil, err
	}
	return enriched, enrichedEdges, warnings, nil
}

func resolvePythonSelectionEntityRange(
	ctx context.Context,
	analyzer pythonSelectionAnalyzer,
	verifier *pythonSelectionSourceVerifier,
	repoPath string,
	entity evidence.Entity,
	contentTerms []string,
) (evidence.Entity, error) {
	if !pythonSelectionRepositoryCallable(entity) {
		return evidence.Entity{}, fmt.Errorf("invalid repository callable")
	}
	if err := verifier.Verify(entity.Location.Path); err != nil {
		return evidence.Entity{}, fmt.Errorf("source preflight: %w", err)
	}
	lookup := evidence.Location{
		Path: entity.Location.Path,
		Line: entity.Location.Line,
	}
	result, err := analyzer.ResolveLocation(ctx, analysis.LocationRequest{
		RepoPath:      repoPath,
		Location:      lookup,
		MaxCandidates: pythonSelectionMaxResolveResults,
		RankTerms:     append([]string(nil), contentTerms...),
	})
	if err != nil {
		return evidence.Entity{}, err
	}
	if err := validateGoSelectionResolution(result); err != nil {
		return evidence.Entity{}, err
	}
	for _, candidate := range result.Candidates {
		resolved := candidate.Entity
		if !candidate.Investigable ||
			resolved.Name != entity.Name ||
			resolved.Kind != entity.Kind ||
			!pythonSelectionRepositoryCallable(resolved) ||
			resolved.Location.Path != entity.Location.Path ||
			resolved.Location.Line != entity.Location.Line {
			continue
		}
		return cloneGoSelectionEntity(resolved), nil
	}
	return evidence.Entity{}, fmt.Errorf(
		"declaration range was not confirmed from %d candidate(s)",
		len(result.Candidates),
	)
}

func pythonSelectionNodeRank(node pythonSelectionNode, contentTerms []string) int {
	score := node.degree*100 + goSelectionNameOverlap(node.entity.Name, contentTerms)*30
	if len(node.queryIDs) > 0 {
		score += 40 + node.candidateScore/10
	}
	if node.dynamic {
		score += 30
	}
	if !goSelectionTestPath(node.entity.Location.Path) {
		score += 20
	}
	return score
}

func buildPythonSelectionPacket(
	repoPath string,
	repository GoSelectionRepository,
	question string,
	selected []pythonSelectionNode,
	edges []pythonSelectionEdge,
) (PythonSourceSelectionPacket, []pythonSelectionNode, []pythonSelectionEdge, error) {
	reader, err := reporead.New(repoPath)
	if err != nil {
		return PythonSourceSelectionPacket{}, nil, nil, err
	}
	defer reader.Close()
	prepared := make(map[string]PythonSourceSelectionSlice, pythonSelectionMaxSlices)
	totalBytes := 0
	for _, mandatory := range []bool{true, false} {
		for _, node := range selected {
			if node.mandatory != mandatory {
				continue
			}
			sourceSlice, err := readPythonSelectionSourceSlice(
				repoPath,
				reader,
				node,
			)
			if err != nil {
				if mandatory {
					return PythonSourceSelectionPacket{}, nil, nil, fmt.Errorf(
						"python source selection: mandatory connected spine source %s could not be retained: %w",
						node.entity.Name,
						err,
					)
				}
				continue
			}
			if totalBytes+len(sourceSlice.Text) > pythonSelectionMaxSourceBytes {
				if mandatory {
					return PythonSourceSelectionPacket{}, nil, nil, fmt.Errorf(
						"python source selection: mandatory connected spine exceeds the %d-byte source budget",
						pythonSelectionMaxSourceBytes,
					)
				}
				continue
			}
			prepared[goSelectionEntityKey(node.entity)] = sourceSlice
			totalBytes += len(sourceSlice.Text)
		}
	}
	retained := make([]pythonSelectionNode, 0, pythonSelectionMaxSlices)
	slices := make([]PythonSourceSelectionSlice, 0, pythonSelectionMaxSlices)
	for _, node := range selected {
		sourceSlice, ok := prepared[goSelectionEntityKey(node.entity)]
		if !ok {
			continue
		}
		retained = append(retained, node)
		slices = append(slices, sourceSlice)
	}
	if len(retained) == 0 {
		return PythonSourceSelectionPacket{}, nil, nil, fmt.Errorf(
			"python source selection: no exact source slice fits the packet budget",
		)
	}
	retainedEdges := projectPythonSelectionEdges(edges, retained)
	if err := validatePythonSelectionMandatorySpine(
		selected,
		retained,
		retainedEdges,
		"source packet construction",
	); err != nil {
		return PythonSourceSelectionPacket{}, nil, nil, err
	}
	edgeIDs := pythonSelectionEdgeIDs(retainedEdges)
	for index, node := range retained {
		slices[index].SelectionReasonIDs = pythonSelectionNodeReasons(node, retainedEdges, edgeIDs)
	}
	return PythonSourceSelectionPacket{
		Version:      pythonSelectionVersion,
		Repository:   repository,
		Question:     question,
		Coverage:     "bounded_pyright_static_call_neighborhood_non_exhaustive",
		SourceSlices: slices,
	}, retained, retainedEdges, nil
}

func readPythonSelectionSourceSlice(
	repoPath string,
	reader *reporead.Reader,
	node pythonSelectionNode,
) (PythonSourceSelectionSlice, error) {
	location := node.entity.Location
	if location == nil ||
		!pythonSelectionRepositoryCallable(node.entity) ||
		location.EndLine-location.Line+1 > pythonSelectionMaxSliceLines {
		return PythonSourceSelectionSlice{}, fmt.Errorf("declaration range is invalid")
	}
	if err := verifyGoSelectionTracked(repoPath, location.Path); err != nil {
		return PythonSourceSelectionSlice{}, err
	}
	content, err := reader.ReadFileNoSymlinks(
		location.Path,
		pythonSelectionMaxSourceFile,
	)
	if err != nil {
		return PythonSourceSelectionSlice{}, err
	}
	if content.Truncated {
		return PythonSourceSelectionSlice{}, fmt.Errorf(
			"source file exceeds the %d-byte read budget",
			pythonSelectionMaxSourceFile,
		)
	}
	text, err := goSelectionLineRange(
		content.Bytes,
		location.Line,
		location.EndLine,
	)
	if err != nil {
		return PythonSourceSelectionSlice{}, err
	}
	return PythonSourceSelectionSlice{
		Path:              location.Path,
		StartLine:         location.Line,
		EndLine:           location.EndLine,
		Text:              text,
		EnclosingSymbolID: goSelectionEntityKey(node.entity),
	}, nil
}

func buildPythonSelectionTrace(
	repository GoSelectionRepository,
	question string,
	queries []PythonSelectionQuery,
	candidates []pythonSelectionCandidateState,
	selected []pythonSelectionNode,
	edges []pythonSelectionEdge,
	version string,
	warnings []string,
) PythonSourceSelectionTrace {
	traceCandidates := make([]PythonSelectionCandidate, len(candidates))
	for index, candidate := range candidates {
		traceCandidates[index] = PythonSelectionCandidate{
			ID:           pythonSelectionWorkspaceKey(candidate.hit),
			Name:         candidate.hit.Name,
			Container:    candidate.hit.Container,
			Kind:         string(candidate.hit.Kind),
			Path:         candidate.hit.Path,
			Line:         candidate.hit.Line,
			Column:       candidate.hit.Column,
			QueryTermIDs: append([]string(nil), candidate.queryIDs...),
			Score:        candidate.score,
		}
	}
	edgeIDs := pythonSelectionEdgeIDs(edges)
	traceSymbols := make([]PythonSelectionSymbol, len(selected))
	for index, node := range selected {
		location := node.entity.Location
		traceSymbols[index] = PythonSelectionSymbol{
			ID:                 goSelectionEntityKey(node.entity),
			Name:               node.entity.Name,
			Kind:               string(node.entity.Kind),
			Path:               location.Path,
			StartLine:          location.Line,
			StartColumn:        location.Column,
			EndLine:            location.EndLine,
			EndColumn:          location.EndColumn,
			SelectionReasonIDs: pythonSelectionNodeReasons(node, edges, edgeIDs),
		}
	}
	traceCalls := make([]PythonSelectionCall, len(edges))
	for index, edge := range edges {
		traceCalls[index] = PythonSelectionCall{
			ID:             edgeIDs[pythonSelectionEdgeKey(edge)],
			CallerSymbolID: edge.from,
			CalleeSymbolID: edge.to,
			Path:           edge.location.Path,
			StartLine:      edge.location.Line,
			StartColumn:    edge.location.Column,
			EndLine:        edge.location.EndLine,
			EndColumn:      edge.location.EndColumn,
		}
	}
	return PythonSourceSelectionTrace{
		Version:    pythonSelectionVersion,
		Repository: repository,
		Question:   question,
		Limits: PythonSelectionLimits{
			QueryTerms:       pythonSelectionMaxQueryTerms,
			HitsPerQuery:     pythonSelectionMaxHitsPerQuery,
			Candidates:       pythonSelectionMaxCandidates,
			RootsPerQuery:    pythonSelectionMaxRootsPerQuery,
			ExactAnalyses:    pythonSelectionMaxExactAnalyses,
			CallEndpoints:    pythonSelectionMaxCallEndpoints,
			SourceSlices:     pythonSelectionMaxSlices,
			SourceTextBytes:  pythonSelectionMaxSourceBytes,
			SourceSliceLines: pythonSelectionMaxSliceLines,
		},
		QueryTerms:      append([]PythonSelectionQuery(nil), queries...),
		Candidates:      traceCandidates,
		SelectedSymbols: traceSymbols,
		ExactCalls:      traceCalls,
		Provenance: PythonSelectionProvenance{
			Provider:         "pyright",
			Version:          version,
			CollectorVersion: pythonSelectionVersion,
			Operations: []string{
				"workspace/symbol",
				"textDocument/documentSymbol",
				"callHierarchy/incomingCalls",
				"callHierarchy/outgoingCalls",
			},
		},
		Coverage: "bounded_pyright_static_call_neighborhood_non_exhaustive",
		UnresolvedFrontiers: []string{
			"runtime-selected imports and concrete classes",
			"runtime-selected method implementations and returned command objects",
			"runtime-selected callback targets",
		},
		Warnings: warnings,
	}
}

func projectPythonSelectionEdges(
	edges []pythonSelectionEdge,
	nodes []pythonSelectionNode,
) []pythonSelectionEdge {
	known := make(map[string]struct{}, pythonSelectionMaxSlices)
	for _, node := range nodes {
		known[goSelectionEntityKey(node.entity)] = struct{}{}
	}
	result := make([]pythonSelectionEdge, 0, pythonSelectionMaxCallEndpoints)
	for _, edge := range edges {
		if _, ok := known[edge.from]; !ok {
			continue
		}
		if _, ok := known[edge.to]; !ok {
			continue
		}
		result = append(result, edge)
	}
	sortPythonSelectionEdges(result)
	return result
}

func pythonSelectionNodeReasons(
	node pythonSelectionNode,
	edges []pythonSelectionEdge,
	edgeIDs map[string]string,
) []string {
	reasons := append([]string(nil), node.queryIDs...)
	key := goSelectionEntityKey(node.entity)
	for _, edge := range edges {
		if edge.from == key || edge.to == key {
			reasons = appendUniqueString(reasons, edgeIDs[pythonSelectionEdgeKey(edge)])
		}
	}
	sort.Strings(reasons)
	if len(reasons) == 0 {
		return []string{"bounded-structural-neighbor"}
	}
	if len(reasons) > 8 {
		reasons = reasons[:8]
	}
	return reasons
}

func pythonSelectionEdgeIDs(edges []pythonSelectionEdge) map[string]string {
	result := make(map[string]string, pythonSelectionMaxCallEndpoints)
	for index, edge := range edges {
		result[pythonSelectionEdgeKey(edge)] = fmt.Sprintf("e%d", index+1)
	}
	return result
}

func pythonSelectionEdgeKey(edge pythonSelectionEdge) string {
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

func sortPythonSelectionEdges(edges []pythonSelectionEdge) {
	sort.Slice(edges, func(i, j int) bool {
		return pythonSelectionEdgeKey(edges[i]) < pythonSelectionEdgeKey(edges[j])
	})
}

func appendUniquePythonSelectionEdge(
	edges []pythonSelectionEdge,
	edge pythonSelectionEdge,
) []pythonSelectionEdge {
	if hasPythonSelectionEdge(edges, edge) {
		return edges
	}
	return append(edges, edge)
}

func hasPythonSelectionEdge(edges []pythonSelectionEdge, edge pythonSelectionEdge) bool {
	key := pythonSelectionEdgeKey(edge)
	for _, existing := range edges {
		if pythonSelectionEdgeKey(existing) == key {
			return true
		}
	}
	return false
}

func appendUniqueStrings(values []string, additions ...string) []string {
	for _, addition := range additions {
		values = appendUniqueString(values, addition)
	}
	sort.Strings(values)
	return values
}

func containsString(values []string, value string) bool {
	for _, existing := range values {
		if existing == value {
			return true
		}
	}
	return false
}

func boundedPythonSelectionWarnings(values []string) []string {
	result := make([]string, 0, pythonSelectionMaxWarnings)
	for _, value := range values {
		if !goSelectionAnalyzerText(value, false) {
			continue
		}
		result = appendUniqueString(result, value)
	}
	sort.Strings(result)
	if len(result) > pythonSelectionMaxWarnings {
		result = result[:pythonSelectionMaxWarnings]
	}
	return result
}

func EncodePythonSourceSelection(value any) ([]byte, error) {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(encoded, '\n'), nil
}
