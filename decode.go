package fastjson

import (
	"errors"
	"reflect"
	"strconv"
	"sync"
	"unsafe"
)

type decField struct {
	name   string
	offset uintptr
	typ    reflect.Type
	kind   reflect.Kind
	set    func([]byte, int, unsafe.Pointer, decodeOptions) (int, error)
}
type decPlan struct {
	typ    reflect.Type
	fields map[string]*decField
	list   []*decField
	byLen  [][]*decField
}

var decCache sync.Map
var unmarshalerType = reflect.TypeOf((*Unmarshaler)(nil)).Elem()

func UnmarshalOptions(data []byte, v any, opt decodeOptions) error {
	if v == nil {
		return errors.New("fastjson: Unmarshal(nil)")
	}
	i := skipWS(data, 0)
	if i >= len(data) {
		return syntaxErr("empty input", 0)
	}
	if u, ok := v.(Unmarshaler); ok {
		return u.UnmarshalJSON(data)
	}
	t := reflect.TypeOf(v)
	if t == nil || t.Kind() != reflect.Pointer {
		return errors.New("fastjson: Unmarshal(non-pointer)")
	}
	base := ifaceData(v)
	if base == nil {
		return errors.New("fastjson: Unmarshal(nil pointer)")
	}
	var n int
	var err error
	if t.Elem().Kind() == reflect.Struct {
		n, err = decodeStructPtr(data, i, t.Elem(), base, opt)
	} else {
		rv := reflect.ValueOf(v)
		if rv.IsNil() {
			return errors.New("fastjson: Unmarshal(nil pointer)")
		}
		n, err = decodeReflect(data, i, rv.Elem(), opt)
	}
	if err != nil {
		return err
	}
	if skipWS(data, n) != len(data) {
		return syntaxErr("trailing data", n)
	}
	return nil
}
func decodeReflect(b []byte, off int, rv reflect.Value, opt decodeOptions) (int, error) {
	if !rv.CanSet() {
		return off, errors.New("fastjson: cannot set " + rv.Type().String())
	}
	i := skipWS(b, off)
	if i >= len(b) {
		return i, syntaxErr("unexpected eof", i)
	}
	if rv.Kind() == reflect.Pointer {
		if hasLit(b, i, "null") {
			rv.SetZero()
			return i + 4, nil
		}
		if rv.IsNil() {
			rv.Set(reflect.New(rv.Type().Elem()))
		}
		return decodeReflect(b, i, rv.Elem(), opt)
	}
	switch rv.Kind() {
	case reflect.Struct:
		n, err := decodeStructPtr(b, i, rv.Type(), unsafe.Pointer(rv.UnsafeAddr()), opt)
		if err != nil {
			return i, err
		}
		return n, nil
	case reflect.String:
		s, n, err := parseStringZC(b, i)
		if err != nil {
			return i, err
		}
		rv.SetString(s)
		return n, nil
	case reflect.Bool:
		if hasLit(b, i, "true") {
			rv.SetBool(true)
			return i + 4, nil
		}
		if hasLit(b, i, "false") {
			rv.SetBool(false)
			return i + 5, nil
		}
		return i, syntaxErr("expected bool", i)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := numberEnd(b, i)
		if err != nil {
			return i, err
		}
		x, err := strconv.ParseInt(unsafeString(b[i:n]), 10, rv.Type().Bits())
		if err != nil {
			return i, err
		}
		rv.SetInt(x)
		return n, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		n, err := numberEnd(b, i)
		if err != nil {
			return i, err
		}
		x, err := strconv.ParseUint(unsafeString(b[i:n]), 10, rv.Type().Bits())
		if err != nil {
			return i, err
		}
		rv.SetUint(x)
		return n, nil
	case reflect.Float32, reflect.Float64:
		n, err := numberEnd(b, i)
		if err != nil {
			return i, err
		}
		x, err := strconv.ParseFloat(unsafeString(b[i:n]), rv.Type().Bits())
		if err != nil {
			return i, err
		}
		rv.SetFloat(x)
		return n, nil
	case reflect.Slice:
		return decodeSlice(b, i, rv, opt)
	case reflect.Map:
		return decodeMap(b, i, rv, opt)
	case reflect.Interface:
		val, err := parseAny(b[i:], opt)
		if err != nil {
			return i, err
		}
		n, _ := valueEnd(b, i)
		if val == nil {
			rv.SetZero()
		} else {
			rv.Set(reflect.ValueOf(val))
		}
		return n, nil
	default:
		return i, errors.New("fastjson: unsupported decode type " + rv.Type().String())
	}
}
func decodeStructPtr(b []byte, off int, t reflect.Type, base unsafe.Pointer, opt decodeOptions) (int, error) {
	i := skipWS(b, off)
	if i >= len(b) || b[i] != '{' {
		return i, syntaxErr("expected object", i)
	}
	i++
	p := getDecPlan(t)
	i = skipWS(b, i)
	if i < len(b) && b[i] == '}' {
		return i + 1, nil
	}
	byLen := p.byLen
	for {
		if i >= len(b) || b[i] != '"' {
			return i, syntaxErr("expected object key", i)
		}
		ks := i + 1
		j := ks
		simple := true
		for j < len(b) && b[j] != '"' {
			if b[j] == '\\' || b[j] < 0x20 {
				simple = false
				break
			}
			j++
		}
		var f *decField
		var key string
		if simple && j < len(b) {
			keyLen := j - ks
			if keyLen < len(byLen) {
				for _, cand := range byLen[keyLen] {
					if bytesEqString(b[ks:j], cand.name) {
						f = cand
						break
					}
				}
			}
			i = j + 1
		} else {
			var n int
			var err error
			key, n, err = parseString(b, i)
			if err != nil {
				return i, err
			}
			f = p.fields[key]
			i = n
		}
		i = skipWS(b, i)
		if i >= len(b) || b[i] != ':' {
			return i, syntaxErr("expected colon", i)
		}
		i = skipWS(b, i+1)
		if f != nil {
			var err error
			i, err = f.set(b, i, unsafe.Pointer(uintptr(base)+f.offset), opt)
			if err != nil {
				return i, err
			}
		} else {
			end, err := valueEnd(b, i)
			if err != nil {
				return i, err
			}
			if opt.disallowUnknown {
				if key == "" {
					key = string(b[ks:j])
				}
				return i, errors.New("fastjson: unknown field " + key)
			}
			i = end
		}
		i = skipWS(b, i)
		if i < len(b) && b[i] == '}' {
			return i + 1, nil
		}
		if i >= len(b) || b[i] != ',' {
			return i, syntaxErr("expected comma", i)
		}
		i = skipWS(b, i+1)
	}
}
func getDecPlan(t reflect.Type) *decPlan {
	if p, ok := decCache.Load(t); ok {
		return p.(*decPlan)
	}
	p := &decPlan{typ: t, fields: map[string]*decField{}}
	ep := getEncPlan(t)
	maxLen := 0
	for _, ef := range ep.fields {
		f := &decField{name: ef.name, offset: ef.offset, typ: ef.typ, kind: ef.kind}
		f.set = makeSetter(ef.typ)
		p.fields[f.name] = f
		p.list = append(p.list, f)
		if l := len(ef.name); l > maxLen {
			maxLen = l
		}
	}
	p.byLen = make([][]*decField, maxLen+1)
	for _, f := range p.list {
		l := len(f.name)
		p.byLen[l] = append(p.byLen[l], f)
	}
	actual, _ := decCache.LoadOrStore(t, p)
	return actual.(*decPlan)
}
func makeSetter(t reflect.Type) func([]byte, int, unsafe.Pointer, decodeOptions) (int, error) {
	pt := reflect.PointerTo(t)
	if pt.Implements(unmarshalerType) {
		return func(b []byte, off int, p unsafe.Pointer, _ decodeOptions) (int, error) {
			end, err := valueEnd(b, off)
			if err != nil {
				return off, err
			}
			return end, reflect.NewAt(t, p).Interface().(Unmarshaler).UnmarshalJSON(b[off:end])
		}
	}
	k := t.Kind()
	switch k {
	case reflect.String:
		return func(b []byte, off int, p unsafe.Pointer, _ decodeOptions) (int, error) {
			s, n, err := parseStringZC(b, off)
			if err == nil {
				*(*string)(p) = s
			}
			return n, err
		}
	case reflect.Bool:
		return func(b []byte, off int, p unsafe.Pointer, _ decodeOptions) (int, error) {
			i := skipWS(b, off)
			if hasLit(b, i, "true") {
				*(*bool)(p) = true
				return i + 4, nil
			}
			if hasLit(b, i, "false") {
				*(*bool)(p) = false
				return i + 5, nil
			}
			return i, syntaxErr("expected bool", i)
		}
	case reflect.Int:
		return func(b []byte, off int, p unsafe.Pointer, _ decodeOptions) (int, error) {
			i := skipWS(b, off)
			n, e := numberEnd(b, i)
			if e != nil {
				return i, e
			}
			x, e := parseIntBytes(b[i:n])
			*(*int)(p) = int(x)
			return n, e
		}
	case reflect.Int64:
		return func(b []byte, off int, p unsafe.Pointer, _ decodeOptions) (int, error) {
			i := skipWS(b, off)
			n, e := numberEnd(b, i)
			if e != nil {
				return i, e
			}
			x, e := parseIntBytes(b[i:n])
			*(*int64)(p) = x
			return n, e
		}
	case reflect.Int32:
		return func(b []byte, off int, p unsafe.Pointer, _ decodeOptions) (int, error) {
			i := skipWS(b, off)
			n, e := numberEnd(b, i)
			if e != nil {
				return i, e
			}
			x, e := parseIntBytes(b[i:n])
			*(*int32)(p) = int32(x)
			return n, e
		}
	case reflect.Int16:
		return func(b []byte, off int, p unsafe.Pointer, _ decodeOptions) (int, error) {
			i := skipWS(b, off)
			n, e := numberEnd(b, i)
			if e != nil {
				return i, e
			}
			x, e := parseIntBytes(b[i:n])
			*(*int16)(p) = int16(x)
			return n, e
		}
	case reflect.Int8:
		return func(b []byte, off int, p unsafe.Pointer, _ decodeOptions) (int, error) {
			i := skipWS(b, off)
			n, e := numberEnd(b, i)
			if e != nil {
				return i, e
			}
			x, e := parseIntBytes(b[i:n])
			*(*int8)(p) = int8(x)
			return n, e
		}
	case reflect.Float64:
		return func(b []byte, off int, p unsafe.Pointer, _ decodeOptions) (int, error) {
			i := skipWS(b, off)
			n, e := numberEnd(b, i)
			if e != nil {
				return i, e
			}
			x, e := strconv.ParseFloat(unsafeString(b[i:n]), 64)
			*(*float64)(p) = x
			return n, e
		}
	case reflect.Float32:
		return func(b []byte, off int, p unsafe.Pointer, _ decodeOptions) (int, error) {
			i := skipWS(b, off)
			n, e := numberEnd(b, i)
			if e != nil {
				return i, e
			}
			x, e := strconv.ParseFloat(unsafeString(b[i:n]), 32)
			*(*float32)(p) = float32(x)
			return n, e
		}
	case reflect.Uint:
		return func(b []byte, off int, p unsafe.Pointer, _ decodeOptions) (int, error) {
			i := skipWS(b, off)
			n, e := numberEnd(b, i)
			if e != nil {
				return i, e
			}
			x, e := parseUintBytes(b[i:n])
			*(*uint)(p) = uint(x)
			return n, e
		}
	case reflect.Uint8:
		return func(b []byte, off int, p unsafe.Pointer, _ decodeOptions) (int, error) {
			i := skipWS(b, off)
			n, e := numberEnd(b, i)
			if e != nil {
				return i, e
			}
			x, e := parseUintBytes(b[i:n])
			*(*uint8)(p) = uint8(x)
			return n, e
		}
	case reflect.Uint16:
		return func(b []byte, off int, p unsafe.Pointer, _ decodeOptions) (int, error) {
			i := skipWS(b, off)
			n, e := numberEnd(b, i)
			if e != nil {
				return i, e
			}
			x, e := parseUintBytes(b[i:n])
			*(*uint16)(p) = uint16(x)
			return n, e
		}
	case reflect.Uint32:
		return func(b []byte, off int, p unsafe.Pointer, _ decodeOptions) (int, error) {
			i := skipWS(b, off)
			n, e := numberEnd(b, i)
			if e != nil {
				return i, e
			}
			x, e := parseUintBytes(b[i:n])
			*(*uint32)(p) = uint32(x)
			return n, e
		}
	case reflect.Uint64:
		return func(b []byte, off int, p unsafe.Pointer, _ decodeOptions) (int, error) {
			i := skipWS(b, off)
			n, e := numberEnd(b, i)
			if e != nil {
				return i, e
			}
			x, e := parseUintBytes(b[i:n])
			*(*uint64)(p) = x
			return n, e
		}
	case reflect.Uintptr:
		return func(b []byte, off int, p unsafe.Pointer, _ decodeOptions) (int, error) {
			i := skipWS(b, off)
			n, e := numberEnd(b, i)
			if e != nil {
				return i, e
			}
			x, e := parseUintBytes(b[i:n])
			*(*uintptr)(p) = uintptr(x)
			return n, e
		}
	case reflect.Slice:
		if t.Elem().Kind() == reflect.String {
			return func(b []byte, off int, p unsafe.Pointer, opt decodeOptions) (int, error) {
				return decodeStringSliceFastInPlace(b, off, p)
			}
		}
		if t.Elem().Kind() == reflect.Int {
			return func(b []byte, off int, p unsafe.Pointer, opt decodeOptions) (int, error) {
				return decodeIntSliceFastInPlace(b, off, p)
			}
		}
		return func(b []byte, off int, p unsafe.Pointer, opt decodeOptions) (int, error) {
			end, err := valueEnd(b, off)
			if err != nil {
				return off, err
			}
			_, err = decodeReflect(b[off:end], 0, reflect.NewAt(t, p).Elem(), opt)
			return end, err
		}
	case reflect.Struct:
		return func(b []byte, off int, p unsafe.Pointer, opt decodeOptions) (int, error) {
			return decodeStructPtr(b, off, t, p, opt)
		}
	default:
		return func(b []byte, off int, p unsafe.Pointer, opt decodeOptions) (int, error) {
			end, err := valueEnd(b, off)
			if err != nil {
				return off, err
			}
			_, err = decodeReflect(b[off:end], 0, reflect.NewAt(t, p).Elem(), opt)
			return end, err
		}
	}
}
func decodeSlice(b []byte, off int, rv reflect.Value, opt decodeOptions) (int, error) {
	i := skipWS(b, off)
	if hasLit(b, i, "null") {
		rv.SetZero()
		return i + 4, nil
	}
	if i >= len(b) || b[i] != '[' {
		return i, syntaxErr("expected array", i)
	}
	i++
	et := rv.Type().Elem()
	out := reflect.MakeSlice(rv.Type(), 0, 0)
	i = skipWS(b, i)
	if i < len(b) && b[i] == ']' {
		rv.Set(out)
		return i + 1, nil
	}
	for {
		var end int
		var err error
		end, err = valueEnd(b, i)
		if err != nil {
			return i, err
		}
		ev := reflect.New(et).Elem()
		if _, err := decodeReflect(b, i, ev, opt); err != nil {
			return i, err
		}
		out = reflect.Append(out, ev)
		i = skipWS(b, end)
		if i < len(b) && b[i] == ']' {
			rv.Set(out)
			return i + 1, nil
		}
		if i >= len(b) || b[i] != ',' {
			return i, syntaxErr("expected comma", i)
		}
		i = skipWS(b, i+1)
	}
}
func decodeMap(b []byte, off int, rv reflect.Value, opt decodeOptions) (int, error) {
	i := skipWS(b, off)
	if hasLit(b, i, "null") {
		rv.SetZero()
		return i + 4, nil
	}
	if rv.Type().Key().Kind() != reflect.String {
		return i, errors.New("fastjson: map key must be string")
	}
	if i >= len(b) || b[i] != '{' {
		return i, syntaxErr("expected object", i)
	}
	i++
	m := reflect.MakeMap(rv.Type())
	et := rv.Type().Elem()
	i = skipWS(b, i)
	if i < len(b) && b[i] == '}' {
		rv.Set(m)
		return i + 1, nil
	}
	for {
		key, n, err := parseString(b, i)
		if err != nil {
			return i, err
		}
		i = skipWS(b, n)
		if i >= len(b) || b[i] != ':' {
			return i, syntaxErr("expected colon", i)
		}
		i = skipWS(b, i+1)
		end, err := valueEnd(b, i)
		if err != nil {
			return i, err
		}
		ev := reflect.New(et).Elem()
		if _, err := decodeReflect(b, i, ev, opt); err != nil {
			return i, err
		}
		m.SetMapIndex(reflect.ValueOf(key), ev)
		i = skipWS(b, end)
		if i < len(b) && b[i] == '}' {
			rv.Set(m)
			return i + 1, nil
		}
		if i >= len(b) || b[i] != ',' {
			return i, syntaxErr("expected comma", i)
		}
		i = skipWS(b, i+1)
	}
}

