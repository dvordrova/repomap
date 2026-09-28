"""A command-line tool: argparse declares its option and its subcommand.

The parser, the subcommand collection and the subcommand's parser are each
the result of a call to argparse; the calls made on those results are the
code's own fact (`result_receives`). `init` is named by add_parser and
handled through set_defaults, two calls that each hand argparse something.
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
    return parser


def main():
    arguments = build_parser().parse_args()
    return arguments.func(arguments)


def revision():
    return subprocess.run(["git", "rev-parse", "HEAD"], check=True, capture_output=True).stdout
