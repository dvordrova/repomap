// Package surfacediscovery contains the isolated Go runtime-surface experiment.
package surfacediscovery

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/dvordrova/repomap/internal/componentmap"
)

const (
	AnalyzerVersion              = "surface-ssa-v12"
	TriggerCatalogVersion        = 7
	CoverageVersion              = 7
	CatalogVersion               = 1
	ArchitectureGroundingVersion = 5
	// MaxArchitectureAnchorMembers is the producer-owned complete chunk size
	// for one persisted behavior anchor. Larger declaration families are split
	// deterministically; they are never silently prefix-truncated.
	MaxArchitectureAnchorMembers = 16
)

type Options struct {
	RepoPath   string
	BuildTags  []string
	MaxDepth   int
	MaxTasks   int
	MaxTargets int
	Progress   func(PhaseProgress)
}

type PhaseProgress struct {
	Phase         string
	State         string
	ElapsedMillis int64
	Completed     int
	Total         int
	Detail        string
}

type PhaseMetric struct {
	Phase         string `json:"phase"`
	LatencyMillis int64  `json:"latency_ms"`
	Completed     int    `json:"completed,omitempty"`
	Total         int    `json:"total,omitempty"`
	Detail        string `json:"detail,omitempty"`
}

func DefaultOptions(repoPath string) Options {
	return Options{
		RepoPath:   repoPath,
		MaxDepth:   defaultMaxDepth,
		MaxTasks:   defaultMaxTasks,
		MaxTargets: defaultMaxTargets,
	}
}

type TriggerCatalog struct {
	Version         int             `json:"version"`
	AnalyzerVersion string          `json:"analyzer_version"`
	CatalogVersion  int             `json:"catalog_version"`
	Repository      Repository      `json:"repository"`
	Scenario        Scenario        `json:"scenario"`
	Triggers        []TriggerRecord `json:"triggers"`
}

type Repository struct {
	Root       string `json:"root"`
	ModulePath string `json:"module_path,omitempty"`
}

type Scenario struct {
	ID      string   `json:"id"`
	GOOS    string   `json:"goos"`
	GOARCH  string   `json:"goarch"`
	Tags    []string `json:"tags"`
	GoFlags string   `json:"go_flags,omitempty"`
}

type TriggerRecord struct {
	ID                   string         `json:"id"`
	ProvisionalID        bool           `json:"provisional_id"`
	Kind                 string         `json:"kind"`
	Producer             string         `json:"producer,omitempty"`
	Identity             Identity       `json:"identity"`
	Transport            string         `json:"transport"`
	Framework            string         `json:"framework"`
	ProcessEntrypoint    Symbol         `json:"process_entrypoint"`
	Dispatcher           Value          `json:"dispatcher"`
	Constructor          Symbol         `json:"constructor,omitempty"`
	RegistrationSite     Location       `json:"registration_site"`
	DescriptorSite       *Location      `json:"descriptor_site,omitempty"`
	ServerStartSite      *Location      `json:"server_start_site,omitempty"`
	Handler              Value          `json:"handler"`
	HandlerLocation      *Location      `json:"handler_location,omitempty"`
	Middleware           []Value        `json:"middleware"`
	WrapperChain         []Wrapper      `json:"wrapper_chain"`
	FinalSeed            string         `json:"final_seed"`
	DiscoveryBasis       string         `json:"discovery_basis"`
	Certainty            string         `json:"certainty"`
	Resolution           string         `json:"resolution"`
	ScenarioID           string         `json:"scenario_id"`
	Evidence             []Evidence     `json:"evidence"`
	Provenance           []Provenance   `json:"provenance"`
	DynamicFrontier      []Frontier     `json:"dynamic_frontier"`
	Status               string         `json:"status"`
	OwningExecutable     string         `json:"owning_executable,omitempty"`
	ExecutableRole       string         `json:"executable_role,omitempty"`
	Availability         string         `json:"availability"`
	UnavailableReason    string         `json:"unavailable_reason,omitempty"`
	TerminalSourceScope  string         `json:"terminal_source_scope,omitempty"`
	ApplicationClass     string         `json:"application_classification,omitempty"`
	PromotionBasis       string         `json:"promotion_basis,omitempty"`
	SurfaceRole          string         `json:"surface_role"`
	TraceReadiness       string         `json:"trace_readiness"`
	TraceReadinessReason string         `json:"trace_readiness_reason"`
	Quality              SurfaceQuality `json:"quality"`
}

