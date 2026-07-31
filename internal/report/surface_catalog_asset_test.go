package report

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestSurfaceCatalogAssetContract(t *testing.T) {
	t.Parallel()

	js := readSurfaceCatalogAsset(t, "surface_catalog.js")
	css := readSurfaceCatalogAsset(t, "surface_catalog.css")
	catalog := readSurfaceCatalogAsset(t, "ui_messages.js")

	tests := []struct {
		name   string
		asset  string
		tokens []string
	}{
		{
			name:  "replaceable runtime API uses the typed message catalog",
			asset: js,
			tokens: []string{
				"global.RepomapSurfaceCatalog",
				"Object.freeze({ mount: mount })",
				"RepomapSurfaceCatalog.mount requires options.message",
				"this.message = this.options.message",
				`this.message("surfaces.error.host_element")`,
			},
		},
		{
			name:  "bounded local filters",
			asset: js,
			tokens: []string{
				"const PAGE_SIZE = 6",
				`message("surfaces.filter.kind.label")`,
				`message("surfaces.filter.evidence.label")`,
				`message("surfaces.action.show_less")`,
				`message("surfaces.action.show_more", { count: hiddenCount })`,
			},
		},
		{
			name:  "honest semantics stay distinct",
			asset: js,
			tokens: []string{
				`message("surfaces.intro")`,
				`message("surfaces.certainty.static_not_observed")`,
				`this.semantic(message("surfaces.field.status")`,
				`this.semantic(message("surfaces.field.role")`,
				`this.semantic(message("surfaces.field.trace_readiness")`,
				`this.semantic(message("surfaces.field.certainty")`,
				`this.semantic(message("surfaces.field.resolution")`,
				`message("surfaces.coverage.caveat")`,
				`messageID: "surfaces.metric.non_worker_async_tasks"`,
				`confirmed_server_start_call: "surfaces.status.static_start_call"`,
				`confirmed_route_descriptor: "surfaces.status.admin_route_descriptor"`,
				`messageID: "surfaces.metric.server_start_sites"`,
				`messageID: "surfaces.metric.http_registrations"`,
				`message("surfaces.coverage.direct_surfaces")`,
				`value: "http_server"`,
				`return message("surfaces.identity.http_server_start")`,
				`value: "process_entry"`,
				`message("surfaces.identity.process_entry", {`,
				`confirmed_process_entry: "surfaces.status.exact_process_entry"`,
				`message("surfaces.owner.executable", { owner: owner })`,
				`targets: "surfaces.budget.targets"`,
				`message("surfaces.field.trace_readiness_reason")`,
				`message("surfaces.field.quality")`,
				`message("surfaces.identity.task", { name: task[1], number: task[2] })`,
			},
		},
		{
			name:  "zero and filtered states do not claim absence",
			asset: js,
			tokens: []string{
				`"surfaces.empty.catalog_with_anchors"`,
				`message("surfaces.empty.filters")`,
				`message("surfaces.empty.scope_note")`,
			},
		},
		{
			name:  "catalog groups and trace progression",
			asset: js + css,
			tokens: []string{
				`message("surfaces.title")`,
				`messageID: "surfaces.group.application"`,
				`messageID: "surfaces.group.secondary_services"`,
				`messageID: "surfaces.group.tooling"`,
				`messageID: "surfaces.group.tests_helpers"`,
				`messageID: "surfaces.group.unassigned"`,
				`messageID: "surfaces.group.dynamic_unresolved"`,
				`messageID: "surfaces.group.unavailable"`,
				`message("surfaces.action.open_saved_trace")`,
				`message("surfaces.progression.trace_unavailable", {`,
				`message("surfaces.action.view_in_architecture")`,
				`case "primary_application": return "application"`,
				`case "secondary_service": return "secondary_service"`,
				`case "secondary_tooling": return "tooling"`,
				`messageID: "surfaces.metric.cli_commands"`,
				`messageID: "surfaces.metric.process_entries"`,
				`messageID: "surfaces.metric.total"`,
				`messageID: "surfaces.metric.trace_ready"`,
				`messageID: "surfaces.metric.partial_trace_candidates"`,
				`messageID: "surfaces.metric.runtime_activities"`,
				`messageID: "surfaces.metric.rejected_noisy"`,
			},
		},
		{
			name:  "empty analysis avoids inert catalog controls",
			asset: js,
			tokens: []string{
				`if (this.triggers.length === 0)`,
				`this.root.classList.add("is-empty")`,
				`this.summary.remove()`,
			},
		},
		{
			name:  "expandable evidence remains separated",
			asset: js,
			tokens: []string{
				`this.detailSection(message("surfaces.section.middleware")`,
				`this.detailSection(message("surfaces.section.wrapper_chain")`,
				`this.detailSection(message("surfaces.section.evidence")`,
				`message("surfaces.section.dynamic_frontiers")`,
				`message("surfaces.coverage.title")`,
				`message("surfaces.section.loop_signals")`,
				`this.detailSection(message("surfaces.section.unavailable_packages")`,
				`this.detailSection(message("surfaces.section.package_diagnostics")`,
			},
		},
		{
			name:  "editor links and hash are supplied integration points",
			asset: js,
			tokens: []string{
				`typeof this.options.openLocation === "function"`,
				"this.options.openLocation(object(location))",
				"new URLSearchParams",
				`params.set("surface", id)`,
				`params.delete("surface")`,
			},
		},
		{
			name:  "keyboard controls and scoped styles",
			asset: css,
			tokens: []string{
				".rm-surface button:focus-visible",
				".rm-surface summary:focus-visible",
				".rm-surface__filter[aria-pressed=\"true\"]",
				".rm-surface__semantics",
				".rm-surface__coverage",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			for _, token := range test.tokens {
				if !strings.Contains(test.asset, token) {
					t.Errorf("asset is missing integration token %q", token)
				}
			}
		})
	}

	for _, forbidden := range []string{"fetch(", "XMLHttpRequest", "WebSocket", "EventSource"} {
		if strings.Contains(js, forbidden) {
			t.Errorf("surface catalog must not initiate external work: found %q", forbidden)
		}
	}
	usedMessageIDs := regexp.MustCompile(`"(surfaces\.[a-z0-9_.]+)"`).FindAllStringSubmatch(js, -1)
	if len(usedMessageIDs) == 0 {
		t.Fatal("surface catalog does not use typed surface message IDs")
	}
	seenMessageIDs := make(map[string]struct{}, len(usedMessageIDs))
	for _, match := range usedMessageIDs {
		id := match[1]
		if _, ok := seenMessageIDs[id]; ok {
			continue
		}
		seenMessageIDs[id] = struct{}{}
		if count := strings.Count(catalog, `"`+id+`"`); count != 2 {
			t.Errorf("surface message %q must occur once in each locale catalog; got %d", id, count)
		}
	}
	for _, forbidden := range []string{
		"translateUI",
		"RU_UI",
		"MutationObserver",
		"TreeWalker",
		`"All surfaces"`,
		`"Local static analysis"`,
		`"No surfaces match these filters."`,
		`"Open saved trace"`,
		`"View in Architecture"`,
		`"View coverage and limits"`,
		`"The editor could not open "`,
	} {
		if strings.Contains(js, forbidden) {
			t.Errorf("surface catalog must select product copy by message ID, found literal/callback %q", forbidden)
		}
	}
	for _, token := range []string{
		`typeof this.options.message !== "function"`,
		`this.message = this.options.message`,
		`message("surfaces.status.showing", {`,
		`message("surfaces.error.editor_open", {`,
		`message("surfaces.owner.executable", { owner: owner })`,
		`message("surfaces.identity.http_route", { method: method, path: path })`,
	} {
		if !strings.Contains(js, token) {
			t.Errorf("surface catalog is missing explicit message usage %q", token)
		}
	}
	if strings.Contains(js, "array(trigger.dynamic_frontier).length > 0") {
		t.Fatal("an auxiliary frontier must not classify an otherwise exact surface as dynamically identified")
	}
	for _, token := range []string{
		`if (trigger.kind === "http_route_frontier") return true`,
		`status === "configured_route_inventory_unresolved"`,
		`trigger.provisional_id && hasDynamicEvidence(trigger)`,
	} {
		if !strings.Contains(js, token) {
			t.Errorf("surface identity classification is missing %q", token)
		}
	}
}

func readSurfaceCatalogAsset(t *testing.T, name string) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("templates", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(data)
}
