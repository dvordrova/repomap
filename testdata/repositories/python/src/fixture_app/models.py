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
