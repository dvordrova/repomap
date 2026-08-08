package report

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/flowexplain"
	"github.com/dvordrova/repomap/internal/flowproof"
	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/guidedtour"
	"github.com/dvordrova/repomap/internal/llmbundle"
	"github.com/dvordrova/repomap/internal/localization"
	"github.com/dvordrova/repomap/internal/modelresearch"
	"github.com/dvordrova/repomap/internal/orient"
)

const (
	selectedResearchRoundReasonCode = "new_exact_evidence_and_high_value_frontier"
	skippedResearchRoundReasonCode  = "runtime_only_frontier"
)

func TestPresentationLocalizationRussianProjectionChangesOnlyInventoriedProse(t *testing.T) {
	t.Parallel()

	canonical := presentationLocalizationFixture()
	prepared, err := PreparePresentationLocalization(canonical, localization.LocaleRussian)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Input.SourceLocale != localization.LocaleEnglish ||
		prepared.Input.TargetLocale != localization.LocaleRussian ||
		len(prepared.Input.Fields) == 0 {
		t.Fatalf("translation input = %#v", prepared.Input)
	}
	inputJSON, err := json.Marshal(prepared.Input)
	if err != nil {
		t.Fatal(err)
	}
	for _, opaque := range []string{
		"internal/storage/replicate.go",
		canonical.ArchitectureCanvas.Title,
		canonical.ArchitectureCanvas.Subtitle,
	} {
		if bytes.Contains(inputJSON, []byte(opaque)) {
			t.Fatalf("translation input exposed opaque value %q: %s", opaque, inputJSON)
		}
	}
	if !bytes.Contains(inputJSON, []byte(`{{term_`)) {
		t.Fatalf("translation input did not replace protected values: %s", inputJSON)
	}

	projected, result, err := ApplyPresentationLocalization(
		canonical,
		prepared,
		russianPresentationProjection(prepared),
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Fallback || len(result.Diagnostics) != 0 ||
		result.Locale != localization.LocaleRussian ||
		projected.ReportLanguage != localization.LocaleRussian {
		t.Fatalf("Russian projection result = %#v, language = %q", result, projected.ReportLanguage)
	}
	for name, text := range map[string]string{
		"repository project":       projected.ProjectGuess,
		"subsystem name":           projected.ArchitectureCanvas.Subsystems[0].Name,
		"component description":    projected.ArchitectureCanvas.Components[0].Description,
		"study question":           projected.StudyMap.Directions[0].Question,
		"study reading guidance":   projected.StudyMap.Directions[0].ReadingAnchors[0].WhatToLookFor,
		"mechanism title":          projected.UserMechanisms[0].Title,
		"mechanism step narrative": projected.UserMechanisms[0].Steps[0].Explanation,
		"implementation title": projected.UserMechanisms[0].Phases[0].
			ImplementationDetails[0].Title,
		"implementation explanation": projected.UserMechanisms[0].Phases[0].
			ImplementationDetails[0].Explanation,
	} {
		if !strings.HasPrefix(text, "Русский текст") {
			t.Errorf("%s was not translated: %q", name, text)
		}
	}
	if projected.StudyMap.Directions[0].SearchQueries[0] !=
		canonical.StudyMap.Directions[0].SearchQueries[0] {
		t.Fatalf(
			"opaque study search query changed: got %q want %q",
			projected.StudyMap.Directions[0].SearchQueries[0],
			canonical.StudyMap.Directions[0].SearchQueries[0],
		)
	}
	if projected.ArchitectureCanvas.Title != canonical.ArchitectureCanvas.Title ||
		projected.ArchitectureCanvas.Subtitle != canonical.ArchitectureCanvas.Subtitle {
		t.Fatalf(
			"product-owned canvas copy entered the prose inventory: %q / %q",
			projected.ArchitectureCanvas.Title,
			projected.ArchitectureCanvas.Subtitle,
		)
	}

	beforeOpaque := marshalPresentationDataWithCanonicalProse(t, canonical, prepared)
	afterOpaque := marshalPresentationDataWithCanonicalProse(t, projected, prepared)
	if !bytes.Equal(beforeOpaque, afterOpaque) {
		t.Fatalf(
			"projection changed IDs, paths, evidence, source, links, or other opaque bytes:\n%s\n%s",
			beforeOpaque,
			afterOpaque,
		)
	}
}

func TestPresentationTextInventoryIncludesVisibleFlowError(t *testing.T) {
	t.Parallel()

	const canonicalError = "The provider rejected this saved flow."
	canonical := &ReportData{
		FormatVersion: CurrentFormatVersion,
		RepoName:      "fixture-repo",
		Flows: []FlowData{{
			ID:    "failed-flow",
			Error: canonicalError,
		}},
	}
	prepared, err := PreparePresentationLocalization(
		canonical,
		localization.LocaleRussian,
	)
	if err != nil {
		t.Fatal(err)
	}

	const fieldID = "presentation_text:flows/failed-flow/error.text"
	found := false
	for _, field := range prepared.Input.Fields {
		if field.ID != fieldID {
			continue
		}
		found = true
		if field.Text != canonicalError {
			t.Fatalf("flow error input = %q, want %q", field.Text, canonicalError)
		}
	}
	if !found {
		t.Fatalf("presentation inventory has no %q field", fieldID)
	}

	projected, _, err := ApplyPresentationLocalization(
		canonical,
		prepared,
		russianPresentationProjection(prepared),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(projected.Flows[0].Error, "Русский текст") {
		t.Fatalf("projected flow error = %q", projected.Flows[0].Error)
	}
	if canonical.Flows[0].Error != canonicalError {
		t.Fatalf("canonical flow error changed to %q", canonical.Flows[0].Error)
	}
}

func TestPresentationTextInventoryLocalizesSavedDirectionProofProse(t *testing.T) {
	t.Parallel()

	canonical := &ReportData{
		FormatVersion: CurrentFormatVersion,
		RepoName:      "fixture-repo",
		CandidateDirections: []CandidateDirection{
			{
				ID:               "scheduler",
				LikelyEntrypoint: "StartScheduler",
				LikelyFiles: []string{
					"internal/scheduler/start.go",
					"internal/scheduler/stop.go",
				},
				LocalVerification: &flowexplain.FlowVerification{
					Status:   "partial",
					Verified: []string{"Readers start at StartScheduler in internal/scheduler/start.go."},
					Missing:  []string{"StopScheduler in internal/scheduler/stop.go remains unresolved."},
				},
				LocalProof: &flowproof.Session{
					Version: flowproof.SessionVersion,
					Proof: flowproof.Proof{
						Version: flowproof.Version,
						ID:      "proof-scheduler",
						Command: "serve",
						Slots: []flowproof.Slot{{
							Kind:    flowproof.SlotEntrypoint,
							Status:  flowproof.SlotPartial,
							Summary: "StartScheduler starts the scheduler loop.",
							Missing: "StopScheduler shutdown remains unverified.",
						}},
						Anchors: []flowproof.Anchor{
							{
								ID:            "anchor-start",
								Kind:          flowproof.AnchorMethod,
								Label:         "StartScheduler",
								QualifiedName: "(*Scheduler).StartScheduler",
								Location: &evidence.Location{
									Path: "internal/scheduler/start.go",
									Line: 17,
								},
							},
							{
								ID:            "anchor-stop",
								Kind:          flowproof.AnchorMethod,
								Label:         "StopScheduler",
								QualifiedName: "(*Scheduler).StopScheduler",
								Location: &evidence.Location{
									Path: "internal/scheduler/stop.go",
									Line: 29,
								},
							},
						},
						CurrentFrontier: "Continue from StopScheduler in internal/scheduler/stop.go.",
					},
					Stop: &flowproof.Stop{
						Reason:  flowproof.StopNoProgress,
						Message: "Inspection stopped before StopScheduler was verified.",
					},
				},
			},
			{
				ID:               "unrelated",
				LikelyEntrypoint: "start",
			},
		},
	}
	canonicalBefore, err := json.Marshal(canonical)
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

	expectedFieldIDs := map[string]bool{
		"presentation_text:orientation/directions/scheduler/local_verification/verified/0.text":        false,
		"presentation_text:orientation/directions/scheduler/local_verification/missing/0.text":         false,
		"presentation_text:orientation/directions/scheduler/local_proof/slots/entrypoint/summary.text": false,
		"presentation_text:orientation/directions/scheduler/local_proof/slots/entrypoint/missing.text": false,
		"presentation_text:orientation/directions/scheduler/local_proof/stop/message.text":             false,
		"presentation_text:orientation/directions/scheduler/local_proof/current_frontier.text":         false,
	}
	for _, field := range prepared.Input.Fields {
		if _, expected := expectedFieldIDs[field.ID]; expected {
			expectedFieldIDs[field.ID] = true
		}
		if field.ID == "presentation_text:orientation/directions/scheduler/local_verification/verified/0.text" {
			if !strings.Contains(field.Text, "Readers start at") {
				t.Fatalf("object-scoped input over-protected unrelated symbol start: %q", field.Text)
			}
			for _, opaque := range []string{"StartScheduler", "internal/scheduler/start.go"} {
				if strings.Contains(field.Text, opaque) {
					t.Fatalf("exact direction identity %q was not protected: %q", opaque, field.Text)
				}
			}
		}
		for _, forbiddenSuffix := range []string{"/status.text", "/reason.text", "/kind.text"} {
			if strings.HasSuffix(field.ID, forbiddenSuffix) {
				t.Fatalf("enum or reason code entered prose inventory: %q", field.ID)
			}
		}
	}
	for fieldID, found := range expectedFieldIDs {
		if !found {
			t.Errorf("presentation inventory has no %q field", fieldID)
		}
	}

	projected, _, err := ApplyPresentationLocalization(
		canonical,
		prepared,
		russianPresentationProjection(prepared),
	)
	if err != nil {
		t.Fatal(err)
	}
	direction := projected.CandidateDirections[0]
	for name, text := range map[string]string{
		"verified statement": direction.LocalVerification.Verified[0],
		"missing statement":  direction.LocalVerification.Missing[0],
		"slot summary":       direction.LocalProof.Proof.Slots[0].Summary,
		"slot missing":       direction.LocalProof.Proof.Slots[0].Missing,
		"stop message":       direction.LocalProof.Stop.Message,
		"current frontier":   direction.LocalProof.Proof.CurrentFrontier,
	} {
		if !strings.HasPrefix(text, "Русский текст") {
			t.Errorf("%s was not localized: %q", name, text)
		}
	}
	for name, text := range map[string]string{
		"verified symbol": direction.LocalVerification.Verified[0],
		"verified path":   direction.LocalVerification.Verified[0],
		"missing symbol":  direction.LocalVerification.Missing[0],
		"missing path":    direction.LocalVerification.Missing[0],
	} {
		want := map[string]string{
			"verified symbol": "StartScheduler",
			"verified path":   "internal/scheduler/start.go",
			"missing symbol":  "StopScheduler",
			"missing path":    "internal/scheduler/stop.go",
		}[name]
		if !strings.Contains(text, want) {
			t.Errorf("%s %q was not preserved in %q", name, want, text)
		}
	}
	if direction.LocalVerification.Status != "partial" ||
		direction.LocalProof.Proof.Slots[0].Kind != flowproof.SlotEntrypoint ||
		direction.LocalProof.Proof.Slots[0].Status != flowproof.SlotPartial ||
		direction.LocalProof.Stop.Reason != flowproof.StopNoProgress {
		t.Fatalf("opaque proof enums changed: %#v", direction)
	}
	canonicalAfter, err := json.Marshal(canonical)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(canonicalBefore, canonicalAfter) {
		t.Fatal("RU proof projection mutated the canonical candidate direction")
	}
}

func TestPresentationLocalizationAcceptsCanonicalEnglishEchoStructurally(t *testing.T) {
	t.Parallel()

	canonical := presentationLocalizationFixture()
	prepared, err := PreparePresentationLocalization(canonical, localization.LocaleRussian)
	if err != nil {
		t.Fatal(err)
	}
	translations := make(map[string]string, len(prepared.Input.Fields))
	for _, field := range prepared.Input.Fields {
		translations[field.ID] = field.Text
	}
	projection := localization.Projection{
		Version:         localization.ProjectionVersion,
		CanonicalSHA256: prepared.Canonical.SHA256,
		Locale:          localization.LocaleRussian,
		Translations:    translations,
	}
	projected, result, err := ApplyPresentationLocalization(
		canonical,
		prepared,
		projection,
	)
	if err != nil || projected == nil ||
		result.Fallback || len(result.Diagnostics) != 0 {
		t.Fatalf(
			"structurally valid English echo was rejected: projected=%#v result=%#v err=%v",
			projected,
			result,
			err,
		)
	}
	if projected.ReportLanguage != localization.LocaleRussian {
		t.Fatalf("report language = %q", projected.ReportLanguage)
	}
	if err := WritePresentationLocalizationSuccess(
		t.TempDir(),
		prepared,
		projection,
		false,
		"request-sha",
		"cache-key",
	); err != nil {
		t.Fatalf("persist structurally valid English echo: %v", err)
	}
}

func TestPresentationLocalizationAcceptsPartiallyEnglishProjectionStructurally(t *testing.T) {
	t.Parallel()

	canonical := presentationLocalizationFixture()
	prepared, err := PreparePresentationLocalization(canonical, localization.LocaleRussian)
	if err != nil {
		t.Fatal(err)
	}
	projection := russianPresentationProjection(prepared)
	var replaced bool
	for _, field := range prepared.Input.Fields {
		if field.ID != "presentation_text:repository/repository/project_guess.text" {
			continue
		}
		projection.Translations[field.ID] = field.Text
		replaced = true
		break
	}
	if !replaced {
		t.Fatal("fixture has no repository project-guess field")
	}

	projected, result, err := ApplyPresentationLocalization(
		canonical,
		prepared,
		projection,
	)
	if err != nil || projected == nil ||
		result.Fallback || len(result.Diagnostics) != 0 {
		t.Fatalf(
			"structurally valid partially English projection was rejected: projected=%#v result=%#v err=%v",
			projected,
			result,
			err,
		)
	}
	if err := WritePresentationLocalizationSuccess(
		t.TempDir(),
		prepared,
		projection,
		false,
		"request-sha",
		"cache-key",
	); err != nil {
		t.Fatalf("persist structurally valid partially English projection: %v", err)
	}
}

func TestPresentationLocalizationMechanismPhaseImplementationDetailsAreProseOnly(t *testing.T) {
	t.Parallel()

	canonical := presentationLocalizationFixture()
	before := canonical.UserMechanisms[0].Phases[0].ImplementationDetails[0]
	prepared, err := PreparePresentationLocalization(canonical, localization.LocaleRussian)
	if err != nil {
		t.Fatal(err)
	}

	var detailFields []localization.CanonicalField
	for _, field := range prepared.Canonical.Fields {
		if field.OwnerKind == localization.OwnerPresentationText &&
			field.Name == localization.FieldText &&
			strings.Contains(field.OwnerID, ":implementation-detail:") {
			detailFields = append(detailFields, field)
		}
	}
	if len(detailFields) != 2 {
		t.Fatalf("implementation detail fields = %#v", detailFields)
	}
	addressSuffixes := map[string]bool{}
	for _, field := range detailFields {
		addressSuffixes[field.OwnerID[strings.LastIndex(field.OwnerID, "/")+1:]] = true
	}
	if !addressSuffixes[string(localization.FieldTitle)] ||
		!addressSuffixes[string(localization.FieldExplanation)] {
		t.Fatalf("implementation detail fields = %#v", detailFields)
	}

	repeated, err := PreparePresentationLocalization(canonical, localization.LocaleRussian)
	if err != nil {
		t.Fatal(err)
	}
	var repeatedIDs []string
	for _, field := range repeated.Canonical.Fields {
		if field.OwnerKind == localization.OwnerPresentationText &&
			field.Name == localization.FieldText &&
			strings.Contains(field.OwnerID, ":implementation-detail:") {
			repeatedIDs = append(repeatedIDs, field.ID)
		}
	}
	if !reflect.DeepEqual(repeatedIDs, []string{detailFields[0].ID, detailFields[1].ID}) {
		t.Fatalf("implementation detail owner identity is not stable: %v", repeatedIDs)
	}

	projected, _, err := ApplyPresentationLocalization(
		canonical,
		prepared,
		russianPresentationProjection(prepared),
	)
	if err != nil {
		t.Fatal(err)
	}
	after := projected.UserMechanisms[0].Phases[0].ImplementationDetails[0]
	if !strings.HasPrefix(after.Title, "Русский текст") ||
		!strings.HasPrefix(after.Explanation, "Русский текст") {
		t.Fatalf("implementation detail prose was not translated: %#v", after)
	}
	if len(after.WhatToNotice) != 1 ||
		!strings.HasPrefix(after.WhatToNotice[0].Text, "Русский текст") {
		t.Fatalf("visible implementation notice was not translated: %#v", after.WhatToNotice)
	}
	if !reflect.DeepEqual(before.Locations, after.Locations) ||
		!reflect.DeepEqual(before.Sources, after.Sources) ||
		before.WhatToNotice[0].Path != after.WhatToNotice[0].Path ||
		!reflect.DeepEqual(
			before.WhatToNotice[0].SupportingRanges,
			after.WhatToNotice[0].SupportingRanges,
		) {
		t.Fatalf("implementation detail evidence/navigation changed:\nbefore=%#v\nafter=%#v", before, after)
	}
}

func TestPresentationTextInventoryIsVersionedGenericAndBroad(t *testing.T) {
	t.Parallel()

	canonical := broadPresentationLocalizationFixture()
	prepared, err := PreparePresentationLocalization(
		canonical,
		localization.LocaleRussian,
	)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Version != PresentationTextInventoryVersion ||
		prepared.Version == "" {
		t.Fatalf(
			"inventory version = %q, want %q",
			prepared.Version,
			PresentationTextInventoryVersion,
		)
	}
	if len(prepared.Canonical.Fields) == 0 {
		t.Fatal("presentation text inventory is empty")
	}
	for _, field := range prepared.Canonical.Fields {
		if field.OwnerKind != localization.OwnerPresentationText ||
			field.Name != localization.FieldText ||
			!strings.HasPrefix(field.ID, "presentation_text:") ||
			!strings.HasSuffix(field.ID, ".text") {
			t.Fatalf("non-generic presentation field = %#v", field)
		}
	}

	for category, prose := range map[string]string{
		"repository guide purpose":      "Guide the reader through replication.",
		"repository thesis story":       "A request crosses the replication boundary.",
		"first-file reason":             "Begin with the request coordinator.",
		"orientation subsystem":         "Replication coordination",
		"orientation direction trigger": "A client requests a replication update.",
		"flow narrative":                "The request is validated before the durable write.",
		"architecture suggestion":       "Inspect the durability boundary",
		"architecture flow":             "Replication write path",
		"guided-tour summary":           "Follow the accepted replication story.",
		"study question":                "How does StartReplication update storage?",
		"mechanism answer":              "It validates the request and writes durable state.",
		"user-topic explanation":        "The exact storage effect is not established yet.",
		"warning presentation":          "Evidence remains bounded.",
	} {
		var matches int
		for _, field := range prepared.Canonical.Fields {
			if field.Text == prose {
				matches++
			}
		}
		if matches != 1 {
			t.Errorf(
				"%s prose appears %d times in inventory, want exactly once: %q",
				category,
				matches,
				prose,
			)
		}
	}

	var inputTexts []string
	for _, field := range prepared.Input.Fields {
		inputTexts = append(inputTexts, field.Text)
	}
	joinedInputText := strings.Join(inputTexts, "\n")
	for _, opaque := range []string{
		"direction-replication",
		"flow-local-01",
		"internal/storage/replicate.go",
		"StartReplication storage",
		strings.Repeat("1", 64),
		strings.Repeat("2", 64),
	} {
		if strings.Contains(joinedInputText, opaque) {
			t.Errorf("opaque value entered localization request prose: %q", opaque)
		}
	}
}

func TestPresentationTextInventoryKeepsResearchSelectionReasonsOpaque(
	t *testing.T,
) {
	t.Parallel()

	canonical := broadPresentationLocalizationFixture()
	prepared, err := PreparePresentationLocalization(
		canonical,
		localization.LocaleRussian,
	)
	if err != nil {
		t.Fatal(err)
	}

	for _, field := range prepared.Canonical.Fields {
		if strings.HasSuffix(field.OwnerID, "/selection_reason") ||
			field.Text == selectedResearchRoundReasonCode ||
			field.Text == skippedResearchRoundReasonCode {
			t.Fatalf("opaque selection reason entered localization inventory: %#v", field)
		}
	}

	projection := russianPresentationProjection(prepared)
	projected, result, err := ApplyPresentationLocalization(canonical, prepared, projection)
	if err != nil {
		t.Fatal(err)
	}
	if result.Fallback || len(result.Diagnostics) != 0 ||
		projected.ModelResearch.Rounds[0].SelectionReason != selectedResearchRoundReasonCode ||
		projected.ModelResearch.SkippedRounds[0].SelectionReason != skippedResearchRoundReasonCode {
		t.Fatalf("selection-reason identities changed: research=%#v result=%#v", projected.ModelResearch, result)
	}
	if canonical.ModelResearch.Rounds[0].SelectionReason != selectedResearchRoundReasonCode ||
		canonical.ModelResearch.SkippedRounds[0].SelectionReason != skippedResearchRoundReasonCode {
		t.Fatal("localization mutated canonical research selection reasons")
	}
}

func TestRunWarningLocalizationUsesTypedMessageIDsWithoutEnglishClassifiers(t *testing.T) {
	t.Parallel()

	for _, forbidden := range []string{
		"Study was not published because the editing stage did not finish.",
		"local confidence gate capped",
		"target module requires go",
		"isModelContextWarning",
		"groundingRepairs",
		"confidenceAdjustments",
	} {
		if strings.Contains(scriptJS, forbidden) {
			t.Errorf("run-warning renderer still classifies canonical English text %q", forbidden)
		}
	}
	if !strings.Contains(scriptJS, "fixedMessages.has(presentationMessageID)") {
		t.Fatal("run-warning renderer does not consume typed catalog message IDs")
	}

	const untypedWarning = "parser normalized an unexpected provider field"
	data := &ReportData{
		Warnings: []string{
			studyPublicationWarningEditingDidNotFinish,
			untypedWarning,
		},
		PresentationWarningKinds: []string{
			studyPublicationMessageEditingDidNotFinish,
			"",
		},
	}
	prepared, err := PreparePresentationLocalization(data, localization.LocaleRussian)
	if err != nil {
		t.Fatal(err)
	}
	foundUntyped := false
	for _, field := range prepared.Canonical.Fields {
		if field.Text == studyPublicationWarningEditingDidNotFinish {
			t.Fatal("typed fixed warning entered provider-visible localization inventory")
		}
		if field.Text == untypedWarning {
			foundUntyped = true
		}
	}
	if !foundUntyped {
		t.Fatal("warning without typed structural diagnostics was hidden")
	}

	persisted := reportDataForPersistence(data)
	if !reflect.DeepEqual(persisted.Warnings, data.Warnings) {
		t.Fatalf("canonical warnings changed: got %#v want %#v", persisted.Warnings, data.Warnings)
	}
	if len(persisted.PresentationWarningKinds) != 0 || len(persisted.PresentationWarnings) != 0 {
		t.Fatalf("canonical report retained transient warning presentation: %#v", persisted)
	}
}

func TestPresentationTextInventoryCoversRichTerminalProseAndProtectsOpaqueValues(
	t *testing.T,
) {
	t.Parallel()

	data := broadPresentationLocalizationFixture()
	location := &evidence.Location{
		Path:   "internal/storage/replicate.go",
		Line:   88,
		Column: 3,
	}
	data.CandidateDirections[0].DispositionReason =
		"Retained because the local request boundary is inspectable."
	data.Flows[0].BundleFiles = []FileItem{{
		Path:   "internal/storage/replicate.go",
		Reason: "The bundle starts at the durable request boundary.",
	}}
	data.ArchitectureCanvas.Suggestions[0].UnavailableReason =
		"A complete trace is not available for this suggestion."
	data.ArchitectureCanvas.Suggestions[0].TraceUnavailableReason =
		"The saved trace stops before remote acknowledgement."
	data.ArchitectureCanvas.Flows[0].Steps = []ArchitectureFlowStep{{
		ID:            "architecture-step-validate",
		Label:         "Validate the replication request",
		QualifiedName: "storage.StartReplication",
		Location:      location,
	}}
	data.ArchitectureCanvas.Flows[0].CurrentFrontier =
		"The remote receiver remains beyond the captured trace."
	data.ArchitectureCanvas.Flows[0].Slots = []flowproof.Slot{{
		Kind:    flowproof.SlotIOBoundary,
		Status:  flowproof.SlotPartial,
		Summary: "The local durable write is visible.",
		Missing: "The remote acknowledgement is not established.",
	}}
	data.ArchitectureCanvas.Frontiers = []ArchitectureFrontier{{
		ID:     "architecture-frontier-ack",
		FlowID: data.ArchitectureCanvas.Flows[0].ID,
		Kind:   "remote_acknowledgement",
		Reason: "Remote acknowledgement remains beyond local evidence.",
	}}
	data.ArchitectureCanvas.Diagnostics = []ArchitectureDiagnostic{{
		ID:      "architecture-diagnostic-01",
		Source:  "local_canvas",
		Code:    "trace_partial",
		Message: "The architecture trace is intentionally partial.",
	}}
	data.ArchitectureCanvas.Surfaces = append(
		data.ArchitectureCanvas.Surfaces,
		ArchitectureSurface{
			ID:                     "architecture-surface-replication",
			Name:                   "Replication request surface",
			TraceUnavailableReason: "No accepted trace reaches the remote client.",
			TraceReadinessReason:   "The local handler boundary is available.",
		},
	)
	documentSource := data.UserSources[0]
	documentSource.PresentationSHA256 = strings.Repeat("3", 64)
	documentSource.LandmarkReason = "This document explains the operational boundary."
	data.StudyMap.Directions[0].Documents = []StudyDocumentReference{{
		Label: "Repository replication guide",
		Location: UserCodeLocation{
			Path: "docs/replication.md",
			Line: 12,
		},
		Source: &documentSource,
	}}
	detailSource := data.UserSources[0]
	detailSource.PresentationSHA256 = strings.Repeat("4", 64)
	detailSource.LandmarkReason = "This source shows the implementation detail."
	data.UserMechanisms[0].Phases[0].ImplementationDetails[0].Sources =
		[]SourceSnippet{detailSource}
	operationSource := data.UserSources[0]
	operationSource.PresentationSHA256 = strings.Repeat("5", 64)
	operationSource.LandmarkReason = "This source explains the expected result."
	data.Operations.Paths[0].ExpectedResults = []OperationalResult{{
		Kind:        OperationalResultCommandOutput,
		Value:       "PASS",
		AfterAction: 1,
		Reference: OperationalReference{
			Label: "The repository check reports success.",
			Role:  "expected_result",
			Location: UserCodeLocation{
				Path: "internal/storage/replicate_test.go",
				Line: 55,
			},
			Source: operationSource,
		},
	}}
	data.DiscoveredSurfaces = &DiscoveredSurfaces{
		ScopeStatement: "Runtime surfaces cover the repository-owned request boundary.",
		Triggers: []DiscoveredTrigger{{
			ID:                   "surface-trigger-01",
			UnavailableReason:    "The external dispatcher is not locally available.",
			TraceReadinessReason: "The repository-owned handler is ready to inspect.",
			Evidence: []SurfaceEvidence{{
				ID:     "surface-evidence-01",
				Kind:   "registration",
				Detail: "The handler is registered beside the server startup.",
				Location: &SurfaceLocation{
					Path: "internal/server/routes.go",
					Line: 42,
				},
			}},
			DynamicFrontier: []SurfaceFrontier{{
				Kind:   "custom_dispatch",
				Detail: "Dynamic dispatch remains unresolved.",
			}},
		}},
		LoopSignals: []SurfaceLoopSignal{{
			Kind:       "worker_loop",
			FunctionID: "server.runLoop",
			Detail:     "The worker loop receives repository events.",
		}},
		UnavailablePackages: []SurfacePackageAvailability{{
			Package: "github.com/example/private",
			Reason:  "The package could not be loaded in this workspace.",
		}},
		PackageDiagnostics: []SurfacePackageDiagnostic{{
			ID:      "surface-diagnostic-01",
			Kind:    "load_error",
			Package: "github.com/example/private",
			Message: "The package loader returned an incomplete result.",
		}},
	}
	warningOffset := len(data.Warnings)
	const typedProductWarning = "local confidence gate capped orientation from 0.90 to 0.60 because focused retrieval is incomplete"
	data.Warnings = append(
		data.Warnings,
		studyPublicationWarningEditingDidNotFinish,
		studyPublicationWarningChecksFailed,
		typedProductWarning,
	)
	data.PresentationWarningKinds = make([]string, len(data.Warnings))
	data.PresentationWarningKinds[warningOffset] = studyPublicationMessageEditingDidNotFinish
	data.PresentationWarningKinds[warningOffset+1] = studyPublicationMessageChecksFailed
	data.runWarningDiagnostics = append(data.runWarningDiagnostics, runWarningDiagnostic{
		WarningIndex: warningOffset + 2,
		Code:         orient.ConfidenceWarningOrientationCapped,
		Proposed:     0.9,
		Capped:       0.6,
	})

	prepared, err := PreparePresentationLocalization(
		data,
		localization.LocaleRussian,
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range prepared.Canonical.Fields {
		if field.Text == studyPublicationWarningEditingDidNotFinish ||
			field.Text == studyPublicationWarningChecksFailed {
			t.Fatalf("fixed product warning entered provider-visible inventory: %q", field.Text)
		}
		if field.Text == typedProductWarning {
			t.Fatalf("typed confidence warning entered provider-visible inventory: %q", field.Text)
		}
	}
	for category, prose := range map[string]string{
		"candidate disposition":         data.CandidateDirections[0].DispositionReason,
		"bundle reason":                 data.Flows[0].BundleFiles[0].Reason,
		"architecture step":             data.ArchitectureCanvas.Flows[0].Steps[0].Label,
		"architecture current frontier": data.ArchitectureCanvas.Flows[0].CurrentFrontier,
		"architecture slot summary":     data.ArchitectureCanvas.Flows[0].Slots[0].Summary,
		"architecture slot missing":     data.ArchitectureCanvas.Flows[0].Slots[0].Missing,
		"architecture frontier":         data.ArchitectureCanvas.Frontiers[0].Reason,
		"architecture diagnostic":       data.ArchitectureCanvas.Diagnostics[0].Message,
		"architecture surface reason":   data.ArchitectureCanvas.Surfaces[len(data.ArchitectureCanvas.Surfaces)-1].TraceReadinessReason,
		"study document label":          data.StudyMap.Directions[0].Documents[0].Label,
		"operation expected result":     data.Operations.Paths[0].ExpectedResults[0].Reference.Label,
		"operation source reason":       operationSource.LandmarkReason,
		"implementation source reason":  detailSource.LandmarkReason,
		"surface scope":                 data.DiscoveredSurfaces.ScopeStatement,
		"surface evidence":              data.DiscoveredSurfaces.Triggers[0].Evidence[0].Detail,
		"surface frontier":              data.DiscoveredSurfaces.Triggers[0].DynamicFrontier[0].Detail,
		"surface loop":                  data.DiscoveredSurfaces.LoopSignals[0].Detail,
		"surface unavailable package":   data.DiscoveredSurfaces.UnavailablePackages[0].Reason,
		"surface diagnostic":            data.DiscoveredSurfaces.PackageDiagnostics[0].Message,
	} {
		found := false
		for _, field := range prepared.Canonical.Fields {
			if field.Text == prose {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s prose is absent from inventory: %q", category, prose)
		}
	}
	inputTexts := make([]string, 0, len(prepared.Input.Fields))
	for _, field := range prepared.Input.Fields {
		inputTexts = append(inputTexts, field.Text)
	}
	joinedInput := strings.Join(inputTexts, "\n")
	for _, opaque := range []string{
		"architecture-step-validate",
		"storage.StartReplication",
		"internal/storage/replicate.go",
		"internal/storage/replicate_test.go",
		"PASS",
		"surface-trigger-01",
		"server.runLoop",
		"github.com/example/private",
	} {
		if strings.Contains(joinedInput, opaque) {
			t.Errorf("opaque value entered provider-visible prose: %q", opaque)
		}
	}
}

func TestPresentationTextInventoryProjectionIsAtomicAndPreservesOpaqueValues(t *testing.T) {
	t.Parallel()

	canonical := broadPresentationLocalizationFixture()
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

	for _, test := range []struct {
		name   string
		mutate func(localization.Projection)
	}{
		{
			name: "missing prose field",
			mutate: func(projection localization.Projection) {
				delete(projection.Translations, prepared.Input.Fields[0].ID)
			},
		},
		{
			name: "unknown prose field",
			mutate: func(projection localization.Projection) {
				projection.Translations["presentation_text:unknown/address.text"] =
					"Неизвестный перевод"
			},
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			projection := russianPresentationProjection(prepared)
			test.mutate(projection)
			projected, result, err := ApplyPresentationLocalization(
				canonical,
				prepared,
				projection,
			)
			if err == nil || projected != nil ||
				!result.Fallback || len(result.Diagnostics) == 0 {
				t.Fatalf(
					"invalid full projection was partially accepted: projected=%#v result=%#v err=%v",
					projected,
					result,
					err,
				)
			}
			after, marshalErr := json.Marshal(canonical)
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			if !bytes.Equal(before, after) {
				t.Fatal("rejected projection mutated canonical report")
			}
		})
	}

	projected, result, err := ApplyPresentationLocalization(
		canonical,
		prepared,
		russianPresentationProjection(prepared),
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Fallback || len(result.Fields) != len(prepared.Canonical.Fields) {
		t.Fatalf("complete projection result = %#v", result)
	}
	if projected.CandidateDirections[0].ID != canonical.CandidateDirections[0].ID ||
		projected.CandidateDirections[0].LikelyEntrypoint !=
			canonical.CandidateDirections[0].LikelyEntrypoint ||
		!reflect.DeepEqual(
			projected.CandidateDirections[0].LikelyFiles,
			canonical.CandidateDirections[0].LikelyFiles,
		) ||
		!reflect.DeepEqual(
			projected.StudyMap.Directions[0].SearchQueries,
			canonical.StudyMap.Directions[0].SearchQueries,
		) ||
		projected.UserSources[0].ContentSHA256 !=
			canonical.UserSources[0].ContentSHA256 ||
		projected.UserSources[0].PresentationSHA256 !=
			canonical.UserSources[0].PresentationSHA256 ||
		projected.UserSources[0].Path != canonical.UserSources[0].Path ||
		projected.UserSources[0].EnclosingSymbol !=
			canonical.UserSources[0].EnclosingSymbol {
		t.Fatalf("projection changed opaque IDs, paths, queries, symbols, or hashes")
	}
	if got := marshalPresentationDataWithCanonicalProse(t, projected, prepared); !bytes.Equal(got, marshalPresentationDataWithCanonicalProse(t, canonical, prepared)) {
		t.Fatal("complete projection changed canonical report domain outside prose")
	}
}

func TestPresentationTextInventoryDistinguishesLocatedFrontiersByCanonicalOrder(t *testing.T) {
	t.Parallel()

	location := &SurfaceLocation{
		Path:   "server/etcdserver/server.go",
		Line:   123,
		Column: 7,
	}
	canonical := &ReportData{DiscoveredSurfaces: &DiscoveredSurfaces{
		DynamicFrontiers: []SurfaceFrontier{
			{
				Kind:     "dynamic_dispatch",
				Detail:   "The first dynamic dispatch remains unresolved.",
				Location: location,
			},
			{
				Kind:     "dynamic_dispatch",
				Detail:   "The second dynamic dispatch remains unresolved.",
				Location: location,
			},
		},
	}}
	before, err := json.Marshal(canonical)
	if err != nil {
		t.Fatal(err)
	}
	first, err := PreparePresentationLocalization(canonical, localization.LocaleRussian)
	if err != nil {
		t.Fatal(err)
	}
	second, err := PreparePresentationLocalization(canonical, localization.LocaleRussian)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("repeated inventory preparation is not deterministic:\nfirst=%#v\nsecond=%#v", first, second)
	}

	fieldIDs := make(map[string]string, 2)
	for _, field := range first.Canonical.Fields {
		switch field.Text {
		case canonical.DiscoveredSurfaces.DynamicFrontiers[0].Detail,
			canonical.DiscoveredSurfaces.DynamicFrontiers[1].Detail:
			fieldIDs[field.Text] = field.ID
		}
	}
	firstID := fieldIDs[canonical.DiscoveredSurfaces.DynamicFrontiers[0].Detail]
	secondID := fieldIDs[canonical.DiscoveredSurfaces.DynamicFrontiers[1].Detail]
	if firstID == "" || secondID == "" || firstID == secondID {
		t.Fatalf("frontier field IDs = %q/%q, want two distinct stable addresses", firstID, secondID)
	}

	projection := russianPresentationProjection(first)
	projection.Translations[firstID] = "Первый динамический переход остаётся неразрешённым."
	projection.Translations[secondID] = "Второй динамический переход остаётся неразрешённым."
	projected, result, err := ApplyPresentationLocalization(canonical, first, projection)
	if err != nil {
		t.Fatal(err)
	}
	if result.Fallback || projected == nil {
		t.Fatalf("complete frontier projection = projected %#v result %#v", projected, result)
	}
	if got := projected.DiscoveredSurfaces.DynamicFrontiers[0].Detail; got != projection.Translations[firstID] {
		t.Fatalf("first frontier detail = %q, want %q", got, projection.Translations[firstID])
	}
	if got := projected.DiscoveredSurfaces.DynamicFrontiers[1].Detail; got != projection.Translations[secondID] {
		t.Fatalf("second frontier detail = %q, want %q", got, projection.Translations[secondID])
	}
	after, err := json.Marshal(canonical)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("inventory preparation/application changed canonical report bytes")
	}
}

func TestPresentationLocalizationSuccessfulSidecarLoadsWithoutChangingCanonicalJSON(t *testing.T) {
	t.Parallel()

	runDir := t.TempDir()
	canonical := presentationLocalizationFixture()
	reportPath := filepath.Join(runDir, "report.json")
	if err := WriteReportJSON(canonical, reportPath); err != nil {
		t.Fatal(err)
	}
	beforeJSON, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatal(err)
	}

	prepared, err := PreparePresentationLocalization(canonical, localization.LocaleRussian)
	if err != nil {
		t.Fatal(err)
	}
	if err := WritePresentationLocalizationSuccess(
		runDir,
		prepared,
		russianPresentationProjection(prepared),
		true,
		strings.Repeat("a", 64),
		strings.Repeat("b", 64),
	); err != nil {
		t.Fatal(err)
	}
	projected, status := LoadPresentationLocalization(
		runDir,
		canonical,
		localization.LocaleRussian,
	)
	if status.State != PresentationLocalizationSucceeded || !status.CacheHit ||
		projected.ReportLanguage != localization.LocaleRussian ||
		projected.presentationLocalizationState != PresentationLocalizationSucceeded ||
		!strings.HasPrefix(projected.ProjectGuess, "Русский текст") {
		t.Fatalf("loaded projection = %#v, status = %#v", projected, status)
	}

	afterJSON, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(beforeJSON, afterJSON) {
		t.Fatalf("render sidecar mutated canonical report.json:\n%s\n%s", beforeJSON, afterJSON)
	}
	if bytes.Contains(afterJSON, []byte(`"report_language"`)) ||
		bytes.Contains(afterJSON, []byte("Русский текст")) {
		t.Fatalf("canonical report.json persisted RU projection: %s", afterJSON)
	}
}

func TestPresentationLocalizationStatusCommitsContentAddressedProjectionAtomically(
	t *testing.T,
) {
	t.Parallel()

	runDir := t.TempDir()
	canonical := presentationLocalizationFixture()
	prepared, err := PreparePresentationLocalization(
		canonical,
		localization.LocaleRussian,
	)
	if err != nil {
		t.Fatal(err)
	}
	first := russianPresentationProjection(prepared)
	if err := WritePresentationLocalizationSuccess(
		runDir,
		prepared,
		first,
		false,
		strings.Repeat("1", 64),
		strings.Repeat("2", 64),
	); err != nil {
		t.Fatal(err)
	}
	firstPath := presentationLocalizationGenerationPath(t, runDir)
	firstProjected, firstStatus := LoadPresentationLocalization(
		runDir,
		canonical,
		localization.LocaleRussian,
	)
	if firstStatus.State != PresentationLocalizationSucceeded {
		t.Fatalf("first status = %#v", firstStatus)
	}

	second := localization.Projection{
		Version:         first.Version,
		CanonicalSHA256: first.CanonicalSHA256,
		Locale:          first.Locale,
		Translations:    make(map[string]string, len(first.Translations)),
	}
	for fieldID, translated := range first.Translations {
		second.Translations[fieldID] = strings.Replace(
			translated,
			"Русский текст",
			"Другой русский текст",
			1,
		)
	}
	secondRecord := PresentationLocalizationProjectionRecord{
		Version:         PresentationLocalizationRecordVersion,
		ContractVersion: PresentationLocalizationContractVersion,
		TargetLocale:    localization.LocaleRussian,
		CanonicalSHA256: prepared.Canonical.SHA256,
		Projection:      second,
	}
	secondJSON, err := json.Marshal(secondRecord)
	if err != nil {
		t.Fatal(err)
	}
	secondJSON = append(secondJSON, '\n')
	secondPath := filepath.Join(
		runDir,
		presentationLocalizationProjectionFilename(
			presentationLocalizationSHA256(secondJSON),
		),
	)
	if err := os.WriteFile(secondPath, secondJSON, 0o600); err != nil {
		t.Fatal(err)
	}

	// A crash after publishing the next generation but before status leaves
	// the previous pair authoritative and loadable.
	stillFirst, stillFirstStatus := LoadPresentationLocalization(
		runDir,
		canonical,
		localization.LocaleRussian,
	)
	if stillFirstStatus.ProjectionSHA256 != firstStatus.ProjectionSHA256 ||
		stillFirst.ProjectGuess != firstProjected.ProjectGuess {
		t.Fatalf(
			"uncommitted generation changed active pair: status=%#v report=%#v",
			stillFirstStatus,
			stillFirst,
		)
	}
	if _, err := os.Stat(firstPath); err != nil {
		t.Fatalf("prior committed projection disappeared: %v", err)
	}

	if err := WritePresentationLocalizationSuccess(
		runDir,
		prepared,
		second,
		false,
		strings.Repeat("3", 64),
		strings.Repeat("4", 64),
	); err != nil {
		t.Fatal(err)
	}
	committed, committedStatus := LoadPresentationLocalization(
		runDir,
		canonical,
		localization.LocaleRussian,
	)
	if committedStatus.State != PresentationLocalizationSucceeded ||
		committedStatus.ProjectionSHA256 == firstStatus.ProjectionSHA256 ||
		committed.ProjectGuess == firstProjected.ProjectGuess ||
		!strings.HasPrefix(committed.ProjectGuess, "Другой русский текст") {
		t.Fatalf(
			"new committed pair was not loaded: status=%#v report=%#v",
			committedStatus,
			committed,
		)
	}
}

func TestPresentationLocalizationRejectsUnsafeProjectionGenerationHash(
	t *testing.T,
) {
	t.Parallel()

	status := PresentationLocalizationStatus{
		Version:          PresentationLocalizationStatusVersion,
		ContractVersion:  PresentationLocalizationContractVersion,
		RequestedLocale:  localization.LocaleRussian,
		State:            PresentationLocalizationSucceeded,
		CanonicalSHA256:  strings.Repeat("1", 64),
		RequestSHA256:    strings.Repeat("2", 64),
		ProjectionSHA256: "../presentation_localization_projection.v1.json",
		CacheKey:         strings.Repeat("3", 64),
	}
	if _, err := marshalPresentationLocalizationStatus(status); err == nil {
		t.Fatal("path-like projection generation hash was accepted")
	}
}

func TestPresentationLocalizationFailureKeepsRussianCatalogAndCanonicalEnglishProse(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name       string
		prepare    func(t *testing.T, runDir string, canonical *ReportData)
		wantReason string
	}{
		{
			name: "recorded provider failure",
			prepare: func(t *testing.T, runDir string, canonical *ReportData) {
				t.Helper()
				prepared, err := PreparePresentationLocalization(
					canonical,
					localization.LocaleRussian,
				)
				if err != nil {
					t.Fatal(err)
				}
				if err := WritePresentationLocalizationFailure(
					runDir,
					PresentationLocalizationFailure{
						ReasonCode:      LocalizationFailureProviderRequest,
						FailureStage:    LocalizationStageProviderRequest,
						ValidationCode:  LocalizationValidationTransport,
						CanonicalSHA256: prepared.Canonical.SHA256,
					},
				); err != nil {
					t.Fatal(err)
				}
			},
			wantReason: LocalizationFailureProviderRequest,
		},
		{
			name: "corrupt status",
			prepare: func(t *testing.T, runDir string, _ *ReportData) {
				t.Helper()
				if err := os.WriteFile(
					filepath.Join(runDir, PresentationLocalizationStatusFile),
					[]byte("{"),
					0o600,
				); err != nil {
					t.Fatal(err)
				}
			},
			wantReason: LocalizationFailureSavedProjection,
		},
		{
			name: "corrupt projection",
			prepare: func(t *testing.T, runDir string, canonical *ReportData) {
				t.Helper()
				prepared, err := PreparePresentationLocalization(
					canonical,
					localization.LocaleRussian,
				)
				if err != nil {
					t.Fatal(err)
				}
				if err := WritePresentationLocalizationSuccess(
					runDir,
					prepared,
					russianPresentationProjection(prepared),
					false,
					strings.Repeat("c", 64),
					strings.Repeat("d", 64),
				); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(
					presentationLocalizationGenerationPath(t, runDir),
					[]byte("{"),
					0o600,
				); err != nil {
					t.Fatal(err)
				}
			},
			wantReason: LocalizationFailureSavedProjection,
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			runDir := t.TempDir()
			canonical := presentationLocalizationFixture()
			test.prepare(t, runDir, canonical)
			got, status := LoadPresentationLocalization(
				runDir,
				canonical,
				localization.LocaleRussian,
			)
			if status.State != PresentationLocalizationFailed ||
				status.ReasonCode != test.wantReason {
				t.Fatalf("status = %#v", status)
			}
			if got.ReportLanguage != localization.LocaleRussian ||
				got.presentationLocalizationState != PresentationLocalizationFailed ||
				got.presentationLocalizationMessageID !=
					"main.localization.ru_unavailable_canonical_en" ||
				got.ProjectGuess != canonical.ProjectGuess ||
				got.ArchitectureCanvas.Title != canonical.ArchitectureCanvas.Title {
				t.Fatalf("failure did not keep RU catalog with canonical EN prose: %#v", got)
			}
		})
	}
}

