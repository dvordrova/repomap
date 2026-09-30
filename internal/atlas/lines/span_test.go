package lines

import (
	"strings"
	"testing"
)

// at is the line and 1-based byte column of the first occurrence of mark in
// src, the position an adapter records for a call.
func at(t *testing.T, src, mark string) (int, int) {
	t.Helper()
	offset := strings.Index(src, mark)
	if offset < 0 {
		t.Fatalf("no %q in the source", mark)
	}
	line := strings.Count(src[:offset], "\n") + 1
	return line, offset - strings.LastIndex(src[:offset], "\n")
}

// The call at an adapter's position is sent as written, with its receiver
// chain and arguments, comments dropped and whitespace folded, in every
// language family: C and Go anchor at the name or its parenthesis, Python
// and JavaScript at the selector after the dot, Clojure at the form.
func TestCallTextIsTheCallAsWritten(t *testing.T) {
	cases := []struct {
		name, path, src, mark, want string
	}{
		{"a trailing comment is not the call", "kvd.c", "int main(void) {\n    x = strcmp(a,\"b\"); /* why */\n}\n", "strcmp", `strcmp(a,"b")`},
		{"a comment inside the call is dropped", "kvd.c", "int f(void) {\n    return fprintf(stderr, /* where */ \"%s\\n\", what);\n}\n", "fprintf", `fprintf(stderr, "%s\n", what)`},
		{"C member call", "loop.c", "void g(void) {\n    fe->rfileProc(loop, fd);\n}\n", "rfileProc", "fe->rfileProc(loop, fd)"},
		{"a table row around its anchor", "kvd.c", "static kvCommand cmdTable[] = {\n    {\"get\", getCommand, 2},\n    {\"set\", setCommand, 3},\n};\n", "setCommand", `{"set", setCommand, 3}`},
		{"an assignment in a function body is its statement", "kvd.c", "void h(void) {\n    struct sigaction act;\n    act.sa_handler = onSignal;\n}\n", "sa_handler", "act.sa_handler = onSignal"},
		{"Go anchors at the parenthesis", "main.go", "package main\n\nvar verbose = flag.Bool(\"verbose\", false, \"log more\") // the flag\n", "(\"verbose\"", `flag.Bool("verbose", false, "log more")`},
		{"a multi-line Go call folds to one line", "list.go", "func run() {\n\tsocket := fs.String(\n\t\t\"socket\", // the path\n\t\t\"/var/run/x.sock\",\n\t\t\"control socket path\",\n\t)\n}\n", "(\n\t\t\"socket\"", `fs.String("socket", "/var/run/x.sock", "control socket path",)`},
		{"a Go assignment is its statement", "list.go", "func run() {\n\tfs := flag.NewFlagSet(\"x\", 0)\n\tfs.Usage = c.Usage\n\tfs.Parse(args)\n}\n", "Usage =", "fs.Usage = c.Usage"},
		{"a Go composite literal element is its row", "log.go", "func run() {\n\topts := &tint.Options{\n\t\tLevel:       level,\n\t\tReplaceAttr: ReplaceAttr,\n\t}\n\t_ = opts\n}\n", "ReplaceAttr:", "{Level: level, ReplaceAttr: ReplaceAttr,}"},
		{"Python decorator", "cli.py", "app = FastAPI()\n\n@app.get(\"/api/levels\")  # list them\ndef levels():\n    pass\n", "get(", `app.get("/api/levels")`},
		{"Python call on a call's result", "events.py", "KafkaConsumer().subscribe(\"orders.chained\", handle).subscribe(\"orders.chained\", record)\n", "subscribe(\"orders.chained\", handle)", `KafkaConsumer().subscribe("orders.chained", handle)`},
		{"Python triple-quoted text stays as written", "cli.py", "parser.add_argument(\"-v\", help=\"\"\"say\n  more\"\"\")  # verbose\n", "add_argument", "parser.add_argument(\"-v\", help=\"\"\"say\n  more\"\"\")"},
		{"a JavaScript chain keeps the calls before it", "cli.ts", "program.option(\"-p, --port <n>\") // port\n  .action(run)\n", "action", `program.option("-p, --port <n>") .action(run)`},
		{"the first call of a JavaScript chain stops at its own parenthesis", "cli.ts", "program.option(\"-p, --port <n>\").action(run)\n", "option", `program.option("-p, --port <n>")`},
		{"a JavaScript template literal is one string", "api.ts", "fetch(`/items/${id}`, { method: \"POST\" })\n", "fetch", "fetch(`/items/${id}`, { method: \"POST\" })"},
		{"a Clojure form", "core.clj", "(defn f [dir]\n  (format \"create %s dir\" dir)) ; comment\n", "(format", `(format "create %s dir" dir)`},
		{"a Clojure character is not a paren", "core.clj", "(str/join \\( [\"a\" \"b\"]) ; x\n", "(str/join", `(str/join \( ["a" "b"])`},
	}
	for _, c := range cases {
		line, column := at(t, c.src, c.mark)
		if got := CallText([]byte(c.src), c.path, line, column); got != c.want {
			t.Errorf("%s: CallText = %q, want %q", c.name, got, c.want)
		}
	}
}

