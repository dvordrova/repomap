"""Outside functions whose arguments decide what they do.

pydantic reads a Field's env variable when a BaseSettings class is built; on
any other model env is metadata nothing reads. yaml.load builds the Python
objects a document names with its unsafe Loader, and only data with
SafeLoader.
"""
import yaml
from pydantic import BaseModel, BaseSettings, Field


class ServiceSettings(BaseSettings):
    port: int = Field(8080, env="FIXTURE_SERVICE_PORT")


class Payload(BaseModel):
    label: str = Field("caption", env="FIXTURE_NOT_A_SETTING")


def read_document(stream):
    return yaml.load(stream, Loader=yaml.SafeLoader)


def build_document(stream):
    return yaml.load(stream, Loader=yaml.Loader)
