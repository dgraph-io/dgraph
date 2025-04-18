/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package query

import (
	"github.com/golang/glog"
	"github.com/pkg/errors"

	"github.com/hypermodeinc/dgraph/v25/types"
)

type mathTree struct {
	Fn    string
	Var   string
	Const types.Val // If its a const value node.
	Val   map[uint64]types.Val
	Child []*mathTree
}

var (
	ErrorIntOverflow     = errors.New("Integer overflow")
	ErrorFloat32Overflow = errors.New("Float32 overflow")
	ErrorDivisionByZero  = errors.New("Division by zero")
	ErrorFractionalPower = errors.New("Fractional power of negative number")
	ErrorNegativeLog     = errors.New("Log of negative number")
	ErrorNegativeRoot    = errors.New("Root of negative number")
	ErrorVectorsNotMatch = errors.New("The length of vectors must match")
	ErrorArgsDisagree    = errors.New("Left and right arguments must match")
	ErrorShouldBeVector  = errors.New("Type should be []float, but is not. Cannot determine type.")
	ErrorBadVectorMult   = errors.New("Cannot multiply vector by vector")
)

// processBinary handles the binary operands like
// +, -, *, /, %, max, min, logbase, dot
func processBinary(mNode *mathTree) error {
	destMap := make(map[uint64]types.Val)
	aggName := mNode.Fn

	mpl := mNode.Child[0].Val
	mpr := mNode.Child[1].Val
	cl := mNode.Child[0].Const
	cr := mNode.Child[1].Const

	f := func(k uint64) error {
		ag := aggregator{
			name: aggName,
		}
		lVal := mpl[k]
		if cl.Value != nil {
			// Use the constant value that was supplied.
			lVal = cl
		}
		rVal := mpr[k]
		if cr.Value != nil {
			// Use the constant value that was supplied.
			rVal = cr
		}
		err := ag.ApplyVal(lVal)
		if err != nil {
			return err
		}
		err = ag.ApplyVal(rVal)
		if err != nil {
			return err
		}
		destMap[k], err = ag.Value()
		if err != nil {
			return err
		}
		return nil
	}

	// If mpl or mpr have 0 and just 0 in it, that means it's an output of aggregation somewhere.
	// This value would need to be applied to all.
	checkAggrResult := func(value map[uint64]types.Val) (types.Val, bool) {
		if len(value) != 1 {
			return types.Val{}, false
		}

		val, ok := value[0]
		return val, ok
	}

	if val, ok := checkAggrResult(mpl); ok {
		cl = val
		mpl = nil
	} else if val, ok := checkAggrResult(mpr); ok {
		cr = val
		mpr = nil
	}

	if len(mpl) != 0 || len(mpr) != 0 {
		for k := range mpr {
			if err := f(k); err != nil {
				return err
			}
		}
		for k := range mpl {
			if _, ok := mpr[k]; ok {
				continue
			}
			if err := f(k); err != nil {
				return err
			}
		}
		mNode.Val = destMap
		return nil
	}

	if cl.Value != nil && cr.Value != nil {
		// Both maps are nil, so 2 constants.
		ag := aggregator{
			name: aggName,
		}
		err := ag.ApplyVal(cl)
		if err != nil {
			return err
		}
		err = ag.ApplyVal(cr)
		if err != nil {
			return err
		}
		mNode.Const, err = ag.Value()
		return err
	}
	return nil
}

// processUnary handles the unary operands like
// u-, log, exp, since, floor, ceil
func processUnary(mNode *mathTree) error {
	destMap := make(map[uint64]types.Val)
	srcMap := mNode.Child[0].Val
	aggName := mNode.Fn
	ch := mNode.Child[0]
	ag := aggregator{
		name: aggName,
	}
	if ch.Const.Value != nil {
		// Use the constant value that was supplied.
		err := ag.ApplyVal(ch.Const)
		if err != nil {
			return err
		}
		mNode.Const, err = ag.Value()
		return err
	}

	for k, val := range srcMap {
		err := ag.ApplyVal(val)
		if err != nil {
			return err
		}
		destMap[k], err = ag.Value()
		if err != nil {
			return err
		}
	}
	mNode.Val = destMap
	return nil

}

// processBinaryBoolean handles the binary operands which
// return a boolean value.
// All the inequality operators (<, >, <=, >=, !=, ==)
func processBinaryBoolean(mNode *mathTree) error {
	destMap := make(map[uint64]types.Val)
	srcMap := mNode.Child[0].Val
	aggName := mNode.Fn

	ch := mNode.Child[1]
	curMap := ch.Val
	for k, val := range srcMap {
		curVal := curMap[k]
		if ch.Const.Value != nil {
			// Use the constant value that was supplied.
			curVal = ch.Const
		}
		res, err := compareValues(aggName, val, curVal)
		if err != nil {
			return errors.Wrapf(err, "Wrong values in comparison function.")
		}
		destMap[k] = types.Val{
			Tid:   types.BoolID,
			Value: res,
		}
	}
	mNode.Val = destMap
	return nil
}

// processTernary handles the ternary operand cond()
func processTernary(mNode *mathTree) error {
	destMap := make(map[uint64]types.Val)
	aggName := mNode.Fn
	condMap := mNode.Child[0].Val
	if len(condMap) == 0 {
		return errors.Errorf("Expected a value variable in %v but missing.", aggName)
	}
	varOne := mNode.Child[1].Val
	varTwo := mNode.Child[2].Val
	constOne := mNode.Child[1].Const
	constTwo := mNode.Child[2].Const
	for k, val := range condMap {
		var res types.Val
		v, ok := val.Value.(bool)
		if !ok {
			return errors.Errorf("First variable of conditional function not a bool value")
		}
		if v {
			// Pick the value of first map.
			if constOne.Value != nil {
				res = constOne
			} else {
				res = varOne[k]
			}
		} else {
			// Pick the value of second map.
			if constTwo.Value != nil {
				res = constTwo
			} else {
				res = varTwo[k]
			}
		}
		destMap[k] = res
	}
	mNode.Val = destMap
	return nil
}

func evalMathTree(mNode *mathTree) error {
	if mNode.Const.Value != nil {
		return nil
	}
	if mNode.Var != "" {
		if len(mNode.Val) == 0 {
			glog.V(2).Infof("Variable %v not yet populated or missing.", mNode.Var)
		}
		// This is a leaf node whose value is already populated. So return.
		return nil
	}

	for _, child := range mNode.Child {
		// Process the child nodes first.
		err := evalMathTree(child)
		if err != nil {
			return err
		}
	}

	aggName := mNode.Fn
	if isUnary(aggName) {
		if len(mNode.Child) != 1 {
			return errors.Errorf("Function %v expects 1 argument. But got: %v", aggName,
				len(mNode.Child))
		}
		return processUnary(mNode)
	}

	if isBinary(aggName) {
		if len(mNode.Child) != 2 {
			return errors.Errorf("Function %v expects 2 argument. But got: %v", aggName,
				len(mNode.Child))
		}
		return processBinary(mNode)
	}

	if isBinaryBoolean(aggName) {
		if len(mNode.Child) != 2 {
			return errors.Errorf("Function %v expects 2 argument. But got: %v", aggName,
				len(mNode.Child))
		}
		return processBinaryBoolean(mNode)
	}

	if isTernary(aggName) {
		if len(mNode.Child) != 3 {
			return errors.Errorf("Function %v expects 3 argument. But got: %v", aggName,
				len(mNode.Child))
		}
		return processTernary(mNode)
	}

	return errors.Errorf("Unhandled Math operator: %v", aggName)
}
