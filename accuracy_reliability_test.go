package fastjson

import (
	"bytes"
	stdjson "encoding/json"
	"errors"
	"io"
	"math"
	"reflect"
	"strings"
	"testing"
)

type AccuracyAddress struct {
	City    string `json:"city"`
	Zip     int    `json:"zip"`
	Country string `json:"country,omitempty"`
}
type AccuracyUser struct {
	ID        int             `json:"id"`
	Name      string          `json:"name"`
	Email     string          `json:"email,omitempty"`
	Active    bool            `json:"active"`
	Score     float64         `json:"score"`
	Tags      []string        `json:"tags"`
	Numbers   []int           `json:"numbers"`
	Address   AccuracyAddress `json:"address"`
	CreatedAt int64           `json:"created_at"`
	Ignored   string          `json:"-"`
}

func sampleAccuracyUser() AccuracyUser {
	return AccuracyUser{ID: 42, Name: "Alice \"Admin\" \n Unicode: Nepal", Email: "alice@example.com", Active: true, Score: 99.75, Tags: []string{"go", "json", "fast"}, Numbers: []int{1, 2, 3, 1000, -99}, Address: AccuracyAddress{City: "Kathmandu", Zip: 44600, Country: "Nepal"}, CreatedAt: 1710000000, Ignored: "no"}
}
func assertJSONEqual(t *testing.T, got, want []byte) {
	t.Helper()
	var g, w any
	if err := stdjson.Unmarshal(got, &g); err != nil {
		t.Fatalf("got invalid %s %v", got, err)
	}
	if err := stdjson.Unmarshal(want, &w); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(g, w) {
		t.Fatalf("json mismatch\ngot %s\nwant %s", got, want)
	}
}
func TestAccuracyMarshalStructMatchesStdlibCanonical(t *testing.T) {
	in := sampleAccuracyUser()
	got, err := Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := stdjson.Marshal(in)
	assertJSONEqual(t, got, want)
	if bytes.Contains(got, []byte("no")) {
		t.Fatal("ignored leaked")
	}
}
func TestAccuracyUnmarshalStructMatchesStdlib(t *testing.T) {
	src := []byte(`{"id":42,"name":"Alice","email":"alice@example.com","active":true,"score":99.75,"tags":["go", "json", "fast"],"numbers":[1, 2, 3, 1000, -99],"address":{"city":"Kathmandu","zip":44600,"country":"Nepal"},"created_at":1710000000,"unknown":"x"}`)
	var got, want AccuracyUser
	if err := Unmarshal(src, &got); err != nil {
		t.Fatal(err)
	}
	if err := stdjson.Unmarshal(src, &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}
func TestAccuracyRoundTripStruct(t *testing.T) {
	in := sampleAccuracyUser()
	b, err := Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out AccuracyUser
	if err := Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	in.Ignored = ""
	if !reflect.DeepEqual(out, in) {
		t.Fatalf("got %#v want %#v", out, in)
	}
}
func TestReliabilityDecoderMultipleValues(t *testing.T) {
	dec := NewDecoder(strings.NewReader(`{"id":1,"name":"A","active":true,"score":1,"tags":[],"numbers":[],"address":{"city":"X","zip":1},"created_at":1} {"id":2,"name":"B","active":false,"score":2,"tags":["x"],"numbers":[9],"address":{"city":"Y","zip":2},"created_at":2}`))
	var a, b AccuracyUser
	if err := dec.Decode(&a); err != nil {
		t.Fatal(err)
	}
	if err := dec.Decode(&b); err != nil {
		t.Fatal(err)
	}
	if a.ID != 1 || b.ID != 2 {
		t.Fatal(a, b)
	}
	var c AccuracyUser
	if err := dec.Decode(&c); err != io.EOF {
		t.Fatalf("want eof got %v", err)
	}
}
func TestAccuracyValidMatchesStdlib(t *testing.T) {
	cases := []string{`null`, `true`, `false`, `0`, `-1`, `3.14`, `"x"`, `[]`, `[1,2,3]`, `{}`, `{"a":1}`, ``, `01`, `[1,]`, `{"a":}`}
	for _, s := range cases {
		if Valid([]byte(s)) != stdjson.Valid([]byte(s)) {
			t.Fatalf("valid mismatch %q", s)
		}
	}
}

type AllTypes struct {
	A int      `json:"a"`
	B int8     `json:"b"`
	C int16    `json:"c"`
	D int32    `json:"d"`
	E int64    `json:"e"`
	F uint     `json:"f"`
	G uint8    `json:"g"`
	H uint16   `json:"h"`
	I uint32   `json:"i"`
	J uint64   `json:"j"`
	K float32  `json:"k"`
	L float64  `json:"l"`
	M bool     `json:"m"`
	N string   `json:"n"`
	O []string `json:"o"`
	P []int    `json:"p"`
	Q string   `json:"-"`
}

func TestAllNumericTypesMarshalUnmarshal(t *testing.T) {
	in := AllTypes{
		A: -1, B: -2, C: -3, D: -4, E: -5,
		F: 1, G: 2, H: 3, I: 4, J: 5,
		K: 1.5, L: 2.5, M: true, N: "hello",
		O: []string{"x", "y"}, P: []int{10, 20},
	}
	b, err := Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out AllTypes
	if err := Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Fatalf("round-trip failed\ngot  %#v\nwant %#v", out, in)
	}
}

