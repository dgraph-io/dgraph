/*
 * Copyright 2021 Dgraph Labs, Inc. and Contributors
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

package testutil

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/dgraph-io/dgraph/x"
	"github.com/stretchr/testify/require"
)

func GalaxySchemaKey(attr string) []byte {
	attr = x.GalaxyAttr(attr)
	return x.SchemaKey(attr)
}

func GalaxyTypeKey(attr string) []byte {
	attr = x.GalaxyAttr(attr)
	return x.TypeKey(attr)
}

func GalaxyDataKey(attr string, uid uint64) []byte {
	attr = x.GalaxyAttr(attr)
	return x.DataKey(attr, uid)
}

func GalaxyReverseKey(attr string, uid uint64) []byte {
	attr = x.GalaxyAttr(attr)
	return x.ReverseKey(attr, uid)
}

func GalaxyIndexKey(attr, term string) []byte {
	attr = x.GalaxyAttr(attr)
	return x.IndexKey(attr, term)
}

func GalaxyCountKey(attr string, count uint32, reverse bool) []byte {
	attr = x.GalaxyAttr(attr)
	return x.CountKey(attr, count, reverse)
}

func WaitForTask(t *testing.T, taskId string, useHttps bool) {
	const query = `query task($id: String!) {
		task(input: {id: $id}) {
			status
		}
	}`
	params := GraphQLParams{
		Query:     query,
		Variables: map[string]interface{}{"id": taskId},
	}
	request, err := json.Marshal(params)
	require.NoError(t, err)

	for {
		var adminUrl string
		var client http.Client
		if useHttps {
			adminUrl = "https://" + SockAddrHttp + "/admin"
			client = GetHttpsClient(t)
		} else {
			adminUrl = "http://" + SockAddrHttp + "/admin"
			client = *http.DefaultClient
		}
		response, err := client.Post(adminUrl, "application/json", bytes.NewBuffer(request))
		require.NoError(t, err)
		defer response.Body.Close()

		var data interface{}
		require.NoError(t, json.NewDecoder(response.Body).Decode(&data))
		type m = map[string]interface{}
		if data.(m)["data"].(m)["task"].(m)["status"] == "Success" {
			return
		}

		time.Sleep(4 * time.Second)
	}
}
