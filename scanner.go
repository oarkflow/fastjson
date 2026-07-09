package fastjson

import (
	"fmt"
	"strconv"
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

var wsTab [256]bool

func init() {
	wsTab[' '] = true
	wsTab['\n'] = true
	wsTab['\r'] = true
	wsTab['\t'] = true
}

func skipWS(b []byte, i int) int {
	for i < len(b) && wsTab[b[i]] {
		i++
	}
	return i
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

//go:nosplit
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
		if i+4 <= len(b) && b[i+1] == 'r' && b[i+2] == 'u' && b[i+3] == 'e' {
			return i + 4, nil
		}
	case 'f':
		if i+5 <= len(b) && b[i+1] == 'a' && b[i+2] == 'l' && b[i+3] == 's' && b[i+4] == 'e' {
			return i + 5, nil
		}
	case 'n':
		if i+4 <= len(b) && b[i+1] == 'u' && b[i+2] == 'l' && b[i+3] == 'l' {
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
		if i < len(b) && b[i] >= '0' && b[i] <= '9' {
			return i, syntaxErr("bad number", i)
		}
	} else if b[i] >= '1' && b[i] <= '9' {
		i++
		for i < len(b) && b[i] >= '0' && b[i] <= '9' {
			i++
		}
	} else {
		return i, syntaxErr("bad number", i)
	}
	if i < len(b) && b[i] == '.' {
		i++
		if i >= len(b) || b[i] < '0' || b[i] > '9' {
			return i, syntaxErr("bad number", i)
		}
		for i < len(b) && b[i] >= '0' && b[i] <= '9' {
			i++
		}
	}
	if i < len(b) && (b[i] == 'e' || b[i] == 'E') {
		i++
		if i < len(b) && (b[i] == '+' || b[i] == '-') {
			i++
		}
		if i >= len(b) || b[i] < '0' || b[i] > '9' {
			return i, syntaxErr("bad number", i)
		}
		for i < len(b) && b[i] >= '0' && b[i] <= '9' {
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
	// manual unrolling: scan 8 bytes at a time for common case
	n := len(b)
	for i+7 < n {
		if b[i] == '"' {
			return i + 1, nil
		}
		if b[i] == '\\' || b[i] < 0x20 {
			goto slow
		}
		if b[i+1] == '"' {
			return i + 2, nil
		}
		if b[i+1] == '\\' || b[i+1] < 0x20 {
			i++
			goto slow
		}
		if b[i+2] == '"' {
			return i + 3, nil
		}
		if b[i+2] == '\\' || b[i+2] < 0x20 {
			i += 2
			goto slow
		}
		if b[i+3] == '"' {
			return i + 4, nil
		}
		if b[i+3] == '\\' || b[i+3] < 0x20 {
			i += 3
			goto slow
		}
		if b[i+4] == '"' {
			return i + 5, nil
		}
		if b[i+4] == '\\' || b[i+4] < 0x20 {
			i += 4
			goto slow
		}
		if b[i+5] == '"' {
			return i + 6, nil
		}
		if b[i+5] == '\\' || b[i+5] < 0x20 {
			i += 5
			goto slow
		}
		if b[i+6] == '"' {
			return i + 7, nil
		}
		if b[i+6] == '\\' || b[i+6] < 0x20 {
			i += 6
			goto slow
		}
		if b[i+7] == '"' {
			return i + 8, nil
		}
		if b[i+7] == '\\' || b[i+7] < 0x20 {
			i += 7
			goto slow
		}
		i += 8
	}
	for i < n {
		c := b[i]
		if c == '"' {
			return i + 1, nil
		}
		if c < 0x20 {
			return i, syntaxErr("bad control in string", i)
		}
		if c == '\\' {
			goto slow
		}
		i++
	}
	return i, syntaxErr("unterminated string", i)
slow:
	// handle escape sequences
	for i < n {
		c := b[i]
		if c == '"' {
			return i + 1, nil
		}
		if c < 0x20 {
			return i, syntaxErr("bad control in string", i)
		}
		if c == '\\' {
			i++
			if i >= n {
				return i, syntaxErr("bad escape", i)
			}
			switch b[i] {
			case '"', '\\', '/', 'b', 'f', 'n', 'r', 't':
				i++
			case 'u':
				if i+4 >= n {
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
	i := 0
	neg := false
	if len(b) > 0 && b[0] == '-' {
		neg = true
		i++
	}
	if i >= len(b) || b[i] < '0' || b[i] > '9' {
		return 0, syntaxErr("bad number", i)
	}
	var n int64
	overflow := int64(1<<63 - 1)
	for i < len(b) && b[i] >= '0' && b[i] <= '9' {
		d := int64(b[i] - '0')
		if n > (overflow-d)/10 {
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
	i := 0
	if i >= len(b) || b[i] < '0' || b[i] > '9' {
		return 0, syntaxErr("bad number", i)
	}
	var n uint64
	overflow := ^uint64(0)
	for i < len(b) && b[i] >= '0' && b[i] <= '9' {
		d := uint64(b[i] - '0')
		if n > (overflow-d)/10 {
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

// --- Lazy field extraction (Get family) ---

// Get extracts the raw JSON value bytes for the named key from a JSON object.
// It performs a single-pass scan and returns the value without any parsing or allocation.
// Returns an error if the key is not found.
func Get(src []byte, key string) ([]byte, error) {
	i := skipWS(src, 0)
	if i >= len(src) || src[i] != '{' {
		return nil, syntaxErr("expected object", i)
	}
	i++
	for {
		i = skipWS(src, i)
		if i >= len(src) || src[i] == '}' {
			return nil, fmt.Errorf("fastjson: key %q not found", key)
		}
		if src[i] != '"' {
			return nil, syntaxErr("expected object key", i)
		}
		ks := i + 1
		j := ks
		simple := true
		for j < len(src) && src[j] != '"' {
			if src[j] == '\\' || src[j] < 0x20 {
				simple = false
				break
			}
			j++
		}
		var matched bool
		if simple && j < len(src) {
			matched = (j-ks) == len(key) && bytesEqString(src[ks:j], key)
			i = j + 1
		} else {
			k, n, err := parseString(src, i)
			if err != nil {
				return nil, err
			}
			matched = k == key
			i = n
		}
		i = skipWS(src, i)
		if i >= len(src) || src[i] != ':' {
			return nil, syntaxErr("expected colon", i)
		}
		valStart := skipWS(src, i+1)
		valEnd, err := valueEnd(src, valStart)
		if err != nil {
			return nil, err
		}
		if matched {
			return src[valStart:valEnd], nil
		}
		i = skipWS(src, valEnd)
		if i < len(src) && src[i] == ',' {
			i++
		} else if i < len(src) && src[i] == '}' {
			return nil, fmt.Errorf("fastjson: key %q not found", key)
		} else {
			return nil, syntaxErr("expected comma or }", i)
		}
	}
}

// GetString extracts the value for key as a string.
func GetString(src []byte, key string) (string, error) {
	raw, err := Get(src, key)
	if err != nil {
		return "", err
	}
	if len(raw) == 0 || raw[0] != '"' {
		return "", syntaxErr("expected string", 0)
	}
	s, _, err := parseStringZC(raw, 0)
	return s, err
}

// GetInt extracts the value for key as an int64.
func GetInt(src []byte, key string) (int64, error) {
	raw, err := Get(src, key)
	if err != nil {
		return 0, err
	}
	return parseIntBytes(raw)
}

// GetFloat64 extracts the value for key as a float64.
func GetFloat64(src []byte, key string) (float64, error) {
	raw, err := Get(src, key)
	if err != nil {
		return 0, err
	}
	return strconv.ParseFloat(unsafeString(raw), 64)
}

// GetBool extracts the value for key as a bool.
func GetBool(src []byte, key string) (bool, error) {
	raw, err := Get(src, key)
	if err != nil {
		return false, err
	}
	if len(raw) == 4 && raw[0] == 't' && raw[1] == 'r' && raw[2] == 'u' && raw[3] == 'e' {
		return true, nil
	}
	if len(raw) == 5 && raw[0] == 'f' && raw[1] == 'a' && raw[2] == 'l' && raw[3] == 's' && raw[4] == 'e' {
		return false, nil
	}
	return false, syntaxErr("expected bool", 0)
}
