/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package testing

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type Res struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Uids    map[string]string `json:"uids"`
}

type Share struct {
	Share     string `json:"_share_"`
	ShareHash string `json:"_share_hash_"`
}

type Res2 struct {
	Root []Share `json:"me"`
}

type Res3 struct {
	Root Res2 `json:"data"`
}

func TestShare(t *testing.T) {
	dgraphServer := "http://localhost:8080/share?debug=true"
	client := new(http.Client)
	q := `%7B%0A%20%20me(func:%20eq(name,%20%22Steven%20Spielberg%22))%20%7B%0A%09%09name%0A%09%09director.film%20%7B%0A%09%09%09name%0A%09%09%7D%0A%20%20%7D%0A%7D`
	req, err := http.NewRequest("POST", dgraphServer, strings.NewReader(q))
	require.NoError(t, err)
	resp, err := client.Do(req)
	require.NoError(t, err)
	b, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err)

	var r Res
	json.Unmarshal(b, &r)
	require.NotNil(t, r.Uids["share"])

	q2 := fmt.Sprintf(`
	{
		me(func: uid(%s)) {
			_share_
			_share_hash_
		}
	}
	`, r.Uids["share"])

	dgraphServer = "http://localhost:8080/query"
	req, err = http.NewRequest("POST", dgraphServer, strings.NewReader(q2))
	require.NoError(t, err)
	resp, err = client.Do(req)
	require.NoError(t, err)
	b, err = ioutil.ReadAll(resp.Body)
	require.NoError(t, err)

	var r3 Res3
	json.Unmarshal(b, &r3)
	r2 := r3.Root
	require.Equal(t, 1, len(r2.Root))
	require.Equal(t, q, r2.Root[0].Share)
	require.NotNil(t, r2.Root[0].ShareHash)

}
