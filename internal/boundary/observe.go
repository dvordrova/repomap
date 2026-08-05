// Package boundary projects exact local resource-boundary call sites from
// build-selected Go source into typed observations. It is a bounded local
// producer: it reads only filtered repository files, parses them with the
// standard library parser, and applies a FIXED detector list (import path +
// call-pattern matchers) — never a plugin registry, never a generic analyzer
// framework. Every observed call site is either published as an observation or
// reported under an explicit closed omission; nothing is dropped silently.
package boundary

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/reporead"
)

const (
	maxBoundaryFileBytes   int64 = 8 << 20
	maxBoundaryFilesPerPkg       = 256
)

// Class is the typed operation class of an observed resource boundary.
type Class string

const (
	// ClassPersistentStorage covers databases, object/file storage and
	// durable writes: database/sql, sqlx, gorm, xorm, redis, mongo, S3/OSS,
	// and os file storage writes.
	ClassPersistentStorage Class = "persistent_storage"
	// ClassOutboundClient covers outbound network clients: http.Client
	// construction and non-handler transport use, grpc dials/client stubs,
	// and SDK client constructors (New*Client from external packages).
	ClassOutboundClient Class = "outbound_client"
)

func (class Class) Valid() bool {
	return class == ClassPersistentStorage || class == ClassOutboundClient
}

// Observation is one exact observed resource-boundary call site. Location is
// the exact position of the call; Symbol is the enclosing function or method.
type Observation struct {
	Class       Class             `json:"class"`
	ImportPath  string            `json:"import_path"`
	PackagePath string            `json:"package_path"`
	Location    evidence.Location `json:"location"`
	Symbol      string            `json:"symbol,omitempty"`
}

// Result carries the complete observation set plus closed omissions. A file
// or call site that cannot be observed is recorded under Omissions, never
// silently dropped.
type Result struct {
	Observations []Observation `json:"observations"`
	Omissions    []Omission    `json:"omissions,omitempty"`
}

// Omission is a closed, wire-visible reason a call site could not be observed.
type Omission struct {
	Reason string `json:"reason"`
	Path   string `json:"path,omitempty"`
	Count  int    `json:"count"`
}

// detector is one fixed import-path + call-pattern matcher.
type detector struct {
	importPath string
	class      Class
	// callNames, when non-empty, restricts the detector to these function
	// names (selector final segment). Empty matches any call through the
	// imported package.
	callNames map[string]bool
	// anyNewClient matches any `New<X>Client` selector call from the
	// imported package (SDK client constructors).
	anyNewClient bool
}

var fixedDetectors = []detector{
	// Persistent storage.
	{importPath: "database/sql", class: ClassPersistentStorage, callNames: map[string]bool{"Open": true, "OpenDB": true, "OpenConnector": true}},
	{importPath: "github.com/jmoiron/sqlx", class: ClassPersistentStorage, callNames: map[string]bool{"Open": true, "Openx": true, "Connect": true, "Connectx": true, "NewDb": true, "New": true}},
	{importPath: "gorm.io/gorm", class: ClassPersistentStorage, callNames: map[string]bool{"Open": true}},
	{importPath: "xorm.io/xorm", class: ClassPersistentStorage, callNames: map[string]bool{"NewEngine": true}},
	{importPath: "github.com/go-redis/redis", class: ClassPersistentStorage, callNames: map[string]bool{"NewClient": true}},
	{importPath: "github.com/go-redis/redis/v8", class: ClassPersistentStorage, callNames: map[string]bool{"NewClient": true}},
	{importPath: "github.com/go-redis/redis/v9", class: ClassPersistentStorage, callNames: map[string]bool{"NewClient": true}},
	{importPath: "github.com/redis/go-redis/v9", class: ClassPersistentStorage, callNames: map[string]bool{"NewClient": true}},
	{importPath: "github.com/redis/go-redis", class: ClassPersistentStorage, callNames: map[string]bool{"NewClient": true}},
	{importPath: "go.mongodb.org/mongo-driver/mongo", class: ClassPersistentStorage, callNames: map[string]bool{"Connect": true, "NewClient": true}},
	{importPath: "github.com/aws/aws-sdk-go/aws/session", class: ClassPersistentStorage, callNames: map[string]bool{"NewSession": true}},
	{importPath: "github.com/aws/aws-sdk-go-v2", class: ClassPersistentStorage, callNames: map[string]bool{"New": true}},
	{importPath: "github.com/aliyun/aliyun-oss-go-sdk/oss", class: ClassPersistentStorage, callNames: map[string]bool{"New": true}},
	{importPath: "os", class: ClassPersistentStorage, callNames: map[string]bool{"OpenFile": true, "Create": true, "WriteFile": true}},
	// Outbound network clients.
	{importPath: "net/http", class: ClassOutboundClient, callNames: map[string]bool{"NewRequest": true, "NewRequestWithContext": true, "NewClient": true, "Get": true, "Post": true, "PostForm": true, "Head": true, "Do": true}},
	{importPath: "google.golang.org/grpc", class: ClassOutboundClient, callNames: map[string]bool{"Dial": true, "DialContext": true, "NewClient": true}},
	{importPath: "google.golang.org/grpc/credentials/insecure", class: ClassOutboundClient, callNames: map[string]bool{"NewCredentials": true}},
	// SDK client constructors from any external package: anyNewClient.
	{importPath: "", class: ClassOutboundClient, anyNewClient: true},
}

