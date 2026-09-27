"""A package facade: what its rates module binds, then its own helpers."""


def shadowed(day):
    return "facade " + str(day)


from .rates import *


def to_text(day):
    return "day " + str(day)
