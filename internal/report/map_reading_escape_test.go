package report

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"io"
	"os/exec"
	"strings"
	"testing"
)

// Exercise the actual card construction, ending at its innerHTML assignment.
// The small DOM stubs model only reads performed while preparing that fragment;
// no HTML handlers, navigation or network requests are executed.
func TestMapReadingPreservesQuotedSourceAttributesAndNames(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node is required to execute the report's JavaScript regression")
	}
	script, err := reportTemplateFS.ReadFile("templates/js/30-map.js")
	if err != nil {
		t.Fatal(err)
	}
	check := exec.CommandContext(t.Context(), node, "--check", "-")
	check.Stdin = bytes.NewReader(script)
	if output, err := check.CombinedOutput(); err != nil {
		t.Fatalf("map JavaScript syntax: %v\n%s", err, output)
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
	helpers := between("function escapeText(text)", "  function bindReading(map)")
	show := between("function show(node) {", "      // Keep the current object and term") + "\n}"
	// A real relative source filename may contain quotes, ampersands and Unicode.
	// The added attribute-looking text is inert data, not an executable payload.
	path := `src/module" data-injected="yes' & 한국.py`
	if err := validateManifestPath(path); err != nil {
		t.Fatalf("fixture is not an ordinary allowed source path: %v", err)
	}
	open := path + ":7:3"
	name := `load "quoted" & <value> '한국'`
	url := `https://example.invalid/module" data-injected="yes.py?key=a&other='b'#L7`
	cases := []struct {
		Source string `json:"source"`
		Open   string `json:"open"`
		Name   string `json:"name"`
	}{
		{Open: open, Name: name},
		{Source: url, Name: name},
		{Open: "src/simple.py:7:3", Name: "load"},
	}
	input, err := json.Marshal(cases)
	if err != nil {
		t.Fatal(err)
	}
	harness := `
const fs = require('node:fs');
const cases = JSON.parse(fs.readFileSync(0, 'utf8'));
// Browser text-node serialization deliberately does not escape quotes. This
// also makes the regression detect the original helper's attribute bug.
const document = {createElement(){return {textContent:'',get innerHTML(){return this.textContent.replace(/[&<>]/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;'}[c]));}};}};
function rmT(key){return key;}
rmT.html = key => key;
` + helpers + `
function render(value) {
  const content = {scrollTop:0}, card = {classList:{remove(){},toggle(){}}};
  const remembered = new Map();
  let inspectedNode = null, inspectionRevision = 0;
  function remember() {}
  function sentences() {return [];}
  const step = {name:value.name,source:value.name,open:value.open,href:value.source};
  const map = {classList:{contains(){return false;}},inspectedOperation:{dataset:{title:'Operation',callPaths:JSON.stringify({'node-1':[step]})}}};
  const attrs = {'data-node':'node-1','data-summary':value.name,'data-source':value.source,'data-source-text':value.name};
  const node = {dataset:{title:value.name,sourceText:value.name,open:value.open,summaryRef:'t1',concepts:'[]'},getAttribute(name){return attrs[name]||'';}};
` + show + `
  show(node);
  return card.innerHTML;
}
process.stdout.write(JSON.stringify(cases.map(render)));
`
	command := exec.CommandContext(t.Context(), node, "--eval", harness)
	command.Stdin = bytes.NewReader(input)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("map card JavaScript failed: %v\n%s", err, output)
	}
	var fragments []string
	if err := json.Unmarshal(output, &fragments); err != nil || len(fragments) != len(cases) {
		t.Fatalf("missing rendered source variants: %v, %s", err, output)
	}
	for i, fragment := range fragments {
		decoder := xml.NewDecoder(strings.NewReader("<div>" + fragment + "</div>"))
		decoder.Strict, decoder.AutoClose, decoder.Entity = false, xml.HTMLAutoClose, xml.HTMLEntity
		var links []map[string]string
		var texts strings.Builder
		for {
			token, err := decoder.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("variant %d has invalid card markup: %v", i, err)
			}
			switch token := token.(type) {
			case xml.StartElement:
				attrs := make(map[string]string)
				for _, attr := range token.Attr {
					if attr.Name.Local == "data-injected" || strings.HasPrefix(attr.Name.Local, "on") {
						t.Fatalf("variant %d acquired an attribute from source data: %+v", i, attr)
					}
					attrs[attr.Name.Local] = attr.Value
				}
				if token.Name.Local == "a" {
					links = append(links, attrs)
				}
			case xml.CharData:
				texts.Write(token)
			}
		}
		if len(links) != 2 {
			t.Fatalf("variant %d lost the node or call-path source: %+v", i, links)
		}
		for _, link := range links {
			if cases[i].Source != "" {
				if len(link) != 3 || link["href"] != cases[i].Source || link["target"] != "_blank" || link["rel"] != "noopener" {
					t.Fatalf("variant %d changed the source URL or attributes: %+v", i, link)
				}
			} else if len(link) != 2 || link["href"] != "#" || link["data-open"] != cases[i].Open {
				t.Fatalf("variant %d changed the exact served source: %+v", i, link)
			}
		}
		// Title, summary, link labels and call-path name retain the exact text.
		if strings.Count(texts.String(), cases[i].Name) != 5 {
			t.Fatalf("variant %d changed the original display name: %s", i, texts.String())
		}
	}
}
