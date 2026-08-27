package report

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/activityentrypoint"
	"github.com/dvordrova/repomap/internal/activitypath"
	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/coremap"
	"github.com/dvordrova/repomap/internal/cubemap"
	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/dependencydeclaration"
	"github.com/dvordrova/repomap/internal/integrationdependency"
	"github.com/dvordrova/repomap/internal/integrationusage"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/pythondeclareddependencies"
	"github.com/dvordrova/repomap/internal/pythontarget"
	"github.com/dvordrova/repomap/internal/readmetargetscout"
)

const maxReportTargetMetadataBytes = 4 << 20

// snapshotJSON is the complete snapshot shape still consumed by report
// publication. GoFacts contributes material source paths only; the report no
// longer recreates a second repository graph or presentation product from it.
type snapshotJSON struct {
	RepoName       string                 `json:"repo_name"`
	AnalysisTarget *analysistarget.Target `json:"analysis_target,omitempty"`
	GoFacts        *snapshotGoFactsJSON   `json:"go_facts"`
}

type snapshotGoFactsJSON struct {
	Modules  []snapshotModuleJSON  `json:"modules"`
	Packages []snapshotPackageJSON `json:"packages"`
}

type snapshotModuleJSON struct {
	ModuleDir string `json:"module_dir"`
}

type snapshotPackageJSON struct {
	Files []string `json:"files"`
}

type runMetadataJSON struct {
	RepoName string   `json:"repo_name"`
	Warnings []string `json:"warnings"`
}

func ReadRunDir(runDir string) (*ReportData, error) {
	return readRunDir(runDir)
}

// readRunDir restores only the ordinary Program report path.
func readRunDir(runDir string) (*ReportData, error) {
	absDir, err := filepath.Abs(runDir)
	if err != nil {
		return nil, fmt.Errorf("resolve run dir: %w", err)
	}
	data := &ReportData{
		FormatVersion: CurrentFormatVersion,
		ArtifactsDir:  absDir,
	}
	if err := parseSnapshot(filepath.Join(absDir, "snapshot.json"), data); err != nil {
		return nil, err
	}
	if err := parseRunMetadata(filepath.Join(absDir, "metadata.json"), data); err != nil {
		return nil, err
	}
	if err := restoreProgramPortfolio(absDir, data); err != nil {
		return nil, err
	}
	if err := restoreDeclaredDependencyAuthority(absDir, data); err != nil {
		return nil, err
	}
	if err := restoreActivityEntrypointView(absDir, data); err != nil {
		return nil, err
	}
	if err := restoreCubeMapView(absDir, data); err != nil {
		return nil, err
	}
	if err := restoreCoreMapView(absDir, data); err != nil {
		return nil, err
	}
	if err := restoreIntegrationUsageView(absDir, data); err != nil {
		return nil, err
	}
	if err := restoreActivityPathView(absDir, data); err != nil {
		return nil, err
	}
	if err := restoreJSTSViews(absDir, data); err != nil {
		return nil, err
	}
	if err := restoreRuntimePortfolioView(absDir, data); err != nil {
		return nil, err
	}
	if err := restoreTargetOutcomePortfolioView(absDir, data); err != nil {
		return nil, err
	}
	if err := validateProgramSemanticPresentation(
		data.ProgramPortfolio, data.AnalysisTarget, data.CubeMapView, data.CoreMapView,
		data.ActivityEntrypointView, data.IntegrationUsageView, data.ActivityPathView,
		jstsSemanticPresentation{data.JSTSSurfaceCatalogView, data.CrossSurfacePathView},
	); err != nil {
		return nil, err
	}
	if err := collectOpenablePaths(data); err != nil {
		return nil, err
	}
	return data, nil
}

