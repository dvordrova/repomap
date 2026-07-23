package report

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/dvordrova/repomap/internal/goldenmechanism"
	"github.com/dvordrova/repomap/internal/semanticdiscovery"
)

const (
	GoldenMechanismProbeFile        = "golden_mechanism_probe_attempt.json"
	MechanismV1CollectionDir        = "fresh_mechanisms/accepted"
	maxMechanismV1FileBytes         = 512 << 10
	maxMechanismV1CollectionEntries = 16
)

// MechanismV1CollectionPath returns the report-owned directory containing
// independently replayable Mechanism v1 entries. Every direct child is named
// by its candidate ID and contains MechanismFile, GoldenMechanismFactsFile,
// and GoldenMechanismProbeFile. The legacy root files remain supported.
func MechanismV1CollectionPath(runDir string) string {
	return filepath.Join(runDir, filepath.FromSlash(MechanismV1CollectionDir))
}

// ExtractMechanismV1 constructs the one approved compatibility object from an
// already accepted saved Golden record. It performs no repository analysis or
// model call.
func ExtractMechanismV1(
	runDir string,
	candidateID string,
	identity semanticdiscovery.MechanismIdentity,
) (semanticdiscovery.Mechanism, semanticdiscovery.Artifact, error) {
	absDir, err := filepath.Abs(runDir)
	if err != nil {
		return semanticdiscovery.Mechanism{}, semanticdiscovery.Artifact{}, err
	}
	data, err := ReadRunDir(absDir)
	if err != nil {
		return semanticdiscovery.Mechanism{}, semanticdiscovery.Artifact{}, err
	}
	if err := validateMechanismV1Scope(data, identity); err != nil {
		return semanticdiscovery.Mechanism{}, semanticdiscovery.Artifact{}, err
	}
	bundle, record, candidate, probe, err := mechanismV1ExtractionInputs(
		data,
		candidateID,
		filepath.Join(absDir, GoldenMechanismFactsFile),
		filepath.Join(absDir, GoldenMechanismRecordFile),
		filepath.Join(absDir, GoldenMechanismProbeFile),
	)
	if err != nil {
		return semanticdiscovery.Mechanism{}, semanticdiscovery.Artifact{}, err
	}
	mechanism, artifact, err := semanticdiscovery.ExtractMechanism(
		bundle,
		record,
		candidate.ID,
		identity,
		probe,
	)
	if err != nil {
		return semanticdiscovery.Mechanism{}, semanticdiscovery.Artifact{}, err
	}
	return mechanism, artifact, nil
}

// replaySavedMechanismV1 layers one independently replayed Mechanism onto the
// existing report artifacts. Failure leaves all previous report state intact.
func replaySavedMechanismV1(
	data *ReportData,
	mechanismPath string,
	factsPath string,
	probePath string,
) string {
	if data == nil {
		return "mechanism v1 unavailable: report data is required"
	}
	startHere := data.StartHereArtifactID
	defer func() { data.StartHereArtifactID = startHere }()
	mechanism, found, warning := loadSavedMechanismV1(mechanismPath)
	if warning != "" || !found {
		return warning
	}
	return replayDecodedMechanismV1(data, mechanism, factsPath, probePath)
}

