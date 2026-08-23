package report

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"

	"github.com/dvordrova/repomap/internal/analysistarget"
)

// OrdinaryReportHTMLAuthority carries the local rendering authority that is
// deliberately absent from canonical report.json.
type OrdinaryReportHTMLAuthority struct {
	TargetNavigation *TargetNavigationPortfolio
	StandaloneSource *StandaloneSourceAuthority
	ArtifactsDir     string
	AnalysisRoot     string
	RepositoryRoot   string
}

// VerifyOrdinaryReportHTMLPayload verifies that one ordinary report page
// embeds exactly the canonical browser projection of its manifest-validated
// report.json. External source routing and sibling navigation must exactly
// match the caller's manifest-derived authority.
func VerifyOrdinaryReportHTMLPayload(
	htmlBytes, reportJSON []byte,
	authority OrdinaryReportHTMLAuthority,
) error {
	data, err := decodeStrictReportJSON(reportJSON)
	if err != nil {
		return fmt.Errorf("report: verify html report data: %w", err)
	}
	if err := validateProgramPresentation(&data); err != nil {
		return fmt.Errorf("report: verify html report data: %w", err)
	}

	payloadJSON, err := extractOrdinaryReportHTMLPayload(htmlBytes)
	if err != nil {
		return err
	}
	var payload programShellPayload
	if err := decodeStrictProgramShellPayload(payloadJSON, &payload); err != nil {
		return err
	}
	if err := payload.GitHubSourceLinks.validate(); err != nil {
		return fmt.Errorf("report: embedded GitHub source links: %w", err)
	}
	if err := payload.GitLabSourceLinks.validate(); err != nil {
		return fmt.Errorf("report: embedded GitLab source links: %w", err)
	}
	if payload.GitHubSourceLinks != nil && payload.GitLabSourceLinks != nil {
		return fmt.Errorf("report: embedded payload contains multiple external source hosts")
	}
	expectedGitHub, expectedGitLab, err := ordinaryHTMLSourceLinks(data.CapturedRevision, authority)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(payload.GitHubSourceLinks, expectedGitHub) ||
		!reflect.DeepEqual(payload.GitLabSourceLinks, expectedGitLab) {
		return fmt.Errorf("report: embedded external source links do not match manifest authority")
	}
	if err := validateTargetNavigation(&data, authority.TargetNavigation); err != nil {
		return fmt.Errorf("report: expected target navigation: %w", err)
	}
	if err := validateTargetNavigation(&data, payload.TargetNavigation); err != nil {
		return fmt.Errorf("report: embedded target navigation: %w", err)
	}
	if !reflect.DeepEqual(payload.TargetNavigation, authority.TargetNavigation) {
		return fmt.Errorf("report: embedded target navigation does not match manifest authority")
	}

	// Source routing and sibling navigation intentionally do not live in
	// report.json. Project the validated render-only values over the canonical
	// report and require every other browser field to remain exactly equal.
	expectedData := data
	expectedData.GitHubSourceLinks = expectedGitHub
	expectedData.GitLabSourceLinks = expectedGitLab
	expected := programShellPayloadForReport(&expectedData, authority.TargetNavigation)
	localRoots := []string{authority.ArtifactsDir}
	if expectedGitHub != nil || expectedGitLab != nil {
		localRoots = append(localRoots, authority.AnalysisRoot, authority.RepositoryRoot)
	}
	expectedJSON, err := marshalHTMLPayloadWithLocalRoots(expected, localRoots)
	if err != nil {
		return fmt.Errorf("report: encode expected html payload: %w", err)
	}
	var renderedExpected programShellPayload
	if err := decodeStrictProgramShellPayload(expectedJSON, &renderedExpected); err != nil {
		return fmt.Errorf("report: decode expected html payload: %w", err)
	}
	if !reflect.DeepEqual(payload, renderedExpected) {
		return fmt.Errorf("report: embedded report payload does not match report.json")
	}
	return nil
}

func ordinaryHTMLSourceLinks(
	revision string,
	authority OrdinaryReportHTMLAuthority,
) (*GitHubSourceLinks, *GitLabSourceLinks, error) {
	if authority.StandaloneSource == nil {
		return nil, nil, nil
	}
	if err := authority.StandaloneSource.validate(); err != nil {
		return nil, nil, err
	}
	prefix, err := standaloneSourcePathPrefix(authority.RepositoryRoot, authority.AnalysisRoot)
	if err != nil {
		return nil, nil, fmt.Errorf("report: embedded source path authority: %w", err)
	}
	switch authority.StandaloneSource.Host {
	case "GitHub":
		links, err := newGitHubSourceLinks(
			authority.StandaloneSource.RepositoryURL, revision, prefix,
		)
		return links, nil, err
	case "GitLab":
		links, err := newGitLabSourceLinks(
			authority.StandaloneSource.RepositoryURL, revision, prefix,
		)
		return nil, links, err
	default:
		return nil, nil, fmt.Errorf("report: unsupported manifest source host")
	}
}

