package componentmap

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/evidence"
)

// Decision 216: bounded local ArchitectureUnit catalog. The model groups
// request-local unit refs (u*) instead of raw package/symbol members; backend
// expansion restores exact membership and coverage.

const (
	// UnitCatalogVersion changes whenever unit identity, role separation, or
	// the unit compiler contract changes.
	UnitCatalogVersion = 2

	// targetMinUnits / targetMaxUnits bound the advertised catalog. Hard
	// bounds are contract-owned and provider-free tested (Decision 216.9).
	targetMinUnits = 24
	targetMaxUnits = 64

	// maxUnitMembers caps one unit before deterministic splitting.
	maxUnitMembers = 48

	// maxUnitWireLabelBytes bounds the semantic label on the wire.
	maxUnitWireLabelBytes = 96
)

// UnitRole separates exact local members by source role. The compiler owns
// this classification; it is never inferred from model prose.
type UnitRole string

const (
	UnitRoleProduction    UnitRole = "production"
	UnitRoleTest          UnitRole = "test"
	UnitRoleTooling       UnitRole = "tooling"
	UnitRoleDocumentation UnitRole = "documentation"
)

// ArchitectureUnit is the private local unit contract. Canonical identity and
// exact membership never enter the provider wire.
type ArchitectureUnit struct {
	// CanonicalID is the stable backend-owned unit identity.
	CanonicalID string `json:"canonical_id"`
	// Role is the deterministic source role of the unit's members.
	Role UnitRole `json:"role"`
	// Label is a short semantic label derived from member names.
	Label string `json:"label"`
	// MemberIDs holds every exact raw member in this primary unit.
	MemberIDs []MemberID `json:"member_ids"`
	// MemberKinds counts members by kind.
	MemberKinds map[MemberKind]int `json:"member_kinds"`
	// PackagePaths are the exact package identities clustered here.
	PackagePaths []string `json:"package_paths,omitempty"`
	// AnchorIDs are exact behavior anchors attached to this unit.
	AnchorIDs []string `json:"anchor_ids,omitempty"`
	// ExpansionDigest binds the exact member set.
	ExpansionDigest string `json:"expansion_digest"`
}

// UnitWireRef is the request-local unit reference (u*). Canonical IDs never
// enter the wire.
type UnitWireRef string

// SynthesisUnit is the provider-visible unit projection (Decision 216 wire).
type SynthesisUnit struct {
	Ref   UnitWireRef `json:"ref"`
	Label string      `json:"label"`
	Role  UnitRole    `json:"role"`
	// MemberKindCounts bounds the wire with counts, never raw IDs.
	MemberKindCounts map[MemberKind]int `json:"member_kind_counts"`
	// RepresentativeLabels are bounded (maxUnitWireLabelBytes each).
	RepresentativeLabels []string `json:"representative_labels,omitempty"`
	// AnchorRefCount bounds anchors to a count on the wire.
	AnchorRefCount int `json:"anchor_ref_count,omitempty"`
	// RelationOutCount aggregates outgoing structural relations.
	RelationOutCount int `json:"relation_out_count,omitempty"`
}

// UnitCatalog is the compiled local unit catalog plus complete coverage.
type UnitCatalog struct {
	Version   int                `json:"version"`
	Units     []ArchitectureUnit `json:"units"`
	WireUnits []SynthesisUnit    `json:"wire_units,omitempty"`
	// MemberToWireUnit is the exact private ownership index after final unit
	// splitting and sorting. It is never serialized or sent to a provider.
	MemberToWireUnit map[MemberID]UnitWireRef `json:"-"`
	CoveredMembers   int                      `json:"covered_members"`
	TotalMembers     int                      `json:"total_members"`
	OmittedMembers   int                      `json:"omitted_members,omitempty"`
	OmittedRoles     []UnitRole               `json:"omitted_roles,omitempty"`
	SHA256           string                   `json:"sha256"`
}

