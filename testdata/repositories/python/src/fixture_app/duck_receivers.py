"""Calls on values nothing types, the way huey's Huey calls its storage.

Queue keeps its storage as huey's Huey does: create_storage builds whatever
class storage_class names, so no store, annotation or flow says which
storage self.storage holds. The code says what it needs of it: Queue uses
enqueue, dequeue and size on it, and every class here having those is one
it may be, so put calls each storage's enqueue (not Storage's own, which
only raises: Storage is never the value) and pending calls the size every
storage inherits. A
Consumer is built through a class held in an attribute, as huey builds its
Worker, and uses take and put on its queue: only Queue has both.

What the code names no more than the method it calls, a value a builtin
type could be, a value whose shape the code tests, a store of an outside
value, and a class whose member of the called name is a number leave their
calls unresolved. A subclass's use narrows only its own calls.
"""

import sqlite3


class Storage:
    def enqueue(self, item):
        raise NotImplementedError

    def dequeue(self):
        raise NotImplementedError

    def size(self):
        return 0


class MemoryStorage(Storage):
    def __init__(self):
        self.items = []

    def enqueue(self, item):
        self.items.append(item)

    def dequeue(self):
        return self.items.pop(0)


class FileStorage(Storage):
    def __init__(self):
        self.path = "queue.txt"

    def enqueue(self, item):
        return self.path + item

    def dequeue(self):
        return self.path


class Queue:
    storage_class = MemoryStorage
    consumer_class = None

    def __init__(self, storage_class=None):
        if storage_class is not None:
            self.storage_class = storage_class
        self.storage = self.create_storage()

    def create_storage(self):
        return self.storage_class()

    def put(self, item):
        return self.storage.enqueue(item)

    def take(self):
        return self.storage.dequeue()

    def pending(self):
        return self.storage.size()

    def consumer(self):
        return self.consumer_class(self)


class FileQueue(Queue):
    storage_class = FileStorage

    def flush(self):
        # Only FileStorage has a path: this call is its enqueue alone.
        return self.storage.enqueue(self.storage.path)


class Consumer:
    def __init__(self, queue):
        self.queue = queue

    def loop(self):
        item = self.queue.take()
        return self.queue.put(item)


Queue.consumer_class = Consumer


class Journal:
    def write(self, text):
        return text


class Notifier:
    def __init__(self, sink):
        self.sink = sink

    def notify(self, text):
        # write is all the code uses on the sink: a method's name alone.
        return self.sink.write(text)


class Settings:
    def get(self, key):
        return key

    def items(self):
        return []


def tally(counts):
    # A dict has get and items too.
    counts.get("total")
    return counts.items()


def drain(storage):
    # The code tests the value's shape: it is not one kind of value.
    if hasattr(storage, "size"):
        storage.size()
    return storage.dequeue()


class Txn:
    def execute(self, statement):
        return statement

    def commit(self):
        return None


class Remote:
    def __init__(self, conn=None):
        self.conn = conn or sqlite3.connect(":memory:")

    def save(self, statement):
        # One store may be an outside connection.
        self.conn.execute(statement)
        return self.conn.commit()


class Metronome:
    def tick(self):
        return 1

    def reset(self):
        return 0


class Counter:
    def __init__(self):
        self.tick = 0

    def reset(self):
        self.tick = 0


class Beat:
    def __init__(self, timer):
        self.timer = timer

    def step(self):
        self.timer.reset()
        # Counter has a tick, but a number: no method is claimed.
        return self.timer.tick()
