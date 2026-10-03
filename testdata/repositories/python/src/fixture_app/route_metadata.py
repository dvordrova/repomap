"""Literals that look like paths but state no prefix.

A router's description and an application's documentation URL are values,
whatever their shape: described_ping answers at /ping and documented_health
at /health. Only the prefix keyword composes: prefixed_ping answers at
/v2/ping.
"""
from fastapi import APIRouter, FastAPI

described = APIRouter(description="/docs")
prefixed = APIRouter(prefix="/v2", description="/docs")
documented = FastAPI(docs_url="/docs")


@described.get("/ping")
def described_ping():
    return "pong"


@prefixed.get("/ping")
def prefixed_ping():
    return "pong"


@documented.get("/health")
def documented_health():
    return "ready"
