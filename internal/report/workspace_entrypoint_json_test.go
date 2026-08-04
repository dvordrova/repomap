package report

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/workspaceentrypoint"
)

func TestDecodeSnapshotExactEntrypointsUsesOnlyNarrowTopLevelFacts(t *testing.T) {
	var ignoredGoFiles strings.Builder
	for index := 0; index < workspaceentrypoint.MaxRawRows+1; index++ {
		if index > 0 {
			ignoredGoFiles.WriteByte(',')
		}
		ignoredGoFiles.WriteString(`"ignored.go"`)
	}
	snapshot := `{
		"repo_name":"fixture",
		"go_facts":{
			"modules":[{"entrypoint_packages":"ignored nested poison"}],
			"entrypoint_packages":[{
				"module_path":"example.com/repo",
				"import_path":"example.com/repo/cmd/app",
				"dir":"/ignored/absolute/directory",
				"package_dir":"cmd/app",
				"module_relative_dir":"cmd/app",
				"module_dir":".",
				"kind":"ignored-editorial-role",
				"go_files":[` + ignoredGoFiles.String() + `],
				"anchors":[{
					"version":1,
					"kind":"go_main_function",
					"path":"cmd/app/main.go",
					"line":9,
					"ordinary_unknown":{"nested":true}
				}],
				"ordinary_unknown":["skipped"]
			}]
		}
	}`
	facts, err := decodeSnapshotExactEntrypoints([]byte(snapshot))
	if err != nil {
		t.Fatalf("decodeSnapshotExactEntrypoints: %v", err)
	}
	want := gofacts.Facts{EntrypointPackages: []gofacts.Entrypoint{{
		ModulePath: "example.com/repo", ImportPath: "example.com/repo/cmd/app",
		PackageDir: "cmd/app", ModuleRelativeDir: "cmd/app", ModuleDir: ".",
		Anchors: []gofacts.EntrypointAnchor{{
			Version: 1, Kind: gofacts.EntrypointAnchorGoMain,
			Path: "cmd/app/main.go", Line: 9,
		}},
	}}}
	if !reflect.DeepEqual(facts, want) {
		t.Fatalf("facts = %#v, want %#v", facts, want)
	}
	entrypoint := facts.EntrypointPackages[0]
	if entrypoint.Dir != "" || entrypoint.Kind != "" || entrypoint.GoFiles != nil {
		t.Fatalf("ignored full-fact fields were retained: %#v", entrypoint)
	}
	if encoded := string(mustJSON(t, facts)); strings.Contains(encoded, "/ignored/absolute") ||
		strings.Contains(encoded, "ignored-editorial-role") ||
		strings.Contains(encoded, "ignored.go") ||
		strings.Contains(encoded, "nested poison") {
		t.Fatalf("narrow facts retained ignored data: %s", encoded)
	}
}

func TestSnapshotExactEntrypointsNilEmptyAndMalformedShapes(t *testing.T) {
	tests := []struct {
		name          string
		snapshot      string
		wantErr       error
		wantOuterNil  bool
		wantAnchorNil *bool
	}{
		{
			name: "missing go facts", snapshot: `{}`,
			wantErr: errReportEntrypointJSONUnavailable,
		},
		{
			name: "null go facts", snapshot: `{"go_facts":null}`,
			wantErr: errReportEntrypointJSONUnavailable,
		},
		{
			name: "missing collection", snapshot: `{"go_facts":{}}`,
			wantErr: errReportEntrypointJSONUnavailable,
		},
		{
			name: "null collection", snapshot: `{"go_facts":{"entrypoint_packages":null}}`,
			wantOuterNil: true,
		},
		{
			name: "empty collection", snapshot: `{"go_facts":{"entrypoint_packages":[]}}`,
			wantOuterNil: false,
		},
		{
			name:          "null anchors",
			snapshot:      `{"go_facts":{"entrypoint_packages":[{"anchors":null}]}}`,
			wantAnchorNil: boolPointer(true),
		},
		{
			name:          "empty anchors",
			snapshot:      `{"go_facts":{"entrypoint_packages":[{"anchors":[]}]}}`,
			wantAnchorNil: boolPointer(false),
		},
		{
			name: "null row", snapshot: `{"go_facts":{"entrypoint_packages":[null]}}`,
			wantErr: errReportEntrypointJSONUnavailable,
		},
		{
			name:     "null anchor",
			snapshot: `{"go_facts":{"entrypoint_packages":[{"anchors":[null]}]}}`,
			wantErr:  errReportEntrypointJSONUnavailable,
		},
		{
			name:     "trailing json",
			snapshot: `{"go_facts":{"entrypoint_packages":[]}} false`,
			wantErr:  errReportEntrypointJSONUnavailable,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			facts, err := decodeSnapshotExactEntrypoints([]byte(test.snapshot))
			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("decode error = %v, want %v", err, test.wantErr)
				}
				if !reflect.DeepEqual(facts, gofacts.Facts{}) {
					t.Fatalf("failed decode returned facts: %#v", facts)
				}
				return
			}
			if err != nil {
				t.Fatalf("decode error = %v", err)
			}
			if (facts.EntrypointPackages == nil) != test.wantOuterNil {
				t.Fatalf(
					"outer nil = %t, want %t",
					facts.EntrypointPackages == nil,
					test.wantOuterNil,
				)
			}
			if test.wantAnchorNil != nil {
				if len(facts.EntrypointPackages) != 1 ||
					(facts.EntrypointPackages[0].Anchors == nil) != *test.wantAnchorNil {
					t.Fatalf("anchor shape = %#v", facts.EntrypointPackages)
				}
			}
		})
	}
}

