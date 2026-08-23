package pythontarget

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"path"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/corpus"
)

const (
	defaultMaxFiles      = 20000
	defaultMaxFileBytes  = int64(2 << 20)
	defaultMaxTotalBytes = int64(64 << 20)
	maxHelperOutputBytes = int64(32 << 20)
	maxShebangBytes      = int64(512)
)

// Options controls bounded local discovery. Repository identity, file modes,
// and all bounded reads come exclusively from the supplied Corpus. The Python
// executable parses request bytes only and never imports repository modules.
type Options struct {
	PythonExecutable string
	MaxFiles         int
	MaxFileBytes     int64
	MaxTotalBytes    int64
}

type inputFile struct {
	ID      corpus.FileID `json:"-"`
	Path    string        `json:"path"`
	Kind    string        `json:"kind"`
	Content string        `json:"content"`
	Bytes   []byte        `json:"-"`
}

type helperRequest struct {
	Files []inputFile `json:"files"`
}

type helperResponse struct {
	Fatal   string         `json:"fatal,omitempty"`
	Configs []parsedConfig `json:"configs"`
	Sources []parsedSource `json:"sources"`
}

type parsedConfig struct {
	Path         string         `json:"path"`
	Scripts      []parsedScript `json:"scripts"`
	SourceRoots  []string       `json:"source_roots"`
	Packages     []string       `json:"packages"`
	Distribution bool           `json:"distribution"`
	Dynamic      bool           `json:"dynamic"`
	Errors       []helperError  `json:"errors"`
}

type helperError struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

type parsedScript struct {
	Name      string `json:"name"`
	Value     string `json:"value"`
	NameValue string `json:"name_value"`
	Kind      string `json:"kind"`
	Path      string `json:"path"`
	Line      int    `json:"line"`
}

type parsedSource struct {
	Path        string          `json:"path"`
	SyntaxError bool            `json:"syntax_error"`
	Bindings    []parsedBinding `json:"bindings"`
	Guards      []int           `json:"guards"`
}

type parsedBinding struct {
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	Module string `json:"module"`
	Target string `json:"target"`
	Level  int    `json:"level"`
	Line   int    `json:"line"`
}

type projectBuild struct {
	dir               string
	configPaths       []string
	packagingPaths    []string
	rawSourceRoots    []string
	explicitPackages  []string
	sourceRoots       []string
	scripts           []parsedScript
	modules           []Module
	packages          []Package
	moduleFiles       map[string][]Module
	bindings          map[string]map[string]parsedBinding
	guards            map[string][]int
	shebangs          map[string]int
	syntaxErrors      map[string]bool
	dynamicSetup      []string
	packagingManifest bool
	fileIDs           map[string]corpus.FileID
}

type targetBuild struct {
	target Target
}

// Discover inventories Python targets from one sealed repository corpus.
func Discover(ctx context.Context, repository *corpus.Corpus) (Catalog, error) {
	return DiscoverWithOptions(ctx, repository, Options{})
}

// DiscoverWithOptions is Discover with an explicit interpreter and resource
// bounds. It returns no partial value on I/O or malformed declarative
// configuration errors; unsupported dynamic facts become a typed omission.
func DiscoverWithOptions(ctx context.Context, repository *corpus.Corpus, options Options) (Catalog, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if repository == nil {
		return Catalog{}, fmt.Errorf("python target discovery: repository corpus is required")
	}
	if err := repository.Snapshot().Validate(); err != nil {
		return Catalog{}, fmt.Errorf("python target discovery: repository corpus: %w", err)
	}
	options = normalizeOptions(options)
	files, launcherFiles, err := readDiscoveryFiles(repository, options)
	if err != nil {
		return Catalog{}, err
	}
	parsed, err := runPythonParser(ctx, options.PythonExecutable, files)
	if err != nil {
		return Catalog{}, err
	}
	catalog, err := buildCatalog(files, launcherFiles, parsed)
	if err != nil {
		return Catalog{}, err
	}
	if err := validateCatalogCorpus(catalog, repository); err != nil {
		return Catalog{}, err
	}
	return catalog, nil
}

func normalizeOptions(options Options) Options {
	if strings.TrimSpace(options.PythonExecutable) == "" {
		options.PythonExecutable = "python3"
	}
	if options.MaxFiles <= 0 {
		options.MaxFiles = defaultMaxFiles
	}
	if options.MaxFileBytes <= 0 {
		options.MaxFileBytes = defaultMaxFileBytes
	}
	if options.MaxTotalBytes <= 0 {
		options.MaxTotalBytes = defaultMaxTotalBytes
	}
	return options
}

