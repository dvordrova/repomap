package report

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	stdhtml "html"
	"html/template"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

func TestOutboundCatalogueRetainsCommunicationWithoutDependencyGroups(t *testing.T) {
	const address = "https://거래소.example/가격/%ED%95%9C?시장=KRW&limit=10"
	index := groupindex.Index{Target: programindex.Target{ID: "service"},
		Groups:     []groupindex.Group{{ID: "handler", Title: "API", Lane: groupindex.LaneTriggers}},
		Operations: []groupindex.Operation{{ID: "serve", GroupID: "handler", Name: "GET /prices", Kind: "request", Source: "model", Location: programindex.Location{Path: "server.go", Line: 8, Column: 3}}},
	}
	for i := 0; i < 7; i++ {
		index.Outbound = append(index.Outbound, groupindex.OutboundCall{
			ID: fmt.Sprintf("out-%d", i), GroupID: "handler", Kind: "http_client",
			Destination: "Pricing service", Summary: "Reads the latest market prices.",
			Address: address, External: "가격조회.Get", Basis: "dispatch", Source: "model",
			Location: programindex.Location{Path: "client.go", Line: 21 + i, Column: 17},
		})
	}
	index.Outbound[1].Destination, index.Outbound[1].Summary = "Trace collector", "Configures trace export."
	index.Outbound[1].Address, index.Outbound[1].External, index.Outbound[1].Basis = "", "otlptracehttp.New", "configuration"
	index.Outbound[6].Destination, index.Outbound[6].Summary, index.Outbound[6].Method, index.Outbound[6].Source = "", "", "GET", "fact"
	section := &pageSection{ID: "service-page", programTargetID: "service", FactsAvailable: true, Map: &pageMap{}}
	builder := pageBuilder{data: &ReportData{}, indexes: []groupindex.Index{index}, links: pageLinks{sourceIDs: map[string]string{"server.go": "s1", "client.go": "s2"}}}
	builder.fillSectionOperations(section)
	builder.fillSectionOutbound(section)
	section.InboundCount, section.InputsCount = len(section.Requests), len(section.Requests)
	if len(section.DependencyGroups) != 0 || len(section.Outbound) != 7 || section.InputsCount != 1 {
		t.Fatalf("mixed component lost its independent outbound inventory: %+v", section)
	}
	if section.Outbound[0].Anchor.Open != "client.go:21:17" || section.Outbound[1].Address != "" || section.Outbound[6].NativeLabel != "GET "+address {
		t.Fatalf("communication source, unknown address or native HTTP syntax was changed: %+v", section.Outbound)
	}
	page := &PreparedPage{view: &pageView{Sections: []*pageSection{section}}, catalog: DisplayTextCatalog{Version: DisplayTextVersion, Entries: []DisplayTextEntry{}}}
	if err := page.collectDisplayTexts(&ReportData{}, false); err != nil {
		t.Fatal(err)
	}
	page.catalog.SHA256 = displayCatalogDigest(page.catalog.Entries)
	wanted := map[string]string{
		"Pricing service": "Сервис котировок", "Reads the latest market prices.": "Получает свежие рыночные цены.",
		"Trace collector": "Приёмник трассировок", "Configures trace export.": "Настраивает экспорт трассировок.",
	}
	translations := DisplayTranslations{Version: DisplayTextVersion, Language: Russian, CatalogSHA256: page.catalog.SHA256}
	for _, entry := range page.catalog.Entries {
		translated, ok := wanted[entry.Text]
		if !ok || entry.Role != "label" && entry.Role != "summary" {
			t.Fatalf("native address, callable, basis or unrelated text entered translation: %+v", entry)
		}
		translations.Entries = append(translations.Entries, DisplayTranslationEntry{Ref: entry.Ref, Text: translated})
	}
	if len(page.catalog.Entries) != 4 {
		t.Fatalf("shared destination and purpose did not retain their display bindings: %+v", page.catalog)
	}
	if err := page.applyDisplay(RenderOptions{Language: Russian, Translations: &translations}); err != nil {
		t.Fatal(err)
	}
	if section.Outbound[0].Destination != "Сервис котировок" || section.Outbound[0].Address != address || section.Outbound[0].External != "가격조회.Get" {
		t.Fatalf("translation changed source values or lost destination prose: %+v", section.Outbound[0])
	}
	parsed, err := template.New("report").Funcs(template.FuncMap{"t": func(key string, args ...any) (string, error) { return uiText(Russian, key, args...) }}).ParseFS(reportTemplateFS, "templates/html/*.html")
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := parsed.ExecuteTemplate(&out, "target.html", section); err != nil {
		t.Fatal(err)
	}
	html := stdhtml.UnescapeString(out.String())
	for _, text := range []string{`data-integration-count="7"`, "Куда обращается сервис", "Получает свежие рыночные цены.", "Адрес не определён", "Настройка взаимодействия", "Вызов взаимодействия", `data-open="client.go:21:17"`, "가격조회.Get", address, "все 7 →", "GET /prices"} {
		if !strings.Contains(html, text) {
			t.Fatalf("first-screen inventory lost %q", text)
		}
	}
	if strings.Index(html, "Куда обращается сервис") > strings.Index(html, `class="component-parts"`) {
		t.Fatal("outbound inventory moved behind the map")
	}
	preview, rest, found := strings.Cut(html, `<details class="input-more">`)
	if !found || strings.Count(preview, "data-integration-item") != 5 || strings.Count(rest, "data-integration-item") != 2 {
		t.Fatal("first five/full disclosure lost or duplicated accepted communication records")
	}
	if !strings.Contains(html, `data-display-ref="`+section.Outbound[0].DestinationRef+`"`) || !strings.Contains(html, `data-display-ref="`+section.Outbound[0].SummaryRef+`"`) {
		t.Fatal("rendered destination or purpose lost its exact display binding")
	}
	if slices.Contains(sectionCoverage(section), "External communication") {
		t.Fatal("outbound calls required a dependency lane to count as observed")
	}
	section.Outbound = nil
	section.DependencyGroups = []pageGroup{{Title: "Standard library"}}
	section.Dependencies = []pageDependency{{Name: "time"}}
	if !slices.Contains(sectionCoverage(section), "External communication") {
		t.Fatal("a package dependency concealed the absence of communication observations")
	}
}

