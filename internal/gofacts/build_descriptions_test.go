package gofacts

import (
	"reflect"
	"testing"
)

// litestream's Makefile builds cmd/litestream-vfs with its tags written out
// on one line and, for each platform, through VFS_BUILD_TAGS and VFS_SRC on
// the others; its `go test -tags=vfs` builds no program.
func TestMakefileCommandsReadGoBuildTags(t *testing.T) {
	makefile := "VFS_BUILD_TAGS := vfs,SQLITE3VFS_LOADABLE_EXT\n" + // 1
		"VFS_SRC := ./cmd/litestream-vfs\n" + // 2
		"GO ?= go\n" + // 3
		"ifeq ($(OS),linux)\n" + // 4
		"EXTRA := -tags=linuxonly\n" + // 5
		"endif\n" + // 6
		"vfs:\n" + // 7
		"\tgo build -tags vfs,SQLITE3VFS_LOADABLE_EXT -o dist/litestream-vfs.a -buildmode=c-archive ./cmd/litestream-vfs\n" + // 8
		"vfs-linux:\n" + // 9
		"\tCGO_ENABLED=1 GOOS=linux \\\n" + // 10
		"\t\t$(GO) build -tags $(VFS_BUILD_TAGS) -o dist/x.a $(VFS_SRC) > build.log 2>&1\n" + // 11
		"vfs-test:\n" + // 12
		"\tgo test -v -tags=vfs ./cmd/litestream-vfs\n" + // 13
		"sub:\n" + // 14
		"\tcd tools && go install -tags=tool .\n" + // 15
		"flags:\n" + // 16
		"\tgo build $(EXTRA) ./cmd/extra\n" + // 17
		"env:\n" + // 18
		"\tgo build -tags \"$$TAGS\" ./cmd/env\n" + // 19
		"files:\n" + // 20
		"\t@go build -C cmd -tags=a,b main.go\n" // 21
	got := makefileCommands("build", []byte(makefile))
	want := []buildCommand{
		{cwd: "build", tags: []string{"SQLITE3VFS_LOADABLE_EXT", "vfs"}, packages: []string{"./cmd/litestream-vfs"}, line: 8},
		{cwd: "build", tags: []string{"SQLITE3VFS_LOADABLE_EXT", "vfs"}, goos: "linux", packages: []string{"./cmd/litestream-vfs"}, line: 11},
		{cwd: "build/tools", tags: []string{"tool"}, packages: []string{"."}, line: 15},
		{cwd: "build/cmd", tags: []string{"a", "b"}, packages: []string{"main.go"}, line: 21},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("commands:\n%+v\nwant\n%+v", got, want)
	}
}

func TestGoreleaserCommandsReadTags(t *testing.T) {
	config := "builds:\n" + // 1
		"  - id: vfs\n" + // 2
		"    main: ./cmd/vfs\n" + // 3
		"    tags:\n" + // 4
		"      - vfs\n" + // 5
		"    goos: [linux, darwin]\n" + // 6
		"  - id: flagged\n" + // 7
		"    dir: tools\n" + // 8
		"    flags:\n" + // 9
		"      - -trimpath\n" + // 10
		"      - -tags=netgo\n" + // 11
		"  - id: templated\n" + // 12
		"    main: ./cmd/t\n" + // 13
		"    tags: ['{{ .Env.TAGS }}']\n" + // 14
		"  - id: plain\n" + // 15
		"    main: ./cmd/plain\n" // 16
	got := goreleaserCommands(".", []byte(config))
	want := []buildCommand{
		{cwd: ".", tags: []string{"vfs"}, goos: "linux", packages: []string{"././cmd/vfs"}, line: 5},
		{cwd: ".", tags: []string{"vfs"}, goos: "darwin", packages: []string{"././cmd/vfs"}, line: 5},
		{cwd: "tools", tags: []string{"netgo"}, packages: []string{"./."}, line: 11},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("builds:\n%+v\nwant\n%+v", got, want)
	}
}

func TestCommandPackageDirs(t *testing.T) {
	modules := []ModuleFact{{ModulePath: "example.com/app", ModuleDir: "."}, {ModulePath: "example.com/app/nested", ModuleDir: "nested"}}
	command := buildCommand{cwd: "build", packages: []string{
		"../cmd/a", "main.go", "example.com/app/cmd/b", "example.com/app/nested/cmd/c",
		"./...", "example.com/other/cmd@latest", "golang.org/x/tools/cmd/stringer", "../../outside",
	}}
	got := commandPackageDirs(command, modules)
	want := []string{"cmd/a", "build", "cmd/b", "nested/cmd/c"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("package directories %v, want %v", got, want)
	}
}
