"""Workers built and run the way freqtrade's trade command runs its Worker.

Calling a class constructs an instance: the call is the class's, and it
runs the class's __init__, its own (Worker) or the one it inherits
(QuietWorker keeps BaseWorker's). A name assigned once from that call holds
that instance, so a call on it is the class's method, an inherited one
included; a store of None does not count, since None has no method to call.
A name stored twice holds either instance, and a call on it stays
unresolved.
"""


class BaseWorker:
    def __init__(self, name):
        self.name = name

    def run(self):
        return self.step()

    def step(self):
        return self.name


class Worker(BaseWorker):
    def __init__(self, name, retries):
        super().__init__(name)
        self.retries = retries

    def step(self):
        return self.name * self.retries


class QuietWorker(BaseWorker):
    """Keeps BaseWorker's __init__, run and step."""


class Plain:
    """No __init__ anywhere: constructing it runs no repository code."""


def start_worker(name):
    worker = None
    try:
        worker = Worker(name, 2)
        worker.run()
    finally:
        if worker:
            worker.step()
    return 0


def start_quiet(name):
    quiet = QuietWorker(name)
    return quiet.run()


def start_either(name, fast):
    chosen = Worker(name, 1)
    if fast:
        chosen = QuietWorker(name)
    return chosen.run()


def make_plain():
    return Plain()
