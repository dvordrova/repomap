// Package coremap progressively groups exact repository files and symbols into
// human-named core responsibilities. Models may name and group only; all
// repository identities are restored from request-local refs.
package coremap

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/gocoreobject"
	"github.com/dvordrova/repomap/internal/integrationusage"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/readmetargetscout"
	"github.com/dvordrova/repomap/internal/surfacediscovery"
)

const (
	Version          = 14
	ArtifactFilename = "core-map.json"
	MaxArtifactBytes = 8 << 20
	// MaxPayloadBytes bounds prompt prose plus the domain JSON before the
	// provider wraps both strings in its transport envelope. MaxRequestBytes
	// reserves the worst-case JSON escaping expansion of that bounded payload.
	MaxPayloadBytes     = 1 << 20
	MaxRequestBytes     = 8 << 20
	MaxResponseBytes    = 2 << 20
	MaxOutputTokens     = 128_000
	maxNameBytes        = 160
	maxPurposeBytes     = 800
	maxBaselineRoots    = 12
	maxChildrenPerBlock = 8
	maxBaselineBlocks   = 64
	maxReduceLevels     = 32
	MaxBlockDepth       = 1
)

const (
	semanticContract         = "repomap.coremap.v10"
	groupingSemanticContract = "repomap.coremap.grouping.v12"
)

type Stage string

const (
	StageBaseline Stage = "baseline"
	StageRefined  Stage = "refined"
)

// StageObserver lets one executor observer retain the semantic stage of every
// call. Bounded refined map and reduce calls share StageRefined; their exact
// phase, level, and batch identity remains in provider State.
type StageObserver interface {
	ObserveCoreMap(Stage, llm.Event) error
}

type FileFact struct {
	FileRef corpus.FileID `json:"file_ref"`
	Path    string        `json:"path"`
}

type SymbolFact struct {
	NodeID             string                    `json:"node_id"`
	Kind               programindex.ObjectKind   `json:"kind,omitempty"`
	Symbol             surfacediscovery.Symbol   `json:"symbol"`
	Package            string                    `json:"package"`
	Exported           bool                      `json:"exported"`
	Declaration        surfacediscovery.Location `json:"declaration"`
	IncomingCalls      int                       `json:"incoming_calls"`
	OutgoingCalls      int                       `json:"outgoing_calls"`
	UnresolvedOutgoing int                       `json:"unresolved_outgoing,omitempty"`
	TargetSeedKinds    []programindex.SeedKind   `json:"target_seed_kinds,omitempty"`
}

type Block struct {
	ID       string       `json:"id"`
	Name     string       `json:"name"`
	Purpose  string       `json:"purpose"`
	Files    []FileFact   `json:"files"`
	Symbols  []SymbolFact `json:"symbols,omitempty"`
	Children []Block      `json:"children,omitempty"`
}

type GroupAuthority string

const (
	GroupAuthorityModel           GroupAuthority = "model"
	GroupAuthorityLocalUnassigned GroupAuthority = "local_unassigned"
)

// Group is a navigation hierarchy over already validated refined
// responsibilities. Model-owned groups retain every supported overlapping
// membership. When a non-empty model grouping leaves responsibilities
// unplaced, one explicitly local structural group accounts for the exact
// complement without inventing a semantic membership.
type Group struct {
	ID        string         `json:"id"`
	Authority GroupAuthority `json:"authority"`
	Name      string         `json:"name"`
	Purpose   string         `json:"purpose"`
	BlockIDs  []string       `json:"block_ids"`
}

type Coverage struct {
	TrackedFiles           int `json:"tracked_files"`
	BaselineRoleFiles      int `json:"baseline_role_files"`
	SymbolsAvailable       int `json:"symbols_available"`
	BaselineBlocks         int `json:"baseline_blocks"`
	BaselineFilesSelected  int `json:"baseline_files_selected"`
	RefinedBlocks          int `json:"refined_blocks"`
	RefinedFilesSelected   int `json:"refined_files_selected"`
	RefinedSymbolsSelected int `json:"refined_symbols_selected"`
	SemanticFacts          int `json:"semantic_facts"`
	DynamicRelationFacts   int `json:"dynamic_relation_facts"`
	RefinedMapCalls        int `json:"refined_map_calls"`
	RefinedReduceCalls     int `json:"refined_reduce_calls"`
	RefinedGroupCalls      int `json:"refined_group_calls"`
	// RefinedGroups counts every navigation record, including the optional
	// explicitly local unassigned ledger. The following fields separate
	// semantic group output from deterministic omission accounting.
	RefinedGroups           int                                      `json:"refined_groups"`
	RefinedModelGroups      int                                      `json:"refined_model_groups"`
	RefinedLocalGroups      int                                      `json:"refined_local_groups"`
	RefinedUnassignedBlocks int                                      `json:"refined_unassigned_blocks"`
	DirectCallState         surfacediscovery.DirectCallIndexState    `json:"direct_call_state"`
	DirectCallCoverage      surfacediscovery.DirectCallIndexCoverage `json:"direct_call_coverage"`
}

type StageRequestSize struct {
	Calls            int `json:"calls"`
	PayloadBytes     int `json:"payload_bytes"`
	ProviderBytes    int `json:"provider_bytes"`
	MaxPayloadBytes  int `json:"max_payload_bytes"`
	MaxProviderBytes int `json:"max_provider_bytes"`
}

type RequestSizes struct {
	Baseline StageRequestSize `json:"baseline"`
	Refined  StageRequestSize `json:"refined"`
	Grouping StageRequestSize `json:"grouping"`
}

type Result struct {
	Version                int                   `json:"version"`
	Repository             string                `json:"repository"`
	CorpusRef              string                `json:"corpus_ref"`
	Target                 analysistarget.Target `json:"target"`
	ProgramTarget          *programindex.Target  `json:"program_target,omitempty"`
	ProgramIndexSHA256     string                `json:"program_index_sha256,omitempty"`
	IntegrationUsageSHA256 string                `json:"integration_usage_sha256,omitempty"`
	DirectCallSHA256       string                `json:"direct_call_sha256"`
	CoreObjectSHA256       string                `json:"core_object_sha256"`
	Baseline               []Block               `json:"baseline"`
	Refined                []Block               `json:"refined"`
	RefinedGroups          []Group               `json:"refined_groups"`
	Coverage               Coverage              `json:"coverage"`
	RequestSizes           RequestSizes          `json:"request_sizes"`
}

type targetRequest struct {
	Language         string                         `json:"language,omitempty"`
	Name             string                         `json:"name,omitempty"`
	Selector         string                         `json:"selector,omitempty"`
	AnchorPath       string                         `json:"anchor_path,omitempty"`
	Kind             string                         `json:"kind"`
	ModulePath       string                         `json:"module_path"`
	ModuleDir        string                         `json:"module_dir"`
	PackagePath      string                         `json:"package_path,omitempty"`
	PackageDir       string                         `json:"package_dir,omitempty"`
	ModulePackages   []analysistarget.TargetPackage `json:"module_packages,omitempty"`
	PublicPackages   []analysistarget.TargetPackage `json:"public_packages,omitempty"`
	RootDeclarations []analysistarget.Root          `json:"root_declarations,omitempty"`
}

type roleFileRequest struct {
	FileRef         corpus.FileID                      `json:"file_ref"`
	Path            string                             `json:"path"`
	Classifications []readmetargetscout.Classification `json:"classifications"`
}

type baselineRequest struct {
	Target          targetRequest         `json:"target"`
	ProgramCoverage programindex.Coverage `json:"program_coverage"`
	RoleFiles       []roleFileRequest     `json:"role_files"`
}

type symbolRequest struct {
	Ref                string                  `json:"ref"`
	Kind               programindex.ObjectKind `json:"kind,omitempty"`
	FileRef            corpus.FileID           `json:"file_ref"`
	Path               string                  `json:"path"`
	Line               int                     `json:"line"`
	Package            string                  `json:"package"`
	Name               string                  `json:"name"`
	Receiver           string                  `json:"receiver,omitempty"`
	Signature          string                  `json:"signature,omitempty"`
	Exported           bool                    `json:"exported"`
	IncomingCalls      int                     `json:"incoming_calls"`
	OutgoingCalls      int                     `json:"outgoing_calls"`
	UnresolvedOutgoing int                     `json:"unresolved_outgoing,omitempty"`
	TargetSeedKinds    []programindex.SeedKind `json:"target_seed_kinds,omitempty"`
}

