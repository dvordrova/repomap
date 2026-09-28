package cproject

import (
	"fmt"
	"reflect"
	"slices"
	"testing"

	"github.com/dvordrova/repomap/internal/programindex"
)

const fieldsSource = `#include <signal.h>
struct dict { int used; };
typedef struct redisDb { struct dict *dict; struct dict *expires; int id; } redisDb;
struct stats { long hits; };
struct server {
    char *masterhost;
    int replstate;
    redisDb *db;
    redisDb dbs[2];
    struct stats stat;
    char buf[8];
    char **argv;
};
typedef struct client { redisDb *db; } client;
struct server server;
#define dictSize(d) ((d)->used)
#define REPLSTATE server.replstate

static void setMaster(const char *host) {
    server.masterhost = (char *)host;
    server.replstate = 1;
}
static int cron(void) {
    if (server.masterhost && REPLSTATE == 1) server.replstate = 2;
    server.stat.hits++;
    server.stat.hits += 2;
    server.dbs[0].id = 3;
    server.argv[0] = 0;
    server.buf[0] = 'a';
    return (int)sizeof(server.replstate) + dictSize(server.db->expires);
}
static struct dict *expiresOf(redisDb *db) { return db->expires; }
static void setExpires(client *c, struct dict *d) {
    c->db->expires = d;
    (*c->db).id = 4;
}
static struct dict *first(void) {
    redisDb *db = &server.dbs[1];
    return db->dict;
}
static void handler(int sig) { (void)sig; }
static void signals(void) {
    struct sigaction act;
    act.sa_flags = 0;
    act.sa_handler = handler;
    sigaction(SIGTERM, &act, 0);
}

int main(void) {
    client c;
    setMaster("h");
    setExpires(&c, expiresOf(server.db));
    signals();
    return cron() + (first() != 0);
}
`

// A function reads or writes each field of a repository record it names, at
// the field as written, with the field's path from its root: the file-scope
// variable, or the record holding the chain's first field for any other
// value. The destination of =, a compound assignment, ++ and -- is written;
// the record a . member is taken from is passed through; the pointer a ->
// member or an indexed pointer member holds is read; sizeof reads nothing and
// a platform record's member is no repository field.
func TestIndexReadsAndWritesRecordFields(t *testing.T) {
	x := indexProgram(t, map[string]string{"fields.c": fieldsSource}, "c:fields.c")
	got := map[string][]string{}
	for _, relation := range x.index.Relations {
		if relation.FieldPath == "" {
			if relation.Kind == programindex.RelationWrites {
				t.Fatalf("write without its field path: %+v", relation)
			}
			continue
		}
		witness := "c_field_read"
		if relation.Kind == programindex.RelationWrites {
			witness = "c_field_write"
		}
		if relation.Resolution != programindex.ResolutionExact || len(relation.ToIDs) != 1 || relation.Location == nil ||
			!slices.ContainsFunc(relation.Witnesses, func(w programindex.Witness) bool { return w.Kind == witness }) {
			t.Fatalf("field access: %+v", relation)
		}
		field := x.byID(relation.ToIDs[0])
		from := x.byID(relation.FromID).Name
		got[from] = append(got[from], fmt.Sprintf("%s %s=%s.%s@%d:%d", relation.Kind, relation.FieldPath,
			x.byID(field.OwnerID).Name, field.Name, relation.Location.Line, relation.Location.Column))
	}
	for _, sites := range got {
		slices.Sort(sites)
	}
	want := map[string][]string{
		"setMaster": {
			"writes server.masterhost=server.masterhost@20:12",
			"writes server.replstate=server.replstate@21:12",
		},
		"cron": {
			"reads server.argv=server.argv@28:12",
			// dictSize's body reads the field of what its argument names.
			"reads server.db.expires.used=dict.used@30:44",
			"reads server.db.expires=redisDb.expires@30:64",
			"reads server.db=server.db@30:60",
			"reads server.masterhost=server.masterhost@24:16",
			"reads server.replstate=server.replstate@24:30",
			"writes server.buf=server.buf@29:12",
			"writes server.dbs.id=redisDb.id@27:19",
			"writes server.replstate=server.replstate@24:53",
			"writes server.stat.hits=stats.hits@25:17",
			"writes server.stat.hits=stats.hits@26:17",
		},
		"expiresOf": {"reads redisDb.expires=redisDb.expires@32:57"},
		"setExpires": {
			"reads client.db=client.db@34:8",
			"reads client.db=client.db@35:10",
			"writes client.db.expires=redisDb.expires@34:12",
			"writes redisDb.id=redisDb.id@35:14",
		},
		"first": {
			"reads redisDb.dict=redisDb.dict@39:16",
			"reads server.dbs=server.dbs@38:27",
		},
		"main": {"reads server.db=server.db@52:37"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("field accesses:\n have %v\n want %v", got, want)
	}
	// A field a macro body names is sited at the macro use and says so.
	cron := x.object(t, "cron", "fields.c")
	for _, relation := range x.relations(programindex.RelationReads, cron.ID, "") {
		if relation.FieldPath == "server.replstate" && relation.Location.Line == 24 && relation.Location.Column == 30 &&
			(len(relation.Witnesses) != 2 || relation.Witnesses[0].Detail != "REPLSTATE expands to a read of server.replstate") {
			t.Fatalf("REPLSTATE: %+v", relation.Witnesses)
		}
		if relation.FieldPath == "server.db.expires" && (len(relation.Witnesses) != 1 || relation.Witnesses[0].Detail != "read of server.db.expires") {
			t.Fatalf("dictSize's argument: %+v", relation.Witnesses)
		}
	}
}
