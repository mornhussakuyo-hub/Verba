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
