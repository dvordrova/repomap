package envfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadParsesWithoutShellEvaluation(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	data := []byte("# comment\nREPOMAP_ENVFILE_SECRET='literal-$(do-not-run)'\nREPOMAP_ENVFILE_MODEL=deepseek-v4-flash\n")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	t.Setenv("REPOMAP_ENVFILE_SECRET", "")
	if err := os.Unsetenv("REPOMAP_ENVFILE_SECRET"); err != nil {
		t.Fatalf("Unsetenv() error = %v", err)
	}
	t.Setenv("REPOMAP_ENVFILE_MODEL", "")
	if err := os.Unsetenv("REPOMAP_ENVFILE_MODEL"); err != nil {
		t.Fatalf("Unsetenv() error = %v", err)
	}

	if err := Load(path); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got := os.Getenv("REPOMAP_ENVFILE_SECRET"); got != "literal-$(do-not-run)" {
		t.Fatalf("secret = %q, want literal value", got)
	}
	if got := os.Getenv("REPOMAP_ENVFILE_MODEL"); got != "deepseek-v4-flash" {
		t.Fatalf("model = %q", got)
	}
}

func TestLoadDoesNotOverrideExistingEnvironment(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("REPOMAP_ENVFILE_EXISTING=from-file\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	t.Setenv("REPOMAP_ENVFILE_EXISTING", "from-environment")

	if err := Load(path); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got := os.Getenv("REPOMAP_ENVFILE_EXISTING"); got != "from-environment" {
		t.Fatalf("existing value = %q", got)
	}
}

func TestLoadIgnoresMissingFile(t *testing.T) {
	t.Parallel()

	if err := Load(filepath.Join(t.TempDir(), "missing.env")); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
}
