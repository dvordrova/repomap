package report

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/localization"
)

func TestArchitecturePresentationLocalizationScopesOpaqueFlowValuesAtomically(
	t *testing.T,
) {
	t.Parallel()

	canonical := &ReportData{
		RepoName:     "fixture",
		ProjectGuess: "The domain remains understandable.",
		ArchitectureCanvas: &ArchitectureCanvas{
			Version: ArchitectureCanvasVersion,
			Flows: []ArchitectureFlow{{
				ID:      componentmap.FlowID("flow-serve"),
				Name:    "Server startup path",
				Command: "serve --config local",
				Trigger: "Call main through serve --config local before startup.",
				Steps: []ArchitectureFlowStep{{
					ID:            "step-main",
					Label:         "Enter main before running serve --config local.",
					QualifiedName: "main",
				}},
			}},
		},
	}
	before, err := json.Marshal(canonical)
	if err != nil {
		t.Fatal(err)
	}

	prepared, err := PreparePresentationLocalization(
		canonical,
		localization.LocaleRussian,
	)
	if err != nil {
		t.Fatal(err)
	}

	triggerField := presentationCanonicalFieldByOwner(
		t,
		prepared,
		"architecture/flows/flow-serve/trigger",
	)
	mainToken := presentationProtectedToken(t, triggerField, "main")
	commandToken := presentationProtectedToken(
		t,
		triggerField,
		"serve --config local",
	)
	stepField := presentationCanonicalFieldByOwner(
		t,
		prepared,
		"architecture/flows/flow-serve/steps/step-main/label",
	)
	stepMainToken := presentationProtectedToken(t, stepField, "main")
	stepCommandToken := presentationProtectedToken(
		t,
		stepField,
		"serve --config local",
	)
	triggerInput := presentationInputTextByID(t, prepared, triggerField.ID)
	for _, opaque := range []string{"main", "serve --config local"} {
		if strings.Contains(triggerInput, opaque) {
			t.Fatalf("translation input exposed Architecture identity %q: %q", opaque, triggerInput)
		}
	}
	for _, token := range []string{mainToken, commandToken} {
		if !strings.Contains(triggerInput, token) {
			t.Fatalf("translation input lost Architecture placeholder %q: %q", token, triggerInput)
		}
	}

	repositoryField := presentationCanonicalFieldByText(
		t,
		prepared,
		canonical.ProjectGuess,
	)
	if !strings.Contains(repositoryField.Text, "domain") {
		t.Fatalf("repository fixture no longer exercises domain prose: %#v", repositoryField)
	}
	for _, term := range repositoryField.ProtectedTerms {
		if term.Value == "main" {
			t.Fatalf("Architecture symbol main leaked into unrelated domain prose: %#v", repositoryField)
		}
	}
	if got := presentationInputTextByID(t, prepared, repositoryField.ID); got != repositoryField.Text {
		t.Fatalf("unrelated domain prose was unexpectedly hidden: got %q want %q", got, repositoryField.Text)
	}

	incomplete := russianPresentationProjection(prepared)
	delete(incomplete.Translations, triggerField.ID)
	projected, result, err := ApplyPresentationLocalization(
		canonical,
		prepared,
		incomplete,
	)
	if err == nil || projected != nil || !result.Fallback {
		t.Fatalf(
			"incomplete Architecture projection was not rejected atomically: projected=%#v result=%#v err=%v",
			projected,
			result,
			err,
		)
	}
	afterRejected, err := json.Marshal(canonical)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, afterRejected) {
		t.Fatal("rejected Architecture projection mutated canonical report")
	}

	projection := russianPresentationProjection(prepared)
	wantTrigger := "Вызвать " + mainToken + " через " + commandToken + " перед запуском."
	projection.Translations[triggerField.ID] = wantTrigger
	projection.Translations[stepField.ID] = "Войти в " + stepMainToken +
		" перед запуском " + stepCommandToken + "."
	projected, result, err = ApplyPresentationLocalization(
		canonical,
		prepared,
		projection,
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Fallback {
		t.Fatalf("complete Architecture projection fell back: %#v", result)
	}
	if got := projected.ArchitectureCanvas.Flows[0].Trigger; got !=
		"Вызвать main через serve --config local перед запуском." {
		t.Fatalf("projected Architecture trigger = %q", got)
	}
	if got := projected.ArchitectureCanvas.Flows[0].Steps[0].Label; got !=
		"Войти в main перед запуском serve --config local." {
		t.Fatalf("projected Architecture step = %q", got)
	}
	if got := projected.ProjectGuess; got != "Русский текст" {
		t.Fatalf("unrelated domain prose was not translated: %q", got)
	}
	if projected.ArchitectureCanvas.Flows[0].Command != "serve --config local" ||
		projected.ArchitectureCanvas.Flows[0].Steps[0].QualifiedName != "main" {
		t.Fatalf(
			"projection changed Architecture identities: %#v",
			projected.ArchitectureCanvas.Flows[0],
		)
	}
	afterAccepted, err := json.Marshal(canonical)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, afterAccepted) {
		t.Fatal("accepted Architecture projection mutated canonical report")
	}
}

