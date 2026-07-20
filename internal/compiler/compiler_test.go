package compiler

import (
	"os"
	"path/filepath"
	"testing"

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
