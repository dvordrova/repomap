package report

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"iter"
	"os"
	"reflect"
	"sort"

	"github.com/dvordrova/repomap/internal/programindex"
)

// ProgramIndexFile names one complete original native artifact. Target and
// SHA256 are its already selected binding, never a substitute for its graph.
type ProgramIndexFile struct {
	Filename string
	Target   programindex.Target
	SHA256   string
}

type programBinding struct {
	Target programindex.Target
	SHA256 string
}

type programFile struct {
	ProgramIndexFile
	byteSHA256  string
	canonical   bool
	sourcePaths []string
}

// BindProgramPortfolioFiles validates every complete artifact before replacing
// the binding. It retains owned target metadata and original byte/native seals;
// later native consumers read the original complete index, one target at a time.
// Public in-memory inputs continue to use the isolated snapshot binding.
func BindProgramPortfolioFiles(data *ReportData, defaultTargetID string, files []ProgramIndexFile) error {
	if data == nil || len(files) == 0 {
		return fmt.Errorf("program portfolio: report data or native files are missing")
	}
	portfolio := &ProgramPortfolio{Version: ProgramPortfolioVersion, DefaultTargetID: defaultTargetID,
		Entries: []programindex.Index{}, files: make([]programFile, 0, len(files))}
	for _, file := range files {
		raw, err := os.ReadFile(file.Filename)
		if err != nil {
			return fmt.Errorf("program portfolio: read %q: %w", file.Filename, err)
		}
		index, err := programindex.Decode(raw)
		if err != nil {
			return err
		}
		if index.SHA256 != file.SHA256 || !reflect.DeepEqual(index.Target, file.Target) {
			return fmt.Errorf("program portfolio: artifact %q does not bind its selected native target", file.Filename)
		}
		encoded, err := programindex.EncodeValidated(index)
		if err != nil {
			return err
		}
		digest := sha256.Sum256(raw)
		file.Target = index.Target.Snapshot()
		sourcePaths, err := nativeSourcePaths(index)
		if err != nil {
			return err
		}
		portfolio.files = append(portfolio.files, programFile{ProgramIndexFile: file,
			byteSHA256: hex.EncodeToString(digest[:]), canonical: bytes.Equal(raw, encoded), sourcePaths: sourcePaths})
	}
	sort.Slice(portfolio.files, func(i, j int) bool {
		return programindex.TargetIDLess(portfolio.files[i].Target.ID, portfolio.files[j].Target.ID)
	})
	if err := portfolio.validateBindings(); err != nil {
		return err
	}
	// No complete default/native graph is retained alongside these bindings.
	data.ProgramPortfolio = portfolio
	data.programIndexes = nil
	data.defaultProgramIndex = nil
	return nil
}

func (portfolio ProgramPortfolio) programs() []programBinding {
	programs := make([]programBinding, 0, len(portfolio.Entries)+len(portfolio.files))
	for _, entry := range portfolio.Entries {
		programs = append(programs, programBinding{Target: entry.Target, SHA256: entry.SHA256})
	}
	for _, file := range portfolio.files {
		programs = append(programs, programBinding{Target: file.Target, SHA256: file.SHA256})
	}
	sort.Slice(programs, func(i, j int) bool {
		return programindex.TargetIDLess(programs[i].Target.ID, programs[j].Target.ID)
	})
	return programs
}

func (portfolio ProgramPortfolio) Len() int {
	return len(portfolio.Entries) + len(portfolio.files)
}

func (portfolio ProgramPortfolio) validateBindings() error {
	if portfolio.Version != ProgramPortfolioVersion || !programindex.ValidTargetID(portfolio.DefaultTargetID) {
		return fmt.Errorf("program portfolio: invalid identity")
	}
	programs := portfolio.programs()
	if len(programs) == 0 {
		return fmt.Errorf("program portfolio: entries are empty")
	}
	defaults := 0
	for i, binding := range programs {
		if err := binding.Target.Validate(); err != nil {
			return err
		}
		if i > 0 && !programindex.TargetIDLess(programs[i-1].Target.ID, binding.Target.ID) {
			return fmt.Errorf("program portfolio: duplicate or noncanonical native targets")
		}
		if binding.Target.ID == portfolio.DefaultTargetID {
			defaults++
		}
	}
	if defaults != 1 {
		return fmt.Errorf("program portfolio: default target must have exactly one entry")
	}
	return nil
}

