from typing import TYPE_CHECKING, Annotated, Generic, TypeVar, overload

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


# A type parameter's bound is an expression of the defining scope too.
class Checked[T: Annotated[object, lambda value: value is not None]]:
    value: T


def first_checked[T: Annotated[object, lambda value: value is not None]](items: list[T]) -> T:
    return items[0]


# A method's overload stubs are signatures of the method that follows them,
# as pick's are of pick: one declaration each, and a call of either reaches
# the implementation (beets's BeatportClient.search). A stub under
# TYPE_CHECKING folds into the def after the block.
class Picker:
    @overload
    def choose(self, items: list[str]) -> str: ...
    @overload
    def choose(self, items: list[int]) -> int: ...
    def choose(self, items):
        return pick(items)


if TYPE_CHECKING:
    @overload
    def checked_pick(items: list[str]) -> str: ...
def checked_pick(items):
    return items[0]


def pick_all(picker: Picker):
    return picker.choose(["a"]), pick([1]), checked_pick(["b"])
