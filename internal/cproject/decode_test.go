package cproject

import (
	"strings"
	"testing"
)

// handDump is a clang JSON AST in clang's own shape: file and line appear only
// when they change against the previous location written anywhere, locations
// of skipped system declarations still count, spellingLoc precedes
// expansionLoc, and includedFrom, presumedFile and presumedLine never change
// the state.
const handDump = `{
 "id": "0x1", "kind": "TranslationUnitDecl", "loc": {}, "range": {"begin": {}, "end": {}},
 "inner": [
  {"id": "0x2", "kind": "TypedefDecl", "loc": {}, "range": {"begin": {}, "end": {}}, "isImplicit": true, "name": "__int128_t",
   "type": {"qualType": "__int128"}, "inner": [{"id": "0x3", "kind": "BuiltinType", "type": {"qualType": "__int128"}}]},
  {"id": "0x10", "kind": "FunctionDecl",
   "loc": {"offset": 100, "file": "/sdk/usr/include/_stdlib.h", "line": 20, "col": 6, "tokLen": 5, "includedFrom": {"file": "/sdk/usr/include/stdlib.h"}},
   "range": {"begin": {"offset": 95, "col": 1, "tokLen": 4, "includedFrom": {"file": "/sdk/usr/include/stdlib.h"}},
             "end": {"offset": 140, "line": 21, "col": 30, "tokLen": 1, "includedFrom": {"file": "/sdk/usr/include/stdlib.h"}}},
   "name": "qsort", "type": {"qualType": "void (void *, int)"},
   "inner": [{"id": "0x11", "kind": "ParmVarDecl", "loc": {"offset": 120, "col": 20, "tokLen": 1, "presumedFile": "/elsewhere.h"},
              "range": {"begin": {"offset": 115, "col": 15, "tokLen": 4},
                        "end": {"offset": 145, "line": 22, "col": 20, "tokLen": 1, "includedFrom": {"file": "/sdk/usr/include/stdlib.h"}}}, "type": {"qualType": "int"}}]},
  {"id": "0x20", "kind": "VarDecl",
   "loc": {"spellingLoc": {"offset": 150, "col": 3, "tokLen": 4},
           "expansionLoc": {"offset": 30, "file": "a.c", "line": 3, "col": 1, "tokLen": 9}},
   "range": {"begin": {"spellingLoc": {"offset": 148, "file": "/sdk/usr/include/_stdlib.h", "line": 22, "col": 1, "tokLen": 2},
                       "expansionLoc": {"offset": 30, "file": "a.c", "line": 3, "col": 1, "tokLen": 9}},
             "end": {"offset": 38, "col": 9, "tokLen": 1}},
   "name": "made_by_a_system_macro", "type": {"qualType": "int"}, "storageClass": "static"},
  {"id": "0x30", "kind": "FunctionDecl",
   "loc": {"offset": 60, "line": 5, "col": 5, "tokLen": 4},
   "range": {"begin": {"offset": 56, "col": 1, "tokLen": 3}, "end": {"offset": 300, "line": 14, "col": 1, "tokLen": 1}},
   "name": "main", "type": {"qualType": "int (void)"},
   "inner": [
    {"id": "0x31", "kind": "CompoundStmt", "range": {"begin": {"offset": 67, "line": 5, "col": 16, "tokLen": 1}, "end": {"offset": 300, "line": 14, "col": 1, "tokLen": 1}},
     "inner": [
      {"id": "0x32", "kind": "CallExpr",
       "range": {"begin": {"spellingLoc": {"offset": 80, "line": 6, "col": 12, "tokLen": 4},
                           "expansionLoc": {"offset": 72, "col": 3, "tokLen": 5, "isMacroArgExpansion": true}},
                 "end": {"spellingLoc": {"offset": 86, "col": 18, "tokLen": 1}, "expansionLoc": {"offset": 72, "col": 3, "tokLen": 5, "isMacroArgExpansion": true}}},
       "type": {"qualType": "int"},
       "inner": [{"id": "0x33", "kind": "ImplicitCastExpr", "range": {"begin": {"spellingLoc": {"offset": 80, "col": 12, "tokLen": 4}, "expansionLoc": {"offset": 72, "col": 3, "tokLen": 5, "isMacroArgExpansion": true}},
                  "end": {"spellingLoc": {"offset": 80, "col": 12, "tokLen": 4}, "expansionLoc": {"offset": 72, "col": 3, "tokLen": 5, "isMacroArgExpansion": true}}},
                  "castKind": "FunctionToPointerDecay",
                  "inner": [{"id": "0x34", "kind": "DeclRefExpr", "range": {"begin": {"spellingLoc": {"offset": 80, "col": 12, "tokLen": 4}, "expansionLoc": {"offset": 72, "col": 3, "tokLen": 5, "isMacroArgExpansion": true}},
                              "end": {"spellingLoc": {"offset": 80, "col": 12, "tokLen": 4}, "expansionLoc": {"offset": 72, "col": 3, "tokLen": 5, "isMacroArgExpansion": true}}},
                              "referencedDecl": {"id": "0x40", "kind": "FunctionDecl", "name": "work", "type": {"qualType": "int (void)"}}}]}]},
      {"id": "0x35", "kind": "CallExpr",
       "range": {"begin": {"spellingLoc": {"offset": 40, "file": "./a.h", "line": 4, "col": 30, "tokLen": 4},
                           "expansionLoc": {"offset": 72, "file": "a.c", "line": 6, "col": 3, "tokLen": 5}},
                 "end": {"spellingLoc": {"offset": 49, "file": "./a.h", "line": 4, "col": 39, "tokLen": 1},
                         "expansionLoc": {"offset": 91, "file": "a.c", "line": 6, "col": 22, "tokLen": 1}}}},
      {"id": "0x36", "kind": "CallExpr",
       "range": {"begin": {"offset": 120, "line": 8, "presumedLine": 100, "col": 3, "tokLen": 6}, "end": {"offset": 140, "col": 23, "tokLen": 1}},
       "inner": [{"id": "0x37", "kind": "StringLiteral", "range": {"begin": {"offset": 127, "col": 10, "tokLen": 9}, "end": {"offset": 127, "col": 10, "tokLen": 9}},
                  "value": "\"a\\\"b\\né\""},
                 {"id": "0x38", "kind": "CharacterLiteral", "range": {"begin": {"offset": 138, "col": 21, "tokLen": 3}, "end": {"offset": 138, "col": 21, "tokLen": 3}}, "value": 97}]},
      {"id": "0x39", "kind": "InitListExpr", "range": {"begin": {"offset": 150, "line": 9, "col": 17, "tokLen": 1}, "end": {"offset": 154, "col": 21, "tokLen": 1}},
       "array_filler": [{"id": "0x3a", "kind": "ImplicitValueInitExpr", "range": {"begin": {}, "end": {}}},
                        {"id": "0x3b", "kind": "IntegerLiteral", "range": {"begin": {"offset": 152, "col": 19, "tokLen": 1}, "end": {"offset": 152, "col": 19, "tokLen": 1}}, "value": "1"}]},
      {"id": "0x3c", "kind": "ForStmt", "range": {"begin": {"offset": 170, "line": 10, "col": 3, "tokLen": 3}, "end": {"offset": 190, "col": 23, "tokLen": 1}},
       "inner": [{}, {}, {}, {}, {"id": "0x3d", "kind": "NullStmt", "range": {"begin": {"offset": 189, "col": 22, "tokLen": 1}, "end": {"offset": 189, "col": 22, "tokLen": 1}}}]}
     ]}
   ]},
  {"id": "0x50", "kind": "RecordDecl",
   "loc": {"offset": 500, "file": "/sdk/usr/include/sys/signal.h", "line": 287, "col": 8, "tokLen": 9, "includedFrom": {"file": "/sdk/usr/include/sys/wait.h"}},
   "range": {"begin": {"offset": 493, "col": 1, "tokLen": 6}, "end": {"offset": 560, "line": 291, "col": 1, "tokLen": 1}},
   "name": "sigaction", "tagUsed": "struct", "completeDefinition": true},
  {"id": "0x60", "kind": "FunctionDecl",
   "loc": {"offset": 310, "file": "a.c", "line": 16, "col": 12, "tokLen": 4},
   "range": {"begin": {"offset": 299, "col": 1, "tokLen": 6}, "end": {"offset": 330, "col": 32, "tokLen": 1}},
   "name": "work", "storageClass": "static", "previousDecl": "0x40", "inline": true}
 ]
}`

