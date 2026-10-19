"""Calls a subclass runs, the way freqtrade's bot calls its strategy.

A method a subclass is left to write (abc.abstractmethod, or a body that
only raises NotImplementedError) never runs its own body: a call of it runs
the method of whichever class the value is, the receiver's class or one
deriving from it, abstract classes never being the value. A concrete method
a subclass overrides is the same: the base's own body or each override. A
protocol's method is each class's that has every member the protocol
declares. A call naming the class, or through super(), runs that very
body. A method nothing implements keeps its call to the method itself.
"""

import abc
from typing import Protocol


class Exporter(abc.ABC):
    @abc.abstractmethod
    def export(self, rows):
        """Write the rows somewhere."""

    def run(self, rows):
        # A template method: whichever exporter self is exports.
        return self.export(rows)


class CsvExporter(Exporter):
    def export(self, rows):
        return ",".join(rows)


class JsonExporter(Exporter):
    def export(self, rows):
        return "[" + ",".join(rows) + "]"


class BufferedExporter(Exporter):
    """Still abstract: it writes no export of its own."""

    def flush(self):
        return self.export([])


class FileExporter(BufferedExporter):
    def export(self, rows):
        return "\n".join(rows)


def export_all(exporter: Exporter, rows):
    return exporter.export(rows)


def flush_buffered(exporter: BufferedExporter):
    # Only FileExporter derives from BufferedExporter.
    return exporter.export([])


class Loader:
    def load(self, name):
        raise NotImplementedError

    def describe(self):
        return "loader"


class DiskLoader(Loader):
    def load(self, name):
        return name


def load_one(loader: Loader, name):
    loader.describe()
    return loader.load(name)


class Logged:
    def log(self, text):
        return text


class Filter(Logged, abc.ABC):
    """A mixin and ABC: its chain goes on through Logged."""

    def filter(self, pairs):
        self.log("filtering")
        return pairs


class AgeFilter(Filter):
    def filter(self, pairs):
        return pairs[:1]


class NameFilter(Filter):
    pass


def apply_filters(filters: list[Filter], pairs):
    for handler in filters:
        pairs = handler.filter(pairs)
    return pairs


class QuietCsvExporter(CsvExporter):
    def export(self, rows):
        # super() runs CsvExporter's own body; it is never this dispatch.
        return super().export(rows)


def csv_by_name(exporter: CsvExporter, rows):
    # Naming the class runs that very body.
    return CsvExporter.export(exporter, rows)


class Sink(Protocol):
    def write(self, text: str) -> None: ...

    def close(self) -> None: ...


class FileSink:
    def write(self, text: str) -> None:
        print(text)

    def close(self) -> None:
        return None


class NamedSink(Sink):
    def write(self, text: str) -> None:
        print("named", text)

    def close(self) -> None:
        return None


class HalfSink:
    def write(self, text: str) -> None:
        print(text)


def emit(sink: Sink, text):
    return sink.write(text)


class Archive(abc.ABC):
    @abc.abstractmethod
    def store(self, item):
        ...


def archive(target: Archive, item):
    # Nothing implements Archive.store: the call stays the method's own.
    return target.store(item)
