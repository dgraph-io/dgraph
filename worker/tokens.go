package worker

import (
	"bytes"

	"github.com/dgraph-io/dgraph/geo"
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/tok"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

func getTokens(funcArgs []string) ([]string, *geo.QueryData, error) {
	x.AssertTruef(len(funcArgs) > 1, "Invalid function")
	switch funcArgs[0] {
	case "anyof":
		tok, err := getStringTokens(funcArgs[1])
		return tok, nil, err
	case "allof":
		tok, err := getStringTokens(funcArgs[1])
		return tok, nil, err
	case "near":
		return geo.QueryTokens(geo.QueryTypeNear, funcArgs[1], funcArgs[2])
	case "within":
		return geo.QueryTokens(geo.QueryTypeWithin, funcArgs[1], "0")
	case "contains":
		return geo.QueryTokens(geo.QueryTypeContains, funcArgs[1], "0")
	case "intersects":
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
