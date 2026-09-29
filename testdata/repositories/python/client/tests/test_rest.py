from fixture_client.rest import RestClient


def test_status_url():
    assert RestClient("http://server").status_url() == "http://server/api/v1/status"
