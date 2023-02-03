/*
 * Copyright 2017-2022 Dgraph Labs, Inc. and Contributors
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

	"github.com/pkg/errors"

	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
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
		switch {
		case va.Tid == types.IntID:
			va.Tid = types.FloatID
			va.Value = float64(va.Value.(int64))
		case vb.Tid == types.IntID:
			vb.Tid = types.FloatID
			vb.Value = float64(vb.Value.(int64))
		default:
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

func applyAdd(a, b, c *types.Val) error {
	vBase := getValType(a)
	switch vBase {
	case INT:
		aVal, bVal := a.Value.(int64), b.Value.(int64)

		if (aVal > 0 && bVal > math.MaxInt64-aVal) ||
			(aVal < 0 && bVal < math.MinInt64-aVal) {
			return ErrorIntOverflow
		}

		c.Value = aVal + bVal

	case FLOAT:
		c.Value = a.Value.(float64) + b.Value.(float64)

	case DEFAULT:
		return errors.Errorf("Wrong type %v encountered for func +", a.Tid)
	}
	return nil
}

func applySub(a, b, c *types.Val) error {
	vBase := getValType(a)
	switch vBase {
	case INT:
		aVal, bVal := a.Value.(int64), b.Value.(int64)

		if (bVal < 0 && aVal > math.MaxInt64+bVal) ||
			(bVal > 0 && aVal < math.MinInt64+bVal) {
			return ErrorIntOverflow
		}

		c.Value = aVal - bVal

	case FLOAT:
		c.Value = a.Value.(float64) - b.Value.(float64)

	case DEFAULT:
		return errors.Errorf("Wrong type %v encountered for func -", a.Tid)
	}
	return nil
}

func applyMul(a, b, c *types.Val) error {
	vBase := getValType(a)
	switch vBase {
	case INT:
		aVal, bVal := a.Value.(int64), b.Value.(int64)
		c.Value = aVal * bVal

		if aVal == 0 || bVal == 0 {
			return nil
		} else if c.Value.(int64)/bVal != aVal {
			return ErrorIntOverflow
		}

	case FLOAT:
		c.Value = a.Value.(float64) * b.Value.(float64)

	case DEFAULT:
		return errors.Errorf("Wrong type %v encountered for func *", a.Tid)
	}
	return nil
}

func applyDiv(a, b, c *types.Val) error {
	vBase := getValType(a)
	switch vBase {
	case INT:
		if b.Value.(int64) == 0 {
			return ErrorDivisionByZero
		}
		c.Value = a.Value.(int64) / b.Value.(int64)

	case FLOAT:
		if b.Value.(float64) == 0 {
			return ErrorDivisionByZero
		}
		c.Value = a.Value.(float64) / b.Value.(float64)

	case DEFAULT:
		return errors.Errorf("Wrong type %v encountered for func /", a.Tid)
	}
	return nil
}

func applyMod(a, b, c *types.Val) error {
	vBase := getValType(a)
	switch vBase {
	case INT:
		if b.Value.(int64) == 0 {
			return ErrorDivisionByZero
		}
		c.Value = a.Value.(int64) % b.Value.(int64)

	case FLOAT:
		if b.Value.(float64) == 0 {
			return ErrorDivisionByZero
		}
		c.Value = math.Mod(a.Value.(float64), b.Value.(float64))

	case DEFAULT:
		return errors.Errorf("Wrong type %v encountered for func %%", a.Tid)
	}
	return nil
}

func applyPow(a, b, c *types.Val) error {
	vBase := getValType(a)
	switch vBase {
	case INT:
		c.Value = math.Pow(float64(a.Value.(int64)), float64(b.Value.(int64)))
		c.Tid = types.FloatID

	case FLOAT:
		// Fractional power of -ve numbers should not be returned.
		if a.Value.(float64) < 0 &&
			math.Abs(math.Ceil(b.Value.(float64))-b.Value.(float64)) > 0 {
			return ErrorFractionalPower
		}
		c.Value = math.Pow(a.Value.(float64), b.Value.(float64))

	case DEFAULT:
		return errors.Errorf("Wrong type %v encountered for func ^", a.Tid)
	}
	return nil
}

func applyLog(a, b, c *types.Val) error {
	vBase := getValType(a)
	switch vBase {
	case INT:
		if a.Value.(int64) < 0 || b.Value.(int64) < 0 {
			return ErrorNegativeLog
		} else if b.Value.(int64) == 1 {
			return ErrorDivisionByZero
		}
		c.Value = math.Log(float64(a.Value.(int64))) / math.Log(float64(b.Value.(int64)))
		c.Tid = types.FloatID

	case FLOAT:
		if a.Value.(float64) < 0 || b.Value.(float64) < 0 {
			return ErrorNegativeLog
		} else if b.Value.(float64) == 1 {
			return ErrorDivisionByZero
		}
		c.Value = math.Log(a.Value.(float64)) / math.Log(b.Value.(float64))

	case DEFAULT:
		return errors.Errorf("Wrong type %v encountered for func log", a.Tid)
	}
	return nil
}

func applyMin(a, b, c *types.Val) error {
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

func applyMax(a, b, c *types.Val) error {
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

func applyLn(a, res *types.Val) error {
	vBase := getValType(a)
	switch vBase {
	case INT:
		if a.Value.(int64) < 0 {
			return ErrorNegativeLog
		}
		res.Value = math.Log(float64(a.Value.(int64)))
		res.Tid = types.FloatID

	case FLOAT:
		if a.Value.(float64) < 0 {
			return ErrorNegativeLog
		}
		res.Value = math.Log(a.Value.(float64))

	case DEFAULT:
		return errors.Errorf("Wrong type %v encountered for func ln", a.Tid)
	}
	return nil
}

func applyExp(a, res *types.Val) error {
	vBase := getValType(a)
	switch vBase {
	case INT:
		res.Value = math.Exp(float64(a.Value.(int64)))
		res.Tid = types.FloatID

	case FLOAT:
		res.Value = math.Exp(a.Value.(float64))

	case DEFAULT:
		return errors.Errorf("Wrong type %v encountered for func exp", a.Tid)
	}
	return nil
}

func applyNeg(a, res *types.Val) error {
	vBase := getValType(a)
	switch vBase {
	case INT:
		// -ve of math.MinInt64 is evaluated as itself (due to overflow)
		if a.Value.(int64) == math.MinInt64 {
			return ErrorIntOverflow
		}
		res.Value = -a.Value.(int64)

	case FLOAT:
		res.Value = -a.Value.(float64)

	case DEFAULT:
		return errors.Errorf("Wrong type %v encountered for func u-", a.Tid)
	}
	return nil
}

func applySqrt(a, res *types.Val) error {
	vBase := getValType(a)
	switch vBase {
	case INT:
		if a.Value.(int64) < 0 {
			return ErrorNegativeRoot
		}
		res.Value = math.Sqrt(float64(a.Value.(int64)))
		res.Tid = types.FloatID

	case FLOAT:
		if a.Value.(float64) < 0 {
			return ErrorNegativeRoot
		}
		res.Value = math.Sqrt(a.Value.(float64))

	case DEFAULT:
		return errors.Errorf("Wrong type %v encountered for func sqrt", a.Tid)
	}
	return nil
}

func applyFloor(a, res *types.Val) error {
	vBase := getValType(a)
	switch vBase {
	case INT:
		res.Value = a.Value.(int64)

	case FLOAT:
		res.Value = math.Floor(a.Value.(float64))

	case DEFAULT:
		return errors.Errorf("Wrong type %v encountered for func floor", a.Tid)
	}
	return nil
}

func applyCeil(a, res *types.Val) error {
	vBase := getValType(a)
	switch vBase {
	case INT:
		res.Value = a.Value.(int64)

	case FLOAT:
		res.Value = math.Ceil(a.Value.(float64))

	case DEFAULT:
		return errors.Errorf("Wrong type %v encountered for fun ceil", a.Tid)
	}
	return nil
}

func applySince(a, res *types.Val) error {
	if a.Tid == types.DateTimeID {
		a.Value = float64(time.Since(a.Value.(time.Time))) / 1000000000.0
		a.Tid = types.FloatID
		*res = *a
		return nil
	}
	return errors.Errorf("Wrong type %v encountered for func since", a.Tid)
}

type unaryFunc func(a, res *types.Val) error
type binaryFunc func(a, b, res *types.Val) error

var unaryFunctions = map[string]unaryFunc{
	"ln":    applyLn,
	"exp":   applyExp,
	"u-":    applyNeg,
	"sqrt":  applySqrt,
	"floor": applyFloor,
	"ceil":  applyCeil,
	"since": applySince,
}

var binaryFunctions = map[string]binaryFunc{
	"+":       applyAdd,
	"-":       applySub,
	"*":       applyMul,
	"/":       applyDiv,
	"%":       applyMod,
	"pow":     applyPow,
	"logbase": applyLog,
	"min":     applyMin,
	"max":     applyMax,
}

type valType int

const (
	INT valType = iota
	FLOAT
	DEFAULT
)

func getValType(v *types.Val) valType {
	var vBase valType
	switch v.Tid {
	case types.IntID:
		vBase = INT
	case types.FloatID:
		vBase = FLOAT
	default:
		vBase = DEFAULT
	}
	return vBase
}

func (ag *aggregator) matchType(v, va *types.Val) error {
	vBase := getValType(v)
	vaBase := getValType(va)
	if vBase == vaBase {
		return nil
	}

	if vBase == DEFAULT || vaBase == DEFAULT {
		return errors.Errorf("Wrong types %v, %v encontered for func %s", v.Tid,
			va.Tid, ag.name)
	}

	// One of them is int and one is float
	if vBase == INT {
		v.Tid = types.FloatID
		v.Value = float64(v.Value.(int64))
	}

	if vaBase == INT {
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

	var res types.Val
	if function, ok := unaryFunctions[ag.name]; ok {
		res.Tid = v.Tid
		err := function(&v, &res)
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
	if err := ag.matchType(&v, &va); err != nil {
		return err
	}

	if function, ok := binaryFunctions[ag.name]; ok {
		res.Tid = va.Tid
		err := function(&va, &v, &res)
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
		switch {
		case va.Tid == types.IntID && vb.Tid == types.IntID:
			va.Value = va.Value.(int64) + vb.Value.(int64)
		case va.Tid == types.FloatID && vb.Tid == types.FloatID:
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
	switch ag.result.Tid {
	case types.IntID:
		v = float64(ag.result.Value.(int64))
	case types.FloatID:
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
		switch {
		case math.IsInf(ag.result.Value.(float64), 1):
			ag.result.Value = math.MaxFloat64
		case math.IsInf(ag.result.Value.(float64), -1):
			ag.result.Value = -1 * math.MaxFloat64
		case math.IsNaN(ag.result.Value.(float64)):
			ag.result.Value = 0.0
		}
	}
	return ag.result, nil
}
