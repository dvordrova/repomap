package report

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/programindex"
)

const (
	BrowserRepositoryPayloadVersion = 1
	BrowserTargetPayloadVersion     = 1
)

// BrowserRepositoryPayload is the repository-scoped browser contract. It is
// deliberately independent of ReportData and contains no target-local graph.
type BrowserRepositoryPayload struct {
	Version                        int                      `json:"version"`
	Repository                     BrowserRepository        `json:"repository"`
	Source                         BrowserSource            `json:"source"`
	LogicalDefaultSelectedTargetID string                   `json:"logical_default_selected_target_id"`
	Targets                        []BrowserTargetIndexItem `json:"targets"`
	Runtime                        *BrowserRuntimeOverview  `json:"runtime,omitempty"`
	OpenablePaths                  []string                 `json:"openable_paths"`
	Warnings                       []string                 `json:"warnings,omitempty"`
}

type BrowserRepository struct {
	Name             string `json:"name"`
	CapturedRevision string `json:"captured_revision"`
}

type BrowserSource struct {
	Kind          string            `json:"kind"`
	RepositoryURL string            `json:"repository_url,omitempty"`
	PathPrefix    string            `json:"path_prefix,omitempty"`
	SourceIDs     map[string]string `json:"source_ids,omitempty"`
}

type BrowserTargetIndexItem struct {
	SelectedTargetID string `json:"selected_target_id"`
	ProgramTargetID  string `json:"program_target_id,omitempty"`
	Language         string `json:"language"`
	Kind             string `json:"kind"`
	DisplayName      string `json:"display_name"`
	State            string `json:"state"`
	Href             string `json:"href,omitempty"`
	FailureStage     string `json:"failure_stage,omitempty"`
	FailureReason    string `json:"failure_reason,omitempty"`
}

type BrowserRuntimeOverview struct {
	Roles               []BrowserRuntimeRole               `json:"roles"`
	UnclassifiedTargets []BrowserRuntimeUnclassifiedTarget `json:"unclassified_targets"`
}

type BrowserRuntimeRole struct {
	Name            string                         `json:"name"`
	Purpose         string                         `json:"purpose"`
	Prominence      string                         `json:"prominence"`
	RoleKind        string                         `json:"role_kind"`
	Requiredness    string                         `json:"requiredness"`
	Confidence      string                         `json:"confidence"`
	Implementations []BrowserRuntimeImplementation `json:"implementations"`
	// Evidence locations intentionally preserve duplicate rows: the UI shows
	// the validated fact count as well as the unique-location count.
	Evidence []BrowserLocation `json:"evidence"`
}

type BrowserRuntimeImplementation struct {
	ProgramTargetID string `json:"program_target_id"`
	Mode            string `json:"mode,omitempty"`
}

type BrowserRuntimeUnclassifiedTarget struct {
	ProgramTargetID string `json:"program_target_id"`
	Reason          string `json:"reason"`
}

// BrowserTargetPayload is one target-local browser contract. Repository-wide
// runtime and selected-target outcome authority cannot enter this type.
type BrowserTargetPayload struct {
	Version       int                   `json:"version"`
	Target        BrowserTarget         `json:"target"`
	OpenablePaths []string              `json:"openable_paths"`
	Features      BrowserTargetFeatures `json:"features"`
}

type BrowserTarget struct {
	ID       string `json:"id"`
	Language string `json:"language"`
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	Selector string `json:"selector"`
}

type BrowserTargetFeatures struct {
	Program           BrowserProgramFeature           `json:"program"`
	Core              *BrowserCoreFeature             `json:"core,omitempty"`
	Entrypoints       *BrowserEntrypointFeature       `json:"entrypoints,omitempty"`
	Integrations      *BrowserIntegrationFeature      `json:"integrations,omitempty"`
	ActivityPaths     *BrowserActivityPathFeature     `json:"activity_paths,omitempty"`
	Surfaces          *BrowserSurfaceFeature          `json:"surfaces,omitempty"`
	CrossSurfacePaths *BrowserCrossSurfacePathFeature `json:"cross_surface_paths,omitempty"`
}

type BrowserProgramFeature struct {
	Objects    []BrowserProgramObject   `json:"objects"`
	Relations  []BrowserProgramRelation `json:"relations"`
	Projection BrowserProgramProjection `json:"projection"`
}

type BrowserProgramObject struct {
	ID          string                 `json:"id"`
	Kind        string                 `json:"kind"`
	Name        string                 `json:"name"`
	OwnerID     string                 `json:"owner_id,omitempty"`
	ContainerID string                 `json:"container_id,omitempty"`
	Location    *BrowserLocation       `json:"location,omitempty"`
	External    *BrowserExternalSymbol `json:"external,omitempty"`
}

type BrowserExternalSymbol struct {
	PackagePath string `json:"package_path"`
	Receiver    string `json:"receiver,omitempty"`
	Name        string `json:"name"`
}

type BrowserProgramRelation struct {
	ID                         string                  `json:"id"`
	Kind                       string                  `json:"kind"`
	Resolution                 string                  `json:"resolution"`
	FromID                     string                  `json:"from_id"`
	ToIDs                      []string                `json:"to_ids"`
	Invocation                 string                  `json:"invocation,omitempty"`
	Location                   *BrowserLocation        `json:"location,omitempty"`
	Witnesses                  []BrowserProgramWitness `json:"witnesses"`
	WitnessesOmitted           int                     `json:"witnesses_omitted"`
	WitnessesProjectionOmitted int                     `json:"witnesses_projection_omitted"`
}

type BrowserProgramWitness struct {
	SourceExpression string           `json:"source_expression,omitempty"`
	Location         *BrowserLocation `json:"location,omitempty"`
}

type BrowserProgramProjection struct {
	Seeds     BrowserProjectionCollection `json:"seeds"`
	Objects   BrowserProjectionCollection `json:"objects"`
	Relations BrowserProjectionCollection `json:"relations"`
}

type BrowserProjectionCollection struct {
	Eligible int `json:"eligible"`
	Omitted  int `json:"omitted"`
}

type BrowserCoreFeature struct {
	RefinedCore   []BrowserCoreBlock  `json:"refined_core"`
	RefinedGroups []BrowserCoreGroup  `json:"refined_groups"`
	Coverage      BrowserCoreCoverage `json:"coverage"`
}

type BrowserCoreBlock struct {
	ID                    string                            `json:"id"`
	Name                  string                            `json:"name"`
	Purpose               string                            `json:"purpose"`
	Files                 []BrowserCoreFile                 `json:"files"`
	RepresentativeSymbols []BrowserCoreRepresentativeSymbol `json:"representative_symbols"`
	Children              []BrowserCoreBlock                `json:"children"`
}

type BrowserCoreFile struct {
	Path string `json:"path"`
}

type BrowserCoreRepresentativeSymbol struct {
	Symbol             BrowserCoreSymbol `json:"symbol"`
	UnresolvedOutgoing int               `json:"unresolved_outgoing"`
}

type BrowserCoreSymbol struct {
	NodeID   string          `json:"node_id"`
	Name     string          `json:"name"`
	Location BrowserLocation `json:"location"`
}

type BrowserCoreGroup struct {
	ID           string   `json:"id"`
	Authority    string   `json:"authority"`
	Name         string   `json:"name"`
	Purpose      string   `json:"purpose"`
	CoreBlockIDs []string `json:"core_block_ids"`
}

type BrowserCoreCoverage struct {
	ProgramObjectsOmitted int `json:"program_objects_omitted"`
}

type BrowserEntrypointFeature struct {
	Entrypoints []BrowserEntrypoint `json:"entrypoints"`
}

type BrowserEntrypoint struct {
	ObjectID  string          `json:"object_id"`
	Kind      string          `json:"kind"`
	Name      string          `json:"name"`
	Signature string          `json:"signature,omitempty"`
	Location  BrowserLocation `json:"location"`
}

type BrowserIntegrationFeature struct {
	Dependencies []BrowserIntegrationDependency `json:"dependencies"`
}

type BrowserIntegrationDependency struct {
	DependencyID string                  `json:"dependency_id"`
	Kind         string                  `json:"kind"`
	Name         string                  `json:"name"`
	PackagePath  string                  `json:"package_path"`
	Uses         []BrowserIntegrationUse `json:"uses"`
}

type BrowserIntegrationUse struct {
	RelationID       string          `json:"relation_id"`
	WitnessIndex     int             `json:"witness_index"`
	ExternalSymbolID string          `json:"external_symbol_id"`
	CallerID         string          `json:"caller_id"`
	CallerName       string          `json:"caller_name"`
	Callsite         BrowserLocation `json:"callsite"`
	CanonicalCallee  string          `json:"canonical_callee"`
	Label            string          `json:"label"`
	Mechanism        string          `json:"mechanism"`
	Authority        string          `json:"authority"`
}

