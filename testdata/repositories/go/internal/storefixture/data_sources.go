package storefixture

// Source query ownership examples; this fixture does not connect to a database.
type queryConnection interface{ Execute(string) }

var globalSQL = "SELECT id FROM global_rows"

func directQuery(connection queryConnection) {
	connection.Execute("SELECT id FROM direct_rows")
}
func queryText() string {
	return "SELECT id FROM returned_rows"
}
func CallQuery(connection queryConnection) {
	directQuery(connection)
	connection.Execute(queryText())
	connection.Execute(globalSQL)
}

// Go's written concrete receiver supplies compiler authority independently of
// Python's weaker annotation evidence.
type TypedQueryClient struct{}

func (*TypedQueryClient) ReadRows()           {}
func TypedParameter(client *TypedQueryClient) { client.ReadRows() }