// SurfaceQuality keeps independently derived dimensions separate so an exact
// identity cannot silently strengthen reachability or traceability.
type SurfaceQuality struct {
	Identity          string `json:"identity"`
	RegistrationStart string `json:"registration_start"`
	HandlerCallback   string `json:"handler_callback"`
	Reachability      string `json:"reachability"`
	Ownership         string `json:"ownership"`
	Traceability      string `json:"traceability"`
}

type Identity struct {
	Method string `json:"method,omitempty"`
	Path   Value  `json:"path"`
	Name   string `json:"name,omitempty"`
}

type Value struct {
	Kind       string   `json:"kind"`
	Text       string   `json:"text,omitempty"`
	Known      bool     `json:"known"`
	Candidates []string `json:"candidates"`
	addressKey string
}

type Symbol struct {
	ID            string   `json:"id"`
	EquivalentIDs []string `json:"equivalent_ids,omitempty"`
	Package       string   `json:"package"`
	Name          string   `json:"name"`
	Location      Location `json:"location"`
}

type Wrapper struct {
	Symbol   Symbol   `json:"symbol"`
	Callsite Location `json:"callsite"`
	Origin   string   `json:"origin"`
}

type Location struct {
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Column int    `json:"column,omitempty"`
}

type Evidence struct {
	ID       string   `json:"id"`
	Kind     string   `json:"kind"`
	Location Location `json:"location"`
	Detail   string   `json:"detail"`
}

type Provenance struct {
	Provider  string `json:"provider"`
	Version   string `json:"version"`
	Operation string `json:"operation"`
	Detail    string `json:"detail,omitempty"`
}

type Frontier struct {
	Kind     string    `json:"kind"`
	Detail   string    `json:"detail"`
	Location *Location `json:"location,omitempty"`
}

type SemanticSummary struct {
	FunctionID       string           `json:"function_id"`
	Effect           string           `json:"effect"`
	FinalSeed        string           `json:"final_seed"`
	WrapperPath      []string         `json:"wrapper_path"`
	Projections      map[string]Value `json:"projections"`
	Certainty        string           `json:"certainty"`
	ScenarioID       string           `json:"scenario_id"`
	Provenance       []Provenance     `json:"provenance"`
	SourceDependency []SourceDigest   `json:"source_dependencies"`
	Frontier         []Frontier       `json:"frontier"`
}