// targetSeedRequest preserves every exact adapter-established launch fact in
// the refined semantic input, including module or application-object seeds
// that are intentionally not eligible as representative core declarations.
// SymbolRef is present only when the seeded object is also in the selectable
// core-symbol catalog.
type targetSeedRequest struct {
	Ref        string                  `json:"ref"`
	SymbolRef  string                  `json:"symbol_ref,omitempty"`
	Kind       programindex.SeedKind   `json:"kind"`
	ObjectKind programindex.ObjectKind `json:"object_kind"`
	Name       string                  `json:"name"`
	Location   programindex.Location   `json:"location"`
}

// integrationUseRequest is a complete, request-local projection of one
// already selected concrete integration operation. It is context for naming
// target-core responsibilities; CoreMap output still cites only exact files
// and symbols owned by this compilation.
type integrationUseRequest struct {
	Ref               string                  `json:"ref"`
	CallerSymbolRef   string                  `json:"caller_symbol_ref,omitempty"`
	DependencyKind    dependencies.Kind       `json:"dependency_kind"`
	DependencyName    string                  `json:"dependency_name"`
	DependencyModule  string                  `json:"dependency_module,omitempty"`
	DependencyPackage string                  `json:"dependency_package"`
	CallerKind        programindex.ObjectKind `json:"caller_kind"`
	CallerName        string                  `json:"caller_name"`
	CallerLocation    programindex.Location   `json:"caller_location"`
	Callsite          programindex.Location   `json:"callsite"`
	CallExpression    string                  `json:"call_expression"`
	CanonicalCallee   string                  `json:"canonical_callee"`
	Invocation        string                  `json:"invocation,omitempty"`
	Authority         string                  `json:"authority"`
	Label             string                  `json:"label"`
	Mechanism         string                  `json:"mechanism"`
}

type integrationUsageRequest struct {
	Uses []integrationUseRequest `json:"uses"`
}

type proposal struct {
	Name       string          `json:"name"`
	Purpose    string          `json:"purpose"`
	FileRefs   []corpus.FileID `json:"file_refs"`
	SymbolRefs []string        `json:"symbol_refs,omitempty"`
	Children   []proposal      `json:"children,omitempty"`
}

type modelResponse struct {
	Blocks []proposal `json:"blocks"`
}

type symbolAuthority struct {
	request         symbolRequest
	fact            SymbolFact
	programObjectID string
}

// groupingEdgeAuthority is retained only inside the sealed compilation. Raw
// ProgramIndex identities never enter a provider request; the grouping phase
// reduces these exact calls/executes edges to bounded block-to-block hop facts.
type groupingEdgeAuthority struct {
	FromObjectID string
	ToObjectID   string
}

type Compilation struct {
	repository             string
	corpusRef              string
	target                 analysistarget.Target
	programTarget          *programindex.Target
	programIndexSHA256     string
	programCoverage        programindex.Coverage
	integrationUsageSHA256 string
	directCallSHA256       string
	coreObjectSHA256       string
	directCallState        surfacediscovery.DirectCallIndexState
	directCoverage         surfacediscovery.DirectCallIndexCoverage
	baselineRequest        baselineRequest
	baselineWire           []byte
	files                  map[corpus.FileID]FileFact
	baselineFiles          map[corpus.FileID]FileFact
	symbols                map[string]symbolAuthority
	symbolRows             []symbolRequest
	targetSeedRows         []targetSeedRequest
	integrationUsage       *integrationUsageRequest
	dynamicRelationRows    []dynamicRelationRequest
	groupingEdges          []groupingEdgeAuthority
	seal                   string
}

func Run(ctx context.Context, executor llm.Executor, provider llm.Provider, compilation Compilation) (Result, error) {
	if err := validateCompilation(compilation); err != nil {
		return Result{}, err
	}
	baselineValue := modelResponse{Blocks: []proposal{}}
	baselineRequestSize := StageRequestSize{}
	if len(compilation.baselineRequest.RoleFiles) > 0 {
		baseline, err := llm.ExecuteJSON(ctx, executorForStage(executor, StageBaseline), provider, llm.Call[modelResponse]{
			State:  stageState(compilation, StageBaseline, compilation.baselineWire),
			Prompt: llm.Prompt{System: baselinePrompt, User: string(compilation.baselineWire), ResponseFormatJSON: true},
			Limits: llm.Limits{MaxRequestBytes: MaxRequestBytes, MaxResponseBytes: MaxResponseBytes, MaxOutputTokens: MaxOutputTokens},
			DecodeValidate: func(raw []byte) (modelResponse, error) {
				response, err := decodeResponse(raw)
				if err == nil {
					response.Blocks = normalizeProposalRefs(response.Blocks, compilation.baselineFiles, nil)
					err = validateProposals(StageBaseline, response.Blocks, compilation.baselineFiles, nil)
				}
				return response, err
			},
		})
		if err != nil {
			return Result{}, fmt.Errorf("coremap: baseline model cube: %w", err)
		}
		baselineValue = baseline.Value
		baselineRequestSize = StageRequestSize{
			Calls: 1, PayloadBytes: len(compilation.baselineWire), ProviderBytes: baseline.RequestBytes,
			MaxPayloadBytes: len(compilation.baselineWire), MaxProviderBytes: baseline.RequestBytes,
		}
	}
	targetKey := compilationTargetKey(compilation)
	baselineBlocks := restoreBlocks(StageBaseline, targetKey, baselineValue.Blocks, compilation.baselineFiles, nil)
	refineFiles := make(map[corpus.FileID]FileFact, len(compilation.baselineFiles)+len(compilation.symbolRows))
	for _, file := range collectFiles(baselineBlocks) {
		refineFiles[file.FileRef] = file
	}
	for _, row := range compilation.symbolRows {
		refineFiles[row.FileRef] = compilation.files[row.FileRef]
	}
	refined, refinedAccounting, err := runRefinedPipeline(
		ctx, executor, provider, compilation, baselineValue,
	)
	if err != nil {
		return Result{}, fmt.Errorf("coremap: refined model cube: %w", err)
	}
	refinedBlocks := restoreBlocks(StageRefined, targetKey, refined.Blocks, refineFiles, compilation.symbols)
	if err := validateRestoredBlocks(StageRefined, targetKey, refinedBlocks, true); err != nil {
		return Result{}, err
	}
	refinedGroups, groupingRequestSize, groupingCalls, err := runRefinedGrouping(
		ctx, executor, provider, compilation, refinedBlocks,
	)
	if err != nil {
		return Result{}, fmt.Errorf("coremap: refined grouping: %w", err)
	}
	modelGroupCount, localGroupCount, unassignedBlockCount := refinedGroupingCounts(refinedGroups)
	result := Result{
		Version: Version, Repository: compilation.repository, CorpusRef: compilation.corpusRef,
		IntegrationUsageSHA256: compilation.integrationUsageSHA256,
		DirectCallSHA256:       compilation.directCallSHA256,
		CoreObjectSHA256:       compilation.coreObjectSHA256,
		Baseline:               baselineBlocks, Refined: refinedBlocks, RefinedGroups: refinedGroups,
		Coverage: Coverage{
			TrackedFiles: len(compilation.files), BaselineRoleFiles: len(compilation.baselineFiles),
			SymbolsAvailable: len(compilation.symbols), BaselineBlocks: countBlocks(baselineBlocks),
			BaselineFilesSelected: len(collectFiles(baselineBlocks)), RefinedBlocks: len(refinedBlocks),
			RefinedFilesSelected: len(collectFiles(refinedBlocks)), RefinedSymbolsSelected: len(collectSymbols(refinedBlocks)),
			SemanticFacts: refinedAccounting.semanticFacts, DynamicRelationFacts: refinedAccounting.dynamicRelationFacts,
			RefinedMapCalls: refinedAccounting.mapCalls, RefinedReduceCalls: refinedAccounting.reduceCalls,
			RefinedGroupCalls: groupingCalls, RefinedGroups: len(refinedGroups),
			RefinedModelGroups: modelGroupCount, RefinedLocalGroups: localGroupCount,
			RefinedUnassignedBlocks: unassignedBlockCount,
			DirectCallState:         compilation.directCallState, DirectCallCoverage: compilation.directCoverage,
		},
		RequestSizes: RequestSizes{
			Baseline: baselineRequestSize,
			Refined:  refinedAccounting.requests,
			Grouping: groupingRequestSize,
		},
	}
	target := compilation.programTarget.Snapshot()
	result.ProgramTarget = &target
	result.ProgramIndexSHA256 = compilation.programIndexSHA256
	if compilationHasGoCompanion(compilation) {
		result.Target = compilation.target.Snapshot()
	}
	if err := result.ValidateAgainst(compilation); err != nil {
		return Result{}, err
	}
	return result, nil
}