// restoreDeclaredDependencyAuthority binds the local package-manager view to
// the exact persisted Python target catalog and default ProgramIndex. It does
// not infer imports from distribution names. The artifact is mandatory for a
// Python default target and absent from the Go capability.
func restoreDeclaredDependencyAuthority(runDir string, data *ReportData) error {
	if data.defaultProgramIndex == nil {
		return fmt.Errorf("report: declared dependencies default ProgramIndex is unavailable")
	}
	declarationRaw, declarationPresent, err := readBoundedProgramArtifact(
		filepath.Join(runDir, dependencydeclaration.ArtifactFilename),
		dependencydeclaration.MaxArtifactBytes,
		"declared dependencies",
		true,
	)
	if err != nil {
		return err
	}
	isPython := data.defaultProgramIndex.Target.Language == "python"
	if !declarationPresent {
		if isPython {
			return fmt.Errorf("report: Python declared dependency authority is missing")
		}
		return nil
	}
	if !isPython {
		return fmt.Errorf("report: declared dependency authority does not bind a Python default target")
	}
	targetRaw, targetPresent, err := readBoundedProgramArtifact(
		filepath.Join(runDir, pythontarget.ArtifactFilename),
		pythontarget.MaxArtifactBytes,
		"Python target catalog",
		true,
	)
	if err != nil {
		return err
	}
	if !targetPresent {
		return fmt.Errorf("report: declared dependency target authority is missing")
	}
	targets, err := pythontarget.DecodeCatalog(targetRaw)
	if err != nil {
		return fmt.Errorf("report: decode Python target catalog: %w", err)
	}
	declarations, err := pythondeclareddependencies.DecodeTargetAuthority(
		declarationRaw, targets, *data.defaultProgramIndex,
	)
	if err != nil {
		return fmt.Errorf("report: decode declared dependencies: %w", err)
	}
	ownedTargets := targets.Snapshot()
	ownedDeclarations := declarations.Snapshot()
	data.pythonTargetCatalog = &ownedTargets
	data.declaredDependencies = &ownedDeclarations
	for _, source := range declarations.Sources {
		data.materialInputPaths = append(data.materialInputPaths, source.Path)
	}
	return nil
}

// restoreActivityEntrypointView revalidates the complete selected-callable
// artifact against the exact default ProgramIndex. Absence remains explicit
// here so the closed semantic-capability validator can reject a missing Python
// cube while allowing the current Go CubeMap path to own its entrypoints.
func restoreActivityEntrypointView(runDir string, data *ReportData) error {
	encoded, present, err := readBoundedProgramArtifact(
		filepath.Join(runDir, activityentrypoint.ArtifactFilename),
		activityentrypoint.MaxArtifactBytes,
		"activity entrypoints",
		true,
	)
	if err != nil || !present {
		return err
	}
	if data.defaultProgramIndex == nil {
		return fmt.Errorf("report: activity entrypoints default ProgramIndex is unavailable")
	}
	result, err := activityentrypoint.Decode(encoded, *data.defaultProgramIndex)
	if err != nil {
		return fmt.Errorf("report: decode activity entrypoints: %w", err)
	}
	view, err := NewActivityEntrypointView(result, *data.defaultProgramIndex)
	if err != nil {
		return fmt.Errorf("report: project activity entrypoint view: %w", err)
	}
	data.ActivityEntrypointView = view
	return nil
}

// restoreIntegrationUsageView revalidates the complete dependency ->
// integration-dependency -> selected-use artifact chain against the exact
// default ProgramIndex. The three artifacts are one material authority: a
// partial chain is terminal. Declaration authority is required only when the
// language adapter actually persisted that distinct artifact.
func restoreIntegrationUsageView(runDir string, data *ReportData) error {
	catalogRaw, catalogPresent, err := readBoundedProgramArtifact(
		filepath.Join(runDir, dependencies.ArtifactFilename),
		dependencies.MaxArtifactBytes,
		"dependency catalog",
		true,
	)
	if err != nil {
		return err
	}
	selectedRaw, selectedPresent, err := readBoundedProgramArtifact(
		filepath.Join(runDir, integrationdependency.ArtifactFilename),
		integrationdependency.MaxArtifactBytes,
		"integration dependencies",
		true,
	)
	if err != nil {
		return err
	}
	usageRaw, usagePresent, err := readBoundedProgramArtifact(
		filepath.Join(runDir, integrationusage.ArtifactFilename),
		integrationusage.MaxArtifactBytes,
		"integration usage",
		true,
	)
	if err != nil {
		return err
	}
	present := 0
	for _, value := range []bool{catalogPresent, selectedPresent, usagePresent} {
		if value {
			present++
		}
	}
	if present == 0 {
		return nil
	}
	if present != 3 {
		return fmt.Errorf("report: integration usage material authority is incomplete")
	}
	if data.defaultProgramIndex == nil {
		return fmt.Errorf("report: integration usage default ProgramIndex is unavailable")
	}
	catalog, err := dependencies.Decode(catalogRaw)
	if err != nil {
		return fmt.Errorf("report: decode dependency catalog: %w", err)
	}
	selected, err := integrationdependency.Decode(selectedRaw)
	if err != nil {
		return fmt.Errorf("report: decode integration dependencies: %w", err)
	}
	if err := validateSelectedDependenciesForProgram(
		selected, catalog, data.declaredDependencies, *data.defaultProgramIndex,
	); err != nil {
		return fmt.Errorf("report: integration dependencies do not match dependency catalog: %w", err)
	}
	usage, err := integrationusage.Decode(usageRaw)
	if err != nil {
		return fmt.Errorf("report: decode integration usage: %w", err)
	}
	view, err := NewIntegrationUsageView(usage, *data.defaultProgramIndex, selected)
	if err != nil {
		return fmt.Errorf("report: project integration usage view: %w", err)
	}
	data.IntegrationUsageView = view
	return nil
}

