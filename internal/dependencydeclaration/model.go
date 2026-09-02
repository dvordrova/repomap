// Package dependencydeclaration owns a language-neutral, sealed inventory of
// package-manager declarations. Declarations are deliberately separate from
// dependencies.Catalog: a manifest row is not evidence that repository code
// imported or called the declared package.
package dependencydeclaration

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/corpus"
)

const (
	Version = 2

	// AdvisoryResultBytes is a diagnostic usual size for the complete sealed
	// in-memory result. Crossing it never narrows or rejects declaration facts.
	AdvisoryResultBytes = 64 << 20

	AdvisorySources         = 4096
	AdvisoryPackages        = 16384
	AdvisoryStatements      = 65536
	AdvisoryIncludes        = 16384
	AdvisoryFrontiers       = 16384
	AdvisoryStringBytes     = 4096
	AdvisorySourceBytes     = 8 << 20
	AdvisoryTotalBytes      = 64 << 20
	AdvisoryStatementExtras = 128
)

type CoverageState string

const (
	CoverageComplete CoverageState = "complete"
	// CoverageFrontier is still a complete ledger: every locally observed
	// unsupported boundary is represented explicitly. Consumers may use the
	// retained positive declarations but must not infer absence beyond it.
	CoverageFrontier CoverageState = "frontier"
)

func (state CoverageState) Valid() bool {
	return state == CoverageComplete || state == CoverageFrontier
}

type SourceState string

const (
	SourceParsed   SourceState = "parsed"
	SourceFrontier SourceState = "frontier"
)

func (state SourceState) Valid() bool {
	return state == SourceParsed || state == SourceFrontier
}

type StatementKind string

const (
	StatementRequirement StatementKind = "requirement"
	StatementConstraint  StatementKind = "constraint"
)

func (kind StatementKind) Valid() bool {
	return kind == StatementRequirement || kind == StatementConstraint
}

type Role string

const (
	RoleRuntime     Role = "runtime"
	RoleOptional    Role = "optional"
	RoleDevelopment Role = "development"
	RoleTest        Role = "test"
	RoleBuild       Role = "build"
	RoleUnspecified Role = "unspecified"
)

func (role Role) Valid() bool {
	switch role {
	case RoleRuntime, RoleOptional, RoleDevelopment, RoleTest, RoleBuild, RoleUnspecified:
		return true
	default:
		return false
	}
}

// LocatorKind intentionally retains only a safe structural category. Raw
// URLs, credentials, absolute paths and package-index options never enter the
// in-memory declaration authority.
type LocatorKind string

const (
	LocatorRegistry       LocatorKind = "registry"
	LocatorVCS            LocatorKind = "vcs"
	LocatorURL            LocatorKind = "url"
	LocatorRepositoryPath LocatorKind = "repository_path"
	LocatorExternalPath   LocatorKind = "external_path"
	LocatorUnknown        LocatorKind = "unknown"
)

func (kind LocatorKind) Valid() bool {
	switch kind {
	case LocatorRegistry, LocatorVCS, LocatorURL, LocatorRepositoryPath,
		LocatorExternalPath, LocatorUnknown:
		return true
	default:
		return false
	}
}

type IncludeKind string

const (
	IncludeRequirement IncludeKind = "requirement"
	IncludeConstraint  IncludeKind = "constraint"
)

func (kind IncludeKind) Valid() bool {
	return kind == IncludeRequirement || kind == IncludeConstraint
}

type IncludeResolution string

const (
	IncludeResolved     IncludeResolution = "resolved"
	IncludeMissing      IncludeResolution = "missing"
	IncludeOutsideScope IncludeResolution = "outside_scope"
)

func (resolution IncludeResolution) Valid() bool {
	return resolution == IncludeResolved || resolution == IncludeMissing || resolution == IncludeOutsideScope
}

type FrontierKind string

