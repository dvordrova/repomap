package report

import (
	"bytes"
	"fmt"
	"html/template"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/cproject"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

// Real clang declarations from the cumulative C fixture pass through the
// ordinary atlas projection and target template. Refusing the map must leave
// their exact declaration keys and source links available outside any group.
func TestNativeRefusedMapKeepsOriginalHelperSourcesInTheOrdinaryReport(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join("..", "..", "testdata", "repositories", "c")
	err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		dest := filepath.Join(root, relative)
		if entry.IsDir() {
			return os.MkdirAll(dest, 0755)
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dest, body, 0644)
	})
	if err != nil {
		t.Fatal(err)
	}
	runManifestGit(t, root, "init", "--quiet")
	runManifestGit(t, root, "add", ".")
	runManifestGit(t, root, "-c", "user.name=fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "--quiet", "-m", "fixture")
	repository, err := corpus.Open(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	project, err := cproject.Discover(t.Context(), root, repository)
	if err != nil {
		t.Fatal(err)
	}
	if project == nil || project.Toolchain.Err != "" {
		t.Fatalf("real C fixture toolchain unavailable: %+v", project)
	}
	var native cproject.Program
	found := false
	for _, candidate := range project.Programs {
		if candidate.Selector == "c:kvd" {
			native = candidate
			found = true
			break
		}
	}
	if !found {
		t.Fatal("cumulative kvd program missing")
	}
	parsed, err := cproject.Parse(t.Context(), root, repository, native, cproject.NewStore())
	if err != nil {
		t.Fatal(err)
	}
	result, err := cproject.Index(repository, parsed)
	if err != nil {
		t.Fatal(err)
	}
	index, err := programindex.New(result.Input)
	if err != nil {
		t.Fatal(err)
	}
	original, err := programindex.Encode(index)
	if err != nil {
		t.Fatal(err)
	}
	target := atlas.Target{ID: index.Target.ID, Name: index.Target.Name, Kind: string(index.Target.Kind), Language: "c", Root: ".", MapFailure: atlas.MapFailureRefused, Zones: []atlas.Zone{}, Boxes: []atlas.Box{}, Arrows: []atlas.Arrow{}, Boundaries: []atlas.Boundary{}}
	for _, object := range index.Objects {
		if object.Location == nil {
			continue
		}
		target.OffMap = append(target.OffMap, atlas.OffMapFile{ID: object.ID, Reason: atlas.OffMapFailure, File: atlas.File{Path: object.Location.Path, Source: atlas.SourceModel, Symbols: []atlas.Symbol{{ID: "s" + object.ID, ObjectID: index.Target.ID + "." + object.ID, Name: object.Name, Kind: string(object.Kind), LineNo: object.Location.Line, Column: object.Location.Column}}}})
	}
	want := map[string]programindex.Object{}
	for _, object := range index.Objects {
		if object.Location == nil {
			continue
		}
		if object.Name == "netListen" && object.Location.Path == "net.c" || object.Name == "sbAppend" && object.Location.Path == "strbuf.c" || object.Name == "loopMain" && object.Location.Path == "loop.c" {
			want[object.Name] = object
		}
	}
	if len(want) != 3 {
		t.Fatalf("real native helper evidence missing: %+v", want)
	}
	proposal := atlas.RefusedPart{Name: "Convenience helpers", Holds: "Utility work without an established common responsibility.", Reason: atlas.PartsIndependentJobs}
	for _, name := range []string{"netListen", "sbAppend", "loopMain"} {
		proposal.MemberIDs = append(proposal.MemberIDs, index.Target.ID+"."+want[name].ID)
	}
	target.RefusedParts = []atlas.RefusedPart{proposal}
	grouped, err := groupindex.ProjectAtlas(map[string]programindex.Index{index.Target.ID: index}, atlas.Atlas{Version: atlas.Version, Repository: "cumulative", Targets: []atlas.Target{target}, Joints: []atlas.Joint{}, Diagnostics: []atlas.Diagnostic{}})
	if err != nil {
		t.Fatal(err)
	}
	// Cross the real saved-overlay seam before consuming source members.
	encoded, err := groupindex.Encode(grouped[0])
	if err != nil {
		t.Fatal(err)
	}
	graph, err := groupindex.Decode(encoded, index)
	if err != nil {
		t.Fatal(err)
	}
	data := &ReportData{}
	if err := BindProgramPortfolio(data, index.Target.ID, []programindex.Index{index}); err != nil {
		t.Fatal(err)
	}
	section := &pageSection{ID: "t1", programTargetID: index.Target.ID, ShortLabel: "kvd", FactsAvailable: true}
	builder := &pageBuilder{data: data, indexes: []groupindex.Index{graph}, byProgram: map[string]*pageSection{index.Target.ID: section}, subjects: map[string]subjectRef{}, links: pageLinks{repositoryURL: "https://github.com/example/kvd", blobPrefix: "/blob/", revision: strings.Repeat("a", 40)}}
	for _, subject := range graph.Subjects {
		builder.subjects[subjectKey(index.Target.ID, subject.ID)] = subjectRef{subject: subject, programTargetID: index.Target.ID}
	}
	builder.fillSectionOffMap(section)
	seen := map[string]bool{}
	for _, file := range section.OffMap {
		for _, chip := range file.Members {
			if object, ok := want[chip.Name]; ok {
				if chip.Anchor.Key() != groupindex.DeclarationKey(object) || chip.Anchor.Path != object.Location.Path || chip.Line != object.Location.Line {
					t.Fatalf("wrong native source binding: %+v", chip)
				}
				seen[chip.Name] = true
			}
		}
	}
	if len(seen) != 3 || len(section.RefusedParts) != 1 || len(section.RefusedParts[0].Members) != 3 || len(graph.Groups) != 0 {
		t.Fatalf("refused original code not readable: %+v %+v", seen, section.RefusedParts)
	}
	tmpl, err := template.New("report").Funcs(pageTemplateFuncs(English)).ParseFS(reportTemplateFS, "templates/html/*.html")
	if err != nil {
		t.Fatal(err)
	}
	var html bytes.Buffer
	if err := tmpl.ExecuteTemplate(&html, "target.html", section); err != nil {
		t.Fatal(err)
	}
	for name, object := range want {
		href := fmt.Sprintf("https://github.com/example/kvd/blob/%s/%s#L%d", strings.Repeat("a", 40), object.Location.Path, object.Location.Line)
		for _, text := range []string{name, href} {
			if !strings.Contains(html.String(), template.HTMLEscapeString(text)) {
				t.Fatalf("ordinary report lost %q", text)
			}
		}
	}
	if !strings.Contains(html.String(), "The map of parts is unavailable") || !strings.Contains(html.String(), "Groupings not established") {
		t.Fatal("refusal not explicit in ordinary report")
	}
	after, err := programindex.Encode(index)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(original, after) {
		t.Fatal("source access changed native authority")
	}
}
