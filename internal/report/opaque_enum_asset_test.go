package report

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestReportAssetsRenderTechnicalEnumsByteIdentically(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is unavailable")
	}
	assetPath, err := filepath.Abs(filepath.Join("templates", "script.js"))
	if err != nil {
		t.Fatal(err)
	}
	runner := `
const fs = require("fs");
const vm = require("vm");

class Element {
  constructor(tag) {
    this.tagName = tag;
    this.className = "";
    this.textContent = "";
    this.children = [];
    this.attributes = {};
    this.classList = { add() {}, remove() {}, toggle() {} };
  }
  get childElementCount() { return this.children.length; }
  appendChild(child) { this.children.push(child); return child; }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  addEventListener() {}
}

function nodeText(node) {
  return String(node && node.textContent || "") + (node && node.children || []).map(nodeText).join("");
}

function render(locale) {
  const report = {
    report_language: locale,
    user_mechanisms: [], user_topics: [], user_sources: [], source_ids: {},
    openable_paths: [],
  };
  const document = {
    documentElement: { lang: locale },
    createElement(tag) { return new Element(tag); },
    createTextNode(value) { return { textContent: String(value), children: [] }; },
    getElementById(id) {
      return id === "rm-report-data" ? { textContent: JSON.stringify(report) } : null;
    },
    querySelectorAll() { return []; },
  };
  const window = {
    document,
    location: { search: "", hash: "", hostname: "example.test", protocol: "file:", pathname: "/report.html" },
    __REPOMAP_WORKSPACE_TEST__: {},
    addEventListener() {},
  };
  vm.runInNewContext(fs.readFileSync(process.argv[2].replace("script.js", "ui_messages.js"), "utf8"), { window });
  vm.runInNewContext(fs.readFileSync(process.argv[2], "utf8"), {
    window, document, URLSearchParams, Set, Map, AbortController, Promise,
  });
  const api = window.__REPOMAP_WORKSPACE_TEST__;
  const roots = [];
  roots.push(api.renderProofStaticRelation({
    id: "transition-one", from: "from", to: "to",
    relation: "registers_command", invocation: "direct_call",
    resolution: "same_package_static", certainty: "possible_static",
  }, {
    from: { id: "from", label: "From" },
    to: { id: "to", label: "To" },
  }));
  roots.push(api.renderLocalProof({
    proof: {
      archetype: "process", trace_quality: "accepted_with_normalization",
      anchors: [], transitions: [],
      slots: [{ kind: "application_callable", status: "not_applicable" }],
    },
    stats: {},
    stop: { reason: "budget_exhausted" },
  }, true));
  roots.push(api.renderSemanticArtifactCard({
    id: "artifact-one", kind: "future_artifact_kind",
    title: "Explanation", verdict: "insufficient_evidence",
  }));
  const research = new Element("section");
  api.appendResearchStage(research, "Stage", { status: "no_new_evidence" });
  api.appendResearchRound(research, "Round", {
    status: "budget_exhausted",
    selection_reason: "no_code_bearing_window",
    stop_reason: "provider_call_failed",
  });
  roots.push(research);
  return {
    text: roots.map(nodeText).join("\n"),
    symbolDetail: api.exactSymbolDetail("source_reference_match"),
    artifactKind: api.semanticArtifactKindLabel("future_artifact_kind"),
    selectedReason: api.researchSelectionReasonLabel("new_exact_evidence_and_high_value_frontier"),
    skippedReason: api.researchSelectionReasonLabel("runtime_only_frontier"),
    unknownReason: api.researchSelectionReasonLabel("future_gate_reason"),
    knownReasons: Object.fromEntries([
      "planned",
      "runtime_only_frontier",
      "unknown_candidate_ids",
      "no_code_bearing_bounded_window",
      "no_new_exact_evidence",
      "no_bounded_local_evidence",
      "new_exact_evidence_and_high_value_frontier",
      "targeted_round_limit",
    ].map((reason) => [reason, api.researchSelectionReasonLabel(reason)])),
  };
}

process.stdout.write(JSON.stringify({ en: render("en"), ru: render("ru") }));
`
	runnerPath := filepath.Join(t.TempDir(), "opaque-enum-render-test.js")
	if err := os.WriteFile(runnerPath, []byte(runner), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(node, runnerPath, assetPath).CombinedOutput()
	if err != nil {
		t.Fatalf("run exact enum renderer: %v\n%s", err, output)
	}
	var got map[string]struct {
		Text           string            `json:"text"`
		SymbolDetail   string            `json:"symbolDetail"`
		ArtifactKind   string            `json:"artifactKind"`
		SelectedReason string            `json:"selectedReason"`
		SkippedReason  string            `json:"skippedReason"`
		UnknownReason  string            `json:"unknownReason"`
		KnownReasons   map[string]string `json:"knownReasons"`
	}
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode exact enum renderer: %v\n%s", err, output)
	}
	for _, locale := range []string{"en", "ru"} {
		result := got[locale]
		for _, exact := range []string{
			"registers_command", "direct_call", "same_package_static", "possible_static",
			"accepted_with_normalization", "application_callable", "budget_exhausted",
			"future_artifact_kind", "insufficient_evidence", "no_new_evidence",
			"no_code_bearing_window", "provider_call_failed",
		} {
			if !strings.Contains(result.Text, exact) {
				t.Errorf("%s render lost exact enum %q:\n%s", locale, exact, result.Text)
			}
		}
		for _, mutated := range []string{
			"registers command", "direct call", "same package static", "possible static",
			"accepted with normalization", "application callable", "budget exhausted",
			"future artifact kind", "insufficient evidence", "no new evidence",
			"no code bearing window", "provider call failed",
		} {
			if strings.Contains(result.Text, mutated) {
				t.Errorf("%s render mutated enum into %q:\n%s", locale, mutated, result.Text)
			}
		}
		if result.SymbolDetail != "source_reference_match" || result.ArtifactKind != "future_artifact_kind" {
			t.Errorf("%s exact helpers = %q/%q", locale, result.SymbolDetail, result.ArtifactKind)
		}
		if result.UnknownReason != "future_gate_reason" {
			t.Errorf("%s unknown research gate reason = %q, want exact opaque code", locale, result.UnknownReason)
		}
	}
	if got["en"].SelectedReason != "New exact evidence addresses a high-value frontier" ||
		got["en"].SkippedReason != "Requires runtime observation" ||
		got["ru"].SelectedReason != "Новые точные свидетельства относятся к важной границе исследования" ||
		got["ru"].SkippedReason != "Требует наблюдения во время выполнения" {
		t.Errorf("typed research selection-reason labels = %#v", got)
	}
	for locale, expected := range map[string]map[string]string{
		"en": {
			"planned":                                    "Planned for targeted research",
			"runtime_only_frontier":                      "Requires runtime observation",
			"unknown_candidate_ids":                      "Candidate identifiers are unavailable",
			"no_code_bearing_bounded_window":             "No code-bearing bounded window was found",
			"no_new_exact_evidence":                      "No new exact evidence was found",
			"no_bounded_local_evidence":                  "No bounded local evidence was found",
			"new_exact_evidence_and_high_value_frontier": "New exact evidence addresses a high-value frontier",
			"targeted_round_limit":                       "Targeted research round limit reached",
		},
		"ru": {
			"planned":                                    "Запланировано для целевого исследования",
			"runtime_only_frontier":                      "Требует наблюдения во время выполнения",
			"unknown_candidate_ids":                      "Идентификаторы кандидатов недоступны",
			"no_code_bearing_bounded_window":             "Не найдено ограниченное окно с кодом",
			"no_new_exact_evidence":                      "Новых точных свидетельств не найдено",
			"no_bounded_local_evidence":                  "Ограниченные локальные свидетельства не найдены",
			"new_exact_evidence_and_high_value_frontier": "Новые точные свидетельства относятся к важной границе исследования",
			"targeted_round_limit":                       "Достигнут предел раундов целевого исследования",
		},
	} {
		for code, want := range expected {
			if got[locale].KnownReasons[code] != want {
				t.Errorf("%s research gate %q = %q, want %q", locale, code, got[locale].KnownReasons[code], want)
			}
		}
		if len(got[locale].KnownReasons) != len(expected) {
			t.Errorf("%s known research gates = %#v, want exactly %#v", locale, got[locale].KnownReasons, expected)
		}
	}
}

