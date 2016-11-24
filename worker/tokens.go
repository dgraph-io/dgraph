package worker

import (
	"bytes"
	"strings"

	"github.com/dgraph-io/dgraph/geo"
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/tok"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

func getTokens(funcArgs []string) ([]string, *geo.QueryData, error) {
	x.AssertTruef(len(funcArgs) > 1, "Invalid function")
	funcName := strings.ToLower(funcArgs[0])
	switch funcName {
	case "anyof":
		if len(funcArgs) != 2 {
			return nil, nil, x.Errorf("anyof function requires 2 arguments, but got %d",
				len(funcArgs))
		}
		tok, err := getStringTokens(funcArgs[1])
		return tok, nil, err
	case "allof":
		if len(funcArgs) != 2 {
			return nil, nil, x.Errorf("allof function requires 2 arguments, but got %d",
				len(funcArgs))
		}
		tok, err := getStringTokens(funcArgs[1])
		return tok, nil, err
	case "near":
		if len(funcArgs) != 3 {
			return nil, nil, x.Errorf("near function requires 3 arguments, but got %d",
				len(funcArgs))
		}
		return geo.QueryTokens(geo.QueryTypeNear, funcArgs[1], funcArgs[2])
	case "within":
		if len(funcArgs) != 2 {
			return nil, nil, x.Errorf("within function requires 2 arguments, but got %d",
				len(funcArgs))
		}
		return geo.QueryTokens(geo.QueryTypeWithin, funcArgs[1], "0")
	case "contains":
		if len(funcArgs) != 2 {
			return nil, nil, x.Errorf("contains function requires 2 arguments, but got %d",
				len(funcArgs))
		}
		return geo.QueryTokens(geo.QueryTypeContains, funcArgs[1], "0")
	case "intersects":
		if len(funcArgs) != 2 {
			return nil, nil, x.Errorf("intersects function requires 2 arguments, but got %d",
				len(funcArgs))
		}
		return geo.QueryTokens(geo.QueryTypeIntersects, funcArgs[1], "0")
	default:
		return nil, nil, x.Errorf("Invalid function")
	}
}

func getStringTokens(term string) ([]string, error) {
	tokenizer, err := tok.NewTokenizer([]byte(term))
	if err != nil {
		return nil, x.Errorf("Could not create tokenizer: %v", term)
	}
	defer tokenizer.Destroy()
	return tokenizer.Tokens(), nil
}

func filterUids(uids *task.List, values []*task.Value, q *geo.QueryData) *task.List {
	x.AssertTruef(len(values) == len(uids.Uids), "lengths not matching")
	rv := &task.List{}
	for i := 0; i < len(values); i++ {
		valBytes := values[i].Val
		if bytes.Equal(valBytes, nil) {
			continue
		}
		vType := values[i].ValType
		if types.TypeID(vType) != types.GeoID {
			continue
		}
		var g types.Geo
		if err := g.UnmarshalBinary(valBytes); err != nil {
			continue
		}

		if !q.MatchesFilter(g) {
			continue
		}

		// we matched the geo filter, add the uid to the list
		rv.Uids = append(rv.Uids, uids.Uids[i])
	}
	return rv
}
