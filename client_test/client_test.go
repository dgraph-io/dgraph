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

package client_test

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/dgraph-io/dgraph/client"
	"github.com/dgraph-io/dgraph/dgraph"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/x"
	"github.com/stretchr/testify/require"
)

func prepare() (res []string, options dgraph.Options) {
	clientDir, err := ioutil.TempDir("", "client_")
	x.Check(err)
	res = append(res, clientDir)

	options = dgraph.GetDefaultEmbeddedConfig()
	postingDir, err := ioutil.TempDir("", "p")
	x.Check(err)
	res = append(res, postingDir)

	options.PostingDir = postingDir
	walDir, err := ioutil.TempDir("", "w")
	x.Check(err)
	res = append(res, walDir)
	options.WALDir = walDir
	options.AllottedMemory = 2048

	return res, options
}

func removeDirs(dirs []string) {
	for _, dir := range dirs {
		os.RemoveAll(dir)
	}
}

var dgraphClient *client.Dgraph

func TestMain(m *testing.M) {
	x.Init()
	x.Logger = log.New(ioutil.Discard, "", 0)
	dirs, options := prepare()
	defer removeDirs(dirs)
	dgraphClient = dgraph.NewEmbeddedDgraphClient(options, client.DefaultOptions, dirs[0])
	r := m.Run()
	dgraph.DisposeEmbeddedDgraph()
	os.Exit(r)
}

type Person struct {
	Name    string   `json:"name"`
	Loc     string   `json:"loc"`
	FallsIn string   `json:"falls.in"`
	Friends []Person `json:"friend"`
}

type Res struct {
	Root []Person `json:"me"`
}

func TestClientDelete(t *testing.T) {
	req := client.Req{}
	alice, err := dgraphClient.NodeBlank("")
	require.NoError(t, err)
	bob, err := dgraphClient.NodeBlank("")
	require.NoError(t, err)

	e := alice.Edge("name")
	require.NoError(t, e.SetValueString("Alice"))
	require.NoError(t, req.Set(e))
	e = bob.Edge("name")
	require.NoError(t, e.SetValueString("Bob"))
	require.NoError(t, req.Set(e))
	e = alice.Edge("falls.in")
	require.NoError(t, e.SetValueString("Rabbit Hole"))
	require.NoError(t, req.Set(e))
	e = alice.ConnectTo("friend", bob)
	require.NoError(t, req.Set(e))
	e = alice.Edge("loc")
	loc := `{"type":"Point","coordinates":[1.1,2]}`
	require.NoError(t, e.SetValueGeoJson(loc))
	require.NoError(t, req.Set(e))
	aliceQuery := fmt.Sprintf(`{
		me(func: uid(%s)) {
			name
			loc
			falls.in
			friend {
				name
			}
		}
	}`, alice)
	req.SetQuery(aliceQuery)
	resp, err := dgraphClient.Run(context.Background(), &req)
	x.Check(err)

	var r Res
	err = client.Unmarshal(resp.N, &r)
	require.Equal(t, 1, len(r.Root))
	require.Equal(t, "Alice", r.Root[0].Name)
	require.Equal(t, loc, r.Root[0].Loc)
	require.Equal(t, "Rabbit Hole", r.Root[0].FallsIn)
	require.Equal(t, 1, len(r.Root[0].Friends))
	require.Equal(t, "Bob", r.Root[0].Friends[0].Name)

	// Lets test Edge delete
	req = client.Req{}
	r = Res{}
	e = alice.Edge("name")
	// S P * deletion for name.
	x.Check(e.Delete())
	x.Check(req.Delete(e))
	// S P * deletion for friend.
	e = alice.Edge("friend")
	x.Check(e.Delete())
	x.Check(req.Delete(e))
	req.SetQuery(aliceQuery)
	resp, err = dgraphClient.Run(context.Background(), &req)
	x.Check(err)
	err = client.Unmarshal(resp.N, &r)
	require.Equal(t, 1, len(r.Root))
	require.Equal(t, "", r.Root[0].Name)
	require.Equal(t, "Rabbit Hole", r.Root[0].FallsIn)
	require.Equal(t, 0, len(r.Root[0].Friends))

	// Lets test Node delete now.
	req = client.Req{}
	r = Res{}
	e = alice.Delete()
	x.Check(e.Delete())
	x.Check(req.Delete(e))
	req.SetQuery(aliceQuery)
	resp, err = dgraphClient.Run(context.Background(), &req)
	x.Check(err)
	err = client.Unmarshal(resp.N, &r)
	require.Equal(t, 0, len(r.Root))
}

