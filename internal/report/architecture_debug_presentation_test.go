package report

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/flowproof"
	"github.com/dvordrova/repomap/internal/localization"
)

func TestArchitectureDebugPresentationClassifiesEveryRenderedDetailAndScenario(t *testing.T) {
	location := evidence.Location{Path: "cmd/server/main.go", Line: 17, Column: 3}
	productBuild := componentmap.ScenarioContext{
		ID: "go:linux:amd64", Name: "Recorded Go build scenario",
		Build: evidence.BuildContext{GOOS: "linux", GOARCH: "amd64", BuildTags: []string{"sqlite"}},
	}
	customScenario := componentmap.ScenarioContext{
		ID: "scenario-staging", Name: "Repository-specific staging scenario.",
	}
	productMemberProvenance := evidence.Provenance{
		Provider: "flowproof", Version: "v1", Operation: "anchor_file",
		Detail: "file contains an exact saved flow anchor", Location: &location,
	}
	customMemberProvenance := evidence.Provenance{
		Provider: "repository_adapter", Version: "v2", Operation: "member_fact",
		Detail: "Member fact explains source participation.", Location: &location,
	}
	productBindingProvenance := evidence.Provenance{
		Provider: "flowproof", Version: "v1", Operation: "bind_anchor_to_exact_member",
		Detail:   "binding produced directly from the saved typed anchor, not by presentation path matching",
		Location: &location,
	}
	customBindingProvenance := evidence.Provenance{
		Provider: "repository_adapter", Version: "v2", Operation: "binding",
		Detail: "Binding provenance explains the exact join.", Location: &location,
	}
	customSlotProvenance := evidence.Provenance{
		Provider: "repository_adapter", Version: "v2", Operation: "slot",
		Detail: "Slot provenance explains the applicability decision.", Location: &location,
	}
	opaqueSlotProvenance := evidence.Provenance{
		Provider: "go_syntax", Version: "v1", Operation: "inspect_handler_concurrency",
		Detail: "absent_from_handler_scope", Location: &location,
	}
	productRelationProvenance := evidence.Provenance{
		Provider: "report_repository_graph", Version: "v1", Operation: "saved_package_import",
		Detail: "exact saved package endpoints; source import callsite unavailable",
	}
	customRelationProvenance := evidence.Provenance{
		Provider: "repository_adapter", Version: "v2", Operation: "relation_probe",
		Detail: "Structural witness explains the relation.", Location: &location,
	}
	productPackageScenario := componentmap.ScenarioContext{
		ID:   "saved-package-graph",
		Name: "Saved package graph context; exact build values unavailable",
	}
	relation := componentmap.LocalRelation{
		ID:   "relation-package-import",
		From: componentmap.MemberID{Kind: componentmap.MemberPackage, Value: "package-a"},
		To:   componentmap.MemberID{Kind: componentmap.MemberPackage, Value: "package-b"},
		Kind: componentmap.StructuralRelationPackageImport,
		Provenance: []evidence.Provenance{
			productRelationProvenance,
			customRelationProvenance,
		},
		Scenarios: []componentmap.ScenarioContext{productPackageScenario, customScenario},
	}
	canonical := &ReportData{
		RepoName: "fixture",
		ArchitectureCanvas: &ArchitectureCanvas{
			Version: ArchitectureCanvasVersion,
			BehaviorAnchors: []componentmap.BehaviorAnchor{{
				ID: "anchor-main", Kind: componentmap.AnchorProcessEntry,
				Label: "Main process entry", Location: location,
				Scenario: productBuild,
				Producer: evidence.Provenance{
					Provider: "repository_adapter", Version: "v2", Operation: "anchor",
					Detail: "Anchor producer explains the detected entrypoint.", Location: &location,
				},
			}},
			Components: []ArchitectureComponent{{
				ID: "component-server", Name: "Server component",
				Members: []componentmap.Candidate{{
					ID:   componentmap.MemberID{Kind: componentmap.MemberSymbol, Value: "member-main"},
					Name: "main",
					Facts: []componentmap.LocalFact{{
						Kind: componentmap.FactDeclaration, Value: "main", Location: &location,
						Certainty: evidence.CertaintyStatic,
						Provenance: []evidence.Provenance{
							productMemberProvenance,
							customMemberProvenance,
						},
					}},
				}},
			}},
			Flows: []ArchitectureFlow{{
				ID: "flow-main", Name: "Main flow",
				Slots: []flowproof.Slot{{
					Kind: flowproof.SlotConcurrency, Status: flowproof.SlotNotApplicable,
					Provenance: []evidence.Provenance{customSlotProvenance, opaqueSlotProvenance},
				}},
				Steps: []ArchitectureFlowStep{{
					ID: "step-main", Kind: flowproof.AnchorFunction,
					Label: "Enter main", QualifiedName: "main", Location: &location,
					Binding: &componentmap.FlowAnchorBinding{
						FlowID: "flow-main", AnchorID: "step-main",
						MemberID: componentmap.MemberID{Kind: componentmap.MemberSymbol, Value: "member-main"},
						Location: &location, Certainty: evidence.CertaintyStatic,
						Provenance: []evidence.Provenance{
							productBindingProvenance,
							customBindingProvenance,
						},
						Scenarios: []componentmap.ScenarioContext{productBuild, customScenario},
					},
				}},
			}},
			StructuralFacts: []componentmap.LocalRelation{relation},
			StructuralEdges: []ArchitectureStructuralEdge{{
				ID: "edge-package-import", FromComponentID: "component-a",
				ToComponentID: "component-b", Witness: relation,
			}},
		},
	}
	before, err := json.Marshal(canonical)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := PreparePresentationLocalization(canonical, localization.LocaleRussian)
	if err != nil {
		t.Fatal(err)
	}

	translations := map[string]string{
		"Anchor producer explains the detected entrypoint.":    "Происхождение опоры объясняет найденную точку входа.",
		"Member fact explains source participation.":           "Факт элемента объясняет участие исходника.",
		"Slot provenance explains the applicability decision.": "Происхождение слота объясняет решение о применимости.",
		"Binding provenance explains the exact join.":          "Происхождение привязки объясняет точное соединение.",
		"Structural witness explains the relation.":            "Структурное свидетельство объясняет связь.",
		"Repository-specific staging scenario.":                "Сценарий промежуточной среды этого репозитория.",
	}
	projection := russianPresentationProjection(prepared)
	for english, russian := range translations {
		matched := 0
		for _, field := range prepared.Canonical.Fields {
			if field.Text == english {
				projection.Translations[field.ID] = russian
				matched++
			}
		}
		if matched == 0 {
			t.Fatalf("presentation text %q is absent", english)
		}
	}
	for _, excluded := range []string{
		productMemberProvenance.Detail,
		productBindingProvenance.Detail,
		productRelationProvenance.Detail,
		productBuild.Name,
		productPackageScenario.Name,
		opaqueSlotProvenance.Detail,
	} {
		for _, field := range prepared.Canonical.Fields {
			if field.Text == excluded {
				t.Fatalf("catalog or opaque Architecture value entered prose inventory: %q", excluded)
			}
		}
	}

	projected, result, err := ApplyPresentationLocalization(canonical, prepared, projection)
	if err != nil {
		t.Fatalf("apply debug projection: %v result=%#v", err, result)
	}
	if result.Fallback || len(result.Diagnostics) != 0 {
		t.Fatalf("complete debug projection result = %#v", result)
	}
	if got := projected.ArchitectureCanvas.StructuralFacts[0].Provenance[1].Detail; got != customRelationProvenance.Detail {
		t.Fatalf("structural fact detail = %q", got)
	}
	if got := projected.ArchitectureCanvas.StructuralEdges[0].Witness.Provenance[1].Detail; got != customRelationProvenance.Detail {
		t.Fatalf("structural edge detail = %q", got)
	}
	if got := projected.ArchitectureCanvas.Flows[0].Slots[0].Provenance[1].Detail; got != opaqueSlotProvenance.Detail {
		t.Fatalf("opaque enum detail changed = %q", got)
	}
	if projected.ArchitectureCanvas.Flows[0].Steps[0].Binding.Scenarios[0].ID != productBuild.ID ||
		projected.ArchitectureCanvas.Flows[0].Steps[0].Binding.Scenarios[0].Build.GOOS != "linux" ||
		projected.ArchitectureCanvas.Flows[0].Steps[0].Binding.Provenance[0].Provider != "flowproof" ||
		projected.ArchitectureCanvas.Flows[0].Steps[0].Binding.Provenance[0].Operation != "bind_anchor_to_exact_member" {
		t.Fatalf("opaque Architecture identity changed: %#v", projected.ArchitectureCanvas.Flows[0].Steps[0].Binding)
	}

	rendered := reportDataForRendering(projected)
	if rendered.ArchitectureCanvas.BehaviorAnchors[0].Producer.Detail !=
		canonical.ArchitectureCanvas.BehaviorAnchors[0].Producer.Detail ||
		rendered.ArchitectureCanvas.BehaviorAnchors[0].Scenario.Name !=
			canonical.ArchitectureCanvas.BehaviorAnchors[0].Scenario.Name ||
		rendered.ArchitectureCanvas.Components[0].Members[0].Facts[0].Provenance[0].Detail !=
			canonical.ArchitectureCanvas.Components[0].Members[0].Facts[0].Provenance[0].Detail ||
		rendered.ArchitectureCanvas.Components[0].Members[0].Facts[0].Provenance[1].Detail !=
			canonical.ArchitectureCanvas.Components[0].Members[0].Facts[0].Provenance[1].Detail ||
		rendered.ArchitectureCanvas.Flows[0].Slots[0].Provenance[0].Detail !=
			canonical.ArchitectureCanvas.Flows[0].Slots[0].Provenance[0].Detail ||
		rendered.ArchitectureCanvas.Flows[0].Steps[0].Binding.Provenance[1].Detail !=
			canonical.ArchitectureCanvas.Flows[0].Steps[0].Binding.Provenance[1].Detail ||
		rendered.ArchitectureCanvas.Flows[0].Steps[0].Binding.Provenance[0].Detail !=
			canonical.ArchitectureCanvas.Flows[0].Steps[0].Binding.Provenance[0].Detail ||
		rendered.ArchitectureCanvas.Flows[0].Steps[0].Binding.Scenarios[0].Name !=
			canonical.ArchitectureCanvas.Flows[0].Steps[0].Binding.Scenarios[0].Name ||
		rendered.ArchitectureCanvas.Flows[0].Steps[0].Binding.Scenarios[1].Name !=
			canonical.ArchitectureCanvas.Flows[0].Steps[0].Binding.Scenarios[1].Name ||
		rendered.ArchitectureCanvas.StructuralEdges[0].Witness.Provenance[0].Detail !=
			canonical.ArchitectureCanvas.StructuralEdges[0].Witness.Provenance[0].Detail ||
		rendered.ArchitectureCanvas.StructuralEdges[0].Witness.Provenance[1].Detail !=
			canonical.ArchitectureCanvas.StructuralEdges[0].Witness.Provenance[1].Detail ||
		rendered.ArchitectureCanvas.StructuralEdges[0].Witness.Scenarios[0].Name !=
			canonical.ArchitectureCanvas.StructuralEdges[0].Witness.Scenarios[0].Name ||
		rendered.ArchitectureCanvas.StructuralEdges[0].Witness.Scenarios[1].Name !=
			canonical.ArchitectureCanvas.StructuralEdges[0].Witness.Scenarios[1].Name {
		t.Fatal("raw Architecture provenance or scenario values changed in render data")
	}
	for _, russian := range translations {
		found := false
		for _, text := range rendered.architectureDebugPresentation {
			if text == russian {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("transient Architecture presentation is missing %q", russian)
		}
	}
	html, err := RenderHTML(projected)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(html, []byte("architecture_debug_presentation")) ||
		!bytes.Contains(html, []byte(translations[customSlotProvenance.Detail])) {
		t.Fatal("RU render payload does not expose the transient Architecture presentation lookup")
	}
	after, err := json.Marshal(canonical)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("Architecture debug projection or render clone mutated canonical report")
	}
}

