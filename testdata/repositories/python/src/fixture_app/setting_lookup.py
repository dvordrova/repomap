"""A setting read as a configuration helper often reads it: from the
environment, else a word kept for two of its keys only. open_store's data
source is neither word; fetch_static's base is the URL."""
import os
import sqlite3

import httpx


def setting_or_default(key):
    value = os.environ.get(key)
    if value:
        return value
    if key == "staticBaseUrl":
        return "https://cdn.example/static"
    elif key == "logConfig":
        return "logs/app.log"
    return ""


def open_store():
    return sqlite3.connect(setting_or_default("dataSourceName"))


def fetch_static():
    return httpx.get(setting_or_default("staticBaseUrl"))
