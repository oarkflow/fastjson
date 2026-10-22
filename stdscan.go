// Portions of this file are adapted from the Go standard library
// (encoding/json/scanner.go). Copyright 2010 The Go Authors.
// Use of this source code is governed by the BSD-style license in LICENSE-GO.

package fastjson

import "strconv"

// stdScanner is a byte-at-a-time port of encoding/json's scanner. It is only
// used after the fast scanner has rejected input, to report the same syntax
// error message and offset that encoding/json reports.
type stdScanner struct {
	step       func(*stdScanner, byte) int
	endTop     bool
	parseState []int
	err        error
	bytes      int64
}

const (
	scanContinue = iota
	scanBeginLiteral
	scanBeginObject
	scanObjectKey
	scanObjectValue
	scanEndObject
	scanBeginArray
	scanArrayValue
	scanEndArray
	scanSkipSpace
	scanEnd
	scanError
)

const (
	parseObjectKey = iota
	parseObjectValue
	parseArrayValue
)

// stdSyntaxError scans data from its beginning and returns the syntax error
// encoding/json reports for it. bytes is the scanner's initial byte count and
// countBytes mirrors whether the std caller advances the count (Compact does
// not, so its offsets stay at the initial value).
func stdSyntaxError(data []byte, bytes int64, countBytes bool) error {
	s := &stdScanner{bytes: bytes}
	s.reset()
	for _, c := range data {
		if countBytes {
			s.bytes++
		}
		if s.step(s, c) == scanError {
			return s.err
		}
	}
	if s.eof() == scanError {
		return s.err
	}
	return newSyntaxError("invalid JSON", s.bytes)
}

// stdValueError scans one value at the start of data the way Decoder does and
// returns the first syntax error, or nil if data holds no error before its end.
func stdValueError(data []byte, bytes int64) error {
	s := &stdScanner{bytes: bytes}
	s.reset()
	for _, c := range data {
		s.bytes++
		switch s.step(s, c) {
		case scanEnd:
			return nil
		case scanEndObject, scanEndArray:
			if stateEndValue(s, ' ') == scanEnd {
				return nil
			}
		case scanError:
			return s.err
		}
	}
	return nil
}

func (s *stdScanner) reset() {
	s.step = stateBeginValue
	s.parseState = s.parseState[0:0]
	s.err = nil
	s.endTop = false
}

func (s *stdScanner) eof() int {
	if s.err != nil {
		return scanError
	}
	if s.endTop {
		return scanEnd
	}
	s.step(s, ' ')
	if s.endTop {
		return scanEnd
	}
	if s.err == nil {
		s.err = newSyntaxError("unexpected end of JSON input", s.bytes)
	}
	return scanError
}

func (s *stdScanner) pushParseState(c byte, newParseState int, successState int) int {
	s.parseState = append(s.parseState, newParseState)
	if len(s.parseState) <= maxNestingDepth {
		return successState
	}
	return s.error(c, "exceeded max depth")
}

func (s *stdScanner) popParseState() {
	n := len(s.parseState) - 1
	s.parseState = s.parseState[0:n]
	if n == 0 {
		s.step = stateEndTop
		s.endTop = true
	} else {
		s.step = stateEndValue
	}
}

func stateBeginValueOrEmpty(s *stdScanner, c byte) int {
	if isSpace(c) {
		return scanSkipSpace
	}
	if c == ']' {
		return stateEndValue(s, c)
	}
	return stateBeginValue(s, c)
}

func stateBeginValue(s *stdScanner, c byte) int {
	if isSpace(c) {
		return scanSkipSpace
	}
	switch c {
	case '{':
		s.step = stateBeginStringOrEmpty
		return s.pushParseState(c, parseObjectKey, scanBeginObject)
	case '[':
		s.step = stateBeginValueOrEmpty
		return s.pushParseState(c, parseArrayValue, scanBeginArray)
	case '"':
		s.step = stateInString
		return scanBeginLiteral
	case '-':
		s.step = stateNeg
		return scanBeginLiteral
	case '0':
		s.step = state0
		return scanBeginLiteral
	case 't':
		s.step = stateT
		return scanBeginLiteral
	case 'f':
		s.step = stateF
		return scanBeginLiteral
	case 'n':
		s.step = stateN
		return scanBeginLiteral
	}
	if '1' <= c && c <= '9' {
		s.step = state1
		return scanBeginLiteral
	}
	return s.error(c, "looking for beginning of value")
}

