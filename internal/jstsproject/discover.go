package jstsproject

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/corpus"
)

const (
	maxManifestBytes         = 4 << 20
	maxHelperOutput          = 64 << 20
	maxHelperStderrBytes     = 8 << 10
	maxHelperDiagnosticBytes = 2_048
)

//go:embed helper.mjs
var nodeHelper string

var ErrTypeScriptCompilerUnavailable = errors.New("jsts project: prepared TypeScript compiler is unavailable")

type packageManifest struct {
	Name                 string            `json:"name"`
	PackageManager       string            `json:"packageManager"`
	Workspaces           json.RawMessage   `json:"workspaces"`
	Main                 string            `json:"main"`
	Module               string            `json:"module"`
	Source               string            `json:"source"`
	Types                string            `json:"types"`
	Typings              string            `json:"typings"`
	Browser              json.RawMessage   `json:"browser"`
	Bin                  json.RawMessage   `json:"bin"`
	Exports              json.RawMessage   `json:"exports"`
	Scripts              map[string]string `json:"scripts"`
	Dependencies         map[string]string `json:"dependencies"`
	DevDependencies      map[string]string `json:"devDependencies"`
	OptionalDependencies map[string]string `json:"optionalDependencies"`
}

type helperRequest struct {
	Version             int                     `json:"version"`
	ProjectDir          string                  `json:"project_dir,omitempty"`
	ConfigPath          string                  `json:"config_path,omitempty"`
	CompilerPackages    []helperCompilerPackage `json:"compiler_packages"`
	PackageBoundaryDirs []string                `json:"package_boundary_dirs"`
	Files               []helperFile            `json:"files"`
	PathAliasPrefixes   []string                `json:"path_alias_prefixes"`
	AdditionalFiles     []string                `json:"additional_files"`
}

type helperCompilerPackage struct {
	Name           string `json:"name"`
	ResolutionBase string `json:"resolution_base"`
}

const (
	helperCompilerResolutionProject        = "project"
	helperCompilerResolutionRepositoryRoot = "repository_root"
)

type helperFile struct {
	Path    string `json:"path"`
	FileRef string `json:"file_ref"`
}

type helperOutput struct {
	HelperVersion    int           `json:"helper_version"`
	SourceSHA256     string        `json:"source_sha256"`
	ModuleResolution string        `json:"module_resolution"`
	BaseURL          string        `json:"base_url"`
	PathAliases      []PathAlias   `json:"path_aliases"`
	Files            []File        `json:"files"`
	Declarations     []Declaration `json:"declarations"`
	Imports          []Import      `json:"imports"`
	Exports          []Export      `json:"exports"`
	Calls            []Call        `json:"calls"`
	Surfaces         []Surface     `json:"surfaces"`
	Routes           []Route       `json:"routes"`
	HTTPUses         []HTTPUse     `json:"http_uses"`
	Contracts        []Contract    `json:"contracts"`
	Resources        []Resource    `json:"resources"`
}

// Discover is the compatibility exact-one compiler path. Ordinary repository
// discovery catalogs every package with ScoutTargets, then calls
// DiscoverSelected with each exact selector. It never installs packages: the
// selected manifest, or its repository-root fallback, must declare a compiler
// already prepared in repository-local node_modules.
func Discover(ctx context.Context, repository *corpus.Corpus, root string) (Result, error) {
	return DiscoverSelected(ctx, repository, root, "")
}

