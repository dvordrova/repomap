import asyncio
from contextlib import asynccontextmanager
from threading import Thread
from multiprocessing import Process
from schedule import Scheduler
from fastapi import FastAPI


class MarketFeed:
    def __init__(self):
        self.thread = Thread(target=self.receive_prices)
        self.thread.start()

    def receive_prices(self):
        """Receive market updates for as long as the event loop runs."""
        loop = asyncio.new_event_loop()
        loop.run_forever()


async def refresh_candles(stream):
    """Refresh stored candles from incoming market updates."""
    async for candle in stream:
        print(candle)


def submit_candles(stream):
    return asyncio.create_task(refresh_candles(stream))


def persist_wallet():
    """Record the latest wallet balances."""
    print("record wallet")


def register_jobs():
    scheduler = Scheduler()
    scheduler.every().day.at("00:07").do(persist_wallet)
    process = Process(target=persist_wallet)
    process.start()
    return scheduler


def bounded_retry():
    for attempt in range(3):
        print(attempt)


@asynccontextmanager
async def lifespan(app):
    print("startup")
    yield
    print("shutdown")


application = FastAPI(lifespan=lifespan)
unstarted = Thread(target=bounded_retry)


class Wallets:
    def record_wallet_state(self):
        print("wallet state")


class Exchange:
    def ws_connection_reset(self):
        print("reset market stream")


class ExchangeResolver:
    @staticmethod
    def load_exchange() -> Exchange:
        return Exchange()


class TradingBot:
    def __init__(self):
        self.wallets = Wallets()
        self.exchange = ExchangeResolver.load_exchange()
        self.schedule = Scheduler()
        self.schedule.every().day.at("00:02").do(self.exchange.ws_connection_reset)
        self.schedule.every().day.at("00:07").do(self.wallets.record_wallet_state)


class UnknownWalletOwner:
    def __init__(self, replacement):
        self.wallets = replacement
        Scheduler().every().day.do(self.wallets.record_wallet_state)
