package report

import (
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/localization"
)

func TestPresentationLocalizationProtectsLowercaseUserTopicIdentities(t *testing.T) {
	t.Parallel()

	data := &ReportData{
		RepoName:     "fixture",
		ProjectGuess: "A service that can serve requests.",
		UserTopics: []UserTopic{{
			CandidateID: "topic-entrypoints",
			Title:       "Inspect serve, start, and main",
			Question:    "How do serve, start, and main cooperate?",
			StartingSymbols: []UserTopicSymbol{
				{Path: "cmd/serve.go", Symbol: "serve"},
				{Path: "cmd/start.go", Symbol: "start"},
				{Path: "cmd/main.go", Symbol: "main"},
			},
			Uncertainty: "The boundary between serve, start, and main is unresolved.",
		}},
	}
	prepared, err := PreparePresentationLocalization(data, localization.LocaleRussian)
	if err != nil {
		t.Fatal(err)
	}
	for _, addressSuffix := range []string{"/title", "/question", "/uncertainty"} {
		for _, value := range []string{"serve", "start", "main"} {
			assertPresentationFieldProtectsValue(
				t,
				prepared,
				"user_topics/topic-entrypoints"+addressSuffix,
				value,
			)
		}
	}
	assertPresentationFieldDoesNotProtectValue(
		t,
		prepared,
		"repository",
		"serve",
	)

	projected, _, err := ApplyPresentationLocalization(
		data,
		prepared,
		russianPresentationProjection(prepared),
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"serve", "start", "main"} {
		for _, text := range []string{
			projected.UserTopics[0].Title,
			projected.UserTopics[0].Question,
			projected.UserTopics[0].Uncertainty,
		} {
			if !strings.Contains(text, value) {
				t.Fatalf("projected topic lost opaque identity %q: %q", value, text)
			}
		}
	}
	if projected.UserTopics[0].StartingSymbols[0].Symbol != "serve" ||
		projected.UserTopics[0].StartingSymbols[1].Symbol != "start" ||
		projected.UserTopics[0].StartingSymbols[2].Symbol != "main" {
		t.Fatalf("projection changed topic symbols: %#v", projected.UserTopics[0].StartingSymbols)
	}
}

func TestPresentationLocalizationDoesNotGlobalizePackageNameAsProse(t *testing.T) {
	t.Parallel()

	data := &ReportData{
		RepoName:     "fixture",
		ProjectGuess: "The main service handles requests.",
		RepositoryGraph: &RepositoryGraph{Packages: []PackageInfo{{
			CanonicalPath: "example.test/fixture/cmd",
			Name:          "main",
			ModulePath:    "example.test/fixture",
			Dir:           "cmd",
		}}},
	}
	prepared, err := PreparePresentationLocalization(data, localization.LocaleRussian)
	if err != nil {
		t.Fatal(err)
	}
	assertPresentationFieldDoesNotProtectValue(
		t,
		prepared,
		"repository",
		"main",
	)
}

func TestPresentationLocalizationScopesRepositoryProductName(t *testing.T) {
	t.Parallel()

	data := &ReportData{
		RepoName:     "Echo",
		ProjectGuess: "Echo routes repository requests.",
		CandidateDirections: []CandidateDirection{{
			ID:   "candidate-routing",
			Name: "Echo behavior in a user-facing explanation",
		}},
	}
	prepared, err := PreparePresentationLocalization(data, localization.LocaleRussian)
	if err != nil {
		t.Fatal(err)
	}
	assertPresentationFieldProtectsValue(
		t,
		prepared,
		"repository",
		"Echo",
	)
	assertPresentationFieldDoesNotProtectValue(
		t,
		prepared,
		"orientation/directions/candidate-routing/name",
		"Echo",
	)
}

func TestPresentationLocalizationScopesStudySymbolToItsDirection(t *testing.T) {
	t.Parallel()

	data := &ReportData{
		RepoName:     "fixture",
		ProjectGuess: "The main service handles requests.",
		StudyMap: &RepositoryStudyMap{
			Directions: []StudyDirection{{
				ID:       "startup",
				Question: "How does main initialize the service?",
				PrincipalAnchors: []StudyCodeAnchor{{
					Path:   "cmd/main.go",
					Symbol: "main",
					Line:   12,
				}},
			}},
		},
	}
	prepared, err := PreparePresentationLocalization(data, localization.LocaleRussian)
	if err != nil {
		t.Fatal(err)
	}
	assertPresentationFieldProtectsValue(
		t,
		prepared,
		"study_direction/startup/question",
		"main",
	)
	assertPresentationFieldDoesNotProtectValue(
		t,
		prepared,
		"repository",
		"main",
	)
}

