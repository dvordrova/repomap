package report

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/surfacediscovery"
)

const (
	ArchitectureGroundingFile            = "architecture_grounding.json"
	ArchitectureGroundingVersion         = 5
	typedArchitectureGroundingVersion    = 4
	previousArchitectureGroundingVersion = 3
	legacyArchitectureGroundingVersion   = 2
	maxArchitectureGroundingSize         = 4 * 1024 * 1024
	maxArchitectureEntryHandoffs         = 256
)

const architectureEntryHandoffLimitation = "Exact repository-local direct static call from a build-selected production process entry; runtime order, successful execution, ownership, and transitive reachability are not observed."

type ArchitectureGrounding struct {
	Version             int                           `json:"version"`
	RepositoryArchetype ArchitectureArchetype         `json:"repository_archetype"`
	GroundingMode       componentmap.GroundingMode    `json:"grounding_mode"`
	BehaviorAnchors     []ArchitectureBehaviorAnchor  `json:"behavior_anchors"`
	Relationships       []ArchitectureBehaviorHandoff `json:"relationships"`
	EntryHandoffs       []ArchitectureEntryHandoff    `json:"entry_handoffs"`
	Coverage            ArchitectureGroundingCoverage `json:"coverage"`
}

type ArchitectureGroundingCoverage struct {
	Complete                           bool                                       `json:"complete"`
	Reasons                            []surfacediscovery.GroundingCoverageReason `json:"reasons"`
	AnchorsConsidered                  int                                        `json:"anchors_considered"`
	AnchorsPublished                   int                                        `json:"anchors_published"`
	RelationshipsConsidered            int                                        `json:"relationships_considered"`
	RelationshipsPublished             int                                        `json:"relationships_published"`
	DeclarationFamilyMembersConsidered int                                        `json:"declaration_family_members_considered"`
	DeclarationFamilyMembersPublished  int                                        `json:"declaration_family_members_published"`
	EntryHandoffs                      ArchitectureEntryHandoffCoverage           `json:"entry_handoffs"`
}

type ArchitectureEntryHandoffCoverage struct {
	Complete             bool                                       `json:"complete"`
	Reasons              []surfacediscovery.GroundingCoverageReason `json:"reasons"`
	CandidateSetSHA256   string                                     `json:"candidate_set_sha256"`
	CandidatesConsidered int                                        `json:"candidates_considered"`
	CandidatesCollected  int                                        `json:"candidates_collected"`
	CandidatesPublished  int                                        `json:"candidates_published"`
	WitnessesConsidered  int                                        `json:"witnesses_considered"`
}

type ArchitectureArchetype struct {
	Selected     componentmap.RepositoryArchetype `json:"selected"`
	Evidence     []string                         `json:"evidence"`
	Alternatives []string                         `json:"alternatives"`
}

type ArchitectureBehaviorAnchor struct {
	ID                string                          `json:"id"`
	Kind              componentmap.BehaviorAnchorKind `json:"kind"`
	ProofMode         componentmap.AnchorProofMode    `json:"proof_mode"`
	Label             string                          `json:"label"`
	Location          evidence.Location               `json:"location"`
	Scenario          architectureGroundingScenario   `json:"scenario"`
	Producer          evidence.Provenance             `json:"producer"`
	Certainty         evidence.Certainty              `json:"certainty"`
	AssociatedMembers []ArchitectureAnchorMember      `json:"associated_members"`
	Limitations       []string                        `json:"limitations"`
}

type ArchitectureAnchorMember struct {
	ID            string            `json:"id"`
	EquivalentIDs []string          `json:"equivalent_ids,omitempty"`
	Package       string            `json:"package"`
	Name          string            `json:"name"`
	Location      evidence.Location `json:"location"`
}

type ArchitectureBehaviorHandoff struct {
	ID                      string              `json:"id"`
	From                    string              `json:"from_anchor_id"`
	To                      string              `json:"to_anchor_id"`
	Kind                    string              `json:"kind"`
	EvidenceKind            string              `json:"evidence_kind,omitempty"`
	Location                evidence.Location   `json:"location"`
	RepresentativeLocations []evidence.Location `json:"representative_locations,omitempty"`
	WitnessIDs              []string            `json:"witness_ids,omitempty"`
	WitnessCount            int                 `json:"witness_count,omitempty"`
	PackageCount            int                 `json:"package_count,omitempty"`
	Certainty               evidence.Certainty  `json:"certainty"`
	Producer                evidence.Provenance `json:"producer"`
}

