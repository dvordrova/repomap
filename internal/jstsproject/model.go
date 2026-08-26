// Package jstsproject owns deterministic JavaScript and TypeScript project
// discovery and the sealed adapter handoff used by the ordinary repomap path.
package jstsproject

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"path"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/secretscan"
)

const (
	Version                      = 4
	HelperVersion                = 5
	ArtifactFilename             = "jsts-project.json"
	ProgramIndexFilename         = "program-index-jsts.json"
	MaxArtifactBytes             = 64 << 20
	maxPackageBinaryCommandBytes = 240
	redactedExpression           = "<persistence-sensitive expression omitted>"
	javascriptPlatform           = "platform:javascript"
)

type Location struct {
	Path    string `json:"path"`
	FileRef string `json:"file_ref"`
	Line    int    `json:"line"`
	Column  int    `json:"column"`
}

type PathAlias struct {
	Pattern string   `json:"pattern"`
	Targets []string `json:"targets"`
}

type Script struct {
	Name          string   `json:"name"`
	Kind          string   `json:"kind"`
	EntryFileRefs []string `json:"entry_file_refs"`
}

type ProjectFile struct {
	Path    string `json:"path"`
	FileRef string `json:"file_ref"`
}

// PackageBinary is one exact command-to-file declaration from package.json#bin.
// Path and FileRef bind only tracked, selected-package-owned corpus files; the
// manifest command itself is never treated as proof of an implementation
// relation to another source file.
type PackageBinary struct {
	Command string `json:"command"`
	Path    string `json:"path"`
	FileRef string `json:"file_ref"`
}

type PackageDependency struct {
	PackagePath string `json:"package_path"`
	Scope       string `json:"scope"`
}

type Project struct {
	Ref              string              `json:"ref"`
	Name             string              `json:"name"`
	PackagePath      string              `json:"package_path"`
	Language         string              `json:"language"`
	Selector         string              `json:"selector"`
	ManifestPath     string              `json:"manifest_path"`
	ManifestFileRef  string              `json:"manifest_file_ref"`
	ConfigPath       string              `json:"config_path,omitempty"`
	ConfigFileRef    string              `json:"config_file_ref,omitempty"`
	PackageManager   string              `json:"package_manager,omitempty"`
	LockfilePath     string              `json:"lockfile_path,omitempty"`
	LockfileFileRef  string              `json:"lockfile_file_ref,omitempty"`
	ModuleResolution string              `json:"module_resolution"`
	BaseURL          string              `json:"base_url,omitempty"`
	PathAliases      []PathAlias         `json:"path_aliases"`
	Scripts          []Script            `json:"scripts"`
	Binaries         []PackageBinary     `json:"binaries"`
	SourceRoots      []string            `json:"source_roots"`
	EntryFileRefs    []string            `json:"entry_file_refs"`
	ToolConfigs      []ProjectFile       `json:"tool_configs"`
	Dependencies     []PackageDependency `json:"dependencies"`
}

type File struct {
	FileRef  string `json:"file_ref"`
	Path     string `json:"path"`
	Language string `json:"language"`
	Module   string `json:"module"`
	SHA256   string `json:"sha256"`
}

type Declaration struct {
	Ref           string   `json:"ref"`
	Kind          string   `json:"kind"`
	Name          string   `json:"name"`
	QualifiedName string   `json:"qualified_name"`
	Signature     string   `json:"signature,omitempty"`
	Exported      bool     `json:"exported"`
	OwnerRef      string   `json:"owner_ref,omitempty"`
	Location      Location `json:"location"`
}

type Import struct {
	Ref             string   `json:"ref"`
	Kind            string   `json:"kind"`
	Specifier       string   `json:"specifier"`
	ImporterFileRef string   `json:"importer_file_ref"`
	ResolvedFileRef string   `json:"resolved_file_ref,omitempty"`
	ExternalPackage string   `json:"external_package,omitempty"`
	Resolution      string   `json:"resolution"`
	Location        Location `json:"location"`
}

type Export struct {
	Ref             string   `json:"ref"`
	Kind            string   `json:"kind"`
	Name            string   `json:"name"`
	DeclarationRef  string   `json:"declaration_ref,omitempty"`
	FromSpecifier   string   `json:"from_specifier,omitempty"`
	ResolvedFileRef string   `json:"resolved_file_ref,omitempty"`
	Resolution      string   `json:"resolution"`
	Location        Location `json:"location"`
}

type Call struct {
	Ref              string   `json:"ref"`
	CallerRef        string   `json:"caller_ref"`
	CalleeRefs       []string `json:"callee_refs"`
	Invocation       string   `json:"invocation"`
	ExternalPackage  string   `json:"external_package,omitempty"`
	ExternalReceiver string   `json:"external_receiver,omitempty"`
	ExternalName     string   `json:"external_name,omitempty"`
	Expression       string   `json:"expression"`
	Resolution       string   `json:"resolution"`
	Location         Location `json:"location"`
}

type SurfaceKind string

