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


def report_failure(name):
    return name


def report_long(name):
    return name


def describe(name):
    return name


def count(name):
    return name


def checked_store(store, name, error):
    """A call's guard: an arm ending in a raise, a raise's operand and an
    except body run only on a failing path; any other arm is a branch; a try
    body and the rest run unguarded."""
    if error is not None:
        report_failure(name)
        raise ValueError(describe(name))
    if len(name) > 64:
        report_long(name)
    try:
        store.put(name)
    except KeyError:
        report_failure(name)
    count(name)


def checked_long(name):
    """An arm with a return before its raise does not only fail: it is a
    branch."""
    if len(name) > 128:
        report_long(name)
        if name.startswith("ok"):
            return name
        raise ValueError(name)
    return count(name)


def checked_branches(name, ready, retry):
    """Each call keeps the written condition and the arm at its own place."""
    if ready:
        count(name)
    else:
        report_long(name)
    ready and retry and count(name)
    ready or retry or report_long(name)
    report_failure(name) if ready else describe(name)
    match ready:
        case True:
            count(name)
