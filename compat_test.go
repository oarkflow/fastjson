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
	"time"
)

// compatCorpus is a set of JSON documents exercising every syntax feature.
var compatCorpus = []string{
	`null`, `true`, `false`, `0`, `-0`, `1`, `-1`, `1.5`, `-1.5e10`, `1E-7`, `1e400`, `-1e400`,
	`123456789012345678901234567890`, `9007199254740993`, `0.1`, `2.2250738585072014e-308`, `5e-324`,
	`""`, `"hello"`, `"\u00e9\ud83d\ude00"`, `"\ud800"`, `"\ud800\u0041"`, `"\"\\\/\b\f\n\r\t"`,
	"\"\xff\xfe\"", "\"a\xc3\"", `"\u2028\u2029<>&"`, `"` + strings.Repeat("abcdefgh", 40) + `"`,
	`[]`, `[1,2,3]`, `[ 1 , [ 2 , [ 3 ] ] ]`, `{}`, `{"a":1}`, `{"a":{"b":{"c":[1,{"d":null}]}}}`,
	`{"a":1,"a":2}`, `{"A":1,"a":2}`, `{"\u0041":1}`, ` {"x" : [true, false, null] } `,
	`{"id":42,"name":"Alice","tags":["go","json"],"score":99.75,"ok":true,"nil":null}`,
	`[{"id":1},{"id":2,"extra":[1,2,{"z":"y"}]}]`,
	// invalid documents
	``, ` `, `{`, `[`, `[1,]`, `{"a":}`, `{"a" 1}`, `{a:1}`, `[1 2]`, `01`, `1.`, `-`, `.5`, `+1`,
	`tru`, `nul`, `"abc`, `"\x"`, `"\u12"`, "\"\x01\"", `[1]x`, `{"a":1}}`, `1 2`, `NaN`,
}

func TestCompatValid(t *testing.T) {
	for _, in := range compatCorpus {
		if got, want := Valid([]byte(in)), stdjson.Valid([]byte(in)); got != want {
			t.Errorf("Valid(%q) = %v, want %v", in, got, want)
		}
	}
}

// checkUnmarshal decodes in with both packages into fresh values of type T
// and compares the results.
func checkUnmarshal[T any](t *testing.T, in []byte) {
	t.Helper()
	var got, want T
	gotErr := Unmarshal(in, &got)
	wantErr := stdjson.Unmarshal(in, &want)
	if (gotErr == nil) != (wantErr == nil) {
		t.Fatalf("Unmarshal(%q) into %T: err = %v, want %v", in, got, gotErr, wantErr)
	}
	var gse, wse *stdjson.SyntaxError
	if errors.As(wantErr, &wse) {
		if !errors.As(gotErr, &gse) {
			t.Fatalf("Unmarshal(%q): got %T error, want *json.SyntaxError", in, gotErr)
		}
	}
	if gotErr == nil && !reflect.DeepEqual(got, want) {
		t.Fatalf("Unmarshal(%q) into %T:\n got  %#v\n want %#v", in, got, got, want)
	}
}

type compatInner struct {
	B string  `json:"b"`
	C []int   `json:"c,omitempty"`
	D float64 `json:"d,string"`
}

type compatStruct struct {
	ID      int               `json:"id"`
	Name    string            `json:"name"`
	Tags    []string          `json:"tags"`
	Score   float64           `json:"score"`
	OK      bool              `json:"ok"`
	Nil     *int              `json:"nil"`
	Any     any               `json:"any"`
	Map     map[string]int    `json:"map"`
	Inner   compatInner       `json:"inner"`
	PInner  *compatInner      `json:"pinner"`
	Num     Number            `json:"num"`
	Raw     RawMessage        `json:"raw"`
	Bytes   []byte            `json:"bytes"`
	Arr     [2]int            `json:"arr"`
	U8      uint8             `json:"u8"`
	I16     int16             `json:"i16"`
	F32     float32           `json:"f32"`
	Time    time.Time         `json:"time"`
	MapKeys map[int64]string  `json:"mapkeys"`
	Nested  map[string][]bool `json:"nested"`
	A       int
	Ignored string `json:"-"`
}

