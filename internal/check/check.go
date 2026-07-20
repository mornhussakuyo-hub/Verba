package check

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/verba-lang/verba/internal/ast"
	"github.com/verba-lang/verba/internal/diagnostic"
)

var sqlParameter = regexp.MustCompile(`:[A-Za-z_][A-Za-z0-9_]*`)
var htmlSlot = regexp.MustCompile(`\{\{\s*([A-Za-z_][A-Za-z0-9_]*)\s*\}\}`)

var builtinTypes = map[string]bool{
	"bool": true, "int": true, "uint": true,
	"int8": true, "int16": true, "int32": true, "int64": true,
	"uint8": true, "uint16": true, "uint32": true, "uint64": true,
	"float32": true, "float64": true, "decimal": true,
	"string": true, "bytes": true, "uuid": true, "time": true,
	"duration": true, "url": true, "path": true,
}

type builtin struct {
	min int
	max int
}

type binding struct {
	typeValue ast.Type
	mutable   bool
}

type scope map[string]binding

var (
	unknownType = ast.Type{Name: "<unknown>"}
	unitType    = ast.Type{Name: "<unit>"}
	boolType    = ast.Type{Name: "bool"}
	intType     = ast.Type{Name: "int"}
	floatType   = ast.Type{Name: "float64"}
	stringType  = ast.Type{Name: "string"}
	bytesType   = ast.Type{Name: "bytes"}
	uuidType    = ast.Type{Name: "uuid"}
)

var builtins = map[string]builtin{
	"add": {2, 2}, "subtract": {2, 2}, "multiply": {2, 2}, "divide": {2, 2}, "remainder": {2, 2},
	"negate": {1, 1}, "greater_than": {2, 2}, "less_than": {2, 2}, "greater_equal": {2, 2}, "less_equal": {2, 2},
	"and": {2, 2}, "or": {2, 2}, "not": {1, 1},
	"concat": {2, -1}, "trim": {1, 1}, "lowercase": {1, 1}, "uppercase": {1, 1}, "contains": {2, 2}, "starts_with": {2, 2},
	"is_some": {1, 1}, "is_none": {1, 1}, "unwrap": {1, 1}, "ok": {1, 1}, "error": {1, 1},
	"new_uuid": {0, 0}, "parse_uuid": {1, 1}, "regex_match": {2, 2},
	"json_decode": {2, 2}, "json_encode": {1, 1},
	"render":   {1, 1},
	"sql_exec": {1, 1}, "sql_one": {1, 1}, "sql_optional": {1, 1}, "sql_many": {1, 1},
}

type Checker struct {
	files       []*ast.File
	decls       map[string]ast.Decl
	functions   map[string]*ast.Function
	records     map[string]*ast.Record
	embeds      map[string]*ast.Embed
	enumCases   map[string]ast.Type
	diagnostics []diagnostic.Diagnostic
}

func Files(files []*ast.File) []diagnostic.Diagnostic {
	c := &Checker{
		files: files, decls: map[string]ast.Decl{}, functions: map[string]*ast.Function{},
		records: map[string]*ast.Record{}, embeds: map[string]*ast.Embed{}, enumCases: map[string]ast.Type{},
	}
	c.collect()
	c.validate()
	return c.diagnostics
}

func (c *Checker) collect() {
	module := ""
	for _, file := range c.files {
		if module == "" {
			module = file.Module
		} else if file.Module != "" && file.Module != module {
			c.error(ast.Position{File: file.Path, Line: 1, Column: 1}, "VRB1001", fmt.Sprintf("module %s does not match module %s", file.Module, module), "all source files in one build must use the same module")
		}
		for _, decl := range file.Decls {
			if previous, exists := c.decls[decl.DeclName()]; exists {
				c.error(decl.DeclPos(), "VRB1002", fmt.Sprintf("duplicate declaration %s", decl.DeclName()), fmt.Sprintf("first declared at %s:%d", previous.DeclPos().File, previous.DeclPos().Line))
				continue
			}
			c.decls[decl.DeclName()] = decl
			switch value := decl.(type) {
			case *ast.Function:
				c.functions[value.Name] = value
			case *ast.Record:
				c.records[value.Name] = value
			case *ast.Embed:
				c.embeds[value.Name] = value
			case *ast.Enum:
				for _, item := range value.Cases {
					c.enumCases[item.Name] = ast.Type{Name: value.Name}
				}
			}
		}
	}
}

