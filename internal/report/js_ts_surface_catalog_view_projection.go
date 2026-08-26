package report

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/jstsproject"
	"github.com/dvordrova/repomap/internal/programindex"
)

const (
	JSTSSurfaceCatalogViewVersion  = 2
	MaxJSTSSurfaceCatalogViewBytes = 16 << 20
)

type JSTSSurfaceDisposition string

const (
	JSTSSurfaceProduct    JSTSSurfaceDisposition = "product_surface"
	JSTSSurfaceSupporting JSTSSurfaceDisposition = "supporting_code"
	JSTSSurfaceTool       JSTSSurfaceDisposition = "tool"
	JSTSSurfaceUnknown    JSTSSurfaceDisposition = "unknown"
)

// JSTSSurfaceCatalogView is the exact repository overview for one JavaScript
// or TypeScript project. Surface identity and evidence remain producer-owned;
// the report only assigns the closed presentation disposition implied by the
// producer's closed SurfaceKind.
type JSTSSurfaceCatalogView struct {
	Version            int                         `json:"version"`
	ProgramTargetID    string                      `json:"program_target_id"`
	ProgramIndexSHA256 string                      `json:"program_index_sha256"`
	JSTSProjectSHA256  string                      `json:"js_ts_project_sha256,omitempty"`
	Project            JSTSSurfaceProjectView      `json:"project"`
	Facts              []JSTSFactView              `json:"facts"`
	Surfaces           []JSTSSurfaceCatalogSurface `json:"surfaces"`
}

type JSTSSurfaceProjectView struct {
	Name             string `json:"name"`
	ManifestPath     string `json:"manifest_path"`
	ConfigPath       string `json:"config_path,omitempty"`
	ModuleResolution string `json:"module_resolution"`
}

// JSTSFactView is one exact producer identity cited by a surface or product
// path. Category is report-owned and closed; Kind and Label retain the exact
// producer fact without asking the browser to recover it from an ID.
type JSTSFactView struct {
	Ref      string                 `json:"ref"`
	Category string                 `json:"category"`
	Kind     string                 `json:"kind"`
	Label    string                 `json:"label"`
	Location *programindex.Location `json:"location,omitempty"`
}

type JSTSSurfaceCatalogSurface struct {
	SurfaceID    string                  `json:"surface_id"`
	Kind         jstsproject.SurfaceKind `json:"kind"`
	Role         jstsproject.SurfaceRole `json:"role"`
	Disposition  JSTSSurfaceDisposition  `json:"disposition"`
	Name         string                  `json:"name"`
	EntryRefs    []string                `json:"entry_refs"`
	EvidenceRefs []string                `json:"evidence_refs"`
	Location     programindex.Location   `json:"location"`
}

