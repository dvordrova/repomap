package report

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/programpage"
	"github.com/dvordrova/repomap/internal/runtimeportfolio"
	"github.com/dvordrova/repomap/internal/snapshot"
)

const runtimePortfolioUnclassifiedReason = "No repository role maps this analyzed target."

// RuntimePortfolioView is the persisted, browser-facing projection of the
// canonical repository runtime portfolio. Provider refs, semantic evidence
// kinds, target catalogs, and coverage accounting remain in the artifact; the
// browser receives only exact ProgramTarget joins and source actions.
type RuntimePortfolioView struct {
	Version             int                                      `json:"version"`
	Roles               []RuntimePortfolioRoleView               `json:"roles"`
	UnclassifiedTargets []RuntimePortfolioUnclassifiedTargetView `json:"unclassified_targets"`
}

type RuntimePortfolioRoleView struct {
	ID              string                            `json:"id"`
	Name            string                            `json:"name"`
	Purpose         string                            `json:"purpose"`
	Prominence      runtimeportfolio.Prominence       `json:"prominence"`
	RoleKind        runtimeportfolio.RoleKind         `json:"role_kind"`
	Requiredness    runtimeportfolio.Requiredness     `json:"requiredness"`
	Confidence      runtimeportfolio.Confidence       `json:"confidence"`
	MappingStatus   runtimeportfolio.MappingStatus    `json:"mapping_status"`
	Implementations []runtimeportfolio.Implementation `json:"implementations"`
	Evidence        []RuntimePortfolioEvidenceView    `json:"evidence"`
}

type RuntimePortfolioEvidenceView struct {
	Label    string                    `json:"label"`
	Location runtimeportfolio.Location `json:"location"`
}

type RuntimePortfolioUnclassifiedTargetView struct {
	ProgramTargetID string `json:"program_target_id"`
	Reason          string `json:"reason"`
}

// NewRuntimePortfolioView removes backend-only semantic authority while
// preserving every role, mapping, evidence location, and unclassified target.
func NewRuntimePortfolioView(result runtimeportfolio.Result) (*RuntimePortfolioView, error) {
	if err := result.Validate(); err != nil {
		return nil, fmt.Errorf("runtime portfolio view: artifact: %w", err)
	}
	view := &RuntimePortfolioView{
		Version:             result.Version,
		Roles:               make([]RuntimePortfolioRoleView, 0, len(result.Roles)),
		UnclassifiedTargets: make([]RuntimePortfolioUnclassifiedTargetView, 0, len(result.UnclassifiedTargetIDs)),
	}
	for _, role := range result.Roles {
		projected := RuntimePortfolioRoleView{
			ID: role.ID, Name: role.Name, Purpose: role.Purpose,
			Prominence: role.Prominence, RoleKind: role.Kind,
			Requiredness: role.Requiredness, Confidence: role.Confidence,
			MappingStatus:   role.MappingStatus,
			Implementations: append([]runtimeportfolio.Implementation(nil), role.Implementations...),
			Evidence:        make([]RuntimePortfolioEvidenceView, 0, len(role.Evidence)),
		}
		for _, evidence := range role.Evidence {
			projected.Evidence = append(projected.Evidence, RuntimePortfolioEvidenceView{
				Label: evidence.Label, Location: evidence.Location,
			})
		}
		view.Roles = append(view.Roles, projected)
	}
	for _, targetID := range result.UnclassifiedTargetIDs {
		view.UnclassifiedTargets = append(view.UnclassifiedTargets, RuntimePortfolioUnclassifiedTargetView{
			ProgramTargetID: targetID,
			Reason:          runtimePortfolioUnclassifiedReason,
		})
	}
	if err := view.Validate(); err != nil {
		return nil, err
	}
	return view, nil
}

