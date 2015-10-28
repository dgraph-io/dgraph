package gqlex

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/Sirupsen/logrus"
	"github.com/manishrjain/dgraph/x"
)

var glog = x.Log("lexer")

type itemType int

const (
	itemEOF       itemType = iota
	itemError              // error
	itemText               // plain text
	itemLeftCurl           // left curly bracket
	itemRightCurl          // right curly bracket
	itemComment            // comment
	itemName               // names
	itemOpType             // operation type
	itemString             // quoted string
)

const EOF = -1

type item struct {
	typ itemType
	val string
}

func (i item) String() string {
	switch i.typ {
	case itemEOF:
		return "EOF"
	case itemError:
		return i.val
	case itemName:
		return fmt.Sprintf("name: [%v]", i.val)
	}
	return fmt.Sprintf("[%v] %q", i.typ, i.val)
}

type lexer struct {
	// NOTE: Using a text scanner wouldn't work because it's designed for parsing
	// Golang. It won't keep track of start position, or allow us to retrieve
	// slice from [start:pos]. Better to just use normal string.
	input string    // string being scanned.
	start int       // start position of this item.
	pos   int       // current position of this item.
	width int       // width of last rune read from input.
	items chan item // channel of scanned items.
	depth int       // nesting of {}
}

func newLexer(input string) *lexer {
	l := &lexer{
		input: input,
		items: make(chan item),
	}
	go l.run()
	return l
}

func (l *lexer) errorf(format string,
	args ...interface{}) stateFn {
	l.items <- item{
		typ: itemError,
		val: fmt.Sprintf(format, args...),
	}
	return nil
}

func (l *lexer) emit(t itemType) {
	if t != itemEOF && l.pos <= l.start {
		// Let itemEOF go through.
		glog.WithFields(logrus.Fields{
			"start": l.start,
			"pos":   l.pos,
			"typ":   t,
		}).Info("Invalid emit")
		return
	}
	l.items <- item{
		typ: t,
		val: l.input[l.start:l.pos],
	}
	l.start = l.pos
}

func (l *lexer) run() {
	for state := lexText; state != nil; {
		state = state(l)
	}
	close(l.items) // No more tokens.
}

func (l *lexer) next() (result rune) {
	if l.pos >= len(l.input) {
		l.width = 0
		return EOF
	}
	r, w := utf8.DecodeRuneInString(l.input[l.pos:])
	l.width = w
	l.pos += l.width
	return r
}

func (l *lexer) backup() {
	l.pos -= l.width
}

func (l *lexer) peek() rune {
	r := l.next()
	l.backup()
	return r
}

func (l *lexer) ignore() {
	l.start = l.pos
}

func (l *lexer) accept(valid string) bool {
	if strings.IndexRune(valid, l.next()) >= 0 {
		return true
	}
	l.backup()
	return false
}

func (l *lexer) acceptRun(valid string) {
	for strings.IndexRune(valid, l.next()) >= 0 {
	}
	l.backup()
}