func readDiscoveryFiles(repository *corpus.Corpus, options Options) ([]inputFile, []inputFile, error) {
	files := make([]inputFile, 0)
	launchers := make([]inputFile, 0)
	var total int64
	executableProbes := 0
	for _, entry := range repository.Entries() {
		filePath := entry.Path
		kind, launcher := discoveryFileKind(filePath)
		probeExecutable := kind == "" && !launcher && entry.Executable && path.Ext(path.Base(filePath)) == ""
		if probeExecutable {
			executableProbes++
			if executableProbes > options.MaxFiles {
				return nil, nil, fmt.Errorf("python target discovery: executable probe limit %d exceeded", options.MaxFiles)
			}
			prefix, err := repository.ReadFile(entry.ID, maxShebangBytes)
			if err != nil {
				return nil, nil, fmt.Errorf("python target discovery: probe %q: %w", filePath, err)
			}
			if !hasExactPythonShebang(prefix.Bytes) {
				continue
			}
			kind = "python_script"
		}
		if kind == "" && !launcher {
			continue
		}
		if len(files)+len(launchers) >= options.MaxFiles {
			return nil, nil, fmt.Errorf("python target discovery: relevant file limit %d exceeded", options.MaxFiles)
		}
		if kind == "pipfile" || kind == "requirements" {
			files = append(files, inputFile{ID: entry.ID, Path: filePath, Kind: kind})
			continue
		}
		content, err := repository.ReadFile(entry.ID, options.MaxFileBytes)
		if err != nil {
			return nil, nil, fmt.Errorf("python target discovery: read %q: %w", filePath, err)
		}
		if content.Truncated {
			return nil, nil, fmt.Errorf("python target discovery: file %q exceeds %d bytes", filePath, options.MaxFileBytes)
		}
		total += int64(len(content.Bytes))
		if total > options.MaxTotalBytes {
			return nil, nil, fmt.Errorf("python target discovery: total byte limit %d exceeded", options.MaxTotalBytes)
		}
		file := inputFile{ID: entry.ID, Path: filePath, Kind: kind, Bytes: content.Bytes}
		if kind != "" {
			file.Content = base64.StdEncoding.EncodeToString(content.Bytes)
			files = append(files, file)
		}
		if launcher {
			launchers = append(launchers, file)
		}
	}
	return files, launchers, nil
}

func discoveryFileKind(filePath string) (string, bool) {
	base := path.Base(filePath)
	switch base {
	case "pyproject.toml":
		return "pyproject", false
	case "setup.cfg":
		return "setup_cfg", false
	case "setup.py":
		return "setup_py", false
	case "Pipfile":
		return "pipfile", false
	}
	if strings.HasPrefix(base, "requirements") && strings.HasSuffix(base, ".txt") {
		return "requirements", false
	}
	launcher := base == "Procfile" || strings.HasPrefix(base, "Procfile.") ||
		base == "Dockerfile" || strings.HasPrefix(base, "Dockerfile.") ||
		base == "compose.yml" || base == "compose.yaml" ||
		base == "docker-compose.yml" || base == "docker-compose.yaml"
	if strings.HasSuffix(base, ".py") {
		return "python", launcher
	}
	return "", launcher
}

func exactPythonShebangs(files []inputFile) map[string]int {
	result := make(map[string]int)
	for _, file := range files {
		if (file.Kind == "python" || file.Kind == "python_script" || file.Kind == "setup_py") && hasExactPythonShebang(file.Bytes) {
			result[file.Path] = 1
		}
	}
	return result
}

func hasExactPythonShebang(content []byte) bool {
	if len(content) < 3 || content[0] != '#' || content[1] != '!' {
		return false
	}
	first := content
	if index := bytes.IndexByte(first, '\n'); index >= 0 {
		first = first[:index]
	}
	first = bytes.TrimSuffix(first, []byte{'\r'})
	if len(first) > 512 {
		return false
	}
	fields := strings.Fields(string(first[2:]))
	if len(fields) == 0 {
		return false
	}
	interpreter := fields[0]
	if path.Base(interpreter) == "env" {
		index := 1
		if index < len(fields) && fields[index] == "-S" {
			index++
		}
		if index >= len(fields) {
			return false
		}
		interpreter = fields[index]
	}
	return isPythonInterpreterName(path.Base(interpreter))
}

func isPythonInterpreterName(name string) bool {
	for _, prefix := range []string{"python", "pythonw"} {
		if name == prefix {
			return true
		}
		suffix := strings.TrimPrefix(name, prefix)
		if suffix == name || suffix == "" {
			continue
		}
		valid := true
		previousDot := false
		for index, char := range suffix {
			if char == '.' {
				if index == 0 || previousDot {
					valid = false
					break
				}
				previousDot = true
				continue
			}
			if char < '0' || char > '9' {
				valid = false
				break
			}
			previousDot = false
		}
		if valid && !previousDot {
			return true
		}
	}
	return false
}

func runPythonParser(ctx context.Context, executable string, files []inputFile) (helperResponse, error) {
	if len(files) == 0 {
		return helperResponse{}, nil
	}
	requestBytes, err := json.Marshal(helperRequest{Files: files})
	if err != nil {
		return helperResponse{}, fmt.Errorf("python target discovery: encode parser request: %w", err)
	}
	command := exec.CommandContext(ctx, executable, "-I", "-S", "-c", pythonParserHelper)
	command.Stdin = bytes.NewReader(requestBytes)
	var stderr limitedBuffer
	stderr.limit = 16 << 10
	command.Stderr = &stderr
	stdout, err := command.StdoutPipe()
	if err != nil {
		return helperResponse{}, fmt.Errorf("python target discovery: parser stdout: %w", err)
	}
	if err := command.Start(); err != nil {
		return helperResponse{}, fmt.Errorf("python target discovery: start isolated parser: %w", err)
	}
	wire, readErr := io.ReadAll(io.LimitReader(stdout, maxHelperOutputBytes+1))
	waitErr := command.Wait()
	if readErr != nil {
		return helperResponse{}, fmt.Errorf("python target discovery: read parser output: %w", readErr)
	}
	if int64(len(wire)) > maxHelperOutputBytes {
		return helperResponse{}, fmt.Errorf("python target discovery: parser output exceeds %d bytes", maxHelperOutputBytes)
	}
	if waitErr != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return helperResponse{}, ctxErr
		}
		return helperResponse{}, fmt.Errorf("python target discovery: isolated parser failed: %s", strings.TrimSpace(stderr.String()))
	}
	var response helperResponse
	decoder := json.NewDecoder(bytes.NewReader(wire))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&response); err != nil {
		return helperResponse{}, fmt.Errorf("python target discovery: decode parser output: %w", err)
	}
	if response.Fatal != "" {
		return helperResponse{}, fmt.Errorf("python target discovery: %s", response.Fatal)
	}
	return response, nil
}

