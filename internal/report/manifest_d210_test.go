package report

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlasstudy"
	"github.com/dvordrova/repomap/internal/mechanismstudy"
	"github.com/dvordrova/repomap/internal/repositoryatlas"
	"github.com/dvordrova/repomap/internal/themestudy"
)

// d210ThemeManifestFixture builds an accepted/accepted_partial theme-run
// manifest: the eight theme artifacts are written into a temp run dir and the
// report carries the re-based v8 Study projection derived from them.
func d210ThemeManifestFixture(t *testing.T, state atlasstudy.ProductState) (RunManifest, []byte, string) {
	t.Helper()
	data := atlasStudyReportFixture(t)
	data.CapturedRevision = strings.Repeat("a", 40)
	runDir := t.TempDir()
	writeThemeStudyAcceptedArtifacts(t, runDir, data)
	status, studyMap, err := readAtlasStudyReportProduct(runDir, data)
	if err != nil {
		t.Fatalf("read accepted product: %v", err)
	}
	if studyMap != nil {
		t.Fatal("theme run must not produce a RepositoryStudyMap")
	}
	if state == atlasstudy.ProductStateAcceptedPartial && status.State == atlasstudy.ProductStateAccepted {
		// Force the partial state: the report projection must agree with the
		// artifacts, so we re-derive with a partial Adjudication result by
		// re-reading after tampering the Adjudication result artifact.
		adjResultRaw := mustReadAtlasStudyFile(t, runDir, themestudy.AdjudicationResultArtifactFilename)
		var adjResult themestudy.AdjudicationResult
		if err := json.Unmarshal(adjResultRaw, &adjResult); err != nil {
			t.Fatal(err)
		}
		if len(adjResult.Themes) > 1 {
			adjResult.Themes = adjResult.Themes[:1]
		}
		adjResult.State = string(atlasstudy.ProductStateAcceptedPartial)
		adjResult.Status.Accepted = len(adjResult.Themes)
		encoded, err := json.Marshal(adjResult)
		if err != nil {
			t.Fatal(err)
		}
		writeThemeArtifact(t, runDir, themestudy.AdjudicationResultArtifactFilename, encoded)

		themesRaw := mustReadAtlasStudyFile(t, runDir, themestudy.StudyThemesArtifactFilename)
		themes, decodeErr := themestudy.DecodeStudyThemes(themesRaw)
		if decodeErr != nil {
			t.Fatal(decodeErr)
		}
		if len(themes.Cards) > 1 {
			themes.Cards = themes.Cards[:1]
		}
		themes.Omitted = 0
		themes.CoProjected = 0
		themesEncoded, encodeErr := themestudy.EncodeStudyThemes(themes)
		if encodeErr != nil {
			t.Fatal(encodeErr)
		}
		writeThemeArtifact(t, runDir, themestudy.StudyThemesArtifactFilename, themesEncoded)
		writePreparedStudyInvestigationArtifacts(t, runDir, data, strings.Repeat("e", 64))

		status, studyMap, err = readAtlasStudyReportProduct(runDir, data)
		if err != nil {
			t.Fatalf("re-read partial product: %v", err)
		}
		if studyMap != nil || status == nil || status.State != atlasstudy.ProductStateAcceptedPartial {
			t.Fatalf("partial re-derivation failed: %#v / %#v", status, studyMap)
		}
	}
	data.FormatVersion = CurrentFormatVersion
	data.AtlasStudy, data.StudyMap = status, studyMap
	// The manifest validates the navigation projection against the canvas, so
	// derive it exactly like a real run does.
	navigation, err := ProjectArchitectureComponentNavigation(data.ArchitectureCanvas, data.OpenablePaths)
	if err != nil {
		t.Fatalf("ProjectArchitectureComponentNavigation: %v", err)
	}
	data.ArchitectureComponentNavigation = navigation
	reportJSON, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	// Bind the repository Atlas artifact exactly as a real run does.
	atlasJSON, err := repositoryatlas.CanonicalJSON(*data.RepositoryAtlas)
	if err != nil {
		t.Fatal(err)
	}
	writeThemeArtifact(t, runDir, repositoryatlas.ArtifactFilename, atlasJSON)
	material := MaterialInputs{
		RepositoryAtlasSHA256:          manifestSHA256(atlasJSON),
		ThemeScoutRequestSHA256:        manifestSHA256(mustReadAtlasStudyFile(t, runDir, themestudy.ScoutRequestArtifactFilename)),
		ThemeScoutResultSHA256:         manifestSHA256(mustReadAtlasStudyFile(t, runDir, themestudy.ScoutResultArtifactFilename)),
		ThemeScoutStatusSHA256:         manifestSHA256(mustReadAtlasStudyFile(t, runDir, themestudy.ScoutStatusArtifactFilename)),
		ThemeSourceExpansionSHA256:     manifestSHA256(mustReadAtlasStudyFile(t, runDir, themestudy.ExpansionArtifactFilename)),
		ThemeAdjudicationRequestSHA256: manifestSHA256(mustReadAtlasStudyFile(t, runDir, themestudy.AdjudicationRequestArtifactFilename)),
		ThemeAdjudicationResultSHA256:  manifestSHA256(mustReadAtlasStudyFile(t, runDir, themestudy.AdjudicationResultArtifactFilename)),
		ThemeAdjudicationStatusSHA256:  manifestSHA256(mustReadAtlasStudyFile(t, runDir, themestudy.AdjudicationStatusArtifactFilename)),
		StudyThemesSHA256:              manifestSHA256(mustReadAtlasStudyFile(t, runDir, themestudy.StudyThemesArtifactFilename)),
	}
	manifest := validRunManifestFixture(t)
	manifest.RepositoryState.Head = data.CapturedRevision
	repositoryDigest, err := manifest.RepositoryState.Digest()
	if err != nil {
		t.Fatalf("digest theme manifest repository state: %v", err)
	}
	manifest.RepositoryStateSHA256 = repositoryDigest
	manifest.MaterialInputs.SelectedRevision = data.CapturedRevision
	writePreparedStudyInvestigationArtifacts(t, runDir, data, repositoryDigest)
	manifest.OpenablePaths = append([]string(nil), data.OpenablePaths...)
	manifest.Components = nil
	manifest.ReportSHA256 = manifestSHA256(reportJSON)
	manifest.MaterialInputs.RepositoryAtlasSHA256 = material.RepositoryAtlasSHA256
	manifest.MaterialInputs.ThemeScoutRequestSHA256 = material.ThemeScoutRequestSHA256
	manifest.MaterialInputs.ThemeScoutResultSHA256 = material.ThemeScoutResultSHA256
	manifest.MaterialInputs.ThemeScoutStatusSHA256 = material.ThemeScoutStatusSHA256
	manifest.MaterialInputs.ThemeSourceExpansionSHA256 = material.ThemeSourceExpansionSHA256
	manifest.MaterialInputs.ThemeAdjudicationRequestSHA256 = material.ThemeAdjudicationRequestSHA256
	manifest.MaterialInputs.ThemeAdjudicationResultSHA256 = material.ThemeAdjudicationResultSHA256
	manifest.MaterialInputs.ThemeAdjudicationStatusSHA256 = material.ThemeAdjudicationStatusSHA256
	manifest.MaterialInputs.StudyThemesSHA256 = material.StudyThemesSHA256
	manifest.MaterialInputs.StudyInvestigationFactsSHA256 = manifestSHA256(
		mustReadAtlasStudyFile(t, runDir, mechanismstudy.FactsArtifactFilename),
	)
	manifest.MaterialInputs.StudyInvestigationCandidatesSHA256 = manifestSHA256(
		mustReadAtlasStudyFile(t, runDir, mechanismstudy.CandidatesArtifactFilename),
	)
	manifest.MaterialInputs.StudyInvestigationResultSHA256 = manifestSHA256(
		mustReadAtlasStudyFile(t, runDir, mechanismstudy.ResultArtifactFilename),
	)
	manifest.MaterialInputs.StudyInvestigationStatusSHA256 = manifestSHA256(
		mustReadAtlasStudyFile(t, runDir, mechanismstudy.StatusArtifactFilename),
	)
	return manifest, reportJSON, runDir
}

