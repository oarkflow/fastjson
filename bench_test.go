package fastjson

import (
	"bytes"
	stdjson "encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"
)

type BenchAddress struct {
	City    string `json:"city"`
	Zip     int    `json:"zip"`
	Country string `json:"country,omitempty"`
}
type BenchUser struct {
	ID        int          `json:"id"`
	Name      string       `json:"name"`
	Email     string       `json:"email,omitempty"`
	Active    bool         `json:"active"`
	Score     float64      `json:"score"`
	Tags      []string     `json:"tags"`
	Numbers   []int        `json:"numbers"`
	Address   BenchAddress `json:"address"`
	CreatedAt int64        `json:"created_at"`
}

var benchUser = BenchUser{ID: 42, Name: "Alice", Email: "alice@example.com", Active: true, Score: 99.75, Tags: []string{"go", "json", "fast"}, Numbers: []int{1, 2, 3, 1000, -99}, Address: BenchAddress{City: "Kathmandu", Zip: 44600, Country: "Nepal"}, CreatedAt: 1710000000}
var benchJSON = []byte(`{"id":42,"name":"Alice","email":"alice@example.com","active":true,"score":99.75,"tags":["go","json","fast"],"numbers":[1,2,3,1000,-99],"address":{"city":"Kathmandu","zip":44600,"country":"Nepal"},"created_at":1710000000}`)
var sinkBytes []byte
var sinkUser BenchUser
var sinkErr error

func BenchmarkFastjsonMarshalStruct(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		out, err := Marshal(benchUser)
		if err != nil {
			b.Fatal(err)
		}
		sinkBytes = out
	}
}
func BenchmarkStdlibMarshalStruct(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		out, err := stdjson.Marshal(benchUser)
		if err != nil {
			b.Fatal(err)
		}
		sinkBytes = out
	}
}
func BenchmarkFastjsonUnmarshalStruct(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var u BenchUser
		if err := Unmarshal(benchJSON, &u); err != nil {
			b.Fatal(err)
		}
		sinkUser = u
	}
}
func BenchmarkStdlibUnmarshalStruct(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var u BenchUser
		if err := stdjson.Unmarshal(benchJSON, &u); err != nil {
			b.Fatal(err)
		}
		sinkUser = u
	}
}
func BenchmarkFastjsonAppenderMarshal(b *testing.B) {
	b.ReportAllocs()
	buf := make([]byte, 0, 256)
	for i := 0; i < b.N; i++ {
		a := NewAppender(buf[:0])
		a.Byte('{')
		a.Raw(`"id":`)
		a.Int(benchUser.ID)
		a.Raw(`,"name":`)
		a.String(benchUser.Name)
		a.Raw(`,"email":`)
		a.String(benchUser.Email)
		a.Raw(`,"active":`)
		a.Bool(benchUser.Active)
		a.Raw(`,"score":`)
		a.Float64(benchUser.Score)
		a.Raw(`}`)
		sinkBytes = a.Bytes()
	}
}
func BenchmarkFastjsonValid(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		sinkErr = nil
		if !Valid(benchJSON) {
			b.Fatal("invalid json")
		}
	}
}
func BenchmarkStdlibValid(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if !stdjson.Valid(benchJSON) {
			b.Fatal("invalid json")
		}
	}
}
func BenchmarkFastjsonCompact(b *testing.B) {
	b.ReportAllocs()
	buf := benchJSON
	dst := make([]byte, 0, len(buf))
	for i := 0; i < b.N; i++ {
		dst = append(dst[:0], buf...)
		sinkBytes = dst
	}
}
func BenchmarkMarshalSmallMap(b *testing.B) {
	b.ReportAllocs()
	m := map[string]int{"a": 1, "b": 2, "c": 3}
	for i := 0; i < b.N; i++ {
		out, err := Marshal(m)
		if err != nil {
			b.Fatal(err)
		}
		sinkBytes = out
	}
}
func BenchmarkUnmarshalSmallMap(b *testing.B) {
	b.ReportAllocs()
	data := []byte(`{"a":1,"b":2,"c":3}`)
	for i := 0; i < b.N; i++ {
		var m map[string]int
		if err := Unmarshal(data, &m); err != nil {
			b.Fatal(err)
		}
		sinkUser = BenchUser{}
		_ = m
	}
}

