// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build go1.7

package http2

import (
	"context"
	"net/http"
)

func reqContext(r *http.Request) context.Context { return r.Context() }

func setResponseUncompressed(res *http.Response) { res.Uncompressed = true }
