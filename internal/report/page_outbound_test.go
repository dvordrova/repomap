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

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

func TestOutboundAddressExplainsConfigurationWithoutResolvingIt(t *testing.T) {
	for _, tc := range []struct {
		address, setting, label, suffix string
	}{
		{"{--proxy-endpoint}/hello", "--proxy-endpoint", "Address from command-line option", "/hello"},
		{"{env:거래소_URL}/가격", "거래소_URL", "Address from environment variable", "/가격"},
		{"{--trace-endpoint}", "--trace-endpoint", "Address from command-line option", ""},
		{"https://prices.example/{market}", "", "", ""},
		{"https://prices.example/{--literal}", "", "", ""},
		{"{--host}/{env:PATH}", "", "", ""},
		{"{--}", "", "", ""},
		{"", "", "", ""},
	} {
		t.Run(tc.address, func(t *testing.T) {
			got := outboundAddressText(tc.address)
			if got.Text != tc.address || got.Setting != tc.setting || got.SettingLabel != tc.label || got.Suffix != tc.suffix {
				t.Fatalf("address notation lost source spelling or acquired a value: %+v", got)
			}
		})
	}
}

func TestOutboundSourceUsesDoNotHideBehindOneSelectedAddress(t *testing.T) {
	index := groupindex.Index{Target: programindex.Target{ID: "service"}, Outbound: []groupindex.OutboundCall{{
		ID: "shared-send", Kind: "http_client", Source: "model", Address: "https://prices.example",
		Uses: []atlas.DestinationUse{
			{Address: "https://prices.example", Steps: []atlas.DestinationStep{{Name: "GetPrices", Path: "prices.go", Line: 12, Column: 3}}},
			{Address: "https://audit.example", Steps: []atlas.DestinationStep{{Name: "WriteAudit", Path: "audit.go", Line: 22, Column: 3}}},
		},
	}}}
	section := &pageSection{ID: "service", programTargetID: "service"}
	builder := pageBuilder{data: &ReportData{}, indexes: []groupindex.Index{index}}
	builder.fillSectionOutbound(section)
	row := section.Outbound[0]
	if row.Address != "" || row.DestinationCount != 2 || len(row.Uses) != 2 || row.Uses[1].Steps[0].Name != "WriteAudit" {
		t.Fatalf("shared helper lost a distinct source use: %+v", row)
	}
	parsed, err := template.New("report").Funcs(pageTemplateFuncs(Russian)).ParseFS(reportTemplateFS, "templates/html/*.html")
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := parsed.ExecuteTemplate(&out, "outbound-row", row); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"https://prices.example", "https://audit.example", "GetPrices", "WriteAudit", "prices.go:12", "audit.go:22"} {
		if !strings.Contains(out.String(), text) {
			t.Fatalf("destination disclosure lost %q", text)
		}
	}
}

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
		"Reads the latest market prices.": "Получает свежие рыночные цены.",
		"Configures trace export.":        "Настраивает экспорт трассировок.",
	}
	translations := DisplayTranslations{Version: DisplayTextVersion, Language: Russian, CatalogSHA256: page.catalog.SHA256}
	for _, entry := range page.catalog.Entries {
		translated, ok := wanted[entry.Text]
		if !ok || entry.Role != "label" && entry.Role != "summary" {
			t.Fatalf("native address, callable, basis or unrelated text entered translation: %+v", entry)
		}
		translations.Entries = append(translations.Entries, DisplayTranslationEntry{Ref: entry.Ref, Text: translated})
	}
	if len(page.catalog.Entries) != 2 {
		t.Fatalf("shared destination and purpose did not retain their display bindings: %+v", page.catalog)
	}
	if err := page.applyDisplay(RenderOptions{Language: Russian, Translations: &translations}); err != nil {
		t.Fatal(err)
	}
	if section.Outbound[0].Summary != "Получает свежие рыночные цены." || section.Outbound[0].Address != address || section.Outbound[0].External != "가격조회.Get" {
		t.Fatalf("translation changed source values or lost destination prose: %+v", section.Outbound[0])
	}
	parsed, err := template.New("report").Funcs(pageTemplateFuncs(Russian)).ParseFS(reportTemplateFS, "templates/html/*.html")
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := parsed.ExecuteTemplate(&out, "target.html", section); err != nil {
		t.Fatal(err)
	}
	html := stdhtml.UnescapeString(out.String())
	for _, text := range []string{`data-integration-count="3"`, "Куда обращается сервис", "Получает свежие рыночные цены.", "Адрес не определён", "Настройка взаимодействия", "Вызов взаимодействия", `data-open="client.go:21:17"`, "가격조회.Get", address, "Записи · 5", "Записи · 1", "GET /prices"} {
		if !strings.Contains(html, text) {
			t.Fatalf("first-screen inventory lost %q", text)
		}
	}
	if strings.Index(html, "Куда обращается сервис") > strings.Index(html, `class="component-parts"`) {
		t.Fatal("outbound inventory moved behind the map")
	}
	// Seven records name three destinations: one group per destination on
	// the page, every record beneath its group, no second disclosure needed.
	if strings.Count(html, "data-integration-item") != 3 || strings.Count(html, "data-integration-record") != 7 || strings.Contains(html, `<details class="input-more">`) {
		t.Fatal("destination groups lost or duplicated accepted communication records")
	}
	if first := strings.Index(html, `Pricing service <span class="meta">· 5</span>`); first < 0 || first > strings.Index(html, "Trace collector") {
		t.Fatal("the destination with the most records is not the first group")
	}
	if !strings.Contains(html, `data-display-ref="`+section.Outbound[0].SummaryRef+`"`) {
		t.Fatal("rendered purpose lost its exact display binding")
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

func TestOutboundGroupsByDestinationWithSharedAddressAndLead(t *testing.T) {
	rows := []pageOutbound{
		{ID: "a", Destination: "Kubernetes API server", Summary: "Lists pods in the namespace. Then filters them.", KindLabel: "SDK", Basis: "dispatch", Source: "model", Address: "{env:KUBECONFIG}"},
		{ID: "b", Destination: "Postgres", Summary: "Stores events.", KindLabel: "Database", Basis: "dispatch", Source: "model", Address: "{env:DATABASE_URL}"},
		{ID: "c", Destination: "kubernetes api server", Summary: "Watches deployments.", KindLabel: "SDK", Basis: "configuration", Source: "model", Address: "{env:KUBECONFIG}"},
		{ID: "d", Destination: "Postgres", Summary: "Reads events.", KindLabel: "Database", Basis: "dispatch", Source: "model", Address: "{env:REPLICA_URL}"},
		{ID: "e", Destination: "Postgres", Summary: "Deletes events.", KindLabel: "Database", Basis: "dispatch", Source: "model"},
		{ID: "f", NativeLabel: "GET https://metrics.example/push", KindLabel: "HTTP", Source: "fact"},
	}
	groups := groupOutbound(rows)
	if len(groups) != 3 || groups[0].Destination != "Postgres" || len(groups[0].Rows) != 3 || groups[1].Destination != "Kubernetes API server" || len(groups[1].Rows) != 2 || groups[2].NativeLabel != "GET https://metrics.example/push" {
		t.Fatalf("groups by destination, most records first: %+v", groups)
	}
	if groups[0].Addresses != 2 || groups[0].Address != "" || groups[1].Addresses != 1 || groups[1].Address != "{env:KUBECONFIG}" {
		t.Fatalf("shared address not aggregated: %+v", groups[:2])
	}
	if groups[1].Basis != "" || groups[0].Basis != "dispatch" || groups[1].KindLabel != "SDK" {
		t.Fatalf("mixed basis or kind not neutralised: %+v", groups[:2])
	}
	if groups[1].Lead() != "" || groups[2].Lead() != "" {
		t.Fatalf("a group of several records spoke with one record's sentence: %q / %q", groups[1].Lead(), groups[2].Lead())
	}
	if single := groupOutbound(rows[:1]); len(single) != 1 || single[0].Lead() != "Lists pods in the namespace." {
		t.Fatalf("a single record lost its lead sentence: %+v", single)
	}
	if groups[0].Rows[0].ID != "b" || groups[0].Rows[2].ID != "e" {
		t.Fatalf("record order inside a group changed: %+v", groups[0].Rows)
	}
}
