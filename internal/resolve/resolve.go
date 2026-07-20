package resolve

import (
	"fmt"
	"sort"
	"strings"

	"github.com/verba-lang/verba/internal/ast"
	"github.com/verba-lang/verba/internal/diagnostic"
)

type Options struct {
	Dependencies    map[string]string
	DatabaseDialect string
	ManifestPath    string
}

type Capability struct {
	Name     string `json:"name"`
	Argument string `json:"argument,omitempty"`
	Explicit bool   `json:"explicit,omitempty"`
	pos      ast.Position
}

type Dependency struct {
	Name       string `json:"name"`
	Constraint string `json:"constraint"`
	Used       bool   `json:"used"`
}

type Result struct {
	Capabilities []Capability `json:"capabilities"`
	Dependencies []Dependency `json:"dependencies"`
}

type resolver struct {
	files       []*ast.File
	options     Options
	result      Result
	declared    map[string]Capability
	required    map[string]ast.Position
	usedDeps    map[string]bool
	diagnostics []diagnostic.Diagnostic
}

var explicitCapabilities = map[string]bool{
	"network_client": true,
	"file_read":      true,
	"file_write":     true,
	"process":        true,
	"environment":    true,
}

func Files(files []*ast.File, options Options) (Result, []diagnostic.Diagnostic) {
	resolver := &resolver{
		files: files, options: options, declared: map[string]Capability{},
		required: map[string]ast.Position{}, usedDeps: map[string]bool{},
		result: Result{Capabilities: []Capability{}, Dependencies: []Dependency{}},
	}
	resolver.collectUses()
	resolver.scanRequirements()
	resolver.validateRequirements()
	resolver.collectDependencies()
	sort.Slice(resolver.result.Capabilities, func(left, right int) bool {
		first, second := resolver.result.Capabilities[left], resolver.result.Capabilities[right]
		if first.Name != second.Name {
			return first.Name < second.Name
		}
		if first.Argument != second.Argument {
			return first.Argument < second.Argument
		}
		return !first.Explicit && second.Explicit
	})
	sort.Slice(resolver.result.Dependencies, func(left, right int) bool {
		return resolver.result.Dependencies[left].Name < resolver.result.Dependencies[right].Name
	})
	diagnostic.Sort(resolver.diagnostics)
	return resolver.result, resolver.diagnostics
}

func (resolver *resolver) collectUses() {
	for _, file := range resolver.files {
		for _, use := range file.Uses {
			resolver.collectUse(use)
		}
	}
}

func (resolver *resolver) collectUse(use ast.Use) {
	if len(use.Parts) == 0 {
		return
	}
	for _, part := range use.Parts {
		if !identifier(part) {
			resolver.error(use.Pos, "VRB1701", fmt.Sprintf("invalid name %q in use declaration", part), "use identifiers containing only letters, digits, and underscores")
			return
		}
	}
	name := use.Parts[0]
	var capability Capability
	switch name {
	case "http", "json", "uuid", "time":
		if len(use.Parts) != 1 {
			resolver.error(use.Pos, "VRB1702", fmt.Sprintf("use %s does not accept an argument", name), fmt.Sprintf("write: use %s", name))
			return
		}
		capability = Capability{Name: name, pos: use.Pos}
	case "sql":
		if len(use.Parts) != 2 {
			resolver.error(use.Pos, "VRB1702", "use sql requires one dialect", "write: use sql postgres")
			return
		}
		if use.Parts[1] != "postgres" {
			resolver.error(use.Pos, "VRB1703", fmt.Sprintf("unsupported SQL dialect %s", use.Parts[1]), "the first SQL adapter supports postgres")
			return
		}
		if resolver.options.DatabaseDialect != "" && resolver.options.DatabaseDialect != use.Parts[1] {
			resolver.error(use.Pos, "VRB1704", fmt.Sprintf("SQL dialect %s does not match manifest dialect %s", use.Parts[1], resolver.options.DatabaseDialect), "make use sql and [database].dialect agree")
			return
		}
		capability = Capability{Name: name, Argument: use.Parts[1], pos: use.Pos}
	case "capability":
		if len(use.Parts) != 2 {
			resolver.error(use.Pos, "VRB1702", "use capability requires one capability name", "example: use capability network_client")
			return
		}
		if !explicitCapabilities[use.Parts[1]] {
			resolver.error(use.Pos, "VRB1705", fmt.Sprintf("unknown explicit capability %s", use.Parts[1]), "use network_client, file_read, file_write, process, or environment")
			return
		}
		capability = Capability{Name: use.Parts[1], Explicit: true, pos: use.Pos}
	default:
		if _, exists := resolver.options.Dependencies[name]; !exists {
			resolver.error(use.Pos, "VRB1706", fmt.Sprintf("unknown capability or dependency %s", name), "declare the dependency in verba.toml or use a supported built-in capability")
			return
		}
		if len(use.Parts) != 1 {
			resolver.error(use.Pos, "VRB1702", fmt.Sprintf("dependency %s does not accept use arguments", name), fmt.Sprintf("write: use %s", name))
			return
		}
		resolver.usedDeps[name] = true
		return
	}
	key := capabilityKey(capability)
	if previous, exists := resolver.declared[key]; exists {
		resolver.warning(use.Pos, "VRB1707", fmt.Sprintf("duplicate use declaration for %s", displayCapability(capability)), fmt.Sprintf("first declared at %s:%d", previous.pos.File, previous.pos.Line))
		return
	}
	resolver.declared[key] = capability
	resolver.result.Capabilities = append(resolver.result.Capabilities, capability)
}

