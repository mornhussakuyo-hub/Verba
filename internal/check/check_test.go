package check

import (
	"testing"

	"github.com/verba-lang/verba/internal/ast"
	"github.com/verba-lang/verba/internal/diagnostic"
	"github.com/verba-lang/verba/internal/parser"
)

func TestSQLMissingBinding(t *testing.T) {
	source := []byte(`module example
embed sql find_user until end_sql
SELECT id FROM users WHERE id = :id AND tenant = :tenant;
end_sql
function load
input id string
begin
    call sql_one find_user
    begin
        with id id
    end
end
`)
	file, parseDiagnostics := parser.Parse("sql.vrb", source)
	if len(parseDiagnostics) != 0 {
		t.Fatalf("parse diagnostics: %#v", parseDiagnostics)
	}
	diagnostics := Files([]*ast.File{file})
	if !hasCode(diagnostics, "SQL2107") {
		t.Fatalf("expected SQL2107, got %#v", diagnostics)
	}
}

func TestSQLUsesSchemaTypesAndDeclaredRows(t *testing.T) {
	source := []byte(`module example
record user
begin
    field id uuid
    field name string
end
embed sql find_user until end_sql
SELECT id, name FROM users WHERE id = :id;
end_sql
function load
input id string
begin
    let result to be call sql_optional find_user
    begin
        with id id
    end
end
`)
	file, parseDiagnostics := parser.Parse("sql.vrb", source)
	if len(parseDiagnostics) != 0 {
		t.Fatalf("parse diagnostics: %#v", parseDiagnostics)
	}
	embed := file.Decls[1].(*ast.Embed)
	embed.SQL = &ast.SQLQuery{
		Parameters: []ast.SQLParameter{{Name: "id", Type: ast.Type{Name: "uuid"}}},
		Columns: []ast.SQLColumn{
			{Name: "id", Type: ast.Type{Name: "uuid"}},
			{Name: "name", Type: ast.Type{Name: "string"}},
		},
	}
	diagnostics := Files([]*ast.File{file})
	if !hasCode(diagnostics, "VRB1422") {
		t.Fatalf("expected typed binding diagnostic, got %#v", diagnostics)
	}
	function := file.Decls[2].(*ast.Function)
	call := function.Body[0].(*ast.LetStmt).Value
	if call.ResolvedType.String() != "result optional user string" || embed.SQL.RowType.Name != "user" {
		t.Fatalf("resolved SQL type = %s, row = %s", call.ResolvedType.String(), embed.SQL.RowType.String())
	}
}

func TestSQLSynthesizesRowAndValidatesCallShape(t *testing.T) {
	source := []byte(`module example
embed sql list_users until end_sql
SELECT id, name FROM users;
end_sql
function load
begin
    let rows to be call sql_many list_users
end
`)
	file, parseDiagnostics := parser.Parse("sql.vrb", source)
	if len(parseDiagnostics) != 0 {
		t.Fatalf("parse diagnostics: %#v", parseDiagnostics)
	}
	embed := file.Decls[0].(*ast.Embed)
	embed.SQL = &ast.SQLQuery{Columns: []ast.SQLColumn{
		{Name: "id", Type: ast.Type{Name: "uuid"}},
		{Name: "name", Type: ast.Type{Name: "string"}},
	}}
	if diagnostics := Files([]*ast.File{file}); len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", diagnostics)
	}
	function := file.Decls[1].(*ast.Function)
	call := function.Body[0].(*ast.LetStmt).Value
	if call.ResolvedType.String() != "result list sql_list_users_row string" {
		t.Fatalf("resolved SQL type = %s", call.ResolvedType.String())
	}
}

