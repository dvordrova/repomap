package cproject

import "testing"

func TestLocSiteKeepsMacroArgumentSpelling(t *testing.T) {
	use := Position{File: "server.c", Line: 20, Col: 5, Offset: 400, TokLen: 8}
	// kvAssert(openSocket(port)): openSocket is written in the argument.
	argument := Loc{Spelling: Position{File: "server.c", Line: 20, Col: 14, Offset: 409, TokLen: 10}, Expansion: use, MacroArg: true}
	if got := argument.Site(); got != argument.Spelling || argument.InMacroBody() {
		t.Fatalf("a call written in a macro argument moved to the macro name: %+v", got)
	}
	// The macro body's own call (_kvAssert) is found at the macro name.
	body := Loc{Spelling: Position{File: "server.h", Line: 7, Col: 30, Offset: 120, TokLen: 9}, Expansion: use}
	if got := body.Site(); got != use || !body.InMacroBody() {
		t.Fatalf("a macro body token is not sited at the macro use: %+v", got)
	}
	// An argument spelled inside another macro's body is that macro's use.
	nested := Loc{Spelling: Position{File: "server.h", Line: 9, Col: 12, Offset: 180, TokLen: 4}, Expansion: use, MacroArg: true}
	if got := nested.Site(); got != use {
		t.Fatalf("an argument written in a header macro body left the reader's site: %+v", got)
	}
	// So is an argument spelled in the body of a macro defined above in
	// the same file (#define RUN(x) kvAssert(work(x))).
	local := Loc{Spelling: Position{File: "server.c", Line: 3, Col: 25, Offset: 60, TokLen: 4}, Expansion: use, MacroArg: true}
	if got := local.Site(); got != use || !local.InMacroBody() {
		t.Fatalf("an argument written in a macro body above in the file left the reader's site: %+v", got)
	}
	plain := Loc{Spelling: use, Expansion: use}
	if plain.FromMacro() || plain.Site() != use {
		t.Fatal("a plain location changed")
	}
}

func TestProgramValidateBindsIdentity(t *testing.T) {
	program := Program{Selector: "c:kvd", Name: "kvd", Kind: ProgramExecutable, CorpusSHA256: "sha", Anchor: Site{Path: "Makefile", Line: 9}, AnchorFileRef: "f1",
		Units: []UnitSpec{{Path: "kvd.c", FileRef: "f2", Dir: ".", Source: "kvd.c", Built: true}}}
	program.Ref = programRef(program)
	if err := program.Validate(); err != nil {
		t.Fatal(err)
	}
	changed := program
	changed.Units = []UnitSpec{{Path: "kvd.c", FileRef: "f2", Dir: ".", Source: "kvd.c", Args: []string{"-DKV_SMALL"}, Built: true}}
	if changed.Validate() == nil {
		t.Fatal("a program whose flags changed kept its identity")
	}
	closure := program
	closure.Closure, closure.Units = true, append(program.Units, program.Units[0])
	closure.Ref = programRef(closure)
	if closure.Validate() == nil {
		t.Fatal("a closure program named more than its main unit")
	}
}

func TestTokenText(t *testing.T) {
	source := []byte("  act.sa_handler = h;")
	if got := TokenText(source, Position{Offset: 6, TokLen: 10}); got != "sa_handler" {
		t.Fatalf("token: %q", got)
	}
	if got := TokenText(source, Position{Offset: 20, TokLen: 5}); got != "" {
		t.Fatalf("out of range token: %q", got)
	}
}