// CompileUnitCatalog deterministically compiles every exact raw conceptual
// member into a bounded ArchitectureUnit catalog (Decision 216).
func CompileUnitCatalog(bundle CandidateBundle) (UnitCatalog, error) {
	if err := bundle.Validate(); err != nil {
		return UnitCatalog{}, err
	}
	known := candidateIndex(bundle)
	canonicalOpaqueIDs := make(map[string]struct{}, len(bundle.Candidates)+len(bundle.BehaviorAnchors))
	candidateNames := make(map[MemberID]string, len(bundle.Candidates))
	for _, candidate := range bundle.Candidates {
		canonicalOpaqueIDs[candidate.ID.Value] = struct{}{}
		candidateNames[candidate.ID] = candidate.Name
	}
	for _, anchor := range bundle.BehaviorAnchors {
		canonicalOpaqueIDs[anchor.ID] = struct{}{}
	}
	// 1. Separate by role: production, tests/integration, tooling/contrib,
	//    examples/docs. Role is derived from exact package paths and member
	//    names, never from model prose.
	units := make([]ArchitectureUnit, 0)

	// Seed package units from every conceptual package candidate.
	packageCandidates := make([]Candidate, 0)
	nonPackageCandidates := make([]Candidate, 0)
	for _, candidate := range bundle.Candidates {
		if candidate.Role != CandidateRoleConceptualMember {
			continue
		}
		switch candidate.ID.Kind {
		case MemberPackage:
			packageCandidates = append(packageCandidates, candidate)
		case MemberSymbol, MemberFile, MemberEntrypoint, MemberFlow:
			nonPackageCandidates = append(nonPackageCandidates, candidate)
		}
	}
	sort.Slice(packageCandidates, func(i, j int) bool {
		return packageCandidates[i].ID.Value < packageCandidates[j].ID.Value
	})
	sort.Slice(nonPackageCandidates, func(i, j int) bool {
		if nonPackageCandidates[i].ID.Kind != nonPackageCandidates[j].ID.Kind {
			return nonPackageCandidates[i].ID.Kind < nonPackageCandidates[j].ID.Kind
		}
		return nonPackageCandidates[i].ID.Value < nonPackageCandidates[j].ID.Value
	})
	packagePrefix := commonPackageDeclarationPrefix(packageCandidates)
	relativePackageNames := make(map[MemberID]string, len(packageCandidates))
	for _, candidate := range packageCandidates {
		relativePackageNames[candidate.ID] = repositoryRelativePackageName(candidate, packagePrefix)
	}

	// Resolve conceptual ownership once through the finite, already advertised
	// candidate graph. Only structural locators may bridge a non-package
	// conceptual member to its nearest conceptual package owner.
	packageOwnerByMember := make(map[MemberID]MemberID, len(packageCandidates)+len(nonPackageCandidates))
	for _, candidate := range append(append([]Candidate(nil), packageCandidates...), nonPackageCandidates...) {
		if owner, ok := nearestConceptualPackageOwner(candidate.ID, known); ok {
			packageOwnerByMember[candidate.ID] = owner
		}
	}

	// packageUnitIndex maps an exact package member to its index in units;
	// attach operations mutate units[index] directly so membership is never
	// lost to slice copies.
	packageUnitIndex := map[MemberID]int{}
	// Anchor-bearing packages stay as individual seed units (goal 4: seed
	// around exact process/library entries and behavior anchors).
	anchorMemberPackages := map[MemberID]bool{}
	for _, anchor := range bundle.BehaviorAnchors {
		for _, memberID := range anchor.MemberIDs {
			if owner, ok := packageOwnerByMember[memberID]; ok {
				anchorMemberPackages[owner] = true
			}
		}
	}
	processEntryPackages := map[MemberID]bool{}
	for _, candidate := range nonPackageCandidates {
		if candidate.ID.Kind != MemberEntrypoint {
			continue
		}
		if owner, ok := packageOwnerByMember[candidate.ID]; ok {
			processEntryPackages[owner] = true
		}
	}
	for _, anchor := range bundle.BehaviorAnchors {
		if anchor.Kind != AnchorProcessEntry {
			continue
		}
		for _, memberID := range anchor.MemberIDs {
			if owner, ok := packageOwnerByMember[memberID]; ok {
				processEntryPackages[owner] = true
			}
		}
	}
	for _, candidate := range packageCandidates {
		// Decision 216.4: cluster remaining packages by top-level/module
		// structure. Only exact package paths that share a top-level module
		// segment merge; anchor-bearing or process-entry packages are never
		// merged away.
		if !anchorMemberPackages[candidate.ID] && !processEntryPackages[candidate.ID] {
			continue
		}
		relativeName := relativePackageNames[candidate.ID]
		role := unitRoleForPackage(candidate, relativeName)
		unit := ArchitectureUnit{
			CanonicalID:  "unit-" + stableUnitID(candidate),
			Role:         role,
			Label:        sanitizeUnitLabel(unitLabelForPackage(relativeName), canonicalOpaqueIDs),
			MemberIDs:    []MemberID{candidate.ID},
			MemberKinds:  map[MemberKind]int{candidate.ID.Kind: 1},
			PackagePaths: []string{candidate.ID.Value},
		}
		packageUnitIndex[candidate.ID] = len(units)
		units = append(units, unit)
	}
	// Cluster non-seed packages by top-level module path.
	moduleUnits := map[string]int{}
	for _, candidate := range packageCandidates {
		if _, seeded := packageUnitIndex[candidate.ID]; seeded {
			continue
		}
		relativeName := relativePackageNames[candidate.ID]
		module := topLevelModule(relativeName)
		role := unitRoleForPackage(candidate, relativeName)
		if module == "" {
			unit := ArchitectureUnit{
				CanonicalID:  "unit-" + stableUnitID(candidate),
				Role:         role,
				Label:        sanitizeUnitLabel(unitLabelForPackage(relativeName), canonicalOpaqueIDs),
				MemberIDs:    []MemberID{candidate.ID},
				MemberKinds:  map[MemberKind]int{candidate.ID.Kind: 1},
				PackagePaths: []string{candidate.ID.Value},
			}
			packageUnitIndex[candidate.ID] = len(units)
			units = append(units, unit)
			continue
		}
		moduleKey := string(role) + "\x00" + module
		unitIndex, exists := moduleUnits[moduleKey]
		if !exists {
			unit := ArchitectureUnit{
				CanonicalID:  "unit-module-" + stableModuleID(role, module),
				Role:         role,
				Label:        sanitizeUnitLabel(module, canonicalOpaqueIDs),
				MemberIDs:    []MemberID{candidate.ID},
				MemberKinds:  map[MemberKind]int{candidate.ID.Kind: 1},
				PackagePaths: []string{candidate.ID.Value},
			}
			packageUnitIndex[candidate.ID] = len(units)
			moduleUnits[moduleKey] = len(units)
			units = append(units, unit)
			continue
		}
		unit := &units[unitIndex]
		unit.MemberIDs = append(unit.MemberIDs, candidate.ID)
		unit.MemberKinds[candidate.ID.Kind]++
		unit.PackagePaths = append(unit.PackagePaths, candidate.ID.Value)
		packageUnitIndex[candidate.ID] = unitIndex
	}

	// Attach non-package conceptual members to their exact owning package unit
	// through the bounded structural-locator parent chain; unresolved members
	// form an explicit local remainder.
	unattached := make([]Candidate, 0)
	for _, candidate := range nonPackageCandidates {
		owner, resolved := packageOwnerByMember[candidate.ID]
		if !resolved {
			unattached = append(unattached, candidate)
			continue
		}
		unitIndex, exists := packageUnitIndex[owner]
		if !exists {
			unattached = append(unattached, candidate)
			continue
		}
		unit := &units[unitIndex]
		unit.MemberIDs = append(unit.MemberIDs, candidate.ID)
		unit.MemberKinds[candidate.ID.Kind]++
	}

	// Preserve every conceptual member in exactly one primary unit. Unattached
	// members become one explicit local remainder with complete coverage
	// accounting (no silent first-N).
	omitted := 0
	omittedRoles := map[UnitRole]bool{}
	if len(unattached) > 0 {
		role := UnitRoleProduction
		remainder := ArchitectureUnit{
			CanonicalID: "unit-local-remainder",
			Role:        role,
			Label:       "local remainder",
			MemberIDs:   make([]MemberID, 0, len(unattached)),
			MemberKinds: map[MemberKind]int{},
		}
		for _, candidate := range unattached {
			remainder.MemberIDs = append(remainder.MemberIDs, candidate.ID)
			remainder.MemberKinds[candidate.ID.Kind]++
		}
		sort.Slice(remainder.MemberIDs, func(i, j int) bool {
			return remainder.MemberIDs[i].key() < remainder.MemberIDs[j].key()
		})
		units = append(units, remainder)
	}

	// Split every oversized unit, including the local remainder,
	// deterministically before anchors or request-local refs are attached.
	split := make([]ArchitectureUnit, 0, len(units))
	for _, unit := range units {
		if len(unit.MemberIDs) <= maxUnitMembers {
			split = append(split, unit)
			continue
		}
		chunks := chunkMemberIDs(unit.MemberIDs, maxUnitMembers)
		for chunkIndex, chunk := range chunks {
			sub := unit
			sub.CanonicalID = fmt.Sprintf("%s-%d", unit.CanonicalID, chunkIndex+1)
			sub.MemberIDs = chunk
			sub.MemberKinds = memberKindCounts(chunk)
			sub.AnchorIDs = nil
			split = append(split, sub)
		}
	}
	units = split

	// Sort units deterministically by canonical ID.
	sort.Slice(units, func(i, j int) bool { return units[i].CanonicalID < units[j].CanonicalID })
	memberToFinalUnit := make(map[MemberID]int, len(packageCandidates)+len(nonPackageCandidates))
	memberToWireUnit := make(map[MemberID]UnitWireRef, len(packageCandidates)+len(nonPackageCandidates))
	for index := range units {
		unit := &units[index]
		sort.Slice(unit.MemberIDs, func(i, j int) bool {
			return unit.MemberIDs[i].key() < unit.MemberIDs[j].key()
		})
		wireRef := UnitWireRef(fmt.Sprintf("u%d", index+1))
		for _, memberID := range unit.MemberIDs {
			memberToFinalUnit[memberID] = index
			memberToWireUnit[memberID] = wireRef
		}
	}

	// Attach every anchor only after final remainder construction, splitting,
	// and sorting. An anchor spanning final units is retained on each exact
	// unit that contains one of its members.
	for _, anchor := range bundle.BehaviorAnchors {
		for _, memberID := range anchor.MemberIDs {
			unitIndex, exists := memberToFinalUnit[memberID]
			if !exists {
				continue
			}
			unit := &units[unitIndex]
			if !containsString(unit.AnchorIDs, anchor.ID) {
				unit.AnchorIDs = append(unit.AnchorIDs, anchor.ID)
			}
		}
	}
	for index := range units {
		unit := &units[index]
		sort.Strings(unit.AnchorIDs)
		unit.ExpansionDigest = unitExpansionDigest(*unit)
	}

	conceptualMemberCount := len(packageCandidates) + len(nonPackageCandidates)
	catalog := UnitCatalog{
		Version:          UnitCatalogVersion,
		Units:            units,
		MemberToWireUnit: memberToWireUnit,
		CoveredMembers:   conceptualMemberCount,
		TotalMembers:     conceptualMemberCount,
		OmittedMembers:   omitted,
	}
	for role := range omittedRoles {
		catalog.OmittedRoles = append(catalog.OmittedRoles, role)
	}
	sort.Slice(catalog.OmittedRoles, func(i, j int) bool { return catalog.OmittedRoles[i] < catalog.OmittedRoles[j] })
	// Decision 223: fill the per-unit outgoing package-import aggregate that
	// Decision 216 promised (projectUnitWire used to hardcode 0). Count
	// exact package_import relations whose source member belongs to the
	// unit and whose target member belongs to a different unit — the raw
	// edges are dropped from the wire in favor of this aggregate.
	outCounts := unitOutgoingRelationCounts(bundle.Relations, units)
	catalog.WireUnits = projectUnitWire(units, candidateNames, canonicalOpaqueIDs, outCounts)
	catalog.SHA256 = catalogDigest(catalog.Units)
	return catalog, nil
}

