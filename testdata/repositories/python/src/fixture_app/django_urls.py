from django.urls import path


def django_health(request):
    return "ready"


urlpatterns = [path("health", django_health)]


from fastapi import FastAPI as APIApplication


class LocalApplication:
    def include_router(self, router, prefix):
        return router


class Server:
    def configure(self, app: APIApplication):
        from fixture_app.route_mounts import other as local_router
        app.include_router(local_router, prefix="/configured")

    def configure_unknown(self, app):
        from fixture_app.route_mounts import other as local_router
        app.include_router(local_router, prefix="/unknown")

    def configure_local(self, app: LocalApplication):
        from fixture_app.route_mounts import other as local_router
        app.include_router(local_router, prefix="/lookalike")

    def configure_reassigned(self, app: APIApplication):
        from fixture_app.route_mounts import other as local_router
        app = LocalApplication()
        app.include_router(local_router, prefix="/reassigned")
