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


# A database reached through objects, the way freqtrade's persistence does:
# its statements name nothing they reach, and are sent through a session
# stored once through its class's name, or through the connection a with
# statement enters. Each object is followed back to create_engine.
import sqlalchemy
from sqlalchemy.orm import scoped_session, sessionmaker


class Ledger:
    """Its session is stored once, through the class's name, by open_ledger."""

    session: "scoped_session"


class Archive:
    """Shares the ledger's session: its one store is the ledger's attribute."""

    session: "scoped_session"


class Journal:
    """Its class body stores a session too: a read through the class names none."""

    session = None


def open_ledger(url):
    engine = sqlalchemy.create_engine(url)
    Ledger.session = scoped_session(sessionmaker(bind=engine))
    Archive.session = Ledger.session
    Journal.session = scoped_session(sessionmaker(bind=engine))
    migrate_ledger(engine)


def migrate_ledger(engine):
    with engine.begin() as connection:
        connection.execute(sqlalchemy.text("ALTER TABLE ledger ADD note TEXT"))
    with engine.begin() as connection:
        statement = sqlalchemy.update(Ledger).values(note="")
        connection.execute(statement)
        statement = sqlalchemy.update(Archive).values(note="")
        connection.execute(statement)


def count_entries():
    return Ledger.session.execute(sqlalchemy.select(Ledger)).scalar_one()


def archived_entries():
    return Archive.session.scalars(sqlalchemy.select(Archive)).all()


def journal_entries():
    return Journal.session.scalars(sqlalchemy.select(Journal)).all()


def deferred_statement():
    # The lambda runs later: a name its function rebinds may hold either.
    statement = sqlalchemy.select(Ledger)
    statement = sqlalchemy.select(Archive)
    return lambda: Ledger.session.execute(statement)


def ledger_query(archived):
    # Each arm binds the statement; after the if it is either, none chosen.
    if archived:
        statement = sqlalchemy.select(Archive)
    else:
        statement = sqlalchemy.select(Ledger)
    if not archived:
        statement = statement.where(Ledger.note == "")
    return statement


def filtered_entries(archived):
    return Ledger.session.scalars(ledger_query(archived)).all()


def prune_ledger(engine):
    # A subquery is handed to a column's not_in, whose result the update
    # takes: it is sent where the update is.
    with engine.begin() as connection:
        statement = sqlalchemy.update(Ledger).where(Ledger.note.not_in(sqlalchemy.select(Archive.note)))
        connection.execute(statement)


def ledger_totals():
    # A statement built as a CTE is sent through the columns another
    # statement reads from it.
    totals = sqlalchemy.select(Ledger).cte("totals")
    return Ledger.session.execute(sqlalchemy.select(totals.c.note)).all()


def open_ledger_to_prune(url):
    prune_ledger(sqlalchemy.create_engine(url))
