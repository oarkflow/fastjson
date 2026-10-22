//go:build !goexperiment.jsonv2

package fastjson

// stdIsV2 reports that the linked encoding/json is the encoding/json/v2-backed
// implementation, whose error messages and error-path behavior differ from
// the classic implementation fastjson follows.
const stdIsV2 = false
