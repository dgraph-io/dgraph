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
	"math/big"
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

type valType int

const (
	DEFAULT valType = iota
	FLOAT
	BIGFLOAT
)

func (ag *aggregator) ApplyVal(v types.Val) error {
	if v.Value == nil {
		// If the value is missing, treat it as 0.
		v.Value = int64(0)
		v.Tid = types.IntID
	}

	var vBase valType
	var valFloat float64
	if v.Tid == types.IntID {
		valFloat = float64(v.Value.(int64))
		v.Value = valFloat
		v.Tid = types.FloatID
		vBase = FLOAT
	} else if v.Tid == types.FloatID {
		valFloat = v.Value.(float64)
		vBase = FLOAT
	} else if v.Tid == types.BigFloatID {
		vBase = BIGFLOAT
	}
	// If its not int or float, keep the type.

	if isUnary(ag.name) {
		return ag.ApplyUnaryFunction(v, vBase, valFloat)
	}

	if ag.result.Value == nil {
		ag.result = v
		return nil
	}

	va := ag.result
	var valBigFloat big.Float
	var vaType valType
	if va.Tid == types.FloatID {
		vaType = FLOAT
		if vBase == BIGFLOAT {
			vaType = BIGFLOAT
			va.Tid = types.BigFloatID
			va.Value = *big.NewFloat(valFloat).SetPrec(types.BigFloatPrecision)
		}
	}
	if va.Tid == types.BigFloatID {
		vaType = BIGFLOAT
		if vBase == FLOAT {
			valBigFloat.SetPrec(types.BigFloatPrecision).SetFloat64(valFloat)
			vBase = BIGFLOAT
		} else if vBase == BIGFLOAT {
			valBigFloat = v.Value.(big.Float)
		}
	}

	if vBase != vaType {
		return errors.Errorf("Different type encountered for binary func %q", ag.name)
	}

	return ag.ApplyBinaryFunction(v, va, vBase, valFloat, valBigFloat)
}

func (ag *aggregator) ApplyUnaryFunction(v types.Val, vBase valType,
	valFloat float64) error {
	var res types.Val
	switch ag.name {
	case "ln":
		if vBase != FLOAT {
			return errors.Errorf("Wrong type encountered for func %q", ag.name)
		}
		v.Value = math.Log(valFloat)
		res = v
	case "exp":
		if vBase != FLOAT {
			return errors.Errorf("Wrong type encountered for func %q", ag.name)
		}
		v.Value = math.Exp(valFloat)
		res = v
	case "u-":
		//This function reverses the sign of the value.
		if vBase == DEFAULT {
			return errors.Errorf("Wrong type encountered for func %q", ag.name)
		}

		if vBase == BIGFLOAT {
			value := v.Value.(big.Float)
			neg := big.NewFloat(0).SetPrec(types.BigFloatPrecision).Neg(&value)
			v.Value = *neg
		} else {
			v.Value = -valFloat
		}

		res = v
	case "sqrt":
		if vBase == DEFAULT {
			return errors.Errorf("Wrong type encountered for func %q", ag.name)
		}

		if vBase == BIGFLOAT {
			value := v.Value.(big.Float)
			res := big.NewFloat(0).SetPrec(types.BigFloatPrecision).Sqrt(&value)
			v.Value = *res
		} else {
			v.Value = math.Sqrt(valFloat)
		}

		res = v
	case "floor":
		if vBase == DEFAULT {
			return errors.Errorf("Wrong type encountered for func %q", ag.name)
		}

		if vBase == BIGFLOAT {
			value := v.Value.(big.Float)
			valFloat, _ = value.Float64()
			v.Value = *big.NewFloat(math.Floor(valFloat)).
				SetPrec(types.BigFloatPrecision)
		} else {
			v.Value = math.Floor(valFloat)
		}

		res = v
	case "ceil":
		if vBase == DEFAULT {
			return errors.Errorf("Wrong type encountered for func %q", ag.name)
		}

		if vBase == BIGFLOAT {
			value := v.Value.(big.Float)
			res, _ := value.Float64()
			v.Value = *big.NewFloat(math.Ceil(res)).
				SetPrec(types.BigFloatPrecision)
		} else {
			v.Value = math.Ceil(valFloat)
		}

		res = v
	case "since":
		if v.Tid == types.DateTimeID {
			v.Value = float64(time.Since(v.Value.(time.Time))) / 1000000000.0
			v.Tid = types.FloatID
		} else {
			return errors.Errorf("Wrong type encountered for func %q", ag.name)
		}
		res = v
	}
	ag.result = res
	return nil
}

