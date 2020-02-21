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
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/query"
	"github.com/dgraph-io/dgraph/testutil"
	"github.com/dgraph-io/dgraph/x"
)

type res struct {
	Data       json.RawMessage   `json:"data"`
	Extensions *query.Extensions `json:"extensions,omitempty"`
	Errors     []x.GqlError      `json:"errors,omitempty"`
}

type params struct {
	Query     string            `json:"query"`
	Variables map[string]string `json:"variables"`
}

// runGzipWithRetry makes request gzip compressed request. If access token is expired,
// it will try to refresh access token.
func runGzipWithRetry(contentType, url string, buf io.Reader, gzReq, gzResp bool) (
	*http.Response, error) {

	client := &http.Client{}
	numRetries := 2

	var resp *http.Response
	var err error
	for i := 0; i < numRetries; i++ {
		req, err := http.NewRequest("POST", url, buf)
		if err != nil {
			return nil, err
		}
		req.Header.Add("Content-Type", contentType)
		req.Header.Set("X-Dgraph-AccessToken", grootAccessJwt)

		if gzReq {
			req.Header.Set("Content-Encoding", "gzip")
		}

		if gzResp {
			req.Header.Set("Accept-Encoding", "gzip")
		}

		resp, err = client.Do(req)
		if err != nil && strings.Contains(err.Error(), "Token is expired") {
			grootAccessJwt, grootRefreshJwt, err = testutil.HttpLogin(&testutil.LoginParams{
				Endpoint:   addr + "/admin",
				RefreshJwt: grootRefreshJwt,
			})

			if err != nil {
				return nil, err
			}
			continue
		} else if err != nil {
			return nil, err
		}
		break
	}

	return resp, err
}