func (c *Checker) validate() {
	for _, file := range c.files {
		for _, decl := range file.Decls {
			switch value := decl.(type) {
			case *ast.Record:
				c.validateRecord(value)
			case *ast.Enum:
				c.validateEnum(value)
			case *ast.Function:
				c.validateFunction(value)
			case *ast.Route:
				c.validateRoute(value)
			case *ast.Embed:
				c.validateEmbed(value)
			}
		}
	}
}

func (c *Checker) validateRecord(record *ast.Record) {
	seen := map[string]bool{}
	for _, field := range record.Fields {
		if seen[field.Name] {
			c.error(field.Pos, "VRB1101", fmt.Sprintf("duplicate field %s in record %s", field.Name, record.Name), "field names must be unique")
		}
		seen[field.Name] = true
		c.validateType(field.Type, field.Pos)
	}
}

func (c *Checker) validateEnum(enum *ast.Enum) {
	seen := map[string]bool{}
	for _, item := range enum.Cases {
		if seen[item.Name] {
			c.error(item.Pos, "VRB1111", fmt.Sprintf("duplicate case %s in enum %s", item.Name, enum.Name), "enum case names must be unique")
		}
		seen[item.Name] = true
	}
}

func (c *Checker) validateFunction(fn *ast.Function) {
	local := scope{}
	for _, input := range fn.Inputs {
		if _, exists := local[input.Name]; exists {
			c.error(input.Pos, "VRB1121", fmt.Sprintf("duplicate input %s", input.Name), "function inputs must have unique names")
		}
		local[input.Name] = binding{typeValue: input.Type}
		c.validateType(input.Type, input.Pos)
	}
	if fn.Output != nil {
		c.validateType(*fn.Output, fn.Pos)
	}
	c.validateStatements(fn.Body, local, false, fn.Output)
	if fn.Output != nil && !blockTerminates(fn.Body) {
		c.error(fn.Pos, "VRB1501", fmt.Sprintf("function %s does not return on every path", fn.Name), "add a return statement or make every if branch return")
	}
}

func (c *Checker) validateRoute(route *ast.Route) {
	if route.Method == "" {
		c.error(route.Pos, "VRB1131", fmt.Sprintf("route %s has no method", route.Name), "add a method declaration before begin")
	} else {
		valid := map[string]bool{"get": true, "post": true, "put": true, "patch": true, "delete": true, "head": true, "options": true}
		if !valid[route.Method] {
			c.error(route.Pos, "VRB1132", fmt.Sprintf("unsupported HTTP method %s", route.Method), "use get, post, put, patch, delete, head, or options")
		}
	}
	if route.Path == "" || !strings.HasPrefix(route.Path, "/") {
		c.error(route.Pos, "VRB1133", fmt.Sprintf("route %s requires an absolute path", route.Name), "add a path beginning with /")
	}
	if route.Output != nil {
		c.validateType(*route.Output, route.Pos)
	}
	local := scope{
		"request_body":    {typeValue: bytesType},
		"request_headers": {typeValue: ast.Type{Name: "map", Args: []ast.Type{stringType, {Name: "list", Args: []ast.Type{stringType}}}}},
		"request_context": {typeValue: unknownType},
	}
	for _, parameter := range routeParameters(route.Path) {
		local[parameter] = binding{typeValue: stringType}
	}
	c.validateStatements(route.Body, local, true, route.Output)
}

func (c *Checker) validateEmbed(embed *ast.Embed) {
	switch embed.Kind {
	case "json":
		var value any
		if err := json.Unmarshal([]byte(embed.Raw), &value); err != nil {
			line := embed.Pos.Line + 1
			if syntax, ok := err.(*json.SyntaxError); ok {
				line += strings.Count(embed.Raw[:min(int(syntax.Offset), len(embed.Raw))], "\n")
			}
			c.error(ast.Position{File: embed.Pos.File, Line: line, Column: 1}, "JSON2001", fmt.Sprintf("invalid JSON in island %s: %v", embed.Name, err), "fix the JSON syntax inside the island")
		}
	case "regex":
		if _, err := regexp.Compile(embed.Raw); err != nil {
			c.error(embed.Pos, "REGEX2201", fmt.Sprintf("invalid regular expression in island %s: %v", embed.Name, err), "fix the regular expression syntax inside the island")
		}
	case "html":
		if strings.Count(embed.Raw, "{{") != len(htmlSlot.FindAllStringSubmatch(embed.Raw, -1)) || strings.Count(embed.Raw, "}}") != len(htmlSlot.FindAllStringSubmatch(embed.Raw, -1)) {
			c.error(embed.Pos, "HTML2301", fmt.Sprintf("HTML island %s contains an invalid template slot", embed.Name), "use slots in the form {{ slot_name }}")
		}
	case "sql", "text", "comment":
		// These adapters preserve raw content; bindings are checked at call sites.
	default:
		c.error(embed.Pos, "VRB1141", fmt.Sprintf("unknown island adapter %s", embed.Kind), "supported adapters are json, sql, html, regex, text, and comment")
	}
}