func TestClientAddFacets(t *testing.T) {
	req := client.Req{}
	alice, err := dgraphClient.NodeBlank("")
	require.NoError(t, err)

	e := alice.Edge("name")
	require.NoError(t, e.SetValueString("Alice"))
	e.AddFacet("xyz", "2")
	e.AddFacet("abc", "1")
	require.NoError(t, req.Set(e))
	aliceQuery := fmt.Sprintf(`{
		me(func: uid(%s)) {
			name @facets(abc)
		}
	}`, alice)
	req.SetQuery(aliceQuery)
	resp, err := dgraphClient.Run(context.Background(), &req)
	require.NoError(t, err)

	require.Equal(t, "abc", resp.N[0].Children[0].Children[0].Children[0].Properties[0].Prop)
	require.Equal(t, int64(1), resp.N[0].Children[0].Children[0].Children[0].Properties[0].Value.GetIntVal())
}

func TestClientAddFacetsError(t *testing.T) {
	req := client.Req{}
	alice, err := dgraphClient.NodeBlank("")
	require.NoError(t, err)

	e := alice.Edge("name")
	require.NoError(t, e.SetValueString("Alice"))
	e.AddFacet("abc", "2")
	e.AddFacet("abc", "1")
	require.NoError(t, req.Set(e))
	aliceQuery := fmt.Sprintf(`{
		me(func: uid(%s)) {
			name @facets(abc)
		}
	}`, alice)
	req.SetQuery(aliceQuery)
	_, err = dgraphClient.Run(context.Background(), &req)
	require.Error(t, err)
}

func TestClientDeletePredicate(t *testing.T) {
	req := client.Req{}
	alice, err := dgraphClient.NodeBlank("")
	require.NoError(t, err)
	bob, err := dgraphClient.NodeBlank("")
	require.NoError(t, err)
	charlie, err := dgraphClient.NodeBlank("")
	require.NoError(t, err)

	e := alice.Edge("name")
	require.NoError(t, e.SetValueString("Alice"))
	require.NoError(t, req.Set(e))

	e = bob.Edge("name")
	require.NoError(t, e.SetValueString("Bob"))
	require.NoError(t, req.Set(e))

	e = charlie.Edge("name")
	require.NoError(t, e.SetValueString("Charlie"))
	require.NoError(t, req.Set(e))

	e = alice.ConnectTo("friend", bob)
	require.NoError(t, req.Set(e))
	e = alice.ConnectTo("friend", charlie)
	require.NoError(t, req.Set(e))
	aliceQuery := fmt.Sprintf(`{
		me(func: uid(%s)) {
			name
			falls.in
			friend {
				name
			}
		}
	}`, alice)
	req.SetQuery(aliceQuery)
	resp, err := dgraphClient.Run(context.Background(), &req)
	x.Check(err)

	var r Res
	err = client.Unmarshal(resp.N, &r)
	require.Equal(t, 1, len(r.Root))
	require.Equal(t, 2, len(r.Root[0].Friends))

	req = client.Req{}
	req.SetQuery(aliceQuery)
	req.Delete(client.DeletePredicate("friend"))
	resp, err = dgraphClient.Run(context.Background(), &req)
	x.Check(err)
	r = Res{}
	err = client.Unmarshal(resp.N, &r)
	require.Equal(t, 1, len(r.Root))
	require.Equal(t, 0, len(r.Root[0].Friends))
}

