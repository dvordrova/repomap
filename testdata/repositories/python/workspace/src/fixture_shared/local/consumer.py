"""Other distributions can supply sibling modules of fixture_shared."""

import fixture_shared
from ..module_loading import load
from ..extensions import *
from .. import runtime_member

# A missing child of this ordinary package has no such namespace authority.
from .missing import unavailable


def read(name):
    return load(name)