type limitedBuffer struct {
	bytes.Buffer
	limit int
}

func (buffer *limitedBuffer) Write(value []byte) (int, error) {
	original := len(value)
	remaining := buffer.limit - buffer.Len()
	if remaining > 0 {
		if len(value) > remaining {
			value = value[:remaining]
		}
		_, _ = buffer.Buffer.Write(value)
	}
	return original, nil
}

func buildCatalog(parsedFiles, launcherFiles []inputFile, parsed helperResponse) (Catalog, error) {
	fileIDs := make(map[string]corpus.FileID, len(parsedFiles)+len(launcherFiles))
	for _, file := range parsedFiles {
		fileIDs[file.Path] = file.ID
	}
	for _, file := range launcherFiles {
		fileIDs[file.Path] = file.ID
	}
	configs := make(map[string]parsedConfig, len(parsed.Configs))
	projectDirs := make(map[string]struct{})
	for _, config := range parsed.Configs {
		if err := validateRepoFile(config.Path); err != nil {
			return Catalog{}, fmt.Errorf("python target discovery: parser returned invalid config path: %w", err)
		}
		for _, issue := range config.Errors {
			return Catalog{}, fmt.Errorf("python target discovery: %s: %s", issue.Path, issue.Reason)
		}
		configs[config.Path] = config
		projectDirs[repoDir(config.Path)] = struct{}{}
	}
	for _, file := range parsedFiles {
		if file.Kind == "pipfile" || file.Kind == "requirements" {
			projectDirs[repoDir(file.Path)] = struct{}{}
		}
	}
	for _, file := range parsedFiles {
		if file.Kind != "python" && file.Kind != "python_script" && file.Kind != "setup_py" {
			continue
		}
		if !pathOwnedByProjectDir(file.Path, projectDirs) {
			projectDirs["."] = struct{}{}
			break
		}
	}
	projects := make(map[string]*projectBuild, len(projectDirs))
	for dir := range projectDirs {
		projects[dir] = &projectBuild{
			dir: dir, moduleFiles: make(map[string][]Module), bindings: make(map[string]map[string]parsedBinding),
			guards: make(map[string][]int), shebangs: make(map[string]int), syntaxErrors: make(map[string]bool),
			fileIDs: fileIDs,
		}
	}
	for _, config := range configs {
		project := projects[repoDir(config.Path)]
		project.configPaths = append(project.configPaths, config.Path)
		project.packagingManifest = project.packagingManifest || config.Distribution
		if config.Distribution {
			project.packagingPaths = append(project.packagingPaths, config.Path)
		}
		project.rawSourceRoots = append(project.rawSourceRoots, config.SourceRoots...)
		project.explicitPackages = append(project.explicitPackages, config.Packages...)
		project.scripts = append(project.scripts, config.Scripts...)
		if config.Dynamic {
			project.dynamicSetup = append(project.dynamicSetup, config.Path)
		}
	}
	for _, file := range parsedFiles {
		if file.Kind == "pipfile" {
			if project := projects[repoDir(file.Path)]; project != nil {
				project.configPaths = append(project.configPaths, file.Path)
			}
		}
	}
	pythonPaths := pythonFilePaths(parsedFiles)
	for _, project := range projects {
		roots, err := resolveSourceRoots(project.dir, project.rawSourceRoots, project.explicitPackages, pythonPaths, projects)
		if err != nil {
			return Catalog{}, err
		}
		project.sourceRoots = roots
	}

	sourceByPath := make(map[string]parsedSource, len(parsed.Sources))
	for _, source := range parsed.Sources {
		sourceByPath[source.Path] = source
	}
	shebangByPath := exactPythonShebangs(parsedFiles)
	scriptPathSet := pythonScriptPathSet(parsedFiles)
	for _, filePath := range pythonPaths {
		owner := nearestProject(filePath, projects)
		if owner == nil {
			continue
		}
		module, ok := moduleForPath(filePath, owner.sourceRoots)
		if !ok {
			if _, scriptFile := scriptPathSet[filePath]; scriptFile {
				module, ok = sourceScriptModuleForPath(filePath, owner.dir)
			} else {
				module, ok = sourceModuleForPath(filePath, owner.dir)
			}
			if !ok {
				continue
			}
		}
		module.FileID = fileIDs[filePath]
		owner.modules = append(owner.modules, module)
		if line := shebangByPath[filePath]; line > 0 {
			owner.shebangs[filePath] = line
		}
		if module.Importable {
			owner.moduleFiles[module.Name] = append(owner.moduleFiles[module.Name], module)
		}
		if source, exists := sourceByPath[filePath]; exists {
			owner.syntaxErrors[filePath] = source.SyntaxError
			bindings := make(map[string]parsedBinding)
			for _, binding := range source.Bindings {
				if !validModulePart(binding.Name) || binding.Line <= 0 {
					continue
				}
				if binding.Kind == "delete" {
					delete(bindings, binding.Name)
					continue
				}
				bindings[binding.Name] = binding
			}
			owner.bindings[filePath] = bindings
			owner.guards[filePath] = canonicalInts(source.Guards)
		}
	}

	for _, project := range projects {
		canonicalModules, err := canonicalModules(project.modules)
		if err != nil {
			return Catalog{}, fmt.Errorf("python target discovery: project %q: %w", project.dir, err)
		}
		project.modules = canonicalModules
		project.packages = discoverPackages(project)
	}

	entries := make([]Target, 0)
	moduleScopes := make([]ModuleScope, 0, len(projects))
	omissions := make([]Omission, 0)
	builds := make(map[string]*targetBuild)
	projectOrder := sortedProjects(projects)
	for _, project := range projectOrder {
		if len(project.modules) > 0 {
			moduleScopes = append(moduleScopes, ModuleScope{
				ProjectDir: project.dir, SourceRoots: cloneStrings(project.sourceRoots),
				Modules: cloneModules(project.modules),
			})
		}
		for _, configPath := range project.dynamicSetup {
			omissions = append(omissions, Omission{Kind: OmissionDynamicSetup, Path: configPath})
		}
		for sourcePath, syntaxError := range project.syntaxErrors {
			if syntaxError {
				omissions = append(omissions, Omission{Kind: OmissionSourceSyntax, Path: sourcePath})
			}
		}
		if err := buildScriptTargets(project, projects, builds, &omissions); err != nil {
			return Catalog{}, err
		}
		buildModuleTargets(project, builds)
		if project.packagingManifest && len(project.packages) > 0 && len(project.modules) > 0 {
			library, err := newLibraryTarget(project)
			if err != nil {
				return Catalog{}, err
			}
			builds[library.Selector] = &targetBuild{target: library}
		}
	}

	omissions = append(omissions, launchOmissions(launcherFiles)...)

	selectors := make([]string, 0, len(builds))
	for selector := range builds {
		selectors = append(selectors, selector)
	}
	sort.Strings(selectors)
	for _, selector := range selectors {
		build := builds[selector]
		entries = append(entries, build.target)
	}
	return newCatalogWithModuleScopes(entries, moduleScopes, omissions)
}

