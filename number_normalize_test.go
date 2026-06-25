package fastjson

import (
	"bytes"
	"testing"
)

func TestMarshalUseNumberTreeEmitsRawNumbers(t *testing.T) {
	var v any
	dec := NewDecoder(bytes.NewReader([]byte(`{"port":8080,"ratio":0.75,"neg":-9,"exp":1.2e3,"nested":[1,{"x":2}]}`)))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		t.Fatal(err)
	}

	b, err := Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(b, []byte(`"8080"`)) || bytes.Contains(b, []byte(`"0.75"`)) || bytes.Contains(b, []byte(`"1.2e3"`)) {
		t.Fatalf("Number encoded as string: %s", b)
	}
	if !Valid(b) {
		t.Fatalf("round-trip produced invalid JSON: %s", b)
	}

	var v2 any
	if err := Unmarshal(b, &v2); err != nil {
		t.Fatalf("round-trip decode failed for %s: %v", b, err)
	}
}

func TestNormalizeNumbersConvertsTree(t *testing.T) {
	v := map[string]any{
		"port": Number("8080"),
		"ratio": Number("0.75"),
		"nested": []any{Number("1"), map[string]any{"x": Number("2.5")}},
	}
	NormalizeNumbers(v)
	if _, ok := v["port"].(int64); !ok {
		t.Fatalf("port type = %T, want int64", v["port"])
	}
	if _, ok := v["ratio"].(float64); !ok {
		t.Fatalf("ratio type = %T, want float64", v["ratio"])
	}
	nested := v["nested"].([]any)
	if _, ok := nested[0].(int64); !ok {
		t.Fatalf("nested[0] type = %T, want int64", nested[0])
	}
}

func TestNumberMarshalRejectsInvalidLiteral(t *testing.T) {
	if _, err := Marshal(map[string]any{"n": Number("01")}); err == nil {
		t.Fatal("expected invalid Number literal error")
	}
}