// Validate checks the standalone report/browser shape. Exact equality with
// the semantic artifact is re-derived by RunManifest verification.
func (view RuntimePortfolioView) Validate() error {
	if view.Version != runtimeportfolio.Version || view.Roles == nil || view.UnclassifiedTargets == nil {
		return fmt.Errorf("runtime portfolio view: invalid identity")
	}
	roleIDs := make(map[string]struct{}, len(view.Roles))
	roleNames := make(map[string]struct{}, len(view.Roles))
	mappedTargets := make(map[string]struct{})
	previousRoleKey := ""
	for index, role := range view.Roles {
		if !validRuntimePortfolioRoleID(role.ID) || !validRuntimePortfolioText(role.Name) ||
			!validRuntimePortfolioText(role.Purpose) || !validRuntimePortfolioProminence(role.Prominence) ||
			!validRuntimePortfolioRoleKind(role.RoleKind) || !validRuntimePortfolioRequiredness(role.Requiredness) ||
			!validRuntimePortfolioConfidence(role.Confidence) || !validRuntimePortfolioMappingStatus(role.MappingStatus) ||
			role.Implementations == nil || role.Evidence == nil || len(role.Evidence) == 0 {
			return fmt.Errorf("runtime portfolio view: role %d is invalid", index)
		}
		if _, duplicate := roleIDs[role.ID]; duplicate {
			return fmt.Errorf("runtime portfolio view: duplicate role identity")
		}
		roleIDs[role.ID] = struct{}{}
		nameKey := strings.ToLower(role.Name)
		if _, duplicate := roleNames[nameKey]; duplicate {
			return fmt.Errorf("runtime portfolio view: duplicate role name")
		}
		roleNames[nameKey] = struct{}{}
		roleKey := runtimePortfolioProminenceOrder(role.Prominence) + "\x00" + nameKey + "\x00" + role.ID
		if previousRoleKey != "" && previousRoleKey >= roleKey {
			return fmt.Errorf("runtime portfolio view: roles are not canonical")
		}
		previousRoleKey = roleKey
		if role.MappingStatus == runtimeportfolio.MappingMapped && len(role.Implementations) == 0 {
			return fmt.Errorf("runtime portfolio view: mapped role has no implementation")
		}
		if role.MappingStatus == runtimeportfolio.MappingUnknown && len(role.Implementations) != 0 {
			return fmt.Errorf("runtime portfolio view: unresolved role cites an implementation")
		}
		previousImplementation := ""
		for _, implementation := range role.Implementations {
			if !validRuntimePortfolioText(implementation.ProgramTargetID) ||
				(implementation.Mode != "" && !validRuntimePortfolioText(implementation.Mode)) {
				return fmt.Errorf("runtime portfolio view: role implementation is invalid")
			}
			key := implementation.ProgramTargetID + "\x00" + implementation.Mode
			if previousImplementation != "" && previousImplementation >= key {
				return fmt.Errorf("runtime portfolio view: role implementations are not canonical")
			}
			previousImplementation = key
			mappedTargets[implementation.ProgramTargetID] = struct{}{}
		}
		for _, evidence := range role.Evidence {
			if !validRuntimePortfolioText(evidence.Label) || !validRuntimePortfolioLocation(evidence.Location) {
				return fmt.Errorf("runtime portfolio view: role evidence is invalid")
			}
		}
	}
	previousTargetID := ""
	for index, target := range view.UnclassifiedTargets {
		if !validRuntimePortfolioText(target.ProgramTargetID) || target.Reason != runtimePortfolioUnclassifiedReason {
			return fmt.Errorf("runtime portfolio view: unclassified target %d is invalid", index)
		}
		if previousTargetID != "" && previousTargetID >= target.ProgramTargetID {
			return fmt.Errorf("runtime portfolio view: unclassified targets are not canonical")
		}
		if _, mapped := mappedTargets[target.ProgramTargetID]; mapped {
			return fmt.Errorf("runtime portfolio view: mapped target is also unclassified")
		}
		previousTargetID = target.ProgramTargetID
	}
	if len(mappedTargets)+len(view.UnclassifiedTargets) == 0 {
		return fmt.Errorf("runtime portfolio view: target coverage is empty")
	}
	return nil
}

