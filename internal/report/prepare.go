package report

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/dvordrova/repomap/internal/documentationreduce"
	"github.com/dvordrova/repomap/internal/programindex"
)

// NewData projects one producer-owned index and documentation. Repository
// assembly then binds the complete original native portfolio, reading saved
// completed targets one at a time. The caller adds the repository graph,
// facts, claims, orientation and timing before Generate. Inputs stay read-only.
func NewData(runDir, repoName string, index programindex.Index, documentation documentationreduce.Result) (*ReportData, error) {
	absDir, err := filepath.Abs(runDir)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(repoName) == "" {
		return nil, fmt.Errorf("report: repository name is missing")
	}
	if err := documentation.Validate(); err != nil {
		return nil, err
	}
	portfolio, err := NewProgramPortfolio(index.Target.ID, []programindex.Index{index})
	if err != nil {
		return nil, err
	}
	data := &ReportData{
		FormatVersion: CurrentFormatVersion, ArtifactsDir: absDir, RepoName: repoName,
		ProgramPortfolio: portfolio, defaultProgramIndex: &index,
		programIndexes:                      []programindex.Index{index},
		defaultProgramIndexArtifactFilename: programindex.ArtifactFilename,
		ReadmeOverview:                      strings.TrimSpace(documentation.Overview),
	}
	for _, source := range documentation.Sources {
		data.materialInputPaths = append(data.materialInputPaths, source.Path)
	}
	return data, nil
}
