package cproject

import (
	"fmt"
	"reflect"
	"slices"
	"testing"

	"github.com/dvordrova/repomap/internal/programindex"
)

const readsSource = `struct config { int port; const char *name; };
static struct config config = {7379, "kv"};
static int counter;
int shared[4];
static const char *names[] = {"a", "b"};
#define PORT config.port
#define TWICE(x) ((x) + (x))

static int bump(void) {
    counter = 0;
    counter++;
    return counter;
}
static void setPort(int port) { config.port = port; }
static int size(void) { return (int)sizeof(names) + (int)sizeof counter; }
static const char *first(void) { return names[0]; }
static int *address(void) { return &shared[1]; }
static int local(int counter) { static int calls; int names = 2; calls++; return counter + calls + names; }
static int port(void) { return PORT; }
static int twice(void) { return TWICE(counter); }

int main(void) {
    extern int shared[4];
    setPort(1);
    return bump() + size() + shared[0] + (first() != 0) + (address() != 0) + local(3) + port() + twice();
}
`

// A function that names a file-scope variable reads it, at each place it
// names it: its value, a member or an element of it, or its address. The
// variable itself as the destination of =, an operand sizeof never
// evaluates, a parameter, a local and a static local are no read.
func TestIndexReadsFileScopeVariables(t *testing.T) {
	x := indexProgram(t, map[string]string{"reads.c": readsSource}, "c:reads.c")
	got := map[string][]string{}
	for _, relation := range x.index.Relations {
		// A field's reads name the field (fields_test.go).
		if relation.Kind != programindex.RelationReads || relation.FieldPath != "" {
			continue
		}
		if relation.Resolution != programindex.ResolutionExact || len(relation.ToIDs) != 1 || relation.Location == nil || len(relation.Witnesses) == 0 || relation.Witnesses[0].Kind != "c_variable_read" {
			t.Fatalf("read: %+v", relation)
		}
		from := x.byID(relation.FromID).Name
		got[from] = append(got[from], fmt.Sprintf("%s@%d:%d", x.byID(relation.ToIDs[0]).Name, relation.Location.Line, relation.Location.Column))
	}
	for _, sites := range got {
		slices.Sort(sites)
	}
	want := map[string][]string{
		"bump":    {"counter@11:5", "counter@12:12"},
		"setPort": {"config@14:33"},
		"first":   {"names@16:41"},
		"address": {"shared@17:37"},
		"port":    {"config@19:32"},
		"twice":   {"counter@20:39"},
		"main":    {"shared@25:30"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("reads:\n have %v\n want %v", got, want)
	}
	// A read a macro body writes is sited at the macro use and says so.
	portFn := x.object(t, "port", "reads.c")
	reads := x.relations(programindex.RelationReads, portFn.ID, "")
	read := reads[slices.IndexFunc(reads, func(r programindex.Relation) bool { return r.FieldPath == "" })]
	if len(read.Witnesses) != 2 || read.Witnesses[1].Kind != "macro_expansion" || read.Witnesses[1].Detail != "PORT expands to a read of config" {
		t.Fatalf("PORT: %+v", read.Witnesses)
	}
}