func TestSnapshotExactEntrypointsRejectsKnownKeyAliasesAndDuplicates(t *testing.T) {
	const canonical = `{"go_facts":{"entrypoint_packages":[{
		"module_path":"example.com/repo",
		"import_path":"example.com/repo/cmd/app",
		"dir":"cmd/app",
		"package_dir":"cmd/app",
		"module_relative_dir":"cmd/app",
		"module_dir":".",
		"kind":"primary_binary",
		"go_files":["main.go"],
		"anchors":[{"version":1,"kind":"go_main_function","path":"cmd/app/main.go","line":9}]
	}]}}`
	tests := []struct {
		name     string
		snapshot string
	}{
		{name: "GO_FACTS", snapshot: strings.Replace(canonical, `"go_facts"`, `"GO_FACTS"`, 1)},
		{
			name: "ENTRYPOINT_PACKAGES",
			snapshot: strings.Replace(
				canonical,
				`"entrypoint_packages"`,
				`"ENTRYPOINT_PACKAGES"`,
				1,
			),
		},
		{name: "MODULE_PATH", snapshot: strings.Replace(canonical, `"module_path"`, `"MODULE_PATH"`, 1)},
		{name: "IMPORT_PATH", snapshot: strings.Replace(canonical, `"import_path"`, `"IMPORT_PATH"`, 1)},
		{name: "DIR", snapshot: strings.Replace(canonical, `"dir"`, `"DIR"`, 1)},
		{name: "PACKAGE_DIR", snapshot: strings.Replace(canonical, `"package_dir"`, `"PACKAGE_DIR"`, 1)},
		{
			name: "MODULE_RELATIVE_DIR",
			snapshot: strings.Replace(
				canonical,
				`"module_relative_dir"`,
				`"MODULE_RELATIVE_DIR"`,
				1,
			),
		},
		{name: "MODULE_DIR", snapshot: strings.Replace(canonical, `"module_dir"`, `"MODULE_DIR"`, 1)},
		{name: "entry KIND", snapshot: strings.Replace(canonical, `"kind":"primary_binary"`, `"KIND":"primary_binary"`, 1)},
		{name: "GO_FILES", snapshot: strings.Replace(canonical, `"go_files"`, `"GO_FILES"`, 1)},
		{name: "ANCHORS", snapshot: strings.Replace(canonical, `"anchors"`, `"ANCHORS"`, 1)},
		{name: "anchor VERSION", snapshot: strings.Replace(canonical, `"version"`, `"VERSION"`, 1)},
		{
			name: "anchor KIND",
			snapshot: strings.Replace(
				canonical,
				`"kind":"go_main_function"`,
				`"KIND":"go_main_function"`,
				1,
			),
		},
		{name: "anchor PATH", snapshot: strings.Replace(canonical, `"path":"cmd/app/main.go"`, `"PATH":"cmd/app/main.go"`, 1)},
		{name: "anchor LINE", snapshot: strings.Replace(canonical, `"line"`, `"LINE"`, 1)},
		{name: `go\u005ffacts`, snapshot: strings.Replace(canonical, `"go_facts"`, `"go\u005ffacts"`, 1)},
		{
			name: `entrypoint\u005fpackages`,
			snapshot: strings.Replace(
				canonical,
				`"entrypoint_packages"`,
				`"entrypoint\u005fpackages"`,
				1,
			),
		},
		{name: `anch\u006frs`, snapshot: strings.Replace(canonical, `"anchors"`, `"anch\u006frs"`, 1)},
		{
			name: `p\u0061th`,
			snapshot: strings.Replace(
				canonical,
				`"path":"cmd/app/main.go"`,
				`"p\u0061th":"cmd/app/main.go"`,
				1,
			),
		},
		{
			name: "duplicate import path",
			snapshot: strings.Replace(
				canonical,
				`"import_path":"example.com/repo/cmd/app",`,
				`"import_path":"example.com/repo/cmd/app","import_path":"duplicate",`,
				1,
			),
		},
		{
			name: "escaped unknown inside consumed row",
			snapshot: strings.Replace(
				canonical,
				`"anchors":[`,
				`"ord\u0069nary_unknown":true,"anchors":[`,
				1,
			),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := preflightSnapshotExactEntrypoints([]byte(test.snapshot)); !errors.Is(
				err,
				errReportEntrypointJSONUnavailable,
			) {
				t.Fatalf("preflight error = %v, want unavailable", err)
			}
			facts, err := decodeSnapshotExactEntrypoints([]byte(test.snapshot))
			if !errors.Is(err, errReportEntrypointJSONUnavailable) {
				t.Fatalf("decode error = %v, want unavailable", err)
			}
			if !reflect.DeepEqual(facts, gofacts.Facts{}) {
				t.Fatalf("alias decode returned facts: %#v", facts)
			}
		})
	}
}

