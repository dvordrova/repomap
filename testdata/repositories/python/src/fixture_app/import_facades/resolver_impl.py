from .exchange import Exchange


class ExchangeResolver:
    @staticmethod
    def load_exchange() -> Exchange:
        return Exchange()

    @staticmethod
    def load_untyped():
        return Exchange()
