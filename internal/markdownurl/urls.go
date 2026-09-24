// Package markdownurl locates resource URL destinations without rewriting Markdown source syntax.
package markdownurl

import "strings"

// Range identifies one URL destination by byte offsets in its Markdown source.
type Range struct {
	// Start is the first byte of the URL.
	Start int
	// End is the byte immediately after the URL.
	End int
}

// Ranges returns the destinations of Markdown links and supported HTML attributes outside code.
func Ranges(source string) []Range {
	locations := markdownResourceURLRanges(source)
	result := make([]Range, len(locations))
	for index, location := range locations {
		result[index] = Range{Start: location.start, End: location.end}
	}
	return result
}

// Rewrite replaces recognized destinations while preserving other Markdown source bytes.
func Rewrite(source string, replace func(string) (string, bool, error)) (string, error) {
	return rewriteMarkdownResourceURLs(source, replace)
}

// markdownURLRange identifies a link destination without including its Markdown delimiters.
type markdownURLRange struct {
	// start is the first byte of the URL in the original source.
	start int
	// end is the byte immediately after the URL in the original source.
	end int
}

// markdownURLScanner preserves code and fence state while locating resource destinations.
type markdownURLScanner struct {
	// fence is the marker used by the current fenced code block.
	fence byte
	// fenceWidth is the number of markers required to close that block.
	fenceWidth int
	// codeWidth is the length of the current inline-code delimiter.
	codeWidth int
	// inComment reports whether an HTML comment continues onto this line.
	inComment bool
	// inHTMLCode reports whether a raw HTML code or pre element is open.
	inHTMLCode bool
	// ranges contains the destinations discovered in source order.
	ranges []markdownURLRange
}

// rewriteMarkdownResourceURLs changes recognized destinations while preserving all other source bytes.
func rewriteMarkdownResourceURLs(source string, replace func(string) (string, bool, error)) (string, error) {
	ranges := markdownResourceURLRanges(source)
	var output strings.Builder
	previous := 0

	for _, location := range ranges {
		url := source[location.start:location.end]
		replacement, changed, err := replace(url)
		if err != nil {
			return "", err
		}
		if !changed {
			continue
		}
		output.WriteString(source[previous:location.start])
		output.WriteString(replacement)
		previous = location.end
	}
	output.WriteString(source[previous:])
	return output.String(), nil
}

// markdownResourceURLRanges finds inline links, reference definitions, and HTML media attributes outside code.
func markdownResourceURLRanges(source string) []markdownURLRange {
	scanner := markdownURLScanner{}
	for start := 0; start < len(source); {
		end := strings.IndexByte(source[start:], '\n')
		if end < 0 {
			end = len(source)
		} else {
			end += start
		}
		scanner.scanLine(source[start:end], start)
		start = end + 1
	}
	return scanner.ranges
}

// scanLine ignores fenced and indented code before scanning one ordinary Markdown line.
func (s *markdownURLScanner) scanLine(line string, offset int) {
	marker, width, after := markdownFence(line)
	if s.fenceWidth != 0 {
		if marker == s.fence && width >= s.fenceWidth && strings.TrimSpace(line[after:]) == "" {
			s.fenceWidth = 0
		}
		return
	}
	if width != 0 && s.codeWidth == 0 && !s.inComment && !s.inHTMLCode {
		s.fence, s.fenceWidth = marker, width
		return
	}
	if strings.HasPrefix(line, "\t") || strings.HasPrefix(line, "    ") {
		return
	}

	if s.scanReferenceDefinition(line, offset) {
		return
	}
	for index := 0; index < len(line); {
		if s.inComment {
			end := strings.Index(line[index:], "-->")
			if end < 0 {
				return
			}
			s.inComment = false
			index += end + len("-->")
			continue
		}
		if strings.HasPrefix(line[index:], "<!--") && s.codeWidth == 0 {
			s.inComment = true
			index += len("<!--")
			continue
		}
		if line[index] == '`' {
			width := markerWidth(line[index:], '`')
			switch s.codeWidth {
			case 0:
				s.codeWidth = width
			case width:
				s.codeWidth = 0
			}
			index += width
			continue
		}
		if s.codeWidth != 0 {
			index++
			continue
		}
		if line[index] == '<' {
			if end, ok := s.scanHTMLTag(line, offset, index); ok {
				index = end
				continue
			}
		}
		if s.inHTMLCode {
			index++
			continue
		}
		if line[index] == ']' && index+1 < len(line) && line[index+1] == '(' &&
			strings.LastIndexByte(line[:index], '[') >= 0 && !isEscapedMarkdownByte(line, index) {
			if location, ok := markdownDestination(line, offset, index+2); ok {
				s.ranges = append(s.ranges, location)
				index = location.end - offset
				continue
			}
		}
		index++
	}
}

// markdownFence identifies a backtick or tilde fence indented by at most three spaces.
func markdownFence(line string) (byte, int, int) {
	indent := len(line) - len(strings.TrimLeft(line, " "))
	if indent > 3 || indent == len(line) {
		return 0, 0, 0
	}
	marker := line[indent]
	if marker != '`' && marker != '~' {
		return 0, 0, 0
	}
	width := markerWidth(line[indent:], marker)
	if width < 3 {
		return 0, 0, 0
	}
	return marker, width, indent + width
}

// markerWidth counts a contiguous delimiter run at the beginning of value.
func markerWidth(value string, marker byte) int {
	width := 0
	for width < len(value) && value[width] == marker {
		width++
	}
	return width
}

