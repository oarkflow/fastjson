package fastjson

import (
	"errors"
	"strconv"
	"unicode/utf8"
)

// ErrNotFound is returned by Get and its typed variants when the path does
// not exist in the document.
var ErrNotFound = errors.New("fastjson: path not found")

// Get returns the raw JSON value found by following path through data
// without decoding the rest of the document. Each path element is an object
// key, or a decimal index when the current value is an array. The returned
// slice aliases data.
//
// Only the parts of data that are traversed are validated.
func Get(data []byte, path ...string) ([]byte, error) {
	s := getValueScanner()
	defer putValueScanner(s)
	i := skipSpace(data, 0)
	for _, key := range path {
		if i >= len(data) {
			return nil, stdSyntaxError(data, 0, true)
		}
		switch data[i] {
		case '{':
			next, found, err := findKey(s, data, i, key)
			if err != nil {
				return nil, err
			}
			if !found {
				return nil, ErrNotFound
			}
			i = next
		case '[':
			idx, err := strconv.Atoi(key)
			if err != nil || idx < 0 {
				return nil, ErrNotFound
			}
			next, found, err := findIndex(s, data, i, idx)
			if err != nil {
				return nil, err
			}
			if !found {
				return nil, ErrNotFound
			}
			i = next
		default:
			if _, ok := valueEndAt(s, data, i); !ok {
				return nil, stdSyntaxError(data, 0, true)
			}
			return nil, ErrNotFound
		}
	}
	end, ok := valueEndAt(s, data, i)
	if !ok {
		return nil, stdSyntaxError(data, 0, true)
	}
	return data[i:end], nil
}

// valueEndAt validates the single value starting at data[i] and returns the
// offset just past it.
func valueEndAt(s *valueScanner, data []byte, i int) (int, bool) {
	s.reset()
	end, res := s.scan(data, i, true)
	return end, res == scanDone
}

// findKey looks for key in the object at data[i] and returns the offset of
// its value.
func findKey(s *valueScanner, data []byte, i int, key string) (int, bool, error) {
	i = skipSpace(data, i+1)
	if i < len(data) && data[i] == '}' {
		return 0, false, nil
	}
	for i < len(data) && data[i] == '"' {
		end, res := scanStringBody(data, i+1)
		if res != scanDone {
			break
		}
		match := keyEquals(data[i+1:end-1], key)
		i = skipSpace(data, end)
		if i >= len(data) || data[i] != ':' {
			break
		}
		i = skipSpace(data, i+1)
		if match {
			return i, true, nil
		}
		vend, ok := valueEndAt(s, data, i)
		if !ok {
			break
		}
		i = skipSpace(data, vend)
		if i < len(data) && data[i] == '}' {
			return 0, false, nil
		}
		if i >= len(data) || data[i] != ',' {
			break
		}
		i = skipSpace(data, i+1)
	}
	return 0, false, stdSyntaxError(data, 0, true)
}

func findIndex(s *valueScanner, data []byte, i, idx int) (int, bool, error) {
	i = skipSpace(data, i+1)
	if i < len(data) && data[i] == ']' {
		return 0, false, nil
	}
	for n := 0; i < len(data); n++ {
		if n == idx {
			return i, true, nil
		}
		vend, ok := valueEndAt(s, data, i)
		if !ok {
			break
		}
		i = skipSpace(data, vend)
		if i < len(data) && data[i] == ']' {
			return 0, false, nil
		}
		if i >= len(data) || data[i] != ',' {
			break
		}
		i = skipSpace(data, i+1)
	}
	return 0, false, stdSyntaxError(data, 0, true)
}

// keyEquals compares a raw (validated) string body with key.
func keyEquals(body []byte, key string) bool {
	for _, c := range body {
		if c == '\\' || c >= utf8.RuneSelf {
			return string(unquoteInto(nil, body)) == key
		}
	}
	return string(body) == key
}

// GetString returns the string value at path.
func GetString(data []byte, path ...string) (string, error) {
	v, err := Get(data, path...)
	if err != nil {
		return "", err
	}
	if v[0] != '"' {
		return "", &UnmarshalTypeError{Value: describeValue(v), Type: stringType}
	}
	body := v[1 : len(v)-1]
	return string(unquoteInto(make([]byte, 0, len(body)), body)), nil
}

// GetInt returns the integer value at path.
func GetInt(data []byte, path ...string) (int64, error) {
	v, err := Get(data, path...)
	if err != nil {
		return 0, err
	}
	if c := v[0]; c != '-' && c-'0' > 9 {
		return 0, &UnmarshalTypeError{Value: describeValue(v), Type: reflectInt64}
	}
	n, ok := parseInt(v)
	if !ok {
		return 0, &UnmarshalTypeError{Value: "number " + string(v), Type: reflectInt64}
	}
	return n, nil
}

// GetFloat64 returns the numeric value at path.
func GetFloat64(data []byte, path ...string) (float64, error) {
	v, err := Get(data, path...)
	if err != nil {
		return 0, err
	}
	if c := v[0]; c != '-' && c-'0' > 9 {
		return 0, &UnmarshalTypeError{Value: describeValue(v), Type: float64Type}
	}
	f, err := parseFloat(v, 64)
	if err != nil {
		return 0, &UnmarshalTypeError{Value: "number " + string(v), Type: float64Type}
	}
	return f, nil
}

// GetBool returns the boolean value at path.
func GetBool(data []byte, path ...string) (bool, error) {
	v, err := Get(data, path...)
	if err != nil {
		return false, err
	}
	switch v[0] {
	case 't':
		return true, nil
	case 'f':
		return false, nil
	}
	return false, &UnmarshalTypeError{Value: describeValue(v), Type: reflectBool}
}

func describeValue(v []byte) string {
	switch v[0] {
	case '{':
		return "object"
	case '[':
		return "array"
	case '"':
		return "string"
	case 't', 'f':
		return "bool"
	case 'n':
		return "null"
	}
	return "number"
}
