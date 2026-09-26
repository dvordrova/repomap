from typing import Generic, TypeVar, overload

T = TypeVar("T")


# A generic class keeps its type parameters in its header; its fields are
# declarations of their own.
class Box(Generic[T]):
    value: T


# PEP 695 writes the same parameters in brackets after the name.
class Crate[T]:
    value: T


class Keyed[K: str, V: (int, str)](Box[V]):
    key: K


def first[T](items: list[T]) -> T:
    return items[0]


# Overload stubs repeat one name: the map of parts reads them and the
# implementation as one unit, shown with the first stub's signature.
@overload
def pick(items: list[str]) -> str: ...
@overload
def pick(items: list[int]) -> int: ...
def pick(items):
    """Returns the first item.

    A docstring, like a comment or a blank line, is no line of code.
    """
    # The first item is the pick.
    head = items[0]

    return head
