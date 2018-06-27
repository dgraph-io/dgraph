/*
 * Copyright 2017-2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"testing"

	"github.com/dgraph-io/dgraph/query"
	"github.com/dgraph-io/dgraph/x"
	"github.com/stretchr/testify/require"
)

type res struct {
	Data       json.RawMessage   `json:"data"`
	Extensions *query.Extensions `json:"extensions,omitempty"`
}

var addr = "http://localhost:8180"

func queryWithTs(q string, ts uint64) (string, uint64, error) {
	url := addr + "/query"
	if ts != 0 {
		url += "/" + strconv.FormatUint(ts, 10)
	}
	req, err := http.NewRequest("POST", url, bytes.NewBufferString(q))
	if err != nil {
		return "", 0, err
	}
	_, body, err := runRequest(req)
	if err != nil {
		return "", 0, err
	}

	var r res
	x.Check(json.Unmarshal(body, &r))
	startTs := r.Extensions.Txn.StartTs

	// Remove the extensions.
	r2 := res{
		Data: r.Data,
	}
	output, err := json.Marshal(r2)

	return string(output), startTs, err
}

func mutationWithTs(m string, isJson bool, commitNow bool, ignoreIndexConflict bool,
	ts uint64) ([]string, uint64, error) {
	url := addr + "/mutate"
	if ts != 0 {
		url += "/" + strconv.FormatUint(ts, 10)
	}
	var keys []string
	req, err := http.NewRequest("POST", url, bytes.NewBufferString(m))
	if err != nil {
		return keys, 0, err
	}

	if isJson {
		req.Header.Set("X-Dgraph-MutationType", "json")
	}
	if commitNow {
		req.Header.Set("X-Dgraph-CommitNow", "true")
	}
	_, body, err := runRequest(req)
	if err != nil {
		return keys, 0, err
	}

	var r res
	x.Check(json.Unmarshal(body, &r))
	startTs := r.Extensions.Txn.StartTs

	return r.Extensions.Txn.Keys, startTs, nil
}

func runRequest(req *http.Request) (*x.QueryResWithData, []byte, error) {
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	if status := resp.StatusCode; status != http.StatusOK {
		return nil, nil, fmt.Errorf("Unexpected status code: %v", status)
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}

	qr := new(x.QueryResWithData)
	json.Unmarshal(body, qr) // Don't check error.
	if len(qr.Errors) > 0 {
		return nil, nil, errors.New(qr.Errors[0].Message)
	}
	return qr, body, nil
}

func commitWithTs(keys []string, ts uint64) error {
	url := addr + "/commit"
	if ts != 0 {
		url += "/" + strconv.FormatUint(ts, 10)
	}

	b, err := json.Marshal(keys)
	if err != nil {
		return err
	}
	req, err := http.NewRequest("POST", url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	_, _, err = runRequest(req)
	return err
}

func TestTransactionBasic(t *testing.T) {
	require.NoError(t, dropAll())
	require.NoError(t, alterSchema(`name: string @index(term) .`))

	q1 := `
	{
	  balances(func: anyofterms(name, "Alice Bob")) {
	    name
	    balance
	  }
	}
	`
	_, ts, err := queryWithTs(q1, 0)
	require.NoError(t, err)

	m1 := `
    {
	  set {
		_:alice <name> "Alice" .
		_:alice <name> "Bob" .
		_:alice <balance> "110" .
		_:bob <balance> "60" .
	  }
	}
	`

	keys, mts, err := mutationWithTs(m1, false, false, true, ts)
	require.NoError(t, err)
	require.Equal(t, mts, ts)
	sort.Strings(keys)
	require.Equal(t, 4, len(keys))

	data, _, err := queryWithTs(q1, 0)
	require.NoError(t, err)
	require.Equal(t, `{"data":{"balances":[]}}`, data)

	// Query with same timestamp.
	data, _, err = queryWithTs(q1, ts)
	require.NoError(t, err)
	require.Equal(t, `{"data":{"balances":[{"name":"Bob","balance":"110"}]}}`, data)

	// Commit and query.
	require.NoError(t, commitWithTs(keys, ts))
	data, _, err = queryWithTs(q1, 0)
	require.NoError(t, err)
	require.Equal(t, `{"data":{"balances":[{"name":"Bob","balance":"110"}]}}`, data)
}

func TestAlterAllFieldsShouldBeSet(t *testing.T) {
	req, err := http.NewRequest("PUT", "/alter", bytes.NewBufferString(
		`{"dropall":true}`, // "dropall" is spelt incorrect - should be "drop_all"
	))
	require.NoError(t, err)
	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(alterHandler)
	handler.ServeHTTP(rr, req)

	require.Equal(t, rr.Code, http.StatusOK)
	var qr x.QueryResWithData
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &qr))
	require.Len(t, qr.Errors, 1)
	require.Equal(t, qr.Errors[0].Code, "Error")
}
