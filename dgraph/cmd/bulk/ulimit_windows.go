// +build windows

/*
 * Copyright 2017-2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

package bulk

import "github.com/pkg/errors"

func queryMaxOpenFiles() (int, error) {
	return 0, errors.New("cannot detect max open files on this platform")
}
