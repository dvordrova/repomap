#!/usr/bin/env python3
# The shebang makes this file a script launch, yet it has no __main__ guard:
# run as a script it only defines main. The console script runs main.
import sys

from fixture_client.rest import RestClient


def main():
    client = RestClient(sys.argv[1] if len(sys.argv) > 1 else "http://localhost:8080")
    print(client.status_url())