func (c *Checker) validateType(t ast.Type, pos ast.Position) {
	if t.Name == "optional" || t.Name == "list" {
		if len(t.Args) != 1 {
			c.error(pos, "VRB1201", fmt.Sprintf("type %s requires one type argument", t.Name), "provide the element type")
			return
		}
	} else if t.Name == "map" || t.Name == "result" {
		if len(t.Args) != 2 {
			c.error(pos, "VRB1202", fmt.Sprintf("type %s requires two type arguments", t.Name), "provide both type arguments")
			return
		}
	} else if !builtinTypes[t.Name] {
		decl, exists := c.decls[t.Name]
		if !exists {
			c.error(pos, "VRB1203", fmt.Sprintf("unknown type %s", t.Name), "declare the record or enum before using it")
		} else {
			switch decl.(type) {
			case *ast.Record, *ast.Enum:
			default:
				c.error(pos, "VRB1204", fmt.Sprintf("%s is not a type", t.Name), "only records and enums can be used as named types")
			}
		}
	}
	for _, arg := range t.Args {
		c.validateType(arg, pos)
	}
}

func (c *Checker) validateStatements(statements []ast.Stmt, local scope, inRoute bool, expectedReturn *ast.Type) {
	for _, statement := range statements {
		switch value := statement.(type) {
		case *ast.LetStmt:
			inferred := c.validateExpr(value.Value, local, expectedReturn)
			if _, exists := local[value.Name]; exists {
				c.error(value.Pos, "VRB1301", fmt.Sprintf("name %s is already bound in this scope", value.Name), "choose a new local name")
			}
			local[value.Name] = binding{typeValue: inferred, mutable: value.Mutable}
		case *ast.SetStmt:
			target := c.validateSetTarget(value, local)
			actual := c.validateExpr(value.Value, local, expectedReturn)
			c.requireAssignable(value.Pos, target, actual, "assigned value")
		case *ast.ExprStmt:
			c.validateExpr(value.Value, local, expectedReturn)
		case *ast.ReturnStmt:
			if expectedReturn == nil {
				if value.Value != nil {
					c.validateExpr(*value.Value, local, nil)
					c.error(value.Pos, "VRB1502", "return provides a value in a function without output", "remove the value or declare an output type")
				}
				continue
			}
			if value.Value == nil {
				c.error(value.Pos, "VRB1503", fmt.Sprintf("return requires a value of type %s", expectedReturn.String()), "return a value matching the function output type")
				continue
			}
			actual := c.validateExpr(*value.Value, local, expectedReturn)
			c.requireAssignable(value.Pos, *expectedReturn, actual, "returned value")
		case *ast.RespondStmt:
			if !inRoute {
				c.error(value.Pos, "VRB1304", "respond can only be used inside a route", "use return inside a function")
			}
			if value.Status < 100 || value.Status > 599 {
				c.error(value.Pos, "VRB1305", fmt.Sprintf("invalid HTTP status %d", value.Status), "status codes must be between 100 and 599")
			}
			if value.Format != "text" && value.Format != "json" && value.Format != "empty" {
				c.error(value.Pos, "VRB1306", fmt.Sprintf("unknown response format %s", value.Format), "use text, json, or empty")
			}
			if value.Format == "empty" && value.Value != nil {
				c.error(value.Pos, "VRB1307", "empty response cannot include a body", "remove the response value")
			}
			if value.Value != nil {
				actual := c.validateExpr(*value.Value, local, expectedReturn)
				if value.Format == "text" {
					c.requireAssignable(value.Pos, stringType, actual, "text response")
				}
			}
		case *ast.IfStmt:
			condition := c.validateExpr(value.Condition, local, expectedReturn)
			c.requireAssignable(value.Pos, boolType, condition, "if condition")
			c.validateStatements(value.Then, cloneScope(local), inRoute, expectedReturn)
			c.validateStatements(value.Else, cloneScope(local), inRoute, expectedReturn)
		case *ast.ForStmt:
			iterable := c.validateExpr(value.Iterable, local, expectedReturn)
			child := cloneScope(local)
			if iterable.Name == "list" && len(iterable.Args) == 1 {
				child[value.Name] = binding{typeValue: iterable.Args[0]}
			} else if !isUnknown(iterable) {
				c.error(value.Pos, "VRB1504", fmt.Sprintf("for requires a list but received %s", iterable.String()), "iterate over a value with type list T")
				child[value.Name] = binding{typeValue: unknownType}
			}
			c.validateStatements(value.Body, child, inRoute, expectedReturn)
		case *ast.WhileStmt:
			condition := c.validateExpr(value.Condition, local, expectedReturn)
			c.requireAssignable(value.Pos, boolType, condition, "while condition")
			c.validateStatements(value.Body, cloneScope(local), inRoute, expectedReturn)
		case *ast.MatchStmt:
			matchedType := c.validateExpr(value.Value, local, nil)
			if !c.isMatchable(matchedType) {
				c.error(value.Pos, "VRB1450", fmt.Sprintf("match value type %s is not comparable", matchedType.String()), "match a scalar or enum value")
			}
			seen := map[string]bool{}
			for _, matchCase := range value.Cases {
				patternType := c.validateExpr(matchCase.Pattern, local, &matchedType)
				c.requireAssignable(matchCase.Pos, matchedType, patternType, "match case")
				if matchCase.Pattern.Kind != ast.ExprAtom {
					c.error(matchCase.Pos, "VRB1451", "match case must be a literal or enum case", "bind computed values before match and use constant case patterns")
				}
				key := matchCase.Pattern.Value
				if seen[key] {
					c.error(matchCase.Pos, "VRB1452", fmt.Sprintf("duplicate match case %s", key), "remove the duplicate case")
				}
				seen[key] = true
				c.validateStatements(matchCase.Body, cloneScope(local), inRoute, expectedReturn)
			}
			c.validateStatements(value.Else, cloneScope(local), inRoute, expectedReturn)
		case *ast.TransactionStmt:
			c.validateStatements(value.Body, cloneScope(local), inRoute, expectedReturn)
		}
	}
}

