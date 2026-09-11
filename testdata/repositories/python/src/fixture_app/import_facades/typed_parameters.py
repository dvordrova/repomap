from .exchange import Exchange


def typed(exchange: Exchange):
    exchange.ws_connection_reset()


def unknown(exchange):
    exchange.ws_connection_reset()


def union(exchange: Exchange | str):
    exchange.ws_connection_reset()


def reassigned(exchange: Exchange, replacement):
    exchange = replacement
    exchange.ws_connection_reset()


def conditional(exchange: Exchange, replacement, flag):
    if flag:
        exchange = replacement
    exchange.ws_connection_reset()


def missing(exchange: "UnknownExchange"):
    exchange.ws_connection_reset()
