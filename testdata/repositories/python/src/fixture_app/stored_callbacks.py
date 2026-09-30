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
# called: the call stays unresolved and names each function stored in it.
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
