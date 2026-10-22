// Package fastjson is a high-performance, drop-in replacement for the
// standard library's encoding/json package.
//
// The exported API mirrors encoding/json: Marshal, Unmarshal, MarshalIndent,
// Valid, Compact, Indent, HTMLEscape, Encoder and Decoder behave the same way
// and produce the same bytes. The public types (Number, RawMessage,
// Marshaler, Unmarshaler, Delim, Token and every error type) are aliases of
// the encoding/json types, so values flow freely between the two packages:
// a fastjson.Number is a json.Number, errors match errors.As against
// *json.SyntaxError, and types implementing json.Marshaler are honored.
//
// fastjson follows the classic encoding/json (v1) semantics: case-insensitive
// field matching, HTML escaping by default, sorted map keys, `string`,
// `omitempty` and `omitzero` tag options, embedded struct promotion, base64
// for []byte, TextMarshaler/TextUnmarshaler support and validation of the
// whole input before decoding.
package fastjson

import (
	stdjson "encoding/json"
	"reflect"
	"unsafe"
)

// Aliases of the encoding/json types so fastjson and encoding/json values are
// interchangeable.
type (
	Number                = stdjson.Number
	RawMessage            = stdjson.RawMessage
	Marshaler             = stdjson.Marshaler
	Unmarshaler           = stdjson.Unmarshaler
	Delim                 = stdjson.Delim
	Token                 = stdjson.Token
	SyntaxError           = stdjson.SyntaxError
	UnmarshalTypeError    = stdjson.UnmarshalTypeError
	InvalidUnmarshalError = stdjson.InvalidUnmarshalError
	UnsupportedTypeError  = stdjson.UnsupportedTypeError
	UnsupportedValueError = stdjson.UnsupportedValueError
	MarshalerError        = stdjson.MarshalerError
	UnmarshalFieldError   = stdjson.UnmarshalFieldError
	InvalidUTF8Error      = stdjson.InvalidUTF8Error
)

// NormalizeNumbers recursively converts Number values in generic value trees
// (as produced by Decoder.UseNumber) to int64 when the literal is an integer
// that fits, and to float64 otherwise. Maps and slices are mutated in place.
//
// Marshal already writes Number as a raw JSON number, so this is only needed
// when handing decoded trees to code that does not understand Number.
func NormalizeNumbers(v any) any {
	switch x := v.(type) {
	case Number:
		if i, err := x.Int64(); err == nil {
			return i
		}
		if f, err := x.Float64(); err == nil {
			return f
		}
		return v
	case map[string]any:
		for k, val := range x {
			x[k] = NormalizeNumbers(val)
		}
		return x
	case []any:
		for i, val := range x {
			x[i] = NormalizeNumbers(val)
		}
		return x
	default:
		return v
	}
}

// The unexported message fields of SyntaxError and MarshalerError are located
// through reflection once, so errors can be built with the exact text
// encoding/json would produce.
var (
	syntaxErrMsgOffset    = unexportedStringField(reflect.TypeFor[SyntaxError](), "msg")
	marshalerErrSrcOffset = unexportedStringField(reflect.TypeFor[MarshalerError](), "sourceFunc")
)

func unexportedStringField(t reflect.Type, name string) uintptr {
	if f, ok := t.FieldByName(name); ok && f.Type.Kind() == reflect.String && len(f.Index) == 1 {
		return f.Offset
	}
	return ^uintptr(0)
}

func newSyntaxError(msg string, offset int64) *SyntaxError {
	e := &SyntaxError{Offset: offset}
	if syntaxErrMsgOffset != ^uintptr(0) {
		*(*string)(unsafe.Add(unsafe.Pointer(e), syntaxErrMsgOffset)) = msg
	}
	return e
}

func newMarshalerError(t reflect.Type, err error, sourceFunc string) *MarshalerError {
	e := &MarshalerError{Type: t, Err: err}
	if marshalerErrSrcOffset != ^uintptr(0) {
		*(*string)(unsafe.Add(unsafe.Pointer(e), marshalerErrSrcOffset)) = sourceFunc
	}
	return e
}

// isValidNumber reports whether s is a valid JSON number literal.
func isValidNumber(s string) bool {
	if s == "" {
		return false
	}
	if s[0] == '-' {
		s = s[1:]
		if s == "" {
			return false
		}
	}
	switch {
	default:
		return false
	case s[0] == '0':
		s = s[1:]
	case '1' <= s[0] && s[0] <= '9':
		s = s[1:]
		for len(s) > 0 && '0' <= s[0] && s[0] <= '9' {
			s = s[1:]
		}
	}
	if len(s) >= 2 && s[0] == '.' && '0' <= s[1] && s[1] <= '9' {
		s = s[2:]
		for len(s) > 0 && '0' <= s[0] && s[0] <= '9' {
			s = s[1:]
		}
	}
	if len(s) >= 2 && (s[0] == 'e' || s[0] == 'E') {
		s = s[1:]
		if s[0] == '+' || s[0] == '-' {
			s = s[1:]
			if s == "" {
				return false
			}
		}
		for len(s) > 0 && '0' <= s[0] && s[0] <= '9' {
			s = s[1:]
		}
	}
	return s == ""
}
