package repoconfig

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLocalConfigCreatesOnceAndNeverInherits(t *testing.T) {
	root := t.TempDir()
	created, err := Ensure(root)
	if err != nil || !created {
		t.Fatalf("create: %t %v", created, err)
	}
	config, err := Load(root)
	if err != nil || !reflect.DeepEqual(config, defaults()) {
		t.Fatalf("initial config: %+v %v", config, err)
	}
	filename := filepath.Join(root, Filename)
	custom := "# Keep my comment.\neditor: [my-editor, '+{{ .Line }}', '{{ .File }}']\n"
	if err := os.WriteFile(filename, []byte(custom), 0o600); err != nil {
		t.Fatal(err)
	}
	if created, err := Ensure(root); err != nil || created {
		t.Fatalf("ensure: %t %v", created, err)
	}
	data, err := os.ReadFile(filename)
	if err != nil || string(data) != custom {
		t.Fatalf("existing config changed: %q %v", data, err)
	}
	child := filepath.Join(root, "another repository")
	if err := os.Mkdir(child, 0o700); err != nil {
		t.Fatal(err)
	}
	config, err = Load(child)
	if err != nil || !reflect.DeepEqual(config, defaults()) {
		t.Fatalf("parent settings leaked: %+v %v", config, err)
	}
}

func TestEditorTemplatesPreserveArgumentsAndCoordinates(t *testing.T) {
	file := "/repo with spaces/$(literal) 'file'.go"
	config := Config{Editor: []string{"/editor with spaces/code", "--goto", "{{ .File }}:{{ .Line }}:{{ .Column }}", ""}}
	got, err := config.Command(file, 42, 7)
	want := []string{"/editor with spaces/code", "--goto", file + ":42:7", ""}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("command: %q %v", got, err)
	}
	got, err = defaults().Command(file, 0, 0)
	if err != nil || got[2] != file+":1:1" {
		t.Fatalf("file-only command: %q %v", got, err)
	}
}

func TestConfigRejectsUnsupportedSettingsAndBrokenEditor(t *testing.T) {
	for _, input := range []string{
		"variants: []\n", "questions: [\"  \"]\n", "editor: []\n", "editor: null\n", "editor: ['']\n",
		"editor: code\n", "editor: [code, '{{ .Unknown }}']\n", "editor: [code, '{{']\n",
		"editor: [code]\n---\neditor: [other]\n", "editor: [code]\neditor: [other]\n",
	} {
		t.Run(input, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, Filename), []byte(input), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(root); err == nil || !strings.Contains(err.Error(), Filename) {
				t.Fatalf("accepted invalid config or omitted its path: %v", err)
			}
		})
	}
}
