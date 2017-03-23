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
		if va.Tid == types.Int32ID && vb.Tid == types.Int32ID {
			va.Value = va.Value.(int32) + vb.Value.(int32)
		} else if va.Tid == types.FloatID && vb.Tid == types.FloatID {
			va.Value = va.Value.(float64) + vb.Value.(float64)
		} else if va.Tid == types.Int32ID && vb.Tid == types.FloatID {
			va.Value = float64(va.Value.(int32)) + vb.Value.(float64)
			va.Tid = types.FloatID
		} else if va.Tid == types.FloatID && vb.Tid == types.Int32ID {
			va.Value = va.Value.(float64) + float64(vb.Value.(int32))
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
	case "sum":
		if va.Tid == types.Int32ID && vb.Tid == types.Int32ID {
			va.Value = va.Value.(int32) + vb.Value.(int32)
		} else if va.Tid == types.FloatID && vb.Tid == types.FloatID {
			va.Value = va.Value.(float64) + vb.Value.(float64)
		} else {
			// This pair cannot be summed. So pass.
			log.Fatalf("Wrong arguments for Sum aggregator.")
		}
		res = va
	default:
		log.Fatalf("Unhandled aggregator function %v", ag.name)
	}
	ag.result = res
}

func (ag *aggregator) ValueMarshalled() (*taskp.Value, error) {
	data := types.ValueForType(types.BinaryID)
	res := &taskp.Value{ValType: int32(ag.result.Tid), Val: x.Nilbyte}
	if ag.result.Value == nil {
		return res, nil
	}
	err := types.Marshal(ag.result, &data)
	if err != nil {
		return res, err
	}
	res.Val = data.Value.([]byte)
	return res, nil
}

func (ag *aggregator) Value() types.Val {
	return ag.result
}
