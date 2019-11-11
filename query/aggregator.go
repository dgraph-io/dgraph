/*
 * Copyright 2017-2018 Dgraph Labs, Inc. and Contributors
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

package query

import (
	"bytes"
	"math"
	"time"

	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
	"github.com/pkg/errors"
)

type aggregator struct {
	name   string
	result types.Val
	count  int // used when we need avergae.
}

func isUnary(f string) bool {
	return f == "ln" || f == "exp" || f == "u-" || f == "sqrt" ||
		f == "floor" || f == "ceil" || f == "since"
}

func isBinaryBoolean(f string) bool {
	return f == "<" || f == ">" || f == "<=" || f == ">=" ||
		f == "==" || f == "!="
}

func isTernary(f string) bool {
	return f == "cond"
}

func isBinary(f string) bool {
	return f == "+" || f == "*" || f == "-" || f == "/" || f == "%" ||
		f == "max" || f == "min" || f == "logbase" || f == "pow"
}

func convertTo(from *pb.TaskValue) (types.Val, error) {
	vh, _ := getValue(from)
	if bytes.Equal(from.Val, x.Nilbyte) {
		return vh, ErrEmptyVal
	}
	va, err := types.Convert(vh, vh.Tid)
	if err != nil {
		return vh, errors.Wrapf(err, "Fail to convert from api.Value to types.Val")
	}
	return va, err
}

func compareValues(ag string, va, vb types.Val) (bool, error) {
	if !isBinaryBoolean(ag) {
		x.Fatalf("Function %v is not binary boolean", ag)
	}

	_, err := types.Less(va, vb)
	if err != nil {
		//Try to convert values.
		if va.Tid == types.IntID {
			va.Tid = types.FloatID
			va.Value = float64(va.Value.(int64))
		} else if vb.Tid == types.IntID {
			vb.Tid = types.FloatID
			vb.Value = float64(vb.Value.(int64))
		} else {
			return false, err
		}
	}
	isLess, err := types.Less(va, vb)
	if err != nil {
		return false, err
	}
	isMore, err := types.Less(vb, va)
	if err != nil {
		return false, err
	}
	isEqual, err := types.Equal(va, vb)
	if err != nil {
		return false, err
	}
	switch ag {
	case "<":
		return isLess, nil
	case ">":
		return isMore, nil
	case "<=":
		return isLess || isEqual, nil
	case ">=":
		return isMore || isEqual, nil
	case "==":
		return isEqual, nil
	case "!=":
		return !isEqual, nil
	}
	return false, errors.Errorf("Invalid compare function %q", ag)
}

func ApplyIntAdd(a, b, c *types.Val) error {
	c.Value = a.Value.(int64) + b.Value.(int64)
	return nil
}

func ApplyFloatAdd(a, b, c *types.Val) error {
	c.Value = a.Value.(float64) + b.Value.(float64)
	return nil
}

func ApplyOtherAdd(a, b, c *types.Val) error {
	return errors.Errorf("Wrong type encountered for func +")
}

func ApplyIntSub(a, b, c *types.Val) error {
	c.Value = a.Value.(int64) - b.Value.(int64)
	return nil
}

func ApplyFloatSub(a, b, c *types.Val) error {
	c.Value = a.Value.(float64) - b.Value.(float64)
	return nil
}

func ApplyOtherSub(a, b, c *types.Val) error {
	return errors.Errorf("Wrong type encountered for func -")
}

func ApplyIntMul(a, b, c *types.Val) error {
	c.Value = a.Value.(int64) * b.Value.(int64)
	return nil
}

func ApplyFloatMul(a, b, c *types.Val) error {
	c.Value = a.Value.(float64) * b.Value.(float64)
	return nil
}

func ApplyOtherMul(a, b, c *types.Val) error {
	return errors.Errorf("Wrong type encountered for func *")
}

func ApplyIntDiv(a, b, c *types.Val) error {
	if b.Value.(int64) == 0 {
		return errors.Errorf("Division by zero")
	}
	c.Value = a.Value.(int64) / b.Value.(int64)
	return nil
}

func ApplyFloatDiv(a, b, c *types.Val) error {
	if b.Value.(float64) == 0 {
		return errors.Errorf("Division by zero")
	}
	c.Value = a.Value.(float64) / b.Value.(float64)
	return nil
}

func ApplyOtherDiv(a, b, c *types.Val) error {
	return errors.Errorf("Wrong type encountered for func /")
}

func ApplyIntMod(a, b, c *types.Val) error {
	if b.Value.(int64) == 0 {
		return errors.Errorf("Division by zero")
	}
	c.Value = a.Value.(int64) % b.Value.(int64)
	return nil
}

func ApplyFloatMod(a, b, c *types.Val) error {
	if b.Value.(float64) == 0 {
		return errors.Errorf("Division by zero")
	}
	c.Value = math.Mod(a.Value.(float64), b.Value.(float64))
	return nil
}

func ApplyOtherMod(a, b, c *types.Val) error {
	return errors.Errorf("Wrong type encountered for func %s", "%")
}

func ApplyIntPow(a, b, c *types.Val) error {
	c.Value = math.Pow(float64(a.Value.(int64)), float64(b.Value.(int64)))
	c.Tid = types.FloatID
	return nil
}

func ApplyFloatPow(a, b, c *types.Val) error {
	c.Value = math.Pow(a.Value.(float64), b.Value.(float64))
	return nil
}

func ApplyOtherPow(a, b, c *types.Val) error {
	return errors.Errorf("Wrong type encountered for func ^")
}

func ApplyIntLog(a, b, c *types.Val) error {
	c.Value = math.Log(float64(a.Value.(int64))) / math.Log(float64(b.Value.(int64)))
	c.Tid = types.FloatID
	return nil
}

func ApplyFloatLog(a, b, c *types.Val) error {
	c.Value = math.Log(a.Value.(float64)) / math.Log(b.Value.(float64))
	return nil
}

func ApplyOtherLog(a, b, c *types.Val) error {
	return errors.Errorf("Wrong type encountered for func ln")
}

func ApplyMin(a, b, c *types.Val) error {
	r, err := types.Less(*a, *b)
	if err != nil {
		return err
	}
	if r {
		*c = *a
		return nil
	}
	*c = *b
	return nil
}

func ApplyMax(a, b, c *types.Val) error {
	r, err := types.Less(*a, *b)
	if err != nil {
		return err
	}
	if r {
		*c = *b
		return nil
	}
	*c = *a
	return nil
}

func ApplyLnInt(a, res *types.Val) error {
	res.Value = math.Log(float64(a.Value.(int64)))
	res.Tid = types.FloatID
	return nil
}

func ApplyLnFloat(a, res *types.Val) error {
	res.Value = math.Log(a.Value.(float64))
	return nil
}

func ApplyLnOther(a, res *types.Val) error {
	return errors.Errorf("Wrong type encountered for func ln")
}

func ApplyExpInt(a, res *types.Val) error {
	res.Value = math.Exp(float64(a.Value.(int64)))
	res.Tid = types.FloatID
	return nil
}

func ApplyExpFloat(a, res *types.Val) error {
	res.Value = math.Exp(a.Value.(float64))
	return nil
}

func ApplyExpOther(a, res *types.Val) error {
	return errors.Errorf("Wrong type encountered for func exp")
}

func ApplyNegInt(a, res *types.Val) error {
	res.Value = -a.Value.(int64)
	return nil
}

func ApplyNegFloat(a, res *types.Val) error {
	res.Value = -a.Value.(float64)
	return nil
}

func ApplyNegOther(a, res *types.Val) error {
	return errors.Errorf("Wrong type encountered for func u-")
}

func ApplySqrtInt(a, res *types.Val) error {
	res.Value = math.Sqrt(float64(a.Value.(int64)))
	res.Tid = types.FloatID
	return nil
}

func ApplySqrtFloat(a, res *types.Val) error {
	res.Value = math.Sqrt(a.Value.(float64))
	return nil
}

func ApplySqrtOther(a, res *types.Val) error {
	return errors.Errorf("Wrong type encountered for func sqrt")
}

func ApplyFloorInt(a, res *types.Val) error {
	res.Value = a.Value.(int64)
	return nil
}

func ApplyFloorFloat(a, res *types.Val) error {
	res.Value = math.Floor(a.Value.(float64))
	return nil
}

func ApplyFloorOther(a, res *types.Val) error {
	return errors.Errorf("Wrong type encountered for func floor")
}

func ApplyCeilInt(a, res *types.Val) error {
	res.Value = a.Value.(int64)
	return nil
}

func ApplyCeilFloat(a, res *types.Val) error {
	res.Value = math.Ceil(a.Value.(float64))
	return nil
}

func ApplyCeilOther(a, res *types.Val) error {
	return errors.Errorf("Wrong type encountered for fun ceil")
}

func ApplySince(a, res *types.Val) error {
	if a.Tid == types.DateTimeID {
		a.Value = float64(time.Since(a.Value.(time.Time))) / 1000000000.0
		a.Tid = types.FloatID
		*res = *a
		return nil
	}
	return errors.Errorf("Wrong type encountered for func since")
}

type MathUnaryFunction func(a, res *types.Val) error
type MathBinaryFunction func(a, b, res *types.Val) error

var supportedUnaryFunctions = map[string][]MathUnaryFunction{
	"ln":    {ApplyLnInt, ApplyLnFloat, ApplyLnOther},
	"exp":   {ApplyExpInt, ApplyExpFloat, ApplyExpOther},
	"u-":    {ApplyNegInt, ApplyNegFloat, ApplyNegOther},
	"sqrt":  {ApplySqrtInt, ApplySqrtFloat, ApplySqrtOther},
	"floor": {ApplyFloorInt, ApplyFloorFloat, ApplyFloorOther},
	"ceil":  {ApplyCeilInt, ApplyCeilFloat, ApplyCeilOther},
	"since": {ApplySince, ApplySince, ApplySince},
}

var supportedBinaryFunctions = map[string][]MathBinaryFunction{
	"+":       {ApplyIntAdd, ApplyFloatAdd, ApplyOtherAdd},
	"-":       {ApplyIntSub, ApplyFloatSub, ApplyOtherSub},
	"*":       {ApplyIntMul, ApplyFloatMul, ApplyOtherMul},
	"/":       {ApplyIntDiv, ApplyFloatDiv, ApplyOtherDiv},
	"%":       {ApplyIntMod, ApplyFloatMod, ApplyOtherMod},
	"pow":     {ApplyIntPow, ApplyFloatPow, ApplyOtherPow},
	"logbase": {ApplyIntLog, ApplyFloatLog, ApplyOtherLog},
	"min":     {ApplyMin, ApplyMin, ApplyMin},
	"max":     {ApplyMax, ApplyMax, ApplyMax},
}

type valType int

const (
	INT valType = iota
	FLOAT
	DEFAULT
)

func getValType(v *types.Val) valType {
	var vBase valType
	if v.Tid == types.IntID {
		vBase = INT
	} else if v.Tid == types.FloatID {
		vBase = FLOAT
	} else {
		vBase = DEFAULT
	}
	return vBase
}

func (ag *aggregator) MatchType(vBase, vaBase *valType, v, va *types.Val) error {
	if *vBase == *vaBase {
		return nil
	}

	if *vBase == DEFAULT || *vaBase == DEFAULT {
		return errors.Errorf("Wrong type encontered for func %s", ag.name)
	}

	// One of them is int and one is float
	if *vBase == INT {
		*vBase = FLOAT
		v.Tid = types.FloatID
		v.Value = float64(v.Value.(int64))
	}

	if *vaBase == INT {
		*vaBase = FLOAT
		va.Tid = types.FloatID
		va.Value = float64(va.Value.(int64))
	}

	return nil
}

func (ag *aggregator) ApplyVal(v types.Val) error {
	if v.Value == nil {
		// If the value is missing, treat it as 0.
		v.Value = int64(0)
		v.Tid = types.IntID
	}

	vBase := getValType(&v)

	var res types.Val
	if isUnary(ag.name) {
		res.Tid = v.Tid
		err := supportedUnaryFunctions[ag.name][vBase](&v, &res)
		if err != nil {
			return err
		}
		ag.result = res
		return nil
	}

	if ag.result.Value == nil {
		ag.result = v
		return nil
	}

	va := ag.result
	vaBase := getValType(&va)

	if err := ag.MatchType(&vBase, &vaBase, &v, &va); err != nil {
		return err
	}

	if function, ok := supportedBinaryFunctions[ag.name]; ok {
		res.Tid = va.Tid
		err := function[vaBase](&va, &v, &res)
		if err != nil {
			return err
		}
		ag.result = res
	} else {
		return errors.Errorf("Unhandled aggregator function %q", ag.name)
	}

	return nil
}

func (ag *aggregator) Apply(val types.Val) {
	if ag.result.Value == nil {
		ag.result = val
		ag.count++
		return
	}

	va := ag.result
	vb := val
	var res types.Val
	switch ag.name {
	case "min":
		r, err := types.Less(va, vb)
		if err == nil && !r {
			res = vb
		} else {
			res = va
		}
	case "max":
		r, err := types.Less(va, vb)
		if err == nil && r {
			res = vb
		} else {
			res = va
		}
	case "sum", "avg":
		if va.Tid == types.IntID && vb.Tid == types.IntID {
			va.Value = va.Value.(int64) + vb.Value.(int64)
		} else if va.Tid == types.FloatID && vb.Tid == types.FloatID {
			va.Value = va.Value.(float64) + vb.Value.(float64)
		}
		// Skipping the else case since that means the pair cannot be summed.
		res = va
	default:
		x.Fatalf("Unhandled aggregator function %v", ag.name)
	}
	ag.count++
	ag.result = res
}

func (ag *aggregator) ValueMarshalled() (*pb.TaskValue, error) {
	data := types.ValueForType(types.BinaryID)
	ag.divideByCount()
	res := &pb.TaskValue{ValType: ag.result.Tid.Enum(), Val: x.Nilbyte}
	if ag.result.Value == nil {
		return res, nil
	}
	// We'll divide it by the count if it's an avg aggregator.
	err := types.Marshal(ag.result, &data)
	if err != nil {
		return res, err
	}
	res.Val = data.Value.([]byte)
	return res, nil
}

func (ag *aggregator) divideByCount() {
	if ag.name != "avg" || ag.count == 0 || ag.result.Value == nil {
		return
	}
	var v float64
	if ag.result.Tid == types.IntID {
		v = float64(ag.result.Value.(int64))
	} else if ag.result.Tid == types.FloatID {
		v = ag.result.Value.(float64)
	}

	ag.result.Tid = types.FloatID
	ag.result.Value = v / float64(ag.count)
}

func (ag *aggregator) Value() (types.Val, error) {
	if ag.result.Value == nil {
		return ag.result, ErrEmptyVal
	}
	ag.divideByCount()
	if ag.result.Tid == types.FloatID {
		if math.IsInf(ag.result.Value.(float64), 1) {
			ag.result.Value = math.MaxFloat64
		} else if math.IsInf(ag.result.Value.(float64), -1) {
			ag.result.Value = -1 * math.MaxFloat64
		} else if math.IsNaN(ag.result.Value.(float64)) {
			ag.result.Value = 0.0
		}
	}
	return ag.result, nil
}
