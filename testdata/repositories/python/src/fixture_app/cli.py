import uvicorn
from fastapi import FastAPI

from . import events
from .levels import fetch_level, retrieve_level


app = FastAPI()
dynamic_path = "/api/dynamic"


@app.get("/api/levels")
def get_levels():
    return []


@app.get("/api/level/{level_id}")
def get_level(level_id: str):
    return retrieve_level(level_id, fetch_level)


@app.post("/api/level/run")
def run_level():
    return {"ok": True}


@app.get("/api/backend-only")
def backend_only():
    return None


@app.get(dynamic_path)
def dynamic_level():
    return None


class LocalRouter:
    def get(self, _path):
        def decorate(handler):
            return handler
        return decorate


local_router = LocalRouter()


@local_router.get("/api/lookalike")
def local_lookalike():
    return None


def main() -> None:
    uvicorn.run(app, host="127.0.0.1", port=8000)


reassigned_path = "/api/old"
reassigned_path = "/api/reassigned"


@app.get(reassigned_path)
def reassigned_level():
    return None
