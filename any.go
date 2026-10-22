// Portions of this file are adapted from the Go standard library
// (encoding/json/decode.go). Copyright 2010 The Go Authors.
// Use of this source code is governed by the BSD-style license in LICENSE-GO.

package fastjson

import (
	"encoding"
	"encoding/base64"
	"fmt"
	"reflect"
	"strconv"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"
)

// valueInterface decodes the value at d.off into its generic Go form:
// map[string]any, []any, string, float64 (or Number), bool or nil.
func (d *decodeState) valueInterface() any {
	switch c := d.data[d.off]; c {
	case '{':
		return d.objectInterface()
	case '[':
		return d.arrayInterface()
	case '"':
		return d.readString()
	case 't':
		d.off += 4
		return true
	case 'f':
		d.off += 5
		return false
	case 'n':
		d.off += 4
		return nil
	default:
		v, _ := d.numberInterface()
		return v
	}
}

func (d *decodeState) arrayInterface() []any {
	v := make([]any, 0)
	d.off++
	d.skipWS()
	if d.data[d.off] == ']' {
		d.off++
		return v
	}
	for {
		v = append(v, d.valueInterface())
		d.skipWS()
		if d.data[d.off] == ',' {
			d.off++
			d.skipWS()
			continue
		}
		d.off++
		return v
	}
}

func (d *decodeState) objectInterface() map[string]any {
	m := make(map[string]any)
	d.off++
	d.skipWS()
	if d.data[d.off] == '}' {
		d.off++
		return m
	}
	for {
		key := d.readString()
		d.skipWS()
		d.off++ // ':'
		d.skipWS()
		m[key] = d.valueInterface()
		d.skipWS()
		if d.data[d.off] == ',' {
			d.off++
			d.skipWS()
			continue
		}
		d.off++
		return m
	}
}

// numberInterface decodes the number at d.off as float64, or Number with
// UseNumber. Out-of-range numbers record an error and report false.
func (d *decodeState) numberInterface() (any, bool) {
	start := d.off
	end := numberEnd(d.data, start)
	d.off = end
	lit := d.data[start:end]
	if d.useNumber {
		return Number(lit), true
	}
	f, err := parseFloat(lit, 64)
	if err != nil {
		d.saveNumberError(lit, float64Type, end+1)
		return nil, false
	}
	return f, true
}

const phasePanicMsg = "JSON decoder out of sync - data changing underfoot?"