type SourceDigest struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type SurfaceCoverage struct {
	Version                     int                   `json:"version"`
	Repository                  Repository            `json:"repository"`
	Scenario                    Scenario              `json:"scenario"`
	EntrypointsConsidered       []Symbol              `json:"entrypoints_considered"`
	DispatchRootsFound          int                   `json:"dispatch_roots_found"`
	ConfiguredSeedsMatched      []string              `json:"configured_seeds_matched"`
	PackagesInspected           int                   `json:"packages_inspected"`
	FunctionsInspected          int                   `json:"functions_inspected"`
	DirectTriggers              int                   `json:"direct_triggers"`
	WrapperDerivedTriggers      int                   `json:"wrapper_derived_triggers"`
	UnresolvedHandlers          int                   `json:"unresolved_handlers"`
	PossibleRegistrations       int                   `json:"possible_registrations"`
	Workers                     int                   `json:"workers"`
	AsyncTasks                  int                   `json:"async_tasks"`
	ProcessEntries              int                   `json:"process_entries"`
	CobraDescriptorCount        int                   `json:"cobra_descriptor_count"`
	CobraExactBindingCount      int                   `json:"cobra_exact_binding_count"`
	CobraExactActivationCount   int                   `json:"cobra_exact_activation_count"`
	CobraPartialRelationCount   int                   `json:"cobra_partial_relation_count"`
	CobraDuplicateRelationCount int                   `json:"cobra_duplicate_relation_count"`
	CobraRecordCount            int                   `json:"cobra_record_count"`
	CobraDroppedRecordCount     int                   `json:"cobra_dropped_record_count"`
	AvailableProcessEntries     int                   `json:"available_process_entries"`
	UnavailableProcessEntries   int                   `json:"unavailable_process_entries"`
	PackageDiagnosticCount      int                   `json:"package_diagnostic_count"`
	UnavailablePackageCount     int                   `json:"unavailable_package_count"`
	PackageDiagnostics          []PackageDiagnostic   `json:"package_diagnostics"`
	UnavailablePackages         []PackageAvailability `json:"unavailable_packages"`
	LoopSignals                 []LoopSignal          `json:"loop_signals"`
	DynamicFrontiers            []Frontier            `json:"dynamic_frontiers"`
	UnsupportedDispatch         []Frontier            `json:"unsupported_dispatch_mechanisms"`
	// Decision 220 D: exact adapter accounting — how many records each
	// framework produced and how many came from the generic typed
	// registration detector (never a source of authority, always counted).
	TypedRegistrationDetectorMatches int            `json:"typed_registration_detector_matches,omitempty"`
	FrameworkMatched                 map[string]int `json:"framework_matched,omitempty"`
	BuildConstraints                 []string       `json:"build_constraints"`
	FilesSkipped                     []string       `json:"files_skipped"`
	PackagesSkipped                  []string       `json:"packages_skipped"`
	BudgetsReached                   []string       `json:"budgets_reached"`
	ColdLatencyMillis                int64          `json:"cold_latency_ms"`
	WarmLatencyMillis                *int64         `json:"warm_latency_ms,omitempty"`
	CacheReuse                       bool           `json:"cache_reuse"`
	Phases                           []PhaseMetric  `json:"phases,omitempty"`
	ScopeStatement                   string         `json:"scope_statement"`
}

type PackageDiagnostic struct {
	ID               string    `json:"id"`
	Kind             string    `json:"kind"`
	Message          string    `json:"message"`
	Package          string    `json:"package"`
	PackageName      string    `json:"package_name,omitempty"`
	OwningExecutable string    `json:"owning_executable,omitempty"`
	ExecutableRole   string    `json:"executable_role,omitempty"`
	Availability     string    `json:"availability"`
	Location         *Location `json:"location,omitempty"`
}

type PackageAvailability struct {
	Package          string   `json:"package"`
	PackageName      string   `json:"package_name,omitempty"`
	OwningExecutable string   `json:"owning_executable,omitempty"`
	ExecutableRole   string   `json:"executable_role,omitempty"`
	Availability     string   `json:"availability"`
	Reason           string   `json:"reason"`
	DiagnosticIDs    []string `json:"diagnostic_ids"`
}

type LoopSignal struct {
	Kind         string   `json:"kind"`
	FunctionID   string   `json:"function_id"`
	Location     Location `json:"location"`
	TerminalSeed string   `json:"terminal_seed,omitempty"`
	Detail       string   `json:"detail"`
	Certainty    string   `json:"certainty"`
}

type Result struct {
	Catalog   TriggerCatalog        `json:"trigger_catalog"`
	Coverage  SurfaceCoverage       `json:"surface_coverage"`
	Summaries []SemanticSummary     `json:"semantic_summaries"`
	Grounding ArchitectureGrounding `json:"architecture_grounding"`
}

type ArchitectureGrounding struct {
	Version             int                    `json:"version"`
	RepositoryArchetype ArchetypeAssessment    `json:"repository_archetype"`
	GroundingMode       string                 `json:"grounding_mode"`
	Anchors             []BehaviorAnchor       `json:"behavior_anchors"`
	Relationships       []BehaviorRelationship `json:"relationships"`
	EntryHandoffs       []EntryHandoff         `json:"entry_handoffs"`
	Coverage            GroundingCoverage      `json:"coverage"`
}

