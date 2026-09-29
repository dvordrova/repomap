"""Clients a class builds once and its subclasses inherit, the way
freqtrade's Exchange keeps its ccxt client and its Webhook its address.

A call on self is the method the class declares or inherits along its chain
of single repository bases: Discord.notify calls the inherited Webhook.send.
A class with two bases, or with a base outside the repository, names none.

A field stored once holds the outside type its store gives it: the type a
repository factory declares it returns (_make_client -> httpx.Client), or
the type its parameter is annotated with (client: httpx.Client). A subclass
sees its base's field. A second store, in the class or in a class deriving
from it, leaves the field unknown, and so does a union.

A method reading a field its subclasses store too reads one of those
stores: Webhook.send's self.url is Webhook's address or Discord's, each
listed and neither chosen.
"""
import threading

import httpx


class Prices:
    def __init__(self, base):
        self.client = self._make_client(base)

    def _make_client(self, base) -> httpx.Client:
        return httpx.Client(base_url=base)

    def ask(self, pair):
        return self.client.get("/prices/" + pair)


class FuturesPrices(Prices):
    def funding(self, pair):
        self.ask(pair)
        return self.client.get("/funding/" + pair)


class StreamedPrices:
    def __init__(self, client: httpx.Client):
        self.client = client

    def poll(self):
        return self.client.get("/stream")


class Quotes:
    def __init__(self, base):
        self.client = httpx.Client(base_url=base)

    def last(self):
        return self.client.get("/last")


class AsyncQuotes(Quotes):
    """Stores a client of its own, so Quotes.last reads either."""

    def __init__(self, base):
        self.client = httpx.AsyncClient(base_url=base)


class MaybePrices:
    def __init__(self, base):
        self.client = self._make_client(base)

    def _make_client(self, base) -> httpx.Client | None:
        return httpx.Client(base_url=base) if base else None

    def ask(self):
        return self.client.get("/prices")


class MixedPrices(Prices, Quotes):
    def both(self):
        return self.ask("BTC")


class Heartbeat(threading.Thread):
    def beat(self):
        return self.start()


class Webhook:
    def __init__(self, config):
        self.url = config["webhook"]["url"]

    def send(self, payload):
        return httpx.post(self.url, json=payload)


class Discord(Webhook):
    def __init__(self, config):
        self.url = config["discord"]["webhook_url"]

    def notify(self, text):
        return self.send({"content": text})
