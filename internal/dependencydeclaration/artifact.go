package dependencydeclaration

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/programindex"
)

// Snapshot returns a fully consumer-owned copy of the sealed result.
func (result Result) Snapshot() Result {
	copyResult := result
	copyResult.Sources = append([]Source{}, result.Sources...)
	copyResult.Packages = clonePackages(result.Packages)
	copyResult.Includes = cloneIncludes(result.Includes)
	copyResult.Frontiers = cloneFrontiers(result.Frontiers)
	return copyResult
}

// Validate checks canonical order, exact local joins, stable identities,
// coverage accounting, bounds and the result seal.
func (result Result) Validate() error {
	if result.Version != Version || !validSHA256(result.CorpusSHA256) ||
		!validSHA256(result.ProgramIndexSHA256) || !plainValue(result.TargetID) ||
		!validSHA256(result.SHA256) {
		return fmt.Errorf("dependency declarations: invalid result identity")
	}
	if err := validateScope(result.Scope); err != nil {
		return err
	}
	if result.Sources == nil || result.Packages == nil || result.Includes == nil || result.Frontiers == nil {
		return fmt.Errorf("dependency declarations: result inventories must be present")
	}
	if len(result.Sources) > MaxSources || len(result.Packages) > MaxPackages ||
		len(result.Includes) > MaxIncludes || len(result.Frontiers) > MaxFrontiers {
		return fmt.Errorf("dependency declarations: result bound exceeded")
	}

	sourceByID := make(map[string]Source, len(result.Sources))
	sourceIdentity := make(map[string]struct{}, len(result.Sources))
	totalBytes := 0
	for position, source := range result.Sources {
		if err := validateSource(result.CorpusSHA256, result.Scope.RepositoryPath, source); err != nil {
			return fmt.Errorf("dependency declarations: source %d: %w", position, err)
		}
		if _, duplicate := sourceByID[source.ID]; duplicate {
			return fmt.Errorf("dependency declarations: duplicate source identity")
		}
		projectionKey := string(source.FileRef) + "\x00" + source.Format
		if _, duplicate := sourceIdentity[projectionKey]; duplicate {
			return fmt.Errorf("dependency declarations: duplicate source projection")
		}
		sourceByID[source.ID] = source
		sourceIdentity[projectionKey] = struct{}{}
		totalBytes += source.ByteCount
		if totalBytes > MaxTotalBytes {
			return fmt.Errorf("dependency declarations: total source byte bound %d exceeded", MaxTotalBytes)
		}
		if position > 0 && sourceKey(result.Sources[position-1]) >= sourceKey(source) {
			return fmt.Errorf("dependency declarations: sources are not canonical")
		}
	}

	statementCount := 0
	seenStatements := make(map[string]struct{})
	for position, value := range result.Packages {
		if err := validatePackage(result.Scope.Ecosystem, value, sourceByID, seenStatements); err != nil {
			return fmt.Errorf("dependency declarations: package %d: %w", position, err)
		}
		statementCount += len(value.Statements)
		if statementCount > MaxStatements {
			return fmt.Errorf("dependency declarations: statement bound %d exceeded", MaxStatements)
		}
		if position > 0 && packageKey(result.Packages[position-1]) >= packageKey(value) {
			return fmt.Errorf("dependency declarations: packages are not canonical")
		}
	}

	seenIncludes := make(map[string]struct{}, len(result.Includes))
	for position, value := range result.Includes {
		if err := validateIncludeShape(value); err != nil {
			return fmt.Errorf("dependency declarations: include %d: %w", position, err)
		}
		if _, ok := sourceByID[value.SourceRef]; !ok {
			return fmt.Errorf("dependency declarations: include has unknown source ref")
		}
		if value.Resolution == IncludeResolved {
			if _, ok := sourceByID[value.TargetSourceRef]; !ok {
				return fmt.Errorf("dependency declarations: include has unknown target source ref")
			}
		}
		if value.ID != includeIdentity(value) {
			return fmt.Errorf("dependency declarations: include identity mismatch")
		}
		if _, duplicate := seenIncludes[value.ID]; duplicate {
			return fmt.Errorf("dependency declarations: duplicate include identity")
		}
		seenIncludes[value.ID] = struct{}{}
		if position > 0 && includeKey(result.Includes[position-1]) >= includeKey(value) {
			return fmt.Errorf("dependency declarations: includes are not canonical")
		}
	}

	seenFrontiers := make(map[string]struct{}, len(result.Frontiers))
	frontierSources := make(map[string]struct{})
	for position, value := range result.Frontiers {
		if err := validateFrontierShape(value); err != nil {
			return fmt.Errorf("dependency declarations: frontier %d: %w", position, err)
		}
		source, ok := sourceByID[value.SourceRef]
		if !ok {
			return fmt.Errorf("dependency declarations: frontier has unknown source ref")
		}
		if value.Kind == FrontierSource {
			if source.State != SourceFrontier {
				return fmt.Errorf("dependency declarations: source frontier references parsed source")
			}
			frontierSources[value.SourceRef] = struct{}{}
		}
		if value.ID != frontierIdentity(value) {
			return fmt.Errorf("dependency declarations: frontier identity mismatch")
		}
		if _, duplicate := seenFrontiers[value.ID]; duplicate {
			return fmt.Errorf("dependency declarations: duplicate frontier identity")
		}
		seenFrontiers[value.ID] = struct{}{}
		if position > 0 && frontierKey(result.Frontiers[position-1]) >= frontierKey(value) {
			return fmt.Errorf("dependency declarations: frontiers are not canonical")
		}
	}
	for _, source := range result.Sources {
		if source.State != SourceFrontier {
			continue
		}
		if _, ok := frontierSources[source.ID]; !ok {
			return fmt.Errorf("dependency declarations: frontier source has no exact boundary")
		}
	}

	wantCoverage := deriveCoverage(result.Sources, result.Packages, result.Includes, result.Frontiers)
	if !reflect.DeepEqual(result.Coverage, wantCoverage) || !result.Coverage.State.Valid() {
		return fmt.Errorf("dependency declarations: coverage ledger mismatch")
	}
	wantDigest, err := resultDigest(result)
	if err != nil {
		return err
	}
	if result.SHA256 != wantDigest {
		return fmt.Errorf("dependency declarations: result sha256 mismatch")
	}
	return nil
}

