package report

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/readmetargetscout"
)

type readmeFileRoleArtifact struct {
	Version int                         `json:"version"`
	Files   []readmeFileRoleArtifactRow `json:"files"`
}

type readmeFileRoleArtifactRow struct {
	FileRef         string                             `json:"file_ref"`
	Path            string                             `json:"path"`
	Classifications []readmetargetscout.Classification `json:"classifications"`
}

// decodeReadmeFileRoleAuthority independently validates the persisted
// classifier handoff and returns only its exact FileRef/path dictionary. The
// prose classifications remain cube input, not report presentation data.
func decodeReadmeFileRoleAuthority(raw []byte) (map[string]string, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("report: README file-role artifact is empty")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var artifact readmeFileRoleArtifact
	if err := decoder.Decode(&artifact); err != nil {
		return nil, fmt.Errorf("report: decode README file-role artifact: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, fmt.Errorf("report: README file-role artifact has multiple JSON values")
		}
		return nil, fmt.Errorf("report: README file-role artifact has trailing data: %w", err)
	}
	if artifact.Version != 1 || artifact.Files == nil || len(artifact.Files) == 0 {
		return nil, fmt.Errorf("report: README file-role artifact has invalid identity or file count")
	}
	pathsByRef := make(map[string]string, len(artifact.Files))
	refsByPath := make(map[string]string, len(artifact.Files))
	for _, file := range artifact.Files {
		if !validProgramViewText(file.FileRef) || validateManifestPath(file.Path) != nil ||
			len(file.Classifications) == 0 {
			return nil, fmt.Errorf("report: README file-role artifact contains an invalid file row")
		}
		if _, duplicate := pathsByRef[file.FileRef]; duplicate {
			return nil, fmt.Errorf("report: README file-role artifact repeats a file ref")
		}
		if _, duplicate := refsByPath[file.Path]; duplicate {
			return nil, fmt.Errorf("report: README file-role artifact repeats a file path")
		}
		classes := make(map[readmetargetscout.FileClass]struct{}, len(file.Classifications))
		for _, classification := range file.Classifications {
			if !validReadmeFileClass(classification.Class) || len(classification.Hypotheses) == 0 {
				return nil, fmt.Errorf("report: README file-role artifact contains an invalid classification")
			}
			if _, duplicate := classes[classification.Class]; duplicate {
				return nil, fmt.Errorf("report: README file-role artifact repeats a classification")
			}
			classes[classification.Class] = struct{}{}
			for _, hypothesis := range classification.Hypotheses {
				if !validReadmeRoleHypothesis(hypothesis) {
					return nil, fmt.Errorf("report: README file-role artifact contains an invalid hypothesis")
				}
			}
		}
		pathsByRef[file.FileRef] = file.Path
		refsByPath[file.Path] = file.FileRef
	}
	return pathsByRef, nil
}

func validReadmeFileClass(value readmetargetscout.FileClass) bool {
	switch value {
	case readmetargetscout.ClassTargetEntry,
		readmetargetscout.ClassExampleEntry,
		readmetargetscout.ClassTestEntry,
		readmetargetscout.ClassSupportToolEntry,
		readmetargetscout.ClassConfiguration,
		readmetargetscout.ClassDatabaseAsset,
		readmetargetscout.ClassClientEntry,
		readmetargetscout.ClassDocumentation,
		readmetargetscout.ClassDeployment,
		readmetargetscout.ClassInterfaceContract:
		return true
	default:
		return false
	}
}

func validReadmeRoleHypothesis(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}
