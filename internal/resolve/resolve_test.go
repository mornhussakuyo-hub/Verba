package resolve

import (
	"testing"

	"github.com/verba-lang/verba/internal/ast"
	"github.com/verba-lang/verba/internal/diagnostic"
	"github.com/verba-lang/verba/internal/parser"
)

func TestFilesResolvesCapabilitiesAndDependencies(t *testing.T) {
	source := []byte(`module example
use http
use json
use uuid
use client_sdk
record request
begin
    field id uuid
end
route create
method post
path /create
begin
    let payload to be try call json_decode request request_body
    respond json 200 payload
end
`)
	file, parseDiagnostics := parser.Parse("main.vrb", source)
	if len(parseDiagnostics) != 0 {
		t.Fatalf("parse diagnostics = %#v", parseDiagnostics)
	}
	result, diagnostics := Files([]*ast.File{file}, Options{Dependencies: map[string]string{"client_sdk": "1.0"}, ManifestPath: "verba.toml"})
	if diagnostic.HasErrors(diagnostics) {
		t.Fatalf("resolve diagnostics = %#v", diagnostics)
	}
	if len(result.Capabilities) != 3 || len(result.Dependencies) != 1 || !result.Dependencies[0].Used {
		t.Fatalf("Files() result = %#v", result)
	}
}

func TestFilesReportsMissingAndUnknownCapabilities(t *testing.T) {
	source := []byte(`module example
use mystery
route create
method get
path /
begin
    respond json 200 ready
end
`)
	file, parseDiagnostics := parser.Parse("main.vrb", source)
	if len(parseDiagnostics) != 0 {
		t.Fatalf("parse diagnostics = %#v", parseDiagnostics)
	}
	_, diagnostics := Files([]*ast.File{file}, Options{})
	for _, code := range []string{"VRB1706", "VRB1710"} {
		if !resolveHasCode(diagnostics, code) {
			t.Fatalf("missing %s in diagnostics %#v", code, diagnostics)
		}
	}
}

func TestFilesChecksManifestSQLDialect(t *testing.T) {
	source := []byte("module example\nuse sql postgres\nembed sql query until done\nselect 1\ndone\n")
	file, _ := parser.Parse("main.vrb", source)
	_, diagnostics := Files([]*ast.File{file}, Options{DatabaseDialect: "sqlite"})
	if !resolveHasCode(diagnostics, "VRB1704") || !resolveHasCode(diagnostics, "VRB1710") {
		t.Fatalf("resolve diagnostics = %#v", diagnostics)
	}
}

func resolveHasCode(items []diagnostic.Diagnostic, code string) bool {
	for _, item := range items {
		if item.Code == code {
			return true
		}
	}
	return false
}
