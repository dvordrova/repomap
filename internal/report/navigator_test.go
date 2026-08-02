package report

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/navigator"
	"github.com/dvordrova/repomap/internal/repositoryatlas"
)

type navigatorArtifactFixture struct {
	projection NavigatorReportProduct
	request    []byte
	result     []byte
	status     []byte
}

func TestNavigatorReportArtifactsBindOfflineEmptyAndSelectedStates(t *testing.T) {
	tests := []struct {
		name            string
		state           navigator.ProductState
		atlas           repositoryatlas.Atlas
		wantRequest     bool
		wantResult      bool
		wantUnavailable navigator.UnavailableCode
		wantSelected    bool
	}{
		{
			name: "offline", state: navigator.ProductStateUnavailable,
			atlas: repositoryAtlasFixture(), wantRequest: true,
			wantUnavailable: navigator.UnavailableOffline,
		},
		{
			name: "empty", state: navigator.ProductStateEmpty,
			atlas: repositoryAtlasWithoutStartup(), wantResult: true,
		},
		{
			name: "selected", state: navigator.ProductStateSelected,
			atlas: repositoryAtlasFixture(), wantRequest: true, wantResult: true, wantSelected: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			atlasJSON, err := repositoryatlas.CanonicalJSON(test.atlas)
			if err != nil {
				t.Fatal(err)
			}
			atlas, err := repositoryatlas.DecodeCanonicalJSON(atlasJSON)
			if err != nil {
				t.Fatal(err)
			}
			fixture := makeNavigatorArtifactFixture(t, atlas, test.state)
			runDir := t.TempDir()
			writeTestFile(t, runDir, repositoryatlas.ArtifactFilename, string(atlasJSON))
			writeNavigatorArtifactFixture(t, runDir, fixture)

			projected, err := readNavigatorReportProduct(runDir, &atlas)
			if err != nil {
				t.Fatalf("readNavigatorReportProduct: %v", err)
			}
			if projected.State != test.state || projected.UnavailableCode != test.wantUnavailable ||
				(projected.Recommendation != nil) != test.wantSelected {
				t.Fatalf("projection = %#v", projected)
			}

			reportJSON, err := json.Marshal(&ReportData{
				FormatVersion:   CurrentFormatVersion,
				RepositoryAtlas: &atlas,
				Navigator:       projected,
			})
			if err != nil {
				t.Fatal(err)
			}
			manifest := validRunManifestFixture(t)
			if manifest.Version != 8 || manifest.ReportFormatVersion != 28 {
				t.Fatalf("Atlas-first wire versions = %d/%d", manifest.Version, manifest.ReportFormatVersion)
			}
			manifest.OpenablePaths = nil
			manifest.Components = nil
			manifest.ReportSHA256 = manifestSHA256(reportJSON)
			manifest.MaterialInputs.RepositoryAtlasSHA256 = manifestSHA256(atlasJSON)
			manifest.MaterialInputs.NavigatorRequestSHA256 = optionalArtifactDigest(fixture.request)
			manifest.MaterialInputs.NavigatorResultSHA256 = optionalArtifactDigest(fixture.result)
			manifest.MaterialInputs.NavigatorStatusSHA256 = manifestSHA256(fixture.status)
			if (fixture.request != nil) != test.wantRequest || (fixture.result != nil) != test.wantResult {
				t.Fatalf("artifact shape request=%t result=%t", fixture.request != nil, fixture.result != nil)
			}
			if err := manifest.VerifyReportJSON(reportJSON); err != nil {
				t.Fatalf("VerifyReportJSON: %v", err)
			}
			if err := manifest.VerifyRepositoryAtlasArtifact(runDir, reportJSON); err != nil {
				t.Fatalf("VerifyRepositoryAtlasArtifact: %v", err)
			}
			if err := manifest.VerifyNavigatorArtifacts(runDir, reportJSON); err != nil {
				t.Fatalf("VerifyNavigatorArtifacts: %v", err)
			}
			if test.state == navigator.ProductStateUnavailable {
				withoutRequest := manifest
				withoutRequest.MaterialInputs.NavigatorRequestSHA256 = ""
				if err := withoutRequest.VerifyReportJSON(reportJSON); err == nil ||
					!strings.Contains(err.Error(), "unavailable Navigator report projection is invalid") {
					t.Fatalf("offline projection without request digest error = %v", err)
				}
			}
			if err := os.WriteFile(
				filepath.Join(runDir, navigator.StatusArtifactFilename),
				append(append([]byte(nil), fixture.status...), '\n'),
				0o600,
			); err != nil {
				t.Fatal(err)
			}
			if err := manifest.VerifyNavigatorArtifacts(runDir, reportJSON); err == nil ||
				!strings.Contains(err.Error(), "status.v1.json sha256 mismatch") {
				t.Fatalf("tampered status error = %v", err)
			}
		})
	}
}

