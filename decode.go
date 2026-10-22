package fastjson

import (
	"encoding"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"math/bits"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"
	"unsafe"
)

// Unmarshal parses the JSON-encoded data and stores the result in the value
// pointed to by v, with the same semantics as encoding/json.Unmarshal. The
// whole input is validated first, so a syntax error leaves v untouched.
func Unmarshal(data []byte, v any) error {
	if err := checkValid(data); err != nil {
		return err
	}
	return unmarshalValid(data, v, false, false)
}

// unmarshalValid decodes data, which must already be known to be valid JSON.
func unmarshalValid(data []byte, v any, useNumber, disallowUnknown bool) error {
	rt := reflect.TypeOf(v)
	if rt == nil || rt.Kind() != reflect.Pointer {
		return &InvalidUnmarshalError{Type: rt}
	}
	ptr := efaceOf(&v).data
	if ptr == nil {
		return &InvalidUnmarshalError{Type: rt}
	}
	d := decodeStatePool.Get().(*decodeState)
	d.data = data
	d.off = skipSpace(data, 0)
	d.useNumber = useNumber
	d.disallowUnknownFields = disallowUnknown
	err := topDecoder(rt, rt)(d, ptr)
	if err != nil {
		err = d.addErrorContext(err)
	} else {
		err = d.savedError
	}
	d.release()
	return err
}

type errorContext struct {
	Struct     reflect.Type
	FieldStack []string
}

type decodeState struct {
	data                  []byte
	off                   int
	useNumber             bool
	disallowUnknownFields bool
	savedError            error
	errorContext          errorContext
	scratch               []byte
}

var decodeStatePool = sync.Pool{New: func() any { return new(decodeState) }}

func (d *decodeState) release() {
	d.data = nil
	d.savedError = nil
	d.errorContext.Struct = nil
	clear(d.errorContext.FieldStack)
	d.errorContext.FieldStack = d.errorContext.FieldStack[:0]
	if cap(d.scratch) > 64<<10 {
		d.scratch = nil
	}
	decodeStatePool.Put(d)
}

func (d *decodeState) saveError(err error) {
	if d.savedError == nil {
		d.savedError = d.addErrorContext(err)
	}
}

func (d *decodeState) saveTypeError(what string, t reflect.Type, off int) {
	if d.savedError == nil {
		d.savedError = d.addErrorContext(&UnmarshalTypeError{Value: what, Type: t, Offset: int64(off)})
	}
}

func (d *decodeState) saveNumberError(lit []byte, t reflect.Type, off int) {
	if d.savedError == nil {
		d.saveTypeError("number "+string(lit), t, off)
	}
}

func (d *decodeState) addErrorContext(err error) error {
	if d.errorContext.Struct != nil || len(d.errorContext.FieldStack) > 0 {
		switch err := err.(type) {
		case *UnmarshalTypeError:
			if d.errorContext.Struct != nil {
				err.Struct = d.errorContext.Struct.Name()
			}
			fieldStack := d.errorContext.FieldStack
			if err.Field != "" {
				fieldStack = append(fieldStack, err.Field)
			}
			err.Field = strings.Join(fieldStack, ".")
		}
	}
	return err
}

func (d *decodeState) skipWS() {
	for d.off < len(d.data) && charClass[d.data[d.off]]&ccSpace != 0 {
		d.off++
	}
}

// skipValue advances past the value starting at d.off.
func (d *decodeState) skipValue() {
	switch d.data[d.off] {
	case '"':
		d.off = skipString(d.data, d.off)
	case '{', '[':
		d.off = skipContainer(d.data, d.off)
	case 't', 'n':
		d.off += 4
	case 'f':
		d.off += 5
	default:
		d.off = numberEnd(d.data, d.off)
	}
}

// skipContainer returns the offset just past the object or array at b[i].
func skipContainer(b []byte, i int) int {
	depth := 0
	for {
		for i+8 <= len(b) {
			w := binary.LittleEndian.Uint64(b[i:])
			lw := w | lsb*0x20
			if m := hasByte(w, '"') | hasByte(lw, '{') | hasByte(lw, '}'); m != 0 {
				i += bits.TrailingZeros64(m) / 8
				break
			}
			i += 8
		}
		switch b[i] {
		case '"':
			i = skipString(b, i)
			continue
		case '{', '[':
			depth++
		case '}', ']':
			depth--
			if depth == 0 {
				return i + 1
			}
		}
		i++
	}
}