func isExternalImport(importPath string, modulePaths []string) bool {
	if importPath == "" {
		return false
	}
	if strings.HasPrefix(importPath, "std/") {
		return false
	}
	// Standard-library packages have no dot in their first path element and
	// are never a client SDK of this repository.
	first := importPath
	if index := strings.Index(importPath, "/"); index > 0 {
		first = importPath[:index]
	}
	if !strings.Contains(first, ".") {
		return false
	}
	for _, modulePath := range modulePaths {
		if importPath == modulePath || strings.HasPrefix(importPath, modulePath+"/") {
			return false
		}
	}
	return true
}

// Observe scans the build-selected Go files of every retained package for
// fixed-detector resource-boundary call sites. It is bounded and
// deterministic; every unobservable file is recorded as a closed omission.
func Observe(
	ctx context.Context,
	repository string,
	filteredFiles []string,
	facts gofacts.Facts,
) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if strings.TrimSpace(repository) == "" {
		return Result{}, fmt.Errorf("resource boundary observation: repository path is required")
	}
	reader, err := reporead.New(repository)
	if err != nil {
		return Result{}, fmt.Errorf("resource boundary observation: %w", err)
	}
	defer reader.Close()

	filtered := make(map[string]struct{}, len(filteredFiles))
	for _, sourcePath := range filteredFiles {
		filtered[sourcePath] = struct{}{}
	}
	modulePaths := make([]string, 0, len(facts.Modules))
	for _, module := range facts.Modules {
		modulePaths = append(modulePaths, module.ModulePath)
	}
	sort.Strings(modulePaths)

	packages := append([]gofacts.PackageFact(nil), facts.Packages...)
	sort.Slice(packages, func(i, j int) bool {
		return packages[i].CanonicalPath < packages[j].CanonicalPath
	})

	result := Result{}
	seen := make(map[string]bool)
	for _, pkg := range packages {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		if pkg.CanonicalPath == "" {
			continue
		}
		files := packageGoFiles(pkg, filtered)
		if len(files) > maxBoundaryFilesPerPkg {
			result.Omissions = append(result.Omissions, Omission{
				Reason: "file_count", Path: pkg.CanonicalPath, Count: len(files) - maxBoundaryFilesPerPkg,
			})
			files = files[:maxBoundaryFilesPerPkg]
		}
		for _, sourcePath := range files {
			if err := ctx.Err(); err != nil {
				return Result{}, err
			}
			content, readErr := reader.ReadFileNoSymlinks(sourcePath, maxBoundaryFileBytes)
			if readErr != nil || content.Truncated || !utf8.Valid(content.Bytes) {
				result.Omissions = append(result.Omissions, Omission{
					Reason: "unreadable", Path: sourcePath, Count: 1,
				})
				continue
			}
			observations, parseErr := observeFile(sourcePath, content.Bytes, pkg.CanonicalPath, modulePaths)
			if parseErr != nil {
				result.Omissions = append(result.Omissions, Omission{
					Reason: "unparseable", Path: sourcePath, Count: 1,
				})
				continue
			}
			for _, observation := range observations {
				key := string(observation.Class) + "\x00" + observation.ImportPath + "\x00" +
					observation.Location.Path + "\x00" + fmt.Sprint(observation.Location.Line) + "\x00" +
					fmt.Sprint(observation.Location.Column) + "\x00" + observation.Symbol
				if seen[key] {
					continue
				}
				seen[key] = true
				result.Observations = append(result.Observations, observation)
			}
		}
	}
	sort.Slice(result.Observations, func(i, j int) bool {
		left, right := result.Observations[i], result.Observations[j]
		if left.Location.Path != right.Location.Path {
			return left.Location.Path < right.Location.Path
		}
		if left.Location.Line != right.Location.Line {
			return left.Location.Line < right.Location.Line
		}
		if left.Location.Column != right.Location.Column {
			return left.Location.Column < right.Location.Column
		}
		return left.Symbol < right.Symbol
	})
	return result, nil
}

