package report

import (
	"os/exec"
	"strings"
	"testing"
)

// A "Where this service connects" section with more than five destinations is
// compacted to five rows and an "All N" disclosure. The destination rows carry
// their own nested record lists; compaction must move the rows, not strip the
// lists inside them. The owner's service (more than five destinations) once
// opened every destination to nothing.
func TestCatalogDisclosureKeepsDestinationRecords(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node is required for the report JavaScript regression")
	}
	raw, err := reportTemplateFS.ReadFile("templates/js/28-map-routing.js")
	if err != nil {
		t.Fatal(err)
	}
	start, end := "// Disclosures change the first view only", "// Folding retains evidence"
	_, tail, ok := strings.Cut(string(raw), start)
	if !ok {
		t.Fatalf("missing %q", start)
	}
	compaction, _, ok := strings.Cut(tail, end)
	if !ok {
		t.Fatalf("missing %q", end)
	}
	harness := `const assert=require('node:assert/strict');
function matches(node,selector){
  return selector.split(',').some(function(simple){
    simple=simple.trim();
    var tag=(simple.match(/^[a-z]+/)||[''])[0];
    if(tag&&node.tag!==tag)return false;
    var classes=simple.match(/\.[\w-]+/g)||[];
    if(!classes.every(function(c){return node.className.split(/\s+/).includes(c.slice(1));}))return false;
    var attrs=simple.match(/\[[\w-]+\]/g)||[];
    return attrs.every(function(a){return a.slice(1,-1) in node.attrs;});
  });
}
function element(tag,className,attrs){
  var node={tag:tag,className:className||'',attrs:attrs||{},dataset:{},children:[],parent:null,textContent:'',hidden:false,
    appendChild:function(child){if(child.parent)child.parent.children.splice(child.parent.children.indexOf(child),1);child.parent=this;this.children.push(child);return child;},
    append:function(){Array.prototype.forEach.call(arguments,this.appendChild,this);},
    remove:function(){if(this.parent){this.parent.children.splice(this.parent.children.indexOf(this),1);this.parent=null;}},
    descendants:function(){var out=[];this.children.forEach(function(c){out.push(c);out.push.apply(out,c.descendants());});return out;},
    querySelectorAll:function(selector){return this.descendants().filter(function(n){return matches(n,selector);});},
    querySelector:function(selector){return this.querySelectorAll(selector)[0]||null;},
    closest:function(selector){for(var n=this;n;n=n.parent)if(matches(n,selector))return n;return null;}};
  Object.keys(node.attrs).forEach(function(k){if(k.startsWith('data-'))node.dataset[k.slice(5).replace(/-([a-z])/g,function(_,c){return c.toUpperCase();})]=node.attrs[k];});
  return node;
}
function destination(i){
  var item=element('li','',{'data-integration-item':'','data-source-kind':'model'});
  var title=element('strong','input-title');title.textContent='Destination '+i;item.appendChild(title);
  var list=element('ul','plain outbound-calls');
  for(var j=0;j<2;j++){var row=element('li','',{'data-integration-record':''});row.textContent='record '+i+'.'+j;list.appendChild(row);}
  item.appendChild(list);return item;
}
const root=element('div');
const group=element('section','',{'data-integration-group':''});root.appendChild(group);
const first=element('ul','plain operation-catalog');group.appendChild(first);
for(var i=1;i<=5;i++)first.appendChild(destination(i));
const more=element('details','input-more');group.appendChild(more);
more.appendChild(element('summary'));const restList=element('ul','plain operation-catalog');more.appendChild(restList);restList.appendChild(destination(6));
const document={createElement:function(tag){return element(tag);},querySelectorAll:function(selector){return root.querySelectorAll(selector);}};
function rmT(key,n){return String(key).replace('{0}',n);}
` + start + compaction + `
const items=group.querySelectorAll('[data-integration-item]');
assert.equal(items.length,6,'every destination row survives compaction');
assert.equal(group.querySelectorAll('[data-integration-record]').length,12,'every nested record survives compaction');
items.forEach(function(item,index){
  assert.ok(item.querySelector('ul.outbound-calls'),'destination '+(index+1)+' keeps its record list');
  assert.equal(item.querySelectorAll('[data-integration-record]').length,2);
});
const ownLists=group.children.filter(function(n){return n.tag==='ul';});
assert.equal(ownLists.length,1,'one preview list at the group level');
assert.equal(ownLists[0].children.length,5,'five rows before the disclosure');
const disclosures=group.children.filter(function(n){return n.className==='input-more';});
assert.equal(disclosures.length,1,'one All N disclosure');
assert.equal(disclosures[0].querySelectorAll('[data-integration-item]').length,1,'the sixth row waits under the disclosure');
assert.equal(disclosures[0].querySelector('summary').textContent,'All 6 →');
`
	if output, err := exec.Command(node, "-e", harness).CombinedOutput(); err != nil {
		t.Fatalf("catalog disclosure regression: %v\n%s", err, output)
	}
}