// mismatch records an UnmarshalTypeError for the value at d.off (which is
// not null) and skips it, using encoding/json's value names and offsets.
func (d *decodeState) mismatch(t reflect.Type) {
	switch d.data[d.off] {
	case '{':
		d.saveTypeError("object", t, d.off+1)
		d.skipValue()
	case '[':
		d.saveTypeError("array", t, d.off+1)
		d.skipValue()
	case '"':
		d.skipValue()
		d.saveTypeError("string", t, d.off)
	case 't', 'f':
		d.skipValue()
		d.saveTypeError("bool", t, d.off)
	case 'n':
		d.off += 4
	default:
		d.skipValue()
		d.saveTypeError("number", t, d.off)
	}
}

// readString decodes the string at d.off into a new Go string.
func (d *decodeState) readString() string {
	start := d.off + 1
	end, flags := scanStringFlags(d.data, d.off)
	d.off = end + 1
	body := d.data[start:end]
	if flags == 0 || (flags == strNonASCII && utf8.Valid(body)) {
		return string(body)
	}
	d.scratch = unquoteInto(d.scratch[:0], body)
	return string(d.scratch)
}

// readStringTransient decodes the string at d.off. The result may alias the
// input or the scratch buffer and is only valid until the next read.
func (d *decodeState) readStringTransient() []byte {
	start := d.off + 1
	end, flags := scanStringFlags(d.data, d.off)
	d.off = end + 1
	body := d.data[start:end]
	if flags == 0 || (flags == strNonASCII && utf8.Valid(body)) {
		return body
	}
	d.scratch = unquoteInto(d.scratch[:0], body)
	return d.scratch
}

// readStringOwned decodes the string at d.off like encoding/json's
// unquoteBytes: unescaped input is aliased, anything else is a new slice the
// callee may keep.
func (d *decodeState) readStringOwned() []byte {
	start := d.off + 1
	end, flags := scanStringFlags(d.data, d.off)
	d.off = end + 1
	body := d.data[start:end]
	if flags == 0 || (flags == strNonASCII && utf8.Valid(body)) {
		return body
	}
	return unquoteInto(make([]byte, 0, len(body)), body)
}

type decoderFunc func(d *decodeState, p unsafe.Pointer) error

type typePair struct{ t, errType reflect.Type }

var (
	locDecoderCache   sync.Map // reflect.Type -> decoderFunc
	plainDecoderCache sync.Map // reflect.Type -> decoderFunc
	ptrDecoderCache   sync.Map // typePair -> decoderFunc
	topDecoderCache   sync.Map // typePair -> decoderFunc
)

var (
	unmarshalerType     = reflect.TypeFor[Unmarshaler]()
	textUnmarshalerType = reflect.TypeFor[encoding.TextUnmarshaler]()
	numberType          = reflect.TypeFor[Number]()
	anyType             = reflect.TypeFor[any]()
	stringType          = reflect.TypeFor[string]()
	float64Type         = reflect.TypeFor[float64]()
	reflectInt64        = reflect.TypeFor[int64]()
	reflectBool         = reflect.TypeFor[bool]()
)

// cachedDecoder returns the decoder stored under key, building it on first
// use. Recursive types see an indirect placeholder while being built.
func cachedDecoder[K comparable](cache *sync.Map, key K, build func() decoderFunc) decoderFunc {
	if f, ok := cache.Load(key); ok {
		return f.(decoderFunc)
	}
	var (
		wg sync.WaitGroup
		f  decoderFunc
	)
	wg.Add(1)
	fi, loaded := cache.LoadOrStore(key, decoderFunc(func(d *decodeState, p unsafe.Pointer) error {
		wg.Wait()
		return f(d, p)
	}))
	if loaded {
		return fi.(decoderFunc)
	}
	f = build()
	wg.Done()
	cache.Store(key, f)
	return f
}

// The decoders below compile encoding/json's indirect() walk into a fixed
// chain per type:
//
//   - locDecoder decodes into an addressable, settable location of type t
//     (struct fields, slice/array/map elements). Named non-pointer types
//     honor Unmarshaler/TextUnmarshaler on *t.
//   - ptrDecoder decodes into a settable pointer variable.
//   - topDecoder decodes through a non-settable pointer value (the argument
//     to Unmarshal, or a pointer held by an interface).
//   - plainDecoder decodes into a non-pointer location without looking at
//     methods.

