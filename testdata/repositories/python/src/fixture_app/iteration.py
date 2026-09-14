"""Typed iteration retains possible method targets, not runtime guarantees."""
from typing import List, Sequence, Tuple


class MovingItem:
    position: int = 0

    def advance(self):
        self.position += 1


class ItemBatch:
    items: List[MovingItem]

    def run(self):
        for item in sorted(self.items, key=lambda item: item.position):
            item.advance()  # typed field through sorted
            item.position = 2  # typed iteration write

    def replaced_receiver(self, replacement):
        self = replacement
        for item in self.items:
            item.advance()  # replaced receiver


def direct(items: Sequence[MovingItem], replacement):
    for item in items:
        item.advance()  # typed parameter
        item = replacement
        item.advance()  # replaced item
    item.advance()  # zero iterations possible


def shadowed_sorted(items: List[MovingItem], sorted):
    for item in sorted(items):
        item.advance()  # shadowed sorted


def replaced_collection(items: List[MovingItem], replacement):
    items = replacement
    for item in items:
        item.advance()  # replaced collection


def unknown_collection(items):
    for item in items:
        item.advance()  # unknown collection


def local_annotation():
    items: list[MovingItem] = []
    for item in items:
        item.advance()  # local annotation
    else:
        item.advance()  # zero iterations in else


def homogeneous(items: Tuple[MovingItem, ...]):
    for item in items:
        item.advance()  # homogeneous tuple


def heterogeneous(items: Tuple[MovingItem, str]):
    for item in items:
        item.advance()  # heterogeneous tuple


def union_items(items: List[MovingItem | str]):
    for item in items:
        item.advance()  # union item


def async_items(items: List[MovingItem]):
    # The synchronous annotation cannot supply an async-iteration target.
    async def run():
        async for item in items:
            item.advance()  # async iteration
    return run


# Python's underscore is a normal parameter/local name, including annotations.
def underscore_parameter(_: MovingItem):
    _.advance()  # annotated underscore parameter


def underscore_result():
    _: MovingItem = MovingItem()
    _.advance()  # annotated underscore call result


def underscore_unknown(_):
    _.advance()  # untyped underscore parameter
