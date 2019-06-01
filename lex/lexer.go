/*
 * Copyright 2015-2018 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
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
	"github.com/pkg/errors"
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

type Item struct {
	Typ    ItemType
	Val    string
	line   int
	column int
}

func (i Item) Errorf(format string, args ...interface{}) error {
	return fmt.Errorf("line %d column %d: "+format,
		append([]interface{}{i.line, i.column}, args...)...)
}

func (i Item) String() string {
	switch i.Typ {
	case ItemEOF:
		return "EOF"
	}
	return fmt.Sprintf("lex.Item [%v] %q at %d:%d", i.Typ, i.Val, i.line, i.column)
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

func (p *ItemIterator) Errorf(format string, args ...interface{}) error {
	nextItem, _ := p.PeekOne()
	return nextItem.Errorf(format, args...)
}

// Next advances the iterator by one.
func (p *ItemIterator) Next() bool {
	p.idx++
	if p.idx >= len(p.l.items) {
		return false
	}
	return true
}

// Item returns the current item.
func (p *ItemIterator) Item() Item {
	if p.idx < 0 || p.idx >= len(p.l.items) {
		return Item{
			line:   -1, // using negative numbers to indicate out-of-range item
			column: -1,
		}
	}
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

// Restore restores the the iterator to position specified.
func (p *ItemIterator) Restore(pos int) {
	x.AssertTrue(pos <= len(p.l.items) && pos >= -1)
	p.idx = pos
}

// Save returns the current position of the iterator which we can use for restoring later.
func (p *ItemIterator) Save() int {
	return p.idx
}

// Peek returns the next n items without consuming them.
func (p *ItemIterator) Peek(num int) ([]Item, error) {
	if (p.idx + num + 1) > len(p.l.items) {
		return nil, errors.Errorf("Out of range for peek")
	}
	return p.l.items[p.idx+1 : p.idx+num+1], nil
}

// PeekOne returns the next 1 item without consuming it.
func (p *ItemIterator) PeekOne() (Item, bool) {
	if p.idx+1 >= len(p.l.items) {
		return Item{
			line:   -1,
			column: -1, // use negative number to indicate out of range
		}, false
	}
	return p.l.items[p.idx+1], true
}

// A RuneWidth represents a consecutive string of runes with the same width
// and the number of runes is stored in count.
// The reason we maintain this information is to properly backup when multiple look-aheads happen.
// For example, if the following sequence of events happen
// 1. Lexer.Next() consumes 1 byte
// 2. Lexer.Next() consumes 1 byte
// 3. Lexer.Next() consumes 3 bytes
// we would create two RunWidthTrackers, the 1st having width 1 and count 2, while the 2nd having
// width 3 and count 1, then the following backups can be done properly:
// 4. Lexer.Backup() should decrement the pos by 3
// 5. Lexer.Backup() should decrement the pos by 1
// 6. Lexer.Backup() should decrement the pos by 1
type RuneWidth struct {
	width int
	// count should be always greater than or equal to 1, because we pop a tracker item
	// from the stack when count is about to reach 0
	count int
}

type Lexer struct {
	// NOTE: Using a text scanner wouldn't work because it's designed for parsing
	// Golang. It won't keep track of Start Position, or allow us to retrieve
	// slice from [Start:Pos]. Better to just use normal string.
	Input      string // string being scanned.
	Start      int    // Start Position of this item.
	Pos        int    // current Position of this item.
	Width      int    // Width of last rune read from input.
	widthStack []*RuneWidth
	items      []Item  // channel of scanned items.
	Depth      int     // nesting of {}
	ArgDepth   int     // nesting of ()
	Mode       StateFn // Default state to go back to after reading a token.
	Line       int     // the current line number corresponding to Start
	Column     int     // the current column number corresponding to Start
}

func NewLexer(input string) *Lexer {
	return &Lexer{
		Input:  input,
		Line:   1,
		Column: 0,
	}
}

func (l *Lexer) ValidateResult() error {
	it := l.NewIterator()
	for it.Next() {
		item := it.Item()
		if item.Typ == ItemError {
			return errors.New(item.Val)
		}
	}
	return nil
}

func (l *Lexer) Run(f StateFn) *Lexer {
	for state := f; state != nil; {
		// The following statement is useful for debugging.
		//fmt.Printf("Func: %v\n", runtime.FuncForPC(reflect.ValueOf(state).Pointer()).Name())
		state = state(l)
	}
	return l
}

// Errorf returns the error state function.
func (l *Lexer) Errorf(format string, args ...interface{}) StateFn {
	l.items = append(l.items, Item{
		Typ: ItemError,
		Val: fmt.Sprintf("while lexing %v at line %d column %d: "+format,
			append([]interface{}{l.Input, l.Line, l.Column}, args...)...),
		line:   l.Line,
		column: l.Column,
	})
	return nil
}

// Emit emits the item with it's type information.
func (l *Lexer) Emit(t ItemType) {
	if t != ItemEOF && l.Pos < l.Start {
		// Let ItemEOF go through.
		return
	}
	l.items = append(l.items, Item{
		Typ:    t,
		Val:    l.Input[l.Start:l.Pos],
		line:   l.Line,
		column: l.Column,
	})
	l.moveStartToPos()
}

func (l *Lexer) pushWidth(width int) {
	wl := len(l.widthStack)
	if wl == 0 || l.widthStack[wl-1].width != width {
		l.widthStack = append(l.widthStack, &RuneWidth{
			count: 1,
			width: width,
		})
	} else {
		l.widthStack[wl-1].count++
	}
}

// Next reads the next rune from the Input, sets the Width and advances Pos.
func (l *Lexer) Next() (result rune) {
	if l.Pos >= len(l.Input) {
		l.pushWidth(0)
		return EOF
	}
	r, w := utf8.DecodeRuneInString(l.Input[l.Pos:])
	l.pushWidth(w)
	l.Pos += w
	return r
}

func (l *Lexer) Backup() {
	wl := len(l.widthStack)
	x.AssertTruef(wl > 0,
		"Backup should not be called when the width tracker stack is empty")
	rw := l.widthStack[wl-1]
	if rw.count == 1 {
		l.widthStack = l.widthStack[:wl-1] // pop the item from the stack
	} else {
		rw.count--
	}
	l.Pos -= rw.width
}

func (l *Lexer) Peek() rune {
	r := l.Next()
	l.Backup()
	return r
}

func (l *Lexer) moveStartToPos() {
	// check if we are about to move Start to a new line
	for offset := l.Start; offset < l.Pos; {
		r, w := utf8.DecodeRuneInString(l.Input[offset:l.Pos])
		offset += w
		if IsEndOfLine(r) {
			l.Line++
			l.Column = 0
		} else {
			l.Column += w
		}
	}
	l.Start = l.Pos
}

func (l *Lexer) Ignore() {
	l.moveStartToPos()
}

// CheckRune is predicate signature for accepting valid runes on input.
type CheckRune func(r rune) bool

// CheckRuneRec is like CheckRune with Lexer as extra argument.
// This can be used to recursively call other CheckRune(s).
type CheckRuneRec func(r rune, l *Lexer) bool

// AcceptRun accepts tokens based on CheckRune
// until it returns false or EOF is reached.
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
// until it returns false or EOF is reached.
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

const (
	quote = '"'
)

// ECHAR ::= '\' [vtbnrf"'\]
func (l *Lexer) IsEscChar(r rune) bool {
	switch r {
	case 'v', 't', 'b', 'n', 'r', 'f', '"', '\'', '\\':
		return true
	}
	return false
}

// IsEndOfLine returns true if the rune is a Linefeed or a Carriage return.
func IsEndOfLine(r rune) bool {
	return r == '\u000A' || r == '\u000D'
}

func (l *Lexer) LexQuotedString() error {
	l.Backup()
	r := l.Next()
	if r != quote {
		return errors.Errorf("String should start with quote.")
	}
	for {
		r := l.Next()
		if r == EOF {
			return errors.Errorf("Unexpected end of input.")
		}
		if r == '\\' {
			r := l.Next()
			if !l.IsEscChar(r) {
				return errors.Errorf("Not a valid escape char: '%c'", r)
			}
			continue // eat the next char
		}
		if r == quote {
			break
		}
	}
	return nil
}