// scanReferenceDefinition captures a leading link definition and reports whether the line was consumed.
func (s *markdownURLScanner) scanReferenceDefinition(line string, offset int) bool {
	if s.codeWidth != 0 || s.inComment || s.inHTMLCode {
		return false
	}
	indent := len(line) - len(strings.TrimLeft(line, " "))
	if indent > 3 || indent >= len(line) || line[indent] != '[' {
		return false
	}
	closing := strings.Index(line[indent+1:], "]:")
	if closing < 1 {
		return false
	}
	if location, ok := markdownDestination(line, offset, indent+1+closing+2); ok {
		s.ranges = append(s.ranges, location)
		return true
	}
	return false
}

// isEscapedMarkdownByte reports whether an odd number of backslashes escape a delimiter.
func isEscapedMarkdownByte(line string, index int) bool {
	backslashes := 0
	for index > 0 && line[index-1] == '\\' {
		index--
		backslashes++
	}
	return backslashes%2 != 0
}

// markdownDestination reads a simple URL or angle-wrapped URL after a link delimiter.
func markdownDestination(line string, offset, start int) (markdownURLRange, bool) {
	for start < len(line) && (line[start] == ' ' || line[start] == '\t') {
		start++
	}
	if start >= len(line) {
		return markdownURLRange{}, false
	}
	if line[start] == '<' {
		start++
		end := strings.IndexByte(line[start:], '>')
		if end < 0 || end == 0 {
			return markdownURLRange{}, false
		}
		return markdownURLRange{start: offset + start, end: offset + start + end}, true
	}
	end := start
	for end < len(line) && !strings.ContainsRune(" \t\r)", rune(line[end])) {
		end++
	}
	return markdownURLRange{start: offset + start, end: offset + end}, end > start
}

// scanHTMLTag recognizes link-bearing attributes and skips raw code elements.
func (s *markdownURLScanner) scanHTMLTag(line string, offset, start int) (int, bool) {
	end, ok := htmlTagEnd(line, start)
	if !ok {
		return 0, false
	}

	name, attributesStart, closing, ok := htmlTagName(line, start, end)
	if !ok {
		return 0, false
	}

	selfClosing := strings.HasSuffix(strings.TrimSpace(line[attributesStart:end]), "/")
	if isHTMLCodeElement(name) {
		s.inHTMLCode = !closing && !selfClosing
	}
	if s.inHTMLCode || closing || !isHTMLResourceElement(name) {
		return end + 1, true
	}

	s.scanHTMLAttributes(line, offset, attributesStart, end, name)
	return end + 1, true
}

// htmlTagName returns a normalized tag name, the attribute offset, and whether the tag closes an element.
func htmlTagName(line string, start, end int) (string, int, bool, bool) {
	index := start + 1
	closing := index < end && line[index] == '/'
	if closing {
		index++
	}

	nameStart := index
	for index < end && isHTMLNameByte(line[index]) {
		index++
	}
	if index == nameStart {
		return "", 0, false, false
	}

	return strings.ToLower(line[nameStart:index]), index, closing, true
}

// scanHTMLAttributes records resource-bearing attribute values from one supported HTML element.
func (s *markdownURLScanner) scanHTMLAttributes(line string, offset, start, end int, element string) {
	index := start
	for index < end {
		if !isHTMLNameByte(line[index]) {
			index++
			continue
		}
		attributeStart := index
		for index < end && isHTMLNameByte(line[index]) {
			index++
		}
		attribute := strings.ToLower(line[attributeStart:index])
		index = skipHTMLSpace(line, index, end)
		if index >= end || line[index] != '=' {
			continue
		}

		valueStart, valueEnd, next := htmlAttributeValue(line, index+1, end)
		index = next
		if valueStart == valueEnd || !isHTMLResourceAttribute(element, attribute) {
			continue
		}
		s.ranges = append(s.ranges, markdownURLRange{start: offset + valueStart, end: offset + valueEnd})
	}
}

// skipHTMLSpace advances past horizontal whitespace within a tag.
func skipHTMLSpace(line string, index, end int) int {
	for index < end && (line[index] == ' ' || line[index] == '\t') {
		index++
	}
	return index
}

// htmlAttributeValue returns the bounds and next scan position for a quoted or unquoted value.
func htmlAttributeValue(line string, index, end int) (int, int, int) {
	index = skipHTMLSpace(line, index, end)
	quote := byte(0)
	if index < end && (line[index] == '"' || line[index] == '\'') {
		quote = line[index]
		index++
	}

	start := index
	for index < end {
		if quote != 0 && line[index] == quote {
			return start, index, index + 1
		}
		if quote == 0 && (line[index] == ' ' || line[index] == '\t' || line[index] == '\r' || line[index] == '>') {
			break
		}
		index++
	}
	return start, index, index
}

// isHTMLCodeElement reports whether an element suppresses Markdown URL scanning in its body.
func isHTMLCodeElement(name string) bool {
	return name == "code" || name == "pre"
}

// isHTMLResourceElement reports whether an element can contain a supported resource URL.
func isHTMLResourceElement(name string) bool {
	return name == "img" || name == "a"
}

// isHTMLResourceAttribute reports whether an attribute contains the supported URL for its element.
func isHTMLResourceAttribute(element, attribute string) bool {
	return element == "img" && attribute == "src" || element == "a" && attribute == "href"
}

// htmlTagEnd finds a closing angle bracket outside quoted attribute values.
func htmlTagEnd(line string, start int) (int, bool) {
	quote := byte(0)
	for index := start + 1; index < len(line); index++ {
		switch {
		case quote != 0 && line[index] == quote:
			quote = 0
		case quote == 0 && (line[index] == '"' || line[index] == '\''):
			quote = line[index]
		case quote == 0 && line[index] == '>':
			return index, true
		}
	}
	return 0, false
}

// isHTMLNameByte reports whether a byte can belong to an HTML tag or attribute name.
func isHTMLNameByte(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value >= '0' && value <= '9' || value == '-' || value == ':'
}
