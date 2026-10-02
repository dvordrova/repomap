#ifndef WIRE_INTERNAL_H
#define WIRE_INTERNAL_H

/* Shared by the library's own files: whether a character needs escaping.
 * Programs using libwire.a include wire.h only. */
int wireNeedsEscape(char c);

#endif
