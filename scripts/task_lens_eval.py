#!/usr/bin/env python3
"""Post-seal historical-gold evaluator for Task Lens v0."""

from __future__ import annotations

import argparse
import sys

sys.dont_write_bytecode = True

from task_lens_harness import (
    HarnessError,
    declare_cheap_exits,
    reject_ambient_git_env,
    render_evaluation,
    unlock_gold,
)


def parser() -> argparse.ArgumentParser:
    result = argparse.ArgumentParser(description=__doc__)
    commands = result.add_subparsers(dest="command", required=True)

    cheap_exit = commands.add_parser(
        "declare-cheap-exits",
        help="seal expected cheap-exit episode IDs before holdout execution and gold access",
    )
    cheap_exit.add_argument("--root", required=True)
    cheap_exit.add_argument("--episode", action="append", required=True)
    cheap_exit.set_defaults(func=declare_cheap_exits)

    unlock = commands.add_parser(
        "unlock-gold",
        help="verify the global holdout seal, then copy and inventory historical gold",
    )
    unlock.add_argument("--root", required=True)
    unlock.add_argument("--gold-dir", required=True)
    unlock.set_defaults(func=unlock_gold)

    evaluate = commands.add_parser(
        "evaluate",
        help="validate separate 0-4 supervisor scores and render the review bundle",
    )
    evaluate.add_argument("--root", required=True)
    evaluate.add_argument("--scores", required=True)
    evaluate.set_defaults(func=render_evaluation)
    return result


def main() -> int:
    try:
        reject_ambient_git_env()
        args = parser().parse_args()
        args.func(args)
        return 0
    except HarnessError as exc:
        print(f"task-lens-eval: error: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