const (
	FrontierSource    FrontierKind = "source"
	FrontierStatement FrontierKind = "statement"
	FrontierDirective FrontierKind = "directive"
)

func (kind FrontierKind) Valid() bool {
	return kind == FrontierSource || kind == FrontierStatement || kind == FrontierDirective
}

type FrontierReason string

const (
	FrontierUnsupportedSetupCFG        FrontierReason = "unsupported_setup_cfg"
	FrontierUnsupportedSetupPY         FrontierReason = "unsupported_setup_py"
	FrontierUnsupportedPipfile         FrontierReason = "unsupported_pipfile"
	FrontierDynamicDeclaration         FrontierReason = "dynamic_declaration"
	FrontierUnsupportedShape           FrontierReason = "unsupported_declaration_shape"
	FrontierPackageIdentityUnavailable FrontierReason = "package_identity_unavailable"
	FrontierUnsupportedRequirement     FrontierReason = "unsupported_requirement"
	FrontierUnsupportedOption          FrontierReason = "unsupported_requirement_option"
)

func (reason FrontierReason) Valid() bool {
	switch reason {
	case FrontierUnsupportedSetupCFG, FrontierUnsupportedSetupPY, FrontierUnsupportedPipfile,
		FrontierDynamicDeclaration, FrontierUnsupportedShape, FrontierPackageIdentityUnavailable,
		FrontierUnsupportedRequirement, FrontierUnsupportedOption:
		return true
	default:
		return false
	}
}

// Scope binds declarations to one exact language target and adapter-owned
// project boundary. AuthoritySHA256 is produced by the language adapter from
// its sealed target catalog and selected target, not from model output.
type Scope struct {
	Language        string `json:"language"`
	Ecosystem       string `json:"ecosystem"`
	RepositoryPath  string `json:"repository_path"`
	AuthoritySHA256 string `json:"authority_sha256"`
}

// Location is an exact position inside the bound Source. It is omitted when a
// parser cannot establish a position; consumers must never approximate one.
type Location struct {
	Line   int `json:"line"`
	Column int `json:"column"`
}

type Locator struct {
	Kind           LocatorKind `json:"kind"`
	Host           string      `json:"host,omitempty"`
	RepositoryPath string      `json:"repository_path,omitempty"`
}

type SourceInput struct {
	// Key is an adapter-local join key and is never persisted. This permits the
	// same corpus file to be interpreted in two explicit formats without
	// requiring later rows to copy a stable artifact identity.
	Key           string
	FileRef       corpus.FileID
	Path          string
	Format        string
	State         SourceState
	ContentSHA256 string
	ByteCount     int
}

type StatementInput struct {
	SourceKey        string
	Kind             StatementKind
	Role             Role
	Group            string
	Name             string
	NormalizedName   string
	Extras           []string
	Specifier        string
	Conditional      bool
	Locator          Locator
	Section          string
	Ordinal          int
	Location         *Location
	ExpressionSHA256 string
}

type IncludeInput struct {
	SourceKey        string
	TargetSourceKey  string
	Kind             IncludeKind
	Resolution       IncludeResolution
	Location         *Location
	ExpressionSHA256 string
}

type FrontierInput struct {
	SourceKey        string
	Kind             FrontierKind
	Reason           FrontierReason
	Section          string
	Ordinal          int
	Location         *Location
	ExpressionSHA256 string
}

type Input struct {
	CorpusSHA256       string
	ProgramIndexSHA256 string
	TargetID           string
	Scope              Scope
	Sources            []SourceInput
	Statements         []StatementInput
	Includes           []IncludeInput
	Frontiers          []FrontierInput
}

type Source struct {
	ID            string        `json:"id"`
	FileRef       corpus.FileID `json:"file_ref"`
	Path          string        `json:"path"`
	Format        string        `json:"format"`
	State         SourceState   `json:"state"`
	ContentSHA256 string        `json:"content_sha256"`
	ByteCount     int           `json:"byte_count"`
}