func (c *Checker) validateSetTarget(statement *ast.SetStmt, local scope) ast.Type {
	if len(statement.Path) == 0 {
		c.error(statement.Pos, "VRB1302", "set targets an unknown name", "declare it with var before using set")
		return unknownType
	}
	root, exists := local[statement.Path[0]]
	if !exists {
		c.error(statement.Pos, "VRB1302", "set targets an unknown name", "declare it with var before using set")
		return unknownType
	}
	if !root.mutable {
		c.error(statement.Pos, "VRB1303", fmt.Sprintf("cannot set immutable name %s", statement.Path[0]), "declare it with var if mutation is required")
	}
	return c.resolveFieldPath(statement.Pos, root.typeValue, statement.Path[1:])
}

func (c *Checker) validateExpr(expr ast.Expr, local scope, expected *ast.Type) ast.Type {
	switch expr.Kind {
	case ast.ExprInvalid:
		return unknownType
	case ast.ExprAtom:
		if literal, ok := literalType(expr.Value); ok {
			return literal
		}
		if value, exists := local[expr.Value]; exists {
			return value.typeValue
		}
		if enumType, exists := c.enumCases[expr.Value]; exists {
			return enumType
		}
		if embed := c.embeds[expr.Value]; embed != nil {
			return ast.Type{Name: "resource_" + embed.Kind}
		}
		c.error(expr.Pos, "VRB1401", fmt.Sprintf("unknown name %s", expr.Value), "declare the name before using it")
		return unknownType
	case ast.ExprText:
		if expr.LiteralType == "" {
			return stringType
		}
		return ast.Type{Name: expr.LiteralType}
	case ast.ExprGet:
		if len(expr.Args) < 2 {
			return unknownType
		}
		root := c.validateExpr(expr.Args[0], local, nil)
		path := make([]string, 0, len(expr.Args)-1)
		for _, part := range expr.Args[1:] {
			path = append(path, part.Value)
		}
		return c.resolveFieldPath(expr.Pos, root, path)
	case ast.ExprRelation:
		if len(expr.Args) != 2 {
			return unknownType
		}
		left := c.validateExpr(expr.Args[0], local, nil)
		right := c.validateExpr(expr.Args[1], local, &left)
		if !sameType(left, right) && !isUnknown(left) && !isUnknown(right) {
			c.error(expr.Pos, "VRB1423", fmt.Sprintf("cannot compare %s with %s", left.String(), right.String()), "compare values with the same type")
		}
		if !c.isComparable(left) {
			c.error(expr.Pos, "VRB1424", fmt.Sprintf("type %s is not comparable", left.String()), "compare a scalar, enum, or comparable optional value")
		}
		return boolType
	case ast.ExprCall:
		return c.validateCall(expr, local, expected)
	default:
		return unknownType
	}
}