type BrowserActivityPathFeature struct {
	Objects  []BrowserActivityPathObject  `json:"objects"`
	Routes   []BrowserActivityPathRoute   `json:"routes"`
	Outcomes []BrowserActivityPathOutcome `json:"outcomes"`
}

type BrowserActivityPathObject struct {
	ObjectID string `json:"object_id"`
	Kind     string `json:"kind"`
	Name     string `json:"name"`
}

type BrowserActivityPathRoute struct {
	RouteID    string                    `json:"route_id"`
	CallerID   string                    `json:"caller_id"`
	Status     string                    `json:"status"`
	ActivityID string                    `json:"activity_id,omitempty"`
	Steps      []BrowserActivityPathStep `json:"steps"`
	Distance   int                       `json:"distance"`
	Frontier   []string                  `json:"frontier"`
}

type BrowserActivityPathStep struct {
	FromID    string           `json:"from_id"`
	ToID      string           `json:"to_id"`
	Kind      string           `json:"kind"`
	Authority string           `json:"authority"`
	Location  *BrowserLocation `json:"location,omitempty"`
}

type BrowserActivityPathOutcome struct {
	DependencyID     string `json:"dependency_id"`
	RelationID       string `json:"relation_id"`
	WitnessIndex     int    `json:"witness_index"`
	ExternalSymbolID string `json:"external_symbol_id"`
	RouteID          string `json:"route_id"`
}

type BrowserSurfaceFeature struct {
	Facts    []BrowserJSTSFact `json:"facts"`
	Surfaces []BrowserSurface  `json:"surfaces"`
}

type BrowserJSTSFact struct {
	Ref      string           `json:"ref"`
	Category string           `json:"category"`
	Kind     string           `json:"kind"`
	Label    string           `json:"label"`
	Location *BrowserLocation `json:"location,omitempty"`
}

type BrowserSurface struct {
	SurfaceID    string          `json:"surface_id"`
	Kind         string          `json:"kind"`
	Role         string          `json:"role"`
	Disposition  string          `json:"disposition"`
	Name         string          `json:"name"`
	EntryRefs    []string        `json:"entry_refs"`
	EvidenceRefs []string        `json:"evidence_refs"`
	Location     BrowserLocation `json:"location"`
}

type BrowserCrossSurfacePathFeature struct {
	Facts    []BrowserJSTSFact           `json:"facts"`
	Paths    []BrowserCrossSurfacePath   `json:"paths"`
	Coverage BrowserCrossSurfaceCoverage `json:"coverage"`
}

type BrowserCrossSurfacePath struct {
	PathID   string                    `json:"path_id"`
	Name     string                    `json:"name"`
	Outcome  string                    `json:"outcome"`
	Steps    []BrowserCrossSurfaceStep `json:"steps"`
	Frontier string                    `json:"frontier,omitempty"`
}

type BrowserCrossSurfaceStep struct {
	Ordinal    int             `json:"ordinal"`
	Kind       string          `json:"kind"`
	Label      string          `json:"label"`
	SourceRef  string          `json:"source_ref"`
	TargetRefs []string        `json:"target_refs"`
	Resolution string          `json:"resolution"`
	Authority  string          `json:"authority"`
	Location   BrowserLocation `json:"location"`
}

type BrowserCrossSurfaceCoverage struct {
	RoutesObserved   int `json:"routes_observed"`
	HTTPUsesObserved int `json:"http_uses_observed"`
}

type BrowserLocation struct {
	Path   string `json:"path"`
	Line   int    `json:"line,omitempty"`
	Column int    `json:"column,omitempty"`
}

// ProjectBrowserRepositoryPayload validates canonical presentation and
// transient navigation/source authority before deriving the shared payload.
func ProjectBrowserRepositoryPayload(
	data *ReportData,
	navigation *TargetNavigationPortfolio,
) (BrowserRepositoryPayload, error) {
	if err := validateProgramPresentation(data); err != nil {
		return BrowserRepositoryPayload{}, fmt.Errorf("browser repository payload: canonical report: %w", err)
	}
	if err := validateTargetNavigation(data, navigation); err != nil {
		return BrowserRepositoryPayload{}, fmt.Errorf("browser repository payload: target navigation: %w", err)
	}
	if err := validateBoundBrowserSource(data); err != nil {
		return BrowserRepositoryPayload{}, err
	}

	targets, logicalDefault, err := projectBrowserTargetIndex(data, navigation)
	if err != nil {
		return BrowserRepositoryPayload{}, err
	}
	openable := newBrowserOpenableCollector(data.OpenablePaths)
	payload := BrowserRepositoryPayload{
		Version: BrowserRepositoryPayloadVersion,
		Repository: BrowserRepository{
			Name: data.RepoName, CapturedRevision: data.CapturedRevision,
		},
		Source:                         projectBrowserSource(data),
		LogicalDefaultSelectedTargetID: logicalDefault,
		Targets:                        targets,
		Warnings:                       append([]string(nil), data.Warnings...),
	}
	if data.RuntimePortfolio != nil {
		payload.Runtime = projectBrowserRuntime(data.RuntimePortfolio, openable)
	}
	payload.OpenablePaths, err = openable.finish()
	if err != nil {
		return BrowserRepositoryPayload{}, fmt.Errorf("browser repository payload: %w", err)
	}
	if err := payload.Validate(); err != nil {
		return BrowserRepositoryPayload{}, fmt.Errorf("browser repository payload: invalid projection: %w", err)
	}
	return payload, nil
}

// ProjectBrowserTargetPayload is the sole semantic target projector for both
// ordinary HTML and standalone chunks.
func ProjectBrowserTargetPayload(data *ReportData) (BrowserTargetPayload, error) {
	if err := validateProgramPresentation(data); err != nil {
		return BrowserTargetPayload{}, fmt.Errorf("browser target payload: canonical report: %w", err)
	}
	if err := validateBoundBrowserSource(data); err != nil {
		return BrowserTargetPayload{}, err
	}
	entry, err := data.ProgramPortfolio.defaultEntry()
	if err != nil {
		return BrowserTargetPayload{}, fmt.Errorf("browser target payload: default program: %w", err)
	}
	openable := newBrowserOpenableCollector(data.OpenablePaths)
	payload := BrowserTargetPayload{
		Version: BrowserTargetPayloadVersion,
		Target: BrowserTarget{
			ID: entry.Target.ID, Language: entry.Target.Language, Kind: entry.Target.Kind,
			Name: entry.Target.Name, Selector: entry.Target.Selector,
		},
		Features: BrowserTargetFeatures{
			Program: projectBrowserProgram(entry.View, openable),
		},
	}
	if data.CoreMapView != nil {
		payload.Features.Core = projectBrowserCore(data.CoreMapView, openable)
	}
	if data.ActivityEntrypointView != nil {
		payload.Features.Entrypoints = projectBrowserEntrypoints(data.ActivityEntrypointView, openable)
	}
	if data.IntegrationUsageView != nil {
		payload.Features.Integrations = projectBrowserIntegrations(data.IntegrationUsageView, openable)
	}
	if data.ActivityPathView != nil {
		payload.Features.ActivityPaths = projectBrowserActivityPaths(data.ActivityPathView, openable)
	}
	if data.JSTSSurfaceCatalogView != nil {
		payload.Features.Surfaces = projectBrowserSurfaces(data.JSTSSurfaceCatalogView, openable)
	}
	if data.CrossSurfacePathView != nil {
		payload.Features.CrossSurfacePaths = projectBrowserCrossSurfacePaths(data.CrossSurfacePathView, openable)
	}
	payload.OpenablePaths, err = openable.finish()
	if err != nil {
		return BrowserTargetPayload{}, fmt.Errorf("browser target payload: %w", err)
	}
	if err := payload.Validate(); err != nil {
		return BrowserTargetPayload{}, fmt.Errorf("browser target payload: invalid projection: %w", err)
	}
	return payload, nil
}

func EncodeBrowserRepositoryPayload(payload BrowserRepositoryPayload) ([]byte, error) {
	if err := payload.Validate(); err != nil {
		return nil, fmt.Errorf("browser repository payload: %w", err)
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("browser repository payload: encode: %w", err)
	}
	return encoded, nil
}

func DecodeBrowserRepositoryPayload(raw []byte) (BrowserRepositoryPayload, error) {
	var payload BrowserRepositoryPayload
	if err := decodeStrictBrowserPayload(raw, &payload, "repository"); err != nil {
		return BrowserRepositoryPayload{}, err
	}
	if err := payload.Validate(); err != nil {
		return BrowserRepositoryPayload{}, fmt.Errorf("browser repository payload: %w", err)
	}
	return payload, nil
}

func EncodeBrowserTargetPayload(payload BrowserTargetPayload) ([]byte, error) {
	if err := payload.Validate(); err != nil {
		return nil, fmt.Errorf("browser target payload: %w", err)
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("browser target payload: encode: %w", err)
	}
	return encoded, nil
}

