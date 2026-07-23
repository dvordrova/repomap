package report

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSemanticSearchAssetContract(t *testing.T) {
	t.Parallel()

	for _, token := range []string{
		"global.RepomapSemanticSearch",
		"rankSemanticSearchItems",
		"nextSearchIndex",
		"event.metaKey || event.ctrlKey",
		`event.key === "ArrowDown"`,
		`event.key === "Enter"`,
		`event.key === "Escape"`,
		`setAttribute("role", "combobox")`,
		`setAttribute("role", "listbox")`,
		"this.backdrop.tabIndex = -1",
		"this.resultHost.tabIndex = -1",
		`this.input.removeAttribute("aria-activedescendant")`,
		`global.document.activeElement === this.input ? this.closeButton : this.input`,
		`this.listen(button, "click", () => this.activateItem(item))`,
		"if (text(item && item.question)) return text(item.question)",
		`case "semantic_artifact": return "Open explanation →"`,
		`case "paved_path": return "Open instructions →"`,
		`paved_path: "How to run and verify"`,
		`return "Ready explanations"`,
		`repository_story: 225`,
		`this.options.targetAvailable`,
	} {
		if !strings.Contains(semanticSearchJS, token) {
			t.Errorf("semantic search JS missing %q", token)
		}
	}
	for _, forbidden := range []string{"fetch(", "innerHTML", "XMLHttpRequest", "WebSocket"} {
		if strings.Contains(semanticSearchJS, forbidden) {
			t.Errorf("semantic search JS contains forbidden runtime dependency %q", forbidden)
		}
	}
	for _, forbiddenCopy := range []string{
		"ни один объект не покрывает весь вопрос",
		"вопрос покрыт не полностью",
		"Известные пробелы",
		"Частичное совпадение",
		"Поиск не сочиняет ответ",
		"отсутствующих данных",
		"В ограниченном поисковом индексе",
		"Индекс был ограничен по размеру",
	} {
		if strings.Contains(semanticSearchJS, forbiddenCopy) {
			t.Errorf("default semantic search copy exposes internal coverage state %q", forbiddenCopy)
		}
	}
	for _, token := range []string{
		`id="rm-semantic-search"`,
		`placeholder="What do you want to understand?"`,
		`<kbd>⌘/Ctrl K</kbd>`,
		`id="rm-semantic-search-js"`,
	} {
		if !strings.Contains(templateHTML, token) {
			t.Errorf("report template missing %q", token)
		}
	}
	for _, token := range []string{
		".rm-semantic-search__entry",
		".rm-search-modal__dialog",
		".rm-search-modal__result.is-active",
		"@media (max-width: 640px)",
	} {
		if !strings.Contains(semanticSearchCSS, token) {
			t.Errorf("semantic search CSS missing %q", token)
		}
	}
}