type Statement struct {
	ID               string        `json:"id"`
	SourceRef        string        `json:"source_ref"`
	Kind             StatementKind `json:"kind"`
	Role             Role          `json:"role"`
	Group            string        `json:"group,omitempty"`
	Name             string        `json:"name"`
	NormalizedName   string        `json:"normalized_name"`
	Extras           []string      `json:"extras"`
	Specifier        string        `json:"specifier,omitempty"`
	Conditional      bool          `json:"conditional"`
	Locator          Locator       `json:"locator"`
	Section          string        `json:"section"`
	Ordinal          int           `json:"ordinal"`
	Location         *Location     `json:"location,omitempty"`
	ExpressionSHA256 string        `json:"expression_sha256"`
}

// Package groups declarations only by adapter-established package-manager
// identity. Every original spelling and statement remains available.
type Package struct {
	ID             string      `json:"id"`
	Ecosystem      string      `json:"ecosystem"`
	Name           string      `json:"name"`
	NormalizedName string      `json:"normalized_name"`
	Names          []string    `json:"names"`
	Statements     []Statement `json:"statements"`
}

type Include struct {
	ID               string            `json:"id"`
	SourceRef        string            `json:"source_ref"`
	TargetSourceRef  string            `json:"target_source_ref,omitempty"`
	Kind             IncludeKind       `json:"kind"`
	Resolution       IncludeResolution `json:"resolution"`
	Location         *Location         `json:"location,omitempty"`
	ExpressionSHA256 string            `json:"expression_sha256"`
}

type Frontier struct {
	ID               string         `json:"id"`
	SourceRef        string         `json:"source_ref"`
	Kind             FrontierKind   `json:"kind"`
	Reason           FrontierReason `json:"reason"`
	Section          string         `json:"section,omitempty"`
	Ordinal          int            `json:"ordinal,omitempty"`
	Location         *Location      `json:"location,omitempty"`
	ExpressionSHA256 string         `json:"expression_sha256"`
}

type Coverage struct {
	State              CoverageState `json:"state"`
	SourcesObserved    int           `json:"sources_observed"`
	SourcesParsed      int           `json:"sources_parsed"`
	SourcesFrontier    int           `json:"sources_frontier"`
	PackagesRetained   int           `json:"packages_retained"`
	StatementsObserved int           `json:"statements_observed"`
	StatementsRetained int           `json:"statements_retained"`
	StatementsFrontier int           `json:"statements_frontier"`
	IncludesObserved   int           `json:"includes_observed"`
	IncludesResolved   int           `json:"includes_resolved"`
	IncludesFrontier   int           `json:"includes_frontier"`
	Boundaries         int           `json:"boundaries"`
}

type Result struct {
	Version            int        `json:"version"`
	CorpusSHA256       string     `json:"corpus_sha256"`
	ProgramIndexSHA256 string     `json:"program_index_sha256"`
	TargetID           string     `json:"target_id"`
	Scope              Scope      `json:"scope"`
	Sources            []Source   `json:"sources"`
	Packages           []Package  `json:"packages"`
	Includes           []Include  `json:"includes"`
	Frontiers          []Frontier `json:"frontiers"`
	Coverage           Coverage   `json:"coverage"`
	SHA256             string     `json:"sha256"`
}