func extractOrdinaryReportHTMLPayload(htmlBytes []byte) ([]byte, error) {
	if !bytes.Contains(bytes.ToLower(htmlBytes), []byte("<html")) {
		return nil, fmt.Errorf("report: ordinary html is missing the report application shell")
	}
	opening := []byte(reportDataScriptOpen)
	if bytes.Count(htmlBytes, opening) != 1 {
		return nil, fmt.Errorf("report: ordinary html must contain exactly one report data payload")
	}
	start := bytes.Index(htmlBytes, opening) + len(opening)
	remaining := htmlBytes[start:]
	closing := []byte(reportDataScriptClose)
	end := bytes.Index(remaining, closing)
	if end < 0 {
		return nil, fmt.Errorf("report: ordinary html report data payload is unterminated")
	}
	payload := remaining[:end]
	if len(bytes.TrimSpace(payload)) == 0 {
		return nil, fmt.Errorf("report: ordinary html report data payload is empty")
	}
	return payload, nil
}

func decodeStrictProgramShellPayload(raw []byte, payload *programShellPayload) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(payload); err != nil {
		return fmt.Errorf("report: decode embedded report payload: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("report: embedded report payload has multiple json values")
		}
		return fmt.Errorf("report: embedded report payload has trailing data: %w", err)
	}
	return nil
}

func programShellPayloadForReport(
	data *ReportData,
	navigation *TargetNavigationPortfolio,
) programShellPayload {
	return programShellPayload{
		FormatVersion:          data.FormatVersion,
		RepoName:               data.RepoName,
		AnalysisTarget:         analysisTargetForBrowser(data),
		ProgramPortfolio:       data.ProgramPortfolio,
		CubeMapView:            cubeMapViewForBrowser(data.CubeMapView),
		CoreMapView:            coreMapViewForBrowser(data.CoreMapView),
		ActivityEntrypointView: data.ActivityEntrypointView,
		IntegrationUsageView:   integrationUsageViewForBrowser(data.IntegrationUsageView),
		ActivityPathView:       activityPathViewForBrowser(data.ActivityPathView),
		OpenablePaths:          append([]string{}, data.OpenablePaths...),
		SourceIDs:              data.SourceIDs,
		GitHubSourceLinks:      data.GitHubSourceLinks,
		GitLabSourceLinks:      data.GitLabSourceLinks,
		CapturedRevision:       data.CapturedRevision,
		CapturedInputCount:     data.CapturedInputCount,
		Warnings:               append([]string(nil), data.Warnings...),
		TargetNavigation:       navigation,
	}
}

// AnalysisTarget is outer Go page authority, not part of the language-neutral
// ProgramIndex/CoreMap browser contract. Retain it only for the retired
// CubeMap browser shape while that reader remains; otherwise the browser must
// see the same semantic payload for Go and Python.
func analysisTargetForBrowser(data *ReportData) *analysistarget.Target {
	if data == nil || data.CubeMapView == nil {
		return nil
	}
	return data.AnalysisTarget
}

// The canonical report and manifest retain producer digests. They are not
// browser joins: the browser has no independent artifact bytes against which
// to verify them, so publishing them would only expose dead provenance fields.
func cubeMapViewForBrowser(value *CubeMapView) *CubeMapView {
	if value == nil {
		return nil
	}
	projected := *value
	projected.SourceIndexSHA256 = ""
	projected.ExternalCallIndexSHA256 = ""
	projected.DependencyCatalogSHA256 = ""
	projected.CoreObjectIndexSHA256 = ""
	projected.CoreObjectProjectionSHA256 = ""
	projected.ActivitySubstrateSHA256 = ""
	if value.SurfaceCoreEffects != nil {
		effects := *value.SurfaceCoreEffects
		effects.AuthoritySHA256 = ""
		projected.SurfaceCoreEffects = &effects
	}
	return &projected
}

func coreMapViewForBrowser(value *CoreMapView) *CoreMapView {
	if value == nil {
		return nil
	}
	projected := *value
	projected.IntegrationUsageSHA256 = ""
	return &projected
}

func integrationUsageViewForBrowser(value *IntegrationUsageView) *IntegrationUsageView {
	if value == nil {
		return nil
	}
	projected := *value
	projected.DependencyCatalogSHA256 = ""
	projected.IntegrationDependenciesSHA256 = ""
	projected.IntegrationUsageSHA256 = ""
	return &projected
}

func activityPathViewForBrowser(value *ActivityPathView) *ActivityPathView {
	if value == nil {
		return nil
	}
	projected := *value
	// The browser joins this projection through exact local object and
	// operation IDs. Artifact digests stay in report.json and the manifest,
	// where the independently read producer bytes can actually verify them.
	projected.ActivityEntrypointsSHA256 = ""
	projected.IntegrationDependenciesSHA256 = ""
	projected.IntegrationUsageSHA256 = ""
	projected.ActivityPathsSHA256 = ""
	return &projected
}
