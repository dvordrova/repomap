package report

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/evidence"
)

const (
	ArchitectureGroundingFile    = "architecture_grounding.json"
	ArchitectureGroundingVersion = 2
	maxArchitectureGroundingSize = 4 * 1024 * 1024
)

type ArchitectureGrounding struct {
	Version             int                           `json:"version"`
	RepositoryArchetype ArchitectureArchetype         `json:"repository_archetype"`
	GroundingMode       componentmap.GroundingMode    `json:"grounding_mode"`
	BehaviorAnchors     []ArchitectureBehaviorAnchor  `json:"behavior_anchors"`
	Relationships       []ArchitectureBehaviorHandoff `json:"relationships"`
}

type ArchitectureArchetype struct {
	Selected     componentmap.RepositoryArchetype `json:"selected"`
	Evidence     []string                         `json:"evidence"`
	Alternatives []string                         `json:"alternatives"`
}

type ArchitectureBehaviorAnchor struct {
	ID                string                          `json:"id"`
	Kind              componentmap.BehaviorAnchorKind `json:"kind"`
	Label             string                          `json:"label"`
	Location          evidence.Location               `json:"location"`
	Scenario          architectureGroundingScenario   `json:"scenario"`
	Producer          evidence.Provenance             `json:"producer"`
	Certainty         evidence.Certainty              `json:"certainty"`
	AssociatedMembers []ArchitectureAnchorMember      `json:"associated_members"`
	Limitations       []string                        `json:"limitations"`
}

type ArchitectureAnchorMember struct {
	ID            string            `json:"id"`
	EquivalentIDs []string          `json:"equivalent_ids,omitempty"`
	Package       string            `json:"package"`
	Name          string            `json:"name"`
	Location      evidence.Location `json:"location"`
}

type ArchitectureBehaviorHandoff struct {
	ID                      string              `json:"id"`
	From                    string              `json:"from_anchor_id"`
	To                      string              `json:"to_anchor_id"`
	Kind                    string              `json:"kind"`
	EvidenceKind            string              `json:"evidence_kind,omitempty"`
	Location                evidence.Location   `json:"location"`
	RepresentativeLocations []evidence.Location `json:"representative_locations,omitempty"`
	WitnessIDs              []string            `json:"witness_ids,omitempty"`
	WitnessCount            int                 `json:"witness_count,omitempty"`
	PackageCount            int                 `json:"package_count,omitempty"`
	Certainty               evidence.Certainty  `json:"certainty"`
	Producer                evidence.Provenance `json:"producer"`
}

type architectureGroundingScenario struct {
	ID     string   `json:"id"`
	GOOS   string   `json:"goos"`
	GOARCH string   `json:"goarch"`
	Tags   []string `json:"tags"`
}

func parseArchitectureGrounding(runDir string) (*ArchitectureGrounding, string) {
	path := filepath.Join(runDir, ArchitectureGroundingFile)
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ""
		}
		return nil, "architecture grounding: cannot inspect saved artifact"
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxArchitectureGroundingSize {
		return nil, "architecture grounding: saved artifact is not a bounded regular file"
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "architecture grounding: cannot read saved artifact"
	}
	var grounding ArchitectureGrounding
	if err := json.Unmarshal(data, &grounding); err != nil {
		return nil, "architecture grounding: saved artifact contains invalid json"
	}
	if err := validateArchitectureGrounding(grounding); err != nil {
		return nil, "architecture grounding: " + err.Error()
	}
	return &grounding, ""
}