func TestD210ManifestAcceptsCompleteAndPartialThemeStudy(t *testing.T) {
	t.Parallel()

	for _, state := range []atlasstudy.ProductState{
		atlasstudy.ProductStateAccepted,
		atlasstudy.ProductStateAcceptedPartial,
	} {
		state := state
		t.Run(string(state), func(t *testing.T) {
			t.Parallel()
			manifest, reportJSON, _ := d210ThemeManifestFixture(t, state)
			if err := manifest.VerifyReportJSON(reportJSON); err != nil {
				t.Fatalf("VerifyReportJSON: %v", err)
			}
		})
	}
}

func TestD210ManifestRejectsThemeStudyDriftAndHistoricalProjection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*ReportData)
		want   string
	}{
		{
			name: "historical projection",
			mutate: func(report *ReportData) {
				report.AtlasStudy.ProjectionVersion--
			},
			want: "projection is incomplete",
		},
		{
			name: "scout-anchored stage cannot exceed the seed-advertised frontier",
			mutate: func(report *ReportData) {
				report.AtlasStudy.ModelSelectedSpanCount = report.AtlasStudy.AdvertisedSpanCount + 1
			},
			want: "accepted Atlas Study projection is invalid",
		},
		{
			name: "published stage cannot exceed scout-anchored",
			mutate: func(report *ReportData) {
				report.AtlasStudy.AcceptedSpanCount = report.AtlasStudy.ModelSelectedSpanCount + 1
			},
			want: "accepted Atlas Study projection is invalid",
		},
		{
			name: "accepted_partial only with a rejected sibling",
			mutate: func(report *ReportData) {
				report.AtlasStudy.State = atlasstudy.ProductStateAcceptedPartial
			},
			want: "accepted Atlas Study projection is invalid",
		},
		{
			name: "browse total must equal the considered count",
			mutate: func(report *ReportData) {
				report.AtlasStudy.FrontierBrowse.Total++
			},
			want: "accepted Atlas Study browse projection is invalid",
		},
		{
			name: "theme card must carry prose and readings",
			mutate: func(report *ReportData) {
				report.AtlasStudy.Themes.Cards[0].FinalTitle = ""
			},
			want: "accepted Atlas Study theme card is invalid",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			manifest, encoded, _ := d210ThemeManifestFixture(t, atlasstudy.ProductStateAccepted)
			var report ReportData
			if err := json.Unmarshal(encoded, &report); err != nil {
				t.Fatal(err)
			}
			test.mutate(&report)
			encoded, err := json.Marshal(report)
			if err != nil {
				t.Fatal(err)
			}
			manifest.ReportSHA256 = manifestSHA256(encoded)
			if err := manifest.VerifyReportJSON(encoded); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("VerifyReportJSON error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestD210ManifestRejectsLegacySingleStageBindingInThemeRun(t *testing.T) {
	t.Parallel()

	manifest, reportJSON, _ := d210ThemeManifestFixture(t, atlasstudy.ProductStateAccepted)
	// A legacy atlas-study artifact digest bound in the manifest must fail
	// closed: a run with both the retired single stage and the theme stages
	// would have three Study semantic calls, which Decision 213 forbids.
	manifest.MaterialInputs.AtlasStudyRequestSHA256 = strings.Repeat("d", 64)
	manifest.MaterialInputs.AtlasStudyResultSHA256 = strings.Repeat("e", 64)
	manifest.MaterialInputs.AtlasStudyStatusSHA256 = strings.Repeat("f", 64)
	manifest.ReportSHA256 = manifestSHA256(reportJSON)
	if err := manifest.VerifyReportJSON(reportJSON); err == nil ||
		!strings.Contains(err.Error(), "legacy atlas-study artifact") {
		t.Fatalf("legacy single-stage binding error = %v", err)
	}
}

func TestD246ManifestRejectsStudyThemesFromAnotherRepositoryRevision(t *testing.T) {
	manifest, reportJSON, runDir := d210ThemeManifestFixture(t, atlasstudy.ProductStateAccepted)

	themesRaw := mustReadAtlasStudyFile(t, runDir, themestudy.StudyThemesArtifactFilename)
	themes, err := themestudy.DecodeStudyThemes(themesRaw)
	if err != nil {
		t.Fatal(err)
	}
	themes.Revision = strings.Repeat("f", 40)
	themesRaw, err = themestudy.EncodeStudyThemes(themes)
	if err != nil {
		t.Fatal(err)
	}
	writeThemeArtifact(t, runDir, themestudy.StudyThemesArtifactFilename, themesRaw)
	manifest.MaterialInputs.StudyThemesSHA256 = manifestSHA256(themesRaw)

	// Rebind the report-side revision too, isolating the manifest's repository
	// authority check from the report hydration mismatch check.
	var data ReportData
	if err := json.Unmarshal(reportJSON, &data); err != nil {
		t.Fatal(err)
	}
	data.CapturedRevision = themes.Revision
	reportJSON, err = json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	manifest.ReportSHA256 = manifestSHA256(reportJSON)
	if err := manifest.VerifyThemesArtifacts(runDir, reportJSON); err == nil ||
		!strings.Contains(err.Error(), "does not match authorized repository revision") {
		t.Fatalf("cross-revision Study themes error = %v", err)
	}
}

func TestD210CandidateCoverageProjectionKeepsExactCountsWithoutPrivateBucketIDs(t *testing.T) {
	t.Parallel()

	projected, err := projectAtlasStudyCandidateCoverage(atlasstudy.CandidateCoverage{
		CandidateSHA256:   strings.Repeat("a", 64),
		TargetsConsidered: 5, TargetsSelected: 3,
		SpansConsidered: 4, SpansSelected: 2, Complete: false,
		PerRole: []atlasstudy.CandidateCoverageCount{
			{Key: string(atlasstudy.SupportSurface), Considered: 2, Selected: 1},
			{Key: string(atlasstudy.SupportProcessEntry), Considered: 3, Selected: 2},
		},
		PerPackage: []atlasstudy.CandidateCoverageCount{
			{Key: "private/package-b", Considered: 1, Selected: 1},
			{Key: "private/package-a", Considered: 4, Selected: 2},
		},
	})
	if err != nil {
		t.Fatalf("projectAtlasStudyCandidateCoverage: %v", err)
	}
	encoded, err := json.Marshal(projected)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "private/package") || strings.Contains(string(encoded), strings.Repeat("a", 64)) {
		t.Fatalf("public candidate coverage leaked private identity: %s", encoded)
	}
	if projected.TargetsConsidered != 5 || projected.TargetsSelected != 3 ||
		projected.SpansConsidered != 4 || projected.SpansSelected != 2 || projected.Complete ||
		len(projected.PerRole) != 2 || len(projected.PackageBuckets) != 2 {
		t.Fatalf("projected candidate coverage = %#v", projected)
	}
}

// keep the repositoryatlas import used by the fixture builder.
var _ = repositoryatlas.CanonicalJSON