func TestLangTag(t *testing.T) {
	req := client.Req{}
	alice, err := dgraphClient.NodeBlank("")
	require.NoError(t, err)

	e := alice.Edge("name")
	require.NoError(t, e.SetValueString("Alice"))
	require.NoError(t, req.Set(e))
	aliceQuery := fmt.Sprintf(`{
		me(func: uid(%s)) {
			name
		}
	}`, alice)
	req.SetQuery(aliceQuery)
	resp, err := dgraphClient.Run(context.Background(), &req)
	x.Check(err)

	var r Res
	err = client.Unmarshal(resp.N, &r)
	require.Equal(t, 1, len(r.Root))
	require.Equal(t, "Alice", r.Root[0].Name)

	type Person struct {
		Name    string   `json:"name@ru"`
		FallsIn string   `json:"falls.in"`
		Friends []Person `json:"friend"`
	}

	type Res struct {
		Root []Person `json:"me"`
	}

	var r2 Res

	req = client.Req{}
	e = alice.Edge("name")
	require.NoError(t, e.SetValueStringWithLang("Алиса", "ru"))
	require.NoError(t, req.Set(e))
	aliceQuery = fmt.Sprintf(`{
		me(func: uid(%s)) {
			name@ru
		}
	}`, alice)
	req.SetQuery(aliceQuery)
	resp, err = dgraphClient.Run(context.Background(), &req)
	x.Check(err)
	err = client.Unmarshal(resp.N, &r2)
	require.Equal(t, 1, len(r2.Root))
	require.Equal(t, "Алиса", r2.Root[0].Name)
}

func TestSchemaError(t *testing.T) {
	req := client.Req{}
	err := req.AddSchema(protos.SchemaUpdate{
		Predicate: "dummy",
		Tokenizer: []string{"term"},
	})
	require.NoError(t, err)
	_, err = dgraphClient.Run(context.Background(), &req)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Directive must be SchemaUpdate_INDEX when a tokenizer is specified")

	req = client.Req{}
	err = req.AddSchema(protos.SchemaUpdate{
		Predicate: "dummy",
		Directive: protos.SchemaUpdate_INDEX,
	})
	require.NoError(t, err)
	_, err = dgraphClient.Run(context.Background(), &req)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Tokenizer must be specified while indexing a predicate")

}

func TestEmptyString(t *testing.T) {
	req := client.Req{}
	alice, err := dgraphClient.NodeBlank("")
	require.NoError(t, err)
	e := alice.Edge("name")
	require.NoError(t, e.SetValueString(""))
	require.NoError(t, req.Set(e))
	_, err = dgraphClient.Run(context.Background(), &req)
	require.NoError(t, err)
}

func TestSetObject(t *testing.T) {
	type School struct {
		Name string `json:"name@en,omitempty"`
	}

	type Person struct {
		Uid      uint64   `json:"_uid_,omitempty"`
		Name     string   `json:"name,omitempty"`
		Age      int      `json:"age,omitempty"`
		Married  bool     `json:"married,omitempty"`
		Raw      []byte   `json:"raw_bytes",omitempty`
		Friends  []Person `json:"friend,omitempty"`
		Location string   `json:"loc,omitempty"`
		School   *School  `json:"school,omitempty"`
	}

	req := client.Req{}

	loc := `{"type":"Point","coordinates":[1.1,2]}`
	p := Person{
		Name:     "Alice",
		Age:      26,
		Married:  true,
		Location: loc,
		Raw:      []byte("raw_bytes"),
		Friends: []Person{{
			Uid:  1000,
			Name: "Bob",
			Age:  24,
		}, {
			Name: "Charlie",
			Age:  29,
		}},
		School: &School{
			Name: "Crown Public School",
		},
	}

	req.SetSchema(`
		age: int .
		married: bool .
	`)

	err := req.SetObject(&p)
	require.NoError(t, err)

	resp, err := dgraphClient.Run(context.Background(), &req)
	require.NoError(t, err)

	puid := resp.AssignedUids["blank-0"]
	q := fmt.Sprintf(`{
		me(func: uid(%d)) {
			name
			age
			loc
			raw_bytes
			married
			friend {
				_uid_
				name
				age
			}
			school {
				name@en
			}
		}
	}`, puid)

	req = client.Req{}
	req.SetQuery(q)
	resp, err = dgraphClient.Run(context.Background(), &req)
	require.NoError(t, err)

	type Root struct {
		Me Person `json:"me"`
	}

	var r Root
	require.NoError(t, client.Unmarshal(resp.N, &r))

	p2 := r.Me
	require.Equal(t, p.Location, p2.Location)
	require.Equal(t, p.Name, p2.Name)
	require.Equal(t, p.Age, p2.Age)
	require.Equal(t, p.Married, p2.Married)
	require.Equal(t, p.Raw, p2.Raw)
	require.Equal(t, p.School.Name, p2.School.Name)
	require.Equal(t, len(p.Friends), len(p2.Friends))
	require.NotNil(t, p2.Friends[0].Name)
	require.NotNil(t, p2.Friends[1].Name)
	require.NotNil(t, p2.Friends[1].Age)
}

