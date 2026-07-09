package fastjson

import (
	"unicode/utf16"
	"unicode/utf8"
)

var hex = "0123456789abcdef"

func appendString(dst []byte, s string, html bool) []byte {
	dst = append(dst, '"')
	start := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < 0x20 || c == '\\' || c == '"' || (html && (c == '<' || c == '>' || c == '&')) {
			dst = append(dst, s[start:i]...)
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
				dst = append(dst, '\\', 'u', '0', '0', hex[c>>4], hex[c&15])
			}
			start = i + 1
		}
	}
	dst = append(dst, s[start:]...)
	dst = append(dst, '"')
	return dst
}

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
	// unrolled scan: 8 bytes at a time for common ASCII-only strings
	for j+7 < n {
		c0 := b[j]
		if c0 == '"' {
			return emptyOrUnsafe(b, start, j), j + 1, nil
		}
		if c0 == '\\' || c0 < 0x20 || c0 >= 0x80 {
			return parseStringFallback(b, i, start, j)
		}
		c1 := b[j+1]
		if c1 == '"' {
			return emptyOrUnsafe(b, start, j+1), j + 2, nil
		}
		if c1 == '\\' || c1 < 0x20 || c1 >= 0x80 {
			return parseStringFallback(b, i, start, j+1)
		}
		c2 := b[j+2]
		if c2 == '"' {
			return emptyOrUnsafe(b, start, j+2), j + 3, nil
		}
		if c2 == '\\' || c2 < 0x20 || c2 >= 0x80 {
			return parseStringFallback(b, i, start, j+2)
		}
		c3 := b[j+3]
		if c3 == '"' {
			return emptyOrUnsafe(b, start, j+3), j + 4, nil
		}
		if c3 == '\\' || c3 < 0x20 || c3 >= 0x80 {
			return parseStringFallback(b, i, start, j+3)
		}
		c4 := b[j+4]
		if c4 == '"' {
			return emptyOrUnsafe(b, start, j+4), j + 5, nil
		}
		if c4 == '\\' || c4 < 0x20 || c4 >= 0x80 {
			return parseStringFallback(b, i, start, j+4)
		}
		c5 := b[j+5]
		if c5 == '"' {
			return emptyOrUnsafe(b, start, j+5), j + 6, nil
		}
		if c5 == '\\' || c5 < 0x20 || c5 >= 0x80 {
			return parseStringFallback(b, i, start, j+5)
		}
		c6 := b[j+6]
		if c6 == '"' {
			return emptyOrUnsafe(b, start, j+6), j + 7, nil
		}
		if c6 == '\\' || c6 < 0x20 || c6 >= 0x80 {
			return parseStringFallback(b, i, start, j+6)
		}
		c7 := b[j+7]
		if c7 == '"' {
			return emptyOrUnsafe(b, start, j+7), j + 8, nil
		}
		if c7 == '\\' || c7 < 0x20 || c7 >= 0x80 {
			return parseStringFallback(b, i, start, j+7)
		}
		j += 8
	}
	for j < n {
		c := b[j]
		if c == '"' {
			return emptyOrUnsafe(b, start, j), j + 1, nil
		}
		if c == '\\' || c < 0x20 || c >= 0x80 {
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

// parseStringFallback is called when the fast path encounters a non-ASCII or escaped char.
func parseStringFallback(b []byte, origOff int, start, end int) (string, int, error) {
	// If we haven't consumed any chars yet, just use parseString from the original offset.
	if end == start {
		return parseString(b, origOff)
	}
	// We have some safe prefix; build result from what we've already scanned plus the rest.
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