func validateCatalogCorpus(catalog Catalog, repository *corpus.Corpus) error {
	for _, scope := range catalog.ModuleScopes {
		for _, module := range scope.Modules {
			info, ok := repository.Info(module.FileID)
			if !ok || info.Entry.Path != module.Path {
				return fmt.Errorf("python target discovery: module scope source %q is not bound to repository corpus", module.Path)
			}
		}
	}
	for _, target := range catalog.Entries {
		for _, module := range target.Modules {
			info, ok := repository.Info(module.FileID)
			if !ok || info.Entry.Path != module.Path {
				return fmt.Errorf("python target discovery: module %q is not bound to repository corpus", module.Path)
			}
		}
		for _, basis := range target.Basis {
			info, ok := repository.Info(basis.FileID)
			if !ok || info.Entry.Path != basis.Path {
				return fmt.Errorf("python target discovery: basis %q is not bound to repository corpus", basis.Path)
			}
		}
		for _, sourceRef := range target.SourceRefs {
			if _, ok := repository.Info(sourceRef); !ok {
				return fmt.Errorf("python target discovery: source ref %q is outside repository corpus", sourceRef)
			}
		}
		if _, ok := repository.Info(target.AnchorFileRef); !ok {
			return fmt.Errorf("python target discovery: anchor ref %q is outside repository corpus", target.AnchorFileRef)
		}
	}
	return nil
}

func pythonFilePaths(files []inputFile) []string {
	values := make([]string, 0)
	for _, file := range files {
		if file.Kind == "python" || file.Kind == "python_script" || file.Kind == "setup_py" {
			values = append(values, file.Path)
		}
	}
	sort.Strings(values)
	return values
}

func pythonScriptPathSet(files []inputFile) map[string]struct{} {
	result := make(map[string]struct{})
	for _, file := range files {
		if file.Kind == "python_script" {
			result[file.Path] = struct{}{}
		}
	}
	return result
}

func resolveSourceRoots(projectDir string, raw, explicitPackages, pythonPaths []string, projects map[string]*projectBuild) ([]string, error) {
	values := make([]string, 0)
	for _, value := range raw {
		value = strings.TrimSpace(value)
		if value == "" || strings.HasPrefix(value, "/") || strings.Contains(value, "\\") {
			return nil, fmt.Errorf("python target discovery: project %q has unsafe source root %q", projectDir, value)
		}
		joined := path.Clean(path.Join(projectDir, value))
		if err := validateRepoDir(joined); err != nil || !pathWithin(projectDir, joined) {
			return nil, fmt.Errorf("python target discovery: project %q source root escapes project: %q", projectDir, value)
		}
		values = append(values, joined)
	}
	if len(values) == 0 {
		if len(explicitPackages) > 0 {
			values = append(values, projectDir)
		}
	}
	if len(values) == 0 {
		src := repoJoin(projectDir, "src")
		if hasOwnedPythonUnder(src, projectDir, pythonPaths, projects) {
			values = append(values, src)
		} else {
			values = append(values, projectDir)
		}
	}
	sort.Strings(values)
	return compactStrings(values), nil
}

func hasOwnedPythonUnder(root, projectDir string, pythonPaths []string, projects map[string]*projectBuild) bool {
	for _, filePath := range pythonPaths {
		if pathWithin(root, filePath) {
			owner := nearestProject(filePath, projects)
			if owner != nil && owner.dir == projectDir {
				return true
			}
		}
	}
	return false
}

