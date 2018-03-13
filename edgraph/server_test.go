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
	tn := time.Now()
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

	oval := &api.Value{&api.Value_StrVal{"Alice"}}
	require.Contains(t, nq, makeNquad("_:blank-0", "name", oval))

	oval = &api.Value{&api.Value_DoubleVal{26}}
	require.Contains(t, nq, makeNquad("_:blank-0", "age", oval))

	oval = &api.Value{&api.Value_BoolVal{true}}
	require.Contains(t, nq, makeNquad("_:blank-0", "married", oval))

	oval = &api.Value{&api.Value_StrVal{tn.Format(time.RFC3339Nano)}}
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

	oval := &api.Value{&api.Value_StrVal{"Charlie"}}
	require.Contains(t, nq, makeNquad("_:blank-1", "name", oval))

	oval = &api.Value{&api.Value_BoolVal{false}}
	require.Contains(t, nq, makeNquad("_:blank-1", "married", oval))

	oval = &api.Value{&api.Value_StrVal{"Bob"}}
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

	oval := &api.Value{&api.Value_StrVal{"Wellington Public School"}}
	require.Contains(t, nq, makeNquad("_:blank-1", "Name", oval))
}

func TestNquadsFromJson4(t *testing.T) {
	json := `[{"name":"Alice","mobile":"040123456","car":"MA0123"}]`

	nq, err := nquadsFromJson([]byte(json), set)
	require.NoError(t, err)
	require.Equal(t, 3, len(nq))
	oval := &api.Value{&api.Value_StrVal{"Alice"}}
	require.Contains(t, nq, makeNquad("_:blank-0", "name", oval))
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
	oval := &api.Value{&api.Value_StrVal{"Name"}}
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
	nqs, err := parseNQuads([]byte(nquads))
	require.NoError(t, err)
	require.Equal(t, []*api.NQuad{
		makeNquad("_:a", "predA", &api.Value{&api.Value_DefaultVal{"A"}}),
		makeNquad("_:b", "predB", &api.Value{&api.Value_DefaultVal{"B"}}),
		makeNquadEdge("_:a", "join", "_:b"),
	}, nqs)
}

func TestParseNQuadsWindowsNewline(t *testing.T) {
	nquads := "_:a <predA> \"A\" .\r\n_:b <predB> \"B\" ."
	nqs, err := parseNQuads([]byte(nquads))
	require.NoError(t, err)
	require.Equal(t, []*api.NQuad{
		makeNquad("_:a", "predA", &api.Value{&api.Value_DefaultVal{"A"}}),
		makeNquad("_:b", "predB", &api.Value{&api.Value_DefaultVal{"B"}}),
	}, nqs)
}

func TestParseNQuadsDelete(t *testing.T) {
	nquads := `_:a * * .`
	nqs, err := parseNQuads([]byte(nquads))
	require.NoError(t, err)
	require.Equal(t, []*api.NQuad{
		makeNquad("_:a", x.Star, &api.Value{&api.Value_DefaultVal{x.Star}}),
	}, nqs)
}
