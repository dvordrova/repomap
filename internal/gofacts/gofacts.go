package gofacts

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/reporead"
	"golang.org/x/mod/modfile"
)

const maxGoModBytes = 1024 * 1024

type Facts struct {
	Modules               []ModuleFact           `json:"modules"`
	Packages              []PackageFact          `json:"packages"`
	PackagesCount         int                    `json:"packages_count"`
	EntrypointPackages    []Entrypoint           `json:"entrypoint_packages"`
	CommandTraces         []CommandTrace         `json:"command_traces,omitempty"`
	ModuleSummaries       []ModuleSummary        `json:"module_summaries"`
	OrientationCandidates []OrientationCandidate `json:"orientation_candidates"`
	InternalEdges         []Edge                 `json:"internal_edges"`
	ExternalImportsTop    []ExtImport            `json:"external_imports_top"`
	Warnings              []string               `json:"warnings,omitempty"`
}

type ModuleFact struct {
	ID                 string         `json:"id"`
	ModulePath         string         `json:"module_path"`
	ModuleDir          string         `json:"module_dir"`
	GoMod              string         `json:"go_mod,omitempty"`
	Main               bool           `json:"main"`
	DisplayName        string         `json:"display_name"`
	Replacement        *ModuleSource  `json:"replacement,omitempty"`
	Replacements       []ModuleSource `json:"replacements,omitempty"`
	PackagesCount      int            `json:"packages_count"`
	EntrypointPackages []Entrypoint   `json:"entrypoint_packages"`
	Warnings           []string       `json:"warnings,omitempty"`
}

type ModuleSource struct {
	Path  string `json:"path"`
	Dir   string `json:"dir,omitempty"`
	GoMod string `json:"go_mod,omitempty"`
	Local bool   `json:"local"`
}

type PackageFact struct {
	CanonicalPath     string   `json:"canonical_package_path"`
	Name              string   `json:"name"`
	ModuleID          string   `json:"owning_module_id"`
	ModulePath        string   `json:"module_path"`
	PackageDir        string   `json:"package_directory"`
	ModuleRelativeDir string   `json:"module_relative_path"`
	DisplayPath       string   `json:"display_path"`
	Locality          string   `json:"locality"`
	Files             []string `json:"files,omitempty"`
}

type ModuleSummary struct {
	ModulePath              string      `json:"module_path"`
	ModuleDir               string      `json:"module_dir"`
	PackagesCount           int         `json:"packages_count"`
	EntrypointsCount        int         `json:"entrypoints_count"`
	RoleGuess               string      `json:"role_guess"`
	TopImportedInternalPkgs []string    `json:"top_imported_internal_packages"`
	TopExternalImports      []ExtImport `json:"top_external_imports"`
}

type OrientationCandidate struct {
	Name              string   `json:"name"`
	Kind              string   `json:"kind"`
	EntrypointPackage string   `json:"entrypoint_package"`
	OpenFiles         []string `json:"open_files"`
	Why               string   `json:"why"`
	Priority          int      `json:"priority"`
}

const OrientationKindSignalFlow = "signal_flow"

type Entrypoint struct {
	ModulePath        string             `json:"module_path"`
	ImportPath        string             `json:"import_path"`
	Dir               string             `json:"dir"`
	PackageDir        string             `json:"package_dir"`
	ModuleRelativeDir string             `json:"module_relative_dir"`
	ModuleDir         string             `json:"module_dir"`
	Kind              string             `json:"kind"`
	GoFiles           []string           `json:"go_files"`
	Anchors           []EntrypointAnchor `json:"anchors,omitempty"`
}

type EntrypointAnchorKind string

const (
	EntrypointAnchorVersion                      = 1
	EntrypointAnchorGoMain  EntrypointAnchorKind = "go_main_function"
)

// EntrypointAnchor is a deterministic source declaration selected under the
// active Go build configuration. Path is repository-relative and Line points
// at the declaration name.
type EntrypointAnchor struct {
	Version int                  `json:"version"`
	Kind    EntrypointAnchorKind `json:"kind"`
	Path    string               `json:"path"`
	Line    int                  `json:"line"`
}