func TestSetObject2(t *testing.T) {
	req := client.Req{}

	type School struct {
		Uid  uint64
		Name string `json:"@,omitempty"`
	}

	type Person struct {
		Uid      uint64   `json:"_uid_,omitempty"`
		Name     string   `json:"name,omitempty"`
		Age      int      `json:"age,omitempty"`
		Married  bool     `json:"married,omitempty"`
		Friends  []Person `json:"friend,omitempty"`
		Location string   `json:"loc,omitempty"`
		School   *School  `json:"school,omitempty"`
	}

	loc := `{"type":"Point","coordinates":[1.1,2]}`
	p := Person{
		Name:     "Alice",
		Age:      26,
		Married:  true,
		Location: loc,
		Friends: []Person{{
			Uid:  1000,
			Name: "Bob",
			Age:  24,
		}, {
			Uid:  1001,
			Name: "Charlie",
			Age:  29,
		}},
		School: &School{
			Uid:  1002,
			Name: "Crown Public School",
		},
	}

	err := req.SetObject(&p)
	require.NoError(t, err)
	_, err = dgraphClient.Run(context.Background(), &req)
	require.NoError(t, err)
}

func TestDeleteObject1(t *testing.T) {
	// In this test we check S P O deletion.
	type School struct {
		Uid  uint64 `json:"_uid_"`
		Name string `json:"name@en,omitempty"`
	}

	type Person struct {
		Uid      uint64   `json:"_uid_,omitempty"`
		Name     string   `json:"name,omitempty"`
		Age      int      `json:"age,omitempty"`
		Married  bool     `json:"married,omitempty"`
		Friends  []Person `json:"friend,omitempty"`
		Location string   `json:"loc,omitempty"`
		School   *School  `json:"school,omitempty"`
	}

	req := client.Req{}

	loc := `{"type":"Point","coordinates":[1.1,2]}`
	p := Person{
		Uid:      1000,
		Name:     "Alice",
		Age:      26,
		Married:  true,
		Location: loc,
		Friends: []Person{{
			Uid:  1001,
			Name: "Bob",
			Age:  24,
		}, {
			Uid:  1002,
			Name: "Charlie",
			Age:  29,
		}},
		School: &School{
			Uid:  1003,
			Name: "Crown Public School",
		},
	}

	req.SetSchema(`
		age: int .
		married: bool .
	`)

	err := req.SetObject(&p)
	require.NoError(t, err)

	q := fmt.Sprintf(`{
		me(func: uid(1000)) {
			name
			age
			loc
			married
			friend {
				_uid_
				name
				age
			}
			school {
				_uid_
				name@en
			}
		}

		me2(func: uid(1001)) {
			name
			age
		}

		me3(func: uid(1003)) {
			name@en
		}

		me4(func: uid(1002)) {
			name
			age
		}
	}`)
	resp, err := dgraphClient.Run(context.Background(), &req)
	require.NoError(t, err)

	req = client.Req{}
	// Delete Charlie from friends so that he is not deleted.
	p.Friends = p.Friends[:1]
	err = req.DeleteObject(&p)
	require.NoError(t, err)

	req.SetQuery(q)
	resp, err = dgraphClient.Run(context.Background(), &req)
	require.NoError(t, err)

	type Root struct {
		Me Person `json:"me"`
	}

	var r Root
	require.NoError(t, client.Unmarshal(resp.N, &r))
	require.Equal(t, 1, len(r.Me.Friends))

	for i := 1; i < len(resp.N)-1; i++ {
		n := resp.N[i]
		require.Equal(t, 0, len(n.Children))
		require.Equal(t, 0, len(n.Properties))
	}

	require.Equal(t, 2, len(resp.N[3].Children[0].Properties))
}

