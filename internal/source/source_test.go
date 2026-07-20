package source

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileIndexesMixedNewlinesAndUnicode(t *testing.T) {
	file, diagnostics := New("example.vrb", []byte("module example\r\nnote 中文\rroute x\n"))
	if len(diagnostics) != 0 {
		t.Fatalf("New() diagnostics = %#v", diagnostics)
	}
	lines := file.Lines()
	if len(lines) != 4 || file.LineText(lines[1]) != "note 中文" {
		t.Fatalf("Lines() = %#v", lines)
	}
	location := file.Position(lines[1].End)
	if location.Line != 2 || location.Column != 8 {
		t.Fatalf("Position() = %#v, want line 2 column 8", location)
	}
}

func TestFileRejectsBOMAndInvalidUTF8(t *testing.T) {
	_, bomDiagnostics := New("bom.vrb", []byte{0xef, 0xbb, 0xbf, 'm'})
	if len(bomDiagnostics) != 1 || bomDiagnostics[0].Code != "VRB0002" {
		t.Fatalf("BOM diagnostics = %#v", bomDiagnostics)
	}
	_, utf8Diagnostics := New("invalid.vrb", []byte{'a', '\n', 0xff})
	if len(utf8Diagnostics) != 1 || utf8Diagnostics[0].Code != "VRB0001" || utf8Diagnostics[0].Line != 2 {
		t.Fatalf("UTF-8 diagnostics = %#v", utf8Diagnostics)
	}
}

func TestManagerDeduplicatesAbsolutePaths(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "main.vrb")
	if err := os.WriteFile(path, []byte("module example\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	manager := NewManager()
	first, diagnostics, err := manager.Load(path)
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("first Load() = %#v, %#v, %v", first, diagnostics, err)
	}
	second, diagnostics, err := manager.Load(filepath.Join(directory, ".", "main.vrb"))
	if err != nil || len(diagnostics) != 0 || first != second || len(manager.Files()) != 1 {
		t.Fatalf("second Load() did not deduplicate: %#v, %#v, %v", second, diagnostics, err)
	}
}
