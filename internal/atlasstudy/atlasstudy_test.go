package atlasstudy

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

func TestCompileBuildsPrivateTypedCatalogAndSafeDeterministicWire(t *testing.T) {
	input := testInput()
	original := cloneTestInput(input)
	product, err := Compile(input)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if !reflect.DeepEqual(input, original) {
		t.Fatal("Compile mutated caller-owned input")
	}
	wire := string(product.WireJSON())
	if strings.Contains(wire, "catalog_ref") || strings.Contains(wire, product.CatalogRef()) {
		t.Fatalf("provider wire exposed private catalog identity: %s", wire)
	}
	for _, hidden := range []string{
		"unit-repository-canonical", "component-api-canonical", "surface-start-canonical",
		"anchor-start-canonical", "cmd/server/main.go", "RunServer",
	} {
		if strings.Contains(wire, hidden) {
			t.Fatalf("provider wire exposed %q: %s", hidden, wire)
		}
	}
	for _, visible := range []string{
		`"ref":"u1"`, `"ref":"ss1"`, `"ref":"c1"`, `"ref":"sf1"`,
		`"ref":"a1"`, `"ref":"e1"`, `"ref":"d1"`,
		`"authority":"resolved"`, `"language":"en"`,
	} {
		if !strings.Contains(wire, visible) {
			t.Fatalf("provider wire missing %q: %s", visible, wire)
		}
	}
	if want := fmt.Sprintf("Return 1-%d directions", MaxDirections); !strings.Contains(product.BuildPrompt().System, want) {
		t.Fatalf("provider prompt does not use the production route bound %q", want)
	}
	catalog := product.Catalog()
	target := catalogObject(t, catalog, RefReadingTarget, "anchor-start-canonical")
	if target.Location == nil || target.Location.Path != "cmd/server/main.go" || target.Symbol != "RunServer" {
		t.Fatalf("private target locator = %#v", target)
	}

	reordered := cloneTestInput(input)
	slices.Reverse(reordered.Architecture.Subsystems)
	slices.Reverse(reordered.Architecture.Components)
	slices.Reverse(reordered.ReadingTargets)
	slices.Reverse(reordered.Evidence)
	second, err := Compile(reordered)
	if err != nil {
		t.Fatalf("Compile reordered: %v", err)
	}
	if product.WireSHA256() != second.WireSHA256() ||
		product.CatalogSHA256() != second.CatalogSHA256() ||
		product.AtlasSHA256() != second.AtlasSHA256() ||
		product.ArchitectureSHA256() != second.ArchitectureSHA256() {
		t.Fatal("permutation changed exact product identity")
	}

	russian := cloneTestInput(input)
	russian.Language = LanguageRussian
	ruProduct, err := Compile(russian)
	if err != nil {
		t.Fatalf("Compile Russian: %v", err)
	}
	if product.WireSHA256() == ruProduct.WireSHA256() ||
		product.CatalogSHA256() == ruProduct.CatalogSHA256() {
		t.Fatal("language did not bind request identity")
	}

	commonSymbol := cloneTestInput(input)
	commonSymbol.ReadingTargets[0].Symbol = "Run"
	commonSymbol.ReadingTargets[0].Fact = "Run the service to inspect startup behavior."
	commonSymbol.Documents[0].Claim = "Run the service through its documented interface."
	commonProduct, err := Compile(commonSymbol)
	if err != nil {
		t.Fatalf("common source symbol collided with ordinary prose: %v", err)
	}
	if !strings.Contains(string(commonProduct.WireJSON()), "Run the service") ||
		strings.Contains(string(commonProduct.WireJSON()), "cmd/server/main.go") {
		t.Fatalf("common-symbol wire safety = %s", commonProduct.WireJSON())
	}
}

func TestCompileRejectsMathematicallyUnanswerableReadingCatalog(t *testing.T) {
	t.Parallel()
	for count := 1; count <= 2; count++ {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			input := cloneTestInput(testInput())
			input.ReadingTargets = input.ReadingTargets[:count]
			ids := make([]string, 0, count)
			for _, target := range input.ReadingTargets {
				ids = append(ids, target.ID)
			}
			slices.Sort(ids)
			input.Architecture.Components[0].ReadingTargetIDs = ids
			_, err := Compile(input)
			if err == nil || !strings.Contains(err.Error(), "at least three reading targets") {
				t.Fatalf("Compile(%d targets) error = %v", count, err)
			}
		})
	}
}

