from .resolver_impl import ExchangeResolver as Reassigned
Reassigned = object()
from .resolver_impl import ExchangeResolver as Deleted
del Deleted
from .resolver_impl import ExchangeResolver as Ambiguous
from .exchange_impl import Exchange as Ambiguous
if __name__ == "dynamic":
    from .resolver_impl import ExchangeResolver as Conditional
from .resolver_impl import ExchangeResolver as Augmented
Augmented += object()
from .resolver_impl import ExchangeResolver as WithBound
with object() as WithBound:
    pass
from .resolver_impl import ExchangeResolver as ExceptBound
try:
    raise RuntimeError()
except RuntimeError as ExceptBound:
    pass
from .resolver_impl import ExchangeResolver as Captured
match object():
    case Captured:
        pass
