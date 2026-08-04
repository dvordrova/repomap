package orient

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/gofacts"
)

func TestExactPackageDeclarationLocationsUsesFirstExactFilteredBuildFile(t *testing.T) {
	t.Parallel()
	repository := t.TempDir()
	writePackageDeclarationTestFile(t, repository, "a.go", "// generated header\npackage fixture\n")
	writePackageDeclarationTestFile(t, repository, "z.go", "package fixture\n")
	writePackageDeclarationTestFile(t, repository, "not-filtered.go", "package fixture\n")

	facts := gofacts.Facts{Packages: []gofacts.PackageFact{{
		CanonicalPath: "example.com/fixture", Name: "fixture",
		Files: []string{"z.go", "not-filtered.go", "a.go", "README.md", "a.go"},
	}}}
	got, err := exactPackageDeclarationLocations(
		context.Background(), repository, []string{"z.go", "a.go"}, facts,
	)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]evidence.Location{
		"example.com/fixture": {Path: "a.go", Line: 2, Column: 1},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("package declarations = %#v, want %#v", got, want)
	}

	reversed := facts
	reversed.Packages = append([]gofacts.PackageFact(nil), facts.Packages...)
	reversed.Packages[0].Files = []string{"a.go", "z.go"}
	again, err := exactPackageDeclarationLocations(
		context.Background(), repository, []string{"a.go", "z.go"}, reversed,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(again, want) {
		t.Fatalf("reordered package declarations = %#v, want %#v", again, want)
	}
}

func TestExactPackageDeclarationLocationsLeavesFailedPackageUnavailable(t *testing.T) {
	t.Parallel()
	repository := t.TempDir()
	writePackageDeclarationTestFile(t, repository, "a.go", "package wrong\n")
	writePackageDeclarationTestFile(t, repository, "b.go", "package fixture\n")

	facts := gofacts.Facts{Packages: []gofacts.PackageFact{{
		CanonicalPath: "example.com/fixture", Name: "fixture",
		Files: []string{"b.go", "a.go"},
	}}}
	got, err := exactPackageDeclarationLocations(
		context.Background(), repository, []string{"a.go", "b.go"}, facts,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("package mismatch fell through to a later file: %#v", got)
	}
}

func TestExactPackageDeclarationLocationsRejectsSymlinkAndInvalidUTF8(t *testing.T) {
	t.Parallel()
	repository := t.TempDir()
	writePackageDeclarationTestFile(t, repository, "real.go", "package fixture\n")
	if err := os.Symlink("real.go", filepath.Join(repository, "alias.go")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repository, "invalid.go"), []byte("package invalid\n\xff"), 0o600); err != nil {
		t.Fatal(err)
	}
	facts := gofacts.Facts{Packages: []gofacts.PackageFact{
		{CanonicalPath: "example.com/fixture", Name: "fixture", Files: []string{"alias.go"}},
		{CanonicalPath: "example.com/invalid", Name: "invalid", Files: []string{"invalid.go"}},
	}}
	got, err := exactPackageDeclarationLocations(
		context.Background(), repository, []string{"alias.go", "invalid.go"}, facts,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("unsafe source became package evidence: %#v", got)
	}
}

func TestExactPackageDeclarationLocationsPropagatesCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := exactPackageDeclarationLocations(ctx, t.TempDir(), nil, gofacts.Facts{}); err == nil {
		t.Fatal("canceled package-declaration extraction succeeded")
	}
}

func writePackageDeclarationTestFile(t *testing.T, repository, sourcePath, content string) {
	t.Helper()
	fullPath := filepath.Join(repository, sourcePath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