type Edge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type ExtImport struct {
	ImportPath  string `json:"import_path"`
	UsedByCount int    `json:"used_by_count"`
}

type goListPackage struct {
	ImportPath      string
	Dir             string
	Name            string
	GoFiles         []string
	CompiledGoFiles []string
	CgoFiles        []string
	CFiles          []string
	CXXFiles        []string
	MFiles          []string
	HFiles          []string
	FFiles          []string
	SFiles          []string
	SwigFiles       []string
	SwigCXXFiles    []string
	SysoFiles       []string
	Imports         []string
	Module          *goListModule
	Error           *goListError
}

type goListModule struct {
	Path    string
	Dir     string
	GoMod   string
	Main    bool
	Replace *goListModule
}

type goListError struct {
	Err string
}

type modulePkgMeta struct {
	moduleDir        string
	modulePath       string
	pkgs             []goListPackage
	entrypointsCount int
}

func DiscoverGoModules(fileList []string, repoPath string) []string {
	seen := make(map[string]struct{})

	for _, f := range fileList {
		if filepath.Base(f) == "go.mod" {
			dir := filepath.Dir(f)
			if dir == "." {
				seen["."] = struct{}{}
			} else {
				seen[dir] = struct{}{}
			}
		}
	}

	if _, ok := seen["."]; !ok {
		if _, err := os.Stat(filepath.Join(repoPath, "go.mod")); err == nil {
			seen["."] = struct{}{}
		}
	}

	result := make([]string, 0, len(seen))
	for d := range seen {
		result = append(result, d)
	}
	sort.Strings(result)
	return result
}