func locDecoder(t reflect.Type) decoderFunc {
	return cachedDecoder(&locDecoderCache, t, func() decoderFunc { return newLocDecoder(t) })
}

func plainDecoder(t reflect.Type) decoderFunc {
	return cachedDecoder(&plainDecoderCache, t, func() decoderFunc { return newPlainDecoder(t) })
}

func ptrDecoder(t, errType reflect.Type) decoderFunc {
	return cachedDecoder(&ptrDecoderCache, typePair{t, errType}, func() decoderFunc { return newPtrDecoder(t, errType) })
}

func topDecoder(t, errType reflect.Type) decoderFunc {
	return cachedDecoder(&topDecoderCache, typePair{t, errType}, func() decoderFunc { return newTopDecoder(t, errType) })
}

func newLocDecoder(t reflect.Type) decoderFunc {
	switch t.Kind() {
	case reflect.Pointer:
		return ptrDecoder(t, t)
	case reflect.Interface:
		return newIfaceDecoder(t, true)
	}
	if t.Name() != "" {
		pt := reflect.PointerTo(t)
		if pt.Implements(unmarshalerType) {
			rt := rtypeOf(pt)
			return func(d *decodeState, p unsafe.Pointer) error {
				return d.callUnmarshaler(ptrIface(rt, p).(Unmarshaler))
			}
		}
		if pt.Implements(textUnmarshalerType) {
			rt := rtypeOf(pt)
			plain := plainDecoder(t)
			return func(d *decodeState, p unsafe.Pointer) error {
				if d.data[d.off] == 'n' {
					return plain(d, p)
				}
				return d.textUnmarshal(ptrIface(rt, p).(encoding.TextUnmarshaler), t)
			}
		}
	}
	return plainDecoder(t)
}

// elemDecoder returns the decoder for the element of a pointer once the
// pointer itself has been handled: no address-taking method lookup happens
// at that level because the pointer's method set was already checked.
func elemDecoder(et, errType reflect.Type) decoderFunc {
	switch et.Kind() {
	case reflect.Pointer:
		return ptrDecoder(et, errType)
	case reflect.Interface:
		return locDecoder(et)
	default:
		return plainDecoder(et)
	}
}

func pointerMethods(t reflect.Type) (isU, isTU bool) {
	if t.NumMethod() == 0 {
		return false, false
	}
	if t.Implements(unmarshalerType) {
		return true, false
	}
	return false, t.Implements(textUnmarshalerType)
}

func newPtrDecoder(t, errType reflect.Type) decoderFunc {
	et := t.Elem()
	next := elemDecoder(et, errType)
	isU, isTU := pointerMethods(t)
	rt := rtypeOf(t)
	elemIsIface := et.Kind() == reflect.Interface
	return func(d *decodeState, p unsafe.Pointer) error {
		if d.data[d.off] == 'n' {
			*(*unsafe.Pointer)(p) = nil
			d.off += 4
			return nil
		}
		ptr := *(*unsafe.Pointer)(p)
		if elemIsIface && ptr != nil && ifaceHoldsSelf(t, ptr) {
			return plainDecoder(et)(d, ptr)
		}
		if ptr == nil {
			ptr = unsafeNew(et)
			*(*unsafe.Pointer)(p) = ptr
		}
		if isU {
			return d.callUnmarshaler(ptrIface(rt, ptr).(Unmarshaler))
		}
		if isTU {
			return d.textUnmarshal(ptrIface(rt, ptr).(encoding.TextUnmarshaler), errType)
		}
		return next(d, ptr)
	}
}

func newTopDecoder(t, errType reflect.Type) decoderFunc {
	et := t.Elem()
	next := elemDecoder(et, errType)
	isU, isTU := pointerMethods(t)
	rt := rtypeOf(t)
	elemIsIface := et.Kind() == reflect.Interface
	return func(d *decodeState, ptr unsafe.Pointer) error {
		if elemIsIface && ifaceHoldsSelf(t, ptr) {
			return plainDecoder(et)(d, ptr)
		}
		if isU {
			return d.callUnmarshaler(ptrIface(rt, ptr).(Unmarshaler))
		}
		if isTU && d.data[d.off] != 'n' {
			return d.textUnmarshal(ptrIface(rt, ptr).(encoding.TextUnmarshaler), errType)
		}
		return next(d, ptr)
	}
}