// literalStore is encoding/json's reflection-based literal decoder. fastjson
// only uses it for the rare cases its compiled decoders do not cover (fields
// with the `string` option and TextUnmarshaler map keys). readIndex is the
// offset reported in type errors.
func (d *decodeState) literalStore(item []byte, v reflect.Value, fromQuoted bool, readIndex int) error {
	// Check for unmarshaler.
	if len(item) == 0 {
		// Empty string given.
		d.saveError(fmt.Errorf("json: invalid use of ,string struct tag, trying to unmarshal %q into %v", item, v.Type()))
		return nil
	}
	isNull := item[0] == 'n' // null
	u, ut, pv := indirect(v, isNull)
	if u != nil {
		return u.UnmarshalJSON(item)
	}
	if ut != nil {
		if item[0] != '"' {
			if fromQuoted {
				d.saveError(fmt.Errorf("json: invalid use of ,string struct tag, trying to unmarshal %q into %v", item, v.Type()))
				return nil
			}
			val := "number"
			switch item[0] {
			case 'n':
				val = "null"
			case 't', 'f':
				val = "bool"
			}
			d.saveTypeError(val, v.Type(), readIndex)
			return nil
		}
		s, ok := unquoteBytes(item)
		if !ok {
			if fromQuoted {
				return fmt.Errorf("json: invalid use of ,string struct tag, trying to unmarshal %q into %v", item, v.Type())
			}
			panic(phasePanicMsg)
		}
		return ut.UnmarshalText(s)
	}

	v = pv

	switch c := item[0]; c {
	case 'n': // null
		// The main parser checks that only true and false can reach here,
		// but if this was a quoted string input, it could be anything.
		if fromQuoted && string(item) != "null" {
			d.saveError(fmt.Errorf("json: invalid use of ,string struct tag, trying to unmarshal %q into %v", item, v.Type()))
			break
		}
		switch v.Kind() {
		case reflect.Interface, reflect.Pointer, reflect.Map, reflect.Slice:
			v.SetZero()
			// otherwise, ignore null for primitives/string
		}
	case 't', 'f': // true, false
		value := item[0] == 't'
		// The main parser checks that only true and false can reach here,
		// but if this was a quoted string input, it could be anything.
		if fromQuoted && string(item) != "true" && string(item) != "false" {
			d.saveError(fmt.Errorf("json: invalid use of ,string struct tag, trying to unmarshal %q into %v", item, v.Type()))
			break
		}
		switch v.Kind() {
		default:
			if fromQuoted {
				d.saveError(fmt.Errorf("json: invalid use of ,string struct tag, trying to unmarshal %q into %v", item, v.Type()))
			} else {
				d.saveTypeError("bool", v.Type(), readIndex)
			}
		case reflect.Bool:
			v.SetBool(value)
		case reflect.Interface:
			if v.NumMethod() == 0 {
				v.Set(reflect.ValueOf(value))
			} else {
				d.saveTypeError("bool", v.Type(), readIndex)
			}
		}

	case '"': // string
		s, ok := unquoteBytes(item)
		if !ok {
			if fromQuoted {
				return fmt.Errorf("json: invalid use of ,string struct tag, trying to unmarshal %q into %v", item, v.Type())
			}
			panic(phasePanicMsg)
		}
		switch v.Kind() {
		default:
			d.saveTypeError("string", v.Type(), readIndex)
		case reflect.Slice:
			if v.Type().Elem().Kind() != reflect.Uint8 {
				d.saveTypeError("string", v.Type(), readIndex)
				break
			}
			b := make([]byte, base64.StdEncoding.DecodedLen(len(s)))
			n, err := base64.StdEncoding.Decode(b, s)
			if err != nil {
				d.saveError(err)
				break
			}
			v.SetBytes(b[:n])
		case reflect.String:
			t := string(s)
			if v.Type() == numberType && !isValidNumber(t) {
				return fmt.Errorf("json: invalid number literal, trying to unmarshal %q into Number", item)
			}
			v.SetString(t)
		case reflect.Interface:
			if v.NumMethod() == 0 {
				v.Set(reflect.ValueOf(string(s)))
			} else {
				d.saveTypeError("string", v.Type(), readIndex)
			}
		}

	default: // number
		if c != '-' && (c < '0' || c > '9') {
			if fromQuoted {
				return fmt.Errorf("json: invalid use of ,string struct tag, trying to unmarshal %q into %v", item, v.Type())
			}
			panic(phasePanicMsg)
		}
		switch v.Kind() {
		default:
			if v.Kind() == reflect.String && v.Type() == numberType {
				// s must be a valid number, because it's
				// already been tokenized.
				v.SetString(string(item))
				break
			}
			if fromQuoted {
				return fmt.Errorf("json: invalid use of ,string struct tag, trying to unmarshal %q into %v", item, v.Type())
			}
			d.saveTypeError("number", v.Type(), readIndex)
		case reflect.Interface:
			n, err := d.convertNumber(string(item), readIndex+1)
			if err != nil {
				d.saveError(err)
				break
			}
			if v.NumMethod() != 0 {
				d.saveTypeError("number", v.Type(), readIndex)
				break
			}
			v.Set(reflect.ValueOf(n))

		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			n, err := strconv.ParseInt(string(item), 10, 64)
			if err != nil || v.OverflowInt(n) {
				d.saveTypeError("number "+string(item), v.Type(), readIndex)
				break
			}
			v.SetInt(n)

		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			n, err := strconv.ParseUint(string(item), 10, 64)
			if err != nil || v.OverflowUint(n) {
				d.saveTypeError("number "+string(item), v.Type(), readIndex)
				break
			}
			v.SetUint(n)

		case reflect.Float32, reflect.Float64:
			n, err := strconv.ParseFloat(string(item), v.Type().Bits())
			if err != nil || v.OverflowFloat(n) {
				d.saveTypeError("number "+string(item), v.Type(), readIndex)
				break
			}
			v.SetFloat(n)
		}
	}
	return nil
}

func (d *decodeState) convertNumber(s string, off int) (any, error) {
	if d.useNumber {
		return Number(s), nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil, &UnmarshalTypeError{Value: "number " + s, Type: float64Type, Offset: int64(off)}
	}
	return f, nil
}

