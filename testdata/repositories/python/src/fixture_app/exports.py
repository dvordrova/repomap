"""A module that declares __all__ exports exactly the names it lists."""

__all__ = ["render_level"]


def render_level(level):
    return format_score(level)


def format_score(level):
    """Public by its spelling, internal because __all__ does not list it."""
    return str(level)
