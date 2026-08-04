package semanticmap

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"
)

const (
	maxSourcePacketBytes  = 32 << 10
	maxSourceTextBytes    = 24 << 10
	maxSourceSlices       = 12
	maxFeedObservations   = 18
	maxObservationSources = 3
)

type sourcePacket struct {
	CaseID       string        `json:"case_id"`
	Repository   repository    `json:"repository"`
	Question     string        `json:"question"`
	SourceSlices []sourceSlice `json:"source_slices"`
}

type sourceSlice struct {
	Path      string `json:"path"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Text      string `json:"text"`
}

type feedResponse struct {
	CaseID       string            `json:"case_id"`
	Repository   repository        `json:"repository"`
	Question     string            `json:"question"`
	Observations []feedObservation `json:"observations"`
}

type feedObservation struct {
	ID      string   `json:"id"`
	State   string   `json:"state"`
	Text    string   `json:"text"`
	Sources []source `json:"sources"`
}

var promptOnlyNegativeFixtureHashes = map[string]struct {
	observations string
	semanticMap  string
}{
	"caddy": {
		observations: "918f6a5fa6856fd31a23dfb20c7ff0d1d21e006f4b9f8f2e1cfaea59e3e524e8",
		semanticMap:  "e5278fb68b19169013c1f99b2cb48b9374ffd7473d67160e3a5b9acd9406dd1e",
	},
	"beets": {
		observations: "a96288c8c8c3155d7b966a4b7e6952654d9729abf07a2ae41107c090f1236f8a",
		semanticMap:  "c917aa1f664fc0e479765a2ece9498eb19a5fea9f7238fbb55a3a2c53e5e5c31",
	},
}

func TestPromptOnlyMapsRemainRejectedNegativeFixtures(t *testing.T) {
	for _, name := range []string{"caddy", "beets"} {
		t.Run(name, func(t *testing.T) {
			packetBytes := readBoundedFile(t, name+".source-slices.json", maxSourcePacketBytes)
			feedBytes := readBoundedFile(t, name+".observations.response.json", maxResponseBytes)
			mapBytes := readBoundedFile(t, name+".feed-map.response.json", maxResponseBytes)
			validateNegativeFixtureHash(t, name, feedBytes, mapBytes)

			packet := decodeStrict[sourcePacket](t, packetBytes)
			feed := decodeStrict[feedResponse](t, feedBytes)
			semanticMap := decodeStrict[semanticMap](t, mapBytes)

			validateSourcePacket(t, packet)
			observations := validateFeedResponse(t, packet, feed)
			validateResponse(t, semanticMap, observations)
			validateFeedMapNoPaths(t, mapBytes, packet)
			validatePromptOnlyMapIsRejected(t, feed, semanticMap)
		})
	}
}

func validateSourcePacket(t *testing.T, packet sourcePacket) {
	t.Helper()
	validateID(t, "case_id", packet.CaseID)
	validateText(t, "repository.name", packet.Repository.Name, maxIDBytes)
	if !revisionPattern.MatchString(packet.Repository.Revision) {
		t.Fatalf("repository.revision = %q, want lowercase 40-byte commit", packet.Repository.Revision)
	}
	validateText(t, "question", packet.Question, maxQuestionBytes)
	if len(packet.SourceSlices) == 0 || len(packet.SourceSlices) > maxSourceSlices {
		t.Fatalf("source_slices count = %d, want 1..%d", len(packet.SourceSlices), maxSourceSlices)
	}

	totalText := 0
	previousPath := ""
	previousStart := 0
	for i, slice := range packet.SourceSlices {
		prefix := fmt.Sprintf("source_slices[%d]", i)
		validateSource(t, prefix, source{
			Path:      slice.Path,
			StartLine: slice.StartLine,
			EndLine:   slice.EndLine,
		})
		validateText(t, prefix+".text", slice.Text, maxSourceTextBytes)
		totalText += len(slice.Text)

		lineCount := len(strings.Split(strings.TrimSuffix(slice.Text, "\n"), "\n"))
		if lineCount != slice.EndLine-slice.StartLine+1 {
			t.Fatalf("%s text has %d lines, range has %d", prefix, lineCount, slice.EndLine-slice.StartLine+1)
		}
		if i > 0 &&
			(slice.Path < previousPath || (slice.Path == previousPath && slice.StartLine <= previousStart)) {
			t.Fatalf("%s is not strictly sorted by path and line", prefix)
		}
		previousPath = slice.Path
		previousStart = slice.StartLine
	}
	if totalText > maxSourceTextBytes {
		t.Fatalf("source slice text = %d bytes, limit %d", totalText, maxSourceTextBytes)
	}
}

func validateFeedResponse(t *testing.T, packet sourcePacket, feed feedResponse) map[string]struct{} {
	t.Helper()
	if feed.CaseID != packet.CaseID ||
		feed.Repository != packet.Repository ||
		feed.Question != packet.Question {
		t.Fatal("observation response did not preserve case, repository, and question")
	}
	if len(feed.Observations) < 6 || len(feed.Observations) > maxFeedObservations {
		t.Fatalf("observations count = %d, want 6..%d", len(feed.Observations), maxFeedObservations)
	}

	known := make(map[string]struct{}, len(feed.Observations))
	for i, observation := range feed.Observations {
		prefix := fmt.Sprintf("observations[%d]", i)
		validateID(t, prefix+".id", observation.ID)
		if _, duplicate := known[observation.ID]; duplicate {
			t.Fatalf("duplicate observation id %q", observation.ID)
		}
		known[observation.ID] = struct{}{}
		switch observation.State {
		case "extracted", "inferred", "unknown":
		default:
			t.Fatalf("%s.state = %q, want extracted, inferred, or unknown", prefix, observation.State)
		}
		validateText(t, prefix+".text", observation.Text, 500)
		if len(observation.Sources) == 0 || len(observation.Sources) > maxObservationSources {
			t.Fatalf("%s.sources count = %d, want 1..%d", prefix, len(observation.Sources), maxObservationSources)
		}
		seenSources := make(map[source]struct{}, len(observation.Sources))
		for j, observationSource := range observation.Sources {
			field := fmt.Sprintf("%s.sources[%d]", prefix, j)
			validateSource(t, field, observationSource)
			if _, duplicate := seenSources[observationSource]; duplicate {
				t.Fatalf("%s duplicates a source range", field)
			}
			seenSources[observationSource] = struct{}{}
			if !containedBySourceSlice(observationSource, packet.SourceSlices) {
				t.Fatalf("%s = %#v, want a range inside supplied source slices", field, observationSource)
			}
		}
	}
	return known
}

func containedBySourceSlice(candidate source, slices []sourceSlice) bool {
	for _, slice := range slices {
		if candidate.Path == slice.Path &&
			candidate.StartLine >= slice.StartLine &&
			candidate.EndLine <= slice.EndLine {
			return true
		}
	}
	return false
}

func validateFeedMapNoPaths(t *testing.T, response []byte, packet sourcePacket) {
	t.Helper()
	text := string(response)
	for _, forbidden := range []string{"/Users/", "/home/", `C:\`, "file://"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("feed map contains absolute source marker %q", forbidden)
		}
	}
	for _, slice := range packet.SourceSlices {
		if strings.Contains(text, slice.Path) {
			t.Fatalf("feed map contains source path %q instead of observation id", slice.Path)
		}
	}
}

func validateNegativeFixtureHash(t *testing.T, name string, feedBytes, mapBytes []byte) {
	t.Helper()
	want, ok := promptOnlyNegativeFixtureHashes[name]
	if !ok {
		t.Fatalf("missing negative-fixture hashes for %q", name)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(feedBytes)); got != want.observations {
		t.Fatalf("%s observations fixture hash = %s, want %s", name, got, want.observations)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(mapBytes)); got != want.semanticMap {
		t.Fatalf("%s map fixture hash = %s, want %s", name, got, want.semanticMap)
	}
}

func validatePromptOnlyMapIsRejected(t *testing.T, feed feedResponse, response semanticMap) {
	t.Helper()
	states := make(map[string]string, len(feed.Observations))
	for _, observation := range feed.Observations {
		states[observation.ID] = observation.State
	}

	violations := 0
	check := func(state string, references []string, relation bool) {
		if state != "supported" {
			return
		}
		if relation && len(references) > 1 {
			violations++
			return
		}
		for _, reference := range references {
			if states[reference] != "extracted" {
				violations++
				return
			}
		}
	}
	for _, node := range response.Nodes {
		check(node.State, node.ObservationIDs, false)
	}
	for _, edge := range response.Edges {
		check(edge.State, edge.ObservationIDs, true)
	}
	if violations == 0 {
		t.Fatal("prompt-only map unexpectedly satisfies deterministic provenance rules")
	}
}
