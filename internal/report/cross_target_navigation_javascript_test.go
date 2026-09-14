package report

import "testing"

// Both sides of a matched boundary now live in the same canvas. Selecting a
// backend operation from a frontend question must keep the original question
// in the ordinary navigator's serialized browser history.
func TestCrossTargetOperationKeepsQuestionAndBackHistory(t *testing.T) {
	modes := systemJSPiece(t, "45-modes.js", "function readingState(){", "  function savedElement(")
	setPage := systemJSPiece(t, "45-modes.js", "function setPage(node){", "  // The former Learn/Work switch")
	runSystemJS(t, `
const home={id:'overview',closest(){return null;},querySelector(){return map;}},repoMap={id:'repository-map'},questionPage={id:'questions'};
const guide={id:'question5',closest(){return null;}},backend={id:'backend-post',dataset:{activation:'request'},closest(){return null;}};
const map={readingState(){return {scope:'',operation:'backend-post'};}};
let current=questionPage,question=guide,term=null,searchIntent='',restoring=false,detailGroup=null;
const globalSearch=null,body={dataset:{}},pages=[home,questionPage];
const document={querySelector(){return null;},querySelectorAll(){return [];}};
let visits=0;
const history={state:null,replaceState(value){this.state=value;},pushState(value,unused,url){visits++;this.state=value;location.href=url.href;}};
const location={href:'https://report.invalid/#question5'};
function enclosing(node){return node===guide?questionPage:home;}function revealConcept(){}function placeSearch(){}function showReturn(){}function showLocation(){}function measureToolbar(){}function selectQuestion(){}
`+modes+setPage+`
setPage(backend);address(backend);
assert.equal(current,home);assert.equal(question,guide);
assert.equal(history.state.repomapReading.question,'question5');
assert.equal(history.state.repomapReading.map.value.operation,'backend-post');
assert.equal(history.state.repomapReading.map.page,'overview');
const before=visits;address(backend);assert.equal(visits,before,'reselecting an item alone is not another visit');
address(backend,false,true);assert.equal(visits,before+1,'opening the same item from overview retains a return visit even though its URL is unchanged');
setPage(home);assert.equal(question,null,'explicit Home clears the question intention');
setPage(guide);assert.equal(current,questionPage);assert.equal(questionPage.hidden,false);
assert.equal(home.hidden,false,'reading an answer keeps the same map mounted above it');
`)
}
