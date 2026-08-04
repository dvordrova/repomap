package semanticmap

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const (
	maxAdapterFactsBytes = 64 << 10
	maxAdapterFacts      = 48
	maxAdapterScalar     = 240
)

type recordedAdapterFact struct {
	ID        string `json:"id"`
	Path      string `json:"path"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Kind      string `json:"kind"`
	Subject   string `json:"subject"`
	Object    string `json:"object"`
}

func TestRecordedAdapterFactsReproduceDeterministically(t *testing.T) {
	t.Run("caddy-go", func(t *testing.T) {
		packetBytes := readBoundedFile(t, "caddy.source-slices.json", maxSourcePacketBytes)
		recordedBytes := readBoundedFile(t, "caddy.adapter-facts.json", maxAdapterFactsBytes)
		recorded := decodeStrict[[]recordedAdapterFact](t, recordedBytes)
		packet := decodeStrict[sourcePacket](t, packetBytes)
		validateSourcePacket(t, packet)
		validateAdapterFacts(t, packet, recorded)
		validateAdapterFactReadiness(t, "caddy", recorded)

		generated, err := ExtractGoAdapterFacts(packetBytes)
		if err != nil {
			t.Fatal(err)
		}
		want := make([]recordedAdapterFact, len(generated))
		for i, fact := range generated {
			want[i] = recordedAdapterFact(fact)
		}
		if !reflect.DeepEqual(recorded, want) {
			t.Fatalf("recorded Go facts differ from deterministic extraction\nrecorded: %#v\ngenerated: %#v", recorded, want)
		}
	})

	t.Run("beets-python", func(t *testing.T) {
		packetBytes := readBoundedFile(t, "beets.source-slices.json", maxSourcePacketBytes)
		recordedBytes := readBoundedFile(t, "beets.adapter-facts.json", maxAdapterFactsBytes)
		recorded := decodeStrict[[]recordedAdapterFact](t, recordedBytes)
		packet := decodeStrict[sourcePacket](t, packetBytes)
		validateSourcePacket(t, packet)
		validateAdapterFacts(t, packet, recorded)
		validateAdapterFactReadiness(t, "beets", recorded)
		outputPath := filepath.Join(t.TempDir(), "beets.adapter-facts.json")
		command := exec.Command(
			"python3",
			"python_ast.py",
			"beets.source-slices.json",
			outputPath,
		)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("python adapter: %v\n%s", err, output)
		}
		generatedBytes, err := os.ReadFile(outputPath)
		if err != nil {
			t.Fatal(err)
		}
		generated := decodeStrict[[]recordedAdapterFact](t, generatedBytes)
		if !reflect.DeepEqual(recorded, generated) {
			t.Fatalf("recorded Python facts differ from deterministic extraction\nrecorded: %#v\ngenerated: %#v", recorded, generated)
		}
	})
}

func validateAdapterFactReadiness(
	t *testing.T,
	name string,
	facts []recordedAdapterFact,
) {
	t.Helper()
	type requirement struct {
		label string
		match func(recordedAdapterFact) bool
	}
	at := func(line int, snippets ...string) func(recordedAdapterFact) bool {
		return func(fact recordedAdapterFact) bool {
			if fact.StartLine != line {
				return false
			}
			text := fact.Subject + "\x00" + fact.Object
			for _, snippet := range snippets {
				if !strings.Contains(text, snippet) {
					return false
				}
			}
			return true
		}
	}
	any := func(matches ...func(recordedAdapterFact) bool) func(recordedAdapterFact) bool {
		return func(fact recordedAdapterFact) bool {
			for _, match := range matches {
				if match(fact) {
					return true
				}
			}
			return false
		}
	}

	var requirements []requirement
	switch name {
	case "caddy":
		requirements = []requirement{
			{"decode and run", at(247, "unsyncedDecodeAndRun")},
			{"raw config restore", any(at(259, "rawCfg"), at(261, "rawCfg"))},
			{"run candidate config", at(363, "run(newCfg, true)")},
			{"provision context", at(420, "provisionContext")},
			{"start application", at(447, "a.Start()")},
			{"stop partially started application", at(452, ".Stop()")},
			{"remember started application", at(460, "append(started, name)")},
			{"provision application", at(575, "ctx.App(appName)")},
			{"capture old context", at(370, "oldCtx", "currentCtx")},
			{"install new context", at(371, "currentCtx", "ctx")},
			{"retire old context", at(375, "unsyncedStop", "oldCtx")},
			{"late setup boundary", at(475, "finishSettingUp")},
		}
	case "beets":
		requirements = []requirement{
			{"configured plugin names", at(386, "plugins", "config")},
			{"disabled plugin filtering", at(403, "plugins", "disabled_plugins")},
			{"dynamic plugin import", at(421, "import_module")},
			{"plugin instantiation", at(438, "obj()")},
			{"loaded instance storage", at(459, "_instances.extend", "_get_plugin")},
			{"plugin command collection", at(475, "plugin.commands()")},
			{"plugin loading", at(755, "plugins.load_plugins")},
			{"command aggregation", at(761, "plugins.commands()")},
			{"parser registration", at(897, "parser.add_subcommand")},
			{"command lookup loop", any(at(708, "subcommand"), at(709, "subcommand"))},
			{"command lookup invocation", at(735, "_subcommand_for_name")},
			{"command dispatch", at(900, "subcommand.func")},
		}
	default:
		t.Fatalf("unknown adapter readiness case %q", name)
	}

	for _, requirement := range requirements {
		found := false
		for _, fact := range facts {
			if requirement.match(fact) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s facts omit readiness record: %s", name, requirement.label)
		}
	}
}

func validateAdapterFacts(
	t *testing.T,
	packet sourcePacket,
	facts []recordedAdapterFact,
) map[string]struct{} {
	t.Helper()
	if len(facts) == 0 || len(facts) > maxAdapterFacts {
		t.Fatalf("adapter facts count = %d, want 1..%d", len(facts), maxAdapterFacts)
	}
	known := make(map[string]struct{}, len(facts))
	previous := ""
	for i, fact := range facts {
		prefix := fmt.Sprintf("facts[%d]", i)
		validateID(t, prefix+".id", fact.ID)
		if fact.ID != fmt.Sprintf("r%d", i+1) {
			t.Fatalf("%s.id = %q, want deterministic r%d", prefix, fact.ID, i+1)
		}
		if _, duplicate := known[fact.ID]; duplicate {
			t.Fatalf("duplicate adapter fact id %q", fact.ID)
		}
		known[fact.ID] = struct{}{}
		validateSource(t, prefix, source{
			Path:      fact.Path,
			StartLine: fact.StartLine,
			EndLine:   fact.EndLine,
		})
		if !containedBySourceSlice(source{
			Path:      fact.Path,
			StartLine: fact.StartLine,
			EndLine:   fact.EndLine,
		}, packet.SourceSlices) {
			t.Fatalf("%s source is outside supplied slices", prefix)
		}
		switch fact.Kind {
		case "call", "assign", "branch", "loop", "return", "defer":
		default:
			t.Fatalf("%s.kind = %q", prefix, fact.Kind)
		}
		validateAdapterSyntaxScalar(t, prefix+".subject", fact.Subject)
		validateAdapterSyntaxScalar(t, prefix+".object", fact.Object)

		orderKey := fmt.Sprintf(
			"%s\x00%09d\x00%09d\x00%s\x00%s\x00%s",
			fact.Path,
			fact.StartLine,
			fact.EndLine,
			fact.Kind,
			fact.Subject,
			fact.Object,
		)
		if i > 0 && orderKey < previous {
			t.Fatalf("%s is not deterministically sorted", prefix)
		}
		previous = orderKey
	}
	return known
}

func validateAdapterSyntaxScalar(t *testing.T, field, value string) {
	t.Helper()
	validateText(t, field, value, maxAdapterScalar)
	if strings.TrimSpace(value) != value || strings.ContainsAny(value, "\r\n") {
		t.Fatalf("%s is not a compact syntax scalar", field)
	}
}
