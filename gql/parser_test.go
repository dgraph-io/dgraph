/*
 * Copyright 2015 Manish R Jain <manishrjain@gmail.com>
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
	"testing"

	"github.com/dgraph-io/dgraph/query"
)

func checkAttr(g *query.SubGraph, attr string) error {
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

	sg, err := Parse(query)
	if err != nil {
		t.Error(err)
	}
	if sg == nil {
		t.Error("subgraph is nil")
		return
	}
	if len(sg.Children) != 4 {
		t.Errorf("Expected 4 children. Got: %v", len(sg.Children))
		return
	}
	if err := checkAttr(sg.Children[0], "friends"); err != nil {
		t.Error(err)
	}
	if err := checkAttr(sg.Children[1], "gender"); err != nil {
		t.Error(err)
	}
	if err := checkAttr(sg.Children[2], "age"); err != nil {
		t.Error(err)
	}
	if err := checkAttr(sg.Children[3], "hometown"); err != nil {
		t.Error(err)
	}
	child := sg.Children[0]
	if len(child.Children) != 1 {
		t.Errorf("Expected 1 child of friends. Got: %v", len(child.Children))
	}
	if err := checkAttr(child.Children[0], "name"); err != nil {
		t.Error(err)
	}
}

func TestParseXid(t *testing.T) {
	// logrus.SetLevel(logrus.DebugLevel)
	query := `
	query {
		user(_uid_: 0x11) {
			type.object.name
		}
	}`
	sg, err := Parse(query)
	if err != nil {
		t.Error(err)
		return
	}
	if sg == nil {
		t.Error("subgraph is nil")
		return
	}
	if len(sg.Children) != 1 {
		t.Errorf("Expected 1 children. Got: %v", len(sg.Children))
	}
	if err := checkAttr(sg.Children[0], "type.object.name"); err != nil {
		t.Error(err)
	}
}

func TestParse_error1(t *testing.T) {
	query := `
		mutation {
			me(_uid_:0x0a) {
				name
			}
		}
	`
	var err error
	_, err = Parse(query)
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
	_, err = Parse(query)
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
	sg, err := Parse(query)
	if err != nil {
		t.Error(err)
	}
	if len(sg.Children) != 2 {
		t.Errorf("Expected 2. Got: %v", len(sg.Children))
	}
	if err := checkAttr(sg.Children[0], "name"); err != nil {
		t.Error(err)
	}
	if err := checkAttr(sg.Children[1], "friends"); err != nil {
		t.Error(err)
	}
	f := sg.Children[1]
	if len(f.Children) != 0 {
		t.Errorf("Expected 0. Got: %v", len(sg.Children))
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
	sg, err := Parse(query)
	if err != nil {
		t.Error(err)
	}
	if len(sg.Children) != 1 {
		t.Errorf("Expected 1. Got: %v", len(sg.Children))
	}
	if err := checkAttr(sg.Children[0], "type.object.name.es-419"); err != nil {
		t.Error(err)
	}
}
