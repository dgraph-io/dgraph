package worker

import (
	"github.com/dgraph-io/dgraph/protos/taskp"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

func CouldApplyAggregatorOn(agrtr string, typ types.TypeID) bool {
	if !typ.IsScalar() {
		return false
	}
	switch agrtr {
	case "min", "max":
		return (typ == types.Int32ID ||
			typ == types.FloatID ||
			typ == types.DateTimeID ||
			typ == types.StringID ||
			typ == types.DateID)
	case "sum":
		return (typ == types.Int32ID ||
			typ == types.FloatID)
	default:
		return false
	}
	return false
}

// getValue gets the value from the task.
func getValue(tv *taskp.Value) (types.Val, error) {
	vID := types.TypeID(tv.ValType)
	val := types.ValueForType(vID)
	val.Value = tv.Val
	return val, nil
}

func convertTo(from *taskp.Value, typ types.TypeID) (types.Val, error) {
	vh, _ := getValue(from)
	va, err := types.Convert(vh, typ)
	if err != nil {
		return vh, x.Errorf("Fail to convert from taskp.Value to types.Val")
	}
	return va, err
}

func Aggregate(agrtr string, values []*taskp.Value, typ types.TypeID) (*taskp.Value, error) {
	if !CouldApplyAggregatorOn(agrtr, typ) {
		return nil, x.Errorf("Cant apply aggregator %v on type %d\n", agrtr, typ)
	}

	// va is accumulated. if some error comes in accumulate, we keep va
	accumulate := func(va, vb types.Val) types.Val {
		switch agrtr {
		case "min":
			r, err := types.Less(va, vb)
			if err != nil || r {
				return va
			} else {
				return vb
			}
		case "max":
			r, err := types.Less(va, vb)
			if err != nil || !r {
				return va
			} else {
				return vb
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

	var lva, rva types.Val
	var err error
	result := &taskp.Value{ValType: int32(typ), Val: x.Nilbyte}
	for _, iter := range values {
		if len(iter.Val) == 0 {
			continue
		} else if len(result.Val) == 0 {
			if lva, err = convertTo(iter, typ); err == nil {
				result = iter
			}
			continue
		}
		if rva, err = convertTo(iter, typ); err != nil {
			continue
		}
		va := accumulate(lva, rva)
		if lva != va {
			result = iter
			lva = va
		}
	}

	if agrtr == "sum" && len(result.Val) > 0 {
		data := types.ValueForType(types.BinaryID)
		err = types.Marshal(lva, &data)
		if err != nil {
			return nil, x.Errorf("Failed aggregator(sum) during Marshal")
		}
		result.Val = data.Value.([]byte)
	}

	return result, nil
}
