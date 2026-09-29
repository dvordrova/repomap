/* dump: prints the keys of a kvd snapshot file. The Makefile does not build
 * it; build it by hand with `cc -o dump tools/dump.c strbuf.c`. */
#include <assert.h>
#include <stdio.h>
#include <string.h>

#include "../strbuf.h"

/* The name the tool was run by, for its error messages. */
static const char *progname = "dump";

int main(int argc, char **argv) {
    const char *filename = argc > 1 ? argv[1] : "dump.kv";
    FILE *fp = fopen(filename, "r");
    char line[1024];
    strbuf keys;

    progname = argv[0];
    if (fp == NULL) {
        fprintf(stderr, "%s: ", progname);
        perror(filename);
        return 1;
    }
    sbInit(&keys);
    /* assert is the platform's macro: its expansion's calls (and any
     * compiler builtin it uses) are one call as written, `assert`. */
    assert(keys.len == 0);
    while (fgets(line, sizeof line, fp) != NULL) {
        char *space = strchr(line, ' ');
        if (space == NULL) continue;
        sbAppend(&keys, line, (size_t)(space - line));
        sbAppend(&keys, "\n", 1);
    }
    fclose(fp);
    /* Another program sorts the keys: the one word popen is given is the
     * whole command line it runs. */
    FILE *sorted = popen("sort -u", "w");
    fputs(keys.buf, sorted != NULL ? sorted : stdout);
    if (sorted != NULL) pclose(sorted);
    sbFree(&keys);
    return 0;
}
