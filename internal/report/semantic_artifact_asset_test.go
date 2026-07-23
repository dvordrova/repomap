package report

import (
	"strings"
	"testing"
)

func TestUserMechanismWorkspaceAssetContract(t *testing.T) {
	t.Parallel()

	for _, token := range []string{
		"var USER_MECHANISMS = Array.isArray(DATA.user_mechanisms)",
		"function reduceWorkspaceState(",
		"function openUserMechanism(",
		"function renderMechanismDetailWorkspace(",
		"function mechanismNarrativeItems(",
		"function renderImplementationDetails(",
		"'Primary implementation'",
		"'← Previous'",
		"'Next →'",
		"openSourceLocation(location)",
		"if (includeMap && step.map_target && userArchitectureAvailable())",
	} {
		if !strings.Contains(scriptJS, token) {
			t.Errorf("report JS is missing User Mechanism workspace token %q", token)
		}
	}
	for _, token := range []string{
		`id="rm-tabs"`,
		`id="rm-mechanisms"`,
		`id="rm-mechanism-detail"`,
		`id="rm-architecture"`,
		`id="rm-source-drawer"`,
	} {
		if !strings.Contains(templateHTML, token) {
			t.Errorf("report template is missing User Report v2 token %q", token)
		}
	}
	for _, token := range []string{
		".rm-workspace",
		".rm-repository-nav",
		".rm-mechanism-layout",
		".rm-step-workspace",
		".rm-source-drawer",
	} {
		if !strings.Contains(styleCSS, token) {
			t.Errorf("report CSS is missing User Report v2 token %q", token)
		}
	}
}

func TestSemanticSearchArtifactTargetOpensDetailWithoutMap(t *testing.T) {
	t.Parallel()

	start := strings.Index(scriptJS, "if (kind === 'semantic_artifact') {")
	if start < 0 {
		t.Fatal("semantic artifact navigation branch is missing")
	}
	rest := scriptJS[start:]
	end := strings.Index(rest, "if (kind === 'component') {")
	if end < 0 {
		t.Fatal("semantic artifact navigation branch has no bounded end")
	}
	branch := rest[:end]
	if !strings.Contains(branch, "openUserMechanism(") {
		t.Fatalf("semantic artifact branch does not open the mechanism detail: %s", branch)
	}
	for _, forbidden := range []string{
		"architectureCanvasView",
		"openArchitectureTarget",
		"showArchitectureFromSearch",
		"scrollIntoView",
	} {
		if strings.Contains(branch, forbidden) {
			t.Errorf("semantic artifact branch still redirects to architecture via %q", forbidden)
		}
	}
}

func TestUserMechanismMapReturnAssetContract(t *testing.T) {
	t.Parallel()

	for _, token := range []string{
		"function showMechanismStepOnMap(",
		"type: 'show_map'",
		"function returnFromArchitecture(",
		"type: 'return_from_map'",
		"workspaceState.mapReturn",
		"workspaceState.mapTarget",
		"else renderArchitectureReturn();",
	} {
		if !strings.Contains(scriptJS, token) {
			t.Errorf("report JS is missing bounded map round-trip token %q", token)
		}
	}
	for _, token := range []string{
		".rm-architecture-return {",
		"min-width: 0",
		".rm-architecture-return .rm-secondary-action { flex: 0 0 auto; order: -1; }",
	} {
		if !strings.Contains(styleCSS, token) {
			t.Errorf("report CSS is missing visible map-return token %q", token)
		}
	}
}

func TestUserReportKeepsProvenanceBehindDebugMode(t *testing.T) {
	t.Parallel()

	for _, token := range []string{
		"var DEBUG_MODE =",
		"function renderProvenanceWorkspace(",
		"if (DEBUG_MODE)",
		"'Provenance'",
	} {
		if !strings.Contains(scriptJS, token) {
			t.Errorf("report JS is missing debug provenance gate token %q", token)
		}
	}
}
