package emitgo

import (
	"bytes"
	"fmt"
	"go/format"
	"strconv"
	"strings"
	"unicode"

	"github.com/verba-lang/verba/internal/ast"
	"github.com/verba-lang/verba/internal/diagnostic"
	"github.com/verba-lang/verba/internal/numeric"
)

type emitter struct {
	files       []*ast.File
	buffer      bytes.Buffer
	diagnostics []diagnostic.Diagnostic
	enumCases   map[string]string
	functions   map[string]*ast.Function
	records     map[string]*ast.Record
	embeds      map[string]*ast.Embed
	features    featureSet
	temp        int
	transaction string
}

type featureSet struct {
	json        bool
	jsonDecode  bool
	regex       bool
	template    bool
	html        bool
	strings     bool
	time        bool
	randomUUID  bool
	parseUUID   bool
	result      bool
	decimal     bool
	floatMath   bool
	sql         bool
	sqlDuration bool
}

func Files(files []*ast.File) ([]byte, []diagnostic.Diagnostic) {
	e := &emitter{files: files, enumCases: map[string]string{}, functions: map[string]*ast.Function{}, records: map[string]*ast.Record{}, embeds: map[string]*ast.Embed{}}
	for _, file := range files {
		for _, decl := range file.Decls {
			switch value := decl.(type) {
			case *ast.Enum:
				for _, item := range value.Cases {
					e.enumCases[item.Name] = exported(value.Name) + exported(item.Name)
				}
			case *ast.Function:
				e.functions[value.Name] = value
			case *ast.Record:
				e.records[value.Name] = value
			case *ast.Embed:
				e.embeds[value.Name] = value
			}
		}
	}
	e.scanFeatures()
	e.validateSupported()
	if diagnostic.HasErrors(e.diagnostics) {
		return nil, e.diagnostics
	}
	e.emitProgram()
	formatted, err := format.Source(e.buffer.Bytes())
	if err != nil {
		e.diagnostics = append(e.diagnostics, diagnostic.Diagnostic{Severity: diagnostic.Error, Code: "VRB3000", File: "<generated>", Line: 1, Column: 1, Message: fmt.Sprintf("generated invalid Go source: %v", err), Hint: "run verba check and report this compiler bug if the source is valid"})
		return e.buffer.Bytes(), e.diagnostics
	}
	return formatted, e.diagnostics
}

func (e *emitter) validateSupported() {
	for _, file := range e.files {
		for _, decl := range file.Decls {
			switch value := decl.(type) {
			case *ast.Function:
				e.validateStatements(value.Body)
			case *ast.Route:
				e.validateStatements(value.Body)
			}
		}
	}
}

func (e *emitter) validateStatements(statements []ast.Stmt) {
	for _, statement := range statements {
		switch value := statement.(type) {
		case *ast.LetStmt:
			e.validateExpr(value.Value)
		case *ast.SetStmt:
			e.validateExpr(value.Value)
		case *ast.ExprStmt:
			e.validateExpr(value.Value)
		case *ast.ReturnStmt:
			if value.Value != nil {
				e.validateExpr(*value.Value)
			}
		case *ast.RespondStmt:
			if value.Value != nil {
				e.validateExpr(*value.Value)
			}
		case *ast.IfStmt:
			e.validateExpr(value.Condition)
			e.validateStatements(value.Then)
			e.validateStatements(value.Else)
		case *ast.ForStmt:
			e.validateExpr(value.Iterable)
			e.validateStatements(value.Body)
		case *ast.WhileStmt:
			e.validateExpr(value.Condition)
			e.validateStatements(value.Body)
		case *ast.MatchStmt:
			e.validateExpr(value.Value)
			for _, matchCase := range value.Cases {
				e.validateExpr(matchCase.Pattern)
				e.validateStatements(matchCase.Body)
			}
			e.validateStatements(value.Else)
		case *ast.TransactionStmt:
			e.validateStatements(value.Body)
		}
	}
}

func (e *emitter) validateExpr(expr ast.Expr) {
	if expr.Kind == ast.ExprCall && strings.HasPrefix(expr.Value, "sql_") {
		if len(expr.Args) == 0 || e.embeds[expr.Args[0].Value] == nil || e.embeds[expr.Args[0].Value].SQL == nil {
			e.error(expr.Pos, "VRB3002", "SQL execution metadata is unavailable", "compile the project with a PostgreSQL schema snapshot configured in verba.toml")
		}
	}
	for _, arg := range expr.Args {
		e.validateExpr(arg)
	}
	for _, arg := range expr.NamedArgs {
		e.validateExpr(arg.Value)
	}
}

func (e *emitter) scanFeatures() {
	for _, file := range e.files {
		for _, decl := range file.Decls {
			switch value := decl.(type) {
			case *ast.Record:
				for _, field := range value.Fields {
					e.scanType(field.Type)
				}
			case *ast.Function:
				for _, input := range value.Inputs {
					e.scanType(input.Type)
				}
				if value.Output != nil {
					e.scanType(*value.Output)
				}
				e.scanStatements(value.Body)
			case *ast.Route:
				if value.Output != nil {
					e.scanType(*value.Output)
				}
				e.scanStatements(value.Body)
			case *ast.Embed:
				if value.Kind == "regex" {
					e.features.regex = true
				} else if value.Kind == "sql" && value.SQL != nil {
					e.features.sql = true
					for _, parameter := range value.SQL.Parameters {
						e.scanType(parameter.Type)
						if sqlScalarType(parameter.Type).Name == "duration" {
							e.features.sqlDuration = true
						}
					}
					for _, column := range value.SQL.Columns {
						e.scanType(column.Type)
						if sqlScalarType(column.Type).Name == "duration" {
							e.features.sqlDuration = true
						}
					}
				}
			}
		}
	}
}

func (e *emitter) scanType(value ast.Type) {
	switch value.Name {
	case "result":
		e.features.result = true
	case "time", "duration":
		e.features.time = true
	case "decimal":
		e.features.decimal = true
		e.features.regex = true
		e.features.strings = true
	}
	for _, argument := range value.Args {
		e.scanType(argument)
	}
}

func (e *emitter) scanStatements(statements []ast.Stmt) {
	for _, statement := range statements {
		switch value := statement.(type) {
		case *ast.LetStmt:
			e.scanExpr(value.Value)
		case *ast.SetStmt:
			e.scanExpr(value.Value)
		case *ast.ExprStmt:
			e.scanExpr(value.Value)
		case *ast.ReturnStmt:
			if value.Value != nil {
				e.scanExpr(*value.Value)
			}
		case *ast.RespondStmt:
			if value.Format == "json" {
				e.features.json = true
			}
			if value.Value != nil {
				e.scanExpr(*value.Value)
			}
		case *ast.IfStmt:
			e.scanExpr(value.Condition)
			e.scanStatements(value.Then)
			e.scanStatements(value.Else)
		case *ast.ForStmt:
			e.scanExpr(value.Iterable)
			e.scanStatements(value.Body)
		case *ast.WhileStmt:
			e.scanExpr(value.Condition)
			e.scanStatements(value.Body)
		case *ast.MatchStmt:
			e.scanExpr(value.Value)
			for _, matchCase := range value.Cases {
				e.scanExpr(matchCase.Pattern)
				e.scanStatements(matchCase.Body)
			}
			e.scanStatements(value.Else)
		case *ast.TransactionStmt:
			e.features.sql = true
			e.scanStatements(value.Body)
		}
	}
}

