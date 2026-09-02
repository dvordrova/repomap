// Package jstsproject owns deterministic JavaScript and TypeScript project
// discovery and the sealed adapter handoff used by the ordinary repomap path.
package jstsproject

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/secretscan"
)

const (
	Version       = 11
	HelperVersion = 13
	// AdvisoryResultBytes is the former adapter-result size threshold.
	// Crossing it is diagnostic only.
	AdvisoryResultBytes = 64 << 20
	redactedExpression  = "<persistence-sensitive expression omitted>"
	javascriptPlatform  = "platform:javascript"
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
	Ref  string `json:"ref"`
	Kind string `json:"kind"`
	Name string `json:"name"`
	// QualifiedName is optional legacy adapter/debug metadata. ProgramIndex
	// graph identity and presentation are derived from Ref, ownership, Name,
	// and Location; no semantic consumer may require or parse this field. When
	// present, it remains covered by the enclosing Result byte seal.
	QualifiedName string   `json:"qualified_name,omitempty"`
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
	Ref              string       `json:"ref"`
	CallerRef        string       `json:"caller_ref"`
	CalleeRefs       []string     `json:"callee_refs"`
	Invocation       string       `json:"invocation"`
	ExternalPackage  string       `json:"external_package,omitempty"`
	ExternalExport   string       `json:"external_export,omitempty"`
	ExternalReceiver string       `json:"external_receiver,omitempty"`
	ExternalName     string       `json:"external_name,omitempty"`
	Expression       string       `json:"expression"`
	Resolution       string       `json:"resolution"`
	Location         Location     `json:"location"`
	Pattern          *CallPattern `json:"pattern,omitempty"`
	PatternsObserved int          `json:"patterns_observed"`
}

// CallPattern retains only adapter-neutral syntax needed by later bounded
// pattern classification. It deliberately carries no framework or protocol
// meaning: the adapter records the terminal selector, exact local receiver
// authority when the compiler has it, and every ordered positional argument.
type CallPattern struct {
	Selector                 string                `json:"selector"`
	ResultRef                string                `json:"result_ref,omitempty"`
	ReceiverRef              string                `json:"receiver_ref,omitempty"`
	ReceiverOriginRefs       []string              `json:"receiver_origin_refs"`
	ReceiverOriginResolution string                `json:"receiver_origin_resolution,omitempty"`
	ReceiverOriginsObserved  int                   `json:"receiver_origins_observed"`
	Arguments                []CallPatternArgument `json:"arguments"`
	ArgumentsObserved        int                   `json:"arguments_observed"`
}

type CallPatternArgument struct {
	Position                int                         `json:"position"`
	Kind                    string                      `json:"kind"`
	Value                   string                      `json:"value,omitempty"`
	Parts                   []CallPatternPart           `json:"parts"`
	ObjectRefs              []string                    `json:"object_refs"`
	Resolution              string                      `json:"resolution,omitempty"`
	ObjectsObserved         int                         `json:"objects_observed"`
	ValueCandidates         []CallPatternValueCandidate `json:"value_candidates"`
	ValueCandidatesObserved int                         `json:"value_candidates_observed"`
}

type CallPatternValueCandidate struct {
	Kind           string            `json:"kind"`
	Value          string            `json:"value,omitempty"`
	Parts          []CallPatternPart `json:"parts"`
	Resolution     string            `json:"resolution"`
	SourceKind     string            `json:"source_kind"`
	SourceCallRef  string            `json:"source_call_ref"`
	SourcePosition int               `json:"source_position"`
}

