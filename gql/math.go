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
	"bytes"
	"strconv"
	"strings"

	"github.com/dgraph-io/dgraph/lex"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

type mathTreeStack struct{ a []*MathTree }

func (s *mathTreeStack) empty() bool      { return len(s.a) == 0 }
func (s *mathTreeStack) size() int        { return len(s.a) }
func (s *mathTreeStack) push(t *MathTree) { s.a = append(s.a, t) }

func (s *mathTreeStack) popAssert() *MathTree {
	x.AssertTruef(!s.empty(), "Expected a non-empty stack")
	last := s.a[len(s.a)-1]
	s.a = s.a[:len(s.a)-1]
	return last
}

func (s *mathTreeStack) pop() (*MathTree, error) {
	if s.empty() {
		return nil, x.Errorf("Empty stack")
	}
	last := s.a[len(s.a)-1]
	s.a = s.a[:len(s.a)-1]
	return last, nil
}

func (s *mathTreeStack) peek() *MathTree {
	x.AssertTruef(!s.empty(), "Trying to peek empty stack")
	return s.a[len(s.a)-1]
}

type MathTree struct {
	Fn    string
	Var   string
	Const types.Val // This will always be parsed as a float value
	Val   map[uint64]types.Val
	Child []*MathTree
}

func isUnary(f string) bool {
	return f == "exp" || f == "ln" || f == "u-" || f == "sqrt" ||
		f == "floor" || f == "ceil" || f == "since"
}

func isBinaryMath(f string) bool {
	return f == "*" || f == "+" || f == "-" || f == "/" || f == "%"
}

func isTernary(f string) bool {
	return f == "cond"
}

func evalMathStack(opStack, valueStack *mathTreeStack) error {
	topOp, err := opStack.pop()
	if err != nil {
		return x.Errorf("Invalid Math expression")
	}
	if isUnary(topOp.Fn) {
		// Since "not" is a unary operator, just pop one value.
		topVal, err := valueStack.pop()
		if err != nil {
			return x.Errorf("Invalid math statement. Expected 1 operands")
		}
		topOp.Child = []*MathTree{topVal}

	} else if isTernary(topOp.Fn) {
		if valueStack.size() < 3 {
			return x.Errorf("Invalid Math expression. Expected 3 operands")
		}
		topVal1 := valueStack.popAssert()
		topVal2 := valueStack.popAssert()
		topVal3 := valueStack.popAssert()
		topOp.Child = []*MathTree{topVal3, topVal2, topVal1}

	} else {
		if valueStack.size() < 2 {
			return x.Errorf("Invalid Math expression. Expected 2 operands")
		}
		topVal1 := valueStack.popAssert()
		topVal2 := valueStack.popAssert()
		topOp.Child = []*MathTree{topVal2, topVal1}

	}
	// Push the new value (tree) into the valueStack.
	valueStack.push(topOp)
	return nil
}

func isMathFunc(f string) bool {
	// While adding an op, also add it to the corresponding function type.
	return f == "*" || f == "%" || f == "+" || f == "-" || f == "/" ||
		f == "exp" || f == "ln" || f == "cond" ||
		f == "<" || f == ">" || f == ">=" || f == "<=" ||
		f == "==" || f == "!=" ||
		f == "min" || f == "max" || f == "sqrt" ||
		f == "pow" || f == "logbase" || f == "floor" || f == "ceil" ||
		f == "since"
}