func (e *emitter) scanExpr(expr ast.Expr) {
	if expr.ResolvedType.Name == "decimal" {
		e.features.decimal = true
		e.features.regex = true
		e.features.strings = true
	}
	if expr.Try {
		e.features.result = true
	}
	if expr.Kind == ast.ExprCall {
		switch expr.Value {
		case "remainder":
			if expr.ResolvedType.Name == "float32" || expr.ResolvedType.Name == "float64" {
				e.features.floatMath = true
			}
		case "trim", "lowercase", "uppercase", "contains", "starts_with", "concat":
			e.features.strings = true
		case "regex_match":
			e.features.regex = true
		case "json_decode":
			e.features.json = true
			e.features.jsonDecode = true
			e.features.result = true
		case "json_encode":
			e.features.json = true
		case "new_uuid":
			e.features.randomUUID = true
		case "parse_uuid":
			e.features.parseUUID = true
			e.features.regex = true
			e.features.strings = true
			e.features.result = true
		case "render":
			e.features.template = true
			e.features.regex = true
			e.features.html = true
		case "ok", "error":
			e.features.result = true
		case "sql_exec", "sql_one", "sql_optional", "sql_many":
			e.features.sql = true
			e.features.result = true
		}
	}
	for _, argument := range expr.Args {
		e.scanExpr(argument)
	}
	for _, argument := range expr.NamedArgs {
		e.scanExpr(argument.Value)
	}
}

func (e *emitter) emitDecimalRuntime() {
	e.line(0, "var decimalLiteralPattern = regexp.MustCompile(%s)", strconv.Quote(`^[+-]?[0-9]+(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?$`))
	e.line(0, "var decimalJSONPattern = regexp.MustCompile(%s)", strconv.Quote(`^-?(?:0|[1-9][0-9]*)(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?$`))
	e.line(0, "")
	e.line(0, "type Decimal struct {")
	e.line(1, "value *big.Rat")
	e.line(0, "}")
	e.line(0, "")
	e.line(0, "func newDecimal(input string) Decimal {")
	e.line(1, "if !decimalLiteralPattern.MatchString(input) { panic(\"invalid compiler-generated decimal literal\") }")
	e.line(1, "value, ok := new(big.Rat).SetString(input)")
	e.line(1, "if !ok { panic(\"invalid compiler-generated decimal literal\") }")
	e.line(1, "return Decimal{value: value}")
	e.line(0, "}")
	e.line(0, "")
	e.line(0, "func (value Decimal) rat() *big.Rat {")
	e.line(1, "if value.value == nil { return new(big.Rat) }")
	e.line(1, "return value.value")
	e.line(0, "}")
	e.line(0, "")
	e.line(0, "func (value Decimal) Add(other Decimal) Decimal { return Decimal{value: new(big.Rat).Add(value.rat(), other.rat())} }")
	e.line(0, "func (value Decimal) Sub(other Decimal) Decimal { return Decimal{value: new(big.Rat).Sub(value.rat(), other.rat())} }")
	e.line(0, "func (value Decimal) Mul(other Decimal) Decimal { return Decimal{value: new(big.Rat).Mul(value.rat(), other.rat())} }")
	e.line(0, "func (value Decimal) Quo(other Decimal) Decimal { return Decimal{value: new(big.Rat).Quo(value.rat(), other.rat())} }")
	e.line(0, "func (value Decimal) Neg() Decimal { return Decimal{value: new(big.Rat).Neg(value.rat())} }")
	e.line(0, "func (value Decimal) Cmp(other Decimal) int { return value.rat().Cmp(other.rat()) }")
	e.line(0, "")
	e.line(0, "func (value Decimal) Rem(other Decimal) Decimal {")
	e.line(1, "quotient := new(big.Rat).Quo(value.rat(), other.rat())")
	e.line(1, "whole := new(big.Int).Quo(quotient.Num(), quotient.Denom())")
	e.line(1, "product := new(big.Rat).Mul(new(big.Rat).SetInt(whole), other.rat())")
	e.line(1, "return Decimal{value: new(big.Rat).Sub(value.rat(), product)}")
	e.line(0, "}")
	e.line(0, "")
	e.line(0, "func decimalText(value *big.Rat) (string, error) {")
	e.line(1, "denominator := new(big.Int).Set(value.Denom())")
	e.line(1, "two := big.NewInt(2)")
	e.line(1, "five := big.NewInt(5)")
	e.line(1, "twos := 0")
	e.line(1, "fives := 0")
	e.line(1, "for new(big.Int).Mod(denominator, two).Sign() == 0 { denominator.Quo(denominator, two); twos++ }")
	e.line(1, "for new(big.Int).Mod(denominator, five).Sign() == 0 { denominator.Quo(denominator, five); fives++ }")
	e.line(1, "if denominator.Cmp(big.NewInt(1)) != 0 { return \"\", fmt.Errorf(\"decimal %%s has no finite JSON representation\", value.RatString()) }")
	e.line(1, "scale := twos")
	e.line(1, "if fives > scale { scale = fives }")
	e.line(1, "text := value.FloatString(scale)")
	e.line(1, "if strings.Contains(text, \".\") { text = strings.TrimRight(strings.TrimRight(text, \"0\"), \".\") }")
	e.line(1, "if text == \"-0\" || text == \"\" { text = \"0\" }")
	e.line(1, "return text, nil")
	e.line(0, "}")
	e.line(0, "")
	e.line(0, "func (value Decimal) String() string {")
	e.line(1, "text, err := decimalText(value.rat())")
	e.line(1, "if err != nil { return value.rat().RatString() }")
	e.line(1, "return text")
	e.line(0, "}")
	e.line(0, "")
	e.line(0, "func (value Decimal) MarshalJSON() ([]byte, error) {")
	e.line(1, "text, err := decimalText(value.rat())")
	e.line(1, "if err != nil { return nil, err }")
	e.line(1, "return []byte(text), nil")
	e.line(0, "}")
	e.line(0, "")
	e.line(0, "func (value *Decimal) UnmarshalJSON(data []byte) error {")
	e.line(1, "input := string(data)")
	e.line(1, "if !decimalJSONPattern.MatchString(input) { return fmt.Errorf(\"invalid decimal JSON number %%q\", input) }")
	e.line(1, "parsed, ok := new(big.Rat).SetString(input)")
	e.line(1, "if !ok { return fmt.Errorf(\"invalid decimal JSON number %%q\", input) }")
	e.line(1, "value.value = parsed")
	e.line(1, "return nil")
	e.line(0, "}")
	e.line(0, "")
	if e.features.sql {
		e.line(0, "func (value *Decimal) Scan(input any) error {")
		e.line(1, "var text string")
		e.line(1, "switch item := input.(type) {")
		e.line(1, "case string:")
		e.line(2, "text = item")
		e.line(1, "case []byte:")
		e.line(2, "text = string(item)")
		e.line(1, "default:")
		e.line(2, "return fmt.Errorf(\"cannot scan decimal from %%T\", input)")
		e.line(1, "}")
		e.line(1, "parsed, ok := new(big.Rat).SetString(text)")
		e.line(1, "if !ok { return fmt.Errorf(\"invalid database decimal %%q\", text) }")
		e.line(1, "value.value = parsed")
		e.line(1, "return nil")
		e.line(0, "}")
		e.line(0, "")
		e.line(0, "func (value Decimal) Value() (driver.Value, error) {")
		e.line(1, "return value.String(), nil")
		e.line(0, "}")
		e.line(0, "")
	}
}

