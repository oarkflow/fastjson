package fastjson

import (
	"unicode/utf16"
	"unicode/utf8"
)

var hex = "0123456789abcdef"

// escapeTable[c] is true for JSON characters that need escaping in strings
var escapeTable [256]bool
var escapeTableHTML [256]bool

func init() {
	for c := 0; c < 0x20; c++ {
		escapeTable[c] = true
		escapeTableHTML[c] = true
	}
	escapeTable['\\'] = true
	escapeTable['"'] = true
	escapeTableHTML['\\'] = true
	escapeTableHTML['"'] = true
	escapeTableHTML['<'] = true
	escapeTableHTML['>'] = true
	escapeTableHTML['&'] = true
}

func appendString(dst []byte, s string, html bool) []byte {
	dst = append(dst, '"')
	if html {
		return appendStringHTML(dst, s)
	}
	return appendStringPlain(dst, s)
}

func appendStringPlain(dst []byte, s string) []byte {
	start := 0
	n := len(s)
	for start < n {
		i := start
		for i < n && !escapeTable[s[i]] {
			i++
		}
		if i > start {
			dst = append(dst, s[start:i]...)
		}
		if i >= n {
			break
		}
		c := s[i]
		switch c {
		case '\\', '"':
			dst = append(dst, '\\', c)
		case '\n':
			dst = append(dst, '\\', 'n')
		case '\r':
			dst = append(dst, '\\', 'r')
		case '\t':
			dst = append(dst, '\\', 't')
		case '\b':
			dst = append(dst, '\\', 'b')
		case '\f':
			dst = append(dst, '\\', 'f')
		default:
			dst = append(dst, "\\u00"...)
			dst = append(dst, hex[c>>4], hex[c&0x0f])
		}
		start = i + 1
	}
	dst = append(dst, '"')
	return dst
}

func appendStringHTML(dst []byte, s string) []byte {
	start := 0
	n := len(s)
	for start < n {
		// bulk-scan using lookup table
		i := start
		for i < n && !escapeTableHTML[s[i]] {
			i++
		}
		if i > start {
			dst = append(dst, s[start:i]...)
		}
		if i >= n {
			break
		}
		c := s[i]
		switch c {
		case '\\', '"':
			dst = append(dst, '\\', c)
		case '\n':
			dst = append(dst, '\\', 'n')
		case '\r':
			dst = append(dst, '\\', 'r')
		case '\t':
			dst = append(dst, '\\', 't')
		case '\b':
			dst = append(dst, '\\', 'b')
		case '\f':
			dst = append(dst, '\\', 'f')
		case '<':
			dst = append(dst, `\u003c`...)
		case '>':
			dst = append(dst, `\u003e`...)
		case '&':
			dst = append(dst, `\u0026`...)
		default:
			dst = append(dst, "\\u00"...)
			dst = append(dst, hex[c>>4], hex[c&0x0f])
		}
		start = i + 1
	}
	dst = append(dst, '"')
	return dst
}

