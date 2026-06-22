package fastjson

import "strconv"

func parseAny(b []byte, opt decodeOptions) (any, error) {
	i := skipWS(b, 0)
	if i >= len(b) {
		return nil, syntaxErr("empty", 0)
	}
	switch b[i] {
	case 'n':
		if hasLit(b, i, "null") {
			return nil, nil
		}
	case 't':
		if hasLit(b, i, "true") {
			return true, nil
		}
	case 'f':
		if hasLit(b, i, "false") {
			return false, nil
		}
	case '"':
		s, _, e := parseString(b, i)
		return s, e
	case '[':
		return parseArrayAny(b, i, opt)
	case '{':
		return parseObjectAny(b, i, opt)
	default:
		if b[i] == '-' || isDigit(b[i]) {
			n, e := numberEnd(b, i)
			if e != nil {
				return nil, e
			}
			s := string(b[i:n])
			if opt.useNumber {
				return Number(s), nil
			}
			f, e := strconv.ParseFloat(s, 64)
			return f, e
		}
	}
	return nil, syntaxErr("invalid value", i)
}
func parseArrayAny(b []byte, i int, opt decodeOptions) (any, error) {
	i++
	arr := make([]any, 0)
	i = skipWS(b, i)
	if i < len(b) && b[i] == ']' {
		return arr, nil
	}
	for {
		end, e := valueEnd(b, i)
		if e != nil {
			return nil, e
		}
		v, e := parseAny(b[i:end], opt)
		if e != nil {
			return nil, e
		}
		arr = append(arr, v)
		i = skipWS(b, end)
		if i < len(b) && b[i] == ']' {
			return arr, nil
		}
		if i >= len(b) || b[i] != ',' {
			return nil, syntaxErr("expected comma", i)
		}
		i = skipWS(b, i+1)
	}
}
func parseObjectAny(b []byte, i int, opt decodeOptions) (any, error) {
	i++
	m := make(map[string]any)
	i = skipWS(b, i)
	if i < len(b) && b[i] == '}' {
		return m, nil
	}
	for {
		k, n, e := parseString(b, i)
		if e != nil {
			return nil, e
		}
		i = skipWS(b, n)
		if i >= len(b) || b[i] != ':' {
			return nil, syntaxErr("expected colon", i)
		}
		i = skipWS(b, i+1)
		end, e := valueEnd(b, i)
		if e != nil {
			return nil, e
		}
		v, e := parseAny(b[i:end], opt)
		if e != nil {
			return nil, e
		}
		m[k] = v
		i = skipWS(b, end)
		if i < len(b) && b[i] == '}' {
			return m, nil
		}
		if i >= len(b) || b[i] != ',' {
			return nil, syntaxErr("expected comma", i)
		}
		i = skipWS(b, i+1)
	}
}
