# A guard beside the command line that the model places apart as a tool. It
# shares its directory with the console script's file, and the console
# script keeps that directory's files after it also claims its library's
# root.
from fixture_client.rest import RestClient


def measure():
    return RestClient("http://localhost:8080").status_url()


if __name__ == "__main__":
    measure()