// A file no lexer here reads sends no call, and a position past the file
// sends none either.
func TestCallTextSendsNothingItCannotRead(t *testing.T) {
	src := "do_something(\"x\")\n"
	for _, path := range []string{"script.rb", "Makefile", "notes.txt"} {
		if got := CallText([]byte(src), path, 1, 1); got != "" {
			t.Fatalf("%s: CallText = %q, want nothing", path, got)
		}
	}
	if got := CallText([]byte(src), "x.py", 9, 1); got != "" {
		t.Fatalf("a line past the file gave %q", got)
	}
}

// A call written over many lines is sent whole: no line or byte count
// shortens the evidence.
func TestCallTextKeepsALongCallWhole(t *testing.T) {
	var src strings.Builder
	src.WriteString("func run() {\n\tmux.HandleFunc(\"/x\", func(w http.ResponseWriter, r *http.Request) {\n")
	for i := 0; i < 40; i++ {
		src.WriteString("\t\tw.Write([]byte(\"line\"))\n")
	}
	src.WriteString("\t})\n}\n")
	line, column := at(t, src.String(), "(\"/x\"")
	got := CallText([]byte(src.String()), "server.go", line, column)
	if !strings.HasPrefix(got, `mux.HandleFunc("/x", func(`) || strings.Count(got, `w.Write([]byte("line"))`) != 40 || !strings.HasSuffix(got, "})") {
		t.Fatalf("the long call was not sent whole: %q", got)
	}
}

// Index and call groups written against a name stay in the chain, a
// generic call reaches its parenthesis, and JavaScript's new stays with the
// constructor it calls.
func TestCallTextFollowsIndexesGenericsAndNew(t *testing.T) {
	cases := []struct{ path, src, mark, want string }{
		{"platform.ts", "const head = path.split(\"/\")[0].split(\"/\")\n", "split(\"/\")\n", `path.split("/")[0].split("/")`},
		{"platform.ts", "const all = [new Date(), new Promise<void>((resolve) => resolve())]\n", "Promise", "new Promise<void>((resolve) => resolve())"},
		{"sort.go", "func f() {\n\tslices.SortFunc[[]int](xs, cmp)\n}\n", "SortFunc", "slices.SortFunc[[]int](xs, cmp)"},
	}
	for _, c := range cases {
		line, column := at(t, c.src, c.mark)
		if got := CallText([]byte(c.src), c.path, line, column); got != c.want {
			t.Errorf("%s: CallText = %q, want %q", c.path, got, c.want)
		}
	}
}

// A table's row is sent as its own element of the table, bounded by the
// other rows' words: a Python dict's row from its key (CallText gave the
// whole dict: 19.7 KB for each of freqtrade's 124 option rows), a list's
// call or word, and a C row's own braces as CallText gives them.
func TestRowTextIsTheRowAsWritten(t *testing.T) {
	cases := []struct {
		name, path, src, mark string
		others               []string
		want                 string
	}{
		{"a dict row from its key", "options.py", "OPTIONS = {\n    \"verbose\": Opt(\"-v\", \"--verbose\", help=\"print more\"),  # loud\n    \"force\": Opt(\"-f\", \"--force\", help=\"overwrite files\"),\n}\n",
			`"force"`, []string{`"print more"`}, `"force": Opt("-f", "--force", help="overwrite files")`},
		{"the first dict row", "options.py", "OPTIONS: dict[str, Opt] = {\n    \"verbose\": Opt(\n        \"-v\",\n        \"--verbose\",\n    ),\n    \"force\": Opt(\"-f\"),\n}\n",
			`"verbose"`, []string{`"force"`}, `"verbose": Opt("-v", "--verbose",)`},
		{"a list row of calls", "options.py", "OPTIONS = [\n    Opt(\"-v\", \"--verbose\"),\n    Opt(\"-f\", \"--force\"),\n]\n",
			`"-f"`, []string{`"--verbose"`}, `Opt("-f", "--force")`},
		{"a list row of words", "arguments.py", "ARGS = [\"db_url\", \"sd_notify\", \"fee\"]\n",
			`"sd_notify"`, []string{`"db_url"`, `"fee"`}, `"sd_notify"`},
		{"a C row's own braces", "kvd.c", "static kvCommand cmdTable[] = {\n    {\"get\", getCommand, 2},\n    {\"set\", setCommand, 3},\n};\n",
			`"set"`, []string{`2}`}, `{"set", setCommand, 3}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			line, column := at(t, c.src, c.mark)
			var others [][2]int
			for _, other := range c.others {
				otherLine, otherColumn := at(t, c.src, other)
				others = append(others, [2]int{otherLine, otherColumn})
			}
			if got := NewCallFile([]byte(c.src), c.path).RowText(line, column, others); got != c.want {
				t.Fatalf("RowText = %q, want %q", got, c.want)
			}
		})
	}
}