func NewJSTSSurfaceCatalogView(
	result jstsproject.Result,
	index programindex.Index,
) (*JSTSSurfaceCatalogView, error) {
	if err := result.Validate(); err != nil {
		return nil, fmt.Errorf("JavaScript/TypeScript surface catalog view: producer authority: %w", err)
	}
	if err := validateJSTSProgramIndexBinding(result, index); err != nil {
		return nil, fmt.Errorf("JavaScript/TypeScript surface catalog view: %w", err)
	}
	directory, err := jstsFactDirectory(result)
	if err != nil {
		return nil, fmt.Errorf("JavaScript/TypeScript surface catalog view: fact directory: %w", err)
	}
	view := &JSTSSurfaceCatalogView{
		Version:            JSTSSurfaceCatalogViewVersion,
		ProgramTargetID:    index.Target.ID,
		ProgramIndexSHA256: index.SHA256,
		JSTSProjectSHA256:  result.SHA256,
		Project: JSTSSurfaceProjectView{
			Name: result.Project.Name, ManifestPath: result.Project.ManifestPath,
			ConfigPath: result.Project.ConfigPath, ModuleResolution: result.Project.ModuleResolution,
		},
		Facts: []JSTSFactView{}, Surfaces: []JSTSSurfaceCatalogSurface{},
	}
	referenced := make(map[string]struct{})
	for _, surface := range result.Surfaces {
		for _, ref := range surface.EntryRefs {
			referenced[ref] = struct{}{}
		}
		for _, ref := range surface.EvidenceRefs {
			referenced[ref] = struct{}{}
		}
		view.Surfaces = append(view.Surfaces, JSTSSurfaceCatalogSurface{
			SurfaceID: surface.Ref, Kind: surface.Kind, Role: surface.Role,
			Disposition: jstsSurfaceDisposition(surface.Role), Name: surface.Name,
			EntryRefs:    append([]string{}, surface.EntryRefs...),
			EvidenceRefs: append([]string{}, surface.EvidenceRefs...),
			Location:     jstsLocation(surface.Location),
		})
	}
	for ref := range referenced {
		fact, ok := directory[ref]
		if !ok {
			return nil, fmt.Errorf("surface cites unknown entry or evidence ref %q", ref)
		}
		view.Facts = append(view.Facts, cloneJSTSFact(fact))
	}
	sort.Slice(view.Facts, func(i, j int) bool { return view.Facts[i].Ref < view.Facts[j].Ref })
	if err := view.Validate(); err != nil {
		return nil, fmt.Errorf("JavaScript/TypeScript surface catalog view: invalid projection: %w", err)
	}
	return view, nil
}

func restoreJSTSViews(runDir string, data *ReportData) error {
	encoded, present, err := readBoundedProgramArtifact(
		filepath.Join(runDir, jstsproject.ArtifactFilename),
		jstsproject.MaxArtifactBytes,
		"JavaScript/TypeScript project",
		true,
	)
	if err != nil || !present {
		return err
	}
	if data.defaultProgramIndex == nil {
		return fmt.Errorf("report: JavaScript/TypeScript project default ProgramIndex is unavailable")
	}
	result, err := jstsproject.Decode(encoded)
	if err != nil {
		return fmt.Errorf("report: decode JavaScript/TypeScript project: %w", err)
	}
	surfaces, err := NewJSTSSurfaceCatalogView(result, *data.defaultProgramIndex)
	if err != nil {
		return fmt.Errorf("report: project JavaScript/TypeScript surface catalog: %w", err)
	}
	paths, err := NewCrossSurfacePathView(result, *data.defaultProgramIndex)
	if err != nil {
		return fmt.Errorf("report: project cross-surface paths: %w", err)
	}
	if err := paths.ValidateSurfaceJoins(surfaces); err != nil {
		return fmt.Errorf("report: join JavaScript/TypeScript surface paths: %w", err)
	}
	data.JSTSSurfaceCatalogView = surfaces
	data.CrossSurfacePathView = paths
	return nil
}

func (view JSTSSurfaceCatalogView) ValidateAgainst(result jstsproject.Result, index programindex.Index) error {
	if err := view.Validate(); err != nil {
		return err
	}
	expected, err := NewJSTSSurfaceCatalogView(result, index)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(view, *expected) {
		return fmt.Errorf("JavaScript/TypeScript surface catalog view: projection does not match exact producer authority")
	}
	return nil
}

