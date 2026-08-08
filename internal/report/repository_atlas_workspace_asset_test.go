package report

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepositoryAtlasWorkspaceShelfUsesOnlyExactSavedEvidence(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is unavailable")
	}
	assetPath, err := filepath.Abs(filepath.Join("templates", "script.js"))
	if err != nil {
		t.Fatal(err)
	}
	const runner = `
const fs = require("fs");
const vm = require("vm");
class Element {
  constructor(tag) {
    this.tagName = tag;
    this.className = "";
    this.textContent = "";
    this.children = [];
    this.attributes = {};
    this.hidden = false;
    this.classList = { add() {}, remove() {}, toggle() {} };
  }
  get childNodes() { return this.children; }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  getAttribute(name) { return this.attributes[name] || null; }
  appendChild(child) { this.children.push(child); return child; }
  append(...children) { this.children.push(...children); }
  replaceChildren(...children) { this.children = children; }
}
function walk(root) {
  const result = [];
  (function visit(node) { result.push(node); (node.children || []).forEach(visit); })(root);
  return result;
}
function text(root) { return walk(root).map((node) => String(node.textContent || "")).join(""); }
function run(report, language) {
  const root = new Element("section");
  const window = {
    location: { search: "", hash: "#/overview", hostname: "example.test", protocol: "file:", pathname: "/report.html" },
    history: {
      state: null,
      pushState(state, _, hash) { this.state = state; window.location.hash = hash; },
      replaceState(state, _, hash) { this.state = state; window.location.hash = hash; },
      back() {},
    },
    __REPOMAP_WORKSPACE_TEST__: {}, addEventListener() {}, open() {}, scrollTo() {},
  };
  const document = {
    createElement(tag) { return new Element(tag); },
    createTextNode(value) { const node = new Element("#text"); node.textContent = String(value); return node; },
    getElementById(id) {
      if (id === "rm-report-data") return { textContent: JSON.stringify(report) };
      return null;
    },
    querySelector() { return null; }, querySelectorAll() { return []; },
  };
  document.documentElement = { lang: language };
  window.document = document;
  vm.runInNewContext(fs.readFileSync(process.argv[2].replace("script.js", "ui_messages.js"), "utf8"), { window });
  vm.runInNewContext(fs.readFileSync(process.argv[2], "utf8"), {
    window, document, URLSearchParams, Set, Map, AbortController, Promise,
  });
  const api = window.__REPOMAP_WORKSPACE_TEST__;
  const shelf = api.repositoryAtlasWorkspaceShelf();
  api.renderRepositoryAtlasWorkspaceShelf(root);
  const nodes = walk(root);
  const unitCards = nodes.filter((node) => String(node.className).split(/\s+/).includes("rm-atlas-unit-card"));
  const unitHeaders = nodes.filter((node) => String(node.className).split(/\s+/).includes("rm-atlas-unit-header"));
  const unitTags = nodes.filter((node) => String(node.className).split(/\s+/).includes("rm-atlas-unit-tag"));
  const unitAuthorityBadges = unitCards.reduce((count, card) => count + walk(card).filter((node) => String(node.className).split(/\s+/).includes("rm-atlas-authority")).length, 0);
  const sourceButtons = nodes.filter((node) => String(node.className).split(/\s+/).includes("rm-atlas-source-action"));
  const packageDisclosure = nodes.find((node) => String(node.className).split(/\s+/).includes("rm-atlas-package-disclosure"));
  const packageRows = packageDisclosure ? walk(packageDisclosure).filter((node) => String(node.className).split(/\s+/).includes("rm-atlas-package-row")) : [];
  const packageActions = packageDisclosure ? walk(packageDisclosure).filter((node) => String(node.className).split(/\s+/).includes("rm-atlas-package-action")) : [];
  const packageUnavailable = packageDisclosure ? walk(packageDisclosure).filter((node) => String(node.className).split(/\s+/).includes("rm-atlas-package-unavailable")) : [];
  const packagePrefixes = packageDisclosure ? walk(packageDisclosure).filter((node) => String(node.className).split(/\s+/).includes("rm-atlas-package-prefix")) : [];
  const packageNames = packageDisclosure ? walk(packageDisclosure).filter((node) => String(node.className).split(/\s+/).includes("rm-atlas-package-name")) : [];
  const packageSourceSummaries = packageDisclosure ? walk(packageDisclosure).filter((node) => String(node.className).split(/\s+/).includes("rm-atlas-package-source-summary")) : [];
  const packageSummary = packageDisclosure && walk(packageDisclosure).find((node) => String(node.className).split(/\s+/).includes("rm-atlas-package-summary"));
  const compactStatus = nodes.filter((node) => String(node.className).split(/\s+/).includes("rm-atlas-compact-status"));
  const compactStatusItems = compactStatus.length ? walk(compactStatus[0]).filter((node) => node.attributes && node.attributes["data-rm-status-code"]) : [];
  const section = root.children[0];
  const sectionClasses = section ? section.children.map((node) => String(node.className || "")) : [];
	const packageActionTargets = packageActions.map((action) => ({
		tag: String(action.tagName || "").toLowerCase(),
		href: action.getAttribute("href") || "",
		hasClick: typeof action.onclick === "function",
	}));
  const packageSourceStates = packageActions.filter((action) => typeof action.onclick === "function").map((action) => {
    action.onclick();
    return api.workspaceStateSnapshot().sourceLocation;
  });
  if (sourceButtons.length && typeof sourceButtons[0].onclick === "function") sourceButtons[0].onclick();
  return {
    units: shelf && shelf.units.length || 0,
    relations: shelf && shelf.relations.length || 0,
    omitted: shelf && shelf.omittedRelations || 0,
    rendered: text(root),
    unitAuthorityBadges,
    sourceButtons: sourceButtons.length,
    sourceState: api.workspaceStateSnapshot().sourceLocation,
    topologyCards: unitCards.length + unitHeaders.length,
    unitTags: unitTags.map((node) => String(node.textContent || "")),
    topologyUnitIDs: shelf && shelf.topologyDisplayUnits.map((unit) => unit.unitIDs) || [],
    relationUnitIDs: shelf && shelf.relations.map((relation) => relation.unit.id) || [],
    packageRows: packageRows.length,
    packageActions: packageActions.length,
    packageUnavailable: packageUnavailable.map((node) => node.getAttribute("data-rm-source-state")),
    packagePrefixes: packagePrefixes.map((node) => String(node.textContent || "")),
    packageNames: packageNames.map((node) => String(node.textContent || "")),
	packageActionTargets,
    packageSourceStates,
    packageSourceSummaries: packageSourceSummaries.map((node) => text(node)),
    compactStatusCount: compactStatus.length,
    compactStatusCodes: compactStatusItems.map((node) => node.attributes["data-rm-status-code"]),
    packageSummary: packageSummary ? String(packageSummary.textContent || "") : "",
    packageDisclosureOpen: !!(packageDisclosure && packageDisclosure.open),
    relationPosition: sectionClasses.findIndex((name) => name.includes("rm-atlas-relations-heading")),
    unitsPosition: sectionClasses.findIndex((name) => name.includes("rm-atlas-units-heading")),
    packagePosition: sectionClasses.findIndex((name) => name.includes("rm-atlas-package-disclosure")),
    compactStatusPosition: sectionClasses.findIndex((name) => name.includes("rm-atlas-compact-status")),
  };
}
const source = {
  path: "cmd/server/startup.go", start_line: 8, end_line: 12,
  presentation_sha256: "a".repeat(64), content_sha256: "b".repeat(64),
  related_evidence_ids: ["etcd-package-evidence-0"],
  lines: [
    { line: 8, text: "package server" },
    { line: 10, text: "func main() {", highlight: true },
    { line: 12, text: "}" },
  ],
};
const secondPackageSource = {
  path: "pkg/one/package.go", start_line: 1, end_line: 3,
  presentation_sha256: "e".repeat(64), content_sha256: "f".repeat(64),
  related_evidence_ids: ["etcd-package-evidence-1"],
  lines: [
    { line: 1, text: "package one", highlight: true },
    { line: 3, text: "const Name = \"one\"" },
  ],
};
const report = {
  user_mechanisms: [], user_topics: [], user_sources: [source],
  openable_paths: ["cmd/server/startup.go", "cmd/server/uncovered.go"], source_ids: {},
  github_source_links: {
    repository_url: "https://github.com/example/fixture",
    revision: "1".repeat(40),
  },
  repository_atlas: {
    version: 1,
    units: [
      { id: "unit-secret-repo", kind: "repository", name: "github.com/casdoor/casdoor" },
      { id: "unit-secret-module", kind: "module", parent_id: "unit-secret-repo", name: "github.com/casdoor/casdoor" },
      { id: "unit-secret-app", kind: "app", parent_id: "unit-secret-repo", name: "github.com/casdoor/casdoor" },
      { id: "unit-secret-package", kind: "package", parent_id: "unit-secret-module", name: "github.com/casdoor/casdoor" },
    ],
    entities: [
      { id: "entity-secret-surface", kind: "surface", unit_id: "unit-secret-app" },
      { id: "entity-secret-operation", kind: "operation", unit_id: "unit-secret-app" },
      { id: "entity-secret-resource", kind: "resource", unit_id: "unit-secret-app" },
    ],
    evidence: [
      { id: "evidence-secret-covered", unit_id: "unit-secret-app", location: { path: "cmd/server/startup.go", line: 10 } },
      { id: "evidence-secret-uncovered", unit_id: "unit-secret-app", location: { path: "cmd/server/uncovered.go", line: 3 } },
      { id: "evidence-secret-package-nondeclaration", unit_id: "unit-secret-package", location: { path: "cmd/server/startup.go", line: 10 }, provenance: { provider: "gofacts", version: "entrypoint-anchor-v1", operation: "build_selected_main_declaration" } },
      { id: "evidence-secret-package-near-match", unit_id: "unit-secret-package", location: { path: "cmd/server/startup.go", line: 10 }, provenance: { provider: "gofacts", version: "package-declaration-v0", operation: "package_declaration" } },
    ],
    relations: [
      { id: "relation-secret-covered", unit_id: "unit-secret-app", kind: "exposes", phase: "startup", authority: "resolved", source: { kind: "surface", id: "entity-secret-surface" }, target: { kind: "operation", id: "entity-secret-operation" }, evidence_refs: ["evidence-secret-covered"] },
      { id: "relation-secret-uncovered", unit_id: "unit-secret-app", kind: "exposes", phase: "startup", authority: "resolved", source: { kind: "surface", id: "entity-secret-surface" }, target: { kind: "operation", id: "entity-secret-operation" }, evidence_refs: ["evidence-secret-uncovered"] },
      { id: "relation-secret-inferred", unit_id: "unit-secret-app", kind: "exposes", phase: "startup", authority: "inferred", source: { kind: "surface", id: "entity-secret-surface" }, target: { kind: "operation", id: "entity-secret-operation" }, evidence_refs: ["evidence-secret-covered"] },
      { id: "relation-secret-runtime", unit_id: "unit-secret-app", kind: "exposes", phase: "runtime", authority: "resolved", source: { kind: "surface", id: "entity-secret-surface" }, target: { kind: "operation", id: "entity-secret-operation" }, evidence_refs: ["evidence-secret-covered"] },
      { id: "relation-secret-resource", unit_id: "unit-secret-app", kind: "exposes", phase: "startup", authority: "resolved", source: { kind: "resource", id: "entity-secret-resource" }, target: { kind: "operation", id: "entity-secret-operation" }, evidence_refs: ["evidence-secret-covered"] },
    ],
  },
};
const en = run(report, "en");
const ru = run(report, "ru");
const unavailable = run(Object.assign({}, report, { repository_atlas: null }), "en");
const nonUserSourceReport = JSON.parse(JSON.stringify(report));
nonUserSourceReport.user_sources = [];
nonUserSourceReport.study_map = { directions: [{ reading_anchors: [{ source }] }] };
const nonUserSource = run(nonUserSourceReport, "en");
const missingBindingReport = JSON.parse(JSON.stringify(report));
missingBindingReport.repository_atlas.evidence.push({
  id: "evidence-secret-package-unbound", unit_id: "unit-secret-package",
  location: { path: "cmd/server/startup.go", line: 10 },
  provenance: { provider: "gofacts", version: "package-declaration-v1", operation: "package_declaration" },
});
const missingBinding = run(missingBindingReport, "en");
const etcdReport = {
  user_mechanisms: [], user_topics: [], user_sources: [source, secondPackageSource],
  openable_paths: ["cmd/server/startup.go", "pkg/one/package.go"], source_ids: {},
  github_source_links: {
    repository_url: "https://github.com/example/fixture",
    revision: "1".repeat(40),
  },
  repository_atlas: { version: 1, units: [], entities: [], evidence: [], relations: [] },
};
etcdReport.repository_atlas.units.push({ id: "etcd-repository", kind: "repository", name: "etcd" });
etcdReport.repository_atlas.units.push({ id: "etcd-module", kind: "module", parent_id: "etcd-repository", name: "go.etcd.io/etcd" });
for (let index = 0; index < 30; index++) {
  etcdReport.repository_atlas.units.push({ id: "etcd-app-" + index, kind: index % 2 ? "service" : "app", parent_id: "etcd-module", name: "runtime " + index });
}
for (let index = 0; index < 183; index++) {
  etcdReport.repository_atlas.units.push({
    id: "etcd-package-" + index, kind: "package", parent_id: "etcd-module",
    name: index === 0 ? "go.etcd.io/etcd" : "go.etcd.io/etcd/pkg/" + index,
  });
}
etcdReport.repository_atlas.evidence.push({
  id: "etcd-package-evidence-0", unit_id: "etcd-package-0",
  location: { path: "cmd/server/startup.go", line: 10 },
  provenance: { provider: "gofacts", version: "package-declaration-v1", operation: "package_declaration" },
});
etcdReport.repository_atlas.evidence.push({
  id: "etcd-package-evidence-1", unit_id: "etcd-package-1",
  location: { path: "pkg/one/package.go", line: 1 },
  provenance: { provider: "gofacts", version: "package-declaration-v1", operation: "package_declaration" },
});
for (let index = 0; index < 18; index++) {
  etcdReport.repository_atlas.entities.push({ id: "etcd-surface-" + index, kind: "surface", unit_id: "etcd-app-" + index });
  etcdReport.repository_atlas.entities.push({ id: "etcd-operation-" + index, kind: "operation", unit_id: "etcd-app-" + index });
  etcdReport.repository_atlas.evidence.push({ id: "etcd-evidence-" + index, unit_id: "etcd-app-" + index, location: { path: "cmd/server/startup.go", line: 10 } });
  etcdReport.repository_atlas.relations.push({
    id: "etcd-relation-" + index, unit_id: "etcd-app-" + index,
    kind: "exposes", phase: "startup", authority: "resolved",
    source: { kind: "surface", id: "etcd-surface-" + index },
    target: { kind: "operation", id: "etcd-operation-" + index },
    evidence_refs: ["etcd-evidence-" + index],
  });
}
const etcdEN = run(etcdReport, "en");
const etcdRU = run(etcdReport, "ru");
// P7-B: a package without an exact saved source still navigates to its
// representative file — the first openable file of the package in the
// saved repository graph. The graph proves the file exists; the package
// itself has no single line, so the file is the exact boundary opened.
const representativeReport = JSON.parse(JSON.stringify(report));
representativeReport.repository_graph = {
  version: 1,
  packages: [
    {
      canonical_package_path: "github.com/casdoor/casdoor",
      files: ["cmd/server/startup.go"],
    },
  ],
};
representativeReport.openable_paths.push("cmd/server/startup.go");
const representative = run(representativeReport, "en");
const representativeWithoutGraph = run(JSON.parse(JSON.stringify(report)), "en");
const conflictReport = JSON.parse(JSON.stringify(report));
conflictReport.repository_atlas.evidence.push({
  id: "evidence-secret-package", unit_id: "unit-secret-package",
  location: { path: "cmd/server/startup.go", line: 10 },
  provenance: { provider: "gofacts", version: "package-declaration-v1", operation: "package_declaration" },
});
conflictReport.user_sources[0].related_evidence_ids.push("evidence-secret-package");
const conflictingSource = JSON.parse(JSON.stringify(source));
conflictingSource.presentation_sha256 = "c".repeat(64);
conflictingSource.content_sha256 = "d".repeat(64);
conflictingSource.lines[1].text = "func conflicting() {";
conflictingSource.related_evidence_ids.push("evidence-secret-package");
conflictReport.user_sources.push(conflictingSource);
const conflict = run(conflictReport, "en");
const multipleDeclarationsReport = JSON.parse(JSON.stringify(report));
multipleDeclarationsReport.openable_paths.push("cmd/server/other.go");
multipleDeclarationsReport.user_sources[0].related_evidence_ids.push("evidence-secret-package-first");
multipleDeclarationsReport.user_sources.push({
  path: "cmd/server/other.go", start_line: 1, end_line: 3,
  presentation_sha256: "e".repeat(64), content_sha256: "f".repeat(64),
  related_evidence_ids: ["evidence-secret-package-second"],
  lines: [
    { line: 1, text: "package server" },
    { line: 2, text: "func other() {}", highlight: true },
  ],
});
multipleDeclarationsReport.repository_atlas.evidence.push(
  { id: "evidence-secret-package-first", unit_id: "unit-secret-package", location: { path: "cmd/server/startup.go", line: 10 }, provenance: { provider: "gofacts", version: "package-declaration-v1", operation: "package_declaration" } },
  { id: "evidence-secret-package-second", unit_id: "unit-secret-package", location: { path: "cmd/server/other.go", line: 2 }, provenance: { provider: "gofacts", version: "package-declaration-v1", operation: "package_declaration" } },
);
const multipleDeclarations = run(multipleDeclarationsReport, "en");
const mixedPrefixReport = JSON.parse(JSON.stringify(etcdReport));
mixedPrefixReport.repository_atlas.units.find((unit) => unit.id === "etcd-package-182").name = "example.net/outside";
const mixedPrefix = run(mixedPrefixReport, "en");
const strippedStaticReport = JSON.parse(JSON.stringify(report));
strippedStaticReport.user_sources = [];
delete strippedStaticReport.study_map;
strippedStaticReport.repository_atlas.units.forEach((unit) => {
  unit.name = "github.com/example/repository";
});
strippedStaticReport.openable_paths = ["internal/server/start.go"];
strippedStaticReport.github_source_links = {
  repository_url: "https://github.com/example/repository",
  revision: "a".repeat(40),
};
strippedStaticReport.repository_atlas.evidence.push({
  id: "evidence-secret-package-static", unit_id: "unit-secret-package",
  location: { path: "internal/server/start.go", line: 10 },
  provenance: { provider: "gofacts", version: "package-declaration-v1", operation: "package_declaration" },
});
const strippedStatic = run(strippedStaticReport, "en");
const prototypeNameReport = JSON.parse(JSON.stringify(report));
prototypeNameReport.repository_atlas.units.forEach((unit) => {
  if (unit.kind !== "package") unit.name = "__proto__";
});
const prototypeName = run(prototypeNameReport, "en");
process.stdout.write(JSON.stringify({ en, ru, unavailable, nonUserSource, missingBinding, etcdEN, etcdRU, conflict, multipleDeclarations, mixedPrefix, strippedStatic, prototypeName, representative, representativeWithoutGraph }));
`
	runnerPath := filepath.Join(t.TempDir(), "repository-atlas-workspace-test.js")
	if err := os.WriteFile(runnerPath, []byte(runner), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(node, runnerPath, assetPath).CombinedOutput()
	if err != nil {
		t.Fatalf("run Repository Atlas workspace smoke: %v\n%s", err, output)
	}
	type result struct {
		Units                int        `json:"units"`
		Relations            int        `json:"relations"`
		Omitted              int        `json:"omitted"`
		Rendered             string     `json:"rendered"`
		UnitAuthorityBadges  int        `json:"unitAuthorityBadges"`
		SourceButtons        int        `json:"sourceButtons"`
		TopologyCards        int        `json:"topologyCards"`
		UnitTags             []string   `json:"unitTags"`
		TopologyUnitIDs      [][]string `json:"topologyUnitIDs"`
		RelationUnitIDs      []string   `json:"relationUnitIDs"`
		PackageRows          int        `json:"packageRows"`
		PackageActions       int        `json:"packageActions"`
		PackageUnavailable   []string   `json:"packageUnavailable"`
		PackagePrefixes      []string   `json:"packagePrefixes"`
		PackageNames         []string   `json:"packageNames"`
		PackageActionTargets []struct {
			Tag      string `json:"tag"`
			Href     string `json:"href"`
			HasClick bool   `json:"hasClick"`
		} `json:"packageActionTargets"`
		PackageSourceSummaries []string `json:"packageSourceSummaries"`
		CompactStatusCount     int      `json:"compactStatusCount"`
		CompactStatusCodes     []string `json:"compactStatusCodes"`
		PackageSummary         string   `json:"packageSummary"`
		PackageDisclosureOpen  bool     `json:"packageDisclosureOpen"`
		RelationPosition       int      `json:"relationPosition"`
		UnitsPosition          int      `json:"unitsPosition"`
		PackagePosition        int      `json:"packagePosition"`
		CompactStatusPosition  int      `json:"compactStatusPosition"`
		SourceState            *struct {
			Path        string `json:"path"`
			Line        int    `json:"line"`
			DrawerFirst bool   `json:"drawerFirst"`
		} `json:"sourceState"`
		PackageSourceStates []struct {
			Path        string `json:"path"`
			Line        int    `json:"line"`
			DrawerFirst bool   `json:"drawerFirst"`
		} `json:"packageSourceStates"`
	}
	var got struct {
		EN                    result `json:"en"`
		RU                    result `json:"ru"`
		Unavailable           result `json:"unavailable"`
		NonUserSource         result `json:"nonUserSource"`
		MissingBinding        result `json:"missingBinding"`
		EtcdEN                result `json:"etcdEN"`
		EtcdRU                result `json:"etcdRU"`
		Conflict              result `json:"conflict"`
		MultipleDeclarations  result `json:"multipleDeclarations"`
		MixedPrefix           result `json:"mixedPrefix"`
		StrippedStatic        result `json:"strippedStatic"`
		PrototypeName         result `json:"prototypeName"`
		Representative        result `json:"representative"`
		RepresentativeNoGraph result `json:"representativeWithoutGraph"`
	}
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode Repository Atlas workspace result: %v\n%s", err, output)
	}
	for language, current := range map[string]result{"en": got.EN, "ru": got.RU} {
		if current.Units != 4 || current.TopologyCards != 1 || current.PackageRows != 1 ||
			current.PackageActions != 0 || len(current.PackageUnavailable) != 1 ||
			current.PackageUnavailable[0] != "unavailable" || current.UnitAuthorityBadges != 0 ||
			current.Relations != 1 || current.Omitted != 1 || current.SourceButtons != 1 {
			t.Fatalf("%s shelf counts = units %d unit authority badges %d relations %d omitted %d buttons %d", language, current.Units, current.UnitAuthorityBadges, current.Relations, current.Omitted, current.SourceButtons)
		}
		if len(current.UnitTags) != 3 || len(current.PackagePrefixes) != 1 ||
			current.PackagePrefixes[0] != "github.com/casdoor/casdoor/" {
			t.Fatalf("%s coalesced units/packages = %#v", language, current)
		}
		if len(current.TopologyUnitIDs) != 1 ||
			strings.Join(current.TopologyUnitIDs[0], ",") != "unit-secret-repo,unit-secret-module,unit-secret-app" ||
			len(current.RelationUnitIDs) != 1 || current.RelationUnitIDs[0] != "unit-secret-app" {
			t.Fatalf("%s coalescing changed canonical unit identities: %#v", language, current)
		}
		if len(current.PackageSourceSummaries) != 1 || len(current.PackageSourceStates) != 0 {
			t.Fatalf("%s non-exact package provenance became actionable: %#v", language, current)
		}
		if current.CompactStatusCount != 0 || current.UnitsPosition < 0 || current.PackagePosition < 0 ||
			current.RelationPosition < 0 || current.UnitsPosition >= current.PackagePosition ||
			current.PackagePosition >= current.RelationPosition {
			t.Fatalf("%s useful shelf order/status = %#v", language, current)
		}
		// Decision 222: the exact source action is a GitHub/GitLab jump in a
		// new tab — it never opens an inline code drawer or mutates state.
		if current.SourceState != nil {
			t.Fatalf("%s exact source action mutated workspace state: %#v", language, current.SourceState)
		}
		for _, forbidden := range []string{
			"unit-secret", "entity-secret", "evidence-secret", "relation-secret",
			"cmd/server/startup.go", "cmd/server/uncovered.go",
		} {
			if strings.Contains(current.Rendered, forbidden) {
				t.Fatalf("%s shelf leaked canonical identity or path %q in %q", language, forbidden, current.Rendered)
			}
		}
	}
	for _, want := range []string{"Repository Atlas", "github.com/casdoor/casdoor", "Repository", "Module", "Application", "root package", "1 without an exact saved source.", "Process entry surface", "Application start operation", "Authority: Resolved"} {
		if !strings.Contains(got.EN.Rendered, want) {
			t.Fatalf("English shelf missing %q in %q", want, got.EN.Rendered)
		}
	}
	for _, want := range []string{"Атлас репозитория", "github.com/casdoor/casdoor", "Репозиторий", "Модуль", "Приложение", "корневой пакет", "Без точного сохранённого исходника: 1.", "Точка входа процесса", "Операция запуска приложения", "Основание: локально подтверждено"} {
		if !strings.Contains(got.RU.Rendered, want) {
			t.Fatalf("Russian shelf missing %q in %q", want, got.RU.Rendered)
		}
	}
	if !strings.Contains(got.Unavailable.Rendered, "Repository Atlas is unavailable for this run.") || got.Unavailable.SourceButtons != 0 {
		t.Fatalf("unavailable Atlas state = %#v", got.Unavailable)
	}
	if got.NonUserSource.Relations != 0 || got.NonUserSource.Omitted != 2 || got.NonUserSource.SourceButtons != 0 ||
		got.NonUserSource.RelationPosition >= 0 ||
		got.NonUserSource.CompactStatusCount != 1 ||
		strings.Join(got.NonUserSource.CompactStatusCodes, ",") != "no_exact_source_backed_relations" ||
		got.NonUserSource.CompactStatusPosition <= got.NonUserSource.PackagePosition {
		t.Fatalf("non-UserSource evidence became clickable: %#v", got.NonUserSource)
	}
	if got.MissingBinding.PackageActions != 0 || len(got.MissingBinding.PackageUnavailable) != 1 ||
		got.MissingBinding.PackageUnavailable[0] != "unavailable" ||
		len(got.MissingBinding.PackageSourceStates) != 0 {
		t.Fatalf("same-locator snippet without exact evidence binding became clickable: %#v", got.MissingBinding)
	}
	for language, current := range map[string]result{"en": got.EtcdEN, "ru": got.EtcdRU} {
		if current.Units != 215 || current.TopologyCards != 32 || current.PackageRows != 183 ||
			current.PackageActions != 2 || len(current.PackageUnavailable) != 181 ||
			current.UnitAuthorityBadges != 0 || current.Relations != 18 || current.SourceButtons != 18 {
			t.Fatalf("%s etcd shelf counts = %#v", language, current)
		}
		if len(current.PackagePrefixes) != 1 || current.PackagePrefixes[0] != "go.etcd.io/etcd/" ||
			len(current.PackageNames) != 183 ||
			len(current.PackageActionTargets) != 2 ||
			current.PackageActionTargets[0].Tag != "a" ||
			!strings.Contains(current.PackageActionTargets[0].Href, "github.com/example/fixture/blob") ||
			!strings.Contains(current.PackageActionTargets[0].Href, "cmd/server/startup.go") ||
			current.PackageActionTargets[1].Tag != "a" ||
			!strings.Contains(current.PackageActionTargets[1].Href, "pkg/one/package.go") {
			t.Fatalf("%s package prefix/source = %#v", language, current)
		}
		if len(current.PackageSourceSummaries) != 1 {
			t.Fatalf("%s package source summary repetition = %#v", language, current.PackageSourceSummaries)
		}
		if current.PackageDisclosureOpen {
			t.Fatalf("%s package disclosure is initially open", language)
		}
		if current.RelationPosition < 0 || current.UnitsPosition < 0 || current.PackagePosition < 0 ||
			current.CompactStatusCount != 0 || current.UnitsPosition >= current.PackagePosition ||
			current.PackagePosition >= current.RelationPosition {
			t.Fatalf("%s shelf order relation=%d units=%d packages=%d", language, current.RelationPosition, current.UnitsPosition, current.PackagePosition)
		}
	}
	if got.EtcdEN.PackageSummary != "Packages (183)" {
		t.Fatalf("English package disclosure = %q", got.EtcdEN.PackageSummary)
	}
	if got.EtcdRU.PackageSummary != "Пакеты (183)" {
		t.Fatalf("Russian package disclosure = %q", got.EtcdRU.PackageSummary)
	}
	if got.Conflict.PackageActions != 0 || len(got.Conflict.PackageUnavailable) != 1 ||
		got.Conflict.PackageUnavailable[0] != "conflict" || len(got.Conflict.PackageSourceStates) != 0 ||
		len(got.Conflict.PackageSourceSummaries) != 1 {
		t.Fatalf("conflicting package evidence became clickable: %#v", got.Conflict)
	}
	if got.MultipleDeclarations.PackageActions != 0 || len(got.MultipleDeclarations.PackageUnavailable) != 1 ||
		got.MultipleDeclarations.PackageUnavailable[0] != "conflict" ||
		len(got.MultipleDeclarations.PackageSourceStates) != 0 {
		t.Fatalf("multiple package declarations chose a source: %#v", got.MultipleDeclarations)
	}
	if len(got.MixedPrefix.PackagePrefixes) != 0 || len(got.MixedPrefix.PackageNames) != 183 ||
		got.MixedPrefix.PackageNames[0] != "go.etcd.io/etcd" ||
		got.MixedPrefix.PackageNames[182] != "example.net/outside" {
		t.Fatalf("non-exact module prefix was factored: %#v", got.MixedPrefix)
	}
	if got.StrippedStatic.PackageActions != 1 || len(got.StrippedStatic.PackageActionTargets) != 1 ||
		got.StrippedStatic.PackageActionTargets[0].Tag != "a" ||
		got.StrippedStatic.PackageActionTargets[0].HasClick ||
		!strings.Contains(got.StrippedStatic.PackageActionTargets[0].Href, "/blob/") ||
		!strings.Contains(got.StrippedStatic.PackageActionTargets[0].Href, "internal/server/start.go#L10") {
		t.Fatalf("stripped-static package did not preserve exact pinned source action: %#v", got.StrippedStatic)
	}
	if got.PrototypeName.TopologyCards != 1 || len(got.PrototypeName.UnitTags) != 3 {
		t.Fatalf("prototype-like exact unit name broke coalescing: %#v", got.PrototypeName)
	}
	// P7-B: a package without an exact saved source opens its representative
	// file from the repository graph (first openable file, sorted order);
	// without a graph the row stays a plain unavailable reference.
	if got.Representative.PackageActions != 1 || len(got.Representative.PackageUnavailable) != 0 {
		t.Fatalf("P7-B representative package row did not become actionable: %#v", got.Representative)
	}
	if got.RepresentativeNoGraph.PackageActions != 0 || len(got.RepresentativeNoGraph.PackageUnavailable) != 1 {
		t.Fatalf("P7-B without graph must stay unavailable: %#v", got.RepresentativeNoGraph)
	}
}

func TestRepositoryAtlasWorkspaceAssetsStayNarrow(t *testing.T) {
	t.Parallel()
	markers := map[string]string{
		"script.js":      "repositoryAtlasWorkspaceShelf",
		"ui_messages.js": "main.atlas.workspace.title",
		"style.css":      "rm-atlas-shelf",
	}
	for name, marker := range markers {
		raw, err := os.ReadFile(filepath.Join("templates", name))
		if err != nil {
			t.Fatal(err)
		}
		asset := string(raw)
		if !strings.Contains(asset, marker) {
			t.Fatalf("%s does not contain the Atlas workspace shelf", name)
		}
	}
}