// DiscoverSelected analyzes the exact owned package named by a
// jsts:<manifest> selector. An empty selector retains the compatibility
// exact-one behavior. Exact selection happens before compiler execution, so a
// workspace coordinator is never compiled merely to validate a nested package
// override.
func DiscoverSelected(ctx context.Context, repository *corpus.Corpus, root, selector string) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if repository == nil {
		return Result{}, fmt.Errorf("jsts project: corpus is required")
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil || strings.TrimSpace(root) == "" {
		return Result{}, fmt.Errorf("jsts project: resolve repository root")
	}
	info, err := os.Stat(absoluteRoot)
	if err != nil || !info.IsDir() {
		return Result{}, fmt.Errorf("jsts project: repository root is not a directory")
	}

	entries := repository.Entries()
	for _, entry := range entries {
		if corpus.ForbiddenPath(entry.Path) {
			return Result{}, fmt.Errorf("jsts project: forbidden corpus path %q", entry.Path)
		}
	}
	selector = strings.TrimSpace(selector)
	var rootManifest *packageManifest
	if selector == "" {
		if _, ok := repository.ID("package.json"); ok {
			manifest, manifestErr := readPackageManifest(repository, "package.json")
			if manifestErr != nil {
				return Result{}, manifestErr
			}
			rootManifest = &manifest
		}
	}
	manifestPath, projectDir, err := selectProjectManifest(entries, selector, rootManifest)
	if err != nil {
		return Result{}, err
	}
	manifestID, ok := repository.ID(manifestPath)
	if !ok {
		return Result{}, fmt.Errorf("jsts project: selected package manifest is unavailable")
	}
	var manifest packageManifest
	if manifestPath == "package.json" && rootManifest != nil {
		manifest = *rootManifest
	} else {
		manifest, err = readPackageManifest(repository, manifestPath)
		if err != nil {
			return Result{}, err
		}
	}

	projectEntries := make([]corpus.Entry, 0, len(entries))
	nestedPackageDirs := make([]string, 0)
	for _, entry := range entries {
		relativePath, withinProject := projectRelativePath(projectDir, entry.Path)
		if !withinProject {
			continue
		}
		projectEntry := entry
		projectEntry.Path = relativePath
		projectEntries = append(projectEntries, projectEntry)
		if relativePath != "package.json" && path.Base(relativePath) == "package.json" {
			nestedPackageDirs = append(nestedPackageDirs, path.Dir(relativePath))
		}
	}
	sort.Strings(nestedPackageDirs)
	isNestedPackageFile := func(filePath string) bool {
		for _, directory := range nestedPackageDirs {
			if strings.HasPrefix(filePath, directory+"/") {
				return true
			}
		}
		return false
	}
	ownedFileRefs := make(map[string]string, len(projectEntries))
	for _, entry := range projectEntries {
		if !isNestedPackageFile(entry.Path) {
			ownedFileRefs[entry.Path] = string(entry.ID)
		}
	}

	compilerPackages := typeScriptCompilerPackagesForProject(manifest, nil)
	if len(compilerPackages) == 0 && projectDir != "." {
		if _, ok := repository.ID("package.json"); ok {
			rootCompilerManifest, rootManifestErr := readPackageManifest(repository, "package.json")
			if rootManifestErr != nil {
				return Result{}, rootManifestErr
			}
			compilerPackages = typeScriptCompilerPackagesForProject(manifest, &rootCompilerManifest)
		}
	}
	if len(compilerPackages) == 0 {
		return Result{}, fmt.Errorf(
			"%w: selected manifest %q and its repository root declare neither the typescript package nor an npm alias to it",
			ErrTypeScriptCompilerUnavailable, manifestPath,
		)
	}
	request := newHelperRequest(compilerPackages, nestedPackageDirs)
	if projectDir != "." {
		request.ProjectDir = projectDir
	}
	for _, entry := range projectEntries {
		if isNestedPackageFile(entry.Path) || !sourceExtension(entry.Path) {
			continue
		}
		request.Files = append(request.Files, helperFile{Path: entry.Path, FileRef: string(entry.ID)})
	}
	if len(request.Files) == 0 {
		return Result{}, fmt.Errorf("jsts project: selected package has no tracked JavaScript or TypeScript source files")
	}
	if projectFileRef(repository, projectDir, "tsconfig.json") != "" {
		request.ConfigPath = "tsconfig.json"
	} else if projectFileRef(repository, projectDir, "jsconfig.json") != "" {
		request.ConfigPath = "jsconfig.json"
	}
	toolConfigs := []ProjectFile{}
	for _, entry := range projectEntries {
		base := path.Base(entry.Path)
		if path.Dir(entry.Path) == "." && (strings.HasPrefix(base, "vite.config.") || strings.HasPrefix(base, "vitest.config.") || strings.HasPrefix(base, "drizzle.config.") || strings.HasPrefix(base, "tailwind.config.") || strings.HasPrefix(base, "eslint.config.") || strings.HasPrefix(base, "postcss.config.")) {
			toolConfigs = append(toolConfigs, ProjectFile{Path: repositoryProjectPath(projectDir, entry.Path), FileRef: string(entry.ID)})
			if sourceExtension(entry.Path) {
				request.AdditionalFiles = append(request.AdditionalFiles, entry.Path)
			}
		}
	}
	for _, command := range manifest.Scripts {
		for _, candidate := range scriptEntryPaths(command) {
			if _, owned := ownedFileRefs[candidate]; owned && sourceExtension(candidate) {
				request.AdditionalFiles = append(request.AdditionalFiles, candidate)
			}
		}
	}
	request.AdditionalFiles = canonicalStrings(request.AdditionalFiles)

	output, err := invokeHelper(ctx, absoluteRoot, request)
	if err != nil {
		return Result{}, err
	}
	if output.HelperVersion != HelperVersion {
		return Result{}, fmt.Errorf("jsts project: helper contract mismatch")
	}
	if len(output.Files) == 0 {
		return Result{}, fmt.Errorf("jsts project: configuration selects no tracked source files")
	}
	sort.Slice(output.Files, func(i, j int) bool { return output.Files[i].Path < output.Files[j].Path })
	language := "javascript"
	for _, file := range output.Files {
		if file.Language == "typescript" {
			language = "typescript"
			break
		}
	}
	packageManager, lockfilePath := packageManagerFacts(manifest.PackageManager, projectEntries)
	projectName := selectedPackageIdentityName(repository, projectDir, manifest.Name)
	selectedSelector := "jsts:" + manifestPath
	scriptFacts := buildScriptFacts(manifest.Scripts, output.Files)
	// npm's string-form bin command is derived specifically from
	// package.json#name. A lockfile/display fallback is project identity, not
	// package-binary authority.
	packageBinaries := packageBinaryFacts(manifest.Name, manifest.Bin, projectDir, ownedFileRefs)
	projectSourceRoots := sourceRoots(output.Files)
	rebaseHelperOutput(projectDir, &output)
	sourceSHA256 := sourceDigest(output.Files)
	pathAliases := rebasePathAliases(projectDir, output.PathAliases)
	baseURL := repositoryProjectPath(projectDir, output.BaseURL)
	sourceRoots := make([]string, 0, len(projectSourceRoots))
	for _, sourceRoot := range projectSourceRoots {
		sourceRoots = append(sourceRoots, repositoryProjectPath(projectDir, sourceRoot))
	}
	lockfilePath = repositoryProjectPath(projectDir, lockfilePath)
	configPath := repositoryProjectPath(projectDir, request.ConfigPath)
	projectRef := "project:root-package"
	if projectDir != "." {
		projectRef = "project:package:" + string(manifestID)
	}
	project := Project{
		Ref: projectRef, Name: projectName, PackagePath: projectName,
		Language: language, Selector: selectedSelector, ManifestPath: manifestPath, ManifestFileRef: string(manifestID),
		ConfigPath: configPath, ConfigFileRef: fileRef(repository, configPath), PackageManager: packageManager, LockfilePath: lockfilePath, LockfileFileRef: fileRef(repository, lockfilePath),
		ModuleResolution: output.ModuleResolution, BaseURL: baseURL, PathAliases: pathAliases,
		Scripts: scriptFacts, Binaries: packageBinaries, SourceRoots: sourceRoots, EntryFileRefs: []string{}, ToolConfigs: toolConfigs, Dependencies: manifestDependencyFacts(manifest),
	}
	result := Result{
		Version: Version, HelperVersion: HelperVersion, CorpusSHA256: repository.SHA256(), SourceSHA256: sourceSHA256, Project: project,
		Files: output.Files, Declarations: output.Declarations, Imports: output.Imports, Exports: output.Exports,
		Calls: output.Calls, Surfaces: output.Surfaces, Routes: output.Routes, HTTPUses: output.HTTPUses,
		Contracts: output.Contracts, Resources: output.Resources, ProductPaths: []ProductPath{},
	}
	addScriptSurfaces(&result)
	addPackageBinarySurfaces(&result)
	for _, surface := range result.Surfaces {
		for _, ref := range surface.EntryRefs {
			if strings.HasPrefix(ref, "module:") {
				result.Project.EntryFileRefs = append(result.Project.EntryFileRefs, strings.TrimPrefix(ref, "module:"))
			}
		}
	}
	for _, script := range result.Project.Scripts {
		result.Project.EntryFileRefs = append(result.Project.EntryFileRefs, script.EntryFileRefs...)
	}
	result.ProductPaths = buildProductPaths(result)
	if err := bindProgramTargetIdentity(&result); err != nil {
		return Result{}, err
	}
	sealed, err := Seal(result)
	if err != nil {
		return Result{}, err
	}
	return sealed, nil
}

