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
	"github.com/verba-lang/verba/internal/manifest"
	"github.com/verba-lang/verba/internal/parser"
	"github.com/verba-lang/verba/internal/source"
)

type Program struct {
	Paths    []string
	Root     string
	Manifest *manifest.Manifest
	Sources  []*source.File
	Files    []*ast.File
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
	program := &Program{Paths: paths, Root: commonDirectory(paths)}
	manager := source.NewManager()
	var diagnostics []diagnostic.Diagnostic
	manifestPath, err := manifest.Find(program.Root)
	if err != nil {
		return nil, nil, err
	}
	if manifestPath != "" {
		program.Manifest, diagnostics, err = manifest.Load(manifestPath)
		if err != nil {
			return nil, nil, err
		}
		program.Root = program.Manifest.Root
	}
	for _, path := range paths {
		fileSource, sourceDiagnostics, err := manager.Load(path)
		if err != nil {
			return nil, nil, err
		}
		diagnostics = append(diagnostics, sourceDiagnostics...)
		if diagnostic.HasErrors(sourceDiagnostics) {
			continue
		}
		file, items := parser.ParseFile(fileSource)
		program.Files = append(program.Files, file)
		diagnostics = append(diagnostics, items...)
	}
	program.Sources = manager.Files()
	if program.Manifest != nil && program.Manifest.Name != "" {
		for _, file := range program.Files {
			if file.Module != "" && file.Module != program.Manifest.Name {
				diagnostics = append(diagnostics, diagnostic.Diagnostic{
					Severity: diagnostic.Error,
					Code:     "VRB1003",
					File:     file.Path,
					Line:     1,
					Column:   1,
					Message:  fmt.Sprintf("module %s does not match project %s", file.Module, program.Manifest.Name),
					Hint:     fmt.Sprintf("change the declaration to module %s or update verba.toml", program.Manifest.Name),
				})
			}
		}
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

func (program *Program) Name() string {
	if program.Manifest != nil && program.Manifest.Name != "" {
		return program.Manifest.Name
	}
	if len(program.Files) != 0 && program.Files[0].Module != "" {
		return program.Files[0].Module
	}
	return "verba-program"
}

func commonDirectory(paths []string) string {
	if len(paths) == 0 {
		return "."
	}
	common := filepath.Dir(paths[0])
	for _, path := range paths[1:] {
		directory := filepath.Dir(path)
		for !containsPath(common, directory) {
			parent := filepath.Dir(common)
			if parent == common {
				return common
			}
			common = parent
		}
	}
	return common
}

func containsPath(parent, child string) bool {
	relative, err := filepath.Rel(parent, child)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