func TestPresentationLocalizationMissingStatusIsExplicitFailureWhenRussianWasRequested(t *testing.T) {
	t.Parallel()

	canonical := presentationLocalizationFixture()
	projected, status := LoadPresentationLocalization(
		t.TempDir(),
		canonical,
		localization.LocaleRussian,
	)
	if status.State != PresentationLocalizationFailed ||
		status.ReasonCode != LocalizationFailureStatusUnavailable {
		t.Fatalf("status = %#v, want explicit missing-status failure", status)
	}
	if projected.ReportLanguage != localization.LocaleRussian ||
		projected.ProjectGuess != canonical.ProjectGuess ||
		projected.presentationLocalizationState != PresentationLocalizationFailed {
		t.Fatalf("missing status did not preserve RU catalog with canonical English prose: %#v", projected)
	}
	html, err := RenderHTML(projected)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(html, []byte(`rm-localization-status--failed`)) ||
		!bytes.Contains(html, []byte(`main.localization.ru_unavailable_canonical_en`)) {
		t.Fatal("missing localization status was silently rendered as ordinary English")
	}
}

func TestAtlasFirstRussianPresentationIsActiveWithoutLegacyLocalizationStatus(t *testing.T) {
	t.Parallel()

	canonical := presentationLocalizationFixture()
	canonical.RepositoryAtlas = repositoryAtlasFixturePtr()
	projected, status := LoadPresentationLocalization(
		t.TempDir(),
		canonical,
		localization.LocaleRussian,
	)
	if status.State != "" || projected.ReportLanguage != localization.LocaleRussian ||
		projected.presentationLocalizationState != presentationLocalizationStageOwned {
		t.Fatalf("stage-owned Russian presentation/status = %#v/%#v", projected, status)
	}
	html, err := RenderHTML(projected)
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range [][]byte{
		[]byte(`<html lang="ru">`),
		[]byte(`rm-localization-status--stage_owned`),
		[]byte(`data-rm-message="main.localization.ru_active"`),
	} {
		if !bytes.Contains(html, marker) {
			t.Fatalf("stage-owned RU render is missing %q", marker)
		}
	}
	if bytes.Contains(html, []byte(`data-rm-message="main.localization.ru_unavailable_canonical_en"`)) {
		t.Fatal("stage-owned RU render showed the legacy unavailable warning")
	}
}