func newHelperRequest(
	compilerPackages []helperCompilerPackage,
	packageBoundaryDirs []string,
) helperRequest {
	return helperRequest{
		Version:             HelperVersion,
		CompilerPackages:    append([]helperCompilerPackage{}, compilerPackages...),
		PackageBoundaryDirs: append([]string{}, packageBoundaryDirs...),
		Files:               []helperFile{},
		PathAliasPrefixes:   []string{"@/", "@shared/"},
		AdditionalFiles:     []string{},
	}
}

// TargetSelector returns the exact selected-package selector advertised to --target.
func TargetSelector(result Result) string { return result.Project.Selector }

type packageProjectCandidate struct {
	manifestPath string
	projectDir   string
	ownSources   map[string]struct{}
	ownFiles     map[string]struct{}
}

func readPackageManifest(repository *corpus.Corpus, manifestPath string) (packageManifest, error) {
	manifestID, ok := repository.ID(manifestPath)
	if !ok {
		return packageManifest{}, fmt.Errorf("jsts project: package manifest %q is unavailable", manifestPath)
	}
	manifestContent, err := repository.ReadFile(manifestID, maxManifestBytes)
	if err != nil {
		return packageManifest{}, fmt.Errorf("jsts project: read package manifest %q: %w", manifestPath, err)
	}
	if manifestContent.Truncated {
		return packageManifest{}, fmt.Errorf("jsts project: package manifest %q exceeds %d bytes", manifestPath, maxManifestBytes)
	}
	trimmed := bytes.TrimSpace(manifestContent.Bytes)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return packageManifest{}, fmt.Errorf("jsts project: invalid package manifest %q", manifestPath)
	}
	var manifest packageManifest
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	if err := decoder.Decode(&manifest); err != nil {
		return packageManifest{}, fmt.Errorf("jsts project: invalid package manifest %q", manifestPath)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return packageManifest{}, fmt.Errorf("jsts project: invalid package manifest %q", manifestPath)
	}
	return manifest, nil
}

// selectedPackageIdentityName restores a stable repository-owned name without
// consulting the absolute checkout path. package.json#name remains primary;
// npm's top-level package-lock name is an exact secondary identity. A package
// that declares neither keeps a deterministic repository-relative label.
func selectedPackageIdentityName(
	repository *corpus.Corpus,
	projectDir, manifestName string,
) string {
	if name := canonicalProjectIdentityName(manifestName); name != "" {
		return name
	}
	// Package-manager selection and project identity are separate authorities:
	// a coexisting bun/pnpm/yarn lockfile must not hide the exact npm root name.
	lockfileRepositoryPath := repositoryProjectPath(projectDir, "package-lock.json")
	if name := packageLockProjectName(repository, lockfileRepositoryPath); name != "" {
		return name
	}
	if projectDir == "." {
		return "root-package"
	}
	return projectDir
}

func packageLockProjectName(repository *corpus.Corpus, lockfilePath string) string {
	if repository == nil || lockfilePath == "" {
		return ""
	}
	lockfileID, ok := repository.ID(lockfilePath)
	if !ok {
		return ""
	}
	content, err := repository.ReadFile(lockfileID, maxManifestBytes)
	if err != nil || content.Truncated {
		return ""
	}
	var metadata struct {
		Name string `json:"name"`
	}
	decoder := json.NewDecoder(bytes.NewReader(content.Bytes))
	if err := decoder.Decode(&metadata); err != nil {
		return ""
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return ""
	}
	return canonicalProjectIdentityName(metadata.Name)
}

