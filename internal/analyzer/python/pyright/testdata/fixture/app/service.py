from app.repository import Repository


def normalize(value: str) -> str:
    return value.strip()


def process(value: str) -> str:
    normalized = normalize(value)
    return Repository().save(normalized)


class LegacyService:
    def process(self, value: str) -> str:
        return value


def dynamic_call(target: object, method_name: str, value: str) -> object:
    return getattr(target, method_name)(value)
