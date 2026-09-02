package claims

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/corpus"
)

const (
	fixtureFirstDate  = "2024-01-10T10:00:00+00:00"
	fixtureSecondDate = "2024-02-20T10:00:00+02:00"
	fixtureAgeDays    = 41
)

func TestExtractQuotesFixtureRepository(t *testing.T) {
	repository, head := fixtureRepository(t)
	result := extractFixture(t, repository, head, Input{
		Targets: []TargetRoot{{ID: "t:backend", Root: "backend"}, {ID: "t:front", Root: "front"}},
	})

	if result.AsOf != "2024-02-20" {
		t.Fatalf("as_of = %q, want revision commit date", result.AsOf)
	}
	if result.Dropped != 1 {
		t.Fatalf("dropped = %d, want the credential-shaped comment withheld", result.Dropped)
	}
	bySource := map[Source][]Claim{}
	for _, claim := range result.Claims {
		bySource[claim.Source] = append(bySource[claim.Source], claim)
	}

	commits := bySource[SourceCommit]
	if len(commits) != 2 {
		t.Fatalf("commit claims = %d, want 2: %+v", len(commits), commits)
	}
	for _, claim := range commits {
		if len(claim.Commit) != shortCommitLength || claim.Path != "" {
			t.Fatalf("commit claim %+v is not short-sha addressed", claim)
		}
	}
	assertClaim(t, commits, Claim{Source: SourceCommit, Text: "Add frontend with a note", Date: "2024-02-20"})
	assertClaim(t, commits, Claim{Source: SourceCommit, Text: "Initial backend", Date: "2024-01-10"})

	assertClaim(t, bySource[SourceReadme], Claim{
		Source: SourceReadme, Path: "README.md", Line: 1, Date: "2024-01-10", AgeDays: fixtureAgeDays,
		Text: "# Tutorial Game A tiny game used to teach Python. Run `python main.py` to start.",
	})
	assertClaim(t, bySource[SourceDocstring], Claim{
		Source: SourceDocstring, Path: "backend/app.py", Line: 1, Date: "2024-01-10",
		AgeDays: fixtureAgeDays, TargetID: "t:backend", Text: "Backend entry module. Serves levels over HTTP.",
	})
	assertClaim(t, bySource[SourceDocstring], Claim{
		Source: SourceDocstring, Path: "backend/app.py", Line: 8, Date: "2024-01-10",
		AgeDays: fixtureAgeDays, TargetID: "t:backend", Text: "Return every level in order.",
	})
	assertClaim(t, bySource[SourceDocstring], Claim{
		Source: SourceDocstring, Path: "tools/gen.go", Line: 3, Date: "2024-01-10",
		AgeDays: fixtureAgeDays, Text: "Generate writes the level table. It is idempotent.",
	})
	assertClaim(t, bySource[SourceDocstring], Claim{
		Source: SourceDocstring, Path: "front/src/http.ts", Line: 1, Date: "2024-02-20",
		TargetID: "t:front", Text: "Fetch levels from the backend. The base URL comes from the proxy.",
	})
	assertClaim(t, bySource[SourceComment], Claim{
		Source: SourceComment, Path: "front/src/http.ts", Line: 8, Date: "2024-02-20",
		TargetID: "t:front", Text: "NOTE: the backend must be running on port 8000.",
	})
	if len(bySource[SourceComment]) != 1 {
		t.Fatalf("comment claims = %+v, want only the NOTE line (TODO is a fact)", bySource[SourceComment])
	}
}

func TestExtractIsDeterministicAndHonorsCommitLimit(t *testing.T) {
	repository, head := fixtureRepository(t)
	first := extractFixture(t, repository, head, Input{})
	second := extractFixture(t, repository, head, Input{})
	firstBytes, err := Encode(first)
	if err != nil {
		t.Fatal(err)
	}
	secondBytes, err := Encode(second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstBytes, secondBytes) || first.SHA256 != second.SHA256 {
		t.Fatalf("two extractions differ:\n%s\n%s", firstBytes, secondBytes)
	}

	limited := extractFixture(t, repository, head, Input{CommitLimit: 1})
	commits := 0
	for _, claim := range limited.Claims {
		if claim.Source == SourceCommit {
			commits++
			if claim.Date != "2024-02-20" {
				t.Fatalf("limited window kept %+v instead of the newest commit", claim)
			}
		}
	}
	if commits != 1 {
		t.Fatalf("commit claims under limit 1 = %d", commits)
	}
}