func DecodeBrowserTargetPayload(raw []byte) (BrowserTargetPayload, error) {
	var payload BrowserTargetPayload
	if err := decodeStrictBrowserPayload(raw, &payload, "target"); err != nil {
		return BrowserTargetPayload{}, err
	}
	if err := payload.Validate(); err != nil {
		return BrowserTargetPayload{}, fmt.Errorf("browser target payload: %w", err)
	}
	return payload, nil
}

func decodeStrictBrowserPayload(raw []byte, destination any, label string) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("browser %s payload: decode: %w", label, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("browser %s payload: multiple JSON values", label)
		}
		return fmt.Errorf("browser %s payload: trailing data: %w", label, err)
	}
	return nil
}

func projectBrowserTargetIndex(
	data *ReportData,
	navigation *TargetNavigationPortfolio,
) ([]BrowserTargetIndexItem, string, error) {
	links := make(map[string]TargetNavigationItem)
	if navigation != nil {
		for _, item := range navigation.Targets {
			links[item.TargetID] = item
		}
	}
	if data.TargetOutcomePortfolio != nil {
		result := make([]BrowserTargetIndexItem, 0, len(data.TargetOutcomePortfolio.Outcomes))
		for _, outcome := range data.TargetOutcomePortfolio.Outcomes {
			row := BrowserTargetIndexItem{
				SelectedTargetID: outcome.SelectedTargetID,
				Language:         string(outcome.Language),
				Kind:             string(outcome.ScopeKind),
				DisplayName:      outcome.DisplayName,
				State:            string(outcome.State),
				ProgramTargetID:  outcome.ProgramTargetID,
				FailureStage:     string(outcome.FailureStage),
				FailureReason:    string(outcome.FailureReason),
			}
			if row.ProgramTargetID != "" {
				navigationItem, ok := links[row.ProgramTargetID]
				if !ok {
					return nil, "", fmt.Errorf("browser repository payload: analyzed target lacks navigation")
				}
				row.Href = navigationItem.Href
			}
			result = append(result, row)
		}
		return result, data.TargetOutcomePortfolio.DefaultSelectedTargetID, nil
	}
	if navigation != nil {
		result := make([]BrowserTargetIndexItem, 0, len(navigation.Targets))
		for _, item := range navigation.Targets {
			result = append(result, BrowserTargetIndexItem{
				SelectedTargetID: item.TargetID,
				ProgramTargetID:  item.TargetID,
				Language:         item.Language,
				Kind:             item.Kind,
				DisplayName:      item.DisplayName,
				State:            "analyzed",
				Href:             item.Href,
			})
		}
		return result, navigation.DefaultTargetID, nil
	}
	entry, err := data.ProgramPortfolio.defaultEntry()
	if err != nil {
		return nil, "", fmt.Errorf("browser repository payload: default target: %w", err)
	}
	return []BrowserTargetIndexItem{{
		SelectedTargetID: entry.Target.ID,
		ProgramTargetID:  entry.Target.ID,
		Language:         entry.Target.Language,
		Kind:             entry.Target.Kind,
		DisplayName:      entry.Target.Name,
		State:            "analyzed",
		Href:             "#/program",
	}}, entry.Target.ID, nil
}

func validateBoundBrowserSource(data *ReportData) error {
	if err := data.GitHubSourceLinks.validate(); err != nil {
		return fmt.Errorf("browser payload: GitHub source authority: %w", err)
	}
	if err := data.GitLabSourceLinks.validate(); err != nil {
		return fmt.Errorf("browser payload: GitLab source authority: %w", err)
	}
	if data.GitHubSourceLinks != nil && data.GitLabSourceLinks != nil {
		return fmt.Errorf("browser payload: multiple static source authorities")
	}
	if data.GitHubSourceLinks != nil && data.GitHubSourceLinks.Revision != data.CapturedRevision ||
		data.GitLabSourceLinks != nil && data.GitLabSourceLinks.Revision != data.CapturedRevision {
		return fmt.Errorf("browser payload: static source revision mismatch")
	}
	if err := validateBrowserSourceIDs(data); err != nil {
		return fmt.Errorf("browser payload: %w", err)
	}
	return nil
}

func projectBrowserSource(data *ReportData) BrowserSource {
	switch {
	case data.GitHubSourceLinks != nil:
		return BrowserSource{
			Kind: "github", RepositoryURL: data.GitHubSourceLinks.RepositoryURL,
			PathPrefix: data.GitHubSourceLinks.PathPrefix,
		}
	case data.GitLabSourceLinks != nil:
		return BrowserSource{
			Kind: "gitlab", RepositoryURL: data.GitLabSourceLinks.RepositoryURL,
			PathPrefix: data.GitLabSourceLinks.PathPrefix,
		}
	case len(data.SourceIDs) != 0:
		ids := make(map[string]string, len(data.SourceIDs))
		for sourcePath, sourceID := range data.SourceIDs {
			ids[sourcePath] = sourceID
		}
		return BrowserSource{Kind: "served", SourceIDs: ids}
	default:
		return BrowserSource{Kind: "none"}
	}
}

func projectBrowserRuntime(
	value *RuntimePortfolioView,
	openable *browserOpenableCollector,
) *BrowserRuntimeOverview {
	result := &BrowserRuntimeOverview{
		Roles: make([]BrowserRuntimeRole, 0, len(value.Roles)),
		UnclassifiedTargets: make(
			[]BrowserRuntimeUnclassifiedTarget, 0, len(value.UnclassifiedTargets),
		),
	}
	for _, role := range value.Roles {
		projected := BrowserRuntimeRole{
			Name: role.Name, Purpose: role.Purpose,
			Prominence: string(role.Prominence), RoleKind: string(role.RoleKind),
			Requiredness: string(role.Requiredness), Confidence: string(role.Confidence),
			Implementations: make([]BrowserRuntimeImplementation, 0, len(role.Implementations)),
			Evidence:        make([]BrowserLocation, 0, len(role.Evidence)),
		}
		for _, implementation := range role.Implementations {
			projected.Implementations = append(projected.Implementations, BrowserRuntimeImplementation{
				ProgramTargetID: implementation.ProgramTargetID,
				Mode:            implementation.Mode,
			})
		}
		for _, evidence := range role.Evidence {
			location := BrowserLocation{
				Path: evidence.Location.Path, Line: evidence.Location.Line,
				Column: evidence.Location.Column,
			}
			openable.add(location.Path)
			projected.Evidence = append(projected.Evidence, location)
		}
		result.Roles = append(result.Roles, projected)
	}
	for _, target := range value.UnclassifiedTargets {
		result.UnclassifiedTargets = append(
			result.UnclassifiedTargets,
			BrowserRuntimeUnclassifiedTarget{
				ProgramTargetID: target.ProgramTargetID, Reason: target.Reason,
			},
		)
	}
	return result
}

func projectBrowserProgram(
	view ProgramView,
	openable *browserOpenableCollector,
) BrowserProgramFeature {
	result := BrowserProgramFeature{
		Objects:   make([]BrowserProgramObject, 0, len(view.Objects)),
		Relations: make([]BrowserProgramRelation, 0, len(view.Relations)),
		Projection: BrowserProgramProjection{
			Seeds: BrowserProjectionCollection{
				Eligible: view.Projection.Seeds.Eligible, Omitted: view.Projection.Seeds.Omitted,
			},
			Objects: BrowserProjectionCollection{
				Eligible: view.Projection.Objects.Eligible, Omitted: view.Projection.Objects.Omitted,
			},
			Relations: BrowserProjectionCollection{
				Eligible: view.Projection.Relations.Eligible, Omitted: view.Projection.Relations.Omitted,
			},
		},
	}
	for _, object := range view.Objects {
		projected := BrowserProgramObject{
			ID: object.ID, Kind: string(object.Kind), Name: object.Name,
			OwnerID: object.OwnerID, ContainerID: object.ContainerID,
			Location: browserProgramLocation(object.Location, openable),
		}
		if object.External != nil {
			projected.External = &BrowserExternalSymbol{
				PackagePath: object.External.PackagePath,
				Receiver:    object.External.Receiver,
				Name:        object.External.Name,
			}
		}
		result.Objects = append(result.Objects, projected)
	}
	for _, relation := range view.Relations {
		projected := BrowserProgramRelation{
			ID: relation.ID, Kind: string(relation.Kind), Resolution: string(relation.Resolution),
			FromID: relation.FromID, ToIDs: append([]string{}, relation.ToIDs...),
			Invocation:                 relation.Invocation,
			Location:                   browserProgramLocation(relation.Location, openable),
			Witnesses:                  make([]BrowserProgramWitness, 0, len(relation.Witnesses)),
			WitnessesOmitted:           relation.WitnessesOmitted,
			WitnessesProjectionOmitted: relation.WitnessesProjectionOmitted,
		}
		for _, witness := range relation.Witnesses {
			projected.Witnesses = append(projected.Witnesses, BrowserProgramWitness{
				SourceExpression: witness.SourceExpression,
				Location:         browserProgramLocation(witness.Location, openable),
			})
		}
		result.Relations = append(result.Relations, projected)
	}
	return result
}

