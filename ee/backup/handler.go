/*
 * Copyright 2018 Dgraph Labs, Inc. All rights reserved.
 *
 */

package backup

import (
	"net/url"
	"sync"

	"github.com/dgraph-io/dgraph/x"
)

// handler interface is implemented by uri scheme handlers.
//
// Session() will read any supported environment variables and authenticate if needed.
// Copy() copies a local file to a new destination, possibly remote.
// Exists() tests if a file exists at destination.
type handler interface {
	Copy(string, string) error
	Session(string, string) error
}

// handlers map uri scheme to a handler
var handlers struct {
	sync.Mutex
	m map[string]handler
}

// getSchemeHandler takes a URI and picks the parts we need for creating a scheme handler.
// The scheme handler knows how to authenticate itself (using URI params), and how to copy
// itself to the destination target.
// Returns a new file handler on success, error otherwise.
func getSchemeHandler(uri string) (handler, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return nil, err
	}
	// target might be just a dir like '/tmp/backup', then default to local file handler.
	if u.Scheme == "" {
		u.Scheme = "file"
	}
	handlers.Lock()
	defer handlers.Unlock()
	h, ok := handlers.m[u.Scheme]
	if !ok {
		return nil, x.Errorf("invalid scheme %q", u.Scheme)
	}
	if err := h.Session(u.Host, u.Path); err != nil {
		return nil, err
	}
	return h, nil
}

func addSchemeHandler(scheme string, h handler) {
	handlers.Lock()
	defer handlers.Unlock()
	if handlers.m == nil {
		handlers.m = make(map[string]handler)
	}
	if _, ok := handlers.m[scheme]; !ok {
		handlers.m[scheme] = h
	}
}
