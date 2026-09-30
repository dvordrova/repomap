from fixture_app import main

from . import runtime


def exercise_facades() -> tuple[object, object]:
    return main, runtime


# SQL a test runs is the test's, not the library's: its table reaches no data
# and its call no outbound call of the program.
def create_test_only_rows(connection):
    connection.execute("CREATE TABLE test_only_rows (id INTEGER PRIMARY KEY)")


# A test hands the program's throttle a lambda: no value of the program's.
def throttle_a_lambda():
    from fixture_app.stored_callbacks import Throttle

    return Throttle().throttle(lambda: 42, 0)
