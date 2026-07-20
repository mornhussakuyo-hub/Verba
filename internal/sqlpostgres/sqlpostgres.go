package sqlpostgres

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/verba-lang/verba/internal/ast"
	"github.com/verba-lang/verba/internal/diagnostic"
	"github.com/verba-lang/verba/internal/source"
)

type Column struct {
	Name string
	Type ast.Type
}

type Table struct {
	Name    string
	Columns []Column
	byName  map[string]Column
}

type Schema struct {
	Path   string
	Tables map[string]*Table
}

type tokenKind uint8

const (
	wordToken tokenKind = iota
	parameterToken
	literalToken
	symbolToken
)

type token struct {
	kind       tokenKind
	text       string
	lower      string
	start, end int
}

func Load(path string) (*Schema, []diagnostic.Diagnostic, error) {
	file, diagnostics, err := source.Load(path)
	if err != nil {
		return nil, nil, err
	}
	schema := &Schema{Path: file.Path, Tables: map[string]*Table{}}
	if diagnostic.HasErrors(diagnostics) {
		return schema, diagnostics, nil
	}
	tokens, tokenDiagnostics := tokenize(string(file.Bytes()), func(offset int) ast.Position {
		location := file.Position(offset)
		return ast.Position{File: file.Path, Offset: offset, Line: location.Line, Column: location.Column}
	})
	diagnostics = append(diagnostics, tokenDiagnostics...)
	for index := 0; index < len(tokens); {
		if tokens[index].lower != "create" {
			index++
			continue
		}
		start := index
		index++
		if index < len(tokens) && (tokens[index].lower == "temporary" || tokens[index].lower == "temp") {
			index++
		}
		if index >= len(tokens) || tokens[index].lower != "table" {
			index = start + 1
			continue
		}
		index++
		if hasWords(tokens, index, "if", "not", "exists") {
			index += 3
		}
		name, next, ok := qualifiedName(tokens, index)
		if !ok || next >= len(tokens) || tokens[next].text != "(" {
			diagnostics = append(diagnostics, schemaError(file, tokens[start].start, "SQL2401", "invalid CREATE TABLE declaration", "declare a table name followed by a parenthesized column list"))
			index = start + 1
			continue
		}
		end := matchingParen(tokens, next)
		if end < 0 {
			diagnostics = append(diagnostics, schemaError(file, tokens[next].start, "SQL2402", fmt.Sprintf("table %s has an unclosed column list", name), "close the CREATE TABLE declaration with )"))
			break
		}
		table, tableDiagnostics := parseTable(file, name, tokens[next+1:end])
		diagnostics = append(diagnostics, tableDiagnostics...)
		if table != nil {
			key := normalizeName(name)
			if schema.Tables[key] != nil {
				diagnostics = append(diagnostics, schemaError(file, tokens[index].start, "SQL2403", fmt.Sprintf("duplicate table %s", name), "keep one CREATE TABLE declaration per table in the schema snapshot"))
			} else {
				schema.Tables[key] = table
				short := shortName(key)
				if existing, exists := schema.Tables[short]; !exists {
					schema.Tables[short] = table
				} else if existing != table {
					schema.Tables[short] = nil
				}
			}
		}
		index = end + 1
	}
	if len(schema.Tables) == 0 && !diagnostic.HasErrors(diagnostics) {
		diagnostics = append(diagnostics, schemaError(file, 0, "SQL2404", "schema snapshot contains no CREATE TABLE declarations", "export PostgreSQL table definitions into the configured schema file"))
	}
	diagnostic.Sort(diagnostics)
	return schema, diagnostics, nil
}

