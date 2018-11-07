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

package edgraph

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
	"github.com/stretchr/testify/require"
	geom "github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/encoding/geojson"
)

func makeNquad(sub, pred string, val *api.Value) *api.NQuad {
	return &api.NQuad{
		Subject:     sub,
		Predicate:   pred,
		ObjectValue: val,
	}
}

func makeNquadEdge(sub, pred, obj string) *api.NQuad {
	return &api.NQuad{
		Subject:   sub,
		Predicate: pred,
		ObjectId:  obj,
	}
}

type School struct {
	Name string `json:",omitempty"`
}

type address struct {
	Type   string    `json:"type,omitempty"`
	Coords []float64 `json:"coordinates,omitempty"`
}

type Person struct {
	Uid     string     `json:"uid,omitempty"`
	Name    string     `json:"name,omitempty"`
	Age     int        `json:"age,omitempty"`
	Married *bool      `json:"married,omitempty"`
	Now     *time.Time `json:"now,omitempty"`
	Address address    `json:"address,omitempty"` // geo value
	Friends []Person   `json:"friend,omitempty"`
	School  *School    `json:"school,omitempty"`
}

func TestNquadsFromJson1(t *testing.T) {
	tn := time.Now().UTC()
	geoVal := `{"Type":"Point", "Coordinates":[1.1,2.0]}`
	m := true
	p := Person{
		Name:    "Alice",
		Age:     26,
		Married: &m,
		Now:     &tn,
		Address: address{
			Type:   "Point",
			Coords: []float64{1.1, 2.0},
		},
	}

	b, err := json.Marshal(p)
	require.NoError(t, err)

	nq, err := nquadsFromJson(b, set)
	require.NoError(t, err)

	require.Equal(t, 5, len(nq))

	oval := &api.Value{Val: &api.Value_StrVal{StrVal: "Alice"}}
	require.Contains(t, nq, makeNquad("_:blank-0", "name", oval))

	oval = &api.Value{Val: &api.Value_IntVal{IntVal: 26}}
	require.Contains(t, nq, makeNquad("_:blank-0", "age", oval))

	oval = &api.Value{Val: &api.Value_BoolVal{BoolVal: true}}
	require.Contains(t, nq, makeNquad("_:blank-0", "married", oval))

	oval, err = types.ObjectValue(types.DateTimeID, tn)
	require.NoError(t, err)
	require.Contains(t, nq, makeNquad("_:blank-0", "now", oval))

	var g geom.T
	err = geojson.Unmarshal([]byte(geoVal), &g)
	require.NoError(t, err)
	geo, err := types.ObjectValue(types.GeoID, g)
	require.NoError(t, err)

	require.Contains(t, nq, makeNquad("_:blank-0", "address", geo))
}

func TestNquadsFromJson2(t *testing.T) {
	m := false

	p := Person{
		Name: "Alice",
		Friends: []Person{{
			Name:    "Charlie",
			Married: &m,
		}, {
			Uid:  "1000",
			Name: "Bob",
		}},
	}

	b, err := json.Marshal(p)
	require.NoError(t, err)

	nq, err := nquadsFromJson(b, set)
	require.NoError(t, err)

	require.Equal(t, 6, len(nq))
	require.Contains(t, nq, makeNquadEdge("_:blank-0", "friend", "_:blank-1"))
	require.Contains(t, nq, makeNquadEdge("_:blank-0", "friend", "1000"))

	oval := &api.Value{Val: &api.Value_StrVal{StrVal: "Charlie"}}
	require.Contains(t, nq, makeNquad("_:blank-1", "name", oval))

	oval = &api.Value{Val: &api.Value_BoolVal{BoolVal: false}}
	require.Contains(t, nq, makeNquad("_:blank-1", "married", oval))

	oval = &api.Value{Val: &api.Value_StrVal{StrVal: "Bob"}}
	require.Contains(t, nq, makeNquad("1000", "name", oval))
}

func TestNquadsFromJson3(t *testing.T) {
	p := Person{
		Name: "Alice",
		School: &School{
			Name: "Wellington Public School",
		},
	}

	b, err := json.Marshal(p)
	require.NoError(t, err)

	nq, err := nquadsFromJson(b, set)
	require.NoError(t, err)

	require.Equal(t, 3, len(nq))
	require.Contains(t, nq, makeNquadEdge("_:blank-0", "school", "_:blank-1"))

	oval := &api.Value{Val: &api.Value_StrVal{StrVal: "Wellington Public School"}}
	require.Contains(t, nq, makeNquad("_:blank-1", "Name", oval))
}

func TestNquadsFromJson4(t *testing.T) {
	json := `[{"name":"Alice","mobile":"040123456","car":"MA0123", "age": 21, "weight": 58.7}]`

	nq, err := nquadsFromJson([]byte(json), set)
	require.NoError(t, err)
	require.Equal(t, 5, len(nq))
	oval := &api.Value{Val: &api.Value_StrVal{StrVal: "Alice"}}
	require.Contains(t, nq, makeNquad("_:blank-0", "name", oval))
	require.Contains(t, nq, makeNquad("_:blank-0", "age", &api.Value{Val: &api.Value_IntVal{IntVal: 21}}))
	require.Contains(t, nq, makeNquad("_:blank-0", "weight",
		&api.Value{Val: &api.Value_DoubleVal{DoubleVal: 58.7}}))
}

