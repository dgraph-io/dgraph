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
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/dgraph-io/dgraph/x"
)

// TODO: connect to zero instead of http.
func runBackup() error {
	if opt.http == "" {
		return x.Errorf("Backup: must specify alpha address http with --http")
	}
	if opt.loc == "" {
		return x.Errorf("Backup: must specify a backup destination with --loc")
	}

	resp, err := http.PostForm(opt.http, url.Values{"destination": {opt.loc}})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var bb bytes.Buffer
	err = json.NewDecoder(resp.Body).Decode(&bb)
	if err != nil {
		return err
	}
	if !strings.Contains(bb.String(), "Success") {
		return x.Errorf("Backup error: %s", bb.String())
	}
	return nil
}
