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

package json

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/tok"
	"github.com/dgraph-io/dgraph/types"
	"github.com/golang/glog"
	"github.com/stretchr/testify/require"
	"github.com/twpayne/go-geom"
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

	nq, err := Parse(b, SetNquads)
	require.NoError(t, err)

	require.Equal(t, 5, len(nq))

	oval := &api.Value{Val: &api.Value_StrVal{StrVal: "Alice"}}
	require.Contains(t, nq, makeNquad("_:blank-0", "name", oval))

	oval = &api.Value{Val: &api.Value_IntVal{IntVal: 26}}
	require.Contains(t, nq, makeNquad("_:blank-0", "age", oval))

	oval = &api.Value{Val: &api.Value_BoolVal{BoolVal: true}}
	require.Contains(t, nq, makeNquad("_:blank-0", "married", oval))

	oval = &api.Value{Val: &api.Value_StrVal{StrVal: tn.Format(time.RFC3339Nano)}}
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

	nq, err := Parse(b, SetNquads)
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

	nq, err := Parse(b, SetNquads)
	require.NoError(t, err)

	require.Equal(t, 3, len(nq))
	require.Contains(t, nq, makeNquadEdge("_:blank-0", "school", "_:blank-1"))

	oval := &api.Value{Val: &api.Value_StrVal{StrVal: "Wellington Public School"}}
	require.Contains(t, nq, makeNquad("_:blank-1", "Name", oval))
}

func TestNquadsFromJson4(t *testing.T) {
	json := `[{"name":"Alice","mobile":"040123456","car":"MA0123", "age": 21, "weight": 58.7}]`

	nq, err := Parse([]byte(json), SetNquads)
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
		{`{"uid": "1", "key": "23452786"}`, &api.Value{Val: &api.Value_StrVal{StrVal: "23452786"}}},
		{`{"uid": "1", "key": "23452786.2378"}`, &api.Value{Val: &api.Value_StrVal{StrVal: "23452786.2378"}}},
		{`{"uid": "1", "key": -1e10}`, &api.Value{Val: &api.Value_DoubleVal{DoubleVal: -1e+10}}},
		{`{"uid": "1", "key": 0E-0}`, &api.Value{Val: &api.Value_DoubleVal{DoubleVal: 0}}},
	}

	for _, test := range tests {
		nqs, err := Parse([]byte(test.in), SetNquads)
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

	_, err := Parse([]byte(json), SetNquads)
	require.Error(t, err)
}

func TestNquadsFromJson_NegativeUidError(t *testing.T) {
	json := `{"uid":"-100","name":"Name","following":[{"name":"Bob"}],"school":[{"uid":"","name@en":"Crown Public School"}]}`

	_, err := Parse([]byte(json), SetNquads)
	require.Error(t, err)
}

func TestNquadsFromJson_EmptyUid(t *testing.T) {
	json := `{"uid":"","name":"Name","following":[{"name":"Bob"}],"school":[{"uid":"","name":"Crown Public School"}]}`

	nq, err := Parse([]byte(json), SetNquads)
	require.NoError(t, err)

	require.Equal(t, 5, len(nq))
	oval := &api.Value{Val: &api.Value_StrVal{StrVal: "Name"}}
	require.Contains(t, nq, makeNquad("_:blank-0", "name", oval))
}

func TestNquadsFromJson_BlankNodes(t *testing.T) {
	json := `{"uid":"_:alice","name":"Alice","following":[{"name":"Bob"}],"school":[{"uid":"_:school","name":"Crown Public School"}]}`

	nq, err := Parse([]byte(json), SetNquads)
	require.NoError(t, err)

	require.Equal(t, 5, len(nq))
	require.Contains(t, nq, makeNquadEdge("_:alice", "school", "_:school"))
}

