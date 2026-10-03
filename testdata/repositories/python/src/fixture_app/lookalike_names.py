"""Functions this package names like a setting read and code evaluation.

A call to them runs the package's own code, whatever the name: it reads no
setting and evaluates nothing. read_setting and read_other_setting read the
process environment.
"""
import os
from os import environ


def getenv(key):
    return "database row: " + key


def Field(*, env):
    return {"label": env}


def eval(value):
    return value


def exec(value):
    return value


def read_lookalikes():
    return getenv("CUSTOMER_ROW"), Field(env="CAPTION"), eval("ordinary data"), exec("ordinary data")


def read_setting():
    return os.environ.get("FIXTURE_LOOKALIKE_SETTING")


def read_other_setting():
    return environ.get("FIXTURE_IMPORTED_ENVIRON_SETTING")


# A module-level value whose name starts with an underscore is a declaration
# like any other.
_token = "opaque"
