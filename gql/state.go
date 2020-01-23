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

// Package gql is responsible for lexing and parsing a GraphQL query/mutation.
package gql

import (
	"github.com/dgraph-io/dgraph/lex"
)

const (
	leftCurl    = '{'
	rightCurl   = '}'
	leftRound   = '('
	rightRound  = ')'
	leftSquare  = '['
	rightSquare = ']'
	period      = '.'
	comma       = ','
	slash       = '/'
	equal       = '='
	quote       = '"'
	at          = '@'
	colon       = ':'
	lsThan      = '<'
	star        = '*'
)

// Constants representing type of different graphql lexed items.
const (
	itemText                 lex.ItemType = 5 + iota // plain text
	itemLeftCurl                                     // left curly bracket
	itemRightCurl                                    // right curly bracket
	itemEqual                                        // equals to symbol
	itemName                                         // [9] names
	itemOpType                                       // operation type
	itemLeftRound                                    // left round bracket
	itemRightRound                                   // right round bracket
	itemColon                                        // Colon
	itemAt                                           // @
	itemPeriod                                       // .
	itemDollar                                       // $
	itemRegex                                        // /
	itemMutationOp                                   // mutation operation (set, delete)
	itemMutationOpContent                            // mutation operation content
	itemUpsertBlock                                  // mutation upsert block
	itemUpsertBlockOp                                // upsert block op (query, mutate)
	itemUpsertBlockOpContent                         // upsert block operations' content
	itemLeftSquare
	itemRightSquare
	itemComma
	itemMathOp
)

// lexIdentifyBlock identifies whether it is an upsert block
// If the block begins with "{" => mutation block
// Else if the block begins with "upsert" => upsert block
func lexIdentifyBlock(l *lex.Lexer) lex.StateFn {
	l.Mode = lexIdentifyBlock
	for {
		switch r := l.Next(); {
		case isSpace(r) || lex.IsEndOfLine(r):
			l.Ignore()
		case isNameBegin(r):
			return lexNameBlock
		case r == leftCurl:
			l.Backup()
			return lexInsideMutation
		case r == '#':
			return lexComment
		case r == lex.EOF:
			return l.Errorf("Invalid mutation block")
		default:
			return l.Errorf("Unexpected character while identifying mutation block: %#U", r)
		}
	}
}

// lexNameBlock lexes the blocks, for now, only upsert block
func lexNameBlock(l *lex.Lexer) lex.StateFn {
	// The caller already checked isNameBegin, and absorbed one rune.
	l.AcceptRun(isNameSuffix)
	switch word := l.Input[l.Start:l.Pos]; word {
	case "upsert":
		l.Emit(itemUpsertBlock)
		return lexUpsertBlock
	default:
		return l.Errorf("Invalid block: [%s]", word)
	}
}

// lexUpsertBlock lexes the upsert block
func lexUpsertBlock(l *lex.Lexer) lex.StateFn {
	l.Mode = lexUpsertBlock
	for {
		switch r := l.Next(); {
		case r == rightCurl:
			l.BlockDepth--
			l.Emit(itemRightCurl)
			if l.BlockDepth == 0 {
				return lexTopLevel
			}
		case r == leftCurl:
			l.BlockDepth++
			l.Emit(itemLeftCurl)
		case isSpace(r) || lex.IsEndOfLine(r):
			l.Ignore()
		case isNameBegin(r):
			return lexNameUpsertOp
		case r == '#':
			return lexComment
		case r == lex.EOF:
			return l.Errorf("Unclosed upsert block")
		default:
			return l.Errorf("Unrecognized character in upsert block: %#U", r)
		}
	}
}

// lexNameUpsertOp parses the operation names inside upsert block
func lexNameUpsertOp(l *lex.Lexer) lex.StateFn {
	// The caller already checked isNameBegin, and absorbed one rune.
	l.AcceptRun(isNameSuffix)
	word := l.Input[l.Start:l.Pos]
	switch word {
	case "query":
		l.Emit(itemUpsertBlockOp)
		return lexBlockContent
	case "mutation":
		l.Emit(itemUpsertBlockOp)
		return lexInsideMutation
	case "fragment":
		l.Emit(itemUpsertBlockOp)
		return lexBlockContent
	default:
		return l.Errorf("Invalid operation type: %s", word)
	}
}

// lexBlockContent lexes and absorbs the text inside a block (covered by braces).
func lexBlockContent(l *lex.Lexer) lex.StateFn {
	return lexContent(l, leftCurl, rightCurl, lexUpsertBlock)
}

