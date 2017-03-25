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

type ItemIterator struct {
	l   *Lexer
	idx int
}

func (l *Lexer) NewIterator() *ItemIterator {
	it := &ItemIterator{
		l:   l,
		idx: -1,
	}
	return it
}

// Valid returns true if we haven't consumed all the items.
func (p *ItemIterator) Next() bool {
	p.idx++
	if p.idx >= len(p.l.items) {
		return false
	}
	return true
}

// Item returns the current item and advances the index.
func (p *ItemIterator) Item() item {
	return (p.l.items)[p.idx]
}

// Prev moves the index back by one.
func (p *ItemIterator) Prev() bool {
	if p.idx > 0 {
		p.idx--
		return true
	}
	return false
}

// Peek returns the next n items without consuming them.
func (p *ItemIterator) Peek(num int) ([]item, error) {
	if (p.idx + num + 1) > len(p.l.items) {
		return nil, x.Errorf("Out of range for peek")
	}
	return p.l.items[p.idx+1 : p.idx+num+1], nil
}

type Lexer struct {
	// NOTE: Using a text scanner wouldn't work because it's designed for parsing
	// Golang. It won't keep track of Start Position, or allow us to retrieve
	// slice from [Start:Pos]. Better to just use normal string.
	Input           string // string being scanned.
	Start           int    // Start Position of this item.
	Pos             int    // current Position of this item.
	Width           int    // Width of last rune read from input.
	items           []item // channel of scanned items.
	Depth           int    // nesting of {}
	ArgDepth        int    // nesting of ()
	Mode            int    // mode based on information so far.
	InsideDirective bool   // To indicate we are inside directive.
}

func NewLexer(input string) *Lexer {
	l := Lexer{}
	l.Input = input
	l.items = make([]item, 0, 100)
	return &l
}

func (l *Lexer) Run(f StateFn) *Lexer {
	for state := f; state != nil; {
		// The following statement is useful for debugging.
		// fmt.Printf("Func: %v\n", runtime.FuncForPC(reflect.ValueOf(state).Pointer()).Name())
		state = state(l)
	}
	return l
}

// Errorf returns the error state function.
func (l *Lexer) Errorf(format string, args ...interface{}) StateFn {
	l.items = append(l.items, item{
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
	l.items = append(l.items, item{
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

// CheckRune is predicate signature for accepting valid runes on input.
type CheckRune func(r rune) bool

// CheckRuneRec is like CheckRune with Lexer as extra argument.
// This can be used to recursively call other CheckRune(s).
type CheckRuneRec func(r rune, l *Lexer) bool

// AcceptRun accepts tokens based on CheckRune
// untill it returns false or EOF is reached.
// Returns last rune accepted and valid flag for rune.
func (l *Lexer) AcceptRun(c CheckRune) (lastr rune, validr bool) {
	validr = false
	for {
		r := l.Next()
		if r == EOF || !c(r) {
			break
		}
		validr = true
		lastr = r
	}
	l.Backup()
	return lastr, validr
}

// AcceptRunRec accepts tokens based on CheckRuneRec
// untill it returns false or EOF is reached.
func (l *Lexer) AcceptRunRec(c CheckRuneRec) {
	for {
		r := l.Next()
		if r == EOF || !c(r, l) {
			break
		}
	}
	l.Backup()
}

// AcceptUntil accepts tokens based on CheckRune
// till it returns false or EOF is reached.
func (l *Lexer) AcceptUntil(c CheckRune) {
	for {
		r := l.Next()
		if r == EOF || c(r) {
			break
		}
	}
	l.Backup()
}

// AcceptRunTimes accepts tokens with CheckRune given number of times.
// returns number of times it was successful.
func (l *Lexer) AcceptRunTimes(c CheckRune, times int) int {
	i := 0
	for ; i < times; i++ {
		r := l.Next()
		if r == EOF || !c(r) {
			break
		}
	}
	l.Backup()
	return i
}

func (l *Lexer) IgnoreRun(c CheckRune) {
	l.AcceptRun(c)
	l.Ignore()
}
