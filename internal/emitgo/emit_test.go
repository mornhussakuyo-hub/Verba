package emitgo

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/verba-lang/verba/internal/ast"
	verbaParser "github.com/verba-lang/verba/internal/parser"
)

func TestEmitFunctionAndRoute(t *testing.T) {
	source := []byte(`module example
function normalize
input value string
output string
begin
    return value
end
route echo
method get
path /echo/{value}
begin
    let result to be call normalize
    begin
        with value value
    end
    respond json 200 result
end
`)
	file, parseDiagnostics := verbaParser.Parse("example.vrb", source)
	if len(parseDiagnostics) != 0 {
		t.Fatalf("parse diagnostics: %#v", parseDiagnostics)
	}
	generated, diagnostics := Files([]*ast.File{file})
	if len(diagnostics) != 0 {
		t.Fatalf("emit diagnostics: %#v", diagnostics)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "main.go", generated, parser.AllErrors); err != nil {
		t.Fatalf("generated Go is invalid: %v\n%s", err, generated)
	}
	if !strings.Contains(string(generated), "func normalize(value string) string") {
		t.Fatalf("generated function lost its result type:\n%s", generated)
	}
}

func TestEmitCompiledRegexAndEscapedTemplate(t *testing.T) {
	source := []byte(`module example
embed regex word until end_word
^[a-z]+$
end_word
embed html page until end_page
<h1>{{ title }}</h1>
end_page
function matches
input value string
output bool
begin
    return call regex_match word value
end
function render_page
input title string
output string
begin
    return call render page
    begin
        with title title
    end
end
`)
	file, parseDiagnostics := verbaParser.Parse("example.vrb", source)
	if len(parseDiagnostics) != 0 {
		t.Fatalf("parse diagnostics: %#v", parseDiagnostics)
	}
	generated, diagnostics := Files([]*ast.File{file})
	if len(diagnostics) != 0 {
		t.Fatalf("emit diagnostics: %#v", diagnostics)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "main.go", generated, parser.AllErrors); err != nil {
		t.Fatalf("generated Go is invalid: %v\n%s", err, generated)
	}
	text := string(generated)
	for _, expected := range []string{"regexp.MustCompile", "word.MatchString(value)", "html.EscapeString", "renderTemplate(page"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("generated source does not contain %q:\n%s", expected, text)
		}
	}
}

func TestEmitMatchAsSwitch(t *testing.T) {
	source := []byte(`module example
enum role
begin
    case admin
    case member
end
function label
input value role
output string
begin
    match value
    begin
        case admin
        begin
            return text administrator
        end
        else
        begin
            return text user
        end
    end
end
`)
	file, parseDiagnostics := verbaParser.Parse("match.vrb", source)
	if len(parseDiagnostics) != 0 {
		t.Fatalf("parse diagnostics: %#v", parseDiagnostics)
	}
	generated, diagnostics := Files([]*ast.File{file})
	if len(diagnostics) != 0 {
		t.Fatalf("emit diagnostics: %#v", diagnostics)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "main.go", generated, parser.AllErrors); err != nil {
		t.Fatalf("generated Go is invalid: %v\n%s", err, generated)
	}
	if !strings.Contains(string(generated), "switch value") {
		t.Fatalf("generated match does not use switch:\n%s", generated)
	}
}

func TestEmitFallibleJSONAndUUIDRuntime(t *testing.T) {
	source := []byte(`module example
record request
begin
    field id string
end
route validate
method post
path /validate
begin
    let payload to be try call json_decode request request_body
    let raw_id to be get payload id
    let parsed_id to be try call parse_uuid raw_id
    respond json 200 parsed_id
end
`)
	file, parseDiagnostics := verbaParser.Parse("runtime.vrb", source)
	if len(parseDiagnostics) != 0 {
		t.Fatalf("parse diagnostics: %#v", parseDiagnostics)
	}
	generated, diagnostics := Files([]*ast.File{file})
	if len(diagnostics) != 0 {
		t.Fatalf("emit diagnostics: %#v", diagnostics)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "main.go", generated, parser.AllErrors); err != nil {
		t.Fatalf("generated Go is invalid: %v\n%s", err, generated)
	}
	text := string(generated)
	for _, expected := range []string{"func decodeJSON[T any](data []byte) Result[T, string]", "func parseUUID", "decodeJSON[Request](request_body)", "parseUUID(raw_id)"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("generated source does not contain %q:\n%s", expected, text)
		}
	}
}
