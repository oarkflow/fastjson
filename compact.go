package fastjson

import "bytes"

// Valid reports whether data is a valid JSON encoding.
func Valid(data []byte) bool { return valid(data) }

// Compact appends to dst the JSON-encoded src with insignificant space
// characters elided.
func Compact(dst *bytes.Buffer, src []byte) error {
	dst.Grow(len(src))
	b, err := appendCompact(dst.AvailableBuffer(), src, false)
	dst.Write(b)
	return err
}

// appendCompact validates src and appends its compact form to dst. With
// escape set, <, >, & and U+2028/U+2029 are escaped as encoding/json does
// for the output of MarshalJSON methods.
func appendCompact(dst, src []byte, escape bool) ([]byte, error) {
	if !valid(src) {
		// encoding/json's compact scanner does not count bytes, so the
		// reported offset stays at zero.
		return dst, stdSyntaxError(src, 0, false)
	}
	return appendCompactValid(dst, src, escape), nil
}

func appendCompactValid(dst, src []byte, escape bool) []byte {
	start := 0
	for i := 0; i < len(src); {
		c := src[i]
		switch {
		case c == '"':
			end := skipString(src, i)
			if escape {
				dst = append(dst, src[start:i]...)
				dst = appendHTMLEscape(dst, src[i:end])
				start = end
			}
			i = end
		case isSpace(c):
			dst = append(dst, src[start:i]...)
			i++
			start = i
		default:
			i++
		}
	}
	return append(dst, src[start:]...)
}

// Indent appends to dst an indented form of the JSON-encoded src.
func Indent(dst *bytes.Buffer, src []byte, prefix, indent string) error {
	if !valid(src) {
		return stdSyntaxError(src, 0, true)
	}
	dst.Grow(2 * len(src))
	dst.Write(appendIndentValid(dst.AvailableBuffer(), src, prefix, indent))
	return nil
}

func appendNewline(dst []byte, prefix, indent string, depth int) []byte {
	dst = append(dst, '\n')
	dst = append(dst, prefix...)
	for i := 0; i < depth; i++ {
		dst = append(dst, indent...)
	}
	return dst
}

// appendIndentValid indents src, which must be valid JSON, exactly like
// encoding/json.Indent: leading space is dropped, trailing space is kept and
// empty arrays and objects stay on one line.
func appendIndentValid(dst, src []byte, prefix, indent string) []byte {
	i := skipSpace(src, 0)
	needIndent := false
	depth := 0
	for i < len(src) {
		c := src[i]
		if isSpace(c) {
			i++
			continue
		}
		if needIndent && c != '}' && c != ']' {
			needIndent = false
			depth++
			dst = appendNewline(dst, prefix, indent, depth)
		}
		switch c {
		case '"':
			end := skipString(src, i)
			dst = append(dst, src[i:end]...)
			i = end
		case '{', '[':
			needIndent = true
			dst = append(dst, c)
			i++
		case ',':
			dst = append(dst, c)
			dst = appendNewline(dst, prefix, indent, depth)
			i++
		case ':':
			dst = append(dst, c, ' ')
			i++
		case '}', ']':
			if needIndent {
				// suppress indent in empty object/array
				needIndent = false
			} else {
				depth--
				dst = appendNewline(dst, prefix, indent, depth)
			}
			dst = append(dst, c)
			i++
		default:
			j := i + 1
			for j < len(src) {
				if d := src[j]; isSpace(d) || d == ',' || d == ':' || d == '}' || d == ']' {
					break
				}
				j++
			}
			dst = append(dst, src[i:j]...)
			i = j
		}
		if depth == 0 && !needIndent {
			// The top-level value is complete; trailing space is preserved.
			return append(dst, src[i:]...)
		}
	}
	return dst
}

// HTMLEscape appends to dst the JSON-encoded src with <, >, &, U+2028 and
// U+2029 characters inside string literals changed to \u003c, \u003e,
// \u0026, \u2028, \u2029.
func HTMLEscape(dst *bytes.Buffer, src []byte) {
	dst.Grow(len(src))
	dst.Write(appendHTMLEscape(dst.AvailableBuffer(), src))
}