func packageGoFiles(pkg gofacts.PackageFact, filtered map[string]struct{}) []string {
	seen := make(map[string]struct{}, len(pkg.Files))
	var files []string
	for _, sourcePath := range pkg.Files {
		if _, allowed := filtered[sourcePath]; !allowed || !strings.HasSuffix(sourcePath, ".go") {
			continue
		}
		if _, duplicate := seen[sourcePath]; duplicate {
			continue
		}
		seen[sourcePath] = struct{}{}
		files = append(files, sourcePath)
	}
	sort.Strings(files)
	return files
}

func observeFile(
	sourcePath string,
	content []byte,
	packagePath string,
	modulePaths []string,
) ([]Observation, error) {
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, sourcePath, content, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	imports := make(map[string]string) // local name -> import path
	for _, spec := range file.Imports {
		if spec == nil || spec.Path == nil {
			continue
		}
		importPath := strings.Trim(spec.Path.Value, "\"")
		name := importLocalName(spec, importPath)
		imports[name] = importPath
	}
	var observations []Observation
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || call.Fun == nil {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || selector.Sel == nil {
			// http.Client{} composite literal is not a call; handled below.
			return true
		}
		pkgIdent, ok := selector.X.(*ast.Ident)
		if !ok {
			return true
		}
		importPath, imported := imports[pkgIdent.Name]
		if !imported {
			return true
		}
		name := selector.Sel.Name
		for _, detector := range fixedDetectors {
			if !detectorMatches(detector, importPath, name, modulePaths) {
				continue
			}
			position := fileSet.PositionFor(selector.Sel.Pos(), false)
			if position.Filename == "" || position.Line <= 0 || position.Column <= 0 {
				continue
			}
			observations = append(observations, Observation{
				Class:       detector.class,
				ImportPath:  importPath,
				PackagePath: packagePath,
				Location: evidence.Location{
					Path: position.Filename, Line: position.Line, Column: position.Column,
				},
				Symbol: enclosingFunction(fileSet, file, call),
			})
			break
		}
		return true
	})
	// http.Client{} composite literal (client construction without a call).
	ast.Inspect(file, func(node ast.Node) bool {
		literal, ok := node.(*ast.CompositeLit)
		if !ok || literal.Type == nil {
			return true
		}
		selector, ok := literal.Type.(*ast.SelectorExpr)
		if !ok || selector.Sel == nil || selector.Sel.Name != "Client" {
			return true
		}
		pkgIdent, ok := selector.X.(*ast.Ident)
		if !ok {
			return true
		}
		if imports[pkgIdent.Name] != "net/http" {
			return true
		}
		position := fileSet.PositionFor(selector.Sel.Pos(), false)
		if position.Filename == "" || position.Line <= 0 || position.Column <= 0 {
			return true
		}
		observations = append(observations, Observation{
			Class: ClassOutboundClient, ImportPath: "net/http", PackagePath: packagePath,
			Location: evidence.Location{
				Path: position.Filename, Line: position.Line, Column: position.Column,
			},
			Symbol: enclosingFunction(fileSet, file, literal),
		})
		return true
	})
	return observations, nil
}

func detectorMatches(d detector, importPath, name string, modulePaths []string) bool {
	if d.anyNewClient {
		if !isExternalImport(importPath, modulePaths) || !isClientConstructor(name) {
			return false
		}
		return true
	}
	if d.importPath != importPath {
		return false
	}
	if len(d.callNames) == 0 {
		return true
	}
	return d.callNames[name]
}

func isClientConstructor(name string) bool {
	if !strings.HasPrefix(name, "New") || !strings.HasSuffix(name, "Client") || len(name) <= len("NewClient") {
		return false
	}
	return true
}

func importLocalName(spec *ast.ImportSpec, importPath string) string {
	if spec.Name != nil && spec.Name.Name != "" && spec.Name.Name != "_" && spec.Name.Name != "." {
		return spec.Name.Name
	}
	base := path.Base(importPath)
	if base == "." || base == "/" || base == "" {
		return importPath
	}
	return base
}

func enclosingFunction(fileSet *token.FileSet, file *ast.File, node ast.Node) string {
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Name == nil || function.Body == nil {
			continue
		}
		start := fileSet.PositionFor(function.Pos(), false)
		end := fileSet.PositionFor(function.End(), false)
		position := fileSet.PositionFor(node.Pos(), false)
		if position.Line < start.Line || position.Line > end.Line {
			continue
		}
		name := function.Name.Name
		if function.Recv != nil && len(function.Recv.List) > 0 {
			recv := function.Recv.List[0]
			if recvType, ok := recv.Type.(*ast.StarExpr); ok {
				if ident, ok := recvType.X.(*ast.Ident); ok {
					name = "(*" + ident.Name + ")." + name
				}
			} else if ident, ok := recv.Type.(*ast.Ident); ok {
				name = ident.Name + "." + name
			}
		}
		return name
	}
	return ""
}
