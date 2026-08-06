package navigator

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/repositoryatlas"
)

func TestCompileBuildsDeterministicScopedWireWithoutCanonicalOrRawFacts(t *testing.T) {
	input := navigatorInput(twoUnitAtlas())
	first, err := Compile(input)
	if err != nil {
		t.Fatal(err)
	}

	shuffled := input
	shuffled.Atlas = reverseAtlas(input.Atlas)
	second, err := Compile(shuffled)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first.WireJSON(), second.WireJSON()) ||
		first.WireSHA256() != second.WireSHA256() ||
		first.CatalogSHA256() != second.CatalogSHA256() ||
		first.CatalogRef() != second.CatalogRef() {
		t.Fatal("shuffled canonical Atlas changed Navigator request identity")
	}

	wireBytes := first.WireJSON()
	for _, forbidden := range []string{
		"module-a-canonical", "app-a-canonical", "surface-a-canonical", "operation-a-canonical",
		"evidence-a-canonical", "relation-a-canonical", "module-b-canonical", "surface-b-canonical",
		"cmd/a/main.go", "example.com/project/cmd/a.main", "file_tree", "internal_edges", "source_signals",
	} {
		if strings.Contains(string(wireBytes), forbidden) {
			t.Fatalf("wire leaked %q: %s", forbidden, wireBytes)
		}
	}
	var wire wireProjection
	if err := json.Unmarshal(wireBytes, &wire); err != nil {
		t.Fatal(err)
	}
	if wire.Version != Version || wire.CatalogRef != first.CatalogRef() ||
		len(wire.Units) != 2 || len(wire.Entities) != 2 || len(wire.SeedRefs) != 1 ||
		len(wire.DirectTrails) != 1 || len(wire.Evidence) != 1 || len(wire.Intersections) != 0 ||
		len(wire.Gaps) != 1 || len(wire.Actions) != 1 {
		t.Fatalf("compiled wire shape = %#v", wire)
	}
	if wire.Units[0].Ref == "" || wire.ScopeRef == "" || wire.DirectTrails[0].Authority != repositoryatlas.AuthorityResolved {
		t.Fatalf("wire refs/authority = %#v", wire)
	}
	labels := unitLabelsByKind(wire.Units)
	if labels[repositoryatlas.UnitModule] != "example.com/project/a" ||
		labels[repositoryatlas.UnitApp] != "example.com/project/a/cmd/a" {
		t.Fatalf("wire Unit labels = %#v", labels)
	}
	roles := entityRolesByKind(wire.Entities)
	if roles[repositoryatlas.EntitySurface] != EntityRoleProcessEntry ||
		roles[repositoryatlas.EntityOperation] != EntityRoleApplicationStart {
		t.Fatalf("resolved startup entity roles = %#v", roles)
	}
	if entries := first.catalog.byCanonical["surface-a-canonical"]; len(entries) != 1 ||
		entries[0].EntityRole != EntityRoleProcessEntry {
		t.Fatalf("private catalog surface role = %#v", entries)
	}

	changed := input
	changed.Question = "Which exact startup boundary is locally resolved?"
	drifted, err := Compile(changed)
	if err != nil {
		t.Fatal(err)
	}
	if drifted.CatalogRef() == first.CatalogRef() || drifted.WireSHA256() == first.WireSHA256() {
		t.Fatal("question drift did not change exact request identity")
	}
	actionDrift := input
	actionDrift.Actions = append([]Action(nil), input.Actions...)
	actionDrift.Actions[0].Operation = "inspect the selected exact evidence"
	drifted, err = Compile(actionDrift)
	if err != nil {
		t.Fatal(err)
	}
	if drifted.CatalogRef() == first.CatalogRef() {
		t.Fatal("action meaning drift did not change request-bound catalog identity")
	}
}

func TestCompileUsesGenericRolesWithoutExactResolvedStartupProof(t *testing.T) {
	atlas := twoUnitAtlas()
	atlas.Relations[0].Authority = repositoryatlas.AuthorityInferred
	compiled, err := Compile(navigatorInput(atlas))
	if err != nil {
		t.Fatal(err)
	}
	var wire wireProjection
	if err := json.Unmarshal(compiled.WireJSON(), &wire); err != nil {
		t.Fatal(err)
	}
	roles := entityRolesByKind(wire.Entities)
	if roles[repositoryatlas.EntitySurface] != EntityRoleGenericSurface ||
		roles[repositoryatlas.EntityOperation] != EntityRoleGenericOperation {
		t.Fatalf("unproved startup entity roles = %#v", roles)
	}
}

