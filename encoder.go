package fastjson

import (
	"encoding"
	"encoding/base64"
	"fmt"
	"math"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"unsafe"
)

// Marshal returns the JSON encoding of v, byte-for-byte identical to
// encoding/json.Marshal.
func Marshal(v any) ([]byte, error) {
	e := getEncodeState()
	err := e.marshal(v, flagHTML)
	var buf []byte
	if err == nil {
		buf = append(make([]byte, 0, len(e.buf)), e.buf...)
	}
	putEncodeState(e)
	return buf, err
}

// MarshalIndent is like Marshal but applies Indent to format the output.
func MarshalIndent(v any, prefix, indent string) ([]byte, error) {
	e := getEncodeState()
	defer putEncodeState(e)
	if err := e.marshal(v, flagHTML); err != nil {
		return nil, err
	}
	return appendIndentValid(make([]byte, 0, 2*len(e.buf)), e.buf, prefix, indent), nil
}

// MarshalNoEscape is like Marshal but does not escape <, > and & in strings,
// matching Encoder.SetEscapeHTML(false).
func MarshalNoEscape(v any) ([]byte, error) {
	e := getEncodeState()
	err := e.marshal(v, 0)
	var buf []byte
	if err == nil {
		buf = append(make([]byte, 0, len(e.buf)), e.buf...)
	}
	putEncodeState(e)
	return buf, err
}

// MarshalEscape is Marshal; it is kept for compatibility with earlier
// fastjson releases, where Marshal did not escape HTML.
func MarshalEscape(v any) ([]byte, error) { return Marshal(v) }

// MarshalTo appends the JSON encoding of v to dst and returns the extended
// buffer. It escapes HTML like Marshal.
func MarshalTo(dst []byte, v any) ([]byte, error) { return marshalTo(dst, v, flagHTML) }

// MarshalEscapeTo is MarshalTo.
func MarshalEscapeTo(dst []byte, v any) ([]byte, error) { return marshalTo(dst, v, flagHTML) }

// MarshalNoEscapeTo is like MarshalTo but does not escape HTML.
func MarshalNoEscapeTo(dst []byte, v any) ([]byte, error) { return marshalTo(dst, v, 0) }

func marshalTo(dst []byte, v any, fl encFlags) ([]byte, error) {
	e := getEncodeState()
	e.buf = dst
	err := e.marshal(v, fl)
	out := e.buf
	e.buf = nil
	putEncodeState(e)
	if err != nil {
		return dst, err
	}
	return out, nil
}

type encFlags uint8

const (
	flagHTML   encFlags = 1 << iota // escape <, > and &
	flagQuoted                      // `string` tag option
	flagAddr                        // value is addressable
)

type encoderFunc func(e *encodeState, p unsafe.Pointer, fl encFlags) error

type encodeState struct {
	buf      []byte
	ptrLevel uint
	ptrSeen  map[any]struct{}
	// words holds pointer-shaped interface payloads so encoders can be
	// given their address without allocating.
	words  [16]unsafe.Pointer
	nwords int
}

const startDetectingCyclesAfter = 1000

var encodeStatePool sync.Pool

func getEncodeState() *encodeState {
	if v := encodeStatePool.Get(); v != nil {
		e := v.(*encodeState)
		e.buf = e.buf[:0]
		return e
	}
	return &encodeState{buf: make([]byte, 0, 1024)}
}

func putEncodeState(e *encodeState) {
	if cap(e.buf) > 1<<20 {
		e.buf = nil
	}
	e.ptrLevel = 0
	clear(e.ptrSeen)
	for i := range e.nwords {
		e.words[i] = nil
	}
	e.nwords = 0
	encodeStatePool.Put(e)
}

func (e *encodeState) marshal(v any, fl encFlags) error {
	if v == nil {
		e.buf = append(e.buf, "null"...)
		return nil
	}
	return e.encodeIface(v, fl)
}

type ifaceEncoder struct {
	enc    encoderFunc
	direct bool
}

var ifaceEncoderCache sync.Map // runtime type pointer -> *ifaceEncoder

