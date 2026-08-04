package semanticmap

import (
	"crypto/sha256"
	"fmt"
	"os"
	"strings"
	"testing"
)

const (
	maxCompactEvidenceBytes = 8 << 10
	maxCompactTextBytes     = 240
	maxCompactFacts         = 8
	maxCompactHypotheses    = 4
	maxCompactUnknowns      = 3
	maxCompactRefs          = 4

	caddyCompactRejectedHash = "58257522d4043b8ad9b6276d48cab68ca205683bffc32b676be7562ff424195f"
)

type compactEvidence struct {
	Facts      []compactFact       `json:"facts"`
	Hypotheses []compactHypothesis `json:"hypotheses"`
	Unknowns   []compactUnknown    `json:"unknowns"`
}

type compactFact struct {
	ID   string `json:"id"`
	Text string `json:"text"`
	At   []int  `json:"at"`
}

type compactHypothesis struct {
	ID   string   `json:"id"`
	Text string   `json:"text"`
	Refs []string `json:"refs"`
}

type compactUnknown struct {
	ID   string   `json:"id"`
	Text string   `json:"text"`
	Refs []string `json:"refs"`
}

func TestCompactFactSelfClassificationStoppedAtCaddyNegativeFixture(t *testing.T) {
	packetBytes := readBoundedFile(t, "caddy.source-slices.json", maxSourcePacketBytes)
	responseBytes := readBoundedFile(
		t,
		"caddy.compact-evidence.response.json",
		maxCompactEvidenceBytes,
	)
	if got := fmt.Sprintf("%x", sha256.Sum256(responseBytes)); got != caddyCompactRejectedHash {
		t.Fatalf("Caddy compact negative fixture hash = %s, want %s", got, caddyCompactRejectedHash)
	}

	packet := decodeStrict[sourcePacket](t, packetBytes)
	evidence := decodeStrict[compactEvidence](t, responseBytes)
	validateSourcePacket(t, packet)
	validateCompactEvidenceFixture(t, packet, evidence)
	validateCompactEvidenceNoPaths(t, responseBytes, packet)

	for _, absent := range []string{
		"caddy.compact-map.response.json",
		"beets.compact-evidence.response.json",
		"beets.compact-map.response.json",
	} {
		if _, err := os.Stat(absent); !os.IsNotExist(err) {
			t.Fatalf("%s must remain absent after stopped experiment; stat error = %v", absent, err)
		}
	}
}

func validateCompactEvidenceFixture(
	t *testing.T,
	packet sourcePacket,
	evidence compactEvidence,
) {
	t.Helper()
	if len(evidence.Facts) < 4 || len(evidence.Facts) > maxCompactFacts {
		t.Fatalf("facts count = %d, want 4..%d", len(evidence.Facts), maxCompactFacts)
	}
	if len(evidence.Hypotheses) == 0 || len(evidence.Hypotheses) > maxCompactHypotheses {
		t.Fatalf("hypotheses count = %d, want 1..%d", len(evidence.Hypotheses), maxCompactHypotheses)
	}
	if len(evidence.Unknowns) == 0 || len(evidence.Unknowns) > maxCompactUnknowns {
		t.Fatalf("unknowns count = %d, want 1..%d", len(evidence.Unknowns), maxCompactUnknowns)
	}

	allIDs := make(map[string]struct{}, len(evidence.Facts)+len(evidence.Hypotheses)+len(evidence.Unknowns))
	factIDs := make(map[string]struct{}, len(evidence.Facts))
	for i, fact := range evidence.Facts {
		prefix := fmt.Sprintf("facts[%d]", i)
		validateCompactFixtureID(t, prefix+".id", fact.ID, "f", allIDs)
		validateText(t, prefix+".text", fact.Text, maxCompactTextBytes)
		if len(fact.At) != 3 {
			t.Fatalf("%s.at count = %d, want 3", prefix, len(fact.At))
		}
		sliceIndex, startLine, endLine := fact.At[0], fact.At[1], fact.At[2]
		if sliceIndex < 0 || sliceIndex >= len(packet.SourceSlices) {
			t.Fatalf("%s.at slice index = %d, want 0..%d", prefix, sliceIndex, len(packet.SourceSlices)-1)
		}
		slice := packet.SourceSlices[sliceIndex]
		if startLine < slice.StartLine || endLine < startLine || endLine > slice.EndLine {
			t.Fatalf("%s.at lines = %d..%d, want inside %d..%d", prefix, startLine, endLine, slice.StartLine, slice.EndLine)
		}
		factIDs[fact.ID] = struct{}{}
	}
	for i, hypothesis := range evidence.Hypotheses {
		prefix := fmt.Sprintf("hypotheses[%d]", i)
		validateCompactFixtureID(t, prefix+".id", hypothesis.ID, "h", allIDs)
		validateText(t, prefix+".text", hypothesis.Text, maxCompactTextBytes)
		validateCompactFixtureReferences(t, prefix+".refs", hypothesis.Refs, factIDs)
	}
	for i, unknown := range evidence.Unknowns {
		prefix := fmt.Sprintf("unknowns[%d]", i)
		validateCompactFixtureID(t, prefix+".id", unknown.ID, "u", allIDs)
		validateText(t, prefix+".text", unknown.Text, maxCompactTextBytes)
		validateCompactFixtureReferences(t, prefix+".refs", unknown.Refs, factIDs)
	}
}

func validateCompactFixtureID(
	t *testing.T,
	field, id, prefix string,
	known map[string]struct{},
) {
	t.Helper()
	validateID(t, field, id)
	if !strings.HasPrefix(id, prefix) {
		t.Fatalf("%s = %q, want prefix %q", field, id, prefix)
	}
	if _, duplicate := known[id]; duplicate {
		t.Fatalf("%s duplicates evidence id %q", field, id)
	}
	known[id] = struct{}{}
}

func validateCompactFixtureReferences(
	t *testing.T,
	field string,
	references []string,
	known map[string]struct{},
) {
	t.Helper()
	if len(references) == 0 || len(references) > maxCompactRefs {
		t.Fatalf("%s count = %d, want 1..%d", field, len(references), maxCompactRefs)
	}
	validateEntityReferences(t, field, references, known)
}

func validateCompactEvidenceNoPaths(t *testing.T, response []byte, packet sourcePacket) {
	t.Helper()
	text := string(response)
	for _, forbidden := range []string{"/Users/", "/home/", `C:\`, "file://"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("compact evidence contains absolute source marker %q", forbidden)
		}
	}
	for _, slice := range packet.SourceSlices {
		if strings.Contains(text, slice.Path) {
			t.Fatalf("compact evidence contains source path %q instead of slice index", slice.Path)
		}
	}
}
