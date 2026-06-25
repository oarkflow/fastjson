package fastjson

import (
	"bytes"
	"errors"
	"io"
	"strconv"
)

type Number string

func (n Number) String() string            { return string(n) }
func (n Number) Float64() (float64, error) { return strconv.ParseFloat(string(n), 64) }
func (n Number) Int64() (int64, error)     { return strconv.ParseInt(string(n), 10, 64) }

// MarshalJSON emits Number as a JSON number literal, not as a quoted string.
// This mirrors encoding/json.Number behavior and keeps UseNumber round-trips
// valid for generic config trees such as map[string]any / []any.
func (n Number) MarshalJSON() ([]byte, error) {
	s := string(n)
	if !validNumberLiteral(s) {
		return nil, errors.New("fastjson: invalid Number literal")
	}
	return []byte(s), nil
}

// NormalizeNumbers recursively converts fastjson.Number values in generic value
// trees to concrete Go numeric types. It mutates maps and slices in place.
//
// Prefer Marshal for normal encoding; Marshal already writes Number as a raw
// JSON number. Use NormalizeNumbers when handing decoded trees to code that
// does not understand fastjson.Number.
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

func validNumberLiteral(s string) bool {
	if s == "" {
		return false
	}
	n, err := numberEnd([]byte(s), 0)
	return err == nil && n == len(s)
}

type RawMessage []byte

func (m RawMessage) MarshalJSON() ([]byte, error) {
	if m == nil {
		return []byte("null"), nil
	}
	return m, nil
}
func (m *RawMessage) UnmarshalJSON(data []byte) error {
	if m == nil {
		return errors.New("fastjson: RawMessage on nil pointer")
	}
	*m = append((*m)[:0], data...)
	return nil
}

type Marshaler interface{ MarshalJSON() ([]byte, error) }
type Unmarshaler interface{ UnmarshalJSON([]byte) error }
type Delim rune
type Token interface{}

var errEOF = io.EOF

type Encoder struct {
	w              io.Writer
	escapeHTML     bool
	prefix, indent string
}

func NewEncoder(w io.Writer) *Encoder              { return &Encoder{w: w} }
func (e *Encoder) SetIndent(prefix, indent string) { e.prefix = prefix; e.indent = indent }
func (e *Encoder) SetEscapeHTML(on bool)           { e.escapeHTML = on }
func (e *Encoder) Encode(v any) error {
	var b []byte
	var err error
	if e.indent != "" {
		b, err = MarshalIndent(v, e.prefix, e.indent)
	} else if e.escapeHTML {
		b, err = MarshalEscape(v)
	} else {
		b, err = Marshal(v)
	}
	if err != nil {
		return err
	}
	b = append(b, '\n')
	_, err = e.w.Write(b)
	return err
}

type Decoder struct {
	data            []byte
	off             int
	useNumber       bool
	disallowUnknown bool
	stack           []byte
}

func NewDecoder(r io.Reader) *Decoder     { b, _ := io.ReadAll(r); return &Decoder{data: b} }
func (d *Decoder) UseNumber()             { d.useNumber = true }
func (d *Decoder) DisallowUnknownFields() { d.disallowUnknown = true }
func (d *Decoder) InputOffset() int64     { return int64(d.off) }
func (d *Decoder) Buffered() io.Reader {
	if d.off >= len(d.data) {
		return bytes.NewReader(nil)
	}
	return bytes.NewReader(d.data[d.off:])
}
func (d *Decoder) Decode(v any) error {
	d.off = skipWS(d.data, d.off)
	if d.off >= len(d.data) {
		return io.EOF
	}
	n, err := valueEnd(d.data, d.off)
	if err != nil {
		return err
	}
	err = UnmarshalOptions(d.data[d.off:n], v, decodeOptions{useNumber: d.useNumber, disallowUnknown: d.disallowUnknown})
	d.off = n
	return err
}
func (d *Decoder) More() bool {
	i := skipWS(d.data, d.off)
	if i >= len(d.data) {
		return false
	}
	return d.data[i] != ']' && d.data[i] != '}'
}
func (d *Decoder) Token() (Token, error) {
	d.off = skipWS(d.data, d.off)
	if d.off >= len(d.data) {
		return nil, io.EOF
	}
	c := d.data[d.off]
	switch c {
	case '{', '[', ']', '}':
		d.off++
		return Delim(c), nil
	case ',':
		d.off++
		return d.Token()
	case ':':
		d.off++
		return d.Token()
	case '"':
		s, n, err := parseString(d.data, d.off)
		if err != nil {
			return nil, err
		}
		d.off = n
		return s, nil
	default:
		n, err := valueEnd(d.data, d.off)
		if err != nil {
			return nil, err
		}
		var x any
		if err := UnmarshalOptions(d.data[d.off:n], &x, decodeOptions{useNumber: d.useNumber}); err != nil {
			return nil, err
		}
		d.off = n
		return x, nil
	}
}

func MarshalIndent(v any, prefix, indent string) ([]byte, error) {
	b, err := Marshal(v)
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err := Indent(&out, b, prefix, indent); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
func MarshalNoEscape(v any) ([]byte, error) {
	buf := make([]byte, 0, 256)
	return appendValue(buf, v, false)
}
func Marshal(v any) ([]byte, error) { buf := make([]byte, 0, 256); return appendValue(buf, v, false) }
func MarshalEscape(v any) ([]byte, error) {
	buf := make([]byte, 0, 256)
	return appendValue(buf, v, true)
}
func MarshalTo(dst []byte, v any) ([]byte, error)         { return appendValue(dst, v, false) }
func MarshalEscapeTo(dst []byte, v any) ([]byte, error)   { return appendValue(dst, v, true) }
func MarshalNoEscapeTo(dst []byte, v any) ([]byte, error) { return appendValue(dst, v, false) }
func Unmarshal(data []byte, v any) error                  { return UnmarshalOptions(data, v, decodeOptions{}) }

type decodeOptions struct{ useNumber, disallowUnknown bool }
