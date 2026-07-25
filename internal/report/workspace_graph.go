package report

import (
	"encoding/json"
	"fmt"

	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/workspacegraph"
	"github.com/dvordrova/repomap/internal/workspacesnapshot"
)

const (
	maxReportGraphFactModules      = 600
	maxReportGraphFactPackages     = 600
	maxReportGraphFilesPerPackage  = 4096
	maxReportGraphAggregateFiles   = 20_000
	maxReportGraphFactEdges        = 1000
	maxReportGraphProjectedModules = 2 * maxReportGraphFactModules
)

type snapshotExactGoFacts struct {
	Modules       []snapshotExactModuleFact  `json:"modules"`
	Packages      []snapshotExactPackageFact `json:"packages"`
	InternalEdges []gofacts.Edge             `json:"internal_edges"`
}

type snapshotExactModuleFact struct {
	ID         string `json:"id"`
	ModulePath string `json:"module_path"`
	ModuleDir  string `json:"module_dir"`
	GoMod      string `json:"go_mod,omitempty"`
	Main       bool   `json:"main"`
}

type snapshotExactPackageFact struct {
	CanonicalPath     string   `json:"canonical_package_path"`
	Name              string   `json:"name"`
	ModuleID          string   `json:"owning_module_id"`
	ModulePath        string   `json:"module_path"`
	PackageDir        string   `json:"package_directory"`
	ModuleRelativeDir string   `json:"module_relative_path"`
	Files             []string `json:"files,omitempty"`
}

// attachAuthorizedWorkspacePackageGraph replaces only the existing
// RepositoryGraph projection. Every failure deliberately retains the complete
// legacy graph without adding a new report warning or partial result.
func attachAuthorizedWorkspacePackageGraph(data *ReportData, authority *RunAuthority) {
	if data == nil || authority == nil || data.RepositoryGraph == nil ||
		len(data.repositoryGoFactsJSON) == 0 {
		return
	}
	if err := authority.validate(); err != nil {
		return
	}
	facts, err := decodeSnapshotExactGoFacts(data.repositoryGoFactsJSON)
	if err != nil {
		return
	}
	snapshot, err := workspacesnapshot.New(workspacesnapshot.Input{
		AnalysisRoot:   authority.analysisRoot,
		Repository:     authority.repository,
		CapturedInputs: authority.inputs,
		AllowedPaths:   data.OpenablePaths,
	})
	if err != nil {
		return
	}
	graph, err := workspacegraph.New(workspacegraph.Input{
		Snapshot: snapshot,
		GoFacts:  facts,
	})
	if err != nil {
		return
	}
	_ = attachWorkspacePackageGraph(data, facts, graph)
}

func decodeSnapshotExactGoFacts(raw json.RawMessage) (gofacts.Facts, error) {
	var saved snapshotExactGoFacts
	if err := json.Unmarshal(raw, &saved); err != nil {
		return gofacts.Facts{}, fmt.Errorf("workspace graph: saved Go facts are unavailable")
	}
	if len(saved.Modules) > maxReportGraphFactModules ||
		len(saved.Packages) > maxReportGraphFactPackages ||
		len(saved.InternalEdges) > maxReportGraphFactEdges {
		return gofacts.Facts{}, fmt.Errorf("workspace graph: saved Go facts exceed bounds")
	}
	totalFiles := 0
	for _, pkg := range saved.Packages {
		if len(pkg.Files) > maxReportGraphFilesPerPackage ||
			len(pkg.Files) > maxReportGraphAggregateFiles-totalFiles {
			return gofacts.Facts{}, fmt.Errorf("workspace graph: saved Go facts exceed bounds")
		}
		totalFiles += len(pkg.Files)
	}

	facts := gofacts.Facts{
		Modules:       make([]gofacts.ModuleFact, len(saved.Modules)),
		Packages:      make([]gofacts.PackageFact, len(saved.Packages)),
		InternalEdges: append([]gofacts.Edge(nil), saved.InternalEdges...),
	}
	for index, module := range saved.Modules {
		facts.Modules[index] = gofacts.ModuleFact{
			ID: module.ID, ModulePath: module.ModulePath, ModuleDir: module.ModuleDir,
			GoMod: module.GoMod, Main: module.Main,
		}
	}
	for index, pkg := range saved.Packages {
		facts.Packages[index] = gofacts.PackageFact{
			CanonicalPath: pkg.CanonicalPath, Name: pkg.Name,
			ModuleID: pkg.ModuleID, ModulePath: pkg.ModulePath,
			PackageDir: pkg.PackageDir, ModuleRelativeDir: pkg.ModuleRelativeDir,
			Files: pkg.Files,
		}
	}
	return facts, nil
}

func attachWorkspacePackageGraph(
	data *ReportData,
	facts gofacts.Facts,
	graph workspacegraph.Graph,
) error {
	if data == nil {
		return fmt.Errorf("workspace graph: report data is required")
	}
	projected, err := projectWorkspacePackageGraph(data.RepositoryGraph, facts, graph)
	if err != nil {
		return err
	}
	data.RepositoryGraph = projected
	return nil
}

