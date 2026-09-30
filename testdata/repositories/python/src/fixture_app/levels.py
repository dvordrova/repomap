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


# A module logger: the file's own handle on the standard library, read only
# by the functions of this file. Its part draws no tile for it.
import logging

logger = logging.getLogger(__name__)


def log_level(level_id: str):
    logger.info("level %s", level_id)


# The client project's package, another target of this repository: the
# import names its module, so the call is the seam between the two targets.
from fixture_client.rest import RestClient


def level_status_url(base_url: str):
    return RestClient(base_url).status_url()