func TestCompileRejectsCrossUnitWrongKindDuplicatesAndExplicitLimits(t *testing.T) {
	base := navigatorInput(twoUnitAtlas())
	tests := []struct {
		name   string
		mutate func(*Input)
		want   string
		limit  bool
	}{
		{
			name: "cross-unit seed",
			mutate: func(input *Input) {
				input.Seeds = []repositoryatlas.EntityRef{{Kind: repositoryatlas.EntitySurface, ID: "surface-b-canonical"}}
			},
			want: "outside the requested Unit scope",
		},
		{
			name: "wrong-kind seed",
			mutate: func(input *Input) {
				input.Seeds[0].Kind = repositoryatlas.EntityOperation
			},
			want: "wrong entity kind",
		},
		{
			name: "duplicate seed",
			mutate: func(input *Input) {
				input.Seeds = append(input.Seeds, input.Seeds[0])
			},
			want: "duplicate seed entity",
		},
		{
			name: "canonical id in question",
			mutate: func(input *Input) {
				input.Question = "Explain surface-a-canonical"
			},
			want: "canonical identity",
		},
		{
			name: "canonical id in unit label",
			mutate: func(input *Input) {
				input.Atlas.Units = append([]repositoryatlas.Unit(nil), input.Atlas.Units...)
				input.Atlas.Units[2].Name = "surface-a-canonical"
			},
			want: "Unit label exposes a canonical identity",
		},
		{
			name: "evidence source locator in unit label",
			mutate: func(input *Input) {
				input.Atlas.Units = append([]repositoryatlas.Unit(nil), input.Atlas.Units...)
				input.Atlas.Units[2].Name = "cmd/a/main.go"
			},
			want: "Unit label exposes a canonical identity or source locator",
		},
		{
			name: "trail budget",
			mutate: func(input *Input) {
				input.Limits.MaxDirectTrails = 0
			},
			want:  "direct_trails",
			limit: true,
		},
		{
			name: "wire budget",
			mutate: func(input *Input) {
				input.Limits.MaxWireBytes = 32
			},
			want:  "wire_bytes",
			limit: true,
		},
		{
			name: "unit label budget",
			mutate: func(input *Input) {
				input.Limits.MaxUnitLabelBytes = 4
			},
			want:  "unit_label_bytes",
			limit: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := base
			input.Seeds = append([]repositoryatlas.EntityRef(nil), base.Seeds...)
			test.mutate(&input)
			_, err := Compile(input)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Compile error = %v, want %q", err, test.want)
			}
			var limitErr *ResourceLimitError
			if test.limit != errors.As(err, &limitErr) {
				t.Fatalf("resource error type = %T, want limit=%t", err, test.limit)
			}
		})
	}
}

func TestCompileDerivesOnlyDirectIntersectionFromSuppliedSeeds(t *testing.T) {
	atlas := sharedOperationAtlas()
	input := Input{
		Atlas: atlas, Question: "Where do the selected entry surfaces meet?", ScopeUnitID: "module-shared-canonical",
		Seeds: []repositoryatlas.EntityRef{
			{Kind: repositoryatlas.EntitySurface, ID: "surface-one-canonical"},
			{Kind: repositoryatlas.EntitySurface, ID: "surface-two-canonical"},
		},
		Limits: generousLimits(),
	}
	compiled, err := Compile(input)
	if err != nil {
		t.Fatal(err)
	}
	var wire wireProjection
	if err := json.Unmarshal(compiled.WireJSON(), &wire); err != nil {
		t.Fatal(err)
	}
	if len(wire.DirectTrails) != 2 || len(wire.Intersections) != 1 ||
		len(wire.Intersections[0].SeedRefs) != 2 || len(wire.Intersections[0].TrailRefs) != 2 {
		t.Fatalf("direct intersection = %#v", wire)
	}
}

