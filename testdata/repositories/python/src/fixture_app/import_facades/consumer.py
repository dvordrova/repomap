from schedule import Scheduler
from .resolvers import Resolver as Factory
import fixture_app.import_facades.resolvers as providers
from . import resolvers as facade
from .cycle_a import Cycle
from .invalidated import Reassigned, Deleted, Ambiguous, Conditional, Augmented, WithBound, ExceptBound, Captured
from .star import ExchangeResolver as StarOnly
from .resolvers import Missing


class Bot:
    def __init__(self):
        self.exchange = Factory.load_exchange()
        self.other = providers.Resolver.load_exchange()
        self.third = facade.Resolver.load_exchange()
        Scheduler().every().day.at("00:02").do(self.exchange.ws_connection_reset)
        Scheduler().every().day.at("00:03").do(self.other.ws_connection_reset)
        Scheduler().every().day.at("00:04").do(self.third.ws_connection_reset)


class UnknownBot:
    def __init__(self):
        self.cycle = Cycle.load_exchange()
        self.reassigned = Reassigned.load_exchange()
        self.deleted = Deleted.load_exchange()
        self.ambiguous = Ambiguous.load_exchange()
        self.conditional = Conditional.load_exchange()
        self.star = StarOnly.load_exchange()
        self.missing = Missing.load_exchange()
        self.untyped = Factory.load_untyped()
        Scheduler().every().day.do(self.cycle.ws_connection_reset)
        Scheduler().every().day.do(self.reassigned.ws_connection_reset)
        Scheduler().every().day.do(self.deleted.ws_connection_reset)
        Scheduler().every().day.do(self.ambiguous.ws_connection_reset)
        Scheduler().every().day.do(self.conditional.ws_connection_reset)
        Scheduler().every().day.do(self.star.ws_connection_reset)
        Scheduler().every().day.do(self.missing.ws_connection_reset)
        Scheduler().every().day.do(self.untyped.ws_connection_reset)
        self.augmented = Augmented.load_exchange()
        self.with_bound = WithBound.load_exchange()
        self.except_bound = ExceptBound.load_exchange()
        self.captured = Captured.load_exchange()
        Scheduler().every().day.do(self.augmented.ws_connection_reset)
        Scheduler().every().day.do(self.with_bound.ws_connection_reset)
        Scheduler().every().day.do(self.except_bound.ws_connection_reset)
        Scheduler().every().day.do(self.captured.ws_connection_reset)


class Exchange:
    def ws_connection_reset(self):
        raise AssertionError("same name is not the selected receiver")
