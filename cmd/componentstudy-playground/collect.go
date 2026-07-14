package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	analysis "github.com/dvordrova/repomap/internal/analyzer"
	"github.com/dvordrova/repomap/internal/componentstudy"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/report"
)

type packageScopeArtifact struct {
	Version          int                   `json:"version"`
	Command          []string              `json:"command"`
	WorkingDirectory string                `json:"working_directory"`
	ImportPath       string                `json:"import_path"`
	Name             string                `json:"name"`
	Directory        string                `json:"directory"`
	Files            []packageFileArtifact `json:"files"`
	Warnings         []string              `json:"warnings,omitempty"`
}

type packageFileArtifact struct {
	Path            string   `json:"path"`
	Kind            string   `json:"kind"`
	SemanticScore   int      `json:"semantic_score"`
	MatchedTerms    []string `json:"matched_terms,omitempty"`
	Openable        bool     `json:"openable"`
	Included        bool     `json:"included"`
	SelectionReason string   `json:"selection_reason,omitempty"`
	Omission        string   `json:"omission,omitempty"`
	SeedFileID      string   `json:"seed_file_id,omitempty"`
}

type packageFile struct {
	Path string
	Kind string
	Rank int
}

type goListPackage struct {
	Dir         string   `json:"Dir"`
	ImportPath  string   `json:"ImportPath"`
	Name        string   `json:"Name"`
	GoFiles     []string `json:"GoFiles"`
	CgoFiles    []string `json:"CgoFiles"`
	TestGoFiles []string `json:"TestGoFiles"`
}

type symbolCatalogArtifact struct {
	Version        int                  `json:"version"`
	RankTerms      []string             `json:"rank_terms"`
	MaxFiles       int                  `json:"max_files"`
	MaxPerFile     int                  `json:"max_symbols_per_file"`
	Files          []symbolFileArtifact `json:"files"`
	CandidateCount int                  `json:"candidate_count"`
}

type symbolFileArtifact struct {
	Path           string                           `json:"path"`
	TargetLine     int                              `json:"target_line"`
	DurationMillis int64                            `json:"duration_ms"`
	Resolution     *analysis.LocationResolution     `json:"resolution,omitempty"`
	SeedSymbols    []componentstudy.SymbolCandidate `json:"seed_symbols,omitempty"`
	Error          string                           `json:"error,omitempty"`
}