func TestCommonSourceSymbolSurvivesExactArtifactRoundTrip(t *testing.T) {
	t.Parallel()
	input := cloneTestInput(testInput())
	input.ReadingTargets[0].Symbol = "Run"
	input.ReadingTargets[0].Fact = "Run the service to inspect startup behavior."
	input.Documents[0].Claim = "Run the service through its documented interface."

	product, err := Compile(input)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	request, err := product.RequestRecord()
	if err != nil {
		t.Fatalf("RequestRecord: %v", err)
	}
	requestJSON, err := EncodeRequestRecord(request)
	if err != nil {
		t.Fatalf("EncodeRequestRecord: %v", err)
	}
	decodedRequest, err := DecodeRequestRecord(requestJSON)
	if err != nil {
		t.Fatalf("DecodeRequestRecord: %v", err)
	}
	if err := ValidateRequestRecordAgainstInput(decodedRequest, input); err != nil {
		t.Fatalf("ValidateRequestRecordAgainstInput: %v", err)
	}

	response := responseMap(t, validResponse(t, product))
	brief := response["brief"].(map[string]any)
	brief["what_it_is"].(map[string]any)["text"] = "Run the service as its documented interface describes."
	direction := response["directions"].([]any)[0].(map[string]any)
	direction["reading"].([]any)[0].(map[string]any)["what_to_look_for"] = "Run the service and inspect startup behavior."
	result, _, err := product.ResolveResponseJSON(marshalTestJSON(t, response))
	if err != nil {
		t.Fatalf("ResolveResponseJSON: %v", err)
	}
	resultJSON, err := EncodeResultRecord(result)
	if err != nil {
		t.Fatalf("EncodeResultRecord: %v", err)
	}
	decodedResult, err := DecodeResultRecord(resultJSON)
	if err != nil {
		t.Fatalf("DecodeResultRecord: %v", err)
	}
	if err := ValidateResultRecordAgainstInput(decodedResult, input); err != nil {
		t.Fatalf("ValidateResultRecordAgainstInput: %v", err)
	}
}

