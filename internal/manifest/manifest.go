package manifest

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/verba-lang/verba/internal/diagnostic"
	"github.com/verba-lang/verba/internal/source"
)

const Filename = "verba.toml"

var identifierPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
var versionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`)

type Database struct {
	Dialect    string `toml:"dialect"`
	Schema     string `toml:"schema"`
	SchemaPath string `toml:"-"`
}

type Manifest struct {
	Name         string            `toml:"name"`
	Version      string            `toml:"version"`
	Target       string            `toml:"target"`
	Database     *Database         `toml:"database"`
	Dependencies map[string]string `toml:"dependencies"`
	Path         string            `toml:"-"`
	Root         string            `toml:"-"`
}

func Find(start string) (string, error) {
	absolute, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(absolute)
	if err == nil && !info.IsDir() {
		absolute = filepath.Dir(absolute)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	for {
		candidate := filepath.Join(absolute, Filename)
		info, statErr := os.Stat(candidate)
		if statErr == nil {
			if info.IsDir() {
				return "", fmt.Errorf("%s is a directory", candidate)
			}
			return candidate, nil
		}
		if !errors.Is(statErr, os.ErrNotExist) {
			return "", statErr
		}
		parent := filepath.Dir(absolute)
		if parent == absolute {
			return "", nil
		}
		absolute = parent
	}
}

func Load(path string) (*Manifest, []diagnostic.Diagnostic, error) {
	file, diagnostics, err := source.Load(path)
	if err != nil {
		return nil, nil, err
	}
	result := &Manifest{Path: file.Path, Root: filepath.Dir(file.Path)}
	if diagnostic.HasErrors(diagnostics) {
		return result, diagnostics, nil
	}
	decoder := toml.NewDecoder(bytes.NewReader(file.Bytes())).DisallowUnknownFields()
	if err := decoder.Decode(result); err != nil {
		line, column := 1, 1
		var decodeError *toml.DecodeError
		if errors.As(err, &decodeError) {
			line, column = decodeError.Position()
		}
		diagnostics = append(diagnostics, item(file.Path, line, column, "VRB0901", fmt.Sprintf("invalid project manifest: %v", err), "fix the TOML syntax or remove unsupported fields"))
		return result, diagnostics, nil
	}
	diagnostics = append(diagnostics, validate(result)...)
	return result, diagnostics, nil
}

func validate(value *Manifest) []diagnostic.Diagnostic {
	var diagnostics []diagnostic.Diagnostic
	if value.Name == "" {
		diagnostics = append(diagnostics, item(value.Path, 1, 1, "VRB0902", "project manifest requires a name", `add name = "your_module"`))
	} else if !identifierPattern.MatchString(value.Name) {
		diagnostics = append(diagnostics, item(value.Path, 1, 1, "VRB0903", fmt.Sprintf("invalid project name %q", value.Name), "use a Verba identifier containing letters, digits, and underscores"))
	}
	if value.Version == "" {
		diagnostics = append(diagnostics, item(value.Path, 1, 1, "VRB0904", "project manifest requires a semantic version", `add version = "0.1.0"`))
	} else if !versionPattern.MatchString(value.Version) {
		diagnostics = append(diagnostics, item(value.Path, 1, 1, "VRB0904", fmt.Sprintf("invalid semantic version %q", value.Version), "use a full version such as 0.1.0"))
	}
	if value.Target == "" {
		diagnostics = append(diagnostics, item(value.Path, 1, 1, "VRB0905", "project manifest requires a target", `add target = "go"`))
	} else if value.Target != "go" {
		diagnostics = append(diagnostics, item(value.Path, 1, 1, "VRB0905", fmt.Sprintf("unsupported compilation target %q", value.Target), "the current compiler supports target = \"go\""))
	}
	if value.Database != nil {
		if value.Database.Dialect != "postgres" {
			diagnostics = append(diagnostics, item(value.Path, 1, 1, "VRB0906", fmt.Sprintf("unsupported database dialect %q", value.Database.Dialect), "the first SQL adapter requires dialect = \"postgres\""))
		}
		if value.Database.Schema == "" {
			diagnostics = append(diagnostics, item(value.Path, 1, 1, "VRB0907", "database configuration requires a schema snapshot", `set schema to a project-relative SQL file`))
		} else if schema, ok := projectPath(value.Root, value.Database.Schema); !ok {
			diagnostics = append(diagnostics, item(value.Path, 1, 1, "VRB0907", "database schema must stay inside the project directory", "use a relative path without escaping the project root"))
		} else if filepath.Ext(schema) != ".sql" {
			diagnostics = append(diagnostics, item(value.Path, 1, 1, "VRB0907", fmt.Sprintf("database schema %q is not a SQL file", value.Database.Schema), "use a schema snapshot with the .sql extension"))
		} else if info, err := os.Stat(schema); err != nil || info.IsDir() {
			diagnostics = append(diagnostics, item(value.Path, 1, 1, "VRB0907", fmt.Sprintf("database schema %q does not exist", value.Database.Schema), "create the schema snapshot or correct its path"))
		} else if !resolvedInside(value.Root, schema) {
			diagnostics = append(diagnostics, item(value.Path, 1, 1, "VRB0907", "database schema symlink resolves outside the project directory", "store the schema snapshot inside the project instead of linking outside it"))
		} else {
			value.Database.SchemaPath = schema
		}
	}
	for name, constraint := range value.Dependencies {
		if !identifierPattern.MatchString(name) {
			diagnostics = append(diagnostics, item(value.Path, 1, 1, "VRB0908", fmt.Sprintf("invalid dependency name %q", name), "dependency names must be Verba identifiers"))
		}
		if strings.TrimSpace(constraint) == "" {
			diagnostics = append(diagnostics, item(value.Path, 1, 1, "VRB0908", fmt.Sprintf("dependency %s has an empty version constraint", name), "provide a version or version constraint"))
		}
	}
	diagnostic.Sort(diagnostics)
	return diagnostics
}

func projectPath(root, relative string) (string, bool) {
	if filepath.IsAbs(relative) {
		return "", false
	}
	path := filepath.Clean(filepath.Join(root, relative))
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return path, true
}

func resolvedInside(root, path string) bool {
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return false
	}
	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return false
	}
	relative, err := filepath.Rel(resolvedRoot, resolvedPath)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func item(path string, line, column int, code, message, hint string) diagnostic.Diagnostic {
	return diagnostic.Diagnostic{Severity: diagnostic.Error, Code: code, File: path, Line: line, Column: column, Message: message, Hint: hint}
}
