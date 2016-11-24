package worker

import (
	"github.com/dgraph-io/dgraph/tok"
	"github.com/dgraph-io/dgraph/x"
)

func getTokens(funcArgs []string) ([]string, error) {
	x.AssertTruef(len(funcArgs) > 1, "Invalid function")
	switch funcArgs[0] {
	case "anyof":
		return getStringTokens(funcArgs[1])
	case "allof":
		return getStringTokens(funcArgs[1])
	default:
		return nil, x.Errorf("Invalid function")
	}
}

func getStringTokens(term string) ([]string, error) {
	tokenizer, err := tok.NewTokenizer([]byte(term))
	if err != nil {
		return nil, x.Errorf("Could not create tokenizer: %v", term)
	}
	defer tokenizer.Destroy()
	return tokenizer.Tokens(), nil
}
