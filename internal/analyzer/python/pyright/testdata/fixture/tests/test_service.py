from app.service import process


def test_process() -> None:
    assert process(" value ") == "value"
