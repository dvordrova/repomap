package componentmap

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/evidence"
)

// Decision 216: bounded local ArchitectureUnit catalog. The model groups
// request-local unit refs (u*) instead of raw package/symbol members; backend
// expansion restores exact membership and coverage.

const (
	// UnitCatalogVersion changes whenever unit identity, role separation, or
	// the unit compiler contract changes.
	UnitCatalogVersion = 1

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
	Version        int                `json:"version"`
	Units          []ArchitectureUnit `json:"units"`
	WireUnits      []SynthesisUnit    `json:"wire_units,omitempty"`
	CoveredMembers int                `json:"covered_members"`
	TotalMembers   int                `json:"total_members"`
	OmittedMembers int                `json:"omitted_members,omitempty"`
	OmittedRoles   []UnitRole         `json:"omitted_roles,omitempty"`
	SHA256         string             `json:"sha256"`
}

// CompileUnitCatalog deterministically compiles every exact raw conceptual
// member into a bounded ArchitectureUnit catalog (Decision 216).
func CompileUnitCatalog(bundle CandidateBundle) (UnitCatalog, error) {
	if err := bundle.Validate(); err != nil {
		return UnitCatalog{}, err
	}
	canonicalOpaqueIDs := make(map[string]struct{}, len(bundle.Candidates)+len(bundle.BehaviorAnchors))
	for _, candidate := range bundle.Candidates {
		canonicalOpaqueIDs[candidate.ID.Value] = struct{}{}
	}
	for _, anchor := range bundle.BehaviorAnchors {
		canonicalOpaqueIDs[anchor.ID] = struct{}{}
	}
	// 1. Separate by role: production, tests/integration, tooling/contrib,
	//    examples/docs. Role is derived from exact package paths and member
	//    names, never from model prose.
	units := make([]ArchitectureUnit, 0)
	memberToUnit := map[MemberID]string{}

	// Seed package units from every conceptual package candidate.
	packageCandidates := make([]Candidate, 0)
	symbolCandidates := make([]Candidate, 0)
	for _, candidate := range bundle.Candidates {
		if candidate.Role != CandidateRoleConceptualMember {
			continue
		}
		switch candidate.ID.Kind {
		case MemberPackage:
			packageCandidates = append(packageCandidates, candidate)
		case MemberSymbol, MemberFile, MemberEntrypoint:
			symbolCandidates = append(symbolCandidates, candidate)
		}
	}
	sort.Slice(packageCandidates, func(i, j int) bool {
		return packageCandidates[i].ID.Value < packageCandidates[j].ID.Value
	})
	sort.Slice(symbolCandidates, func(i, j int) bool {
		if symbolCandidates[i].ID.Kind != symbolCandidates[j].ID.Kind {
			return symbolCandidates[i].ID.Kind < symbolCandidates[j].ID.Kind
		}
		return symbolCandidates[i].ID.Value < symbolCandidates[j].ID.Value
	})

	// packageUnitIndex maps a package canonical value to its index in units;
	// attach operations mutate units[index] directly so membership is never
	// lost to slice copies.
	packageUnitIndex := map[string]int{}
	// Anchor-bearing packages stay as individual seed units (goal 4: seed
	// around exact process/library entries and behavior anchors).
	anchorMemberPackages := map[string]bool{}
	for _, anchor := range bundle.BehaviorAnchors {
		for _, memberID := range anchor.MemberIDs {
			if memberID.Kind == MemberPackage {
				anchorMemberPackages[memberID.Value] = true
			}
		}
	}
	for _, candidate := range packageCandidates {
		// Decision 216.4: cluster remaining packages by top-level/module
		// structure. Only exact package paths that share a top-level module
		// segment merge; anchor-bearing or process-entry packages are never
		// merged away.
		if !anchorMemberPackages[candidate.ID.Value] && !processEntryPackage(bundle, candidate.ID.Value) {
			continue
		}
		role := unitRoleForPackage(candidate.Name, candidate.ID.Value)
		unit := ArchitectureUnit{
			CanonicalID:  "unit-" + stableUnitID(candidate),
			Role:         role,
			Label:        sanitizeUnitLabel(unitLabelForPackage(candidate.Name), canonicalOpaqueIDs),
			MemberIDs:    []MemberID{candidate.ID},
			MemberKinds:  map[MemberKind]int{candidate.ID.Kind: 1},
			PackagePaths: []string{candidate.ID.Value},
		}
		packageUnitIndex[candidate.ID.Value] = len(units)
		memberToUnit[candidate.ID] = unit.CanonicalID
		units = append(units, unit)
	}
	// Cluster non-seed packages by top-level module path.
	moduleUnits := map[string]int{}
	for _, candidate := range packageCandidates {
		if _, seeded := packageUnitIndex[candidate.ID.Value]; seeded {
			continue
		}
		module := topLevelModule(candidate.Name, candidate.ID.Value)
		if module == "" {
			module = packageModuleFromFacts(candidate)
		}
		if module == "" {
			role := unitRoleForPackage(candidate.Name, candidate.ID.Value)
			unit := ArchitectureUnit{
				CanonicalID:  "unit-" + stableUnitID(candidate),
				Role:         role,
				Label:        sanitizeUnitLabel(unitLabelForPackage(candidate.Name), canonicalOpaqueIDs),
				MemberIDs:    []MemberID{candidate.ID},
				MemberKinds:  map[MemberKind]int{candidate.ID.Kind: 1},
				PackagePaths: []string{candidate.ID.Value},
			}
			packageUnitIndex[candidate.ID.Value] = len(units)
			memberToUnit[candidate.ID] = unit.CanonicalID
			units = append(units, unit)
			continue
		}
		unitIndex, exists := moduleUnits[module]
		if !exists {
			role := unitRoleForPackage(candidate.Name, candidate.ID.Value)
			unit := ArchitectureUnit{
				CanonicalID:  "unit-module-" + stableModuleID(module),
				Role:         role,
				Label:        sanitizeUnitLabel(module, canonicalOpaqueIDs),
				MemberIDs:    []MemberID{candidate.ID},
				MemberKinds:  map[MemberKind]int{candidate.ID.Kind: 1},
				PackagePaths: []string{candidate.ID.Value},
			}
			packageUnitIndex[candidate.ID.Value] = len(units)
			memberToUnit[candidate.ID] = unit.CanonicalID
			moduleUnits[module] = len(units)
			units = append(units, unit)
			continue
		}
		unit := &units[unitIndex]
		unit.MemberIDs = append(unit.MemberIDs, candidate.ID)
		unit.MemberKinds[candidate.ID.Kind]++
		unit.PackagePaths = append(unit.PackagePaths, candidate.ID.Value)
		packageUnitIndex[candidate.ID.Value] = unitIndex
		memberToUnit[candidate.ID] = unit.CanonicalID
	}

	// 3. Attach symbols/files/entrypoints to their exact owning package unit
	//    via ParentID; members without an owning package form a local
	//    remainder unit only when no package parent exists.
	unattached := make([]Candidate, 0)
	for _, candidate := range symbolCandidates {
		if candidate.ParentID == nil || candidate.ParentID.Kind != MemberPackage {
			unattached = append(unattached, candidate)
			continue
		}
		unitIndex, exists := packageUnitIndex[candidate.ParentID.Value]
		if !exists {
			unattached = append(unattached, candidate)
			continue
		}
		unit := &units[unitIndex]
		unit.MemberIDs = append(unit.MemberIDs, candidate.ID)
		unit.MemberKinds[candidate.ID.Kind]++
		memberToUnit[candidate.ID] = unit.CanonicalID
	}
	// Attach anchors to the package of their first member, else leave them
	// unit-less (they remain exact evidence on the response).
	anchorByMember := map[string][]string{}
	for _, anchor := range bundle.BehaviorAnchors {
		for _, memberID := range anchor.MemberIDs {
			anchorByMember[memberID.key()] = append(anchorByMember[memberID.key()], anchor.ID)
		}
	}
	for index := range units {
		unit := &units[index]
		for _, memberID := range unit.MemberIDs {
			for _, anchorID := range anchorByMember[memberID.key()] {
				if !containsString(unit.AnchorIDs, anchorID) {
					unit.AnchorIDs = append(unit.AnchorIDs, anchorID)
				}
			}
		}
		sort.Strings(unit.AnchorIDs)
	}

	// 5. Split oversized units deterministically (stable by member value).
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
			split = append(split, sub)
		}
	}
	units = split

	// 6/7. Preserve every conceptual member in exactly one primary unit;
	//      unattached members become one explicit local remainder with
	//      complete coverage accounting (no silent first-N).
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
			memberToUnit[candidate.ID] = remainder.CanonicalID
		}
		sort.Slice(remainder.MemberIDs, func(i, j int) bool {
			return remainder.MemberIDs[i].key() < remainder.MemberIDs[j].key()
		})
		units = append(units, remainder)
	}

	// Sort units deterministically by canonical ID.
	sort.Slice(units, func(i, j int) bool { return units[i].CanonicalID < units[j].CanonicalID })
	// Compute expansion digests and wire projections.
	for index := range units {
		unit := &units[index]
		sort.Slice(unit.MemberIDs, func(i, j int) bool {
			return unit.MemberIDs[i].key() < unit.MemberIDs[j].key()
		})
		unit.ExpansionDigest = unitExpansionDigest(*unit)
	}

	catalog := UnitCatalog{
		Version:        UnitCatalogVersion,
		Units:          units,
		CoveredMembers: len(bundle.Candidates),
		TotalMembers:   len(bundle.Candidates),
		OmittedMembers: omitted,
	}
	for role := range omittedRoles {
		catalog.OmittedRoles = append(catalog.OmittedRoles, role)
	}
	sort.Slice(catalog.OmittedRoles, func(i, j int) bool { return catalog.OmittedRoles[i] < catalog.OmittedRoles[j] })
	catalog.WireUnits = projectUnitWire(units, canonicalOpaqueIDs)
	catalog.SHA256 = catalogDigest(catalog.Units)
	return catalog, nil
}

