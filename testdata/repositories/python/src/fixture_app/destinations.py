import os
import requests


def prepared_dispatch(session, address):
    request = requests.Request("GET", address)
    return session.send(request)


def dispatch(session, address):
    return requests.get(address)


def exchange_prices(session, base):
    endpoint = f"{base}/시세"
    return dispatch(session, endpoint)


def notify_orders(session, config):
    return dispatch(session, config["notifications"]["url"])


def captured_exchange(session, base):
    def refresh():
        return dispatch(session, base + "/candles")
    return refresh


def direct_price_feed():
    return requests.get("https://시세.example/prices%20latest")


def configured_price_feed(session):
    return dispatch(session, os.getenv("PRICE_FEED_URL"))


def choose_feed(session, preferred, fallback, use_preferred):
    return dispatch(session, preferred if use_preferred else fallback)


class ExchangeAdapter:
    def __init__(self, session, config):
        self.session = session
        self.base = config["exchange"]["url"]

    def refresh(self):
        return dispatch(self.session, self.base + "/candles")


def reassigned_address(session, replacement):
    address = "https://old.example/"
    address = replacement
    return dispatch(session, address)


def build_exchange(session, config):
    return ExchangeAdapter(session, config)


def run_exchange(session, config):
    adapter = build_exchange(session, config)
    return adapter.refresh()


class MutableAdapter:
    def __init__(self):
        self.url = "https://old.example/"

    def replace(self, url):
        self.url = url

    def send(self):
        return requests.get(self.url)


def mutable_adapter():
    return MutableAdapter()


class LiteralAdapter:
    def __init__(self, base):
        self.base = base

    def refresh(self):
        return requests.get(self.base + "/candles")


def run_literal_adapter():
    adapter = LiteralAdapter("https://exchange.example")
    return adapter.refresh()


def unused_literal_adapter():
    return LiteralAdapter("https://unrelated.example")


class DocumentedClient:
    """An author-described client used by a local test."""

    def setup(self):
        """Build the client with a nested test helper."""

        def setup_mock_client():
            """Set up a mock HTTP client for the test."""
            return requests.Session()

        return setup_mock_client()

    async def ready(self):
        """Report that the test setup is ready."""
        return True

    def undocumented(self):
        pass
        """A later string is not documentation for this method."""