func (result Result) Validate() error {
	if result.Version != Version || !validText(result.Repository, 256) || result.CorpusRef == "" {
		return fmt.Errorf("coremap: invalid result identity")
	}
	if result.ProgramTarget == nil {
		return fmt.Errorf("coremap: result has no ProgramTarget authority")
	}
	if err := result.ProgramTarget.Validate(); err != nil {
		return fmt.Errorf("coremap: result program target: %w", err)
	}
	if !validSHA256(result.ProgramIndexSHA256) ||
		(result.IntegrationUsageSHA256 != "" && !validSHA256(result.IntegrationUsageSHA256)) {
		return fmt.Errorf("coremap: invalid program result authority")
	}
	if resultHasGoCompanion(result) {
		if result.ProgramTarget.Language != "go" {
			return fmt.Errorf("coremap: non-Go result carries Go companion authority")
		}
		if !validSHA256(result.DirectCallSHA256) || !validSHA256(result.CoreObjectSHA256) {
			return fmt.Errorf("coremap: incomplete Go ProgramIndex companion authority")
		}
		if err := result.Target.Validate(); err != nil {
			return fmt.Errorf("coremap: result Go target: %w", err)
		}
		if result.Coverage.DirectCallState != surfacediscovery.DirectCallIndexReady {
			return fmt.Errorf("coremap: Go result has no ready direct-call authority")
		}
	}
	targetKey := result.ProgramTarget.ID
	if result.Coverage.TrackedFiles < 0 || result.Coverage.BaselineRoleFiles < 0 ||
		result.Coverage.BaselineRoleFiles > result.Coverage.TrackedFiles {
		return fmt.Errorf("coremap: invalid baseline role coverage")
	}
	if result.Coverage.SymbolsAvailable < 1 || result.Coverage.SemanticFacts < result.Coverage.SymbolsAvailable ||
		result.Coverage.DynamicRelationFacts < 0 ||
		result.Coverage.DynamicRelationFacts > result.Coverage.SemanticFacts ||
		result.Coverage.RefinedMapCalls < 1 || result.Coverage.RefinedReduceCalls < 0 ||
		result.Coverage.RefinedGroupCalls < 0 || result.Coverage.RefinedGroupCalls > 1 ||
		result.Coverage.RefinedGroups < 0 || result.Coverage.RefinedModelGroups < 0 ||
		result.Coverage.RefinedLocalGroups < 0 || result.Coverage.RefinedLocalGroups > 1 ||
		result.Coverage.RefinedUnassignedBlocks < 0 ||
		result.Coverage.RefinedUnassignedBlocks > result.Coverage.RefinedBlocks {
		return fmt.Errorf("coremap: invalid refined pipeline coverage")
	}
	if result.Coverage.BaselineRoleFiles == 0 {
		if len(result.Baseline) != 0 || result.Coverage.BaselineBlocks != 0 ||
			result.Coverage.BaselineFilesSelected != 0 || result.RequestSizes.Baseline != (StageRequestSize{}) {
			return fmt.Errorf("coremap: not-applicable baseline retained model output or request accounting")
		}
	} else {
		if err := validateRestoredBlocks(StageBaseline, targetKey, result.Baseline, false); err != nil {
			return err
		}
		if err := validateStageRequestSize(result.RequestSizes.Baseline, 1, MaxPayloadBytes); err != nil {
			return fmt.Errorf("coremap: invalid baseline request-size accounting")
		}
	}
	if err := validateRestoredBlocks(StageRefined, targetKey, result.Refined, result.Coverage.SymbolsAvailable > 0); err != nil {
		return err
	}
	if err := validateRefinedGroups(result.RefinedGroups, result.Refined, targetKey); err != nil {
		return err
	}
	if err := validateStageRequestSize(
		result.RequestSizes.Refined,
		result.Coverage.RefinedMapCalls+result.Coverage.RefinedReduceCalls,
		maxRefinedPayloadBytes,
	); err != nil {
		return fmt.Errorf("coremap: invalid request-size accounting")
	}
	if len(result.Refined) < 2 {
		if result.Coverage.RefinedGroupCalls != 0 || len(result.RefinedGroups) != 0 ||
			result.Coverage.RefinedGroups != 0 || result.Coverage.RefinedModelGroups != 0 ||
			result.Coverage.RefinedLocalGroups != 0 || result.Coverage.RefinedUnassignedBlocks != 0 ||
			result.RequestSizes.Grouping != (StageRequestSize{}) {
			return fmt.Errorf("coremap: trivial refined map retained grouping output")
		}
	} else {
		modelGroups, localGroups, unassignedBlocks := refinedGroupingCounts(result.RefinedGroups)
		if result.Coverage.RefinedGroupCalls != 1 || result.Coverage.RefinedGroups != len(result.RefinedGroups) ||
			result.Coverage.RefinedModelGroups != modelGroups || result.Coverage.RefinedLocalGroups != localGroups ||
			result.Coverage.RefinedUnassignedBlocks != unassignedBlocks {
			return fmt.Errorf("coremap: invalid refined grouping coverage")
		}
		if err := validateStageRequestSize(result.RequestSizes.Grouping, 1, maxRefinedPayloadBytes); err != nil {
			return fmt.Errorf("coremap: invalid grouping request-size accounting")
		}
	}
	return nil
}

func (result Result) ValidateAgainst(compilation Compilation) error {
	if err := validateCompilation(compilation); err != nil {
		return err
	}
	if err := result.Validate(); err != nil {
		return err
	}
	if result.Repository != compilation.repository || result.CorpusRef != compilation.corpusRef ||
		result.DirectCallSHA256 != compilation.directCallSHA256 ||
		result.CoreObjectSHA256 != compilation.coreObjectSHA256 ||
		result.ProgramIndexSHA256 != compilation.programIndexSHA256 ||
		result.IntegrationUsageSHA256 != compilation.integrationUsageSHA256 {
		return fmt.Errorf("coremap: result authority mismatch")
	}
	if result.ProgramTarget == nil || !reflect.DeepEqual(*result.ProgramTarget, *compilation.programTarget) {
		return fmt.Errorf("coremap: result program-target authority mismatch")
	}
	if !reflect.DeepEqual(result.Target, compilation.target) {
		return fmt.Errorf("coremap: result Go companion target authority mismatch")
	}
	for _, block := range result.Baseline {
		if err := validateBlockAuthority(block, compilation.baselineFiles, nil); err != nil {
			return err
		}
	}
	refinedFiles := make(map[corpus.FileID]FileFact, len(compilation.symbolRows)+result.Coverage.BaselineFilesSelected)
	for _, file := range collectFiles(result.Baseline) {
		refinedFiles[file.FileRef] = file
	}
	for _, row := range compilation.symbolRows {
		refinedFiles[row.FileRef] = compilation.files[row.FileRef]
	}
	for _, block := range result.Refined {
		if err := validateBlockAuthority(block, refinedFiles, compilation.symbols); err != nil {
			return err
		}
	}
	want := result.Coverage
	dynamicFacts := countDynamicRelationFacts(compilation.dynamicRelationRows)
	integrationFacts := 0
	if compilation.integrationUsage != nil {
		integrationFacts = len(compilation.integrationUsage.Uses)
	}
	semanticFacts := countBlocks(result.Baseline) + len(compilation.symbolRows) +
		len(compilation.targetSeedRows) + integrationFacts + len(compilation.dynamicRelationRows)
	if want.TrackedFiles != len(compilation.files) || want.BaselineRoleFiles != len(compilation.baselineFiles) ||
		want.SymbolsAvailable != len(compilation.symbols) || want.BaselineBlocks != countBlocks(result.Baseline) ||
		want.BaselineFilesSelected != len(collectFiles(result.Baseline)) || want.RefinedBlocks != len(result.Refined) ||
		want.RefinedFilesSelected != len(collectFiles(result.Refined)) || want.RefinedSymbolsSelected != len(collectSymbols(result.Refined)) ||
		want.SemanticFacts != semanticFacts || want.DynamicRelationFacts != dynamicFacts ||
		want.RefinedGroups != len(result.RefinedGroups) || want.RefinedGroupCalls != boolCount(len(result.Refined) >= 2) ||
		want.DirectCallState != compilation.directCallState || !reflect.DeepEqual(want.DirectCallCoverage, compilation.directCoverage) {
		return fmt.Errorf("coremap: coverage does not match exact authority")
	}
	modelGroups, localGroups, unassignedBlocks := refinedGroupingCounts(result.RefinedGroups)
	if want.RefinedModelGroups != modelGroups || want.RefinedLocalGroups != localGroups ||
		want.RefinedUnassignedBlocks != unassignedBlocks {
		return fmt.Errorf("coremap: grouping coverage does not match exact authority")
	}
	return nil
}

