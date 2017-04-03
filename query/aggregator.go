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
	"log"

	"github.com/dgraph-io/dgraph/protos/taskp"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

type aggregator struct {
	name   string
	result types.Val
	count  int // used when we need avergae.
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

func (ag *aggregator) ApplyVal(v types.Val) error {
	if v.Value == nil {
		return ErrEmptyVal
	}
	if ag.result.Value == nil {
		ag.result = v
		return nil
	}

	va := ag.result
	vb := v
	var res types.Val
	switch ag.name {
	case "sumvar":
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
			// This pair cannot be summed. So pass.
			log.Fatalf("Wrong arguments for Sum aggregator.")
		}
		res = va
	case "mulvar":
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
			// This pair cannot be summed. So pass.
			log.Fatalf("Wrong arguments for Sum aggregator.")
		}
		res = va
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
	return ag.result
}