func Load(ctx context.Context, repoPath string, fileList []string, maxPkgs, maxEdges int) (*Facts, error) {
	if maxPkgs <= 0 {
		maxPkgs = 300
	}
	if maxEdges <= 0 {
		maxEdges = 500
	}

	absRepoPath, err := filepath.Abs(repoPath)
	if err != nil {
		absRepoPath = filepath.Clean(repoPath)
	}
	resolvedRepoPath, err := filepath.EvalSymlinks(absRepoPath)
	if err != nil {
		resolvedRepoPath = absRepoPath
	}

	moduleDirs := DiscoverGoModules(fileList, resolvedRepoPath)
	if len(moduleDirs) == 0 {
		return &Facts{}, nil
	}
	repoReader, err := reporead.New(resolvedRepoPath)
	if err != nil {
		return nil, fmt.Errorf("prepare bounded go.mod reads: %w", err)
	}
	defer repoReader.Close()

	var allPkgs []goListPackage
	var packageFacts []PackageFact
	var allEntrypoints []Entrypoint
	var allCommandTraces []CommandTrace
	var topWarnings []string
	modules := make([]ModuleFact, 0, len(moduleDirs))
	totalPkgs := 0

	modMetas := make([]modulePkgMeta, 0, len(moduleDirs))

	for _, modRelDir := range moduleDirs {
		if err := verifyModuleGoMod(repoReader, modRelDir); err != nil {
			warning := fmt.Sprintf("unsafe go.mod; skipping go list: %v", err)
			topWarnings = append(topWarnings, fmt.Sprintf("module %s: %s", modRelDir, warning))
			modules = append(modules, ModuleFact{
				ModulePath: modRelDir,
				ModuleDir:  modRelDir,
				Warnings:   []string{warning},
			})
			continue
		}

		absDir := resolvedRepoPath
		if modRelDir != "." {
			absDir = filepath.Join(resolvedRepoPath, modRelDir)
		}

		moduleInfo, err := runGoModule(ctx, absDir)
		if err != nil {
			topWarnings = append(topWarnings, fmt.Sprintf("module %s: go list module failed: %v", modRelDir, err))
			modules = append(modules, ModuleFact{
				ID: moduleFactID(modRelDir, modRelDir), ModulePath: modRelDir, ModuleDir: modRelDir,
				DisplayName: moduleDisplayName(modRelDir, modRelDir),
				Warnings:    []string{fmt.Sprintf("go list module failed: %v", err)},
			})
			continue
		}
		modulePath := moduleInfo.Path
		moduleID := moduleFactID(modulePath, modRelDir)
		pkgs, modWarnings, err := runGoList(ctx, absDir)
		if err != nil {
			topWarnings = append(topWarnings, fmt.Sprintf("module %s: go list failed: %v", modRelDir, err))
			modules = append(modules, ModuleFact{
				ID: moduleID, ModulePath: modulePath,
				ModuleDir:   modRelDir,
				DisplayName: moduleDisplayName(modulePath, modRelDir), Main: moduleInfo.Main,
				Warnings: []string{fmt.Sprintf("go list failed: %v", err)},
			})
			continue
		}

		remaining := maxPkgs - totalPkgs
		if remaining <= 0 {
			topWarnings = append(topWarnings, fmt.Sprintf("reached max packages (%d); skipping module %s", maxPkgs, modulePath))
			break
		}
		if len(pkgs) > remaining {
			topWarnings = append(topWarnings, fmt.Sprintf("truncated packages for module %s: kept %d of %d (max %d total)", modulePath, remaining, len(pkgs), maxPkgs))
			pkgs = pkgs[:remaining]
		}

		totalPkgs += len(pkgs)
		allPkgs = append(allPkgs, pkgs...)
		for _, pkg := range pkgs {
			_, moduleRelativeDir, packageDir, normalizeErr := normalizePackagePaths(resolvedRepoPath, absDir, pkg.Dir)
			if normalizeErr != nil || pkg.ImportPath == "" {
				continue
			}
			displayPath := filepath.ToSlash(moduleRelativeDir)
			if displayPath == "." || displayPath == "" {
				displayPath = pkg.Name
			}
			packageFacts = append(packageFacts, PackageFact{
				CanonicalPath: pkg.ImportPath, Name: pkg.Name, ModuleID: moduleID, ModulePath: modulePath,
				PackageDir: filepath.ToSlash(packageDir), ModuleRelativeDir: filepath.ToSlash(moduleRelativeDir),
				DisplayPath: displayPath, Locality: "local", Files: packageInputFiles(packageDir, pkg),
			})
		}

		entrypointCandidates := buildEntrypointCandidates(pkgs, resolvedRepoPath, absDir, modRelDir, modulePath)
		modEntrypoints, entrypointWarnings := resolveMainEntrypoints(repoReader, entrypointCandidates)
		if len(entrypointWarnings) > 0 {
			modWarnings = append(modWarnings, entrypointWarnings...)
			for _, warning := range entrypointWarnings {
				topWarnings = append(topWarnings, fmt.Sprintf("module %s: %s", modRelDir, warning))
			}
		}
		allEntrypoints = append(allEntrypoints, modEntrypoints...)
		commandTraces, commandTraceWarnings := buildCommandTraces(repoReader, modEntrypoints)
		allCommandTraces = append(allCommandTraces, commandTraces...)
		if len(commandTraceWarnings) > 0 {
			modWarnings = append(modWarnings, commandTraceWarnings...)
			for _, warning := range commandTraceWarnings {
				topWarnings = append(topWarnings, fmt.Sprintf("module %s: %s", modRelDir, warning))
			}
		}

		modules = append(modules, ModuleFact{
			ID:                 moduleID,
			ModulePath:         modulePath,
			ModuleDir:          modRelDir,
			GoMod:              repositoryRelativeMetadataPath(resolvedRepoPath, moduleInfo.GoMod),
			Main:               moduleInfo.Main,
			DisplayName:        moduleDisplayName(modulePath, modRelDir),
			Replacement:        moduleSource(resolvedRepoPath, moduleInfo.Replace),
			Replacements:       readModuleReplacements(repoReader, resolvedRepoPath, modRelDir),
			PackagesCount:      len(pkgs),
			EntrypointPackages: modEntrypoints,
			Warnings:           modWarnings,
		})

		modMetas = append(modMetas, modulePkgMeta{
			moduleDir:        modRelDir,
			modulePath:       modulePath,
			pkgs:             pkgs,
			entrypointsCount: len(modEntrypoints),
		})
	}

	known := buildKnownSet(allPkgs)

	edges := buildInternalEdges(allPkgs, known)
	if len(edges) > maxEdges {
		topWarnings = append(topWarnings, fmt.Sprintf("truncated internal edges: kept %d of %d (max %d)", maxEdges, len(edges), maxEdges))
		edges = edges[:maxEdges]
	}

	extImports := buildExternalImports(allPkgs, known)
	sort.Slice(packageFacts, func(i, j int) bool { return packageFacts[i].PackageDir < packageFacts[j].PackageDir })
	moduleSummaries := buildModuleSummaries(modMetas, known)
	orientationCandidates := buildOrientationCandidates(allEntrypoints)

	return &Facts{
		Modules:               modules,
		Packages:              packageFacts,
		PackagesCount:         totalPkgs,
		EntrypointPackages:    allEntrypoints,
		CommandTraces:         allCommandTraces,
		ModuleSummaries:       moduleSummaries,
		OrientationCandidates: orientationCandidates,
		InternalEdges:         edges,
		ExternalImportsTop:    extImports,
		Warnings:              topWarnings,
	}, nil
}

