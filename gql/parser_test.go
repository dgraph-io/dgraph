/*
 * Copyright 2015 DGraph Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * 		http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package gql

import (
	"fmt"
	"strings"
	"testing"
)

func checkAttr(g *GraphQuery, attr string) error {
	if g.Attr != attr {
		return fmt.Errorf("Expected: %v. Got: %v", attr, g.Attr)
	}
	return nil
}

func TestParse(t *testing.T) {
	query := `
	query {
		me(_uid_:0x0a) {
			friends {
				name
			}
			gender,age
			hometown
		}
	}
`

	gq, _, err := Parse(query)
	if err != nil {
		t.Error(err)
	}
	if gq == nil {
		t.Error("subgraph is nil")
		return
	}
	if len(gq.Children) != 4 {
		t.Errorf("Expected 4 children. Got: %v", len(gq.Children))
		return
	}
	if err := checkAttr(gq.Children[0], "friends"); err != nil {
		t.Error(err)
	}
	if err := checkAttr(gq.Children[1], "gender"); err != nil {
		t.Error(err)
	}
	if err := checkAttr(gq.Children[2], "age"); err != nil {
		t.Error(err)
	}
	if err := checkAttr(gq.Children[3], "hometown"); err != nil {
		t.Error(err)
	}
	child := gq.Children[0]
	if len(child.Children) != 1 {
		t.Errorf("Expected 1 child of friends. Got: %v", len(child.Children))
	}
	if err := checkAttr(child.Children[0], "name"); err != nil {
		t.Error(err)
	}
}

func TestParseXid(t *testing.T) {
	// logrus.SetLevel(logrus.DebugLevel)
	// TODO: Why does the query not have _xid_ attribute?
	query := `
	query {
		user(_uid_: 0x11) {
			type.object.name
		}
	}`
	gq, _, err := Parse(query)
	if err != nil {
		t.Error(err)
		return
	}
	if gq == nil {
		t.Error("subgraph is nil")
		return
	}
	if len(gq.Children) != 1 {
		t.Errorf("Expected 1 children. Got: %v", len(gq.Children))
	}
	if err := checkAttr(gq.Children[0], "type.object.name"); err != nil {
		t.Error(err)
	}
}

func TestParseFirst(t *testing.T) {
	query := `
	query {
		user(_xid_: m.abcd) {
			type.object.name
			friends (first: 10) {
			}
		}
	}`
	gq, _, err := Parse(query)
	if err != nil {
		t.Error(err)
		return
	}
	if gq == nil {
		t.Error("subgraph is nil")
		return
	}
	if len(gq.Children) != 2 {
		t.Errorf("Expected 2 children. Got: %v", len(gq.Children))
	}
	if err := checkAttr(gq.Children[0], "type.object.name"); err != nil {
		t.Error(err)
	}
	if gq.Children[0].First != 0 {
		t.Errorf("Expected count 0. Got: %v", gq.Children[0].First)
	}
	if err := checkAttr(gq.Children[1], "friends"); err != nil {
		t.Error(err)
	}
	if gq.Children[1].First != 10 {
		t.Errorf("Expected count 10. Got: %v", gq.Children[1].First)
	}
}

func TestParseFirst_error(t *testing.T) {
	query := `
	query {
		user(_xid_: m.abcd) {
			type.object.name
			friends (first: ) {
			}
		}
	}`
	var err error
	_, _, err = Parse(query)
	t.Log(err)
	if err == nil {
		t.Error("Expected error")
	}
}

func TestParse_error2(t *testing.T) {
	query := `
		query {
			me {
				name
			}
		}
	`
	var err error
	_, _, err = Parse(query)
	t.Log(err)
	if err == nil {
		t.Error("Expected error")
	}
}

func TestParse_pass1(t *testing.T) {
	query := `
		{
			me(_uid_:0x0a) {
				name,
				friends(xid:what) {  # xid would be ignored.
				}
			}
		}
	`
	gq, _, err := Parse(query)
	if err != nil {
		t.Error(err)
	}
	if len(gq.Children) != 2 {
		t.Errorf("Expected 2. Got: %v", len(gq.Children))
	}
	if err := checkAttr(gq.Children[0], "name"); err != nil {
		t.Error(err)
	}
	if err := checkAttr(gq.Children[1], "friends"); err != nil {
		t.Error(err)
	}
	f := gq.Children[1]
	if len(f.Children) != 0 {
		t.Errorf("Expected 0. Got: %v", len(gq.Children))
	}
}

func TestParse_block(t *testing.T) {
	query := `
		{
			root(_uid_: 0x0a) {
				type.object.name.es-419
			}
		}
	`
	gq, _, err := Parse(query)
	if err != nil {
		t.Error(err)
	}
	if len(gq.Children) != 1 {
		t.Errorf("Expected 1. Got: %v", len(gq.Children))
	}
	if err := checkAttr(gq.Children[0], "type.object.name.es-419"); err != nil {
		t.Error(err)
	}
}

func TestParseMutation(t *testing.T) {
	query := `
		mutation {
			set {
				<name> <is> <something> .
				<hometown> <is> <san francisco> .
			}
			delete {
				<name> <is> <something-else> .
			}
		}
	`
	_, mu, err := Parse(query)
	if err != nil {
		t.Error(err)
		return
	}
	if strings.Index(mu.Set, "<name> <is> <something> .") == -1 {
		t.Error("Unable to find mutation content.")
	}
	if strings.Index(mu.Set, "<hometown> <is> <san francisco> .") == -1 {
		t.Error("Unable to find mutation content.")
	}
	if strings.Index(mu.Del, "<name> <is> <something-else> .") == -1 {
		t.Error("Unable to find mutation content.")
	}
}

func TestParseMutation_error(t *testing.T) {
	query := `
		mutation {
			set {
				<name> <is> <something> .
				<hometown> <is> <san francisco> .
			}
			delete {
				<name> <is> <something-else> .
		}
	`
	_, _, err := Parse(query)
	if err == nil {
		t.Error(err)
		return
	}
	t.Log(err)
}

func TestParseMutation_error2(t *testing.T) {
	query := `
		mutation {
			set {
				<name> <is> <something> .
				<hometown> <is> <san francisco> .
			}
			delete {
				<name> <is> <something-else> .
			}
		}
		mutation {
			set {
				another one?
			}
		}

	`
	_, _, err := Parse(query)
	if err == nil {
		t.Error(err)
		return
	}
	t.Log(err)
}

func TestParseMutationAndQuery(t *testing.T) {
	query := `
		mutation {
			set {
				<name> <is> <something> .
				<hometown> <is> <san francisco> .
			}
			delete {
				<name> <is> <something-else> .
			}
		}
		query {
			me(_xid_: tomhanks) {
				name
				hometown
			}
		}
	`
	gq, mu, err := Parse(query)
	if err != nil {
		t.Error(err)
		return
	}

	if mu == nil {
		t.Error("mutation is nil")
		return
	}
	if strings.Index(mu.Set, "<name> <is> <something> .") == -1 {
		t.Error("Unable to find mutation content.")
	}
	if strings.Index(mu.Set, "<hometown> <is> <san francisco> .") == -1 {
		t.Error("Unable to find mutation content.")
	}
	if strings.Index(mu.Del, "<name> <is> <something-else> .") == -1 {
		t.Error("Unable to find mutation content.")
	}

	if gq == nil {
		t.Error("subgraph is nil")
		return
	}
	if gq.XID != "tomhanks" {
		t.Errorf("Expected: tomhanks. Got: %v", gq.XID)
		return
	}
	if len(gq.Children) != 2 {
		t.Errorf("Expected 2 children. Got: %v", len(gq.Children))
		return
	}
	if err := checkAttr(gq.Children[0], "name"); err != nil {
		t.Error(err)
	}
	if err := checkAttr(gq.Children[1], "hometown"); err != nil {
		t.Error(err)
	}
}