func Analyze(embed *ast.Embed, schema *Schema) []diagnostic.Diagnostic {
	position := func(offset int) ast.Position {
		line := embed.Pos.Line + 1 + strings.Count(embed.Raw[:min(offset, len(embed.Raw))], "\n")
		column := 1
		if previous := strings.LastIndex(embed.Raw[:min(offset, len(embed.Raw))], "\n"); previous >= 0 {
			column = offset - previous
		} else {
			column = offset + 1
		}
		return ast.Position{File: embed.Pos.File, Offset: embed.RawStart + offset, Line: line, Column: column}
	}
	tokens, diagnostics := tokenize(embed.Raw, position)
	if len(tokens) == 0 {
		diagnostics = append(diagnostics, queryError(position(0), "SQL2410", fmt.Sprintf("SQL island %s is empty", embed.Name), "add one SELECT, INSERT, UPDATE, or DELETE statement"))
		return diagnostics
	}
	statement := tokens[0].lower
	if statement != "select" && statement != "insert" && statement != "update" && statement != "delete" {
		diagnostics = append(diagnostics, queryError(position(tokens[0].start), "SQL2411", fmt.Sprintf("unsupported SQL statement %s", tokens[0].text), "use SELECT, INSERT, UPDATE, or DELETE in executable SQL islands"))
		return diagnostics
	}
	if multipleStatements(tokens) {
		diagnostics = append(diagnostics, queryError(position(tokens[0].start), "SQL2412", "SQL island contains more than one statement", "keep one statement per SQL island so result and parameter types stay deterministic"))
		return diagnostics
	}
	tableName, tableIndex, ok := queryTable(tokens, statement)
	if !ok {
		diagnostics = append(diagnostics, queryError(position(tokens[0].start), "SQL2413", fmt.Sprintf("cannot determine the table used by %s", embed.Name), "use a direct single-table SELECT, INSERT, UPDATE, or DELETE statement"))
		return diagnostics
	}
	var table *Table
	if schema != nil {
		table = schema.Tables[normalizeName(tableName)]
		if table == nil {
			diagnostics = append(diagnostics, queryError(position(tokens[tableIndex].start), "SQL2414", fmt.Sprintf("table %s is not declared by the schema snapshot", tableName), "update the schema snapshot or correct the SQL table name"))
			return diagnostics
		}
	}
	shapeDiagnostics := validateQueryShape(tokens, statement, table, position)
	diagnostics = append(diagnostics, shapeDiagnostics...)
	query := &ast.SQLQuery{Statement: statement}
	query.Text, query.Parameters = rewriteParameters(embed.Raw, tokens)
	if schema != nil {
		parameterTypes, parameterDiagnostics := inferParameters(tokens, statement, table, position)
		diagnostics = append(diagnostics, parameterDiagnostics...)
		for index := range query.Parameters {
			column, exists := parameterTypes[query.Parameters[index].Name]
			if !exists {
				diagnostics = append(diagnostics, queryError(position(parameterOffset(tokens, query.Parameters[index].Name)), "SQL2415", fmt.Sprintf("cannot infer a schema type for parameter :%s", query.Parameters[index].Name), "compare the parameter with a table column or use it in a direct INSERT/UPDATE column assignment"))
				continue
			}
			query.Parameters[index].Type = column.Type
		}
		columns, columnDiagnostics := inferResultColumns(tokens, statement, table, position)
		diagnostics = append(diagnostics, columnDiagnostics...)
		query.Columns = columns
	}
	embed.SQL = query
	diagnostic.Sort(diagnostics)
	return diagnostics
}

