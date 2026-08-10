package report

import (
	"strings"
	"testing"
)

func TestCurrentWorkspaceHasOneAuthorityPerConcept(t *testing.T) {
	t.Parallel()

	for _, forbidden := range []string{
		"DATA.user_mechanisms",
		"DATA.candidate_flows",
		"DATA.flows",
		"function openReportTarget(",
		"function openUserMechanism(",
		"function renderMechanismDetailWorkspace(",
		"#/mechanism",
		"kind === 'semantic_artifact'",
	} {
		if strings.Contains(scriptJS, forbidden) {
			t.Errorf("current report JS still publishes legacy authority %q", forbidden)
		}
	}
	for _, forbidden := range []string{
		`id="rm-overview"`,
		`id="rm-mechanisms"`,
		`id="rm-mechanism-detail"`,
		`id="rm-study-overview"`,
		`id="rm-study-detail"`,
	} {
		if strings.Contains(templateHTML, forbidden) {
			t.Errorf("current report template still publishes legacy DOM %q", forbidden)
		}
	}
	for _, token := range []string{
		`id="rm-architecture"`,
		`id="rm-task-investigation"`,
		`id="rm-source-drawer"`,
	} {
		if !strings.Contains(templateHTML, token) {
			t.Errorf("current report template is missing authority %q", token)
		}
	}
}

func TestLegacyDeepLinksFailClosedToMap(t *testing.T) {
	t.Parallel()

	for _, forbidden := range []string{
		"route.kind === 'mechanisms'",
		"route.kind === 'mechanism'",
		"route.kind === 'search'",
	} {
		if strings.Contains(scriptJS, forbidden) {
			t.Errorf("legacy deep-link route is still reachable through %q", forbidden)
		}
	}
	for _, token := range []string{"function parseWorkspaceHash(", "defaultWorkspaceHash()", "if (!valid) state = emptyWorkspaceState()"} {
		if !strings.Contains(scriptJS, token) {
			t.Errorf("deep-link fail-close contract is missing %q", token)
		}
	}
}

func TestStudyMechanismMapReturnAssetContract(t *testing.T) {
	t.Parallel()

	for _, token := range []string{
		"function showStudyMechanismTargetOnMap(",
		"function showStudyMechanismOnMap(",
		"type: 'show_map'",
		"function returnFromArchitecture(",
		"type: 'return_from_map'",
		"workspaceState.mapReturn",
		"workspaceState.mapTarget",
	} {
		if !strings.Contains(scriptJS, token) {
			t.Errorf("report JS is missing bounded Study map round-trip token %q", token)
		}
	}
}

func TestUserReportKeepsProvenanceBehindDebugMode(t *testing.T) {
	t.Parallel()

	for _, token := range []string{
		"var DEBUG_MODE =",
		"function renderProvenanceWorkspace(",
		"if (DEBUG_MODE)",
		"'main.provenance'",
	} {
		if !strings.Contains(scriptJS, token) {
			t.Errorf("report JS is missing debug provenance gate token %q", token)
		}
	}
}
