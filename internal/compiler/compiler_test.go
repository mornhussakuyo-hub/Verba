package compiler

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/verba-lang/verba/internal/ast"
	"github.com/verba-lang/verba/internal/diagnostic"
)

func TestLoadUsesManifestAndValidatesModule(t *testing.T) {
	directory := t.TempDir()
	writeCompilerTestFile(t, filepath.Join(directory, "verba.toml"), []byte("name = \"users\"\nversion = \"0.1.0\"\ntarget = \"go\"\n"))
	writeCompilerTestFile(t, filepath.Join(directory, "main.vrb"), []byte("module other\n"))

	program, diagnostics, err := Load([]string{directory})
	if err != nil {
		t.Fatal(err)
	}
	if program.Manifest == nil || program.Name() != "users" || !compilerHasCode(diagnostics, "VRB1003") {
		t.Fatalf("Load() = %#v, %#v", program, diagnostics)
	}
}

func TestLoadRejectsInvalidSourceEncoding(t *testing.T) {
	directory := t.TempDir()
	writeCompilerTestFile(t, filepath.Join(directory, "main.vrb"), []byte{'m', 'o', 'd', 'u', 'l', 'e', ' ', 0xff, '\n'})

	program, diagnostics, err := Load([]string{directory})
	if err != nil {
		t.Fatal(err)
	}
	if len(program.Sources) != 1 || len(program.Files) != 0 || !compilerHasCode(diagnostics, "VRB0001") {
		t.Fatalf("Load() = %#v, %#v", program, diagnostics)
	}
}

func TestLoadAnalyzesPostgresIslands(t *testing.T) {
	directory := t.TempDir()
	writeCompilerTestFile(t, filepath.Join(directory, "verba.toml"), []byte("name = \"users\"\nversion = \"0.1.0\"\ntarget = \"go\"\n[database]\ndialect = \"postgres\"\nschema = \"schema.sql\"\n"))
	writeCompilerTestFile(t, filepath.Join(directory, "schema.sql"), []byte("CREATE TABLE users (id uuid PRIMARY KEY, name text NOT NULL);\n"))
	writeCompilerTestFile(t, filepath.Join(directory, "main.vrb"), []byte(`module users
use sql postgres
use uuid
record user
begin
    field id uuid
    field name string
end
embed sql find_user until end_sql
SELECT id, name FROM users WHERE id = :id;
end_sql
function find
input id uuid
output result user string
begin
    return call sql_one find_user
    begin
        with id id
    end
end
`))

	program, diagnostics, err := Load([]string{directory})
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", diagnostics)
	}
	if len(program.Files) != 1 {
		t.Fatalf("files = %#v", program.Files)
	}
	embed := program.Files[0].Decls[1].(*ast.Embed)
	if embed.SQL == nil || embed.SQL.Text != "SELECT id, name FROM users WHERE id = $1;" || embed.SQL.RowType.Name != "user" {
		t.Fatalf("SQL metadata = %#v", embed.SQL)
	}
}

func TestLoadRequiresDatabaseForSQL(t *testing.T) {
	directory := t.TempDir()
	writeCompilerTestFile(t, filepath.Join(directory, "main.vrb"), []byte("module users\nembed sql query until done\nSELECT 1;\ndone\n"))
	_, diagnostics, err := Load([]string{directory})
	if err != nil {
		t.Fatal(err)
	}
	if !compilerHasCode(diagnostics, "SQL2408") {
		t.Fatalf("expected SQL2408, got %#v", diagnostics)
	}
}

func TestLoadRequiresDatabaseForTransaction(t *testing.T) {
	directory := t.TempDir()
	writeCompilerTestFile(t, filepath.Join(directory, "main.vrb"), []byte(`module users
use sql postgres
function update
begin
    transaction database
    begin
    end
end
`))
	_, diagnostics, err := Load([]string{directory})
	if err != nil {
		t.Fatal(err)
	}
	if !compilerHasCode(diagnostics, "SQL2408") {
		t.Fatalf("expected SQL2408, got %#v", diagnostics)
	}
}

func writeCompilerTestFile(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
}

func compilerHasCode(items []diagnostic.Diagnostic, code string) bool {
	for _, item := range items {
		if item.Code == code {
			return true
		}
	}
	return false
}