func (c *Checker) validateCall(expr ast.Expr, local scope, expected *ast.Type) ast.Type {
	if fn := c.functions[expr.Value]; fn != nil {
		result := unitType
		if fn.Output != nil {
			result = *fn.Output
		}
		if len(expr.NamedArgs) > 0 {
			if len(expr.Args) > 0 {
				c.error(expr.Pos, "VRB1413", fmt.Sprintf("call %s mixes positional and named arguments", expr.Value), "use either positional arguments or a with argument block")
			}
			inputs := map[string]ast.Field{}
			for _, input := range fn.Inputs {
				inputs[input.Name] = input
			}
			actual := map[string]bool{}
			for _, arg := range expr.NamedArgs {
				input, exists := inputs[arg.Name]
				if actual[arg.Name] {
					c.error(arg.Pos, "VRB1414", fmt.Sprintf("duplicate argument %s in call %s", arg.Name, expr.Value), "provide each named argument once")
				} else if !exists {
					c.error(arg.Pos, "VRB1415", fmt.Sprintf("call %s has no input named %s", expr.Value, arg.Name), "use a name from the function input declarations")
				} else {
					argType := c.validateExpr(arg.Value, local, &input.Type)
					c.requireAssignable(arg.Pos, input.Type, argType, fmt.Sprintf("argument %s", arg.Name))
				}
				actual[arg.Name] = true
			}
			for _, input := range fn.Inputs {
				if !actual[input.Name] {
					c.error(expr.Pos, "VRB1416", fmt.Sprintf("call %s is missing named argument %s", expr.Value, input.Name), "add the missing with entry")
				}
			}
		} else {
			if len(expr.Args) != len(fn.Inputs) {
				c.error(expr.Pos, "VRB1410", fmt.Sprintf("call %s expects %d arguments but received %d", expr.Value, len(fn.Inputs), len(expr.Args)), "pass one argument for each input declaration")
			}
			for index, arg := range expr.Args {
				if index >= len(fn.Inputs) {
					c.validateExpr(arg, local, nil)
					continue
				}
				input := fn.Inputs[index]
				argType := c.validateExpr(arg, local, &input.Type)
				c.requireAssignable(arg.Pos, input.Type, argType, fmt.Sprintf("argument %s", input.Name))
			}
		}
		return c.applyTry(expr, result, expected)
	}
	b, exists := builtins[expr.Value]
	if !exists {
		for _, arg := range expr.Args {
			c.validateExpr(arg, local, nil)
		}
		c.error(expr.Pos, "VRB1411", fmt.Sprintf("unknown function %s", expr.Value), "declare the function or use a supported standard function")
		return unknownType
	}
	count := len(expr.Args)
	if count < b.min || (b.max >= 0 && count > b.max) {
		expectedCount := strconv.Itoa(b.min)
		if b.max < 0 {
			expectedCount = fmt.Sprintf("at least %d", b.min)
		}
		c.error(expr.Pos, "VRB1412", fmt.Sprintf("call %s expects %s positional arguments but received %d", expr.Value, expectedCount, count), "check the function call arguments")
	}
	if strings.HasPrefix(expr.Value, "sql_") {
		return c.validateSQLBuiltin(expr, local, expected)
	}
	if expr.Value == "render" {
		return c.validateRenderBuiltin(expr, local)
	}
	if len(expr.NamedArgs) > 0 {
		for _, arg := range expr.NamedArgs {
			c.validateExpr(arg.Value, local, nil)
		}
		c.error(expr.Pos, "VRB1417", fmt.Sprintf("standard function %s does not accept a named argument block", expr.Value), "use positional arguments for this function")
	}
	result := c.validateBuiltin(expr, local, expected)
	return c.applyTry(expr, result, expected)
}

