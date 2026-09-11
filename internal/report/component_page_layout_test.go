package report

import (
	"bytes"
	"html/template"
	"strings"
	"testing"
)

// The component page opens with the orientation's one-line role and purpose
// and the entrypoints, and its main flow and configuration follow the map.
// The owner's audit found five visible words below the map on every page and
// no sentence saying what the component is; the reference block keeps only
// the inventory-grade sections.
func TestComponentPageLeadsWithPurposeAndKeepsFlowBelowTheMap(t *testing.T) {
	section := &pageSection{ID: "svc", ShortLabel: "svc", Language: "go", Kind: "executable", FactsAvailable: true,
		Role: "Go web server", Purpose: "Serves the meetup pages and talks to PostgreSQL.",
		Entrypoints: []pageEntrypoint{{Symbol: "main", Kind: "callable", Anchor: &pageAnchor{Text: "main.go:157"}}},
		Map:         &pageMap{},
		Flow:        &pageFlow{Title: "Serving the events page", Steps: []pageFlowStep{{Label: "main", Explanation: "Parses flags and starts Echo."}}},
		Config:      []pageConfig{{Key: "PG_URL", Anchor: &pageAnchor{Text: "main.go:166"}}},
	}
	parsed, err := template.New("report").Funcs(pageTemplateFuncs(Russian)).ParseFS(reportTemplateFS, "templates/html/*.html")
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := parsed.ExecuteTemplate(&out, "target.html", section); err != nil {
		t.Fatal(err)
	}
	html := out.String()
	at := func(marker string) int {
		i := strings.Index(html, marker)
		if i < 0 {
			t.Fatalf("component page lost %q", marker)
		}
		return i
	}
	header, purpose, entry := at(`class="component-intro"`), at(`class="component-purpose"`), at(`class="component-entrypoints"`)
	parts, flow, config, reference := at(`class="component-parts"`), at(`class="component-flow"`), at(`class="component-config"`), at(`class="component-reference"`)
	if !(header < purpose && purpose < entry && entry < parts) {
		t.Fatal("purpose and entrypoints do not lead the page")
	}
	if !(parts < flow && flow < config && config < reference) {
		t.Fatal("main flow and configuration are not the first content after the map")
	}
	if !strings.Contains(html, "Go web server") || !strings.Contains(html, "Serves the meetup pages") || !strings.Contains(html, "main.go:157") {
		t.Fatal("role, purpose or entrypoint text missing")
	}
	// Without a flow or a start list the page says nothing about a flow:
	// "the model did not include this component" explains the tool, not the code.
	section.Flow, section.Start, section.FlowMissing = nil, nil, "The model did not include this target."
	out.Reset()
	if err := parsed.ExecuteTemplate(&out, "target.html", section); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "component-flow") || strings.Contains(out.String(), "did not include") {
		t.Fatal("an absent flow still produced a flow section")
	}
}