func (ag *aggregator) ApplyBinaryFunction(v, va types.Val, vBase valType,
	valFloat float64, valBigFloat big.Float) error {
	var zero big.Float
	var res types.Val
	divisionError := errors.Errorf("Division by zero")
	switch ag.name {
	case "+":
		if vBase == DEFAULT {
			return errors.Errorf("Wrong type encountered for func %q", ag.name)
		}

		if vBase == BIGFLOAT {
			value := va.Value.(big.Float)
			value.Add(&value, &valBigFloat)
			va.Value = value
		} else {
			va.Value = va.Value.(float64) + valFloat
		}
		res = va
	case "-":
		if vBase == DEFAULT {
			return errors.Errorf("Wrong type encountered for func %q", ag.name)
		}

		if vBase == BIGFLOAT {
			value := va.Value.(big.Float)
			value.Sub(&value, &valBigFloat)
			va.Value = value
		} else {
			va.Value = va.Value.(float64) - valFloat
		}
		res = va
	case "*":
		if vBase == DEFAULT {
			return errors.Errorf("Wrong type encountered for func %q", ag.name)
		}

		if vBase == BIGFLOAT {
			value := va.Value.(big.Float)
			value.Mul(&value, &valBigFloat)
			va.Value = value
		} else {
			va.Value = va.Value.(float64) * valFloat
		}
		res = va
	case "/":
		if vBase == DEFAULT {
			return errors.Errorf("Wrong type encountered for func %q %q", ag.name, va.Tid)
		}

		if vBase == BIGFLOAT {
			if valBigFloat.Cmp(&zero) == 0 {
				return divisionError
			}
			value := va.Value.(big.Float)
			value.Quo(&value, &valBigFloat)
			va.Value = value
		} else {
			if valFloat == 0 {
				return divisionError
			}
			va.Value = va.Value.(float64) / valFloat
		}
		res = va
	case "%":
		if vBase != FLOAT {
			return errors.Errorf("Wrong type encountered for func %q", ag.name)
		}

		if valFloat == 0 {
			return divisionError
		}
		va.Value = math.Mod(va.Value.(float64), valFloat)
		res = va
	case "pow":
		if vBase != FLOAT {
			return errors.Errorf("Wrong type encountered for func %q", ag.name)
		}
		va.Value = math.Pow(va.Value.(float64), valFloat)
		res = va
	case "logbase":
		if vBase != FLOAT {
			return errors.Errorf("Wrong type encountered for func %q", ag.name)
		}
		if valFloat == 1 {
			return nil
		}
		va.Value = math.Log(va.Value.(float64)) / math.Log(valFloat)
		res = va
	case "min":
		r, err := types.Less(va, v)
		if err == nil && !r {
			res = v
		} else {
			res = va
		}
	case "max":
		r, err := types.Less(va, v)
		if err == nil && r {
			res = v
		} else {
			res = va
		}
	default:
		return errors.Errorf("Unhandled aggregator function %q", ag.name)
	}
	ag.result = res
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
		} else if va.Tid == types.BigFloatID && vb.Tid == types.BigFloatID {
			lhs := va.Value.(big.Float)
			rhs := vb.Value.(big.Float)
			va.Value = *new(big.Float).SetPrec(types.BigFloatPrecision).Add(&lhs, &rhs)
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

	if ag.result.Tid == types.BigFloatID {
		val := ag.result.Value.(big.Float)
		val.Quo(&val, new(big.Float).SetPrec(types.BigFloatPrecision).SetInt64(int64(ag.count)))
		ag.result.Tid = types.BigFloatID
		ag.result.Value = val
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
