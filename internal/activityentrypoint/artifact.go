package activityentrypoint

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"reflect"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/programindex"
)

const MaxArtifactBytes = 64 << 20

func newResult(index programindex.Index, selected []programindex.Object, coverage Coverage) (Result, error) {
	result := Result{
		Version: Version, ProgramIndexSHA256: index.SHA256,
		Objects: cloneObjects(selected), Coverage: coverage,
	}
	digest, err := resultDigest(result)
	if err != nil {
		return Result{}, err
	}
	result.SHA256 = digest
	if err := result.Validate(); err != nil {
		return Result{}, err
	}
	return result, nil
}

// Snapshot returns a consumer-owned copy of the selected exact object rows.
func (result Result) Snapshot() Result {
	copy := result
	copy.Objects = cloneObjects(result.Objects)
	return copy
}

// Validate checks the canonical standalone shape, complete candidate ledger,
// activity object rows, and artifact seal. ValidateAgainst additionally proves
// that every row and the coverage ledger came from one exact ProgramIndex.
func (result Result) Validate() error {
	if result.Version != Version || !validSHA256(result.ProgramIndexSHA256) || !validSHA256(result.SHA256) || result.Objects == nil {
		return fmt.Errorf("activity entrypoint: invalid result identity")
	}
	if err := validateCoverage(result.Coverage); err != nil {
		return err
	}
	if len(result.Objects) != result.Coverage.Selected || len(result.Objects) > result.Coverage.CandidatesAdvertised ||
		len(result.Objects) > MaxSelectedEntrypoints {
		return fmt.Errorf("activity entrypoint: selected object bound exceeded")
	}
	for position, object := range result.Objects {
		if err := validateSelectedObject(object); err != nil {
			return err
		}
		if position > 0 && !candidateObjectLess(result.Objects[position-1], object) {
			return fmt.Errorf("activity entrypoint: selected objects are not canonical")
		}
	}
	want, err := resultDigest(result)
	if err != nil {
		return err
	}
	if result.SHA256 != want {
		return fmt.Errorf("activity entrypoint: artifact sha256 mismatch")
	}
	return nil
}

// ValidateAgainst proves the ProgramIndex identity, complete activity-anchor
// coverage, and byte-for-byte restoration of each selected Object row.
func (result Result) ValidateAgainst(index programindex.Index) error {
	if err := result.Validate(); err != nil {
		return err
	}
	compiled, err := compile(index)
	if err != nil {
		return err
	}
	if result.ProgramIndexSHA256 != compiled.index.SHA256 {
		return fmt.Errorf("activity entrypoint: ProgramIndex identity mismatch")
	}
	wantCoverage := compiled.coverage
	wantCoverage.Batches = len(compiled.batches)
	wantCoverage.ModelCalled = len(compiled.candidates) > 0
	wantCoverage.Selected = len(result.Objects)
	if result.Coverage != wantCoverage {
		return fmt.Errorf("activity entrypoint: candidate coverage does not match ProgramIndex")
	}
	authority := make(map[string]programindex.Object, len(compiled.candidates))
	for _, value := range compiled.candidates {
		authority[value.object.ID] = value.object
	}
	for _, object := range result.Objects {
		want, ok := authority[object.ID]
		if !ok || !reflect.DeepEqual(want, object) {
			return fmt.Errorf("activity entrypoint: selected object is outside exact callable authority")
		}
	}
	return nil
}

// Encode returns exact canonical standalone artifact bytes.
func Encode(result Result) ([]byte, error) {
	if err := result.Validate(); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("activity entrypoint: encode artifact: %w", err)
	}
	if len(encoded) == 0 || len(encoded) > MaxArtifactBytes {
		return nil, fmt.Errorf(
			"activity entrypoint: artifact is %d bytes, limit is %d", len(encoded), MaxArtifactBytes,
		)
	}
	return encoded, nil
}