func validateQueryShape(tokens []token, statement string, table *Table, position func(int) ast.Position) []diagnostic.Diagnostic {
	var diagnostics []diagnostic.Diagnostic
	for _, unsupported := range []string{"join", "union", "intersect", "except", "using"} {
		if index := findWord(tokens, 0, unsupported); index >= 0 {
			diagnostics = append(diagnostics, queryError(position(tokens[index].start), "SQL2423", fmt.Sprintf("%s is not supported by typed single-table SQL islands", strings.ToUpper(unsupported)), "split the operation into direct single-table statements or extend the PostgreSQL adapter"))
		}
	}
	selectCount := 0
	for _, item := range tokens {
		if item.lower == "select" {
			selectCount++
		}
	}
	if selectCount > 1 {
		diagnostics = append(diagnostics, queryError(position(tokens[0].start), "SQL2423", "subqueries are not supported by typed SQL islands", "use a direct single-table statement so row and parameter types remain deterministic"))
	}
	if statement == "insert" && findWord(tokens, 0, "values") < 0 {
		diagnostics = append(diagnostics, queryError(position(tokens[0].start), "SQL2423", "typed INSERT islands require a VALUES clause", "insert direct values with named parameters and an explicit column list"))
	}
	if statement == "update" {
		if from := findWord(tokens, 1, "from"); from >= 0 {
			diagnostics = append(diagnostics, queryError(position(tokens[from].start), "SQL2423", "UPDATE FROM is not supported by typed SQL islands", "use a direct single-table UPDATE statement"))
		}
	}
	if table == nil {
		return diagnostics
	}
	switch statement {
	case "insert":
		into := findWord(tokens, 0, "into")
		values := findWord(tokens, into+1, "values")
		open := findSymbol(tokens, into+1, "(")
		if open < 0 || open > values {
			diagnostics = append(diagnostics, queryError(position(tokens[into].start), "SQL2423", "typed INSERT islands require an explicit column list", "list target columns between the table name and VALUES"))
			break
		}
		end := matchingParen(tokens, open)
		if end < 0 {
			break
		}
		for _, item := range splitTopLevel(tokens[open+1:end], ",") {
			validateMutationColumn(item, table, position, &diagnostics)
		}
		valueOpen := findSymbol(tokens, values+1, "(")
		if valueOpen >= 0 {
			valueEnd := matchingParen(tokens, valueOpen)
			if valueEnd < 0 {
				break
			}
			columns := splitTopLevel(tokens[open+1:end], ",")
			valuesList := splitTopLevel(tokens[valueOpen+1:valueEnd], ",")
			if len(columns) != len(valuesList) {
				diagnostics = append(diagnostics, queryError(position(tokens[valueOpen].start), "SQL2426", fmt.Sprintf("INSERT lists %d columns but provides %d values", len(columns), len(valuesList)), "provide exactly one value for each target column"))
			}
			if valueEnd+1 < len(tokens) && tokens[valueEnd+1].text == "," {
				diagnostics = append(diagnostics, queryError(position(tokens[valueEnd+1].start), "SQL2423", "multi-row INSERT is not supported by typed SQL islands", "use one VALUES row per SQL execution"))
			}
		}
	case "update":
		set := findWord(tokens, 1, "set")
		end := len(tokens)
		for _, marker := range []string{"where", "returning"} {
			if index := findWord(tokens, set+1, marker); index >= 0 && index < end {
				end = index
			}
		}
		if set >= 0 {
			for _, assignment := range splitTopLevel(tokens[set+1:end], ",") {
				equals := findSymbol(assignment, 0, "=")
				if equals < 0 {
					continue
				}
				validateMutationColumn(assignment[:equals], table, position, &diagnostics)
			}
		}
	}
	return diagnostics
}

func validateMutationColumn(tokens []token, table *Table, position func(int) ast.Position, diagnostics *[]diagnostic.Diagnostic) {
	if len(tokens) != 1 || tokens[0].kind != wordToken {
		if len(tokens) > 0 {
			*diagnostics = append(*diagnostics, queryError(position(tokens[0].start), "SQL2423", "unsupported mutation target expression", "assign direct columns in typed INSERT and UPDATE statements"))
		}
		return
	}
	name := normalizeName(tokens[0].text)
	if _, exists := table.byName[name]; !exists {
		*diagnostics = append(*diagnostics, queryError(position(tokens[0].start), "SQL2424", fmt.Sprintf("column %s is not declared by table %s", name, table.Name), "update the schema snapshot or correct the mutation column"))
	}
}