// Build canonicalizes local adapter rows, restores all joins and seals the
// resulting declaration artifact. It does not read repository files or infer
// package semantics.
func Build(input Input) (Result, error) {
	if !validSHA256(input.CorpusSHA256) || !validSHA256(input.ProgramIndexSHA256) ||
		!plainValue(input.TargetID) {
		return Result{}, fmt.Errorf("dependency declarations: invalid input identity")
	}
	if err := validateScope(input.Scope); err != nil {
		return Result{}, err
	}
	if input.Sources == nil || input.Statements == nil || input.Includes == nil || input.Frontiers == nil {
		return Result{}, fmt.Errorf("dependency declarations: input inventories must be present")
	}
	sources, sourceByKey, err := buildSources(input.CorpusSHA256, input.Sources)
	if err != nil {
		return Result{}, err
	}
	packages, err := buildPackages(input.Scope.Ecosystem, input.Statements, sourceByKey)
	if err != nil {
		return Result{}, err
	}
	includes, err := buildIncludes(input.Includes, sourceByKey)
	if err != nil {
		return Result{}, err
	}
	frontiers, err := buildFrontiers(input.Frontiers, sourceByKey)
	if err != nil {
		return Result{}, err
	}
	result := Result{
		Version: Version, CorpusSHA256: input.CorpusSHA256,
		ProgramIndexSHA256: input.ProgramIndexSHA256, TargetID: input.TargetID,
		Scope: input.Scope, Sources: sources, Packages: packages,
		Includes: includes, Frontiers: frontiers,
	}
	result.Coverage = deriveCoverage(result.Sources, result.Packages, result.Includes, result.Frontiers)
	digest, err := resultDigest(result)
	if err != nil {
		return Result{}, err
	}
	result.SHA256 = digest
	if err := result.Validate(); err != nil {
		return Result{}, err
	}
	return result, nil
}

func buildSources(corpusSHA string, inputs []SourceInput) ([]Source, map[string]Source, error) {
	result := make([]Source, 0, len(inputs))
	byKey := make(map[string]Source, len(inputs))
	seenIdentity := make(map[string]struct{}, len(inputs))
	for _, input := range inputs {
		if !plainValue(input.Key) || !validFileRef(input.FileRef) || !repositoryPath(input.Path) ||
			!token(input.Format) || !input.State.Valid() || !validSHA256(input.ContentSHA256) ||
			input.ByteCount < 0 {
			return nil, nil, fmt.Errorf("dependency declarations: invalid source input")
		}
		if _, duplicate := byKey[input.Key]; duplicate {
			return nil, nil, fmt.Errorf("dependency declarations: duplicate source key %q", input.Key)
		}
		source := Source{
			FileRef: input.FileRef, Path: input.Path, Format: input.Format, State: input.State,
			ContentSHA256: input.ContentSHA256, ByteCount: input.ByteCount,
		}
		source.ID = identity("ddsrc-", struct {
			Version                     int
			CorpusSHA256                string
			FileRef                     corpus.FileID
			Path, Format, ContentSHA256 string
			ByteCount                   int
		}{Version, corpusSHA, source.FileRef, source.Path, source.Format, source.ContentSHA256, source.ByteCount})
		identityKey := string(source.FileRef) + "\x00" + source.Format
		if _, duplicate := seenIdentity[identityKey]; duplicate {
			return nil, nil, fmt.Errorf("dependency declarations: duplicate source projection")
		}
		seenIdentity[identityKey] = struct{}{}
		byKey[input.Key] = source
		result = append(result, source)
	}
	sort.Slice(result, func(i, j int) bool { return sourceKey(result[i]) < sourceKey(result[j]) })
	return result, byKey, nil
}

