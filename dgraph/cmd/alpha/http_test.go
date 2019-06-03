/*
 * Copyright 2017-2018 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package alpha

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/query"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/dgraph/z"
)

type respError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type res struct {
	Data       json.RawMessage   `json:"data"`
	Extensions *query.Extensions `json:"extensions,omitempty"`
	Errors     []respError       `json:"errors,omitempty"`
}

type params struct {
	Query     string            `json:"query"`
	Variables map[string]string `json:"variables"`
}

func queryWithGz(q, t string, gzReq bool, gzResp bool, timeout string) (
	string, *http.Response, error) {

	url := addr + "/query"
	if timeout != "" {
		url = url + fmt.Sprintf("?timeout=%v", timeout)
	}

	var buf *bytes.Buffer
	if gzReq {
		var b bytes.Buffer
		gz := gzip.NewWriter(&b)
		gz.Write([]byte(q))
		gz.Close()
		buf = &b
	} else {
		buf = bytes.NewBufferString(q)
	}

	req, err := http.NewRequest("POST", url, buf)
	if err != nil {
		return "", nil, err
	}
	req.Header.Add("Content-Type", t)

	if gzReq {
		req.Header.Set("Content-Encoding", "gzip")
	}

	if gzResp {
		req.Header.Set("Accept-Encoding", "gzip")
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", nil, err
	}
	if status := resp.StatusCode; status != http.StatusOK {
		return "", nil, fmt.Errorf("Unexpected status code: %v", status)
	}

	defer resp.Body.Close()
	rd := resp.Body
	if gzResp {
		if strings.Contains(resp.Header.Get("Content-Encoding"), "gzip") {
			rd, err = gzip.NewReader(rd)
			if err != nil {
				return "", nil, err
			}
			defer rd.Close()
		} else {
			return "", resp, fmt.Errorf("Response not compressed")
		}
	}
	body, err := ioutil.ReadAll(rd)
	if err != nil {
		return "", nil, err
	}

	var r res
	x.Check(json.Unmarshal(body, &r))

	// Check for errors
	if len(r.Errors) != 0 {
		return "", nil, errors.New(r.Errors[0].Message)
	}

	// Remove the extensions.
	r2 := res{
		Data: r.Data,
	}
	output, err := json.Marshal(r2)

	return string(output), resp, err
}

func queryWithTs(q, t string, ts uint64) (string, uint64, error) {
	url := addr + "/query"
	if ts != 0 {
		url += "?startTs=" + strconv.FormatUint(ts, 10)
	}

	_, body, err := runWithRetries("POST", t, url, q)
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

func mutationWithTs(m, t string, isJson bool, commitNow bool, ignoreIndexConflict bool,
	ts uint64) ([]string, []string, uint64, error) {

	queryParams := make([]string, 0)
	if ts != 0 {
		queryParams = append(queryParams, "startTs="+strconv.FormatUint(ts, 10))
	}
	var keys []string
	var preds []string
	if commitNow {
		queryParams = append(queryParams, "commitNow=true")
	}

	url := addr + "/mutate?" + strings.Join(queryParams, "&")
	_, body, err := runWithRetries("POST", t, url, m)
	if err != nil {
		return keys, preds, 0, err
	}

	var r res
	x.Check(json.Unmarshal(body, &r))
	startTs := r.Extensions.Txn.StartTs

	return r.Extensions.Txn.Keys, r.Extensions.Txn.Preds, startTs, nil
}

func createRequest(method, contentType, url string, body string) (*http.Request, error) {
	req, err := http.NewRequest(method, url, bytes.NewBufferString(body))
	if err != nil {
		return nil, err
	}

	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	return req, nil
}

func runWithRetries(method, contentType, url string, body string) (
	*x.QueryResWithData, []byte, error) {

	req, err := createRequest(method, contentType, url, body)
	if err != nil {
		return nil, nil, err
	}

	qr, respBody, err := runRequest(req)
	if err != nil && strings.Contains(err.Error(), "Token is expired") {
		grootAccessJwt, grootRefreshJwt, err = z.HttpLogin(&z.LoginParams{
			Endpoint:   addr + "/login",
			RefreshJwt: grootRefreshJwt,
		})

		// create a new request since the previous request would have been closed upon the err
		retryReq, err := createRequest(method, contentType, url, body)
		if err != nil {
			return nil, nil, err
		}

		return runRequest(retryReq)
	}
	return qr, respBody, err
}

// attach the grootAccessJWT to the request and sends the http request
func runRequest(req *http.Request) (*x.QueryResWithData, []byte, error) {
	client := &http.Client{}
	req.Header.Set("X-Dgraph-AccessToken", grootAccessJwt)
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
		return nil, nil, fmt.Errorf("unable to read from body: %v", err)
	}

	qr := new(x.QueryResWithData)
	json.Unmarshal(body, qr) // Don't check error.
	if len(qr.Errors) > 0 {
		return nil, nil, errors.New(qr.Errors[0].Message)
	}
	return qr, body, nil
}

func commitWithTs(keys, preds []string, ts uint64) error {
	url := addr + "/commit"
	if ts != 0 {
		url += "?startTs=" + strconv.FormatUint(ts, 10)
	}

	m := make(map[string]interface{})
	m["keys"] = keys
	m["preds"] = preds
	b, err := json.Marshal(m)
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

func commitWithTsKeysOnly(keys []string, ts uint64) error {
	url := addr + "/commit"
	if ts != 0 {
		url += "?startTs=" + strconv.FormatUint(ts, 10)
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
	_, ts, err := queryWithTs(q1, "application/graphqlpm", 0)
	require.NoError(t, err)

	m1 := `
    {
	  set {
		_:alice <name> "Bob" .
		_:alice <balance> "110" .
		_:bob <balance> "60" .
	  }
	}
	`

	keys, preds, mts, err := mutationWithTs(m1, "application/rdf", false, false, true, ts)
	require.NoError(t, err)
	require.Equal(t, mts, ts)
	require.Equal(t, 3, len(keys))
	require.Equal(t, 2, len(preds))
	var parsedPreds []string
	for _, pred := range preds {
		parsedPreds = append(parsedPreds, strings.Join(strings.Split(pred, "-")[1:], "-"))
	}
	sort.Strings(parsedPreds)
	require.Equal(t, "balance", parsedPreds[0])
	require.Equal(t, "name", parsedPreds[1])

	data, _, err := queryWithTs(q1, "application/graphqlpm", 0)
	require.NoError(t, err)
	require.Equal(t, `{"data":{"balances":[]}}`, data)

	// Query with same timestamp.
	data, _, err = queryWithTs(q1, "application/graphqlpm", ts)
	require.NoError(t, err)
	require.Equal(t, `{"data":{"balances":[{"name":"Bob","balance":"110"}]}}`, data)

	// Commit and query.
	require.NoError(t, commitWithTs(keys, preds, ts))
	data, _, err = queryWithTs(q1, "application/graphqlpm", 0)
	require.NoError(t, err)
	require.Equal(t, `{"data":{"balances":[{"name":"Bob","balance":"110"}]}}`, data)
}

func TestTransactionBasicNoPreds(t *testing.T) {
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
	_, ts, err := queryWithTs(q1, "application/graphqlpm", 0)
	require.NoError(t, err)

	m1 := `
    {
	  set {
		_:alice <name> "Bob" .
		_:alice <balance> "110" .
		_:bob <balance> "60" .
	  }
	}
	`

	keys, _, mts, err := mutationWithTs(m1, "application/rdf", false, false, true, ts)
	require.NoError(t, err)
	require.Equal(t, mts, ts)
	require.Equal(t, 3, len(keys))

	data, _, err := queryWithTs(q1, "application/graphqlpm", 0)
	require.NoError(t, err)
	require.Equal(t, `{"data":{"balances":[]}}`, data)

	// Query with same timestamp.
	data, _, err = queryWithTs(q1, "application/graphqlpm", ts)
	require.NoError(t, err)
	require.Equal(t, `{"data":{"balances":[{"name":"Bob","balance":"110"}]}}`, data)

	// Commit and query.
	require.NoError(t, commitWithTs(keys, nil, ts))
	data, _, err = queryWithTs(q1, "application/graphqlpm", 0)
	require.NoError(t, err)
	require.Equal(t, `{"data":{"balances":[{"name":"Bob","balance":"110"}]}}`, data)
}

func TestTransactionBasicOldCommitFormat(t *testing.T) {
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
	_, ts, err := queryWithTs(q1, "application/graphqlpm", 0)
	require.NoError(t, err)

	m1 := `
    {
	  set {
		_:alice <name> "Bob" .
		_:alice <balance> "110" .
		_:bob <balance> "60" .
	  }
	}
	`

	keys, _, mts, err := mutationWithTs(m1, "application/rdf", false, false, true, ts)
	require.NoError(t, err)
	require.Equal(t, mts, ts)
	require.Equal(t, 3, len(keys))

	data, _, err := queryWithTs(q1, "application/graphqlpm", 0)
	require.NoError(t, err)
	require.Equal(t, `{"data":{"balances":[]}}`, data)

	// Query with same timestamp.
	data, _, err = queryWithTs(q1, "application/graphqlpm", ts)
	require.NoError(t, err)
	require.Equal(t, `{"data":{"balances":[{"name":"Bob","balance":"110"}]}}`, data)

	// One more time, with json body this time.
	d1, err := json.Marshal(params{Query: q1})
	require.NoError(t, err)
	data, _, err = queryWithTs(string(d1), "application/json", ts)
	require.NoError(t, err)
	require.Equal(t, `{"data":{"balances":[{"name":"Bob","balance":"110"}]}}`, data)

	// Commit (using a list of keys instead of a map) and query.
	require.NoError(t, commitWithTsKeysOnly(keys, ts))
	data, _, err = queryWithTs(q1, "application/graphqlpm", 0)
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

func TestHttpCompressionSupport(t *testing.T) {
	require.NoError(t, dropAll())
	require.NoError(t, alterSchema(`name: string @index(term) .`))

	q1 := `
	{
	  names(func: has(name), orderasc: name) {
	    name
	  }
	}
	`

	q2 := `
	query all($name: string) {
	  names(func: eq(name, $name)) {
	    name
	  }
	}
	`

	m1 := `
	{
	  set {
		_:a <name> "Alice" .
		_:b <name> "Bob" .
		_:c <name> "Charlie" .
		_:d <name> "David" .
		_:e <name> "Emily" .
		_:f <name> "Frank" .
		_:g <name> "Gloria" .
		_:h <name> "Hannah" .
		_:i <name> "Ian" .
		_:j <name> "Judy" .
		_:k <name> "Kevin" .
		_:l <name> "Linda" .
		_:m <name> "Michael" .
	  }
	}
	`

	r1 := `{"data":{"names":[{"name":"Alice"},{"name":"Bob"},{"name":"Charlie"},{"name":"David"},` +
		`{"name":"Emily"},{"name":"Frank"},{"name":"Gloria"},{"name":"Hannah"},{"name":"Ian"},` +
		`{"name":"Judy"},{"name":"Kevin"},{"name":"Linda"},{"name":"Michael"}]}}`
	err := runMutation(m1)
	require.NoError(t, err)

	data, resp, err := queryWithGz(q1, "application/graphqlpm", false, false, "")
	require.NoError(t, err)
	require.Equal(t, r1, data)
	require.Empty(t, resp.Header.Get("Content-Encoding"))

	data, resp, err = queryWithGz(q1, "application/graphqlpm", false, true, "")
	require.NoError(t, err)
	require.Equal(t, r1, data)
	require.Equal(t, "gzip", resp.Header.Get("Content-Encoding"))

	data, resp, err = queryWithGz(q1, "application/graphqlpm", true, false, "")
	require.NoError(t, err)
	require.Equal(t, r1, data)
	require.Empty(t, resp.Header.Get("Content-Encoding"))

	data, resp, err = queryWithGz(q1, "application/graphqlpm", true, true, "")
	require.NoError(t, err)
	require.Equal(t, r1, data)
	require.Equal(t, "gzip", resp.Header.Get("Content-Encoding"))

	// query with timeout
	data, resp, err = queryWithGz(q1, "application/graphqlpm", false, false, "1ms")
	require.EqualError(t, err, ": context deadline exceeded")
	require.Equal(t, "", data)

	data, resp, err = queryWithGz(q1, "application/graphqlpm", false, false, "1s")
	require.NoError(t, err)
	require.Equal(t, r1, data)
	require.Empty(t, resp.Header.Get("Content-Encoding"))

	d1, err := json.Marshal(params{Query: q1})
	require.NoError(t, err)
	data, resp, err = queryWithGz(string(d1), "application/json", false, false, "1s")
	require.NoError(t, err)
	require.Equal(t, r1, data)
	require.Empty(t, resp.Header.Get("Content-Encoding"))

	d2, err := json.Marshal(params{
		Query: q2,
		Variables: map[string]string{
			"$name": "Alice",
		},
	})
	require.NoError(t, err)
	data, resp, err = queryWithGz(string(d2), "application/json", false, false, "1s")
	require.NoError(t, err)
	fmt.Println(data)
	require.Equal(t, `{"data":{"names":[{"name":"Alice"}]}}`, data)
	require.Empty(t, resp.Header.Get("Content-Encoding"))
}
