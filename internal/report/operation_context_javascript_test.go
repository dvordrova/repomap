package report

import (
	"os/exec"
	"strings"
	"testing"
)

// Exercise the actual graph projection with a chain longer than one neighbour.
// Opening A or B must keep C, the remote endpoint and all original relations.
func TestOperationContextSurvivesOpeningAPart(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node is required for the report JavaScript regression")
	}
	read := func(file string) string {
		t.Helper()
		raw, err := reportTemplateFS.ReadFile("templates/js/" + file)
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	part := func(script, start, end string) string {
		t.Helper()
		_, tail, ok := strings.Cut(script, start)
		if !ok {
			t.Fatalf("missing %q", start)
		}
		body, _, ok := strings.Cut(tail, end)
		if !ok {
			t.Fatalf("missing %q", end)
		}
		return start + body
	}
	ops := read("29-operation-view.js")
	harness := `const assert=require('node:assert/strict');
const byID={};
function n(id,children='',remote='false',activation=''){
 const value={id,dataset:{title:id,children,remote,activation,branch:children?'area':''}};byID[id]=value;return value;
}
const operation=n('op','','false','command');
const groups=[n('area','a b c'),n('a'),n('b'),n('c'),n('remote','','true'),n('unrelated')];
const roots=['area','remote','unrelated'],nearOf={op:['a','b','c','remote']};
const search={value:''},directParts={};
` + part(ops, "function children(n)", "    // An identically named area") +
		part(ops, "function displayed(id)", "    var nearOf=") +
		part(ops, "function allowed()", "    function scopePath(") +
		part(read("28-map-routing.js"), "function fold(", "  function draw(") + `
const repomapGraph={fold};
const rawEdges=[['op','a','entry'],['a','b','ab'],['b','c','bc'],['c','remote','external'],['b','c','second-source']].map(([from,to,label])=>({from,to,label,scope:'operation',operations:['op']}));
rawEdges.push({from:'c',to:'unrelated',label:'other-operation',scope:'operation',operations:['other']});
function projection(scope){
 let currentEdges=[];
` + part(ops, "var limit=allowed()", "      var boxes={}") + `
 return {visible,edges:currentEdges};
}
const complete=projection('');
assert.deepEqual(complete.visible,['op','a','b','c','remote']);
assert.equal(complete.edges.flatMap(e=>e.relations).length,5);
assert.ok(complete.edges.some(e=>e.from==='c'&&e.to==='remote'));
for(const scope of ['area','a','b','c'])assert.deepEqual(projection(scope),complete,scope);
`
	if output, err := exec.Command(node, "-e", harness).CombinedOutput(); err != nil {
		t.Fatalf("operation context regression: %v\n%s", err, output)
	}
}