func TestSnapshotExactEntrypointsPreflightBudgetsAndPrecedence(t *testing.T) {
	t.Run("outer raw count before excess item", func(t *testing.T) {
		var input strings.Builder
		input.WriteString(`{"go_facts":{"entrypoint_packages":[`)
		for index := 0; index < workspaceentrypoint.MaxRawRows; index++ {
			if index > 0 {
				input.WriteByte(',')
			}
			input.WriteString(`{}`)
		}
		input.WriteString(`,{"module_path":"`)
		input.WriteString(strings.Repeat("x", 2*1024*1024))
		_, err := preflightSnapshotExactEntrypoints([]byte(input.String()))
		if !errors.Is(err, errReportEntrypointJSONBounds) {
			t.Fatalf("preflight error = %v, want bounds", err)
		}
	})

	t.Run("aggregate anchor count before excess item", func(t *testing.T) {
		var input strings.Builder
		input.WriteString(`{"go_facts":{"entrypoint_packages":[{"anchors":[`)
		for index := 0; index < workspaceentrypoint.MaxRawRows; index++ {
			if index > 0 {
				input.WriteByte(',')
			}
			input.WriteString(`{}`)
		}
		input.WriteString(`,{"path":"`)
		input.WriteString(strings.Repeat("x", 2*1024*1024))
		_, err := preflightSnapshotExactEntrypoints([]byte(input.String()))
		if !errors.Is(err, errReportEntrypointJSONBounds) {
			t.Fatalf("preflight error = %v, want bounds", err)
		}
	})

	t.Run("scalar boundary", func(t *testing.T) {
		for _, test := range []struct {
			name    string
			size    int
			wantErr error
		}{
			{name: "exact", size: workspaceentrypoint.MaxScalarBytes},
			{
				name: "plus one", size: workspaceentrypoint.MaxScalarBytes + 1,
				wantErr: errReportEntrypointJSONBounds,
			},
		} {
			t.Run(test.name, func(t *testing.T) {
				input := `{"go_facts":{"entrypoint_packages":[{"module_path":"` +
					strings.Repeat("x", test.size) + `"}]}}`
				_, err := preflightSnapshotExactEntrypoints([]byte(input))
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("preflight error = %v, want %v", err, test.wantErr)
				}
			})
		}
	})

	t.Run("aggregate scalar budget", func(t *testing.T) {
		kind := strings.Repeat("k", 2097)
		sourcePath := strings.Repeat("p", 2097)
		var input strings.Builder
		input.Grow(workspaceentrypoint.MaxAggregateScalarBytes + 256*1024)
		input.WriteString(`{"go_facts":{"entrypoint_packages":[{"anchors":[`)
		for index := 0; index < workspaceentrypoint.MaxRawRows; index++ {
			if index > 0 {
				input.WriteByte(',')
			}
			input.WriteString(`{"version":1,"kind":"`)
			input.WriteString(kind)
			input.WriteString(`","path":"`)
			input.WriteString(sourcePath)
			input.WriteString(`","line":1}`)
		}
		input.WriteString(`]}]}}`)
		_, err := preflightSnapshotExactEntrypoints([]byte(input.String()))
		if !errors.Is(err, errReportEntrypointJSONBounds) {
			t.Fatalf("preflight error = %v, want aggregate bounds", err)
		}
	})

	t.Run("oversized scalar before aggregate remaining", func(t *testing.T) {
		budget := savedEntrypointJSONBudget{scalarBytes: 1}
		input := []byte(`"` + strings.Repeat("x", workspaceentrypoint.MaxScalarBytes+1) + `"`)
		_, err := preflightSavedEntrypointJSONString(input, 0, &budget)
		if !errors.Is(err, errReportGraphJSONBounds) {
			t.Fatalf("string preflight error = %v, want scalar bounds", err)
		}
	})
}

