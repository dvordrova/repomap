package semanticmap

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/lspclient"
)

const (
	pyrightWorkspaceMaxQueries       = 12
	pyrightWorkspaceMaxRawResults    = 512
	pyrightWorkspaceMaxHitsPerQuery  = 64
	pyrightWorkspaceMaxQueryIDBytes  = 64
	pyrightWorkspaceMaxQueryBytes    = 512
	pyrightWorkspaceMaxScalarBytes   = 240
	pyrightWorkspaceMaxURIBytes      = 4 << 10
	pyrightWorkspaceMaxPosition      = 10_000_000
	pyrightWorkspaceRequestTimeout   = 30 * time.Second
	pyrightWorkspaceWarmupTimeout    = 20 * time.Second
	pyrightWorkspaceWarmupAttempts   = 20
	pyrightWorkspaceWarmupDelay      = 500 * time.Millisecond
	pyrightWorkspaceShutdownTimeout  = 2 * time.Second
	pyrightWorkspaceUnknownVersion   = "unknown"
	pyrightWorkspaceClientName       = "repomap-python-source-selector"
	pyrightWorkspaceClientVersion    = "1"
	pyrightWorkspaceTruncationFormat = "Pyright workspace/symbol query %s truncated from %d to %d callable repository hits"
)

type pyrightWorkspaceQuery struct {
	ID   string
	Text string
}

type pyrightWorkspaceHit struct {
	QueryID   string
	Name      string
	Container string
	Path      string
	Kind      evidence.EntityKind
	Line      int
	Column    int
}

type pyrightWorkspaceResult struct {
	Version  string
	Hits     []pyrightWorkspaceHit
	Warnings []string
}

type pythonWorkspaceSymbolFinder interface {
	Find(context.Context, string, []pyrightWorkspaceQuery) (pyrightWorkspaceResult, error)
}

type pyrightWorkspaceSymbolFinder struct {
	binary string
}

type pyrightWorkspacePosition struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type pyrightWorkspaceRange struct {
	Start pyrightWorkspacePosition `json:"start"`
	End   pyrightWorkspacePosition `json:"end"`
}

type pyrightWorkspaceLocation struct {
	URI   string                `json:"uri"`
	Range pyrightWorkspaceRange `json:"range"`
}

type pyrightWorkspaceRawSymbol struct {
	Name          string                   `json:"name"`
	Kind          int                      `json:"kind"`
	ContainerName string                   `json:"containerName,omitempty"`
	Location      pyrightWorkspaceLocation `json:"location"`
}

type pyrightWorkspaceInitializeResult struct {
	Capabilities struct {
		WorkspaceSymbolProvider any `json:"workspaceSymbolProvider,omitempty"`
	} `json:"capabilities"`
	ServerInfo struct {
		Name    string `json:"name,omitempty"`
		Version string `json:"version,omitempty"`
	} `json:"serverInfo,omitempty"`
}

func newPyrightWorkspaceSymbolFinder(binary string) pythonWorkspaceSymbolFinder {
	return &pyrightWorkspaceSymbolFinder{binary: strings.TrimSpace(binary)}
}

