package report

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/gofacts"
)

const (
	surfaceCatalogFilename         = "trigger_catalog.json"
	surfaceCoverageFilename        = "surface_coverage.json"
	surfaceArtifactVersion         = 7
	previousSurfaceArtifactVersion = 6
	legacySurfaceArtifactVersion   = 5
	oldestSurfaceArtifactVersion   = 2
	surfaceSemanticCatalogVersion  = 1
	maxSurfaceArtifactBytes        = 4 * 1024 * 1024
	maxDiscoveredSurfaceTriggers   = 256
	maxSurfaceCoverageItems        = 128
	maxSurfaceNestedItems          = 32
	maxSurfaceValueCandidates      = 16
	maxDiscoveredSurfacePathBytes  = 4096
)

const (
	SurfaceProducerGeneric = "generic_surface_scan"
	SurfaceProducerCobra   = "deterministic_cobra"

	ExecutableRolePrimaryApplication = "primary_application"
	ExecutableRoleSecondaryService   = "secondary_service"
	ExecutableRoleTooling            = "tooling"
	ExecutableRoleTestOrHelper       = "test_or_helper"
	ExecutableRoleUnknown            = "unknown"
	// ExecutableRoleSecondaryTooling keeps source compatibility while new
	// artifacts use the role name exposed by the product.
	ExecutableRoleSecondaryTooling = ExecutableRoleTooling

	SurfaceAvailabilityAvailable   = "available"
	SurfaceAvailabilityUnavailable = "unavailable"
	SurfaceAvailabilityUnknown     = "unknown"

	SurfaceRoleEntrySurface    = "entry_surface"
	SurfaceRoleRuntimeActivity = "runtime_activity"
	SurfaceRoleDescriptor      = "descriptor"
	SurfaceRoleDynamicFrontier = "dynamic_frontier"
	SurfaceRoleRejected        = "rejected"
	SurfaceRoleNoisy           = "noisy"

	SurfaceTraceReady        = "trace_ready"
	SurfaceTracePartialReady = "partial_trace_ready"
	SurfaceTraceUnsupported  = "unsupported"
	SurfaceTraceRejected     = "rejected"

	SurfaceApplicationOwned     = "application_surface"
	SurfaceSupportingDependency = "supporting_dependency_behavior"
	SurfaceDependencyOnly       = "dependency_only"
)

// DiscoveredSurfaces is the bounded presentation projection of a paired
// trigger catalog and coverage artifact. Repository roots are intentionally
// absent: reports only retain repository-relative evidence locations.
type DiscoveredSurfaces struct {
	Version                     int                          `json:"version"`
	AnalyzerVersion             string                       `json:"analyzer_version"`
	ScenarioID                  string                       `json:"scenario_id"`
	ScopeStatement              string                       `json:"scope_statement"`
	TotalCount                  int                          `json:"total_count"`
	Truncated                   bool                         `json:"truncated,omitempty"`
	HTTPRouteCount              int                          `json:"http_route_count"`
	HTTPServerCount             int                          `json:"http_server_count"`
	HTTPRouteDescriptorCount    int                          `json:"http_route_descriptor_count"`
	HTTPRouteFrontierCount      int                          `json:"http_route_frontier_count"`
	DirectCount                 int                          `json:"direct_count"`
	WrapperCount                int                          `json:"wrapper_count"`
	WorkerCount                 int                          `json:"worker_count"`
	AsyncTaskCount              int                          `json:"async_task_count"`
	CLICommandCount             int                          `json:"cli_command_count"`
	CobraDescriptorCount        int                          `json:"cobra_descriptor_count"`
	CobraExactBindingCount      int                          `json:"cobra_exact_binding_count"`
	CobraExactActivationCount   int                          `json:"cobra_exact_activation_count"`
	CobraPartialRelationCount   int                          `json:"cobra_partial_relation_count"`
	CobraDuplicateRelationCount int                          `json:"cobra_duplicate_relation_count"`
	CobraRecordCount            int                          `json:"cobra_record_count"`
	CobraDroppedRecordCount     int                          `json:"cobra_dropped_record_count"`
	ProcessEntryCount           int                          `json:"process_entry_count"`
	GenericSurfaceCount         int                          `json:"generic_surface_count"`
	ApplicationCount            int                          `json:"application_count"`
	SecondaryServiceCount       int                          `json:"secondary_service_count"`
	ToolingCount                int                          `json:"tooling_count"`
	TestHelperCount             int                          `json:"test_helper_count"`
	UnassignedCount             int                          `json:"unassigned_count"`
	UnavailableSurfaceCount     int                          `json:"unavailable_surface_count"`
	PackageDiagnosticCount      int                          `json:"package_diagnostic_count"`
	UnavailablePackageCount     int                          `json:"unavailable_package_count"`
	SupportingDependencyCount   int                          `json:"supporting_dependency_count"`
	DependencyOnlyCount         int                          `json:"dependency_only_count"`
	DynamicFrontierCount        int                          `json:"dynamic_frontier_count"`
	PossibleRegistrationCount   int                          `json:"possible_registration_count"`
	UnresolvedHandlerCount      int                          `json:"unresolved_handler_count"`
	PackagesInspected           int                          `json:"packages_inspected"`
	FunctionsInspected          int                          `json:"functions_inspected"`
	EntrypointsConsidered       []SurfaceSymbol              `json:"entrypoints_considered"`
	ConfiguredSeedsMatched      []string                     `json:"configured_seeds_matched"`
	Triggers                    []DiscoveredTrigger          `json:"triggers"`
	LoopSignals                 []SurfaceLoopSignal          `json:"loop_signals"`
	DynamicFrontiers            []SurfaceFrontier            `json:"dynamic_frontiers"`
	UnsupportedDispatch         []SurfaceFrontier            `json:"unsupported_dispatch_mechanisms"`
	PackageDiagnostics          []SurfacePackageDiagnostic   `json:"package_diagnostics"`
	UnavailablePackages         []SurfacePackageAvailability `json:"unavailable_packages"`
	BudgetsReached              []string                     `json:"budgets_reached"`
}

// DiscoveredTrigger keeps the catalog's distinct semantic roles distinct.
// In particular, middleware, wrappers, evidence, and unresolved frontiers are
// not flattened into a handler or an invented execution chain.
type DiscoveredTrigger struct {
	ID                        string                     `json:"id"`
	ProvisionalID             bool                       `json:"provisional_id"`
	Kind                      string                     `json:"kind"`
	Producer                  string                     `json:"producer"`
	Identity                  SurfaceIdentity            `json:"identity"`
	Transport                 string                     `json:"transport"`
	Framework                 string                     `json:"framework"`
	ProcessEntrypoint         SurfaceSymbol              `json:"process_entrypoint"`
	Dispatcher                SurfaceValue               `json:"dispatcher"`
	Constructor               SurfaceSymbol              `json:"constructor,omitempty"`
	RegistrationSite          *SurfaceLocation           `json:"registration_site,omitempty"`
	DescriptorSite            *SurfaceLocation           `json:"descriptor_site,omitempty"`
	ServerStartSite           *SurfaceLocation           `json:"server_start_site,omitempty"`
	Handler                   SurfaceValue               `json:"handler"`
	HandlerLocation           *SurfaceLocation           `json:"handler_location,omitempty"`
	Middleware                []SurfaceValue             `json:"middleware"`
	WrapperChain              []SurfaceWrapper           `json:"wrapper_chain"`
	FinalSeed                 string                     `json:"final_seed"`
	DiscoveryBasis            string                     `json:"discovery_basis"`
	Certainty                 string                     `json:"certainty"`
	Resolution                string                     `json:"resolution"`
	Evidence                  []SurfaceEvidence          `json:"evidence"`
	Provenance                []SurfaceProvenance        `json:"provenance,omitempty"`
	DynamicFrontier           []SurfaceFrontier          `json:"dynamic_frontier"`
	Status                    string                     `json:"status"`
	OwningExecutable          string                     `json:"owning_executable,omitempty"`
	ExecutableRole            string                     `json:"executable_role"`
	Availability              string                     `json:"availability"`
	UnavailableReason         string                     `json:"unavailable_reason,omitempty"`
	TerminalSourceScope       string                     `json:"terminal_source_scope,omitempty"`
	ApplicationClass          string                     `json:"application_classification,omitempty"`
	PromotionBasis            string                     `json:"promotion_basis,omitempty"`
	OwningComponentID         componentmap.ComponentID   `json:"owning_component_id,omitempty"`
	ParticipatingComponentIDs []componentmap.ComponentID `json:"participating_component_ids,omitempty"`
	RelatedTraceID            componentmap.FlowID        `json:"related_saved_trace_id,omitempty"`
	TraceUnavailableReason    string                     `json:"trace_unavailable_reason,omitempty"`
	SurfaceRole               string                     `json:"surface_role"`
	TraceReadiness            string                     `json:"trace_readiness"`
	TraceReadinessReason      string                     `json:"trace_readiness_reason"`
	Quality                   SurfaceQuality             `json:"quality"`
}

