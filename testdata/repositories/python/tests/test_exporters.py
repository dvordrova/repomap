"""A test's exporter is no value the program's export_all can be given."""

from fixture_app.subclass_calls import Exporter, export_all


class MockExporter(Exporter):
    def export(self, rows):
        return rows


def test_export_all():
    assert export_all(MockExporter(), ["a"]) == ["a"]


def export_in_test(exporter: Exporter):
    # The test's own call may reach its own exporter.
    return exporter.export(["b"])
