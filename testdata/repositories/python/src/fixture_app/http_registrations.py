from fastapi import APIRouter, FastAPI

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


# install_route_tree hands one router to two helpers. The leaves' router
# reaches the leaf through one more parameter, so its route is held by the
# APIRouter() call. The branch also hands its router to itself: following that
# parameter back through its callers comes around to the same parameter, so
# its routes have no holder.
def install_route_tree():
    router = APIRouter()
    register_route_leaves(router)
    register_route_branch(router, False)


def register_route_leaves(router):
    register_route_leaf(router)


def register_route_leaf(router):
    router.add_api_route("/tree/leaf", empty_health_handler)


def register_route_branch(router, nested):
    if nested:
        router.add_api_route("/tree/branch/nested", empty_health_handler)
        return
    router.add_api_route("/tree/branch", empty_health_handler)
    register_route_branch(router, True)
