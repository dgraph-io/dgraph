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

// getInequalityTokens gets tokens geq / leq value given in funcArgs.
// If value's token is in TokensTable, then we return the parsed value so
// that for the bucket of the first token, we will compare against the parsed
// value. Otherwise, we return the parsed value as nil.
func getInequalityTokens(attr string, geq bool,
	funcArgs []string) ([]string, *types.Value, error) {
	x.AssertTruef(len(funcArgs) > 1, "Invalid function")
	if len(funcArgs) != 2 {
		return nil, nil, x.Errorf("Function requires 2 arguments, but got %d %v",
			len(funcArgs), funcArgs)
	}

	tt := posting.GetTokensTable(attr)
	if tt == nil {
		return nil, nil, x.Errorf("Attribute %s is not indexed", attr)
	}

	// Parse given value and get token. There should be only one token.
	t := schema.TypeOf(attr)
	if t == nil || !t.IsScalar() {
		return nil, nil, x.Errorf("Attribute %s is not valid scalar type", attr)
	}

	schemaType := t.(types.Scalar)
	v := types.ValueForType(schemaType.ID())
	err := v.UnmarshalText([]byte(funcArgs[1]))
	if err != nil {
		return nil, nil, err
	}

	tokens, err := posting.IndexTokens(attr, v)
	if err != nil {
		return nil, nil, err
	}
	if len(tokens) != 1 {
		x.Printf("Expected only 1 token but got: %d %v", len(tokens), tokens)
	}

	var s string
	if geq {
		s = tt.GetNextOrEqual(tokens[0])
	} else {
		s = tt.GetPrevOrEqual(tokens[0])
	}
	if s == "" {
		return []string{}, nil, nil
	}
	// At least one token.
	out := make([]string, 0, 10)
	for s != "" {
		out = append(out, s)
		if geq {
			s = tt.GetNext(s)
		} else {
			s = tt.GetPrev(s)
		}
	}
	if tokens[0] == out[0] {
		return out, &v, nil
	}
	return out, nil, nil
}
