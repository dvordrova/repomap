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