// ifaceHoldsSelf reports whether the interface at ptr (of type t.Elem())
// holds the pointer ptr itself, which encoding/json treats specially to
// avoid looping forever.
func ifaceHoldsSelf(t reflect.Type, ptr unsafe.Pointer) bool {
	et := t.Elem()
	if et.NumMethod() == 0 {
		e := (*eface)(ptr)
		return e.typ == rtypeOf(t) && e.data == ptr
	}
	v := reflect.NewAt(et, ptr).Elem()
	if v.IsNil() {
		return false
	}
	e := v.Elem()
	return e.Type() == t && e.UnsafePointer() == ptr
}

// ptrIface returns an interface value of pointer type rt holding p.
func ptrIface(rt, p unsafe.Pointer) any {
	var x any
	e := efaceOf(&x)
	e.typ = rt
	e.data = p
	return x
}

func (d *decodeState) callUnmarshaler(u Unmarshaler) error {
	start := d.off
	d.skipValue()
	return u.UnmarshalJSON(d.data[start:d.off])
}

func (d *decodeState) textUnmarshal(u encoding.TextUnmarshaler, errType reflect.Type) error {
	switch c := d.data[d.off]; c {
	case '"':
		return u.UnmarshalText(d.readStringOwned())
	case '{':
		d.saveTypeError("object", errType, d.off+1)
		d.skipValue()
	case '[':
		d.saveTypeError("array", errType, d.off+1)
		d.skipValue()
	default:
		d.skipValue()
		what := "number"
		switch c {
		case 'n':
			what = "null"
		case 't', 'f':
			what = "bool"
		}
		d.saveTypeError(what, errType, d.off)
	}
	return nil
}

func newIfaceDecoder(t reflect.Type, descend bool) decoderFunc {
	empty := t.NumMethod() == 0
	return func(d *decodeState, p unsafe.Pointer) error {
		c := d.data[d.off]
		if descend {
			var dyn reflect.Type
			var dptr unsafe.Pointer
			if empty {
				if e := (*eface)(p); e.typ != nil {
					dyn = reflect.TypeOf(*(*any)(p))
					if dyn.Kind() == reflect.Pointer {
						dptr = e.data
					}
				}
			} else if v := reflect.NewAt(t, p).Elem(); !v.IsNil() {
				if e := v.Elem(); e.Kind() == reflect.Pointer {
					dyn, dptr = e.Type(), e.UnsafePointer()
				}
			}
			if dptr != nil && (c != 'n' || dyn.Elem().Kind() == reflect.Pointer) {
				return topDecoder(dyn, t)(d, dptr)
			}
		}
		if c == 'n' {
			if empty {
				*(*any)(p) = nil
			} else {
				reflect.NewAt(t, p).Elem().SetZero()
			}
			d.off += 4
			return nil
		}
		if empty {
			if c == '-' || c-'0' <= 9 {
				if v, ok := d.numberInterface(); ok {
					*(*any)(p) = v
				}
				return nil
			}
			*(*any)(p) = d.valueInterface()
			return nil
		}
		if c == '-' || c-'0' <= 9 {
			if _, ok := d.numberInterface(); !ok {
				d.saveTypeError("number", t, d.off)
				return nil
			}
			d.saveTypeError("number", t, d.off)
			return nil
		}
		d.mismatch(t)
		return nil
	}
}

