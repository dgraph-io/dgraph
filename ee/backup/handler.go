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
	"fmt"
	"io"
	"net/url"
	"path"

	"github.com/dgraph-io/dgraph/x"
)

const backupFmt = "r%d-g%d.backup"

// handler interface is implemented by URI scheme handlers.
type handler interface {
	// Handlers know how to Write and Close their location.
	io.WriteCloser
	// Create prepares the location for write operations.
	Create(*url.URL, *Request) error
	// Load will scan location URI for backup files, then load them with loadFunc.
	Load(*url.URL, uint64, loadFn) error
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

// getInfo scans the file name and returns its the read timestamp and group ID.
// If the file is not scannable, returns an error.
// Returns read timestamp and group ID, or an error otherwise.
func getInfo(file string) (uint64, int, error) {
	var readTs uint64
	var groupId int
	_, err := fmt.Sscanf(path.Base(file), backupFmt, &readTs, &groupId)
	return readTs, groupId, err
}

// loadFn is a function that will receive the current file being read.
// A reader and the backup readTs are passed as arguments.
type loadFn func(io.Reader, int) error

// Load will scan location l for backup files, then load them sequentially through reader.
// If since is non-zero, handlers might filter files based on readTs.
// Returns nil on success, error otherwise.
func Load(l string, since uint64, fn loadFn) error {
	uri, err := url.Parse(l)
	if err != nil {
		return err
	}

	h := getHandler(uri.Scheme)
	if h == nil {
		return x.Errorf("Unsupported URI: %v", uri)
	}

	return h.Load(uri, since, fn)
}
