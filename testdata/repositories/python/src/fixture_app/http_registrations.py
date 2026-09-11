from fastapi import FastAPI

application = FastAPI()


@application.get("/health")
def empty_health_handler():
    pass


class RouteDefinition:
    def __init__(self, path):
        self.path = path


def register_route(path):
    @application.get(path)
    def empty_registered_handler():
        pass
    return empty_registered_handler


def install_routes(runtime_path):
    update = RouteDefinition("/v1/update")
    metrics = RouteDefinition("/v1/metrics")
    register_route(update.path)
    register_route(metrics.path)
    register_route(runtime_path)


def unused_route_definition():
    return RouteDefinition("/never-registered")


class LocalRouteNames:
    def get(self, path):
        return empty_health_handler


def local_name_control():
    return LocalRouteNames().get("/not-a-route")


class MethodArgumentApplication:
    def first(self, next_handler):
        return next_handler

    def second(self, next_handler):
        return next_handler


def receive_method_arguments(*values):
    pass


def pass_method_arguments(app: MethodArgumentApplication):
    receive_method_arguments(app.first, app.second, app.first, (app.second))