func TestNavigatorReportArtifactsRejectIncompleteClosedStates(t *testing.T) {
	atlas := repositoryAtlasFixture()
	offline := makeNavigatorArtifactFixture(t, atlas, navigator.ProductStateUnavailable)
	runDir := t.TempDir()
	writeNavigatorArtifactFixture(t, runDir, offline)
	if err := os.Remove(filepath.Join(runDir, navigator.RequestArtifactFilename)); err != nil {
		t.Fatal(err)
	}
	if _, err := readNavigatorReportProduct(runDir, &atlas); err == nil ||
		!strings.Contains(err.Error(), "requires its exact request artifact") {
		t.Fatalf("missing offline request error = %v", err)
	}

	emptyAtlas := repositoryAtlasWithoutStartup()
	empty := makeNavigatorArtifactFixture(t, emptyAtlas, navigator.ProductStateEmpty)
	runDir = t.TempDir()
	writeNavigatorArtifactFixture(t, runDir, empty)
	writeTestFile(t, runDir, navigator.RequestArtifactFilename, string(offline.request))
	if _, err := readNavigatorReportProduct(runDir, &emptyAtlas); err == nil ||
		!strings.Contains(err.Error(), "must not have a provider request artifact") {
		t.Fatalf("empty request error = %v", err)
	}
}

func TestReadRunDirSkipsLegacyOrientationOnlyForAtlasFirstArtifacts(t *testing.T) {
	atlas := repositoryatlas.Atlas{
		Version: repositoryatlas.Version,
		Units: []repositoryatlas.Unit{{
			ID: "repository", Kind: repositoryatlas.UnitRepository, Name: "fixture",
		}},
	}
	atlasJSON, err := repositoryatlas.CanonicalJSON(atlas)
	if err != nil {
		t.Fatal(err)
	}
	fixture := makeNavigatorArtifactFixture(t, atlas, navigator.ProductStateEmpty)
	runDir := t.TempDir()
	writeTestFile(t, runDir, repositoryatlas.ArtifactFilename, string(atlasJSON))
	writeNavigatorArtifactFixture(t, runDir, fixture)

	data, err := ReadRunDir(runDir)
	if err != nil {
		t.Fatalf("ReadRunDir Atlas-first: %v", err)
	}
	if data.Navigator == nil || data.Navigator.State != navigator.ProductStateEmpty {
		t.Fatalf("Navigator = %#v", data.Navigator)
	}
	for _, warning := range data.Warnings {
		if strings.Contains(warning, "orientation_report.json") {
			t.Fatalf("Atlas-first warnings include obsolete orientation artifact: %q", warning)
		}
	}

	writeTestFile(t, runDir, navigator.StatusArtifactFilename, `{}`)
	if _, err := ReadRunDir(runDir); err == nil || !strings.Contains(err.Error(), "navigator report: status") {
		t.Fatalf("invalid Atlas-first status error = %v", err)
	}

	legacy, err := ReadRunDir(t.TempDir())
	if err != nil {
		t.Fatalf("ReadRunDir legacy: %v", err)
	}
	for _, warning := range legacy.Warnings {
		if strings.Contains(warning, "orientation_report.json") {
			return
		}
	}
	t.Fatal("legacy run without orientation_report.json lost its warning")
}