func newPlainDecoder(t reflect.Type) decoderFunc {
	switch t.Kind() {
	case reflect.Bool:
		return func(d *decodeState, p unsafe.Pointer) error {
			switch d.data[d.off] {
			case 't':
				*(*bool)(p) = true
				d.off += 4
			case 'f':
				*(*bool)(p) = false
				d.off += 5
			case 'n':
				d.off += 4
			default:
				d.mismatch(t)
			}
			return nil
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return newIntDecoder(t)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return newUintDecoder(t)
	case reflect.Float32, reflect.Float64:
		return newFloatDecoder(t)
	case reflect.String:
		if t == numberType {
			return decodeNumber
		}
		return func(d *decodeState, p unsafe.Pointer) error {
			switch d.data[d.off] {
			case '"':
				*(*string)(p) = d.readString()
			case 'n':
				d.off += 4
			default:
				d.mismatch(t)
			}
			return nil
		}
	case reflect.Slice:
		return newSliceDecoder(t)
	case reflect.Array:
		return newArrayDecoder(t)
	case reflect.Map:
		return newMapDecoder(t)
	case reflect.Struct:
		return newStructDecoder(t)
	case reflect.Interface:
		return newIfaceDecoder(t, false)
	default:
		return func(d *decodeState, p unsafe.Pointer) error {
			if d.data[d.off] == 'n' {
				d.off += 4
				return nil
			}
			d.mismatch(t)
			return nil
		}
	}
}

func newIntDecoder(t reflect.Type) decoderFunc {
	size := t.Size()
	bitSize := t.Bits()
	return func(d *decodeState, p unsafe.Pointer) error {
		c := d.data[d.off]
		if c != '-' && c-'0' > 9 {
			if c == 'n' {
				d.off += 4
			} else {
				d.mismatch(t)
			}
			return nil
		}
		start := d.off
		end := numberEnd(d.data, start)
		d.off = end
		n, ok := parseInt(d.data[start:end])
		if !ok || (bitSize < 64 && (n < -1<<(bitSize-1) || n >= 1<<(bitSize-1))) {
			d.saveNumberError(d.data[start:end], t, end)
			return nil
		}
		switch size {
		case 8:
			*(*int64)(p) = n
		case 4:
			*(*int32)(p) = int32(n)
		case 2:
			*(*int16)(p) = int16(n)
		default:
			*(*int8)(p) = int8(n)
		}
		return nil
	}
}

func newUintDecoder(t reflect.Type) decoderFunc {
	size := t.Size()
	bitSize := t.Bits()
	return func(d *decodeState, p unsafe.Pointer) error {
		c := d.data[d.off]
		if c != '-' && c-'0' > 9 {
			if c == 'n' {
				d.off += 4
			} else {
				d.mismatch(t)
			}
			return nil
		}
		start := d.off
		end := numberEnd(d.data, start)
		d.off = end
		n, ok := parseUint(d.data[start:end])
		if !ok || (bitSize < 64 && n >= 1<<bitSize) {
			d.saveNumberError(d.data[start:end], t, end)
			return nil
		}
		switch size {
		case 8:
			*(*uint64)(p) = n
		case 4:
			*(*uint32)(p) = uint32(n)
		case 2:
			*(*uint16)(p) = uint16(n)
		default:
			*(*uint8)(p) = uint8(n)
		}
		return nil
	}
}

func newFloatDecoder(t reflect.Type) decoderFunc {
	bitSize := t.Bits()
	return func(d *decodeState, p unsafe.Pointer) error {
		c := d.data[d.off]
		if c != '-' && c-'0' > 9 {
			if c == 'n' {
				d.off += 4
			} else {
				d.mismatch(t)
			}
			return nil
		}
		start := d.off
		end := numberEnd(d.data, start)
		d.off = end
		f, err := parseFloat(d.data[start:end], bitSize)
		if err != nil {
			d.saveNumberError(d.data[start:end], t, end)
			return nil
		}
		if bitSize == 32 {
			*(*float32)(p) = float32(f)
		} else {
			*(*float64)(p) = f
		}
		return nil
	}
}

func decodeNumber(d *decodeState, p unsafe.Pointer) error {
	switch c := d.data[d.off]; {
	case c == '"':
		start := d.off
		s := d.readStringTransient()
		if !isValidNumber(unsafeString(s)) {
			return fmt.Errorf("json: invalid number literal, trying to unmarshal %q into Number", d.data[start:d.off])
		}
		*(*Number)(p) = Number(s)
	case c == '-' || c-'0' <= 9:
		start := d.off
		d.off = numberEnd(d.data, start)
		*(*Number)(p) = Number(d.data[start:d.off])
	case c == 'n':
		d.off += 4
	default:
		d.mismatch(numberType)
	}
	return nil
}

func newSliceDecoder(t reflect.Type) decoderFunc {
	et := t.Elem()
	esize := et.Size()
	isBytes := et.Kind() == reflect.Uint8
	emptyData := reflect.MakeSlice(t, 0, 0).UnsafePointer()
	var elemDec decoderFunc
	var once sync.Once
	return func(d *decodeState, p unsafe.Pointer) error {
		switch d.data[d.off] {
		case 'n':
			*(*sliceHeader)(p) = sliceHeader{}
			d.off += 4
			return nil
		case '"':
			if !isBytes {
				d.mismatch(t)
				return nil
			}
			s := d.readStringTransient()
			b := make([]byte, base64.StdEncoding.DecodedLen(len(s)))
			n, err := base64.StdEncoding.Decode(b, s)
			if err != nil {
				d.saveError(err)
				return nil
			}
			*(*[]byte)(p) = b[:n]
			return nil
		case '[':
		default:
			d.mismatch(t)
			return nil
		}
		once.Do(func() { elemDec = locDecoder(et) })
		d.off++
		d.skipWS()
		sh := (*sliceHeader)(p)
		i := 0
		if d.data[d.off] == ']' {
			d.off++
		} else {
			for {
				if i >= sh.cap {
					// Start at four elements and double, instead of
					// growing one element at a time.
					n := sh.cap
					if n < 4 {
						n = 4
					}
					reflect.NewAt(t, p).Elem().Grow(n)
				}
				if i >= sh.len {
					sh.len = i + 1
				}
				if err := elemDec(d, unsafe.Add(sh.data, uintptr(i)*esize)); err != nil {
					return err
				}
				i++
				d.skipWS()
				if d.data[d.off] == ',' {
					d.off++
					d.skipWS()
					continue
				}
				d.off++
				break
			}
		}
		if i < sh.len {
			sh.len = i
		}
		if i == 0 {
			*sh = sliceHeader{data: emptyData}
		}
		return nil
	}
}

func newArrayDecoder(t reflect.Type) decoderFunc {
	et := t.Elem()
	esize := et.Size()
	n := t.Len()
	var elemDec decoderFunc
	var once sync.Once
	return func(d *decodeState, p unsafe.Pointer) error {
		switch d.data[d.off] {
		case 'n':
			d.off += 4
			return nil
		case '[':
		default:
			d.mismatch(t)
			return nil
		}
		once.Do(func() { elemDec = locDecoder(et) })
		d.off++
		d.skipWS()
		i := 0
		if d.data[d.off] == ']' {
			d.off++
		} else {
			for {
				if i < n {
					if err := elemDec(d, unsafe.Add(p, uintptr(i)*esize)); err != nil {
						return err
					}
				} else {
					d.skipValue()
				}
				i++
				d.skipWS()
				if d.data[d.off] == ',' {
					d.off++
					d.skipWS()
					continue
				}
				d.off++
				break
			}
		}
		for ; i < n; i++ {
			reflect.NewAt(et, unsafe.Add(p, uintptr(i)*esize)).Elem().SetZero()
		}
		return nil
	}
}

func newMapDecoder(t reflect.Type) decoderFunc {
	kt, et := t.Key(), t.Elem()
	textKey := reflect.PointerTo(kt).Implements(textUnmarshalerType)
	keyOK := textKey
	switch kt.Kind() {
	case reflect.String,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		keyOK = true
	}
	if !textKey && kt == stringType {
		switch et {
		case anyType:
			return func(d *decodeState, p unsafe.Pointer) error {
				return d.mapStringAny(t, p)
			}
		case stringType:
			return func(d *decodeState, p unsafe.Pointer) error {
				return d.mapStringString(t, p)
			}
		}
	}
	var elemDec decoderFunc
	var once sync.Once
	return func(d *decodeState, p unsafe.Pointer) error {
		switch d.data[d.off] {
		case 'n':
			*(*unsafe.Pointer)(p) = nil
			d.off += 4
			return nil
		case '{':
		default:
			d.mismatch(t)
			return nil
		}
		if !keyOK {
			d.saveTypeError("object", t, d.off+1)
			d.skipValue()
			return nil
		}
		once.Do(func() { elemDec = locDecoder(et) })
		mv := reflect.NewAt(t, p).Elem()
		if mv.IsNil() {
			mv.Set(reflect.MakeMap(t))
		}
		elem := reflect.New(et)
		ev, ep := elem.Elem(), elem.UnsafePointer()
		var kv reflect.Value
		d.off++
		d.skipWS()
		if d.data[d.off] == '}' {
			d.off++
			return nil
		}
		first := true
		for {
			keyStart := d.off
			end, flags := scanStringFlags(d.data, keyStart)
			d.off = end + 1
			item := d.data[keyStart : end+1]
			d.skipWS()
			d.off++ // ':'
			d.skipWS()
			if !first {
				ev.SetZero()
			}
			first = false
			if err := elemDec(d, ep); err != nil {
				return err
			}

			var key reflect.Value
			if textKey {
				kp := reflect.New(kt)
				if err := d.literalStore(item, kp, true, d.off); err != nil {
					return err
				}
				key = kp.Elem()
			} else {
				s := unquoteItem(item, flags)
				switch kt.Kind() {
				case reflect.String:
					if !kv.IsValid() {
						kv = reflect.New(kt).Elem()
					}
					kv.SetString(s)
					key = kv
				case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
					n, err := strconv.ParseInt(s, 10, 64)
					if err != nil || kt.OverflowInt(n) {
						d.saveTypeError("number "+s, kt, keyStart+1)
						break
					}
					key = reflect.New(kt).Elem()
					key.SetInt(n)
				default:
					n, err := strconv.ParseUint(s, 10, 64)
					if err != nil || kt.OverflowUint(n) {
						d.saveTypeError("number "+s, kt, keyStart+1)
						break
					}
					key = reflect.New(kt).Elem()
					key.SetUint(n)
				}
			}
			if key.IsValid() {
				mv.SetMapIndex(key, ev)
			}
			d.skipWS()
			if d.data[d.off] == ',' {
				d.off++
				d.skipWS()
				continue
			}
			d.off++
			return nil
		}
	}
}

// unquoteItem decodes a quoted string item (including its quotes).
func unquoteItem(item []byte, flags uint8) string {
	body := item[1 : len(item)-1]
	if flags == 0 || (flags == strNonASCII && utf8.Valid(body)) {
		return string(body)
	}
	return string(unquoteInto(make([]byte, 0, len(body)), body))
}

func (d *decodeState) mapStringAny(t reflect.Type, p unsafe.Pointer) error {
	switch d.data[d.off] {
	case 'n':
		*(*unsafe.Pointer)(p) = nil
		d.off += 4
		return nil
	case '{':
	default:
		d.mismatch(t)
		return nil
	}
	m := *(*map[string]any)(p)
	if m == nil {
		m = make(map[string]any)
		*(*map[string]any)(p) = m
	}
	d.off++
	d.skipWS()
	if d.data[d.off] == '}' {
		d.off++
		return nil
	}
	for {
		key := d.readString()
		d.skipWS()
		d.off++
		d.skipWS()
		m[key] = d.valueInterface()
		d.skipWS()
		if d.data[d.off] == ',' {
			d.off++
			d.skipWS()
			continue
		}
		d.off++
		return nil
	}
}

func (d *decodeState) mapStringString(t reflect.Type, p unsafe.Pointer) error {
	switch d.data[d.off] {
	case 'n':
		*(*unsafe.Pointer)(p) = nil
		d.off += 4
		return nil
	case '{':
	default:
		d.mismatch(t)
		return nil
	}
	m := *(*map[string]string)(p)
	if m == nil {
		m = make(map[string]string)
		*(*map[string]string)(p) = m
	}
	d.off++
	d.skipWS()
	if d.data[d.off] == '}' {
		d.off++
		return nil
	}
	for {
		key := d.readString()
		d.skipWS()
		d.off++
		d.skipWS()
		var val string
		switch d.data[d.off] {
		case '"':
			val = d.readString()
		case 'n':
			d.off += 4
		default:
			d.mismatch(stringType)
		}
		m[key] = val
		d.skipWS()
		if d.data[d.off] == ',' {
			d.off++
			d.skipWS()
			continue
		}
		d.off++
		return nil
	}
}

type ptrStep struct {
	offset   uintptr // offset of the embedded pointer field from the current base
	elem     reflect.Type
	settable bool
}

type decField struct {
	name       string
	offset     uintptr // offset from the base reached after ptrSteps
	ptrSteps   []ptrStep
	embedNames []string // Go names of the embedded fields on the path
	typ        reflect.Type
	dec        decoderFunc
	quoted     bool
}

type structDecoder struct {
	t      reflect.Type
	fields []decField
	exact  map[string]int
	folded map[string]int
}

func newStructDecoder(t reflect.Type) decoderFunc {
	sd := &structDecoder{t: t}
	var once sync.Once
	return func(d *decodeState, p unsafe.Pointer) error {
		once.Do(sd.init)
		return sd.decode(d, p)
	}
}

func (sd *structDecoder) init() {
	fs := cachedTypeFields(sd.t)
	sd.exact, sd.folded = fs.byExactName, fs.byFoldedName
	sd.fields = make([]decField, len(fs.list))
	for i := range fs.list {
		f := &fs.list[i]
		df := &sd.fields[i]
		df.name = f.name
		df.quoted = f.quoted
		cur := sd.t
		var off uintptr
		for k, idx := range f.index {
			sf := cur.Field(idx)
			if k == len(f.index)-1 {
				off += sf.Offset
				df.typ = sf.Type
				break
			}
			df.embedNames = append(df.embedNames, sf.Name)
			if sf.Type.Kind() == reflect.Pointer {
				df.ptrSteps = append(df.ptrSteps, ptrStep{offset: off + sf.Offset, elem: sf.Type.Elem(), settable: sf.IsExported()})
				off = 0
				cur = sf.Type.Elem()
			} else {
				off += sf.Offset
				cur = sf.Type
			}
		}
		df.offset = off
		df.dec = locDecoder(df.typ)
	}
}

func (sd *structDecoder) decode(d *decodeState, p unsafe.Pointer) error {
	switch d.data[d.off] {
	case '{':
	case 'n':
		d.off += 4
		return nil
	default:
		d.mismatch(sd.t)
		return nil
	}
	d.off++
	d.skipWS()
	if d.data[d.off] == '}' {
		d.off++
		return nil
	}
	next := 0
	for {
		keyStart := d.off + 1
		end, flags := scanStringFlags(d.data, d.off)
		d.off = end + 1
		key := d.data[keyStart:end]
		if flags != 0 && !(flags == strNonASCII && utf8.Valid(key)) {
			d.scratch = unquoteInto(d.scratch[:0], key)
			key = d.scratch
		}
		fi := -1
		if next < len(sd.fields) && string(key) == sd.fields[next].name {
			fi = next
		} else if i, ok := sd.exact[string(key)]; ok {
			fi = i
		} else if i, ok := sd.folded[string(foldName(key))]; ok {
			fi = i
		}
		d.skipWS()
		d.off++ // ':'
		d.skipWS()
		if fi >= 0 {
			next = fi + 1
			if err := d.decodeField(sd, &sd.fields[fi], p); err != nil {
				return err
			}
		} else {
			if d.disallowUnknownFields {
				d.saveError(fmt.Errorf("json: unknown field %q", key))
			}
			d.skipValue()
		}
		d.skipWS()
		if d.data[d.off] == ',' {
			d.off++
			d.skipWS()
			continue
		}
		d.off++
		return nil
	}
}

func (d *decodeState) decodeField(sd *structDecoder, f *decField, p unsafe.Pointer) error {
	for _, st := range f.ptrSteps {
		pp := unsafe.Add(p, st.offset)
		ptr := *(*unsafe.Pointer)(pp)
		if ptr == nil {
			if !st.settable {
				d.saveError(fmt.Errorf("json: cannot set embedded pointer to unexported struct: %v", st.elem))
				d.skipValue()
				return nil
			}
			ptr = unsafeNew(st.elem)
			*(*unsafe.Pointer)(pp) = ptr
		}
		p = ptr
	}
	p = unsafe.Add(p, f.offset)

	ctx := &d.errorContext
	stackLen, oldStruct := len(ctx.FieldStack), ctx.Struct
	ctx.FieldStack = append(ctx.FieldStack, f.embedNames...)
	ctx.Struct = sd.t
	ctx.FieldStack = append(ctx.FieldStack, f.name)
	var err error
	if f.quoted {
		err = d.decodeQuoted(reflect.NewAt(f.typ, p).Elem())
	} else {
		err = f.dec(d, p)
	}
	if err != nil {
		return err
	}
	ctx.FieldStack = ctx.FieldStack[:stackLen]
	ctx.Struct = oldStruct
	return nil
}

var nullLiteral = []byte("null")

// decodeQuoted handles a field with the `string` tag option.
func (d *decodeState) decodeQuoted(v reflect.Value) error {
	switch c := d.data[d.off]; c {
	case 'n':
		d.off += 4
		return d.literalStore(nullLiteral, v, false, d.off)
	case '"':
		s := d.readString()
		return d.literalStore([]byte(s), v, true, d.off)
	case '{', '[', 't', 'f':
		d.skipValue()
	default:
		d.numberInterface()
	}
	d.saveError(fmt.Errorf("json: invalid use of ,string struct tag, trying to unmarshal unquoted value into %v", v.Type()))
	return nil
}
