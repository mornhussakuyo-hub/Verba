package emitgo

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/verba-lang/verba/internal/ast"
	"github.com/verba-lang/verba/internal/check"
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

func TestMinimalProgramOmitsUnusedRuntime(t *testing.T) {
	source := []byte(`module minimal
route health
method get
path /health
begin
    respond text 200 ready
end
`)
	file, parseDiagnostics := verbaParser.Parse("minimal.vrb", source)
	if len(parseDiagnostics) != 0 {
		t.Fatalf("parse diagnostics: %#v", parseDiagnostics)
	}
	generated, diagnostics := Files([]*ast.File{file})
	if len(diagnostics) != 0 {
		t.Fatalf("emit diagnostics: %#v", diagnostics)
	}
	text := string(generated)
	for _, unused := range []string{`"crypto/rand"`, `"encoding/json"`, `"html"`, `"regexp"`, `"strings"`, `"time"`, "type Result[", "renderTemplate", "decodeJSON", "parseUUID"} {
		if strings.Contains(text, unused) {
			t.Fatalf("minimal generated source unexpectedly contains %q:\n%s", unused, text)
		}
	}
}

func TestEmitTypedPostgresRuntimeAndTransaction(t *testing.T) {
	source := []byte(`module example
enum app_error
begin
    case database_failure
end
record user
begin
    field id uuid
    field balance decimal
end
embed sql find_user until end_find
SELECT id, balance FROM users WHERE id = :id;
end_find
embed sql rename_user until end_rename
UPDATE users SET name = :name WHERE id = :id;
end_rename
function load
input id uuid
output result user app_error
begin
    return call sql_one find_user
    begin
        with id id
    end
end
route rename
method post
path /users/{id}
begin
    transaction database
    begin
        try call sql_exec rename_user
        begin
            with name text updated
            with id id
        end
    end
    respond empty 204
end
`)
	file, parseDiagnostics := verbaParser.Parse("postgres.vrb", source)
	if len(parseDiagnostics) != 0 {
		t.Fatalf("parse diagnostics: %#v", parseDiagnostics)
	}
	findUser := file.Decls[2].(*ast.Embed)
	findUser.SQL = &ast.SQLQuery{
		Statement:  "select",
		Text:       "SELECT id, balance FROM users WHERE id = $1;",
		Parameters: []ast.SQLParameter{{Name: "id", Type: ast.Type{Name: "uuid"}}},
		Columns: []ast.SQLColumn{
			{Name: "id", Type: ast.Type{Name: "uuid"}},
			{Name: "balance", Type: ast.Type{Name: "decimal"}},
		},
	}
	renameUser := file.Decls[3].(*ast.Embed)
	renameUser.SQL = &ast.SQLQuery{
		Statement: "update",
		Text:      "UPDATE users SET name = $1 WHERE id = $2;",
		Parameters: []ast.SQLParameter{
			{Name: "name", Type: ast.Type{Name: "string"}},
			{Name: "id", Type: ast.Type{Name: "string"}},
		},
	}
	if diagnostics := check.Files([]*ast.File{file}); len(diagnostics) != 0 {
		t.Fatalf("check diagnostics: %#v", diagnostics)
	}
	generated, diagnostics := Files([]*ast.File{file})
	if len(diagnostics) != 0 {
		t.Fatalf("emit diagnostics: %#v", diagnostics)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "main.go", generated, parser.AllErrors); err != nil {
		t.Fatalf("generated Go is invalid: %v\n%s", err, generated)
	}
	text := string(generated)
	for _, expected := range []string{
		`_ "github.com/jackc/pgx/v5/stdlib"`,
		`sql.Open("pgx", databaseURL)`,
		`func scanFindUserRow(scanner sqlScanner) (User, error)`,
		`mapSQLError(sqlOne[User](verbaSQLExecutor, verbaSQLContext, find_user, scanFindUserRow, id), AppErrorDatabaseFailure)`,
		`database.BeginTx(verbaSQLContext, nil)`,
		`defer verbaTemp`,
		`.Rollback()`,
		`.Commit()`,
		`func (value *Decimal) Scan(input any) error`,
		`func (value Decimal) Value() (driver.Value, error)`,
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("generated source does not contain %q:\n%s", expected, text)
		}
	}
}