type SurfaceQuality struct {
	Identity          string `json:"identity"`
	RegistrationStart string `json:"registration_start"`
	HandlerCallback   string `json:"handler_callback"`
	Reachability      string `json:"reachability"`
	Ownership         string `json:"ownership"`
	Traceability      string `json:"traceability"`
}

type SurfaceProvenance struct {
	Provider  string `json:"provider"`
	Version   string `json:"version"`
	Operation string `json:"operation"`
	Detail    string `json:"detail,omitempty"`
}

type SurfaceIdentity struct {
	Method string       `json:"method,omitempty"`
	Path   SurfaceValue `json:"path"`
	Name   string       `json:"name,omitempty"`
}

type SurfaceValue struct {
	Kind       string   `json:"kind"`
	Text       string   `json:"text,omitempty"`
	Known      bool     `json:"known"`
	Candidates []string `json:"candidates"`
}

type SurfaceSymbol struct {
	ID       string           `json:"id"`
	Package  string           `json:"package"`
	Name     string           `json:"name"`
	Location *SurfaceLocation `json:"location,omitempty"`
}

type SurfaceWrapper struct {
	Symbol   SurfaceSymbol    `json:"symbol"`
	Callsite *SurfaceLocation `json:"callsite,omitempty"`
	Origin   string           `json:"origin"`
}

type SurfaceLocation struct {
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Column int    `json:"column,omitempty"`
}

type SurfaceEvidence struct {
	ID       string           `json:"id"`
	Kind     string           `json:"kind"`
	Location *SurfaceLocation `json:"location,omitempty"`
	Detail   string           `json:"detail"`
}

type SurfaceFrontier struct {
	Kind     string           `json:"kind"`
	Detail   string           `json:"detail"`
	Location *SurfaceLocation `json:"location,omitempty"`
}

type SurfaceLoopSignal struct {
	Kind         string           `json:"kind"`
	FunctionID   string           `json:"function_id"`
	Location     *SurfaceLocation `json:"location,omitempty"`
	TerminalSeed string           `json:"terminal_seed,omitempty"`
	Detail       string           `json:"detail"`
	Certainty    string           `json:"certainty"`
}

type SurfacePackageDiagnostic struct {
	ID               string           `json:"id"`
	Kind             string           `json:"kind"`
	Message          string           `json:"message"`
	Package          string           `json:"package"`
	PackageName      string           `json:"package_name,omitempty"`
	OwningExecutable string           `json:"owning_executable,omitempty"`
	ExecutableRole   string           `json:"executable_role"`
	Availability     string           `json:"availability"`
	Location         *SurfaceLocation `json:"location,omitempty"`
}

type SurfacePackageAvailability struct {
	Package          string   `json:"package"`
	PackageName      string   `json:"package_name,omitempty"`
	OwningExecutable string   `json:"owning_executable,omitempty"`
	ExecutableRole   string   `json:"executable_role"`
	Availability     string   `json:"availability"`
	Reason           string   `json:"reason"`
	DiagnosticIDs    []string `json:"diagnostic_ids"`
}

type rawSurfaceCatalog struct {
	Version         int                  `json:"version"`
	AnalyzerVersion string               `json:"analyzer_version"`
	CatalogVersion  int                  `json:"catalog_version"`
	Repository      rawSurfaceRepository `json:"repository"`
	Scenario        rawSurfaceScenario   `json:"scenario"`
	Triggers        []rawSurfaceTrigger  `json:"triggers"`
}

type rawSurfaceCoverage struct {
	Version                     int                             `json:"version"`
	Repository                  rawSurfaceRepository            `json:"repository"`
	Scenario                    rawSurfaceScenario              `json:"scenario"`
	DirectTriggers              int                             `json:"direct_triggers"`
	WrapperDerivedTriggers      int                             `json:"wrapper_derived_triggers"`
	UnresolvedHandlers          int                             `json:"unresolved_handlers"`
	PossibleRegistrations       int                             `json:"possible_registrations"`
	Workers                     int                             `json:"workers"`
	AsyncTasks                  int                             `json:"async_tasks"`
	ProcessEntries              int                             `json:"process_entries"`
	CobraDescriptorCount        int                             `json:"cobra_descriptor_count"`
	CobraExactBindingCount      int                             `json:"cobra_exact_binding_count"`
	CobraExactActivationCount   int                             `json:"cobra_exact_activation_count"`
	CobraPartialRelationCount   int                             `json:"cobra_partial_relation_count"`
	CobraDuplicateRelationCount int                             `json:"cobra_duplicate_relation_count"`
	CobraRecordCount            int                             `json:"cobra_record_count"`
	CobraDroppedRecordCount     int                             `json:"cobra_dropped_record_count"`
	AvailableProcessEntries     int                             `json:"available_process_entries"`
	UnavailableProcessEntries   int                             `json:"unavailable_process_entries"`
	PackageDiagnosticCount      int                             `json:"package_diagnostic_count"`
	UnavailablePackageCount     int                             `json:"unavailable_package_count"`
	PackageDiagnostics          []rawSurfacePackageDiagnostic   `json:"package_diagnostics"`
	UnavailablePackages         []rawSurfacePackageAvailability `json:"unavailable_packages"`
	ConfiguredSeedsMatched      []string                        `json:"configured_seeds_matched"`
	PackagesInspected           int                             `json:"packages_inspected"`
	FunctionsInspected          int                             `json:"functions_inspected"`
	EntrypointsConsidered       []rawSurfaceSymbol              `json:"entrypoints_considered"`
	LoopSignals                 []rawSurfaceLoopSignal          `json:"loop_signals"`
	DynamicFrontiers            []rawSurfaceFrontier            `json:"dynamic_frontiers"`
	UnsupportedDispatch         []rawSurfaceFrontier            `json:"unsupported_dispatch_mechanisms"`
	BudgetsReached              []string                        `json:"budgets_reached"`
	ScopeStatement              string                          `json:"scope_statement"`
}

type rawSurfaceRepository struct {
	Root       string `json:"root"`
	ModulePath string `json:"module_path"`
}

type rawSurfaceScenario struct {
	ID      string   `json:"id"`
	GOOS    string   `json:"goos"`
	GOARCH  string   `json:"goarch"`
	Tags    []string `json:"tags"`
	GoFlags string   `json:"go_flags"`
}

