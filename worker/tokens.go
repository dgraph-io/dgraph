package worker

import (
	"github.com/dgraph-io/dgraph/tok"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

//  Might want to allow user to replace this.
var termTokenizer tok.TermTokenizer
var fullTextTokenizer tok.FullTextTokenizer

func getStringTokens(funcArgs []string, funcType FuncType) ([]string, error) {
	switch funcType {
	case FullTextSearchFn:
		return tokenize(funcArgs, fullTextTokenizer)
	default:
		return tokenize(funcArgs, termTokenizer)
	}
}

func getTokens(funcArgs []string) ([]string, error) {
	return tokenize(funcArgs, termTokenizer)
}

func tokenize(funcArgs []string, tokenizer tok.Tokenizer) ([]string, error) {
	if len(funcArgs) != 1 {
		return nil, x.Errorf("Function requires 1 arguments, but got %d",
			len(funcArgs))
	}
	sv := types.Val{types.StringID, funcArgs[0]}
	return tokenizer.Tokens(sv)
}

// getInequalityTokens gets tokens geq / leq compared to given token.
func getInequalityTokens(attr, ineqValueToken string, f string) ([]string, error) {
	it := pstore.NewIterator()
	defer it.Close()
	it.Seek(x.IndexKey(attr, ineqValueToken))

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
	isGeqOrGt := f == "geq" || f == "gt"

	if !isGeqOrGt && idxKey.Term != ineqValueToken {
		it.Prev()
	}
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