// lexIfContent lexes the whole of @if directive in a mutation block (covered by small brackets)
func lexIfContent(l *lex.Lexer) lex.StateFn {
	if r := l.Next(); r != at {
		return l.Errorf("Expected [@], found; [%#U]", r)
	}

	l.AcceptRun(isNameSuffix)
	word := l.Input[l.Start:l.Pos]
	if word != "@if" {
		return l.Errorf("Expected @if, found [%v]", word)
	}

	return lexContent(l, '(', ')', lexInsideMutation)
}

func lexContent(l *lex.Lexer, leftRune, rightRune rune, returnTo lex.StateFn) lex.StateFn {
	depth := 0
	for {
		switch l.Next() {
		case lex.EOF:
			return l.Errorf("Matching brackets not found")
		case quote:
			if err := l.LexQuotedString(); err != nil {
				return l.Errorf(err.Error())
			}
		case leftRune:
			depth++
		case rightRune:
			depth--
			switch {
			case depth < 0:
				return l.Errorf("Unopened %c found", rightRune)
			case depth == 0:
				l.Emit(itemUpsertBlockOpContent)
				return returnTo
			}
		}
	}

}

func lexInsideMutation(l *lex.Lexer) lex.StateFn {
	l.Mode = lexInsideMutation
	for {
		switch r := l.Next(); {
		case r == at:
			l.Backup()
			return lexIfContent
		case r == rightCurl:
			l.Depth--
			l.Emit(itemRightCurl)
			if l.Depth == 0 {
				return lexTopLevel
			}
		case r == leftCurl:
			l.Depth++
			l.Emit(itemLeftCurl)
			if l.Depth >= 2 {
				return lexTextMutation
			}
		case isSpace(r) || lex.IsEndOfLine(r):
			l.Ignore()
		case isNameBegin(r):
			return lexNameMutation
		case r == '#':
			return lexComment
		case r == lex.EOF:
			return l.Errorf("Unclosed mutation action")
		default:
			return l.Errorf("Unrecognized character inside mutation: %#U", r)
		}
	}
}

func lexInsideSchema(l *lex.Lexer) lex.StateFn {
	l.Mode = lexInsideSchema
	for {
		switch r := l.Next(); {
		case r == rightRound:
			l.Emit(itemRightRound)
		case r == leftRound:
			l.Emit(itemLeftRound)
		case r == rightCurl:
			l.Depth--
			l.Emit(itemRightCurl)
			if l.Depth == 0 {
				return lexTopLevel
			}
		case r == leftCurl:
			l.Depth++
			l.Emit(itemLeftCurl)
		case r == leftSquare:
			l.Emit(itemLeftSquare)
		case r == rightSquare:
			l.Emit(itemRightSquare)
		case isSpace(r) || lex.IsEndOfLine(r):
			l.Ignore()
		case isNameBegin(r):
			return lexArgName
		case r == '#':
			return lexComment
		case r == colon:
			l.Emit(itemColon)
		case r == comma:
			l.Emit(itemComma)
		case r == lex.EOF:
			return l.Errorf("Unclosed schema action")
		default:
			return l.Errorf("Unrecognized character inside schema: %#U", r)
		}
	}
}

func lexFuncOrArg(l *lex.Lexer) lex.StateFn {
	l.Mode = lexFuncOrArg
	var empty bool
	for {
		switch r := l.Next(); {
		case r == at:
			l.Emit(itemAt)
			return lexDirectiveOrLangList
		case isNameBegin(r) || isNumber(r):
			return lexArgName
		case r == slash:
			// if argument starts with '/' it's a regex, otherwise it's a division
			if empty {
				return lexRegex(l)
			}
			fallthrough
		case isMathOp(r):
			l.Emit(itemMathOp)
		case isInequalityOp(r):
			if r == equal {
				if !isInequalityOp(l.Peek()) {
					l.Emit(itemEqual)
					continue
				}
			}
			if r == lsThan {
				if !isSpace(l.Peek()) && l.Peek() != '=' {
					// as long as its not '=' or ' '
					return lexIRIRef
				}
			}
			if isInequalityOp(l.Peek()) {
				l.Next()
			}
			l.Emit(itemMathOp)
		case r == leftRound:
			l.Emit(itemLeftRound)
			l.ArgDepth++
		case r == rightRound:
			if l.ArgDepth == 0 {
				return l.Errorf("Unexpected right round bracket")
			}
			l.ArgDepth--
			l.Emit(itemRightRound)
			if empty {
				return l.Errorf("Empty Argument")
			}
			if l.ArgDepth == 0 {
				return lexQuery // Filter directive is done.
			}
		case r == lex.EOF:
			return l.Errorf("Unclosed Brackets")
		case isSpace(r) || lex.IsEndOfLine(r):
			l.Ignore()
		case r == comma:
			if empty {
				return l.Errorf("Consecutive commas not allowed.")
			}
			empty = true
			l.Emit(itemComma)
		case isDollar(r):
			l.Emit(itemDollar)
		case r == colon:
			l.Emit(itemColon)
		case r == quote:
			{
				empty = false
				if err := l.LexQuotedString(); err != nil {
					return l.Errorf(err.Error())
				}
				l.Emit(itemName)
			}
		case isEndLiteral(r):
			{
				empty = false
				l.AcceptUntil(isEndLiteral) // This call will backup the ending ".
				l.Next()                    // Consume the " .
				l.Emit(itemName)
			}
		case r == leftSquare:
			l.Emit(itemLeftSquare)
		case r == rightSquare:
			l.Emit(itemRightSquare)
		case r == '#':
			return lexComment
		case r == '.':
			l.Emit(itemPeriod)
		default:
			return l.Errorf("Unrecognized character inside a func: %#U", r)
		}
	}
}

