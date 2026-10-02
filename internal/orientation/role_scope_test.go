package orientation

import (
	"strings"
	"testing"
)

// A role describes its own target from its own evidence. Lua 5.1.5's etc
// library (t2, one file, etc/noparser.c) had been described from etc/README
// as the whole directory's extras: every ref it cited was a README line. A
// ref of another target is ignored and recorded; a role citing only another
// target's evidence is refused; a target the request lists no fact or seed
// of keeps a role that cites nothing, and one that has some must cite them.
// A shared makefile's rule is the own evidence of the program it builds.
func TestARoleIsReadFromItsOwnTargetsEvidence(t *testing.T) {
	cat := newCatalog()
	for _, ref := range []string{"t1", "t2", "t3", "t5", "t6"} {
		cat.targets[ref] = ref
	}
	// Lua's root makefile is the lua program's and liblua.a's manifest; its
	// rule for liblua.a is liblua.a's own, its rule for lua is lua's.
	cat.facts["a477"] = factEntry{id: "a477", byTarget: map[string]string{"t6": "a477"}}
	cat.facts["a478"] = factEntry{id: "a478", byTarget: map[string]string{"t1": "a478"}}
	cat.facts["a6"] = factEntry{id: "a6", byTarget: map[string]string{"t3": "a6"}}
	cat.facts["a9"] = factEntry{id: "a9", byTarget: map[string]string{"t3": "a9", "t5": "a10"}}
	cat.facts["a30"] = factEntry{id: "a30"} // repository-wide: no target's own
	cat.subjects["t5.n1"] = subjectEntry{id: "n1", targetRef: "t5"}

	result, err := normalizeOverview([]byte(`{"roles":[
		{"target":"t2","role":"Example and helper sources","purpose":"Holds public-domain extras.","refs":["a6"]},
		{"target":"t2","role":"Parser stub","purpose":"Stands in for the parser when it is left out.","refs":["a30"]},
		{"target":"t3","role":"Minimal interpreter","purpose":"Runs Lua from stdin.","refs":["a6","t5.n1","a9"]},
		{"target":"t5","role":"Interpreter","purpose":"Runs Lua scripts.","refs":[]},
		{"target":"t6","role":"Core static library","purpose":"Packages the core objects into liblua.a.","refs":["a477","a478"]}
	]}`), cat)
	if err != nil {
		t.Fatal(err)
	}
	roles := map[string]Role{}
	for _, role := range result.roles {
		roles[role.TargetID] = role
	}
	if role, kept := roles["t2"]; !kept || role.Role != "Parser stub" || len(role.FactIDs) != 0 {
		t.Fatalf("t2 has no fact or seed of its own: its role stands without refs, never on another target's: %+v", roles["t2"])
	}
	if role := roles["t3"]; len(role.FactIDs) != 2 || role.FactIDs[0] != "a6" || role.FactIDs[1] != "a9" || len(role.SubjectIDs) != 0 {
		t.Fatalf("t3 keeps its own facts and drops t5's seed: %+v", role)
	}
	if role := roles["t6"]; len(role.FactIDs) != 1 || role.FactIDs[0] != "a477" {
		t.Fatalf("t6 keeps its own rule and drops lua's: %+v", role)
	}
	if _, kept := roles["t5"]; kept {
		t.Fatal("t5 has a seed of its own, so a role citing nothing is refused")
	}
	var reasons []string
	for _, row := range result.rejected {
		reasons = append(reasons, row.Reason)
	}
	joined := strings.Join(reasons, "\n")
	for _, want := range []string{
		`the role of "t2" cites only other targets' evidence`,
		`another target's evidence ignored for the role of "t3"`,
		`the role of "t5" cites none of its own facts or seeds`,
		"unsupported reference ignored",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("refusals do not say %q:\n%s", want, joined)
		}
	}
}
