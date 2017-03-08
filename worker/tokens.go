package worker

import (
	"github.com/dgraph-io/dgraph/tok"
	"github.com/dgraph-io/dgraph/x"
)

// Return string tokens from function arguments. It maps funcion type to correct tokenizer.
// Note: regexp functions require regexp compilation of argument, not tokenization.
func getStringTokens(funcArgs []string, funcType FuncType) ([]string, error) {
	switch funcType {
	case FullTextSearchFn:
		return tok.GetTextTokens(funcArgs)
	default:
		return tok.GetTokens(funcArgs)
	}
}

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
