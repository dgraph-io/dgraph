package worker

import (
	"github.com/dgraph-io/dgraph/keys"
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
func getInequalityTokens(attr, ineqValueToken string, f string,
	pluginContexts []string) ([]string, error) {
	it := pstore.NewIterator()
	defer it.Close()
	it.Seek(keys.IndexKey(attr, ineqValueToken, pluginContexts))

	hit := it.Value() != nil && it.Value().Size() > 0
	if f == "eq" {
		if hit {
			return []string{ineqValueToken}, nil
		}
		return []string{}, nil
	}

	var out []string
	if hit {
		out = []string{ineqValueToken}
	}

	indexPrefix := keys.ParsedKey{Attr: attr}.IndexPrefix()
	isGeqOrGt := f == "geq" || f == "gt"

	for {
		if isGeqOrGt {
			it.Next()
		} else {
			it.Prev()
		}
		if !it.ValidForPrefix(indexPrefix) {
			break
		}

		k := keys.Parse(it.Key().Data())
		x.AssertTrue(k != nil)
		out = append(out, k.Term)
	}
	return out, nil
}
