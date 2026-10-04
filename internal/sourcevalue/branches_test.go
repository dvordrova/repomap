package sourcevalue

import "testing"

func TestOutsideItsBranchKeepsWhatItCannotRuleOut(t *testing.T) {
	cases := []Compared{{Position: 1, Name: "key", Cases: []ComparedCase{
		{Words: []string{"staticBaseUrl"}, Line: 15, EndLine: 17, Exclusive: true},
		{Words: []string{"logConfig"}, Line: 17, EndLine: 19, Exclusive: true},
	}}}
	given := func(word string, known bool) func(int, string) (string, bool) {
		return func(int, string) (string, bool) { return word, known }
	}
	// func pick(key string) string { if key == "x" { return "https://x" }; return "https://default" }
	oneLine := []Compared{{Position: 1, Cases: []ComparedCase{{Words: []string{"x"}, Line: 9, Column: 46, EndLine: 9, EndColumn: 68, Exclusive: true}}}}
	at := func(line int) *Anchor { return &Anchor{Path: "conf.go", Line: line} }
	for name, check := range map[string]struct {
		anchor   *Anchor
		path     string
		compared []Compared
		supplied func(int, string) (string, bool)
		want     bool
	}{
		"another key's branch":                   {at(16), "conf.go", cases, given("dataSourceName", true), true},
		"its own branch":                         {at(16), "conf.go", cases, given("staticBaseUrl", true), false},
		"an unknown key":                         {at(16), "conf.go", cases, given("", false), false},
		"the line of two branches":               {at(17), "conf.go", cases, given("dataSourceName", true), false},
		"outside every branch":                   {at(20), "conf.go", cases, given("dataSourceName", true), false},
		"another file":                           {at(16), "main.go", cases, given("dataSourceName", true), false},
		"no anchor":                              {nil, "conf.go", cases, given("dataSourceName", true), false},
		"a case not known exclusive":             {at(16), "conf.go", []Compared{{Position: 1, Cases: []ComparedCase{{Words: []string{"nu"}, Line: 15, EndLine: 17}}}}, given("en", true), false},
		"one line, outside the branch's columns": {&Anchor{Path: "conf.go", Line: 9, Column: 70}, "conf.go", oneLine, given("y", true), false},
		"one line, inside the branch's columns":  {&Anchor{Path: "conf.go", Line: 9, Column: 50}, "conf.go", oneLine, given("y", true), true},
		"one line, columns not known":            {&Anchor{Path: "conf.go", Line: 9, Column: 40}, "conf.go", []Compared{{Position: 1, Cases: []ComparedCase{{Words: []string{"x"}, Line: 9, EndLine: 9, Exclusive: true}}}}, given("y", true), false},
		"a value that is no parameter":           {at(16), "conf.go", []Compared{{Cases: []ComparedCase{{Words: []string{"nu"}, Line: 15, EndLine: 17, Exclusive: true}}}}, given("en", true), false},
	} {
		if got := OutsideItsBranch(check.anchor, check.path, check.compared, check.supplied); got != check.want {
			t.Errorf("%s: %v, want %v", name, got, check.want)
		}
	}
}
