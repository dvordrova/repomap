package clientrecipe

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"path"
	"reflect"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/programindex"
)

const AuthorityVersion = 1

type SourceClass string

const (
	SourceProduction SourceClass = "production"
	SourceTest       SourceClass = "test"
	SourceGenerated  SourceClass = "generated"
	SourceProse      SourceClass = "prose"
	SourceManifest   SourceClass = "manifest"
	SourceOther      SourceClass = "other"
)

type SourceFact struct {
	Path           string      `json:"path"`
	Class          SourceClass `json:"class"`
	SHA256         string      `json:"sha256"`
	Bytes          int         `json:"bytes"`
	ProductionCode bool        `json:"production_code"`
}

type ExternalOperationFact struct {
	RelationID       string                `json:"relation_id"`
	DependencyID     string                `json:"dependency_id"`
	ImporterRef      string                `json:"importer_ref"`
	ExternalObjectID string                `json:"external_object_id"`
	CallerID         string                `json:"caller_id"`
	PackagePath      string                `json:"package_path"`
	CanonicalCallee  string                `json:"canonical_callee"`
	Callsite         programindex.Location `json:"callsite"`
	SourceExpression string                `json:"source_expression"`
	Generated        bool                  `json:"generated"`
}

type CallbackCoverage struct {
	ExactPassRelations    int    `json:"exact_pass_relations"`
	UnresolvedInvocations int    `json:"unresolved_invocations"`
	Status                string `json:"status"`
}

type AuthorityCoverage struct {
	FilesObserved          int `json:"files_observed"`
	ProductionFiles        int `json:"production_files"`
	TestFiles              int `json:"test_files"`
	GeneratedFiles         int `json:"generated_files"`
	ProseFiles             int `json:"prose_files"`
	ManifestFiles          int `json:"manifest_files"`
	OtherFiles             int `json:"other_files"`
	DependencyUsesObserved int `json:"dependency_uses_observed"`
	ExternalCallsObserved  int `json:"external_calls_observed"`
}

// Authority is the experiment's deterministic input boundary. It contains
// repository facts only; evaluator expectations are intentionally absent.
type Authority struct {
	Version            int                     `json:"version"`
	RepositorySHA256   string                  `json:"repository_sha256"`
	Program            programindex.Index      `json:"program_index"`
	Dependencies       dependencies.Catalog    `json:"dependencies"`
	Sources            []SourceFact            `json:"sources"`
	ExternalOperations []ExternalOperationFact `json:"external_operations"`
	Callbacks          CallbackCoverage        `json:"callbacks"`
	Coverage           AuthorityCoverage       `json:"coverage"`
	SHA256             string                  `json:"sha256"`
}

func EncodeAuthority(value Authority) ([]byte, error) {
	if err := value.Validate(); err != nil {
		return nil, err
	}
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("client recipe authority: encode: %w", err)
	}
	return append(raw, '\n'), nil
}

