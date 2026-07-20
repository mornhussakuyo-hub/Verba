package parser

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/verba-lang/verba/internal/ast"
	"github.com/verba-lang/verba/internal/diagnostic"
	"github.com/verba-lang/verba/internal/lexer"
	"github.com/verba-lang/verba/internal/region"
	"github.com/verba-lang/verba/internal/source"
)

type sourceLine struct {
	number int
	offset int
	raw    string
	text   string
}

type parser struct {
	path        string
	source      *source.File
	regions     region.Result
	lines       []sourceLine
	index       int
	declKind    string
	declName    string
	diagnostics []diagnostic.Diagnostic
}

func Parse(path string, content []byte) (*ast.File, []diagnostic.Diagnostic) {
	file, sourceDiagnostics := source.New(path, content)
	if diagnostic.HasErrors(sourceDiagnostics) {
		return &ast.File{Path: path}, sourceDiagnostics
	}
	parsed, diagnostics := ParseFile(file)
	return parsed, append(sourceDiagnostics, diagnostics...)
}

func ParseFile(file *source.File) (*ast.File, []diagnostic.Diagnostic) {
	regions := region.Scan(file)
	lexed := lexer.Lex(file, regions)
	p := &parser{path: file.Path, source: file, regions: regions}
	p.diagnostics = append(p.diagnostics, regions.Diagnostics...)
	p.diagnostics = append(p.diagnostics, lexed.Diagnostics...)
	for _, line := range file.Lines() {
		raw := file.LineText(line)
		p.lines = append(p.lines, sourceLine{number: line.Number, offset: line.Start, raw: raw, text: strings.TrimSpace(raw)})
	}
	parsed := p.parseFile()
	return parsed, p.diagnostics
}

func (p *parser) parseFile() *ast.File {
	file := &ast.File{Path: p.path}
	p.skipTrivia()
	if p.done() || !strings.HasPrefix(p.current().text, "module ") {
		p.error(p.position(), "VRB0101", "the file must begin with a module declaration", "add: module your_module")
	} else {
		parts := strings.Fields(p.current().text)
		if len(parts) != 2 || !validIdentifier(parts[1]) {
			p.error(p.position(), "VRB0102", "module requires one valid identifier", "example: module user_service")
		} else {
			file.Module = parts[1]
		}
		p.index++
	}

	for !p.done() {
		p.skipTrivia()
		if p.done() {
			break
		}
		line := p.current()
		parts := strings.Fields(line.text)
		if len(parts) == 0 {
			p.index++
			continue
		}
		switch parts[0] {
		case "use":
			if len(parts) < 2 {
				p.error(p.position(), "VRB0110", "use requires a capability or package name", "example: use http")
			} else {
				file.Uses = append(file.Uses, ast.Use{Parts: parts[1:], Pos: p.position()})
			}
			p.index++
		case "record":
			if decl := p.parseRecord(parts); decl != nil {
				file.Decls = append(file.Decls, decl)
			}
		case "enum":
			if decl := p.parseEnum(parts); decl != nil {
				file.Decls = append(file.Decls, decl)
			}
		case "function":
			if decl := p.parseFunction(parts); decl != nil {
				file.Decls = append(file.Decls, decl)
			}
		case "route":
			if decl := p.parseRoute(parts); decl != nil {
				file.Decls = append(file.Decls, decl)
			}
		case "embed":
			if decl := p.parseEmbed(parts); decl != nil {
				file.Decls = append(file.Decls, decl)
			}
		default:
			p.error(p.position(), "VRB0103", fmt.Sprintf("unexpected top-level declaration %q", parts[0]), "expected use, record, enum, function, route, or embed")
			p.index++
		}
	}
	return file
}