type rawSurfaceTrigger struct {
	ID                   string                 `json:"id"`
	ProvisionalID        bool                   `json:"provisional_id"`
	Kind                 string                 `json:"kind"`
	Producer             string                 `json:"producer"`
	Identity             rawSurfaceIdentity     `json:"identity"`
	Transport            string                 `json:"transport"`
	Framework            string                 `json:"framework"`
	ProcessEntrypoint    rawSurfaceSymbol       `json:"process_entrypoint"`
	Dispatcher           rawSurfaceValue        `json:"dispatcher"`
	Constructor          rawSurfaceSymbol       `json:"constructor"`
	RegistrationSite     rawSurfaceLocation     `json:"registration_site"`
	DescriptorSite       *rawSurfaceLocation    `json:"descriptor_site"`
	ServerStartSite      *rawSurfaceLocation    `json:"server_start_site"`
	Handler              rawSurfaceValue        `json:"handler"`
	HandlerLocation      *rawSurfaceLocation    `json:"handler_location"`
	Middleware           []rawSurfaceValue      `json:"middleware"`
	WrapperChain         []rawSurfaceWrapper    `json:"wrapper_chain"`
	FinalSeed            string                 `json:"final_seed"`
	DiscoveryBasis       string                 `json:"discovery_basis"`
	Certainty            string                 `json:"certainty"`
	Resolution           string                 `json:"resolution"`
	ScenarioID           string                 `json:"scenario_id"`
	Evidence             []rawSurfaceEvidence   `json:"evidence"`
	Provenance           []rawSurfaceProvenance `json:"provenance"`
	DynamicFrontier      []rawSurfaceFrontier   `json:"dynamic_frontier"`
	Status               string                 `json:"status"`
	OwningExecutable     string                 `json:"owning_executable"`
	ExecutableRole       string                 `json:"executable_role"`
	Availability         string                 `json:"availability"`
	UnavailableReason    string                 `json:"unavailable_reason"`
	TerminalSourceScope  string                 `json:"terminal_source_scope"`
	ApplicationClass     string                 `json:"application_classification"`
	PromotionBasis       string                 `json:"promotion_basis"`
	SurfaceRole          string                 `json:"surface_role"`
	TraceReadiness       string                 `json:"trace_readiness"`
	TraceReadinessReason string                 `json:"trace_readiness_reason"`
	Quality              rawSurfaceQuality      `json:"quality"`
}

type rawSurfaceQuality struct {
	Identity          string `json:"identity"`
	RegistrationStart string `json:"registration_start"`
	HandlerCallback   string `json:"handler_callback"`
	Reachability      string `json:"reachability"`
	Ownership         string `json:"ownership"`
	Traceability      string `json:"traceability"`
}

type rawSurfaceProvenance struct {
	Provider  string `json:"provider"`
	Version   string `json:"version"`
	Operation string `json:"operation"`
	Detail    string `json:"detail"`
}

type rawSurfaceIdentity struct {
	Method string          `json:"method"`
	Path   rawSurfaceValue `json:"path"`
	Name   string          `json:"name"`
}

type rawSurfaceValue struct {
	Kind       string   `json:"kind"`
	Text       string   `json:"text"`
	Known      bool     `json:"known"`
	Candidates []string `json:"candidates"`
}

type rawSurfaceSymbol struct {
	ID       string             `json:"id"`
	Package  string             `json:"package"`
	Name     string             `json:"name"`
	Location rawSurfaceLocation `json:"location"`
}

type rawSurfaceWrapper struct {
	Symbol   rawSurfaceSymbol   `json:"symbol"`
	Callsite rawSurfaceLocation `json:"callsite"`
	Origin   string             `json:"origin"`
}

type rawSurfaceLocation struct {
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}

type rawSurfaceEvidence struct {
	ID       string             `json:"id"`
	Kind     string             `json:"kind"`
	Location rawSurfaceLocation `json:"location"`
	Detail   string             `json:"detail"`
}

type rawSurfaceFrontier struct {
	Kind     string              `json:"kind"`
	Detail   string              `json:"detail"`
	Location *rawSurfaceLocation `json:"location"`
}

type rawSurfaceLoopSignal struct {
	Kind         string             `json:"kind"`
	FunctionID   string             `json:"function_id"`
	Location     rawSurfaceLocation `json:"location"`
	TerminalSeed string             `json:"terminal_seed"`
	Detail       string             `json:"detail"`
	Certainty    string             `json:"certainty"`
}

type rawSurfacePackageDiagnostic struct {
	ID               string              `json:"id"`
	Kind             string              `json:"kind"`
	Message          string              `json:"message"`
	Package          string              `json:"package"`
	PackageName      string              `json:"package_name"`
	OwningExecutable string              `json:"owning_executable"`
	ExecutableRole   string              `json:"executable_role"`
	Availability     string              `json:"availability"`
	Location         *rawSurfaceLocation `json:"location"`
}

type rawSurfacePackageAvailability struct {
	Package          string   `json:"package"`
	PackageName      string   `json:"package_name"`
	OwningExecutable string   `json:"owning_executable"`
	ExecutableRole   string   `json:"executable_role"`
	Availability     string   `json:"availability"`
	Reason           string   `json:"reason"`
	DiagnosticIDs    []string `json:"diagnostic_ids"`
}

// parseDiscoveredSurfaces loads a complete v2 catalog/coverage pair. Missing
// artifacts are a valid legacy-run outcome. Any present but unusable pair is
// omitted and reported through one bounded warning.
func parseDiscoveredSurfaces(runDir string) (*DiscoveredSurfaces, []string) {
	catalogPath := filepath.Join(runDir, surfaceCatalogFilename)
	coveragePath := filepath.Join(runDir, surfaceCoverageFilename)

	hasCatalog, catalogWarning := surfaceArtifactExists(catalogPath, surfaceCatalogFilename)
	hasCoverage, coverageWarning := surfaceArtifactExists(coveragePath, surfaceCoverageFilename)
	if catalogWarning != "" {
		return nil, []string{catalogWarning}
	}
	if coverageWarning != "" {
		return nil, []string{coverageWarning}
	}
	if !hasCatalog && !hasCoverage {
		return nil, nil
	}
	if !hasCatalog {
		return nil, []string{"discovered surfaces: trigger_catalog.json is missing from an incomplete artifact pair"}
	}
	if !hasCoverage {
		return nil, []string{"discovered surfaces: surface_coverage.json is missing from an incomplete artifact pair"}
	}

	var catalog rawSurfaceCatalog
	if warning := readSurfaceArtifact(catalogPath, surfaceCatalogFilename, &catalog); warning != "" {
		return nil, []string{warning}
	}
	var coverage rawSurfaceCoverage
	if warning := readSurfaceArtifact(coveragePath, surfaceCoverageFilename, &coverage); warning != "" {
		return nil, []string{warning}
	}
	if warning := validateSurfaceArtifactPair(catalog, coverage); warning != "" {
		return nil, []string{warning}
	}

	return projectDiscoveredSurfaces(catalog, coverage), nil
}

func surfaceArtifactExists(path, name string) (bool, string) {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, ""
		}
		return true, fmt.Sprintf("discovered surfaces: cannot inspect %s", name)
	}
	if !info.Mode().IsRegular() {
		return true, fmt.Sprintf("discovered surfaces: %s is not a regular file", name)
	}
	return true, ""
}

func readSurfaceArtifact(path, name string, target any) string {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return fmt.Sprintf("discovered surfaces: cannot read %s", name)
	}
	if info.Size() < 0 || info.Size() > maxSurfaceArtifactBytes {
		return fmt.Sprintf(
			"discovered surfaces: %s exceeds the %d-byte limit",
			name,
			maxSurfaceArtifactBytes,
		)
	}
	file, err := os.Open(path)
	if err != nil {
		return fmt.Sprintf("discovered surfaces: cannot read %s", name)
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxSurfaceArtifactBytes+1))
	if err != nil {
		return fmt.Sprintf("discovered surfaces: cannot read %s", name)
	}
	if len(data) > maxSurfaceArtifactBytes {
		return fmt.Sprintf(
			"discovered surfaces: %s exceeds the %d-byte limit",
			name,
			maxSurfaceArtifactBytes,
		)
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Sprintf("discovered surfaces: %s contains invalid json", name)
	}
	return ""
}

