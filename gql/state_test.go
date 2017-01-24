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
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/lex"
)

func TestNewLexer(t *testing.T) {
	input := `
	query {
		me(_xid_: rick, id:10 ) {
			name0 # my name
			_city, # 0what would fail lex.
			profilePic(width: 100, height: 100)
			friends {
				name
			}
		}
	}`
	l := lex.NewLexer(input).Run(lexText)

	it := l.NewIterator()
	for it.Next() {
		item := it.Item()
		require.NotEqual(t, item.Typ, lex.ItemError)
		t.Log(item.String())
	}
}

func TestNewLexerMutation(t *testing.T) {
	input := `
	mutation {
		set {
			What is <this> .
			Why is this #!!?
			How is this?
		}
		delete {
			Why is this
		}
	}
	query {
		me(_xid_: rick) {
			_city
		}
	}`
	l := lex.NewLexer(input).Run(lexText)
	it := l.NewIterator()
	for it.Next() {
		item := it.Item()
		require.NotEqual(t, item.Typ, lex.ItemError)
		t.Log(item.String())
	}
}

func TestAbruptMutation(t *testing.T) {
	input := `
	mutation {
		set {
			What is <this> .
			Why is this #!!?
			How is this?
	}`
	l := lex.NewLexer(input).Run(lexText)
	var typ lex.ItemType
	it := l.NewIterator()
	for it.Next() {
		item := it.Item()
		t.Log(item.String())
		typ = item.Typ
	}
	require.Equal(t, lex.ItemError, typ)
}

func TestVariables1(t *testing.T) {
	input := `
	query testQuery($username: String!) {
		me(_xid_: rick) {
			_city
		}
	}`
	l := lex.NewLexer(input).Run(lexText)
	it := l.NewIterator()
	for it.Next() {
		item := it.Item()
		require.NotEqual(t, item.Typ, lex.ItemError)
		t.Log(item.String(), item.Typ)
	}
}

func TestVariables2(t *testing.T) {
	input := `
	query testQuery ($username: String, $id: int, $email: string) {
		me(_xid_: rick) {
			_city
		}
	}`
	l := lex.NewLexer(input).Run(lexText)
	it := l.NewIterator()
	for it.Next() {
		item := it.Item()
		require.NotEqual(t, item.Typ, lex.ItemError)
		t.Log(item.String(), item.Typ)
	}
}

func TestVariablesDefault(t *testing.T) {
	input := `
	query testQuery ($username: string = abc, $id: int = 5, $email: string) {
		me(_xid_: rick) {
			_city
		}
	}`
	l := lex.NewLexer(input).Run(lexText)
	it := l.NewIterator()
	for it.Next() {
		item := it.Item()
		require.NotEqual(t, item.Typ, lex.ItemError)
		t.Log(item.String())
	}
}

func TestIRIRef(t *testing.T) {
	input := `
	query testQuery {
		me(id : <http://helloworld.com/how/are/you>) {
		        <http://verygood.com/what/about/you>
		}
	}`
	l := lex.NewLexer(input).Run(lexText)
	it := l.NewIterator()
	for it.Next() {
		item := it.Item()
		require.NotEqual(t, item.Typ, lex.ItemError)
		t.Log(item.String())
	}
}