// ValidateAgainst binds the standalone artifact to one exact corpus snapshot
// and ProgramIndex. It deliberately does not reread live file bytes: repository
// mutation during a run is allowed and every Source already binds the bytes
// actually read by its producing adapter.
func (result Result) ValidateAgainst(snapshot corpus.Snapshot, index programindex.Index) error {
	if err := result.Validate(); err != nil {
		return err
	}
	if err := snapshot.Validate(); err != nil {
		return fmt.Errorf("dependency declarations: corpus: %w", err)
	}
	ownedIndex := index.Snapshot()
	if err := ownedIndex.Validate(); err != nil {
		return fmt.Errorf("dependency declarations: ProgramIndex: %w", err)
	}
	if result.CorpusSHA256 != snapshot.SHA256 || result.ProgramIndexSHA256 != ownedIndex.SHA256 ||
		result.TargetID != ownedIndex.Target.ID || result.Scope.Language != ownedIndex.Target.Language {
		return fmt.Errorf("dependency declarations: input authority mismatch")
	}
	entries := make(map[corpus.FileID]corpus.Entry, len(snapshot.Entries))
	for _, entry := range snapshot.Entries {
		entries[entry.ID] = entry
	}
	for _, source := range result.Sources {
		entry, ok := entries[source.FileRef]
		if !ok || entry.Path != source.Path {
			return fmt.Errorf("dependency declarations: source %q does not match corpus", source.ID)
		}
	}
	return nil
}

// Encode emits the unique canonical JSON representation of a valid artifact.
func Encode(result Result) ([]byte, error) {
	if err := result.Validate(); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("dependency declarations: encode artifact: %w", err)
	}
	if len(encoded) == 0 || len(encoded) > MaxArtifactBytes {
		return nil, fmt.Errorf("dependency declarations: artifact is %d bytes, limit is %d", len(encoded), MaxArtifactBytes)
	}
	return encoded, nil
}