func parseMathFunc(it *lex.ItemIterator, again bool) (*MathTree, bool, error) {
	if !again {
		it.Next()
		item := it.Item()
		if item.Typ != itemLeftRound {
			return nil, false, x.Errorf("Expected ( after math")
		}
	}

	// opStack is used to collect the operators in right order.
	opStack := new(mathTreeStack)
	opStack.push(&MathTree{Fn: "("}) // Push ( onto operator stack.
	// valueStack is used to collect the values.
	valueStack := new(mathTreeStack)

	for it.Next() {
		item := it.Item()
		lval := strings.ToLower(item.Val)
		if isMathFunc(lval) {
			op := lval
			it.Prev()
			lastItem := it.Item()
			it.Next()
			if op == "-" &&
				(lastItem.Val == "(" || lastItem.Val == "," || isBinaryMath(lastItem.Val)) {
				op = "u-" // This is a unary -
			}
			opPred := mathOpPrecedence[op]
			x.AssertTruef(opPred > 0, "Expected opPred > 0 for %v: %d", op, opPred)
			// Evaluate the stack until we see an operator with strictly lower pred.
			for !opStack.empty() {
				topOp := opStack.peek()
				if mathOpPrecedence[topOp.Fn] < opPred {
					break
				}
				err := evalMathStack(opStack, valueStack)
				if err != nil {
					return nil, false, err
				}
			}
			opStack.push(&MathTree{Fn: op}) // Push current operator.
			peekIt, err := it.Peek(1)
			if err != nil {
				return nil, false, err
			}
			if peekIt[0].Typ == itemLeftRound {
				again := false
				var child *MathTree
				for {
					child, again, err = parseMathFunc(it, again)
					if err != nil {
						return nil, false, err
					}
					valueStack.push(child)
					if !again {
						break
					}
				}
			}
		} else if item.Typ == itemName { // Value.
			peekIt, err := it.Peek(1)
			if err != nil {
				return nil, false, err
			}
			if peekIt[0].Typ == itemLeftRound {
				again := false
				if !isMathFunc(item.Val) {
					return nil, false, x.Errorf("Unknown math function: %v", item.Val)
				}
				var child *MathTree
				for {
					child, again, err = parseMathFunc(it, again)
					if err != nil {
						return nil, false, err
					}
					valueStack.push(child)
					if !again {
						break
					}
				}
				continue
			}
			// Try to parse it as a constant.
			child := &MathTree{}
			v, err := strconv.ParseFloat(item.Val, 64)
			if err != nil {
				child.Var = item.Val
			} else {
				child.Const = types.Val{
					Tid:   types.FloatID,
					Value: v,
				}
			}
			valueStack.push(child)
		} else if item.Typ == itemLeftRound { // Just push to op stack.
			opStack.push(&MathTree{Fn: "("})

		} else if item.Typ == itemComma {
			for !opStack.empty() {
				topOp := opStack.peek()
				if topOp.Fn == "(" {
					break
				}
				err := evalMathStack(opStack, valueStack)
				if err != nil {
					return nil, false, err
				}
			}
			_, err := opStack.pop() // Pop away the (.
			if err != nil {
				return nil, false, x.Errorf("Invalid Math expression")
			}
			if !opStack.empty() {
				return nil, false, x.Errorf("Invalid math expression.")
			}
			if valueStack.size() != 1 {
				return nil, false, x.Errorf("Expected one item in value stack, but got %d",
					valueStack.size())
			}
			res, err := valueStack.pop()
			if err != nil {
				return nil, false, err
			}
			return res, true, nil
		} else if item.Typ == itemRightRound { // Pop op stack until we see a (.
			for !opStack.empty() {
				topOp := opStack.peek()
				if topOp.Fn == "(" {
					break
				}
				err := evalMathStack(opStack, valueStack)
				if err != nil {
					return nil, false, err
				}
			}
			_, err := opStack.pop() // Pop away the (.
			if err != nil {
				return nil, false, x.Errorf("Invalid Math expression")
			}
			if opStack.empty() {
				// The parentheses are balanced out. Let's break.
				break
			}
		} else {
			return nil, false, x.Errorf("Unexpected item while parsing math expression: %v", item)
		}
	}

	// For math Expressions, we start with ( and end with ). We expect to break out of loop
	// when the parentheses balance off, and at that point, opStack should be empty.
	// For other applications, typically after all items are
	// consumed, we will run a loop like "while opStack is nonempty, evalStack".
	// This is not needed here.
	x.AssertTruef(opStack.empty(), "Op stack should be empty when we exit")

	if valueStack.empty() {
		// This happens when we have math(). We can either return an error or
		// ignore. Currently, let's just ignore and pretend there is no expression.
		return nil, false, x.Errorf("Empty () not allowed in math block.")
	}

	if valueStack.size() != 1 {
		return nil, false, x.Errorf("Expected one item in value stack, but got %d",
			valueStack.size())
	}
	res, err := valueStack.pop()
	return res, false, err
}

// debugString converts mathTree to a string. Good for testing, debugging.
func (t *MathTree) debugString() string {
	buf := bytes.NewBuffer(make([]byte, 0, 20))
	t.stringHelper(buf)
	return buf.String()
}

// stringHelper does simple DFS to convert MathTree to string.
func (t *MathTree) stringHelper(buf *bytes.Buffer) {
	x.AssertTruef(t != nil, "Nil Math tree")
	if t.Var != "" {
		// Leaf node.
		buf.WriteString(t.Var)
		return
	}
	if t.Const.Value != nil {
		// Leaf node.
		buf.WriteString(strconv.FormatFloat(t.Const.Value.(float64), 'E', -1, 64))
		return
	}
	// Non-leaf node.
	buf.WriteRune('(')
	switch t.Fn {
	case "+", "-", "/", "*", "%", "exp", "ln", "cond", "min",
		"sqrt", "max", "<", ">", "<=", ">=", "==", "!=", "u-",
		"logbase", "pow":
		buf.WriteString(t.Fn)
	default:
		x.Fatalf("Unknown operator: %q", t.Fn)
	}

	for _, c := range t.Child {
		buf.WriteRune(' ')
		c.stringHelper(buf)
	}
	buf.WriteRune(')')
}
