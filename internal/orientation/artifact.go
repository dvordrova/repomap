package orientation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/dvordrova/repomap/internal/debugdump"
)

// Encode validates and returns the exact canonical JSON of one artifact.
func Encode(result Result) ([]byte, error) {
	if err := result.Validate(); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("orientation: encode artifact: %w", err)
	}
	return encoded, nil
}

// Decode strictly restores one canonical orientation artifact.
func Decode(encoded []byte) (Result, error) {
	if len(encoded) == 0 {
		return Result{}, fmt.Errorf("orientation: invalid artifact size")
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var result Result
	if err := decoder.Decode(&result); err != nil {
		return Result{}, fmt.Errorf("orientation: decode artifact: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return Result{}, fmt.Errorf("orientation: trailing JSON value")
		}
		return Result{}, fmt.Errorf("orientation: trailing artifact data: %w", err)
	}
	if err := result.Validate(); err != nil {
		return Result{}, err
	}
	canonical, err := Encode(result)
	if err != nil {
		return Result{}, err
	}
	if !bytes.Equal(encoded, canonical) {
		return Result{}, fmt.Errorf("orientation: artifact is not canonical")
	}
	return result, nil
}

// Persist atomically writes the orientation artifact into one run directory.
func Persist(runDir string, result Result) error {
	owned, err := result.Snapshot()
	if err != nil {
		return fmt.Errorf("orientation: own artifact: %w", err)
	}
	encoded, err := Encode(owned)
	if err != nil {
		return err
	}
	writer, err := debugdump.OpenWriter(runDir, false)
	if err != nil {
		return fmt.Errorf("orientation: open artifact writer: %w", err)
	}
	defer writer.Close()
	if err := writer.WriteValidatedFile(
		ArtifactFilename,
		encoded,
		func(saved []byte) error {
			restored, decodeErr := Decode(saved)
			if decodeErr != nil {
				return decodeErr
			}
			if restored.SHA256 != owned.SHA256 || !bytes.Equal(saved, encoded) {
				return fmt.Errorf("orientation: persisted authority mismatch")
			}
			return nil
		},
	); err != nil {
		return fmt.Errorf("orientation: persist artifact: %w", err)
	}
	return nil
}

// PersistRejected writes the rejected rows as JSON lines. An empty slice
// still writes the file so a reader can tell "nothing rejected" from
// "stage did not run".
func PersistRejected(runDir string, rows []RejectedRow) error {
	var buffer bytes.Buffer
	for _, row := range rows {
		if row.Raw == nil {
			row.Raw = json.RawMessage("null")
		}
		encoded, err := json.Marshal(row)
		if err != nil {
			return fmt.Errorf("orientation: encode rejected row: %w", err)
		}
		buffer.Write(encoded)
		buffer.WriteByte('\n')
	}
	writer, err := debugdump.OpenWriter(runDir, false)
	if err != nil {
		return fmt.Errorf("orientation: open rejected writer: %w", err)
	}
	defer writer.Close()
	if err := writer.WriteFile(RejectedFilename, buffer.Bytes()); err != nil {
		return fmt.Errorf("orientation: persist rejected rows: %w", err)
	}
	return nil
}

// ReadRejected restores the rejected rows from one run directory. A missing
// file is an error; an empty file yields no rows.
func ReadRejected(runDir string) ([]RejectedRow, error) {
	raw, err := os.ReadFile(filepath.Join(runDir, RejectedFilename))
	if err != nil {
		return nil, fmt.Errorf("orientation: read rejected rows: %w", err)
	}
	rows := []RejectedRow{}
	for _, line := range bytes.Split(raw, []byte{'\n'}) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var row RejectedRow
		if err := json.Unmarshal(line, &row); err != nil {
			return nil, fmt.Errorf("orientation: decode rejected row: %w", err)
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// Read restores the canonical orientation artifact from one run directory.
func Read(runDir string) (Result, error) {
	encoded, err := os.ReadFile(filepath.Join(runDir, ArtifactFilename))
	if err != nil {
		return Result{}, fmt.Errorf("orientation: read artifact: %w", err)
	}
	result, err := Decode(encoded)
	if err != nil {
		return Result{}, fmt.Errorf("orientation: decode persisted artifact: %w", err)
	}
	return result, nil
}
