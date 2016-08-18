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

	"github.com/dgraph-io/dgraph/lex"
)

func TestNewLexer(t *testing.T) {
	input := `
	query {
		me( id: 10, xid: rick ) {
			name0 # my name
			_city, # 0what would fail lex.
			profilePic(width: 100, height: 100)
			friends {
				name
			}
		}
	}`
	l := &lex.Lexer{}
	l.Init(input)
	go run(l)
	for item := range l.Items {
		if item.Typ == lex.ItemError {
			t.Error(item.String())
		}
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
		me(xid: rick) {
			_city
		}
	}`
	l := &lex.Lexer{}
	l.Init(input)
	go run(l)
	for item := range l.Items {
		if item.Typ == lex.ItemError {
			t.Error(item.String())
		}
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
	l := &lex.Lexer{}
	l.Init(input)
	go run(l)
	var typ lex.ItemType
	for item := range l.Items {
		t.Log(item.String())
		typ = item.Typ
	}
	if typ != lex.ItemError {
		t.Error("This should fail.")
	}
}

func TestVariables1(t *testing.T) {
	input := `
	query testQuery($username: String!) {
		me(xid: rick) {
			_city
		}
	}`
	l := &lex.Lexer{}
	l.Init(input)
	go run(l)
	for item := range l.Items {
		if item.Typ == lex.ItemError {
			t.Error(item.String())
		}
		t.Log(item.String(), item.Typ)
	}
}

func TestVariables2(t *testing.T) {
	input := `
	query testQuery ($username: String!, $id: int!, $email: string) {
		me(xid: rick) {
			_city
		}
	}`
	l := &lex.Lexer{}
	l.Init(input)
	go run(l)
	for item := range l.Items {
		if item.Typ == lex.ItemError {
			t.Error(item.String())
		}
		t.Log(item.String(), item.Typ)
	}
}

func TestVariablesError(t *testing.T) {
	input := `
	query testQuery($username: String! {
		me(xid: rick) {
			_city
		}
	}`
	l := &lex.Lexer{}
	l.Init(input)
	go run(l)
	var typ lex.ItemType
	for item := range l.Items {
		t.Log(item.String())
		typ = item.Typ
	}
	if typ != lex.ItemError {
		t.Error("This should fail.")
	}
}