func TestResolveResponseProducesSupportedBriefExactDirectionsAndCanonicalArtifacts(t *testing.T) {
	product := mustCompileTestProduct(t, testInput())
	response := validResponse(t, product)
	result, diagnostics, err := product.ResolveResponseJSON(response)
	if err != nil {
		t.Fatalf("ResolveResponseJSON: %v", err)
	}
	if diagnostics.DirectionsReceived != 1 || diagnostics.DirectionsAccepted != 1 ||
		diagnostics.DirectionsRejected != 0 || len(diagnostics.Issues) != 0 {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
	if len(result.Directions) != 1 || len(result.Directions[0].Reading) != 3 ||
		len(result.ShapeComponentRefs) != 1 ||
		result.ShapeComponentRefs[0] != (CanonicalRef{Kind: RefComponent, ID: "component-api-canonical"}) {
		t.Fatalf("resolved result = %#v", result)
	}
	if result.Directions[0].Reading[0].Target.ID != "anchor-config-canonical" &&
		result.Directions[0].Reading[0].Target.ID != "anchor-route-canonical" &&
		result.Directions[0].Reading[0].Target.ID != "anchor-start-canonical" {
		t.Fatalf("reading target was not restored: %#v", result.Directions[0].Reading[0])
	}
	if err := product.ValidateResultRecord(result); err != nil {
		t.Fatalf("ValidateResultRecord: %v", err)
	}

	request, err := product.RequestRecord()
	if err != nil {
		t.Fatalf("RequestRecord: %v", err)
	}
	requestJSON, err := EncodeRequestRecord(request)
	if err != nil {
		t.Fatalf("EncodeRequestRecord: %v", err)
	}
	decodedRequest, err := DecodeRequestRecord(requestJSON)
	if err != nil {
		t.Fatalf("DecodeRequestRecord: %v", err)
	}
	if err := ValidateRequestRecordAgainstInput(decodedRequest, testInput()); err != nil {
		t.Fatalf("ValidateRequestRecordAgainstInput: %v", err)
	}
	resultJSON, err := EncodeResultRecord(result)
	if err != nil {
		t.Fatalf("EncodeResultRecord: %v", err)
	}
	decodedResult, err := DecodeResultRecord(resultJSON)
	if err != nil {
		t.Fatalf("DecodeResultRecord: %v", err)
	}
	if err := ValidateResultRecordAgainstInput(decodedResult, testInput()); err != nil {
		t.Fatalf("ValidateResultRecordAgainstInput: %v", err)
	}
	status, err := product.AcceptedStatus(result)
	if err != nil {
		t.Fatalf("AcceptedStatus: %v", err)
	}
	statusJSON, err := EncodeStatus(status)
	if err != nil {
		t.Fatalf("EncodeStatus: %v", err)
	}
	decodedStatus, err := DecodeStatus(statusJSON)
	if err != nil {
		t.Fatalf("DecodeStatus: %v", err)
	}
	if err := ValidateStatusAgainstInput(decodedStatus, testInput()); err != nil {
		t.Fatalf("ValidateStatusAgainstInput: %v", err)
	}
}

func TestResolveResponseFailsClosedForRequiredBriefAndKeepsCatalogIdentityPrivate(t *testing.T) {
	product := mustCompileTestProduct(t, testInput())
	valid := responseMap(t, validResponse(t, product))

	if _, exists := valid["catalog_ref"]; exists {
		t.Fatal("model output DTO contains private catalog_ref")
	}
	if _, exists := valid["version"]; exists {
		t.Fatal("model output DTO contains backend-owned version")
	}
	for _, field := range []string{"catalog_ref", "version"} {
		withPrivateEcho := cloneResponseMap(valid)
		withPrivateEcho[field] = "unexpected"
		if field == "version" {
			withPrivateEcho[field] = float64(1)
		}
		if _, _, err := product.ResolveResponseJSON(marshalTestJSON(t, withPrivateEcho)); err == nil ||
			!strings.Contains(err.Error(), "unknown field") {
			t.Fatalf("private output field %q error = %v", field, err)
		}
	}

	rawCanonical := cloneResponseMap(valid)
	brief := rawCanonical["brief"].(map[string]any)
	statement := brief["what_it_is"].(map[string]any)
	statement["support_refs"] = []any{"component-api-canonical"}
	_, _, err := product.ResolveResponseJSON(marshalTestJSON(t, rawCanonical))
	var reference *ReferenceError
	if !errors.As(err, &reference) || reference.Code != "raw_canonical_ref" {
		t.Fatalf("raw canonical support error = %v", err)
	}

	wrongKind := cloneResponseMap(valid)
	brief = wrongKind["brief"].(map[string]any)
	statement = brief["problem"].(map[string]any)
	statement["support_refs"] = []any{refFor(t, product, RefUnit, "unit-repository-canonical")}
	_, _, err = product.ResolveResponseJSON(marshalTestJSON(t, wrongKind))
	if !errors.As(err, &reference) || reference.Code != "wrong_kind_ref" {
		t.Fatalf("wrong-kind support error = %v", err)
	}

	duplicate := cloneResponseMap(valid)
	brief = duplicate["brief"].(map[string]any)
	statement = brief["main_input"].(map[string]any)
	support := refFor(t, product, RefDocument, "document-purpose-canonical")
	statement["support_refs"] = []any{support, support}
	_, _, err = product.ResolveResponseJSON(marshalTestJSON(t, duplicate))
	if !errors.As(err, &reference) || reference.Code != "duplicate_ref" {
		t.Fatalf("duplicate support error = %v", err)
	}

	runtime := cloneResponseMap(valid)
	brief = runtime["brief"].(map[string]any)
	statement = brief["central_responsibility"].(map[string]any)
	statement["text"] = "The system executes before the handler."
	if _, _, err := product.ResolveResponseJSON(marshalTestJSON(t, runtime)); err == nil ||
		!strings.Contains(err.Error(), "runtime-order") {
		t.Fatalf("runtime-order Brief error = %v", err)
	}

	changed := testInput()
	changed.Architecture.Title = "A different accepted architecture"
	changedProduct := mustCompileTestProduct(t, changed)
	changedResult, _, err := changedProduct.ResolveResponseJSON(validResponse(t, changedProduct))
	if err != nil {
		t.Fatalf("changed Product response: %v", err)
	}
	if err := ValidateResultRecordAgainstInput(changedResult, testInput()); err == nil {
		t.Fatal("private catalog-bound result validated against a different exact input")
	}
}

func TestResolveResponseDropsOnlyInvalidDirectionItemsWithBoundedDiagnostics(t *testing.T) {
	product := mustCompileTestProduct(t, testInput())
	valid := responseMap(t, validResponse(t, product))
	directions := valid["directions"].([]any)
	bad := cloneMap(directions[0].(map[string]any))
	bad["principal_refs"] = []any{refFor(t, product, RefDocument, "document-purpose-canonical")}
	valid["directions"] = append([]any{bad}, directions...)
	result, diagnostics, err := product.ResolveResponseJSON(marshalTestJSON(t, valid))
	if err != nil {
		t.Fatalf("ResolveResponseJSON: %v", err)
	}
	if len(result.Directions) != 1 || diagnostics.DirectionsReceived != 2 ||
		diagnostics.DirectionsAccepted != 1 || diagnostics.DirectionsRejected != 1 ||
		len(diagnostics.Issues) != 1 || diagnostics.Issues[0].Code != "wrong_kind_principal_ref" {
		t.Fatalf("item-local diagnostics = %#v", diagnostics)
	}

	overflow := cloneResponseMap(responseMap(t, validResponse(t, product)))
	base := overflow["directions"].([]any)[0]
	items := make([]any, 15)
	for index := range items {
		item := cloneMap(base.(map[string]any))
		item["question"] = "Where should a reader inspect area number " + string(rune('A'+index)) + "?"
		items[index] = item
	}
	items[0].(map[string]any)["principal_refs"] = []any{
		refFor(t, product, RefDocument, "document-purpose-canonical"),
	}
	overflow["directions"] = items
	result, diagnostics, err = product.ResolveResponseJSON(marshalTestJSON(t, overflow))
	if err != nil {
		t.Fatalf("ResolveResponseJSON overflow: %v", err)
	}
	if len(result.Directions) != MaxDirections-1 || diagnostics.DirectionsReceived != 15 ||
		diagnostics.DirectionsRejected != 4 || len(diagnostics.Issues) != 4 ||
		diagnostics.Issues[0].Position != 0 || diagnostics.Issues[1].Position != MaxDirections ||
		diagnostics.Issues[1].Code != "unrequested_output" {
		t.Fatalf("overflow diagnostics = %#v", diagnostics)
	}
}

func TestCompileAndDecodeEnforceTerminalResourceAndCanonicalArtifacts(t *testing.T) {
	input := testInput()
	input.Limits.MaxWireBytes = 32
	_, err := Compile(input)
	var resource *ResourceLimitError
	if !errors.As(err, &resource) || resource.Section != "wire_bytes" {
		t.Fatalf("wire resource error = %v", err)
	}

	product := mustCompileTestProduct(t, testInput())
	request, err := product.RequestRecord()
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := EncodeRequestRecord(request)
	if err != nil {
		t.Fatal(err)
	}
	var generic map[string]any
	if err := json.Unmarshal(encoded, &generic); err != nil {
		t.Fatal(err)
	}
	generic["unexpected"] = true
	if _, err := DecodeRequestRecord(marshalTestJSON(t, generic)); err == nil ||
		!strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unknown field error = %v", err)
	}
	compact, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeRequestRecord(compact); err == nil || !strings.Contains(err.Error(), "not canonical") {
		t.Fatalf("noncanonical artifact error = %v", err)
	}
}