func TestNquadsDeleteEdges(t *testing.T) {
	json := `[{"uid": "0x1","name":null,"mobile":null,"car":null}]`
	nq, err := Parse([]byte(json), DeleteNquads)
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

func getMapOfFacets(facets []*api.Facet) map[string]*api.Facet {
	res := make(map[string]*api.Facet)
	for _, f := range facets {
		res[f.Key] = f
	}
	return res
}

func checkFacets(t *testing.T, nq []*api.NQuad, pred string, facets []*api.Facet) {
	for _, n := range nq {
		if n.Predicate == pred {
			require.Equal(t, len(facets), len(n.Facets),
				fmt.Sprintf("expected %d facets, got %d", len(facets), len(n.Facets)))

			expectedFacets := getMapOfFacets(facets)
			actualFacets := getMapOfFacets(n.Facets)
			for key, f := range expectedFacets {
				actualF, ok := actualFacets[key]
				if !ok {
					t.Fatalf("facet for key %s not found", key)
				}
				require.Equal(t, f, actualF, fmt.Sprintf("expected:%v\ngot:%v", f, actualF))
			}
		}
	}
}

func TestNquadsFromJsonFacets1(t *testing.T) {
	// test the 5 data types on facets, string, bool, int, float and datetime
	operation := "READ WRITE"
	operationTokens, err := tok.GetTermTokens([]string{operation})
	require.NoError(t, err, "unable to get tokens from the string %s", operation)

	timeStr := "2006-01-02T15:04:05Z"
	time, err := types.ParseTime(timeStr)
	if err != nil {
		t.Fatalf("unable to convert string %s to time", timeStr)
	}
	timeBinary, err := time.MarshalBinary()
	if err != nil {
		t.Fatalf("unable to marshal time %v to binary", time)
	}

	carPrice := 30000.56
	var priceBytes [8]byte
	u := math.Float64bits(float64(carPrice))
	binary.LittleEndian.PutUint64(priceBytes[:], u)

	carAge := 3
	var ageBytes [8]byte
	binary.LittleEndian.PutUint64(ageBytes[:], uint64(carAge))

	json := fmt.Sprintf(`[{"name":"Alice","mobile":"040123456","car":"MA0123",`+
		`"mobile|operation": "%s",
         "car|first":true,
         "car|age": %d,
         "car|price": %f,
         "car|since": "%s"
}]`, operation, carAge, carPrice, timeStr)

	nq, err := Parse([]byte(json), SetNquads)
	require.NoError(t, err)
	require.Equal(t, 3, len(nq))

	for _, n := range nq {
		glog.Infof("%v", n)

	}

	checkFacets(t, nq, "mobile", []*api.Facet{
		{
			Key:     "operation",
			Value:   []byte(operation),
			ValType: api.Facet_STRING,
			Tokens:  operationTokens,
		},
	})

	checkFacets(t, nq, "car", []*api.Facet{
		{
			Key:     "first",
			Value:   []byte{1},
			ValType: api.Facet_BOOL,
		},
		{
			Key:     "age",
			Value:   ageBytes[:],
			ValType: api.Facet_INT,
		},
		{
			Key:     "price",
			Value:   priceBytes[:],
			ValType: api.Facet_FLOAT,
		},
		{
			Key:     "since",
			Value:   timeBinary,
			ValType: api.Facet_DATETIME,
		},
	})
}

func TestNquadsFromJsonFacets2(t *testing.T) {
	// Dave has uid facets which should go on the edge between Alice and Dave
	json := `[{"name":"Alice","friend":[{"name":"Dave","friend|close":"true"}]}]`

	nq, err := Parse([]byte(json), SetNquads)
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

	_, err = Parse(b, DeleteNquads)
	require.Error(t, err)
	require.Contains(t, err.Error(), "UID must be present and non-zero while deleting edges.")
}

func TestNquadsFromJsonList(t *testing.T) {
	json := `{"address":["Riley Street","Redfern"],"phone_number":[123,9876],"points":[{"type":"Point", "coordinates":[1.1,2.0]},{"type":"Point", "coordinates":[2.0,1.1]}]}`

	nq, err := Parse([]byte(json), SetNquads)
	require.NoError(t, err)
	require.Equal(t, 6, len(nq))
}

func TestNquadsFromJsonDelete(t *testing.T) {
	json := `{"uid":1000,"friend":[{"uid":1001}]}`

	nq, err := Parse([]byte(json), DeleteNquads)
	require.NoError(t, err)
	require.Equal(t, nq[0], makeNquadEdge("1000", "friend", "1001"))
}

func TestNquadsFromJsonDeleteStar(t *testing.T) {
	json := `{"uid":1000,"name": null}`

	nq, err := Parse([]byte(json), DeleteNquads)
	require.NoError(t, err)
	expected := &api.NQuad{
		Subject:   "1000",
		Predicate: "name",
		ObjectValue: &api.Value{
			Val: &api.Value_DefaultVal{
				DefaultVal: "_STAR_ALL",
			},
		},
	}
	require.Equal(t, expected, nq[0])
}

func TestNquadsFromJsonDeleteStarLang(t *testing.T) {
	json := `{"uid":1000,"name@es": null}`

	nq, err := Parse([]byte(json), DeleteNquads)
	require.NoError(t, err)
	expected := &api.NQuad{
		Subject:   "1000",
		Predicate: "name",
		ObjectValue: &api.Value{
			Val: &api.Value_DefaultVal{
				DefaultVal: "_STAR_ALL",
			},
		},
		Lang: "es",
	}
	require.Equal(t, expected, nq[0])
}

func TestSetNquadNilValue(t *testing.T) {
	json := `{"uid":1000,"name": null}`

	nq, err := Parse([]byte(json), SetNquads)
	require.NoError(t, err)
	require.Equal(t, 0, len(nq))
}