func (p *parser) parseRecord(parts []string) ast.Decl {
	pos := p.position()
	name, ok := p.declarationName(parts, "record")
	p.index++
	if !p.consumeBegin("record", name) {
		return nil
	}
	record := &ast.Record{Name: name, Pos: pos}
	for !p.done() {
		p.skipTrivia()
		if p.done() {
			break
		}
		lineParts := strings.Fields(p.current().text)
		if len(lineParts) == 1 && lineParts[0] == "end" {
			p.index++
			return record
		}
		if len(lineParts) < 3 || lineParts[0] != "field" || !validIdentifier(lineParts[1]) {
			p.error(p.position(), "VRB0201", "record body only accepts field declarations", "example: field name string")
			p.index++
			continue
		}
		t, consumed, valid := parseType(lineParts[2:])
		if !valid || consumed != len(lineParts)-2 {
			p.error(p.position(), "VRB0202", "invalid field type", "use a named type or optional/list/map/result type constructor")
		} else {
			record.Fields = append(record.Fields, ast.Field{Name: lineParts[1], Type: t, Pos: p.position()})
		}
		p.index++
	}
	p.error(pos, "VRB0203", fmt.Sprintf("record %s is missing end", name), "close the record with end")
	if !ok {
		return nil
	}
	return record
}

func (p *parser) parseEnum(parts []string) ast.Decl {
	pos := p.position()
	name, ok := p.declarationName(parts, "enum")
	p.index++
	if !p.consumeBegin("enum", name) {
		return nil
	}
	enum := &ast.Enum{Name: name, Pos: pos}
	for !p.done() {
		p.skipTrivia()
		if p.done() {
			break
		}
		lineParts := strings.Fields(p.current().text)
		if len(lineParts) == 1 && lineParts[0] == "end" {
			p.index++
			return enum
		}
		if len(lineParts) != 2 || lineParts[0] != "case" || !validIdentifier(lineParts[1]) {
			p.error(p.position(), "VRB0211", "enum body only accepts case declarations", "example: case active")
		} else {
			enum.Cases = append(enum.Cases, ast.Field{Name: lineParts[1], Pos: p.position()})
		}
		p.index++
	}
	p.error(pos, "VRB0212", fmt.Sprintf("enum %s is missing end", name), "close the enum with end")
	if !ok {
		return nil
	}
	return enum
}

func (p *parser) parseFunction(parts []string) ast.Decl {
	pos := p.position()
	name, ok := p.declarationName(parts, "function")
	p.index++
	fn := &ast.Function{Name: name, Pos: pos}
	for !p.done() {
		p.skipTrivia()
		if p.done() {
			break
		}
		lineParts := strings.Fields(p.current().text)
		if len(lineParts) > 0 && lineParts[0] == "begin" {
			previousKind, previousName := p.declKind, p.declName
			p.declKind, p.declName = "function", name
			fn.Body = p.parseBlock("function", name)
			p.declKind, p.declName = previousKind, previousName
			if !ok {
				return nil
			}
			return fn
		}
		if len(lineParts) >= 3 && lineParts[0] == "input" && validIdentifier(lineParts[1]) {
			t, consumed, valid := parseType(lineParts[2:])
			if !valid || consumed != len(lineParts)-2 {
				p.error(p.position(), "VRB0221", "invalid input type", "example: input value string")
			} else {
				fn.Inputs = append(fn.Inputs, ast.Field{Name: lineParts[1], Type: t, Pos: p.position()})
			}
			p.index++
			continue
		}
		if len(lineParts) >= 2 && lineParts[0] == "output" {
			t, consumed, valid := parseType(lineParts[1:])
			if !valid || consumed != len(lineParts)-1 {
				p.error(p.position(), "VRB0222", "invalid output type", "example: output string")
			} else {
				fn.Output = &t
			}
			p.index++
			continue
		}
		p.error(p.position(), "VRB0223", "expected input, output, or begin in function declaration", "function bodies start with begin")
		p.index++
	}
	p.error(pos, "VRB0224", fmt.Sprintf("function %s has no body", name), "add begin and end")
	return fn
}

