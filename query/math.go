package query

import (
	"fmt"

	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

// processBinary handles the binary operands like
// +, -, *, /, %, max, min, logbase
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
			err = ag.ApplyVal(lVal)
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
			err = ag.ApplyVal(lVal)
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
		}
		mNode.Val = destMap
		return nil
	}
	// Both maps are nil, so 2 constatns.
	ag := aggregator{
		name: aggName,
	}
	err = ag.ApplyVal(cl)
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

// processUnary handles the unary operands like
// u-, log, exp, since, floor, ceil
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
		err = ag.ApplyVal(ch.Const)
		if err != nil {
			return err
		}
		mNode.Const, err = ag.Value()
		return err
	}

	for k, val := range srcMap {
		err = ag.ApplyVal(val)
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

// processTernary handles the ternary operand cond()
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

func rec(mNode *gql.MathTree, allowedVars map[string]struct{}) error {
	if mNode == nil {
		return nil
	}
	if _, ok := allowedVars[mNode.Var]; mNode.Var != "" && !ok {
		return x.Errorf("Var %v used at wrong math level", mNode.Var)
	}
	for _, ch := range mNode.Child {
		if err := rec(ch, allowedVars); err != nil {
			return err
		}
	}
	return nil
}

// transformParentVar transforms the value variable at the parent level to the variable
// at its child level using the childs UID matrix.
func transformParentVar(mNode *gql.MathTree, parent *SubGraph, doneVars map[string]values) error {
	// If a variable from parent level is used at this level, we do some
	// level based processing before doing the math op.
	siblingVar, parentVar := "", ""
	isSameLevel := true
	var modMathNode *gql.MathTree
	for _, mch := range mNode.Child {
		for _, fvar := range parent.Params.FacetVar {
			if fvar == mch.Var {
				parentVar = mch.Var
				isSameLevel = false
				modMathNode = mch
			} else {
				siblingVar = mch.Var
			}
		}
	}
	fmt.Println(parentVar, siblingVar, "***", isSameLevel)

	if isSameLevel {
		// No ransformation required.
		return nil
	}

	// The parent has the variable. So do an iterative summing.
	var siblingNode *SubGraph
	for _, ch := range parent.Children {
		for _, fvar := range ch.Params.FacetVar {
			if fvar == siblingVar {
				siblingNode = ch
				break
			}
		}
		if ch.Params.Var == siblingVar {
			siblingNode = ch
		}
	}
	if siblingNode == nil {
		return x.Errorf("Var %v is used at a wrong level", parentVar)
	}
	v, ok := doneVars[parentVar]
	if !ok {
		return x.Errorf("Variable not found.")
	}
	newMap := make(map[uint64]types.Val)
	if v.valType != 1 {
		// Ensure that this is a facet var.
		return x.Errorf("Invalid variable type encountered in math.")
	}
	for i := 0; i < len(siblingNode.uidMatrix); i++ {
		ul := siblingNode.uidMatrix[i]
		srcUid := siblingNode.SrcUIDs.Uids[i]
		curVal, ok := v.vals[srcUid]
		if curVal.Tid != types.IntID && curVal.Tid != types.FloatID {
			return x.Errorf("Encountered non int/float type for summing")
		}
		if !ok || curVal.Value == nil {
			continue
		}
		for j := 0; j < len(ul.Uids); j++ {
			dstUid := ul.Uids[j]
			ag := aggregator{name: "sum"}
			ag.Apply(curVal)
			ag.Apply(newMap[dstUid])
			val, err := ag.Value()
			if err != nil {
				continue
			}
			newMap[dstUid] = val
		}
	}
	fmt.Println(newMap, "$$$")
	modMathNode.Val = newMap
	return nil
}

func (sg *SubGraph) evalMathTree(mNode *gql.MathTree, parent *SubGraph, doneVars map[string]values) (err error) {
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
		// Process the child nodes first.
		err := sg.evalMathTree(child, parent, doneVars)
		if err != nil {
			return err
		}
	}

	allowedVars := make(map[string]struct{})
	if parent.Params.FacetVar != nil {
		for _, v := range parent.Params.FacetVar {
			allowedVars[v] = struct{}{}
		}
	}

	for _, ch := range parent.Children {
		if ch.Params.FacetVar != nil {
			for _, v := range ch.Params.FacetVar {
				allowedVars[v] = struct{}{}
			}
		}
		allowedVars[ch.Params.Var] = struct{}{}
	}

	if err := rec(mNode, allowedVars); err != nil {
		return err
	}
	fmt.Println(allowedVars)

	aggName := mNode.Fn
	if isBinary(aggName) {
		if len(mNode.Child) != 2 {
			return x.Errorf("Function %v expects 2 argument. But got: %v", aggName,
				len(mNode.Child))
		}
		err := transformParentVar(mNode, parent, doneVars)
		if err != nil {
			return err
		}
		fmt.Println(mNode.Child[0].Val, "@@@")
		fmt.Println(mNode.Child[1].Val, "@@@")
		return processBinary(mNode)
	}

	if isUnary(aggName) {
		if len(mNode.Child) != 1 {
			return x.Errorf("Function %v expects 1 argument. But got: %v", aggName,
				len(mNode.Child))
		}
		return processUnary(mNode)
	}

	if isBinaryBoolean(aggName) {
		if len(mNode.Child) != 2 {
			return x.Errorf("Function %v expects 2 argument. But got: %v", aggName,
				len(mNode.Child))
		}
		return processBinaryBoolean(mNode)
	}

	if isTernary(aggName) {
		if len(mNode.Child) != 3 {
			return x.Errorf("Function %v expects 3 argument. But got: %v", aggName,
				len(mNode.Child))
		}
		return processTernary(mNode)
	}

	return x.Errorf("Unhandled Math operator: %v", aggName)
}
