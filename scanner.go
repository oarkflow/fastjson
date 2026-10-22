package fastjson

import (
	"encoding/binary"
	"math/bits"
	"sync"
)

const maxNestingDepth = 10000

// Character classes used by the scanners.
const (
	ccSpace = 1 << iota
	ccDigit
	ccHex
	ccNumChar    // digits, '.', 'e', 'E', '+', '-'
	ccStrSpecial // '"', '\\' and control characters
)

var charClass = func() (t [256]uint8) {
	for _, c := range []byte(" \t\r\n") {
		t[c] |= ccSpace
	}
	for c := '0'; c <= '9'; c++ {
		t[c] |= ccDigit | ccHex | ccNumChar
	}
	for c := 'a'; c <= 'f'; c++ {
		t[c] |= ccHex
		t[c-'a'+'A'] |= ccHex
	}
	for _, c := range []byte(".eE+-") {
		t[c] |= ccNumChar
	}
	t['"'] |= ccStrSpecial
	t['\\'] |= ccStrSpecial
	for c := 0; c < 0x20; c++ {
		t[c] |= ccStrSpecial
	}
	return t
}()

func isSpace(c byte) bool { return charClass[c]&ccSpace != 0 }

func skipSpace(b []byte, i int) int {
	for i < len(b) && charClass[b[i]]&ccSpace != 0 {
		i++
	}
	return i
}

// SWAR helpers operating on 8 bytes at a time.
const (
	lsb = 0x0101010101010101
	msb = 0x8080808080808080
)

// hasZero returns a word whose high bit is set in the first byte of x that is
// zero (higher bytes may contain false positives, which callers tolerate).
func hasZero(x uint64) uint64 { return (x - lsb) & ^x & msb }

func hasByte(x uint64, c byte) uint64 { return hasZero(x ^ (lsb * uint64(c))) }

// hasLess returns a non-zero word if any byte of x is less than n (n <= 128).
func hasLess(x uint64, n byte) uint64 { return (x - lsb*uint64(n)) & ^x & msb }

// stringSpecial finds the index of the first '"', '\\' or control character
// at or after b[i] in a JSON string body. It returns len(b) if none is found.
func stringSpecial(b []byte, i int) int {
	for i+8 <= len(b) {
		w := binary.LittleEndian.Uint64(b[i:])
		if m := hasByte(w, '"') | hasByte(w, '\\') | hasLess(w, 0x20); m != 0 {
			return i + bits.TrailingZeros64(m)/8
		}
		i += 8
	}
	for i < len(b) {
		if charClass[b[i]]&ccStrSpecial != 0 {
			return i
		}
		i++
	}
	return i
}

// Scanner states for valueScanner.
const (
	stValue        = iota // expecting a value
	stValueOrClose        // just after '[': value or ']'
	stKeyOrClose          // just after '{': key or '}'
	stKey                 // after ',' in an object: key
	stColon               // after an object key
	stCommaOrClose        // after a value inside a container
)

type scanResult uint8

const (
	scanDone     scanResult = iota // a complete top-level value was scanned
	scanNeedMore                   // input ended before the value did
	scanInvalid                    // input is not valid JSON
)

// valueScanner is a fast, resumable JSON validator. It only decides validity
// and where a value ends; exact error messages come from stdScanner, which
// is run on the error path.
type valueScanner struct {
	stack    []byte // open containers: '{' or '['
	state    uint8
	inString bool  // suspended inside a string body
	afterStr uint8 // state to resume after the suspended string
}

func (s *valueScanner) reset() {
	s.stack = s.stack[:0]
	s.state = stValue
	s.inString = false
}

// scan continues validating b starting at i. It returns the offset just past
// the completed top-level value (scanDone), the offset at which to resume
// once more data is available (scanNeedMore), or scanInvalid. When eof is
// true the input is known to be complete, so a trailing number is accepted.
func (s *valueScanner) scan(b []byte, i int, eof bool) (int, scanResult) {
	state := s.state
	if s.inString {
		j, res := scanStringBody(b, i)
		if res != scanDone {
			if res == scanNeedMore && !eof {
				return j, scanNeedMore
			}
			return j, scanInvalid
		}
		s.inString = false
		i = j
		if s.afterStr == stColon {
			state = stColon
		} else {
			if len(s.stack) == 0 {
				return i, scanDone
			}
			state = stCommaOrClose
		}
	}
	for {
		for i < len(b) && charClass[b[i]]&ccSpace != 0 {
			i++
		}
		if i >= len(b) {
			s.state = state
			if eof {
				return i, scanInvalid
			}
			return i, scanNeedMore
		}
		c := b[i]
		switch state {
		case stColon:
			if c != ':' {
				return i, scanInvalid
			}
			i++
			state = stValue
			continue
		case stCommaOrClose:
			top := s.stack[len(s.stack)-1]
			if c == ',' {
				i++
				if top == '{' {
					state = stKey
				} else {
					state = stValue
				}
				continue
			}
			if c == top+2 { // '[' + 2 == ']' and '{' + 2 == '}'
				i++
				s.stack = s.stack[:len(s.stack)-1]
				if len(s.stack) == 0 {
					return i, scanDone
				}
				continue
			}
			return i, scanInvalid
		case stKeyOrClose:
			if c == '}' {
				i++
				s.stack = s.stack[:len(s.stack)-1]
				if len(s.stack) == 0 {
					return i, scanDone
				}
				state = stCommaOrClose
				continue
			}
			fallthrough
		case stKey:
			if c != '"' {
				return i, scanInvalid
			}
			j, res := scanStringBody(b, i+1)
			if res != scanDone {
				if res == scanNeedMore && !eof {
					s.inString, s.afterStr, s.state = true, stColon, state
					return j, scanNeedMore
				}
				return j, scanInvalid
			}
			i = j
			state = stColon
			continue
		case stValueOrClose:
			if c == ']' {
				i++
				s.stack = s.stack[:len(s.stack)-1]
				if len(s.stack) == 0 {
					return i, scanDone
				}
				state = stCommaOrClose
				continue
			}
		}
		// stValue (or stValueOrClose with a value)
		switch c {
		case '{', '[':
			if len(s.stack) >= maxNestingDepth {
				return i, scanInvalid
			}
			s.stack = append(s.stack, c)
			i++
			if c == '{' {
				state = stKeyOrClose
			} else {
				state = stValueOrClose
			}
			continue
		case '"':
			j, res := scanStringBody(b, i+1)
			if res != scanDone {
				if res == scanNeedMore && !eof {
					s.inString, s.afterStr, s.state = true, stCommaOrClose, state
					return j, scanNeedMore
				}
				return j, scanInvalid
			}
			i = j
		case 't', 'f', 'n':
			lit := "true"
			if c == 'f' {
				lit = "false"
			} else if c == 'n' {
				lit = "null"
			}
			for k := 1; k < len(lit); k++ {
				if i+k >= len(b) {
					s.state = state
					if eof {
						return i, scanInvalid
					}
					return i, scanNeedMore
				}
				if b[i+k] != lit[k] {
					return i + k, scanInvalid
				}
			}
			i += len(lit)
		default:
			j, res := scanNumber(b, i, eof)
			if res != scanDone {
				if res == scanNeedMore {
					s.state = state
				}
				return j, res
			}
			i = j
		}
		if len(s.stack) == 0 {
			return i, scanDone
		}
		state = stCommaOrClose
	}
}

