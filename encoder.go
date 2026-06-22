package fastjson

import (
	"encoding"
	"errors"
	"reflect"
	"sort"
	"strconv"
	"sync"
	"unsafe"
)

type encField struct {
	name    string
	quoted  string
	index   []int
	offset  uintptr
	typ     reflect.Type
	kind    reflect.Kind
	omit    bool
	encoder func([]byte, unsafe.Pointer, bool) ([]byte, error)
}
type encPlan struct {
	typ    reflect.Type
	fields []encField
}

var encCache sync.Map
var marshalerType = reflect.TypeOf((*Marshaler)(nil)).Elem()

func appendValue(dst []byte, v any, html bool) ([]byte, error) {
	if v == nil {
		return append(dst, "null"...), nil
	}
	if m, ok := v.(Marshaler); ok {
		b, err := m.MarshalJSON()
		if err != nil {
			return dst, err
		}
		return append(dst, b...), nil
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Struct {
		return appendStructPtr(dst, rv.Type(), ifaceData(v), html)
	}
	if rv.Kind() == reflect.Pointer && !rv.IsNil() && rv.Type().Elem().Kind() == reflect.Struct {
		return appendStructPtr(dst, rv.Type().Elem(), unsafe.Pointer(rv.Pointer()), html)
	}
	return appendReflect(dst, rv, html)
}

type emptyInterface struct {
	typ  unsafe.Pointer
	data unsafe.Pointer
}

func ifaceData(v any) unsafe.Pointer { return (*emptyInterface)(unsafe.Pointer(&v)).data }
func appendReflect(dst []byte, rv reflect.Value, html bool) ([]byte, error) {
	if !rv.IsValid() {
		return append(dst, "null"...), nil
	}
	if rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return append(dst, "null"...), nil
		}
		if rv.CanInterface() {
			if m, ok := rv.Interface().(Marshaler); ok {
				b, e := m.MarshalJSON()
				if e != nil {
					return dst, e
				}
				return append(dst, b...), nil
			}
		}
		rv = rv.Elem()
	}
	if rv.CanInterface() {
		if m, ok := rv.Interface().(Marshaler); ok {
			b, e := m.MarshalJSON()
			if e != nil {
				return dst, e
			}
			return append(dst, b...), nil
		}
		if tm, ok := rv.Interface().(encoding.TextMarshaler); ok {
			b, e := tm.MarshalText()
			if e != nil {
				return dst, e
			}
			return appendString(dst, string(b), html), nil
		}
	}
	switch rv.Kind() {
	case reflect.Bool:
		if rv.Bool() {
			return append(dst, "true"...), nil
		}
		return append(dst, "false"...), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.AppendInt(dst, rv.Int(), 10), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return strconv.AppendUint(dst, rv.Uint(), 10), nil
	case reflect.Float32:
		return strconv.AppendFloat(dst, rv.Float(), 'g', -1, 32), nil
	case reflect.Float64:
		return strconv.AppendFloat(dst, rv.Float(), 'g', -1, 64), nil
	case reflect.String:
		return appendString(dst, rv.String(), html), nil
	case reflect.Struct:
		return appendStruct(dst, rv, html)
	case reflect.Slice, reflect.Array:
		if rv.Kind() == reflect.Slice && rv.IsNil() {
			return append(dst, "null"...), nil
		}
		dst = append(dst, '[')
		for i := 0; i < rv.Len(); i++ {
			if i > 0 {
				dst = append(dst, ',')
			}
			var e error
			dst, e = appendReflect(dst, rv.Index(i), html)
			if e != nil {
				return dst, e
			}
		}
		dst = append(dst, ']')
		return dst, nil
	case reflect.Map:
		return appendMap(dst, rv, html)
	case reflect.Interface:
		if rv.IsNil() {
			return append(dst, "null"...), nil
		}
		return appendReflect(dst, rv.Elem(), html)
	default:
		return dst, errors.New("fastjson: unsupported type " + rv.Type().String())
	}
}
func appendStruct(dst []byte, rv reflect.Value, html bool) ([]byte, error) {
	if rv.CanAddr() {
		return appendStructPtr(dst, rv.Type(), unsafe.Pointer(rv.UnsafeAddr()), html)
	}
	tmp := reflect.New(rv.Type()).Elem()
	tmp.Set(rv)
	return appendStructPtr(dst, rv.Type(), unsafe.Pointer(tmp.UnsafeAddr()), html)
}
func appendStructPtr(dst []byte, t reflect.Type, base unsafe.Pointer, html bool) ([]byte, error) {
	p := getEncPlan(t)
	dst = append(dst, '{')
	var i int
	// find first non-omit field
	for i < len(p.fields) {
		f := &p.fields[i]
		fp := unsafe.Pointer(uintptr(base) + f.offset)
		i++
		if f.omit && isEmptyPtr(fp, f.typ, f.kind) {
			continue
		}
		dst = append(dst, f.quoted...)
		var err error
		dst, err = f.encoder(dst, fp, html)
		if err != nil {
			return dst, err
		}
		break
	}
	for i < len(p.fields) {
		f := &p.fields[i]
		fp := unsafe.Pointer(uintptr(base) + f.offset)
		i++
		if f.omit && isEmptyPtr(fp, f.typ, f.kind) {
			continue
		}
		dst = append(dst, ',')
		dst = append(dst, f.quoted...)
		var err error
		dst, err = f.encoder(dst, fp, html)
		if err != nil {
			return dst, err
		}
	}
	dst = append(dst, '}')
	return dst, nil
}
func appendMap(dst []byte, rv reflect.Value, html bool) ([]byte, error) {
	if rv.IsNil() {
		return append(dst, "null"...), nil
	}
	if rv.Type().Key().Kind() != reflect.String {
		return dst, errors.New("fastjson: only map[string] supported")
	}
	keys := rv.MapKeys()
	sort.Slice(keys, func(i, j int) bool { return keys[i].String() < keys[j].String() })
	dst = append(dst, '{')
	for i, k := range keys {
		if i > 0 {
			dst = append(dst, ',')
		}
		dst = appendString(dst, k.String(), html)
		dst = append(dst, ':')
		var err error
		dst, err = appendReflect(dst, rv.MapIndex(k), html)
		if err != nil {
			return dst, err
		}
	}
	dst = append(dst, '}')
	return dst, nil
}
func getEncPlan(t reflect.Type) *encPlan {
	if p, ok := encCache.Load(t); ok {
		return p.(*encPlan)
	}
	p := buildEncPlan(t)
	actual, _ := encCache.LoadOrStore(t, p)
	return actual.(*encPlan)
}
func buildEncPlan(t reflect.Type) *encPlan {
	p := &encPlan{typ: t}
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		if sf.PkgPath != "" {
			continue
		}
		name, omit, skip := parseTag(sf)
		if skip {
			continue
		}
		f := encField{name: name, quoted: "\"" + name + "\":", index: sf.Index, offset: sf.Offset, typ: sf.Type, kind: sf.Type.Kind(), omit: omit}
		f.encoder = makeFieldEncoder(sf.Type)
		p.fields = append(p.fields, f)
	}
	return p
}
func parseTag(sf reflect.StructField) (string, bool, bool) {
	tag := sf.Tag.Get("json")
	if tag == "-" {
		return "", false, true
	}
	name := sf.Name
	omit := false
	if tag != "" {
		part := tag
		if c := indexByte(part, ','); c >= 0 {
			part = tag[:c]
			opts := tag[c+1:]
			if containsOpt(opts, "omitempty") {
				omit = true
			}
		}
		if part != "" {
			name = part
		}
	}
	return name, omit, false
}
func indexByte(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}
func containsOpt(s, opt string) bool {
	for len(s) > 0 {
		p := s
		if c := indexByte(s, ','); c >= 0 {
			p = s[:c]
			s = s[c+1:]
		} else {
			s = ""
		}
		if p == opt {
			return true
		}
	}
	return false
}
func makeFieldEncoder(t reflect.Type) func([]byte, unsafe.Pointer, bool) ([]byte, error) {
	pt := reflect.PointerTo(t)
	if pt.Implements(marshalerType) {
		return func(d []byte, p unsafe.Pointer, _ bool) ([]byte, error) {
			b, err := reflect.NewAt(t, p).Interface().(Marshaler).MarshalJSON()
			if err != nil {
				return d, err
			}
			return append(d, b...), nil
		}
	}
	k := t.Kind()
	switch k {
	case reflect.Bool:
		return func(d []byte, p unsafe.Pointer, _ bool) ([]byte, error) {
			if *(*bool)(p) {
				return append(d, "true"...), nil
			}
			return append(d, "false"...), nil
		}
	case reflect.Int:
		return func(d []byte, p unsafe.Pointer, _ bool) ([]byte, error) {
			return strconv.AppendInt(d, int64(*(*int)(p)), 10), nil
		}
	case reflect.Int64:
		return func(d []byte, p unsafe.Pointer, _ bool) ([]byte, error) {
			return strconv.AppendInt(d, *(*int64)(p), 10), nil
		}
	case reflect.Int32:
		return func(d []byte, p unsafe.Pointer, _ bool) ([]byte, error) {
			return strconv.AppendInt(d, int64(*(*int32)(p)), 10), nil
		}
	case reflect.Int16:
		return func(d []byte, p unsafe.Pointer, _ bool) ([]byte, error) {
			return strconv.AppendInt(d, int64(*(*int16)(p)), 10), nil
		}
	case reflect.Int8:
		return func(d []byte, p unsafe.Pointer, _ bool) ([]byte, error) {
			return strconv.AppendInt(d, int64(*(*int8)(p)), 10), nil
		}
	case reflect.Uint:
		return func(d []byte, p unsafe.Pointer, _ bool) ([]byte, error) {
			return strconv.AppendUint(d, uint64(*(*uint)(p)), 10), nil
		}
	case reflect.Uint64:
		return func(d []byte, p unsafe.Pointer, _ bool) ([]byte, error) {
			return strconv.AppendUint(d, *(*uint64)(p), 10), nil
		}
	case reflect.Uint32:
		return func(d []byte, p unsafe.Pointer, _ bool) ([]byte, error) {
			return strconv.AppendUint(d, uint64(*(*uint32)(p)), 10), nil
		}
	case reflect.Uint16:
		return func(d []byte, p unsafe.Pointer, _ bool) ([]byte, error) {
			return strconv.AppendUint(d, uint64(*(*uint16)(p)), 10), nil
		}
	case reflect.Uint8:
		return func(d []byte, p unsafe.Pointer, _ bool) ([]byte, error) {
			return strconv.AppendUint(d, uint64(*(*uint8)(p)), 10), nil
		}
	case reflect.Float64:
		return func(d []byte, p unsafe.Pointer, _ bool) ([]byte, error) {
			return strconv.AppendFloat(d, *(*float64)(p), 'g', -1, 64), nil
		}
	case reflect.Float32:
		return func(d []byte, p unsafe.Pointer, _ bool) ([]byte, error) {
			return strconv.AppendFloat(d, float64(*(*float32)(p)), 'g', -1, 32), nil
		}
	case reflect.String:
		return func(d []byte, p unsafe.Pointer, h bool) ([]byte, error) {
			return appendString(d, *(*string)(p), h), nil
		}
	case reflect.Struct:
		return func(d []byte, p unsafe.Pointer, h bool) ([]byte, error) { return appendStructPtr(d, t, p, h) }
	case reflect.Slice:
		if t.Elem().Kind() == reflect.String {
			return func(d []byte, p unsafe.Pointer, h bool) ([]byte, error) {
				s := *(*[]string)(p)
				if s == nil {
					return append(d, "null"...), nil
				}
				d = append(d, '[')
				for i, v := range s {
					if i > 0 {
						d = append(d, ',')
					}
					d = appendString(d, v, h)
				}
				d = append(d, ']')
				return d, nil
			}
		}
		if t.Elem().Kind() == reflect.Int {
			return func(d []byte, p unsafe.Pointer, h bool) ([]byte, error) {
				s := *(*[]int)(p)
				if s == nil {
					return append(d, "null"...), nil
				}
				d = append(d, '[')
				for i, v := range s {
					if i > 0 {
						d = append(d, ',')
					}
					d = strconv.AppendInt(d, int64(v), 10)
				}
				d = append(d, ']')
				return d, nil
			}
		}
		return func(d []byte, p unsafe.Pointer, h bool) ([]byte, error) {
			return appendReflect(d, reflect.NewAt(t, p).Elem(), h)
		}
	case reflect.Array, reflect.Map, reflect.Pointer, reflect.Interface:
		return func(d []byte, p unsafe.Pointer, h bool) ([]byte, error) {
			return appendReflect(d, reflect.NewAt(t, p).Elem(), h)
		}
	default:
		return func(d []byte, p unsafe.Pointer, h bool) ([]byte, error) {
			return appendReflect(d, reflect.NewAt(t, p).Elem(), h)
		}
	}
}
func isEmptyPtr(p unsafe.Pointer, t reflect.Type, k reflect.Kind) bool {
	switch k {
	case reflect.Bool:
		return !*(*bool)(p)
	case reflect.String:
		return *(*string)(p) == ""
	case reflect.Int:
		return *(*int)(p) == 0
	case reflect.Int8:
		return *(*int8)(p) == 0
	case reflect.Int16:
		return *(*int16)(p) == 0
	case reflect.Int32:
		return *(*int32)(p) == 0
	case reflect.Int64:
		return *(*int64)(p) == 0
	case reflect.Uint:
		return *(*uint)(p) == 0
	case reflect.Uint8:
		return *(*uint8)(p) == 0
	case reflect.Uint16:
		return *(*uint16)(p) == 0
	case reflect.Uint32:
		return *(*uint32)(p) == 0
	case reflect.Uint64:
		return *(*uint64)(p) == 0
	case reflect.Uintptr:
		return *(*uintptr)(p) == 0
	case reflect.Float32:
		return *(*float32)(p) == 0
	case reflect.Float64:
		return *(*float64)(p) == 0
	case reflect.Slice, reflect.Map, reflect.Pointer, reflect.Interface:
		return reflect.NewAt(t, p).Elem().IsNil()
	}
	return false
}