func stateBeginStringOrEmpty(s *stdScanner, c byte) int {
	if isSpace(c) {
		return scanSkipSpace
	}
	if c == '}' {
		n := len(s.parseState)
		s.parseState[n-1] = parseObjectValue
		return stateEndValue(s, c)
	}
	return stateBeginString(s, c)
}

func stateBeginString(s *stdScanner, c byte) int {
	if isSpace(c) {
		return scanSkipSpace
	}
	if c == '"' {
		s.step = stateInString
		return scanBeginLiteral
	}
	return s.error(c, "looking for beginning of object key string")
}

func stateEndValue(s *stdScanner, c byte) int {
	n := len(s.parseState)
	if n == 0 {
		s.step = stateEndTop
		s.endTop = true
		return stateEndTop(s, c)
	}
	if isSpace(c) {
		s.step = stateEndValue
		return scanSkipSpace
	}
	ps := s.parseState[n-1]
	switch ps {
	case parseObjectKey:
		if c == ':' {
			s.parseState[n-1] = parseObjectValue
			s.step = stateBeginValue
			return scanObjectKey
		}
		return s.error(c, "after object key")
	case parseObjectValue:
		if c == ',' {
			s.parseState[n-1] = parseObjectKey
			s.step = stateBeginString
			return scanObjectValue
		}
		if c == '}' {
			s.popParseState()
			return scanEndObject
		}
		return s.error(c, "after object key:value pair")
	case parseArrayValue:
		if c == ',' {
			s.step = stateBeginValue
			return scanArrayValue
		}
		if c == ']' {
			s.popParseState()
			return scanEndArray
		}
		return s.error(c, "after array element")
	}
	return s.error(c, "")
}

func stateEndTop(s *stdScanner, c byte) int {
	if !isSpace(c) {
		s.error(c, "after top-level value")
	}
	return scanEnd
}

func stateInString(s *stdScanner, c byte) int {
	if c == '"' {
		s.step = stateEndValue
		return scanContinue
	}
	if c == '\\' {
		s.step = stateInStringEsc
		return scanContinue
	}
	if c < 0x20 {
		return s.error(c, "in string literal")
	}
	return scanContinue
}

func stateInStringEsc(s *stdScanner, c byte) int {
	switch c {
	case 'b', 'f', 'n', 'r', 't', '\\', '/', '"':
		s.step = stateInString
		return scanContinue
	case 'u':
		s.step = stateInStringEscU
		return scanContinue
	}
	return s.error(c, "in string escape code")
}

func stateInStringEscU(s *stdScanner, c byte) int {
	if charClass[c]&ccHex != 0 {
		s.step = stateInStringEscU1
		return scanContinue
	}
	return s.error(c, "in \\u hexadecimal character escape")
}

func stateInStringEscU1(s *stdScanner, c byte) int {
	if charClass[c]&ccHex != 0 {
		s.step = stateInStringEscU12
		return scanContinue
	}
	return s.error(c, "in \\u hexadecimal character escape")
}

func stateInStringEscU12(s *stdScanner, c byte) int {
	if charClass[c]&ccHex != 0 {
		s.step = stateInStringEscU123
		return scanContinue
	}
	return s.error(c, "in \\u hexadecimal character escape")
}

func stateInStringEscU123(s *stdScanner, c byte) int {
	if charClass[c]&ccHex != 0 {
		s.step = stateInString
		return scanContinue
	}
	return s.error(c, "in \\u hexadecimal character escape")
}

