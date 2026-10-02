package report

import (
	"bytes"
	"fmt"
	stdhtml "html"
	"html/template"
	"maps"
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

func TestOutboundInputsFollowCallerSubjectsNotTheirSharedPart(t *testing.T) {
	index := groupindex.Index{Target: programindex.Target{ID: "service"},
		Operations: []groupindex.Operation{{ID: "timer", SubjectID: "tick"}, {ID: "get", SubjectID: "read"}},
		StructuralEdges: []groupindex.StructuralEdge{
			{FromSubjectID: "tick", ToSubjectID: "send", Role: groupindex.EdgeRelationTarget, RelationKind: programindex.RelationCalls},
			{FromSubjectID: "send", ToSubjectID: "tick", Role: groupindex.EdgeRelationTarget, RelationKind: programindex.RelationCalls},
			{FromSubjectID: "read", ToSubjectID: "send", Role: groupindex.EdgeRelationTarget, RelationKind: programindex.RelationReads},
		},
		Outbound: []groupindex.OutboundCall{{ID: "queue", SubjectID: "send", GroupID: "shared"}, {ID: "unresolved", GroupID: "shared"}},
	}
	for _, id := range []string{"tick", "send", "read"} {
		index.Subjects = append(index.Subjects, groupindex.Subject{ID: id, Object: &groupindex.ObjectFacts{Name: id, Kind: programindex.ObjectFunction}})
	}
	groupindex.Derive(&index)
	section := &pageSection{ID: "service", programTargetID: "service"}
	builder := pageBuilder{data: &ReportData{}, indexes: []groupindex.Index{index}}
	builder.fillSectionOutbound(section)
	if section.Outbound[0].Operations != operationNodeID("service", "timer") || section.Outbound[1].Operations != "" {
		t.Fatalf("outbound gained a caller through membership or a non-execution edge: %+v", section.Outbound)
	}
}

// A destination chain says only what the record does not: a frontier that
// is the record's own callable, one step at its own line, named the call a
// second time ("Address passes through ccxt.Exchange.create_order"), and
// two steps of one declaration on one line read as one name (freqtrade's
// start_install_ui and its dl_url both read "install-ui").
func TestOutboundChainsSayNothingTheRecordSays(t *testing.T) {
	step := func(subject, name, path string, line, column int) atlas.DestinationStep {
		return atlas.DestinationStep{SubjectID: subject, Name: name, Path: path, Line: line, Column: column}
	}
	index := groupindex.Index{Target: programindex.Target{ID: "t1"},
		Operations: []groupindex.Operation{{ID: "install", SubjectID: "n134", Name: "install-ui"}},
		Outbound: []groupindex.OutboundCall{
			{ID: "b1543", Kind: "sdk", External: "ccxt.Exchange.create_order", Location: programindex.Location{Path: "exchange.py", Line: 1481, Column: 31},
				Uses: []atlas.DestinationUse{{Frontier: "ccxt.Exchange.create_order", Steps: []atlas.DestinationStep{step("n2627", "Exchange.create_order", "exchange.py", 1481, 31)}}}},
			{ID: "b76", Kind: "client_request", External: "requests.get", Location: programindex.Location{Path: "deploy_ui.py", Line: 42, Column: 21},
				Uses: []atlas.DestinationUse{{Frontier: "dl_url", Steps: []atlas.DestinationStep{step("n302", "download_and_install_ui", "deploy_ui.py", 42, 21),
					step("n134", "start_install_ui", "deploy_commands.py", 133, 9), step("n134", "dl_url", "deploy_commands.py", 133, 46)}}}},
		}}
	section := &pageSection{ID: "t1", programTargetID: "t1"}
	builder := pageBuilder{data: &ReportData{}, indexes: []groupindex.Index{index}}
	builder.fillSectionOutbound(section)
	if uses := section.Outbound[0].InformativeUses(); len(uses) != 0 {
		t.Fatalf("a chain naming the call itself stands under it: %+v", uses)
	}
	uses := section.Outbound[1].InformativeUses()
	if len(uses) != 1 {
		t.Fatalf("the address's own chain is gone: %+v", uses)
	}
	var names []string
	for _, step := range uses[0].Steps {
		names = append(names, step.Name)
	}
	if !slices.Equal(names, []string{"download_and_install_ui", "install-ui"}) {
		t.Fatalf("the chain reads %q", names)
	}
}

// A frontier that names nothing is no address step: freqtrade's
// getattr(ccxt, name)(config) reached its ccxt calls through a frontier
// "()", and 41 of its exchange tiles read "Address passes through ()". The
// chain prints its steps and no address line.
func TestOutboundFrontierNamingNothingPrintsNoAddress(t *testing.T) {
	index := groupindex.Index{Target: programindex.Target{ID: "t1"}, Outbound: []groupindex.OutboundCall{{
		ID: "b1554", Kind: "sdk", External: "ccxt.Exchange.fetch_funding_rates", Location: programindex.Location{Path: "binance.py", Line: 288, Column: 35},
		Uses: []atlas.DestinationUse{{Frontier: "()", Steps: []atlas.DestinationStep{
			{SubjectID: "s638", Name: "Binance.fetch_funding_rates", Path: "binance.py", Line: 288, Column: 35},
			{SubjectID: "s749", Name: "Exchange._init_ccxt", Path: "exchange.py", Line: 424, Column: 19},
		}}},
	}}}
	section := &pageSection{ID: "t1", programTargetID: "t1"}
	builder := pageBuilder{data: &ReportData{}, indexes: []groupindex.Index{index}}
	builder.fillSectionOutbound(section)
	row := section.Outbound[0]
	if uses := row.InformativeUses(); len(uses) != 1 || uses[0].FrontierName() != "" {
		t.Fatalf("the chain through an unnamed call is lost or names it: %+v", uses)
	}
	parsed, err := template.New("report").Funcs(pageTemplateFuncs(English)).ParseFS(reportTemplateFS, "templates/html/*.html")
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := parsed.ExecuteTemplate(&out, "outbound-row", row); err != nil {
		t.Fatal(err)
	}
	page := out.String()
	for _, absent := range []string{"passes through", "<code>()</code>", "not determined"} {
		if strings.Contains(page, absent) {
			t.Errorf("a frontier naming nothing printed %q:\n%s", absent, page)
		}
	}
	if !strings.Contains(page, "Exchange._init_ccxt") {
		t.Fatalf("the chain lost its steps:\n%s", page)
	}
}

func TestOutboundSourceUsesDoNotHideBehindOneSelectedAddress(t *testing.T) {
	index := groupindex.Index{Target: programindex.Target{ID: "service"}, Outbound: []groupindex.OutboundCall{{
		ID: "shared-send", Kind: "client_request", Source: "model", Address: "https://prices.example",
		Uses: []atlas.DestinationUse{
			{Address: "https://prices.example", Steps: []atlas.DestinationStep{{Name: "GetPrices", Path: "prices.go", Line: 12, Column: 3}}},
			{Address: "https://audit.example", Steps: []atlas.DestinationStep{{Name: "WriteAudit", Path: "audit.go", Line: 22, Column: 3}}},
		},
	}}}
	section := &pageSection{ID: "service", programTargetID: "service"}
	builder := pageBuilder{data: &ReportData{}, indexes: []groupindex.Index{index}}
	builder.fillSectionOutbound(section)
	row := section.Outbound[0]
	if row.Address != "https://prices.example" || len(row.Uses) != 2 || row.Uses[1].Steps[0].Name != "WriteAudit" {
		t.Fatalf("shared helper lost a distinct source use or its accepted address: %+v", row)
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
	// The accepted address is said once, as the address; each chain's value
	// is the argument's (Russian: Адрес once, Значение аргумента twice).
	if page := out.String(); strings.Count(page, russianUI["Address"]+":") != 1 || strings.Count(page, russianUI["Argument value"]+":") != 2 {
		t.Fatalf("the accepted address and the chains' values are not told apart:\n%s", page)
	}
}

// A walked value is the call's argument, never its address, until the
// boundary's decision accepts one: casdoor's oss Put at storage.go:171
// printed its object key "%s/%s" as "Address" eleven times after the
// decision said unknown, from the uses and from a destination group that
// promoted their one value. With one literal end or several, an unknown
// address prints no address; every chain and its steps stay.
func TestAWalkedValueIsNeverCalledAnAddress(t *testing.T) {
	key := atlas.DestinationUse{Address: "%s/%s", Steps: []atlas.DestinationStep{{Name: "uploadFile", Path: "object/storage.go", Line: 171, Column: 30}, {Name: "UrlJoin", Path: "util/path.go", Line: 61, Column: 9}}}
	other := atlas.DestinationUse{Address: "avatars/%s", Steps: []atlas.DestinationStep{{Name: "uploadFile", Path: "object/storage.go", Line: 171, Column: 30}, {Name: "refineObjectKey", Path: "object/storage.go", Line: 224, Column: 2}}}
	for name, uses := range map[string][]atlas.DestinationUse{"one": {key}, "several": {key, other}} {
		index := groupindex.Index{Target: programindex.Target{ID: "t1"}, Outbound: []groupindex.OutboundCall{{
			ID: "b3006", Kind: "sdk", Destination: "Object Storage", External: "oss.StorageInterface.Put", Location: programindex.Location{Path: "object/storage.go", Line: 171, Column: 30},
			Uses: uses,
		}}}
		section := &pageSection{ID: "t1", programTargetID: "t1"}
		builder := pageBuilder{data: &ReportData{}, indexes: []groupindex.Index{index}}
		builder.fillSectionOutbound(section)
		row := section.Outbound[0]
		if row.Address != "" || len(row.Uses) != len(uses) {
			t.Fatalf("%s: the row completed an address or lost a chain: %+v", name, row)
		}
		parsed, err := template.New("report").Funcs(pageTemplateFuncs(English)).ParseFS(reportTemplateFS, "templates/html/*.html")
		if err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		if err := parsed.ExecuteTemplate(&out, "outbound-group", groupOutbound(section.Outbound)[0]); err != nil {
			t.Fatal(err)
		}
		page := out.String()
		if strings.Contains(page, "Address") || strings.Count(page, "Argument value: <code>") != len(uses) {
			t.Fatalf("%s: a walked value was called an address:\n%s", name, page)
		}
		for _, text := range []string{"util/path.go:61", "uploadFile"} {
			if !strings.Contains(page, text) {
				t.Fatalf("%s: the chain lost %q:\n%s", name, text, page)
			}
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
			ID: fmt.Sprintf("out-%d", i), GroupID: "handler", Kind: "client_request",
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
	for _, text := range []string{`data-integration-count="3"`, "Куда обращается сервис", "Получает свежие рыночные цены.", "настройка клиента", "вызов в коде", `data-open="client.go:21:17"`, "가격조회.Get", address} {
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
	// All call lines are visible beneath their destination. Only the original
	// evidence of an individual call requires expansion.
	if strings.Count(html, `<details class="outbound-call">`) != 7 || strings.Contains(html, `<details class="outbound-more">`) || strings.Count(html, `<strong class="input-title">Pricing service`) != 1 {
		t.Fatal("destination records are not complete compact nested lines")
	}
	if first := strings.Index(html, `<strong class="input-title">Pricing service`); first < 0 || first > strings.Index(html, "Trace collector") {
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
	index, err := groupindex.WithOutbound(index, []groupindex.OutboundCall{{ID: "out", Kind: "client_request", External: "http.Client.Do", Method: "GET", Address: "https://가격.example/시장", Source: "fact", Basis: "dispatch", Location: programindex.Location{Path: "clients/가격.go", Line: 31, Column: 19}}})
	if err != nil {
		t.Fatal(err)
	}
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

func TestOutboundGroupsByDestinationAndPreview(t *testing.T) {
	rows := []pageOutbound{
		{ID: "a", Destination: "Kubernetes API server", Summary: "Lists pods in the namespace. Then filters them.", KindLabel: "SDK", Basis: "dispatch", Source: "model", Address: "{env:KUBECONFIG}"},
		{ID: "b", Destination: "Postgres", Summary: "Stores events.", KindLabel: "Database", Basis: "dispatch", Source: "model", Address: "{env:DATABASE_URL}"},
		{ID: "c", Destination: "kubernetes api server", Summary: "Watches deployments.", KindLabel: "SDK", Basis: "configuration", Source: "model", Address: "{env:KUBECONFIG}"},
		{ID: "d", Destination: "Postgres", Summary: "Reads events.", KindLabel: "Database", Basis: "dispatch", Source: "model", Address: "{env:REPLICA_URL}"},
		{ID: "e", Destination: "Postgres", Summary: "Deletes events.", KindLabel: "Database", Basis: "dispatch", Source: "model"},
		{ID: "f", NativeLabel: "GET https://metrics.example/push", KindLabel: "Request", Source: "fact"},
	}
	groups := groupOutbound(rows)
	if len(groups) != 3 || groups[0].Destination != "Postgres" || len(groups[0].Rows) != 3 || groups[1].Destination != "Kubernetes API server" || len(groups[1].Rows) != 2 || groups[2].NativeLabel != "GET https://metrics.example/push" {
		t.Fatalf("groups by destination, most records first: %+v", groups)
	}
	if groups[1].Basis != "" || groups[0].Basis != "dispatch" || groups[1].KindLabel != "SDK" {
		t.Fatalf("mixed basis or kind not neutralised: %+v", groups[:2])
	}
	if first, rest := groups[0].First(), groups[0].Rest(); len(first) != 3 || len(rest) != 0 {
		t.Fatalf("three records need no expansion: %d / %d", len(first), len(rest))
	}
	five := groupOutbound(append(slices.Clone(rows[:5]), pageOutbound{ID: "g", Destination: "Postgres"}, pageOutbound{ID: "h", Destination: "Postgres"}))
	if first, rest := five[0].First(), five[0].Rest(); len(first) != 3 || first[2].ID != "e" || len(rest) != 2 || rest[1].ID != "h" {
		t.Fatalf("records beyond three wait under one expansion in order: %+v / %+v", first, rest)
	}
	if rows[0].Brief() != "Lists pods in the namespace." || rows[5].Brief() != "" {
		t.Fatalf("a record's line is its first sentence, or nothing when it has no purpose: %q / %q", rows[0].Brief(), rows[5].Brief())
	}
	if groups[0].Rows[0].ID != "b" || groups[0].Rows[2].ID != "e" {
		t.Fatalf("record order inside a group changed: %+v", groups[0].Rows)
	}
}

func TestDisplayCallableDropsPlatformNotation(t *testing.T) {
	for name, want := range map[string]string{"platform:javascript.WebSocket": "WebSocket", "platform:python": "python", "websocket.Codec.Receive": "websocket.Codec.Receive", "": ""} {
		if got := displayCallable(name); got != want {
			t.Fatalf("displayCallable(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestOutboundGroupsNameOneSystemOnce(t *testing.T) {
	rows := []pageOutbound{
		{ID: "a", Destination: "RabbitMQ (morfeu.events exchange)", KindLabel: "Queue"},
		{ID: "b", Destination: "rabbitmq", KindLabel: "External communication"},
		{ID: "c", Destination: "RabbitMQ (queue topology)", KindLabel: "External communication"},
		{ID: "d", Destination: "message broker", KindLabel: "Queue"},
		{ID: "e", Destination: "Redis", KindLabel: "SDK"},
		{ID: "f", Destination: "Redis", KindLabel: "Database"},
		{ID: "g", Destination: "remote PostgreSQL database", KindLabel: "Database"},
		{ID: "h", Destination: "PostgreSQL", KindLabel: "Database"},
	}
	groups := groupOutbound(rows)
	got := map[string]int{}
	for _, group := range groups {
		got[group.Destination] = len(group.Rows)
	}
	// Another wording is another group: no text is folded onto a name it
	// merely contains.
	want := map[string]int{"RabbitMQ": 3, "message broker": 1, "Redis": 2, "remote PostgreSQL database": 1, "PostgreSQL": 1}
	if len(got) != len(want) {
		t.Fatalf("groups: %v", got)
	}
	for name, count := range want {
		if got[name] != count {
			t.Fatalf("group %q has %d rows, want %d (%v)", name, got[name], count, got)
		}
	}
	if groups[0].Destination != "RabbitMQ" || groups[0].KindLabel != "External communication" {
		t.Fatalf("mixed kinds under one system did not neutralise: %+v", groups[0])
	}
}

func TestOutboundLineDropsThePackageTheGroupImplies(t *testing.T) {
	for external, want := range map[string]string{
		"amqp091-go.Channel.Confirm": "Confirm", "amqp091-go.Connection.NotifyClose": "NotifyClose", "amqp091-go.Channel.ExchangeDeclare": "ExchangeDeclare",
		"pgxpool.Pool.Ping": "Pool.Ping", "v4.Migrate.Up": "Migrate.Up", "v4.New": "New", "v5.Tx.Commit": "Tx.Commit", "WebSocket": "WebSocket", "": "",
	} {
		if got := shortCallable(external, false); got != want {
			t.Fatalf("shortCallable(%q) = %q, want %q", external, got, want)
		}
	}
	native := pageOutbound{Method: "GET", Address: "https://api.example/v1", NativeLabel: "GET https://api.example/v1", External: "http.Client.Do"}
	if native.Line() != "GET https://api.example/v1" {
		t.Fatalf("a native HTTP fact lost its method and address: %q", native.Line())
	}
}

func TestOutboundLineFallsBackToTheCallingFunction(t *testing.T) {
	row := pageOutbound{Uses: []pageOutboundUse{{Steps: []pageOutboundStep{{Name: "Relay.publicarUm"}}}}}
	if row.Line() != "publicarUm" {
		t.Fatalf("an unresolved interface call did not take its caller's name: %q", row.Line())
	}
	if len(row.InformativeUses()) != 0 {
		t.Fatal("a one-step chain without address or frontier is not informative")
	}
	row.Uses = append(row.Uses, pageOutboundUse{Frontier: "v5.Connect", Steps: []pageOutboundStep{{Name: "main"}}}, pageOutboundUse{Value: "{env:PG_URL}"})
	if len(row.InformativeUses()) != 2 {
		t.Fatalf("chains with a frontier or an address were dropped: %+v", row.InformativeUses())
	}
}

func TestOutboundLinesKeepTheTypeWhenAMemberRepeatsAcrossTypes(t *testing.T) {
	rows := []pageOutbound{
		{ID: "a", Destination: "Kubernetes API server", External: "v1.PodInterface.Patch"},
		{ID: "b", Destination: "Kubernetes API server", External: "v1.DeploymentInterface.Patch"},
		{ID: "c", Destination: "Kubernetes API server", External: "v1.PodInterface.Evict"},
		{ID: "d", Destination: "RabbitMQ broker", External: "amqp091-go.Channel.ExchangeDeclare"},
		{ID: "e", Destination: "RabbitMQ broker", External: "amqp091-go.Channel.Close"},
		{ID: "f", Destination: "RabbitMQ broker", External: "amqp091-go.Connection.Close"},
	}
	groups := groupOutbound(rows)
	lines := map[string]string{}
	for _, group := range groups {
		for _, row := range group.Rows {
			lines[row.ID] = row.Line()
		}
	}
	want := map[string]string{"a": "PodInterface.Patch", "b": "DeploymentInterface.Patch", "c": "Evict", "d": "ExchangeDeclare", "e": "Channel.Close", "f": "Connection.Close"}
	for id, line := range want {
		if lines[id] != line {
			t.Fatalf("line for %s = %q, want %q (%v)", id, lines[id], line, lines)
		}
	}
}

func TestOutboundPeerBindingRequiresExactCallerAndCallLocation(t *testing.T) {
	location := programindex.Location{Path: "http.ts", Line: 12, Column: 8}
	otherSite := programindex.Location{Path: "http.ts", Line: 12, Column: 28}
	index := groupindex.Index{Target: programindex.Target{ID: "front"},
		Outbound: []groupindex.OutboundCall{{ID: "send", SubjectID: "client", Location: location}},
		Connections: []groupindex.Connection{
			{ID: "match", From: groupindex.Endpoint{TargetID: "front"}, SourceKind: "integration", FromSubjectID: "client", FromLocation: &location},
			{ID: "column", From: groupindex.Endpoint{TargetID: "front"}, SourceKind: "integration", FromSubjectID: "client", FromLocation: &otherSite},
			{ID: "subject", From: groupindex.Endpoint{TargetID: "front"}, SourceKind: "integration", FromSubjectID: "another", FromLocation: &location},
			{ID: "owner", From: groupindex.Endpoint{TargetID: "another"}, SourceKind: "integration", FromSubjectID: "client", FromLocation: &location},
			{ID: "native", From: groupindex.Endpoint{TargetID: "front"}, SourceKind: "native_calls", FromSubjectID: "client", FromLocation: &location},
		},
	}
	builder := pageBuilder{data: &ReportData{}, indexes: []groupindex.Index{index}}
	section := &pageSection{ID: "front", programTargetID: "front"}
	builder.fillSectionOutbound(section)
	if !slices.Equal(section.Outbound[0].Connections, []string{connectionKey("front", "match")}) {
		t.Fatalf("incorrect peer bindings: %+v", section.Outbound)
	}
}

// An outgoing fact the model did not explain has no summary: the given
// text restated the call. Its row still names something: its call, or else
// its kind.
func TestAnOutgoingFactWithoutALineIsNamedByItsCallOrItsKind(t *testing.T) {
	parsed, err := template.New("report").Funcs(pageTemplateFuncs(English)).ParseFS(reportTemplateFS, "templates/html/*.html")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		row  pageOutbound
		want string
	}{
		{pageOutbound{ID: "call", External: "net/http.Get", KindLabel: "Request", Source: "fact", Anchor: pageAnchor{Text: "client.go:9"}}, "Get"},
		{pageOutbound{ID: "kind", KindLabel: "Request", Source: "fact", Anchor: pageAnchor{Text: "client.go:9"}}, "Request"},
	} {
		var out bytes.Buffer
		if err := parsed.ExecuteTemplate(&out, "outbound-row", test.row); err != nil {
			t.Fatal(err)
		}
		_, summary, _ := strings.Cut(out.String(), "<summary>")
		summary, _, _ = strings.Cut(summary, "</summary>")
		if !strings.Contains(summary, test.want) || strings.Contains(out.String(), `class="outbound-note"`) {
			t.Fatalf("%s: %s", test.row.ID, out.String())
		}
	}
}

// A started program is one destination only with the very word its calls
// wrote: equal words are one literal, a word in another case is another,
// and a program's word is never folded onto a known system's name. The
// record keeps every word its call writes. A program no word names is an
// unknown about its one call, no destination: it is listed under "What is
// missing", labelled for what the reading knows (named at run time, or not
// established), and never grouped, framed or connected as an outside system
// (litestream's -exec launch had drawn a "Program not established" frame
// twice).
func TestOutboundProgramsAreOneDestinationOnlyByTheirWord(t *testing.T) {
	index := groupindex.Index{Target: programindex.Target{ID: "tool"}, Outbound: []groupindex.OutboundCall{
		{ID: "b1", Kind: atlas.BoundaryRunsProgram, External: "os/exec.CommandContext", Destination: "git", Values: []string{"git", "status"}, Source: "model"},
		{ID: "b2", Kind: atlas.BoundaryRunsProgram, External: "os/exec.CommandContext", Destination: "git", Values: []string{"git", "log", "-n"}, Source: "model"},
		{ID: "b3", Kind: atlas.BoundaryRunsProgram, External: "os/exec.Command", Destination: "Git", Values: []string{"Git"}, Source: "model"},
		{ID: "b4", Kind: atlas.BoundaryRunsProgram, External: "stdlib.h.system", Destination: "redis-server", Values: []string{"redis-server"}, Source: "model"},
		{ID: "b5", Kind: atlas.BoundaryRunsProgram, External: "os/exec.CommandContext", ProgramNotNamed: true, Values: []string{"--version"}, Source: "model"},
		{ID: "b6", Kind: atlas.BoundaryRunsProgram, External: "os/exec.CommandContext", ProgramNotNamed: true, Values: []string{"-json"}, Source: "model"},
		{ID: "b7", Kind: atlas.BoundaryRunsProgram, External: "os/exec.Command", Values: []string{}, Source: "model"},
		{ID: "b8", Kind: atlas.BoundarySDK, External: "redis.Client.Get", Destination: "Redis", Values: []string{"key"}, Source: "model"},
	}}
	section := &pageSection{ID: "tool", programTargetID: "tool"}
	builder := pageBuilder{data: &ReportData{}, indexes: []groupindex.Index{index}}
	builder.fillSectionOutbound(section)
	if row := section.Outbound[1]; !row.Program || row.KindLabel != "Runs a program" || !slices.Equal(row.Words, []string{"git", "log", "-n"}) || section.Outbound[4].Program {
		t.Fatalf("a started program's record lost its kind or its words: %+v", section.Outbound)
	}
	got := map[string]int{}
	for _, group := range groupOutbound(section.Outbound) {
		got[group.Destination]++
		if group.Program {
			got[group.Destination+" rows"] = len(group.Rows)
		}
	}
	want := map[string]int{"git": 1, "git rows": 2, "Git": 1, "Git rows": 1, "redis-server": 1, "redis-server rows": 1, "Redis": 1}
	if !maps.Equal(got, want) {
		t.Fatalf("program destinations = %v\nwant %v", got, want)
	}
	var unnamed []string
	for _, row := range section.UnnamedLaunches {
		unnamed = append(unnamed, row.ID+" "+row.ProgramLabel())
	}
	if want := []string{"tool-out-b5 A program named at run time", "tool-out-b6 A program named at run time", "tool-out-b7 Program not established"}; !slices.Equal(unnamed, want) {
		t.Fatalf("unnamed launches = %q\nwant %q", unnamed, want)
	}
}

// A destination one of whose records connects to another program of this
// repository (an integration connection the joints confirmed) is that
// program: every record of the destination reaches it and says so, and a
// destination reaching none, or a started program, keeps what it names.
// redis-cli's "Redis server" was its connect, joined to redis-server's
// listening socket, and the gethostbyname resolving the server's host,
// drawn as an outside system beside the arrow into redis-server.
func TestADestinationThatIsOneOfTheRepositorysProgramsJoinsIt(t *testing.T) {
	connect := programindex.Location{Path: "anet.c", Line: 158, Column: 9}
	client := groupindex.Index{Target: programindex.Target{ID: "t4"}, Outbound: []groupindex.OutboundCall{
		{ID: "b1", SubjectID: "n1", Kind: atlas.BoundaryClientRequest, External: "socket.h.connect", Destination: "Redis server", Location: connect, Source: "model"},
		{ID: "b2", SubjectID: "n1", Kind: atlas.BoundarySDK, External: "netdb.h.gethostbyname", Destination: "Redis server (host lookup)", Location: programindex.Location{Path: "anet.c", Line: 146, Column: 9}, Source: "model"},
		{ID: "b3", SubjectID: "n2", Kind: atlas.BoundarySDK, External: "netdb.h.gethostbyname", Destination: "DNS resolver", Location: programindex.Location{Path: "anet.c", Line: 115, Column: 9}, Source: "model"},
	}, Connections: []groupindex.Connection{
		{ID: "x9", From: groupindex.Endpoint{TargetID: "t4"}, To: groupindex.Endpoint{TargetID: "t1"}, SourceKind: "integration", FromSubjectID: "n1", FromLocation: &connect},
	}}
	server := groupindex.Index{Target: programindex.Target{ID: "t1"}}
	builder := pageBuilder{data: &ReportData{}, indexes: []groupindex.Index{server, client}}
	serverSection := &pageSection{ID: "t1", programTargetID: "t1", ShortLabel: "redis-server"}
	clientSection := &pageSection{ID: "t4", programTargetID: "t4", ShortLabel: "redis-cli"}
	builder.sections = []*pageSection{serverSection, clientSection}
	builder.byProgram = map[string]*pageSection{"t1": serverSection, "t4": clientSection}
	builder.fillSectionOutbound(clientSection)
	got := map[string]string{}
	for _, row := range clientSection.Outbound {
		var reached []string
		for _, program := range row.Runs {
			reached = append(reached, program.Title)
		}
		got[row.ID] = strings.Join(reached, ",")
	}
	if want := map[string]string{"t4-out-b1": "redis-server", "t4-out-b2": "redis-server", "t4-out-b3": ""}; !maps.Equal(got, want) {
		t.Fatalf("records reach %v\nwant %v", got, want)
	}
}
