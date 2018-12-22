// +build !oss

/*
 * Copyright 2018 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Dgraph Community License (the "License"); you
 * may not use this file except in compliance with the License. You
 * may obtain a copy of the License at
 *
 *     https://github.com/dgraph-io/dgraph/blob/master/licenses/DCL.txt
 */

package backup

import (
	"io"
	"net/url"

	"github.com/dgraph-io/dgraph/x"
)

// handler interface is implemented by URI scheme handlers.
type handler interface {
	// Handlers know how to Write and Close their location.
	io.WriteCloser
	// Create prepares the location for write operations.
	Create(*url.URL, *Request) error
	// Load will scan location URI for backup files, then load them with loadFunc.
	Load(*url.URL) (io.Reader, error)
}

// getHandler returns a handler for the URI scheme.
// Returns new handler on success, nil otherwise.
func getHandler(scheme string) handler {
	switch scheme {
	case "file", "":
		return &fileHandler{}
	case "s3":
		return &s3Handler{}
	}
	return nil
}

// Load will scan location l for backup files, then load them sequentially through reader.
// Returns nil on success, error otherwise.
func Load(l string) (io.Reader, error) {
	uri, err := url.Parse(l)
	if err != nil {
		return nil, err
	}

	h := getHandler(uri.Scheme)
	if h == nil {
		return nil, x.Errorf("Unsupported URI: %v", uri)
	}

	return h.Load(uri)
}