func nearestProject(filePath string, projects map[string]*projectBuild) *projectBuild {
	var selected *projectBuild
	for dir, project := range projects {
		if pathWithin(dir, filePath) && (selected == nil || len(dir) > len(selected.dir)) {
			selected = project
		}
	}
	return selected
}

func pathOwnedByProjectDir(filePath string, projectDirs map[string]struct{}) bool {
	for dir := range projectDirs {
		if pathWithin(dir, filePath) {
			return true
		}
	}
	return false
}

func moduleForPath(filePath string, sourceRoots []string) (Module, bool) {
	root := longestContainingPath(sourceRoots, filePath)
	if root == "" {
		return Module{}, false
	}
	rel := filePath
	if root != "." {
		rel = strings.TrimPrefix(filePath, root+"/")
	}
	parts := strings.Split(rel, "/")
	if len(parts) == 0 || !strings.HasSuffix(parts[len(parts)-1], ".py") {
		return Module{}, false
	}
	fileName := strings.TrimSuffix(parts[len(parts)-1], ".py")
	isPackage := fileName == "__init__"
	if isPackage {
		parts = parts[:len(parts)-1]
	} else {
		parts[len(parts)-1] = fileName
	}
	if len(parts) == 0 {
		return Module{}, false
	}
	for _, part := range parts {
		if !validModulePart(part) {
			return Module{}, false
		}
	}
	return Module{Name: strings.Join(parts, "."), Path: filePath, Importable: true, Package: isPackage}, true
}

func sourceModuleForPath(filePath, projectDir string) (Module, bool) {
	rel := filePath
	if projectDir != "." {
		rel = strings.TrimPrefix(filePath, projectDir+"/")
	}
	if rel == "" || !strings.HasSuffix(rel, ".py") {
		return Module{}, false
	}
	parts := strings.Split(strings.TrimSuffix(rel, ".py"), "/")
	for _, part := range parts {
		if part == "" {
			return Module{}, false
		}
	}
	name := strings.Join(parts, ".")
	if !validSourceIdentity(name) {
		return Module{}, false
	}
	return Module{Name: name, Path: filePath}, true
}

func sourceScriptModuleForPath(filePath, projectDir string) (Module, bool) {
	rel := filePath
	if projectDir != "." {
		rel = strings.TrimPrefix(filePath, projectDir+"/")
	}
	if rel == "" {
		return Module{}, false
	}
	name := strings.ReplaceAll(rel, "/", ".")
	if !validSourceIdentity(name) {
		return Module{}, false
	}
	return Module{Name: name, Path: filePath}, true
}

func canonicalModules(values []Module) ([]Module, error) {
	sort.Slice(values, func(i, j int) bool { return moduleLess(values[i], values[j]) })
	result := values[:0]
	seenImportNames := make(map[string]string)
	seenPaths := make(map[string]Module)
	for _, value := range values {
		if previous, ok := seenImportNames[value.Name]; value.Importable && ok && previous != value.Path {
			return nil, fmt.Errorf("ambiguous import module %q resolves to %q and %q", value.Name, previous, value.Path)
		}
		if previous, ok := seenPaths[value.Path]; ok && previous != value {
			return nil, fmt.Errorf("source %q has conflicting module names", value.Path)
		}
		if len(result) == 0 || result[len(result)-1] != value {
			result = append(result, value)
		}
		if value.Importable {
			seenImportNames[value.Name] = value.Path
		}
		seenPaths[value.Path] = value
	}
	return result, nil
}

func discoverPackages(project *projectBuild) []Package {
	byName := make(map[string]Package)
	for _, module := range project.modules {
		if !module.Importable {
			continue
		}
		name := strings.Split(module.Name, ".")[0]
		if _, exists := byName[name]; exists {
			continue
		}
		root := longestContainingPath(project.sourceRoots, module.Path)
		if root == "" {
			continue
		}
		rel := module.Path
		if root != "." {
			rel = strings.TrimPrefix(module.Path, root+"/")
		}
		parts := strings.Split(rel, "/")
		if len(parts) == 1 {
			byName[name] = Package{Name: name, Dir: root, Path: module.Path}
			continue
		}
		dir := repoJoin(root, parts[0])
		initPath := repoJoin(dir, "__init__.py")
		if moduleAtPath(project.modules, initPath) {
			byName[name] = Package{Name: name, Dir: dir, Path: initPath}
		} else {
			byName[name] = Package{Name: name, Dir: dir, Namespace: true}
		}
	}
	values := make([]Package, 0, len(byName))
	for _, value := range byName {
		values = append(values, value)
	}
	sort.Slice(values, func(i, j int) bool { return packageLess(values[i], values[j]) })
	return values
}