func parseTable(file *source.File, name string, body []token) (*Table, []diagnostic.Diagnostic) {
	table := &Table{Name: normalizeName(name), byName: map[string]Column{}}
	var diagnostics []diagnostic.Diagnostic
	definitions := splitTopLevel(body, ",")
	for _, definition := range definitions {
		if len(definition) == 0 || isTableConstraint(definition[0].lower) {
			continue
		}
		if definition[0].kind != wordToken || len(definition) < 2 {
			diagnostics = append(diagnostics, schemaError(file, definition[0].start, "SQL2405", fmt.Sprintf("invalid column declaration in table %s", name), "declare columns as name type followed by optional constraints"))
			continue
		}
		columnName := normalizeName(definition[0].text)
		columnType, ok := postgresType(definition[1:])
		if !ok {
			diagnostics = append(diagnostics, schemaError(file, definition[1].start, "SQL2406", fmt.Sprintf("unsupported PostgreSQL type for %s.%s", name, columnName), "use a supported scalar PostgreSQL type or extend the postgres adapter"))
			continue
		}
		nullable := true
		for index := 1; index < len(definition); index++ {
			if hasWords(definition, index, "not", "null") || hasWords(definition, index, "primary", "key") {
				nullable = false
			}
		}
		if nullable {
			columnType = ast.Type{Name: "optional", Args: []ast.Type{columnType}}
		}
		column := Column{Name: columnName, Type: columnType}
		if _, exists := table.byName[columnName]; exists {
			diagnostics = append(diagnostics, schemaError(file, definition[0].start, "SQL2407", fmt.Sprintf("duplicate column %s.%s", name, columnName), "keep one declaration for each table column"))
			continue
		}
		table.Columns = append(table.Columns, column)
		table.byName[columnName] = column
	}
	for _, definition := range definitions {
		for _, columnName := range tablePrimaryKeyColumns(definition) {
			column, exists := table.byName[columnName]
			if !exists {
				diagnostics = append(diagnostics, schemaError(file, definition[0].start, "SQL2409", fmt.Sprintf("primary key references unknown column %s.%s", name, columnName), "use columns declared by the table in PRIMARY KEY constraints"))
				continue
			}
			if column.Type.Name == "optional" && len(column.Type.Args) == 1 {
				column.Type = column.Type.Args[0]
				table.byName[columnName] = column
				for index := range table.Columns {
					if table.Columns[index].Name == columnName {
						table.Columns[index] = column
					}
				}
			}
		}
	}
	if len(table.Columns) == 0 {
		return nil, diagnostics
	}
	return table, diagnostics
}

func tablePrimaryKeyColumns(definition []token) []string {
	index := 0
	if hasWords(definition, index, "constraint") {
		index += 2
	}
	if !hasWords(definition, index, "primary", "key") {
		return nil
	}
	open := findSymbol(definition, index+2, "(")
	if open < 0 {
		return nil
	}
	end := matchingParen(definition, open)
	if end < 0 {
		return nil
	}
	var columns []string
	for _, item := range splitTopLevel(definition[open+1:end], ",") {
		if len(item) == 1 && item[0].kind == wordToken {
			columns = append(columns, normalizeName(item[0].text))
		}
	}
	return columns
}

