from fastapi import APIRouter, FastAPI
from flask import Blueprint, Flask
from django.urls import include, path

api = APIRouter(prefix="/v1")
other = APIRouter()
app = FastAPI()


@api.get("/ping")
def ping():
    return {"status": "ready"}


@other.get("/ping")
def other_ping():
    return {"status": "other"}


app.include_router(api, prefix="/api")
app.include_router(api, prefix="/alternate")
app.include_router(other, prefix="/private")

blueprint = Blueprint("public", __name__, url_prefix="/default")
flask_app = Flask(__name__)


@blueprint.route("/health")
def health():
    return "ready"


flask_app.register_blueprint(blueprint, url_prefix="/flask")


class LocalLookalike:
    def include_router(self, router, prefix):
        return router


LocalLookalike().include_router(api, prefix="/not-a-route")
urlpatterns = [path("django/", include("fixture_app.django_urls"))]