func TestExtractRejectsIncompleteInput(t *testing.T) {
	if _, err := Extract(context.Background(), Input{Revision: "abc"}); err == nil {
		t.Fatal("missing repository path and corpus were accepted")
	}
}

func extractFixture(t *testing.T, repository, head string, input Input) Result {
	t.Helper()
	repositoryCorpus, err := corpus.Open(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	defer repositoryCorpus.Close()
	input.Revision = head
	input.RepoPath = repository
	input.Repository = repositoryCorpus
	result, err := Extract(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if err := result.Validate(); err != nil {
		t.Fatal(err)
	}
	return result
}

func assertClaim(t *testing.T, claims []Claim, want Claim) {
	t.Helper()
	for _, claim := range claims {
		if claim.Text != want.Text {
			continue
		}
		got := claim
		got.ID, got.Commit = "", ""
		if got != want {
			t.Fatalf("claim %q:\n got %+v\nwant %+v", want.Text, got, want)
		}
		return
	}
	t.Fatalf("no claim with text %q among %+v", want.Text, claims)
}

// fixtureRepository builds a two-commit repository with fixed dates so ages
// are stable: README, Python and Go land first; the TypeScript file later.
func fixtureRepository(t *testing.T) (string, string) {
	t.Helper()
	directory := t.TempDir()
	fixtureGit(t, directory, "init", "--quiet")
	fixtureGit(t, directory, "config", "user.email", "claims@example.test")
	fixtureGit(t, directory, "config", "user.name", "claims test")

	writeFixtureFile(t, directory, "README.md", "# Tutorial Game\n\nA tiny game used to teach Python.\n\n```sh\npip install -r requirements.txt\n```\n\nRun `python main.py` to start.\n")
	writeFixtureFile(t, directory, "backend/app.py", strings.Join([]string{
		`"""Backend entry module.`,
		``,
		`Serves levels over HTTP.`,
		`"""`,
		``,
		``,
		`def levels():`,
		`    """Return every level in order."""`,
		`    # TODO: paginate`,
		`    return []`,
		`# NOTE: api_key = "s3cr3tvalue1234"`,
		``,
	}, "\n"))
	writeFixtureFile(t, directory, "tools/gen.go", "package tools\n\n// Generate writes the level table.\n// It is idempotent.\nfunc Generate() {}\n")
	fixtureGit(t, directory, "add", "--all")
	fixtureCommit(t, directory, fixtureFirstDate, "Initial backend")

	writeFixtureFile(t, directory, "front/src/http.ts", strings.Join([]string{
		`/**`,
		` * Fetch levels from the backend.`,
		` * The base URL comes from the proxy.`,
		` * @returns the level list`,
		` */`,
		`export async function fetchLevels() {`,
		`  const url = "/api/levels";`,
		`  // NOTE: the backend must be running on port 8000.`,
		`  return fetch(url);`,
		`}`,
		``,
	}, "\n"))
	fixtureGit(t, directory, "add", "--all")
	fixtureCommit(t, directory, fixtureSecondDate, "Add frontend with a note")
	return directory, strings.TrimSpace(fixtureGitOutput(t, directory, "rev-parse", "HEAD"))
}

func fixtureCommit(t *testing.T, directory, date, subject string) {
	t.Helper()
	command := fixtureGitCommand(t, directory, "commit", "--quiet", "--no-gpg-sign", "-m", subject)
	command.Env = append(command.Env, "GIT_AUTHOR_DATE="+date, "GIT_COMMITTER_DATE="+date)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v: %s", err, output)
	}
}

func fixtureGit(t *testing.T, directory string, arguments ...string) {
	t.Helper()
	fixtureGitOutput(t, directory, arguments...)
}

func fixtureGitOutput(t *testing.T, directory string, arguments ...string) string {
	t.Helper()
	output, err := fixtureGitCommand(t, directory, arguments...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", arguments, err, output)
	}
	return string(output)
}

func fixtureGitCommand(t *testing.T, directory string, arguments ...string) *exec.Cmd {
	t.Helper()
	command := exec.CommandContext(t.Context(), "git", append([]string{"-C", directory}, arguments...)...)
	command.Env = append(os.Environ(),
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_CONFIG_SYSTEM="+os.DevNull,
	)
	return command
}

func writeFixtureFile(t *testing.T, root, relative, content string) {
	t.Helper()
	fullPath := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
