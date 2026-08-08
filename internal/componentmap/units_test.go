package componentmap

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/evidence"
)

func TestCompileUnitCatalogDeterministicUnderShuffledInput(t *testing.T) {
	t.Parallel()

	bundle := unitFixtureBundle()
	first, err := CompileUnitCatalog(bundle)
	if err != nil {
		t.Fatal(err)
	}
	shuffled := bundle
	// Reverse candidate order deterministically.
	reversed := make([]Candidate, len(bundle.Candidates))
	for index, candidate := range bundle.Candidates {
		reversed[len(bundle.Candidates)-1-index] = candidate
	}
	shuffled.Candidates = reversed
	second, err := CompileUnitCatalog(shuffled)
	if err != nil {
		t.Fatal(err)
	}
	if first.SHA256 != second.SHA256 {
		t.Fatalf("unit catalog digest changed under input reordering: %s vs %s", first.SHA256, second.SHA256)
	}
	if len(first.Units) != len(second.Units) {
		t.Fatalf("unit count changed under input reordering: %d vs %d", len(first.Units), len(second.Units))
	}
}

func TestCompileUnitCatalogCompleteMemberCoverage(t *testing.T) {
	t.Parallel()

	bundle := unitFixtureBundle()
	catalog, err := CompileUnitCatalog(bundle)
	if err != nil {
		t.Fatal(err)
	}
	total := 0
	for _, unit := range catalog.Units {
		total += len(unit.MemberIDs)
	}
	if total != len(bundle.Candidates) {
		t.Fatalf("member coverage = %d, want %d (every exact raw member in exactly one primary unit)", total, len(bundle.Candidates))
	}
	if catalog.CoveredMembers != len(bundle.Candidates) {
		t.Fatalf("covered = %d, want %d", catalog.CoveredMembers, len(bundle.Candidates))
	}
}

func TestCompileUnitCatalogRoleSeparation(t *testing.T) {
	t.Parallel()

	bundle := unitFixtureBundle()
	catalog, err := CompileUnitCatalog(bundle)
	if err != nil {
		t.Fatal(err)
	}
	var testUnits, productionUnits int
	for _, unit := range catalog.Units {
		switch unit.Role {
		case UnitRoleTest:
			testUnits++
		case UnitRoleProduction:
			productionUnits++
		}
	}
	if testUnits == 0 {
		t.Fatalf("no test-role units: %#v", catalog.Units)
	}
	if productionUnits == 0 {
		t.Fatalf("no production-role units: %#v", catalog.Units)
	}
}

func TestCompileUnitCatalogWireRefsAreBoundedAndCanonicalFree(t *testing.T) {
	t.Parallel()

	bundle := unitFixtureBundle()
	catalog, err := CompileUnitCatalog(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.WireUnits) != len(catalog.Units) {
		t.Fatalf("wire unit count = %d, want %d", len(catalog.WireUnits), len(catalog.Units))
	}
	// Canonical IDs must never leak into wire labels or representative labels.
	for _, wireUnit := range catalog.WireUnits {
		for _, candidate := range bundle.Candidates {
			if containsSubstring(wireUnit.Label, candidate.ID.Value) {
				t.Fatalf("wire unit label leaked canonical ID %q: %q", candidate.ID.Value, wireUnit.Label)
			}
			for _, label := range wireUnit.RepresentativeLabels {
				if containsSubstring(label, candidate.ID.Value) {
					t.Fatalf("wire representative label leaked canonical ID %q: %q", candidate.ID.Value, label)
				}
			}
		}
	}
}

func TestUnitCatalogUnitMembersByWireRefExpansion(t *testing.T) {
	t.Parallel()

	bundle := unitFixtureBundle()
	catalog, err := CompileUnitCatalog(bundle)
	if err != nil {
		t.Fatal(err)
	}
	membersByRef := unitCatalogUnitMembersByWireRef(catalog)
	if len(membersByRef) != len(catalog.Units) {
		t.Fatalf("expansion map count = %d, want %d", len(membersByRef), len(catalog.Units))
	}
	seen := map[string]bool{}
	total := 0
	for ref, members := range membersByRef {
		if !containsSubstring(ref, "u") {
			t.Fatalf("unit wire ref %q is not a u* ref", ref)
		}
		for _, memberID := range members {
			if seen[memberID.key()] {
				t.Fatalf("member %s expanded into more than one unit", memberID.key())
			}
			seen[memberID.key()] = true
			total++
		}
	}
	if total != len(bundle.Candidates) {
		t.Fatalf("expansion total = %d, want %d", total, len(bundle.Candidates))
	}
}