func (p *parser) parseRoute(parts []string) ast.Decl {
	pos := p.position()
	name, ok := p.declarationName(parts, "route")
	p.index++
	route := &ast.Route{Name: name, Pos: pos}
	for !p.done() {
		p.skipTrivia()
		if p.done() {
			break
		}
		line := p.current().text
		lineParts := strings.Fields(line)
		if len(lineParts) > 0 && lineParts[0] == "begin" {
			previousKind, previousName := p.declKind, p.declName
			p.declKind, p.declName = "route", name
			route.Body = p.parseBlock("route", name)
			p.declKind, p.declName = previousKind, previousName
			if !ok {
				return nil
			}
			return route
		}
		switch {
		case len(lineParts) == 2 && lineParts[0] == "method":
			route.Method = strings.ToLower(lineParts[1])
		case strings.HasPrefix(line, "path "):
			route.Path = strings.TrimSpace(strings.TrimPrefix(line, "path "))
		case len(lineParts) >= 2 && lineParts[0] == "output":
			t, consumed, valid := parseType(lineParts[1:])
			if !valid || consumed != len(lineParts)-1 {
				p.error(p.position(), "VRB0231", "invalid route output type", "example: output user")
			} else {
				route.Output = &t
			}
		default:
			p.error(p.position(), "VRB0232", "expected method, path, output, or begin in route declaration", "routes require method and path metadata")
		}
		p.index++
	}
	p.error(pos, "VRB0233", fmt.Sprintf("route %s has no body", name), "add begin and end")
	return route
}

func (p *parser) parseEmbed(parts []string) ast.Decl {
	pos := p.position()
	if len(parts) != 5 || parts[3] != "until" || !validIdentifier(parts[1]) || !validIdentifier(parts[2]) || !validIdentifier(parts[4]) {
		p.error(pos, "VRB0241", "invalid embed declaration", "use: embed kind name until terminator")
		p.index++
		return nil
	}
	embed := &ast.Embed{Kind: parts[1], Name: parts[2], Terminator: parts[4], Pos: pos}
	island, ok := p.regions.IslandAtHeader(pos.Line)
	if !ok {
		p.index++
		return embed
	}
	embed.RawStart = island.Content.Start
	embed.RawEnd = island.Content.End
	embed.Raw = string(p.source.Slice(embed.RawStart, embed.RawEnd))
	if island.Terminated {
		p.index = island.TerminatorLine
	} else {
		p.index = len(p.lines)
	}
	return embed
}

func (p *parser) parseBlock(ownerKind, ownerName string) []ast.Stmt {
	if p.done() || p.current().text != "begin" {
		p.error(p.position(), "VRB0301", fmt.Sprintf("expected begin for %s", p.describeOwner(ownerKind, ownerName)), "add begin on its own line")
		return nil
	}
	p.index++
	var statements []ast.Stmt
	for !p.done() {
		p.skipTrivia()
		if p.done() {
			break
		}
		if p.current().text == "end" {
			p.index++
			return statements
		}
		if stmt := p.parseStatement(); stmt != nil {
			statements = append(statements, stmt)
		}
	}
	p.error(ast.Position{File: p.path, Line: max(1, len(p.lines)), Column: 1}, "VRB0311", fmt.Sprintf("expected end for %s before end of file", p.describeOwner(ownerKind, ownerName)), "close the block with end")
	return statements
}