type GroundingCoverageReason string

const (
	GroundingCoverageCollectionLimit  GroundingCoverageReason = "collection_limit"
	GroundingCoveragePersistenceLimit GroundingCoverageReason = "persistence_limit"
)

type GroundingCoverage struct {
	Complete                           bool                      `json:"complete"`
	Reasons                            []GroundingCoverageReason `json:"reasons"`
	AnchorsConsidered                  int                       `json:"anchors_considered"`
	AnchorsPublished                   int                       `json:"anchors_published"`
	RelationshipsConsidered            int                       `json:"relationships_considered"`
	RelationshipsPublished             int                       `json:"relationships_published"`
	DeclarationFamilyMembersConsidered int                       `json:"declaration_family_members_considered"`
	DeclarationFamilyMembersPublished  int                       `json:"declaration_family_members_published"`
	EntryHandoffs                      EntryHandoffCoverage      `json:"entry_handoffs"`
}

// EntryHandoffCoverage describes the complete producer candidate set and the
// bounded collection persisted in Architecture Grounding. A digest remains
// meaningful when a bound is reached, while Complete prevents the retained
// prefix from being presented as exhaustive.
type EntryHandoffCoverage struct {
	Complete             bool                      `json:"complete"`
	Reasons              []GroundingCoverageReason `json:"reasons"`
	CandidateSetSHA256   string                    `json:"candidate_set_sha256"`
	CandidatesConsidered int                       `json:"candidates_considered"`
	CandidatesCollected  int                       `json:"candidates_collected"`
	CandidatesPublished  int                       `json:"candidates_published"`
	WitnessesConsidered  int                       `json:"witnesses_considered"`
}

type ArchetypeAssessment struct {
	Selected     string   `json:"selected"`
	Evidence     []string `json:"evidence"`
	Alternatives []string `json:"alternatives"`
}

type BehaviorAnchor struct {
	ID                string                       `json:"id"`
	Kind              string                       `json:"kind"`
	ProofMode         componentmap.AnchorProofMode `json:"proof_mode"`
	Label             string                       `json:"label"`
	Location          Location                     `json:"location"`
	Scenario          Scenario                     `json:"scenario"`
	Producer          Provenance                   `json:"producer"`
	Certainty         string                       `json:"certainty"`
	AssociatedMembers []Symbol                     `json:"associated_members"`
	Limitations       []string                     `json:"limitations"`
}

type BehaviorRelationship struct {
	ID                      string     `json:"id"`
	From                    string     `json:"from_anchor_id"`
	To                      string     `json:"to_anchor_id"`
	Kind                    string     `json:"kind"`
	EvidenceKind            string     `json:"evidence_kind"`
	Location                Location   `json:"location"`
	RepresentativeLocations []Location `json:"representative_locations"`
	WitnessIDs              []string   `json:"witness_ids"`
	WitnessCount            int        `json:"witness_count"`
	PackageCount            int        `json:"package_count"`
	Certainty               string     `json:"certainty"`
	Producer                Provenance `json:"producer"`
	witnessPackages         map[string]struct{}
}

// EntryHandoff is one exact, producer-owned first-hop static source edge from
// a production process entry. It is deliberately not a BehaviorAnchor or an
// Architecture relationship: the edge supplies Study eligibility without
// claiming runtime order, ownership, execution, or transitive reachability.
type EntryHandoff struct {
	ID                     string     `json:"id"`
	ProcessEntrypoint      Symbol     `json:"process_entrypoint"`
	Callee                 Symbol     `json:"callee"`
	RepresentativeCallsite Location   `json:"representative_callsite"`
	WitnessCount           int        `json:"witness_count"`
	TargetPackage          string     `json:"target_package"`
	Scenario               Scenario   `json:"scenario"`
	Certainty              string     `json:"certainty"`
	Producer               Provenance `json:"producer"`
	Limitations            []string   `json:"limitations"`
}