func validateArchitectureGrounding(grounding ArchitectureGrounding) error {
	if grounding.Version != 1 && grounding.Version != ArchitectureGroundingVersion {
		return fmt.Errorf("unsupported version %d", grounding.Version)
	}
	if !validArchitectureArchetype(grounding.RepositoryArchetype.Selected) {
		return fmt.Errorf("unsupported repository archetype")
	}
	if !validArchitectureGroundingMode(grounding.GroundingMode) {
		return fmt.Errorf("unsupported grounding mode")
	}
	if len(grounding.BehaviorAnchors) > 256 || len(grounding.Relationships) > 512 {
		return fmt.Errorf("artifact exceeds grounding limits")
	}
	anchorIDs := make(map[string]struct{}, len(grounding.BehaviorAnchors))
	for _, anchor := range grounding.BehaviorAnchors {
		if strings.TrimSpace(anchor.ID) == "" || strings.TrimSpace(anchor.Label) == "" ||
			!validArchitectureBehaviorAnchorKind(anchor.Kind) || !validGroundingLocation(anchor.Location) ||
			!anchor.Certainty.Valid() || anchor.Scenario.ID == "" || len(anchor.AssociatedMembers) == 0 ||
			len(anchor.AssociatedMembers) > 16 || len(anchor.Limitations) == 0 || len(anchor.Limitations) > 8 {
			return fmt.Errorf("behavior anchor is incomplete")
		}
		if _, duplicate := anchorIDs[anchor.ID]; duplicate {
			return fmt.Errorf("behavior anchor ids are not unique")
		}
		anchorIDs[anchor.ID] = struct{}{}
		for _, member := range anchor.AssociatedMembers {
			if member.ID == "" || !validGroundingLocation(member.Location) {
				return fmt.Errorf("behavior anchor member is incomplete")
			}
		}
	}
	for _, relationship := range grounding.Relationships {
		if relationship.ID == "" || relationship.From == relationship.To ||
			!validGroundingLocation(relationship.Location) || !relationship.Certainty.Valid() {
			return fmt.Errorf("behavior relationship is incomplete")
		}
		if _, exists := anchorIDs[relationship.From]; !exists {
			return fmt.Errorf("behavior relationship has unknown source")
		}
		if _, exists := anchorIDs[relationship.To]; !exists {
			return fmt.Errorf("behavior relationship has unknown target")
		}
		if grounding.Version >= 2 {
			if !validArchitectureRelationshipKind(relationship.Kind) || relationship.EvidenceKind != "bounded_direct_call" ||
				relationship.WitnessCount < 1 || len(relationship.WitnessIDs) != relationship.WitnessCount ||
				len(relationship.RepresentativeLocations) == 0 || len(relationship.RepresentativeLocations) > 8 {
				return fmt.Errorf("behavior relationship aggregation is incomplete")
			}
		}
	}
	return nil
}

func validArchitectureRelationshipKind(kind string) bool {
	switch kind {
	case "dispatches_to", "loads_or_adapts_config", "registers_extension_family", "starts_lifecycle",
		"exposes_admin_control_plane", "dispatches_http_request", "configures_security_boundary",
		"static_call_supporting_relation":
		return true
	default:
		return false
	}
}

func ensureArchitectureGrounding(data *ReportData) {
	if data.ArchitectureGrounding != nil {
		return
	}
	archetype := componentmap.ArchetypeApplication
	if data.RepositoryGraph == nil || len(data.RepositoryGraph.PackageEdges) == 0 {
		archetype = componentmap.ArchetypeLibraryFramework
	}
	data.ArchitectureGrounding = &ArchitectureGrounding{
		Version: ArchitectureGroundingVersion,
		RepositoryArchetype: ArchitectureArchetype{
			Selected: archetype,
			Evidence: []string{"No persisted behavior-grounding artifact was available."},
		},
		GroundingMode: componentmap.GroundingPackages,
	}
}

func validGroundingLocation(location evidence.Location) bool {
	return location.Line > 0 && location.Column >= 0 && location.Path != "." &&
		fs.ValidPath(location.Path) && !strings.ContainsRune(location.Path, '\\')
}

func validArchitectureArchetype(value componentmap.RepositoryArchetype) bool {
	switch value {
	case componentmap.ArchetypeApplication, componentmap.ArchetypeModularPlatformServer,
		componentmap.ArchetypeLibraryFramework, componentmap.ArchetypeCLITool,
		componentmap.ArchetypeDaemonWorkerSystem, componentmap.ArchetypeMonorepoMixed:
		return true
	default:
		return false
	}
}

func validArchitectureGroundingMode(value componentmap.GroundingMode) bool {
	return value == componentmap.GroundingBehavior || value == componentmap.GroundingMixed ||
		value == componentmap.GroundingPackages
}

func validArchitectureBehaviorAnchorKind(value componentmap.BehaviorAnchorKind) bool {
	switch value {
	case componentmap.AnchorProcessEntry, componentmap.AnchorCommandDispatch, componentmap.AnchorConfigIngress,
		componentmap.AnchorConfigAdapter, componentmap.AnchorConfigApply, componentmap.AnchorRegistryWrite,
		componentmap.AnchorRegistryLookup, componentmap.AnchorLifecycleInterface, componentmap.AnchorLifecycleStart,
		componentmap.AnchorAdminControlPlane, componentmap.AnchorRequestDispatchRoot,
		componentmap.AnchorApplicationData, componentmap.AnchorSecurityBoundary,
		componentmap.AnchorExtensionFamily, componentmap.AnchorUnresolvedFrontier:
		return true
	default:
		return false
	}
}

func sortedArchitectureGroundingAnchors(values []ArchitectureBehaviorAnchor) []ArchitectureBehaviorAnchor {
	result := append([]ArchitectureBehaviorAnchor(nil), values...)
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}