func refinedGroupingCounts(groups []Group) (modelGroups, localGroups, unassignedBlocks int) {
	for _, group := range groups {
		switch group.Authority {
		case GroupAuthorityModel:
			modelGroups++
		case GroupAuthorityLocalUnassigned:
			localGroups++
			unassignedBlocks += len(group.BlockIDs)
		}
	}
	return modelGroups, localGroups, unassignedBlocks
}

func boolCount(value bool) int {
	if value {
		return 1
	}
	return 0
}

func validateStageRequestSize(value StageRequestSize, calls int, maximumPayload int) error {
	if calls < 1 || value.Calls != calls || value.PayloadBytes < value.MaxPayloadBytes ||
		value.ProviderBytes < value.MaxProviderBytes || value.MaxPayloadBytes <= 0 ||
		value.MaxProviderBytes <= 0 || value.MaxPayloadBytes > maximumPayload ||
		value.MaxProviderBytes > MaxRequestBytes {
		return fmt.Errorf("invalid stage request-size accounting")
	}
	return nil
}

func countDynamicRelationFacts(values []dynamicRelationRequest) int {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		seen[value.Ref] = struct{}{}
	}
	return len(seen)
}

func validateCoreObjectScope(target analysistarget.Target, index gocoreobject.Index) error {
	if err := index.Validate(); err != nil {
		return fmt.Errorf("coremap: Go core object index: %w", err)
	}
	wantKind := gocoreobject.ScopeExecutablePackage
	wantPackage := target.PackagePath
	wantPackages := []string{target.PackagePath}
	if target.Kind == analysistarget.KindModuleLibrary {
		wantKind = gocoreobject.ScopeModuleLibrary
		wantPackage = ""
		wantPackages = make([]string, 0, len(target.RootPackages()))
		for _, pkg := range target.RootPackages() {
			wantPackages = append(wantPackages, pkg.PackagePath)
		}
	}
	if index.Scope.TargetRef != target.Ref || index.Scope.TargetKind != wantKind ||
		index.Scope.TargetModuleID != target.ModuleID || index.Scope.TargetModulePath != target.ModulePath ||
		index.Scope.TargetModuleDir != target.ModuleDir || index.Scope.TargetPackage != wantPackage ||
		!reflect.DeepEqual(index.Scope.TargetPackages, wantPackages) {
		return fmt.Errorf("coremap: Go core object index belongs to another target")
	}
	return nil
}

func decodeResponse(raw []byte) (modelResponse, error) {
	var response modelResponse
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&response); err != nil {
		return modelResponse{}, fmt.Errorf("coremap: decode response: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return modelResponse{}, fmt.Errorf("coremap: response contains trailing JSON")
	}
	return response, nil
}

