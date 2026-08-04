package report

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

func TestTypedUIMessageCatalogAcceptance(t *testing.T) {
	catalogPath := filepath.Join("templates", "ui_messages.js")
	raw, err := os.ReadFile(catalogPath)
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	enStart := strings.Index(source, "  var EN = {")
	ruStart := strings.Index(source, "  var RU = {")
	helpersStart := strings.Index(source, "  function englishCount(")
	if enStart < 0 || ruStart <= enStart || helpersStart <= ruStart {
		t.Fatal("typed UI catalog source boundaries are absent")
	}
	entryPattern := regexp.MustCompile(`(?m)^\s{4}"([^"]+)":\s*\{`)
	sourceKeys := func(locale string, section string) []string {
		t.Helper()
		matches := entryPattern.FindAllStringSubmatch(section, -1)
		keys := make([]string, 0, len(matches))
		seen := make(map[string]struct{}, len(matches))
		for _, match := range matches {
			key := match[1]
			if _, exists := seen[key]; exists {
				t.Fatalf("%s catalog source contains duplicate key %q", locale, key)
			}
			seen[key] = struct{}{}
			keys = append(keys, key)
		}
		slices.Sort(keys)
		return keys
	}
	enSourceKeys := sourceKeys("EN", source[enStart:ruStart])
	ruSourceKeys := sourceKeys("RU", source[ruStart:helpersStart])
	if len(enSourceKeys) == 0 || !slices.Equal(enSourceKeys, ruSourceKeys) {
		t.Fatalf("EN/RU source IDs differ: EN=%d RU=%d", len(enSourceKeys), len(ruSourceKeys))
	}

	used := map[string]string{}
	literalID := regexp.MustCompile(`["']((?:main|architecture|surfaces)\.[A-Za-z0-9_.]+)["']`)
	assetNames := []string{"script.js", "architecture_canvas.js", "surface_catalog.js", "report.html"}
	for _, name := range assetNames {
		asset, readErr := os.ReadFile(filepath.Join("templates", name))
		if readErr != nil {
			t.Fatal(readErr)
		}
		for _, match := range literalID.FindAllStringSubmatch(string(asset), -1) {
			used[match[1]] = name
		}
		for _, forbidden := range []string{"RU_UI", "translateUI", "TreeWalker", "MutationObserver"} {
			if strings.Contains(string(asset), forbidden) {
				t.Errorf("%s retains forbidden legacy translation machinery %q", name, forbidden)
			}
		}
	}
	scriptSource, err := os.ReadFile(filepath.Join("templates", "script.js"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(scriptSource), "UI.hasMessage(presentation.message_id)") {
		t.Error("typed task warning rendering does not resolve message membership through the catalog")
	}
	if strings.Contains(string(scriptSource), "body.error") {
		t.Error("browser actions must not surface raw server error prose")
	}
	for _, forbidden := range []string{
		"error.message",
		"flow.bundle_stats_label",
		"humanizeSymbolDetail(reason)",
		"boundedText(inspection.error",
		"boundedText(state.error",
		"boundedText(warning",
	} {
		if strings.Contains(string(scriptSource), forbidden) {
			t.Errorf("browser retains a direct unlocalized runtime-prose channel %q", forbidden)
		}
	}
	if !strings.Contains(string(scriptSource), `msg('main.flow.bundle_stats_compact'`) {
		t.Error("browser bundle stats do not render through the typed message catalog")
	}
	architectureSource, err := os.ReadFile(filepath.Join("templates", "architecture_canvas.js"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(architectureSource), "error.message") {
		t.Error("architecture canvas must not surface raw layout-library error prose")
	}
	for _, key := range enSourceKeys {
		if strings.HasPrefix(key, "main.task_lens.warning.") {
			used[key] = "typed task warning diagnostics"
		}
		if strings.HasPrefix(key, "main.browser_error.") {
			used[key] = "typed browser error presentation"
		}
	}
	for _, forbidden := range []string{
		`+ ' anchors'`,
		`' model calls'`,
		`+ ' of ' +`,
		`label += ' · registration'`,
		`label += ' · process entry'`,
		`status += ' · cached'`,
		`validated_model: 'validated model'`,
		`round.status || 'unknown'`,
	} {
		if strings.Contains(string(scriptSource), forbidden) {
			t.Errorf("script.js retains uncataloged dynamic product copy %q", forbidden)
		}
	}
	reportTemplate, err := os.ReadFile(filepath.Join("templates", "report.html"))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		`{{if eq .Language "ru"}}Localization`,
	} {
		if strings.Contains(string(reportTemplate), forbidden) {
			t.Errorf("report.html bypasses the typed product-copy catalog with %q", forbidden)
		}
	}
	for _, required := range []string{
		`<noscript>`,
		`{{if eq .Language "ru"}}Для этого отчёта нужен JavaScript.`,
		`This report requires JavaScript.`,
		`body > :not(noscript)`,
	} {
		if !strings.Contains(string(reportTemplate), required) {
			t.Errorf("report.html is missing the sole no-JS locale exception %q", required)
		}
	}
	catalogKeys := make(map[string]struct{}, len(enSourceKeys))
	var orphaned []string
	for _, key := range enSourceKeys {
		catalogKeys[key] = struct{}{}
		if _, ok := used[key]; !ok {
			orphaned = append(orphaned, key)
		}
	}
	var uncataloged []string
	for key, asset := range used {
		if _, ok := catalogKeys[key]; !ok {
			uncataloged = append(uncataloged, asset+":"+key)
		}
	}
	slices.Sort(orphaned)
	slices.Sort(uncataloged)
	if len(orphaned) > 0 {
		t.Errorf("catalog contains %d orphaned production IDs: %q", len(orphaned), orphaned)
	}
	if len(uncataloged) > 0 {
		t.Errorf("production assets contain %d uncataloged literal message IDs: %q", len(uncataloged), uncataloged)
	}

	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is unavailable")
	}
	absoluteCatalogPath, err := filepath.Abs(catalogPath)
	if err != nil {
		t.Fatal(err)
	}
	runner := `
const fs = require("fs");
const vm = require("vm");
const window = { document: { documentElement: { lang: "en" } } };
vm.runInNewContext(fs.readFileSync(process.argv[2], "utf8"), {
  window, Set, Object, String, Array, Error, Number,
});
const api = window.__REPOMAP_UI_MESSAGES_TEST__;
const en = api.catalogs.en;
const ru = api.catalogs.ru;
const enIDs = Object.keys(en).sort();
const ruIDs = Object.keys(ru).sort();
const same = (left, right) => JSON.stringify(left) === JSON.stringify(right);
const failure = (locale, id, params) => {
  try {
    api.messageForLocale(locale, id, params);
    return false;
  } catch (_) {
    return true;
  }
};
const unicode = {
  path: "путь/雪/😀.go",
  symbol: "Обработчик.方法😀",
  package: "пример.рф/пакет/雪",
};
const rendered = {
  path: api.messageForLocale("ru", "main.source.location_lines", {
    path: unicode.path, start: 7, end: 9,
  }),
  symbol: api.messageForLocale("ru", "main.source.inspect_exact", {
    label: unicode.symbol,
  }),
  package: api.messageForLocale("ru", "main.toast.copied", {
    value: unicode.package,
  }),
};
process.stdout.write(JSON.stringify({
  version: window.RepomapUI.version,
  membership: {
    known: window.RepomapUI.hasMessage("main.task_lens.warning.join_rejected"),
    unknown: window.RepomapUI.hasMessage("main.not_a_real_message"),
    nonString: window.RepomapUI.hasMessage(null),
  },
  idParity: same(enIDs, ruIDs),
  paramParity: enIDs.every((id) => same(en[id].params.slice().sort(), ru[id].params.slice().sort())),
  enCount: enIDs.length,
  ruCount: ruIDs.length,
  failures: {
    unknown: failure("en", "main.not_a_real_message"),
    missing: failure("en", "main.count.steps"),
    undefined: failure("en", "main.count.steps", { count: undefined }),
    extra: failure("en", "main.count.steps", { count: 1, extra: true }),
    unsupportedLocale: failure("fr", "main.overview"),
    emptyLocale: failure("", "main.overview"),
  },
  plurals: [1, 2, 5, 11, 21].map((count) =>
    api.messageForLocale("ru", "architecture.count.components", { count })
  ),
  unicode,
  rendered,
  identical: enIDs.filter((id) => {
    if (typeof en[id].text === "string") return en[id].text === ru[id].text;
    return en[id].format.toString() === ru[id].format.toString();
  }),
  russianCopy: {
    chrome: api.messageForLocale("ru", "main.chrome.what.to.notice"),
    workspace: api.messageForLocale("ru", "main.repository.workspace"),
    study: api.messageForLocale("ru", "main.study.incomplete_direction"),
    taskLens: api.messageForLocale("ru", "main.task_lens.working_hypothesis"),
    progress: api.messageForLocale("ru", "main.unit_progress", {
      unit: "Раздел", current: 2, total: 5,
    }),
    architecture: api.messageForLocale("ru", "architecture.action.open_code"),
    surfaces: api.messageForLocale("ru", "surfaces.action.reset_filters"),
    traceReadinessReason: api.messageForLocale("ru", "surfaces.field.trace_readiness_reason"),
    localizationStatus: api.messageForLocale("ru", "main.localization.status_label"),
    localizationActive: api.messageForLocale("ru", "main.localization.ru_active"),
    localizationFallback: api.messageForLocale("ru", "main.localization.ru_unavailable_canonical_en"),
  },
  opaqueTechnical: {
    route: api.messageForLocale("ru", "surfaces.identity.http_route", {
      method: "PATCH", path: "/雪/😀",
    }),
    location: api.messageForLocale("ru", "surfaces.location.open", {
      location: "путь/雪/😀.go:7",
    }),
    protocol: api.messageForLocale("ru", "surfaces.value.http"),
  },
  dynamic: {
    component: api.messageForLocale("ru", "main.component.role_anchor_count", {
      role: "Граница", count: 2,
    }),
    budget: api.messageForLocale("ru", "main.task.budget_summary", {
      files: 2, bytes: 5, calls: 11,
    }),
    registration: api.messageForLocale("ru", "main.surface.registration_suffix"),
    processEntry: api.messageForLocale("ru", "main.surface.process_entry_suffix"),
    cached: api.messageForLocale("ru", "main.research.cached"),
    unknown: api.messageForLocale("ru", "main.research.unknown_status"),
    bundleStats: api.messageForLocale("ru", "main.flow.bundle_stats_compact", {
      source: 21, test: 2, doc: 5,
    }),
    ranking: api.messageForLocale("ru", "main.symbol.ranked_by", { count: 3 }),
    layoutFailure: api.messageForLocale("ru", "architecture.error.layout_failed"),
  },
}));
`
	runnerPath := filepath.Join(t.TempDir(), "ui-message-catalog-test.js")
	if err := os.WriteFile(runnerPath, []byte(runner), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(node, runnerPath, absoluteCatalogPath).CombinedOutput()
	if err != nil {
		t.Fatalf("evaluate typed UI catalog: %v\n%s", err, output)
	}
	var got struct {
		Version    int `json:"version"`
		Membership struct {
			Known     bool `json:"known"`
			Unknown   bool `json:"unknown"`
			NonString bool `json:"nonString"`
		} `json:"membership"`
		IDParity    bool `json:"idParity"`
		ParamParity bool `json:"paramParity"`
		ENCount     int  `json:"enCount"`
		RUCount     int  `json:"ruCount"`
		Failures    struct {
			Unknown           bool `json:"unknown"`
			Missing           bool `json:"missing"`
			Undefined         bool `json:"undefined"`
			Extra             bool `json:"extra"`
			UnsupportedLocale bool `json:"unsupportedLocale"`
			EmptyLocale       bool `json:"emptyLocale"`
		} `json:"failures"`
		Plurals []string `json:"plurals"`
		Unicode struct {
			Path    string `json:"path"`
			Symbol  string `json:"symbol"`
			Package string `json:"package"`
		} `json:"unicode"`
		Rendered struct {
			Path    string `json:"path"`
			Symbol  string `json:"symbol"`
			Package string `json:"package"`
		} `json:"rendered"`
		Identical   []string `json:"identical"`
		RussianCopy struct {
			Chrome               string `json:"chrome"`
			Workspace            string `json:"workspace"`
			Study                string `json:"study"`
			TaskLens             string `json:"taskLens"`
			Progress             string `json:"progress"`
			Architecture         string `json:"architecture"`
			Surfaces             string `json:"surfaces"`
			TraceReadinessReason string `json:"traceReadinessReason"`
			LocalizationStatus   string `json:"localizationStatus"`
			LocalizationActive   string `json:"localizationActive"`
			LocalizationFallback string `json:"localizationFallback"`
		} `json:"russianCopy"`
		OpaqueTechnical struct {
			Route    string `json:"route"`
			Location string `json:"location"`
			Protocol string `json:"protocol"`
		} `json:"opaqueTechnical"`
		Dynamic struct {
			Component     string `json:"component"`
			Budget        string `json:"budget"`
			Registration  string `json:"registration"`
			ProcessEntry  string `json:"processEntry"`
			Cached        string `json:"cached"`
			Unknown       string `json:"unknown"`
			BundleStats   string `json:"bundleStats"`
			Ranking       string `json:"ranking"`
			LayoutFailure string `json:"layoutFailure"`
		} `json:"dynamic"`
	}
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode typed UI catalog acceptance result: %v\n%s", err, output)
	}
	if got.Version != 9 {
		t.Errorf("catalog version = %d, want 9", got.Version)
	}
	if !got.Membership.Known || got.Membership.Unknown || got.Membership.NonString {
		t.Errorf("catalog membership contract = %#v", got.Membership)
	}
	if !got.IDParity || !got.ParamParity || got.ENCount != len(enSourceKeys) || got.RUCount != len(ruSourceKeys) {
		t.Errorf("evaluated catalog parity = IDs %t params %t EN %d RU %d; source EN %d RU %d",
			got.IDParity, got.ParamParity, got.ENCount, got.RUCount, len(enSourceKeys), len(ruSourceKeys))
	}
	if !got.Failures.Unknown || !got.Failures.Missing || !got.Failures.Undefined ||
		!got.Failures.Extra || !got.Failures.UnsupportedLocale || !got.Failures.EmptyLocale {
		t.Errorf("catalog rejection contract = %#v", got.Failures)
	}
	wantIdentical := []string{
		"surfaces.identity.http_route",
		"surfaces.location.open",
		"surfaces.value.http",
	}
	if !slices.Equal(got.Identical, wantIdentical) {
		t.Errorf("EN/RU byte-identical renderers = %#v, want opaque-only allowlist %#v", got.Identical, wantIdentical)
	}
	wantPlurals := []string{"1 компонент", "2 компонента", "5 компонентов", "11 компонентов", "21 компонент"}
	if !slices.Equal(got.Plurals, wantPlurals) {
		t.Errorf("RU component plurals = %#v, want %#v", got.Plurals, wantPlurals)
	}
	for name, pair := range map[string][2]string{
		"path":    {got.Unicode.Path, got.Rendered.Path},
		"symbol":  {got.Unicode.Symbol, got.Rendered.Symbol},
		"package": {got.Unicode.Package, got.Rendered.Package},
	} {
		if !strings.Contains(pair[1], pair[0]) {
			t.Errorf("Unicode %s parameter was not byte-preserved: value %q rendered %q", name, pair[0], pair[1])
		}
	}
	wantRussianCopy := struct {
		Chrome               string
		Workspace            string
		Study                string
		TaskLens             string
		Progress             string
		Architecture         string
		Surfaces             string
		TraceReadinessReason string
		LocalizationStatus   string
		LocalizationActive   string
		LocalizationFallback string
	}{
		Chrome:               "На что обратить внимание",
		Workspace:            "Рабочее пространство репозитория",
		Study:                "Неполное направление изучения",
		TaskLens:             "Рабочая гипотеза",
		Progress:             "Раздел 2 из 5",
		Architecture:         "Открыть код",
		Surfaces:             "Сбросить фильтры",
		TraceReadinessReason: "Причина готовности трассировки",
		LocalizationStatus:   "Статус локализации",
		LocalizationActive:   "Русская локализация включена.",
		LocalizationFallback: "Русская локализация недоступна. Показан канонический отчёт на английском языке.",
	}
	if got.RussianCopy.Chrome != wantRussianCopy.Chrome ||
		got.RussianCopy.Workspace != wantRussianCopy.Workspace ||
		got.RussianCopy.Study != wantRussianCopy.Study ||
		got.RussianCopy.TaskLens != wantRussianCopy.TaskLens ||
		got.RussianCopy.Progress != wantRussianCopy.Progress ||
		got.RussianCopy.Architecture != wantRussianCopy.Architecture ||
		got.RussianCopy.Surfaces != wantRussianCopy.Surfaces ||
		got.RussianCopy.TraceReadinessReason != wantRussianCopy.TraceReadinessReason ||
		got.RussianCopy.LocalizationStatus != wantRussianCopy.LocalizationStatus ||
		got.RussianCopy.LocalizationActive != wantRussianCopy.LocalizationActive ||
		got.RussianCopy.LocalizationFallback != wantRussianCopy.LocalizationFallback {
		t.Errorf("representative RU product copy = %#v, want %#v", got.RussianCopy, wantRussianCopy)
	}
	wantOpaqueTechnical := struct {
		Route    string
		Location string
		Protocol string
	}{
		Route:    "PATCH /雪/😀",
		Location: "путь/雪/😀.go:7 ↗",
		Protocol: "HTTP",
	}
	if got.OpaqueTechnical.Route != wantOpaqueTechnical.Route ||
		got.OpaqueTechnical.Location != wantOpaqueTechnical.Location ||
		got.OpaqueTechnical.Protocol != wantOpaqueTechnical.Protocol {
		t.Errorf("opaque technical RU values = %#v, want %#v", got.OpaqueTechnical, wantOpaqueTechnical)
	}
	wantDynamic := struct {
		Component     string
		Budget        string
		Registration  string
		ProcessEntry  string
		Cached        string
		Unknown       string
		BundleStats   string
		Ranking       string
		LayoutFailure string
	}{
		Component:     "Граница · 2 опоры",
		Budget:        "2 файла · 5 байтов · 11 вызовов модели",
		Registration:  "регистрация",
		ProcessEntry:  "точка входа процесса",
		Cached:        "кэшировано",
		Unknown:       "неизвестно",
		BundleStats:   "21 файл исходников · 2 тестовых файла · 5 документов",
		Ranking:       "При ранжировании учтено 3 сигнала анализатора",
		LayoutFailure: "Не удалось построить компоновку архитектуры.",
	}
	if got.Dynamic.Component != wantDynamic.Component ||
		got.Dynamic.Budget != wantDynamic.Budget ||
		got.Dynamic.Registration != wantDynamic.Registration ||
		got.Dynamic.ProcessEntry != wantDynamic.ProcessEntry ||
		got.Dynamic.Cached != wantDynamic.Cached ||
		got.Dynamic.Unknown != wantDynamic.Unknown ||
		got.Dynamic.BundleStats != wantDynamic.BundleStats ||
		got.Dynamic.Ranking != wantDynamic.Ranking ||
		got.Dynamic.LayoutFailure != wantDynamic.LayoutFailure {
		t.Errorf("dynamic RU product copy = %#v, want %#v", got.Dynamic, wantDynamic)
	}
}