func (portfolio ProgramPortfolio) defaultBinding() (programBinding, error) {
	for _, binding := range portfolio.programs() {
		if binding.Target.ID == portfolio.DefaultTargetID {
			return binding, nil
		}
	}
	return programBinding{}, fmt.Errorf("program portfolio: default target is missing")
}

func (file programFile) read() (programindex.Index, error) {
	raw, err := file.readBytes()
	if err != nil {
		return programindex.Index{}, err
	}
	// readBytes matched the bytes to those the validated index was saved as.
	index, err := programindex.DecodeVerified(raw)
	if err != nil {
		return programindex.Index{}, err
	}
	if index.SHA256 != file.SHA256 || !reflect.DeepEqual(index.Target, file.Target) {
		return programindex.Index{}, fmt.Errorf("program portfolio: original native target binding changed")
	}
	return index, nil
}

func (file programFile) readBytes() ([]byte, error) {
	raw, err := os.ReadFile(file.Filename)
	if err != nil {
		return nil, fmt.Errorf("program portfolio: read %q: %w", file.Filename, err)
	}
	digest := sha256.Sum256(raw)
	if hex.EncodeToString(digest[:]) != file.byteSHA256 {
		return nil, fmt.Errorf("program portfolio: original artifact bytes changed: %s", file.Filename)
	}
	return raw, nil
}

// Path-only consumers share the complete inventory gathered during binding.
// Recheck every original byte without decoding another full native graph.
func (portfolio ProgramPortfolio) visitSourcePaths(add func(string) error) error {
	for _, entry := range portfolio.Entries {
		if err := visitNativeSourcePaths(entry, add); err != nil {
			return err
		}
	}
	for _, file := range portfolio.files {
		stream, err := os.Open(file.Filename)
		if err != nil {
			return fmt.Errorf("program portfolio: read %q: %w", file.Filename, err)
		}
		digest := sha256.New()
		_, readErr := io.Copy(digest, stream)
		closeErr := stream.Close()
		if readErr != nil {
			return readErr
		}
		if closeErr != nil {
			return closeErr
		}
		if hex.EncodeToString(digest.Sum(nil)) != file.byteSHA256 {
			return fmt.Errorf("program portfolio: original artifact bytes changed: %s", file.Filename)
		}
		for _, path := range file.sourcePaths {
			if err := add(path); err != nil {
				return err
			}
		}
	}
	return nil
}

func (portfolio ProgramPortfolio) readTarget(targetID string) (programindex.Index, bool, error) {
	for _, entry := range portfolio.Entries {
		if entry.Target.ID == targetID {
			return entry, true, nil
		}
	}
	for _, file := range portfolio.files {
		if file.Target.ID == targetID {
			index, err := file.read()
			return index, true, err
		}
	}
	return programindex.Index{}, false, nil
}

func (portfolio ProgramPortfolio) indexes() iter.Seq2[programindex.Index, error] {
	return func(yield func(programindex.Index, error) bool) {
		for _, binding := range portfolio.programs() {
			index, found, err := portfolio.readTarget(binding.Target.ID)
			if err == nil && !found {
				err = fmt.Errorf("program portfolio: native target is missing")
			}
			if !yield(index, err) || err != nil {
				return
			}
		}
	}
}

// ReadProgramIndexes visits every complete native index in canonical target
// order. Callers consume it during the callback instead of accumulating bodies.
func (portfolio ProgramPortfolio) ReadProgramIndexes(visit func(programindex.Index) error) error {
	for index, err := range portfolio.indexes() {
		if err != nil {
			return err
		}
		if err := visit(index); err != nil {
			return err
		}
	}
	return nil
}

// The complete path inventory is derived only from the original validated
// native value, shared by ordinary binding and saved-file restoration.
func nativeSourcePaths(index programindex.Index) ([]string, error) {
	paths := map[string]struct{}{}
	if err := visitNativeSourcePaths(index, func(path string) error {
		if err := validateManifestPath(path); err != nil {
			return err
		}
		paths[path] = struct{}{}
		return nil
	}); err != nil {
		return nil, err
	}
	result := make([]string, 0, len(paths))
	for path := range paths {
		result = append(result, path)
	}
	sort.Strings(result)
	return result, nil
}
