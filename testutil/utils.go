/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package testutil

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/golang/glog"
	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgo/v250"
	"github.com/hypermodeinc/dgraph/v25/x"
)

func RootNsSchemaKey(attr string) []byte {
	attr = x.AttrInRootNamespace(attr)
	return x.SchemaKey(attr)
}

func RootNsTypeKey(attr string) []byte {
	attr = x.AttrInRootNamespace(attr)
	return x.TypeKey(attr)
}

func RootNsDataKey(attr string, uid uint64) []byte {
	attr = x.AttrInRootNamespace(attr)
	return x.DataKey(attr, uid)
}

func RootNsReverseKey(attr string, uid uint64) []byte {
	attr = x.AttrInRootNamespace(attr)
	return x.ReverseKey(attr, uid)
}

func RootNsIndexKey(attr, term string) []byte {
	attr = x.AttrInRootNamespace(attr)
	return x.IndexKey(attr, term)
}

func RootNsCountKey(attr string, count uint32, reverse bool) []byte {
	attr = x.AttrInRootNamespace(attr)
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
		defer func() {
			if err := response.Body.Close(); err != nil {
				glog.Warningf("error closing body: %v", err)
			}
		}()

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