// ArchitectureEntryHandoff is the report-side mirror of the producer-owned
// D210 first-hop edge. It is intentionally not an Architecture behavior
// anchor or relationship and therefore cannot enter the canonical Canvas.
type ArchitectureEntryHandoff struct {
	ID                     string                        `json:"id"`
	ProcessEntrypoint      ArchitectureAnchorMember      `json:"process_entrypoint"`
	Callee                 ArchitectureAnchorMember      `json:"callee"`
	RepresentativeCallsite evidence.Location             `json:"representative_callsite"`
	WitnessCount           int                           `json:"witness_count"`
	TargetPackage          string                        `json:"target_package"`
	Scenario               architectureGroundingScenario `json:"scenario"`
	Certainty              evidence.Certainty            `json:"certainty"`
	Producer               evidence.Provenance           `json:"producer"`
	Limitations            []string                      `json:"limitations"`
}

type architectureGroundingScenario struct {
	ID      string   `json:"id"`
	GOOS    string   `json:"goos"`
	GOARCH  string   `json:"goarch"`
	Tags    []string `json:"tags"`
	GoFlags string   `json:"go_flags,omitempty"`
}

func parseArchitectureGrounding(runDir string) (*ArchitectureGrounding, string) {
	path := filepath.Join(runDir, ArchitectureGroundingFile)
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ""
		}
		return nil, "architecture grounding: cannot inspect saved artifact"
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxArchitectureGroundingSize {
		return nil, "architecture grounding: saved artifact is not a bounded regular file"
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "architecture grounding: cannot read saved artifact"
	}
	var grounding ArchitectureGrounding
	if err := json.Unmarshal(data, &grounding); err != nil {
		return nil, "architecture grounding: saved artifact contains invalid json"
	}
	if err := validateArchitectureGrounding(grounding); err != nil {
		return nil, "architecture grounding: " + err.Error()
	}
	return &grounding, ""
}

