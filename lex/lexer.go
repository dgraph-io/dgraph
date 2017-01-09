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

package lex

import (
	"fmt"
	"unicode/utf8"

	"github.com/dgraph-io/dgraph/x"
)

const EOF = -1

// ItemType is used to set the type of a token. These constants can be defined
// in the file containing state functions. Note that their value should be >= 5.
type ItemType int

const (
	ItemEOF   ItemType = iota
	ItemError          // error
)

// stateFn represents the state of the scanner as a function that
// returns the next state.
type StateFn func(*Lexer) StateFn

type item struct {
	Typ ItemType
	Val string
}

func (i item) String() string {
	switch i.Typ {
	case 0:
		return "EOF"
	}
	return fmt.Sprintf("lex.Item [%v] %q", i.Typ, i.Val)
}

type ParseIterator struct {
	items []item
	idx   int
}

func (l *Lexer) NewIterator() *ParseIterator {
	it := &ParseIterator{
		items: l.Items,
		idx:   -1,
	}
	return it
}

// Valid returns true if we haven't consumed all the items.
func (p *ParseIterator) Next() bool {
	p.idx++
	if p.idx >= len(p.items) {
		return false
	}
	return true
}

// Item returns the current item and advances the index.
func (p *ParseIterator) Item() item {
	return p.items[p.idx]
}

// Prev moves the index back by one.
func (p *ParseIterator) Prev() bool {
	if p.idx > 0 {
		p.idx--
		return true
	}
	return false
}

// Peek returns the next n items without consuming them.
func (p *ParseIterator) Peek(num int) ([]item, error) {
	if (p.idx + num + 1) > len(p.items) {
		return nil, x.Errorf("Out of range for peek")
	}
	return p.items[p.idx+1 : p.idx+num+1], nil
}

type Lexer struct {
	// NOTE: Using a text scanner wouldn't work because it's designed for parsing
	// Golang. It won't keep track of start position, or allow us to retrieve
	// slice from [start:pos]. Better to just use normal string.
	Input           string // string being scanned.
	Start           int    // start position of this item.
	Pos             int    // current position of this item.
	Width           int    // width of last rune read from input.
	Items           []item // channel of scanned items.
	Depth           int    // nesting of {}
	ArgDepth        int    // nesting of ()
	Mode            int    // mode based on information so far.
	InsideDirective bool   // To indicate we are inside directive.
}

func (l *Lexer) Init(input string) {
	l.Input = input
	l.Items = make([]item, 0, 100)
}

// Errorf returns the error state function.
func (l *Lexer) Errorf(format string, args ...interface{}) StateFn {
	l.Items = append(l.Items, item{
		Typ: ItemError,
		Val: fmt.Sprintf(format, args...),
	})
	return nil
}

// Emit emits the item with it's type information.
func (l *Lexer) Emit(t ItemType) {
	if t != ItemEOF && l.Pos < l.Start {
		// Let ItemEOF go through.
		return
	}
	l.Items = append(l.Items, item{
		Typ: t,
		Val: l.Input[l.Start:l.Pos],
	})
	l.Start = l.Pos
}

// Next reads the next rune from the Input, sets the Width and advances Pos.
func (l *Lexer) Next() (result rune) {
	if l.Pos >= len(l.Input) {
		l.Width = 0
		return EOF
	}
	r, w := utf8.DecodeRuneInString(l.Input[l.Pos:])
	l.Width = w
	l.Pos += l.Width
	return r
}

func (l *Lexer) Backup() {
	l.Pos -= l.Width
}

func (l *Lexer) Peek() rune {
	r := l.Next()
	l.Backup()
	return r
}

func (l *Lexer) Ignore() {
	l.Start = l.Pos
}

type CheckRune func(r rune) bool

func (l *Lexer) AcceptRun(c CheckRune) {
	for {
		r := l.Next()
		if r == EOF || !c(r) {
			break
		}
	}
	l.Backup()
}

func (l *Lexer) AcceptUntil(c CheckRune) {
	for {
		r := l.Next()
		if r == EOF || c(r) {
			break
		}
	}
	l.Backup()
}