func TestNearestConceptualPackageOwnerResolvesBoundedStructuralChains(t *testing.T) {
	t.Parallel()

	packageCandidate := unitTestCandidate(MemberPackage, "member-package-runner", "runner", CandidateRoleConceptualMember, nil)
	fileCandidate := unitTestCandidate(MemberFile, "member-file-options", "runner/options.go", CandidateRoleStructuralLocator, &packageCandidate.ID)
	direct := unitTestCandidate(MemberSymbol, "member-symbol-direct", "runner.Direct", CandidateRoleConceptualMember, &packageCandidate.ID)
	mediated := unitTestCandidate(MemberSymbol, "member-symbol-mediated", "runner.Mediated", CandidateRoleConceptualMember, &fileCandidate.ID)
	known := map[MemberID]Candidate{
		packageCandidate.ID: packageCandidate,
		fileCandidate.ID:    fileCandidate,
		direct.ID:           direct,
		mediated.ID:         mediated,
	}
	for _, memberID := range []MemberID{packageCandidate.ID, direct.ID, mediated.ID} {
		owner, ok := nearestConceptualPackageOwner(memberID, known)
		if !ok || owner != packageCandidate.ID {
			t.Fatalf("owner(%s) = %#v, %t; want %s", memberID.key(), owner, ok, packageCandidate.ID.key())
		}
	}

	unknownParentID := MemberID{Kind: MemberFile, Value: "member-file-unknown"}
	unresolved := unitTestCandidate(MemberSymbol, "member-symbol-unresolved", "Unresolved", CandidateRoleConceptualMember, &unknownParentID)
	known[unresolved.ID] = unresolved
	if owner, ok := nearestConceptualPackageOwner(unresolved.ID, known); ok {
		t.Fatalf("unknown parent resolved to %#v", owner)
	}

	cycleAID := MemberID{Kind: MemberFile, Value: "member-file-cycle-a"}
	cycleBID := MemberID{Kind: MemberFile, Value: "member-file-cycle-b"}
	cyclicMember := unitTestCandidate(MemberSymbol, "member-symbol-cycle", "Cyclic", CandidateRoleConceptualMember, &cycleAID)
	cycleA := unitTestCandidate(MemberFile, cycleAID.Value, "cycle/a.go", CandidateRoleStructuralLocator, &cycleBID)
	cycleB := unitTestCandidate(MemberFile, cycleBID.Value, "cycle/b.go", CandidateRoleStructuralLocator, &cycleAID)
	cycleKnown := map[MemberID]Candidate{cyclicMember.ID: cyclicMember, cycleA.ID: cycleA, cycleB.ID: cycleB}
	if owner, ok := nearestConceptualPackageOwner(cyclicMember.ID, cycleKnown); ok {
		t.Fatalf("cyclic parent graph resolved to %#v", owner)
	}
}

func TestCompileUnitCatalogFileMediatedMembersShareFinalPackageUnit(t *testing.T) {
	t.Parallel()

	pkg := unitTestCandidate(MemberPackage, "member-package-runner", "runner", CandidateRoleConceptualMember, nil)
	file := unitTestCandidate(MemberFile, "member-file-options", "runner/options.go", CandidateRoleStructuralLocator, &pkg.ID)
	direct := unitTestCandidate(MemberSymbol, "member-symbol-direct", "runner.Direct", CandidateRoleConceptualMember, &pkg.ID)
	mediated := unitTestCandidate(MemberSymbol, "member-symbol-mediated", "runner.Mediated", CandidateRoleConceptualMember, &file.ID)
	entrypoint := unitTestCandidate(MemberEntrypoint, "member-entrypoint-main", "cmd/ghz.main", CandidateRoleConceptualMember, &file.ID)
	bundle := unitTestBundle(
		[]Candidate{pkg, file, direct, mediated, entrypoint},
		[]BehaviorAnchor{unitTestAnchor("anchor-process", AnchorProcessEntry, entrypoint.ID)},
	)
	catalog, err := CompileUnitCatalog(bundle)
	if err != nil {
		t.Fatal(err)
	}
	packageRef := catalog.MemberToWireUnit[pkg.ID]
	for _, memberID := range []MemberID{direct.ID, mediated.ID, entrypoint.ID} {
		if got := catalog.MemberToWireUnit[memberID]; got != packageRef {
			t.Fatalf("member %s unit = %q, want package unit %q", memberID.key(), got, packageRef)
		}
	}
	if _, leaked := catalog.MemberToWireUnit[file.ID]; leaked {
		t.Fatalf("structural locator received conceptual unit ownership: %s", file.ID.key())
	}
	if catalog.TotalMembers != 4 || catalog.CoveredMembers != 4 || len(catalog.MemberToWireUnit) != 4 {
		t.Fatalf("conceptual coverage = total %d covered %d map %d, want 4/4/4", catalog.TotalMembers, catalog.CoveredMembers, len(catalog.MemberToWireUnit))
	}
	for _, unit := range catalog.Units {
		if strings.HasPrefix(unit.CanonicalID, "unit-local-remainder") {
			t.Fatalf("file-mediated conceptual members fell into remainder: %#v", unit)
		}
	}
	encoded, err := json.Marshal(catalog)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "member_to_wire_unit") || strings.Contains(string(encoded), "MemberToWireUnit") {
		t.Fatalf("private ownership map was serialized: %s", encoded)
	}
}

func TestCompileUnitCatalogMapsEveryConceptualKindIncludingFlow(t *testing.T) {
	t.Parallel()

	bundle := landscapeTestBundle()
	catalog, err := CompileUnitCatalog(bundle)
	if err != nil {
		t.Fatal(err)
	}
	conceptualCount := 0
	for _, candidate := range bundle.Candidates {
		if candidate.Role != CandidateRoleConceptualMember {
			continue
		}
		conceptualCount++
		if ref := catalog.MemberToWireUnit[candidate.ID]; ref == "" {
			t.Fatalf("conceptual %s member has no final unit", candidate.ID.Kind)
		}
	}
	if len(catalog.MemberToWireUnit) != conceptualCount {
		t.Fatalf("final ownership map = %d, want %d conceptual members", len(catalog.MemberToWireUnit), conceptualCount)
	}
}