type NullableStruct struct {
	A *int    `json:"a"`
	B *string `json:"b"`
	C *bool   `json:"c"`
}

func TestNullablePointerFields(t *testing.T) {
	v := NullableStruct{A: nil, B: nil, C: nil}
	b, err := Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"a":null,"b":null,"c":null}`
	if string(b) != want {
		t.Fatalf("got %s want %s", b, want)
	}
	var out NullableStruct
	if err := Unmarshal([]byte(want), &out); err != nil {
		t.Fatal(err)
	}
	if out.A != nil || out.B != nil || out.C != nil {
		t.Fatal("expected all nil")
	}

	x := 42
	y := "hi"
	z := true
	v2 := NullableStruct{A: &x, B: &y, C: &z}
	b2, err := Marshal(v2)
	if err != nil {
		t.Fatal(err)
	}
	var out2 NullableStruct
	if err := Unmarshal(b2, &out2); err != nil {
		t.Fatal(err)
	}
	if *out2.A != 42 || *out2.B != "hi" || *out2.C != true {
		t.Fatal("pointer round-trip failed")
	}
}

func TestAccuracyOmitEmpty(t *testing.T) {
	type S struct {
		A string  `json:"a,omitempty"`
		B int     `json:"b,omitempty"`
		C bool    `json:"c,omitempty"`
		D float64 `json:"d,omitempty"`
	}
	v := S{}
	b, err := Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "{}" {
		t.Fatalf("got %s want {}", b)
	}
}

func TestEmptyArrayAndObject(t *testing.T) {
	b, err := Marshal([]string{})
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "[]" {
		t.Fatalf("got %s want []", b)
	}
	b2, err := Marshal(map[string]int{})
	if err != nil {
		t.Fatal(err)
	}
	if string(b2) != "{}" {
		t.Fatalf("got %s want {}", b2)
	}
}

func TestStringUnicodeEscapes(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"he\nllo", `"he\nllo"`},
		{"he\tllo", `"he\tllo"`},
		{"he\rllo", `"he\rllo"`},
		{"he\bllo", `"he\bllo"`},
		{"he\fllo", `"he\fllo"`},
		{"he/llo", `"he/llo"`},
		{"he\\llo", `"he\\llo"`},
		{"he\"llo", `"he\"llo"`},
		{"hello", `"hello"`},
		{"\u00e9", `"é"`},
	}
	for _, tt := range tests {
		b := appendString(nil, tt.input, false)
		if string(b) != tt.want {
			t.Errorf("appendString(%q) = %s, want %s", tt.input, b, tt.want)
		}
	}
}

func TestStringParseRoundTrip(t *testing.T) {
	inputs := []string{
		"hello",
		"hello\nworld",
		"hello\tworld",
		"hello\rworld",
		"hello\"world",
		"hello\\world",
		"hello\x00world",
		"café",
		"\\u00e9",
		"\u00e9\u00e9\u00e9",
	}
	for _, s := range inputs {
		encoded := appendString(nil, s, false)
		parsed, _, err := parseString(encoded, 0)
		if err != nil {
			t.Errorf("parseString(%s) error: %v", encoded, err)
			continue
		}
		if parsed != s {
			t.Errorf("round-trip failed: encoded %s, got %q, want %q", encoded, parsed, s)
		}
	}
}

func TestUnicodeSurrogatePair(t *testing.T) {
	input := `"\ud83d\ude00"` // grinning face emoji U+1F600
	got, _, err := parseString([]byte(input), 0)
	if err != nil {
		t.Fatal(err)
	}
	want := "\U0001F600"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}

	encoded := appendString(nil, want, false)
	parsed, _, err := parseString(encoded, 0)
	if err != nil {
		t.Fatal(err)
	}
	if parsed != want {
		t.Fatalf("round-trip: got %q want %q", parsed, want)
	}
}

func TestNumberOverflow(t *testing.T) {
	_, err := parseIntBytes([]byte("9999999999999999999999999999"))
	if err == nil {
		t.Fatal("expected overflow error")
	}
	_, err = parseIntBytes([]byte("-9999999999999999999999999999"))
	if err == nil {
		t.Fatal("expected overflow error")
	}
	_, err = parseUintBytes([]byte("9999999999999999999999999999"))
	if err == nil {
		t.Fatal("expected overflow error")
	}
}

func TestCompactMatchesStdlib(t *testing.T) {
	tests := []string{
		`{"a": 1, "b": [2, 3]}`,
		`[1, 2, 3]`,
		`null`,
		`true`,
		`"hello"`,
		`{"nested": {"deep": [1, {"x": "y"}]}}`,
	}
	for _, s := range tests {
		src := []byte(s)
		var ourBuf bytes.Buffer
		var stdBuf bytes.Buffer
		ourErr := Compact(&ourBuf, src)
		stdErr := stdjson.Compact(&stdBuf, src)
		if (ourErr == nil) != (stdErr == nil) {
			t.Errorf("Compact(%q): our err=%v std err=%v", s, ourErr, stdErr)
			continue
		}
		if ourErr == nil && ourBuf.String() != stdBuf.String() {
			t.Errorf("Compact(%q): got %q want %q", s, ourBuf.String(), stdBuf.String())
		}
	}
}

func TestIndentMatchesStdlib(t *testing.T) {
	src := []byte(`{"a":1,"b":[2,3],"c":{"d":4}}`)
	var ourBuf, stdBuf bytes.Buffer
	ourErr := Indent(&ourBuf, src, "", "  ")
	stdErr := stdjson.Indent(&stdBuf, src, "", "  ")
	if (ourErr == nil) != (stdErr == nil) {
		t.Fatalf("Indent: our err=%v std err=%v", ourErr, stdErr)
	}
	if ourErr == nil && ourBuf.String() != stdBuf.String() {
		t.Fatalf("Indent mismatch\nour:\n%s\nstd:\n%s", ourBuf.String(), stdBuf.String())
	}
}

func TestAccuracyHTMLEscape(t *testing.T) {
	src := []byte(`{"x":"<hello> & world"}`)
	var ourBuf, stdBuf bytes.Buffer
	HTMLEscape(&ourBuf, src)
	stdjson.HTMLEscape(&stdBuf, src)
	if ourBuf.String() != stdBuf.String() {
		t.Fatalf("HTMLEscape: got %q want %q", ourBuf.String(), stdBuf.String())
	}
}

type HTMLStruct struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

func TestHTMLEscapeMarshalDefault(t *testing.T) {
	v := HTMLStruct{Name: "test", Value: "<hidden>"}
	b, err := Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := stdjson.Marshal(v)
	if !bytes.Equal(b, want) {
		t.Fatalf("Marshal should escape HTML like encoding/json: got %s want %s", b, want)
	}
	b, err = MarshalNoEscape(v)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(b, []byte(`<hidden>`)) {
		t.Fatalf("MarshalNoEscape should contain raw <hidden>, got: %s", b)
	}
}

func TestHTMLEscapeMarshalEscape(t *testing.T) {
	v := HTMLStruct{Name: "test", Value: "<hidden>"}
	b, err := MarshalEscape(v)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(b, []byte(`\u003c`)) {
		t.Fatalf("MarshalEscape should escape '<', got: %s", b)
	}
	if !bytes.Contains(b, []byte(`\u003e`)) {
		t.Fatalf("MarshalEscape should escape '>', got: %s", b)
	}
}

func TestHTMLEscapeEncoderToggle(t *testing.T) {
	v := HTMLStruct{Name: "test", Value: "<hidden>"}

	// Default: HTML escaped, as with encoding/json
	var buf1 bytes.Buffer
	enc1 := NewEncoder(&buf1)
	if err := enc1.Encode(v); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(buf1.Bytes(), []byte(`\u003c`)) {
		t.Fatalf("Encoder default should escape HTML, got: %s", buf1.String())
	}

	// SetEscapeHTML(true): should escape
	var buf2 bytes.Buffer
	enc2 := NewEncoder(&buf2)
	enc2.SetEscapeHTML(true)
	if err := enc2.Encode(v); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(buf2.Bytes(), []byte(`\u003c`)) {
		t.Fatalf("Encoder SetEscapeHTML(true) should escape HTML, got: %s", buf2.String())
	}

	// SetEscapeHTML(false): should NOT escape
	var buf3 bytes.Buffer
	enc3 := NewEncoder(&buf3)
	enc3.SetEscapeHTML(true)
	enc3.SetEscapeHTML(false)
	if err := enc3.Encode(v); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(buf3.Bytes(), []byte(`\u003c`)) {
		t.Fatalf("Encoder SetEscapeHTML(false) should NOT escape HTML, got: %s", buf3.String())
	}
}

func TestHTMLEscapeAppenderToggle(t *testing.T) {
	// Default: no HTML escape
	a := NewAppender(nil)
	a.String("<hidden>")
	if bytes.Contains(a.Bytes(), []byte(`\u003c`)) {
		t.Fatalf("Appender default should NOT escape HTML, got: %s", a.Bytes())
	}
	if string(a.Bytes()) != `"<hidden>"` {
		t.Fatalf("Appender default should contain raw <hidden>, got: %s", a.Bytes())
	}

	// EscapeHTML: true
	a2 := NewAppender(nil)
	a2.EscapeHTML = true
	a2.String("<hidden>")
	if !bytes.Contains(a2.Bytes(), []byte(`\u003c`)) {
		t.Fatalf("Appender EscapeHTML=true should escape HTML, got: %s", a2.Bytes())
	}
}

func TestHTMLEscapeMapAndSlice(t *testing.T) {
	m := map[string]string{"key": "<hidden>"}

	// MarshalNoEscape: no escape
	b, err := MarshalNoEscape(m)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(b, []byte(`\u003c`)) {
		t.Fatalf("MarshalNoEscape map should NOT escape HTML, got: %s", b)
	}

	// MarshalEscape: escape
	b2, err := MarshalEscape(m)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(b2, []byte(`\u003c`)) {
		t.Fatalf("MarshalEscape map should escape HTML, got: %s", b2)
	}

	// Slice
	s := []string{"<hidden>"}
	b3, err := MarshalNoEscape(s)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(b3, []byte(`\u003c`)) {
		t.Fatalf("MarshalNoEscape slice should NOT escape HTML, got: %s", b3)
	}

	b4, err := MarshalEscape(s)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(b4, []byte(`\u003c`)) {
		t.Fatalf("MarshalEscape slice should escape HTML, got: %s", b4)
	}
}

func TestMarshalIndent(t *testing.T) {
	in := sampleAccuracyUser()
	got, err := MarshalIndent(in, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	want, err := stdjson.MarshalIndent(in, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	assertJSONEqual(t, got, want)
}

func TestEncoderDecoderStreaming(t *testing.T) {
	var buf bytes.Buffer
	enc := NewEncoder(&buf)
	if err := enc.Encode(sampleAccuracyUser()); err != nil {
		t.Fatal(err)
	}
	dec := NewDecoder(&buf)
	var out AccuracyUser
	if err := dec.Decode(&out); err != nil {
		t.Fatal(err)
	}
}

func TestAccuracyRawMessage(t *testing.T) {
	src := []byte(`{"raw":{"a":1},"normal":42}`)
	var got struct {
		Raw    RawMessage `json:"raw"`
		Normal int        `json:"normal"`
	}
	if err := Unmarshal(src, &got); err != nil {
		t.Fatal(err)
	}
	if string(got.Raw) != `{"a":1}` {
		t.Fatalf("raw: got %q", got.Raw)
	}
	got.Raw = RawMessage(`[1,2,3]`)
	b, err := Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(b, []byte(`[1,2,3]`)) {
		t.Fatalf("marshal raw: got %s", b)
	}
}

func TestNullUnmarshal(t *testing.T) {
	var v *AccuracyUser
	if err := Unmarshal([]byte(`null`), &v); err != nil {
		t.Fatal(err)
	}
	if v != nil {
		t.Fatal("expected nil")
	}
}

func TestDisallowUnknownFields(t *testing.T) {
	src := []byte(`{"id":1,"unknown":"x"}`)
	dec := NewDecoder(bytes.NewReader(src))
	dec.DisallowUnknownFields()
	var v AccuracyUser
	err := dec.Decode(&v)
	if err == nil {
		t.Fatal("expected error for unknown field")
	}
}

func TestUseNumber(t *testing.T) {
	src := []byte(`{"val":12345678901234567890}`)
	dec := NewDecoder(bytes.NewReader(src))
	dec.UseNumber()
	var got map[string]any
	if err := dec.Decode(&got); err != nil {
		t.Fatal(err)
	}
	_, ok := got["val"].(Number)
	if !ok {
		t.Fatalf("expected Number, got %T", got["val"])
	}
}

func TestMapStringAny(t *testing.T) {
	in := map[string]any{"a": float64(1), "b": "hello", "c": true, "d": nil, "e": []any{float64(1), "x"}}
	b, err := Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Fatalf("got %#v want %#v", out, in)
	}
}

func TestErrorMessages(t *testing.T) {
	var se *SyntaxError
	if !errors.As(newSyntaxError("test", 5), &se) {
		t.Fatal("expected SyntaxError")
	}
	if se.Offset != 5 || se.Error() != "test" {
		t.Fatalf("got %q at %d, want %q at 5", se.Error(), se.Offset, "test")
	}
	// The error from a real syntax problem must match encoding/json exactly.
	in := []byte(`{"a": tru}`)
	err := Unmarshal(in, new(any))
	stdErr := stdjson.Unmarshal(in, new(any))
	var stdSE *stdjson.SyntaxError
	if !errors.As(err, &se) || !errors.As(stdErr, &stdSE) {
		t.Fatalf("expected *json.SyntaxError, got %T and %T", err, stdErr)
	}
	if se.Error() != stdSE.Error() || se.Offset != stdSE.Offset {
		t.Fatalf("got %q at %d, want %q at %d", se, se.Offset, stdSE, stdSE.Offset)
	}
}

// Test helpers standing in for parsers that used to be internal.
func parseString(b []byte, _ int) (string, int, error) {
	var s string
	err := Unmarshal(b, &s)
	return s, len(b), err
}

func parseIntBytes(b []byte) (int64, error) {
	var n int64
	return n, Unmarshal(b, &n)
}

func parseUintBytes(b []byte) (uint64, error) {
	var n uint64
	return n, Unmarshal(b, &n)
}

func TestLargeIntegerValues(t *testing.T) {
	var x int64 = math.MaxInt64
	b, err := Marshal(x)
	if err != nil {
		t.Fatal(err)
	}
	var out int64
	if err := Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if out != x {
		t.Fatalf("got %d want %d", out, x)
	}

	var y uint64 = math.MaxUint64
	b2, err := Marshal(y)
	if err != nil {
		t.Fatal(err)
	}
	var out2 uint64
	if err := Unmarshal(b2, &out2); err != nil {
		t.Fatal(err)
	}
	if out2 != y {
		t.Fatalf("got %d want %d", out2, y)
	}

	var z int64 = math.MinInt64
	b3, err := Marshal(z)
	if err != nil {
		t.Fatal(err)
	}
	var out3 int64
	if err := Unmarshal(b3, &out3); err != nil {
		t.Fatal(err)
	}
	if out3 != z {
		t.Fatalf("got %d want %d", out3, z)
	}
}