func restoreRuntimePortfolioView(runDir string, data *ReportData) error {
	legacyPortfolioRaw, legacyPortfolioPresent, err := readBoundedProgramArtifact(
		filepath.Join(runDir, snapshot.TargetPagePortfolioArtifactFilename),
		snapshot.MaxTargetPagePortfolioBytes,
		"target page portfolio",
		true,
	)
	if err != nil {
		return err
	}
	programPortfolioRaw, programPortfolioPresent, err := readBoundedProgramArtifact(
		filepath.Join(runDir, programpage.ArtifactFilename),
		programpage.MaxArtifactBytes,
		"program page portfolio",
		true,
	)
	if err != nil {
		return err
	}
	runtimeRaw, runtimePresent, err := readBoundedProgramArtifact(
		filepath.Join(runDir, runtimeportfolio.ArtifactFilename),
		runtimeportfolio.MaxArtifactBytes,
		"runtime portfolio",
		true,
	)
	if err != nil {
		return err
	}
	if legacyPortfolioPresent && programPortfolioPresent {
		return fmt.Errorf("report: legacy and neutral page portfolio authority are mutually exclusive")
	}
	pagePortfolioPresent := legacyPortfolioPresent || programPortfolioPresent
	if pagePortfolioPresent != runtimePresent {
		return fmt.Errorf("report: page and runtime portfolio authority must be published together")
	}
	if !runtimePresent {
		return nil
	}
	result, err := runtimeportfolio.Decode(runtimeRaw)
	if err != nil {
		return fmt.Errorf("report: decode runtime portfolio: %w", err)
	}
	if programPortfolioPresent {
		portfolio, decodeErr := programpage.Decode(programPortfolioRaw)
		if decodeErr != nil {
			return fmt.Errorf("report: decode runtime program page portfolio: %w", decodeErr)
		}
		if err := validateRuntimeProgramPageCoverage(result, portfolio); err != nil {
			return fmt.Errorf("report: %w", err)
		}
	} else {
		containerRaw, _, readErr := readBoundedProgramArtifact(
			filepath.Join(runDir, snapshot.TargetRunContainerArtifactFilename),
			snapshot.MaxTargetRunContainerBytes,
			"target run container",
			false,
		)
		if readErr != nil {
			return readErr
		}
		container, decodeErr := snapshot.DecodeTargetRunContainer(containerRaw)
		if decodeErr != nil {
			return fmt.Errorf("report: decode runtime portfolio target container: %w", decodeErr)
		}
		portfolio, decodeErr := snapshot.DecodeTargetPagePortfolio(legacyPortfolioRaw)
		if decodeErr != nil {
			return fmt.Errorf("report: decode runtime target page portfolio: %w", decodeErr)
		}
		if err := portfolio.ValidateAgainstContainer(container); err != nil {
			return fmt.Errorf("report: runtime target page portfolio: %w", err)
		}
		if result.TargetPagePortfolioSHA256 != portfolio.SHA256 {
			return fmt.Errorf("report: runtime portfolio target-page binding mismatch")
		}
		if len(result.Targets) != len(portfolio.Targets) {
			return fmt.Errorf("report: runtime portfolio target coverage does not match target pages")
		}
	}
	if data.ProgramPortfolio == nil {
		return fmt.Errorf("report: runtime portfolio requires an exact current ProgramTarget")
	}
	defaultEntry, err := data.ProgramPortfolio.defaultEntry()
	if err != nil {
		return fmt.Errorf("report: runtime portfolio current ProgramTarget: %w", err)
	}
	if err := validateRuntimePortfolioCurrentTarget(result, defaultEntry.Target); err != nil {
		return err
	}
	view, err := NewRuntimePortfolioView(result)
	if err != nil {
		return fmt.Errorf("report: project runtime portfolio: %w", err)
	}
	data.RuntimePortfolio = view
	return nil
}

