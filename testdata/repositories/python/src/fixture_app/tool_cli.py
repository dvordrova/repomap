"""A command-line tool: argparse declares its option and its subcommand.

The parser, the subcommand collection and the subcommand's parser are each
the result of a call to argparse; the calls made on those results are the
code's own fact (`result_receives`). `init` is named by add_parser and
handled through set_defaults, two calls that each hand argparse something,
one input. `--force` is declared on init's own parser, `--verbose` on the
tool's.
`revision` starts another program, git: the words of its command line are
git's, not this tool's options.
"""
import argparse
import subprocess


def run_init(arguments):
    return arguments.cmd


def build_parser():
    parser = argparse.ArgumentParser("tool")
    parser.add_argument("-v", "--verbose", action="store_true")
    commands = parser.add_subparsers(dest="cmd")
    init = commands.add_parser("init")
    init.set_defaults(func=run_init)
    init.add_argument("--force", action="store_true")
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
