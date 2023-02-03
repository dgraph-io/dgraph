/*
 * Copyright 2022 Dgraph Labs, Inc. and Contributors
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
	"log"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgo/v210"
	"github.com/dgraph-io/dgraph/x"
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

func WaitForTask(t *testing.T, taskId string, useHttps bool, socketAddrHttp string) {
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
		time.Sleep(4 * time.Second)

		var adminUrl string
		var client http.Client
		if useHttps {
			adminUrl = "https://" + socketAddrHttp + "/admin"
			client = GetHttpsClient(t)
		} else {
			adminUrl = "http://" + socketAddrHttp + "/admin"
			client = *http.DefaultClient
		}
		response, err := client.Post(adminUrl, "application/json", bytes.NewBuffer(request))
		require.NoError(t, err)
		defer response.Body.Close()

		var data interface{}
		require.NoError(t, json.NewDecoder(response.Body).Decode(&data))
		status := JsonGet(data, "data", "task", "status").(string)
		switch status {
		case "Success":
			log.Printf("export complete")
			return
		case "Failed", "Unknown":
			t.Errorf("task failed with status: %s", status)
		}
	}
}

func JsonGet(j interface{}, components ...string) interface{} {
	for _, component := range components {
		j = j.(map[string]interface{})[component]
	}
	return j
}

func PollTillPassOrTimeout(t *testing.T, dc *dgo.Dgraph, query, want string, timeout time.Duration) {
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	to := time.NewTimer(timeout)
	defer to.Stop()
	for {
		select {
		case <-to.C:
			wantMap := UnmarshalJSON(t, want)
			gotMap := UnmarshalJSON(t, string(QueryData(t, dc, query)))
			DiffJSONMaps(t, wantMap, gotMap, "", false)
			return // timeout
		case <-ticker.C:
			wantMap := UnmarshalJSON(t, want)
			gotMap := UnmarshalJSON(t, string(QueryData(t, dc, query)))
			if DiffJSONMaps(t, wantMap, gotMap, "", true) {
				return
			}
		}
	}
}
