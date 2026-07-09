package fastjson

import "strconv"

// parseAnyWithEnd returns (value, endOffset, error).
// It parses from b[i:] and returns the end offset into b.
func parseAnyWithEnd(b []byte, i int, opt decodeOptions) (any, int, error) {
	i = skipWS(b, i)
	if i >= len(b) {
		return nil, i, syntaxErr("empty", i)
	}
	switch b[i] {
	case 'n':
		if hasLit(b, i, "null") {
			return nil, i + 4, nil
		}
	case 't':
		if hasLit(b, i, "true") {
			return true, i + 4, nil
		}
	case 'f':
		if hasLit(b, i, "false") {
			return false, i + 5, nil
		}
	case '"':
		s, n, e := parseStringZC(b, i)
		return s, n, e
	case '[':
		return parseArrayAnyWithEnd(b, i, opt)
	case '{':
		return parseObjectAnyWithEnd(b, i, opt)
	default:
		if b[i] == '-' || isDigit(b[i]) {
			n, e := numberEnd(b, i)
			if e != nil {
				return nil, n, e
			}
			s := unsafeString(b[i:n])
			if opt.useNumber {
				return Number(s), n, nil
			}
			f, e := strconv.ParseFloat(s, 64)
			return f, n, e
		}
	}
	return nil, i, syntaxErr("invalid value", i)
}

func parseAny(b []byte, opt decodeOptions) (any, error) {
	v, _, e := parseAnyWithEnd(b, 0, opt)
	return v, e
}

func parseArrayAny(b []byte, i int, opt decodeOptions) (any, error) {
	v, _, e := parseArrayAnyWithEnd(b, i, opt)
	return v, e
}

func parseArrayAnyWithEnd(b []byte, i int, opt decodeOptions) (any, int, error) {
	i++
	arr := make([]any, 0, 4)
	i = skipWS(b, i)
	if i < len(b) && b[i] == ']' {
		return arr, i + 1, nil
	}
	for {
		var v any
		var e error
		v, i, e = parseAnyWithEnd(b, i, opt)
		if e != nil {
			return nil, i, e
		}
		arr = append(arr, v)
		i = skipWS(b, i)
		if i < len(b) && b[i] == '}' {
			return nil, i, syntaxErr("expected ]", i)
		}
		if i < len(b) && b[i] == ']' {
			return arr, i + 1, nil
		}
		if i >= len(b) || b[i] != ',' {
			return nil, i, syntaxErr("expected comma", i)
		}
		i = skipWS(b, i+1)
	}
}

func parseObjectAny(b []byte, i int, opt decodeOptions) (any, error) {
	v, _, e := parseObjectAnyWithEnd(b, i, opt)
	return v, e
}

func parseObjectAnyWithEnd(b []byte, i int, opt decodeOptions) (any, int, error) {
	i++
	m := make(map[string]any, 8)
	i = skipWS(b, i)
	if i < len(b) && b[i] == '}' {
		return m, i + 1, nil
	}
	for {
		k, n, e := parseStringZC(b, i)
		if e != nil {
			return nil, n, e
		}
		i = skipWS(b, n)
		if i >= len(b) || b[i] != ':' {
			return nil, i, syntaxErr("expected colon", i)
		}
		i = skipWS(b, i+1)
		var v any
		v, i, e = parseAnyWithEnd(b, i, opt)
		if e != nil {
			return nil, i, e
		}
		m[k] = v
		i = skipWS(b, i)
		if i < len(b) && b[i] == '}' {
			return m, i + 1, nil
		}
		if i >= len(b) || b[i] != ',' {
			return nil, i, syntaxErr("expected comma", i)
		}
		i = skipWS(b, i+1)
	}
}
