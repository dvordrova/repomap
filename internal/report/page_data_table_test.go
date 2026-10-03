package report

import (
	"encoding/json"
	"strconv"
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

// The compaction is read back exactly (owner, 2026-09-29: "the output and
// behaviour stay the same; only rm-page-data shrinks"): a site's link its
// place says, a declaration's link to all its lines, a call's ends and
// words, a tile's declaration, a reading's calls, an anchor, and a part
// written twice, each restored as it was registered, on GitHub and GitLab.
func TestThePageScriptReadsTheCompactDataBackExactly(t *testing.T) {
	for _, host := range []struct{ base, sep string }{{"https://github.com/o/r/blob/abc/", "-L"}, {"https://gitlab.com/o/r/-/blob/abc/", "-"}} {
		base := host.base
		data := newPageDataRange(base, host.sep)
		key := func(name, path string, line, column int) string {
			return path + ":" + strconv.Itoa(line) + ":" + strconv.Itoa(column) + ":function:" + name
		}
		decl := func(name, path string, line, end int) string {
			return `{"name":"` + name + `","key":"` + key(name, path, line, 5) + `","href":"` + base + path + `#L` + strconv.Itoa(line) + `","code":"` + base + path + `#L` + strconv.Itoa(line) + host.sep + strconv.Itoa(end) +
				`","at":"` + path + `:` + strconv.Itoa(line) + `","file":"` + path + `","kind":"function","part":"#t1-g2"}`
		}
		cron, resize := key("serverCron", "redis.c", 1250, 5), key("tryResizeHashTables", "redis.c", 1180, 5)
		values := map[string]string{
			"reading": `{"decls":[` + decl("serverCron", "redis.c", 1250, 1400) + `,` + decl("tryResizeHashTables", "redis.c", 1180, 1190) + `,{"name":"lookupKeyRead","key":"k","at":"db.c:9"}],` +
				`"members":[{"kind":"function","decls":[0,1]}],"in":[{"part":"#p","title":"P","count":1,"lines":[{"caller":0,"ends":[{"decl":1,"kind":"calls","sites":[{"at":"redis.c:1277","href":"` + base + `redis.c#L1277"}]},{"decl":2,"kind":"passes_callback"}]}]}],` +
				`"own":[{"decl":0,"callees":[{"part":"#p","title":"P","decls":[{"decl":1,"kind":"calls","possible":true,"sites":[{"at":"redis.c:1277","href":"` + base + `redis.c#L1277"},{"at":"weird path.c:3","href":"` + base + `weird%20path.c#L3"}]}]}],"uses":[{"decl":2,"kind":"reads"},{"decl":1,"kind":"calls"}]}]}`,
			// A call landing at its callee's declaration, one landing inside
			// it, one to an outside symbol, one made inline (its caller no
			// declaration of the page), an input's handler, and words.
			"calls": `[{"at":"redis.c:1277","callee":"` + cron[:0] + resize + `","caller":"` + cron + `","caller_name":"serverCron","callee_name":"tryResizeHashTables","from":"` + base + `redis.c#L1277","kind":"calls","to":"` + base + `redis.c#L1180"},` +
				`{"at":"redis.c:1290","caller":"` + cron + `","callee":"` + resize + `","caller_name":"serverCron","callee_name":"tryResizeHashTables","from":"` + base + `redis.c#L1290","kind":"passes_callback","to":"` + base + `redis.c#L1185"},` +
				`{"at":"anet.c:146","callee":"","caller":"` + cron + `","caller_name":"serverCron","callee_name":"netdb.h.gethostbyname","from":"` + base + `anet.c#L128","kind":"calls","to":"` + base + `anet.c#L146"},` +
				`{"at":"redis.c:1301","callee":"` + resize + `","caller":"redis.c:1299:9:function:serverCron$1","caller_name":"one of two anonymous functions in serverCron","callee_name":"tryResizeHashTables","from":"` + base + `redis.c#L1301","kind":"calls","to":"` + base + `redis.c#L1180"},` +
				`{"label":"implemented in","name":"serverCron","to":"` + base + `redis.c#L1250","callee":"` + cron + `"},` +
				`{"label":"Сервер вызывает хранилище","from":"` + base + `redis.c#L1250","at":"redis.c:1250"}]`,
			// A tile of a declaration, one named otherwise, and a second
			// declaration on serverCron's line, its own identity beside the
			// link both share.
			"symbols": `[{"name":"serverCron","kind":"function","href":"` + base + `redis.c#L1250","code":"` + base + `redis.c#L1250` + host.sep + `1400","path":"redis.c","line":1250,"text":"(): int","decl_key":"` + cron + `"},` +
				`{"name":"Server.serverCron","kind":"function","href":"` + base + `redis.c#L1250","path":"redis.c"},{"name":"other","href":"` + base + `x.c#L2"},` +
				`{"name":"cronHelper","kind":"function","href":"` + base + `redis.c#L1250","path":"redis.c","line":1250,"decl_key":"redis.c:1250:40:function:cronHelper"}]`,
			"writes": `[{"callers":[{"href":"` + base + `redis.c#L2718","name":"decrRefCount","source":"redis.c:2718"}],"entity":{"Code":"` + base + `adlist.h#L36` + host.sep + `40","Href":"` + base + `adlist.h#L36","Key":"adlist.h:36:16:type:listNode","Line":36,"Open":"","Path":"adlist.h","Text":"adlist.h:36"},"entity_name":"listNode","field":"value","possible":false,"source":{"Href":"` + base + `adlist.c#L86","Line":86,"Open":"","Path":"adlist.c","Text":"adlist.c:86"}},` +
				`{"callers":[{"href":"` + base + `redis.c#L2718","name":"decrRefCount","source":"redis.c:2718"}],"entity":{"Code":"` + base + `adlist.h#L36` + host.sep + `40","Href":"` + base + `adlist.h#L36","Key":"adlist.h:36:16:type:listNode","Line":36,"Open":"","Path":"adlist.h","Text":"adlist.h:36"},"entity_name":"listNode","field":"next","possible":false,"source":{"Href":"` + base + `adlist.c#L89","Line":89,"Open":"","Path":"adlist.c","Text":"adlist.c:89"}}]`,
		}
		refs := map[string]string{}
		for _, name := range []string{"reading", "calls", "symbols", "writes"} {
			ref, err := data.ref(name, values[name])
			if err != nil {
				t.Fatal(err)
			}
			refs[name] = ref
		}
		raw, err := data.JSON()
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), `"href":"\u0001redis.c#L1277"`) || !strings.Contains(string(raw), `"shared"`) ||
			!strings.Contains(string(raw), `{"at":"redis.c:1277","caller":0,"from":1,"to":1}`) || strings.Contains(string(raw), `"caller_name":"serverCron"`) ||
			!strings.Contains(string(raw), `"caller_name":"one of two anonymous functions in serverCron"`) || !strings.Contains(string(raw), `"callee_name":"netdb.h.gethostbyname"`) ||
			!strings.Contains(string(raw), `"key":":5:function:serverCron"`) || !strings.Contains(string(raw), `"decl_key":":40:function:cronHelper"`) ||
			!strings.Contains(string(raw), `":16:type:listNode"`) || strings.Contains(string(raw), `adlist.h:36:16`) || !strings.Contains(string(raw), `{"d":0,"kind":"function","line":1250,"text":"(): int"}`) {
			t.Fatalf("the data is not compact: %s", raw)
		}
		script := systemJSPiece(t, "10-ui.js", "var rmPage = (function () {", "})();") + "})();\n"
		checks := ""
		for name, ref := range refs {
			expected := values[name]
			if name == "reading" {
				expected = `(()=>{const v=` + values[name] + `;return v;})()`
			}
			checks += "assert.deepEqual(page_.data({dataset:{" + name + ":'" + ref + "'}},'" + name + "')," + expected + ",'" + name + " on " + host.base + "');\n"
		}
		runSystemJS(t, `const document={getElementById(){return {textContent:String.raw`+"`"+string(raw)+"`"+`};}};
`+strings.Replace(script, "var rmPage", "var page_", 1)+checks)
	}
}