// unitOutgoingRelationCounts counts, per unit canonical ID, the exact
// outgoing package-import relations from members of the unit to members of
// other units. It resolves membership from the final post-split units.
// Deterministic and provider-free; only the wire aggregate is published,
// never the raw edges.
func unitOutgoingRelationCounts(
	relations []LocalRelation,
	units []ArchitectureUnit,
) map[string]int {
	// Map every exact member to its final post-split unit canonical ID.
	memberToFinalUnit := make(map[MemberID]string)
	for _, unit := range units {
		for _, memberID := range unit.MemberIDs {
			memberToFinalUnit[memberID] = unit.CanonicalID
		}
	}
	outCounts := make(map[string]int, len(units))
	for _, relation := range relations {
		if relation.Kind != StructuralRelationPackageImport {
			continue
		}
		fromUnit, fromOK := memberToFinalUnit[relation.From]
		toUnit, toOK := memberToFinalUnit[relation.To]
		if !fromOK || !toOK || fromUnit == toUnit {
			continue
		}
		outCounts[fromUnit]++
	}
	return outCounts
}

// projectUnitWire builds the bounded provider-visible unit projection.
func projectUnitWire(
	units []ArchitectureUnit,
	candidateNames map[MemberID]string,
	canonicalOpaqueIDs map[string]struct{},
	outCounts map[string]int,
) []SynthesisUnit {
	wire := make([]SynthesisUnit, 0, len(units))
	for index, unit := range units {
		labels := representativeLabels(unit, candidateNames, 4, canonicalOpaqueIDs)
		label := sanitizeUnitLabel(unit.Label, canonicalOpaqueIDs)
		if label == "" {
			label = "package"
		}
		wire = append(wire, SynthesisUnit{
			Ref:                  UnitWireRef(fmt.Sprintf("u%d", index+1)),
			Label:                truncateUTF8Bytes(label, maxUnitWireLabelBytes),
			Role:                 unit.Role,
			MemberKindCounts:     unit.MemberKinds,
			RepresentativeLabels: labels,
			AnchorRefCount:       len(unit.AnchorIDs),
			RelationOutCount:     outCounts[unit.CanonicalID],
		})
	}
	return wire
}