func TestValidateResponseJSONResolvesExactRefsAndRejectsRepair(t *testing.T) {
	compiled, err := Compile(navigatorInput(twoUnitAtlas()))
	if err != nil {
		t.Fatal(err)
	}
	var wire wireProjection
	if err := json.Unmarshal(compiled.WireJSON(), &wire); err != nil {
		t.Fatal(err)
	}
	valid := responseEnvelope{
		Version: Version, CatalogRef: compiled.CatalogRef(),
		ActionRefs: []string{wire.Actions[0].Ref},
	}
	resolved, err := compiled.ValidateResponseJSON(mustJSON(t, valid))
	if err != nil {
		t.Fatal(err)
	}
	// Decision 232 (Navigator v2): the model selects the action only; the
	// trail/endpoints/evidence are backend-restored and not echoed.
	if len(resolved.Entities) != 0 || len(resolved.RelationIDs) != 0 ||
		len(resolved.EvidenceIDs) != 0 || len(resolved.GapKeys) != 0 {
		t.Fatalf("v2 response resolved backend-owned refs from the wire: %#v", resolved)
	}
	if !reflect.DeepEqual(resolved.ActionKeys, []string{"inspect-evidence"}) ||
		!reflect.DeepEqual(resolved.Actions, []ResolvedAction{{
			Key: "inspect-evidence", Operation: "inspect exact supporting evidence",
			Target: repositoryatlas.EntityRef{Kind: repositoryatlas.EntitySurface, ID: "surface-a-canonical"},
		}}) {
		t.Fatalf("resolved response = %#v", resolved)
	}

	driftedInput := navigatorInput(twoUnitAtlas())
	driftedInput.Question = "What else is exact?"
	drifted, err := Compile(driftedInput)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name  string
		value responseEnvelope
		want  string
	}{
		{name: "cross request", value: responseEnvelope{Version: Version, CatalogRef: drifted.CatalogRef()}, want: "catalog_ref_mismatch"},
		{name: "wrong kind", value: responseEnvelope{Version: Version, CatalogRef: compiled.CatalogRef(), ActionRefs: []string{wire.SeedRefs[0]}}, want: "wrong_kind_ref"},
		{name: "duplicate", value: responseEnvelope{Version: Version, CatalogRef: compiled.CatalogRef(), ActionRefs: []string{wire.Actions[0].Ref, wire.Actions[0].Ref}}, want: "duplicate_ref"},
		{name: "unknown substituted ref", value: responseEnvelope{Version: Version, CatalogRef: compiled.CatalogRef(), ActionRefs: []string{"a9"}}, want: "unknown_ref"},
		{name: "cross-unit canonical ref", value: responseEnvelope{Version: Version, CatalogRef: compiled.CatalogRef(), ActionRefs: []string{"surface-b-canonical"}}, want: "cross_scope_ref"},
		{name: "raw canonical", value: responseEnvelope{Version: Version, CatalogRef: compiled.CatalogRef(), ActionRefs: []string{"surface-a-canonical"}}, want: "raw_canonical_ref"},
		{name: "echo forbidden", value: responseEnvelope{Version: Version, CatalogRef: compiled.CatalogRef(), ActionRefs: []string{wire.Actions[0].Ref}, EntityRefs: []string{wire.SeedRefs[0]}}, want: "must not echo"},
		{name: "no action", value: responseEnvelope{Version: Version, CatalogRef: compiled.CatalogRef()}, want: "exactly one advertised action"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := compiled.ValidateResponseJSON(mustJSON(t, test.value))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ValidateResponseJSON error = %v, want %q", err, test.want)
			}
		})
	}
	validJSON := mustJSON(t, valid)
	unknown := append(validJSON[:len(validJSON)-1], []byte(`,"extra":true}`)...)
	if _, err := compiled.ValidateResponseJSON(unknown); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unknown field error = %v", err)
	}
	oversized := make([]byte, compiled.maxResponseBytes+1)
	_, err = compiled.ValidateResponseJSON(oversized)
	var limitErr *ResourceLimitError
	if !errors.As(err, &limitErr) || limitErr.Section != "response_bytes" {
		t.Fatalf("oversized response error = %v", err)
	}
}

