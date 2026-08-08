package report

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/componentmap"
)

const entryHandoffPackageCanonicalPath = "example.com/repo/service"

func entryHandoffPackageProjectionFixture(
	ownerIDs []componentmap.ComponentID,
	packageFile string,
) *ReportData {
	data := entrypointHandoffGroupFixture()
	canvas := data.ArchitectureCanvas
	// Remove the exact callee declaration member while retaining the exact
	// process-entry member. This is the real report shape that requires the
	// package-member fallback.
	canvas.Components[0].Members = append(
		[]componentmap.Candidate(nil),
		canvas.Components[0].Members[:1]...,
	)
	packageMemberID := componentmap.MemberID{
		Kind:  componentmap.MemberPackage,
		Value: "member-package-service",
	}
	packageMember := func() componentmap.Candidate {
		return componentmap.Candidate{
			ID:   packageMemberID,
			Role: componentmap.CandidateRoleConceptualMember,
			Name: "service",
			Facts: []componentmap.LocalFact{{
				Kind:  componentmap.FactDeclaration,
				Value: entryHandoffPackageCanonicalPath,
			}},
		}
	}
	for index, ownerID := range ownerIDs {
		component := ArchitectureComponent{ID: ownerID, Name: string(ownerID)}
		wrongKind := componentmap.Candidate{
			ID: componentmap.MemberID{Kind: componentmap.MemberFile, Value: "service.go"},
			Facts: []componentmap.LocalFact{{
				Kind: componentmap.FactDeclaration, Value: entryHandoffPackageCanonicalPath,
			}},
		}
		if index == 0 {
			component.Members = []componentmap.Candidate{packageMember(), wrongKind}
		} else {
			component.SharedMembers = []componentmap.Candidate{packageMember(), wrongKind}
		}
		canvas.Components = append(canvas.Components, component)
	}
	// These two adversaries must never become owners: the first has a
	// suggestive name but the wrong exact package fact; the second is the
	// explicitly excluded local remainder with the right fact.
	canvas.Components = append(canvas.Components,
		ArchitectureComponent{
			ID: "component-name-lookalike", Name: "service.Start service package",
			Members: []componentmap.Candidate{{
				ID: componentmap.MemberID{Kind: componentmap.MemberPackage, Value: "member-package-wrong"},
				Facts: []componentmap.LocalFact{{
					Kind: componentmap.FactDeclaration, Value: "example.com/repo/not-service",
				}},
			}},
		},
		ArchitectureComponent{
			ID: "local-remainder", Name: "Local remainder",
			Members: []componentmap.Candidate{packageMember()},
		},
	)
	canvas.LocalRemainderComponentID = "local-remainder"
	data.RepositoryGraph = &RepositoryGraph{
		Version: 2,
		Packages: []PackageInfo{
			{
				CanonicalPath: entryHandoffPackageCanonicalPath,
				Dir:           "service",
				Files:         []string{"service/extra.go", packageFile},
			},
			{
				CanonicalPath: "example.com/repo/not-service",
				Dir:           "other",
				Files:         []string{"other/service.go"},
			},
		},
	}
	return data
}

