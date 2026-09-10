package facts

import "fmt"

// DataObject is source evidence about a declaration or SQL text, not proof of
// an executed database operation. Scope identifies source metadata/configuration;
// an empty connection is deliberately unknown, never a repository-wide database.
type DataObject struct {
	Kind       string       `json:"kind"`
	Origin     string       `json:"origin"`
	Scope      string       `json:"scope"`
	Connection string       `json:"connection,omitempty"`
	Schema     string       `json:"schema,omitempty"`
	Name       string       `json:"name"`
	Statement  string       `json:"statement,omitempty"`
	SQL        string       `json:"sql,omitempty"`
	Expression string       `json:"expression,omitempty"`
	Partial    bool         `json:"partial,omitempty"`
	Tables     []string     `json:"tables,omitempty"`
	Columns    []DataColumn `json:"columns,omitempty"`
	Owner      *Anchor      `json:"owner,omitempty"`
}
type DataColumn struct {
	Name       string `json:"name"`
	Type       string `json:"type,omitempty"`
	PrimaryKey bool   `json:"primary_key,omitempty"`
	ForeignKey string `json:"foreign_key,omitempty"`
	Anchor     Anchor `json:"anchor"`
}

func (data *DataObject) Validate() error {
	if data == nil {
		return nil
	}
	if (data.Kind != "table" && data.Kind != "query") || data.Scope == "" || data.Name == "" {
		return fmt.Errorf("data evidence needs kind, source scope and name")
	}
	if data.Origin != "ddl" && data.Origin != "orm" && data.Origin != "query" {
		return fmt.Errorf("unknown data source origin")
	}
	if data.Kind == "query" && (data.SQL == "" || data.Statement == "") {
		return fmt.Errorf("query needs original SQL and statement")
	}
	if data.Owner != nil {
		if err := data.Owner.validate(); err != nil {
			return err
		}
	}
	for _, column := range data.Columns {
		if column.Name == "" {
			return fmt.Errorf("unnamed data column")
		}
		if err := column.Anchor.validate(); err != nil {
			return err
		}
	}
	return nil
}
func CloneData(data *DataObject) *DataObject {
	if data == nil {
		return nil
	}
	out := *data
	out.Tables = append([]string(nil), data.Tables...)
	out.Columns = append([]DataColumn(nil), data.Columns...)
	if data.Owner != nil {
		anchor := *data.Owner
		out.Owner = &anchor
	}
	return &out
}