func TestStatusStateMatrixIsClosedAndBoundToExactInput(t *testing.T) {
	product := mustCompileTestProduct(t, testInput())
	statuses := []Status{product.PreparedStatus()}
	offline, err := product.UnavailableStatus(UnavailableOffline)
	if err != nil {
		t.Fatal(err)
	}
	statuses = append(statuses, offline)
	failed, err := product.FailureStatus(FailureReference)
	if err != nil {
		t.Fatal(err)
	}
	statuses = append(statuses, failed)
	for _, status := range statuses {
		if err := product.ValidateStatus(status); err != nil {
			t.Fatalf("ValidateStatus(%s): %v", status.State, err)
		}
	}
	if _, err := product.UnavailableStatus("network_maybe"); err == nil {
		t.Fatal("accepted open unavailable status")
	}
	if _, err := product.FailureStatus("mystery"); err == nil {
		t.Fatal("accepted open failure status")
	}
	changed := testInput()
	changed.Architecture.Title = "Different accepted architecture"
	if err := ValidateStatusAgainstInput(product.PreparedStatus(), changed); err == nil {
		t.Fatal("status accepted against different Architecture")
	}
}

func testInput() Input {
	atlas := repositoryatlas.Atlas{
		Version: repositoryatlas.Version,
		Units: []repositoryatlas.Unit{
			{ID: "unit-repository-canonical", Kind: repositoryatlas.UnitRepository, Name: "Example repository"},
			{ID: "unit-module-canonical", Kind: repositoryatlas.UnitModule, ParentID: "unit-repository-canonical", Name: "Example module"},
			{ID: "unit-app-canonical", Kind: repositoryatlas.UnitApp, ParentID: "unit-module-canonical", Name: "Server application"},
		},
		Entities: []repositoryatlas.Entity{
			{ID: "surface-start-canonical", Kind: repositoryatlas.EntitySurface, UnitID: "unit-app-canonical"},
			{ID: "operation-start-canonical", Kind: repositoryatlas.EntityOperation, UnitID: "unit-app-canonical"},
		},
		Evidence: []repositoryatlas.Evidence{{
			ID: "evidence-start-canonical", UnitID: "unit-app-canonical",
			Location: evidence.Location{Path: "cmd/server/main.go", Line: 20}, Symbol: "RunServer",
			Provenance: evidence.Provenance{Provider: "fixture", Operation: "observe_start"},
		}},
		Observations: []repositoryatlas.Observation{{
			ID: "observation-start-canonical", UnitID: "unit-app-canonical",
			Subject:      repositoryatlas.EntityRef{Kind: repositoryatlas.EntitySurface, ID: "surface-start-canonical"},
			EvidenceRefs: []string{"evidence-start-canonical"},
		}},
		Relations: []repositoryatlas.Relation{{
			ID: "relation-start-canonical", UnitID: "unit-app-canonical",
			Kind: repositoryatlas.RelationExposes, Phase: repositoryatlas.PhaseStartup,
			Authority:    repositoryatlas.AuthorityResolved,
			Source:       repositoryatlas.EntityRef{Kind: repositoryatlas.EntitySurface, ID: "surface-start-canonical"},
			Target:       repositoryatlas.EntityRef{Kind: repositoryatlas.EntityOperation, ID: "operation-start-canonical"},
			EvidenceRefs: []string{"evidence-start-canonical"},
		}},
	}
	return Input{
		Atlas: atlas,
		Architecture: ArchitectureInput{
			Version: 5, Source: "normalized_model", Title: "Server anatomy",
			Subtitle: "Accepted conceptual grouping",
			Subsystems: []Subsystem{{
				ID: "subsystem-core-canonical", Name: "Core server", Description: "Owns request setup.",
				Authority:    repositoryatlas.AuthorityResolved,
				ComponentIDs: []string{"component-api-canonical", "component-config-canonical"},
			}},
			Components: []Component{
				{ID: "component-api-canonical", SubsystemID: "subsystem-core-canonical", Name: "API server", Description: "Accepts requests.", Authority: repositoryatlas.AuthorityResolved, ReadingTargetIDs: []string{"anchor-config-canonical", "anchor-route-canonical", "anchor-start-canonical"}},
				{ID: "component-config-canonical", SubsystemID: "subsystem-core-canonical", Name: "Configuration", Description: "Provides settings.", Authority: repositoryatlas.AuthorityResolved},
			},
		},
		Language: LanguageEnglish,
		Surfaces: []Surface{{
			ID: "surface-start-canonical", UnitID: "unit-app-canonical", Name: "Server process entry",
			Kind: "process_entry", Authority: repositoryatlas.AuthorityResolved,
		}},
		ReadingTargets: []ReadingTarget{
			{ID: "anchor-start-canonical", Owner: CanonicalRef{Kind: RefComponent, ID: "component-api-canonical"}, Kind: ReadingTargetEntrypoint, Label: "Server startup", Fact: "Initializes the application shell.", Authority: repositoryatlas.AuthorityObserved, Location: evidence.Location{Path: "cmd/server/main.go", Line: 20}, Symbol: "RunServer"},
			{ID: "anchor-config-canonical", Owner: CanonicalRef{Kind: RefComponent, ID: "component-api-canonical"}, Kind: ReadingTargetFunction, Label: "Configuration load", Fact: "Loads bounded application settings.", Authority: repositoryatlas.AuthorityObserved, Location: evidence.Location{Path: "internal/config/load.go", Line: 14}, Symbol: "Load"},
			{ID: "anchor-route-canonical", Owner: CanonicalRef{Kind: RefComponent, ID: "component-api-canonical"}, Kind: ReadingTargetFunction, Label: "Route registration", Fact: "Registers the public request surface.", Authority: repositoryatlas.AuthorityObserved, Location: evidence.Location{Path: "internal/server/routes.go", Line: 31}, Symbol: "RegisterRoutes"},
		},
		Evidence: []EvidenceFact{{
			ID:          "evidence-start-canonical",
			SubjectRefs: []CanonicalRef{{Kind: RefSurface, ID: "surface-start-canonical"}},
			Authority:   repositoryatlas.AuthorityResolved, Fact: "The application exposes a startup surface.",
		}},
		Documents: []DocumentClaim{{
			ID: "document-purpose-canonical", Label: "Documented purpose",
			Claim:     "The project provides a server for identity workflows.",
			Authority: repositoryatlas.AuthorityObserved,
		}},
		Limits: DefaultLimits(),
	}
}