func validateSurfaceArtifactPair(catalog rawSurfaceCatalog, coverage rawSurfaceCoverage) string {
	if !supportedSurfaceArtifactVersion(catalog.Version) {
		return fmt.Sprintf(
			"discovered surfaces: unsupported %s version %d (want %d)",
			surfaceCatalogFilename,
			catalog.Version,
			surfaceArtifactVersion,
		)
	}
	if !supportedSurfaceArtifactVersion(coverage.Version) {
		return fmt.Sprintf(
			"discovered surfaces: unsupported %s version %d (want %d)",
			surfaceCoverageFilename,
			coverage.Version,
			surfaceArtifactVersion,
		)
	}
	if catalog.CatalogVersion != surfaceSemanticCatalogVersion {
		return fmt.Sprintf(
			"discovered surfaces: unsupported terminal catalog version %d (want %d)",
			catalog.CatalogVersion,
			surfaceSemanticCatalogVersion,
		)
	}
	if catalog.Repository.Root == "" || coverage.Repository.Root == "" ||
		catalog.Repository != coverage.Repository {
		return "discovered surfaces: catalog and coverage repository identities do not match"
	}
	if !surfaceScenariosMatch(catalog.Scenario, coverage.Scenario) {
		return "discovered surfaces: catalog and coverage scenarios do not match"
	}
	if catalog.Scenario.ID == "" {
		return "discovered surfaces: catalog and coverage scenario id is missing"
	}

	seenIDs := make(map[string]struct{}, len(catalog.Triggers))
	for _, trigger := range catalog.Triggers {
		if strings.TrimSpace(trigger.ID) == "" {
			return "discovered surfaces: trigger id is missing"
		}
		if _, duplicate := seenIDs[trigger.ID]; duplicate {
			return "discovered surfaces: trigger ids are not unique"
		}
		seenIDs[trigger.ID] = struct{}{}
		if trigger.ScenarioID != catalog.Scenario.ID {
			return "discovered surfaces: a trigger scenario does not match the catalog scenario"
		}
	}
	return ""
}

func supportedSurfaceArtifactVersion(version int) bool {
	return version >= oldestSurfaceArtifactVersion && version <= surfaceArtifactVersion
}

func surfaceScenariosMatch(left, right rawSurfaceScenario) bool {
	return left.ID == right.ID &&
		left.GOOS == right.GOOS &&
		left.GOARCH == right.GOARCH &&
		left.GoFlags == right.GoFlags &&
		slices.Equal(left.Tags, right.Tags)
}

func projectDiscoveredSurfaces(
	catalog rawSurfaceCatalog,
	coverage rawSurfaceCoverage,
) *DiscoveredSurfaces {
	uniqueDynamicFrontiers := uniqueRawSurfaceFrontiers(coverage.DynamicFrontiers)
	rawTriggers := slices.Clone(catalog.Triggers)
	sort.SliceStable(rawTriggers, func(i, j int) bool {
		return rawTriggers[i].ID < rawTriggers[j].ID
	})
	totalCount := len(rawTriggers)
	if len(rawTriggers) > maxDiscoveredSurfaceTriggers {
		rawTriggers = rawTriggers[:maxDiscoveredSurfaceTriggers]
	}

	triggers := make([]DiscoveredTrigger, 0, len(rawTriggers))
	for _, trigger := range rawTriggers {
		triggers = append(triggers, projectDiscoveredTrigger(trigger))
	}
	httpRouteCount := 0
	httpServerCount := 0
	httpRouteDescriptorCount := 0
	httpRouteFrontierCount := 0
	processEntryCount := 0
	cliCommandCount := 0
	genericSurfaceCount := 0
	unavailableSurfaceCount := 0
	applicationCount := 0
	secondaryServiceCount := 0
	toolingCount := 0
	testHelperCount := 0
	unassignedCount := 0
	for _, trigger := range catalog.Triggers {
		if trigger.Producer == SurfaceProducerCobra {
			cliCommandCount++
		} else {
			genericSurfaceCount++
		}
		switch trigger.Kind {
		case "http_route":
			httpRouteCount++
		case "http_server":
			httpServerCount++
		case "http_route_descriptor":
			httpRouteDescriptorCount++
		case "http_route_frontier":
			httpRouteFrontierCount++
		case "process_entry":
			processEntryCount++
		}
		if trigger.Availability == SurfaceAvailabilityUnavailable {
			unavailableSurfaceCount++
		}
		switch normalizeSurfaceExecutableRole(trigger.ExecutableRole) {
		case ExecutableRolePrimaryApplication:
			applicationCount++
		case ExecutableRoleSecondaryService:
			secondaryServiceCount++
		case ExecutableRoleTooling:
			toolingCount++
		case ExecutableRoleTestOrHelper:
			testHelperCount++
		default:
			unassignedCount++
		}
	}
	return &DiscoveredSurfaces{
		Version:                     catalog.Version,
		AnalyzerVersion:             catalog.AnalyzerVersion,
		ScenarioID:                  catalog.Scenario.ID,
		ScopeStatement:              coverage.ScopeStatement,
		TotalCount:                  totalCount,
		Truncated:                   totalCount > len(triggers),
		HTTPRouteCount:              httpRouteCount,
		HTTPServerCount:             httpServerCount,
		HTTPRouteDescriptorCount:    httpRouteDescriptorCount,
		HTTPRouteFrontierCount:      httpRouteFrontierCount,
		ProcessEntryCount:           processEntryCount,
		CLICommandCount:             cliCommandCount,
		CobraDescriptorCount:        coverage.CobraDescriptorCount,
		CobraExactBindingCount:      coverage.CobraExactBindingCount,
		CobraExactActivationCount:   coverage.CobraExactActivationCount,
		CobraPartialRelationCount:   coverage.CobraPartialRelationCount,
		CobraDuplicateRelationCount: coverage.CobraDuplicateRelationCount,
		CobraRecordCount:            coverage.CobraRecordCount,
		CobraDroppedRecordCount:     coverage.CobraDroppedRecordCount,
		GenericSurfaceCount:         genericSurfaceCount,
		ApplicationCount:            applicationCount,
		SecondaryServiceCount:       secondaryServiceCount,
		ToolingCount:                toolingCount,
		TestHelperCount:             testHelperCount,
		UnassignedCount:             unassignedCount,
		UnavailableSurfaceCount:     unavailableSurfaceCount,
		PackageDiagnosticCount:      coverage.PackageDiagnosticCount,
		UnavailablePackageCount:     coverage.UnavailablePackageCount,
		DirectCount:                 coverage.DirectTriggers,
		WrapperCount:                coverage.WrapperDerivedTriggers,
		WorkerCount:                 coverage.Workers,
		AsyncTaskCount:              coverage.AsyncTasks,
		DynamicFrontierCount:        len(uniqueDynamicFrontiers),
		PossibleRegistrationCount:   coverage.PossibleRegistrations,
		UnresolvedHandlerCount:      coverage.UnresolvedHandlers,
		PackagesInspected:           coverage.PackagesInspected,
		FunctionsInspected:          coverage.FunctionsInspected,
		EntrypointsConsidered:       projectSurfaceSymbols(boundedSurfaceItems(coverage.EntrypointsConsidered, maxSurfaceCoverageItems)),
		ConfiguredSeedsMatched:      boundedSurfaceItems(coverage.ConfiguredSeedsMatched, maxSurfaceCoverageItems),
		Triggers:                    triggers,
		LoopSignals:                 projectSurfaceLoopSignals(boundedSurfaceItems(coverage.LoopSignals, maxSurfaceCoverageItems)),
		DynamicFrontiers: projectSurfaceFrontiers(boundedSurfaceItems(
			uniqueDynamicFrontiers,
			maxSurfaceCoverageItems,
		)),
		UnsupportedDispatch: projectSurfaceFrontiers(boundedSurfaceItems(
			uniqueRawSurfaceFrontiers(coverage.UnsupportedDispatch),
			maxSurfaceCoverageItems,
		)),
		PackageDiagnostics: projectSurfacePackageDiagnostics(boundedSurfaceItems(
			coverage.PackageDiagnostics,
			maxSurfaceCoverageItems,
		)),
		UnavailablePackages: projectSurfacePackageAvailability(boundedSurfaceItems(
			coverage.UnavailablePackages,
			maxSurfaceCoverageItems,
		)),
		BudgetsReached: boundedSurfaceItems(uniqueSurfaceStrings(coverage.BudgetsReached), maxSurfaceCoverageItems),
	}
}

