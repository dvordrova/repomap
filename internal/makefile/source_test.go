package makefile

import (
	"fmt"
	"path"
	"slices"
	"strings"
	"testing"
)

func sourceRows(files map[string]string, manifest string, reads *[]string) []Row {
	return ReadSource(manifest, func(name string) ([]string, bool) {
		if reads != nil {
			*reads = append(*reads, name)
		}
		text, found := files[name]
		return strings.Split(text, "\n"), found
	}, func(name string) (string, bool) {
		if path.IsAbs(name) || strings.Contains(name, "$") {
			return "", false
		}
		name = path.Join(path.Dir(manifest), name)
		return name, !strings.HasPrefix(name, "../")
	})
}

func TestSourceIncludesCompleteGoalFragmentsFromTheInvocationDirectory(t *testing.T) {
	files := map[string]string{
		"app/Makefile":        "TOP = .\nall:\ninclude\t$(TOP)/build/first.mk second.mk\n",
		"app/build/first.mk":  "LINK_FLAGS = -pthread\nall: daemon\ndaemon: daemon.o\n\t$(CC) -o daemon daemon.o $(LINK_FLAGS)\ninclude nested.mk\n",
		"app/second.mk":       "all: client\nclient: client.o\n\t$(CC) -o client client.o\n",
		"app/nested.mk":       "ifeq ($(WITH_TOOL),1)\nall: tool\ntool: tool.o\n\t$(CC) -o tool tool.o\nendif\n",
		"app/build/nested.mk": "wrong: wrong.o\n",
	}
	var reads []string
	rows := sourceRows(files, "app/Makefile", &reads)
	if !slices.Equal(reads, []string{"app/Makefile", "app/build/first.mk", "app/nested.mk", "app/second.mk"}) {
		t.Fatalf("include words or invocation cwd changed: %v", reads)
	}
	bySource := map[string]Row{}
	for _, row := range rows {
		bySource[fmt.Sprintf("%s:%d %s", row.Path, row.Line, row.Key)] = row
	}
	for key, value := range map[string]string{
		"app/Makefile:2 default_goal":                          "all: daemon client — fragments: app/Makefile:2; app/build/first.mk:2; app/nested.mk:2 when ifeq ($(WITH_TOOL),1); app/second.mk:1",
		"app/Makefile:3 include":                               "include\t$(TOP)/build/first.mk second.mk — reads app/build/first.mk; reads app/second.mk",
		"app/build/first.mk:3 rule.daemon":                     "daemon.o — runs: $(CC) -o daemon daemon.o $(LINK_FLAGS)",
		"app/build/first.mk:1 variable.LINK_FLAGS":             "-pthread",
		"app/nested.mk:2 rule.all when ifeq ($(WITH_TOOL),1)":  "tool",
		"app/nested.mk:3 rule.tool when ifeq ($(WITH_TOOL),1)": "tool.o — runs: $(CC) -o tool tool.o",
		"app/second.mk:1 rule.all":                             "client",
	} {
		if got, exists := bySource[key]; !exists || got.Value != value {
			t.Fatalf("complete fragment %s: got %+v; want %q", key, got, value)
		}
	}
}

