
package worker

import (
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)


func CouldApplyAgrtrOn(agrtr, attr string) bool {
	if agrtr == "count" {
		return true
	}
	typ, err := schema.TypeOf(attr)
	if err != nil {
		return false
	}
	return couldApplyAgrtrOn(agrtr, typ)
}

func couldApplyAgrtrOn(agrtr string, typ types.TypeID) bool {
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
func getValue(tv *task.Value) (types.Val, error) {
	vID := types.TypeID(tv.ValType)
	val := types.ValueForType(vID)
	val.Value = tv.Val
	return val, nil
}

func convertTo(from *task.Value, typ types.TypeID) (types.Val, error) {
	vh, _ := getValue(from)
	va, err := types.Convert(vh, typ)
	if err != nil {
		return vh, x.Errorf("Fail to convert from task.Value to types.Val")
	}
	return va, err
}

func Aggregate(agrtr string, values []*task.Value, typ types.TypeID) (*task.Value, error) {
	if !couldApplyAgrtrOn(agrtr, typ) {
		return nil, x.Errorf("Cant apply aggregator %v on type %d\n", agrtr, typ)
	}
	
	accumulate := func(va, vb types.Val) (types.Val) {
		switch agrtr {
		case "min":
			if types.Less(va, vb) {
				return va
			} else {
				return vb
			}
		case "max":
			if types.Less(va, vb) {
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
	
	var lva, rva types.Val
	var err error
	result := &task.Value{ValType: int32(typ), Val:x.Nilbyte}
	for _, iter := range values {
		if len(iter.Val) == 0 {
			continue
		} else if len(result.Val) == 0 {
			result = iter
			if lva, err = convertTo(iter, typ); err != nil {
				return result, err
			}
			continue
		}
		if rva, err = convertTo(iter, typ); err != nil {
			return result, err
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
