package region

import (
	"fmt"
	"strings"

	"github.com/verba-lang/verba/internal/diagnostic"
	"github.com/verba-lang/verba/internal/source"
)

type Kind uint8

const (
	Core Kind = iota
	IslandContent
	IslandTerminator
)

type Span struct {
	Start int
	End   int
}

type Region struct {
	Kind Kind
	Span Span
}

type Island struct {
	Adapter        string
	Name           string
	Terminator     string
	Header         Span
	Content        Span
	TerminatorSpan Span
	HeaderLine     int
	TerminatorLine int
	Terminated     bool
}

type Result struct {
	Regions     []Region
	Islands     map[int]Island
	Diagnostics []diagnostic.Diagnostic
}

func Scan(file *source.File) Result {
	result := Result{Islands: map[int]Island{}}
	lines := file.Lines()
	coreStart := 0
	for index := 0; index < len(lines); index++ {
		line := lines[index]
		adapter, name, terminator, ok := embedHeader(file.LineText(line))
		if !ok {
			continue
		}
		headerEnd := lineEnd(lines, index, file.Len())
		if coreStart < headerEnd {
			result.Regions = append(result.Regions, Region{Kind: Core, Span: Span{Start: coreStart, End: headerEnd}})
		}
		island := Island{
			Adapter: adapter, Name: name, Terminator: terminator,
			Header: Span{Start: line.Start, End: line.End}, HeaderLine: line.Number,
		}
		contentStart := headerEnd
		terminatorIndex := -1
		for candidate := index + 1; candidate < len(lines); candidate++ {
			if file.LineText(lines[candidate]) == terminator {
				terminatorIndex = candidate
				break
			}
		}
		if terminatorIndex >= 0 {
			terminatorLine := lines[terminatorIndex]
			contentEnd := contentStart
			if terminatorIndex > index+1 {
				contentEnd = lines[terminatorIndex-1].End
			}
			island.Content = Span{Start: contentStart, End: contentEnd}
			island.TerminatorSpan = Span{Start: terminatorLine.Start, End: terminatorLine.End}
			island.TerminatorLine = terminatorLine.Number
			island.Terminated = true
			result.Regions = append(result.Regions,
				Region{Kind: IslandContent, Span: island.Content},
				Region{Kind: IslandTerminator, Span: island.TerminatorSpan},
			)
			coreStart = lineEnd(lines, terminatorIndex, file.Len())
			index = terminatorIndex
		} else {
			island.Content = Span{Start: contentStart, End: file.Len()}
			result.Regions = append(result.Regions, Region{Kind: IslandContent, Span: island.Content})
			result.Diagnostics = append(result.Diagnostics, diagnostic.Diagnostic{
				Severity: diagnostic.Error,
				Code:     "VRB0242",
				File:     file.Path,
				Line:     line.Number,
				Column:   1,
				Message:  fmt.Sprintf("embed %s is missing terminator %s", name, terminator),
				Hint:     "the terminator must appear exactly and alone on a line",
			})
			coreStart = file.Len()
			index = len(lines)
		}
		result.Islands[line.Number] = island
	}
	if coreStart < file.Len() {
		result.Regions = append(result.Regions, Region{Kind: Core, Span: Span{Start: coreStart, End: file.Len()}})
	}
	return result
}

func (result Result) IslandAtHeader(line int) (Island, bool) {
	island, ok := result.Islands[line]
	return island, ok
}

func (result Result) CoreSpans() []Span {
	var spans []Span
	for _, item := range result.Regions {
		if item.Kind == Core {
			spans = append(spans, item.Span)
		}
	}
	return spans
}

func embedHeader(raw string) (string, string, string, bool) {
	parts := strings.Fields(strings.TrimSpace(raw))
	if len(parts) != 5 || parts[0] != "embed" || parts[3] != "until" {
		return "", "", "", false
	}
	if !identifier(parts[1]) || !identifier(parts[2]) || !identifier(parts[4]) {
		return "", "", "", false
	}
	return parts[1], parts[2], parts[4], true
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

func lineEnd(lines []source.Line, index, fileLength int) int {
	if index+1 < len(lines) {
		return lines[index+1].Start
	}
	return fileLength
}