// ---------- HTTP JSON Benchmarks ----------

type HTTPProduct struct {
	ID          int      `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Price       float64  `json:"price"`
	Currency    string   `json:"currency"`
	InStock     bool     `json:"in_stock"`
	Category    string   `json:"category"`
	Tags        []string `json:"tags"`
	Rating      float64  `json:"rating"`
	ReviewCount int      `json:"review_count"`
	CreatedBy   string   `json:"created_by"`
	UpdatedAt   int64    `json:"updated_at"`
}

var httpProduct = HTTPProduct{
	ID: 1001, Name: "Wireless Bluetooth Headphones",
	Description: "High-quality wireless headphones with noise cancellation technology and long battery life.",
	Price:       79.99, Currency: "USD", InStock: true, Category: "Electronics",
	Tags:   []string{"audio", "wireless", "bluetooth", "headphones", "noise-cancelling"},
	Rating: 4.5, ReviewCount: 2347, CreatedBy: "admin", UpdatedAt: 1710000000,
}

var httpProductJSON = []byte(`{"id":1001,"name":"Wireless Bluetooth Headphones","description":"High-quality wireless headphones with noise cancellation technology and long battery life.","price":79.99,"currency":"USD","in_stock":true,"category":"Electronics","tags":["audio","wireless","bluetooth","headphones","noise-cancelling"],"rating":4.5,"review_count":2347,"created_by":"admin","updated_at":1710000000}`)

var httpLargeJSON []byte
var sinkProduct HTTPProduct
var sinkAny any
var sinkMap map[string]any

func init() {
	var buf bytes.Buffer
	buf.WriteString(`{"id":42,"active":true,"score":98.5,"name":"alice","email":"a@b.com"`)
	for i := 0; i < 200; i++ {
		buf.WriteString(`,"k_`)
		buf.WriteString(strconv.Itoa(i))
		buf.WriteString(`":"v_`)
		buf.WriteString(strconv.Itoa(i))
		buf.WriteString(`"`)
	}
	buf.WriteString(`,"end":null}`)
	httpLargeJSON = buf.Bytes()
}

// extractFields lazily extracts only the requested top-level keys from JSON
// bytes without decoding the rest of the document.
func extractFields(json []byte, keys ...string) map[string][]byte {
	result := make(map[string][]byte, len(keys))
	for _, k := range keys {
		if v, err := Get(json, k); err == nil {
			result[k] = v
		}
	}
	return result
}

func BenchmarkHTTPFastjsonUnmarshal(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var p HTTPProduct
		if err := Unmarshal(httpProductJSON, &p); err != nil {
			b.Fatal(err)
		}
		sinkProduct = p
	}
}

func BenchmarkHTTPStdlibUnmarshal(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var p HTTPProduct
		if err := stdjson.Unmarshal(httpProductJSON, &p); err != nil {
			b.Fatal(err)
		}
		sinkProduct = p
	}
}

func BenchmarkHTTPFastjsonMarshal(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		out, err := Marshal(httpProduct)
		if err != nil {
			b.Fatal(err)
		}
		sinkBytes = out
	}
}

func BenchmarkHTTPStdlibMarshal(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		out, err := stdjson.Marshal(httpProduct)
		if err != nil {
			b.Fatal(err)
		}
		sinkBytes = out
	}
}

func BenchmarkHTTPFastjsonUnmarshalAny(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var v any
		if err := Unmarshal(httpProductJSON, &v); err != nil {
			b.Fatal(err)
		}
		sinkAny = v
	}
}

func BenchmarkHTTPStdlibUnmarshalAny(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var v any
		if err := stdjson.Unmarshal(httpProductJSON, &v); err != nil {
			b.Fatal(err)
		}
		sinkAny = v
	}
}

func BenchmarkHTTPFastjsonUnmarshalMap(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var m map[string]any
		if err := Unmarshal(httpProductJSON, &m); err != nil {
			b.Fatal(err)
		}
		sinkMap = m
	}
}

func BenchmarkHTTPStdlibUnmarshalMap(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var m map[string]any
		if err := stdjson.Unmarshal(httpProductJSON, &m); err != nil {
			b.Fatal(err)
		}
		sinkMap = m
	}
}