func TestOutboundSourcePathsAndFoldKeepOriginalEvidence(t *testing.T) {
	index := reportGroupIndexFixture(t, "api", "fixture:api", "main.go")
	index.Outbound = []groupindex.OutboundCall{{ID: "out", Kind: "http_client", External: "http.Client.Do", Method: "GET", Address: "https://가격.example/시장", Source: "fact", Basis: "dispatch", Location: programindex.Location{Path: "clients/가격.go", Line: 31, Column: 19}}}
	index.SHA256 = ""
	raw, err := json.Marshal(index)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	index.SHA256 = hex.EncodeToString(digest[:])
	view, err := NewGroupGraphView([]groupindex.Index{index}, index.Target.ID)
	if err != nil {
		t.Fatal(err)
	}
	paths, err := view.SourcePaths()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(paths, "clients/가격.go") {
		t.Fatalf("ungrouped communication source did not enter openable paths: %v", paths)
	}
	// Folding groups must not turn two observations into one or alter the
	// saved index when their groups share a display title.
	index = groupindex.Index{Target: programindex.Target{ID: "t"}, Groups: []groupindex.Group{
		{ID: "large", Title: "Requests", MemberSubjectIDs: []string{"a", "b"}},
		{ID: "small", Title: "Requests", MemberSubjectIDs: []string{"c"}},
	}, Outbound: []groupindex.OutboundCall{{ID: "a", GroupID: "small"}, {ID: "b", GroupID: "large"}}}
	folded := foldIndexes([]groupindex.Index{index})[0]
	if len(folded.Outbound) != 2 || folded.Outbound[0].GroupID != "large" || index.Outbound[0].GroupID != "small" {
		t.Fatal("presentation group folding changed or lost communication evidence")
	}
}
