from fixture_app import main

from . import runtime


def exercise_facades() -> tuple[object, object]:
    return main, runtime
