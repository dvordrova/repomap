// Package workspacegraph defines a bounded immutable package graph over one
// already collected local Go-facts snapshot.
package workspacegraph

import (
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/workspacesnapshot"
)

const (
	maxModules              = 600
	maxPackages             = 4096
	maxFilesPerPackage      = 4096
	maxAggregateFiles       = 20_000
	maxEdges                = 1000
	maxScalarBytes          = 4096
	maxAggregateScalarBytes = 4 * 1024 * 1024
)

// Input is the complete existing-facts authority needed to construct a Graph.
type Input struct {
	Snapshot workspacesnapshot.Snapshot
	GoFacts  gofacts.Facts
}

// Module is one exact local Go module. Dir and GoMod are canonical
// analysis-root-relative slash paths; "." denotes the analysis root for Dir.
type Module struct {
	ID    string
	Path  string
	Dir   string
	Main  bool
	GoMod string
}

// Package is one exact local Go package and its safe structural files.
type Package struct {
	CanonicalPath     string
	Name              string
	ModuleID          string
	ModulePath        string
	Dir               string
	ModuleRelativeDir string
	Files             []File
}

// File is one safe analysis-root-relative package file. Openable is true only
// for exact membership in the input snapshot's source catalog.
type File struct {
	Path     string
	Openable bool
}

// Edge is one exact retained local package import.
type Edge struct {
	FromPackage string
	ToPackage   string
}

type moduleKey struct {
	id   string
	path string
	dir  string
}

type moduleLocationKey struct {
	path string
	dir  string
}

type edgeKey struct {
	from string
	to   string
}

// Graph is an immutable deterministic projection. All slices and indexes are
// private; accessors return defensive copies.
type Graph struct {
	modules       []Module
	packages      []Package
	edges         []Edge
	moduleLookup  map[moduleKey]int
	packageLookup map[string]int
	edgeLookup    map[edgeKey]int
	initialized   bool
}

// New constructs one graph without reading files, executing commands,
// invoking analyzers, or calling providers.
func New(input Input) (Graph, error) {
	if err := preflight(input.GoFacts); err != nil {
		return Graph{}, err
	}
	if input.Snapshot.AnalysisRoot() == "" ||
		input.Snapshot.RepositoryDigest() == "" ||
		input.Snapshot.CapturedInputsDigest() == "" ||
		input.Snapshot.Catalog().AnalysisRoot() == "" {
		return Graph{}, fmt.Errorf("workspace graph: workspace snapshot is unavailable")
	}

	modules, err := buildModules(input.GoFacts.Modules)
	if err != nil {
		return Graph{}, err
	}
	packages, err := buildPackages(input.GoFacts.Packages, modules, input.Snapshot)
	if err != nil {
		return Graph{}, err
	}
	edges := buildEdges(input.GoFacts.InternalEdges, packages)

	moduleLookup := make(map[moduleKey]int, min(len(modules), maxModules))
	for index, module := range modules {
		moduleLookup[moduleKey{id: module.ID, path: module.Path, dir: module.Dir}] = index
	}
	packageLookup := make(map[string]int, min(len(packages), maxPackages))
	for index, pkg := range packages {
		packageLookup[pkg.CanonicalPath] = index
	}
	edgeLookup := make(map[edgeKey]int, min(len(edges), maxEdges))
	for index, edge := range edges {
		edgeLookup[edgeKey{from: edge.FromPackage, to: edge.ToPackage}] = index
	}
	return Graph{
		modules:       modules,
		packages:      packages,
		edges:         edges,
		moduleLookup:  moduleLookup,
		packageLookup: packageLookup,
		edgeLookup:    edgeLookup,
		initialized:   true,
	}, nil
}

// Modules returns a defensive copy in deterministic order.
func (graph Graph) Modules() []Module {
	if !graph.initialized || graph.modules == nil {
		return nil
	}
	return append([]Module(nil), graph.modules...)
}

// Packages returns a deep defensive copy in deterministic order.
func (graph Graph) Packages() []Package {
	if !graph.initialized || graph.packages == nil {
		return nil
	}
	result := make([]Package, len(graph.packages))
	for index := range graph.packages {
		result[index] = clonePackage(graph.packages[index])
	}
	return result
}