// Decode rejects unknown fields, trailing values, noncanonical bytes, seal
// failures, and any object or coverage row not owned by index.
func Decode(encoded []byte, index programindex.Index) (Result, error) {
	if len(encoded) == 0 || len(encoded) > MaxArtifactBytes {
		return Result{}, fmt.Errorf("activity entrypoint: invalid artifact size")
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var result Result
	if err := decoder.Decode(&result); err != nil {
		return Result{}, fmt.Errorf("activity entrypoint: decode artifact: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return Result{}, fmt.Errorf("activity entrypoint: trailing JSON value")
		}
		return Result{}, fmt.Errorf("activity entrypoint: trailing artifact data: %w", err)
	}
	if err := result.ValidateAgainst(index); err != nil {
		return Result{}, err
	}
	canonical, err := Encode(result)
	if err != nil {
		return Result{}, err
	}
	if !bytes.Equal(encoded, canonical) {
		return Result{}, fmt.Errorf("activity entrypoint: artifact is not canonical")
	}
	return result, nil
}

// ArtifactSHA256 returns the digest of the exact canonical artifact bytes.
func (result Result) ArtifactSHA256() (string, error) {
	encoded, err := Encode(result)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func validateCoverage(coverage Coverage) error {
	if coverage.ObjectsIndexed < 0 || coverage.ObjectsIndexed > programindex.MaxObjects ||
		coverage.ProgramObjectsOmitted < 0 || coverage.ProgramRelationsOmitted < 0 ||
		coverage.ProgramTargetsOmitted < 0 || coverage.ProgramWitnessesOmitted < 0 ||
		coverage.CallablesIndexed < 0 || coverage.CallablesIndexed > coverage.ObjectsIndexed ||
		coverage.CallablesWithoutLocation < 0 || coverage.CallablesWithoutLocation > coverage.CallablesIndexed ||
		coverage.CallablesIneligible < 0 ||
		coverage.CallablesIneligible > coverage.CallablesIndexed-coverage.CallablesWithoutLocation ||
		coverage.SeededModulesIndexed < 0 ||
		coverage.SeededModulesIndexed > coverage.ObjectsIndexed-coverage.CallablesIndexed ||
		coverage.SeededModulesWithoutLocation < 0 ||
		coverage.SeededModulesWithoutLocation > coverage.SeededModulesIndexed ||
		coverage.CandidatesObserved < 0 ||
		coverage.CandidatesObserved+coverage.CallablesWithoutLocation+coverage.CallablesIneligible+
			coverage.SeededModulesWithoutLocation != coverage.CallablesIndexed+coverage.SeededModulesIndexed ||
		coverage.CandidatesAdvertised < 0 || coverage.CandidatesOmitted < 0 ||
		coverage.CandidatesAdvertised+coverage.CandidatesOmitted != coverage.CandidatesObserved ||
		coverage.CandidatesAdvertised > MaxAdvertisedCandidates || coverage.CandidatesOmitted != 0 ||
		coverage.Selected < 0 || coverage.Selected > coverage.CandidatesAdvertised ||
		coverage.Selected > MaxSelectedEntrypoints {
		return fmt.Errorf("activity entrypoint: invalid candidate coverage")
	}
	if coverage.CandidatesAdvertised == 0 {
		if coverage.Batches != 0 || coverage.ModelCalled {
			return fmt.Errorf("activity entrypoint: empty coverage cannot call the model")
		}
		return nil
	}
	if coverage.Batches <= 0 || coverage.Batches > MaxCandidateBatches || !coverage.ModelCalled {
		return fmt.Errorf("activity entrypoint: non-empty coverage has invalid batch execution")
	}
	return nil
}

func validateSelectedObject(object programindex.Object) error {
	selectableKind := callable(object.Kind) || object.Kind == programindex.ObjectModule || object.Kind == programindex.ObjectPackage
	if !selectableKind || !validProgramID(object.ID) || !validText(object.SourceRef) || !validText(object.Name) ||
		!object.Visibility.Valid() || !validOptionalText(object.Signature) || !validOptionalProgramID(object.OwnerID) ||
		!validOptionalProgramID(object.ContainerID) || object.Location == nil || !validLocation(*object.Location) {
		return fmt.Errorf("activity entrypoint: invalid selected activity object")
	}
	return nil
}

func resultDigest(result Result) (string, error) {
	payload := result.Snapshot()
	payload.SHA256 = ""
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("activity entrypoint: encode digest material: %w", err)
	}
	if len(encoded) > MaxArtifactBytes {
		return "", fmt.Errorf("activity entrypoint: digest material exceeds artifact bound")
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func cloneObjects(values []programindex.Object) []programindex.Object {
	result := make([]programindex.Object, len(values))
	for position, value := range values {
		result[position] = cloneObject(value)
	}
	return result
}

func validProgramID(value string) bool {
	const prefix = "program-object-"
	return strings.HasPrefix(value, prefix) && validSHA256(strings.TrimPrefix(value, prefix))
}

func validOptionalProgramID(value string) bool {
	return value == "" || validProgramID(value)
}

func validLocation(value programindex.Location) bool {
	return validPath(value.Path) && value.Line > 0 && value.Column > 0
}

func validPath(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || strings.Contains(value, "\\") ||
		strings.HasPrefix(value, "/") || !fs.ValidPath(value) || value == "." || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func validText(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || len(value) > programindex.MaxTextBytes || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func validOptionalText(value string) bool {
	return value == "" || validText(value)
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
