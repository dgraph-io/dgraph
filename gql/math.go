package gql

import (
	"github.com/dgraph-io/dgraph/lex"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

type MathTree struct {
	Fn    string
	Var   string
	Val   map[uint64]*types.Val
	Child []*MathTree
}

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
		Val: make(map[uint64]*types.Val),
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
			child := &MathTree{
				Var: item.Val,
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

/*
var mathOpPrecedence map[string]int

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

// parseFilter parses the filter directive to produce a QueryFilter / parse tree.
func parseMathFunc(it *lex.ItemIterator) (*MathTree, error) {
	it.Next()
	item := it.Item()
	if item.Typ != itemLeftRound {
		return nil, x.Errorf("Expected ( after filter directive")
	}

	// opStack is used to collect the operators in right order.
	opStack := new(mathTreeStack)
	opStack.push(&MathTree{Op: "("}) // Push ( onto operator stack.
	// valueStack is used to collect the values.
	valueStack := new(mathTreeStack)

	for it.Next() {
		item := it.Item()
		lval := strings.ToLower(item.Val)
		if lval == "and" || lval == "or" || lval == "not" { // Handle operators.
			op := lval
			opPred := filterOpPrecedence[op]
			x.AssertTruef(opPred > 0, "Expected opPred > 0: %d", opPred)
			// Evaluate the stack until we see an operator with strictly lower pred.
			for !opStack.empty() {
				topOp := opStack.peek()
				if filterOpPrecedence[topOp.Op] < opPred {
					break
				}
				err := evalStack(opStack, valueStack)
				if err != nil {
					return nil, err
				}
			}
			opStack.push(&MathTree{Op: op}) // Push current operator.
		} else if item.Typ == itemName { // Value.
			it.Prev()
			f, err := parseFunction(it)
			if err != nil {
				return nil, err
			}
			leaf := &MathTree{Func: f}
			valueStack.push(leaf)
		} else if item.Typ == itemLeftRound { // Just push to op stack.
			opStack.push(&MathTree{Op: "("})

		} else if item.Typ == itemRightRound { // Pop op stack until we see a (.
			for !opStack.empty() {
				topOp := opStack.peek()
				if topOp.Op == "(" {
					break
				}
				err := evalStack(opStack, valueStack)
				if err != nil {
					return nil, err
				}
			}
			_, err := opStack.pop() // Pop away the (.
			if err != nil {
				return nil, x.Errorf("Invalid filter statement")
			}
			if opStack.empty() {
				// The parentheses are balanced out. Let's break.
				break
			}
		} else {
			return nil, x.Errorf("Unexpected item while parsing @filter: %v", item)
		}
	}

	// For filters, we start with ( and end with ). We expect to break out of loop
	// when the parentheses balance off, and at that point, opStack should be empty.
	// For other applications, typically after all items are
	// consumed, we will run a loop like "while opStack is nonempty, evalStack".
	// This is not needed here.
	x.AssertTruef(opStack.empty(), "Op stack should be empty when we exit")

	if valueStack.empty() {
		// This happens when we have @filter(). We can either return an error or
		// ignore. Currently, let's just ignore and pretend there is no filter.
		return nil, nil
	}

	if valueStack.size() != 1 {
		return nil, x.Errorf("Expected one item in value stack, but got %d",
			valueStack.size())
	}
	return valueStack.pop()
}
*/