func TestCompileUnitCatalogAttachesAnchorsToExactFinalUnits(t *testing.T) {
	t.Parallel()

	t.Run("split", func(t *testing.T) {
		pkg := unitTestCandidate(MemberPackage, "member-package-large", "runtime", CandidateRoleConceptualMember, nil)
		candidates := []Candidate{pkg}
		var target MemberID
		for index := 0; index < maxUnitMembers+2; index++ {
			candidate := unitTestCandidate(
				MemberSymbol,
				fmt.Sprintf("member-symbol-%03d", index),
				fmt.Sprintf("runtime.Symbol%03d", index),
				CandidateRoleConceptualMember,
				&pkg.ID,
			)
			candidates = append(candidates, candidate)
			if index == maxUnitMembers+1 {
				target = candidate.ID
			}
		}
		bundle := unitTestBundle(candidates, []BehaviorAnchor{
			unitTestAnchor("anchor-split", AnchorExtensionFamily, target),
		})
		catalog, err := CompileUnitCatalog(bundle)
		if err != nil {
			t.Fatal(err)
		}
		targetRef := catalog.MemberToWireUnit[target]
		anchoredUnits := 0
		for index, unit := range catalog.Units {
			anchored := containsString(unit.AnchorIDs, "anchor-split")
			containsTarget := catalog.WireUnits[index].Ref == targetRef
			if anchored != containsTarget {
				t.Fatalf("unit %q anchor=%t, contains target=%t", unit.CanonicalID, anchored, containsTarget)
			}
			if anchored {
				anchoredUnits++
				if catalog.WireUnits[index].AnchorRefCount != 1 {
					t.Fatalf("anchored wire count = %d, want 1", catalog.WireUnits[index].AnchorRefCount)
				}
			}
		}
		if anchoredUnits != 1 {
			t.Fatalf("anchor attached to %d split units, want exactly 1", anchoredUnits)
		}
	})

	t.Run("remainder", func(t *testing.T) {
		orphan := unitTestCandidate(MemberSymbol, "member-symbol-orphan", "Orphan", CandidateRoleConceptualMember, nil)
		bundle := unitTestBundle([]Candidate{orphan}, []BehaviorAnchor{
			unitTestAnchor("anchor-remainder", AnchorExtensionFamily, orphan.ID),
		})
		catalog, err := CompileUnitCatalog(bundle)
		if err != nil {
			t.Fatal(err)
		}
		if len(catalog.Units) != 1 || !strings.HasPrefix(catalog.Units[0].CanonicalID, "unit-local-remainder") {
			t.Fatalf("unexpected remainder units: %#v", catalog.Units)
		}
		if !containsString(catalog.Units[0].AnchorIDs, "anchor-remainder") || catalog.WireUnits[0].AnchorRefCount != 1 {
			t.Fatalf("remainder anchor missing: unit=%#v wire=%#v", catalog.Units[0], catalog.WireUnits[0])
		}
	})
}

func TestCompileUnitCatalogRepresentativeLabelsUseSafeCandidateNames(t *testing.T) {
	t.Parallel()

	pkg := unitTestCandidate(MemberPackage, "member-package-router-private", "github.com/bojand/ghz/web/router", CandidateRoleConceptualMember, nil)
	validator := unitTestCandidate(MemberSymbol, "member-symbol-validator-private", "github.com/bojand/ghz/web/router.Validator", CandidateRoleConceptualMember, &pkg.ID)
	unsafe := unitTestCandidate(MemberSymbol, "member-symbol-secret-private", "web/router.member-symbol-secret-private", CandidateRoleConceptualMember, &pkg.ID)
	long := unitTestCandidate(MemberSymbol, "member-symbol-unicode-private", "web/router."+strings.Repeat("界", 40), CandidateRoleConceptualMember, &pkg.ID)
	rootFile := unitTestCandidate(MemberFile, "member-file-root-private", "main.go", CandidateRoleConceptualMember, &pkg.ID)
	bundle := unitTestBundle(
		[]Candidate{pkg, validator, unsafe, long, rootFile},
		[]BehaviorAnchor{unitTestAnchor("anchor-router-private", AnchorExtensionFamily, pkg.ID)},
	)
	for index := range bundle.Candidates {
		if bundle.Candidates[index].ID == pkg.ID {
			bundle.Candidates[index].Facts = []LocalFact{unitTestFact(FactDeclaration, "github.com/bojand/ghz/web/router")}
		}
	}
	catalog, err := CompileUnitCatalog(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.WireUnits) != 1 || len(catalog.WireUnits[0].RepresentativeLabels) == 0 {
		t.Fatalf("representative labels are empty: %#v", catalog.WireUnits)
	}
	foundValidator := false
	for _, label := range catalog.WireUnits[0].RepresentativeLabels {
		if label == "Validator" {
			foundValidator = true
		}
		if len(label) > maxUnitWireLabelBytes || !utf8.ValidString(label) {
			t.Fatalf("representative label is not a valid %d-byte value: %q (%d bytes)", maxUnitWireLabelBytes, label, len(label))
		}
		if strings.Contains(label, "/") || strings.Contains(label, "github.com") || strings.Contains(label, "bojand") {
			t.Fatalf("representative label leaked a path or qualified identity: %q", label)
		}
		if label == "main.go" {
			t.Fatalf("representative label leaked a root repository path: %q", label)
		}
		for _, candidate := range bundle.Candidates {
			if strings.Contains(label, candidate.ID.Value) {
				t.Fatalf("representative label leaked canonical token %q: %q", candidate.ID.Value, label)
			}
		}
	}
	if !foundValidator {
		t.Fatalf("candidate display name did not produce semantic representative: %#v", catalog.WireUnits[0].RepresentativeLabels)
	}
}