func TestDecodeReplaysElidedLocations(t *testing.T) {
	names := &fileNames{cwd: "/repo", roots: []string{"/repo"}, corpus: map[string]bool{"a.c": true, "a.h": true}, cache: map[string]fileName{}}
	decls, external, size, err := decodeUnit(strings.NewReader(handDump), names)
	if err != nil {
		t.Fatal(err)
	}
	if size != int64(len(handDump)) {
		t.Fatalf("read %d of %d bytes", size, len(handDump))
	}
	if len(decls) != 3 || decls[0].Name != "made_by_a_system_macro" || decls[1].Name != "main" || decls[2].Name != "work" {
		t.Fatalf("kept declarations: %+v", decls)
	}
	// Skipped system declarations: kept as external declarations only.
	if len(external) != 2 || external[0].Name != "qsort" || external[0].Position != (Position{File: "/sdk/usr/include/_stdlib.h", Line: 20, Col: 6, Offset: 100, TokLen: 5}) ||
		external[1].Name != "sigaction" || external[1].TagUsed != "struct" || external[1].Position.File != "/sdk/usr/include/sys/signal.h" {
		t.Fatalf("external: %+v", external)
	}

	// The spelling's file and line come from the last location of the
	// skipped qsort's parameter, not from its includedFrom.
	variable := decls[0]
	if variable.Loc.Spelling != (Position{File: "/sdk/usr/include/_stdlib.h", Line: 22, Col: 3, Offset: 150, TokLen: 4}) ||
		variable.Loc.Expansion != (Position{File: "a.c", Line: 3, Col: 1, Offset: 30, TokLen: 9}) || variable.StorageClass != "static" {
		t.Fatalf("macro-made declaration: %+v", variable.Loc)
	}
	// The spelling precedes the expansion, so the elided end is in a.c.
	if variable.End.Expansion != (Position{File: "a.c", Line: 3, Col: 9, Offset: 38, TokLen: 1}) || variable.End.FromMacro() {
		t.Fatalf("range end after a macro location: %+v", variable.End)
	}

	main := decls[1]
	if main.Loc.Site() != (Position{File: "a.c", Line: 5, Col: 5, Offset: 60, TokLen: 4}) || main.End.Expansion.Line != 14 || !hasBody(main) || !exactMain(main) {
		t.Fatalf("main: %+v", main)
	}
	body := main.Inner[0].Inner
	argument := body[0]
	if !argument.Begin.MacroArg || argument.Begin.Site() != (Position{File: "a.c", Line: 6, Col: 12, Offset: 80, TokLen: 4}) ||
		argument.Begin.Expansion != (Position{File: "a.c", Line: 6, Col: 3, Offset: 72, TokLen: 5}) {
		t.Fatalf("call in a macro argument: %+v", argument.Begin)
	}
	reference := argument.Inner[0].Inner[0]
	if reference.Kind != "DeclRefExpr" || reference.ReferencedDecl == nil || reference.ReferencedDecl.Name != "work" || reference.ReferencedDecl.ID != "0x40" {
		t.Fatalf("callee: %+v", reference)
	}
	macroBody := body[1]
	if !macroBody.Begin.InMacroBody() || macroBody.Begin.Spelling.File != "a.h" || macroBody.Begin.Site() != macroBody.Begin.Expansion || macroBody.End.Expansion.Col != 22 {
		t.Fatalf("call in a macro body: %+v", macroBody.Begin)
	}
	lineDirective := body[2]
	if lineDirective.Begin.Site().Line != 8 || lineDirective.End.Site().Line != 8 {
		t.Fatalf("presumedLine changed the actual line: %+v", lineDirective.Begin)
	}
	if got := lineDirective.Inner[0].Value; got != "\"a\\\"b\\né\"" {
		t.Fatalf("string literal value: %q", got)
	}
	if got := lineDirective.Inner[1].Value; got != "97" {
		t.Fatalf("character literal value: %q", got)
	}
	list := body[3]
	if list.ArrayFiller == nil || list.ArrayFiller.Kind != "ImplicitValueInitExpr" || len(list.Inner) != 1 || list.Inner[0].Value != "1" || list.Inner[0].Begin.Site().Line != 9 {
		t.Fatalf("initializers written after array_filler: %+v", list)
	}
	loop := body[4]
	if len(loop.Inner) != 5 || loop.Inner[0].Kind != "" || loop.Inner[4].Kind != "NullStmt" || loop.Inner[4].Begin.Site() != (Position{File: "a.c", Line: 10, Col: 22, Offset: 189, TokLen: 1}) {
		t.Fatalf("empty child slots: %+v", loop.Inner)
	}
	work := decls[2]
	if work.PreviousDecl != "0x40" || !work.Inline || work.Loc.Site().Line != 16 {
		t.Fatalf("definition: %+v", work)
	}
}