func TestCompileCasdoorAndEtcdLikeExactSlicesStayBounded(t *testing.T) {
	for _, test := range []struct {
		name  string
		count int
	}{
		{name: "casdoor-like", count: 1},
		{name: "etcd-like", count: 18},
	} {
		t.Run(test.name, func(t *testing.T) {
			atlas, seeds := manyProcessAtlas(test.count)
			limits := generousLimits()
			limits.MaxSeeds = test.count
			limits.MaxDirectTrails = test.count
			limits.MaxEvidence = test.count
			compiled, err := Compile(Input{
				Atlas: atlas, Question: "Which locally resolved process entries support this orientation?",
				ScopeUnitID: "repository-many-canonical", Seeds: seeds, Limits: limits,
			})
			if err != nil {
				t.Fatal(err)
			}
			var wire wireProjection
			if err := json.Unmarshal(compiled.WireJSON(), &wire); err != nil {
				t.Fatal(err)
			}
			if len(wire.SeedRefs) != test.count || len(wire.DirectTrails) != test.count ||
				len(wire.Evidence) != test.count || len(compiled.WireJSON()) > limits.MaxWireBytes {
				t.Fatalf("compiled counts/bytes = %d/%d/%d/%d", len(wire.SeedRefs), len(wire.DirectTrails), len(wire.Evidence), len(compiled.WireJSON()))
			}
			if strings.Contains(string(compiled.WireJSON()), "cmd/service-") {
				t.Fatalf("wire leaked source locators: %s", compiled.WireJSON())
			}
		})
	}
}

func navigatorInput(atlas repositoryatlas.Atlas) Input {
	return Input{
		Atlas: atlas, Question: "Which exact startup surface is locally resolved?", ScopeUnitID: "module-a-canonical",
		Seeds: []repositoryatlas.EntityRef{{Kind: repositoryatlas.EntitySurface, ID: "surface-a-canonical"}},
		Gaps: []ProvenGap{{
			Key: "missing-shutdown", Meaning: "No shutdown relation is proven for the selected surface.",
			Subject:     repositoryatlas.EntityRef{Kind: repositoryatlas.EntitySurface, ID: "surface-a-canonical"},
			EvidenceIDs: []string{"evidence-a-canonical"},
		}},
		Actions: []Action{{
			Key: "inspect-evidence", Operation: "inspect exact supporting evidence",
			Target: repositoryatlas.EntityRef{Kind: repositoryatlas.EntitySurface, ID: "surface-a-canonical"},
		}},
		Limits: generousLimits(),
	}
}

func generousLimits() Limits {
	return Limits{
		MaxWireBytes: 128 << 10, MaxResponseBytes: 128 << 10,
		MaxUnitLabelBytes: 512,
		MaxSeeds:          32, MaxDirectTrails: 64,
		MaxIntersections: 32, MaxEvidence: 128, MaxGaps: 32, MaxActions: 32,
	}
}

func twoUnitAtlas() repositoryatlas.Atlas {
	atlas, _ := manyProcessAtlas(2)
	atlas.Units = []repositoryatlas.Unit{
		{ID: "repository-root-canonical", Kind: repositoryatlas.UnitRepository, Name: "fixture"},
		{ID: "module-a-canonical", Kind: repositoryatlas.UnitModule, ParentID: "repository-root-canonical", Name: "example.com/project/a"},
		{ID: "app-a-canonical", Kind: repositoryatlas.UnitApp, ParentID: "module-a-canonical", Name: "example.com/project/a/cmd/a"},
		{ID: "module-b-canonical", Kind: repositoryatlas.UnitModule, ParentID: "repository-root-canonical", Name: "example.com/project/b"},
		{ID: "app-b-canonical", Kind: repositoryatlas.UnitApp, ParentID: "module-b-canonical", Name: "example.com/project/b/cmd/b"},
	}
	atlas.Entities = nil
	atlas.Observations = nil
	atlas.Evidence = nil
	atlas.Relations = nil
	for _, suffix := range []string{"a", "b"} {
		appID := "app-" + suffix + "-canonical"
		surfaceID := "surface-" + suffix + "-canonical"
		operationID := "operation-" + suffix + "-canonical"
		evidenceID := "evidence-" + suffix + "-canonical"
		relationID := "relation-" + suffix + "-canonical"
		atlas.Entities = append(atlas.Entities,
			repositoryatlas.Entity{ID: surfaceID, Kind: repositoryatlas.EntitySurface, UnitID: appID},
			repositoryatlas.Entity{ID: operationID, Kind: repositoryatlas.EntityOperation, UnitID: appID},
		)
		atlas.Evidence = append(atlas.Evidence, repositoryatlas.Evidence{
			ID: evidenceID, UnitID: appID,
			Location:   evidence.Location{Path: "cmd/" + suffix + "/main.go", Line: 7},
			Symbol:     "example.com/project/cmd/" + suffix + ".main",
			Provenance: evidence.Provenance{Provider: "gofacts", Operation: "build_selected_main_declaration"},
		})
		atlas.Observations = append(atlas.Observations,
			repositoryatlas.Observation{ID: "observation-surface-" + suffix, UnitID: appID, Subject: repositoryatlas.EntityRef{Kind: repositoryatlas.EntitySurface, ID: surfaceID}, EvidenceRefs: []string{evidenceID}},
			repositoryatlas.Observation{ID: "observation-operation-" + suffix, UnitID: appID, Subject: repositoryatlas.EntityRef{Kind: repositoryatlas.EntityOperation, ID: operationID}, EvidenceRefs: []string{evidenceID}},
		)
		atlas.Relations = append(atlas.Relations, repositoryatlas.Relation{
			ID: relationID, UnitID: appID, Kind: repositoryatlas.RelationExposes,
			Source: repositoryatlas.EntityRef{Kind: repositoryatlas.EntitySurface, ID: surfaceID},
			Target: repositoryatlas.EntityRef{Kind: repositoryatlas.EntityOperation, ID: operationID},
			Phase:  repositoryatlas.PhaseStartup, Authority: repositoryatlas.AuthorityResolved,
			EvidenceRefs: []string{evidenceID},
		})
	}
	return atlas
}

