package tok

import (
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

//  Might want to allow user to replace this.
var termTokenizer TermTokenizer

func GetTokens(funcArgs []string) ([]string, error) {
	if len(funcArgs) != 1 {
		return nil, x.Errorf("Function requires 1 arguments, but got %d",
			len(funcArgs))
	}
	sv := types.Val{types.StringID, funcArgs[0]}
	return termTokenizer.Tokens(sv)
}
