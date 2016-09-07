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
		return fmt.Errorf("Expected attr: %v. Got: %v", attr, g.Attr)
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
	if err := checkAttr(gq.Children[1], "friends"); err != nil {
		t.Error(err)
	}
	if gq.Children[1].Args["first"] != "10" {
		t.Errorf("Expected count 10. Got: %v", gq.Children[1].Args["first"])
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

func TestParseAfter(t *testing.T) {
	query := `
	query {
		user(_xid_: m.abcd) {
			type.object.name
			friends (first: 10, after: 3) {
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
	if err := checkAttr(gq.Children[1], "friends"); err != nil {
		t.Error(err)
	}
	if gq.Children[1].Args["first"] != "10" {
		t.Errorf("Expected count 10. Got: %v", gq.Children[1].Args["first"])
	}
	if gq.Children[1].Args["after"] != "3" {
		t.Errorf("Expected after to be 3. Got: %v", gq.Children[1].Args["after"])
	}
}

func TestParseOffset(t *testing.T) {
	query := `
	query {
		user(_xid_: m.abcd) {
			type.object.name
			friends (first: 10, offset: 3) {
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
	if err := checkAttr(gq.Children[1], "friends"); err != nil {
		t.Error(err)
	}
	if gq.Children[1].Args["first"] != "10" {
		t.Errorf("Expected count 10. Got: %v", gq.Children[1].Args["First"])
	}
	if gq.Children[1].Args["offset"] != "3" {
		t.Errorf("Expected Offset 3. Got: %v", gq.Children[1].Args["offset"])
	}
}

func TestParseOffset_error(t *testing.T) {
	query := `
	query {
		user(_xid_: m.abcd) {
			type.object.name
			friends (first: 10, offset: ) {
			}
		}
	}`
	_, _, err := Parse(query)
	if err == nil {
		t.Error("Expected error on negative offset")
		return
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

func TestParseFragmentNoNesting(t *testing.T) {
	query := `
	query {
		user(_uid_:0x0a) {
			...fragmenta,...fragmentb
			friends {
				name
			}
			...fragmentc
			hobbies
			...fragmentd
		}
	}
	
	fragment fragmenta {
		name
	}
	
	fragment fragmentb {
		id
	}
	
	fragment fragmentc {
		name
	}
	
	fragment fragmentd {
		id
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
	if len(gq.Children) != 6 {
		t.Errorf("Expected 6 children. Got: %v", len(gq.Children))
		return
	}

	// Notice that the order is preserved.
	expectedFields := []string{"name", "id", "friends", "name", "hobbies", "id"}
	for i, v := range expectedFields {
		if err := checkAttr(gq.Children[i], v); err != nil {
			t.Error(err)
			return
		}
	}
}

func TestParseFragmentNest1(t *testing.T) {
	query := `
	query {
		user(_uid_:0x0a) {
			...fragmenta
			friends {
				name
			}
		}
	}
	
	fragment fragmenta {
		id
		...fragmentb
	}
	
	fragment fragmentb {
		hobbies
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
	if len(gq.Children) != 3 {
		t.Errorf("Expected 3 children. Got: %v", len(gq.Children))
		return
	}

	// Notice that the order is preserved.
	expectedFields := []string{"id", "hobbies", "friends"}
	for i, v := range expectedFields {
		if err := checkAttr(gq.Children[i], v); err != nil {
			t.Error(err)
			return
		}
	}
}

func TestParseFragmentNest2(t *testing.T) {
	query := `
	query {
		user(_uid_:0x0a) {
			friends {
				...fragmenta
			}
		}
	}
	fragment fragmenta {
		name
		...fragmentb
	}
	fragment fragmentb {
		nickname
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
	if len(gq.Children) != 1 {
		t.Errorf("Expected 1 child. Got: %v", len(gq.Children))
		return
	}

	gq = gq.Children[0]
	if len(gq.Children) != 2 {
		t.Errorf("Expected 2 child. Got: %v", len(gq.Children))
		return
	}

	expectedFields := []string{"name", "nickname"}
	for i, v := range expectedFields {
		if err := checkAttr(gq.Children[i], v); err != nil {
			t.Error(err)
			return
		}
	}
}

func TestParseFragmentCycle(t *testing.T) {
	query := `
	query {
		user(_uid_:0x0a) {
			...fragmenta
		}
	}
	fragment fragmenta {
		name
		...fragmentb
	}
	fragment fragmentb {
		...fragmentc
	}
	fragment fragmentc {
		id
		...fragmenta
	}
`
	_, _, err := Parse(query)
	if err == nil {
		t.Error("Expected error with cycle")
	}
}

func TestParseVariables(t *testing.T) {
	query := `{
		"query": "query testQuery( $a  : int   , $b: int){root(_uid_: 0x0a) {name(first: $b, after: $a){english}}}", 
		"variables": {"$a": "6", "$b": "5" } 
	}`
	_, _, err := Parse(query)
	if err != nil {
		t.Error(err)
	}
}

func TestParseVariables1(t *testing.T) {
	query := `{
		"query": "query testQuery($a: int , $b: int!){root(_uid_: 0x0a) {name(first: $b){english}}}", 
		"variables": {"$b": "5" } 
	}`
	_, _, err := Parse(query)
	if err != nil {
		t.Error(err)
	}
}

func TestParseVariables2(t *testing.T) {
	query := `{
		"query": "query testQuery($a: float , $b: bool!){root(_uid_: 0x0a) {name{english}}}", 
		"variables": {"$b": "false", "$a": "3.33" } 
	}`
	_, _, err := Parse(query)
	if err != nil {
		t.Error(err)
	}
}

func TestParseVariables3(t *testing.T) {
	query := `{
		"query": "query testQuery($a: int , $b: int! = 3){root(_uid_: 0x0a) {name(first: $b){english}}}", 
		"variables": {"$a": "5" } 
	}`
	_, _, err := Parse(query)
	if err != nil {
		t.Error(err)
	}
}

func TestParseVariablesStringfiedJSON(t *testing.T) {
	query := `{
		"query": "query testQuery($a: int , $b: int! = 3){root(_uid_: 0x0a) {name(first: $b){english}}}", 
		"variables": "{\"$a\": \"5\" }" 
	}`
	_, _, err := Parse(query)
	if err != nil {
		t.Error(err)
	}
}

func TestParseVariablesDefault1(t *testing.T) {
	query := `{
		"query": "query testQuery($a: int = 3  , $b: int =  4 ,  $c : int = 3){root(_uid_: 0x0a) {name(first: $b, after: $a, offset: $c){english}}}", 
		"variables": {"$b": "5" } 
	}`
	_, _, err := Parse(query)
	if err != nil {
		t.Error(err)
	}
}
func TestParseVariablesFragments(t *testing.T) {
	query := `{
	"query": "query test($a: int){user(_uid_:0x0a) {...fragmentd,friends(first: $a, offset: $a) {name}}} fragment fragmentd {id(first: $a)}",
	"variables": {"$a": "5"}
}`
	gq, _, err := Parse(query)
	if err != nil {
		t.Error(err)
	}
	if gq == nil {
		t.Error("subgraph is nil")
		return
	}
	if len(gq.Children) != 2 {
		t.Errorf("Expected 2 children. Got: %v", len(gq.Children))
		return
	}

	// Notice that the order is preserved.

	if gq.Children[0].Args["first"] != "5" {
		t.Error("Expected first to be 5. Got %v", gq.Children[2].Args["first"])
	}
}

func TestParseVariablesError1(t *testing.T) {
	query := `
	query testQuery($a: string, $b: int!){
			root(_uid_: 0x0a) {
				type.object.name.es-419
			}
		}
	`
	_, _, err := Parse(query)
	if err == nil {
		t.Error("Expected error")
	}
}

func TestParseVariablesError2(t *testing.T) {
	query := `{
		"query": "query testQuery($a: int, $b: int, $c: int!){
			root(_uid_: 0x0a) {name(first: $b, after: $a){english}}
		}", 
		"variables": {"$a": "6", "$b": "5" } 
	}`
	_, _, err := Parse(query)
	if err == nil {
		t.Error("Expected value for variable $c")
	}
}

func TestParseVariablesError3(t *testing.T) {
	query := `{
		"query": "query testQuery($a: int, $b: , $c: int!){
			root(_uid_: 0x0a) {name(first: $b, after: $a){english}}
		}", 
		"variables": {"$a": "6", "$b": "5" } 
	}`
	_, _, err := Parse(query)
	if err == nil {
		t.Error("Expected type for variable $b")
	}
}

func TestParseVariablesError4(t *testing.T) {
	query := `{
		"query": "query testQuery($a: bool , $b: float! = 3){root(_uid_: 0x0a) {name(first: $b){english}}}", 
		"variables": {"$a": "5" } 
	}`
	_, _, err := Parse(query)
	if err == nil {
		t.Error("Expected type error")
	}
}

func TestParseVariablesError5(t *testing.T) {
	query := `{
		"query": "query ($a: int, $b: int){root(_uid_: 0x0a) {name(first: $b, after: $a){english}}}", 
		"variables": {"$a": "6", "$b": "5" } 
	}`
	_, _, err := Parse(query)
	if err == nil {
		t.Error("Expected error: Query with variables should be named")
	}
}

func TestParseVariablesError6(t *testing.T) {
	query := `{
		"query": "query ($a: int, $b: random){root(_uid_: 0x0a) {name(first: $b, after: $a){english}}}", 
		"variables": {"$a": "6", "$b": "5" } 
	}`
	_, _, err := Parse(query)
	if err == nil {
		t.Error("Expected error: Type random not supported")
	}
}

func TestParseVariablesError7(t *testing.T) {
	query := `{
		"query": "query testQuery($a: int, $b: int, $c: int!){
			root(_uid_: 0x0a) {name(first: $b, after: $a){english}}
		}", 
		"variables": {"$a": "6", "$b": "5", "$d": "abc" } 
	}`
	_, _, err := Parse(query)
	if err == nil {
		t.Error("Expected type for variable $d")
	}
}