func (r *Result) normalize() {
	if r.Catalog.Triggers == nil {
		r.Catalog.Triggers = []TriggerRecord{}
	}
	for index := range r.Catalog.Triggers {
		trigger := &r.Catalog.Triggers[index]
		normalizeValue(&trigger.Identity.Path)
		normalizeValue(&trigger.Dispatcher)
		normalizeValue(&trigger.Handler)
		if trigger.Middleware == nil {
			trigger.Middleware = []Value{}
		}
		for valueIndex := range trigger.Middleware {
			normalizeValue(&trigger.Middleware[valueIndex])
		}
		if trigger.WrapperChain == nil {
			trigger.WrapperChain = []Wrapper{}
		}
		if trigger.Evidence == nil {
			trigger.Evidence = []Evidence{}
		}
		if trigger.Provenance == nil {
			trigger.Provenance = []Provenance{}
		}
		if trigger.DynamicFrontier == nil {
			trigger.DynamicFrontier = []Frontier{}
		}
		trigger.Evidence = compactEvidence(trigger.Evidence)
		sort.Slice(trigger.Evidence, func(i, j int) bool {
			left := trigger.Evidence[i]
			right := trigger.Evidence[j]
			return left.ID+"\x00"+left.Kind+"\x00"+locationKey(left.Location) <
				right.ID+"\x00"+right.Kind+"\x00"+locationKey(right.Location)
		})
		trigger.Provenance = compactProvenance(trigger.Provenance)
		sort.Slice(trigger.Provenance, func(i, j int) bool {
			left := trigger.Provenance[i]
			right := trigger.Provenance[j]
			return left.Provider+"\x00"+left.Version+"\x00"+left.Operation+"\x00"+left.Detail <
				right.Provider+"\x00"+right.Version+"\x00"+right.Operation+"\x00"+right.Detail
		})
		trigger.DynamicFrontier = compactFrontiers(trigger.DynamicFrontier)
	}
	if r.Summaries == nil {
		r.Summaries = []SemanticSummary{}
	}
	if r.Coverage.EntrypointsConsidered == nil {
		r.Coverage.EntrypointsConsidered = []Symbol{}
	}
	if r.Coverage.ConfiguredSeedsMatched == nil {
		r.Coverage.ConfiguredSeedsMatched = []string{}
	}
	if r.Coverage.DynamicFrontiers == nil {
		r.Coverage.DynamicFrontiers = []Frontier{}
	}
	if r.Coverage.LoopSignals == nil {
		r.Coverage.LoopSignals = []LoopSignal{}
	}
	if r.Coverage.UnsupportedDispatch == nil {
		r.Coverage.UnsupportedDispatch = []Frontier{}
	}
	if r.Coverage.FrameworkMatched == nil {
		r.Coverage.FrameworkMatched = map[string]int{}
	}
	if r.Coverage.PackageDiagnostics == nil {
		r.Coverage.PackageDiagnostics = []PackageDiagnostic{}
	}
	if r.Coverage.UnavailablePackages == nil {
		r.Coverage.UnavailablePackages = []PackageAvailability{}
	}
	if r.Coverage.BuildConstraints == nil {
		r.Coverage.BuildConstraints = []string{}
	}
	if r.Coverage.FilesSkipped == nil {
		r.Coverage.FilesSkipped = []string{}
	}
	if r.Coverage.PackagesSkipped == nil {
		r.Coverage.PackagesSkipped = []string{}
	}
	if r.Coverage.BudgetsReached == nil {
		r.Coverage.BudgetsReached = []string{}
	}
	if r.Coverage.Phases == nil {
		r.Coverage.Phases = []PhaseMetric{}
	}
	r.Coverage.DynamicFrontiers = compactFrontiers(r.Coverage.DynamicFrontiers)
	r.Coverage.UnsupportedDispatch = compactFrontiers(r.Coverage.UnsupportedDispatch)
	sort.Strings(r.Coverage.BudgetsReached)
	r.Coverage.BudgetsReached = compactStrings(r.Coverage.BudgetsReached)
	sort.Strings(r.Coverage.FilesSkipped)
	r.Coverage.FilesSkipped = compactStrings(r.Coverage.FilesSkipped)
	sort.Strings(r.Coverage.PackagesSkipped)
	r.Coverage.PackagesSkipped = compactStrings(r.Coverage.PackagesSkipped)
	sort.Slice(r.Coverage.PackageDiagnostics, func(i, j int) bool {
		return r.Coverage.PackageDiagnostics[i].ID < r.Coverage.PackageDiagnostics[j].ID
	})
	sort.Slice(r.Coverage.UnavailablePackages, func(i, j int) bool {
		return r.Coverage.UnavailablePackages[i].Package < r.Coverage.UnavailablePackages[j].Package
	})
	for index := range r.Coverage.UnavailablePackages {
		sort.Strings(r.Coverage.UnavailablePackages[index].DiagnosticIDs)
		r.Coverage.UnavailablePackages[index].DiagnosticIDs = compactStrings(
			r.Coverage.UnavailablePackages[index].DiagnosticIDs,
		)
	}
	if r.Grounding.Anchors == nil {
		r.Grounding.Anchors = []BehaviorAnchor{}
	}
	if r.Grounding.Relationships == nil {
		r.Grounding.Relationships = []BehaviorRelationship{}
	}
	if r.Grounding.EntryHandoffs == nil {
		r.Grounding.EntryHandoffs = []EntryHandoff{}
	}
	if r.Grounding.Coverage.Reasons == nil {
		r.Grounding.Coverage.Reasons = []GroundingCoverageReason{}
	}
	if r.Grounding.Coverage.EntryHandoffs.Reasons == nil {
		r.Grounding.Coverage.EntryHandoffs.Reasons = []GroundingCoverageReason{}
	}
	sort.Slice(r.Grounding.Coverage.Reasons, func(i, j int) bool {
		return r.Grounding.Coverage.Reasons[i] < r.Grounding.Coverage.Reasons[j]
	})
	r.Grounding.Coverage.Reasons = compactGroundingCoverageReasons(r.Grounding.Coverage.Reasons)
	sort.Slice(r.Grounding.Coverage.EntryHandoffs.Reasons, func(i, j int) bool {
		return r.Grounding.Coverage.EntryHandoffs.Reasons[i] < r.Grounding.Coverage.EntryHandoffs.Reasons[j]
	})
	r.Grounding.Coverage.EntryHandoffs.Reasons = compactGroundingCoverageReasons(
		r.Grounding.Coverage.EntryHandoffs.Reasons,
	)
	sort.Slice(r.Catalog.Triggers, func(i, j int) bool {
		return r.Catalog.Triggers[i].ID < r.Catalog.Triggers[j].ID
	})
	sort.Strings(r.Coverage.ConfiguredSeedsMatched)
	r.Coverage.ConfiguredSeedsMatched = compactStrings(r.Coverage.ConfiguredSeedsMatched)
	sort.Slice(r.Coverage.LoopSignals, func(i, j int) bool {
		left := r.Coverage.LoopSignals[i]
		right := r.Coverage.LoopSignals[j]
		if left.FunctionID != right.FunctionID {
			return left.FunctionID < right.FunctionID
		}
		if locationKey(left.Location) != locationKey(right.Location) {
			return locationKey(left.Location) < locationKey(right.Location)
		}
		return left.Kind < right.Kind
	})
	sort.Slice(r.Summaries, func(i, j int) bool {
		left := r.Summaries[i]
		right := r.Summaries[j]
		if left.FunctionID != right.FunctionID {
			return left.FunctionID < right.FunctionID
		}
		if left.FinalSeed != right.FinalSeed {
			return left.FinalSeed < right.FinalSeed
		}
		if left.Effect != right.Effect {
			return left.Effect < right.Effect
		}
		return strings.Join(left.WrapperPath, "\x00") < strings.Join(right.WrapperPath, "\x00")
	})
	sort.Slice(r.Grounding.Anchors, func(i, j int) bool {
		return r.Grounding.Anchors[i].ID < r.Grounding.Anchors[j].ID
	})
	sort.Slice(r.Grounding.Relationships, func(i, j int) bool {
		return r.Grounding.Relationships[i].ID < r.Grounding.Relationships[j].ID
	})
	sort.Slice(r.Grounding.EntryHandoffs, func(i, j int) bool {
		return r.Grounding.EntryHandoffs[i].ID < r.Grounding.EntryHandoffs[j].ID
	})
}