func TestPresentationLocalizationEndToEndRendersEnglishRussianAndDegradation(t *testing.T) {
	t.Parallel()

	canonical := presentationLocalizationFixture()
	englishHTML, err := RenderHTML(canonical)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(englishHTML, []byte(`<html lang="en">`)) ||
		!bytes.Contains(englishHTML, []byte(canonical.ProjectGuess)) ||
		bytes.Contains(englishHTML, []byte(`<div class="rm-localization-status`)) {
		t.Fatalf("canonical English render is not clean: %s", englishHTML[:min(len(englishHTML), 1200)])
	}

	prepared, err := PreparePresentationLocalization(canonical, localization.LocaleRussian)
	if err != nil {
		t.Fatal(err)
	}
	russianRun := t.TempDir()
	if err := WritePresentationLocalizationSuccess(
		russianRun,
		prepared,
		russianPresentationProjection(prepared),
		false,
		strings.Repeat("e", 64),
		strings.Repeat("f", 64),
	); err != nil {
		t.Fatal(err)
	}
	russianData, status := LoadPresentationLocalization(
		russianRun,
		canonical,
		localization.LocaleRussian,
	)
	if status.State != PresentationLocalizationSucceeded {
		t.Fatalf("Russian status = %#v", status)
	}
	russianHTML, err := RenderHTML(russianData)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(russianHTML, []byte(`<html lang="ru">`)) ||
		!bytes.Contains(russianHTML, []byte("Русский текст")) ||
		!bytes.Contains(russianHTML, []byte(`rm-localization-status--succeeded`)) ||
		bytes.Contains(russianHTML, []byte(`"project_guess":"`+canonical.ProjectGuess+`"`)) {
		t.Fatalf("successful Russian render is incomplete: %s", russianHTML[:min(len(russianHTML), 1600)])
	}

	failureRun := t.TempDir()
	if err := WritePresentationLocalizationFailure(
		failureRun,
		PresentationLocalizationFailure{
			ReasonCode:      LocalizationFailureProviderRequest,
			FailureStage:    LocalizationStageProviderRequest,
			ValidationCode:  LocalizationValidationTransport,
			CanonicalSHA256: prepared.Canonical.SHA256,
		},
	); err != nil {
		t.Fatal(err)
	}
	degradedData, status := LoadPresentationLocalization(
		failureRun,
		canonical,
		localization.LocaleRussian,
	)
	if status.State != PresentationLocalizationFailed {
		t.Fatalf("degradation status = %#v", status)
	}
	degradedHTML, err := RenderHTML(degradedData)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(degradedHTML, []byte(`<html lang="ru">`)) ||
		!bytes.Contains(degradedHTML, []byte(canonical.ProjectGuess)) ||
		!bytes.Contains(degradedHTML, []byte(`rm-localization-status--failed`)) ||
		!bytes.Contains(
			degradedHTML,
			[]byte(`data-rm-message="main.localization.ru_unavailable_canonical_en"`),
		) ||
		bytes.Contains(degradedHTML, []byte("Русский текст")) {
		t.Fatalf("degraded render did not show RU product UI with canonical EN prose: %s", degradedHTML[:min(len(degradedHTML), 1600)])
	}
}