func packageInputFiles(packageDir string, pkg goListPackage) []string {
	fileNames := make([]string, 0, len(pkg.GoFiles)+len(pkg.CgoFiles)+len(pkg.CFiles)+len(pkg.HFiles)+len(pkg.SFiles))
	fileNames = append(fileNames, pkg.GoFiles...)
	fileNames = append(fileNames, pkg.CgoFiles...)
	fileNames = append(fileNames, pkg.CFiles...)
	fileNames = append(fileNames, pkg.CXXFiles...)
	fileNames = append(fileNames, pkg.MFiles...)
	fileNames = append(fileNames, pkg.HFiles...)
	fileNames = append(fileNames, pkg.FFiles...)
	fileNames = append(fileNames, pkg.SFiles...)
	fileNames = append(fileNames, pkg.SwigFiles...)
	fileNames = append(fileNames, pkg.SwigCXXFiles...)
	fileNames = append(fileNames, pkg.SysoFiles...)
	result := make([]string, 0, len(fileNames))
	for _, name := range fileNames {
		name = filepath.ToSlash(filepath.Clean(name))
		if name == "." || name == ".." || strings.HasPrefix(name, "../") || strings.Contains(name, "/") {
			continue
		}
		if packageDir == "." || packageDir == "" {
			result = append(result, name)
		} else {
			result = append(result, filepath.ToSlash(filepath.Join(packageDir, name)))
		}
	}
	sort.Strings(result)
	return compactStrings(result)
}

func compactStrings(values []string) []string {
	result := values[:0]
	for _, value := range values {
		if len(result) == 0 || result[len(result)-1] != value {
			result = append(result, value)
		}
	}
	return result
}

func readModuleReplacements(reader *reporead.Reader, repoRoot, moduleDir string) []ModuleSource {
	goModPath := "go.mod"
	if moduleDir != "." {
		goModPath = filepath.ToSlash(filepath.Join(moduleDir, goModPath))
	}
	content, err := reader.ReadFile(goModPath, maxGoModBytes)
	if err != nil || content.Truncated {
		return nil
	}
	parsed, err := modfile.Parse(goModPath, content.Bytes, nil)
	if err != nil {
		return nil
	}
	result := make([]ModuleSource, 0, len(parsed.Replace))
	moduleRoot := repoRoot
	if moduleDir != "." {
		moduleRoot = filepath.Join(repoRoot, filepath.FromSlash(moduleDir))
	}
	for _, replacement := range parsed.Replace {
		source := ModuleSource{Path: replacement.Old.Path}
		if replacement.New.Version == "" && replacement.New.Path != "" {
			target := replacement.New.Path
			if !filepath.IsAbs(target) {
				target = filepath.Join(moduleRoot, filepath.FromSlash(target))
			}
			if relative := repositoryRelativeMetadataPath(repoRoot, target); relative != "" {
				source.Dir = relative
				source.GoMod = filepath.ToSlash(filepath.Join(relative, "go.mod"))
				source.Local = true
			}
		}
		result = append(result, source)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Path < result[j].Path })
	return result
}

