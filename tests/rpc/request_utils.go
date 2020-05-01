// Copyright 2020 ChainSafe Systems (ON) Corp.
// This file is part of gossamer.
//
// The gossamer library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The gossamer library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the gossamer library. If not, see <http://www.gnu.org/licenses/>.

package rpc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

// PostRPC utils for sending payload to endpoint and getting []byte back
func PostRPC(t *testing.T, method, host, params string) ([]byte, error) {

	data := []byte(`{"jsonrpc":"2.0","method":"` + method + `","params":` + params + `,"id":1}`)
	buf := &bytes.Buffer{}
	_, err := buf.Write(data)
	require.Nil(t, err)

	r, err := http.NewRequest("POST", host, buf)
	if err != nil {
		return nil, fmt.Errorf("not available")
	}

	r.Header.Set("Content-Type", ContentTypeJSON)
	r.Header.Set("Accept", ContentTypeJSON)

	resp, err := httpClient.Do(r)
	if err != nil || resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("not available")
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	respBody, err := ioutil.ReadAll(resp.Body)
	require.Nil(t, err)

	return respBody, nil

}

// DecodeRPC will decode []body into target interface
func DecodeRPC(t *testing.T, body []byte, target interface{}) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()

	var response ServerResponse
	err := decoder.Decode(&response)
	require.Nil(t, err, "respBody", string(body))

	t.Log("Got payload from RPC request", "serverResponse", response, "string(respBody)", string(body))

	require.Nil(t, response.Error, "respBody", string(body))
	require.Equal(t, response.Version, "2.0")

	decoder = json.NewDecoder(bytes.NewReader(response.Result))
	decoder.DisallowUnknownFields()

	err = decoder.Decode(&target)
	require.Nil(t, err, "respBody", string(body))
}
