package markdown

import (
	"regexp"
	"strconv"
	"strings"
)

// HeadingNumberingProperty stores the per-page automatic heading-numbering setting.
const HeadingNumberingProperty = "kumbuka.heading-numbering"

// HeadingNumberingEnabled reports whether a page's saved properties enable automatic heading numbering.
func HeadingNumberingEnabled(properties map[string]string) bool {
	switch properties[HeadingNumberingProperty] {
	case "true", "roman", "roman-lower":
		return true
	default:
		return false
	}
}

// RenumberHeadings rewrites ATX headings with deterministic outline numbers. It leaves fenced code and all non-heading source unchanged.
func RenumberHeadings(source string) string {
	return RenumberHeadingsWithFormat(source, "true")
}

// RenumberHeadingsWithFormat applies decimal or Roman outline numbers to Markdown headings.
func RenumberHeadingsWithFormat(source, format string) string {
	prefixPattern := generatedHeadingNumber
	if format == "roman" || format == "roman-lower" {
		prefixPattern = romanHeadingNumber
	}
	lines := strings.SplitAfter(source, "\n")
	counters := [6]int{}
	inFence := ""

	for index, line := range lines {
		body := strings.TrimSuffix(line, "\n")
		body = strings.TrimSuffix(body, "\r")
		ending := line[len(body):]
		trimmed := strings.TrimLeft(body, " ")
		indentLength := len(body) - len(trimmed)
		if indentLength > 3 {
			continue
		}
		marker := fenceMarker(trimmed)
		if inFence != "" {
			if closesNumberingFence(trimmed, marker, inFence) {
				inFence = ""
			}
			continue
		}
		if opensNumberingFence(trimmed, marker) {
			inFence = marker
			continue
		}

		level := headingLevel(trimmed)
		if level == 0 {
			continue
		}
		title := strings.TrimSpace(trimmed[level:])
		if title == "" {
			continue
		}
		title = strings.TrimSpace(prefixPattern.ReplaceAllString(title, ""))
		prefix := nextHeadingNumber(&counters, level, format)
		lines[index] = body[:indentLength] + strings.Repeat("#", level) + " " + prefix + " " + title + ending
	}

	return strings.Join(lines, "")
}

// nextHeadingNumber advances the outline at level and clears its descendant counters.
func nextHeadingNumber(counters *[6]int, level int, format string) string {
	counters[level-1]++
	for deeper := level; deeper < len(counters); deeper++ {
		counters[deeper] = 0
	}
	parts := make([]string, 0, level)
	for _, counter := range counters[:level] {
		if counter != 0 {
			parts = append(parts, formatHeadingCounter(counter, format))
		}
	}
	prefix := strings.Join(parts, ".")
	if len(parts) == 1 {
		prefix += "."
	}
	return prefix
}

// opensNumberingFence reports whether the marker and its info string start a fenced block.
func opensNumberingFence(line, marker string) bool {
	if marker == "" {
		return false
	}
	return marker[0] == '~' || !strings.Contains(line[len(marker):], "`")
}

// closesNumberingFence accepts a matching marker followed only by optional indentation whitespace.
func closesNumberingFence(line, marker, opening string) bool {
	return strings.HasPrefix(marker, opening) && strings.Trim(line[len(marker):], " \t") == ""
}

// headingLevel returns the ATX heading depth, or zero for ordinary text.
func headingLevel(line string) int {
	hashes := len(line) - len(strings.TrimLeft(line, "#"))
	if hashes < 1 || hashes > 6 || hashes == len(line) {
		return 0
	}
	if line[hashes] != ' ' && line[hashes] != '\t' {
		return 0
	}
	return hashes
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

// generatedHeadingNumber matches the single-level and nested prefixes emitted by RenumberHeadings.
var generatedHeadingNumber = regexp.MustCompile(`^[0-9]+(?:\.|(?:\.[0-9]+)+)[ \t]+`)

// romanHeadingNumber matches Roman outline prefixes in pages using Roman numbering.
var romanHeadingNumber = regexp.MustCompile(`(?i)^[ivxlcdm]+(?:\.|(?:\.[ivxlcdm]+)+)[ \t]+`)

// formatHeadingCounter renders one outline component in the selected numeral style.
func formatHeadingCounter(value int, format string) string {
	if format != "roman" && format != "roman-lower" {
		return strconv.Itoa(value)
	}
	values := [...]int{1000, 900, 500, 400, 100, 90, 50, 40, 10, 9, 5, 4, 1}
	symbols := [...]string{"M", "CM", "D", "CD", "C", "XC", "L", "XL", "X", "IX", "V", "IV", "I"}
	var result strings.Builder
	for i, unit := range values {
		result.WriteString(strings.Repeat(symbols[i], value/unit))
		value %= unit
	}
	if format == "roman-lower" {
		return strings.ToLower(result.String())
	}
	return result.String()
}
