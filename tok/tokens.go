package tok

import (
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

//  Might want to allow user to replace this.
var termTokenizer TermTokenizer
var fullTextTokenizer FullTextTokenizer

func GetTokens(funcArgs []string) ([]string, error) {
	return tokenize(funcArgs, termTokenizer)
}

func GetTextTokens(funcArgs []string, lang string) ([]string, error) {
	t, found := GetTokenizer("fulltext" + lang)
	x.AssertTruef(found, "Tokenizer not found for %s", "fulltext"+lang)
	return tokenize(funcArgs, t)
}

func tokenize(funcArgs []string, tokenizer Tokenizer) ([]string, error) {
	if len(funcArgs) != 1 {
		return nil, x.Errorf("Function requires 1 arguments, but got %d",
			len(funcArgs))
	}
	sv := types.Val{types.StringID, funcArgs[0]}
	return tokenizer.Tokens(sv)
}