func TestSourcePreservesRepeatedConditionalUsesCyclesAndUnresolvedExpressions(t *testing.T) {
	files := map[string]string{
		"Makefile":         "all:\ninclude shared.mk\nifeq ($(OPTION),one)\ninclude shared.mk\nendif\ninclude $(wildcard hidden/*.mk) $(UNKNOWN)/x.mk missing.mk\n",
		"shared.mk":        "all: app\ninclude Makefile\napp: main.o\n\t$(CC) -o app main.o\n",
		"hidden/secret.mk": "all: invented\n",
	}
	var reads []string
	rows := sourceRows(files, "Makefile", &reads)
	if !slices.Equal(reads, []string{"Makefile", "shared.mk", "shared.mk", "missing.mk"}) {
		t.Fatalf("repeated source was suppressed or unknown expression executed: %v", reads)
	}
	var unconditional, conditional, cycles int
	for _, row := range rows {
		if row.Path == "shared.mk" && row.Key == "rule.app" {
			unconditional++
		}
		if row.Path == "shared.mk" && row.Key == "rule.app when ifeq ($(OPTION),one)" {
			conditional++
		}
		if row.Path == "shared.mk" && strings.HasPrefix(row.Key, "include") && strings.Contains(row.Value, "cycle to Makefile") {
			cycles++
		}
		if row.Path == "Makefile" && row.Line == 6 && row.Value != "include $(wildcard hidden/*.mk) $(UNKNOWN)/x.mk missing.mk — unresolved expression $(wildcard hidden/*.mk); unresolved expression $(UNKNOWN)/x.mk; unavailable source missing.mk" {
			t.Fatalf("unresolved source was lost: %+v", row)
		}
	}
	if unconditional != 1 || conditional != 1 || cycles != 2 {
		t.Fatalf("source uses/conditions/cycles changed: %d/%d/%d", unconditional, conditional, cycles)
	}
}

func TestSourceIncludeVariablesDoNotChooseConditionalOrLateImmediateValues(t *testing.T) {
	for name, text := range map[string]string{
		"conditional":    "all:\nifeq ($(PLATFORM),a)\nRULES = first.mk\nelse\nRULES = second.mk\nendif\ninclude $(RULES)\n",
		"late immediate": "all:\nRULES := $(LATER)\nLATER = first.mk\ninclude $(RULES)\n",
	} {
		t.Run(name, func(t *testing.T) {
			var reads []string
			rows := sourceRows(map[string]string{"Makefile": text, "first.mk": "all: invented\n", "second.mk": "all: other\n"}, "Makefile", &reads)
			if !slices.Equal(reads, []string{"Makefile"}) {
				t.Fatalf("unknown variable selected an include: %v", reads)
			}
			if !slices.ContainsFunc(rows, func(row Row) bool {
				return row.Key == "include" && strings.Contains(row.Value, "unresolved expression $(RULES)")
			}) {
				t.Fatalf("unknown variable was not explicit: %v", rows)
			}
		})
	}
}

func TestSourceFreezesImmediateAssignmentsAndRulePrerequisitesAtTheirSourcePosition(t *testing.T) {
	files := map[string]string{
		"Makefile":        "P = first\nRULES := $(P)/rules.mk\nall: $(P)\nP = second\ninclude $(RULES)\nlate: $(AFTER)\nAFTER = invented\n",
		"first/rules.mk":  "all: more\n",
		"second/rules.mk": "all: wrong\n",
	}
	var reads []string
	rows := sourceRows(files, "Makefile", &reads)
	if !slices.Equal(reads, []string{"Makefile", "first/rules.mk"}) {
		t.Fatalf("immediate assignment changed its source binding: %v", reads)
	}
	for _, row := range rows {
		if row.Key == "default_goal" && row.Value != "all: first more — fragments: Makefile:3; first/rules.mk:1" {
			t.Fatalf("late variable changed a prerequisite: %+v", row)
		}
		if row.Key == "rule.late" && row.Value != "$(AFTER)" {
			t.Fatalf("forward assignment invented a parsed prerequisite: %+v", row)
		}
		if row.Key == "variable.RULES" && row.Value != "$(P)/rules.mk" {
			t.Fatalf("native authored assignment was rewritten: %+v", row)
		}
	}
}