func uniqueRawSurfaceFrontiers(frontiers []rawSurfaceFrontier) []rawSurfaceFrontier {
	seen := make(map[string]struct{}, len(frontiers))
	result := make([]rawSurfaceFrontier, 0, len(frontiers))
	for _, frontier := range frontiers {
		location := ""
		if frontier.Location != nil {
			location = fmt.Sprintf(
				"%s:%d:%d",
				frontier.Location.Path,
				frontier.Location.Line,
				frontier.Location.Column,
			)
		}
		key := frontier.Kind + "\x00" + frontier.Detail + "\x00" + location
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, frontier)
	}
	return result
}

func uniqueSurfaceStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, duplicate := seen[value]; duplicate {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func projectDiscoveredTrigger(trigger rawSurfaceTrigger) DiscoveredTrigger {
	projected := DiscoveredTrigger{
		ID:            trigger.ID,
		ProvisionalID: trigger.ProvisionalID,
		Kind:          trigger.Kind,
		Producer:      trigger.Producer,
		Identity: SurfaceIdentity{
			Method: trigger.Identity.Method,
			Path:   projectSurfaceValue(trigger.Identity.Path),
			Name:   trigger.Identity.Name,
		},
		Transport:            trigger.Transport,
		Framework:            trigger.Framework,
		ProcessEntrypoint:    projectSurfaceSymbol(trigger.ProcessEntrypoint),
		Dispatcher:           projectSurfaceValue(trigger.Dispatcher),
		Constructor:          projectSurfaceSymbol(trigger.Constructor),
		RegistrationSite:     projectSurfaceLocation(trigger.RegistrationSite),
		DescriptorSite:       projectOptionalSurfaceLocation(trigger.DescriptorSite),
		ServerStartSite:      projectOptionalSurfaceLocation(trigger.ServerStartSite),
		Handler:              projectSurfaceValue(trigger.Handler),
		HandlerLocation:      projectOptionalSurfaceLocation(trigger.HandlerLocation),
		Middleware:           projectSurfaceValues(boundedSurfaceItems(trigger.Middleware, maxSurfaceNestedItems)),
		WrapperChain:         projectSurfaceWrappers(boundedSurfaceItems(trigger.WrapperChain, maxSurfaceNestedItems)),
		FinalSeed:            trigger.FinalSeed,
		DiscoveryBasis:       trigger.DiscoveryBasis,
		Certainty:            trigger.Certainty,
		Resolution:           trigger.Resolution,
		Evidence:             projectSurfaceEvidence(boundedSurfaceItems(trigger.Evidence, maxSurfaceNestedItems)),
		Provenance:           projectSurfaceProvenance(boundedSurfaceItems(trigger.Provenance, maxSurfaceNestedItems)),
		DynamicFrontier:      projectSurfaceFrontiers(boundedSurfaceItems(trigger.DynamicFrontier, maxSurfaceNestedItems)),
		Status:               trigger.Status,
		OwningExecutable:     cleanSurfacePath(trigger.OwningExecutable),
		ExecutableRole:       normalizeSurfaceExecutableRole(trigger.ExecutableRole),
		Availability:         normalizeSurfaceAvailability(trigger.Availability),
		UnavailableReason:    trigger.UnavailableReason,
		TerminalSourceScope:  trigger.TerminalSourceScope,
		ApplicationClass:     normalizeSurfaceApplicationClass(trigger.ApplicationClass, trigger.DynamicFrontier),
		PromotionBasis:       trigger.PromotionBasis,
		SurfaceRole:          trigger.SurfaceRole,
		TraceReadiness:       trigger.TraceReadiness,
		TraceReadinessReason: trigger.TraceReadinessReason,
		Quality: SurfaceQuality{
			Identity: trigger.Quality.Identity, RegistrationStart: trigger.Quality.RegistrationStart,
			HandlerCallback: trigger.Quality.HandlerCallback, Reachability: trigger.Quality.Reachability,
			Ownership: trigger.Quality.Ownership, Traceability: trigger.Quality.Traceability,
		},
	}
	ensureProjectedSurfaceSemantics(&projected)
	return projected
}

func normalizeSurfaceExecutableRole(role string) string {
	switch role {
	case ExecutableRolePrimaryApplication, ExecutableRoleSecondaryService,
		ExecutableRoleTooling, ExecutableRoleTestOrHelper:
		return role
	case "secondary_tooling":
		return ExecutableRoleTooling
	case "":
		return ""
	default:
		return ExecutableRoleUnknown
	}
}

func normalizeSurfaceAvailability(availability string) string {
	switch availability {
	case SurfaceAvailabilityAvailable, SurfaceAvailabilityUnavailable:
		return availability
	case "":
		return ""
	default:
		return SurfaceAvailabilityUnknown
	}
}

func normalizeSurfaceApplicationClass(classification string, frontiers []rawSurfaceFrontier) string {
	switch classification {
	case SurfaceApplicationOwned, SurfaceSupportingDependency, SurfaceDependencyOnly:
		return classification
	}
	for _, frontier := range frontiers {
		if frontier.Kind == "entrypoint_dispatch_unresolved" {
			return SurfaceSupportingDependency
		}
	}
	return SurfaceApplicationOwned
}

func projectSurfaceProvenance(values []rawSurfaceProvenance) []SurfaceProvenance {
	result := make([]SurfaceProvenance, 0, len(values))
	for _, value := range values {
		result = append(result, SurfaceProvenance{
			Provider: value.Provider, Version: value.Version,
			Operation: value.Operation, Detail: value.Detail,
		})
	}
	return result
}

func mergeCommandSurfaceCatalog(data *ReportData) {
	if data == nil {
		return
	}
	if data.DiscoveredSurfaces == nil {
		data.DiscoveredSurfaces = &DiscoveredSurfaces{
			Version:         1,
			AnalyzerVersion: "unified-surface-catalog-v1",
			ScopeStatement:  "Build-selected deterministic command registrations and bounded generic registration/start analysis.",
		}
	}
	catalog := data.DiscoveredSurfaces
	wasTruncated := catalog.Truncated
	var typedCommands []DiscoveredTrigger
	for index := range catalog.Triggers {
		trigger := &catalog.Triggers[index]
		if trigger.Producer == "" {
			if isTypedCobraSurface(*trigger) {
				trigger.Producer = SurfaceProducerCobra
			} else {
				trigger.Producer = SurfaceProducerGeneric
			}
		}
		if trigger.ExecutableRole == "" {
			trigger.ExecutableRole = ExecutableRoleUnknown
		}
		if trigger.Availability == "" {
			trigger.Availability = SurfaceAvailabilityAvailable
		}
		ensureProjectedSurfaceSemantics(trigger)
		if isTypedCobraSurface(*trigger) {
			typedCommands = append(typedCommands, *trigger)
		}
	}
	if wasTruncated && (catalog.CLICommandCount > 0 || len(typedCommands) > 0) {
		// The retained trigger subset cannot prove that a legacy trace is not
		// equivalent to an omitted typed command. Keep the authoritative raw
		// counts and normalize only the retained records rather than risking a
		// duplicate count or replacing typed evidence hidden by truncation.
		sort.Slice(catalog.Triggers, func(i, j int) bool { return catalog.Triggers[i].ID < catalog.Triggers[j].ID })
		if len(catalog.Triggers) > maxDiscoveredSurfaceTriggers {
			catalog.Triggers = catalog.Triggers[:maxDiscoveredSurfaceTriggers]
		}
		refreshSurfaceCatalogCounts(catalog)
		return
	}

	seen := make(map[string]struct{}, len(catalog.Triggers)+len(data.CommandTraces))
	for _, trigger := range catalog.Triggers {
		seen[trigger.ID] = struct{}{}
	}
	addedCLICommands := 0
	addedGenericSurfaces := 0
	addedTriggerIDs := make(map[string]struct{})
	for _, trace := range data.CommandTraces {
		trigger := commandTraceSurface(data, trace)
		if trigger.ID == "" {
			continue
		}
		if _, duplicate := seen[trigger.ID]; duplicate {
			continue
		}
		if hasEquivalentTypedCobraSurface(typedCommands, trigger) {
			continue
		}
		seen[trigger.ID] = struct{}{}
		addedTriggerIDs[trigger.ID] = struct{}{}
		catalog.Triggers = append(catalog.Triggers, trigger)
		if trigger.Producer == SurfaceProducerCobra {
			addedCLICommands++
		} else {
			addedGenericSurfaces++
		}
	}
	sort.Slice(catalog.Triggers, func(i, j int) bool { return catalog.Triggers[i].ID < catalog.Triggers[j].ID })
	if wasTruncated {
		// Old and generic artifacts have no typed Cobra inventory to conflict
		// with legacy gofacts command traces. Their raw counts remain
		// authoritative, with only the accepted legacy additions applied.
		catalog.TotalCount += addedCLICommands + addedGenericSurfaces
		catalog.CLICommandCount += addedCLICommands
		catalog.GenericSurfaceCount += addedGenericSurfaces
		if len(catalog.Triggers) > maxDiscoveredSurfaceTriggers {
			bounded := make([]DiscoveredTrigger, 0, maxDiscoveredSurfaceTriggers)
			for _, trigger := range catalog.Triggers {
				if _, added := addedTriggerIDs[trigger.ID]; added {
					bounded = append(bounded, trigger)
					if len(bounded) == maxDiscoveredSurfaceTriggers {
						break
					}
				}
			}
			if len(bounded) < maxDiscoveredSurfaceTriggers {
				for _, trigger := range catalog.Triggers {
					if _, added := addedTriggerIDs[trigger.ID]; added {
						continue
					}
					bounded = append(bounded, trigger)
					if len(bounded) == maxDiscoveredSurfaceTriggers {
						break
					}
				}
			}
			catalog.Triggers = bounded
			sort.Slice(catalog.Triggers, func(i, j int) bool { return catalog.Triggers[i].ID < catalog.Triggers[j].ID })
		}
		refreshSurfaceCatalogCounts(catalog)
		return
	}
	if len(catalog.Triggers) > maxDiscoveredSurfaceTriggers {
		catalog.TotalCount = len(catalog.Triggers)
		catalog.CLICommandCount = 0
		catalog.GenericSurfaceCount = 0
		for _, trigger := range catalog.Triggers {
			if trigger.Producer == SurfaceProducerCobra {
				catalog.CLICommandCount++
			} else {
				catalog.GenericSurfaceCount++
			}
		}
		catalog.Truncated = true
		// Typed and generic surface discovery facts are the authoritative
		// catalog. Legacy traces only fill remaining presentation slots; they
		// must not evict a fact that was already present when the merge started.
		bounded := make([]DiscoveredTrigger, 0, maxDiscoveredSurfaceTriggers)
		for _, trigger := range catalog.Triggers {
			if _, added := addedTriggerIDs[trigger.ID]; added {
				continue
			}
			bounded = append(bounded, trigger)
			if len(bounded) == maxDiscoveredSurfaceTriggers {
				break
			}
		}
		if len(bounded) < maxDiscoveredSurfaceTriggers {
			for _, trigger := range catalog.Triggers {
				if _, added := addedTriggerIDs[trigger.ID]; !added {
					continue
				}
				bounded = append(bounded, trigger)
				if len(bounded) == maxDiscoveredSurfaceTriggers {
					break
				}
			}
		}
		catalog.Triggers = bounded
		sort.Slice(catalog.Triggers, func(i, j int) bool {
			return catalog.Triggers[i].ID < catalog.Triggers[j].ID
		})
	}
	refreshSurfaceCatalogCounts(catalog)
}

func isTypedCobraSurface(trigger DiscoveredTrigger) bool {
	if trigger.Kind != "cli_command" || trigger.Framework != "cobra" {
		return false
	}
	if trigger.Producer == SurfaceProducerCobra &&
		strings.HasPrefix(trigger.DiscoveryBasis, "build_selected_typed_cobra_") {
		return true
	}
	for _, provenance := range trigger.Provenance {
		if provenance.Operation == "discover_typed_cobra_command" ||
			provenance.Provider == "go_types_ast" &&
				(provenance.Operation == "detect_descriptor" ||
					provenance.Operation == "detect_binding" ||
					provenance.Operation == "detect_activation") {
			return true
		}
	}
	return false
}

func hasEquivalentTypedCobraSurface(
	typed []DiscoveredTrigger,
	legacy DiscoveredTrigger,
) bool {
	for _, candidate := range typed {
		if equivalentCobraCommandSurface(candidate, legacy) {
			return true
		}
	}
	return false
}

func equivalentCobraCommandSurface(left, right DiscoveredTrigger) bool {
	if left.Kind != "cli_command" || right.Kind != "cli_command" ||
		left.Framework != "cobra" || right.Framework != "cobra" {
		return false
	}
	if left.ProcessEntrypoint.Package != "" && right.ProcessEntrypoint.Package != "" &&
		left.ProcessEntrypoint.Package != right.ProcessEntrypoint.Package {
		return false
	}
	if left.Constructor.ID != "" && right.Constructor.ID != "" {
		return left.Constructor.ID == right.Constructor.ID
	}
	if left.Constructor.Package != "" && right.Constructor.Package != "" &&
		left.Constructor.Name != "" && right.Constructor.Name != "" {
		return left.Constructor.Package == right.Constructor.Package &&
			left.Constructor.Name == right.Constructor.Name
	}
	if sameSurfaceSourceLine(left.Constructor.Location, right.Constructor.Location) {
		return true
	}
	leftCommand := cobraCommandLeaf(left)
	rightCommand := cobraCommandLeaf(right)
	if leftCommand == "" || rightCommand == "" || leftCommand != rightCommand {
		return false
	}
	return sameSurfaceSourceLine(left.RegistrationSite, right.RegistrationSite) ||
		sameSurfaceSourceLine(left.HandlerLocation, right.HandlerLocation)
}

func cobraCommandLeaf(trigger DiscoveredTrigger) string {
	value := strings.TrimSpace(trigger.Identity.Name)
	if value == "" {
		value = strings.TrimSpace(trigger.Identity.Path.Text)
	}
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return ""
	}
	return fields[len(fields)-1]
}