func validateArchitectureGrounding(grounding ArchitectureGrounding) error {
	if grounding.Version != 1 && grounding.Version != legacyArchitectureGroundingVersion &&
		grounding.Version != previousArchitectureGroundingVersion &&
		grounding.Version != typedArchitectureGroundingVersion &&
		grounding.Version != ArchitectureGroundingVersion {
		return fmt.Errorf("unsupported version %d", grounding.Version)
	}
	if grounding.Version < typedArchitectureGroundingVersion &&
		(len(grounding.BehaviorAnchors) != 0 || len(grounding.Relationships) != 0) {
		// Historical anchors did not carry producer-owned proof mode. Treating
		// their semantic kind as proof would recreate the name-based inference
		// removed by Decision 205. The caller will install the package-grounded
		// local D177 canvas instead of reinterpreting these bytes.
		return fmt.Errorf("legacy behavior grounding has no typed proof mode")
	}
	if grounding.Version < ArchitectureGroundingVersion &&
		(len(grounding.EntryHandoffs) != 0 || !zeroArchitectureEntryHandoffCoverage(grounding.Coverage.EntryHandoffs)) {
		return fmt.Errorf("legacy grounding cannot carry entry handoffs")
	}
	if !validArchitectureArchetype(grounding.RepositoryArchetype.Selected) {
		return fmt.Errorf("unsupported repository archetype")
	}
	if !validArchitectureGroundingMode(grounding.GroundingMode) {
		return fmt.Errorf("unsupported grounding mode")
	}
	if len(grounding.BehaviorAnchors) > 256 || len(grounding.Relationships) > 512 {
		return fmt.Errorf("artifact exceeds grounding limits")
	}
	anchorIDs := make(map[string]struct{}, len(grounding.BehaviorAnchors))
	for _, anchor := range grounding.BehaviorAnchors {
		if strings.TrimSpace(anchor.ID) == "" || strings.TrimSpace(anchor.Label) == "" ||
			!validArchitectureBehaviorAnchorKind(anchor.Kind) || !validGroundingLocation(anchor.Location) ||
			!anchor.Certainty.Valid() || anchor.Scenario.ID == "" || len(anchor.AssociatedMembers) == 0 ||
			len(anchor.AssociatedMembers) > surfacediscovery.MaxArchitectureAnchorMembers ||
			len(anchor.Limitations) == 0 || len(anchor.Limitations) > 8 {
			return fmt.Errorf("behavior anchor is incomplete")
		}
		if grounding.Version >= typedArchitectureGroundingVersion &&
			!validArchitectureAnchorProofMode(anchor.Kind, anchor.ProofMode) {
			return fmt.Errorf("behavior anchor proof mode is invalid")
		}
		if _, duplicate := anchorIDs[anchor.ID]; duplicate {
			return fmt.Errorf("behavior anchor ids are not unique")
		}
		anchorIDs[anchor.ID] = struct{}{}
		for _, member := range anchor.AssociatedMembers {
			if member.ID == "" || !validGroundingLocation(member.Location) {
				return fmt.Errorf("behavior anchor member is incomplete")
			}
		}
	}
	for _, relationship := range grounding.Relationships {
		if relationship.ID == "" || relationship.From == relationship.To ||
			!validGroundingLocation(relationship.Location) || !relationship.Certainty.Valid() {
			return fmt.Errorf("behavior relationship is incomplete")
		}
		if _, exists := anchorIDs[relationship.From]; !exists {
			return fmt.Errorf("behavior relationship has unknown source")
		}
		if _, exists := anchorIDs[relationship.To]; !exists {
			return fmt.Errorf("behavior relationship has unknown target")
		}
		if grounding.Version >= 2 {
			if !validArchitectureRelationshipKind(relationship.Kind) || relationship.EvidenceKind != "bounded_direct_call" ||
				relationship.WitnessCount < 1 || len(relationship.WitnessIDs) != relationship.WitnessCount ||
				len(relationship.RepresentativeLocations) == 0 || len(relationship.RepresentativeLocations) > 8 {
				return fmt.Errorf("behavior relationship aggregation is incomplete")
			}
		}
	}
	if grounding.Version >= ArchitectureGroundingVersion {
		if grounding.EntryHandoffs == nil || grounding.Coverage.Reasons == nil ||
			grounding.Coverage.EntryHandoffs.Reasons == nil {
			return fmt.Errorf("entry handoff projection is incomplete")
		}
		if err := validateArchitectureEntryHandoffs(grounding.EntryHandoffs); err != nil {
			return err
		}
		if err := validateArchitectureEntryHandoffCoverage(
			grounding.EntryHandoffs,
			grounding.Coverage.EntryHandoffs,
		); err != nil {
			return err
		}
	}
	if grounding.Version >= typedArchitectureGroundingVersion {
		if err := validateArchitectureGroundingCoverage(grounding); err != nil {
			return err
		}
	}
	return nil
}

func validArchitectureAnchorProofMode(
	kind componentmap.BehaviorAnchorKind,
	mode componentmap.AnchorProofMode,
) bool {
	switch mode {
	case componentmap.AnchorProofProcessEntry:
		return kind == componentmap.AnchorProcessEntry
	case componentmap.AnchorProofCallTarget, componentmap.AnchorProofDeclarationFamily:
		return kind != componentmap.AnchorProcessEntry && validArchitectureBehaviorAnchorKind(kind)
	default:
		return false
	}
}