// restoreActivityPathView binds the complete deterministic activity-path
// artifact to the same ProgramIndex, ActivityEntrypoint and IntegrationUsage
// authority already used by the surrounding report projections. It does not
// synthesize a route when the artifact is absent or incomplete.
func restoreActivityPathView(runDir string, data *ReportData) error {
	pathRaw, pathPresent, err := readBoundedProgramArtifact(
		filepath.Join(runDir, activitypath.ArtifactFilename),
		activitypath.MaxArtifactBytes,
		"activity paths",
		true,
	)
	if err != nil || !pathPresent {
		return err
	}
	if data.defaultProgramIndex == nil || data.ActivityEntrypointView == nil ||
		data.IntegrationUsageView == nil {
		return fmt.Errorf("report: activity path material authority is incomplete")
	}
	activityRaw, activityPresent, err := readBoundedProgramArtifact(
		filepath.Join(runDir, activityentrypoint.ArtifactFilename),
		activityentrypoint.MaxArtifactBytes,
		"activity entrypoints",
		true,
	)
	if err != nil {
		return err
	}
	catalogRaw, catalogPresent, err := readBoundedProgramArtifact(
		filepath.Join(runDir, dependencies.ArtifactFilename),
		dependencies.MaxArtifactBytes,
		"dependency catalog",
		true,
	)
	if err != nil {
		return err
	}
	selectedRaw, selectedPresent, err := readBoundedProgramArtifact(
		filepath.Join(runDir, integrationdependency.ArtifactFilename),
		integrationdependency.MaxArtifactBytes,
		"integration dependencies",
		true,
	)
	if err != nil {
		return err
	}
	usageRaw, usagePresent, err := readBoundedProgramArtifact(
		filepath.Join(runDir, integrationusage.ArtifactFilename),
		integrationusage.MaxArtifactBytes,
		"integration usage",
		true,
	)
	if err != nil {
		return err
	}
	if !activityPresent || !catalogPresent || !selectedPresent || !usagePresent {
		return fmt.Errorf("report: activity path input authority is incomplete")
	}
	index := *data.defaultProgramIndex
	activities, err := activityentrypoint.Decode(activityRaw, index)
	if err != nil {
		return fmt.Errorf("report: decode activity path activity entrypoints: %w", err)
	}
	catalog, err := dependencies.Decode(catalogRaw)
	if err != nil {
		return fmt.Errorf("report: decode activity path dependency catalog: %w", err)
	}
	selected, err := integrationdependency.Decode(selectedRaw)
	if err != nil {
		return fmt.Errorf("report: decode activity path integration dependencies: %w", err)
	}
	if err := validateSelectedDependenciesForProgram(selected, catalog, data.declaredDependencies, index); err != nil {
		return fmt.Errorf("report: activity path integration dependencies: %w", err)
	}
	usage, err := integrationusage.Decode(usageRaw)
	if err != nil {
		return fmt.Errorf("report: decode activity path integration usage: %w", err)
	}
	result, err := activitypath.Decode(pathRaw, index, activities, selected, usage)
	if err != nil {
		return fmt.Errorf("report: decode activity paths: %w", err)
	}
	view, err := NewActivityPathView(result, index, activities, selected, usage)
	if err != nil {
		return fmt.Errorf("report: project activity path view: %w", err)
	}
	if err := view.ValidateReportJoins(data.ActivityEntrypointView, data.IntegrationUsageView); err != nil {
		return fmt.Errorf("report: join activity path view: %w", err)
	}
	data.ActivityPathView = view
	return nil
}

