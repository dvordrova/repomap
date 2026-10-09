package cproject

import "testing"

func TestRuleSiteKeepsIncludedSourceAndUnknownConditions(t *testing.T) {
	for _, test := range []struct {
		name  string
		root  string
		child string
		want  Site
	}{
		{"included recipe", "all: app\ninclude build/main.mk\n", "app: app.o\n\t$(CC) -o app app.o\n", Site{Path: "build/main.mk", Line: 1}},
		{"separate prerequisite fragment", "all: app\napp: app.o\ninclude build/main.mk\n", "app:\n\t$(CC) -o app app.o\n", Site{Path: "build/main.mk", Line: 1}},
		{"competing recipes", "all: app\napp: app.o\n\t$(CC) -o app app.o\ninclude build/main.mk\n", "app:\n\t$(CC) -o app app.o\n", Site{Path: "Makefile"}},
		{"conditional only", "all: app\nifeq ($(OPTION),1)\ninclude build/main.mk\nendif\n", "app: app.o\n\t$(CC) -o app app.o\n", Site{Path: "Makefile"}},
		{"conditional versus implicit rule", "all: app\napp: app.o\nifeq ($(OPTION),1)\ninclude build/main.mk\nendif\n", "app:\n\t$(CC) -o app app.o\n", Site{Path: "Makefile"}},
		{"same source conditional and unconditional", "all: app\ninclude build/main.mk\nifeq ($(OPTION),1)\ninclude build/main.mk\nendif\n", "app: app.o\n\t$(CC) -o app app.o\n", Site{Path: "build/main.mk", Line: 1}},
	} {
		t.Run(test.name, func(t *testing.T) {
			root, repository := writeRepository(t, map[string]string{"Makefile": test.root, "build/main.mk": test.child, "app.c": "int main(void) { return 0; }\n"})
			env := parseEnv{root: root, roots: rootsOf(root), repository: repository}
			if got := ruleSite(env, "Makefile", "app"); got != test.want {
				t.Fatalf("native output source = %+v; want %+v", got, test.want)
			}
			if !hasRule(env, "Makefile", "app") {
				t.Fatal("source uncertainty erased the authored output rule")
			}
		})
	}
}

func TestRulePresenceAndInvocationDirectoryAreIndependentOfSourceSite(t *testing.T) {
	root, repository := writeRepository(t, map[string]string{
		"Makefile":       "all:\ninclude build/main.mk\n",
		"build/main.mk":  "all: app\ninclude rules.mk\n",
		"rules.mk":       "app: app.o\n\t$(CC) -o app app.o\n",
		"build/rules.mk": "wrong: wrong.o\n\t$(CC) -o wrong wrong.o\n",
		"app.c":          "int main(void) { return 0; }\n",
	})
	env := parseEnv{root: root, roots: rootsOf(root), repository: repository}
	if !hasRule(env, "Makefile", "all") || ruleSite(env, "Makefile", "all") != (Site{Path: "Makefile"}) {
		t.Fatal("several default fragments were confused with an absent default target")
	}
	if got := ruleSite(env, "Makefile", "app"); got != (Site{Path: "rules.mk", Line: 1}) {
		t.Fatalf("included filename used the child directory rather than invocation cwd: %+v", got)
	}
	if hasRule(env, "Makefile", "wrong") {
		t.Fatal("a source not named by the invocation-relative include was read")
	}
}

func TestProgramBindingKeepsBothManifestAndRuleSource(t *testing.T) {
	_, repository := writeRepository(t, map[string]string{
		"Makefile":      "include build/main.mk\n",
		"build/main.mk": "app: app.o\n\t$(CC) -o app app.o\n",
		"app.c":         "int main(void) { return 0; }\n",
	})
	ref := func(name string) string { value, _ := repository.ID(name); return string(value) }
	program := Program{Selector: "c:app", Name: "app", Kind: ProgramExecutable, CorpusSHA256: repository.SHA256(),
		Anchor: Site{Path: "build/main.mk", Line: 1}, AnchorFileRef: ref("build/main.mk"), Manifest: "Makefile", ManifestFileRef: ref("Makefile"),
		Units:    []UnitSpec{{Path: "app.c", FileRef: ref("app.c"), Dir: ".", Source: "app.c", Built: true}},
		Evidence: []Observation{{Kind: "c_link", Path: "build/main.mk", Line: 1}},
	}
	program.Ref = programRef(program)
	if err := program.ValidateAgainst(repository); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*Program){
		func(p *Program) { p.Manifest = "" },
		func(p *Program) { p.ManifestFileRef = p.AnchorFileRef },
		func(p *Program) { p.AnchorFileRef = p.ManifestFileRef },
		func(p *Program) { p.Evidence = []Observation{{Kind: "c_link", Path: "absent.mk", Line: 1}} },
	} {
		changed := program
		change(&changed)
		changed.Ref = programRef(changed)
		if changed.ValidateAgainst(repository) == nil {
			t.Fatalf("a changed native root/source binding was accepted: %+v", changed)
		}
	}
	changed := program
	changed.Manifest, changed.ManifestFileRef = changed.Anchor.Path, changed.AnchorFileRef
	if changed.Validate() == nil {
		t.Fatal("changing the owning manifest retained the sealed program identity")
	}
}
