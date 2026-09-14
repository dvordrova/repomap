import httpx


def fetch_level(level_id: str):
    return httpx.get(f"https://catalog.example/levels/{level_id}")


def retrieve_level(level_id: str, loader):
    return loader(level_id)


import requests


def read_local_http_name():
    return requests.get("/local-key")


# A data dependency is a read, even when no function in this module is called.
READ_VALUES = {"one": 1}
READ_LIMIT = 8