// representativeLabels returns bounded member-name labels for the unit.
// Labels come from exact candidate display names, never canonical member
// IDs (Decision 216: canonical identity stays private).
func representativeLabels(
	unit ArchitectureUnit,
	candidateNames map[MemberID]string,
	limit int,
	canonicalOpaqueIDs map[string]struct{},
) []string {
	labels := make([]string, 0, limit)
	seen := map[string]bool{}
	for _, memberID := range unit.MemberIDs {
		name, exists := candidateNames[memberID]
		if !exists {
			continue
		}
		label := representativeCandidateLabel(memberID.Kind, name, canonicalOpaqueIDs)
		if label == "" {
			continue
		}
		if len(label) > maxUnitWireLabelBytes {
			label = truncateUTF8Bytes(label, maxUnitWireLabelBytes)
		}
		if seen[label] {
			continue
		}
		seen[label] = true
		labels = append(labels, label)
		if len(labels) >= limit {
			break
		}
	}
	return labels
}

func representativeCandidateLabel(
	kind MemberKind,
	name string,
	canonicalOpaqueIDs map[string]struct{},
) string {
	label := sanitizeUnitLabel(name, canonicalOpaqueIDs)
	if label == "" {
		return ""
	}
	label = strings.ReplaceAll(label, "\\", "/")
	if kind == MemberFile && !strings.Contains(label, "/") {
		// File display names are repository-relative paths. A root-level file
		// has no safe path-free semantic projection.
		return ""
	}
	if strings.Contains(label, "/") {
		label = path.Base(strings.TrimSuffix(label, "/"))
	}
	if kind == MemberSymbol || kind == MemberEntrypoint {
		for _, separator := range []string{"::", "#", "."} {
			if index := strings.LastIndex(label, separator); index >= 0 {
				label = label[index+len(separator):]
			}
		}
	}
	label = sanitizeUnitLabel(label, canonicalOpaqueIDs)
	if label == "" {
		return ""
	}
	return truncateUTF8Bytes(label, maxUnitWireLabelBytes)
}