func TestCompatUnmarshalCorpus(t *testing.T) {
	docs := append([]string{}, compatCorpus...)
	docs = append(docs,
		`{"id":7,"name":"x","tags":["a"],"score":1.25,"ok":true,"nil":null,"any":{"k":[1,"2"]},"map":{"a":1},`+
			`"inner":{"b":"q","c":[1,2],"d":"2.5"},"pinner":{"b":"p"},"num":12.5e3,"raw":{"keep":[1, 2]},`+
			`"bytes":"aGVsbG8=","arr":[1,2,3],"u8":255,"i16":-300,"f32":3.25,"time":"2024-01-02T03:04:05Z",`+
			`"mapkeys":{"-5":"neg","10":"ten"},"nested":{"x":[true,false]},"a":9,"Ignored":"no"}`,
		`{"ID":1,"NAME":"case-insensitive","Inner":{"B":"y"}}`,
		`{"num":"42"}`, `{"u8":256}`, `{"i16":1.5}`, `{"name":5}`, `{"tags":{}}`, `{"arr":null}`,
	)
	for _, in := range docs {
		b := []byte(in)
		checkUnmarshal[any](t, b)
		checkUnmarshal[map[string]any](t, b)
		checkUnmarshal[[]any](t, b)
		checkUnmarshal[string](t, b)
		checkUnmarshal[float64](t, b)
		checkUnmarshal[int](t, b)
		checkUnmarshal[bool](t, b)
		checkUnmarshal[Number](t, b)
		checkUnmarshal[RawMessage](t, b)
		checkUnmarshal[*compatStruct](t, b)
		checkUnmarshal[[]compatStruct](t, b)
		checkUnmarshal[map[string]compatInner](t, b)
		checkUnmarshal[map[string]Number](t, b)
	}
}

func TestCompatMarshal(t *testing.T) {
	n := 3
	values := []any{
		nil, true, 0, -1, int8(-128), uint64(math.MaxUint64), 1.0, -0.0, 1e21, 1e-7, 123456.789, float32(3.4e38),
		"", "hello <world> & \u2028", "\xff\xfe", "\x00\x1f\x7f", Number("12.5"), Number(""),
		[]byte(nil), []byte("bytes"), []int(nil), []string{}, [3]bool{true}, map[string]int(nil),
		map[string]any{"b": 1, "a": []any{nil, "x", 2.5, map[string]any{}}},
		map[int]string{10: "a", -1: "b", 2: "c"}, map[uint8]bool{1: true},
		&n, (*int)(nil), RawMessage(`{ "a" : [ 1 , 2 ] }`), RawMessage(nil),
		compatStruct{ID: 1, Name: "n", Tags: []string{"<t>"}, Any: map[string]any{"k": 1}, Num: "3", Time: time.Unix(0, 0).UTC(), Raw: RawMessage(`null`)},
		&compatStruct{PInner: &compatInner{B: "x", C: []int{1}, D: 0.5}, Bytes: []byte{0, 1, 2}},
		struct {
			A int `json:",omitempty"`
			B int `json:",omitzero"`
			C *int
			D any       `json:"d,omitempty"`
			E time.Time `json:",omitzero"`
			F int64     `json:",string"`
			G bool      `json:",string"`
			H string    `json:",string"`
		}{F: 5, G: true, H: "q"},
	}
	for _, v := range values {
		got, gotErr := Marshal(v)
		want, wantErr := stdjson.Marshal(v)
		if (gotErr == nil) != (wantErr == nil) || !bytes.Equal(got, want) {
			t.Errorf("Marshal(%#v):\n got  %s, %v\n want %s, %v", v, got, gotErr, want, wantErr)
		}
		got, gotErr = MarshalIndent(v, ">", "\t")
		want, wantErr = stdjson.MarshalIndent(v, ">", "\t")
		if (gotErr == nil) != (wantErr == nil) || !bytes.Equal(got, want) {
			t.Errorf("MarshalIndent(%#v):\n got  %s, %v\n want %s, %v", v, got, gotErr, want, wantErr)
		}
		var gb, wb bytes.Buffer
		ge, we := NewEncoder(&gb), stdjson.NewEncoder(&wb)
		ge.SetEscapeHTML(false)
		we.SetEscapeHTML(false)
		gotErr, wantErr = ge.Encode(v), we.Encode(v)
		if (gotErr == nil) != (wantErr == nil) || gb.String() != wb.String() {
			t.Errorf("Encoder(%#v):\n got  %s, %v\n want %s, %v", v, gb.Bytes(), gotErr, wb.Bytes(), wantErr)
		}
	}
	for _, v := range []any{math.NaN(), math.Inf(1), make(chan int), func() {}, complex(1, 2), map[[2]int]int{{1, 2}: 3}} {
		if _, err := Marshal(v); err == nil {
			t.Errorf("Marshal(%T) succeeded, want error", v)
		}
	}
}

