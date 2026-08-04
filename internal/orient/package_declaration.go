package orient

import (
	"context"
	"go/parser"
	"go/token"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/reporead"
)

const maxPackageDeclarationSourceBytes int64 = 8 << 20

// exactPackageDeclarationLocations selects one deterministic build-selected
// source file per package and parses only its package clause. A package-level
// read or parse failure makes that optional evidence unavailable; it never
// falls through to another file or guesses a nearby declaration.
func exactPackageDeclarationLocations(
	ctx context.Context,
	repository string,
	filteredFiles []string,
	facts gofacts.Facts,
) (map[string]evidence.Location, error) {
	result := make(map[string]evidence.Location)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	reader, err := reporead.New(repository)
	if err != nil {
		return result, nil
	}
	defer reader.Close()

	filtered := make(map[string]struct{}, len(filteredFiles))
	for _, sourcePath := range filteredFiles {
		filtered[sourcePath] = struct{}{}
	}
	packages := append([]gofacts.PackageFact(nil), facts.Packages...)
	sort.Slice(packages, func(i, j int) bool {
		return packages[i].CanonicalPath < packages[j].CanonicalPath
	})
	for _, pkg := range packages {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if pkg.CanonicalPath == "" || pkg.Name == "" {
			continue
		}
		candidates := make([]string, 0, len(pkg.Files))
		seen := make(map[string]struct{}, len(pkg.Files))
		for _, sourcePath := range pkg.Files {
			if _, allowed := filtered[sourcePath]; !allowed || !strings.HasSuffix(sourcePath, ".go") {
				continue
			}
			if _, duplicate := seen[sourcePath]; duplicate {
				continue
			}
			seen[sourcePath] = struct{}{}
			candidates = append(candidates, sourcePath)
		}
		if len(candidates) == 0 {
			continue
		}
		sort.Strings(candidates)
		sourcePath := candidates[0]
		content, readErr := reader.ReadFileNoSymlinks(sourcePath, maxPackageDeclarationSourceBytes)
		if readErr != nil || content.Truncated || !utf8.Valid(content.Bytes) {
			continue
		}
		files := token.NewFileSet()
		parsed, parseErr := parser.ParseFile(files, sourcePath, content.Bytes, parser.PackageClauseOnly)
		if parseErr != nil || parsed.Name == nil || parsed.Name.Name != pkg.Name {
			continue
		}
		position := files.PositionFor(parsed.Package, true)
		if position.Filename != sourcePath || position.Line <= 0 || position.Column <= 0 {
			continue
		}
		result[pkg.CanonicalPath] = evidence.Location{
			Path: sourcePath, Line: position.Line, Column: position.Column,
		}
	}
	return result, nil
}