func sharedOperationAtlas() repositoryatlas.Atlas {
	atlas := repositoryatlas.Atlas{
		Version: repositoryatlas.Version,
		Units: []repositoryatlas.Unit{
			{ID: "repository-shared-canonical", Kind: repositoryatlas.UnitRepository, Name: "shared"},
			{ID: "module-shared-canonical", Kind: repositoryatlas.UnitModule, ParentID: "repository-shared-canonical", Name: "example.com/shared"},
			{ID: "app-shared-canonical", Kind: repositoryatlas.UnitApp, ParentID: "module-shared-canonical", Name: "example.com/shared/cmd/app"},
		},
		Entities: []repositoryatlas.Entity{
			{ID: "surface-one-canonical", Kind: repositoryatlas.EntitySurface, UnitID: "app-shared-canonical"},
			{ID: "surface-two-canonical", Kind: repositoryatlas.EntitySurface, UnitID: "app-shared-canonical"},
			{ID: "operation-shared-canonical", Kind: repositoryatlas.EntityOperation, UnitID: "app-shared-canonical"},
		},
	}
	for index, surfaceID := range []string{"surface-one-canonical", "surface-two-canonical"} {
		evidenceID := fmt.Sprintf("evidence-shared-%d-canonical", index+1)
		atlas.Evidence = append(atlas.Evidence, repositoryatlas.Evidence{
			ID: evidenceID, UnitID: "app-shared-canonical", Location: evidence.Location{Path: fmt.Sprintf("cmd/app/main%d.go", index+1), Line: 7},
			Provenance: evidence.Provenance{Provider: "fixture", Operation: "exact_registration"},
		})
		atlas.Observations = append(atlas.Observations,
			repositoryatlas.Observation{ID: "observation-" + surfaceID, UnitID: "app-shared-canonical", Subject: repositoryatlas.EntityRef{Kind: repositoryatlas.EntitySurface, ID: surfaceID}, EvidenceRefs: []string{evidenceID}},
			repositoryatlas.Observation{ID: fmt.Sprintf("observation-operation-%d", index+1), UnitID: "app-shared-canonical", Subject: repositoryatlas.EntityRef{Kind: repositoryatlas.EntityOperation, ID: "operation-shared-canonical"}, EvidenceRefs: []string{evidenceID}},
		)
		atlas.Relations = append(atlas.Relations, repositoryatlas.Relation{
			ID: fmt.Sprintf("relation-shared-%d-canonical", index+1), UnitID: "app-shared-canonical", Kind: repositoryatlas.RelationExposes,
			Source: repositoryatlas.EntityRef{Kind: repositoryatlas.EntitySurface, ID: surfaceID},
			Target: repositoryatlas.EntityRef{Kind: repositoryatlas.EntityOperation, ID: "operation-shared-canonical"},
			Phase:  repositoryatlas.PhaseStartup, Authority: repositoryatlas.AuthorityResolved,
			EvidenceRefs: []string{evidenceID},
		})
	}
	return atlas
}

