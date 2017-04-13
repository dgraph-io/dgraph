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
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func decodeResponse(q string) string {
	dgraphServer := "http://localhost:8080/query"
	client := new(http.Client)
	req, err := http.NewRequest("POST", dgraphServer, strings.NewReader(q))
	resp, err := client.Do(req)
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}
	return string(b)
}

func TestSimple(t *testing.T) {
	// Run mutation.
	m := `mutation {
        set {
            <alice-in-wonderland> <type> <novel> .
            <alice-in-wonderland> <character> <alice> .
            <alice-in-wonderland> <author> <lewis-carrol> .
            <alice-in-wonderland> <written-in> "1865" .
            <alice-in-wonderland> <name> "Alice in Wonderland" .
            <alice-in-wonderland> <sequel> <looking-glass> .
            <alice> <name> "Alice" .
            <lewis-carrol> <name> "Lewis Carroll" .
            <lewis-carrol> <born> "1832" .
            <lewis-carrol> <died> "1898" .
        }
     }`

	decodeResponse(m)

	q := `
    {
    	me(id: alice-in-wonderland) {
    		type
    		written-in
    		name
    		character {
                name
    		}
    		author {
                name
                born
                died
    		}
    	}
    }`

	expectedRes := `{"me":[{"author":[{"born":"1832","died":"1898","name":"Lewis Carroll"}],"character":[{"name":"Alice"}],"name":"Alice in Wonderland","written-in":"1865"}]}`
	res := decodeResponse(q)
	require.JSONEq(t, expectedRes, res)
}
