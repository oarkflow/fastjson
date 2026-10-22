// Adapted from the Go standard library (encoding/json) test suite.
// Copyright The Go Authors. Use of this source code is governed by the
// BSD-style license in LICENSE-GO.

// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fastjson

import "testing"

func TestTagParsing(t *testing.T) {
	name, opts := parseTag("field,foobar,foo")
	if name != "field" {
		t.Fatalf("name = %q, want field", name)
	}
	for _, tt := range []struct {
		opt  string
		want bool
	}{
		{"foobar", true},
		{"foo", true},
		{"bar", false},
	} {
		if opts.Contains(tt.opt) != tt.want {
			t.Errorf("Contains(%q) = %v, want %v", tt.opt, !tt.want, tt.want)
		}
	}
}
