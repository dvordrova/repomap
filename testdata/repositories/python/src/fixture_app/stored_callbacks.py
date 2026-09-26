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
