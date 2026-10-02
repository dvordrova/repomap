"""Shared by the package's own modules. The leading underscore of the module's
name keeps score_text out of the library's API, although its own name is
public."""


def score_text(level):
    return str(level)