func canonicalProjectIdentityName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, "\x00\r\n") {
		return ""
	}
	return value
}

func selectProjectManifest(
	entries []corpus.Entry,
	selector string,
	rootManifest *packageManifest,
) (string, string, error) {
	candidates := packageProjectCandidates(entries)
	eligible := make([]packageProjectCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if len(candidate.ownSources) > 0 {
			eligible = append(eligible, candidate)
		}
	}

	if selector != "" {
		manifestPath, ok := strings.CutPrefix(selector, "jsts:")
		if !ok || manifestPath == "" || selector != "jsts:"+manifestPath {
			return "", "", fmt.Errorf(
				"jsts project: invalid exact selector %q; exact choices: %s",
				selector, packageSelectorChoices(eligible),
			)
		}
		for _, candidate := range eligible {
			if candidate.manifestPath == manifestPath {
				return candidate.manifestPath, candidate.projectDir, nil
			}
		}
		return "", "", fmt.Errorf(
			"jsts project: selector %q does not name a package with owned JavaScript or TypeScript source; exact choices: %s",
			selector, packageSelectorChoices(eligible),
		)
	}

	var root *packageProjectCandidate
	for index := range candidates {
		if candidates[index].manifestPath == "package.json" {
			root = &candidates[index]
			break
		}
	}
	if root != nil {
		if rootManifest == nil {
			return "", "", fmt.Errorf("jsts project: root package manifest metadata is unavailable")
		}
		if !declaresWorkspaces(rootManifest.Workspaces) {
			// A repository-root package remains authoritative only when it owns
			// source. Tooling-only manifests are common beside an independently
			// packaged frontend; selecting such a root here would advertise a
			// target that the exact-selector materialization path must reject.
			if len(root.ownSources) > 0 {
				return root.manifestPath, root.projectDir, nil
			}
		} else {
			if len(root.ownSources) > 0 && hasExactOwnedPackageEntry(*rootManifest, *root) {
				return root.manifestPath, root.projectDir, nil
			}
			delegates := exactWorkspaceDelegates(*rootManifest, eligible)
			if len(delegates) == 1 {
				return delegates[0].manifestPath, delegates[0].projectDir, nil
			}
			return "", "", fmt.Errorf(
				"jsts project: workspace-only root package has %d exact dev/start --cwd delegates; exact choices: %s",
				len(delegates), packageSelectorChoices(eligible),
			)
		}
	}

	switch len(eligible) {
	case 0:
		return "", "", fmt.Errorf("jsts project: package.json with owned JavaScript or TypeScript source is required")
	case 1:
		return eligible[0].manifestPath, eligible[0].projectDir, nil
	default:
		return "", "", fmt.Errorf(
			"jsts project: multiple nested package projects are ambiguous; exact choices: %s",
			packageSelectorChoices(eligible),
		)
	}
}

func packageProjectCandidates(entries []corpus.Entry) []packageProjectCandidate {
	manifestPaths := make([]string, 0)
	for _, entry := range entries {
		if path.Base(entry.Path) == "package.json" {
			manifestPaths = append(manifestPaths, entry.Path)
		}
	}
	sort.Strings(manifestPaths)
	candidates := make([]packageProjectCandidate, len(manifestPaths))
	for index, manifestPath := range manifestPaths {
		candidates[index] = packageProjectCandidate{
			manifestPath: manifestPath,
			projectDir:   path.Dir(manifestPath),
			ownSources:   map[string]struct{}{},
			ownFiles:     map[string]struct{}{},
		}
	}
	for _, entry := range entries {
		owner := -1
		for index, candidate := range candidates {
			if !projectContainsPath(candidate.projectDir, entry.Path) {
				continue
			}
			if owner == -1 || projectDirectoryDepth(candidate.projectDir) > projectDirectoryDepth(candidates[owner].projectDir) {
				owner = index
			}
		}
		if owner >= 0 {
			candidates[owner].ownFiles[entry.Path] = struct{}{}
			if sourceExtension(entry.Path) {
				candidates[owner].ownSources[entry.Path] = struct{}{}
			}
		}
	}
	return candidates
}

func projectContainsPath(projectDir, filePath string) bool {
	return projectDir == "." || strings.HasPrefix(filePath, projectDir+"/")
}

func projectDirectoryDepth(projectDir string) int {
	if projectDir == "." {
		return 0
	}
	return strings.Count(projectDir, "/") + 1
}

func packageSelectorChoices(candidates []packageProjectCandidate) string {
	choices := make([]string, len(candidates))
	for index, candidate := range candidates {
		choices[index] = "jsts:" + candidate.manifestPath
	}
	if len(choices) == 0 {
		return "none"
	}
	return strings.Join(choices, ", ")
}