func (finder *pyrightWorkspaceSymbolFinder) Find(
	ctx context.Context,
	repositoryPath string,
	queries []pyrightWorkspaceQuery,
) (result pyrightWorkspaceResult, resultErr error) {
	repoPath, err := pyrightWorkspaceRepositoryPath(repositoryPath)
	if err != nil {
		return pyrightWorkspaceResult{}, err
	}
	if err := validatePyrightWorkspaceQueries(queries); err != nil {
		return pyrightWorkspaceResult{}, err
	}
	binary, err := pyrightWorkspaceBinary(finder.binary)
	if err != nil {
		return pyrightWorkspaceResult{}, err
	}
	client, err := lspclient.Start(ctx, lspclient.Options{
		Binary: binary,
		Args:   []string{"--stdio"},
		Dir:    repoPath,
		Configuration: map[string]any{
			"python": map[string]any{
				"analysis": map[string]string{
					"diagnosticMode": "workspace",
				},
			},
		},
	})
	if err != nil {
		return pyrightWorkspaceResult{}, fmt.Errorf(
			"pyright workspace symbols: start language server: %w",
			err,
		)
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(
			context.Background(),
			pyrightWorkspaceShutdownTimeout,
		)
		defer cancel()
		if closeErr := client.Close(closeCtx); closeErr != nil {
			closeErr = fmt.Errorf(
				"pyright workspace symbols: close language server: %w",
				closeErr,
			)
			if resultErr != nil {
				resultErr = errors.Join(resultErr, closeErr)
			}
			// Close kills the server after a failed bounded shutdown. A
			// cleanup timeout does not invalidate already returned hits.
		}
	}()

	rootURI := pyrightWorkspacePathURI(repoPath)
	var initialized pyrightWorkspaceInitializeResult
	if err := pyrightWorkspaceCall(
		ctx,
		client,
		"initialize",
		map[string]any{
			"processId": nil,
			"rootUri":   rootURI,
			"workspaceFolders": []map[string]string{{
				"uri":  rootURI,
				"name": filepath.Base(repoPath),
			}},
			"capabilities": map[string]any{
				"workspace": map[string]any{
					"configuration":    true,
					"workspaceFolders": true,
					"symbol": map[string]any{
						"symbolKind": map[string]any{
							"valueSet": []int{6, 9, 12},
						},
					},
				},
			},
			"clientInfo": map[string]string{
				"name":    pyrightWorkspaceClientName,
				"version": pyrightWorkspaceClientVersion,
			},
		},
		&initialized,
	); err != nil {
		return pyrightWorkspaceResult{}, fmt.Errorf(
			"pyright workspace symbols: initialize workspace: %w",
			err,
		)
	}
	if !pyrightWorkspaceCapabilityAdvertised(
		initialized.Capabilities.WorkspaceSymbolProvider,
	) {
		return pyrightWorkspaceResult{}, fmt.Errorf(
			"pyright workspace symbols: server did not advertise workspace/symbol",
		)
	}
	if !pyrightWorkspaceScalar(
		initialized.ServerInfo.Name,
		pyrightWorkspaceMaxScalarBytes,
		true,
	) || !pyrightWorkspaceScalar(
		initialized.ServerInfo.Version,
		pyrightWorkspaceMaxScalarBytes,
		true,
	) {
		return pyrightWorkspaceResult{}, fmt.Errorf(
			"pyright workspace symbols: server identity exceeds the scalar budget",
		)
	}
	result.Version = initialized.ServerInfo.Version
	if result.Version == "" {
		result.Version = pyrightWorkspaceUnknownVersion
		result.Warnings = append(
			result.Warnings,
			"Pyright did not report a server version",
		)
	}
	if err := client.Notify("initialized", map[string]any{}); err != nil {
		return pyrightWorkspaceResult{}, fmt.Errorf(
			"pyright workspace symbols: send initialized: %w",
			err,
		)
	}

	result.Hits = make(
		[]pyrightWorkspaceHit,
		0,
		pyrightWorkspaceMaxQueries*pyrightWorkspaceMaxHitsPerQuery,
	)
	for _, query := range queries {
		hits, err := pyrightWorkspaceFindQuery(
			ctx,
			client,
			repoPath,
			query,
		)
		if err != nil {
			return pyrightWorkspaceResult{}, err
		}
		hits, discarded, err := filterPyrightWorkspaceTrackedHits(repoPath, hits)
		if err != nil {
			return pyrightWorkspaceResult{}, fmt.Errorf(
				"pyright workspace symbols: query %s: %w",
				query.ID,
				err,
			)
		}
		if discarded > 0 {
			result.Warnings = append(
				result.Warnings,
				fmt.Sprintf(
					"Pyright workspace/symbol query %s discarded %d non-ordinary or untracked repository hit(s)",
					query.ID,
					discarded,
				),
			)
		}
		sortPyrightWorkspaceHitsForQuery(hits, query.Text)
		if len(hits) > pyrightWorkspaceMaxHitsPerQuery {
			result.Warnings = append(
				result.Warnings,
				fmt.Sprintf(
					pyrightWorkspaceTruncationFormat,
					query.ID,
					len(hits),
					pyrightWorkspaceMaxHitsPerQuery,
				),
			)
			hits = hits[:pyrightWorkspaceMaxHitsPerQuery]
		}
		result.Hits = append(result.Hits, hits...)
	}
	return result, nil
}

