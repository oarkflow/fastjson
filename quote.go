package fastjson

import (
	"encoding/binary"
	stdjson "encoding/json"
	"math"
	"math/bits"
	"strconv"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"
	"unsafe"
)

const hex = "0123456789abcdef"

// invalidUTF8Repl replaces each invalid UTF-8 byte when encoding strings.
// Classic encoding/json writes the \ufffd escape while the
// encoding/json/v2-backed implementation (GOEXPERIMENT=jsonv2, the default
// in recent Go releases) writes a raw U+FFFD; it is taken from the linked
// encoding/json so output stays byte-identical.
var invalidUTF8Repl = func() string {
	if b, err := stdjson.Marshal("\xff"); err == nil && len(b) > 2 {
		return string(b[1 : len(b)-1])
	}
	return `\ufffd`
}()

// htmlSafe and safe report, for ASCII bytes, whether the byte can be copied
// verbatim into a JSON string (with and without HTML escaping).
var safeSet, htmlSafeSet = func() (safe, html [utf8.RuneSelf]bool) {
	for c := 0x20; c < utf8.RuneSelf; c++ {
		if c == '"' || c == '\\' {
			continue
		}
		safe[c] = true
		html[c] = c != '<' && c != '>' && c != '&'
	}
	return
}()

// escapeIndex returns the index of the first byte in s at or after i that
// cannot be copied verbatim (control, quote, backslash, non-ASCII, and the
// HTML characters when html is set).
func escapeIndex(s string, i int, html bool) int {
	for i+8 <= len(s) {
		w := binary.LittleEndian.Uint64(unsafeStringBytes(s[i : i+8]))
		m := hasLess(w, 0x20) | hasByte(w, '"') | hasByte(w, '\\') | (w & msb)
		if html {
			m |= hasByte(w, '<') | hasByte(w, '>') | hasByte(w, '&')
		}
		if m != 0 {
			return i + bits.TrailingZeros64(m)/8
		}
		i += 8
	}
	for i < len(s) {
		c := s[i]
		if c >= utf8.RuneSelf || !safeSet[c] || (html && !htmlSafeSet[c]) {
			return i
		}
		i++
	}
	return i
}

func unsafeStringBytes(s string) []byte {
	return unsafe.Slice(unsafe.StringData(s), len(s))
}

// appendString appends the JSON encoding of s exactly as encoding/json does:
// invalid UTF-8 is replaced (see invalidUTF8Repl) and U+2028/U+2029 are escaped.
func appendString(dst []byte, s string, html bool) []byte {
	dst = append(dst, '"')
	start := 0
	i := escapeIndex(s, 0, html)
	for i < len(s) {
		b := s[i]
		if b < utf8.RuneSelf {
			if htmlSafeSet[b] || (!html && safeSet[b]) {
				i++
				continue
			}
			dst = append(dst, s[start:i]...)
			switch b {
			case '\\', '"':
				dst = append(dst, '\\', b)
			case '\b':
				dst = append(dst, '\\', 'b')
			case '\f':
				dst = append(dst, '\\', 'f')
			case '\n':
				dst = append(dst, '\\', 'n')
			case '\r':
				dst = append(dst, '\\', 'r')
			case '\t':
				dst = append(dst, '\\', 't')
			default:
				dst = append(dst, '\\', 'u', '0', '0', hex[b>>4], hex[b&0xF])
			}
			i++
			start = i
			i = escapeIndex(s, i, html)
			continue
		}
		c, size := utf8.DecodeRuneInString(s[i:])
		if c == utf8.RuneError && size == 1 {
			dst = append(dst, s[start:i]...)
			dst = append(dst, invalidUTF8Repl...)
			i += size
			start = i
			continue
		}
		if c == '\u2028' || c == '\u2029' {
			dst = append(dst, s[start:i]...)
			dst = append(dst, '\\', 'u', '2', '0', '2', hex[c&0xF])
			i += size
			start = i
			continue
		}
		i += size
	}
	dst = append(dst, s[start:]...)
	return append(dst, '"')
}