func projectBrowserCore(
	value *CoreMapView,
	openable *browserOpenableCollector,
) *BrowserCoreFeature {
	result := &BrowserCoreFeature{
		RefinedCore:   projectBrowserCoreBlocks(value.RefinedCore, openable),
		RefinedGroups: make([]BrowserCoreGroup, 0, len(value.RefinedGroups)),
		Coverage: BrowserCoreCoverage{
			ProgramObjectsOmitted: value.Coverage.ProgramObjectsOmitted,
		},
	}
	for _, group := range value.RefinedGroups {
		result.RefinedGroups = append(result.RefinedGroups, BrowserCoreGroup{
			ID: group.ID, Authority: string(group.Authority), Name: group.Name,
			Purpose: group.Purpose, CoreBlockIDs: append([]string{}, group.CoreBlockIDs...),
		})
	}
	return result
}

func projectBrowserCoreBlocks(
	values []CoreMapViewBlock,
	openable *browserOpenableCollector,
) []BrowserCoreBlock {
	result := make([]BrowserCoreBlock, 0, len(values))
	for _, block := range values {
		projected := BrowserCoreBlock{
			ID: block.ID, Name: block.Name, Purpose: block.Purpose,
			Files:                 make([]BrowserCoreFile, 0, len(block.Files)),
			RepresentativeSymbols: make([]BrowserCoreRepresentativeSymbol, 0, len(block.RepresentativeSymbols)),
			Children:              projectBrowserCoreBlocks(block.Children, openable),
		}
		for _, file := range block.Files {
			openable.add(file.Path)
			projected.Files = append(projected.Files, BrowserCoreFile{Path: file.Path})
		}
		for _, representative := range block.RepresentativeSymbols {
			location := BrowserLocation{
				Path:   representative.Symbol.Location.Path,
				Line:   representative.Symbol.Location.Line,
				Column: representative.Symbol.Location.Column,
			}
			openable.add(location.Path)
			projected.RepresentativeSymbols = append(
				projected.RepresentativeSymbols,
				BrowserCoreRepresentativeSymbol{
					Symbol: BrowserCoreSymbol{
						NodeID: representative.Symbol.NodeID,
						Name:   representative.Symbol.Name, Location: location,
					},
					UnresolvedOutgoing: representative.UnresolvedOutgoing,
				},
			)
		}
		result = append(result, projected)
	}
	return result
}

func projectBrowserEntrypoints(
	value *ActivityEntrypointView,
	openable *browserOpenableCollector,
) *BrowserEntrypointFeature {
	result := &BrowserEntrypointFeature{
		Entrypoints: make([]BrowserEntrypoint, 0, len(value.Entrypoints)),
	}
	for _, entrypoint := range value.Entrypoints {
		location := BrowserLocation{
			Path: entrypoint.Location.Path, Line: entrypoint.Location.Line,
			Column: entrypoint.Location.Column,
		}
		openable.add(location.Path)
		result.Entrypoints = append(result.Entrypoints, BrowserEntrypoint{
			ObjectID: entrypoint.ObjectID, Kind: string(entrypoint.Kind), Name: entrypoint.Name,
			Signature: entrypoint.Signature, Location: location,
		})
	}
	return result
}

func projectBrowserIntegrations(
	value *IntegrationUsageView,
	openable *browserOpenableCollector,
) *BrowserIntegrationFeature {
	result := &BrowserIntegrationFeature{
		Dependencies: make([]BrowserIntegrationDependency, 0, len(value.Dependencies)),
	}
	for _, dependency := range value.Dependencies {
		projected := BrowserIntegrationDependency{
			DependencyID: dependency.DependencyID, Kind: string(dependency.Kind),
			Name: dependency.Name, PackagePath: dependency.PackagePath,
			Uses: make([]BrowserIntegrationUse, 0, len(dependency.Uses)),
		}
		for _, use := range dependency.Uses {
			callsite := BrowserLocation{
				Path: use.Callsite.Path, Line: use.Callsite.Line, Column: use.Callsite.Column,
			}
			openable.add(callsite.Path)
			projected.Uses = append(projected.Uses, BrowserIntegrationUse{
				RelationID: use.RelationID, WitnessIndex: use.WitnessIndex,
				ExternalSymbolID: use.ExternalSymbolID,
				CallerID:         use.CallerID, CallerName: use.CallerName, Callsite: callsite,
				CanonicalCallee: use.CanonicalCallee, Label: use.Label,
				Mechanism: use.Mechanism, Authority: use.Authority,
			})
		}
		result.Dependencies = append(result.Dependencies, projected)
	}
	return result
}

func projectBrowserActivityPaths(
	value *ActivityPathView,
	openable *browserOpenableCollector,
) *BrowserActivityPathFeature {
	result := &BrowserActivityPathFeature{
		Objects:  make([]BrowserActivityPathObject, 0, len(value.Objects)),
		Routes:   make([]BrowserActivityPathRoute, 0, len(value.Routes)),
		Outcomes: make([]BrowserActivityPathOutcome, 0, len(value.Outcomes)),
	}
	for _, object := range value.Objects {
		result.Objects = append(result.Objects, BrowserActivityPathObject{
			ObjectID: object.ObjectID, Kind: string(object.Kind), Name: object.Name,
		})
	}
	for _, route := range value.Routes {
		projected := BrowserActivityPathRoute{
			RouteID: route.RouteID, CallerID: route.CallerID, Status: string(route.Status),
			ActivityID: route.ActivityID, Steps: make([]BrowserActivityPathStep, 0, len(route.Steps)),
			Distance: route.Distance, Frontier: make([]string, 0, len(route.Frontier)),
		}
		for _, frontier := range route.Frontier {
			projected.Frontier = append(projected.Frontier, string(frontier))
		}
		for _, step := range route.Steps {
			projected.Steps = append(projected.Steps, BrowserActivityPathStep{
				FromID: step.FromID, ToID: step.ToID, Kind: string(step.Kind),
				Authority: string(step.Authority),
				Location:  browserProgramLocation(step.Location, openable),
			})
		}
		result.Routes = append(result.Routes, projected)
	}
	for _, outcome := range value.Outcomes {
		result.Outcomes = append(result.Outcomes, BrowserActivityPathOutcome{
			DependencyID: outcome.DependencyID, RelationID: outcome.RelationID,
			WitnessIndex: outcome.WitnessIndex, ExternalSymbolID: outcome.ExternalSymbolID,
			RouteID: outcome.RouteID,
		})
	}
	return result
}

func projectBrowserSurfaces(
	value *JSTSSurfaceCatalogView,
	openable *browserOpenableCollector,
) *BrowserSurfaceFeature {
	result := &BrowserSurfaceFeature{
		Facts:    make([]BrowserJSTSFact, 0, len(value.Facts)),
		Surfaces: make([]BrowserSurface, 0, len(value.Surfaces)),
	}
	for _, fact := range value.Facts {
		result.Facts = append(result.Facts, projectBrowserJSTSFact(fact, openable))
	}
	for _, surface := range value.Surfaces {
		location := BrowserLocation{
			Path: surface.Location.Path, Line: surface.Location.Line, Column: surface.Location.Column,
		}
		openable.add(location.Path)
		result.Surfaces = append(result.Surfaces, BrowserSurface{
			SurfaceID: surface.SurfaceID, Kind: string(surface.Kind), Role: string(surface.Role),
			Disposition: string(surface.Disposition), Name: surface.Name,
			EntryRefs:    append([]string{}, surface.EntryRefs...),
			EvidenceRefs: append([]string{}, surface.EvidenceRefs...), Location: location,
		})
	}
	return result
}

func projectBrowserCrossSurfacePaths(
	value *CrossSurfacePathView,
	openable *browserOpenableCollector,
) *BrowserCrossSurfacePathFeature {
	result := &BrowserCrossSurfacePathFeature{
		Facts: make([]BrowserJSTSFact, 0, len(value.Facts)),
		Paths: make([]BrowserCrossSurfacePath, 0, len(value.Paths)),
		Coverage: BrowserCrossSurfaceCoverage{
			RoutesObserved:   value.Coverage.RoutesObserved,
			HTTPUsesObserved: value.Coverage.HTTPUsesObserved,
		},
	}
	for _, fact := range value.Facts {
		result.Facts = append(result.Facts, projectBrowserJSTSFact(fact, openable))
	}
	for _, path := range value.Paths {
		projected := BrowserCrossSurfacePath{
			PathID: path.PathID, Name: path.Name, Outcome: path.Outcome, Frontier: path.Frontier,
			Steps: make([]BrowserCrossSurfaceStep, 0, len(path.Steps)),
		}
		for _, step := range path.Steps {
			location := BrowserLocation{
				Path: step.Location.Path, Line: step.Location.Line, Column: step.Location.Column,
			}
			openable.add(location.Path)
			projected.Steps = append(projected.Steps, BrowserCrossSurfaceStep{
				Ordinal: step.Ordinal, Kind: string(step.Kind), Label: step.Label,
				SourceRef: step.SourceRef, TargetRefs: append([]string{}, step.TargetRefs...),
				Resolution: string(step.Resolution), Authority: string(step.Authority), Location: location,
			})
		}
		result.Paths = append(result.Paths, projected)
	}
	return result
}

