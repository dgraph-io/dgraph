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
	"net/url"

	"github.com/dgraph-io/dgraph/x"
)

// Load will scan location l for backup files, then load them sequentially.
// The loadFunc loadFn is called for each backup file found.
// Returns nil on success, error otherwise.
func Load(l string, loadFn loadFunc) error {
	uri, err := url.Parse(l)
	if err != nil {
		return err
	}

	h := getHandler(uri.Scheme)
	if h == nil {
		return x.Errorf("Unsupported URI: %v", uri)
	}

	if loadFn == nil {
		return x.Errorf("No load function specified.")
	}

	if err := h.Load(uri, loadFn); err != nil {
		return err
	}

	return nil
}
