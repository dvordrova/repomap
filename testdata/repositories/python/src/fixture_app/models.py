class GetLevelsInfoResponse:
    count: int


class OtherResponse:
    count: str


# TODO: document count validation.

# NOTE: count describes a quantity, not a list of levels.

# Several local names may refer to the same imported declaration.
from json import loads, loads as parse_json, loads as decode_json
import json as first_json, json as second_json
from json import loads as other_parser


def parse_alias_inputs(text):
    return (
        loads(text),
        parse_json(text),
        decode_json(text),
        first_json.loads(text),
        second_json.loads(text),
        other_parser(text),
    )


def deliver_callback(callback):
    return callback("fixture")


def handle_delivery(value):
    return value


def register_callback_aliases(handler, replace_handler):
    if replace_handler:
        handler = handle_delivery
    deliver_callback(handler)
    sentinel = lambda value: value
    deliver_callback(sentinel)
    deliver_callback(callback=sentinel)
    deliver_callback(lambda value: value)


def register_unknown_callback(handler):
    deliver_callback(handler)


def register_overwritten_callback(handler):
    sentinel = lambda value: value
    sentinel = handler
    deliver_callback(sentinel)


def register_chained_callbacks(stream):
    # Both calls start at stream, but each owns a different lambda argument.
    return stream.map(lambda value: value * 2).map(lambda value: {"value": value})


class MutableCounter:
    """A counter with explicit writes, a read-only path and dynamic frontiers."""
    count: int

    def __init__(self):
        self.count = 0

    def increment(self):
        self.count += 1

    def read(self):
        return self.count

    def clear(self):
        del self.count

    def reset(receiver):
        receiver.count: int = 0

    def aliased(self):
        counter = self
        counter.count = 2

    def rebound(self, other):
        self = other
        self.count = 3

    @staticmethod
    def static_write(self):
        self.count = 4

    def captured(self):
        class OtherCounter:
            count: int

            def write(inner):
                self.count = 5
                inner.count = 6
        return OtherCounter()


def update_counter(counter: MutableCounter):
    counter.count = 7


def create_counter():
    counter = MutableCounter()
    counter.count = 8
    return counter


def unknown_counter(counter):
    counter.count = 9
    setattr(counter, "count", 10)


def replaced_counter(counter: MutableCounter, other):
    counter = other
    counter.count = 11


# Reads keep the original declaration across imports, with lexical shadowing.
from fixture_app.levels import READ_VALUES as values, READ_LIMIT
import fixture_app.levels as level_data


def read_level_data(key):
    return values[key], level_data.READ_LIMIT, READ_LIMIT + READ_LIMIT


def shadow_level_data(values):
    READ_LIMIT = 3
    return values, READ_LIMIT


def comprehension_data():
    return [READ_LIMIT for READ_LIMIT in values], READ_LIMIT


class ReadScope:
    READ_LIMIT = 4
    snapshot = READ_LIMIT

    def method(self):
        return READ_LIMIT


def read_global_data():
    global READ_LIMIT
    return READ_LIMIT


def outer_data():
    READ_LIMIT = 5

    def inner_data():
        nonlocal READ_LIMIT
        return READ_LIMIT

    return inner_data


def with_shadow(manager):
    with manager as READ_LIMIT:
        return READ_LIMIT


def except_shadow():
    try:
        pass
    except Exception as READ_LIMIT:
        return READ_LIMIT


def match_shadow(value):
    match value:
        case {"limit": READ_LIMIT}:
            return READ_LIMIT


def read_counter(counter: MutableCounter):
    return counter.count


def read_replaced_counter(counter: MutableCounter, other):
    counter = other
    return counter.count


def store_level_data(key):
    values[key] = READ_LIMIT