// projectUnitWire builds the bounded provider-visible unit projection.
func projectUnitWire(units []ArchitectureUnit, canonicalOpaqueIDs map[string]struct{}) []SynthesisUnit {
	wire := make([]SynthesisUnit, 0, len(units))
	for index, unit := range units {
		labels := representativeLabels(unit, 4, canonicalOpaqueIDs)
		label := sanitizeUnitLabel(unit.Label, canonicalOpaqueIDs)
		if label == "" {
			label = "package"
		}
		wire = append(wire, SynthesisUnit{
			Ref:                  UnitWireRef(fmt.Sprintf("u%d", index+1)),
			Label:                truncateRunes(label, maxUnitWireLabelBytes),
			Role:                 unit.Role,
			MemberKindCounts:     unit.MemberKinds,
			RepresentativeLabels: labels,
			AnchorRefCount:       len(unit.AnchorIDs),
			RelationOutCount:     0,
		})
	}
	return wire
}

// representativeLabels returns bounded member-name labels for the unit.
// Labels come from exact candidate display names, never canonical member
// IDs (Decision 216: canonical identity stays private).
func representativeLabels(unit ArchitectureUnit, limit int, canonicalOpaqueIDs map[string]struct{}) []string {
	labels := make([]string, 0, limit)
	seen := map[string]bool{}
	for _, memberID := range unit.MemberIDs {
		label := memberID.Value
		if strings.HasPrefix(label, "member-") {
			continue
		}
		label = sanitizeUnitLabel(label, canonicalOpaqueIDs)
		if label == "" {
			continue
		}
		if len(label) > maxUnitWireLabelBytes {
			label = truncateRunes(label, maxUnitWireLabelBytes)
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

// unitRoleForPackage classifies a package path by exact role markers.
func unitRoleForPackage(displayName, packagePath string) UnitRole {
	lower := strings.ToLower(packagePath + " " + displayName)
	if strings.Contains(lower, "/test") || strings.Contains(lower, "_test") ||
		strings.Contains(lower, "/tests/") || strings.HasSuffix(strings.ToLower(packagePath), "/tests") {
		return UnitRoleTest
	}
	if strings.Contains(lower, "/contrib") || strings.Contains(lower, "/examples") ||
		strings.Contains(lower, "/example") || strings.Contains(lower, "/tools") ||
		strings.Contains(lower, "/cmd/") && strings.Contains(lower, "/benchmark") {
		return UnitRoleTooling
	}
	if strings.Contains(lower, "/docs") || strings.Contains(lower, "/documentation") {
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

// processEntryPackage reports whether a package hosts an exact process-entry
// candidate (MemberEntrypoint) or anchors a process_entry anchor. Such
// packages are never merged into module clusters (Decision 216.4).
func processEntryPackage(bundle CandidateBundle, packageValue string) bool {
	for _, candidate := range bundle.Candidates {
		if candidate.ID.Kind == MemberEntrypoint &&
			candidate.ParentID != nil && candidate.ParentID.Kind == MemberPackage &&
			candidate.ParentID.Value == packageValue {
			return true
		}
	}
	return false
}

// topLevelModule returns the top-level module segment of an exact package
// path when it is safe to cluster by, else "" (never cluster by a canonical
// member ID or a root-level package).
func topLevelModule(displayName, packageValue string) string {
	name := strings.TrimSuffix(strings.TrimSpace(displayName), "/")
	if name == "" || strings.HasPrefix(name, "member-") || !strings.Contains(name, "/") {
		return ""
	}
	segments := strings.Split(name, "/")
	if len(segments) < 2 {
		return ""
	}
	module := segments[0]
	if module == "" || module == "." || module == ".." || module == "cmd" {
		return ""
	}
	return module
}

// packageModuleFromFacts derives the top-level module from the exact
// package path carried by declaration facts (e.g. go.etcd.io/etcd/server/v3
// -> server), when the display name alone has no path structure.
func packageModuleFromFacts(candidate Candidate) string {
	for _, fact := range candidate.Facts {
		if fact.Kind != FactDeclaration {
			continue
		}
		value := strings.TrimSpace(fact.Value)
		if value == "" || strings.HasPrefix(value, "member-") {
			continue
		}
		// Strip the module root (scheme/domain prefix) to reach the
		// repository-relative module segment.
		trimmed := value
		if index := strings.Index(trimmed, "/"); index >= 0 {
			trimmed = trimmed[index+1:]
		}
		segments := strings.Split(trimmed, "/")
		if len(segments) < 1 || segments[0] == "" {
			continue
		}
		module := segments[0]
		if module == "cmd" && len(segments) > 1 {
			module = segments[1]
		}
		return module
	}
	return ""
}

// stableModuleID derives a stable identity for a module-cluster unit.
func stableModuleID(module string) string {
	hash := sha256.New()
	fmt.Fprintf(hash, "module-v%d/%s\n", UnitCatalogVersion, module)
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
	containsCanonical := false
	kept := make([]string, 0, len(fields))
	for _, field := range fields {
		if _, canonical := canonicalOpaqueIDs[strings.TrimSpace(field)]; canonical {
			containsCanonical = true
			continue
		}
		kept = append(kept, field)
	}
	if !containsCanonical {
		return trimmed
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

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
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
