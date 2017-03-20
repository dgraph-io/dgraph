package query

import (
	"github.com/dgraph-io/dgraph/protos/taskp"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

func convertTo(from *taskp.Value, typ types.TypeID) (types.Val, error) {
	vh, _ := getValue(from)
	va, err := types.Convert(vh, typ)
	if err != nil {
		return vh, x.Errorf("Fail to convert from taskp.Value to types.Val")
	}
	return va, err
}

func Aggregate(agrtr string, values []*taskp.Value, typ types.TypeID) (res *taskp.Value, rerr error) {
	// va is accumulated. if some error comes in accumulate, we keep va
	accumulate := func(va, vb types.Val) types.Val {
		switch agrtr {
		case "min":
			r, err := types.Less(va, vb)
			if err == nil && r {
				return va
			} else {
				return vb
			}
		case "max":
			r, err := types.Less(va, vb)
			if err == nil && r {
				return vb
			} else {
				return va
			}
		case "sum":
			if typ == types.Int32ID {
				va.Value = va.Value.(int32) + vb.Value.(int32)
			} else if typ == types.FloatID {
				va.Value = va.Value.(float64) + vb.Value.(float64)
			}
			return va
		default:
			return va
		}
	}

	res = &taskp.Value{ValType: int32(typ), Val: x.Nilbyte}
	result := types.Val{typ, x.Nilbyte}
	if len(values) == 0 {
		return res, rerr
	}
	result, rerr = convertTo(values[0], typ)
	if rerr != nil {
		return res, rerr
	}
	for i := 1; i < len(values); i++ {
		val := values[i]
		if len(val.Val) == 0 {
			continue
		}
		rval, err := convertTo(val, typ)
		if err != nil {
			continue
		}
		result = accumulate(result, rval)
	}

	data := types.ValueForType(types.BinaryID)
	rerr = types.Marshal(result, &data)
	if rerr != nil {
		return nil, x.Errorf("Failed aggregator(sum) during Marshal")
	}
	res.Val = data.Value.([]byte)

	return res, nil
}