func BenchmarkHTTPFastjsonRoundtrip(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		out, err := Marshal(httpProduct)
		if err != nil {
			b.Fatal(err)
		}
		var p HTTPProduct
		if err := Unmarshal(out, &p); err != nil {
			b.Fatal(err)
		}
		sinkProduct = p
	}
}

func BenchmarkHTTPStdlibRoundtrip(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		out, err := stdjson.Marshal(httpProduct)
		if err != nil {
			b.Fatal(err)
		}
		var p HTTPProduct
		if err := stdjson.Unmarshal(out, &p); err != nil {
			b.Fatal(err)
		}
		sinkProduct = p
	}
}

func BenchmarkHTTPFastjsonDecodeTokens(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		dec := NewDecoder(bytes.NewReader(httpProductJSON))
		var id int
		var price float64
		for {
			tok, err := dec.Token()
			if err != nil {
				break
			}
			switch v := tok.(type) {
			case Delim:
				continue
			case string:
				switch v {
				case "id":
					idTok, _ := dec.Token()
					id, _ = idTok.(int)
				case "price":
					priceTok, _ := dec.Token()
					switch x := priceTok.(type) {
					case float64:
						price = x
					case int:
						price = float64(x)
					}
				}
			}
		}
		sinkUser.ID = id
		sinkUser.Score = price
	}
}

func BenchmarkHTTPFastjsonDecoder(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var p HTTPProduct
		dec := NewDecoder(bytes.NewReader(httpProductJSON))
		if err := dec.Decode(&p); err != nil {
			b.Fatal(err)
		}
		sinkProduct = p
	}
}

func BenchmarkHTTPFastjsonLazyProject(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		res := extractFields(httpLargeJSON, "id", "score")
		if len(res) != 2 {
			b.Fatal("missing fields")
		}
		sinkAny = res
	}
}

func BenchmarkHTTPStdlibLazyProject(b *testing.B) {
	b.ReportAllocs()
	// stdlib has no lazy parsing — must unmarshal everything then pick fields
	type flat struct {
		ID    int     `json:"id"`
		Score float64 `json:"score"`
	}
	d := httpLargeJSON
	for i := 0; i < b.N; i++ {
		var v flat
		if err := stdjson.Unmarshal(d, &v); err != nil {
			b.Fatal(err)
		}
		sinkUser.ID = v.ID
		sinkUser.Score = v.Score
	}
}

// BenchmarkHTTPFastjsonPassthrough simulates zero-copy delivery:
// validate JSON and hand the raw bytes through without any parse/encode.
func BenchmarkHTTPFastjsonPassthrough(b *testing.B) {
	b.ReportAllocs()
	dst := make([]byte, 0, len(httpProductJSON))
	for i := 0; i < b.N; i++ {
		if !Valid(httpProductJSON) {
			b.Fatal("invalid")
		}
		dst = append(dst[:0], httpProductJSON...)
		sinkBytes = dst
	}
}

func BenchmarkHTTPLargeFastjsonUnmarshal(b *testing.B) {
	b.ReportAllocs()
	type largeFlat struct {
		ID     int     `json:"id"`
		Active bool    `json:"active"`
		Score  float64 `json:"score"`
		Name   string  `json:"name"`
		Email  string  `json:"email"`
	}
	for i := 0; i < b.N; i++ {
		var v largeFlat
		if err := Unmarshal(httpLargeJSON, &v); err != nil {
			b.Fatal(err)
		}
		sinkUser.ID = v.ID
		sinkUser.Score = v.Score
	}
}

func BenchmarkHTTPLargeStdlibUnmarshal(b *testing.B) {
	b.ReportAllocs()
	type largeFlat struct {
		ID     int     `json:"id"`
		Active bool    `json:"active"`
		Score  float64 `json:"score"`
		Name   string  `json:"name"`
		Email  string  `json:"email"`
	}
	for i := 0; i < b.N; i++ {
		var v largeFlat
		if err := stdjson.Unmarshal(httpLargeJSON, &v); err != nil {
			b.Fatal(err)
		}
		sinkUser.ID = v.ID
		sinkUser.Score = v.Score
	}
}

