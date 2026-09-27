package report

import "testing"

// Find lists a declaration no part holds as code: it opens its row in the
// component's Not on the map list and its source, and names no part.
// A declaration off the map is code to find: one no box of its split file
// took, listed under Not on the map, and one of a part its program never
// runs, listed under Not reachable from the entrypoints (redis-cli's
// listCreate, whose part left its map).
func TestFindListsDeclarationsOffTheMap(t *testing.T) {
	code := systemJSPiece(t, "40-find.js", "  // A declaration no part holds", "  function sectionsFor(")
	runSystemJS(t, `
const entries=[],codeEntries=new Map(),components={server:'redis-server',cli:'redis-cli'};
function add(entry){entries.push(entry);}
function rmT(text){return text;}
const chipIn=(name,path,id,where)=>{const row={dataset:{path}},section={id};
  return {row,chip:{textContent:name,where,closest:selector=>selector==='[data-off-map-file]'?row:selector==='[data-report-page]'?section:null}};};
const undecided=chipIn('saveparam:301','redis.c','server','.off-map-catalog'),unreached=chipIn('listCreate:41','adlist.c','cli','.unreached-parts');
const document={querySelectorAll:selector=>[undecided,unreached].filter(one=>selector.split(',').map(s=>s.trim()).includes(one.chip.where+' [data-off-map-file] .chip')).map(one=>one.chip)};
`+code+`
assert.equal(entries.length,2);
for(const [entry,one,path,component] of [[entries[0],undecided,'redis.c','redis-server'],[entries[1],unreached,'adlist.c','redis-cli']]){
  assert.equal(entry.kind,'code');assert.equal(entry.title,one.chip.textContent);assert.equal(entry.path,path);
  assert.equal(entry.component,component);assert.deepEqual(entry.memberships,[]);
  assert.equal(entry.destination,one.row);assert.equal(entry.source,one.chip);
}
`)
}
