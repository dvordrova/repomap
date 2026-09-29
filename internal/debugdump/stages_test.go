package debugdump

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// A stage that asks the model but is missing from semanticStages loses every
// exchange it makes: the write is refused and the run warns stage=unknown
// (atlas_inputs and atlas_systems did, 2026-09-29). The stage owners cannot
// be imported here (they import this package), so their declarations are
// read from source: every string constant a stage owner names Stage… is
// a stage its requests are journaled under.
func TestEveryDefinedStageIsASemanticStage(t *testing.T) {
	owners := map[string]*regexp.Regexp{
		".":                    regexp.MustCompile(`^SemanticStage[A-Z]\w*$`),
		"../atlas/lines":       regexp.MustCompile(`^Stage[A-Z]\w*$`),
		"../atlas/reading":     regexp.MustCompile(`^[Ss]tage[A-Z]\w*$`),
		"../terminology":       regexp.MustCompile(`^StageName$`),
		"../reporttranslation": regexp.MustCompile(`^StageName$`),
	}
	found := 0
	for dir, name := range owners {
		for stage, where := range stageConstants(t, dir, name) {
			found++
			if !validSemanticStage(stage) {
				t.Errorf("%s declares stage %q, which is not a semantic stage: its exchanges would not be journaled", where, stage)
			}
		}
	}
	if found < len(semanticStages) {
		t.Fatalf("read %d stage declarations, fewer than the %d semantic stages: the source scan missed its owners", found, len(semanticStages))
	}
}

// stageConstants returns each string constant of the package in dir whose
// name matches, with the file and name that declare it.
func stageConstants(t *testing.T, dir string, name *regexp.Regexp) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	stages := map[string]string{}
	files := token.NewFileSet()
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(files, path, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			block, ok := decl.(*ast.GenDecl)
			if !ok || block.Tok != token.CONST {
				continue
			}
			for _, spec := range block.Specs {
				values := spec.(*ast.ValueSpec)
				for i, ident := range values.Names {
					if !name.MatchString(ident.Name) || i >= len(values.Values) {
						continue
					}
					literal, ok := values.Values[i].(*ast.BasicLit)
					if !ok || literal.Kind != token.STRING {
						continue
					}
					value, err := strconv.Unquote(literal.Value)
					if err != nil {
						t.Fatal(err)
					}
					stages[value] = path + ": " + ident.Name
				}
			}
		}
	}
	if len(stages) == 0 {
		t.Fatalf("%s declares no stage constants: the scan no longer reads its owner", dir)
	}
	return stages
}
