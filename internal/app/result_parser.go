package app

import (
	"path/filepath"
	"strings"
)

const solvedMarkerLine = "这道题目已经解出"
const finalReadableMarker = "Observation: final readable OpenCode output:"
const writeupFileMarker = "Observation: writeup file="

type parsedRunnerOutput struct {
	Solved          bool
	Flag            string
	WriteupFileName string
}

func parseRunnerOutput(logs string) parsedRunnerOutput {
	finalText := latestFinalReadableOutput(logs)
	parsed := parsedRunnerOutput{WriteupFileName: latestWriteupFileName(logs)}
	if finalText == "" {
		return parsed
	}
	lines := strings.Split(strings.ReplaceAll(finalText, "\r\n", "\n"), "\n")
	for index, line := range lines {
		if strings.TrimSpace(line) != solvedMarkerLine {
			continue
		}
		if index+1 >= len(lines) {
			return parsed
		}
		flag := strings.TrimRight(lines[index+1], "\r")
		if strings.TrimSpace(flag) == "" {
			return parsed
		}
		parsed.Solved = true
		parsed.Flag = flag
		return parsed
	}
	return parsed
}

func latestFinalReadableOutput(logs string) string {
	normalized := strings.ReplaceAll(logs, "\r\n", "\n")
	index := strings.LastIndex(normalized, finalReadableMarker)
	if index < 0 {
		return ""
	}
	return strings.TrimSpace(normalized[index+len(finalReadableMarker):])
}

func latestWriteupFileName(logs string) string {
	lines := strings.Split(strings.ReplaceAll(logs, "\r\n", "\n"), "\n")
	for index := len(lines) - 1; index >= 0; index-- {
		line := strings.TrimSpace(lines[index])
		if !strings.HasPrefix(line, writeupFileMarker) {
			continue
		}
		name := strings.TrimSpace(strings.TrimPrefix(line, writeupFileMarker))
		if isSafeStoredFilename(name) {
			return name
		}
	}
	return ""
}

func writeupFilenameForTask(task *Task) string {
	if task == nil {
		return "task-wp.md"
	}
	base := safeFilenameStem(task.Name)
	if base == "" {
		base = safeFilenameStem(task.ID)
	}
	if base == "" {
		base = "task"
	}
	return base + "-wp.md"
}

func safeFilenameStem(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var builder strings.Builder
	lastDash := false
	for _, char := range value {
		switch {
		case char >= 'a' && char <= 'z',
			char >= 'A' && char <= 'Z',
			char >= '0' && char <= '9',
			char >= 0x4e00 && char <= 0x9fff:
			builder.WriteRune(char)
			lastDash = false
		case char == '-' || char == '_' || char == '.':
			builder.WriteRune(char)
			lastDash = false
		default:
			if !lastDash {
				builder.WriteByte('-')
				lastDash = true
			}
		}
	}
	stem := strings.Trim(builder.String(), "-_. ")
	if stem == "" || filepath.Base(stem) != stem || strings.ContainsAny(stem, `/\`) {
		return ""
	}
	if len([]rune(stem)) > 80 {
		runes := []rune(stem)
		stem = string(runes[:80])
		stem = strings.Trim(stem, "-_. ")
	}
	return stem
}
