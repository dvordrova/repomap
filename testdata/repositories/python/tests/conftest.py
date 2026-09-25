# pytest loads this file itself as a local plugin; its name is pytest's.
def pytest_configure(config):
    config.addinivalue_line("markers", "market: exercises the fixture market")
