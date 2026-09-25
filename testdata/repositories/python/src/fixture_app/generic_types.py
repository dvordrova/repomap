from typing import Generic, TypeVar

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