func (resolver *resolver) scanRequirements() {
	for _, file := range resolver.files {
		for _, declaration := range file.Decls {
			switch value := declaration.(type) {
			case *ast.Record:
				for _, field := range value.Fields {
					resolver.scanType(field.Type, field.Pos)
				}
			case *ast.Function:
				for _, input := range value.Inputs {
					resolver.scanType(input.Type, input.Pos)
				}
				if value.Output != nil {
					resolver.scanType(*value.Output, value.Pos)
				}
				resolver.scanStatements(value.Body)
			case *ast.Route:
				resolver.require("http", value.Pos)
				if value.Output != nil {
					resolver.scanType(*value.Output, value.Pos)
				}
				resolver.scanStatements(value.Body)
			case *ast.Embed:
				switch value.Kind {
				case "json":
					resolver.require("json", value.Pos)
				case "sql":
					resolver.require("sql", value.Pos)
				}
			}
		}
	}
}

func (resolver *resolver) scanType(value ast.Type, pos ast.Position) {
	switch value.Name {
	case "uuid":
		resolver.require("uuid", pos)
	case "time", "duration":
		resolver.require("time", pos)
	}
	for _, argument := range value.Args {
		resolver.scanType(argument, pos)
	}
}

func (resolver *resolver) scanStatements(statements []ast.Stmt) {
	for _, statement := range statements {
		switch value := statement.(type) {
		case *ast.LetStmt:
			resolver.scanExpr(value.Value)
		case *ast.SetStmt:
			resolver.scanExpr(value.Value)
		case *ast.ExprStmt:
			resolver.scanExpr(value.Value)
		case *ast.ReturnStmt:
			if value.Value != nil {
				resolver.scanExpr(*value.Value)
			}
		case *ast.RespondStmt:
			if value.Format == "json" {
				resolver.require("json", value.Pos)
			}
			if value.Value != nil {
				resolver.scanExpr(*value.Value)
			}
		case *ast.IfStmt:
			resolver.scanExpr(value.Condition)
			resolver.scanStatements(value.Then)
			resolver.scanStatements(value.Else)
		case *ast.ForStmt:
			resolver.scanExpr(value.Iterable)
			resolver.scanStatements(value.Body)
		case *ast.WhileStmt:
			resolver.scanExpr(value.Condition)
			resolver.scanStatements(value.Body)
		case *ast.MatchStmt:
			resolver.scanExpr(value.Value)
			for _, matchCase := range value.Cases {
				resolver.scanExpr(matchCase.Pattern)
				resolver.scanStatements(matchCase.Body)
			}
			resolver.scanStatements(value.Else)
		case *ast.TransactionStmt:
			resolver.require("sql", value.Pos)
			resolver.scanStatements(value.Body)
		}
	}
}

func (resolver *resolver) scanExpr(expression ast.Expr) {
	if expression.Kind == ast.ExprCall {
		switch expression.Value {
		case "json_decode", "json_encode":
			resolver.require("json", expression.Pos)
		case "new_uuid", "parse_uuid":
			resolver.require("uuid", expression.Pos)
		default:
			if strings.HasPrefix(expression.Value, "sql_") {
				resolver.require("sql", expression.Pos)
			}
		}
	}
	for _, argument := range expression.Args {
		resolver.scanExpr(argument)
	}
	for _, argument := range expression.NamedArgs {
		resolver.scanExpr(argument.Value)
	}
}

func (resolver *resolver) validateRequirements() {
	for name, pos := range resolver.required {
		if _, exists := resolver.declared[name]; exists {
			continue
		}
		hint := fmt.Sprintf("add use %s near the module declaration", name)
		if name == "sql" {
			hint = "add use sql postgres near the module declaration"
		}
		resolver.error(pos, "VRB1710", fmt.Sprintf("program uses %s without declaring the capability", name), hint)
	}
	for key, capability := range resolver.declared {
		_, required := resolver.required[key]
		if capability.Explicit || required {
			continue
		}
		resolver.warning(capability.pos, "VRB1711", fmt.Sprintf("unused capability %s", displayCapability(capability)), "remove the use declaration or use the capability")
	}
}

func (resolver *resolver) collectDependencies() {
	for name, constraint := range resolver.options.Dependencies {
		used := resolver.usedDeps[name]
		resolver.result.Dependencies = append(resolver.result.Dependencies, Dependency{Name: name, Constraint: constraint, Used: used})
		if !used {
			pos := ast.Position{File: resolver.options.ManifestPath, Line: 1, Column: 1}
			resolver.warning(pos, "VRB1712", fmt.Sprintf("dependency %s is declared but not used", name), fmt.Sprintf("add use %s or remove it from verba.toml", name))
		}
	}
}

func (resolver *resolver) require(name string, pos ast.Position) {
	if _, exists := resolver.required[name]; !exists {
		resolver.required[name] = pos
	}
}

func (resolver *resolver) error(pos ast.Position, code, message, hint string) {
	resolver.diagnostics = append(resolver.diagnostics, diagnostic.Diagnostic{Severity: diagnostic.Error, Code: code, File: pos.File, Line: pos.Line, Column: pos.Column, Message: message, Hint: hint})
}

func (resolver *resolver) warning(pos ast.Position, code, message, hint string) {
	resolver.diagnostics = append(resolver.diagnostics, diagnostic.Diagnostic{Severity: diagnostic.Warning, Code: code, File: pos.File, Line: pos.Line, Column: pos.Column, Message: message, Hint: hint})
}

func capabilityKey(capability Capability) string {
	if capability.Explicit {
		return "capability:" + capability.Name
	}
	return capability.Name
}

func displayCapability(capability Capability) string {
	if capability.Argument != "" {
		return capability.Name + " " + capability.Argument
	}
	if capability.Explicit {
		return "capability " + capability.Name
	}
	return capability.Name
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
