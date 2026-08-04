package report

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/goldenmechanism"
	"github.com/dvordrova/repomap/internal/semanticdiscovery"
)

func TestValidateMechanismV1ScopeRequiresOwnedModuleAndPackage(t *testing.T) {
	t.Parallel()

	identity := semanticdiscovery.MechanismIdentity{
		RepositoryNamespace: "example.com/repository/v2",
		IntentKey:           "directory-listing",
		Scope: semanticdiscovery.MechanismScope{
			Kind:  semanticdiscovery.MechanismScopeGoPackage,
			Value: "example.com/repository/v2/fileserver",
		},
	}
	data := &ReportData{RepositoryGraph: &RepositoryGraph{
		Modules: []ModuleInfo{{Path: identity.RepositoryNamespace}},
		Packages: []PackageInfo{{
			CanonicalPath: identity.Scope.Value,
			ModulePath:    identity.RepositoryNamespace,
			Locality:      "local",
		}},
	}}
	if err := validateMechanismV1Scope(data, identity); err != nil {
		t.Fatal(err)
	}

	wrongRepository := identity
	wrongRepository.RepositoryNamespace = "example.com/other"
	if err := validateMechanismV1Scope(data, wrongRepository); err == nil ||
		!strings.Contains(err.Error(), "namespace") {
		t.Fatalf("wrong repository error = %v", err)
	}

	wrongPackage := identity
	wrongPackage.Scope.Value = "example.com/repository/v2/other"
	if err := validateMechanismV1Scope(data, wrongPackage); err == nil ||
		!strings.Contains(err.Error(), "package scope") {
		t.Fatalf("wrong package error = %v", err)
	}
}

func TestReplaySavedMechanismV1CollectionPreservesRootAndContinuesAfterInvalidEntry(t *testing.T) {
	t.Parallel()

	runDir := t.TempDir()
	data := mechanismV1CollectionTestData()
	data.StartHereArtifactID = "existing-selection"
	rootMechanism, rootArtifact := writeMechanismV1TestEntry(t, data, runDir, "root", true)
	if warning := replaySavedMechanismV1(
		data,
		filepath.Join(runDir, semanticdiscovery.MechanismFile),
		filepath.Join(runDir, GoldenMechanismFactsFile),
		filepath.Join(runDir, GoldenMechanismProbeFile),
	); warning != "" {
		t.Fatal(warning)
	}
	if data.StartHereArtifactID != "existing-selection" {
		t.Fatalf("root replay changed Start Here to %q", data.StartHereArtifactID)
	}
	rootSupplement := cloneSemanticSupplementFacts(data.SemanticSupplementalFacts)

	duplicateDir := filepath.Join(
		MechanismV1CollectionPath(runDir),
		rootMechanism.Payload.Candidate.ID,
	)
	copyMechanismV1TestEntry(t, runDir, duplicateDir)
	brokenMechanism, _ := writeMechanismV1TestEntry(t, data, runDir, "broken", false)
	brokenDir := filepath.Join(
		MechanismV1CollectionPath(runDir),
		brokenMechanism.Payload.Candidate.ID,
	)
	if err := os.Remove(filepath.Join(brokenDir, GoldenMechanismFactsFile)); err != nil {
		t.Fatal(err)
	}
	_, addedArtifact := writeMechanismV1TestEntry(t, data, runDir, "added", false)
	data.SemanticSupplementalFacts = cloneSemanticSupplementFacts(rootSupplement)

	warnings := replaySavedMechanismV1Collection(data, runDir)
	if len(warnings) != 1 ||
		!strings.Contains(warnings[0], brokenMechanism.Payload.Candidate.ID) ||
		!strings.Contains(warnings[0], "current inputs are invalid") {
		t.Fatalf("collection warnings = %#v", warnings)
	}
	if data.StartHereArtifactID != "existing-selection" {
		t.Fatalf("collection changed Start Here to %q", data.StartHereArtifactID)
	}
	if !reflect.DeepEqual(data.SemanticSupplementalFacts, rootSupplement) {
		t.Fatal("collection leaked entry-local supplemental facts")
	}
	if len(data.SemanticArtifacts) != 2 {
		t.Fatalf("semantic artifacts = %#v, want root plus one collection artifact", data.SemanticArtifacts)
	}
	assertMechanismV1ArtifactCount(t, data.SemanticArtifacts, rootArtifact.ID, 1)
	assertMechanismV1ArtifactCount(t, data.SemanticArtifacts, addedArtifact.ID, 1)
}

