package gql

import (
	"strconv"

	"github.com/dgraph-io/dgraph/lex"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

type MathTree struct {
	Fn    string
	Var   string
	Const types.Val // This will always be parsed as a float value
	Val   map[uint64]types.Val
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