// indirect walks down v allocating pointers as needed, until it gets to a
// non-pointer. If it encounters an Unmarshaler, indirect stops and returns
// that. If decodingNull is true, indirect stops at the first settable
// pointer so it can be set to nil.
func indirect(v reflect.Value, decodingNull bool) (Unmarshaler, encoding.TextUnmarshaler, reflect.Value) {
	v0 := v
	haveAddr := false

	// If v is a named type and is addressable,
	// start with its address, so that if the type has pointer methods,
	// we find them.
	if v.Kind() != reflect.Pointer && v.Type().Name() != "" && v.CanAddr() {
		haveAddr = true
		v = v.Addr()
	}
	for {
		// Load value from interface, but only if the result will be
		// usefully addressable.
		if v.Kind() == reflect.Interface && !v.IsNil() {
			e := v.Elem()
			if e.Kind() == reflect.Pointer && !e.IsNil() && (!decodingNull || e.Elem().Kind() == reflect.Pointer) {
				haveAddr = false
				v = e
				continue
			}
		}

		if v.Kind() != reflect.Pointer {
			break
		}

		if decodingNull && v.CanSet() {
			break
		}

		// Prevent infinite loop if v is an interface pointing to its own address:
		//     var v any
		//     v = &v
		if v.Elem().Kind() == reflect.Interface && v.Elem().Elem().Equal(v) {
			v = v.Elem()
			break
		}
		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}
		if v.Type().NumMethod() > 0 && v.CanInterface() {
			if u, ok := v.Interface().(Unmarshaler); ok {
				return u, nil, reflect.Value{}
			}
			if !decodingNull {
				if u, ok := v.Interface().(encoding.TextUnmarshaler); ok {
					return nil, u, reflect.Value{}
				}
			}
		}

		if haveAddr {
			v = v0 // restore original value after round-trip Value.Addr().Elem()
			haveAddr = false
		} else {
			v = v.Elem()
		}
	}
	return nil, nil, v
}

// unquoteBytes converts a quoted JSON string literal into its content,
// validating escapes. It aliases s when no decoding is needed.
func unquoteBytes(s []byte) (t []byte, ok bool) {
	if len(s) < 2 || s[0] != '"' || s[len(s)-1] != '"' {
		return
	}
	s = s[1 : len(s)-1]

	// Check for unusual characters. If there are none,
	// then no unquoting is needed, so return a slice of the
	// original bytes.
	r := 0
	for r < len(s) {
		c := s[r]
		if c == '\\' || c == '"' || c < ' ' {
			break
		}
		if c < utf8.RuneSelf {
			r++
			continue
		}
		rr, size := utf8.DecodeRune(s[r:])
		if rr == utf8.RuneError && size == 1 {
			break
		}
		r += size
	}
	if r == len(s) {
		return s, true
	}

	b := make([]byte, len(s)+2*utf8.UTFMax)
	w := copy(b, s[0:r])
	for r < len(s) {
		// Out of room? Can only happen if s is full of
		// malformed UTF-8 and we're replacing each
		// byte with RuneError.
		if w >= len(b)-2*utf8.UTFMax {
			nb := make([]byte, (len(b)+utf8.UTFMax)*2)
			copy(nb, b[0:w])
			b = nb
		}
		switch c := s[r]; {
		case c == '\\':
			r++
			if r >= len(s) {
				return
			}
			switch s[r] {
			default:
				return
			case '"', '\\', '/', '\'':
				b[w] = s[r]
				r++
				w++
			case 'b':
				b[w] = '\b'
				r++
				w++
			case 'f':
				b[w] = '\f'
				r++
				w++
			case 'n':
				b[w] = '\n'
				r++
				w++
			case 'r':
				b[w] = '\r'
				r++
				w++
			case 't':
				b[w] = '\t'
				r++
				w++
			case 'u':
				r--
				rr := getu4(s[r:])
				if rr < 0 {
					return
				}
				r += 6
				if utf16.IsSurrogate(rr) {
					rr1 := getu4(s[r:])
					if dec := utf16.DecodeRune(rr, rr1); dec != unicode.ReplacementChar {
						// A valid pair; consume.
						r += 6
						w += utf8.EncodeRune(b[w:], dec)
						break
					}
					// Invalid surrogate; fall back to replacement rune.
					rr = unicode.ReplacementChar
				}
				w += utf8.EncodeRune(b[w:], rr)
			}

		// Quote, control characters are invalid.
		case c == '"', c < ' ':
			return

		// ASCII
		case c < utf8.RuneSelf:
			b[w] = c
			r++
			w++

		// Coerce to well-formed UTF-8.
		default:
			rr, size := utf8.DecodeRune(s[r:])
			r += size
			w += utf8.EncodeRune(b[w:], rr)
		}
	}
	return b[0:w], true
}
