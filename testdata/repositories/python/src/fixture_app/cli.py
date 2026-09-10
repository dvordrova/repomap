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


# A local router keeps inherited framework methods, including multiline calls.
from fastapi import APIRouter as BaseRouter


class ApplicationRouter(BaseRouter):
    def api_route(self, path, **kwargs):
        return lambda handler: handler


class ChildRouter(ApplicationRouter):
    pass


class OverriddenRouter(ApplicationRouter):
    def get(self, path):
        return lambda handler: handler


class MixedRouter(LocalRouter, ApplicationRouter):
    pass


inherited_router = ChildRouter()
overridden_router = OverriddenRouter()
mixed_router = MixedRouter()


@inherited_router.get(
    "/api/inherited",
    tags=["fixture"],
)
def inherited_level():
    return None


@overridden_router.get("/api/overridden-lookalike")
def overridden_level():
    return None


@mixed_router.get("/api/mixed-lookalike")
def mixed_level():
    return None
