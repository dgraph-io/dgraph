
package worker

import (
	"github.com/dgraph-io/dgraph/x"
)


func Compare(cmp string, lv, rv int64) (bool, error) {
	switch cmp {
	case "leq":
		return lv <= rv, nil
	case "geq":
		return lv >= rv, nil
	case "lt":
		return lv < rv, nil
	case "gt":
		return lv > rv, nil
	case "eq":
		return lv == rv, nil
	}
	return true, x.Errorf("Invalid comparator")
}