func declaresWorkspaces(raw json.RawMessage) bool {
	if len(bytes.TrimSpace(raw)) == 0 {
		return false
	}
	var list []string
	if err := json.Unmarshal(raw, &list); err == nil {
		for _, value := range list {
			if strings.TrimSpace(value) != "" {
				return true
			}
		}
		return false
	}
	var object struct {
		Packages []string `json:"packages"`
	}
	if err := json.Unmarshal(raw, &object); err != nil {
		return false
	}
	for _, value := range object.Packages {
		if strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}

func hasExactOwnedPackageEntry(manifest packageManifest, candidate packageProjectCandidate) bool {
	entries := []string{manifest.Main, manifest.Module, manifest.Source, manifest.Types, manifest.Typings}
	for _, raw := range []json.RawMessage{manifest.Browser, manifest.Exports} {
		entries = append(entries, jsonStringLeaves(raw)...)
	}
	for _, entry := range entries {
		entry = strings.TrimPrefix(strings.TrimSpace(entry), "./")
		if entry == "" || entry == ".." || strings.HasPrefix(entry, "../") ||
			!safeRepositoryPath(entry) || !sourceExtension(entry) {
			continue
		}
		repositoryPath := repositoryProjectPath(candidate.projectDir, entry)
		if _, ok := candidate.ownSources[repositoryPath]; ok {
			return true
		}
	}
	for _, binary := range packageBinaryCandidates(manifest.Name, manifest.Bin) {
		repositoryPath := repositoryProjectPath(candidate.projectDir, binary.Path)
		if _, ok := candidate.ownFiles[repositoryPath]; ok {
			return true
		}
	}
	for _, name := range []string{"dev", "start"} {
		for _, entry := range scriptEntryPathsAtWorkingDirectory(manifest.Scripts[name]) {
			repositoryPath := repositoryProjectPath(candidate.projectDir, entry)
			if _, ok := candidate.ownSources[repositoryPath]; ok {
				return true
			}
		}
	}
	return false
}

func jsonStringLeaves(raw json.RawMessage) []string {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil
	}
	result := []string{}
	var visit func(any)
	visit = func(current any) {
		switch typed := current.(type) {
		case string:
			result = append(result, typed)
		case []any:
			for _, child := range typed {
				visit(child)
			}
		case map[string]any:
			keys := make([]string, 0, len(typed))
			for key := range typed {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				visit(typed[key])
			}
		}
	}
	visit(value)
	return canonicalStrings(result)
}

type packageBinaryCandidate struct {
	Command string
	Path    string
}

func packageBinaryFacts(
	packageName string,
	raw json.RawMessage,
	projectDir string,
	ownedFileRefs map[string]string,
) []PackageBinary {
	candidates := packageBinaryCandidates(packageName, raw)
	result := make([]PackageBinary, 0, len(candidates))
	for _, candidate := range candidates {
		fileRef := ownedFileRefs[candidate.Path]
		if fileRef == "" {
			continue
		}
		result = append(result, PackageBinary{
			Command: candidate.Command,
			Path:    repositoryProjectPath(projectDir, candidate.Path),
			FileRef: fileRef,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Command != result[j].Command {
			return result[i].Command < result[j].Command
		}
		if result[i].Path != result[j].Path {
			return result[i].Path < result[j].Path
		}
		return result[i].FileRef < result[j].FileRef
	})
	return result
}

// packageBinaryCandidates decodes only npm's string and command-to-path object
// forms. Invalid entries lose authority locally; duplicate command keys are
// ambiguous and discard that command without failing discovery of the package.
func packageBinaryCandidates(packageName string, raw json.RawMessage) []packageBinaryCandidate {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil
	}
	values := map[string]string{}
	invalid := map[string]struct{}{}
	if trimmed[0] == '"' {
		var target string
		if err := json.Unmarshal(trimmed, &target); err != nil {
			return nil
		}
		command := packageBinaryCommandFromName(packageName)
		values[command] = target
	} else if trimmed[0] == '{' {
		decoder := json.NewDecoder(bytes.NewReader(trimmed))
		token, err := decoder.Token()
		if err != nil || token != json.Delim('{') {
			return nil
		}
		for decoder.More() {
			token, err := decoder.Token()
			if err != nil {
				return nil
			}
			command, ok := token.(string)
			if !ok {
				return nil
			}
			var encodedTarget json.RawMessage
			if err := decoder.Decode(&encodedTarget); err != nil {
				return nil
			}
			if _, duplicate := values[command]; duplicate {
				delete(values, command)
				invalid[command] = struct{}{}
				continue
			}
			if _, alreadyInvalid := invalid[command]; alreadyInvalid {
				continue
			}
			var target string
			if err := json.Unmarshal(encodedTarget, &target); err != nil {
				invalid[command] = struct{}{}
				continue
			}
			values[command] = target
		}
		if token, err = decoder.Token(); err != nil || token != json.Delim('}') {
			return nil
		}
		var trailing any
		if err := decoder.Decode(&trailing); err != io.EOF {
			return nil
		}
	} else {
		return nil
	}

	result := make([]packageBinaryCandidate, 0, len(values))
	for command, rawPath := range values {
		if _, rejected := invalid[command]; rejected || !validPackageBinaryCommand(command) || rawPath != strings.TrimSpace(rawPath) {
			continue
		}
		binaryPath := strings.TrimPrefix(rawPath, "./")
		if binaryPath == "" || binaryPath == ".." || strings.HasPrefix(binaryPath, "../") || !safeRepositoryPath(binaryPath) {
			continue
		}
		result = append(result, packageBinaryCandidate{Command: command, Path: binaryPath})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Command != result[j].Command {
			return result[i].Command < result[j].Command
		}
		return result[i].Path < result[j].Path
	})
	return result
}

func packageBinaryCommandFromName(packageName string) string {
	if strings.HasPrefix(packageName, "@") {
		_, packageName, _ = strings.Cut(packageName, "/")
	}
	return packageName
}