func validateProposals(stage Stage, blocks []proposal, files map[corpus.FileID]FileFact, symbols map[string]symbolAuthority) error {
	if (len(blocks) == 0 && stage != StageBaseline) || (stage == StageBaseline && len(blocks) > maxBaselineRoots) {
		return fmt.Errorf("coremap: %s response has invalid block count", stage)
	}
	total := 0
	var visit func(proposal, int) error
	visit = func(block proposal, depth int) error {
		total++
		if !validText(block.Name, maxNameBytes) || !validText(block.Purpose, maxPurposeBytes) {
			return fmt.Errorf("coremap: %s response has invalid block text", stage)
		}
		if stage == StageRefined && len(block.Children) != 0 || depth > MaxBlockDepth || len(block.Children) > maxChildrenPerBlock {
			return fmt.Errorf("coremap: %s response has invalid hierarchy", stage)
		}
		if len(block.FileRefs)+len(block.SymbolRefs)+len(block.Children) == 0 {
			return fmt.Errorf("coremap: %s response has an ungrounded block", stage)
		}
		if stage == StageBaseline && len(block.SymbolRefs) != 0 {
			return fmt.Errorf("coremap: baseline response cites symbols")
		}
		if stage == StageRefined && len(symbols) > 0 && len(block.SymbolRefs) == 0 {
			return fmt.Errorf("coremap: refined response has no exact target symbol")
		}
		seenFiles := make(map[corpus.FileID]struct{}, len(block.FileRefs))
		for _, ref := range block.FileRefs {
			if _, ok := files[ref]; !ok {
				return fmt.Errorf("coremap: %s response cites unknown file ref %q", stage, ref)
			}
			if _, duplicate := seenFiles[ref]; duplicate {
				return fmt.Errorf("coremap: %s response repeats file ref %q inside one block", stage, ref)
			}
			seenFiles[ref] = struct{}{}
		}
		seenSymbols := make(map[string]struct{}, len(block.SymbolRefs))
		for _, ref := range block.SymbolRefs {
			if _, ok := symbols[ref]; !ok {
				return fmt.Errorf("coremap: refined response cites unknown symbol ref %q", ref)
			}
			if _, duplicate := seenSymbols[ref]; duplicate {
				return fmt.Errorf("coremap: refined response repeats symbol ref %q inside one block", ref)
			}
			seenSymbols[ref] = struct{}{}
		}
		for _, child := range block.Children {
			if err := visit(child, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	for _, block := range blocks {
		if err := visit(block, 0); err != nil {
			return err
		}
	}
	if stage == StageBaseline && total > maxBaselineBlocks {
		return fmt.Errorf("coremap: baseline response exceeds block limit")
	}
	return nil
}

// normalizeProposalRefs treats model-returned evidence refs as sets over the
// exact request-local catalogs. Unknown refs have no local identity or
// semantic authority, so they are discarded rather than guessed or allowed to
// invalidate otherwise grounded blocks. Exact duplicate block records are set
// duplicates after ref normalization and are canonicalized locally. Blocks
// with different model-authored semantic claims remain distinct even when the
// same exact evidence supports both. Validation still rejects blocks that
// become ungrounded or incomplete after normalization.
func normalizeProposalRefs(
	blocks []proposal,
	files map[corpus.FileID]FileFact,
	symbols map[string]symbolAuthority,
) []proposal {
	for position := range blocks {
		seenFiles := make(map[corpus.FileID]struct{}, len(blocks[position].FileRefs))
		fileRefs := blocks[position].FileRefs[:0]
		for _, ref := range blocks[position].FileRefs {
			if _, advertised := files[ref]; !advertised {
				continue
			}
			if _, duplicate := seenFiles[ref]; duplicate {
				continue
			}
			seenFiles[ref] = struct{}{}
			fileRefs = append(fileRefs, ref)
		}
		blocks[position].FileRefs = fileRefs

		seenSymbols := make(map[string]struct{}, len(blocks[position].SymbolRefs))
		symbolRefs := blocks[position].SymbolRefs[:0]
		for _, ref := range blocks[position].SymbolRefs {
			if _, advertised := symbols[ref]; !advertised {
				continue
			}
			if _, duplicate := seenSymbols[ref]; duplicate {
				continue
			}
			seenSymbols[ref] = struct{}{}
			symbolRefs = append(symbolRefs, ref)
		}
		blocks[position].SymbolRefs = symbolRefs
		blocks[position].Children = normalizeProposalRefs(blocks[position].Children, files, symbols)
	}
	return deduplicateProposals(blocks)
}

func deduplicateProposals(blocks []proposal) []proposal {
	seen := make(map[string]struct{}, len(blocks))
	result := blocks[:0]
	for _, block := range blocks {
		key := proposalSetKey(block)
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, block)
	}
	return result
}

func proposalSetKey(block proposal) string {
	fileRefs := make([]string, len(block.FileRefs))
	for position, ref := range block.FileRefs {
		fileRefs[position] = string(ref)
	}
	sort.Strings(fileRefs)
	symbolRefs := append([]string(nil), block.SymbolRefs...)
	sort.Strings(symbolRefs)
	children := make([]string, len(block.Children))
	for position, child := range block.Children {
		children[position] = proposalSetKey(child)
	}
	sort.Strings(children)
	wire, _ := json.Marshal(struct {
		Name       string   `json:"name"`
		Purpose    string   `json:"purpose"`
		FileRefs   []string `json:"file_refs"`
		SymbolRefs []string `json:"symbol_refs"`
		Children   []string `json:"children"`
	}{
		Name: block.Name, Purpose: block.Purpose, FileRefs: fileRefs,
		SymbolRefs: symbolRefs, Children: children,
	})
	return string(wire)
}

func validateRefinedBatchProposals(
	blocks []proposal,
	files map[corpus.FileID]FileFact,
	symbols map[string]symbolAuthority,
	allowEmpty bool,
) error {
	if len(blocks) == 0 {
		if allowEmpty {
			return nil
		}
		return fmt.Errorf("coremap: refined response has no blocks")
	}
	for _, block := range blocks {
		if !validText(block.Name, maxNameBytes) || !validText(block.Purpose, maxPurposeBytes) ||
			len(block.Children) != 0 || len(block.SymbolRefs) == 0 {
			return fmt.Errorf("coremap: refined response has an invalid grounded block")
		}
		seenFiles := make(map[corpus.FileID]struct{}, len(block.FileRefs))
		for _, ref := range block.FileRefs {
			if _, ok := files[ref]; !ok {
				return fmt.Errorf("coremap: refined response cites unknown file ref %q", ref)
			}
			if _, duplicate := seenFiles[ref]; duplicate {
				return fmt.Errorf("coremap: refined response repeats file ref %q inside one block", ref)
			}
			seenFiles[ref] = struct{}{}
		}
		seenSymbols := make(map[string]struct{}, len(block.SymbolRefs))
		for _, ref := range block.SymbolRefs {
			if _, ok := symbols[ref]; !ok {
				return fmt.Errorf("coremap: refined response cites unknown symbol ref %q", ref)
			}
			if _, duplicate := seenSymbols[ref]; duplicate {
				return fmt.Errorf("coremap: refined response repeats symbol ref %q inside one block", ref)
			}
			seenSymbols[ref] = struct{}{}
		}
	}
	return nil
}

func restoreBlocks(stage Stage, targetRef string, values []proposal, files map[corpus.FileID]FileFact, symbols map[string]symbolAuthority) []Block {
	result := make([]Block, len(values))
	for index, value := range values {
		block := Block{Name: value.Name, Purpose: value.Purpose, Files: make([]FileFact, len(value.FileRefs)), Symbols: make([]SymbolFact, len(value.SymbolRefs))}
		for position, ref := range value.FileRefs {
			block.Files[position] = files[ref]
		}
		for position, ref := range value.SymbolRefs {
			block.Symbols[position] = cloneSymbolFact(symbols[ref].fact)
		}
		block.Children = restoreBlocks(stage, targetRef, value.Children, files, symbols)
		block.ID = stableBlockID(stage, targetRef, block)
		result[index] = block
	}
	return result
}

func validateRestoredBlocks(stage Stage, targetRef string, blocks []Block, requireSymbols bool) error {
	if (len(blocks) == 0 && stage != StageBaseline) || (stage == StageBaseline && len(blocks) > maxBaselineRoots) {
		return fmt.Errorf("coremap: invalid %s blocks", stage)
	}
	seenBlockIDs := make(map[string]struct{}, countBlocks(blocks))
	var visit func(Block, int) error
	visit = func(block Block, depth int) error {
		if !validText(block.Name, maxNameBytes) || !validText(block.Purpose, maxPurposeBytes) || block.ID != stableBlockID(stage, targetRef, block) {
			return fmt.Errorf("coremap: invalid %s block", stage)
		}
		if _, duplicate := seenBlockIDs[block.ID]; duplicate {
			return fmt.Errorf("coremap: duplicate %s block evidence", stage)
		}
		seenBlockIDs[block.ID] = struct{}{}
		if stage == StageBaseline && len(block.Symbols) != 0 || stage == StageRefined && len(block.Children) != 0 || depth > MaxBlockDepth {
			return fmt.Errorf("coremap: invalid %s hierarchy", stage)
		}
		if stage == StageRefined && requireSymbols && len(block.Symbols) == 0 {
			return fmt.Errorf("coremap: refined block has no exact target symbol")
		}
		if len(block.Files)+len(block.Symbols)+len(block.Children) == 0 {
			return fmt.Errorf("coremap: ungrounded %s block", stage)
		}
		seenFiles := make(map[corpus.FileID]struct{}, len(block.Files))
		for _, file := range block.Files {
			if file.FileRef == "" || file.Path == "" {
				return fmt.Errorf("coremap: invalid restored file")
			}
			if _, duplicate := seenFiles[file.FileRef]; duplicate {
				return fmt.Errorf("coremap: duplicate restored file inside one block")
			}
			seenFiles[file.FileRef] = struct{}{}
		}
		seenSymbols := make(map[string]struct{}, len(block.Symbols))
		for _, symbol := range block.Symbols {
			if symbol.NodeID == "" || symbol.Declaration.Path == "" || symbol.Declaration.Line < 1 || symbol.IncomingCalls < 0 || symbol.OutgoingCalls < 0 {
				return fmt.Errorf("coremap: invalid restored symbol")
			}
			if symbol.UnresolvedOutgoing < 0 || symbol.Kind != "" && !symbol.Kind.Valid() {
				return fmt.Errorf("coremap: invalid restored symbol details")
			}
			for position, kind := range symbol.TargetSeedKinds {
				if !kind.Valid() || position > 0 && symbol.TargetSeedKinds[position-1] >= kind {
					return fmt.Errorf("coremap: invalid restored symbol target seeds")
				}
			}
			if _, duplicate := seenSymbols[symbol.NodeID]; duplicate {
				return fmt.Errorf("coremap: duplicate restored symbol inside one block")
			}
			seenSymbols[symbol.NodeID] = struct{}{}
		}
		for _, child := range block.Children {
			if err := visit(child, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	for _, block := range blocks {
		if err := visit(block, 0); err != nil {
			return err
		}
	}
	return nil
}

func validateBlockAuthority(
	block Block,
	files map[corpus.FileID]FileFact,
	symbols map[string]symbolAuthority,
) error {
	for _, file := range block.Files {
		if exact, ok := files[file.FileRef]; !ok || exact != file {
			return fmt.Errorf("coremap: restored file authority mismatch")
		}
	}
	for _, symbol := range block.Symbols {
		found := false
		for _, exact := range symbols {
			if exact.fact.NodeID == symbol.NodeID {
				found = reflect.DeepEqual(exact.fact, symbol)
				break
			}
		}
		if !found {
			return fmt.Errorf("coremap: restored symbol authority mismatch")
		}
	}
	for _, child := range block.Children {
		if err := validateBlockAuthority(child, files, symbols); err != nil {
			return err
		}
	}
	return nil
}

func collectFiles(blocks []Block) []FileFact {
	result := make([]FileFact, 0)
	seen := make(map[corpus.FileID]struct{})
	var visit func([]Block)
	visit = func(values []Block) {
		for _, block := range values {
			for _, file := range block.Files {
				if _, exists := seen[file.FileRef]; exists {
					continue
				}
				seen[file.FileRef] = struct{}{}
				result = append(result, file)
			}
			visit(block.Children)
		}
	}
	visit(blocks)
	sort.Slice(result, func(i, j int) bool { return result[i].FileRef < result[j].FileRef })
	return result
}

func collectSymbols(blocks []Block) []SymbolFact {
	result := make([]SymbolFact, 0)
	seen := make(map[string]struct{})
	for _, block := range blocks {
		for _, symbol := range block.Symbols {
			if _, exists := seen[symbol.NodeID]; exists {
				continue
			}
			seen[symbol.NodeID] = struct{}{}
			result = append(result, symbol)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].NodeID < result[j].NodeID })
	return result
}

func countBlocks(blocks []Block) int {
	total := len(blocks)
	for _, block := range blocks {
		total += countBlocks(block.Children)
	}
	return total
}

func stableBlockID(stage Stage, targetRef string, block Block) string {
	keys := make([]string, 0, len(block.Files)+len(block.Symbols)+len(block.Children))
	for _, file := range block.Files {
		keys = append(keys, "f:"+string(file.FileRef))
	}
	for _, symbol := range block.Symbols {
		keys = append(keys, "s:"+symbol.NodeID)
	}
	for _, child := range block.Children {
		keys = append(keys, "c:"+child.ID)
	}
	sort.Strings(keys)
	wire, _ := json.Marshal(struct {
		Contract string   `json:"contract"`
		Stage    Stage    `json:"stage"`
		Target   string   `json:"target"`
		Name     string   `json:"name"`
		Purpose  string   `json:"purpose"`
		Evidence []string `json:"evidence"`
	}{
		Contract: "coremap-block-v2", Stage: stage, Target: targetRef,
		Name: block.Name, Purpose: block.Purpose, Evidence: keys,
	})
	digest := sha256.Sum256(wire)
	return "core-" + hex.EncodeToString(digest[:8])
}

func stageState(compilation Compilation, stage Stage, wire []byte) []byte {
	state, _ := json.Marshal(struct {
		Contract            string `json:"contract"`
		Stage               Stage  `json:"stage"`
		PromptSHA           string `json:"prompt_sha256"`
		InputSHA            string `json:"input_sha256"`
		ProgramIndexSHA     string `json:"program_index_sha256,omitempty"`
		IntegrationUsageSHA string `json:"integration_usage_sha256,omitempty"`
		CoreObjectSHA       string `json:"core_object_sha256,omitempty"`
		Compilation         string `json:"compilation"`
	}{
		Contract: semanticContract, Stage: stage,
		PromptSHA: sha256Hex([]byte(map[Stage]string{StageBaseline: baselinePrompt, StageRefined: refinedPrompt}[stage])),
		InputSHA:  sha256Hex(wire), ProgramIndexSHA: compilation.programIndexSHA256,
		IntegrationUsageSHA: compilation.integrationUsageSHA256,
		CoreObjectSHA:       compilation.coreObjectSHA256,
		Compilation:         compilation.seal,
	})
	return state
}

func refinedCallState(
	compilation Compilation,
	phase string,
	level int,
	ordinal int,
	count int,
	wire []byte,
	prompt string,
) []byte {
	contract := semanticContract
	if phase == "group" {
		contract = groupingSemanticContract
	}
	state, _ := json.Marshal(struct {
		Contract            string `json:"contract"`
		Stage               Stage  `json:"stage"`
		Phase               string `json:"phase"`
		Level               int    `json:"level"`
		BatchOrdinal        int    `json:"batch_ordinal"`
		BatchCount          int    `json:"batch_count"`
		PromptSHA           string `json:"prompt_sha256"`
		InputSHA            string `json:"input_sha256"`
		ProgramIndexSHA     string `json:"program_index_sha256,omitempty"`
		IntegrationUsageSHA string `json:"integration_usage_sha256,omitempty"`
		CoreObjectSHA       string `json:"core_object_sha256,omitempty"`
		Compilation         string `json:"compilation"`
	}{
		Contract: contract, Stage: StageRefined, Phase: phase,
		Level: level, BatchOrdinal: ordinal, BatchCount: count,
		PromptSHA: sha256Hex([]byte(prompt)), InputSHA: sha256Hex(wire),
		ProgramIndexSHA:     compilation.programIndexSHA256,
		IntegrationUsageSHA: compilation.integrationUsageSHA256,
		CoreObjectSHA:       compilation.coreObjectSHA256,
		Compilation:         compilation.seal,
	})
	return state
}

func executorForStage(executor llm.Executor, stage Stage) llm.Executor {
	if observer, ok := executor.Observer.(StageObserver); ok {
		executor.Observer = llm.ObserverFunc(func(event llm.Event) error { return observer.ObserveCoreMap(stage, event) })
	}
	return executor
}

func enforceRequestLimit(stage Stage, wire []byte, prompt string) error {
	if len(wire)+len(prompt) > MaxPayloadBytes {
		return fmt.Errorf("coremap: %s request is %d bytes plus prompt, payload limit is %d; bounded evidence was not truncated", stage, len(wire), MaxPayloadBytes)
	}
	return nil
}

func validateCompilation(compilation Compilation) error {
	if compilation.repository == "" || compilation.corpusRef == "" || compilation.seal == "" ||
		compilation.files == nil || compilation.baselineFiles == nil || compilation.symbols == nil || compilation.symbolRows == nil ||
		len(compilation.baselineRequest.RoleFiles) != len(compilation.baselineFiles) {
		return fmt.Errorf("coremap: invalid compilation")
	}
	if err := validateSymbolRequests(compilation); err != nil {
		return err
	}
	if compilation.programTarget == nil {
		return fmt.Errorf("coremap: compilation has no ProgramTarget authority")
	}
	if err := compilation.programTarget.Validate(); err != nil {
		return fmt.Errorf("coremap: compilation program target: %w", err)
	}
	if !validSHA256(compilation.programIndexSHA256) || compilation.dynamicRelationRows == nil || compilation.groupingEdges == nil ||
		!reflect.DeepEqual(compilation.programCoverage, compilation.baselineRequest.ProgramCoverage) ||
		compilation.baselineRequest.Target.Language != compilation.programTarget.Language ||
		compilation.baselineRequest.Target.Kind != compilation.programTarget.Kind ||
		compilation.baselineRequest.Target.Name != compilation.programTarget.Name ||
		compilation.baselineRequest.Target.Selector != compilation.programTarget.Selector {
		return fmt.Errorf("coremap: invalid program compilation authority")
	}
	if compilationHasGoCompanion(compilation) {
		if compilation.programTarget.Language != "go" {
			return fmt.Errorf("coremap: non-Go compilation carries Go companion authority")
		}
		directIdentityValid := validSHA256(compilation.directCallSHA256)
		coreObjectIdentityValid := validSHA256(compilation.coreObjectSHA256)
		if !directIdentityValid || !coreObjectIdentityValid || compilation.directCallState != surfacediscovery.DirectCallIndexReady {
			return fmt.Errorf(
				"coremap: invalid Go ProgramIndex companion authority (direct_call_sha256=%t, core_object_sha256=%t, direct_call_state=%q)",
				directIdentityValid, coreObjectIdentityValid, compilation.directCallState,
			)
		}
		if err := compilation.target.Validate(); err != nil {
			return fmt.Errorf("coremap: compilation Go target: %w", err)
		}
	}
	if compilation.integrationUsageSHA256 == "" {
		if compilation.integrationUsage != nil {
			return fmt.Errorf("coremap: integration usage request has no bound artifact authority")
		}
	} else if !validSHA256(compilation.integrationUsageSHA256) || compilation.integrationUsage == nil {
		return fmt.Errorf("coremap: invalid integration usage authority")
	}
	if err := validateTargetSeedRequests(compilation); err != nil {
		return err
	}
	if err := validateIntegrationUsageRequest(compilation); err != nil {
		return err
	}
	if err := validateDynamicRelationRequests(compilation); err != nil {
		return err
	}
	if err := validateGroupingEdges(compilation.groupingEdges); err != nil {
		return err
	}
	for _, row := range compilation.baselineRequest.RoleFiles {
		if exact, ok := compilation.baselineFiles[row.FileRef]; !ok || exact.Path != row.Path ||
			compilation.files[row.FileRef] != exact || len(row.Classifications) == 0 {
			return fmt.Errorf("coremap: compilation role-file binding mismatch")
		}
	}
	var wire []byte
	if len(compilation.baselineRequest.RoleFiles) > 0 {
		var err error
		wire, err = json.Marshal(compilation.baselineRequest)
		if err != nil {
			return fmt.Errorf("coremap: encode baseline request: %w", err)
		}
	}
	if !bytes.Equal(wire, compilation.baselineWire) || compilation.seal != sealCompilation(compilation) {
		return fmt.Errorf("coremap: compilation binding mismatch")
	}
	return nil
}

func resultHasGoCompanion(result Result) bool {
	return result.DirectCallSHA256 != "" || result.CoreObjectSHA256 != "" ||
		!reflect.DeepEqual(result.Target, analysistarget.Target{}) ||
		result.Coverage.DirectCallState != "" ||
		!reflect.DeepEqual(result.Coverage.DirectCallCoverage, surfacediscovery.DirectCallIndexCoverage{})
}

func compilationHasGoCompanion(compilation Compilation) bool {
	return compilation.directCallSHA256 != "" || compilation.coreObjectSHA256 != "" ||
		!reflect.DeepEqual(compilation.target, analysistarget.Target{}) ||
		compilation.directCallState != "" ||
		!reflect.DeepEqual(compilation.directCoverage, surfacediscovery.DirectCallIndexCoverage{})
}

func validateSymbolRequests(compilation Compilation) error {
	if len(compilation.symbolRows) != len(compilation.symbols) || len(compilation.symbolRows) == 0 {
		return fmt.Errorf("coremap: symbol request authority is incomplete")
	}
	seenNodes := make(map[string]struct{}, len(compilation.symbolRows))
	for position, row := range compilation.symbolRows {
		ref := fmt.Sprintf("s%d", position+1)
		authority, ok := compilation.symbols[ref]
		file, fileOK := compilation.files[row.FileRef]
		if !ok || !fileOK || !reflect.DeepEqual(authority.request, row) || file.Path != row.Path ||
			authority.programObjectID == "" ||
			!validText(row.Path, programindex.MaxTextBytes) || row.Line < 1 ||
			!validText(row.Package, programindex.MaxTextBytes) ||
			!validText(row.Name, programindex.MaxTextBytes) ||
			(row.Signature != "" && !validText(row.Signature, programindex.MaxTextBytes)) ||
			row.IncomingCalls < 0 || row.OutgoingCalls < 0 || row.UnresolvedOutgoing < 0 ||
			authority.fact.NodeID == "" || authority.fact.Symbol.Name != row.Name ||
			authority.fact.Package != row.Package || authority.fact.Kind != row.Kind ||
			authority.fact.Exported != row.Exported || authority.fact.Declaration.Path != row.Path ||
			authority.fact.Declaration.Line != row.Line ||
			authority.fact.IncomingCalls != row.IncomingCalls ||
			authority.fact.OutgoingCalls != row.OutgoingCalls ||
			authority.fact.UnresolvedOutgoing != row.UnresolvedOutgoing ||
			!reflect.DeepEqual(authority.fact.TargetSeedKinds, row.TargetSeedKinds) {
			return fmt.Errorf("coremap: invalid symbol request row %q", ref)
		}
		if row.Kind != "" && !row.Kind.Valid() {
			return fmt.Errorf("coremap: symbol request %q has invalid kind", ref)
		}
		if _, duplicate := seenNodes[authority.fact.NodeID]; duplicate {
			return fmt.Errorf("coremap: symbol request repeats exact node authority")
		}
		seenNodes[authority.fact.NodeID] = struct{}{}
	}
	return nil
}

func sealCompilation(compilation Compilation) string {
	symbolWire, _ := json.Marshal(compilation.symbolRows)
	seedWire, _ := json.Marshal(compilation.targetSeedRows)
	integrationWire, _ := json.Marshal(compilation.integrationUsage)
	dynamicWire, _ := json.Marshal(compilation.dynamicRelationRows)
	groupingWire, _ := json.Marshal(compilation.groupingEdges)
	coverageWire, _ := json.Marshal(compilation.programCoverage)
	return sha256Hex([]byte("coremap-compilation-v10\x00" + compilation.repository + "\x00" + compilation.corpusRef + "\x00" + compilationTargetKey(compilation) + "\x00" + compilation.target.Ref + "\x00" + compilation.programIndexSHA256 + "\x00" + compilation.integrationUsageSHA256 + "\x00" + compilation.directCallSHA256 + "\x00" + compilation.coreObjectSHA256 + "\x00" + sha256Hex(coverageWire) + "\x00" + sha256Hex(compilation.baselineWire) + "\x00" + sha256Hex(symbolWire) + "\x00" + sha256Hex(seedWire) + "\x00" + sha256Hex(integrationWire) + "\x00" + sha256Hex(dynamicWire) + "\x00" + sha256Hex(groupingWire)))
}

func validateGroupingEdges(values []groupingEdgeAuthority) error {
	for position, edge := range values {
		if edge.FromObjectID == "" || edge.ToObjectID == "" || edge.FromObjectID == edge.ToObjectID {
			return fmt.Errorf("coremap: invalid grouping topology edge")
		}
		if position > 0 {
			previous := values[position-1]
			if previous.FromObjectID > edge.FromObjectID ||
				(previous.FromObjectID == edge.FromObjectID && previous.ToObjectID >= edge.ToObjectID) {
				return fmt.Errorf("coremap: grouping topology edges are not canonical")
			}
		}
	}
	return nil
}

func compilationTargetKey(compilation Compilation) string {
	if compilation.programTarget == nil {
		return ""
	}
	return compilation.programTarget.ID
}

func cloneClassifications(values []readmetargetscout.Classification) []readmetargetscout.Classification {
	result := make([]readmetargetscout.Classification, len(values))
	for position, value := range values {
		result[position] = readmetargetscout.Classification{
			Class: value.Class, Hypotheses: append([]string(nil), value.Hypotheses...),
		}
	}
	return result
}

func cloneSymbolFact(value SymbolFact) SymbolFact {
	value.TargetSeedKinds = append([]programindex.SeedKind(nil), value.TargetSeedKinds...)
	return value
}

func validateTargetSeedRequests(compilation Compilation) error {
	if compilation.programTarget == nil || len(compilation.targetSeedRows) != len(compilation.programTarget.Seeds) {
		return fmt.Errorf("coremap: target-seed request does not match program target")
	}
	for position, row := range compilation.targetSeedRows {
		seed := compilation.programTarget.Seeds[position]
		if row.Ref != fmt.Sprintf("t%d", position+1) || row.Kind != seed.Kind ||
			seed.Location == nil || row.Location != *seed.Location || !row.ObjectKind.Valid() ||
			!validText(row.Name, programindex.MaxTextBytes) {
			return fmt.Errorf("coremap: invalid target-seed request row")
		}
		if row.SymbolRef != "" {
			symbol, ok := compilation.symbols[row.SymbolRef]
			if !ok || symbol.fact.NodeID != seed.ObjectID || symbol.fact.Kind != row.ObjectKind ||
				symbol.fact.Symbol.Name != row.Name {
				return fmt.Errorf("coremap: target seed has invalid symbol binding")
			}
		}
	}
	return nil
}

func validateIntegrationUsageRequest(compilation Compilation) error {
	value := compilation.integrationUsage
	if value == nil {
		return nil
	}
	if value.Uses == nil {
		return fmt.Errorf("coremap: integration usage request must retain an exact array")
	}
	for position, use := range value.Uses {
		if use.Ref != fmt.Sprintf("u%d", position+1) || !validDependencyKind(use.DependencyKind) ||
			!validText(use.DependencyName, programindex.MaxTextBytes) ||
			!validText(use.DependencyPackage, programindex.MaxTextBytes) ||
			!use.CallerKind.Valid() || !validText(use.CallerName, programindex.MaxTextBytes) ||
			!validProgramLocation(use.CallerLocation) || !validProgramLocation(use.Callsite) ||
			(use.CallExpression != "" && !validText(use.CallExpression, programindex.MaxTextBytes)) ||
			!validText(use.CanonicalCallee, programindex.MaxTextBytes) ||
			!validIntegrationUsageAuthority(use.Authority) ||
			!validText(use.Label, integrationusage.MaxLabelBytes) ||
			!validText(use.Mechanism, integrationusage.MaxMechanismBytes) {
			return fmt.Errorf("coremap: invalid integration usage request row")
		}
		if use.DependencyModule != "" && !validText(use.DependencyModule, programindex.MaxTextBytes) ||
			use.Invocation != "" && !validText(use.Invocation, programindex.MaxTextBytes) {
			return fmt.Errorf("coremap: invalid optional integration usage context")
		}
		if use.CallerSymbolRef != "" {
			symbol, ok := compilation.symbols[use.CallerSymbolRef]
			if !ok || symbol.fact.Kind != use.CallerKind || symbol.fact.Symbol.Name != use.CallerName ||
				symbol.fact.Declaration.Path != use.CallerLocation.Path ||
				symbol.fact.Declaration.Line != use.CallerLocation.Line ||
				symbol.fact.Declaration.Column != use.CallerLocation.Column {
				return fmt.Errorf("coremap: integration use has invalid caller-symbol binding")
			}
		}
	}
	return nil
}

func validIntegrationUsageAuthority(value string) bool {
	return value == integrationusage.AuthoritySyntacticUnresolved ||
		value == integrationusage.AuthorityExactExternalSymbol
}

func validateDynamicRelationRequests(compilation Compilation) error {
	expectedOrdinal := 1
	representatives := make([]dynamicRelationRequest, 0)
	for position := 0; position < len(compilation.dynamicRelationRows); {
		ref := fmt.Sprintf("r%d", expectedOrdinal)
		if compilation.dynamicRelationRows[position].Ref != ref {
			return fmt.Errorf("coremap: dynamic relation refs are not deterministic")
		}
		end := position + 1
		for end < len(compilation.dynamicRelationRows) && compilation.dynamicRelationRows[end].Ref == ref {
			end++
		}
		if end-position > 2 {
			return fmt.Errorf("coremap: dynamic relation %q has too many mirrored perspectives", ref)
		}
		for current := position; current < end; current++ {
			if err := validateDynamicRelationRequest(compilation, compilation.dynamicRelationRows[current]); err != nil {
				return err
			}
		}
		first := compilation.dynamicRelationRows[position]
		mirrored := first.To != nil && first.From.SymbolRef != "" && first.To.SymbolRef != "" &&
			first.From.SymbolRef != first.To.SymbolRef
		if mirrored {
			if end-position != 2 || compilation.dynamicRelationRows[position].Perspective != "from" ||
				compilation.dynamicRelationRows[position+1].Perspective != "to" {
				return fmt.Errorf("coremap: cross-symbol dynamic relation %q is not mirrored", ref)
			}
			left := compilation.dynamicRelationRows[position]
			right := compilation.dynamicRelationRows[position+1]
			left.Perspective = ""
			right.Perspective = ""
			if !reflect.DeepEqual(left, right) {
				return fmt.Errorf("coremap: mirrored dynamic relation %q changed evidence", ref)
			}
		} else if end-position != 1 || first.Perspective != dynamicRelationPerspective(first) {
			return fmt.Errorf("coremap: dynamic relation %q has an invalid perspective", ref)
		}
		representatives = append(representatives, first)
		position = end
		expectedOrdinal++
	}
	return validateDynamicRelationJoints(representatives)
}

func validateDynamicRelationRequest(compilation Compilation, row dynamicRelationRequest) error {
	allowed := row.Resolution == programindex.ResolutionAlternatives
	switch row.Kind {
	case programindex.RelationImplements, programindex.RelationDecorates, programindex.RelationPassesCallback:
		allowed = true
	}
	if !allowed || !row.Kind.Valid() || !row.Resolution.Valid() ||
		row.TargetsObserved < 0 || row.TargetsRetained < 0 || row.TargetsOmitted < 0 ||
		row.TargetsRetained+row.TargetsOmitted != row.TargetsObserved {
		return fmt.Errorf("coremap: invalid dynamic relation request row")
	}
	if row.Invocation != "" && !validText(row.Invocation, programindex.MaxTextBytes) {
		return fmt.Errorf("coremap: invalid dynamic relation invocation")
	}
	if row.Location != nil && !validProgramLocation(*row.Location) {
		return fmt.Errorf("coremap: invalid dynamic relation location")
	}
	if err := validateRelationEndpoint(compilation, row.From); err != nil {
		return err
	}
	if row.TargetsRetained == 0 {
		if row.To != nil || row.TargetOrdinal != 0 {
			return fmt.Errorf("coremap: targetless dynamic relation retains a target part")
		}
	} else {
		if row.To == nil || row.TargetOrdinal < 1 || row.TargetOrdinal > row.TargetsRetained {
			return fmt.Errorf("coremap: dynamic relation target partition is invalid")
		}
		if row.TargetsObserved < 1 {
			return fmt.Errorf("coremap: dynamic relation target has no observed-count authority")
		}
		if err := validateRelationEndpoint(compilation, *row.To); err != nil {
			return err
		}
	}
	return nil
}

func validateDynamicRelationJoints(values []dynamicRelationRequest) error {
	expectedJoint := 1
	for position := 0; position < len(values); {
		jointRef := fmt.Sprintf("j%d", expectedJoint)
		if values[position].JointRef != jointRef {
			return fmt.Errorf("coremap: dynamic relation joint refs are not deterministic")
		}
		end := position + 1
		for end < len(values) && values[end].JointRef == jointRef {
			end++
		}
		first := values[position]
		if first.TargetsRetained == 0 {
			if end-position != 1 {
				return fmt.Errorf("coremap: targetless dynamic relation joint %q has several parts", jointRef)
			}
			position = end
			expectedJoint++
			continue
		} else if end-position != first.TargetsRetained {
			return fmt.Errorf(
				"coremap: dynamic relation joint %q retains %d of %d advertised target parts",
				jointRef, end-position, first.TargetsRetained,
			)
		}
		seenTargets := make(map[string]struct{}, first.TargetsRetained)
		for current := position; current < end; current++ {
			row := values[current]
			wantOrdinal := current - position + 1
			identityMatches := row.JointRef == first.JointRef && row.Kind == first.Kind && row.Resolution == first.Resolution
			evidenceMatches := reflect.DeepEqual(row.From, first.From) && row.Invocation == first.Invocation &&
				reflect.DeepEqual(row.Location, first.Location)
			coverageMatches := row.TargetsObserved == first.TargetsObserved &&
				row.TargetsRetained == first.TargetsRetained && row.TargetsOmitted == first.TargetsOmitted
			if !identityMatches || !evidenceMatches || !coverageMatches || row.TargetOrdinal != wantOrdinal {
				return fmt.Errorf(
					"coremap: dynamic relation joint %q has inconsistent target part %d (identity=%t, evidence=%t, coverage=%t, ordinal=%d, want=%d)",
					jointRef, wantOrdinal, identityMatches, evidenceMatches, coverageMatches, row.TargetOrdinal, wantOrdinal,
				)
			}
			if row.To != nil {
				encoded, err := json.Marshal(row.To)
				if err != nil {
					return fmt.Errorf("coremap: encode dynamic relation target: %w", err)
				}
				key := string(encoded)
				if _, duplicate := seenTargets[key]; duplicate {
					return fmt.Errorf("coremap: dynamic relation joint %q repeats a target", jointRef)
				}
				seenTargets[key] = struct{}{}
			}
		}
		position = end
		expectedJoint++
	}
	return nil
}

func validateRelationEndpoint(compilation Compilation, endpoint relationEndpointRequest) error {
	if !endpoint.Kind.Valid() || !endpoint.Visibility.Valid() ||
		!validText(endpoint.Name, programindex.MaxTextBytes) ||
		(endpoint.Package != "" && !validText(endpoint.Package, programindex.MaxTextBytes)) ||
		(endpoint.Signature != "" && !validText(endpoint.Signature, programindex.MaxTextBytes)) ||
		(endpoint.Location != nil && !validProgramLocation(*endpoint.Location)) {
		return fmt.Errorf("coremap: invalid dynamic relation endpoint")
	}
	if endpoint.SymbolRef == "" {
		return nil
	}
	symbol, ok := compilation.symbols[endpoint.SymbolRef]
	if !ok || symbol.fact.Kind != endpoint.Kind || symbol.fact.Symbol.Name != endpoint.Name ||
		symbol.fact.Package != endpoint.Package || symbol.request.Signature != endpoint.Signature ||
		endpoint.Location == nil || symbol.fact.Declaration.Path != endpoint.Location.Path ||
		symbol.fact.Declaration.Line != endpoint.Location.Line ||
		symbol.fact.Declaration.Column != endpoint.Location.Column ||
		(endpoint.Visibility == programindex.VisibilityPublic) != symbol.fact.Exported {
		return fmt.Errorf("coremap: dynamic relation endpoint has invalid symbol binding")
	}
	return nil
}

func validDependencyKind(kind dependencies.Kind) bool {
	return kind == dependencies.KindStdlib || kind == dependencies.KindExternal
}

func validProgramLocation(location programindex.Location) bool {
	return location.Path != "" && location.Line > 0 && location.Column > 0
}

func sha256Hex(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func validText(value string, maximum int) bool {
	if value == "" || value != strings.TrimSpace(value) || len(value) > maximum || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}