func postgresType(tokens []token) (ast.Type, bool) {
	if len(tokens) == 0 {
		return ast.Type{}, false
	}
	for _, item := range tokens {
		if item.text == "[" {
			return ast.Type{}, false
		}
	}
	name := tokens[0].lower
	if name == "double" && len(tokens) > 1 && tokens[1].lower == "precision" {
		name = "double precision"
	}
	if name == "character" && len(tokens) > 1 && tokens[1].lower == "varying" {
		name = "character varying"
	}
	switch name {
	case "boolean", "bool":
		return ast.Type{Name: "bool"}, true
	case "smallint", "int2", "smallserial", "serial2":
		return ast.Type{Name: "int16"}, true
	case "integer", "int", "int4", "serial", "serial4":
		return ast.Type{Name: "int32"}, true
	case "bigint", "int8", "bigserial", "serial8":
		return ast.Type{Name: "int64"}, true
	case "real", "float4":
		return ast.Type{Name: "float32"}, true
	case "double precision", "float8":
		return ast.Type{Name: "float64"}, true
	case "numeric", "decimal", "money":
		return ast.Type{Name: "decimal"}, true
	case "text", "varchar", "character varying", "character", "char", "citext":
		return ast.Type{Name: "string"}, true
	case "bytea", "json", "jsonb":
		return ast.Type{Name: "bytes"}, true
	case "uuid":
		return ast.Type{Name: "uuid"}, true
	case "date", "timestamp", "timestamptz":
		return ast.Type{Name: "time"}, true
	case "interval":
		return ast.Type{Name: "duration"}, true
	default:
		return ast.Type{}, false
	}
}

func inferParameters(tokens []token, statement string, table *Table, position func(int) ast.Position) (map[string]Column, []diagnostic.Diagnostic) {
	result := map[string]Column{}
	var diagnostics []diagnostic.Diagnostic
	assign := func(parameter string, column Column, offset int) {
		if column.Type.Name == "optional" && len(column.Type.Args) == 1 {
			column.Type = column.Type.Args[0]
		}
		if previous, exists := result[parameter]; exists && previous.Type.String() != column.Type.String() {
			diagnostics = append(diagnostics, queryError(position(offset), "SQL2416", fmt.Sprintf("parameter :%s is used with both %s and %s", parameter, previous.Type.String(), column.Type.String()), "use separate parameter names for values with different schema types"))
			return
		}
		result[parameter] = column
	}
	for index := 0; index+2 < len(tokens); index++ {
		if tokens[index+1].text != "=" {
			continue
		}
		if column, ok := columnAt(tokens, index, table); ok && tokens[index+2].kind == parameterToken {
			assign(tokens[index+2].text, column, tokens[index+2].start)
		}
		if tokens[index].kind == parameterToken {
			if column, ok := columnAt(tokens, index+2, table); ok {
				assign(tokens[index].text, column, tokens[index].start)
			}
		}
	}
	if statement == "insert" {
		into := findWord(tokens, 0, "into")
		values := findWord(tokens, into+1, "values")
		columnOpen := findSymbol(tokens, into+1, "(")
		valueOpen := findSymbol(tokens, values+1, "(")
		if columnOpen >= 0 && valueOpen >= 0 {
			columnEnd := matchingParen(tokens, columnOpen)
			valueEnd := matchingParen(tokens, valueOpen)
			if columnEnd > columnOpen && valueEnd > valueOpen {
				columns := splitTopLevel(tokens[columnOpen+1:columnEnd], ",")
				valuesList := splitTopLevel(tokens[valueOpen+1:valueEnd], ",")
				for index := 0; index < len(columns) && index < len(valuesList); index++ {
					if len(columns[index]) == 1 && len(valuesList[index]) == 1 && valuesList[index][0].kind == parameterToken {
						if column, exists := table.byName[normalizeName(columns[index][0].text)]; exists {
							assign(valuesList[index][0].text, column, valuesList[index][0].start)
						}
					}
				}
			}
		}
	}
	return result, diagnostics
}