func (p *parser) parseStatement() ast.Stmt {
	line := p.current()
	parts := strings.Fields(line.text)
	pos := p.position()
	if len(parts) == 0 {
		p.index++
		return nil
	}
	switch parts[0] {
	case "let", "var":
		if len(parts) < 5 || parts[2] != "to" || parts[3] != "be" || !validIdentifier(parts[1]) {
			p.error(pos, "VRB0401", "invalid binding statement", "use: let name to be expression")
			p.index++
			return nil
		}
		expr := p.parseExpr(parts[4:], pos)
		p.index++
		p.attachNamedArguments(&expr)
		return &ast.LetStmt{Name: parts[1], Mutable: parts[0] == "var", Value: expr, Pos: pos}
	case "set":
		marker := findPair(parts, "to", "be")
		if marker < 2 {
			p.error(pos, "VRB0402", "invalid set statement", "use: set name to be expression")
			p.index++
			return nil
		}
		expr := p.parseExpr(parts[marker+2:], pos)
		p.index++
		p.attachNamedArguments(&expr)
		return &ast.SetStmt{Path: parts[1:marker], Value: expr, Pos: pos}
	case "call", "try":
		expr := p.parseExpr(parts, pos)
		p.index++
		p.attachNamedArguments(&expr)
		return &ast.ExprStmt{Value: expr, Pos: pos}
	case "return":
		p.index++
		if len(parts) == 1 {
			return &ast.ReturnStmt{Pos: pos}
		}
		expr := p.parseExpr(parts[1:], pos)
		p.attachNamedArguments(&expr)
		return &ast.ReturnStmt{Value: &expr, Pos: pos}
	case "respond":
		p.index++
		if len(parts) < 3 {
			p.error(pos, "VRB0403", "respond requires a format and status", "example: respond text 200 healthy")
			return nil
		}
		status, err := strconv.Atoi(parts[2])
		if err != nil {
			p.error(pos, "VRB0404", "response status must be an integer", "example: respond json 200 value")
		}
		stmt := &ast.RespondStmt{Format: parts[1], Status: status, Pos: pos}
		if len(parts) > 3 {
			var expr ast.Expr
			if parts[1] == "text" {
				expr = ast.Expr{Kind: ast.ExprText, Value: strings.Join(parts[3:], " "), Pos: pos}
			} else {
				expr = p.parseExpr(parts[3:], pos)
			}
			stmt.Value = &expr
		}
		return stmt
	case "if":
		condition := p.parseExpr(parts[1:], pos)
		p.index++
		thenBody := p.parseBlock("if", "condition")
		p.skipTrivia()
		var elseBody []ast.Stmt
		if !p.done() && p.current().text == "else" {
			p.index++
			elseBody = p.parseBlock("else", "branch")
		}
		return &ast.IfStmt{Condition: condition, Then: thenBody, Else: elseBody, Pos: pos}
	case "for":
		if len(parts) < 4 || parts[2] != "in" || !validIdentifier(parts[1]) {
			p.error(pos, "VRB0405", "invalid for statement", "use: for item in items")
			p.index++
			return nil
		}
		iterable := p.parseExpr(parts[3:], pos)
		p.index++
		body := p.parseBlock("for", parts[1])
		return &ast.ForStmt{Name: parts[1], Iterable: iterable, Body: body, Pos: pos}
	case "while":
		condition := p.parseExpr(parts[1:], pos)
		p.index++
		body := p.parseBlock("while", "condition")
		return &ast.WhileStmt{Condition: condition, Body: body, Pos: pos}
	case "match":
		return p.parseMatch(parts, pos)
	case "transaction":
		if len(parts) != 2 {
			p.error(pos, "VRB0406", "transaction requires one resource name", "example: transaction database")
		}
		resource := ""
		if len(parts) > 1 {
			resource = parts[1]
		}
		p.index++
		body := p.parseBlock("transaction", resource)
		return &ast.TransactionStmt{Resource: resource, Body: body, Pos: pos}
	default:
		p.error(pos, "VRB0400", fmt.Sprintf("unknown statement %q", parts[0]), "expected let, var, set, call, return, respond, if, for, while, match, or transaction")
		p.index++
		return nil
	}
}

func (p *parser) parseMatch(parts []string, pos ast.Position) ast.Stmt {
	if len(parts) < 2 {
		p.error(pos, "VRB0410", "match requires a value", "example: match current_role")
		p.index++
		return nil
	}
	value := p.parseExpr(parts[1:], pos)
	p.index++
	if !p.consumeBegin("match", "value") {
		return &ast.MatchStmt{Value: value, Pos: pos}
	}
	statement := &ast.MatchStmt{Value: value, Pos: pos}
	seenElse := false
	for !p.done() {
		p.skipTrivia()
		if p.done() {
			break
		}
		line := p.current()
		caseParts := strings.Fields(line.text)
		if len(caseParts) == 1 && caseParts[0] == "end" {
			p.index++
			return statement
		}
		if len(caseParts) == 1 && caseParts[0] == "else" {
			if seenElse {
				p.error(p.position(), "VRB0411", "match has more than one else branch", "keep a single final else branch")
			}
			seenElse = true
			p.index++
			statement.Else = p.parseBlock("match else", "branch")
			continue
		}
		if len(caseParts) < 2 || caseParts[0] != "case" {
			p.error(p.position(), "VRB0412", "match body only accepts case, else, or end", "example: case admin")
			p.index++
			continue
		}
		if seenElse {
			p.error(p.position(), "VRB0413", "case cannot appear after match else", "move the else branch after all cases")
		}
		patternPos := p.position()
		pattern := p.parseExpr(caseParts[1:], patternPos)
		p.index++
		body := p.parseBlock("match case", strings.Join(caseParts[1:], " "))
		statement.Cases = append(statement.Cases, ast.MatchCase{Pattern: pattern, Body: body, Pos: patternPos})
	}
	p.error(pos, "VRB0414", fmt.Sprintf("%s is missing end", p.describeOwner("match", "value")), "close the match after its cases with end")
	return statement
}

