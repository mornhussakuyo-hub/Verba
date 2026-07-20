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
		left, right := items[i], items[j]
		if left.File != right.File {
			return left.File < right.File
		}
		if left.Line != right.Line {
			return left.Line < right.Line
		}
		if left.Column != right.Column {
			return left.Column < right.Column
		}
		if left.Code != right.Code {
			return left.Code < right.Code
		}
		if left.Message != right.Message {
			return left.Message < right.Message
		}
		if left.Hint != right.Hint {
			return left.Hint < right.Hint
		}
		return left.Severity < right.Severity
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