func TestCompileUnitCatalogUsesRepositoryRelativeModuleLabels(t *testing.T) {
	t.Parallel()

	definitions := []struct {
		id, name, declaration string
	}{
		{"member-package-runner", "runner", "github.com/bojand/ghz/runner"},
		{"member-package-web-api", "web/api", "github.com/bojand/ghz/web/api"},
		{"member-package-web-router", "web/router", "github.com/bojand/ghz/web/router"},
		{"member-package-command", "cmd/ghz-web", "github.com/bojand/ghz/cmd/ghz-web"},
		{"member-package-printer", "github.com/bojand/ghz/printer", "github.com/bojand/ghz/printer"},
	}
	candidates := make([]Candidate, 0, len(definitions))
	for _, definition := range definitions {
		candidate := unitTestCandidate(MemberPackage, definition.id, definition.name, CandidateRoleConceptualMember, nil)
		candidate.Facts = []LocalFact{unitTestFact(FactDeclaration, definition.declaration)}
		candidates = append(candidates, candidate)
	}
	bundle := unitTestBundle(candidates, []BehaviorAnchor{
		unitTestAnchor("anchor-runner", AnchorExtensionFamily, candidates[0].ID),
	})
	catalog, err := CompileUnitCatalog(bundle)
	if err != nil {
		t.Fatal(err)
	}
	labels := map[string]bool{}
	for _, wire := range catalog.WireUnits {
		labels[wire.Label] = true
		if strings.Contains(wire.Label, "/") || strings.Contains(wire.Label, "github.com") || strings.Contains(wire.Label, "bojand") {
			t.Fatalf("unit label is not repository-relative semantic context: %q", wire.Label)
		}
	}
	for _, want := range []string{"runner", "web", "ghz-web", "printer"} {
		if !labels[want] {
			t.Fatalf("missing semantic module label %q in %#v", want, catalog.WireUnits)
		}
	}
}

func TestCompileUnitCatalogDoesNotMergeCommandWithSameNamedTopLevelPackage(t *testing.T) {
	t.Parallel()

	definitions := []struct {
		id, name string
	}{
		{"member-package-command-cli", "cmd/dive/cli"},
		{"member-package-command-runtime", "cmd/dive/runtime"},
		{"member-package-library-filetree", "dive/filetree"},
		{"member-package-library-image", "dive/image"},
	}
	candidates := make([]Candidate, 0, len(definitions))
	for _, definition := range definitions {
		candidate := unitTestCandidate(
			MemberPackage,
			definition.id,
			definition.name,
			CandidateRoleConceptualMember,
			nil,
		)
		candidate.Facts = []LocalFact{
			unitTestFact(FactDeclaration, "github.com/wagoodman/dive/"+definition.name),
		}
		candidates = append(candidates, candidate)
	}
	entry := unitTestCandidate(
		MemberPackage,
		"member-package-entry",
		"entry",
		CandidateRoleConceptualMember,
		nil,
	)
	entry.Facts = []LocalFact{
		unitTestFact(FactDeclaration, "github.com/wagoodman/dive/entry"),
	}
	candidates = append(candidates, entry)
	catalog, err := CompileUnitCatalog(unitTestBundle(
		candidates,
		[]BehaviorAnchor{unitTestAnchor("anchor-entry", AnchorExtensionFamily, entry.ID)},
	))
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.WireUnits) > targetMaxUnits {
		t.Fatalf("collision-safe grouping exceeded the hard unit bound: %d > %d", len(catalog.WireUnits), targetMaxUnits)
	}
	commandRef := catalog.MemberToWireUnit[candidates[0].ID]
	libraryRef := catalog.MemberToWireUnit[candidates[2].ID]
	if commandRef == "" || libraryRef == "" || commandRef == libraryRef {
		t.Fatalf("command/library ownership refs = %q/%q, want two exact final units", commandRef, libraryRef)
	}
	if catalog.MemberToWireUnit[candidates[1].ID] != commandRef {
		t.Fatalf("second command package did not share command unit %q", commandRef)
	}
	if catalog.MemberToWireUnit[candidates[3].ID] != libraryRef {
		t.Fatalf("second library package did not share library unit %q", libraryRef)
	}
	commandUnit, commandOK := unitWireByRef(catalog, commandRef)
	libraryUnit, libraryOK := unitWireByRef(catalog, libraryRef)
	if !commandOK || !libraryOK || commandUnit.Label != "dive" || libraryUnit.Label != "dive" {
		t.Fatalf("bounded wire labels changed: command=%#v library=%#v", commandUnit, libraryUnit)
	}
	for _, unit := range []SynthesisUnit{commandUnit, libraryUnit} {
		if strings.Contains(unit.Label, "/") || strings.Contains(unit.Label, "cmd") {
			t.Fatalf("private grouping identity leaked into wire label: %#v", unit)
		}
	}
}

