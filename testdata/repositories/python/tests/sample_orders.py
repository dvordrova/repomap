# Orders the tests build their markets from. No pytest pattern names this
# file, but it lives in tests/, a directory beside the packages the build
# declares that holds the test modules: it is test code, and no part of the
# program's map (freqtrade's tests/conftest_trades.py and its strategies
# loaded by path had drawn test parts on the product map).
def sample_orders() -> list[dict[str, object]]:
    return [{"pair": "ETH/BTC", "amount": 1.0}]
