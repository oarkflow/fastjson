package fastjson

import (
	"bytes"
	"errors"
	stdjson "encoding/json"
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
	A  int      `json:"a"`
	B  int8     `json:"b"`
	C  int16    `json:"c"`
	D  int32    `json:"d"`
	E  int64    `json:"e"`
	F  uint     `json:"f"`
	G  uint8    `json:"g"`
	H  uint16   `json:"h"`
	I  uint32   `json:"i"`
	J  uint64   `json:"j"`
	K  float32  `json:"k"`
	L  float64  `json:"l"`
	M  bool     `json:"m"`
	N  string   `json:"n"`
	O  []string `json:"o"`
	P  []int    `json:"p"`
	Q  string   `json:"-"`
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

func TestOmitEmpty(t *testing.T) {
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

func TestHTMLEscape(t *testing.T) {
	src := []byte(`{"x":"<hello> & world"}`)
	var ourBuf, stdBuf bytes.Buffer
	HTMLEscape(&ourBuf, src)
	stdjson.HTMLEscape(&stdBuf, src)
	if ourBuf.String() != stdBuf.String() {
		t.Fatalf("HTMLEscape: got %q want %q", ourBuf.String(), stdBuf.String())
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

func TestRawMessage(t *testing.T) {
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
	if !errors.As(syntaxErr("test", 5), &se) {
		t.Fatal("expected SyntaxError")
	}
	if se.Offset != 5 {
		t.Fatalf("offset: got %d want 5", se.Offset)
	}
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