const (
	SurfaceBrowser SurfaceKind = "browser_application"
	SurfaceServer  SurfaceKind = "node_server"
	SurfaceCLI     SurfaceKind = "command_line_application"
	SurfaceShared  SurfaceKind = "shared_contracts"
	SurfaceTool    SurfaceKind = "tool"
	SurfaceUnknown SurfaceKind = "unknown"
)

type SurfaceRole string

const (
	SurfaceProduct      SurfaceRole = "product"
	SurfaceSupporting   SurfaceRole = "supporting"
	SurfaceScript       SurfaceRole = "script"
	SurfaceUnclassified SurfaceRole = "unknown"
)

type Surface struct {
	Ref          string      `json:"ref"`
	Kind         SurfaceKind `json:"kind"`
	Role         SurfaceRole `json:"role"`
	Name         string      `json:"name"`
	EntryRefs    []string    `json:"entry_refs"`
	EvidenceRefs []string    `json:"evidence_refs"`
	Location     Location    `json:"location"`
}

type RouteKind string

const (
	RouteBrowser     RouteKind = "browser_route"
	RouteBrowserLink RouteKind = "browser_link"
	RouteHTTP        RouteKind = "http_route"
	RouteMiddleware  RouteKind = "middleware"
)

type Route struct {
	Ref            string    `json:"ref"`
	Kind           RouteKind `json:"kind"`
	Method         string    `json:"method,omitempty"`
	Path           string    `json:"path"`
	OwnerRef       string    `json:"owner_ref,omitempty"`
	ComponentRef   string    `json:"component_ref,omitempty"`
	MiddlewareRefs []string  `json:"middleware_refs"`
	HandlerRefs    []string  `json:"handler_refs"`
	Resolution     string    `json:"resolution"`
	Location       Location  `json:"location"`
}

type HTTPUse struct {
	Ref        string   `json:"ref"`
	Kind       string   `json:"kind"`
	Method     string   `json:"method"`
	Path       string   `json:"path"`
	CallerRef  string   `json:"caller_ref"`
	QueryKeys  []string `json:"query_keys"`
	Resolution string   `json:"resolution"`
	Location   Location `json:"location"`
}

type Contract struct {
	Ref            string   `json:"ref"`
	Kind           string   `json:"kind"`
	Name           string   `json:"name"`
	Value          string   `json:"value,omitempty"`
	DeclarationRef string   `json:"declaration_ref,omitempty"`
	UsedByRefs     []string `json:"used_by_refs"`
	Location       Location `json:"location"`
}

type Resource struct {
	Ref          string   `json:"ref"`
	Kind         string   `json:"kind"`
	Name         string   `json:"name"`
	PackagePath  string   `json:"package_path,omitempty"`
	UsedByRefs   []string `json:"used_by_refs"`
	EvidenceRefs []string `json:"evidence_refs"`
	Location     Location `json:"location"`
}

type PathStep struct {
	Ordinal    int      `json:"ordinal"`
	Kind       string   `json:"kind"`
	Label      string   `json:"label"`
	SourceRef  string   `json:"source_ref"`
	TargetRefs []string `json:"target_refs"`
	Resolution string   `json:"resolution"`
	Authority  string   `json:"authority"`
	Location   Location `json:"location"`
}

type ProductPath struct {
	Ref      string     `json:"ref"`
	Name     string     `json:"name"`
	Outcome  string     `json:"outcome"`
	Steps    []PathStep `json:"steps"`
	Frontier string     `json:"frontier,omitempty"`
}

type Result struct {
	Version         int           `json:"version"`
	HelperVersion   int           `json:"helper_version"`
	CorpusSHA256    string        `json:"corpus_sha256"`
	SourceSHA256    string        `json:"source_sha256"`
	ProgramTargetID string        `json:"program_target_id"`
	Project         Project       `json:"project"`
	Files           []File        `json:"files"`
	Declarations    []Declaration `json:"declarations"`
	Imports         []Import      `json:"imports"`
	Exports         []Export      `json:"exports"`
	Calls           []Call        `json:"calls"`
	Surfaces        []Surface     `json:"surfaces"`
	Routes          []Route       `json:"routes"`
	HTTPUses        []HTTPUse     `json:"http_uses"`
	Contracts       []Contract    `json:"contracts"`
	Resources       []Resource    `json:"resources"`
	ProductPaths    []ProductPath `json:"product_paths"`
	SHA256          string        `json:"sha256"`
}

func (result Result) Snapshot() Result {
	encoded, _ := json.Marshal(result)
	var snapshot Result
	_ = json.Unmarshal(encoded, &snapshot)
	return snapshot
}

func Seal(result Result) (Result, error) {
	result.Version = Version
	result.HelperVersion = HelperVersion
	omitPersistenceSensitiveOptionalMetadata(&result)
	canonicalize(&result)
	result.SHA256 = ""
	encoded, err := json.Marshal(result)
	if err != nil {
		return Result{}, fmt.Errorf("jsts project: seal: %w", err)
	}
	if err := rejectPersistenceSensitiveArtifact(encoded); err != nil {
		return Result{}, err
	}
	digest := sha256.Sum256(encoded)
	result.SHA256 = hex.EncodeToString(digest[:])
	if err := result.Validate(); err != nil {
		return Result{}, err
	}
	return result, nil
}