func (c *Checker) validateBuiltin(expr ast.Expr, local scope, expected *ast.Type) ast.Type {
	if expr.Value == "json_decode" {
		if len(expr.Args) != 2 {
			for _, arg := range expr.Args {
				c.validateExpr(arg, local, nil)
			}
			return unknownType
		}
		target := c.resolveTypeArgument(expr.Args[0])
		body := c.validateExpr(expr.Args[1], local, &bytesType)
		c.requireAssignable(expr.Args[1].Pos, bytesType, body, "JSON input")
		return ast.Type{Name: "result", Args: []ast.Type{target, stringType}}
	}

	argumentTypes := make([]ast.Type, 0, len(expr.Args))
	for _, arg := range expr.Args {
		argumentTypes = append(argumentTypes, c.validateExpr(arg, local, nil))
	}
	if len(argumentTypes) < builtins[expr.Value].min {
		return unknownType
	}

	switch expr.Value {
	case "add", "subtract", "multiply", "divide", "remainder":
		c.requireNumeric(expr.Pos, argumentTypes[0], expr.Value)
		c.requireAssignable(expr.Pos, argumentTypes[0], argumentTypes[1], expr.Value+" operands")
		return argumentTypes[0]
	case "negate":
		c.requireNumeric(expr.Pos, argumentTypes[0], expr.Value)
		return argumentTypes[0]
	case "greater_than", "less_than", "greater_equal", "less_equal":
		c.requireNumeric(expr.Pos, argumentTypes[0], expr.Value)
		c.requireAssignable(expr.Pos, argumentTypes[0], argumentTypes[1], expr.Value+" operands")
		return boolType
	case "and", "or":
		c.requireAssignable(expr.Pos, boolType, argumentTypes[0], expr.Value+" operand")
		c.requireAssignable(expr.Pos, boolType, argumentTypes[1], expr.Value+" operand")
		return boolType
	case "not":
		c.requireAssignable(expr.Pos, boolType, argumentTypes[0], "not operand")
		return boolType
	case "concat":
		for _, argumentType := range argumentTypes {
			c.requireAssignable(expr.Pos, stringType, argumentType, "concat argument")
		}
		return stringType
	case "trim", "lowercase", "uppercase":
		c.requireAssignable(expr.Pos, stringType, argumentTypes[0], expr.Value+" argument")
		return stringType
	case "contains", "starts_with":
		c.requireAssignable(expr.Pos, stringType, argumentTypes[0], expr.Value+" argument")
		c.requireAssignable(expr.Pos, stringType, argumentTypes[1], expr.Value+" argument")
		return boolType
	case "is_some", "is_none":
		if argumentTypes[0].Name != "optional" && !isUnknown(argumentTypes[0]) {
			c.error(expr.Pos, "VRB1430", fmt.Sprintf("%s requires optional T but received %s", expr.Value, argumentTypes[0].String()), "pass an optional value")
		}
		return boolType
	case "unwrap":
		if argumentTypes[0].Name == "optional" && len(argumentTypes[0].Args) == 1 {
			return argumentTypes[0].Args[0]
		}
		if !isUnknown(argumentTypes[0]) {
			c.error(expr.Pos, "VRB1431", fmt.Sprintf("unwrap requires optional T but received %s", argumentTypes[0].String()), "check with is_some before unwrapping an optional value")
		}
		return unknownType
	case "ok", "error":
		if expected == nil || expected.Name != "result" || len(expected.Args) != 2 {
			c.error(expr.Pos, "VRB1432", fmt.Sprintf("cannot infer result type for %s", expr.Value), "use ok or error where a result T E output type is expected")
			return unknownType
		}
		argumentIndex := 0
		if expr.Value == "error" {
			argumentIndex = 1
		}
		c.requireAssignable(expr.Pos, expected.Args[argumentIndex], argumentTypes[0], expr.Value+" value")
		return *expected
	case "new_uuid":
		return uuidType
	case "parse_uuid":
		c.requireAssignable(expr.Pos, stringType, argumentTypes[0], "parse_uuid argument")
		return ast.Type{Name: "result", Args: []ast.Type{uuidType, stringType}}
	case "regex_match":
		if argumentTypes[0].Name != "resource_regex" && !sameType(argumentTypes[0], stringType) && !isUnknown(argumentTypes[0]) {
			c.error(expr.Pos, "VRB1433", fmt.Sprintf("regex_match requires a regex resource or string pattern, received %s", argumentTypes[0].String()), "pass an embed regex resource")
		}
		c.requireAssignable(expr.Pos, stringType, argumentTypes[1], "regex input")
		return boolType
	case "json_encode":
		return bytesType
	default:
		return unknownType
	}
}

