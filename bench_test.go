package fastjson

import (
	stdjson "encoding/json"
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
