package report

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"
)

// Execute the owning member, selection and history code. The DOM stubs only
// supply ordinary element storage; identities and selection come from the UI.
func TestUnavailableSourcesKeepCodeMembersAndSelection(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node is required to execute the report's JavaScript regression")
	}
	read := func(name string) string {
		t.Helper()
		raw, err := reportTemplateFS.ReadFile("templates/js/" + name)
		if err != nil {
			t.Fatal(err)
		}
		check := exec.CommandContext(t.Context(), node, "--check", "-")
		check.Stdin = bytes.NewReader(raw)
		if output, err := check.CombinedOutput(); err != nil {
			t.Fatalf("%s syntax: %v\n%s", name, err, output)
		}
		return string(raw)
	}
	between := func(script, start, end string) string {
		t.Helper()
		_, tail, ok := strings.Cut(script, start)
		if !ok {
			t.Fatalf("missing UI function %q", start)
		}
		body, _, ok := strings.Cut(tail, end)
		if !ok {
			t.Fatalf("missing UI function boundary %q", end)
		}
		return start + body
	}
	members := read("26-map-members.js")
	operations := read("29-operation-view.js")
	reading := read("30-map.js")
	snapshot := between(operations, "function snapshot(){", "    async function restore(saved)")
	explain := between(reading, "map.explainSource=function(source){", "    map.showNode=")
	harness := `
const assert = require('node:assert/strict');
function element(tag) {
  return {tag,children:[],dataset:{},attrs:{},events:{},textContent:'',
    classList:{add(){}},
    appendChild(child){this.children.push(child);return child;},
    replaceChildren(...children){this.children=children;},
    setAttribute(k,v){this.attrs[k]=v;},
    addEventListener(k,fn){this.events[k]=fn;},
    querySelectorAll(){return this.children.filter(n=>n.tag==='button');}};
}
const groups=new Map();
const document={createElement:element,getElementById(id){return groups.get(id)||null;}};
function rmT(key){return key==='No source'?'Нет ссылки на исходник':key;}
` + members + `
const first={name:'Same',explanation:'First explanation',source:{NoSource:true,Path:'src/one.go',Line:4,Text:'src/one.go:4:7'}};
const second={name:'Same',explanation:'Second explanation',source:{NoSource:true,Path:'src/two.go',Line:4,Text:'src/two.go:4:9'}};
const remote={name:'Remote',source:{Href:'https://example.invalid/blob/rev/source.go#L8',Text:'source.go:8'}};
const local={name:'Local',source:{Open:'working.go:8:3',Text:'working.go:8'}};
// The same declaration also appears in static highlights, whose text has no
// column suffix. Link absence must not turn that second projection into a cube.
const chip={dataset:{noSource:'true',sourcePath:'src/one.go',sourceLine:'4'},
  getAttribute(){return null;},cloneNode(){return {textContent:'Same',querySelectorAll(){return [];}};}};
const row={dataset:{},querySelector(selector){return selector==='strong > .chip'?chip:selector==='.anchor'?{textContent:'src/one.go:4'}:null;}};
groups.set('part',{querySelectorAll(){return [row];}});
const node={dataset:{concepts:JSON.stringify([first,second,remote,local,first]),title:'Part'},getAttribute(){return '#part';}};
const items=repomapMembers.items(node);
assert.equal(items.length,4,'unavailable members remain distinct; repeated source is deduplicated');
assert.equal(items[1].explanation,second.explanation);
for(const item of [first,second]){
  const link=repomapMembers.sourceLink(item.source);
  assert.equal(link.tag,'span');assert.equal(link.textContent,item.source.Text);
  assert.equal(link.title,'Нет ссылки на исходник');
  assert.equal(link.href,undefined);assert.equal(link.dataset.open,undefined);
}
assert.equal(repomapMembers.sourceLink(remote.source).href,remote.source.Href);
assert.equal(repomapMembers.sourceLink(local.source).dataset.open,local.source.Open);
const map={exploreNode(){},showMember(node,item){this.picked=item;}};
const grid=repomapMembers.grid(map,node);
assert.equal(grid.children.length,4);
const explainButtons=grid.children.map(row=>row.children[0]);
explainButtons[1].events.click({stopPropagation(){}});
assert.equal(map.picked.source.Text,second.source.Text);
assert.notEqual(explainButtons[0].dataset.memberSource,explainButtons[1].dataset.memberSource);
assert.match(grid.children[1].children[0].title,/Нет ссылки на исходник/);
assert.equal(grid.children[2].children[0].href,remote.source.Href,'the code name links directly to its source');
assert.equal(grid.children[3].children[0].dataset.open,local.source.Open,'the local code name retains its editor destination');
let inspectedNode=node;
map.inspectConcept=index=>{map.selected=items[index];};
` + explain + `
map.explainSource({key:repomapMembers.sourceKey(second.source)});
assert.equal(map.selected.source.Text,second.source.Text);
const scope='part',operation=null,pinned=false,mode='structure',search={value:''};
const byID={part:node},window={scrollY:50};
function readingDisclosures(){return null;}
map.explorerMember={owner:scope,name:'Same',key:repomapMembers.sourceKey(second.source),href:'',open:''};
` + snapshot + `
const saved=JSON.parse(JSON.stringify(snapshot()));
map.selected=items[0];
map.explainSource(saved.source);
assert.equal(map.selected.source.Text,second.source.Text,'Back restores exact missing-source member');

`
	command := exec.CommandContext(t.Context(), node, "--eval", harness)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("missing-source UI behavior: %v\n%s", err, output)
	}
}