// nearestConceptualPackageOwner follows only the finite candidate graph that
// is already present in the bundle. The starting candidate may be conceptual;
// every intermediate non-package node must be a structural locator.
func nearestConceptualPackageOwner(
	memberID MemberID,
	known map[MemberID]Candidate,
) (MemberID, bool) {
	currentID := memberID
	seen := make(map[MemberID]struct{})
	for step := 0; step < len(known); step++ {
		if _, duplicate := seen[currentID]; duplicate {
			return MemberID{}, false
		}
		seen[currentID] = struct{}{}
		candidate, exists := known[currentID]
		if !exists {
			return MemberID{}, false
		}
		if candidate.Role == CandidateRoleConceptualMember && candidate.ID.Kind == MemberPackage {
			return candidate.ID, true
		}
		if currentID != memberID && candidate.Role != CandidateRoleStructuralLocator {
			return MemberID{}, false
		}
		if candidate.ParentID == nil {
			return MemberID{}, false
		}
		currentID = *candidate.ParentID
	}
	return MemberID{}, false
}

// commonPackageDeclarationPrefix identifies only a source-host-qualified
// common prefix (for example github.com/org/repository). Relative repository
// paths are deliberately not reinterpreted as hosts or organizations.
func commonPackageDeclarationPrefix(candidates []Candidate) string {
	var common []string
	for _, candidate := range candidates {
		declaration := packageDeclarationPath(candidate)
		if !qualifiedPackagePath(declaration) {
			continue
		}
		segments := strings.Split(declaration, "/")
		if common == nil {
			common = append([]string(nil), segments...)
			continue
		}
		limit := len(common)
		if len(segments) < limit {
			limit = len(segments)
		}
		matched := 0
		for matched < limit && common[matched] == segments[matched] {
			matched++
		}
		common = common[:matched]
	}
	// A host-only prefix would make the next segment (commonly an
	// organization) provider-visible. Require at least one repository/module
	// segment beyond the source host before deriving a relative name.
	if len(common) < 2 {
		return ""
	}
	return strings.Join(common, "/")
}

