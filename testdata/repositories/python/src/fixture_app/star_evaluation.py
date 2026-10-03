"""A star import may bind any name, so the eval below may be the builtin or a
name math brings: the adapter cannot tell which, and the facts claim neither."""
from math import *  # noqa: F403


def evaluate_unknown(text):
    return eval(text)
