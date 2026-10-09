package report

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/claims"
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/orientation"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/programpage"
	"github.com/dvordrova/repomap/internal/terminology"
)

// report.json is compact JSON. Each ProgramIndex is written in its own
// artifact encoding (programindex.EncodeValidated). A section whose encoding
// is, byte for byte, a file of the run's own target directories is not
// written a second time: report.json names that file by its path relative to
// the run directory with the SHA-256 of its bytes, and the reader decodes the
// file where the section would have been. Those directories are the run
// directory itself and, for another target's ProgramIndex, that target's run
// directory beside it, which the run's program page portfolio names.

// The sections report.json can name a run-directory file for.
const (
	savedSectionProgramIndex = "program_index"
	savedSectionFacts        = "facts"
	savedSectionClaims       = "claims"
	savedSectionOrientation  = "orientation"
	savedSectionGlossary     = "glossary"
)

// savedFile is a file one section of report.json is read from. Path is
// relative to the run directory: a file of the run directory itself, or
// ../<run-id>/<file> in another target's run directory of the same run.
type savedFile struct {
	Section string `json:"section"`
	Path    string `json:"path"`
	SHA256  string `json:"sha256"`
}

// savedReport is report.json as it is written. It reads back into
// ReportData plus Files.
type savedReport struct {
	*ReportData
	Files            []savedFile     `json:"files,omitempty"`
	ProgramPortfolio *savedPortfolio `json:"program_portfolio"`
}

// savedPortfolio is a ProgramPortfolio whose entries are already their
// artifact bytes; it reads back as ProgramPortfolio.
type savedPortfolio struct {
	Version         int               `json:"version"`
	DefaultTargetID string            `json:"default_target_id"`
	Entries         []json.RawMessage `json:"entries"`
}

func encodeReportJSON(data *ReportData, maxBytes int) ([]byte, error) {
	if data == nil {
		return nil, fmt.Errorf("report: data is required")
	}
	if err := validateProgramPresentation(data); err != nil {
		return nil, err
	}
	persisted := reportDataForPersistence(data)
	// SourceIDs are issued by the local report server after manifest
	// verification. They are session navigation IDs, not persistent evidence.
	persisted.SourceIDs = nil
	saved, err := newSavedReport(persisted, data.ArtifactsDir)
	if err != nil {
		return nil, err
	}
	b, err := json.Marshal(saved)
	if err != nil {
		return nil, err
	}
	b = append(b, '\n')
	if maxBytes > 0 && len(b) > maxBytes {
		return nil, &ReportResourceLimitError{
			LimitBytes:  maxBytes,
			ActualBytes: len(b),
		}
	}
	return b, nil
}

// newSavedReport prepares data, a copy the caller owns, for writing: it
// encodes every ProgramIndex and names each file of the run's target
// directories that holds a section's exact bytes. Without a run directory
// everything is written.
func newSavedReport(data *ReportData, runDir string) (savedReport, error) {
	saved := savedReport{ReportData: data}
	indexFiles, err := targetProgramIndexFiles(runDir)
	if err != nil {
		return savedReport{}, err
	}
	portfolio := data.ProgramPortfolio
	saved.ProgramPortfolio = &savedPortfolio{
		Version: portfolio.Version, DefaultTargetID: portfolio.DefaultTargetID,
		Entries: make([]json.RawMessage, 0, portfolio.Len()),
	}
	for _, binding := range portfolio.programs() {
		kept := false
		for _, original := range portfolio.files {
			path, named := indexFiles[binding.Target.ID+"\x00"+binding.SHA256]
			if original.Target.ID != binding.Target.ID || !named || !original.canonical ||
				filepath.Clean(original.Filename) != filepath.Clean(filepath.Join(runDir, filepath.FromSlash(path))) {
				continue
			}
			if _, err := original.readBytes(); err != nil {
				return savedReport{}, err
			}
			saved.Files = append(saved.Files, savedFile{Section: savedSectionProgramIndex, Path: path, SHA256: original.byteSHA256})
			kept = true
			break
		}
		if kept {
			continue
		}
		entry, found, err := portfolio.readTarget(binding.Target.ID)
		if err != nil || !found {
			if err == nil {
				err = fmt.Errorf("report: complete native index is missing")
			}
			return savedReport{}, err
		}
		encoded, err := programindex.EncodeValidated(entry)
		if err != nil {
			return savedReport{}, fmt.Errorf("report: program index %q: %w", entry.Target.ID, err)
		}
		if path, ok := indexFiles[entry.Target.ID+"\x00"+entry.SHA256]; ok {
			file, same, err := runFileHolding(runDir, savedSectionProgramIndex, path, encoded)
			if err != nil {
				return savedReport{}, err
			}
			if same {
				saved.Files = append(saved.Files, file)
				continue
			}
		}
		saved.ProgramPortfolio.Entries = append(saved.ProgramPortfolio.Entries, encoded)
	}
	if err := errors.Join(
		nameRunFile(&saved, runDir, savedSectionFacts, facts.ArtifactFilename, &data.Facts),
		nameRunFile(&saved, runDir, savedSectionClaims, claims.ArtifactFilename, &data.Claims),
		nameRunFile(&saved, runDir, savedSectionOrientation, orientation.ArtifactFilename, &data.Orientation),
		nameRunFile(&saved, runDir, savedSectionGlossary, terminology.CatalogFilename, &data.Glossary),
	); err != nil {
		return savedReport{}, err
	}
	return saved, nil
}

