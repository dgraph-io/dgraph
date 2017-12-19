package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
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

func queryWithTs(q string, ts uint64) (string, uint64, error) {
	url := "/query"
	if ts != 0 {
		url += "/" + strconv.FormatUint(ts, 10)
	}
	req, err := http.NewRequest("POST", url, bytes.NewBufferString(q))
	if err != nil {
		return "", 0, err
	}
	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(queryHandler)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		return "", 0, fmt.Errorf("Unexpected status code: %v", status)
	}

	var qr x.QueryResWithData
	json.Unmarshal(rr.Body.Bytes(), &qr)
	if len(qr.Errors) > 0 {
		return "", 0, errors.New(qr.Errors[0].Message)
	}

	var r res
	x.Check(json.Unmarshal(rr.Body.Bytes(), &r))
	startTs := r.Extensions.Txn.StartTs

	// Remove the extensions.
	r2 := res{
		Data: r.Data,
	}
	output, err := json.Marshal(r2)

	return string(output), startTs, err
}

func mutationWithTs(m string, commitNow bool, ignoreIndexConflict bool,
	ts uint64) ([]string, uint64, error) {
	url := "/mutate"
	if ts != 0 {
		url += "/" + strconv.FormatUint(ts, 10)
	}
	var keys []string
	req, err := http.NewRequest("POST", url, bytes.NewBufferString(m))
	if err != nil {
		return keys, 0, err
	}

	if commitNow {
		req.Header.Set("X-Dgraph-CommitNow", "true")
	}
	if ignoreIndexConflict {
		req.Header.Set("X-Dgraph-IgnoreIndexConflict", "true")
	}
	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(mutationHandler)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		return keys, 0, fmt.Errorf("Unexpected status code: %v", status)
	}
	var qr x.QueryResWithData
	json.Unmarshal(rr.Body.Bytes(), &qr)
	if len(qr.Errors) > 0 {
		return keys, 0, errors.New(qr.Errors[0].Message)
	}

	var r res
	x.Check(json.Unmarshal(rr.Body.Bytes(), &r))
	startTs := r.Extensions.Txn.StartTs

	return r.Extensions.Txn.Keys, startTs, nil
}

func commitWithTs(keys []string, ts uint64) error {
	url := "/commit"
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

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(commitHandler)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		return fmt.Errorf("Unexpected status code: %v", status)
	}

	var qr x.QueryResWithData
	json.Unmarshal(rr.Body.Bytes(), &qr)
	if len(qr.Errors) > 0 {
		return errors.New(qr.Errors[0].Message)
	}

	return nil
}

func TestTransactionBasic(t *testing.T) {
	require.NoError(t, dropAll())
	require.NoError(t, alterSchema(`name: string @index(term) .`))

	q1 := `
	{
	  balances(func: anyofterms(name, "Alice Bob")) {
	    uid
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
		<0x1> <name> "Alice" .
		<0x1> <name> "Bob" .
		<0x1> <balance> "110" .
	    <0x2> <balance> "60" .
	  }
	}
	`

	keys, mts, err := mutationWithTs(m1, false, true, ts)
	require.NoError(t, err)
	require.Equal(t, mts, ts)
	expected := []string{"321112eei4n9g", "321112eei4n9g", "3fk4wxiwz6h3r", "3mlibw7eeno0x"}
	sort.Strings(expected)
	sort.Strings(keys)
	require.Equal(t, expected, keys)

	data, _, err := queryWithTs(q1, 0)
	require.NoError(t, err)
	require.Equal(t, `{"data":{"balances":[]}}`, data)

	// Query with same timestamp.
	data, _, err = queryWithTs(q1, ts)
	require.NoError(t, err)
	require.Equal(t, `{"data":{"balances":[{"uid":"0x1","name":"Bob","balance":"110"}]}}`, data)

	// Commit and query.
	require.NoError(t, commitWithTs(keys, ts))
	data, _, err = queryWithTs(q1, 0)
	require.NoError(t, err)
	require.Equal(t, `{"data":{"balances":[{"uid":"0x1","name":"Bob","balance":"110"}]}}`, data)
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