func lexTopLevel(l *lex.Lexer) lex.StateFn {
	// TODO(Aman): Find a way to identify different blocks in future. We only have
	// Upsert block right now. BlockDepth tells us nesting of blocks. Currently, only
	// the Upsert block has nested mutation/query/fragment blocks.
	if l.BlockDepth != 0 {
		return lexUpsertBlock
	}

	l.Mode = lexTopLevel
Loop:
	for {
		switch r := l.Next(); {
		case r == leftCurl:
			l.Depth++ // one level down.
			l.Emit(itemLeftCurl)
			return lexQuery
		case r == rightCurl:
			return l.Errorf("Too many right curl")
		case r == lex.EOF:
			break Loop
		case r == '#':
			return lexComment
		case r == leftRound:
			l.Backup()
			l.Emit(itemText)
			l.Next()
			l.Emit(itemLeftRound)
			l.ArgDepth++
			return lexQuery
		case isSpace(r) || lex.IsEndOfLine(r):
			l.Ignore()
		case isNameBegin(r):
			l.Backup()
			return lexOperationType
		}
	}
	if l.Pos > l.Start {
		l.Emit(itemText)
	}
	l.Emit(lex.ItemEOF)
	return nil
}

// lexQuery lexes the input string and calls other lex functions.
func lexQuery(l *lex.Lexer) lex.StateFn {
	l.Mode = lexQuery
	for {
		switch r := l.Next(); {
		case r == period:
			l.Emit(itemPeriod)
		case r == rightCurl:
			l.Depth--
			l.Emit(itemRightCurl)
			if l.Depth == 0 {
				return lexTopLevel
			}
		case r == leftCurl:
			l.Depth++
			l.Emit(itemLeftCurl)
		case r == lex.EOF:
			return l.Errorf("Unclosed action")
		case isSpace(r) || lex.IsEndOfLine(r):
			l.Ignore()
		case r == comma:
			l.Emit(itemComma)
		case isNameBegin(r):
			return lexName
		case r == '#':
			return lexComment
		case r == '-':
			l.Emit(itemMathOp)
		case r == leftRound:
			l.Emit(itemLeftRound)
			l.AcceptRun(isSpace)
			l.Ignore()
			l.ArgDepth++
			return lexFuncOrArg
		case r == colon:
			l.Emit(itemColon)
		case r == at:
			l.Emit(itemAt)
			return lexDirectiveOrLangList
		case r == lsThan:
			return lexIRIRef
		default:
			return l.Errorf("Unrecognized character in lexText: %#U", r)
		}
	}
}

func lexIRIRef(l *lex.Lexer) lex.StateFn {
	if err := lex.IRIRef(l, itemName); err != nil {
		return l.Errorf(err.Error())
	}
	return l.Mode
}

// lexDirectiveOrLangList is called right after we see a @.
func lexDirectiveOrLangList(l *lex.Lexer) lex.StateFn {
	r := l.Next()
	// Check first character.
	if !isNameBegin(r) && r != period && r != star {
		return l.Errorf("Unrecognized character in lexDirective: %#U", r)
	}
	l.Backup()

	for {
		r := l.Next()
		if r == period {
			l.Emit(itemName)
			return l.Mode
		}
		if isLangOrDirective(r) {
			continue
		}
		l.Backup()
		l.Emit(itemName)
		break
	}
	return l.Mode
}

func lexName(l *lex.Lexer) lex.StateFn {
	l.AcceptRun(isNameSuffix)
	l.Emit(itemName)
	return l.Mode
}

