package parser

import (
	"strings"
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
	tests := []struct {
		name   string
		source string
		owner  string
	}{
		{name: "record", source: "module broken\nrecord profile\nbegin\nfield name string\n", owner: "record profile"},
		{name: "enum", source: "module broken\nenum role\nbegin\ncase admin\n", owner: "enum role"},
		{name: "function", source: "module broken\nfunction load\nbegin\nlet value to be 1\n", owner: "function load"},
		{name: "route", source: "module broken\nroute health\nmethod get\npath /health\nbegin\nrespond empty 204\n", owner: "route health"},
		{name: "nested if", source: "module broken\nfunction load\nbegin\nif true\nbegin\nreturn\n", owner: "function load"},
		{name: "nested match", source: "module broken\nfunction load\nbegin\nmatch true\nbegin\ncase true\nbegin\nreturn\n", owner: "function load"},
		{name: "argument block", source: "module broken\nfunction load\nbegin\ncall fetch\nbegin\nwith id 1\n", owner: "function load"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, diagnostics := Parse("broken.vrb", []byte(test.source))
			if len(diagnostics) == 0 {
				t.Fatal("expected a missing-end diagnostic")
			}
			found := false
			for _, item := range diagnostics {
				if item.Code == "VRB0203" || item.Code == "VRB0212" || item.Code == "VRB0311" || item.Code == "VRB0414" || item.Code == "VRB0511" {
					found = true
					if !strings.Contains(item.Message, test.owner) {
						t.Fatalf("diagnostic does not name %s: %#v", test.owner, item)
					}
				}
			}
			if !found {
				t.Fatalf("expected a missing-end diagnostic, got %#v", diagnostics)
			}
		})
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

func TestParseIslandPreservesOriginalNewlinesAndOffsets(t *testing.T) {
	source := []byte("module example\r\nembed text message until done\r\nfirst\r\nsecond\r\ndone\r\n")
	file, diagnostics := Parse("island.vrb", source)
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", diagnostics)
	}
	embed := file.Decls[0].(*ast.Embed)
	if embed.Raw != "first\r\nsecond" || embed.RawStart <= embed.Pos.Offset || embed.RawEnd-embed.RawStart != len(embed.Raw) {
		t.Fatalf("unexpected embed: %#v", embed)
	}
}