// hasLit checks if the bytes at b[i:] match string s using direct byte comparison.
func hasLit(b []byte, i int, s string) bool {
	if i+len(s) > len(b) {
		return false
	}
	for j := 0; j < len(s); j++ {
		if b[i+j] != s[j] {
			return false
		}
	}
	return true
}
func stringTrimEq(b []byte, s string) bool {
	i := skipWS(b, 0)
	return i+len(s) == skipWS(b, i+len(s)) && hasLit(b, i, s)
}
func decodeStringSliceFastInPlace(b []byte, off int, p unsafe.Pointer) (int, error) {
	i := skipWS(b, off)
	if hasLit(b, i, "null") {
		*(*[]string)(p) = nil
		return i + 4, nil
	}
	if i >= len(b) || b[i] != '[' {
		return i, syntaxErr("expected array", i)
	}
	i = skipWS(b, i+1)
	out := make([]string, 0, 4)
	if i < len(b) && b[i] == ']' {
		*(*[]string)(p) = out
		return i + 1, nil
	}
	for {
		s, n, e := parseStringZC(b, i)
		if e != nil {
			return i, e
		}
		out = append(out, s)
		i = skipWS(b, n)
		if i < len(b) && b[i] == ']' {
			*(*[]string)(p) = out
			return i + 1, nil
		}
		if i >= len(b) || b[i] != ',' {
			return i, syntaxErr("expected comma", i)
		}
		i = skipWS(b, i+1)
	}
}
func decodeIntSliceFastInPlace(b []byte, off int, p unsafe.Pointer) (int, error) {
	i := skipWS(b, off)
	if hasLit(b, i, "null") {
		*(*[]int)(p) = nil
		return i + 4, nil
	}
	if i >= len(b) || b[i] != '[' {
		return i, syntaxErr("expected array", i)
	}
	i = skipWS(b, i+1)
	out := make([]int, 0, 8)
	if i < len(b) && b[i] == ']' {
		*(*[]int)(p) = out
		return i + 1, nil
	}
	for {
		end, e := numberEnd(b, i)
		if e != nil {
			return i, e
		}
		x, e := parseIntBytes(b[i:end])
		if e != nil {
			return i, e
		}
		out = append(out, int(x))
		i = skipWS(b, end)
		if i < len(b) && b[i] == ']' {
			*(*[]int)(p) = out
			return i + 1, nil
		}
		if i >= len(b) || b[i] != ',' {
			return i, syntaxErr("expected comma", i)
		}
		i = skipWS(b, i+1)
	}
}
