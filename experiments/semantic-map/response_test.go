package semanticmap

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"
)

const (
	maxInputBytes          = 16 << 10
	maxResponseBytes       = 64 << 10
	maxObservations        = 18
	maxNodes               = 8
	maxEdges               = 12
	maxReferences          = 18
	maxUnknowns            = 6
	maxIDBytes             = 80
	maxPathBytes           = 240
	maxQuestionBytes       = 500
	maxObservationBytes    = 1000
	maxLabelBytes          = 60
	maxResponsibilityBytes = 240
	maxVerbBytes           = 120
	maxSummaryBytes        = 800
	maxUnknownBytes        = 500
)

var (
	idPattern       = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	revisionPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)
)

type observationInput struct {
	CaseID       string        `json:"case_id"`
	Repository   repository    `json:"repository"`
	Question     string        `json:"question"`
	Observations []observation `json:"observations"`
}

type repository struct {
	Name     string `json:"name"`
	Revision string `json:"revision"`
}

type observation struct {
	ID     string `json:"id"`
	State  string `json:"state"`
	Text   string `json:"text"`
	Source source `json:"source"`
}

type source struct {
	Path      string `json:"path"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
}

type semanticMap struct {
	Nodes           []node          `json:"nodes"`
	Edges           []edge          `json:"edges"`
	QuestionOverlay questionOverlay `json:"question_overlay"`
}

type node struct {
	ID             string   `json:"id"`
	Label          string   `json:"label"`
	Responsibility string   `json:"responsibility"`
	State          string   `json:"state"`
	ObservationIDs []string `json:"observation_ids"`
}

type edge struct {
	ID             string   `json:"id"`
	From           string   `json:"from"`
	To             string   `json:"to"`
	Verb           string   `json:"verb"`
	State          string   `json:"state"`
	ObservationIDs []string `json:"observation_ids"`
}

type questionOverlay struct {
	Summary  string   `json:"summary"`
	NodeIDs  []string `json:"node_ids"`
	EdgeIDs  []string `json:"edge_ids"`
	Unknowns []string `json:"unknowns"`
}

func TestRecordedSemanticMapsStayWithinExperimentContract(t *testing.T) {
	for _, name := range []string{"caddy", "beets"} {
		t.Run(name, func(t *testing.T) {
			inputPath := name + ".observations.json"
			responsePath := name + ".response.json"

			inputBytes := readBoundedFile(t, inputPath, maxInputBytes)
			responseBytes := readBoundedFile(t, responsePath, maxResponseBytes)
			input := decodeStrict[observationInput](t, inputBytes)
			response := decodeStrict[semanticMap](t, responseBytes)

			observations := validateInput(t, input)
			validateResponse(t, response, observations)
			validateNoSourceLeak(t, responseBytes, input)
		})
	}
}

func readBoundedFile(t *testing.T, filename string, limit int) []byte {
	t.Helper()
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 || len(data) > limit {
		t.Fatalf("%s size = %d, want 1..%d bytes", filename, len(data), limit)
	}
	return data
}

func decodeStrict[T any](t *testing.T, data []byte) T {
	t.Helper()
	var value T
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		t.Fatalf("JSON contains trailing data: %v", err)
	}
	return value
}

func validateInput(t *testing.T, input observationInput) map[string]struct{} {
	t.Helper()
	validateID(t, "case_id", input.CaseID)
	validateText(t, "repository.name", input.Repository.Name, maxIDBytes)
	if !revisionPattern.MatchString(input.Repository.Revision) {
		t.Fatalf("repository.revision = %q, want lowercase 40-byte commit", input.Repository.Revision)
	}
	validateText(t, "question", input.Question, maxQuestionBytes)
	if len(input.Observations) == 0 || len(input.Observations) > maxObservations {
		t.Fatalf("observations count = %d, want 1..%d", len(input.Observations), maxObservations)
	}

	known := make(map[string]struct{}, len(input.Observations))
	for i, observation := range input.Observations {
		prefix := fmt.Sprintf("observations[%d]", i)
		validateID(t, prefix+".id", observation.ID)
		if _, duplicate := known[observation.ID]; duplicate {
			t.Fatalf("duplicate observation id %q", observation.ID)
		}
		known[observation.ID] = struct{}{}
		if observation.State != "extracted" {
			t.Fatalf("%s.state = %q, want extracted", prefix, observation.State)
		}
		validateText(t, prefix+".text", observation.Text, maxObservationBytes)
		validateSource(t, prefix+".source", observation.Source)
	}
	return known
}

func validateSource(t *testing.T, field string, source source) {
	t.Helper()
	validateText(t, field+".path", source.Path, maxPathBytes)
	if path.IsAbs(source.Path) ||
		path.Clean(source.Path) != source.Path ||
		source.Path == "." ||
		strings.HasPrefix(source.Path, "../") ||
		strings.Contains(source.Path, `\`) ||
		strings.Contains(source.Path, ":") {
		t.Fatalf("%s.path = %q, want canonical repository-relative path", field, source.Path)
	}
	if source.StartLine < 1 || source.EndLine < source.StartLine || source.EndLine-source.StartLine > 200 {
		t.Fatalf("%s lines = %d..%d, want a positive bounded range", field, source.StartLine, source.EndLine)
	}
}

func validateResponse(t *testing.T, response semanticMap, observations map[string]struct{}) {
	t.Helper()
	if len(response.Nodes) < 3 || len(response.Nodes) > maxNodes {
		t.Fatalf("nodes count = %d, want 3..%d", len(response.Nodes), maxNodes)
	}
	if len(response.Edges) < 2 || len(response.Edges) > maxEdges {
		t.Fatalf("edges count = %d, want 2..%d", len(response.Edges), maxEdges)
	}

	nodes := make(map[string]struct{}, len(response.Nodes))
	for i, node := range response.Nodes {
		prefix := fmt.Sprintf("nodes[%d]", i)
		validateID(t, prefix+".id", node.ID)
		if _, duplicate := nodes[node.ID]; duplicate {
			t.Fatalf("duplicate node id %q", node.ID)
		}
		nodes[node.ID] = struct{}{}
		validateText(t, prefix+".label", node.Label, maxLabelBytes)
		validateText(t, prefix+".responsibility", node.Responsibility, maxResponsibilityBytes)
		validateState(t, prefix+".state", node.State)
		validateReferences(t, prefix+".observation_ids", node.ObservationIDs, observations)
	}

	edges := make(map[string]struct{}, len(response.Edges))
	for i, edge := range response.Edges {
		prefix := fmt.Sprintf("edges[%d]", i)
		validateID(t, prefix+".id", edge.ID)
		if _, duplicate := edges[edge.ID]; duplicate {
			t.Fatalf("duplicate edge id %q", edge.ID)
		}
		edges[edge.ID] = struct{}{}
		if _, ok := nodes[edge.From]; !ok {
			t.Fatalf("%s.from = %q, want returned node", prefix, edge.From)
		}
		if _, ok := nodes[edge.To]; !ok {
			t.Fatalf("%s.to = %q, want returned node", prefix, edge.To)
		}
		validateText(t, prefix+".verb", edge.Verb, maxVerbBytes)
		lowerVerb := strings.ToLower(edge.Verb)
		if strings.Contains(lowerVerb, "package import") || lowerVerb == "imports" {
			t.Fatalf("%s.verb = %q, package-import graph is outside the experiment", prefix, edge.Verb)
		}
		validateState(t, prefix+".state", edge.State)
		validateReferences(t, prefix+".observation_ids", edge.ObservationIDs, observations)
	}

	validateText(t, "question_overlay.summary", response.QuestionOverlay.Summary, maxSummaryBytes)
	validateEntityReferences(t, "question_overlay.node_ids", response.QuestionOverlay.NodeIDs, nodes)
	validateEntityReferences(t, "question_overlay.edge_ids", response.QuestionOverlay.EdgeIDs, edges)
	if len(response.QuestionOverlay.Unknowns) == 0 || len(response.QuestionOverlay.Unknowns) > maxUnknowns {
		t.Fatalf("question_overlay.unknowns count = %d, want 1..%d", len(response.QuestionOverlay.Unknowns), maxUnknowns)
	}
	for i, unknown := range response.QuestionOverlay.Unknowns {
		validateText(t, fmt.Sprintf("question_overlay.unknowns[%d]", i), unknown, maxUnknownBytes)
	}
}

func validateState(t *testing.T, field, state string) {
	t.Helper()
	switch state {
	case "supported", "inferred", "unknown":
	default:
		t.Fatalf("%s = %q, want supported, inferred, or unknown", field, state)
	}
}

func validateReferences(t *testing.T, field string, references []string, known map[string]struct{}) {
	t.Helper()
	if len(references) == 0 || len(references) > maxReferences {
		t.Fatalf("%s count = %d, want 1..%d", field, len(references), maxReferences)
	}
	validateEntityReferences(t, field, references, known)
}

func validateEntityReferences(t *testing.T, field string, references []string, known map[string]struct{}) {
	t.Helper()
	if len(references) == 0 || len(references) > maxReferences {
		t.Fatalf("%s count = %d, want 1..%d", field, len(references), maxReferences)
	}
	seen := make(map[string]struct{}, len(references))
	for _, reference := range references {
		if _, duplicate := seen[reference]; duplicate {
			t.Fatalf("%s contains duplicate %q", field, reference)
		}
		seen[reference] = struct{}{}
		if _, ok := known[reference]; !ok {
			t.Fatalf("%s contains unresolved id %q", field, reference)
		}
	}
}

func validateID(t *testing.T, field, value string) {
	t.Helper()
	validateText(t, field, value, maxIDBytes)
	if !idPattern.MatchString(value) {
		t.Fatalf("%s = %q, want a stable identifier", field, value)
	}
}

func validateText(t *testing.T, field, value string, limit int) {
	t.Helper()
	if value == "" || len(value) > limit || !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
		t.Fatalf("%s has invalid or unbounded text (%d bytes, limit %d)", field, len(value), limit)
	}
}

func validateNoSourceLeak(t *testing.T, response []byte, input observationInput) {
	t.Helper()
	text := string(response)
	for _, forbidden := range []string{"/Users/", "/home/", `C:\`, "file://"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("response contains absolute source marker %q", forbidden)
		}
	}
	for _, observation := range input.Observations {
		if strings.Contains(text, observation.Source.Path) {
			t.Fatalf("response contains source path %q instead of observation id", observation.Source.Path)
		}
	}
}