// Edges returns a defensive copy in deterministic order.
func (graph Graph) Edges() []Edge {
	if !graph.initialized || graph.edges == nil {
		return nil
	}
	return append([]Edge(nil), graph.edges...)
}

// Module looks up one exact module identity without accepting an oversized or
// malformed caller-controlled key.
func (graph Graph) Module(id, modulePath, dir string) (Module, bool) {
	if !graph.initialized ||
		!queryScalarsBounded(id, modulePath, dir) ||
		!validOpaqueIdentity(id) ||
		!validImportIdentity(modulePath) ||
		!validDirectory(dir) {
		return Module{}, false
	}
	index, ok := graph.moduleLookup[moduleKey{id: id, path: modulePath, dir: dir}]
	if !ok {
		return Module{}, false
	}
	return graph.modules[index], true
}

// Package looks up one exact package identity and returns a deep copy.
func (graph Graph) Package(canonicalPath string) (Package, bool) {
	if !graph.initialized ||
		!queryScalarsBounded(canonicalPath) ||
		!validImportIdentity(canonicalPath) {
		return Package{}, false
	}
	index, ok := graph.packageLookup[canonicalPath]
	if !ok {
		return Package{}, false
	}
	return clonePackage(graph.packages[index]), true
}

// Edge looks up one exact local import without accepting oversized or
// malformed caller-controlled keys.
func (graph Graph) Edge(fromPackage, toPackage string) (Edge, bool) {
	if !graph.initialized ||
		!queryScalarsBounded(fromPackage, toPackage) ||
		!validImportIdentity(fromPackage) ||
		!validImportIdentity(toPackage) {
		return Edge{}, false
	}
	index, ok := graph.edgeLookup[edgeKey{from: fromPackage, to: toPackage}]
	if !ok {
		return Edge{}, false
	}
	return graph.edges[index], true
}

func preflight(facts gofacts.Facts) error {
	if len(facts.Modules) > maxModules {
		return fmt.Errorf("workspace graph: module facts exceed %d entries", maxModules)
	}
	if len(facts.Packages) > maxPackages {
		return fmt.Errorf("workspace graph: package facts exceed %d entries", maxPackages)
	}
	if len(facts.InternalEdges) > maxEdges {
		return fmt.Errorf("workspace graph: edge facts exceed %d entries", maxEdges)
	}

	totalFiles := 0
	for index, pkg := range facts.Packages {
		if len(pkg.Files) > maxFilesPerPackage {
			return fmt.Errorf(
				"workspace graph: package %d files exceed %d entries",
				index,
				maxFilesPerPackage,
			)
		}
		if len(pkg.Files) > maxAggregateFiles-totalFiles {
			return fmt.Errorf("workspace graph: package files exceed %d entries", maxAggregateFiles)
		}
		totalFiles += len(pkg.Files)
	}

	budget := scalarBudget{remaining: maxAggregateScalarBytes}
	for index, module := range facts.Modules {
		if !budget.consume(module.ID, module.ModulePath, module.ModuleDir, module.GoMod) {
			return fmt.Errorf("workspace graph: module %d scalar facts exceed bounds", index)
		}
	}
	for index, pkg := range facts.Packages {
		if !budget.consume(
			pkg.CanonicalPath,
			pkg.Name,
			pkg.ModuleID,
			pkg.ModulePath,
			pkg.PackageDir,
			pkg.ModuleRelativeDir,
		) {
			return fmt.Errorf("workspace graph: package %d scalar facts exceed bounds", index)
		}
		for _, file := range pkg.Files {
			if !budget.consume(file) {
				return fmt.Errorf("workspace graph: package %d scalar facts exceed bounds", index)
			}
		}
	}
	for index, edge := range facts.InternalEdges {
		if !budget.consume(edge.From, edge.To) {
			return fmt.Errorf("workspace graph: edge %d scalar facts exceed bounds", index)
		}
	}
	return nil
}

type scalarBudget struct {
	remaining int
}

func (budget *scalarBudget) consume(values ...string) bool {
	for _, value := range values {
		if len(value) > maxScalarBytes || len(value) > budget.remaining {
			return false
		}
		budget.remaining -= len(value)
	}
	return true
}

