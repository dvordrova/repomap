package claims

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/corpus"
)

func TestExtractKeepsClaimsAcrossFormerWholeFileLimit(t *testing.T) {
	const formerLimit = 1 << 20
	const note = "NOTE: count describes a quantity, not a list of levels."
	for _, newline := range []string{"\n", "\r\n"} {
		name := "LF"
		if newline == "\r\n" {
			name = "CRLF"
		}
		t.Run(name, func(t *testing.T) {
			directory := t.TempDir()
			fixtureGit(t, directory, "init", "--quiet")
			fixtureGit(t, directory, "config", "user.email", "claims@example.test")
			fixtureGit(t, directory, "config", "user.name", "claims test")
			fixtureGit(t, directory, "config", "core.autocrlf", "false")
			type source struct {
				path, before, after string
				want                Claim
			}
			var sources []source
			for _, relative := range []string{
				"README.md",
				"go/internal/storefixture/level_responses.go",
				"python/src/fixture_app/models.py",
				"jsts/src/type-members.ts",
			} {
				data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "repositories", filepath.FromSlash(relative)))
				if err != nil {
					t.Fatal(err)
				}
				text := strings.ReplaceAll(string(data), "\n", newline)
				cut := 0
				if relative != "README.md" {
					marker := strings.Index(text, note)
					if marker < 0 {
						t.Fatalf("cumulative source %s has no count claim", relative)
					}
					cut = strings.LastIndex(text[:marker], "\n") + 1
				}
				s := source{path: relative, before: text[:cut], after: text[cut:]}
				if relative != "README.md" {
					s.want = Claim{Source: SourceComment, Path: relative,
						Line: strings.Count(s.before, "\n") + 2, Text: note, Date: "2024-01-10",
						TargetID: strings.SplitN(relative, "/", 2)[0]}
				}
				sources = append(sources, s)
				writeFixtureFile(t, directory, relative, s.before+" "+newline+s.after)
			}
			fixtureGit(t, directory, "add", "--all")
			fixtureCommit(t, directory, fixtureFirstDate, "Record cumulative claims")
			head := strings.TrimSpace(fixtureGitOutput(t, directory, "rev-parse", "HEAD"))
			input := Input{Targets: []TargetRoot{{ID: "go", Root: "go"}, {ID: "python", Root: "python"}, {ID: "jsts", Root: "jsts"}}}
			baseline := extractFixture(t, directory, head, input)
			for _, s := range sources {
				if s.path != "README.md" {
					var owned []Claim
					for _, claim := range baseline.Claims {
						if claim.Path == s.path {
							owned = append(owned, claim)
						}
					}
					assertClaim(t, owned, s.want)
					continue
				}
				found := false
				for _, claim := range baseline.Claims {
					if claim.Path == s.path && claim.Source == SourceReadme && claim.Line == 2 && strings.HasPrefix(claim.Text, "Cumulative language repositories — This directory contains exactly one small real repository") {
						found = true
					}
				}
				if !found {
					t.Fatal("cumulative README has no claim at its original heading")
				}
			}
			want, err := Encode(baseline)
			if err != nil {
				t.Fatal(err)
			}
			for _, boundary := range []struct {
				name string
				size int
			}{
				{"below", formerLimit - 1},
				{"exact", formerLimit},
				{"above", formerLimit + 1},
				{"quote_beyond", 2 * formerLimit},
			} {
				t.Run(boundary.name, func(t *testing.T) {
					for _, s := range sources {
						padding := boundary.size - len(s.before) - len(newline) - len(s.after)
						text := s.before + strings.Repeat(" ", padding) + newline + s.after
						writeFixtureFile(t, directory, s.path, text)
						if boundary.name == "quote_beyond" && len(s.before)+padding+len(newline) <= formerLimit {
							t.Fatalf("%s quote must start beyond the former read boundary", s.path)
						}
					}
					got := extractFixture(t, directory, head, input)
					encoded, err := Encode(got)
					if err != nil {
						t.Fatal(err)
					}
					if !bytes.Equal(encoded, want) {
						for _, s := range sources {
							t.Errorf("%s (%d bytes): claims %d, want %d", s.path, boundary.size, claimCountForPath(got, s.path), claimCountForPath(baseline, s.path))
						}
						t.Fatal("padding changed quoted text, physical anchors, IDs, dates, target binding, or seal")
					}
				})
			}
		})
	}
}

func claimCountForPath(result Result, path string) int {
	count := 0
	for _, claim := range result.Claims {
		if claim.Path == path {
			count++
		}
	}
	return count
}

func TestFileQuotesStillRejectsNonUTF8AfterFormerReadBoundary(t *testing.T) {
	directory := t.TempDir()
	writeFixtureFile(t, directory, "source.py", "# NOTE: original quote.\n"+strings.Repeat(" ", 1<<20)+"\xff")
	repository, err := corpus.Open(context.Background(), directory)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	quotes, err := fileQuotes(repository, repository.Entries()[0])
	if err != nil || len(quotes) != 0 {
		t.Fatalf("non-UTF-8 file quotes = %v, err = %v", quotes, err)
	}
}
