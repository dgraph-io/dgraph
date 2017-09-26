package dgraph

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/types"
	"github.com/stretchr/testify/require"
	geom "github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/encoding/geojson"
)

func makeNquad(sub, pred string, val *protos.Value, t types.TypeID) *protos.NQuad {
	return &protos.NQuad{
		Subject:     sub,
		Predicate:   pred,
		ObjectValue: val,
		ObjectType:  int32(t),
	}
}

func makeNquadEdge(sub, pred, obj string) *protos.NQuad {
	return &protos.NQuad{
		Subject:   sub,
		Predicate: pred,
		ObjectId:  obj,
	}
}

type School struct {
	Name string `json:",omitempty"`
}

type Person struct {
	Uid     uint64     `json:"_uid_,omitempty"`
	Name    string     `json:"name,omitempty"`
	Age     int        `json:"age,omitempty"`
	Married *bool      `json:"married,omitempty"`
	Now     *time.Time `json:"now,omitempty"`
	Address string     `json:"address,omitempty"` // geo value
	Friends []Person   `json:"friend,omitempty"`
	School  *School    `json:"school,omitempty"`
}

func TestNquadsFromJson1(t *testing.T) {
	tn := time.Now()
	geoVal := `{"Type":"Point", "Coordinates":[1.1,2.0]}`
	m := true
	p := Person{
		Name:    "Alice",
		Age:     26,
		Married: &m,
		Now:     &tn,
		Address: geoVal,
	}

	b, err := json.Marshal(p)
	require.NoError(t, err)

	nq, err := nquadsFromJson(b, set)
	require.NoError(t, err)

	require.Equal(t, 5, len(nq))

	oval := &protos.Value{&protos.Value_StrVal{"Alice"}}
	require.Contains(t, nq, makeNquad("_:blank-0", "name", oval, types.StringID))

	oval = &protos.Value{&protos.Value_DoubleVal{26}}
	require.Contains(t, nq, makeNquad("_:blank-0", "age", oval, types.FloatID))

	oval = &protos.Value{&protos.Value_BoolVal{true}}
	require.Contains(t, nq, makeNquad("_:blank-0", "married", oval, types.BoolID))

	oval = &protos.Value{&protos.Value_StrVal{tn.Format(time.RFC3339Nano)}}
	require.Contains(t, nq, makeNquad("_:blank-0", "now", oval, types.StringID))

	var g geom.T
	err = geojson.Unmarshal([]byte(geoVal), &g)
	require.NoError(t, err)
	geo, err := types.ObjectValue(types.GeoID, g)
	require.NoError(t, err)

	require.Contains(t, nq, makeNquad("_:blank-0", "address", geo, types.GeoID))
}

func TestNquadsFromJson2(t *testing.T) {
	m := false

	p := Person{
		Name: "Alice",
		Friends: []Person{{
			Name:    "Charlie",
			Married: &m,
		}, {
			Uid:  1000,
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

	oval := &protos.Value{&protos.Value_StrVal{"Charlie"}}
	require.Contains(t, nq, makeNquad("_:blank-1", "name", oval, types.StringID))

	oval = &protos.Value{&protos.Value_BoolVal{false}}
	require.Contains(t, nq, makeNquad("_:blank-1", "married", oval, types.BoolID))

	oval = &protos.Value{&protos.Value_StrVal{"Bob"}}
	require.Contains(t, nq, makeNquad("1000", "name", oval, types.StringID))
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

	oval := &protos.Value{&protos.Value_StrVal{"Wellington Public School"}}
	require.Contains(t, nq, makeNquad("_:blank-1", "Name", oval, types.StringID))
}

func TestNquadsFromJsonError(t *testing.T) {
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
	require.Contains(t, err.Error(), "_uid_ must be present and non-zero.")
}