func buildModules(facts []gofacts.ModuleFact) ([]Module, error) {
	modules := make([]Module, 0, min(len(facts), maxModules))
	byID := make(map[string]Module, min(len(facts), maxModules))
	byLocation := make(map[moduleLocationKey]Module, min(len(facts), maxModules))
	for index, fact := range facts {
		if !validOpaqueIdentity(fact.ID) ||
			!validImportIdentity(fact.ModulePath) ||
			!validDirectory(fact.ModuleDir) ||
			(fact.GoMod != "" && !validFilePath(fact.GoMod)) {
			return nil, fmt.Errorf("workspace graph: module %d is invalid", index)
		}
		module := Module{
			ID: fact.ID, Path: fact.ModulePath, Dir: fact.ModuleDir,
			Main: fact.Main, GoMod: fact.GoMod,
		}
		if existing, duplicate := byID[module.ID]; duplicate {
			if existing != module {
				return nil, fmt.Errorf("workspace graph: module %d conflicts with an existing identity", index)
			}
			continue
		}
		location := moduleLocationKey{path: module.Path, dir: module.Dir}
		if existing, duplicate := byLocation[location]; duplicate {
			if existing != module {
				return nil, fmt.Errorf("workspace graph: module %d conflicts with an existing location", index)
			}
			continue
		}
		byID[module.ID] = module
		byLocation[location] = module
		modules = append(modules, module)
	}
	sort.Slice(modules, func(i, j int) bool {
		left, right := modules[i], modules[j]
		if left.Dir != right.Dir {
			return left.Dir < right.Dir
		}
		if left.Path != right.Path {
			return left.Path < right.Path
		}
		if left.ID != right.ID {
			return left.ID < right.ID
		}
		if left.Main != right.Main {
			return !left.Main && right.Main
		}
		return left.GoMod < right.GoMod
	})
	return modules, nil
}

func buildPackages(
	facts []gofacts.PackageFact,
	modules []Module,
	snapshot workspacesnapshot.Snapshot,
) ([]Package, error) {
	moduleByID := make(map[string]Module, min(len(modules), maxModules))
	for _, module := range modules {
		moduleByID[module.ID] = module
	}

	packages := make([]Package, 0, min(len(facts), maxPackages))
	byCanonicalPath := make(map[string]Package, min(len(facts), maxPackages))
	catalog := snapshot.Catalog()
	for index, fact := range facts {
		if !validPackageFact(fact) {
			continue
		}
		owner, ok := moduleByID[fact.ModuleID]
		if !ok || owner.Path != fact.ModulePath {
			return nil, fmt.Errorf("workspace graph: package %d has invalid module ownership", index)
		}
		if !packageWithinModule(
			fact.PackageDir,
			fact.ModuleRelativeDir,
			owner.Dir,
		) {
			return nil, fmt.Errorf("workspace graph: package %d has inconsistent directories", index)
		}

		files := make([]File, 0, min(len(fact.Files), maxFilesPerPackage))
		for _, filePath := range fact.Files {
			if !validFilePath(filePath) || path.Dir(filePath) != fact.PackageDir {
				continue
			}
			_, openable := catalog.Lookup(filePath)
			files = append(files, File{Path: filePath, Openable: openable})
		}
		sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
		files = compactFiles(files)

		pkg := Package{
			CanonicalPath:     fact.CanonicalPath,
			Name:              fact.Name,
			ModuleID:          fact.ModuleID,
			ModulePath:        fact.ModulePath,
			Dir:               fact.PackageDir,
			ModuleRelativeDir: fact.ModuleRelativeDir,
			Files:             files,
		}
		if existing, duplicate := byCanonicalPath[pkg.CanonicalPath]; duplicate {
			if !packagesEqual(existing, pkg) {
				return nil, fmt.Errorf("workspace graph: package %d conflicts with an existing identity", index)
			}
			continue
		}
		byCanonicalPath[pkg.CanonicalPath] = clonePackage(pkg)
		packages = append(packages, pkg)
	}
	sort.Slice(packages, func(i, j int) bool {
		left, right := packages[i], packages[j]
		if left.Dir != right.Dir {
			return left.Dir < right.Dir
		}
		if left.CanonicalPath != right.CanonicalPath {
			return left.CanonicalPath < right.CanonicalPath
		}
		if left.ModulePath != right.ModulePath {
			return left.ModulePath < right.ModulePath
		}
		if left.ModuleID != right.ModuleID {
			return left.ModuleID < right.ModuleID
		}
		return left.Name < right.Name
	})
	return packages, nil
}