func (e *emitter) emitSQLRuntime() {
	e.line(0, "type sqlExecutor interface {")
	e.line(1, "ExecContext(context.Context, string, ...any) (sql.Result, error)")
	e.line(1, "QueryContext(context.Context, string, ...any) (*sql.Rows, error)")
	e.line(0, "}")
	e.line(0, "")
	e.line(0, "type sqlScanner interface {")
	e.line(1, "Scan(...any) error")
	e.line(0, "}")
	e.line(0, "")
	e.line(0, "var database *sql.DB")
	e.line(0, "")
	if e.features.sqlDuration {
		e.line(0, "type sqlDuration struct {")
		e.line(1, "Duration time.Duration")
		e.line(1, "Valid bool")
		e.line(0, "}")
		e.line(0, "")
		e.line(0, "func (value *sqlDuration) Scan(input any) error {")
		e.line(1, "var interval pgtype.Interval")
		e.line(1, "if err := interval.Scan(input); err != nil { return err }")
		e.line(1, "if !interval.Valid { *value = sqlDuration{}; return nil }")
		e.line(1, "if interval.Months != 0 { return fmt.Errorf(\"PostgreSQL interval with months cannot be represented as duration\") }")
		e.line(1, "value.Duration = time.Duration(interval.Microseconds)*time.Microsecond + time.Duration(interval.Days)*24*time.Hour")
		e.line(1, "value.Valid = true")
		e.line(1, "return nil")
		e.line(0, "}")
		e.line(0, "")
		e.line(0, "func (value sqlDuration) Value() (driver.Value, error) {")
		e.line(1, "if !value.Valid { return nil, nil }")
		e.line(1, "if value.Duration%%time.Microsecond != 0 { return nil, fmt.Errorf(\"duration %%s exceeds PostgreSQL microsecond precision\", value.Duration) }")
		e.line(1, "return pgtype.Interval{Microseconds: int64(value.Duration / time.Microsecond), Valid: true}.Value()")
		e.line(0, "}")
		e.line(0, "")
		e.line(0, "func sqlDurationArgument(value *time.Duration) any {")
		e.line(1, "if value == nil { return nil }")
		e.line(1, "return sqlDuration{Duration: *value, Valid: true}")
		e.line(0, "}")
		e.line(0, "")
	}
	e.line(0, "func sqlError[T any](err error) Result[T, string] {")
	e.line(1, "message := err.Error()")
	e.line(1, "return Result[T, string]{Err: &message}")
	e.line(0, "}")
	e.line(0, "")
	e.line(0, "func mapSQLError[T any, E any](input Result[T, string], mapped E) Result[T, E] {")
	e.line(1, "if input.Err != nil { return Result[T, E]{Err: &mapped} }")
	e.line(1, "return Result[T, E]{Value: input.Value}")
	e.line(0, "}")
	e.line(0, "")
	e.line(0, "func sqlExec(executor sqlExecutor, ctx context.Context, query string, args ...any) Result[int64, string] {")
	e.line(1, "result, err := executor.ExecContext(ctx, query, args...)")
	e.line(1, "if err != nil { return sqlError[int64](err) }")
	e.line(1, "affected, err := result.RowsAffected()")
	e.line(1, "if err != nil { return sqlError[int64](err) }")
	e.line(1, "return Result[int64, string]{Value: affected}")
	e.line(0, "}")
	e.line(0, "")
	e.line(0, "func sqlOne[T any](executor sqlExecutor, ctx context.Context, query string, scan func(sqlScanner) (T, error), args ...any) Result[T, string] {")
	e.line(1, "rows, err := executor.QueryContext(ctx, query, args...)")
	e.line(1, "if err != nil { return sqlError[T](err) }")
	e.line(1, "defer rows.Close()")
	e.line(1, "if !rows.Next() {")
	e.line(2, "if err := rows.Err(); err != nil { return sqlError[T](err) }")
	e.line(2, "return sqlError[T](sql.ErrNoRows)")
	e.line(1, "}")
	e.line(1, "value, err := scan(rows)")
	e.line(1, "if err != nil { return sqlError[T](err) }")
	e.line(1, "if rows.Next() { return sqlError[T](fmt.Errorf(\"query returned more than one row\")) }")
	e.line(1, "if err := rows.Err(); err != nil { return sqlError[T](err) }")
	e.line(1, "return Result[T, string]{Value: value}")
	e.line(0, "}")
	e.line(0, "")
	e.line(0, "func sqlOptional[T any](executor sqlExecutor, ctx context.Context, query string, scan func(sqlScanner) (T, error), args ...any) Result[*T, string] {")
	e.line(1, "rows, err := executor.QueryContext(ctx, query, args...)")
	e.line(1, "if err != nil { return sqlError[*T](err) }")
	e.line(1, "defer rows.Close()")
	e.line(1, "if !rows.Next() {")
	e.line(2, "if err := rows.Err(); err != nil { return sqlError[*T](err) }")
	e.line(2, "return Result[*T, string]{}")
	e.line(1, "}")
	e.line(1, "value, err := scan(rows)")
	e.line(1, "if err != nil { return sqlError[*T](err) }")
	e.line(1, "if rows.Next() { return sqlError[*T](fmt.Errorf(\"query returned more than one row\")) }")
	e.line(1, "if err := rows.Err(); err != nil { return sqlError[*T](err) }")
	e.line(1, "return Result[*T, string]{Value: &value}")
	e.line(0, "}")
	e.line(0, "")
	e.line(0, "func sqlMany[T any](executor sqlExecutor, ctx context.Context, query string, scan func(sqlScanner) (T, error), args ...any) Result[[]T, string] {")
	e.line(1, "rows, err := executor.QueryContext(ctx, query, args...)")
	e.line(1, "if err != nil { return sqlError[[]T](err) }")
	e.line(1, "defer rows.Close()")
	e.line(1, "values := make([]T, 0)")
	e.line(1, "for rows.Next() {")
	e.line(2, "value, err := scan(rows)")
	e.line(2, "if err != nil { return sqlError[[]T](err) }")
	e.line(2, "values = append(values, value)")
	e.line(1, "}")
	e.line(1, "if err := rows.Err(); err != nil { return sqlError[[]T](err) }")
	e.line(1, "return Result[[]T, string]{Value: values}")
	e.line(0, "}")
	e.line(0, "")
}