func TestCompileUnitCatalogRefinesBroadModuleByNextExactSegment(t *testing.T) {
	t.Parallel()

	entry := unitTestCandidate(MemberPackage, "member-package-entry", "entry", CandidateRoleConceptualMember, nil)
	entry.Facts = []LocalFact{unitTestFact(FactDeclaration, "github.com/foxcpp/maddy/entry")}
	candidates := []Candidate{entry}
	familyRefs := make(map[string]UnitWireRef)
	for index := 0; index < 38; index++ {
		family := fmt.Sprintf("family-%02d", index%8)
		name := fmt.Sprintf("internal/%s/package-%02d", family, index)
		candidate := unitTestCandidate(
			MemberPackage,
			fmt.Sprintf("member-package-internal-%02d", index),
			name,
			CandidateRoleConceptualMember,
			nil,
		)
		candidate.Facts = []LocalFact{unitTestFact(FactDeclaration, "github.com/foxcpp/maddy/"+name)}
		candidates = append(candidates, candidate)
	}
	catalog, err := CompileUnitCatalog(unitTestBundle(
		candidates,
		[]BehaviorAnchor{unitTestAnchor("anchor-entry", AnchorExtensionFamily, entry.ID)},
	))
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.WireUnits) != 9 || len(catalog.WireUnits) > targetMaxUnits {
		t.Fatalf("refined Maddy-like units = %d, want entry + 8 bounded families", len(catalog.WireUnits))
	}
	for index := 0; index < 38; index++ {
		memberID := candidates[index+1].ID
		family := fmt.Sprintf("family-%02d", index%8)
		ref := catalog.MemberToWireUnit[memberID]
		if ref == "" {
			t.Fatalf("internal member %s lost exact ownership", memberID.key())
		}
		if previous := familyRefs[family]; previous != "" && previous != ref {
			t.Fatalf("family %q split across refs %q and %q", family, previous, ref)
		}
		familyRefs[family] = ref
		wire, ok := unitWireByRef(catalog, ref)
		if !ok || wire.Label != family || strings.Contains(wire.Label, "/") || strings.Contains(wire.Label, "internal") {
			t.Fatalf("family wire projection = %#v, found=%t", wire, ok)
		}
	}
	if len(familyRefs) != 8 || catalog.CoveredMembers != 39 || len(catalog.MemberToWireUnit) != 39 {
		t.Fatalf("refinement coverage/families = %d/%d/%d", catalog.CoveredMembers, len(catalog.MemberToWireUnit), len(familyRefs))
	}
}

func TestCompileUnitCatalogKeepsBroadModuleWhenRefinementExceedsUnitBound(t *testing.T) {
	t.Parallel()

	candidates := make([]Candidate, 0, 68)
	seedIDs := make([]MemberID, 0, 30)
	for index := 0; index < 30; index++ {
		name := fmt.Sprintf("service-%02d", index)
		candidate := unitTestCandidate(
			MemberPackage,
			fmt.Sprintf("member-package-seed-%02d", index),
			name,
			CandidateRoleConceptualMember,
			nil,
		)
		candidate.Facts = []LocalFact{unitTestFact(FactDeclaration, "github.com/example/large/"+name)}
		candidates = append(candidates, candidate)
		seedIDs = append(seedIDs, candidate.ID)
	}
	internalStart := len(candidates)
	for index := 0; index < 38; index++ {
		name := fmt.Sprintf("internal/family-%02d/package", index)
		candidate := unitTestCandidate(
			MemberPackage,
			fmt.Sprintf("member-package-internal-%02d", index),
			name,
			CandidateRoleConceptualMember,
			nil,
		)
		candidate.Facts = []LocalFact{unitTestFact(FactDeclaration, "github.com/example/large/"+name)}
		candidates = append(candidates, candidate)
	}
	catalog, err := CompileUnitCatalog(unitTestBundle(
		candidates,
		[]BehaviorAnchor{unitTestAnchor("anchor-seeds", AnchorExtensionFamily, seedIDs...)},
	))
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.WireUnits) != 31 || len(catalog.WireUnits) > targetMaxUnits {
		t.Fatalf("fallback units = %d, want 30 seeds + one original broad bucket", len(catalog.WireUnits))
	}
	broadRef := catalog.MemberToWireUnit[candidates[internalStart].ID]
	for _, candidate := range candidates[internalStart:] {
		if got := catalog.MemberToWireUnit[candidate.ID]; got != broadRef {
			t.Fatalf("over-budget refinement split original bucket: %s = %q, want %q", candidate.ID.key(), got, broadRef)
		}
	}
	wire, ok := unitWireByRef(catalog, broadRef)
	if !ok || wire.Label != "internal" || len(catalog.MemberToWireUnit) != len(candidates) {
		t.Fatalf("fallback broad unit/coverage = %#v, found=%t map=%d", wire, ok, len(catalog.MemberToWireUnit))
	}
}

func TestCompileUnitCatalogBudgetsBroadRefinementAfterAttachmentAndSplitting(t *testing.T) {
	t.Parallel()

	// Sixty-two exact seed units plus the unrefined broad unit's two chunks
	// reach the hard bound exactly. Splitting the broad package bucket into a
	// 49-member family and a one-member family would require three chunks and
	// produce 65 final wire units, even though the package-only projection
	// appears to fit as 62 + 2 units.
	candidates := make([]Candidate, 0, 112)
	seedIDs := make([]MemberID, 0, 62)
	for index := 0; index < 62; index++ {
		name := fmt.Sprintf("service-%02d", index)
		candidate := unitTestCandidate(
			MemberPackage,
			fmt.Sprintf("member-package-seed-%02d", index),
			name,
			CandidateRoleConceptualMember,
			nil,
		)
		candidate.Facts = []LocalFact{unitTestFact(FactDeclaration, "github.com/example/adversarial/"+name)}
		candidates = append(candidates, candidate)
		seedIDs = append(seedIDs, candidate.ID)
	}
	var symbolParent MemberID
	for index := 0; index < 25; index++ {
		family := "large"
		if index == 24 {
			family = "small"
		}
		name := fmt.Sprintf("internal/%s/package-%02d", family, index)
		candidate := unitTestCandidate(
			MemberPackage,
			fmt.Sprintf("member-package-internal-%02d", index),
			name,
			CandidateRoleConceptualMember,
			nil,
		)
		candidate.Facts = []LocalFact{unitTestFact(FactDeclaration, "github.com/example/adversarial/"+name)}
		candidates = append(candidates, candidate)
		if index == 0 {
			symbolParent = candidate.ID
		}
	}
	for index := 0; index < 25; index++ {
		candidates = append(candidates, unitTestCandidate(
			MemberSymbol,
			fmt.Sprintf("member-symbol-internal-%02d", index),
			fmt.Sprintf("internal.large.Symbol%02d", index),
			CandidateRoleConceptualMember,
			&symbolParent,
		))
	}

	catalog, err := CompileUnitCatalog(unitTestBundle(
		candidates,
		[]BehaviorAnchor{unitTestAnchor("anchor-seeds", AnchorExtensionFamily, seedIDs...)},
	))
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.WireUnits) != targetMaxUnits {
		t.Fatalf("post-attachment wire units = %d, want exact hard bound %d", len(catalog.WireUnits), targetMaxUnits)
	}
	internalUnits := 0
	for _, wire := range catalog.WireUnits {
		if wire.Label == "large" || wire.Label == "small" {
			t.Fatalf("over-budget refinement leaked child unit: %#v", wire)
		}
		if wire.Label == "internal" {
			internalUnits++
		}
	}
	if internalUnits != 2 {
		t.Fatalf("unrefined broad bucket chunks = %d, want 2", internalUnits)
	}
	if catalog.CoveredMembers != len(candidates) || len(catalog.MemberToWireUnit) != len(candidates) {
		t.Fatalf("exact coverage = %d/%d, ownership=%d", catalog.CoveredMembers, len(candidates), len(catalog.MemberToWireUnit))
	}
}