func buildEdges(facts []gofacts.Edge, packages []Package) []Edge {
	known := make(map[string]struct{}, min(len(packages), maxPackages))
	for _, pkg := range packages {
		known[pkg.CanonicalPath] = struct{}{}
	}
	edges := make([]Edge, 0, min(len(facts), maxEdges))
	seen := make(map[edgeKey]struct{}, min(len(facts), maxEdges))
	for _, fact := range facts {
		if !validImportIdentity(fact.From) || !validImportIdentity(fact.To) {
			continue
		}
		if _, ok := known[fact.From]; !ok {
			continue
		}
		if _, ok := known[fact.To]; !ok {
			continue
		}
		key := edgeKey{from: fact.From, to: fact.To}
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		edges = append(edges, Edge{FromPackage: fact.From, ToPackage: fact.To})
	}
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].FromPackage != edges[j].FromPackage {
			return edges[i].FromPackage < edges[j].FromPackage
		}
		return edges[i].ToPackage < edges[j].ToPackage
	})
	return edges
}

func validPackageFact(fact gofacts.PackageFact) bool {
	return validImportIdentity(fact.CanonicalPath) &&
		validPackageName(fact.Name) &&
		validOpaqueIdentity(fact.ModuleID) &&
		validImportIdentity(fact.ModulePath) &&
		validDirectory(fact.PackageDir) &&
		validDirectory(fact.ModuleRelativeDir)
}

func packageWithinModule(packageDir, moduleRelativeDir, moduleDir string) bool {
	if moduleDir == "." {
		return moduleRelativeDir == packageDir
	}
	if packageDir == moduleDir {
		return moduleRelativeDir == "."
	}
	prefix := moduleDir + "/"
	return strings.HasPrefix(packageDir, prefix) &&
		moduleRelativeDir == strings.TrimPrefix(packageDir, prefix)
}

func validOpaqueIdentity(value string) bool {
	return validIdentityText(value) &&
		!strings.ContainsRune(value, '/') &&
		!strings.ContainsRune(value, '\\')
}

func validPackageName(value string) bool {
	return validOpaqueIdentity(value)
}

func validImportIdentity(value string) bool {
	if !validIdentityText(value) ||
		strings.HasPrefix(value, "/") ||
		strings.HasSuffix(value, "/") ||
		strings.Contains(value, "//") ||
		strings.ContainsRune(value, '\\') {
		return false
	}
	for _, element := range strings.Split(value, "/") {
		if element == "" || element == "." || element == ".." {
			return false
		}
	}
	return true
}

func validIdentityText(value string) bool {
	if value == "" || len(value) > maxScalarBytes || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) || unicode.IsSpace(character) {
			return false
		}
	}
	return true
}

func validDirectory(value string) bool {
	return value == "." || validFilePath(value)
}

func validFilePath(value string) bool {
	if value == "" ||
		value == "." ||
		len(value) > maxScalarBytes ||
		!utf8.ValidString(value) ||
		!fs.ValidPath(value) ||
		path.Clean(value) != value ||
		strings.ContainsRune(value, '\\') {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func queryScalarsBounded(values ...string) bool {
	total := 0
	for _, value := range values {
		if len(value) > maxScalarBytes || len(value) > maxAggregateScalarBytes-total {
			return false
		}
		total += len(value)
	}
	return true
}

func compactFiles(files []File) []File {
	if len(files) < 2 {
		return files
	}
	write := 1
	for read := 1; read < len(files); read++ {
		if files[read].Path == files[write-1].Path {
			continue
		}
		files[write] = files[read]
		write++
	}
	return files[:write]
}

func clonePackage(pkg Package) Package {
	cloned := pkg
	if pkg.Files != nil {
		cloned.Files = append([]File(nil), pkg.Files...)
	}
	return cloned
}

func packagesEqual(left, right Package) bool {
	if left.CanonicalPath != right.CanonicalPath ||
		left.Name != right.Name ||
		left.ModuleID != right.ModuleID ||
		left.ModulePath != right.ModulePath ||
		left.Dir != right.Dir ||
		left.ModuleRelativeDir != right.ModuleRelativeDir ||
		len(left.Files) != len(right.Files) {
		return false
	}
	for index := range left.Files {
		if left.Files[index] != right.Files[index] {
			return false
		}
	}
	return true
}
