package main

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/dvordrova/repomap/internal/atlasstudy"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/modelresearch"
	"github.com/dvordrova/repomap/internal/repositoryatlas"
	"github.com/dvordrova/repomap/internal/themestudy"
)

func TestD239ThemeStudyClosureCompiles998UnitAtlasWithoutChangingScoutWire(t *testing.T) {
	input := atlasStudyRuntimeInput()
	const packageCount = 995
	for index := 0; index < packageCount; index++ {
		input.Atlas.Units = append(input.Atlas.Units, repositoryatlas.Unit{
			ID:       fmt.Sprintf("unit-package-%03d", index),
			Kind:     repositoryatlas.UnitPackage,
			ParentID: "unit-module-runtime",
			Name:     fmt.Sprintf("example.com/runtime/generated/%03d", index),
		})
	}
	// One disconnected observation proves the closure does not keep an Atlas
	// fact merely because its package happened to be present in the complete
	// authoritative artifact.
	input.Atlas.Entities = append(input.Atlas.Entities, repositoryatlas.Entity{
		ID: "boundary-unrelated", Kind: repositoryatlas.EntityBoundary,
		UnitID: "unit-package-994",
	})
	input.Atlas.Evidence = append(input.Atlas.Evidence, repositoryatlas.Evidence{
		ID: "evidence-unrelated", UnitID: "unit-package-994",
		Location:   evidence.Location{Path: "generated/unused.go", Line: 1},
		Provenance: evidence.Provenance{Provider: "fixture", Operation: "observe_unused"},
	})
	input.Atlas.Observations = append(input.Atlas.Observations, repositoryatlas.Observation{
		ID: "observation-unrelated", UnitID: "unit-package-994",
		Subject: repositoryatlas.EntityRef{
			Kind: repositoryatlas.EntityBoundary, ID: "boundary-unrelated",
		},
		EvidenceRefs: []string{"evidence-unrelated"},
	})
	if got := len(input.Atlas.Units); got != 998 {
		t.Fatalf("fixture units = %d, want 998", got)
	}
	if _, err := atlasstudy.Compile(input); err == nil {
		t.Fatal("unshaped 998-unit input unexpectedly compiled")
	} else {
		var limit *atlasstudy.ResourceLimitError
		if !errors.As(err, &limit) || limit.Section != "units" ||
			limit.Limit != atlasstudy.DefaultLimits().MaxUnits || limit.Actual != 998 {
			t.Fatalf("unshaped error = %#v, want exact units resource limit", err)
		}
	}

	shaped, closure, err := shapeThemeStudyCompileInput(input)
	if err != nil {
		t.Fatalf("shapeThemeStudyCompileInput: %v", err)
	}
	if closure.ObservedUnits != 998 || closure.RetainedUnits != 3 ||
		closure.ObservedEntities != 3 || closure.RetainedEntities != 2 ||
		closure.ObservedEvidence != 2 || closure.RetainedEvidence != 1 ||
		closure.ObservedObservations != 1 || closure.RetainedObservations != 0 ||
		closure.ObservedRelations != 1 || closure.RetainedRelations != 1 {
		t.Fatalf("closure = %#v", closure)
	}
	if got := len(input.Atlas.Units); got != 998 {
		t.Fatalf("closure mutated authoritative caller Atlas: units = %d", got)
	}
	product, err := atlasstudy.Compile(shaped)
	if err != nil {
		t.Fatalf("compile shaped input: %v", err)
	}

	baseline := atlasStudyRuntimeInput()
	baselineProduct, err := atlasstudy.Compile(baseline)
	if err != nil {
		t.Fatalf("compile semantic baseline: %v", err)
	}
	if !reflect.DeepEqual(themeSeedSpecsFromInput(shaped), themeSeedSpecsFromInput(baseline)) {
		t.Fatal("private Atlas closure changed Theme seed identities")
	}
	reader := func(string, int, int) ([]string, error) {
		return []string{"package fixture", "func RunServer() {}"}, nil
	}
	totalLines := func(string) (int, error) { return 2, nil }
	packs, err := themestudy.BuildSeedPacks(
		themeSeedSpecsFromInput(shaped), 0, 0, 0, 0, reader, totalLines,
	)
	if err != nil {
		t.Fatalf("build seed packs: %v", err)
	}
	vocabulary := themestudy.BuildFileVocabulary([]string{
		"cmd/server/main.go", "internal/config/load.go", "internal/server/routes.go",
	}, 0, nil)
	spanRefs := themeSpanAnchorRefsFromPacks(packs)
	shapedRequest, err := themestudy.CompileScout(
		themestudy.LanguageEnglish, vocabulary, packs,
		themeScoutContext(product, "fixture", spanRefs), "",
	)
	if err != nil {
		t.Fatalf("compile shaped Scout request: %v", err)
	}
	baselineRequest, err := themestudy.CompileScout(
		themestudy.LanguageEnglish, vocabulary, packs,
		themeScoutContext(baselineProduct, "fixture", spanRefs), "",
	)
	if err != nil {
		t.Fatalf("compile baseline Scout request: %v", err)
	}
	if shapedRequest.WireJSON != baselineRequest.WireJSON ||
		shapedRequest.WireSHA256 != baselineRequest.WireSHA256 ||
		shapedRequest.CatalogSHA256 != baselineRequest.CatalogSHA256 {
		t.Fatal("private Atlas scaffold changed the exact model-visible Scout request")
	}
}

func TestD239ThemeResourceLimitClassifiesCatalogItemsTruthfully(t *testing.T) {
	tests := []struct {
		section string
		kind    modelresearch.ResourceLimitKind
	}{
		{section: "units", kind: modelresearch.ResourceLimitCatalogItems},
		{section: "reading_targets", kind: modelresearch.ResourceLimitCatalogItems},
		{section: "wire_bytes", kind: modelresearch.ResourceLimitRequestBytes},
		{section: "response_bytes", kind: modelresearch.ResourceLimitResponseBytes},
		{section: "theme_scout_request_artifact_bytes", kind: modelresearch.ResourceLimitRecordBytes},
	}
	for _, test := range tests {
		t.Run(test.section, func(t *testing.T) {
			err := themeTerminalResource(&atlasstudy.ResourceLimitError{
				Section: test.section, Limit: 512, Actual: 998,
			}, 0)
			var resource *modelresearch.ResourceLimitError
			if !errors.As(err, &resource) {
				t.Fatalf("error = %T %v, want typed resource limit", err, err)
			}
			if resource.Stage != "theme_study" || resource.Kind != test.kind ||
				resource.Limit != 512 || resource.Observed != 998 || !resource.ObservedKnown {
				t.Fatalf("resource = %#v", resource)
			}
		})
	}
}