func (p *parser) parseExpr(parts []string, pos ast.Position) ast.Expr {
	if len(parts) == 0 {
		p.error(pos, "VRB0501", "expected an expression", "provide a name, literal, call, or get expression")
		return ast.Expr{Kind: ast.ExprInvalid, Pos: pos}
	}
	try := false
	if parts[0] == "try" {
		try = true
		parts = parts[1:]
	}
	if len(parts) == 0 {
		p.error(pos, "VRB0502", "try must be followed by a call", "example: try call load_user id")
		return ast.Expr{Kind: ast.ExprInvalid, Try: true, Pos: pos}
	}
	if parts[0] == "call" {
		if len(parts) < 2 || !validIdentifier(parts[1]) {
			p.error(pos, "VRB0503", "call requires a function name", "example: call lowercase value")
			return ast.Expr{Kind: ast.ExprInvalid, Pos: pos}
		}
		expr := ast.Expr{Kind: ast.ExprCall, Value: parts[1], Try: try, Pos: pos}
		for _, value := range parts[2:] {
			expr.Args = append(expr.Args, ast.Expr{Kind: ast.ExprAtom, Value: value, Pos: pos})
		}
		return expr
	}
	if try {
		p.error(pos, "VRB0504", "try can only be applied to a call", "example: try call parse_uuid value")
	}
	if parts[0] == "get" {
		if len(parts) < 3 {
			p.error(pos, "VRB0505", "get requires a value and at least one field", "example: get user email")
		}
		return ast.Expr{Kind: ast.ExprGet, Args: atoms(parts[1:], pos), Pos: pos}
	}
	if parts[0] == "text" || parts[0] == "url" || parts[0] == "path" {
		literalType := parts[0]
		if literalType == "text" {
			literalType = "string"
		}
		return ast.Expr{Kind: ast.ExprText, Value: strings.Join(parts[1:], " "), LiteralType: literalType, Pos: pos}
	}
	for i := 1; i < len(parts); i++ {
		if parts[i] == "is" {
			right := i + 1
			not := false
			if right < len(parts) && parts[right] == "not" {
				not = true
				right++
			}
			if i != 1 || right != len(parts)-1 {
				p.error(pos, "VRB0506", "relation operands must each be one name or literal", "split complex expressions into separate let statements")
			}
			if right >= len(parts) {
				p.error(pos, "VRB0507", "is requires a right operand", "provide the value to compare")
				return ast.Expr{Kind: ast.ExprInvalid, Pos: pos}
			}
			return ast.Expr{Kind: ast.ExprRelation, Args: []ast.Expr{{Kind: ast.ExprAtom, Value: parts[0], Pos: pos}, {Kind: ast.ExprAtom, Value: parts[right], Pos: pos}}, Not: not, Pos: pos}
		}
	}
	if len(parts) != 1 {
		p.error(pos, "VRB0508", "an atom expression cannot contain multiple words", "use text for raw text or call for a function invocation")
	}
	return ast.Expr{Kind: ast.ExprAtom, Value: parts[0], Pos: pos}
}

