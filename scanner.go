package fastjson

import (
	"fmt"
	"unsafe"
)

type SyntaxError struct {
	msg    string
	Offset int64
}

func (e *SyntaxError) Error() string {
	return fmt.Sprintf("fastjson: %s at byte offset %d", e.msg, e.Offset)
}
func syntaxErr(msg string, off int) error { return &SyntaxError{msg: msg, Offset: int64(off)} }

func skipWS(b []byte, i int) int {
	for i < len(b) {
		switch b[i] {
		case ' ', '\n', '\r', '\t':
			i++
		default:
			return i
		}
	}
	return i
}
func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func valueEnd(b []byte, i int) (int, error) {
	i = skipWS(b, i)
	if i >= len(b) {
		return i, syntaxErr("unexpected eof", i)
	}
	switch b[i] {
	case '"':
		return stringEnd(b, i)
	case '{':
		return objectEnd(b, i)
	case '[':
		return arrayEnd(b, i)
	case 't':
		if i+4 <= len(b) && string(b[i:i+4]) == "true" {
			return i + 4, nil
		}
	case 'f':
		if i+5 <= len(b) && string(b[i:i+5]) == "false" {
			return i + 5, nil
		}
	case 'n':
		if i+4 <= len(b) && string(b[i:i+4]) == "null" {
			return i + 4, nil
		}
	default:
		if b[i] == '-' || isDigit(b[i]) {
			return numberEnd(b, i)
		}
	}
	return i, syntaxErr("invalid value", i)
}
func numberEnd(b []byte, i int) (int, error) {
	start := i
	if b[i] == '-' {
		i++
		if i >= len(b) {
			return i, syntaxErr("bad number", start)
		}
	}
	if b[i] == '0' {
		i++
		if i < len(b) && isDigit(b[i]) {
			return i, syntaxErr("bad number", i)
		}
	} else if isDigit(b[i]) {
		for i < len(b) && isDigit(b[i]) {
			i++
		}
	} else {
		return i, syntaxErr("bad number", i)
	}
	if i < len(b) && b[i] == '.' {
		i++
		if i >= len(b) || !isDigit(b[i]) {
			return i, syntaxErr("bad number", i)
		}
		for i < len(b) && isDigit(b[i]) {
			i++
		}
	}
	if i < len(b) && (b[i] == 'e' || b[i] == 'E') {
		i++
		if i < len(b) && (b[i] == '+' || b[i] == '-') {
			i++
		}
		if i >= len(b) || !isDigit(b[i]) {
			return i, syntaxErr("bad number", i)
		}
		for i < len(b) && isDigit(b[i]) {
			i++
		}
	}
	return i, nil
}
func objectEnd(b []byte, i int) (int, error) {
	i++
	i = skipWS(b, i)
	if i < len(b) && b[i] == '}' {
		return i + 1, nil
	}
	for {
		if i >= len(b) || b[i] != '"' {
			return i, syntaxErr("expected object key", i)
		}
		n, err := stringEnd(b, i)
		if err != nil {
			return n, err
		}
		i = skipWS(b, n)
		if i >= len(b) || b[i] != ':' {
			return i, syntaxErr("expected colon", i)
		}
		n, err = valueEnd(b, i+1)
		if err != nil {
			return n, err
		}
		i = skipWS(b, n)
		if i < len(b) && b[i] == '}' {
			return i + 1, nil
		}
		if i >= len(b) || b[i] != ',' {
			return i, syntaxErr("expected comma", i)
		}
		i = skipWS(b, i+1)
	}
}
func arrayEnd(b []byte, i int) (int, error) {
	i++
	i = skipWS(b, i)
	if i < len(b) && b[i] == ']' {
		return i + 1, nil
	}
	for {
		n, err := valueEnd(b, i)
		if err != nil {
			return n, err
		}
		i = skipWS(b, n)
		if i < len(b) && b[i] == ']' {
			return i + 1, nil
		}
		if i >= len(b) || b[i] != ',' {
			return i, syntaxErr("expected comma", i)
		}
		i = skipWS(b, i+1)
	}
}
func stringEnd(b []byte, i int) (int, error) {
	if i >= len(b) || b[i] != '"' {
		return i, syntaxErr("expected string", i)
	}
	i++
	for i < len(b) {
		c := b[i]
		if c == '"' {
			return i + 1, nil
		}
		if c < 0x20 {
			return i, syntaxErr("bad control in string", i)
		}
		if c == '\\' {
			i++
			if i >= len(b) {
				return i, syntaxErr("bad escape", i)
			}
			switch b[i] {
			case '"', '\\', '/', 'b', 'f', 'n', 'r', 't':
				i++
			case 'u':
				if i+4 >= len(b) {
					return i, syntaxErr("bad unicode escape", i)
				}
				for k := 1; k <= 4; k++ {
					x := b[i+k]
					if !((x >= '0' && x <= '9') || (x >= 'a' && x <= 'f') || (x >= 'A' && x <= 'F')) {
						return i, syntaxErr("bad unicode escape", i)
					}
				}
				i += 5
			default:
				return i, syntaxErr("bad escape", i)
			}
			continue
		}
		i++
	}
	return i, syntaxErr("unterminated string", i)
}

func Valid(b []byte) bool {
	n, err := valueEnd(b, 0)
	if err != nil {
		return false
	}
	return skipWS(b, n) == len(b)
}
func bytesEqString(b []byte, s string) bool {
	if len(b) != len(s) {
		return false
	}
	for i := 0; i < len(b); i++ {
		if b[i] != s[i] {
			return false
		}
	}
	return true
}
func parseIntBytes(b []byte) (int64, error) {
	i := skipWS(b, 0)
	neg := false
	if i < len(b) && b[i] == '-' {
		neg = true
		i++
	}
	if i >= len(b) || !isDigit(b[i]) {
		return 0, syntaxErr("bad number", i)
	}
	var n int64
	for i < len(b) && isDigit(b[i]) {
		d := int64(b[i] - '0')
		if n > (1<<63-1-d)/10 {
			return 0, syntaxErr("number overflow", i)
		}
		n = n*10 + d
		i++
	}
	if neg {
		n = -n
	}
	return n, nil
}
func parseUintBytes(b []byte) (uint64, error) {
	i := skipWS(b, 0)
	if i >= len(b) || !isDigit(b[i]) {
		return 0, syntaxErr("bad number", i)
	}
	var n uint64
	for i < len(b) && isDigit(b[i]) {
		d := uint64(b[i] - '0')
		if n > (^uint64(0)-d)/10 {
			return 0, syntaxErr("number overflow", i)
		}
		n = n*10 + d
		i++
	}
	return n, nil
}
func unsafeString(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	return unsafe.String(unsafe.SliceData(b), len(b))
}