func validatePyrightWorkspaceQueries(queries []pyrightWorkspaceQuery) error {
	if len(queries) == 0 || len(queries) > pyrightWorkspaceMaxQueries {
		return fmt.Errorf(
			"pyright workspace symbols: query count must be between 1 and %d",
			pyrightWorkspaceMaxQueries,
		)
	}
	seen := make(map[string]struct{}, pyrightWorkspaceMaxQueries)
	for index, query := range queries {
		if !pyrightWorkspaceScalar(
			query.ID,
			pyrightWorkspaceMaxQueryIDBytes,
			false,
		) || !pyrightWorkspaceScalar(
			query.Text,
			pyrightWorkspaceMaxQueryBytes,
			false,
		) {
			return fmt.Errorf(
				"pyright workspace symbols: queries[%d] has invalid scalar metadata",
				index,
			)
		}
		if _, duplicate := seen[query.ID]; duplicate {
			return fmt.Errorf(
				"pyright workspace symbols: duplicate query ID %q",
				query.ID,
			)
		}
		seen[query.ID] = struct{}{}
	}
	return nil
}

func pyrightWorkspaceFindQuery(
	ctx context.Context,
	client *lspclient.Client,
	repoPath string,
	query pyrightWorkspaceQuery,
) ([]pyrightWorkspaceHit, error) {
	warmupCtx, cancel := context.WithTimeout(ctx, pyrightWorkspaceWarmupTimeout)
	defer cancel()
	return awaitPyrightWorkspaceCandidates(
		warmupCtx,
		query,
		pyrightWorkspaceWarmupAttempts,
		pyrightWorkspaceWarmupDelay,
		func(sampleCtx context.Context) ([]pyrightWorkspaceHit, error) {
			return pyrightWorkspaceQueryOnce(
				sampleCtx,
				client,
				repoPath,
				query,
			)
		},
	)
}

func awaitPyrightWorkspaceCandidates(
	ctx context.Context,
	query pyrightWorkspaceQuery,
	attempts int,
	delay time.Duration,
	queryOnce func(context.Context) ([]pyrightWorkspaceHit, error),
) ([]pyrightWorkspaceHit, error) {
	for attempt := 0; attempt < attempts; attempt++ {
		current, err := queryOnce(ctx)
		if err != nil {
			return nil, err
		}
		if len(current) > 0 {
			return current, nil
		}
		if attempt+1 == attempts {
			break
		}
		if delay <= 0 {
			continue
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, fmt.Errorf(
				"pyright workspace symbols: query %s did not return a nonempty candidate sample within the bounded warmup: %w",
				query.ID,
				ctx.Err(),
			)
		case <-timer.C:
		}
	}
	return nil, fmt.Errorf(
		"pyright workspace symbols: query %s did not return a nonempty candidate sample within the bounded warmup",
		query.ID,
	)
}

