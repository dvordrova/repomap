package atlasstudy

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestReplayResponseRecordRevalidatesExactPartialSpanCoverage(t *testing.T) {
	product := mustCompileArtifactTestProduct(t, LanguageRussian)
	request, err := product.RequestRecord()
	if err != nil {
		t.Fatalf("RequestRecord: %v", err)
	}
	response := artifactResponse(t, product, 1, false)

	result, status, diagnostics, err := ReplayResponseRecord(request, response)
	if err != nil {
		t.Fatalf("ReplayResponseRecord: %v", err)
	}
	if result.State != ProductStateAcceptedPartial || status.State != ProductStateAcceptedPartial ||
		result.SpanCoverage.Complete || status.CoverageComplete ||
		len(result.SpanCoverage.Requested) != 3 || len(result.SpanCoverage.Covered) != 1 ||
		len(result.SpanCoverage.Uncovered) != 2 || status.RequestedSpanCount != 3 ||
		status.CoveredSpanCount != 1 || status.UncoveredSpanCount != 2 {
		t.Fatalf("partial replay coverage = result:%+v status:%+v", result.SpanCoverage, status)
	}
	if diagnostics.DirectionsReceived != 1 || diagnostics.DirectionsAccepted != 1 ||
		diagnostics.DirectionsRejected != 0 {
		t.Fatalf("partial replay diagnostics = %+v", diagnostics)
	}
	if !reflect.DeepEqual(request.CandidateCoverage, result.CandidateCoverage) ||
		!reflect.DeepEqual(request.CandidateCoverage, status.CandidateCoverage) {
		t.Fatal("candidate shelf coverage was not preserved across replay")
	}
	if _, err := EncodeResultRecord(result); err != nil {
		t.Fatalf("EncodeResultRecord: %v", err)
	}
	if _, err := EncodeStatus(status); err != nil {
		t.Fatalf("EncodeStatus: %v", err)
	}
	tamperedResult := result
	tamperedResult.SpanCoverage = cloneSpanCoverage(result.SpanCoverage)
	tamperedResult.SpanCoverage.Uncovered = nil
	tamperedResult.SpanCoverage.Complete = true
	if err := product.ValidateResultRecord(tamperedResult); err == nil {
		t.Fatal("tampered exact span identities validated")
	}
	tamperedStatus := status
	tamperedStatus.UncoveredSpanCount = 0
	tamperedStatus.CoverageComplete = true
	if err := product.ValidateStatus(tamperedStatus); err == nil {
		t.Fatal("tampered span counts validated")
	}
}

func TestReplayResponseRecordKeepsValidSiblingAndExactCoverage(t *testing.T) {
	product := mustCompileArtifactTestProduct(t, LanguageEnglish)
	request, err := product.RequestRecord()
	if err != nil {
		t.Fatal(err)
	}
	response := artifactResponseMap(t, product, 1)
	response["directions"] = append([]any{map[string]any{
		"span_ref": "sp999", "why_it_matters": "Unknown span.",
		"learning_outcome": "Unknown span.", "target_job": string(JobFirstContact),
		"learning_stage": string(StageOrientation), "principal_refs": []string{"c1"},
		"reading": []any{},
	}}, response["directions"].([]any)...)

	result, status, diagnostics, err := ReplayResponseRecord(request, marshalTestJSON(t, response))
	if err != nil {
		t.Fatalf("ReplayResponseRecord valid sibling: %v", err)
	}
	if len(result.Directions) != 1 || status.State != ProductStateAcceptedPartial ||
		diagnostics.DirectionsReceived != 2 || diagnostics.DirectionsAccepted != 1 ||
		diagnostics.DirectionsRejected != 1 || len(diagnostics.Issues) != 1 {
		t.Fatalf("sibling replay = result:%+v status:%+v diagnostics:%+v", result, status, diagnostics)
	}
}