type CallPatternPart struct {
	Kind string `json:"kind"`
	Text string `json:"text,omitempty"`
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

type Contract struct {
	Ref            string   `json:"ref"`
	Kind           string   `json:"kind"`
	Name           string   `json:"name"`
	Value          string   `json:"value,omitempty"`
	DeclarationRef string   `json:"declaration_ref,omitempty"`
	UsedByRefs     []string `json:"used_by_refs"`
	Location       Location `json:"location"`
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
	Contracts       []Contract    `json:"contracts"`
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
	if err := rejectPersistenceSensitiveResult(encoded); err != nil {
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
			result.Calls[index].Pattern = nil
			redactedCallRefs[result.Calls[index].Ref] = struct{}{}
		}
	}
	// Redacting one side of a chained call must not leave a receiver pointing
	// at a producer pattern that no longer survives in the artifact.
	availableResults := make(map[string]struct{})
	for index := range result.Calls {
		if pattern := result.Calls[index].Pattern; pattern != nil && pattern.ResultRef != "" {
			availableResults[pattern.ResultRef] = struct{}{}
		}
	}
	usedResults := make(map[string]struct{})
	for index := range result.Calls {
		pattern := result.Calls[index].Pattern
		if pattern == nil || pattern.ReceiverRef == "" {
			continue
		}
		if _, callResult := availableResults[pattern.ReceiverRef]; callResult {
			usedResults[pattern.ReceiverRef] = struct{}{}
		} else if strings.HasPrefix(pattern.ReceiverRef, "call-result:") {
			pattern.ReceiverRef = ""
		}
	}
	for index := range result.Calls {
		pattern := result.Calls[index].Pattern
		if pattern == nil || pattern.ResultRef == "" {
			continue
		}
		if _, used := usedResults[pattern.ResultRef]; !used {
			pattern.ResultRef = ""
		}
	}
	// A recovered actual-to-formal value is optional structural metadata. If
	// its source call was redacted, retaining the candidate would either leave
	// a dangling provenance ref or persist the same sensitive literal through
	// the destination argument.
	for callIndex := range result.Calls {
		pattern := result.Calls[callIndex].Pattern
		if pattern == nil {
			continue
		}
		for argumentIndex := range pattern.Arguments {
			argument := &pattern.Arguments[argumentIndex]
			retained := argument.ValueCandidates[:0]
			for _, candidate := range argument.ValueCandidates {
				if _, redacted := redactedCallRefs[candidate.SourceCallRef]; redacted {
					continue
				}
				retained = append(retained, candidate)
			}
			argument.ValueCandidates = retained
			argument.ValueCandidatesObserved = len(retained)
		}
	}
}

func unsafeDeclarationSignature(value string) bool {
	return strings.Contains(value, "node_modules/") || strings.Contains(value, `import("/`)
}

