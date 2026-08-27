package clientrecipe

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"

	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/programindex"
)

const H0Version = 1

type H0Call struct {
	RelationID      string                `json:"relation_id"`
	CanonicalCallee string                `json:"canonical_callee"`
	Callsite        programindex.Location `json:"callsite"`
}

// H0Candidate is intentionally limited to dependency/importer/callsite facts.
// It owns no configuration, wiring, interface, verification, or observability
// semantics.
type H0Candidate struct {
	ID                     string   `json:"id"`
	DependencyID           string   `json:"dependency_id"`
	PackagePath            string   `json:"package_path"`
	ImporterRef            string   `json:"importer_ref"`
	ImporterPackagePath    string   `json:"importer_package_path"`
	ImporterRepositoryPath string   `json:"importer_repository_path"`
	Calls                  []H0Call `json:"calls"`
}

type H0Excluded struct {
	ID           string            `json:"id"`
	DependencyID string            `json:"dependency_id"`
	ImporterRef  string            `json:"importer_ref"`
	Kind         dependencies.Kind `json:"kind"`
	Reason       string            `json:"reason"`
}

type H0Ledger struct {
	Observed int `json:"observed"`
	Admitted int `json:"admitted"`
	Excluded int `json:"excluded"`
}

type H0Result struct {
	Version         int           `json:"version"`
	AuthoritySHA256 string        `json:"authority_sha256"`
	Candidates      []H0Candidate `json:"candidates"`
	Excluded        []H0Excluded  `json:"excluded"`
	Ledger          H0Ledger      `json:"ledger"`
	SHA256          string        `json:"sha256"`
}

func BuildH0(authority Authority) (H0Result, error) {
	if err := authority.Validate(); err != nil {
		return H0Result{}, err
	}
	result, err := buildH0Projection(authority)
	if err != nil {
		return H0Result{}, err
	}
	if err := result.ValidateAgainst(authority); err != nil {
		return H0Result{}, err
	}
	return result, nil
}

func buildH0Projection(authority Authority) (H0Result, error) {
	importers := make(map[string]dependencies.Importer, len(authority.Dependencies.Importers))
	for _, importer := range authority.Dependencies.Importers {
		importers[importer.Ref] = importer
	}
	operations := make(map[string][]H0Call)
	for _, operation := range authority.ExternalOperations {
		key := operation.DependencyID + "\x00" + operation.ImporterRef
		operations[key] = append(operations[key], H0Call{
			RelationID: operation.RelationID, CanonicalCallee: operation.CanonicalCallee,
			Callsite: operation.Callsite,
		})
	}
	result := H0Result{
		Version: H0Version, AuthoritySHA256: authority.SHA256,
		Candidates: []H0Candidate{}, Excluded: []H0Excluded{},
	}
	for _, dependency := range authority.Dependencies.Dependencies {
		for _, importerRef := range dependency.ImporterRefs {
			importer := importers[importerRef]
			id := h0CandidateID(dependency.ID, importerRef)
			result.Ledger.Observed++
			if dependency.Kind != dependencies.KindExternal {
				reason := "workspace"
				if dependency.Kind == dependencies.KindStdlib {
					reason = "standard_library"
				}
				result.Excluded = append(result.Excluded, H0Excluded{
					ID: id, DependencyID: dependency.ID, ImporterRef: importerRef,
					Kind: dependency.Kind, Reason: reason,
				})
				continue
			}
			calls := append([]H0Call{}, operations[dependency.ID+"\x00"+importerRef]...)
			sort.Slice(calls, func(i, j int) bool { return h0CallKey(calls[i]) < h0CallKey(calls[j]) })
			result.Candidates = append(result.Candidates, H0Candidate{
				ID: id, DependencyID: dependency.ID, PackagePath: dependency.PackagePath,
				ImporterRef: importerRef, ImporterPackagePath: importer.PackagePath,
				ImporterRepositoryPath: importer.RepositoryPath, Calls: calls,
			})
		}
	}
	sort.Slice(result.Candidates, func(i, j int) bool { return result.Candidates[i].ID < result.Candidates[j].ID })
	sort.Slice(result.Excluded, func(i, j int) bool { return result.Excluded[i].ID < result.Excluded[j].ID })
	result.Ledger.Admitted = len(result.Candidates)
	result.Ledger.Excluded = len(result.Excluded)
	result.SHA256 = h0Digest(result)
	if err := result.Validate(); err != nil {
		return H0Result{}, err
	}
	return result, nil
}

func EncodeH0(value H0Result) ([]byte, error) {
	if err := value.Validate(); err != nil {
		return nil, err
	}
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("client recipe H0: encode: %w", err)
	}
	return append(raw, '\n'), nil
}

