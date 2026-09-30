package report

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

// A card prints no line number (owner: the purple name is the link). Its
// source reads as its file and still links to the line; under a linked
// "code as written" it is not printed again. freqtrade's Remote Pairlist
// Server had read "freqtrade/plugins/pairlist/RemotePairList.py:46".
// Exercises the actual card construction of 30-map.js, as
// TestMapReadingPreservesQuotedSourceAttributesAndNames does.
func TestACardSourcePrintsNoLineNumber(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node is required to execute the report's JavaScript regression")
	}
	script, err := reportTemplateFS.ReadFile("templates/js/30-map.js")
	if err != nil {
		t.Fatal(err)
	}
	between := func(start, end string) string {
		_, tail, found := strings.Cut(string(script), start)
		if !found {
			t.Fatalf("map reading fragment start moved: %s", start)
		}
		value, _, found := strings.Cut(tail, end)
		if !found {
			t.Fatalf("map reading fragment end moved: %s", end)
		}
		return start + value
	}
	helpers := between("function rmInputPart(", "// A dispatch site read with its declaration") + between("function escapeText(text)", "  function bindReading(map)")
	show := between("function show(node) {", "      // Keep the current object and term") + "\n}"
	type variant struct {
		Source  string `json:"source"`
		Open    string `json:"open"`
		Text    string `json:"text"`
		Written string `json:"written"`
		Missing bool   `json:"missing"`
	}
	href := "https://example.invalid/blob/rev/freqtrade/plugins/pairlist/RemotePairList.py#L46"
	variants := []variant{
		{Source: href, Text: "freqtrade/plugins/pairlist/RemotePairList.py:46"},
		{Source: href, Text: "freqtrade/plugins/pairlist/RemotePairList.py:46", Written: "get()"},
		{Open: "src/app.py:7:3", Text: "src/app.py:7"},
		{Missing: true, Text: "src/app.py:4:9"},
	}
	input, err := json.Marshal(variants)
	if err != nil {
		t.Fatal(err)
	}
	harness := jsPageDataStandIn + `
const fs = require('node:fs');
const variants = JSON.parse(fs.readFileSync(0, 'utf8'));
const document = {createElement(){return {textContent:'',get innerHTML(){return this.textContent.replace(/[&<>]/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;'}[c]));}};}};
function rmT(key){return key;}
rmT.html = key => key;
` + helpers + `
function render(value) {
  const content = {scrollTop:0}, card = {classList:{remove(){},toggle(){}}};
  const remembered = new Map();
  let inspectedNode = null, inspectionKey = '', inspectionPending = false, inspectionRevision = 0;
  function remember() {}
  function sentences() {return [];}
  const map = {classList:{contains(){return false;}},hasAttribute(){return false;}};
  const attrs = {'data-node':'node-1','data-source':value.source,'data-source-text':value.text};
  const node = {dataset:{title:'Remote Pairlist Server',sourceText:value.text,open:value.open,noSource:String(!!value.missing),written:value.written,source:value.source,concepts:'[]'},getAttribute(name){return attrs[name]||'';}};
` + show + `
  show(node);
  return card.innerHTML;
}
process.stdout.write(JSON.stringify(variants.map(render)));
`
	command := exec.CommandContext(t.Context(), node, "--eval", harness)
	command.Stdin = bytes.NewReader(input)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("map card JavaScript failed: %v\n%s", err, output)
	}
	var cards []string
	if err := json.Unmarshal(output, &cards); err != nil || len(cards) != len(variants) {
		t.Fatalf("missing rendered cards: %v, %s", err, output)
	}
	tags := regexp.MustCompile(`<[^>]*>`)
	line := regexp.MustCompile(`\S:\d+`)
	for i, card := range cards {
		text := tags.ReplaceAllString(card, " ")
		if at := line.FindString(text); at != "" {
			t.Fatalf("variant %d prints a line number (%q): %s", i, at, text)
		}
		switch {
		case variants[i].Written != "":
			if strings.Count(card, href) != 1 || strings.Contains(text, "RemotePairList.py") {
				t.Fatalf("the code as written is the one link to its line: %s", card)
			}
		case variants[i].Source != "":
			if !strings.Contains(card, `href="`+href+`"`) || !strings.Contains(text, "freqtrade/plugins/pairlist/RemotePairList.py") {
				t.Fatalf("the source lost its file or its link to the line: %s", card)
			}
		case variants[i].Open != "":
			if !strings.Contains(card, `data-open="`+variants[i].Open+`"`) || !strings.Contains(text, "src/app.py") {
				t.Fatalf("the served source lost its file or its line: %s", card)
			}
		}
	}
}
