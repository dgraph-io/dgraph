/*
 * Copyright 2018 Dgraph Labs, Inc. All rights reserved.
 *
 */

package backup

import (
	"net/url"

	"github.com/dgraph-io/dgraph/x"
)

// handler interface is implemented by uri scheme handlers.
//
// Session() will read any supported environment variables and authenticate if needed.
// Copy() copies a local file (from tmpDir) to a new destination, possibly remote.
// Exists() tests if a file exists.
type handler interface {
	Copy(string, string) error
	Session(string, string) error
}

// handlers map uri scheme to a handler
var handlers = make(map[string]handler, 0)

func findHandler(uri string) (handler, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return nil, err
	}
	if u.Scheme == "" {
		u.Scheme = "file"
	}
	h, ok := handlers[u.Scheme]
	if !ok {
		return nil, x.Errorf("invalid scheme %q", u.Scheme)
	}
	if err := h.Session(u.Host, u.Path); err != nil {
		return nil, err
	}
	return h, nil
}
