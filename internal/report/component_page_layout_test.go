package report

import (
	"bytes"
	"html/template"
	"strings"
	"testing"
)

// The component page opens with the orientation's one-line role and purpose
// and the entrypoints; a component with no flow says nothing about one.
func TestComponentPageLeadsWithPurpose(t *testing.T) {
	section := &pageSection{ID: "svc", ShortLabel: "svc", Language: "go", Kind: "executable", FactsAvailable: true,
		Role: "Go web server", Purpose: "Serves the meetup pages and talks to PostgreSQL.",
		Entrypoints: []pageEntrypoint{{Symbol: "main", Kind: "callable", Anchor: &pageAnchor{Text: "main.go:157"}}},
		Map:         &pageMap{},
		Flow:        &pageFlow{Steps: []pageFlowStep{{Label: "main", Explanation: "Parses flags and starts Echo."}}},
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
	if !(header < purpose && purpose < entry && entry < at(`class="component-parts"`)) {
		t.Fatal("purpose and entrypoints do not lead the page")
	}
	at(`class="component-flow"`)
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

// Orientation may keep a role whose purpose was left empty; the page shows
// the role alone, with no dangling dash.
func TestComponentPageShowsARoleWithoutAPurpose(t *testing.T) {
	section := &pageSection{ID: "svc", ShortLabel: "svc", Language: "go", Kind: "executable", FactsAvailable: true, Role: "Go web server", Map: &pageMap{}}
	parsed, err := template.New("report").Funcs(pageTemplateFuncs(English)).ParseFS(reportTemplateFS, "templates/html/*.html")
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := parsed.ExecuteTemplate(&out, "target.html", section); err != nil {
		t.Fatal(err)
	}
	html := out.String()
	start := strings.Index(html, `class="component-purpose"`)
	if start < 0 {
		t.Fatal("a role without a purpose is not shown")
	}
	line := html[start : strings.Index(html[start:], "</p>")+start]
	if !strings.Contains(line, "Go web server") || strings.Contains(line, "—") {
		t.Fatalf("role line: %s", line)
	}
}
