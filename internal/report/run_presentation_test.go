package report

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/localization"
)

func TestPrepareRunPresentationOwnsLocalizableEnrichmentOnly(t *testing.T) {
	t.Parallel()

	canonical := &ReportData{
		FormatVersion: CurrentFormatVersion,
		RepoName:      "example.test/coherent",
		ProjectGuess:  "repository orientation",
		OpenablePaths: []string{"README.md"},
		ArchitectureCanvas: &ArchitectureCanvas{
			Title:    "Repository architecture",
			Subtitle: "A bounded architecture view.",
			Subsystems: []ArchitectureSubsystem{{
				ID:           "subsystem-core",
				Name:         "Core subsystem",
				Description:  "Owns the central behavior.",
				ComponentIDs: []componentmap.ComponentID{"component-core"},
			}},
			Components: []ArchitectureComponent{{
				ID:          "component-core",
				SubsystemID: "subsystem-core",
				Name:        "Core component",
				Description: "Coordinates the example service.",
			}},
		},
	}
	canonicalJSON, err := json.Marshal(canonical)
	if err != nil {
		t.Fatal(err)
	}
	direct, err := PreparePresentationLocalization(
		canonical,
		localization.LocaleRussian,
	)
	if err != nil {
		t.Fatal(err)
	}

	preparedReport, err := PrepareRunPresentation(t.TempDir(), canonical, nil)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := PreparePresentationLocalization(
		preparedReport,
		localization.LocaleRussian,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared.Input.Fields) <= len(direct.Input.Fields) ||
		prepared.Canonical.SHA256 == direct.Canonical.SHA256 {
		t.Fatalf(
			"coherent inventory did not expose the old mismatch: direct=%d/%s prepared=%d/%s",
			len(direct.Input.Fields),
			direct.Canonical.SHA256,
			len(prepared.Input.Fields),
			prepared.Canonical.SHA256,
		)
	}
	assertReportJSONUnchanged(t, canonical, canonicalJSON)

	catalog := exactSearchTestCatalog(t, canonical.OpenablePaths)
	if err := AttachExactWorkspaceSearch(preparedReport, catalog); err != nil {
		t.Fatal(err)
	}
	withSearch, err := PreparePresentationLocalization(
		preparedReport,
		localization.LocaleRussian,
	)
	if err != nil {
		t.Fatal(err)
	}
	if preparedReport.SemanticSearch == nil {
		t.Fatal("exact workspace search was not attached")
	}
	if withSearch.Canonical.SHA256 != prepared.Canonical.SHA256 ||
		!reflect.DeepEqual(
			presentationLocalizationFieldIDs(withSearch),
			presentationLocalizationFieldIDs(prepared),
		) {
		t.Fatalf(
			"opaque exact search changed localization identity: before=%d/%s after=%d/%s",
			len(prepared.Input.Fields),
			prepared.Canonical.SHA256,
			len(withSearch.Input.Fields),
			withSearch.Canonical.SHA256,
		)
	}
	assertReportJSONUnchanged(t, canonical, canonicalJSON)
}

func TestPrepareRunPresentationSourceEpisodeOrderingIsStable(t *testing.T) {
	t.Parallel()

	raw, episode := readSourceEpisodeFixture(
		t,
		filepath.Join(
			"..",
			"..",
			"experiments",
			"source-episode",
			"etcd-put",
			"episode.json",
		),
	)
	canonical := authorizedSourceEpisodeReport(episode)
	canonical.SourceIDs = nil
	canonicalJSON, err := json.Marshal(canonical)
	if err != nil {
		t.Fatal(err)
	}

	producer, err := PrepareRunPresentation(t.TempDir(), canonical, raw)
	if err != nil {
		t.Fatal(err)
	}
	server, err := PrepareRunPresentation(t.TempDir(), canonical, nil)
	if err != nil {
		t.Fatal(err)
	}
	server.SourceIDs = make(map[string]string, len(episode.Anchors))
	for index, anchor := range episode.Anchors {
		server.SourceIDs[anchor.Path] = string(rune('a'+index)) + "-opaque-source"
	}
	if err := AttachSourceEpisodePresentation(server, raw); err != nil {
		t.Fatal(err)
	}
	producerPrepared, err := PreparePresentationLocalization(
		producer,
		localization.LocaleRussian,
	)
	if err != nil {
		t.Fatal(err)
	}
	serverPrepared, err := PreparePresentationLocalization(
		server,
		localization.LocaleRussian,
	)
	if err != nil {
		t.Fatal(err)
	}
	if producerPrepared.Canonical.SHA256 != serverPrepared.Canonical.SHA256 ||
		!reflect.DeepEqual(
			presentationLocalizationFieldIDs(producerPrepared),
			presentationLocalizationFieldIDs(serverPrepared),
		) {
		t.Fatalf(
			"source-episode attachment order changed localization identity: producer=%d/%s server=%d/%s",
			len(producerPrepared.Input.Fields),
			producerPrepared.Canonical.SHA256,
			len(serverPrepared.Input.Fields),
			serverPrepared.Canonical.SHA256,
		)
	}
	assertReportJSONUnchanged(t, canonical, canonicalJSON)
}

func presentationLocalizationFieldIDs(
	prepared PreparedPresentationLocalization,
) []string {
	ids := make([]string, 0, len(prepared.Input.Fields))
	for _, field := range prepared.Input.Fields {
		ids = append(ids, field.ID)
	}
	return ids
}

func assertReportJSONUnchanged(t *testing.T, data *ReportData, want []byte) {
	t.Helper()
	got, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("presentation preparation mutated canonical report state")
	}
}