// encodeIface encodes the dynamic value held by v (which is not nil). The
// value is not addressable.
func (e *encodeState) encodeIface(v any, fl encFlags) error {
	fl &^= flagAddr
	html := fl&flagHTML != 0
	switch x := v.(type) {
	case string:
		e.buf = appendString(e.buf, x, html)
		return nil
	case float64:
		if math.IsInf(x, 0) || math.IsNaN(x) {
			return &UnsupportedValueError{Value: reflect.ValueOf(x), Str: strconv.FormatFloat(x, 'g', -1, 64)}
		}
		e.buf = appendFloat(e.buf, x, 64)
		return nil
	case bool:
		e.buf = strconv.AppendBool(e.buf, x)
		return nil
	case int:
		e.buf = strconv.AppendInt(e.buf, int64(x), 10)
		return nil
	case int64:
		e.buf = strconv.AppendInt(e.buf, x, 10)
		return nil
	case map[string]any:
		return e.encodeMapStringAny(x, fl)
	case []any:
		return e.encodeSliceAny(x, fl)
	}
	ef := efaceOf(&v)
	var ie *ifaceEncoder
	if c, ok := ifaceEncoderCache.Load(ef.typ); ok {
		ie = c.(*ifaceEncoder)
	} else {
		t := reflect.TypeOf(v)
		ie = &ifaceEncoder{enc: typeEncoder(t), direct: isDirectIface(t)}
		ifaceEncoderCache.Store(ef.typ, ie)
	}
	if !ie.direct {
		return ie.enc(e, ef.data, fl)
	}
	if e.nwords < len(e.words) {
		slot := &e.words[e.nwords]
		*slot = ef.data
		e.nwords++
		err := ie.enc(e, unsafe.Pointer(slot), fl)
		e.nwords--
		*slot = nil
		return err
	}
	slot := new(unsafe.Pointer)
	*slot = ef.data
	return ie.enc(e, unsafe.Pointer(slot), fl)
}

var encoderCache sync.Map // reflect.Type -> encoderFunc

func typeEncoder(t reflect.Type) encoderFunc {
	if fi, ok := encoderCache.Load(t); ok {
		return fi.(encoderFunc)
	}
	// To deal with recursive types, populate the map with an indirect func
	// before we build it. This type waits on the real func to be ready.
	var (
		wg sync.WaitGroup
		f  encoderFunc
	)
	wg.Add(1)
	fi, loaded := encoderCache.LoadOrStore(t, encoderFunc(func(e *encodeState, p unsafe.Pointer, fl encFlags) error {
		wg.Wait()
		return f(e, p, fl)
	}))
	if loaded {
		return fi.(encoderFunc)
	}
	f = newTypeEncoder(t, true)
	wg.Done()
	encoderCache.Store(t, f)
	return f
}

var (
	marshalerType     = reflect.TypeFor[Marshaler]()
	textMarshalerType = reflect.TypeFor[encoding.TextMarshaler]()
	rawMessageType    = reflect.TypeFor[RawMessage]()
)

// hasMarshalMethods reports whether t or *t has MarshalJSON or MarshalText.
func hasMarshalMethods(t reflect.Type) bool {
	pt := reflect.PointerTo(t)
	return pt.Implements(marshalerType) || pt.Implements(textMarshalerType) ||
		t.Implements(marshalerType) || t.Implements(textMarshalerType)
}

func newTypeEncoder(t reflect.Type, allowAddr bool) encoderFunc {
	if t == rawMessageType {
		return encodeRawMessage
	}
	if t.Kind() != reflect.Pointer && allowAddr && reflect.PointerTo(t).Implements(marshalerType) {
		return condAddrEncoder(addrMarshalerEncoder(t), newTypeEncoder(t, false))
	}
	if t.Implements(marshalerType) {
		return marshalerEncoder(t)
	}
	if t.Kind() != reflect.Pointer && allowAddr && reflect.PointerTo(t).Implements(textMarshalerType) {
		return condAddrEncoder(addrTextMarshalerEncoder(t), newTypeEncoder(t, false))
	}
	if t.Implements(textMarshalerType) {
		return textMarshalerEncoder(t)
	}

	switch t.Kind() {
	case reflect.Bool:
		return encodeBool
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return intEncoder(t.Size())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return uintEncoder(t.Size())
	case reflect.Float32:
		return floatEncoder(t, 32)
	case reflect.Float64:
		return floatEncoder(t, 64)
	case reflect.String:
		if t == numberType {
			return encodeNumber
		}
		return encodeString
	case reflect.Interface:
		return interfaceEncoder(t)
	case reflect.Struct:
		return newStructEncoder(t)
	case reflect.Map:
		return newMapEncoder(t)
	case reflect.Slice:
		return newSliceEncoder(t)
	case reflect.Array:
		return newArrayEncoder(t)
	case reflect.Pointer:
		return newPtrEncoder(t)
	default:
		return unsupportedTypeEncoder(t)
	}
}

func condAddrEncoder(canAddrEnc, elseEnc encoderFunc) encoderFunc {
	return func(e *encodeState, p unsafe.Pointer, fl encFlags) error {
		if fl&flagAddr != 0 {
			return canAddrEnc(e, p, fl)
		}
		return elseEnc(e, p, fl)
	}
}

