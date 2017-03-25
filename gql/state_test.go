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

func TestNewSchemaQuery(t *testing.T) {
	input := `
	schema {
		pred
		type
	}
	schema( pred : name ) {
		pred
		type
	}
	schema( pred : name,hi ) { #This is valid in lexer, parser would throw error
		pred
		type
	}
	schema( pred : [name,hi] ) {
		pred
		type
	}`
	l := lex.NewLexer(input).Run(lexText)
	it := l.NewIterator()
	for it.Next() {
		item := it.Item()
		require.NotEqual(t, item.Typ, lex.ItemError)
		t.Log(item.String())
	}
}

func TestAbruptSchemaQuery(t *testing.T) {
	input := `
	schema {
		pred
	`
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

func TestLangSupport(t *testing.T) {
	input := `
	query {
		me(id: test) {
			name@en
		}
	}
	`
	l := lex.NewLexer(input).Run(lexText)
	it := l.NewIterator()
	for it.Next() {
		item := it.Item()
		require.NotEqual(t, item.Typ, lex.ItemError)
		t.Log(item.String())
	}
}

func TestMultiLangSupport(t *testing.T) {
	input := `
	query {
		me(id: test) {
			name@en, name@en:ru:fr:de
		}
	}
	`
	l := lex.NewLexer(input).Run(lexText)
	it := l.NewIterator()
	for it.Next() {
		item := it.Item()
		require.NotEqual(t, item.Typ, lex.ItemError)
		t.Log(item.String())
	}
}