func TestArchitectureAssetKeepsOpaqueEnumsAndModelLabelsExact(t *testing.T) {
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

function inspect(locale) {
  const document = { documentElement: { lang: locale } };
  const window = { document, __REPOMAP_LAYOUT_TEST__: {} };
  vm.runInNewContext(fs.readFileSync(process.argv[2].replace("architecture_canvas.js", "ui_messages.js"), "utf8"), { window });
  vm.runInNewContext(fs.readFileSync(process.argv[2], "utf8"), { window });
  const api = window.__REPOMAP_LAYOUT_TEST__;
  return {
    unknownProofArea: api.proofAreaLabel("future_proof_area", window.RepomapUI.message),
    knownProofArea: api.proofAreaLabel("application_callable", window.RepomapUI.message),
    modelLabel: api.presentationValueLabel({ label: "model_label_with_underscores" }),
    enumValue: api.presentationValueLabel("behavior_grounded"),
  };
}

process.stdout.write(JSON.stringify({ en: inspect("en"), ru: inspect("ru") }));
`
	runnerPath := filepath.Join(t.TempDir(), "architecture-opaque-enum-test.js")
	if err := os.WriteFile(runnerPath, []byte(runner), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(node, runnerPath, assetPath).CombinedOutput()
	if err != nil {
		t.Fatalf("run architecture exact enum renderer: %v\n%s", err, output)
	}
	var got map[string]struct {
		UnknownProofArea string `json:"unknownProofArea"`
		KnownProofArea   string `json:"knownProofArea"`
		ModelLabel       string `json:"modelLabel"`
		EnumValue        string `json:"enumValue"`
	}
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode architecture exact enum renderer: %v\n%s", err, output)
	}
	for _, locale := range []string{"en", "ru"} {
		result := got[locale]
		if result.UnknownProofArea != "future_proof_area" ||
			result.ModelLabel != "model_label_with_underscores" ||
			result.EnumValue != "behavior_grounded" {
			t.Errorf("%s architecture identities = %#v", locale, result)
		}
	}
	if got["en"].KnownProofArea != "Application callable" ||
		got["ru"].KnownProofArea != "Вызываемый объект приложения" {
		t.Errorf("known proof-area product copy did not use typed catalog: en=%q ru=%q",
			got["en"].KnownProofArea, got["ru"].KnownProofArea)
	}

	for _, name := range []string{"script.js", "architecture_canvas.js"} {
		asset := readCanvasAsset(t, name)
		for _, forbidden := range []string{
			`.replaceAll("_", " ")`, `.replaceAll('_', ' ')`,
			`.replace(/_/g, " ")`, `.replace(/_/g, ' ')`,
			`.replaceAll("_", "/")`, `.replaceAll('_', '/')`,
		} {
			if strings.Contains(asset, forbidden) {
				t.Errorf("%s still mutates underscore-bearing identity with %s", name, forbidden)
			}
		}
	}
}