func TestSemanticSearchRankingNaturalLanguageAndAbstention(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is unavailable")
	}
	assetPath, err := filepath.Abs(filepath.Join("templates", "semantic_search.js"))
	if err != nil {
		t.Fatal(err)
	}
	runner := `
const fs = require("fs");
const vm = require("vm");
const window = { __REPOMAP_SEARCH_TEST__: {} };
vm.runInNewContext(fs.readFileSync(process.argv[2], "utf8"), { window });
const api = window.__REPOMAP_SEARCH_TEST__;
const target = { kind: "map" };
const items = [
  { id: "direction:report", kind: "direction", title: "CLI orientation command flow", summary: "The core workflow produces the orientation report.", aliases: ["internal/report/report.go"], target },
  { id: "component:llm", kind: "component", title: "LLM Bundle", summary: "LLM integration and DeepSeek client", aliases: ["github.com/example/internal/deepseek"], target: { kind: "component", component_id: "llm" } },
  { id: "member:deepseek", kind: "member", title: "github.com/example/internal/deepseek", summary: "LLM Bundle", aliases: ["package"], target: { kind: "component", component_id: "llm" } },
  { id: "component:probe", kind: "component", title: "Component Probe", summary: "Component probing and validation", aliases: ["internal/componentprobe"], target: { kind: "component", component_id: "probe" } },
  { id: "concept:surface", kind: "domain_term", title: "surface discovery", summary: "Static analysis of runtime registrations and workers", aliases: ["runtime entries"], target },
  { id: "map", kind: "map", title: "Architecture", summary: "Saved component landscape", aliases: ["main components", "главные компоненты"], target },
  { id: "component:analyzer", kind: "component", title: "Analyzer Core", summary: "Shared analyzer infrastructure", aliases: ["internal/analyzer"], target: { kind: "component", component_id: "analyzer" } },
  { id: "component:go", kind: "component", title: "Go Analyzer", summary: "Go language analysis using gopls", aliases: ["internal/analyzer/golang/gopls"], target: { kind: "component", component_id: "go" } }
  ,{ id: "artifact:reducer", kind: "mechanism", title: "Evidence reducer", question: "Зачем нужен локальный редуктор?", summary: "A saved evidence-backed explanation.", aliases: [], target: { kind: "semantic_artifact", artifact_id: "reducer" } }
  ,{ id: "artifact:listing", kind: "mechanism", title: "How Caddy builds directory listings", question: "How does the file server generate and sort directory listings?", summary: "Directory listing covers collection, request options, sorting and paging, representation selection, and response output.", aliases: ["как Caddy строит список файлов", "how Caddy builds a file listing", "how does Caddy build directory listings"], target: { kind: "semantic_artifact", artifact_id: "listing" } }
  ,{ id: "location:browse", kind: "location", title: "modules/caddyhttp/fileserver/browse.go", summary: "Exact source location", aliases: ["modules/caddyhttp/fileserver/browse.go"], target: { kind: "location", location: { path: "modules/caddyhttp/fileserver/browse.go", line: 78 } } }
];
const queries = [
  "как строится отчет",
  "deepseek",
  "как проверяется ответ модели",
  "runtime surfaces",
  "кеширование",
  "какие тут главные компоненты",
  "как добавить новый анализ",
  "как здесь используется go/packages",
  "зачем нужен локальный редуктор",
  "как Caddy строит список файлов",
  "how Caddy builds a file listing",
  "directory listing",
  "how Caddy lists files",
  "how sorting and paging work",
  "как сортируются файлы в Caddy",
  "modules/caddyhttp/fileserver/browse.go"
];
const result = {};
queries.forEach((query) => {
  const ranked = api.rankSemanticSearchItems(items, query, 10);
  result[query] = {
    ids: ranked.map((entry) => entry.item.id),
    complete: ranked.map((entry) => entry.complete),
    exact: ranked.map((entry) => entry.exact),
  };
});
result.technical = api.rankSemanticSearchItems(items, "github.com/example/internal/deepseek", 3).map((entry) => ({ id: entry.item.id, exact: entry.exact }));
result.tokens = api.tokenize("go/packages packages.Load кеширование");
result.navigation = [api.nextSearchIndex(0, -1, 3), api.nextSearchIndex(2, 1, 3), api.nextSearchIndex(0, 1, 0)];
process.stdout.write(JSON.stringify(result));
`
	runnerPath := filepath.Join(t.TempDir(), "semantic-search-test.js")
	if err := os.WriteFile(runnerPath, []byte(runner), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(node, runnerPath, assetPath).CombinedOutput()
	if err != nil {
		t.Fatalf("run semantic search scorer: %v\n%s", err, output)
	}
	result := make(map[string]struct {
		IDs      []string `json:"ids"`
		Complete []bool   `json:"complete"`
		Exact    []bool   `json:"exact"`
	})
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(output, &raw); err != nil {
		t.Fatalf("decode semantic search scorer: %v\n%s", err, output)
	}
	for key, value := range raw {
		var entry struct {
			IDs      []string `json:"ids"`
			Complete []bool   `json:"complete"`
			Exact    []bool   `json:"exact"`
		}
		if json.Unmarshal(value, &entry) == nil && entry.IDs != nil {
			result[key] = entry
		}
	}
	assertTop := func(query, want string, complete bool) {
		t.Helper()
		got := result[query]
		if len(got.IDs) == 0 || got.IDs[0] != want {
			t.Fatalf("%q results = %v, want %q first", query, got.IDs, want)
		}
		if len(got.Complete) == 0 || got.Complete[0] != complete {
			t.Fatalf("%q complete = %v, want first %t", query, got.Complete, complete)
		}
	}
	assertTop("как строится отчет", "direction:report", true)
	assertTop("deepseek", "component:llm", true)
	assertTop("runtime surfaces", "concept:surface", true)
	assertTop("какие тут главные компоненты", "map", true)
	assertTop("как добавить новый анализ", "component:analyzer", false)
	assertTop("зачем нужен локальный редуктор", "artifact:reducer", true)
	assertTop("как Caddy строит список файлов", "artifact:listing", true)
	assertTop("how Caddy builds a file listing", "artifact:listing", true)
	assertTop("directory listing", "artifact:listing", true)
	assertTop("how Caddy lists files", "artifact:listing", false)
	assertTop("how sorting and paging work", "artifact:listing", true)
	assertTop("как сортируются файлы в Caddy", "artifact:listing", false)
	assertTop("modules/caddyhttp/fileserver/browse.go", "location:browse", true)

	compound := result["как проверяется ответ модели"]
	if len(compound.IDs) < 2 {
		t.Fatalf("compound model-validation results = %v, want both partial artifacts", compound.IDs)
	}
	for _, complete := range compound.Complete {
		if complete {
			t.Fatalf("compound model-validation incorrectly marked complete: %#v", compound)
		}
	}
	for _, query := range []string{"кеширование", "как здесь используется go/packages"} {
		if got := result[query].IDs; len(got) != 0 {
			t.Fatalf("%q results = %v, want honest no-result", query, got)
		}
	}

	var technical []struct {
		ID    string `json:"id"`
		Exact bool   `json:"exact"`
	}
	if err := json.Unmarshal(raw["technical"], &technical); err != nil {
		t.Fatal(err)
	}
	if len(technical) == 0 || technical[0].ID != "member:deepseek" || !technical[0].Exact {
		t.Fatalf("technical exact match = %#v", technical)
	}
	if len(technical) < 2 || technical[1].ID != "component:llm" || !technical[1].Exact {
		t.Fatalf("technical semantic fallback = %#v", technical)
	}
	var tokens []string
	if err := json.Unmarshal(raw["tokens"], &tokens); err != nil {
		t.Fatal(err)
	}
	if strings.Join(tokens, ",") != "go_packages,packages_load,cache" {
		t.Fatalf("protected tokens = %v", tokens)
	}
	var navigation []int
	if err := json.Unmarshal(raw["navigation"], &navigation); err != nil {
		t.Fatal(err)
	}
	if len(navigation) != 3 || navigation[0] != 2 || navigation[1] != 0 || navigation[2] != -1 {
		t.Fatalf("navigation = %v", navigation)
	}
}
