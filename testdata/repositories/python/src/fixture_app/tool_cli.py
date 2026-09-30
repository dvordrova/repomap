"""A command-line tool: argparse declares its option and its subcommand.

The parser, the subcommand collection and the subcommand's parser are each
the result of a call to argparse; the calls made on those results are the
code's own fact (`result_receives`). `init` is named by add_parser and
handled through set_defaults, two calls that each hand argparse something,
one input. `--force` is declared on init's own parser, `--verbose` on the
tool's.
`revision` starts another program, git: the words of its command line are
git's, not this tool's options. `init-*` is a word only init's handler
compares with what it was handed: a sub-argument of init.
"""
import argparse
import fnmatch
import subprocess


def run_init(arguments):
    if fnmatch.fnmatch(arguments.cmd, "init-*"):
        return "variant"
    return arguments.cmd


def build_parser():
    parser = argparse.ArgumentParser("tool")
    parser.add_argument("-v", "--verbose", action="store_true")
    commands = parser.add_subparsers(dest="cmd")
    init = commands.add_parser("init")
    init.set_defaults(func=run_init)
    init.add_argument("--force", action="store_true")
    add_common(init)
    add_output(init)
    status = commands.add_parser("status")
    add_common(status)
    add_output(parser)
    return parser


def main():
    arguments = build_parser().parse_args()
    return arguments.func(arguments)


def revision():
    return subprocess.run(["git", "rev-parse", "HEAD"], check=True, capture_output=True).stdout


def run_serve(arguments):
    return arguments.cmd


class ServiceCommands:
    """Subcommands built on the instance, as freqtrade's Arguments builds
    its own: the parser and the subcommand collection are fields, each
    stored once from argparse's call, so the calls made on them are
    argparse's and `serve` is one input with its handler."""

    def __init__(self):
        self.parser = argparse.ArgumentParser("service")

    def build(self):
        self._subparsers = self.parser.add_subparsers(dest="cmd")
        serve = self._subparsers.add_parser("serve")
        serve.set_defaults(func=run_serve)
        return self.parser


class RebuiltParser:
    """A field stored twice holds either parser: the calls on it stay
    unresolved."""

    def __init__(self):
        self.parser = argparse.ArgumentParser("first")

    def rebuild(self):
        self.parser = argparse.ArgumentParser("second")
        self.parser.add_argument("--again")


def add_common(command: argparse.ArgumentParser):
    """Declares --quiet on the parser it is handed. Every call hands it a
    subcommand's own parser, init's and status's: --quiet is an option of
    each, and no flag of the tool's own."""
    command.add_argument("--quiet", action="store_true")


def add_output(parser: argparse.ArgumentParser):
    """Declares --json on init's parser and on the tool's own: an option of
    init that the tool also takes."""
    parser.add_argument("--json", action="store_true")


def build_remote():
    """A command group: remote is a word a person types before its own
    subcommands, add among them, which run_remote_add handles. Each
    add_subparsers is given its word only as dest=, where the chosen word
    is kept: no input of its own."""
    parser = argparse.ArgumentParser("remote-tool")
    commands = parser.add_subparsers(dest="cmd")
    remote = commands.add_parser("remote")
    remote_commands = remote.add_subparsers(dest="remote_cmd")
    add = remote_commands.add_parser("add")
    add.set_defaults(func=run_remote_add)
    return parser


def run_remote_add(arguments):
    return arguments.cmd