// targetProgramIndexFiles lists, by target ID and seal, the ProgramIndex
// files the artifact sets of the run's own target directories bind, as paths
// relative to runDir: runDir's own, and those of each other target's run
// directory that runDir's program page portfolio names.
func targetProgramIndexFiles(runDir string) (map[string]string, error) {
	if runDir == "" {
		return nil, nil
	}
	files := map[string]string{}
	if err := addProgramIndexFiles(files, runDir, ""); err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(filepath.Join(runDir, programpage.ArtifactFilename))
	if errors.Is(err, fs.ErrNotExist) {
		return files, nil
	}
	if err != nil {
		return nil, fmt.Errorf("report: read program page portfolio: %w", err)
	}
	pages, err := programpage.Decode(raw)
	if err != nil {
		return nil, fmt.Errorf("report: decode program page portfolio: %w", err)
	}
	for _, page := range pages.Pages {
		if page.RunID == filepath.Base(runDir) {
			continue
		}
		targetDir := filepath.Join(filepath.Dir(runDir), page.RunID)
		if err := addProgramIndexFiles(files, targetDir, "../"+page.RunID+"/"); err != nil {
			return nil, err
		}
	}
	return files, nil
}

// addProgramIndexFiles adds the files dir's artifact set binds, each named
// by prefix and its file name.
func addProgramIndexFiles(files map[string]string, dir, prefix string) error {
	raw, err := os.ReadFile(filepath.Join(dir, programindex.ArtifactSetFilename))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("report: read program index set: %w", err)
	}
	set, err := programindex.DecodeArtifactSet(raw)
	if err != nil {
		return fmt.Errorf("report: decode program index set of %s: %w", filepath.Base(dir), err)
	}
	for _, entry := range set.Entries {
		files[entry.TargetID+"\x00"+entry.IndexSHA256] = prefix + entry.Filename
	}
	return nil
}

// nameRunFile writes a section as the run directory's file when that file
// holds exactly the section's encoding.
func nameRunFile[T any](saved *savedReport, runDir, section, name string, field **T) error {
	if runDir == "" || *field == nil {
		return nil
	}
	encoded, err := json.Marshal(*field)
	if err != nil {
		return fmt.Errorf("report: encode %s: %w", section, err)
	}
	file, same, err := runFileHolding(runDir, section, name, encoded)
	if err != nil || !same {
		return err
	}
	saved.Files = append(saved.Files, file)
	*field = nil
	return nil
}

// runFileHolding tells whether the file at path, relative to the run
// directory, is exactly encoded, a final newline aside, and names it by the
// digest of its bytes.
func runFileHolding(runDir, section, path string, encoded []byte) (savedFile, bool, error) {
	raw, err := os.ReadFile(filepath.Join(runDir, filepath.FromSlash(path)))
	if errors.Is(err, fs.ErrNotExist) {
		return savedFile{}, false, nil
	}
	if err != nil {
		return savedFile{}, false, fmt.Errorf("report: read %s: %w", path, err)
	}
	if !bytes.Equal(bytes.TrimSuffix(raw, []byte("\n")), encoded) {
		return savedFile{}, false, nil
	}
	digest := sha256.Sum256(raw)
	return savedFile{Section: section, Path: path, SHA256: hex.EncodeToString(digest[:])}, true, nil
}

// decodeStrictReportJSON reads report.json exactly as it was written: no
// unknown fields, one value, the format this code renders, and every file of
// the run's target directories it names with the bytes it was written
// against.
func decodeStrictReportJSON(reportJSON []byte, runDir string) (ReportData, error) {
	var data ReportData
	saved := struct {
		*ReportData
		Files []savedFile `json:"files,omitempty"`
	}{ReportData: &data}
	decoder := json.NewDecoder(bytes.NewReader(reportJSON))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&saved); err != nil {
		return ReportData{}, fmt.Errorf("report: decode report.json: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return ReportData{}, fmt.Errorf("report: report.json contains multiple values")
		}
		return ReportData{}, fmt.Errorf("report: report.json has trailing data: %w", err)
	}
	if data.FormatVersion != CurrentFormatVersion {
		return ReportData{}, fmt.Errorf("report: unsupported report format version %d", data.FormatVersion)
	}
	if data.ProgramPortfolio == nil || data.GroupGraph == nil {
		return ReportData{}, fmt.Errorf("report: report.json is missing its program or group graph")
	}
	if err := readSavedFiles(&data, saved.Files, runDir); err != nil {
		return ReportData{}, err
	}
	if err := data.GroupGraph.hydratePortfolio(data.ProgramPortfolio); err != nil {
		return ReportData{}, fmt.Errorf("report: restore group graph: %w", err)
	}
	return data, nil
}

