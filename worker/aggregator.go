
package worker

import (
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)


func CouldApplyOpOn(agrtr, attr string) bool {
	if agrtr == "count" {
		return true
	}
	typ, err := schema.TypeOf(attr)
	if err != nil {
		return false
	}
	if !typ.IsScalar() {
		return false
	}
	switch agrtr {
	case "min":
		fallthrough
	case "max":
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
	default:
		return lh, x.Errorf("Invalid Aggregator provided")
	}

}
