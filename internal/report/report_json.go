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

	"github.com/dvordrova/repomap/internal/claims"
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/orientation"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/terminology"
)

// report.json is compact JSON. Each ProgramIndex is written in its own
// artifact encoding (programindex.EncodeValidated). A section whose encoding
// is, byte for byte, a file of the same run directory is not written a second
// time: report.json names that file with the SHA-256 of its bytes, and the
// reader decodes the file where the section would have been. The other
// targets' ProgramIndexes belong to their own run directories and stay in
// report.json.

// The sections report.json can name a run-directory file for.
const (
	savedSectionProgramIndex = "program_index"
	savedSectionFacts        = "facts"
	savedSectionClaims       = "claims"
	savedSectionOrientation  = "orientation"
	savedSectionGlossary     = "glossary"
)

// savedFile is a run-directory file one section of report.json is read from.
type savedFile struct {
	Section string `json:"section"`
	Name    string `json:"name"`
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
// encodes every ProgramIndex and names each file of runDir that holds a
// section's exact bytes. Without a run directory everything is written.
func newSavedReport(data *ReportData, runDir string) (savedReport, error) {
	saved := savedReport{ReportData: data}
	indexFiles, err := runProgramIndexFiles(runDir)
	if err != nil {
		return savedReport{}, err
	}
	portfolio := data.ProgramPortfolio
	saved.ProgramPortfolio = &savedPortfolio{
		Version: portfolio.Version, DefaultTargetID: portfolio.DefaultTargetID,
		Entries: make([]json.RawMessage, 0, len(portfolio.Entries)),
	}
	for _, entry := range portfolio.Entries {
		encoded, err := programindex.EncodeValidated(entry)
		if err != nil {
			return savedReport{}, fmt.Errorf("report: program index %q: %w", entry.Target.ID, err)
		}
		if name, ok := indexFiles[entry.Target.ID+"\x00"+entry.SHA256]; ok {
			file, same, err := runFileHolding(runDir, savedSectionProgramIndex, name, encoded)
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

// runProgramIndexFiles lists the ProgramIndex files runDir's artifact set
// binds, by target ID and seal.
func runProgramIndexFiles(runDir string) (map[string]string, error) {
	if runDir == "" {
		return nil, nil
	}
	raw, err := os.ReadFile(filepath.Join(runDir, programindex.ArtifactSetFilename))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("report: read program index set: %w", err)
	}
	set, err := programindex.DecodeArtifactSet(raw)
	if err != nil {
		return nil, fmt.Errorf("report: decode program index set: %w", err)
	}
	files := make(map[string]string, len(set.Entries))
	for _, entry := range set.Entries {
		files[entry.TargetID+"\x00"+entry.IndexSHA256] = entry.Filename
	}
	return files, nil
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

// runFileHolding tells whether the run directory's file is exactly encoded,
// a final newline aside, and names it by the digest of its bytes.
func runFileHolding(runDir, section, name string, encoded []byte) (savedFile, bool, error) {
	raw, err := os.ReadFile(filepath.Join(runDir, name))
	if errors.Is(err, fs.ErrNotExist) {
		return savedFile{}, false, nil
	}
	if err != nil {
		return savedFile{}, false, fmt.Errorf("report: read %s: %w", name, err)
	}
	if !bytes.Equal(bytes.TrimSuffix(raw, []byte("\n")), encoded) {
		return savedFile{}, false, nil
	}
	digest := sha256.Sum256(raw)
	return savedFile{Section: section, Name: name, SHA256: hex.EncodeToString(digest[:])}, true, nil
}

// decodeStrictReportJSON reads report.json exactly as it was written: no
// unknown fields, one value, the format this code renders, and every file of
// runDir it names with the bytes it was written against.
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
	if err := data.GroupGraph.Hydrate(data.ProgramPortfolio.Entries); err != nil {
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
		if named[file.Name] {
			return fmt.Errorf("report: report.json names %s twice", file.Name)
		}
		named[file.Name] = true
		raw, err := readSavedFile(runDir, file)
		if err != nil {
			return err
		}
		switch file.Section {
		case savedSectionProgramIndex:
			var index programindex.Index
			if err := json.Unmarshal(raw, &index); err != nil {
				return fmt.Errorf("report: decode %s: %w", file.Name, err)
			}
			data.ProgramPortfolio.Entries = append(data.ProgramPortfolio.Entries, index)
		case savedSectionFacts:
			err = restoreSection(raw, file.Name, &data.Facts)
		case savedSectionClaims:
			err = restoreSection(raw, file.Name, &data.Claims)
		case savedSectionOrientation:
			err = restoreSection(raw, file.Name, &data.Orientation)
		case savedSectionGlossary:
			err = restoreSection(raw, file.Name, &data.Glossary)
		default:
			err = fmt.Errorf("report: report.json names %s for an unknown section %q", file.Name, file.Section)
		}
		if err != nil {
			return err
		}
	}
	entries := data.ProgramPortfolio.Entries
	sort.SliceStable(entries, func(left, right int) bool {
		return programindex.TargetIDLess(entries[left].Target.ID, entries[right].Target.ID)
	})
	return nil
}

func readSavedFile(runDir string, file savedFile) ([]byte, error) {
	if file.Name == "" || file.Name != filepath.Base(file.Name) || file.Name == "." || file.Name == ".." {
		return nil, fmt.Errorf("report: report.json names an invalid file %q", file.Name)
	}
	raw, err := os.ReadFile(filepath.Join(runDir, file.Name))
	if err != nil {
		return nil, fmt.Errorf("report: read %s named by report.json: %w", file.Name, err)
	}
	digest := sha256.Sum256(raw)
	if hex.EncodeToString(digest[:]) != file.SHA256 {
		return nil, fmt.Errorf("report: %s is not the file report.json was written with", file.Name)
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