func omitPersistenceSensitiveOptionalMetadata(result *Result) {
	for index := range result.Declarations {
		signature := result.Declarations[index].Signature
		if _, sensitive := secretscan.DetectPersistenceSensitive(signature); sensitive || unsafeDeclarationSignature(signature) {
			result.Declarations[index].Signature = ""
		}
	}
	redactedCallRefs := make(map[string]struct{})
	for index := range result.Calls {
		if _, sensitive := secretscan.DetectPersistenceSensitive(result.Calls[index].Expression); sensitive {
			result.Calls[index].Expression = redactedExpression
			redactedCallRefs[result.Calls[index].Ref] = struct{}{}
		}
	}
	for pathIndex := range result.ProductPaths {
		result.ProductPaths[pathIndex].Frontier = ""
		for stepIndex := range result.ProductPaths[pathIndex].Steps {
			step := &result.ProductPaths[pathIndex].Steps[stepIndex]
			if _, redacted := redactedCallRefs[step.SourceRef]; redacted {
				step.Label = redactedExpression
			}
			if result.ProductPaths[pathIndex].Frontier == "" && step.Resolution == "unresolved" {
				result.ProductPaths[pathIndex].Frontier = step.Label
			}
		}
	}
}

func unsafeDeclarationSignature(value string) bool {
	return strings.Contains(value, "node_modules/") || strings.Contains(value, `import("/`)
}

func rejectPersistenceSensitiveArtifact(encoded []byte) error {
	if kind, sensitive := secretscan.DetectPersistenceSensitive(string(encoded)); sensitive {
		return fmt.Errorf("jsts project: persistence-sensitive artifact content (%s)", kind)
	}
	return nil
}

