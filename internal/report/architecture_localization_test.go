package report

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/localization"
)

func TestArchitectureLocalizationIdentityPreservesCanvasBytes(t *testing.T) {
	t.Parallel()

	canvas := architectureLocalizationFixture()
	before := marshalArchitectureLocalizationFixture(t, canvas)
	canonical, input, err := buildArchitectureLocalization(canvas, localization.LocaleEnglish)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := localization.IdentityProjection(canonical)
	if err != nil {
		t.Fatal(err)
	}
	projected, result, err := applyArchitectureLocalization(canvas, canonical, input, projection)
	if err != nil {
		t.Fatal(err)
	}
	if result.Fallback || len(result.Diagnostics) != 0 {
		t.Fatalf("identity projection result = %#v", result)
	}
	after := marshalArchitectureLocalizationFixture(t, projected)
	if !bytes.Equal(before, after) {
		t.Fatalf("identity projection changed Architecture Canvas:\n%s\n%s", before, after)
	}
}

func TestArchitectureLocalizationIdentityPreservesReportJSON(t *testing.T) {
	t.Parallel()

	canvas := architectureLocalizationFixture()
	canonical, input, err := buildArchitectureLocalization(canvas, localization.LocaleEnglish)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := localization.IdentityProjection(canonical)
	if err != nil {
		t.Fatal(err)
	}
	projected, _, err := applyArchitectureLocalization(canvas, canonical, input, projection)
	if err != nil {
		t.Fatal(err)
	}

	directory := t.TempDir()
	beforePath := filepath.Join(directory, "before.json")
	afterPath := filepath.Join(directory, "after.json")
	beforeData := &ReportData{
		FormatVersion:      CurrentFormatVersion,
		RepoName:           "architecture localization fixture",
		CandidateFlows:     []string{},
		Flows:              []FlowData{},
		ArchitectureCanvas: &canvas,
	}
	afterData := *beforeData
	afterData.ArchitectureCanvas = &projected
	if err := WriteReportJSON(beforeData, beforePath); err != nil {
		t.Fatal(err)
	}
	if err := WriteReportJSON(&afterData, afterPath); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(beforePath)
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(afterPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("identity projection changed report JSON:\n%s\n%s", before, after)
	}
}