func (p *parser) attachNamedArguments(expr *ast.Expr) {
	if expr.Kind != ast.ExprCall {
		return
	}
	p.skipTrivia()
	if p.done() || p.current().text != "begin" {
		return
	}
	p.index++
	for !p.done() {
		p.skipTrivia()
		if p.done() {
			break
		}
		if p.current().text == "end" {
			p.index++
			return
		}
		parts := strings.Fields(p.current().text)
		pos := p.position()
		if len(parts) < 3 || parts[0] != "with" || !validIdentifier(parts[1]) {
			p.error(pos, "VRB0510", "call argument block only accepts with entries", "example: with user_id id")
			p.index++
			continue
		}
		expr.NamedArgs = append(expr.NamedArgs, ast.NamedArg{Name: parts[1], Value: p.parseExpr(parts[2:], pos), Pos: pos})
		p.index++
	}
	p.error(expr.Pos, "VRB0511", fmt.Sprintf("%s is missing end", p.describeOwner("argument block for call", expr.Value)), "close the argument block with end")
}

func (p *parser) consumeBegin(kind, name string) bool {
	p.skipTrivia()
	if p.done() || p.current().text != "begin" {
		p.error(p.position(), "VRB0301", fmt.Sprintf("expected begin for %s", p.describeOwner(kind, name)), "add begin on its own line")
		return false
	}
	p.index++
	return true
}

func (p *parser) describeOwner(kind, name string) string {
	owner := strings.TrimSpace(kind + " " + name)
	if p.declKind != "" && (kind != p.declKind || name != p.declName) {
		owner += fmt.Sprintf(" in %s %s", p.declKind, p.declName)
	}
	return owner
}

func (p *parser) declarationName(parts []string, kind string) (string, bool) {
	if len(parts) != 2 || !validIdentifier(parts[1]) {
		p.error(p.position(), "VRB0104", fmt.Sprintf("%s requires one valid name", kind), fmt.Sprintf("example: %s example_name", kind))
		return "invalid", false
	}
	return parts[1], true
}

func (p *parser) skipTrivia() {
	for !p.done() {
		text := p.current().text
		if text != "" && text != "note" && !strings.HasPrefix(text, "note ") {
			return
		}
		p.index++
	}
}

func (p *parser) current() sourceLine { return p.lines[p.index] }
func (p *parser) done() bool          { return p.index >= len(p.lines) }

func (p *parser) position() ast.Position {
	if p.done() {
		location := p.source.Position(p.source.Len())
		return ast.Position{File: p.path, Offset: location.Offset, Line: location.Line, Column: location.Column}
	}
	line := p.current()
	indent := len(line.raw) - len(strings.TrimLeft(line.raw, " \t"))
	location := p.source.Position(line.offset + indent)
	return ast.Position{File: p.path, Offset: location.Offset, Line: line.number, Column: location.Column}
}

func (p *parser) error(pos ast.Position, code, message, hint string) {
	p.diagnostics = append(p.diagnostics, diagnostic.Diagnostic{Severity: diagnostic.Error, Code: code, File: pos.File, Line: pos.Line, Column: pos.Column, Message: message, Hint: hint})
}

func parseType(parts []string) (ast.Type, int, bool) {
	if len(parts) == 0 || !validIdentifier(parts[0]) {
		return ast.Type{}, 0, false
	}
	t := ast.Type{Name: parts[0]}
	consumed := 1
	arity := 0
	switch t.Name {
	case "optional", "list":
		arity = 1
	case "map", "result":
		arity = 2
	}
	for range arity {
		arg, count, ok := parseType(parts[consumed:])
		if !ok {
			return ast.Type{}, 0, false
		}
		t.Args = append(t.Args, arg)
		consumed += count
	}
	return t, consumed, true
}

func atoms(values []string, pos ast.Position) []ast.Expr {
	result := make([]ast.Expr, 0, len(values))
	for _, value := range values {
		result = append(result, ast.Expr{Kind: ast.ExprAtom, Value: value, Pos: pos})
	}
	return result
}

func findPair(parts []string, left, right string) int {
	for i := 0; i+1 < len(parts); i++ {
		if parts[i] == left && parts[i+1] == right {
			return i
		}
	}
	return -1
}

func validIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for i, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_' || (i > 0 && r >= '0' && r <= '9') {
			continue
		}
		return false
	}
	return true
}