func (result Result) Validate() error {
	if result.Version != Version || result.HelperVersion != HelperVersion || !validSHA(result.CorpusSHA256) || !validSHA(result.SourceSHA256) || !validSHA(result.SHA256) || !strings.HasPrefix(result.ProgramTargetID, "program-target-") {
		return fmt.Errorf("jsts project: invalid producer identity")
	}
	if strings.TrimSpace(result.Project.Ref) == "" || strings.TrimSpace(result.Project.Name) == "" || path.Base(result.Project.ManifestPath) != "package.json" || strings.TrimSpace(result.Project.ManifestFileRef) == "" || !safeRepositoryPath(result.Project.ManifestPath) ||
		(result.Project.Language != "typescript" && result.Project.Language != "javascript") || result.Project.Selector != "jsts:"+result.Project.ManifestPath {
		return fmt.Errorf("jsts project: invalid selected project")
	}
	projectDir := path.Dir(result.Project.ManifestPath)
	projectOwnsPath := func(value string) bool {
		return projectDir == "." || value == projectDir || strings.HasPrefix(value, projectDir+"/")
	}
	if result.Project.ConfigPath != "" && !safeRepositoryPath(result.Project.ConfigPath) {
		return fmt.Errorf("jsts project: invalid config path")
	}
	if result.Project.ConfigPath != "" && !projectOwnsPath(result.Project.ConfigPath) {
		return fmt.Errorf("jsts project: config escapes selected package")
	}
	if (result.Project.ConfigPath == "") != (result.Project.ConfigFileRef == "") {
		return fmt.Errorf("jsts project: incomplete config identity")
	}
	if (result.Project.LockfilePath == "") != (result.Project.LockfileFileRef == "") {
		return fmt.Errorf("jsts project: incomplete lockfile identity")
	}
	if result.Project.LockfilePath != "" && !safeRepositoryPath(result.Project.LockfilePath) {
		return fmt.Errorf("jsts project: invalid lockfile path")
	}
	if result.Project.LockfilePath != "" && !projectOwnsPath(result.Project.LockfilePath) {
		return fmt.Errorf("jsts project: lockfile escapes selected package")
	}
	for _, alias := range result.Project.PathAliases {
		if strings.TrimSpace(alias.Pattern) == "" || alias.Targets == nil {
			return fmt.Errorf("jsts project: invalid path alias")
		}
		for _, target := range alias.Targets {
			if target == "" || strings.HasPrefix(target, "/") || strings.Contains(target, "\\") || corpus.ForbiddenPath(strings.TrimPrefix(target, "./")) {
				return fmt.Errorf("jsts project: forbidden path alias target")
			}
		}
	}
	binaryFiles := make(map[string]ProjectFile, len(result.Project.Binaries))
	binaryCommands := make(map[string]struct{}, len(result.Project.Binaries))
	for _, binary := range result.Project.Binaries {
		if !validPackageBinaryCommand(binary.Command) || !safeRepositoryPath(binary.Path) ||
			!projectOwnsPath(binary.Path) || strings.TrimSpace(binary.FileRef) == "" {
			return fmt.Errorf("jsts project: invalid package binary")
		}
		if _, duplicate := binaryCommands[binary.Command]; duplicate {
			return fmt.Errorf("jsts project: duplicate package binary command %q", binary.Command)
		}
		binaryCommands[binary.Command] = struct{}{}
		if previous, duplicate := binaryFiles[binary.FileRef]; duplicate && previous.Path != binary.Path {
			return fmt.Errorf("jsts project: package binary FileRef has conflicting paths")
		}
		binaryFiles[binary.FileRef] = ProjectFile{Path: binary.Path, FileRef: binary.FileRef}
	}
	fileRefs := map[string]File{}
	for _, file := range result.Files {
		if strings.TrimSpace(file.FileRef) == "" || !safeRepositoryPath(file.Path) || !projectOwnsPath(file.Path) || !validSHA(file.SHA256) || (file.Language != "javascript" && file.Language != "typescript") {
			return fmt.Errorf("jsts project: invalid source file")
		}
		if _, exists := fileRefs[file.FileRef]; exists {
			return fmt.Errorf("jsts project: duplicate source file ref %q", file.FileRef)
		}
		fileRefs[file.FileRef] = file
	}
	for fileRef, binary := range binaryFiles {
		if file, parsedSource := fileRefs[fileRef]; parsedSource && file.Path != binary.Path {
			return fmt.Errorf("jsts project: package binary source identity mismatch")
		}
	}
	if sourceDigest(result.Files) != result.SourceSHA256 {
		return fmt.Errorf("jsts project: source-byte identity mismatch")
	}
	for _, ref := range result.Project.EntryFileRefs {
		if _, ok := fileRefs[ref]; !ok {
			return fmt.Errorf("jsts project: project entry has unknown file ref")
		}
	}
	for _, config := range result.Project.ToolConfigs {
		if !safeRepositoryPath(config.Path) || !projectOwnsPath(config.Path) || strings.TrimSpace(config.FileRef) == "" {
			return fmt.Errorf("jsts project: invalid tool config")
		}
	}
	for _, script := range result.Project.Scripts {
		if strings.TrimSpace(script.Name) == "" || strings.TrimSpace(script.Kind) == "" || script.EntryFileRefs == nil {
			return fmt.Errorf("jsts project: invalid package script")
		}
		for _, ref := range script.EntryFileRefs {
			if _, ok := fileRefs[ref]; !ok {
				return fmt.Errorf("jsts project: script has unknown entry file ref")
			}
		}
	}
	for _, root := range result.Project.SourceRoots {
		if root != "." && (!safeRepositoryPath(root) || !projectOwnsPath(root)) {
			return fmt.Errorf("jsts project: invalid source root")
		}
	}
	for _, dependency := range result.Project.Dependencies {
		if strings.TrimSpace(dependency.PackagePath) == "" || dependency.PackagePath == javascriptPlatform ||
			strings.ContainsAny(dependency.PackagePath, "\x00\r\n") ||
			(dependency.Scope != "production" && dependency.Scope != "development" && dependency.Scope != "optional") {
			return fmt.Errorf("jsts project: invalid manifest dependency")
		}
	}
	declarations := map[string]struct{}{}
	for _, declaration := range result.Declarations {
		if declaration.Ref == "" || declaration.Name == "" || declaration.QualifiedName == "" {
			return fmt.Errorf("jsts project: invalid declaration identity")
		}
		if unsafeDeclarationSignature(declaration.Signature) {
			return fmt.Errorf("jsts project: invalid declaration signature for %q", declaration.Ref)
		}
		if !validLocation(declaration.Location, fileRefs) {
			return fmt.Errorf("jsts project: invalid declaration location for %q", declaration.Ref)
		}
		if _, exists := declarations[declaration.Ref]; exists {
			return fmt.Errorf("jsts project: duplicate declaration ref %q", declaration.Ref)
		}
		declarations[declaration.Ref] = struct{}{}
	}
	for _, declaration := range result.Declarations {
		if declaration.OwnerRef != "" {
			if _, ok := declarations[declaration.OwnerRef]; !ok {
				return fmt.Errorf("jsts project: unknown owner ref %q", declaration.OwnerRef)
			}
		}
	}
	factRefs := make(map[string]string, len(result.Imports)+len(result.Exports)+len(result.Calls)+len(result.Surfaces)+len(result.Routes)+len(result.HTTPUses)+len(result.Contracts)+len(result.Resources)+len(result.ProductPaths))
	registerFact := func(ref, kind string) error {
		if strings.TrimSpace(ref) == "" {
			return fmt.Errorf("jsts project: empty %s ref", kind)
		}
		if previous, exists := factRefs[ref]; exists {
			return fmt.Errorf("jsts project: duplicate fact ref %q (%s and %s)", ref, previous, kind)
		}
		if _, exists := declarations[ref]; exists {
			return fmt.Errorf("jsts project: fact ref collides with declaration %q", ref)
		}
		if _, exists := fileRefs[ref]; exists {
			return fmt.Errorf("jsts project: fact ref collides with file %q", ref)
		}
		if _, exists := binaryFiles[ref]; exists {
			return fmt.Errorf("jsts project: fact ref collides with package binary file %q", ref)
		}
		factRefs[ref] = kind
		return nil
	}
	for _, value := range result.Imports {
		if err := registerFact(value.Ref, "import"); err != nil {
			return err
		}
	}
	for _, value := range result.Exports {
		if err := registerFact(value.Ref, "export"); err != nil {
			return err
		}
	}
	for _, value := range result.Calls {
		if err := registerFact(value.Ref, "call"); err != nil {
			return err
		}
	}
	for _, value := range result.Surfaces {
		if err := registerFact(value.Ref, "surface"); err != nil {
			return err
		}
	}
	for _, value := range result.Routes {
		if err := registerFact(value.Ref, "route"); err != nil {
			return err
		}
	}
	for _, value := range result.HTTPUses {
		if err := registerFact(value.Ref, "http use"); err != nil {
			return err
		}
	}
	for _, value := range result.Contracts {
		if err := registerFact(value.Ref, "contract"); err != nil {
			return err
		}
	}
	for _, value := range result.Resources {
		if err := registerFact(value.Ref, "resource"); err != nil {
			return err
		}
	}
	for _, value := range result.ProductPaths {
		if err := registerFact(value.Ref, "product path"); err != nil {
			return err
		}
	}
	knownRef := func(ref string) bool {
		_, file := fileRefs[ref]
		_, binary := binaryFiles[ref]
		_, decl := declarations[ref]
		_, fact := factRefs[ref]
		return file || binary || decl || fact
	}
	knownDeclaration := func(ref string) bool { _, ok := declarations[ref]; return ok }
	for _, value := range result.Imports {
		if value.Ref == "" || value.Specifier == "" || !validLocation(value.Location, fileRefs) || value.ImporterFileRef != value.Location.FileRef || !validResolution(value.Resolution) {
			return fmt.Errorf("jsts project: invalid import")
		}
		if unsafeURLUserinfo(value.Specifier) || strings.Contains(value.Specifier, "://") {
			return fmt.Errorf("jsts project: credential-bearing import specifier")
		}
		if value.ExternalPackage == javascriptPlatform {
			return fmt.Errorf("jsts project: JavaScript platform authority cannot originate from an import")
		}
		if value.ResolvedFileRef != "" {
			if _, ok := fileRefs[value.ResolvedFileRef]; !ok {
				return fmt.Errorf("jsts project: import resolves outside corpus")
			}
		}
	}
	for _, value := range result.Exports {
		if value.Ref == "" || value.Name == "" || !validLocation(value.Location, fileRefs) || !validResolution(value.Resolution) {
			return fmt.Errorf("jsts project: invalid export")
		}
		if value.DeclarationRef != "" && !knownDeclaration(value.DeclarationRef) {
			return fmt.Errorf("jsts project: export has unknown declaration")
		}
		if value.ResolvedFileRef != "" {
			if _, ok := fileRefs[value.ResolvedFileRef]; !ok {
				return fmt.Errorf("jsts project: export resolves outside corpus")
			}
		}
	}
	for _, value := range result.Calls {
		if value.Ref == "" || value.CallerRef == "" || value.Expression == "" || !validInvocation(value.Invocation) || !validLocation(value.Location, fileRefs) || !validResolution(value.Resolution) {
			return fmt.Errorf("jsts project: invalid call")
		}
		if _, ok := declarations[value.CallerRef]; !ok {
			return fmt.Errorf("jsts project: call has unknown caller")
		}
		if (value.ExternalReceiver != "" || value.ExternalName != "") && value.ExternalPackage == "" {
			return fmt.Errorf("jsts project: call has external symbol without package authority")
		}
		if value.ExternalPackage != "" && len(value.CalleeRefs) != 0 {
			return fmt.Errorf("jsts project: call mixes local and external authority")
		}
		if value.ExternalPackage == javascriptPlatform && (value.ExternalName == "" || value.Resolution == "unresolved") {
			return fmt.Errorf("jsts project: invalid JavaScript platform call authority")
		}
		for _, ref := range value.CalleeRefs {
			if _, ok := declarations[ref]; !ok {
				return fmt.Errorf("jsts project: call has unknown callee")
			}
		}
	}
	expectedCLISurfaces := make(map[string]Surface, len(result.Project.Binaries))
	for _, binary := range result.Project.Binaries {
		surface := cliSurfaceForBinary(binary)
		expectedCLISurfaces[surface.Ref] = surface
	}
	seenCLISurfaces := make(map[string]struct{}, len(expectedCLISurfaces))
	for _, value := range result.Surfaces {
		if value.Ref == "" || value.Name == "" || !validSurfaceKind(value.Kind) || !validSurfaceRole(value.Role) || !validSurfaceLocation(value.Location, fileRefs, binaryFiles) {
			return fmt.Errorf("jsts project: invalid surface")
		}
		if value.Kind == SurfaceCLI {
			expected, ok := expectedCLISurfaces[value.Ref]
			if !ok || !reflect.DeepEqual(value, expected) {
				return fmt.Errorf("jsts project: CLI surface does not match exact package binary authority")
			}
			seenCLISurfaces[value.Ref] = struct{}{}
		}
		for _, ref := range append(append([]string{}, value.EntryRefs...), value.EvidenceRefs...) {
			if !knownRef(ref) {
				return fmt.Errorf("jsts project: surface has unknown evidence ref %q", ref)
			}
		}
	}
	if len(seenCLISurfaces) != len(expectedCLISurfaces) {
		return fmt.Errorf("jsts project: package binary authority is missing its CLI surface")
	}
	for _, value := range result.Routes {
		if value.Ref == "" || value.Path == "" || !validRouteKind(value.Kind) || !validLocation(value.Location, fileRefs) || !validResolution(value.Resolution) {
			return fmt.Errorf("jsts project: invalid route")
		}
		for _, ref := range append(append(append([]string{}, value.MiddlewareRefs...), value.HandlerRefs...), value.OwnerRef, value.ComponentRef) {
			if ref != "" && !knownDeclaration(ref) {
				return fmt.Errorf("jsts project: route has unknown program ref %q", ref)
			}
		}
	}
	for _, value := range result.HTTPUses {
		if value.Ref == "" || value.Method == "" || value.Path == "" || value.CallerRef == "" || !validLocation(value.Location, fileRefs) || !validResolution(value.Resolution) {
			return fmt.Errorf("jsts project: invalid HTTP use")
		}
		if !knownDeclaration(value.CallerRef) {
			return fmt.Errorf("jsts project: HTTP use has unknown caller")
		}
	}
	for _, value := range result.Contracts {
		if value.Ref == "" || value.Kind == "" || value.Name == "" || !validLocation(value.Location, fileRefs) {
			return fmt.Errorf("jsts project: invalid contract")
		}
		if value.DeclarationRef != "" && !knownDeclaration(value.DeclarationRef) {
			return fmt.Errorf("jsts project: contract has unknown declaration")
		}
		for _, ref := range value.UsedByRefs {
			if !knownDeclaration(ref) {
				return fmt.Errorf("jsts project: contract has unknown use ref")
			}
		}
	}
	for _, value := range result.Resources {
		if value.Ref == "" || value.Kind == "" || value.Name == "" || !validLocation(value.Location, fileRefs) {
			return fmt.Errorf("jsts project: invalid resource")
		}
		for _, ref := range value.UsedByRefs {
			if !knownDeclaration(ref) {
				return fmt.Errorf("jsts project: resource has unknown use ref")
			}
		}
		for _, ref := range value.EvidenceRefs {
			if !knownRef(ref) {
				return fmt.Errorf("jsts project: resource has unknown evidence ref")
			}
		}
	}
	for _, value := range result.ProductPaths {
		if value.Ref == "" || value.Name == "" || value.Outcome == "" || len(value.Steps) == 0 {
			return fmt.Errorf("jsts project: invalid product path")
		}
		for index, step := range value.Steps {
			if step.Ordinal != index+1 || !validPathStepKind(step.Kind) || step.Label == "" || step.SourceRef == "" || !validResolution(step.Resolution) || !validPathAuthority(step.Authority) || !compatiblePathAuthority(step.Resolution, step.Authority) || !validLocation(step.Location, fileRefs) {
				return fmt.Errorf("jsts project: invalid product path step")
			}
			if !knownRef(step.SourceRef) {
				return fmt.Errorf("jsts project: product path has unknown source ref %q", step.SourceRef)
			}
			for _, ref := range step.TargetRefs {
				if !knownRef(ref) {
					return fmt.Errorf("jsts project: product path has unknown target ref %q", ref)
				}
			}
		}
	}
	canonical := result.Snapshot()
	canonical.SHA256 = ""
	canonicalize(&canonical)
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return fmt.Errorf("jsts project: validate seal: %w", err)
	}
	digest := sha256.Sum256(encoded)
	if result.SHA256 != hex.EncodeToString(digest[:]) {
		return fmt.Errorf("jsts project: artifact SHA mismatch")
	}
	canonical.SHA256 = result.SHA256
	if !reflect.DeepEqual(result, canonical) {
		return fmt.Errorf("jsts project: artifact is not canonical")
	}
	return nil
}