// TestNumberInterop checks that fastjson.Number is json.Number and behaves
// like it everywhere a Number can appear.
func TestNumberInterop(t *testing.T) {
	var n stdjson.Number = Number("1.5") // same type, no conversion needed
	if f, err := n.Float64(); err != nil || f != 1.5 {
		t.Fatalf("Float64 = %v, %v", f, err)
	}

	type S struct {
		N  Number            `json:"n"`
		P  *Number           `json:"p"`
		Q  Number            `json:"q,string"`
		M  map[string]Number `json:"m"`
		L  []Number          `json:"l"`
		A  any               `json:"a"`
		NS stdjson.Number    `json:"ns"`
	}
	in := []byte(`{"n":12345678901234567890,"p":-0.5e-3,"q":"7","m":{"x":1e2},"l":[1,"2"],"a":3,"ns":"4"}`)
	var got, want S
	if err := Unmarshal(in, &got); err != nil {
		t.Fatal(err)
	}
	if err := stdjson.Unmarshal(in, &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
	if got.N != "12345678901234567890" || got.L[1] != "2" {
		t.Fatalf("Number literals not preserved: %+v", got)
	}

	// UseNumber yields json.Number values in generic trees, and they
	// re-encode as the original literals.
	dec := NewDecoder(strings.NewReader(`{"big":12345678901234567890,"f":1.10}`))
	dec.UseNumber()
	var tree map[string]any
	if err := dec.Decode(&tree); err != nil {
		t.Fatal(err)
	}
	if _, ok := tree["big"].(stdjson.Number); !ok {
		t.Fatalf("UseNumber produced %T, want json.Number", tree["big"])
	}
	b, err := Marshal(tree)
	if err != nil || string(b) != `{"big":12345678901234567890,"f":1.10}` {
		t.Fatalf("Marshal(UseNumber tree) = %s, %v", b, err)
	}
	// A tree produced by encoding/json works with fastjson and vice versa.
	stdDec := stdjson.NewDecoder(strings.NewReader(`[1.0, 2]`))
	stdDec.UseNumber()
	var stdTree any
	if err := stdDec.Decode(&stdTree); err != nil {
		t.Fatal(err)
	}
	if b, err := Marshal(stdTree); err != nil || string(b) != `[1.0,2]` {
		t.Fatalf("Marshal(std tree) = %s, %v", b, err)
	}

	// Invalid Number values are rejected the same way encoding/json does.
	if _, err := Marshal(Number("01")); err == nil {
		t.Fatal("Marshal accepted invalid Number")
	}
	if err := Unmarshal([]byte(`"abc"`), new(Number)); err == nil {
		t.Fatal("Unmarshal accepted invalid quoted Number")
	}
	if b, _ := Marshal(struct{ N Number }{}); string(b) != `{"N":0}` {
		t.Fatalf("zero Number encodes as %s", b)
	}
}

type ptrMarshaler struct{ V int }

func (p *ptrMarshaler) MarshalJSON() ([]byte, error) { return []byte(`"ptr"`), nil }

type valMarshaler struct{ V int }

func (v valMarshaler) MarshalJSON() ([]byte, error) { return []byte(` { "v" : 1 } `), nil }

type textKey struct{ A, B string }

func (k textKey) MarshalText() ([]byte, error) { return []byte(k.A + "-" + k.B), nil }
func (k *textKey) UnmarshalText(b []byte) error {
	k.A, k.B, _ = strings.Cut(string(b), "-")
	return nil
}

func TestCompatMethods(t *testing.T) {
	type S struct {
		P  ptrMarshaler
		PP *ptrMarshaler
		V  valMarshaler
		VP *valMarshaler
		M  map[textKey]int
	}
	s := S{PP: &ptrMarshaler{}, VP: &valMarshaler{}, M: map[textKey]int{{"a", "b"}: 1, {"c", "d"}: 2}}
	for _, v := range []any{s, &s, []S{s}, map[string]S{"k": s}, []any{s, &s}} {
		got, err := Marshal(v)
		want, _ := stdjson.Marshal(v)
		if err != nil || !bytes.Equal(got, want) {
			t.Errorf("Marshal(%T):\n got  %s %v\n want %s", v, got, err, want)
		}
	}
	checkUnmarshal[map[textKey]int](t, []byte(`{"x-y":1,"p-q":2}`))
	checkUnmarshal[S](t, []byte(`{"M":{"a-b":3}}`))
}

// TestCompatErrorTypes checks that errors are the encoding/json types.
func TestCompatErrorTypes(t *testing.T) {
	var se *stdjson.SyntaxError
	if err := Unmarshal([]byte(`{"a":`), new(any)); !errors.As(err, &se) {
		t.Fatalf("got %T, want *json.SyntaxError", err)
	}
	var ute *stdjson.UnmarshalTypeError
	if err := Unmarshal([]byte(`{"a":"x"}`), new(struct{ A int })); !errors.As(err, &ute) || ute.Field != "A" {
		t.Fatalf("got %#v, want *json.UnmarshalTypeError for field A", err)
	}
	var iue *stdjson.InvalidUnmarshalError
	if err := Unmarshal([]byte(`1`), nil); !errors.As(err, &iue) {
		t.Fatalf("got %T, want *json.InvalidUnmarshalError", err)
	}
	var uve *stdjson.UnsupportedValueError
	if _, err := Marshal(math.NaN()); !errors.As(err, &uve) {
		t.Fatalf("got %T, want *json.UnsupportedValueError", err)
	}
	var ute2 *stdjson.UnsupportedTypeError
	if _, err := Marshal(make(chan int)); !errors.As(err, &ute2) {
		t.Fatalf("got %T, want *json.UnsupportedTypeError", err)
	}
	var me *stdjson.MarshalerError
	if _, err := Marshal(badMarshaler{}); !errors.As(err, &me) {
		t.Fatalf("got %T, want *json.MarshalerError", err)
	}
	// Syntax errors carry the same message and offset as encoding/json.
	for _, in := range compatCorpus {
		err := Unmarshal([]byte(in), new(any))
		stdErr := stdjson.Unmarshal([]byte(in), new(any))
		var a, b *stdjson.SyntaxError
		if errors.As(stdErr, &b) {
			if !errors.As(err, &a) || (!stdIsV2 && (a.Error() != b.Error() || a.Offset != b.Offset)) {
				t.Errorf("Unmarshal(%q) = %v (offset %v), want %v (offset %d)", in, err, offsetOf(a), stdErr, b.Offset)
			}
		}
	}
}

func isTypeError(err error) bool {
	var ute *stdjson.UnmarshalTypeError
	return errors.As(err, &ute)
}

// sameDecodeResult compares results of decoding one value with fastjson and
// encoding/json. Values are only compared on success: after a type error
// the v2-backed encoding/json may have stored a partial value.
func sameDecodeResult(got, want any, gotErr, wantErr error) bool {
	if (gotErr == nil) != (wantErr == nil) || isTypeError(gotErr) != isTypeError(wantErr) {
		return false
	}
	return gotErr != nil || reflect.DeepEqual(got, want)
}

func offsetOf(e *stdjson.SyntaxError) any {
	if e == nil {
		return nil
	}
	return e.Offset
}

type badMarshaler struct{}

func (badMarshaler) MarshalJSON() ([]byte, error) { return []byte(`{bad`), nil }

// oneByteReader returns at most one byte per Read, forcing the Decoder to
// resume scanning at every possible position.
type oneByteReader struct{ r io.Reader }

func (o oneByteReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	return o.r.Read(p[:1])
}

func TestCompatDecoderStream(t *testing.T) {
	stream := strings.Join(compatCorpus[:44], "\n") + ` 1 2 "x" [3] {"y":4} 5`
	for _, oneByte := range []bool{false, true} {
		var r io.Reader = strings.NewReader(stream)
		var sr io.Reader = strings.NewReader(stream)
		if oneByte {
			r, sr = oneByteReader{r}, oneByteReader{sr}
		}
		dec, std := NewDecoder(r), stdjson.NewDecoder(sr)
		for i := 0; ; i++ {
			var got, want any
			gotErr, wantErr := dec.Decode(&got), std.Decode(&want)
			if !sameDecodeResult(got, want, gotErr, wantErr) {
				t.Fatalf("value %d: got %#v, %v; want %#v, %v", i, got, gotErr, want, wantErr)
			}
			if gotErr != nil && !isTypeError(gotErr) {
				break
			}
		}
	}
}

func TestCompatTokens(t *testing.T) {
	in := `{"a":[1,"x",{"b":null}],"c":true} [] "end"`
	dec, std := NewDecoder(strings.NewReader(in)), stdjson.NewDecoder(strings.NewReader(in))
	for {
		got, gotErr := dec.Token()
		want, wantErr := std.Token()
		if !reflect.DeepEqual(got, want) || (gotErr == nil) != (wantErr == nil) {
			t.Fatalf("Token = %#v, %v; want %#v, %v", got, gotErr, want, wantErr)
		}
		if dec.More() != std.More() || dec.InputOffset() != std.InputOffset() {
			t.Fatalf("More/InputOffset diverged at %#v", got)
		}
		if gotErr != nil {
			break
		}
	}
}

func FuzzUnmarshalMatchesStdlib(f *testing.F) {
	for _, s := range compatCorpus {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, in []byte) {
		checkUnmarshal[any](t, in)
		checkUnmarshal[*compatStruct](t, in)
		checkUnmarshal[[]compatInner](t, in)
		checkUnmarshal[map[string]Number](t, in)
		// Re-encoding the generic value must match encoding/json too.
		var v any
		if Unmarshal(in, &v) == nil {
			got, err := Marshal(v)
			want, _ := stdjson.Marshal(v)
			if err != nil || !bytes.Equal(got, want) {
				t.Fatalf("Marshal(%#v) = %s, %v; want %s", v, got, err, want)
			}
		}
	})
}