// lexComment lexes a comment text.
func lexComment(l *lex.Lexer) lex.StateFn {
	for {
		r := l.Next()
		if lex.IsEndOfLine(r) {
			l.Ignore()
			return l.Mode
		}
		if r == lex.EOF {
			break
		}
	}
	l.Ignore()
	l.Emit(lex.ItemEOF)
	return l.Mode
}

// lexNameMutation lexes the itemMutationOp, which could be set or delete.
func lexNameMutation(l *lex.Lexer) lex.StateFn {
	for {
		// The caller already checked isNameBegin, and absorbed one rune.
		r := l.Next()
		if isNameSuffix(r) {
			continue
		}
		l.Backup()
		l.Emit(itemMutationOp)
		break
	}
	return l.Mode
}

// lexTextMutation lexes and absorbs the text inside a mutation operation block.
func lexTextMutation(l *lex.Lexer) lex.StateFn {
	for {
		r := l.Next()
		if r == lex.EOF {
			return l.Errorf("Unclosed mutation text")
		}
		if r == quote {
			return lexMutationValue
		}
		if r == leftCurl {
			return l.Errorf("Invalid character '{' inside mutation text")
		}
		if r != rightCurl {
			// Absorb everything until we find '}'.
			continue
		}
		l.Backup()
		l.Emit(itemMutationOpContent)
		break
	}
	return lexInsideMutation
}

// This function is used to absorb the object value.
func lexMutationValue(l *lex.Lexer) lex.StateFn {
LOOP:
	for {
		r := l.Next()
		switch r {
		case lex.EOF:
			return l.Errorf("Unclosed mutation value")
		case quote:
			break LOOP
		case '\\':
			l.Next() // skip one.
		}
	}
	return lexTextMutation
}

func lexRegex(l *lex.Lexer) lex.StateFn {
LOOP:
	for {
		r := l.Next()
		switch r {
		case lex.EOF:
			return l.Errorf("Unclosed regexp")
		case '\\':
			l.Next()
		case '/':
			break LOOP
		}
	}
	l.AcceptRun(isRegexFlag)
	l.Emit(itemRegex)
	return l.Mode
}

// lexOperationType lexes a query or mutation or schema operation type.
func lexOperationType(l *lex.Lexer) lex.StateFn {
	l.AcceptRun(isNameSuffix)
	// l.Pos would be index of the end of operation type + 1.
	word := l.Input[l.Start:l.Pos]
	switch word {
	case "mutation":
		l.Emit(itemOpType)
		return lexInsideMutation
	case "fragment":
		l.Emit(itemOpType)
		return lexQuery
	case "query":
		l.Emit(itemOpType)
		return lexQuery
	case "schema":
		l.Emit(itemOpType)
		return lexInsideSchema
	default:
		return l.Errorf("Invalid operation type: %s", word)
	}
}

// lexArgName lexes and emits the name part of an argument.
func lexArgName(l *lex.Lexer) lex.StateFn {
	l.AcceptRun(isNameSuffix)
	l.Emit(itemName)
	return l.Mode
}

// isDollar returns true if the rune is a Dollar($).
func isDollar(r rune) bool {
	return r == '$' || r == '\u0024'
}

// isSpace returns true if the rune is a tab or space.
func isSpace(r rune) bool {
	return r == '\u0009' || r == '\u0020'
}

// isEndLiteral returns true if rune is quotation mark.
func isEndLiteral(r rune) bool {
	return r == '"' || r == '\u000d' || r == '\u000a'
}

func isLangOrDirective(r rune) bool {
	if isNameBegin(r) {
		return true
	}
	if r == '-' {
		return true
	}
	if r >= '0' && r <= '9' {
		return true
	}
	if r == '*' {
		return true
	}
	return false
}

// isNameBegin returns true if the rune is an alphabet or an '_' or '~'.
func isNameBegin(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z':
		return true
	case r >= 'A' && r <= 'Z':
		return true
	case r == '_':
		return true
	case r == '~':
		return true
	default:
		return false
	}
}

func isMathOp(r rune) bool {
	switch r {
	case '+', '-', '*', '/', '%':
		return true
	default:
		return false
	}
}

func isInequalityOp(r rune) bool {
	switch r {
	case '<', '>', '=', '!':
		return true
	default:
		return false
	}
}

func isNumber(r rune) bool {
	switch {
	case (r >= '0' && r <= '9'):
		return true
	default:
		return false
	}
}

func isNameSuffix(r rune) bool {
	if isMathOp(r) {
		return false
	}
	if isNameBegin(r) || isNumber(r) {
		return true
	}
	if r == '.' /*|| r == '!'*/ { // Use by freebase.
		return true
	}
	return false
}

func isRegexFlag(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z':
		return true
	case r >= 'A' && r <= 'Z':
		return true
	default:
		return false
	}
}
