package worker

import (
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

func getTokens(funcArgs []string) ([]string, error) {
	x.AssertTruef(len(funcArgs) > 1, "Invalid function")
	if len(funcArgs) != 2 {
		return nil, x.Errorf("Function requires 2 arguments, but got %d",
			len(funcArgs))
	}
	return types.DefaultIndexKeys(funcArgs[1])
}

// getInequalityTokens gets tokens geq / leq compared to given token.
func getInequalityTokens(attr, ineqValueToken string, f string) ([]string, error) {
	tt := posting.GetTokensTable(attr)
	if tt == nil {
		return nil, x.Errorf("Attribute %s is not indexed", attr)
	}

	if f == "eq" {
		pos := tt.Get(ineqValueToken)
		if pos == -1 {
			return []string{}, nil
		}
		return []string{ineqValueToken}, nil
	}

	var s string
	isGeqOrGt := f == "geq" || f == "gt"
	if isGeqOrGt {
		s = tt.GetNextOrEqual(ineqValueToken)
	} else {
		s = tt.GetPrevOrEqual(ineqValueToken)
	}

	out := make([]string, 0, 10)
	for s != "" {
		out = append(out, s)
		if isGeqOrGt {
			s = tt.GetNext(s)
		} else {
			s = tt.GetPrev(s)
		}
	}
	return out, nil
}