func FuzzIndentMatchesStdlib(f *testing.F) {
	for _, s := range compatCorpus {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, in []byte) {
		if stdIsV2 && len(in) > 0 && isSpace(in[len(in)-1]) && Valid(in) {
			t.Skip("the v2-backed encoding/json re-indents trailing whitespace; classic keeps it verbatim")
		}
		var a, b bytes.Buffer
		ge := Indent(&a, in, "-", "  ")
		we := stdjson.Indent(&b, in, "-", "  ")
		if (ge == nil) != (we == nil) || a.String() != b.String() {
			t.Fatalf("Indent(%q) = %q, %v; want %q, %v", in, a.String(), ge, b.String(), we)
		}
	})
}

func FuzzDecoderMatchesStdlib(f *testing.F) {
	f.Add([]byte(`1 2 [3] {"a":"b"} "x"`))
	f.Add([]byte(`{"a":1}{"b":2}`))
	f.Fuzz(func(t *testing.T, in []byte) {
		dec := NewDecoder(oneByteReader{bytes.NewReader(in)})
		std := stdjson.NewDecoder(bytes.NewReader(in))
		for i := 0; i < 100; i++ {
			var got, want any
			gotErr, wantErr := dec.Decode(&got), std.Decode(&want)
			if !sameDecodeResult(got, want, gotErr, wantErr) {
				t.Fatalf("value %d of %q: got %#v, %v; want %#v, %v", i, in, got, gotErr, want, wantErr)
			}
			if gotErr != nil && !isTypeError(gotErr) {
				return
			}
		}
	})
}

// FuzzScannerMatchesStdScanner checks the fast validator against the port
// of encoding/json's scanner, which produces the error messages.
func FuzzScannerMatchesStdScanner(f *testing.F) {
	for _, s := range compatCorpus {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, in []byte) {
		fast := valid(in)
		std := stdSyntaxError(in, 0, true)
		if _, isGeneric := std.(*SyntaxError); fast != (std == nil || (isGeneric && std.Error() == "invalid JSON")) {
			t.Fatalf("valid(%q) = %v, std scanner error = %v", in, fast, std)
		}
	})
}
