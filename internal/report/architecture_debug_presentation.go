package report

import (
	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/evidence"
)

// architectureProvenanceProductMessage classifies deterministic product-owned
// provenance copy by its typed producer identity. The detail text itself is
// deliberately not consulted: canonical English prose is not an enum and must
// never become the routing key for localization.
func architectureProvenanceProductMessage(provenance evidence.Provenance) string {
	switch provenance.Provider + "\x00" + provenance.Operation {
	case "architecture_grounding\x00behavior_anchor_file":
		return "architecture.provenance.behavior_anchor_file"
	case "architecture_grounding\x00behavior_anchor_member":
		return "architecture.provenance.behavior_anchor_member"
	case "surface_catalog\x00exact_process_entry_role":
		return "architecture.provenance.exact_process_entry_role"
	case "report_repository_graph\x00saved_package_import":
		return "architecture.provenance.saved_package_import"
	case "report_repository_graph\x00saved_package_member":
		return "architecture.provenance.saved_package_member"
	case "flowproof\x00saved_proof":
		return "architecture.provenance.saved_proof"
	case "flowproof\x00saved_flow_member":
		return "architecture.provenance.saved_flow_member"
	case "flowproof\x00anchor_file":
		return "architecture.provenance.anchor_file"
	case "flowproof\x00anchor_declaration":
		return "architecture.provenance.anchor_declaration"
	case "flowproof\x00bind_anchor_to_exact_member":
		return "architecture.provenance.bind_anchor_to_exact_member"
	case "flowproof\x00anchor_flow_participation":
		return "architecture.provenance.anchor_flow_participation"
	default:
		return ""
	}
}

// architectureProvenanceDetailIsOpaque identifies analyzer enum payloads that
// are displayed for debugging but are not prose. Provider, version, operation,
// location, and this exact enum remain byte-identical across locales.
func architectureProvenanceDetailIsOpaque(provenance evidence.Provenance) bool {
	return provenance.Provider == "go_syntax" &&
		provenance.Operation == "inspect_handler_concurrency"
}

// architectureScenarioProductMessage classifies the only scenarios currently
// synthesized by the deterministic Architecture projection. Unknown scenario
// names remain terminal prose and therefore enter PresentationTextInventory.
func architectureScenarioProductMessage(scenario componentmap.ScenarioContext) string {
	if scenario.ID == "saved-package-graph" {
		return "architecture.scenario.saved_package_graph"
	}
	if scenario.Build.GOOS != "" || scenario.Build.GOARCH != "" ||
		len(scenario.Build.BuildTags) > 0 {
		return "architecture.scenario.recorded_go_build"
	}
	return ""
}