func compactGroundingCoverageReasons(input []GroundingCoverageReason) []GroundingCoverageReason {
	result := make([]GroundingCoverageReason, 0, len(input))
	for _, reason := range input {
		if len(result) == 0 || result[len(result)-1] != reason {
			result = append(result, reason)
		}
	}
	return result
}

func normalizeValue(value *Value) {
	if value != nil && value.Candidates == nil {
		value.Candidates = []string{}
	}
}

func compactFrontiers(input []Frontier) []Frontier {
	result := make([]Frontier, 0, len(input))
	seen := make(map[string]struct{}, len(input))
	for _, frontier := range input {
		location := ""
		if frontier.Location != nil {
			location = locationKey(*frontier.Location)
		}
		key := frontier.Kind + "\x00" + frontier.Detail + "\x00" + location
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, frontier)
	}
	sort.Slice(result, func(i, j int) bool {
		leftLocation := ""
		if result[i].Location != nil {
			leftLocation = locationKey(*result[i].Location)
		}
		rightLocation := ""
		if result[j].Location != nil {
			rightLocation = locationKey(*result[j].Location)
		}
		return result[i].Kind+"\x00"+result[i].Detail+"\x00"+leftLocation <
			result[j].Kind+"\x00"+result[j].Detail+"\x00"+rightLocation
	})
	return result
}

