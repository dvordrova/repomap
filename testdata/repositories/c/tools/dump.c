/* dump: prints the keys of a kvd snapshot file. The Makefile does not build
 * it; build it by hand with `cc -o dump tools/dump.c strbuf.c`. */
#include <stdio.h>
#include <string.h>

#include "../strbuf.h"

int main(int argc, char **argv) {
    const char *filename = argc > 1 ? argv[1] : "dump.kv";
    FILE *fp = fopen(filename, "r");
    char line[1024];
    strbuf keys;

    if (fp == NULL) {
        perror(filename);
        return 1;
    }
    sbInit(&keys);
    while (fgets(line, sizeof line, fp) != NULL) {
        char *space = strchr(line, ' ');
        if (space == NULL) continue;
        sbAppend(&keys, line, (size_t)(space - line));
        sbAppend(&keys, "\n", 1);
    }
    fclose(fp);
    fputs(keys.buf, stdout);
    sbFree(&keys);
    return 0;
}
