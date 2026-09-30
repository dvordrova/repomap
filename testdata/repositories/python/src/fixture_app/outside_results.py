"""Calls on what a call returns, and a callable handed over through
functools.partial, the way freqtrade builds and wires its Telegram bot.

A call on a call's result is a member of what that call produces:
Path(name).open() is pathlib.Path.open, a repository class's call gives
that class's method (Worker(name, 1).run() is BaseWorker.run), and a chain
goes on from there (Application.builder().token(token).build()).

A field stored from a repository function with no declared return type
holds what the function's one return statement returns when that is an
outside call: Bot._app is telegram.ext.Application.builder.token.build, so
self._app.bot.send_message is that symbol's member. A function with two
return statements gives the field no type.

functools.partial(f, ...) given as an argument hands f over, as f would.
"""
from functools import partial
from pathlib import Path

from telegram.ext import Application, CommandHandler

from fixture_app.workers import Worker


def read_settings(name):
    with Path(name).open() as handle:
        return handle.read()


def run_once(name):
    return Worker(name, 1).run()


class Bot:
    def __init__(self, token):
        self._app = self._build_app(token)
        self._fallback = self._either_app(token, False)

    def _build_app(self, token):
        return Application.builder().token(token).build()

    def _either_app(self, token, local):
        if local:
            return Application.builder().token(token).local_mode(True).build()
        return Application.builder().token(token).build()

    def register(self):
        self._app.add_handler(CommandHandler("forcebuy", partial(self._force_enter, side="long")))
        self._app.add_handler(CommandHandler("status", self._status))

    def announce(self, chat, text):
        return self._app.bot.send_message(chat, text)

    def announce_fallback(self, chat, text):
        return self._fallback.bot.send_message(chat, text)

    def _force_enter(self, update, side):
        return side

    def _status(self, update):
        return update


def create_datadir(config, datadir=None):
    """A path whose value is None names no file; one the walk writes around a
    value it cannot follow reads by its key: {user_data_dir}/data."""
    return Path(datadir) if datadir else Path(f"{config['user_data_dir']}/data")


def default_datadir(config):
    return create_datadir(config, None)
