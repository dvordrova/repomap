package atlasstudy

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
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
		"anchor-start-canonical",
	} {
		if strings.Contains(wire, hidden) {
			t.Fatalf("provider wire exposed %q: %s", hidden, wire)
		}
	}
	for _, visible := range []string{
		`"ref":"u1"`, `"ref":"ss1"`, `"ref":"c1"`, `"ref":"sf1"`,
		`"ref":"a1"`, `"ref":"e1"`, `"ref":"d1"`,
		`"authority":"resolved"`, `"language":"en"`,
		`"allowed_paths":["cmd/server/main.go","internal/config/load.go","internal/server/routes.go"]`,
		`"path":"cmd/server/main.go","line":20`,
	} {
		if !strings.Contains(wire, visible) {
			t.Fatalf("provider wire missing %q: %s", visible, wire)
		}
	}
	if want := fmt.Sprintf("Return 1-%d directions", MaxDirections); !strings.Contains(product.BuildPrompt().System, want) {
		t.Fatalf("provider prompt does not use the production route bound %q", want)
	}
	if want := fmt.Sprintf(
		"%d-%d distinct reading items",
		MinDirectionReadingCount, MaxDirectionReadingCount,
	); !strings.Contains(product.BuildPrompt().System, want) {
		t.Fatalf("provider prompt does not use the task-sized reading bound %q", want)
	}
	for _, exactRule := range []string{
		"every such path is repeated in allowed_paths",
		"Identity fields return only short refs",
		"never copy a short ref into prose",
		"component c* or surface sf* ref",
		"Never use unit u*, subsystem ss*, reading-target a*, evidence e*, or document d* refs as direction principals",
		"Every reading target_ref must be an a* reading_target ref",
	} {
		if !strings.Contains(product.BuildPrompt().System, exactRule) {
			t.Fatalf("provider prompt is missing closed typed-ref rule %q", exactRule)
		}
	}
	if prompt := product.BuildPrompt(); prompt.Language != LanguageEnglish ||
		!strings.Contains(prompt.User, "Requested prose language: en.") {
		t.Fatalf("English provider prompt language = %#v", prompt)
	}
	catalog := product.Catalog()
	target := catalogObject(t, catalog, RefReadingTarget, "anchor-start-canonical")
	if target.Location == nil || target.Location.Path != "cmd/server/main.go" || target.Symbol != "RunServer" {
		t.Fatalf("private target locator = %#v", target)
	}
	if strings.Contains(wire, `"symbol":"RunServer"`) {
		t.Fatalf("ambiguous bare symbol leaked into provider locator context: %s", wire)
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
	if prompt := ruProduct.BuildPrompt(); prompt.Language != LanguageRussian ||
		!strings.Contains(prompt.User, "Requested prose language: ru.") {
		t.Fatalf("Russian provider prompt language = %#v", prompt)
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
		!strings.Contains(string(commonProduct.WireJSON()), `"path":"cmd/server/main.go"`) {
		t.Fatalf("common-symbol wire safety = %s", commonProduct.WireJSON())
	}
}

func TestCompileUsesCompleteReadingLocatorIdentity(t *testing.T) {
	t.Parallel()

	duplicateInput := func() Input {
		input := cloneTestInput(testInput())
		location := evidence.Location{
			Path: "internal/shared/run.go", Line: 20, Column: 3,
			EndLine: 20, EndColumn: 9,
		}
		for _, index := range []int{0, 1} {
			input.ReadingTargets[index].Kind = ReadingTargetFunction
			input.ReadingTargets[index].Location = location
			input.ReadingTargets[index].Symbol = "Run"
		}
		return input
	}

	if _, err := Compile(duplicateInput()); err == nil ||
		!strings.Contains(err.Error(), "duplicate one exact locator") {
		t.Fatalf("complete duplicate locator error = %v", err)
	}

	tests := []struct {
		name   string
		change func(*ReadingTarget)
	}{
		{name: "path", change: func(target *ReadingTarget) { target.Location.Path = "internal/shared/other.go" }},
		{name: "line", change: func(target *ReadingTarget) { target.Location.Line = 19 }},
		{name: "column", change: func(target *ReadingTarget) { target.Location.Column = 4 }},
		{name: "end line", change: func(target *ReadingTarget) { target.Location.EndLine = 21 }},
		{name: "end column", change: func(target *ReadingTarget) { target.Location.EndColumn = 10 }},
		{name: "symbol", change: func(target *ReadingTarget) { target.Symbol = "RunOther" }},
		{name: "kind", change: func(target *ReadingTarget) { target.Kind = ReadingTargetMethod }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			input := duplicateInput()
			test.change(&input.ReadingTargets[1])
			if _, err := Compile(input); err != nil {
				t.Fatalf("distinct %s locator rejected: %v", test.name, err)
			}
		})
	}
}

func TestCompileRejectsMalformedModelVisibleReadingSymbols(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		symbol string
	}{
		{name: "invalid utf8", symbol: string([]byte{0xff, '.', 'R'})},
		{name: "line feed", symbol: "example.com/server.\nRun"},
		{name: "carriage return", symbol: "example.com/server.\rRun"},
		{name: "over bound", symbol: strings.Repeat("x", DefaultLimits().MaxTextBytes+1) + ".Run"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			input := cloneTestInput(testInput())
			input.ReadingTargets[0].Symbol = test.symbol
			if _, err := Compile(input); err == nil || !strings.Contains(err.Error(), "reading target symbol") {
				t.Fatalf("malformed model-visible symbol error = %v", err)
			}
		})
	}
}

func TestRepositoryLocationRejectsNegativeAndInconsistentCoordinates(t *testing.T) {
	t.Parallel()

	valid := []evidence.Location{
		{Path: "service.go", Line: 10},
		{Path: "service.go", Line: 10, Column: 3},
		{Path: "service.go", Line: 10, Column: 3, EndLine: 10, EndColumn: 8},
		{Path: "service.go", Line: 10, Column: 3, EndLine: 12},
	}
	for _, location := range valid {
		if !repositoryLocation(location) {
			t.Errorf("valid repository location rejected: %#v", location)
		}
	}

	invalid := []evidence.Location{
		{Path: "service.go", Line: -1},
		{Path: "service.go", Line: 10, Column: -1},
		{Path: "service.go", Line: 10, EndLine: -1},
		{Path: "service.go", Line: 10, EndColumn: -1},
		{Path: "service.go", Line: 10, EndColumn: 4},
		{Path: "service.go", Line: 10, EndLine: 9},
		{Path: "service.go", Line: 10, Column: 8, EndLine: 10, EndColumn: 3},
	}
	for _, location := range invalid {
		if repositoryLocation(location) {
			t.Errorf("invalid repository location accepted: %#v", location)
		}
	}
}