func TestCompileUnitCatalogKeepsClosedRolesSeparate(t *testing.T) {
	t.Parallel()

	api := unitTestCandidate(MemberPackage, "member-package-api", "server/api", CandidateRoleConceptualMember, nil)
	tests := unitTestCandidate(MemberPackage, "member-package-tests", "server/tests/e2e", CandidateRoleConceptualMember, nil)
	tools := unitTestCandidate(MemberPackage, "member-package-tools", "tools/generate", CandidateRoleConceptualMember, nil)
	docs := unitTestCandidate(MemberPackage, "member-package-docs", "docs/guide", CandidateRoleConceptualMember, nil)
	testament := unitTestCandidate(MemberPackage, "member-package-testament", "internal/testament", CandidateRoleConceptualMember, nil)
	evidenced := unitTestCandidate(MemberPackage, "member-package-evidenced", "services/admin", CandidateRoleConceptualMember, nil)
	evidenced.Facts = append(evidenced.Facts, unitTestFact(FactExecutableRole, "test_or_helper"))
	candidates := []Candidate{api, tests, tools, docs, testament, evidenced}
	bundle := unitTestBundle(candidates, []BehaviorAnchor{
		unitTestAnchor("anchor-api", AnchorExtensionFamily, api.ID),
	})
	catalog, err := CompileUnitCatalog(bundle)
	if err != nil {
		t.Fatal(err)
	}
	wantRoles := map[MemberID]UnitRole{
		api.ID:       UnitRoleProduction,
		tests.ID:     UnitRoleTest,
		tools.ID:     UnitRoleTooling,
		docs.ID:      UnitRoleDocumentation,
		testament.ID: UnitRoleProduction,
		evidenced.ID: UnitRoleTest,
	}
	for memberID, want := range wantRoles {
		ref := catalog.MemberToWireUnit[memberID]
		wire, ok := unitWireByRef(catalog, ref)
		if !ok || wire.Role != want {
			t.Fatalf("member %s role = %#v, found=%t; want %q", memberID.key(), wire.Role, ok, want)
		}
	}
	if catalog.MemberToWireUnit[api.ID] == catalog.MemberToWireUnit[tests.ID] {
		t.Fatal("production and test packages from one module were merged")
	}
}