func validateArchitectureGroundingCoverage(grounding ArchitectureGrounding) error {
	coverage := grounding.Coverage
	if coverage.AnchorsConsidered < coverage.AnchorsPublished ||
		coverage.AnchorsPublished != len(grounding.BehaviorAnchors) ||
		coverage.RelationshipsConsidered < coverage.RelationshipsPublished ||
		coverage.RelationshipsPublished != len(grounding.Relationships) ||
		coverage.DeclarationFamilyMembersConsidered < coverage.DeclarationFamilyMembersPublished {
		return fmt.Errorf("grounding coverage counts are inconsistent")
	}
	publishedFamilyMembers := 0
	for _, anchor := range grounding.BehaviorAnchors {
		if anchor.ProofMode == componentmap.AnchorProofDeclarationFamily {
			publishedFamilyMembers += len(anchor.AssociatedMembers)
		}
	}
	if coverage.DeclarationFamilyMembersPublished != publishedFamilyMembers {
		return fmt.Errorf("grounding declaration-family coverage is inconsistent")
	}
	seenReasons := make(map[surfacediscovery.GroundingCoverageReason]struct{}, len(coverage.Reasons))
	for _, reason := range coverage.Reasons {
		if reason != surfacediscovery.GroundingCoverageCollectionLimit &&
			reason != surfacediscovery.GroundingCoveragePersistenceLimit {
			return fmt.Errorf("grounding coverage reason is invalid")
		}
		if _, duplicate := seenReasons[reason]; duplicate {
			return fmt.Errorf("grounding coverage repeats a reason")
		}
		seenReasons[reason] = struct{}{}
	}
	if grounding.Version >= ArchitectureGroundingVersion {
		for _, reason := range coverage.EntryHandoffs.Reasons {
			if _, present := seenReasons[reason]; !present {
				return fmt.Errorf("grounding coverage omits an entry handoff reason")
			}
		}
	}
	countsComplete := coverage.AnchorsConsidered == coverage.AnchorsPublished &&
		coverage.RelationshipsConsidered == coverage.RelationshipsPublished &&
		coverage.DeclarationFamilyMembersConsidered == coverage.DeclarationFamilyMembersPublished
	entryHandoffsComplete := grounding.Version < ArchitectureGroundingVersion || coverage.EntryHandoffs.Complete
	if coverage.Complete != (countsComplete && entryHandoffsComplete && len(coverage.Reasons) == 0) ||
		!coverage.Complete && len(coverage.Reasons) == 0 {
		return fmt.Errorf("grounding coverage completeness is inconsistent")
	}
	return nil
}

func validateArchitectureEntryHandoffs(handoffs []ArchitectureEntryHandoff) error {
	if len(handoffs) > maxArchitectureEntryHandoffs {
		return fmt.Errorf("artifact exceeds entry handoff limits")
	}
	seenIDs := make(map[string]struct{}, len(handoffs))
	seenEdges := make(map[string]struct{}, len(handoffs))
	previousID := ""
	sharedScenario := ""
	for _, handoff := range handoffs {
		if !validArchitectureEntryHandoffSymbol(handoff.ProcessEntrypoint) ||
			!validArchitectureEntryHandoffSymbol(handoff.Callee) ||
			!validArchitectureEntryHandoffLocation(handoff.RepresentativeCallsite) ||
			handoff.ProcessEntrypoint.Name != "main" ||
			handoff.ProcessEntrypoint.ID == handoff.Callee.ID ||
			handoff.Callee.Name == "init" ||
			handoff.RepresentativeCallsite.Path != handoff.ProcessEntrypoint.Location.Path ||
			handoff.RepresentativeCallsite.Line < handoff.ProcessEntrypoint.Location.Line ||
			handoff.TargetPackage == "" || handoff.TargetPackage != handoff.Callee.Package ||
			handoff.WitnessCount < 1 || handoff.Certainty != evidence.CertaintyStatic ||
			!validArchitectureEntryHandoffScenario(handoff.Scenario) ||
			!validArchitectureEntryHandoffProducer(handoff.Producer) ||
			len(handoff.Limitations) != 1 || handoff.Limitations[0] != architectureEntryHandoffLimitation {
			return fmt.Errorf("entry handoff is incomplete")
		}
		if handoff.ID != architectureEntryHandoffID(handoff) {
			return fmt.Errorf("entry handoff id is invalid")
		}
		if _, duplicate := seenIDs[handoff.ID]; duplicate {
			return fmt.Errorf("entry handoff ids are not unique")
		}
		if previousID > handoff.ID {
			return fmt.Errorf("entry handoffs are not canonical")
		}
		seenIDs[handoff.ID] = struct{}{}
		previousID = handoff.ID
		edge := architectureEntryHandoffEdgeKey(handoff)
		if _, duplicate := seenEdges[edge]; duplicate {
			return fmt.Errorf("entry handoff edges are not unique")
		}
		seenEdges[edge] = struct{}{}
		scenario := architectureEntryHandoffScenarioKey(handoff.Scenario)
		if sharedScenario != "" && sharedScenario != scenario {
			return fmt.Errorf("entry handoff scenarios are inconsistent")
		}
		sharedScenario = scenario
	}
	return nil
}

