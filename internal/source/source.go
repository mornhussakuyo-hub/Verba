package source

import (
	"os"
	"path/filepath"
	"sort"
	"unicode/utf8"

	"github.com/verba-lang/verba/internal/diagnostic"
)

type Location struct {
	Offset int
	Line   int
	Column int
}

type Line struct {
	Number int
	Start  int
	End    int
}

type File struct {
	ID      int
	Path    string
	content []byte
	lines   []Line
}

type Manager struct {
	files  []*File
	byPath map[string]*File
}

func NewManager() *Manager {
	return &Manager{byPath: map[string]*File{}}
}

func New(path string, content []byte) (*File, []diagnostic.Diagnostic) {
	file := &File{Path: path, content: append([]byte(nil), content...)}
	file.indexLines()

	var diagnostics []diagnostic.Diagnostic
	if len(content) >= 3 && content[0] == 0xef && content[1] == 0xbb && content[2] == 0xbf {
		diagnostics = append(diagnostics, diagnostic.Diagnostic{
			Severity: diagnostic.Error,
			Code:     "VRB0002",
			File:     path,
			Line:     1,
			Column:   1,
			Message:  "Verba source files must not contain a UTF-8 byte order mark",
			Hint:     "save the file as UTF-8 without BOM",
		})
	}
	if !utf8.Valid(content) {
		offset := firstInvalidUTF8(content)
		location := file.Position(offset)
		diagnostics = append(diagnostics, diagnostic.Diagnostic{
			Severity: diagnostic.Error,
			Code:     "VRB0001",
			File:     path,
			Line:     location.Line,
			Column:   location.Column,
			Message:  "Verba source files must contain valid UTF-8",
			Hint:     "convert the file to UTF-8 and replace the invalid byte sequence",
		})
	}
	return file, diagnostics
}

func Load(path string) (*File, []diagnostic.Diagnostic, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, nil, err
	}
	file, diagnostics := New(filepath.Clean(absolute), content)
	return file, diagnostics, nil
}

func (manager *Manager) Load(path string) (*File, []diagnostic.Diagnostic, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, nil, err
	}
	absolute = filepath.Clean(absolute)
	if file := manager.byPath[absolute]; file != nil {
		return file, nil, nil
	}
	file, diagnostics, err := Load(absolute)
	if err != nil {
		return nil, nil, err
	}
	file.ID = len(manager.files) + 1
	manager.files = append(manager.files, file)
	manager.byPath[absolute] = file
	return file, diagnostics, nil
}

func (manager *Manager) Files() []*File {
	return append([]*File(nil), manager.files...)
}

func (file *File) Lines() []Line {
	return append([]Line(nil), file.lines...)
}

func (file *File) Bytes() []byte {
	return append([]byte(nil), file.content...)
}

func (file *File) Len() int {
	return len(file.content)
}

func (file *File) LineText(line Line) string {
	if line.Start < 0 || line.End < line.Start || line.End > len(file.content) {
		return ""
	}
	return string(file.content[line.Start:line.End])
}

func (file *File) Position(offset int) Location {
	offset = min(max(offset, 0), len(file.content))
	index := sort.Search(len(file.lines), func(index int) bool {
		return file.lines[index].Start > offset
	}) - 1
	if index < 0 {
		index = 0
	}
	line := file.lines[index]
	columnBytes := file.content[line.Start:offset]
	column := utf8.RuneCount(columnBytes) + 1
	return Location{Offset: offset, Line: line.Number, Column: column}
}

func (file *File) indexLines() {
	start := 0
	number := 1
	for index := 0; index < len(file.content); index++ {
		switch file.content[index] {
		case '\n':
			file.lines = append(file.lines, Line{Number: number, Start: start, End: index})
			start = index + 1
			number++
		case '\r':
			file.lines = append(file.lines, Line{Number: number, Start: start, End: index})
			if index+1 < len(file.content) && file.content[index+1] == '\n' {
				index++
			}
			start = index + 1
			number++
		}
	}
	file.lines = append(file.lines, Line{Number: number, Start: start, End: len(file.content)})
}

func firstInvalidUTF8(content []byte) int {
	for offset := 0; offset < len(content); {
		_, size := utf8.DecodeRune(content[offset:])
		if size == 1 && content[offset] >= utf8.RuneSelf {
			return offset
		}
		offset += size
	}
	return 0
}
