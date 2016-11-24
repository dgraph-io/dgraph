package worker

import (
	"strings"

	"github.com/dgraph-io/dgraph/tok"
	"github.com/dgraph-io/dgraph/x"
)

func getTokens(funcArgs []string) ([]string, error) {
	x.AssertTruef(len(funcArgs) > 1, "Invalid function")
	funcName := strings.ToLower(funcArgs[0])
	switch funcName {
	case "anyof":
		if len(funcArgs) != 2 {
			return nil, x.Errorf("anyof function requires 2 arguments, but got %d",
				len(funcArgs))
		}
		return getStringTokens(funcArgs[1])
	case "allof":
		if len(funcArgs) != 2 {
			return nil, x.Errorf("allof function requires 2 arguments, but got %d",
				len(funcArgs))
		}
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