func DecodeAuthority(raw []byte) (Authority, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var value Authority
	if err := decoder.Decode(&value); err != nil {
		return Authority{}, fmt.Errorf("client recipe authority: decode: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return Authority{}, fmt.Errorf("client recipe authority: trailing data")
	}
	if err := value.Validate(); err != nil {
		return Authority{}, err
	}
	canonical, err := EncodeAuthority(value)
	if err != nil {
		return Authority{}, err
	}
	if !bytes.Equal(raw, canonical) {
		return Authority{}, fmt.Errorf("client recipe authority: non-canonical bytes")
	}
	return value, nil
}

func (value Authority) Validate() error {
	if value.Version != AuthorityVersion || !validSHA256(value.RepositorySHA256) ||
		value.Sources == nil || value.ExternalOperations == nil || !validSHA256(value.SHA256) {
		return fmt.Errorf("client recipe authority: invalid identity")
	}
	if err := value.Program.Validate(); err != nil {
		return fmt.Errorf("client recipe authority: ProgramIndex: %w", err)
	}
	if err := value.Dependencies.Validate(); err != nil {
		return fmt.Errorf("client recipe authority: dependencies: %w", err)
	}
	if value.Dependencies.Coverage.State != dependencies.CoverageComplete ||
		len(value.Dependencies.Coverage.Omissions) != 0 {
		return fmt.Errorf("client recipe authority: dependency coverage is incomplete")
	}
	if value.Program.SourceSHA256 != value.RepositorySHA256 {
		return fmt.Errorf("client recipe authority: ProgramIndex source binding mismatch")
	}
	objects := make(map[string]programindex.Object, len(value.Program.Objects))
	for _, object := range value.Program.Objects {
		objects[object.ID] = object
	}
	relations := make(map[string]programindex.Relation, len(value.Program.Relations))
	for _, relation := range value.Program.Relations {
		relations[relation.ID] = relation
	}
	dependenciesByID := make(map[string]dependencies.Dependency, len(value.Dependencies.Dependencies))
	dependencyImporters := make(map[string]map[string]struct{}, len(value.Dependencies.Dependencies))
	for _, dependency := range value.Dependencies.Dependencies {
		dependenciesByID[dependency.ID] = dependency
		refs := make(map[string]struct{}, len(dependency.ImporterRefs))
		for _, ref := range dependency.ImporterRefs {
			refs[ref] = struct{}{}
		}
		dependencyImporters[dependency.ID] = refs
	}
	wantCoverage := AuthorityCoverage{FilesObserved: len(value.Sources)}
	previousPath := ""
	generatedPaths := make(map[string]struct{})
	productionGoPaths := make(map[string]struct{})
	productionDirectories := make(map[string]struct{})
	for _, source := range value.Sources {
		if !source.Class.Valid() || !validSourcePath(source.Path) || !validSHA256(source.SHA256) || source.Bytes < 0 ||
			(previousPath != "" && previousPath >= source.Path) {
			return fmt.Errorf("client recipe authority: invalid or non-canonical source %q", source.Path)
		}
		previousPath = source.Path
		switch source.Class {
		case SourceProduction:
			wantCoverage.ProductionFiles++
		case SourceTest:
			wantCoverage.TestFiles++
		case SourceGenerated:
			wantCoverage.GeneratedFiles++
			generatedPaths[source.Path] = struct{}{}
		case SourceProse:
			wantCoverage.ProseFiles++
		case SourceManifest:
			wantCoverage.ManifestFiles++
		case SourceOther:
			wantCoverage.OtherFiles++
		}
		if source.ProductionCode != (source.Class == SourceProduction || source.Class == SourceGenerated) {
			return fmt.Errorf("client recipe authority: source production classification mismatch")
		}
		if source.ProductionCode && strings.HasSuffix(source.Path, ".go") {
			productionGoPaths[source.Path] = struct{}{}
			productionDirectories[path.Dir(source.Path)] = struct{}{}
		}
	}
	if value.RepositorySHA256 != sourceFactsDigest(value.Sources) {
		return fmt.Errorf("client recipe authority: repository/source inventory binding mismatch")
	}
	remainingTargetSources := make(map[string]struct{}, len(productionGoPaths))
	for sourcePath := range productionGoPaths {
		remainingTargetSources[sourcePath] = struct{}{}
	}
	for _, source := range value.Program.Target.Sources {
		if _, found := remainingTargetSources[source.Path]; !found {
			return fmt.Errorf("client recipe authority: ProgramIndex target source %q is not production authority", source.Path)
		}
		delete(remainingTargetSources, source.Path)
	}
	if len(remainingTargetSources) != 0 {
		return fmt.Errorf("client recipe authority: ProgramIndex target source coverage is incomplete")
	}
	for _, importer := range value.Dependencies.Importers {
		if _, found := productionDirectories[importer.RepositoryPath]; !found {
			return fmt.Errorf("client recipe authority: importer %q has no production source authority", importer.Ref)
		}
	}
	wantCoverage.DependencyUsesObserved = value.Dependencies.Coverage.ImportsObserved
	wantCoverage.ExternalCallsObserved = len(value.ExternalOperations)
	if wantCoverage != value.Coverage {
		return fmt.Errorf("client recipe authority: coverage mismatch")
	}
	if value.Callbacks.ExactPassRelations < 0 || value.Callbacks.UnresolvedInvocations < 0 ||
		(value.Callbacks.Status != "frontier" && value.Callbacks.Status != "none") {
		return fmt.Errorf("client recipe authority: invalid callback coverage")
	}
	previousOperation := ""
	for _, operation := range value.ExternalOperations {
		if operation.RelationID == "" || operation.DependencyID == "" || operation.ImporterRef == "" || operation.ExternalObjectID == "" ||
			operation.CallerID == "" || operation.PackagePath == "" || operation.CanonicalCallee == "" ||
			operation.SourceExpression == "" {
			return fmt.Errorf("client recipe authority: invalid external operation")
		}
		key := externalOperationKey(operation)
		if previousOperation != "" && previousOperation >= key {
			return fmt.Errorf("client recipe authority: operations are not canonical")
		}
		previousOperation = key
		if _, found := dependencyImporters[operation.DependencyID][operation.ImporterRef]; !found {
			return fmt.Errorf("client recipe authority: external operation has no exact dependency/importer join")
		}
		dependency := dependenciesByID[operation.DependencyID]
		if dependency.Kind != dependencies.KindExternal || dependency.PackagePath != operation.PackagePath {
			return fmt.Errorf("client recipe authority: external operation dependency mismatch")
		}
		relation, found := relations[operation.RelationID]
		if !found || relation.Kind != programindex.RelationInvokesExternal ||
			relation.Resolution != programindex.ResolutionExact || relation.FromID != operation.CallerID ||
			len(relation.ToIDs) != 1 || relation.ToIDs[0] != operation.ExternalObjectID || relation.Location == nil ||
			*relation.Location != operation.Callsite || len(relation.Witnesses) != 1 ||
			relation.Witnesses[0].SourceExpression != operation.SourceExpression {
			return fmt.Errorf("client recipe authority: external operation does not restore its exact ProgramIndex relation")
		}
		external := objects[operation.ExternalObjectID]
		if external.External == nil || external.External.PackagePath != operation.PackagePath {
			return fmt.Errorf("client recipe authority: external operation package mismatch")
		}
		_, generated := generatedPaths[operation.Callsite.Path]
		if generated != operation.Generated {
			return fmt.Errorf("client recipe authority: generated operation classification mismatch")
		}
	}
	if wantCallbacks := callbackCoverage(value.Program); value.Callbacks != wantCallbacks {
		return fmt.Errorf("client recipe authority: callback coverage mismatch")
	}
	wantOperations, err := externalOperationFacts(value.Program, value.Dependencies, value.Sources)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(value.ExternalOperations, wantOperations) {
		return fmt.Errorf("client recipe authority: external operation projection is incomplete")
	}
	wantSHA, err := authorityDigest(value)
	if err != nil {
		return err
	}
	if value.SHA256 != wantSHA {
		return fmt.Errorf("client recipe authority: sha256 mismatch")
	}
	return nil
}

func sealAuthority(value Authority) (Authority, error) {
	value.SHA256 = ""
	digest, err := authorityDigest(value)
	if err != nil {
		return Authority{}, err
	}
	value.SHA256 = digest
	if err := value.Validate(); err != nil {
		return Authority{}, err
	}
	return value, nil
}

func authorityDigest(value Authority) (string, error) {
	value.SHA256 = ""
	raw, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("client recipe authority: digest: %w", err)
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}

func (class SourceClass) Valid() bool {
	switch class {
	case SourceProduction, SourceTest, SourceGenerated, SourceProse, SourceManifest, SourceOther:
		return true
	default:
		return false
	}
}

func externalOperationKey(value ExternalOperationFact) string {
	return fmt.Sprintf("%s\x00%09d\x00%09d\x00%s\x00%s", value.Callsite.Path, value.Callsite.Line,
		value.Callsite.Column, value.CanonicalCallee, value.RelationID)
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func sourceFactsDigest(values []SourceFact) string {
	raw, _ := json.Marshal(values)
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

func validSourcePath(value string) bool {
	return value != "." && fs.ValidPath(value) && !strings.Contains(value, "\\")
}

func canonicalSourceFacts(values []SourceFact) []SourceFact {
	result := append([]SourceFact(nil), values...)
	sort.Slice(result, func(i, j int) bool { return result[i].Path < result[j].Path })
	return result
}