func TestSQLMapsContextualDatabaseError(t *testing.T) {
	source := []byte(`module example
enum app_error
begin
    case database_failure
end
record user
begin
    field id uuid
end
embed sql find_user until end_sql
SELECT id FROM users;
end_sql
function load
output result user app_error
begin
    let row to be try call sql_one find_user
    return call ok row
end
`)
	file, parseDiagnostics := parser.Parse("sql.vrb", source)
	if len(parseDiagnostics) != 0 {
		t.Fatalf("parse diagnostics: %#v", parseDiagnostics)
	}
	embed := file.Decls[2].(*ast.Embed)
	embed.SQL = &ast.SQLQuery{Columns: []ast.SQLColumn{{Name: "id", Type: ast.Type{Name: "uuid"}}}}
	if diagnostics := Files([]*ast.File{file}); len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", diagnostics)
	}
	function := file.Decls[3].(*ast.Function)
	call := function.Body[0].(*ast.LetStmt).Value
	if call.CallResultType.String() != "result user app_error" || call.ResolvedType.String() != "user" {
		t.Fatalf("SQL call types = %s, %s", call.CallResultType.String(), call.ResolvedType.String())
	}
}

func TestTransactionRestrictions(t *testing.T) {
	source := []byte(`module example
function invalid
output string
begin
    transaction cache
    begin
        transaction database
        begin
            return text invalid
        end
    end
    return text unreachable
end
`)
	diagnostics := checkSource(t, source)
	for _, code := range []string{"SQL2110", "SQL2111", "SQL2112"} {
		if !hasCode(diagnostics, code) {
			t.Errorf("expected %s, got %#v", code, diagnostics)
		}
	}
}

func TestInvalidJSON(t *testing.T) {
	source := []byte("module example\nembed json value until done\n{bad}\ndone\n")
	file, _ := parser.Parse("json.vrb", source)
	diagnostics := Files([]*ast.File{file})
	if !hasCode(diagnostics, "JSON2001") {
		t.Fatalf("expected JSON2001, got %#v", diagnostics)
	}
}

func TestTypedProgram(t *testing.T) {
	source := []byte(`module example
record profile
begin
    field display_name string
end
record user
begin
    field profile profile
    field nickname optional string
end
function visible_name
input value user
output string
begin
    let nickname to be get value nickname
    let present to be call is_some nickname
    if present
    begin
        return call unwrap nickname
    end
    else
    begin
        return get value profile display_name
    end
end
`)
	if diagnostics := checkSource(t, source); len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", diagnostics)
	}
}

func TestFunctionArgumentTypeMismatch(t *testing.T) {
	source := []byte(`module example
function takes_string
input value string
begin
end
function caller
begin
    call takes_string 42
end
`)
	if diagnostics := checkSource(t, source); !hasCode(diagnostics, "VRB1422") {
		t.Fatalf("expected VRB1422, got %#v", diagnostics)
	}
}

func TestOptionalFieldRequiresUnwrap(t *testing.T) {
	source := []byte(`module example
record profile
begin
    field name string
end
record user
begin
    field profile optional profile
end
function name
input value user
output string
begin
    return get value profile name
end
`)
	if diagnostics := checkSource(t, source); !hasCode(diagnostics, "VRB1440") {
		t.Fatalf("expected VRB1440, got %#v", diagnostics)
	}
}

func TestConditionMustBeBoolean(t *testing.T) {
	source := []byte(`module example
function invalid
begin
    if 1
    begin
        return
    end
end
`)
	if diagnostics := checkSource(t, source); !hasCode(diagnostics, "VRB1422") {
		t.Fatalf("expected VRB1422, got %#v", diagnostics)
	}
}

func TestFunctionMustReturnOnEveryPath(t *testing.T) {
	source := []byte(`module example
function invalid
output string
begin
    let value to be text missing return
end
`)
	if diagnostics := checkSource(t, source); !hasCode(diagnostics, "VRB1501") {
		t.Fatalf("expected VRB1501, got %#v", diagnostics)
	}
}

func TestTryRequiresCompatibleResultError(t *testing.T) {
	source := []byte(`module example
enum first_error
begin
    case failed_first
end
enum second_error
begin
    case failed_second
end
function fallible
output result string first_error
begin
    return call error failed_first
end
function wrapper
output result string second_error
begin
    let value to be try call fallible
    return call ok value
end
`)
	if diagnostics := checkSource(t, source); !hasCode(diagnostics, "VRB1437") {
		t.Fatalf("expected VRB1437, got %#v", diagnostics)
	}
}

func TestInvalidRegex(t *testing.T) {
	source := []byte(`module example
embed regex invalid until done
(
done
`)
	if diagnostics := checkSource(t, source); !hasCode(diagnostics, "REGEX2201") {
		t.Fatalf("expected REGEX2201, got %#v", diagnostics)
	}
}

