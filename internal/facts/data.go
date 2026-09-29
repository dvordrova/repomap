package facts

import "fmt"

// DataObject is source evidence about a declaration or SQL text, not proof of
// an executed database operation. Scope identifies source metadata/configuration;
// an empty connection is deliberately unknown, never a repository-wide database.
//
// A file (kind `file`, origin `call`) is a file the program's own code
// reaches by its path: the calls of outside symbols a `talks` answer made
// file calls, walked along their decided path argument (READING § What a
// call reaches). Its Name is where the walk ends: a literal or a template as
// written (`dump.rdb`, `{db.path}-wal`, `{--config}`, `{env:KVD_CONFIG}`), a
// field in braces (`{server.dbfilename}`), or empty when the path is not
// established; Scope is the program's target. File holds its calls and,
// for a field, the values its writes store.
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
	File       *DataFile    `json:"file,omitempty"`
}
type DataColumn struct {
	Name       string `json:"name"`
	Type       string `json:"type,omitempty"`
	PrimaryKey bool   `json:"primary_key,omitempty"`
	ForeignKey string `json:"foreign_key,omitempty"`
	Anchor     Anchor `json:"anchor"`
}

// DataFile is how a program's code reaches one file. Field is the field
// its path is read from, when the walk ends at one (`server.dbfilename`),
// and Values each write of that field in the program with the path it
// stores, or none when that is not established (`zstrdup(argv[1])` in the
// configuration's reader). Calls are the calls reaching the file, each
// site once.
type DataFile struct {
	Field  string      `json:"field,omitempty"`
	Values []DataValue `json:"values,omitempty"`
	Calls  []DataCall  `json:"calls"`
}

// DataValue is one write of a file's field: the path it stores as the walk
// reads it (a literal or a template), empty when not established; the
// native declaration writing it; and the site of the write.
type DataValue struct {
	Value    string `json:"value,omitempty"`
	ObjectID string `json:"object_id,omitempty"`
	Anchor   Anchor `json:"anchor"`
}

// DataCall is one call reaching a file: the outside symbol called
// (`stdio.h.fopen`), the native declaration making the call and its site.
type DataCall struct {
	Symbol   string `json:"symbol"`
	ObjectID string `json:"object_id,omitempty"`
	Anchor   Anchor `json:"anchor"`
}

func (data *DataObject) Validate() error {
	if data == nil {
		return nil
	}
	if data.Kind == "file" {
		return data.validateFile()
	}
	if (data.Kind != "table" && data.Kind != "query") || data.Scope == "" || data.Name == "" || data.File != nil {
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

// validateFile holds a file to its shape: its calls, a field's values only
// with its field, and no table's evidence.
func (data *DataObject) validateFile() error {
	file := data.File
	if data.Origin != "call" || data.Scope == "" || file == nil || len(file.Calls) == 0 || file.Field == "" && len(file.Values) > 0 ||
		data.Owner != nil || data.SQL != "" || data.Statement != "" || len(data.Tables) > 0 || len(data.Columns) > 0 {
		return fmt.Errorf("a data file needs its program and the calls reaching it")
	}
	for _, value := range file.Values {
		if err := value.Anchor.validate(); err != nil || value.Anchor.Line < 1 {
			return fmt.Errorf("a data file's value needs its write site")
		}
	}
	for _, call := range file.Calls {
		if err := call.Anchor.validate(); err != nil || call.Symbol == "" || call.Anchor.Line < 1 {
			return fmt.Errorf("a data file's call needs its symbol and site")
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
	if data.File != nil {
		file := *data.File
		file.Values = append([]DataValue(nil), data.File.Values...)
		file.Calls = append([]DataCall(nil), data.File.Calls...)
		out.File = &file
	}
	return &out
}
