package worker

import "github.com/dgraph-io/dgraph/x"

// getInequalityTokens gets tokens geq / leq compared to given token.
func getInequalityTokens(attr, ineqValueToken string, f string) ([]string, error) {
	it := pstore.NewIterator()
	defer it.Close()
	it.Seek(x.IndexKey(attr, ineqValueToken))

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

	indexPrefix := x.ParsedKey{Attr: attr}.IndexPrefix()
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

		k := x.Parse(it.Key().Data())
		x.AssertTrue(k != nil)
		out = append(out, k.Term)
	}
	return out, nil
}
