package manifest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/verba-lang/verba/internal/diagnostic"
)

func TestLoadValidManifest(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "schema.sql"), []byte("CREATE TABLE users (id text);\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, Filename)
	content := []byte("name = \"users\"\nversion = \"0.1.0\"\ntarget = \"go\"\n\n[database]\ndialect = \"postgres\"\nschema = \"schema.sql\"\n\n[dependencies]\nverba_http = \"0.1\"\n")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
	value, diagnostics, err := Load(path)
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("Load() = %#v, %#v, %v", value, diagnostics, err)
	}
	if value.Name != "users" || value.Database == nil || value.Database.SchemaPath != filepath.Join(directory, "schema.sql") || value.Dependencies["verba_http"] != "0.1" {
		t.Fatalf("Load() value = %#v", value)
	}
}

func TestLoadRejectsUnknownFieldAndEscapingSchema(t *testing.T) {
	directory := t.TempDir()
	unknown := filepath.Join(directory, Filename)
	if err := os.WriteFile(unknown, []byte("name = \"users\"\nversion = \"0.1.0\"\ntarget = \"go\"\nunknown = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, diagnostics, err := Load(unknown)
	if err != nil || !hasCode(diagnostics, "VRB0901") {
		t.Fatalf("unknown field diagnostics = %#v, %v", diagnostics, err)
	}
	escaping := filepath.Join(directory, "escaping.toml")
	if err := os.WriteFile(escaping, []byte("name = \"users\"\nversion = \"0.1.0\"\ntarget = \"go\"\n[database]\ndialect = \"postgres\"\nschema = \"../schema.sql\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, diagnostics, err = Load(escaping)
	if err != nil || !hasCode(diagnostics, "VRB0907") {
		t.Fatalf("escaping schema diagnostics = %#v, %v", diagnostics, err)
	}
}

func TestFindWalksParentDirectories(t *testing.T) {
	directory := t.TempDir()
	nested := filepath.Join(directory, "src", "api")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, Filename)
	if err := os.WriteFile(path, []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}
	found, err := Find(nested)
	if err != nil || found != path {
		t.Fatalf("Find() = %q, %v; want %q", found, err, path)
	}
}

func TestLoadRejectsNonSQLSchema(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "schema.txt"), []byte("schema"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, Filename)
	content := []byte("name = \"users\"\nversion = \"0.1.0\"\ntarget = \"go\"\n[database]\ndialect = \"postgres\"\nschema = \"schema.txt\"\n")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
	_, diagnostics, err := Load(path)
	if err != nil || !hasCode(diagnostics, "VRB0907") {
		t.Fatalf("non-SQL schema diagnostics = %#v, %v", diagnostics, err)
	}
}

func hasCode(items []diagnostic.Diagnostic, code string) bool {
	for _, item := range items {
		if item.Code == code {
			return true
		}
	}
	return false
}