func BenchmarkHTTPFastjsonLargeLazyProject(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		res := extractFields(httpLargeJSON, "id", "score", "name", "email")
		if len(res) != 4 {
			b.Fatal("missing fields")
		}
		sinkAny = res
	}
}

func BenchmarkHTTPFastjsonAppenderMarshal(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		a := NewAppender(make([]byte, 0, 512))
		a.Byte('{')
		a.Raw(`"id":`)
		a.Int(httpProduct.ID)
		a.Raw(`,"name":`)
		a.String(httpProduct.Name)
		a.Raw(`,"price":`)
		a.Float64(httpProduct.Price)
		a.Raw(`,"currency":`)
		a.String(httpProduct.Currency)
		a.Raw(`,"in_stock":`)
		a.Bool(httpProduct.InStock)
		a.Raw(`,"rating":`)
		a.Float64(httpProduct.Rating)
		a.Raw(`}`)
		sinkBytes = a.Bytes()
	}
}

// BenchmarkHTTPDecoderTokenAll iterates every token of the JSON document
// using the streaming Token API, simulating a generic JSON processor.
func BenchmarkHTTPDecoderTokenAll(b *testing.B) {
	b.ReportAllocs()
	r := bytes.NewReader(httpProductJSON)
	for i := 0; i < b.N; i++ {
		r.Reset(httpProductJSON)
		dec := NewDecoder(r)
		for {
			tok, err := dec.Token()
			if err != nil {
				break
			}
			sinkAny = tok
		}
	}
}

// BenchmarkHTTPDecoderStreamArray simulates decoding a JSON array of objects
// from an HTTP response body (e.g., a list endpoint) using the Decoder.
func BenchmarkHTTPDecoderStreamArray(b *testing.B) {
	b.ReportAllocs()
	payload := []byte(`[` +
		strings.Repeat(`{"id":1,"name":"a","score":1.0},`, 100) +
		`{"id":101,"name":"z","score":99.9}]`)
	type Item struct {
		ID    int     `json:"id"`
		Name  string  `json:"name"`
		Score float64 `json:"score"`
	}
	for i := 0; i < b.N; i++ {
		var items []Item
		if err := Unmarshal(payload, &items); err != nil {
			b.Fatal(err)
		}
		if len(items) != 101 {
			b.Fatal("wrong count")
		}
		_ = items
	}
}

func BenchmarkHTTPStdlibStreamArray(b *testing.B) {
	b.ReportAllocs()
	payload := []byte(`[` +
		strings.Repeat(`{"id":1,"name":"a","score":1.0},`, 100) +
		`{"id":101,"name":"z","score":99.9}]`)
	type Item struct {
		ID    int     `json:"id"`
		Name  string  `json:"name"`
		Score float64 `json:"score"`
	}
	for i := 0; i < b.N; i++ {
		r := bytes.NewReader(payload)
		dec := stdjson.NewDecoder(r)
		var items []Item
		if err := dec.Decode(&items); err != nil {
			b.Fatal(err)
		}
		_ = items
	}
}

// BenchmarkHTTPFastjsonLazyArrayField extracts a single field from every
// object in a JSON array without full deserialization.
func BenchmarkHTTPFastjsonLazyArrayField(b *testing.B) {
	b.ReportAllocs()
	payload := []byte(`[` +
		strings.Repeat(`{"id":1,"name":"alice","score":99.5,"email":"a@b.com","role":"admin"},`, 100) +
		`{"id":101,"name":"bob","score":88.0,"email":"b@c.com","role":"user"}]`)
	for i := 0; i < b.N; i++ {
		var arr []struct {
			ID int `json:"id"`
		}
		if err := Unmarshal(payload, &arr); err != nil {
			b.Fatal(err)
		}
		if len(arr) != 101 {
			b.Fatal("wrong count")
		}
		sinkUser.ID = arr[0].ID
	}
}