func sameSurfaceSourceLine(left, right *SurfaceLocation) bool {
	return left != nil && right != nil && left.Path != "" && left.Line > 0 &&
		left.Path == right.Path && left.Line == right.Line
}

func commandTraceSurface(data *ReportData, trace gofacts.CommandTrace) DiscoveredTrigger {
	if trace.Framework != "cobra" || strings.TrimSpace(trace.Command) == "" {
		return DiscoveredTrigger{}
	}
	entrypoint, ok := commandTraceStep(trace, "entrypoint")
	if !ok {
		return DiscoveredTrigger{}
	}
	root, _ := commandTraceStep(trace, "calls")
	constructor, constructorOK := commandTraceStep(trace, "registers_command")
	callback, callbackOK := commandTraceStep(trace, "callback")
	id, command, constructorDerivedIdentity := gofacts.CommandSurfaceIdentity(trace)
	if id == "" {
		return DiscoveredTrigger{}
	}
	registration := surfaceLocationFromEvidence(constructor.CallsiteLocation)
	constructorLocation := surfaceLocationFromEvidence(&constructor.TargetLocation)
	entrypointLocation := surfaceLocationFromEvidence(&entrypoint.TargetLocation)
	handlerLocation := surfaceLocationFromEvidence(&callback.TargetLocation)
	status := "confirmed_command_registration"
	resolution := "exact"
	if !trace.Complete || !constructorOK || !callbackOK {
		status = "partial_command_registration"
		resolution = "partial"
	}
	identityKind := "command"
	identityKnown := true
	discoveryBasis := "build_selected_cobra_registration"
	if constructorDerivedIdentity {
		identityKind = "constructor_derived_command"
		identityKnown = false
		discoveryBasis = "build_selected_cobra_registration_constructor_derived_identity"
	}
	trigger := DiscoveredTrigger{
		ID: id, Kind: "cli_command", Producer: SurfaceProducerCobra,
		Identity: SurfaceIdentity{Name: command, Path: SurfaceValue{
			Kind: identityKind, Text: command, Known: identityKnown, Candidates: []string{command},
		}},
		Transport: "cli", Framework: trace.Framework,
		ProcessEntrypoint: SurfaceSymbol{
			ID:      trace.EntrypointPackage + "." + entrypoint.Symbol,
			Package: trace.EntrypointPackage, Name: entrypoint.Symbol, Location: entrypointLocation,
		},
		Dispatcher: SurfaceValue{Kind: "function", Text: root.Symbol, Known: root.Symbol != ""},
		Constructor: SurfaceSymbol{
			ID:      trace.EntrypointPackage + "." + constructor.Symbol,
			Package: trace.EntrypointPackage, Name: constructor.Symbol, Location: constructorLocation,
		},
		RegistrationSite: registration,
		Handler:          SurfaceValue{Kind: "function", Text: callback.Symbol, Known: callback.Symbol != ""},
		HandlerLocation:  handlerLocation,
		DiscoveryBasis:   discoveryBasis,
		Certainty:        "static", Resolution: resolution, Status: status,
		OwningExecutable:    surfaceExecutableForPackage(data, trace.EntrypointPackage),
		ExecutableRole:      ExecutableRoleUnknown,
		Availability:        SurfaceAvailabilityAvailable,
		TerminalSourceScope: "repository",
		ApplicationClass:    SurfaceApplicationOwned,
		PromotionBasis:      "repository_registration",
		Provenance: []SurfaceProvenance{{
			Provider: "gofacts", Version: fmt.Sprintf("command-trace-v%d", trace.Version),
			Operation: "build_selected_cobra_registration",
			Detail:    "exact AddCommand registration and package-local callback syntax",
		}},
	}
	for _, step := range trace.Steps {
		if location := surfaceLocationFromEvidence(step.CallsiteLocation); location != nil {
			trigger.Evidence = append(trigger.Evidence, SurfaceEvidence{
				ID:   stableSurfaceID(id, step.Relation, "callsite", surfaceLocationKeyValue(location)),
				Kind: step.Relation + "_callsite", Location: location,
				Detail: "exact " + step.Relation + " callsite for " + step.Symbol,
			})
		}
		if location := surfaceLocationFromEvidence(&step.TargetLocation); location != nil {
			trigger.Evidence = append(trigger.Evidence, SurfaceEvidence{
				ID:   stableSurfaceID(id, step.Relation, "target", surfaceLocationKeyValue(location)),
				Kind: step.Relation + "_target", Location: location,
				Detail: "exact declaration for " + step.Symbol,
			})
		}
	}
	ensureProjectedSurfaceSemantics(&trigger)
	return trigger
}