func TestSourceConditionalDefaultGoalRemainsUnresolved(t *testing.T) {
	rows := sourceRows(map[string]string{"Makefile": "all: app\nifeq ($(MODE),test)\n.DEFAULT_GOAL := test\nelse\n.DEFAULT_GOAL := all\nendif\ntest: app\n\t./app --test\n"}, "Makefile", nil)
	var assignments int
	unknown := false
	for _, row := range rows {
		if row.Key == "default_goal" {
			t.Fatalf("an unknown branch became the default target: %+v", row)
		}
		if strings.HasPrefix(row.Key, "variable..DEFAULT_GOAL when ") {
			assignments++
		}
		unknown = unknown || row.Key == "default_goal_unresolved"
	}
	if assignments != 2 || !unknown {
		t.Fatalf("conditional default assignments were lost: %+v", rows)
	}
}

func TestSourceCompleteDefaultCatalogueKeepsItsFirstFragmentRecipe(t *testing.T) {
	rows := sourceRows(map[string]string{"Makefile": "all: app\n\techo completed\ninclude extra.mk\n", "extra.mk": "all: header.h\n"}, "Makefile", nil)
	if !slices.ContainsFunc(rows, func(row Row) bool {
		return row.Path == "Makefile" && row.Line == 1 && row.Key == "rule.all" && row.Value == "app — runs: echo completed"
	}) {
		t.Fatalf("default catalogue erased its original first fragment: %+v", rows)
	}
}

func TestSourceLaterKnownBindingsAndImmediateAppendsSelectOnlyTheirExactFiles(t *testing.T) {
	for name, test := range map[string]struct {
		text     string
		expected []string
	}{
		"later replaces unknown":             {"all:\nP := $(UNKNOWN)\nP = known.mk\ninclude $(P)\n", []string{"Makefile", "known.mk"}},
		"simple append freezes":              {"all:\nP := first.mk\nQ = one.mk\nP += $(Q)\nQ = two.mk\ninclude $(P)\n", []string{"Makefile", "first.mk", "one.mk"}},
		"recursive append remains recursive": {"all:\nP = first.mk\nQ = one.mk\nP += $(Q)\nQ = two.mk\ninclude $(P)\n", []string{"Makefile", "first.mk", "two.mk"}},
	} {
		t.Run(name, func(t *testing.T) {
			var reads []string
			sourceRows(map[string]string{"Makefile": test.text, "known.mk": "all: known\n", "first.mk": "all: first\n", "one.mk": "all: one\n", "two.mk": "all: two\n"}, "Makefile", &reads)
			if !slices.Equal(reads, test.expected) {
				t.Fatalf("authored binding changed include identity: got %v, want %v", reads, test.expected)
			}
		})
	}
}

func TestSourceUnsupportedDefaultAssignmentDoesNotChooseAnEarlierOrLaterTarget(t *testing.T) {
	for _, assignment := range []string{".DEFAULT_GOAL ?= later", ".DEFAULT_GOAL :="} {
		rows := sourceRows(map[string]string{"Makefile": "all: app\n" + assignment + "\nlater: other\n"}, "Makefile", nil)
		if slices.ContainsFunc(rows, func(row Row) bool { return row.Key == "default_goal" }) || !slices.ContainsFunc(rows, func(row Row) bool { return row.Key == "default_goal_unresolved" }) {
			t.Fatalf("unsupported default assignment became a chosen target: %s / %+v", assignment, rows)
		}
	}
}

func TestSourceEscapedIncludeWordDoesNotReadAnUnrelatedSuffix(t *testing.T) {
	var reads []string
	rows := sourceRows(map[string]string{"Makefile": "all:\ninclude foo\\ bar.mk known.mk\n", "bar.mk": "all: wrong\n", "known.mk": "all: supported\n"}, "Makefile", &reads)
	if !slices.Equal(reads, []string{"Makefile", "known.mk"}) || !slices.ContainsFunc(rows, func(row Row) bool {
		return row.Key == "include" && row.Value == "include foo\\ bar.mk known.mk — unresolved expression foo\\ bar.mk; reads known.mk"
	}) {
		t.Fatalf("escaped include split into another native filename: reads=%v rows=%+v", reads, rows)
	}
}