func buildScriptTargets(project *projectBuild, projects map[string]*projectBuild, builds map[string]*targetBuild, omissions *[]Omission) error {
	sort.Slice(project.scripts, func(i, j int) bool {
		left, right := project.scripts[i], project.scripts[j]
		if left.Path != right.Path {
			return left.Path < right.Path
		}
		if left.Line != right.Line {
			return left.Line < right.Line
		}
		return left.Name+left.NameValue < right.Name+right.NameValue
	})
	for _, script := range project.scripts {
		name, spec, ok := normalizeScript(script)
		if !ok {
			label := strings.TrimSpace(script.Name)
			if !safeLabel(label) {
				label = ""
			}
			*omissions = append(*omissions, Omission{Kind: OmissionUnresolvedRoot, Path: script.Path, Line: script.Line, Label: label})
			continue
		}
		moduleName, qualname, ok := parseObjectSpec(spec)
		if !ok {
			*omissions = append(*omissions, Omission{Kind: OmissionUnresolvedRoot, Path: script.Path, Line: script.Line, Label: name})
			continue
		}
		rootProject, root, state := resolveMetadataEntrypointRoot(project, projects, moduleName, qualname)
		if state != 1 {
			kind := OmissionUnresolvedRoot
			if state > 1 {
				kind = OmissionAmbiguousRoot
			}
			*omissions = append(*omissions, Omission{Kind: kind, Path: script.Path, Line: script.Line, Label: name})
			continue
		}
		basisKind := scriptBasisKind(script.Kind)
		basis := project.basis(basisKind, script.Path, script.Line, name)
		selector := selectorFor(project.dir, scriptSelectorKind(script.Kind), name)
		target := Target{
			Version: TargetVersion, Kind: KindExecutable, Selector: selector, DisplayName: name,
			ProjectDir: rootProject.dir, SourceRoots: cloneStrings(rootProject.sourceRoots), Modules: cloneModules(rootProject.modules),
			Roots: []Root{root},
			Basis: []Basis{basis},
		}
		if existing, found := builds[selector]; found {
			if !sameRoots(existing.target.Roots, target.Roots) {
				return fmt.Errorf("python target discovery: script %q in project %q has conflicting roots", name, project.dir)
			}
			existing.target.Basis = append(existing.target.Basis, basis)
			continue
		}
		builds[selector] = &targetBuild{target: target}
	}
	return nil
}

// resolveMetadataEntrypointRoot gives the declaring project first authority.
// Only when it has no module with that exact import name may declarative
// packaging metadata refer to one globally unique module owned by another
// discovered project. Duplicate global import names remain ambiguous.
func resolveMetadataEntrypointRoot(project *projectBuild, projects map[string]*projectBuild, moduleName, qualname string) (*projectBuild, Root, int) {
	if len(project.moduleFiles[moduleName]) > 0 {
		root, state := resolveEntrypointRoot(project, moduleName, qualname, nil)
		return project, root, state
	}
	var owner *projectBuild
	moduleCount := 0
	for _, candidate := range sortedProjects(projects) {
		if candidate == project {
			continue
		}
		count := len(candidate.moduleFiles[moduleName])
		if count == 0 {
			continue
		}
		moduleCount += count
		owner = candidate
	}
	if moduleCount == 0 {
		return nil, Root{}, 0
	}
	if moduleCount != 1 {
		return nil, Root{}, moduleCount
	}
	root, state := resolveEntrypointRoot(owner, moduleName, qualname, nil)
	return owner, root, state
}

// resolveEntrypointRoot follows only explicit top-level aliases. Repository
// modules are never imported, and cycles or long chains close as unresolved.
func resolveEntrypointRoot(project *projectBuild, moduleName, qualname string, seen map[string]struct{}) (Root, int) {
	modules := project.moduleFiles[moduleName]
	if len(modules) != 1 {
		return Root{}, len(modules)
	}
	module := modules[0]
	if project.syntaxErrors[module.Path] {
		return Root{}, 0
	}
	if seen == nil {
		seen = make(map[string]struct{})
	}
	key := moduleName + ":" + qualname
	if len(seen) >= 8 {
		return Root{}, 0
	}
	if _, exists := seen[key]; exists {
		return Root{}, 0
	}
	seen[key] = struct{}{}
	parts := strings.Split(qualname, ".")
	binding, exists := project.bindings[module.Path][parts[0]]
	if !exists {
		return Root{}, 0
	}
	switch binding.Kind {
	case "function", "async_function":
		if len(parts) != 1 {
			return Root{}, 0
		}
		return Root{Kind: RootCallable, Module: moduleName, Qualname: binding.Name, Path: module.Path, Line: binding.Line}, 1
	case "class", "object":
		if len(parts) != 1 {
			return Root{}, 0
		}
		return Root{Kind: RootBoundObject, Module: moduleName, Qualname: binding.Name, Path: module.Path, Line: binding.Line}, 1
	case "alias":
		if len(parts) != 1 {
			return Root{}, 0
		}
		targetModule, ok := resolveAliasModule(module, binding)
		if !ok || !validModulePart(binding.Target) {
			return Root{}, 0
		}
		return resolveEntrypointRoot(project, targetModule, binding.Target, seen)
	case "alias_module":
		if len(parts) < 2 {
			return Root{}, 0
		}
		targetModule, ok := resolveAliasModule(module, binding)
		if !ok {
			return Root{}, 0
		}
		return resolveEntrypointRoot(project, targetModule, strings.Join(parts[1:], "."), seen)
	default:
		return Root{}, 0
	}
}

func resolveAliasModule(current Module, alias parsedBinding) (string, bool) {
	if alias.Level == 0 {
		return alias.Module, validModule(alias.Module)
	}
	base := current.Name
	if !current.Package {
		if index := strings.LastIndex(base, "."); index >= 0 {
			base = base[:index]
		} else {
			base = ""
		}
	}
	parts := []string(nil)
	if base != "" {
		parts = strings.Split(base, ".")
	}
	remove := alias.Level - 1
	if remove > len(parts) {
		return "", false
	}
	parts = parts[:len(parts)-remove]
	if alias.Module != "" {
		parts = append(parts, strings.Split(alias.Module, ".")...)
	}
	value := strings.Join(parts, ".")
	return value, validModule(value)
}