func projectWorkspacePackageGraph(
	legacy *RepositoryGraph,
	facts gofacts.Facts,
	graph workspacegraph.Graph,
) (*RepositoryGraph, error) {
	if legacy == nil {
		if len(facts.Modules) == 0 && len(facts.Packages) == 0 {
			return nil, nil
		}
		return nil, fmt.Errorf("workspace graph: legacy projection is unavailable")
	}
	if len(legacy.Modules) < len(facts.Modules) ||
		len(legacy.Modules) > maxReportGraphProjectedModules ||
		len(legacy.Packages) != len(facts.Packages) ||
		len(legacy.Packages) > maxReportGraphFactPackages ||
		len(legacy.PackageEdges) > maxReportGraphFactEdges {
		return nil, fmt.Errorf("workspace graph: legacy projection shape differs")
	}
	totalFiles := 0
	for _, pkg := range legacy.Packages {
		if len(pkg.Files) > maxReportGraphFilesPerPackage ||
			len(pkg.Files) > maxReportGraphAggregateFiles-totalFiles {
			return nil, fmt.Errorf("workspace graph: legacy projection shape differs")
		}
		totalFiles += len(pkg.Files)
	}

	projected := cloneRepositoryGraph(legacy)
	for index, fact := range facts.Modules {
		module, ok := graph.Module(fact.ID, fact.ModulePath, fact.ModuleDir)
		if !ok {
			return nil, fmt.Errorf("workspace graph: module projection %d is unavailable", index)
		}
		legacyModule := projected.Modules[index]
		if legacyModule.ID != fact.ID ||
			legacyModule.Path != fact.ModulePath ||
			legacyModule.Dir != reportModuleDir(fact.ModuleDir) {
			return nil, fmt.Errorf("workspace graph: module projection %d differs", index)
		}
		projected.Modules[index] = ModuleInfo{
			ID:          module.ID,
			Path:        module.Path,
			Dir:         reportModuleDir(module.Dir),
			DisplayName: legacyModule.DisplayName,
		}
	}

	for index, fact := range facts.Packages {
		legacyPackage := projected.Packages[index]
		if !reportPackageMatchesFact(legacyPackage, fact) {
			return nil, fmt.Errorf("workspace graph: package projection %d differs", index)
		}
		pkg, ok := graph.Package(fact.CanonicalPath)
		if !ok || !workspacePackageMatchesFact(pkg, fact) {
			return nil, fmt.Errorf("workspace graph: package projection %d is unavailable", index)
		}
		files := make([]string, len(pkg.Files))
		for fileIndex, file := range pkg.Files {
			files[fileIndex] = file.Path
		}
		if len(files) == 0 && legacyPackage.Files == nil {
			files = nil
		}
		projected.Packages[index] = PackageInfo{
			CanonicalPath:     pkg.CanonicalPath,
			Name:              pkg.Name,
			ModuleID:          pkg.ModuleID,
			ModulePath:        pkg.ModulePath,
			Dir:               pkg.Dir,
			ModuleRelativeDir: pkg.ModuleRelativeDir,
			DisplayPath:       legacyPackage.DisplayPath,
			Locality:          legacyPackage.Locality,
			Files:             files,
		}
	}

	for index, legacyEdge := range projected.PackageEdges {
		edge, ok := graph.Edge(legacyEdge.From, legacyEdge.To)
		if !ok {
			return nil, fmt.Errorf("workspace graph: edge projection %d is unavailable", index)
		}
		projected.PackageEdges[index] = EdgeInfo{
			From: edge.FromPackage,
			To:   edge.ToPackage,
		}
	}
	return projected, nil
}

func cloneRepositoryGraph(graph *RepositoryGraph) *RepositoryGraph {
	if graph == nil {
		return nil
	}
	cloned := &RepositoryGraph{Version: graph.Version}
	if graph.Modules != nil {
		cloned.Modules = append([]ModuleInfo(nil), graph.Modules...)
	}
	if graph.Packages != nil {
		cloned.Packages = make([]PackageInfo, len(graph.Packages))
		for index, pkg := range graph.Packages {
			cloned.Packages[index] = pkg
			if pkg.Files != nil {
				cloned.Packages[index].Files = append([]string(nil), pkg.Files...)
			}
		}
	}
	if graph.PackageEdges != nil {
		cloned.PackageEdges = append([]EdgeInfo(nil), graph.PackageEdges...)
	}
	return cloned
}

func reportModuleDir(dir string) string {
	if dir == "." {
		return ""
	}
	return dir
}

func reportPackageMatchesFact(pkg PackageInfo, fact gofacts.PackageFact) bool {
	return pkg.CanonicalPath == fact.CanonicalPath &&
		pkg.Name == fact.Name &&
		pkg.ModuleID == fact.ModuleID &&
		pkg.ModulePath == fact.ModulePath &&
		pkg.Dir == fact.PackageDir &&
		pkg.ModuleRelativeDir == fact.ModuleRelativeDir &&
		stringSlicesEqual(pkg.Files, fact.Files)
}

func workspacePackageMatchesFact(pkg workspacegraph.Package, fact gofacts.PackageFact) bool {
	if pkg.CanonicalPath != fact.CanonicalPath ||
		pkg.Name != fact.Name ||
		pkg.ModuleID != fact.ModuleID ||
		pkg.ModulePath != fact.ModulePath ||
		pkg.Dir != fact.PackageDir ||
		pkg.ModuleRelativeDir != fact.ModuleRelativeDir ||
		len(pkg.Files) != len(fact.Files) {
		return false
	}
	for index, file := range pkg.Files {
		if file.Path != fact.Files[index] {
			return false
		}
	}
	return true
}

func stringSlicesEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