func runGoModule(ctx context.Context, repoDir string) (goListModule, error) {
	cmd := exec.CommandContext(ctx, "go", "list", "-m", "-json")
	cmd.Dir = repoDir
	cmd.Env = append(os.Environ(), "GOWORK=off")
	output, err := cmd.Output()
	if err != nil {
		return goListModule{}, fmt.Errorf("go list -m: %w", err)
	}
	var module goListModule
	if err := json.Unmarshal(output, &module); err != nil {
		return goListModule{}, fmt.Errorf("decode go list module: %w", err)
	}
	if module.Path == "" {
		return goListModule{}, fmt.Errorf("go list module has no declared path")
	}
	return module, nil
}

func moduleFactID(modulePath, moduleDir string) string {
	digest := sha256.Sum256([]byte(modulePath + "\x00" + filepath.ToSlash(moduleDir)))
	return fmt.Sprintf("module-%x", digest[:12])
}

func moduleDisplayName(_ string, moduleDir string) string {
	if moduleDir != "" && moduleDir != "." {
		return filepath.ToSlash(moduleDir)
	}
	return "."
}

func repositoryRelativeMetadataPath(repoRoot, value string) string {
	if value == "" {
		return ""
	}
	relative, err := filepath.Rel(repoRoot, value)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return ""
	}
	return filepath.ToSlash(relative)
}

func moduleSource(repoRoot string, module *goListModule) *ModuleSource {
	if module == nil {
		return nil
	}
	dir := repositoryRelativeMetadataPath(repoRoot, module.Dir)
	goMod := repositoryRelativeMetadataPath(repoRoot, module.GoMod)
	return &ModuleSource{Path: module.Path, Dir: dir, GoMod: goMod, Local: dir != ""}
}

func verifyModuleGoMod(reader *reporead.Reader, moduleDir string) error {
	goModPath := "go.mod"
	if moduleDir != "." {
		goModPath = filepath.Join(moduleDir, goModPath)
	}

	content, err := reader.ReadFile(goModPath, maxGoModBytes)
	if err != nil {
		return fmt.Errorf("verify %q: %w", goModPath, err)
	}
	if content.Truncated {
		return fmt.Errorf("verify %q: file exceeds %d-byte limit", goModPath, maxGoModBytes)
	}
	return nil
}

func runGoList(ctx context.Context, repoDir string) ([]goListPackage, []string, error) {
	cmd := exec.CommandContext(ctx, "go", "list", "-e", "-json", "./...")
	cmd.Dir = repoDir
	cmd.Env = append(os.Environ(), "GOWORK=off")

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, nil, fmt.Errorf("go list failed: %s: %w", strings.TrimSpace(stderr.String()), err)
	}

	return parseGoListOutput(&stdout)
}

func parseGoListOutput(r io.Reader) ([]goListPackage, []string, error) {
	dec := json.NewDecoder(r)
	var pkgs []goListPackage
	var warnings []string

	for {
		var pkg goListPackage
		if err := dec.Decode(&pkg); err == io.EOF {
			break
		} else if err != nil {
			return nil, nil, fmt.Errorf("decode package %d: %w", len(pkgs), err)
		}
		if pkg.Error != nil {
			warnings = append(warnings, fmt.Sprintf("package %s: %s", pkg.ImportPath, pkg.Error.Err))
		}
		pkgs = append(pkgs, pkg)
	}

	return pkgs, warnings, nil
}

