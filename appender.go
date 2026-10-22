package fastjson

import (
	"math"
	"strconv"
)

// Appender builds JSON output by hand with no reflection. Strings and floats
// are formatted exactly as Marshal formats them.
type Appender struct {
	B          []byte
	EscapeHTML bool
}

func NewAppender(dst []byte) *Appender { return &Appender{B: dst} }
func (a *Appender) Reset(dst []byte)   { a.B = dst }
func (a *Appender) Bytes() []byte      { return a.B }
func (a *Appender) Null()              { a.B = append(a.B, "null"...) }
func (a *Appender) Bool(v bool)        { a.B = strconv.AppendBool(a.B, v) }
func (a *Appender) Int(v int)          { a.B = strconv.AppendInt(a.B, int64(v), 10) }
func (a *Appender) Int64(v int64)      { a.B = strconv.AppendInt(a.B, v, 10) }
func (a *Appender) Uint64(v uint64)    { a.B = strconv.AppendUint(a.B, v, 10) }
func (a *Appender) String(v string)    { a.B = appendString(a.B, v, a.EscapeHTML) }
func (a *Appender) Raw(v string)       { a.B = append(a.B, v...) }
func (a *Appender) Byte(c byte)        { a.B = append(a.B, c) }

// Float64 appends v as Marshal would. NaN and infinities have no JSON
// representation and are written as null.
func (a *Appender) Float64(v float64) {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		a.Null()
		return
	}
	a.B = appendFloat(a.B, v, 64)
}