// replaySavedMechanismV1Collection layers every valid collection entry onto
// the report independently. One malformed or stale entry cannot remove an
// already replayed artifact or prevent later entries from being considered.
// Start Here remains owned by the later onboarding finalizer, so collection
// replay always preserves the incoming selection.
func replaySavedMechanismV1Collection(data *ReportData, runDir string) []string {
	if data == nil {
		return []string{"mechanism v1 collection unavailable: report data is required"}
	}
	collectionPath := MechanismV1CollectionPath(runDir)
	info, err := os.Lstat(collectionPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return []string{"mechanism v1 collection unavailable: cannot inspect saved collection"}
	}
	if !info.IsDir() {
		return []string{"mechanism v1 collection unavailable: saved collection is not a directory"}
	}
	entries, err := os.ReadDir(collectionPath)
	if err != nil {
		return []string{"mechanism v1 collection unavailable: cannot read saved collection"}
	}

	warnings := make([]string, 0)
	if len(entries) > maxMechanismV1CollectionEntries {
		warnings = append(warnings, fmt.Sprintf(
			"mechanism v1 collection: %d entries exceed the limit of %d; remaining entries were skipped",
			len(entries),
			maxMechanismV1CollectionEntries,
		))
		entries = entries[:maxMechanismV1CollectionEntries]
	}

	startHere := data.StartHereArtifactID
	previousSupplement := cloneSemanticSupplementFacts(data.SemanticSupplementalFacts)
	defer func() {
		data.StartHereArtifactID = startHere
		data.SemanticSupplementalFacts = previousSupplement
	}()
	seenCandidates, seenMechanisms := replayedMechanismV1Index(data, runDir)
	for _, entry := range entries {
		entryName := entry.Name()
		entryPath := filepath.Join(collectionPath, entryName)
		entryInfo, entryErr := os.Lstat(entryPath)
		if entryErr != nil || !entryInfo.IsDir() {
			warnings = append(warnings, mechanismV1CollectionWarning(
				entryName,
				"entry is not a directory",
			))
			continue
		}

		mechanismPath := filepath.Join(entryPath, semanticdiscovery.MechanismFile)
		mechanism, found, warning := loadSavedMechanismV1(mechanismPath)
		if warning != "" {
			warnings = append(warnings, mechanismV1CollectionWarning(entryName, warning))
			continue
		}
		if !found {
			warnings = append(warnings, mechanismV1CollectionWarning(
				entryName,
				"mechanism v1 unavailable: saved object is missing",
			))
			continue
		}
		candidateID := mechanism.Payload.Candidate.ID
		if entryName != candidateID {
			warnings = append(warnings, mechanismV1CollectionWarning(
				entryName,
				"entry name does not match the bound candidate id",
			))
			continue
		}
		fingerprint := mechanismV1Fingerprint{
			MechanismID: mechanism.ID,
			ContentSHA:  mechanism.ContentSHA256,
			CandidateID: candidateID,
		}
		if duplicate, conflict := duplicateMechanismV1(
			fingerprint,
			seenCandidates,
			seenMechanisms,
		); duplicate {
			continue
		} else if conflict != "" {
			warnings = append(warnings, mechanismV1CollectionWarning(entryName, conflict))
			continue
		}

		warning = replayDecodedMechanismV1(
			data,
			mechanism,
			filepath.Join(entryPath, GoldenMechanismFactsFile),
			filepath.Join(entryPath, GoldenMechanismProbeFile),
		)
		if warning != "" {
			warnings = append(warnings, mechanismV1CollectionWarning(entryName, warning))
			continue
		}
		seenCandidates[candidateID] = fingerprint
		seenMechanisms[mechanism.ID] = fingerprint
	}
	return warnings
}

type mechanismV1Fingerprint struct {
	MechanismID string
	ContentSHA  string
	CandidateID string
}

func replayedMechanismV1Index(
	data *ReportData,
	runDir string,
) (map[string]mechanismV1Fingerprint, map[string]mechanismV1Fingerprint) {
	byCandidate := make(map[string]mechanismV1Fingerprint)
	byMechanism := make(map[string]mechanismV1Fingerprint)
	root, found, warning := loadSavedMechanismV1(filepath.Join(runDir, semanticdiscovery.MechanismFile))
	if warning != "" || !found {
		return byCandidate, byMechanism
	}
	startHere := data.StartHereArtifactID
	previousSupplement := cloneSemanticSupplementFacts(data.SemanticSupplementalFacts)
	warning = replayDecodedMechanismV1(
		data,
		root,
		filepath.Join(runDir, GoldenMechanismFactsFile),
		filepath.Join(runDir, GoldenMechanismProbeFile),
	)
	data.StartHereArtifactID = startHere
	data.SemanticSupplementalFacts = previousSupplement
	if warning != "" {
		return byCandidate, byMechanism
	}
	fingerprint := mechanismV1Fingerprint{
		MechanismID: root.ID,
		ContentSHA:  root.ContentSHA256,
		CandidateID: root.Payload.Candidate.ID,
	}
	byCandidate[fingerprint.CandidateID] = fingerprint
	byMechanism[fingerprint.MechanismID] = fingerprint
	return byCandidate, byMechanism
}