func unsupportedTypeEncoder(t reflect.Type) encoderFunc {
	return func(e *encodeState, p unsafe.Pointer, fl encFlags) error {
		return &UnsupportedTypeError{Type: t}
	}
}

// valueIface returns the value of type t at p as an interface, without
// copying. The result must only be used transiently.
func valueIface(t reflect.Type) func(p unsafe.Pointer) any {
	rt, direct := rtypeOf(t), isDirectIface(t)
	if t.Kind() == reflect.Interface {
		return func(p unsafe.Pointer) any { return reflect.NewAt(t, p).Elem().Interface() }
	}
	return func(p unsafe.Pointer) any { return packEface(rt, direct, p) }
}

func (e *encodeState) appendMarshalJSON(t reflect.Type, m Marshaler, fl encFlags) error {
	b, err := m.MarshalJSON()
	if err == nil {
		e.buf, err = appendCompact(e.buf, b, fl&flagHTML != 0)
	}
	if err != nil {
		return newMarshalerError(t, err, "MarshalJSON")
	}
	return nil
}

func encodeRawMessage(e *encodeState, p unsafe.Pointer, fl encFlags) error {
	b := *(*[]byte)(p)
	if b == nil {
		e.buf = append(e.buf, "null"...)
		return nil
	}
	var err error
	if e.buf, err = appendCompact(e.buf, b, fl&flagHTML != 0); err != nil {
		return newMarshalerError(rawMessageType, err, "MarshalJSON")
	}
	return nil
}

func marshalerEncoder(t reflect.Type) encoderFunc {
	iface := valueIface(t)
	kind := t.Kind()
	return func(e *encodeState, p unsafe.Pointer, fl encFlags) error {
		if (kind == reflect.Pointer || kind == reflect.Interface) && *(*unsafe.Pointer)(p) == nil {
			e.buf = append(e.buf, "null"...)
			return nil
		}
		m, ok := iface(p).(Marshaler)
		if !ok {
			e.buf = append(e.buf, "null"...)
			return nil
		}
		return e.appendMarshalJSON(t, m, fl)
	}
}

func addrMarshalerEncoder(t reflect.Type) encoderFunc {
	rt := rtypeOf(reflect.PointerTo(t))
	return func(e *encodeState, p unsafe.Pointer, fl encFlags) error {
		return e.appendMarshalJSON(t, ptrIface(rt, p).(Marshaler), fl)
	}
}

func (e *encodeState) appendMarshalText(t reflect.Type, m encoding.TextMarshaler, fl encFlags) error {
	b, err := m.MarshalText()
	if err != nil {
		return newMarshalerError(t, err, "MarshalText")
	}
	e.buf = appendString(e.buf, unsafeString(b), fl&flagHTML != 0)
	return nil
}

func textMarshalerEncoder(t reflect.Type) encoderFunc {
	iface := valueIface(t)
	kind := t.Kind()
	return func(e *encodeState, p unsafe.Pointer, fl encFlags) error {
		if (kind == reflect.Pointer || kind == reflect.Interface) && *(*unsafe.Pointer)(p) == nil {
			e.buf = append(e.buf, "null"...)
			return nil
		}
		m, ok := iface(p).(encoding.TextMarshaler)
		if !ok {
			e.buf = append(e.buf, "null"...)
			return nil
		}
		return e.appendMarshalText(t, m, fl)
	}
}

func addrTextMarshalerEncoder(t reflect.Type) encoderFunc {
	rt := rtypeOf(reflect.PointerTo(t))
	return func(e *encodeState, p unsafe.Pointer, fl encFlags) error {
		return e.appendMarshalText(t, ptrIface(rt, p).(encoding.TextMarshaler), fl)
	}
}

func encodeBool(e *encodeState, p unsafe.Pointer, fl encFlags) error {
	if fl&flagQuoted != 0 {
		e.buf = append(e.buf, '"')
		e.buf = strconv.AppendBool(e.buf, *(*bool)(p))
		e.buf = append(e.buf, '"')
		return nil
	}
	e.buf = strconv.AppendBool(e.buf, *(*bool)(p))
	return nil
}

func loadInt(p unsafe.Pointer, size uintptr) int64 {
	switch size {
	case 8:
		return *(*int64)(p)
	case 4:
		return int64(*(*int32)(p))
	case 2:
		return int64(*(*int16)(p))
	default:
		return int64(*(*int8)(p))
	}
}

func loadUint(p unsafe.Pointer, size uintptr) uint64 {
	switch size {
	case 8:
		return *(*uint64)(p)
	case 4:
		return uint64(*(*uint32)(p))
	case 2:
		return uint64(*(*uint16)(p))
	default:
		return uint64(*(*uint8)(p))
	}
}

