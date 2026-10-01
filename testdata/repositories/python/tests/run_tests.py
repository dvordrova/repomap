"""A runner the tests directory keeps: a script in a test directory is test
code, never one of the project's programs."""
import sys

import pytest

if __name__ == "__main__":
    sys.exit(pytest.main([__file__.rsplit("/", 1)[0]]))
