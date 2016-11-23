package worker

import (
	"github.com/dgraph-io/dgraph/geo"
	"github.com/dgraph-io/dgraph/tok"
	"github.com/dgraph-io/dgraph/x"
)

func getTokens(Func []string) (*geo.QueryType, []string, bool, error) {
	x.AssertTruef(len(Func) > 1, "Invalid function")
	switch Func[0] {
	case "anyof":
		tok, err := getStringTokens(Func[1])
		return nil, tok, false, err
	case "allof":
		tok, err := getStringTokens(Func[1])
		return nil, tok, true, err
	default:
		return nil, nil, false, x.Errorf("Invalid function")
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
