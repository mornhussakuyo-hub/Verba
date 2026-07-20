package diagnostic

import (
	"fmt"
	"sort"
)

type Severity string

const (
	Error   Severity = "error"
	Warning Severity = "warning"
)

type Diagnostic struct {
	Severity Severity
	Code     string
	File     string
	Line     int
	Column   int
	Message  string
	Hint     string
}

func (d Diagnostic) String() string {
	location := d.File
	if d.Line > 0 {
		location = fmt.Sprintf("%s:%d:%d", d.File, d.Line, max(d.Column, 1))
	}
	result := fmt.Sprintf("%s: %s %s: %s", location, d.Severity, d.Code, d.Message)
	if d.Hint != "" {
		result += "\n  hint: " + d.Hint
	}
	return result
}

func Sort(items []Diagnostic) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].File != items[j].File {
			return items[i].File < items[j].File
		}
		if items[i].Line != items[j].Line {
			return items[i].Line < items[j].Line
		}
		return items[i].Column < items[j].Column
	})
}

func HasErrors(items []Diagnostic) bool {
	for _, item := range items {
		if item.Severity == Error {
			return true
		}
	}
	return false
}