func buildPackages(ecosystem string, inputs []StatementInput, sources map[string]Source) ([]Package, error) {
	byPackage := make(map[string]*Package)
	seenStatements := make(map[string]struct{}, len(inputs))
	for _, input := range inputs {
		source, ok := sources[input.SourceKey]
		if !ok {
			return nil, fmt.Errorf("dependency declarations: statement has unknown source key %q", input.SourceKey)
		}
		statement := Statement{
			SourceRef: source.ID, Kind: input.Kind, Role: input.Role, Group: input.Group,
			Name: input.Name, NormalizedName: input.NormalizedName,
			Extras: canonicalStrings(input.Extras), Specifier: input.Specifier, Conditional: input.Conditional,
			Locator: input.Locator, Section: input.Section, Ordinal: input.Ordinal,
			Location: cloneLocation(input.Location), ExpressionSHA256: input.ExpressionSHA256,
		}
		if err := validateStatementShape(statement); err != nil {
			return nil, err
		}
		statement.ID = statementIdentity(statement)
		if _, duplicate := seenStatements[statement.ID]; duplicate {
			return nil, fmt.Errorf("dependency declarations: duplicate statement identity")
		}
		seenStatements[statement.ID] = struct{}{}
		packageKey := ecosystem + "\x00" + statement.NormalizedName
		value := byPackage[packageKey]
		if value == nil {
			value = &Package{Ecosystem: ecosystem, NormalizedName: statement.NormalizedName}
			byPackage[packageKey] = value
		}
		value.Names = append(value.Names, statement.Name)
		value.Statements = append(value.Statements, statement)
	}
	result := make([]Package, 0, len(byPackage))
	for _, value := range byPackage {
		value.Names = canonicalStrings(value.Names)
		value.Name = value.Names[0]
		sort.Slice(value.Statements, func(i, j int) bool {
			return statementKey(value.Statements[i]) < statementKey(value.Statements[j])
		})
		value.ID = packageIdentity(*value)
		result = append(result, *value)
	}
	sort.Slice(result, func(i, j int) bool { return packageKey(result[i]) < packageKey(result[j]) })
	return result, nil
}

func buildIncludes(inputs []IncludeInput, sources map[string]Source) ([]Include, error) {
	result := make([]Include, 0, len(inputs))
	seen := make(map[string]struct{}, len(inputs))
	for _, input := range inputs {
		source, ok := sources[input.SourceKey]
		if !ok {
			return nil, fmt.Errorf("dependency declarations: include has unknown source key %q", input.SourceKey)
		}
		value := Include{
			SourceRef: source.ID, Kind: input.Kind, Resolution: input.Resolution,
			Location: cloneLocation(input.Location), ExpressionSHA256: input.ExpressionSHA256,
		}
		if input.Resolution == IncludeResolved {
			target, ok := sources[input.TargetSourceKey]
			if !ok {
				return nil, fmt.Errorf("dependency declarations: resolved include has unknown target source")
			}
			value.TargetSourceRef = target.ID
		} else if input.TargetSourceKey != "" {
			return nil, fmt.Errorf("dependency declarations: unresolved include has a target source")
		}
		if err := validateIncludeShape(value); err != nil {
			return nil, err
		}
		value.ID = includeIdentity(value)
		if _, duplicate := seen[value.ID]; duplicate {
			return nil, fmt.Errorf("dependency declarations: duplicate include identity")
		}
		seen[value.ID] = struct{}{}
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return includeKey(result[i]) < includeKey(result[j]) })
	return result, nil
}

func buildFrontiers(inputs []FrontierInput, sources map[string]Source) ([]Frontier, error) {
	result := make([]Frontier, 0, len(inputs))
	seen := make(map[string]struct{}, len(inputs))
	for _, input := range inputs {
		source, ok := sources[input.SourceKey]
		if !ok {
			return nil, fmt.Errorf("dependency declarations: frontier has unknown source key %q", input.SourceKey)
		}
		value := Frontier{
			SourceRef: source.ID, Kind: input.Kind, Reason: input.Reason,
			Section: input.Section, Ordinal: input.Ordinal, Location: cloneLocation(input.Location),
			ExpressionSHA256: input.ExpressionSHA256,
		}
		if err := validateFrontierShape(value); err != nil {
			return nil, err
		}
		value.ID = frontierIdentity(value)
		if _, duplicate := seen[value.ID]; duplicate {
			return nil, fmt.Errorf("dependency declarations: duplicate frontier identity")
		}
		seen[value.ID] = struct{}{}
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return frontierKey(result[i]) < frontierKey(result[j]) })
	return result, nil
}