func pyrightWorkspaceQueryOnce(
	ctx context.Context,
	client *lspclient.Client,
	repoPath string,
	query pyrightWorkspaceQuery,
) ([]pyrightWorkspaceHit, error) {
	var raw []pyrightWorkspaceRawSymbol
	if err := pyrightWorkspaceCall(
		ctx,
		client,
		"workspace/symbol",
		map[string]string{"query": query.Text},
		&raw,
	); err != nil {
		return nil, fmt.Errorf(
			"pyright workspace symbols: query %s: %w",
			query.ID,
			err,
		)
	}
	if len(raw) > pyrightWorkspaceMaxRawResults {
		return nil, fmt.Errorf(
			"pyright workspace symbols: query %s returned %d raw results; limit is %d",
			query.ID,
			len(raw),
			pyrightWorkspaceMaxRawResults,
		)
	}
	hits := make(
		[]pyrightWorkspaceHit,
		0,
		pyrightWorkspaceMaxRawResults,
	)
	for index, symbol := range raw {
		if err := preflightPyrightWorkspaceRawSymbol(symbol); err != nil {
			return nil, fmt.Errorf(
				"pyright workspace symbols: query %s result[%d]: %w",
				query.ID,
				index,
				err,
			)
		}
		relativePath, inside, err := pyrightWorkspaceRelativePath(
			repoPath,
			symbol.Location.URI,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"pyright workspace symbols: query %s result[%d]: %w",
				query.ID,
				index,
				err,
			)
		}
		if !inside || !pyrightWorkspaceCallableKind(symbol.Kind) {
			continue
		}
		kind := evidence.EntityFunction
		if symbol.Kind == 6 {
			kind = evidence.EntityMethod
		}
		hits = append(hits, pyrightWorkspaceHit{
			QueryID:   query.ID,
			Name:      symbol.Name,
			Container: symbol.ContainerName,
			Path:      relativePath,
			Kind:      kind,
			Line:      symbol.Location.Range.Start.Line + 1,
			Column:    symbol.Location.Range.Start.Character + 1,
		})
	}
	sort.Slice(hits, func(i, j int) bool {
		return pyrightWorkspaceHitKey(hits[i]) <
			pyrightWorkspaceHitKey(hits[j])
	})
	deduplicated := make(
		[]pyrightWorkspaceHit,
		0,
		pyrightWorkspaceMaxRawResults,
	)
	for _, hit := range hits {
		if len(deduplicated) > 0 &&
			pyrightWorkspaceHitKey(deduplicated[len(deduplicated)-1]) ==
				pyrightWorkspaceHitKey(hit) {
			continue
		}
		deduplicated = append(deduplicated, hit)
	}
	return deduplicated, nil
}

func filterPyrightWorkspaceTrackedHits(
	repoPath string,
	hits []pyrightWorkspaceHit,
) ([]pyrightWorkspaceHit, int, error) {
	if len(hits) > pyrightWorkspaceMaxRawResults {
		return nil, 0, fmt.Errorf("tracked-hit input exceeds the processing budget")
	}
	paths := make(map[string]struct{}, pyrightWorkspaceMaxRawResults)
	args := make([]string, 0, pyrightWorkspaceMaxRawResults+5)
	args = append(args, "ls-files", "-v", "-z", "--")
	for _, hit := range hits {
		if _, duplicate := paths[hit.Path]; duplicate {
			continue
		}
		paths[hit.Path] = struct{}{}
		args = append(args, ":(literal)"+hit.Path)
	}
	if len(paths) == 0 {
		return nil, 0, nil
	}
	output, err := runGit(repoPath, args...)
	if err != nil {
		return nil, 0, err
	}
	ordinary := make(map[string]struct{}, len(paths))
	for _, record := range strings.Split(output, "\x00") {
		if !strings.HasPrefix(record, "H ") {
			continue
		}
		sourcePath := strings.TrimPrefix(record, "H ")
		if _, requested := paths[sourcePath]; requested {
			ordinary[sourcePath] = struct{}{}
		}
	}
	retained := make([]pyrightWorkspaceHit, 0, pyrightWorkspaceMaxRawResults)
	for _, hit := range hits {
		if _, ok := ordinary[hit.Path]; ok {
			retained = append(retained, hit)
		}
	}
	return retained, len(hits) - len(retained), nil
}