func DecodeH0(raw []byte) (H0Result, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var value H0Result
	if err := decoder.Decode(&value); err != nil {
		return H0Result{}, fmt.Errorf("client recipe H0: decode: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return H0Result{}, fmt.Errorf("client recipe H0: trailing data")
	}
	if err := value.Validate(); err != nil {
		return H0Result{}, err
	}
	canonical, err := EncodeH0(value)
	if err != nil {
		return H0Result{}, err
	}
	if !bytes.Equal(raw, canonical) {
		return H0Result{}, fmt.Errorf("client recipe H0: non-canonical bytes")
	}
	return value, nil
}

func (value H0Result) Validate() error {
	if value.Version != H0Version || !validSHA256(value.AuthoritySHA256) ||
		value.Candidates == nil || value.Excluded == nil || !validSHA256(value.SHA256) {
		return fmt.Errorf("client recipe H0: invalid identity")
	}
	if value.Ledger.Observed != len(value.Candidates)+len(value.Excluded) ||
		value.Ledger.Admitted != len(value.Candidates) || value.Ledger.Excluded != len(value.Excluded) {
		return fmt.Errorf("client recipe H0: ledger mismatch")
	}
	seen := make(map[string]struct{}, value.Ledger.Observed)
	previous := ""
	for _, candidate := range value.Candidates {
		if !validH0ID(candidate.ID, candidate.DependencyID, candidate.ImporterRef) ||
			candidate.PackagePath == "" || candidate.ImporterPackagePath == "" ||
			candidate.ImporterRepositoryPath == "" || candidate.Calls == nil ||
			(previous != "" && previous >= candidate.ID) {
			return fmt.Errorf("client recipe H0: invalid candidate")
		}
		previous = candidate.ID
		seen[candidate.ID] = struct{}{}
		previousCall := ""
		for _, call := range candidate.Calls {
			key := h0CallKey(call)
			if call.RelationID == "" || call.CanonicalCallee == "" || (previousCall != "" && previousCall >= key) {
				return fmt.Errorf("client recipe H0: invalid candidate call")
			}
			previousCall = key
		}
	}
	previous = ""
	for _, excluded := range value.Excluded {
		kindReasonValid := excluded.Kind == dependencies.KindStdlib && excluded.Reason == "standard_library" ||
			excluded.Kind == dependencies.KindWorkspace && excluded.Reason == "workspace"
		if !validH0ID(excluded.ID, excluded.DependencyID, excluded.ImporterRef) ||
			(excluded.Kind != dependencies.KindStdlib && excluded.Kind != dependencies.KindWorkspace) ||
			!kindReasonValid ||
			(previous != "" && previous >= excluded.ID) {
			return fmt.Errorf("client recipe H0: invalid exclusion")
		}
		if _, duplicate := seen[excluded.ID]; duplicate {
			return fmt.Errorf("client recipe H0: duplicate ledger identity")
		}
		seen[excluded.ID] = struct{}{}
		previous = excluded.ID
	}
	if value.SHA256 != h0Digest(value) {
		return fmt.Errorf("client recipe H0: sha256 mismatch")
	}
	return nil
}

// ValidateAgainst proves that the sealed baseline is the complete canonical
// projection of one exact Authority, rather than merely a self-consistent
// replacement that repeats its hash.
func (value H0Result) ValidateAgainst(authority Authority) error {
	if err := value.Validate(); err != nil {
		return err
	}
	if err := authority.Validate(); err != nil {
		return err
	}
	want, err := buildH0Projection(authority)
	if err != nil {
		return err
	}
	actualRaw, err := EncodeH0(value)
	if err != nil {
		return err
	}
	wantRaw, err := EncodeH0(want)
	if err != nil {
		return err
	}
	if !bytes.Equal(actualRaw, wantRaw) {
		return fmt.Errorf("client recipe H0: authority-bound projection mismatch")
	}
	return nil
}

func h0CandidateID(dependencyID, importerRef string) string {
	digest := sha256.Sum256([]byte("clientrecipe-h0-v1\x00" + dependencyID + "\x00" + importerRef))
	return "h0-" + hex.EncodeToString(digest[:12])
}

func validH0ID(id, dependencyID, importerRef string) bool {
	return id != "" && id == h0CandidateID(dependencyID, importerRef)
}

func h0CallKey(value H0Call) string {
	return fmt.Sprintf("%s\x00%09d\x00%09d\x00%s", value.Callsite.Path, value.Callsite.Line,
		value.Callsite.Column, value.RelationID)
}

func h0Digest(value H0Result) string {
	value.SHA256 = ""
	raw, _ := json.Marshal(value)
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}
