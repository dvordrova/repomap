package runtimeportfolio

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func TestCompactReduceStateBindsHiddenExactCandidateAuthority(t *testing.T) {
	input := oversizedRuntimePortfolioInput(1, 2)
	compilation := mustCompile(t, input)
	roles := make([]Role, 0, 2)
	for responsibilityIndex := range 2 {
		role, err := restoreRole(responseRole{
			Name: "Distinct runtime", Purpose: "Runs one distinct repository role.",
			Prominence: ProminenceUnknown, Kind: RoleKindService,
			Requiredness: RequirednessUnknown, Confidence: ConfidenceHigh,
			MappingStatus:   MappingMapped,
			Implementations: []responseImplementation{{TargetRef: "t1"}},
			EvidenceRefs: []string{evidenceRefForLabel(
				t, compilation,
				fmt.Sprintf("Evidence 00 %04d %s", responsibilityIndex, strings.Repeat("e", 440)),
			)},
		}, compilation.targetsByRef, compilation.evidenceByRef)
		if err != nil {
			t.Fatal(err)
		}
		roles = append(roles, role)
	}

	firstBatches, err := packReduceRequests(compilation, 2, []Role{roles[0]}, MaxRequestBytes)
	if err != nil {
		t.Fatal(err)
	}
	secondBatches, err := packReduceRequests(compilation, 2, []Role{roles[1]}, MaxRequestBytes)
	if err != nil {
		t.Fatal(err)
	}
	if len(firstBatches) != 1 || len(secondBatches) != 1 ||
		!bytes.Equal(firstBatches[0].wire, secondBatches[0].wire) {
		t.Fatal("same compact summary produced different provider wire")
	}
	if bytes.Contains(firstBatches[0].wire, []byte("candidate_authority_sha256")) {
		t.Fatal("local candidate authority digest leaked into provider wire")
	}

	firstDigest, err := candidateAuthoritySHA256(firstBatches[0].candidatesByRef)
	if err != nil {
		t.Fatal(err)
	}
	secondDigest, err := candidateAuthoritySHA256(secondBatches[0].candidatesByRef)
	if err != nil {
		t.Fatal(err)
	}
	if firstDigest == secondDigest {
		t.Fatal("changed hidden exact evidence did not change candidate authority digest")
	}
	firstState, err := runtimeCallState(
		compilation, "reduce", 2, 1, 1, firstBatches[0].wire, reducePrompt, firstDigest,
	)
	if err != nil {
		t.Fatal(err)
	}
	secondState, err := runtimeCallState(
		compilation, "reduce", 2, 1, 1, secondBatches[0].wire, reducePrompt, secondDigest,
	)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(firstState, secondState) {
		t.Fatal("changed hidden exact evidence reused compact reduce cache state")
	}

	forward := map[string]Role{"c1": roles[0], "c2": roles[1]}
	reversed := make(map[string]Role, 2)
	reversed["c2"] = roles[1]
	reversed["c1"] = roles[0]
	forwardDigest, err := candidateAuthoritySHA256(forward)
	if err != nil {
		t.Fatal(err)
	}
	reversedDigest, err := candidateAuthoritySHA256(reversed)
	if err != nil {
		t.Fatal(err)
	}
	if forwardDigest != reversedDigest {
		t.Fatal("candidate authority digest depends on map insertion order")
	}
}
