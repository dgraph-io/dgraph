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
	"time"

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
	isLess, err = types.Less(va, vb)
	isMore, err := types.Less(vb, va)
	if err != nil {
		return false, err
	}
	switch ag {
	case "<":
		return isLess, nil
	case ">":
		return isMore, nil
	case "<=":
		return isLess && !isMore, nil
	case ">=":
		return isMore && !isLess, nil
	case "==":
		return !isMore && !isLess, nil
	case "!=":
		return isMore || isLess, nil
	default:
		return false, x.Errorf("Invalid compare function %v", ag)
	}
	return false, nil
}

func (ag *aggregator) ApplyVal(v types.Val) error {
	if v.Value == nil {
		return nil
	}

	var isIntOrFloat bool
	var l float64
	if v.Tid == types.IntID {
		l = float64(v.Value.(int64))
		v.Value = l
		v.Tid = types.FloatID
		isIntOrFloat = true
	} else if v.Tid == types.FloatID {
		l = v.Value.(float64)
		isIntOrFloat = true
	}
	// If its not int or float, keep the type.

	var res types.Val
	if isUnary(ag.name) {
		switch ag.name {
		case "ln":
			if !isIntOrFloat {
				return x.Errorf("Wrong type encountered for func %v", ag.name)
			}
			v.Value = math.Log(l)
			res = v
		case "exp":
			if !isIntOrFloat {
				return x.Errorf("Wrong type encountered for func %v", ag.name)
			}
			v.Value = math.Exp(l)
			res = v
		case "u-":
			if !isIntOrFloat {
				return x.Errorf("Wrong type encountered for func %v", ag.name)
			}
			v.Value = -l
			res = v
		case "sqrt":
			if !isIntOrFloat {
				return x.Errorf("Wrong type encountered for func %v", ag.name)
			}
			v.Value = math.Sqrt(l)
			res = v
		case "floor":
			if !isIntOrFloat {
				return x.Errorf("Wrong type encountered for func %v", ag.name)
			}
			v.Value = math.Floor(l)
			res = v
		case "ceil":
			if !isIntOrFloat {
				return x.Errorf("Wrong type encountered for func %v", ag.name)
			}
			v.Value = math.Ceil(l)
			res = v
		case "since":
			if v.Tid == types.DateID {
				v.Value = float64(time.Since(v.Value.(time.Time))) / 1000000000.0
				v.Tid = types.FloatID
			} else if v.Tid == types.DateTimeID {
				v.Value = float64(time.Since(v.Value.(time.Time))) / 1000000000.0
				v.Tid = types.FloatID
			} else {
				return x.Errorf("Wrong type encountered for func %v", ag.name)
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
	if va.Tid != types.IntID && va.Tid != types.FloatID {
		isIntOrFloat = false
	}
	switch ag.name {
	case "+":
		if !isIntOrFloat {
			return x.Errorf("Wrong type encountered for func %v", ag.name)
		}
		va.Value = va.Value.(float64) + l
		res = va
	case "-":
		if !isIntOrFloat {
			return x.Errorf("Wrong type encountered for func %v", ag.name)
		}
		va.Value = va.Value.(float64) - l
		res = va
	case "*":
		if !isIntOrFloat {
			return x.Errorf("Wrong type encountered for func %v", ag.name)
		}
		va.Value = va.Value.(float64) * l
		res = va
	case "/":
		if !isIntOrFloat {
			return x.Errorf("Wrong type encountered for func %v %v %v", ag.name, va.Tid, v.Tid)
		}
		va.Value = va.Value.(float64) / l
		res = va
	case "%":
		if !isIntOrFloat {
			return x.Errorf("Wrong type encountered for func %v", ag.name)
		}
		va.Value = math.Mod(va.Value.(float64), l)
		res = va
	case "pow":
		if !isIntOrFloat {
			return x.Errorf("Wrong type encountered for func %v", ag.name)
		}
		va.Value = math.Pow(va.Value.(float64), l)
		res = va
	case "logbase":
		if l == 1 {
			return nil
		}
		if !isIntOrFloat {
			return x.Errorf("Wrong type encountered for func %v", ag.name)
		}
		va.Value = math.Log(va.Value.(float64)) / math.Log(l)
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
		return x.Errorf("Unhandled aggregator function %v", ag.name)
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

func (ag *aggregator) Value() (types.Val, error) {
	ag.divideByCount()
	if ag.result.Tid == types.FloatID {
		if math.IsInf(ag.result.Value.(float64), 1) {
			ag.result.Value = math.MaxFloat64
		} else if math.IsInf(ag.result.Value.(float64), -1) {
			ag.result.Value = -1 * math.MaxFloat64
		} else if math.IsNaN(ag.result.Value.(float64)) {
			return ag.result, x.Errorf("Invalid math operation. Produced NaN")
		}
	}
	return ag.result, nil
}