func assertPresentationFieldDoesNotProtectValue(
	t *testing.T,
	prepared PreparedPresentationLocalization,
	ownerNeedle,
	value string,
) {
	t.Helper()

	for fieldIndex, field := range prepared.Canonical.Fields {
		if !strings.Contains(field.OwnerID, ownerNeedle) ||
			!strings.Contains(field.Text, value) {
			continue
		}
		for _, term := range field.ProtectedTerms {
			if term.Value == value {
				t.Fatalf("unrelated field %q globally protected %q", field.ID, value)
			}
		}
		if !strings.Contains(prepared.Input.Fields[fieldIndex].Text, value) {
			t.Fatalf("unrelated field unexpectedly hid %q: %#v", value, prepared.Input.Fields[fieldIndex])
		}
		return
	}
	t.Fatalf("no unrelated presentation field containing %q matched %q", value, ownerNeedle)
}

func TestPresentationLocalizationProtectsLowercaseTaskIdentities(t *testing.T) {
	t.Parallel()

	data := &ReportData{
		RepoName: "fixture",
		TaskInvestigation: &TaskInvestigationWorkspace{
			TaskID:     "task-entrypoint",
			Repository: "fixture",
			Task:       "Inspect how serve reaches package main.",
			Interpretation: TaskInvestigationInterpretation{
				Restatement: "Trace serve through main.",
				Observable:  "The serve result is visible from main.",
				FoundTerms:  []string{"serve", "main"},
			},
			LikelyAreas: []TaskInvestigationArea{{
				Label: "Entrypoint",
				Why:   "serve is implemented in main.",
			}},
			Anchors: []TaskInvestigationAnchor{{
				Path:    "cmd/server.go",
				Symbol:  "serve",
				Package: "main",
				Why:     "serve is owned by main.",
				Source: SourceSnippet{
					Path:            "cmd/server.go",
					EnclosingSymbol: "serve",
				},
			}},
		},
	}
	prepared, err := PreparePresentationLocalization(data, localization.LocaleRussian)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"serve", "main"} {
		assertPresentationFieldProtectsValue(
			t,
			prepared,
			"task_investigation/task-entrypoint/anchors/",
			value,
		)
	}

	projected, _, err := ApplyPresentationLocalization(
		data,
		prepared,
		russianPresentationProjection(prepared),
	)
	if err != nil {
		t.Fatal(err)
	}
	why := projected.TaskInvestigation.Anchors[0].Why
	for _, value := range []string{"serve", "main"} {
		if !strings.Contains(why, value) {
			t.Fatalf("projected task anchor lost opaque identity %q: %q", value, why)
		}
	}
	anchor := projected.TaskInvestigation.Anchors[0]
	if anchor.Path != "cmd/server.go" || anchor.Symbol != "serve" ||
		anchor.Package != "main" || anchor.Source.EnclosingSymbol != "serve" {
		t.Fatalf("projection changed task anchor identity: %#v", anchor)
	}
}

func assertPresentationFieldProtectsValue(
	t *testing.T,
	prepared PreparedPresentationLocalization,
	ownerNeedle,
	value string,
) {
	t.Helper()

	for fieldIndex, field := range prepared.Canonical.Fields {
		if !strings.Contains(field.ID, ownerNeedle) ||
			!strings.Contains(field.Text, value) {
			continue
		}
		for _, term := range field.ProtectedTerms {
			if term.Value != value {
				continue
			}
			input := prepared.Input.Fields[fieldIndex]
			if strings.Contains(input.Text, value) ||
				!strings.Contains(input.Text, term.Token) {
				t.Fatalf(
					"translation input exposed %q or lost its placeholder: %#v",
					value,
					input,
				)
			}
			return
		}
		t.Fatalf("field %q did not protect %q: %#v", field.ID, value, field.ProtectedTerms)
	}
	t.Fatalf("no presentation field containing %q matched %q", value, ownerNeedle)
}
