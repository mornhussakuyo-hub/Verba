package compiler

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/verba-lang/verba/internal/ast"
	"github.com/verba-lang/verba/internal/check"
	"github.com/verba-lang/verba/internal/diagnostic"
	"github.com/verba-lang/verba/internal/emitgo"
	"github.com/verba-lang/verba/internal/parser"
)

type Program struct {
	Paths []string
	Files []*ast.File
}

func Discover(inputs []string) ([]string, error) {
	if len(inputs) == 0 {
		inputs = []string{"."}
	}
	seen := map[string]bool{}
	var paths []string
	for _, input := range inputs {
		info, err := os.Stat(input)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			if filepath.Ext(input) != ".vrb" {
				return nil, fmt.Errorf("%s is not a .vrb source file", input)
			}
			absolute, err := filepath.Abs(input)
			if err != nil {
				return nil, err
			}
			if !seen[absolute] {
				paths = append(paths, absolute)
				seen[absolute] = true
			}
			continue
		}
		err = filepath.WalkDir(input, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				name := entry.Name()
				if path != input && (strings.HasPrefix(name, ".") || name == "build" || name == "dist" || name == "vendor") {
					return filepath.SkipDir
				}
				return nil
			}
			if filepath.Ext(path) != ".vrb" {
				return nil
			}
			absolute, err := filepath.Abs(path)
			if err != nil {
				return err
			}
			if !seen[absolute] {
				paths = append(paths, absolute)
				seen[absolute] = true
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(paths)
	if len(paths) == 0 {
		return nil, fmt.Errorf("no .vrb source files found")
	}
	return paths, nil
}

func Load(inputs []string) (*Program, []diagnostic.Diagnostic, error) {
	paths, err := Discover(inputs)
	if err != nil {
		return nil, nil, err
	}
	program := &Program{Paths: paths}
	var diagnostics []diagnostic.Diagnostic
	for _, path := range paths {
		source, err := os.ReadFile(path)
		if err != nil {
			return nil, nil, err
		}
		file, items := parser.Parse(path, source)
		program.Files = append(program.Files, file)
		diagnostics = append(diagnostics, items...)
	}
	if !diagnostic.HasErrors(diagnostics) {
		diagnostics = append(diagnostics, check.Files(program.Files)...)
	}
	diagnostic.Sort(diagnostics)
	return program, diagnostics, nil
}

func Emit(program *Program) ([]byte, []diagnostic.Diagnostic) {
	generated, diagnostics := emitgo.Files(program.Files)
	diagnostic.Sort(diagnostics)
	return generated, diagnostics
}
