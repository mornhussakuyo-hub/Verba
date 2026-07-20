package format

import (
	"bytes"
	"strings"

	"github.com/verba-lang/verba/internal/lexer"
	"github.com/verba-lang/verba/internal/region"
	"github.com/verba-lang/verba/internal/source"
)

func Source(content []byte) []byte {
	file, _ := source.New("<format>", content)
	regions := region.Scan(file)
	lines := file.Lines()
	var result bytes.Buffer
	indent := 0
	pendingBlank := false

	writeLine := func(value string) {
		if pendingBlank && result.Len() > 0 {
			result.WriteByte('\n')
		}
		pendingBlank = false
		result.WriteString(value)
		result.WriteByte('\n')
	}

	for index := 0; index < len(lines); index++ {
		lineInfo := lines[index]
		raw := file.LineText(lineInfo)
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			if result.Len() > 0 {
				pendingBlank = true
			}
			continue
		}

		fields := strings.Fields(trimmed)
		if len(fields) > 0 && fields[0] == "end" && indent > 0 {
			indent--
		}
		line := normalizeCoreLine(strings.TrimLeft(raw, " \t"))
		writeLine(strings.Repeat("    ", indent) + line)

		if island, ok := regions.IslandAtHeader(lineInfo.Number); ok {
			rawIsland := file.Slice(island.Content.Start, island.Content.End)
			if len(rawIsland) > 0 {
				result.Write(rawIsland)
				result.WriteByte('\n')
			}
			if island.Terminated {
				writeLine(island.Terminator)
				index = island.TerminatorLine - 1
			} else {
				return result.Bytes()
			}
			continue
		}
		if trimmed == "begin" {
			indent++
		}
	}
	return result.Bytes()
}

func normalizeCoreLine(line string) string {
	if prefix, literal, _, ok := lexer.SplitControlled(line); ok {
		result := strings.Join(strings.Fields(prefix), " ")
		if literal != "" {
			result += " " + literal
		}
		return result
	}
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return ""
	}
	return strings.Join(fields, " ")
}
