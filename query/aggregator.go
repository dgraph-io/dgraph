/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package query

import (
	"bytes"
	"math"
	"math/big"
	"time"

	"github.com/pkg/errors"

	"github.com/hypermodeinc/dgraph/v25/protos/pb"
	"github.com/hypermodeinc/dgraph/v25/types"
	"github.com/hypermodeinc/dgraph/v25/x"
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
		f == "max" || f == "min" || f == "logbase" || f == "pow" ||
		f == "dot"
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
	case VFLOAT:
		// When adding vectors of floats, we add then item-wise
		// so that c.Value[i] = a.Value[i] + b.Value[i] for all i
		// in range. If lengths of a and b are different, we treat
		// this as an error.
		aVal := a.Value.([]float32)
		bVal, ok := b.Value.([]float32)
		if !ok {
			return ErrorArgsDisagree
		}
		if len(aVal) != len(bVal) {
			return ErrorVectorsNotMatch
		}
		cVal := make([]float32, len(aVal))
		for i := range aVal {
			cVal[i] = aVal[i] + bVal[i]
		}
		c.Value = cVal

	case BIGFLOAT:
		aVal, bVal := a.Value.(big.Float), b.Value.(big.Float)
		aVal.Add(&aVal, &bVal)
		c.Value = aVal

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
	case VFLOAT:
		// When subtracting vectors of floats, we add then item-wise
		// so that c.Value[i] = a.Value[i] - b.Value[i] for all i
		// in range. If lengths of a and b are different, we treat
		// this as an error.
		aVal := a.Value.([]float32)
		bVal, ok := b.Value.([]float32)
		if !ok {
			return ErrorArgsDisagree
		}
		if len(aVal) != len(bVal) {
			return ErrorVectorsNotMatch
		}
		cVal := make([]float32, len(aVal))
		for i := range aVal {
			cVal[i] = aVal[i] - bVal[i]
		}
		c.Value = cVal

	case BIGFLOAT:
		aVal, bVal := a.Value.(big.Float), b.Value.(big.Float)
		aVal.Sub(&aVal, &bVal)
		c.Value = aVal

	case DEFAULT:
		return errors.Errorf("Wrong type %v encountered for func -", a.Tid)
	}
	return nil
}

func applyMul(a, b, c *types.Val) error {
	// Possible input combinations:
	//   INT * INT
	//   FLOAT * FLOAT
	//   FLOAT * VFLOAT
	//   VFLOAT * FLOAT
	// Other combinations should have been eliminated via matchType.
	// Some operations, such as INT * FLOAT might be allowed conceptually,
	// but we would have already cast the INT value to a FLOAT in the
	// matchType invocation.
	lValType := getValType(a)
	rValType := getValType(b)
	switch lValType {
	case INT:
		aVal, bVal := a.Value.(int64), b.Value.(int64)
		c.Tid = types.IntID
		c.Value = aVal * bVal
		if aVal == 0 || bVal == 0 {
			return nil
		} else if c.Value.(int64)/bVal != aVal {
			return ErrorIntOverflow
		}

	case FLOAT:
		aVal := a.Value.(float64)
		switch rValType {
		case FLOAT:
			c.Tid = types.FloatID
			c.Value = aVal * b.Value.(float64)
		case VFLOAT:
			bVal := b.Value.([]float32)
			cVal := make([]float32, len(bVal))
			for i := range bVal {
				cVal[i] = float32(aVal) * bVal[i]
			}
			c.Value = cVal
			c.Tid = types.VFloatID
		default:
			return invalidTypeError(lValType, rValType, "*")
		}

	case VFLOAT:
		aVal := a.Value.([]float32)
		if rValType != FLOAT {
			return invalidTypeError(lValType, rValType, "*")
		}
		bVal := b.Value.(float64)

		cVal := make([]float32, len(aVal))
		c.Value = cVal
		// If you convert from float64 to float32, sometimes we can get inf.
		if math.IsInf(float64(float32(bVal)), 0) {
			return ErrorFloat32Overflow
		}
		for i := range aVal {
			cVal[i] = aVal[i] * float32(bVal)
		}
		c.Tid = types.VFloatID

	case BIGFLOAT:
		aVal, bVal := a.Value.(big.Float), b.Value.(big.Float)
		aVal.Mul(&aVal, &bVal)
		c.Value = aVal

	case DEFAULT:
		return invalidTypeError(lValType, rValType, "*")
	}
	return nil
}

