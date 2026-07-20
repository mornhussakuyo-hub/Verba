package sqlpostgres

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/verba-lang/verba/internal/ast"
	"github.com/verba-lang/verba/internal/diagnostic"
)

func TestLoadSchema(t *testing.T) {
	path := writeSchema(t, `
CREATE TABLE public.users (
	    id uuid,
    name text NOT NULL,
    email varchar(320),
    age integer,
    balance numeric(20, 4) NOT NULL,
    created_at timestamptz NOT NULL,
	    payload jsonb,
	    PRIMARY KEY (id)
);
`)
	schema, diagnostics, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", diagnostics)
	}
	table := schema.Tables["public.users"]
	if table == nil || schema.Tables["users"] != table {
		t.Fatalf("qualified and short table lookup failed: %#v", schema.Tables)
	}
	want := map[string]string{
		"id":         "uuid",
		"name":       "string",
		"email":      "optional string",
		"age":        "optional int32",
		"balance":    "decimal",
		"created_at": "time",
		"payload":    "optional bytes",
	}
	for name, typeName := range want {
		column, exists := table.byName[name]
		if !exists || column.Type.String() != typeName {
			t.Errorf("column %s = %#v, want %s", name, column, typeName)
		}
	}
}

func TestLoadSchemaDiagnostics(t *testing.T) {
	tests := []struct {
		name string
		sql  string
		code string
	}{
		{"unsupported type", `CREATE TABLE samples (location point);`, "SQL2406"},
		{"unsupported array", `CREATE TABLE samples (labels text[]);`, "SQL2406"},
		{"duplicate table", `CREATE TABLE samples (id int); CREATE TABLE samples (id int);`, "SQL2403"},
		{"duplicate column", `CREATE TABLE samples (id int, id text);`, "SQL2407"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, diagnostics, err := Load(writeSchema(t, test.sql))
			if err != nil {
				t.Fatal(err)
			}
			if !hasDiagnostic(diagnostics, test.code) {
				t.Fatalf("expected %s, got %#v", test.code, diagnostics)
			}
		})
	}
}

func TestAnalyzeSelectAndParameters(t *testing.T) {
	schema := loadTestSchema(t)
	embed := queryEmbed(`SELECT id, name AS display_name, email
FROM users
WHERE id = :user_id OR id = :user_id;`)
	diagnostics := Analyze(embed, schema)
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", diagnostics)
	}
	if embed.SQL.Statement != "select" {
		t.Fatalf("statement = %q", embed.SQL.Statement)
	}
	if embed.SQL.Text != `SELECT id, name AS display_name, email
FROM users
WHERE id = $1 OR id = $1;` {
		t.Fatalf("rewritten query = %q", embed.SQL.Text)
	}
	if len(embed.SQL.Parameters) != 1 || embed.SQL.Parameters[0].Name != "user_id" || embed.SQL.Parameters[0].Type.String() != "uuid" {
		t.Fatalf("parameters = %#v", embed.SQL.Parameters)
	}
	wantColumns := []ast.SQLColumn{
		{Name: "id", Type: ast.Type{Name: "uuid"}},
		{Name: "display_name", Type: ast.Type{Name: "string"}},
		{Name: "email", Type: ast.Type{Name: "optional", Args: []ast.Type{{Name: "string"}}}},
	}
	if len(embed.SQL.Columns) != len(wantColumns) {
		t.Fatalf("columns = %#v", embed.SQL.Columns)
	}
	for index, want := range wantColumns {
		got := embed.SQL.Columns[index]
		if got.Name != want.Name || got.Type.String() != want.Type.String() {
			t.Errorf("column %d = %#v, want %#v", index, got, want)
		}
	}
}

func TestAnalyzeInsertReturning(t *testing.T) {
	schema := loadTestSchema(t)
	embed := queryEmbed(`INSERT INTO users (id, name, email)
VALUES (:id, :name, :email)
RETURNING *;`)
	if diagnostics := Analyze(embed, schema); len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", diagnostics)
	}
	wantParameters := map[string]string{"id": "uuid", "name": "string", "email": "string"}
	for _, parameter := range embed.SQL.Parameters {
		if parameter.Type.String() != wantParameters[parameter.Name] {
			t.Errorf("parameter %s = %s", parameter.Name, parameter.Type.String())
		}
	}
	if len(embed.SQL.Columns) != 5 || embed.SQL.Columns[4].Name != "manager_id" {
		t.Fatalf("returning columns = %#v", embed.SQL.Columns)
	}
}

func TestAnalyzeDoesNotTreatPostgresCastAsParameter(t *testing.T) {
	schema := loadTestSchema(t)
	embed := queryEmbed(`SELECT id FROM users WHERE id = :id::uuid;`)
	if diagnostics := Analyze(embed, schema); len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", diagnostics)
	}
	if len(embed.SQL.Parameters) != 1 || embed.SQL.Parameters[0].Name != "id" || embed.SQL.Text != `SELECT id FROM users WHERE id = $1::uuid;` {
		t.Fatalf("SQL metadata = %#v", embed.SQL)
	}
}

func TestAnalyzeQueryDiagnostics(t *testing.T) {
	schema := loadTestSchema(t)
	tests := []struct {
		name string
		sql  string
		code string
	}{
		{"unknown table", `SELECT * FROM accounts;`, "SQL2414"},
		{"unknown column", `SELECT missing FROM users;`, "SQL2418"},
		{"result expression", `SELECT count(*) FROM users;`, "SQL2417"},
		{"duplicate result", `SELECT id, id FROM users;`, "SQL2419"},
		{"unknown parameter", `SELECT id FROM users WHERE :value IS NOT NULL;`, "SQL2415"},
		{"multiple statements", `SELECT id FROM users; DELETE FROM users;`, "SQL2412"},
		{"join", `SELECT users.id FROM users JOIN managers ON managers.id = users.manager_id;`, "SQL2423"},
		{"unknown insert column", `INSERT INTO users (missing) VALUES (:value);`, "SQL2424"},
		{"unknown update column", `UPDATE users SET missing = :value;`, "SQL2424"},
		{"insert arity", `INSERT INTO users (id, name) VALUES (:id);`, "SQL2426"},
		{"unterminated string", `SELECT id FROM users WHERE name = 'broken;`, "SQL2421"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			embed := queryEmbed(test.sql)
			diagnostics := Analyze(embed, schema)
			if !hasDiagnostic(diagnostics, test.code) {
				t.Fatalf("expected %s, got %#v", test.code, diagnostics)
			}
		})
	}
}

func loadTestSchema(t *testing.T) *Schema {
	t.Helper()
	schema, diagnostics, err := Load(writeSchema(t, `
CREATE TABLE users (
    id uuid PRIMARY KEY,
    name text NOT NULL,
    email text,
    created_at timestamptz NOT NULL,
    manager_id uuid
);
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 0 {
		t.Fatalf("schema diagnostics: %#v", diagnostics)
	}
	return schema
}

func writeSchema(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "schema.sql")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func queryEmbed(raw string) *ast.Embed {
	return &ast.Embed{Name: "query", Kind: "sql", Raw: raw, Pos: ast.Position{File: "query.vrb", Line: 1}, RawStart: 1}
}

func hasDiagnostic(items []diagnostic.Diagnostic, code string) bool {
	for _, item := range items {
		if item.Code == code {
			return true
		}
	}
	return false
}