func extractModulePath(pkgs []goListPackage) string {
	for _, p := range pkgs {
		if p.Error != nil {
			continue
		}
		if p.Module != nil && p.Module.Path != "" {
			return p.Module.Path
		}
	}
	return ""
}

func normalizePackagePaths(repoRoot, moduleRoot, pkgDir string) (moduleDir string, moduleRelativeDir string, packageDir string, err error) {
	repoRoot = filepath.Clean(repoRoot)
	moduleRoot = filepath.Clean(moduleRoot)
	pkgDir = filepath.Clean(pkgDir)

	packageDir, err = filepath.Rel(repoRoot, pkgDir)
	if err != nil {
		repoRoot, _ = filepath.EvalSymlinks(repoRoot)
		pkgDir, _ = filepath.EvalSymlinks(pkgDir)
		packageDir, err = filepath.Rel(repoRoot, pkgDir)
		if err != nil {
			return "", "", "", fmt.Errorf("package dir %q not under repo root %q: %w", pkgDir, repoRoot, err)
		}
	}
	if packageDir == "" {
		packageDir = "."
	}
	if packageDir == ".." || strings.HasPrefix(packageDir, ".."+string(filepath.Separator)) {
		return "", "", "", fmt.Errorf("package dir %q is outside repo root %q", pkgDir, repoRoot)
	}

	moduleDir, err = filepath.Rel(repoRoot, moduleRoot)
	if err != nil {
		repoRoot, _ = filepath.EvalSymlinks(repoRoot)
		moduleRoot, _ = filepath.EvalSymlinks(moduleRoot)
		moduleDir, err = filepath.Rel(repoRoot, moduleRoot)
		if err != nil {
			return "", "", "", fmt.Errorf("module root %q not under repo root %q: %w", moduleRoot, repoRoot, err)
		}
	}
	if moduleDir == "" {
		moduleDir = "."
	}

	moduleRelativeDir, err = filepath.Rel(moduleRoot, pkgDir)
	if err != nil {
		moduleRoot, _ = filepath.EvalSymlinks(moduleRoot)
		pkgDir, _ = filepath.EvalSymlinks(pkgDir)
		moduleRelativeDir, err = filepath.Rel(moduleRoot, pkgDir)
		if err != nil {
			return "", "", "", fmt.Errorf("package dir %q not under module root %q: %w", pkgDir, moduleRoot, err)
		}
	}
	if moduleRelativeDir == "" {
		moduleRelativeDir = "."
	}
	if moduleRelativeDir == ".." || strings.HasPrefix(moduleRelativeDir, ".."+string(filepath.Separator)) {
		return "", "", "", fmt.Errorf("package dir %q is outside module root %q", pkgDir, moduleRoot)
	}

	return moduleDir, moduleRelativeDir, packageDir, nil
}

func buildKnownSet(pkgs []goListPackage) map[string]struct{} {
	known := make(map[string]struct{}, len(pkgs))
	for _, p := range pkgs {
		known[p.ImportPath] = struct{}{}
	}
	return known
}

func buildEntrypointCandidates(pkgs []goListPackage, repoRoot string, moduleRoot string, modRelDir string, modulePath string) []Entrypoint {
	eps := make([]Entrypoint, 0)
	for _, p := range pkgs {
		if p.Name != "main" || p.Error != nil {
			continue
		}

		md, mrd, pd, err := normalizePackagePaths(repoRoot, moduleRoot, p.Dir)
		if err != nil {
			md = modRelDir
			pd = filepath.Base(p.Dir)
			mrd = pd
		}

		kind := classifyEntrypoint(p.ImportPath, pd, md)

		eps = append(eps, Entrypoint{
			ModulePath:        modulePath,
			ImportPath:        p.ImportPath,
			Dir:               pd,
			PackageDir:        pd,
			ModuleRelativeDir: mrd,
			ModuleDir:         md,
			Kind:              kind,
			GoFiles:           p.GoFiles,
		})
	}
	return eps
}