func queryWithGz(queryText, contentType, debug, timeout string, gzReq, gzResp bool) (
	string, *http.Response, error) {

	params := make([]string, 0, 2)
	if debug != "" {
		params = append(params, "debug="+debug)
	}
	if timeout != "" {
		params = append(params, fmt.Sprintf("timeout=%v", timeout))
	}
	url := addr + "/query?" + strings.Join(params, "&")

	var buf *bytes.Buffer
	if gzReq {
		var b bytes.Buffer
		gz := gzip.NewWriter(&b)
		gz.Write([]byte(queryText))
		gz.Close()
		buf = &b
	} else {
		buf = bytes.NewBufferString(queryText)
	}

	resp, err := runGzipWithRetry(contentType, url, buf, gzReq, gzResp)
	if err != nil {
		return "", nil, err
	}

	defer resp.Body.Close()
	rd := resp.Body
	if err != nil {
		return "", nil, err
	}

	if gzResp {
		if strings.Contains(resp.Header.Get("Content-Encoding"), "gzip") {
			rd, err = gzip.NewReader(rd)
			if err != nil {
				return "", nil, err
			}
			defer rd.Close()
		} else {
			return "", resp, errors.Errorf("Response not compressed")
		}
	}
	body, err := ioutil.ReadAll(rd)
	if err != nil {
		return "", nil, err
	}

	var r res
	if err := json.Unmarshal(body, &r); err != nil {
		return "", nil, err
	}

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

func queryWithTs(queryText, contentType, debug string, ts uint64) (string, uint64, error) {
	params := make([]string, 0, 2)
	if debug != "" {
		params = append(params, "debug="+debug)
	}
	if ts != 0 {
		params = append(params, fmt.Sprintf("startTs=%v", strconv.FormatUint(ts, 10)))
	}
	url := addr + "/query?" + strings.Join(params, "&")

	_, body, err := runWithRetries("POST", contentType, url, queryText)
	if err != nil {
		return "", 0, err
	}

	var r res
	if err := json.Unmarshal(body, &r); err != nil {
		return "", 0, err
	}
	startTs := r.Extensions.Txn.StartTs

	// Remove the extensions.
	r2 := res{
		Data: r.Data,
	}
	output, err := json.Marshal(r2)

	return string(output), startTs, err
}

type mutationResponse struct {
	keys    []string
	preds   []string
	startTs uint64
	data    json.RawMessage
}

func mutationWithTs(m, t string, isJson bool, commitNow bool, ts uint64) (
	mutationResponse, error) {

	params := make([]string, 2)
	if ts != 0 {
		params = append(params, "startTs="+strconv.FormatUint(ts, 10))
	}

	var mr mutationResponse
	if commitNow {
		params = append(params, "commitNow=true")
	}

	url := addr + "/mutate?" + strings.Join(params, "&")
	_, body, err := runWithRetries("POST", t, url, m)
	if err != nil {
		return mr, err
	}

	var r res
	if err := json.Unmarshal(body, &r); err != nil {
		return mr, err
	}

	mr.keys = r.Extensions.Txn.Keys
	mr.preds = r.Extensions.Txn.Preds
	mr.startTs = r.Extensions.Txn.StartTs
	sort.Strings(mr.preds)

	var d map[string]interface{}
	if err := json.Unmarshal(r.Data, &d); err != nil {
		return mr, err
	}
	delete(d, "code")
	delete(d, "message")
	delete(d, "uids")
	mr.data, err = json.Marshal(d)
	if err != nil {
		return mr, err
	}
	return mr, nil
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
	retried := false
	for err != nil {
		if strings.Contains(err.Error(), "Token is expired") {
			grootAccessJwt, grootRefreshJwt, err = testutil.HttpLogin(&testutil.LoginParams{
				Endpoint:   addr + "/admin",
				RefreshJwt: grootRefreshJwt,
			})
			if err != nil {
				return nil, nil, err
			}
		}

		if retried && !strings.Contains(err.Error(), "is not indexed") {
			return nil, nil, err
		}
		retried = true

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
		return nil, nil, errors.Errorf("Unexpected status code: %v", status)
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, errors.Errorf("unable to read from body: %v", err)
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
	_, ts, err := queryWithTs(q1, "application/graphql+-", "", 0)
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

	mr, err := mutationWithTs(m1, "application/rdf", false, false, ts)
	require.NoError(t, err)
	require.Equal(t, mr.startTs, ts)
	require.Equal(t, 4, len(mr.keys))
	require.Equal(t, 2, len(mr.preds))
	var parsedPreds []string
	for _, pred := range mr.preds {
		parsedPreds = append(parsedPreds, strings.Join(strings.Split(pred, "-")[1:], "-"))
	}
	sort.Strings(parsedPreds)
	require.Equal(t, "balance", parsedPreds[0])
	require.Equal(t, "name", parsedPreds[1])

	data, _, err := queryWithTs(q1, "application/graphql+-", "", 0)
	require.NoError(t, err)
	require.Equal(t, `{"data":{"balances":[]}}`, data)

	// Query with same timestamp.
	data, _, err = queryWithTs(q1, "application/graphql+-", "", ts)
	require.NoError(t, err)
	require.Equal(t, `{"data":{"balances":[{"name":"Bob","balance":"110"}]}}`, data)

	// Commit and query.
	require.NoError(t, commitWithTs(mr.keys, mr.preds, ts))
	data, _, err = queryWithTs(q1, "application/graphql+-", "", 0)
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
	_, ts, err := queryWithTs(q1, "application/graphql+-", "", 0)
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

	mr, err := mutationWithTs(m1, "application/rdf", false, false, ts)
	require.NoError(t, err)
	require.Equal(t, mr.startTs, ts)
	require.Equal(t, 4, len(mr.keys))

	data, _, err := queryWithTs(q1, "application/graphql+-", "", 0)
	require.NoError(t, err)
	require.Equal(t, `{"data":{"balances":[]}}`, data)

	// Query with same timestamp.
	data, _, err = queryWithTs(q1, "application/graphql+-", "", ts)
	require.NoError(t, err)
	require.Equal(t, `{"data":{"balances":[{"name":"Bob","balance":"110"}]}}`, data)

	// Commit and query.
	require.NoError(t, commitWithTs(mr.keys, nil, ts))
	data, _, err = queryWithTs(q1, "application/graphql+-", "", 0)
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
	_, ts, err := queryWithTs(q1, "application/graphql+-", "", 0)
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

	mr, err := mutationWithTs(m1, "application/rdf", false, false, ts)
	require.NoError(t, err)
	require.Equal(t, mr.startTs, ts)
	require.Equal(t, 4, len(mr.keys))

	data, _, err := queryWithTs(q1, "application/graphql+-", "", 0)
	require.NoError(t, err)
	require.Equal(t, `{"data":{"balances":[]}}`, data)

	// Query with same timestamp.
	data, _, err = queryWithTs(q1, "application/graphql+-", "", ts)
	require.NoError(t, err)
	require.Equal(t, `{"data":{"balances":[{"name":"Bob","balance":"110"}]}}`, data)

	// One more time, with json body this time.
	d1, err := json.Marshal(params{Query: q1})
	require.NoError(t, err)
	data, _, err = queryWithTs(string(d1), "application/json", "", ts)
	require.NoError(t, err)
	require.Equal(t, `{"data":{"balances":[{"name":"Bob","balance":"110"}]}}`, data)

	// Commit (using a list of keys instead of a map) and query.
	require.NoError(t, commitWithTsKeysOnly(mr.keys, ts))
	data, _, err = queryWithTs(q1, "application/graphql+-", "", 0)
	require.NoError(t, err)
	require.Equal(t, `{"data":{"balances":[{"name":"Bob","balance":"110"}]}}`, data)

	// Aborting a transaction
	url := fmt.Sprintf("%s/commit?startTs=%d&abort=true", addr, ts)
	req, err := http.NewRequest("POST", url, nil)
	require.NoError(t, err)
	_, _, err = runRequest(req)
	require.NoError(t, err)
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
	require.Equal(t, qr.Errors[0].Extensions["code"], "Error")
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

	data, resp, err := queryWithGz(q1, "application/graphql+-", "false", "", false, false)
	require.NoError(t, err)
	require.Equal(t, r1, data)
	require.Empty(t, resp.Header.Get("Content-Encoding"))

	data, resp, err = queryWithGz(q1, "application/graphql+-", "", "", false, true)
	require.NoError(t, err)
	require.Equal(t, r1, data)
	require.Equal(t, "gzip", resp.Header.Get("Content-Encoding"))

	data, resp, err = queryWithGz(q1, "application/graphql+-", "", "", true, false)
	require.NoError(t, err)
	require.Equal(t, r1, data)
	require.Empty(t, resp.Header.Get("Content-Encoding"))

	data, resp, err = queryWithGz(q1, "application/graphql+-", "", "", true, true)
	require.NoError(t, err)
	require.Equal(t, r1, data)
	require.Equal(t, "gzip", resp.Header.Get("Content-Encoding"))

	// query with timeout
	data, _, err = queryWithGz(q1, "application/graphql+-", "", "1ms", false, false)
	require.EqualError(t, err, ": context deadline exceeded")
	require.Equal(t, "", data)

	data, resp, err = queryWithGz(q1, "application/graphql+-", "", "1s", false, false)
	require.NoError(t, err)
	require.Equal(t, r1, data)
	require.Empty(t, resp.Header.Get("Content-Encoding"))

	d1, err := json.Marshal(params{Query: q1})
	require.NoError(t, err)
	data, resp, err = queryWithGz(string(d1), "application/json", "", "1s", false, false)
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
	data, resp, err = queryWithGz(string(d2), "application/json", "", "1s", false, false)
	require.NoError(t, err)
	require.Equal(t, `{"data":{"names":[{"name":"Alice"}]}}`, data)
	require.Empty(t, resp.Header.Get("Content-Encoding"))
}

func TestDebugSupport(t *testing.T) {
	require.NoError(t, dropAll())
	require.NoError(t, alterSchema(`name: string @index(term) .`))

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
	  }
	}
	`
	err := runMutation(m1)
	require.NoError(t, err)

	q1 := `
	{
	  users(func: has(name), orderasc: name) {
	    name
	  }
	}
	`

	requireEqual := func(t *testing.T, data string) {
		var r struct {
			Data struct {
				Users []struct {
					Name string `json:"name"`
					UID  string `json:"uid"`
				} `json:"users"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(data), &r); err != nil {
			require.NoError(t, err)
		}

		exp := []string{"Alice", "Bob", "Charlie", "David", "Emily", "Frank", "Gloria"}
		actual := make([]string, 0, len(exp))
		for _, u := range r.Data.Users {
			actual = append(actual, u.Name)
			require.NotEmpty(t, u.UID, "uid should be nonempty in debug mode")
		}
		sort.Strings(actual)
		require.Equal(t, exp, actual)
	}

	data, resp, err := queryWithGz(q1, "application/graphql+-", "true", "", false, false)
	require.NoError(t, err)
	requireEqual(t, data)
	require.Empty(t, resp.Header.Get("Content-Encoding"))

	data, resp, err = queryWithGz(q1, "application/graphql+-", "true", "", false, true)
	require.NoError(t, err)
	requireEqual(t, data)
	require.Equal(t, "gzip", resp.Header.Get("Content-Encoding"))

	data, resp, err = queryWithGz(q1, "application/graphql+-", "true", "", true, false)
	require.NoError(t, err)
	requireEqual(t, data)
	require.Empty(t, resp.Header.Get("Content-Encoding"))

	data, resp, err = queryWithGz(q1, "application/graphql+-", "true", "", true, true)
	require.NoError(t, err)
	requireEqual(t, data)
	require.Equal(t, "gzip", resp.Header.Get("Content-Encoding"))

	// query with timeout
	data, _, err = queryWithGz(q1, "application/graphql+-", "true", "1ms", false, false)
	require.EqualError(t, err, ": context deadline exceeded")
	require.Equal(t, "", data)

	data, resp, err = queryWithGz(q1, "application/graphql+-", "true", "1s", false, false)
	require.NoError(t, err)
	requireEqual(t, data)
	require.Empty(t, resp.Header.Get("Content-Encoding"))

	d1, err := json.Marshal(params{Query: q1})
	require.NoError(t, err)
	data, resp, err = queryWithGz(string(d1), "application/json", "true", "1s", false, false)
	require.NoError(t, err)
	requireEqual(t, data)
	require.Empty(t, resp.Header.Get("Content-Encoding"))

	// This test passes access token along with debug flag
	data, _, err = queryWithTs(q1, "application/graphql+-", "true", 0)
	require.NoError(t, err)
	requireEqual(t, data)
	require.Empty(t, resp.Header.Get("Content-Encoding"))
}