func intEncoder(size uintptr) encoderFunc {
	return func(e *encodeState, p unsafe.Pointer, fl encFlags) error {
		if fl&flagQuoted != 0 {
			e.buf = append(e.buf, '"')
			e.buf = strconv.AppendInt(e.buf, loadInt(p, size), 10)
			e.buf = append(e.buf, '"')
			return nil
		}
		e.buf = strconv.AppendInt(e.buf, loadInt(p, size), 10)
		return nil
	}
}

func uintEncoder(size uintptr) encoderFunc {
	return func(e *encodeState, p unsafe.Pointer, fl encFlags) error {
		if fl&flagQuoted != 0 {
			e.buf = append(e.buf, '"')
			e.buf = strconv.AppendUint(e.buf, loadUint(p, size), 10)
			e.buf = append(e.buf, '"')
			return nil
		}
		e.buf = strconv.AppendUint(e.buf, loadUint(p, size), 10)
		return nil
	}
}

func floatEncoder(t reflect.Type, bits int) encoderFunc {
	return func(e *encodeState, p unsafe.Pointer, fl encFlags) error {
		var f float64
		if bits == 32 {
			f = float64(*(*float32)(p))
		} else {
			f = *(*float64)(p)
		}
		if math.IsInf(f, 0) || math.IsNaN(f) {
			return &UnsupportedValueError{Value: reflect.NewAt(t, p).Elem(), Str: strconv.FormatFloat(f, 'g', -1, bits)}
		}
		if fl&flagQuoted != 0 {
			e.buf = append(e.buf, '"')
			e.buf = appendFloat(e.buf, f, bits)
			e.buf = append(e.buf, '"')
			return nil
		}
		e.buf = appendFloat(e.buf, f, bits)
		return nil
	}
}

func encodeString(e *encodeState, p unsafe.Pointer, fl encFlags) error {
	s := *(*string)(p)
	if fl&flagQuoted != 0 {
		b := appendString(nil, s, fl&flagHTML != 0)
		e.buf = appendString(e.buf, unsafeString(b), false)
		return nil
	}
	e.buf = appendString(e.buf, s, fl&flagHTML != 0)
	return nil
}

func encodeNumber(e *encodeState, p unsafe.Pointer, fl encFlags) error {
	numStr := *(*string)(p)
	if numStr == "" {
		numStr = "0" // Number's zero-val
	}
	if !isValidNumber(numStr) {
		return fmt.Errorf("json: invalid number literal %q", numStr)
	}
	if fl&flagQuoted != 0 {
		e.buf = append(e.buf, '"')
		e.buf = append(e.buf, numStr...)
		e.buf = append(e.buf, '"')
		return nil
	}
	e.buf = append(e.buf, numStr...)
	return nil
}

func interfaceEncoder(t reflect.Type) encoderFunc {
	if t.NumMethod() == 0 {
		return func(e *encodeState, p unsafe.Pointer, fl encFlags) error {
			v := *(*any)(p)
			if v == nil {
				e.buf = append(e.buf, "null"...)
				return nil
			}
			return e.encodeIface(v, fl)
		}
	}
	return func(e *encodeState, p unsafe.Pointer, fl encFlags) error {
		v := reflect.NewAt(t, p).Elem()
		if v.IsNil() {
			e.buf = append(e.buf, "null"...)
			return nil
		}
		return e.encodeIface(v.Interface(), fl)
	}
}

// Struct encoding.

const (
	fastNone = iota
	fastString
	fastInt
	fastBool
	fastFloat64
)

type encField struct {
	nameNonEsc  string // `,"name":`
	nameEscHTML string // `,"name":` with HTML escaping
	offset      uintptr
	ptrSteps    []uintptr // offsets of embedded pointer fields to follow
	typ         reflect.Type
	kind        reflect.Kind
	enc         encoderFunc
	omitEmpty   bool
	omitZero    bool
	isZero      func(p unsafe.Pointer, addressable bool) bool
	quoted      bool
	fast        uint8
}

type structEncoder struct {
	t      reflect.Type
	fields []encField
	once   sync.Once
}

func newStructEncoder(t reflect.Type) encoderFunc {
	se := &structEncoder{t: t}
	return func(e *encodeState, p unsafe.Pointer, fl encFlags) error {
		se.once.Do(se.init)
		return se.encode(e, p, fl)
	}
}