func stableTriggerID(record TriggerRecord) string {
	path := record.Identity.Path.Text
	if !record.Identity.Path.Known {
		path = "<dynamic>"
	}
	dispatcher := record.Dispatcher.Text
	if !record.Dispatcher.Known {
		dispatcher = "<dynamic>"
	}
	handler := record.Handler.Text
	if !record.Handler.Known {
		handler = "<dynamic>"
	}
	if record.Kind == "http_server" {
		path = "<server-start>"
		dispatcher = "<server-handler>"
		handler = "<server-handler>"
	}
	parts := []string{
		"trigger-v1", record.Kind, record.Identity.Method, path,
		dispatcher, record.ProcessEntrypoint.ID, record.ScenarioID,
		stableRecordLocation(record).Path,
		strconv.Itoa(stableRecordLocation(record).Line),
		strconv.Itoa(stableRecordLocation(record).Column),
		handler, record.FinalSeed,
	}
	if record.Identity.Name != "" {
		parts = append(parts, record.Identity.Name)
	}
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "trigger-" + hex.EncodeToString(digest[:12])
}

func stableRecordLocation(record TriggerRecord) Location {
	if !filepath.IsAbs(record.RegistrationSite.Path) {
		return record.RegistrationSite
	}
	for _, wrapper := range record.WrapperChain {
		if wrapper.Callsite.Path != "" && !filepath.IsAbs(wrapper.Callsite.Path) {
			return wrapper.Callsite
		}
	}
	return Location{Path: "<external>", Line: record.RegistrationSite.Line, Column: record.RegistrationSite.Column}
}

func compactStrings(input []string) []string {
	result := make([]string, 0, len(input))
	for _, item := range input {
		if len(result) == 0 || result[len(result)-1] != item {
			result = append(result, item)
		}
	}
	return result
}

func MarshalDeterministic(value any) ([]byte, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}