func TestJsonNumberParsing(t *testing.T) {
	tests := []struct {
		in  string
		out *api.Value
	}{
		{`{"uid": "1", "key": 9223372036854775299}`, &api.Value{Val: &api.Value_IntVal{IntVal: 9223372036854775299}}},
		{`{"uid": "1", "key": 9223372036854775299.0}`, &api.Value{Val: &api.Value_DoubleVal{DoubleVal: 9223372036854775299.0}}},
		{`{"uid": "1", "key": 27670116110564327426}`, nil},
		{`{"uid": "1", "key": "23452786"}`, &api.Value{Val: &api.Value_IntVal{IntVal: 23452786}}},
		{`{"uid": "1", "key": "23452786.2378"}`, &api.Value{Val: &api.Value_DoubleVal{DoubleVal: 23452786.2378}}},
		{`{"uid": "1", "key": -1e10}`, &api.Value{Val: &api.Value_DoubleVal{DoubleVal: -1e+10}}},
		{`{"uid": "1", "key": 0E-0}`, &api.Value{Val: &api.Value_DoubleVal{DoubleVal: 0}}},
	}

	for _, test := range tests {
		nqs, err := nquadsFromJson([]byte(test.in), set)
		if test.out != nil {
			require.NoError(t, err, "%T", err)
			require.Equal(t, makeNquad("1", "key", test.out), nqs[0])
		} else {
			require.Error(t, err)
		}
	}
}

func TestNquadsFromJson_UidOutofRangeError(t *testing.T) {
	json := `{"uid":"0xa14222b693e4ba34123","name":"Name","following":[{"name":"Bob"}],"school":[{"uid":"","name@en":"Crown Public School"}]}`

	_, err := nquadsFromJson([]byte(json), set)
	require.Error(t, err)
}

func TestNquadsFromJson_NegativeUidError(t *testing.T) {
	json := `{"uid":"-100","name":"Name","following":[{"name":"Bob"}],"school":[{"uid":"","name@en":"Crown Public School"}]}`

	_, err := nquadsFromJson([]byte(json), set)
	require.Error(t, err)
}

func TestNquadsFromJson_EmptyUid(t *testing.T) {
	json := `{"uid":"","name":"Name","following":[{"name":"Bob"}],"school":[{"uid":"","name":"Crown Public School"}]}`

	nq, err := nquadsFromJson([]byte(json), set)
	require.NoError(t, err)

	require.Equal(t, 5, len(nq))
	oval := &api.Value{Val: &api.Value_StrVal{StrVal: "Name"}}
	require.Contains(t, nq, makeNquad("_:blank-0", "name", oval))
}

func TestNquadsFromJson_BlankNodes(t *testing.T) {
	json := `{"uid":"_:alice","name":"Alice","following":[{"name":"Bob"}],"school":[{"uid":"_:school","name":"Crown Public School"}]}`

	nq, err := nquadsFromJson([]byte(json), set)
	require.NoError(t, err)

	require.Equal(t, 5, len(nq))
	require.Contains(t, nq, makeNquadEdge("_:alice", "school", "_:school"))
}

func TestNquadsDeleteEdges(t *testing.T) {
	json := `[{"uid": "0x1","name":null,"mobile":null,"car":null}]`
	nq, err := nquadsFromJson([]byte(json), delete)
	require.NoError(t, err)
	require.Equal(t, 3, len(nq))
}

func checkCount(t *testing.T, nq []*api.NQuad, pred string, count int) {
	for _, n := range nq {
		if n.Predicate == pred {
			require.Equal(t, count, len(n.Facets))
			break
		}
	}
}

func TestNquadsFromJsonFacets1(t *testing.T) {
	json := `[{"name":"Alice","mobile":"040123456","car":"MA0123","mobile|since":"2006-01-02T15:04:05Z","car|first":"true"}]`

	nq, err := nquadsFromJson([]byte(json), set)
	require.NoError(t, err)
	require.Equal(t, 3, len(nq))
	checkCount(t, nq, "mobile", 1)
	checkCount(t, nq, "car", 1)
}

func TestNquadsFromJsonFacets2(t *testing.T) {
	// Dave has uid facets which should go on the edge between Alice and Dave
	json := `[{"name":"Alice","friend":[{"name":"Dave","friend|close":"true"}]}]`

	nq, err := nquadsFromJson([]byte(json), set)
	require.NoError(t, err)
	require.Equal(t, 3, len(nq))
	checkCount(t, nq, "friend", 1)
}