func validateArchitectureEntryHandoffCoverage(
	handoffs []ArchitectureEntryHandoff,
	coverage ArchitectureEntryHandoffCoverage,
) error {
	if !validArchitectureEntryHandoffSHA256(coverage.CandidateSetSHA256) ||
		coverage.CandidatesConsidered < coverage.CandidatesCollected ||
		coverage.CandidatesCollected < coverage.CandidatesPublished ||
		coverage.CandidatesPublished != len(handoffs) || coverage.WitnessesConsidered < 0 {
		return fmt.Errorf("entry handoff coverage counts are inconsistent")
	}
	publishedWitnesses := 0
	for _, handoff := range handoffs {
		publishedWitnesses += handoff.WitnessCount
	}
	if coverage.WitnessesConsidered < publishedWitnesses {
		return fmt.Errorf("entry handoff witness coverage is inconsistent")
	}
	seenReasons := make(map[surfacediscovery.GroundingCoverageReason]struct{}, len(coverage.Reasons))
	previous := surfacediscovery.GroundingCoverageReason("")
	for _, reason := range coverage.Reasons {
		if reason != surfacediscovery.GroundingCoverageCollectionLimit &&
			reason != surfacediscovery.GroundingCoveragePersistenceLimit {
			return fmt.Errorf("entry handoff coverage reason is invalid")
		}
		if _, duplicate := seenReasons[reason]; duplicate || previous > reason {
			return fmt.Errorf("entry handoff coverage reasons are not canonical")
		}
		seenReasons[reason] = struct{}{}
		previous = reason
	}
	_, collectionLimited := seenReasons[surfacediscovery.GroundingCoverageCollectionLimit]
	_, persistenceLimited := seenReasons[surfacediscovery.GroundingCoveragePersistenceLimit]
	if collectionLimited != (coverage.CandidatesConsidered > coverage.CandidatesCollected) ||
		persistenceLimited != (coverage.CandidatesCollected > coverage.CandidatesPublished) {
		return fmt.Errorf("entry handoff coverage reasons do not match counts")
	}
	countsComplete := coverage.CandidatesConsidered == coverage.CandidatesCollected &&
		coverage.CandidatesCollected == coverage.CandidatesPublished &&
		coverage.WitnessesConsidered == publishedWitnesses
	if coverage.Complete != (countsComplete && len(coverage.Reasons) == 0) {
		return fmt.Errorf("entry handoff coverage completeness is inconsistent")
	}
	if coverage.Complete && coverage.CandidateSetSHA256 != architectureEntryHandoffCandidateSetSHA256(handoffs) {
		return fmt.Errorf("entry handoff candidate set sha256 is inconsistent")
	}
	return nil
}

func validArchitectureEntryHandoffSymbol(symbol ArchitectureAnchorMember) bool {
	if !exactArchitectureGroundingString(symbol.ID) || !exactArchitectureGroundingString(symbol.Package) ||
		!exactArchitectureGroundingString(symbol.Name) || !validArchitectureEntryHandoffLocation(symbol.Location) {
		return false
	}
	seen := make(map[string]struct{}, len(symbol.EquivalentIDs))
	for _, equivalentID := range symbol.EquivalentIDs {
		if !exactArchitectureGroundingString(equivalentID) || equivalentID == symbol.ID {
			return false
		}
		if _, duplicate := seen[equivalentID]; duplicate {
			return false
		}
		seen[equivalentID] = struct{}{}
	}
	return true
}

func validArchitectureEntryHandoffLocation(location evidence.Location) bool {
	return validGroundingLocation(location) && location.EndLine == 0 && location.EndColumn == 0
}