func stateNeg(s *stdScanner, c byte) int {
	if c == '0' {
		s.step = state0
		return scanContinue
	}
	if '1' <= c && c <= '9' {
		s.step = state1
		return scanContinue
	}
	return s.error(c, "in numeric literal")
}

func state1(s *stdScanner, c byte) int {
	if '0' <= c && c <= '9' {
		s.step = state1
		return scanContinue
	}
	return state0(s, c)
}

func state0(s *stdScanner, c byte) int {
	if c == '.' {
		s.step = stateDot
		return scanContinue
	}
	if c == 'e' || c == 'E' {
		s.step = stateE
		return scanContinue
	}
	return stateEndValue(s, c)
}

func stateDot(s *stdScanner, c byte) int {
	if '0' <= c && c <= '9' {
		s.step = stateDot0
		return scanContinue
	}
	return s.error(c, "after decimal point in numeric literal")
}

func stateDot0(s *stdScanner, c byte) int {
	if '0' <= c && c <= '9' {
		return scanContinue
	}
	if c == 'e' || c == 'E' {
		s.step = stateE
		return scanContinue
	}
	return stateEndValue(s, c)
}

func stateE(s *stdScanner, c byte) int {
	if c == '+' || c == '-' {
		s.step = stateESign
		return scanContinue
	}
	return stateESign(s, c)
}

func stateESign(s *stdScanner, c byte) int {
	if '0' <= c && c <= '9' {
		s.step = stateE0
		return scanContinue
	}
	return s.error(c, "in exponent of numeric literal")
}

func stateE0(s *stdScanner, c byte) int {
	if '0' <= c && c <= '9' {
		return scanContinue
	}
	return stateEndValue(s, c)
}

func stateT(s *stdScanner, c byte) int {
	if c == 'r' {
		s.step = stateTr
		return scanContinue
	}
	return s.error(c, "in literal true (expecting 'r')")
}

func stateTr(s *stdScanner, c byte) int {
	if c == 'u' {
		s.step = stateTru
		return scanContinue
	}
	return s.error(c, "in literal true (expecting 'u')")
}

func stateTru(s *stdScanner, c byte) int {
	if c == 'e' {
		s.step = stateEndValue
		return scanContinue
	}
	return s.error(c, "in literal true (expecting 'e')")
}

func stateF(s *stdScanner, c byte) int {
	if c == 'a' {
		s.step = stateFa
		return scanContinue
	}
	return s.error(c, "in literal false (expecting 'a')")
}

func stateFa(s *stdScanner, c byte) int {
	if c == 'l' {
		s.step = stateFal
		return scanContinue
	}
	return s.error(c, "in literal false (expecting 'l')")
}

func stateFal(s *stdScanner, c byte) int {
	if c == 's' {
		s.step = stateFals
		return scanContinue
	}
	return s.error(c, "in literal false (expecting 's')")
}

func stateFals(s *stdScanner, c byte) int {
	if c == 'e' {
		s.step = stateEndValue
		return scanContinue
	}
	return s.error(c, "in literal false (expecting 'e')")
}

func stateN(s *stdScanner, c byte) int {
	if c == 'u' {
		s.step = stateNu
		return scanContinue
	}
	return s.error(c, "in literal null (expecting 'u')")
}

func stateNu(s *stdScanner, c byte) int {
	if c == 'l' {
		s.step = stateNul
		return scanContinue
	}
	return s.error(c, "in literal null (expecting 'l')")
}

func stateNul(s *stdScanner, c byte) int {
	if c == 'l' {
		s.step = stateEndValue
		return scanContinue
	}
	return s.error(c, "in literal null (expecting 'l')")
}

func stateError(s *stdScanner, c byte) int { return scanError }

func (s *stdScanner) error(c byte, context string) int {
	s.step = stateError
	s.err = newSyntaxError("invalid character "+quoteChar(c)+" "+context, s.bytes)
	return scanError
}

func quoteChar(c byte) string {
	if c == '\'' {
		return `'\''`
	}
	if c == '"' {
		return `'"'`
	}
	s := strconv.Quote(string(c))
	return "'" + s[1:len(s)-1] + "'"
}