func duplicateMechanismV1(
	fingerprint mechanismV1Fingerprint,
	byCandidate map[string]mechanismV1Fingerprint,
	byMechanism map[string]mechanismV1Fingerprint,
) (duplicate bool, conflict string) {
	if existing, found := byCandidate[fingerprint.CandidateID]; found {
		if existing == fingerprint {
			return true, ""
		}
		return false, "candidate is already bound to different canonical mechanism content"
	}
	if existing, found := byMechanism[fingerprint.MechanismID]; found {
		if existing == fingerprint {
			return true, ""
		}
		return false, "logical mechanism id is already bound to different canonical content"
	}
	return false, ""
}

func mechanismV1CollectionWarning(entryName string, warning string) string {
	return fmt.Sprintf("mechanism v1 collection entry %q: %s", entryName, warning)
}

func loadSavedMechanismV1(
	mechanismPath string,
) (semanticdiscovery.Mechanism, bool, string) {
	info, err := os.Lstat(mechanismPath)
	if err != nil {
		if os.IsNotExist(err) {
			return semanticdiscovery.Mechanism{}, false, ""
		}
		return semanticdiscovery.Mechanism{}, false,
			"mechanism v1 unavailable: cannot inspect saved object"
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxMechanismV1FileBytes {
		return semanticdiscovery.Mechanism{}, false,
			"mechanism v1 unavailable: saved object is not a bounded regular file"
	}
	raw, err := os.ReadFile(mechanismPath)
	if err != nil {
		return semanticdiscovery.Mechanism{}, false,
			"mechanism v1 unavailable: cannot read saved object"
	}
	mechanism, err := semanticdiscovery.DecodeMechanism(raw)
	if err != nil {
		return semanticdiscovery.Mechanism{}, false,
			fmt.Sprintf("mechanism v1 unavailable: saved object is invalid: %v", err)
	}
	return mechanism, true, ""
}

func replayDecodedMechanismV1(
	data *ReportData,
	mechanism semanticdiscovery.Mechanism,
	factsPath string,
	probePath string,
) string {
	if err := validateMechanismV1Scope(data, mechanism.Identity); err != nil {
		return fmt.Sprintf("mechanism v1 unavailable: object scope is invalid: %v", err)
	}

	previousSupplement := cloneSemanticSupplementFacts(data.SemanticSupplementalFacts)
	previousArtifacts := append([]semanticdiscovery.Artifact(nil), data.SemanticArtifacts...)
	bundle, probe, sourceResult, err := mechanismV1ReplaySourceInputs(
		data,
		mechanism.Payload.Candidate.ID,
		factsPath,
		probePath,
	)
	if err != nil {
		data.SemanticSupplementalFacts = previousSupplement
		data.SemanticArtifacts = previousArtifacts
		return fmt.Sprintf("mechanism v1 unavailable: current inputs are invalid: %v", err)
	}
	artifact, err := semanticdiscovery.ReplayMechanism(bundle, probe, mechanism)
	if err != nil {
		data.SemanticSupplementalFacts = previousSupplement
		data.SemanticArtifacts = previousArtifacts
		return fmt.Sprintf("mechanism v1 unavailable: saved object is stale or invalid: %v", err)
	}

	merged := make([]semanticdiscovery.Artifact, 0, len(previousArtifacts)+1)
	for _, existing := range previousArtifacts {
		if existing.ID == artifact.ID || existing.CandidateID == artifact.CandidateID {
			continue
		}
		merged = append(merged, existing)
	}
	merged = append(merged, artifact)
	sort.Slice(merged, func(i, j int) bool { return merged[i].ID < merged[j].ID })
	data.SemanticArtifacts = merged
	data.StartHereArtifactID = artifact.ID
	if userMechanism, ok := projectUserMechanism(data, artifact, sourceResult); ok {
		if intent := mechanism.Payload.Candidate.ProductIntent; intent != nil {
			userMechanism.OpportunityKind = intent.OpportunityKind
			userMechanism.TargetUserJob = intent.TargetUserJob
			userMechanism.SearchQueries = append([]string(nil), intent.SearchQueries...)
		}
		data.UserMechanisms = mergeUserMechanism(data.UserMechanisms, userMechanism)
	}
	if mechanism.Identity.RepositoryNamespace == "github.com/go-chi/chi/v5" &&
		mechanism.Identity.IntentKey == "chi-request-dispatch" {
		data.SemanticCoverage = &SemanticCoverageSummary{
			OpportunitiesAttempted:       data.semanticAttempted,
			CandidatesInvestigated:       data.semanticInvestigated,
			CanonicalMechanismsPublished: 1,
		}
		status := "partial"
		if artifact.Verdict == semanticdiscovery.VerdictInsufficientEvidence {
			status = "unresolved"
		} else if artifact.Verdict == semanticdiscovery.VerdictSupported &&
			len(artifact.UncoveredAspectIDs) == 0 && len(artifact.Unknowns) == 0 {
			status = "confirmed"
		}
		data.SemanticCoverage.CentralRoutingMechanism = status
	}
	return ""
}

func mechanismV1ExtractionInputs(
	data *ReportData,
	candidateID string,
	factsPath string,
	recordPath string,
	probePath string,
) (
	semanticdiscovery.Bundle,
	semanticdiscovery.Record,
	semanticdiscovery.OpportunityCandidate,
	semanticdiscovery.MechanismProbeInput,
	error,
) {
	bundle, probe, err := mechanismV1ReplayInputs(data, candidateID, factsPath, probePath)
	if err != nil {
		return semanticdiscovery.Bundle{}, semanticdiscovery.Record{},
			semanticdiscovery.OpportunityCandidate{}, semanticdiscovery.MechanismProbeInput{}, err
	}
	record, candidate, err := readMechanismV1Candidate(recordPath, candidateID)
	if err != nil {
		return semanticdiscovery.Bundle{}, semanticdiscovery.Record{},
			semanticdiscovery.OpportunityCandidate{}, semanticdiscovery.MechanismProbeInput{}, err
	}
	return bundle, record, candidate, probe, nil
}

// mechanismV1ReplayInputs loads only the object's bound deterministic facts
// and exact probe. The legacy global semantic Record is intentionally not a
// replay input; it is needed only once by mechanismV1ExtractionInputs.
func mechanismV1ReplayInputs(
	data *ReportData,
	candidateID string,
	factsPath string,
	probePath string,
) (semanticdiscovery.Bundle, semanticdiscovery.MechanismProbeInput, error) {
	bundle, probe, _, err := mechanismV1ReplaySourceInputs(data, candidateID, factsPath, probePath)
	return bundle, probe, err
}

func mechanismV1ReplaySourceInputs(
	data *ReportData,
	candidateID string,
	factsPath string,
	probePath string,
) (
	semanticdiscovery.Bundle,
	semanticdiscovery.MechanismProbeInput,
	goldenmechanism.Result,
	error,
) {
	supplement, err := readDetachedSemanticSupplement(factsPath)
	if err != nil {
		return semanticdiscovery.Bundle{}, semanticdiscovery.MechanismProbeInput{}, goldenmechanism.Result{}, err
	}
	boundProbeSHA, err := semanticSupplementProbeSHA(supplement, candidateID)
	if err != nil {
		return semanticdiscovery.Bundle{}, semanticdiscovery.MechanismProbeInput{}, goldenmechanism.Result{}, err
	}
	probe, sourceResult, err := readMechanismV1ProbeResult(probePath)
	if err != nil {
		return semanticdiscovery.Bundle{}, semanticdiscovery.MechanismProbeInput{}, goldenmechanism.Result{}, err
	}
	if probe.SHA256 != boundProbeSHA {
		return semanticdiscovery.Bundle{}, semanticdiscovery.MechanismProbeInput{}, goldenmechanism.Result{},
			fmt.Errorf("bounded probe digest does not match the candidate binding")
	}

	data.SemanticSupplementalFacts = cloneSemanticSupplementFacts(supplement.Facts)
	bundle, err := BuildSemanticDiscoveryBundle(data)
	if err != nil {
		return semanticdiscovery.Bundle{}, semanticdiscovery.MechanismProbeInput{}, goldenmechanism.Result{}, err
	}
	return bundle, probe, sourceResult, nil
}

func readMechanismV1Candidate(
	recordPath string,
	candidateID string,
) (semanticdiscovery.Record, semanticdiscovery.OpportunityCandidate, error) {
	recordRaw, err := readMechanismV1File(recordPath, maxSavedSemanticDiscoveryRecordBytes)
	if err != nil {
		return semanticdiscovery.Record{}, semanticdiscovery.OpportunityCandidate{}, err
	}
	record, err := semanticdiscovery.DecodeRecord(recordRaw)
	if err != nil {
		return semanticdiscovery.Record{}, semanticdiscovery.OpportunityCandidate{}, err
	}
	var candidate semanticdiscovery.OpportunityCandidate
	for _, item := range record.Opportunity.Candidates {
		if item.ID != candidateID {
			continue
		}
		if candidate.ID != "" {
			return semanticdiscovery.Record{}, semanticdiscovery.OpportunityCandidate{},
				fmt.Errorf("candidate is duplicated")
		}
		candidate = item
	}
	if candidate.ID == "" || candidate.Kind != semanticdiscovery.ArtifactMechanism {
		return semanticdiscovery.Record{}, semanticdiscovery.OpportunityCandidate{},
			fmt.Errorf("bound mechanism candidate is unavailable")
	}
	return record, candidate, nil
}

func validateMechanismV1Scope(
	data *ReportData,
	identity semanticdiscovery.MechanismIdentity,
) error {
	if data == nil || data.RepositoryGraph == nil {
		return fmt.Errorf("semantic mechanism: repository graph is unavailable")
	}
	moduleFound := false
	for _, module := range data.RepositoryGraph.Modules {
		if module.Path == identity.RepositoryNamespace {
			moduleFound = true
			break
		}
	}
	if !moduleFound {
		return fmt.Errorf("semantic mechanism: repository namespace is not owned by this report")
	}
	for _, pkg := range data.RepositoryGraph.Packages {
		if pkg.CanonicalPath == identity.Scope.Value &&
			pkg.ModulePath == identity.RepositoryNamespace &&
			(pkg.Locality == "" || pkg.Locality == "local") {
			return nil
		}
	}
	return fmt.Errorf("semantic mechanism: package scope is not owned by this report")
}

func readDetachedSemanticSupplement(path string) (SemanticSupplement, error) {
	raw, err := readMechanismV1File(path, maxSemanticSupplementFileBytes)
	if err != nil {
		return SemanticSupplement{}, err
	}
	var record SemanticSupplement
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&record); err != nil {
		return SemanticSupplement{}, fmt.Errorf("saved facts contain invalid json")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return SemanticSupplement{}, fmt.Errorf("saved facts contain trailing json")
	}
	if err := validateSemanticSupplement(record); err != nil {
		return SemanticSupplement{}, err
	}
	return record, nil
}