func TestArchitecturePresentationInventoryCoversRenderedAnchorAndMemberProse(
	t *testing.T,
) {
	t.Parallel()

	location := evidence.Location{Path: "cmd/server/main.go", Line: 42}
	symbolID := componentmap.MemberID{
		Kind: componentmap.MemberSymbol, Value: "member-entry-opaque-id",
	}
	flowID := componentmap.MemberID{
		Kind: componentmap.MemberFlow, Value: "member-flow-opaque-id",
	}
	fileID := componentmap.MemberID{
		Kind: componentmap.MemberFile, Value: "member-file-opaque-id",
	}
	canonical := &ReportData{
		RepoName: "fixture",
		ArchitectureCanvas: &ArchitectureCanvas{
			Version: ArchitectureCanvasVersion,
			BehaviorAnchors: []componentmap.BehaviorAnchor{{
				ID:        "anchor-process-entry",
				Kind:      componentmap.AnchorProcessEntry,
				Label:     "process entry main",
				Location:  location,
				ProofMode: componentmap.AnchorProofProcessEntry,
				Scenario:  componentmap.ScenarioContext{ID: "scenario-product"},
				MemberIDs: []componentmap.MemberID{
					symbolID,
				},
			}},
			Components: []ArchitectureComponent{{
				ID:          "component-server",
				Name:        "Server component",
				Description: "The Start server through main path is central.",
				AnchorIDs:   []string{"anchor-process-entry"},
				Members: []componentmap.Candidate{
					{
						ID:   symbolID,
						Name: "Start server through main",
						Facts: []componentmap.LocalFact{{
							Kind: componentmap.FactDeclaration, Value: "main",
							Location: &location, Certainty: evidence.CertaintyStatic,
						}},
					},
					{
						ID:   flowID,
						Name: "Durable replication flow around StartReplication",
						Facts: []componentmap.LocalFact{
							{
								Kind:      componentmap.FactFlowParticipation,
								Value:     "direction-replication",
								Certainty: evidence.CertaintyStatic,
							},
							{
								Kind:      componentmap.FactDeclaration,
								Value:     "StartReplication",
								Location:  &location,
								Certainty: evidence.CertaintyStatic,
							},
						},
					},
					{
						ID: fileID, Name: location.Path,
						Facts: []componentmap.LocalFact{{
							Kind: componentmap.FactRepositoryPath, Value: location.Path,
							Location: &location, Certainty: evidence.CertaintyStatic,
						}},
					},
				},
			}},
		},
	}
	before, err := json.Marshal(canonical)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := PreparePresentationLocalization(
		canonical,
		localization.LocaleRussian,
	)
	if err != nil {
		t.Fatal(err)
	}

	anchorField := presentationCanonicalFieldByOwner(
		t,
		prepared,
		"architecture/behavior_anchors/anchor-process-entry/label",
	)
	symbolOwner := "architecture/components/component-server/members/" +
		presentationOwnerDigest(string(symbolID.Kind), symbolID.Value) + "/name"
	symbolField := presentationCanonicalFieldByOwner(t, prepared, symbolOwner)
	flowOwner := "architecture/components/component-server/members/" +
		presentationOwnerDigest(string(flowID.Kind), flowID.Value) + "/name"
	flowField := presentationCanonicalFieldByOwner(t, prepared, flowOwner)
	componentDescriptionField := presentationCanonicalFieldByText(
		t,
		prepared,
		canonical.ArchitectureCanvas.Components[0].Description,
	)
	mainAnchorToken := presentationProtectedToken(t, anchorField, "main")
	mainMemberToken := presentationProtectedToken(t, symbolField, "main")
	startReplicationToken := presentationProtectedToken(
		t,
		flowField,
		"StartReplication",
	)
	for _, check := range []struct {
		field  localization.CanonicalField
		prose  string
		opaque string
	}{
		{anchorField, "process entry", "main"},
		{symbolField, "Start server through", "main"},
		{flowField, "Durable replication flow around", "StartReplication"},
		{componentDescriptionField, "Start server through", "main"},
	} {
		input := presentationInputTextByID(t, prepared, check.field.ID)
		if !strings.Contains(input, check.prose) || strings.Contains(input, check.opaque) {
			t.Fatalf(
				"Architecture presentation classification for %q = %q",
				check.field.OwnerID,
				input,
			)
		}
	}
	for _, field := range prepared.Canonical.Fields {
		if field.Text == location.Path {
			t.Fatalf("exact file member name entered prose inventory: %#v", field)
		}
	}

	incomplete := russianPresentationProjection(prepared)
	delete(incomplete.Translations, anchorField.ID)
	projected, result, err := ApplyPresentationLocalization(
		canonical,
		prepared,
		incomplete,
	)
	if err == nil || projected != nil || !result.Fallback {
		t.Fatalf(
			"incomplete Architecture projection was not rejected atomically: projected=%#v result=%#v err=%v",
			projected,
			result,
			err,
		)
	}
	afterRejected, err := json.Marshal(canonical)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, afterRejected) {
		t.Fatal("rejected Architecture projection mutated canonical report")
	}

	projection := russianPresentationProjection(prepared)
	projection.Translations[anchorField.ID] = "точка входа " + mainAnchorToken
	projection.Translations[symbolField.ID] = "Запустить сервер через " + mainMemberToken
	projection.Translations[flowField.ID] = "Путь долговечной репликации вокруг " +
		startReplicationToken
	projected, result, err = ApplyPresentationLocalization(
		canonical,
		prepared,
		projection,
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Fallback || len(result.Diagnostics) != 0 {
		t.Fatalf("complete Architecture projection result = %#v", result)
	}
	if got := projected.ArchitectureCanvas.BehaviorAnchors[0].Label; got != "точка входа main" {
		t.Fatalf("projected behavior anchor label = %q", got)
	}
	if got := projected.ArchitectureCanvas.Components[0].Members[0].Name; got != "Запустить сервер через main" {
		t.Fatalf("projected entrypoint member name = %q", got)
	}
	if got := projected.ArchitectureCanvas.Components[0].Members[1].Name; got !=
		"Путь долговечной репликации вокруг StartReplication" {
		t.Fatalf("projected flow member name = %q", got)
	}
	if got := projected.ArchitectureCanvas.Components[0].Members[2].Name; got != location.Path {
		t.Fatalf("projected file member name = %q", got)
	}
	html, err := RenderHTML(projected)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"process entry main",
		"Start server through main",
		"Durable replication flow around StartReplication",
	} {
		if bytes.Contains(html, []byte(forbidden)) {
			t.Errorf("English Architecture prose survived RU render: %q", forbidden)
		}
	}
	for _, opaque := range []string{"main", "StartReplication", location.Path} {
		if !bytes.Contains(html, []byte(opaque)) {
			t.Errorf("Architecture opaque identity changed or disappeared: %q", opaque)
		}
	}
	afterAccepted, err := json.Marshal(canonical)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, afterAccepted) {
		t.Fatal("accepted Architecture projection mutated canonical report")
	}
}