func inferResultColumns(tokens []token, statement string, table *Table, position func(int) ast.Position) ([]ast.SQLColumn, []diagnostic.Diagnostic) {
	start, end := -1, len(tokens)
	if statement == "select" {
		start = 1
		end = findWord(tokens, start, "from")
	} else if returning := findWord(tokens, 0, "returning"); returning >= 0 {
		start = returning + 1
	}
	if end > start && tokens[end-1].text == ";" {
		end--
	}
	if start < 0 || end < 0 || start >= end {
		return nil, nil
	}
	var result []ast.SQLColumn
	var diagnostics []diagnostic.Diagnostic
	seen := map[string]bool{}
	appendColumn := func(column ast.SQLColumn, offset int) {
		if seen[column.Name] {
			diagnostics = append(diagnostics, queryError(position(offset), "SQL2419", fmt.Sprintf("query result contains duplicate column %s", column.Name), "use AS aliases so every result column has a unique name"))
			return
		}
		seen[column.Name] = true
		result = append(result, column)
	}
	for _, item := range splitTopLevel(tokens[start:end], ",") {
		if isStarSelection(item) {
			for _, column := range table.Columns {
				appendColumn(ast.SQLColumn{Name: column.Name, Type: column.Type}, item[0].start)
			}
			continue
		}
		columnName, outputName, ok := selectedColumn(item)
		if !ok {
			diagnostics = append(diagnostics, queryError(position(item[0].start), "SQL2417", "query result contains an unsupported expression", "select direct columns with optional AS aliases so the compiler can derive a row type"))
			continue
		}
		column, exists := table.byName[normalizeName(columnName)]
		if !exists {
			diagnostics = append(diagnostics, queryError(position(item[0].start), "SQL2418", fmt.Sprintf("column %s is not declared by table %s", columnName, table.Name), "update the schema snapshot or correct the selected column"))
			continue
		}
		appendColumn(ast.SQLColumn{Name: normalizeName(outputName), Type: column.Type}, item[0].start)
	}
	return result, diagnostics
}

func isStarSelection(tokens []token) bool {
	return len(tokens) == 1 && tokens[0].text == "*" || len(tokens) == 3 && tokens[0].kind == wordToken && tokens[1].text == "." && tokens[2].text == "*"
}

func selectedColumn(tokens []token) (string, string, bool) {
	if len(tokens) == 0 {
		return "", "", false
	}
	columnIndex := 0
	if len(tokens) >= 3 && tokens[1].text == "." {
		columnIndex = 2
	}
	if tokens[columnIndex].kind != wordToken {
		return "", "", false
	}
	name := tokens[columnIndex].text
	output := name
	rest := tokens[columnIndex+1:]
	if len(rest) == 2 && rest[0].lower == "as" && rest[1].kind == wordToken {
		output = rest[1].text
	} else if len(rest) == 1 && rest[0].kind == wordToken {
		output = rest[0].text
	} else if len(rest) != 0 {
		return "", "", false
	}
	return name, output, true
}

func rewriteParameters(raw string, tokens []token) (string, []ast.SQLParameter) {
	indices := map[string]int{}
	var parameters []ast.SQLParameter
	var builder strings.Builder
	last := 0
	for _, item := range tokens {
		if item.kind != parameterToken {
			continue
		}
		index, exists := indices[item.text]
		if !exists {
			index = len(parameters) + 1
			indices[item.text] = index
			parameters = append(parameters, ast.SQLParameter{Name: item.text})
		}
		builder.WriteString(raw[last:item.start])
		fmt.Fprintf(&builder, "$%d", index)
		last = item.end
	}
	builder.WriteString(raw[last:])
	return builder.String(), parameters
}