func (se *structEncoder) init() {
	fs := cachedTypeFields(se.t)
	se.fields = make([]encField, len(fs.list))
	for i := range fs.list {
		f := &fs.list[i]
		ef := &se.fields[i]
		ef.nameNonEsc = "," + f.nameNonEsc
		ef.nameEscHTML = "," + f.nameEscHTML
		ef.omitEmpty = f.omitEmpty
		ef.omitZero = f.omitZero
		ef.quoted = f.quoted
		cur := se.t
		var off uintptr
		for k, idx := range f.index {
			sf := cur.Field(idx)
			if k == len(f.index)-1 {
				off += sf.Offset
				ef.typ = sf.Type
				break
			}
			if sf.Type.Kind() == reflect.Pointer {
				ef.ptrSteps = append(ef.ptrSteps, off+sf.Offset)
				off = 0
				cur = sf.Type.Elem()
			} else {
				off += sf.Offset
				cur = sf.Type
			}
		}
		ef.offset = off
		ef.kind = ef.typ.Kind()
		ef.enc = typeEncoder(ef.typ)
		if ef.omitZero {
			ef.isZero = zeroChecker(ef.typ)
		}
		if !hasMarshalMethods(ef.typ) {
			switch ef.kind {
			case reflect.String:
				if ef.typ != numberType {
					ef.fast = fastString
				}
			case reflect.Int, reflect.Int64:
				if ef.typ.Size() == 8 {
					ef.fast = fastInt
				}
			case reflect.Bool:
				ef.fast = fastBool
			case reflect.Float64:
				ef.fast = fastFloat64
			}
		}
	}
}

func (se *structEncoder) encode(e *encodeState, p unsafe.Pointer, fl encFlags) error {
	html := fl&flagHTML != 0
	e.buf = append(e.buf, '{')
	first := true
FieldLoop:
	for i := range se.fields {
		f := &se.fields[i]
		fp, ffl := p, fl
		if len(f.ptrSteps) > 0 {
			for _, off := range f.ptrSteps {
				fp = *(*unsafe.Pointer)(unsafe.Add(fp, off))
				if fp == nil {
					continue FieldLoop
				}
			}
			ffl |= flagAddr
		}
		fp = unsafe.Add(fp, f.offset)
		if f.omitEmpty && isEmptyValue(f.kind, f.typ, fp) {
			continue
		}
		if f.omitZero && f.isZero(fp, ffl&flagAddr != 0) {
			continue
		}
		name := f.nameNonEsc
		if html {
			name = f.nameEscHTML
		}
		if first {
			name = name[1:]
			first = false
		}
		e.buf = append(e.buf, name...)
		if f.quoted {
			ffl |= flagQuoted
		} else {
			ffl &^= flagQuoted
			switch f.fast {
			case fastString:
				e.buf = appendString(e.buf, *(*string)(fp), html)
				continue
			case fastInt:
				e.buf = strconv.AppendInt(e.buf, *(*int64)(fp), 10)
				continue
			case fastBool:
				e.buf = strconv.AppendBool(e.buf, *(*bool)(fp))
				continue
			case fastFloat64:
				if x := *(*float64)(fp); !math.IsInf(x, 0) && !math.IsNaN(x) {
					e.buf = appendFloat(e.buf, x, 64)
					continue
				}
			}
		}
		if err := f.enc(e, fp, ffl); err != nil {
			return err
		}
	}
	e.buf = append(e.buf, '}')
	return nil
}

// isEmptyValue implements the `omitempty` test of encoding/json.
func isEmptyValue(k reflect.Kind, t reflect.Type, p unsafe.Pointer) bool {
	switch k {
	case reflect.Array:
		return t.Len() == 0
	case reflect.Map:
		return *(*unsafe.Pointer)(p) == nil || reflect.NewAt(t, p).Elem().Len() == 0
	case reflect.Slice:
		return (*sliceHeader)(p).len == 0
	case reflect.String:
		return len(*(*string)(p)) == 0
	case reflect.Bool:
		return !*(*bool)(p)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return loadInt(p, t.Size()) == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return loadUint(p, t.Size()) == 0
	case reflect.Float32:
		return math.Float32bits(*(*float32)(p)) == 0
	case reflect.Float64:
		return math.Float64bits(*(*float64)(p)) == 0
	case reflect.Interface, reflect.Pointer:
		return *(*unsafe.Pointer)(p) == nil
	}
	return false
}

type isZeroer interface {
	IsZero() bool
}

var isZeroerType = reflect.TypeFor[isZeroer]()