func TestReplayResponseRecordRejectsTamperedRequestCoverageAndRoutePromise(t *testing.T) {
	product := mustCompileArtifactTestProduct(t, LanguageEnglish)
	request, err := product.RequestRecord()
	if err != nil {
		t.Fatal(err)
	}
	response := artifactResponse(t, product, 1, false)

	t.Run("candidate coverage", func(t *testing.T) {
		tampered := request
		tampered.CandidateCoverage.CandidateSHA256 = strings.Repeat("0", 64)
		if _, _, _, err := ReplayResponseRecord(tampered, response); err == nil {
			t.Fatal("tampered candidate shelf coverage replayed")
		}
	})

	t.Run("localized question", func(t *testing.T) {
		tampered := request
		tampered.Catalog = cloneCatalog(request.Catalog)
		for index := range tampered.Catalog {
			if tampered.Catalog[index].Kind == RefRouteSpan {
				tampered.Catalog[index].Question = "A different backend question?"
				break
			}
		}
		if _, _, _, err := ReplayResponseRecord(tampered, response); err == nil {
			t.Fatal("tampered localized span question replayed")
		}
	})

	t.Run("allowed targets", func(t *testing.T) {
		tampered := request
		tampered.Catalog = cloneCatalog(request.Catalog)
		for index := range tampered.Catalog {
			if tampered.Catalog[index].Kind == RefRouteSpan {
				tampered.Catalog[index].AllowedTargetRefs = nil
				break
			}
		}
		if _, _, _, err := ReplayResponseRecord(tampered, response); err == nil {
			t.Fatal("tampered span target allowlist replayed")
		}
	})

	t.Run("support target with rehashed catalog", func(t *testing.T) {
		tampered := request
		tampered.Catalog = cloneCatalog(request.Catalog)
		var replacement CanonicalRef
		for _, object := range tampered.Catalog {
			if object.Kind == RefReadingTarget {
				replacement = CanonicalRef{Kind: object.Kind, ID: object.CanonicalID}
			}
		}
		for index := range tampered.Catalog {
			if tampered.Catalog[index].Kind == RefRouteSupport &&
				tampered.Catalog[index].SupportTarget != nil &&
				*tampered.Catalog[index].SupportTarget != replacement {
				tampered.Catalog[index].SupportTarget = &replacement
				break
			}
		}
		rebindRequestCatalog(t, &tampered)
		if _, _, _, err := ReplayResponseRecord(tampered, response); err == nil {
			t.Fatal("rehashed support target tampering replayed")
		}
	})
}