func tokenize(raw string, position func(int) ast.Position) ([]token, []diagnostic.Diagnostic) {
	var tokens []token
	var diagnostics []diagnostic.Diagnostic
	for index := 0; index < len(raw); {
		char := raw[index]
		if char == ' ' || char == '\t' || char == '\r' || char == '\n' {
			index++
			continue
		}
		if char == '-' && index+1 < len(raw) && raw[index+1] == '-' {
			index += 2
			for index < len(raw) && raw[index] != '\n' {
				index++
			}
			continue
		}
		if char == '/' && index+1 < len(raw) && raw[index+1] == '*' {
			start := index
			index += 2
			for index+1 < len(raw) && !(raw[index] == '*' && raw[index+1] == '/') {
				index++
			}
			if index+1 >= len(raw) {
				diagnostics = append(diagnostics, queryError(position(start), "SQL2420", "unterminated SQL block comment", "close the comment with */"))
				break
			}
			index += 2
			continue
		}
		if char == '\'' {
			start := index
			index++
			closed := false
			for index < len(raw) {
				if raw[index] == '\'' {
					index++
					if index < len(raw) && raw[index] == '\'' {
						index++
						continue
					}
					closed = true
					break
				}
				index++
			}
			if !closed {
				diagnostics = append(diagnostics, queryError(position(start), "SQL2421", "unterminated SQL string literal", "close the string literal with a single quote"))
			}
			tokens = append(tokens, token{kind: literalToken, text: raw[start:index], start: start, end: index})
			continue
		}
		if char == '"' {
			start := index
			index++
			var builder strings.Builder
			closed := false
			for index < len(raw) {
				if raw[index] == '"' {
					index++
					if index < len(raw) && raw[index] == '"' {
						builder.WriteByte('"')
						index++
						continue
					}
					closed = true
					break
				}
				builder.WriteByte(raw[index])
				index++
			}
			if !closed {
				diagnostics = append(diagnostics, queryError(position(start), "SQL2422", "unterminated quoted SQL identifier", "close the identifier with a double quote"))
			}
			text := builder.String()
			tokens = append(tokens, token{kind: wordToken, text: text, lower: text, start: start, end: index})
			continue
		}
		if char == '$' {
			if delimiter := dollarQuoteDelimiter(raw[index:]); delimiter != "" {
				start := index
				index += len(delimiter)
				if end := strings.Index(raw[index:], delimiter); end >= 0 {
					index += end + len(delimiter)
				} else {
					index = len(raw)
					diagnostics = append(diagnostics, queryError(position(start), "SQL2421", "unterminated dollar-quoted SQL string", "close the string with "+delimiter))
				}
				tokens = append(tokens, token{kind: literalToken, text: raw[start:index], start: start, end: index})
				continue
			}
		}
		if char == ':' && index+1 < len(raw) && (index == 0 || raw[index-1] != ':') && raw[index+1] != ':' && identifierStart(rune(raw[index+1])) {
			start := index
			index += 2
			for index < len(raw) && identifierContinue(rune(raw[index])) {
				index++
			}
			text := raw[start+1 : index]
			tokens = append(tokens, token{kind: parameterToken, text: text, lower: strings.ToLower(text), start: start, end: index})
			continue
		}
		if identifierStart(rune(char)) || char >= 0x80 {
			start := index
			index++
			for index < len(raw) && (identifierContinue(rune(raw[index])) || raw[index] >= 0x80) {
				index++
			}
			text := raw[start:index]
			tokens = append(tokens, token{kind: wordToken, text: text, lower: strings.ToLower(text), start: start, end: index})
			continue
		}
		tokens = append(tokens, token{kind: symbolToken, text: raw[index : index+1], lower: raw[index : index+1], start: index, end: index + 1})
		index++
	}
	return tokens, diagnostics
}

func dollarQuoteDelimiter(raw string) string {
	if len(raw) < 2 || raw[0] != '$' {
		return ""
	}
	for index := 1; index < len(raw); index++ {
		if raw[index] == '$' {
			return raw[:index+1]
		}
		if raw[index] != '_' && !unicode.IsLetter(rune(raw[index])) && !unicode.IsDigit(rune(raw[index])) {
			return ""
		}
	}
	return ""
}

func splitTopLevel(tokens []token, separator string) [][]token {
	var result [][]token
	start, depth := 0, 0
	for index, item := range tokens {
		switch item.text {
		case "(":
			depth++
		case ")":
			depth--
		case separator:
			if depth == 0 {
				result = append(result, tokens[start:index])
				start = index + 1
			}
		}
	}
	result = append(result, tokens[start:])
	return result
}

