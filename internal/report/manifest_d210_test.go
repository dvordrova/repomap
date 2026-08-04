package report

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlasstudy"
	"github.com/dvordrova/repomap/internal/repositoryatlas"
)

func TestD210ManifestAcceptsCompleteAndPartialAtlasStudyCoverage(t *testing.T) {
	t.Parallel()

	for _, state := range []atlasstudy.ProductState{
		atlasstudy.ProductStateAccepted,
		atlasstudy.ProductStateAcceptedPartial,
	} {
		state := state
		t.Run(string(state), func(t *testing.T) {
			t.Parallel()
			manifest, reportJSON := d210AtlasStudyManifestFixture(t, state)
			if err := manifest.VerifyReportJSON(reportJSON); err != nil {
				t.Fatalf("VerifyReportJSON: %v", err)
			}
		})
	}
}

func TestD210ManifestRejectsAtlasStudyCoverageDriftAndHistoricalProjection(t *testing.T) {
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
			name: "accepted collapses the model-selected stage onto accepted spans",
			mutate: func(report *ReportData) {
				report.AtlasStudy.ModelSelectedSpanCount = report.AtlasStudy.AcceptedSpanCount + 1
			},
			want: "complete Atlas Study projection",
		},
		{
			name: "accepted_partial only with a rejected sibling",
			mutate: func(report *ReportData) {
				report.AtlasStudy.State = atlasstudy.ProductStateAcceptedPartial
			},
			want: "accepted Atlas Study projection is invalid",
		},
		{
			name: "model-selected stage cannot exceed the advertised frontier",
			mutate: func(report *ReportData) {
				report.AtlasStudy.ModelSelectedSpanCount = report.AtlasStudy.AdvertisedSpanCount + 1
			},
			want: "accepted Atlas Study projection is invalid",
		},
		{
			name: "accepted span count must equal the direction counts",
			mutate: func(report *ReportData) {
				report.AtlasStudy.AcceptedSpanCount = report.AtlasStudy.DirectionCount + 1
			},
			want: "accepted Atlas Study projection is invalid",
		},
		{
			name: "advertised spans differ from candidate shelf",
			mutate: func(report *ReportData) {
				report.AtlasStudy.CandidateCoverage.SpansConsidered++
				report.AtlasStudy.CandidateCoverage.SpansSelected++
			},
			want: "accepted Atlas Study candidate/span counts do not match",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			manifest, encoded := d210AtlasStudyManifestFixture(t, atlasstudy.ProductStateAccepted)
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

func d210AtlasStudyManifestFixture(
	t *testing.T,
	state atlasstudy.ProductState,
) (RunManifest, []byte) {
	t.Helper()
	atlas := repositoryAtlasFixture()
	atlasJSON, err := repositoryatlas.CanonicalJSON(atlas)
	if err != nil {
		t.Fatal(err)
	}
	navigatorFixture := makeNavigatorArtifactFixture(t, atlas, "selected")
	// Four-stage projection: two considered spans, both advertised, one
	// returned direction locally accepted. The second advertised span receives
	// no returned direction — normal not_selected under D211 — and never turns
	// the accepted result into accepted_partial.
	status := &AtlasStudyReportStatus{
		Version: atlasstudy.ResultVersion, ProjectionVersion: AtlasStudyReportProjectionVersion,
		State: state,
		CandidateCoverage: &AtlasStudyCandidateCoverage{
			TargetsConsidered: 3, TargetsSelected: 2,
			SpansConsidered: 2, SpansSelected: 2, Complete: false,
			PerRole: []AtlasStudyRoleCandidateCoverage{
				{Role: atlasstudy.SupportEntryHandoff, Considered: 2, Selected: 1},
				{Role: atlasstudy.SupportProcessEntry, Considered: 1, Selected: 1},
			},
			PackageBuckets: []AtlasStudyAnonymousCoverage{
				{Considered: 1, Selected: 1},
				{Considered: 2, Selected: 1},
			},
		},
		DirectionCount: 1, PublishedDirectionCount: 1,
		ConsideredSpanCount:    2,
		AdvertisedSpanCount:    2,
		ModelSelectedSpanCount: 1,
		AcceptedSpanCount:      1,
		FrontierComplete:       true,
		SupportCoverageComplete: true,
	}
	if state == atlasstudy.ProductStateAccepted {
		status.SelectedItemsComplete = true
	} else {
		// A rejected returned sibling keeps the model-selected stage above the
		// locally accepted count while the flag stays false.
		status.ModelSelectedSpanCount = 2
	}
	reportJSON, err := json.Marshal(&ReportData{
		FormatVersion:   CurrentFormatVersion,
		RepositoryAtlas: &atlas,
		Navigator:       &navigatorFixture.projection,
		AtlasStudy:      status,
		StudyMap:        &RepositoryStudyMap{Directions: []StudyDirection{{ID: "direction-1"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	manifest := validRunManifestFixture(t)
	manifest.OpenablePaths, manifest.Components = nil, nil
	manifest.ReportSHA256 = manifestSHA256(reportJSON)
	manifest.MaterialInputs.RepositoryAtlasSHA256 = manifestSHA256(atlasJSON)
	manifest.MaterialInputs.NavigatorRequestSHA256 = manifestSHA256(navigatorFixture.request)
	manifest.MaterialInputs.NavigatorResultSHA256 = manifestSHA256(navigatorFixture.result)
	manifest.MaterialInputs.NavigatorStatusSHA256 = manifestSHA256(navigatorFixture.status)
	manifest.MaterialInputs.AtlasStudyRequestSHA256 = strings.Repeat("d", 64)
	manifest.MaterialInputs.AtlasStudyResultSHA256 = strings.Repeat("e", 64)
	manifest.MaterialInputs.AtlasStudyStatusSHA256 = strings.Repeat("f", 64)
	return manifest, reportJSON
}