func projectBrowserJSTSFact(
	fact JSTSFactView,
	openable *browserOpenableCollector,
) BrowserJSTSFact {
	result := BrowserJSTSFact{
		Ref: fact.Ref, Category: fact.Category, Kind: fact.Kind, Label: fact.Label,
	}
	result.Location = browserProgramLocation(fact.Location, openable)
	return result
}

func browserProgramLocation(
	value *programindex.Location,
	openable *browserOpenableCollector,
) *BrowserLocation {
	if value == nil {
		return nil
	}
	result := &BrowserLocation{Path: value.Path, Line: value.Line, Column: value.Column}
	openable.add(result.Path)
	return result
}

type browserOpenableCollector struct {
	authorized map[string]struct{}
	referenced map[string]struct{}
	err        error
}

func newBrowserOpenableCollector(authorized []string) *browserOpenableCollector {
	result := &browserOpenableCollector{
		authorized: make(map[string]struct{}, len(authorized)),
		referenced: make(map[string]struct{}),
	}
	for _, sourcePath := range authorized {
		result.authorized[sourcePath] = struct{}{}
	}
	return result
}

func (collector *browserOpenableCollector) add(sourcePath string) {
	if collector == nil || collector.err != nil {
		return
	}
	if _, ok := collector.authorized[sourcePath]; !ok {
		collector.err = fmt.Errorf("projected source path %q is outside canonical openability", sourcePath)
		return
	}
	collector.referenced[sourcePath] = struct{}{}
}

func (collector *browserOpenableCollector) finish() ([]string, error) {
	if collector == nil {
		return nil, fmt.Errorf("openability collector is missing")
	}
	if collector.err != nil {
		return nil, collector.err
	}
	result := make([]string, 0, len(collector.referenced))
	for sourcePath := range collector.referenced {
		result = append(result, sourcePath)
	}
	sort.Strings(result)
	return result, nil
}

func (payload BrowserRepositoryPayload) Validate() error {
	if payload.Version != BrowserRepositoryPayloadVersion {
		return fmt.Errorf("unsupported version %d", payload.Version)
	}
	if !validTargetNavigationText(payload.Repository.Name) ||
		!validGitRevision(payload.Repository.CapturedRevision) ||
		payload.Repository.CapturedRevision != strings.ToLower(payload.Repository.CapturedRevision) {
		return fmt.Errorf("invalid repository identity")
	}
	if err := payload.Source.validate(payload.Repository.CapturedRevision); err != nil {
		return err
	}
	openable, err := validateBrowserOpenablePaths(payload.OpenablePaths)
	if err != nil {
		return err
	}
	for index, warning := range payload.Warnings {
		if !validTargetNavigationText(warning) {
			return fmt.Errorf("warning %d is invalid", index)
		}
	}
	if payload.Targets == nil || len(payload.Targets) == 0 ||
		!validTargetNavigationText(payload.LogicalDefaultSelectedTargetID) {
		return fmt.Errorf("target index is incomplete")
	}
	selectedIDs := make(map[string]struct{}, len(payload.Targets))
	analyzedIDs := make(map[string]struct{}, len(payload.Targets))
	defaultFound := false
	for index, target := range payload.Targets {
		if !validTargetNavigationText(target.SelectedTargetID) ||
			!validTargetNavigationText(target.Language) ||
			!validTargetNavigationText(target.Kind) ||
			!validTargetNavigationText(target.DisplayName) {
			return fmt.Errorf("target %d has invalid public identity", index)
		}
		if _, duplicate := selectedIDs[target.SelectedTargetID]; duplicate {
			return fmt.Errorf("duplicate selected target identity")
		}
		selectedIDs[target.SelectedTargetID] = struct{}{}
		defaultFound = defaultFound || target.SelectedTargetID == payload.LogicalDefaultSelectedTargetID
		switch target.State {
		case "analyzed":
			if !validTargetNavigationText(target.ProgramTargetID) ||
				!validBrowserTargetHref(target.Href) || target.FailureStage != "" ||
				target.FailureReason != "" {
				return fmt.Errorf("analyzed target %d has an invalid binding", index)
			}
			if _, duplicate := analyzedIDs[target.ProgramTargetID]; duplicate {
				return fmt.Errorf("duplicate analyzed ProgramTarget identity")
			}
			analyzedIDs[target.ProgramTargetID] = struct{}{}
		case "not_analyzed":
			if target.ProgramTargetID != "" || target.Href != "" ||
				!browserClosedText(target.FailureStage,
					"target_preparation", "program_analysis", "dependency_analysis",
					"semantic_analysis", "target_page") ||
				!browserClosedText(target.FailureReason,
					"source_not_analyzable", "required_tool_unavailable", "resource_limit",
					"model_result_rejected", "analysis_failed", "target_output_invalid") {
				return fmt.Errorf("not-analyzed target %d has an invalid failure", index)
			}
		default:
			return fmt.Errorf("target %d has unsupported state", index)
		}
	}
	if !defaultFound {
		return fmt.Errorf("logical default selected target is absent")
	}
	if payload.Runtime != nil {
		if err := payload.Runtime.validate(analyzedIDs, openable); err != nil {
			return err
		}
	}
	runtimePaths := make(map[string]struct{})
	if payload.Runtime != nil {
		for _, role := range payload.Runtime.Roles {
			for _, location := range role.Evidence {
				runtimePaths[location.Path] = struct{}{}
			}
		}
	}
	if !sameBrowserPathSet(runtimePaths, openable) {
		return fmt.Errorf("repository openable paths do not exactly cover runtime evidence")
	}
	return nil
}

func (source BrowserSource) validate(revision string) error {
	switch source.Kind {
	case "none":
		if source.RepositoryURL != "" || source.PathPrefix != "" || len(source.SourceIDs) != 0 {
			return fmt.Errorf("empty source authority carries fields")
		}
	case "github", "gitlab":
		if source.RepositoryURL == "" || len(source.SourceIDs) != 0 {
			return fmt.Errorf("static source authority is invalid")
		}
		var err error
		if source.Kind == "github" {
			err = (&GitHubSourceLinks{
				RepositoryURL: source.RepositoryURL, Revision: revision, PathPrefix: source.PathPrefix,
			}).validate()
		} else {
			err = (&GitLabSourceLinks{
				RepositoryURL: source.RepositoryURL, Revision: revision, PathPrefix: source.PathPrefix,
			}).validate()
		}
		if err != nil {
			return fmt.Errorf("static source authority: %w", err)
		}
	case "served":
		if source.RepositoryURL != "" || source.PathPrefix != "" || len(source.SourceIDs) == 0 {
			return fmt.Errorf("served source authority is invalid")
		}
		seen := make(map[string]struct{}, len(source.SourceIDs))
		for sourcePath, sourceID := range source.SourceIDs {
			if validateManifestPath(sourcePath) != nil || !validBrowserSourceID(sourceID) {
				return fmt.Errorf("served source authority contains an invalid entry")
			}
			if _, duplicate := seen[sourceID]; duplicate {
				return fmt.Errorf("served source authority contains a duplicate source ID")
			}
			seen[sourceID] = struct{}{}
		}
	default:
		return fmt.Errorf("unsupported source authority kind %q", source.Kind)
	}
	return nil
}