func TestPresentationLocalizationEnglishRequestIgnoresValidRussianSidecar(t *testing.T) {
	t.Parallel()

	runDir := t.TempDir()
	canonical := presentationLocalizationFixture()
	prepared, err := PreparePresentationLocalization(canonical, localization.LocaleRussian)
	if err != nil {
		t.Fatal(err)
	}
	if err := WritePresentationLocalizationSuccess(
		runDir,
		prepared,
		russianPresentationProjection(prepared),
		false,
		strings.Repeat("1", 64),
		strings.Repeat("2", 64),
	); err != nil {
		t.Fatal(err)
	}

	for _, requestedLocale := range []string{"", localization.LocaleEnglish} {
		requestedLocale := requestedLocale
		t.Run(requestedLocale, func(t *testing.T) {
			t.Parallel()

			got, status := LoadPresentationLocalization(
				runDir,
				canonical,
				requestedLocale,
			)
			if status.State != "" ||
				got.ReportLanguage != "" ||
				got.presentationLocalizationState != "" ||
				got.ProjectGuess != canonical.ProjectGuess {
				t.Fatalf(
					"EN request consumed RU sidecar: locale=%q status=%#v report=%#v",
					requestedLocale,
					status,
					got,
				)
			}
		})
	}
}

func TestPresentationLocalizationRejectsSecretBearingRunSidecar(t *testing.T) {
	t.Parallel()

	runDir := t.TempDir()
	canonical := presentationLocalizationFixture()
	prepared, err := PreparePresentationLocalization(
		canonical,
		localization.LocaleRussian,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := WritePresentationLocalizationSuccess(
		runDir,
		prepared,
		russianPresentationProjection(prepared),
		false,
		strings.Repeat("3", 64),
		"translation-"+strings.Repeat("4", 64),
	); err != nil {
		t.Fatal(err)
	}

	recordPath := presentationLocalizationGenerationPath(t, runDir)
	recordJSON, err := os.ReadFile(recordPath)
	if err != nil {
		t.Fatal(err)
	}
	var record PresentationLocalizationProjectionRecord
	if err := json.Unmarshal(recordJSON, &record); err != nil {
		t.Fatal(err)
	}
	for fieldID := range record.Projection.Translations {
		record.Projection.Translations[fieldID] =
			"api_key=secret-value-123456"
		break
	}
	recordJSON, err = json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	recordJSON = append(recordJSON, '\n')
	if err := os.WriteFile(recordPath, recordJSON, 0o600); err != nil {
		t.Fatal(err)
	}

	statusPath := filepath.Join(runDir, PresentationLocalizationStatusFile)
	statusJSON, err := os.ReadFile(statusPath)
	if err != nil {
		t.Fatal(err)
	}
	var savedStatus PresentationLocalizationStatus
	if err := json.Unmarshal(statusJSON, &savedStatus); err != nil {
		t.Fatal(err)
	}
	savedStatus.ProjectionSHA256 = presentationLocalizationSHA256(recordJSON)
	statusJSON, err = marshalPresentationLocalizationStatus(savedStatus)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statusPath, statusJSON, 0o600); err != nil {
		t.Fatal(err)
	}

	got, status := LoadPresentationLocalization(
		runDir,
		canonical,
		localization.LocaleRussian,
	)
	if status.State != PresentationLocalizationFailed ||
		status.ReasonCode != LocalizationFailureSavedProjection ||
		got.ReportLanguage != localization.LocaleRussian ||
		got.ProjectGuess != canonical.ProjectGuess {
		t.Fatalf(
			"unsafe run sidecar was consumed: status=%#v report=%#v",
			status,
			got,
		)
	}
}