// readSavedFiles puts each named file's section back where report.json
// would have written it.
func readSavedFiles(data *ReportData, files []savedFile, runDir string) error {
	if len(files) == 0 {
		return nil
	}
	if runDir == "" {
		return fmt.Errorf("report: report.json names files of its run directory, which is not given")
	}
	named := make(map[string]bool, len(files))
	for _, file := range files {
		if named[file.Path] {
			return fmt.Errorf("report: report.json names %s twice", file.Path)
		}
		named[file.Path] = true
		raw, err := readSavedFile(runDir, file)
		if err != nil {
			return err
		}
		switch file.Section {
		case savedSectionProgramIndex:
			index, err := programindex.Decode(raw)
			if err != nil {
				return fmt.Errorf("report: decode %s: %w", file.Path, err)
			}
			encoded, err := programindex.EncodeValidated(index)
			if err != nil {
				return err
			}
			sourcePaths, err := nativeSourcePaths(index)
			if err != nil {
				return err
			}
			data.ProgramPortfolio.files = append(data.ProgramPortfolio.files, programFile{
				ProgramIndexFile: ProgramIndexFile{Filename: filepath.Join(runDir, filepath.FromSlash(file.Path)),
					Target: index.Target.Snapshot(), SHA256: index.SHA256},
				byteSHA256: file.SHA256, canonical: bytes.Equal(raw, encoded), sourcePaths: sourcePaths,
			})
		case savedSectionFacts:
			err = restoreSection(raw, file.Path, &data.Facts)
		case savedSectionClaims:
			err = restoreSection(raw, file.Path, &data.Claims)
		case savedSectionOrientation:
			// An orientation of another version is refused by its version.
			if err = orientation.CheckVersion(raw); err != nil {
				err = fmt.Errorf("report: %s: %w", file.Path, err)
			} else {
				err = restoreSection(raw, file.Path, &data.Orientation)
			}
		case savedSectionGlossary:
			err = restoreSection(raw, file.Path, &data.Glossary)
		default:
			err = fmt.Errorf("report: report.json names %s for an unknown section %q", file.Path, file.Section)
		}
		if err != nil {
			return err
		}
	}
	entries := data.ProgramPortfolio.Entries
	sort.SliceStable(entries, func(left, right int) bool {
		return programindex.TargetIDLess(entries[left].Target.ID, entries[right].Target.ID)
	})
	return data.ProgramPortfolio.validateBindings()
}

// readSavedFile reads a file report.json names: one of the run directory,
// or another target's ProgramIndex as ../<run-id>/<file>, with the bytes it
// was written against.
func readSavedFile(runDir string, file savedFile) ([]byte, error) {
	parts := strings.Split(file.Path, "/")
	name := parts[len(parts)-1]
	valid := name != "" && name != "." && name != ".." && name == filepath.Base(name)
	switch len(parts) {
	case 1:
	case 3:
		valid = valid && parts[0] == ".." && file.Section == savedSectionProgramIndex &&
			programpage.ValidateRunID(parts[1]) == nil && parts[1] != filepath.Base(runDir)
	default:
		valid = false
	}
	if !valid {
		return nil, fmt.Errorf("report: report.json names an invalid file %q", file.Path)
	}
	raw, err := os.ReadFile(filepath.Join(runDir, filepath.FromSlash(file.Path)))
	if err != nil {
		return nil, fmt.Errorf("report: read %s named by report.json: %w", file.Path, err)
	}
	digest := sha256.Sum256(raw)
	if hex.EncodeToString(digest[:]) != file.SHA256 {
		return nil, fmt.Errorf("report: %s is not the file report.json was written with", file.Path)
	}
	return raw, nil
}

// restoreSection decodes a named file exactly as the section's own field
// would have been decoded inside report.json.
func restoreSection[T any](raw []byte, name string, field **T) error {
	if *field != nil {
		return fmt.Errorf("report: report.json writes the section of %s as well", name)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	value := new(T)
	if err := decoder.Decode(value); err != nil {
		return fmt.Errorf("report: decode %s: %w", name, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("report: %s has trailing data", name)
	}
	*field = value
	return nil
}