func sortPyrightWorkspaceHitsForQuery(hits []pyrightWorkspaceHit, query string) {
	sort.Slice(hits, func(i, j int) bool {
		leftRank := pyrightWorkspaceHitQueryRank(hits[i], query)
		rightRank := pyrightWorkspaceHitQueryRank(hits[j], query)
		if leftRank != rightRank {
			return leftRank > rightRank
		}
		return pyrightWorkspaceHitKey(hits[i]) < pyrightWorkspaceHitKey(hits[j])
	})
}

func pyrightWorkspaceHitQueryRank(hit pyrightWorkspaceHit, query string) int {
	query = normalizePythonSelectionTerm(strings.ToLower(query))
	score := 0
	for _, nameTerm := range splitGoSelectionName(hit.Name) {
		nameTerm = normalizePythonSelectionTerm(nameTerm)
		switch {
		case nameTerm == query:
			score = max(score, 3)
		case strings.HasPrefix(nameTerm, query) || strings.HasPrefix(query, nameTerm):
			score = max(score, 2)
		case strings.Contains(nameTerm, query) || strings.Contains(query, nameTerm):
			score = max(score, 1)
		}
	}
	if !goSelectionTestPath(hit.Path) {
		score += 1
	}
	if hit.Kind == evidence.EntityFunction {
		score += 1
	}
	return score
}

func preflightPyrightWorkspaceRawSymbol(
	symbol pyrightWorkspaceRawSymbol,
) error {
	if !pyrightWorkspaceScalar(
		symbol.Name,
		pyrightWorkspaceMaxScalarBytes,
		false,
	) || !pyrightWorkspaceScalar(
		symbol.ContainerName,
		pyrightWorkspaceMaxScalarBytes,
		true,
	) {
		return fmt.Errorf("symbol scalar exceeds the budget")
	}
	if !pyrightWorkspaceScalar(
		symbol.Location.URI,
		pyrightWorkspaceMaxURIBytes,
		false,
	) {
		return fmt.Errorf("symbol URI exceeds the budget")
	}
	if !pyrightWorkspaceRangeValid(symbol.Location.Range) {
		return fmt.Errorf("symbol range is invalid")
	}
	return nil
}