func validateRuntimePortfolioCurrentTarget(result runtimeportfolio.Result, current programindex.Target) error {
	for _, target := range result.Targets {
		if target.ProgramTargetID != current.ID {
			continue
		}
		if target.DisplayName != current.Name || target.Language != current.Language ||
			target.Kind != current.Kind || target.Selector != current.Selector {
			return fmt.Errorf("report: runtime portfolio current ProgramTarget metadata mismatch")
		}
		return nil
	}
	return fmt.Errorf("report: runtime portfolio omits the current ProgramTarget")
}

func (view RuntimePortfolioView) validateTargetNavigation(navigation *TargetNavigationPortfolio) error {
	if err := view.Validate(); err != nil {
		return err
	}
	if navigation == nil {
		return fmt.Errorf("runtime portfolio view: complete target navigation is missing")
	}
	portfolioTargets := make(map[string]struct{})
	for _, role := range view.Roles {
		for _, implementation := range role.Implementations {
			portfolioTargets[implementation.ProgramTargetID] = struct{}{}
		}
	}
	for _, target := range view.UnclassifiedTargets {
		portfolioTargets[target.ProgramTargetID] = struct{}{}
	}
	if len(portfolioTargets) != len(navigation.Targets) {
		return fmt.Errorf("runtime portfolio view: ProgramTarget set does not match target navigation")
	}
	for _, target := range navigation.Targets {
		if _, present := portfolioTargets[target.TargetID]; !present {
			return fmt.Errorf("runtime portfolio view: ProgramTarget set does not match target navigation")
		}
	}
	return nil
}

func validRuntimePortfolioLocation(location runtimeportfolio.Location) bool {
	if location.Line < 0 || location.Column < 0 || (location.Line == 0 && location.Column != 0) {
		return false
	}
	return validateManifestPath(location.Path) == nil
}

func validRuntimePortfolioText(value string) bool {
	return len(value) <= runtimeportfolio.MaxTextBytes && validTargetNavigationText(value)
}

func validRuntimePortfolioRoleID(value string) bool {
	const prefix = "runtime-role-"
	if !strings.HasPrefix(value, prefix) || len(value) != len(prefix)+24 {
		return false
	}
	for _, character := range value[len(prefix):] {
		if character < '0' || character > '9' && character < 'a' || character > 'f' {
			return false
		}
	}
	return true
}

func validRuntimePortfolioProminence(value runtimeportfolio.Prominence) bool {
	return value == runtimeportfolio.ProminencePrimary || value == runtimeportfolio.ProminenceSupporting ||
		value == runtimeportfolio.ProminenceUnknown
}

func runtimePortfolioProminenceOrder(value runtimeportfolio.Prominence) string {
	switch value {
	case runtimeportfolio.ProminencePrimary:
		return "1"
	case runtimeportfolio.ProminenceSupporting:
		return "2"
	default:
		return "3"
	}
}

func validRuntimePortfolioRoleKind(value runtimeportfolio.RoleKind) bool {
	switch value {
	case runtimeportfolio.RoleKindLibrary, runtimeportfolio.RoleKindService, runtimeportfolio.RoleKindDaemon, runtimeportfolio.RoleKindWorker,
		runtimeportfolio.RoleKindCLI, runtimeportfolio.RoleKindExample, runtimeportfolio.RoleKindSupportingTool, runtimeportfolio.RoleKindUnknown:
		return true
	default:
		return false
	}
}

func validRuntimePortfolioRequiredness(value runtimeportfolio.Requiredness) bool {
	switch value {
	case runtimeportfolio.RequirednessRequired, runtimeportfolio.RequirednessOptional,
		runtimeportfolio.RequirednessExperimental, runtimeportfolio.RequirednessUnknown:
		return true
	default:
		return false
	}
}

func validRuntimePortfolioConfidence(value runtimeportfolio.Confidence) bool {
	return value == runtimeportfolio.ConfidenceHigh || value == runtimeportfolio.ConfidenceMedium ||
		value == runtimeportfolio.ConfidenceLow || value == runtimeportfolio.ConfidenceUnknown
}

func validRuntimePortfolioMappingStatus(value runtimeportfolio.MappingStatus) bool {
	return value == runtimeportfolio.MappingMapped || value == runtimeportfolio.MappingUnknown
}