func TestDeleteObject2(t *testing.T) {
	// In this test we check S P * deletion.
	type School struct {
		Uid  uint64 `json:"_uid_"`
		Name string `json:"name@en,omitempty"`
	}

	type Person struct {
		Uid      uint64   `json:"_uid_,omitempty"`
		Name     *string  `json:"name"`
		Age      int      `json:"age,omitempty"`
		Married  bool     `json:"married,omitempty"`
		Friends  []Person `json:"friend"`
		Location string   `json:"loc,omitempty"`
		School   *School  `json:"school"`
	}

	req := client.Req{}

	loc := `{"type":"Point","coordinates":[1.1,2]}`
	alice, bob, charlie := "Alice", "Bob", "Charlie"
	p := Person{
		Uid:      1000,
		Name:     &alice,
		Age:      26,
		Married:  true,
		Location: loc,
		Friends: []Person{{
			Uid:  1001,
			Name: &bob,
			Age:  24,
		}, {
			Uid:  1002,
			Name: &charlie,
			Age:  29,
		}},
		School: &School{
			Uid:  1003,
			Name: "Crown Public School",
		},
	}

	req.SetSchema(`
		age: int .
		married: bool .
	`)

	err := req.SetObject(&p)
	require.NoError(t, err)

	q := fmt.Sprintf(`{
		me(func: uid(1000)) {
			name
			age
			loc
			married
			friend {
				_uid_
				name
				age
			}
			school {
				_uid_
				name@en
			}
		}

		me2(func: uid(1001)) {
			name
			age
		}

		me3(func: uid(1003)) {
			name@en
		}
	}`)

	resp, err := dgraphClient.Run(context.Background(), &req)
	require.NoError(t, err)

	n := ""
	// Now persons name, friends and school should be deleted but not location.
	p2 := Person{
		Uid:     1000,
		Name:    &n,
		Friends: nil,
		School:  nil,
	}

	req = client.Req{}
	err = req.DeleteObject(&p2)
	require.NoError(t, err)

	req.SetQuery(q)
	resp, err = dgraphClient.Run(context.Background(), &req)
	require.NoError(t, err)

	type Root struct {
		Me Person `json:"me"`
	}

	var r Root
	require.NoError(t, client.Unmarshal(resp.N, &r))

	p3 := r.Me
	require.Equal(t, p.Location, p3.Location)
	require.Nil(t, p3.Name)
	require.Equal(t, p3.Age, p.Age)
	require.Equal(t, p3.Married, p.Married)
	require.Nil(t, p3.School)
	require.Equal(t, 0, len(p3.Friends))
}