// parseString fully parses a JSON string, handling escapes and Unicode.
func parseString(b []byte, i int) (string, int, error) {
	if i >= len(b) || b[i] != '"' {
		return "", i, syntaxErr("expected string", i)
	}
	i++
	start := i
	for i < len(b) {
		c := b[i]
		if c == '"' {
			return string(b[start:i]), i + 1, nil
		}
		if c == '\\' || c < 0x20 || c >= 0x80 {
			break
		}
		i++
	}
	out := make([]byte, 0, len(b)-start)
	out = append(out, b[start:i]...)
	for i < len(b) {
		c := b[i]
		if c == '"' {
			return string(out), i + 1, nil
		}
		if c < 0x20 {
			return "", i, syntaxErr("bad control in string", i)
		}
		if c == '\\' {
			i++
			if i >= len(b) {
				return "", i, syntaxErr("bad escape", i)
			}
			switch b[i] {
			case '"', '\\', '/':
				out = append(out, b[i])
				i++
			case 'b':
				out = append(out, '\b')
				i++
			case 'f':
				out = append(out, '\f')
				i++
			case 'n':
				out = append(out, '\n')
				i++
			case 'r':
				out = append(out, '\r')
				i++
			case 't':
				out = append(out, '\t')
				i++
			case 'u':
				if i+4 >= len(b) {
					return "", i, syntaxErr("bad unicode escape", i)
				}
				r := rune(0)
				for k := 1; k <= 4; k++ {
					x := b[i+k]
					var v byte
					switch {
					case x >= '0' && x <= '9':
						v = x - '0'
					case x >= 'a' && x <= 'f':
						v = x - 'a' + 10
					case x >= 'A' && x <= 'F':
						v = x - 'A' + 10
					default:
						return "", i, syntaxErr("bad unicode escape", i)
					}
					r = r*16 + rune(v)
				}
				if utf16.IsSurrogate(r) {
					if i+6 < len(b) && b[i+5] == '\\' && b[i+6] == 'u' {
						r2 := rune(0)
						for k := 7; k <= 10; k++ {
							if k >= len(b) {
								return "", i, syntaxErr("bad unicode escape", i)
							}
							x := b[i+k]
							var v byte
							switch {
							case x >= '0' && x <= '9':
								v = x - '0'
							case x >= 'a' && x <= 'f':
								v = x - 'a' + 10
							case x >= 'A' && x <= 'F':
								v = x - 'A' + 10
							default:
								return "", i, syntaxErr("bad unicode escape", i)
							}
							r2 = r2*16 + rune(v)
						}
						if utf16.IsSurrogate(r2) {
							r = utf16.DecodeRune(r, r2)
							i += 11
							out = append(out, string(r)...)
							continue
						}
					}
					return "", i, syntaxErr("bad unicode surrogate pair", i)
				}
				out = append(out, string(r)...)
				i += 5
			default:
				return "", i, syntaxErr("bad escape", i)
			}
			continue
		}
		if c < utf8.RuneSelf {
			out = append(out, c)
			i++
			continue
		}
		r, n := utf8.DecodeRune(b[i:])
		if r == utf8.RuneError && n == 1 {
			return "", i, syntaxErr("bad utf8", i)
		}
		out = append(out, b[i:i+n]...)
		i += n
	}
	return "", i, syntaxErr("unterminated string", i)
}

func parseStringZC(b []byte, i int) (string, int, error) {
	if i >= len(b) || b[i] != '"' {
		return "", i, syntaxErr("expected string", i)
	}
	start := i + 1
	j := start
	n := len(b)
	for j+7 < n {
		if b[j] == '"' {
			return emptyOrUnsafe(b, start, j), j + 1, nil
		}
		if escapeTable[b[j]] {
			return parseStringFallback(b, i, start, j)
		}
		if b[j+1] == '"' {
			return emptyOrUnsafe(b, start, j+1), j + 2, nil
		}
		if escapeTable[b[j+1]] {
			return parseStringFallback(b, i, start, j+1)
		}
		if b[j+2] == '"' {
			return emptyOrUnsafe(b, start, j+2), j + 3, nil
		}
		if escapeTable[b[j+2]] {
			return parseStringFallback(b, i, start, j+2)
		}
		if b[j+3] == '"' {
			return emptyOrUnsafe(b, start, j+3), j + 4, nil
		}
		if escapeTable[b[j+3]] {
			return parseStringFallback(b, i, start, j+3)
		}
		if b[j+4] == '"' {
			return emptyOrUnsafe(b, start, j+4), j + 5, nil
		}
		if escapeTable[b[j+4]] {
			return parseStringFallback(b, i, start, j+4)
		}
		if b[j+5] == '"' {
			return emptyOrUnsafe(b, start, j+5), j + 6, nil
		}
		if escapeTable[b[j+5]] {
			return parseStringFallback(b, i, start, j+5)
		}
		if b[j+6] == '"' {
			return emptyOrUnsafe(b, start, j+6), j + 7, nil
		}
		if escapeTable[b[j+6]] {
			return parseStringFallback(b, i, start, j+6)
		}
		if b[j+7] == '"' {
			return emptyOrUnsafe(b, start, j+7), j + 8, nil
		}
		if escapeTable[b[j+7]] {
			return parseStringFallback(b, i, start, j+7)
		}
		j += 8
	}
	for j < n {
		if b[j] == '"' {
			return emptyOrUnsafe(b, start, j), j + 1, nil
		}
		if escapeTable[b[j]] {
			return parseStringFallback(b, i, start, j)
		}
		j++
	}
	for j < n {
		c := b[j]
		if c == '"' {
			return emptyOrUnsafe(b, start, j), j + 1, nil
		}
		if escapeTable[c] {
			return parseStringFallback(b, i, start, j)
		}
		j++
	}
	return "", j, syntaxErr("unterminated string", i)
}