func TestHealth(t *testing.T) {
	url := fmt.Sprintf("%s/health", addr)
	resp, err := http.Get(url)
	require.NoError(t, err)

	defer resp.Body.Close()
	data, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err)

	var info []pb.HealthInfo
	require.NoError(t, json.Unmarshal(data, &info))
	require.Equal(t, "alpha", info[0].Instance)
	require.True(t, info[0].Uptime > int64(time.Duration(1)))
}

func setDrainingMode(t *testing.T, enable bool) {
	drainingRequest := `mutation drain($enable: Boolean) {
		draining(enable: $enable) {
			response {
				code
			}
		}
	}`
	adminUrl := fmt.Sprintf("%s/admin", addr)
	params := testutil.GraphQLParams{
		Query:     drainingRequest,
		Variables: map[string]interface{}{"enable": enable},
	}
	b, err := json.Marshal(params)
	require.NoError(t, err)

	resp, err := http.Post(adminUrl, "application/json", bytes.NewBuffer(b))
	require.NoError(t, err)

	defer resp.Body.Close()
	b, err = ioutil.ReadAll(resp.Body)
	require.NoError(t, err)
	require.JSONEq(t, `{"data":{"draining":{"response":{"code":"Success"}}}}`,
		string(b))
}

func TestDrainingMode(t *testing.T) {
	runRequests := func(expectErr bool) {
		q1 := `
	{
	  alice(func: has(name)) {
	    name
	  }
	}
	`
		_, _, err := queryWithTs(q1, "application/graphql+-", "", 0)
		if expectErr {
			require.True(t, err != nil && strings.Contains(err.Error(), "the server is in draining mode"))
		} else {
			require.NoError(t, err, "Got error while running query: %v", err)
		}

		m1 := `
    {
	  set {
		_:alice <name> "Alice" .
	  }
	}
	`
		_, err = mutationWithTs(m1, "application/rdf", false, true, ts)
		if expectErr {
			require.True(t, err != nil && strings.Contains(err.Error(), "the server is in draining mode"))
		} else {
			require.NoError(t, err, "Got error while running mutation: %v", err)
		}

		err = alterSchema(`name: string @index(term) .`)
		if expectErr {
			require.True(t, err != nil && strings.Contains(err.Error(), "the server is in draining mode"))
		} else {
			require.NoError(t, err, "Got error while running alter: %v", err)
		}

	}

	setDrainingMode(t, true)
	runRequests(true)

	setDrainingMode(t, false)
	runRequests(false)
}
