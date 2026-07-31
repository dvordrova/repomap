package localization

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestCanonicalIdentityProjectionIsDeterministicAndExact(t *testing.T) {
	t.Parallel()

	specs := fixtureSpecs()
	first, err := NewCanonical(specs)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewCanonical([]FieldSpec{specs[1], specs[0]})
	if err != nil {
		t.Fatal(err)
	}
	firstBytes, err := MarshalCanonical(first)
	if err != nil {
		t.Fatal(err)
	}
	secondBytes, err := MarshalCanonical(second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstBytes, secondBytes) {
		t.Fatalf("canonical bytes differ:\n%s\n%s", firstBytes, secondBytes)
	}
	const wantSHA256 = "00b4e135c518d68918594b1a605d5758de081f800e50459f58e0b18ad988db59"
	if first.SHA256 != wantSHA256 {
		t.Fatalf("canonical SHA-256 = %q, want %q", first.SHA256, wantSHA256)
	}

	projection, err := IdentityProjection(first)
	if err != nil {
		t.Fatal(err)
	}
	input, err := BuildInput(first, LocaleEnglish)
	if err != nil {
		t.Fatal(err)
	}
	result, err := Apply(first, input, projection)
	if err != nil {
		t.Fatal(err)
	}
	if result.Fallback || len(result.Diagnostics) != 0 {
		t.Fatalf("identity result = %#v", result)
	}
	for index, field := range first.Fields {
		if result.Fields[index].ID != field.ID || result.Fields[index].Text != field.Text {
			t.Fatalf("identity field %d = %#v, want %#v", index, result.Fields[index], field)
		}
	}
}

