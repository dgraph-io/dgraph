package worker

import "github.com/dgraph-io/dgraph/x"

// getInequalityTokens gets tokens geq / leq compared to given token.
func getInequalityTokens(attr, ineqValueToken string, f string) ([]string, error) {
	it := pstore.NewIterator()
	defer it.Close()
	isGeqOrGt := f == "geq" || f == "gt"
	if isGeqOrGt {
		it.Seek(x.IndexKey(attr, ineqValueToken))
	} else {
		it.SeekForPrev(x.IndexKey(attr, ineqValueToken))
	}

	isPresent := it.Valid() && it.Value() != nil && it.Value().Size() > 0
	idxKey := x.Parse(it.Key().Data())
	if f == "eq" {
		if isPresent && idxKey.Term == ineqValueToken {
			return []string{ineqValueToken}, nil
		}
		return []string{}, nil
	}

	var out []string
	indexPrefix := x.ParsedKey{Attr: attr}.IndexPrefix()
	for it.Valid() && it.ValidForPrefix(indexPrefix) {
		k := x.Parse(it.Key().Data())
		x.AssertTrue(k != nil)
		out = append(out, k.Term)
		if isGeqOrGt {
			it.Next()
		} else {
			it.Prev()
		}
	}
	return out, nil
}
