package pythonprogramindex

import (
	"reflect"
	"testing"

	"github.com/dvordrova/repomap/internal/pythontarget"
)

func TestCumulativePytestMetadataPreservesAllIndexedDeclarations(t *testing.T) {
	repository := pythonCorpus(t, cumulativePythonSources(t, "src/fixture_app/test_market.py", "src/fixture_app/runtime_registrations.py"))
	index, err := buildOneForTest(t.Context(), repository, targetOfKind(t, repository, pythontarget.KindLibrary))
	if err != nil {
		t.Fatal(err)
	}
	tests := make(map[string]bool)
	for _, source := range index.Target.TestSources {
		tests[source] = true
	}
	if !reflect.DeepEqual(tests, map[string]bool{"src/fixture_app/test_market.py": true}) {
		t.Fatalf("pytest selection guessed a filename role: %+v sources=%+v", tests, index.Target.Sources)
	}
	found := false
	for _, object := range index.Objects {
		found = found || object.Name == "exercise_market"
	}
	if !found {
		t.Fatal("test classification removed the original declaration")
	}
}

func TestPytestFileNamesNeedFrameworkAuthorityAndNearestSettings(t *testing.T) {
	for _, test := range []struct {
		name, config string
		want         map[string]bool
	}{
		{"no runner", "[project]\nname='sample'\n[tool.pytest]\n", map[string]bool{}},
		{"runner defaults", "[project]\nname='sample'\ndependencies=['pytest>=9']\n[tool.pytest]\n", map[string]bool{"test_ready.py": true, "ready_test.py": true}},
		{"authored override", "[project]\nname='sample'\ndependencies=['pytest']\n[tool.pytest.ini_options]\npython_files=['check_*.py']\n", map[string]bool{"check_ready.py": true}},
		{"authored config with external dev dependencies", "[project]\nname='sample'\n[tool.pytest.ini_options]\npython_files=['check_*.py']\n", map[string]bool{"check_ready.py": true}},
		{"unrelated table", "[project]\nname='sample'\ndependencies=['pytest']\n[tool.another]\npython_files=['test_*.py']\n", map[string]bool{}},
		{"invalid setting", "[project]\nname='sample'\ndependencies=['pytest']\n[tool.pytest]\npython_files=42\n", map[string]bool{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository := pythonCorpus(t, map[string]string{"pyproject.toml": test.config, "test_ready.py": "def check(): pass", "ready_test.py": "def check(): pass", "check_ready.py": "def check(): pass", "testing/service.py": "def serve(): pass"})
			got, err := configuredPythonTests(repository)
			if err != nil || !reflect.DeepEqual(got, test.want) {
				t.Fatalf("discovery metadata=%v want=%v: %v", got, test.want, err)
			}
		})
	}
	repository := pythonCorpus(t, map[string]string{
		"pyproject.toml":       "[project]\nname='outer'\ndependencies=['pytest']\n[tool.pytest]\n",
		"inner/pyproject.toml": "[project]\nname='inner'\ndependencies=['pytest']\n[tool.pytest]\npython_files=[]\n",
		"test_ready.py":        "def check(): pass", "inner/test_ready.py": "def check(): pass",
	})
	got, err := configuredPythonTests(repository)
	if err != nil || !reflect.DeepEqual(got, map[string]bool{"test_ready.py": true}) {
		t.Fatalf("parent config overrode a complete empty local selection: %v / %v", got, err)
	}
	unknown := pythonCorpus(t, map[string]string{
		"pyproject.toml":       "[project]\nname='outer'\ndependencies=['pytest']\n[tool.pytest]\n",
		"inner/pyproject.toml": "[project]\nname='inner'\n[tool.pytest]\npython_files=42\n",
		"inner/test_ready.py":  "def serve(): pass",
	})
	got, err = configuredPythonTests(unknown)
	if err != nil || len(got) != 0 {
		t.Fatalf("parent config invented an inner project's testing role: %v / %v", got, err)
	}
}
