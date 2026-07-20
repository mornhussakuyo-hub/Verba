package format

import (
	"strings"
)

func Source(source []byte) []byte {
	text := strings.ReplaceAll(string(source), "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	lines := strings.Split(text, "\n")
	result := make([]string, 0, len(lines))
	indent := 0
	inIsland := false
	terminator := ""
	lastBlank := false

	for _, raw := range lines {
		trimmed := strings.TrimSpace(raw)
		if inIsland {
			if trimmed == terminator && strings.TrimSpace(raw) == terminator {
				result = append(result, terminator)
				inIsland = false
				terminator = ""
				lastBlank = false
			} else {
				result = append(result, strings.TrimRight(raw, " \t"))
			}
			continue
		}
		if trimmed == "" {
			if len(result) > 0 && !lastBlank {
				result = append(result, "")
				lastBlank = true
			}
			continue
		}

		fields := strings.Fields(trimmed)
		if len(fields) > 0 && fields[0] == "end" && indent > 0 {
			indent--
		}
		line := normalizeCoreLine(trimmed)
		result = append(result, strings.Repeat("    ", indent)+line)
		lastBlank = false

		if len(fields) > 0 && fields[0] == "embed" && len(fields) == 5 && fields[3] == "until" {
			inIsland = true
			terminator = fields[4]
			continue
		}
		if trimmed == "begin" {
			indent++
		}
	}
	for len(result) > 0 && result[len(result)-1] == "" {
		result = result[:len(result)-1]
	}
	return []byte(strings.Join(result, "\n") + "\n")
}

func normalizeCoreLine(line string) string {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return ""
	}
	switch fields[0] {
	case "note":
		if len(fields) == 1 {
			return "note"
		}
		return "note " + strings.TrimSpace(strings.TrimPrefix(line, "note"))
	case "path":
		return "path " + strings.TrimSpace(strings.TrimPrefix(line, "path"))
	}
	return strings.Join(fields, " ")
}