func exactWorkspaceDelegates(
	manifest packageManifest,
	eligible []packageProjectCandidate,
) []packageProjectCandidate {
	byManifest := make(map[string]packageProjectCandidate, len(eligible))
	for _, candidate := range eligible {
		if candidate.manifestPath != "package.json" {
			byManifest[candidate.manifestPath] = candidate
		}
	}
	selected := map[string]packageProjectCandidate{}
	for _, name := range []string{"dev", "start"} {
		for _, directory := range scriptWorkingDirectories(manifest.Scripts[name]) {
			manifestPath := path.Join(directory, "package.json")
			if candidate, ok := byManifest[manifestPath]; ok {
				selected[manifestPath] = candidate
			}
		}
	}
	paths := make([]string, 0, len(selected))
	for manifestPath := range selected {
		paths = append(paths, manifestPath)
	}
	sort.Strings(paths)
	result := make([]packageProjectCandidate, len(paths))
	for index, manifestPath := range paths {
		result[index] = selected[manifestPath]
	}
	return result
}

func scriptWorkingDirectories(command string) []string {
	result, valid := parsedScriptWorkingDirectories(command)
	if !valid {
		return nil
	}
	write := 0
	for _, directory := range result {
		if directory == "." {
			continue
		}
		result[write] = directory
		write++
	}
	return result[:write]
}

func parsedScriptWorkingDirectories(command string) ([]string, bool) {
	fields := strings.Fields(command)
	result := []string{}
	for index := 0; index < len(fields); index++ {
		field := trimScriptToken(fields[index])
		value := ""
		switch {
		case field == "--cwd" && index+1 < len(fields):
			index++
			value = trimScriptToken(fields[index])
		case strings.HasPrefix(field, "--cwd="):
			value = trimScriptToken(strings.TrimPrefix(field, "--cwd="))
		case field == "--cwd":
			return nil, false
		default:
			continue
		}
		value = strings.TrimPrefix(value, "./")
		if value == "" {
			value = "."
		}
		if value == ".." || strings.HasPrefix(value, "../") ||
			!safeRepositoryPath(value) {
			return nil, false
		}
		result = append(result, value)
	}
	return canonicalStrings(result), true
}

func scriptEntryPathsAtWorkingDirectory(command string) []string {
	directories, valid := parsedScriptWorkingDirectories(command)
	if !valid || len(directories) > 1 {
		return nil
	}
	directory := "."
	if len(directories) == 1 {
		directory = directories[0]
	}
	result := []string{}
	for _, entry := range scriptEntryPaths(command) {
		resolved := path.Join(directory, entry)
		if resolved == ".." || strings.HasPrefix(resolved, "../") || !safeRepositoryPath(resolved) {
			continue
		}
		result = append(result, resolved)
	}
	return canonicalStrings(result)
}

func trimScriptToken(value string) string {
	if len(value) >= 2 && ((value[0] == '\'' && value[len(value)-1] == '\'') ||
		(value[0] == '"' && value[len(value)-1] == '"')) {
		return value[1 : len(value)-1]
	}
	return value
}

func projectRelativePath(projectDir, repositoryPath string) (string, bool) {
	if projectDir == "." {
		return repositoryPath, true
	}
	prefix := projectDir + "/"
	if !strings.HasPrefix(repositoryPath, prefix) {
		return "", false
	}
	return strings.TrimPrefix(repositoryPath, prefix), true
}

func repositoryProjectPath(projectDir, projectPath string) string {
	if projectPath == "" {
		return ""
	}
	if projectDir == "." {
		return projectPath
	}
	return path.Join(projectDir, projectPath)
}

func projectFileRef(repository *corpus.Corpus, projectDir, projectPath string) string {
	return fileRef(repository, repositoryProjectPath(projectDir, projectPath))
}

func rebasePathAliases(projectDir string, aliases []PathAlias) []PathAlias {
	if projectDir == "." {
		return aliases
	}
	result := make([]PathAlias, len(aliases))
	for index, alias := range aliases {
		result[index] = PathAlias{Pattern: alias.Pattern, Targets: make([]string, len(alias.Targets))}
		for targetIndex, target := range alias.Targets {
			result[index].Targets[targetIndex] = repositoryProjectPath(projectDir, target)
		}
	}
	return result
}

func rebaseHelperOutput(projectDir string, output *helperOutput) {
	if projectDir == "." || output == nil {
		return
	}
	rebaseLocation := func(location *Location) {
		location.Path = repositoryProjectPath(projectDir, location.Path)
	}
	for index := range output.Files {
		output.Files[index].Path = repositoryProjectPath(projectDir, output.Files[index].Path)
	}
	for index := range output.Declarations {
		rebaseLocation(&output.Declarations[index].Location)
	}
	for index := range output.Imports {
		rebaseLocation(&output.Imports[index].Location)
	}
	for index := range output.Exports {
		rebaseLocation(&output.Exports[index].Location)
	}
	for index := range output.Calls {
		rebaseLocation(&output.Calls[index].Location)
	}
	for index := range output.Surfaces {
		rebaseLocation(&output.Surfaces[index].Location)
	}
	for index := range output.Routes {
		rebaseLocation(&output.Routes[index].Location)
	}
	for index := range output.HTTPUses {
		rebaseLocation(&output.HTTPUses[index].Location)
	}
	for index := range output.Contracts {
		rebaseLocation(&output.Contracts[index].Location)
	}
	for index := range output.Resources {
		rebaseLocation(&output.Resources[index].Location)
	}
}

