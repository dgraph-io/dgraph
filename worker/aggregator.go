
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

func Aggregate(agrtr string, lh, rh *task.Value, typ types.TypeID) (*task.Value, error) {
	if len(lh.Val) == 0 {
		return rh, nil
	} else if len(rh.Val) == 0 {
		return lh, nil
	}
	if !couldApplyAgrtrOn(agrtr, typ) {
		return lh, x.Errorf("Cant apply aggregator %v on type %d\n", agrtr, typ)
	}
	lvh, _ := getValue(lh)
	va, erra := types.Convert(lvh, typ)
	if erra != nil {
		return lh, x.Errorf("Fail to convert from task.Value to types.Val")
	}
	rvh, _ := getValue(rh)
	vb, errb := types.Convert(rvh, typ)
	if errb != nil {
		return lh, x.Errorf("Fail to convert from task.Value to types.Val")
	}
	switch agrtr {
	case "min":
		if types.Less(va, vb) {
			return lh, nil
		} else {
			return rh, nil
		}
	case "max":
		if types.Less(va, vb) {
			return rh, nil
		} else {
			return lh, nil
		}
	case "sum":
		data := types.ValueForType(types.BinaryID)
		var err error
		if typ == types.Int32ID {
			num := va.Value.(int32) + vb.Value.(int32)
			intV := types.Val{types.Int32ID, num}
			err = types.Marshal(intV, &data)
		} else if typ == types.FloatID {
			num := va.Value.(float64) + vb.Value.(float64)
			floatV := types.Val{types.FloatID, num}
			err = types.Marshal(floatV, &data)
		}
		if err != nil {
			return lh, x.Errorf("Failed aggregator(sum) during Marshal")
		}
		lh.Val = data.Value.([]byte)
		return lh, nil
	default:
		return lh, x.Errorf("Invalid Aggregator provided")
	}
}
