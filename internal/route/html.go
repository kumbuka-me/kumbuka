package route

import (
	"bytes"
	"io"
	"strings"

	"github.com/containeroo/httpprefix"
	"golang.org/x/net/html"
)

// HTML rewrites local URL attributes in already sanitized HTML. It is not a sanitizer.
// Text, code examples, and external URLs remain untouched.
func HTML(prefix, source string) string {
	if prefix == "" {
		return source
	}
	tokens := html.NewTokenizer(strings.NewReader(source))
	var out bytes.Buffer
	for {
		kind := tokens.Next()
		if kind == html.ErrorToken {
			if tokens.Err() != io.EOF {
				return source
			}
			return out.String()
		}
		raw := append([]byte(nil), tokens.Raw()...)
		if kind != html.StartTagToken && kind != html.SelfClosingTagToken {
			out.Write(raw)
			continue
		}
		token := tokens.Token()
		changed := false
		for i := range token.Attr {
			attr := &token.Attr[i]
			switch attr.Key {
			case "href", "src", "action", "poster":
				target := httpprefix.RouteURL(prefix, attr.Val)
				if target != attr.Val {
					attr.Val = target
					changed = true
				}
			}
		}
		if changed {
			out.WriteString(token.String())
		} else {
			out.Write(raw)
		}
	}
}