func makeNavigatorArtifactFixture(
	t *testing.T,
	atlas repositoryatlas.Atlas,
	state navigator.ProductState,
) navigatorArtifactFixture {
	t.Helper()
	product, err := navigator.CompileProduct(navigator.ProductInput{
		Atlas: atlas,
		Limits: navigator.Limits{
			MaxWireBytes: 1 << 20, MaxResponseBytes: 1 << 20, MaxUnitLabelBytes: 4096,
			MaxSeeds: 64, MaxDirectTrails: 128, MaxIntersections: 64,
			MaxEvidence: 256, MaxGaps: 64, MaxActions: 64,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture := navigatorArtifactFixture{}
	var status navigator.Status
	switch state {
	case navigator.ProductStateEmpty:
		record, recordErr := product.EmptyRecord()
		if recordErr != nil {
			t.Fatal(recordErr)
		}
		fixture.result, err = navigator.EncodeRecommendationRecord(record)
		status = product.PreparedStatus()
	case navigator.ProductStateUnavailable:
		request, requestErr := product.RequestRecord()
		if requestErr != nil {
			t.Fatal(requestErr)
		}
		fixture.request, err = navigator.EncodeRequestRecord(request)
		if err == nil {
			status, err = product.UnavailableStatus(navigator.UnavailableOffline)
		}
	case navigator.ProductStateSelected:
		request, requestErr := product.RequestRecord()
		if requestErr != nil {
			t.Fatal(requestErr)
		}
		fixture.request, err = navigator.EncodeRequestRecord(request)
		if err != nil {
			break
		}
		selected := request.Actions[0]
		record := navigator.RecommendationRecord{
			Version: navigator.ProductVersion, State: navigator.ProductStateSelected,
			AtlasSHA256: request.AtlasSHA256, WireSHA256: request.WireSHA256,
			CatalogSHA256: request.CatalogSHA256, CatalogRef: request.CatalogRef,
			Question: request.Question, Actions: request.Actions, Selected: &selected,
		}
		fixture.result, err = navigator.EncodeRecommendationRecord(record)
		if err == nil {
			status, err = product.SelectedStatus(record)
		}
	default:
		t.Fatalf("unsupported fixture state %q", state)
	}
	if err != nil {
		t.Fatal(err)
	}
	fixture.status, err = navigator.EncodeStatus(status)
	if err != nil {
		t.Fatal(err)
	}
	projected := NavigatorReportProduct{
		Version: navigator.ProductVersion, State: status.State,
		UnavailableCode: status.UnavailableCode,
	}
	if state == navigator.ProductStateSelected {
		result, decodeErr := navigator.DecodeRecommendationRecord(fixture.result)
		if decodeErr != nil {
			t.Fatal(decodeErr)
		}
		selected := *result.Selected
		projected.Recommendation = &selected
	}
	fixture.projection = projected
	return fixture
}

func writeNavigatorArtifactFixture(t *testing.T, runDir string, fixture navigatorArtifactFixture) {
	t.Helper()
	for name, data := range map[string][]byte{
		navigator.RequestArtifactFilename: fixture.request,
		navigator.RecordArtifactFilename:  fixture.result,
		navigator.StatusArtifactFilename:  fixture.status,
	} {
		if data == nil {
			continue
		}
		if err := os.WriteFile(filepath.Join(runDir, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func repositoryAtlasWithoutStartup() repositoryatlas.Atlas {
	atlas := repositoryAtlasFixture()
	atlas.Relations = nil
	return atlas
}

func optionalArtifactDigest(data []byte) string {
	if data == nil {
		return ""
	}
	return manifestSHA256(data)
}