func TestCompilePublishesDistinctBriefSupportChoicesWithoutUnitRefs(t *testing.T) {
	t.Parallel()
	product := mustCompileTestProduct(t, testInput())
	var wire wireProjection
	if err := json.Unmarshal(product.WireJSON(), &wire); err != nil {
		t.Fatal(err)
	}
	if len(wire.BriefSupportChoices) == 0 {
		t.Fatal("provider wire has no explicit Brief support choices")
	}
	wantChoices := make(map[string]RefKind)
	for _, object := range product.Catalog() {
		if briefSupportKind(object.Kind) {
			wantChoices[object.Ref] = object.Kind
		}
	}
	if len(wire.BriefSupportChoices) != len(wantChoices) {
		t.Fatalf("provider-visible Brief support choices = %d, want complete allowlist of %d", len(wire.BriefSupportChoices), len(wantChoices))
	}
	wantKinds := map[RefKind]bool{
		RefSubsystem: false, RefComponent: false, RefSurface: false,
		RefReadingTarget: false, RefEvidence: false, RefDocument: false,
	}
	seen := make(map[string]struct{}, len(wire.BriefSupportChoices))
	for _, choice := range wire.BriefSupportChoices {
		if choice.Ref == "" || !briefSupportKind(choice.Kind) || choice.Kind == RefUnit {
			t.Fatalf("invalid provider-visible Brief support choice: %#v", choice)
		}
		if _, duplicate := seen[choice.Ref]; duplicate {
			t.Fatalf("duplicate provider-visible Brief support ref %q", choice.Ref)
		}
		seen[choice.Ref] = struct{}{}
		if wantKind, found := wantChoices[choice.Ref]; !found || wantKind != choice.Kind {
			t.Fatalf("provider-visible Brief support choice is not exact catalog allowlist: %#v", choice)
		}
		wantKinds[choice.Kind] = true
	}
	for kind, found := range wantKinds {
		if !found {
			t.Fatalf("provider-visible Brief support choices omit %s", kind)
		}
	}
	for _, unit := range wire.Units {
		if _, selectable := seen[unit.Ref]; selectable {
			t.Fatalf("unit ref %q is selectable as Brief support", unit.Ref)
		}
	}
	prompt := product.BuildPrompt()
	if !strings.Contains(prompt.System, "selected only from brief_support_choices") ||
		!strings.Contains(prompt.System, "including every unit ref") ||
		!strings.Contains(prompt.System, fmt.Sprintf("0-%d optional domain_terms", MaxDomainTerms)) ||
		!strings.Contains(prompt.System, "Terms beyond that explicit count are unrequested output") {
		t.Fatalf("Brief support prompt contract is not structurally explicit: %q", prompt.System)
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

func TestResolveBriefAcceptsCompleteUniqueSupportSetWithoutMagicCountCap(t *testing.T) {
	product := mustCompileTestProduct(t, testInput())
	var support []string
	for _, object := range product.Catalog() {
		if briefSupportKind(object.Kind) {
			support = append(support, object.Ref)
		}
	}
	if len(support) < 9 {
		t.Fatalf("fixture support catalog = %d, want at least 9", len(support))
	}
	response := responseMap(t, validResponse(t, product))
	brief := response["brief"].(map[string]any)
	brief["what_it_is"].(map[string]any)["support_refs"] = support
	result, _, err := product.ResolveResponseJSON(marshalTestJSON(t, response))
	if err != nil {
		t.Fatalf("complete support set: %v", err)
	}
	if got := len(result.Brief.WhatItIs.SupportRefs); got != len(support) {
		t.Fatalf("resolved support = %d, want %d", got, len(support))
	}
}

func TestResolveResponseAllowsOnlyScopedExactReadingLocatorEchoes(t *testing.T) {
	input := testInput()
	for index := range input.ReadingTargets {
		if input.ReadingTargets[index].ID == "anchor-config-canonical" {
			input.ReadingTargets[index].Symbol = "example.com/config.Load"
		}
	}
	product := mustCompileTestProduct(t, input)
	response := responseMap(t, validResponse(t, product))
	direction := response["directions"].([]any)[0].(map[string]any)
	direction["learning_outcome"] = "The reader can inspect internal/config/load.go as the configuration seam."
	reading := direction["reading"].([]any)
	reading[0].(map[string]any)["what_to_look_for"] =
		"Inspect example.com/config.Load at internal/config/load.go."
	if _, _, err := product.ResolveResponseJSON(marshalTestJSON(t, response)); err != nil {
		t.Fatalf("scoped exact locator echo: %v", err)
	}

	shortRefCopy := cloneResponseMap(response)
	direction = shortRefCopy["directions"].([]any)[0].(map[string]any)
	direction["learning_outcome"] = "The reader can map this route to c1."
	_, diagnostics, err := product.ResolveResponseJSON(marshalTestJSON(t, shortRefCopy))
	if err == nil || diagnostics.DirectionsAccepted != 0 || len(diagnostics.Issues) != 1 ||
		diagnostics.Issues[0].Code != IssueInvalidOutcome {
		t.Fatalf("request-local ref leaked into direction prose = %v / %#v", err, diagnostics)
	}

	briefScoped := responseMap(t, validResponse(t, product))
	brief := briefScoped["brief"].(map[string]any)
	target := refFor(t, product, RefReadingTarget, "anchor-config-canonical")
	statement := brief["what_it_is"].(map[string]any)
	statement["text"] = "Read internal/config/load.go and example.com/config.Load to understand configuration."
	statement["support_refs"] = []string{target}
	term := brief["domain_terms"].([]any)[0].(map[string]any)
	term["meaning"] = "Configuration loaded by example.com/config.Load in internal/config/load.go."
	term["support_refs"] = []string{target}
	result, _, err := product.ResolveResponseJSON(marshalTestJSON(t, briefScoped))
	if err != nil {
		t.Fatalf("support-scoped Brief locator echo: %v", err)
	}
	encoded, err := EncodeResultRecord(result)
	if err != nil {
		t.Fatalf("encode support-scoped result: %v", err)
	}
	if _, err := DecodeResultRecord(encoded); err != nil {
		t.Fatalf("standalone support-scoped result: %v", err)
	}

	unsupportedBrief := cloneResponseMap(briefScoped)
	brief = unsupportedBrief["brief"].(map[string]any)
	statement = brief["what_it_is"].(map[string]any)
	statement["support_refs"] = []string{refFor(t, product, RefDocument, "document-purpose-canonical")}
	if _, _, err := product.ResolveResponseJSON(marshalTestJSON(t, unsupportedBrief)); err == nil ||
		!strings.Contains(err.Error(), "canonical identity or source locator") {
		t.Fatalf("Brief locator without matching reading support = %v", err)
	}

	foreignBrief := cloneResponseMap(briefScoped)
	brief = foreignBrief["brief"].(map[string]any)
	statement = brief["what_it_is"].(map[string]any)
	statement["text"] = "Read internal/server/routes.go to understand configuration."
	if _, _, err := product.ResolveResponseJSON(marshalTestJSON(t, foreignBrief)); err == nil ||
		!strings.Contains(err.Error(), "canonical identity or source locator") {
		t.Fatalf("Brief locator from unsupported reading target = %v", err)
	}

	locatorTerm := cloneResponseMap(briefScoped)
	brief = locatorTerm["brief"].(map[string]any)
	term = brief["domain_terms"].([]any)[0].(map[string]any)
	term["term"] = "internal/config/load.go"
	result, diagnostics, err = product.ResolveResponseJSON(marshalTestJSON(t, locatorTerm))
	if err != nil || len(result.Brief.DomainTerms) != 0 ||
		diagnostics.DomainTermsReceived != 1 || diagnostics.DomainTermsAccepted != 0 ||
		diagnostics.DomainTermsRejected != 1 ||
		!reflect.DeepEqual(diagnostics.DomainTermIssues, []DomainTermIssue{{
			Position: 0, Code: DomainTermIssueInvalidTerm,
		}}) {
		t.Fatalf("domain-term name locator echo = %v / %#v", err, diagnostics)
	}

	wrongTarget := cloneResponseMap(response)
	direction = wrongTarget["directions"].([]any)[0].(map[string]any)
	reading = direction["reading"].([]any)
	reading[0].(map[string]any)["what_to_look_for"] =
		"Inspect internal/server/routes.go instead."
	_, diagnostics, err = product.ResolveResponseJSON(marshalTestJSON(t, wrongTarget))
	if err == nil || diagnostics.DirectionsAccepted != 0 || len(diagnostics.Issues) != 1 ||
		diagnostics.Issues[0].Code != IssueInvalidReadingCopy {
		t.Fatalf("cross-target locator echo = %v / %#v", err, diagnostics)
	}

	collision := testInput()
	collision.ReadingTargets[0].Location.Path = "component-api-canonical"
	if _, err := Compile(collision); err == nil ||
		!strings.Contains(err.Error(), "canonical identity or source locator") {
		t.Fatalf("locator/canonical identity collision = %v", err)
	}
}

func TestResolveResponseDropsInvalidOptionalDomainTermsAndPreservesSiblings(t *testing.T) {
	product := mustCompileTestProduct(t, testInput())
	response := responseMap(t, validResponse(t, product))
	brief := response["brief"].(map[string]any)
	document := refFor(t, product, RefDocument, "document-purpose-canonical")
	configTarget := refFor(t, product, RefReadingTarget, "anchor-config-canonical")
	brief["domain_terms"] = []any{
		map[string]any{
			"term": "identity workflow", "meaning": "A bounded user-facing identity operation.",
			"support_refs": []string{document},
		},
		map[string]any{
			"term": "unknown support", "meaning": "This optional item cites no advertised support.",
			"support_refs": []string{"a999"},
		},
		map[string]any{
			"term": "unsupported locator", "meaning": "Configuration is loaded in internal/config/load.go.",
			"support_refs": []string{document},
		},
		map[string]any{
			"term": "unknown field", "meaning": "This optional item has a malformed shape.",
			"support_refs": []string{document}, "unexpected": true,
		},
		map[string]any{
			"term": "configuration seam", "meaning": "Configuration is loaded in internal/config/load.go.",
			"support_refs": []string{configTarget},
		},
	}

	result, diagnostics, err := product.ResolveResponseJSON(marshalTestJSON(t, response))
	if err != nil {
		t.Fatalf("ResolveResponseJSON: %v", err)
	}
	if len(result.Directions) != 1 || result.Brief.WhatItIs.Text == "" ||
		len(result.Brief.DomainTerms) != 2 ||
		result.Brief.DomainTerms[0].Term != "identity workflow" ||
		result.Brief.DomainTerms[1].Term != "configuration seam" {
		t.Fatalf("valid Brief siblings were not preserved: %#v", result)
	}
	wantIssues := []DomainTermIssue{
		{Position: 1, Code: DomainTermIssueInvalidSupport},
		{Position: 2, Code: DomainTermIssueInvalidMeaning},
		{Position: 3, Code: DomainTermIssueDecodeCandidate},
	}
	if diagnostics.DomainTermsReceived != 5 || diagnostics.DomainTermsAccepted != 2 ||
		diagnostics.DomainTermsRejected != 3 ||
		!reflect.DeepEqual(diagnostics.DomainTermIssues, wantIssues) {
		t.Fatalf("domain-term diagnostics = %#v, want issues %#v", diagnostics, wantIssues)
	}
	encoded, err := EncodeResultRecord(result)
	if err != nil {
		t.Fatalf("EncodeResultRecord: %v", err)
	}
	decoded, err := DecodeResultRecord(encoded)
	if err != nil {
		t.Fatalf("DecodeResultRecord: %v", err)
	}
	if err := product.ValidateResultRecord(decoded); err != nil {
		t.Fatalf("ValidateResultRecord: %v", err)
	}
}

func TestResolveResponseBoundsUnrequestedDomainTermDiagnostics(t *testing.T) {
	product := mustCompileTestProduct(t, testInput())
	response := responseMap(t, validResponse(t, product))
	brief := response["brief"].(map[string]any)
	document := refFor(t, product, RefDocument, "document-purpose-canonical")
	count := MaxDomainTerms + MaxDomainTermDiagnostics + 3
	terms := make([]any, 0, count)
	for index := 0; index < count; index++ {
		terms = append(terms, map[string]any{
			"term":         fmt.Sprintf("term %02d", index),
			"meaning":      "A bounded optional repository term.",
			"support_refs": []string{document},
		})
	}
	brief["domain_terms"] = terms

	result, diagnostics, err := product.ResolveResponseJSON(marshalTestJSON(t, response))
	if err != nil {
		t.Fatalf("ResolveResponseJSON: %v", err)
	}
	if len(result.Brief.DomainTerms) != MaxDomainTerms ||
		diagnostics.DomainTermsReceived != count ||
		diagnostics.DomainTermsAccepted != MaxDomainTerms ||
		diagnostics.DomainTermsRejected != count-MaxDomainTerms ||
		len(diagnostics.DomainTermIssues) != MaxDomainTermDiagnostics ||
		diagnostics.DomainTermIssues[0] != (DomainTermIssue{
			Position: MaxDomainTerms, Code: DomainTermIssueUnrequestedOutput,
		}) || diagnostics.DomainTermIssues[MaxDomainTermDiagnostics-1] != (DomainTermIssue{
		Position: MaxDomainTerms + MaxDomainTermDiagnostics - 1,
		Code:     DomainTermIssueUnrequestedOutput,
	}) {
		t.Fatalf("bounded domain-term diagnostics = %#v", diagnostics)
	}
	if err := product.ValidateResultRecord(result); err != nil {
		t.Fatalf("ValidateResultRecord: %v", err)
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
		} else {
			var decodeErr *ResponseDecodeError
			if !errors.As(err, &decodeErr) {
				t.Fatalf("private output field %q was not classified as decode: %v", field, err)
			}
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

	shortRefProse := cloneResponseMap(valid)
	brief = shortRefProse["brief"].(map[string]any)
	statement = brief["what_it_is"].(map[string]any)
	statement["text"] = "The service begins at c1."
	if _, _, err := product.ResolveResponseJSON(marshalTestJSON(t, shortRefProse)); err == nil ||
		!strings.Contains(err.Error(), "canonical identity or source locator") {
		t.Fatalf("request-local ref leaked into Brief prose: %v", err)
	}

	locatorEcho := cloneResponseMap(valid)
	brief = locatorEcho["brief"].(map[string]any)
	statement = brief["what_it_is"].(map[string]any)
	statement["text"] = "Start by reading cmd/server/main.go before continuing."
	if _, _, err := product.ResolveResponseJSON(marshalTestJSON(t, locatorEcho)); err == nil ||
		!strings.Contains(err.Error(), "canonical identity or source locator") {
		t.Fatalf("model echoed an input-only source locator: %v", err)
	}
	targetOnlyInput := cloneTestInput(testInput())
	targetOnlyInput.ReadingTargets[0].Location.Path = "internal/target-only/start.go"
	targetOnlyProduct := mustCompileTestProduct(t, targetOnlyInput)
	targetOnlyEcho := responseMap(t, validResponse(t, targetOnlyProduct))
	brief = targetOnlyEcho["brief"].(map[string]any)
	statement = brief["what_it_is"].(map[string]any)
	statement["text"] = "Start by reading internal/target-only/start.go before continuing."
	if _, _, err := targetOnlyProduct.ResolveResponseJSON(marshalTestJSON(t, targetOnlyEcho)); err == nil ||
		!strings.Contains(err.Error(), "canonical identity or source locator") {
		t.Fatalf("model echoed a target-only source locator: %v", err)
	}
	qualifiedInput := testInput()
	qualifiedInput.ReadingTargets[0].Symbol = "example.com/server.RunServer"
	qualifiedProduct := mustCompileTestProduct(t, qualifiedInput)
	qualifiedEcho := responseMap(t, validResponse(t, qualifiedProduct))
	brief = qualifiedEcho["brief"].(map[string]any)
	statement = brief["what_it_is"].(map[string]any)
	statement["text"] = "Inspect example.com/server.RunServer before continuing."
	if _, _, err := qualifiedProduct.ResolveResponseJSON(marshalTestJSON(t, qualifiedEcho)); err == nil ||
		!strings.Contains(err.Error(), "canonical identity or source locator") {
		t.Fatalf("model echoed an input-only qualified symbol: %v", err)
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

func TestStudyDirectionReadingBoundsAreItemLocalAndPersisted(t *testing.T) {
	input := studyRouteBoundaryInput()
	product := mustCompileTestProduct(t, input)
	response := responseMap(t, validResponse(t, product))
	base := response["directions"].([]any)[0].(map[string]any)
	targets := studyRouteTargetIDs()

	directions := make([]any, 0, 4)
	for index, count := range []int{0, 1, 5, 6} {
		direction := cloneMap(base)
		direction["question"] = fmt.Sprintf(
			"Which exact target explains bounded route number %d?", index+1,
		)
		direction["reading"] = providerReadingItems(t, product, targets[:count])
		directions = append(directions, direction)
	}
	response["directions"] = directions

	result, diagnostics, err := product.ResolveResponseJSON(marshalTestJSON(t, response))
	if err != nil {
		t.Fatalf("ResolveResponseJSON: %v", err)
	}
	if got := []int{len(result.Directions[0].Reading), len(result.Directions[1].Reading)}; !reflect.DeepEqual(got, []int{1, 5}) {
		t.Fatalf("accepted route reading counts = %v, want [1 5]", got)
	}
	if diagnostics.DirectionsReceived != 4 || diagnostics.DirectionsAccepted != 2 ||
		diagnostics.DirectionsRejected != 2 || !reflect.DeepEqual(diagnostics.Issues, []DirectionIssue{
		{Position: 0, Code: IssueInvalidReadingCount},
		{Position: 3, Code: IssueInvalidReadingCount},
	}) {
		t.Fatalf("boundary diagnostics = %#v", diagnostics)
	}
	if err := product.ValidateResultRecord(result); err != nil {
		t.Fatalf("persisted 1..5 result: %v", err)
	}

	zero := result
	zero.Directions = cloneDirections(result.Directions)
	zero.Directions[0].Reading = nil
	zero.Directions[0].ID = stableDirectionID(zero.Directions[0])
	if err := product.ValidateResultRecord(zero); err == nil ||
		!strings.Contains(err.Error(), "invalid canonical direction") {
		t.Fatalf("persisted zero-reading route error = %v", err)
	}

	six := result
	six.Directions = cloneDirections(result.Directions)
	six.Directions[1].Reading = append(six.Directions[1].Reading, ResolvedReading{
		Target: CanonicalRef{Kind: RefReadingTarget, ID: targets[5]},
		Label:  ReadingContinue, WhatToLookFor: "Inspect the final bounded target.",
	})
	six.Directions[1].ID = stableDirectionID(six.Directions[1])
	if err := product.ValidateResultRecord(six); err == nil ||
		!strings.Contains(err.Error(), "invalid canonical direction") {
		t.Fatalf("persisted six-reading route error = %v", err)
	}
}

func TestStudyAcceptsCompactCasdoorReadingShapeAndReplaysIt(t *testing.T) {
	input := studyRouteBoundaryInput()
	product := mustCompileTestProduct(t, input)
	response := responseMap(t, validResponse(t, product))
	base := response["directions"].([]any)[0].(map[string]any)
	targets := studyRouteTargetIDs()
	selections := [][]string{
		targets[0:1],
		targets[0:3],
		targets[1:5],
		targets[4:5],
		targets[5:6],
	}
	directions := make([]any, 0, len(selections))
	for index, selection := range selections {
		direction := cloneMap(base)
		direction["question"] = fmt.Sprintf(
			"Which bounded route answers focused task number %d?", index+1,
		)
		direction["reading"] = providerReadingItems(t, product, selection)
		directions = append(directions, direction)
	}
	response["directions"] = directions

	result, diagnostics, err := product.ResolveResponseJSON(marshalTestJSON(t, response))
	if err != nil {
		t.Fatalf("ResolveResponseJSON [1,3,4,1,1]: %v", err)
	}
	counts := make([]int, 0, len(result.Directions))
	for _, direction := range result.Directions {
		counts = append(counts, len(direction.Reading))
	}
	if !reflect.DeepEqual(counts, []int{1, 3, 4, 1, 1}) ||
		diagnostics.DirectionsReceived != 5 || diagnostics.DirectionsAccepted != 5 ||
		diagnostics.DirectionsRejected != 0 || len(diagnostics.Issues) != 0 {
		t.Fatalf("compact Casdoor shape = %v / %#v", counts, diagnostics)
	}

	encoded, err := EncodeResultRecord(result)
	if err != nil {
		t.Fatalf("EncodeResultRecord: %v", err)
	}
	replayed, err := DecodeResultRecord(encoded)
	if err != nil {
		t.Fatalf("DecodeResultRecord: %v", err)
	}
	if err := ValidateResultRecordAgainstInput(replayed, input); err != nil {
		t.Fatalf("provider-free replay: %v", err)
	}
}

func TestStudyV5IdentityRejectsV4Artifacts(t *testing.T) {
	if Version != 5 || PromptVersion != "atlas-study-prompt-v11" ||
		RequestArtifactFilename != "atlas_study_request.v5.json" ||
		ResultArtifactFilename != "atlas_study_result.v5.json" ||
		StatusArtifactFilename != "atlas_study_status.v5.json" {
		t.Fatalf("Study v5 identity is incomplete: %d %q %q %q %q",
			Version, PromptVersion, RequestArtifactFilename,
			ResultArtifactFilename, StatusArtifactFilename)
	}
	product := mustCompileTestProduct(t, testInput())
	request, err := product.RequestRecord()
	if err != nil {
		t.Fatal(err)
	}
	request.Version = 4
	request.PromptVersion = "atlas-study-prompt-v10"
	request.CatalogRef = fmt.Sprintf("atlas-study-v4-%s", request.CatalogSHA256)
	if err := product.ValidateRequestRecord(request); err == nil {
		t.Fatal("Study v4 request replayed under the v5 contract")
	}
}

func TestSavedCasdoor144414ResponseRejectsUnitBriefSupportAndPreservesValidRoutesAfterCorrection(t *testing.T) {
	product := mustCompileTestProduct(t, casdoor144414ShapeInput())
	saved, err := os.ReadFile("testdata/casdoor_20260802_144414_response_shape.json")
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = product.ResolveResponseJSON(saved)
	var reference *ReferenceError
	if !errors.As(err, &reference) || reference.Field != "brief.what_it_is.support_refs" ||
		reference.Position != 0 || reference.Code != "wrong_kind_ref" {
		t.Fatalf("saved 14:44 wrong-kind Brief support error = %#v / %v", reference, err)
	}

	corrected := responseMap(t, saved)
	brief := corrected["brief"].(map[string]any)
	component1 := refFor(t, product, RefComponent, "component-api-canonical")
	component5 := refFor(t, product, RefComponent, "component-extra-05")
	component6 := refFor(t, product, RefComponent, "component-extra-06")
	surface := refFor(t, product, RefSurface, "surface-start-canonical")
	document := refFor(t, product, RefDocument, "document-purpose-canonical")
	brief["what_it_is"].(map[string]any)["support_refs"] = []any{document}
	brief["problem"].(map[string]any)["support_refs"] = []any{document, component1}
	brief["main_input"].(map[string]any)["support_refs"] = []any{surface, component5}
	brief["central_responsibility"].(map[string]any)["support_refs"] = []any{component5, component6}
	brief["observable_result"].(map[string]any)["support_refs"] = []any{document, component5}
	for _, rawTerm := range brief["domain_terms"].([]any) {
		rawTerm.(map[string]any)["support_refs"] = []any{document}
	}

	result, diagnostics, err := product.ResolveResponseJSON(marshalTestJSON(t, corrected))
	if err != nil {
		t.Fatalf("corrected saved 14:44 response: %v", err)
	}
	if diagnostics.DirectionsReceived != 6 || diagnostics.DirectionsAccepted != 3 ||
		diagnostics.DirectionsRejected != 3 || len(result.Directions) != 3 ||
		len(diagnostics.Issues) != 3 {
		t.Fatalf("corrected saved-response result/diagnostics = %d / %#v", len(result.Directions), diagnostics)
	}
	wantIssues := []DirectionIssue{
		{Position: 1, Code: IssueWrongKindPrincipalRef},
		{Position: 3, Code: IssuePrincipalNotAdvertised},
		{Position: 5, Code: IssuePrincipalNotAdvertised},
	}
	if !reflect.DeepEqual(diagnostics.Issues, wantIssues) {
		t.Fatalf("corrected saved-response route diagnostics = %#v, want %#v", diagnostics.Issues, wantIssues)
	}
	for _, statement := range []SupportedStatement{
		result.Brief.WhatItIs, result.Brief.Problem, result.Brief.MainInput,
		result.Brief.CentralResponsibility, result.Brief.ObservableResult,
	} {
		for _, support := range statement.SupportRefs {
			if support.Kind == RefUnit || !briefSupportKind(support.Kind) {
				t.Fatalf("corrected Brief retained disallowed support %#v", support)
			}
		}
	}
}

func TestSavedCasdoor175017ResponseDropsNinthUnrequestedTermAndDiagnosesRoutesIndependently(t *testing.T) {
	product := mustCompileTestProduct(t, casdoor175017ResponseInput())
	saved, err := os.ReadFile("testdata/casdoor_20260802_175017_response.json")
	if err != nil {
		t.Fatal(err)
	}
	if got := digest([]byte(strings.TrimSuffix(string(saved), "\n"))); got != "446f1110993403b41f2592054500242616bec8433a4a21569caa12d41efc7fca" {
		t.Fatalf("saved live response content SHA-256 = %s", got)
	}

	var envelope responseEnvelope
	if err := decodeStrict(saved, &envelope); err != nil {
		t.Fatalf("saved live response is not strict JSON: %v", err)
	}
	brief, termDiagnostics, err := product.resolveBrief(envelope.Brief)
	if err != nil {
		t.Fatalf("saved live Brief: %v", err)
	}
	if len(brief.DomainTerms) != MaxDomainTerms || brief.DomainTerms[7].Term != "MFA" ||
		termDiagnostics.DomainTermsReceived != 9 || termDiagnostics.DomainTermsAccepted != MaxDomainTerms ||
		termDiagnostics.DomainTermsRejected != 1 ||
		!reflect.DeepEqual(termDiagnostics.DomainTermIssues, []DomainTermIssue{{
			Position: MaxDomainTerms, Code: DomainTermIssueUnrequestedOutput,
		}}) {
		t.Fatalf("saved live domain terms/diagnostics = %#v / %#v", brief.DomainTerms, termDiagnostics)
	}
	directions, directionDiagnostics := product.resolveDirections(envelope.Directions)
	if len(directions) != 0 || directionDiagnostics.DirectionsReceived != 5 ||
		directionDiagnostics.DirectionsAccepted != 0 || directionDiagnostics.DirectionsRejected != 5 {
		t.Fatalf("saved live direction result = %d / %#v", len(directions), directionDiagnostics)
	}
	wantRawIssues := []DirectionIssue{
		{Position: 0, Code: IssueWrongKindPrincipalRef},
		{Position: 1, Code: IssueWrongKindPrincipalRef},
		{Position: 2, Code: IssueInvalidPrincipalCount},
		{Position: 3, Code: IssueWrongKindPrincipalRef},
		{Position: 4, Code: IssueInvalidPrincipalCount},
	}
	if !reflect.DeepEqual(directionDiagnostics.Issues, wantRawIssues) {
		t.Fatalf("saved live route diagnostics = %#v, want %#v", directionDiagnostics.Issues, wantRawIssues)
	}
	_, returnedDiagnostics, err := product.ResolveResponseJSON(saved)
	var decodeErr *ResponseDecodeError
	if err == nil || errors.As(err, &decodeErr) ||
		!strings.Contains(err.Error(), "no valid Study directions") ||
		returnedDiagnostics.DomainTermsReceived != 9 ||
		returnedDiagnostics.DomainTermsAccepted != MaxDomainTerms ||
		!reflect.DeepEqual(returnedDiagnostics.DomainTermIssues, termDiagnostics.DomainTermIssues) ||
		!reflect.DeepEqual(returnedDiagnostics.Issues, directionDiagnostics.Issues) {
		t.Fatalf("saved live validation failure = %v / %#v", err, returnedDiagnostics)
	}

	corrected := responseMap(t, saved)
	routes := corrected["directions"].([]any)
	routes[2].(map[string]any)["principal_refs"] = []any{"c4"}
	routes[3].(map[string]any)["principal_refs"] = []any{"c2", "c3", "c4"}
	routes[4].(map[string]any)["principal_refs"] = []any{"c4"}
	result, correctedDiagnostics, err := product.ResolveResponseJSON(marshalTestJSON(t, corrected))
	if err != nil {
		t.Fatalf("manually corrected sibling routes: %v", err)
	}
	if len(result.Brief.DomainTerms) != MaxDomainTerms || len(result.Directions) != 3 ||
		correctedDiagnostics.DirectionsAccepted != 3 ||
		correctedDiagnostics.DirectionsRejected != 2 ||
		correctedDiagnostics.DomainTermsReceived != 9 ||
		correctedDiagnostics.DomainTermsAccepted != MaxDomainTerms ||
		correctedDiagnostics.DomainTermsRejected != 1 {
		t.Fatalf("corrected result = terms:%d directions:%d diagnostics:%#v",
			len(result.Brief.DomainTerms), len(result.Directions), correctedDiagnostics)
	}
	wantCorrectedIssues := []DirectionIssue{
		{Position: 0, Code: IssueWrongKindPrincipalRef},
		{Position: 1, Code: IssueWrongKindPrincipalRef},
	}
	if !reflect.DeepEqual(correctedDiagnostics.Issues, wantCorrectedIssues) {
		t.Fatalf("corrected sibling diagnostics = %#v", correctedDiagnostics.Issues)
	}
}

func TestSavedCasdoor190133ResponsePreservesBriefAndAcceptsScopedLocatorRoute(t *testing.T) {
	requestJSON, err := os.ReadFile("testdata/casdoor_20260802_190133_request_v6.json")
	if err != nil {
		t.Fatal(err)
	}
	if got := digest(requestJSON); got != "fa221b6119a090515e0bfdc770d9318d362ba008e7c9ffdacf3d53192d54ecb2" {
		t.Fatalf("saved request SHA-256 = %s", got)
	}
	if _, err := DecodeRequestRecord(requestJSON); err == nil {
		t.Fatal("stale prompt-v6 request replayed under the current prompt contract")
	}
	var request RequestRecord
	if err := json.Unmarshal(requestJSON, &request); err != nil {
		t.Fatal(err)
	}
	request.CatalogRef = fmt.Sprintf("atlas-study-v%d-%s", Version, request.CatalogSHA256)
	product := productFromArtifact(
		request.AtlasSHA256, request.ArchitectureSHA256, request.WireSHA256,
		request.CatalogSHA256, request.CatalogRef, request.Language, request.Catalog,
	)
	product.wire = []byte(request.WireJSON)
	response, err := os.ReadFile("testdata/casdoor_20260802_190133_response.json")
	if err != nil {
		t.Fatal(err)
	}
	if got := digest(bytes.TrimSuffix(response, []byte("\n"))); got != "fbe083abe67397e128a9c05723c3d251bab8f34a8be7e5c0bc86a357c6567d5b" {
		t.Fatalf("saved response content SHA-256 = %s", got)
	}
	result, diagnostics, err := product.ResolveResponseJSON(response)
	if err != nil {
		t.Fatalf("saved response: %v", err)
	}
	if len(result.Brief.WhatItIs.SupportRefs) != 10 || len(result.Directions) != 2 ||
		diagnostics.DirectionsReceived != 6 || diagnostics.DirectionsAccepted != 2 ||
		diagnostics.DirectionsRejected != 4 {
		t.Fatalf("saved response result = %#v / %#v", result, diagnostics)
	}
	wantIssues := []DirectionIssue{
		{Position: 1, Code: IssuePrincipalNotAdvertised},
		{Position: 2, Code: IssueReadingPrincipalMissing},
		{Position: 4, Code: IssuePrincipalNotAdvertised},
		{Position: 5, Code: IssuePrincipalNotAdvertised},
	}
	if !reflect.DeepEqual(diagnostics.Issues, wantIssues) {
		t.Fatalf("saved response diagnostics = %#v, want %#v", diagnostics.Issues, wantIssues)
	}
}

func TestSavedCasdoor193502ResponsePublishesBriefAndUsefulRoutesWithoutWireRefs(t *testing.T) {
	requestJSON, err := os.ReadFile("testdata/casdoor_20260802_193502_request_v9.json")
	if err != nil {
		t.Fatal(err)
	}
	if got := digest(requestJSON); got != "a4550a1f40338d2691402551582ecd563e2d07017f56fa9116f8eca38348f10f" {
		t.Fatalf("saved request SHA-256 = %s", got)
	}
	if _, err := DecodeRequestRecord(requestJSON); err == nil {
		t.Fatal("stale prompt-v9 request replayed under the item-local domain-term contract")
	}
	var request RequestRecord
	if err := json.Unmarshal(requestJSON, &request); err != nil {
		t.Fatal(err)
	}
	request.CatalogRef = fmt.Sprintf("atlas-study-v%d-%s", Version, request.CatalogSHA256)
	product := productFromArtifact(
		request.AtlasSHA256, request.ArchitectureSHA256, request.WireSHA256,
		request.CatalogSHA256, request.CatalogRef, request.Language, request.Catalog,
	)
	product.wire = []byte(request.WireJSON)
	response, err := os.ReadFile("testdata/casdoor_20260802_193502_response.json")
	if err != nil {
		t.Fatal(err)
	}
	if got := digest(response); got != "0190db2f9068a2bf220fff83b4be0a42ba61d98b1f9e2c10c362f64e89d8a361" {
		t.Fatalf("saved response SHA-256 = %s", got)
	}
	result, diagnostics, err := product.ResolveResponseJSON(response)
	if err != nil {
		t.Fatalf("saved response: %v", err)
	}
	if len(result.Brief.DomainTerms) != 3 || len(result.Directions) != 4 ||
		diagnostics.DirectionsReceived != 5 || diagnostics.DirectionsAccepted != 4 ||
		diagnostics.DirectionsRejected != 1 || !reflect.DeepEqual(diagnostics.Issues, []DirectionIssue{{
		Position: 0, Code: IssueInvalidReadingCopy,
	}}) {
		t.Fatalf("saved response result = %#v / %#v", result, diagnostics)
	}
	for _, direction := range result.Directions {
		for _, wireRef := range []string{" c1", " c2", " c3", " c4", " c5", " c6", " c7"} {
			if strings.Contains(direction.Question, wireRef) ||
				strings.Contains(direction.WhyItMatters, wireRef) ||
				strings.Contains(direction.LearningOutcome, wireRef) {
				t.Fatalf("accepted direction leaked request-local ref %q: %#v", wireRef, direction)
			}
		}
	}
	encoded, err := EncodeResultRecord(result)
	if err != nil {
		t.Fatalf("EncodeResultRecord: %v", err)
	}
	if _, err := DecodeResultRecord(encoded); err != nil {
		t.Fatalf("DecodeResultRecord: %v", err)
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
	validationFailed, err := product.FailureStatus(FailureValidation)
	if err != nil {
		t.Fatal(err)
	}
	statuses = append(statuses, validationFailed)
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
			{ID: "anchor-start-canonical", Owner: CanonicalRef{Kind: RefComponent, ID: "component-api-canonical"}, RelatedComponentIDs: []string{"component-api-canonical"}, PrincipalRefs: []CanonicalRef{{Kind: RefComponent, ID: "component-api-canonical"}}, Kind: ReadingTargetEntrypoint, Label: "Server startup", Fact: "Initializes the application shell.", Authority: repositoryatlas.AuthorityObserved, Location: evidence.Location{Path: "cmd/server/main.go", Line: 20}, Symbol: "RunServer"},
			{ID: "anchor-config-canonical", Owner: CanonicalRef{Kind: RefComponent, ID: "component-api-canonical"}, RelatedComponentIDs: []string{"component-api-canonical"}, PrincipalRefs: []CanonicalRef{{Kind: RefComponent, ID: "component-api-canonical"}}, Kind: ReadingTargetFunction, Label: "Configuration load", Fact: "Loads bounded application settings.", Authority: repositoryatlas.AuthorityObserved, Location: evidence.Location{Path: "internal/config/load.go", Line: 14}, Symbol: "Load"},
			{ID: "anchor-route-canonical", Owner: CanonicalRef{Kind: RefComponent, ID: "component-api-canonical"}, RelatedComponentIDs: []string{"component-api-canonical"}, PrincipalRefs: []CanonicalRef{{Kind: RefComponent, ID: "component-api-canonical"}}, Kind: ReadingTargetFunction, Label: "Route registration", Fact: "Registers the public request surface.", Authority: repositoryatlas.AuthorityObserved, Location: evidence.Location{Path: "internal/server/routes.go", Line: 31}, Symbol: "RegisterRoutes"},
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

func studyRouteBoundaryInput() Input {
	input := cloneTestInput(testInput())
	for ordinal := 4; ordinal <= 6; ordinal++ {
		id := fmt.Sprintf("anchor-extra-%02d-canonical", ordinal)
		input.ReadingTargets = append(input.ReadingTargets, ReadingTarget{
			ID: id,
			Owner: CanonicalRef{
				Kind: RefComponent, ID: "component-api-canonical",
			},
			RelatedComponentIDs: []string{"component-api-canonical"},
			PrincipalRefs: []CanonicalRef{{
				Kind: RefComponent, ID: "component-api-canonical",
			}},
			Kind: ReadingTargetFunction, Label: fmt.Sprintf("Extra target %02d", ordinal),
			Fact:      "Shows one exact bounded repository target.",
			Authority: repositoryatlas.AuthorityObserved,
			Location: evidence.Location{
				Path: fmt.Sprintf("internal/extra/target_%02d.go", ordinal), Line: ordinal,
			},
			Symbol: fmt.Sprintf("Target%02d", ordinal),
		})
		input.Architecture.Components[0].ReadingTargetIDs = append(
			input.Architecture.Components[0].ReadingTargetIDs, id,
		)
	}
	return input
}

func studyRouteTargetIDs() []string {
	return []string{
		"anchor-config-canonical",
		"anchor-route-canonical",
		"anchor-start-canonical",
		"anchor-extra-04-canonical",
		"anchor-extra-05-canonical",
		"anchor-extra-06-canonical",
	}
}

func providerReadingItems(t *testing.T, product Product, targetIDs []string) []any {
	t.Helper()
	labels := []ReadingLabel{
		ReadingStart, ReadingContinue, ReadingConnect, ReadingVerify, ReadingContrast,
	}
	items := make([]any, 0, len(targetIDs))
	for index, targetID := range targetIDs {
		items = append(items, map[string]any{
			"target_ref": refFor(t, product, RefReadingTarget, targetID),
			"label":      string(labels[index%len(labels)]),
			"what_to_look_for": fmt.Sprintf(
				"Inspect bounded evidence item number %d.", index+1,
			),
		})
	}
	return items
}

func casdoor144414ShapeInput() Input {
	input := cloneTestInput(testInput())
	input.Language = LanguageRussian
	for index := 1; index <= 21; index++ {
		input.Atlas.Units = append(input.Atlas.Units, repositoryatlas.Unit{
			ID:       fmt.Sprintf("unit-package-%02d", index),
			Kind:     repositoryatlas.UnitPackage,
			ParentID: "unit-module-canonical",
			Name:     fmt.Sprintf("Fixture package %02d", index),
		})
	}
	for index := 3; index <= 6; index++ {
		id := fmt.Sprintf("component-extra-%02d", index)
		input.Architecture.Components = append(input.Architecture.Components, Component{
			ID: id, SubsystemID: "subsystem-core-canonical",
			Name:        fmt.Sprintf("Fixture component %02d", index),
			Description: "Provides one bounded repository responsibility.",
			Authority:   repositoryatlas.AuthorityResolved,
		})
		input.Architecture.Subsystems[0].ComponentIDs = append(
			input.Architecture.Subsystems[0].ComponentIDs,
			id,
		)
	}
	ownerByOrdinal := map[int]string{
		4: "component-api-canonical", 5: "component-extra-05",
		6: "component-extra-05", 7: "component-extra-06",
		8: "component-extra-06", 9: "component-api-canonical",
		10: "component-api-canonical", 11: "component-extra-03",
	}
	componentIndex := make(map[string]int, len(input.Architecture.Components))
	for index, component := range input.Architecture.Components {
		componentIndex[component.ID] = index
	}
	for ordinal := 4; ordinal <= 11; ordinal++ {
		id := fmt.Sprintf("anchor-z%02d-canonical", ordinal)
		owner := ownerByOrdinal[ordinal]
		input.ReadingTargets = append(input.ReadingTargets, ReadingTarget{
			ID: id, Owner: CanonicalRef{Kind: RefComponent, ID: owner},
			RelatedComponentIDs: []string{owner},
			PrincipalRefs:       []CanonicalRef{{Kind: RefComponent, ID: owner}},
			Kind:                ReadingTargetFunction, Label: fmt.Sprintf("Fixture target %02d", ordinal),
			Fact:      "Shows one exact local reading target.",
			Authority: repositoryatlas.AuthorityObserved,
			Location: evidence.Location{
				Path: fmt.Sprintf("internal/fixture/target_%02d.go", ordinal), Line: ordinal,
			},
			Symbol: fmt.Sprintf("Target%02d", ordinal),
		})
		index := componentIndex[owner]
		input.Architecture.Components[index].ReadingTargetIDs = append(
			input.Architecture.Components[index].ReadingTargetIDs,
			id,
		)
	}
	return input
}

func casdoor175017ResponseInput() Input {
	input := cloneTestInput(testInput())
	input.Language = LanguageRussian
	input.Architecture = ArchitectureInput{
		Version: 6, Source: "local_anchors", Title: "Casdoor architecture",
		Subsystems: []Subsystem{
			{ID: "subsystem-01", Name: "Security", Authority: repositoryatlas.AuthorityResolved, ComponentIDs: []string{"component-02"}},
			{ID: "subsystem-02", Name: "Entry and dispatch", Authority: repositoryatlas.AuthorityResolved, ComponentIDs: []string{"component-03"}},
			{ID: "subsystem-03", Name: "Supporting evidence", Authority: repositoryatlas.AuthorityResolved, ComponentIDs: []string{"component-01"}},
			{ID: "subsystem-04", Name: "Runtime and extensions", Authority: repositoryatlas.AuthorityResolved, ComponentIDs: []string{"component-04", "component-05"}},
		},
		Components: []Component{
			{ID: "component-01", SubsystemID: "subsystem-03", Name: "Supporting repository evidence", Authority: repositoryatlas.AuthorityResolved, ReadingTargetIDs: []string{"reading-04"}},
			{ID: "component-02", SubsystemID: "subsystem-01", Name: "TLS and security boundary", Authority: repositoryatlas.AuthorityResolved, ReadingTargetIDs: []string{"reading-08"}},
			{ID: "component-03", SubsystemID: "subsystem-02", Name: "Primary application", Authority: repositoryatlas.AuthorityResolved, ReadingTargetIDs: []string{"reading-04"}},
			{ID: "component-04", SubsystemID: "subsystem-04", Name: "Lifecycle startup", Authority: repositoryatlas.AuthorityResolved, ReadingTargetIDs: []string{"reading-01", "reading-02", "reading-03", "reading-05", "reading-06", "reading-09", "reading-10"}},
			{ID: "component-05", SubsystemID: "subsystem-04", Name: "Lifecycle contracts", Authority: repositoryatlas.AuthorityResolved, ReadingTargetIDs: []string{"reading-07"}},
		},
	}
	input.Surfaces = []Surface{{
		ID: "surface-start-canonical", UnitID: "unit-app-canonical", Name: "main", Kind: "process_entry",
		Authority: repositoryatlas.AuthorityResolved, ReadingTargetIDs: []string{"reading-04"},
	}}
	targetPrincipals := [][]CanonicalRef{
		{{Kind: RefComponent, ID: "component-04"}},
		{{Kind: RefComponent, ID: "component-04"}},
		{{Kind: RefComponent, ID: "component-04"}},
		{{Kind: RefComponent, ID: "component-01"}, {Kind: RefComponent, ID: "component-03"}, {Kind: RefSurface, ID: "surface-start-canonical"}},
		{{Kind: RefComponent, ID: "component-04"}},
		{{Kind: RefComponent, ID: "component-04"}},
		{{Kind: RefComponent, ID: "component-05"}},
		{{Kind: RefComponent, ID: "component-02"}},
		{{Kind: RefComponent, ID: "component-04"}},
		{{Kind: RefComponent, ID: "component-04"}},
	}
	input.ReadingTargets = nil
	for index, principals := range targetPrincipals {
		ordinal := index + 1
		owner := principals[0]
		related := make([]string, 0, len(principals))
		for _, principal := range principals {
			if principal.Kind == RefComponent {
				related = append(related, principal.ID)
			}
		}
		input.ReadingTargets = append(input.ReadingTargets, ReadingTarget{
			ID: fmt.Sprintf("reading-%02d", ordinal), Owner: owner,
			RelatedComponentIDs: related, PrincipalRefs: principals,
			Kind: ReadingTargetFunction, Label: fmt.Sprintf("Repository target %02d", ordinal),
			Fact: "Exact bounded repository reading target.", Authority: repositoryatlas.AuthorityObserved,
			Location: evidence.Location{Path: fmt.Sprintf("internal/fixture/target_%02d.go", ordinal), Line: ordinal},
			Symbol:   fmt.Sprintf("Target%02d", ordinal),
		})
	}
	input.Evidence = nil
	input.Documents = []DocumentClaim{{
		ID: "document-01", Label: "Casdoor overview",
		Claim:     "Casdoor provides identity and access management capabilities.",
		Authority: repositoryatlas.AuthorityObserved,
	}}
	return input
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