func sourceExtension(filePath string) bool {
	switch strings.ToLower(path.Ext(filePath)) {
	case ".ts", ".tsx", ".js", ".jsx", ".mts", ".cts", ".mjs", ".cjs":
		return true
	default:
		return false
	}
}

func packageManagerFacts(declared string, entries []corpus.Entry) (string, string) {
	if before, _, found := strings.Cut(strings.TrimSpace(declared), "@"); found && before != "" {
		for _, entry := range entries {
			switch entry.Path {
			case "pnpm-lock.yaml", "yarn.lock", "package-lock.json", "bun.lock", "bun.lockb":
				return before, entry.Path
			}
		}
		return before, ""
	}
	for _, entry := range entries {
		switch entry.Path {
		case "pnpm-lock.yaml":
			return "pnpm", entry.Path
		case "yarn.lock":
			return "yarn", entry.Path
		case "package-lock.json":
			return "npm", entry.Path
		case "bun.lock", "bun.lockb":
			return "bun", entry.Path
		}
	}
	return "npm", ""
}

type boundedBuffer struct {
	buffer   bytes.Buffer
	limit    int
	exceeded bool
}

func (writer *boundedBuffer) Write(value []byte) (int, error) {
	if writer.buffer.Len()+len(value) > writer.limit {
		writer.exceeded = true
		remaining := writer.limit - writer.buffer.Len()
		if remaining > 0 {
			_, _ = writer.buffer.Write(value[:remaining])
		}
		return len(value), fmt.Errorf("output exceeds limit")
	}
	return writer.buffer.Write(value)
}

func invokeHelper(ctx context.Context, repositoryRoot string, request helperRequest) (helperOutput, error) {
	encoded, err := json.Marshal(request)
	if err != nil {
		return helperOutput{}, fmt.Errorf("jsts project: encode helper request: %w", err)
	}
	nodePath, err := exec.LookPath("node")
	if err != nil {
		return helperOutput{}, fmt.Errorf("jsts project: Node.js executable is required")
	}
	command := exec.CommandContext(ctx, nodePath, "--input-type=module", "--eval", nodeHelper)
	command.Dir = repositoryRoot
	command.Env = []string{}
	command.Stdin = bytes.NewReader(encoded)
	stdout := &boundedBuffer{limit: maxHelperOutput}
	stderr := &boundedBuffer{limit: maxHelperStderrBytes}
	command.Stdout = stdout
	command.Stderr = stderr
	if err := command.Run(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return helperOutput{}, ctxErr
		}
		if stdout.exceeded {
			return helperOutput{}, fmt.Errorf("jsts project: TypeScript helper output exceeds %d bytes", maxHelperOutput)
		}
		closedDiagnostic := sanitizeDiagnostic(stderr.buffer.String(), repositoryRoot)
		if stderr.exceeded {
			closedDiagnostic = strings.TrimSpace(closedDiagnostic + fmt.Sprintf(" (diagnostic truncated at %d bytes)", maxHelperStderrBytes))
		}
		if strings.Contains(closedDiagnostic, "load prepared TypeScript compiler") || strings.Contains(closedDiagnostic, "TypeScript compiler is not rooted in analyzed node_modules") {
			if closedDiagnostic == "" {
				return helperOutput{}, ErrTypeScriptCompilerUnavailable
			}
			return helperOutput{}, fmt.Errorf("%w: %s", ErrTypeScriptCompilerUnavailable, closedDiagnostic)
		}
		if closedDiagnostic != "" {
			return helperOutput{}, fmt.Errorf("jsts project: TypeScript helper failed: %s", closedDiagnostic)
		}
		return helperOutput{}, fmt.Errorf("jsts project: TypeScript helper failed: %s", sanitizeDiagnostic(err.Error(), repositoryRoot))
	}
	decoder := json.NewDecoder(io.LimitReader(bytes.NewReader(stdout.buffer.Bytes()), maxHelperOutput))
	decoder.DisallowUnknownFields()
	var output helperOutput
	if err := decoder.Decode(&output); err != nil {
		return helperOutput{}, fmt.Errorf("jsts project: decode helper output: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return helperOutput{}, fmt.Errorf("jsts project: trailing helper output")
	}
	return output, nil
}

func sanitizeDiagnostic(value, root string) string {
	value = strings.ReplaceAll(value, root, "<repository>")
	value = strings.ReplaceAll(value, filepath.ToSlash(root), "<repository>")
	value = strings.Map(func(character rune) rune {
		if character == '\n' || character == '\r' || character == '\t' || character >= 0x20 {
			return character
		}
		return -1
	}, value)
	value = strings.TrimSpace(value)
	if len(value) > maxHelperDiagnosticBytes {
		value = value[:maxHelperDiagnosticBytes]
	}
	return value
}

func addScriptSurfaces(result *Result) {
	fileByPath := make(map[string]File, len(result.Files))
	for _, file := range result.Files {
		fileByPath[file.Path] = file
	}
	for _, script := range result.Project.Scripts {
		if script.Kind != "build" && script.Kind != "migration" {
			continue
		}
		for _, fileRef := range script.EntryFileRefs {
			var file File
			for _, candidate := range fileByPath {
				if candidate.FileRef == fileRef {
					file = candidate
					break
				}
			}
			if file.Path == "" {
				continue
			}
			ref := "surface:script:" + strings.ReplaceAll(script.Name, ":", "-")
			result.Surfaces = append(result.Surfaces, Surface{
				Ref: ref, Kind: SurfaceTool, Role: SurfaceScript, Name: script.Name + " script",
				EntryRefs: []string{moduleRefForFile(file.FileRef)}, EvidenceRefs: []string{},
				Location: Location{Path: file.Path, FileRef: file.FileRef, Line: 1, Column: 1},
			})
			break
		}
	}
}