func repositoryRelativePackageName(candidate Candidate, packagePrefix string) string {
	name := cleanPackageDisplayPath(candidate.Name)
	declaration := packageDeclarationPath(candidate)
	if qualifiedPackagePath(declaration) && packagePrefix != "" {
		if declaration == packagePrefix {
			return path.Base(packagePrefix)
		}
		if strings.HasPrefix(declaration, packagePrefix+"/") {
			return strings.TrimPrefix(declaration, packagePrefix+"/")
		}
	}
	if name != "" && !qualifiedPackagePath(name) {
		return name
	}
	if name != "" {
		return path.Base(name)
	}
	if declaration != "" {
		return path.Base(declaration)
	}
	return ""
}

func packageDeclarationPath(candidate Candidate) string {
	for _, fact := range candidate.Facts {
		if fact.Kind != FactDeclaration {
			continue
		}
		if value := cleanPackageDisplayPath(fact.Value); value != "" {
			return value
		}
	}
	return ""
}

func cleanPackageDisplayPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.HasPrefix(value, "member-") || strings.Contains(value, "\\") ||
		strings.HasPrefix(value, "/") || strings.ContainsAny(value, "\t\r\n ") {
		return ""
	}
	cleaned := path.Clean(strings.TrimSuffix(value, "/"))
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return ""
	}
	return cleaned
}

