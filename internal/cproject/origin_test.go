package cproject

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/sourcevalue"
)

const originMakefile = `CFLAGS = -std=c99 -Wall

all: cfg

cfg: cfg.o
	$(CC) -o cfg cfg.o

.c.o:
	$(CC) -c $(CFLAGS) $<
`

const originSource = `#include <string.h>
#include <strings.h>
#include <stdio.h>

struct client { char **argv; };

static char *parts[4];

static char **split(const char *line, int *count) { (void)line; *count = 1; return parts; }

static int load(const char *line) {
    char **words;
    int n;
    words = split(line, &n);
    if (!strcasecmp(words[0], "port")) return 1;
    return 0;
}

static int handle(struct client *c) { return strcasecmp(c->argv[0], "quit"); }

static int options(int argc, char **argv) {
    int i;
    for (i = 1; i < argc; i++) {
        if (!strcmp(argv[i], "-h")) i++;
    }
    return i;
}

static int pick(int argc) {
    const char *mode;
    if (argc > 1) mode = "fast"; else mode = "slow";
    return puts(mode);
}

int main(int argc, char **argv) {
    struct client c = {argv};
    return load(argv[0]) + handle(&c) + options(argc, argv) + pick(argc);
}
`

// An argument is recorded as what it is: an element of what a local holds,
// a field of a parameter, an element whose index a loop counts, and a
// local written on either branch. strcasecmp(words[0], "port") reads a line
// the program split, not its argument vector.
func TestArgumentOriginsFollowElementsFieldsAndLocals(t *testing.T) {
	x := indexProgram(t, map[string]string{"Makefile": originMakefile, "cfg.c": originSource}, "c:cfg")
	origin := func(function, selector string) *sourcevalue.Value {
		t.Helper()
		relation := x.one(t, programindex.RelationInvokesExternal, x.object(t, function, "cfg.c").ID, selector)
		arguments := relation.Patterns[0].Arguments
		if len(arguments) == 0 || arguments[0].Origin == nil {
			t.Fatalf("%s %s arguments: %+v", function, selector, arguments)
		}
		return arguments[0].Origin
	}
	shape := func(value *sourcevalue.Value) string {
		copied := sourcevalue.Clone(value)
		var strip func(*sourcevalue.Value)
		strip = func(v *sourcevalue.Value) {
			v.Anchor, v.Owner = nil, nil
			for i := range v.Parts {
				strip(&v.Parts[i])
			}
		}
		strip(copied)
		var raw strings.Builder
		encoder := json.NewEncoder(&raw)
		encoder.SetEscapeHTML(false)
		_ = encoder.Encode(copied)
		return strings.TrimSpace(raw.String())
	}
	for _, test := range []struct{ function, selector, want string }{
		{"load", "strcasecmp", `{"kind":"index","text":"words[0]","parts":[{"kind":"call_result","text":"split(line, &n)"},{"kind":"literal","text":"0"}]}`},
		{"handle", "strcasecmp", `{"kind":"index","text":"c->argv[0]","parts":[{"kind":"field","text":"argv","parts":[{"kind":"parameter","text":"c","position":1}]},{"kind":"literal","text":"0"}]}`},
		{"options", "strcmp", `{"kind":"index","text":"argv[i]","parts":[{"kind":"parameter","text":"argv","position":2},{"kind":"alternatives","parts":[{"kind":"literal","text":"1"},{"kind":"unknown","text":"i++"}]}]}`},
		{"pick", "puts", `{"kind":"alternatives","parts":[{"kind":"literal","text":"fast"},{"kind":"literal","text":"slow"},{"kind":"unknown","text":"mode"}]}`},
	} {
		value := origin(test.function, test.selector)
		if err := sourcevalue.Validate(value); err != nil {
			t.Fatalf("%s: %v", test.function, err)
		}
		if got := shape(value); got != test.want {
			t.Fatalf("%s %s origin:\n got %s\nwant %s", test.function, test.selector, got, test.want)
		}
	}
}
