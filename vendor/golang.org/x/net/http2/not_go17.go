// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build !go1.7

package http2

import "net/http"

type fakeContext struct{}

func (fakeContext) Done() <-chan struct{} { return nil }
func (fakeContext) Err() error            { panic("should not be called") }

func reqContext(r *http.Request) fakeContext {
	return fakeContext{}
}

func setResponseUncompressed(res *http.Response) {
	// Nothing.
}