func (e *emitter) emitRuntime() {
	if e.features.decimal {
		e.emitDecimalRuntime()
	}
	if e.features.template {
		e.line(0, `var templateSlotPattern = regexp.MustCompile(%s)`, strconv.Quote(`\{\{\s*([A-Za-z_][A-Za-z0-9_]*)\s*\}\}`))
	}
	if e.features.parseUUID {
		e.line(0, `var uuidPattern = regexp.MustCompile(%s)`, strconv.Quote(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-5][0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`))
	}
	if e.features.template || e.features.parseUUID {
		e.line(0, "")
	}
	if e.features.result {
		e.line(0, "type Result[T any, E any] struct {")
		e.line(1, "Value T")
		e.line(1, "Err *E")
		e.line(0, "}")
		e.line(0, "")
	}
	if e.features.sql {
		e.emitSQLRuntime()
	}
	if e.features.template {
		e.line(0, "func renderTemplate(source string, values map[string]string, escapeHTML bool) string {")
		e.line(1, "return templateSlotPattern.ReplaceAllStringFunc(source, func(slot string) string {")
		e.line(2, `matches := templateSlotPattern.FindStringSubmatch(slot)`)
		e.line(2, `value := values[matches[1]]`)
		e.line(2, `if escapeHTML { return html.EscapeString(value) }`)
		e.line(2, `return value`)
		e.line(1, "})")
		e.line(0, "}")
		e.line(0, "")
	}
	if e.features.jsonDecode {
		e.line(0, "func decodeJSON[T any](data []byte) Result[T, string] {")
		e.line(1, "var value T")
		e.line(1, "if err := json.Unmarshal(data, &value); err != nil {")
		e.line(2, "message := err.Error()")
		e.line(2, "return Result[T, string]{Err: &message}")
		e.line(1, "}")
		e.line(1, "return Result[T, string]{Value: value}")
		e.line(0, "}")
		e.line(0, "")
	}
	if e.features.randomUUID {
		e.line(0, "func newUUID() string {")
		e.line(1, "var value [16]byte")
		e.line(1, "if _, err := rand.Read(value[:]); err != nil { panic(err) }")
		e.line(1, "value[6] = (value[6] & 0x0f) | 0x40")
		e.line(1, "value[8] = (value[8] & 0x3f) | 0x80")
		e.line(1, `return fmt.Sprintf("%%x-%%x-%%x-%%x-%%x", value[0:4], value[4:6], value[6:8], value[8:10], value[10:16])`)
		e.line(0, "}")
		e.line(0, "")
	}
	if e.features.parseUUID {
		e.line(0, "func parseUUID(input string) Result[string, string] {")
		e.line(1, "if !uuidPattern.MatchString(input) {")
		e.line(2, `message := "invalid UUID"`)
		e.line(2, "return Result[string, string]{Err: &message}")
		e.line(1, "}")
		e.line(1, "return Result[string, string]{Value: strings.ToLower(input)}")
		e.line(0, "}")
		e.line(0, "")
	}
}

func (e *emitter) emitProgram() {
	e.line(0, "// Code generated by verba 0.1.0. DO NOT EDIT.")
	e.line(0, "package main")
	e.line(0, "")
	e.line(0, `import (`)
	imports := []string{"fmt", "io", "net/http", "os"}
	if e.features.sql {
		imports = append(imports, "context", "database/sql")
	}
	if e.features.randomUUID {
		imports = append(imports, "crypto/rand")
	}
	if e.features.json {
		imports = append(imports, "encoding/json")
	}
	if e.features.html {
		imports = append(imports, "html")
	}
	if e.features.floatMath {
		imports = append(imports, "math")
	}
	if e.features.decimal {
		imports = append(imports, "math/big")
	}
	if e.features.sql && (e.features.decimal || e.features.sqlDuration) {
		imports = append(imports, "database/sql/driver")
	}
	if e.features.regex {
		imports = append(imports, "regexp")
	}
	if e.features.strings {
		imports = append(imports, "strings")
	}
	if e.features.time {
		imports = append(imports, "time")
	}
	for _, item := range imports {
		e.line(1, "%s", strconv.Quote(item))
	}
	if e.features.sql {
		e.line(1, "_ %s", strconv.Quote("github.com/jackc/pgx/v5/stdlib"))
		if e.features.sqlDuration {
			e.line(1, "%s", strconv.Quote("github.com/jackc/pgx/v5/pgtype"))
		}
	}
	e.line(0, `)`)
	e.line(0, "")
	e.emitRuntime()

	for _, file := range e.files {
		for _, decl := range file.Decls {
			switch value := decl.(type) {
			case *ast.Record:
				e.emitRecord(value)
			case *ast.Enum:
				e.emitEnum(value)
			case *ast.Embed:
				if value.Kind == "regex" {
					e.line(0, "var %s = regexp.MustCompile(%s)", safeName(value.Name), strconv.Quote(value.Raw))
				} else {
					contents := value.Raw
					if value.Kind == "sql" && value.SQL != nil {
						contents = value.SQL.Text
					}
					e.line(0, "var %s = %s", safeName(value.Name), strconv.Quote(contents))
				}
				e.line(0, "")
			}
		}
	}
	emittedRows := map[string]bool{}
	for _, file := range e.files {
		for _, decl := range file.Decls {
			embed, ok := decl.(*ast.Embed)
			if !ok || embed.SQL == nil || len(embed.SQL.Columns) == 0 || embed.SQL.RowType.Name == "" || e.records[embed.SQL.RowType.Name] != nil || emittedRows[embed.SQL.RowType.Name] {
				continue
			}
			e.line(0, "type %s struct {", goType(embed.SQL.RowType))
			for _, column := range embed.SQL.Columns {
				e.line(1, "%s %s `json:%s`", exported(column.Name), goType(column.Type), strconv.Quote(column.Name))
			}
			e.line(0, "}")
			e.line(0, "")
			emittedRows[embed.SQL.RowType.Name] = true
		}
	}
	for _, file := range e.files {
		for _, decl := range file.Decls {
			embed, ok := decl.(*ast.Embed)
			if ok && embed.SQL != nil && len(embed.SQL.Columns) > 0 && embed.SQL.RowType.Name != "" {
				e.emitSQLScanner(embed)
			}
		}
	}
	for _, file := range e.files {
		for _, decl := range file.Decls {
			if value, ok := decl.(*ast.Function); ok {
				e.emitFunction(value)
			}
		}
	}
	for _, file := range e.files {
		for _, decl := range file.Decls {
			if value, ok := decl.(*ast.Route); ok {
				e.emitRoute(value)
			}
		}
	}
	e.emitMain()
}

func (e *emitter) emitSQLScanner(embed *ast.Embed) {
	name := "scan" + exported(embed.Name) + "Row"
	typeName := goType(embed.SQL.RowType)
	e.line(0, "func %s(scanner sqlScanner) (%s, error) {", name, typeName)
	e.line(1, "var value %s", typeName)
	destinations := make([]string, 0, len(embed.SQL.Columns))
	var assignments []string
	for index, column := range embed.SQL.Columns {
		if sqlScalarType(column.Type).Name == "duration" {
			temporary := fmt.Sprintf("sqlColumn%d", index+1)
			e.line(1, "var %s sqlDuration", temporary)
			destinations = append(destinations, "&"+temporary)
			field := "value." + exported(column.Name)
			if column.Type.Name == "optional" {
				assignments = append(assignments, fmt.Sprintf("if %s.Valid { item := %s.Duration; %s = &item }", temporary, temporary, field))
			} else {
				assignments = append(assignments, fmt.Sprintf("if !%s.Valid { return value, fmt.Errorf(%s) }; %s = %s.Duration", temporary, strconv.Quote("database returned NULL for "+column.Name), field, temporary))
			}
		} else {
			destinations = append(destinations, "&value."+exported(column.Name))
		}
	}
	e.line(1, "err := scanner.Scan(%s)", strings.Join(destinations, ", "))
	e.line(1, "if err != nil { return value, err }")
	for _, assignment := range assignments {
		e.line(1, "%s", assignment)
	}
	e.line(1, "return value, nil")
	e.line(0, "}")
	e.line(0, "")
}

func (e *emitter) emitRecord(record *ast.Record) {
	e.line(0, "type %s struct {", exported(record.Name))
	for _, field := range record.Fields {
		e.line(1, "%s %s `json:%s`", exported(field.Name), goType(field.Type), strconv.Quote(field.Name))
	}
	e.line(0, "}")
	e.line(0, "")
}

func (e *emitter) emitEnum(enum *ast.Enum) {
	name := exported(enum.Name)
	e.line(0, "type %s string", name)
	e.line(0, "")
	e.line(0, "const (")
	for _, item := range enum.Cases {
		e.line(1, "%s%s %s = %s", name, exported(item.Name), name, strconv.Quote(item.Name))
	}
	e.line(0, ")")
	e.line(0, "")
}

func (e *emitter) emitFunction(fn *ast.Function) {
	params := make([]string, 0, len(fn.Inputs))
	for _, input := range fn.Inputs {
		params = append(params, safeName(input.Name)+" "+goType(input.Type))
	}
	result := ""
	if fn.Output != nil {
		result = " " + goType(*fn.Output)
	}
	e.line(0, "func %s(%s)%s {", safeName(fn.Name), strings.Join(params, ", "), result)
	if statementsUseSQL(fn.Body) {
		e.line(1, "verbaSQLExecutor := sqlExecutor(database)")
		e.line(1, "verbaSQLContext := context.Background()")
	}
	e.emitStatements(fn.Body, 1, fn.Output, false)
	if fn.Output != nil && !blockTerminates(fn.Body) {
		e.line(1, "var zero %s", goType(*fn.Output))
		e.line(1, "return zero")
	}
	e.line(0, "}")
	e.line(0, "")
}

func (e *emitter) emitRoute(route *ast.Route) {
	e.line(0, "func %s(w http.ResponseWriter, r *http.Request) {", safeName(route.Name))
	e.line(1, "request_body, _ := io.ReadAll(r.Body)")
	e.line(1, "_ = request_body")
	e.line(1, "request_headers := r.Header")
	e.line(1, "_ = request_headers")
	e.line(1, "request_context := r.Context()")
	e.line(1, "_ = request_context")
	if statementsUseSQL(route.Body) {
		e.line(1, "verbaSQLExecutor := sqlExecutor(database)")
		e.line(1, "verbaSQLContext := request_context")
	}
	for _, parameter := range routeParameters(route.Path) {
		e.line(1, "%s := r.PathValue(%s)", safeName(parameter), strconv.Quote(parameter))
	}
	e.emitStatements(route.Body, 1, route.Output, true)
	if !blockTerminates(route.Body) {
		e.line(1, "w.WriteHeader(http.StatusNoContent)")
	}
	e.line(0, "}")
	e.line(0, "")
}

func (e *emitter) emitStatements(statements []ast.Stmt, indent int, output *ast.Type, inRoute bool) {
	for _, statement := range statements {
		switch value := statement.(type) {
		case *ast.LetStmt:
			if value.Value.Try && value.Value.Kind == ast.ExprCall {
				tmp := e.nextTemp()
				expr := value.Value
				expr.Try = false
				e.line(indent, "%s := %s", tmp, e.expr(expr))
				e.line(indent, "if %s.Err != nil {", tmp)
				e.emitTryFailure(tmp, indent+1, output, inRoute)
				e.line(indent, "}")
				e.line(indent, "%s := %s.Value", safeName(value.Name), tmp)
				e.line(indent, "_ = %s", safeName(value.Name))
			} else {
				e.line(indent, "%s := %s", safeName(value.Name), e.expr(value.Value))
				e.line(indent, "_ = %s", safeName(value.Name))
			}
		case *ast.SetStmt:
			path := make([]string, len(value.Path))
			for i, part := range value.Path {
				if i == 0 {
					path[i] = safeName(part)
				} else {
					path[i] = exported(part)
				}
			}
			e.line(indent, "%s = %s", strings.Join(path, "."), e.expr(value.Value))
		case *ast.ExprStmt:
			if value.Value.Try && value.Value.Kind == ast.ExprCall {
				tmp := e.nextTemp()
				expr := value.Value
				expr.Try = false
				e.line(indent, "%s := %s", tmp, e.expr(expr))
				e.line(indent, "if %s.Err != nil {", tmp)
				e.emitTryFailure(tmp, indent+1, output, inRoute)
				e.line(indent, "}")
			} else {
				e.line(indent, "%s", e.expr(value.Value))
			}
		case *ast.ReturnStmt:
			if value.Value == nil {
				e.line(indent, "return")
			} else if output != nil && output.Name == "result" && value.Value.Kind == ast.ExprCall && (value.Value.Value == "ok" || value.Value.Value == "error") {
				e.emitResultReturn(*value.Value, *output, indent)
			} else {
				e.line(indent, "return %s", e.expr(*value.Value))
			}
		case *ast.RespondStmt:
			e.emitRespond(value, indent)
		case *ast.IfStmt:
			e.line(indent, "if %s {", e.expr(value.Condition))
			e.emitStatements(value.Then, indent+1, output, inRoute)
			if len(value.Else) > 0 {
				e.line(indent, "} else {")
				e.emitStatements(value.Else, indent+1, output, inRoute)
			}
			e.line(indent, "}")
		case *ast.ForStmt:
			e.line(indent, "for _, %s := range %s {", safeName(value.Name), e.expr(value.Iterable))
			e.emitStatements(value.Body, indent+1, output, inRoute)
			e.line(indent, "}")
		case *ast.WhileStmt:
			e.line(indent, "for %s {", e.expr(value.Condition))
			e.emitStatements(value.Body, indent+1, output, inRoute)
			e.line(indent, "}")
		case *ast.MatchStmt:
			e.line(indent, "switch %s {", e.expr(value.Value))
			for _, matchCase := range value.Cases {
				e.line(indent, "case %s:", e.expr(matchCase.Pattern))
				e.emitStatements(matchCase.Body, indent+1, output, inRoute)
			}
			if len(value.Else) > 0 {
				e.line(indent, "default:")
				e.emitStatements(value.Else, indent+1, output, inRoute)
			}
			e.line(indent, "}")
		case *ast.TransactionStmt:
			txName := e.nextTemp()
			errorName := e.nextTemp()
			previousName := e.nextTemp()
			e.line(indent, "%s, %s := database.BeginTx(verbaSQLContext, nil)", txName, errorName)
			e.line(indent, "if %s != nil {", errorName)
			e.emitDatabaseFailure(errorName, indent+1, output, inRoute)
			e.line(indent, "}")
			e.line(indent, "defer %s.Rollback()", txName)
			e.line(indent, "%s := verbaSQLExecutor", previousName)
			e.line(indent, "verbaSQLExecutor = %s", txName)
			previousTransaction := e.transaction
			e.transaction = txName
			e.emitStatements(value.Body, indent, output, inRoute)
			e.transaction = previousTransaction
			e.line(indent, "%s = %s.Commit()", errorName, txName)
			e.line(indent, "verbaSQLExecutor = %s", previousName)
			e.line(indent, "if %s != nil {", errorName)
			e.emitDatabaseFailure(errorName, indent+1, output, inRoute)
			e.line(indent, "}")
		}
	}
}

func (e *emitter) emitTryFailure(result string, indent int, output *ast.Type, inRoute bool) {
	if e.transaction != "" {
		e.line(indent, "_ = %s.Rollback()", e.transaction)
	}
	if inRoute {
		e.line(indent, `http.Error(w, fmt.Sprint(*%s.Err), http.StatusInternalServerError)`, result)
		e.line(indent, "return")
	} else if output != nil && output.Name == "result" {
		errorName := e.nextTemp()
		e.line(indent, "%s := *%s.Err", errorName, result)
		e.line(indent, "return %s{Err: &%s}", goType(*output), errorName)
	} else {
		e.line(indent, "panic(fmt.Sprint(*%s.Err))", result)
	}
}

func (e *emitter) emitDatabaseFailure(err string, indent int, output *ast.Type, inRoute bool) {
	if inRoute {
		e.line(indent, "http.Error(w, %s.Error(), http.StatusInternalServerError)", err)
		e.line(indent, "return")
	} else if output != nil && output.Name == "result" && len(output.Args) == 2 && output.Args[1].Name == "string" {
		messageName := e.nextTemp()
		e.line(indent, "%s := %s.Error()", messageName, err)
		e.line(indent, "return %s{Err: &%s}", goType(*output), messageName)
	} else if output != nil && output.Name == "result" && len(output.Args) == 2 && e.enumCases["database_failure"] != "" {
		errorName := e.nextTemp()
		e.line(indent, "%s := %s", errorName, e.enumCases["database_failure"])
		e.line(indent, "return %s{Err: &%s}", goType(*output), errorName)
	} else {
		e.line(indent, "panic(%s)", err)
	}
}

func (e *emitter) emitResultReturn(expr ast.Expr, output ast.Type, indent int) {
	typeName := goType(output)
	value := "nil"
	if len(expr.Args) > 0 {
		value = e.expr(expr.Args[0])
	}
	if expr.Value == "ok" {
		e.line(indent, "return %s{Value: %s}", typeName, value)
		return
	}
	tmp := e.nextTemp()
	e.line(indent, "%s := %s", tmp, value)
	e.line(indent, "return %s{Err: &%s}", typeName, tmp)
}

func (e *emitter) emitRespond(stmt *ast.RespondStmt, indent int) {
	switch stmt.Format {
	case "json":
		if stmt.Value != nil {
			dataName := e.nextTemp()
			errorName := e.nextTemp()
			e.line(indent, "%s, %s := json.Marshal(%s)", dataName, errorName, e.expr(*stmt.Value))
			e.line(indent, "if %s != nil {", errorName)
			e.line(indent+1, "http.Error(w, %s.Error(), http.StatusInternalServerError)", errorName)
			e.line(indent+1, "return")
			e.line(indent, "}")
			e.line(indent, `w.Header().Set("Content-Type", "application/json; charset=utf-8")`)
			e.line(indent, "w.WriteHeader(%d)", stmt.Status)
			e.line(indent, "_, _ = w.Write(%s)", dataName)
			e.line(indent, `_, _ = w.Write([]byte("\n"))`)
		} else {
			e.line(indent, `w.Header().Set("Content-Type", "application/json; charset=utf-8")`)
			e.line(indent, "w.WriteHeader(%d)", stmt.Status)
		}
	case "empty":
		e.line(indent, "w.WriteHeader(%d)", stmt.Status)
	default:
		e.line(indent, `w.Header().Set("Content-Type", "text/plain; charset=utf-8")`)
		e.line(indent, "w.WriteHeader(%d)", stmt.Status)
		if stmt.Value != nil {
			e.line(indent, "_, _ = io.WriteString(w, %s)", e.expr(*stmt.Value))
		}
	}
	e.line(indent, "return")
}

func (e *emitter) expr(expr ast.Expr) string {
	switch expr.Kind {
	case ast.ExprText:
		return strconv.Quote(expr.Value)
	case ast.ExprAtom:
		if enum, exists := e.enumCases[expr.Value]; exists {
			return enum
		}
		if numeric.Classify(expr.Value) != numeric.Invalid {
			if expr.ResolvedType.Name == "decimal" {
				return "newDecimal(" + strconv.Quote(expr.Value) + ")"
			}
			if numeric.IsType(expr.ResolvedType.Name) {
				return goType(expr.ResolvedType) + "(" + expr.Value + ")"
			}
		}
		return safeName(expr.Value)
	case ast.ExprGet:
		parts := make([]string, 0, len(expr.Args))
		for i, arg := range expr.Args {
			if i == 0 {
				parts = append(parts, safeName(arg.Value))
			} else {
				parts = append(parts, exported(arg.Value))
			}
		}
		return strings.Join(parts, ".")
	case ast.ExprRelation:
		if len(expr.Args) == 2 && expr.Args[0].ResolvedType.Name == "decimal" {
			comparison := "== 0"
			if expr.Not {
				comparison = "!= 0"
			}
			return fmt.Sprintf("(%s.Cmp(%s) %s)", e.expr(expr.Args[0]), e.expr(expr.Args[1]), comparison)
		}
		op := "=="
		if expr.Not {
			op = "!="
		}
		return fmt.Sprintf("(%s %s %s)", e.expr(expr.Args[0]), op, e.expr(expr.Args[1]))
	case ast.ExprCall:
		return e.call(expr)
	default:
		return "nil"
	}
}

func (e *emitter) call(expr ast.Expr) string {
	args := make([]string, 0, len(expr.Args))
	for _, arg := range expr.Args {
		args = append(args, e.expr(arg))
	}
	if fn := e.functions[expr.Value]; fn != nil && len(expr.NamedArgs) > 0 {
		named := map[string]ast.Expr{}
		for _, arg := range expr.NamedArgs {
			named[arg.Name] = arg.Value
		}
		args = args[:0]
		for _, input := range fn.Inputs {
			args = append(args, e.expr(named[input.Name]))
		}
	}
	switch expr.Value {
	case "sql_exec", "sql_one", "sql_optional", "sql_many":
		return e.sqlCall(expr)
	case "add":
		if expr.ResolvedType.Name == "decimal" {
			return args[0] + ".Add(" + args[1] + ")"
		}
		return "(" + args[0] + " + " + args[1] + ")"
	case "subtract":
		if expr.ResolvedType.Name == "decimal" {
			return args[0] + ".Sub(" + args[1] + ")"
		}
		return "(" + args[0] + " - " + args[1] + ")"
	case "multiply":
		if expr.ResolvedType.Name == "decimal" {
			return args[0] + ".Mul(" + args[1] + ")"
		}
		return "(" + args[0] + " * " + args[1] + ")"
	case "divide":
		if expr.ResolvedType.Name == "decimal" {
			return args[0] + ".Quo(" + args[1] + ")"
		}
		return "(" + args[0] + " / " + args[1] + ")"
	case "remainder":
		if expr.ResolvedType.Name == "decimal" {
			return args[0] + ".Rem(" + args[1] + ")"
		}
		if expr.ResolvedType.Name == "float64" {
			return "math.Mod(" + args[0] + ", " + args[1] + ")"
		}
		if expr.ResolvedType.Name == "float32" {
			return "float32(math.Mod(float64(" + args[0] + "), float64(" + args[1] + ")))"
		}
		return "(" + args[0] + " % " + args[1] + ")"
	case "negate":
		if expr.ResolvedType.Name == "decimal" {
			return args[0] + ".Neg()"
		}
		return "(-" + args[0] + ")"
	case "greater_than":
		if len(expr.Args) > 0 && expr.Args[0].ResolvedType.Name == "decimal" {
			return "(" + args[0] + ".Cmp(" + args[1] + ") > 0)"
		}
		return "(" + args[0] + " > " + args[1] + ")"
	case "less_than":
		if len(expr.Args) > 0 && expr.Args[0].ResolvedType.Name == "decimal" {
			return "(" + args[0] + ".Cmp(" + args[1] + ") < 0)"
		}
		return "(" + args[0] + " < " + args[1] + ")"
	case "greater_equal":
		if len(expr.Args) > 0 && expr.Args[0].ResolvedType.Name == "decimal" {
			return "(" + args[0] + ".Cmp(" + args[1] + ") >= 0)"
		}
		return "(" + args[0] + " >= " + args[1] + ")"
	case "less_equal":
		if len(expr.Args) > 0 && expr.Args[0].ResolvedType.Name == "decimal" {
			return "(" + args[0] + ".Cmp(" + args[1] + ") <= 0)"
		}
		return "(" + args[0] + " <= " + args[1] + ")"
	case "and":
		return "(" + args[0] + " && " + args[1] + ")"
	case "or":
		return "(" + args[0] + " || " + args[1] + ")"
	case "not":
		return "(!" + args[0] + ")"
	case "trim":
		return "strings.TrimSpace(" + args[0] + ")"
	case "lowercase":
		return "strings.ToLower(" + args[0] + ")"
	case "uppercase":
		return "strings.ToUpper(" + args[0] + ")"
	case "contains":
		return "strings.Contains(" + args[0] + ", " + args[1] + ")"
	case "starts_with":
		return "strings.HasPrefix(" + args[0] + ", " + args[1] + ")"
	case "concat":
		return "strings.Join([]string{" + strings.Join(args, ", ") + "}, \"\")"
	case "new_uuid":
		return "newUUID()"
	case "parse_uuid":
		return "parseUUID(" + args[0] + ")"
	case "regex_match":
		if len(expr.Args) == 2 {
			if embed := e.embeds[expr.Args[0].Value]; embed != nil && embed.Kind == "regex" {
				return args[0] + ".MatchString(" + args[1] + ")"
			}
		}
		return "func() bool { ok, _ := regexp.MatchString(" + args[0] + ", " + args[1] + "); return ok }()"
	case "json_decode":
		if len(expr.Args) == 2 {
			return "decodeJSON[" + exported(expr.Args[0].Value) + "](" + args[1] + ")"
		}
	case "json_encode":
		return "func() []byte { data, _ := json.Marshal(" + args[0] + "); return data }()"
	case "is_some":
		return "(" + args[0] + " != nil)"
	case "is_none":
		return "(" + args[0] + " == nil)"
	case "unwrap":
		return "(*" + args[0] + ")"
	case "render":
		values := make([]string, 0, len(expr.NamedArgs))
		for _, arg := range expr.NamedArgs {
			values = append(values, strconv.Quote(arg.Name)+": "+e.expr(arg.Value))
		}
		escapeHTML := "false"
		if len(expr.Args) > 0 {
			if embed := e.embeds[expr.Args[0].Value]; embed != nil && embed.Kind == "html" {
				escapeHTML = "true"
			}
		}
		return "renderTemplate(" + args[0] + ", map[string]string{" + strings.Join(values, ", ") + "}, " + escapeHTML + ")"
	}
	return safeName(expr.Value) + "(" + strings.Join(args, ", ") + ")"
}

func (e *emitter) sqlCall(expr ast.Expr) string {
	if len(expr.Args) == 0 {
		return "nil"
	}
	embed := e.embeds[expr.Args[0].Value]
	if embed == nil || embed.SQL == nil {
		return "nil"
	}
	bindings := map[string]ast.Expr{}
	for _, argument := range expr.NamedArgs {
		bindings[argument.Name] = argument.Value
	}
	arguments := []string{"verbaSQLExecutor", "verbaSQLContext", safeName(embed.Name)}
	if expr.Value != "sql_exec" {
		arguments = append(arguments, "scan"+exported(embed.Name)+"Row")
	}
	for _, parameter := range embed.SQL.Parameters {
		binding := bindings[parameter.Name]
		value := e.expr(binding)
		if sqlScalarType(parameter.Type).Name == "duration" {
			if binding.ResolvedType.Name == "optional" {
				value = "sqlDurationArgument(" + value + ")"
			} else {
				value = "sqlDuration{Duration: " + value + ", Valid: true}"
			}
		}
		arguments = append(arguments, value)
	}
	result := "nil"
	switch expr.Value {
	case "sql_exec":
		result = "sqlExec(" + strings.Join(arguments, ", ") + ")"
	case "sql_one":
		result = "sqlOne[" + goType(embed.SQL.RowType) + "](" + strings.Join(arguments, ", ") + ")"
	case "sql_optional":
		result = "sqlOptional[" + goType(embed.SQL.RowType) + "](" + strings.Join(arguments, ", ") + ")"
	case "sql_many":
		result = "sqlMany[" + goType(embed.SQL.RowType) + "](" + strings.Join(arguments, ", ") + ")"
	}
	if expr.CallResultType.Name == "result" && len(expr.CallResultType.Args) == 2 && expr.CallResultType.Args[1].Name != "string" {
		if mapped := e.enumCases["database_failure"]; mapped != "" {
			result = "mapSQLError(" + result + ", " + mapped + ")"
		}
	}
	return result
}

func (e *emitter) emitMain() {
	e.line(0, "func main() {")
	if e.features.sql {
		e.line(1, `databaseURL := os.Getenv("VERBA_DATABASE_URL")`)
		e.line(1, `if databaseURL == "" { panic("VERBA_DATABASE_URL is required") }`)
		e.line(1, "var err error")
		e.line(1, `database, err = sql.Open("pgx", databaseURL)`)
		e.line(1, "if err != nil { panic(err) }")
		e.line(1, "defer database.Close()")
		e.line(1, "if err := database.Ping(); err != nil { panic(err) }")
	}
	e.line(1, "mux := http.NewServeMux()")
	count := 0
	for _, file := range e.files {
		for _, decl := range file.Decls {
			if route, ok := decl.(*ast.Route); ok {
				pattern := strings.ToUpper(route.Method) + " " + route.Path
				e.line(1, "mux.HandleFunc(%s, %s)", strconv.Quote(pattern), safeName(route.Name))
				count++
			}
		}
	}
	if count == 0 {
		e.line(1, `mux.HandleFunc("GET /", func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "Verba program is running") })`)
	}
	e.line(1, `address := os.Getenv("VERBA_ADDRESS")`)
	e.line(1, `if address == "" { address = ":8080" }`)
	e.line(1, `fmt.Printf("Verba server listening on %%s\n", address)`)
	e.line(1, "if err := http.ListenAndServe(address, mux); err != nil { panic(err) }")
	e.line(0, "}")
}

func goType(t ast.Type) string {
	switch t.Name {
	case "bool", "string", "int8", "int16", "int32", "int64", "uint8", "uint16", "uint32", "uint64", "float32", "float64":
		return t.Name
	case "int":
		return "int64"
	case "uint":
		return "uint64"
	case "bytes":
		return "[]byte"
	case "decimal":
		return "Decimal"
	case "uuid", "url", "path":
		return "string"
	case "time":
		return "time.Time"
	case "duration":
		return "time.Duration"
	case "optional":
		return "*" + goType(t.Args[0])
	case "list":
		return "[]" + goType(t.Args[0])
	case "map":
		return "map[" + goType(t.Args[0]) + "]" + goType(t.Args[1])
	case "result":
		return "Result[" + goType(t.Args[0]) + ", " + goType(t.Args[1]) + "]"
	default:
		return exported(t.Name)
	}
}

func statementsUseSQL(statements []ast.Stmt) bool {
	for _, statement := range statements {
		switch value := statement.(type) {
		case *ast.LetStmt:
			if exprUsesSQL(value.Value) {
				return true
			}
		case *ast.SetStmt:
			if exprUsesSQL(value.Value) {
				return true
			}
		case *ast.ExprStmt:
			if exprUsesSQL(value.Value) {
				return true
			}
		case *ast.ReturnStmt:
			if value.Value != nil && exprUsesSQL(*value.Value) {
				return true
			}
		case *ast.RespondStmt:
			if value.Value != nil && exprUsesSQL(*value.Value) {
				return true
			}
		case *ast.IfStmt:
			if exprUsesSQL(value.Condition) || statementsUseSQL(value.Then) || statementsUseSQL(value.Else) {
				return true
			}
		case *ast.ForStmt:
			if exprUsesSQL(value.Iterable) || statementsUseSQL(value.Body) {
				return true
			}
		case *ast.WhileStmt:
			if exprUsesSQL(value.Condition) || statementsUseSQL(value.Body) {
				return true
			}
		case *ast.MatchStmt:
			if exprUsesSQL(value.Value) || statementsUseSQL(value.Else) {
				return true
			}
			for _, matchCase := range value.Cases {
				if exprUsesSQL(matchCase.Pattern) || statementsUseSQL(matchCase.Body) {
					return true
				}
			}
		case *ast.TransactionStmt:
			return true
		}
	}
	return false
}

func exprUsesSQL(expr ast.Expr) bool {
	if expr.Kind == ast.ExprCall && strings.HasPrefix(expr.Value, "sql_") {
		return true
	}
	for _, argument := range expr.Args {
		if exprUsesSQL(argument) {
			return true
		}
	}
	for _, argument := range expr.NamedArgs {
		if exprUsesSQL(argument.Value) {
			return true
		}
	}
	return false
}

func sqlScalarType(value ast.Type) ast.Type {
	if value.Name == "optional" && len(value.Args) == 1 {
		return value.Args[0]
	}
	return value
}

func exported(value string) string {
	parts := strings.Split(value, "_")
	for i, part := range parts {
		if part == "" {
			continue
		}
		runes := []rune(part)
		runes[0] = unicode.ToUpper(runes[0])
		parts[i] = string(runes)
	}
	return strings.Join(parts, "")
}

func safeName(value string) string {
	if value == "true" || value == "false" || isNumber(value) {
		return value
	}
	reserved := map[string]bool{"break": true, "default": true, "func": true, "interface": true, "select": true, "case": true, "defer": true, "go": true, "map": true, "struct": true, "chan": true, "else": true, "goto": true, "package": true, "switch": true, "const": true, "fallthrough": true, "if": true, "range": true, "type": true, "continue": true, "for": true, "import": true, "return": true, "var": true}
	if reserved[value] {
		return value + "Value"
	}
	return value
}

func isNumber(value string) bool {
	return numeric.Classify(value) != numeric.Invalid
}

func routeParameters(path string) []string {
	var result []string
	for {
		start := strings.IndexByte(path, '{')
		if start < 0 {
			return result
		}
		end := strings.IndexByte(path[start+1:], '}')
		if end < 0 {
			return result
		}
		result = append(result, path[start+1:start+1+end])
		path = path[start+1+end+1:]
	}
}

func blockTerminates(statements []ast.Stmt) bool {
	if len(statements) == 0 {
		return false
	}
	switch value := statements[len(statements)-1].(type) {
	case *ast.ReturnStmt, *ast.RespondStmt:
		return true
	case *ast.IfStmt:
		return len(value.Else) > 0 && blockTerminates(value.Then) && blockTerminates(value.Else)
	case *ast.MatchStmt:
		if len(value.Else) == 0 || !blockTerminates(value.Else) {
			return false
		}
		for _, matchCase := range value.Cases {
			if !blockTerminates(matchCase.Body) {
				return false
			}
		}
		return len(value.Cases) > 0
	default:
		return false
	}
}

func (e *emitter) nextTemp() string { e.temp++; return fmt.Sprintf("verbaTemp%d", e.temp) }
func (e *emitter) line(indent int, format string, args ...any) {
	e.buffer.WriteString(strings.Repeat("\t", indent))
	fmt.Fprintf(&e.buffer, format, args...)
	e.buffer.WriteByte('\n')
}
func (e *emitter) error(pos ast.Position, code, message, hint string) {
	e.diagnostics = append(e.diagnostics, diagnostic.Diagnostic{Severity: diagnostic.Error, Code: code, File: pos.File, Line: pos.Line, Column: pos.Column, Message: message, Hint: hint})
}