// zeroChecker implements the `omitzero` test of encoding/json for type t.
func zeroChecker(t reflect.Type) func(p unsafe.Pointer, addressable bool) bool {
	switch {
	case t.Kind() == reflect.Interface && t.Implements(isZeroerType):
		return func(p unsafe.Pointer, _ bool) bool {
			v := reflect.NewAt(t, p).Elem()
			// Avoid panics calling IsZero on a nil interface or
			// non-nil interface with nil pointer.
			return v.IsNil() ||
				(v.Elem().Kind() == reflect.Pointer && v.Elem().IsNil()) ||
				v.Interface().(isZeroer).IsZero()
		}
	case t.Kind() == reflect.Pointer && t.Implements(isZeroerType):
		rt := rtypeOf(t)
		return func(p unsafe.Pointer, _ bool) bool {
			ptr := *(*unsafe.Pointer)(p)
			// Avoid panics calling IsZero on nil pointer.
			return ptr == nil || ptrIface(rt, ptr).(isZeroer).IsZero()
		}
	case t.Implements(isZeroerType):
		iface := valueIface(t)
		return func(p unsafe.Pointer, _ bool) bool {
			return iface(p).(isZeroer).IsZero()
		}
	case reflect.PointerTo(t).Implements(isZeroerType):
		rt := rtypeOf(reflect.PointerTo(t))
		return func(p unsafe.Pointer, addressable bool) bool {
			if !addressable {
				// Temporarily box v so we can take the address.
				v2 := reflect.New(t)
				v2.Elem().Set(reflect.NewAt(t, p).Elem())
				p = v2.UnsafePointer()
			}
			return ptrIface(rt, p).(isZeroer).IsZero()
		}
	}
	switch k := t.Kind(); k {
	case reflect.Bool, reflect.String, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64, reflect.Interface, reflect.Pointer:
		return func(p unsafe.Pointer, _ bool) bool { return isEmptyValue(k, t, p) }
	case reflect.Slice:
		return func(p unsafe.Pointer, _ bool) bool { return (*sliceHeader)(p).data == nil }
	case reflect.Map:
		return func(p unsafe.Pointer, _ bool) bool { return *(*unsafe.Pointer)(p) == nil }
	}
	return func(p unsafe.Pointer, _ bool) bool { return reflect.NewAt(t, p).Elem().IsZero() }
}

// Map encoding.

func (e *encodeState) enterCycleCheck(key any, v func() reflect.Value, t reflect.Type) error {
	if _, ok := e.ptrSeen[key]; ok {
		return &UnsupportedValueError{Value: v(), Str: fmt.Sprintf("encountered a cycle via %s", t)}
	}
	if e.ptrSeen == nil {
		e.ptrSeen = make(map[any]struct{})
	}
	e.ptrSeen[key] = struct{}{}
	return nil
}

type mapKV struct {
	k string
	i int
}

var mapKVPool = sync.Pool{New: func() any { return new([]mapKV) }}

type kvAny struct {
	k string
	v any
}

var kvAnyPool = sync.Pool{New: func() any { return new([]kvAny) }}

func newMapEncoder(t reflect.Type) encoderFunc {
	kt, et := t.Key(), t.Elem()
	switch kt.Kind() {
	case reflect.String,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
	default:
		if !kt.Implements(textMarshalerType) {
			return unsupportedTypeEncoder(t)
		}
	}
	if kt == stringType && et == anyType {
		return func(e *encodeState, p unsafe.Pointer, fl encFlags) error {
			return e.encodeMapStringAny(*(*map[string]any)(p), fl)
		}
	}
	elemEnc := typeEncoder(et)
	esize := et.Size()
	sliceType := reflect.SliceOf(et)
	keyName := keyResolver(kt)
	return func(e *encodeState, p unsafe.Pointer, fl encFlags) error {
		mptr := *(*unsafe.Pointer)(p)
		if mptr == nil {
			e.buf = append(e.buf, "null"...)
			return nil
		}
		if e.ptrLevel++; e.ptrLevel > startDetectingCyclesAfter {
			if err := e.enterCycleCheck(mptr, func() reflect.Value { return reflect.NewAt(t, p).Elem() }, t); err != nil {
				return err
			}
			defer delete(e.ptrSeen, mptr)
		}
		mv := reflect.NewAt(t, p).Elem()
		n := mv.Len()
		vals := reflect.MakeSlice(sliceType, n, n)
		base := vals.UnsafePointer()
		kvp := mapKVPool.Get().(*[]mapKV)
		kvs := (*kvp)[:0]
		kv := reflect.New(kt).Elem()
		iter := mv.MapRange()
		for i := 0; iter.Next(); i++ {
			kv.SetIterKey(iter)
			ks, err := keyName(kv)
			if err != nil {
				*kvp = kvs[:0]
				mapKVPool.Put(kvp)
				return fmt.Errorf("json: encoding error for type %q: %q", t.String(), err.Error())
			}
			vals.Index(i).SetIterValue(iter)
			kvs = append(kvs, mapKV{ks, i})
		}
		slices.SortFunc(kvs, func(a, b mapKV) int { return strings.Compare(a.k, b.k) })
		html := fl&flagHTML != 0
		efl := fl &^ flagAddr
		e.buf = append(e.buf, '{')
		var err error
		for j, kv := range kvs {
			if j > 0 {
				e.buf = append(e.buf, ',')
			}
			e.buf = appendString(e.buf, kv.k, html)
			e.buf = append(e.buf, ':')
			if err = elemEnc(e, unsafe.Add(base, uintptr(kv.i)*esize), efl); err != nil {
				break
			}
		}
		clear(kvs)
		*kvp = kvs[:0]
		mapKVPool.Put(kvp)
		if err != nil {
			return err
		}
		e.buf = append(e.buf, '}')
		e.ptrLevel--
		return nil
	}
}