func applyDiv(a, b, c *types.Val) error {
	// Possible types (after having been filtered by matchType):
	// INT / INT
	// FLOAT / FLOAT
	// VFLOAT / FLOAT
	// We assume that this filtering has already occurred.
	numeratorType := getValType(a)
	switch numeratorType {
	case INT:
		denom := b.Value.(int64)
		if denom == 0 {
			return ErrorDivisionByZero
		}
		c.Value = a.Value.(int64) / denom
		c.Tid = types.IntID
	case FLOAT:
		denom := b.Value.(float64)
		if denom == 0 {
			return ErrorDivisionByZero
		}
		c.Value = a.Value.(float64) / denom
		c.Tid = types.FloatID
	case VFLOAT:
		denom := b.Value.(float64)
		if denom == 0 {
			return ErrorDivisionByZero
		}
		// If you convert from float64 to float32, sometimes we can get inf.
		if math.IsInf(float64(float32(denom)), 0) {
			return ErrorFloat32Overflow
		}
		aVal := a.Value.([]float32)
		cVal := make([]float32, len(aVal))
		for i := range aVal {
			cVal[i] = aVal[i] / float32(denom)
		}
		c.Value = cVal
		c.Tid = types.VFloatID
	case BIGFLOAT:
		aVal, bVal := a.Value.(big.Float), b.Value.(big.Float)
		var zero big.Float
		if bVal.Cmp(&zero) == 0 {
			return ErrorDivisionByZero
		}
		aVal.Quo(&aVal, &bVal)
		c.Value = aVal
	case DEFAULT:
		return invalidTypeError(numeratorType, getValType(b), "/")
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

func applyDot(a, b, c *types.Val) error {
	if getValType(a) != VFLOAT || getValType(b) != VFLOAT {
		return invalidTypeError(getValType(a), getValType(b), "dot")
	}
	aVal := a.Value.([]float32)
	bVal := b.Value.([]float32)
	if len(aVal) != len(bVal) {
		return ErrorVectorsNotMatch
	}
	c.Tid = types.FloatID
	var cVal float64
	for i := range aVal {
		cVal += (float64)(aVal[i]) * (float64)(bVal[i])
	}
	c.Value = cVal
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
		res.Tid = types.IntID

	case FLOAT:
		res.Value = -a.Value.(float64)
		res.Tid = types.FloatID
	case VFLOAT:
		aVal := a.Value.([]float32)
		resVal := make([]float32, len(aVal))
		for i, v := range aVal {
			resVal[i] = -v
		}
		res.Value = resVal
		res.Tid = types.VFloatID

	case BIGFLOAT:
		value := a.Value.(big.Float)
		neg := big.NewFloat(0).SetPrec(types.BigFloatPrecision).Neg(&value)
		res.Value = *neg

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

	case BIGFLOAT:
		value := a.Value.(big.Float)
		rt := big.NewFloat(0).SetPrec(types.BigFloatPrecision).Sqrt(&value)
		res.Value = *rt

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

	case BIGFLOAT:
		value := a.Value.(big.Float)
		f, _ := value.Float64()
		res.Value = *big.NewFloat(math.Floor(f)).SetPrec(types.BigFloatPrecision)

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

	case BIGFLOAT:
		value := a.Value.(big.Float)
		f, _ := value.Float64()
		res.Value = *big.NewFloat(math.Ceil(f)).SetPrec(types.BigFloatPrecision)

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
	"dot":     applyDot,
}

// mixedScalarVectOps enumerates the binary functions that allow for
// one argument to be a vector and the other a scalar.
// In fact, if one of the arguments is a vector then the other *must* be
// a scalar!
var mixedScalarVectOps = map[string]struct{}{
	"*": {},
	"/": {},
}

var opsAllowingVectorsOnRight = map[string]struct{}{
	"u-":  {},
	"+":   {},
	"-":   {},
	"*":   {},
	"dot": {},
}

type valType int

const (
	INT valType = iota
	FLOAT
	VFLOAT
	DEFAULT
	BIGFLOAT
)

func getValType(v *types.Val) valType {
	var vBase valType
	switch v.Tid {
	case types.IntID:
		vBase = INT
	case types.FloatID:
		vBase = FLOAT
	case types.VFloatID:
		vBase = VFLOAT
	case types.BigFloatID:
		vBase = BIGFLOAT
	default:
		vBase = DEFAULT
	}
	return vBase
}

func invalidTypeError(left, right valType, funcName string) error {
	return errors.Errorf("invalid types %v, %v for func %s", left, right, funcName)
}

// matchType(left, right) will make sure that the left and right type
// arguments agree, and otherwise convert left/right to an appropriate
// type in some cases where it can.
// matchType is invoked right before evaluating either a unary or binary
// function indicated by ag.name. If evaluating a unary function, then
// right is actually the single argument type, and left is actually the
// resulting type. For invoking a binary function then left and right
// of course play the role of being the left/right types (as expected).
func (ag *aggregator) matchType(left, right *types.Val) error {
	leftType := getValType(left)
	rightType := getValType(right)

	if rightType == VFLOAT {
		if _, ok := opsAllowingVectorsOnRight[ag.name]; !ok {
			return invalidTypeError(leftType, rightType, ag.name)
		}
	}

	if leftType == rightType {
		if leftType == VFLOAT {
			if _, found := mixedScalarVectOps[ag.name]; found {
				return invalidTypeError(leftType, rightType, ag.name)
			}
		}
		return nil
	}

	if leftType == DEFAULT || rightType == DEFAULT {
		return invalidTypeError(leftType, rightType, ag.name)
	}

	// We can assume at this point that left and right do not match and
	// are either INT, FLOAT, or VFLOAT.
	switch leftType {
	case INT:
		if rightType == VFLOAT {
			if _, found := mixedScalarVectOps[ag.name]; !found {
				return invalidTypeError(leftType, rightType, ag.name)
			}
		} else if rightType == BIGFLOAT {
			var bf big.Float
			left.Value = bf.SetPrec(types.BigFloatPrecision).SetInt64(left.Value.(int64))
			left.Tid = types.BigFloatID
		}
		// rightType must be either FLOAT or VFLOAT. In either case, we
		// must cast the left type to a FLOAT.
		left.Tid = types.FloatID
		left.Value = float64(left.Value.(int64))
	case FLOAT:
		if rightType == VFLOAT {
			if _, found := mixedScalarVectOps[ag.name]; !found {
				return invalidTypeError(leftType, rightType, ag.name)
			}
		} else if rightType == BIGFLOAT {
			var bf big.Float
			left.Value = bf.SetPrec(types.BigFloatPrecision).SetFloat64(left.Value.(float64))
			left.Tid = types.BigFloatID
		} else {
			// We can assume here: rightType == INT
			right.Tid = types.FloatID
			right.Value = float64(right.Value.(int64))
		}
	case VFLOAT:
		if rightType == INT {
			right.Tid = types.FloatID
			right.Value = float64(right.Value.(int64))
		}
	case BIGFLOAT:
		if rightType == FLOAT {
			var bf big.Float
			right.Value = *bf.SetPrec(types.BigFloatPrecision).SetFloat64(right.Value.(float64))
			right.Tid = types.BigFloatID
		} else if rightType == INT {
			var bf big.Float
			right.Value = *bf.SetPrec(types.BigFloatPrecision).SetInt64(right.Value.(int64))
			right.Tid = types.BigFloatID
		}

		// We can assume that if rightType is not INT then it must
		// be FLOAT, in which case, there is no further step needed.
	default:
		// This should be unreachable.
		return invalidTypeError(leftType, rightType, ag.name)
	}

	return nil
}

// ag.ApplyVal(v) evaluates the function indicated by
// ag.name and places result of evaluation into ag.result.
// If ag.name indicates a single argument function then
// this evaluation is tantamount to:
//
//	ag.result = unaryFunc(v)
//
// If however, ag.name indicates a binary function, then this
// evaluates to:
//
//	ag.result = binaryFunc(ag.result, v)
//
// In other words, for the binary result will replace the prior
// value of ag.result.
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

	left := ag.result
	if err := ag.matchType(&left, &v); err != nil {
		return err
	}

	if function, ok := binaryFunctions[ag.name]; ok {
		res.Tid = left.Tid
		err := function(&left, &v, &res)
		if err != nil {
			return err
		}
		ag.result = res
	} else {
		return errors.Errorf("Unhandled aggregator function %q", ag.name)
	}

	return nil
}

func (ag *aggregator) Apply(val types.Val) error {
	if ag.result.Value == nil {
		if val.Tid == types.VFloatID {
			// Copy array if it's VFloat, otherwise we overwrite value.
			va := val.Value.([]float32)
			res := make([]float32, len(va))
			copy(res, va)
			ag.result = types.Val{Tid: types.VFloatID, Value: res}
		} else {
			ag.result = val
		}
		ag.count++
		return nil
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
		case va.Tid == types.VFloatID && vb.Tid == types.VFloatID:
			accumVal := va.Value.([]float32)
			bVal := vb.Value.([]float32)
			if len(bVal) != len(accumVal) {
				return ErrorVectorsNotMatch
			}
			for i := range bVal {
				accumVal[i] += bVal[i]
			}
		case va.Tid == types.BigFloatID && vb.Tid == types.BigFloatID:
			lhs := va.Value.(big.Float)
			rhs := vb.Value.(big.Float)
			va.Value = *new(big.Float).SetPrec(types.BigFloatPrecision).Add(&lhs, &rhs)
		}
		res = va
	default:
		x.Fatalf("Unhandled aggregator function %v", ag.name)
	}
	ag.count++
	ag.result = res
	return nil
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

	if ag.result.Tid == types.BigFloatID {
		val := ag.result.Value.(big.Float)
		val.Quo(&val, new(big.Float).SetPrec(types.BigFloatPrecision).SetInt64(int64(ag.count)))
		ag.result.Tid = types.BigFloatID
		ag.result.Value = val
		return
	}

	var v float64
	switch ag.result.Tid {
	case types.IntID:
		v = float64(ag.result.Value.(int64))
	case types.FloatID:
		v = ag.result.Value.(float64)
	case types.VFloatID:
		arr := ag.result.Value.([]float32)
		res := make([]float32, len(arr))
		for i := range arr {
			res[i] = arr[i] / float32(ag.count)
		}
		ag.result.Value = res
		return
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