func TestSetObjectWithFacets(t *testing.T) {
	req := client.Req{}

	type friendFacet struct {
		Since  time.Time `json:"since"`
		Family string    `json:"family"`
		Age    float64   `json:"age"`
		Close  bool      `json:"close"`
	}

	type nameFacets struct {
		Origin string `json:"origin"`
	}

	type schoolFacet struct {
		Since time.Time `json:"since"`
	}

	type School struct {
		Name   string      `json:"name"`
		Facets schoolFacet `json:"@facets"`
	}

	type Person struct {
		Name       string      `json:"name"`
		NameFacets nameFacets  `json:"name@facets"`
		Facets     friendFacet `json:"@facets"`
		Friends    []Person    `json:"friend"`
		School     School      `json:"school"`
	}

	ti := time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)
	p := Person{
		Name: "Alice",
		NameFacets: nameFacets{
			Origin: "Indonesia",
		},
		Friends: []Person{
			Person{
				Name: "Bob",
				Facets: friendFacet{
					Since:  ti,
					Family: "yes",
					Age:    13,
					Close:  true,
				},
			},
			Person{
				Name: "Charlie",
				Facets: friendFacet{
					Family: "maybe",
					Age:    16,
				},
			},
		},
		School: School{
			Name: "Wellington School",
			Facets: schoolFacet{
				Since: ti,
			},
		},
	}

	err := req.SetObject(&p)
	require.NoError(t, err)
	resp, err := dgraphClient.Run(context.Background(), &req)
	require.NoError(t, err)

	auid := resp.AssignedUids["blank-0"]

	q := fmt.Sprintf(`
	{

		me(func: uid(%v)) {
			name @facets
			friend @facets {
				name
			}
			school @facets {
				name
			}

		}
	}`, auid)

	req.SetQuery(q)
	resp, err = dgraphClient.Run(context.Background(), &req)
	require.NoError(t, err)
	type Root struct {
		Me Person `json:"me"`
	}

	var r Root
	require.NoError(t, client.Unmarshal(resp.N, &r))
	// Compare the objects.
	sort.Slice(r.Me.Friends, func(i, j int) bool {
		return r.Me.Friends[i].Name < r.Me.Friends[j].Name
	})
	require.EqualValues(t, p, r.Me)
}

func TestSetObjectUpdateFacets(t *testing.T) {
	req := client.Req{}

	type friendFacet struct {
		// All facets should be marshalled to a string. Server interprets the type from the value.
		Since  time.Time `json:"since, omitempty"`
		Family string    `json:"family"`
		Age    float64   `json:"age"`
		Close  bool      `json:"close"`
	}

	type nameFacets struct {
		Origin string `json:"origin"`
	}

	type Person struct {
		Uid        uint64      `json:"_uid_"`
		Name       string      `json:"name"`
		NameFacets nameFacets  `json:"name@facets"`
		Facets     friendFacet `json:"@facets"`
		Friends    []Person    `json:"friend"`
	}

	ti := time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)
	p := Person{
		Uid:  1001,
		Name: "Alice",
		NameFacets: nameFacets{
			Origin: "Indonesia",
		},
		Friends: []Person{
			Person{
				Uid:  1002,
				Name: "Bob",
				Facets: friendFacet{
					Since:  ti,
					Family: "yes",
					Age:    13,
					Close:  true,
				},
			},
			Person{
				Uid:  1003,
				Name: "Charlie",
				Facets: friendFacet{
					Family: "maybe",
					Age:    16,
				},
			},
		},
	}

	err := req.SetObject(&p)
	require.NoError(t, err)
	_, err = dgraphClient.Run(context.Background(), &req)
	require.NoError(t, err)

	// Now update some facets and do a set again.
	p.NameFacets.Origin = "Jakarta"
	p.Friends[0].Facets.Family = "No"
	p.Friends[0].Facets.Age = 14
	p.Friends[1].Facets = friendFacet{}

	err = req.SetObject(&p)
	require.NoError(t, err)
	_, err = dgraphClient.Run(context.Background(), &req)
	require.NoError(t, err)

	q := `
	{

		me(func: uid(1001)) {
			_uid_
			name @facets
			friend @facets {
				_uid_
				name
			}

		}
	}`

	req.SetQuery(q)
	resp, err := dgraphClient.Run(context.Background(), &req)
	require.NoError(t, err)
	type Root struct {
		Me Person `json:"me"`
	}

	var r Root
	require.NoError(t, client.Unmarshal(resp.N, &r))
	// Compare the objects.
	require.EqualValues(t, p, r.Me)
}

