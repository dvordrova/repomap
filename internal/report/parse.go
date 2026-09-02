package report

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/claims"
	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/documentationreduce"
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/orientation"
	"github.com/dvordrova/repomap/internal/programindex"
)

const (
	advisoryReportTargetMetadataBytes = 4 << 20
	maxReportTargetMetadataBytes      = 0
)

// snapshotJSON is the neutral provenance shape consumed by report publication.
// Adapter-native facts remain outside report and semantic authority.
type snapshotJSON struct {
	RepoName string `json:"repo_name"`
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
	if err := restoreDependencyCatalog(absDir, data); err != nil {
		return nil, err
	}
	if err := restoreReducedDocumentation(absDir, data); err != nil {
		return nil, err
	}
	if err := restoreGroupGraphView(absDir, data); err != nil {
		return nil, err
	}
	if err := restoreTargetOutcomePortfolioView(absDir, data); err != nil {
		return nil, err
	}
	if err := restoreFirstDayArtifacts(absDir, data); err != nil {
		return nil, err
	}
	if err := collectOpenablePaths(data); err != nil {
		return nil, err
	}
	return data, nil
}

// restoreFirstDayArtifacts reads facts.json, claims.json and orientation.json
// when they exist. A missing file leaves the field nil so run directories
// written before these stages still restore; a present but invalid file is
// an error because a corrupt artifact must never publish as "absent".
func restoreFirstDayArtifacts(runDir string, data *ReportData) error {
	if data == nil {
		return fmt.Errorf("report: first-day artifacts require report data")
	}
	if present, err := artifactFilePresent(runDir, facts.ArtifactFilename); err != nil {
		return err
	} else if present {
		result, err := facts.Read(runDir)
		if err != nil {
			return fmt.Errorf("report: %w", err)
		}
		data.Facts = &result
	}
	if present, err := artifactFilePresent(runDir, claims.ArtifactFilename); err != nil {
		return err
	} else if present {
		result, err := claims.Read(runDir)
		if err != nil {
			return fmt.Errorf("report: %w", err)
		}
		data.Claims = &result
	}
	if present, err := artifactFilePresent(runDir, orientation.ArtifactFilename); err != nil {
		return err
	} else if present {
		result, err := orientation.Read(runDir)
		if err != nil {
			return fmt.Errorf("report: %w", err)
		}
		data.Orientation = &result
	}
	return nil
}

func artifactFilePresent(runDir, name string) (bool, error) {
	info, err := os.Lstat(filepath.Join(runDir, name))
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("report: inspect %s: %w", name, err)
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("report: %s is not a regular file", name)
	}
	return true, nil
}

func restoreDependencyCatalog(runDir string, data *ReportData) error {
	if data == nil {
		return fmt.Errorf("report: dependency catalog requires report data")
	}
	encoded, _, err := readBoundedProgramArtifact(
		filepath.Join(runDir, dependencies.ArtifactFilename),
		dependencies.MaxArtifactBytes,
		"dependency catalog",
		false,
	)
	if err != nil {
		return err
	}
	catalog, err := dependencies.Decode(encoded)
	if err != nil {
		return fmt.Errorf("report: decode dependency catalog: %w", err)
	}
	owned := catalog
	data.dependencyCatalog = &owned
	return nil
}

// restoreReducedDocumentation proves that every ProgramIndex in this page is
// the enriched adapter result and that all of them bind the same exact
// repository documentation reduction. A base ProgramIndex is not a publishable
// page authority.
func restoreReducedDocumentation(runDir string, data *ReportData) error {
	if data == nil || len(data.programIndexes) == 0 {
		return fmt.Errorf("report: reduced documentation ProgramIndex authority is unavailable")
	}
	encoded, _, err := readBoundedProgramArtifact(
		filepath.Join(runDir, documentationreduce.ArtifactFilename),
		0,
		"reduced documentation",
		false,
	)
	if err != nil {
		return err
	}
	reduced, err := documentationreduce.Decode(encoded)
	if err != nil {
		return fmt.Errorf("report: decode reduced documentation: %w", err)
	}
	for _, index := range data.programIndexes {
		if index.Categorization == nil {
			return fmt.Errorf("report: ProgramIndex %q is not categorized", index.Target.ID)
		}
		if index.Categorization.ReducedDocumentationSHA256 != reduced.ReductionSHA256 {
			return fmt.Errorf("report: ProgramIndex %q does not bind reduced documentation", index.Target.ID)
		}
	}
	owned, err := reduced.Snapshot()
	if err != nil {
		return fmt.Errorf("report: reduced documentation: %w", err)
	}
	data.reducedDocumentation = &owned
	for _, source := range owned.Sources {
		data.materialInputPaths = append(data.materialInputPaths, source.Path)
	}
	return nil
}

// restoreGroupGraphView binds the required page-local GroupsIndex directly to
// the default ProgramIndex. Multi-target finalization may replace this
// singleton view with the complete transaction-local matched set later.
func restoreGroupGraphView(runDir string, data *ReportData) error {
	encoded, present, err := readBoundedProgramArtifact(
		filepath.Join(runDir, groupindex.ArtifactFilename),
		0,
		"groups index",
		false,
	)
	if err != nil {
		return err
	}
	if !present {
		return fmt.Errorf("report: groups index is missing")
	}
	if data.defaultProgramIndex == nil || data.ProgramPortfolio == nil {
		return fmt.Errorf("report: groups index default ProgramIndex is unavailable")
	}
	index, err := groupindex.Decode(encoded)
	if err != nil {
		return fmt.Errorf("report: decode groups index: %w", err)
	}
	if index.ProgramIndexSHA256 != data.defaultProgramIndex.SHA256 ||
		!reflect.DeepEqual(index.Target, data.defaultProgramIndex.Target) {
		return fmt.Errorf("report: groups index does not bind the default ProgramIndex")
	}
	local := index.Snapshot()
	data.localGroupsIndex = &local
	for _, connection := range index.Connections {
		if connection.From.TargetID != index.Target.ID || connection.To.TargetID != index.Target.ID {
			// A matched page-local artifact can legitimately cite a foreign
			// endpoint. Its complete set is transaction-local and is bound by
			// BindRunAuthorityGroupGraph before publication.
			return nil
		}
	}
	if err := BindGroupGraphView(data, []groupindex.Index{index}); err != nil {
		return fmt.Errorf("report: project group graph view: %w", err)
	}
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
	data.programIndexes = make([]programindex.Index, len(indexes))
	for position := range indexes {
		data.programIndexes[position] = indexes[position].Snapshot()
	}
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
	if !info.Mode().IsRegular() || info.Size() <= 0 ||
		(maxBytes > 0 && info.Size() > int64(maxBytes)) {
		return nil, false, fmt.Errorf("report: %s is not a bounded regular file", label)
	}
	encoded, err := os.ReadFile(artifactPath)
	if err != nil {
		return nil, false, fmt.Errorf("report: read %s: %w", label, err)
	}
	if len(encoded) == 0 || (maxBytes > 0 && len(encoded) > maxBytes) {
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
	if data.GroupGraph != nil {
		groupPaths, err := data.GroupGraph.SourcePaths()
		if err != nil {
			return fmt.Errorf("report: group graph source paths: %w", err)
		}
		for _, sourcePath := range groupPaths {
			if err := add(sourcePath); err != nil {
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
	data.targetMetadataBytes = len(encoded)
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
	data.RepoName = snapshot.RepoName
	return nil
}