func validateSelectedDependenciesForProgram(
	selected integrationdependency.Result,
	catalog dependencies.Catalog,
	declarations *dependencydeclaration.Result,
	index programindex.Index,
) error {
	if declarations == nil {
		if selected.Declarations != nil {
			return fmt.Errorf("selected declaration candidates have no declaration authority")
		}
		return selected.ValidateAgainst(catalog)
	}
	return selected.ValidateAgainstDeclarations(catalog, *declarations, index.Target)
}

// restoreCoreMapView projects the complete ProgramIndex-backed CoreMap. The
// artifact is required by the default Python capability and is never treated
// as a substitute for the richer Go CubeMap.
func restoreCoreMapView(runDir string, data *ReportData) error {
	encoded, present, err := readBoundedProgramArtifact(
		filepath.Join(runDir, coremap.ArtifactFilename),
		coremap.MaxArtifactBytes,
		"core map",
		true,
	)
	if err != nil || !present {
		return err
	}
	value, err := coremap.Decode(encoded)
	if err != nil {
		return fmt.Errorf("report: decode core map: %w", err)
	}
	if data.ProgramPortfolio == nil {
		return fmt.Errorf("report: core map requires an exact default program target")
	}
	if data.defaultProgramIndex == nil {
		return fmt.Errorf("report: core map default ProgramIndex is unavailable")
	}
	readmeFiles := map[string]string{}
	readmeRaw, readmePresent, err := readBoundedProgramArtifact(
		filepath.Join(runDir, readmetargetscout.ArtifactFilename),
		maxReadmeFileRoleArtifactBytes,
		"README file-role artifact",
		true,
	)
	if err != nil {
		return err
	}
	if readmePresent {
		readmeFiles, err = decodeReadmeFileRoleAuthority(readmeRaw)
		if err != nil {
			return err
		}
	}
	view, err := NewCoreMapView(value, *data.defaultProgramIndex, readmeFiles)
	if err != nil {
		return fmt.Errorf("report: project core map view: %w", err)
	}
	data.CoreMapView = view
	return nil
}

// restoreProgramPortfolio installs every language-neutral target selected by
// the sealed ProgramIndex artifact set. Its default entry is the only default
// target/view authority used by later report stages.
func restoreProgramPortfolio(runDir string, data *ReportData) error {
	setBytes, _, err := readBoundedProgramArtifact(
		filepath.Join(runDir, programindex.ArtifactSetFilename),
		programindex.MaxArtifactSetBytes,
		"program index set",
		false,
	)
	if err != nil {
		return err
	}
	set, err := programindex.DecodeArtifactSet(setBytes)
	if err != nil {
		return fmt.Errorf("report: decode program index set: %w", err)
	}
	indexes := make([]programindex.Index, 0, len(set.Entries))
	for _, entry := range set.Entries {
		indexBytes, _, readErr := readBoundedProgramArtifact(
			filepath.Join(runDir, entry.Filename),
			programindex.MaxIndexBytes,
			"program index "+entry.TargetID,
			false,
		)
		if readErr != nil {
			return readErr
		}
		index, decodeErr := programindex.Decode(indexBytes)
		if decodeErr != nil {
			return fmt.Errorf("report: decode program index %q: %w", entry.TargetID, decodeErr)
		}
		if index.Target.ID != entry.TargetID || index.SHA256 != entry.IndexSHA256 {
			return fmt.Errorf("report: program index %q does not match its artifact-set binding", entry.TargetID)
		}
		indexes = append(indexes, index)
	}
	portfolio, err := NewProgramPortfolio(set.DefaultTargetID, indexes)
	if err != nil {
		return fmt.Errorf("report: project program portfolio: %w", err)
	}
	data.ProgramPortfolio = portfolio
	for position := range indexes {
		if indexes[position].Target.ID == set.DefaultTargetID {
			value := indexes[position]
			data.defaultProgramIndex = &value
			data.defaultProgramIndexArtifactFilename = set.Entries[position].Filename
			break
		}
	}
	if data.defaultProgramIndex == nil || data.defaultProgramIndexArtifactFilename == "" {
		return fmt.Errorf("report: default ProgramIndex is missing after portfolio projection")
	}
	return nil
}

