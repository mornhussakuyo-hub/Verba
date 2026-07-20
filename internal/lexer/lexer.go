package lexer

import (
	"github.com/verba-lang/verba/internal/ast"
	"github.com/verba-lang/verba/internal/diagnostic"
	"github.com/verba-lang/verba/internal/numeric"
	"github.com/verba-lang/verba/internal/region"
	"github.com/verba-lang/verba/internal/source"
)

type Kind uint8

const (
	Invalid Kind = iota
	Keyword
	Identifier
	Integer
	Float
	ControlledLiteral
	Comment
	Newline
	EOF
)

type Token struct {
	Kind  Kind
	Text  string
	Pos   ast.Position
	Start int
	End   int
}

type Result struct {
	Tokens      []Token
	Diagnostics []diagnostic.Diagnostic
}

type word struct {
	text       string
	start, end int
}

var keywords = map[string]bool{
	"module": true, "use": true, "begin": true, "end": true,
	"record": true, "enum": true, "function": true, "route": true, "embed": true,
	"field": true, "input": true, "output": true, "method": true,
	"let": true, "var": true, "set": true, "to": true, "be": true, "is": true, "not": true,
	"call": true, "with": true, "try": true, "get": true,
	"if": true, "else": true, "match": true, "case": true, "while": true, "for": true, "in": true, "return": true,
	"respond": true, "transaction": true, "until": true, "text": true, "url": true, "path": true, "note": true,
	"bool": true, "int": true, "uint": true, "float": true, "decimal": true, "string": true, "bytes": true,
	"optional": true, "list": true, "map": true, "result": true,
}

func Lex(file *source.File, regions region.Result) Result {
	var result Result
	spans := regions.CoreSpans()
	spanIndex := 0
	for _, line := range file.Lines() {
		for spanIndex < len(spans) && line.Start >= spans[spanIndex].End {
			spanIndex++
		}
		if spanIndex >= len(spans) || line.Start < spans[spanIndex].Start || line.Start >= spans[spanIndex].End {
			continue
		}
		lexLine(file, line, &result)
	}
	location := file.Position(file.Len())
	result.Tokens = append(result.Tokens, Token{Kind: EOF, Pos: position(file.Path, location), Start: file.Len(), End: file.Len()})
	return result
}

func SplitControlled(raw string) (string, string, Kind, bool) {
	words := splitWords(raw)
	wordLimit, kind, literalStart := controlledBoundary(words, len(raw))
	if kind == Invalid || wordLimit == 0 {
		return "", "", Invalid, false
	}
	start := literalStart
	for start < len(raw) && (raw[start] == ' ' || raw[start] == '\t') {
		start++
	}
	prefix := raw[:words[wordLimit-1].end]
	if start == len(raw) {
		return prefix, "", kind, true
	}
	return prefix, raw[start:], kind, true
}

func lexLine(file *source.File, line source.Line, result *Result) {
	raw := file.LineText(line)
	words := splitWords(raw)
	wordLimit, literalKind, literalStart := controlledBoundary(words, len(raw))
	for index, item := range words {
		if index >= wordLimit {
			break
		}
		start := line.Start + item.start
		kind := classify(item.text)
		if kind == Invalid {
			code, message, hint := invalidToken(item.text)
			location := file.Position(start + invalidByte(item.text))
			result.Diagnostics = append(result.Diagnostics, diagnostic.Diagnostic{
				Severity: diagnostic.Error,
				Code:     code,
				File:     file.Path,
				Line:     location.Line,
				Column:   location.Column,
				Message:  message,
				Hint:     hint,
			})
		}
		location := file.Position(start)
		result.Tokens = append(result.Tokens, Token{Kind: kind, Text: item.text, Pos: position(file.Path, location), Start: start, End: line.Start + item.end})
	}
	if literalKind != Invalid && literalStart < len(raw) {
		start := literalStart
		for start < len(raw) && (raw[start] == ' ' || raw[start] == '\t') {
			start++
		}
		if start < len(raw) {
			location := file.Position(line.Start + start)
			result.Tokens = append(result.Tokens, Token{Kind: literalKind, Text: raw[start:], Pos: position(file.Path, location), Start: line.Start + start, End: line.End})
		}
	}
	location := file.Position(line.End)
	result.Tokens = append(result.Tokens, Token{Kind: Newline, Pos: position(file.Path, location), Start: line.End, End: line.End})
}

func splitWords(raw string) []word {
	var result []word
	for start := 0; start < len(raw); {
		for start < len(raw) && (raw[start] == ' ' || raw[start] == '\t') {
			start++
		}
		if start == len(raw) {
			break
		}
		end := start
		for end < len(raw) && raw[end] != ' ' && raw[end] != '\t' {
			end++
		}
		result = append(result, word{text: raw[start:end], start: start, end: end})
		start = end
	}
	return result
}

func controlledBoundary(words []word, lineLength int) (int, Kind, int) {
	if len(words) == 0 {
		return 0, Invalid, lineLength
	}
	if words[0].text == "note" {
		return 1, Comment, afterWord(words, 0, lineLength)
	}
	if words[0].text == "path" {
		return 1, ControlledLiteral, afterWord(words, 0, lineLength)
	}
	if words[0].text == "respond" && len(words) >= 3 && words[1].text == "text" {
		return min(3, len(words)), ControlledLiteral, afterWord(words, 2, lineLength)
	}
	marker := -1
	switch words[0].text {
	case "let", "var", "set":
		for index := 1; index+2 < len(words); index++ {
			if words[index].text == "to" && words[index+1].text == "be" {
				marker = index + 2
				break
			}
		}
	case "return", "case":
		marker = 1
	case "with":
		marker = 2
	}
	if marker >= 0 && marker < len(words) && isLiteralMarker(words[marker].text) {
		return marker + 1, ControlledLiteral, afterWord(words, marker, lineLength)
	}
	return len(words), Invalid, lineLength
}

func afterWord(words []word, index, lineLength int) int {
	if index < 0 || index >= len(words) {
		return lineLength
	}
	return words[index].end
}

func isLiteralMarker(value string) bool {
	return value == "text" || value == "url" || value == "path"
}

func classify(value string) Kind {
	if keywords[value] {
		return Keyword
	}
	if identifier(value) {
		return Identifier
	}
	if numeric.Classify(value) == numeric.Integer {
		return Integer
	}
	if numeric.Classify(value) == numeric.Real {
		return Float
	}
	return Invalid
}

func invalidToken(value string) (string, string, string) {
	if len(value) != 0 && (value[0] >= '0' && value[0] <= '9' || (value[0] == '+' || value[0] == '-') && len(value) > 1 && value[1] >= '0' && value[1] <= '9') {
		return "VRB0602", "invalid numeric literal " + value, "use digits with an optional sign, decimal fraction, or exponent"
	}
	return "VRB0601", "invalid core token " + value, "core syntax accepts identifiers, numbers, and whitespace; introduce text, url, or path for symbolic content"
}

func invalidByte(value string) int {
	for index, char := range value {
		if char == '_' || char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || index > 0 && char >= '0' && char <= '9' {
			continue
		}
		if index == 0 && (char == '+' || char == '-') {
			continue
		}
		return index
	}
	return 0
}

func identifier(value string) bool {
	if value == "" {
		return false
	}
	for index, char := range value {
		if char == '_' || char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || index > 0 && char >= '0' && char <= '9' {
			continue
		}
		return false
	}
	return true
}

func position(path string, location source.Location) ast.Position {
	return ast.Position{File: path, Offset: location.Offset, Line: location.Line, Column: location.Column}
}