// appendFloat formats f exactly like encoding/json's float encoder. The
// caller must reject NaN and infinities.
func appendFloat(b []byte, f float64, bitSize int) []byte {
	abs := math.Abs(f)
	format := byte('f')
	if abs != 0 {
		if bitSize == 64 && (abs < 1e-6 || abs >= 1e21) || bitSize == 32 && (float32(abs) < 1e-6 || float32(abs) >= 1e21) {
			format = 'e'
		}
	}
	b = strconv.AppendFloat(b, f, format, -1, bitSize)
	if format == 'e' {
		// clean up e-09 to e-9
		n := len(b)
		if n >= 4 && b[n-4] == 'e' && b[n-3] == '-' && b[n-2] == '0' {
			b[n-2] = b[n-1]
			b = b[:n-1]
		}
	}
	return b
}

// String scanning for the decoder. Input has already been validated.

const (
	strEscaped  = 1 << iota // contains a backslash escape
	strNonASCII             // contains bytes >= 0x80
)

// scanStringFlags returns the index of the closing quote of the string whose
// opening quote is at b[i], plus flags describing its content.
func scanStringFlags(b []byte, i int) (int, uint8) {
	var flags uint8
	i++
	for {
		for i+8 <= len(b) {
			w := binary.LittleEndian.Uint64(b[i:])
			m := hasByte(w, '"') | hasByte(w, '\\')
			if flags&strNonASCII == 0 {
				m |= w & msb
			}
			if m != 0 {
				i += bits.TrailingZeros64(m) / 8
				break
			}
			i += 8
		}
		c := b[i]
		switch {
		case c == '"':
			return i, flags
		case c == '\\':
			flags |= strEscaped
			i += 2
		case c >= utf8.RuneSelf:
			flags |= strNonASCII
			i++
		default:
			i++
		}
	}
}

// skipString returns the offset just past the string starting at b[i].
func skipString(b []byte, i int) int {
	i++
	for {
		for i+8 <= len(b) {
			w := binary.LittleEndian.Uint64(b[i:])
			if m := hasByte(w, '"') | hasByte(w, '\\'); m != 0 {
				i += bits.TrailingZeros64(m) / 8
				break
			}
			i += 8
		}
		switch b[i] {
		case '"':
			return i + 1
		case '\\':
			i += 2
		default:
			i++
		}
	}
}

// unquoteInto appends the decoded content of the raw string body s (without
// quotes) to dst, following encoding/json: invalid UTF-8 and unpaired
// surrogates become U+FFFD.
func unquoteInto(dst, s []byte) []byte {
	r := 0
	for r < len(s) {
		c := s[r]
		switch {
		case c == '\\':
			r++
			switch s[r] {
			case '"', '\\', '/', '\'':
				dst = append(dst, s[r])
				r++
			case 'b':
				dst = append(dst, '\b')
				r++
			case 'f':
				dst = append(dst, '\f')
				r++
			case 'n':
				dst = append(dst, '\n')
				r++
			case 'r':
				dst = append(dst, '\r')
				r++
			case 't':
				dst = append(dst, '\t')
				r++
			case 'u':
				r--
				rr := getu4(s[r:])
				r += 6
				if utf16.IsSurrogate(rr) {
					rr1 := getu4(s[r:])
					if dec := utf16.DecodeRune(rr, rr1); dec != unicode.ReplacementChar {
						r += 6
						dst = utf8.AppendRune(dst, dec)
						break
					}
					rr = unicode.ReplacementChar
				}
				dst = utf8.AppendRune(dst, rr)
			}
		case c < utf8.RuneSelf:
			start := r
			for r < len(s) && s[r] < utf8.RuneSelf && s[r] != '\\' {
				r++
			}
			dst = append(dst, s[start:r]...)
		default:
			rr, size := utf8.DecodeRune(s[r:])
			if rr == utf8.RuneError && size == 1 {
				dst = append(dst, "\uFFFD"...)
			} else {
				dst = append(dst, s[r:r+size]...)
			}
			r += size
		}
	}
	return dst
}

func getu4(s []byte) rune {
	if len(s) < 6 || s[0] != '\\' || s[1] != 'u' {
		return -1
	}
	var r rune
	for _, c := range s[2:6] {
		switch {
		case '0' <= c && c <= '9':
			c = c - '0'
		case 'a' <= c && c <= 'f':
			c = c - 'a' + 10
		case 'A' <= c && c <= 'F':
			c = c - 'A' + 10
		default:
			return -1
		}
		r = r*16 + rune(c)
	}
	return r
}

