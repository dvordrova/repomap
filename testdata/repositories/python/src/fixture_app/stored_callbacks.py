# An event loop stores a handler in one of two attributes, chosen by a flag.
# The calls through those attributes stay unresolved: neither attribute is
# given the handler registered for the other.
class EventLoop:
    def __init__(self):
        self.on_read = None
        self.on_write = None

    def register(self, readable, handler):
        if readable:
            self.on_read = handler
        else:
            self.on_write = handler

    def fire(self):
        self.on_read()
        self.on_write()


def accept_client():
    pass


def flush_replies():
    pass


def run_event_loop():
    loop = EventLoop()
    loop.register(True, accept_client)
    loop.register(False, flush_replies)
    loop.fire()


# A local name bound once to a function calls that function.
def run_single_handler():
    handler = accept_client
    handler()


# A name reassigned under a branch may hold either function when it is
# called: the call calls one of them, each store naming its function.
def run_chosen_handler(readable):
    handler = flush_replies
    if readable:
        handler = accept_client
    handler()


# A def calling its own parameter calls what the program's calls into it
# hand that parameter (freqtrade's Worker._throttle(func=...)): the two
# callables run's calls hand are the call's alternatives, and the test's
# lambda (tests/test_facade.py) is none of the program's values.
class Throttle:
    def run(self, running):
        if running:
            self.throttle(step=self.process_running, secs=1)
        else:
            self.throttle(self.process_stopped, 1)

    def throttle(self, step, secs, *args, **kwargs):
        return step(*args, **kwargs)

    def process_running(self):
        accept_client()

    def process_stopped(self):
        flush_replies()


# One callable handed is the call's exact target.
def run_once(job):
    job()


def start_once():
    run_once(accept_client)


# A call handing any other value leaves the call unresolved, the callable
# another call hands still its witness.
def run_any(job):
    job()


def start_any(name):
    run_any(flush_replies)
    run_any(name)


# A loop throttling its one step, as freqtrade's Worker hands its bot's
# process to _throttle(func=...): run hands _process to _throttle, which
# calls it, and _process does the work. Both are private helpers folded into
# run, so a Main flow walked through run follows func to _process's
# accept_client and asks nothing.
class Loop:
    def run(self):
        return self._throttle(func=self._process)

    def _throttle(self, func):
        return func()

    def _process(self):
        return accept_client()


# A callee chosen by a condition calls one of the callables its branches
# name, the condition deciding which (C.md's conditional callee, Lua's
# f_parser): alternatives through a function value. Nested conditions and
# parenthesised names are the same choice, classes are constructed and
# outside functions are invoked. A constant condition, or two branches
# naming one function, calls it plainly; a branch naming anything else, here
# a parameter, leaves the call open.
import time


def tick_seconds(ms):
    return ms // 1000


def tick_millis(ms):
    return ms


def tick_tenths(ms):
    return ms // 100


def watch_tick(ms, seconds, tenths, held):
    (tick_seconds if seconds else tick_millis)(ms)
    (tick_tenths if tenths else (tick_seconds if seconds else (tick_millis)))(ms)
    (tick_seconds if seconds else held)(ms)


def watch_tick_again(ms, seconds):
    (tick_seconds if True else tick_millis)(ms)
    (tick_millis if seconds else tick_millis)(ms)
    (time.monotonic if seconds else time.perf_counter)()
    (EventLoop if seconds else Loop)()


# A parameter reassigned under a branch still holds what its caller handed
# when the branch is skipped: the call through it stays open.
def run_defaulted_handler(handler=None):
    if handler is None:
        handler = accept_client
    handler()


# A list, set or dict comprehension runs where it stands: a call in it finds
# only the stores before it, so a store after it is never its target (each
# raises UnboundLocalError), and its body repeats like a loop's. A generator
# expression runs when it is consumed, so any store may be what it finds.
def eager_list(flag):
    [handler() for _ in [0]]
    if flag:
        handler = accept_client


def eager_set(flag):
    {handler() for _ in [0]}
    if flag:
        handler = accept_client


def eager_dict(flag):
    {0: handler() for _ in [0]}
    if flag:
        handler = accept_client


def deferred_generator(flag):
    calls = (handler() for _ in [0])
    if flag:
        handler = accept_client
    return list(calls)


def overwritten_before_call(flag):
    handler = accept_client
    if flag:
        handler = flush_replies
    handler = accept_client
    handler()


def comprehension_in_loop(items):
    handler = flush_replies
    for item in items:
        [handler() for _ in [item]]
        if item:
            handler = accept_client