func commandTraceStep(trace gofacts.CommandTrace, relation string) (gofacts.CommandTraceStep, bool) {
	for _, step := range trace.Steps {
		if step.Relation == relation {
			return step, true
		}
	}
	return gofacts.CommandTraceStep{}, false
}

func surfaceLocationFromEvidence(location *evidence.Location) *SurfaceLocation {
	if location == nil || location.Line <= 0 || !validDiscoveredSurfacePath(location.Path) {
		return nil
	}
	return &SurfaceLocation{Path: location.Path, Line: location.Line, Column: location.Column}
}

func surfaceLocationKeyValue(location *SurfaceLocation) string {
	if location == nil {
		return ""
	}
	return fmt.Sprintf("%s:%d:%d", location.Path, location.Line, location.Column)
}

func stableSurfaceID(parts ...string) string {
	hash := sha256.New()
	for _, part := range parts {
		fmt.Fprintf(hash, "%d:%s\n", len(part), part)
	}
	return "surface-" + fmt.Sprintf("%x", hash.Sum(nil))[:24]
}

func surfaceExecutableForPackage(data *ReportData, packagePath string) string {
	if data != nil && data.RepositoryGraph != nil {
		for _, pkg := range data.RepositoryGraph.Packages {
			if pkg.CanonicalPath == packagePath {
				return firstNonEmpty(pkg.DisplayPath, pkg.ModuleRelativeDir, pkg.Dir, packagePath)
			}
		}
	}
	return packagePath
}

func refreshSurfaceCatalogCounts(catalog *DiscoveredSurfaces) {
	if catalog == nil {
		return
	}
	rawTotalCount := catalog.TotalCount
	rawCLICommandCount := catalog.CLICommandCount
	rawGenericSurfaceCount := catalog.GenericSurfaceCount
	rawUnavailableSurfaceCount := catalog.UnavailableSurfaceCount
	rawApplicationCount := catalog.ApplicationCount
	rawSecondaryServiceCount := catalog.SecondaryServiceCount
	rawToolingCount := catalog.ToolingCount
	rawTestHelperCount := catalog.TestHelperCount
	rawUnassignedCount := catalog.UnassignedCount
	rawSupportingDependencyCount := catalog.SupportingDependencyCount
	rawDependencyOnlyCount := catalog.DependencyOnlyCount
	preserveRawCounts := catalog.Truncated
	if !preserveRawCounts {
		catalog.TotalCount = len(catalog.Triggers)
	}
	if !preserveRawCounts {
		catalog.CLICommandCount = 0
		catalog.ProcessEntryCount = 0
		catalog.GenericSurfaceCount = 0
		catalog.HTTPRouteCount = 0
		catalog.HTTPServerCount = 0
		catalog.HTTPRouteDescriptorCount = 0
		catalog.HTTPRouteFrontierCount = 0
		catalog.UnavailableSurfaceCount = 0
	} else {
		catalog.UnavailableSurfaceCount = rawUnavailableSurfaceCount
	}
	catalog.ApplicationCount = 0
	catalog.SecondaryServiceCount = 0
	catalog.ToolingCount = 0
	catalog.TestHelperCount = 0
	catalog.UnassignedCount = 0
	catalog.SupportingDependencyCount = 0
	catalog.DependencyOnlyCount = 0
	for _, trigger := range catalog.Triggers {
		switch trigger.Kind {
		case "http_route":
			if !preserveRawCounts {
				catalog.HTTPRouteCount++
			}
		case "http_server":
			if !preserveRawCounts {
				catalog.HTTPServerCount++
			}
		case "http_route_descriptor":
			if !preserveRawCounts {
				catalog.HTTPRouteDescriptorCount++
			}
		case "http_route_frontier":
			if !preserveRawCounts {
				catalog.HTTPRouteFrontierCount++
			}
		case "process_entry":
			if !preserveRawCounts {
				catalog.ProcessEntryCount++
			}
		}
		switch trigger.Producer {
		case SurfaceProducerCobra:
			if !preserveRawCounts {
				catalog.CLICommandCount++
			}
		default:
			if !preserveRawCounts {
				catalog.GenericSurfaceCount++
			}
		}
		switch trigger.ApplicationClass {
		case SurfaceSupportingDependency:
			catalog.SupportingDependencyCount++
		case SurfaceDependencyOnly:
			catalog.DependencyOnlyCount++
		default:
			switch trigger.ExecutableRole {
			case ExecutableRolePrimaryApplication:
				catalog.ApplicationCount++
			case ExecutableRoleSecondaryService:
				catalog.SecondaryServiceCount++
			case ExecutableRoleTooling, "secondary_tooling":
				catalog.ToolingCount++
			case ExecutableRoleTestOrHelper:
				catalog.TestHelperCount++
			default:
				catalog.UnassignedCount++
			}
		}
		if !preserveRawCounts && trigger.Availability == SurfaceAvailabilityUnavailable {
			catalog.UnavailableSurfaceCount++
		}
	}
	if preserveRawCounts {
		catalog.TotalCount = rawTotalCount
		catalog.CLICommandCount = rawCLICommandCount
		catalog.GenericSurfaceCount = rawGenericSurfaceCount
		catalog.ApplicationCount = rawApplicationCount
		catalog.SecondaryServiceCount = rawSecondaryServiceCount
		catalog.ToolingCount = rawToolingCount
		catalog.TestHelperCount = rawTestHelperCount
		catalog.UnassignedCount = rawUnassignedCount
		catalog.SupportingDependencyCount = rawSupportingDependencyCount
		catalog.DependencyOnlyCount = rawDependencyOnlyCount
	}
}