// restoreCubeMapView projects the complete saved semantic cube result into a
// bounded browser handoff. The artifact is optional for language adapters
// that do not produce this cube yet; a present artifact is never ignored.
func restoreCubeMapView(runDir string, data *ReportData) error {
	encoded, present, err := readBoundedProgramArtifact(
		filepath.Join(runDir, cubemap.ArtifactFilename),
		maxManifestReportBytes,
		"cube map",
		true,
	)
	if err != nil || !present {
		return err
	}
	value, err := cubemap.Decode(encoded)
	if err != nil {
		return fmt.Errorf("report: decode cube map: %w", err)
	}
	if data.ProgramPortfolio == nil {
		return fmt.Errorf("report: cube map requires an exact default program target")
	}
	defaultEntry, err := data.ProgramPortfolio.defaultEntry()
	if err != nil {
		return fmt.Errorf("report: cube map default program target: %w", err)
	}
	view, err := NewCubeMapView(value, defaultEntry.Target, defaultEntry.View.IndexSHA256)
	if err != nil {
		return fmt.Errorf("report: project cube map view: %w", err)
	}
	data.CubeMapView = view
	return nil
}

func readBoundedProgramArtifact(
	artifactPath string,
	maxBytes int,
	label string,
	optional bool,
) ([]byte, bool, error) {
	info, err := os.Lstat(artifactPath)
	if err != nil {
		if optional && os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("report: inspect %s: %w", label, err)
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > int64(maxBytes) {
		return nil, false, fmt.Errorf("report: %s is not a bounded regular file", label)
	}
	encoded, err := os.ReadFile(artifactPath)
	if err != nil {
		return nil, false, fmt.Errorf("report: read %s: %w", label, err)
	}
	if len(encoded) == 0 || len(encoded) > maxBytes {
		return nil, false, fmt.Errorf("report: %s is not bounded", label)
	}
	return encoded, true, nil
}

func collectOpenablePaths(data *ReportData) error {
	if data == nil {
		return fmt.Errorf("report: collect openable paths: report data is missing")
	}
	paths := make(map[string]struct{})
	add := func(sourcePath string) error {
		if err := validateManifestPath(sourcePath); err != nil {
			return fmt.Errorf("report: invalid openable path %q: %w", sourcePath, err)
		}
		paths[sourcePath] = struct{}{}
		return nil
	}
	if data.AnalysisTarget != nil {
		for _, root := range data.AnalysisTarget.Roots {
			if err := add(root.Path); err != nil {
				return err
			}
		}
	}
	addProgram := func(target programindex.Target, view ProgramView) error {
		for _, source := range target.Sources {
			if err := add(source.Path); err != nil {
				return err
			}
		}
		for _, seed := range target.Seeds {
			if seed.Location != nil {
				if err := add(seed.Location.Path); err != nil {
					return err
				}
			}
		}
		for _, seed := range view.Seeds {
			if seed.LaunchLocation != nil {
				if err := add(seed.LaunchLocation.Path); err != nil {
					return err
				}
			}
			if seed.DeclarationLocation != nil {
				if err := add(seed.DeclarationLocation.Path); err != nil {
					return err
				}
			}
		}
		for _, object := range view.Objects {
			if object.Location != nil {
				if err := add(object.Location.Path); err != nil {
					return err
				}
			}
		}
		for _, relation := range view.Relations {
			if relation.Location != nil {
				if err := add(relation.Location.Path); err != nil {
					return err
				}
			}
			for _, witness := range relation.Witnesses {
				if witness.Location != nil {
					if err := add(witness.Location.Path); err != nil {
						return err
					}
				}
			}
		}
		return nil
	}
	if data.ProgramPortfolio != nil {
		for _, entry := range data.ProgramPortfolio.Entries {
			if err := addProgram(entry.Target, entry.View); err != nil {
				return err
			}
		}
	}
	if data.CubeMapView != nil {
		var addCoreBlocks func([]CubeMapViewCoreBlock)
		var cubePathErr error
		addCoreBlocks = func(blocks []CubeMapViewCoreBlock) {
			if cubePathErr != nil {
				return
			}
			for _, block := range blocks {
				for _, file := range block.Files {
					if cubePathErr = add(file.Path); cubePathErr != nil {
						return
					}
				}
				for _, symbol := range block.Symbols {
					if cubePathErr = add(symbol.Symbol.Location.Path); cubePathErr != nil {
						return
					}
				}
				addCoreBlocks(block.Children)
				if cubePathErr != nil {
					return
				}
			}
		}
		addCoreBlocks(data.CubeMapView.BaselineCore)
		addCoreBlocks(data.CubeMapView.RefinedCore)
		if cubePathErr != nil {
			return cubePathErr
		}
		for _, object := range data.CubeMapView.CoreObjects {
			if err := add(object.Location.Path); err != nil {
				return err
			}
		}
		for _, surface := range data.CubeMapView.ActivitySurfaces {
			if err := add(surface.Registration.Path); err != nil {
				return err
			}
			for _, value := range []*CubeMapViewSurfaceValue{
				surface.Identity, surface.Method, surface.Path, surface.Handler,
			} {
				if value != nil {
					if err := add(value.Location.Path); err != nil {
						return err
					}
				}
			}
		}
		for _, entrypoint := range data.CubeMapView.Entrypoints {
			if err := add(entrypoint.Location.Path); err != nil {
				return err
			}
		}
		// Dependency importer repository paths identify package directories, not
		// captured regular source files. Exact integration symbols and callsites
		// below provide the file-level source actions for those packages.
		for _, integration := range data.CubeMapView.IntegrationSymbols {
			if err := add(integration.Symbol.Location.Path); err != nil {
				return err
			}
			for _, operation := range integration.Operations {
				for _, callsite := range operation.Callsites {
					if err := add(callsite.Path); err != nil {
						return err
					}
				}
			}
		}
		for _, reversePath := range data.CubeMapView.ReversePaths {
			for _, node := range reversePath.Nodes {
				if err := add(node.Location.Path); err != nil {
					return err
				}
			}
		}
	}
	if data.CoreMapView != nil {
		var addCoreMapBlocks func([]CoreMapViewBlock)
		var corePathErr error
		addCoreMapBlocks = func(blocks []CoreMapViewBlock) {
			if corePathErr != nil {
				return
			}
			for _, block := range blocks {
				for _, file := range block.Files {
					if corePathErr = add(file.Path); corePathErr != nil {
						return
					}
				}
				for _, symbol := range block.RepresentativeSymbols {
					if corePathErr = add(symbol.Symbol.Location.Path); corePathErr != nil {
						return
					}
				}
				addCoreMapBlocks(block.Children)
				if corePathErr != nil {
					return
				}
			}
		}
		addCoreMapBlocks(data.CoreMapView.BaselineCore)
		addCoreMapBlocks(data.CoreMapView.RefinedCore)
		if corePathErr != nil {
			return corePathErr
		}
	}
	if data.IntegrationUsageView != nil {
		for _, candidate := range data.IntegrationUsageView.DeclaredCandidates {
			for _, sourcePath := range candidate.SourcePaths {
				if err := add(sourcePath); err != nil {
					return err
				}
			}
		}
		for _, dependency := range data.IntegrationUsageView.Dependencies {
			for _, use := range dependency.Uses {
				if err := add(use.CallerLocation.Path); err != nil {
					return err
				}
				if err := add(use.Callsite.Path); err != nil {
					return err
				}
			}
		}
	}
	if data.ActivityEntrypointView != nil {
		for _, entrypoint := range data.ActivityEntrypointView.Entrypoints {
			if err := add(entrypoint.Location.Path); err != nil {
				return err
			}
		}
	}
	if data.ActivityPathView != nil {
		for _, object := range data.ActivityPathView.Objects {
			if object.Location != nil {
				if err := add(object.Location.Path); err != nil {
					return err
				}
			}
		}
		for _, route := range data.ActivityPathView.Routes {
			for _, step := range route.Steps {
				if step.Location != nil {
					if err := add(step.Location.Path); err != nil {
						return err
					}
				}
			}
		}
	}
	if data.JSTSSurfaceCatalogView != nil {
		for _, fact := range data.JSTSSurfaceCatalogView.Facts {
			if fact.Location != nil {
				if err := add(fact.Location.Path); err != nil {
					return err
				}
			}
		}
		for _, surface := range data.JSTSSurfaceCatalogView.Surfaces {
			if err := add(surface.Location.Path); err != nil {
				return err
			}
		}
	}
	if data.CrossSurfacePathView != nil {
		for _, fact := range data.CrossSurfacePathView.Facts {
			if fact.Location != nil {
				if err := add(fact.Location.Path); err != nil {
					return err
				}
			}
		}
		for _, path := range data.CrossSurfacePathView.Paths {
			for _, step := range path.Steps {
				if err := add(step.Location.Path); err != nil {
					return err
				}
			}
		}
	}
	if data.RuntimePortfolio != nil {
		for _, role := range data.RuntimePortfolio.Roles {
			for _, evidence := range role.Evidence {
				if err := add(evidence.Location.Path); err != nil {
					return err
				}
			}
		}
	}
	data.OpenablePaths = data.OpenablePaths[:0]
	for sourcePath := range paths {
		data.OpenablePaths = append(data.OpenablePaths, sourcePath)
	}
	sort.Strings(data.OpenablePaths)
	return nil
}

func parseRunMetadata(metadataPath string, data *ReportData) error {
	if data == nil {
		return fmt.Errorf("report: metadata requires report data")
	}
	encoded, _, err := readBoundedProgramArtifact(
		metadataPath,
		maxReportTargetMetadataBytes,
		"metadata",
		false,
	)
	if err != nil {
		return err
	}
	var metadata runMetadataJSON
	if err := json.Unmarshal(encoded, &metadata); err != nil {
		return fmt.Errorf("report: metadata unmarshal: %w", err)
	}
	if metadata.RepoName == "" || strings.TrimSpace(metadata.RepoName) != metadata.RepoName {
		return fmt.Errorf("report: metadata repository name must be exact and non-empty")
	}
	if data.RepoName == "" {
		return fmt.Errorf("report: metadata cannot replace a missing snapshot repository name")
	}
	if metadata.RepoName != data.RepoName {
		return fmt.Errorf("report: metadata repository name does not match snapshot")
	}
	data.Warnings = append(data.Warnings, metadata.Warnings...)
	return nil
}

func parseSnapshot(snapshotPath string, data *ReportData) error {
	if data == nil {
		return fmt.Errorf("report: snapshot requires report data")
	}
	encoded, _, err := readBoundedProgramArtifact(
		snapshotPath,
		maxManifestSnapshotBytes,
		"snapshot",
		false,
	)
	if err != nil {
		return err
	}
	var snapshot snapshotJSON
	if err := json.Unmarshal(encoded, &snapshot); err != nil {
		return fmt.Errorf("report: snapshot unmarshal: %w", err)
	}
	if snapshot.RepoName == "" || strings.TrimSpace(snapshot.RepoName) != snapshot.RepoName {
		return fmt.Errorf("report: snapshot repository name must be exact and non-empty")
	}
	var analysisTarget *analysistarget.Target
	if snapshot.AnalysisTarget != nil {
		if err := snapshot.AnalysisTarget.Validate(); err != nil {
			return fmt.Errorf("report: snapshot analysis target: %w", err)
		}
		target := snapshot.AnalysisTarget.Snapshot()
		analysisTarget = &target
	}
	materialPaths, err := snapshotMaterialInputPaths(snapshot.GoFacts)
	if err != nil {
		return fmt.Errorf("report: snapshot material inputs: %w", err)
	}
	data.AnalysisTarget = analysisTarget
	data.RepoName = snapshot.RepoName
	data.materialInputPaths = materialPaths
	return nil
}

func snapshotMaterialInputPaths(facts *snapshotGoFactsJSON) ([]string, error) {
	if facts == nil {
		return nil, nil
	}
	paths := make(map[string]struct{})
	add := func(value string) error {
		if err := validateManifestPath(value); err != nil {
			return err
		}
		paths[value] = struct{}{}
		return nil
	}
	for _, pkg := range facts.Packages {
		for _, sourcePath := range pkg.Files {
			if err := add(sourcePath); err != nil {
				return nil, fmt.Errorf("package file %q: %w", sourcePath, err)
			}
		}
	}
	for _, module := range facts.Modules {
		moduleDir := module.ModuleDir
		if moduleDir == "" || moduleDir == "." {
			moduleDir = ""
		} else if err := validateManifestPath(moduleDir); err != nil {
			return nil, fmt.Errorf("module directory %q: %w", module.ModuleDir, err)
		}
		for _, filename := range []string{"go.mod", "go.sum"} {
			moduleFile := filename
			if moduleDir != "" {
				moduleFile = path.Join(moduleDir, filename)
			}
			paths[moduleFile] = struct{}{}
		}
	}
	result := make([]string, 0, len(paths))
	for sourcePath := range paths {
		result = append(result, sourcePath)
	}
	sort.Strings(result)
	return result, nil
}
