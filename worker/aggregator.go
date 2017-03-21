package worker

import "github.com/dgraph-io/dgraph/types"

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
