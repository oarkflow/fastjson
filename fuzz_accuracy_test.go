package fastjson

import (
	"bytes"
	stdjson "encoding/json"
	"testing"
)

func FuzzValidMatchesStdlib(f *testing.F) {
	for _, seed := range [][]byte{[]byte(`null`), []byte(`true`), []byte(`123`), []byte(`"x"`), []byte(`{"a":1}`), []byte(`[1,2,3]`), []byte(`{"bad":`)} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		if Valid(data) != stdjson.Valid(data) {
			t.Fatalf("Valid mismatch for %q", data)
		}
	})
}
func FuzzCompactMatchesStdlib(f *testing.F) {
	for _, seed := range [][]byte{[]byte(`null`), []byte(`{"a":1,"b":[true,false,null]}`), []byte(`[1,2,3]`)} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		var a, b bytes.Buffer
		se := stdjson.Compact(&a, data)
		fe := Compact(&b, data)
		if (se == nil) != (fe == nil) {
			t.Fatalf("err mismatch fast=%v std=%v", fe, se)
		}
		if se == nil && a.String() != b.String() {
			t.Fatalf("compact mismatch fast=%q std=%q", b.String(), a.String())
		}
	})
}