func validResponse(t *testing.T, product Product) []byte {
	t.Helper()
	component := refFor(t, product, RefComponent, "component-api-canonical")
	surface := refFor(t, product, RefSurface, "surface-start-canonical")
	document := refFor(t, product, RefDocument, "document-purpose-canonical")
	evidenceRef := refFor(t, product, RefEvidence, "evidence-start-canonical")
	targets := []string{
		refFor(t, product, RefReadingTarget, "anchor-config-canonical"),
		refFor(t, product, RefReadingTarget, "anchor-route-canonical"),
		refFor(t, product, RefReadingTarget, "anchor-start-canonical"),
	}
	return marshalTestJSON(t, map[string]any{
		"repository_type": string(RepositoryService),
		"brief": map[string]any{
			"what_it_is":             map[string]any{"text": "A server for identity workflows.", "support_refs": []string{document}},
			"problem":                map[string]any{"text": "It centralizes identity-facing request handling.", "support_refs": []string{component}},
			"main_input":             map[string]any{"text": "Configured requests enter through an application surface.", "support_refs": []string{surface}},
			"central_responsibility": map[string]any{"text": "The API component owns request setup.", "support_refs": []string{component}},
			"observable_result":      map[string]any{"text": "A startup surface becomes available for requests.", "support_refs": []string{evidenceRef}},
			"domain_terms":           []any{map[string]any{"term": "identity workflow", "meaning": "A bounded user-facing identity operation.", "support_refs": []string{document}}},
		},
		"directions": []any{map[string]any{
			"question":         "Where should a reader begin exploring the server?",
			"why_it_matters":   "This route connects the accepted component to exact reading targets.",
			"learning_outcome": "The reader can identify the configuration and request setup seams.",
			"target_job":       string(JobFirstContact), "learning_stage": string(StageOrientation),
			"principal_refs": []string{component},
			"reading": []any{
				map[string]any{"target_ref": targets[0], "label": string(ReadingStart), "what_to_look_for": "Inspect how settings enter the component."},
				map[string]any{"target_ref": targets[1], "label": string(ReadingConnect), "what_to_look_for": "Inspect how request handlers are registered."},
				map[string]any{"target_ref": targets[2], "label": string(ReadingVerify), "what_to_look_for": "Confirm the application startup boundary."},
			},
		}},
	})
}

