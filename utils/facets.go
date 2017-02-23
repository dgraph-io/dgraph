package utils

import (
	"sort"

	"github.com/dgraph-io/dgraph/tok"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/types/facets"
	"github.com/dgraph-io/dgraph/x"
)

// FacetFor returns Facet for given key and val.
func FacetFor(key, val string) (*facets.Facet, error) {
	v, vt, err := facets.ValAndValType(val)
	if err != nil {
		return nil, err
	}

	// convert facet val interface{} to binary
	tid := types.TypeIDFor(&facets.Facet{ValType: vt})
	fVal := &types.Val{Tid: types.BinaryID}
	if err = types.Marshal(types.Val{Tid: tid, Value: v}, fVal); err != nil {
		return nil, err
	}

	fval, ok := fVal.Value.([]byte)
	if !ok {
		return nil, x.Errorf("Error while marshalling types.Val into binary.")
	}
	res := &facets.Facet{Key: key, Value: fval, ValType: vt}
	if vt == facets.Facet_STRING {
		// tokenize val.
		res.Tokens, err = tok.GetTokens([]string{val})
		if err == nil {
			sort.Strings(res.Tokens)
		}
	}
	return res, err
}
