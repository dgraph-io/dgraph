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
	"log"
	"net/http"
	"testing"
	"time"

	"github.com/dgraph-io/dgraph/x"
	"github.com/dgrijalva/jwt-go"
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

type JwtParams struct {
	User   string
	Groups []string
	Ns     uint64
	Exp    time.Duration
	Secret []byte
}

// GetAccessJwt constructs an access jwt with the given user id, groupIds, namespace
// and expiration TTL.
func GetAccessJwt(t *testing.T, params JwtParams) string {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"userid":    params.User,
		"groups":    params.Groups,
		"namespace": params.Ns,
		// set the jwt exp according to the ttl
		"exp": time.Now().Add(params.Exp).Unix(),
	})

	jwtString, err := token.SignedString(params.Secret)
	require.NoError(t, err)
	return jwtString
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
		time.Sleep(4 * time.Second)

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