func queryTable(tokens []token, statement string) (string, int, bool) {
	marker := statement
	switch statement {
	case "select":
		marker = "from"
	case "insert":
		marker = "into"
	case "delete":
		marker = "from"
	}
	index := 0
	if marker != statement {
		index = findWord(tokens, 0, marker)
		if index < 0 {
			return "", 0, false
		}
		index++
	} else {
		index = 1
	}
	name, _, ok := qualifiedName(tokens, index)
	return name, index, ok
}

func columnAt(tokens []token, index int, table *Table) (Column, bool) {
	if index < 0 || index >= len(tokens) || tokens[index].kind != wordToken {
		return Column{}, false
	}
	name := tokens[index].text
	if index+2 < len(tokens) && tokens[index+1].text == "." && tokens[index+2].kind == wordToken {
		name = tokens[index+2].text
	}
	column, ok := table.byName[normalizeName(name)]
	return column, ok
}

func qualifiedName(tokens []token, index int) (string, int, bool) {
	if index < 0 || index >= len(tokens) || tokens[index].kind != wordToken {
		return "", index, false
	}
	name := tokens[index].text
	index++
	if index+1 < len(tokens) && tokens[index].text == "." && tokens[index+1].kind == wordToken {
		name += "." + tokens[index+1].text
		index += 2
	}
	return name, index, true
}

func matchingParen(tokens []token, open int) int {
	if open < 0 || open >= len(tokens) || tokens[open].text != "(" {
		return -1
	}
	depth := 0
	for index := open; index < len(tokens); index++ {
		switch tokens[index].text {
		case "(":
			depth++
		case ")":
			depth--
			if depth == 0 {
				return index
			}
		}
	}
	return -1
}

func multipleStatements(tokens []token) bool {
	for index, item := range tokens {
		if item.text == ";" && index != len(tokens)-1 {
			return true
		}
	}
	return false
}

func parameterOffset(tokens []token, name string) int {
	for _, item := range tokens {
		if item.kind == parameterToken && item.text == name {
			return item.start
		}
	}
	return 0
}

func findWord(tokens []token, start int, word string) int {
	for index := max(start, 0); index < len(tokens); index++ {
		if tokens[index].lower == word {
			return index
		}
	}
	return -1
}

func findSymbol(tokens []token, start int, symbol string) int {
	for index := max(start, 0); index < len(tokens); index++ {
		if tokens[index].text == symbol {
			return index
		}
	}
	return -1
}

func hasWords(tokens []token, start int, words ...string) bool {
	if start < 0 || start+len(words) > len(tokens) {
		return false
	}
	for index, word := range words {
		if tokens[start+index].lower != word {
			return false
		}
	}
	return true
}

func isTableConstraint(value string) bool {
	switch value {
	case "constraint", "primary", "unique", "foreign", "check", "exclude":
		return true
	default:
		return false
	}
}

func identifierStart(value rune) bool {
	return value == '_' || unicode.IsLetter(value)
}

func identifierContinue(value rune) bool {
	return identifierStart(value) || unicode.IsDigit(value) || value == '$'
}

func normalizeName(value string) string { return strings.ToLower(value) }

func shortName(value string) string {
	if index := strings.LastIndexByte(value, '.'); index >= 0 {
		return value[index+1:]
	}
	return value
}

func schemaError(file *source.File, offset int, code, message, hint string) diagnostic.Diagnostic {
	location := file.Position(offset)
	return diagnostic.Diagnostic{Severity: diagnostic.Error, Code: code, File: file.Path, Line: location.Line, Column: location.Column, Message: message, Hint: hint}
}

func queryError(position ast.Position, code, message, hint string) diagnostic.Diagnostic {
	return diagnostic.Diagnostic{Severity: diagnostic.Error, Code: code, File: position.File, Line: position.Line, Column: position.Column, Message: message, Hint: hint}
}