func TestCompileUnitCatalogGhzLikeFileMediatedProjection(t *testing.T) {
	t.Parallel()

	packageNames := []string{
		"web/router", "internal/helloworld", "web/model", "internal/wrapped", "printer", "internal/gtime",
		"runner", "web/router/statik", "internal/sleep", "internal", "web/api", "web/config", "ghz",
		"web/database", "cmd/ghz-web", "protodesc", "load", "cmd/ghz",
	}
	packages := make([]Candidate, 0, len(packageNames))
	packageByName := make(map[string]Candidate, len(packageNames))
	for index, name := range packageNames {
		candidate := unitTestCandidate(MemberPackage, fmt.Sprintf("member-package-%02d", index), name, CandidateRoleConceptualMember, nil)
		candidate.Facts = []LocalFact{unitTestFact(FactDeclaration, "github.com/bojand/ghz/"+name)}
		if name == "ghz" {
			candidate.Facts = []LocalFact{unitTestFact(FactDeclaration, "github.com/bojand/ghz")}
		}
		packages = append(packages, candidate)
		packageByName[name] = candidate
	}
	owners := []string{"runner", "runner", "cmd/ghz-web", "cmd/ghz", "web/router", "web/config", "web/api", "load", "printer", "ghz"}
	candidates := append([]Candidate(nil), packages...)
	type ownership struct{ member, owner MemberID }
	ownerships := make([]ownership, 0, len(owners))
	var anchored MemberID
	for index, ownerName := range owners {
		owner := packageByName[ownerName]
		file := unitTestCandidate(MemberFile, fmt.Sprintf("member-file-%02d", index), ownerName+fmt.Sprintf("/file-%02d.go", index), CandidateRoleStructuralLocator, &owner.ID)
		symbol := unitTestCandidate(MemberSymbol, fmt.Sprintf("member-symbol-%02d", index), ownerName+fmt.Sprintf(".Symbol%02d", index), CandidateRoleConceptualMember, &file.ID)
		candidates = append(candidates, file, symbol)
		ownerships = append(ownerships, ownership{member: symbol.ID, owner: owner.ID})
		if index == 0 {
			anchored = symbol.ID
		}
	}
	bundle := unitTestBundle(candidates, []BehaviorAnchor{
		unitTestAnchor("anchor-ghz-symbol", AnchorExtensionFamily, anchored),
	})
	catalog, err := CompileUnitCatalog(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if catalog.TotalMembers != 28 || catalog.CoveredMembers != 28 || len(catalog.MemberToWireUnit) != 28 {
		t.Fatalf("ghz conceptual coverage = %d/%d map=%d, want 28/28 map=28", catalog.CoveredMembers, catalog.TotalMembers, len(catalog.MemberToWireUnit))
	}
	for _, pair := range ownerships {
		if catalog.MemberToWireUnit[pair.member] != catalog.MemberToWireUnit[pair.owner] {
			t.Fatalf("mediated symbol %s is not in owner %s final unit", pair.member.key(), pair.owner.key())
		}
	}
	for _, candidate := range candidates {
		if candidate.Role == CandidateRoleStructuralLocator {
			if _, exists := catalog.MemberToWireUnit[candidate.ID]; exists {
				t.Fatalf("structural locator entered final unit map: %s", candidate.ID.key())
			}
		}
	}
	for index, unit := range catalog.Units {
		if strings.HasPrefix(unit.CanonicalID, "unit-local-remainder") {
			t.Fatalf("ghz-like mediated symbols produced a local remainder: %#v", unit)
		}
		if len(catalog.WireUnits[index].RepresentativeLabels) == 0 {
			t.Fatalf("ghz-like unit %q has no representative labels", unit.CanonicalID)
		}
	}
}

// unitFixtureBundle builds a small mixed-role bundle for unit compiler tests.
func unitFixtureBundle() CandidateBundle {
	declarationFact := func(value string) []LocalFact {
		return []LocalFact{{Kind: FactDeclaration, Value: value, Certainty: evidence.CertaintyStatic,
			Provenance: []evidence.Provenance{{Provider: "fixture", Version: "v1", Operation: "local_fact"}}}}
	}
	production := Candidate{
		ID:   MemberID{Kind: MemberPackage, Value: "member-package-prod-a"},
		Role: CandidateRoleConceptualMember, Name: "server/api",
		Facts: declarationFact("server/api"),
	}
	productionSymbol := Candidate{
		ID:   MemberID{Kind: MemberSymbol, Value: "member-symbol-prod-handler"},
		Role: CandidateRoleConceptualMember, Name: "server/api/handler",
		ParentID: &production.ID, Facts: declarationFact("server/api/handler"),
	}
	testPkg := Candidate{
		ID:   MemberID{Kind: MemberPackage, Value: "member-package-test-e2e"},
		Role: CandidateRoleConceptualMember, Name: "server/tests/e2e",
		Facts: declarationFact("server/tests/e2e"),
	}
	toolPkg := Candidate{
		ID:   MemberID{Kind: MemberPackage, Value: "member-package-tools-gen"},
		Role: CandidateRoleConceptualMember, Name: "tools/generate",
		Facts: declarationFact("tools/generate"),
	}
	docPkg := Candidate{
		ID:   MemberID{Kind: MemberPackage, Value: "member-package-docs-guide"},
		Role: CandidateRoleConceptualMember, Name: "docs/guide",
		Facts: declarationFact("docs/guide"),
	}
	bundle := CandidateBundle{
		Version:             ContractVersion,
		RepositoryArchetype: ArchetypeModularPlatformServer,
		GroundingMode:       GroundingMixed,
		Candidates:          []Candidate{production, productionSymbol, testPkg, toolPkg, docPkg},
		BehaviorAnchors: []BehaviorAnchor{{
			ID: "anchor-process", Kind: AnchorProcessEntry, ProofMode: AnchorProofProcessEntry,
			Label: "process entry", Certainty: evidence.CertaintyStatic,
			Location:    evidence.Location{Path: "server/main.go", Line: 1, Column: 1},
			Scenario:    ScenarioContext{ID: "scenario-unit-test", Name: "unit test scenario"},
			Producer:    evidence.Provenance{Provider: "fixture", Version: "v1", Operation: "anchor"},
			Limitations: []string{"static fixture anchor"},
			MemberIDs:   []MemberID{production.ID},
		}},
		Relations: []LocalRelation{},
	}
	return bundle
}

func unitTestFact(kind FactKind, value string) LocalFact {
	return LocalFact{
		Kind: kind, Value: value, Certainty: evidence.CertaintyStatic,
		Provenance: []evidence.Provenance{{Provider: "fixture", Version: "v1", Operation: "local_fact"}},
	}
}

func unitTestCandidate(
	kind MemberKind,
	id string,
	name string,
	role CandidateRole,
	parentID *MemberID,
) Candidate {
	return Candidate{
		ID: MemberID{Kind: kind, Value: id}, Role: role, Name: name, ParentID: parentID,
		Facts: []LocalFact{unitTestFact(FactDeclaration, name)},
	}
}

func unitTestAnchor(id string, kind BehaviorAnchorKind, memberIDs ...MemberID) BehaviorAnchor {
	proofMode := AnchorProofDeclarationFamily
	if kind == AnchorProcessEntry {
		proofMode = AnchorProofProcessEntry
	}
	return BehaviorAnchor{
		ID: id, Kind: kind, ProofMode: proofMode, Label: "unit projection anchor",
		Location:  evidence.Location{Path: "fixture.go", Line: 1, Column: 1},
		Scenario:  ScenarioContext{ID: "scenario-unit-projection", Name: "unit projection scenario"},
		Producer:  evidence.Provenance{Provider: "fixture", Version: "v1", Operation: "anchor"},
		Certainty: evidence.CertaintyStatic, MemberIDs: memberIDs,
		Limitations: []string{"static unit projection fixture"},
	}
}

func unitTestBundle(candidates []Candidate, anchors []BehaviorAnchor) CandidateBundle {
	return CandidateBundle{
		Version: ContractVersion, RepositoryArchetype: ArchetypeModularPlatformServer,
		GroundingMode: GroundingMixed, Candidates: candidates, BehaviorAnchors: anchors,
		Relations: []LocalRelation{},
	}
}

func unitWireByRef(catalog UnitCatalog, ref UnitWireRef) (SynthesisUnit, bool) {
	for _, wire := range catalog.WireUnits {
		if wire.Ref == ref {
			return wire, true
		}
	}
	return SynthesisUnit{}, false
}

func containsSubstring(value, substring string) bool {
	for index := 0; index+len(substring) <= len(value); index++ {
		if value[index:index+len(substring)] == substring {
			return true
		}
	}
	return false
}

// TestCompileUnitCatalogFillsRelationOutCountAggregate covers Decision 223:
// the per-unit outgoing package-import aggregate replaces the raw edges on
// the wire and is counted deterministically (source unit -> other unit).
func TestCompileUnitCatalogFillsRelationOutCountAggregate(t *testing.T) {
	t.Parallel()

	bundle := unitFixtureBundle()
	prodID := MemberID{Kind: MemberPackage, Value: "member-package-prod-a"}
	testID := MemberID{Kind: MemberPackage, Value: "member-package-test-e2e"}
	toolID := MemberID{Kind: MemberPackage, Value: "member-package-tools-gen"}
	rel := func(id string, from, to MemberID) LocalRelation {
		return LocalRelation{
			ID: id, From: from, To: to, Kind: StructuralRelationPackageImport,
			Certainty: evidence.CertaintyStatic,
			Provenance: []evidence.Provenance{{
				Provider: "fixture", Version: "v1", Operation: "relate_members",
			}},
			Scenarios: []ScenarioContext{{ID: "go:test", Name: "test build"}},
		}
	}
	bundle.Relations = []LocalRelation{
		rel("prod-imports-test", prodID, testID),
		rel("prod-imports-tool", prodID, toolID),
		rel("test-imports-prod", testID, prodID),
	}
	catalog, err := CompileUnitCatalog(bundle)
	if err != nil {
		t.Fatal(err)
	}
	byLabel := map[string]SynthesisUnit{}
	for _, wire := range catalog.WireUnits {
		byLabel[wire.Label] = wire
	}
	prod, ok := byLabel["api"]
	if !ok {
		t.Fatalf("production unit not found in wire: %#v", catalog.WireUnits)
	}
	// prod (api) imports test + tool (2 distinct targets); test imports api (1).
	if prod.RelationOutCount != 2 {
		t.Fatalf("production relation_out_count = %d, want 2", prod.RelationOutCount)
	}
	test, ok := byLabel["server"]
	if !ok {
		t.Fatalf("test unit not found in wire: %#v", catalog.WireUnits)
	}
	if test.RelationOutCount != 1 {
		t.Fatalf("test relation_out_count = %d, want 1", test.RelationOutCount)
	}
}

// TestBuildSynthesisRequestDropsPackageImportsKeepsHandoffs covers Decision
// 223 wire behavior: with a unit catalog present, package_import relations
// vanish while behavior_handoff relations remain; a bundle whose units
// compile to zero units (defensive legacy path) keeps raw relations.
func TestBuildSynthesisRequestDropsPackageImportsKeepsHandoffs(t *testing.T) {
	t.Parallel()

	bundle := landscapeTestBundle()
	request, encoded, err := BuildSynthesisRequest(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if len(request.Units) == 0 {
		t.Fatal("fixture bundle produced no units; test premise broken")
	}
	for _, relation := range request.Relations {
		if relation.Kind == StructuralRelationPackageImport {
			t.Fatalf("package_import relation survived unit-catalog request: %#v", relation)
		}
	}
	if strings.Contains(string(encoded), "package_import") {
		t.Fatalf("request JSON leaked package_import: %s", encoded)
	}

	// behavior_handoff relations must survive the filter.
	handoff := bundle
	handoff.Relations = append(append([]LocalRelation(nil), bundle.Relations...), LocalRelation{
		ID: "handoff-1", From: MemberID{Kind: MemberEntrypoint, Value: "backup-command"},
		To: MemberID{Kind: MemberFile, Value: "repo-file"}, Kind: StructuralRelationBehaviorHandoff,
		Certainty: evidence.CertaintyStatic,
		Provenance: []evidence.Provenance{{
			Provider: "fixture", Version: "v1", Operation: "relate_members",
		}},
		Scenarios: []ScenarioContext{{ID: "go:test", Name: "test build"}},
	})
	handoffRequest, handoffEncoded, err := BuildSynthesisRequest(handoff)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, relation := range handoffRequest.Relations {
		if relation.Kind == StructuralRelationBehaviorHandoff {
			found = true
		}
	}
	if !found {
		t.Fatalf("behavior_handoff relation was dropped: %s", handoffEncoded)
	}
	if strings.Contains(string(handoffEncoded), "package_import") {
		t.Fatalf("request JSON leaked package_import beside handoff: %s", handoffEncoded)
	}
}
