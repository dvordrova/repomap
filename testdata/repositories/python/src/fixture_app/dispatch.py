"""A command line read by comparing its first word, and tables of names.

`dispatch` compares the first argument with each subcommand's name in an
if/elif chain, one value compared with several words: one question for the
whole chain. `describe` matches a status with a match statement. A lone
comparison (`is_default`) is no dispatch.

`OPTIONS` is a table of option rows a function reads: each row is one
`Opt(...)` call under a key, one shape. `REQUIRED` is a list of names a
function reads. `FORMATS` is a list nothing reads: no table of names.

`ARGS_SERVE` and `ARGS_INIT` list keys of `OPTIONS`: `build_args` looks up
each element of the list it is handed in `OPTIONS` (by keyword, and by
position), and `init_flags` looks up each of `ARGS_INIT` itself, so their
rows are `OPTIONS`' rows. `NO_CONFIG` is only tested for a subcommand's
membership: the words of one condition, and "prune" is no subcommand.
`KNOWN` is only tested too, before `run_known` calls a method named after
the subcommand: its rows are none as well, as a list written in the
condition would be.

None of these holds keys of another table: `HELP`, looked up with its own
rows; `COMMANDS`, looked up in `HELP` only under a condition and handed
to `Builder.build` through its class, where its position is not the
parameter's; `READ_ONLY` and `WRITES`, tested for one value's membership
case by case, as an if/elif chain compares words.

`build_serve` hands `build_args` ARGS_SERVE with the parser its own call
to add_parser made for `serve`: the OPTIONS rows ARGS_SERVE names are
serve's options. They stay the program's too, since `build_subcommands`
hands `build_args` parsers the code does not follow.
"""
import argparse


class Opt:
    def __init__(self, *flags, help=""):
        self.flags = flags
        self.help = help


OPTIONS = {
    "verbose": Opt("-v", "--verbose", help="print more"),
    "force": Opt("-f", "--force", help="overwrite files"),
}

REQUIRED = ["port", "dbfile"]

FORMATS = ["json", "csv"]

ARGS_SERVE = ["verbose", "force"]

ARGS_INIT = ["force", "verbose"]

NO_CONFIG = ["init", "help", "prune"]

KNOWN = ["serve", "init"]

HELP = {"serve": "run the server", "init": "create the files"}

COMMANDS = ["serve", "init", "status"]

READ_ONLY = ["status", "help"]

WRITES = ["init", "serve"]


def run_init(arguments):
    return arguments


def dispatch(argv):
    command, rest = argv[0], argv[1:]
    if command == "init":
        return run_init(rest)
    elif command in ("serve", "run"):
        return "serve"
    elif command == "help":
        return "usage"
    return "unknown"


def describe(status):
    match status:
        case "up":
            return "running"
        case "down" | "stopped":
            return "stopped"
    return "unknown"


def is_default(level):
    return level == "default"


def add_options(parser):
    for name, opt in OPTIONS.items():
        parser.add_argument(*opt.flags, dest=name, help=opt.help)


def missing_settings(settings):
    return [name for name in REQUIRED if name not in settings]


def build_args(optionlist, parser):
    for val in optionlist:
        opt = OPTIONS[val]
        parser.add_argument(*opt.flags, dest=val, help=opt.help)


def build_subcommands(serve_parser, init_parser):
    build_args(optionlist=ARGS_SERVE, parser=serve_parser)
    build_args(ARGS_INIT, init_parser)


def init_flags():
    return [OPTIONS[name].flags for name in ARGS_INIT]


def needs_config(command):
    return command not in NO_CONFIG


def run_known(commands, command):
    if command not in KNOWN:
        return None
    return getattr(commands, "do_" + command)()


def help_lines():
    lines = {}
    for name in HELP:
        lines[name] = HELP[name]
    return lines


def described_commands():
    lines = {}
    for name in COMMANDS:
        if name in HELP:
            lines[name] = HELP[name]
    return lines


class Builder:
    def build(self, first, second):
        flags = {}
        for val in second:
            flags[val] = OPTIONS[val]
        return first, flags


def build_through_class(builder):
    return Builder.build(builder, COMMANDS, None)


def access(command):
    if command in READ_ONLY:
        return "read"
    elif command in WRITES:
        return "write"
    return "none"


def build_serve():
    parser = argparse.ArgumentParser("dispatch")
    commands = parser.add_subparsers(dest="command")
    serve = commands.add_parser("serve")
    build_args(optionlist=ARGS_SERVE, parser=serve)
    return parser