func (c *Checker) validateRenderBuiltin(expr ast.Expr, local scope) ast.Type {
	if len(expr.Args) != 1 {
		for _, arg := range expr.Args {
			c.validateExpr(arg, local, nil)
		}
		return stringType
	}
	resource := expr.Args[0]
	embed := c.embeds[resource.Value]
	if resource.Kind != ast.ExprAtom || embed == nil || (embed.Kind != "html" && embed.Kind != "text") {
		c.validateExpr(resource, local, nil)
		c.error(expr.Pos, "HTML2302", fmt.Sprintf("%s is not an HTML or text template island", resource.Value), "pass the name of an embed html or embed text resource")
		return stringType
	}
	expectedSlots := map[string]bool{}
	for _, match := range htmlSlot.FindAllStringSubmatch(embed.Raw, -1) {
		expectedSlots[match[1]] = true
	}
	actual := map[string]bool{}
	for _, arg := range expr.NamedArgs {
		valueType := c.validateExpr(arg.Value, local, &stringType)
		c.requireAssignable(arg.Pos, stringType, valueType, fmt.Sprintf("template slot %s", arg.Name))
		if actual[arg.Name] {
			c.error(arg.Pos, "HTML2303", fmt.Sprintf("duplicate template binding %s", arg.Name), "bind each template slot once")
		}
		actual[arg.Name] = true
		if !expectedSlots[arg.Name] {
			c.error(arg.Pos, "HTML2304", fmt.Sprintf("binding %s is not used by template island %s", arg.Name, embed.Name), "remove the extra binding or add the slot to the template")
		}
	}
	for name := range expectedSlots {
		if !actual[name] {
			c.error(expr.Pos, "HTML2307", fmt.Sprintf("template slot %s in island %s is not bound", name, embed.Name), "add a with binding for the template slot")
		}
	}
	return stringType
}

func (c *Checker) validateSQLBuiltin(expr ast.Expr, local scope, expected *ast.Type) ast.Type {
	if len(expr.Args) > 0 {
		resource := expr.Args[0]
		if resource.Kind != ast.ExprAtom || c.embeds[resource.Value] == nil || c.embeds[resource.Value].Kind != "sql" {
			c.validateExpr(resource, local, nil)
		}
	}
	for _, arg := range expr.Args[1:] {
		c.validateExpr(arg, local, nil)
	}
	for _, arg := range expr.NamedArgs {
		c.validateExpr(arg.Value, local, nil)
	}
	c.validateSQLCall(expr)
	rowType := unknownType
	var valueType ast.Type
	switch expr.Value {
	case "sql_exec":
		valueType = intType
	case "sql_one":
		valueType = rowType
	case "sql_optional":
		valueType = ast.Type{Name: "optional", Args: []ast.Type{rowType}}
	case "sql_many":
		valueType = ast.Type{Name: "list", Args: []ast.Type{rowType}}
	default:
		return unknownType
	}
	return c.applyTry(expr, ast.Type{Name: "result", Args: []ast.Type{valueType, stringType}}, expected)
}

func (c *Checker) resolveTypeArgument(expr ast.Expr) ast.Type {
	if expr.Kind != ast.ExprAtom {
		c.error(expr.Pos, "VRB1434", "JSON target must be a record or enum type name", "pass a declared type as the first json_decode argument")
		return unknownType
	}
	decl := c.decls[expr.Value]
	switch decl.(type) {
	case *ast.Record, *ast.Enum:
		return ast.Type{Name: expr.Value}
	default:
		c.error(expr.Pos, "VRB1434", fmt.Sprintf("%s is not a record or enum type", expr.Value), "pass a declared type as the first json_decode argument")
		return unknownType
	}
}

func (c *Checker) applyTry(expr ast.Expr, result ast.Type, expected *ast.Type) ast.Type {
	if !expr.Try {
		return result
	}
	if result.Name != "result" || len(result.Args) != 2 {
		c.error(expr.Pos, "VRB1435", fmt.Sprintf("try requires result T E but call %s returns %s", expr.Value, result.String()), "remove try or call a function that returns result")
		return unknownType
	}
	if expected == nil || expected.Name != "result" || len(expected.Args) != 2 {
		c.error(expr.Pos, "VRB1436", "try can only be used in a function or route returning result T E", "declare a compatible result output type or handle the error explicitly")
		return result.Args[0]
	}
	if !sameType(result.Args[1], expected.Args[1]) && !isUnknown(result.Args[1]) && !isUnknown(expected.Args[1]) {
		c.error(expr.Pos, "VRB1437", fmt.Sprintf("try error type %s does not match enclosing error type %s", result.Args[1].String(), expected.Args[1].String()), "map the error to the enclosing result error type")
	}
	return result.Args[0]
}

func (c *Checker) resolveFieldPath(pos ast.Position, root ast.Type, path []string) ast.Type {
	current := root
	for _, name := range path {
		if isUnknown(current) {
			return unknownType
		}
		if current.Name == "optional" {
			c.error(pos, "VRB1440", fmt.Sprintf("cannot access field %s through optional %s", name, current.String()), "check with is_some and unwrap the value first")
			return unknownType
		}
		record := c.records[current.Name]
		if record == nil {
			c.error(pos, "VRB1441", fmt.Sprintf("type %s has no fields", current.String()), "use get only with record values")
			return unknownType
		}
		found := false
		for _, field := range record.Fields {
			if field.Name == name {
				current = field.Type
				found = true
				break
			}
		}
		if !found {
			c.error(pos, "VRB1442", fmt.Sprintf("record %s has no field %s", record.Name, name), "use a field declared by the record")
			return unknownType
		}
	}
	return current
}