func projectEntrypointPackageFixture(t *testing.T, data *ReportData) EntrypointTransition {
	t.Helper()
	groups, err := ProjectEntrypointHandoffGroups(
		data.ArchitectureCanvas,
		data.ArchitectureGrounding,
		data.RepositoryGraph,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || len(groups[0].EntryHandoffs) != 1 {
		t.Fatalf("entry handoff groups = %#v, want one group with one handoff", groups)
	}
	return groups[0].EntryHandoffs[0]
}

func TestEntrypointHandoffPackageMemberFallbackUniquePluralAndZero(t *testing.T) {
	t.Run("unique", func(t *testing.T) {
		data := entryHandoffPackageProjectionFixture(
			[]componentmap.ComponentID{"component-package-a"},
			"service.go",
		)
		transition := projectEntrypointPackageFixture(t, data)
		if !reflect.DeepEqual(
			transition.ComponentIDs,
			[]componentmap.ComponentID{"component-package-a"},
		) {
			t.Fatalf("unique package owner = %#v", transition.ComponentIDs)
		}
		if transition.Target == nil || transition.Target.Symbol != "service.Start" {
			t.Fatalf("exact package-joined target = %#v", transition.Target)
		}
	})

	t.Run("plural", func(t *testing.T) {
		data := entryHandoffPackageProjectionFixture(
			[]componentmap.ComponentID{"component-package-b", "component-package-a"},
			"service.go",
		)
		transition := projectEntrypointPackageFixture(t, data)
		if !reflect.DeepEqual(
			transition.ComponentIDs,
			[]componentmap.ComponentID{"component-package-a", "component-package-b"},
		) {
			t.Fatalf("plural package owners = %#v", transition.ComponentIDs)
		}
	})

	t.Run("zero_without_inventory", func(t *testing.T) {
		data := entryHandoffPackageProjectionFixture(
			[]componentmap.ComponentID{"component-package-a"},
			"service.go",
		)
		data.RepositoryGraph = nil
		transition := projectEntrypointPackageFixture(t, data)
		if len(transition.ComponentIDs) != 0 || transition.Target == nil || transition.Target.Symbol != "" {
			t.Fatalf("missing inventory guessed a package owner: %#v", transition)
		}
	})

	t.Run("remainder_only", func(t *testing.T) {
		data := entryHandoffPackageProjectionFixture(nil, "service.go")
		transition := projectEntrypointPackageFixture(t, data)
		if len(transition.ComponentIDs) != 0 {
			t.Fatalf("local remainder became a cube: %#v", transition.ComponentIDs)
		}
	})

	t.Run("wrong_exact_path", func(t *testing.T) {
		data := entryHandoffPackageProjectionFixture(
			[]componentmap.ComponentID{"component-package-a"},
			"other/service.go",
		)
		transition := projectEntrypointPackageFixture(t, data)
		if len(transition.ComponentIDs) != 0 {
			t.Fatalf("directory/name similarity became a package join: %#v", transition.ComponentIDs)
		}
	})
}

func TestEntrypointHandoffExactDeclarationOwnerPrecedesPackageFallback(t *testing.T) {
	data := entryHandoffPackageProjectionFixture(
		[]componentmap.ComponentID{"component-package-owner"},
		"service.go",
	)
	handoff := data.ArchitectureGrounding.EntryHandoffs[0]
	data.ArchitectureCanvas.Components = append(
		data.ArchitectureCanvas.Components,
		ArchitectureComponent{
			ID: "component-exact-declaration", Name: "Exact declaration",
			Members: []componentmap.Candidate{{
				ID: componentmap.MemberID{Kind: componentmap.MemberSymbol, Value: "member-exact-service-start"},
				Facts: []componentmap.LocalFact{{
					Kind:     componentmap.FactDeclaration,
					Value:    handoff.Callee.ID,
					Location: &handoff.Callee.Location,
				}},
			}},
		},
	)
	transition := projectEntrypointPackageFixture(t, data)
	if !reflect.DeepEqual(
		transition.ComponentIDs,
		[]componentmap.ComponentID{"component-exact-declaration"},
	) {
		t.Fatalf("package fallback augmented an exact declaration join: %#v", transition.ComponentIDs)
	}
}

func TestEntrypointHandoffPackageMemberFallbackPermutationStable(t *testing.T) {
	firstData := entryHandoffPackageProjectionFixture(
		[]componentmap.ComponentID{"component-package-b", "component-package-a"},
		"service.go",
	)
	first, err := ProjectEntrypointHandoffGroups(
		firstData.ArchitectureCanvas,
		firstData.ArchitectureGrounding,
		firstData.RepositoryGraph,
	)
	if err != nil {
		t.Fatal(err)
	}

	reversedData := entryHandoffPackageProjectionFixture(
		[]componentmap.ComponentID{"component-package-a", "component-package-b"},
		"service.go",
	)
	reverseArchitectureComponents(reversedData.ArchitectureCanvas.Components)
	for index := range reversedData.ArchitectureCanvas.Components {
		reverseCandidates(reversedData.ArchitectureCanvas.Components[index].Members)
		reverseCandidates(reversedData.ArchitectureCanvas.Components[index].SharedMembers)
	}
	reversePackageInfo(reversedData.RepositoryGraph.Packages)
	for index := range reversedData.RepositoryGraph.Packages {
		reverseStrings(reversedData.RepositoryGraph.Packages[index].Files)
	}
	reverseBehaviorAnchors(reversedData.ArchitectureCanvas.BehaviorAnchors)
	reversed, err := ProjectEntrypointHandoffGroups(
		reversedData.ArchitectureCanvas,
		reversedData.ArchitectureGrounding,
		reversedData.RepositoryGraph,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, reversed) {
		t.Fatalf("package/component/file permutation changed projection:\nfirst=%#v\nreversed=%#v", first, reversed)
	}
}

func TestRunManifestRevalidatesEntrypointHandoffPackageInventory(t *testing.T) {
	data := entryHandoffPackageProjectionFixture(
		[]componentmap.ComponentID{"component-package-a"},
		"service.go",
	)
	data.FormatVersion = CurrentFormatVersion
	data.OpenablePaths = []string{"main.go", "service.go"}
	if err := ensureEntrypointHandoffGroups(data); err != nil {
		t.Fatal(err)
	}
	if err := ensureArchitectureComponentNavigation(data); err != nil {
		t.Fatal(err)
	}

	verify := func(t *testing.T, data *ReportData) error {
		t.Helper()
		reportJSON, err := json.Marshal(data)
		if err != nil {
			t.Fatal(err)
		}
		manifest := validRunManifestFixture(t)
		manifest.OpenablePaths = append([]string(nil), data.OpenablePaths...)
		manifest.Components = nil
		manifest.ReportSHA256 = manifestSHA256(reportJSON)
		return manifest.VerifyReportJSON(reportJSON)
	}
	if err := verify(t, data); err != nil {
		t.Fatalf("valid package-bound entry handoff rejected: %v", err)
	}

	// Rebind the outer report SHA to isolate the semantic verifier: changing the
	// exact package file inventory while retaining the persisted cube join must
	// still fail re-derivation.
	data.RepositoryGraph.Packages[0].Files = []string{"service/generated.go"}
	if err := verify(t, data); err == nil ||
		!strings.Contains(err.Error(), "persisted projection does not match exact local evidence") {
		t.Fatalf("package-inventory drift error = %v", err)
	}
}

func reverseArchitectureComponents(values []ArchitectureComponent) {
	for left, right := 0, len(values)-1; left < right; left, right = left+1, right-1 {
		values[left], values[right] = values[right], values[left]
	}
}

func reversePackageInfo(values []PackageInfo) {
	for left, right := 0, len(values)-1; left < right; left, right = left+1, right-1 {
		values[left], values[right] = values[right], values[left]
	}
}

func reverseCandidates(values []componentmap.Candidate) {
	for left, right := 0, len(values)-1; left < right; left, right = left+1, right-1 {
		values[left], values[right] = values[right], values[left]
	}
}

func reverseStrings(values []string) {
	for left, right := 0, len(values)-1; left < right; left, right = left+1, right-1 {
		values[left], values[right] = values[right], values[left]
	}
}

func reverseBehaviorAnchors(values []componentmap.BehaviorAnchor) {
	for left, right := 0, len(values)-1; left < right; left, right = left+1, right-1 {
		values[left], values[right] = values[right], values[left]
	}
}