func semanticSupplementProbeSHA(record SemanticSupplement, candidateID string) (string, error) {
	if record.Version == semanticSupplementLegacyVersion {
		if record.CandidateID != candidateID {
			return "", fmt.Errorf("candidate is not bound by saved facts")
		}
		return record.ProbeSHA256, nil
	}
	for _, binding := range record.CandidateBindings {
		if binding.CandidateID == candidateID {
			return binding.ProbeSHA256, nil
		}
	}
	return "", fmt.Errorf("candidate is not bound by saved facts")
}

func readMechanismV1Probe(path string) (semanticdiscovery.MechanismProbeInput, error) {
	probe, _, err := readMechanismV1ProbeResult(path)
	return probe, err
}

func readMechanismV1ProbeResult(
	path string,
) (semanticdiscovery.MechanismProbeInput, goldenmechanism.Result, error) {
	raw, err := readMechanismV1File(path, maxMechanismV1FileBytes)
	if err != nil {
		return semanticdiscovery.MechanismProbeInput{}, goldenmechanism.Result{}, err
	}
	var result goldenmechanism.Result
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return semanticdiscovery.MechanismProbeInput{}, goldenmechanism.Result{}, fmt.Errorf("bounded probe contains invalid json")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return semanticdiscovery.MechanismProbeInput{}, goldenmechanism.Result{}, fmt.Errorf("bounded probe contains trailing json")
	}
	if err := result.Validate(); err != nil {
		return semanticdiscovery.MechanismProbeInput{}, goldenmechanism.Result{}, err
	}
	digest := sha256.Sum256(raw)
	return semanticdiscovery.MechanismProbeInput{
		ContractVersion: result.Version,
		ID:              result.MechanismID,
		SHA256:          hex.EncodeToString(digest[:]),
	}, result, nil
}

func readMechanismV1File(path string, limit int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > limit {
		return nil, fmt.Errorf("%s is not a bounded regular file", filepath.Base(path))
	}
	return os.ReadFile(path)
}
