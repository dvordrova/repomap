package report

import (
	"encoding/json"
	"strings"
	"testing"
)

// The page's data writes each value once, each declaration a reading names
// once, and the source links' base once (the page-size study's M1-M4):
// Redis's 148 members of one catalogue had carried its reading 148 times,
// and its inputs 4,966 copies of 831 declarations.
func TestPageDataWritesEachValueAndDeclarationOnce(t *testing.T) {
	base := "https://github.com/o/r/blob/abc/"
	data := newPageData(base)
	decl := `{"name":"call","href":"` + base + `redis.c#L2000"}`
	first, err := data.ref("inputPath", `{"decls":[`+decl+`,{"name":"getCommand"}],"parts":[{"handler":1}]}`)
	if err != nil {
		t.Fatal(err)
	}
	second, _ := data.ref("inputPath", `{"decls":[{"name":"getCommand"},`+decl+`],"parts":[]}`)
	catalogue := `{"decls":[` + decl + `],"members":["o1","o2"]}`
	one, _ := data.ref("catalogue", catalogue)
	again, _ := data.ref("catalogue", catalogue)
	if one != again || first == second {
		t.Fatalf("a value shared by members is written once, distinct ones apart: %s %s %s %s", first, second, one, again)
	}
	if len(data.decls) != 2 {
		t.Fatalf("each declaration once: %s", data.decls)
	}
	if !strings.Contains(string(data.decls[0]), `"\u0001redis.c#L2000"`) || strings.Contains(string(data.decls[0]), base) {
		t.Fatalf("the link's base is written once, not in each link: %s", data.decls[0])
	}
	if got := string(data.values[0]); got != `{"decls":[0,1],"parts":[{"handler":1}]}` {
		t.Fatalf("a reading names its declarations by their index: %s", got)
	}
	if empty, _ := data.ref("symbols", ""); empty != "" {
		t.Fatal("no value, no reference")
	}
	if data.attrLink(base+"anet.c#L12") != "@anet.c#L12" || newPageData("").attrLink("x.go:1:2") != "x.go:1:2" {
		t.Fatal("a key in an attribute is written without its base only on a static page")
	}
	raw, err := data.JSON()
	if err != nil {
		t.Fatal(err)
	}
	var page struct {
		Base   string            `json:"base"`
		Decls  []json.RawMessage `json:"decls"`
		Values []json.RawMessage `json:"values"`
	}
	if err := json.Unmarshal([]byte(raw), &page); err != nil || page.Base != base || len(page.Values) != 3 {
		t.Fatalf("page data: %s %v", raw, err)
	}
}

// The page's script reads a value back as it was: its links whole and its
// declarations in place.
func TestThePageScriptReadsItsDataBackWhole(t *testing.T) {
	base := "https://github.com/o/r/blob/abc/"
	data := newPageData(base)
	reading := `{"decls":[{"name":"serverCron","href":"` + base + `redis.c#L1250","key":"k"}],"members":[{"kind":"function","decls":[0]}]}`
	ref, err := data.ref("reading", reading)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := data.JSON()
	script := systemJSPiece(t, "10-ui.js", "var rmPage = (function () {", "})();") + "})();\n"
	runSystemJS(t, `const document={getElementById(){return {textContent:String.raw`+"`"+string(raw)+"`"+`};}};
`+strings.Replace(script, "var rmPage", "var page_", 1)+`
const group={dataset:{reading:'`+ref+`'}};
assert.deepEqual(page_.data(group,'reading'),`+reading+`);
assert.equal(page_.data(group,'reading'),page_.data(group,'reading'),'parsed once');
assert.equal(page_.data({dataset:{}},'reading'),null);
assert.equal(page_.link('@anet.c#L12'),'`+base+`anet.c#L12');
`)
}