func validArchitectureEntryHandoffScenario(scenario architectureGroundingScenario) bool {
	if !exactArchitectureGroundingString(scenario.GOOS) || !exactArchitectureGroundingString(scenario.GOARCH) ||
		scenario.Tags == nil || scenario.GoFlags != "" {
		return false
	}
	tags := append([]string(nil), scenario.Tags...)
	for _, tag := range tags {
		if !exactArchitectureGroundingString(tag) {
			return false
		}
	}
	sort.Strings(tags)
	wantID := "go:" + scenario.GOOS + "/" + scenario.GOARCH + ":tags=" + strings.Join(tags, ",")
	return scenario.ID == wantID
}

func architectureEntryHandoffScenarioKey(scenario architectureGroundingScenario) string {
	return strings.Join([]string{
		scenario.ID,
		scenario.GOOS,
		scenario.GOARCH,
		strings.Join(scenario.Tags, "\x00"),
		scenario.GoFlags,
	}, "\x01")
}

func validArchitectureEntryHandoffProducer(producer evidence.Provenance) bool {
	return producer.Provider == "go_ssa" && producer.Version == surfacediscovery.AnalyzerVersion &&
		producer.Operation == "collect_entry_direct_static_handoff" && producer.Detail == "" &&
		producer.Location == nil
}

func exactArchitectureGroundingString(value string) bool {
	return value != "" && value == strings.TrimSpace(value) &&
		!strings.ContainsAny(value, "\x00\r\n")
}

func architectureEntryHandoffID(handoff ArchitectureEntryHandoff) string {
	digest := sha256.New()
	for _, field := range []string{
		"entry-handoff-v1",
		handoff.ProcessEntrypoint.ID,
		architectureEntryHandoffLocationKey(handoff.ProcessEntrypoint.Location),
		handoff.Callee.ID,
		architectureEntryHandoffLocationKey(handoff.Callee.Location),
		handoff.Scenario.ID,
	} {
		writeArchitectureEntryHandoffIdentityField(digest, field)
	}
	return "entry-handoff-" + hex.EncodeToString(digest.Sum(nil)[:12])
}

func architectureEntryHandoffCandidateSetSHA256(handoffs []ArchitectureEntryHandoff) string {
	canonical := append([]ArchitectureEntryHandoff(nil), handoffs...)
	sort.Slice(canonical, func(i, j int) bool {
		return architectureEntryHandoffEdgeKey(canonical[i]) < architectureEntryHandoffEdgeKey(canonical[j])
	})
	digest := sha256.New()
	for _, handoff := range canonical {
		for _, field := range []string{
			handoff.ID,
			handoff.ProcessEntrypoint.ID,
			architectureEntryHandoffLocationKey(handoff.ProcessEntrypoint.Location),
			handoff.Callee.ID,
			architectureEntryHandoffLocationKey(handoff.Callee.Location),
			architectureEntryHandoffLocationKey(handoff.RepresentativeCallsite),
			strconv.Itoa(handoff.WitnessCount),
			handoff.TargetPackage,
			handoff.Scenario.ID,
		} {
			writeArchitectureEntryHandoffIdentityField(digest, field)
		}
	}
	return hex.EncodeToString(digest.Sum(nil))
}

func architectureEntryHandoffEdgeKey(handoff ArchitectureEntryHandoff) string {
	return strings.Join([]string{
		handoff.ProcessEntrypoint.ID,
		architectureEntryHandoffLocationKey(handoff.ProcessEntrypoint.Location),
		handoff.Callee.ID,
		architectureEntryHandoffLocationKey(handoff.Callee.Location),
	}, "\x00")
}

func architectureEntryHandoffLocationKey(location evidence.Location) string {
	return location.Path + ":" + strconv.Itoa(location.Line) + ":" + strconv.Itoa(location.Column)
}

func writeArchitectureEntryHandoffIdentityField(writer io.Writer, value string) {
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(value)))
	_, _ = writer.Write(length[:])
	_, _ = io.WriteString(writer, value)
}