func keyResolver(kt reflect.Type) func(reflect.Value) (string, error) {
	if kt.Kind() == reflect.String {
		return func(k reflect.Value) (string, error) { return k.String(), nil }
	}
	if kt.Implements(textMarshalerType) {
		return func(k reflect.Value) (string, error) {
			if k.Kind() == reflect.Pointer && k.IsNil() {
				return "", nil
			}
			buf, err := k.Interface().(encoding.TextMarshaler).MarshalText()
			return string(buf), err
		}
	}
	switch kt.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return func(k reflect.Value) (string, error) { return strconv.FormatInt(k.Int(), 10), nil }
	default:
		return func(k reflect.Value) (string, error) { return strconv.FormatUint(k.Uint(), 10), nil }
	}
}

var mapStringAnyType = reflect.TypeFor[map[string]any]()

func (e *encodeState) encodeMapStringAny(m map[string]any, fl encFlags) error {
	if m == nil {
		e.buf = append(e.buf, "null"...)
		return nil
	}
	if e.ptrLevel++; e.ptrLevel > startDetectingCyclesAfter {
		ptr := *(*unsafe.Pointer)(unsafe.Pointer(&m))
		if err := e.enterCycleCheck(ptr, func() reflect.Value { return reflect.ValueOf(m) }, mapStringAnyType); err != nil {
			return err
		}
		defer delete(e.ptrSeen, ptr)
	}
	kvp := kvAnyPool.Get().(*[]kvAny)
	kvs := (*kvp)[:0]
	for k, v := range m {
		kvs = append(kvs, kvAny{k, v})
	}
	slices.SortFunc(kvs, func(a, b kvAny) int { return strings.Compare(a.k, b.k) })
	html := fl&flagHTML != 0
	e.buf = append(e.buf, '{')
	var err error
	for j := range kvs {
		if j > 0 {
			e.buf = append(e.buf, ',')
		}
		e.buf = appendString(e.buf, kvs[j].k, html)
		e.buf = append(e.buf, ':')
		if v := kvs[j].v; v == nil {
			e.buf = append(e.buf, "null"...)
		} else if err = e.encodeIface(v, fl); err != nil {
			break
		}
	}
	clear(kvs)
	*kvp = kvs[:0]
	kvAnyPool.Put(kvp)
	if err != nil {
		return err
	}
	e.buf = append(e.buf, '}')
	e.ptrLevel--
	return nil
}

var sliceAnyType = reflect.TypeFor[[]any]()

func (e *encodeState) encodeSliceAny(s []any, fl encFlags) error {
	if s == nil {
		e.buf = append(e.buf, "null"...)
		return nil
	}
	if e.ptrLevel++; e.ptrLevel > startDetectingCyclesAfter {
		key := struct {
			ptr any
			len int
		}{unsafe.Pointer(unsafe.SliceData(s)), len(s)}
		if err := e.enterCycleCheck(key, func() reflect.Value { return reflect.ValueOf(s) }, sliceAnyType); err != nil {
			return err
		}
		defer delete(e.ptrSeen, key)
	}
	e.buf = append(e.buf, '[')
	for i, v := range s {
		if i > 0 {
			e.buf = append(e.buf, ',')
		}
		if v == nil {
			e.buf = append(e.buf, "null"...)
		} else if err := e.encodeIface(v, fl); err != nil {
			return err
		}
	}
	e.buf = append(e.buf, ']')
	e.ptrLevel--
	return nil
}

// Slices, arrays and pointers.

func encodeByteSlice(e *encodeState, p unsafe.Pointer, _ encFlags) error {
	s := *(*[]byte)(p)
	if s == nil {
		e.buf = append(e.buf, "null"...)
		return nil
	}
	e.buf = append(e.buf, '"')
	e.buf = base64.StdEncoding.AppendEncode(e.buf, s)
	e.buf = append(e.buf, '"')
	return nil
}

