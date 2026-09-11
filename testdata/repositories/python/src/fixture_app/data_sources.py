# Source query ownership examples; this fixture does not connect to a database.
GLOBAL_SQL = "SELECT id FROM global_rows"


def direct_query(connection):
    connection.execute("SELECT id FROM direct_rows")


def query_text():
    return "SELECT id FROM returned_rows"


def call_query(connection):
    direct_query(connection)
    connection.execute(query_text())
    connection.execute(GLOBAL_SQL)