func validArchitectureEntryHandoffSHA256(value string) bool {
	if len(value) != sha256.Size*2 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func zeroArchitectureEntryHandoffCoverage(coverage ArchitectureEntryHandoffCoverage) bool {
	return !coverage.Complete && coverage.Reasons == nil && coverage.CandidateSetSHA256 == "" &&
		coverage.CandidatesConsidered == 0 && coverage.CandidatesCollected == 0 &&
		coverage.CandidatesPublished == 0 && coverage.WitnessesConsidered == 0
}

func emptyArchitectureEntryHandoffCoverage() ArchitectureEntryHandoffCoverage {
	return ArchitectureEntryHandoffCoverage{
		Complete:           true,
		Reasons:            []surfacediscovery.GroundingCoverageReason{},
		CandidateSetSHA256: architectureEntryHandoffCandidateSetSHA256(nil),
	}
}

func validArchitectureRelationshipKind(kind string) bool {
	switch kind {
	case "dispatches_to", "loads_or_adapts_config", "registers_extension_family", "starts_lifecycle",
		"exposes_admin_control_plane", "dispatches_http_request", "configures_security_boundary",
		"static_call_supporting_relation":
		return true
	default:
		return false
	}
}

func ensureArchitectureGrounding(data *ReportData) {
	if data.ArchitectureGrounding != nil {
		return
	}
	archetype := componentmap.ArchetypeApplication
	if data.RepositoryGraph == nil || len(data.RepositoryGraph.PackageEdges) == 0 {
		archetype = componentmap.ArchetypeLibraryFramework
	}
	data.ArchitectureGrounding = &ArchitectureGrounding{
		Version: ArchitectureGroundingVersion,
		RepositoryArchetype: ArchitectureArchetype{
			Selected: archetype,
			Evidence: []string{"No persisted behavior-grounding artifact was available."},
		},
		GroundingMode: componentmap.GroundingPackages,
		EntryHandoffs: []ArchitectureEntryHandoff{},
		Coverage: ArchitectureGroundingCoverage{
			Complete: true, Reasons: []surfacediscovery.GroundingCoverageReason{},
			EntryHandoffs: emptyArchitectureEntryHandoffCoverage(),
		},
	}
}

func validGroundingLocation(location evidence.Location) bool {
	return location.Line > 0 && location.Column >= 0 && location.Path != "." &&
		fs.ValidPath(location.Path) && !strings.ContainsRune(location.Path, '\\')
}

func validArchitectureArchetype(value componentmap.RepositoryArchetype) bool {
	switch value {
	case componentmap.ArchetypeApplication, componentmap.ArchetypeModularPlatformServer,
		componentmap.ArchetypeLibraryFramework, componentmap.ArchetypeCLITool,
		componentmap.ArchetypeDaemonWorkerSystem, componentmap.ArchetypeMonorepoMixed:
		return true
	default:
		return false
	}
}

func validArchitectureGroundingMode(value componentmap.GroundingMode) bool {
	return value == componentmap.GroundingBehavior || value == componentmap.GroundingMixed ||
		value == componentmap.GroundingPackages
}

func validArchitectureBehaviorAnchorKind(value componentmap.BehaviorAnchorKind) bool {
	switch value {
	case componentmap.AnchorProcessEntry, componentmap.AnchorCommandDispatch, componentmap.AnchorConfigIngress,
		componentmap.AnchorConfigAdapter, componentmap.AnchorConfigApply, componentmap.AnchorRegistryWrite,
		componentmap.AnchorRegistryLookup, componentmap.AnchorLifecycleInterface, componentmap.AnchorLifecycleStart,
		componentmap.AnchorAdminControlPlane, componentmap.AnchorRequestDispatchRoot,
		componentmap.AnchorApplicationData, componentmap.AnchorSecurityBoundary,
		componentmap.AnchorExtensionFamily, componentmap.AnchorUnresolvedFrontier:
		return true
	default:
		return false
	}
}

func sortedArchitectureGroundingAnchors(values []ArchitectureBehaviorAnchor) []ArchitectureBehaviorAnchor {
	result := append([]ArchitectureBehaviorAnchor(nil), values...)
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}
