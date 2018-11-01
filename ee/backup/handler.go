/*
 * Copyright 2018 Dgraph Labs, Inc. All rights reserved.
 *
 */

package backup

import (
	"io"
	"sync"

	"github.com/dgraph-io/dgraph/x"
)

// handler interface is implemented by URI scheme handlers.
type handler interface {
	// Handlers know how to Write and Close to their target.
	io.WriteCloser
	// Session receives the host and path of the target. It should get all its configuration
	// from the environment.
	Open(*session) error
}

// handlers map URI scheme to a handler.
// List of possible handlers:
//   file - local file or NFS mounted (default if no scheme detected)
//   http - multipart HTTP upload
//     gs - Google Cloud Storage
//     s3 - Amazon S3
//     as - Azure Storage
var handlers struct {
	sync.Mutex
	m map[string]handler
}

// getHandler takes an URI scheme and finds a handler for it.
// Returns the handler on success, error otherwise.
func getHandler(scheme string) (handler, error) {
	handlers.Lock()
	defer handlers.Unlock()
	// target might be just a dir like '/tmp/backup', then default to local file handler.
	if scheme == "" {
		scheme = "file"
	}
	if h, ok := handlers.m[scheme]; ok {
		return h, nil
	}
	return nil, x.Errorf("Unsupported URI scheme %q", scheme)
}

// addHandler registers a new scheme handler. If the handler is already registered
// we just ignore the request.
func addHandler(scheme string, h handler) {
	handlers.Lock()
	defer handlers.Unlock()
	if handlers.m == nil {
		handlers.m = make(map[string]handler)
	}
	if _, ok := handlers.m[scheme]; !ok {
		handlers.m[scheme] = h
	}
}