func pyrightWorkspaceRelativePath(
	repoPath string,
	rawURI string,
) (string, bool, error) {
	parsed, err := url.Parse(rawURI)
	if err != nil {
		return "", false, fmt.Errorf("parse symbol URI: %w", err)
	}
	if parsed.Scheme != "file" {
		return "", false, nil
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" ||
		(parsed.Host != "" && parsed.Host != "localhost") {
		return "", false, fmt.Errorf("file URI has unsupported authority or suffix")
	}
	decoded, err := url.PathUnescape(parsed.Path)
	if err != nil {
		return "", false, fmt.Errorf("decode symbol URI path: %w", err)
	}
	if !pyrightWorkspaceScalar(
		decoded,
		pyrightWorkspaceMaxURIBytes,
		false,
	) {
		return "", false, fmt.Errorf("decoded symbol path exceeds the budget")
	}
	absolutePath := filepath.Clean(filepath.FromSlash(decoded))
	relativePath, inside := pyrightWorkspaceLexicalRelative(
		repoPath,
		absolutePath,
	)
	if !inside {
		return "", false, nil
	}
	info, err := os.Lstat(absolutePath)
	if err != nil {
		return "", false, fmt.Errorf("inspect symbol path: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", false, nil
	}
	resolvedPath, err := filepath.EvalSymlinks(absolutePath)
	if err != nil {
		return "", false, fmt.Errorf("resolve symbol path: %w", err)
	}
	resolvedRelative, resolvedInside := pyrightWorkspaceLexicalRelative(
		repoPath,
		resolvedPath,
	)
	if !resolvedInside || resolvedRelative != relativePath {
		return "", false, nil
	}
	relativePath = filepath.ToSlash(relativePath)
	if !strings.HasSuffix(strings.ToLower(relativePath), ".py") {
		return "", false, nil
	}
	if !goSelectionCanonicalPath(relativePath) ||
		!pyrightWorkspaceScalar(
			relativePath,
			pyrightWorkspaceMaxScalarBytes,
			false,
		) {
		return "", false, fmt.Errorf(
			"repository symbol path exceeds the canonical path budget",
		)
	}
	return relativePath, true, nil
}

func pyrightWorkspaceLexicalRelative(
	repoPath string,
	absolutePath string,
) (string, bool) {
	relativePath, err := filepath.Rel(repoPath, absolutePath)
	if err != nil || relativePath == "." || relativePath == ".." ||
		strings.HasPrefix(
			relativePath,
			".."+string(filepath.Separator),
		) {
		return "", false
	}
	return relativePath, true
}

func pyrightWorkspaceRepositoryPath(value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf(
			"pyright workspace symbols: repository path is required",
		)
	}
	absolutePath, err := filepath.Abs(value)
	if err != nil {
		return "", fmt.Errorf(
			"pyright workspace symbols: resolve repository: %w",
			err,
		)
	}
	repoPath, err := filepath.EvalSymlinks(absolutePath)
	if err != nil {
		return "", fmt.Errorf(
			"pyright workspace symbols: resolve repository symlinks: %w",
			err,
		)
	}
	info, err := os.Stat(repoPath)
	if err != nil {
		return "", fmt.Errorf(
			"pyright workspace symbols: inspect repository: %w",
			err,
		)
	}
	if !info.IsDir() {
		return "", fmt.Errorf(
			"pyright workspace symbols: repository path is not a directory",
		)
	}
	return repoPath, nil
}

func pyrightWorkspaceBinary(configured string) (string, error) {
	binary := strings.TrimSpace(configured)
	if binary == "" {
		binary = "pyright-langserver"
	}
	resolved, err := exec.LookPath(binary)
	if err != nil {
		return "", fmt.Errorf(
			"pyright workspace symbols: %q was not found",
			binary,
		)
	}
	return resolved, nil
}

func pyrightWorkspaceCall(
	ctx context.Context,
	client *lspclient.Client,
	method string,
	params any,
	result any,
) error {
	callCtx, cancel := context.WithTimeout(
		ctx,
		pyrightWorkspaceRequestTimeout,
	)
	defer cancel()
	return client.Call(callCtx, method, params, result)
}

func pyrightWorkspacePathURI(value string) string {
	return (&url.URL{
		Scheme: "file",
		Path:   filepath.ToSlash(value),
	}).String()
}

func pyrightWorkspaceCallableKind(kind int) bool {
	return kind == 6 || kind == 9 || kind == 12
}

func pyrightWorkspaceRangeValid(value pyrightWorkspaceRange) bool {
	for _, position := range []pyrightWorkspacePosition{
		value.Start,
		value.End,
	} {
		if position.Line < 0 || position.Line > pyrightWorkspaceMaxPosition ||
			position.Character < 0 ||
			position.Character > pyrightWorkspaceMaxPosition {
			return false
		}
	}
	return value.End.Line > value.Start.Line ||
		value.End.Line == value.Start.Line &&
			value.End.Character >= value.Start.Character
}

func pyrightWorkspaceScalar(
	value string,
	maxBytes int,
	allowEmpty bool,
) bool {
	if len(value) > maxBytes || (!allowEmpty && value == "") {
		return false
	}
	return utf8.ValidString(value) &&
		!strings.ContainsRune(value, 0) &&
		!strings.ContainsAny(value, "\r\n") &&
		strings.TrimSpace(value) == value
}

func pyrightWorkspaceHitKey(hit pyrightWorkspaceHit) string {
	return fmt.Sprintf(
		"%s\x00%s\x00%09d\x00%09d\x00%s\x00%s\x00%s",
		hit.QueryID,
		hit.Path,
		hit.Line,
		hit.Column,
		hit.Kind,
		hit.Container,
		hit.Name,
	)
}

func pyrightWorkspaceCapabilityAdvertised(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case map[string]any:
		return true
	default:
		return false
	}
}