func newSliceEncoder(t reflect.Type) encoderFunc {
	et := t.Elem()
	// Byte slices get special treatment; arrays don't.
	if et.Kind() == reflect.Uint8 {
		pe := reflect.PointerTo(et)
		if !pe.Implements(marshalerType) && !pe.Implements(textMarshalerType) {
			return encodeByteSlice
		}
	}
	arrayEnc := arrayElemsEncoder(et)
	return func(e *encodeState, p unsafe.Pointer, fl encFlags) error {
		sh := (*sliceHeader)(p)
		if sh.data == nil {
			e.buf = append(e.buf, "null"...)
			return nil
		}
		if e.ptrLevel++; e.ptrLevel > startDetectingCyclesAfter {
			// Here we use a struct to memorize the pointer to the first
			// element of the slice and its length.
			key := struct {
				ptr any
				len int
			}{sh.data, sh.len}
			if err := e.enterCycleCheck(key, func() reflect.Value { return reflect.NewAt(t, p).Elem() }, t); err != nil {
				return err
			}
			defer delete(e.ptrSeen, key)
		}
		// Slice elements are always addressable.
		if err := arrayEnc(e, sh.data, sh.len, fl|flagAddr); err != nil {
			return err
		}
		e.ptrLevel--
		return nil
	}
}

func newArrayEncoder(t reflect.Type) encoderFunc {
	arrayEnc := arrayElemsEncoder(t.Elem())
	n := t.Len()
	return func(e *encodeState, p unsafe.Pointer, fl encFlags) error {
		return arrayEnc(e, p, n, fl)
	}
}

// arrayElemsEncoder encodes n consecutive elements of type et as a JSON
// array.
func arrayElemsEncoder(et reflect.Type) func(e *encodeState, p unsafe.Pointer, n int, fl encFlags) error {
	esize := et.Size()
	if !hasMarshalMethods(et) {
		switch et.Kind() {
		case reflect.String:
			if et != numberType {
				return func(e *encodeState, p unsafe.Pointer, n int, fl encFlags) error {
					if fl&flagQuoted != 0 {
						return encodeElems(e, p, n, esize, encodeString, fl)
					}
					html := fl&flagHTML != 0
					e.buf = append(e.buf, '[')
					for i := 0; i < n; i++ {
						if i > 0 {
							e.buf = append(e.buf, ',')
						}
						e.buf = appendString(e.buf, *(*string)(unsafe.Add(p, uintptr(i)*esize)), html)
					}
					e.buf = append(e.buf, ']')
					return nil
				}
			}
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return func(e *encodeState, p unsafe.Pointer, n int, fl encFlags) error {
				if fl&flagQuoted != 0 {
					return encodeElems(e, p, n, esize, intEncoder(esize), fl)
				}
				e.buf = append(e.buf, '[')
				for i := 0; i < n; i++ {
					if i > 0 {
						e.buf = append(e.buf, ',')
					}
					e.buf = strconv.AppendInt(e.buf, loadInt(unsafe.Add(p, uintptr(i)*esize), esize), 10)
				}
				e.buf = append(e.buf, ']')
				return nil
			}
		}
	}
	var elemEnc encoderFunc
	var once sync.Once
	return func(e *encodeState, p unsafe.Pointer, n int, fl encFlags) error {
		once.Do(func() { elemEnc = typeEncoder(et) })
		return encodeElems(e, p, n, esize, elemEnc, fl)
	}
}

func encodeElems(e *encodeState, p unsafe.Pointer, n int, esize uintptr, enc encoderFunc, fl encFlags) error {
	e.buf = append(e.buf, '[')
	for i := 0; i < n; i++ {
		if i > 0 {
			e.buf = append(e.buf, ',')
		}
		if err := enc(e, unsafe.Add(p, uintptr(i)*esize), fl); err != nil {
			return err
		}
	}
	e.buf = append(e.buf, ']')
	return nil
}

func newPtrEncoder(t reflect.Type) encoderFunc {
	var elemEnc encoderFunc
	var once sync.Once
	rt := rtypeOf(t)
	return func(e *encodeState, p unsafe.Pointer, fl encFlags) error {
		ptr := *(*unsafe.Pointer)(p)
		if ptr == nil {
			e.buf = append(e.buf, "null"...)
			return nil
		}
		once.Do(func() { elemEnc = typeEncoder(t.Elem()) })
		if e.ptrLevel++; e.ptrLevel > startDetectingCyclesAfter {
			// We're a large number of nested ptrEncoder.encode calls deep;
			// start checking if we've run into a pointer cycle.
			key := ptrIface(rt, ptr)
			if err := e.enterCycleCheck(key, func() reflect.Value { return reflect.NewAt(t, p).Elem() }, t); err != nil {
				return err
			}
			defer delete(e.ptrSeen, key)
		}
		if err := elemEnc(e, ptr, fl|flagAddr); err != nil {
			return err
		}
		e.ptrLevel--
		return nil
	}
}
