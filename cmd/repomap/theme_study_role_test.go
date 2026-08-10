package main

import (
	"testing"

	"github.com/dvordrova/repomap/internal/artifactrole"
	"github.com/dvordrova/repomap/internal/atlasstudy"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/themestudy"
)

func TestThemeReadingTargetRoleKeepsProducerEvidenceAndPathRoleDistinct(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		path     string
		supports []atlasstudy.SupportRole
		want     artifactrole.Role
	}{
		{
			name: "ordinary observed call is not an integration boundary",
			path: "internal/config/parse.go", supports: []atlasstudy.SupportRole{atlasstudy.SupportObservedCallBoundary},
			want: artifactrole.RoleProductionCore,
		},
		{
			name: "effect path remains an integration boundary",
			path: "internal/storage/write.go", supports: []atlasstudy.SupportRole{atlasstudy.SupportObservedCallBoundary},
			want: artifactrole.RoleEffectBoundary,
		},
		{
			name: "testing path defeats process-entry hint",
			path: "plugin/testing/broken/main.go", supports: []atlasstudy.SupportRole{atlasstudy.SupportProcessEntry},
			want: artifactrole.RoleTest,
		},
		{
			name: "script path defeats process-entry hint",
			path: "scripts/release/main.go", supports: []atlasstudy.SupportRole{atlasstudy.SupportProcessEntry},
			want: artifactrole.RoleTooling,
		},
		{
			name: "selected public API root defeats conventional main path",
			path: "cmd/library/main.go", supports: []atlasstudy.SupportRole{atlasstudy.SupportAnalysisTargetRoot},
			want: artifactrole.RolePublicAPI,
		},
		{
			name: "selected public API root defeats example path",
			path: "examples/client.go", supports: []atlasstudy.SupportRole{atlasstudy.SupportAnalysisTargetRoot},
			want: artifactrole.RolePublicAPI,
		},
		{
			name: "selected public API root defeats generated path",
			path: "generated/client.pb.go", supports: []atlasstudy.SupportRole{atlasstudy.SupportAnalysisTargetRoot},
			want: artifactrole.RolePublicAPI,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			target := atlasstudy.ReadingTarget{
				ID: "target", Kind: atlasstudy.ReadingTargetFunction,
				Location: evidence.Location{Path: test.path, Line: 1},
			}
			input := atlasstudy.Input{ReadingTargets: []atlasstudy.ReadingTarget{target}}
			for index, role := range test.supports {
				input.ReadingSupports = append(input.ReadingSupports, atlasstudy.ReadingSupport{
					ID: string(rune('a' + index)), TargetID: target.ID, Role: role,
				})
			}
			if got := themeReadingTargetRole(input, target); got != test.want {
				t.Fatalf("themeReadingTargetRole(%q) = %q, want %q", test.path, got, test.want)
			}
			seeds := themeSeedSpecsFromInput(input)
			if len(seeds) != 1 || seeds[0].Role != themestudy.Role(test.want) {
				t.Fatalf("theme seed role for %q = %#v, want %q", test.path, seeds, test.want)
			}
		})
	}
}