func TestReplayResponseRecordRejectsRehashedPrivateProducerRelationTampering(t *testing.T) {
	product := mustCompileTestProduct(t, testInput())
	request := mustRequestRecord(t, product)

	tests := []struct {
		name   string
		change func([]CatalogObject)
	}{
		{
			name: "producer identity",
			change: func(catalog []CatalogObject) {
				for index := range catalog {
					if catalog[index].Kind == RefRouteRelation {
						catalog[index].ProducerID = ""
						return
					}
				}
			},
		},
		{
			name: "support endpoint",
			change: func(catalog []CatalogObject) {
				for index := range catalog {
					if catalog[index].Kind == RefRouteRelation {
						catalog[index].ToSupport = catalog[index].FromSupport
						return
					}
				}
			},
		},
		{
			name: "span join",
			change: func(catalog []CatalogObject) {
				for index := range catalog {
					if catalog[index].Kind == RefRouteSpan {
						catalog[index].SpanJoins = []CanonicalSpanJoin{{
							Relation: CanonicalRef{Kind: RefRouteRelation, ID: "unknown-relation"},
						}}
						return
					}
				}
			},
		},
		{
			name: "multiple joins for one system path",
			change: func(catalog []CatalogObject) {
				for index := range catalog {
					if catalog[index].Kind == RefRouteSpan {
						join := catalog[index].SpanJoins[0]
						catalog[index].SpanJoins = []CanonicalSpanJoin{join, join}
						return
					}
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tampered := request
			tampered.Catalog = cloneCatalog(request.Catalog)
			test.change(tampered.Catalog)
			rebindRequestCatalog(t, &tampered)
			if _, _, _, err := ReplayResponseRecord(tampered, []byte(`{}`)); err == nil {
				t.Fatal("rehashed private route relation tampering replayed")
			}
		})
	}
}

func TestReplayResponseRecordAcceptsExactlyOneDirectedProducerRelation(t *testing.T) {
	product := mustCompileTestProduct(t, testInput())
	request := mustRequestRecord(t, product)
	result, status, diagnostics, err := ReplayResponseRecord(request, validResponse(t, product))
	if err != nil {
		t.Fatalf("exact one-relation replay rejected: %v", err)
	}
	if result.State != ProductStateAccepted || status.State != ProductStateAccepted ||
		len(result.Directions) != 1 || diagnostics.DirectionsAccepted != 1 ||
		diagnostics.DirectionsRejected != 0 {
		t.Fatalf("exact one-relation replay = result:%+v status:%+v diagnostics:%+v", result, status, diagnostics)
	}
}

func TestD210RequestAndResultIdentityMissEarlierArtifactsClosed(t *testing.T) {
	legacy, err := os.ReadFile("testdata/casdoor_20260803_075743_request_v5.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeRequestRecord(legacy); err == nil {
		t.Fatal("D209 v5 request decoded under D210 v6")
	}
	product := mustCompileArtifactTestProduct(t, LanguageEnglish)
	request, err := product.RequestRecord()
	if err != nil {
		t.Fatal(err)
	}
	request.Version--
	if _, err := EncodeRequestRecord(request); err == nil {
		t.Fatal("earlier request version encoded under D210")
	}
	result, _, _, err := ReplayResponseRecord(mustRequestRecord(t, product), artifactResponse(t, product, 3, false))
	if err != nil {
		t.Fatal(err)
	}
	result.Version--
	if _, err := EncodeResultRecord(result); err == nil {
		t.Fatal("earlier result version encoded under D210")
	}
}

func TestD210RequestIdentitySeparatesEnglishAndRussianQuestions(t *testing.T) {
	en := mustCompileArtifactTestProduct(t, LanguageEnglish)
	ru := mustCompileArtifactTestProduct(t, LanguageRussian)
	if en.WireSHA256() == ru.WireSHA256() || en.CatalogSHA256() == ru.CatalogSHA256() {
		t.Fatal("localized backend questions did not bind request identity")
	}
	enRequest := mustRequestRecord(t, en)
	ruRequest := mustRequestRecord(t, ru)
	if reflect.DeepEqual(enRequest.Catalog, ruRequest.Catalog) || enRequest.WireJSON == ruRequest.WireJSON {
		t.Fatal("EN/RU route span questions were not persisted distinctly")
	}
}

func mustCompileArtifactTestProduct(t *testing.T, language Language) Product {
	t.Helper()
	input := cloneTestInput(testInput())
	input.Language = language
	input.ProducerRelations = nil
	input.RouteSpans = nil
	for _, support := range input.ReadingSupports {
		input.RouteSpans = append(input.RouteSpans, RouteSpan{
			ID: "artifact-span-" + support.ID, Kind: RouteSpanFocused,
			QuestionEnglish: "How should this exact code location be studied?",
			QuestionRussian: "Как изучить это точное место в коде?",
			TargetJob:       JobFirstContact, LearningStage: StageOrientation,
			RequiredSupportIDs: []string{support.ID}, AllowedTargetIDs: []string{support.TargetID},
		})
	}
	product, err := Compile(input)
	if err != nil {
		t.Fatalf("Compile artifact product: %v", err)
	}
	return product
}

func mustRequestRecord(t *testing.T, product Product) RequestRecord {
	t.Helper()
	record, err := product.RequestRecord()
	if err != nil {
		t.Fatalf("RequestRecord: %v", err)
	}
	return record
}

func artifactResponse(t *testing.T, product Product, directions int, invalidSibling bool) []byte {
	t.Helper()
	response := artifactResponseMap(t, product, directions)
	if invalidSibling {
		response["directions"] = append(response["directions"].([]any), map[string]any{"span_ref": "unknown"})
	}
	return marshalTestJSON(t, response)
}

func artifactResponseMap(t *testing.T, product Product, directionCount int) map[string]any {
	t.Helper()
	catalog := product.Catalog()
	component := catalogObject(t, catalog, RefComponent, "component-api-canonical")
	spans := make([]CatalogObject, 0)
	byCanonical := make(map[CanonicalRef]CatalogObject, len(catalog))
	for _, object := range catalog {
		byCanonical[CanonicalRef{Kind: object.Kind, ID: object.CanonicalID}] = object
		if object.Kind == RefRouteSpan {
			spans = append(spans, object)
		}
	}
	if directionCount > len(spans) {
		t.Fatalf("requested %d directions from %d spans", directionCount, len(spans))
	}
	directions := make([]any, 0, directionCount)
	for _, span := range spans[:directionCount] {
		target := byCanonical[span.AllowedTargetRefs[0]]
		principalRefs := make([]string, 0, len(target.PrincipalRefs))
		for _, principal := range target.PrincipalRefs {
			principalRefs = append(principalRefs, byCanonical[principal].Ref)
		}
		directions = append(directions, map[string]any{
			"span_ref": span.Ref, "why_it_matters": "This exact location is a supported starting point.",
			"learning_outcome": "The reader can identify the supported code boundary.",
			"target_job":       string(span.TargetJob), "learning_stage": string(span.LearningStage),
			"principal_refs": principalRefs,
			"reading": []any{map[string]any{
				"target_ref": target.Ref, "label": string(ReadingStart),
				"what_to_look_for": "Inspect the exact supported local behavior.",
			}},
		})
	}
	return map[string]any{
		"repository_type": string(RepositoryService),
		"brief": map[string]any{
			"what_it_is":             map[string]any{"text": "A repository service.", "support_refs": []string{component.Ref}},
			"problem":                map[string]any{"text": "It handles bounded repository behavior.", "support_refs": []string{component.Ref}},
			"main_input":             map[string]any{"text": "Requests enter through a supported component.", "support_refs": []string{component.Ref}},
			"central_responsibility": map[string]any{"text": "The component coordinates repository behavior.", "support_refs": []string{component.Ref}},
			"observable_result":      map[string]any{"text": "The service exposes observable behavior.", "support_refs": []string{component.Ref}},
		},
		"directions": directions,
	}
}

func TestArtifactResponseHelperPreservesCanonicalSpanOrder(t *testing.T) {
	product := mustCompileArtifactTestProduct(t, LanguageEnglish)
	result, status, _, err := ReplayResponseRecord(mustRequestRecord(t, product), artifactResponse(t, product, 3, false))
	if err != nil {
		t.Fatal(err)
	}
	covered := make([]string, 0, len(result.SpanCoverage.Covered))
	for _, ref := range result.SpanCoverage.Covered {
		covered = append(covered, ref.ID)
	}
	if result.State != ProductStateAccepted || status.State != ProductStateAccepted ||
		!result.SpanCoverage.Complete || !status.CoverageComplete ||
		len(result.SpanCoverage.Uncovered) != 0 || !slices.IsSorted(covered) {
		t.Fatalf("covered span order is not canonical: %v", covered)
	}
}

func decodeArtifactResponse(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func rebindRequestCatalog(t *testing.T, request *RequestRecord) {
	t.Helper()
	material := catalogMaterial{
		Version: Version, AtlasSHA256: request.AtlasSHA256,
		ArchitectureSHA256: request.ArchitectureSHA256, Language: request.Language,
		Limits: DefaultLimits(), ProjectionSHA256: request.WireSHA256,
		Coverage: cloneCandidateCoverage(request.CandidateCoverage), Objects: cloneCatalog(request.Catalog),
	}
	encoded, err := json.Marshal(material)
	if err != nil {
		t.Fatal(err)
	}
	request.CatalogSHA256 = digest(encoded)
	request.CatalogRef = fmt.Sprintf("atlas-study-v%d-%s", Version, request.CatalogSHA256)
}