// DecodeStandalone rejects unknown fields, trailing values, noncanonical
// bytes and invalid seals. Cross-input consumers should additionally call the
// adapter-owned authority validator for the inputs they actually possess.
func DecodeStandalone(encoded []byte) (Result, error) {
	if len(encoded) == 0 || len(encoded) > MaxArtifactBytes {
		return Result{}, fmt.Errorf("dependency declarations: invalid artifact size")
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var result Result
	if err := decoder.Decode(&result); err != nil {
		return Result{}, fmt.Errorf("dependency declarations: decode artifact: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return Result{}, fmt.Errorf("dependency declarations: trailing JSON value")
		}
		return Result{}, fmt.Errorf("dependency declarations: trailing artifact data: %w", err)
	}
	if err := result.Validate(); err != nil {
		return Result{}, err
	}
	canonical, err := Encode(result)
	if err != nil {
		return Result{}, err
	}
	if !bytes.Equal(encoded, canonical) {
		return Result{}, fmt.Errorf("dependency declarations: artifact is not canonical")
	}
	return result, nil
}

// Decode additionally binds the canonical artifact to the exact repository
// corpus and ProgramIndex used by an ordinary producer.
func Decode(encoded []byte, snapshot corpus.Snapshot, index programindex.Index) (Result, error) {
	result, err := DecodeStandalone(encoded)
	if err != nil {
		return Result{}, err
	}
	if err := result.ValidateAgainst(snapshot, index); err != nil {
		return Result{}, err
	}
	return result, nil
}

func (result Result) ArtifactSHA256() (string, error) {
	encoded, err := Encode(result)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func validateSource(corpusSHA, scopeRoot string, source Source) error {
	if !validFileRef(source.FileRef) || !repositoryPath(source.Path) || !pathWithin(scopeRoot, source.Path) ||
		!token(source.Format) || !source.State.Valid() || !validSHA256(source.ContentSHA256) ||
		source.ByteCount < 0 || source.ByteCount > MaxSourceBytes {
		return fmt.Errorf("invalid source")
	}
	wantID := identity("ddsrc-", struct {
		Version                     int
		CorpusSHA256                string
		FileRef                     corpus.FileID
		Path, Format, ContentSHA256 string
		ByteCount                   int
	}{Version, corpusSHA, source.FileRef, source.Path, source.Format, source.ContentSHA256, source.ByteCount})
	if source.ID != wantID {
		return fmt.Errorf("source identity mismatch")
	}
	return nil
}

func validatePackage(ecosystem string, value Package, sources map[string]Source, seen map[string]struct{}) error {
	if value.Ecosystem != ecosystem || !token(value.Ecosystem) || !plainValue(value.Name) ||
		!token(value.NormalizedName) || value.Names == nil || len(value.Names) == 0 ||
		value.Statements == nil || len(value.Statements) == 0 || value.Name != value.Names[0] ||
		value.ID != packageIdentity(value) {
		return fmt.Errorf("invalid package")
	}
	for index, name := range value.Names {
		if !plainValue(name) || (index > 0 && value.Names[index-1] >= name) {
			return fmt.Errorf("package names are not canonical")
		}
	}
	observedNames := make([]string, 0, len(value.Statements))
	for position, statement := range value.Statements {
		if err := validateStatementShape(statement); err != nil {
			return err
		}
		if _, ok := sources[statement.SourceRef]; !ok || statement.NormalizedName != value.NormalizedName ||
			statement.ID != statementIdentity(statement) {
			return fmt.Errorf("statement authority mismatch")
		}
		if _, duplicate := seen[statement.ID]; duplicate {
			return fmt.Errorf("duplicate statement identity")
		}
		seen[statement.ID] = struct{}{}
		observedNames = append(observedNames, statement.Name)
		if position > 0 && statementKey(value.Statements[position-1]) >= statementKey(statement) {
			return fmt.Errorf("statements are not canonical")
		}
	}
	if !reflect.DeepEqual(value.Names, canonicalStrings(observedNames)) {
		return fmt.Errorf("package name ledger mismatch")
	}
	return nil
}

func resultDigest(result Result) (string, error) {
	copyResult := result.Snapshot()
	copyResult.SHA256 = ""
	encoded, err := json.Marshal(copyResult)
	if err != nil {
		return "", fmt.Errorf("dependency declarations: seal result: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func clonePackages(values []Package) []Package {
	result := make([]Package, len(values))
	for index, value := range values {
		result[index] = value
		result[index].Names = append([]string{}, value.Names...)
		result[index].Statements = make([]Statement, len(value.Statements))
		for statementIndex, statement := range value.Statements {
			result[index].Statements[statementIndex] = statement
			result[index].Statements[statementIndex].Extras = append([]string{}, statement.Extras...)
			result[index].Statements[statementIndex].Location = cloneLocation(statement.Location)
		}
	}
	return result
}

func cloneIncludes(values []Include) []Include {
	result := append([]Include{}, values...)
	for index := range result {
		result[index].Location = cloneLocation(values[index].Location)
	}
	return result
}

func cloneFrontiers(values []Frontier) []Frontier {
	result := append([]Frontier{}, values...)
	for index := range result {
		result[index].Location = cloneLocation(values[index].Location)
	}
	return result
}

func pathWithin(root, candidate string) bool {
	return root == "" || candidate == root || strings.HasPrefix(candidate, root+"/")
}