func deriveCoverage(sources []Source, packages []Package, includes []Include, frontiers []Frontier) Coverage {
	coverage := Coverage{
		State: CoverageComplete, SourcesObserved: len(sources), PackagesRetained: len(packages),
		IncludesObserved: len(includes), Boundaries: len(frontiers),
	}
	for _, source := range sources {
		if source.State == SourceParsed {
			coverage.SourcesParsed++
		} else {
			coverage.SourcesFrontier++
		}
	}
	for _, value := range packages {
		coverage.StatementsRetained += len(value.Statements)
	}
	for _, frontier := range frontiers {
		if frontier.Kind == FrontierStatement {
			coverage.StatementsFrontier++
		}
	}
	coverage.StatementsObserved = coverage.StatementsRetained + coverage.StatementsFrontier
	for _, include := range includes {
		if include.Resolution == IncludeResolved {
			coverage.IncludesResolved++
		} else {
			coverage.IncludesFrontier++
		}
	}
	coverage.Boundaries += coverage.IncludesFrontier
	if coverage.SourcesFrontier > 0 || coverage.Boundaries > 0 {
		coverage.State = CoverageFrontier
	}
	return coverage
}

func validateScope(scope Scope) error {
	if !token(scope.Language) || !token(scope.Ecosystem) || !repositoryDir(scope.RepositoryPath) ||
		!validSHA256(scope.AuthoritySHA256) {
		return fmt.Errorf("dependency declarations: invalid scope")
	}
	return nil
}

func validateStatementShape(value Statement) error {
	if !strings.HasPrefix(value.SourceRef, "ddsrc-") || !value.Kind.Valid() || !value.Role.Valid() ||
		!plainValue(value.Name) || !token(value.NormalizedName) || !plainValue(value.Section) ||
		value.Ordinal <= 0 || !validSHA256(value.ExpressionSHA256) ||
		(value.Group != "" && !plainValue(value.Group)) ||
		(value.Specifier != "" && (!plainValue(value.Specifier) || !safeSpecifier(value.Specifier))) ||
		value.Extras == nil {
		return fmt.Errorf("dependency declarations: invalid statement")
	}
	for index, extra := range value.Extras {
		if !plainValue(extra) || (index > 0 && value.Extras[index-1] >= extra) {
			return fmt.Errorf("dependency declarations: invalid statement extras")
		}
	}
	if err := validateLocator(value.Locator); err != nil {
		return err
	}
	return validateLocation(value.Location)
}

func validateLocator(locator Locator) error {
	if !locator.Kind.Valid() || (locator.Host != "" && !plainValue(locator.Host)) ||
		(locator.RepositoryPath != "" && !repositoryPath(locator.RepositoryPath)) {
		return fmt.Errorf("dependency declarations: invalid locator")
	}
	if locator.RepositoryPath != "" && locator.Kind != LocatorRepositoryPath {
		return fmt.Errorf("dependency declarations: locator path authority mismatch")
	}
	if locator.Host != "" && locator.Kind != LocatorURL && locator.Kind != LocatorVCS {
		return fmt.Errorf("dependency declarations: locator host authority mismatch")
	}
	return nil
}

func validateIncludeShape(value Include) error {
	if !strings.HasPrefix(value.SourceRef, "ddsrc-") || !value.Kind.Valid() ||
		!value.Resolution.Valid() || !validSHA256(value.ExpressionSHA256) {
		return fmt.Errorf("dependency declarations: invalid include")
	}
	if value.Resolution == IncludeResolved {
		if !strings.HasPrefix(value.TargetSourceRef, "ddsrc-") {
			return fmt.Errorf("dependency declarations: resolved include has no target")
		}
	} else if value.TargetSourceRef != "" {
		return fmt.Errorf("dependency declarations: unresolved include has a target")
	}
	return validateLocation(value.Location)
}

