package contracttest

import (
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/jstsproject"
)

// A copied frontend template has no src directory yet, but its written Vite
// config is an explicit compiler input. Its excluded template is not one.
func TestCumulativeJSTSEmptyConfigKeepsExplicitToolSource(t *testing.T) {
	root, repository := materializeFixtureRepository(t, "jsts")
	result, err := sharedJSTSSelected(t, repository, root, "jsts:packages/template-tool/package.json")
	if err != nil {
		t.Fatal(err)
	}
	const source = "packages/template-tool/vite.config.ts"
	if len(result.Files) != 1 || result.Files[0].Path != source {
		t.Fatalf("explicit inferred tool program = %#v, files = %#v", result.Project, result.Files)
	}
	// The config selects nothing, so the tool program takes the compiler's
	// own inferred defaults, the ones a package without a config gets: the
	// legacy helper's nodenext, TypeScript 7's bundler. The written
	// "bundler" of its config is not read for it.
	configless, err := sharedJSTSSelected(t, repository, root, "jsts:packages/canvas-ui/package.json")
	if err != nil {
		t.Fatal(err)
	}
	if configless.Project.ConfigPath != "" || !slices.Contains([]string{"nodenext", "bundler"}, configless.Project.ModuleResolution) {
		t.Fatalf("the package without a config resolves modules as %q under %q", configless.Project.ModuleResolution, configless.Project.ConfigPath)
	}
	if result.Project.ModuleResolution != configless.Project.ModuleResolution {
		t.Fatalf("explicit inferred tool program resolves modules as %q, a package without a config as %q", result.Project.ModuleResolution, configless.Project.ModuleResolution)
	}
	index, _, err := jstsproject.BuildFromResult(result)
	if err != nil {
		t.Fatal(err)
	}
	if err := jstsproject.ValidateProgramIndex(result, index); err != nil {
		t.Fatal(err)
	}
	var owner string
	for _, object := range index.Objects {
		if object.Name == "excludedTemplateEntry" || object.Location.Path == "packages/template-tool/index.tsx" {
			t.Fatalf("excluded template acquired native ownership: %#v", object)
		}
		if object.Name == "templateConfig" {
			if object.Location.Path != source || object.Location.Line != 1 || object.Location.Column != 17 {
				t.Fatalf("tool declaration lost compiler source: %#v", object)
			}
			owner = object.ID
		}
	}
	if owner == "" {
		t.Fatal("explicit tool declaration missing from sealed native index")
	}
	found := false
	for _, call := range index.Relations {
		if slices.Contains(call.ToIDs, owner) && call.Location != nil && call.Location.Path == source && call.Location.Line == 4 {
			found = true
		}
	}
	if !found {
		t.Fatalf("exact configuration call lost: %#v", index.Relations)
	}
}

func TestCumulativeJSTSDependencyAliasDoesNotInventInstalledSource(t *testing.T) {
	root, repository := materializeFixtureRepository(t, "jsts")
	result, err := sharedJSTSSelected(t, repository, root, "jsts:package.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Project.PathAliases) != 1 || result.Project.PathAliases[0].Pattern != "@fixture/dependency-alias" || !slices.Equal(result.Project.PathAliases[0].Targets, []string{"node_modules/@fixture/missing-sdk"}) {
		t.Fatalf("written dependency alias not preserved: %#v", result.Project.PathAliases)
	}
	index, _, err := jstsproject.BuildFromResult(result)
	if err != nil {
		t.Fatal(err)
	}
	const source = "src/dependency-alias.ts"
	found := false
	for _, call := range result.Calls {
		if call.Location.Path == source && call.Expression == "embed" {
			found = true
			if call.Resolution != "unresolved" || len(call.CalleeRefs) != 0 {
				t.Fatalf("missing SDK acquired native call authority: %#v", call)
			}
		}
	}
	if !found {
		t.Fatalf("original missing dependency call disappeared: %#v", result.Calls)
	}
	for _, object := range index.Objects {
		if object.Location != nil && strings.Contains(object.Location.Path, "node_modules/") {
			t.Fatalf("configured dependency became repository source: %#v", object)
		}
	}
}
