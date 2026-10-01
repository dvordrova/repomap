package pythonprogramindex

import (
	"context"
	"reflect"
	"testing"

	"github.com/dvordrova/repomap/internal/pythontarget"
)

func TestCumulativePytestMetadataPreservesAllIndexedDeclarations(t *testing.T) {
	repository := pythonCorpus(t, cumulativePythonSources(t, "src/fixture_app/test_market.py", "src/fixture_app/runtime_registrations.py", "tests/__init__.py", "tests/conftest.py", "tests/test_facade.py", "tests/sample_orders.py"))
	index, err := buildOneForTest(t.Context(), repository, targetOfKind(t, repository, pythontarget.KindLibrary))
	if err != nil {
		t.Fatal(err)
	}
	tests := make(map[string]bool)
	for _, source := range index.Target.TestSources {
		tests[source] = true
	}
	// tests/ holds test modules beside the packages the build declares: its
	// package file and its helper module are test code too. test_market.py
	// inside the declared fixture_app package is selected alone.
	if !reflect.DeepEqual(tests, map[string]bool{"src/fixture_app/test_market.py": true, "tests/__init__.py": true, "tests/conftest.py": true, "tests/test_facade.py": true, "tests/sample_orders.py": true}) {
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
		{"runner defaults", "[project]\nname='sample'\ndependencies=['pytest>=9']\n[tool.pytest]\n", map[string]bool{"test_ready.py": true, "ready_test.py": true, "testing/conftest.py": true}},
		{"authored override", "[project]\nname='sample'\ndependencies=['pytest']\n[tool.pytest.ini_options]\npython_files=['check_*.py']\n", map[string]bool{"check_ready.py": true, "testing/conftest.py": true}},
		{"authored config with external dev dependencies", "[project]\nname='sample'\n[tool.pytest.ini_options]\npython_files=['check_*.py']\n", map[string]bool{"check_ready.py": true, "testing/conftest.py": true}},
		{"unrelated table", "[project]\nname='sample'\ndependencies=['pytest']\n[tool.another]\npython_files=['test_*.py']\n", map[string]bool{}},
		{"invalid setting", "[project]\nname='sample'\ndependencies=['pytest']\n[tool.pytest]\npython_files=42\n", map[string]bool{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository := pythonCorpus(t, map[string]string{"pyproject.toml": test.config, "test_ready.py": "def check(): pass", "ready_test.py": "def check(): pass", "check_ready.py": "def check(): pass", "testing/service.py": "def serve(): pass", "testing/conftest.py": "def pytest_configure(config): pass"})
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

// A test directory is the topmost directory below a resolved pytest
// table's own directory that holds a selected test module while no file
// under it is a module the build declares or a program's launch file, in a
// project declaring its packages: every Python file in it is test code.
func TestATestDirectoryBesideTheDeclaredPackagesIsTestCode(t *testing.T) {
	config := "[project]\nname='sample'\ndependencies=['pytest']\n[project.scripts]\nsample='sample.cli:main'\n" +
		"[tool.setuptools.packages.find]\ninclude=['sample*']\n[tool.pytest]\n"
	files := map[string]string{
		"pyproject.toml": config, "sample/__init__.py": "", "sample/cli.py": "def main(): pass", "sample/test_cli.py": "def test_main(): pass",
		"tests/__init__.py": "", "tests/conftest.py": "", "tests/trades.py": "TRADES = []", "tests/strats/buy.py": "class Buy: pass",
		"tests/strats/test_buy.py": "def test_buy(): pass", "tools/lint.py": "def lint(): pass",
	}
	for _, launched := range []bool{false, true} {
		// A script launched from tests/ (a `__main__` guard no build names)
		// is test code with it, never a block: beets' test/testall.py.
		if launched {
			files["tests/run_checks.py"] = "if __name__ == '__main__':\n    print('checks')\n"
		}
		repository := pythonCorpus(t, files)
		catalog, err := pythontarget.Discover(context.Background(), repository)
		if err != nil {
			t.Fatal(err)
		}
		got, err := pythonTestSources(repository, catalog.Entries)
		if err != nil {
			t.Fatal(err)
		}
		want := map[string]bool{"sample/test_cli.py": true, "tests/conftest.py": true, "tests/strats/test_buy.py": true, "tests/strats/buy.py": true,
			"tests/__init__.py": true, "tests/trades.py": true}
		if launched {
			want["tests/run_checks.py"] = true
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("launched=%v: test sources %v, want %v", launched, got, want)
		}
	}
}

// pytest reads its INI configurations too: setup.cfg's [tool:pytest] (beets),
// tox.ini's and pytest.ini's [pytest], with its default patterns when the
// project declares pytest; pytest.ini comes before a pyproject.toml table.
func TestPytestINIConfigurationsSelectTests(t *testing.T) {
	files := map[string]string{"test_ready.py": "def check(): pass", "check_ready.py": "def check(): pass"}
	for _, test := range []struct {
		name  string
		files map[string]string
		want  map[string]bool
	}{
		{"setup.cfg with a declared pytest", map[string]string{"pyproject.toml": "[project]\nname='s'\n[project.optional-dependencies]\ntest=['pytest']\n", "setup.cfg": "[tool:pytest]\naddopts =\n    -ra\n"}, map[string]bool{"test_ready.py": true}},
		{"setup.cfg requiring pytest itself", map[string]string{"setup.cfg": "[options.extras_require]\ntest =\n    pytest>=7\n[tool:pytest]\n"}, map[string]bool{"test_ready.py": true}},
		{"setup.cfg without a runner", map[string]string{"setup.cfg": "[tool:pytest]\n"}, map[string]bool{}},
		{"tox.ini's own patterns", map[string]string{"tox.ini": "[pytest]\npython_files = check_*.py\n"}, map[string]bool{"check_ready.py": true}},
		{"pytest.ini before pyproject", map[string]string{"pyproject.toml": "[project]\nname='s'\ndependencies=['pytest']\n[tool.pytest]\n", "pytest.ini": "[pytest]\npython_files = check_*.py\n"}, map[string]bool{"check_ready.py": true}},
	} {
		t.Run(test.name, func(t *testing.T) {
			all := map[string]string{}
			for name, content := range files {
				all[name] = content
			}
			for name, content := range test.files {
				all[name] = content
			}
			got, err := configuredPythonTests(pythonCorpus(t, all))
			if err != nil || !reflect.DeepEqual(got, test.want) {
				t.Fatalf("tests %v, want %v: %v", got, test.want, err)
			}
		})
	}
}