func TestArchitectureDebugProductCopyUsesRussianCatalog(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is unavailable")
	}
	assetPath, err := filepath.Abs(filepath.Join("templates", "architecture_canvas.js"))
	if err != nil {
		t.Fatal(err)
	}
	runner := `
const fs = require("fs");
const vm = require("vm");
const window = {
  document: { documentElement: { lang: "ru" } },
  __REPOMAP_LAYOUT_TEST__: {},
};
const context = { window, document: window.document, Set, Map, Object, String, Array, Error, Number };
vm.runInNewContext(fs.readFileSync(process.argv[2].replace("architecture_canvas.js", "ui_messages.js"), "utf8"), context);
vm.runInNewContext(fs.readFileSync(process.argv[2], "utf8"), context);
const api = window.__REPOMAP_LAYOUT_TEST__;
const provenanceID = api.architectureProvenanceProductMessageID({
  provider: "report_repository_graph", operation: "saved_package_import",
});
const scenarioID = api.architectureScenarioProductMessageID({
  id: "go:linux:amd64", build: { goos: "linux", goarch: "amd64" },
});
const customAddress = "architecture/flows/flow-main/slots/concurrency/provenance/0/detail";
process.stdout.write(JSON.stringify({
  provenanceID,
  provenance: window.RepomapUI.message(provenanceID),
  scenarioID,
  scenario: window.RepomapUI.message(scenarioID),
  custom: api.architecturePresentationText(
    { [customAddress]: "Переведённое объяснение." },
    customAddress,
    "Canonical English explanation."
  ),
  rawFallback: api.architecturePresentationText({}, customAddress, "opaque_enum"),
}));
`
	runnerPath := filepath.Join(t.TempDir(), "architecture-debug-presentation.js")
	if err := os.WriteFile(runnerPath, []byte(runner), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(node, runnerPath, assetPath).CombinedOutput()
	if err != nil {
		t.Fatalf("run Architecture debug presentation contract: %v\n%s", err, output)
	}
	var got struct {
		ProvenanceID string `json:"provenanceID"`
		Provenance   string `json:"provenance"`
		ScenarioID   string `json:"scenarioID"`
		Scenario     string `json:"scenario"`
		Custom       string `json:"custom"`
		RawFallback  string `json:"rawFallback"`
	}
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode Architecture debug presentation result: %v\n%s", err, output)
	}
	if got.ProvenanceID != "architecture.provenance.saved_package_import" ||
		got.Provenance != "точные сохранённые границы пакетов; место импорта в исходниках недоступно" ||
		got.ScenarioID != "architecture.scenario.recorded_go_build" ||
		got.Scenario != "Сценарий сохранённой сборки Go" ||
		got.Custom != "Переведённое объяснение." ||
		got.RawFallback != "opaque_enum" {
		t.Fatalf("RU Architecture debug catalog projection = %#v", got)
	}
}