func TestReplaySavedMechanismV1CollectionDoesNotSelectStartHere(t *testing.T) {
	t.Parallel()

	runDir := t.TempDir()
	data := mechanismV1CollectionTestData()
	_, artifact := writeMechanismV1TestEntry(t, data, runDir, "only", false)
	data.SemanticSupplementalFacts = nil

	if warnings := replaySavedMechanismV1Collection(data, runDir); len(warnings) != 0 {
		t.Fatalf("collection warnings = %#v", warnings)
	}
	if data.StartHereArtifactID != "" {
		t.Fatalf("collection selected Start Here %q", data.StartHereArtifactID)
	}
	assertMechanismV1ArtifactCount(t, data.SemanticArtifacts, artifact.ID, 1)
}

func TestReplaySavedMechanismV1CollectionRejectsNonDirectoryEntry(t *testing.T) {
	t.Parallel()

	runDir := t.TempDir()
	collectionDir := MechanismV1CollectionPath(runDir)
	if err := os.MkdirAll(collectionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(collectionDir, "not-an-entry"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	warnings := replaySavedMechanismV1Collection(mechanismV1CollectionTestData(), runDir)
	if len(warnings) != 1 || !strings.Contains(warnings[0], "entry is not a directory") {
		t.Fatalf("collection warnings = %#v", warnings)
	}
}

func mechanismV1CollectionTestData() *ReportData {
	const module = "example.com/repository"
	const pkg = module + "/core"
	return &ReportData{
		RepoName: "repository",
		RepositoryGraph: &RepositoryGraph{
			Modules: []ModuleInfo{{Path: module}},
			Packages: []PackageInfo{{
				CanonicalPath: pkg,
				ModulePath:    module,
				Locality:      "local",
			}},
			PackageEdges: []EdgeInfo{{From: pkg, To: module + "/adapter"}},
		},
	}
}

func writeMechanismV1TestEntry(
	t *testing.T,
	data *ReportData,
	runDir string,
	suffix string,
	root bool,
) (semanticdiscovery.Mechanism, semanticdiscovery.Artifact) {
	t.Helper()
	data.SemanticSupplementalFacts = nil
	facts := []semanticdiscovery.Fact{
		mechanismV1TestFact(
			"fact-"+suffix+"-input",
			"The public operation accepts a request",
			"input",
			semanticdiscovery.CapabilityBehavior,
		),
		mechanismV1TestFact(
			"fact-"+suffix+"-work",
			"The core worker reads stored state",
			"work",
			semanticdiscovery.CapabilityDataRead,
		),
		mechanismV1TestFact(
			"fact-"+suffix+"-effect",
			"The result writer emits a response",
			"effect",
			semanticdiscovery.CapabilityOutputEffect,
		),
	}
	data.SemanticSupplementalFacts = cloneSemanticSupplementFacts(facts)
	bundle, err := BuildSemanticDiscoveryBundle(data)
	if err != nil {
		t.Fatal(err)
	}
	rawOpportunity := semanticdiscovery.OpportunityProposal{
		Version: semanticdiscovery.OpportunityProposalVersion,
		Candidates: []semanticdiscovery.OpportunityCandidate{{
			Kind:             semanticdiscovery.ArtifactMechanism,
			Title:            "Public operation response " + suffix,
			QuestionAnswered: "How does the public operation read state and emit a response " + suffix + "?",
			SupportIDs:       []string{facts[0].ID, facts[1].ID, facts[2].ID},
			ExpectedValue:    semanticdiscovery.ExpectedValueHigh,
			Confidence:       semanticdiscovery.ConfidenceHigh,
		}},
	}
	opportunity, _ := semanticdiscovery.NormalizeOpportunityProposal(bundle, rawOpportunity)
	if len(opportunity.Candidates) != 1 {
		t.Fatalf("normalized opportunity = %#v", opportunity)
	}
	candidate := opportunity.Candidates[0]
	candidate.CapabilityContract = &semanticdiscovery.CapabilityContract{
		RequiredCapabilities: []semanticdiscovery.Capability{
			semanticdiscovery.CapabilityBehavior,
			semanticdiscovery.CapabilityDataRead,
			semanticdiscovery.CapabilityOutputEffect,
		},
		AvailableCapabilities: []semanticdiscovery.Capability{
			semanticdiscovery.CapabilityBehavior,
			semanticdiscovery.CapabilityDataRead,
			semanticdiscovery.CapabilityOutputEffect,
		},
		Resolution: semanticdiscovery.CapabilityResolutionReady,
	}
	candidate.IntentContract = &semanticdiscovery.IntentContract{
		RequiredAnswerAspects: []semanticdiscovery.AnswerAspect{
			{ID: "input", Label: "Public input", RequiredCapabilities: []semanticdiscovery.Capability{semanticdiscovery.CapabilityBehavior}, Key: true},
			{ID: "work", Label: "Core work", RequiredCapabilities: []semanticdiscovery.Capability{semanticdiscovery.CapabilityDataRead}, Key: true},
			{ID: "effect", Label: "External effect", RequiredCapabilities: []semanticdiscovery.Capability{semanticdiscovery.CapabilityOutputEffect}, Key: true},
		},
		MinCovered:         3,
		MinKeyCovered:      3,
		LocalSearchAliases: []string{"public operation response " + suffix},
	}
	opportunity.Candidates[0] = candidate
	if err := semanticdiscovery.ValidateOpportunityProposal(bundle, opportunity); err != nil {
		t.Fatal(err)
	}
	tasks, err := semanticdiscovery.PlanLeafTasks(bundle, []semanticdiscovery.OpportunityCandidate{candidate})
	if err != nil {
		t.Fatal(err)
	}
	task := tasks[0]
	observations := make([]semanticdiscovery.LeafObservation, 0, len(facts))
	claims := make([]semanticdiscovery.ProposedClaim, 0, len(facts))
	for index, fact := range facts {
		observations = append(observations, semanticdiscovery.LeafObservation{
			Text: fact.Statement, SupportIDs: []string{fact.ID},
		})
		claims = append(claims, semanticdiscovery.ProposedClaim{
			Title:      []string{"Accept input", "Read state", "Emit response"}[index],
			Text:       fact.Statement,
			Basis:      semanticdiscovery.ClaimDirect,
			SupportIDs: []string{fact.ID},
			ObservationRefs: []semanticdiscovery.ObservationRef{{
				TaskID: task.ID, ObservationIndex: index,
			}},
		})
	}
	leaf := semanticdiscovery.LeafResult{
		Task: task,
		Artifact: semanticdiscovery.LeafArtifact{
			Version:      semanticdiscovery.LeafArtifactVersion,
			TaskID:       task.ID,
			CandidateID:  candidate.ID,
			Status:       semanticdiscovery.LeafStatusUsable,
			Observations: observations,
			CandidateConnection: semanticdiscovery.LeafCandidateConnection{
				CandidateID: candidate.ID,
				Relation:    "needs_combination",
				Explanation: "The supported observations combine into the requested mechanism",
				SupportIDs:  append([]string(nil), candidate.SupportIDs...),
			},
		},
	}
	if err := semanticdiscovery.ValidateLeafArtifact(task, leaf.Artifact); err != nil {
		t.Fatal(err)
	}
	proposal := semanticdiscovery.FanInArtifact{
		Version: semanticdiscovery.FanInArtifactVersion,
		Artifacts: []semanticdiscovery.ArtifactProposal{{
			CandidateID:     candidate.ID,
			Verdict:         semanticdiscovery.VerdictSupported,
			Title:           candidate.Title,
			Summary:         facts[0].Statement + ". " + facts[1].Statement + ". " + facts[2].Statement + ".",
			Claims:          claims,
			Aliases:         append([]string(nil), candidate.IntentContract.LocalSearchAliases...),
			LikelyQuestions: []string{candidate.QuestionAnswered},
		}},
	}
	probeRaw, probe := mechanismV1TestProbe(t, "intent-"+suffix, facts)
	data.SemanticSupplementalFacts = nil
	supplement, enriched, err := PrepareSemanticSupplement(
		data,
		candidate.ID,
		probe.SHA256,
		facts,
	)
	if err != nil {
		t.Fatal(err)
	}
	recordRaw, err := semanticdiscovery.EncodeRecord(
		enriched,
		opportunity,
		[]semanticdiscovery.OpportunityCandidate{candidate},
		[]semanticdiscovery.LeafResult{leaf},
		proposal,
	)
	if err != nil {
		t.Fatal(err)
	}
	record, err := semanticdiscovery.DecodeRecord(recordRaw)
	if err != nil {
		t.Fatal(err)
	}
	identity := semanticdiscovery.MechanismIdentity{
		RepositoryNamespace: "example.com/repository",
		IntentKey:           probe.ID,
		Scope: semanticdiscovery.MechanismScope{
			Kind:  semanticdiscovery.MechanismScopeGoPackage,
			Value: "example.com/repository/core",
		},
	}
	mechanism, artifact, err := semanticdiscovery.ExtractMechanism(
		enriched,
		record,
		candidate.ID,
		identity,
		probe,
	)
	if err != nil {
		t.Fatal(err)
	}
	entryDir := runDir
	if !root {
		entryDir = filepath.Join(MechanismV1CollectionPath(runDir), candidate.ID)
	}
	if err := os.MkdirAll(entryDir, 0o700); err != nil {
		t.Fatal(err)
	}
	mechanismRaw, err := semanticdiscovery.EncodeMechanism(mechanism)
	if err != nil {
		t.Fatal(err)
	}
	writeMechanismV1TestFile(t, filepath.Join(entryDir, semanticdiscovery.MechanismFile), mechanismRaw)
	supplementRaw, err := json.Marshal(supplement)
	if err != nil {
		t.Fatal(err)
	}
	writeMechanismV1TestFile(t, filepath.Join(entryDir, GoldenMechanismFactsFile), supplementRaw)
	writeMechanismV1TestFile(t, filepath.Join(entryDir, GoldenMechanismProbeFile), probeRaw)
	data.SemanticSupplementalFacts = nil
	return mechanism, artifact
}

func mechanismV1TestFact(
	id string,
	statement string,
	aspect string,
	capability semanticdiscovery.Capability,
) semanticdiscovery.Fact {
	return semanticdiscovery.Fact{
		ID:           id,
		Kind:         semanticdiscovery.FactSourceSignal,
		Statement:    statement,
		Keywords:     []string{"answer_aspect:" + aspect},
		SourceGroup:  "group-" + id,
		Capabilities: []semanticdiscovery.Capability{capability},
		Scope:        semanticdiscovery.FactScopeLocal,
	}
}

func mechanismV1TestProbe(
	t *testing.T,
	intent string,
	facts []semanticdiscovery.Fact,
) ([]byte, semanticdiscovery.MechanismProbeInput) {
	t.Helper()
	seeds := make([]goldenmechanism.SeedResolution, 0, len(facts))
	for index, fact := range facts {
		seeds = append(seeds, goldenmechanism.SeedResolution{
			Seed: goldenmechanism.Seed{
				OriginFactID:     fact.ID,
				OriginEvidenceID: "evidence-" + fact.ID,
				Path:             "source.go",
				Symbol:           []string{"Accept", "Read", "Emit"}[index],
			},
			Status: goldenmechanism.SeedSkippedTimeout,
		})
	}
	result := goldenmechanism.Result{
		Version:     goldenmechanism.Version,
		MechanismID: intent,
		Seeds:       seeds,
		Budget: goldenmechanism.BudgetStats{
			SeedCount: len(seeds),
		},
		Partial:    true,
		StopReason: goldenmechanism.StopTimeout,
	}
	if err := result.Validate(); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	return raw, semanticdiscovery.MechanismProbeInput{
		ContractVersion: goldenmechanism.Version,
		ID:              intent,
		SHA256:          hex.EncodeToString(digest[:]),
	}
}

func copyMechanismV1TestEntry(t *testing.T, sourceDir string, targetDir string) {
	t.Helper()
	if err := os.MkdirAll(targetDir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		semanticdiscovery.MechanismFile,
		GoldenMechanismFactsFile,
		GoldenMechanismProbeFile,
	} {
		raw, err := os.ReadFile(filepath.Join(sourceDir, name))
		if err != nil {
			t.Fatal(err)
		}
		writeMechanismV1TestFile(t, filepath.Join(targetDir, name), raw)
	}
}

func writeMechanismV1TestFile(t *testing.T, path string, raw []byte) {
	t.Helper()
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertMechanismV1ArtifactCount(
	t *testing.T,
	artifacts []semanticdiscovery.Artifact,
	id string,
	want int,
) {
	t.Helper()
	count := 0
	for _, artifact := range artifacts {
		if artifact.ID == id {
			count++
		}
	}
	if count != want {
		t.Fatalf("artifact %q count = %d, want %d", id, count, want)
	}
}