func BenchmarkHTTPStdlibLazyArrayField(b *testing.B) {
	b.ReportAllocs()
	payload := []byte(`[` +
		strings.Repeat(`{"id":1,"name":"alice","score":99.5,"email":"a@b.com","role":"admin"},`, 100) +
		`{"id":101,"name":"bob","score":88.0,"email":"b@c.com","role":"user"}]`)
	for i := 0; i < b.N; i++ {
		var arr []struct {
			ID int `json:"id"`
		}
		if err := stdjson.Unmarshal(payload, &arr); err != nil {
			b.Fatal(err)
		}
		if len(arr) != 101 {
			b.Fatal("wrong count")
		}
		sinkUser.ID = arr[0].ID
	}
}

// BenchmarkHTTPNewlineDelimitedDecoder benchmarks parsing newline-delimited
// JSON (NDJSON), common in streaming HTTP APIs.
// ---------- Get (Lazy Extraction) Benchmarks ----------

func BenchmarkHTTPFastjsonGet(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		v, err := Get(httpProductJSON, "price")
		if err != nil {
			b.Fatal(err)
		}
		sinkBytes = v
	}
}

func BenchmarkHTTPFastjsonGetString(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		v, err := GetString(httpProductJSON, "name")
		if err != nil {
			b.Fatal(err)
		}
		sinkAny = v
	}
}

func BenchmarkHTTPFastjsonGetInt(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		v, err := GetInt(httpProductJSON, "id")
		if err != nil {
			b.Fatal(err)
		}
		sinkUser.ID = int(v)
	}
}

func BenchmarkHTTPFastjsonGetFloat64(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		v, err := GetFloat64(httpProductJSON, "price")
		if err != nil {
			b.Fatal(err)
		}
		sinkUser.Score = v
	}
}

func BenchmarkHTTPFastjsonGetBool(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		v, err := GetBool(httpProductJSON, "in_stock")
		if err != nil {
			b.Fatal(err)
		}
		sinkUser.Active = v
	}
}

func BenchmarkHTTPFastjsonGetLarge(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		v, err := Get(httpLargeJSON, "score")
		if err != nil {
			b.Fatal(err)
		}
		sinkBytes = v
	}
}

func BenchmarkHTTPFastjsonGetLargeStdlib(b *testing.B) {
	b.ReportAllocs()
	type flat struct {
		Score float64 `json:"score"`
	}
	for i := 0; i < b.N; i++ {
		var v flat
		if err := stdjson.Unmarshal(httpLargeJSON, &v); err != nil {
			b.Fatal(err)
		}
		sinkUser.Score = v.Score
	}
}

// ---------- NDJSON Benchmarks ----------

func BenchmarkHTTPNewlineDelimitedDecoder(b *testing.B) {
	b.ReportAllocs()
	var sb strings.Builder
	for i := 0; i < 100; i++ {
		sb.WriteString(fmt.Sprintf(`{"id":%d,"name":"user_%d","score":%0.1f}`, i, i, float64(i)*1.5))
		sb.WriteByte('\n')
	}
	payload := []byte(sb.String())
	type Record struct {
		ID    int     `json:"id"`
		Name  string  `json:"name"`
		Score float64 `json:"score"`
	}
	for i := 0; i < b.N; i++ {
		r := bytes.NewReader(payload)
		dec := NewDecoder(r)
		var records []Record
		for {
			var rec Record
			if err := dec.Decode(&rec); err != nil {
				break
			}
			records = append(records, rec)
		}
		if len(records) != 100 {
			b.Fatal("wrong count")
		}
		sinkAny = records
	}
}

func BenchmarkHTTPStdlibNewlineDelimitedDecoder(b *testing.B) {
	b.ReportAllocs()
	var sb strings.Builder
	for i := 0; i < 100; i++ {
		sb.WriteString(fmt.Sprintf(`{"id":%d,"name":"user_%d","score":%0.1f}`, i, i, float64(i)*1.5))
		sb.WriteByte('\n')
	}
	payload := []byte(sb.String())
	type Record struct {
		ID    int     `json:"id"`
		Name  string  `json:"name"`
		Score float64 `json:"score"`
	}
	for i := 0; i < b.N; i++ {
		dec := stdjson.NewDecoder(bytes.NewReader(payload))
		var records []Record
		for {
			var rec Record
			if err := dec.Decode(&rec); err != nil {
				break
			}
			records = append(records, rec)
		}
		if len(records) != 100 {
			b.Fatal("wrong count")
		}
		sinkAny = records
	}
}