func (c *Checker) requireAssignable(pos ast.Position, expected, actual ast.Type, subject string) {
	if isUnknown(expected) || isUnknown(actual) || sameType(expected, actual) {
		return
	}
	c.error(pos, "VRB1422", fmt.Sprintf("%s requires %s but received %s", subject, expected.String(), actual.String()), "use a value with the expected type; Verba does not apply implicit conversions")
}

func (c *Checker) requireNumeric(pos ast.Position, value ast.Type, subject string) {
	if isUnknown(value) || isNumeric(value) {
		return
	}
	c.error(pos, "VRB1425", fmt.Sprintf("%s requires a numeric value but received %s", subject, value.String()), "use an integer, decimal, or floating-point value")
}

func (c *Checker) isComparable(value ast.Type) bool {
	if isUnknown(value) {
		return true
	}
	if value.Name == "optional" && len(value.Args) == 1 {
		return c.isComparable(value.Args[0])
	}
	if _, exists := c.records[value.Name]; exists {
		return false
	}
	switch value.Name {
	case "list", "map", "result", "<unit>":
		return false
	default:
		return true
	}
}

func (c *Checker) isMatchable(value ast.Type) bool {
	if !c.isComparable(value) {
		return false
	}
	switch value.Name {
	case "bytes", "optional", "time":
		return false
	default:
		return true
	}
}

func (c *Checker) validateSQLCall(expr ast.Expr) {
	if len(expr.Args) == 0 {
		return
	}
	resource := expr.Args[0].Value
	embed := c.embeds[resource]
	if embed == nil || embed.Kind != "sql" {
		c.error(expr.Pos, "SQL2101", fmt.Sprintf("%s is not a SQL island", resource), "pass the name of an embed sql resource")
		return
	}
	expected := map[string]bool{}
	for _, match := range sqlParameter.FindAllString(embed.Raw, -1) {
		expected[strings.TrimPrefix(match, ":")] = true
	}
	actual := map[string]bool{}
	for _, arg := range expr.NamedArgs {
		if actual[arg.Name] {
			c.error(arg.Pos, "SQL2102", fmt.Sprintf("duplicate SQL binding %s", arg.Name), "bind each SQL parameter once")
		}
		actual[arg.Name] = true
		if !expected[arg.Name] {
			c.error(arg.Pos, "SQL2103", fmt.Sprintf("binding %s is not used by SQL island %s", arg.Name, resource), "remove the extra binding or update the SQL parameter")
		}
	}
	for name := range expected {
		if !actual[name] {
			c.error(expr.Pos, "SQL2107", fmt.Sprintf("parameter :%s is declared by SQL island %s but is not bound", name, resource), "add a with binding in the call argument block")
		}
	}
}

func (c *Checker) error(pos ast.Position, code, message, hint string) {
	c.diagnostics = append(c.diagnostics, diagnostic.Diagnostic{Severity: diagnostic.Error, Code: code, File: pos.File, Line: pos.Line, Column: pos.Column, Message: message, Hint: hint})
}

func cloneScope(source scope) scope {
	result := make(scope, len(source))
	for name, value := range source {
		result[name] = value
	}
	return result
}

func literalType(value string) (ast.Type, bool) {
	if value == "true" || value == "false" {
		return boolType, true
	}
	if _, err := strconv.ParseInt(value, 10, 64); err == nil {
		return intType, true
	}
	if _, err := strconv.ParseFloat(value, 64); err == nil {
		return floatType, true
	}
	return ast.Type{}, false
}

func sameType(left, right ast.Type) bool {
	if left.Name != right.Name || len(left.Args) != len(right.Args) {
		return false
	}
	for index := range left.Args {
		if !sameType(left.Args[index], right.Args[index]) {
			return false
		}
	}
	return true
}

func isUnknown(value ast.Type) bool {
	return value.Name == unknownType.Name
}

func isNumeric(value ast.Type) bool {
	switch value.Name {
	case "int", "uint", "int8", "int16", "int32", "int64", "uint8", "uint16", "uint32", "uint64", "float32", "float64", "decimal":
		return true
	default:
		return false
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
		name := path[start+1 : start+1+end]
		if name != "" {
			result = append(result, name)
		}
		path = path[start+1+end+1:]
	}
}
