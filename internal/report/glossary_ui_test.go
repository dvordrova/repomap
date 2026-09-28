package report

import (
	"bytes"
	"html/template"
	"strings"
	"testing"
)

// Equal names in separate sources remain separately addressable without JS.
// Explicit answer terms must work even when no code concept was selected.
func TestGlossaryHTMLKeepsAnswerTermsAndSourceDistinctDefinitions(t *testing.T) {
	for _, language := range []DisplayLanguage{English, Russian} {
		t.Run(string(language), func(t *testing.T) {
			view := &pageView{Glossary: []pageGlossaryTerm{
				{ID: "term-first", Name: "Разбор", OriginalName: "Parsing", Explanation: "First explanation with <source> intact.",
					Occurrences: []pageAnchor{{Path: "first.py", Line: 12, Text: "first.py:12", Href: "https://example.test/first.py#L12"}, {Path: "first.py", Line: 18, Text: "first.py:18", Href: "https://example.test/first.py#L18"}},
					Questions:   []pageLearnLink{{Title: "How does the first parser work?", Href: "#question-first"}},
					Places:      []pageLearnLink{{Title: "First parser", Href: "#part-first"}}},
				{ID: "term-second", Name: "Разбор", OriginalName: "Parsing", Explanation: "A separate definition from another source.",
					Occurrences: []pageAnchor{{Path: "second.py", Line: 30, Text: "second.py:30", Href: "https://example.test/second.py#L30"}},
					Questions:   []pageLearnLink{{Title: "How does the second parser work?", Href: "#question-second"}},
					Places:      []pageLearnLink{{Title: "Second parser", Href: "#part-second"}}},
			}}
			templates, err := template.New("report").Funcs(pageTemplateFuncs(language)).ParseFS(reportTemplateFS, "templates/html/*.html")
			if err != nil {
				t.Fatal(err)
			}
			var html bytes.Buffer
			if err := templates.ExecuteTemplate(&html, "concepts.html", view); err != nil {
				t.Fatal(err)
			}
			for _, term := range view.Glossary {
				if strings.Count(html.String(), `id="`+term.ID+`"`) != 1 {
					t.Fatalf("term %s lost its separate address", term.ID)
				}
				for _, source := range term.Occurrences {
					if strings.Count(html.String(), `href="`+source.Href+`"`) != 1 || !strings.Contains(html.String(), source.Text) {
						t.Fatalf("term %s lost source %+v", term.ID, source)
					}
				}
				for _, link := range append(term.Questions, term.Places...) {
					if strings.Count(html.String(), `href="`+link.Href+`"`) != 1 {
						t.Fatalf("term %s lost destination %s", term.ID, link.Href)
					}
				}
			}
			for _, preserved := range []string{"First explanation with &lt;source&gt; intact.", "A separate definition from another source.", `data-term-original="Parsing"`, `class="learn-concept"`, `class="concept-search"`, `class="concept-pages"`} {
				if !strings.Contains(html.String(), preserved) {
					t.Fatalf("static glossary lost %q", preserved)
				}
			}
			if strings.Contains(html.String(), `class="term-mention"`) {
				t.Fatal("glossary definitions contain nested inline term controls")
			}
			if strings.Count(html.String(), `<code>first.py</code>`) != 1 ||
				strings.Count(html.String(), `<details class="glossary-source-file">`) != 2 ||
				strings.Count(html.String(), `<details class="glossary-sources">`) != 2 ||
				strings.Contains(html.String(), `<p class="model glossary-explanation"><span`) {
				t.Fatal("glossary repeats paths, expands its context by default, or prefixes the explanation with a badge")
			}
			if strings.Contains(html.String(), "glossary-comparison-note") {
				t.Fatal("complete glossary carries an incomplete-comparison notice")
			}
			view.GlossaryPartialComparison = true
			html.Reset()
			if err := templates.ExecuteTemplate(&html, "concepts.html", view); err != nil {
				t.Fatal(err)
			}
			notice, err := uiText(language, "Some explanations were not compared together; similar entries may remain separate.")
			if err != nil || !strings.Contains(html.String(), notice) || strings.Index(html.String(), notice) > strings.Index(html.String(), `class="concept-results"`) {
				t.Fatal("partial glossary hides its localized notice behind term expansion")
			}
		})
	}
}

// A term's card opens on a click, or once the pointer has rested on the term
// for 600 ms, never as the pointer passes (a Redis reader's pointer crossing
// "Redis" in Main flow's caption opened it over the canvas twice).
func TestATermCardOpensOnlyOnAClickOrADeliberatePause(t *testing.T) {
	preview := systemJSPiece(t, "25-preview.js", "var repomapPreview = (function () {", "\n(function () {")
	glossary, err := reportTemplateFS.ReadFile("templates/js/46-glossary.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(glossary), "hoverDelay:600") {
		t.Fatal("the term cards do not wait for a deliberate pause")
	}
	runSystemJS(t, `
class Element{}
globalThis.Element=Element;
const doc={body:{appendChild(){}},addEventListener(){}};globalThis.document=doc;globalThis.window={addEventListener(){},innerWidth:1000,innerHeight:800};
function node(extra){const n=Object.assign(new Element(),{listeners:{},hovered:false,parentElement:null,classList:{add(){},remove(){}},dataset:{},hidden:true,
  addEventListener(kind,f){(this.listeners[kind]||=[]).push(f);},fire(kind,event={}){(this.listeners[kind]||[]).forEach(f=>f({type:kind,target:this,currentTarget:this,clientX:0,clientY:0,...event}));},
  matches(selector){return selector.includes(':hover')&&this.hovered;},setAttribute(){},dispatchEvent(){},getBoundingClientRect(){return {left:0,right:10,top:0,bottom:10};},closest(){return null;},contains(other){return other===this;},focus(){},offsetWidth:10,offsetHeight:10,style:{}},extra);return n;}
`+preview+`
const trigger=node(),card=node({parentNode:{}});let shown=0;
repomapPreview.bind(trigger,card,()=>shown++,{pinOnClick:true,hoverDelay:600});
(async()=>{
 trigger.hovered=true;trigger.fire('mouseenter');
 await new Promise(r=>setTimeout(r,200));trigger.hovered=false;trigger.fire('mouseleave');
 await new Promise(r=>setTimeout(r,600));
 assert.equal(shown,0,'a pointer passing over the term opens nothing');
 trigger.hovered=true;trigger.fire('mouseenter');
 await new Promise(r=>setTimeout(r,650));
 assert.equal(shown,1,'a pause of 600 ms opens it');
 trigger.fire('click');
 assert.equal(card.hidden,false,'a click pins it');
})().catch(error=>{console.error(error);process.exit(1);});
`)
}
