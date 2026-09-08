package run

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"testing"

	"github.com/dvordrova/repomap/internal/report"
)

func TestStandaloneReportKeepsUncommittedSourcesWithoutBrokenLinks(t *testing.T) {
	for _, host := range []string{"github", "gitlab"} {
		t.Run(host, func(t *testing.T) {
			repositoryRoot := ordinaryGraphGoRepository(t)
			for path, contents := range map[string]string{
				"internal/work/work.go": "package work\nfunc Run() { extra() }\n",
				"internal/work/new.go":  "package work\nfunc extra() {}\n",
			} {
				if err := os.WriteFile(filepath.Join(repositoryRoot, path), []byte(contents), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			runRoot := t.TempDir()
			args := []string{"--no-model", "--target", "example.com/common-page@.::example.com/common-page/cmd/app",
				"--no-serve", "--no-open", "--debug-dir", runRoot,
				"--" + host + "-url", "https://" + host + ".com/team/project"}
			if err := runDefaultWithDeps(repositoryRoot, args, defaultRunDeps{stdout: io.Discard, stderr: io.Discard}); err != nil {
				t.Fatalf("ordinary standalone publication: %v", err)
			}
			runs := ordinaryGraphRunDirs(t, runRoot)
			if len(runs) != 1 {
				t.Fatalf("published common reports = %v", runs)
			}
			receipt, err := report.ReadRunReceipt(runs[0])
			if err != nil {
				t.Fatal(err)
			}
			manifest := receipt.Manifest()
			want := []string{"internal/work/new.go", "internal/work/work.go"}
			if manifest.StandaloneSource == nil || !reflect.DeepEqual(manifest.StandaloneSource.UnavailablePaths, want) {
				t.Fatalf("per-path source availability = %+v", manifest.StandaloneSource)
			}
			html, err := os.ReadFile(filepath.Join(runs[0], receipt.HTMLFilename()))
			if err != nil {
				t.Fatal(err)
			}
			blob := "/blob/"
			if host == "gitlab" {
				blob = "/-/blob/"
			}
			prefix := "https://" + host + ".com/team/project" + blob + manifest.RepositoryState.Head + "/"
			if !bytes.Contains(html, []byte(prefix+"cmd/app/main.go#L")) {
				t.Fatal("clean source lost its remote link")
			}
			for _, sourcePath := range want {
				if !bytes.Contains(html, []byte(sourcePath)) || bytes.Contains(html, []byte(prefix+sourcePath)) {
					t.Fatalf("source %q was removed or still has a broken remote link", sourcePath)
				}
			}
			if !bytes.Contains(html, []byte("title=\"No source\"")) {
				t.Fatal("unavailable source has no hover explanation")
			}
			canonical, err := os.ReadFile(filepath.Join(runs[0], "report.json"))
			if err != nil || bytes.Contains(canonical, []byte("unavailable_source_paths")) {
				t.Fatalf("source link presentation leaked into semantic report: %v", err)
			}
			// Saved rendering must not inspect the checkout again.
			movedRoot := repositoryRoot + "-moved"
			if err := os.Rename(repositoryRoot, movedRoot); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Rename(movedRoot, repositoryRoot) })
			restored, err := report.RenderSavedHTML(runs[0])
			// The ordinary English pass assigns display refs before publishing;
			// saved rendering may renumber those refs. Compare the source markup.
			anchors := regexp.MustCompile(`<(?:a|span) class="anchor"[^>]*>[^<]*</(?:a|span)>`)
			if err != nil || !reflect.DeepEqual(anchors.FindAll(html, -1), anchors.FindAll(restored, -1)) {
				t.Fatalf("saved HTML lost exact source availability: %v", err)
			}
		})
	}
}
