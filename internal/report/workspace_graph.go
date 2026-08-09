package report

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/workspacegraph"
	"github.com/dvordrova/repomap/internal/workspacepackageselection"
	"github.com/dvordrova/repomap/internal/workspacesnapshot"
)

// ExactWorkspaceGraphUnavailableError means a Go-backed local package graph
// could not be materialized completely from the captured producer facts. It
// never authorizes a partial Architecture model request.
type ExactWorkspaceGraphUnavailableError struct{}

func (*ExactWorkspaceGraphUnavailableError) Error() string {
	return "exact workspace graph is unavailable"
}

// IsExactWorkspaceGraphUnavailable reports the closed, provider-free local
// precondition failure used by Architecture synthesis.
func IsExactWorkspaceGraphUnavailable(err error) bool {
	var target *ExactWorkspaceGraphUnavailableError
	return errors.As(err, &target)
}

const (
	maxReportGraphFactModules      = 600
	maxReportGraphFactPackages     = 4096
	maxReportGraphFilesPerPackage  = 4096
	maxReportGraphAggregateFiles   = 20_000
	maxReportGraphFactEdges        = workspacegraph.MaxExactEdges
	maxReportGraphScalarBytes      = 4096
	maxReportGraphAggregateScalars = 4 * 1024 * 1024
	maxReportGraphProjectedModules = 2 * maxReportGraphFactModules

	workspaceGraphUnavailableWarning = "workspace graph unavailable: exact local Go package relations were not attached"
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
// RepositoryGraph projection. Every failure retains the complete legacy graph
// transactionally and records that the exact local graph was unavailable.
func attachAuthorizedWorkspacePackageGraph(data *ReportData, authority *RunAuthority) {
	if data == nil || authority == nil || data.RepositoryGraph == nil {
		return
	}
	if data.repositoryGoFacts == nil {
		appendWorkspaceGraphUnavailableWarning(data)
		return
	}
	if err := authority.validate(); err != nil {
		appendWorkspaceGraphUnavailableWarning(data)
		return
	}
	snapshot, err := workspacesnapshot.New(workspacesnapshot.Input{
		AnalysisRoot:   authority.analysisRoot,
		Repository:     authority.repository,
		CapturedInputs: authority.inputs,
		AllowedPaths:   data.OpenablePaths,
	})
	if err != nil {
		appendWorkspaceGraphUnavailableWarning(data)
		return
	}
	graph, err := workspacegraph.New(workspacegraph.Input{
		Snapshot: snapshot,
		GoFacts:  *data.repositoryGoFacts,
	})
	if err != nil {
		appendWorkspaceGraphUnavailableWarning(data)
		return
	}
	if err := attachWorkspacePackageGraph(data, *data.repositoryGoFacts, graph); err != nil {
		appendWorkspaceGraphUnavailableWarning(data)
	}
}

func appendWorkspaceGraphUnavailableWarning(data *ReportData) {
	for _, warning := range data.Warnings {
		if warning == workspaceGraphUnavailableWarning {
			return
		}
	}
	data.Warnings = append(data.Warnings, workspaceGraphUnavailableWarning)
}

func requireCompleteExactWorkspaceGraph(data *ReportData) error {
	if data == nil {
		return &ExactWorkspaceGraphUnavailableError{}
	}
	facts := data.repositoryGoFacts
	if data.RepositoryGraph == nil {
		if facts == nil || len(facts.Modules) == 0 && len(facts.Packages) == 0 &&
			len(facts.InternalEdges) == 0 {
			return nil
		}
		return &ExactWorkspaceGraphUnavailableError{}
	}
	if facts == nil || len(facts.InternalEdges) > maxReportGraphFactEdges {
		return &ExactWorkspaceGraphUnavailableError{}
	}
	for _, warning := range data.Warnings {
		if warning == workspaceGraphUnavailableWarning {
			return &ExactWorkspaceGraphUnavailableError{}
		}
	}

	type edgeKey struct {
		from string
		to   string
	}
	want := make(map[edgeKey]struct{}, len(facts.InternalEdges))
	for _, edge := range facts.InternalEdges {
		if edge.From == "" || edge.To == "" {
			return &ExactWorkspaceGraphUnavailableError{}
		}
		want[edgeKey{from: edge.From, to: edge.To}] = struct{}{}
	}
	if len(data.RepositoryGraph.PackageEdges) != len(want) {
		return &ExactWorkspaceGraphUnavailableError{}
	}
	var previous edgeKey
	for index, edge := range data.RepositoryGraph.PackageEdges {
		key := edgeKey{from: edge.From, to: edge.To}
		if _, ok := want[key]; !ok {
			return &ExactWorkspaceGraphUnavailableError{}
		}
		if index > 0 && (key.from < previous.from ||
			key.from == previous.from && key.to <= previous.to) {
			return &ExactWorkspaceGraphUnavailableError{}
		}
		previous = key
	}
	return nil
}

func decodeSnapshotExactGoFacts(snapshotJSON []byte) (gofacts.Facts, error) {
	goFacts, err := preflightSnapshotExactGoFacts(snapshotJSON)
	if err != nil {
		return gofacts.Facts{}, err
	}
	var saved snapshotExactGoFacts
	if err := json.Unmarshal(snapshotJSON[goFacts.start:goFacts.end], &saved); err != nil {
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
		len(legacy.Packages) > maxReportGraphFactPackages {
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

	var packageCandidates []workspacepackageselection.Candidate
	if legacy.Packages != nil {
		packageCandidates = make(
			[]workspacepackageselection.Candidate,
			0,
			min(len(legacy.Packages), workspacepackageselection.MaxRows),
		)
	}
	for _, legacyPackage := range legacy.Packages {
		packageCandidates = append(
			packageCandidates,
			workspacepackageselection.Candidate{
				CanonicalPath:     legacyPackage.CanonicalPath,
				Name:              legacyPackage.Name,
				ModuleID:          legacyPackage.ModuleID,
				ModulePath:        legacyPackage.ModulePath,
				PackageDir:        legacyPackage.Dir,
				ModuleRelativeDir: legacyPackage.ModuleRelativeDir,
			},
		)
	}
	packageSelection, err := workspacepackageselection.New(
		workspacepackageselection.Input{
			Graph:      graph,
			Candidates: packageCandidates,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("workspace graph: package projection is unavailable")
	}
	selectedPackages := packageSelection.Packages()

	selectedEdges, err := exactWorkspacePackageEdges(facts, graph)
	if err != nil {
		return nil, fmt.Errorf("workspace graph: edge projection is unavailable")
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
		pkg, ok := graph.PackageInModule(fact.CanonicalPath, fact.ModuleID)
		if !ok || !workspacePackageFilesMatchFact(pkg, fact) {
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
			CanonicalPath:     selectedPackages[index].CanonicalPath,
			Name:              selectedPackages[index].Name,
			ModuleID:          selectedPackages[index].ModuleID,
			ModulePath:        selectedPackages[index].ModulePath,
			Dir:               selectedPackages[index].PackageDir,
			ModuleRelativeDir: selectedPackages[index].ModuleRelativeDir,
			DisplayPath:       legacyPackage.DisplayPath,
			Locality:          legacyPackage.Locality,
			Files:             files,
		}
	}

	if selectedEdges == nil {
		projected.PackageEdges = nil
	} else {
		projected.PackageEdges = make([]EdgeInfo, len(selectedEdges))
		for index, edge := range selectedEdges {
			projected.PackageEdges[index].From = edge.From
			projected.PackageEdges[index].To = edge.To
		}
	}
	return projected, nil
}

func exactWorkspacePackageEdges(
	facts gofacts.Facts,
	graph workspacegraph.Graph,
) ([]EdgeInfo, error) {
	if len(facts.InternalEdges) > maxReportGraphFactEdges {
		return nil, fmt.Errorf("workspace graph: exact edge facts exceed bounds")
	}

	type edgeKey struct {
		from string
		to   string
	}
	want := make(map[edgeKey]struct{}, len(facts.InternalEdges))
	for _, fact := range facts.InternalEdges {
		edge, ok := graph.Edge(fact.From, fact.To)
		if !ok {
			return nil, fmt.Errorf("workspace graph: exact edge fact is unavailable")
		}
		want[edgeKey{from: edge.FromPackage, to: edge.ToPackage}] = struct{}{}
	}

	graphEdges := graph.Edges()
	if len(graphEdges) != len(want) {
		return nil, fmt.Errorf("workspace graph: exact edge set differs")
	}
	if graphEdges == nil {
		return nil, nil
	}
	result := make([]EdgeInfo, len(graphEdges))
	for index, edge := range graphEdges {
		if _, ok := want[edgeKey{from: edge.FromPackage, to: edge.ToPackage}]; !ok {
			return nil, fmt.Errorf("workspace graph: exact edge set differs")
		}
		result[index] = EdgeInfo{From: edge.FromPackage, To: edge.ToPackage}
	}
	return result, nil
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
		cloned.PackageEdges = make([]EdgeInfo, len(graph.PackageEdges))
		copy(cloned.PackageEdges, graph.PackageEdges)
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

func workspacePackageFilesMatchFact(pkg workspacegraph.Package, fact gofacts.PackageFact) bool {
	if len(pkg.Files) != len(fact.Files) {
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