func TestDecodeRejectsTruncatedDump(t *testing.T) {
	names := &fileNames{cwd: "/repo", roots: []string{"/repo"}, corpus: map[string]bool{"a.c": true}, cache: map[string]fileName{}}
	if _, _, _, err := decodeUnit(strings.NewReader(handDump[:len(handDump)/2]), names); err == nil {
		t.Fatal("a truncated dump decoded")
	}
}

func TestDecodeNormalizesFileNames(t *testing.T) {
	names := &fileNames{cwd: "/repo/src", roots: []string{"/repo", "/private/repo"}, corpus: map[string]bool{"src/a.c": true, "include/a.h": true}, cache: map[string]fileName{}}
	for raw, want := range map[string]string{
		"a.c":                       "src/a.c",
		"./a.c":                     "src/a.c",
		"../include/a.h":            "include/a.h",
		"/private/repo/include/a.h": "include/a.h",
		"/repo/generated.h":         "/repo/generated.h",
		"/usr/include/stdio.h":      "/usr/include/stdio.h",
		"<scratch space>":           "<scratch space>",
	} {
		if got := names.normalize(raw); got != want {
			t.Errorf("%s: got %s want %s", raw, got, want)
		}
	}
}

func TestDecodeFilteredKeepsFunctionsNamedMain(t *testing.T) {
	dumps := `{"id": "0x1", "kind": "FunctionDecl", "loc": {"offset": 10, "file": "/sdk/math.h", "line": 3, "col": 8, "tokLen": 9},
  "range": {"begin": {"offset": 3, "col": 1, "tokLen": 6}, "end": {"offset": 30, "col": 30, "tokLen": 1}}, "name": "remainder",
  "inner": [{"id": "0x2", "kind": "ParmVarDecl", "loc": {"offset": 25, "col": 25, "tokLen": 1}, "range": {"begin": {"offset": 20, "col": 20, "tokLen": 6}, "end": {"offset": 25, "col": 25, "tokLen": 1}}}]}
{"id": "0x3", "kind": "FunctionDecl", "loc": {"offset": 11, "file": "t.c", "line": 1, "col": 12, "tokLen": 4},
  "range": {"begin": {"offset": 0, "col": 1, "tokLen": 6}, "end": {"offset": 30, "col": 31, "tokLen": 1}}, "name": "main", "storageClass": "static",
  "inner": [{"id": "0x4", "kind": "CompoundStmt", "range": {"begin": {"offset": 22, "col": 23, "tokLen": 1}, "end": {"offset": 30, "col": 31, "tokLen": 1}}}]}
{"id": "0x5", "kind": "FunctionDecl", "loc": {"offset": 36, "file": "t.c", "line": 2, "col": 5, "tokLen": 4},
  "range": {"begin": {"offset": 32, "col": 1, "tokLen": 3}, "end": {"offset": 45, "col": 14, "tokLen": 1}}, "name": "main",
  "inner": [{"id": "0x6", "kind": "ParmVarDecl", "loc": {}, "range": {"begin": {"offset": 41, "col": 10, "tokLen": 4}, "end": {"offset": 41, "col": 10, "tokLen": 4}}}]}
{"id": "0x7", "kind": "FunctionDecl", "loc": {"offset": 52, "file": "t.c", "line": 3, "col": 5, "tokLen": 4},
  "range": {"begin": {"offset": 48, "col": 1, "tokLen": 3}, "end": {"offset": 80, "col": 33, "tokLen": 1}}, "name": "main",
  "inner": [{"id": "0x8", "kind": "CompoundStmt", "range": {"begin": {"offset": 64, "col": 17, "tokLen": 1}, "end": {"offset": 80, "col": 33, "tokLen": 1}}}]}
`
	names := &fileNames{cwd: "/repo", roots: []string{"/repo"}, corpus: map[string]bool{"t.c": true}, cache: map[string]fileName{}}
	nodes, err := decodeFiltered(strings.NewReader(dumps), names)
	if err != nil {
		t.Fatal(err)
	}
	var exact []Position
	for _, node := range nodes {
		if exactMain(node) {
			exact = append(exact, node.Loc.Expansion)
		}
	}
	if len(nodes) != 3 || len(exact) != 1 || exact[0] != (Position{File: "t.c", Line: 3, Col: 5, Offset: 52, TokLen: 4}) {
		t.Fatalf("main candidates %d, exact %+v", len(nodes), exact)
	}
}