func (runtime *BrowserRuntimeOverview) validate(
	analyzed map[string]struct{},
	openable map[string]struct{},
) error {
	if runtime.Roles == nil || runtime.UnclassifiedTargets == nil {
		return fmt.Errorf("runtime overview is incomplete")
	}
	mapped := make(map[string]struct{})
	for index, role := range runtime.Roles {
		if !validTargetNavigationText(role.Name) || !validTargetNavigationText(role.Purpose) ||
			!browserClosedText(role.Prominence, "primary", "supporting", "unknown") ||
			!browserClosedText(role.RoleKind,
				"library", "service", "daemon", "worker", "cli", "example", "supporting_tool", "unknown") ||
			!browserClosedText(role.Requiredness, "required", "optional", "experimental", "unknown") ||
			!browserClosedText(role.Confidence, "high", "medium", "low", "unknown") ||
			role.Implementations == nil || role.Evidence == nil {
			return fmt.Errorf("runtime role %d is invalid", index)
		}
		implementations := make(map[string]struct{}, len(role.Implementations))
		for _, implementation := range role.Implementations {
			if _, ok := analyzed[implementation.ProgramTargetID]; !ok {
				return fmt.Errorf("runtime role cites an unknown analyzed target")
			}
			if implementation.Mode != "" && !validTargetNavigationText(implementation.Mode) {
				return fmt.Errorf("runtime implementation mode is invalid")
			}
			key := implementation.ProgramTargetID + "\x00" + implementation.Mode
			if _, duplicate := implementations[key]; duplicate {
				return fmt.Errorf("runtime role repeats an implementation")
			}
			implementations[key] = struct{}{}
			mapped[implementation.ProgramTargetID] = struct{}{}
		}
		for _, location := range role.Evidence {
			if err := validateBrowserLocation(location, openable); err != nil {
				return fmt.Errorf("runtime evidence: %w", err)
			}
		}
	}
	unclassified := make(map[string]struct{}, len(runtime.UnclassifiedTargets))
	for _, target := range runtime.UnclassifiedTargets {
		if _, ok := analyzed[target.ProgramTargetID]; !ok || !validTargetNavigationText(target.Reason) {
			return fmt.Errorf("runtime unclassified target is invalid")
		}
		if _, ok := mapped[target.ProgramTargetID]; ok {
			return fmt.Errorf("mapped runtime target is also unclassified")
		}
		if _, duplicate := unclassified[target.ProgramTargetID]; duplicate {
			return fmt.Errorf("duplicate unclassified runtime target")
		}
		unclassified[target.ProgramTargetID] = struct{}{}
	}
	return nil
}

func (payload BrowserTargetPayload) Validate() error {
	if payload.Version != BrowserTargetPayloadVersion {
		return fmt.Errorf("unsupported version %d", payload.Version)
	}
	if !validTargetNavigationText(payload.Target.ID) ||
		!validTargetNavigationText(payload.Target.Language) ||
		!validTargetNavigationText(payload.Target.Kind) ||
		!validTargetNavigationText(payload.Target.Name) ||
		!validTargetNavigationText(payload.Target.Selector) {
		return fmt.Errorf("invalid target identity")
	}
	openable, err := validateBrowserOpenablePaths(payload.OpenablePaths)
	if err != nil {
		return err
	}
	if err := payload.Features.Program.validate(openable); err != nil {
		return err
	}
	semanticCount := 0
	for _, present := range []bool{
		payload.Features.Core != nil,
		payload.Features.Entrypoints != nil,
		payload.Features.Integrations != nil,
		payload.Features.ActivityPaths != nil,
	} {
		if present {
			semanticCount++
		}
	}
	if semanticCount != 0 && semanticCount != 4 {
		return fmt.Errorf("semantic target features are incomplete")
	}
	if payload.Features.Core != nil {
		if err := payload.Features.Core.validate(payload.Features.Program, openable); err != nil {
			return err
		}
		if err := payload.Features.Entrypoints.validate(openable); err != nil {
			return err
		}
		uses, err := payload.Features.Integrations.validate(openable)
		if err != nil {
			return err
		}
		if err := payload.Features.ActivityPaths.validate(
			payload.Features.Entrypoints, uses, openable,
		); err != nil {
			return err
		}
	}
	isJSTS := payload.Target.Language == "javascript" || payload.Target.Language == "typescript"
	if (payload.Features.Surfaces != nil) != (payload.Features.CrossSurfacePaths != nil) ||
		isJSTS != (payload.Features.Surfaces != nil) {
		return fmt.Errorf("JavaScript/TypeScript surface features are incomplete")
	}
	if payload.Features.Surfaces != nil {
		surfaces, err := payload.Features.Surfaces.validate(openable)
		if err != nil {
			return err
		}
		if err := payload.Features.CrossSurfacePaths.validate(surfaces, openable); err != nil {
			return err
		}
	}
	actualPaths := make(map[string]struct{})
	payload.Features.collectLocations(actualPaths)
	if len(actualPaths) != len(openable) {
		return fmt.Errorf("openable paths do not exactly cover projected source actions")
	}
	for sourcePath := range actualPaths {
		if _, ok := openable[sourcePath]; !ok {
			return fmt.Errorf("projected source path is not openable")
		}
	}
	return nil
}

