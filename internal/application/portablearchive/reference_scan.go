package portablearchive

import (
	"strconv"
	"strings"

	"github.com/kumbuka-me/kumbuka/pkg/ascii"
)

// portableResourceKind identifies the Kumbuka resource URL family represented by a reference.
type portableResourceKind string

const (
	portableMediaResource      portableResourceKind = "media"
	portableAttachmentResource portableResourceKind = "attachments"
)

// portableResourceReference identifies one stored resource reference in Markdown source.
type portableResourceReference struct {
	// Start is the byte offset where the resource URL begins.
	Start int
	// End is the byte offset immediately after the resource URL.
	End int
	// Kind identifies whether the reference points at an image or attachment.
	Kind portableResourceKind
	// ID is the source-instance resource identifier.
	ID int64
}

// nextPortableResourceReference returns the earliest stored image or attachment URL in source.
func nextPortableResourceReference(source string) (portableResourceReference, bool) {
	bestStart := len(source) + 1
	var best portableResourceReference

	for _, candidate := range []struct {
		// kind selects the media service used to load this resource.
		kind portableResourceKind
		// prefix identifies stored URLs belonging to that resource kind.
		prefix string
	}{
		{kind: portableMediaResource, prefix: "/media/"},
		{kind: portableAttachmentResource, prefix: "/attachments/"},
	} {
		reference, ok := findPortableResourceReference(source, candidate.prefix)
		if ok && reference.Start < bestStart {
			reference.Kind = candidate.kind
			bestStart = reference.Start
			best = reference
		}
	}

	return best, bestStart <= len(source)
}

// exactPortableResourceReference validates one complete stored-resource URL of the requested kind.
func exactPortableResourceReference(value string, kind portableResourceKind) (portableResourceReference, bool) {
	reference, ok := findPortableResourceReference(value, "/"+string(kind)+"/")
	if !ok || reference.Start != 0 || reference.End != len(value) {
		return portableResourceReference{}, false
	}
	reference.Kind = kind
	return reference, true
}

// findPortableResourceReference skips malformed URLs to find the first valid stored resource of one kind.
func findPortableResourceReference(source, prefix string) (portableResourceReference, bool) {
	for offset := 0; offset < len(source); {
		relative := strings.Index(source[offset:], prefix)
		if relative < 0 {
			break
		}
		start := offset + relative
		digitsStart := start + len(prefix)
		offset = digitsStart

		digitsEnd := portableResourceIDEnd(source, digitsStart)
		if !hasPortableResourceIDTerminator(source, digitsStart, digitsEnd) {
			continue
		}

		end := portableResourceReferenceEnd(source, digitsEnd+1)
		id, err := strconv.ParseInt(source[digitsStart:digitsEnd], 10, 64)
		if err != nil || !validPortableResourceReference(id, digitsEnd, end) {
			continue
		}

		return portableResourceReference{Start: start, End: end, ID: id}, true
	}
	return portableResourceReference{}, false
}

// portableResourceIDEnd returns the first byte after a decimal resource ID beginning at start.
func portableResourceIDEnd(value string, start int) int {
	end := start
	for end < len(value) && ascii.IsDigit(value[end]) {
		end++
	}
	return end
}

// hasPortableResourceIDTerminator reports whether a resource reference contains digits followed by a path separator.
func hasPortableResourceIDTerminator(source string, digitsStart, digitsEnd int) bool {
	return digitsEnd > digitsStart && digitsEnd < len(source) && source[digitsEnd] == '/'
}

// validPortableResourceReference reports whether a parsed resource ID is positive and has a non-empty filename suffix.
func validPortableResourceReference(id int64, digitsEnd, end int) bool {
	return id > 0 && end > digitsEnd+1
}

// portableResourceReferenceEnd returns the first byte after a bare stored-resource reference suffix.
func portableResourceReferenceEnd(value string, start int) int {
	end := start
	for end < len(value) && !isPortableResourceReferenceTerminator(value[end]) {
		end++
	}
	return end
}

// isPortableResourceReferenceTerminator reports whether value ends a bare stored-resource URL in Markdown text.
func isPortableResourceReferenceTerminator(value byte) bool {
	switch value {
	case ' ', '\t', '\n', '\r', '\f', ')', '"', '\'':
		return true
	default:
		return false
	}
}