// scanStringBody validates a string body starting just after the opening
// quote and returns the offset past the closing quote. On scanNeedMore the
// returned offset is a safe point to resume from (never inside an escape).
func scanStringBody(b []byte, i int) (int, scanResult) {
	for {
		i = stringSpecial(b, i)
		if i >= len(b) {
			return i, scanNeedMore
		}
		switch c := b[i]; {
		case c == '"':
			return i + 1, scanDone
		case c < 0x20:
			return i, scanInvalid
		}
		// backslash escape
		if i+1 >= len(b) {
			return i, scanNeedMore
		}
		switch b[i+1] {
		case '"', '\\', '/', 'b', 'f', 'n', 'r', 't':
			i += 2
		case 'u':
			for k := 2; k < 6; k++ {
				if i+k >= len(b) {
					return i, scanNeedMore
				}
				if charClass[b[i+k]]&ccHex == 0 {
					return i + k, scanInvalid
				}
			}
			i += 6
		default:
			return i + 1, scanInvalid
		}
	}
}

// scanNumber validates a number starting at b[i]. Without eof, a number that
// runs to the end of b needs more data and is rescanned from i later.
func scanNumber(b []byte, i int, eof bool) (int, scanResult) {
	start := i
	more := func() (int, scanResult) {
		if eof {
			return len(b), scanInvalid
		}
		return start, scanNeedMore
	}
	if b[i] == '-' {
		i++
		if i >= len(b) {
			return more()
		}
	}
	switch c := b[i]; {
	case c == '0':
		i++
	case '1' <= c && c <= '9':
		i++
		for i < len(b) && charClass[b[i]]&ccDigit != 0 {
			i++
		}
	default:
		return i, scanInvalid
	}
	if i >= len(b) {
		if eof {
			return i, scanDone
		}
		return start, scanNeedMore
	}
	if b[i] == '.' {
		i++
		if i >= len(b) {
			return more()
		}
		if charClass[b[i]]&ccDigit == 0 {
			return i, scanInvalid
		}
		for i < len(b) && charClass[b[i]]&ccDigit != 0 {
			i++
		}
		if i >= len(b) {
			if eof {
				return i, scanDone
			}
			return start, scanNeedMore
		}
	}
	if b[i] == 'e' || b[i] == 'E' {
		i++
		if i >= len(b) {
			return more()
		}
		if b[i] == '+' || b[i] == '-' {
			i++
			if i >= len(b) {
				return more()
			}
		}
		if charClass[b[i]]&ccDigit == 0 {
			return i, scanInvalid
		}
		for i < len(b) && charClass[b[i]]&ccDigit != 0 {
			i++
		}
		if i >= len(b) && !eof {
			return start, scanNeedMore
		}
	}
	return i, scanDone
}

var valueScannerPool = sync.Pool{New: func() any { return new(valueScanner) }}

func getValueScanner() *valueScanner {
	s := valueScannerPool.Get().(*valueScanner)
	s.reset()
	return s
}

func putValueScanner(s *valueScanner) {
	if cap(s.stack) > 1024 {
		s.stack = nil
	}
	valueScannerPool.Put(s)
}

// valid reports whether data is exactly one JSON value surrounded by
// optional whitespace.
func valid(data []byte) bool {
	s := getValueScanner()
	n, res := s.scan(data, 0, true)
	putValueScanner(s)
	return res == scanDone && skipSpace(data, n) == len(data)
}

// checkValid validates data and returns the exact encoding/json syntax error
// when it is invalid.
func checkValid(data []byte) error {
	if valid(data) {
		return nil
	}
	return stdSyntaxError(data, 0, true)
}
