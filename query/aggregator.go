/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package query

import (
	"bytes"
	"math"

	"github.com/dgraph-io/dgraph/protos/taskp"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

type aggregator struct {
	name   string
	result types.Val
	count  int // used when we need avergae.
}

func isUnary(f string) bool {
	return f == "log" || f == "exp"
}

func isBinaryBoolean(f string) bool {
	return f == "lt" || f == "gt" || f == "leq" || f == "geq" ||
		f == "eq"
}

func isTernary(f string) bool {
	return f == "conditional"
}

func isBinary(f string) bool {
	return f == "+" || f == "*" || f == "-"
}

func isMultiArgFunc(f string) bool {
	return f == "max" || f == "min"
}

func convertTo(from *taskp.Value) (types.Val, error) {
	vh, _ := getValue(from)
	if bytes.Equal(from.Val, x.Nilbyte) {
		return vh, ErrEmptyVal
	}
	va, err := types.Convert(vh, vh.Tid)
	if err != nil {
		return vh, x.Wrapf(err, "Fail to convert from taskp.Value to types.Val")
	}
	return va, err
}

func compareValues(ag string, va, vb types.Val) (bool, error) {
	if !isBinaryBoolean(ag) {
		x.Fatalf("Function %v is not binary boolean", ag)
	}

	isLess, err := types.Less(va, vb)
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
	isMore, err := types.Less(vb, va)
	if err != nil {
		return false, err
	}
	switch ag {
	case "lt":
		return isLess, nil
	case "gt":
		return isMore, nil
	case "leq":
		return isLess && !isMore, nil
	case "geq":
		return isMore && !isLess, nil
	case "eq":
		return !isMore && !isLess, nil
	default:
		x.Fatalf("Function %v not handled in compareValues", ag)
	}
	return false, nil
}

func (ag *aggregator) ApplyVal(v types.Val) error {
	if v.Value == nil {
		return ErrEmptyVal
	}

	var res types.Val
	if isUnary(ag.name) {
		switch ag.name {
		case "log":
			if v.Tid == types.IntID {
				v.Value = math.Log(float64(v.Value.(int64)))
				v.Tid = types.FloatID
			} else if v.Tid == types.FloatID {
				v.Value = math.Log(v.Value.(float64))
			}
			res = v
		case "exp":
			if v.Tid == types.IntID {
				v.Value = math.Exp(float64(v.Value.(int64)))
				v.Tid = types.FloatID
			} else if v.Tid == types.FloatID {
				v.Value = math.Exp(v.Value.(float64))
			}
			res = v
		}
		ag.result = res
		return nil
	}

	if ag.result.Value == nil {
		ag.result = v
		return nil
	}

	va := ag.result
	vb := v
	switch ag.name {
	case "+":
		if va.Tid == types.IntID && vb.Tid == types.IntID {
			va.Value = va.Value.(int64) + vb.Value.(int64)
		} else if va.Tid == types.FloatID && vb.Tid == types.FloatID {
			va.Value = va.Value.(float64) + vb.Value.(float64)
		} else if va.Tid == types.IntID && vb.Tid == types.FloatID {
			va.Value = float64(va.Value.(int64)) + vb.Value.(float64)
			va.Tid = types.FloatID
		} else if va.Tid == types.FloatID && vb.Tid == types.IntID {
			va.Value = va.Value.(float64) + float64(vb.Value.(int64))
		} else {
			// This pair cannot be aggregated. So pass.
		}
		res = va
	case "-":
		if va.Tid == types.IntID && vb.Tid == types.IntID {
			va.Value = va.Value.(int64) - vb.Value.(int64)
		} else if va.Tid == types.FloatID && vb.Tid == types.FloatID {
			va.Value = va.Value.(float64) - vb.Value.(float64)
		} else if va.Tid == types.IntID && vb.Tid == types.FloatID {
			va.Value = float64(va.Value.(int64)) - vb.Value.(float64)
			va.Tid = types.FloatID
		} else if va.Tid == types.FloatID && vb.Tid == types.IntID {
			va.Value = va.Value.(float64) - float64(vb.Value.(int64))
		} else {
			// This pair cannot be aggregated. So pass.
		}
		res = va
	case "*":
		if va.Tid == types.IntID && vb.Tid == types.IntID {
			va.Value = va.Value.(int64) * vb.Value.(int64)
		} else if va.Tid == types.FloatID && vb.Tid == types.FloatID {
			va.Value = va.Value.(float64) * vb.Value.(float64)
		} else if va.Tid == types.IntID && vb.Tid == types.FloatID {
			va.Value = float64(va.Value.(int64)) * vb.Value.(float64)
			va.Tid = types.FloatID
		} else if va.Tid == types.FloatID && vb.Tid == types.IntID {
			va.Value = va.Value.(float64) * float64(vb.Value.(int64))
		} else {
			// This pair cannot be aggregated. So pass.
		}
		res = va
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
	default:
		return x.Errorf("Unhandled aggregator function %v", ag.name)
	}
	ag.result = res
	return nil
}

func (ag *aggregator) Apply(val *taskp.Value) {
	if ag.result.Value == nil {
		v, err := convertTo(val)
		if err != nil {
			x.AssertTruef(err == ErrEmptyVal, "Expected Empty Val error. But got: %v", err)
			return
		}
		ag.result = v
		ag.count++
		return
	}

	va := ag.result
	vb, err := convertTo(val)
	if err != nil {
		x.AssertTruef(err == ErrEmptyVal, "Expected Empty Val error. But got: %v", err)
		return
	}
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
		} else {
			// This pair cannot be summed. So pass.
		}
		res = va
	default:
		x.Fatalf("Unhandled aggregator function %v", ag.name)
	}
	ag.count++
	ag.result = res
}

func (ag *aggregator) ValueMarshalled() (*taskp.Value, error) {
	data := types.ValueForType(types.BinaryID)
	ag.divideByCount()
	res := &taskp.Value{ValType: int32(ag.result.Tid), Val: x.Nilbyte}
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

func (ag *aggregator) Value() types.Val {
	ag.divideByCount()
	if ag.result.Tid == types.FloatID {
		if math.IsInf(ag.result.Value.(float64), 1) {
			ag.result.Value = math.MaxFloat64
		} else if math.IsInf(ag.result.Value.(float64), -1) {
			ag.result.Value = -1 * math.MaxFloat64
		}
	}
	return ag.result
}