func emptyOrUnsafe(b []byte, start, end int) string {
	if end == start {
		return ""
	}
	return unsafeString(b[start:end])
}

func parseStringFallback(b []byte, origOff int, start, end int) (string, int, error) {
	if end == start {
		return parseString(b, origOff)
	}
	out := make([]byte, 0, len(b)-start)
	out = append(out, b[start:end]...)
	i := end
	n := len(b)
	for i < n {
		c := b[i]
		if c == '"' {
			return string(out), i + 1, nil
		}
		if c < 0x20 {
			return "", i, syntaxErr("bad control in string", i)
		}
		if c == '\\' {
			i++
			if i >= n {
				return "", i, syntaxErr("bad escape", i)
			}
			switch b[i] {
			case '"', '\\', '/':
				out = append(out, b[i])
				i++
			case 'b':
				out = append(out, '\b')
				i++
			case 'f':
				out = append(out, '\f')
				i++
			case 'n':
				out = append(out, '\n')
				i++
			case 'r':
				out = append(out, '\r')
				i++
			case 't':
				out = append(out, '\t')
				i++
			case 'u':
				if i+4 >= n {
					return "", i, syntaxErr("bad unicode escape", i)
				}
				r := rune(0)
				for k := 1; k <= 4; k++ {
					x := b[i+k]
					var v byte
					switch {
					case x >= '0' && x <= '9':
						v = x - '0'
					case x >= 'a' && x <= 'f':
						v = x - 'a' + 10
					case x >= 'A' && x <= 'F':
						v = x - 'A' + 10
					default:
						return "", i, syntaxErr("bad unicode escape", i)
					}
					r = r*16 + rune(v)
				}
				if utf16.IsSurrogate(r) {
					if i+6 < n && b[i+5] == '\\' && b[i+6] == 'u' {
						r2 := rune(0)
						for k := 7; k <= 10; k++ {
							if k >= n {
								return "", i, syntaxErr("bad unicode escape", i)
							}
							x := b[i+k]
							var v byte
							switch {
							case x >= '0' && x <= '9':
								v = x - '0'
							case x >= 'a' && x <= 'f':
								v = x - 'a' + 10
							case x >= 'A' && x <= 'F':
								v = x - 'A' + 10
							default:
								return "", i, syntaxErr("bad unicode escape", i)
							}
							r2 = r2*16 + rune(v)
						}
						if utf16.IsSurrogate(r2) {
							r = utf16.DecodeRune(r, r2)
							i += 11
							out = append(out, string(r)...)
							continue
						}
					}
					return "", i, syntaxErr("bad unicode surrogate pair", i)
				}
				out = append(out, string(r)...)
				i += 5
			default:
				return "", i, syntaxErr("bad escape", i)
			}
			continue
		}
		if c < utf8.RuneSelf {
			out = append(out, c)
			i++
			continue
		}
		r, rn := utf8.DecodeRune(b[i:])
		if r == utf8.RuneError && rn == 1 {
			return "", i, syntaxErr("bad utf8", i)
		}
		out = append(out, b[i:i+rn]...)
		i += rn
	}
	return "", i, syntaxErr("unterminated string", i)
}
