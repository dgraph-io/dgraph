package gqlex

import (
	"strings"
	"unicode"
)

const (
	leftCurl  = "{"
	rightCurl = "}"
)

// stateFn represents the state of the scanner as a function that
// returns the next state.
type stateFn func(*lexer) stateFn

func lexText(l *lexer) stateFn {
	for {
		if strings.HasPrefix(l.input[l.pos:], leftCurl) {
			if l.pos > l.start {
				l.emit(itemText)
			}
			return lexLeftCurl
		}
		if strings.HasPrefix(l.input[l.pos:], rightCurl) {
			return l.errorf("Too many right brackets")
		}
		if l.next() == EOF {
			break
		}
	}
	// Correctly reached EOF.
	if l.pos > l.start {
		l.emit(itemText)
	}
	l.emit(itemEOF)
	return nil // Stop the run loop.
}

func lexLeftCurl(l *lexer) stateFn {
	l.pos += len(leftCurl)
	l.depth += 1
	l.emit(itemLeftCurl)
	return lexInside(l)
}

func lexRightCurl(l *lexer) stateFn {
	l.pos += len(rightCurl)
	l.depth -= 1
	l.emit(itemRightCurl)

	if l.depth == 0 {
		return lexText
	} else {
		return lexInside
	}
}

func lexInside(l *lexer) stateFn {
	for {
		if strings.HasPrefix(l.input[l.pos:], rightCurl) {
			return lexRightCurl
		}
		if strings.HasPrefix(l.input[l.pos:], leftCurl) {
			return lexLeftCurl
		}

		switch r := l.next(); {
		case r == EOF:
			return l.errorf("unclosed action")
		case isSpace(r) || isEndOfLine(r):
			l.ignore()
		case isAlphaNumeric(r):
			l.backup()
			return lexIdentifier
		}
	}
}

func lexIdentifier(l *lexer) stateFn {
Loop:
	for {
		switch r := l.next(); {
		case isAlphaNumeric(r):
			// absorb.
		default:
			l.backup()
			l.emit(itemIdentifier)
			break Loop
		}
	}
	return lexInside
}

func isSpace(r rune) bool {
	return r == ' ' || r == '\t'
}

func isEndOfLine(r rune) bool {
	return r == '\r' || r == '\n'
}

func isAlphaNumeric(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}