func qualifiedPackagePath(value string) bool {
	if value == "" {
		return false
	}
	first := value
	if index := strings.IndexByte(first, '/'); index >= 0 {
		first = first[:index]
	}
	return strings.Contains(first, ".") || strings.Contains(first, ":")
}

func packageRoleSegments(value string) []string {
	value = strings.ToLower(strings.ReplaceAll(value, "\\", "/"))
	raw := strings.Split(value, "/")
	segments := make([]string, 0, len(raw))
	for _, segment := range raw {
		segment = strings.TrimSpace(segment)
		if segment != "" && segment != "." && segment != ".." {
			segments = append(segments, segment)
		}
	}
	return segments
}

func containsExactSegment(segments []string, targets ...string) bool {
	for _, segment := range segments {
		for _, target := range targets {
			if segment == target {
				return true
			}
		}
	}
	return false
}

func containsSegmentSuffix(segments []string, suffix string) bool {
	for _, segment := range segments {
		if strings.HasSuffix(segment, suffix) {
			return true
		}
	}
	return false
}

// unitRoleForPackage classifies a package only from closed local facts and
// exact repository-relative path segments. Opaque IDs and provider prose are
// never interpreted.
func unitRoleForPackage(candidate Candidate, relativeName string) UnitRole {
	for _, fact := range candidate.Facts {
		if fact.Kind != FactExecutableRole {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(fact.Value)) {
		case "test_or_helper":
			return UnitRoleTest
		case "tooling", "secondary_tooling":
			return UnitRoleTooling
		case "primary_application", "secondary_service":
			return UnitRoleProduction
		}
	}
	segments := append(packageRoleSegments(relativeName), packageRoleSegments(candidate.Name)...)
	if containsExactSegment(segments,
		"test", "tests", "testing", "testutil", "testutils", "testdata",
		"integration", "e2e", "fixture", "fixtures", "mock", "mocks",
	) || containsSegmentSuffix(segments, "_test") {
		return UnitRoleTest
	}
	if containsExactSegment(segments,
		"contrib", "example", "examples", "tool", "tools", "tooling",
		"hack", "script", "scripts", "generator", "generators",
		"benchmark", "benchmarks", "bench",
	) {
		return UnitRoleTooling
	}
	if containsExactSegment(segments, "doc", "docs", "documentation") {
		return UnitRoleDocumentation
	}
	return UnitRoleProduction
}

// unitLabelForPackage derives a short semantic label from the last path
// segment of a package display name. Canonical member IDs are never used as
// labels (Decision 216: canonical identity stays private).
func unitLabelForPackage(displayName string) string {
	name := strings.TrimSpace(displayName)
	if name == "" || strings.HasPrefix(name, "member-") {
		return "package"
	}
	trimmed := strings.TrimSuffix(name, "/")
	base := path.Base(trimmed)
	if base == "." || base == "/" || base == "" {
		return name
	}
	return base
}

// topLevelModule returns one repository-relative semantic segment. Command
// packages use the command name rather than the generic cmd directory.
func topLevelModule(displayName string) string {
	name := strings.TrimSuffix(strings.TrimSpace(displayName), "/")
	if name == "" || strings.HasPrefix(name, "member-") {
		return ""
	}
	segments := strings.Split(name, "/")
	if len(segments) == 0 {
		return ""
	}
	module := segments[0]
	if module == "cmd" && len(segments) > 1 {
		module = segments[1]
	}
	if module == "" || module == "." || module == ".." || strings.Contains(module, ".") {
		return ""
	}
	return module
}

