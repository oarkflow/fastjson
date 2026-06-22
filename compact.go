package fastjson

import "bytes"

func Compact(dst *bytes.Buffer, src []byte) error {
	if !Valid(src) {
		_, err := valueEnd(src, 0)
		if err != nil {
			return err
		}
		return syntaxErr("invalid json", 0)
	}
	compactValue(dst, src, 0)
	return nil
}
func compactValue(dst *bytes.Buffer, b []byte, i int) int {
	i = skipWS(b, i)
	switch b[i] {
	case '"':
		_, n, _ := parseString(b, i)
		dst.Write(b[i:n])
		return n
	case '[':
		dst.WriteByte('[')
		i = skipWS(b, i+1)
		if b[i] == ']' {
			dst.WriteByte(']')
			return i + 1
		}
		first := true
		for {
			if !first {
				dst.WriteByte(',')
			} else {
				first = false
			}
			i = compactValue(dst, b, i)
			i = skipWS(b, i)
			if b[i] == ']' {
				dst.WriteByte(']')
				return i + 1
			}
			i = skipWS(b, i+1)
		}
	case '{':
		dst.WriteByte('{')
		i = skipWS(b, i+1)
		if b[i] == '}' {
			dst.WriteByte('}')
			return i + 1
		}
		first := true
		for {
			if !first {
				dst.WriteByte(',')
			} else {
				first = false
			}
			_, n, _ := parseString(b, i)
			dst.Write(b[i:n])
			dst.WriteByte(':')
			i = skipWS(b, n)
			i = compactValue(dst, b, skipWS(b, i+1))
			i = skipWS(b, i)
			if b[i] == '}' {
				dst.WriteByte('}')
				return i + 1
			}
			i = skipWS(b, i+1)
		}
	default:
		n, _ := valueEnd(b, i)
		dst.Write(b[i:n])
		return n
	}
}
func Indent(dst *bytes.Buffer, src []byte, prefix, indent string) error {
	if !Valid(src) {
		return syntaxErr("invalid json", 0)
	}
	var level int
	var inStr, esc bool
	for i, c := range src {
		_ = i
		if inStr {
			dst.WriteByte(c)
			if esc {
				esc = false
			} else if c == '\\' {
				esc = true
			} else if c == '"' {
				inStr = false
			}
			continue
		}
		switch c {
		case ' ', '\n', '\r', '\t':
			continue
		case '"':
			inStr = true
			dst.WriteByte(c)
		case '{', '[':
			dst.WriteByte(c)
			level++
			dst.WriteByte('\n')
			writeIndent(dst, prefix, indent, level)
		case '}', ']':
			level--
			dst.WriteByte('\n')
			writeIndent(dst, prefix, indent, level)
			dst.WriteByte(c)
		case ',':
			dst.WriteByte(c)
			dst.WriteByte('\n')
			writeIndent(dst, prefix, indent, level)
		case ':':
			dst.WriteString(": ")
		default:
			dst.WriteByte(c)
		}
	}
	return nil
}
func writeIndent(dst *bytes.Buffer, prefix, indent string, level int) {
	dst.WriteString(prefix)
	for i := 0; i < level; i++ {
		dst.WriteString(indent)
	}
}
func HTMLEscape(dst *bytes.Buffer, src []byte) {
	for _, c := range src {
		switch c {
		case '<':
			dst.WriteString(`\u003c`)
		case '>':
			dst.WriteString(`\u003e`)
		case '&':
			dst.WriteString(`\u0026`)
		default:
			dst.WriteByte(c)
		}
	}
}
