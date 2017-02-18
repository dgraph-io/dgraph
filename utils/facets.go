package utils

import (
	"sort"

	"github.com/dgraph-io/dgraph/tok"
	"github.com/dgraph-io/dgraph/types/facets"
)

// FacetFor returns Facet for given key and val.
func FacetFor(key, val string) (*facets.Facet, error) {
	_, vt, err := facets.ValAndValType(val)
	if err != nil {
		return nil, err
	}
	res := &facets.Facet{Key: key, Value: []byte(val), ValType: vt}
	if vt == facets.Facet_STRING {
		// tokenize val.
		res.Tokens, err = tok.GetTokens([]string{val})
		if err == nil {
			sort.Strings(res.Tokens)
		}
	}
	return res, err
}