// numberEnd returns the offset just past the number starting at b[i].
func numberEnd(b []byte, i int) int {
	for i < len(b) && charClass[b[i]]&ccNumChar != 0 {
		i++
	}
	return i
}

// parseInt parses a JSON number literal as an int64 with the same
// acceptance rules as strconv.ParseInt(s, 10, 64).
func parseInt(b []byte) (int64, bool) {
	neg := false
	i := 0
	if len(b) > 0 && b[0] == '-' {
		neg = true
		i = 1
	}
	if i >= len(b) || len(b)-i > 19 {
		n, err := strconv.ParseInt(unsafeString(b), 10, 64)
		return n, err == nil
	}
	var n uint64
	for ; i < len(b); i++ {
		c := b[i] - '0'
		if c > 9 {
			return 0, false
		}
		n = n*10 + uint64(c)
	}
	if neg {
		if n > 1<<63 {
			return 0, false
		}
		return -int64(n), true
	}
	if n > math.MaxInt64 {
		return 0, false
	}
	return int64(n), true
}

// parseUint parses like strconv.ParseUint(s, 10, 64).
func parseUint(b []byte) (uint64, bool) {
	if len(b) == 0 || len(b) > 19 {
		n, err := strconv.ParseUint(unsafeString(b), 10, 64)
		return n, err == nil
	}
	var n uint64
	for _, c := range b {
		c -= '0'
		if c > 9 {
			return 0, false
		}
		n = n*10 + uint64(c)
	}
	return n, true
}

var float64pow10 = [...]float64{
	1e0, 1e1, 1e2, 1e3, 1e4, 1e5, 1e6, 1e7, 1e8, 1e9,
	1e10, 1e11, 1e12, 1e13, 1e14, 1e15, 1e16, 1e17, 1e18, 1e19,
	1e20, 1e21, 1e22,
}

var float32pow10 = [...]float32{1e0, 1e1, 1e2, 1e3, 1e4, 1e5, 1e6, 1e7, 1e8, 1e9, 1e10}

// decimalParts splits a JSON number literal into a mantissa and a decimal
// exponent. ok is false when the mantissa does not fit in 19 digits or the
// exponent is out of the fast-path range.
func decimalParts(b []byte) (mant uint64, exp int, neg, ok bool) {
	i := 0
	if b[0] == '-' {
		neg = true
		i++
	}
	nd := 0
	for ; i < len(b) && b[i] >= '0' && b[i] <= '9'; i++ {
		if nd > 0 || b[i] != '0' {
			nd++
		}
		mant = mant*10 + uint64(b[i]-'0')
	}
	if i < len(b) && b[i] == '.' {
		i++
		for ; i < len(b) && b[i] >= '0' && b[i] <= '9'; i++ {
			if nd > 0 || b[i] != '0' {
				nd++
			}
			mant = mant*10 + uint64(b[i]-'0')
			exp--
		}
	}
	if nd > 19 {
		return 0, 0, neg, false
	}
	if i < len(b) {
		i++ // 'e' or 'E'
		esign := 1
		if b[i] == '+' {
			i++
		} else if b[i] == '-' {
			esign = -1
			i++
		}
		e := 0
		for ; i < len(b); i++ {
			if e > 1000 {
				return 0, 0, neg, false
			}
			e = e*10 + int(b[i]-'0')
		}
		exp += esign * e
	}
	return mant, exp, neg, true
}

// parseFloat parses a JSON number literal like strconv.ParseFloat, using an
// exact fast path for the common case of short decimals.
func parseFloat(b []byte, bitSize int) (float64, error) {
	if mant, exp, neg, ok := decimalParts(b); ok {
		if bitSize == 64 && mant < 1<<53 && exp >= -22 && exp <= 22 {
			f := float64(mant)
			if exp < 0 {
				f /= float64pow10[-exp]
			} else {
				f *= float64pow10[exp]
			}
			if neg {
				f = -f
			}
			return f, nil
		}
		if bitSize == 32 && mant < 1<<24 && exp >= -10 && exp <= 10 {
			f := float32(mant)
			if exp < 0 {
				f /= float32pow10[-exp]
			} else {
				f *= float32pow10[exp]
			}
			if neg {
				f = -f
			}
			return float64(f), nil
		}
	}
	return strconv.ParseFloat(unsafeString(b), bitSize)
}
