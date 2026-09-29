"""A command line read by comparing its first word, and tables of names.

`dispatch` compares the first argument with each subcommand's name in an
if/elif chain, one value compared with several words: one question for the
whole chain. `describe` matches a status with a match statement. A lone
comparison (`is_default`) is no dispatch.

`OPTIONS` is a table of option rows a function reads: each row is one
`Opt(...)` call under a key, one shape. `REQUIRED` is a list of names a
function reads. `FORMATS` is a list nothing reads: no table of names.
"""


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