func buildModuleTargets(project *projectBuild, builds map[string]*targetBuild) {
	for _, module := range project.modules {
		if project.syntaxErrors[module.Path] {
			continue
		}
		guards := project.guards[module.Path]
		shebangLine := project.shebangs[module.Path]
		if module.Importable && strings.HasSuffix(module.Name, ".__main__") {
			name := strings.TrimSuffix(module.Name, ".__main__")
			selector := selectorFor(project.dir, "module", name)
			basis := []Basis{project.basis(BasisPackageMain, module.Path, 1, name)}
			for _, line := range guards {
				basis = append(basis, project.basis(BasisNameMainGuard, module.Path, line, module.Name))
			}
			if shebangLine > 0 {
				basis = append(basis, project.basis(BasisPythonShebang, module.Path, shebangLine, module.Name))
			}
			target := Target{
				Version: TargetVersion, Kind: KindExecutable, Selector: selector, DisplayName: "python -m " + name,
				ProjectDir: project.dir, SourceRoots: cloneStrings(project.sourceRoots), Modules: cloneModules(project.modules),
				Roots: []Root{{Kind: RootModule, Module: name, Path: module.Path, Line: 1}},
				Basis: basis,
			}
			builds[selector] = &targetBuild{target: target}
			continue
		}
		if len(guards) > 0 {
			selector := selectorFor(project.dir, "guard", guardSelectorLabel(project.dir, module.Path))
			roots := make([]Root, 0, len(guards))
			basis := make([]Basis, 0, len(guards)+1)
			for _, line := range guards {
				roots = append(roots, Root{Kind: RootMainGuard, Module: module.Name, Path: module.Path, Line: line})
				basis = append(basis, project.basis(BasisNameMainGuard, module.Path, line, module.Name))
			}
			if shebangLine > 0 {
				basis = append(basis, project.basis(BasisPythonShebang, module.Path, shebangLine, module.Name))
			}
			builds[selector] = &targetBuild{target: Target{
				Version: TargetVersion, Kind: KindExecutable, Selector: selector, DisplayName: module.Name,
				ProjectDir: project.dir, SourceRoots: cloneStrings(project.sourceRoots), Modules: cloneModules(project.modules),
				Roots: roots, Basis: basis,
			}}
			continue
		}
		if shebangLine == 0 {
			continue
		}
		label := guardSelectorLabel(project.dir, module.Path)
		selector := selectorFor(project.dir, "script-file", label)
		builds[selector] = &targetBuild{target: Target{
			Version: TargetVersion, Kind: KindExecutable, Selector: selector, DisplayName: module.Name,
			ProjectDir: project.dir, SourceRoots: cloneStrings(project.sourceRoots), Modules: cloneModules(project.modules),
			Roots: []Root{{Kind: RootScriptFile, Module: module.Name, Path: module.Path, Line: shebangLine}},
			Basis: []Basis{project.basis(BasisPythonShebang, module.Path, shebangLine, module.Name)},
		}}
	}
}

func guardSelectorLabel(projectDir, filePath string) string {
	rel := filePath
	if projectDir != "." {
		rel = strings.TrimPrefix(filePath, projectDir+"/")
	}
	return strings.TrimSuffix(rel, ".py")
}

func newLibraryTarget(project *projectBuild) (Target, error) {
	var basis Basis
	if len(project.packagingPaths) > 0 {
		sort.Strings(project.packagingPaths)
		basis = project.basis(BasisImportPackage, project.packagingPaths[0], 0, "")
	} else {
		first := project.packages[0]
		basisPath := first.Path
		if basisPath == "" {
			for _, module := range project.modules {
				if strings.Split(module.Name, ".")[0] == first.Name {
					basisPath = module.Path
					break
				}
			}
		}
		basis = project.basis(BasisImportPackage, basisPath, 0, first.Name)
	}
	return Target{
		Version: TargetVersion, Kind: KindLibrary, Selector: selectorFor(project.dir, "library", "library"),
		DisplayName: projectDisplay(project.dir) + " library", ProjectDir: project.dir,
		SourceRoots: cloneStrings(project.sourceRoots), Modules: cloneModules(project.modules),
		Packages: append([]Package(nil), project.packages...), Basis: []Basis{basis},
	}, nil
}

func normalizeScript(script parsedScript) (string, string, bool) {
	name, value := strings.TrimSpace(script.Name), strings.TrimSpace(script.Value)
	if script.NameValue != "" {
		var found bool
		name, value, found = strings.Cut(script.NameValue, "=")
		if !found {
			return "", "", false
		}
		name, value = strings.TrimSpace(name), strings.TrimSpace(value)
	}
	return name, value, safeLabel(name) && value != ""
}

func parseObjectSpec(value string) (string, string, bool) {
	if index := strings.LastIndex(value, "["); index >= 0 && strings.HasSuffix(value, "]") {
		value = strings.TrimSpace(value[:index])
	}
	moduleName, qualname, found := strings.Cut(value, ":")
	moduleName, qualname = strings.TrimSpace(moduleName), strings.TrimSpace(qualname)
	return moduleName, qualname, found && validModule(moduleName) && validQualname(qualname)
}