func TestDeleteObjectNode(t *testing.T) {
	// In this test we check S * * deletion.
	type Person struct {
		Uid     uint64    `json:"_uid_,omitempty"`
		Name    string    `json:"name,omitempty"`
		Age     int       `json:"age,omitempty"`
		Married bool      `json:"married,omitempty"`
		Friends []*Person `json:"friend,omitempty"`
	}

	req := client.Req{}

	p := Person{
		Uid:     1000,
		Name:    "Alice",
		Age:     26,
		Married: true,
		Friends: []*Person{&Person{
			Uid:  1001,
			Name: "Bob",
			Age:  24,
		}, &Person{
			Uid:  1002,
			Name: "Charlie",
			Age:  29,
		}},
	}

	req.SetSchema(`
		age: int .
		married: bool .
	`)

	err := req.SetObject(&p)
	require.NoError(t, err)

	q := fmt.Sprintf(`{
		me(func: uid(1000)) {
			name
			age
			married
			friend {
				_uid_
				name
				age
			}
		}

		me2(func: uid(1001)) {
			name
			age
		}

		me3(func: uid(1002)) {
			name
			age
		}
	}`)
	req.SetQuery(q)

	resp, err := dgraphClient.Run(context.Background(), &req)
	require.NoError(t, err)
	first := resp.N[0].Children[0]
	require.Equal(t, 2, len(first.Children))
	require.Equal(t, 3, len(first.Properties))

	// Now lets try to delete Alice. This won't delete Bob and Charlie but just remove the
	// connection between Alice and them.
	p2 := Person{
		Uid: 1000,
	}

	req = client.Req{}
	err = req.DeleteObject(&p2)
	require.NoError(t, err)

	req.SetQuery(q)
	resp, err = dgraphClient.Run(context.Background(), &req)
	require.NoError(t, err)
	require.Equal(t, 0, len(resp.N[0].Children))
	second := resp.N[1].Children[0]
	require.Equal(t, 2, len(second.Properties))
	third := resp.N[2].Children[0]
	require.Equal(t, 2, len(third.Properties))
}

func TestDeleteObjectPredicate(t *testing.T) {
	// In this test we check * P * deletion.
	type Person struct {
		Uid     uint64    `json:"_uid_,omitempty"`
		Name    string    `json:"name,omitempty"`
		Age     int       `json:"age,omitempty"`
		Married bool      `json:"married,omitempty"`
		Friends []*Person `json:"friend,omitempty"`
	}

	req := client.Req{}

	p := Person{
		Uid:     1000,
		Name:    "Alice",
		Age:     26,
		Married: true,
		Friends: []*Person{&Person{
			Uid:  1001,
			Name: "Bob",
			Age:  24,
		}, &Person{
			Uid:  1002,
			Name: "Charlie",
			Age:  29,
		}},
	}

	req.SetSchema(`
		age: int .
		married: bool .
	`)

	err := req.SetObject(&p)
	require.NoError(t, err)

	q := fmt.Sprintf(`{
		me(func: uid(1000)) {
			name
			age
			married
			friend {
				_uid_
				name
				age
			}
		}

		me2(func: uid(1001)) {
			name
			age
		}

		me3(func: uid(1002)) {
			name
			age
		}
	}`)
	req.SetQuery(q)

	resp, err := dgraphClient.Run(context.Background(), &req)
	require.NoError(t, err)
	first := resp.N[0].Children[0]
	require.Equal(t, 2, len(first.Children))
	require.Equal(t, 3, len(first.Properties))

	// Now lets try to delete friend and married predicate.
	type DeletePred struct {
		Friend  interface{} `json:"friend"`
		Married interface{} `json:"married"`
	}
	dp := DeletePred{}
	// Basically we want predicate as JSON keys with value null.
	// After marshalling this would become { "friend" : null, "married": null }

	req = client.Req{}
	err = req.DeleteObject(&dp)
	require.NoError(t, err)

	req.SetQuery(q)
	resp, err = dgraphClient.Run(context.Background(), &req)
	require.NoError(t, err)

	first = resp.N[0].Children[0]
	// Alice should have no friends and only two attributes now.
	require.Equal(t, 0, len(first.Children))
	require.Equal(t, 2, len(first.Properties))
	second := resp.N[1].Children[0]
	require.Equal(t, 2, len(second.Properties))
	third := resp.N[2].Children[0]
	require.Equal(t, 2, len(third.Properties))
}

