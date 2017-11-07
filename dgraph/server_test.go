package dgraph

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
	"github.com/stretchr/testify/require"
	geom "github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/encoding/geojson"
)

func makeNquad(sub, pred string, val *protos.Value) *protos.NQuad {
	return &protos.NQuad{
		Subject:     sub,
		Predicate:   pred,
		ObjectValue: val,
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
	Uid     uint64     `json:"uid,omitempty"`
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
	require.Contains(t, nq, makeNquad("_:blank-0", "name", oval))

	oval = &protos.Value{&protos.Value_DoubleVal{26}}
	require.Contains(t, nq, makeNquad("_:blank-0", "age", oval))

	oval = &protos.Value{&protos.Value_BoolVal{true}}
	require.Contains(t, nq, makeNquad("_:blank-0", "married", oval))

	oval = &protos.Value{&protos.Value_StrVal{tn.Format(time.RFC3339Nano)}}
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
	require.Contains(t, nq, makeNquad("_:blank-1", "name", oval))

	oval = &protos.Value{&protos.Value_BoolVal{false}}
	require.Contains(t, nq, makeNquad("_:blank-1", "married", oval))

	oval = &protos.Value{&protos.Value_StrVal{"Bob"}}
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

	oval := &protos.Value{&protos.Value_StrVal{"Wellington Public School"}}
	require.Contains(t, nq, makeNquad("_:blank-1", "Name", oval))
}

func TestNquadsFromJson4(t *testing.T) {
	json := `[{"name":"Alice","mobile":"040123456","car":"MA0123"}]`

	nq, err := nquadsFromJson([]byte(json), set)
	require.NoError(t, err)
	require.Equal(t, 3, len(nq))
	oval := &protos.Value{&protos.Value_StrVal{"Alice"}}
	require.Contains(t, nq, makeNquad("_:blank-0", "name", oval))
}

func checkCount(t *testing.T, nq []*protos.NQuad, pred string, count int) {
	for _, n := range nq {
		if n.Predicate == pred {
			require.Equal(t, count, len(n.Facets))
			break
		}
	}
}

func TestNquadsFromJsonFacets1(t *testing.T) {
	json := `[{"name":"Alice","mobile":"040123456","car":"MA0123","mobile@facets":{"since":"2006-01-02T15:04:05Z"},"car@facets":{"first":"true"}}]`

	nq, err := nquadsFromJson([]byte(json), set)
	require.NoError(t, err)
	require.Equal(t, 3, len(nq))
	checkCount(t, nq, "mobile", 1)
	checkCount(t, nq, "car", 1)
}

func TestNquadsFromJsonFacets2(t *testing.T) {
	// Dave has uid facets which should go on the edge between Alice and Dave
	json := `[{"name":"Alice","friend":[{"name":"Dave","@facets":{"close":"true"}}]}]`

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
	require.Contains(t, err.Error(), "uid must be present and non-zero.")
}

func TestNquadsFromJsonList(t *testing.T) {
	json := `{"address":["Riley Street","Redfern"],"phone_number":[123,9876]}`

	nq, err := nquadsFromJson([]byte(json), set)
	require.NoError(t, err)
	require.Equal(t, 4, len(nq))
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
	nqs, err := parseNQuads([]byte(nquads), set)
	require.NoError(t, err)
	require.Equal(t, []*protos.NQuad{
		makeNquad("_:a", "predA", &protos.Value{&protos.Value_DefaultVal{"A"}}),
		makeNquad("_:b", "predB", &protos.Value{&protos.Value_DefaultVal{"B"}}),
		makeNquadEdge("_:a", "join", "_:b"),
	}, nqs)
}

func TestParseNQuadsWindowsNewline(t *testing.T) {
	nquads := "_:a <predA> \"A\" .\r\n_:b <predB> \"B\" ."
	nqs, err := parseNQuads([]byte(nquads), set)
	require.NoError(t, err)
	require.Equal(t, []*protos.NQuad{
		makeNquad("_:a", "predA", &protos.Value{&protos.Value_DefaultVal{"A"}}),
		makeNquad("_:b", "predB", &protos.Value{&protos.Value_DefaultVal{"B"}}),
	}, nqs)
}

func TestParseNQuadsDelete(t *testing.T) {
	nquads := `_:a * * .`
	nqs, err := parseNQuads([]byte(nquads), delete)
	require.NoError(t, err)
	require.Equal(t, []*protos.NQuad{
		makeNquad("_:a", x.Star, &protos.Value{&protos.Value_DefaultVal{x.Star}}),
	}, nqs)
}