func scriptBasisKind(kind string) BasisKind {
	switch kind {
	case "pep621":
		return BasisPEP621Script
	case "pep621_gui":
		return BasisPEP621GUIScript
	case "poetry":
		return BasisPoetryScript
	case "setup_cfg":
		return BasisSetupCFGScript
	case "setup_cfg_gui":
		return BasisSetupCFGGUIScript
	case "setup_py_gui":
		return BasisSetupPYGUIScript
	default:
		return BasisSetupPYScript
	}
}

func scriptSelectorKind(kind string) string {
	switch kind {
	case "pep621_gui", "setup_cfg_gui", "setup_py_gui":
		return "gui-script"
	default:
		return "script"
	}
}

func canonicalizeTarget(target *Target) {
	sort.Strings(target.SourceRoots)
	target.SourceRoots = compactStrings(target.SourceRoots)
	sort.Slice(target.Modules, func(i, j int) bool { return moduleLess(target.Modules[i], target.Modules[j]) })
	sort.Slice(target.Roots, func(i, j int) bool { return rootLess(target.Roots[i], target.Roots[j]) })
	sort.Slice(target.Packages, func(i, j int) bool { return packageLess(target.Packages[i], target.Packages[j]) })
	sort.Slice(target.Basis, func(i, j int) bool { return basisLess(target.Basis[i], target.Basis[j]) })
	target.Basis = compactBasis(target.Basis)
}

func sortedProjects(projects map[string]*projectBuild) []*projectBuild {
	dirs := make([]string, 0, len(projects))
	for dir := range projects {
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)
	result := make([]*projectBuild, 0, len(dirs))
	for _, dir := range dirs {
		result = append(result, projects[dir])
	}
	return result
}

func repoDir(filePath string) string {
	dir := path.Dir(filePath)
	if dir == "" {
		return "."
	}
	return dir
}

func repoJoin(base, value string) string {
	if base == "." {
		return path.Clean(value)
	}
	return path.Join(base, value)
}

func pathWithin(dir, value string) bool {
	return dir == "." || value == dir || strings.HasPrefix(value, dir+"/")
}

func longestContainingPath(values []string, filePath string) string {
	selected := ""
	for _, value := range values {
		if pathWithin(value, filePath) && len(value) > len(selected) {
			selected = value
		}
	}
	return selected
}

func moduleAtPath(values []Module, filePath string) bool {
	for _, value := range values {
		if value.Path == filePath {
			return true
		}
	}
	return false
}

func compactStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	result := values[:0]
	for _, value := range values {
		if len(result) == 0 || result[len(result)-1] != value {
			result = append(result, value)
		}
	}
	return result
}

func canonicalInts(values []int) []int {
	sort.Ints(values)
	result := values[:0]
	for _, value := range values {
		if value > 0 && (len(result) == 0 || result[len(result)-1] != value) {
			result = append(result, value)
		}
	}
	return result
}

func cloneStrings(values []string) []string { return append([]string(nil), values...) }
func cloneModules(values []Module) []Module { return append([]Module(nil), values...) }

func (project *projectBuild) basis(kind BasisKind, filePath string, line int, label string) Basis {
	return Basis{FileID: project.fileIDs[filePath], Kind: kind, Path: filePath, Line: line, Label: label}
}

func selectorFor(projectDir, kind, name string) string {
	return "python:" + projectDir + ":" + kind + ":" + name
}

func projectDisplay(projectDir string) string {
	if projectDir == "." {
		return "Python"
	}
	return path.Base(projectDir)
}

func sameRoots(left, right []Root) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func compactBasis(values []Basis) []Basis {
	if len(values) == 0 {
		return nil
	}
	result := values[:0]
	for _, value := range values {
		if len(result) == 0 || result[len(result)-1] != value {
			result = append(result, value)
		}
	}
	return result
}

func safeLabel(value string) bool {
	return value != "" && len(value) <= 256 && strings.TrimSpace(value) == value &&
		!strings.ContainsAny(value, "\x00\r\n")
}

func launchOmissions(files []inputFile) []Omission {
	var omissions []Omission
	for _, file := range files {
		base := path.Base(file.Path)
		switch {
		case base == "Procfile" || strings.HasPrefix(base, "Procfile."):
			for index, row := range strings.Split(string(file.Bytes), "\n") {
				trimmed := strings.TrimSpace(row)
				if trimmed == "" || strings.HasPrefix(trimmed, "#") {
					continue
				}
				label, _, _ := strings.Cut(trimmed, ":")
				label = strings.TrimSpace(label)
				if !safeLabel(label) {
					label = ""
				}
				omissions = append(omissions, Omission{Kind: OmissionUnsupportedLaunch, Path: file.Path, Line: index + 1, Label: label})
			}
		case base == "Dockerfile" || strings.HasPrefix(base, "Dockerfile."):
			for index, row := range strings.Split(string(file.Bytes), "\n") {
				trimmed := strings.TrimSpace(row)
				upper := strings.ToUpper(trimmed)
				if !strings.HasPrefix(upper, "CMD ") && !strings.HasPrefix(upper, "ENTRYPOINT ") {
					continue
				}
				omissions = append(omissions, Omission{Kind: OmissionUnsupportedLaunch, Path: file.Path, Line: index + 1})
			}
		default:
			for index, row := range strings.Split(string(file.Bytes), "\n") {
				trimmed := strings.TrimSpace(row)
				if !strings.HasPrefix(trimmed, "command:") && !strings.HasPrefix(trimmed, "entrypoint:") {
					continue
				}
				omissions = append(omissions, Omission{Kind: OmissionUnsupportedLaunch, Path: file.Path, Line: index + 1})
			}
		}
	}
	return omissions
}