func TestNquadsFromJsonError1(t *testing.T) {
	p := Person{
		Name: "Alice",
		School: &School{
			Name: "Wellington Public School",
		},
	}

	b, err := json.Marshal(p)
	require.NoError(t, err)

	_, err = nquadsFromJson(b, delete)
	require.Error(t, err)
	require.Contains(t, err.Error(), "uid must be present and non-zero while deleting edges.")
}

func TestNquadsFromJsonList(t *testing.T) {
	json := `{"address":["Riley Street","Redfern"],"phone_number":[123,9876],"points":[{"type":"Point", "coordinates":[1.1,2.0]},{"type":"Point", "coordinates":[2.0,1.1]}]}`

	nq, err := nquadsFromJson([]byte(json), set)
	require.NoError(t, err)
	require.Equal(t, 6, len(nq))
}

func TestNquadsFromJsonDelete(t *testing.T) {
	json := `{"uid":1000,"friend":[{"uid":1001}]}`

	nq, err := nquadsFromJson([]byte(json), delete)
	require.NoError(t, err)
	require.Equal(t, nq[0], makeNquadEdge("1000", "friend", "1001"))
}

func TestParseNQuads(t *testing.T) {
	nquads := `
		_:a <predA> "A" .
		_:b <predB> "B" .
		# this line is a comment
		_:a <join> _:b .
	`
	nqs, err := parseNQuads([]byte(nquads))
	require.NoError(t, err)
	require.Equal(t, []*api.NQuad{
		makeNquad("_:a", "predA", &api.Value{Val: &api.Value_DefaultVal{DefaultVal: "A"}}),
		makeNquad("_:b", "predB", &api.Value{Val: &api.Value_DefaultVal{DefaultVal: "B"}}),
		makeNquadEdge("_:a", "join", "_:b"),
	}, nqs)
}

func TestParseNQuadsWindowsNewline(t *testing.T) {
	nquads := "_:a <predA> \"A\" .\r\n_:b <predB> \"B\" ."
	nqs, err := parseNQuads([]byte(nquads))
	require.NoError(t, err)
	require.Equal(t, []*api.NQuad{
		makeNquad("_:a", "predA", &api.Value{Val: &api.Value_DefaultVal{DefaultVal: "A"}}),
		makeNquad("_:b", "predB", &api.Value{Val: &api.Value_DefaultVal{DefaultVal: "B"}}),
	}, nqs)
}

func TestParseNQuadsDelete(t *testing.T) {
	nquads := `_:a * * .`
	nqs, err := parseNQuads([]byte(nquads))
	require.NoError(t, err)
	require.Equal(t, []*api.NQuad{
		makeNquad("_:a", x.Star, &api.Value{Val: &api.Value_DefaultVal{DefaultVal: x.Star}}),
	}, nqs)
}

func TestValidateKeys(t *testing.T) {
	tests := []struct {
		name    string
		nquad   string
		noError bool
	}{
		{name: "test 1", nquad: `_:alice <knows> "stuff" ( "key 1" = 12 ) .`, noError: false},
		{name: "test 2", nquad: `_:alice <knows> "stuff" ( "key	1" = 12 ) .`, noError: false},
		{name: "test 3", nquad: `_:alice <knows> "stuff" ( ~key1 = 12 ) .`, noError: false},
		{name: "test 4", nquad: `_:alice <knows> "stuff" ( "~key1" = 12 ) .`, noError: false},
		{name: "test 5", nquad: `_:alice <~knows> "stuff" ( "key 1" = 12 ) .`, noError: false},
		{name: "test 6", nquad: `_:alice <~knows> "stuff" ( "key	1" = 12 ) .`, noError: false},
		{name: "test 7", nquad: `_:alice <~knows> "stuff" ( key1 = 12 ) .`, noError: false},
		{name: "test 8", nquad: `_:alice <~knows> "stuff" ( "key1" = 12 ) .`, noError: false},
		{name: "test 9", nquad: `_:alice <~knows> "stuff" ( "key	1" = 12 ) .`, noError: false},
		{name: "test 10", nquad: `_:alice <knows> "stuff" ( key1 = 12 , "key 2" = 13 ) .`, noError: false},
		{name: "test 11", nquad: `_:alice <knows> "stuff" ( "key1" = 12, key2 = 13 , "key	3" = "a b" ) .`, noError: false},
		{name: "test 12", nquad: `_:alice <knows~> "stuff" ( key1 = 12 ) .`, noError: false},
		{name: "test 13", nquad: `_:alice <knows> "stuff" ( key1 = 12 ) .`, noError: true},
		{name: "test 14", nquad: `_:alice <knows@some> "stuff" .`, noError: true},
		{name: "test 15", nquad: `_:alice <knows@some@en> "stuff" .`, noError: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			nq, err := parseNQuads([]byte(tc.nquad))
			require.NoError(t, err)

			err = validateKeys(nq[0])
			if tc.noError {
				require.NoError(t, err, "Unexpected error for: %+v", nq)
			} else {
				require.Error(t, err, "Expected an error: %+v", nq)
			}
		})
	}
}
