package fastjson

import (
	"reflect"
	"sync"
	"unsafe"
)

// eface is the runtime layout of an empty interface.
type eface struct {
	typ  unsafe.Pointer
	data unsafe.Pointer
}

type sliceHeader struct {
	data unsafe.Pointer
	len  int
	cap  int
}

func efaceOf(p *any) *eface { return (*eface)(unsafe.Pointer(p)) }

// rtypeOf returns the runtime type descriptor behind t, which is what an
// interface value stores in its type word.
func rtypeOf(t reflect.Type) unsafe.Pointer {
	return (*[2]unsafe.Pointer)(unsafe.Pointer(&t))[1]
}

var directIfaceCache sync.Map // reflect.Type -> bool

// isDirectIface reports whether values of type t are stored directly in the
// data word of an interface (pointer-shaped types) rather than behind a
// pointer. It is probed from the runtime instead of re-deriving the
// compiler's rules: the zero value of a pointer-shaped type has a nil data
// word, while any other type points at zeroed memory.
func isDirectIface(t reflect.Type) bool {
	if v, ok := directIfaceCache.Load(t); ok {
		return v.(bool)
	}
	z := reflect.Zero(t).Interface()
	direct := efaceOf(&z).data == nil
	directIfaceCache.Store(t, direct)
	return direct
}

// packEface builds an interface value of type t for the value stored at p
// without copying it. The result must only be used transiently (for method
// calls); it aliases p.
func packEface(rt unsafe.Pointer, direct bool, p unsafe.Pointer) any {
	var x any
	e := efaceOf(&x)
	e.typ = rt
	if direct {
		e.data = *(*unsafe.Pointer)(p)
	} else {
		e.data = p
	}
	return x
}

func unsafeString(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	return unsafe.String(unsafe.SliceData(b), len(b))
}

func unsafeNew(t reflect.Type) unsafe.Pointer { return reflect.New(t).UnsafePointer() }