// stableModuleID derives a stable identity for a module-cluster unit.
func stableModuleID(role UnitRole, module string) string {
	hash := sha256.New()
	fmt.Fprintf(hash, "module-v%d/%s/%s\n", UnitCatalogVersion, role, module)
	return hex.EncodeToString(hash.Sum(nil)[:12])
}

// sanitizeUnitLabel removes canonical opaque ID tokens from a unit label so
// the provider wire never carries canonical identity (Decision 216).
func sanitizeUnitLabel(value string, canonicalOpaqueIDs map[string]struct{}) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	fields := strings.Fields(trimmed)
	kept := make([]string, 0, len(fields))
	for _, field := range fields {
		containsCanonical := false
		for canonical := range canonicalOpaqueIDs {
			if canonical != "" && strings.Contains(field, canonical) {
				containsCanonical = true
				break
			}
		}
		if containsCanonical {
			continue
		}
		kept = append(kept, field)
	}
	return strings.Join(kept, " ")
}

// stableUnitID derives a stable unit identity from the package candidate.
func stableUnitID(candidate Candidate) string {
	hash := sha256.New()
	fmt.Fprintf(hash, "unit-v%d/%s/%s\n", UnitCatalogVersion, candidate.ID.Kind, candidate.ID.Value)
	return hex.EncodeToString(hash.Sum(nil)[:12])
}

// unitExpansionDigest binds the exact member set of one unit.
func unitExpansionDigest(unit ArchitectureUnit) string {
	hash := sha256.New()
	for _, memberID := range unit.MemberIDs {
		fmt.Fprintf(hash, "%s\n", memberID.key())
	}
	return hex.EncodeToString(hash.Sum(nil)[:12])
}

// catalogDigest binds the complete unit catalog deterministically.
func catalogDigest(units []ArchitectureUnit) string {
	hash := sha256.New()
	for _, unit := range units {
		fmt.Fprintf(hash, "%s/%s/%s\n", unit.CanonicalID, unit.Role, unit.ExpansionDigest)
	}
	return hex.EncodeToString(hash.Sum(nil)[:16])
}

func chunkMemberIDs(members []MemberID, size int) [][]MemberID {
	sorted := append([]MemberID(nil), members...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].key() < sorted[j].key() })
	chunks := make([][]MemberID, 0, (len(sorted)+size-1)/size)
	for len(sorted) > 0 {
		n := size
		if len(sorted) < n {
			n = len(sorted)
		}
		chunks = append(chunks, sorted[:n])
		sorted = sorted[n:]
	}
	return chunks
}

func memberKindCounts(members []MemberID) map[MemberKind]int {
	counts := map[MemberKind]int{}
	for _, memberID := range members {
		counts[memberID.Kind]++
	}
	return counts
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func truncateUTF8Bytes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if len(value) <= limit {
		return value
	}
	cut := limit
	for cut > 0 && !utf8.ValidString(value[:cut]) {
		cut--
	}
	return value[:cut]
}

// UnitWireRefsFromCatalog maps canonical unit IDs to request-local refs in
// canonical order (deterministic regardless of input ordering).
func UnitWireRefsFromCatalog(catalog UnitCatalog) map[string]UnitWireRef {
	result := make(map[string]UnitWireRef, len(catalog.Units))
	for index, unit := range catalog.Units {
		result[unit.CanonicalID] = UnitWireRef(fmt.Sprintf("u%d", index+1))
	}
	return result
}

// unitCatalogUnitMembersByWireRef maps wire unit refs (u*) to their exact
// member IDs for local expansion (Decision 216). Wire refs are positional:
// unit i in the catalog corresponds to u<i+1>.
func unitCatalogUnitMembersByWireRef(catalog UnitCatalog) map[string][]MemberID {
	result := make(map[string][]MemberID, len(catalog.Units))
	for index, unit := range catalog.Units {
		ref := fmt.Sprintf("u%d", index+1)
		result[ref] = append([]MemberID(nil), unit.MemberIDs...)
	}
	return result
}

// Evidence is unused in this file directly but kept for future overlay use.
var _ = evidence.CertaintyStatic
