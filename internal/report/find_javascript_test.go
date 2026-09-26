package report

import "testing"

// Find lists a declaration no part holds as code: it opens its row in the
// component's Not on the map list and its source, and names no part.
func TestFindListsDeclarationsOffTheMap(t *testing.T) {
	code := systemJSPiece(t, "40-find.js", "  // A declaration no part holds", "  function sectionsFor(")
	runSystemJS(t, `
const entries=[],codeEntries=new Map(),components={server:'redis-server'};
function add(entry){entries.push(entry);}
function rmT(text){return text;}
const section={id:'server'},row={dataset:{path:'redis.c'}};
const chip={textContent:'saveparam:301',closest:selector=>selector==='[data-off-map-file]'?row:selector==='[data-report-page]'?section:null};
const document={querySelectorAll:selector=>selector==='.off-map-catalog [data-off-map-file] .chip'?[chip]:[]};
`+code+`
assert.equal(entries.length,1);
const entry=entries[0];
assert.equal(entry.kind,'code');assert.equal(entry.title,'saveparam:301');assert.equal(entry.path,'redis.c');
assert.equal(entry.component,'redis-server');assert.deepEqual(entry.memberships,[]);
assert.equal(entry.destination,row);assert.equal(entry.source,chip);
`)
}