func projectSurfaceValue(value rawSurfaceValue) SurfaceValue {
	candidates := boundedSurfaceItems(value.Candidates, maxSurfaceValueCandidates)
	for index := range candidates {
		candidates[index] = sanitizeSurfaceValueText(candidates[index])
	}
	return SurfaceValue{
		Kind:       value.Kind,
		Text:       sanitizeSurfaceValueText(value.Text),
		Known:      value.Known,
		Candidates: candidates,
	}
}

func sanitizeSurfaceValueText(value string) string {
	parts := strings.Split(value, " | ")
	for index, part := range parts {
		if marker := strings.Index(part, "@/"); marker >= 0 {
			parts[index] = part[:marker] + "@<external>"
		}
	}
	return strings.Join(parts, " | ")
}

func boundedSurfaceItems[T any](values []T, limit int) []T {
	if len(values) > limit {
		values = values[:limit]
	}
	return append([]T(nil), values...)
}

func projectSurfaceValues(values []rawSurfaceValue) []SurfaceValue {
	result := make([]SurfaceValue, 0, len(values))
	for _, value := range values {
		result = append(result, projectSurfaceValue(value))
	}
	return result
}

func projectSurfaceSymbol(symbol rawSurfaceSymbol) SurfaceSymbol {
	return SurfaceSymbol{
		ID:       symbol.ID,
		Package:  symbol.Package,
		Name:     symbol.Name,
		Location: projectSurfaceLocation(symbol.Location),
	}
}

func projectSurfaceSymbols(symbols []rawSurfaceSymbol) []SurfaceSymbol {
	result := make([]SurfaceSymbol, 0, len(symbols))
	for _, symbol := range symbols {
		result = append(result, projectSurfaceSymbol(symbol))
	}
	return result
}

func projectSurfaceWrappers(wrappers []rawSurfaceWrapper) []SurfaceWrapper {
	result := make([]SurfaceWrapper, 0, len(wrappers))
	for _, wrapper := range wrappers {
		result = append(result, SurfaceWrapper{
			Symbol:   projectSurfaceSymbol(wrapper.Symbol),
			Callsite: projectSurfaceLocation(wrapper.Callsite),
			Origin:   wrapper.Origin,
		})
	}
	return result
}

func projectSurfaceEvidence(evidence []rawSurfaceEvidence) []SurfaceEvidence {
	result := make([]SurfaceEvidence, 0, len(evidence))
	for _, item := range evidence {
		result = append(result, SurfaceEvidence{
			ID:       item.ID,
			Kind:     item.Kind,
			Location: projectSurfaceLocation(item.Location),
			Detail:   item.Detail,
		})
	}
	return result
}

func projectSurfaceFrontiers(frontiers []rawSurfaceFrontier) []SurfaceFrontier {
	result := make([]SurfaceFrontier, 0, len(frontiers))
	for _, frontier := range frontiers {
		result = append(result, SurfaceFrontier{
			Kind:     frontier.Kind,
			Detail:   frontier.Detail,
			Location: projectOptionalSurfaceLocation(frontier.Location),
		})
	}
	return result
}

func projectSurfaceLoopSignals(signals []rawSurfaceLoopSignal) []SurfaceLoopSignal {
	result := make([]SurfaceLoopSignal, 0, len(signals))
	for _, signal := range signals {
		result = append(result, SurfaceLoopSignal{
			Kind:         signal.Kind,
			FunctionID:   signal.FunctionID,
			Location:     projectSurfaceLocation(signal.Location),
			TerminalSeed: signal.TerminalSeed,
			Detail:       signal.Detail,
			Certainty:    signal.Certainty,
		})
	}
	return result
}

func projectSurfacePackageDiagnostics(values []rawSurfacePackageDiagnostic) []SurfacePackageDiagnostic {
	result := make([]SurfacePackageDiagnostic, 0, len(values))
	for _, value := range values {
		result = append(result, SurfacePackageDiagnostic{
			ID: value.ID, Kind: value.Kind, Message: value.Message,
			Package: value.Package, PackageName: value.PackageName,
			OwningExecutable: cleanSurfacePath(value.OwningExecutable),
			ExecutableRole:   normalizeSurfaceExecutableRole(value.ExecutableRole),
			Availability:     normalizeSurfaceAvailability(value.Availability),
			Location:         projectOptionalSurfaceDiagnosticLocation(value.Location),
		})
	}
	return result
}

func projectSurfacePackageAvailability(values []rawSurfacePackageAvailability) []SurfacePackageAvailability {
	result := make([]SurfacePackageAvailability, 0, len(values))
	for _, value := range values {
		result = append(result, SurfacePackageAvailability{
			Package: value.Package, PackageName: value.PackageName,
			OwningExecutable: cleanSurfacePath(value.OwningExecutable),
			ExecutableRole:   normalizeSurfaceExecutableRole(value.ExecutableRole),
			Availability:     normalizeSurfaceAvailability(value.Availability),
			Reason:           value.Reason,
			DiagnosticIDs:    boundedSurfaceItems(value.DiagnosticIDs, maxSurfaceNestedItems),
		})
	}
	return result
}

func projectOptionalSurfaceLocation(location *rawSurfaceLocation) *SurfaceLocation {
	if location == nil {
		return nil
	}
	return projectSurfaceLocation(*location)
}

func projectSurfaceLocation(location rawSurfaceLocation) *SurfaceLocation {
	if location.Line <= 0 || location.Column < 0 || !validDiscoveredSurfacePath(location.Path) {
		return nil
	}
	return &SurfaceLocation{
		Path:   location.Path,
		Line:   location.Line,
		Column: location.Column,
	}
}

func projectOptionalSurfaceDiagnosticLocation(location *rawSurfaceLocation) *SurfaceLocation {
	if location == nil || location.Line <= 0 || location.Column < 0 || !validDiscoveredDiagnosticPath(location.Path) {
		return nil
	}
	return &SurfaceLocation{Path: location.Path, Line: location.Line, Column: location.Column}
}

func validDiscoveredSurfacePath(value string) bool {
	return len(value) <= maxDiscoveredSurfacePathBytes &&
		value != "." &&
		fs.ValidPath(value) &&
		!strings.ContainsRune(value, '\\') &&
		strings.HasSuffix(value, ".go")
}

func validDiscoveredDiagnosticPath(value string) bool {
	return len(value) <= maxDiscoveredSurfacePathBytes &&
		value != "." && fs.ValidPath(value) && !strings.ContainsRune(value, '\\')
}

// collectDiscoveredSurfacePaths adds every retained, exact surface evidence
// path in deterministic order. Invalid and absolute locations have already
// been removed by the projection boundary.
func collectDiscoveredSurfacePaths(surfaces *DiscoveredSurfaces, add func(string)) {
	if surfaces == nil || add == nil {
		return
	}
	paths := make(map[string]struct{})
	addLocation := func(location *SurfaceLocation) {
		if location != nil {
			paths[location.Path] = struct{}{}
		}
	}
	for _, trigger := range surfaces.Triggers {
		addLocation(trigger.ProcessEntrypoint.Location)
		addLocation(trigger.Constructor.Location)
		addLocation(trigger.RegistrationSite)
		addLocation(trigger.DescriptorSite)
		addLocation(trigger.ServerStartSite)
		addLocation(trigger.HandlerLocation)
		for _, wrapper := range trigger.WrapperChain {
			addLocation(wrapper.Symbol.Location)
			addLocation(wrapper.Callsite)
		}
		for _, item := range trigger.Evidence {
			addLocation(item.Location)
		}
		for _, frontier := range trigger.DynamicFrontier {
			addLocation(frontier.Location)
		}
	}
	for _, signal := range surfaces.LoopSignals {
		addLocation(signal.Location)
	}
	for _, frontier := range surfaces.DynamicFrontiers {
		addLocation(frontier.Location)
	}
	for _, frontier := range surfaces.UnsupportedDispatch {
		addLocation(frontier.Location)
	}
	for _, diagnostic := range surfaces.PackageDiagnostics {
		addLocation(diagnostic.Location)
	}

	ordered := make([]string, 0, len(paths))
	for value := range paths {
		ordered = append(ordered, value)
	}
	sort.Strings(ordered)
	for _, value := range ordered {
		add(value)
	}
}
