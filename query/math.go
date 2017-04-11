package query

import (
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

func processBinary(mNode *gql.MathTree) (err error) {
	destMap := make(map[uint64]types.Val)
	aggName := mNode.Fn

	mpl := mNode.Child[0].Val
	mpr := mNode.Child[1].Val
	cl := mNode.Child[0].Const
	cr := mNode.Child[1].Const

	if mpl != nil {
		for k, lVal := range mpl {
			ag := aggregator{
				name: aggName,
			}
			// Only the UIDs that have all the values will be considered.
			rVal := mpr[k]
			if cr.Value != nil {
				// Use the constant value that was supplied.
				rVal = cr
			}
			ag.ApplyVal(lVal)
			ag.ApplyVal(rVal)
			destMap[k], err = ag.Value()
			if err != nil {
				return err
			}
		}
		mNode.Val = destMap
		return nil
	}
	if mpr != nil {
		for k, rVal := range mpr {
			ag := aggregator{
				name: aggName,
			}
			// Only the UIDs that have all the values will be considered.
			lVal := mpl[k]
			if cl.Value != nil {
				// Use the constant value that was supplied.
				lVal = cl
			}
			ag.ApplyVal(lVal)
			ag.ApplyVal(rVal)
			destMap[k], err = ag.Value()
			if err != nil {
				return err
			}
		}
		mNode.Val = destMap
		return nil
	}
	// Both maps are nil, so 2 constatns.
	ag := aggregator{
		name: aggName,
	}
	ag.ApplyVal(cl)
	ag.ApplyVal(cr)
	mNode.Const, err = ag.Value()
	return err
}
func processUnary(mNode *gql.MathTree) (err error) {
	destMap := make(map[uint64]types.Val)
	srcMap := mNode.Child[0].Val
	aggName := mNode.Fn
	ch := mNode.Child[0]
	ag := aggregator{
		name: aggName,
	}
	if ch.Const.Value != nil {
		// Use the constant value that was supplied.
		ag.ApplyVal(ch.Const)
		mNode.Const, err = ag.Value()
		return err
	}

	for k, val := range srcMap {
		ag.ApplyVal(val)
		destMap[k], err = ag.Value()
		if err != nil {
			return err
		}
	}
	mNode.Val = destMap
	return nil

}
func processBinaryBoolean(mNode *gql.MathTree) (err error) {
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
			return x.Wrapf(err, "Wrong values in comaprison function.")
		}
		destMap[k] = types.Val{
			Tid:   types.BoolID,
			Value: res,
		}
	}
	mNode.Val = destMap
	return nil
}

func processTernary(mNode *gql.MathTree) (err error) {
	destMap := make(map[uint64]types.Val)
	aggName := mNode.Fn
	condMap := mNode.Child[0].Val
	if condMap == nil {
		return x.Errorf("Expected a value variable in %v but missing.", aggName)
	}
	varOne := mNode.Child[1].Val
	varTwo := mNode.Child[2].Val
	constOne := mNode.Child[1].Const
	constTwo := mNode.Child[2].Const
	for k, val := range condMap {
		var res types.Val
		v, ok := val.Value.(bool)
		if !ok {
			return x.Errorf("First variable of conditional function not a bool value")
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

func evalMathTree(mNode *gql.MathTree, doneVars map[string]values) (err error) {
	if mNode.Const.Value != nil {
		return nil
	}
	if mNode.Var != "" {
		d, ok := doneVars[mNode.Var]
		if !ok || d.vals == nil {
			return x.Errorf("Variable %v not yet populated or missing.", mNode.Var)
		}
		mNode.Val = d.vals
		return nil
	}

	for _, child := range mNode.Child {
		err := evalMathTree(child, doneVars)
		if err != nil {
			return err
		}
	}

	aggName := mNode.Fn
	if isUnary(aggName) && len(mNode.Child) != 1 {
		return x.Errorf("Function %v expects 1 argument. But got: %v", aggName,
			len(mNode.Child))
	}
	if (isBinary(aggName) || isBinaryBoolean(aggName)) && len(mNode.Child) != 2 {
		return x.Errorf("Function %v expects 2 argument. But got: %v", aggName,
			len(mNode.Child))
	}
	if isTernary(aggName) && len(mNode.Child) != 3 {
		return x.Errorf("Function %v expects 3 argument. But got: %v", aggName,
			len(mNode.Child))
	}

	// Handle ternary conditional operator separately here.
	if isTernary(aggName) {
		return processTernary(mNode)
	}

	// Handle binary boolean operators separately here.
	if isBinaryBoolean(aggName) {
		return processBinaryBoolean(mNode)
	}

	// Handle unary operators here.
	if isUnary(aggName) {
		return processUnary(mNode)
	}

	if isBinary(aggName) {
		return processBinary(mNode)
	}
	return x.Errorf("Unhandled Math operator: %v", aggName)
}