func manyProcessAtlas(count int) (repositoryatlas.Atlas, []repositoryatlas.EntityRef) {
	atlas := repositoryatlas.Atlas{
		Version: repositoryatlas.Version,
		Units: []repositoryatlas.Unit{
			{ID: "repository-many-canonical", Kind: repositoryatlas.UnitRepository, Name: "many"},
			{ID: "module-many-canonical", Kind: repositoryatlas.UnitModule, ParentID: "repository-many-canonical", Name: "example.com/many"},
		},
	}
	var seeds []repositoryatlas.EntityRef
	for index := 1; index <= count; index++ {
		suffix := fmt.Sprintf("%02d-canonical", index)
		appID := "app-many-" + suffix
		surfaceID := "surface-many-" + suffix
		operationID := "operation-many-" + suffix
		evidenceID := "evidence-many-" + suffix
		atlas.Units = append(atlas.Units, repositoryatlas.Unit{ID: appID, Kind: repositoryatlas.UnitApp, ParentID: "module-many-canonical", Name: "service-" + suffix})
		atlas.Entities = append(atlas.Entities,
			repositoryatlas.Entity{ID: surfaceID, Kind: repositoryatlas.EntitySurface, UnitID: appID},
			repositoryatlas.Entity{ID: operationID, Kind: repositoryatlas.EntityOperation, UnitID: appID},
		)
		atlas.Evidence = append(atlas.Evidence, repositoryatlas.Evidence{
			ID: evidenceID, UnitID: appID, Location: evidence.Location{Path: "cmd/service-" + suffix + "/main.go", Line: 7},
			Symbol:     "example.com/many/service-" + suffix + ".main",
			Provenance: evidence.Provenance{Provider: "gofacts", Operation: "build_selected_main_declaration"},
		})
		atlas.Observations = append(atlas.Observations,
			repositoryatlas.Observation{ID: "observation-surface-many-" + suffix, UnitID: appID, Subject: repositoryatlas.EntityRef{Kind: repositoryatlas.EntitySurface, ID: surfaceID}, EvidenceRefs: []string{evidenceID}},
			repositoryatlas.Observation{ID: "observation-operation-many-" + suffix, UnitID: appID, Subject: repositoryatlas.EntityRef{Kind: repositoryatlas.EntityOperation, ID: operationID}, EvidenceRefs: []string{evidenceID}},
		)
		atlas.Relations = append(atlas.Relations, repositoryatlas.Relation{
			ID: "relation-many-" + suffix, UnitID: appID, Kind: repositoryatlas.RelationExposes,
			Source: repositoryatlas.EntityRef{Kind: repositoryatlas.EntitySurface, ID: surfaceID},
			Target: repositoryatlas.EntityRef{Kind: repositoryatlas.EntityOperation, ID: operationID},
			Phase:  repositoryatlas.PhaseStartup, Authority: repositoryatlas.AuthorityResolved,
			EvidenceRefs: []string{evidenceID},
		})
		seeds = append(seeds, repositoryatlas.EntityRef{Kind: repositoryatlas.EntitySurface, ID: surfaceID})
	}
	return atlas, seeds
}

func reverseAtlas(atlas repositoryatlas.Atlas) repositoryatlas.Atlas {
	reversed := atlas
	reversed.Units = slices.Clone(atlas.Units)
	reversed.Entities = slices.Clone(atlas.Entities)
	reversed.Observations = slices.Clone(atlas.Observations)
	reversed.Evidence = slices.Clone(atlas.Evidence)
	reversed.Relations = slices.Clone(atlas.Relations)
	slices.Reverse(reversed.Units)
	slices.Reverse(reversed.Entities)
	slices.Reverse(reversed.Observations)
	slices.Reverse(reversed.Evidence)
	slices.Reverse(reversed.Relations)
	return reversed
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func entityRolesByKind(values []wireEntity) map[repositoryatlas.EntityKind]EntityRole {
	result := make(map[repositoryatlas.EntityKind]EntityRole, len(values))
	for _, value := range values {
		result[value.Kind] = value.Role
	}
	return result
}

func unitLabelsByKind(values []wireUnit) map[repositoryatlas.UnitKind]string {
	result := make(map[repositoryatlas.UnitKind]string, len(values))
	for _, value := range values {
		result[value.Kind] = value.Label
	}
	return result
}