func TestRussianProjectionChangesOnlyExplicitTypedValues(t *testing.T) {
	t.Parallel()

	canonical, err := NewCanonical(fixtureSpecs())
	if err != nil {
		t.Fatal(err)
	}
	input, err := BuildInput(canonical, LocaleRussian)
	if err != nil {
		t.Fatal(err)
	}
	encodedInput, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	for _, protected := range []string{
		"cmd/сервер main.go:42",
		"StartReplication",
		"PostgreSQL",
		"API/v2",
		"CJK",
	} {
		if bytes.Contains(encodedInput, []byte(protected)) {
			t.Fatalf("localization input exposed protected term %q: %s", protected, encodedInput)
		}
	}

	translations := make(map[string]string, len(input.Fields))
	for _, field := range input.Fields {
		translations[field.ID] = russianFixtureText(field)
	}
	result, err := Apply(canonical, input, Projection{
		Version:         ProjectionVersion,
		CanonicalSHA256: canonical.SHA256,
		Locale:          LocaleRussian,
		Translations:    translations,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Fallback || len(result.Diagnostics) != 0 || result.Locale != LocaleRussian {
		t.Fatalf("Russian result = %#v", result)
	}
	joined := result.Fields[0].Text + "\n" + result.Fields[1].Text
	for _, protected := range []string{
		"cmd/сервер main.go:42",
		"StartReplication",
		"PostgreSQL",
		"API/v2",
	} {
		if !strings.Contains(joined, protected) {
			t.Fatalf("projected text lost protected term %q: %s", protected, joined)
		}
	}
	if !strings.Contains(joined, "東京") {
		t.Fatalf("projected text lost CJK prose: %s", joined)
	}
}

func TestRussianProjectionAcceptsStructurallyValidMixedScriptProse(t *testing.T) {
	t.Parallel()

	canonical, err := NewCanonical([]FieldSpec{{
		OwnerKind: OwnerPresentationText,
		OwnerID:   "architecture/flows/http-request-handling/trigger",
		Name:      FieldText,
		Text: "Incoming HTTP request to the Go server " +
			"(e.g., curl http://localhost:30080/svc2/proxy) with Grafana.",
		ProtectedTerms: []ProtectedValue{
			{Kind: ProtectedURL, Value: "http://localhost:30080/svc2/proxy"},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	input, err := BuildInput(canonical, LocaleRussian)
	if err != nil {
		t.Fatal(err)
	}
	urlToken := protectedToken(t, canonical.Fields[0], "http://localhost:30080/svc2/proxy")
	wantText := "Входящий HTTP-запрос к Go-серверу " +
		"(например, curl " + urlToken + ") с Grafana."
	result, err := Apply(canonical, input, Projection{
		Version:         ProjectionVersion,
		CanonicalSHA256: canonical.SHA256,
		Locale:          LocaleRussian,
		Translations: map[string]string{
			input.Fields[0].ID: wantText,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Fallback || len(result.Diagnostics) != 0 {
		t.Fatalf("mixed-script projection = %#v", result)
	}
	wantText = strings.ReplaceAll(
		wantText,
		urlToken,
		"http://localhost:30080/svc2/proxy",
	)
	if result.Fields[0].Text != wantText {
		t.Fatalf("mixed-script field = %q, want %q", result.Fields[0].Text, wantText)
	}
}

func TestRussianProjectionAllowsOpaqueOnlyField(t *testing.T) {
	t.Parallel()

	canonical, err := NewCanonical([]FieldSpec{{
		OwnerKind: OwnerComponent,
		OwnerID:   "protocol",
		Name:      FieldSummary,
		Text:      "RESP3",
		ProtectedTerms: []ProtectedValue{{
			Kind:  ProtectedProtocol,
			Value: "RESP3",
		}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	input, err := BuildInput(canonical, LocaleRussian)
	if err != nil {
		t.Fatal(err)
	}
	if len(canonical.Fields[0].ProtectedTerms) != 1 ||
		input.Fields[0].Text != canonical.Fields[0].ProtectedTerms[0].Token {
		t.Fatalf("opaque input = %#v from %#v", input.Fields[0], canonical.Fields[0])
	}
	result, err := Apply(canonical, input, Projection{
		Version:         ProjectionVersion,
		CanonicalSHA256: canonical.SHA256,
		Locale:          LocaleRussian,
		Translations: map[string]string{
			input.Fields[0].ID: input.Fields[0].Text,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Fallback || len(result.Diagnostics) != 0 ||
		result.Fields[0].Text != "RESP3" {
		t.Fatalf("opaque-only result = %#v", result)
	}
}

func TestCanonicalProtectsOnlyExplicitTypedValues(t *testing.T) {
	t.Parallel()

	canonical, err := NewCanonical([]FieldSpec{
		{
			OwnerKind: OwnerComponent,
			OwnerID:   "redis",
			Name:      FieldSummary,
			Text:      "Uses Redis over RESP3.",
			ProtectedTerms: []ProtectedValue{
				{Kind: ProtectedProduct, Value: "Redis"},
				{Kind: ProtectedProtocol, Value: "RESP3"},
			},
		},
		{
			OwnerKind: OwnerComponent,
			OwnerID:   "ordinary-title",
			Name:      FieldSummary,
			Text:      "HTTP ServerControllers V2",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	byID := make(map[string]CanonicalField, len(canonical.Fields))
	for _, field := range canonical.Fields {
		byID[field.OwnerID] = field
	}
	redisField := byID["redis"]
	for _, value := range []string{"Redis", "RESP3"} {
		if token := protectedToken(t, redisField, value); token == "" {
			t.Fatalf("technical value %q has no placeholder", value)
		}
	}
	if terms := byID["ordinary-title"].ProtectedTerms; len(terms) != 0 {
		t.Fatalf("untyped technical-looking words became opaque: %#v", terms)
	}

	input, err := BuildInput(canonical, LocaleRussian)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range input.Fields {
		if field.ID == redisField.ID &&
			(strings.Contains(field.Text, "Redis") || strings.Contains(field.Text, "RESP3")) {
			t.Fatalf("technical values leaked into provider input: %q", field.Text)
		}
	}
}

func TestProtectedIdentifierRequiresAnIdentityBoundary(t *testing.T) {
	t.Parallel()

	const text = "The domain invokes main for requests."
	canonical, err := NewCanonical([]FieldSpec{{
		OwnerKind: OwnerComponent,
		OwnerID:   "main-boundary",
		Name:      FieldSummary,
		Text:      text,
		ProtectedTerms: []ProtectedValue{{
			Kind:  ProtectedSymbol,
			Value: "main",
		}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	field := canonical.Fields[0]
	if len(field.ProtectedTerms) != 1 || field.ProtectedTerms[0].Count != 1 {
		t.Fatalf("protected terms = %#v, want one standalone main occurrence", field.ProtectedTerms)
	}
	input, err := BuildInput(canonical, LocaleRussian)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(input.Fields[0].Text, "domain") ||
		strings.Contains(input.Fields[0].Text, " main ") {
		t.Fatalf("boundary-aware input = %q", input.Fields[0].Text)
	}

	_, err = NewCanonical([]FieldSpec{{
		OwnerKind: OwnerComponent,
		OwnerID:   "embedded-main",
		Name:      FieldSummary,
		Text:      "The domain processes requests.",
		ProtectedTerms: []ProtectedValue{{
			Kind:  ProtectedSymbol,
			Value: "main",
		}},
	}})
	if err == nil || !strings.Contains(err.Error(), "protected term is absent") {
		t.Fatalf("embedded identifier error = %v, want absent protected term", err)
	}
}

func TestRussianProjectionRestoresProtectedIdentifierInMixedScriptProse(t *testing.T) {
	t.Parallel()

	canonical, err := NewCanonical([]FieldSpec{{
		OwnerKind: OwnerComponent,
		OwnerID:   "domain-main",
		Name:      FieldSummary,
		Text:      "The domain invokes main.",
		ProtectedTerms: []ProtectedValue{{
			Kind:  ProtectedSymbol,
			Value: "main",
		}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	input, err := BuildInput(canonical, LocaleRussian)
	if err != nil {
		t.Fatal(err)
	}
	mainToken := protectedToken(t, canonical.Fields[0], "main")
	result, err := Apply(canonical, input, Projection{
		Version:         ProjectionVersion,
		CanonicalSHA256: canonical.SHA256,
		Locale:          LocaleRussian,
		Translations: map[string]string{
			input.Fields[0].ID: "Слой domain вызывает " + mainToken + ".",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Fallback || len(result.Diagnostics) != 0 ||
		result.Fields[0].Text != "Слой domain вызывает main." {
		t.Fatalf("mixed-script protected result = %#v", result)
	}
}

func TestUntypedAcronymDoesNotRequireAnOpaquePlaceholder(t *testing.T) {
	t.Parallel()

	canonical, err := NewCanonical([]FieldSpec{{
		OwnerKind: OwnerComponent,
		OwnerID:   "http-routing",
		Name:      FieldSummary,
		Text:      "HTTP Request Routing handles traffic.",
	}})
	if err != nil {
		t.Fatal(err)
	}
	field := canonical.Fields[0]
	if len(field.ProtectedTerms) != 0 {
		t.Fatalf("untyped acronym became opaque: %#v", field.ProtectedTerms)
	}
	input, err := BuildInput(canonical, LocaleRussian)
	if err != nil {
		t.Fatal(err)
	}
	if input.Fields[0].Text != canonical.Fields[0].Text {
		t.Fatalf("provider input = %q, want complete human prose", input.Fields[0].Text)
	}
	result, err := Apply(canonical, input, Projection{
		Version:         ProjectionVersion,
		CanonicalSHA256: canonical.SHA256,
		Locale:          LocaleRussian,
		Translations: map[string]string{
			input.Fields[0].ID: "HTTP маршрутизация обрабатывает трафик.",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Fallback || len(result.Diagnostics) != 0 ||
		result.Fields[0].Text != "HTTP маршрутизация обрабатывает трафик." {
		t.Fatalf("mixed-script acronym result = %#v", result)
	}
}

func TestProjectionFallsBackPerFieldWithDeterministicDiagnostics(t *testing.T) {
	t.Parallel()

	canonical, err := NewCanonical(fixtureSpecs())
	if err != nil {
		t.Fatal(err)
	}
	input, err := BuildInput(canonical, LocaleRussian)
	if err != nil {
		t.Fatal(err)
	}
	translations := map[string]string{
		"component:unknown.summary": "unknown",
		input.Fields[0].ID:          "Потерян placeholder.",
	}
	result, err := Apply(canonical, input, Projection{
		Version:         ProjectionVersion,
		CanonicalSHA256: canonical.SHA256,
		Locale:          LocaleRussian,
		Translations:    translations,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Fallback {
		t.Fatal("invalid projection did not report fallback")
	}
	wantCodes := []Diagnostic{
		{Code: "unknown_field_id", FieldID: "component:unknown.summary"},
		{Code: "placeholder_mismatch", FieldID: input.Fields[0].ID},
		{Code: "missing_translation", FieldID: input.Fields[1].ID},
	}
	if !equalDiagnostics(result.Diagnostics, wantCodes) {
		t.Fatalf("diagnostics = %#v, want %#v", result.Diagnostics, wantCodes)
	}
	for index, field := range canonical.Fields {
		if result.Fields[index].Text != field.Text {
			t.Fatalf("field %q did not fall back to canonical text", field.ID)
		}
	}
}

func TestProjectionHashMismatchFallsBackAtomically(t *testing.T) {
	t.Parallel()

	canonical, err := NewCanonical(fixtureSpecs())
	if err != nil {
		t.Fatal(err)
	}
	input, err := BuildInput(canonical, LocaleRussian)
	if err != nil {
		t.Fatal(err)
	}
	result, err := Apply(canonical, input, Projection{
		Version:         ProjectionVersion,
		CanonicalSHA256: strings.Repeat("0", 64),
		Locale:          LocaleRussian,
		Translations: map[string]string{
			canonical.Fields[0].ID: "Не должно примениться.",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Fallback ||
		!equalDiagnostics(result.Diagnostics, []Diagnostic{{Code: "canonical_hash_mismatch"}}) {
		t.Fatalf("hash mismatch result = %#v", result)
	}
	for index, field := range canonical.Fields {
		if result.Fields[index].Text != field.Text {
			t.Fatalf("hash mismatch changed field %q", field.ID)
		}
	}
}

func TestProjectionRejectsInvalidUTF8AndAddedPlaceholder(t *testing.T) {
	t.Parallel()

	canonical, err := NewCanonical(fixtureSpecs())
	if err != nil {
		t.Fatal(err)
	}
	input, err := BuildInput(canonical, LocaleRussian)
	if err != nil {
		t.Fatal(err)
	}
	result, err := Apply(canonical, input, Projection{
		Version:         ProjectionVersion,
		CanonicalSHA256: canonical.SHA256,
		Locale:          LocaleRussian,
		Translations: map[string]string{
			input.Fields[0].ID: string([]byte{0xff}),
			input.Fields[1].ID: "Как {{term_01}} использует {{term_02}} и {{term_99}}?",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []Diagnostic{
		{Code: "invalid_utf8", FieldID: input.Fields[0].ID},
		{Code: "placeholder_mismatch", FieldID: input.Fields[1].ID},
	}
	if !equalDiagnostics(result.Diagnostics, want) {
		t.Fatalf("diagnostics = %#v, want %#v", result.Diagnostics, want)
	}
}

func TestProjectionIsBoundToRequestedLocale(t *testing.T) {
	t.Parallel()

	canonical, err := NewCanonical(fixtureSpecs())
	if err != nil {
		t.Fatal(err)
	}
	input, err := BuildInput(canonical, LocaleRussian)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := IdentityProjection(canonical)
	if err != nil {
		t.Fatal(err)
	}
	result, err := Apply(canonical, input, projection)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Fallback || result.Locale != LocaleEnglish ||
		!equalDiagnostics(result.Diagnostics, []Diagnostic{{Code: "projection_locale_mismatch"}}) {
		t.Fatalf("locale mismatch result = %#v", result)
	}
}

func TestProjectionProcessingIsBoundedByCanonicalInput(t *testing.T) {
	t.Parallel()

	canonical, err := NewCanonical(fixtureSpecs())
	if err != nil {
		t.Fatal(err)
	}
	input, err := BuildInput(canonical, LocaleRussian)
	if err != nil {
		t.Fatal(err)
	}

	tooMany := map[string]string{
		input.Fields[0].ID: "one",
		input.Fields[1].ID: "two",
		"unknown":          strings.Repeat("x", 1<<20),
	}
	result, err := Apply(canonical, input, Projection{
		Version:         ProjectionVersion,
		CanonicalSHA256: canonical.SHA256,
		Locale:          LocaleRussian,
		Translations:    tooMany,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !equalDiagnostics(result.Diagnostics, []Diagnostic{{Code: "projection_field_count_exceeded"}}) {
		t.Fatalf("field-count diagnostics = %#v", result.Diagnostics)
	}

	tooLarge := strings.Repeat("x", translationByteLimit(input.Fields[0].Text)+1)
	result, err = Apply(canonical, input, Projection{
		Version:         ProjectionVersion,
		CanonicalSHA256: canonical.SHA256,
		Locale:          LocaleRussian,
		Translations: map[string]string{
			input.Fields[0].ID: tooLarge,
			input.Fields[1].ID: "Как {{term_01}} использует {{term_02}}?",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !equalDiagnostics(result.Diagnostics, []Diagnostic{{
		Code:    "translation_too_large",
		FieldID: input.Fields[0].ID,
	}}) {
		t.Fatalf("scalar diagnostics = %#v", result.Diagnostics)
	}
}

func TestCanonicalValidationRederivesProtectedMetadata(t *testing.T) {
	t.Parallel()

	canonical, err := NewCanonical(fixtureSpecs())
	if err != nil {
		t.Fatal(err)
	}
	canonical.Fields[0].ProtectedTerms[0].Count++
	canonical.SHA256 = canonicalHash(canonical)
	if _, err := BuildInput(canonical, LocaleRussian); err == nil {
		t.Fatal("self-hashed malformed placeholder count unexpectedly accepted")
	}

	canonical, err = NewCanonical(fixtureSpecs())
	if err != nil {
		t.Fatal(err)
	}
	canonical.Fields[0].ProtectedTerms[0].Value = string([]byte{0xff})
	canonical.SHA256 = canonicalHash(canonical)
	if _, err := BuildInput(canonical, LocaleRussian); err == nil {
		t.Fatal("self-hashed invalid UTF-8 placeholder unexpectedly accepted")
	}
}

func TestFieldIDsAreAllowlistedAndStable(t *testing.T) {
	t.Parallel()

	id, err := FieldID(OwnerComponent, "component-01", FieldSummary)
	if err != nil {
		t.Fatal(err)
	}
	if id != "component:component-01.summary" {
		t.Fatalf("FieldID = %q", id)
	}
	if _, err := FieldID(OwnerSubsystem, "subsystem-01", FieldDescription); err != nil {
		t.Fatalf("subsystem description is not allowlisted: %v", err)
	}
	if _, err := FieldID(OwnerSubsystem, "subsystem-01", FieldWhyItMatters); err != nil {
		t.Fatalf("existing subsystem why_it_matters field was removed: %v", err)
	}
	if _, err := FieldID(OwnerComponent, "component-01", FieldDescription); err != nil {
		t.Fatalf("component description is not allowlisted: %v", err)
	}
	if _, err := FieldID(OwnerComponent, "component-01", FieldQuestion); err == nil {
		t.Fatal("component question unexpectedly entered the localization allowlist")
	}
	if _, err := FieldID(OwnerComponent, " translated text ", FieldSummary); err == nil {
		t.Fatal("unstable owner id unexpectedly accepted")
	}
}

func TestRepositoryBriefFieldIDsAreAllowlistedAndDeterministic(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name FieldName
		want string
	}{
		{name: FieldWhatItIs, want: "study_brief:repository-brief.what_it_is"},
		{name: FieldProblem, want: "study_brief:repository-brief.problem"},
		{name: FieldMainInput, want: "study_brief:repository-brief.main_input"},
		{name: FieldCentralResponsibility, want: "study_brief:repository-brief.central_responsibility"},
		{name: FieldObservableResult, want: "study_brief:repository-brief.observable_result"},
	}
	for _, test := range tests {
		id, err := FieldID(OwnerStudyBrief, "repository-brief", test.name)
		if err != nil {
			t.Fatalf("FieldID(%q): %v", test.name, err)
		}
		if id != test.want {
			t.Fatalf("FieldID(%q) = %q, want %q", test.name, id, test.want)
		}
	}

	termID, err := FieldID(
		OwnerBriefDomainTerm,
		"repository-brief:domain-term:write-ahead-log",
		FieldDomainTermMeaning,
	)
	if err != nil {
		t.Fatal(err)
	}
	const wantTermID = "brief_domain_term:repository-brief:domain-term:write-ahead-log.domain_term_meaning"
	if termID != wantTermID {
		t.Fatalf("domain term FieldID = %q, want %q", termID, wantTermID)
	}

	specs := []FieldSpec{
		{
			OwnerKind: OwnerBriefDomainTerm,
			OwnerID:   "repository-brief:domain-term:write-ahead-log",
			Name:      FieldDomainTermMeaning,
			Text:      "A durable record of pending changes.",
		},
		{
			OwnerKind: OwnerStudyBrief,
			OwnerID:   "repository-brief",
			Name:      FieldWhatItIs,
			Text:      "A small storage library.",
		},
	}
	first, err := NewCanonical(specs)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewCanonical([]FieldSpec{specs[1], specs[0]})
	if err != nil {
		t.Fatal(err)
	}
	firstBytes, err := MarshalCanonical(first)
	if err != nil {
		t.Fatal(err)
	}
	secondBytes, err := MarshalCanonical(second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstBytes, secondBytes) {
		t.Fatalf("RepositoryBrief canonical bytes differ:\n%s\n%s", firstBytes, secondBytes)
	}
}

func TestRepositoryBriefFieldAllowlistRejectsWrongOwners(t *testing.T) {
	t.Parallel()

	if _, err := FieldID(OwnerStudyBrief, "repository-brief", FieldDomainTermMeaning); err == nil {
		t.Fatal("domain term meaning unexpectedly accepted without a stable term owner")
	}
	if _, err := FieldID(OwnerBriefDomainTerm, "", FieldDomainTermMeaning); err == nil {
		t.Fatal("domain term meaning unexpectedly accepted without an owner id")
	}
	if _, err := FieldID(OwnerBriefDomainTerm, "repository-brief:domain-term:wal", FieldNameText); err == nil {
		t.Fatal("domain term name unexpectedly entered the prose allowlist")
	}
	if _, err := FieldID(OwnerComponent, "component-01", FieldObservableResult); err == nil {
		t.Fatal("RepositoryBrief field unexpectedly accepted for a component")
	}
}

func TestCanonicalRejectsReservedPlaceholderAndHandlesNestedTerms(t *testing.T) {
	t.Parallel()

	if _, err := NewCanonical([]FieldSpec{{
		OwnerKind: OwnerComponent,
		OwnerID:   "unsafe",
		Name:      FieldSummary,
		Text:      "Already contains {{term_01}}.",
	}}); err == nil {
		t.Fatal("reserved placeholder unexpectedly accepted")
	}

	canonical, err := NewCanonical([]FieldSpec{{
		OwnerKind: OwnerComponent,
		OwnerID:   "nested",
		Name:      FieldSummary,
		Text:      "PostgreSQL accepts SQL and SQL.",
		ProtectedTerms: []ProtectedValue{
			{Kind: ProtectedProduct, Value: "PostgreSQL"},
			{Kind: ProtectedProtocol, Value: "SQL"},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	input, err := BuildInput(canonical, LocaleRussian)
	if err != nil {
		t.Fatal(err)
	}
	if len(input.Fields) != 1 || len(input.Fields[0].Placeholders) != 2 ||
		input.Fields[0].Placeholders[1].Count != 2 {
		t.Fatalf("nested placeholders = %#v", input.Fields)
	}

	protectedNested, err := NewCanonical([]FieldSpec{{
		OwnerKind: OwnerComponent,
		OwnerID:   "nested-protected",
		Name:      FieldSummary,
		Text:      "Calls API/v2.",
		ProtectedTerms: []ProtectedValue{{
			Kind:  ProtectedAPI,
			Value: "API/v2",
		}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	protectedInput, err := BuildInput(protectedNested, LocaleRussian)
	if err != nil {
		t.Fatal(err)
	}
	if len(protectedInput.Fields) != 1 ||
		len(protectedInput.Fields[0].Placeholders) != 1 ||
		protectedInput.Fields[0].Placeholders[0].Token != "{{term_01}}" {
		t.Fatalf("nested protected placeholders = %#v", protectedInput.Fields)
	}
}

func TestCanonicalDoesNotInferNestedSnakeCaseToken(t *testing.T) {
	t.Parallel()

	canonical, err := NewCanonical([]FieldSpec{{
		OwnerKind: OwnerComponent,
		OwnerID:   "storage",
		Name:      FieldSummary,
		Text:      "storage/aws_s3.go is one storage adapter.",
		ProtectedTerms: []ProtectedValue{{
			Kind:  ProtectedPath,
			Value: "storage/aws_s3.go",
		}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	input, err := BuildInput(canonical, LocaleRussian)
	if err != nil {
		t.Fatal(err)
	}
	if len(input.Fields) != 1 || len(input.Fields[0].Placeholders) != 1 {
		t.Fatalf("snake-case placeholders = %#v, want only the full opaque path", input.Fields)
	}
	if input.Fields[0].Placeholders[0].Kind != ProtectedPath ||
		input.Fields[0].Placeholders[0].Count != 1 {
		t.Fatalf("protected placeholder = %#v, want one full path", input.Fields[0].Placeholders[0])
	}
	if input.Fields[0].Text != "{{term_01}} is one storage adapter." {
		t.Fatalf("protected text = %q, want only the full path replaced", input.Fields[0].Text)
	}
}

func TestCanonicalProtectsOnlyExplicitPathBesideUntypedProtocol(t *testing.T) {
	t.Parallel()

	canonical, err := NewCanonical([]FieldSpec{{
		OwnerKind: OwnerComponent,
		OwnerID:   "storage-protocol",
		Name:      FieldSummary,
		Text:      "storage/aws_s3.go implements S3 storage.",
		ProtectedTerms: []ProtectedValue{{
			Kind:  ProtectedPath,
			Value: "storage/aws_s3.go",
		}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	input, err := BuildInput(canonical, LocaleRussian)
	if err != nil {
		t.Fatal(err)
	}
	if len(input.Fields) != 1 || len(input.Fields[0].Placeholders) != 1 {
		t.Fatalf("path/protocol placeholders = %#v, want only explicit path", input.Fields)
	}
	if input.Fields[0].Text != "{{term_01}} implements S3 storage." {
		t.Fatalf("protected text = %q", input.Fields[0].Text)
	}
}

func fixtureSpecs() []FieldSpec {
	return []FieldSpec{
		{
			OwnerKind: OwnerComponent,
			OwnerID:   "startup",
			Name:      FieldSummary,
			Text:      "Opens cmd/сервер main.go:42 through StartReplication; CJK: 東京.",
			ProtectedTerms: []ProtectedValue{
				{Kind: ProtectedPath, Value: "cmd/сервер main.go:42"},
				{Kind: ProtectedSymbol, Value: "StartReplication"},
				{Kind: ProtectedProtocol, Value: "CJK"},
				{Kind: ProtectedIdentifier, Value: "東京"},
			},
		},
		{
			OwnerKind: OwnerStudyDirection,
			OwnerID:   "replication",
			Name:      FieldQuestion,
			Text:      "How does PostgreSQL use API/v2?",
			ProtectedTerms: []ProtectedValue{
				{Kind: ProtectedProduct, Value: "PostgreSQL"},
				{Kind: ProtectedAPI, Value: "API/v2"},
			},
		},
	}
}

func equalDiagnostics(left, right []Diagnostic) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func russianFixtureText(field InputField) string {
	translated := "Русское описание"
	for _, placeholder := range field.Placeholders {
		for count := 0; count < placeholder.Count; count++ {
			translated += " " + placeholder.Token
		}
	}
	return translated
}

func protectedToken(t *testing.T, field CanonicalField, value string) string {
	t.Helper()
	for _, term := range field.ProtectedTerms {
		if term.Value == value {
			return term.Token
		}
	}
	t.Fatalf("field %q has no protected term %q: %#v", field.ID, value, field.ProtectedTerms)
	return ""
}