func classifyEntrypoint(importPath string, packageDir string, moduleDir string) string {
	lower := strings.ToLower(importPath)
	lowerPkgDir := strings.ToLower(packageDir)

	if moduleDir == "server" || packageDir == "server" || strings.HasSuffix(lower, "/server/v3") {
		return "primary_binary"
	}
	if moduleDir == "etcdctl" || packageDir == "etcdctl" || strings.Contains(lowerPkgDir, "ctl") {
		return "cli"
	}
	if moduleDir == "etcdutl" || packageDir == "etcdutl" {
		return "tool"
	}
	if strings.HasPrefix(lowerPkgDir, "tools/") || strings.HasPrefix(moduleDir, "tools/") {
		return "tool"
	}
	if strings.HasPrefix(lowerPkgDir, "contrib/") || strings.Contains(lowerPkgDir, "example") {
		return "example"
	}
	if strings.HasPrefix(lowerPkgDir, "tests/") || strings.Contains(lowerPkgDir, "/test-template/") {
		return "test_binary"
	}
	return "unknown"
}

func buildInternalEdges(pkgs []goListPackage, known map[string]struct{}) []Edge {
	edges := make([]Edge, 0)
	for _, p := range pkgs {
		if p.Error != nil {
			continue
		}
		for _, imp := range p.Imports {
			if _, ok := known[imp]; ok {
				edges = append(edges, Edge{From: p.ImportPath, To: imp})
			}
		}
	}
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From == edges[j].From {
			return edges[i].To < edges[j].To
		}
		return edges[i].From < edges[j].From
	})
	return edges
}

func buildExternalImports(pkgs []goListPackage, known map[string]struct{}) []ExtImport {
	extCount := make(map[string]int)
	for _, p := range pkgs {
		if p.Error != nil {
			continue
		}
		seen := make(map[string]struct{})
		for _, imp := range p.Imports {
			if _, isInternal := known[imp]; isInternal {
				continue
			}
			if _, counted := seen[imp]; counted {
				continue
			}
			seen[imp] = struct{}{}
			extCount[imp]++
		}
	}

	extImports := make([]ExtImport, 0, len(extCount))
	for imp, count := range extCount {
		extImports = append(extImports, ExtImport{ImportPath: imp, UsedByCount: count})
	}
	sort.Slice(extImports, func(i, j int) bool {
		if extImports[i].UsedByCount == extImports[j].UsedByCount {
			return extImports[i].ImportPath < extImports[j].ImportPath
		}
		return extImports[i].UsedByCount > extImports[j].UsedByCount
	})
	if len(extImports) > 50 {
		extImports = extImports[:50]
	}
	return extImports
}

func guessModuleRole(moduleDir string) string {
	if moduleDir == "server" {
		return "server_runtime"
	}
	if strings.Contains(moduleDir, "client") {
		return "client_library"
	}
	if moduleDir == "api" {
		return "api_definitions"
	}
	if moduleDir == "tests" {
		return "tests"
	}
	if strings.HasPrefix(moduleDir, "tools") {
		return "tools"
	}
	if moduleDir == "pkg" {
		return "shared_library"
	}
	if moduleDir == "." {
		return "repository_root"
	}
	return "unknown"
}