func addPackageBinarySurfaces(result *Result) {
	for _, binary := range result.Project.Binaries {
		result.Surfaces = append(result.Surfaces, cliSurfaceForBinary(binary))
	}
}

func scriptEntryPaths(command string) []string {
	result := []string{}
	for _, field := range strings.Fields(command) {
		candidate := strings.TrimPrefix(strings.Trim(field, "\"'(),"), "./")
		if sourceExtension(candidate) && safeRepositoryPath(candidate) {
			result = append(result, candidate)
		}
	}
	return canonicalStrings(result)
}

func buildScriptFacts(scripts map[string]string, files []File) []Script {
	fileRefByPath := map[string]string{}
	for _, file := range files {
		fileRefByPath[file.Path] = file.FileRef
	}
	names := make([]string, 0, len(scripts))
	for name := range scripts {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]Script, 0, len(names))
	for _, name := range names {
		command := scripts[name]
		kind := "other"
		lower := strings.ToLower(name)
		switch {
		case strings.Contains(lower, "migrat"):
			kind = "migration"
		case strings.Contains(lower, "build"):
			kind = "build"
		case strings.Contains(lower, "test") || strings.Contains(lower, "lint") || strings.Contains(lower, "check") || strings.Contains(lower, "quality") || strings.Contains(lower, "verify") || strings.Contains(lower, "format"):
			kind = "quality"
		case name == "start" || name == "dev":
			kind = "runtime"
		}
		refs := []string{}
		for _, entry := range scriptEntryPaths(command) {
			if ref := fileRefByPath[entry]; ref != "" {
				refs = append(refs, ref)
			}
		}
		result = append(result, Script{Name: name, Kind: kind, EntryFileRefs: refs})
	}
	return result
}

func sourceRoots(files []File) []string {
	result := []string{}
	for _, file := range files {
		directory := path.Dir(file.Path)
		if directory == "." {
			result = append(result, ".")
			continue
		}
		result = append(result, strings.Split(directory, "/")[0])
	}
	return canonicalStrings(result)
}

func fileRef(repository *corpus.Corpus, filePath string) string {
	if filePath == "" {
		return ""
	}
	id, ok := repository.ID(filePath)
	if !ok {
		return ""
	}
	return string(id)
}

// typeScriptCompilerPackageNames returns only manifest-declared package names
// that can legitimately resolve to the TypeScript package. The helper still
// verifies the installed package.json#name and its Compiler API shape before
// granting compiler authority. This admits ordinary npm aliases without
// treating every installed or transitive node_modules package as a compiler.
func typeScriptCompilerPackageNames(manifests ...packageManifest) []string {
	packages := make([]string, 0, len(manifests))
	for _, manifest := range manifests {
		for _, dependencies := range []map[string]string{
			manifest.Dependencies,
			manifest.OptionalDependencies,
			manifest.DevDependencies,
		} {
			for packageName, declaration := range dependencies {
				declaration = strings.TrimSpace(declaration)
				if packageName == "typescript" || declaration == "npm:typescript" || strings.HasPrefix(declaration, "npm:typescript@") {
					packages = append(packages, packageName)
				}
			}
		}
	}
	return canonicalStrings(packages)
}

// typeScriptCompilerPackagesForProject keeps the selected package
// manifest authoritative. A repository-root manifest is consulted only when a
// nested selected package declares no compiler of its own; its unrelated
// compiler aliases can therefore neither override nor make the selected
// package's compiler ambiguous.
func typeScriptCompilerPackagesForProject(selected packageManifest, rootFallback *packageManifest) []helperCompilerPackage {
	if selectedPackages := typeScriptCompilerPackageNames(selected); len(selectedPackages) > 0 {
		return helperCompilerPackages(selectedPackages, helperCompilerResolutionProject)
	}
	if rootFallback == nil {
		return []helperCompilerPackage{}
	}
	return helperCompilerPackages(
		typeScriptCompilerPackageNames(*rootFallback),
		helperCompilerResolutionRepositoryRoot,
	)
}

func helperCompilerPackages(names []string, resolutionBase string) []helperCompilerPackage {
	result := make([]helperCompilerPackage, 0, len(names))
	for _, name := range names {
		result = append(result, helperCompilerPackage{Name: name, ResolutionBase: resolutionBase})
	}
	return result
}

func manifestDependencyFacts(manifest packageManifest) []PackageDependency {
	byPackage := map[string]PackageDependency{}
	add := func(values map[string]string, scope string) {
		for packagePath := range values {
			if _, exists := byPackage[packagePath]; exists {
				continue
			}
			byPackage[packagePath] = PackageDependency{PackagePath: packagePath, Scope: scope}
		}
	}
	add(manifest.Dependencies, "production")
	add(manifest.OptionalDependencies, "optional")
	add(manifest.DevDependencies, "development")
	result := make([]PackageDependency, 0, len(byPackage))
	for _, value := range byPackage {
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].PackagePath < result[j].PackagePath })
	return result
}

func moduleRefForFile(fileRef string) string { return "module:" + fileRef }