func TestSnapshotExactEntrypointsPreflightRejectsInvalidIntegerScalars(t *testing.T) {
	for _, test := range []struct {
		token   string
		wantErr error
	}{
		{token: "0"},
		{token: "-1"},
		{token: "1"},
		{token: "+1", wantErr: errReportEntrypointJSONUnavailable},
		{token: "01", wantErr: errReportEntrypointJSONUnavailable},
		{token: "-01", wantErr: errReportEntrypointJSONUnavailable},
		{token: "1.0", wantErr: errReportEntrypointJSONUnavailable},
		{token: "1e0", wantErr: errReportEntrypointJSONUnavailable},
		{token: "true", wantErr: errReportEntrypointJSONUnavailable},
		{token: "null", wantErr: errReportEntrypointJSONUnavailable},
		{token: `"1"`, wantErr: errReportEntrypointJSONUnavailable},
		{
			token:   strings.Repeat("9", workspaceentrypoint.MaxScalarBytes+1),
			wantErr: errReportEntrypointJSONBounds,
		},
	} {
		t.Run(test.token[:min(len(test.token), 16)], func(t *testing.T) {
			input := `{"go_facts":{"entrypoint_packages":[{"anchors":[{"version":` +
				test.token + `,"line":1}]}]}}`
			_, err := preflightSnapshotExactEntrypoints([]byte(input))
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("preflight error = %v, want %v", err, test.wantErr)
			}
		})
	}
}

func TestMalformedEntrypointExtensionKeepsGraphAndPublicProjection(t *testing.T) {
	oversized := strings.Repeat("x", workspaceentrypoint.MaxScalarBytes+1)
	dir := t.TempDir()
	writeTestFile(t, dir, "snapshot.json", `{
		"repo_name":"fixture",
		"go_facts":{
			"modules":[{
				"id":"root-id",
				"module_path":"example.com/repo",
				"module_dir":".",
				"display_name":".",
				"main":true
			}],
			"packages":[{
				"canonical_package_path":"example.com/repo/cmd/app",
				"name":"main",
				"owning_module_id":"root-id",
				"module_path":"example.com/repo",
				"package_directory":"cmd/app",
				"module_relative_path":"cmd/app",
				"display_path":"cmd/app",
				"locality":"local",
				"files":["cmd/app/main.go"]
			}],
			"internal_edges":[],
			"entrypoint_packages":[{
				"module_path":"example.com/repo",
				"import_path":"example.com/repo/cmd/app",
				"package_dir":"cmd/app",
				"module_relative_dir":"cmd/app",
				"module_dir":".",
				"anchors":[{
					"version":1,
					"kind":"go_main_function",
					"path":"`+oversized+`",
					"line":9
				}]
			}]
		}
	}`)
	data := &ReportData{}
	if warning := parseSnapshotWithExactFacts(
		dir+"/snapshot.json",
		data,
		true,
	); warning != "" {
		t.Fatalf("legacy parse changed: %s", warning)
	}
	if data.repositoryGoFacts == nil {
		t.Fatal("malformed entrypoint extension disabled valid package graph facts")
	}
	if data.repositoryEntrypointFacts != nil {
		t.Fatal("malformed entrypoint extension was retained")
	}
	if data.RepositoryGraph == nil || len(data.RepositoryGraph.Packages) != 1 {
		t.Fatalf("legacy graph missing: %#v", data.RepositoryGraph)
	}
	encoded := string(mustJSON(t, data))
	if strings.Contains(encoded, oversized) ||
		strings.Contains(encoded, "/definitely-not-present") {
		t.Fatalf("oversized scalar or absolute root reached public JSON: %s", encoded)
	}
}

func TestSnapshotExactEntrypointsCanonicalOversizedScalarPreflightAllocatesNothing(t *testing.T) {
	input := []byte(
		`{"go_facts":{"entrypoint_packages":[{"module_path":"` +
			strings.Repeat("x", workspaceentrypoint.MaxScalarBytes+1) +
			`"}]}}`,
	)
	allocations := testing.AllocsPerRun(20, func() {
		_, _ = preflightSnapshotExactEntrypoints(input)
	})
	if allocations != 0 {
		t.Fatalf("preflight allocations = %f, want 0", allocations)
	}
}

func BenchmarkSnapshotExactEntrypointsPreflightOversizedScalar(b *testing.B) {
	input := []byte(
		`{"go_facts":{"entrypoint_packages":[{"module_path":"` +
			strings.Repeat("x", 2*1024*1024) +
			`"}]}}`,
	)
	b.ReportAllocs()
	b.SetBytes(int64(len(input)))
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		_, _ = preflightSnapshotExactEntrypoints(input)
	}
}

func boolPointer(value bool) *bool {
	return &value
}
