class RestClient:
    def __init__(self, base_url):
        self.base_url = base_url

    def status_url(self):
        return self.base_url + "/api/v1/status"
