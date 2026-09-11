// Source query ownership examples; this fixture does not connect to a database.
interface QueryConnection { execute(sql: string): void }
const GLOBAL_SQL = "SELECT id FROM global_rows";
export function directQuery(connection: QueryConnection) {
    connection.execute("SELECT id FROM direct_rows");
}
export function queryText(): string {
    return "SELECT id FROM returned_rows";
}
export function callQuery(connection: QueryConnection) {
    directQuery(connection);
    connection.execute(queryText());
    connection.execute(GLOBAL_SQL);
}
