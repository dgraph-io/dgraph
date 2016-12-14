package worker

import (
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/tok"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

func getTokens(funcArgs []string) ([]string, error) {
	x.AssertTruef(len(funcArgs) > 1, "Invalid function")
	if len(funcArgs) != 2 {
		return nil, x.Errorf("Function requires 2 arguments, but got %d",
			len(funcArgs))
	}
	return getStringTokens(funcArgs[1])
}

func getStringTokens(term string) ([]string, error) {
	tokenizer, err := tok.NewTokenizer([]byte(term))
	if err != nil {
		return nil, x.Errorf("Could not create tokenizer: %v", term)
	}
	defer tokenizer.Destroy()
	return tokenizer.Tokens(), nil
}

func getInequalityTokens(funcArgs []string) ([]string, error) {
	x.AssertTruef(len(funcArgs) > 1, "Invalid function")
	if len(funcArgs) != 3 {
		return nil, x.Errorf("Function requires 3 arguments, but got %d",
			len(funcArgs))
	}
	// Example: "geq releaseDate 2016-01-02".
	attr := funcArgs[1]
	tt := posting.GetTokensTable(attr)
	if tt == nil {
		return nil, x.Errorf("Attribute %s is not indexed", attr)
	}

	// Parse given value and get token. There should be only one token.
	t := schema.TypeOf(attr)
	if t == nil || !t.IsScalar() {
		return nil, x.Errorf("Attribute %s is not valid scalar type", attr)
	}

	schemaType := t.(types.Scalar)
	v := types.ValueForType(schemaType.ID())
	err := v.UnmarshalText([]byte(funcArgs[2]))
	if err != nil {
		return nil, err
	}
	return nil, nil
}