func buildModuleSummaries(modMetas []modulePkgMeta, known map[string]struct{}) []ModuleSummary {
	summaries := make([]ModuleSummary, 0, len(modMetas))

	for _, mm := range modMetas {
		internalImportCount := make(map[string]int)
		for _, p := range mm.pkgs {
			if p.Error != nil {
				continue
			}
			for _, imp := range p.Imports {
				if _, ok := known[imp]; ok {
					internalImportCount[imp]++
				}
			}
		}
		type importCount struct {
			path  string
			count int
		}
		var ranked []importCount
		for path, count := range internalImportCount {
			ranked = append(ranked, importCount{path, count})
		}
		sort.Slice(ranked, func(i, j int) bool {
			if ranked[i].count == ranked[j].count {
				return ranked[i].path < ranked[j].path
			}
			return ranked[i].count > ranked[j].count
		})
		topInternal := make([]string, 0, 10)
		for i, ic := range ranked {
			if i >= 10 {
				break
			}
			topInternal = append(topInternal, ic.path)
		}

		extCount := make(map[string]int)
		for _, p := range mm.pkgs {
			if p.Error != nil {
				continue
			}
			seen := make(map[string]struct{})
			for _, imp := range p.Imports {
				if _, isInternal := known[imp]; isInternal {
					continue
				}
				if _, counted := seen[imp]; counted {
					continue
				}
				seen[imp] = struct{}{}
				extCount[imp]++
			}
		}
		var modExtImports []ExtImport
		for imp, count := range extCount {
			modExtImports = append(modExtImports, ExtImport{ImportPath: imp, UsedByCount: count})
		}
		sort.Slice(modExtImports, func(i, j int) bool {
			if modExtImports[i].UsedByCount == modExtImports[j].UsedByCount {
				return modExtImports[i].ImportPath < modExtImports[j].ImportPath
			}
			return modExtImports[i].UsedByCount > modExtImports[j].UsedByCount
		})
		if len(modExtImports) > 10 {
			modExtImports = modExtImports[:10]
		}

		summaries = append(summaries, ModuleSummary{
			ModulePath:              mm.modulePath,
			ModuleDir:               mm.moduleDir,
			PackagesCount:           len(mm.pkgs),
			EntrypointsCount:        mm.entrypointsCount,
			RoleGuess:               guessModuleRole(mm.moduleDir),
			TopImportedInternalPkgs: topInternal,
			TopExternalImports:      modExtImports,
		})
	}

	return summaries
}

func buildOrientationCandidates(entrypoints []Entrypoint) []OrientationCandidate {
	candidates := make([]OrientationCandidate, 0, len(entrypoints))

	for _, ep := range entrypoints {
		kind := ep.Kind
		priority := priorityForKind(kind)

		openFiles := make([]string, 0, len(ep.Anchors)+len(ep.GoFiles))
		seenOpenFiles := make(map[string]struct{}, cap(openFiles))
		for _, anchor := range ep.Anchors {
			if _, seen := seenOpenFiles[anchor.Path]; seen {
				continue
			}
			seenOpenFiles[anchor.Path] = struct{}{}
			openFiles = append(openFiles, anchor.Path)
		}
		for _, gf := range ep.GoFiles {
			var openFile string
			if ep.PackageDir == "." || ep.PackageDir == "" {
				openFile = filepath.ToSlash(gf)
			} else {
				openFile = filepath.ToSlash(ep.PackageDir) + "/" + filepath.ToSlash(gf)
			}
			if _, seen := seenOpenFiles[openFile]; seen {
				continue
			}
			seenOpenFiles[openFile] = struct{}{}
			openFiles = append(openFiles, openFile)
		}

		why := whyForKind(kind)

		candidates = append(candidates, OrientationCandidate{
			Name:              ep.ImportPath,
			Kind:              kind,
			EntrypointPackage: ep.ImportPath,
			OpenFiles:         openFiles,
			Why:               why,
			Priority:          priority,
		})
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Priority > candidates[j].Priority
	})

	if len(candidates) > 20 {
		candidates = candidates[:20]
	}

	return candidates
}

func priorityForKind(kind string) int {
	switch kind {
	case "primary_binary":
		return 5
	case "cli":
		return 4
	case "tool":
		return 3
	case "example":
		return 2
	case OrientationKindSignalFlow:
		return 2
	case "test_binary":
		return 1
	default:
		return 0
	}
}

func whyForKind(kind string) string {
	switch kind {
	case "primary_binary":
		return "main server binary; likely the primary runtime entrypoint"
	case "cli":
		return "command-line interface; likely the main user-facing tool"
	case "tool":
		return "utility tool; used for maintenance or data operations"
	case "example":
		return "example code; good for understanding usage patterns"
	case "test_binary":
		return "test binary; useful for understanding expected behaviour"
	case OrientationKindSignalFlow:
		return "operational flow discovered from source signals"
	default:
		return "entrypoint of unknown role"
	}
}
