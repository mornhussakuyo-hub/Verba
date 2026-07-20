package parser

import (
	"testing"

	"github.com/verba-lang/verba/internal/ast"
)

func TestParseRouteAndIsland(t *testing.T) {
	source := []byte(`module example

embed json metadata until end_metadata
{"ok": true}
end_metadata

route health
method get
path /health
begin
    respond text 200 healthy
end
`)
	file, diagnostics := Parse("example.vrb", source)
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", diagnostics)
	}
	if file.Module != "example" || len(file.Decls) != 2 {
		t.Fatalf("unexpected file: %#v", file)
	}
	if _, ok := file.Decls[0].(*ast.Embed); !ok {
		t.Fatalf("first declaration is %T, want *ast.Embed", file.Decls[0])
	}
	route, ok := file.Decls[1].(*ast.Route)
	if !ok || route.Method != "get" || route.Path != "/health" || len(route.Body) != 1 {
		t.Fatalf("unexpected route: %#v", file.Decls[1])
	}
}

func TestMissingEndProducesDiagnostic(t *testing.T) {
	_, diagnostics := Parse("broken.vrb", []byte("module broken\nfunction f\nbegin\nlet x to be 1\n"))
	if len(diagnostics) == 0 || diagnostics[0].Code != "VRB0311" {
		t.Fatalf("expected VRB0311, got %#v", diagnostics)
	}
}

func TestParseMatch(t *testing.T) {
	source := []byte(`module example
function label
input value string
output string
begin
    match value
    begin
        case active
        begin
            return text enabled
        end
        else
        begin
            return text disabled
        end
    end
end
`)
	file, diagnostics := Parse("match.vrb", source)
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", diagnostics)
	}
	function := file.Decls[0].(*ast.Function)
	statement, ok := function.Body[0].(*ast.MatchStmt)
	if !ok || len(statement.Cases) != 1 || len(statement.Else) != 1 {
		t.Fatalf("unexpected match statement: %#v", function.Body[0])
	}
}