func (view JSTSSurfaceCatalogView) Validate() error {
	if view.Version != JSTSSurfaceCatalogViewVersion ||
		!validProgramViewText(view.ProgramTargetID) ||
		!validCubeMapViewSHA256(view.ProgramIndexSHA256) ||
		!validCubeMapViewSHA256(view.JSTSProjectSHA256) ||
		!validProgramViewText(view.Project.Name) ||
		validateManifestPath(view.Project.ManifestPath) != nil ||
		(view.Project.ConfigPath != "" && validateManifestPath(view.Project.ConfigPath) != nil) ||
		!validProgramViewText(view.Project.ModuleResolution) || view.Facts == nil || view.Surfaces == nil {
		return fmt.Errorf("JavaScript/TypeScript surface catalog view: invalid identity or collection shape")
	}
	facts, err := validateJSTSFactViews(view.Facts)
	if err != nil {
		return fmt.Errorf("JavaScript/TypeScript surface catalog view: %w", err)
	}
	referenced := make(map[string]struct{})
	previousID := ""
	for position, surface := range view.Surfaces {
		if !validProgramViewText(surface.SurfaceID) || !validProgramViewText(surface.Name) ||
			!validJSTSSurfaceKind(surface.Kind) ||
			jstsSurfaceDisposition(surface.Role) != surface.Disposition ||
			!validJSTSSourceLocation(&surface.Location, false) ||
			surface.EntryRefs == nil || surface.EvidenceRefs == nil {
			return fmt.Errorf("JavaScript/TypeScript surface catalog view: surface %d is invalid", position)
		}
		if previousID != "" && previousID >= surface.SurfaceID {
			return fmt.Errorf("JavaScript/TypeScript surface catalog view: surfaces are not canonical")
		}
		previousID = surface.SurfaceID
		for _, refs := range [][]string{surface.EntryRefs, surface.EvidenceRefs} {
			if err := validateCanonicalJSTSRefs(refs); err != nil {
				return fmt.Errorf("JavaScript/TypeScript surface catalog view: surface references: %w", err)
			}
			for _, ref := range refs {
				if _, ok := facts[ref]; !ok {
					return fmt.Errorf("JavaScript/TypeScript surface catalog view: surface cites unknown fact %q", ref)
				}
				referenced[ref] = struct{}{}
			}
		}
	}
	if len(referenced) != len(view.Facts) {
		return fmt.Errorf("JavaScript/TypeScript surface catalog view: fact dictionary contains dead entries")
	}
	encoded, err := json.Marshal(view)
	if err != nil {
		return fmt.Errorf("JavaScript/TypeScript surface catalog view: encode bound check: %w", err)
	}
	if len(encoded) > MaxJSTSSurfaceCatalogViewBytes {
		return fmt.Errorf("JavaScript/TypeScript surface catalog view: exact projection requires %d bytes; limit is %d", len(encoded), MaxJSTSSurfaceCatalogViewBytes)
	}
	return nil
}

func validJSTSSurfaceKind(kind jstsproject.SurfaceKind) bool {
	switch kind {
	case jstsproject.SurfaceBrowser, jstsproject.SurfaceServer, jstsproject.SurfaceCLI, jstsproject.SurfaceShared,
		jstsproject.SurfaceTool, jstsproject.SurfaceUnknown:
		return true
	default:
		return false
	}
}

func jstsSurfaceDisposition(role jstsproject.SurfaceRole) JSTSSurfaceDisposition {
	switch role {
	case jstsproject.SurfaceProduct:
		return JSTSSurfaceProduct
	case jstsproject.SurfaceSupporting:
		return JSTSSurfaceSupporting
	case jstsproject.SurfaceScript:
		return JSTSSurfaceTool
	case jstsproject.SurfaceUnclassified:
		return JSTSSurfaceUnknown
	default:
		return ""
	}
}

func validateJSTSProgramIndexBinding(result jstsproject.Result, index programindex.Index) error {
	if err := jstsproject.ValidateProgramIndex(result, index); err != nil {
		return fmt.Errorf("project artifact does not bind the exact ProgramTarget and ProgramIndex: %w", err)
	}
	return nil
}

