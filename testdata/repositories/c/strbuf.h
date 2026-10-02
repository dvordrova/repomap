#ifndef STRBUF_H
#define STRBUF_H

#include <stddef.h>

/* A growable byte string that always ends in a NUL byte. */
typedef struct {
    char *buf;
    size_t len;
    size_t cap;
} strbuf;

void sbInit(strbuf *sb);
void sbAppend(strbuf *sb, const char *data, size_t len);
void sbConsume(strbuf *sb, size_t len);
void sbFree(strbuf *sb);

/* Reserve room for len more bytes: a size of zero or past the limit fails. */
void sbReserve(strbuf *sb, size_t len);

/* Drain the buffer for ever: declared noreturn, it loops and never fails. */
_Noreturn void sbDrainForever(strbuf *sb);

/* Free bytes left before the buffer has to grow. */
static inline size_t sbAvail(const strbuf *sb) {
    return sb->cap - sb->len;
}

#endif
