package fastjson

import "strconv"

type Appender struct {
	B          []byte
	EscapeHTML bool
}

func NewAppender(dst []byte) *Appender { return &Appender{B: dst, EscapeHTML: true} }
func (a *Appender) Reset(dst []byte)   { a.B = dst }
func (a *Appender) Bytes() []byte      { return a.B }
func (a *Appender) Null()              { a.B = append(a.B, "null"...) }
func (a *Appender) Bool(v bool) {
	if v {
		a.B = append(a.B, "true"...)
	} else {
		a.B = append(a.B, "false"...)
	}
}
func (a *Appender) Int(v int)         { a.B = strconv.AppendInt(a.B, int64(v), 10) }
func (a *Appender) Int64(v int64)     { a.B = strconv.AppendInt(a.B, v, 10) }
func (a *Appender) Float64(v float64) { a.B = strconv.AppendFloat(a.B, v, 'g', -1, 64) }
func (a *Appender) String(v string)   { a.B = appendString(a.B, v, a.EscapeHTML) }
func (a *Appender) Raw(v string)      { a.B = append(a.B, v...) }
func (a *Appender) Byte(c byte)       { a.B = append(a.B, c) }