func mustCompileTestProduct(t *testing.T, input Input) Product {
	t.Helper()
	product, err := Compile(input)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	return product
}

func refFor(t *testing.T, product Product, kind RefKind, id string) string {
	t.Helper()
	return catalogObject(t, product.Catalog(), kind, id).Ref
}

func catalogObject(t *testing.T, catalog []CatalogObject, kind RefKind, id string) CatalogObject {
	t.Helper()
	for _, object := range catalog {
		if object.Kind == kind && object.CanonicalID == id {
			return object
		}
	}
	t.Fatalf("catalog object %s/%s not found", kind, id)
	return CatalogObject{}
}

func responseMap(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	return normalizeJSONMap(result)
}

func normalizeJSONMap(value map[string]any) map[string]any {
	for key, item := range value {
		switch typed := item.(type) {
		case map[string]any:
			value[key] = normalizeJSONMap(typed)
		case []any:
			for index, child := range typed {
				if object, ok := child.(map[string]any); ok {
					typed[index] = normalizeJSONMap(object)
				}
			}
		}
	}
	return value
}

func cloneResponseMap(value map[string]any) map[string]any {
	encoded, _ := json.Marshal(value)
	var result map[string]any
	_ = json.Unmarshal(encoded, &result)
	return normalizeJSONMap(result)
}

func cloneMap(value map[string]any) map[string]any { return cloneResponseMap(value) }

func marshalTestJSON(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func cloneTestInput(value Input) Input {
	encoded, _ := json.Marshal(value)
	var result Input
	_ = json.Unmarshal(encoded, &result)
	return result
}
