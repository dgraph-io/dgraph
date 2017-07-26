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
	"io/ioutil"
	"log"
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

func TestShare(t *testing.T) {
	dgraphServer := "http://localhost:8080/share?debug=true"
	client := new(http.Client)
	q := `%7B%0A%20%20me(func:%20eq(name,%20%22Steven%20Spielberg%22))%20%7B%0A%09%09name%0A%09%09director.film%20%7B%0A%09%09%09name%0A%09%09%7D%0A%20%20%7D%0A%7D`
	req, err := http.NewRequest("POST", dgraphServer, strings.NewReader(q))
	resp, err := client.Do(req)
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}

	var r Res
	json.Unmarshal(b, &r)
	require.NotNil(t, r.Uids["share"])
}