func (feature BrowserProgramFeature) validate(openable map[string]struct{}) error {
	if feature.Objects == nil || feature.Relations == nil ||
		feature.Projection.Seeds.Eligible < feature.Projection.Seeds.Omitted ||
		feature.Projection.Objects.Eligible < feature.Projection.Objects.Omitted ||
		feature.Projection.Relations.Eligible < feature.Projection.Relations.Omitted ||
		feature.Projection.Seeds.Omitted < 0 || feature.Projection.Objects.Omitted < 0 ||
		feature.Projection.Relations.Omitted < 0 {
		return fmt.Errorf("program feature is invalid")
	}
	objects := make(map[string]struct{}, len(feature.Objects))
	for index, object := range feature.Objects {
		if !validTargetNavigationText(object.ID) || !validTargetNavigationText(object.Kind) ||
			!validTargetNavigationText(object.Name) {
			return fmt.Errorf("program object %d is invalid", index)
		}
		if _, duplicate := objects[object.ID]; duplicate {
			return fmt.Errorf("duplicate program object")
		}
		objects[object.ID] = struct{}{}
		if object.Location != nil {
			if err := validateBrowserLocation(*object.Location, openable); err != nil {
				return err
			}
		}
		if object.External != nil &&
			(!validTargetNavigationText(object.External.PackagePath) ||
				!validTargetNavigationText(object.External.Name) ||
				object.External.Receiver != "" && !validTargetNavigationText(object.External.Receiver)) {
			return fmt.Errorf("program external symbol is invalid")
		}
	}
	for _, object := range feature.Objects {
		for _, linked := range []string{object.OwnerID, object.ContainerID} {
			if linked != "" {
				if _, ok := objects[linked]; !ok {
					return fmt.Errorf("program object context is absent")
				}
			}
		}
	}
	relations := make(map[string]struct{}, len(feature.Relations))
	for index, relation := range feature.Relations {
		if !validTargetNavigationText(relation.ID) || !validTargetNavigationText(relation.Kind) ||
			!browserClosedText(relation.Resolution, "exact", "alternatives", "unresolved") ||
			relation.ToIDs == nil || relation.Witnesses == nil || relation.WitnessesOmitted < 0 ||
			relation.WitnessesProjectionOmitted < 0 {
			return fmt.Errorf("program relation %d is invalid", index)
		}
		if _, duplicate := relations[relation.ID]; duplicate {
			return fmt.Errorf("duplicate program relation")
		}
		relations[relation.ID] = struct{}{}
		if _, ok := objects[relation.FromID]; !ok {
			return fmt.Errorf("program relation source is absent")
		}
		for _, targetID := range relation.ToIDs {
			if _, ok := objects[targetID]; !ok {
				return fmt.Errorf("program relation target is absent")
			}
		}
		if relation.Location != nil {
			if err := validateBrowserLocation(*relation.Location, openable); err != nil {
				return err
			}
		}
		for _, witness := range relation.Witnesses {
			if witness.SourceExpression != "" && !validTargetNavigationText(witness.SourceExpression) {
				return fmt.Errorf("program witness expression is invalid")
			}
			if witness.Location != nil {
				if err := validateBrowserLocation(*witness.Location, openable); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (feature *BrowserCoreFeature) validate(
	program BrowserProgramFeature,
	openable map[string]struct{},
) error {
	if feature.RefinedCore == nil || feature.RefinedGroups == nil ||
		feature.Coverage.ProgramObjectsOmitted < 0 {
		return fmt.Errorf("core feature is incomplete")
	}
	objects := make(map[string]struct{}, len(program.Objects))
	for _, object := range program.Objects {
		objects[object.ID] = struct{}{}
	}
	blocks := make(map[string]struct{})
	if err := validateBrowserCoreBlocks(feature.RefinedCore, blocks, objects, openable); err != nil {
		return err
	}
	groups := make(map[string]struct{}, len(feature.RefinedGroups))
	modelGroups := 0
	localGroups := 0
	for index, group := range feature.RefinedGroups {
		if !validTargetNavigationText(group.ID) || !validTargetNavigationText(group.Name) ||
			!validTargetNavigationText(group.Purpose) || group.CoreBlockIDs == nil ||
			len(group.CoreBlockIDs) == 0 ||
			!browserClosedText(group.Authority, "model", "local_unassigned") {
			return fmt.Errorf("core group %d is invalid", index)
		}
		if _, duplicate := groups[group.ID]; duplicate {
			return fmt.Errorf("duplicate core group")
		}
		groups[group.ID] = struct{}{}
		if group.Authority == "model" {
			modelGroups++
		} else {
			localGroups++
			if localGroups != 1 || index != len(feature.RefinedGroups)-1 {
				return fmt.Errorf("local-unassigned core group is not canonical")
			}
		}
		inside := make(map[string]struct{}, len(group.CoreBlockIDs))
		for _, blockID := range group.CoreBlockIDs {
			if _, ok := blocks[blockID]; !ok {
				return fmt.Errorf("core group cites an unknown responsibility")
			}
			if _, duplicate := inside[blockID]; duplicate {
				return fmt.Errorf("core group repeats a responsibility")
			}
			inside[blockID] = struct{}{}
		}
	}
	if localGroups != 0 && modelGroups == 0 {
		return fmt.Errorf("local-unassigned core group lacks model grouping authority")
	}
	return nil
}

func validateBrowserCoreBlocks(
	values []BrowserCoreBlock,
	blocks map[string]struct{},
	objects map[string]struct{},
	openable map[string]struct{},
) error {
	if values == nil {
		return fmt.Errorf("core block collection is missing")
	}
	for index, block := range values {
		if !validTargetNavigationText(block.ID) || !validTargetNavigationText(block.Name) ||
			!validTargetNavigationText(block.Purpose) || block.Files == nil ||
			block.RepresentativeSymbols == nil || block.Children == nil {
			return fmt.Errorf("core block %d is invalid", index)
		}
		if _, duplicate := blocks[block.ID]; duplicate {
			return fmt.Errorf("duplicate core responsibility")
		}
		blocks[block.ID] = struct{}{}
		for _, file := range block.Files {
			if _, ok := openable[file.Path]; !ok || validateManifestPath(file.Path) != nil {
				return fmt.Errorf("core file is not openable")
			}
		}
		for _, representative := range block.RepresentativeSymbols {
			if !validTargetNavigationText(representative.Symbol.NodeID) ||
				!validTargetNavigationText(representative.Symbol.Name) ||
				representative.UnresolvedOutgoing < 0 {
				return fmt.Errorf("core representative symbol is invalid")
			}
			if _, ok := objects[representative.Symbol.NodeID]; !ok {
				return fmt.Errorf("core representative symbol is absent from program objects")
			}
			if err := validateBrowserLocation(representative.Symbol.Location, openable); err != nil {
				return err
			}
		}
		if err := validateBrowserCoreBlocks(block.Children, blocks, objects, openable); err != nil {
			return err
		}
	}
	return nil
}

func (feature *BrowserEntrypointFeature) validate(openable map[string]struct{}) error {
	if feature == nil || feature.Entrypoints == nil {
		return fmt.Errorf("entrypoint feature is incomplete")
	}
	seen := make(map[string]struct{}, len(feature.Entrypoints))
	for index, entrypoint := range feature.Entrypoints {
		if !validTargetNavigationText(entrypoint.ObjectID) ||
			!validTargetNavigationText(entrypoint.Kind) ||
			!validTargetNavigationText(entrypoint.Name) ||
			entrypoint.Signature != "" && !validTargetNavigationText(entrypoint.Signature) {
			return fmt.Errorf("entrypoint %d is invalid", index)
		}
		if _, duplicate := seen[entrypoint.ObjectID]; duplicate {
			return fmt.Errorf("duplicate entrypoint")
		}
		seen[entrypoint.ObjectID] = struct{}{}
		if err := validateBrowserLocation(entrypoint.Location, openable); err != nil {
			return err
		}
	}
	return nil
}

func (feature *BrowserIntegrationFeature) validate(
	openable map[string]struct{},
) (map[string]struct{}, error) {
	if feature == nil || feature.Dependencies == nil {
		return nil, fmt.Errorf("integration feature is incomplete")
	}
	dependencies := make(map[string]struct{}, len(feature.Dependencies))
	uses := make(map[string]struct{})
	for index, dependency := range feature.Dependencies {
		if !validTargetNavigationText(dependency.DependencyID) ||
			!validTargetNavigationText(dependency.Kind) ||
			!validTargetNavigationText(dependency.Name) ||
			!validTargetNavigationText(dependency.PackagePath) || dependency.Uses == nil {
			return nil, fmt.Errorf("integration dependency %d is invalid", index)
		}
		if _, duplicate := dependencies[dependency.DependencyID]; duplicate {
			return nil, fmt.Errorf("duplicate integration dependency")
		}
		dependencies[dependency.DependencyID] = struct{}{}
		for _, use := range dependency.Uses {
			if !validTargetNavigationText(use.RelationID) || use.WitnessIndex < 0 ||
				!validTargetNavigationText(use.ExternalSymbolID) ||
				!validTargetNavigationText(use.CallerID) ||
				!validTargetNavigationText(use.CallerName) ||
				!validTargetNavigationText(use.CanonicalCallee) ||
				!validTargetNavigationText(use.Label) || !validTargetNavigationText(use.Mechanism) ||
				!browserClosedText(use.Authority, "exact_external_symbol", "syntactic_unresolved") {
				return nil, fmt.Errorf("integration use is invalid")
			}
			if err := validateBrowserLocation(use.Callsite, openable); err != nil {
				return nil, err
			}
			key := browserIntegrationUseKey(
				dependency.DependencyID, use.RelationID, use.WitnessIndex, use.ExternalSymbolID,
			)
			if _, duplicate := uses[key]; duplicate {
				return nil, fmt.Errorf("duplicate integration use")
			}
			uses[key] = struct{}{}
		}
	}
	return uses, nil
}

func (feature *BrowserActivityPathFeature) validate(
	entrypoints *BrowserEntrypointFeature,
	uses map[string]struct{},
	openable map[string]struct{},
) error {
	if feature == nil || feature.Objects == nil || feature.Routes == nil || feature.Outcomes == nil {
		return fmt.Errorf("activity path feature is incomplete")
	}
	objects := make(map[string]struct{}, len(feature.Objects))
	for _, object := range feature.Objects {
		if !validTargetNavigationText(object.ObjectID) || !validTargetNavigationText(object.Kind) ||
			!validTargetNavigationText(object.Name) {
			return fmt.Errorf("activity path object is invalid")
		}
		if _, duplicate := objects[object.ObjectID]; duplicate {
			return fmt.Errorf("duplicate activity path object")
		}
		objects[object.ObjectID] = struct{}{}
	}
	activityIDs := make(map[string]struct{}, len(entrypoints.Entrypoints))
	for _, entrypoint := range entrypoints.Entrypoints {
		activityIDs[entrypoint.ObjectID] = struct{}{}
	}
	routes := make(map[string]struct{}, len(feature.Routes))
	for _, route := range feature.Routes {
		if !validTargetNavigationText(route.RouteID) ||
			!browserClosedText(route.Status, "exact", "possible", "frontier", "unconnected") ||
			route.Steps == nil || route.Frontier == nil || route.Distance != len(route.Steps) {
			return fmt.Errorf("activity path route is invalid")
		}
		if _, duplicate := routes[route.RouteID]; duplicate {
			return fmt.Errorf("duplicate activity path route")
		}
		routes[route.RouteID] = struct{}{}
		if _, ok := objects[route.CallerID]; !ok {
			return fmt.Errorf("activity path caller is absent")
		}
		if route.ActivityID != "" {
			if _, ok := objects[route.ActivityID]; !ok {
				return fmt.Errorf("activity path start object is absent")
			}
			if _, ok := activityIDs[route.ActivityID]; !ok {
				return fmt.Errorf("activity path start is not an entrypoint")
			}
		}
		for _, step := range route.Steps {
			if _, ok := objects[step.FromID]; !ok {
				return fmt.Errorf("activity path step source is absent")
			}
			if _, ok := objects[step.ToID]; !ok {
				return fmt.Errorf("activity path step target is absent")
			}
			if !validTargetNavigationText(step.Kind) ||
				!browserClosedText(step.Authority, "exact", "possible") {
				return fmt.Errorf("activity path step is invalid")
			}
			if step.Location != nil {
				if err := validateBrowserLocation(*step.Location, openable); err != nil {
					return err
				}
			}
		}
		for _, frontier := range route.Frontier {
			if !validTargetNavigationText(frontier) {
				return fmt.Errorf("activity path frontier is invalid")
			}
		}
	}
	coveredUses := make(map[string]struct{}, len(feature.Outcomes))
	for _, outcome := range feature.Outcomes {
		if _, ok := routes[outcome.RouteID]; !ok {
			return fmt.Errorf("activity path outcome route is absent")
		}
		key := browserIntegrationUseKey(
			outcome.DependencyID, outcome.RelationID, outcome.WitnessIndex, outcome.ExternalSymbolID,
		)
		if _, ok := uses[key]; !ok {
			return fmt.Errorf("activity path outcome use is absent")
		}
		if _, duplicate := coveredUses[key]; duplicate {
			return fmt.Errorf("activity path outcome repeats a use")
		}
		coveredUses[key] = struct{}{}
	}
	if len(coveredUses) != len(uses) {
		return fmt.Errorf("activity paths do not cover every integration use")
	}
	return nil
}

func (feature *BrowserSurfaceFeature) validate(
	openable map[string]struct{},
) (map[string]BrowserSurface, error) {
	if feature == nil || feature.Facts == nil || feature.Surfaces == nil {
		return nil, fmt.Errorf("surface feature is incomplete")
	}
	facts, err := validateBrowserJSTSFacts(feature.Facts, openable)
	if err != nil {
		return nil, err
	}
	surfaces := make(map[string]BrowserSurface, len(feature.Surfaces))
	for _, surface := range feature.Surfaces {
		if !validTargetNavigationText(surface.SurfaceID) || !validTargetNavigationText(surface.Kind) ||
			!browserClosedText(surface.Role, "product", "supporting", "script", "unknown") ||
			!browserClosedText(surface.Disposition,
				"product_surface", "supporting_code", "tool", "unknown") ||
			!validTargetNavigationText(surface.Name) || surface.EntryRefs == nil ||
			surface.EvidenceRefs == nil {
			return nil, fmt.Errorf("surface is invalid")
		}
		if _, duplicate := surfaces[surface.SurfaceID]; duplicate {
			return nil, fmt.Errorf("duplicate surface")
		}
		for _, refs := range [][]string{surface.EntryRefs, surface.EvidenceRefs} {
			for _, ref := range refs {
				if _, ok := facts[ref]; !ok {
					return nil, fmt.Errorf("surface fact reference is absent")
				}
			}
		}
		if err := validateBrowserLocation(surface.Location, openable); err != nil {
			return nil, err
		}
		surfaces[surface.SurfaceID] = surface
	}
	return surfaces, nil
}

func (feature *BrowserCrossSurfacePathFeature) validate(
	surfaces map[string]BrowserSurface,
	openable map[string]struct{},
) error {
	if feature == nil || feature.Facts == nil || feature.Paths == nil ||
		feature.Coverage.RoutesObserved < 0 || feature.Coverage.HTTPUsesObserved < 0 {
		return fmt.Errorf("cross-surface path feature is incomplete")
	}
	facts, err := validateBrowserJSTSFacts(feature.Facts, openable)
	if err != nil {
		return err
	}
	paths := make(map[string]struct{}, len(feature.Paths))
	for _, path := range feature.Paths {
		if !validTargetNavigationText(path.PathID) || !validTargetNavigationText(path.Name) ||
			!validTargetNavigationText(path.Outcome) || path.Steps == nil || len(path.Steps) == 0 ||
			path.Frontier != "" && !validTargetNavigationText(path.Frontier) {
			return fmt.Errorf("cross-surface path is invalid")
		}
		if _, duplicate := paths[path.PathID]; duplicate {
			return fmt.Errorf("duplicate cross-surface path")
		}
		paths[path.PathID] = struct{}{}
		for position, step := range path.Steps {
			if step.Ordinal != position+1 || !validTargetNavigationText(step.Kind) ||
				!validTargetNavigationText(step.Label) ||
				!browserClosedText(step.Resolution, "exact", "alternatives", "unresolved") ||
				!browserClosedText(step.Authority,
					"exact_static", "resolved_indirect", "possible", "unresolved_frontier") ||
				step.TargetRefs == nil {
				return fmt.Errorf("cross-surface step is invalid")
			}
			if _, ok := facts[step.SourceRef]; !ok {
				return fmt.Errorf("cross-surface source fact is absent")
			}
			for _, ref := range step.TargetRefs {
				if _, ok := facts[ref]; !ok {
					return fmt.Errorf("cross-surface target fact is absent")
				}
			}
			if err := validateBrowserLocation(step.Location, openable); err != nil {
				return err
			}
		}
	}
	for _, fact := range feature.Facts {
		if fact.Category != "surface" {
			continue
		}
		surface, ok := surfaces[fact.Ref]
		if !ok || fact.Kind != surface.Kind || fact.Label != surface.Name || fact.Location == nil ||
			*fact.Location != surface.Location {
			return fmt.Errorf("cross-surface fact does not match surface authority")
		}
	}
	return nil
}

func validateBrowserJSTSFacts(
	values []BrowserJSTSFact,
	openable map[string]struct{},
) (map[string]BrowserJSTSFact, error) {
	facts := make(map[string]BrowserJSTSFact, len(values))
	for _, fact := range values {
		if !validTargetNavigationText(fact.Ref) || !validTargetNavigationText(fact.Category) ||
			!validTargetNavigationText(fact.Kind) || !validTargetNavigationText(fact.Label) {
			return nil, fmt.Errorf("JavaScript/TypeScript fact is invalid")
		}
		if _, duplicate := facts[fact.Ref]; duplicate {
			return nil, fmt.Errorf("duplicate JavaScript/TypeScript fact")
		}
		if fact.Location != nil {
			if err := validateBrowserLocation(*fact.Location, openable); err != nil {
				return nil, err
			}
		}
		facts[fact.Ref] = fact
	}
	return facts, nil
}

func (features BrowserTargetFeatures) collectLocations(paths map[string]struct{}) {
	add := func(location *BrowserLocation) {
		if location != nil {
			paths[location.Path] = struct{}{}
		}
	}
	for _, object := range features.Program.Objects {
		add(object.Location)
	}
	for _, relation := range features.Program.Relations {
		add(relation.Location)
		for _, witness := range relation.Witnesses {
			add(witness.Location)
		}
	}
	if features.Core != nil {
		collectBrowserCoreLocations(features.Core.RefinedCore, paths)
	}
	if features.Entrypoints != nil {
		for _, entrypoint := range features.Entrypoints.Entrypoints {
			location := entrypoint.Location
			add(&location)
		}
	}
	if features.Integrations != nil {
		for _, dependency := range features.Integrations.Dependencies {
			for _, use := range dependency.Uses {
				location := use.Callsite
				add(&location)
			}
		}
	}
	if features.ActivityPaths != nil {
		for _, route := range features.ActivityPaths.Routes {
			for _, step := range route.Steps {
				add(step.Location)
			}
		}
	}
	for _, feature := range []*BrowserSurfaceFeature{features.Surfaces} {
		if feature == nil {
			continue
		}
		for _, fact := range feature.Facts {
			add(fact.Location)
		}
		for _, surface := range feature.Surfaces {
			location := surface.Location
			add(&location)
		}
	}
	if features.CrossSurfacePaths != nil {
		for _, fact := range features.CrossSurfacePaths.Facts {
			add(fact.Location)
		}
		for _, path := range features.CrossSurfacePaths.Paths {
			for _, step := range path.Steps {
				location := step.Location
				add(&location)
			}
		}
	}
}

func collectBrowserCoreLocations(values []BrowserCoreBlock, paths map[string]struct{}) {
	for _, block := range values {
		for _, file := range block.Files {
			paths[file.Path] = struct{}{}
		}
		for _, representative := range block.RepresentativeSymbols {
			paths[representative.Symbol.Location.Path] = struct{}{}
		}
		collectBrowserCoreLocations(block.Children, paths)
	}
}

func validateBrowserOpenablePaths(values []string) (map[string]struct{}, error) {
	if values == nil {
		return nil, fmt.Errorf("openable paths are missing")
	}
	result := make(map[string]struct{}, len(values))
	previous := ""
	for index, sourcePath := range values {
		if err := validateManifestPath(sourcePath); err != nil {
			return nil, fmt.Errorf("openable path %d: %w", index, err)
		}
		if previous != "" && previous >= sourcePath {
			return nil, fmt.Errorf("openable paths are not uniquely sorted")
		}
		previous = sourcePath
		result[sourcePath] = struct{}{}
	}
	return result, nil
}

func validateBrowserLocation(location BrowserLocation, openable map[string]struct{}) error {
	if err := validateManifestPath(location.Path); err != nil || location.Line < 0 || location.Column < 0 {
		return fmt.Errorf("source location is invalid")
	}
	if _, ok := openable[location.Path]; !ok {
		return fmt.Errorf("source location is outside browser openability")
	}
	return nil
}

func browserIntegrationUseKey(dependencyID, relationID string, witnessIndex int, externalSymbolID string) string {
	return dependencyID + "\x00" + relationID + "\x00" + fmt.Sprintf("%d", witnessIndex) +
		"\x00" + externalSymbolID
}

func browserClosedText(value string, allowed ...string) bool {
	if !validTargetNavigationText(value) {
		return false
	}
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func validBrowserTargetHref(value string) bool {
	if value == "#/program" || validTargetNavigationHref(value, false) {
		return true
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Path != "" || parsed.Fragment != "/program" ||
		!strings.HasPrefix(parsed.RawQuery, "target=") || strings.Contains(parsed.RawQuery, "&") {
		return false
	}
	index := strings.TrimPrefix(parsed.RawQuery, "target=")
	if index == "" || len(index) > 1 && index[0] == '0' {
		return false
	}
	for _, character := range index {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func sameBrowserPathSet(left, right map[string]struct{}) bool {
	if len(left) != len(right) {
		return false
	}
	for sourcePath := range left {
		if _, ok := right[sourcePath]; !ok {
			return false
		}
	}
	return true
}