func TestHTMLTemplateBindings(t *testing.T) {
	source := []byte(`module example
embed html page until done
<h1>{{ title }}</h1><p>{{ body }}</p>
done
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
	diagnostics := checkSource(t, source)
	if !hasCode(diagnostics, "HTML2307") {
		t.Fatalf("expected HTML2307, got %#v", diagnostics)
	}
}

func TestTypedMatch(t *testing.T) {
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
        case member
        begin
            return text member
        end
        else
        begin
            return text unknown
        end
    end
end
`)
	if diagnostics := checkSource(t, source); len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", diagnostics)
	}
}

func TestRouteIsTryErrorBoundary(t *testing.T) {
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
	if diagnostics := checkSource(t, source); len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", diagnostics)
	}
}

func TestNumericLiteralsUseContextualWidths(t *testing.T) {
	source := []byte(`module example
function small_add
input value int8
output int8
begin
    return call add value 1
end
function caller
begin
    call small_add 127
end
`)
	file, parseDiagnostics := parser.Parse("numeric.vrb", source)
	if len(parseDiagnostics) != 0 {
		t.Fatalf("parse diagnostics: %#v", parseDiagnostics)
	}
	if diagnostics := Files([]*ast.File{file}); len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", diagnostics)
	}
	function := file.Decls[0].(*ast.Function)
	returned := function.Body[0].(*ast.ReturnStmt)
	if returned.Value.ResolvedType.Name != "int8" || returned.Value.Args[1].ResolvedType.Name != "int8" {
		t.Fatalf("arithmetic literals were not resolved as int8: %#v", returned.Value)
	}
}

func TestNumericLiteralRangeDiagnostics(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{
			name: "function argument",
			source: `module example
function take
input value uint8
begin
end
function caller
begin
    call take 256
end
`,
		},
		{
			name: "set target",
			source: `module example
function seed
output int8
begin
    return 0
end
function update
begin
    var value to be call seed
    set value to be 128
end
`,
		},
		{
			name: "unsigned negative",
			source: `module example
function invalid
output uint
begin
    return -1
end
`,
		},
		{
			name: "float32 overflow",
			source: `module example
function invalid
output float32
begin
    return 3.4028236e38
end
`,
		},
		{
			name: "default integer overflow",
			source: `module example
function invalid
begin
    let value to be 9223372036854775808
end
`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if diagnostics := checkSource(t, []byte(test.source)); !hasCode(diagnostics, "VRB1460") {
				t.Fatalf("expected VRB1460, got %#v", diagnostics)
			}
		})
	}
}

func TestDecimalArithmeticIsContextuallyTyped(t *testing.T) {
	source := []byte(`module example
function exact_total
output decimal
begin
    return call add 0.1 0.2
end
`)
	file, parseDiagnostics := parser.Parse("decimal.vrb", source)
	if len(parseDiagnostics) != 0 {
		t.Fatalf("parse diagnostics: %#v", parseDiagnostics)
	}
	if diagnostics := Files([]*ast.File{file}); len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", diagnostics)
	}
	function := file.Decls[0].(*ast.Function)
	returned := function.Body[0].(*ast.ReturnStmt)
	if returned.Value.ResolvedType.Name != "decimal" {
		t.Fatalf("decimal expression resolved as %s", returned.Value.ResolvedType.String())
	}
	for _, argument := range returned.Value.Args {
		if argument.ResolvedType.Name != "decimal" {
			t.Fatalf("decimal argument resolved as %s", argument.ResolvedType.String())
		}
	}
}

func checkSource(t *testing.T, source []byte) []diagnostic.Diagnostic {
	t.Helper()
	file, parseDiagnostics := parser.Parse("test.vrb", source)
	if len(parseDiagnostics) != 0 {
		t.Fatalf("parse diagnostics: %#v", parseDiagnostics)
	}
	return Files([]*ast.File{file})
}

func hasCode(items []diagnostic.Diagnostic, code string) bool {
	for _, item := range items {
		if item.Code == code {
			return true
		}
	}
	return false
}