func jstsFactDirectory(result jstsproject.Result) (map[string]JSTSFactView, error) {
	resultByRef := make(map[string]JSTSFactView)
	add := func(fact JSTSFactView) error {
		if !validProgramViewText(fact.Ref) {
			return fmt.Errorf("invalid fact ref")
		}
		if _, duplicate := resultByRef[fact.Ref]; duplicate {
			return fmt.Errorf("ambiguous duplicate fact ref %q", fact.Ref)
		}
		resultByRef[fact.Ref] = fact
		return nil
	}
	for _, file := range result.Files {
		location := &programindex.Location{Path: file.Path}
		if err := add(JSTSFactView{Ref: file.FileRef, Category: "file", Kind: file.Language, Label: file.Path, Location: location}); err != nil {
			return nil, err
		}
	}
	for _, value := range result.Declarations {
		if err := add(jstsFact(value.Ref, "declaration", value.Kind, value.QualifiedName, value.Location)); err != nil {
			return nil, err
		}
	}
	for _, value := range result.Imports {
		if err := add(jstsFact(value.Ref, "import", value.Kind, value.Specifier, value.Location)); err != nil {
			return nil, err
		}
	}
	for _, value := range result.Exports {
		if err := add(jstsFact(value.Ref, "export", value.Kind, value.Name, value.Location)); err != nil {
			return nil, err
		}
	}
	for _, value := range result.Calls {
		if err := add(jstsFact(value.Ref, "call", "call", value.Expression, value.Location)); err != nil {
			return nil, err
		}
	}
	for _, value := range result.Surfaces {
		if err := add(jstsFact(value.Ref, "surface", string(value.Kind), value.Name, value.Location)); err != nil {
			return nil, err
		}
	}
	for _, value := range result.Routes {
		label := strings.TrimSpace(value.Method + " " + value.Path)
		if err := add(jstsFact(value.Ref, "route", string(value.Kind), label, value.Location)); err != nil {
			return nil, err
		}
	}
	for _, value := range result.HTTPUses {
		if err := add(jstsFact(value.Ref, "http_use", value.Kind, value.Method+" "+value.Path, value.Location)); err != nil {
			return nil, err
		}
	}
	for _, value := range result.Contracts {
		if err := add(jstsFact(value.Ref, "contract", value.Kind, value.Name, value.Location)); err != nil {
			return nil, err
		}
	}
	for _, value := range result.Resources {
		if err := add(jstsFact(value.Ref, "resource", value.Kind, value.Name, value.Location)); err != nil {
			return nil, err
		}
	}
	return resultByRef, nil
}

func jstsFact(ref, category, kind, label string, location jstsproject.Location) JSTSFactView {
	projected := jstsLocation(location)
	return JSTSFactView{Ref: ref, Category: category, Kind: kind, Label: label, Location: &projected}
}

func jstsLocation(value jstsproject.Location) programindex.Location {
	return programindex.Location{Path: value.Path, Line: value.Line, Column: value.Column}
}

func cloneJSTSFact(value JSTSFactView) JSTSFactView {
	result := value
	if value.Location != nil {
		location := *value.Location
		result.Location = &location
	}
	return result
}

func validateJSTSFactViews(values []JSTSFactView) (map[string]JSTSFactView, error) {
	result := make(map[string]JSTSFactView, len(values))
	previous := ""
	for position, fact := range values {
		if !validProgramViewText(fact.Ref) || !validJSTSFactCategory(fact.Category) ||
			!validProgramViewText(fact.Kind) || !validProgramViewText(fact.Label) ||
			!validJSTSSourceLocation(fact.Location, fact.Category == "file") {
			return nil, fmt.Errorf("fact %d is invalid", position)
		}
		if previous != "" && previous >= fact.Ref {
			return nil, fmt.Errorf("facts are not canonical")
		}
		previous = fact.Ref
		result[fact.Ref] = fact
	}
	return result, nil
}

func validJSTSFactCategory(value string) bool {
	switch value {
	case "file", "declaration", "import", "export", "call", "surface", "route", "http_use", "contract", "resource":
		return true
	default:
		return false
	}
}

func validJSTSSourceLocation(value *programindex.Location, fileOnly bool) bool {
	if value == nil || validateManifestPath(value.Path) != nil || value.Line < 0 || value.Column < 0 {
		return false
	}
	if fileOnly {
		return value.Line == 0 && value.Column == 0
	}
	return value.Line > 0 && value.Column > 0
}

func validateCanonicalJSTSRefs(values []string) error {
	previous := ""
	for _, value := range values {
		if !validProgramViewText(value) || previous != "" && previous >= value {
			return fmt.Errorf("references are not canonical")
		}
		previous = value
	}
	return nil
}
