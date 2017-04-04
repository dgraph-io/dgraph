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
	return f == "exp" || f == "log"
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
			return x.Errorf("Invalid math statement")
		}
		topOp.Child = []*MathTree{topVal}
	} else {
		// "and" and "or" are binary operators, so pop two values.
		if valueStack.size() < 2 {
			return x.Errorf("Invalid Math expression")
		}
		topVal1, _ := valueStack.pop()
		topVal2, _ := valueStack.pop()
		topOp.Child = []*MathTree{topVal2, topVal1}
	}
	// Push the new value (tree) into the valueStack.
	valueStack.push(topOp)
	return nil
}

func parseMathFunc(it *lex.ItemIterator) (*MathTree, error) {
	it.Next()
	item := it.Item()
	if item.Typ != itemLeftRound {
		return nil, x.Errorf("Expected ( after math")
	}

	// opStack is used to collect the operators in right order.
	opStack := new(mathTreeStack)
	opStack.push(&MathTree{Fn: "("}) // Push ( onto operator stack.
	// valueStack is used to collect the values.
	valueStack := new(mathTreeStack)

	for it.Next() {
		item := it.Item()
		lval := strings.ToLower(item.Val)
		if lval == "*" || lval == "+" || lval == "-" || lval == "/" ||
			lval == "exp" || lval == "log" { // Handle operators.
			op := lval
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
					return nil, err
				}
			}
			opStack.push(&MathTree{Fn: op}) // Push current operator.
			peekIt, err := it.Peek(1)
			if err != nil {
				return nil, err
			}
			if peekIt[0].Typ == itemLeftRound {
				child, err := parseMathFunc(it)
				if err != nil {
					return nil, err
				}
				valueStack.push(child)
			}
		} else if item.Typ == itemName { // Value.
			peekIt, err := it.Peek(1)
			if err != nil {
				return nil, err
			}
			if peekIt[0].Typ == itemLeftRound {
				child, err := parseMathFunc(it)
				if err != nil {
					return nil, err
				}
				//curNode.Child = append(curNode.Child, child)
				valueStack.push(child)
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
			//curNode.Child = append(curNode.Child, child)
			//continue
			//f, err := parseFunction(it)
			//if err != nil {
			//	return nil, err
			//}
			//leaf := &MathTree{Func: f}
			valueStack.push(child)
		} else if item.Typ == itemLeftRound { // Just push to op stack.
			opStack.push(&MathTree{Fn: "("})

		} else if item.Typ == itemRightRound { // Pop op stack until we see a (.
			for !opStack.empty() {
				topOp := opStack.peek()
				if topOp.Fn == "(" {
					break
				}
				err := evalMathStack(opStack, valueStack)
				if err != nil {
					return nil, err
				}
			}
			_, err := opStack.pop() // Pop away the (.
			if err != nil {
				return nil, x.Errorf("Invalid Math expression")
			}
			if opStack.empty() {
				// The parentheses are balanced out. Let's break.
				break
			}
		} else {
			return nil, x.Errorf("Unexpected item while parsing math expression: %v", item)
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
		return nil, nil
	}

	if valueStack.size() != 1 {
		return nil, x.Errorf("Expected one item in value stack, but got %d",
			valueStack.size())
	}
	return valueStack.pop()
}

// debugString converts mathTree to a string. Good for testing, debugging.
func (t *MathTree) debugString() string {
	buf := bytes.NewBuffer(make([]byte, 0, 20))
	t.stringHelper(buf)
	return buf.String()
}

// stringHelper does simple DFS to convert MathTree to string.
func (t *MathTree) stringHelper(buf *bytes.Buffer) {
	x.AssertTrue(t != nil)
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
	case "+":
		buf.WriteString("+")
	case "-":
		buf.WriteString("-")
	case "/":
		buf.WriteString("/")
	case "*":
		buf.WriteString("*")
	case "exp":
		buf.WriteString("exp")
	case "log":
		buf.WriteString("log")
	default:
		x.Fatalf("Unknown operator: %q", t.Fn)
	}

	for _, c := range t.Child {
		buf.WriteRune(' ')
		c.stringHelper(buf)
	}
	buf.WriteRune(')')
}

/*
func parseMathFunc(it *lex.ItemIterator) (*MathTree, error) {
	it.Next()
	fn := it.Item()
	if fn.Typ != itemName {
		return nil, x.Errorf("Expected a math function")
	}
	if !isValVarFunc(fn.Val) {
		return nil, x.Errorf("Expected a math function but got: %v", fn.Val)
	}

	curNode := &MathTree{
		Fn:  fn.Val,
		Val: make(map[uint64]types.Val),
	}

	it.Next()
	item := it.Item()
	if item.Typ != itemLeftRound {
		return nil, x.Errorf("Expected ( after a math function")
	}

	expectedArg := true
	for it.Next() {
		item := it.Item()
		if item.Typ == itemRightRound {
			if expectedArg {
				return nil, x.Errorf("Missing arg after comma")
			}
			break
		}
		if item.Typ == itemComma {
			if expectedArg {
				return nil, x.Errorf("Missing arg after comma")
			}
			expectedArg = true
			continue
		}
		if item.Typ == itemName {
			if !expectedArg {
				return nil, x.Errorf("Missing comma in math function")
			}
			expectedArg = false
			peekIt, err := it.Peek(1)
			if err != nil {
				return nil, err
			}
			if peekIt[0].Typ == itemLeftRound {
				// Recursively parse the child.
				it.Prev()
				child, err := parseMathFunc(it)
				if err != nil {
					return nil, err
				}
				curNode.Child = append(curNode.Child, child)
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
			curNode.Child = append(curNode.Child, child)
			continue
		}
		return nil, x.Errorf("Unexpected argument in math func: %v", item.Val)
	}
	if len(curNode.Child) == 0 {
		return nil, x.Errorf("Math function \"%v\" should have atleast one variable inside", curNode.Fn)
	}
	return curNode, nil
}
*/