func rejectPersistenceSensitiveResult(encoded []byte) error {
	if kind, sensitive := secretscan.DetectPersistenceSensitive(string(encoded)); sensitive {
		return fmt.Errorf("jsts project: persistence-sensitive result content (%s)", kind)
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
		if declaration.Ref == "" || declaration.Name == "" {
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
	factRefs := make(map[string]string, len(result.Imports)+len(result.Exports)+len(result.Calls)+len(result.Surfaces)+len(result.Contracts))
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
	patternExternalOrigins := make(map[string]struct{})
	patternResults := make(map[string]struct{})
	callsByRef := make(map[string]Call, len(result.Calls))
	for _, value := range result.Calls {
		callsByRef[value.Ref] = value
		if value.Pattern != nil && value.Pattern.ResultRef != "" {
			expected := "call-result:" + value.Ref
			if value.Pattern.ResultRef != expected {
				return fmt.Errorf("jsts project: call has invalid result identity")
			}
			if _, duplicate := patternResults[value.Pattern.ResultRef]; duplicate {
				return fmt.Errorf("jsts project: duplicate call result identity")
			}
			patternResults[value.Pattern.ResultRef] = struct{}{}
		}
		if value.ExternalPackage == "" || value.ExternalName == "" || value.Resolution == "unresolved" {
			continue
		}
		patternExternalOrigins[externalProgramObjectRef(value.ExternalPackage, value.ExternalReceiver, value.ExternalName)] = struct{}{}
	}
	usedPatternResults := make(map[string]struct{})
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
	for _, value := range result.Contracts {
		if err := registerFact(value.Ref, "contract"); err != nil {
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
		if (value.ExternalExport != "" || value.ExternalReceiver != "" || value.ExternalName != "") && value.ExternalPackage == "" {
			return fmt.Errorf("jsts project: call has external symbol without package authority")
		}
		if value.ExternalPackage != "" && value.ExternalPackage != javascriptPlatform &&
			(value.ExternalExport == "" || value.ExternalName == "") && value.Resolution != "unresolved" {
			return fmt.Errorf("jsts project: resolved external call lacks package export authority")
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
		if value.Invocation == "call" {
			if value.PatternsObserved != 1 || !validCallPattern(
				value.Ref, value.Pattern, declarations, patternExternalOrigins, patternResults, callsByRef,
			) {
				return fmt.Errorf("jsts project: invalid call pattern")
			}
			if value.Pattern != nil {
				if _, callResult := patternResults[value.Pattern.ReceiverRef]; callResult {
					usedPatternResults[value.Pattern.ReceiverRef] = struct{}{}
				}
			}
		} else if value.PatternsObserved != 0 || value.Pattern != nil {
			return fmt.Errorf("jsts project: constructor has call pattern authority")
		}
	}
	if len(usedPatternResults) != len(patternResults) {
		return fmt.Errorf("jsts project: call result is not consumed by a retained receiver")
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
	canonical := result.Snapshot()
	canonical.SHA256 = ""
	canonicalize(&canonical)
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return fmt.Errorf("jsts project: validate seal: %w", err)
	}
	digest := sha256.Sum256(encoded)
	if result.SHA256 != hex.EncodeToString(digest[:]) {
		return fmt.Errorf("jsts project: result SHA mismatch")
	}
	canonical.SHA256 = result.SHA256
	if !reflect.DeepEqual(result, canonical) {
		return fmt.Errorf("jsts project: result is not canonical")
	}
	return nil
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
		if result.Calls[i].Pattern != nil {
			pattern := result.Calls[i].Pattern
			pattern.ReceiverOriginRefs = canonicalStrings(pattern.ReceiverOriginRefs)
			if pattern.Arguments == nil {
				pattern.Arguments = []CallPatternArgument{}
			}
			for j := range pattern.Arguments {
				pattern.Arguments[j].ObjectRefs = canonicalStrings(pattern.Arguments[j].ObjectRefs)
				if pattern.Arguments[j].Parts == nil {
					pattern.Arguments[j].Parts = []CallPatternPart{}
				}
				if pattern.Arguments[j].ValueCandidates == nil {
					pattern.Arguments[j].ValueCandidates = []CallPatternValueCandidate{}
				}
				for candidatePosition := range pattern.Arguments[j].ValueCandidates {
					if pattern.Arguments[j].ValueCandidates[candidatePosition].Parts == nil {
						pattern.Arguments[j].ValueCandidates[candidatePosition].Parts = []CallPatternPart{}
					}
				}
			}
		}
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
	if result.Contracts == nil {
		result.Contracts = []Contract{}
	}
	for i := range result.Contracts {
		result.Contracts[i].UsedByRefs = canonicalStrings(result.Contracts[i].UsedByRefs)
	}
	sort.Slice(result.Contracts, func(i, j int) bool { return result.Contracts[i].Ref < result.Contracts[j].Ref })
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
	if value == "" || !utf8.ValidString(value) || value != strings.TrimSpace(value) ||
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

func validCallPattern(
	ownerCallRef string,
	value *CallPattern,
	declarations, externalOrigins, callResults map[string]struct{},
	callsByRef map[string]Call,
) bool {
	// A computed call still counts as observed, but has no materialized
	// pattern because inventing a selector would create false authority.
	if value == nil {
		return true
	}
	if strings.TrimSpace(value.Selector) == "" ||
		value.ArgumentsObserved < len(value.Arguments) || value.ArgumentsObserved < 0 {
		return false
	}
	if value.ResultRef != "" {
		if _, ok := callResults[value.ResultRef]; !ok {
			return false
		}
	}
	if value.ReceiverRef != "" {
		if _, declaration := declarations[value.ReceiverRef]; !declaration {
			if _, callResult := callResults[value.ReceiverRef]; !callResult {
				return false
			}
		}
	}
	if value.ReceiverOriginsObserved < len(value.ReceiverOriginRefs) || value.ReceiverOriginsObserved < 0 {
		return false
	}
	if len(value.ReceiverOriginRefs) == 0 {
		if value.ReceiverOriginsObserved == 0 && value.ReceiverOriginResolution != "" ||
			value.ReceiverOriginsObserved > 0 && value.ReceiverOriginResolution != "unresolved" {
			return false
		}
	} else if !validResolution(value.ReceiverOriginResolution) {
		return false
	}
	for _, ref := range value.ReceiverOriginRefs {
		if _, local := declarations[ref]; local {
			continue
		}
		if _, external := externalOrigins[ref]; !external {
			return false
		}
	}
	for index, argument := range value.Arguments {
		if argument.Position != index+1 ||
			argument.ObjectsObserved < len(argument.ObjectRefs) || argument.ObjectsObserved < 0 {
			return false
		}
		if len(argument.ObjectRefs) == 0 {
			if argument.ObjectsObserved == 0 && argument.Resolution != "" ||
				argument.ObjectsObserved > 0 && argument.Resolution != "unresolved" {
				return false
			}
		} else if !validResolution(argument.Resolution) {
			return false
		}
		for _, ref := range argument.ObjectRefs {
			if _, ok := declarations[ref]; !ok {
				return false
			}
		}
		if argument.ValueCandidatesObserved != len(argument.ValueCandidates) ||
			len(argument.ValueCandidates) > 1 || len(argument.ValueCandidates) > 0 && argument.Kind != "dynamic" {
			return false
		}
		for _, candidate := range argument.ValueCandidates {
			if candidate.Resolution != "possible" || candidate.SourceKind != "actual_argument" ||
				candidate.SourceCallRef == "" || candidate.SourceCallRef == ownerCallRef || candidate.SourcePosition < 1 ||
				!validCallPatternValue(candidate.Kind, candidate.Value, candidate.Parts) {
				return false
			}
			source, ok := callsByRef[candidate.SourceCallRef]
			if !ok || source.Resolution != "exact" || len(source.CalleeRefs) != 1 || source.ExternalPackage != "" ||
				source.Pattern == nil || candidate.SourcePosition > len(source.Pattern.Arguments) {
				return false
			}
			sourceArgument := source.Pattern.Arguments[candidate.SourcePosition-1]
			if sourceArgument.Position != candidate.SourcePosition || sourceArgument.Kind != candidate.Kind ||
				sourceArgument.Value != candidate.Value || !reflect.DeepEqual(sourceArgument.Parts, candidate.Parts) ||
				(sourceArgument.Kind != "literal_string" && sourceArgument.Kind != "string_template") {
				return false
			}
		}
		switch argument.Kind {
		case "literal_string":
			if len(argument.Parts) != 0 {
				return false
			}
		case "string_template":
			if argument.Value != "" || len(argument.Parts) == 0 {
				return false
			}
			hasHole := false
			for _, part := range argument.Parts {
				switch part.Kind {
				case "literal":
					if part.Text == "" {
						return false
					}
				case "hole":
					if part.Text != "" {
						return false
					}
					hasHole = true
				default:
					return false
				}
			}
			if !hasHole {
				return false
			}
		case "dynamic":
			if argument.Value != "" || len(argument.Parts) != 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func validCallPatternValue(kind, value string, parts []CallPatternPart) bool {
	switch kind {
	case "literal_string":
		return len(parts) == 0
	case "string_template":
		if value != "" || len(parts) == 0 {
			return false
		}
		hasHole := false
		previousLiteral := false
		for _, part := range parts {
			if part.Kind == "literal" {
				if part.Text == "" || previousLiteral {
					return false
				}
				previousLiteral = true
				continue
			}
			if part.Kind != "hole" || part.Text != "" {
				return false
			}
			hasHole = true
			previousLiteral = false
		}
		return hasHole
	default:
		return false
	}
}
func validSurfaceKind(value SurfaceKind) bool {
	return value == SurfaceBrowser || value == SurfaceServer || value == SurfaceCLI || value == SurfaceShared || value == SurfaceTool || value == SurfaceUnknown
}
func validSurfaceRole(value SurfaceRole) bool {
	return value == SurfaceProduct || value == SurfaceSupporting || value == SurfaceScript || value == SurfaceUnclassified
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
