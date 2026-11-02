package markdown

import (
	"strconv"
	"strings"
)

// HeadingNumberingProperty stores the per-page automatic heading-numbering setting.
const HeadingNumberingProperty = "kumbuka.heading-numbering"

// HeadingNumberingEnabled reports whether a page's saved properties enable automatic heading numbering.
func HeadingNumberingEnabled(properties map[string]string) bool {
	return properties[HeadingNumberingProperty] == "true"
}

// RenumberHeadings rewrites ATX headings with deterministic outline numbers. It leaves fenced code and all non-heading source unchanged.
func RenumberHeadings(source string) string {
	lines := strings.SplitAfter(source, "\n")
	counters := [6]int{}
	inFence := ""

	for index, line := range lines {
		body, ending := strings.CutSuffix(line, "\n")
		trimmed := strings.TrimLeft(body, " \t")
		if fence := fenceMarker(trimmed); fence != "" {
			if inFence == "" {
				inFence = fence
			} else if strings.HasPrefix(trimmed, inFence) {
				inFence = ""
			}
			continue
		}
		if inFence != "" {
			continue
		}

		indentLength := len(body) - len(trimmed)
		level := 0
		for level < len(trimmed) && trimmed[level] == '#' {
			level++
		}
		if level == 0 || level > 6 || level == len(trimmed) || (trimmed[level] != ' ' && trimmed[level] != '\t') {
			continue
		}
		title := strings.TrimSpace(trimmed[level:])
		if title == "" {
			continue
		}
		title = strings.TrimSpace(stripGeneratedHeadingNumber(title))
		counters[level-1]++
		for deeper := level; deeper < len(counters); deeper++ {
			counters[deeper] = 0
		}
		parts := make([]string, 0, level)
		for _, counter := range counters[:level] {
			if counter != 0 {
				parts = append(parts, strconv.Itoa(counter))
			}
		}
		prefix := strings.Join(parts, ".")
		if len(parts) == 1 {
			prefix += "."
		}
		lines[index] = body[:indentLength] + strings.Repeat("#", level) + " " + prefix + " " + title
		if ending {
			lines[index] += "\n"
		}
	}

	return strings.Join(lines, "")
}

// fenceMarker returns the opening marker for a fenced code block line.
func fenceMarker(line string) string {
	if len(line) < 3 {
		return ""
	}
	marker := line[0]
	if marker != '`' && marker != '~' {
		return ""
	}
	count := 0
	for count < len(line) && line[count] == marker {
		count++
	}
	if count < 3 {
		return ""
	}
	return strings.Repeat(string(marker), count)
}

// stripGeneratedHeadingNumber removes a leading outline number emitted by RenumberHeadings.
func stripGeneratedHeadingNumber(title string) string {
	index := 0
	hasDigit := false
	for index < len(title) {
		if title[index] >= '0' && title[index] <= '9' {
			hasDigit = true
			index++
			continue
		}
		if title[index] == '.' {
			index++
			continue
		}
		break
	}
	if !hasDigit || index == 0 || index == len(title) || (title[index] != ' ' && title[index] != '\t') {
		return title
	}
	return strings.TrimLeft(title[index:], " \t")
}