func TestObjectList(t *testing.T) {
	req := client.Req{}

	type Person struct {
		Uid         uint64   `json:"_uid_"`
		Address     []string `json:"address"`
		PhoneNumber []int64  `json:"phone_number"`
	}

	p := Person{
		Address:     []string{"Redfern", "Riley Street"},
		PhoneNumber: []int64{9876, 123},
	}

	req.SetSchema(`
		address: [string] .
		phone_number: [int] .
	`)

	err := req.SetObject(&p)
	require.NoError(t, err)
	resp, err := dgraphClient.Run(context.Background(), &req)
	require.NoError(t, err)

	uid := resp.AssignedUids["blank-0"]

	q := fmt.Sprintf(`
	{
		me(func: uid(%d)) {
			_uid_
			address
			phone_number
		}
	}
	`, uid)

	req.SetQuery(q)
	resp, err = dgraphClient.Run(context.Background(), &req)
	require.NoError(t, err)

	type Root struct {
		Me Person `json:"me"`
	}

	var r Root
	require.NoError(t, client.Unmarshal(resp.N, &r))

	require.Equal(t, 2, len(r.Me.Address))
	require.Equal(t, 2, len(r.Me.PhoneNumber))
	sort.Strings(r.Me.Address)
	require.Equal(t, p.Address, r.Me.Address)
	require.Equal(t, p.PhoneNumber, r.Me.PhoneNumber)

	// Now add some more values and do the query again.
	p2 := Person{
		Uid:         uid,
		Address:     []string{"Surry Hills"},
		PhoneNumber: []int64{1234},
	}
	err = req.SetObject(&p2)
	require.NoError(t, err)
	req.SetQuery(q)
	resp, err = dgraphClient.Run(context.Background(), &req)
	require.NoError(t, err)

	require.NoError(t, client.Unmarshal(resp.N, &r))
	require.Equal(t, 3, len(r.Me.Address))
	require.Equal(t, 3, len(r.Me.PhoneNumber))

	// Now lets delete 2 values.
	p3 := Person{
		Uid:         uid,
		Address:     p.Address,
		PhoneNumber: p.PhoneNumber,
	}

	err = req.DeleteObject(&p3)
	require.NoError(t, err)
	req.SetQuery(q)
	resp, err = dgraphClient.Run(context.Background(), &req)
	require.NoError(t, err)

	require.NoError(t, client.Unmarshal(resp.N, &r))
	require.Equal(t, p2, r.Me)
}

func TestInitialSchema(t *testing.T) {
	type Person struct {
		Name string `json:"name,omitempty"`
		Age  int    `json:"age,omitempty"`
	}

	req := client.Req{}

	p := Person{
		Name: "Alice",
		Age:  26,
	}

	req.SetSchema(`
		age: int .
	`)

	err := req.SetObject(&p)
	require.NoError(t, err)

	resp, err := dgraphClient.Run(context.Background(), &req)
	require.NoError(t, err)

	puid := resp.AssignedUids["blank-0"]
	q := fmt.Sprintf(`{
		me(func: uid(%d)) {
			_predicate_
		}
	}`, puid)

	req.SetQuery(q)
	resp, err = dgraphClient.Run(context.Background(), &req)
	require.NoError(t, err)

	r := resp.N[0].Children[0]
	require.Equal(t, 2, len(r.Properties))

	req.SetQuery(`schema{}`)
	resp, err = dgraphClient.Run(context.Background(), &req)
	require.NoError(t, err)
	require.Equal(t, 3, len(resp.Schema))
	for _, s := range resp.Schema {
		if s.Predicate == x.PredicateListAttr {
			require.Equal(t, &protos.SchemaNode{
				Predicate: x.PredicateListAttr,
				List:      true,
				Type:      "string",
			}, s)
		}
	}
}
