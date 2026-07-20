package cli

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/verba-lang/verba/internal/compiler"
	"github.com/verba-lang/verba/internal/diagnostic"
	verbaformat "github.com/verba-lang/verba/internal/format"
	"github.com/verba-lang/verba/internal/source"
)

const Version = "0.1.0"

type CLI struct {
	Stdout io.Writer
	Stderr io.Writer
	Stdin  io.Reader
}

func New() *CLI {
	return &CLI{Stdout: os.Stdout, Stderr: os.Stderr, Stdin: os.Stdin}
}

func (c *CLI) Run(args []string) int {
	if len(args) == 0 {
		c.help()
		return 0
	}
	switch args[0] {
	case "help", "-h", "--help":
		c.help()
		return 0
	case "version", "-v", "--version":
		fmt.Fprintf(c.Stdout, "verba %s\n", Version)
		return 0
	case "check":
		return c.check(args[1:])
	case "fmt":
		return c.format(args[1:])
	case "build":
		return c.build(args[1:])
	case "run":
		return c.run(args[1:])
	default:
		fmt.Fprintf(c.Stderr, "verba: unknown command %q\n\n", args[0])
		c.help()
		return 2
	}
}

func (c *CLI) help() {
	fmt.Fprintln(c.Stdout, `Verba is the compiler and developer tool for the Verba language.

Usage:
  verba <command> [options] [files or directories]

Commands:
  check     Parse and statically check Verba source
  fmt       Format Verba source files in place
  build     Generate Go and build an executable
  run       Build and run a Verba program
  version   Print the installed version
  help      Show this help

Examples:
  verba check .
  verba fmt --check .
  verba build -o build/server.exe examples/hello
  verba run examples/hello

Environment:
  VERBA_ADDRESS   Address used by generated HTTP servers (default :8080)`)
}

func (c *CLI) check(args []string) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	program, diagnostics, err := compiler.Load(fs.Args())
	if err != nil {
		return c.failure(err)
	}
	if c.printDiagnostics(diagnostics) {
		return 1
	}
	fmt.Fprintf(c.Stdout, "checked %d file(s): no errors\n", len(program.Files))
	return 0
}

func (c *CLI) format(args []string) int {
	fs := flag.NewFlagSet("fmt", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	checkOnly := fs.Bool("check", false, "report files that are not formatted without changing them")
	stdout := fs.Bool("stdout", false, "write one formatted source file to standard output")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	paths, err := compiler.Discover(fs.Args())
	if err != nil {
		return c.failure(err)
	}
	if *stdout && len(paths) != 1 {
		return c.failure(fmt.Errorf("fmt --stdout requires exactly one source file"))
	}
	changed := 0
	for _, path := range paths {
		fileSource, sourceDiagnostics, err := source.Load(path)
		if err != nil {
			return c.failure(err)
		}
		if c.printDiagnostics(sourceDiagnostics) {
			return 1
		}
		content := fileSource.Bytes()
		formatted := verbaformat.Source(content)
		if *stdout {
			_, _ = c.Stdout.Write(formatted)
			continue
		}
		if bytes.Equal(content, formatted) {
			continue
		}
		changed++
		if *checkOnly {
			fmt.Fprintln(c.Stdout, displayPath(path))
			continue
		}
		info, err := os.Stat(path)
		if err != nil {
			return c.failure(err)
		}
		if err := os.WriteFile(path, formatted, info.Mode()); err != nil {
			return c.failure(err)
		}
		fmt.Fprintln(c.Stdout, displayPath(path))
	}
	if *checkOnly && changed > 0 {
		fmt.Fprintf(c.Stderr, "%d file(s) need formatting\n", changed)
		return 1
	}
	if changed == 0 && !*stdout {
		fmt.Fprintln(c.Stdout, "all files are formatted")
	}
	return 0
}

func (c *CLI) build(args []string) int {
	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	output := fs.String("o", "", "output executable path")
	emit := fs.String("emit-go", "", "also write generated Go source to this path")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	program, diagnostics, err := compiler.Load(fs.Args())
	if err != nil {
		return c.failure(err)
	}
	if c.printDiagnostics(diagnostics) {
		return 1
	}
	generated, emitDiagnostics := compiler.Emit(program)
	if c.printDiagnostics(emitDiagnostics) {
		return 1
	}
	if *emit != "" {
		if err := writeFile(*emit, generated, 0o644); err != nil {
			return c.failure(err)
		}
	}
	target := *output
	if target == "" {
		name := program.Name()
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		target = filepath.Join("build", name)
	}
	if err := c.compileGo(generated, target); err != nil {
		return c.failure(err)
	}
	absolute, _ := filepath.Abs(target)
	fmt.Fprintf(c.Stdout, "built %s\n", absolute)
	return 0
}

func (c *CLI) run(args []string) int {
	separator := len(args)
	for i, arg := range args {
		if arg == "--" {
			separator = i
			break
		}
	}
	inputs := args[:separator]
	var programArgs []string
	if separator < len(args) {
		programArgs = args[separator+1:]
	}
	program, diagnostics, err := compiler.Load(inputs)
	if err != nil {
		return c.failure(err)
	}
	if c.printDiagnostics(diagnostics) {
		return 1
	}
	generated, emitDiagnostics := compiler.Emit(program)
	if c.printDiagnostics(emitDiagnostics) {
		return 1
	}
	temp, err := os.MkdirTemp("", "verba-run-")
	if err != nil {
		return c.failure(err)
	}
	defer os.RemoveAll(temp)
	executable := filepath.Join(temp, "program")
	if runtime.GOOS == "windows" {
		executable += ".exe"
	}
	if err := c.compileGo(generated, executable); err != nil {
		return c.failure(err)
	}
	command := exec.Command(executable, programArgs...)
	command.Stdin, command.Stdout, command.Stderr = c.Stdin, c.Stdout, c.Stderr
	if err := command.Run(); err != nil {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			return exitError.ExitCode()
		}
		return c.failure(err)
	}
	return 0
}

func (c *CLI) compileGo(source []byte, target string) error {
	temp, err := os.MkdirTemp("", "verba-build-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temp)
	mainPath := filepath.Join(temp, "main.go")
	if err := os.WriteFile(mainPath, source, 0o644); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	absolute, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	command := exec.Command("go", "build", "-trimpath", "-buildvcs=false", "-ldflags=-s -w", "-o", absolute, mainPath)
	command.Stdout, command.Stderr = c.Stdout, c.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("Go backend failed: %w", err)
	}
	return nil
}

func (c *CLI) printDiagnostics(items []diagnostic.Diagnostic) bool {
	for _, item := range items {
		fmt.Fprintln(c.Stderr, item.String())
	}
	return diagnostic.HasErrors(items)
}

func (c *CLI) failure(err error) int {
	fmt.Fprintf(c.Stderr, "verba: %v\n", err)
	return 1
}

func writeFile(path string, content []byte, mode os.FileMode) error {
	if directory := filepath.Dir(path); directory != "." {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, content, mode)
}

func displayPath(path string) string {
	if relative, err := filepath.Rel(".", path); err == nil && !strings.HasPrefix(relative, "..") {
		return relative
	}
	return path
}
