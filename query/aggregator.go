package query

import (
	"github.com/dgraph-io/dgraph/protos/taskp"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

type aggregator struct {
	name   string
	typ    types.TypeID
	result types.Val
}

func convertTo(from *taskp.Value, typ types.TypeID) (types.Val, error) {
	vh, _ := getValue(from)
	va, err := types.Convert(vh, typ)
	if err != nil {
		return vh, x.Wrapf(err, "Fail to convert from taskp.Value to types.Val")
	}
	return va, err
}

func (ag *aggregator) Apply(val *taskp.Value) {
	if ag.result.Value == nil {
		ag.result, _ = convertTo(val, ag.typ)
		return
	}

	va := ag.result
	vb, err := convertTo(val, ag.typ)
	if err != nil {
		return
	}
	var res types.Val
	switch ag.name {
	case "min":
		r, err := types.Less(va, vb)
		if err == nil && r {
			res = va
		} else {
			res = vb
		}
	case "max":
		r, err := types.Less(va, vb)
		if err == nil && r {
			res = vb
		} else {
			res = va
		}
	case "sum":
		if ag.typ == types.Int32ID {
			va.Value = va.Value.(int32) + vb.Value.(int32)
		} else if ag.typ == types.FloatID {
			va.Value = va.Value.(float64) + vb.Value.(float64)
		}
		res = va
	default:
		return
	}
	ag.result = res
}

func (ag *aggregator) Value() (*taskp.Value, error) {
	data := types.ValueForType(types.BinaryID)
	if ag.result.Value == nil {
		return nil, nil
	}
	err := types.Marshal(ag.result, &data)
	if err != nil {
		return nil, err
	}
	res := &taskp.Value{ValType: int32(ag.typ), Val: data.Value.([]byte)}
	return res, nil
}