func TestArchitectureLocalizationAcceptsDecodedCurrentArtifacts(t *testing.T) {
	t.Parallel()

	canvas := architectureLocalizationFixture()
	canonical, input, err := buildArchitectureLocalization(canvas, localization.LocaleEnglish)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := localization.IdentityProjection(canonical)
	if err != nil {
		t.Fatal(err)
	}
	roundTripArchitectureLocalizationJSON(t, &canonical)
	roundTripArchitectureLocalizationJSON(t, &input)
	roundTripArchitectureLocalizationJSON(t, &projection)

	projected, result, err := applyArchitectureLocalization(
		canvas,
		canonical,
		input,
		projection,
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Fallback || len(result.Diagnostics) != 0 {
		t.Fatalf("decoded identity result = %#v", result)
	}
	before := marshalArchitectureLocalizationFixture(t, canvas)
	after := marshalArchitectureLocalizationFixture(t, projected)
	if !bytes.Equal(before, after) {
		t.Fatalf("decoded identity artifacts changed Architecture Canvas:\n%s\n%s", before, after)
	}
}

func TestArchitectureLocalizationRussianProjectionChangesOnlyAllowlistedProse(t *testing.T) {
	t.Parallel()

	canvas := architectureLocalizationFixture()
	canonical, input, err := buildArchitectureLocalization(canvas, localization.LocaleRussian)
	if err != nil {
		t.Fatal(err)
	}
	inputJSON, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	for _, protected := range []string{
		"PostgreSQL",
		"StartReplication",
		"API/v2",
		"cmd/сервер main.go",
		"github.com/example/storage",
		"source-subsystem-01",
		"flow-01",
		"surface-01",
		"participating-surface-01",
		"investigation-01",
		"anchor-01",
		"source-component-01",
	} {
		if bytes.Contains(inputJSON, []byte(protected)) {
			t.Fatalf("localization input exposed protected term %q: %s", protected, inputJSON)
		}
	}

	translations := make(map[string]string, len(input.Fields))
	for _, field := range input.Fields {
		translations[field.ID] = "Русская проекция: " + field.Text
	}
	projected, result, err := applyArchitectureLocalization(
		canvas,
		canonical,
		input,
		localization.Projection{
			Version:         localization.ProjectionVersion,
			CanonicalSHA256: canonical.SHA256,
			Locale:          localization.LocaleRussian,
			Translations:    translations,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Fallback || len(result.Diagnostics) != 0 || result.Locale != localization.LocaleRussian {
		t.Fatalf("Russian projection result = %#v", result)
	}
	if !strings.HasPrefix(projected.Subsystems[0].Name, "Русская проекция: ") ||
		!strings.HasPrefix(projected.Subsystems[0].Description, "Русская проекция: ") ||
		!strings.HasPrefix(projected.Components[0].Name, "Русская проекция: ") ||
		!strings.HasPrefix(projected.Components[0].Description, "Русская проекция: ") {
		t.Fatalf("allowlisted prose was not projected: %#v", projected)
	}
	joined := projected.Subsystems[0].Name + "\n" +
		projected.Subsystems[0].Description + "\n" +
		projected.Components[0].Name + "\n" +
		projected.Components[0].Description
	for _, protected := range []string{
		"PostgreSQL",
		"StartReplication",
		"API/v2",
		"cmd/сервер main.go",
		"github.com/example/storage",
		"source-subsystem-01",
		"flow-01",
		"surface-01",
		"participating-surface-01",
		"investigation-01",
		"anchor-01",
		"source-component-01",
	} {
		if !strings.Contains(joined, protected) {
			t.Fatalf("Russian projection lost protected term %q: %s", protected, joined)
		}
	}
	if !strings.Contains(joined, "東京") {
		t.Fatalf("Russian projection lost CJK prose: %s", joined)
	}

	projected.Subsystems[0].Name = canvas.Subsystems[0].Name
	projected.Subsystems[0].Description = canvas.Subsystems[0].Description
	projected.Components[0].Name = canvas.Components[0].Name
	projected.Components[0].Description = canvas.Components[0].Description
	before := marshalArchitectureLocalizationFixture(t, canvas)
	after := marshalArchitectureLocalizationFixture(t, projected)
	if !bytes.Equal(before, after) {
		t.Fatalf("Russian projection changed non-prose Architecture data:\n%s\n%s", before, after)
	}
}

func TestArchitectureLocalizationProtectsStructuredKinds(t *testing.T) {
	t.Parallel()

	canvas := architectureLocalizationFixture()
	canonical, _, err := buildArchitectureLocalization(canvas, localization.LocaleRussian)
	if err != nil {
		t.Fatal(err)
	}
	kinds := make(map[string]localization.ProtectedKind)
	for _, field := range canonical.Fields {
		for _, term := range field.ProtectedTerms {
			kinds[term.Value] = term.Kind
		}
	}
	for value, want := range map[string]localization.ProtectedKind{
		"StartReplication":           localization.ProtectedSymbol,
		"API/v2":                     localization.ProtectedSymbol,
		"PostgreSQL":                 localization.ProtectedSymbol,
		"cmd/сервер main.go":         localization.ProtectedPath,
		"github.com/example/storage": localization.ProtectedPackage,
		"flow-01":                    localization.ProtectedIdentifier,
		"surface-01":                 localization.ProtectedIdentifier,
		"anchor-01":                  localization.ProtectedIdentifier,
	} {
		if got := kinds[value]; got != want {
			t.Fatalf("protected kind for %q = %q, want %q", value, got, want)
		}
	}
}

func TestArchitectureLocalizationFieldIDsIgnoreProse(t *testing.T) {
	t.Parallel()

	canvas := architectureLocalizationFixture()
	first, _, err := buildArchitectureLocalization(canvas, localization.LocaleRussian)
	if err != nil {
		t.Fatal(err)
	}
	canvas.Subsystems[0].Name = "Completely different subsystem prose"
	canvas.Subsystems[0].Description = "Different description"
	canvas.Components[0].Name = "Different component prose"
	canvas.Components[0].Description = "Another description"
	second, _, err := buildArchitectureLocalization(canvas, localization.LocaleRussian)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Fields) != len(second.Fields) {
		t.Fatalf("field counts differ: %d != %d", len(first.Fields), len(second.Fields))
	}
	for index := range first.Fields {
		if first.Fields[index].ID != second.Fields[index].ID {
			t.Fatalf("field ID depends on prose: %q != %q", first.Fields[index].ID, second.Fields[index].ID)
		}
	}
}

func TestArchitectureLocalizationEnvelopeFailureLeavesCanvasExact(t *testing.T) {
	t.Parallel()

	canvas := architectureLocalizationFixture()
	before := marshalArchitectureLocalizationFixture(t, canvas)
	canonical, input, err := buildArchitectureLocalization(canvas, localization.LocaleRussian)
	if err != nil {
		t.Fatal(err)
	}
	projected, result, err := applyArchitectureLocalization(
		canvas,
		canonical,
		input,
		localization.Projection{
			Version:         localization.ProjectionVersion,
			CanonicalSHA256: strings.Repeat("0", 64),
			Locale:          localization.LocaleRussian,
			Translations: map[string]string{
				input.Fields[0].ID: "Это не должно примениться.",
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Fallback ||
		len(result.Diagnostics) != 1 ||
		result.Diagnostics[0].Code != "canonical_hash_mismatch" {
		t.Fatalf("envelope failure result = %#v", result)
	}
	if after := marshalArchitectureLocalizationFixture(t, projected); !bytes.Equal(before, after) {
		t.Fatalf("invalid envelope changed Architecture Canvas:\n%s\n%s", before, after)
	}
	if after := marshalArchitectureLocalizationFixture(t, canvas); !bytes.Equal(before, after) {
		t.Fatal("projection mutated its Architecture Canvas input")
	}
}

func TestArchitectureLocalizationFallsBackPerFieldWithoutMutatingInput(t *testing.T) {
	t.Parallel()

	canvas := architectureLocalizationFixture()
	before := marshalArchitectureLocalizationFixture(t, canvas)
	canonical, input, err := buildArchitectureLocalization(canvas, localization.LocaleRussian)
	if err != nil {
		t.Fatal(err)
	}
	translations := make(map[string]string, len(input.Fields))
	for _, field := range input.Fields {
		translations[field.ID] = "Переведено: " + field.Text
	}
	invalidID, err := localization.FieldID(
		localization.OwnerComponent,
		string(canvas.Components[0].ID),
		localization.FieldDescription,
	)
	if err != nil {
		t.Fatal(err)
	}
	translations[invalidID] = "Перевод потерял обязательные технические placeholders."

	projected, result, err := applyArchitectureLocalization(
		canvas,
		canonical,
		input,
		localization.Projection{
			Version:         localization.ProjectionVersion,
			CanonicalSHA256: canonical.SHA256,
			Locale:          localization.LocaleRussian,
			Translations:    translations,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Fallback ||
		len(result.Diagnostics) != 1 ||
		result.Diagnostics[0].Code != "placeholder_mismatch" ||
		result.Diagnostics[0].FieldID != invalidID {
		t.Fatalf("field fallback result = %#v", result)
	}
	if projected.Components[0].Description != canvas.Components[0].Description {
		t.Fatalf("invalid field did not fall back to canonical English: %q", projected.Components[0].Description)
	}
	if !strings.HasPrefix(projected.Components[0].Name, "Переведено: ") {
		t.Fatalf("valid Russian field was discarded: %q", projected.Components[0].Name)
	}
	if after := marshalArchitectureLocalizationFixture(t, canvas); !bytes.Equal(before, after) {
		t.Fatal("field fallback mutated its Architecture Canvas input")
	}
}

func TestArchitectureLocalizationRejectsStaleCanonicalForSameOwners(t *testing.T) {
	t.Parallel()

	firstCanvas := architectureLocalizationFixture()
	canonical, input, err := buildArchitectureLocalization(firstCanvas, localization.LocaleRussian)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := localization.IdentityProjection(canonical)
	if err != nil {
		t.Fatal(err)
	}

	currentCanvas := architectureLocalizationFixture()
	currentCanvas.Subsystems[0].Name = "Current subsystem prose"
	currentCanvas.Components[0].Description = "Current component prose"
	before := marshalArchitectureLocalizationFixture(t, currentCanvas)
	projected, _, err := applyArchitectureLocalization(
		currentCanvas,
		canonical,
		input,
		projection,
	)
	if err == nil || !strings.Contains(err.Error(), "does not match current canvas") {
		t.Fatalf("stale canonical error = %v", err)
	}
	if after := marshalArchitectureLocalizationFixture(t, projected); !bytes.Equal(before, after) {
		t.Fatalf("stale canonical changed current canvas:\n%s\n%s", before, after)
	}
}

func TestArchitectureLocalizationRejectsDuplicateSemanticOwner(t *testing.T) {
	t.Parallel()

	canvas := architectureLocalizationFixture()
	canvas.Components = append(canvas.Components, canvas.Components[0])
	if _, _, err := buildArchitectureLocalization(canvas, localization.LocaleRussian); err == nil {
		t.Fatal("duplicate component ID was accepted as a localization owner")
	}
}

func architectureLocalizationFixture() ArchitectureCanvas {
	location := evidence.Location{Path: "cmd/сервер main.go", Line: 42}
	return ArchitectureCanvas{
		Version:             ArchitectureCanvasVersion,
		LandscapeVersion:    componentmap.ContractVersion,
		FlowProofVersion:    1,
		ValidationOutcome:   componentmap.ValidationAccepted,
		ArchitectureSource:  componentmap.SourceValidatedModel,
		ArchitectureLevel:   2,
		RepositoryArchetype: componentmap.ArchetypeLibraryFramework,
		GroundingMode:       componentmap.GroundingBehavior,
		Title:               "Architecture title remains outside this slice",
		Subtitle:            "Architecture subtitle remains outside this slice",
		Subsystems: []ArchitectureSubsystem{{
			ID:           componentmap.SubsystemID("subsystem-member-set-01"),
			Name:         "PostgreSQL storage",
			Description:  "source-subsystem-01 coordinates StartReplication from cmd/сервер main.go.",
			ComponentIDs: []componentmap.ComponentID{"component-member-set-01"},
			SourceIDs:    []componentmap.SubsystemID{"source-subsystem-01"},
		}},
		Components: []ArchitectureComponent{{
			ID:          componentmap.ComponentID("component-member-set-01"),
			SubsystemID: componentmap.SubsystemID("subsystem-member-set-01"),
			Name:        "StartReplication API/v2",
			Description: "Calls StartReplication through API/v2 and github.com/example/storage from cmd/сервер main.go; flow-01, surface-01, participating-surface-01, investigation-01, anchor-01, and source-component-01 remain exact; 東京 remains readable.",
			Members: []componentmap.Candidate{{
				ID:   componentmap.MemberID{Kind: componentmap.MemberSymbol, Value: "StartReplication"},
				Name: "StartReplication",
				Participations: []componentmap.FlowParticipation{{
					FlowID: "flow-01",
					Evidence: componentmap.LocalFact{
						Kind:      componentmap.FactFlowParticipation,
						Value:     "flow-01",
						Location:  &location,
						Certainty: evidence.CertaintyStatic,
					},
				}},
				Facts: []componentmap.LocalFact{
					{
						Kind:      componentmap.FactRepositoryPath,
						Value:     "cmd/сервер main.go",
						Location:  &location,
						Certainty: evidence.CertaintyStatic,
					},
					{
						Kind:      componentmap.FactDeclaration,
						Value:     "API/v2",
						Location:  &location,
						Certainty: evidence.CertaintyStatic,
					},
					{
						Kind:      componentmap.FactDeclaration,
						Value:     "PostgreSQL",
						Location:  &location,
						Certainty: evidence.CertaintyStatic,
					},
					{
						Kind:      componentmap.FactContainment,
						Value:     "github.com/example/storage",
						Location:  &location,
						Certainty: evidence.CertaintyStatic,
					},
				},
			}},
			ParticipatingFlowIDs:      []componentmap.FlowID{"flow-01"},
			OwnedSurfaceIDs:           []string{"surface-01"},
			ParticipatingSurfaceIDs:   []string{"participating-surface-01"},
			SuggestedInvestigationIDs: []string{"investigation-01"},
			AnchorIDs:                 []string{"anchor-01"},
			Hypothesis:                true,
			SourceIDs:                 []componentmap.ComponentID{"source-component-01"},
		}},
		StructuralFacts: []componentmap.LocalRelation{{
			ID:        "relation-01",
			From:      componentmap.MemberID{Kind: componentmap.MemberSymbol, Value: "StartReplication"},
			To:        componentmap.MemberID{Kind: componentmap.MemberPackage, Value: "github.com/example/storage"},
			Kind:      componentmap.StructuralRelationBehaviorHandoff,
			Location:  &location,
			Certainty: evidence.CertaintyStatic,
		}},
		Diagnostics: []ArchitectureDiagnostic{{
			ID:       "diagnostic-01",
			Source:   "fixture",
			Severity: "advisory",
			Code:     "fixture.code",
			Message:  "Diagnostic prose remains outside this slice.",
		}},
	}
}

func marshalArchitectureLocalizationFixture(t *testing.T, canvas ArchitectureCanvas) []byte {
	t.Helper()
	encoded, err := json.Marshal(canvas)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func roundTripArchitectureLocalizationJSON(t *testing.T, value any) {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(encoded, value); err != nil {
		t.Fatal(err)
	}
}