func TestPresentationLocalizationAuthorizedCanonicalRoundTrip(t *testing.T) {
	t.Parallel()

	repository := t.TempDir()
	writeTestFile(t, repository, "main.go", "package main\n\nfunc main() {}\n")
	runManifestGit(t, repository, "init", "--quiet")
	runManifestGit(t, repository, "add", "--all")
	runManifestGit(
		t,
		repository,
		"-c", "user.name=repomap test",
		"-c", "user.email=repomap@example.invalid",
		"-c", "commit.gpgsign=false",
		"commit", "--quiet", "-m", "fixture",
	)
	initial, err := freshness.CaptureRepository(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	current, err := freshness.CaptureRepository(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := ConfirmRunAuthority(repository, initial, current)
	if err != nil {
		t.Fatal(err)
	}

	runDir := t.TempDir()
	writeRunManifestMetadata(t, runDir, repository)
	metadataJSON, err := json.Marshal(map[string]any{
		"repo_name": "manifest-fixture",
		"repo_path": repository,
		"effective_options": map[string]any{
			"report_language": localization.LocaleRussian,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(runDir, "metadata.json"),
		metadataJSON,
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, runDir, "snapshot.json", `{
		"repo_name":"localized-authority",
		"file_tree":["main.go"],
		"files_considered":1
	}`)
	modelBundle := `{"allowed_paths":["main.go"]}`
	writeTestFile(t, runDir, "llm_bundle.json", modelBundle)
	if err := os.WriteFile(
		filepath.Join(runDir, llmbundle.OrientationContextSelectionFilename),
		validOrientationContextSelectionArtifact(t, []byte(modelBundle)),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, runDir, "orientation_report.json", fmt.Sprintf(`{
		"project_guess":"An English guide for captured revision %s",
		"high_level_map":[],
		"candidate_flows":[],
		"warnings":[]
	}`, initial.Head))

	if err := GenerateAuthorized(runDir, authority); err != nil {
		t.Fatalf("first GenerateAuthorized() error = %v", err)
	}
	canonicalJSON, err := os.ReadFile(filepath.Join(runDir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	var canonical ReportData
	if err := json.Unmarshal(canonicalJSON, &canonical); err != nil {
		t.Fatal(err)
	}
	if canonical.CapturedRevision != initial.Head ||
		!strings.Contains(canonical.ProjectGuess, initial.Head) {
		t.Fatalf("authority-bound canonical report = %#v", canonical)
	}
	canonicalPresentation, err := PrepareRunPresentation(runDir, &canonical, nil)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := PreparePresentationLocalization(
		canonicalPresentation,
		localization.LocaleRussian,
	)
	if err != nil {
		t.Fatal(err)
	}
	inputJSON, err := json.Marshal(prepared.Input)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(inputJSON, []byte(initial.Head)) {
		t.Fatalf("captured revision leaked instead of remaining an opaque placeholder: %s", inputJSON)
	}
	if err := WritePresentationLocalizationSuccess(
		runDir,
		prepared,
		russianPresentationProjection(prepared),
		false,
		"request-sha",
		"cache-key",
	); err != nil {
		t.Fatal(err)
	}
	if err := GenerateAuthorized(runDir, authority); err != nil {
		t.Fatalf("localized GenerateAuthorized() error = %v", err)
	}
	html, err := os.ReadFile(filepath.Join(runDir, "report.html"))
	if err != nil {
		t.Fatal(err)
	}
	afterJSON, err := os.ReadFile(filepath.Join(runDir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	var afterCanonical ReportData
	if err := json.Unmarshal(afterJSON, &afterCanonical); err != nil {
		t.Fatal(err)
	}
	for _, marker := range [][]byte{
		[]byte(`<html lang="ru">`),
		[]byte(`data-rm-message="main.localization.ru_active"`),
		[]byte(initial.Head),
	} {
		if !bytes.Contains(html, marker) {
			t.Fatalf("authorized RU render is missing %q", marker)
		}
	}
	if !bytes.Equal(afterJSON, canonicalJSON) {
		t.Fatal("localized authorized render changed canonical report.json")
	}
}

func presentationLocalizationFixture() *ReportData {
	canvas := architectureLocalizationFixture()
	canvas.Components[0].Members[0].Facts[0].Provenance = []evidence.Provenance{{
		Provider:  "go_types",
		Version:   "1",
		Operation: "inspect",
		Location: &evidence.Location{
			Path: "cmd/сервер main.go",
			Line: 42,
		},
	}}
	canvas.StructuralFacts[0].Provenance = []evidence.Provenance{{
		Provider:  "go_types",
		Operation: "relations",
		Location: &evidence.Location{
			Path: "internal/storage/replicate.go",
			Line: 88,
		},
	}}

	source := SourceSnippet{
		Path:            "internal/storage/replicate.go",
		Language:        "go",
		EnclosingSymbol: "StartReplication",
		StartLine:       88,
		EndLine:         88,
		HighlightRanges: []SourceHighlight{{StartLine: 88, EndLine: 88}},
		Content:         "StartReplication(ctx)",
		Lines: []SourceSnippetLine{{
			Line: 88,
			Text: "StartReplication(ctx)",
		}},
		ContentSHA256:      strings.Repeat("1", 64),
		PresentationSHA256: strings.Repeat("2", 64),
		RelatedEvidenceIDs: []string{"evidence-01"},
		Role:               "mechanism",
		Revision:           strings.Repeat("a", 40),
	}
	location := UserCodeLocation{
		Path:   "internal/storage/replicate.go",
		Line:   88,
		Column: 3,
	}
	return &ReportData{
		FormatVersion:     CurrentFormatVersion,
		RepoName:          "fixture-repo",
		ProjectGuess:      "A PostgreSQL replication service",
		DocumentedPurpose: "Explains how StartReplication reaches durable storage.",
		CandidateFlows:    []string{"flow-01"},
		Flows:             []FlowData{},
		Warnings:          []string{"Evidence remains bounded."},
		CapturedRevision:  strings.Repeat("a", 40),
		OpenablePaths: []string{
			"cmd/сервер main.go",
			"internal/storage/replicate.go",
		},
		GitHubSourceLinks: &GitHubSourceLinks{
			RepositoryURL:    "https://github.example.test/team/fixture-repo",
			Revision:         strings.Repeat("a", 40),
			WorkingTreeDirty: true,
			WorkingTreePaths: []string{"internal/storage/replicate.go"},
		},
		ArchitectureCanvas: &canvas,
		StudyMap: &RepositoryStudyMap{
			Version: 1,
			Brief: RepositoryBrief{
				WhatItIs:              "A PostgreSQL replication service.",
				Problem:               "It coordinates durable replication.",
				MainInput:             "A StartReplication request.",
				CentralResponsibility: "Validate and persist replication state.",
				ObservableResult:      "A durable storage update.",
				DomainTerms: []RepositoryBriefDomainTerm{{
					Term:    "LSN",
					Meaning: "A PostgreSQL log sequence number.",
				}},
			},
			Shape: []RepositoryStudyArea{{
				ID:             "area-storage",
				Name:           "Storage coordination",
				Responsibility: "Own durable replication state.",
				CodeLocation:   &location,
				Source:         &source,
			}},
			Directions: []StudyDirection{{
				ID:              "direction-replication",
				Question:        "How does StartReplication update storage?",
				WhyItMatters:    "This explains the principal durability path.",
				LearningOutcome: "Follow the request into persistent state.",
				PrincipalAnchors: []StudyCodeAnchor{{
					Path:   "internal/storage/replicate.go",
					Symbol: "StartReplication",
					Line:   88,
				}},
				ReadingAnchors: []StudyReadingAnchor{{
					Label:         "StartReplication call",
					WhatToLookFor: "Find the durable write after StartReplication.",
					Location:      location,
					Source:        source,
				}},
				SearchQueries: []string{"StartReplication storage"},
			}},
		},
		UserMechanisms: []UserMechanism{{
			ArtifactID: "mechanism-replication",
			Title:      "Persist a replication update",
			Question:   "How does the service persist a replication update?",
			Answer:     "It validates the request and writes durable state.",
			Steps: []UserMechanismStep{{
				Title:       "Validate the request",
				Explanation: "StartReplication validates the incoming state.",
				Locations:   []UserCodeLocation{location},
				Sources:     []SourceSnippet{source},
				WhatToNotice: []SourceNotice{{
					Text: "The request is validated before persistence.",
					Path: "internal/storage/replicate.go",
					SupportingRanges: []SourceHighlight{{
						StartLine: 88,
						EndLine:   88,
					}},
				}},
			}},
			Phases: []UserMechanismPhase{{
				Title:                     "Validate and persist",
				Explanation:               "The service validates the request before persistence.",
				Locations:                 []UserCodeLocation{location},
				Sources:                   []SourceSnippet{source},
				ImplementationStepIndexes: []int{0},
				ImplementationDetails: []UserMechanismStep{{
					Title:       "Inspect the storage write",
					Explanation: "StartReplication writes the validated state.",
					Locations:   []UserCodeLocation{location},
					Sources:     []SourceSnippet{source},
					WhatToNotice: []SourceNotice{{
						Text: "The durable write follows request validation.",
						Path: "internal/storage/replicate.go",
						SupportingRanges: []SourceHighlight{{
							StartLine: 88,
							EndLine:   88,
						}},
					}},
				}},
			}},
			Files: []UserCodeLocation{location},
		}},
		UserSources: []SourceSnippet{source},
	}
}

func broadPresentationLocalizationFixture() *ReportData {
	data := presentationLocalizationFixture()
	location := UserCodeLocation{
		Path:   "internal/storage/replicate.go",
		Line:   88,
		Column: 3,
	}
	source := data.UserSources[0]

	data.HighLevelMap = []Subsystem{{
		Name:         "Replication coordination",
		Evidence:     []string{"opaque-orientation-evidence"},
		WhyItMatters: "It owns the durable replication boundary.",
	}}
	data.FirstFilesToOpen = []FileItem{{
		Path:     "internal/storage/replicate.go",
		Reason:   "Begin with the request coordinator.",
		Priority: 1,
	}}
	data.ImportantDomainWords = []DomainWord{{
		Word:     "LSN",
		Guess:    "The durable PostgreSQL log position.",
		Evidence: []string{"opaque-domain-evidence"},
	}}
	data.QuestionsForHuman = []string{
		"Which replication mode is deployed in production?",
	}
	data.OrientationUnverifiedPaths = []PathItem{{
		Path:   "internal/storage/unknown.go",
		Reason: "The model mentioned this path without local confirmation.",
	}}
	data.CandidateDirections = []CandidateDirection{{
		ID:               "direction-replication",
		Name:             "Replication update",
		Trigger:          "A client requests a replication update.",
		LikelyEntrypoint: "StartReplication",
		LikelyFiles:      []string{"internal/storage/replicate.go"},
		WhyInteresting:   "It connects the request to durable state.",
		Evidence:         []string{"The request coordinator is locally visible."},
		Disposition:      "retained",
		CandidateBasis:   "model",
	}}
	data.Flows = []FlowData{{
		ID:      "flow-local-01",
		Name:    "Durable replication update",
		Summary: "The request is validated before the durable write.",
		LikelyChain: []ChainStep{{
			Step:          1,
			Name:          "Validate replication request",
			WhatHappens:   "The service validates the requested log position.",
			EvidenceFiles: []string{"internal/storage/replicate.go"},
		}},
		FilesToRead: []FileItem{{
			Path:     "internal/storage/replicate.go",
			Reason:   "Read the validation and persistence boundary together.",
			Priority: 1,
		}},
		Unknowns: []string{"The remote acknowledgement remains unknown."},
		Warnings: []string{"The flow stops at the durable local effect."},
	}}
	data.ArchitectureCanvas.Suggestions = []ArchitectureSuggestion{{
		ID:     "suggestion-durability",
		Title:  "Inspect the durability boundary",
		Reason: "This area connects validation to persistence.",
	}}
	data.ArchitectureCanvas.Flows = []ArchitectureFlow{{
		ID:              "architecture-flow-replication",
		Name:            "Replication write path",
		Trigger:         "A replication request arrives.",
		Scope:           "Validation and durable persistence.",
		MentalModel:     "A request crosses one guarded storage boundary.",
		Goal:            "Understand where the durable effect occurs.",
		WhyInspect:      "This is the central state transition.",
		FrontierSummary: "Remote acknowledgement remains outside local evidence.",
	}}
	data.GuidedTour = &guidedtour.Story{
		Version:       1,
		CandidateID:   "tour-replication",
		CandidateName: "Replication tour",
		Trigger:       "A user asks how replication becomes durable.",
		Title:         "Follow a durable replication update",
		Summary:       "Follow the accepted replication story.",
		Steps: []guidedtour.StoryStep{{
			Title:       "Start at request validation",
			Explanation: "Read the validation boundary before persistence.",
			BeatIDs:     []string{"beat-validation"},
		}},
		GapSummary: []guidedtour.StoryGapSummary{{
			Explanation: "Remote acknowledgement is not locally established.",
			GapIDs:      []string{"gap-ack"},
			Gaps: []guidedtour.Gap{{
				ID:     "gap-ack",
				Label:  "Remote acknowledgement",
				Detail: "The receiver outcome remains unresolved.",
			}},
		}},
	}
	data.RepositoryThesis = &RepositoryThesis{
		Purpose:     "Explain the durable replication boundary.",
		SystemStory: []string{"A request crosses the replication boundary."},
		Areas: []RepositoryThesisArea{{
			Label:          "Replication storage",
			Responsibility: "Validate and persist the requested log position.",
			CodeLocation:   &location,
		}},
	}
	data.RepositoryGuide = &RepositoryGuide{
		Purpose:     "Guide the reader through replication.",
		SystemStory: []string{"Start with validation, then inspect persistence."},
		Areas: []RepositoryThesisArea{{
			Label:          "Durability path",
			Responsibility: "Connect the request to its local durable effect.",
			CodeLocation:   &location,
		}},
		ReadNext: []UserReadNextTarget{{
			Label:  "Continue at the storage write",
			Path:   "internal/storage/replicate.go",
			Symbol: "StartReplication",
			Line:   88,
		}},
	}
	data.UserTopics = []UserTopic{{
		CandidateID: "topic-receiver",
		Title:       "Receiver acknowledgement",
		Question:    "How is a durable replication update acknowledged?",
		StartingSymbols: []UserTopicSymbol{{
			Path:   "internal/storage/replicate.go",
			Symbol: "StartReplication",
			Line:   88,
		}},
		Uncertainty: "The exact storage effect is not established yet.",
	}}
	data.UserMechanisms[0].Context = []UserMechanismContext{{
		Label:          "Replication coordinator",
		Responsibility: "Own request validation and durable persistence.",
		CodeLocation:   &location,
	}}
	data.UserMechanisms[0].ReadNext = []UserReadNextTarget{{
		Label:  "Inspect the durable write",
		Path:   "internal/storage/replicate.go",
		Symbol: "StartReplication",
		Line:   88,
	}}
	data.Operations = &RepositoryOperations{
		Version: 1,
		Paths: []RepositoryPavedPath{{
			ID:    "operate-replication",
			Title: "Run the replication validation",
			Goal:  "Observe the validated durable result.",
			Actions: []OperationalAction{{
				Instruction: "Run the repository-owned replication check.",
				Command:     "go test ./internal/storage",
				Reference: OperationalReference{
					Label:    "Replication validation test",
					Location: location,
					Source:   source,
				},
			}},
		}},
	}
	data.ModelResearch = &modelresearch.State{
		Version: modelresearch.ContractVersion,
		Policy:  modelresearch.DefaultPolicy(),
		Rounds: []modelresearch.ResearchRound{{
			Version:         modelresearch.ContractVersion,
			ID:              "round-durable-handoff",
			SelectionReason: selectedResearchRoundReasonCode,
			Status:          modelresearch.RoundCompleted,
		}},
		SkippedRounds: []modelresearch.ResearchRound{{
			Version:         modelresearch.ContractVersion,
			ID:              "round-runtime-observation",
			SelectionReason: skippedResearchRoundReasonCode,
			Status:          modelresearch.RoundSkipped,
		}},
	}
	return data
}

func russianPresentationProjection(
	prepared PreparedPresentationLocalization,
) localization.Projection {
	translations := make(map[string]string, len(prepared.Input.Fields))
	for _, field := range prepared.Input.Fields {
		placeholders := presentationLocalizationTestPlaceholderPattern.FindAllString(
			field.Text,
			-1,
		)
		translations[field.ID] = strings.TrimSpace(
			"Русский текст " + strings.Join(placeholders, " "),
		)
	}
	return localization.Projection{
		Version:         localization.ProjectionVersion,
		CanonicalSHA256: prepared.Canonical.SHA256,
		Locale:          localization.LocaleRussian,
		Translations:    translations,
	}
}

var presentationLocalizationTestPlaceholderPattern = regexp.MustCompile(
	`\{\{term_[0-9]{2}\}\}`,
)

func presentationLocalizationGenerationPath(t *testing.T, runDir string) string {
	t.Helper()
	statusJSON, err := os.ReadFile(filepath.Join(
		runDir,
		PresentationLocalizationStatusFile,
	))
	if err != nil {
		t.Fatal(err)
	}
	var status PresentationLocalizationStatus
	if err := decodePresentationLocalizationJSON(statusJSON, &status); err != nil {
		t.Fatal(err)
	}
	if err := validatePresentationLocalizationStatus(status); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(
		runDir,
		presentationLocalizationProjectionFilename(status.ProjectionSHA256),
	)
}

func marshalPresentationDataWithCanonicalProse(
	t *testing.T,
	data *ReportData,
	prepared PreparedPresentationLocalization,
) []byte {
	t.Helper()

	restored, err := cloneReportData(data)
	if err != nil {
		t.Fatal(err)
	}
	bindings, err := buildPresentationLocalizationBindings(restored)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range prepared.Canonical.Fields {
		binding := bindings.byID[field.ID]
		if binding == nil {
			t.Fatalf("canonical field %q has no current binding", field.ID)
		}
		for _, setter := range binding.setters {
			if !setter(restored, field.Text) {
				t.Fatalf("canonical field %q lost its semantic owner", field.ID)
			}
		}
	}
	restored.ReportLanguage = ""
	restored.presentationLocalizationState = ""
	restored.presentationLocalizationMessageID = ""
	encoded, err := json.Marshal(restored)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}
