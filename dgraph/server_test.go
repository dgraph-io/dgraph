package dgraph

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	geom "github.com/twpayne/go-geom"
)

func TestNquadsFromJson(t *testing.T) {
	type Sport struct {
		Name string
	}

	type School struct {
		Name    string
		Address string
		Sports  []Sport
	}

	type Person struct {
		Name    string    `json:"name"`
		Age     int       `json:"age"`
		Married bool      `json:"married"`
		Now     time.Time `json:"now"`
		Address geom.T    `json:"address"`
		Friends []Person  `json:"friend"`
		School  School    `json:"school"`
	}

	p := Person{
		Name:    "Alice",
		Age:     26,
		Married: false,
		Now:     time.Now(),
		Friends: []Person{{
			Name: "Jan",
			Age:  24,
		}},
		Address: geom.NewPoint(geom.XY).MustSetCoords(geom.Coord{-122.082506, 37.4249518}),
		School: School{
			Name:    "Wellington Public School",
			Address: "Crown Street",
			Sports: []Sport{
				{"Cricket"},
			},
		},
	}

	b, err := json.Marshal(p)
	require.NoError(t, err)
	nq, err := nquadsFromJson(b)
	require.NoError(t, err)
	for _, n := range nq {
		fmt.Println(n)
	}
}