func validateFrontierShape(value Frontier) error {
	if !strings.HasPrefix(value.SourceRef, "ddsrc-") || !value.Kind.Valid() || !value.Reason.Valid() ||
		!validSHA256(value.ExpressionSHA256) || (value.Section != "" && !plainValue(value.Section)) ||
		value.Ordinal < 0 {
		return fmt.Errorf("dependency declarations: invalid frontier")
	}
	if value.Kind == FrontierSource && (value.Ordinal != 0 || value.Location != nil) {
		return fmt.Errorf("dependency declarations: source frontier has statement location")
	}
	return validateLocation(value.Location)
}

func validateLocation(location *Location) error {
	if location == nil {
		return nil
	}
	if location.Line <= 0 || location.Column <= 0 {
		return fmt.Errorf("dependency declarations: invalid exact location")
	}
	return nil
}

func packageIdentity(value Package) string {
	return identity("ddpkg-", struct {
		Version                   int
		Ecosystem, NormalizedName string
	}{
		Version, value.Ecosystem, value.NormalizedName,
	})
}

func statementIdentity(value Statement) string {
	copyValue := value
	copyValue.ID = ""
	return identity("ddstmt-", copyValue)
}

func includeIdentity(value Include) string {
	copyValue := value
	copyValue.ID = ""
	return identity("ddinc-", copyValue)
}

func frontierIdentity(value Frontier) string {
	copyValue := value
	copyValue.ID = ""
	return identity("ddfront-", copyValue)
}

func identity(prefix string, value any) string {
	encoded, _ := json.Marshal(value)
	digest := sha256.Sum256(encoded)
	return prefix + hex.EncodeToString(digest[:12])
}

func sourceKey(value Source) string {
	return value.Path + "\x00" + value.Format + "\x00" + value.ID
}

func packageKey(value Package) string {
	return value.Ecosystem + "\x00" + value.NormalizedName
}

func statementKey(value Statement) string {
	return value.SourceRef + "\x00" + value.Section + fmt.Sprintf("\x00%012d\x00", value.Ordinal) + string(value.Kind) + "\x00" + value.ID
}

func includeKey(value Include) string {
	line := 0
	if value.Location != nil {
		line = value.Location.Line
	}
	return value.SourceRef + fmt.Sprintf("\x00%012d\x00", line) + string(value.Kind) + "\x00" + value.ID
}

func frontierKey(value Frontier) string {
	return value.SourceRef + "\x00" + value.Section + fmt.Sprintf("\x00%012d\x00", value.Ordinal) + string(value.Kind) + "\x00" + value.ID
}

func canonicalStrings(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	if len(result) == 0 {
		return []string{}
	}
	write := 1
	for read := 1; read < len(result); read++ {
		if result[read] == result[write-1] {
			continue
		}
		result[write] = result[read]
		write++
	}
	return result[:write]
}

func cloneLocation(value *Location) *Location {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil && value == strings.ToLower(value)
}

func validFileRef(value corpus.FileID) bool {
	text := string(value)
	if len(text) < 2 || text[0] != 'f' || text[1] == '0' {
		return false
	}
	for _, character := range text[1:] {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func plainValue(value string) bool {
	if value == "" || !utf8.ValidString(value) || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) || character == '\u2028' || character == '\u2029' {
			return false
		}
	}
	return true
}

func safeSpecifier(value string) bool {
	for _, character := range value {
		if unicode.IsLetter(character) || unicode.IsDigit(character) || strings.ContainsRune("._-*+!<>=~^,|() ", character) {
			continue
		}
		return false
	}
	return true
}

func token(value string) bool {
	if !plainValue(value) {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') ||
			character == '_' || character == '-' || character == '.' {
			continue
		}
		return false
	}
	return true
}

func repositoryDir(value string) bool {
	return value == "" || repositoryPath(value)
}

func repositoryPath(value string) bool {
	return value != "" && path.Clean(value) == value &&
		!path.IsAbs(value) && value != "." && value != ".." && !strings.HasPrefix(value, "../") &&
		!strings.Contains(value, "\\") && !strings.ContainsRune(value, 0)
}