func collectPackageScope(ctx context.Context, run authorizedRun, terms []string) (packageScopeArtifact, []packageFile, error) {
	workingDirectory := filepath.Dir(filepath.Join(run.analysisRoot, filepath.FromSlash(run.anchor.Path)))
	command := exec.CommandContext(ctx, "go", "list", "-json", ".")
	command.Dir = workingDirectory
	stdout, err := command.Output()
	if err != nil {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			message := cleanText(string(exitError.Stderr), 2048)
			if message != "" {
				return packageScopeArtifact{}, nil, fmt.Errorf("componentstudy-playground: go list package: %w: %s", err, message)
			}
		}
		return packageScopeArtifact{}, nil, fmt.Errorf("componentstudy-playground: go list package: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(stdout))
	var pkg goListPackage
	if err := decoder.Decode(&pkg); err != nil {
		return packageScopeArtifact{}, nil, fmt.Errorf("componentstudy-playground: decode go list package: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return packageScopeArtifact{}, nil, fmt.Errorf("componentstudy-playground: go list returned multiple packages")
		}
		return packageScopeArtifact{}, nil, fmt.Errorf("componentstudy-playground: decode trailing go list output: %w", err)
	}
	if pkg.Dir == "" || pkg.ImportPath == "" || pkg.Name == "" {
		return packageScopeArtifact{}, nil, fmt.Errorf("componentstudy-playground: go list returned incomplete package identity")
	}
	packageDirectory, err := filepath.Abs(pkg.Dir)
	if err != nil {
		return packageScopeArtifact{}, nil, fmt.Errorf("componentstudy-playground: resolve package directory: %w", err)
	}
	if !pathWithin(run.analysisRoot, packageDirectory) {
		return packageScopeArtifact{}, nil, fmt.Errorf("componentstudy-playground: go list package is outside analysis root")
	}

	type discovered struct {
		name         string
		kind         string
		score        int
		matchedTerms []string
	}
	all := make([]discovered, 0, len(pkg.GoFiles)+len(pkg.CgoFiles)+len(pkg.TestGoFiles))
	for _, name := range pkg.GoFiles {
		all = append(all, discovered{name: name, kind: "go"})
	}
	for _, name := range pkg.CgoFiles {
		all = append(all, discovered{name: name, kind: "cgo"})
	}
	for _, name := range pkg.TestGoFiles {
		all = append(all, discovered{name: name, kind: "test"})
	}
	for index := range all {
		all[index].score, all[index].matchedTerms = fileSemanticScore(all[index].name, terms)
	}
	sort.SliceStable(all, func(i, j int) bool {
		leftSelected := all[i].name == filepath.Base(run.anchor.Path)
		rightSelected := all[j].name == filepath.Base(run.anchor.Path)
		if leftSelected != rightSelected {
			return leftSelected
		}
		if all[i].score != all[j].score {
			return all[i].score > all[j].score
		}
		if packageKindRank(all[i].kind) != packageKindRank(all[j].kind) {
			return packageKindRank(all[i].kind) < packageKindRank(all[j].kind)
		}
		return all[i].name < all[j].name
	})

	scope := packageScopeArtifact{
		Version:          artifactVersion,
		Command:          []string{"go", "list", "-json", "."},
		WorkingDirectory: workingDirectory,
		ImportPath:       pkg.ImportPath,
		Name:             pkg.Name,
		Directory:        packageDirectory,
	}
	seen := make(map[string]struct{})
	var included []packageFile
	for _, item := range all {
		absolute := filepath.Join(packageDirectory, item.name)
		relative, relErr := repoRelativePath(run.analysisRoot, absolute)
		artifact := packageFileArtifact{
			Path:          relative,
			Kind:          item.kind,
			SemanticScore: item.score,
			MatchedTerms:  append([]string(nil), item.matchedTerms...),
		}
		if relErr != nil {
			artifact.Path = filepath.ToSlash(item.name)
			artifact.Omission = relErr.Error()
			scope.Files = append(scope.Files, artifact)
			continue
		}
		if _, duplicate := seen[relative]; duplicate {
			artifact.Omission = "duplicate package file"
			scope.Files = append(scope.Files, artifact)
			continue
		}
		seen[relative] = struct{}{}
		_, artifact.Openable = run.openable[relative]
		if !artifact.Openable {
			artifact.Omission = "not present in verified run openable_paths"
			scope.Files = append(scope.Files, artifact)
			continue
		}
		if len(included) >= maxPackageFiles {
			artifact.Omission = fmt.Sprintf("package file limit %d", maxPackageFiles)
			scope.Files = append(scope.Files, artifact)
			continue
		}
		artifact.Included = true
		artifact.SeedFileID = stableID("file", relative)
		artifact.SelectionReason = "build-selected package file"
		if relative == run.anchor.Path {
			artifact.SelectionReason = "user-selected anchor"
		} else if len(item.matchedTerms) > 0 {
			artifact.SelectionReason = "filename matches research terms: " + strings.Join(item.matchedTerms, ", ")
		}
		included = append(included, packageFile{Path: relative, Kind: item.kind, Rank: len(included) + 1})
		scope.Files = append(scope.Files, artifact)
	}
	if !containsPackageFile(included, run.anchor.Path) {
		return packageScopeArtifact{}, nil, fmt.Errorf("componentstudy-playground: selected anchor is absent from its build-selected package files")
	}
	return scope, included, nil
}

func collectSymbolCatalog(
	ctx context.Context,
	resolver analysis.LocationResolver,
	run authorizedRun,
	packageFiles []packageFile,
	terms []string,
) (symbolCatalogArtifact, []componentstudy.SymbolCandidate, error) {
	catalog := symbolCatalogArtifact{
		Version:    artifactVersion,
		RankTerms:  terms,
		MaxFiles:   maxSymbolFiles,
		MaxPerFile: maxSymbolsPerFile,
	}
	var symbols []componentstudy.SymbolCandidate
	seen := make(map[string]struct{})
	queried := 0
	for _, file := range packageFiles {
		if queried >= maxSymbolFiles || len(symbols) >= maxSeedSymbols || !strings.HasSuffix(file.Path, ".go") {
			continue
		}
		queried++
		targetLine := targetLineForPath(run.componentAuthority.Anchors, file.Path)
		started := time.Now()
		resolution, err := resolver.ResolveLocation(ctx, analysis.LocationRequest{
			RepoPath:      run.analysisRoot,
			Location:      evidence.Location{Path: file.Path, Line: targetLine},
			MaxCandidates: maxSymbolsPerFile,
			RankTerms:     terms,
		})
		entry := symbolFileArtifact{
			Path:           file.Path,
			TargetLine:     targetLine,
			DurationMillis: time.Since(started).Milliseconds(),
		}
		if err != nil {
			if ctx.Err() != nil {
				return catalog, symbols, fmt.Errorf("componentstudy-playground: collect gopls symbols: %w", ctx.Err())
			}
			entry.Error = cleanText(err.Error(), 2048)
			catalog.Files = append(catalog.Files, entry)
			continue
		}
		entry.Resolution = &resolution
		for index, candidate := range resolution.Candidates {
			if len(symbols) >= maxSeedSymbols || candidate.Entity.Location == nil {
				break
			}
			location := candidate.Entity.Location
			if location.Path != file.Path || location.Line <= 0 {
				continue
			}
			id := stableID(
				"symbol",
				location.Path,
				fmt.Sprintf("%d", location.Line),
				fmt.Sprintf("%d", location.Column),
				string(candidate.Entity.Kind),
				candidate.Entity.Name,
			)
			if _, duplicate := seen[id]; duplicate {
				continue
			}
			seen[id] = struct{}{}
			reason := "gopls declaration candidate"
			if candidate.Match != "" {
				reason += " (" + candidate.Match + ")"
			}
			if len(candidate.RankReasons) > 0 {
				reason += "; " + strings.Join(candidate.RankReasons, "; ")
			}
			detail := candidate.Match
			if resolution.Provenance.Version != "" {
				detail = resolution.Provenance.Version + " " + detail
			}
			item := componentstudy.SymbolCandidate{
				ID:     id,
				Rank:   len(symbols) + 1,
				Name:   cleanText(candidate.Entity.Name, 256),
				Kind:   cleanText(string(candidate.Entity.Kind), 64),
				Path:   location.Path,
				Line:   location.Line,
				Column: location.Column,
				Reason: cleanText(reason, 512),
				Provenance: componentstudy.Provenance{
					Source:    "gopls",
					Operation: "document_symbols",
					Detail:    cleanText(detail, 256),
				},
				Certainty: componentCertainty(candidate.Certainty),
			}
			// Preserve the resolver order inside each file while package-file rank
			// remains the primary frontier choice.
			item.Rank = file.Rank*maxSymbolsPerFile + index + 1
			symbols = append(symbols, item)
			entry.SeedSymbols = append(entry.SeedSymbols, item)
		}
		catalog.Files = append(catalog.Files, entry)
	}
	sort.SliceStable(symbols, func(i, j int) bool {
		if symbols[i].Rank != symbols[j].Rank {
			return symbols[i].Rank < symbols[j].Rank
		}
		return symbols[i].ID < symbols[j].ID
	})
	for index := range symbols {
		symbols[index].Rank = index + 1
	}
	normalizedByID := make(map[string]componentstudy.SymbolCandidate, len(symbols))
	for _, symbol := range symbols {
		normalizedByID[symbol.ID] = symbol
	}
	for fileIndex := range catalog.Files {
		for symbolIndex, symbol := range catalog.Files[fileIndex].SeedSymbols {
			if normalized, ok := normalizedByID[symbol.ID]; ok {
				catalog.Files[fileIndex].SeedSymbols[symbolIndex] = normalized
			}
		}
	}
	catalog.CandidateCount = len(symbols)
	return catalog, symbols, nil
}

func packageKindRank(kind string) int {
	switch kind {
	case "go":
		return 0
	case "cgo":
		return 1
	case "test":
		return 2
	default:
		return 3
	}
}

func targetLineForPath(anchors []report.AnchorAuthority, path string) int {
	for _, anchor := range anchors {
		if anchor.Path == path && len(anchor.AllowedLines) > 0 {
			return anchor.AllowedLines[0]
		}
	}
	return 1
}

func componentCertainty(certainty evidence.Certainty) componentstudy.Certainty {
	switch certainty {
	case evidence.CertaintyVerified:
		return componentstudy.CertaintyVerified
	case evidence.CertaintyObserved:
		return componentstudy.CertaintyObserved
	case evidence.CertaintyStatic:
		return componentstudy.CertaintyStatic
	case evidence.CertaintyPossible:
		return componentstudy.CertaintyPossible
	case evidence.CertaintyHypothesis:
		return componentstudy.CertaintyHypothesis
	default:
		return componentstudy.CertaintyPossible
	}
}

func containsPackageFile(files []packageFile, path string) bool {
	for _, file := range files {
		if file.Path == path {
			return true
		}
	}
	return false
}

func rankTerms(values ...string) []string {
	stop := map[string]struct{}{
		"after": {}, "and": {}, "are": {}, "does": {}, "for": {}, "from": {},
		"how": {}, "into": {}, "is": {}, "of": {}, "the": {}, "this": {},
		"what": {}, "when": {}, "where": {}, "which": {}, "with": {},
	}
	seen := make(map[string]struct{})
	var result []string
	for _, value := range values {
		for _, term := range strings.FieldsFunc(strings.ToLower(value), func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsNumber(r)
		}) {
			if len(term) < 3 || len(term) > 64 {
				continue
			}
			if _, ignored := stop[term]; ignored {
				continue
			}
			if _, duplicate := seen[term]; duplicate {
				continue
			}
			seen[term] = struct{}{}
			result = append(result, term)
			if len(result) == 16 {
				return result
			}
		}
	}
	return result
}

func fileSemanticScore(name string, terms []string) (int, []string) {
	name = strings.ToLower(filepath.ToSlash(name))
	base := strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
	var matched []string
	score := 0
	for _, term := range terms {
		if !strings.Contains(name, term) {
			continue
		}
		matched = append(matched, term)
		score++
		if base == term {
			score++
		}
	}
	return score, matched
}