func Encode(result Result) ([]byte, error) {
	if err := result.Validate(); err != nil {
		return nil, err
	}
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("jsts project: encode: %w", err)
	}
	if len(encoded) > MaxArtifactBytes {
		return nil, fmt.Errorf("jsts project: artifact exceeds %d bytes", MaxArtifactBytes)
	}
	if err := rejectPersistenceSensitiveArtifact(encoded); err != nil {
		return nil, err
	}
	return append(encoded, '\n'), nil
}

func Decode(encoded []byte) (Result, error) {
	if len(encoded) == 0 || len(encoded) > MaxArtifactBytes {
		return Result{}, fmt.Errorf("jsts project: invalid artifact size")
	}
	if err := rejectPersistenceSensitiveArtifact(encoded); err != nil {
		return Result{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var result Result
	if err := decoder.Decode(&result); err != nil {
		return Result{}, fmt.Errorf("jsts project: decode: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return Result{}, fmt.Errorf("jsts project: trailing JSON value")
		}
		return Result{}, fmt.Errorf("jsts project: trailing data: %w", err)
	}
	if err := result.Validate(); err != nil {
		return Result{}, err
	}
	return result, nil
}

func canonicalize(result *Result) {
	if result.Project.PathAliases == nil {
		result.Project.PathAliases = []PathAlias{}
	}
	for index := range result.Project.PathAliases {
		result.Project.PathAliases[index].Targets = canonicalStrings(result.Project.PathAliases[index].Targets)
	}
	sort.Slice(result.Project.PathAliases, func(i, j int) bool {
		return result.Project.PathAliases[i].Pattern < result.Project.PathAliases[j].Pattern
	})
	if result.Project.Scripts == nil {
		result.Project.Scripts = []Script{}
	}
	for i := range result.Project.Scripts {
		result.Project.Scripts[i].EntryFileRefs = canonicalStrings(result.Project.Scripts[i].EntryFileRefs)
	}
	sort.Slice(result.Project.Scripts, func(i, j int) bool { return result.Project.Scripts[i].Name < result.Project.Scripts[j].Name })
	if result.Project.Binaries == nil {
		result.Project.Binaries = []PackageBinary{}
	}
	sort.Slice(result.Project.Binaries, func(i, j int) bool {
		if result.Project.Binaries[i].Command != result.Project.Binaries[j].Command {
			return result.Project.Binaries[i].Command < result.Project.Binaries[j].Command
		}
		if result.Project.Binaries[i].Path != result.Project.Binaries[j].Path {
			return result.Project.Binaries[i].Path < result.Project.Binaries[j].Path
		}
		return result.Project.Binaries[i].FileRef < result.Project.Binaries[j].FileRef
	})
	result.Project.SourceRoots = canonicalStrings(result.Project.SourceRoots)
	result.Project.EntryFileRefs = canonicalStrings(result.Project.EntryFileRefs)
	if result.Project.ToolConfigs == nil {
		result.Project.ToolConfigs = []ProjectFile{}
	}
	sort.Slice(result.Project.ToolConfigs, func(i, j int) bool { return result.Project.ToolConfigs[i].Path < result.Project.ToolConfigs[j].Path })
	if result.Project.Dependencies == nil {
		result.Project.Dependencies = []PackageDependency{}
	}
	sort.Slice(result.Project.Dependencies, func(i, j int) bool {
		if result.Project.Dependencies[i].PackagePath != result.Project.Dependencies[j].PackagePath {
			return result.Project.Dependencies[i].PackagePath < result.Project.Dependencies[j].PackagePath
		}
		return result.Project.Dependencies[i].Scope < result.Project.Dependencies[j].Scope
	})
	if result.Files == nil {
		result.Files = []File{}
	}
	sort.Slice(result.Files, func(i, j int) bool { return result.Files[i].Path < result.Files[j].Path })
	if result.Declarations == nil {
		result.Declarations = []Declaration{}
	}
	sort.Slice(result.Declarations, func(i, j int) bool { return result.Declarations[i].Ref < result.Declarations[j].Ref })
	if result.Imports == nil {
		result.Imports = []Import{}
	}
	sort.Slice(result.Imports, func(i, j int) bool { return result.Imports[i].Ref < result.Imports[j].Ref })
	if result.Exports == nil {
		result.Exports = []Export{}
	}
	sort.Slice(result.Exports, func(i, j int) bool { return result.Exports[i].Ref < result.Exports[j].Ref })
	if result.Calls == nil {
		result.Calls = []Call{}
	}
	for i := range result.Calls {
		result.Calls[i].CalleeRefs = canonicalStrings(result.Calls[i].CalleeRefs)
	}
	sort.Slice(result.Calls, func(i, j int) bool { return result.Calls[i].Ref < result.Calls[j].Ref })
	if result.Surfaces == nil {
		result.Surfaces = []Surface{}
	}
	for i := range result.Surfaces {
		result.Surfaces[i].EntryRefs = canonicalStrings(result.Surfaces[i].EntryRefs)
		result.Surfaces[i].EvidenceRefs = canonicalStrings(result.Surfaces[i].EvidenceRefs)
	}
	sort.Slice(result.Surfaces, func(i, j int) bool { return result.Surfaces[i].Ref < result.Surfaces[j].Ref })
	if result.Routes == nil {
		result.Routes = []Route{}
	}
	for i := range result.Routes {
		result.Routes[i].MiddlewareRefs = canonicalStrings(result.Routes[i].MiddlewareRefs)
		result.Routes[i].HandlerRefs = canonicalStrings(result.Routes[i].HandlerRefs)
	}
	sort.Slice(result.Routes, func(i, j int) bool { return result.Routes[i].Ref < result.Routes[j].Ref })
	if result.HTTPUses == nil {
		result.HTTPUses = []HTTPUse{}
	}
	for i := range result.HTTPUses {
		result.HTTPUses[i].QueryKeys = canonicalStrings(result.HTTPUses[i].QueryKeys)
	}
	sort.Slice(result.HTTPUses, func(i, j int) bool { return result.HTTPUses[i].Ref < result.HTTPUses[j].Ref })
	if result.Contracts == nil {
		result.Contracts = []Contract{}
	}
	for i := range result.Contracts {
		result.Contracts[i].UsedByRefs = canonicalStrings(result.Contracts[i].UsedByRefs)
	}
	sort.Slice(result.Contracts, func(i, j int) bool { return result.Contracts[i].Ref < result.Contracts[j].Ref })
	if result.Resources == nil {
		result.Resources = []Resource{}
	}
	for i := range result.Resources {
		result.Resources[i].UsedByRefs = canonicalStrings(result.Resources[i].UsedByRefs)
		result.Resources[i].EvidenceRefs = canonicalStrings(result.Resources[i].EvidenceRefs)
	}
	sort.Slice(result.Resources, func(i, j int) bool { return result.Resources[i].Ref < result.Resources[j].Ref })
	if result.ProductPaths == nil {
		result.ProductPaths = []ProductPath{}
	}
	for i := range result.ProductPaths {
		for j := range result.ProductPaths[i].Steps {
			result.ProductPaths[i].Steps[j].TargetRefs = canonicalStrings(result.ProductPaths[i].Steps[j].TargetRefs)
		}
	}
	sort.Slice(result.ProductPaths, func(i, j int) bool { return result.ProductPaths[i].Ref < result.ProductPaths[j].Ref })
}

func canonicalStrings(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	result := append([]string(nil), values...)
	sort.Strings(result)
	write := 0
	for _, value := range result {
		if write == 0 || result[write-1] != value {
			result[write] = value
			write++
		}
	}
	return result[:write]
}

func validSHA(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
func safeRepositoryPath(value string) bool {
	return value != "" && value == path.Clean(value) && !strings.HasPrefix(value, "/") && !strings.Contains(value, "\\") && !corpus.ForbiddenPath(value)
}
func validLocation(value Location, files map[string]File) bool {
	file, ok := files[value.FileRef]
	return ok && file.Path == value.Path && value.Line > 0 && value.Column > 0 && safeRepositoryPath(value.Path)
}
func validSurfaceLocation(value Location, files map[string]File, binaries map[string]ProjectFile) bool {
	if validLocation(value, files) {
		return true
	}
	binary, ok := binaries[value.FileRef]
	return ok && binary.Path == value.Path && value.Line > 0 && value.Column > 0 && safeRepositoryPath(value.Path)
}
func validPackageBinaryCommand(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || len(value) > maxPackageBinaryCommandBytes ||
		value == "." || value == ".." || path.Base(value) != value || strings.Contains(value, "\\") {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}
func cliSurfaceForBinary(binary PackageBinary) Surface {
	digest := sha256.Sum256([]byte(binary.Command + "\x00" + binary.Path + "\x00" + binary.FileRef))
	return Surface{
		Ref:          "surface:cli:" + hex.EncodeToString(digest[:]),
		Kind:         SurfaceCLI,
		Role:         SurfaceProduct,
		Name:         binary.Command + " command-line application",
		EntryRefs:    []string{},
		EvidenceRefs: []string{},
		Location:     Location{Path: binary.Path, FileRef: binary.FileRef, Line: 1, Column: 1},
	}
}
func validResolution(value string) bool {
	return value == "exact" || value == "alternatives" || value == "unresolved"
}
func validInvocation(value string) bool {
	return value == "call" || value == "construct"
}
func validSurfaceKind(value SurfaceKind) bool {
	return value == SurfaceBrowser || value == SurfaceServer || value == SurfaceCLI || value == SurfaceShared || value == SurfaceTool || value == SurfaceUnknown
}
func validSurfaceRole(value SurfaceRole) bool {
	return value == SurfaceProduct || value == SurfaceSupporting || value == SurfaceScript || value == SurfaceUnclassified
}
func validRouteKind(value RouteKind) bool {
	return value == RouteBrowser || value == RouteBrowserLink || value == RouteHTTP || value == RouteMiddleware
}
func validPathAuthority(value string) bool {
	return value == "exact_static" || value == "resolved_indirect" || value == "possible" || value == "unresolved_frontier"
}
func compatiblePathAuthority(resolution, authority string) bool {
	return (resolution == "exact" && (authority == "exact_static" || authority == "resolved_indirect")) || (resolution == "alternatives" && authority == "possible") || (resolution == "unresolved" && authority == "unresolved_frontier")
}
func validPathStepKind(value string) bool {
	switch value {
	case "page_route", "render_target", "mutation_site", "program_call", "client_http_use", "http_method_path_match", "server_route", "middleware", "handler_factory", "handler", "contract_validation", "storage_call", "resource_boundary":
		return true
	}
	return false
}
func unsafeURLUserinfo(value string) bool {
	if !strings.Contains(value, "://") {
		return false
	}
	parsed, err := url.Parse(value)
	return err != nil || parsed.User != nil
}

func sourceDigest(files []File) string {
	digest := sha256.New()
	for _, file := range files {
		for _, field := range []string{file.Path, file.FileRef, file.SHA256} {
			_, _ = digest.Write([]byte(strconv.Itoa(len(field))))
			_, _ = digest.Write([]byte{0})
			_, _ = digest.Write([]byte(field))
		}
	}
	return hex.EncodeToString(digest.Sum(nil))
}
