#!/usr/bin/env python3
"""A file launch marker alongside package-relative import syntax."""
from .models import GetLevelsInfoResponse
from .models import *


def load_later():
    from .models import OtherResponse
    return OtherResponse
