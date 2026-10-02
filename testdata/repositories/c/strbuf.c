/* strbuf.c - growable byte strings shared by kvd, kvcli and the dump tool.
 *
 * Copyright (c) 2026, the kvd authors. All rights reserved.
 * Redistribution and use in source and binary forms, with or without
 * modification, are permitted under the terms of the BSD licence.
 */
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "strbuf.h"

/* loop.c has its own oom: each is static to its file. */
static void oom(const char *what) {
    fprintf(stderr, "strbuf: out of memory growing %s\n", what);
    abort();
}

void sbInit(strbuf *sb) {
    sb->buf = malloc(16);
    if (sb->buf == NULL) oom("a new buffer");
    sb->buf[0] = '\0';
    sb->len = 0;
    sb->cap = 15;
}

void sbAppend(strbuf *sb, const char *data, size_t len) {
    if (sbAvail(sb) < len) {
        size_t cap = (sb->len + len) * 2;
        char *grown = realloc(sb->buf, cap + 1);
        if (grown == NULL) oom("a buffer");
        sb->buf = grown;
        sb->cap = cap;
    }
    memcpy(sb->buf + sb->len, data, len);
    sb->len += len;
    sb->buf[sb->len] = '\0';
}

void sbConsume(strbuf *sb, size_t len) {
    if (len > sb->len) len = sb->len;
    memmove(sb->buf, sb->buf + len, sb->len - len);
    sb->len -= len;
    sb->buf[sb->len] = '\0';
}

void sbFree(strbuf *sb) {
    free(sb->buf);
    sb->buf = NULL;
    sb->len = sb->cap = 0;
}

/* The most bytes one reservation may ask for. */
#define SB_LIMIT (1u << 20)

static void sbTrace(const strbuf *sb, const char *what) {
    fprintf(stderr, "strbuf at %zu: %s\n", sb->len, what);
}

/* sbCheckOrAbort returns when its check holds and aborts otherwise: with a
 * path that returns it is no function that never returns, so its call runs
 * unguarded and what follows it runs (`if (ok) return; abort();`). */
static void sbCheckOrAbort(int ok) {
    if (ok) return;
    abort();
}

/* sbReserve fails on a size of zero or past the limit: oom ends in abort,
 * so its calls, and what the arm ending in one does first, run only on a
 * failing path. Draining for ever is declared noreturn but loops, so its
 * call is an ordinary branch; growing is unguarded. */
void sbReserve(strbuf *sb, size_t len) {
    if (len == 0) oom("an empty reservation");
    if (len > SB_LIMIT) {
        sbTrace(sb, "past the limit");
        oom("a reservation past the limit");
    } else {
        sbTrace(sb, "within the limit");
    }
    if (len == SB_LIMIT) sbDrainForever(sb);
    sbCheckOrAbort(sb->buf != NULL);
    sbAppend(sb, "", 0);
}

_Noreturn void sbDrainForever(strbuf *sb) {
    for (;;) sbConsume(sb, sb->len);
}
