from django.urls import path


def django_health(request):
    return "ready"


urlpatterns = [path("health", django_health)]