func presentationCanonicalFieldByOwner(
	t *testing.T,
	prepared PreparedPresentationLocalization,
	owner string,
) localization.CanonicalField {
	t.Helper()

	for _, field := range prepared.Canonical.Fields {
		if field.OwnerID == owner {
			return field
		}
	}
	t.Fatalf("presentation owner %q is absent", owner)
	return localization.CanonicalField{}
}

func presentationCanonicalFieldByText(
	t *testing.T,
	prepared PreparedPresentationLocalization,
	text string,
) localization.CanonicalField {
	t.Helper()

	for _, field := range prepared.Canonical.Fields {
		if field.Text == text {
			return field
		}
	}
	t.Fatalf("presentation text %q is absent", text)
	return localization.CanonicalField{}
}

func presentationProtectedToken(
	t *testing.T,
	field localization.CanonicalField,
	value string,
) string {
	t.Helper()

	for _, term := range field.ProtectedTerms {
		if term.Value == value {
			return term.Token
		}
	}
	t.Fatalf("presentation field %q does not protect %q: %#v", field.ID, value, field.ProtectedTerms)
	return ""
}

func presentationInputTextByID(
	t *testing.T,
	prepared PreparedPresentationLocalization,
	id string,
) string {
	t.Helper()

	for _, field := range prepared.Input.Fields {
		if field.ID == id {
			return field.Text
		}
	}
	t.Fatalf("presentation input field %q is absent", id)
	return ""
}
